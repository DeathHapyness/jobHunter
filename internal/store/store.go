// Package store persiste vagas, estado pessoal e histórico de coletas em
// SQLite (modernc.org/sqlite, sem CGO).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // driver "sqlite"

	"vagas/internal/model"
)

// ErrNotFound indica que o registro não existe.
var ErrNotFound = errors.New("não encontrado")

// Store encapsula o banco.
type Store struct {
	db *sql.DB
}

// Open abre (ou cria) o banco em path e aplica as migrações.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)" +
		"&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_pragma=temp_store(MEMORY)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Poucas conexões bastam para uso pessoal e mantêm a RAM baixa.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha o banco.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifica se o banco responde.
func (s *Store) Ping(ctx context.Context) error {
	var one int
	return s.db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migração %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

const upsertSQL = `
INSERT INTO jobs (source, source_job_id, company, title, description, location, city, country,
	remote, level, area, url, posted_at, first_seen, last_seen, closed_at, fingerprint, aggregator)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT (source, company, source_job_id) DO UPDATE SET
	title       = excluded.title,
	description = CASE WHEN excluded.description <> '' THEN excluded.description ELSE jobs.description END,
	location    = excluded.location,
	city        = excluded.city,
	country     = excluded.country,
	remote      = excluded.remote,
	level       = excluded.level,
	area        = excluded.area,
	url         = excluded.url,
	posted_at   = COALESCE(excluded.posted_at, jobs.posted_at),
	last_seen   = excluded.last_seen,
	closed_at   = NULL,
	fingerprint = excluded.fingerprint,
	aggregator  = excluded.aggregator`

// UpsertJobs grava um lote de vagas vistas em seenAt. Vagas que já existiam
// são atualizadas e reabertas; first_seen é preservado.
func (s *Store) UpsertJobs(ctx context.Context, jobs []model.Job, seenAt time.Time) error {
	if len(jobs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	seen := seenAt.Unix()
	for _, j := range jobs {
		if j.Fingerprint == "" {
			j.Fingerprint = model.Fingerprint(j.Company, j.Title, j.Location)
		}
		_, err := stmt.ExecContext(ctx,
			j.Source, j.SourceJobID, j.Company, j.Title, j.Description, j.Location, j.City, j.Country,
			string(orRemote(j.Remote)), string(orLevel(j.Level)), string(orArea(j.Area)),
			j.URL, nullTime(j.PostedAt), seen, seen, j.Fingerprint, j.Aggregator)
		if err != nil {
			return fmt.Errorf("upsert %s/%s/%s: %w", j.Source, j.Company, j.SourceJobID, err)
		}
	}
	return tx.Commit()
}

// CloseMissing fecha as vagas abertas de (source, company) que não foram
// vistas desde runStart. Só deve ser chamada após uma coleta completa e sem
// erros dessa empresa nessa fonte.
func (s *Store) CloseMissing(ctx context.Context, source, company string, runStart, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET closed_at = ?
		WHERE source = ? AND company = ? AND closed_at IS NULL AND last_seen < ?`,
		now.Unix(), source, company, runStart.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Run é o registro de uma coleta de uma empresa em uma fonte.
type Run struct {
	Source     string
	Company    string
	StartedAt  time.Time
	FinishedAt time.Time
	OK         bool
	Error      string
	Jobs       int
	Skipped    int // descartadas pelo filtro (ex.: fora de TI); não gravadas
	Closed     int64
}

// RecordRun grava o resultado de uma coleta e mantém só o histórico recente.
func (s *Store) RecordRun(ctx context.Context, r Run) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO collection_runs (source, company, started_at, finished_at, ok, error, jobs, closed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Source, r.Company, r.StartedAt.Unix(), r.FinishedAt.Unix(), r.OK, r.Error, r.Jobs, r.Closed)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM collection_runs WHERE finished_at < ?`,
		r.FinishedAt.Add(-30*24*time.Hour).Unix())
	return err
}

// SourceStatus resume a última coleta de cada fonte.
type SourceStatus struct {
	Source       string
	LastFinished time.Time
	Companies    int
	Failed       int
	Jobs         int
	LastError    string
}

// SourceStatuses considera a coleta mais recente de cada empresa e agrega por fonte.
func (s *Store) SourceStatuses(ctx context.Context) ([]SourceStatus, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH latest AS (
			SELECT r.* FROM collection_runs r
			WHERE r.id = (SELECT r2.id FROM collection_runs r2
			              WHERE r2.source = r.source AND r2.company = r.company
			              ORDER BY r2.finished_at DESC, r2.id DESC LIMIT 1)
		)
		SELECT source, MAX(finished_at), COUNT(*), SUM(ok = 0), SUM(jobs),
		       COALESCE((SELECT l2.company || ': ' || l2.error FROM latest l2
		                 WHERE l2.source = latest.source AND l2.ok = 0
		                 ORDER BY l2.finished_at DESC LIMIT 1), '')
		FROM latest GROUP BY source ORDER BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceStatus
	for rows.Next() {
		var st SourceStatus
		var fin int64
		if err := rows.Scan(&st.Source, &fin, &st.Companies, &st.Failed, &st.Jobs, &st.LastError); err != nil {
			return nil, err
		}
		st.LastFinished = time.Unix(fin, 0)
		out = append(out, st)
	}
	return out, rows.Err()
}

// SetStatus define o status pessoal de uma vaga.
func (s *Store) SetStatus(ctx context.Context, jobID int64, status model.Status) error {
	if !model.ValidStatus(status) {
		return fmt.Errorf("status inválido: %q", status)
	}
	return s.upsertState(ctx, jobID, `
		INSERT INTO job_state (job_id, status, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (job_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		jobID, string(status), time.Now().Unix())
}

// SetNotes define as anotações pessoais de uma vaga.
func (s *Store) SetNotes(ctx context.Context, jobID int64, notes string) error {
	return s.upsertState(ctx, jobID, `
		INSERT INTO job_state (job_id, notes, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (job_id) DO UPDATE SET notes = excluded.notes, updated_at = excluded.updated_at`,
		jobID, notes, time.Now().Unix())
}

func (s *Store) upsertState(ctx context.Context, jobID int64, q string, args ...any) error {
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil && isFKError(err) {
		return ErrNotFound
	}
	return err
}

// GetJob devolve uma vaga com o estado pessoal.
func (s *Store) GetJob(ctx context.Context, id int64) (model.JobView, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs j LEFT JOIN job_state st ON st.job_id = j.id WHERE j.id = ?", id)
	v, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// GetAppState lê um valor de app_state ("" se ausente).
func (s *Store) GetAppState(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM app_state WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetAppState grava um valor em app_state.
func (s *Store) SetAppState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func orRemote(r model.Remote) model.Remote {
	if r == "" {
		return model.RemoteUnknown
	}
	return r
}

func orLevel(l model.Level) model.Level {
	if l == "" {
		return model.LevelUnknown
	}
	return l
}

func orArea(a model.Area) model.Area {
	if a == "" {
		return model.AreaOther
	}
	return a
}

func isFKError(err error) bool {
	return err != nil && containsFold(err.Error(), "FOREIGN KEY constraint failed")
}
