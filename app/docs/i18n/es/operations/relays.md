<!-- source: federation-relays bbd5bb98dde3 -->
# Relés de federación

Un relé es un servidor que transmite las publicaciones públicas de los servidores que se unen a él a los demás servidores
unidos. Incluso una instancia pequeña con pocos seguimientos obtiene una cronología federada y cronologías de etiquetas
llenas, y sus propias publicaciones públicas llegan más lejos. Una instancia que no se une a un relé no gana filas ni
trabajos.

<!-- source: join-a-relay 147b07e65534 -->
## Unirse a un relé

Usa «Relés» en el área de administración (`/admin/relays`). Requiere el permiso `manage_settings`, que tiene el rol
integrado `admin`. «Unirse a un relé» abre el formulario que registra uno. La lista muestra el estado de cada relé con un
botón que lo abandona o se une a él de nuevo; al elegir un relé se abre su página, con sus direcciones, lo que aportó en
las últimas 24 horas y el botón que lo elimina. La CLI es `my admin relay` (`my admin relay -help`):

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. Registra la URL del relé. Hay dos protocolos:
   - **Mastodon**: la **URL del buzón** (inbox) del relé (como `https://relay.example/inbox`). Muchos relés, incluidos pub-relay y Activity-Relay, la aceptan.
   - **LitePub**: la **URL del actor** del relé (como `https://relay.example/relay`). Es el protocolo de los relés de Pleroma y Akkoma.
   Si no eliges un protocolo («Detectar automáticamente»), el servidor usa LitePub cuando la URL sirve un documento de actor, y Mastodon en caso contrario.
2. Después de registrarlo, el actor de la instancia envía al relé un `Follow`, y el estado es «Esperando la respuesta del relé» (`pending`). Cuando el relé acepta, el estado pasa a «Unido» (`accepted`); cuando rechaza, a «Rechazado por el relé» (`rejected`). Algunos relés tardan porque un operador aprueba cada solicitud a mano.
3. Si desactivas «Recibir publicaciones del relé» al registrarlo, el servidor solo envía. Para cambiarlo, abandona el relé y regístralo de nuevo.

No puedes registrar el relé de un servidor sujeto a una acción sobre dominios (`suspend`). Si el servidor de un relé unido
se suspende más tarde, el estado no cambia, pero no se envía nada a ese relé ni se recibe nada de él, y el área de
administración muestra una advertencia.

<!-- source: what-the-server-sends 1db52c508f9c -->
## Qué envía el servidor

- Solo se envían la creación, la edición y la eliminación de publicaciones locales **públicas** (`public`). Las publicaciones `unlisted`, solo para seguidores, directas o solo locales (`local`), y los impulsos, nunca se envían. Para mantener una publicación fuera de los relés, publícala como `unlisted` o `local`. No hay exclusión por usuario.
- Los documentos de las publicaciones públicas llevan una firma de Linked Data (`RsaSignature2017`), para que Mastodon y Misskey puedan aceptar publicaciones que un relé transmite sin cambios.
- El número de entregas crece según el número de relés unidos multiplicado por el número de publicaciones públicas (las entregas a un relé se combinan en una).

<!-- source: what-the-server-receives ff962150d1a7 -->
## Qué recibe el servidor

- Una publicación pública transportada por un relé unido se recupera de nuevo desde su origen y se comprueba antes de incorporarla. Aparece entonces en la cronología federada y en las cronologías de etiquetas. No aparece en las cronologías de inicio y no crea ninguna notificación.
- Se incorporan como máximo `relay_max_activities_per_hour` publicaciones por hora (1000 de forma predeterminada) por relé. Lo que supera el límite se descarta sin recuperarse. La página del relé en el área de administración y `my admin relay list` muestran cuántas se incorporaron en las últimas 24 horas y cuántas se descartaron por superar el límite. Si se descartan muchas, sube el límite o replantéate el relé.
- Una publicación no se incorpora cuando su dominio o su cuenta tiene un `suspend` (una restricción de federación) o un `limit` (una restricción que la retira de los lugares públicos).
- Una publicación que llegó solo mediante un relé se retira de la vista pública tras el ajuste de instancia `relay_status_retention_days` (7 de forma predeterminada). Una publicación a la que una persona local respondió, que impulsó, marcó como favorita o guardó en marcadores, y las publicaciones de autores a los que sigue una persona local, tienen la misma retención que otras publicaciones remotas (`remote_status_retention_days`).
- Incorporar cuesta una recuperación desde el origen por publicación (más una recuperación del actor para un autor desconocido). Con el límite de 1000 por hora, son como máximo 24 000 recuperaciones al día.

Su configuración está en la [referencia de configuración](../reference/configuration.md#relay_max_activities_per_hour). El resultado de la incorporación también se exporta como la métrica `my.relay.activities` (consulta [Ejecutar el servidor](running.md)).

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## Abandonar, unirse de nuevo y eliminar

- «Abandonar» envía un `Undo` del `Follow` y establece el estado en «Abandonado» (`left`). Después no se envía nada al relé ni se recibe nada de él. Las publicaciones ya incorporadas se retiran cuando termina su retención.
- «Volver a unirse» envía un `Follow` nuevo a un relé que rechazó al servidor o que el servidor abandonó.
- «Eliminar» quita el registro de un relé que se abandonó o que rechazó al servidor. Las ediciones y eliminaciones de publicaciones ya enviadas al relé se le siguen entregando después de eliminarlo.

Registrar, unirse, abandonar y eliminar, y la aceptación o el rechazo del relé, quedan registrados en el registro de
auditoría con el tipo de objetivo «Relé» (la aceptación y el rechazo como acciones remotas).
