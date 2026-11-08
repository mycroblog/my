<!-- source: decision-hooks 9945608ea473 -->
# Hooks de decisión

Un hook de decisión consulta a un [plugin](plugins.md) tuyo si debe aceptarse algo antes de que el servidor lo acepte. El plugin
responde con un veredicto de una lista fija. Nunca reescribe nada ni toma medidas sobre una cuenta: lo máximo que puede hacer
es detener una publicación, un seguimiento, un registro, una subida de archivo o un cambio de perfil, o presentar una denuncia para tus moderadores. Son las personas quienes deciden qué ocurre con una cuenta.

Por defecto no se revisa nada. Un servidor sin dirección de hook no hace ninguna llamada ni dedica tiempo a ellos. Esta versión conserva
los registros, los modos y las pantallas de administración que se describen aquí. De las vías de entrada, las publicaciones de otros servidores consultan
`inbound.status`, los seguimientos de otros servidores consultan `inbound.follow`, los registros consultan `signup.check`, las subidas consultan
`media.check`, las publicaciones de cuentas locales consultan `local.status`, los perfiles de cuentas de otros servidores consultan `inbound.actor` y
los cambios de perfil de cuentas locales consultan `local.profile`; los demás hooks todavía no se llaman.

![El recorrido de un hook de decisión: una publicación remota, un seguimiento remoto, un registro, una subida, una publicación local o el perfil modificado de un actor remoto pasa por el modo del hook hasta tu plugin, cuyo veredicto o fallo lleva a una decisión registrada. La lista siguiente describe cada paso.](images/decision-hooks.svg)

De arriba abajo en el diagrama:

1. **Qué se revisa y quién espera.** Una publicación de otro servidor (`inbound.status`), un seguimiento de otro servidor
   (`inbound.follow`) y una subida (`media.check`) se revisan en un trabajo. Un registro (`signup.check`) se revisa mientras
   el solicitante espera, una publicación de una cuenta local (`local.status`) mientras espera su autor, y un cambio del perfil de una cuenta
   local (`local.profile`) mientras espera su titular. Un perfil de otro servidor (`inbound.actor`) se
   revisa dondequiera que el servidor obtenga la cuenta: en un trabajo y también mientras espera una solicitud, como la de un servidor
   cuya cuenta es nueva aquí.
