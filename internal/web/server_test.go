package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"vagas/internal/collector"
	"vagas/internal/model"
	"vagas/internal/store"
)

type fakeCollector struct {
	triggered bool
	completed uint64
}

func (f *fakeCollector) Running() bool                  { return false }
func (f *fakeCollector) Trigger() bool                  { f.triggered = true; return true }
func (f *fakeCollector) Completed() uint64              { return f.completed }
func (f *fakeCollector) LastSummary() collector.Summary { return collector.Summary{New: 7} }

func setup(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	jobs := []model.Job{
		{Source: "greenhouse", SourceJobID: "1", Company: "Acme", Title: "SRE Júnior", URL: "https://x/1",
			Location: "São Paulo, SP, Brasil", City: "São Paulo", State: "SP", Country: "Brasil", Level: model.LevelJunior, Area: model.AreaSRE, Remote: model.RemoteYes},
		{Source: "greenhouse", SourceJobID: "2", Company: "Beta", Title: "Backend <script>", URL: "https://x/2",
			Level: model.LevelSenior, Area: model.AreaDev, Remote: model.RemoteNo},
	}
	if err := st.UpsertJobs(context.Background(), jobs, time.Now()); err != nil {
		t.Fatal(err)
	}
	srv, err := New(st, &fakeCollector{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return srv, st
}

func get(t *testing.T, h http.Handler, target string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "results")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndexRendersAndEscapes(t *testing.T) {
	srv, _ := setup(t)
	rec := get(t, srv, "/", false)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SRE Júnior") || !strings.Contains(body, "<b>2</b> vagas") {
		t.Fatal("jobs missing from page")
	}
	if strings.Contains(body, "Backend <script>") {
		t.Fatal("title not escaped")
	}
	if !strings.Contains(body, `<html`) {
		t.Fatal("full page expected")
	}
}

func TestFilterPartial(t *testing.T) {
	srv, _ := setup(t)
	rec := get(t, srv, "/?area=sre&remoto=sim", true)
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Fatal("HTMX request should get the partial")
	}
	if !strings.Contains(body, "<b>1</b> vaga") || strings.Contains(body, "Backend") {
		t.Fatalf("filter not applied: %s", body)
	}
}

