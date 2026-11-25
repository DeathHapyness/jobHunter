package adapters

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/textutil"
)

// Adaptadores de ATS com endpoints públicos e documentados.
// Todos aceitam params.base_url para domínio próprio ou testes.

func init() {
	Register(&Lever{})
	Register(&Ashby{})
	Register(&Workable{})
	Register(&SmartRecruiters{})
	Register(&Recruitee{})
	Register(&Personio{})
	Register(&Teamtailor{})
	Register(&BambooHR{})
	Register(&Breezy{})
	Register(&Pinpoint{})
}

// ---------------------------------------------------------------- Lever

// Lever: GET https://api.lever.co/v0/postings/{id}?mode=json
// params.region = "eu" usa api.eu.lever.co.
type Lever struct{}

func (*Lever) Source() string   { return "lever" }
func (*Lever) Aggregator() bool { return false }

func (l *Lever) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	host := "https://api.lever.co"
	if c.Param("region") == "eu" {
		host = "https://api.eu.lever.co"
	}
	u := baseOr(c, host) + "/v0/postings/" + url.PathEscape(c.ID) + "?mode=json"
	resp, err := hc.Get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "", func(dec *json.Decoder) error {
		var j struct {
			ID         string      `json:"id"`
			Text       string      `json:"text"`
			HostedURL  string      `json:"hostedUrl"`
			CreatedAt  json.Number `json:"createdAt"`
			Country    string      `json:"country"`
			Workplace  string      `json:"workplaceType"`
			Desc       string      `json:"descriptionPlain"`
			Additional string      `json:"additionalPlain"`
			Categories struct {
				Location     string   `json:"location"`
				AllLocations []string `json:"allLocations"`
				Commitment   string   `json:"commitment"`
			} `json:"categories"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("lever: %w", err)
		}
		loc := j.Categories.Location
		if len(j.Categories.AllLocations) > 1 {
			loc = strings.Join(j.Categories.AllLocations, " | ")
		}
		job := model.Job{
			SourceJobID: j.ID, Title: strings.TrimSpace(j.Text), URL: j.HostedURL,
			Location: loc, Country: countryCode(j.Country), PostedAt: unixMillis(j.CreatedAt),
			Description: strings.TrimSpace(j.Desc + "\n" + j.Additional),
			Remote:      remoteFromText(j.Workplace),
		}
		if strings.Contains(strings.ToLower(j.Categories.Commitment), "intern") {
			job.Level = model.LevelIntern
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- Ashby

// Ashby: GET https://api.ashbyhq.com/posting-api/job-board/{id}
type Ashby struct{}

func (*Ashby) Source() string   { return "ashby" }
func (*Ashby) Aggregator() bool { return false }

func (a *Ashby) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	u := baseOr(c, "https://api.ashbyhq.com") + "/posting-api/job-board/" + url.PathEscape(c.ID)
	resp, err := hc.Get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "jobs", func(dec *json.Decoder) error {
		var j struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Location  string `json:"location"`
			Published string `json:"publishedAt"`
			IsListed  *bool  `json:"isListed"`
			IsRemote  bool   `json:"isRemote"`
			Workplace string `json:"workplaceType"`
			JobURL    string `json:"jobUrl"`
			Desc      string `json:"descriptionPlain"`
			EmpType   string `json:"employmentType"`
			Address   struct {
				Postal struct {
					Country string `json:"addressCountry"`
				} `json:"postalAddress"`
			} `json:"address"`
			Secondary []struct {
				Location string `json:"location"`
			} `json:"secondaryLocations"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("ashby: %w", err)
		}
		if j.IsListed != nil && !*j.IsListed {
			return nil
		}
		locs := []string{j.Location}
		for _, s := range j.Secondary {
			locs = append(locs, s.Location)
		}
		job := model.Job{
			SourceJobID: j.ID, Title: strings.TrimSpace(j.Title), URL: j.JobURL,
			Location: joinNonEmpty(" | ", locs...), Country: j.Address.Postal.Country,
			PostedAt: parseTime(j.Published), Description: strings.TrimSpace(j.Desc),
			Remote: remoteFromText(j.Workplace),
		}
		if job.Remote == model.RemoteUnknown && j.IsRemote {
			job.Remote = model.RemoteYes
		}
		if j.EmpType == "Intern" {
			job.Level = model.LevelIntern
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- Workable

// Workable: GET https://apply.workable.com/api/v1/widget/accounts/{id}?details=true
type Workable struct{}

func (*Workable) Source() string   { return "workable" }
func (*Workable) Aggregator() bool { return false }

func (w *Workable) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	u := baseOr(c, "https://apply.workable.com") + "/api/v1/widget/accounts/" + url.PathEscape(c.ID) + "?details=true"
	resp, err := hc.Get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "jobs", func(dec *json.Decoder) error {
		var j struct {
			Title     string `json:"title"`
			Shortcode string `json:"shortcode"`
			URL       string `json:"url"`
			Remote    bool   `json:"telecommuting"`
			Published string `json:"published_on"`
			Created   string `json:"created_at"`
			Country   string `json:"country"`
			City      string `json:"city"`
			State     string `json:"state"`
			Desc      string `json:"description"`
			Exp       string `json:"experience"`
			Locations []struct {
				Country string `json:"country"`
				City    string `json:"city"`
			} `json:"locations"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("workable: %w", err)
		}
		loc := joinNonEmpty(", ", j.City, j.State, j.Country)
		if len(j.Locations) > 1 {
			var ls []string
			for _, l := range j.Locations {
				ls = append(ls, joinNonEmpty(", ", l.City, l.Country))
			}
			loc = joinNonEmpty(" | ", ls...)
		}
		posted := parseTime(j.Published)
		if posted.IsZero() {
			posted = parseTime(j.Created)
		}
		job := model.Job{
			SourceJobID: j.Shortcode, Title: strings.TrimSpace(j.Title), URL: j.URL,
			Location: loc, Country: j.Country, PostedAt: posted,
			Description: textutil.HTMLToText(j.Desc), Level: levelFromCode(j.Exp),
		}
		if j.Remote {
			job.Remote = model.RemoteYes
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- SmartRecruiters

// SmartRecruiters: GET https://api.smartrecruiters.com/v1/companies/{id}/postings?limit=100&offset=N
// params.max_pages limita a paginação (padrão 100 páginas = 10 mil vagas).
type SmartRecruiters struct{}

func (*SmartRecruiters) Source() string   { return "smartrecruiters" }
func (*SmartRecruiters) Aggregator() bool { return false }

func (s *SmartRecruiters) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	const limit = 100
	maxPages := intParam(c, "max_pages", 100)
	base := baseOr(c, "https://api.smartrecruiters.com") + "/v1/companies/" + url.PathEscape(c.ID) + "/postings"
	for page, offset := 0, 0; page < maxPages; page++ {
		var r struct {
			Total   int `json:"totalFound"`
			Content []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Released string `json:"releasedDate"`
				Company  struct {
					Identifier string `json:"identifier"`
				} `json:"company"`
				Location struct {
					City    string `json:"city"`
					Region  string `json:"region"`
					Country string `json:"country"`
					Remote  bool   `json:"remote"`
					Hybrid  bool   `json:"hybrid"`
				} `json:"location"`
				Experience struct {
					ID string `json:"id"`
				} `json:"experienceLevel"`
			} `json:"content"`
		}
		if err := getJSON(ctx, hc, base+"?"+query("limit", fmt.Sprint(limit), "offset", fmt.Sprint(offset)), &r); err != nil {
			return err
		}
		for _, j := range r.Content {
			ident := j.Company.Identifier
			if ident == "" {
				ident = c.ID
			}
			job := model.Job{
				SourceJobID: j.ID, Title: strings.TrimSpace(j.Name),
				URL:      "https://jobs.smartrecruiters.com/" + url.PathEscape(ident) + "/" + url.PathEscape(j.ID),
				Location: joinNonEmpty(", ", j.Location.City, j.Location.Region, strings.ToUpper(j.Location.Country)),
				Country:  countryCode(j.Location.Country), PostedAt: parseTime(j.Released),
				Remote: remoteFlags(j.Location.Remote, j.Location.Hybrid), Level: levelFromCode(j.Experience.ID),
			}
			if err := emit(job); err != nil {
				return err
			}
		}
		offset += len(r.Content)
		if len(r.Content) == 0 || offset >= r.Total {
			return nil
		}
	}
	return fmt.Errorf("smartrecruiters: limite de %d páginas atingido", maxPages)
}

// ---------------------------------------------------------------- Recruitee

// Recruitee: GET https://{id}.recruitee.com/api/offers/
type Recruitee struct{}

func (*Recruitee) Source() string   { return "recruitee" }
func (*Recruitee) Aggregator() bool { return false }

func (r *Recruitee) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "recruitee.com", "")
	if err != nil {
		return err
	}
	resp, err := hc.Get(ctx, baseOr(c, def)+"/api/offers/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "offers", func(dec *json.Decoder) error {
		var j struct {
			ID        json.Number `json:"id"`
			Title     string      `json:"title"`
			URL       string      `json:"careers_url"`
			Location  string      `json:"location"`
			Country   string      `json:"country"`
			Remote    bool        `json:"remote"`
			Hybrid    bool        `json:"hybrid"`
			OnSite    bool        `json:"on_site"`
			Published string      `json:"published_at"`
			Created   string      `json:"created_at"`
			Desc      string      `json:"description"`
			Req       string      `json:"requirements"`
			Exp       string      `json:"experience_code"`
			Status    string      `json:"status"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("recruitee: %w", err)
		}
		if j.Status != "" && j.Status != "published" {
			return nil
		}
		posted := parseTime(j.Published)
		if posted.IsZero() {
			posted = parseTime(j.Created)
		}
		job := model.Job{
			SourceJobID: j.ID.String(), Title: strings.TrimSpace(j.Title), URL: j.URL,
			Location: j.Location, Country: j.Country, PostedAt: posted,
			Description: textutil.HTMLToText(j.Desc + "\n" + j.Req),
			Remote:      remoteFlags(j.Remote, j.Hybrid), Level: levelFromCode(j.Exp),
		}
		if job.Remote == model.RemoteUnknown && j.OnSite {
			job.Remote = model.RemoteNo
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- Personio

// Personio: feed XML GET https://{id}.jobs.personio.de/xml?language=en
// params.domain = "jobs.personio.com" para contas .com; params.language.
type Personio struct{}

func (*Personio) Source() string   { return "personio" }
func (*Personio) Aggregator() bool { return false }

func (p *Personio) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	domain := c.Param("domain")
	if domain == "" {
		domain = "jobs.personio.de"
	}
	def, err := subdomainURL(c, domain, "")
	if err != nil {
		return err
	}
	base := baseOr(c, def)
	lang := c.Param("language")
	if lang == "" {
		lang = "en"
	}
	resp, err := hc.Do(ctx, httpx.Request{URL: base + "/xml?" + query("language", lang), Header: map[string][]string{"Accept": {"application/xml, text/xml"}}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamXML(resp.Body, "position", func(dec *xml.Decoder, start xml.StartElement) error {
		var j struct {
			ID         string   `xml:"id"`
			Name       string   `xml:"name"`
			Office     string   `xml:"office"`
			Additional []string `xml:"additionalOffices>office"`
			Seniority  string   `xml:"seniority"`
			Created    string   `xml:"createdAt"`
			Descs      []struct {
				Name  string `xml:"name"`
				Value string `xml:"value"`
			} `xml:"jobDescriptions>jobDescription"`
		}
		if err := dec.DecodeElement(&j, &start); err != nil {
			return fmt.Errorf("personio: %w", err)
		}
		var desc strings.Builder
		for _, d := range j.Descs {
			desc.WriteString(d.Name + "\n" + d.Value + "\n")
		}
		job := model.Job{
			SourceJobID: strings.TrimSpace(j.ID), Title: strings.TrimSpace(j.Name),
			URL:      base + "/job/" + url.PathEscape(strings.TrimSpace(j.ID)),
			Location: joinNonEmpty(" | ", append([]string{j.Office}, j.Additional...)...),
			PostedAt: parseTime(j.Created), Description: textutil.HTMLToText(desc.String()),
			Level: levelFromCode(j.Seniority),
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- Teamtailor

// Teamtailor: feed RSS público da página de carreiras
// GET https://{id}.teamtailor.com/jobs.rss (params.base_url para domínio próprio).
type Teamtailor struct{}

func (*Teamtailor) Source() string   { return "teamtailor" }
func (*Teamtailor) Aggregator() bool { return false }

func (t *Teamtailor) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "teamtailor.com", "")
	if err != nil {
		return err
	}
	resp, err := hc.Do(ctx, httpx.Request{URL: baseOr(c, def) + "/jobs.rss", Header: map[string][]string{"Accept": {"application/rss+xml, application/xml"}}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamXML(resp.Body, "item", func(dec *xml.Decoder, start xml.StartElement) error {
		var j struct {
			Title     string `xml:"title"`
			Desc      string `xml:"description"`
			PubDate   string `xml:"pubDate"`
			Link      string `xml:"link"`
			Remote    string `xml:"remoteStatus"`
			GUID      string `xml:"guid"`
			Locations []struct {
				City    string `xml:"city"`
				Country string `xml:"country"`
			} `xml:"locations>location"`
		}
		if err := dec.DecodeElement(&j, &start); err != nil {
			return fmt.Errorf("teamtailor: %w", err)
		}
		var locs []string
		country := ""
		for _, l := range j.Locations {
			locs = append(locs, joinNonEmpty(", ", l.City, l.Country))
			country = l.Country
		}
		posted, _ := time.Parse(time.RFC1123Z, strings.TrimSpace(j.PubDate))
		job := model.Job{
			SourceJobID: strings.TrimSpace(j.GUID), Title: strings.TrimSpace(j.Title), URL: strings.TrimSpace(j.Link),
			Location: joinNonEmpty(" | ", locs...), PostedAt: posted,
			Description: textutil.HTMLToText(html.UnescapeString(j.Desc)),
		}
		if len(j.Locations) == 1 {
			job.Country = country
		}
		switch strings.ToLower(strings.TrimSpace(j.Remote)) {
		case "fully", "temporary":
			job.Remote = model.RemoteYes
		case "hybrid":
			job.Remote = model.RemoteHybrid
		case "none":
			job.Remote = model.RemoteNo
		}
		if job.SourceJobID == "" {
			job.SourceJobID = job.URL
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- BambooHR

// BambooHR: GET https://{id}.bamboohr.com/careers/list (Accept: application/json)
type BambooHR struct{}

func (*BambooHR) Source() string   { return "bamboohr" }
func (*BambooHR) Aggregator() bool { return false }

func (b *BambooHR) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "bamboohr.com", "")
	if err != nil {
		return err
	}
	base := baseOr(c, def)
	var r struct {
		Result []struct {
			ID       any    `json:"id"`
			Name     string `json:"jobOpeningName"`
			Status   string `json:"employmentStatusLabel"`
			IsRemote any    `json:"isRemote"`
			LocType  any    `json:"locationType"`
			Location struct {
				City  string `json:"city"`
				State string `json:"state"`
			} `json:"location"`
			ATS struct {
				City    string `json:"city"`
				State   string `json:"state"`
				Country string `json:"country"`
			} `json:"atsLocation"`
		} `json:"result"`
	}
	if err := getJSON(ctx, hc, base+"/careers/list", &r); err != nil {
		return err
	}
	for _, j := range r.Result {
		id := numStr(j.ID)
		city, state := j.Location.City, j.Location.State
		if city == "" {
			city, state = j.ATS.City, j.ATS.State
		}
		job := model.Job{
			SourceJobID: id, Title: strings.TrimSpace(j.Name), URL: base + "/careers/" + url.PathEscape(id),
			Location: joinNonEmpty(", ", city, state, j.ATS.Country), Country: j.ATS.Country,
		}
		switch numStr(j.LocType) {
		case "1":
			job.Remote = model.RemoteYes
		case "2":
			job.Remote = model.RemoteHybrid
		case "0":
			job.Remote = model.RemoteNo
		}
		if job.Remote == "" && (j.IsRemote == true || numStr(j.IsRemote) == "1") {
			job.Remote = model.RemoteYes
		}
		if strings.Contains(strings.ToLower(j.Status), "intern") {
			job.Level = model.LevelIntern
		}
		if err := emit(job); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Breezy HR

// Breezy HR: GET https://{id}.breezy.hr/json
type Breezy struct{}

func (*Breezy) Source() string   { return "breezy" }
func (*Breezy) Aggregator() bool { return false }

func (b *Breezy) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "breezy.hr", "")
	if err != nil {
		return err
	}
	resp, err := hc.Get(ctx, baseOr(c, def)+"/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "", func(dec *json.Decoder) error {
		var j struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			URL       string `json:"url"`
			Published string `json:"published_date"`
			Type      struct {
				ID string `json:"id"`
			} `json:"type"`
			Location struct {
				Name     string `json:"name"`
				IsRemote bool   `json:"is_remote"`
				Country  struct {
					Name string `json:"name"`
				} `json:"country"`
			} `json:"location"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("breezy: %w", err)
		}
		job := model.Job{
			SourceJobID: j.ID, Title: strings.TrimSpace(j.Name), URL: j.URL,
			Location: joinNonEmpty(", ", j.Location.Name, j.Location.Country.Name),
			Country:  j.Location.Country.Name, PostedAt: parseTime(j.Published),
		}
		if j.Location.IsRemote {
			job.Remote = model.RemoteYes
		}
		if j.Type.ID == "internship" {
			job.Level = model.LevelIntern
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- Pinpoint

// Pinpoint: GET https://{id}.pinpointhq.com/postings.json
type Pinpoint struct{}

func (*Pinpoint) Source() string   { return "pinpoint" }
func (*Pinpoint) Aggregator() bool { return false }

func (p *Pinpoint) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	def, err := subdomainURL(c, "pinpointhq.com", "")
	if err != nil {
		return err
	}
	resp, err := hc.Get(ctx, baseOr(c, def)+"/postings.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return streamArray(resp.Body, "data", func(dec *json.Decoder) error {
		var j struct {
			ID        any    `json:"id"`
			Title     string `json:"title"`
			URL       string `json:"url"`
			Workplace string `json:"workplace_type"`
			EmpType   string `json:"employment_type"`
			Desc      string `json:"description"`
			Resp      string `json:"key_responsibilities"`
			Skills    string `json:"skills_knowledge_expertise"`
			Location  struct {
				City     string `json:"city"`
				Name     string `json:"name"`
				Province string `json:"province"`
			} `json:"location"`
		}
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("pinpoint: %w", err)
		}
		job := model.Job{
			SourceJobID: numStr(j.ID), Title: strings.TrimSpace(j.Title), URL: j.URL,
			Location:    joinNonEmpty(", ", j.Location.Name, j.Location.City, j.Location.Province),
			Description: textutil.HTMLToText(j.Desc + "\n" + j.Resp + "\n" + j.Skills),
			Remote:      remoteFromText(j.Workplace),
		}
		if j.Workplace == "remote" {
			job.Remote = model.RemoteYes
		}
		if strings.Contains(j.EmpType, "intern") {
			job.Level = model.LevelIntern
		}
		return emit(job)
	})
}

// ---------------------------------------------------------------- comuns

func intParam(c config.Company, key string, def int) int {
	var n int
	if _, err := fmt.Sscan(c.Param(key), &n); err == nil && n > 0 {
		return n
	}
	return def
}

// countryCode converte código ISO-3166 alfa-2 no nome usado pela classificação.
func countryCode(code string) string {
	if name, ok := isoCountries[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return name
	}
	return code
}

var isoCountries = map[string]string{
	"BR": "Brasil", "US": "Estados Unidos", "GB": "Reino Unido", "UK": "Reino Unido", "CA": "Canadá",
	"PT": "Portugal", "DE": "Alemanha", "IE": "Irlanda", "NL": "Holanda", "ES": "Espanha", "FR": "França",
	"MX": "México", "AR": "Argentina", "CL": "Chile", "CO": "Colômbia", "PE": "Peru", "UY": "Uruguai",
	"IN": "Índia", "JP": "Japão", "SG": "Singapura", "AU": "Austrália", "CH": "Suíça", "IT": "Itália",
	"PL": "Polônia", "SE": "Suécia", "DK": "Dinamarca", "NO": "Noruega", "FI": "Finlândia", "BE": "Bélgica",
	"AT": "Áustria", "CZ": "Tchéquia", "RO": "Romênia", "IL": "Israel", "AE": "Emirados Árabes", "TR": "Turquia",
	"CN": "China", "KR": "Coreia do Sul", "HK": "Hong Kong", "TW": "Taiwan", "PH": "Filipinas", "ZA": "África do Sul",
	"NZ": "Nova Zelândia", "HU": "Hungria", "GR": "Grécia", "LU": "Luxemburgo", "EE": "Estônia", "UA": "Ucrânia",
}
