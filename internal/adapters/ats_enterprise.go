package adapters

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/textutil"
)

// Adaptadores de ATS corporativos com endpoints internos ou pouco
// documentados. Todos são considerados frágeis.

func init() {
	Register(&Workday{})
	Register(&ICIMS{})
	Register(&Jobvite{})
	Register(&Oracle{})
	Register(&SuccessFactors{})
	Register(&unavailable{source: "taleo", err: fmt.Errorf("%w: o Taleo clássico foi desativado (*.taleo.net não resolve); as empresas migraram para o Oracle Recruiting Cloud — use source: oracle", ErrDiscontinued)})
}

// ---------------------------------------------------------------- Workday

// Workday: POST https://{tenant}.{host}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs
// (API usada pela própria página de vagas). params: tenant (padrão: id),
// host (wd1…wd12, obrigatório), site (obrigatório), search (opcional),
// max_pages (padrão 50 × 20 vagas).
type Workday struct{}

func (*Workday) Source() string   { return "workday" }
func (*Workday) Aggregator() bool { return false }

func (w *Workday) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	tenant := c.Param("tenant")
	if tenant == "" {
		tenant = c.ID
	}
	host, site := c.Param("host"), c.Param("site")
	if tenant == "" || host == "" || site == "" {
		return errors.New("workday: params tenant/host/site obrigatórios (ex.: host: wd5, site: External)")
	}
	base := baseOr(c, "https://"+tenant+"."+host+".myworkdayjobs.com")
	api := base + "/wday/cxs/" + url.PathEscape(tenant) + "/" + url.PathEscape(site) + "/jobs"
	const limit = 20
	maxPages := intParam(c, "max_pages", 50)
	now := time.Now()
	total := 0
	for page, offset := 0, 0; page < maxPages; page++ {
		var r struct {
			Total int `json:"total"`
			Posts []struct {
				Title    string   `json:"title"`
				Path     string   `json:"externalPath"`
				Location string   `json:"locationsText"`
				PostedOn string   `json:"postedOn"`
				Bullets  []string `json:"bulletFields"`
			} `json:"jobPostings"`
		}
		body := map[string]any{"appliedFacets": map[string]any{}, "limit": limit, "offset": offset, "searchText": c.Param("search")}
		if err := postJSON(ctx, hc, api, body, nil, &r); err != nil {
			return err
		}
		for _, p := range r.Posts {
			id := p.Path
			if len(p.Bullets) > 0 && p.Bullets[0] != "" {
				id = p.Bullets[0]
			}
			loc := p.Location
			if strings.HasSuffix(loc, "Locations") || loc == "" {
				// "5 Locations": usa o local do caminho (/job/US-CA-Remote/...).
				if parts := strings.Split(strings.Trim(p.Path, "/"), "/"); len(parts) >= 2 {
					loc = strings.ReplaceAll(parts[1], "-", ", ")
				}
			}
			job := model.Job{
				SourceJobID: id, Title: strings.TrimSpace(p.Title), URL: base + "/" + site + p.Path,
				Location: loc, PostedAt: workdayPosted(p.PostedOn, now),
			}
			if err := emit(job); err != nil {
				return err
			}
		}
		// O Workday só informa o total na primeira página.
		if r.Total > 0 {
			total = r.Total
		}
		offset += len(r.Posts)
		if len(r.Posts) == 0 || offset >= total {
			return nil
		}
	}
	return truncated("workday", maxPages)
}

var workdayDaysRe = regexp.MustCompile(`(\d+)\+?\s+Days?`)

// workdayPosted converte "Posted Today", "Posted Yesterday", "Posted 3 Days Ago".
func workdayPosted(s string, now time.Time) time.Time {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "today"):
		return now
	case strings.Contains(l, "yesterday"):
		return now.AddDate(0, 0, -1)
	}
	if m := workdayDaysRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return now.AddDate(0, 0, -n)
	}
	return time.Time{}
}

// ---------------------------------------------------------------- iCIMS

// iCIMS: HTML de https://careers-{id}.icims.com/jobs/search?pr=N&in_iframe=1.
// params.host substitui o host (ex.: "jobs-acme.icims.com").
type ICIMS struct{}

func (*ICIMS) Source() string   { return "icims" }
func (*ICIMS) Aggregator() bool { return false }

