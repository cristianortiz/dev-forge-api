# Observabilidad para dev-forge y el blueprint Go

**Fecha:** 2026-10-30  
**Estado:** análisis y propuesta; no implica que la instrumentación o el stack estén implementados.

## 1. Resumen ejecutivo

La idea de usar `dev-forge-api` como laboratorio para construir un blueprint opinado de Go es razonable: ya tiene una estructura modular, Fiber, PostgreSQL, Zap, autenticación y un contrato OpenAPI. Pero **dev-forge no es todavía un template genérico** y tampoco es suficiente copiarlo para tener un proyecto listo para producción. La plataforma tiene dominio, credenciales, despliegue y dependencias propios que no deberían heredarse indiscriminadamente.

Mi recomendación es:

1. Instrumentar primero `dev-forge-api` como aplicación real y validar allí el patrón que después se extraerá al blueprint.
2. Separar el **código que emite telemetría** de la **infraestructura que la recoge, almacena y visualiza**. Cada aplicación generada lleva lo primero; no debe desplegar su propio Prometheus, Loki, Tempo ni Grafana en Kubernetes.
3. Mantener el stack ya propuesto en [DESIGN.md](docs/DESIGN.md): OpenTelemetry, Collector Contrib, Prometheus, Loki, Tempo y Grafana. Recomiendo **Tempo** frente a Jaeger para este proyecto, aunque ambos son alternativas válidas.
4. Usar **una sola ruta para las tres señales: OTLP hacia un único endpoint** (trazas, métricas y logs). Stdout se mantiene para consola y `kubectl logs`, pero no es la vía de ingesta. Así local y homelab usan la misma app y solo cambia `OTEL_EXPORTER_OTLP_ENDPOINT`. *(Decisión 2026-11-01; sustituye la propuesta inicial de scrape de `/metrics` y recolección de logs por stdout. Ver §13.)*
5. Entregar un entorno local reproducible con un único contenedor `grafana/otel-lgtm` (versión fijada), verificado el 2026-11-01 con las tres señales. En Kubernetes, conectar las aplicaciones al stack compartido del cluster mediante configuración inyectada.
6. No llamar «production-ready» al resultado solo por tener observabilidad. Es una **base orientada a producción**, sujeta a pruebas, operación y decisiones de seguridad del entorno.

Esta recomendación **adelanta parte de la Fase 4** como trabajo de infraestructura de aplicación. No completa por sí sola el despliegue del stack en el homelab ni los endpoints de producto `observe/logs` y `observe/metrics`. El estado actual y el orden previo están en [DEV-FORGE-PLAN.md](DEV-FORGE-PLAN.md).

## 2. Qué problema resolvemos

Queremos que, al generar una API Go desde dev-forge:

- el código ya produzca trazas, métricas y logs correlacionables;
- el desarrollador pueda observarla en local con instrucciones mínimas;
- la misma imagen pueda desplegarse en Kubernetes y conectarse al stack existente;
- no haya credenciales, direcciones permanentes ni backends embebidos en el código;
- una caída del sistema de observabilidad no impida atender requests.

Hay dos sujetos diferentes:

| Sujeto | Qué observamos | Quién lo usa |
|---|---|---|
| **Control plane**: `dev-forge-api` | Requests del producto, catálogo, auth, DB y, cuando existan, builds y deploys | Equipo que opera la plataforma |
| **Workloads**: apps generadas | Sus propias requests, dependencias, métricas de negocio y logs | Equipos dueños de cada aplicación |

Pueden compartir infraestructura sin convertirse en la misma aplicación. Deben conservar identidad y autorización separadas. El futuro módulo `observe` será un consumidor autorizado de las APIs de los backends, **no otro almacén de telemetría**.

### Monitorización frente a observabilidad

Una alerta de latencia indica *qué* se degradó; una traza y sus logs ayudan a investigar *por qué*. Las métricas son agregados numéricos; los logs describen eventos; las trazas relacionan operaciones de una misma petición. Son complementarios, no tres copias de los mismos datos.

## 3. Estado real de `dev-forge-api`

La revisión es estática; no se ejecutó el stack ni se realizaron pruebas de carga.

| Pieza | Evidencia actual | Qué falta |
|---|---|---|
| Composición | [cmd/server/main.go](cmd/server/main.go) inicializa configuración, Zap, PostgreSQL, Fiber y shutdown | Inicializar providers de telemetría antes de crear DB y servidor; apagarlos con timeout |
| Configuración | [.env.example](.env.example) contiene `OTEL_ENABLED`, nombre, versión y endpoint | Definir y probar el contrato completo por señal, protocolo, sampling y seguridad; las variables existentes no prueban que exista un exporter |
| PostgreSQL | [database.go](internal/shared/database/database.go) instala `otelpgx` si OTEL está habilitado | Un `TracerProvider` configurado y un contexto propagado para producir y exportar spans útiles |
| HTTP y logs | [middleware.go](internal/shared/server/middleware.go) registra método, path, status, latencia, IP y request ID con Zap | Spans y métricas HTTP, propagación y correlación con trace/span; revisar rutas reales, IPs y errores por privacidad |
| Logger | [logger.go](internal/shared/logger/logger.go) usa salida de consola en debug y JSON en producción | Perfil JSON consistente cuando un recolector debe parsear logs, y enriquecimiento desde el contexto |
| Salud | [health.go](internal/shared/server/health.go) responde estáticamente a `/health` y `/ready` | Diferenciar semántica y comprobar dependencias críticas en readiness; observabilidad no debe ser una de ellas |
| Stack | [deployments/docker-compose.yml](deployments/docker-compose.yml) está orientado a PostgreSQL | No hay un laboratorio de observabilidad local listo para usar |

