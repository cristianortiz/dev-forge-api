# Análisis y plan de migración a Fiber v3

**Fecha de análisis:** 2026-10-18  
**Alcance:** análisis de la actualización y seguimiento de las fases de migración. La CLI ya se ejecutó; el estado actual y las verificaciones realizadas se registran a continuación.

### Estado de ejecución

Actualizado el 2026-10-18, después de ejecutar la CLI:

- La CLI migró el proyecto a Fiber `v3.6.1`; se sustituyó el adaptador externo de Fiber v2 para Swagger por `fiber/v3/middleware/adaptor`.
- Se ejecutaron `go mod tidy` y `go mod vendor`; no quedan imports de Fiber v2 en código de la aplicación ni dependencias de Fiber v2 en `go.mod`.
- La CLI cambió `BodyParser` por `c.Bind().Body`; el middleware del proyecto sigue aplicando `go-playground/validator` explícitamente y conserva las respuestas `400`/`422`.
- Pasaron `go test ./...`, `make test`, `go vet ./...`, `go mod verify` y `go build ./cmd/server`. Se añadió una prueba de la ruta de Swagger con el adaptador v3.
- `make lint` no se pudo completar: la versión instalada de `golangci-lint` fue compilada con soporte hasta Go 1.24 y no admite el `go 1.26.2` objetivo del módulo. La migración queda funcional; queda pendiente disponer de una versión del linter compatible y volver a ejecutar ese gate.

## Resumen ejecutivo

La actualización es viable y el requisito de Go ya está cubierto: el proyecto declara Go `1.26.2`, mientras que la documentación vigente de Fiber v3 requiere Go `1.26` o posterior. Fiber ofrece la CLI `fiber migrate` para actualizar las dependencias e incorporar transformaciones de código; conviene usarla en una rama dedicada y revisar cada cambio, no tratarla como sustituto de pruebas y revisión manual.

En el estado actual —auth con Zitadel, templates y creación de aplicaciones en curso; builds, deploy y observabilidad aún pendientes— se recomienda:

1. **Migrar primero sin rediseñar el servidor.** Mantener el comportamiento de rutas, errores, auth, CORS, logging y validación.
2. **Resolver como parte de la migración la integración Swagger.** Hoy `cmd/server/routes.go` usa `github.com/gofiber/adaptor/v2`; reemplazarla por el adaptador oficial incluido en Fiber v3 (`github.com/gofiber/fiber/v3/middleware/adaptor`) o registrar directamente el handler `net/http`, y comprobar `/api/v1/docs`.
3. **Revisar explícitamente el significado de `c.Context()`.** Hay llamadas desde auth y handlers hacia servicios con contexto; en v3 `c.Context()` devuelve el contexto configurado con `SetContext` o, si no hay uno, `context.Background()`. No asumir que cancela automáticamente al desconectarse el cliente o al apagarse el servidor.
4. **No añadir middlewares por el mero hecho de actualizar.** Los middlewares base actuales cubren necesidades concretas. La CLI ya migró el parseo a `Bind`; se mantiene la validación explícita existente. Evaluar `StructValidator` solo en una tarea posterior y si se conserva el contrato actual de errores `400`/`422`.

## Estado base antes de la migración

| Área | Situación actual | Implicación para v3 |
|---|---|---|
| Go y Fiber | `go.mod` declara Go `1.26.2` y `github.com/gofiber/fiber/v2 v2.52.13`. | El mínimo de Go publicado para Fiber v3 ya se satisface. Confirmar la versión v3 elegida antes de modificar el módulo. |
| Servidor | `internal/shared/server/server.go` registra `requestid`, `recover`, `cors` y un logger propio con Zap. | Migrar firmas y configuración; mantener esos comportamientos. No sustituir el logger Zap sin un motivo funcional. |
| Errores | Hay un `ErrorHandler` propio que serializa JSON y distingue `*fiber.Error`. | Probar los errores de binding y validación: los errores del validador enchufable de v3 no se convierten automáticamente en `400`/`422`. |
| Validación | `shared/middleware.ValidateBody[T]()` parsea JSON, valida con `go-playground/validator/v10`, responde `400` ante JSON inválido y `422` ante fallos de validación. | La validación integrada de `Bind` es una opción de simplificación, no una sustitución automática equivalente. |
| Contexto | `c.Context()` se pasa a servicios en middleware de auth y handlers. | Auditar cancelación, deadlines y shutdown; v3 devuelve `context.Background()` por defecto si no se llamó a `SetContext`. |
| Swagger | `cmd/server/routes.go` importa `github.com/gofiber/adaptor/v2` para montar `http-swagger`. | Esa dependencia externa corresponde a Fiber v2. Usar el adaptador de Fiber v3 o el soporte directo para `http.Handler`, y verificar la UI. |
| Fase del producto | El plan prioriza cerrar el recorrido de aplicaciones; builds/deploy/observabilidad están pendientes. | Mantener la migración acotada y evitar introducir complejidad de plataforma antes de que exista un caso de uso. |

