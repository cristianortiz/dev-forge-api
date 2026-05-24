# DEV-FORGE — Plan de Implementación y Seguimiento

> Tracking de tareas, estado de fases y criterios de verificación.
> Actualizar este archivo cada vez que se complete una tarea.
>
> **Diseño y arquitectura:** [docs/DESIGN.md](docs/DESIGN.md)
> **Schema de base de datos:** [docs/db-schema.md](docs/db-schema.md)

**Última actualización:** 2026-05-21
**Estado global:** 🔵 En progreso — Fase 1 en curso (auth completo, próximo: templates)

---

## Leyenda de Estado

| Símbolo | Estado |
|---|---|
| ⬜ | No iniciado |
| 🔵 | En progreso |
| ✅ | Completado |
| ⏸️ | Pausado / Bloqueado |
| ⏭️ | Skipped (decisión explícita) |

---

## Resumen por Fase

| Fase | Nombre | Estado | Progreso |
|---|---|---|---|
| 0 | Setup del Proyecto | ✅ | 9/10 activas (1 skipped) |
| 1 | Auth (Zitadel) + Applications + Templates + Git | 🔵 | 5/12 |
| 2 | Build → Release → Deploy Pipeline | ⬜ | 0/11 |
| 3 | Configuration + Services | ⬜ | 0/6 |
| 4 | Observabilidad (OTEL + Grafana Stack) | ⬜ | 0/9 |
| 5 | Governance + Audit + Notifications | ⬜ | 0/8 |
| 6 | Self-Deploy + Portfolio Ready | ⬜ | 0/9 |

---

## Fase 0: Setup del Proyecto 🔵

> Fundamentos del proyecto: estructura, infraestructura local, tooling.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 0.1 | Crear `dev-forge.code-workspace` | ⏭️ | — | — | Skipped: se trabaja directamente en `dev_forge/` |
| 0.2 | Inicializar repo Go con go modules | ✅ | 2026-04-28 | 2026-04-28 | `go mod init github.com/cristianortiz/dev-forge` |
| 0.3 | Crear estructura de carpetas (cmd, internal, migrations, deployments) | ✅ | 2026-04-28 | 2026-04-28 | Todos los módulos creados |
| 0.4 | Docker Compose: PostgreSQL + Redis + Zitadel | ✅ | 2026-04-28 | 2026-04-28 | `deployments/docker-compose.yml` + `init-db.sql` |
| 0.5 | Package `shared/config` | ✅ | 2026-04-28 | 2026-04-28 | Struct tipada, helpers `getEnv*`, `Load()`, `LogLevel()` |
| 0.6 | Package `shared/database` | ✅ | 2026-04-28 | 2026-04-28 | pgxpool + otelpgx opcional |
| 0.7 | Package `shared/logger` | ✅ | 2026-04-28 | 2026-04-28 | Zap, JSON en prod / console en dev |
| 0.8 | Package `shared/server` | ✅ | 2026-04-28 | 2026-04-28 | Fiber v2, CORS, recovery, zapLogger, `/health`, `/ready` |
| 0.9 | Makefile | ✅ | 2026-04-28 | 2026-04-28 | Targets: build, test, lint, migrate-*, dev, docker-* |
| 0.10 | Verificar conectividad homelab K8s (kubeconfig, client-go ping) | ⬜ | — | — | Requiere kubeconfig local configurado |
| 0.11 | Configurar Zitadel: proyecto `dev-forge`, roles, app OIDC | ✅ | 2026-05-11 | 2026-05-21 | Zitadel Cloud; app JWT Profile; service user dev-forge-test con PAT |

**Notas de implementación verificadas:**
- `go.mod`: Go 1.26, Fiber v2, pgx v5, Zap, OTEL, otelpgx ✅
- `.env.example`: variables App, DB, Zitadel, OTEL documentadas ✅
- Zitadel Cloud configurado: proyecto `dev-forge`, roles admin/developer/viewer, app JWT Profile ✅
- `secrets/zitadel-api-key.json`: key RSA de la API app (gitignored) ✅
- `docs/zitadel-setup.md`: guía completa de setup y arquitectura de auth ✅
- `internal/shared/telemetry/`: vacío — pendiente Fase 4 (tarea 4.2)
- `internal/shared/middleware/`: vacío — pendiente Fase 1 (tarea 1.3)
- `internal/shared/crypto/`: vacío — pendiente Fase 3 (tarea 3.1)
- `migrations/`: vacío — se puebla en Fases 1, 2, 3 y 5

