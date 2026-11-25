package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
)

// fixtureServer serve arquivos de testdata/ mapeando caminho -> arquivo.
func fixtureServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		file, ok := routes[key]
		if !ok {
			file, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", file))
		if err != nil {
			t.Errorf("fixture %s: %v", file, err)
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient() *httpx.Client {
	return httpx.New(httpx.Options{UserAgent: "vagas-test", RequestTimeout: 5 * time.Second, PerHost: 2, BaseBackoff: time.Millisecond})
}

func collect(t *testing.T, a Adapter, c config.Company) []model.Job {
	t.Helper()
	var out []model.Job
	err := a.Fetch(context.Background(), testClient(), c, func(j model.Job) error {
		out = append(out, j)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRegistryHasGreenhouse(t *testing.T) {
	if _, ok := Get("greenhouse"); !ok {
		t.Fatal("greenhouse not registered")
	}
}

func TestGreenhouse(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/v1/boards/anthropic/jobs": "greenhouse/jobs.json"})
	jobs := collect(t, &Greenhouse{BaseURL: srv.URL}, config.Company{Name: "Anthropic", Source: "greenhouse", ID: "anthropic"})
	if len(jobs) != 4 {
		t.Fatalf("want 4 jobs, got %d", len(jobs))
	}
	j := jobs[0]
	if j.SourceJobID != "5382750008" || j.Company != "Anthropic" || j.Source != "greenhouse" {
		t.Fatalf("ids wrong: %+v", j)
	}
	if !strings.HasPrefix(j.URL, "https://job-boards.greenhouse.io/anthropic/jobs/") {
		t.Fatalf("url: %s", j.URL)
	}
	if j.PostedAt.IsZero() || j.Remote != model.RemoteNo || j.Location == "" {
		t.Fatalf("fields not parsed: %+v", j)
	}
	if j.Description == "" || strings.Contains(j.Description, "<") || strings.Contains(j.Description, "&lt;") {
		t.Fatalf("description not cleaned: %q", j.Description[:min(200, len(j.Description))])
	}
}

func TestGreenhouseNotFound(t *testing.T) {
	srv := fixtureServer(t, nil)
	err := (&Greenhouse{BaseURL: srv.URL}).Fetch(context.Background(), testClient(),
		config.Company{Name: "X", ID: "nope"}, func(model.Job) error { return nil })
	if err == nil {
		t.Fatal("expected error for unknown board")
	}
}

func TestStreamArrayTruncated(t *testing.T) {
	err := streamArray(strings.NewReader(`{"jobs":[{"id":1},{"id":`), "jobs", func(dec *json.Decoder) error {
		var v map[string]any
		return dec.Decode(&v)
	})
	if err == nil {
		t.Fatal("truncated JSON must fail so jobs are not closed")
	}
}

func TestRemoteFromText(t *testing.T) {
	cases := map[string]model.Remote{
		"Remote": model.RemoteYes, "Híbrido": model.RemoteHybrid, "On-Site": model.RemoteNo,
		"Presencial": model.RemoteNo, "": model.RemoteUnknown, "Flexible": model.RemoteUnknown,
	}
	for in, want := range cases {
		if got := remoteFromText(in); got != want {
			t.Errorf("remoteFromText(%q)=%s want %s", in, got, want)
		}
	}
}