El hecho de tener paquetes OTEL en `go.mod`, un endpoint configurado o `otelpgx` conectado **no significa** que se estén enviando trazas. Tampoco `request_id` equivale a `trace_id`: el primero identifica una request según la política de la aplicación; el segundo relaciona spans a través de procesos.

Además, el middleware actual registra el status tras `c.Next()`; al instrumentarlo debemos probar que errores y panics reflejen el **status HTTP finalmente enviado**, no asumir que ya estaba escrito cuando el middleware leyó la respuesta.

## 4. Análisis de `observ-monit-go`

Se revisó el repositorio local `/Users/cristianortiznavia/goProjects/observ-monit-go` como referencia, sin copiar su configuración ni sus credenciales. Su `README` mezcla la terminología de monolito modular y microservicios; la conclusión siguiente se basa en los archivos revisados, no en asumir que todas las capacidades descritas en el README se ejecutan juntas.

### 4.1 Cómo funciona la ruta principal de `factorit`

Según `cmd/factorit/main.go`, `docker-compose.yml`, `config/otel-collector/otel-collector-config.yaml` y `pkg/config/prometheus/prometheus.yml`:

```text
Fiber v2 + otelfiber ── OTLP traces ──> Collector ── OTLP ──> Jaeger
OTEL MeterProvider   ── OTLP metrics ─> Collector ── /metrics ──> Prometheus
Zap + bridge OTEL    ── OTLP logs ────> Collector ── exporter Loki ──> Loki
Zap ────────────────── stdout        (docker logs)
Grafana consulta las fuentes que estén configuradas.
```

El Compose contiene PostgreSQL, su exporter, Prometheus, Grafana, Jaeger, Collector y Loki. Promtail figura comentado, no como servicio activo. Hay dashboards JSON en el repo; el Compose revisado monta un volumen de datos de Grafana, pero no configura en él provisioning de datasources y dashboards. Por tanto **no se puede concluir que un entorno nuevo los cargue automáticamente**.

La referencia tiene un segundo entrypoint, `cmd/users-service/main.go`, que inicializa servidor, DB y health, pero no configura el mismo pipeline de tracing y métricas que `factorit`. No se debe presentar todo el repositorio como instrumentado uniformemente.

### 4.2 Ideas que sí conviene reutilizar

- Separación entre configuración, logger, tracing, métricas y health.
- Middleware HTTP y `otelpgx`, con propagación de contexto hacia el repositorio.
- Recursos OTEL con nombre, versión y entorno; batch y shutdown con timeout.
- Histogramas de latencia y métricas de solicitudes, además de métricas de DB y negocio cuando tengan utilidad.
- Correlación explícita de logs con el span activo (`WithTraceContext`).
- Un laboratorio Compose que permita experimentar sin depender del cluster.

### 4.3 Qué no trasladaría literalmente

1. **Compatibilidad de Fiber:** usa Fiber v2 y `github.com/gofiber/contrib/otelfiber/v2`; dev-forge usa Fiber v3. No copiar middleware ni asumir compatibilidad. Verificar una integración compatible y mantenida, sus convenciones semánticas y propagación; si no existe una adecuada, diseñar una capa pequeña y probarla.
2. **Cardinalidad:** `pkg/observability/metrics/otel_middleware.go` recurre al path concreto cuando no hay ruta; el error handler de `cmd/factorit/main.go` también usa `c.Path()`. Un 404 con un identificador distinto por request puede crear series ilimitadas. Usar una etiqueta fija como `unmatched` para rutas no reconocidas; nunca el path crudo como label.
3. **Datos personales:** `pkg/observability/tracing/attributes.go` define y añade ID, email y nombre a spans, y el repositorio de users los utiliza. No adoptar esto por defecto. Los traces también son datos sensibles aunque sus atributos no sean labels de métricas.
4. **Errores y dimensiones:** la referencia pasa valores como `err.Error()` y un ID de usuario a funciones de métricas de auth. Revisar la implementación y reemplazar cualquier dimensión basada en valores variables por categorías acotadas; no convertir errores, identidades o entradas arbitrarias en labels.
5. **Deshabilitar métricas:** `NewOTELMetrics` devuelve instrumentos sin inicializar si se deshabilitan, pero el middleware se instala igualmente y llama métodos sobre ellos. Es un riesgo de panic que debe cubrirse con un test; usar instrumentos no-op reales o no instalar el middleware.
6. **Dos salidas de logs:** Zap escribe a stdout y también envía OTLP. Eso no es duplicación en Loki si solo se ingiere OTLP, como pretende ese Compose; **sí lo sería** si Kubernetes recogiera stdout y además se ingiriera el mismo evento por OTLP.
7. **Seguridad y sampling:** la referencia usa OTLP sin TLS y `AlwaysSample`; son decisiones de laboratorio, no defaults universales para producción. No copiar credenciales ni datos de su `.env`.
8. **Versiones y configuración:** usa imágenes `latest` y Collector Contrib `0.91.0`, con un exporter específico de Loki y convenciones antiguas. No asumir que esa configuración funcione sin cambios en versiones nuevas. Fijar versiones compatibles y validar la configuración real; Loki soporta actualmente ingesta OTLP/HTTP nativa.
9. **Observabilidad de infraestructura:** Prometheus, Loki, Collector y Grafana también necesitan límites, métricas de salud, persistencia y acceso controlado. Un Compose experimental no equivale a operación de producción.