**Criterios de verificación:**
- [x] `make docker-up` levanta PostgreSQL
- [x] `make dev` arranca el servidor y responde en `/health`
- [x] `make test` ejecuta tests
- [x] `make migrate-up` aplica migración users
- [ ] `kubectl get nodes` conecta al homelab cluster (pendiente 0.10)
- [x] Zitadel Cloud configurado: proyecto, roles, app JWT Profile ✅
- [x] VS Code workspace abierto correctamente

**Próximo paso:** `make docker-up` → verificar Zitadel → completar 0.11 → verificar kubeconfig (0.10).

---

## Fase 1: Auth (Zitadel) + Applications + Templates + Git ⬜

> Primer corte vertical: autenticación, CRUD de apps desde plantilla, integración GitHub.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 1.1 | Módulo `auth`: domain (User entity, zitadel_id) | ✅ | 2026-05-02 | 2026-05-02 | `internal/auth/domain/user.go` |
| 1.2 | Módulo `auth`: ports + service (validar token, sync user, get me) | ✅ | 2026-05-02 | 2026-05-21 | zitadel/oidc/v3 introspection; JWT Profile con key RSA |
| 1.3 | Módulo `auth`: middleware Zitadel + `RequireRole` | ✅ | 2026-05-02 | 2026-05-02 | `internal/shared/middleware/auth.go`; claim `urn:zitadel:iam:org:project:roles` |
| 1.4 | Módulo `template`: domain + ports + service (CRUD) | ✅ | 2026-05-21 | 2026-05-21 | domain, ports, service + unit tests |
| 1.5 | Módulo `template`: adapters (HTTP handlers, PostgreSQL repo) | ⬜ | — | — | |
| 1.6 | Módulo `template`: seed inicial (Go API, React SPA, Node.js, Python) | ⬜ | — | — | Incluye Dockerfile template y defaults |
| 1.7 | Módulo `app`: domain + ports + service (CRUD, crear desde template) | ⬜ | — | — | Aplica defaults de template al crear |
| 1.8 | Módulo `app`: adapters (HTTP handlers, PostgreSQL repo) | ⬜ | — | — | |
| 1.9 | Módulo `git`: GitHub API (listar repos, branches) | ⬜ | — | — | Requiere GitHub token |
| 1.10 | Migraciones SQL: users, project_templates, clusters, applications | 🔵 | 2026-05-02 | — | `000001_create_users.up/down.sql` creado; resto pendiente |
| 1.11 | Módulo `cluster`: domain + ports + service (CRUD, token encriptado) | ⬜ | — | — | Admin-only; usado por `deploy` para construir client-go config |
| 1.12 | Módulo `cluster`: adapters (HTTP handlers admin + PostgreSQL repo) | ⬜ | — | — | Encripta token con `shared/crypto` |

**Notas de implementación verificadas:**
- `go build ./...` limpio ✅
- `go test ./...` pasa ✅
- Token introspection via `zitadel/oidc/v3` — soporta PAT y JWT ✅
- `GET /api/v1/auth/me` responde `{id, email, name, role}` ✅
- Usuario sincronizado en DB en primera llamada ✅
- `internal/auth/adapters/handler/` → `GET /api/v1/auth/me`
- `internal/auth/adapters/repository/` → pgx, users table
- `migrations/000001_create_users.up.sql` aplicada
- Auth module wired en `cmd/server/routes.go`
- **Unit tests añadidos (2026-05-21):**
  - `auth/domain`: `Role.IsValid()` — 100% coverage
  - `auth/service`: `SyncUser`, `GetUserByID`, `extractRoles` — 67% (ValidateToken/GetMe/New requieren Zitadel vivo → integration tests)
  - `template/service`: CRUD completo, validaciones, partial update, errores — 86% coverage
  - `shared/middleware`: `Authenticated`, `RequireRole`, `bearerToken`, `GetUser`, `GetClaims` — 100% coverage
- Estrategia de testing documentada en `docs/DESIGN.md` §10

