package model

import "testing"

func TestNormalize(t *testing.T) {
	if got := Normalize("  Engenheiro(a) de Produção — SÃO Paulo! "); got != "engenheiro a de producao sao paulo" {
		t.Fatalf("got %q", got)
	}
}

func TestFingerprintIgnoresFormatting(t *testing.T) {
	a := Fingerprint("Acme", "SRE Júnior", "São Paulo, SP, Brasil")
	b := Fingerprint("ACME", "sre junior", "Sao Paulo")
	if a != b {
		t.Fatalf("fingerprints differ: %s vs %s", a, b)
	}
	if a == Fingerprint("Acme", "SRE Pleno", "São Paulo") {
		t.Fatal("different titles should differ")
	}
}
