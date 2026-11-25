# Status das fontes

Legenda: **ok** funciona e tem teste · **frágil** funciona, mas depende de endpoint interno/não documentado · **quebrada** não funciona hoje · **descontinuada** o sistema não existe mais · **pendente** ainda não implementado.

| Fonte | Tipo | Status | Identificador (`id` / `params`) | Observações |
|---|---|---|---|---|
| Greenhouse | ATS público | ok | `id`: token do board (`boards.greenhouse.io/<token>`) | Job Board API pública com `content=true`. Leitura em streaming. |
| Lever | ATS público | ok | `id`: slug (`jobs.lever.co/<slug>`); `params.region: eu` para contas EU | API pública `v0/postings?mode=json`, streaming. |
| Ashby | ATS público | ok | `id`: nome do board (`jobs.ashbyhq.com/<id>`) | Posting API pública; ignora vagas `isListed=false`. |
| Workable | ATS público | ok | `id`: subdomínio (`apply.workable.com/<id>`) | Widget API `?details=true`. |
| SmartRecruiters | ATS público | **bloqueada (robots.txt)** | `id`: identificador da empresa (case-sensitive) | Paginação `limit/offset` (máx. `params.max_pages`, padrão 100). Lista não traz descrição. **Desde 2026-11-25 o `robots.txt` de `api.smartrecruiters.com` tem `Disallow: /` para todos exceto LinkedInBot**; o adaptador respeita e não coleta. Empresas ficam com `disabled: true`. |
| Recruitee | ATS público | ok | `id`: subdomínio (`<id>.recruitee.com`) | `/api/offers/`. |
| Personio | ATS público (XML) | ok | `id`: subdomínio; `params.domain: jobs.personio.com` para contas .com; `params.language` | Feed XML `/xml`, leitura em streaming. |
| Teamtailor | ATS público | ok | `id`: subdomínio (`<id>.teamtailor.com`); `params.base_url` para domínio próprio | API oficial exige chave; usa o feed RSS público da página de carreiras (`/jobs.rss`). |
| BambooHR | ATS público | ok (fixture sintética) | `id`: subdomínio (`<id>.bamboohr.com`) | `/careers/list` (JSON). Nenhuma empresa testada tinha vagas abertas; fixture segue o formato documentado. Subdomínios inexistentes redirecionam (302) para o site da BambooHR → erro. |
| Breezy HR | ATS público | ok | `id`: subdomínio (`<id>.breezy.hr`) | `/json`. |
| Pinpoint | ATS público | ok | `id`: subdomínio (`<id>.pinpointhq.com`) | `/postings.json`. Sem data de publicação (usa primeira vez vista). |
| Gupy | ATS BR | frágil | `id`: subdomínio (`<id>.gupy.io`) | Lê a lista de vagas do `__NEXT_DATA__` da página de carreiras (liberada no robots.txt; `/companies` e `/candidates` são proibidos e não são usados). Ignora bancos de talentos. Sem data de publicação nem descrição na lista. O feed `job-board-content` só existe para algumas empresas. |
| Kenoby | ATS BR | **descontinuada** | — | `jobs.kenoby.com` não resolve mais (NXDOMAIN); a Kenoby foi incorporada pela Gupy. O adaptador existe e devolve erro explicativo. |
| Solides | ATS BR | frágil | `id`: slug (`<id>.vagas.solides.com.br`) | API `apigw.solides.com.br/jobs/v3/home/vacancy?slug=&page=` usada pela própria página de carreiras; paginada. Não documentada. |
| Inhire | ATS BR | frágil | `id`: tenant (`<id>.inhire.app`) | `api.inhire.app/job-posts/public/pages` com cabeçalho `X-Tenant`; usada pela página de carreiras. Sem data. |
| Pandapé | ATS BR | frágil | `id`: subdomínio (`<id>.pandape.infojobs.com.br`) | Sem API: lê os cartões do HTML de `/Vacancy/Vacancies` (robots.txt permite). Quebra se a marcação mudar. Só a primeira página. |
| Abler | ATS BR | **não suportada** | — | Não há endpoint público por empresa: o portal `candidatos.abler.com.br` só expõe um sitemap global (14 mil URLs, sem empresa). Buscar cada vaga para descobrir a empresa seria abusivo. O adaptador existe e devolve erro explicativo. |
| Workday | endpoint interno | pendente | | Fase 4 |
| iCIMS | endpoint interno | pendente | | Fase 4 |
| Jobvite | endpoint interno | pendente | | Fase 4 |
| Oracle Recruiting / Taleo | endpoint interno | pendente | | Fase 4 |
| SAP SuccessFactors | endpoint interno | pendente | | Fase 4 |
| Amazon | site próprio | pendente | | Fase 5 |
| Apple | site próprio | pendente | | Fase 5 |
| Microsoft | site próprio | pendente | | Fase 5 |
| Google | site próprio | pendente | | Fase 5 |
| Adzuna | agregador (API) | pendente | | Fase 5 — requer `ADZUNA_APP_ID`/`ADZUNA_APP_KEY` |
| Jooble | agregador (API) | pendente | | Fase 5 — requer `JOOBLE_API_KEY` |
| The Muse | agregador (API) | pendente | | Fase 5 |

## Empresas descartadas na validação

| Empresa | Tentativa | Motivo |
|---|---|---|
| DigitalOcean | greenhouse `digitalocean` | 404 |
| Nubank | greenhouse `nubank` | board existe, mas sem vagas publicadas por ele |
