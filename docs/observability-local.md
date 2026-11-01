# Laboratorio local de observabilidad

Este laboratorio es para desarrollo y pruebas. `grafana/otel-lgtm` reúne un
Collector, Grafana, Prometheus, Tempo, Loki y otros componentes en un contenedor;
no representa una instalación durable ni de alta disponibilidad.

## Requisitos

- Docker con Compose y el daemon en ejecución.
- Python 3 para la prueba de humo.
- Puertos locales 3000, 4317 y 4318 libres. Se publican solo en `127.0.0.1`.
- Memoria disponible para Docker: el consumo depende de la carga. En una
  ejecución local con Docker Desktop configurado con aproximadamente 4 GiB,
  el contenedor llegó a utilizar alrededor de 1,4 GiB.

Desde la raíz del repositorio:

```sh
make obs-up
make obs-smoke-stack
```

La prueba descarga una imagen de `telemetrygen` fijada por digest si hace
falta, envía trazas, métricas y logs sintéticos mediante OTLP/HTTP y busca cada
señal en su backend a través de Grafana. Utiliza un nombre de servicio nuevo
en cada ejecución y espera hasta 120 segundos por la ingesta. No verifica la
instrumentación de la API, la correlación log-traza ni los dashboards propios:
esas verificaciones corresponden a fases posteriores.

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

## Estado y límites

Las variables OTEL de `.env.example` documentan el contrato objetivo. En las
fases 0 y 1 todavía **no se inicializan exporters ni se instrumenta HTTP**;
poner `OTEL_ENABLED=true` no hace que la API envíe las tres señales. El
endpoint `localhost` corresponde a una app ejecutada en el host, no dentro de
un contenedor.

Una prueba de humo exitosa no valida retención, backup, aislamiento entre
usuarios, seguridad del homelab, disponibilidad del stack ni el pipeline de
la aplicación real. Esos requisitos y las pruebas restantes están en
[OBSERVABILITY-PLAN.md](../OBSERVABILITY-PLAN.md).
