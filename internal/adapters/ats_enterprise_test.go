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
	"vagas/internal/model"
)

func TestWorkdayPaginatesByPOSTBody(t *testing.T) {
	var offsets []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/wday/cxs/nvidia/NVIDIAExternalCareerSite/jobs" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		offsets = append(offsets, body["offset"].(float64))
		file := "page1.json"
		if body["offset"].(float64) > 0 {
			file = "page2.json"
		}
		data, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "workday", file))
		w.Write(data)
	}))
	defer srv.Close()
	c := config.Company{Name: "NVIDIA", ID: "nvidia", Params: map[string]string{"host": "wd5", "site": "NVIDIAExternalCareerSite", "base_url": srv.URL}}
	jobs := collect(t, &Workday{}, c)
	checkCommon(t, jobs, 3)
	if len(offsets) != 2 || offsets[1] != 2 {
		t.Fatalf("pagination offsets: %v", offsets)
	}
	if jobs[0].SourceJobID != "JR2030637" || !strings.HasPrefix(jobs[0].URL, srv.URL+"/NVIDIAExternalCareerSite/job/") {
		t.Fatalf("job0: %+v", jobs[0])
	}
	if jobs[0].Location != "US, CA, Remote" {
		t.Fatalf("location from path: %q", jobs[0].Location)
	}
}

func TestWorkdayRequiresParams(t *testing.T) {
	err := (&Workday{}).Fetch(context.Background(), testClient(), config.Company{ID: "x"}, func(model.Job) error { return nil })
	if err == nil {
		t.Fatal("missing host/site must fail")
	}
}

func TestWorkdayPosted(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	cases := map[string]int{"Posted Today": 0, "Posted Yesterday": 1, "Posted 3 Days Ago": 3, "Posted 30+ Days Ago": 30}
	for in, days := range cases {
		if got := workdayPosted(in, now); !got.Equal(now.AddDate(0, 0, -days)) {
			t.Errorf("%s -> %v", in, got)
		}
	}
	if !workdayPosted("", now).IsZero() {
		t.Error("empty should be zero")
	}
}

func TestICIMS(t *testing.T) {
	srv := fixtureServer(t, map[string]string{
		"/jobs/search?in_iframe=1&pr=0&ss=1": "icims/page0.html",
		"/jobs/search?in_iframe=1&pr=1&ss=1": "icims/page1.html",
	})
	jobs := collect(t, &ICIMS{}, company("Peraton", "peraton", srv.URL))
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.SourceJobID != "178240" || j.Title != "ARC Team Lead" || j.Location != "Fort Huachuca, AZ, US" {
		t.Fatalf("fields: %+v", j)
	}
	if j.PostedAt.IsZero() || j.Remote != model.RemoteNo || j.Description == "" || strings.Contains(j.URL, "in_iframe") {
		t.Fatalf("fields: %+v", j)
	}
}

func TestJobvite(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/nutanix/jobs": "jobvite/jobs.html"})
	jobs := collect(t, &Jobvite{}, company("Nutanix", "nutanix", srv.URL))
	checkCommon(t, jobs, 3)
	if jobs[0].Title != "Finance Manager" || jobs[0].Location != "San Jose, California" || jobs[0].SourceJobID != "oUQKAfwO" {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestOracle(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/hcmRestApi/resources/latest/recruitingCEJobRequisitions": "oracle/page.json"})
	c := config.Company{Name: "Oracle", Params: map[string]string{"host": "x", "site": "CX_45001", "base_url": srv.URL}}
	jobs := collect(t, &Oracle{}, c)
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.Title != "Senior Core Infrastructure Engineer" || j.Country != "Estados Unidos" || j.PostedAt.IsZero() {
		t.Fatalf("fields: %+v", j)
	}
	if j.URL != srv.URL+"/hcmUI/CandidateExperience/en/sites/CX_45001/job/348884" {
		t.Fatalf("url: %s", j.URL)
	}
}

func TestSuccessFactorsSitemap(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/en/jobs/sitemap.xml": "successfactors/sitemap.xml"})
	c := config.Company{Name: "SAP", Params: map[string]string{"base_url": srv.URL, "sitemap": "/en/jobs/sitemap.xml"}}
	jobs := collect(t, &SuccessFactors{}, c)
	checkCommon(t, jobs, 3) // a URL sem id é ignorada
	if jobs[1].Title != "Senior AI Engineer Fmd For Physical AI Data Analytics" || jobs[1].PostedAt.IsZero() {
		t.Fatalf("title from slug: %+v", jobs[1])
	}
}

func TestTaleoDiscontinued(t *testing.T) {
	a, _ := Get("taleo")
	if err := a.Fetch(context.Background(), testClient(), config.Company{}, nil); err == nil || !strings.Contains(err.Error(), "oracle") {
		t.Fatalf("taleo should point to oracle: %v", err)
	}
}

func TestSuccessFactorsRSS(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/sitemal.xml": "successfactors/rss.xml"})
	c := config.Company{Name: "John Deere", Params: map[string]string{"base_url": srv.URL}}
	jobs := collect(t, &SuccessFactors{}, c)
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.SourceJobID != "1401141500" || j.Title != "HR Stage" || j.Location != "Horst, Limburg, NL" || j.Description == "" {
		t.Fatalf("fields: %+v", j)
	}
}
