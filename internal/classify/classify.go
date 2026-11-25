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
	{model.LevelIntern, []string{"estagio", "estagiario", "estagiaria", "estagiarios", "estagiario a", "intern", "interns", "internship", "trainee", "aprendiz", "jovem aprendiz", "apprentice", "apprenticeship", "working student", "co op", "coop", "estudante"}},
	{model.LevelLead, []string{"lead", "lider", "head of", "principal", "staff", "manager", "gerente", "coordenador", "coordenadora", "director", "diretor", "architect", "arquiteto"}},
	{model.LevelSenior, []string{"senior", "sr", "iii", "level 3", "l3"}},
	{model.LevelMid, []string{"pleno", "mid", "mid level", "ii", "level 2"}},
	{model.LevelJunior, []string{"junior", "jr", "jr a", "entry level", "entry", "early career", "new grad", "graduate", "recem formado", "recem formada", "associate", "i", "nivel i", "nivel 1", "level 1", "l1", "assistente", "auxiliar"}},
}

var areaRules = []rule[model.Area]{
	{model.AreaSRE, []string{"sre", "site reliability", "reliability engineer", "confiabilidade"}},
	{model.AreaDevOps, []string{"devops", "dev ops", "devsecops", "platform engineer", "platform engineering", "build engineer", "release engineer", "ci cd"}},
	{model.AreaSecurity, []string{"security", "seguranca", "cybersecurity", "ciberseguranca", "soc", "pentest", "appsec", "infosec", "iam"}},
	{model.AreaCloud, []string{"cloud", "aws", "azure", "gcp", "nuvem"}},
	{model.AreaInfra, []string{"infrastructure", "infraestrutura", "infra", "sysadmin", "system administrator", "systems administrator", "administrador de sistemas", "systems engineer", "network", "networking", "redes", "linux", "unix", "windows server", "datacenter", "data center", "noc", "suporte de infraestrutura", "it operations", "operations engineer", "help desk", "helpdesk", "service desk", "suporte tecnico", "it support", "suporte de ti", "suporte ti", "analista de ti", "tecnico de ti", "tecnico em ti", "analista de suporte", "dba", "database administrator", "storage", "virtualizacao", "virtualization", "kubernetes", "it specialist", "it engineer", "it analyst", "it technician", "desktop support", "field engineer", "production engineer", "observability", "telecom"}},
	{model.AreaData, []string{"data engineer", "engenheiro de dados", "engenheira de dados", "data scientist", "cientista de dados", "data analyst", "analista de dados", "machine learning", "ml engineer", "ml", "ai engineer", "applied ai", "llm", "research engineer", "analytics engineer", "data platform", "bi", "business intelligence", "big data"}},
	{model.AreaDev, []string{"software engineer", "software developer", "software engineering", "engineer software", "desenvolvedor", "desenvolvedora", "developer", "programador", "programadora", "backend", "back end", "frontend", "front end", "full stack", "fullstack", "mobile", "ios", "android", "engenheiro de software", "engenheira de software", "qa", "quality assurance", "test engineer", "sdet", "tester", "analista de testes", "web developer", "golang", "java", "python", "react", "node", "net", "php", "ruby", "rust", "embedded", "firmware", "systems programmer", "analista de sistemas", "analista desenvolvedor", "engenheiro de computacao", "computer engineer", "technical support engineer", "support engineer", "desenvolvimento de software", "desenvolvimento", "programacao", "engenharia de software", "software"}},
	// Termos genéricos de TI (por último, para não roubar áreas mais específicas).
	{model.AreaInfra, []string{"ti", "it", "tecnologia", "tecnologia da informacao", "information technology", "sistemas", "sistemas de informacao", "computacao", "ciencia da computacao", "informatica", "suporte tecnico", "tech", "technology"}},
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

// nonTech são termos de título que indicam vaga fora de TI mesmo quando há
// uma palavra técnica (ex.: "Account Executive, Cloud").
var nonTech = []string{
	"account executive", "account manager", "sales", "vendas", "vendedor", "vendedora", "comercial",
	"business development", "recruiter", "recrutador", "recrutadora", "talent acquisition", "people partner",
	"marketing", "legal", "counsel", "attorney", "advogado", "advogada", "juridico", "paralegal",
	"accountant", "accounting", "contador", "contadora", "contabil", "finance", "financeiro", "financial", "tax", "payroll",
	"customer success", "customer support specialist", "sucesso do cliente", "communications", "public relations",
	"copywriter", "content writer", "designer grafico", "facilities", "executive assistant", "office manager",
	"physical security", "security officer", "security guard", "vigilante", "procurement", "compras",
}

// IsTech informa se a vaga é de TI: área técnica reconhecida e nenhum termo
// claramente não técnico no título.
func IsTech(j model.Job) bool {
	title := " " + model.Normalize(j.Title) + " "
	if containsAny(title, nonTech...) {
		return false
	}
	return j.Area != "" && j.Area != model.AreaOther
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
	city, state, country := Place(j.Location)
	if j.City == "" {
		j.City = city
	}
	j.City = CanonicalCity(j.City)
	if j.State == "" {
		j.State = state
	}
	switch {
	case Country(j.Location) != "":
		j.Country = Country(j.Location)
	case Country(j.Country) != "":
		j.Country = Country(j.Country)
	case country != "" && !strings.ContainsAny(j.Location, "|;/"):
		// Inferido do estado (ex.: "Seattle, WA"); só com um único local,
		// para não escolher um país arbitrário entre vários.
		j.Country = country
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

// countryOnly são nomes que identificam país/região, não cidade.
var countryOnly = map[string]bool{
	"brasil": true, "brazil": true, "united states": true, "usa": true, "united kingdom": true, "uk": true,
	"canada": true, "portugal": true, "germany": true, "ireland": true, "netherlands": true, "spain": true,
	"france": true, "mexico": true, "argentina": true, "chile": true, "colombia": true, "india": true,
	"japan": true, "singapore": true, "australia": true, "switzerland": true, "europe": true, "emea": true,
	"latam": true, "americas": true, "apac": true, "north america": true, "south america": true, "united states of america": true,
	"sweden": true, "poland": true, "italy": true, "turkiye": true, "turkey": true, "israel": true,
}

func containsAny(text string, keys ...string) bool {
	for _, k := range keys {
		if strings.Contains(text, " "+k+" ") {
			return true
		}
	}
	return false
}
