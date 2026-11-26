package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/textutil"
)

// Adaptadores de sistemas de recrutamento brasileiros.

// ErrDiscontinued indica uma fonte que deixou de existir.
var ErrDiscontinued = errors.New("fonte descontinuada")

// ErrUnsupported indica uma fonte sem forma pública de listar vagas por empresa.
var ErrUnsupported = errors.New("fonte sem endpoint público por empresa")

func init() {
	Register(&Gupy{})
	Register(&Solides{})
	Register(&Inhire{})
	Register(&Pandape{})
	Register(&unavailable{source: "kenoby", err: fmt.Errorf("%w: jobs.kenoby.com não existe mais (Kenoby foi incorporada pela Gupy; use source: gupy)", ErrDiscontinued)})
	Register(&unavailable{source: "abler", err: fmt.Errorf("%w: a Abler só publica um sitemap global sem empresa", ErrUnsupported)})
}

// maxHTML limita páginas HTML lidas inteiras.
const maxHTML = 16 << 20

func getHTML(ctx context.Context, hc *httpx.Client, rawURL string) (string, error) {
	resp, err := hc.Do(ctx, httpx.Request{URL: rawURL, Header: http.Header{"Accept": {"text/html,application/xhtml+xml"}}})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxHTML))
	return string(b), err
}

var nextDataRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json"[^>]*>(.*?)</script>`)

