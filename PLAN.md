# Vagas — hub pessoal de vagas de TI

Agregador pessoal de vagas de TI (foco: infra, DevOps, SRE, cloud, júnior/estágio). Coleta vagas direto dos sistemas de recrutamento das empresas, guarda num banco local e mostra tudo numa interface minimalista com filtros.

Projeto privado, uso pessoal, roda no homelab.

## Regras de trabalho (Claude Code)

- **Autonomia dentro do projeto.** Execute o plano de ponta a ponta sem pedir aprovação entre fases. Só pare se algo for realmente impossível ou ambíguo de um jeito que muda o resultado.
- **Commits livres.** Pode commitar e criar branches neste repositório sempre que fizer sentido. Commits pequenos e frequentes, mensagens no padrão Conventional Commits (`feat:`, `fix:`, `chore:`, `test:`, `docs:`).
- **Peça autorização antes de:**
  - baixar ou instalar qualquer coisa nova que não esteja na stack deste plano (pacotes, ferramentas, imagens Docker, binários, dependências extras). As dependências da stack definida abaixo já estão autorizadas;
  - mexer em qualquer arquivo ou pasta fora do diretório deste projeto (configs do sistema, dotfiles, outros projetos, `~/.ssh`, etc.);
  - criar, clonar ou dar push em qualquer repositório além do deste projeto.
  Ao pedir, diga o que é, por que precisa e qual a alternativa sem isso.
- **Sem didática.** Não explique conceitos nem peça confirmação a cada passo. Entregue código funcionando e um resumo curto no final de cada fase.
- Código limpo, organizado e profissional; README claro.
- Se uma fonte não funcionar (endpoint mudou, exige login, bloqueia, foi descontinuada), registre em `docs/FONTES.md` com o motivo e siga em frente. Nunca contorne bloqueios nem captchas.
- Fontes possivelmente descontinuadas ou pouco documentadas (Kenoby — adquirida pela Gupy —, Pandapé, Abler): verificar primeiro se ainda existem; se não, registrar como "descontinuada" e não insistir.

## Escopo de fontes

Implementar adaptadores para **todos** os sistemas abaixo. Cada adaptador é independente: se um falhar, os outros continuam.

**Públicos e simples**
- Greenhouse
- Lever
- Ashby
- Workable
- SmartRecruiters
- Recruitee
- Personio (feed XML)
- Teamtailor
- BambooHR
- Breezy HR
- Pinpoint

**Endpoints internos / menos documentados**
- Workday
- iCIMS
- Jobvite
- Oracle Recruiting / Taleo
- SAP SuccessFactors

**Brasileiros**
- Gupy
- Kenoby
- Solides
- Inhire
- Pandapé
- Abler

**Sites próprios (módulos separados, marcados como frágeis)**
- Amazon
- Apple
- Microsoft
- Google

**Agregadores com API**
- Adzuna
- Jooble
- The Muse
(chaves de API ficam em `.env`; se a chave não existir, o adaptador é pulado)

**Fora de escopo, não implementar:** LinkedIn, Indeed, Glassdoor ou qualquer scraping que viole termos de uso ou exija login.

## Stack

Prioridades do dono: linguagem **rápida, segura e leve**. Go é a linguagem principal dele.

- **Linguagem:** Go (binário único, sem runtime externo)
- **Servidor e templates:** `net/http` + `html/template` da stdlib, HTMX para filtros sem recarregar a página (arquivo servido localmente, embutido via `embed`, nunca por CDN), CSS próprio enxuto (sem biblioteca pesada). Roteador opcional: `chi`
- **Banco:** SQLite via `modernc.org/sqlite` (Go puro, sem CGO), modo WAL, busca textual com FTS5
- **Toolchain:** Go 1.22 instalado localmente; escolher versões de dependências compatíveis com ele
- **Coleta:** goroutines com `context`, timeout, retry com backoff e concorrência limitada por host (`golang.org/x/sync/errgroup` ou semáforo)
- **Config:** YAML (`gopkg.in/yaml.v3`) e `.env`
- **Agendamento:** goroutine com `time.Ticker` dentro do próprio app
- **Logs:** `log/slog`
- **Deploy:** Docker multi-stage com imagem final `gcr.io/distroless/static` (já traz certificados CA e tzdata) + docker-compose no homelab. Healthcheck via subcomando do próprio binário (`vagas healthcheck`), já que a imagem não tem `curl`
- **Testes:** `go test`, com respostas salvas (fixtures) para os adaptadores, sem acessar a rede. `go vet` e `gofmt` sem avisos, `-race` nos testes de concorrência