**Criterios de verificación:**
- [x] GET `/api/v1/auth/me` → perfil + user sincronizado en DB ✅
- [ ] GET `/api/v1/templates` lista plantillas disponibles
- [ ] POST `/api/v1/apps` con `template_id` → defaults pre-cargados
- [ ] POST `/api/v1/apps` sin `template_id` → app vacía
- [ ] GET `/api/v1/github/repos` → repos reales de GitHub
- [ ] Viewer no puede crear apps (RBAC)
- [ ] Solo admin puede crear/editar plantillas
- [ ] POST `/api/v1/admin/clusters` registra cluster con token encriptado
- [ ] Tests ≥70% coverage en `domain/` y `service/` de cada módulo (unit) — ver `docs/DESIGN.md §10`

---

## Fase 2: Build → Release → Deploy Pipeline ⬜

> Flujo principal del IDP: Docker build, versionado, deploy al homelab K8s.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 2.1 | Módulo `build`: domain + ports + service | ⬜ | — | — | |
| 2.2 | Módulo `build`: adapter Docker SDK | ⬜ | — | — | |
| 2.3 | Módulo `build`: HTTP handlers + PostgreSQL repo | ⬜ | — | — | |
| 2.4 | Módulo `release`: completo (create from build, semver, list) | ⬜ | — | — | |
| 2.5 | Módulo `scope`: domain + service (CRUD) | ⬜ | — | — | |
| 2.6 | Módulo `scope`: adapter K8s (namespace via client-go) | ⬜ | — | — | |
| 2.7 | Módulo `deploy`: domain + service (provision → switch → finalize, rollback) | ⬜ | — | — | Usa interfaces de approval, scope, release |
| 2.8 | Módulo `deploy`: adapter K8s — objetos Go en memoria (Deployment, Service, Namespace) | ⬜ | — | — | Sin YAML en disco |
| 2.9 | Módulo `deploy`: HTTP handlers | ⬜ | — | — | |
| 2.10 | Migraciones SQL: builds, releases, scopes, deployments, deployment_events | ⬜ | — | — | |
| 2.11 | K8s RBAC en Helm chart: ServiceAccount + ClusterRole + ClusterRoleBinding | ⬜ | — | — | `deployments/helm/dev-forge/templates/rbac.yaml` |

**Criterios de verificación:**
- [ ] POST trigger build → imagen construida → status `successful`
- [ ] POST create release → versión semver creada
- [ ] POST create scope → namespace en homelab
- [ ] POST create deployment → pod running en homelab
- [ ] POST rollback → pod revierte al release anterior
- [ ] Tests unitarios + integración pasan

---

## Fase 3: Configuration + Services ⬜

> Parámetros, secrets y servicios externos.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 3.1 | Package `shared/crypto`: AES-256 | ⬜ | — | — | |
| 3.2 | Módulo `param`: completo (CRUD, scoping por env/scope, encrypt/decrypt) | ⬜ | — | — | |
| 3.3 | Módulo `param`: adapter K8s (ConfigMap/Secret desde parámetros) | ⬜ | — | — | |
| 3.4 | Módulo `service`: completo (catálogo tipos, CRUD, create vía Docker) | ⬜ | — | — | |
| 3.5 | Módulo `service`: service links (vincular a scope, inyectar conn params) | ⬜ | — | — | |
| 3.6 | Migraciones SQL: parameters, services, service_links | ⬜ | — | — | |

**Criterios de verificación:**
- [ ] Crear parámetro → deploy → env var visible en pod
- [ ] Parámetro secret almacenado encriptado en DB
- [ ] Crear servicio postgres → link a scope → conn params inyectados
- [ ] Filtros por environment y scope funcionan
- [ ] Tests pasan

---

## Fase 4: Observabilidad (OTEL + Grafana Stack) ⬜