Estas son conclusiones por lectura de código/configuración, no resultados de una ejecución. La investigación de compatibilidad concreta y las pruebas pertenecen a la implementación posterior.

## 5. Componentes y responsabilidades

| Componente | Responsabilidad | No es |
|---|---|---|
| OpenTelemetry API/SDK | Crear y procesar spans y métricas; definir recursos y contexto | Un backend de almacenamiento |
| OTLP | Transportar señales OTEL entre componentes | Sinónimo de OpenTelemetry ni requisito para todas las métricas |
| Collector Contrib | Recibir, procesar y enviar señales; recoger archivos de logs donde sea apropiado | Grafana, almacenamiento permanente ni garantía de entrega |
| Prometheus | Descubrir/scrapear targets, almacenar series, consultar con PromQL y evaluar reglas | Un sistema de logs o trazas |
| Loki | Ingerir, almacenar y consultar logs | Un sustituto de la instrumentación de la aplicación |
| Tempo | Almacenar y consultar trazas | Un generador automático de spans |
| Grafana | Dashboards, Explore, correlación y visualización | El almacén de las señales |

La instrumentación OTEL no obliga a enviar métricas por OTLP. Un exporter Prometheus compatible puede exponer instrumentos OTEL por HTTP para que Prometheus los recoja. Los nombres, unidades y labels **reales** dependen de la versión y traducción elegidas: el dashboard debe probarse contra el endpoint real, no contra nombres supuestos.

## 6. Tempo o Jaeger

| Criterio | Tempo | Jaeger |
|---|---|---|
| Función | Backend de trazas, búsqueda y consulta | Backend y herramientas de consulta de trazas |
| OTLP | Compatible | Compatible; Jaeger v2 se basa en componentes del ecosistema Collector |
| Experiencia | Natural con Grafana/Explore, Loki y Prometheus | UI y experiencia especializadas en trazas; Grafana también puede integrarse |
| Operación | Perfil local sencillo; perfiles durables requieren diseñar almacenamiento y retención | Perfil local sencillo; el backend de almacenamiento y su operación dependen del despliegue elegido |
| En este proyecto | Una sola interfaz Grafana y coherencia con el diseño existente | Alternativa válida si la experiencia propia de Jaeger aporta valor suficiente |

**Recomendación: Tempo.** Ya queremos Grafana y Loki, y el diseño existente lo contempla. No porque Jaeger sea malo ni porque Tempo sea obligatorio para OTEL, sino porque simplifica la experiencia y evita mantener dos backends de trazas para el MVP. El contrato de la app seguirá siendo OTLP hacia el Collector; cambiar Tempo por Jaeger más adelante debe ser una decisión de plataforma, no un rewrite del dominio.

No ejecutar Jaeger y Tempo en paralelo por defecto. Una comparación temporal o una migración justificada son excepciones explícitas; duplicar exportación aumenta coste y dificulta saber cuál es la fuente de verdad.

## 7. Arquitectura recomendada por entorno

### 7.1 Contrato común de la aplicación

```text
HTTP / DB / operaciones   -> OTEL SDK -> OTLP traces  ┐
Métricas HTTP y runtime   -> OTEL SDK -> OTLP metrics ├─> Collector -> Tempo / Prometheus / Loki
Zap (+ contexto de span)  -> bridge otelzap -> OTLP logs ┘
Zap                       -> stdout (consola, kubectl logs; no se ingiere)
```

Consecuencias: no hace falta `/metrics`, ServiceMonitor, agente de logs por nodo ni acceso al socket de Docker. Se pierde la métrica `up` por target; se compensa con la salud del Collector. El Prometheus del homelab necesita el receptor OTLP habilitado (`--web.enable-otlp-receiver`) o que el Collector escriba por remote-write. Los nombres reales cambian por la traducción OTEL → Prometheus (p. ej. `http.server.request.duration` → `http_server_request_duration_seconds_*`): los dashboards se validan contra el backend real.

