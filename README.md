# jobHunter (Vagas)

Agregador pessoal de vagas de TI: infra, DevOps, SRE, cloud, segurança, dev e dados, com atenção a vagas de júnior e estágio.

Ele coleta as vagas direto dos sistemas de recrutamento das empresas, guarda tudo num SQLite local e mostra numa interface web retrô e minimalista, com filtros, busca e controle do que já foi visto, salvo, aplicado ou descartado.

Roda num container leve (imagem com cerca de 23 MB, uso de RAM entre 30 e 60 MB), pensado para homelab.

> **Sobre este projeto:** a ideia, os requisitos e as decisões são humanos.
> O código foi escrito com ajuda de IA ([Claude Code](https://claude.com/claude-code)),
> seguindo um plano detalhado (`PLAN.md`) e revisado por mim.

## Funcionalidades

- **Mais de 25 fontes:**
  - ATS públicos: Greenhouse, Lever, Ashby, Workable, Recruitee, Personio, Teamtailor, BambooHR, Breezy e Pinpoint.
  - Corporativos: Workday, iCIMS, Jobvite, Oracle Recruiting Cloud e SAP SuccessFactors.
  - Brasileiros: Gupy, Sólides, Inhire e Pandapé.
  - Big techs: Amazon, Apple e Microsoft.
  - O status de cada fonte está em [`docs/FONTES.md`](docs/FONTES.md).
- **Só vagas de TI:** classifica nível (estágio → lead), área, modelo de trabalho (remoto, híbrido ou presencial) e local (país › estado › cidade).
- **Sem duplicatas:** a mesma vaga publicada em fontes diferentes aparece uma vez só.
- **Fecha vagas com segurança:** uma vaga só é marcada como fechada depois de uma coleta completa e bem-sucedida da empresa.
- **Interface HTMX:**
  - Filtros múltiplos com local em cascata, busca de texto (FTS5) e destaque para as vagas novas desde a última visita.
  - Notas por vaga.
  - Coleta manual com resultados aparecendo ao vivo.
- **Coleta educada:** User-Agent identificável, respeita o `robots.txt` e os limites de requisição (429/Retry-After) e limita a concorrência por host. Não faz login, não contorna bloqueios nem captchas. LinkedIn, Indeed e Glassdoor estão fora do escopo.

## Rodando com Docker

```sh
cp .env.example .env          # opcional: chaves de agregadores
docker compose up -d --build
```

Abra http://127.0.0.1:8080. Se a porta 8080 estiver ocupada, use `VAGAS_PORT=8091 docker compose up -d`.

O banco fica no volume `./data`. O container sobe com `mem_limit: 400m` e `GOMEMLIMIT=300MiB`, sistema de arquivos somente-leitura e sem capabilities.

## Rodando sem Docker

Requer Go 1.22 ou mais novo.

```sh
go build -o bin/vagas ./cmd/vagas
./bin/vagas serve       # interface + coletas agendadas
./bin/vagas collect     # uma coleta e sai
```

Flags: `-config config.yaml`, `-companies companies.yaml`, `-env .env` e `-addr :8080`.

## Configuração

- **`config.yaml`:** endereço, banco, intervalo de coleta (padrão 6 h), timeouts, concorrência, User-Agent e `only_tech`.
- **`companies.yaml`:** lista de empresas acompanhadas.
- **`.env`:** segredos (chaves de API). Nunca vai para o repositório.

Variáveis de ambiente: `VAGAS_ADDR`, `VAGAS_DB`, `VAGAS_LOG_LEVEL` e `VAGAS_LOG_FORMAT`.

### Adicionando uma empresa

Descubra qual sistema a página de vagas usa (normalmente aparece na URL) e adicione uma linha:

```yaml
companies:
  - {name: Cloudflare, source: greenhouse, id: cloudflare}            # boards.greenhouse.io/cloudflare
  - {name: Stefanini, source: gupy, id: stefanini}                    # stefanini.gupy.io
  - {name: NVIDIA, source: workday, id: nvidia,                       # nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite
     params: {host: wd5, site: NVIDIAExternalCareerSite}}
  - {name: Exemplo, source: lever, id: exemplo, disabled: true}       # desativada
```

Os `params` aceitos por cada fonte estão em [`docs/FONTES.md`](docs/FONTES.md).

### Adicionando uma fonte

1. Crie um tipo em `internal/adapters/` que implemente `Adapter` (`Source`, `Aggregator`, `Fetch`) e registre-o com `Register` em `init()`.
2. Em `Fetch`, emita cada vaga com `emit(model.Job{...})` assim que ela for lida (streaming, sem acumular em memória).
   - Devolva erro se a listagem não foi lida inteira.
   - Devolva `ErrTruncated` se parou num limite de páginas.
3. Use o `httpx.Client` recebido: ele já cuida do robots.txt, dos retries e da concorrência por host.
4. Grave uma resposta real reduzida em `testdata/<fonte>/` e escreva um teste com `fixtureServer`.
5. Registre a fonte em `docs/FONTES.md`.

## Acesso pela rede

Por padrão a porta só escuta em `127.0.0.1`. Para acessar de outros aparelhos, troque o mapeamento no `docker-compose.yml` pelo IP da máquina na rede local (por exemplo `192.168.0.10:8080:8080`) ou use uma VPN como o Tailscale.

Não há autenticação: **não exponha na internet.**

## Desenvolvimento

```sh
go test -race ./...
go vet ./...
```

Estrutura:

| Pasta | Conteúdo |
|---|---|
| `cmd/vagas` | binário (`serve`, `collect`, `healthcheck`) |
| `internal/adapters` | um adaptador por fonte, com testes e fixtures em `testdata/` |
| `internal/collector` | coleta concorrente, agendador e fechamento de vagas |
| `internal/classify` | nível, área, modelo de trabalho, local e filtro de TI |
| `internal/store` | SQLite (WAL, FTS5, migrações) |
| `internal/web` | servidor HTTP, templates e estáticos (HTMX local, CSP restrita) |
| `internal/httpx` | cliente HTTP com robots.txt, retry e limite por host |

## Licença

[MIT](LICENSE).
