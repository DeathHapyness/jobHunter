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
		"Estágio em TI":                              true,
		"Estagiário(a) de Tecnologia":                true,
		"Jovem Aprendiz - Sistemas":                  true,
		"IT Intern":                                  true,
		"Estágio em Desenvolvimento":                 true,
		"Estágio em Marketing":                       false,
		"Assistente Administrativo":                  false,
	}
	for title, want := range cases {
		j := model.Job{Title: title}
		Apply(&j)
		if got := IsTech(j); got != want {
			t.Errorf("IsTech(%q)=%v want %v (area=%s)", title, got, want, j.Area)
		}
	}
}

func TestPlace(t *testing.T) {
	cases := []struct{ in, city, state, country string }{
		{"São Paulo, SP, Brasil", "São Paulo", "SP", "Brasil"},
		{"Curitiba, Paraná", "Curitiba", "PR", "Brasil"},
		{"Remote - Brasil", "", "", "Brasil"},
		{"Remote, United States | Seattle, WA", "Seattle", "WA", "Estados Unidos"},
		{"San Francisco, CA | New York City, NY", "San Francisco", "CA", "Estados Unidos"},
		{"Dublin, Ireland", "Dublin", "", "Irlanda"},
		{"Lisboa, Lisboa, Portugal", "Lisboa", "Lisboa", "Portugal"},
		{"Brasil", "", "", "Brasil"},
		{"Austin, Texas, United States", "Austin", "TX", "Estados Unidos"},
		{"", "", "", ""},
	}
	for _, c := range cases {
		city, state, country := Place(c.in)
		if city != c.city || state != c.state || country != c.country {
			t.Errorf("Place(%q) = %q,%q,%q want %q,%q,%q", c.in, city, state, country, c.city, c.state, c.country)
		}
	}
}

func TestCanonicalCity(t *testing.T) {
	j := model.Job{Title: "SRE", Location: "Sao Paulo, SP"}
	Apply(&j)
	if j.City != "São Paulo" || j.State != "SP" || j.Country != "Brasil" {
		t.Fatalf("got %q %q %q", j.City, j.State, j.Country)
	}
}

func TestLevelBR(t *testing.T) {
	cases := map[string]model.Level{
		"Estágio em TI": model.LevelIntern, "Estagiário(a) de Tecnologia": model.LevelIntern,
		"Jovem Aprendiz - Sistemas": model.LevelIntern, "Analista de Suporte I": model.LevelJunior,
		"Desenvolvedor Jr.": model.LevelJunior, "Assistente de TI": model.LevelJunior,
		"Software Engineer, New Grad": model.LevelJunior, "Analista de Sistemas Pleno": model.LevelMid,
	}
	for title, want := range cases {
		j := model.Job{Title: title}
		Apply(&j)
		if j.Level != want {
			t.Errorf("%q: level=%s want %s", title, j.Level, want)
		}
	}
}