## Cambios de v3 que afectan a este repositorio

- **Contexto y handlers:** las funciones Fiber pasan de recibir `*fiber.Ctx` a `fiber.Ctx` (la interfaz nueva de v3). La CLI puede transformar usos comunes, pero hay que compilar y revisar handlers, middleware, tests y cualquier tipo propio que dependa de la implementación concreta.
- **Contexto Go:** en v3 `c.Context()` es el contexto configurado mediante `c.SetContext`; sin configuración devuelve `context.Background()`. No es equivalente a confiar en un contexto cancelable de la petición. Si las llamadas a Zitadel/PostgreSQL deben detenerse por cancelación o timeout, definir explícitamente cómo se crea, propaga y cancela ese contexto y probarlo.
- **Binding y validación:** `c.Bind()` concentra el binding de body, query, headers y otras fuentes. `fiber.Config.StructValidator` permite conectar un validador como `go-playground/validator/v10`, llamado al bindear structs. Los errores del validador se propagan como errores ordinarios y el error handler por defecto los convierte en `500`; el handler actual del proyecto tampoco reconoce automáticamente errores del validador. Antes de adoptar `Bind`, mapear de forma explícita errores de validación a `422` (y fallos de parseo a `400`) sin ocultar errores internos.
- **Configuración y servidor:** revisar cambios de tipos/firma de `fiber.Config`, `Listen` y `Test` que señale la CLI o el compilador. En v3 hay propiedades de configuración de escucha que se movieron; no se observan configuraciones de prefork/listener personalizadas en el servidor actual.
- **Rutas no coincidentes:** `SkipUnmatchedRoutes` es opt-in. Cambia si los middleware globales se ejecutan para rutas sin coincidencia; esto puede afectar logging y respuestas 404/405. Mantenerlo deshabilitado en esta migración salvo que se diseñe y pruebe ese cambio de comportamiento.
- **Rutas y proxy:** v3 normaliza rutas antes de compararlas. Añadir pruebas para rutas codificadas y no coincidentes si el API se expone tras un ingress. Si se configura IP del cliente detrás de proxy, revisar la nueva configuración `TrustProxy`/`ProxyHeader` y confiar solo en proxies conocidos; no activar confianza indiscriminada.
- **Buffers:** v3 cambia los tamaños predeterminados de buffers de lectura/escritura a 8192 bytes (antes 4096). No parece requerir ajuste para este API, pero queda como cambio de comportamiento a registrar.

## Middlewares y validación: qué incorporar

### Mantener durante la migración

- **`requestid`:** ya se usa y es útil para correlacionar los logs y errores del API.
- **`recover`:** conservar la recuperación de panics y la opción de stack trace ligada a `cfg.Debug`; verificar la configuración equivalente tras la conversión.
- **CORS:** conservar inicialmente la política y validar preflight y métodos permitidos. Revisar el origen `*` como endurecimiento separado cuando se defina el origen de producción de `dev-forge-ui`; no mezclar ese cambio de política con la actualización mayor.
- **Logger Zap propio:** mantenerlo porque integra Zap y registra campos propios (`request_id`, IP y latencia). Comprobar que sigue registrando respuestas 404/405 y errores tras migrar.

### Evaluar después de estabilizar v3

**`StructValidator`** es la posible mejora posterior: podría consolidar la validación en la configuración de Fiber y eliminar parte del middleware genérico `ValidateBody[T]`. La migración ya usa `c.Bind().Body` para el parseo, pero la validación sigue siendo explícita con `shared/validator.Struct()`. No añadiría reglas nuevas ni cambiaría tags: `go-playground/validator/v10` ya está instalado y el proyecto ya tiene una capa de validación reutilizable.

Adoptar esa consolidación solo en una tarea posterior, con una decisión clara sobre:

1. Mantener el formato de errores consumido por los clientes.
2. Mantener `400` para JSON mal formado y `422` para datos inválidos.
3. Traducir únicamente errores de validación conocidos; errores inesperados deben seguir siendo errores internos.
4. Evitar validar dos veces el mismo DTO y conservar los tests de handlers y middleware.

El middleware oficial `healthcheck` puede ser útil más adelante para probes Kubernetes, especialmente si `/ready` comprueba dependencias reales. Hoy `/health` y `/ready` siempre responden éxito, así que instalar el middleware sin definir qué significa readiness no mejora la señal operativa. Añadir rate limiting, Helmet u otros middlewares solo cuando exista un endpoint expuesto/caso de abuso y una política concreta; no son requisitos de v3.

