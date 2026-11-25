// Package model define os tipos centrais do domínio: vaga e estado pessoal.
package model

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

// Remote indica a modalidade de trabalho.
type Remote string

const (
	RemoteYes     Remote = "sim"
	RemoteHybrid  Remote = "hibrido"
	RemoteNo      Remote = "nao"
	RemoteUnknown Remote = "desconhecido"
)

// Level indica a senioridade da vaga.
type Level string

const (
	LevelIntern  Level = "estagio"
	LevelJunior  Level = "junior"
	LevelMid     Level = "pleno"
	LevelSenior  Level = "senior"
	LevelLead    Level = "lead"
	LevelUnknown Level = "desconhecido"
)

// Area indica a área técnica da vaga.
type Area string

const (
	AreaInfra    Area = "infra"
	AreaDevOps   Area = "devops"
	AreaSRE      Area = "sre"
	AreaCloud    Area = "cloud"
	AreaSecurity Area = "seguranca"
	AreaDev      Area = "dev"
	AreaData     Area = "dados"
	AreaOther    Area = "outros"
)

// Status é o estado pessoal do usuário em relação a uma vaga.
type Status string

const (
	StatusNew       Status = "nova"
	StatusSeen      Status = "vista"
	StatusSaved     Status = "salva"
	StatusApplied   Status = "aplicada"
	StatusDiscarded Status = "descartada"
)

// ValidStatus informa se s é um status conhecido.
func ValidStatus(s Status) bool {
	switch s {
	case StatusNew, StatusSeen, StatusSaved, StatusApplied, StatusDiscarded:
		return true
	}
	return false
}

// Job é uma vaga coletada de uma fonte.
type Job struct {
	ID          int64
	Source      string
	SourceJobID string
	Company     string
	Title       string
	Description string
	Location    string
	City        string
	Country     string
	Remote      Remote
	Level       Level
	Area        Area
	URL         string
	PostedAt    time.Time // zero quando desconhecido
	FirstSeen   time.Time
	LastSeen    time.Time
	ClosedAt    time.Time // zero quando aberta
	Fingerprint string
	Aggregator  bool // veio de agregador (Adzuna, Jooble...) e não do ATS direto
}

// JobView é a vaga combinada com o estado pessoal, usada na interface.
type JobView struct {
	Job
	Status Status
	Notes  string
}

// Fingerprint gera um identificador estável para a mesma vaga vinda de
// fontes diferentes: empresa + título + cidade, normalizados.
func Fingerprint(company, title, location string) string {
	city := location
	if i := strings.IndexAny(city, ",|;/("); i >= 0 {
		city = city[:i]
	}
	key := Normalize(company) + "|" + Normalize(title) + "|" + Normalize(city)
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:10])
}

var accentReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// Normalize deixa o texto em minúsculas, sem acentos e sem pontuação,
// com espaços simples.
func Normalize(s string) string {
	s = accentReplacer.Replace(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(s))
	space := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
