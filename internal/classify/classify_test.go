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
