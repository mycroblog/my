<!-- source: run-voice-chat 54808e78eb0d -->
# Ejecutar el chat de voz

El chat de voz permite a los usuarios con la sesión iniciada abrir una sala, invitar a otras personas y hablar. Está desactivado hasta que eliges cómo viaja el audio.
Esta guía explica esa elección, los servidores STUN y TURN o el servidor multimedia que necesita cada opción, y lo que conviene saber sobre el despliegue, la privacidad y la moderación.
Toda la configuración que se nombra aquí se describe en la [referencia de configuración](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## Elegir cómo viaja el audio

`voice` elige un método para toda la instancia; todas las salas lo usan.

| `voice` | Qué transporta el audio | Para quién |
| --- | --- | --- |
| `off` | Nada. La API del chat de voz responde `404`, el cliente web no muestra el chat de voz y los demás valores de configuración `voice_*` no se comprueban al arrancar. | El valor predeterminado. |
| `p2p` | Los navegadores de los participantes se envían el audio directamente entre sí. El servidor solo retransmite los mensajes que establecen las conexiones. | Salas pequeñas y la configuración más barata: ningún servidor adicional, salvo un servidor STUN o TURN opcional. |
| `livekit` | Un servidor LiveKit, alojado por ti o LiveKit Cloud. | Salas más grandes y un control estricto de quién puede hablar. |
| `sora` | Un servidor Sora, alojado por ti o Sora Cloud. | Lo mismo, si ya ejecutas Sora. |

![Los tres métodos lado a lado. Con p2p, el servidor retransmite las señales y los navegadores se envían el audio entre sí, o a través de un servidor TURN. Con livekit y sora, los navegadores envían las señales y el audio al servidor multimedia, y el servidor habla con ese servidor multimedia mediante su API y webhooks. La lista de abajo describe cada método.](images/voice-methods.svg)

En todos los métodos, el servidor (`my serve`) guarda las salas, quién participa en ellas y sus sesiones, y entrega a cada
participante una concesión: lo que el navegador necesita para conectarse. Nunca transporta el audio. Lo que cambia es adónde van la señalización
(los mensajes que establecen una conexión) y el audio:

- **`p2p`.** Un navegador envía sus señales (ofertas, respuestas y candidatos ICE) al servidor, que las pasa al
  otro navegador por el flujo de eventos de la sala; una señal espera en el almacén de clave-valor como máximo 30 segundos. El audio
  va directamente de navegador a navegador. La concesión lista los servidores STUN de `voice_stun_urls` y el servidor TURN de
  `voice_turn` (`coturn` o `cloudflare`) con credenciales de corta duración. Un navegador conoce su dirección pública mediante STUN,
  y el servidor TURN retransmite el audio cuando los dos navegadores no encuentran un camino directo.
- **`livekit`.** La concesión contiene `voice_livekit_url` y un token de acceso de 60 segundos. El navegador envía sus señales y su
  audio al servidor LiveKit, que aporta sus propios STUN y TURN y reenvía el audio a los demás participantes (una unidad de reenvío selectivo,
  o SFU).
  El servidor crea las salas de LiveKit y cambia quién puede hablar mediante la API de LiveKit, y LiveKit informa al servidor de las entradas y
  salidas con webhooks.
- **`sora`.** La concesión contiene `voice_sora_signaling_urls` y un valor de unión firmado. El navegador envía sus señales y su
  audio al servidor Sora, que aporta sus propios STUN y TURN. Sora consulta el webhook de autenticación del servidor antes de cada
  conexión, el servidor controla las salas mediante la API de Sora y Sora informa de las conexiones con webhooks.

Notas sobre la elección:

- Con `p2p` el audio nunca pasa por este servidor (pasa por el servidor TURN cuando se usa uno). La carga del servidor son solo los mensajes de la sala, los latidos y los flujos.
- Una instancia pequeña se sirve mejor con `p2p`, o con el plan gratuito Build de LiveKit Cloud (consulta [LiveKit Cloud](#livekit-cloud)).
- Con `livekit` o `sora`, la configuración de STUN y TURN y la política de ICE de más abajo no se usan (el servidor avisa al arrancar si los defines), porque el servidor multimedia aporta sus propios servidores. Las direcciones IP de los participantes solo las ve entonces el servidor multimedia, no los demás participantes.
- Las etiquetas de compilación `no_voice_livekit` y `no_voice_sora` dejan fuera de un binario los dos métodos con servidor multimedia ([etiquetas de compilación](../reference/build-tags.md)).
- Sirve la instancia por HTTPS: los navegadores permiten el acceso al micrófono solo en páginas seguras.

Los límites mantienen acotado el costo de un servidor multimedia de pago. `0` quita cualquiera de ellos salvo el último:

| Ajuste | Predeterminado | Significado |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | Lo máximo que una sala permanece abierta. Una sala que lo alcanza se termina. |
| `voice_max_open_rooms` | `10` | Cuántas salas pueden estar abiertas a la vez en este servidor. |
| `voice_sfu_max_participants` | `10` | El tamaño de sala para `livekit` y `sora`. |
| `voice_p2p_max_participants` | `10` | El tamaño de sala para `p2p`, de 2 a 50. |

Quién puede abrir salas lo decide el ajuste de instancia `voice_room_starters` ([configuración de la instancia](instance-settings.md)): por defecto solo quienes tienen el permiso `start_voice_rooms` (el rol integrado `admin` lo tiene); `everyone` permite abrir salas a todos los usuarios locales con la sesión iniciada que no estén suspendidos ni limitados.

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## Salas peer-to-peer

<!-- source: try-it-with-docker-compose ac3681681768 -->
### Probar con Docker Compose

Para probar el chat de voz en tu equipo con la configuración de Docker Compose ([`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)),
define `MY_VOICE=p2p` en `.env` y vuelve a crear el servidor.

```sh
cd deploy/docker
docker compose up -d
```

- Abre la instancia en `http://localhost:8080` (el `MY_WEB_ORIGIN` de `example.env`). Los navegadores permiten que una página use el micrófono solo en
  `localhost` o por HTTPS, así que con una dirección como `http://192.168.1.10:8080` no se puede hablar.
- Los navegadores del mismo equipo (dos perfiles de navegador, o una ventana normal y una privada) se conectan entre sí sin un servidor STUN o TURN.
  Para llegar a navegadores de otros equipos, pon la instancia detrás de HTTPS y define `MY_VOICE_STUN_URLS` en `.env`.
- Por defecto solo el rol `admin` puede abrir salas. Inicia sesión como el administrador que creaste, o define `voice_room_starters` como `everyone`
  en la página «Chat de voz» de la configuración del servidor para que todos los usuarios puedan abrir salas.

<!-- source: size-and-bandwidth 9872bf829e26 -->
### Tamaño y ancho de banda

Dos navegadores se conectan entre sí cuando al menos uno de ellos habla; dos personas que solo escuchan no se conectan.
Por tanto, quien habla envía una copia independiente de su audio a cada uno de los demás participantes. La voz en Opus ocupa aproximadamente de 30 a 50 kbit/s con la sobrecarga, así que quien habla en una sala de 10 sube unas 9 copias, unos cientos de kbit/s,
y quien habla en una sala de 50 sube 49 copias, alrededor de 2 Mbit/s, que un dispositivo móvil con datos móviles o una conexión doméstica puede no sostener. El número de conexiones en una sala de N personas es como máximo N(N-1)/2: 45 para 10 y 1225 para 50.
Por eso el valor predeterminado es 10. Aumenta `voice_p2p_max_participants` solo cuando tus usuarios tengan ancho de banda de subida; usa `livekit` o `sora` para salas grandes.

<!-- source: stun-and-turn 51e427976a77 -->
### STUN y TURN

Un navegador detrás de un NAT necesita un servidor STUN para conocer su dirección pública, y un servidor TURN para retransmitir el audio cuando no existe un camino directo (NAT estrictos, cortafuegos corporativos, algunas redes móviles).

- `voice_stun_urls`: URL `stun:` o `stuns:` separadas por comas. Sirve cualquier servidor STUN público, y el servidor coturn de abajo también responde a STUN.
- `voice_turn`: `none`, `coturn` o `cloudflare`.

Con ninguno de los dos, el servidor avisa al arrancar: solo se conectan los navegadores de la misma red. Una instancia pública necesita STUN, y TURN para los usuarios que no pueden conectarse directamente.

El servidor entrega a cada participante credenciales TURN de corta duración, nunca una contraseña compartida. `voice_turn_credential_ttl` (`6h` por defecto) fija cuánto duran. El cliente web pide credenciales nuevas diez minutos antes de que caduquen y reinicia ICE, así que una llamada no se detiene cuando se agotan.

<!-- source: coturn d665e942c91b -->
#### coturn

Define `voice_turn = coturn`, `voice_turn_coturn_urls` y `voice_turn_coturn_secret`. El servidor calcula él mismo las credenciales (el nombre de usuario es la hora de caducidad y el ID de sesión, la contraseña es un HMAC-SHA1 de ese nombre con el secreto), así que no hace ninguna solicitud al servidor TURN.
Configura coturn para que coincida, en `turnserver.conf`:

```conf
# Time-limited credentials signed with a shared secret.
use-auth-secret
static-auth-secret=<the same value as voice_turn_coturn_secret>
realm=turn.example.org

listening-port=3478
tls-listening-port=5349
# If the host is behind NAT (most cloud VMs), tell coturn its public address.
external-ip=203.0.113.10

# Do not let the relay reach your internal networks.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

- `denied-peer-ip` es importante. Sin él, cualquiera con una credencial puede usar el servidor TURN para llegar a equipos de su red interna (servicios de metadatos de la nube, bases de datos, otras máquinas virtuales). Deniega todos los rangos privados, de bucle local y de enlace local; permite una dirección interna (`allowed-peer-ip`) solo si de verdad necesitas retransmitir hacia ella.
- Abre UDP y TCP 3478, TCP 5349 para `turns:` y el rango de puertos de retransmisión de coturn (`min-port` / `max-port`, por defecto 49152-65535).
- Lista también en `voice_turn_coturn_urls` una URL `turns:` o una con `?transport=tcp`, para que los usuarios en redes que bloquean UDP puedan conectarse igualmente.
- coturn comprueba la caducidad de una credencial solo cuando se crea una asignación por primera vez. Una asignación que ya existe sigue funcionando después de esa fecha, y las credenciales nuevas surten efecto en la siguiente asignación, que el cliente web crea reiniciando ICE.

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

Define `voice_turn = cloudflare`, `voice_turn_cloudflare_key_id` y `voice_turn_cloudflare_api_token`, a partir de una clave TURN creada en el panel de Cloudflare. El servidor pide a Cloudflare las credenciales de cada participante cuando abre una sala o se une a ella (si eso falla, la solicitud responde `503` y no abre ni se une a nada, así que se puede volver a intentar) y las revoca cuando el participante sale o es expulsado.

- Las credenciales duran como máximo 48 horas, así que `voice_turn_credential_ttl` no puede superar `48h` (de lo contrario, el servidor se niega a arrancar).
- **Cloudflare desconecta una retransmisión poco después de que caduquen sus credenciales.** El cliente web las renueva antes, así que mantén `voice_turn_credential_ttl` muy por encima de diez minutos.
- Precios, según la comprobación del 2026-10-26: los primeros 1000 GB al mes son gratuitos, y después 0,05 USD por GB enviado a los clientes. Comprueba los precios actuales de Cloudflare antes de confiar en ellos.

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### Ocultar las direcciones IP

Con `voice_ice_transport_policy = all` (el valor predeterminado) los navegadores se conectan directamente cuando pueden, así que los participantes de una sala pueden llegar a conocer las direcciones IP de los demás.

Con `relay`, los navegadores usan solo el servidor TURN, así que un participante ve la dirección del servidor TURN y no la de los demás participantes. El audio permanece cifrado entre los navegadores. Lo que `relay` no oculta: quien ejecuta el servidor TURN (tú, o Cloudflare) ve la dirección de cada participante, y este servidor ve la dirección de todos los que llaman a su API.
`relay` necesita que `voice_turn` sea `coturn` o `cloudflare` (de lo contrario, el servidor se niega a arrancar), y envía todas las llamadas a través del servidor TURN, lo que supone tu ancho de banda con coturn y tu factura de transferencia con Cloudflare.

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### Lo que P2P no puede hacer cumplir

Solo algunos participantes pueden hablar en una sala: los anfitriones y los oyentes que un anfitrión permite. En `p2p` esto se hace cumplir en dos lugares: el servidor se niega a retransmitir una oferta de conexión que enviaría audio de alguien que no puede hablar, y los demás navegadores no reproducen el audio de un participante que no es hablante.

Eso detiene a los usuarios normales y al cliente web oficial, pero no es una defensa completa. **Dos participantes con clientes modificados que colaboran entre sí aún pueden intercambiar audio**: un cliente modificado de alguien que no puede hablar puede enviarlo, y un cliente modificado de un receptor puede reproducirlo, y el servidor no puede ver dentro del contenido multimedia cifrado.
Si debes garantizar quién habla, por ejemplo en eventos públicos moderados, usa `livekit` o `sora`, donde el propio servidor multimedia descarta el audio de quien no tiene permiso.

<!-- source: deployment-notes b0f03da306b7 -->
## Notas de despliegue

- **Dos flujos por usuario**: durante una llamada, un participante mantiene el flujo de eventos propio de la sala (Server-Sent Events) además del flujo que la aplicación abre siempre. Dimensiona los límites de conexiones para dos solicitudes de larga duración por usuario en una llamada.
- **Varios procesos**: usa un adaptador de flujo compartido (`stream` definido como `postgres`, `valkey`, `sns` o `pubsub`), como para el resto de la aplicación. Con `stream = hub` y un `role` distinto de `all`, el servidor avisa al arrancar de que el chat de voz no funcionará entre procesos.
- **Cloud Run**: un flujo es una solicitud, y la CPU se asigna mientras está abierta. Cuando el tiempo de espera de la solicitud la corta, el cliente se reconecta. Fija el tiempo de espera tan largo como puedas.
- **Los servidores multimedia necesitan una máquina virtual**: LiveKit y Sora reciben el audio por UDP en un proceso de larga duración. Cloud Run solo acepta solicitudes HTTP, así que no puede alojarlos. Ejecútalos (y coturn) en una máquina virtual, o usa LiveKit Cloud, Sora Cloud o Cloudflare TURN.

<!-- source: livekit b16c1d192419 -->
## LiveKit

Define `voice = livekit` y:

| Ajuste | Valor |
| --- | --- |
| `voice_livekit_url` | La URL `wss:` a la que se conectan los navegadores, por ejemplo `wss://livekit.example.org`. |
| `voice_livekit_api_url` | La URL `https:` a la que llama este servidor, cuando difiere de la anterior. Por defecto es el mismo host con `https:`. |
| `voice_livekit_api_key`, `voice_livekit_api_secret` | Un par de claves del servidor LiveKit. El secreto firma los tokens de acceso de los participantes y verifica los webhooks. |

El servidor crea cada sala en LiveKit, la nombra con el ID de la sala y da a cada participante un token de acceso de 60 segundos. El token de un oyente no puede publicar; el de un hablante puede publicar únicamente un micrófono.

<!-- source: self-hosted-livekit 2b118136f4dc -->
### LiveKit alojado por ti

En `livekit.yaml`:

```yaml
keys:
  <api key>: <api secret>
room:
  # Rooms are created only by this server.
  auto_create: false
  # Audio only.
  enabled_codecs:
    - mime: audio/opus
    - mime: audio/red
webhook:
  api_key: <api key>
  urls:
    - https://<your domain>/voice/webhooks/livekit
```

- El servidor LiveKit debe llegar a la URL del webhook por HTTPS. El servidor verifica la firma de cada webhook con el secreto de la API.
- Los webhooks solo aceleran las cosas. Cada minuto el servidor compara los participantes de cada sala abierta con lo que informa LiveKit y retira a quien no debería estar, así que se compensa un webhook perdido mientras la instancia estaba fría.
- Para probarlo en tu equipo, `livekit-server --dev` arranca con un par de claves de desarrollo fijo.
- **Un participante expulsado puede volver a unirse durante un breve tiempo.** LiveKit alojado por ti no puede revocar un token: sigue enviando tokens nuevos a los clientes conectados, cada uno válido hasta unos 10 minutos. Por tanto, alguien expulsado puede volver a unirse con su último token hasta unos 10 minutos, hasta que el servidor lo detecta (el webhook de unión o la comprobación de un minuto) y lo corta, y es posible que se le oiga de unos segundos a un minuto mientras tanto. Usa LiveKit Cloud si la expulsión debe ser inmediata: revoca el token al instante.

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

Usa la URL WebSocket, la clave de API y el secreto del proyecto, y añade `https://<your domain>/voice/webhooks/livekit` como endpoint de webhook en la configuración del proyecto.
El plan gratuito Build da 5000 minutos y 50 GB al mes y 100 conexiones simultáneas, según la comprobación del 2026-10-26; consulta la página de precios de LiveKit para ver los planes actuales.

<!-- source: sora 5e9209dd3baa -->
## Sora

**El adaptador de Sora se ha probado solo con un Sora simulado que sigue la documentación de Sora, no con un servidor Sora real.** Pruébalo en una instancia de preproducción con un Sora real antes de depender de él.

Define `voice = sora`, `voice_sora_signaling_urls` (URL `wss:` separadas por comas a las que se conectan los navegadores), `voice_sora_join_secret` (de al menos 32 bytes, por ejemplo de `openssl rand -base64 32`; firma los valores con los que se unen los participantes) y `voice_sora_mode` (`self_hosted` o `cloud`).
La instancia debe servirse por HTTPS, porque Sora llama de vuelta a `https://<your domain>`.

<!-- source: webhooks 11b2c486a476 -->
### Webhooks

Sora llama a dos endpoints de este servidor:

| Endpoint | Finalidad |
| --- | --- |
| `/voice/webhooks/sora/auth` | El webhook de autenticación. Sora lo llama antes de cada conexión. El servidor lo permite solo para un participante que está en la sala, no ha sido expulsado, puede hablar si se conecta como hablante y está dentro del límite de tamaño, y le dice a Sora que acepte solo audio. |
| `/voice/webhooks/sora/events` | Los webhooks de eventos y de sesión (conexiones creadas y destruidas, sesiones terminadas). |

**El webhook de autenticación debe responder a tiempo.** Sora espera 5 segundos cuando está alojado por ti y 10 segundos en Sora Cloud. El servidor responde con sus propios datos sin llamar a nada más, pero un arranque en frío de la instancia (Cloud Run escalando desde cero) puede superar ese plazo, y entonces la conexión se rechaza. Mantén una instancia activa mientras se use el chat de voz.

Cuando lo alojes tú, define `default_duplicate_client_id = evict` en `sora.conf`. Un oyente que pasa a ser hablante se reconecta con la misma identidad (Sora no puede cambiar el rol de una conexión), y `evict` hace que Sora descarte la conexión anterior cuando llega la nueva.
Con el valor predeterminado `allow`, el servidor corta él mismo la conexión anterior en cuanto conoce la nueva, así que las dos se solapan un instante.

<!-- source: self-hosted-sora dd3244368b73 -->
### Sora alojado por ti

| Ajuste | Valor |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | La URL `https:` de un proxy inverso delante de la API HTTP de Sora. |
| `voice_sora_api_header_name`, `voice_sora_api_header_value` | La cabecera con la que el proxy autentica a este servidor, por ejemplo `Authorization` y `Basic ...`. |
| `voice_sora_webhook_username`, `voice_sora_webhook_password` | Las credenciales Basic de los webhooks. Obligatorias. |

**La propia API HTTP de Sora no tiene autenticación** y está pensada para escuchar solo en la dirección de bucle local. Nunca la expongas. Pon delante un proxy inverso que exija una credencial y entrega esa credencial a este servidor mediante los dos ajustes de cabecera.
Configura las URL de los webhooks en Sora con las mismas credenciales Basic. El servidor acepta solo una URL de API `https:` (`http:` solo para localhost) y no arranca sin las credenciales de los webhooks. mTLS no está admitido, porque Cloud Run y el balanceador de carga delante de ECS terminan TLS antes de llegar al servidor.

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

Define `voice_sora_mode = cloud`, `voice_sora_cloud_project_id` y `voice_sora_cloud_api_key`. El servidor firma con la clave los tokens de acceso, llama con ella a la API y verifica con ella las firmas de los webhooks; no hacen falta credenciales Basic.

<!-- source: behavior-to-expect 03fd5e1655fa -->
### Comportamiento esperado

- Un oyente que puede hablar se reconecta automáticamente como hablante, así que hay una breve pausa mientras empieza la conexión nueva.
- A un hablante al que se le quita el derecho se le silencia de inmediato con un filtro de reenvío y después se le reconecta como oyente.

<!-- source: licensing 534e304817e3 -->
### Licencias

Sora es un producto comercial de Shiguredo. Según la comprobación del 2026-10-26: Sora Cloud tiene una cuota base de 5000 yenes al mes y ningún periodo gratuito; alojarlo tú requiere una licencia de paquete, pagada por adelantado para un número de conexiones simultáneas y un periodo. La página del producto dice que no se ofrece a empresas, organizaciones ni particulares fuera de Japón, así que comprueba que puedes comprarlo antes de planificar en torno a él. LiveKit es de código abierto cuando lo alojas tú.

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## Terminar salas y registros

- Un moderador que tenga `manage_reports` puede terminar una sala abierta para todos, incluso mientras el chat de voz está desactivado ([moderación](moderation.md#voice-chat-rooms)).
- Quién participó en una sala (participaciones, hablantes y oyentes, manos levantadas, invitaciones y expulsiones) se conserva durante `voice_participants_retention_days` días después de que la sala termine (90 por defecto, al menos 30), o mientras una denuncia apunte a la sala. Se conservan el nombre de la sala, su descripción, sus anfitriones y cómo terminó. El audio, el estado del micrófono y las reacciones nunca se registran.
- `voice_public_summary` controla si los visitantes que no pueden unirse a una sala `local` ven su nombre y si está abierta, en su página y en la vista previa de su enlace ([configuración de la instancia](instance-settings.md)).

<!-- source: notes-for-users f44150f15915 -->
## Notas para los usuarios

- En un iPhone o un iPad, el envío de audio puede detenerse mientras la aplicación está en segundo plano, porque iOS puede suspender una página web que no está en primer plano. Mantén la aplicación abierta en pantalla durante una llamada.
- El navegador pide permiso para usar el micrófono la primera vez. Si lo denegaste, permítelo de nuevo en la configuración del sitio del navegador.
- En una sala peer-to-peer, otros participantes pueden llegar a conocer tu dirección IP a menos que la instancia encamine las llamadas por un relé. Pregunta al operador si esto te importa.
