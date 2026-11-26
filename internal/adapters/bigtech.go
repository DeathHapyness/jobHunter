package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
)

// Sites de carreira próprios das big techs. Usam a mesma API que a página
// pública consome; todos frágeis (podem mudar sem aviso).

func init() {
	Register(&Amazon{})
	Register(&Apple{})
	Register(&Microsoft{})
	Register(&unavailable{source: "google", err: fmt.Errorf("%w: google.com/robots.txt proíbe /about/careers/applications/jobs/results", httpx.ErrDisallowed)})
}

// searchTerms devolve os termos de busca de params.search, separados por "|".
// Sem termos, faz uma única busca vazia (todas as vagas).
func searchTerms(c config.Company, def string) []string {
	s := c.Param("search")
	if s == "" {
		s = def
	}
	var out []string
	for _, t := range strings.Split(s, "|") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// ---------------------------------------------------------------- Amazon

// Amazon: GET https://www.amazon.jobs/en/search.json (só /internal é
// proibido no robots.txt). params: search (termos separados por "|",
// padrão: busca vazia), country (ISO-3, ex.: BRA), max_pages (× 100).
type Amazon struct{}

func (*Amazon) Source() string   { return "amazon" }
func (*Amazon) Aggregator() bool { return false }

func (a *Amazon) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	base := baseOr(c, "https://www.amazon.jobs")
	const limit = 100
	maxPages := intParam(c, "max_pages", 20)
	seen := map[string]bool{}
	var trunc error
	for _, term := range searchTerms(c, "") {
		for page := 0; ; page++ {
			if page >= maxPages {
				trunc = truncated("amazon", maxPages)
				break
			}
			kv := []string{"offset", strconv.Itoa(page * limit), "result_limit", strconv.Itoa(limit), "sort", "recent", "base_query", strings.TrimSpace(term)}
			if cc := c.Param("country"); cc != "" {
				kv = append(kv, "normalized_country_code[]", cc)
			}
			var r struct {
				Error any `json:"error"`
				Hits  int `json:"hits"`
				Jobs  []struct {
					ID       string `json:"id_icims"`
					Title    string `json:"title"`
					Location string `json:"normalized_location"`
					Posted   string `json:"posted_date"`
					Short    string `json:"description_short"`
					Path     string `json:"job_path"`
				} `json:"jobs"`
			}
			if err := getJSON(ctx, hc, base+"/en/search.json?"+query(kv...), &r); err != nil {
				return err
			}
			if r.Error != nil {
				return fmt.Errorf("amazon: %v", r.Error)
			}
			for _, j := range r.Jobs {
				if seen[j.ID] {
					continue
				}
				seen[j.ID] = true
				posted, _ := time.Parse("January 2, 2006", j.Posted)
				job := model.Job{
					SourceJobID: j.ID, Title: strings.TrimSpace(j.Title), URL: base + j.Path,
					Location: amazonLocation(j.Location), PostedAt: posted, Description: strings.TrimSpace(j.Short),
				}
				if err := emit(job); err != nil {
					return err
				}
			}
			if len(r.Jobs) < limit || (page+1)*limit >= r.Hits {
				break
			}
		}
	}
	return trunc
}

// amazonLocation troca o país ISO-3 final ("Mexico City, Mexico City, MEX")
// por ISO-2, que a classificação reconhece.
func amazonLocation(s string) string {
	i := strings.LastIndex(s, ", ")
	if i < 0 {
		return s
	}
	if cc, ok := iso3to2[s[i+2:]]; ok {
		return s[:i+2] + cc
	}
	return s
}

var iso3to2 = map[string]string{
	"USA": "US", "BRA": "BR", "CAN": "CA", "MEX": "MX", "GBR": "GB", "DEU": "DE", "FRA": "FR", "ESP": "ES",
	"ITA": "IT", "IRL": "IE", "NLD": "NL", "POL": "PL", "IND": "IN", "JPN": "JP", "CHN": "CN", "AUS": "AU",
	"SGP": "SG", "ISR": "IL", "ARG": "AR", "CHL": "CL", "COL": "CO", "PRT": "PT", "CZE": "CZ", "ROU": "RO",
	"ZAF": "ZA", "ARE": "AE", "SAU": "SA", "KOR": "KR", "TWN": "TW", "PHL": "PH", "VNM": "VN", "LUX": "LU",
	"SWE": "SE", "CRI": "CR", "EGY": "EG", "TUR": "TR", "AUT": "AT", "BEL": "BE", "CHE": "CH", "DNK": "DK",
}

// ---------------------------------------------------------------- Apple

// Apple: a página https://jobs.apple.com/en-us/search traz os resultados
// em window.__staticRouterHydrationData (20 por página). Sem filtro por
// área na URL, então busca por termos (params.search, separados por "|").
type Apple struct{}

func (*Apple) Source() string   { return "apple" }
func (*Apple) Aggregator() bool { return false }

const appleDefaultSearch = "devops|kubernetes|site reliability engineer|software engineer|infrastructure engineer|security engineer|data engineer|machine learning|backend|cloud"

var appleDataRe = regexp.MustCompile(`(?s)window\.__staticRouterHydrationData\s*=\s*JSON\.parse\((".*?")\);`)