> Traces, metrics y logs centralizados con el stack CNCF estándar.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 4.1 | Deploy observability stack en homelab (OTEL Collector, Jaeger, Prometheus, Grafana, Loki) | ⬜ | — | — | Manifests en `deployments/observability/` |
| 4.2 | Package `shared/telemetry`: OTEL SDK setup (tracer + meter provider) | ⬜ | — | — | Inicializar en `main.go` |
| 4.3 | Instrumentar middleware HTTP: traces por request, métricas latencia/status | ⬜ | — | — | |
| 4.4 | Instrumentar módulo `deploy`: spans por step | ⬜ | — | — | |
| 4.5 | Instrumentar módulo `build`: spans Docker build | ⬜ | — | — | |
| 4.6 | Structured logging → Loki (Promtail o OTEL Collector logs) | ⬜ | — | — | |
| 4.7 | Módulo `observe/logs`: pod logs con client-go + WebSocket streaming | ⬜ | — | — | |
| 4.8 | Módulo `observe/metrics`: proxy Prometheus API | ⬜ | — | — | CPU, mem, replicas |
| 4.9 | Grafana dashboards: dev-forge API + apps gestionadas | ⬜ | — | — | JSON/ConfigMap |

**Criterios de verificación:**
- [ ] Request → trace en Jaeger con spans handler/service/repository
- [ ] Grafana: dashboard dev-forge (request rate, latency p99, error rate)
- [ ] Grafana: dashboard apps (CPU, mem, replicas)
- [ ] Logs API visibles en Grafana → Loki
- [ ] GET `/api/v1/apps/:id/logs?scope=X` → logs reales del pod
- [ ] WS `/api/v1/apps/:id/logs/stream` → logs en tiempo real
- [ ] GET `/api/v1/apps/:id/metrics` → métricas reales
- [ ] Tests pasan

---

## Fase 5: Governance + Audit + Notifications ⬜

> Políticas de aprobación, audit log y webhooks.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 5.1 | Módulo `approval`: domain + service (políticas, requests, aprobar/rechazar) | ⬜ | — | — | |
| 5.2 | Módulo `approval`: integración con `deploy` (check policy) | ⬜ | — | — | |
| 5.3 | Módulo `approval`: HTTP handlers | ⬜ | — | — | |
| 5.4 | Módulo `audit`: middleware → audit_logs | ⬜ | — | — | |
| 5.5 | Módulo `audit`: HTTP handlers (listar con filtros) | ⬜ | — | — | |
| 5.6 | Módulo `notification`: domain + service (CRUD webhooks, dispatch) | ⬜ | — | — | HMAC signature |
| 5.7 | Módulo `notification`: HTTP handlers + dispatch async (goroutines, retry) | ⬜ | — | — | |
| 5.8 | Migraciones SQL: approval_policies, approval_requests, audit_logs, notification_webhooks | ⬜ | — | — | |

**Criterios de verificación:**
- [ ] Deploy a prod → request pendiente → estado `awaiting_approval`
- [ ] Aprobar request → deployment continúa → pod desplegado
- [ ] Deploy a dev → auto-aprobado
- [ ] GET `/api/v1/audit-logs` → historial completo
- [ ] Webhook recibe POST con payload al deployar
- [ ] Tests pasan

---

## Fase 6: Self-Deploy + Portfolio Ready ⬜

> dev-forge desplegado en homelab via Helm + polish para entrevistas.

| # | Tarea | Estado | Fecha inicio | Fecha fin | Notas |
|---|---|---|---|---|---|
| 6.1 | Helm chart completo: deployment, service, PostgreSQL, Redis | ⬜ | — | — | `deployments/helm/dev-forge/` |
| 6.2 | OpenAPI spec auto-generada + Swagger UI en `/api/v1/docs` | ⬜ | — | — | swag |
| 6.3 | Deploy dev-forge en homelab via Helm | ⬜ | — | — | Dogfooding |
| 6.4 | Seed data script: datos de demo realistas | ⬜ | — | — | |
| 6.5 | Docker Compose local: API + DB + Redis + Frontend | ⬜ | — | — | |
| 6.6 | README.md: arquitectura, screenshots, quick start | ⬜ | — | — | |
| 6.7 | Demo script: flujo guiado ~5 min para entrevistas | ⬜ | — | — | |
| 6.8 | CI pipeline (GitHub Actions): test + lint + build + push image | ⬜ | — | — | |
| 6.9 | Ingress + TLS en homelab para acceso externo | ⬜ | — | — | cert-manager + Let's Encrypt |

**Criterios de verificación:**
- [ ] `helm install dev-forge ./deployments/helm/dev-forge` despliega en homelab
- [ ] Swagger UI funciona en `/api/v1/docs`
- [ ] Docker Compose local levanta todo
- [ ] Demo script ejecutable de principio a fin
- [ ] CI pipeline pasa en GitHub Actions