**Limite de recursos: o app inteiro deve usar no máximo 400 MB de RAM**, de preferência bem menos (alvo: abaixo de 100 MB). Definir `mem_limit: 400m` no docker-compose e `GOMEMLIMIT=300MiB`. Evitar guardar descrições completas em memória: processar e gravar no banco.

Dependências acima já estão autorizadas. Não trocar de linguagem: manter Go, SQLite, Docker e o limite de RAM.

## Estrutura sugerida

Raiz do repositório: `jobHunter/` (binário e módulo chamados `vagas`).

```
jobHunter/
├── README.md
├── go.mod
├── docs/FONTES.md           # status de cada fonte (ok / frágil / quebrada / descontinuada)
├── companies.yaml           # empresa, sistema, identificador + params específicos do adaptador
├── config.yaml              # palavras-chave, intervalos, preferências
├── .env.example
├── cmd/vagas/main.go        # ponto de entrada: servidor + agendador
├── internal/
│   ├── model/               # Job e estado pessoal
│   ├── store/               # SQLite
│   ├── collector/           # orquestra os adaptadores
│   ├── classify/            # nível, área, remoto, país
│   ├── adapters/            # um pacote/arquivo por sistema
│   └── web/                 # handlers, templates, static
├── testdata/                # fixtures dos adaptadores
├── Dockerfile
└── docker-compose.yml
```

## Modelo de dados

`Job`: `id`, `source`, `source_job_id`, `company`, `title`, `description` (texto limpo, se disponível), `location`, `country`, `remote` (sim/híbrido/não/desconhecido), `level` (estágio/júnior/pleno/sênior/lead/desconhecido), `area` (infra/devops/sre/cloud/segurança/dev/dados/outros), `url`, `posted_at`, `first_seen`, `last_seen`, `closed_at`.

Campo extra `fingerprint`: hash de empresa + título + cidade normalizados (minúsculas, sem acento, sem pontuação). Usado para agrupar a mesma vaga vinda de fontes diferentes (ex.: Adzuna/Jooble repetindo uma vaga do Greenhouse); a interface mostra uma só, priorizando a fonte direta (ATS) sobre agregadores.

Estado pessoal por vaga fica em **tabela separada** (`job_state`: `job_id`, `status` nova/vista/salva/aplicada/descartada, `notes`, `updated_at`), para que a coleta nunca sobrescreva o que o usuário marcou.

Tabela `app_state` (chave/valor) guarda, entre outros, a data da última visita (usuário único, sem login) para o filtro "novas desde minha última visita".

Tabela `collection_runs`: fonte, empresa, início, fim, sucesso/erro, nº de vagas — base para o indicador de última coleta.

Chave única: `(source, company, source_job_id)`.

### Regra de fechamento de vagas

Uma vaga só recebe `closed_at` quando a coleta **daquela empresa naquela fonte terminou sem nenhum erro** (todas as páginas lidas) e a vaga não apareceu. Coleta parcial ou com falha nunca fecha vagas. Se a vaga reaparecer, `closed_at` volta a nulo.

### `companies.yaml`

Cada entrada tem `name`, `source`, `id` e um mapa livre `params` para o que for específico do sistema (ex.: Workday precisa de `tenant`, `host` como `wd1`…`wd5` e `site`; SuccessFactors e Taleo têm formatos próprios). Cada adaptador valida os próprios params.

