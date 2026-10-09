# DEV-FORGE — Diseño y Arquitectura

> Documento de referencia estático. Contiene la visión, el modelo de dominio, la arquitectura, las decisiones técnicas y el stack. **No** contiene estado de tareas ni progreso — eso vive en `DEV-FORGE-PLAN.md`.

**Última revisión:** 2026-10-09

---

## Índice

1. [Visión del Proyecto](#1-visión-del-proyecto)
2. [Análisis de Nully — Referencia](#2-análisis-de-nully--referencia)
3. [Modelo de Dominio](#3-modelo-de-dominio)
4. [Arquitectura](#4-arquitectura)
5. [Estructura del Proyecto](#5-estructura-del-proyecto)
6. [API REST](#6-api-rest)
7. [Estado de Capacidades](#7-estado-de-capacidades)
8. [Decisiones Técnicas](#8-decisiones-técnicas)
9. [Stack Tecnológico](#9-stack-tecnológico)
10. [Project Blueprints — Scaffolds Curados y Versionados](#10-project-blueprints--scaffolds-curados-y-versionados)
11. [Estrategia de Testing](#11-estrategia-de-testing)
12. [Decisiones Pendientes](#12-decisiones-pendientes)

---

## 1. Visión del Proyecto

**dev-forge** es un prototipo semi-funcional de Internal Developer Platform (IDP) diseñado como proyecto de portafolio para roles de Platform Engineer. Está inspirado en el flujo de trabajo de la plataforma Nully.

**Objetivo:** Demostrar competencias de Platform Engineering con una plataforma API-first que genera, construye y despliega APIs Go estandarizadas en Kubernetes. El backend es un monolito modular con límites hexagonales; el frontend React es un cliente separado. El recorrido end-to-end y las capacidades diferidas se delimitan en `DEV-FORGE-PLAN.md`.

**Alcance objetivo:** un recorrido self-service sobre un cluster de homelab. No asumir que una integración está implementada por aparecer en la arquitectura: la sección 7 distingue el código disponible de las capacidades planeadas y `DEV-FORGE-PLAN.md` mantiene el estado verificable. El despliegue de dev-forge en el cluster es una meta posterior de dogfooding.

**Diseño:** API-first — la API es el producto. El frontend `dev-forge-ui` es un cliente más de la API, en su propio repositorio.

**Tenancy:** Single-tenant con RBAC básico (admin, developer, viewer).

**Git Provider:** GitHub.

---

## 2. Análisis de Nully — Referencia

De las 20 capturas analizadas, Nully expone estos conceptos:

| Concepto Nully | Mapping en dev-forge | Simplificaciones |
|---|---|---|
| New Application (templates, config presets) | Application + blueprint versionado | MVP: un blueprint curado de API Go con monolito modular. No se combinan libremente arquitectura, runtime y capacidades. |
| Application | Application | Igual |
| Dashboard | Application Dashboard | Igual |
| Builds | Builds | Igual |
| Releases | Releases | Igual |
| Deployments (traffic switch, rollback, steps) | Deployments | Traffic switching simplificado (0/100) |
| Scopes (Environment × Country × Cloud) | Scopes | **Sin Country/Cloud** — solo Environment + Cluster |
| Parameters (dimensional scoping, secrets) | Parameters | Scoping por environment y scope, sin cloud dimension |
| Services (mongo-atlas, redis, postgres, links) | Service links | MVP: enlazar PostgreSQL ya existente; no aprovisionar servicios arbitrarios vía Docker |
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
                    │- slug/version  │
                    │- language      │
                    │- framework     │
                    │- source_repo   │
                    │- source_commit │
                    │- defaults      │
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
| 2 | **Application** | id, name, description, repo_url, template_id (requerido en el MVP), provisioning_status, owner_id, created_at |
| 2b | **ProjectTemplate** | id, name, slug, version, description, language, framework, source_repository, source_commit, default_params (JSONB), default_scope_config (JSONB), is_active, created_at. Cada fila identifica una versión publicada e inmutable del blueprint. |
| 2c | **Cluster** | id, name, slug, api_endpoint, token_encrypted, ca_cert, status (active/inactive), created_at |
| 3 | **Build** | id, app_id, description, branch, commit_sha, status (pending/building/successful/failed), docker_image, coverage, created_at |
| 4 | **Release** | id, app_id, version (semver), build_id, status (active/superseded), created_at |
| 5 | **Scope** | id, app_id, name, environment (dev/staging/prod), cluster_id (FK → Cluster), namespace, ram_mb, cpu_millicores, replicas, visibility (internal/public), health_check_path, status (running/stopped), created_at |
| 6 | **Deployment** | id, release_id, scope_id, status (pending/provisioning/switching_traffic/finalized/rolled_back), created_by, created_at, updated_at |
| 7 | **DeploymentEvent** | id, deployment_id, step, status, message, created_at |
| 8 | **Parameter** | id, app_id, name, value_encrypted, is_secret, environment, scope_id (nullable), delivered_as (env_var/file), created_at |
| 9 | **Service** | id, app_id, type (postgres inicialmente), name, provider, status (active/cancelled), created_at |
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
                            ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
                            │ PostgreSQL   │ │ GitHub API   │ │ Homelab K8s  │ │ Build runner │
                            │ control DB   │ │ repos        │ │ API / RBAC   │ │ aislado      │
                            └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
                              Observability Stack (objetivo, en el cluster):
                              Apps/API → OTLP → Collector Contrib → Tempo → Grafana (traces)
                              Apps/API /metrics ────────────────> Prometheus → Grafana (metrics)
                              Kubernetes stdout → Collector Contrib → Loki → Grafana (logs)
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
└── usecase/         # Use case implementations (Clean Architecture naming)
    └── usecase.go
```

### Comunicación entre Módulos

**Llamadas directas por interfaces (ports).** Sin event bus, sin REST ni gRPC entre módulos.

```go
// Ejemplo: deploy/usecase/deploy_usecase.go
type DeployUseCase struct {
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

### Abstracción de K8s — Las apps no tienen manifests ni CD propio

El repositorio generado contiene el código, Dockerfile y CI de validación que pertenecen a la aplicación. No contiene manifests Kubernetes ni un workflow de CD: dev-forge conserva el control del build y del despliegue. El proceso API no construye código ni monta el socket Docker del nodo; los builds se ejecutan en un runner aislado y efímero, con permisos, recursos, tiempo y red limitados. La tecnología concreta del runner queda pendiente en la sección 12.

| Recurso | De dónde sale | Cómo se aplica |
|---|---|---|
| **Dockerfile** | Repo versionado del blueprint | Forma parte del repo generado y se construye desde el commit exacto de la app en un runner aislado. No se guarda ni se inyecta desde PostgreSQL. |
| **CI de aplicación** | Repo generado | Tests, lint y verificaciones de dependencias en PR; no publica ni despliega imágenes. |
| **Build runner** | Infraestructura de dev-forge | Ejecuta builds aislados; nunca comparte el Docker socket del API ni recibe credenciales más amplias que las necesarias para publicar la imagen. |
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

La arquitectura separa **señales**, **recolección**, **almacenamiento** y **consulta**. Son responsabilidades distintas; Grafana no almacena telemetría y OpenTelemetry no es un backend.

| Concepto | Responsabilidad en dev-forge |
|---|---|
| Instrumentación | SDKs OpenTelemetry generan spans y métricas; las métricas se exponen en `/metrics`. Zap escribe logs JSON a stdout. |
| Collector | OpenTelemetry Collector Contrib como DaemonSet recibe traces OTLP, añade contexto de Kubernetes, recoge stdout/stderr y exporta traces/logs. |
| Backend de traces | Tempo almacena y consulta traces. Recibe OTLP del Collector. |
| Backend de métricas | Prometheus scrapea `/metrics` de las apps y métricas nativas del cluster. No hay un salto intermedio por el Collector. |
| Backend de logs | Loki almacena logs estructurados exportados por el Collector. El logger de cada app añade IDs de trace/span cuando hay span activo; el Collector no infiere esa relación. Los IDs son campos, no labels indexados. |
| Interfaz | Grafana consulta Tempo, Prometheus y Loki; contiene dashboards, Explore y enlaces log ↔ trace. |

```text
API y apps instrumentadas ── OTLP traces ────────> OpenTelemetry Collector Contrib ──> Tempo ──┐
Kubernetes stdout/stderr ── filelog receiver ────> OpenTelemetry Collector Contrib ──> Loki ────┤
API y apps instrumentadas ── /metrics ──────────> Prometheus scrape ────────────────────────────┤──> Grafana
Kubernetes / exporters ───── /metrics ──────────> Prometheus scrape ────────────────────────────┘
```

#### Dos objetivos de observabilidad

1. **Control plane:** observar la propia API dev-forge, PostgreSQL y los procesos de build/deploy. Las trazas deben conectar HTTP → caso de uso → dependencias instrumentadas, sin crear spans redundantes en cada llamada interna.
2. **Workloads administrados:** observar las aplicaciones desplegadas por dev-forge. Los templates definen cómo emitir OTLP y exponer métricas; la recolección de logs de contenedor no requiere modificar la aplicación.

El módulo `observe` es una **capacidad de producto**, no otro backend: autoriza consultas a Loki/Prometheus para una app y scope. Los endpoints de logs consultan Loki (incluido el tail en vivo); no consultan Kubernetes como una fuente paralela. Las métricas consultan Prometheus. Esto mantiene una fuente de verdad por señal y permite aplicar autorización antes de devolver datos.

#### Decisiones operativas y límites

- **Una ruta por señal:** OTLP para traces; scrape Prometheus para métricas; stdout JSON para logs. No duplicar logs vía Zap→OTLP y stdout→Collector. No desplegar Jaeger, Tempo, Promtail y Alloy simultáneamente.
- **OpenTelemetry como contrato, no como almacenamiento:** `OTEL_EXPORTER_OTLP_ENDPOINT` apunta al Collector, no a Tempo. Cambiar el backend no requiere cambiar los SDKs de las apps.
- **Tolerancia a fallos:** exportación asíncrona, batch, timeout y colas acotadas. Si el Collector o un backend no está disponible, las requests de negocio, `/health` y `/ready` siguen funcionando; telemetría puede perderse cuando se agote la cola. La API no debe bloquear esperando al backend.
- **Cardinalidad y privacidad:** usar nombres de ruta normalizados y atributos de recurso estables (`service.name`, versión, entorno, namespace). No usar IDs de usuario, app, request, trace ni valores arbitrarios como labels de métricas/Loki. No exportar tokens, headers de autorización, cuerpos de request ni secretos; revisar/redactar atributos antes de exportar.
- **Perfil inicial del homelab:** una instancia por backend, recursos y retención limitados, PVCs y backups verificados. Es persistencia de nodo único, no HA ni un SLA de producción: una caída del nodo interrumpe consulta/ingesta y la pérdida del volumen pierde datos posteriores al último backup.
- **Perfil durable de producción:** Tempo y Loki usan almacenamiento de objetos compatible y respaldado; Prometheus tiene retención, backup y recuperación definidos. Se habilitan autenticación/TLS, RBAC, límites de recursos y alertas sobre API/pipeline. Esto mejora durabilidad pero no garantiza HA.
- **Alta disponibilidad (opcional y posterior):** réplicas, almacenamiento compartido, distribución de Collector y pruebas de fallo/recuperación. No llamarlo HA hasta verificar el comportamiento ante pérdida de nodo y backend. Retención y sampling son configurables por entorno, nunca valores embebidos en el código.

dev-forge opera en dos roles: como **app instrumentada** (emite telemetría estándar) y como **gestor** (inyecta en los Deployments endpoint OTLP, atributos de entorno y configuración de scrape/logs). La disponibilidad de observabilidad no forma parte de la disponibilidad de serving de la API.

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
│   ├── service/           # Links a servicios externos; MVP: PostgreSQL existente
│   ├── observe/           # Logs + metrics proxy
│   ├── approval/          # Approval policies + requests
│   ├── audit/             # Audit log middleware + handlers
│   ├── notification/      # Webhook dispatch
│   ├── git/               # GitHub API
│   └── cluster/           # Cluster registry (admin)
├── migrations/            # .up.sql / .down.sql (golang-migrate)
├── deployments/
│   ├── docker-compose.yml # PostgreSQL local; Zitadel Cloud externo
│   ├── helm/dev-forge/    # Helm chart para deploy en K8s
│   ├── zitadel/           # Manifests K8s para Zitadel self-hosted
│   └── observability/     # Helm/manifests: Collector Contrib, Tempo, Prometheus, Grafana, Loki
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

Las rutas siguientes describen el contrato objetivo del producto; no todas están implementadas ni cableadas todavía. El estado ejecutable a la fecha está resumido en la sección 7 y el avance por tarea en `DEV-FORGE-PLAN.md`.

### Auth
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/auth/me` | Perfil del usuario autenticado (sync en DB al primer request) |
| GET | `/api/v1/auth/callback` | Zitadel OIDC callback |

> Login, registro, MFA gestionados por Zitadel. El frontend redirige a Zitadel Login UI y recibe el JWT.

### Project Templates
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/templates` | Listar blueprints/versiones publicadas que admiten nuevas apps |
| GET | `/api/v1/templates/:id` | Detalle de una versión inmutable del blueprint |
| POST | `/api/v1/templates` | Registrar una versión ya validada (solo admin/automatización de publicación) |
| PATCH | `/api/v1/templates/:id` | Activar/desactivar una versión; no mutar su repo, commit ni contrato |
| DELETE | `/api/v1/templates/:id` | Retirar del catálogo (soft delete); nunca borrar una versión publicada |

### Applications
| Method | Endpoint | Descripción |
|---|---|---|
| GET | `/api/v1/apps` | Listar aplicaciones |
| POST | `/api/v1/apps` | Crear aplicación desde versión activa del blueprint (`template_id` obligatorio en el MVP) |
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

## 7. Estado de Capacidades

El estado describe el código del repositorio a **2026-10-09**. Una fila en la arquitectura o el schema objetivo no implica que exista una integración ejecutable.

| Capacidad | Estado actual | Alcance / siguiente límite |
|---|---|---|---|
| Auth y sincronización de usuario | Implementado | Zitadel introspection; revisar sincronización/revocación de roles antes de ampliar RBAC |
| Catálogo de templates | Parcial, modelo legacy | CRUD y seed existen; no representan todavía versiones publicadas e inmutables |
| Generación de repos desde blueprint | Pendiente | GitHub API y provisioning idempotente |
| GitHub repos, branches y webhooks | Pendiente/no cableado | Integración objetivo de Fase 1 |
| Build de imágenes | Pendiente | Requiere runner aislado; no ejecutar Docker SDK desde el proceso API |
| Scopes y despliegue Kubernetes | Pendiente | client-go y objetos tipados son el diseño objetivo |
| Trazas, métricas y logs centralizados | Pendiente | Objetivo de Fase 4: OTLP/Tempo, scrape Prometheus, stdout/Loki |
| Logs de pods, approvals, audit y notifications | Pendiente | Módulos no cableados; implementar solo en el alcance definido por fase |
| OpenAPI/Swagger | Disponible para endpoints existentes | Spec actual cubre auth/templates; regenerar al cambiar contratos |
| Servicios y secretos | Pendiente | Definir primero conexión segura a PostgreSQL; no asumir provisioning vía Docker |
| Frontend y self-deploy | Pendiente | Proyectos/fases posteriores; no condicionan el contrato de API |

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
| Auth | Zitadel Cloud (OAuth2/OIDC) | Gestor de login/MFA; la estrategia de roles, revocación y dependencia de introspection queda por cerrar |
| RBAC aplicación | Zitadel project roles | Roles en claim JWT; sin auth propio |
| RBAC cluster | K8s ServiceAccount + ClusterRole | Principio menor privilegio para el proceso |
| Comunicación módulos | Interfaces directas (ports) | Hexagonal puro; sin event bus |
| Observabilidad | OpenTelemetry + Collector Contrib + Tempo + Prometheus + Loki + Grafana | OTLP desacopla instrumentación y backend; un pipeline por señal; perfil inicial de nodo único, no HA |
| API docs | OpenAPI/Swagger (swag) | Auto-generada desde annotations |
| Audit trail | Tabla `audit_logs` + middleware | Trazabilidad completa |
| Secretos | Cifrado en DB solo con clave inyectada externamente; entrega como Secret | No guardar la clave maestra en DB, repo ni imagen; definir rotación/recuperación antes de producción |
| K8s abstraction | Objetos Go en memoria (no YAML) | Apps gestionadas sin manifests; client-go puro |
| Ejecución de builds | Runner efímero y aislado | El API orquesta; no monta Docker socket ni ejecuta Dockerfiles de apps no confiables en su proceso |
| Clusters | Entidad `Cluster` en DB (token encriptado) | Admin registra via API; `Scope.cluster_id` FK |
| Router HTTP | Fiber v2 | Alto rendimiento, API limpia |
| SQL | pgx v5 + pgxpool | Sin ORM, pool nativo, soporte otelpgx |
| Migraciones | golang-migrate | CLI externa, archivos `.up.sql` / `.down.sql` |
| Logger | Zap + otelzap | Structured, bridge OTEL |
| Config | `os.Getenv` explícito | Sin dependencias externas |
| Validación | go-playground/validator/v10 | Tags `validate:` en DTOs de ports; `middleware.ValidateBody[T]()` valida y llama `c.Next()`; `middleware.GetBody[T](c)` recupera el valor en el handler; lógica pura en `shared/validator.Struct()` |
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
- **Builds:** runner aislado (BuildKit rootless o equivalente; elección pendiente)
- **K8s:** client-go (`k8s.io/client-go`)
- **Telemetry:** OTEL SDK (`go.opentelemetry.io/otel`)
- **API docs:** swag (`github.com/swaggo/swag`)
- **Testing:** stdlib + testify + testcontainers-go

### Frontend (`dev-forge-ui`)
- React 18 + Vite, Zitadel OIDC (PKCE)
- State / UI / Charts: por decidir
- Contract: OpenAPI spec generada por el backend

### Infraestructura
- PostgreSQL 16, Homelab K8s, Helm, Docker Compose, GitHub Actions; Redis fuera del MVP hasta justificarlo

### Observability Stack
- Traces: OTEL SDK → OTLP → Collector Contrib → Tempo → Grafana
- Metrics: OTEL MeterProvider con exporter Prometheus → `/metrics` → Prometheus scrape → Grafana; Prometheus también scrapea métricas del cluster
- Logs: Zap JSON enriquecido con `trace_id`/`span_id` desde el contexto → stdout → Collector Contrib (`filelog`) → Loki → Grafana
- Product APIs: `observe/logs` consulta/tail de Loki y `observe/metrics` consulta Prometheus, aplicando autorización por app/scope

---

## 10. Project Blueprints — Scaffolds Curados y Versionados

### Alcance del MVP

En la interfaz se pueden llamar **blueprints**; `ProjectTemplate` se conserva como nombre del modelo y de la API por compatibilidad. Un registro `ProjectTemplate` representa una publicación concreta de un scaffold completo, no un formulario para ensamblar arquitectura, lenguaje, framework y servicios de manera arbitraria.

El primer y único blueprint soportado será **Go API — Monolito Modular** (`slug: go-modular-monolith`). Hexagonal es la organización de sus módulos internos, no una alternativa combinable con “monolito modular”. La selección inicial captura nombre, descripción y visibilidad del repo; `template_id` es obligatorio y no hay toggles de tecnologías. No existe creación vacía en el MVP, porque produciría una app sin fuente que el pipeline no puede construir ni desplegar. Una futura arquitectura se incorpora como otro blueprint curado únicamente cuando exista y pase sus propios gates.

Esta restricción reduce el costo real: cada combinación publicada se convierte en una promesa que dev-forge debe generar, construir, desplegar y mantener. Un catálogo pequeño, reproducible y probado tiene más valor que varias entradas de seed que describen capacidades aún inexistentes.

### Qué contiene el blueprint

El repo versionado contiene los archivos que necesita el equipo para desarrollar la aplicación:

| Área | Contrato del blueprint inicial |
|---|---|
| Estructura | Monolito modular Go con `cmd/server`, composition root, módulos `domain/`, `ports/`, `usecase/` y `adapters/`, más un módulo de ejemplo completo. Sin microservicios ni event bus. |
| API | Fiber, validación de requests, errores consistentes, Swagger y ejemplos de configuración. |
| Persistencia | PostgreSQL/pgx y migraciones. Compose local provee PostgreSQL para desarrollo; Zitadel es externo (Cloud) en la configuración actual. La imagen de la app no empaqueta una DB. La conexión desplegada requiere parámetros secretos y un endpoint PostgreSQL; acordar su alcance mínimo antes del primer recorrido deployable. |
| Runtime | Configuración externa, graceful shutdown, readiness y liveness diferenciadas. Liveness no falla porque una dependencia remota esté temporalmente caída. |
| Contenedor | Dockerfile multi-stage, usuario no root, build reproducible, imágenes base con versiones controladas y sin secretos ni herramientas de build en la imagen final. El gate valida el Dockerfile real. |
| Observabilidad | Logs JSON a stdout, trazas OTEL por OTLP hacia la configuración inyectada por dev-forge y métricas en `/metrics`. Sin URL de backend, credenciales ni dependencia obligatoria de que el stack de observabilidad esté disponible. |
| Seguridad | Auth OIDC/Zitadel disponible en el chassis con health endpoints públicos; configuración mediante variables/secretos externos. Sin credenciales dentro del repo, workflow ni imagen. |
| CI | GitHub Actions ejecuta tests, lint y verificaciones de dependencias en PR. No despliega ni publica automáticamente a un cluster. |
| Entrega | El Dockerfile pertenece al repo; dev-forge construye y publica la imagen y es el único responsable del despliegue. No se generan manifests K8s ni un workflow de CD duplicado. |

“Production-oriented baseline” significa que el scaffold aplica estas prácticas y supera pruebas automatizadas; **no** garantiza por sí solo que una aplicación concreta sea segura, escalable o apta para producción. Eso depende de sus dependencias, configuración, threat model, recursos, políticas y operación del cluster.

### Fuente y ciclo de vida de versiones

El repositorio GitHub contiene la fuente de verdad: código, Dockerfile, workflows, migraciones, ejemplos y tests. PostgreSQL almacena metadata del catálogo, defaults no secretos y la identidad de la versión publicada; nunca almacena código ejecutable del template ni reemplaza archivos durante el build.

Cada versión publicable es inmutable: `slug + version` es único, `source_repository` identifica un repo snapshot de esa versión y `source_commit` guarda su SHA de procedencia. Para mantener simple el MVP con la API nativa de GitHub Template Repositories, cada publicación apunta a un repo snapshot cuya rama por defecto no se modifica. Un tag dentro de un repo mutable no basta si la API de generación copia siempre la rama por defecto. Publicar cambios crea una versión y un snapshot nuevos; no se edita una versión ya usada. Filas legacy sin fuente pueden conservarse solo inactivas durante la migración.

`Application.template_id` referencia la fila exacta de blueprint usada. Desactivar una versión solo impide crear nuevas apps desde ella; las apps existentes mantienen su procedencia y continúan construyéndose desde sus propios repos. Los defaults JSON solo pueden contener configuración no sensible; secretos se crean y entregan mediante los mecanismos de parámetros/secretos de la plataforma.

### Flujo de creación

```text
POST /api/v1/apps { name, template_id, visibility }  // template_id obligatorio
  → validar que la versión está publicada/activa y que el usuario puede crear el repo
  → reservar Application en estado provisioning con clave de idempotencia
  → generar repo desde el snapshot versionado en GitHub
  → sustituir únicamente tokens explícitos (nombre, módulo Go, service.name); nunca secretos
  → guardar repo_url y transición a ready
  → GitHub Actions valida cambios del desarrollador; dev-forge build/deploy gestiona la entrega
```

La llamada a GitHub y la transacción PostgreSQL no son atómicas. La reserva, una clave de idempotencia y estados `provisioning/ready/failed` permiten reintentar o reconciliar una creación interrumpida sin crear repos duplicados ni ocultar repos huérfanos. La publicación de templates no ocurre en `POST /apps`: la realiza el proceso controlado de release del blueprint.

### Gates para publicar el blueprint

La automatización de publicación debe crear una app de prueba desde el snapshot y comprobar:

- no quedan placeholders sin resolver ni configuración sensible;
- `go test ./...`, lint y análisis de dependencias pasan;
- el Dockerfile construye la imagen y el contenedor inicia como usuario no root;
- `/health` y `/ready` tienen semántica distinta y responden correctamente;
- configuración OTEL ausente o Collector caído no bloquean el serving;
- el workflow generado solo valida código y no despliega;
- la metadata publicada identifica exactamente versión, repo y commit.

Los templates existentes de React, Node.js y Python son prototipos del seed, no blueprints soportados: se retiran del catálogo activo hasta tener repos fuente, propietarios, gates y soporte de build/deploy. No se amplía el catálogo en esta fase.

---

## 11. Estrategia de Testing

### Capas

| Capa | Herramientas | Scope | Cuándo |
|---|---|---|---|
| **Unit** | stdlib `testing`, mocks hand-rolled | `domain/`, `usecase/`, `middleware/` | En cada PR, sin infra externa |
| **Integration** | testify + testcontainers-go | `adapters/repository/` | CI pipeline (DB real en Docker) |
| **E2E** | curl / httptest | HTTP handlers completos | Antes de release |

### Qué se testea en unit

- **`domain/`**: lógica pura — métodos de entidad, validaciones, errores de dominio
- **`usecase/`**: casos de uso con `RepositoryMock` hand-rolled en `_test.go` local
- **`shared/middleware/`**: handlers Fiber con `fiber.App.Test()` + mock de `ports.AuthService`

### Qué NO se testea en unit (requiere integración)

| Código | Razón | Cobertura alternativa |
|---|---|---|
| `adapters/repository/` | Requiere PostgreSQL real | Integration tests (testcontainers-go) |
| `auth/usecase.New`, introspection | Requiere Zitadel/configuración de key real | E2E con servidor real; cubrir sincronización de roles con tests unitarios |
| `cmd/server/` | Wire-up — lógica mínima | Smoke test en CI |

### Objetivo de cobertura

| Package | Target | Estado actual |
|---|---|---|
| `auth/domain` | ≥ 70% | 100% ✅ |
| `auth/usecase` | ≥ 70% unitesteable¹ | 67% (introspection excluida) |
| `template/usecase` | ≥ 70% | 86% ✅ |
| `shared/middleware` | ≥ 70% | 100% ✅ |

> ¹ `auth/usecase` excluye la inicialización del resource server y la introspection del cómputo unitario — requieren configuración/servicio Zitadel y se cubren en integración/E2E.

### Patrones establecidos

- Mocks inline en `*_test.go`, **mismo package** (`package usecase`) — sin frameworks de generación
- `zap.NewNop()` para logger en todos los tests
- `context.Background()` como contexto base
- Un archivo `_test.go` por package: `domain/X_test.go`, `usecase/X_test.go`, `middleware/auth_test.go`
- Errores de dominio propagados con `errors.Is()` — los tests verifican el tipo exacto

---

## 12. Decisiones Pendientes

Estas preguntas son límites de diseño, no permisos implícitos para ampliar el MVP. Resolverlas antes de implementar los módulos que dependen de ellas y registrar la respuesta aquí.

| # | Pregunta | Recomendación de partida | Bloquea |
|---|---|---|---|
| 1 | ¿Qué ejecuta los builds y con qué aislamiento? | Runner efímero con BuildKit rootless o equivalente; sin Docker socket en el API, credenciales acotadas, límites de recursos/tiempo y red restringida. | Fase 2 |
| 2 | ¿Los repos se crean en una organización GitHub o bajo cuentas de usuarios, y qué credencial los autoriza? | GitHub App instalada solo en la organización destino, con permisos mínimos y propietario explícito. | Fase 1 |
| 3 | ¿Zitadel es fuente de verdad de roles o los roles locales son administrables? ¿Cómo se revoca un rol en la siguiente request? | No confiar en una copia persistida que no se refresca; definir sincronización/revocación y cubrir descenso de privilegios. | Fase 1 |
| 4 | ¿El blueprint inicial exige PostgreSQL para arrancar, o el primer deploy puede funcionar sin DB? | Mantener la dependencia opcional en el ejemplo o adelantar parámetros secretos y enlace PostgreSQL al primer recorrido deployable. | Fases 2–3 |
| 5 | ¿dev-forge aprovisiona PostgreSQL o solo enlaza servicios ya existentes? | MVP: enlazar endpoint externo y entregar credenciales como secretos; no administrar bases con Docker desde el API. | Fase 3 |
| 6 | ¿Cómo se protegen, rotan y recuperan la clave de cifrado y las credenciales de plataforma? | Secretos inyectados fuera de Git/DB, rotación documentada y permisos distintos para API, runner y workloads. | Fases 1–3 |
| 7 | ¿Se requiere Redis en el MVP y en qué caso de uso concreto? | No incluirlo en Compose/Helm hasta que un flujo lo necesite. | Fase 0 / 6 |
| 8 | ¿Dónde corre la PostgreSQL del control plane y quién prueba su backup/restore? | DB externa como opción preferida; subchart solo para homelab. Separar este ciclo de vida del PostgreSQL que usan las apps generadas. | Fase 6 |

La guía de Zitadel también debe decidir si los service users sin email son válidos: el schema actual de `users` exige email no vacío, por lo que el test PAT necesita un claim compatible o un contrato explícito para identidades de servicio.
