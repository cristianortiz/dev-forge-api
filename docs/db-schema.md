# DEV-FORGE — Schema de Base de Datos

> Referencia del schema PostgreSQL. La fuente de verdad real son los archivos en `migrations/`. Este documento sirve para consulta rápida sin ejecutar queries.

**Última revisión:** 2026-10-09
**Base de datos:** `dev_forge` (PostgreSQL 16)  
**Nota:** Zitadel usa su propia DB `zitadel` en el mismo PostgreSQL — esas tablas son gestionadas automáticamente por Zitadel y no se tocan desde dev-forge.

> **Estado del diseño de blueprints:** `project_templates` y `applications` describen aquí el schema objetivo del MVP versionado. Las migraciones actuales todavía reflejan el modelo anterior (Dockerfile en DB y sin provisioning de repo); actualizarlo es tarea 1.10 de `DEV-FORGE-PLAN.md`. No asumir que esta definición ya está aplicada.

---

## Diagrama de Relaciones

```
users ──────────────────────────────────────────────────────────────────┐
  │                                                                      │
  │ owner_id                                                             │ created_by
  ▼                                                                      │
applications ──────────────────────────────────────────────────────┐    │
  │ template_id                                                     │    │
  │    ▼                                                            │    │
  │  project_templates                                              │    │
  │                                                                 │    │
  ├──▶ builds                                                       │    │
  │      └──▶ releases ─────────────────────────────────────────┐  │    │
  │                                                              │  │    │
  ├──▶ scopes ──────────────────────────────┐    ┌──────────────┘  │    │
  │      │ cluster_id                       │    │ release_id      │    │
  │      ▼                                  │    │ scope_id        │    │
  │    clusters                             └────┴──▶ deployments ─┘────┘
  │                                                    └──▶ deployment_events
  ├──▶ parameters (scope_id nullable)
  │
  ├──▶ services
  │      └──▶ service_links (scope_id)
  │
  ├──▶ approval_policies
  │      └──▶ approval_requests
  │
  ├──▶ audit_logs
  │
  └──▶ notification_webhooks
```

---

## Tablas

### `users`
```sql
CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zitadel_id   VARCHAR(255) UNIQUE NOT NULL,  -- sub claim del JWT
    email        VARCHAR(255) UNIQUE NOT NULL,
    name         VARCHAR(255) NOT NULL,
    role         VARCHAR(20) NOT NULL DEFAULT 'developer', -- admin | developer | viewer
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `project_templates`
```sql
CREATE TABLE project_templates (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(255) NOT NULL,         -- "Go API — Modular Monolith"
    slug                 VARCHAR(100) NOT NULL,         -- identidad estable: "go-modular-monolith"
    version              VARCHAR(50) NOT NULL,          -- versión publicada del scaffold: "v1.0.0"
    description          TEXT NOT NULL DEFAULT '',
    language             VARCHAR(50) NOT NULL,          -- "go"
    framework            VARCHAR(100) NOT NULL,         -- "fiber"
    source_repository    VARCHAR(500),                  -- snapshot GitHub Template Repository de esta versión
    source_commit        CHAR(40),                      -- SHA del contenido publicado
    default_params       JSONB NOT NULL DEFAULT '{}',   -- solo configuración no sensible
    default_scope_config JSONB NOT NULL DEFAULT '{}',   -- recursos/probes sugeridos; ajustables por scope
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (slug, version),
    CHECK (NOT is_active OR (source_repository IS NOT NULL AND source_commit IS NOT NULL))
);
```

Cada versión publicable es inmutable. `is_active = FALSE` la oculta para nuevas apps, pero no debe borrarse ni cambiar su repo/commit si alguna `application` la referencia. Las columnas de fuente solo pueden ser nulas en filas históricas inactivas; el endpoint de catálogo nunca devuelve una versión sin fuente verificable. `default_params` no admite secretos ni credenciales.

### `clusters`
```sql
CREATE TABLE clusters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    slug            VARCHAR(100) UNIQUE NOT NULL,
    api_endpoint    VARCHAR(500) NOT NULL,    -- "https://192.168.1.10:6443"
    token_encrypted TEXT NOT NULL,           -- ServiceAccount token (AES-256)
    ca_cert         TEXT,                    -- CA cert del cluster (base64)
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active | inactive
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `applications`
```sql
CREATE TABLE applications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               VARCHAR(255) UNIQUE NOT NULL,
    description        TEXT,
    repo_url           VARCHAR(500),
    template_id        UUID NOT NULL REFERENCES project_templates(id), -- versión exacta; obligatoria en el MVP
    owner_id           UUID NOT NULL REFERENCES users(id),
    provisioning_status VARCHAR(20) NOT NULL DEFAULT 'provisioning', -- provisioning | ready | failed
    idempotency_key    UUID UNIQUE,                            -- evita duplicar repos al reintentar POST /apps
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (provisioning_status <> 'ready' OR repo_url IS NOT NULL)
);
```

