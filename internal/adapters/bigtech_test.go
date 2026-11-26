package adapters

import (
	"context"
	"errors"
	"testing"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
)

func TestAmazon(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/en/search.json": "amazon/search.json"})
	jobs := collect(t, &Amazon{}, company("Amazon", "amazon", srv.URL))
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.SourceJobID != "10615039" || j.Location != "Mexico City, Mexico City, MX" || j.PostedAt.IsZero() || j.URL != srv.URL+"/en/jobs/10615039/senior-account-executive-amazon-ads" {
		t.Fatalf("fields: %+v", j)
	}
}

func TestApple(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/en-us/search": "apple/search.html"})
	c := company("Apple", "apple", srv.URL)
	c.Params["search"] = "devops"
	jobs := collect(t, &Apple{}, c)
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.SourceJobID != "114438346" || j.Title != "TH - Specialist" || j.Location == "" || j.PostedAt.IsZero() {
		t.Fatalf("fields: %+v", j)
	}
	if j.URL != srv.URL+"/en-us/details/114438346/th-specialist" || jobs[2].Remote != model.RemoteYes {
		t.Fatalf("url/remote: %+v", jobs)
	}
}

func TestMicrosoft(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/api/pcsx/search": "microsoft/search.json"})
	c := company("Microsoft", "microsoft", srv.URL)
	c.Params["delay_ms"] = "0"
	jobs := collect(t, &Microsoft{}, c)
	checkCommon(t, jobs, 3)
	j := jobs[1]
	if j.Title != "Research Intern - Machine Learning for Biology and Healthcare" || j.Location != "Redmond, WA, US" || j.Remote != model.RemoteNo || j.PostedAt.IsZero() {
		t.Fatalf("fields: %+v", j)
	}
}

func TestGoogleBlocked(t *testing.T) {
	a, _ := Get("google")
	if err := a.Fetch(context.Background(), testClient(), config.Company{}, nil); !errors.Is(err, httpx.ErrDisallowed) {
		t.Fatalf("google must be blocked by robots: %v", err)
	}
}
