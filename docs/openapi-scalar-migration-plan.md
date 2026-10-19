# Plan: contrato OpenAPI 3 como fuente de verdad y Scalar como UI

**Fecha:** 2026-10-18  
**Estado:** implementación y validación automatizada completas; pendiente únicamente el smoke test manual de “Try it” con el entorno local.
**Objetivo:** reemplazar la generación de Swagger 2.0 desde comentarios Go por un contrato OpenAPI 3 explícito en YAML; usar Scalar para documentar y probar los endpoints; conservar Fiber v3 y la arquitectura hexagonal existente.

## 1. Decisión recomendada

Adoptar un flujo **contract-first**:

1. Un archivo YAML versionado es la fuente canónica del contrato HTTP.
2. Scalar renderiza el contrato canónico y habilita “Try it”.
3. Los handlers siguen implementados con Fiber; no se migra a Huma ni se reemplaza el router.
4. Los DTOs HTTP actuales permanecen manuales: la última versión de `oapi-codegen` examinada no declara soporte para OAS 3.2. Los handlers continúan mapeando esos DTOs a los inputs/outputs de `ports`.
5. `make openapi-check` valida el YAML y comprueba que se sirve el mismo contrato embebido; los tests existentes siguen cubriendo el comportamiento HTTP. No se genera código desde la spec en esta fase.

Scalar es la **presentación** de la documentación, no el generador ni la fuente de verdad. El cambio solicitado no es solo cambiar Swagger UI por Scalar: también retira `swaggo/swag` v1 y las anotaciones de endpoints de los handlers.

## 2. OpenAPI desde cero: conceptos necesarios para este plan

### Qué es OpenAPI

OpenAPI (OAS, OpenAPI Specification) es un estándar independiente de lenguaje y framework para describir el contrato HTTP de una API. Declara qué puede pedir un cliente y qué puede recibir como respuesta, sin describir cómo está programado el servidor.

Un documento OpenAPI puede servir como:

- Contrato compartido entre quienes implementan el backend y quienes consumen la API.
- Fuente para generar documentación interactiva, clientes, modelos y mocks.
- Entrada para validadores y pruebas automatizadas de requests y responses.
- Referencia versionada para revisar cambios incompatibles antes de publicarlos.

OpenAPI no es un router, una librería de validación ni una UI. Agregar un YAML no valida por sí solo los requests que recibe Fiber ni garantiza que el código responda como el documento declara: eso requiere generación, validación en runtime o pruebas de conformidad.

### OpenAPI, Swagger, Scalar y YAML

- **OpenAPI** es el estándar del contrato.
- **Swagger 2.0** es la versión anterior de la especificación (también conocida como OpenAPI 2.0). OpenAPI 3 es su evolución, con una estructura más completa para cuerpos, respuestas, componentes y servidores.
- **Scalar** es una UI que lee un documento OpenAPI y presenta sus operaciones; no escribe ni mantiene el contrato.
- **YAML** es solo el formato legible usado para serializar el documento. El mismo contrato también puede expresarse como JSON.

El antiguo `swagger.yaml` pertenecía a la familia histórica de OpenAPI, pero su cabecera `swagger: "2.0"` no lo convertía en OpenAPI 3. Como lo generaba `swaggo` desde anotaciones Go, antes de esta migración el origen de verdad eran los comentarios y no ese archivo.

### Qué describe el contrato

Para cada operación HTTP, OpenAPI describe:

1. **Ruta y método:** por ejemplo `GET /templates/{id}`.
2. **Parámetros:** path (`id`), query (`language`), header o cookie; su tipo y si son obligatorios.
3. **Request body:** media type (normalmente JSON), schema y campos requeridos.
4. **Responses:** status HTTP (`200`, `400`, `401`, `404`, `422`, etc.), media type y forma del body para cada resultado relevante.
5. **Seguridad:** mecanismo de autenticación y operaciones que lo requieren.
6. **Metadata:** `operationId`, tags, resumen, descripción y ejemplos para consumidores y herramientas.