var (
	icimsCardRe   = regexp.MustCompile(`(?s)<li class="iCIMS_JobCardItem">(.*?)</li>`)
	icimsLinkRe   = regexp.MustCompile(`(?s)<a href="([^"]*/jobs/(\d+)/[^"]*)"[^>]*class="iCIMS_Anchor"[^>]*>.*?<h3[^>]*>\s*(.*?)\s*</h3>`)
	icimsLocRe    = regexp.MustCompile(`(?s)Job Locations</span>\s*<span[^>]*>\s*(.*?)\s*</span>`)
	icimsDateRe   = regexp.MustCompile(`<span title="(\d{1,2}/\d{1,2}/\d{4} \d{1,2}:\d{2} [AP]M)"`)
	icimsRemoteRe = regexp.MustCompile(`(?s)Telecommute Options</dt>\s*<dd[^>]*><span[^>]*>\s*(.*?)\s*</span>`)
	icimsDescRe   = regexp.MustCompile(`(?s)<div class="col-xs-12 description">(.*?)</div>`)
)

func (i *ICIMS) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	host := c.Param("host")
	if host == "" {
		if _, err := subdomainURL(c, "icims.com", ""); err != nil {
			return err
		}
		host = "careers-" + c.ID + ".icims.com"
	}
	base := baseOr(c, "https://"+host)
	maxPages := intParam(c, "max_pages", 30)
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		doc, err := getHTML(ctx, hc, base+"/jobs/search?"+query("pr", strconv.Itoa(page), "in_iframe", "1", "ss", "1"))
		if err != nil {
			return err
		}
		cards := icimsCardRe.FindAllStringSubmatch(doc, -1)
		if len(cards) == 0 {
			if page == 0 && !strings.Contains(doc, "iCIMS_JobsTable") {
				return errors.New("icims: lista de vagas não encontrada (marcação mudou?)")
			}
			return nil
		}
		added := 0
		for _, cm := range cards {
			card := cm[1]
			l := icimsLinkRe.FindStringSubmatch(card)
			if l == nil || seen[l[2]] {
				continue
			}
			seen[l[2]] = true
			added++
			job := model.Job{
				SourceJobID: l[2], Title: strings.TrimSpace(html.UnescapeString(l[3])),
				URL: strings.Replace(html.UnescapeString(l[1]), "?in_iframe=1", "", 1),
			}
			if m := icimsLocRe.FindStringSubmatch(card); m != nil {
				job.Location = icimsLocation(html.UnescapeString(m[1]))
			}
			if m := icimsDateRe.FindStringSubmatch(card); m != nil {
				job.PostedAt, _ = time.Parse("1/2/2006 3:04 PM", m[1])
			}
			if m := icimsRemoteRe.FindStringSubmatch(card); m != nil {
				job.Remote = icimsRemote(m[1])
			}
			if m := icimsDescRe.FindStringSubmatch(card); m != nil {
				job.Description = textutil.HTMLToText(m[1])
			}
			if err := emit(job); err != nil {
				return err
			}
		}
		if added == 0 {
			return nil // página repetida: fim da paginação
		}
	}
	return truncated("icims", maxPages)
}

// icimsLocation converte "US-AZ-Fort Huachuca" em "Fort Huachuca, AZ, US".
func icimsLocation(s string) string {
	parts := strings.SplitN(strings.TrimSpace(s), "-", 3)
	if len(parts) == 3 && len(parts[0]) == 2 {
		return joinNonEmpty(", ", parts[2], parts[1], parts[0])
	}
	return s
}

func icimsRemote(s string) model.Remote {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "no remote"), strings.Contains(l, "not allowed"):
		return model.RemoteNo
	case strings.Contains(l, "100%"), strings.Contains(l, "fully"):
		return model.RemoteYes
	case strings.Contains(l, "occasional"), strings.Contains(l, "hybrid"), strings.Contains(l, "flexible"):
		return model.RemoteHybrid
	}
	return remoteFromText(s)
}

// ---------------------------------------------------------------- Jobvite

// Jobvite: HTML de https://jobs.jobvite.com/{id}/jobs.
type Jobvite struct{}

func (*Jobvite) Source() string   { return "jobvite" }
func (*Jobvite) Aggregator() bool { return false }

var (
	jobviteRowRe = regexp.MustCompile(`(?s)<td class="jv-job-list-name">\s*<a href="([^"]*/job/([A-Za-z0-9]+))"[^>]*>(.*?)</a>\s*</td>\s*<td class="jv-job-list-location">(.*?)</td>`)
	spacesRe     = regexp.MustCompile(`\s+`)
)

