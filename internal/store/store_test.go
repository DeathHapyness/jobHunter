package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"vagas/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func job(company, id, title string) model.Job {
	return model.Job{
		Source: "greenhouse", SourceJobID: id, Company: company, Title: title,
		Location: "São Paulo, Brasil", URL: "https://example.com/" + id,
		Level: model.LevelJunior, Area: model.AreaDevOps, Remote: model.RemoteYes,
	}
}

func TestUpsertPreservesFirstSeenAndState(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	t1 := time.Unix(1_700_000_000, 0)
	if err := s.UpsertJobs(ctx, []model.Job{job("Acme", "1", "DevOps Jr")}, t1); err != nil {
		t.Fatal(err)
	}
	jobs, _, err := s.ListJobs(ctx, Filter{})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("list: %v %d", err, len(jobs))
	}
	id := jobs[0].ID
	if err := s.SetStatus(ctx, id, model.StatusSaved); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotes(ctx, id, "falar com fulano"); err != nil {
		t.Fatal(err)
	}

	t2 := t1.Add(time.Hour)
	if err := s.UpsertJobs(ctx, []model.Job{job("Acme", "1", "DevOps Júnior")}, t2); err != nil {
		t.Fatal(err)
	}
	v, err := s.GetJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !v.FirstSeen.Equal(t1) || !v.LastSeen.Equal(t2) {
		t.Fatalf("first/last seen wrong: %v %v", v.FirstSeen, v.LastSeen)
	}
	if v.Status != model.StatusSaved || v.Notes != "falar com fulano" || v.Title != "DevOps Júnior" {
		t.Fatalf("state lost or title not updated: %+v", v)
	}
}

func TestCloseMissingAndReopen(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	t1 := time.Unix(1_700_000_000, 0)
	s.UpsertJobs(ctx, []model.Job{job("Acme", "1", "A"), job("Acme", "2", "B"), job("Other", "9", "C")}, t1)

	t2 := t1.Add(time.Hour)
	s.UpsertJobs(ctx, []model.Job{job("Acme", "1", "A")}, t2)
	n, err := s.CloseMissing(ctx, "greenhouse", "Acme", t2, t2)
	if err != nil || n != 1 {
		t.Fatalf("closed=%d err=%v", n, err)
	}
	open, total, _ := s.ListJobs(ctx, Filter{})
	if total != 2 || len(open) != 2 {
		t.Fatalf("want 2 open (Acme/1 and Other/9), got %d", total)
	}

	t3 := t2.Add(time.Hour)
	s.UpsertJobs(ctx, []model.Job{job("Acme", "2", "B")}, t3)
	_, total, _ = s.ListJobs(ctx, Filter{})
	if total != 3 {
		t.Fatalf("reappeared job should reopen, total=%d", total)
	}
}

func TestDedupPrefersATSOverAggregator(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	agg := job("Acme", "x1", "SRE Junior")
	agg.Source, agg.Aggregator = "adzuna", true
	direct := job("ACME", "1", "SRE Júnior")
	if err := s.UpsertJobs(ctx, []model.Job{agg, direct}, now); err != nil {
		t.Fatal(err)
	}
	jobs, total, err := s.ListJobs(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || jobs[0].Source != "greenhouse" {
		t.Fatalf("want only the greenhouse job, got total=%d %+v", total, jobs)
	}
}

func TestFilters(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	base := time.Unix(1_700_000_000, 0)
	a := job("Acme", "1", "Engenheiro de Infraestrutura")
	a.Description = "Kubernetes e Terraform"
	b := job("Beta", "2", "Desenvolvedor Backend")
	b.Level, b.Area, b.Remote = model.LevelSenior, model.AreaDev, model.RemoteNo
	b.Location, b.City = "Lisboa, Portugal", "Lisboa"
	s.UpsertJobs(ctx, []model.Job{a}, base)
	s.UpsertJobs(ctx, []model.Job{b}, base.Add(2*time.Hour))

	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"todas", Filter{}, 2},
		{"fts descrição sem acento", Filter{Query: "kubern"}, 1},
		{"fts título acento", Filter{Query: "infraestrutura"}, 1},
		{"empresa", Filter{Companies: []string{"Beta"}}, 1},
		{"nível+área", Filter{Levels: []string{"junior"}, Areas: []string{"devops"}}, 1},
		{"remoto", Filter{Remotes: []string{"nao"}}, 1},
		{"cidade", Filter{Cities: []string{"Lisboa", "Recife"}}, 1},
		{"novas desde visita", Filter{SeenSince: base.Add(time.Hour)}, 1},
		{"publicação", Filter{PostedSince: base.Add(time.Hour)}, 1},
		{"query com aspas", Filter{Query: `"terra`}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := s.ListJobs(ctx, c.f)
			if err != nil {
				t.Fatal(err)
			}
			if total != c.want {
				t.Fatalf("total=%d want %d", total, c.want)
			}
		})
	}

	jobs, _, _ := s.ListJobs(ctx, Filter{})
	s.SetStatus(ctx, jobs[0].ID, model.StatusDiscarded)
	if _, total, _ := s.ListJobs(ctx, Filter{HideDiscarded: true}); total != 1 {
		t.Fatalf("hide discarded: total=%d", total)
	}
	if _, total, _ := s.ListJobs(ctx, Filter{Statuses: []string{"descartada"}}); total != 1 {
		t.Fatalf("status filter: total=%d", total)
	}
}

func TestSetStatusUnknownJob(t *testing.T) {
	s := openTest(t)
	if err := s.SetStatus(context.Background(), 999, model.StatusSaved); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.SetStatus(context.Background(), 1, "xpto"); err == nil {
		t.Fatal("invalid status accepted")
	}
}

func TestRunsAndSourceStatus(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	s.RecordRun(ctx, Run{Source: "greenhouse", Company: "A", StartedAt: now, FinishedAt: now, OK: false, Error: "boom"})
	s.RecordRun(ctx, Run{Source: "greenhouse", Company: "A", StartedAt: now, FinishedAt: now.Add(time.Second), OK: true, Jobs: 5})
	s.RecordRun(ctx, Run{Source: "greenhouse", Company: "B", StartedAt: now, FinishedAt: now, OK: false, Error: "timeout"})
	st, err := s.SourceStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 || st[0].Companies != 2 || st[0].Failed != 1 || st[0].Jobs != 5 || st[0].LastError != "B: timeout" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestAppState(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if v, _ := s.GetAppState(ctx, "k"); v != "" {
		t.Fatal("expected empty")
	}
	s.SetAppState(ctx, "k", "1")
	s.SetAppState(ctx, "k", "2")
	if v, _ := s.GetAppState(ctx, "k"); v != "2" {
		t.Fatalf("got %q", v)
	}
}