## Procedimiento recomendado

### 1. Preparar una migración aislada

1. Crear una rama dedicada y asegurarse de que los cambios previos están guardados.
2. Ejecutar `git status --short` y conservar una referencia del estado actual.
3. Registrar el baseline: ejecutar `go test ./...`, `make test`, `make build` y `make lint` si las herramientas están disponibles. Anotar fallos preexistentes antes de atribuir regresiones.
4. Confirmar el toolchain con `go version`; el proyecto ya declara `go 1.26.2`.

### 2. Instalar y ejecutar la CLI oficial

La CLI requiere Go `1.25+`, compatible con el toolchain del proyecto:

```sh
go install github.com/gofiber/cli/fiber@latest
fiber version
fiber migrate --help
```

`fiber migrate` usa por defecto la última versión publicada de Fiber. Para una migración reproducible, primero elegir y registrar una versión v3 concreta y fijarla con `--to`:

```sh
fiber migrate --to <version-v3-elegida> --verbose
```

Por ejemplo, reemplazar `<version-v3-elegida>` por el tag exacto seleccionado tras revisar las releases oficiales. La CLI puede actualizar `go.mod`, ejecutar `go mod tidy`/`download`/`vendor` y transformar el código; no pasar `--skip_go_mod` salvo que haya una razón concreta para omitir esos pasos.

### 3. Revisar y completar la migración

1. Inspeccionar el diff de `go.mod`, `go.sum` y fuentes; confirmar que Fiber importa `github.com/gofiber/fiber/v3` y que no queda código productivo en v2.
2. Migrar `github.com/gofiber/adaptor/v2` a `github.com/gofiber/fiber/v3/middleware/adaptor` o registrar el `http.Handler` directamente. Comprobar que `/api/v1/docs/index.html` y sus recursos Swagger funcionan.
3. Revisar en particular `c.Context()` en auth y handlers; decidir explícitamente si el contexto de servicios requiere `SetContext`, timeout o cancelación.
4. Revisar el handler de errores y `ValidateBody[T]`; no cambiar a `Bind` hasta poder conservar los códigos y el formato actuales.
5. Mantener `SkipUnmatchedRoutes` desactivado y no cambiar CORS, logging o trusted proxies en el mismo cambio.
6. Revisar y adaptar anotaciones o ejemplos generados de Swagger si la herramienta los afecta.

### 4. Verificar comportamiento y calidad

Ejecutar como mínimo:

```sh
gofmt -w .
go test ./...
make test
make build
make lint
```

Además, verificar manualmente o con pruebas de integración:

- `/health` y `/ready` conservan sus respuestas.
- Rutas públicas y protegidas conservan status codes; auth, `RequireRole` y `request_id` siguen funcionando.
- JSON inválido devuelve `400`; DTO inválido devuelve `422`; error interno continúa como `500`.
- CORS preflight, headers permitidos y el logger para rutas existentes y 404/405 se comportan como antes.
- `/api/v1/docs/index.html` carga y el adaptador no rompe el handler HTTP de Swagger.
- Las pruebas cubren el contexto que se pasa a Zitadel/repositorios, además de rutas codificadas/no coincidentes cuando aplique.

No ejecutar migraciones de base de datos: esta actualización del framework HTTP no requiere cambios de schema.

### 5. Cerrar la actualización

Fijar la versión aprobada de Fiber en `go.mod`/`go.sum`, confirmar que CI y el entorno de desarrollo usan Go compatible, actualizar las referencias internas que aún indiquen Fiber v2 y documentar cualquier cambio de comportamiento. La adopción de `Bind` y cualquier endurecimiento de CORS/proxy deben quedar en cambios separados con sus propios criterios y pruebas.

## Fuentes oficiales

- [Fiber: What's New in v3](https://docs.gofiber.io/whats_new/)
- [Fiber: guía de validación](https://docs.gofiber.io/guide/validation/)
- [Fiber: API de Bind](https://docs.gofiber.io/api/bind/)
- [Fiber: API de Ctx](https://docs.gofiber.io/api/ctx/)
- [Fiber: middleware adaptor](https://docs.gofiber.io/middleware/adaptor/)
- [Fiber: middleware healthcheck](https://docs.gofiber.io/middleware/healthcheck/)
- [Fiber CLI: instalación y comandos](https://github.com/gofiber/cli)
- [Fiber CLI: guía de migración](https://github.com/gofiber/cli/blob/master/docs/guide/migrate.md)

## Referencias del proyecto

- [Diseño y arquitectura](./DESIGN.md)
- [Plan de implementación](../DEV-FORGE-PLAN.md)
- Implementación de servidor: `internal/shared/server/`
- Middleware de validación: `internal/shared/middleware/validation.go`
- Composición de rutas y Swagger: `cmd/server/routes.go`