// nextData extrai props.pageProps do JSON embutido por páginas Next.js.
func nextData(page string, v any) error {
	m := nextDataRe.FindStringSubmatch(page)
	if m == nil {
		return errors.New("__NEXT_DATA__ ausente na página")
	}
	var wrap struct {
		Props struct {
			PageProps json.RawMessage `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(m[1]), &wrap); err != nil {
		return fmt.Errorf("__NEXT_DATA__: %w", err)
	}
	return json.Unmarshal(wrap.Props.PageProps, v)
}

// ---------------------------------------------------------------- Gupy

// Gupy: página de carreiras https://{id}.gupy.io/ (liberada no robots.txt),
// que traz a lista de vagas no __NEXT_DATA__. Ignora bancos de talentos.
type Gupy struct{}

func (*Gupy) Source() string   { return "gupy" }
func (*Gupy) Aggregator() bool { return false }

func (g *Gupy) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "gupy.io", "")
	if err != nil {
		return err
	}
	base := baseOr(c, def)
	page, err := getHTML(ctx, hc, base+"/")
	if err != nil {
		return err
	}
	var pp struct {
		Jobs []struct {
			ID         json.Number `json:"id"`
			Title      string      `json:"title"`
			Type       string      `json:"type"`
			Department string      `json:"department"`
			Workplace  struct {
				Type    string `json:"workplaceType"`
				Address struct {
					Country string `json:"country"`
					State   string `json:"stateShortName"`
					City    string `json:"city"`
				} `json:"address"`
			} `json:"workplace"`
		} `json:"jobs"`
	}
	if err := nextData(page, &pp); err != nil {
		return fmt.Errorf("gupy: %w", err)
	}
	for _, j := range pp.Jobs {
		if j.Type == "vacancy_type_talent_pool" {
			continue
		}
		a := j.Workplace.Address
		job := model.Job{
			SourceJobID: j.ID.String(), Title: strings.TrimSpace(j.Title),
			URL:      base + "/jobs/" + url.PathEscape(j.ID.String()),
			Location: joinNonEmpty(", ", a.City, a.State, a.Country),
			City:     strings.TrimSpace(a.City), State: strings.ToUpper(strings.TrimSpace(a.State)), Country: a.Country,
			Remote: remoteFromText(j.Workplace.Type),
		}
		if j.Workplace.Type == "remote" {
			job.Remote = model.RemoteYes
		}
		switch j.Type {
		case "vacancy_type_internship", "vacancy_type_apprentice", "vacancy_type_trainee":
			job.Level = model.LevelIntern
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Sólides

// Sólides: GET https://apigw.solides.com.br/jobs/v3/home/vacancy?slug={id}&page=N
// (API usada pela própria página de carreiras {id}.vagas.solides.com.br).
type Solides struct{}

func (*Solides) Source() string   { return "solides" }
func (*Solides) Aggregator() bool { return false }

func (s *Solides) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	if _, err := subdomainURL(c, "vagas.solides.com.br", ""); err != nil {
		return err
	}
	api := baseOr(c, "https://apigw.solides.com.br") + "/jobs/v3/home/vacancy"
	maxPages := intParam(c, "max_pages", 50)
	for page := 1; page <= maxPages; page++ {
		var r struct {
			Success bool `json:"success"`
			Data    struct {
				TotalPages int `json:"totalPages"`
				Data       []struct {
					ID        json.Number `json:"id"`
					Title     string      `json:"title"`
					Desc      string      `json:"description"`
					Status    string      `json:"currentState"`
					Slug      string      `json:"slug"`
					Hidden    bool        `json:"isHiddenJob"`
					Home      bool        `json:"homeOffice"`
					JobType   string      `json:"jobType"`
					CreatedAt string      `json:"createdAt"`
					State     struct {
						Code string `json:"code"`
					} `json:"state"`
					City struct {
						Name string `json:"name"`
					} `json:"city"`
					Seniority []struct {
						Name string `json:"name"`
					} `json:"seniority"`
					Contract []struct {
						Name string `json:"name"`
					} `json:"recruitmentContractType"`
				} `json:"data"`
			} `json:"data"`
		}
		if err := getJSON(ctx, hc, api+"?"+query("slug", c.ID, "page", fmt.Sprint(page)), &r); err != nil {
			return err
		}
		for _, j := range r.Data.Data {
			if j.Hidden || (j.Status != "" && j.Status != "em_andamento") {
				continue
			}
			slug := j.Slug
			if slug == "" {
				slug = c.ID
			}
			job := model.Job{
				SourceJobID: j.ID.String(), Title: strings.TrimSpace(j.Title),
				URL:      "https://" + slug + ".vagas.solides.com.br/vaga/" + url.PathEscape(j.ID.String()),
				Location: joinNonEmpty(", ", j.City.Name, j.State.Code), City: j.City.Name, State: j.State.Code,
				Country: "Brasil", PostedAt: parseTime(j.CreatedAt), Description: textutil.HTMLToText(j.Desc),
				Remote: remoteFromText(j.JobType),
			}
			if j.Home {
				job.Remote = model.RemoteYes
			}
			for _, sn := range j.Seniority {
				if l := levelFromCode(sn.Name); l != model.LevelUnknown {
					job.Level = l
				}
			}
			for _, ct := range j.Contract {
				if n := model.Normalize(ct.Name); strings.Contains(n, "estagi") || strings.Contains(n, "aprendiz") || strings.Contains(n, "trainee") {
					job.Level = model.LevelIntern
				}
			}
			if err := emit(job); err != nil {
				return err
			}
		}
		if page >= r.Data.TotalPages || len(r.Data.Data) == 0 {
			return nil
		}
	}
	return truncated("solides", maxPages)
}

// ---------------------------------------------------------------- Inhire

// Inhire: GET https://api.inhire.app/job-posts/public/pages com X-Tenant: {id}
// (API pública da página de carreiras {id}.inhire.app).
type Inhire struct{}

func (*Inhire) Source() string   { return "inhire" }
func (*Inhire) Aggregator() bool { return false }

func (i *Inhire) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	site, err := subdomainURL(c, "inhire.app", "")
	if err != nil {
		return err
	}
	var r struct {
		Jobs []struct {
			Title     string `json:"displayName"`
			JobID     string `json:"jobId"`
			Status    string `json:"status"`
			Workplace string `json:"workplaceType"`
			Location  string `json:"location"`
		} `json:"jobsPage"`
	}
	req := httpx.Request{
		URL:    baseOr(c, "https://api.inhire.app") + "/job-posts/public/pages",
		Header: http.Header{"X-Tenant": {c.ID}},
	}
	if err := doJSON(ctx, hc, req, &r); err != nil {
		return err
	}
	for _, j := range r.Jobs {
		if j.Status != "" && j.Status != "published" {
			continue
		}
		job := model.Job{
			SourceJobID: j.JobID, Title: strings.TrimSpace(j.Title),
			URL:      site + "/vagas/" + url.PathEscape(j.JobID),
			Location: j.Location, Remote: remoteFromText(j.Workplace),
		}
		if strings.EqualFold(j.Location, "BR") {
			job.Location, job.Country = "Brasil", "Brasil"
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Pandapé

// Pandapé (InfoJobs): HTML de https://{id}.pandape.infojobs.com.br/Vacancy/Vacancies.
// Frágil: depende da marcação dos cartões de vaga.
type Pandape struct{}

func (*Pandape) Source() string   { return "pandape" }
func (*Pandape) Aggregator() bool { return false }

var (
	pandapeCardRe  = regexp.MustCompile(`(?s)<a[^>]+class="card card-vacancy[^"]*"[^>]+href="/Detail/(\d+)"[^>]*>(.*?)</a>`)
	pandapeTitleRe = regexp.MustCompile(`(?s)<h3[^>]*title="([^"]*)"`)
	pandapeInfoRe  = regexp.MustCompile(`(?s)</i>\s*</div>\s*([^<]+?)\s*</div>`)
)

func (p *Pandape) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "pandape.infojobs.com.br", "")
	if err != nil {
		return err
	}
	base := baseOr(c, def)
	page, err := getHTML(ctx, hc, base+"/Vacancy/Vacancies")
	if err != nil {
		return err
	}
	if !strings.Contains(page, `id="VacancyList"`) {
		return errors.New("pandape: lista de vagas não encontrada (marcação mudou?)")
	}
	for _, m := range pandapeCardRe.FindAllStringSubmatch(page, -1) {
		id, card := m[1], m[2]
		t := pandapeTitleRe.FindStringSubmatch(card)
		if t == nil {
			continue
		}
		job := model.Job{SourceJobID: id, Title: strings.TrimSpace(html.UnescapeString(t[1])), URL: base + "/Detail/" + id, Country: "Brasil"}
		infos := pandapeInfoRe.FindAllStringSubmatch(card, -1)
		for k, info := range infos {
			v := strings.TrimSpace(html.UnescapeString(info[1]))
			if k == 0 {
				job.Location = v // "São Paulo - SP"
				if city, uf, ok := strings.Cut(v, " - "); ok {
					job.City, job.State = strings.TrimSpace(city), strings.TrimSpace(uf)
				}
				continue
			}
			if r := remoteFromText(v); r != model.RemoteUnknown {
				job.Remote = r
			}
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- indisponíveis

// unavailable registra fontes do escopo que não podem ser coletadas, para que
// uma entrada em companies.yaml gere um erro claro no status das fontes.
type unavailable struct {
	source string
	err    error
}

func (u *unavailable) Source() string   { return u.source }
func (u *unavailable) Aggregator() bool { return false }
func (u *unavailable) Fetch(context.Context, *httpx.Client, config.Company, Emit) error {
	return u.err
}
