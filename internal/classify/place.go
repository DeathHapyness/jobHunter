package classify

import (
	"strings"

	"vagas/internal/model"
)

// Place extrai cidade, estado e país do primeiro local de um texto de
// localização ("São Paulo, SP, Brasil", "Seattle, WA", "Dublin, Ireland").
// Estados do Brasil e dos EUA viram sigla (SP, CA); o país é inferido do
// estado quando não vem escrito.
func Place(loc string) (city, state, country string) {
	fallbackCountry := ""
	groups := strings.FieldsFunc(loc, func(r rune) bool { return r == '|' || r == ';' || r == '/' || r == '\n' })
	for _, g := range groups {
		var segs []string
		for _, s := range strings.Split(g, ",") {
			s = cleanPlace(s)
			if s != "" && !isRemoteWord(s) {
				segs = append(segs, s)
			}
		}
		if len(segs) == 0 {
			continue
		}
		// País: último segmento, se reconhecido.
		if c := lookupCountry(model.Normalize(segs[len(segs)-1])); c != "" && countryOnly[model.Normalize(segs[len(segs)-1])] || isCountryCode(segs[len(segs)-1]) {
			country = lookupCountry(model.Normalize(segs[len(segs)-1]))
			segs = segs[:len(segs)-1]
		}
		if len(segs) == 0 {
			// Só país neste local ("Remote, United States"): tenta o próximo
			// local para achar a cidade.
			if fallbackCountry == "" {
				fallbackCountry = country
			}
			country = ""
			continue
		}
		city = segs[0]
		if len(segs) >= 2 {
			st, c := normState(segs[1], country)
			state = st
			if country == "" {
				country = c
			}
		} else if st, c := normState(city, country); st != "" && c != "" && isStateOnly(city) {
			// Só um segmento e é um estado ("SP", "California").
			return "", st, firstNonEmpty(country, c)
		}
		return city, state, firstNonEmpty(country, fallbackCountry)
	}
	return "", "", fallbackCountry
}

func cleanPlace(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"Remote - ", "Remoto - ", "Hybrid - ", "Híbrido - ", "Remote – ", "Remoto – "} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.Index(s, "("); i > 0 {
		s = s[:i]
	}
	return strings.Trim(s, " -–")
}

func isRemoteWord(s string) bool {
	n := " " + model.Normalize(s) + " "
	return containsAny(n, "remote", "remoto", "remota", "anywhere", "worldwide", "global", "multiple locations", "various", "home office", "hybrid", "hibrido")
}

func isCountryCode(s string) bool {
	n := model.Normalize(s)
	return n == "br" || n == "usa" || n == "us" || n == "uk"
}

