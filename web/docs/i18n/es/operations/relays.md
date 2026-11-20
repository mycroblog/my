<!-- source: federation-relays bbd5bb98dde3 -->
# Relés de federación

Un relé es un servidor que transmite las publicaciones públicas de los servidores que se unen a él a los demás servidores
unidos. Incluso una instancia pequeña con pocos seguimientos obtiene una cronología federada y cronologías de etiquetas
llenas, y sus propias publicaciones públicas llegan más lejos. Una instancia que no se une a un relé no gana filas ni
trabajos.

<!-- source: join-a-relay 551373ffaed6 -->
## Unirse a un relay

Usa «Relays» en el área de administración (`/admin/relays`). Requiere el permiso `manage_settings`, que tiene el rol integrado `admin`.
«Unirse a un relay» abre el formulario que registra uno. La lista muestra el estado de cada relay y lo que trajo durante las últimas 24
horas; al elegir un relay se abre su página, con sus direcciones y el botón que lo abandona, o, una vez que el servidor lo abandonó o este
lo rechazó, los botones que se unen de nuevo a él y lo eliminan. Abandonar y eliminar preguntan antes.
La CLI es `my admin relay` (`my admin relay -help`):

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. Registra la URL del relay. Hay dos protocolos:
   - **Mastodon**: la **URL del inbox** del relay (como `https://relay.example/inbox`). Muchos relays, incluidos pub-relay y Activity-Relay, la aceptan.
   - **LitePub**: la **URL del actor** del relay (como `https://relay.example/relay`). Este es el protocolo de los relays de Pleroma y Akkoma.
   Si no eliges un protocolo («Detectar automáticamente»), el servidor usa LitePub cuando la URL sirve un documento de actor, y Mastodon en otro caso.
2. Tras registrarlo, el actor de la instancia envía al relay un `Follow`, y el estado es «Esperando la respuesta del relay» (`pending`).
   Cuando el relay acepta, el estado pasa a «Unido» (`accepted`); cuando rechaza, «Rechazado por el relay» (`rejected`). Algunos relays tardan porque un operador aprueba cada solicitud a mano.
3. Si desactivas «Recibir publicaciones del relay» al registrarlo, el servidor solo envía. Para cambiarlo, abandona el relay y regístralo de nuevo.

No puedes registrar el relay de un servidor bajo una acción de dominio (`suspend`). Si el servidor de un relay al que te uniste se suspende después, el estado no cambia,
pero no se envía ni se recibe nada de ese relay, y el área de administración lo dice en el estado del relay.

<!-- source: what-the-server-sends c55ab0a5b079 -->
## Qué envía el servidor

- Solo se envían la creación, la edición y la eliminación de publicaciones locales **públicas** (`public`). Las publicaciones `unlisted`, solo para seguidores, directas o para vecinos (`local`), y los impulsos, nunca se envían.
  Para mantener una publicación fuera de los relés, publícala como `unlisted` o `local`. No hay exclusión por usuario.
- Los documentos de las publicaciones públicas llevan una firma de Linked Data (`RsaSignature2017`), para que Mastodon y Misskey puedan aceptar publicaciones que un relé transmite sin cambios.
- El número de entregas crece según el número de relés unidos multiplicado por el número de publicaciones públicas (las entregas a un relé se combinan en una).

<!-- source: what-the-server-receives fa99c4ec4217 -->
## Qué recibe el servidor

- Una publicación pública transportada por un relé unido se recupera de nuevo desde su origen y se comprueba antes de incorporarla. Aparece entonces en la cronología federada y en las cronologías de etiquetas, y en las cronologías de inicio de las personas que siguen una de sus etiquetas. No aparece en ninguna otra cronología de inicio y no crea ninguna notificación.
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
