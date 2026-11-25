// Package textutil contém utilidades de limpeza de texto.
package textutil

import (
	"html"
	"strings"
)

// blockTags viram quebra de linha ao converter HTML em texto.
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "ul": true, "ol": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"tr": true, "section": true, "article": true, "header": true, "footer": true,
}

// HTMLToText converte HTML (inclusive HTML com entidades escapadas, como o
// Greenhouse devolve) em texto limpo, preservando parágrafos.
func HTMLToText(s string) string {
	if s == "" {
		return ""
	}
	// Alguns ATS devolvem o HTML escapado ("&lt;p&gt;"): desescapa uma vez
	// antes de remover as tags.
	if strings.Contains(s, "&lt;") {
		s = html.UnescapeString(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c != '<' {
			b.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		tag := s[i+1 : i+end]
		name := strings.ToLower(strings.TrimLeft(tag, "/"))
		if j := strings.IndexAny(name, " \t\n/"); j >= 0 {
			name = name[:j]
		}
		if name == "script" || name == "style" {
			closeTag := "</" + name
			if k := strings.Index(strings.ToLower(s[i+end:]), closeTag); k >= 0 {
				i = i + end + k
				if e := strings.IndexByte(s[i:], '>'); e >= 0 {
					i += e + 1
				} else {
					i = len(s)
				}
				continue
			}
		}
		if blockTags[name] {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
		i += end + 1
	}
	return collapse(html.UnescapeString(b.String()))
}

// collapse normaliza espaços: um espaço entre palavras e um parágrafo por
// linha, sem linhas vazias.
func collapse(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.Join(strings.Fields(strings.ReplaceAll(l, " ", " ")), " ")
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