El contrato debe reflejar el comportamiento observable real: no basta con documentar el caso exitoso. Por ejemplo, un `POST /templates` necesita describir el body, el `201` de éxito y los errores de parseo, autorización y validación que realmente produce el API.

### Estructura típica de un documento OpenAPI 3

Este ejemplo es ilustrativo; no es el contrato final de dev-forge:

```yaml
openapi: 3.1.1
info:
  title: Dev Forge API
  version: 1.0.0
servers:
  - url: /api/v1
paths:
  /templates:
    get:
      operationId: listTemplates
      tags: [templates]
      security:
        - BearerAuth: []
      parameters:
        - name: language
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: Templates encontrados
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Template"
        "401":
          $ref: "#/components/responses/Unauthorized"
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
  schemas:
    Template:
      type: object
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
  responses:
    Unauthorized:
      description: Falta un token válido
```

Las piezas principales son:

- **`openapi`:** versión del formato OAS que el documento utiliza.
- **`info`:** identidad y versión del contrato/API.
- **`servers`:** URL base a la que se suman las rutas de `paths`.
- **`paths`:** cada ruta, sus métodos HTTP y operaciones.
- **`components`:** schemas, respuestas, parámetros y esquemas de seguridad reutilizables.
- **`$ref`:** referencia una definición reutilizable dentro del documento; evita copiar el mismo schema en cada operación.

OpenAPI 3 también distingue explícitamente `requestBody` de `parameters` y ubica tipos reutilizables bajo `components.schemas`. Son cambios estructurales frente al Swagger 2.0 generado actualmente.

### Tres versiones distintas que no deben confundirse

| Campo o ruta          | Qué versiona                                                       | Ejemplo de dev-forge                                                        |
| --------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------- |
| `openapi: 3.2.0`      | La versión del estándar/sintaxis usada para escribir el documento. | OAS 3.2.0, seleccionada después del gate de compatibilidad.                |
| `info.version: 1.0.0` | La versión publicada del contrato/API.                             | Se incrementa de acuerdo con la política de cambios compatibles del API.    |
| `servers: /api/v1`    | La base de URL de la API desplegada.                               | Corresponde a las rutas actuales del API; no cambia por usar OAS 3.1 o 3.2. |

Cambiar la primera no obliga a cambiar la tercera. Tampoco es necesario incrementar `/api/v1` solo por trasladar la documentación de Swagger 2.0 a OpenAPI 3: se debe mantener el comportamiento HTTP y versionar el contrato según las reglas de compatibilidad acordadas.

### Especificar un schema no equivale a validar en Fiber

Declarar en YAML que `name` es requerido o que `id` tiene formato UUID documenta esa regla, pero el servidor no la aplica automáticamente. Para mantener alineados documento y runtime se necesita una combinación explícita de:

- Tipos HTTP generados desde el YAML, si las herramientas soportan la versión elegida; en esta migración se conservan los DTOs actuales.
- Validación de requests/responses, si se adopta en runtime.
- Pruebas de contrato que comparen las respuestas reales con los schemas y status codes.
- Validación/lint del documento en CI y generación reproducible.

El dominio y los `ports` no dependen de OpenAPI ni de sus tipos; permanecen en la capa de transporte los DTOs actuales.

### Cómo encajan Scalar y el flujo contract-first

El flujo esperado es:

```text
openapi.yaml (contrato canónico)
        ├──> Scalar (documentación interactiva y Try it)
        ├──> JSON de presentación derivado en memoria para la integración Scalar
        └──> lint / validación / pruebas de contrato

Fiber handlers ── implementan el mismo contrato ──> puertos y casos de uso
```

Scalar consume el YAML para representar operaciones, auth, parámetros y responses. El botón **Authorize** guarda el Bearer token en la UI y **Try it** envía requests contra `servers`; Scalar no obtiene tokens de Zitadel ni sustituye la configuración de autenticación del API.

