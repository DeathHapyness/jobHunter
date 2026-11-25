// Package web serve a interface (uma tela, HTMX) e o healthcheck.
package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vagas/internal/model"
	"vagas/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	keyVisitCurrent  = "visit_current"
	keyVisitPrevious = "visit_previous"
	// visitGap é o intervalo mínimo entre acessos para contar uma nova visita.
	visitGap = 30 * time.Minute
	pageSize = 50
)

// Collector é a parte do coletor que a interface usa.
type Collector interface {
	Running() bool
	Trigger() bool
}

// Server é o handler HTTP da aplicação.
type Server struct {
	store     *store.Store
	collector Collector
	log       *slog.Logger
	tpl       *template.Template
	mux       *http.ServeMux
}

// New cria o servidor.
func New(s *store.Store, c Collector, log *slog.Logger) (*Server, error) {
	versions, err := assetVersions()
	if err != nil {
		return nil, err
	}
	fm := template.FuncMap{"asset": func(name string) string { return "/static/" + name + "?v=" + versions[name] }}
	tpl, err := template.New("").Funcs(funcs).Funcs(fm).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	srv := &Server{store: s, collector: c, log: log, tpl: tpl, mux: http.NewServeMux()}
	static, _ := fs.Sub(staticFS, "static")
	srv.mux.Handle("GET /static/", cacheStatic(http.StripPrefix("/static/", http.FileServerFS(static))))
	srv.mux.HandleFunc("GET /{$}", srv.index)
	srv.mux.HandleFunc("POST /jobs/{id}/status", srv.setStatus)
	srv.mux.HandleFunc("POST /jobs/{id}/notes", srv.setNotes)
	srv.mux.HandleFunc("POST /collect", srv.collect)
	srv.mux.HandleFunc("GET /healthz", srv.healthz)
	return srv, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:")
	s.mux.ServeHTTP(w, r)
}

type pageData struct {
	Filter      formFilter
	Jobs        []model.JobView
	Total       int
	Page        int
	Pages       int
	PrevURL     string
	NextURL     string
	Facets      store.Facets
	Statuses    []store.SourceStatus
	LastVisit   time.Time
	Collecting  bool
	Levels      []option
	Areas       []option
	Remotes     []option
	StatusOpts  []option
	PostedOpts  []option
	SortOpts    []option
	QueryString string
}

type option struct{ Value, Label string }

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	htmx := r.Header.Get("HX-Request") == "true"

	lastVisit, err := s.lastVisit(ctx, !htmx)
	if err != nil {
		s.fail(w, err)
		return
	}
	ff := parseFilter(r.URL.Query())
	f := ff.toStore(time.Now(), lastVisit)
	jobs, total, err := s.store.ListJobs(ctx, f)
	if err != nil {
		s.fail(w, err)
		return
	}
	data := pageData{
		Filter: ff, Jobs: jobs, Total: total, Page: ff.Page,
		Pages:      (total + pageSize - 1) / pageSize,
		LastVisit:  lastVisit,
		Collecting: s.collector != nil && s.collector.Running(),
		Levels:     levelOpts, Areas: areaOpts, Remotes: remoteOpts,
		StatusOpts: statusOpts, PostedOpts: postedOpts, SortOpts: sortOpts,
		QueryString: ff.encode(0),
	}
	if ff.Page > 1 {
		data.PrevURL = "/?" + ff.encode(ff.Page-1)
	}
	if ff.Page < data.Pages {
		data.NextURL = "/?" + ff.encode(ff.Page+1)
	}
	if data.Statuses, err = s.store.SourceStatuses(ctx); err != nil {
		s.fail(w, err)
		return
	}
	tpl := "page"
	if htmx && r.Header.Get("HX-Target") == "results" {
		tpl = "results"
	} else if data.Facets, err = s.store.Facets(ctx); err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, tpl, data)
}

// lastVisit devolve o início da visita anterior. Em carregamentos completos
// da página (não HTMX) registra a visita atual.
func (s *Server) lastVisit(ctx context.Context, record bool) (time.Time, error) {
	cur, err := s.store.GetAppState(ctx, keyVisitCurrent)
	if err != nil {
		return time.Time{}, err
	}
	prev, err := s.store.GetAppState(ctx, keyVisitPrevious)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	if record {
		curT := parseUnix(cur)
		if curT.IsZero() || now.Sub(curT) > visitGap {
			if !curT.IsZero() {
				prev = cur
				if err := s.store.SetAppState(ctx, keyVisitPrevious, prev); err != nil {
					return time.Time{}, err
				}
			}
		}
		if err := s.store.SetAppState(ctx, keyVisitCurrent, strconv.FormatInt(now.Unix(), 10)); err != nil {
			return time.Time{}, err
		}
	}
	return parseUnix(prev), nil
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.jobID(w, r)
	if !ok {
		return
	}
	status := model.Status(r.FormValue("status"))
	if !model.ValidStatus(status) {
		http.Error(w, "status inválido", http.StatusBadRequest)
		return
	}
	if err := s.store.SetStatus(r.Context(), id, status); err != nil {
		s.storeErr(w, err)
		return
	}
	s.renderJob(w, r, id)
}

func (s *Server) setNotes(w http.ResponseWriter, r *http.Request) {
	id, ok := s.jobID(w, r)
	if !ok {
		return
	}
	notes := strings.TrimSpace(r.FormValue("notes"))
	if len(notes) > 5000 {
		http.Error(w, "anotação muito longa", http.StatusBadRequest)
		return
	}
	if err := s.store.SetNotes(r.Context(), id, notes); err != nil {
		s.storeErr(w, err)
		return
	}
	s.renderJob(w, r, id)
}

func (s *Server) renderJob(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	lastVisit, _ := s.lastVisit(r.Context(), false)
	s.render(w, "job", jobData{JobView: v, LastVisit: lastVisit})
}

func (s *Server) collect(w http.ResponseWriter, r *http.Request) {
	if s.collector == nil {
		http.Error(w, "coletor indisponível", http.StatusServiceUnavailable)
		return
	}
	msg := "Coleta iniciada. Recarregue em alguns minutos."
	if !s.collector.Trigger() {
		msg = "Já existe uma coleta em andamento."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	template.HTMLEscape(w, []byte(msg))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

func (s *Server) jobID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template", "nome", name, "erro", err)
	}
}

func (s *Server) storeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "vaga não encontrada", http.StatusNotFound)
		return
	}
	s.fail(w, err)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("erro interno", "erro", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		h.ServeHTTP(w, r)
	})
}

// assetVersions calcula um hash curto de cada arquivo estático, usado na URL
// para que o navegador nunca use CSS/JS antigo do cache.
func assetVersions() (map[string]string, error) {
	out := map[string]string{}
	err := fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := staticFS.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[strings.TrimPrefix(path, "static/")] = hex.EncodeToString(sum[:4])
		return nil
	})
	return out, err
}

func parseUnix(s string) time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
