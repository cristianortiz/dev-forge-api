# Laboratorio local de observabilidad

Este laboratorio es para desarrollo y pruebas. `grafana/otel-lgtm` reúne un
Collector, Grafana, Prometheus, Tempo, Loki y otros componentes en un contenedor;
no representa una instalación durable ni de alta disponibilidad.

## Requisitos

- Docker con Compose y el daemon en ejecución.
- Python 3 para la prueba de humo.
- Go y PostgreSQL para la prueba integrada de la API.
- Puertos locales 3000, 4317 y 4318 libres. Se publican solo en `127.0.0.1`.
- Memoria disponible para Docker: el consumo depende de la carga. En una
  ejecución local con Docker Desktop configurado con aproximadamente 4 GiB,
  el contenedor llegó a utilizar alrededor de 1,4 GiB.

Desde la raíz del repositorio:

```sh
make dev
```

`make dev` inicia PostgreSQL y el stack de observabilidad, aplica las
migraciones y ejecuta la API con hot reload. `make run` hace lo mismo sin
hot reload; ambos cargan las variables de `.env`. Zitadel continúa siendo un
servicio externo. Para iniciar solo los servicios y aplicar migraciones, usa
`make setup`.

Las comprobaciones del stack se ejecutan por separado:

```sh
make obs-smoke-stack
make obs-smoke
```

La prueba descarga una imagen de `telemetrygen` fijada por digest si hace
falta, envía trazas, métricas y logs sintéticos mediante OTLP/HTTP y busca cada
señal en su backend a través de Grafana. Utiliza un nombre de servicio nuevo
en cada ejecución y espera hasta 120 segundos por la ingesta. No verifica la
instrumentación de la API, la correlación log-traza ni los dashboards propios:
esas verificaciones corresponden a fases posteriores.

`make obs-smoke` levanta o espera Grafana/OTel-LGTM y PostgreSQL, ejecuta el
test Go etiquetado `obsintegration` y consulta Tempo, Loki y Prometheus por el
proxy de Grafana. Verifica respuestas 2xx/4xx/5xx y panic, un span de pgx,
correlación de logs, cardinalidad de rutas/404, el dashboard HTTP provisionado
y el caso sin muestreo. Funciona con los defaults si `.env` no existe; si existe,
el target lo carga para respetar la configuración local de PostgreSQL. También
acepta `OBS_GRAFANA_URL`, `OBS_GRAFANA_USER`, `OBS_GRAFANA_PASSWORD` y
`OBS_OTLP_ENDPOINT` desde el entorno. `obs-smoke-stack` sigue siendo el smoke
sintético independiente.

## Dashboard de observabilidad

Compose monta y provisiona [Dev Forge · Observabilidad](../deployments/grafana/dashboards/dev-forge-red.json)
en la carpeta *Dev Forge*, sin pasos de importación en la UI. El dashboard se
limita a `service_name="dev-forge"` para excluir las series aleatorias de los
smoke tests. Muestra el total y los conteos 2xx/3xx/4xx/5xx del rango
seleccionado, el conteo por código HTTP y la tasa en solicitudes por minuto
agrupada por código HTTP a lo largo del tiempo. Los conteos usan `increase` y se redondean a solicitudes
enteras; la tabla muestra solo códigos con solicitudes y permite distinguir,
por ejemplo, 200 de 401 y 500. El
datasource Prometheus usa el UID `prometheus`; la provisión de archivos está en
[dev-forge.yaml](../deployments/grafana/provisioning/dev-forge.yaml).

También incluye una tabla de solicitudes por endpoint ordenada de mayor a menor,
latencia p95 por endpoint para el rango seleccionado, un panel con los últimos
50 logs INFO de acceso, un resumen de errores HTTP por status y endpoint, una
lista de requests 4xx/5xx y una vista de hasta 200 diagnósticos de aplicación.
La vista de requests fallidas selecciona solo logs de acceso (`method` presente)
y muestra en una única entrada el método, ruta, status, `error_code`, mensaje y,
cuando corresponde, la causa técnica. El panel de diagnóstico se reserva para
eventos DEBUG/WARN/ERROR que no corresponden a una request HTTP, evitando
repetir el detalle en dos paneles. Los 4xx mantienen WARN y los 5xx ERROR: el
panel dedicado destaca los errores por status sin cambiar su severidad. Se omite
el stack trace para mantener los paneles legibles; el log completo sigue
disponible al consultar Loki desde Explore. Al abrir una entrada se pueden
inspeccionar atributos como la latencia y los IDs de traza.
Las rutas se toman de `http_route` en Prometheus y se omiten
requests sin ruta reconocida (por ejemplo, 404) para evitar cardinalidad por
paths arbitrarios. Los logs muestran los atributos estructurados `method`,
`route`, `status`, `latency` y los IDs de traza; no se indexan `trace_id` ni
`span_id` como labels de Loki.