func TestStatusAndNotes(t *testing.T) {
	srv, st := setup(t)
	jobs, _, _ := st.ListJobs(context.Background(), store.Filter{Companies: []string{"Acme"}})
	id := jobs[0].ID
	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	path := "/jobs/" + itoa(id)
	if rec := post(path+"/status", url.Values{"status": {"descartada"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Descartada") {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	if rec := post(path+"/notes", url.Values{"notes": {"ligar sexta"}}); !strings.Contains(rec.Body.String(), "ligar sexta") {
		t.Fatal("notes not rendered")
	}
	// Descartadas somem por padrão e voltam com descartadas=1.
	if body := get(t, srv, "/", true).Body.String(); strings.Contains(body, "SRE Júnior") {
		t.Fatal("discarded job should be hidden by default")
	}
	if body := get(t, srv, "/?descartadas=1", true).Body.String(); !strings.Contains(body, "SRE Júnior") {
		t.Fatal("discarded job should show with descartadas=1")
	}
	if rec := post(path+"/status", url.Values{"status": {"xpto"}}); rec.Code != 400 {
		t.Fatalf("invalid status accepted: %d", rec.Code)
	}
	if rec := post("/jobs/99999/status", url.Values{"status": {"salva"}}); rec.Code != 404 {
		t.Fatalf("unknown job: %d", rec.Code)
	}
}

func TestHealthzAndStatic(t *testing.T) {
	srv, _ := setup(t)
	if rec := get(t, srv, "/healthz", false); rec.Code != 200 {
		t.Fatalf("healthz %d", rec.Code)
	}
	if rec := get(t, srv, "/static/htmx.min.js", false); rec.Code != 200 {
		t.Fatalf("htmx %d", rec.Code)
	}
	if rec := get(t, srv, "/nada", false); rec.Code != 404 {
		t.Fatalf("unknown path %d", rec.Code)
	}
}

func TestFilterURLRoundTrip(t *testing.T) {
	q, _ := url.ParseQuery("q=sre&empresa=A&empresa=B&nivel=junior&publicada=7d&novas=1&ordem=empresa&pagina=2")
	f := parseFilter(q)
	again := parseFilter(mustParse(t, f.encode(f.Page)))
	if again.encode(again.Page) != f.encode(f.Page) || f.Page != 2 || len(f.Companies) != 2 || !f.OnlyNew {
		t.Fatalf("round trip mismatch: %+v", f)
	}
	if parseFilter(mustParse(t, "publicada=1ano&ordem=xx")).Posted != "" {
		t.Fatal("invalid posted value accepted")
	}
}

func mustParse(t *testing.T, s string) url.Values {
	v, err := url.ParseQuery(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestIsNewRespectsAge(t *testing.T) {
	now := time.Now()
	old := jobData{JobView: model.JobView{Job: model.Job{PostedAt: now.Add(-43 * 24 * time.Hour), FirstSeen: now}}}
	if old.IsNew() {
		t.Fatal("43-day-old job must not be new")
	}
	fresh := jobData{JobView: model.JobView{Job: model.Job{PostedAt: now.Add(-2 * 24 * time.Hour), FirstSeen: now}}}
	if !fresh.IsNew() {
		t.Fatal("2-day-old job should be new on first visit")
	}
	fresh.LastVisit = now.Add(time.Minute)
	if fresh.IsNew() {
		t.Fatal("job seen before last visit is not new")
	}
	f := parseFilter(mustParse(t, "novas=1")).toStore(now, time.Time{})
	if f.PostedSince.IsZero() || now.Sub(f.PostedSince) > newWindow+time.Second {
		t.Fatalf("novas filter must cap age: %v", f.PostedSince)
	}
}

func TestCollectPollsUntilDone(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "c.db"))
	defer st.Close()
	fc := &fakeCollector{completed: 3}
	srv, err := New(st, fc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/collect", nil))
	if !fc.triggered || !strings.Contains(rec.Body.String(), "/collect/status?apos=3") {
		t.Fatalf("collect should start polling: %s", rec.Body)
	}
	if rec := get(t, srv, "/collect/status?apos=3", true); rec.Header().Get("HX-Trigger") != "" {
		t.Fatal("not finished yet")
	}
	fc.completed = 4
	rec = get(t, srv, "/collect/status?apos=3", true)
	if rec.Header().Get("HX-Trigger") != "collected" || !strings.Contains(rec.Body.String(), "7 vagas novas") {
		t.Fatalf("finished status wrong: %v %s", rec.Header(), rec.Body)
	}
}

func TestPlacesCascade(t *testing.T) {
	srv, _ := setup(t)
	body := get(t, srv, "/locais", true).Body.String()
	if strings.Contains(body, `name="estado"`) || strings.Contains(body, `name="cidade"`) {
		t.Fatal("state/city must appear only after a country")
	}
	body = get(t, srv, "/locais?pais=Brasil", true).Body.String()
	if !strings.Contains(body, `name="estado"`) || strings.Contains(body, `name="cidade"`) {
		t.Fatalf("state should appear after country: %s", body)
	}
	body = get(t, srv, "/locais?pais=Brasil&estado=SP", true).Body.String()
	if !strings.Contains(body, `value="São Paulo"`) {
		t.Fatalf("city should appear after state: %s", body)
	}
	rec := get(t, srv, "/locais?estado=SP&cidade=S%C3%A3o+Paulo", true)
	if rec.Header().Get("HX-Trigger-After-Swap") != "refilter" {
		t.Fatal("orphan state/city must be pruned and trigger refilter")
	}
}
