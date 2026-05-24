# Zitadel Setup — dev-forge API

> Guía de referencia para configurar autenticación y autorización con Zitadel Cloud.
> Nivel de producción desde el inicio: JWT Profile (sin client secrets viajando por red).

---

## Arquitectura de autenticación

```
                    ┌─────────────────────────────────┐
                    │         Zitadel Cloud            │
                    │  (Identity Provider - IdP)       │
                    └────────────┬────────────────┬────┘
                                 │ JWKS / OIDC    │ Introspection
                                 │ Discovery      │ Endpoint
              ┌──────────────────▼───┐    ┌───────▼──────────────┐
              │   dev-forge-ui       │    │   dev-forge API       │
              │   (React, PKCE flow) │    │   (Go, Fiber)         │
              │   → obtiene JWT      │    │   → valida tokens via │
              │   → lo envía en      │───▶│     introspección     │
              │     Authorization    │    │   → sincroniza users  │
              └──────────────────────┘    └───────────────────────┘
```

**Flujo de producción (Frontend):**
1. Usuario hace login → Zitadel devuelve JWT (PKCE flow)
2. Frontend envía `Authorization: Bearer <JWT>` en cada request
3. API llama a Zitadel `/oauth/v2/introspect` con su propia key privada (JWT Profile)
4. Zitadel confirma si el token es válido y devuelve claims (roles, email, etc.)
5. API sincroniza el usuario en la DB y responde

**Flujo de testing (Service User PAT):**
1. Service user tiene PAT (opaque token)
2. El mismo endpoint de introspección funciona — Zitadel valida el PAT
3. Misma respuesta, mismos claims

---

## Paso 1 — Configurar Zitadel Cloud

### 1.1 Borrar la app antigua