2. **El modo**, `hook_<hook>_mode`: `off` no hace ninguna llamada, `observe` registra el veredicto y no actúa sobre nada, y `enforce`
   actúa en consecuencia ([Modos](#modes)).
3. **Tu plugin** recibe `POST /my.plugin.hook.v1.HookService/Check`. El servidor espera hasta `plugin_<hook>_timeout`, hace
   como máximo 8 llamadas a la vez y, tras 5 fallos seguidos, omite el plugin durante 30 segundos (`breaker_open`).
4. **Un veredicto**: `allow` acepta el sujeto, `flag` lo acepta y presenta una denuncia, `mark_sensitive` lo acepta marcado como
   sensible, `require_approval` retiene un registro, y `drop` y `reject` lo detienen y no conservan nada (el `reject` de un registro solo con
   [`hook_signup_check_reject`](#sign-ups)). **Un fallo** es un `timeout`, un `error`, una respuesta `invalid` o `breaker_open`, y una respuesta que el hook no admite
   cuenta como uno. Cuando el hook aplica, `hook_<hook>_on_failure` decide: `pass` acepta el sujeto sin revisar, `retry`
   no registra ninguna decisión y vuelve a ejecutar el trabajo, y `require_approval` retiene un registro
   ([Cuando el plugin falla](#when-the-plugin-fails)).
5. **Se registra una decisión por hook y sujeto**, y un trabajo que se ejecuta de nuevo la lee en lugar de llamar al plugin
   ([Las decisiones](#the-decisions)).

<!-- source: the-hooks f1005c829ed5 -->
## Los hooks

| Hook | Qué revisa | Qué puede responder | Cuando el plugin falla |
| --- | --- | --- | --- |
| `inbound.status` | Publicaciones de otros servidores | `allow`, `drop`, `flag`, `mark_sensitive` | `pass` (por defecto) o `retry` |
| `inbound.follow` | Seguimientos de otros servidores | `allow`, `drop`, `flag` | `pass` (por defecto) o `retry` |
| `signup.check` | Registros, tras el CAPTCHA y la lista de bloqueo | `allow`, `require_approval`, `reject` | `pass` (por defecto) o `require_approval` |
| `media.check` | Archivos subidos, antes de que nadie pueda usarlos | `allow`, `mark_sensitive`, `flag`, `reject` | `pass` (por defecto) o `retry` |
| `local.status` | Publicaciones de cuentas locales y sus ediciones | `allow`, `flag`, `reject` | `pass` |
| `inbound.actor` | Perfiles de cuentas de otros servidores, cuando cambian | `allow`, `drop`, `flag` | `pass` (por defecto) o `retry` |
| `local.profile` | Cambios de perfil de cuentas locales | `allow`, `flag`, `reject` | `pass` |

Define `plugin_<hook>_address` para asignar un plugin a un hook, y cada punto del nombre pasa a ser `_`
(`plugin_inbound_status_address`). [Plugins](plugins.md) explica el destino, su autenticación y cómo probar un
plugin con `my plugin test --kind inbound.status <address>`. La llamada es `POST /my.plugin.hook.v1.HookService/Check`.

Qué hace cada respuesta:

- `drop` detiene el sujeto y no guarda nada de él. En `inbound.actor` conserva el perfil guardado antes
  ([más abajo](#profiles-from-other-servers)). Un `drop` erróneo oculta una publicación en silencio también a sus seguidores, así que revisa
  en `observe` lo que un hook descartaría antes de aplicarlo.
- `flag` lo deja pasar y presenta una denuncia por decisión. En `inbound.follow` la denuncia es sobre la cuenta que envió
  el seguimiento, no menciona ninguna publicación y se presenta una vez registrado el seguimiento; su denunciante, categoría y comentario son los de
  [la denuncia de una publicación](#what-inboundstatus-reviews). En `inbound.actor` la denuncia es sobre la cuenta a la que pertenece el perfil,
  de la misma manera.
- `mark_sensitive` lo deja pasar y muestra sus adjuntos como sensibles hasta que un moderador cancele la decisión.
- `require_approval` retiene el registro hasta su aprobación.
- `reject` lo rechaza, con un texto fijo que no muestra nada de la respuesta del plugin. En `signup.check` esto depende además de
  [la configuración de más abajo](#sign-ups). En `media.check` no conserva nada de la subida, ni siquiera el original
  ([más abajo](#what-mediacheck-reviews)). En `local.status` su autor puede [apelar](#local-posts). En `local.profile` el
  perfil queda como estaba y no hay apelación ([más abajo](#local-profiles)).

Una respuesta puede incluir un motivo codificado (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) y una nota de
hasta 200 caracteres para tus moderadores. Una respuesta que el hook no admite cuenta como un fallo.

<!-- source: what-inboundstatus-reviews 1dcf4ccd16f9 -->
## Qué revisa `inbound.status`

El servidor consulta cada `Create` de una publicación y cada `Update` de una que vaya a aceptar, antes de guardar nada. Cada
revisión es un sujeto propio con su propia decisión.

- Solo se consulta una publicación cuando está relacionada con una cuenta local: una cuenta local sigue a su autor, menciona a una cuenta
  local o está en un hilo con una publicación local. Que una cuenta local siga al autor no restringe más la consulta:
  el plugin recibe `any_local_follower` y puede tenerlo en cuenta. Nunca se consulta una publicación traída solo por un relay, ni un
  `Update` de una publicación guardada que ya no se relaciona con una cuenta local: su revisión se guarda sin consulta.
- Una publicación directa de un autor al que ninguna cuenta local sigue no se envía salvo que actives
  `hook_inbound_status_unfollowed_direct`. Las publicaciones de tus cuentas locales nunca se envían, con ninguna configuración.
- Nunca se consultan los votos de encuestas, las eliminaciones, las anulaciones, los favoritos, las reacciones ni los impulsos, así que detener un plugin no puede impedir que
  llegue una eliminación. Una obtención de hilo o la obtención de un impulso aún pueden traer la misma publicación por otra vía.

Qué hace cada respuesta con una publicación:

- `drop` en un `Create` no guarda nada y no crea ninguna notificación ni entrada de cronología, y un reenvío se ignora como
  duplicado. `drop` en un `Update` no crea ninguna revisión y conserva la anterior. Una publicación descartada no se guarda, así que un
  `Update` suyo publicado en el último día se toma como una publicación nueva: se vuelve a consultar al plugin sobre esa revisión, como sobre
  cualquier otra, y su respuesta al `Create` anterior no se traslada.
- `flag` presenta una denuncia por decisión, con el actor de tu instancia como denunciante, la categoría del motivo (`spam` y
  `malware` como Spam, `illegal` como Legal, `harassment`, `sexual` y `violence` como Violation, y cualquier otro como Other) y
  la nota del plugin como comentario. La denuncia indica que la presentó una revisión. No se reenvía al servidor del autor
  salvo que un moderador [la reenvíe](moderation.md#reports), y un reenvío nunca envía la nota: solo el texto que
  escribe entonces el moderador. Resolverla no notifica a nadie. Cada decisión presenta su propia
  denuncia, así que una oleada de spam puede llenar la cola.
- `mark_sensitive` marca la publicación, no la revisión: la marca permanece en las revisiones posteriores, sea cual sea su respuesta, hasta que un
  moderador cancele la decisión. Muestra los adjuntos como sensibles y no toca el texto, así que una publicación sin
  adjuntos no se ve distinta, y otros servidores no la ven.

<!-- source: what-mediacheck-reviews f93110fe5a83 -->
## Qué revisa `media.check`

El servidor consulta cada archivo que sube una cuenta local, para cualquier uso: un adjunto, un avatar, una cabecera o un emoji
personalizado. Consulta una sola vez, cuando el archivo se procesa por primera vez y antes de que esté listo, de modo que nadie puede usarlo mientras el plugin
decide. Si más adelante se usa el mismo archivo para otra cosa, se lee la decisión ya tomada. Nunca se consulta por medios remotos.

Envía la imagen más pequeña que el servidor hizo del archivo, sin sus metadatos, y nunca el original: para un archivo de video o
audio, la imagen pequeña de su póster o de su carátula incrustada, o ninguna imagen si no la hay. Con ella van para qué sirve el archivo,
si es una imagen, un video o un audio, su tamaño, cuánto dura y si tiene movimiento. No se envían el texto alternativo ni
nada de la cuenta o de una publicación. El mensaje `MediaSubject` del protocolo `HookService` tiene los campos.

Qué hace cada respuesta con una subida:

- `reject` hace que el procesamiento falle, como con un archivo que el servidor no puede leer: quien lo sube ve que el archivo no se pudo
  procesar, y una publicación no puede adjuntarlo. **El servidor no conserva nada de un archivo rechazado.** Elimina el original cuando
  termina el procesamiento, sea cual sea la respuesta, y un archivo rechazado no obtiene versión procesada. Si necesitas conservar esos
  archivos o informar de ellos a alguien, tu plugin tiene que hacerlo al responder: el servidor no puede devolver el archivo.
  Cancelar un `reject` solo cambia su registro.
- `mark_sensitive` marca la publicación que adjunta el archivo, en la transacción que crea la publicación o la edición que añade
  el archivo, y la marca permanece hasta que un moderador cancele la decisión, igual que en `inbound.status`. Un avatar, una cabecera o un
  emoji no tienen publicación, así que la decisión solo se registra.
- `flag` presenta una denuncia por decisión, con el actor de tu instancia como denunciante. Para un adjunto se presenta junto con la
  publicación que adjunta el archivo y apunta a esa publicación; un archivo que nunca se publica no presenta ninguna. Para un avatar, una cabecera o
  un emoji se presenta cuando se procesa el archivo, sobre la cuenta que lo subió.

El trabajo de procesamiento espera al plugin hasta `plugin_media_check_timeout` (10 segundos por defecto). Añadir un emoji
personalizado de otro servidor o aprobar una solicitud de emoji procesa la imagen mientras espera el moderador, así que esa espera
es suya.

<!-- source: sign-ups be7ddf1149a9 -->
## Registros

`signup.check` se ejecuta en los registros desde la página de registro y mediante un proveedor de inicio de sesión, tras el CAPTCHA y la
[lista de bloqueo de registros](moderation.md), así que una solicitud que cualquiera de ellos rechace nunca llega al plugin. Un registro con
código de invitación nunca lo llama. Envía la dirección de correo electrónico que escribió el solicitante y la dirección IP del cliente; la cuenta
aún no existe, así que la clave del sujeto es el ID que recibiría la cuenta, y un solicitante que lo intenta de nuevo es un sujeto nuevo.

Cuando el hook aplica, las respuestas actúan como lo hace la lista de bloqueo:

- `require_approval` deja el registro en espera en **Moderación** > **Registros**, igual que un bloqueo con la misma gravedad,
  en todos los modos de registro. Apruébalo o recházalo ahí: cancelar la decisión no cambia el registro.
- `reject` no hace nada hasta que actives `hook_signup_check_reject` (**Configuración del servidor** > **Federación** > **Revisión externa**, desactivado por
  defecto). Mientras está desactivado, un `reject` se registra en `observe`, ya que no detiene nada, y el registro continúa. Mientras
  está activado, el solicitante ve el mismo texto fijo que con un bloqueo («Este servidor no puede aceptar este registro.»). No se
  escribe ninguna cuenta, así que cancelar una decisión así solo cambia su registro, y el solicitante se registra de nuevo.

Los solicitantes nunca ven el motivo ni la nota de una decisión.

La solicitud del solicitante espera al plugin hasta `plugin_signup_check_timeout` (2 segundos por defecto; mantenlo corto, ya que
hay una persona esperando). La espera no forma parte de una transacción de base de datos. Un registro que falla después, por ejemplo por un
nombre de usuario ya en uso, conserva su decisión hasta que la retención la elimine.

<!-- source: profiles-from-other-servers e324990a50b1 -->
## Perfiles de otros servidores

`inbound.actor` revisa el perfil de una cuenta de otro servidor allí donde el servidor lo incorpora: la primera vez que obtiene
la cuenta, cuando la cuenta envía un `Update` de sí misma y cuando el servidor la obtiene de nuevo, como hace una vez al día con las cuentas
que siguen tus usuarios. Solo consulta cuando el perfil difiere del guardado, así que obtener de nuevo una cuenta con
el mismo perfil no hace ninguna llamada. El perfil que revisa es:

- el nombre visible, como texto sin formato,
- la biografía y los campos del perfil, tal como los guarda el servidor (HTML depurado),
- las direcciones del avatar y de la imagen de cabecera en el otro servidor. El servidor no envía ninguna imagen.

El plugin recibe también la dirección de la cuenta, su servidor, su nombre de usuario y si la cuenta es nueva para tu servidor. La
clave de una decisión es la dirección de la cuenta, `#` y un resumen del perfil, de modo que cada perfil distinto recibe una decisión y
el mismo perfil enviado de nuevo la lee.

Qué hacen los veredictos:

- `drop` conserva el perfil guardado antes: el nombre visible, la biografía, los campos, el avatar, la cabecera y sus emojis
  personalizados quedan como estaban. El servidor sigue guardando el resto de lo que recibió: las claves de la cuenta, su bandeja de entrada y demás
  direcciones, su nombre de usuario y si está bloqueada, es un bot, es descubrible e indexable. Sin las claves y las direcciones el
  servidor no podría verificar las actividades de la cuenta ni entregarle nada, y la configuración de bloqueada e indexable indica cómo
  seguir la cuenta y si su titular permite la indexación. Mientras el perfil siga descartado, obtenerlo de nuevo lee la
  misma decisión y conserva el guardado.
- Una cuenta obtenida por primera vez siempre se guarda, porque los seguimientos y las menciones la necesitan, así que su `drop` se toma como un
  `flag`: el perfil se guarda y se presenta una denuncia. La decisión sigue mostrando `drop`.
- `flag` guarda el perfil y presenta una denuncia sobre la cuenta por decisión.

Cuando el plugin falla, `pass` guarda el perfil sin revisar, y el servidor no vuelve a consultar sobre él hasta que cambie.
Con `retry`, el servidor no guarda una cuenta cuyo perfil cambió mientras el plugin falla: un `Update` espera en su
trabajo, que retiene las demás actividades de esa cuenta como en [`inbound.status`](#when-the-plugin-fails), y una obtención
hecha mientras espera una solicitud falla, de modo que un servidor cuya cuenta es nueva aquí ve rechazada su entrega hasta que el plugin vuelva
y la envíe de nuevo.

Cancelar un `drop` de `inbound.actor` obtiene de nuevo la cuenta desde su servidor, que guarda el perfil que sirve ahora
sin llamar al plugin cuando es el descartado, y consulta cuando ha cambiado desde entonces.

<!-- source: local-posts 37ba27537191 -->
## Publicaciones locales

`local.status` revisa una publicación de una cuenta de tu servidor antes de que el servidor la cree, y cada edición de una antes de que el
servidor la guarde. El autor espera al plugin, así que
la espera es corta: `plugin_local_status_timeout` (1 segundo por defecto). Un fallo, un tiempo de espera agotado o un breaker abierto siempre dejan pasar la
publicación (`pass` es su único comportamiento ante fallos), y la espera no forma parte de una transacción de base de datos.

Qué envía y cuándo:

- Solo las publicaciones de cuentas más recientes que `hook_local_status_account_age_days` (**Configuración del servidor** > **Federación** >
  **Revisión externa**, 7 días por defecto; `0` envía las publicaciones de todas las cuentas). Las cuentas nuevas son de donde viene la mayor parte del spam, y las cuentas
  consolidadas no se envían.
- Solo publicaciones `public`, `local` y `unlisted`. Las publicaciones `private` y `direct` nunca se envían.
- El texto tal como lo escribió el autor y tal como lo renderiza el servidor, la advertencia de contenido, el idioma, si está marcada
  como sensible, las descripciones de las imágenes, las opciones de la encuesta, las etiquetas, a cuántas cuentas menciona, la dirección que tendría
  la publicación, la publicación a la que responde o que cita (solo por dirección), la dirección y el nombre de usuario del autor, y cuándo se
  creó la cuenta. No se envía ningún medio.
- La clave del sujeto es el ID que recibiría la publicación. Enviar de nuevo una publicación con el mismo `Idempotency-Key` devuelve la publicación
  que se creó, sin llamar al plugin; un intento nuevo es un sujeto nuevo.
- Una edición se envía de la misma manera, con la publicación ya editada, a las mismas cuentas: las que están dentro de
  `hook_local_status_account_age_days`. Su clave es el ID de la publicación, `#` y el ID de la nueva revisión, de modo que cada edición es un
  sujeto propio.
- Iniciar una sala de chat de voz publica su primera publicación sin pasar por este hook.

Cuando aplica, `reject` rechaza la publicación y no se guarda nada. El autor ve un texto fijo («Esta publicación no se ha publicado.»)
y una forma de apelar; nunca el motivo ni la nota. `flag` deja pasar la publicación y presenta una denuncia sobre ella, contra su
autor.

En una edición, `reject` rechaza solo esa edición: la publicación queda como estaba, y el autor ve un texto fijo («Esta edición no se
guardó.»). Una edición no se puede apelar, ya que la publicación sigue ahí para que tus moderadores actúen como de costumbre. `flag` guarda la
edición y presenta una denuncia sobre la publicación, y `allow` la deja pasar.

<!-- source: appeals 79991f552151 -->
### Apelaciones

El autor de una publicación rechazada puede apelar en el plazo de `appeal_window_days` (la misma ventana que para las acciones de moderación), una vez por
decisión. La apelación lleva el texto de la publicación, ya que el servidor no guardó nada de ella, y un comentario opcional para tus
moderadores. No se llama a ningún plugin.

Tus moderadores (`manage_reports`) reciben una notificación de cada nueva apelación (**Apelaciones de publicaciones rechazadas** en su configuración de
notificaciones, que también pueden recibir por correo electrónico; la notificación abre la apelación) y revisan las apelaciones en **Moderación** >
**Revisiones automáticas** > **Apelaciones de publicaciones rechazadas**. Cada apelación se abre en su propia página, con el texto que envió el autor:

- **Mantener** deja la decisión como está y consta en el registro de auditoría como `hook_appeal_upheld`.
- **Anular** cancela la decisión, que consta en el registro de auditoría como `hook_decision_overridden`, y deja de enviar las
  publicaciones de la cuenta a `local.status`, todo en una transacción. La publicación rechazada no se restaura: el autor la publica de nuevo.

En ambos casos se notifica al autor el resultado, entre los avisos de los moderadores, y el formulario de publicación lo muestra bajo
el rechazo. La notificación y el formulario solo dicen si la decisión se mantiene o se canceló, nunca el motivo ni la
nota. Un cliente propio consulta el estado de una apelación con `GET /api/v1/hook_decisions/{id}/appeal`.

Una cuenta que una apelación anulada sacó del hook permanece fuera de él hasta que se elimine la cuenta. Si más tarde otra persona
la toma, `local.status` ya no la revisa.

<!-- source: local-profiles dbafa1545e2a -->
## Perfiles locales

`local.profile` revisa un cambio del perfil de una cuenta de tu servidor antes de que el servidor lo guarde. El titular espera
al plugin, así que la espera es corta: `plugin_local_profile_timeout` (1 segundo por defecto). Un fallo, un tiempo de espera agotado o un
breaker abierto siempre dejan pasar el cambio (`pass` es su único comportamiento ante fallos), y la espera no forma parte de una
transacción de base de datos.

Qué envía y cuándo:

- Solo los cambios de cuentas más recientes que `hook_local_profile_account_age_days` (**Configuración del servidor** > **Federación** >
  **Revisión externa**, 7 días por defecto; `0` envía los de todas las cuentas).
- Solo un cambio de lo que ve el hook: el nombre visible, la biografía, los campos del perfil, el avatar o la cabecera. Un cambio
  que solo bloquea la cuenta o modifica su configuración de descubrimiento no se envía.
- El nombre visible, la biografía tal como se escribió y tal como la renderiza el servidor, el nombre y el valor de cada campo del perfil (tal como se escribieron y tal como se
  renderizan), si el cambio establece, sustituye o quita el avatar o la cabecera, la dirección y el nombre de usuario de la cuenta,
  y cuándo se creó la cuenta. No se envía ninguna imagen. El perfil es público, así que el hook puede usar TCP sin cifrar.
- La clave del sujeto es el ID de la cuenta, `#` y el ID de la revisión del perfil que guardaría el cambio, de modo que cada cambio
  es un sujeto propio.

Cuando aplica, `reject` rechaza solo ese cambio: el perfil queda como estaba, y el titular ve un texto fijo («Este
cambio de tu perfil no se guardó.»), nunca el motivo ni la nota. No hay apelación: el titular puede modificar el perfil
y guardarlo de nuevo. `flag` guarda el cambio y presenta una denuncia contra la cuenta, que no menciona ninguna publicación; su denunciante,
categoría y comentario son los de [la denuncia de una publicación](#what-inboundstatus-reviews).

<!-- source: modes 6133f824d202 -->
## Modos

Cada hook tiene un modo, una [configuración de la instancia](instance-settings.md) (`hook_inbound_status_mode` y así sucesivamente) que cambias en
**Configuración del servidor** > **Federación** > **Revisión externa**. Los cambios constan en el registro de auditoría como `setting_changed`.

| Modo | Qué hace |
| --- | --- |
| `off` | No llama a nada. |
| `observe` | Registra el veredicto y no actúa sobre nada. Es el valor por defecto de todos los hooks. |
| `enforce` | Actúa según el veredicto. |

Un hook pasa de `off` a `enforce` solo a través de `observe`.

<!-- source: when-the-plugin-fails 7c987d71eb78 -->
## Cuando el plugin falla

Un tiempo de espera agotado, un error y una respuesta no válida se registran como resultados propios (`timeout`, `error`, `invalid`), nunca como
veredictos. Tras 5 llamadas fallidas seguidas, el servidor omite el plugin durante 30 segundos y luego prueba una llamada, de modo que una caída
no cuesta ninguna espera a cada sujeto; una llamada omitida se registra como `breaker_open`. Cada proceso del servidor cuenta por su cuenta.

Lo que ocurre entonces con el sujeto depende de `hook_<hook>_on_failure`:

- `pass` lo deja pasar sin revisar. Puede colarse spam durante una caída, pero nada más se detiene. Es el valor por defecto.
- `retry` hace fallar el trabajo, que se ejecuta de nuevo y vuelve a llamar al plugin, y no registra ninguna decisión. Los trabajos de un mismo actor se ejecutan
  en orden, así que todo lo posterior de ese actor espera mientras tanto, incluidas las eliminaciones, hasta unas 4 horas y 15 minutos.
  Tras su última ejecución, el trabajo pasa a ser un [trabajo muerto](running.md#the-job-queue-and-failed-dead-jobs), lo que libera a los demás.
- `require_approval` retiene el registro para un moderador.

En `media.check`, `pass` deja el archivo listo sin revisar. `retry` mantiene la subida como aún en procesamiento hasta que el plugin
responda o el trabajo agote sus ejecuciones; mientras tanto el archivo nunca está listo, y un trabajo muerto lo deja sin procesar, y la limpieza de
subidas sin adjuntar lo elimina después. Ningún otro trabajo lo espera.

Cada proceso hace como máximo 8 llamadas a un plugin a la vez, y una llamada que espera su turno más allá del plazo del hook cuenta como un
tiempo de espera agotado. Las esperas se ven en `my.plugin.slot_wait` ([Running](running.md)). Mantén corto `plugin_inbound_status_timeout`: mientras una llamada
espera, los trabajos posteriores del mismo actor esperan con ella.

En `observe` un fallo nunca retiene nada. Los moderadores reciben una notificación una vez al día por hook cuando sus llamadas siguen fallando.
Las métricas son `my.hook.decisions`, `my.hook.failures` y `my.hook.duration` ([Running](running.md)).

<!-- source: the-decisions 4d41f7512468 -->
## Las decisiones

El servidor registra una decisión por hook y sujeto, de modo que un trabajo que se ejecuta de nuevo la lee y no vuelve a llamar al plugin.
Una decisión contiene el hook, la clave del sujeto, el modo, el resultado, el motivo, la nota y cuánto tardó la llamada. No
contiene ningún texto de una publicación, ninguna dirección de correo electrónico ni ninguna dirección IP.

**Moderación** > **Revisiones automáticas** (`manage_reports`) muestra los recuentos y las esperas de los últimos 7 días, y lo que los hooks
detuvieron, habrían detenido en `observe` o no pudieron comprobar. Cada decisión se abre en su propia página con su motivo, nota y
espera. Cancelar una decisión, desde la lista o desde su página, consta en el registro de auditoría como `hook_decision_overridden`. Un `reject` de `local.status` no se cancela ahí: su autor [apela](#appeals).

Lo que hace cancelar depende de la respuesta. Un `mark_sensitive` pierde su marca, y una marca de una acción de moderación sobre la misma
publicación permanece. Un `flag` conserva su denuncia, que resuelves como de costumbre. Un `drop` de `inbound.status` procesa de nuevo la publicación
recibida sin consultar al plugin, mientras el servidor conserve la actividad recibida (7 días); pasado ese plazo, la pantalla
lo indica y cancelar solo registra la cancelación.

Cancelar un `drop` de `inbound.follow` ejecuta de nuevo el seguimiento, sin llamar al plugin, mientras el servidor conserve
la actividad recibida (7 días). Después de eso el seguimiento no puede volver y la cancelación solo queda registrada; el
otro servidor tiene que enviar un seguimiento nuevo.

Un seguimiento solo consulta al hook cuando se registraría como un seguimiento o una solicitud de seguimiento nuevos. Retirar un seguimiento (`Undo`) nunca
consulta, ni tampoco un seguimiento que el servidor rechaza de todos modos: uno que rechaza el modo de federación, uno de una cuenta migrada o
desactivada, uno del actor de la instancia o uno entre cuentas que se bloquean mutuamente, que recibe su rechazo como
de costumbre. Un seguimiento enviado de nuevo para un seguimiento o una solicitud que ya existe tampoco consulta: el servidor lo responde como de costumbre,
y lo acepta de nuevo cuando el seguimiento ya está aceptado. Un seguimiento descartado no recibe respuesta, así que el otro servidor lo sigue mostrando
como solicitado. Enviar de nuevo la misma actividad se ignora como duplicado.

Un efecto automático consta en el registro de auditoría, con el sistema como actor, solo cuando deja estado: `mark_sensitive`, un
`flag` presentado y `require_approval` (`hook_decision_applied`). `drop` y `reject` no guardan nada, así que quedan solo en las
decisiones.

El trabajo `hook.purge_decisions` elimina las decisiones tras `hook_decisions_retention_days` días (90 por defecto, como mínimo 30). Conserva
aquellas en las que se apoya una marca de sensible sobre una publicación existente, las de una cuenta bajo una
[retención legal](moderation.md#record-retention-and-holds) y cada `reject` de una publicación nueva de `local.status` que su autor aún
pueda apelar o cuya apelación esté pendiente de revisión. Un `reject` de una edición no se puede apelar, así que desaparece cuando termina el periodo de retención.
Una apelación se va con su decisión. Eliminar una cuenta elimina sus decisiones, sus apelaciones y
su exención de `local.status`.

<!-- source: what-you-tell-your-users 2ab3b8f14096 -->
## Qué les dices a tus usuarios

Mientras un hook tiene un plugin y no está en `off`, tu página de información (`/about`) indica que se usa una revisión externa, qué
abarca y qué se le envía. El servidor lo redacta a partir de tu configuración, incluida la antigüedad de cuenta de
`hook_local_status_account_age_days` y `hook_local_profile_account_age_days` (o que se envían las de todas las cuentas, con
`0`) y si
`hook_inbound_status_unfollowed_direct` envía las publicaciones directas de autores a quienes nadie de aquí sigue.

<!-- source: what-is-sent-and-what-it-costs fd966d9abd0d -->
## Qué se envía y qué cuesta

Las llamadas, su costo y los datos enviados crecen con el volumen que recibe el servidor, sobre todo en `inbound.status`: cada publicación incluida
es una llamada, y cada revisión otra. Revisa lo que envías antes de activar un hook:

- `inbound.status` y `local.status` envían el texto de las publicaciones y rechazan TCP sin cifrar, también en loopback. `inbound.status` envía
  lo que envió el servidor remoto: el texto en cada uno de sus idiomas, la advertencia de contenido, si está marcada como sensible, cuándo
  se publicó, la dirección y la URL de la publicación, la dirección de la publicación a la que responde, las direcciones y descripciones de
  los adjuntos (sin medios), las opciones de la encuesta, las etiquetas, el autor y el servidor del autor, la visibilidad, y a cuántas cuentas
  menciona la publicación y cuántas de ellas son locales. Envía también si una cuenta local sigue al autor
  (`any_local_follower`), que es un dato sobre tus usuarios. Las publicaciones solo para seguidores (`private`) se envían igual que las públicas, y
  las directas como [más arriba](#what-inboundstatus-reviews). No envía nada de la publicación a la que responde salvo su dirección.
  `local.status` envía los campos enumerados en [Publicaciones locales](#local-posts). El mensaje `InboundStatus` del protocolo `HookService` tiene los campos.
- `signup.check` envía la dirección de correo electrónico y la dirección IP de cada solicitante, y rechaza TCP sin cifrar.
- `inbound.follow` envía las dos partes de un seguimiento.
- `inbound.actor` envía el perfil público de una cuenta de otro servidor, como se enumera en
  [Perfiles de otros servidores](#profiles-from-other-servers), y puede usar TCP sin cifrar. Se llama siempre que un perfil así
  cambia, así que su volumen sigue al número de cuentas que conoce tu servidor.
- `media.check` envía la imagen procesada más pequeña de cada subida, nunca el original, con para qué sirve el archivo, su
  tipo, tamaño y duración. Puede usar TCP sin cifrar, ya que no envía texto: una imagen procesada aún puede mostrar a una persona.
- `local.profile` envía el perfil público enumerado en [Perfiles locales](#local-profiles) y ninguna imagen. Puede usar TCP
  sin cifrar.