## 3. Baseline comprobado antes de la migración

- El antiguo `docs/swagger/swagger.yaml` declaraba `swagger: "2.0"` y se generaba desde anotaciones Go.
- `make swagger` y `swaggo/http-swagger` servían el flujo basado en comentarios; se retiraron durante esta ejecución.
- La especificación generada contiene 6 operaciones: `GET /auth/me`; `GET` y `POST /templates`; `GET`, `PATCH` y `DELETE /templates/{id}`. El `basePath` actual es `/api/v1`.
- Además están implementados `GET /health` y `GET /ready`. Se deben incluir en el nuevo contrato como endpoints operacionales públicos, o excluirlos deliberadamente con esa decisión documentada.
- Los handlers y tests existentes ya describen parte del contrato real: DTOs de requests/responses, validaciones, status codes y auth. Se deben tomar como evidencia, no copiar a ciegas las anotaciones actuales, que pueden estar incompletas o desactualizadas.

## 4. Versión de OpenAPI y resultado del gate

La especificación oficial publicada más reciente es OpenAPI **3.2.0**, elegida como solicitó el usuario. El gate ejecutado el 2026-10-18 validó el YAML con Vacuum y la UI/asset offline de Scalar.

### Gate de compatibilidad

Durante la selección de herramientas:

1. Se contrastaron las versiones publicadas: `gofiber-scalar/scalar/v3` **v3.1.14**, Vacuum **v0.33.1** y `oapi-codegen` **v2.8.0**.
2. Vacuum v0.33.1 aceptó `openapi: 3.2.0` con cero errores. El ruleset recomendado informa advertencias de estilo y ejemplos opcionales; no bloquea la validación.
3. `gofiber-scalar` v3.1.14 incluye Scalar API Reference v1.65.1 y el bundle offline; las pruebas verifican que la UI recibe el contrato 3.2 serializado y que el asset local se sirve.
4. `oapi-codegen` v2.8.0 documenta soporte hasta OpenAPI 3.1, por lo que no se usa para no rebajar el contrato; los DTOs actuales se mantienen manuales.
5. Se conserva **3.2.0** porque el validador elegido acepta el documento y no se ha identificado una incompatibilidad bloqueante en las operaciones/schemas utilizados. Si una herramienta esencial falla más adelante, el fallback acordado sigue siendo **3.1.1**.

La integración Scalar renderiza contenido JSON derivado del YAML en memoria porque su API no consume una URL de spec. El documento fuente sigue siendo solo `docs/openapi/openapi.yaml`.

## 5. Estructura objetivo

### Fuente y publicación del contrato

- Fuente única: `docs/openapi/openapi.yaml`.
- Mantener inicialmente un solo archivo legible; usar `$ref` internos para reutilizar schemas y responses. Dividirlo en múltiples archivos solo cuando su tamaño lo justifique.
- Publicarlo en la misma aplicación, por ejemplo en `GET /api/v1/openapi.yaml`, con `Content-Type` YAML correcto.
- Incluir el spec en el binario mediante `//go:embed` desde un paquete ubicado junto al archivo. Así la documentación no depende del directorio de trabajo ni de archivos sueltos en la imagen de ejecución.
- No mantener un JSON o YAML generado como segunda fuente versionada. Si se requiere JSON, derivarlo en build/runtime desde el YAML canónico.

### Scalar