En [https://dev-forge-2hcwhk.us1.zitadel.cloud/ui/console](https://dev-forge-2hcwhk.us1.zitadel.cloud/ui/console):

1. **Projects** → `dev-forge` → app `dev-forge-api`
2. **Actions** → **Delete** → confirmar

### 1.2 Crear nueva API app con JWT Profile

1. **Projects** → `dev-forge` → **+ New Application**
2. Nombre: `dev-forge-api` | Tipo: **API** → Continue
3. Método de autenticación: **JWT** (NO Basic) → Continue → Create
4. Copiar el **Client ID** que aparece (lo necesitas para `.env`)

### 1.3 Generar key.json para la API app

1. En la app `dev-forge-api` recién creada → sección **Keys** → **+ New Key**
2. Tipo: **JSON** | Sin fecha de expiración (o 2 años para prod) → **Add**
3. **Descargar el JSON** → guardar como `secrets/zitadel-api-key.json`
4. Click **Close**

> ⚠️ Este JSON se descarga UNA sola vez. No tiene client secret — usa criptografía
> de clave pública. Es seguro en el servidor, nunca va al cliente.

El archivo tiene esta estructura:
```json
{
  "type": "application",
  "keyId": "<KEY_ID>",
  "key": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n",
  "appId": "<APP_ID>",
  "clientId": "<CLIENT_ID>"
}
```

### 1.4 Configurar roles del proyecto

En **Projects** → `dev-forge` → **Roles** — verificar que existen:

| Key | Display Name | Group |
|-----|-------------|-------|
| `admin` | Administrator | platform |
| `developer` | Developer | platform |
| `viewer` | Viewer | platform |

En **Settings** del proyecto:
- ✅ **Assert Roles on Authentication** → ON
- ✅ **Check authorization on authentication** → ON

### 1.5 Crear service user para testing

1. **Users** → **Service Users** → **+ New**
2. Username: `dev-forge-test` | Access Token Type: **Bearer** → Create
3. Tab **Personal Access Tokens** → **+ New** → sin expiración → copiar PAT
4. Tab **Metadata** (opcional) → ignorar

### 1.6 Asignar rol admin al service user

1. **Projects** → `dev-forge` → **Authorizations** → **+ New**
2. Seleccionar `dev-forge-test` → rol `admin` → Save

### 1.7 Asignar rol admin al usuario humano (cris-admin)

1. **Projects** → `dev-forge` → **Authorizations** → **+ New**
2. Seleccionar `cris-admin` → rol `admin` → Save

---

## Paso 2 — Configurar la API (código)

### 2.1 Variables de entorno

```env
ZITADEL_ISSUER=https://dev-forge-2hcwhk.us1.zitadel.cloud
ZITADEL_CLIENT_ID=<CLIENT_ID_de_la_nueva_app>
ZITADEL_KEY_PATH=secrets/zitadel-api-key.json
```

> El `CLIENT_ID` es solo informativo en config (el key.json ya lo contiene).
> El key.json es la única credencial que necesita la API.

### 2.2 Cómo funciona el código

```
auth_service.go
  └─ New(ctx, repo, issuer, keyPath, logger)
       └─ rs.NewResourceServerFromKeyFile(ctx, issuer, keyPath)
            └─ Lee key.json → crea signer RSA
            └─ Auto-descubre introspection endpoint via OIDC Discovery
  └─ ValidateToken(ctx, rawToken)
       └─ rs.Introspect[*oidc.IntrospectionResponse](ctx, resourceServer, token)
            └─ Genera JWT assertion firmado con private key
            └─ POST /oauth/v2/introspect con client_assertion
            └─ Zitadel valida → devuelve {active, sub, email, roles...}
```

**Por qué JWT Profile y no Basic auth:**
- Basic auth: el client_secret viaja en cada request a Zitadel (riesgo si hay MITM)
- JWT Profile: solo viaja un JWT firmado con clave privada RSA, nunca el secret
- JWT Profile: es el estándar recomendado para backend APIs en producción

---

## Paso 3 — Verificar funcionamiento

### 3.1 Verificar introspección directamente con Zitadel

```bash
# Generar el JWT assertion manualmente (solo para debug)
# O simplemente arrancar la API y hacer curl:
make run
```

### 3.2 Probar el endpoint protegido

```bash
# Con el PAT del service user dev-forge-test:
export TOKEN="<PAT_de_dev-forge-test>"

curl -s http://localhost:9000/api/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" | jq
```

**Respuesta esperada:**
```json
{
  "id": "uuid-generado",
  "email": "",
  "name": "dev-forge-test",
  "role": "admin"
}
```

### 3.3 Verificar sincronización en DB

```bash
make docker-exec-db
# dentro del contenedor:
psql -U postgres -d dev_forge -c "SELECT * FROM users;"
```

### 3.4 Probar que RBAC funciona

```bash
# Un endpoint que requiere rol admin debe devolver 200:
curl -s http://localhost:9000/api/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" | jq

# Sin token debe devolver 401:
curl -s http://localhost:9000/api/v1/auth/me | jq
# {"error":"missing authorization header","request_id":"..."}
```

---

## Paso 4 — Para el frontend (Fase F.2 — futuro)

Cuando se implemente `dev-forge-ui`, el flujo será:

1. Crear nueva app en Zitadel: tipo **SPA** (Single Page Application)
2. Auth method: **PKCE** (no client secret en frontend)
3. Redirect URI: `http://localhost:3000/callback` (dev) / `https://dev-forge.yourdomain.com/callback` (prod)
4. El frontend usará el Zitadel SDK para React o OIDC Client JS
5. Los tokens que obtiene el frontend son JWTs válidos que la API acepta directamente

---

## Archivos relevantes

| Archivo | Propósito |
|---------|-----------|
| `secrets/zitadel-api-key.json` | Private key de la API app — gitignored |
| `secrets/.gitkeep` | Mantiene la carpeta en git |
| `.env` | Variables de entorno locales — gitignored |
| `internal/auth/service/auth_service.go` | Lógica de introspección |
| `internal/shared/middleware/auth.go` | Middleware Fiber (Authenticated, RequireRole) |
| `internal/auth/adapters/handler/auth_handler.go` | `GET /api/v1/auth/me` |
| `internal/auth/adapters/repository/user_repository.go` | Sync user en PostgreSQL |
| `migrations/000001_create_users.up.sql` | Tabla `users` |

---

## Troubleshooting

| Error | Causa | Solución |
|-------|-------|----------|
| `open : no such file or directory` | `ZITADEL_KEY_PATH` vacío o no cargado | Verificar `.env` y que `make run` usa `set -a && . ./.env` |
| `unauthorized_client` | App creada con Basic auth, no JWT Profile | Recrear app con método JWT |
| `{"active": false}` | PAT expirado o service user sin autorización | Nuevo PAT + agregar Authorization en proyecto |
| `invalid or expired token` | Token inválido o key.json del service user (no de la API app) | Usar key.json de la API app, no del service user |
| `initializing resource server` | key.json malformado o issuer incorrecto | Verificar contenido del JSON y `ZITADEL_ISSUER` |
