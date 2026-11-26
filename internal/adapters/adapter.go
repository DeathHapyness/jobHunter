// Package adapters contém um adaptador por sistema de recrutamento (ATS),
// site próprio ou agregador. Cada adaptador é independente e se registra
// em init().
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"

	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
)

// ErrTruncated indica que a listagem foi lida só até o limite configurado
// (ex.: params.max_pages). As vagas lidas são válidas, mas a coleta não é
// completa: o coletor não fecha vagas ausentes, só as que não aparecem há
// muito tempo.
var ErrTruncated = errors.New("listagem truncada no limite de páginas")

// truncated cria um ErrTruncated com contexto.
func truncated(source string, pages int) error {
	return fmt.Errorf("%s: %w (%d páginas)", source, ErrTruncated, pages)
}

// Emit recebe cada vaga assim que ela é lida, para não acumular a lista
// inteira (e as descrições) em memória.
type Emit func(model.Job) error

// Adapter coleta as vagas de uma empresa em um sistema.
type Adapter interface {
	// Source é o nome usado em companies.yaml (ex.: "greenhouse").
	Source() string
	// Aggregator indica fontes que repetem vagas de outros sistemas.
	Aggregator() bool
	// Fetch deve devolver erro se a listagem não foi lida por completo:
	// só uma coleta completa permite fechar vagas que sumiram.
	Fetch(ctx context.Context, hc *httpx.Client, c config.Company, emit Emit) error
}

var (
	mu       sync.RWMutex
	registry = map[string]Adapter{}
)

// Register adiciona um adaptador ao registro global.
func Register(a Adapter) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[a.Source()]; dup {
		panic("adaptador duplicado: " + a.Source())
	}
	registry[a.Source()] = a
}

// Get devolve o adaptador de uma fonte.
func Get(source string) (Adapter, bool) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := registry[source]
	return a, ok
}

// Sources lista as fontes registradas, em ordem alfabética.
func Sources() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// streamArray percorre, sem carregar tudo em memória, o array JSON que está
// na chave key do objeto raiz (ou o próprio array raiz se key == "").
// Para cada elemento chama fn com o decoder posicionado nele.
func streamArray(r io.Reader, key string, fn func(dec *json.Decoder) error) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("json: %w", err)
	}
	if key == "" {
		if tok != json.Delim('[') {
			return fmt.Errorf("json: esperava array na raiz, veio %v", tok)
		}
		return eachElement(dec, fn)
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("json: esperava objeto na raiz, veio %v", tok)
	}
	found := false
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return fmt.Errorf("json: %w", err)
		}
		if k, _ := kt.(string); k == key {
			t, err := dec.Token()
			if err != nil {
				return fmt.Errorf("json: %w", err)
			}
			if t == nil { // "key": null
				found = true
				continue
			}
			if t != json.Delim('[') {
				return fmt.Errorf("json: %q não é array", key)
			}
			if err := eachElement(dec, fn); err != nil {
				return err
			}
			found = true
			continue
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return fmt.Errorf("json: %w", err)
		}
	}
	if !found {
		return fmt.Errorf("json: chave %q ausente", key)
	}
	return nil
}

func eachElement(dec *json.Decoder, fn func(*json.Decoder) error) error {
	for dec.More() {
		if err := fn(dec); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // ']'
		return fmt.Errorf("json: %w", err)
	}
	return nil
}
