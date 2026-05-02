# DEV-FORGE — Schema de Base de Datos

> Referencia del schema PostgreSQL. La fuente de verdad real son los archivos en `migrations/`. Este documento sirve para consulta rápida sin ejecutar queries.

**Última revisión:** 2026-05-01  
**Base de datos:** `dev_forge` (PostgreSQL 16)  
**Nota:** Zitadel usa su propia DB `zitadel` en el mismo PostgreSQL — esas tablas son gestionadas automáticamente por Zitadel y no se tocan desde dev-forge.

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
    name                 VARCHAR(255) NOT NULL,         -- "Go REST API", "React SPA"
    slug                 VARCHAR(100) UNIQUE NOT NULL,  -- "go-rest-api", "react-spa"
    description          TEXT,
    language             VARCHAR(50) NOT NULL,          -- "go", "javascript", "python"
    framework            VARCHAR(100),                  -- "fiber", "react", "fastapi"
    dockerfile_template  TEXT NOT NULL,                 -- contenido del Dockerfile
    default_params       JSONB DEFAULT '{}',            -- env vars por defecto
    default_scope_config JSONB DEFAULT '{}',            -- RAM, CPU, replicas, health check
    repo_template_url    VARCHAR(500),                  -- GitHub template repo (opcional)
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

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
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    repo_url    VARCHAR(500),
    template_id UUID REFERENCES project_templates(id), -- NULL si no usa template
    owner_id    UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

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
    type       VARCHAR(50) NOT NULL,   -- postgres | redis | mongo
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

Los archivos viven en `migrations/` con el formato de golang-migrate:

```
migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_create_templates_and_clusters.up.sql
├── 000002_create_templates_and_clusters.down.sql
├── 000003_create_apps_builds_releases.up.sql    ← Fase 1 (tarea 1.10)
├── 000003_create_apps_builds_releases.down.sql
├── 000004_create_scopes_deployments.up.sql      ← Fase 2 (tarea 2.10)
├── 000004_create_scopes_deployments.down.sql
├── 000005_create_parameters_services.up.sql     ← Fase 3 (tarea 3.6)
├── 000005_create_parameters_services.down.sql
├── 000006_create_approval_audit_webhooks.up.sql ← Fase 5 (tarea 5.8)
└── 000006_create_approval_audit_webhooks.down.sql
```

```bash
# Comandos
make migrate-up                        # aplica todas las pendientes
make migrate-down                      # revierte la última
make migrate-create name=add_indexes   # crea nuevo par up/down
```