Los backends no viven dentro del binario ni de la imagen. Se configuran de forma externa. La aplicación puede ejecutarse sin acceso a los backends; la telemetría encolada se perderá si se agotan los límites o no se logra exportar.

### 7.2 Local: experiencia reproducible

El repositorio generado incluye un perfil opcional de Docker Compose (`observability`) con **un único servicio**, `grafana/otel-lgtm` con versión fijada (probada: `0.39.0`):

```text
app (en el host o en contenedor) + PostgreSQL   # perfil funcional
otel-lgtm: Collector + Prometheus + Tempo
           + Loki + Grafana                     # perfil de observabilidad local
```

- Puertos publicados: Grafana `3000`, OTLP gRPC `4317`, OTLP HTTP `4318`. La app en el host usa `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`; dentro de la red de Compose, `http://<servicio>:4318`.
- **Verificado el 2026-11-01** en Docker Desktop (macOS): arranque en menos de un minuto, ~550 MB de RAM en reposo y recepción de trazas, métricas y logs vía OTLP/HTTP. Los datasources vienen provisionados, con enlace Loki → Tempo (`trace_id`) y Tempo → Loki. Esa prueba usó `telemetrygen`, no la app: **falta verificarlo con la app real**.
- El upstream declara la imagen solo para desarrollo, demos y pruebas; no es el stack del homelab ni valida HA, retención de producción ni autorización.
- Los datos son efímeros por defecto; montar `/data` es opcional. Documentar cómo limpiar solo los volúmenes del proyecto.
- Un dashboard mínimo propio (RED) se provisiona por separado, contra los nombres de métrica reales.

**Logs (decisión):** se descarta leer logs de Docker (Alloy + socket): no cubre la app ejecutada en el host y exige acceso privilegiado al daemon. Se usa el bridge `otelzap` junto a stdout con un `zapcore.NewTee`; el logger recibe el contexto del span. Riesgo conocido: la señal de logs de OTEL Go está en *release candidate* y `otelzap` (v0.21) y `otlploghttp` (v0.23) son 0.x; se fijan versiones. Plan B si se rompe: stdout + Alloy.

### 7.3 Kubernetes: una plataforma compartida

```text
Pods de aplicaciones
  └─ OTLP (traces + metrics + logs) ──> Collector compartido ──> Tempo / Prometheus / Loki ──> Grafana

Cluster / nodos / exporters ──────────> Prometheus scrape ───────────────────────────────────┘
```

- Las aplicaciones solo necesitan el endpoint OTLP del Collector (un Service). No hay ServiceMonitor ni agente de logs por nodo para las apps.
- Métricas de cluster y nodos (kube-state-metrics, node-exporter) siguen por scrape de Prometheus con el mecanismo efectivamente instalado. Si más adelante se quiere recoger stdout de workloads que no emiten OTLP, un Collector en DaemonSet con `filelog` es una adición aparte, sin duplicar lo que la app ya envía por OTLP.
- OTLP exige que Prometheus acepte escritura OTLP o que el Collector use remote-write.
- Una instalación compartida de Tempo/Loki/Prometheus/Grafana observa tanto dev-forge como las apps. En un cluster que ya tenga un stack compatible, integrarse con él en vez de instalar otro; la plataforma de ese cluster define endpoints, autenticación y políticas.
- El template declara su contrato; el despliegue de dev-forge o la configuración del cluster inyecta endpoint OTLP, atributos, puerto y descubrimiento de métricas. No hace falta una instancia por aplicación.

### 7.4 ¿Hay solapamiento entre app y cluster?

No necesariamente. El cluster observa **pods, nodos, CPU, memoria, reinicios y red**; la app conoce **la ruta HTTP, el caso de uso y la operación de DB**. Un pico de latencia puede correlacionarse con reinicios o saturación del nodo: las perspectivas se complementan.

Sí hay redundancia cuando ambos recogen exactamente la misma señal dos veces: por ejemplo dos Prometheus que scrapean el mismo target sin una razón operativa; un agente que lee stdout y un bridge que manda el mismo log a Loki; o dos middlewares que registran la misma métrica HTTP. HA, migraciones y separación de dominios pueden justificar copias, pero requieren deduplicación, costes y responsabilidades explícitos. El stack local y el del cluster son **entornos distintos**, no duplicados dentro del mismo despliegue.

## 8. Diseño de instrumentación

### 8.1 Recursos y configuración

**Stack de librerías elegido** (versiones verificadas en el proxy de Go el 2026-11-01; `go.mod` actual está en otel v1.43 y debe subirse):

