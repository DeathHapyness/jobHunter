// Package collector orquestra os adaptadores: roda cada empresa com timeout,
// concorrência limitada, grava em lotes e aplica a regra de fechamento.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"vagas/internal/adapters"
	"vagas/internal/classify"
	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/model"
	"vagas/internal/store"
)

const batchSize = 200

// Lookup encontra o adaptador de uma fonte (permite injetar em testes).
type Lookup func(source string) (adapters.Adapter, bool)

// Collector coleta todas as empresas configuradas.
type Collector struct {
	Store       *store.Store
	HTTP        *httpx.Client
	Log         *slog.Logger
	Concurrency int
	Timeout     time.Duration // por empresa
	Lookup      Lookup
	// OnlyTech descarta vagas que não são de TI (classify.IsTech).
	OnlyTech bool
	Now      func() time.Time

	running atomic.Bool
}

// Summary resume uma rodada completa.
type Summary struct {
	Companies int
	Failed    int
	Jobs      int
	New       int // vagas vistas pela primeira vez nesta rodada
	Closed    int64
	Duration  time.Duration
}

// ErrAlreadyRunning indica que já há uma coleta em andamento.
var ErrAlreadyRunning = errors.New("coleta já em andamento")

// Running informa se há coleta em andamento.
func (c *Collector) Running() bool { return c.running.Load() }

// RunAll coleta todas as empresas. Falha em uma não interrompe as outras.
func (c *Collector) RunAll(ctx context.Context, companies []config.Company) (Summary, error) {
	if !c.running.CompareAndSwap(false, true) {
		return Summary{}, ErrAlreadyRunning
	}
	defer c.running.Store(false)

	start := c.now()
	var (
		mu  sync.Mutex
		sum = Summary{Companies: len(companies)}
		wg  sync.WaitGroup
		sem = make(chan struct{}, max(c.Concurrency, 1))
	)
	for _, comp := range companies {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return sum, ctx.Err()
		}
		wg.Add(1)
		go func(comp config.Company) {
			defer wg.Done()
			defer func() { <-sem }()
			r := c.RunCompany(ctx, comp)
			mu.Lock()
			defer mu.Unlock()
			sum.Jobs += r.Jobs
			sum.Closed += r.Closed
			if !r.OK {
				sum.Failed++
			}
		}(comp)
	}
	wg.Wait()
	sum.Duration = c.now().Sub(start)
	if n, err := c.Store.CountFirstSeenSince(context.WithoutCancel(ctx), start); err == nil {
		sum.New = n
	}
	c.Log.Info("coleta concluída", "empresas", sum.Companies, "falhas", sum.Failed,
		"vagas", sum.Jobs, "novas", sum.New, "fechadas", sum.Closed, "duração", sum.Duration.Round(time.Millisecond))
	return sum, nil
}

// RunCompany coleta uma empresa e registra o resultado. Nunca entra em pânico
// por causa de um adaptador.
func (c *Collector) RunCompany(ctx context.Context, comp config.Company) (run store.Run) {
	run = store.Run{Source: comp.Source, Company: comp.Name, StartedAt: c.now()}
	log := c.Log.With("fonte", comp.Source, "empresa", comp.Name)
	defer func() {
		if p := recover(); p != nil {
			run.OK, run.Error = false, fmt.Sprintf("panic: %v", p)
		}
		run.FinishedAt = c.now()
		if err := c.Store.RecordRun(context.WithoutCancel(ctx), run); err != nil {
			log.Error("falha ao registrar coleta", "erro", err)
		}
		if run.OK {
			log.Info("empresa coletada", "vagas", run.Jobs, "fora_de_ti", run.Skipped, "fechadas", run.Closed,
				"duração", run.FinishedAt.Sub(run.StartedAt).Round(time.Millisecond))
		} else {
			log.Warn("falha na coleta", "erro", run.Error, "vagas_lidas", run.Jobs)
		}
	}()

	a, ok := c.lookup(comp.Source)
	if !ok {
		run.Error = "fonte desconhecida: " + comp.Source
		return run
	}

	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	batch := make([]model.Job, 0, batchSize)
	flush := func() error {
		if err := c.Store.UpsertJobs(cctx, batch, run.StartedAt); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	err := a.Fetch(cctx, c.HTTP, comp, func(j model.Job) error {
		if j.SourceJobID == "" || j.Title == "" || j.URL == "" {
			return nil // vaga incompleta: ignora sem invalidar a coleta
		}
		j.Source, j.Company, j.Aggregator = a.Source(), comp.Name, a.Aggregator()
		classify.Apply(&j)
		if c.OnlyTech && !classify.IsTech(j) {
			run.Skipped++
			return nil
		}
		j.Fingerprint = model.Fingerprint(j.Company, j.Title, j.Location)
		batch = append(batch, j)
		run.Jobs++
		if len(batch) >= batchSize {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	} else if ferr := flush(); ferr != nil { // grava o que foi lido, mesmo com falha
		log.Error("falha ao gravar lote parcial", "erro", ferr)
	}
	if err != nil {
		run.Error = err.Error()
		return run
	}

	// Coleta completa e sem erro: só agora é seguro fechar as que sumiram.
	closed, err := c.Store.CloseMissing(cctx, comp.Source, comp.Name, run.StartedAt, c.now())
	if err != nil {
		run.Error = "fechar vagas: " + err.Error()
		return run
	}
	run.Closed, run.OK = closed, true
	return run
}

func (c *Collector) lookup(source string) (adapters.Adapter, bool) {
	if c.Lookup != nil {
		return c.Lookup(source)
	}
	return adapters.Get(source)
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}
