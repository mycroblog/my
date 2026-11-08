<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# Monitorizar el servidor desde fuera

Cómo apuntar un monitor de disponibilidad al servidor, cómo evitar que la monitorización aumente el costo de funcionamiento
y cómo distinguir el modo de mantenimiento ([Mantenimiento](maintenance.md)) de una caída. Las rutas de las sondas están en
[la sección de comprobaciones de estado de la guía de ejecución](running.md#health-checks).

Los datos de terceros que aparecen abajo llevan la fecha en que se confirmaron y su fuente. Los servicios cambian sus planes y sus pantallas; comprueba la página enlazada antes de fiarte de un límite.

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## Qué URL monitorizar

| Ruta | Para | Toca la base de datos | En modo de mantenimiento |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | Monitores de disponibilidad, páginas de estado, el cliente web | Como máximo una vez cada 60 s por proceso | `503` con `Retry-After` y `{"status":"maintenance"}` |
| `GET /healthz` | La sonda de vida (liveness) de la plataforma | Nunca | `200` `{"status":"maintenance"}` |
| `GET /readyz` | La sonda de disponibilidad (readiness) del balanceador de carga o de Kubernetes | En cada petición (las peticiones simultáneas comparten una comprobación) | `200` `{"status":"maintenance"}` |

Apunta el monitor externo a **`/api/v1/instance/health`**, no a `/readyz`: `/readyz` consulta la base de datos en cada petición,
así que un monitor con un intervalo corto mantiene despierta una base de datos que escala a cero. Las sondas son para tu plataforma.
Los módulos [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) y [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) comprueban `/readyz` desde el monitor propio de la plataforma con un intervalo largo, y sus README indican cuánto cuesta.

Respuestas de `/api/v1/instance/health` (sin autenticación, `Cache-Control: no-store`):

| Estado | HTTP | Cuerpo |
| --- | --- | --- |
| Sano | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| No disponible (la base de datos no es alcanzable o no se ha aplicado el esquema) | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| Modo de mantenimiento | `503` con `Retry-After` | `{"status":"maintenance"}` |

`/healthz` y `/readyz` responden `200` con `{"status":"ok"}` cuando todo está sano, y `/readyz` responde `503` con un documento de problema cuando la base de datos no está lista.
El cuerpo nunca incluye un motivo, un nombre de host ni una versión. La causa está en el registro del servidor.

`checked_at` es el momento de la comprobación en la que se basa la respuesta. El servidor ejecuta la comprobación de la base de datos como máximo una vez por minuto y por proceso y reutiliza el resultado,
así que un monitor que consulta con más frecuencia que cada 60 s ve el mismo `checked_at` y no gana nada. Con varios procesos, cada uno ejecuta su propia comprobación.

Con [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) definido, estas tres rutas siguen respondiendo sin el encabezado, así que no muestran que la CDN lo haya perdido. Monitoriza también una página a través de la CDN, como `https://<host>/about`, si un monitor es tu primera señal de problemas.

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## Mantener barata la monitorización

- **Bases de datos que escalan a cero.** Si el intervalo del monitor es más corto que el tiempo que la base de datos espera antes de detenerse, la base de datos nunca se detiene.
  Neon suspende un cómputo tras 5 minutos sin actividad; el plan Free no permite desactivarlo
  (confirmado el 2026-10-25, [Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)).
  Como regla general, un intervalo de 5 minutos o más deja que una base de datos inactiva se detenga entre comprobaciones, y cada comprobación la despierta durante la comprobación más el tiempo de inactividad.
  Un servidor federado recibe entregas de otros servidores a todas horas, así que su base de datos está despierta buena parte del día de todos modos;
  el costo adicional se limita a las horas tranquilas.
- **Prefiere una división más barata si lo que más importa es el costo en reposo.** Monitoriza `/healthz` (nunca toca la base de datos) con un intervalo corto, y `/api/v1/instance/health`, que usa la base de datos, con uno largo (15 minutos o más). `/healthz` también responde `200` durante el modo de mantenimiento, así que busca `"status":"ok"` en el cuerpo.
- **Las peticiones en sí son pequeñas.** Una comprobación cada 5 minutos son unas 8640 peticiones al mes, una cada minuto unas 43 200. Cabe en el nivel gratuito
  de Cloud Run (2 millones de peticiones, 180 000 vCPU-segundos y 360 000 GiB-segundos al mes; tiempo redondeado hacia arriba a 100 ms; confirmado el 2026-10-23, [precios de Cloud Run](https://cloud.google.com/run/pricing)).

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## Distinguir el modo de mantenimiento de una caída

Durante el modo de mantenimiento ([guía de mantenimiento](maintenance.md)) el endpoint de estado responde `503`, que la mayoría de los monitores tratan como una caída.
El código de estado por sí solo no permite distinguirlos, así que decide según el cuerpo:

- Trata `"status":"ok"` en el cuerpo como sano y cualquier otra cosa como caído. Un monitor de palabra clave o de valor JSON lo hace.
- Para evitar alertas durante el mantenimiento programado, define la ventana de mantenimiento del monitor si el servicio la tiene (consulta la tabla de abajo),
  o pausa el monitor a mano antes de empezar.
- El plan Free de UptimeRobot no tiene ventana de mantenimiento: pausa el monitor o acepta la alerta.

<!-- source: setting-up-the-monitors e91307d78a94 -->
## Configurar los monitores

Todos comprueban lo mismo: `GET https://<your-host>/api/v1/instance/health` está sano cuando la respuesta es `200` y el cuerpo contiene `"status":"ok"`.

| Servicio | Tipo de monitor | Sano cuando | Ventana de mantenimiento | Confirmado |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | El cuerpo contiene `"status":"ok"` | No en Free (Solo y superiores) | 2026-10-25, [precios](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status o Keyword | `2xx`, o el cuerpo contiene `"status":"ok"` | Sin verificar | 2026-10-25 |
| Gatus | Condiciones del endpoint | `[STATUS] == 200` y `[BODY].status == ok` | `maintenance` en la configuración | 2026-10-25, [README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword o JSON Query | El cuerpo contiene `"status":"ok"`, o `status` es `ok` | Integrada | 2026-10-23, [README](https://github.com/louislam/uptime-kuma) |
| Upptime | `sites` en `.upptimerc.yml` | `200` y el cuerpo contiene `"status":"ok"` | Ninguna | 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

Crea un monitor de tipo **Keyword**: URL `https://<your-host>/api/v1/instance/health`, palabra clave `"status":"ok"`, alerta cuando la palabra clave **no** existe.
El plan Free permite 50 monitores con un intervalo de 5 minutos e incluye monitores de palabra clave.
«Custom headers & statuses» y las ventanas de mantenimiento empiezan en el plan Solo (de pago, intervalo de 60 segundos)
(confirmado el 2026-10-25, [precios](https://uptimerobot.com/pricing.md)). Aquí basta un monitor de palabra clave, porque la comprobación es sobre el cuerpo.

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

Funcionan dos formas (confirmado el 2026-10-25, [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/), [Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)):

- Un monitor de **HTTP status**: trata una respuesta `2xx` como activo, así que `200` es activo y `503` (no disponible o mantenimiento) abre un incidente.
- Un monitor de **Keyword**: «Alert us when» es «URL doesn't contain keyword», palabra clave `"status":"ok"`. La coincidencia no distingue mayúsculas de minúsculas.

La frecuencia de comprobación y las ventanas de mantenimiento del plan Free no están verificadas: las páginas oficiales no las indican para el plan Free. Las cantidades del nivel gratuito (10 monitores y heartbeats juntos, una página de estado) proceden de la [página de precios](https://betterstack.com/pricing), confirmadas el 2026-10-23.

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

En `config.yaml` (intervalo predeterminado `60s`; confirmado el 2026-10-25, [README](https://github.com/TwiN/gatus)):

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus tiene una sección `maintenance` de nivel superior (`start` como `hh:mm`, `duration`, `timezone` con `UTC` por defecto, `every` como lista de días de la semana, vacía significa todos los días)
para trabajos programados. El README solo dice que sirve para evitar alertas durante las ventanas de mantenimiento; comprueba el comportamiento en tu versión.

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

Añade un monitor de tipo **HTTP(s) - Keyword** con la palabra clave `"status":"ok"`, o **HTTP(s) - Json Query** con la consulta `status` y el valor esperado `ok`.
Los códigos de estado aceptados son `200-299` por defecto, así que el `503` de mantenimiento y el `503` de caída cuentan ambos como caídos. Uptime Kuma tiene una función de mantenimiento
para ventanas de monitores (la [wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) la menciona; sus detalles no están verificados).
Los tipos de monitor se confirmaron el 2026-10-23 en el código fuente ([monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720),
[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)).

<!-- source: upptime 90c96c946d06 -->
### Upptime

En `.upptimerc.yml` (confirmado el 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/)):

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

Por defecto `20x` y `30x` cuentan como activo; define `expectedStatusCodes: [200]` para aceptar solo `200`. La comprobación se ejecuta desde GitHub Actions con una programación definida para todos los sitios
(`workflowSchedule.uptime`, por defecto cada 5 minutos, que también es lo más corto que permite GitHub Actions). El nombre de la opción dice que no se recomienda,
pero es la forma de comprobar el cuerpo.

<!-- source: a-status-page 64712ed6d8a5 -->
## Una página de estado

Define `status_page_url` (`MY_STATUS_PAGE_URL`, una URL `https:`; consulta la [referencia de configuración](../reference/configuration.md#status_page_url))
con la página pública de tu monitor. El servidor muestra el enlace en `GET /api/v1/instance`, en las páginas de inicio de sesión y de error que renderiza, en el cliente web
(también cuando el servidor no responde) y en la página de mantenimiento, de modo que un usuario que no puede llegar al servidor pueda averiguar el motivo.
Se lee de la configuración, no de la base de datos, así que funciona cuando la base de datos está caída. Un cambio surte efecto tras reiniciar.

Opciones para la página en sí (este servidor no importa el estado de una página de estado, así que publica también los anuncios como aviso para todo el servidor):

- La página de estado alojada del servicio de monitorización que elegiste (el plan Free de UptimeRobot tiene una página de estado sin personalización; el nivel gratuito de Better Stack tiene una),
  las páginas que sirven por sí mismos Gatus y Uptime Kuma, o el sitio estático de Upptime en GitHub Pages.
- Atlassian Statuspage, que es un lugar donde publicar y no un monitor. Un componente tiene uno de cinco estados: Operational, Under maintenance, Degraded performance,
  Partial outage, Major outage (valores de la API `operational`, `under_maintenance`, `degraded_performance`, `partial_outage`, `major_outage`;
  confirmado el 2026-10-23, [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/), [Statuspage API](https://developer.statuspage.io/)).
  Cómo cambiar automáticamente el estado de un componente a partir de la alerta de un monitor no está verificado (requiere una integración por servicio configurada en una cuenta real).
