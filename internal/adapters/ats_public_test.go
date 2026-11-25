package adapters

import (
	"strings"
	"testing"

	"vagas/internal/config"
	"vagas/internal/model"
)

// company monta uma empresa apontando o adaptador para o servidor de fixtures.
func company(name, id, base string) config.Company {
	return config.Company{Name: name, ID: id, Params: map[string]string{"base_url": base}}
}

func checkCommon(t *testing.T, jobs []model.Job, want int) {
	t.Helper()
	if len(jobs) != want {
		t.Fatalf("want %d jobs, got %d", want, len(jobs))
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		if j.SourceJobID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "http") {
			t.Fatalf("incomplete job: %+v", j)
		}
		if seen[j.SourceJobID] {
			t.Fatalf("duplicate id %s", j.SourceJobID)
		}
		seen[j.SourceJobID] = true
		if strings.Contains(j.Description, "<p>") || strings.Contains(j.Description, "&lt;") {
			t.Fatalf("description not cleaned: %.120q", j.Description)
		}
	}
}

func TestAllPhase2Registered(t *testing.T) {
	for _, s := range []string{"greenhouse", "lever", "ashby", "workable", "smartrecruiters", "recruitee",
		"personio", "teamtailor", "bamboohr", "breezy", "pinpoint"} {
		if _, ok := Get(s); !ok {
			t.Errorf("%s not registered", s)
		}
	}
}

func TestLever(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/v0/postings/spotify": "lever/postings.json"})
	jobs := collect(t, &Lever{}, company("Spotify", "spotify", srv.URL))
	checkCommon(t, jobs, 3)
	j := jobs[0]
	if j.PostedAt.IsZero() || j.Location == "" || j.Description == "" {
		t.Fatalf("fields: %+v", j)
	}
	if !strings.HasPrefix(j.URL, "https://jobs.lever.co/spotify/") {
		t.Fatalf("url %s", j.URL)
	}
}

func TestAshbySkipsUnlisted(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/posting-api/job-board/elevenlabs": "ashby/board.json"})
	jobs := collect(t, &Ashby{}, company("ElevenLabs", "elevenlabs", srv.URL))
	checkCommon(t, jobs, 2) // a 3ª tem isListed=false
	if jobs[0].Remote != model.RemoteYes || jobs[0].PostedAt.IsZero() {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestWorkable(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/api/v1/widget/accounts/huggingface": "workable/account.json"})
	jobs := collect(t, &Workable{}, company("Hugging Face", "huggingface", srv.URL))
	checkCommon(t, jobs, 3)
	if jobs[0].Remote != model.RemoteYes || jobs[0].Country == "" || jobs[0].PostedAt.IsZero() {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestSmartRecruitersPaginates(t *testing.T) {
	srv := fixtureServer(t, map[string]string{
		"/v1/companies/BoschGroup/postings?limit=100&offset=0": "smartrecruiters/page1.json",
		"/v1/companies/BoschGroup/postings?limit=100&offset=2": "smartrecruiters/page2.json",
	})
	jobs := collect(t, &SmartRecruiters{}, company("Bosch", "BoschGroup", srv.URL))
	checkCommon(t, jobs, 3)
	if !strings.HasPrefix(jobs[0].URL, "https://jobs.smartrecruiters.com/BoschGroup/") || jobs[0].Country == "" {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestRecruitee(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/api/offers/": "recruitee/offers.json"})
	jobs := collect(t, &Recruitee{}, company("bunq", "bunq", srv.URL))
	checkCommon(t, jobs, 3)
	if jobs[0].Remote != model.RemoteHybrid || jobs[0].PostedAt.IsZero() || jobs[0].Description == "" {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestPersonioXML(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/xml": "personio/feed.xml"})
	jobs := collect(t, &Personio{}, company("Personio", "personio", srv.URL))
	checkCommon(t, jobs, 2)
	j := jobs[0]
	if j.SourceJobID != "2885178" || j.Location != "Berlin" || j.Level != model.LevelJunior || j.PostedAt.IsZero() {
		t.Fatalf("fields: %+v", j)
	}
	if !strings.HasSuffix(j.URL, "/job/2885178") || j.Description == "" {
		t.Fatalf("url/desc: %s %q", j.URL, j.Description)
	}
}

func TestTeamtailorRSS(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/jobs.rss": "teamtailor/jobs.rss"})
	jobs := collect(t, &Teamtailor{}, company("Teamtailor", "career", srv.URL))
	checkCommon(t, jobs, 2)
	j := jobs[0]
	if j.Title != "Integration Success Specialist" || j.Country != "Sweden" || j.Remote != model.RemoteNo || j.PostedAt.IsZero() {
		t.Fatalf("fields: %+v", j)
	}
}

func TestBambooHR(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/careers/list": "bamboohr/list.json"})
	jobs := collect(t, &BambooHR{}, company("Acme", "acme", srv.URL))
	checkCommon(t, jobs, 2)
	if jobs[0].Remote != model.RemoteHybrid || jobs[0].Location != "Curitiba, PR, Brazil" {
		t.Fatalf("job0: %+v", jobs[0])
	}
	if jobs[1].SourceJobID != "43" || jobs[1].Remote != model.RemoteYes || jobs[1].Level != model.LevelIntern {
		t.Fatalf("job1: %+v", jobs[1])
	}
}

func TestBreezy(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/json": "breezy/jobs.json"})
	jobs := collect(t, &Breezy{}, company("Breezy", "breezy", srv.URL))
	checkCommon(t, jobs, 2)
	if jobs[0].Country != "United States" || jobs[0].PostedAt.IsZero() {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestPinpoint(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/postings.json": "pinpoint/postings.json"})
	jobs := collect(t, &Pinpoint{}, company("Pinpoint", "workwithus", srv.URL))
	checkCommon(t, jobs, 2)
	if jobs[0].Remote != model.RemoteYes || jobs[0].Description == "" {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestSubdomainRejectsBadID(t *testing.T) {
	if _, err := subdomainURL(config.Company{ID: "evil.com/x"}, "breezy.hr", ""); err == nil {
		t.Fatal("id with slash must be rejected")
	}
}