- UI en `GET /api/v1/docs` (y su ruta de assets, si la integración la requiere).
- Configurar Scalar para cargar el YAML publicado por la propia API, en una URL relativa y same-origin; no fijar `localhost`, puerto de desarrollo ni dominio de Zitadel dentro del spec.
- Integración usada: [`gofiber-scalar`](https://github.com/yokeTH/gofiber-scalar) v3.1.14 para Fiber v3. Es de terceros, no del equipo oficial de Scalar; el módulo depende internamente de `swaggo/swag/v2`, pero el API ya no usa anotaciones ni el generador Swagger v1.
- Se fuerza el modo offline: el bundle Scalar v1.65.1 se sirve localmente y no depende de un CDN. La integración recibe JSON derivado en memoria del YAML embebido.
- No exponer tokens ni claves de Zitadel en el documento, en ejemplos, HTML, logs o repositorio.

### Tipos y límites de generación

- Candidato evaluado: `oapi-codegen` v2.8.0. No se adopta con OAS 3.2 porque su documentación declara soporte hasta 3.1.
- Generar requests/responses en un paquete de transporte separado (por ejemplo, `internal/transport/openapi`); mantener los puertos y modelos de dominio actuales independientes del contrato.
- Los handlers Fiber siguen siendo explícitos: parsean el request, llaman al caso de uso y mapean al response generado.
- No generar un servidor para otro framework ni acoplar dominio/usecases al código generado. La compatibilidad con Fiber v3 y la arquitectura actual tiene prioridad.
- Por ahora se conservan los DTOs manuales, con validación del spec y pruebas del endpoint de publicación. No se afirma que esos DTOs se sincronicen automáticamente con el contrato.

## 6. Contenido que debe cubrir el contrato

Antes de escribir el YAML, contrastar rutas, DTOs, tests, middleware, error handler y comportamiento observado. Incluir al menos:

### Operaciones

| Método y ruta            | Seguridad / intención                                                              |
| ------------------------ | ---------------------------------------------------------------------------------- |
| `GET /auth/me`           | Bearer token; devuelve el perfil autenticado y puede sincronizar el usuario en DB. |
| `GET /templates`         | Bearer token; filtros opcionales actuales `language` y `framework`.                |
| `GET /templates/{id}`    | Bearer token; ID UUID; errores de formato y not-found.                             |
| `POST /templates`        | Bearer token + rol admin; request de creación y validación.                        |
| `PATCH /templates/{id}`  | Bearer token + rol admin; actualización parcial.                                   |
| `DELETE /templates/{id}` | Bearer token + rol admin; desactivación lógica y respuesta sin body.               |
| `GET /health`            | Público; probe de liveness actual.                                                 |
| `GET /ready`             | Público; probe de readiness actual.                                                |

La UI y el documento OpenAPI no se describen como operaciones de negocio. Documentar sus URLs de publicación en la guía de desarrollo; no incluirlas en `paths` salvo que se decida expresamente que forman parte del contrato público.

### Convenciones OpenAPI 3

- `openapi: 3.2.0`; `info.version` versiona el contrato/API, no la versión de la especificación OpenAPI.
- Reemplazar `basePath: /api/v1` por `servers`, preferentemente con URL relativa `/api/v1`.
- Como `/health` y `/ready` viven en la raíz, darles un `servers` de operación con URL `/`; no deben heredar `/api/v1`.
- `components.schemas` para requests, responses y errores reutilizables; `components.securitySchemes` para auth.
- Bearer auth como `type: http`, `scheme: bearer`. Documentar que el valor aceptado es un token emitido por Zitadel (PAT o access token); `zitadel-api-key.json` es una credencial de servidor y nunca se introduce en Swagger/Scalar.
- Declarar por operación qué endpoints son públicos, autenticados o admin-only. Para permisos de rol que OpenAPI no expresa directamente, incluir el requisito en la descripción y `403` en responses; no inventar roles OAuth scopes que el backend no valida.
- Documentar `Content-Type`, headers de request/response, parámetros, campos requeridos, formatos (`uuid`, `date-time`), nulabilidad, y `additionalProperties` donde aplique.
- Compartir schemas `ErrorResponse` para el formato real del error handler (`error`, `request_id`) y detallar status codes efectivamente devueltos, no solo los anotados hoy.
- Mantener ejemplos pequeños, sintéticos y sin tokens, datos personales ni secretos.

## 7. Fases de trabajo

### Fase 0 — Baseline y decisión de herramientas [completada]

1. Confirmar la versión objetivo de OAS mediante el gate de la sección 3.
   - R1: usemos OpenAPI 3.2; si algo no funciona, rollback a 3.1.1
2. Fijar versiones de Scalar integration, parser/linter y `oapi-codegen` (si se adopta); no depender de `latest` en CI.
   - R2: fijaremos las últimas versiones disponibles a la fecha
3. Probar la carga del YAML canónico en Scalar, salida offline, URL relativa de `servers`, autenticación Bearer y soporte de los schemas reales.
4. Guardar un inventario de rutas/resultados actuales desde handlers y tests, incluidos `400`, `401`, `403`, `404`, `409`, `422` y `500` donde existan.

**Salida / aceptación:** versión de OAS y herramientas fijadas; spike mínimo que demuestra render, `$ref`, Bearer y “Try it” sin cambiar el API.

**Resultado de fase 0:** OAS 3.2.0, Scalar v3.1.14 y Vacuum v0.33.1 fijados; Vacuum valida el documento con cero errores. `oapi-codegen` v2.8.0 no se adopta porque declara soporte hasta OAS 3.1.

### Fase 1 — Escribir el contrato canónico [completada]

1. Crear `docs/openapi/openapi.yaml` con `info`, `servers`, tags, seguridad y schemas comunes.
2. Especificar las 8 operaciones de la tabla, reutilizando schemas mediante `$ref`.
3. Describir cuerpos y respuestas desde DTOs y tests; corregir discrepancias en el spec, no conservar una anotación solo porque exista.
4. Revisar especialmente validación de templates, PATCH parcial, UUID, respuesta 204 y formato JSON de errores.
5. Revisar si `/health` y `/ready` deben aparecer como operaciones documentadas y reflejar la decisión.

**Salida / aceptación:** spec completo, legible, parseable y sin dependencias de código Go ni de los artefactos de `swag`.

**Resultado de fase 1:** `docs/openapi/openapi.yaml` define las ocho operaciones actuales, schemas reutilizables, seguridad, errores y probes públicos; no depende de anotaciones Go ni de artefactos Swagger.

### Fase 2 — Asegurar conformidad sin generación [completada]

1. Se evaluó `oapi-codegen` v2.8.0, pero no se adopta con OAS 3.2 porque su documentación declara soporte hasta 3.1.
2. Se conservan los DTOs actuales y se eliminaron sus tags `swaggertype`, que ya no tienen consumidor.
3. Un test asegura que el contrato declara exactamente las ocho operaciones actuales y OAS 3.2.0.
4. Los tests existentes de handlers/middleware siguen verificando validación, status codes y respuestas; el contrato documenta los resultados observados.
5. Probar cada endpoint protegido con token simulado en unit tests y comprobar la definición Bearer en el contrato; no depender de Zitadel real en la suite unitaria.

**Resultado:** no hay archivos generados desde el contrato; las pruebas fijan la lista de operaciones y la publicación exacta del YAML.

### Fase 3 — Publicar spec y UI Scalar en Fiber [completada]

1. Embeder el YAML canónico y servirlo en `/api/v1/openapi.yaml`.
2. Montar Scalar en `/api/v1/docs`; convertir el YAML embebido a JSON en memoria para la integración, que solo admite contenido inline.
3. Mantener la ruta de documentación pública y verificar que no hereda el middleware de auth de `/api/v1` por accidente.
4. Configurar el UI para que “Try it” use `/api/v1` del mismo origin y permita introducir el Bearer token. No inyectar PAT ni credenciales en HTML/JS.
5. Verificar el modo offline y la carga en una build binaria, no solo desde el directorio del repo.

**Resultado:** `/api/v1/openapi.yaml` sirve el YAML canónico y `/api/v1/docs` sirve Scalar con asset local; ambas rutas son públicas. Los tests verifican el HTML, el contenido 3.2 embebido y el bundle offline.

### Fase 4 — Retirar Swagger 2 y comentarios [completada]

1. Retirar `swaggo/http-swagger`, `swaggo/swag` v1 y dependencias sin uso. `gofiber-scalar` conserva `swaggo/swag/v2` como dependencia transitiva, pero no se usa para generar docs desde comentarios.
2. Eliminar `docs/swagger/` generado y retirar las anotaciones del antiguo `cmd/server/docs.go`, que ahora registra Scalar.
3. Eliminar las anotaciones de endpoints (`@Summary`, `@Router`, etc.) de handlers.
4. Reemplazar `make swagger` por targets explícitos, por ejemplo:
   - `make openapi-lint`: validar estructura, referencias y convenciones.
   - `make openapi-check`: ejecutar lint y tests de publicación/contenido; no modifica el worktree.
5. Actualizar `docs/DESIGN.md` y el análisis de tooling para señalar el YAML como fuente de verdad y Scalar como renderer.

**Salida / aceptación:** ningún generador lee comentarios Go para producir el contrato; ningún artefacto Swagger 2.0 obsoleto queda servido o referenciado.

**Resultado de fase 4:** anotaciones, artefactos Swagger 2 y dependencias antiguas retirados; Makefile, `docs/DESIGN.md`, análisis de tooling y workflow CI actualizados.

### Fase 5 — CI, pruebas funcionales y cierre [validación automatizada completa]

En CI y local ejecutar:

```sh
make openapi-check
go test ./...
make test
go vet ./...
go build ./cmd/server
```

Pruebas funcionales con API local:

- Abrir `/api/v1/docs`, confirmar que se ven las 8 operaciones acordadas y no hay errores de carga del spec.
- Abrir/descargar `/api/v1/openapi.yaml` y verificar que corresponde al archivo fuente.
- Ejecutar `GET /health` y `GET /ready` desde Scalar o directamente; probar un endpoint protegido con un PAT de prueba.
- Verificar que Scalar no envía token si no se autorizó y que los fallos 401/403 coinciden con el contrato.
- Ejecutar un request inválido a `POST`/`PATCH /templates` y comparar `400`/`422` y formato de error con el spec.
- Confirmar `make run`, build del binario y rutas detrás del prefijo de API.

**Salida / aceptación:** los checks automatizados locales pasan; el workflow agregado ejecutará los mismos gates en push/PR. El smoke test interactivo con el entorno local y un PAT queda pendiente, sin cambios de rutas, autorizaciones ni status codes.

**Resultado automatizado:** `make openapi-check`, 145 tests, `make test` (74.6% de cobertura total), `go vet ./...` y `go build ./cmd/server` pasan. Vacuum v0.33.1 informa cero errores y 44 advertencias informativas (score 86/B), principalmente ejemplos opcionales y la convención snake_case del JSON existente. El smoke test interactivo con un PAT real queda para el entorno local; no se leyó ni usó ningún secreto en esta ejecución.

## 8. Cambios esperados en el repositorio

| Área         | Cambio planeado                                                                                           |
| ------------ | --------------------------------------------------------------------------------------------------------- |
| Contrato     | Nuevo `docs/openapi/openapi.yaml` (fuente canónica).                                                      |
| UI           | Sustituir `http-swagger` por Scalar en la ruta de docs.                                                   |
| Servidor     | Añadir embed/handler del spec y configuración Scalar en Fiber v3.                                         |
| Modelos      | Se mantienen DTOs manuales; `oapi-codegen` no declara soporte para la versión OAS 3.2 elegida.           |
| Handler      | Eliminar anotaciones `swag`; conservar handlers Fiber y puertos de negocio.                               |
| Dependencias | Retirar generador/UI antiguos; añadir y pinnear solo herramientas/integ. necesarias.                      |
| Make/CI      | `openapi-lint` / `openapi-check` fijados y ejecutados en `.github/workflows/ci.yml`.                      |
| Docs         | Actualizar guía de herramientas, arquitectura y forma de probar con Scalar.                               |

## 9. Riesgos y controles

| Riesgo                                                | Control                                                                                                                  |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Spec distinto al comportamiento                       | Inventario desde código/tests; test de las ocho operaciones; tests existentes de status y respuestas.                   |
| OpenAPI 3.2 aún no soportado por una herramienta      | Gate de compatibilidad antes del contrato definitivo; fallback explícito a 3.1.1.                                        |
| Scalar obtiene scripts de CDN externo                 | Integración fijada y modo offline; probar build/ejecución sin CDN.                                                       |
| DTOs manuales se desincronizan del spec                | Revisar el contrato junto con handlers; mantener el test de operaciones y los tests HTTP existentes.                    |
| URLs incorrectas por `localhost`, proxy o prefijo     | URL relativa `/api/v1`; prueba same-origin y entorno local.                                                              |
| Seguridad mostrada incorrectamente en UI              | Esquema Bearer común; probar PAT válido/ausente; no documentar la key privada como token.                                |
| Cambios de comportamiento mezclados con documentación | No cambiar handlers, auth ni status codes salvo discrepancias aprobadas; separar cualquier cambio contractual funcional. |

## 10. Fuera de alcance

- Reemplazar Fiber o adoptar Huma/otro router.
- Cambiar endpoints, payloads, roles, auth, status codes o comportamiento de negocio.
- Diseñar nuevos endpoints todavía no implementados (build, deploy, apps, etc.); se agregarán al YAML cuando se implementen, contract-first.
- Añadir un frontend separado de documentación o pipeline Node permanente si la integración Go resuelve render y offline.
- Cambiar la UI del frontend de `dev-forge-ui`; Scalar es solo la referencia interactiva de esta API.

## 11. Criterios de finalización

- [x] Un único YAML OpenAPI 3.2.0 versionado define rutas, seguridad, requests y responses actuales.
- [x] `swagger.yaml` generado y anotaciones fueron retirados; `swaggo/swag` v1 ya no es dependencia directa ni requisito de build.
- [ ] Scalar renderiza el YAML local y puede probar endpoints con Bearer sin exponer credenciales.
- [x] No se generan tipos HTTP; se mantienen DTOs manuales porque `oapi-codegen` v2.8.0 no declara soporte para OAS 3.2.
- [x] CI valida la spec, la lista de operaciones y la publicación del contrato; no hace validación runtime automática de cada schema de respuesta.
- [x] Las 8 operaciones acordadas y los errores documentados coinciden con el inventario y los tests existentes.
- [x] Se actualizaron la arquitectura, el análisis histórico y la guía del plan.

## Fuentes consultadas

- [OpenAPI Specification 3.2.0](https://spec.openapis.org/oas/v3.2.0.html)
- [OpenAPI Specification 3.1.1](https://spec.openapis.org/oas/v3.1.1.html)
- [Scalar: configuración de API Reference; URL admite YAML o JSON](https://scalar.com/products/api-references/configuration)
- [Scalar: integraciones Go](https://scalar.com/products/api-references/integrations/go)
- [Scalar API Reference: integración Fiber v3 de terceros](https://github.com/yokeTH/gofiber-scalar)
- [oapi-codegen: generación Go desde OpenAPI 3.0/3.1](https://github.com/oapi-codegen/oapi-codegen)

## Referencias del proyecto

- [Análisis histórico de herramientas](./api-docs-tooling-analysis.md)
- [Arquitectura](./DESIGN.md)
- [`Makefile`](../Makefile)
- [`docs/openapi/openapi.yaml`](./openapi/openapi.yaml)
- [Workflow CI](../.github/workflows/ci.yml)
