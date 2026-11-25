package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"vagas/internal/model"
)

// Sort define a ordenação da lista.
type Sort string

const (
	SortRecent  Sort = "recentes"
	SortCompany Sort = "empresa"
)

// Filter combina todos os filtros da interface. Campos vazios não filtram.
type Filter struct {
	Query     string
	Companies []string
	Sources   []string
	Levels    []string
	Areas     []string
	Remotes   []string
	Countries []string
	Cities    []string
	// PostedSince filtra por data de publicação (ou primeira vez vista,
	// quando a fonte não informa).
	PostedSince time.Time
	// SeenSince filtra vagas vistas pela primeira vez depois desse instante
	// (usado para "novas desde minha última visita").
	SeenSince     time.Time
	Statuses      []string
	HideDiscarded bool
	IncludeClosed bool
	Sort          Sort
	Limit         int
	Offset        int
}

const jobColumns = `j.id, j.source, j.source_job_id, j.company, j.title, j.description, j.location,
	j.city, j.country, j.remote, j.level, j.area, j.url, j.posted_at, j.first_seen, j.last_seen, j.closed_at,
	j.fingerprint, j.aggregator, COALESCE(st.status, 'nova'), COALESCE(st.notes, '')`

// ListJobs devolve uma página de vagas e o total que atende ao filtro.
// Vagas com o mesmo fingerprint aparecem uma vez só, priorizando a fonte
// direta (ATS) sobre agregadores.
func (s *Store) ListJobs(ctx context.Context, f Filter) ([]model.JobView, int, error) {
	where, args := buildWhere(f)
	from := " FROM jobs j LEFT JOIN job_state st ON st.job_id = j.id"
	if q := ftsQuery(f.Query); q != "" {
		from += " JOIN jobs_fts ON jobs_fts.rowid = j.id"
		where = append(where, "jobs_fts MATCH ?")
		args = append(args, q)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*)"+from+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := " ORDER BY COALESCE(j.posted_at, j.first_seen) DESC, j.id DESC"
	if f.Sort == SortCompany {
		order = " ORDER BY j.company COLLATE NOCASE, COALESCE(j.posted_at, j.first_seen) DESC, j.id DESC"
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := "SELECT " + jobColumns + from + cond + order + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]model.JobView, 0, limit)
	for rows.Next() {
		v, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func buildWhere(f Filter) ([]string, []any) {
	var where []string
	var args []any
	in := func(col string, vals []string) {
		vals = nonEmpty(vals)
		if len(vals) == 0 {
			return
		}
		where = append(where, col+" IN ("+placeholders(len(vals))+")")
		for _, v := range vals {
			args = append(args, v)
		}
	}
	if !f.IncludeClosed {
		where = append(where, "j.closed_at IS NULL")
	}
	// Deduplicação entre fontes.
	where = append(where, `NOT EXISTS (
		SELECT 1 FROM jobs d WHERE d.fingerprint = j.fingerprint AND d.id <> j.id
		AND (d.closed_at IS NULL OR j.closed_at IS NOT NULL)
		AND (d.aggregator < j.aggregator OR (d.aggregator = j.aggregator AND d.id < j.id)))`)
	in("j.company", f.Companies)
	in("j.source", f.Sources)
	in("j.level", f.Levels)
	in("j.area", f.Areas)
	in("j.remote", f.Remotes)
	in("j.country", f.Countries)
	in("j.city", f.Cities)
	if !f.PostedSince.IsZero() {
		where = append(where, "COALESCE(j.posted_at, j.first_seen) >= ?")
		args = append(args, f.PostedSince.Unix())
	}
	if !f.SeenSince.IsZero() {
		where = append(where, "j.first_seen > ?")
		args = append(args, f.SeenSince.Unix())
	}
	in("COALESCE(st.status, 'nova')", f.Statuses)
	if f.HideDiscarded {
		where = append(where, "COALESCE(st.status, 'nova') <> 'descartada'")
	}
	return where, args
}

// ftsQuery transforma a busca do usuário numa consulta FTS5 segura:
// cada termo vira um prefixo entre aspas, todos obrigatórios.
func ftsQuery(q string) string {
	var terms []string
	for _, t := range strings.Fields(q) {
		t = strings.ReplaceAll(t, `"`, "")
		if t == "" {
			continue
		}
		terms = append(terms, `"`+t+`"*`)
	}
	return strings.Join(terms, " ")
}

// Facets são os valores distintos disponíveis para os filtros.
type Facets struct {
	Companies []string
	Sources   []string
	Countries []string
	Cities    []string
}

// Facets lista valores distintos entre as vagas abertas.
func (s *Store) Facets(ctx context.Context) (Facets, error) {
	var f Facets
	var err error
	if f.Companies, err = s.distinct(ctx, "company"); err != nil {
		return f, err
	}
	if f.Sources, err = s.distinct(ctx, "source"); err != nil {
		return f, err
	}
	if f.Countries, err = s.distinct(ctx, "country"); err != nil {
		return f, err
	}
	f.Cities, err = s.distinct(ctx, "city")
	return f, err
}

func (s *Store) distinct(ctx context.Context, col string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT "+col+" FROM jobs WHERE closed_at IS NULL AND "+col+" <> '' ORDER BY "+col+" COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanJob(sc scanner) (model.JobView, error) {
	var v model.JobView
	var posted, closed sql.NullInt64
	var first, last int64
	var remote, level, area, status string
	err := sc.Scan(&v.ID, &v.Source, &v.SourceJobID, &v.Company, &v.Title, &v.Description, &v.Location,
		&v.City, &v.Country, &remote, &level, &area, &v.URL, &posted, &first, &last, &closed,
		&v.Fingerprint, &v.Aggregator, &status, &v.Notes)
	if err != nil {
		return v, err
	}
	v.Remote, v.Level, v.Area, v.Status = model.Remote(remote), model.Level(level), model.Area(area), model.Status(status)
	v.FirstSeen, v.LastSeen = time.Unix(first, 0), time.Unix(last, 0)
	if posted.Valid {
		v.PostedAt = time.Unix(posted.Int64, 0)
	}
	if closed.Valid {
		v.ClosedAt = time.Unix(closed.Int64, 0)
	}
	return v, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func nonEmpty(vals []string) []string {
	out := vals[:0:0]
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
