# DEV-FORGE — Diseño y Arquitectura

> Documento de referencia estático. Contiene la visión, el modelo de dominio, la arquitectura, las decisiones técnicas y el stack. **No** contiene estado de tareas ni progreso — eso vive en `DEV-FORGE-PLAN.md`.

**Última revisión:** 2026-05-01

---

## Índice

1. [Visión del Proyecto](#1-visión-del-proyecto)
2. [Análisis de Nully — Referencia](#2-análisis-de-nully--referencia)
3. [Modelo de Dominio](#3-modelo-de-dominio)
4. [Arquitectura](#4-arquitectura)
5. [Estructura del Proyecto](#5-estructura-del-proyecto)
6. [API REST](#6-api-rest)
7. [Integraciones Reales vs Simuladas](#7-integraciones-reales-vs-simuladas)
8. [Decisiones Técnicas](#8-decisiones-técnicas)
9. [Stack Tecnológico](#9-stack-tecnológico)

---

## 1. Visión del Proyecto

**dev-forge** es un prototipo semi-funcional de Internal Developer Platform (IDP) diseñado como proyecto de portafolio para roles de Platform Engineer. Está inspirado en el flujo de trabajo de la plataforma Nully.

**Objetivo:** Demostrar competencias en orquestación de infraestructura, gestión de configuración, observabilidad, CI/CD y governance, con un backend en Go (monolito modular, arquitectura hexagonal) y un frontend React en proyecto separado que consume la API exclusivamente.

**Alcance:** Semi-funcional — integraciones reales con GitHub, Docker y K8s (homelab cluster real), con observabilidad completa (OTEL + Grafana stack). El propio dev-forge corre en el cluster que gestiona (dogfooding).

**Diseño:** API-first — la API es el producto. El frontend `dev-forge-ui` es un cliente más de la API, en su propio repositorio.

**Tenancy:** Single-tenant con RBAC básico (admin, developer, viewer).

**Git Provider:** GitHub.

---

## 2. Análisis de Nully — Referencia

De las 20 capturas analizadas, Nully expone estos conceptos:

| Concepto Nully | Mapping en dev-forge | Simplificaciones |
|---|---|---|
| New Application (templates, config presets) | Application + Templates | Plantillas predefinidas por tipo de proyecto (Go API, React SPA, etc.) |
| Application | Application | Igual |
| Dashboard | Application Dashboard | Igual |
| Builds | Builds | Igual |
| Releases | Releases | Igual |
| Deployments (traffic switch, rollback, steps) | Deployments | Traffic switching simplificado (0/100) |
| Scopes (Environment × Country × Cloud) | Scopes | **Sin Country/Cloud** — solo Environment + Cluster |
| Parameters (dimensional scoping, secrets) | Parameters | Scoping por environment y scope, sin cloud dimension |
| Services (mongo-atlas, redis, postgres, links) | Services | Provisioning vía Docker, no cloud providers |
| Logs (real-time, structured JSON, filtros) | Observe/Logs | Logs reales vía Loki + Grafana |
| Performance (throughput, response, CPU, mem) | Observe/Metrics | OTEL + Prometheus + Grafana (stack real, no simulado) |
| Approvals (policies by action, request flow) | Approvals | Auto-approve dev, manual prod |

---

## 3. Modelo de Dominio

### Entidades Principales

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│     User     │     │  Application │────▶│    Build     │
│              │     │              │     │              │
│ - zitadel_id │     │ - name       │     │ - commit_sha │
│ - email      │     │ - repo_url   │     │ - branch     │
│ - role       │     │ - description│     │ - status     │
│   (admin/    │     │ - owner_id   │     │ - image      │
│    developer/│     │ - template_id│     │ - coverage   │
│    viewer)   │     │              │     │              │
└──────────────┘     └──────┬───────┘     └──────┬───────┘
                            │
                    ┌───────▼────────┐
                    │ProjectTemplate │
                    │                │
                    │- name          │
                    │- language      │
                    │- framework     │
                    │- dockerfile    │
                    │- default_params│
                    │- default_scope │
                    └───────────────┘
                            │                     │
                 ┌──────────┼──────────┐          │
                 ▼          ▼          ▼          ▼
          ┌──────────┐ ┌────────┐ ┌─────────┐ ┌─────────┐
          │  Scope   │ │ Param  │ │ Service │ │ Release │
          │          │ │        │ │         │ │         │
          │- environ │ │- name  │ │- type   │ │- version│
          │- cluster │ │- value │ │- name   │ │- build  │
          │- config  │ │- secret│ │- status │ │- status │
          └────┬─────┘ └────────┘ └────┬────┘ └────┬────┘
               │                       │            │
               │    ┌──────────────┐   │            │
               │    │ ServiceLink  │◀──┘            │
               │    │ - scope_id   │                │
               │    │ - conn_params│                │
               │    └──────────────┘                │
               │                                    │
               ▼                                    ▼
          ┌─────────────────────────────────────────────┐
          │              Deployment                     │
          │ - release_id    - scope_id                  │
          │ - status (pending/provisioning/switching/   │
          │           finalized/rolled_back)            │
          │ - created_by                                │
          └─────────────────┬───────────────────────────┘
                            │
                            ▼
          ┌─────────────────────────────────────────────┐
          │    ApprovalPolicy / ApprovalRequest          │
          │ - action (new_scope/deploy/delete_scope...) │
          │ - environment (dev auto / prod manual)      │
          │ - status (pending/approved/rejected)        │
          └─────────────────────────────────────────────┘
```

### Entidades Detalladas

| # | Entidad | Campos Clave |
|---|---|---|
| 1 | **User** | id, zitadel_id (unique), email, name, role (admin/developer/viewer), created_at |
| 2 | **Application** | id, name, description, repo_url, template_id (nullable), owner_id, created_at |
| 2b | **ProjectTemplate** | id, name, slug, description, language, framework, dockerfile_template, default_params (JSONB), default_scope_config (JSONB), repo_template_url, is_active, created_at |
| 2c | **Cluster** | id, name, slug, api_endpoint, token_encrypted, ca_cert, status (active/inactive), created_at |
| 3 | **Build** | id, app_id, description, branch, commit_sha, status (pending/building/successful/failed), docker_image, coverage, created_at |
| 4 | **Release** | id, app_id, version (semver), build_id, status (active/superseded), created_at |
| 5 | **Scope** | id, app_id, name, environment (dev/staging/prod), cluster_id (FK → Cluster), namespace, ram_mb, cpu_millicores, replicas, visibility (internal/public), health_check_path, status (running/stopped), created_at |
| 6 | **Deployment** | id, release_id, scope_id, status (pending/provisioning/switching_traffic/finalized/rolled_back), created_by, created_at, updated_at |
| 7 | **DeploymentEvent** | id, deployment_id, step, status, message, created_at |
| 8 | **Parameter** | id, app_id, name, value_encrypted, is_secret, environment, scope_id (nullable), delivered_as (env_var/file), created_at |
| 9 | **Service** | id, app_id, type (postgres/redis/mongo), name, provider, status (active/cancelled), created_at |
| 10 | **ServiceLink** | id, service_id, scope_id, status (active/inactive), connection_params_encrypted, created_at |
| 11 | **ApprovalPolicy** | id, app_id, action, environment, requires_approval, created_at |
| 12 | **ApprovalRequest** | id, policy_id, resource_type, resource_id, action, status (pending/approved/rejected/cancelled), requester_id, approver_id, created_at, resolved_at |
| 13 | **AuditLog** | id, user_id, action, resource_type, resource_id, details (JSONB), ip_address, created_at |
| 14 | **NotificationWebhook** | id, app_id, url, events (JSONB array), is_active, secret, created_at |

---

## 4. Arquitectura

### Vista General

> **API-first:** La API es el producto principal. El frontend es un cliente externo independiente que consume la API exclusivamente via REST + WebSocket.

```
┌──────────────────────┐      ┌──────────────────────────────────────────┐
│  dev-forge-ui        │      │  dev-forge API (Go modular monolith)     │
│  (React SPA)         │◄────►│                                          │
│  repo separado       │ REST │  ┌─────────┬──────────┬──────────┐      │
└──────────────────────┘  +   │  │   app   │  build   │ release  │      │
                          WS   │  ├─────────┼──────────┼──────────┤      │
┌──────────────────────┐      │  │ deploy  │  scope   │  param   │      │
│  Otros clientes      │      │  ├─────────┼──────────┼──────────┤      │
│  (CLI, curl, scripts)│◄────►│  │ service │ observe  │ approval │      │
└──────────────────────┘      │  ├─────────┼──────────┼──────────┤      │
                              │  │  auth   │   git    │ template │      │
                              │  └─────────┴──────────┴──────────┘      │
                              │                                          │
                              │  Shared: DB, Logger, Config, Telemetry   │
                              └──────┬────────┬────────┬────────────────┘
                                     │        │        │
                              ┌──────▼──┐ ┌───▼───┐ ┌─▼──────────┐
                              │PostgreSQL│ │ Redis │ │ Homelab K8s│
                              └─────────┘ └───────┘ │ Docker     │
                                                     │ GitHub API │
                                                     └────────────┘
                              Observability Stack (en el cluster):
                              OTEL Collector → Jaeger (traces)
                              Prometheus → Grafana (metrics)
                              Loki → Grafana (logs)
```

### Patrón Hexagonal por Módulo

```
internal/<module>/
├── domain/          # Entidades, Value Objects, Domain Services, Errors
│   ├── entity.go
│   └── errors.go
├── ports/           # Interfaces (inbound = use cases, outbound = repos/external)
│   ├── inbound.go
│   └── outbound.go
├── adapters/
│   ├── handler/     # HTTP handlers (inbound adapter)
│   ├── repository/  # PostgreSQL implementation (outbound adapter)
│   └── client/      # External API clients (outbound adapter)
└── service/         # Use case implementations
    └── service.go
```

### Comunicación entre Módulos

**Llamadas directas por interfaces (ports).** Sin event bus, sin REST ni gRPC entre módulos.

```go
// Ejemplo: deploy/service/service.go
type DeployService struct {
    approvalChecker approval.PolicyChecker  // port interface del módulo approval
    scopeProvider   scope.ScopeProvider     // port interface del módulo scope
    k8sDeployer     deploy.K8sDeployer      // port interface outbound
}
```

| Módulo | Depende de (via interfaces) | Para qué |
|---|---|---|
| deploy | approval, scope, release | Verificar políticas, obtener config de scope, obtener release |
| app | template | Aplicar defaults al crear desde plantilla |
| build | app, git | Obtener repo URL, clonar código |
| scope | param | Propagar parámetros al crear scope |
| observe | scope | Identificar pods por scope |

### Modelo de Auth: Zitadel (app) + K8s RBAC (cluster)

Dos capas independientes:

#### Zitadel — RBAC de aplicación

```
Usuario (browser) → Zitadel Login UI → JWT con roles → dev-forge API → middleware valida JWT
```

- **Mecanismo:** OAuth2/OIDC → JWT RS256, JWKS en `{issuer}/oauth/v2/keys`
- **Roles en JWT:** claim `urn:zitadel:iam:org:project:roles` → admin / developer / viewer
- **DB:** Zitadel usa su propia DB `zitadel` en el mismo PostgreSQL. No se toca desde dev-forge.
- **Variables de entorno:** `ZITADEL_ISSUER`, `ZITADEL_CLIENT_ID`

#### K8s RBAC — Permisos del proceso dev-forge

```
dev-forge API (client-go) → K8s API Server → RBAC check → permite/deniega
```

- **Mecanismo:** ServiceAccount + token montado en el pod
- **YAML en Helm chart:** `deployments/helm/dev-forge/templates/rbac.yaml`

```yaml
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["services", "configmaps", "secrets"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
```

**Zitadel decide:** ¿este usuario puede pedir un deploy desde la UI?  
**K8s RBAC decide:** ¿el proceso de dev-forge puede crear un pod en el cluster?

### Abstracción de K8s — Las apps no tienen manifests

El repositorio de la app gestionada **solo contiene código fuente**. No tiene Dockerfile, manifests K8s ni pipeline CI.

| Recurso | De dónde sale | Cómo se aplica |
|---|---|---|
| **Dockerfile** | `ProjectTemplate.dockerfile_template` (DB) | dev-forge lo inyecta al hacer Docker build |
| **K8s Deployment** | Generado en memoria como objeto Go | `client-go` → K8s API Server |
| **K8s Service** | Generado en memoria | `client-go` → K8s API Server |
| **K8s Namespace** | Generado desde Scope config | `client-go` → K8s API Server |
| **ConfigMap / Secret** | Generado desde Parameters | `client-go` → K8s API Server |
| **Resource limits** | Scope config (DB) | Inyectado en el Deployment object |
| **OTEL env vars** | Constantes internas | Inyectadas en el Deployment object |
| **Prometheus annotations** | Constantes internas | Inyectadas en el Deployment metadata |

Los manifests se construyen como **objetos Go tipados** (`appsv1.Deployment`, etc.), no como templates YAML:

```go
// internal/deploy/adapters/client/k8s.go
func (c *K8sClient) ApplyDeployment(ctx context.Context, opts DeployOpts) error {
    deployment := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      opts.AppName,
            Namespace: opts.Namespace,
            Annotations: map[string]string{
                "prometheus.io/scrape": "true",
                "prometheus.io/port":   strconv.Itoa(opts.Port),
            },
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &opts.Replicas,
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{{
                        Name:      opts.AppName,
                        Image:     opts.DockerImage,
                        Resources: buildResources(opts.CpuMillicores, opts.RamMB),
                        Env:       opts.EnvVars,
                        EnvFrom:   opts.SecretRefs,
                    }},
                },
            },
        },
    }
    _, err := c.clientset.AppsV1().Deployments(opts.Namespace).
        Apply(ctx, deployment, metav1.ApplyOptions{FieldManager: "dev-forge"})
    return err
}
```

### Arquitectura de Observabilidad en K8s

Tres capas:

**Capa 1 — Instrumentación en código (por app):** OTEL SDK → push al Collector via env var `OTEL_EXPORTER_OTLP_ENDPOINT`.

**Capa 2 — Infraestructura centralizada (namespace `observability`):**
```
OTEL Collector → Jaeger (traces)
             → Prometheus (metrics via remote write)
Promtail (DaemonSet) → Loki (logs)
Grafana ← Jaeger + Prometheus + Loki
```

**Capa 3 — Recolección automática K8s:** Promtail recoge logs de stdout sin instrumentación. Prometheus scrape kubelet/cAdvisor para CPU/mem.

dev-forge opera en dos roles: como **app instrumentada** (emite su propia telemetría) y como **gestor** (inyecta annotations de Prometheus y `OTEL_EXPORTER_OTLP_ENDPOINT` en los Deployments que genera).

---

## 5. Estructura del Proyecto

```
go-projects/dev_forge/
├── dev-forge-api/             # repo Git — API backend (Go)
├── dev-forge-ui/              # repo Git — frontend React
├── dev-forge.code-workspace   # VS Code multi-root workspace
└── base/                      # screenshots de referencia (Nully)
```

```
dev-forge-api/
├── cmd/server/main.go
├── internal/
│   ├── shared/
│   │   ├── config/        # Config struct + os.Getenv helpers
│   │   ├── database/      # pgxpool + otelpgx
│   │   ├── logger/        # Zap JSON/console
│   │   ├── server/        # Fiber + middleware base
│   │   ├── middleware/    # Auth JWT, RequireRole, audit
│   │   ├── telemetry/     # OTEL SDK setup
│   │   └── crypto/        # AES-256 para secrets
│   ├── auth/              # Zitadel JWT + user sync
│   ├── app/               # Application CRUD
│   ├── template/          # ProjectTemplate CRUD
│   ├── build/             # Docker build pipeline
│   ├── release/           # Release management
│   ├── deploy/            # K8s deployment orchestration
│   ├── scope/             # Scope + K8s namespace
│   ├── param/             # Parameters + K8s ConfigMap/Secret
│   ├── service/           # External services (postgres, redis)
│   ├── observe/           # Logs + metrics proxy
│   ├── approval/          # Approval policies + requests
│   ├── audit/             # Audit log middleware + handlers
│   ├── notification/      # Webhook dispatch
│   ├── git/               # GitHub API
│   └── cluster/           # Cluster registry (admin)
├── migrations/            # .up.sql / .down.sql (golang-migrate)
├── deployments/
│   ├── docker-compose.yml # PostgreSQL + Redis + Zitadel (local)
│   ├── helm/dev-forge/    # Helm chart para deploy en K8s
│   ├── zitadel/           # Manifests K8s para Zitadel self-hosted
│   └── observability/     # Manifests: OTEL Collector, Jaeger, Prometheus, Grafana, Loki
├── docs/
│   ├── DESIGN.md          # Este archivo
│   ├── db-schema.md       # Schema SQL de referencia
│   └── openapi.yaml       # OpenAPI spec (auto-generada por swag)
├── scripts/               # seed, demo
├── Makefile
└── DEV-FORGE-PLAN.md      # Tracking de tareas y estado de fases
```

---

## 6. API REST

### Auth
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/auth/me` | Perfil del usuario autenticado (sync en DB al primer request) |
| GET | `/api/v1/auth/callback` | Zitadel OIDC callback |

> Login, registro, MFA gestionados por Zitadel. El frontend redirige a Zitadel Login UI y recibe el JWT.

### Project Templates
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/templates` | Listar plantillas (filtros: language, framework) |
| GET | `/api/v1/templates/:id` | Detalle de plantilla |
| POST | `/api/v1/templates` | Crear plantilla (solo admin) |
| PUT | `/api/v1/templates/:id` | Editar plantilla (solo admin) |
| DELETE | `/api/v1/templates/:id` | Desactivar plantilla (solo admin) |

### Applications
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps` | Listar aplicaciones |
| POST | `/api/v1/apps` | Crear aplicación (opcionalmente desde template_id) |
| GET | `/api/v1/apps/:id` | Detalle |
| PUT | `/api/v1/apps/:id` | Actualizar |
| DELETE | `/api/v1/apps/:id` | Eliminar |
| GET | `/api/v1/apps/:id/dashboard` | Dashboard agregado |

### Builds
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/builds` | Listar builds |
| POST | `/api/v1/apps/:appId/builds` | Trigger build |
| GET | `/api/v1/apps/:appId/builds/:id` | Detalle |

### Releases
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/releases` | Listar releases |
| POST | `/api/v1/apps/:appId/releases` | Crear release desde build |
| GET | `/api/v1/apps/:appId/releases/:id` | Detalle |

### Scopes
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/scopes` | Listar scopes |
| POST | `/api/v1/apps/:appId/scopes` | Crear scope |
| GET | `/api/v1/apps/:appId/scopes/:id` | Detalle |
| PUT | `/api/v1/apps/:appId/scopes/:id` | Editar |
| DELETE | `/api/v1/apps/:appId/scopes/:id` | Eliminar |

### Deployments
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/deployments` | Listar deployments |
| POST | `/api/v1/apps/:appId/deployments` | Crear deployment |
| GET | `/api/v1/apps/:appId/deployments/:id` | Detalle con steps |
| POST | `/api/v1/apps/:appId/deployments/:id/rollback` | Rollback |
| POST | `/api/v1/apps/:appId/deployments/:id/finalize` | Finalizar |

### Parameters
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/parameters` | Listar (filtros: environment, scope) |
| POST | `/api/v1/apps/:appId/parameters` | Crear |
| PUT | `/api/v1/apps/:appId/parameters/:id` | Editar |
| DELETE | `/api/v1/apps/:appId/parameters/:id` | Eliminar |

### Services
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/services` | Listar (tabs: in_use, available, owned) |
| POST | `/api/v1/apps/:appId/services` | Crear |
| GET | `/api/v1/apps/:appId/services/:id` | Detalle |
| POST | `/api/v1/apps/:appId/services/:id/links` | Crear link a scope |
| DELETE | `/api/v1/apps/:appId/services/:id/links/:linkId` | Eliminar link |

### Observe
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/logs` | Logs (query: scope, instance, version, date) |
| WS | `/api/v1/apps/:appId/logs/stream` | Streaming en tiempo real |
| GET | `/api/v1/apps/:appId/metrics` | Métricas (query: scope, period) |

### Approvals
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/approvals` | Listar requests |
| POST | `/api/v1/apps/:appId/approvals/:id/approve` | Aprobar |
| POST | `/api/v1/apps/:appId/approvals/:id/reject` | Rechazar |
| GET | `/api/v1/apps/:appId/approvals/policies` | Listar políticas |
| POST | `/api/v1/apps/:appId/approvals/policies` | Crear política |
| PUT | `/api/v1/apps/:appId/approvals/policies/:id` | Editar política |

### Audit Log
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/audit-logs` | Listar (filtros: user, resource_type, action, date) |
| GET | `/api/v1/apps/:appId/audit-logs` | Por aplicación |

### Notifications
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps/:appId/webhooks` | Listar webhooks |
| POST | `/api/v1/apps/:appId/webhooks` | Crear |
| PUT | `/api/v1/apps/:appId/webhooks/:id` | Editar |
| DELETE | `/api/v1/apps/:appId/webhooks/:id` | Eliminar |
| POST | `/api/v1/apps/:appId/webhooks/:id/test` | Test |

### GitHub
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/github/repos` | Listar repos del usuario |
| GET | `/api/v1/github/repos/:owner/:repo/branches` | Listar branches |
| POST | `/api/v1/webhooks/github` | Webhook receiver (push → trigger build) |

### Admin (solo rol `admin`)
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/admin/clusters` | Listar clusters |
| POST | `/api/v1/admin/clusters` | Registrar cluster |
| GET | `/api/v1/admin/clusters/:id` | Detalle |
| PUT | `/api/v1/admin/clusters/:id` | Editar |
| DELETE | `/api/v1/admin/clusters/:id` | Desregistrar |
| GET | `/api/v1/admin/users` | Listar usuarios |
| GET | `/api/v1/admin/settings` | Configuración de plataforma |

### OpenAPI
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/docs` | Swagger UI |
| GET | `/api/v1/docs/openapi.yaml` | OpenAPI spec |

---

## 7. Integraciones Reales vs Simuladas

| Feature | Real | Simulado | Notas |
|---|---|---|---|
| Project Templates | ✅ | — | Templates en DB + Dockerfile templates |
| GitHub repos/branches/webhooks | ✅ | — | GitHub API v3 |
| Docker image build | ✅ | — | Docker SDK for Go |
| Deploy a K8s | ✅ | — | client-go, objetos Go en memoria |
| Distributed tracing | ✅ | — | OTEL SDK → Jaeger |
| Metrics & dashboards | ✅ | — | OTEL → Prometheus → Grafana |
| Centralized logs | ✅ | — | Structured logging → Loki → Grafana |
| Pod logs streaming | ✅ | — | client-go + WebSocket |
| Audit log | ✅ | — | Middleware captura todas las acciones |
| Webhook notifications | ✅ | — | HMAC signed |
| OpenAPI documentation | ✅ | — | swag → Swagger UI |
| Self-deploy (Helm) | ✅ | — | dev-forge corre en el cluster que gestiona |
| DB provisioning (Services) | Parcial | ✅ | Docker para local, mock para "cloud" |
| Approval workflow | ✅ | — | Lógica real completa |
| Encryption de secrets | ✅ | — | AES-256 en DB |

---

## 8. Decisiones Técnicas

| Decisión | Valor | Razón |
|---|---|---|
| Lenguaje backend | Go | Estándar en platform engineering |
| Patrón backend | Monolito modular | Demuestra diseño sin complejidad de microservicios |
| Arquitectura | Hexagonal | Separación limpia dominio / puertos / adaptadores |
| Frontend | React SPA (`dev-forge-ui`, repo separado) | Consume API via REST + WebSocket; desacoplado |
| Base de datos | PostgreSQL | Relacional, UUID, JSONB, robusto |
| K8s | Homelab cluster real | Dogfooding, experiencia con K8s de producción |
| Git provider | GitHub | API madura, webhooks |
| Tenancy | Single-tenant + RBAC (3 roles) | Suficiente para demostrar governance |
| Auth | Zitadel self-hosted (OAuth2/OIDC) | JWT RS256, gestiona login/MFA/password reset |
| RBAC aplicación | Zitadel project roles | Roles en claim JWT; sin auth propio |
| RBAC cluster | K8s ServiceAccount + ClusterRole | Principio menor privilegio para el proceso |
| Comunicación módulos | Interfaces directas (ports) | Hexagonal puro; sin event bus |
| Observabilidad | OTEL + Jaeger + Prometheus + Grafana + Loki | Stack CNCF estándar, self-hosted |
| API docs | OpenAPI/Swagger (swag) | Auto-generada desde annotations |
| Audit trail | Tabla `audit_logs` + middleware | Trazabilidad completa |
| Secrets en DB | AES-256 (`shared/crypto`) | Sin Vault para simplificar |
| K8s abstraction | Objetos Go en memoria (no YAML) | Apps gestionadas sin manifests; client-go puro |
| Dockerfile source | `ProjectTemplate.dockerfile_template` (DB) | App repo solo tiene código fuente |
| Clusters | Entidad `Cluster` en DB (token encriptado) | Admin registra via API; `Scope.cluster_id` FK |
| Router HTTP | Fiber v2 | Alto rendimiento, API limpia |
| SQL | pgx v5 + pgxpool | Sin ORM, pool nativo, soporte otelpgx |
| Migraciones | golang-migrate | CLI externa, archivos `.up.sql` / `.down.sql` |
| Logger | Zap + otelzap | Structured, bridge OTEL |
| Config | `os.Getenv` explícito | Sin dependencias externas |
| Validación | go-playground/validator/v10 | Annotations en DTOs |
| Swagger | swaggo/swag | Annotations en handlers → spec + UI |
| Testing | stdlib + testify + testcontainers-go | Tests de integración con DB real |
| Frontend state | Por decidir (Zustand / TanStack Query) | — |
| Frontend UI | Por decidir (shadcn/ui / MUI) | — |
| Frontend charts | Por decidir (Recharts / Chart.js) | — |

---

## 9. Stack Tecnológico

### Backend
- **Lenguaje:** Go 1.26
- **Router:** Fiber v2 (`github.com/gofiber/fiber/v2`)
- **SQL:** pgx v5 + pgxpool (`github.com/jackc/pgx/v5`)
- **Migraciones:** golang-migrate
- **Logger:** Zap (`go.uber.org/zap`) + otelzap
- **Config:** struct `Config` con helpers `os.Getenv`
- **Validación:** go-playground/validator/v10
- **Auth:** Zitadel SDK (`github.com/zitadel/zitadel-go/v3`)
- **Docker:** Docker SDK (`github.com/docker/docker`)
- **K8s:** client-go (`k8s.io/client-go`)
- **Telemetry:** OTEL SDK (`go.opentelemetry.io/otel`)
- **API docs:** swag (`github.com/swaggo/swag`)
- **Testing:** stdlib + testify + testcontainers-go

### Frontend (`dev-forge-ui`)
- React 18 + Vite, Zitadel OIDC (PKCE)
- State / UI / Charts: por decidir
- Contract: OpenAPI spec generada por el backend

### Infraestructura
- PostgreSQL 16, Redis 7, Homelab K8s, Helm, Docker Compose, GitHub Actions

### Observability Stack
- Traces: OTEL Collector → Jaeger
- Metrics: OTEL Collector → Prometheus → Grafana
- Logs: Promtail → Loki → Grafana

---

## 10. Estrategia de Testing

### Capas

| Capa | Herramientas | Scope | Cuándo |
|---|---|---|---|
| **Unit** | stdlib `testing`, mocks hand-rolled | `domain/`, `service/`, `middleware/` | En cada PR, sin infra externa |
| **Integration** | testify + testcontainers-go | `adapters/repository/` | CI pipeline (DB real en Docker) |
| **E2E** | curl / httptest | HTTP handlers completos | Antes de release |

### Qué se testea en unit

- **`domain/`**: lógica pura — métodos de entidad, validaciones, errores de dominio
- **`service/`**: casos de uso con `RepositoryMock` hand-rolled en `_test.go` local
- **`shared/middleware/`**: handlers Fiber con `fiber.App.Test()` + mock de `ports.AuthService`

### Qué NO se testea en unit (requiere integración)

| Código | Razón | Cobertura alternativa |
|---|---|---|
| `adapters/repository/` | Requiere PostgreSQL real | Integration tests (testcontainers-go) |
| `auth/service.New`, `ValidateToken`, `GetMe` | Requiere Zitadel vivo (rs.NewResourceServerFromKeyFile) | E2E con servidor real |
| `cmd/server/` | Wire-up — lógica mínima | Smoke test en CI |

### Objetivo de cobertura

| Package | Target | Estado actual |
|---|---|---|
| `auth/domain` | ≥ 70% | 100% ✅ |
| `auth/service` | ≥ 70% unitesteable¹ | 67% (ValidateToken/GetMe excluidos) |
| `template/service` | ≥ 70% | 86% ✅ |
| `shared/middleware` | ≥ 70% | 100% ✅ |

> ¹ `auth/service` excluye `New`, `ValidateToken` y `GetMe` del cómputo unitario — estos requieren conexión viva a Zitadel y se cubren en tests de integración/E2E.

### Patrones establecidos

- Mocks inline en `*_test.go`, **mismo package** (`package service`) — sin frameworks de generación
- `zap.NewNop()` para logger en todos los tests
- `context.Background()` como contexto base
- Un archivo `_test.go` por package: `domain/X_test.go`, `service/X_test.go`, `middleware/auth_test.go`
- Errores de dominio propagados con `errors.Is()` — los tests verifican el tipo exacto
