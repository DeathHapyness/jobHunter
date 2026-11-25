package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/textutil"
)

// Greenhouse usa a Job Board API pública:
// GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true
// companies.yaml: id = token do board (ex.: "anthropic").
type Greenhouse struct {
	BaseURL string
}

func init() { Register(&Greenhouse{BaseURL: "https://boards-api.greenhouse.io"}) }

func (*Greenhouse) Source() string   { return "greenhouse" }
func (*Greenhouse) Aggregator() bool { return false }

type ghJob struct {
	ID             json.Number `json:"id"`
	Title          string      `json:"title"`
	AbsoluteURL    string      `json:"absolute_url"`
	UpdatedAt      string      `json:"updated_at"`
	FirstPublished string      `json:"first_published"`
	Content        string      `json:"content"`
	Location       struct {
		Name string `json:"name"`
	} `json:"location"`
	Metadata []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"metadata"`
	Offices []struct {
		Location string `json:"location"`
	} `json:"offices"`
}

func (g *Greenhouse) Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error {
	if c.ID == "" {
		return fmt.Errorf("greenhouse: id (token do board) obrigatório")
	}
	u := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", g.BaseURL, url.PathEscape(c.ID))
	resp, err := hc.Get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return streamArray(resp.Body, "jobs", func(dec *json.Decoder) error {
		var j ghJob
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("greenhouse: %w", err)
		}
		posted := parseTime(j.FirstPublished)
		if posted.IsZero() {
			posted = parseTime(j.UpdatedAt)
		}
		job := model.Job{
			Source:      g.Source(),
			SourceJobID: j.ID.String(),
			Company:     c.Name,
			Title:       strings.TrimSpace(j.Title),
			Description: textutil.HTMLToText(j.Content),
			Location:    strings.TrimSpace(j.Location.Name),
			URL:         j.AbsoluteURL,
			PostedAt:    posted,
		}
		for _, m := range j.Metadata {
			if strings.Contains(strings.ToLower(m.Name), "location type") || strings.Contains(strings.ToLower(m.Name), "workplace") {
				if s, ok := m.Value.(string); ok {
					job.Remote = remoteFromText(s)
				}
			}
		}
		if len(j.Offices) > 0 && job.Country == "" {
			job.Country = lastSegment(j.Offices[0].Location)
		}
		return emit(job)
	})
}

// parseTime aceita os formatos de data mais comuns nas APIs de ATS.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// remoteFromText interpreta rótulos de modalidade ("Remote", "Hybrid", "On-site"...).
func remoteFromText(s string) model.Remote {
	s = model.Normalize(s)
	switch {
	case s == "":
		return model.RemoteUnknown
	case strings.Contains(s, "hybrid"), strings.Contains(s, "hibrid"):
		return model.RemoteHybrid
	case strings.Contains(s, "remote"), strings.Contains(s, "remoto"), strings.Contains(s, "home office"):
		return model.RemoteYes
	case strings.Contains(s, "on site"), strings.Contains(s, "onsite"), strings.Contains(s, "presencial"), strings.Contains(s, "in office"):
		return model.RemoteNo
	}
	return model.RemoteUnknown
}

func lastSegment(loc string) string {
	parts := strings.Split(loc, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}
