<!-- source: run-voice-chat f44427fbbe76 -->
# Ejecutar el chat de voz

El chat de voz permite a los usuarios con la sesión iniciada abrir una sala, invitar a otras personas y hablar. Está desactivado hasta que eliges cómo viaja el audio.
Esta guía explica esa elección, los servidores STUN y TURN o los nodos multimedia que necesita cada opción, y lo que conviene saber sobre el despliegue, la privacidad y la moderación.
Toda la configuración que se nombra aquí se describe en la [referencia de configuración](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## Elegir cómo viaja el audio

`voice` elige un método para toda la instancia; todas las salas lo usan.

| `voice` | Qué transporta el audio | Para quién |
| --- | --- | --- |
| `off` | Nada. La API del chat de voz responde `404`, el cliente web no muestra el chat de voz y los demás valores de configuración `voice_*` no se comprueban al arrancar. | El valor predeterminado. |
| `p2p` | Los navegadores de los participantes se envían el audio directamente entre sí. El servidor solo retransmite los mensajes que establecen las conexiones. | Salas pequeñas y la configuración más barata: ningún servidor adicional, salvo un servidor STUN o TURN opcional. |
| `sfu` | Los nodos multimedia del propio servidor: cada navegador envía su audio al nodo que aloja su sala, y ese nodo reenvía el audio de quienes hablan a los participantes. | Salas más grandes y un control estricto de quién puede hablar. |

![Los dos métodos lado a lado. Con p2p, el servidor retransmite las señales y los navegadores se envían el audio entre sí, o a través de un servidor TURN. Con sfu, los navegadores obtienen sus concesiones del servidor y envían las señales y el audio al nodo multimedia de su sala. La lista de abajo describe cada método.](images/voice-methods.svg)

En todos los métodos, el servidor (`my serve`) guarda las salas, quién participa en ellas y sus sesiones, y entrega a cada
participante una concesión: lo que el navegador necesita para conectarse. Lo que cambia es adónde van la señalización
(los mensajes que establecen una conexión) y el audio:

- **`p2p`.** Un navegador envía sus señales (ofertas, respuestas y candidatos ICE) al servidor, que las pasa al
  otro navegador por el flujo de eventos de la sala; una señal espera en el almacén de clave-valor como máximo 30 segundos. El audio
  va directamente de navegador a navegador. La concesión lista los servidores STUN de `voice_stun_urls` y el servidor TURN de
  `voice_turn` (`coturn` o `cloudflare`) con credenciales de corta duración. Un navegador conoce su dirección pública mediante STUN,
  y el servidor TURN retransmite el audio cuando los dos navegadores no encuentran un camino directo.
- **`sfu`.** El servidor coloca cada sala en un nodo multimedia (una unidad de reenvío selectivo, o SFU): un proceso con
  `role = all` o `sfu`. La concesión contiene la URL `wss:` de ese nodo (`voice_sfu_public_url`) y un ticket de un solo uso.
  El navegador envía sus señales por un WebSocket al nodo y su audio al puerto UDP del nodo `voice_sfu_udp_port`
  (el puerto TCP `voice_sfu_tcp_port` cuando se bloquea UDP), y el nodo reenvía el audio de quienes hablan a los demás
  participantes. El servidor TURN de `voice_turn` retransmite para los navegadores que no pueden llegar directamente al nodo.

Notas sobre la elección:

- **`p2p`** no necesita ningún puerto UDP en el servidor: el audio nunca pasa por él (pasa por el servidor TURN cuando se usa uno), y la carga del servidor son solo los mensajes de la sala, los latidos y los flujos. Encaja en una instancia pequeña, en salas pequeñas y en un despliegue solo en Cloud Run.
- **`sfu`** hace cumplir en el servidor quién puede hablar: el nodo reenvía solo el audio de quienes hablan. Los navegadores llegan únicamente al nodo multimedia, así que las direcciones IP de los participantes las ve el nodo, no los demás participantes. Necesita una máquina con IP pública y puertos UDP y TCP abiertos para cada nodo ([Nodos multimedia](#media-nodes)). `voice_stun_urls` no hace falta, porque un nodo tiene una dirección pública, y `voice_ice_transport_policy = relay` se rechaza.
- Sirve la instancia por HTTPS: los navegadores permiten el acceso al micrófono solo en páginas seguras.

Los límites mantienen acotado el costo de las salas. `0` quita cualquiera de ellos salvo el último:

| Ajuste | Predeterminado | Significado |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | Lo máximo que una sala permanece abierta. Una sala que lo alcanza se termina. |
| `voice_max_open_rooms` | `10` | Cuántas salas pueden estar abiertas a la vez en este servidor. Una sala abierta para una hora programada cuenta desde su apertura, mientras espera a su primer participante. |
| `voice_max_participants` | `10` | La sala más grande, de 2 a 500 (50 con `p2p`). Quien aloja una sala puede fijar un máximo menor para ella. |

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

<!-- source: size-and-bandwidth e54157c29322 -->
### Tamaño y ancho de banda

Dos navegadores se conectan entre sí cuando al menos uno de ellos habla; dos personas que solo escuchan no se conectan.
Por tanto, quien habla envía una copia independiente de su audio a cada uno de los demás participantes. La voz en Opus ocupa aproximadamente de 30 a 50 kbit/s con la sobrecarga, así que quien habla en una sala de 10 sube unas 9 copias, unos cientos de kbit/s,
y quien habla en una sala de 50 sube 49 copias, alrededor de 2 Mbit/s, que un dispositivo móvil con datos móviles o una conexión doméstica puede no sostener. El número de conexiones en una sala de N personas es como máximo N(N-1)/2: 45 para 10 y 1225 para 50.
Por eso el valor predeterminado es 10. Aumenta `voice_max_participants` solo cuando tus usuarios tengan ancho de banda de subida; usa `sfu` para salas grandes.

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

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### Lo que P2P no puede hacer cumplir

Solo algunos participantes pueden hablar en una sala: los anfitriones y los oyentes que un anfitrión permite. En `p2p` esto se hace cumplir en dos lugares: el servidor se niega a retransmitir una oferta de conexión que enviaría audio de alguien que no puede hablar, y los demás navegadores no reproducen el audio de un participante que no es hablante.

Eso detiene a los usuarios normales y al cliente web oficial, pero no es una defensa completa. **Dos participantes con clientes modificados que colaboran entre sí aún pueden intercambiar audio**: un cliente modificado de alguien que no puede hablar puede enviarlo, y un cliente modificado de un receptor puede reproducirlo, y el servidor no puede ver dentro del contenido multimedia cifrado.
Si debes garantizar quién habla, por ejemplo en eventos públicos moderados, usa `sfu`, donde el propio nodo multimedia reenvía solo el audio de quienes hablan.

<!-- source: media-nodes 17f4fc8eb80b -->
## Nodos multimedia

Con `voice = sfu`, cada sala la aloja un nodo multimedia, cuyo proceso reenvía el audio de la sala. Una sala nunca se reparte
entre nodos.

<!-- source: run-media-nodes fac01c5bdbf9 -->
### Ejecutar nodos multimedia

- **`role = all`** ejecuta un nodo en el mismo proceso que todo lo demás, en `wss://<host>/voice/sfu`. Basta una máquina
  con los puertos UDP y TCP abiertos.
- **`role = sfu`** ejecuta un nodo propio, junto a los procesos `web` y `worker`. Comparte con ellos la base de datos, el
  `stream` (no `hub`) y el `kv` (no `memory`): los nodos se registran a sí mismos y registran sus salas en el almacén de
  clave-valor, y se enteran de los cambios en sus salas a través del flujo. No ejecuta trabajos y solo atiende las
  comprobaciones de estado y el WebSocket del nodo. Ejecuta tantos como necesites: un proceso `web` coloca cada sala nueva
  en el nodo con menos plazas reservadas que tenga sitio para ella. Un proceso `web` no ejecuta ningún nodo.

Lo que necesita cada nodo:

| Qué | Ajuste | Notas |
| --- | --- | --- |
| Una dirección IP pública | `voice_sfu_public_ip` detrás de un NAT | El nodo da a los navegadores esta dirección, o su propia dirección cuando no está definida. Una máquina virtual o un contenedor en la nube suele tener una dirección privada propia: defínela. |
| El puerto UDP, abierto a internet | `voice_sfu_udp_port` (7882) | Lo comparten todas las sesiones del nodo. |
| El puerto TCP, abierto a internet | `voice_sfu_tcp_port` (7881) | Para los navegadores en redes que bloquean UDP. |
| Una URL `wss:` con TLS | `voice_sfu_public_url` (`wss://<host>/voice/sfu`) | Cada nodo tiene la suya, como `wss://sfu1.example.org/voice/sfu`. Un proxy inverso delante termina el TLS y reenvía el WebSocket, o el propio nodo sirve HTTPS: los ajustes `tls` funcionan con `role = sfu`, para el host de `voice_sfu_public_url` ([Servir HTTPS desde el servidor](running.md#serving-https-from-the-server)). |
| Cuántos participantes aloja | `voice_sfu_node_capacity` (500) | Consulta [Capacidad](#capacity). |

Reenvía los puertos UDP y TCP al nodo sin cambios: el nodo da a los navegadores sus propios números de puerto. Un nodo en
un host distinto de `host` necesita además `voice_sfu_origins` en los procesos `web`, como `wss://*.sfu.example.org`, para
que la Content Security Policy del cliente web permita la conexión.

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

La capa [`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) ejecuta el servidor como `role = web`, un `worker` y
un nodo multimedia `sfu` en una sola máquina, sobre PostgreSQL detrás de Caddy ([Archivos de Docker Compose](running.md#docker-compose-files)):

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy envía `/voice/sfu` al nodo y todas las demás solicitudes al servidor, así que la URL del nodo es la predeterminada,
  `wss://<MY_HOST>/voice/sfu`, y `voice_sfu_origins` no hace falta.
- Abre UDP 7882 y TCP 7881 en el cortafuegos de la máquina, además de 80 y 443.
- La capa define `voice = sfu` y `stream = postgres` para los tres procesos, diga lo que diga `.env`.

<!-- source: capacity 2137d8a7bcce -->
### Capacidad

Una sala reserva su máximo (el máximo del anfitrión para la sala, o `voice_max_participants`) en su nodo cuando se
coloca, sin importar cuántos se hayan unido. Un nodo acepta una sala solo mientras la suma de sus reservas se mantenga
dentro de `voice_sfu_node_capacity`, así que un nodo nunca aloja más participantes que su capacidad. Cuando ningún nodo
tiene sitio para el máximo de una sala, abrir la sala o unirse por primera vez responde `503`, y el usuario puede volver a
intentarlo más tarde.

Define `voice_sfu_node_capacity` según lo que la máquina pueda reenviar ([Costo medido](#measured-cost)), y mantén
`voice_max_participants` no mayor de lo que necesiten tus salas: una sala de 500 reserva 500 plazas aunque haya 3 personas.

<!-- source: measured-cost fef7c9ecfe21 -->
### Costo medido

Medido con clientes simulados que envían audio Opus a 50 paquetes por segundo, en un AMD Ryzen 9 7900 (12 núcleos) con
Linux bajo WSL2 y Go 1.27, todo en loopback. La CPU se expresa en núcleos de esa máquina; la memoria es el heap del nodo y
las pilas de las goroutines, sumados al resto del proceso.

| Qué | CPU | Memoria |
| --- | --- | --- |
| Un nodo inactivo | ninguna medible | unos 0,1 MiB |
| Cada sala abierta | ninguna medible | unos 24 KiB |
| Cada participante | unos 0,0002 núcleos en silencio | unos 200 KiB |
| Cada flujo reenviado (alguien que habla, oído por un participante) | de 20 a 27 µs por paquete, unos 0,001 núcleos | unos 24 KiB |
| Una sala de 100 con 3 personas hablando | unos 0,3 núcleos | unos 25 MiB |

Una máquina más rápida o más lenta escala estas cifras; las métricas del propio nodo ([Métricas que vigilar](#metrics-to-watch)) muestran
lo que soporta.

El costo de reenviar sigue a los paquetes que envía el nodo: el audio de cada persona que habla va a todos los demás
participantes, así que una sala de N participantes con K personas hablando envía unos K × (N − 1) flujos de 30 a 50 kbit/s. Una sala con
muchos oyentes y pocas personas hablando es barata; una sala donde todos hablan a la vez es la que más cuesta. Planifica el
ancho de banda de salida del nodo del mismo modo.

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### Vaciar un nodo para un despliegue

Con SIGTERM, un nodo se vacía antes de detenerse:

1. Deja de aceptar salas nuevas.
2. Coloca cada una de sus salas en otro nodo e indica a los clientes de la sala que se muevan. Se reconectan al nuevo nodo
   con una concesión nueva, y el audio se detiene durante aproximadamente 1 a 2 segundos.
3. Espera a que sus clientes se hayan ido, como máximo 20 de los 25 segundos del apagado, y luego espera a las
   solicitudes restantes y termina ([Detener el servidor](running.md#stop-the-server)).

Da al proceso un periodo de gracia de al menos 30 segundos antes de que se lo mate (Kubernetes y ECS usan 30 por defecto, Docker
10, valor que `compose.sfu.yaml` aumenta), y reinicia los nodos de uno en uno, para que los demás puedan acoger sus salas.
Una sala para la que ningún otro nodo tiene sitio permanece en el nodo que se vacía hasta que termina o pasan los 20 segundos.
**Con un solo nodo**, todas las salas permanecen: sus participantes pierden el audio cuando el nodo termina y se reconectan cuando
arranca el nodo nuevo.

<!-- source: when-a-node-is-lost c577f2240d7e -->
### Cuando se pierde un nodo

Cuando un nodo se detiene sin vaciarse (una caída, una máquina perdida), los WebSockets de sus clientes se cierran y sus
arrendamientos en el almacén de clave-valor caducan en un plazo de 15 segundos. Los clientes piden una concesión nueva, y el proceso `web` vuelve a colocar la sala
en otro nodo. Una sala no pierde nada con su nodo: sus participantes, quienes hablan y los silenciados están en la base de datos.

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### TURN para clientes sin UDP

Un navegador llega a un nodo por UDP, o por TCP cuando se bloquea UDP. En las redes que solo dejan pasar el puerto 443 (algunas
redes corporativas y públicas) necesita un servidor TURN sobre TLS en el 443: define `voice_turn` como `coturn`, con una
URL `turns:` en el puerto 443 en `voice_turn_coturn_urls`, o como `cloudflare`, igual que para `p2p` ([coturn](#coturn),
[Cloudflare TURN](#cloudflare-turn)). `voice_ice_transport_policy = relay` se rechaza con `sfu`: los participantes
ya ven solo la dirección del nodo.

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

Cloud Run acepta solo solicitudes HTTP, así que `role = sfu` no puede ejecutarse allí. Ejecuta los procesos `web` y `worker` en Cloud
Run y los nodos en máquinas virtuales con IP pública, compartiendo con ellos la base de datos, el `stream` (`postgres` o `pubsub`) y el `kv`
(`db` o `valkey`). Da a cada nodo su propio nombre de host y `voice_sfu_public_url`, sirve TLS en el nodo
(`tls = acme`) o delante de él, y define `voice_sfu_origins` en el servicio de Cloud Run con los orígenes de los nodos. Los
módulos de OpenTofu de [Desplegar en Google Cloud](deploy-google-cloud.md) no crean los nodos.

<!-- source: metrics-to-watch 3346746d92f1 -->
### Métricas que vigilar

Cada nodo exporta sus propias métricas `my.voice.sfu.*` ([Métricas](running.md#metrics)).

| Métrica | Qué te dice |
| --- | --- |
| `my.voice.sfu.capacity.reserved` frente a `voice_sfu_node_capacity` | Qué tan lleno está un nodo para salas nuevas. Cuando todos los nodos están cerca del límite, las salas nuevas reciben `503`: añade un nodo, o aumenta la capacidad si el nodo tiene CPU de sobra. |
| `my.voice.sfu.capacity.connected`, `my.voice.sfu.sessions`, `my.voice.sfu.rooms` | Quién está conectado. Muy por debajo de las reservas significa que los máximos de las salas son mayores de lo necesario. |
| `my.voice.sfu.packets` por `my.outcome` | `forwarded` es el trabajo del nodo. `silent` y `not_allowed` (un participante que no puede hablar o está silenciado) se descartan por diseño. `queue_full` significa que la conexión de un participante no pudo seguir el ritmo: un cliente lento cuando son pocos, un nodo con falta de CPU o de ancho de banda cuando sube para todos los clientes a la vez. |
| `my.voice.sfu.bytes` | El audio reenviado: el ancho de banda de salida del nodo. |
| `my.voice.sfu.client.rtt`, `my.voice.sfu.client.packet_loss` | La calidad del camino hacia los participantes. Alta para la mayoría de los clientes apunta a la red o la ubicación del nodo; para unos pocos, a sus propias redes. |
| `my.voice.sfu.migrations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.renegotiations` | Salas movidas por un vaciado, clientes que cambiaron de red y cambios de quienes hablan. |
| `my.voice.sfu.ticket_refusals` por `my.voice.sfu.reason` | Un `other_node` que sigue subiendo significa que los clientes llegan a un nodo distinto del de su concesión: comprueba que el `voice_sfu_public_url` de cada nodo lleve a ese nodo. |

<!-- source: deployment-notes 63042f72a7c5 -->
## Notas de despliegue

- **Dos flujos por usuario**: durante una llamada, un participante mantiene el flujo de eventos propio de la sala (Server-Sent Events) además del flujo que la aplicación abre siempre. Dimensiona los límites de conexiones para dos solicitudes de larga duración por usuario en una llamada.
- **Varios procesos**: usa un adaptador de flujo compartido (`stream` definido como `postgres`, `valkey`, `sns` o `pubsub`), como para el resto de la aplicación. Con `stream = hub` y un `role` distinto de `all`, el servidor avisa al arrancar de que el chat de voz no funcionará entre procesos.
- **Cloud Run**: un flujo es una solicitud, y la CPU se asigna mientras está abierta. Cuando el tiempo de espera de la solicitud la corta, el cliente se reconecta. Fija el tiempo de espera tan largo como puedas: `service.request_timeout_seconds` de [Desplegar en Google Cloud](deploy-google-cloud.md#limits).
- **Los nodos multimedia necesitan una máquina virtual**: un nodo multimedia de `sfu` recibe el audio por UDP en un proceso de larga duración, que Cloud Run no puede alojar ([Cloud Run](#cloud-run)).

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