func (a *Apple) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	base := baseOr(c, "https://jobs.apple.com")
	locale := c.Param("locale")
	if locale == "" {
		locale = "en-us"
	}
	maxPages := intParam(c, "max_pages", 5)
	seen := map[string]bool{}
	var trunc error
	for _, term := range searchTerms(c, appleDefaultSearch) {
		for page := 1; ; page++ {
			if page > maxPages {
				trunc = truncated("apple", maxPages)
				break
			}
			doc, err := getHTML(ctx, hc, base+"/"+locale+"/search?"+query("search", term, "sort", "newest", "page", strconv.Itoa(page)))
			if err != nil {
				return err
			}
			res, total, err := appleResults(doc)
			if err != nil {
				return err
			}
			for _, r := range res {
				if seen[r.ID] {
					continue
				}
				seen[r.ID] = true
				job := model.Job{
					SourceJobID: r.ID, Title: strings.TrimSpace(r.Title), Description: strings.TrimSpace(r.Summary),
					URL:      base + "/" + locale + "/details/" + r.ID + "/" + r.Slug,
					PostedAt: parseTime(r.Posted), Location: r.location(),
				}
				if r.Home {
					job.Remote = model.RemoteYes
				}
				if err := emit(job); err != nil {
					return err
				}
			}
			if len(res) == 0 || page*20 >= total {
				break
			}
		}
	}
	return trunc
}

type appleResult struct {
	ID        string `json:"positionId"`
	Title     string `json:"postingTitle"`
	Slug      string `json:"transformedPostingTitle"`
	Posted    string `json:"postDateInGMT"`
	Summary   string `json:"jobSummary"`
	Home      bool   `json:"homeOffice"`
	Locations []struct {
		City    string `json:"city"`
		State   string `json:"stateProvince"`
		Country string `json:"countryName"`
		Name    string `json:"name"`
	} `json:"locations"`
}

func (r appleResult) location() string {
	var parts []string
	for _, l := range r.Locations {
		s := joinNonEmpty(", ", l.City, l.State, l.Country)
		if s == "" {
			s = l.Name
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

func appleResults(doc string) ([]appleResult, int, error) {
	m := appleDataRe.FindStringSubmatch(doc)
	if m == nil {
		return nil, 0, errors.New("apple: dados da busca não encontrados (marcação mudou?)")
	}
	var raw string // o argumento de JSON.parse é uma string JS ≈ string JSON
	if err := json.Unmarshal([]byte(m[1]), &raw); err != nil {
		return nil, 0, fmt.Errorf("apple: %w", err)
	}
	var d struct {
		LoaderData struct {
			Search struct {
				Total   json.Number   `json:"totalRecords"`
				Results []appleResult `json:"searchResults"`
			} `json:"search"`
		} `json:"loaderData"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, 0, fmt.Errorf("apple: %w", err)
	}
	total, _ := strconv.Atoi(d.LoaderData.Search.Total.String())
	return d.LoaderData.Search.Results, total, nil
}

// ---------------------------------------------------------------- Microsoft

// Microsoft: GET https://apply.careers.microsoft.com/api/pcsx/search
// (permitido no robots.txt). 10 por página. params: search (termos "|"),
// location, max_pages (por termo).
type Microsoft struct{}

func (*Microsoft) Source() string   { return "microsoft" }
func (*Microsoft) Aggregator() bool { return false }

func (m *Microsoft) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	base := baseOr(c, "https://apply.careers.microsoft.com")
	const limit = 10
	maxPages := intParam(c, "max_pages", 60)
	// A API devolve 429 com rajadas; espaça as páginas (params.delay_ms).
	delay := time.Duration(intParam(c, "delay_ms", 2000)) * time.Millisecond
	seen := map[int64]bool{}
	first := true
	var trunc error
	for _, term := range searchTerms(c, "") {
		for page := 0; ; page++ {
			if page >= maxPages {
				trunc = truncated("microsoft", maxPages)
				break
			}
			if !first {
				if err := pause(ctx, delay); err != nil {
					return err
				}
			}
			first = false
			u := base + "/api/pcsx/search?" + query("domain", "microsoft.com", "query", strings.TrimSpace(term), "location", c.Param("location"),
				"start", strconv.Itoa(page*limit), "num", strconv.Itoa(limit), "sort_by", "timestamp")
			var r struct {
				Data struct {
					Count     int `json:"count"`
					Positions []struct {
						ID        int64    `json:"id"`
						Name      string   `json:"name"`
						Locations []string `json:"standardizedLocations"`
						Posted    int64    `json:"postedTs"`
						Where     string   `json:"workLocationOption"`
						URL       string   `json:"positionUrl"`
					} `json:"positions"`
				} `json:"data"`
			}
			if err := getJSON(ctx, hc, u, &r); err != nil {
				var se *httpx.StatusError
				if errors.As(err, &se) && se.Code == 429 && len(seen) > 0 {
					// Limite de requisições: para aqui e guarda o que já leu.
					return fmt.Errorf("microsoft: HTTP 429 após %d vagas: %w", len(seen), ErrTruncated)
				}
				return err
			}
			for _, p := range r.Data.Positions {
				if seen[p.ID] {
					continue
				}
				seen[p.ID] = true
				job := model.Job{
					SourceJobID: strconv.FormatInt(p.ID, 10), Title: strings.TrimSpace(p.Name), URL: base + p.URL,
					Location: strings.Join(p.Locations, "; "), Remote: microsoftRemote(p.Where),
				}
				if p.Posted > 0 {
					job.PostedAt = time.Unix(p.Posted, 0).UTC()
				}
				if err := emit(job); err != nil {
					return err
				}
			}
			if len(r.Data.Positions) == 0 || (page+1)*limit >= r.Data.Count {
				break
			}
		}
	}
	return trunc
}

func microsoftRemote(s string) model.Remote {
	switch strings.ToLower(s) {
	case "remote":
		return model.RemoteYes
	case "hybrid":
		return model.RemoteHybrid
	case "onsite":
		return model.RemoteNo
	}
	return model.RemoteUnknown
}

// pause espera d ou até o contexto ser cancelado.
func pause(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
