# Status das fontes

Legenda: **ok** funciona e tem teste · **frágil** funciona, mas depende de endpoint interno/não documentado · **quebrada** não funciona hoje · **descontinuada** o sistema não existe mais · **pendente** ainda não implementado.

| Fonte | Tipo | Status | Identificador (`id` / `params`) | Observações |
|---|---|---|---|---|
| Greenhouse | ATS público | ok | `id`: token do board (`boards.greenhouse.io/<token>`) | Job Board API pública com `content=true`. Leitura em streaming. |
| Lever | ATS público | pendente | | Fase 2 |
| Ashby | ATS público | pendente | | Fase 2 |
| Workable | ATS público | pendente | | Fase 2 |
| SmartRecruiters | ATS público | pendente | | Fase 2 |
| Recruitee | ATS público | pendente | | Fase 2 |
| Personio | ATS público (XML) | pendente | | Fase 2 |
| Teamtailor | ATS público | pendente | | Fase 2 |
| BambooHR | ATS público | pendente | | Fase 2 |
| Breezy HR | ATS público | pendente | | Fase 2 |
| Pinpoint | ATS público | pendente | | Fase 2 |
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
