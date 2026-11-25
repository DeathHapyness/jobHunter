// Package config carrega config.yaml, companies.yaml e .env.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config é a configuração geral do app.
type Config struct {
	Server    Server    `yaml:"server"`
	Database  Database  `yaml:"database"`
	Collector Collector `yaml:"collector"`
}

type Server struct {
	Addr string `yaml:"addr"`
}

type Database struct {
	Path string `yaml:"path"`
}

type Collector struct {
	Interval           time.Duration `yaml:"interval"`
	RequestTimeout     time.Duration `yaml:"request_timeout"`
	CompanyTimeout     time.Duration `yaml:"company_timeout"`
	MaxConcurrency     int           `yaml:"max_concurrency"`
	PerHostConcurrency int           `yaml:"per_host_concurrency"`
	Retries            int           `yaml:"retries"`
	UserAgent          string        `yaml:"user_agent"`
	RunOnStart         bool          `yaml:"run_on_start"`
}

// Company é uma empresa a ser coletada em uma fonte.
type Company struct {
	Name   string            `yaml:"name"`
	Source string            `yaml:"source"`
	ID     string            `yaml:"id"`
	Params map[string]string `yaml:"params"`
	// Disabled permite manter a entrada no arquivo sem coletar.
	Disabled bool `yaml:"disabled"`
}

// Param devolve um parâmetro específico do adaptador.
func (c Company) Param(key string) string { return c.Params[key] }

type companiesFile struct {
	Companies []Company `yaml:"companies"`
}

// Defaults devolve a configuração padrão.
func Defaults() Config {
	return Config{
		Server:   Server{Addr: ":8080"},
		Database: Database{Path: "data/vagas.db"},
		Collector: Collector{
			Interval:           6 * time.Hour,
			RequestTimeout:     30 * time.Second,
			CompanyTimeout:     5 * time.Minute,
			MaxConcurrency:     8,
			PerHostConcurrency: 2,
			Retries:            3,
			UserAgent:          "vagas-hub/0.1 (agregador pessoal de vagas)",
			RunOnStart:         true,
		},
	}
}

// Load lê config.yaml (se existir) sobre os padrões e aplica variáveis de
// ambiente VAGAS_ADDR e VAGAS_DB.
func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return cfg, err
	}
	if v := os.Getenv("VAGAS_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("VAGAS_DB"); v != "" {
		cfg.Database.Path = v
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	col := c.Collector
	switch {
	case col.Interval < time.Minute:
		return errors.New("collector.interval deve ser >= 1m")
	case col.MaxConcurrency < 1 || col.PerHostConcurrency < 1:
		return errors.New("concorrência deve ser >= 1")
	case col.RequestTimeout <= 0 || col.CompanyTimeout <= 0:
		return errors.New("timeouts devem ser > 0")
	case col.UserAgent == "":
		return errors.New("collector.user_agent é obrigatório")
	}
	return nil
}

// LoadCompanies lê companies.yaml e valida as entradas.
func LoadCompanies(path string) ([]Company, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f companiesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("companies %s: %w", path, err)
	}
	seen := make(map[string]bool)
	out := f.Companies[:0]
	for i, c := range f.Companies {
		c.Source = strings.ToLower(strings.TrimSpace(c.Source))
		if c.Name == "" || c.Source == "" {
			return nil, fmt.Errorf("companies[%d]: name e source são obrigatórios", i)
		}
		key := c.Source + "/" + c.Name + "/" + c.ID
		if seen[key] {
			return nil, fmt.Errorf("companies[%d]: entrada duplicada %s", i, key)
		}
		seen[key] = true
		if !c.Disabled {
			out = append(out, c)
		}
	}
	return out, nil
}

// LoadDotEnv carrega um arquivo .env simples (CHAVE=valor) sem sobrescrever
// variáveis já definidas no ambiente. Ausência do arquivo não é erro.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}
