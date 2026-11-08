<!-- source: alerts-and-runbook 2c201450ab32 -->
# Alertas y guía de actuación

Sobre qué deberían avisarte las métricas del propio servidor, qué urgencia tiene cada alerta y qué comprobar
y hacer cuando salta una. La vigilancia del servidor desde fuera (monitores de disponibilidad y una página de estado) está en [Vigilar el servidor desde fuera](monitoring.md);
configúrala también, porque un servidor caído no envía ninguna métrica.

<!-- source: getting-the-metrics 58d8055fe44d -->
## Obtener las métricas

El servidor exporta sus métricas cuando `metrics_exporter` está definido ([referencia de configuración](../reference/configuration.md#metrics_exporter));
la [guía de ejecución](running.md) enumera todas las métricas y sus etiquetas. Las alertas siguientes están escritas para Prometheus, y los nombres son
los que ve Prometheus: los puntos se convierten en guiones bajos y se añaden los sufijos de unidad y de contador
(`my.queue.oldest_ready_age` es `my_queue_oldest_ready_age_seconds`, `my.delivery.attempts` es `my_delivery_attempts_total`).

| Destino | Configuración | Alertas |
| --- | --- | --- |
| Autoalojado (Docker Compose, Kubernetes, una VM) | `metrics_exporter = prometheus` | Prometheus consulta `GET /metrics` en `metrics_listen`, Alertmanager enruta las alertas. Las reglas de abajo funcionan tal cual |
| Cualquier destino con un OpenTelemetry Collector | `metrics_exporter = otlp` | El Collector reenvía al backend que elijas. Un backend compatible con Prometheus que añada los sufijos de unidad y de tipo (como hace por defecto el receptor OTLP del propio Prometheus) ve los mismos nombres |
| Google Cloud (Cloud Run) | `metrics_exporter = none` | Políticas de alertas de Cloud Monitoring sobre una comprobación de disponibilidad, las métricas de la plataforma y métricas basadas en registros (consulta [Cloud Run](#google-cloud-cloud-run)) |
| AWS (ECS) | `metrics_exporter = emf` | Alarmas de CloudWatch sobre las métricas que extrae CloudWatch Logs (consulta [AWS ECS](#aws-ecs)) |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus y Alertmanager

Las reglas están en [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml). Copia el archivo junto a tu
`prometheus.yml` y cárgalo:

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` vale por defecto `127.0.0.1:9464`, al que solo llega un Prometheus que esté en el mismo host. En Docker Compose o Kubernetes, define
  `MY_METRICS_LISTEN=0.0.0.0:9464` y mantén el puerto en la red privada: el listener no tiene autenticación.
- Consulta todos los procesos (cada proceso `role = web` y `role = worker` tiene su propio listener). Los indicadores de la cola, los archivos multimedia, el tamaño de la base de datos,
  los servidores remotos, la importación de facturación y la instantánea llevan el mismo valor para toda la instancia en cada proceso, así que las reglas toman su máximo.
- Cada regla tiene una etiqueta `severity`: **`page`** necesita a alguien ahora (hay usuarios afectados o los habrá pronto), **`ticket`** puede esperar al horario laboral.
  Enrútalas a receptores distintos en Alertmanager. El `runbook_url` de cada regla apunta a su sección de abajo.
- Los umbrales son puntos de partida para un servidor pequeño. Observa los valores durante una semana y ajusta los umbrales y las duraciones `for` a tu tráfico.
  `MyRemoteMediaCacheOverLimit`, `MyMediaStorageLarge` y `MyDatabaseStorageLarge` llevan tamaños que debes ajustar a tus propios límites.
- Durante el modo de mantenimiento el servidor no sirve métricas y responde `503`, así que saltan `MyTargetDown` y las alertas de HTTP.
  Silencia las alertas en Alertmanager durante la ventana antes de empezar ([Maintenance](maintenance.md#steps)).

Después de cambiar las reglas, comprueba el archivo y ejecuta sus pruebas unitarias (`alerts.test.yml`) con `promtool`, que viene con Prometheus:

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud (Cloud Run)

Con `metrics_exporter = none`, construye las alertas a partir de lo que Cloud Run y Cloud Logging ya tienen:

- **Errores HTTP y latencia**: el recuento de peticiones de Cloud Run por clase de código de respuesta y la métrica de latencia de peticiones del servicio.
- **Errores**: las líneas `error report` van a Error Reporting, que las agrupa y puede avisarte de errores nuevos y recurrentes ([Informe de errores](error-reporting.md#google-cloud-cloud-run)).
- **Trabajos y peticiones**: el servidor escribe una línea de registro por petición HTTP (`msg` = `http request`) y por ejecución de trabajo (`msg` = `job`), con las
  mismas etiquetas que las métricas. Crea métricas basadas en registros a partir de ellas, por ejemplo un contador de `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  para los trabajos fallidos, o una distribución de `jsonPayload.duration_seconds` filtrada por `jsonPayload."my.surface"="client_api"` para la latencia.
- **Acumulación en la cola**: con `queue = cloudtasks` los trabajos están en la base de datos, así que la pantalla de trabajos y las líneas de registro de las ejecuciones muestran la acumulación;
  la métrica de profundidad de cola de Cloud Tasks cuenta las señales que esperan a ser entregadas.
- **Base de datos**: las métricas de uso de disco y de conexiones de Cloud SQL, o las alertas de uso de tu proveedor de base de datos.
- **Costos**: un presupuesto de Cloud Billing con umbrales de alerta ([Costos y presupuesto](#cost-and-budget)).

El módulo [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) las crea como políticas con los nombres de la tabla de abajo, y enumera cada alerta que cubre de otra manera o que no puede expresar. Nunca se ha aplicado. Una métrica que solo produce el tráfico tiene huecos en un servicio que escala a cero, así que una alerta por su ausencia salta en falso; su comprobación de disponibilidad es lo que detecta un servidor inactivo que ha caído.

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

Con `metrics_exporter = emf`, CloudWatch Logs convierte las líneas del servidor en métricas en el espacio de nombres `metrics_emf_namespace`, con los nombres
con puntos (`my.job.duration`, `my.queue.oldest_ready_age`). Las métricas de duración de peticiones y de trabajos proceden de las líneas de registro de peticiones y de trabajos, y las
dimensiones son las etiquetas que enumeras en `metrics_emf_dimensions`; incluye `my.outcome` para crear alarmas sobre trabajos fallidos y `http.response.status_code`
para crear alarmas sobre 5xx. Los indicadores de toda la instancia (cola, archivos multimedia, tamaño de la base de datos, servidores remotos, importaciones de facturación, instantánea) los escribe el trabajo periódico de instantánea. Las métricas del runtime de Go (`go.*`)
no se escriben en EMF; usa en su lugar las métricas de CPU y de memoria del servicio de ECS.

- **Errores HTTP**: las métricas de 5xx y de recuento de peticiones del Application Load Balancer.
- **Acumulación en la cola**: `my.queue.oldest_ready_age`, que escribe el trabajo de instantánea.
- **Base de datos**: las métricas de almacenamiento y de conexiones de RDS o Aurora, o las alertas de uso de tu proveedor de base de datos.
- **Correo electrónico** (`mail = ses`): las métricas de reputación de SES (tasas de rebotes y de quejas) en CloudWatch.
- **Costos**: un presupuesto de AWS Budgets con alertas ([Costos y presupuesto](#cost-and-budget)).

El módulo [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) las crea como alarmas con los nombres de la tabla de abajo, con la misma lista. Nunca se ha aplicado.

<!-- source: the-alerts d76842ca02b8 -->
## Las alertas

Gravedad: **page** = actuar ya, **ticket** = durante el horario laboral. «–» en la columna de la regla significa que todavía no hay una métrica para ella; compruébala como se describe.

| Área | Alerta | Señal | Umbral | Gravedad | Causas probables |
| --- | --- | --- | --- | --- | --- |
| Cola | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | más de 5 min durante 10 min | ticket | Pocos workers para una ráfaga (una publicación popular, un relay), trabajos lentos, un worker que se detuvo |
| Cola | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | más de 30 min durante 5 min | page | No se ejecuta ningún worker (se cayó, no se desplegó, solo `role = web`), ningún worker ejecuta un tipo de trabajo, workers bloqueados por la base de datos |
| Cola | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | más de 10 por hora durante 15 min | ticket | Un error en un trabajo, un adaptador mal configurado (correo, blob, facturación), un servicio remoto que rechaza |
| Cola | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | parte de la cola más de 5 min, parte de los totales más de 1 h, durante 15 min | ticket | El almacén KV no es accesible, las consultas a la base de datos de la instantánea fallan o agotan el tiempo |
| Federación | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_delivery_attempts_total` con `my_outcome` `retry` o `failed` | más del 50% de los intentos durante 30 min | ticket | Muchos seguidores en servidores caídos, un servidor grande caído, problemas de red de salida |
| Federación | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | igual | más del 95% de los intentos durante 15 min | page | Red de salida o DNS averiados, salida bloqueada, claves del servidor o `host` incorrectos |
| Federación | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 5 más que el día anterior | ticket | Servidores que se cerraron o que bloquean a este, un problema de red de este lado que duró días |
| Federación | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | más del 25% de los POST durante 30 min | ticket | Falla la verificación de firmas (desfase del reloj, fallan las obtenciones de claves), un spammer o un dominio suspendido que reintenta, límites de frecuencia |
| Federación | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | más del 10% de los POST durante 30 min | ticket | El reloj del servidor está desajustado, falla la obtención de las claves de los remitentes, un servidor que cambió sus claves |
| Federación | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | más del 5% de los POST durante 15 min | ticket | La base de datos falla, un error en el buzón de entrada |
| Errores | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | más de 10 en 15 min y 3 × la media del día anterior, durante 15 min | ticket | Un error nuevo de una versión, una dependencia que falla de una forma nueva, una configuración rota |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | 5xx de `http_server_request_duration_seconds_count` | más del 5% durante 10 min | page | La base de datos o el KV caídos, una versión defectuosa, el modo de mantenimiento |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | igual | más del 1% durante 30 min | ticket | Una ruta que falla, un adaptador que falla de vez en cuando |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | p95 de la API de cliente (sin streaming) | más de 300 ms durante 30 min | ticket | Base de datos saturada o lejana, una consulta lenta, poca CPU, arranques en frío |
| Recursos | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 durante 5 min | page | El proceso está caído, en bucle de fallos o en modo de mantenimiento; la vía de consulta está rota |
| Recursos | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | cualquier aumento durante 5 min | page | La base de datos no es accesible o el esquema no está aplicado |
| Recursos | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 3 reinicios en una hora | ticket | Terminado por falta de memoria (OOM), un panic, una sonda de actividad que falla |
| Recursos | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | más del 90% durante 15 min | ticket | `GOMEMLIMIT` demasiado bajo para la carga, muchos flujos abiertos, una fuga |
| Recursos | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | más del 90% durante 15 min | ticket | Pool demasiado pequeño para el tráfico, consultas lentas que retienen conexiones |
| Recursos | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`, `db_client_connection_timeouts_total` | espera de más de 0,5 s por segundo, o cualquier timeout, durante 10 min | ticket | Igual que arriba; se alcanzó el límite de conexiones de la propia base de datos |
| Recursos | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | más del límite de tu plan menos un margen (ejemplo 8 GiB) durante 1 h | ticket | Crecimiento de las publicaciones y del contenido remoto en caché; es el momento de ampliar el plan o acortar la retención |
| Recursos | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | más del 80% durante 15 min | ticket | Muchos flujos y conexiones abiertos, un `ulimit -n` bajo |
| Recursos | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | más del 5% de las operaciones de un adaptador durante 15 min | ticket | Fallan Valkey, el directorio de blobs o el backend de flujos |
| Recursos | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | más de 1,2 × `remote_media_cache_max_bytes` durante 3 h | ticket | La limpieza horaria no se ejecuta o falla |
| Recursos | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | más del tamaño previsto (ejemplo 50 GiB) durante 1 h | ticket | Crecimiento de las subidas; es el momento de ampliar el plan o el presupuesto |
| Correo electrónico | [`MyEmailFailing`](#myemailfailing) | `my_job_duration_seconds_count` de los trabajos de correo con `my_outcome="dead"` | cualquiera en una hora | ticket | Configuración de SMTP o SES incorrecta, una cuenta suspendida o sin cuota, direcciones rechazadas de forma definitiva |
| Correo electrónico | [`MyEmailRetrying`](#myemailretrying) | igual, `my_outcome="retry"` | más del 50% de las ejecuciones durante 15 min | ticket | El servidor de correo no es accesible o rechaza el inicio de sesión, un límite de frecuencia de envío |
| Costos | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 durante 6 h | ticket | Faltan permisos, la exportación de facturación no está configurada, se alcanzó el tope de peticiones, el proveedor no está disponible |
| Costos | Presupuesto superado | – | – | – | Tráfico, almacenamiento o una mala configuración que cuestan más de lo previsto ([más abajo](#cost-and-budget)) |

<!-- source: where-to-look 9ef1d37c4f05 -->
## Dónde mirar

Estas herramientas reaparecen en la mayoría de las entradas de la guía de actuación de abajo.

- **La pantalla de trabajos** (Administración → Trabajos, `/admin/jobs`, requiere `manage_jobs`): los recuentos por estado (en cola, en ejecución, reintentando, fallidos) y por tipo, y
  el trabajo en espera más antiguo; su página **Trabajos** (`/admin/jobs/list`) enumera los trabajos de un estado, y la página de cada trabajo tiene su último error y los intentos que no tuvieron éxito. Los trabajos pendientes solo se enumeran para las colas que se guardan en la base de datos
  (`queue = db` y `cloudtasks`); los trabajos y los intentos fallidos se enumeran para todas las colas.
- **El comando `my job`** en un host que tenga la configuración del servidor (`queue = db` y `cloudtasks`):
  `my job list` enumera los trabajos fallidos, los más recientes primero; `my job retry <id>...` los devuelve a la cola; `my job delete <id>...` los descarta.
  Los trabajos se pueden ejecutar de nuevo sin riesgo.
- **La pantalla de servidores remotos** (Moderación → Servidores remotos, `/moderation/instances`, requiere `manage_domains`): los servidores a los que las entregas no pudieron llegar,
  y aquellos cuyas entregas están en pausa, con el motivo y el momento. «Reanudar envíos» vuelve a marcar un servidor como disponible.
- **Los registros**: líneas JSON en la salida de error estándar. Cada petición HTTP termina con una línea cuyo `msg` es `http request`, y cada ejecución de un trabajo con una línea cuyo `msg` es `job`
  (`INFO` si tiene éxito, `WARN` si se reintenta, `ERROR` si el trabajo falló definitivamente, con `error`, `job_id` y `my.job.type`).
  Las líneas dentro de una petición llevan su `request_id`, que el cliente ve como `X-Request-Id`.
  - Docker Compose: `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging: `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights: ``filter msg = "job" and `my.outcome` = "dead"``
- **Informes de errores**: las líneas `error report` y la pantalla de Errores (Administración → Errores, `/admin/errors`, requiere `manage_errors`) ([Informe de errores](error-reporting.md)).
- **Trazas**, cuando `traces_exporter = otlp`: las líneas de registro llevan `trace_id`; ábrelo en tu backend de trazas para ver dónde pasó el tiempo una petición lenta.
- **El modo de mantenimiento** ([Mantenimiento](maintenance.md)): responde a todas las peticiones con una página de mantenimiento y no toca ningún adaptador. Úsalo mientras reparas la base de datos
  o el almacén KV.

<!-- source: runbook e60e811fcff0 -->
## Guía de actuación

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

Los trabajos (entregas, notificaciones, correos electrónicos, distribución a las líneas de tiempo) esperan más de 5 minutos a un worker.

Comprueba:

- La pantalla de trabajos: qué tipos están en cola y si muchos se están reintentando. Una ráfaga de un solo tipo (`federation.deliver` tras una publicación con muchos seguidores,
  actividades de relay) se despeja sola; comprueba que la antigüedad baja.
- Que los procesos worker se ejecutan (`role = all` o `role = worker`) y que sus registros muestran líneas `job`.
- `MyDBPoolSaturated` o `MyAPILatencyHigh` al mismo tiempo: los workers esperan a la base de datos.

Soluciona:

- Ejecuta más procesos worker (`role = worker`), o da más CPU a los existentes.
- Si un tipo de trabajo es lento, búscalo en las líneas de registro `job` (`duration_seconds`) y busca errores de ese tipo.

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

El trabajo listo más antiguo lleva esperando más de 30 minutos: no se está procesando nada, o casi nada.

Comprueba:

- Que al menos un proceso se ejecuta con `role = all` o `role = worker`, y que no está en bucle de fallos (`MyProcessRestarting`).
- Los registros del worker, en busca de errores que se repiten en cada trabajo (base de datos, KV, un panic).
- `MyNotReady`: un worker que no puede llegar a la base de datos no puede tomar trabajos.
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`: si un tipo espera y los demás no, comprueba que algún worker lo ejecuta ([Splitting workers by job type](running.md#splitting-workers-by-job-type)).

Soluciona:

- Inicia o reinicia los workers. Los trabajos que retenía un worker detenido vuelven a la cola cuando expira su concesión; se pueden ejecutar de nuevo sin riesgo.
- Si la causa es la base de datos, sigue [MyNotReady](#mynotready).

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

Más de 10 trabajos por hora fallan definitivamente: agotaron los reintentos, o el trabajo se negó a ejecutarse de nuevo.

Comprueba:

- La pantalla de trabajos, filtrada por Fallidos: los tipos y el último error. Los intentos de un trabajo muestran cada error por el camino.
- Los mismos errores en las líneas de registro `job` con `my.outcome` = `dead`.

Soluciona:

- Un error de configuración (correo, blob, importación de facturación, un destino de webhook): corrígelo y después vuelve a ejecutar los trabajos desde la pantalla de trabajos («Ejecutar de nuevo») o con `my job retry <id>...`.
- Las entregas a servidores que ya no existen fallan definitivamente tras unos 4 días de reintentos; es lo esperado y no requiere nada.
- Los trabajos fallidos que nadie reintenta se eliminan tras `dead_job_retention` (30 días por defecto). Elimina con `my job delete` los que no vayas a reintentar.
- Un error que parece un fallo del software: infórmalo con el tipo de trabajo y el error (no contiene datos personales).

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

Los indicadores de la cola y del almacenamiento proceden de una instantánea que los procesos comparten a través del almacén KV. Es antigua, así que las alertas de la cola y del almacenamiento
ven valores antiguos y pueden permanecer en silencio.

Comprueba:

- Los registros, en busca de errores sobre la instantánea de métricas o el almacén KV.
- `MyAdapterErrors` para `kv.valkey` o `kv.db`, y el estado de la base de datos.

Soluciona:

- Repara el almacén KV o la conexión con la base de datos. La instantánea se calcula de nuevo en la siguiente recolección; el trabajo periódico de instantánea también se ejecuta cada 15 minutos.

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

Más de la mitad de los intentos de entrega a otros servidores fallan. Algunos fallos son normales: los servidores desaparecen, y este servidor sigue reintentándolos durante unos 4 días.

Comprueba:

- La pantalla de servidores remotos: si los fallos se concentran en pocos servidores (están caídos; no hay nada que hacer) o se reparten entre muchos (sospecha de este servidor).
- `http_client_request_duration_seconds_count` por `error_type`: timeouts, errores de DNS o `forbidden_destination` (la protección SSRF rechazó el destino).

Soluciona:

- Fallos en pocos servidores: nada. Tras fallos en 7 días distintos, las entregas a ese servidor se ponen en pausa, y el servidor se sondea a diario para reanudarlas.
- Fallos en todas partes: sigue [MyDeliveryAllFailing](#mydeliveryallfailing).

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

Casi todas las entregas fallan. Casi siempre es cosa de este lado del servidor.

Comprueba:

- Desde el host o el contenedor del servidor, que el HTTPS de salida funciona (DNS, cortafuegos de salida, NAT, un proxy).
- Las líneas de registro `job` de `federation.deliver` para ver el error: errores de TLS, timeouts o `401` de muchos servidores (no pueden verificar la firma de este servidor).
- Tras un cambio de dominio o de claves: `host` debe seguir igual una vez que el servidor se ha federado, y la KEK debe ser aquella con la que se sellaron las claves
  ([Gestión de la KEK](key-management.md)).

Soluciona:

- Repara la red de salida. Los intentos fallidos se reintentan con espera creciente (1 minuto, 5 minutos, 30 minutos, 2 horas, etc.), así que las entregas se ponen al día solas.
- Si las entregas agotaron los reintentos, vuelve a ejecutar los trabajos `federation.deliver` fallidos desde la pantalla de trabajos.
- Si se pusieron en pausa servidores remotos durante la interrupción, se reanudan con la siguiente entrega correcta, petición firmada o con el sondeo diario; también puedes pulsar
  «Reanudar envíos» en la pantalla de servidores remotos.

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

Se pusieron en pausa las entregas a 5 o más servidores en el último día. Un servidor se pone en pausa cuando las entregas a él fallaron en 7 días distintos, así que unas pocas pausas a la semana
son normales: los servidores se cierran. Muchas a la vez significan que muchos servidores fallaron durante días al mismo tiempo. `my_federation_instances` con
`my_federation_instance_state` = `available` cuenta los servidores a los que todavía se entrega pero que fallaron algún día; un salto ahí precede a las pausas.

La métrica no tiene etiqueta por servidor (las etiquetas nunca llevan un dominio remoto). Comprueba:

- La pantalla de servidores remotos: la pestaña «En pausa» enumera los servidores en pausa, con cuándo y por qué. Si comparten algo (un proveedor de alojamiento, un software)
  o están dispersos.
- `MyDeliveryFailuresHigh` y `MyDeliveryAllFailing` durante la última semana: un problema de red de este lado que duró días pone en pausa a muchos servidores.

Soluciona:

- Servidores que ya no existen: nada. Los servidores en pausa se sondean a diario y se reanudan solos cuando responden.
- Tras un problema de este lado: repáralo y después pulsa «Reanudar envíos» para los servidores que te importen, o espera al sondeo diario.
- Un servidor cuyo operador informa de que bloquea a este: nada de lo que hagas de este lado ayuda. Para dejar de federarte con un servidor a propósito, usa una acción sobre dominios
  (Moderación → Medidas de dominio).

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

Se rechaza más de una cuarta parte de las actividades que otros servidores envían por POST a los buzones de entrada.

Comprueba:

- `my_inbox_activities_total{my_outcome="rejected"}` por `my_inbox_rejection`: `signature` (sigue [MyInboxSignatureFailures](#myinboxsignaturefailures)),
  `suspended` (un dominio suspendido que sigue reintentando), `federation_off` (la federación está desactivada), `rate_limited` (una IP de cliente o un servidor por encima del
  límite de frecuencia) o `invalid` (actividades sin un `id`, `type`, `actor` u `object` válidos).
- Las líneas de registro `http request` de las rutas del buzón de entrada (`my.surface` = `activitypub`) llevan la misma división por `http.response.status_code`
  (`401`, `403`, `429`, `400`) y la dirección del cliente, cuando necesitas saber de dónde vienen las peticiones.

Soluciona:

- Los rechazos de un dominio suspendido, o con la federación desactivada, no requieren nada: se rechazan según lo previsto.
- Peticiones limitadas por frecuencia de un solo servidor: un spammer o una ráfaga; suspende el dominio si continúa.
- Actividades no válidas de un solo servidor: el software de ese servidor envía algo que este servidor no lee; infórmalo con el software y la versión del servidor.

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

Se rechaza más del 10% de las actividades que otros servidores envían por POST a los buzones de entrada porque falta su firma o su prueba de integridad o no se verifica,
o porque no se pudo obtener la clave del remitente.

Comprueba:

- El reloj del servidor: las firmas se rechazan fuera de una ventana de tiempo. Comprueba NTP en el host o en el runtime de contenedores.
- La obtención de las claves de los remitentes: `MyDeliveryAllFailing` y `http_client_request_duration_seconds_count{my_http_purpose="json"}` por `error_type`
  (la red de salida, DNS, la protección SSRF).
- Si los rechazos proceden de muchos servidores o de uno (las líneas `http request` con estado `401`): un servidor que cambió sus claves, o que firma
  de una forma que este servidor no acepta, afecta solo a ese servidor.

Soluciona:

- Corrige el reloj o la red de salida. Otros servidores reintentan sus entregas, así que no se pierde nada si se corrige en un día más o menos.
- Un servidor que falla: infórmalo al operador de ese servidor, o a este proyecto con el software y la versión del servidor.

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

El buzón de entrada responde `500` a más del 5% de las actividades. Los remitentes reintentan, pero las actividades llegan tarde.

**Comprueba** las líneas de registro `http request` y de errores de las rutas del buzón de entrada para ver el error, y el estado de la base de datos (`MyNotReady`, `MyDBPoolWaiting`).

**Soluciona** la base de datos o informa del error como un fallo del software. No hace falta reproducir nada: los remitentes reintentan.

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

El servidor informa de errores (panics, los errores detrás de los 5xx, trabajos fallidos) a más del triple de su frecuencia habitual.

Comprueba:

- Las líneas `error report` de la última hora, agrupadas por `my.error.fingerprint`: las más frecuentes, su `my.error.where` y la línea de encabezado de `message`.
  En Cloud Run, Google Cloud Error Reporting muestra los mismos grupos.
- Si una versión, un cambio de configuración o una dependencia que falla (`MyNotReady`, `MyAdapterErrors`) empezaron al mismo tiempo.
- Las líneas de petición o de trabajo con el mismo `request_id` o `job_id`.

Soluciona:

- Una dependencia que falla o un error de configuración: corrígelo; los trabajos fallidos se pueden volver a ejecutar después desde la pantalla de trabajos.
- Una versión defectuosa: vuelve a la imagen anterior.
- Un fallo del software: infórmalo con el encabezado y la pila de la línea. Ambos están depurados de datos personales.

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

Más del 5% de todas las peticiones responden 5xx. Los usuarios ven errores.

Comprueba:

- Si estás en el modo de mantenimiento (se espera `503`; silencia la alerta la próxima vez).
- `MyNotReady` (la base de datos), `MyAdapterErrors` (KV, blob, flujos) y una versión reciente.
- Las líneas `http request` con `http.response.status_code` ≥ 500 por `http.route`, y las líneas de error con el mismo `request_id`.

Soluciona:

- Repara la dependencia que falla. Si la base de datos necesita trabajo, cambia al modo de mantenimiento para que los usuarios vean una página clara en lugar de errores.
- Una versión defectuosa: vuelve a la imagen anterior.
- Informa a los usuarios ([Informar a los usuarios de una incidencia](#telling-users-about-an-incident)) si se prolonga.

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

Más del 1% de las peticiones responden 5xx durante media hora. Normalmente una ruta o un adaptador.

**Comprueba** las líneas `http request` con un 5xx por `http.route`, y las líneas de error de esas peticiones. **Soluciona** la dependencia, o informa del error como un fallo del software.

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

El percentil 95 de la API de cliente supera los 300 ms (el objetivo para una página de la línea de tiempo de inicio de 20 publicaciones, en caliente). El endpoint de streaming se deja fuera.

Comprueba:

- `http_server_request_duration_seconds` por `http_route`: una ruta lenta, o todas.
- La base de datos: `MyDBPoolWaiting`, la CPU de la base de datos y su registro de consultas lentas, la distancia de red entre el servidor y la base de datos.
- El proceso: limitación de CPU, `MyMemoryNearLimit` (un GC bajo presión de memoria), `go_schedule_duration_seconds` (goroutines que esperan una CPU).
- Las trazas de las peticiones lentas, si están activadas.

Soluciona:

- Da más recursos a la base de datos o al servidor, o colócalos más cerca.
- Los arranques en frío tras escalar a cero elevan el percentil en un servidor tranquilo; es lo esperado.

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus no puede consultar un proceso.

Comprueba:

- Que el proceso se ejecuta: `docker compose ps`, `kubectl get pods` o la consola de la plataforma. Sus últimas líneas de registro dicen por qué se detuvo.
- Si está en el modo de mantenimiento (no se sirven métricas; es lo esperado).
- Que `metrics_exporter = prometheus` y que `metrics_listen` es accesible desde Prometheus.

Soluciona:

- Inicia el proceso de nuevo. Si no arranca, el registro dice por qué: un error de configuración (`my config check` lo muestra), la base de datos inaccesible, un esquema sin aplicar.

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

Falla la comprobación de disponibilidad: la base de datos no es accesible, o sus tablas no coinciden con esta versión. El balanceador de carga deja de enviar tráfico al proceso,
y el endpoint de salud externo informa `unavailable`.

Comprueba:

- Las líneas de registro de la comprobación de disponibilidad: un error de conexión (red, credenciales, la base de datos caída o sin conexiones), o las tablas que faltan.
- La página de estado y la consola del proveedor de la base de datos: ¿está activa, sin almacenamiento o en mantenimiento?
- Tras una actualización: ¿se aplicó el esquema antes de que arrancara la nueva versión ([Inicializar la base de datos](database.md))?

Soluciona:

- Base de datos caída o inaccesible: restáurala, o haz una conmutación por error. Mientras trabajas, el modo de mantenimiento muestra a los usuarios una página clara.
- Base de datos sin almacenamiento: amplía el plan o libera espacio ([MyDatabaseStorageLarge](#mydatabasestoragelarge)).
- Esquema sin aplicar: ejecuta `my schema apply` de la misma versión, y los procesos pasan a estar listos por sí solos.
- Datos perdidos o dañados: restaura desde tu copia de seguridad y conserva la KEK con la que se sellaron las claves de la copia ([Gestión de la KEK](key-management.md)).

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

Un proceso se reinició 3 veces en una hora.

Comprueba:

- El motivo del reinicio según la plataforma: `OOMKilled` (memoria), una sonda de actividad fallida o el código de salida.
- Las últimas líneas de registro antes de cada reinicio, en busca de un panic o de un error fatal de configuración.

Soluciona:

- Sin memoria: define `GOMEMLIMIT` en torno al 90% del límite de memoria del contenedor, o da más memoria al contenedor ([MyMemoryNearLimit](#mymemorynearlimit)).
- Un panic: infórmalo con las líneas de registro que lo rodean.
- El registro dice que la base de datos pertenece a otro `host` o `account_domain`: define en la configuración los valores registrados, o apunta `database_url` a la base de datos correcta. Estos valores no se pueden cambiar ([Settings you cannot change later](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

El runtime de Go usa más del 90% de `GOMEMLIMIT`. Cerca del límite, el recolector de basura se ejecuta con mucha más frecuencia, lo que cuesta CPU y latencia,
y pasado el límite la plataforma puede terminar el proceso. Esta alerta necesita `GOMEMLIMIT`; sin él, vigila en su lugar las métricas de memoria del contenedor.

**Comprueba** `http_server_active_requests` y los flujos abiertos (`my_stream_connections`), y si la memoria sigue creciendo mientras la carga se mantiene estable (una fuga).

**Soluciona** aumentando la memoria del contenedor y `GOMEMLIMIT` con ella, o ejecutando más procesos. Informa de un crecimiento constante que no siga la carga.

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

Más del 90% de las conexiones de base de datos de un proceso están en uso. Pronto las peticiones esperarán una conexión (`MyDBPoolWaiting`).

**Comprueba** si el tráfico creció o las consultas se volvieron más lentas (el registro de consultas lentas de la base de datos, `my_adapter_operation_duration_seconds`).

Soluciona:

- Aumenta el tamaño del pool en `database_url` (en PostgreSQL, `pool_max_conns`), manteniendo el total de todos los procesos por debajo del límite de conexiones de la base de datos.
- Pon un agrupador de conexiones delante de la base de datos si la comparten muchos procesos.

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

El trabajo espera una conexión libre con la base de datos, o dejó de esperar tras [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (los timeouts son solo de PostgreSQL): las peticiones responden `503` y los trabajos se ejecutan de nuevo más tarde. Sigue [MyDBPoolSaturated](#mydbpoolsaturated);
comprueba también que la propia base de datos no ha alcanzado su límite de conexiones.

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

La base de datos ocupa más de lo que habías previsto. `my_db_size_bytes` es el tamaño de la base de datos PostgreSQL (`pg_database_size`, lo que cuentan los límites de almacenamiento
de los proveedores) o del archivo SQLite (sin contar su registro de escritura anticipada). Esto es crecimiento, no un fallo. Define el umbral de la regla con un margen por debajo del límite de almacenamiento
de tu plan de base de datos, o del volumen que contiene el archivo SQLite, que también debe dejar espacio para el registro de escritura anticipada y para las copias de seguridad.

Soluciona:

- Amplía el plan o el volumen.
- Las publicaciones remotas se eliminan tras la configuración de la instancia `remote_status_retention_days`; reducirlo ralentiza el crecimiento.
- Las filas eliminadas liberan espacio que ambas bases de datos reutilizan, pero el tamaño rara vez baja: PostgreSQL conserva el espacio de sus tablas, y SQLite conserva las páginas
  libres en el archivo hasta un `VACUUM` (ejecútalo con `sqlite3` con el servidor detenido).

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

El proceso tiene abiertos más del 80% de su límite de descriptores de archivo. Pasado el límite, las conexiones nuevas fallan.

**Soluciona** aumentando el límite (`ulimit -n`, el `nofile` del runtime de contenedores) o ejecutando más procesos. Comprueba `my_stream_connections` y
`http_client_open_connections` para ver qué los retiene.

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

Falla más del 5% de las operaciones de un adaptador: `kv.valkey` (Valkey), `kv.db` (la tabla KV), `blob.fs` (el directorio multimedia),
`queue.db` (la tabla de la cola) o un `stream.<backend>` (las notificaciones entre procesos).

**Comprueba** las líneas de error de ese adaptador y el servicio que hay detrás: Valkey activo y accesible, el volumen multimedia montado, con permiso de escritura y no lleno.

**Soluciona** el servicio. Las operaciones de trabajos fallidas se reintentan; las notificaciones de flujo fallidas solo retrasan las actualizaciones en directo.

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

Las copias de archivos multimedia remotos se mantienen por encima de `remote_media_cache_max_bytes`. La limpieza horaria normalmente elimina las copias caducadas y las menos usadas recientemente.
Define el umbral de la regla con tu valor de `remote_media_cache_max_bytes`.

**Comprueba** en la pantalla de trabajos y en los registros los fallos de los trabajos de limpieza, y `MyAdapterErrors` para el almacenamiento de blobs.

**Soluciona** la causa de los fallos, o reduce el tamaño de la caché o la política de archivos multimedia remotos de la instancia.

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

Los archivos multimedia ocupan más de lo que habías previsto. Esto es crecimiento, no un fallo. Define el umbral de la regla con lo que permitan tu plan de almacenamiento o tu presupuesto.

**Soluciona** ampliando el plan o el presupuesto, o reduciendo los límites de subida (`media_max_image_bytes`, `media_max_av_bytes`).

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

Hubo trabajos de correo electrónico que fallaron definitivamente en la última hora: verificación, restablecimiento de contraseña, avisos de contraseña, notificaciones o los correos de cierre.
Los usuarios que esperan un correo de verificación o de restablecimiento no pueden terminar.

**Comprueba** la pantalla de trabajos filtrada por los tipos de trabajo de correo (terminan en `_email`) y los últimos errores. [Envío de correo electrónico](email.md#look-at-send-failures)
enumera los errores y lo que significan.

Soluciona:

- Host, puerto o credenciales de SMTP incorrectos, o un problema de cuenta o de permisos de SES: corrígelo y después vuelve a ejecutar los trabajos fallidos.
  Los correos de verificación y de restablecimiento más antiguos que la vida útil de su enlace se completan sin enviarse cuando se reintentan, así que pide a esos usuarios que soliciten uno nuevo.
- Las direcciones rechazadas de forma definitiva se suprimen; no hay nada que hacer.

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

La mayoría de los envíos de correo electrónico fallan y esperan un reintento: el servidor de correo no es accesible, rechaza el inicio de sesión o limita la frecuencia. Los reintentos duran unos 15 minutos para
los correos transaccionales y unas 4 horas para las notificaciones antes de fallar definitivamente. Sigue [MyEmailFailing](#myemailfailing) antes de que lo hagan.

<!-- source: cost-and-budget 5a63bdd2754d -->
### Costos y presupuesto

El servidor no avisa sobre los costos. Define un presupuesto con alertas en el proveedor, que ve primero los cargos:

- AWS: un presupuesto de costos de AWS Budgets con alertas, por ejemplo, al 80% y al 100% del importe mensual, filtrado por la etiqueta de asignación de costos de este servidor.
- Google Cloud: un presupuesto de Cloud Billing para el proyecto, con umbrales de alerta al 50%, al 90% y al 100%. Las notificaciones del mismo presupuesto pueden alimentar la
  importación de costos ([Importar los cargos de uso de la nube](billing-import.md)).

Los cargos importados y la estimación del mes hasta la fecha aparecen en Administración → Costos → Importaciones de facturación. Las importaciones fallidas se registran allí y las reintenta la siguiente importación diaria;
[`MyBillingImportFailing`](#mybillingimportfailing) te avisa antes de que las cifras del mes queden obsoletas.

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

La importación más reciente de los cargos de uso de una fuente de facturación falló, y ninguna importación posterior ha tenido éxito en 6 horas. Las importaciones se ejecutan una vez al día; la limitación de frecuencia y
la falta de disponibilidad se reintentan en cuestión de horas, y los demás fallos esperan a la importación del día siguiente, así que la pantalla de costos muestra cifras más antiguas hasta entonces.

Comprueba:

- Administración → Costos → Importaciones de facturación: la última importación de la fuente y su error.
- `my_billing_imports_total{my_outcome="failed"}` por `error_type`, o la línea de registro `WARN` cuyo `msg` es `billing import failed`, con la `source` y el
  `error_kind`. [Importar los cargos de uso de la nube](billing-import.md) enumera los tipos y cómo solucionar cada uno.

Soluciona:

- Corrige la causa (permisos, la exportación de facturación, el tope de peticiones) y después pulsa «Importar ahora» en Administración → Costos → Importaciones de facturación en lugar de esperar al día siguiente.

Sin Prometheus (Cloud Run con `metrics_exporter = none`), crea en su lugar una alerta basada en registros sobre la línea `billing import failed`.

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## Informar a los usuarios de una incidencia

1. **Confirma recepción** de la alerta para que los demás de guardia sepan que alguien se está ocupando.
2. **Publica en la página de estado** (`status_page_url`, consulta [Vigilar el servidor desde fuera](monitoring.md#a-status-page)) en cuanto haya usuarios afectados:
   qué está roto, desde cuándo y cuándo volverás a publicar. La página de estado sigue siendo accesible cuando este servidor no lo es, y el cliente web enlaza a ella.
3. **Publica un anuncio** si los usuarios aún pueden llegar al servidor (degradado, no caído): Administración → Anuncios, escribe el texto y publícalo.
   Los anuncios aparecen sobre la línea de tiempo de inicio de todos. Usa en su lugar el modo de mantenimiento con un `maintenance_message` si el servidor debe detenerse.
4. **Actualiza** ambos a la hora que prometiste, aunque no haya cambiado nada.
5. **Cierra**: marca la incidencia como resuelta en la página de estado, retira el anuncio (o publica un breve seguimiento) y anota lo que ocurrió
   y lo que cambiaste mientras lo tienes fresco.
