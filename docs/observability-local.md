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
make obs-up
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

## Dashboard HTTP

Compose monta y provisiona [Dev Forge · Solicitudes HTTP](../deployments/grafana/dashboards/dev-forge-red.json)
en la carpeta *Dev Forge*, sin pasos de importación en la UI. El dashboard se
limita a `service_name="dev-forge"` para excluir las series aleatorias de los
smoke tests. Muestra el total y los conteos 2xx/3xx/4xx/5xx del rango
seleccionado, el conteo por código HTTP y la tasa en solicitudes por minuto
agrupada por código HTTP a lo largo del tiempo. Los conteos usan `increase` y se redondean a solicitudes
enteras; la tabla muestra solo códigos con solicitudes y permite distinguir,
por ejemplo, 200 de 401 y 500. El
datasource Prometheus usa el UID `prometheus`; la provisión de archivos está en
[dev-forge.yaml](../deployments/grafana/provisioning/dev-forge.yaml).

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
