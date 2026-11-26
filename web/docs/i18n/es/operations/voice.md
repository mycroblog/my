<!-- source: run-voice-chat 54808e78eb0d -->
# Ejecutar el chat de voz

El chat de voz permite a los usuarios con la sesión iniciada abrir una sala, invitar a otras personas y hablar. Está desactivado hasta que eliges cómo viaja el audio.
Esta guía explica esa elección, los servidores STUN y TURN o el servidor multimedia que necesita cada opción, y lo que conviene saber sobre el despliegue, la privacidad y la moderación.
Toda la configuración que se nombra aquí se describe en la [referencia de configuración](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 1fe795fb2216 -->
## Elegir cómo viaja el audio

`voice` elige un método para toda la instancia; todas las salas lo usan.

| `voice` | Qué transporta el audio | Para quién |
| --- | --- | --- |
| `off` | Nada. La API del chat de voz responde `404`, el cliente web no muestra el chat de voz y los demás valores de configuración `voice_*` no se comprueban al arrancar. | El valor predeterminado. |
| `p2p` | Los navegadores de los participantes se envían el audio directamente entre sí. El servidor solo retransmite los mensajes que establecen las conexiones. | Salas pequeñas y la configuración más barata: ningún servidor adicional, salvo un servidor STUN o TURN opcional. |

![Los tres métodos lado a lado. Con p2p, el servidor retransmite las señales y los navegadores se envían el audio entre sí, o a través de un servidor TURN. Con livekit y sora, los navegadores envían las señales y el audio al servidor multimedia, y el servidor habla con ese servidor multimedia mediante su API y webhooks. La lista de abajo describe cada método.](images/voice-methods.svg)

En todos los métodos, el servidor (`my serve`) guarda las salas, quién participa en ellas y sus sesiones, y entrega a cada
participante una concesión: lo que el navegador necesita para conectarse. Nunca transporta el audio. Lo que cambia es adónde van la señalización
(los mensajes que establecen una conexión) y el audio:

- **`p2p`.** Un navegador envía sus señales (ofertas, respuestas y candidatos ICE) al servidor, que las pasa al
  otro navegador por el flujo de eventos de la sala; una señal espera en el almacén de clave-valor como máximo 30 segundos. El audio
  va directamente de navegador a navegador. La concesión lista los servidores STUN de `voice_stun_urls` y el servidor TURN de
  `voice_turn` (`coturn` o `cloudflare`) con credenciales de corta duración. Un navegador conoce su dirección pública mediante STUN,
  y el servidor TURN retransmite el audio cuando los dos navegadores no encuentran un camino directo.

Notas sobre la elección:

- Con `p2p` el audio nunca pasa por este servidor (pasa por el servidor TURN cuando se usa uno). La carga del servidor son solo los mensajes de la sala, los latidos y los flujos.
- Una instancia pequeña se sirve mejor con `p2p`, o con el plan gratuito Build de LiveKit Cloud (consulta LiveKit Cloud).
- Con `livekit` o `sora`, la configuración de STUN y TURN y la política de ICE de más abajo no se usan (el servidor avisa al arrancar si los defines), porque el servidor multimedia aporta sus propios servidores. Las direcciones IP de los participantes solo las ve entonces el servidor multimedia, no los demás participantes.
- Sirve la instancia por HTTPS: los navegadores permiten el acceso al micrófono solo en páginas seguras.

Los límites mantienen acotado el costo de un servidor multimedia de pago. `0` quita cualquiera de ellos salvo el último:

| Ajuste | Predeterminado | Significado |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | Lo máximo que una sala permanece abierta. Una sala que lo alcanza se termina. |
| `voice_max_open_rooms` | `10` | Cuántas salas pueden estar abiertas a la vez en este servidor. Una sala abierta para una hora programada cuenta desde su apertura, mientras espera a su primer participante. |
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

<!-- source: deployment-notes 9daf6e4d8d98 -->
## Notas de despliegue

- **Dos flujos por usuario**: durante una llamada, un participante mantiene el flujo de eventos propio de la sala (Server-Sent Events) además del flujo que la aplicación abre siempre. Dimensiona los límites de conexiones para dos solicitudes de larga duración por usuario en una llamada.
- **Varios procesos**: usa un adaptador de flujo compartido (`stream` definido como `postgres`, `valkey`, `sns` o `pubsub`), como para el resto de la aplicación. Con `stream = hub` y un `role` distinto de `all`, el servidor avisa al arrancar de que el chat de voz no funcionará entre procesos.
- **Cloud Run**: un flujo es una solicitud, y la CPU se asigna mientras está abierta. Cuando el tiempo de espera de la solicitud la corta, el cliente se reconecta. Fija el tiempo de espera tan largo como puedas: `service.request_timeout_seconds` de [Desplegar en Google Cloud](deploy-google-cloud.md#limits).
- **Los servidores multimedia necesitan una máquina virtual**: LiveKit y Sora reciben el audio por UDP en un proceso de larga duración. Cloud Run solo acepta solicitudes HTTP, así que no puede alojarlos. Ejecútalos (y coturn) en una máquina virtual, o usa LiveKit Cloud, Sora Cloud o Cloudflare TURN.

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
