# PR: Migrar la API a Fiber v3

## Resumen

Actualiza el servidor de Fiber v2 a Fiber v3.6.1 y adapta las integraciones y handlers manteniendo el comportamiento actual de la API.

## Cambios

- Migra handlers y middlewares a las firmas y APIs de Fiber v3.
- Sustituye el adaptador externo de Fiber v2 por `fiber/v3/middleware/adaptor` para servir Swagger.
- Usa `c.Bind().Body` para parsear solicitudes y conserva la validación existente y sus respuestas `400`/`422`.
- Actualiza dependencias y referencias de documentación; elimina la dependencia de Fiber v2.
- Añade una prueba que verifica que Swagger se sirve correctamente mediante el adaptador v3.

## Verificación

- `go test ./...`
- `make test`
- `go vet ./...`
- `go mod verify`
- `go build ./cmd/server`

`make lint` queda pendiente: la versión disponible de `golangci-lint` no admite el Go `1.26.2` requerido por el módulo.
