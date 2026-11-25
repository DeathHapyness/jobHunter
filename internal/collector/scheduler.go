package collector

import (
	"context"
	"errors"
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

	trigger chan struct{}
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
		return
	}
	if _, err := s.Collector.RunAll(ctx, companies); err != nil && !errors.Is(err, context.Canceled) {
		s.Collector.Log.Error("coleta", "erro", err)
	}
}
