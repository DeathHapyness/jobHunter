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
| Gupy | ATS BR | pendente | | Fase 3 |
| Kenoby | ATS BR | pendente | | Fase 3 — possivelmente descontinuada (adquirida pela Gupy) |
| Solides | ATS BR | pendente | | Fase 3 |
| Inhire | ATS BR | pendente | | Fase 3 |
| Pandapé | ATS BR | pendente | | Fase 3 |
| Abler | ATS BR | pendente | | Fase 3 |
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
