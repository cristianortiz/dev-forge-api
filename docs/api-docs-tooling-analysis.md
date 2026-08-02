# Análisis: Herramienta de documentación de API

## Estado actual

- `swaggo/swag` genera OpenAPI 2.0 (Swagger) desde comentarios `@...` en handlers.
- `swaggo/http-swagger` sirve la UI clásica de Swagger en `/api/v1/docs/*`.
- Problema: anotaciones manuales se desincronizan del código (ya desactualizado), UI Swagger es visualmente anticuada, sin soporte nativo OpenAPI 3.1.

## Opciones modernas evaluadas

| Herramienta                          | Motor spec                                                       | Genera desde código Go        | UI                                | Esfuerzo migración                                           | Notas                                                                       |
| ------------------------------------ | ---------------------------------------------------------------- | ----------------------------- | --------------------------------- | ------------------------------------------------------------ | --------------------------------------------------------------------------- |
| **Scalar** (`scalar/scalar`)         | Renderiza spec existente (JSON/YAML)                             | No, solo UI                   | Excelente (moderna, tipo Postman) | **Bajo**                                                     | Solo reemplaza la UI; sigue usando `swag` o cualquier generador de spec     |
| **Redoc**                            | Renderiza spec existente                                         | No                            | Buena, estática                   | Bajo                                                         | Similar a Scalar, menos interactiva (no "try it")                           |
| **swaggo + swagger-ui** (actual)     | swag genera 2.0                                                  | Sí (anotaciones)              | Clásica                           | —                                                            | Baseline                                                                    |
| **go-swagger**                       | Genera 2.0/3.0                                                   | Sí (anotaciones o spec-first) | Config.                           | Medio                                                        | Más pesado, poco mantenido                                                  |
| **huma** (framework)                 | Genera OpenAPI 3.1 automático desde structs Go (sin anotaciones) | Sí, automático                | Integra Scalar/Stoplight          | **Alto** (requiere reescribir handlers sobre huma o wrapper) | Mejor DX a largo plazo, pero acopla el router                               |
| **ogen / oapi-codegen** (spec-first) | Vos escribís YAML/JSON OpenAPI 3.1 a mano, se genera código      | —                             | Cualquiera (Scalar/Redoc)         | Alto                                                         | Cambia el flujo a "spec primero", más disciplina pero cero desincronización |

## Recomendación

**Fase 1 (rápida, bajo riesgo):** mantener `swaggo/swag` como generador (ya funciona, solo hay que regenerar con `make swagger` y revisar anotaciones faltantes en handlers de `app`, `template`, `auth`), y **reemplazar solo la UI** por **Scalar** vía su paquete Go embebido (`github.com/MarceloPetrucio/go-scalar-api-reference` o servir el HTML estático de `@scalar/api-reference` apuntando al `swagger.json` ya generado).

**Fase 2 (opcional, futuro):** si se busca eliminar anotaciones manuales por completo, migrar a **huma** — pero implica reescribir el binding de request/response de cada handler, no es trivial con Fiber (huma tiene adapter para Fiber vía `humafiber`). Evaluar solo si el dolor de mantener anotaciones se vuelve recurrente.

## Estimación de costo (Fase 1: Swagger UI → Scalar)

| Tarea                                                                                                       | Esfuerzo       |
| ----------------------------------------------------------------------------------------------------------- | -------------- |
| Reemplazar dependencia `swaggo/http-swagger` por handler estático que sirva HTML de Scalar + `swagger.json` | ~1h            |
| Actualizar `routes.go` (ruta `/api/v1/docs/*`)                                                              | ~15min         |
| Regenerar spec y corregir anotaciones desactualizadas/faltantes (`app` module sin docs aún)                 | 1-2h           |
| Actualizar `Makefile` (target `swagger`, sin cambios si se mantiene `swag`)                                 | ~10min         |
| Actualizar `DEV-FORGE-PLAN.md` (tarea 1.13) y README si aplica                                              | ~10min         |
| Verificación manual (`make dev` + revisar `/api/v1/docs`)                                                   | ~15min         |
| **Total estimado**                                                                                          | **~3-4 horas** |

No requiere migraciones DB, no rompe contrato de API, cero downtime — solo capa de presentación de docs.

## Plan de pasos detallado (una vez se elija Scalar)

1. **Dependencia:** agregar el paquete de UI de Scalar para Go (o simplemente vendorizar el CDN script `@scalar/api-reference` en un HTML estático servido por Fiber) — decidir: paquete Go vs. HTML embebido con `//go:embed`.
2. **Handler nuevo:** crear `cmd/server/docs_handler.go` (o similar) que sirva:
   - `GET /api/v1/docs` → HTML con `<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference">` apuntando a `data-url="/api/v1/docs/openapi.json"`.
   - `GET /api/v1/docs/openapi.json` → sirve `docs/swagger/swagger.json` generado por `swag`.
3. **routes.go:** reemplazar el bloque de `swaggo/http-swagger` por el nuevo handler; eliminar import `httpswagger` y `adaptor` si ya no se usa para esto.
4. **go.mod:** correr `go mod tidy` para quitar `swaggo/http-swagger` y `swaggo/files` si no se usan más (mantener `swaggo/swag` como generador de spec).
5. **Completar anotaciones faltantes:** revisar módulo `app` (handlers en progreso, tarea 1.7/1.8) y agregar comentarios `@Summary/@Router/@Param/@Success` antes de regenerar.
6. **Regenerar spec:** `make swagger`.
7. **Probar localmente:** `make dev` → abrir `/api/v1/docs` → validar que carga UI Scalar, "try it" funciona con Bearer token.
8. **Actualizar docs internas:** `DEV-FORGE-PLAN.md` tarea 1.13 (nota de cambio de herramienta) y `docs/DESIGN.md` si menciona Swagger UI explícitamente.
9. **Commit + pre-commit hook:** verificar que `make test`/`build` siguen pasando (hook ya configurado).
10. **(Opcional futuro)** evaluar `huma` si el mantenimiento de anotaciones sigue siendo fricción — requiere spike aparte, no incluido en este alcance.
