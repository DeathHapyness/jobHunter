package adapters

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
)

// maxJSONBody limita respostas lidas inteiras em memória (as grandes usam streaming).
const maxJSONBody = 32 << 20

// getJSON busca e decodifica uma resposta JSON pequena/média.
func getJSON(ctx context.Context, hc *httpx.Client, rawURL string, v any) error {
	return doJSON(ctx, hc, httpx.Request{Method: http.MethodGet, URL: rawURL}, v)
}

// doJSON executa a requisição e decodifica o corpo JSON.
func doJSON(ctx context.Context, hc *httpx.Client, req httpx.Request, v any) error {
	if req.Header == nil {
		req.Header = http.Header{}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := hc.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBody))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("json de %s: %w", req.URL, err)
	}
	return nil
}

// postJSON envia body como JSON e decodifica a resposta.
func postJSON(ctx context.Context, hc *httpx.Client, rawURL string, body any, header http.Header, v any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")
	return doJSON(ctx, hc, httpx.Request{
		Method: http.MethodPost, URL: rawURL, Header: header,
		Body: func() io.Reader { return strings.NewReader(string(data)) },
	}, v)
}

// streamXML percorre os elementos <elem> de um documento XML, decodificando
// um por vez em fn.
func streamXML(r io.Reader, elem string, fn func(dec *xml.Decoder, start xml.StartElement) error) error {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("xml: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == elem {
			if err := fn(dec, se); err != nil {
				return err
			}
		}
	}
}

// subdomainURL monta https://{id}.{domain}{path}, validando o id.
func subdomainURL(c config.Company, domain, path string) (string, error) {
	id := strings.TrimSpace(c.ID)
	if id == "" || strings.ContainsAny(id, "/:?#@ ") {
		return "", fmt.Errorf("id inválido: %q", c.ID)
	}
	return "https://" + id + "." + domain + path, nil
}

// baseOr devolve params["base_url"] (para domínios próprios / testes) ou def.
func baseOr(c config.Company, def string) string {
	if b := strings.TrimRight(c.Param("base_url"), "/"); b != "" {
		return b
	}
	return def
}

// joinNonEmpty junta as partes não vazias com sep.
func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// numStr converte json.Number / número / string em string.
func numStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case json.Number:
		return x.String()
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// unixMillis converte milissegundos desde a época.
func unixMillis(v json.Number) time.Time {
	n, err := v.Int64()
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(n)
}

// remoteFlags converte booleans de remoto/híbrido numa modalidade.
func remoteFlags(remote, hybrid bool) model.Remote {
	switch {
	case hybrid:
		return model.RemoteHybrid
	case remote:
		return model.RemoteYes
	}
	return model.RemoteUnknown
}

// levelFromCode mapeia códigos de senioridade comuns nos ATS.
func levelFromCode(code string) model.Level {
	c := model.Normalize(code)
	switch {
	case c == "":
		return model.LevelUnknown
	case strings.Contains(c, "intern"), strings.Contains(c, "student"), strings.Contains(c, "estagi"), strings.Contains(c, "trainee"):
		return model.LevelIntern
	case strings.Contains(c, "entry"), strings.Contains(c, "junior"), strings.Contains(c, "associate"), strings.Contains(c, "graduate"):
		return model.LevelJunior
	case strings.Contains(c, "mid"), strings.Contains(c, "pleno"):
		return model.LevelMid
	case strings.Contains(c, "senior"), strings.Contains(c, "experienced"):
		return model.LevelSenior
	case strings.Contains(c, "director"), strings.Contains(c, "executive"), strings.Contains(c, "manager"), strings.Contains(c, "lead"):
		return model.LevelLead
	}
	return model.LevelUnknown
}

// query monta uma query string.
func query(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v.Encode()
}
