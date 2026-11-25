package adapters

import (
	"context"
	"errors"
	"testing"

	"vagas/internal/config"
	"vagas/internal/model"
)

func TestGupySkipsTalentPool(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/": "gupy/careers.html"})
	jobs := collect(t, &Gupy{}, company("Ambev", "ambev", srv.URL))
	checkCommon(t, jobs, 3) // 4ª é banco de talentos
	j := jobs[0]
	if j.Country != "Brasil" || j.State == "" || j.City == "" || j.URL != srv.URL+"/jobs/12848146" {
		t.Fatalf("fields: %+v", j)
	}
	if jobs[2].Level != model.LevelIntern {
		t.Fatalf("apprentice should be intern: %+v", jobs[2])
	}
}

func TestGupyMissingData(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/": "pandape/vacancies.html"})
	err := (&Gupy{}).Fetch(context.Background(), testClient(), company("X", "x", srv.URL), func(model.Job) error { return nil })
	if err == nil {
		t.Fatal("page without __NEXT_DATA__ must fail")
	}
}

func TestSolidesPaginates(t *testing.T) {
	srv := fixtureServer(t, map[string]string{
		"/jobs/v3/home/vacancy?page=1&slug=solides": "solides/page1.json",
		"/jobs/v3/home/vacancy?page=2&slug=solides": "solides/page2.json",
	})
	jobs := collect(t, &Solides{}, company("Sólides", "solides", srv.URL))
	checkCommon(t, jobs, 4)
	j := jobs[0]
	if j.State != "MG" || j.City != "Belo Horizonte" || j.PostedAt.IsZero() || j.Level != model.LevelIntern {
		t.Fatalf("fields: %+v", j)
	}
	if j.URL != "https://solides.vagas.solides.com.br/vaga/957225" {
		t.Fatalf("url: %s", j.URL)
	}
}

func TestInhire(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/job-posts/public/pages": "inhire/pages.json"})
	jobs := collect(t, &Inhire{}, company("Cora", "cora", srv.URL))
	checkCommon(t, jobs, 3)
	if jobs[0].Remote != model.RemoteYes || jobs[0].Country != "Brasil" {
		t.Fatalf("fields: %+v", jobs[0])
	}
}

func TestPandape(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/Vacancy/Vacancies": "pandape/vacancies.html"})
	jobs := collect(t, &Pandape{}, company("InfoJobs", "infojobs", srv.URL))
	checkCommon(t, jobs, 4)
	j := jobs[0]
	if j.Title != "Analista de Onboarding Jr." || j.State != "SP" || j.City != "São Paulo" || j.Remote != model.RemoteHybrid {
		t.Fatalf("fields: %+v", j)
	}
}

func TestUnavailableSources(t *testing.T) {
	for src, want := range map[string]error{"kenoby": ErrDiscontinued, "abler": ErrUnsupported} {
		a, ok := Get(src)
		if !ok {
			t.Fatalf("%s not registered", src)
		}
		err := a.Fetch(context.Background(), testClient(), config.Company{Name: "X", ID: "x"}, func(model.Job) error { return nil })
		if !errors.Is(err, want) {
			t.Fatalf("%s: want %v, got %v", src, want, err)
		}
	}
}
