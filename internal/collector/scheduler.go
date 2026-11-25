package collector

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"vagas/internal/config"
)

// Scheduler roda coletas periódicas e aceita disparos manuais.
type Scheduler struct {
	Collector  *Collector
	Interval   time.Duration
	RunOnStart bool
	// Companies é relida a cada rodada, para que editar companies.yaml não
	// exija reiniciar o app.
	Companies func() ([]config.Company, error)

	trigger   chan struct{}
	completed atomic.Uint64
	mu        sync.Mutex
	last      Summary
}

// Completed devolve quantas rodadas já terminaram (para a interface saber
// quando uma coleta disparada acabou).
func (s *Scheduler) Completed() uint64 { return s.completed.Load() }

// LastSummary devolve o resumo da última rodada concluída.
func (s *Scheduler) LastSummary() Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// NewScheduler cria um agendador.
func NewScheduler(c *Collector, interval time.Duration, runOnStart bool, companies func() ([]config.Company, error)) *Scheduler {
	return &Scheduler{Collector: c, Interval: interval, RunOnStart: runOnStart, Companies: companies, trigger: make(chan struct{}, 1)}
}

// Running informa se há coleta em andamento.
func (s *Scheduler) Running() bool { return s.Collector.Running() }

// Trigger pede uma coleta imediata. Devolve false se já há uma em andamento
// ou pendente.
func (s *Scheduler) Trigger() bool {
	if s.Collector.Running() {
		return false
	}
	select {
	case s.trigger <- struct{}{}:
		return true
	default:
		return false
	}
}

// Run bloqueia até ctx ser cancelado.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	if s.RunOnStart {
		s.once(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.trigger:
			ticker.Reset(s.Interval)
		}
		s.once(ctx)
	}
}

func (s *Scheduler) once(ctx context.Context) {
	companies, err := s.Companies()
	if err != nil {
		s.Collector.Log.Error("falha ao ler empresas", "erro", err)
		s.completed.Add(1) // libera a interface que aguarda o fim da rodada
		return
	}
	sum, err := s.Collector.RunAll(ctx, companies)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.Collector.Log.Error("coleta", "erro", err)
	}
	if errors.Is(err, ErrAlreadyRunning) {
		return
	}
	s.mu.Lock()
	s.last = sum
	s.mu.Unlock()
	s.completed.Add(1)
}
