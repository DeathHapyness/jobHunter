package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"vagas/internal/adapters"
	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/store"
)

// fakeAdapter devolve vagas configuradas por empresa e, opcionalmente, falha
// depois de emitir algumas.
type fakeAdapter struct {
	mu    sync.Mutex
	jobs  map[string][]string // empresa -> ids
	fail  map[string]bool
	panic bool
}

func (*fakeAdapter) Source() string   { return "fake" }
func (*fakeAdapter) Aggregator() bool { return false }
func (f *fakeAdapter) Fetch(ctx context.Context, _ *httpx.Client, c config.Company, emit adapters.Emit) error {
	if f.panic {
		panic("boom")
	}
	f.mu.Lock()
	ids, fail := f.jobs[c.Name], f.fail[c.Name]
	f.mu.Unlock()
	for _, id := range ids {
		if err := emit(model.Job{SourceJobID: id, Title: "SRE Jr " + id, URL: "https://x/" + id, Location: "Remote"}); err != nil {
			return err
		}
	}
	if fail {
		return errors.New("página 2 falhou")
	}
	return nil
}

func setup(t *testing.T, fa *fakeAdapter) (*Collector, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	clock := time.Unix(1_700_000_000, 0)
	var mu sync.Mutex
	c := &Collector{
		Store: s, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Concurrency: 4, Timeout: 5 * time.Second,
		Lookup: func(src string) (adapters.Adapter, bool) {
			if src == "fake" {
				return fa, true
			}
			return nil, false
		},
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			clock = clock.Add(time.Second)
			return clock
		},
	}
	return c, s
}

func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}

func TestPartialFailureNeverClosesJobs(t *testing.T) {
	ctx := context.Background()
	fa := &fakeAdapter{jobs: map[string][]string{"Acme": ids(3)}, fail: map[string]bool{}}
	c, s := setup(t, fa)
	comps := []config.Company{{Name: "Acme", Source: "fake"}}

	if _, err := c.RunAll(ctx, comps); err != nil {
		t.Fatal(err)
	}
	// Segunda coleta lê só 1 vaga e falha: as outras 2 devem continuar abertas.
	fa.jobs["Acme"], fa.fail["Acme"] = ids(1), true
	sum, _ := c.RunAll(ctx, comps)
	if sum.Failed != 1 || sum.Closed != 0 {
		t.Fatalf("summary: %+v", sum)
	}
	if _, total, _ := s.ListJobs(ctx, store.Filter{}); total != 3 {
		t.Fatalf("partial failure closed jobs: open=%d", total)
	}
	// Coleta completa com 1 vaga: fecha as 2 que sumiram.
	fa.fail["Acme"] = false
	sum, _ = c.RunAll(ctx, comps)
	if sum.Closed != 2 {
		t.Fatalf("want 2 closed, got %+v", sum)
	}
}

func TestBatchingAndClassification(t *testing.T) {
	ctx := context.Background()
	fa := &fakeAdapter{jobs: map[string][]string{"Acme": ids(batchSize*2 + 7)}}
	c, s := setup(t, fa)
	c.RunAll(ctx, []config.Company{{Name: "Acme", Source: "fake"}})
	jobs, total, _ := s.ListJobs(ctx, store.Filter{Limit: 1})
	if total != batchSize*2+7 {
		t.Fatalf("total=%d", total)
	}
	if jobs[0].Level != model.LevelJunior || jobs[0].Area != model.AreaSRE || jobs[0].Remote != model.RemoteYes {
		t.Fatalf("not classified: %+v", jobs[0])
	}
}

func TestIsolationBetweenCompanies(t *testing.T) {
	ctx := context.Background()
	fa := &fakeAdapter{jobs: map[string][]string{"A": ids(2), "B": ids(2)}, fail: map[string]bool{"A": true}}
	c, s := setup(t, fa)
	sum, _ := c.RunAll(ctx, []config.Company{
		{Name: "A", Source: "fake"}, {Name: "B", Source: "fake"}, {Name: "C", Source: "inexistente"},
	})
	if sum.Failed != 2 {
		t.Fatalf("want A and C failed: %+v", sum)
	}
	st, _ := s.SourceStatuses(ctx)
	if len(st) != 2 {
		t.Fatalf("want runs for fake and inexistente: %+v", st)
	}
}

func TestPanicInAdapterIsContained(t *testing.T) {
	fa := &fakeAdapter{panic: true}
	c, _ := setup(t, fa)
	sum, err := c.RunAll(context.Background(), []config.Company{{Name: "A", Source: "fake"}})
	if err != nil || sum.Failed != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
}

func TestOnlyTechSkipsNonTech(t *testing.T) {
	ctx := context.Background()
	fa := &fakeAdapter{jobs: map[string][]string{"Acme": ids(2)}}
	c, s := setup(t, fa)
	c.OnlyTech = true
	c.Lookup = func(string) (adapters.Adapter, bool) { return mixedAdapter{}, true }
	c.RunAll(ctx, []config.Company{{Name: "Acme", Source: "mixed"}})
	jobs, total, _ := s.ListJobs(ctx, store.Filter{})
	if total != 1 || jobs[0].Title != "DevOps Engineer" {
		t.Fatalf("want only the tech job, got %d %+v", total, jobs)
	}
}

type mixedAdapter struct{}

func (mixedAdapter) Source() string   { return "mixed" }
func (mixedAdapter) Aggregator() bool { return false }
func (mixedAdapter) Fetch(_ context.Context, _ *httpx.Client, _ config.Company, emit adapters.Emit) error {
	emit(model.Job{SourceJobID: "1", Title: "DevOps Engineer", URL: "https://x/1"})
	return emit(model.Job{SourceJobID: "2", Title: "Senior Accountant", URL: "https://x/2"})
}
