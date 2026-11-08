<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# Poner un CDN delante de la entrega de archivos multimedia

Un CDN delante del servidor (Cloudflare, Google Cloud CDN, CloudFront, etc.) puede entregar los archivos multimedia (`/media/` y `/media/proxy/`).
Las solicitudes que se responden desde la caché del CDN no consumen CPU ni transferencia del servidor.
Esta guía cubre la configuración en la que el servidor es el origen del CDN (origin pull). Se aplica a cualquier almacén de blobs: con `blob = s3` o `blob = gcs` el servidor
lee los archivos del bucket privado y los sirve bajo `/media/` igual que con `blob = fs`, así que el bucket nunca es el origen del CDN.
Cuando todo el servidor está detrás del CDN, también puede [rechazar las solicitudes que no han pasado por él](#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: configure-it e915ce01da94 -->
## Configurarlo

Establece `media_origin` (`MY_MEDIA_ORIGIN`) con el nombre de host del CDN (por ejemplo, `media.example.org`).

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- El valor es solo un origen (`scheme://host[:port]`), sin ruta, consulta ni barra final. Se exige `https`, y `http` se admite únicamente para hosts de loopback como `localhost`. Un valor no válido hace que el arranque falle.
- Si no se establece, los archivos multimedia se sirven desde el origen del propio servidor (`https://{host}`).
- Estas URL usan este origen: `{media_origin}/media/...` para los archivos multimedia locales y `{media_origin}/media/proxy/...` para el proxy de los archivos multimedia remotos.
  Quedan cubiertas todas las URL que emiten la API de cliente, ActivityPub (`attachment`, `icon`, `image` y el `icon` de los emoji personalizados) y las vistas previas de enlaces (OGP).
- La Content-Security-Policy del cliente web añade este origen a `img-src` y `media-src`.
- Los URI federados, como el `id` de un objeto de ActivityPub, siguen bajo `https://{host}/` sea cual sea `media_origin`. Solo cambia el lugar desde el que se sirven los archivos, no los identificadores.
- Las URL se construyen cada vez que se leen y no se guardan en la base de datos, así que cambiar `media_origin` cambia también las URL de los archivos multimedia existentes.
  Las URL antiguas que guardaron servidores remotos siguen funcionando mientras `/media/` en el origen antiguo siga sirviendo.

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## Apuntar el CDN al servidor

- Haz que el origen (backend) sea el nombre de host público del servidor (`https://{host}`). El CDN solo necesita obtener del servidor las rutas bajo `/media/`.
- El servidor no restringe `/media/` ni `/media/proxy/` por nombre de host. Sirve cualquier `Host` que envíe el CDN.
- `/media/` no requiere autenticación y la Authorized Fetch no se aplica a esa ruta. El servidor responde tal cual a las solicitudes sin firmar del CDN.
- En los archivos multimedia remotos (`/media/proxy/{id}/{variant}?sig=...`), `sig` forma parte de la URL. **Incluye la cadena de consulta en la clave de caché.** De lo contrario, las URL con firmas distintas compartirían una misma entrada.
- Deja que el `Cache-Control` del origen decida cuánto tiempo se guarda en caché (no lo sobrescribas en el CDN).

| Ruta | `Cache-Control` |
| --- | --- |
| `/media/{...}` (archivos multimedia locales) | `public, max-age=31536000, immutable` (un año) |
| `/media/proxy/{...}` (archivos multimedia remotos) | `public, max-age=` el número de segundos de la configuración de instancia `remote_media_cache_ttl_seconds` |

Ambos se devuelven con `Cross-Origin-Resource-Policy: cross-origin`, de modo que las páginas de otros orígenes pueden leerlos.

En Cloud Run, `ingress-cloudflare-media` lo configura con OpenTofu: consulta [Despliegue en Cloud Run](deploy-cloud-run.md#cloudflare).

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## Quién puede abrir las URL de archivos multimedia

Las URL de los archivos multimedia no tienen control de acceso. Cualquiera que tenga la URL de un archivo puede abrirlo, sin iniciar sesión, sea cual sea la visibilidad de la publicación a la que está adjunto.
Esto es así con o sin CDN, e incluye los adjuntos de las publicaciones solo para seguidores y de los mensajes directos, y las URL de proxy de los archivos multimedia remotos.

- Las URL son difíciles de adivinar: la URL de un archivo local contiene identificadores con una parte aleatoria, y una URL de proxy lleva una firma. Conocer una URL no revela las demás.
- Cualquiera que haya visto una publicación puede compartir las URL de sus archivos multimedia, y esas URL siguen funcionando para otras personas.
- Las respuestas se marcan como `public`, así que el CDN y cualquier caché compartida entre el servidor y quien lee pueden conservar una copia (en la tabla anterior verás cuánto tiempo).
  Esto es lo que permite a un CDN servir archivos multimedia sin consultar al servidor.

Explica a tus usuarios que una publicación no pública limita quién ve la publicación, pero no impide que quien tenga el enlace de un archivo adjunto lo abra.

<!-- source: deletion-and-the-cache baac405df923 -->
## Eliminación y caché

Después de eliminar un archivo multimedia, la caché del CDN lo conserva durante `max-age`. Un archivo eliminado puede seguir visible hasta un año en el caso de los archivos locales, y hasta `remote_media_cache_ttl_seconds` en el caso del proxy.
El servidor no purga la caché del CDN. Si algo debe desaparecer justo después de una eliminación o de una acción de moderación, purga la caché a mano en la consola o la API del CDN.

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

El Service Worker del cliente web no guarda en caché los archivos multimedia de otro origen y deja que los gestione la red del navegador, así que solo se aplica la caché del CDN.

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## Rechazar las solicitudes que no han pasado por el CDN

Un CDN solo protege al servidor de las solicitudes que pasan por él. Cuando el origen también es accesible en su propia dirección, como ocurre con `*.run.app` en Cloud Run con tráfico desde internet, cualquiera puede llamarlo directamente, esquivando el WAF y la caché.
Si las direcciones del CDN están en `trusted_proxies`, quien llame desde la propia plataforma sin servidor del CDN (por ejemplo, un Cloudflare Worker propio) también se conecta desde una dirección de confianza y puede elegir la IP de cliente que ven los límites de frecuencia.

Los tres valores de configuración siguientes cierran esa vía. El CDN añade una cabecera secreta a cada solicitud que reenvía, y el servidor responde `403` a cualquier solicitud que no la lleve, antes de hacer nada más con ella.

![Las rutas de las solicitudes con la comprobación de origen activada: los navegadores, las aplicaciones y otros servidores pasan por el CDN, que añade la cabecera secreta; la plataforma y cualquier otra persona llegan directamente al servidor, sin ella; la comprobación de origen del servidor deja pasar la cabecera y las solicitudes exentas, y rechaza el resto con 403. La lista siguiente describe cada ruta.](images/cdn-request-flow.svg)

- Los navegadores y las aplicaciones (páginas, la API de cliente, OAuth y URL de archivos multimedia) y otros servidores (entregas y consultas de ActivityPub)
  llegan al CDN en `host` y en el host de `media_origin`. El CDN responde por sí mismo a los archivos `/media/` en caché y establece
  `origin_secret_header` con el valor de `origin_secret` en cada solicitud que reenvía al servidor.
- La plataforma (sondeos de salud, monitores de disponibilidad, Cloud Tasks y Cloud Scheduler) y cualquier otra persona, como un script o un
  webhook propio, llegan directamente a la dirección del servidor, sin la cabecera.
- La comprobación de origen se ejecuta primero en el servidor. Deja pasar una solicitud que lleva la cabecera una vez con `origin_secret` o
  `origin_secret_previous` y, sin la cabecera, solo `GET` y `HEAD` de las comprobaciones de salud y `POST` a `/_internal/`.
  Elimina la cabecera de todas las solicitudes y rechaza el resto con `403`.
- Una solicitud que pasa llega al cliente web, la API de cliente, OAuth, ActivityPub, las comprobaciones de salud o `/_internal/`,
  o a `/media/` y `/media/proxy/`, que leen el almacenamiento multimedia (`blob = fs`, `s3` o `gcs`) o retransmiten el archivo de
  otro servidor. Una solicitud rechazada recibe `403` con `Cache-Control: no-store` y se suma a `my.http.origin_refused`.

Úsalo cuando el servidor tenga una dirección pública que la plataforma no pueda cerrar a todo excepto al CDN. No lo necesitas cuando un cortafuegos o la propia plataforma ya limitan quién puede conectarse, por ejemplo un servidor que escucha solo en una red privada detrás de tu balanceador de carga.
Tampoco cuando ese balanceador de carga comprueba él mismo la cabecera del CDN y admite solo las direcciones del CDN, como puede hacer un Application Load Balancer de AWS detrás de CloudFront con una regla de listener y la lista de prefijos administrada `com.amazonaws.global.cloudfront.origin-facing`. En ese caso, incluye en `trusted_proxies` tanto las direcciones del balanceador de carga como las del CDN, ya que cada uno añade su entrada a `X-Forwarded-For`.

No sirve cuando es una función de la propia plataforma la que reenvía el tráfico y no añade ninguna cabecera que controles, como una asignación de dominio de Cloud Run. Solo sirve un CDN o un balanceador de carga que pueda añadir una cabecera de solicitud.

| Configuración | Qué contiene |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret) (`MY_ORIGIN_SECRET`) | El valor, de al menos 32 bytes de ASCII visible. Es un secreto, y se lee del entorno o de `MY_ORIGIN_SECRET_FILE` |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header) (`MY_ORIGIN_SECRET_HEADER`) | El nombre de la cabecera, el que elijas. Obligatorio junto con `origin_secret`. No son válidos los nombres que un CDN se niega a establecer o que lee el servidor o un proxy, como los que empiezan por `cf-` o `x-cf-` |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous) (`MY_ORIGIN_SECRET_PREVIOUS`) | Un segundo valor aceptado, solo [durante una rotación](#rotate-the-value) |

Si no se establece ninguno, no se comprueba nada y el servidor se comporta como si esta sección no existiera.

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### Qué pasa sin la cabecera

| Solicitud | Motivo |
| --- | --- |
| `GET` o `HEAD` de `/healthz`, `/readyz`, `/api/v1/instance/health` | Los sondeos de la plataforma y los monitores de disponibilidad los llaman directamente |
| `POST` a rutas `/_internal/` | Cloud Tasks y Cloud Scheduler las llaman en la URL propia del servidor. Llevan su propio token firmado |

Todo lo demás necesita la cabecera, también en modo de mantenimiento: el cliente web y sus recursos, `/media/`, la API de cliente, OAuth y ActivityPub, incluidas las entregas de otros servidores.
Por eso se rechaza todo lo que llame al servidor directamente por su propia dirección, en lugar de pasar por el CDN: un webhook propio, un servidor multimedia de voz, un script. Dirígelo al nombre de host del CDN.

El puerto de métricas (`metrics_listen`) es un listener aparte, que no es público y no se comprueba.

Una solicitud que no lleva el secreto no influye en el resto del tratamiento, ni siquiera cuando pasa: su IP de cliente es la del par de la conexión, diga lo que diga `X-Forwarded-For`, y se ignoran su `traceparent` y su `X-Request-Id`.

<!-- source: set-it-up ee0a17ad2d33 -->
### Configurarlo

Todos los nombres de host que el CDN reenvía al servidor necesitan la cabecera: `host`, el host de `media_origin` y un `account_domain` aparte si el CDN lo sirve. Un host de archivos multimedia detrás del mismo CDN también la establece, ya que `/media/` se rechaza como todo lo demás.

1. Genera el valor y elige un nombre que no use nada más:

   ```sh
   openssl rand -hex 32
   ```

2. **Establece primero la cabecera en el CDN.** El servidor ignora una cabecera que no conoce, así que este paso aún no cambia nada.
   - **Cloudflare**: una regla de transformación de cabecera de solicitud (Rules, después Transform Rules y después Modify Request Header), disponible en todos los planes. Haz que coincida con todas las solicitudes entrantes de la zona y elige **Set static** con tu nombre y tu valor. Reemplaza un valor enviado por un cliente. Una regla que cubra solo algunos nombres de host deja bloqueados al resto.
   - **Amazon CloudFront**: una cabecera personalizada de origen en el origen de cada distribución. También reemplaza el valor de quien visita.
   - **Balanceadores de carga de Google Cloud**: una cabecera de solicitud personalizada en el servicio de backend.
   - Cualquier otro CDN o proxy: la cabecera debe establecerse, no añadirse, en cada solicitud al servidor.
3. Establece `MY_ORIGIN_SECRET` y `MY_ORIGIN_SECRET_HEADER` y despliega. `my config check` los valida y repite lo que se comprueba, pero no puede ver el CDN.
4. Comprueba desde fuera, con la dirección del propio origen:

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

Incluye también las direcciones del CDN en [`trusted_proxies`](../reference/configuration.md#trusted_proxies); si no, el servidor ve un único cliente y todas las solicitudes comparten los límites de frecuencia del CDN. `my config check` avisa de ello.

Una solicitud rechazada recibe el mismo documento de problema `403` que cualquier otra solicitud prohibida, con `Cache-Control: no-store`, sea cual sea el fallo de la cabecera. Nada en la respuesta nombra la cabecera.

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### Cuando todos quedan bloqueados

Una configuración errónea rechaza todas las solicitudes mientras las comprobaciones de salud siguen en `200`, de modo que la plataforma ve un servidor sano y un monitor de disponibilidad de `/healthz` no detecta nada. Las señales son:

- `403` en todas las páginas a través del CDN.
- La métrica `my.http.origin_refused` sube mientras las solicitudes de `http.server.request.duration` se detienen.
- Líneas `http request` en el registro con el estado `403` y sin `http.route`. El registro y las trazas nunca contienen la cabecera, su nombre ni su valor. Una solicitud rechazada no crea ningún span de traza.

Para recuperarte, despliega sin la comprobación: quita `MY_ORIGIN_SECRET` y `MY_ORIGIN_SECRET_HEADER`. Otros servidores reintentan sus entregas, pero mientras tanto las personas no podían llegar al servidor.
Después compara el nombre y el valor del CDN con la configuración, comprueba que la regla cubre todos los nombres de host y vuelve a establecerlos.

<!-- source: rotate-the-value b88f695fde31 -->
### Rotar el valor

El CDN y el servidor no pueden cambiar en el mismo instante, y cada réplica del servidor se reinicia a su ritmo, así que con un solo valor aceptado un cambio rechazaría el tráfico mientras difieran. Por eso el servidor acepta dos valores durante una rotación:

1. Establece `MY_ORIGIN_SECRET` con el valor nuevo y `MY_ORIGIN_SECRET_PREVIOUS` con el antiguo, y despliega. Ambos valores pasan.
2. Cambia el valor en el CDN y espera a que el cambio llegue a todas sus ubicaciones.
3. Quita `MY_ORIGIN_SECRET_PREVIOUS` y despliega. `my config check` avisa mientras esté establecido.

El nombre de la cabecera no rota de este modo. Para cambiarlo, repite [la configuración](#set-it-up) con la comprobación del servidor desactivada un momento, o establece el nombre nuevo en el CDN junto al antiguo antes de desplegarlo.