| Pieza | Módulo | Versión |
|---|---|---|
| Middleware HTTP Fiber v3 (span de servidor + métricas semconv 1.43) | `github.com/gofiber/contrib/v3/otel` | v1.3.2 |
| API/SDK (trazas, métricas, logs) | `go.opentelemetry.io/otel`, `sdk`, `sdk/metric`, `sdk/log` | v1.47.0 |
| Exporters por variables de entorno estándar | `go.opentelemetry.io/contrib/exporters/autoexport` | v0.72.0 |
| Bridge Zap → OTEL logs (señal en *release candidate*) | `go.opentelemetry.io/contrib/bridges/otelzap` | v0.21.0 |
| Métricas de runtime Go | `go.opentelemetry.io/contrib/instrumentation/runtime` | v0.72.0 |
| PostgreSQL | `github.com/exaring/otelpgx` | v0.13.0 |

El middleware de Fiber va después de `recover` y antes del logger de acceso; se excluyen `/health` y `/ready` con `WithNext`. Usa la ruta con plantilla (`http.route`), por lo que no genera series por ID. `autoexport` construye los exporters a partir de `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`, `OTEL_EXPORTER_OTLP_*`, y acepta `none`: la configuración propia se reduce a lo que OTEL no define (habilitación global, atributos de entorno).

Usar recursos consistentes para las tres señales: `service.name`, `service.version` y entorno según la convención OTEL elegida. Kubernetes puede añadir namespace, nombre del workload y otros atributos mediante detección/enriquecimiento apropiados. Distinguir múltiples instancias sin convertir cada identificador efímero en label indexado por defecto.

La app debe aceptar nombre de servicio, versión de build, entorno, habilitación por señal, endpoint OTLP, protocolo, TLS/autenticación, sampling y límites. Preferir variables estándar OTEL donde estén soportadas realmente por el SDK y documentar precedencia frente a configuración propia. No prometer que una variable funciona solo por tener nombre estándar: testear su efecto.

Para OTLP, distinguir `grpc` (habitualmente puerto 4317) de `http/protobuf` (habitualmente 4318); un endpoint de red no indica por sí solo el protocolo. Con OTLP/HTTP la URL base y las URLs específicas por señal tienen reglas distintas de construcción de rutas. TLS y credenciales provienen del entorno o gestor de secretos; no incluirlas en defaults del catálogo.

### 8.2 Trazas y propagación

- Un span servidor por request relevante con método, ruta normalizada y status; evitar capturar headers o bodies por defecto.
- Propagar contexto W3C Trace Context hacia llamadas HTTP salientes y al contexto de las consultas pgx. Si hay jobs/colas en el futuro, definir también su propagación y límites de confianza.
- Un span para operaciones de negocio que realmente ayuden a diagnosticar, y spans de DB/dependencias. No abrir un span para cada función interna.
- Inicializar el provider **antes** de instalar `otelpgx` y el middleware. Comprobar con tests que HTTP → use case → DB comparten trace y la relación padre-hijo correcta.
- Sampling configurable: 100 % puede ser útil localmente; en producción se decide por coste y riesgo, idealmente parent-based. El tail sampling necesita estado y capacidad en Collector; no introducirlo como requisito inicial.
- Un trace ID válido en un log **no garantiza** que el trace esté disponible: puede no haber sido muestreado, exportado, retenido o permitido por autorización.

### 8.3 Métricas

Priorizar RED: tasa de requests, errores y distribución de latencia. Agregar métricas del runtime Go y del pool de DB cuando estén verificadas y aporten valor. Añadir métricas de negocio solo para preguntas concretas.

- Contadores e histogramas, con unidades y buckets apropiados; comprobar traducción OTEL → Prometheus y alineación de dashboards.
- Labels de baja cardinalidad: método, ruta declarada, status y dimensiones controladas. Usar un valor fijo para 404 y otros casos sin ruta.
- Nunca labels con IDs de usuario, requests, trace, emails, tokens, errores variables, URLs completas ni valores de entrada arbitrarios.
- Evitar recolectar una misma métrica HTTP mediante dos librerías o rutas de exportación. No crear un contador adicional para cada categoría que pueda derivarse de métricas fiables existentes sin una razón.
- Si en el futuro se expone `/metrics`, no lleva secretos pero puede revelar detalles operativos; restringir red y acceso. Excluir o tratar explícitamente probes (`/health`, `/ready`) para que no distorsionen RED (`WithNext` del middleware).
- Los exemplars pueden enlazar métricas con traces en sistemas compatibles; son una mejora posterior y requieren verificar soporte de extremo a extremo.

### 8.4 Logs y correlación

Zap emite una línea JSON estructurada a stdout (consola legible en debug) y, en paralelo, envía los mismos registros por OTLP mediante el bridge `otelzap`; solo la vía OTLP se ingiere en Loki. Incluye tiempo, nivel, mensaje, servicio y, cuando exista contexto de span válido, `trace_id` y `span_id`. El request ID se conserva como campo separado. Los IDs son **campos de log**, no labels indexadas de Loki.

