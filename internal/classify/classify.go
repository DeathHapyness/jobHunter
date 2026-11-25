// Package classify infere nível, área, modalidade e país a partir de título,
// local e descrição, usando regras de palavras-chave (sem LLM).
package classify

import (
	"strings"

	"vagas/internal/model"
)

type rule[T any] struct {
	value    T
	keywords []string // já normalizadas; casam como palavra/expressão inteira
}

// Ordem importa: a primeira regra que casar vence.
var levelRules = []rule[model.Level]{
	{model.LevelIntern, []string{"estagio", "estagiario", "estagiaria", "intern", "internship", "trainee", "aprendiz", "apprentice", "working student"}},
	{model.LevelLead, []string{"lead", "lider", "head of", "principal", "staff", "manager", "gerente", "coordenador", "coordenadora", "director", "diretor", "architect", "arquiteto"}},
	{model.LevelSenior, []string{"senior", "sr", "iii", "level 3", "l3"}},
	{model.LevelMid, []string{"pleno", "mid", "mid level", "ii", "level 2"}},
	{model.LevelJunior, []string{"junior", "jr", "entry level", "early career", "new grad", "graduate", "associate", "i"}},
}

var areaRules = []rule[model.Area]{
	{model.AreaSRE, []string{"sre", "site reliability", "reliability engineer", "confiabilidade"}},
	{model.AreaDevOps, []string{"devops", "dev ops", "devsecops", "platform engineer", "platform engineering", "build engineer", "release engineer", "ci cd"}},
	{model.AreaSecurity, []string{"security", "seguranca", "cybersecurity", "ciberseguranca", "soc", "pentest", "appsec", "infosec", "iam"}},
	{model.AreaCloud, []string{"cloud", "aws", "azure", "gcp", "nuvem"}},
	{model.AreaInfra, []string{"infrastructure", "infraestrutura", "infra", "sysadmin", "system administrator", "administrador de sistemas", "systems engineer", "network", "redes", "linux", "datacenter", "data center", "noc", "suporte de infraestrutura", "it operations", "operations engineer"}},
	{model.AreaData, []string{"data engineer", "engenheiro de dados", "engenheira de dados", "data scientist", "cientista de dados", "data analyst", "analista de dados", "machine learning", "ml engineer", "analytics", "bi"}},
	{model.AreaDev, []string{"software engineer", "software developer", "desenvolvedor", "desenvolvedora", "developer", "programador", "backend", "back end", "frontend", "front end", "full stack", "fullstack", "mobile", "ios", "android", "engenheiro de software", "engenheira de software"}},
}

var countryAliases = map[string]string{
	"brasil": "Brasil", "brazil": "Brasil", "br": "Brasil",
	"sp": "Brasil", "rj": "Brasil", "mg": "Brasil", "pr": "Brasil", "sc": "Brasil", "rs": "Brasil",
	"sao paulo": "Brasil", "rio de janeiro": "Brasil", "belo horizonte": "Brasil", "curitiba": "Brasil",
	"florianopolis": "Brasil", "porto alegre": "Brasil", "recife": "Brasil", "campinas": "Brasil",
	"united states": "Estados Unidos", "usa": "Estados Unidos", "us": "Estados Unidos", "united states of america": "Estados Unidos",
	"san francisco": "Estados Unidos", "new york": "Estados Unidos", "new york city": "Estados Unidos", "seattle": "Estados Unidos",
	"ca": "Estados Unidos", "ny": "Estados Unidos", "wa": "Estados Unidos", "tx": "Estados Unidos", "ma": "Estados Unidos",
	"united kingdom": "Reino Unido", "uk": "Reino Unido", "england": "Reino Unido", "london": "Reino Unido",
	"canada": "Canadá", "can": "Canadá", "ontario": "Canadá", "toronto": "Canadá",
	"portugal": "Portugal", "lisboa": "Portugal", "lisbon": "Portugal",
	"germany": "Alemanha", "deutschland": "Alemanha", "berlin": "Alemanha",
	"ireland": "Irlanda", "dublin": "Irlanda",
	"netherlands": "Holanda", "amsterdam": "Holanda",
	"spain": "Espanha", "espana": "Espanha", "madrid": "Espanha", "barcelona": "Espanha",
	"france": "França", "paris": "França",
	"mexico": "México", "argentina": "Argentina", "chile": "Chile", "colombia": "Colômbia",
	"india": "Índia", "japan": "Japão", "tokyo": "Japão", "singapore": "Singapura",
	"australia": "Austrália", "sydney": "Austrália", "switzerland": "Suíça", "zurich": "Suíça",
}

// Apply preenche os campos inferíveis que o adaptador deixou vazios ou
// desconhecidos. Campos já definidos pela fonte são respeitados.
func Apply(j *model.Job) {
	title := " " + model.Normalize(j.Title) + " "
	if j.Level == "" || j.Level == model.LevelUnknown {
		j.Level = match(levelRules, title, model.LevelUnknown)
	}
	if j.Area == "" || j.Area == model.AreaOther {
		j.Area = match(areaRules, title, model.AreaOther)
	}
	if j.Remote == "" || j.Remote == model.RemoteUnknown {
		j.Remote = remote(" " + model.Normalize(j.Location+" "+j.Title) + " ")
	}
	if c := Country(j.Location); c != "" {
		j.Country = c
	} else if c := Country(j.Country); c != "" {
		j.Country = c
	}
}

func match[T any](rules []rule[T], text string, fallback T) T {
	for _, r := range rules {
		for _, k := range r.keywords {
			if strings.Contains(text, " "+k+" ") {
				return r.value
			}
		}
	}
	return fallback
}

func remote(text string) model.Remote {
	switch {
	case containsAny(text, "hybrid", "hibrido", "hibrida"):
		return model.RemoteHybrid
	case containsAny(text, "remote", "remoto", "remota", "home office", "anywhere", "teletrabalho", "remote friendly"):
		return model.RemoteYes
	case containsAny(text, "presencial", "on site", "onsite", "in office"):
		return model.RemoteNo
	}
	return model.RemoteUnknown
}

// Country tenta extrair o país de um texto de localização. Percorre os
// segmentos (separados por vírgula, |, ; ou /) do fim para o começo e
// devolve o primeiro reconhecido. Vários países diferentes → "".
func Country(loc string) string {
	if strings.TrimSpace(loc) == "" {
		return ""
	}
	groups := strings.FieldsFunc(loc, func(r rune) bool { return r == '|' || r == ';' || r == '/' })
	found := ""
	for _, g := range groups {
		segs := strings.Split(g, ",")
		for i := len(segs) - 1; i >= 0; i-- {
			if c := lookupCountry(model.Normalize(segs[i])); c != "" {
				if found != "" && found != c {
					return ""
				}
				found = c
				break
			}
		}
	}
	return found
}

// lookupCountry tenta o segmento inteiro e depois sufixos dele, para casar
// "remoto brasil" ou "remote friendly united states".
func lookupCountry(n string) string {
	words := strings.Fields(n)
	for i := range words {
		if c, ok := countryAliases[strings.Join(words[i:], " ")]; ok {
			return c
		}
	}
	return ""
}

func containsAny(text string, keys ...string) bool {
	for _, k := range keys {
		if strings.Contains(text, " "+k+" ") {
			return true
		}
	}
	return false
}