## Fases

1. **Base + fatia vertical:** repositório (`git init`), estrutura, modelos, banco, interface de adaptador, configuração, **um adaptador real (Greenhouse)**, lista simples na interface e Docker. Objetivo: validar ponta a ponta (coleta → banco → tela → container → RAM) antes de escalar.
2. **Adaptadores simples:** Lever, Ashby, Workable, SmartRecruiters, Recruitee, Personio, Teamtailor, BambooHR, Breezy, Pinpoint, com testes por fixture.
3. **Adaptadores brasileiros:** Gupy, Kenoby, Solides, Inhire, Pandapé, Abler.
4. **Adaptadores complexos:** Workday, iCIMS, Jobvite, Oracle/Taleo, SuccessFactors.
5. **Sites próprios e agregadores:** Amazon, Apple, Microsoft, Google, Adzuna, Jooble, The Muse.
6. **Classificação:** inferir nível, área, remoto e país a partir de título, local e descrição (regras e palavras-chave configuráveis, sem LLM obrigatório).
7. **Interface e filtros** (ver abaixo).
8. **Automação e deploy:** coleta periódica, logs, healthcheck, docker-compose pronto para o homelab, README final.
9. **Empresas:** preencher `companies.yaml` com o máximo de empresas relevantes que o Claude Code conseguir validar (incluindo Anthropic, OpenAI, ElevenLabs, Linux Foundation, Amazon, Apple, Harvard, KaBuM, Casas Bahia, entre outras de tech, infra e cloud, brasileiras e internacionais). Descobrir o sistema de cada uma pela URL da página de carreiras e validar que o identificador responde.

## Interface

Minimalista: uma tela só, sem login, claro/escuro automático, funciona bem no celular.

- Lista de vagas com empresa, título, local, nível, selo "remoto", selo "nova" e link direto para a vaga.
- Busca por texto (título e descrição).
- **Filtros** (combináveis, refletidos na URL para poder salvar e compartilhar):
  - empresa (multi)
  - fonte/sistema (multi)
  - nível (estágio, júnior, pleno, sênior…)
  - área (infra, devops, sre, cloud, segurança…)
  - país / cidade
  - remoto / híbrido / presencial
  - data de publicação (24h, 7 dias, 30 dias)
  - somente novas desde minha última visita
  - status pessoal (salva, aplicada, descartada, ocultar descartadas)
- Ordenação por mais recente ou por empresa.
- Ações por vaga: salvar, marcar como aplicada, descartar, anotar.
- Filtros salvos (ex.: "Júnior DevOps Brasil") acessíveis por link.
- Contador de resultados e indicador da última coleta por fonte.

## Extras (depois do essencial)

- Aviso de vagas novas por Telegram ou Discord (webhook), respeitando os filtros salvos.
- Feed RSS por filtro.
- Exportar para CSV.

## Segurança e operação

- Uso privado: sem autenticação própria, mas o serviço deve escutar só na rede local (ou atrás de Tailscale/Cloudflare Tunnel com acesso restrito). Documentar no README.
- User-Agent identificável (genérico, sem dados pessoais), respeito a rate limit e a `robots.txt` em **todas** as fontes.
- Concorrência limitada por host real (ex.: vários tenants Workday compartilham `*.myworkdayjobs.com`).
- Segredos só em `.env`, nunca no repositório.

## Critérios de pronto

- `docker compose up` sobe o app, faz a primeira coleta e mostra vagas.
- Adicionar uma empresa exige editar só `companies.yaml`.
- Todas as fontes do escopo têm adaptador, teste e entrada em `docs/FONTES.md` com o status real.
- Filtros funcionam combinados e persistem na URL.
- Testes passam offline (`go test -race ./...`, `go vet` e `gofmt` limpos).
- Consumo de RAM abaixo de 400 MB (medido com `docker stats` após uma coleta completa).
- README explica instalação, configuração, como adicionar empresa e fonte.