El logger debe recibir explícitamente el contexto actual; ni Zap ni un Collector pueden deducirlo de una llamada que no se lo proporciona. Probar logs de handlers y de dependencias, incluyendo contextos sin span, para evitar suposiciones sobre bridges y propagación. Un span hijo puede requerir extraer sus IDs en el momento del log, no reutilizar siempre el logger creado para el span padre.

Configurar Grafana para extraer el trace ID y enlazar con Tempo; el parsing del JSON y el enlace deben validarse juntos. No registrar tokens, `Authorization`, cookies, contraseñas, bodies, SQL con valores, datos personales ni mensajes de error que los contengan. La redacción en Collector es defensa adicional, no permiso para emitir secretos desde la app.

### 8.5 Health y ciclo de vida

- `/health` o liveness: el proceso puede seguir ejecutándose; no depende de Tempo, Loki o Prometheus.
- `/ready`: instancia lista para aceptar tráfico, incluyendo dependencias estrictamente necesarias como DB cuando corresponda, con timeout. Una caída de observabilidad no cambia readiness.
- Al terminar: dejar de aceptar requests, esperar operaciones en curso dentro de un plazo, cerrar providers y hacer flush acotado, cerrar DB y logger. Ajustar los plazos al grace period de Kubernetes.
- En modo deshabilitado, usar una implementación no-op segura o no registrar middleware; no provocar panic.

### 8.6 Si falla el pipeline

El SDK y el Collector usan batching, límites de memoria y colas/reintentos acotados. Configuración inválida debe informarse explícitamente; la caída temporal del Collector o del backend no debe bloquear requests de negocio. La telemetría no es transaccional: puede perderse durante reinicios, saturación o caída prolongada.

Monitorizar fallos de exportación, colas, datos descartados y salud de backends y del Collector; definir un punto de observación alternativo para no depender exclusivamente de un pipeline que está caído. Colas persistentes pueden ayudar, pero no garantizan entrega exacta una sola vez; la prueba de duplicación debe definir su ventana y expectativas.

## 9. Seguridad, aislamiento y operación

1. **Acceso:** Grafana, Prometheus, Loki, Tempo y Collector no se publican sin protección. Definir TLS, autenticación, permisos de red, RBAC y exposición según entorno. TLS interno sin autenticar también requiere un modelo de confianza explícito.
2. **Aislamiento por aplicación/scope:** filtrar por labels elegidas no es autorización. El futuro módulo `observe` debe derivar los filtros autorizados en servidor y prevenir consultas arbitrarias entre apps; Grafana/backends también necesitan políticas de acceso apropiadas.
3. **Privacidad y cardinalidad:** auditar atributos, errores, paths, SQL y etiquetas. Un token puede aparecer en una URL o mensaje de error incluso si no se registran headers.
4. **Retención y almacenamiento:** homelab de una instancia con PVCs, retención y recursos limitados es útil, pero no es HA. Para un entorno durable evaluar object storage para Loki/Tempo y estrategia de persistencia/backup de Prometheus; probar restauración. Retenciones y volúmenes se dimensionan con mediciones, no con cifras universales.
5. **Alertas:** empezar con disponibilidad, errores/latencia sostenidos y salud del pipeline/recursos; no alertar por cada línea de log. Definir destinatario y procedimiento para que una alerta sea accionable.
6. **Sampling y costes:** fijar presupuesto de series, volumen de logs y trazas; revisar queries y cardinalidad antes de subir retención o tasa de muestreo.
7. **Imágenes y dependencias:** versiones fijadas y actualizadas mediante un proceso verificable, compatibles entre sí. No usar `latest` como política de releases.