func (j *Jobvite) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	if c.ID == "" || strings.ContainsAny(c.ID, "/?#") {
		return fmt.Errorf("jobvite: id inválido %q", c.ID)
	}
	base := baseOr(c, "https://jobs.jobvite.com")
	doc, err := getHTML(ctx, hc, base+"/"+url.PathEscape(c.ID)+"/jobs")
	if err != nil {
		return err
	}
	if !strings.Contains(doc, "jv-job-list") {
		return errors.New("jobvite: lista de vagas não encontrada (empresa inexistente ou marcação mudou)")
	}
	for _, m := range jobviteRowRe.FindAllStringSubmatch(doc, -1) {
		href := m[1]
		if strings.HasPrefix(href, "/") {
			href = base + href
		}
		job := model.Job{
			SourceJobID: m[2], Title: strings.TrimSpace(html.UnescapeString(m[3])), URL: href,
			Location: strings.TrimSpace(spacesRe.ReplaceAllString(html.UnescapeString(textutil.HTMLToText(m[4])), " ")),
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Oracle Recruiting Cloud

// Oracle Recruiting Cloud (sucessor do Taleo):
// GET https://{host}/hcmRestApi/resources/latest/recruitingCEJobRequisitions?finder=findReqs;siteNumber={site},...
// params: host (ex.: eeho.fa.us2.oraclecloud.com), site (ex.: CX_45001), max_pages.
type Oracle struct{}

func (*Oracle) Source() string   { return "oracle" }
func (*Oracle) Aggregator() bool { return false }

func (o *Oracle) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	host, site := c.Param("host"), c.Param("site")
	if host == "" || site == "" {
		return errors.New("oracle: params host e site obrigatórios")
	}
	base := baseOr(c, "https://"+host)
	const limit = 25
	maxPages := intParam(c, "max_pages", 40)
	for page, offset := 0, 0; page < maxPages; page++ {
		finder := fmt.Sprintf("findReqs;siteNumber=%s,limit=%d,offset=%d,sortBy=POSTING_DATES_DESC", site, limit, offset)
		u := base + "/hcmRestApi/resources/latest/recruitingCEJobRequisitions?onlyData=true&expand=requisitionList.secondaryLocations&finder=" + url.QueryEscape(finder)
		var r struct {
			Items []struct {
				Total int `json:"TotalJobsCount"`
				Reqs  []struct {
					ID        string `json:"Id"`
					Title     string `json:"Title"`
					Posted    string `json:"PostedDate"`
					Location  string `json:"PrimaryLocation"`
					Country   string `json:"PrimaryLocationCountry"`
					Short     string `json:"ShortDescriptionStr"`
					Workplace string `json:"WorkplaceType"`
					WPCode    string `json:"WorkplaceTypeCode"`
				} `json:"requisitionList"`
			} `json:"items"`
		}
		if err := getJSON(ctx, hc, u, &r); err != nil {
			return err
		}
		if len(r.Items) == 0 {
			return nil
		}
		it := r.Items[0]
		for _, q := range it.Reqs {
			job := model.Job{
				SourceJobID: q.ID, Title: strings.TrimSpace(q.Title),
				URL:      base + "/hcmUI/CandidateExperience/en/sites/" + url.PathEscape(site) + "/job/" + url.PathEscape(q.ID),
				Location: q.Location, Country: countryCode(q.Country), PostedAt: parseTime(q.Posted),
				Description: strings.TrimSpace(q.Short), Remote: oracleRemote(q.WPCode, q.Workplace),
			}
			if err := emit(job); err != nil {
				return err
			}
		}
		offset += len(it.Reqs)
		if len(it.Reqs) == 0 || offset >= it.Total {
			return nil
		}
	}
	return truncated("oracle", maxPages)
}

func oracleRemote(code, label string) model.Remote {
	switch code {
	case "ORA_REMOTE":
		return model.RemoteYes
	case "ORA_HYBRID":
		return model.RemoteHybrid
	case "ORA_ON_SITE":
		return model.RemoteNo
	}
	return remoteFromText(label)
}

// ---------------------------------------------------------------- SAP SuccessFactors

// SuccessFactors (Recruiting Marketing): lê o feed de vagas do site de
// carreiras (permitido no robots.txt; a busca costuma ser proibida).
// params: base_url (site, ex.: https://jobs.empresa.com), sitemap (caminho,
// padrão /sitemal.xml). Aceita os dois formatos que o RMK publica:
//   - RSS (/sitemal.xml): título, link, local e descrição;
//   - sitemap <urlset>: só URL e data; o título vem do slug.
type SuccessFactors struct{}

func (*SuccessFactors) Source() string   { return "successfactors" }
func (*SuccessFactors) Aggregator() bool { return false }

var sfIDRe = regexp.MustCompile(`/(\d{6,})(?:/|$)`)

type sfItem struct {
	Title    string `xml:"title"`
	Link     string `xml:"link"`
	GUID     string `xml:"guid"`
	Desc     string `xml:"description"`
	PubDate  string `xml:"pubDate"`
	Location string `xml:"http://base.google.com/ns/1.0 location"`
	ID       string `xml:"http://base.google.com/ns/1.0 id"`
	Loc      string `xml:"loc"`
	LastMod  string `xml:"lastmod"`
}

func (s *SuccessFactors) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	base := strings.TrimRight(c.Param("base_url"), "/")
	if base == "" {
		return errors.New("successfactors: params.base_url obrigatório (ex.: https://jobs.empresa.com)")
	}
	path := c.Param("sitemap")
	if path == "" {
		path = "/sitemal.xml"
	}
	resp, err := hc.Do(ctx, httpx.Request{URL: base + path, Header: http.Header{"Accept": {"application/rss+xml, application/xml, text/xml"}}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := xml.NewDecoder(resp.Body)
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	found := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("successfactors: xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || (se.Name.Local != "item" && se.Name.Local != "url") {
			continue
		}
		found = true
		var it sfItem
		if err := dec.DecodeElement(&it, &se); err != nil {
			return fmt.Errorf("successfactors: %w", err)
		}
		job, ok := it.job()
		if !ok {
			continue
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	if !found {
		return errors.New("successfactors: feed sem vagas (formato inesperado)")
	}
	return nil
}

func (it sfItem) job() (model.Job, bool) {
	if it.Loc != "" { // sitemap
		loc := strings.TrimSpace(it.Loc)
		m := sfIDRe.FindStringSubmatch(loc)
		if m == nil {
			return model.Job{}, false
		}
		title := sfTitle(loc, m[1])
		return model.Job{SourceJobID: m[1], Title: title, URL: loc, PostedAt: parseTime(it.LastMod)}, title != ""
	}
	link := strings.TrimSpace(it.Link)
	id := strings.TrimSpace(it.ID)
	if id == "" {
		id = strings.TrimSpace(it.GUID)
	}
	if id == "" {
		if m := sfIDRe.FindStringSubmatch(link); m != nil {
			id = m[1]
		}
	}
	title := strings.TrimSpace(it.Title)
	loc := strings.TrimSpace(it.Location)
	// O RMK anexa o local ao título: "Analista (São Paulo, SP, BR, 01000)".
	if loc != "" {
		title = strings.TrimSpace(strings.TrimSuffix(title, "("+loc+")"))
	}
	if id == "" || title == "" || link == "" {
		return model.Job{}, false
	}
	return model.Job{
		SourceJobID: id, Title: title, URL: link, Location: sfLocation(loc),
		Description: textutil.HTMLToText(it.Desc), PostedAt: parseTime(it.PubDate),
	}, true
}

var sfZipRe = regexp.MustCompile(`,\s*[A-Z0-9-]*\d[A-Z0-9 -]*$`)

// sfLocation remove o CEP final ("Horst, Limburg, NL, 5961 PT" → "Horst, Limburg, NL").
func sfLocation(s string) string {
	return strings.TrimSpace(sfZipRe.ReplaceAllString(s, ""))
}

// sfTitle reconstrói o título a partir do slug vizinho ao id na URL.
func sfTitle(loc, id string) string {
	parts := strings.Split(strings.Trim(loc, "/"), "/")
	for i, p := range parts {
		if p != id {
			continue
		}
		slug := ""
		switch {
		case i+1 < len(parts):
			slug = parts[i+1] // /jobs/{id}/{slug}/
		case i > 0:
			slug = parts[i-1] // /job/{slug}/{id}/
		}
		slug, _ = url.PathUnescape(slug)
		words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(slug))
		for k, w := range words {
			if len(w) <= 3 && strings.ToUpper(w) == strings.ToUpper(w) && k > 0 && isAcronym(w) {
				words[k] = strings.ToUpper(w)
				continue
			}
			words[k] = strings.ToUpper(w[:1]) + w[1:]
		}
		return strings.Join(words, " ")
	}
	return ""
}

var acronyms = map[string]bool{"ai": true, "it": true, "sap": true, "ux": true, "ui": true, "qa": true, "sre": true, "hr": true, "bi": true, "aws": true, "ml": true}

func isAcronym(w string) bool { return acronyms[strings.ToLower(w)] }