---

## Frontend — `dev-forge-ui` (repo separado)

> Se crea al terminar Fase 1. El plan detallado vive en `dev-forge-ui/DEV-FORGE-UI-PLAN.md`.
> Contrato: OpenAPI spec en `/api/v1/docs/openapi.yaml`.

| # | Tarea | API asociada | Estado |
|---|---|---|---|
| F.1 | Setup React + Vite + routing + layout base | — | ⬜ |
| F.2 | Login + Zitadel OIDC callback | Auth API | ⬜ |
| F.3 | Application list + create (template selector) | Apps + Templates API | ⬜ |
| F.4 | Application dashboard shell | Apps API | ⬜ |
| F.5 | Builds page | Builds API | ⬜ |
| F.6 | Releases page | Releases API | ⬜ |
| F.7 | Scopes page | Scopes API | ⬜ |
| F.8 | Deployments page (steps, traffic view) | Deployments API | ⬜ |
| F.9 | Parameters page | Parameters API | ⬜ |
| F.10 | Services page | Services API | ⬜ |
| F.11 | Logs viewer (live tail) | `WS /apps/:id/logs/stream` | ⬜ |
| F.12 | Performance dashboard | Metrics API | ⬜ |
| F.13 | Approvals + policies | Approvals API | ⬜ |
| F.14 | Application dashboard completo | Dashboard API | ⬜ |
| A.1 | Admin layout + guard de rol | — | ⬜ |
| A.2 | Admin: Clusters CRUD | Admin Clusters API | ⬜ |
| A.3 | Admin: Users (solo lectura) | Admin Users API | ⬜ |
| A.4 | Admin: Platform settings | Admin Settings API | ⬜ |

---

## Log de Cambios

| Fecha | Cambio |
|---|---|
| 2026-03-12 | Plan inicial — análisis de Nully + diseño de dev-forge |
| 2026-03-12 | Módulo ProjectTemplates: apps desde plantillas predefinidas |
| 2026-03-12 | Kind → Homelab K8s (dogfooding) |
| 2026-03-12 | Event bus eliminado → llamadas directas por interfaces |
| 2026-03-12 | Observabilidad real: OTEL + Jaeger + Prometheus + Grafana + Loki |
| 2026-03-12 | Agregados: audit log, webhooks, OpenAPI docs, Helm chart |
| 2026-03-13 | Arquitectura de observabilidad en K8s documentada (3 capas) |
| 2026-03-13 | Auth propio → Zitadel OAuth2/OIDC; tabla users sin password_hash |
| 2026-03-13 | K8s RBAC dual documentado (app vs. proceso) |
| 2026-03-13 | K8s abstraction: apps sin manifests, objetos Go en memoria |
| 2026-04-28 | Entidad Cluster agregada; Scope.cluster (VARCHAR) → cluster_id (FK) |
| 2026-04-28 | Auth0 → Zitadel self-hosted; auth0_id → zitadel_id |
| 2026-04-28 | Frontend como repo separado `dev-forge-ui`; API-first |
| 2026-04-28 | Fase 0 completada (tareas 0.2–0.9) |
| 2026-05-01 | Separación en tres archivos: PLAN (tracking) + DESIGN + db-schema |
| 2026-05-02 | Pre-commit hook instalado (build + test en cada commit) |
| 2026-05-02 | Fase 1 iniciada: auth domain + ports + service + middleware + handler + repo + migración users |
| 2026-05-11 | Zitadel: app reemplazada por JWT Profile; introspección via zitadel/oidc/v3 |
| 2026-05-21 | Auth end-to-end verificado: GET /api/v1/auth/me → 200, user sincronizado en DB |
| 2026-05-21 | Unit tests añadidos: auth/domain (100%), auth/service (67%), template/service (86%), middleware (100%) |
| 2026-05-21 | Estrategia de testing documentada en docs/DESIGN.md §10 (unit/integration/E2E, targets, patrones) |
| 2026-05-21 | Tarea 1.4 completada: template domain + ports + service |

---

> **Próximo paso:** 1.4 módulo `template` (domain + ports + service) → 1.5 adapters → 1.6 seed data.
