<!-- source: maintenance 46ba28c229dd -->
# Mantenimiento

Cómo poner el servidor fuera de servicio para trabajar en la base de datos, el KV o la cola, y qué mostrar mientras está caído.

Hay dos herramientas:

- **El modo de mantenimiento** (`maintenance = true`, `MY_MAINTENANCE=true` o `my serve -maintenance`). `my serve` arranca sin abrir ningún adaptador
  y responde a cada solicitud con un `503`, un `Retry-After` y una respuesta que entiende cada cliente. Úsalo siempre que `my serve` pueda ejecutarse.
- **La página exportada** (`my maintenance export`). HTML estático de la misma plantilla, para que la sirva un balanceador de carga, una CDN o un proxy inverso
  cuando `my serve` no puede ejecutarse en absoluto (una imagen que se está reemplazando, una revisión que no arranca). Consulta [Exportar la página de mantenimiento](admin-cli.md#exporting-the-maintenance-page).

<!-- source: what-the-maintenance-mode-answers 05dd30158a8e -->
## Qué responde el modo de mantenimiento

El proceso lee solo la dirección de escucha, `host`, `account_domain`, `web_origin`, los proxies (`trusted_proxies`, `trusted_proxy_header`),
la [comprobación de origen](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) (`origin_secret`, `origin_secret_previous`, `origin_secret_header`), `status_page_url` y las configuraciones `maintenance_*` ([referencia de configuración](../reference/configuration.md)).
Las configuraciones de la base de datos, la cola, el almacén de blobs, el KV, el correo y el stream no se leen ni se comprueban, así que pueden ser erróneos o inaccesibles mientras tanto.
Sea cual sea el `role`, sirve solo HTTP: sin trabajos, sin trabajos periódicos, sin broker de stream.
`my config check -maintenance` comprueba exactamente lo que lee el modo de mantenimiento.

Con [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) definido, una solicitud que necesita la cabecera y no la tiene recibe el `403` de esa comprobación en lugar de lo que indica la tabla siguiente.

| Solicitud | Estado | Cuerpo |
| --- | --- | --- |
| `GET /healthz`, `GET /readyz` | `200` | `{"status":"maintenance"}` |
| `GET /api/v1/instance/health` | `503` | `{"status":"maintenance"}` |
| El resto de la API de cliente (`/api/*`) y los endpoints de OAuth que responden JSON | `503` | `application/problem+json` de tipo `/api/v1/problems/maintenance`, con `message`, `until` y `status_page_url` cuando está definido |
| ActivityPub (por ruta, `Accept` o `Content-Type`), WebFinger, host-meta, NodeInfo, los webhooks de pago, las llamadas de Cloud Tasks y Cloud Scheduler (`/_internal/*`) | `503` | `{"error":"maintenance"}` |
| Cualquier otra cosa (páginas, el cliente web, sus recursos, envíos de formularios) | `503` | La página de mantenimiento, o el problema cuando `Accept` pide JSON |

Cada respuesta lleva `Retry-After` y `Cache-Control: no-store`. `Retry-After` cuenta los segundos hasta `maintenance_until` mientras esa fecha esté por llegar
(como máximo 86 400), y en caso contrario es `maintenance_retry_after` (una duración de Go, `10m` de forma predeterminada).

La página y el problema muestran:

| Configuración | Se muestra como |
| --- | --- |
| `maintenance_server_name` | El nombre del servidor. El nombre de host si no está definido (el nombre propio de la instancia está en la base de datos) |
| `maintenance_message` | Un aviso en texto plano, de 2000 caracteres como máximo, igual en todos los idiomas |
| `maintenance_until` | Cuándo se espera que termine el trabajo (RFC 3339). La página lo muestra en UTC; el cliente web, en la zona horaria del usuario |
| `status_page_url` | Un enlace a la página de estado ([Monitorización](monitoring.md#a-status-page)) |

Las configuraciones `maintenance_*` pueden quedar definidas durante el funcionamiento normal: solo se usan mientras `maintenance` es true.
Los textos de la página siguen `Accept-Language` (inglés en caso contrario).

El cliente web muestra un aviso de mantenimiento a pantalla completa cuando la API responde con el problema de mantenimiento,
mantiene la sesión iniciada del usuario y reintenta después de `Retry-After`.

<!-- source: how-long a7b6a44f0269 -->
## Cuánto tiempo

Mantén un mantenimiento **en unas pocas horas**. Otros servidores reintentan sus entregas ante un `503`, pero ninguno lee `Retry-After`,
y cuando se agotan sus reintentos las actividades enviadas entretanto se pierden para este servidor.
La ventana de reintentos más corta entre las principales implementaciones es de aproximadamente un día (Akkoma, cerca de 1 día; Misskey, cerca de 1,5 días; Mastodon, cerca de 2 a 3 días; Pleroma, cerca de 7 días,
según sus calendarios de reintentos).
Los motores de búsqueda tratan un `503` breve como una interrupción temporal, pero Google desaconseja servir `503` para `robots.txt` durante más de un día o dos.
Una parada de más de un día o dos no es un mantenimiento: cierra el servicio ([Cerrar el servicio](service-closure.md)).

<!-- source: steps 1427d2c6d499 -->
## Pasos

1. **Anúncialo.** Publica la ventana en la página de estado (`status_page_url`) y como anuncio (Administración → Anuncios) con algo de antelación.
   Define `maintenance_message` y `maintenance_until` en la configuración del despliegue, y actualiza las páginas exportadas
   (`my maintenance export`) si el front end las sirve.
2. **Detén la cola push** (Cloud Tasks): consulta [más abajo](#push-queues). De lo contrario, cada entrega durante la ventana
   es un `503` que consume uno de los intentos de la tarea.
3. **Cambia al modo de mantenimiento**: vuelve a desplegar con `MY_MAINTENANCE=true` (según el destino, más abajo). Cada réplica se reinicia en el modo de mantenimiento;
   el balanceador de carga las conserva porque `/readyz` responde `200`.
4. **Haz el trabajo.**
5. **Vuelve atrás**: vuelve a desplegar sin `MY_MAINTENANCE` (o con `false`), comprueba que `/readyz` y `/api/v1/instance/health` responden `{"status":"ok"}`,
   y después reanuda la cola push y cierra el anuncio.

![Qué responde el servidor antes, durante y después del modo de mantenimiento, por tipo de solicitud: como de costumbre antes y después, y 503 con Retry-After durante él, salvo /healthz y /readyz, que siguen en 200. La lista siguiente dice lo mismo.](images/maintenance.svg)

De izquierda a derecha:

- **Antes**, mientras anuncias el mantenimiento, cada solicitud se responde como de costumbre: `/healthz`, `/readyz` y
  `/api/v1/instance/health` responden `200` con `{"status":"ok"}`.
- **En el modo de mantenimiento** (`MY_MAINTENANCE=true`):
  - `/healthz` y `/readyz` responden `200` con `{"status":"maintenance"}`.
  - `/api/v1/instance/health` responde `503` con `{"status":"maintenance"}`.
  - El resto de la API de cliente y los endpoints de OAuth que responden JSON reciben `503` con el problema de mantenimiento.
  - ActivityPub, WebFinger y NodeInfo responden `503` con `{"error":"maintenance"}`.
  - Las páginas y el cliente web reciben `503` con la página de mantenimiento.
  - Cada respuesta lleva `Retry-After` y `Cache-Control: no-store`.
- **Después** de volver a desplegar sin `MY_MAINTENANCE`, cada solicitud se responde de nuevo como de costumbre.

<!-- source: push-queues 0d0d1fc56a29 -->
### Colas push

- **Cloud Tasks**: pausa la cola, que sigue aceptando tareas nuevas pero deja de entregarlas, y reanúdala después del trabajo.
  Cuántos intentos tiene una tarea lo define `retryConfig.maxAttempts` de la cola.

  ```sh
  gcloud tasks queues pause my-jobs --location=asia-northeast1
  gcloud tasks queues resume my-jobs --location=asia-northeast1
  ```

  Pausa también el trabajo de Cloud Scheduler de `/_internal/tick` (`gcloud scheduler jobs pause` / `resume`), o deja que sus llamadas fallen con `503` mientras tanto.
  Los trabajos en sí están en la base de datos: una tarea que agotó sus intentos pierde solo su señal, y el primer tick tras el trabajo señala
  de nuevo todos los trabajos pendientes.

<!-- source: per-target 4e060394be4b -->
## Según el destino

Las páginas de error de los front ends que siguen responden a los errores del servidor con la página exportada. Haz coincidir solo los errores de un servidor que no se está ejecutando
(`502` y `504`, y el `503` propio del balanceador de carga donde no hay ningún backend sano), o activa la regla solo mientras `my serve` no pueda ejecutarse:
una regla sobre cada `503` también reemplazaría las respuestas propias del modo de mantenimiento, como el problema que leen la API de cliente y el cliente web.

<!-- source: cloud-run b728619281de -->
### Cloud Run

Despliega una revisión nueva con la variable y quítala después:

```sh
gcloud run services update my --region=asia-northeast1 \
  --update-env-vars=MY_MAINTENANCE=true,MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
gcloud run services update my --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE,MY_MAINTENANCE_UNTIL
```

Cuando Cloud Run está expuesto directamente, sin balanceador de carga, no hay nada delante de él, así que el modo de mantenimiento es la única herramienta.

**Cuando `my serve` no puede ejecutarse** detrás de un Application Load Balancer externo global: sube el `index.html` exportado a un bucket de Cloud Storage
añadido como bucket de backend, y añade una política de respuesta de error personalizada que responda con él a los `502` y `504` del backend, con `overrideResponseCode: 503`.
Solo el Application Load Balancer externo global tiene respuestas de error personalizadas (no los regionales ni los clásicos), y la política se aplica solo
a las respuestas del backend, no a las que genera el propio Google Front End. Esta vía no puede enviar `Retry-After`: las cabeceras personalizadas de la fuente del error no se añaden.

```yaml
# Part of the URL map (gcloud compute url-maps import).
defaultCustomErrorResponsePolicy:
  errorService: https://www.googleapis.com/compute/v1/projects/<project>/global/backendBuckets/my-maintenance
  errorResponseRules:
    - matchResponseCodes: ["502", "504"]
      path: /index.html
      overrideResponseCode: 503
```

<!-- source: aws-ecs 8c6aa496b517 -->
### AWS ECS

Registra una revisión de la definición de tarea del servicio con `MY_MAINTENANCE=true` en el entorno del contenedor, actualiza el servicio a ella
y haz lo mismo sin la variable después. Con los módulos de OpenTofu, `deploy/opentofu/upgrade.sh --platform aws-ecs --maintenance`
cambia el servicio al modo de mantenimiento antes de una actualización, y lo devuelve con la nueva revisión.

**Cuando `my serve` no puede ejecutarse** detrás de CloudFront: sube el `index.html` exportado a un bucket de S3 que sea un segundo origen
(la página de error no puede venir del origen que falla), y añade respuestas de error personalizadas para `502` y `504` (y `503` solo mientras `my serve` no pueda ejecutarse)
con `ResponsePagePath: /maintenance/index.html` y `ResponseCode: 503`.
CloudFront puede añadir `Retry-After` mediante una política de cabeceras de respuesta con una cabecera personalizada, pero la política se aplica a cada respuesta del comportamiento de caché,
así que adjúntala solo durante el mantenimiento y sepárala después.

La respuesta fija de un Application Load Balancer no puede llevar la página exportada: su cuerpo es de 1024 caracteres como máximo y no define cabeceras como `Retry-After`.
Úsala solo para un texto corto y el estado:

```json
{
  "Type": "fixed-response",
  "FixedResponseConfig": {
    "StatusCode": "503",
    "ContentType": "text/plain",
    "MessageBody": "This server is under maintenance. Please try again later. / メンテナンス中です。しばらくしてからお試しください。"
  }
}
```

Sin `Retry-After`, los demás servidores reintentan según su propio calendario, que la ventana anterior ya contempla.

<!-- source: docker-compose d564c0b39ae8 -->
### Docker Compose

`deploy/docker/compose.yaml` pasa las configuraciones de `.env` al servidor, y toma también `MY_MAINTENANCE`, `MY_MAINTENANCE_MESSAGE` y `MY_MAINTENANCE_UNTIL` del shell.
Defínelos y recrea el servicio `my`; después quita `MY_MAINTENANCE` y recréalo de nuevo tras el trabajo.
`--no-deps` evita que se ejecuten los servicios de base de datos de una sola vez (`upgrade-check`, `schema`, `upgrade`); una actualización sí los ejecuta ([Actualizar](database.md#upgrading)):

```sh
MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z docker compose up -d --no-deps --force-recreate my
docker compose up -d --no-deps --force-recreate my
```

**Cuando `my serve` no puede ejecutarse** detrás de nginx: sirve las páginas exportadas desde un directorio, elige una según `Accept-Language`,
y responde con ella, como un `503`, a los propios `502` y `504` de nginx (el servidor está caído o no responde).
Los `503` propios del modo de mantenimiento pasan sin cambios, ya que `proxy_intercept_errors` sigue desactivado.

```nginx
map $http_accept_language $maintenance_page {
    default  en.html;
    ~*^ja    ja.html;
}

server {
    # ...
    location / {
        proxy_pass http://my:8080;
        error_page 502 504 =503 /maintenance/$maintenance_page;
    }
    location /maintenance/ {
        internal;
        alias /srv/maintenance/;   # the -out directory of my maintenance export
        add_header Retry-After 600 always;
        add_header Cache-Control no-store always;
    }
}
```

<!-- source: microk8s-kubernetes a831623b2454 -->
### microk8s (Kubernetes)

Define la variable en el Deployment, lo que despliega los pods de nuevo, y quítala después:

```sh
microk8s kubectl set env deployment/my MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
microk8s kubectl rollout status deployment/my
microk8s kubectl set env deployment/my MY_MAINTENANCE- MY_MAINTENANCE_UNTIL-
```

La sonda de disponibilidad en `/readyz` sigue pasando en el modo de mantenimiento, así que el despliegue se completa y el Service sigue dirigiendo el tráfico a los pods.

**Cuando `my serve` no puede ejecutarse** detrás del NGINX Ingress Controller (el complemento `ingress` de microk8s): sirve las páginas exportadas desde un pequeño
Deployment de `nginx` (por ejemplo desde un ConfigMap con `index.html`, `en.html` y `ja.html`) y conviértelo en el backend predeterminado personalizado,
de modo que el controlador responda desde él a los `502` y `504` del servidor (añade `503` solo mientras `my serve` no pueda ejecutarse):

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/custom-http-errors: "502,504"
    nginx.ingress.kubernetes.io/default-backend: my-maintenance   # a Service in the same namespace
```

<!-- source: monitoring-during-the-maintenance 91e64028447e -->
## Monitorización durante el mantenimiento

- `/healthz` y `/readyz` siguen en `200` con `{"status":"maintenance"}`: los balanceadores de carga y Kubernetes conservan las instancias.
- `/api/v1/instance/health` responde `503` con `{"status":"maintenance"}` ([Monitorización](monitoring.md)).
  Los monitores de disponibilidad ven el servidor como caído a menos que comprueben el cuerpo o tengan una ventana de mantenimiento; páusalos o define la ventana.
