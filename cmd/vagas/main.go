// Comando vagas: servidor web + agendador de coletas.
//
//	vagas [serve]      sobe a interface e a coleta periódica (padrão)
//	vagas collect      roda uma coleta completa e sai
//	vagas healthcheck  consulta /healthz (usado pelo Docker)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"vagas/internal/collector"
	"vagas/internal/config"
	"vagas/internal/httpx"
	"vagas/internal/store"
	"vagas/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", env("VAGAS_CONFIG", "config.yaml"), "arquivo de configuração")
	compPath := fs.String("companies", env("VAGAS_COMPANIES", "companies.yaml"), "arquivo de empresas")
	envPath := fs.String("env", ".env", "arquivo .env (opcional)")
	addr := fs.String("addr", "", "endereço do healthcheck (padrão: o do config)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := config.LoadDotEnv(*envPath); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	if cmd == "healthcheck" {
		a := *addr
		if a == "" {
			a = cfg.Server.Addr
		}
		return healthcheck(a)
	}

	log := newLogger()
	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("banco: %w", err)
	}
	defer st.Close()

	hc := httpx.New(httpx.Options{
		UserAgent:      cfg.Collector.UserAgent,
		RequestTimeout: cfg.Collector.RequestTimeout,
		PerHost:        cfg.Collector.PerHostConcurrency,
		Retries:        cfg.Collector.Retries,
	})
	col := &collector.Collector{
		Store: st, HTTP: hc, Log: log,
		Concurrency: cfg.Collector.MaxConcurrency, Timeout: cfg.Collector.CompanyTimeout,
		OnlyTech: cfg.Collector.OnlyTech,
	}
	companies := func() ([]config.Company, error) { return config.LoadCompanies(*compPath) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "collect":
		cs, err := companies()
		if err != nil {
			return err
		}
		sum, err := col.RunAll(ctx, cs)
		if err != nil {
			return err
		}
		if sum.Failed > 0 {
			return fmt.Errorf("%d de %d empresas falharam", sum.Failed, sum.Companies)
		}
		return nil
	case "serve":
		return serve(ctx, cfg, st, col, companies, log)
	default:
		return fmt.Errorf("comando desconhecido %q (use serve, collect ou healthcheck)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, st *store.Store, col *collector.Collector,
	companies func() ([]config.Company, error), log *slog.Logger) error {
	if _, err := companies(); err != nil {
		return fmt.Errorf("empresas: %w", err)
	}
	sched := collector.NewScheduler(col, cfg.Collector.Interval, cfg.Collector.RunOnStart, companies)
	handler, err := web.New(st, sched, log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go sched.Run(ctx)

	errc := make(chan error, 1)
	go func() {
		log.Info("servidor no ar", "addr", cfg.Server.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("encerrando")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func healthcheck(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("VAGAS_LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(os.Stdout, opts)
	if strings.EqualFold(os.Getenv("VAGAS_LOG_FORMAT"), "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