Promtail está [fuera de soporte desde el 2 de marzo de 2026](https://grafana.com/docs/loki/latest/send-data/promtail/); no es la elección para un sistema nuevo. Alloy es una alternativa de recolección, no una dependencia obligatoria de Loki ni un motivo para instalar dos agentes que lean los mismos logs.

## 10. Cómo convertir dev-forge en un blueprint

El objetivo no es que cada proyecto nuevo sea una copia funcional de la plataforma, sino extraer un **chassis**:

| Incluir en el blueprint | Separar o revisar antes de copiar |
|---|---|
| Estructura modular, composition root y módulo de ejemplo pequeño | Módulos de catálogo, apps, Git y reglas propias de dev-forge |
| Fiber v3, validación, errores, OpenAPI y middleware probados | Nombre, rutas, defaults, paquetes y documentación específicos de la plataforma |
| Logger, telemetría, health, config y shutdown | URLs reales, secretos, datos de prueba y configuración del entorno del autor |
| PostgreSQL y migraciones de ejemplo si forman parte del contrato | Esquemas, seeds y datos de producto |
| CI, Dockerfile y laboratorio local con versiones fijadas | Pipelines o manifests de despliegue que compitan con dev-forge |
| Auth como capacidad definida y configurable | No copiar por defecto una configuración de Zitadel Cloud específica ni credenciales |

Definir tokens explícitos de generación como nombre de app, módulo Go y `service.name`; probar la sustitución en todos los archivos relevantes, no solo en el código. Las opciones de auth y DB deben responder a una decisión de producto: incluirlas en el scaffold no significa que la plataforma pueda aprovisionar automáticamente Zitadel o PostgreSQL.

El blueprint debe tener repo y commit propios, pruebas y gates. Según [DESIGN.md §10](docs/DESIGN.md#10-project-blueprints--scaffolds-curados-y-versionados), una versión publicada referencia un snapshot inmutable; usar el commit provisional de `dev-forge-api` en el catálogo **no crea** ese blueprint ni valida su contenido. Para la generación mediante GitHub Template Repositories hay que comprobar que el snapshot y su rama por defecto correspondan a la versión publicada.

## 11. Secuencia propuesta

Es una propuesta para revisar antes de cambiar el plan, no una afirmación de que estas tareas ya estén hechas.

| Paso | Alcance | Resultado verificable |
|---|---|---|
| 1 | Definir contrato de telemetría, contexto y configuración | Señales, seguridad, rutas locales/cluster y compatibilidad Fiber v3 decididas |
| 2 | Instrumentar `dev-forge-api`: provider, HTTP, pgx, logs, health y shutdown | Tests unitarios y HTTP; correlación y errores correctos; app operativa sin backend |
| 3 | Laboratorio local reproducible con stack y provisioning | Una request visible en métricas, trace y logs; enlace log → trace |
| 4 | Robustez, privacidad y regresiones | Tests de cardinalidad, secretos, collector caído, deshabilitación y apagado |
| 5 | Extraer el blueprint a repo propio y parametrizarlo | Proyecto generado manualmente compila, pasa tests y levanta su laboratorio |
| 6 | Integrar el blueprint versionado en catálogo y generación | Snapshot verificado, gates y generación reproducible desde el commit publicado |
| 7 | Integración en homelab | Reutilizar stack del cluster o instalarlo una sola vez; descubrimiento y seguridad probados |
| 8 | Capacidades de producto `observe` | Consultas autorizadas por app/scope, límites y pruebas de aislamiento |

Pasos 1–4 adelantan parte de 4.2–4.4 y 4.6 del plan; el paso 5 se relaciona con 4.5 y 1.6; los pasos 7–8 con 4.1 y 4.7–4.9. El progreso de esas tareas debe reflejar **solo** lo realmente comprobado en cada entorno. No es necesario implementar todo Kubernetes para empezar a instrumentar localmente.

## 12. Criterios de aceptación para «out of the box»

Se debe poder repetir desde un clon limpio, sin credenciales del autor:

1. Generar o clonar una instancia del blueprint con nombre y módulo propios, siguiendo las instrucciones incluidas; compilar, ejecutar tests y arrancar la app.
2. Levantar el perfil local de observabilidad con configuración de ejemplo y versiones fijadas. Grafana debe abrir con datasources y dashboard provisionados sin pasos manuales ocultos.
3. Enviar requests a una ruta normal y comprobar en Prometheus el conteo, status y una observación de latencia con labels esperadas. Requests con múltiples IDs y rutas 404 distintas no generan series por cada ID/path.
4. Encontrar una traza en Tempo con span HTTP y, para un flujo que realmente consulta DB, span pgx relacionado; comprobar propagación de contexto y parentesco.
5. Encontrar el log de esa request **una vez en el backend**, extraer su `trace_id` y abrir el trace correspondiente desde Grafana. Repetir con muestreo habilitado y contemplar el caso no muestreado.
6. Comprobar 2xx, 4xx, 5xx, errores devueltos por handlers y panic recuperado: status final, métricas, spans y logs coherentes.
7. Con el Collector o un backend detenido, verificar que una operación de negocio y readiness siguen funcionando dentro de un timeout; observar errores/descartes de exportación sin consumir memoria indefinidamente.
8. Verificar sin telemetría configurada o con ella deshabilitada que no hay panic y que el shutdown termina dentro del plazo.
9. Inspeccionar señales de prueba para confirmar que no contienen tokens, headers sensibles, cuerpos, credenciales ni PII; comprobar que `trace_id` no es label indexada.
10. En Kubernetes, probar descubrimiento y recolección con **dos apps** usando la infraestructura compartida, sin duplicados y con etiquetas/consultas separables. Esto requiere un cluster y es un gate distinto del laboratorio local.
11. Generar un segundo proyecto con nombre distinto y repetir las pruebas, garantizando que nombres, módulo, dashboard y datos del primer proyecto no quedan codificados.

Los unit tests cubren configuración, mapeos y propagación; una prueba integrada con Collector/backends y una prueba de cluster son necesarias para afirmar que el pipeline completo funciona. El laboratorio local no valida HA, retención de producción ni autorización multiusuario.

## 13. Decisiones y preguntas abiertas

### Decididas (2026-11-01)

| # | Tema | Decisión |
|---|---|---|
| 1 | Auth | Zitadel es el proveedor actual; el objetivo es poder cambiarlo (p. ej. a AWS Cognito) tocando solo el adaptador y variables de entorno. **Hoy no se cumple del todo:** `ValidateToken` usa introspección específica de Zitadel con key file, los roles se leen del claim `urn:zitadel:iam:org:project:roles` y `ports.Claims.ZitadelID`, `GetByZitadelID` y `users.zitadel_id` nombran al proveedor. Tarea aparte: validador OIDC/JWKS genérico, mapeo de claims por configuración y renombrar la columna a `external_id`. En el blueprint, auth es una capacidad activada por configuración. |
| 2 | Logs y stack local | Todo por OTLP (§7.1); stack local `grafana/otel-lgtm` (§7.2). Sin acceso al socket de Docker. |
| 3 | Homelab | Aún no operativo: solo se valida en local. El diseño exige que el homelab necesite únicamente variables de entorno (`OTEL_EXPORTER_OTLP_ENDPOINT` y atributos de recurso). El inventario del homelab queda pendiente hasta que exista. |
| 4 | Acceso a la telemetría | Grafana y backends sin exposición pública, solo para operadores y con SSO. Los equipos ven su telemetría únicamente mediante el módulo `observe`, que aplica en servidor los filtros por app y scope (`deployment.environment`, `service.name`). Sin multi-tenancy en los backends por ahora. |
| 5 | Dimensionado inicial | Sampling 100 % parent-based; retención trazas 3–7 d, logs 7 d, métricas 15 d; ~10 Gi Prometheus, ~10 Gi Loki, ~5 Gi Tempo; export de métricas cada 30–60 s; ~10k series activas por app. **Valores de partida** a ajustar con mediciones del homelab real. |
| 6 | Integración Fiber v3 | Opción A (§8.1): middleware oficial `gofiber/contrib/v3/otel` + `autoexport` + `otelzap` + `otelpgx`, con las últimas versiones. Descartadas: middleware propio (más mantenimiento) y auto-instrumentación eBPF (solo Linux, privilegiada). |
| 7 | Fallos de telemetría | Configuración inválida (exporter, protocolo o sampler desconocido): error explícito al arrancar. Backend caído o inalcanzable: operación normal, descarte acotado y log limitado. `OTEL_ENABLED=false`: providers no-op, sin panic. |

### Pendientes

- Verificar el stack local con la **app real** instrumentada (hasta ahora solo con `telemetrygen`): métricas, trazas HTTP→pgx, log con `trace_id` y enlace log → traza.
- Confirmar que `otelzap` propaga el contexto del span a los logs de handlers y de dependencias.
- Inventariar el homelab cuando exista (Prometheus/Operator, Grafana, Loki, Tempo, agentes) y decidir si se instala el stack con Helm o se usa el manifiesto de `otel-lgtm` como solución provisional para dev/qa.

## 14. Fuentes

**Repositorio actual**

- [Diseño de observabilidad y contrato del blueprint](docs/DESIGN.md).
- [Plan de implementación y Fase 4](DEV-FORGE-PLAN.md).
- [Composición de la API](cmd/server/main.go), [PostgreSQL y otelpgx](internal/shared/database/database.go), [middleware HTTP](internal/shared/server/middleware.go) y [health](internal/shared/server/health.go).

**Repositorio local de referencia**

- `observ-monit-go/docker-compose.yml` y `config/otel-collector/otel-collector-config.yaml`.
- `observ-monit-go/cmd/factorit/main.go` y `cmd/users-service/main.go`.
- `observ-monit-go/pkg/observability/{tracing,metrics,logger}/` y `pkg/config/prometheus/prometheus.yml`.

**Documentación oficial consultada**

- [OpenTelemetry Go: estado de las señales](https://opentelemetry.io/docs/languages/go/).
- [OpenTelemetry Collector: arquitectura](https://opentelemetry.io/docs/collector/architecture/) y [patrón agente](https://opentelemetry.io/docs/collector/deploy/agent/).
- [Configuración del exporter OTLP](https://opentelemetry.io/docs/specs/otel/protocol/exporter/).
- [Prometheus: arquitectura y modelo de recolección](https://prometheus.io/docs/introduction/overview/).
- [Grafana Tempo: introducción](https://grafana.com/docs/tempo/latest/introduction/) y [Jaeger: arquitectura](https://www.jaegertracing.io/docs/latest/architecture/).
- [Loki: ingesta de logs OTEL](https://grafana.com/docs/loki/latest/send-data/otel/) y [estado de Promtail](https://grafana.com/docs/loki/latest/send-data/promtail/).
- [Alloy: lectura de logs de Docker](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.docker/) (opción descartada, plan B).
- [gofiber/contrib: middleware OTEL para Fiber v3](https://github.com/gofiber/contrib/tree/main/v3/otel), [grafana/docker-otel-lgtm](https://github.com/grafana/docker-otel-lgtm) y [autoexport](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport).
- [Grafana: provisioning de datasources y dashboards](https://grafana.com/docs/grafana/latest/administration/provisioning/).
