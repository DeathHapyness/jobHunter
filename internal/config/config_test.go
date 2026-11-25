package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAppliesYAMLOverDefaults(t *testing.T) {
	p := write(t, "config.yaml", "collector:\n  interval: 2h\n  max_concurrency: 3\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Collector.Interval != 2*time.Hour || cfg.Collector.MaxConcurrency != 3 {
		t.Fatalf("unexpected collector: %+v", cfg.Collector)
	}
	if cfg.Server.Addr != ":8080" {
		t.Fatalf("default addr lost: %q", cfg.Server.Addr)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCompanies(t *testing.T) {
	p := write(t, "companies.yaml", `companies:
  - name: Acme
    source: Greenhouse
    id: acme
  - name: Off
    source: lever
    id: off
    disabled: true
  - name: Wd
    source: workday
    id: wd
    params: {tenant: wd, host: wd5, site: External}
`)
	cs, err := LoadCompanies(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("want 2 enabled companies, got %d", len(cs))
	}
	if cs[0].Source != "greenhouse" {
		t.Fatalf("source not normalized: %q", cs[0].Source)
	}
	if cs[1].Param("host") != "wd5" {
		t.Fatalf("params not loaded: %+v", cs[1].Params)
	}
}

func TestLoadCompaniesRejectsDuplicates(t *testing.T) {
	p := write(t, "companies.yaml", "companies:\n  - {name: A, source: lever, id: a}\n  - {name: A, source: lever, id: a}\n")
	if _, err := LoadCompanies(p); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	t.Setenv("VAGAS_TEST_KEEP", "original")
	p := write(t, ".env", "# comentário\nVAGAS_TEST_KEEP=novo\nVAGAS_TEST_NEW=\"valor\"\n")
	if err := LoadDotEnv(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("VAGAS_TEST_NEW") })
	if os.Getenv("VAGAS_TEST_KEEP") != "original" || os.Getenv("VAGAS_TEST_NEW") != "valor" {
		t.Fatal("dotenv precedence wrong")
	}
}