El datasource Loki ya enlaza `trace_id` con Tempo y Tempo busca en Loki por
servicio e ID de traza. Así, al abrir el detalle de un log se puede navegar a su
traza; desde Tempo se puede volver a los logs relacionados. La consulta de Tempo
amplía el intervalo del span 5 segundos en ambos extremos para incluir logs
emitidos justo antes o después del span. El smoke integrado verifica ambas
configuraciones de correlación, el margen temporal y los campos de los logs.

En desarrollo, la consola conserva el mensaje del error y el caller, pero no
imprime el stack trace completo. Fiber también recupera los panics sin volcar
el stack a `stderr`; la respuesta sigue siendo 500 y el error queda registrado.
Los errores de dominio tipados mantienen sus mensajes y mapeos HTTP actuales.
Los handlers de templates y el middleware de autenticación añaden códigos de
error de baja cardinalidad al log de acceso consolidado, no a las labels
indexadas de Loki ni al contrato HTTP de respuesta. La validación fallida usa
`invalid_or_expired_token` y la sincronización fallida del usuario usa
`auth_user_sync_failed`; el valor del token no se añade como campo del log. Las
alertas operativas por tasa 5xx o p95 quedan pendientes de acordar umbrales y
canal de notificación.

## Contrato de señales comprobado

El smoke integrado observó estos nombres después de la traducción OTEL →
Prometheus:

| Señal | Nombres observados |
|---|---|
| HTTP | `http_server_request_duration_seconds_{bucket,count,sum}`, `http_server_request_body_size_bytes_{bucket,count,sum}`, `http_server_response_body_size_bytes_{bucket,count,sum}`, `http_server_active_requests` |
| PostgreSQL | `db_client_operation_duration_seconds_{bucket,count,sum}`, `db_client_operation_errors_total` |
| Runtime/recursos | `go_memory_used_bytes`, `go_memory_allocated_bytes_total`, `go_memory_allocations_total`, `go_memory_gc_goal_bytes`, `go_goroutine_count`, `go_processor_limit`, `go_config_gogc_percent`, `target_info` |

En Prometheus se comprobaron `service_name`, `deployment_environment_name` y
`service_version`, además de dimensiones HTTP como `http_route`,
`http_response_status_code` y `http_request_method`; los histogramas agregan
`le`. En Loki, `/series` confirmó que `service_name` y
`deployment_environment_name` son labels indexadas. `trace_id` y `span_id`
aparecen como metadatos estructurados consultables, no como labels indexadas.

Medición del servicio muestreado de una ejecución de `make obs-smoke` (65
requests; la ejecución incluye además una request no muestreada para verificar
ese caso). Las cifras son respuestas JSON sin comprimir de las APIs de consulta:

| Medición | Resultado |
|---|---:|
| Series Prometheus del servicio integrado | 429 |
| Eventos de log consultados para 65 requests | 102 |
| Respuesta de consulta Loki | 74,139 bytes (≈1,141 bytes/request) |
| Trazas consultadas | 65 |
| Respuestas Tempo | 105,593 bytes (≈1,624 bytes/trace) |
| Extrapolación a 1 request/s durante una hora | ≈3.9 MiB de logs y ≈5.6 MiB de trazas |

La extrapolación sirve para comparar cargas, no representa bytes en disco: no
incluye compresión, índices, WAL, replicación ni retención. Las 429 series son
una instantánea del conjunto fijo de rutas, estados, buckets y métricas de
runtime; los 30 IDs y 30 paths 404 del test no agregaron series por valor de
entrada.

Grafana está en <http://localhost:3000>; sus credenciales **solo para este
laboratorio local** son `admin` / `admin`. No exponer esta instalación en una
red pública ni reutilizar esas credenciales en el homelab.