Para crear desde blueprint, reservar la app con estado `provisioning` y una clave de idempotencia antes de llamar a GitHub. Marcar `ready` solo cuando el repo exista y se haya guardado `repo_url`. Como PostgreSQL y GitHub no comparten transacción, los reintentos deben reconciliar el repo determinístico existente; no guardar tokens de GitHub ni errores remotos sensibles en esta tabla. El MVP no admite apps sin blueprint.

### Evolución desde la migración actual

La migración de `project_templates` debe agregar `version`, `source_repository` y `source_commit`, reemplazar la unicidad global de `slug` por `(slug, version)` y eliminar `dockerfile_template` cuando ningún build lo consuma. Las filas históricas del seed que no tengan repo/commit real reciben una versión legacy y se conservan inactivas, nunca seleccionables. El nuevo seed publica una sola versión validada del blueprint Go monolito modular. Añadir `provisioning_status` e `idempotency_key` antes de cablear la generación de repos en `POST /apps`.

### `builds`
```sql
CREATE TABLE builds (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id       UUID NOT NULL REFERENCES applications(id),
    description  TEXT,
    branch       VARCHAR(255) NOT NULL,
    commit_sha   VARCHAR(40) NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- pending | building | successful | failed
    docker_image VARCHAR(500),
    coverage     DECIMAL(5,2),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `releases`
```sql
CREATE TABLE releases (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id     UUID NOT NULL REFERENCES applications(id),
    version    VARCHAR(50) NOT NULL,   -- semver: "v1.0.0"
    build_id   UUID NOT NULL REFERENCES builds(id),
    status     VARCHAR(20) NOT NULL DEFAULT 'active', -- active | superseded
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app_id, version)
);
```

### `scopes`
```sql
CREATE TABLE scopes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id            UUID NOT NULL REFERENCES applications(id),
    name              VARCHAR(255) NOT NULL,
    environment       VARCHAR(20) NOT NULL,  -- dev | staging | prod
    cluster_id        UUID NOT NULL REFERENCES clusters(id),
    namespace         VARCHAR(100) NOT NULL,
    ram_mb            INT NOT NULL DEFAULT 128,
    cpu_millicores    INT NOT NULL DEFAULT 0, -- 0 = sin límite
    replicas          INT NOT NULL DEFAULT 1,
    visibility        VARCHAR(20) NOT NULL DEFAULT 'internal', -- internal | public
    health_check_path VARCHAR(255) DEFAULT '/health',
    status            VARCHAR(20) NOT NULL DEFAULT 'running', -- running | stopped
    current_release_id UUID REFERENCES releases(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `deployments`
```sql
CREATE TABLE deployments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id UUID NOT NULL REFERENCES releases(id),
    scope_id   UUID NOT NULL REFERENCES scopes(id),
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- pending | awaiting_approval | provisioning | switching_traffic
    -- | finalized | rolled_back | cancelled
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `deployment_events`
```sql
CREATE TABLE deployment_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployments(id),
    step          VARCHAR(50) NOT NULL,
    status        VARCHAR(20) NOT NULL, -- started | completed | failed
    message       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `parameters`
```sql
CREATE TABLE parameters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id          UUID NOT NULL REFERENCES applications(id),
    name            VARCHAR(255) NOT NULL,
    value_encrypted TEXT NOT NULL,        -- siempre encriptado (AES-256)
    is_secret       BOOLEAN NOT NULL DEFAULT FALSE,
    environment     VARCHAR(20),          -- NULL = todos los environments
    scope_id        UUID REFERENCES scopes(id), -- NULL = todos los scopes del environment
    delivered_as    VARCHAR(20) NOT NULL DEFAULT 'env_var', -- env_var | file
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `services`
```sql
CREATE TABLE services (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id     UUID NOT NULL REFERENCES applications(id),
    type       VARCHAR(50) NOT NULL,   -- postgres en el MVP; otros tipos requieren provider y gates propios
    name       VARCHAR(255) NOT NULL,
    provider   VARCHAR(100),
    status     VARCHAR(20) NOT NULL DEFAULT 'active', -- active | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `service_links`
```sql
CREATE TABLE service_links (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id                  UUID NOT NULL REFERENCES services(id),
    scope_id                    UUID NOT NULL REFERENCES scopes(id),
    status                      VARCHAR(20) NOT NULL DEFAULT 'active', -- active | inactive
    connection_params_encrypted TEXT,   -- AES-256
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(service_id, scope_id)
);
```

### `approval_policies`
```sql
CREATE TABLE approval_policies (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id           UUID NOT NULL REFERENCES applications(id),
    action           VARCHAR(50) NOT NULL,
    -- new_scope | modify_scope | delete_scope | create_service
    -- | new_deployment | stop_scope
    environment      VARCHAR(20), -- NULL = todos
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `approval_requests`
```sql
CREATE TABLE approval_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id     UUID NOT NULL REFERENCES approval_policies(id),
    resource_type VARCHAR(50) NOT NULL,
    resource_id   UUID NOT NULL,
    action        VARCHAR(50) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- pending | approved | rejected | cancelled
    requester_id  UUID NOT NULL REFERENCES users(id),
    approver_id   UUID REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at   TIMESTAMPTZ
);
```

### `audit_logs`
```sql
CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID REFERENCES users(id),
    action        VARCHAR(100) NOT NULL,
    -- app.created | deployment.finalized | parameter.updated | etc.
    resource_type VARCHAR(50) NOT NULL,
    resource_id   UUID NOT NULL,
    details       JSONB DEFAULT '{}',  -- before/after, metadata
    ip_address    VARCHAR(45),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_user     ON audit_logs(user_id, created_at DESC);
```

### `notification_webhooks`
```sql
CREATE TABLE notification_webhooks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id     UUID NOT NULL REFERENCES applications(id),
    url        VARCHAR(500) NOT NULL,
    events     JSONB NOT NULL DEFAULT '["deployment.finalized"]',
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    secret     VARCHAR(255),   -- HMAC secret para firmar el payload
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## Migraciones

Los archivos existentes en `migrations/` usan el formato de golang-migrate:

```
migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_create_project_templates.up.sql       ← modelo legacy
└── 000002_create_project_templates.down.sql
```

La definición de las tablas de este documento es el **schema objetivo**, no el estado aplicado. La siguiente migración de Fase 1 debe evolucionar `project_templates` y crear `applications`; Fase 2 agrega clusters, builds, releases, scopes y deployments; Fase 3 agrega parámetros y enlaces a servicios externos; Fase 5 agrega approvals, audit y webhooks. Nombrar/versionar cada archivo al implementarlo, sin mantener una lista de rutas futuras que pueda confundirse con archivos existentes.

```bash
# Comandos
make migrate-up                        # aplica todas las pendientes
make migrate-down                      # revierte la última
make migrate-create name=add_indexes   # crea nuevo par up/down
```
