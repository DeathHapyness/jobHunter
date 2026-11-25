package classify

import (
	"testing"

	"vagas/internal/model"
)

func TestApply(t *testing.T) {
	cases := []struct {
		title, loc string
		level      model.Level
		area       model.Area
		remote     model.Remote
		country    string
	}{
		{"Analista DevOps Jr.", "São Paulo, SP", model.LevelJunior, model.AreaDevOps, model.RemoteUnknown, "Brasil"},
		{"Estágio em Infraestrutura", "Remoto - Brasil", model.LevelIntern, model.AreaInfra, model.RemoteYes, "Brasil"},
		{"Senior Site Reliability Engineer", "London, UK", model.LevelSenior, model.AreaSRE, model.RemoteUnknown, "Reino Unido"},
		{"Cloud Engineer (Híbrido)", "Lisboa, Portugal", model.LevelUnknown, model.AreaCloud, model.RemoteHybrid, "Portugal"},
		{"Staff Security Engineer", "San Francisco, CA | New York City, NY", model.LevelLead, model.AreaSecurity, model.RemoteUnknown, "Estados Unidos"},
		{"Accountant", "London, UK; Ontario, CAN", model.LevelUnknown, model.AreaOther, model.RemoteUnknown, ""},
		{"Desenvolvedor Backend Pleno", "Remote", model.LevelMid, model.AreaDev, model.RemoteYes, ""},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			j := model.Job{Title: c.title, Location: c.loc}
			Apply(&j)
			if j.Level != c.level || j.Area != c.area || j.Remote != c.remote || j.Country != c.country {
				t.Fatalf("got level=%s area=%s remote=%s country=%q", j.Level, j.Area, j.Remote, j.Country)
			}
		})
	}
}

func TestApplyKeepsSourceValues(t *testing.T) {
	j := model.Job{Title: "Remote SRE", Remote: model.RemoteNo, Level: model.LevelSenior}
	Apply(&j)
	if j.Remote != model.RemoteNo || j.Level != model.LevelSenior {
		t.Fatalf("source values overwritten: %+v", j)
	}
}

func TestIsTech(t *testing.T) {
	cases := map[string]bool{
		"Analista de Suporte de TI Jr":               true,
		"Site Reliability Engineer":                  true,
		"QA Engineer":                                true,
		"Help Desk Analyst":                          true,
		"Account Executive, Cloud":                   false,
		"3P Revenue Accounting & Reporting":          false,
		"Area Lead, Data Center Security Operations": true,
		"Legal Counsel, Infrastructure":              false,
		"Recruiter, Engineering":                     false,
		"Office Manager":                             false,
	}
	for title, want := range cases {
		j := model.Job{Title: title}
		Apply(&j)
		if got := IsTech(j); got != want {
			t.Errorf("IsTech(%q)=%v want %v (area=%s)", title, got, want, j.Area)
		}
	}
}

func TestCity(t *testing.T) {
	cases := map[string]string{
		"São Paulo, SP, Brasil":                 "São Paulo",
		"Remote - Brasil":                       "",
		"Remote, United States | Seattle, WA":   "Seattle",
		"San Francisco, CA | New York City, NY": "San Francisco",
		"Brasil":                                "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := City(in); got != want {
			t.Errorf("City(%q)=%q want %q", in, got, want)
		}
	}
}