| Servicio | Desde el host | Desde otro contenedor en la red Compose |
|---|---|---|
| Grafana | `http://localhost:3000` | `http://otel-lgtm:3000` |
| OTLP/HTTP | `http://localhost:4318` | `http://otel-lgtm:4318` |
| OTLP/gRPC | `localhost:4317` | `otel-lgtm:4317` |

Para observar el arranque: `make obs-logs` (Ctrl+C para dejar de seguir los
logs). `make obs-down` detiene y elimina solo el contenedor de observabilidad,
sin detener PostgreSQL ni borrar volúmenes.

## Persistencia opcional

Por defecto, los datos se pierden cuando se elimina el contenedor. Para
mantenerlos al recrearlo, iniciar con:

```sh
make obs-up OBS_PERSIST=1
```

El volumen de observabilidad pertenece al proyecto Compose. Para eliminarlo
explícitamente, junto con el contenedor de observabilidad:

```sh
make obs-clean OBS_CONFIRM_CLEAN=yes
```

Este comando no elimina el volumen de PostgreSQL. Mantén el mismo
`COMPOSE_PROJECT_NAME` entre los comandos si lo configuras.

## Enviar la telemetría de la API

La configuración vive en `.env` (ignorado por Git; `.env.example` solo lista las
variables). `make run` y `make dev` lo cargan. Para exportar:

```sh
make obs-up
# en .env: OTEL_ENABLED=true  (el resto de OTEL_* ya apunta a http://localhost:4318)
make run
```

Genera tráfico y abre Grafana en <http://localhost:3000> → *Explore*:

- **Tempo:** trazas del servicio `dev-forge`. Cada respuesta incluye el
  encabezado `X-Trace-Id` y el log de acceso lleva el mismo `trace_id`.
- **Prometheus:** `http_server_request_duration_seconds_count`,
  `db_client_operation_duration_seconds_count`, `go_goroutine_count`, etc.,
  filtrando por `service_name="dev-forge"`.
- **Loki:** logs de aplicación por OTLP, filtrando por
  `{service_name="dev-forge"}`. El `trace_id` permite abrir la traza en Tempo;
  desde una traza, el datasource de Tempo consulta los logs de Loki.
  Loki los devuelve como metadatos estructurados para filtrarlos, no como
  labels indexadas de cada serie.

Para comprobar la correlación sin credenciales de API, envía una request no
autenticada y conserva el `X-Trace-Id` de la respuesta:

```sh
curl -i 'http://localhost:9000/api/v1/templates/example?access_token=probe'
```

En Explore, filtra Loki por `{service_name="dev-forge"} | trace_id="<id>"`.
El log de acceso contiene la plantilla de ruta, no el segmento ni el query; no
incluye IP, cabeceras, cuerpo o token.

Comportamiento a tener presente:

- Con `OTEL_ENABLED=false` no se crea ningún exporter ni se registra el
  middleware de trazas.
- Un valor inválido en cualquier `OTEL_*` (endpoint sin `http://`, sampler
  desconocido, intervalo no numérico…) **impide arrancar** y el error nombra la
  variable. `OTEL_ENABLED=ture` también.
- Si el stack no está levantado, la API funciona con normalidad: descarta la
  telemetría, usa memoria acotada y escribe como máximo un aviso por minuto. El
  primer aviso puede tardar ~1 minuto porque el exportador reintenta antes de
  rendirse.
- Al apagarse (Ctrl+C o `SIGTERM`) la API vacía la telemetría pendiente durante
  como máximo `OTEL_SHUTDOWN_TIMEOUT`.
- Los probes `/health` y `/ready` no generan trazas ni métricas.

## Estado y límites

Implementado: trazas, métricas y logs correlacionados de HTTP y aplicación por
OTLP; trazas y métricas de PostgreSQL; métricas de runtime de Go; dashboard RED
provisionado y smoke integrado de la fase 5. El endpoint `localhost`
corresponde a una app ejecutada en el host, no dentro de un contenedor.

Una prueba de humo exitosa no valida retención, backup, aislamiento entre
usuarios, seguridad del homelab ni disponibilidad del stack. Esos requisitos y
las pruebas restantes están en [OBSERVABILITY-PLAN.md](../OBSERVABILITY-PLAN.md).