// isStateOnly evita tratar uma cidade homônima de estado como estado
// (ex.: "São Paulo" sozinho é tratado como cidade).
func isStateOnly(s string) bool {
	n := model.Normalize(s)
	return len(n) == 2 || usStates[n] != ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// normState devolve o estado normalizado e o país que ele implica.
func normState(s, country string) (string, string) {
	n := model.Normalize(s)
	up := strings.ToUpper(n)
	switch country {
	case "", "Brasil":
		if _, ok := brUF[up]; ok && len(n) == 2 {
			return up, "Brasil"
		}
		if uf, ok := brStates[n]; ok {
			return uf, "Brasil"
		}
	}
	switch country {
	case "", "Estados Unidos":
		if _, ok := usCodes[up]; ok && len(n) == 2 {
			return up, "Estados Unidos"
		}
		if code := usStates[n]; code != "" {
			return code, "Estados Unidos"
		}
	}
	if country == "" {
		return "", ""
	}
	return strings.TrimSpace(s), country
}

var brStates = map[string]string{
	"acre": "AC", "alagoas": "AL", "amapa": "AP", "amazonas": "AM", "bahia": "BA", "ceara": "CE",
	"distrito federal": "DF", "espirito santo": "ES", "goias": "GO", "maranhao": "MA", "mato grosso": "MT",
	"mato grosso do sul": "MS", "minas gerais": "MG", "para": "PA", "paraiba": "PB", "parana": "PR",
	"pernambuco": "PE", "piaui": "PI", "rio de janeiro": "RJ", "rio grande do norte": "RN",
	"rio grande do sul": "RS", "rondonia": "RO", "roraima": "RR", "santa catarina": "SC",
	"sao paulo": "SP", "sergipe": "SE", "tocantins": "TO",
}

var brUF = func() map[string]bool {
	m := map[string]bool{}
	for _, uf := range brStates {
		m[uf] = true
	}
	return m
}()

var usStates = map[string]string{
	"alabama": "AL", "alaska": "AK", "arizona": "AZ", "arkansas": "AR", "california": "CA", "colorado": "CO",
	"connecticut": "CT", "delaware": "DE", "florida": "FL", "georgia": "GA", "hawaii": "HI", "idaho": "ID",
	"illinois": "IL", "indiana": "IN", "iowa": "IA", "kansas": "KS", "kentucky": "KY", "louisiana": "LA",
	"maine": "ME", "maryland": "MD", "massachusetts": "MA", "michigan": "MI", "minnesota": "MN",
	"mississippi": "MS", "missouri": "MO", "montana": "MT", "nebraska": "NE", "nevada": "NV",
	"new hampshire": "NH", "new jersey": "NJ", "new mexico": "NM", "new york": "NY", "north carolina": "NC",
	"north dakota": "ND", "ohio": "OH", "oklahoma": "OK", "oregon": "OR", "pennsylvania": "PA",
	"rhode island": "RI", "south carolina": "SC", "south dakota": "SD", "tennessee": "TN", "texas": "TX",
	"utah": "UT", "vermont": "VT", "virginia": "VA", "washington": "WA", "west virginia": "WV",
	"wisconsin": "WI", "wyoming": "WY", "district of columbia": "DC", "washington dc": "DC",
}

var usCodes = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range usStates {
		m[c] = true
	}
	return m
}()

// canonicalCity padroniza grafias comuns (sem acento, abreviações) para que
// "Sao Paulo" e "São Paulo" virem a mesma opção no filtro.
var canonicalCity = map[string]string{
	"sao paulo": "São Paulo", "sampa": "São Paulo", "rio de janeiro": "Rio de Janeiro", "rj": "Rio de Janeiro",
	"belo horizonte": "Belo Horizonte", "bh": "Belo Horizonte", "brasilia": "Brasília", "curitiba": "Curitiba",
	"florianopolis": "Florianópolis", "floripa": "Florianópolis", "porto alegre": "Porto Alegre",
	"recife": "Recife", "salvador": "Salvador", "fortaleza": "Fortaleza", "goiania": "Goiânia",
	"campinas": "Campinas", "sao jose dos campos": "São José dos Campos", "ribeirao preto": "Ribeirão Preto",
	"sorocaba": "Sorocaba", "santos": "Santos", "barueri": "Barueri", "osasco": "Osasco", "joinville": "Joinville",
	"blumenau": "Blumenau", "londrina": "Londrina", "maringa": "Maringá", "uberlandia": "Uberlândia",
	"vitoria": "Vitória", "manaus": "Manaus", "belem": "Belém", "natal": "Natal", "joao pessoa": "João Pessoa",
	"maceio": "Maceió", "sao carlos": "São Carlos", "sao bernardo do campo": "São Bernardo do Campo",
	"santo andre": "Santo André", "niteroi": "Niterói", "jundiai": "Jundiaí", "itajai": "Itajaí",
	"nyc": "New York", "new york city": "New York", "sf": "San Francisco", "sao leopoldo": "São Leopoldo",
}

// CanonicalCity devolve a grafia padrão de uma cidade.
func CanonicalCity(city string) string {
	if c, ok := canonicalCity[model.Normalize(city)]; ok {
		return c
	}
	return city
}
