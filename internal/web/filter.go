package web

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"vagas/internal/model"
	"vagas/internal/store"
)

// formFilter é o filtro como aparece na URL (nomes em português, para links
// legíveis e compartilháveis).
type formFilter struct {
	Q         string
	Companies []string // empresa
	Sources   []string // fonte
	Levels    []string // nivel
	Areas     []string // area
	Remotes   []string // remoto
	Countries []string // pais
	City      string   // cidade
	Posted    string   // publicada: 24h | 7d | 30d
	OnlyNew   bool     // novas=1
	Statuses  []string // status
	ShowDisc  bool     // descartadas=1 mostra descartadas (ocultas por padrão)
	Sort      string   // ordem: recentes | empresa
	Page      int      // pagina
}

func parseFilter(q url.Values) formFilter {
	f := formFilter{
		Q:         strings.TrimSpace(q.Get("q")),
		Companies: clean(q["empresa"]),
		Sources:   clean(q["fonte"]),
		Levels:    clean(q["nivel"]),
		Areas:     clean(q["area"]),
		Remotes:   clean(q["remoto"]),
		Countries: clean(q["pais"]),
		City:      strings.TrimSpace(q.Get("cidade")),
		Posted:    q.Get("publicada"),
		OnlyNew:   q.Get("novas") == "1",
		Statuses:  clean(q["status"]),
		ShowDisc:  q.Get("descartadas") == "1",
		Sort:      q.Get("ordem"),
		Page:      1,
	}
	if p, err := strconv.Atoi(q.Get("pagina")); err == nil && p > 1 {
		f.Page = p
	}
	if f.Sort != string(store.SortCompany) {
		f.Sort = string(store.SortRecent)
	}
	if _, ok := postedDur[f.Posted]; !ok {
		f.Posted = ""
	}
	return f
}

var postedDur = map[string]time.Duration{"": 0, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

func (f formFilter) toStore(now, lastVisit time.Time) store.Filter {
	sf := store.Filter{
		Query: f.Q, Companies: f.Companies, Sources: f.Sources, Levels: f.Levels,
		Areas: f.Areas, Remotes: f.Remotes, Countries: f.Countries, City: f.City,
		Statuses: f.Statuses, Sort: store.Sort(f.Sort),
		HideDiscarded: !f.ShowDisc && !contains(f.Statuses, string(model.StatusDiscarded)),
		Limit:         pageSize, Offset: (f.Page - 1) * pageSize,
	}
	if d := postedDur[f.Posted]; d > 0 {
		sf.PostedSince = now.Add(-d)
	}
	if f.OnlyNew {
		// Sem visita anterior, tudo é novo.
		sf.SeenSince = lastVisit
	}
	return sf
}

// encode gera a query string; page 0 omite a paginação.
func (f formFilter) encode(page int) string {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	set("q", f.Q)
	set("cidade", f.City)
	set("publicada", f.Posted)
	for k, vals := range map[string][]string{
		"empresa": f.Companies, "fonte": f.Sources, "nivel": f.Levels, "area": f.Areas,
		"remoto": f.Remotes, "pais": f.Countries, "status": f.Statuses,
	} {
		for _, x := range vals {
			v.Add(k, x)
		}
	}
	if f.OnlyNew {
		v.Set("novas", "1")
	}
	if f.ShowDisc {
		v.Set("descartadas", "1")
	}
	if f.Sort == string(store.SortCompany) {
		v.Set("ordem", f.Sort)
	}
	if page > 1 {
		v.Set("pagina", strconv.Itoa(page))
	}
	return v.Encode()
}

// Active informa se algum filtro está ativo (para mostrar "limpar").
func (f formFilter) Active() bool {
	return f.encode(0) != ""
}

// Has informa se val está selecionado no grupo name (usado nos templates).
func (f formFilter) Has(name, val string) bool {
	var vals []string
	switch name {
	case "empresa":
		vals = f.Companies
	case "fonte":
		vals = f.Sources
	case "nivel":
		vals = f.Levels
	case "area":
		vals = f.Areas
	case "remoto":
		vals = f.Remotes
	case "pais":
		vals = f.Countries
	case "status":
		vals = f.Statuses
	case "publicada":
		return f.Posted == val
	case "ordem":
		return f.Sort == val
	}
	return contains(vals, val)
}

// Count devolve quantos valores estão selecionados no grupo.
func (f formFilter) Count(name string) int {
	switch name {
	case "empresa":
		return len(f.Companies)
	case "fonte":
		return len(f.Sources)
	case "pais":
		return len(f.Countries)
	}
	return 0
}

func clean(vals []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] && len(out) < 100 {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func contains(vals []string, v string) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

var (
	levelOpts = []option{
		{string(model.LevelIntern), "Estágio"}, {string(model.LevelJunior), "Júnior"},
		{string(model.LevelMid), "Pleno"}, {string(model.LevelSenior), "Sênior"},
		{string(model.LevelLead), "Lead"}, {string(model.LevelUnknown), "Não informado"},
	}
	areaOpts = []option{
		{string(model.AreaInfra), "Infra"}, {string(model.AreaDevOps), "DevOps"},
		{string(model.AreaSRE), "SRE"}, {string(model.AreaCloud), "Cloud"},
		{string(model.AreaSecurity), "Segurança"}, {string(model.AreaDev), "Dev"},
		{string(model.AreaData), "Dados"}, {string(model.AreaOther), "Outros"},
	}
	remoteOpts = []option{
		{string(model.RemoteYes), "Remoto"}, {string(model.RemoteHybrid), "Híbrido"},
		{string(model.RemoteNo), "Presencial"}, {string(model.RemoteUnknown), "Não informado"},
	}
	statusOpts = []option{
		{string(model.StatusNew), "Nova"}, {string(model.StatusSeen), "Vista"},
		{string(model.StatusSaved), "Salva"}, {string(model.StatusApplied), "Aplicada"},
		{string(model.StatusDiscarded), "Descartada"},
	}
	postedOpts = []option{{"", "Qualquer data"}, {"24h", "Últimas 24h"}, {"7d", "Últimos 7 dias"}, {"30d", "Últimos 30 dias"}}
	sortOpts   = []option{{string(store.SortRecent), "Mais recentes"}, {string(store.SortCompany), "Empresa"}}
)
