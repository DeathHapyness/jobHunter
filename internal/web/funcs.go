package web

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"vagas/internal/model"
)

// jobData é o contexto do template de uma vaga.
type jobData struct {
	model.JobView
	LastVisit time.Time
}

// newWindow é a idade máxima para uma vaga ser considerada "nova".
const newWindow = 7 * 24 * time.Hour

// IsNew indica vaga publicada há no máximo newWindow (ou, sem data de
// publicação, vista pela primeira vez nesse período) e que surgiu depois da
// visita anterior.
func (j jobData) IsNew() bool {
	age := j.PostedAt
	if age.IsZero() {
		age = j.FirstSeen
	}
	if time.Since(age) > newWindow {
		return false
	}
	return j.LastVisit.IsZero() || j.FirstSeen.After(j.LastVisit)
}

var labels = map[string]string{}

func init() {
	for _, group := range [][]option{levelOpts, areaOpts, remoteOpts, statusOpts} {
		for _, o := range group {
			labels[o.Value] = o.Label
		}
	}
}

var funcs = template.FuncMap{
	"label": func(v any) string {
		s := fmt.Sprint(v)
		if l, ok := labels[s]; ok {
			return l
		}
		return s
	},
	"ago":   ago,
	"lower": strings.ToLower,
	// opts transforma valores de faceta em opções (valor = rótulo).
	"opts": func(vals []string) []option {
		out := make([]option, len(vals))
		for i, v := range vals {
			out[i] = option{v, v}
		}
		return out
	},
	"short": func(t time.Time) string {
		if t.IsZero() {
			return "--/--"
		}
		return t.Local().Format("02/01")
	},
	"dict": func(kv ...any) (map[string]any, error) {
		if len(kv)%2 != 0 {
			return nil, fmt.Errorf("dict: número ímpar de argumentos")
		}
		m := make(map[string]any, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			k, ok := kv[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict: chave não é string")
			}
			m[k] = kv[i+1]
		}
		return m, nil
	},
	"date": func(t time.Time) string { return t.Local().Format("02/01/2006 15:04") },
	"job": func(v model.JobView, lastVisit time.Time) jobData {
		return jobData{JobView: v, LastVisit: lastVisit}
	},
	"excerpt": func(s string, n int) string {
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		return string(r[:n]) + "…"
	},
}

// ago formata uma data de forma relativa e curta.
func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "agora"
	case d < time.Hour:
		return fmt.Sprintf("há %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("há %d h", int(d.Hours()))
	case d < 60*24*time.Hour:
		n := int(d.Hours() / 24)
		if n == 1 {
			return "ontem"
		}
		return fmt.Sprintf("há %d dias", n)
	}
	return t.Local().Format("02/01/2006")
}
