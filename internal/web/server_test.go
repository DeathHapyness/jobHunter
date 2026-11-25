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

	"vagas/internal/model"
	"vagas/internal/store"
)

type fakeCollector struct{ triggered bool }

func (f *fakeCollector) Running() bool { return false }
func (f *fakeCollector) Trigger() bool { f.triggered = true; return true }

func setup(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	jobs := []model.Job{
		{Source: "greenhouse", SourceJobID: "1", Company: "Acme", Title: "SRE Júnior", URL: "https://x/1",
			Location: "São Paulo, Brasil", Country: "Brasil", Level: model.LevelJunior, Area: model.AreaSRE, Remote: model.RemoteYes},
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
