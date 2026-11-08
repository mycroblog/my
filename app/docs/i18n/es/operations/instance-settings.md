<!-- source: instance-settings 4eb467f0dbca -->
# Configuración de la instancia

Las configuraciones que definen cómo se gestiona tu instancia: modo de registro, longitud de las publicaciones,
idioma predeterminado, quién puede ver las líneas de tiempo, el nombre de la instancia, la descripción, los términos, las reglas, el tema y los emojis personalizados.

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## Configuración y configuración de la instancia

| | Configuración | configuración de la instancia |
| --- | --- | --- |
| Qué | Cómo se construye el despliegue: roles, adaptadores, endpoints, nombres de host, secretos | Cómo se gestiona la instancia: modo de registro, límite de longitud de las publicaciones, idioma predeterminado, quién puede ver las líneas de tiempo, el nombre de la instancia, la descripción y los términos |
| Dónde | Variables de entorno `MY_*` y el archivo de configuración ([referencia de configuración](../reference/configuration.md)) | La base de datos. Cada cambio se conserva como una revisión |
| Cambiar | Surte efecto cuando se reinicia el proceso | Surte efecto de inmediato con `my instance set`, las pantallas de administración del cliente web o la API de cliente ([pantallas de administración y API de cliente](#admin-screens-and-the-client-api)) |
| Sin definir | El valor predeterminado del esquema (el servidor no arranca si la clave es obligatoria) | El valor predeterminado definido en el código |

- **Ninguna clave está en ambos sitios.** Ninguno anula al otro. No puedes dar una configuración de la instancia mediante una variable de entorno o
  el archivo de configuración, ni guardar un valor de configuración en la base de datos.
- Un valor que es fijo por despliegue y cambia la estructura del proceso es configuración; un valor que cambia mientras
  la instancia está en funcionamiento es una configuración de la instancia.

<!-- source: prerequisites 12391490eea0 -->
## Requisitos previos

- La configuración se da como se describe en la [referencia de configuración](../reference/configuration.md) (`my config check`).
- El esquema está aplicado a la base de datos (`my schema apply`; consulta [Base de datos](database.md)).

<!-- source: keys ae25d36239c2 -->
## Claves

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### Configuraciones (una clave, un valor)

| Clave | Tipo | Valor predeterminado | Descripción |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | El modo de registro ([registro](registration.md)). Mientras se anuncia el cierre del servicio y después de cerrarlo, y en un [servidor de una sola persona](#one-person-server), el valor efectivo es `closed`. El valor guardado no cambia, y cancelar el anuncio o desactivar `single_user` lo restaura. `GET /instance` y NodeInfo muestran el valor efectivo |
| `max_status_length` | entero 1-100000 | `500` | La longitud máxima de una publicación, en caracteres |
| `default_locale` | etiqueta de idioma BCP 47 | `en` | El idioma predeterminado. Es el idioma inicial de la interfaz de los usuarios nuevos y el recurso cuando ningún catálogo coincide con la preferencia de un usuario |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | Quién puede ver la línea de tiempo local (`GET /api/v1/timelines/local`): nadie, los usuarios con sesión iniciada o todo el mundo. `off` responde 404 y `signed_in` responde 401 a los visitantes que no han iniciado sesión; en ambos casos la navegación la oculta. Salvo que el valor sea `everyone`, la página de inicio no muestra publicaciones de ejemplo. En un [servidor de una sola persona](#one-person-server) se lee como `off` |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | Quién puede ver la línea de tiempo federada (`GET /api/v1/timelines/federated`). Los valores significan lo mismo que para la línea de tiempo local, y en un servidor de una sola persona también se lee como `off` |
| `preview_image` | ID de medio (vacío = sin definir) | vacío | La imagen de vista previa del enlace (`og:image`) para publicaciones y perfiles sin imagen. Sin definir, se usa la imagen predeterminada incluida ([vistas previas de enlaces](link-previews.md)) |
| `contact_account` | nombre de usuario de una cuenta local (vacío = sin definir) | vacío | La cuenta para contactar con los operadores. Consulta [contacto y miniatura](#contact-and-thumbnail) |
| `contact_email` | dirección de correo electrónico (vacío = sin definir) | vacío | La dirección de correo electrónico para contactar con los operadores. Es también el `Reply-To` y la firma de los correos que envía el servidor |
| `thumbnail` | ID de medio (vacío = sin definir) | vacío | Una imagen que representa la instancia (miniatura) |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | Si se conservan copias de los medios remotos. `none`: el proxy de medios retransmite cada solicitud. `avatars`: los avatares, las cabeceras y los emojis personalizados se guardan en caché en el almacenamiento de blobs durante `remote_media_cache_ttl_seconds`. `all`: todos los medios remotos. El tamaño total de la caché está limitado por la configuración `remote_media_cache_max_bytes` |
| `remote_media_cache_ttl_seconds` | entero 60-31536000 (segundos) | `86400` | Cuánto tiempo pueden conservarse los medios remotos obtenidos. Es también el `Cache-Control: max-age` de las respuestas del proxy de medios, así que las CDN y los navegadores los guardan en caché durante este tiempo |
| `remote_media_cache_max_bytes` | entero desde 1 (bytes) | `1073741824` | El límite de tamaño de las copias de medios remotos que conserva el proxy de medios. La limpieza cada hora elimina las copias caducadas y después las de acceso menos reciente que excedan este límite |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch (modo seguro). Con `true`, un GET de ActivityPub sin una firma HTTP verificable recibe `401` para objetos y colecciones, y un documento mínimo con la clave para el actor de un usuario. El actor de la instancia devuelve siempre su documento completo, de modo que cualquier servidor puede obtener la clave para verificar una firma |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | La dirección de la federación. `off`: sin federación. `inbound`: sigues y lees cuentas remotas, pero las publicaciones, los perfiles y las reacciones locales no se envían, y los seguimientos desde fuera se rechazan (`Reject`). `outbound`: aceptas seguimientos desde fuera y entregas publicaciones locales, pero no se incorporan las publicaciones ni las reacciones remotas. `both`: enviar y recibir. Un cambio surte efecto de inmediato y requiere el permiso `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | Registro dinámico de clientes (RFC 7591). Con `true`, `POST /oauth/register` acepta el registro de clientes públicos sin autenticación. Con `false`, el endpoint rechaza cada solicitud con `403` (`access_denied`); los clientes ya registrados siguen funcionando. Un trabajo periódico elimina los clientes registrados dinámicamente que no se usan, según la configuración `dynamic_client_unredeemed_retention` y `dynamic_client_idle_retention`. Un cambio surte efecto de inmediato y requiere `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | Quién puede ver la lista de dominios con acciones tomadas (`GET /api/v1/instance/domain_actions`): nadie, los usuarios con sesión iniciada o todo el mundo ([moderación](moderation.md#publishing-the-domains-you-act-on)) |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | Quién puede ver los comentarios públicos de esa lista. Quien no puede ver la lista no los ve nunca |
| `appeal_window_days` | entero 20-365 (días) | `180` | Cuántos días después de una acción puede apelar la persona afectada ([moderación](moderation.md#appeals)). Cambiarlo cambia también el plazo de las acciones ya tomadas |
| `session_origins_retention_days` | entero 90-3650 (días) | `180` | Cuántos días después de un inicio de sesión se conservan la dirección IP y el User-Agent de ese inicio de sesión ([moderación](moderation.md#record-retention-and-holds)) |
| `registration_origins_retention_days` | entero 90-3650 (días) | `180` | Cuántos días después del registro se conserva la dirección IP del registro |
| `reports_retention_days` | entero 180-3650 (días) | `365` | Cuántos días después de la última resolución se conserva una denuncia. Una denuncia a la que se refiere una acción se conserva mientras dure la acción |
| `moderation_actions_retention_days` | entero 180-3650 (días) | `365` | Cuántos días después del fin de una acción (el levantamiento, la caducidad o la revisión de una apelación, lo que ocurra más tarde) se conservan las acciones sobre cuentas y las apelaciones |
| `audit_log_retention_days` | entero 365-3650 (días) | `730` | Cuántos días después de registrarse se conserva una fila del registro de auditoría |
| `remote_status_retention_days` | entero 1-3650 (días) | `90` | Cuántos días se conserva una publicación remota después de su publicación cuando nada en esta instancia se refiere a ella (una respuesta local, un impulso, un favorito, un marcador, una notificación o un hilo con una publicación local) |
| `remote_status_max_retention_days` | entero 0-3650 (días) | `0` | El máximo de días que se conserva cualquier publicación remota después de su publicación, incluso cuando esta instancia se refiere a ella. Las respuestas, los impulsos, los favoritos y los marcadores locales permanecen y la muestran como eliminada. `0` no fija ningún límite |
| `relay_status_retention_days` | entero 1-3650 (días) | `7` | Cuántos días se conserva una publicación remota que solo trajo un relay, cuando nada en esta instancia se refiere a ella y ninguna cuenta local sigue a su autor |
| `oauth_client_rate_limits` | pares `client_id=requests` separados por comas | vacío | Límites a las solicitudes de la API de cliente de los clientes OAuth, para frenar a un cliente que sobrecarga el servidor: las solicitudes que pueden hacer todos sus usuarios juntos en 5 minutos. Un cambio tarda hasta un minuto en aplicarse |
| `search_indexing` | `allow` / `deny` | `deny` | Si los buscadores pueden indexar la instancia. Incluso con `allow`, solo se indexan los perfiles de las cuentas `indexable` y las publicaciones `public` ([buscadores y rastreadores de IA](crawlers.md)) |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | La declaración dirigida a los servicios de IA. `deny_training` rechaza el entrenamiento; `deny` rechaza el entrenamiento y el uso por parte de la IA (búsqueda con IA, asistentes, agentes) |
| `indexable_default` | `true` / `false` | `false` | El valor predeterminado de `indexable` para las cuentas nuevas. Cambiarlo no cambia las cuentas existentes |
| `auto_delete_default_enabled` | `true` / `false` | `false` | Si las cuentas nuevas empiezan con la eliminación automática de sus publicaciones antiguas activada. Junto con las demás claves `auto_delete_default_*`, es la política que recibe una cuenta nueva ([eliminación automática de publicaciones antiguas](#automatic-deletion-of-old-posts)) |
| `auto_delete_default_min_age_days` | entero 1-3650 (días) | `30` | Cuántos días después de su publicación elimina una publicación la política de una cuenta nueva |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | Si la política de una cuenta nueva conserva las publicaciones que su autor fijó, marcó o marcó como favoritas |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | Si la política de una cuenta nueva conserva las publicaciones con medios o con una encuesta |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | Si la política de una cuenta nueva conserva las publicaciones de esa visibilidad |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | Si la política de una cuenta nueva conserva las publicaciones directas |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | entero 0-1000000 | `0` | La política de una cuenta nueva conserva una publicación impulsada o marcada como favorita al menos este número de veces. `0` no fija ningún umbral |
| `auto_delete_max_age_days` | entero 0-3650 (días) | `0` | El máximo de días después de su publicación que se conserva cualquier publicación local ([máximo](#a-maximum-for-keeping-posts)). `0` no fija ningún máximo |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | Si las publicaciones fijadas pueden seguir conservándose bajo el máximo |
| `auto_delete_max_kept_pins` | entero 1-5 | `5` | Cuántas publicaciones fijadas conservan publicaciones bajo el máximo, empezando por la fijada más recientemente. `5`, el máximo de publicaciones fijadas de una cuenta, conserva todas las fijadas |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | Si las publicaciones que el autor marcó o marcó como favoritas, o con medios o con una encuesta, pueden seguir conservándose bajo el máximo |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | Si las publicaciones pueden seguir conservándose por su visibilidad bajo el máximo |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | Si las publicaciones pueden seguir conservándose por sus impulsos y favoritos bajo el máximo |
| `auto_delete_grace_days` | entero 0-365 (días) | `30` | Cuántos días conserva sus propias configuraciones una cuenta a la que el máximo se aplica por primera vez, desde que se le avisa. Un cambio no mueve las fechas de las cuentas ya avisadas |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | Qué publicaciones cubre la búsqueda de texto completo de un servidor con un backend de búsqueda. Las publicaciones `unlisted`, `private` y `direct` no se cubren nunca, y solo las cuentas con `indexable`. Dejar fuera una visibilidad elimina sus publicaciones del índice. Añadir una encuentra solo las publicaciones nuevas de ella, hasta que [reconstruyas el índice](search.md#changing-search_visibilities). No hace nada con `search = none` |
| `security_contact` | URI `https:` o `mailto:` (vacío = sin definir) | vacío | Dónde informar de problemas de seguridad de este servidor: cómo está configurado y se ejecuta, y sus cuentas y datos. Cuando está definido, el servidor sirve `/.well-known/security.txt` (RFC 9116). Este es tu contacto, no el del software: reenvía al proyecto el informe de una vulnerabilidad del propio software como describe su [política de seguridad](../../SECURITY.md) |
| `require_media_description` | `true` / `false` | `false` | Si cada archivo multimedia (imagen, video o audio) adjunto a una publicación local necesita una descripción. Con `true`, se rechaza con 422 una publicación o una edición con un archivo cuya descripción falta o es solo espacios en blanco. Se publica como `limits.require_media_description` en `GET /api/v1/instance`, y el cliente web hace obligatorio el campo de descripción. Es independiente de la configuración por usuario `require_media_description`, que solo comprueba antes de enviar |
| `link_cards` | `true` / `false` | `true` | Si se muestran tarjetas para los enlaces de las publicaciones. Con `true`, el servidor obtiene la página enlazada para crear una tarjeta. Con `false`, deja de obtenerla y también oculta las tarjetas ya creadas. Para detenerlo solo en algunos dominios, consulta [dominios de tarjetas de enlace](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | Si las obtenciones de tarjetas de enlace omiten `robots.txt`. Con `false`, el servidor obedece el `robots.txt` de los sitios enlazados, así que un sitio que no permite `my-linkcard` o `*` no obtiene tarjeta ([vistas previas de enlaces](link-previews.md#cards-of-links-in-posts)). `/about/crawler` indica a los sitios enlazados cuál es el caso. Con `true`, el servidor ni obtiene ni usa `robots.txt`, y un sitio enlazado ya no puede rechazar estas obtenciones con él; el servidor sigue identificándose como `my-linkcard`. Actívalo solo cuando falten demasiadas tarjetas. Las tarjetas creadas mientras estaba activado se comprueban en su siguiente nueva obtención después de desactivarlo. Las directivas de una página y los dominios de tarjetas de enlace se aplican en ambos casos |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Si se aceptan clientes con Client ID Metadata Document. Con `true`, para una solicitud de autorización cuyo `client_id` es una URL `https`, el servidor obtiene los metadatos en esa URL y los trata como el cliente (sin registro previo). La obtención no sigue redirecciones, lee como máximo 5 KB y reutiliza la respuesta durante 24 horas como máximo, según sus directivas de caché. Con `false`, deja de obtenerlos y rechaza las solicitudes de autorización, los intercambios de tokens y las renovaciones de estos clientes |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | Cuánto de los costos de funcionamiento (registrados en `/admin/finance/costs` y definidos en `/admin/finance/publication`) se publica. `private`: no se publica nada (`GET /api/v1/instance/cost_totals` es 404). `totals`: solo el total por periodo y moneda. `breakdown`: los totales y un desglose por proveedor y categoría, mostrados a todo el mundo en las páginas de información (`/about` y `/about/costs`) y en la API. `breakdown` revela en qué proveedor (por ejemplo, qué nube) te ejecutas. En todos los niveles, los nombres de los conceptos, las notas y los pagos individuales siguen siendo privados. Se publican los periodos hasta el que contiene el día de hoy (UTC) inclusive |
| `finance_viewer_item_names` | `true` / `false` | `false` | Si los usuarios con el permiso `view_finance` ven los nombres de los conceptos de costo y los nombres de servicio de los costos importados en la pantalla de finanzas (`/finance`, `GET /api/v1/finance/cost_totals`). Los totales y el desglose por proveedor y categoría se muestran siempre, sea cual sea esta configuración |
| `finance_viewer_item_memos` | `true` / `false` | `false` | Si los usuarios con `view_finance` ven las notas de los conceptos de costo en la pantalla de finanzas. Si la configuración del nombre o la de la nota es `true`, cada periodo muestra filas de conceptos (proveedor, categoría y el importe de ese periodo, más el nombre y la nota donde estén activados). Quienes tienen `manage_finance` lo ven todo en cualquier caso |
| `finance_viewer_income` | `totals` / `plans` | `totals` | Si los usuarios con `view_finance` ven solo los totales de ingresos (`totals`) o también el número de suscriptores por plan (`plans`). El importe por plan y quién pagó no se muestran nunca. Si `income_visibility` es `plans`, los recuentos publicados se muestran sea cual sea este valor. Los ingresos se leen en la pantalla de finanzas del cliente web (`/finance`) y en `GET /api/v1/finance/income_totals` |
| `income_visibility` | `private` / `totals` / `plans` | `private` | Cuánto de los ingresos de los mecenas (el importe tras las comisiones del proveedor de pagos) se publica. Se elige por separado de `finance_visibility`. `private`: no se publica nada (`GET /api/v1/instance/income_totals` es 404). `totals`: solo el total de ingresos por periodo y moneda. `plans`: también el número actual de suscriptores por plan, mostrado a todo el mundo. En un plan con pocos suscriptores, el recuento puede permitir adivinar quién se suscribe. El balance (ingresos menos costos) se muestra solo cuando ni este valor ni `finance_visibility` son `private`; de lo contrario, el balance revelaría el lado que ocultaste. En todos los niveles, los pagos individuales, los suscriptores, el importe por plan y los importes brutos y comisiones siguen siendo privados. Se publican los periodos hasta el que contiene el día de hoy (UTC) inclusive |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | Quién puede ver la lista de mecenas (`GET /api/v1/instance/supporters` y `/about/supporters`). `off`: nadie (la API responde 404 y `/about` no tiene enlace). `signed_in`: solo quienes han iniciado sesión en este servidor (en caso contrario la API responde 401). `everyone`: todo el mundo. La lista muestra solo las cuentas y los planes de los suscriptores activos que eligieron figurar en ella; los importes, la facturación mensual o anual, las fechas y los recuentos no se muestran nunca. La página de la lista y la respuesta de la API son siempre `noindex, noarchive`, sea cual sea `search_indexing` |
| `user_app_registration` | `true` / `false` | `true` | Si los usuarios con sesión iniciada pueden registrar sus propias aplicaciones OAuth (públicas o confidenciales) con la API de cliente (`POST /api/v1/oauth_apps`). Con `false`, registrar responde 403. Listar, cambiar y eliminar las aplicaciones registradas, volver a emitir secretos, emitirte un token a ti mismo y la autorización por otros usuarios siguen funcionando |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | Quién puede abrir salas de chat de voz, mientras la configuración `voice` no sea `off` ([configuración](../reference/configuration.md#voice)). `everyone`: todo usuario local con sesión iniciada que no esté suspendido ni limitado. `permitted`: solo quienes tienen el permiso `start_voice_rooms`, que por defecto solo tiene el rol integrado `admin` y que puede tener un rol de un plan de mecenas |
| `voice_public_summary` | `true` / `false` | `false` | Si la página de una sala de chat de voz cuya visibilidad es `local` muestra su nombre, descripción, anfitrión y si sigue abierta a los visitantes que no han iniciado sesión en este servidor y a los usuarios remotos, y en su vista previa de enlace. Los participantes no se muestran nunca. Con `false`, esos visitantes solo ven que la sala es para los usuarios locales de este servidor |
| `public_counts` | `true` / `false` | `true` | Si la página de inicio (`/`, sin sesión) y `GET /api/v1/instance/overview` muestran cuántos miembros tiene el servidor, cuántas publicaciones públicas hicieron sus usuarios cada uno de los últimos 7 días y cuándo arrancó el servidor. Solo cuentan las publicaciones públicas de los usuarios de este servidor; las cifras se actualizan cada pocos minutos. Con `false`, la API las omite |
| `featured_accounts` | nombres de usuario de hasta 20 cuentas locales, separados por espacios o comas (vacío = ninguna) | vacío | Las cuentas que las sugerencias de personas a las que seguir muestran primero, en este orden, antes de las personas de este servidor que publicaron recientemente. Una cuenta cuyo propietario desactivó «Sugerir mi cuenta y permitir encontrarla por parte de su nombre» no se sugiere. Consulta [sugerencias](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | Si el registro y el inicio de sesión se limitan a los proveedores de inicio de sesión mientras haya al menos uno activado: las contraseñas y las claves de acceso (passkeys) ya no inician sesión, y los usuarios sin un proveedor vinculado no pueden volver a iniciar sesión cuando terminen sus sesiones. Sin ningún proveedor activado no se aplica. `my instance set` imprime cuántos usuarios activos no tienen vínculo con un proveedor activado. Consulta [limitar el inicio de sesión a los proveedores de inicio de sesión](login-providers.md#limit-sign-in-to-login-providers). |
| `invite_creators` | `everyone` / `permitted` | `permitted` | Quién puede crear invitaciones propias además del personal con `manage_invites` ([invitaciones de miembros](registration.md#member-invites)). `everyone`: todo usuario local con sesión iniciada que no esté suspendido ni limitado. `permitted`: solo quienes tienen el permiso `create_invites`, que por defecto solo tiene el rol integrado `admin` y que puede tener un rol de un plan de mecenas. En el modo `approval`, una invitación utilizable aprueba al usuario nuevo de inmediato, así que `everyone` permite que todos los usuarios eviten la revisión |
| `member_invite_max_active` | entero 0-100 | `5` | Cuántas invitaciones utilizables (no revocadas, caducadas ni agotadas) puede tener un miembro a la vez; `0` no deja crear ninguna a los miembros. Las invitaciones del personal no cuentan |
| `member_invite_max_uses` | entero 1-1000 | `10` | Cuántas personas pueden registrarse con una invitación creada por un miembro |
| `member_invite_max_lifetime_seconds` | entero 3600-31536000 (segundos) | `2592000` (30 días) | Cuánto tiempo puede funcionar como máximo una invitación creada por un miembro. Reducirlo no acorta las invitaciones existentes |
| `translation_providers` | IDs de proveedor separados por comas | vacío | Los proveedores de traducción del servidor que están activados, probando primero el primero. Cuando está vacío no se envía nada a ningún proveedor. Un ID para el que este servidor no tiene proveedor no hace nada. Los [proveedores de traducción](translation.md) integrados tienen los IDs `libretranslate`, `deepl`, `google` y `aws`, y un [plugin traductor](plugins.md#translating-posts) el ID `plugin` |
| `translation_provider_visibilities` | pares `provider=visibility\|visibility` separados por comas | vacío | Qué puede recibir cada proveedor. Un proveedor no nombrado recibe publicaciones `public`, `local` y `unlisted`; `provider=` sin nada detrás no recibe ninguna. `private` y `direct` van solo a un proveedor nombrado con ellos, así que permitírselo a un proveedor no se lo permite al siguiente |
| `translation_browser` | booleano | `false` | Si los lectores pueden usar la traducción integrada en su navegador, que no envía nada a este servidor ni a un proveedor. Con un proveedor del servidor también activado, el proveedor va primero y el navegador traduce lo que el servidor rechaza: una publicación o un idioma que ningún proveedor acepta, un límite mensual o un proveedor que falla |
| `translation_user_monthly_characters` | entero 0-1000000000000 | `50000` | Cuántos caracteres pueden enviar a los proveedores de traducción las solicitudes de un usuario en un mes UTC. Con `0` no se envía nada. No hay un valor ilimitado: define un número grande |
| `translation_server_monthly_characters` | entero 0-1000000000000 | `500000` | Cuántos caracteres puede enviar todo el servidor a los proveedores de traducción en un mes UTC. Con `0` no se envía nada. No hay un valor ilimitado: define un número grande |
| `translation_retention_days` | entero 1-3650 (días) | `30` | Cuántos días se conserva una traducción después de hacerse. Leerla no lo prolonga, y desaparece con la publicación |
| `translation_reports_to_server` | booleano | `false` | Si los moderadores pueden enviar el texto de una denuncia, las publicaciones denunciadas y el comentario de quien denuncia, a los proveedores del servidor. Cada proveedor sigue recibiendo solo las visibilidades que `translation_provider_visibilities` le permite, quienquiera que lo pida: una publicación directa denunciada va solo a un proveedor con `direct` permitido, y el comentario cuenta como `private`. La traducción del comentario no se conserva nunca, ni tampoco la de una publicación eliminada, y los caracteres cuentan solo contra `translation_server_monthly_characters`. Con `false`, nada de una denuncia llega a un proveedor |
| `voice_participants_retention_days` | entero 30-3650 (días) | `90` | Cuántos días después de que terminó una sala de chat de voz se conservan los registros de quién participó: las participaciones, los oradores y oyentes, las manos levantadas, las invitaciones y las expulsiones. La sala en sí (su nombre, descripción, anfitriones y cómo terminó) se conserva. Los registros de una sala denunciada se conservan mientras dure la denuncia, y las retenciones se aplican como a los demás registros |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | Hasta dónde actúa cada [hook de decisión](decision-hooks.md#modes) mientras tiene un plugin. `off` pasa a `enforce` solo a través de `observe`. |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | Qué hace el hook cuando aplica la decisión y su plugin falla ([detalles](decision-hooks.md#when-the-plugin-fails)). |
| `hook_inbound_status_unfollowed_direct` | booleano | `false` | Si `inbound.status` envía también las publicaciones directas de autores a los que no sigue ninguna cuenta local ([detalles](decision-hooks.md#what-inboundstatus-reviews)). La página de información (`/about`) lo indica mientras el hook está en uso. |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | Qué hace `signup.check` cuando aplica la decisión y su plugin falla. |
| `hook_signup_check_reject` | booleano | `false` | Si un `reject` de `signup.check` [rechaza el registro](decision-hooks.md#sign-ups) mientras aplica la decisión. Desactivado, la respuesta solo se registra, como en `observe`. |
| `hook_local_status_account_age_days` | entero 0-3650 (días) | `7` | `local.status` [revisa las publicaciones y sus ediciones](decision-hooks.md#local-posts) de las cuentas más jóvenes que esto; `0` revisa todas las cuentas. La página de información (`/about`) lo indica mientras el hook está en uso. |
| `hook_local_profile_account_age_days` | entero 0-3650 (días) | `7` | `local.profile` [revisa los cambios de perfil](decision-hooks.md#local-profiles) de las cuentas más jóvenes que esto; `0` revisa todas las cuentas. La página de información (`/about`) lo indica mientras el hook está en uso. |
| `hook_decisions_retention_days` | entero 30-3650 (días) | `90` | Cuántos días se conservan las [decisiones de los hooks](decision-hooks.md#the-decisions). |
| `single_user` | booleano | `false` | Si el servidor es un [servidor de una sola persona](#one-person-server). Solo puede activarse mientras el servidor tiene exactamente una cuenta local, y puede desactivarse en cualquier momento |

- Los valores se normalizan antes de guardarse (`TRUE` pasa a `true`, `0750` pasa a `750`, `ja_jp` pasa a `ja-JP`, los IDs de medios se pasan a minúsculas).
  Un valor no válido se rechaza y no se escribe nada.
- `preview_image` y `thumbnail` aceptan solo el medio de una imagen que existe (incluida una que aún se está procesando).
  Hasta que termina el procesamiento, y en lugar de una imagen cuyo procesamiento falló, `preview_image` recurre a la imagen predeterminada.
  Una vez que un medio se ha definido en cualquier revisión, la limpieza de subidas sin adjuntar no lo elimina.
- `contact_account` acepta solo el nombre de usuario de un usuario local que no esté eliminado (se guarda en minúsculas). `contact_email`
  acepta solo una dirección con la forma `alice@example.com`.
- `featured_accounts` acepta solo los nombres de usuario de usuarios locales que no estén eliminados; se elimina una `@` inicial, los nombres
  se pasan a minúsculas, un nombre repetido se conserva una vez y se guardan separados por espacios simples.

<!-- source: suggestions-of-accounts b7d362466e33 -->
### Sugerencias de cuentas

Los usuarios con sesión iniciada ven sugerencias de personas a las que seguir (en la línea de tiempo de inicio vacía tras el registro, y con
`GET /api/v1/accounts/suggestions`). Las sugerencias son las cuentas de `featured_accounts` en su orden y después las
cuentas de este servidor que publicaron una publicación pública o local en los últimos 30 días, empezando por la que publicó más recientemente. Se excluyen
la propia cuenta del usuario, las cuentas que el usuario sigue, bloquea o silencia, o por las que está bloqueado, y las cuentas suspendidas o
desactivadas.

Las cuentas nuevas se sugieren y pueden encontrarse por parte de su nombre en la búsqueda de cuentas. Un usuario que desactiva «Sugerir mi
cuenta y encontrarla por parte de su nombre» en la configuración del perfil queda fuera de ambas: la búsqueda encuentra entonces la cuenta solo por su
nombre de usuario completo, o para las personas que la siguen.

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### Eliminación automática de publicaciones antiguas

Los usuarios pueden hacer que sus propias publicaciones se eliminen automáticamente unos días después de publicarlas (Configuración, «Eliminar publicaciones antiguas»).
La política de cada usuario es suya: si está activada, después de cuántos días y qué publicaciones conserva (las publicaciones fijadas, marcadas y
marcadas como favoritas, las publicaciones con medios o con una encuesta, las publicaciones de ciertas visibilidades y las publicaciones con al menos cierto número de impulsos o
favoritos). Eliminar una publicación de este modo equivale a que su autor la elimine: los servidores a los que llegó reciben un `Delete`.
De este modo solo se eliminan las publicaciones de cuentas locales, nunca los impulsos.

Las claves `auto_delete_default_*` son la política con la que empieza cada cuenta local nueva, se cree como se cree
(el formulario de registro, un proveedor de inicio de sesión externo, una invitación o `my admin`). La cuenta la recibe al crearse y
pasa a ser suya: cambiar las claves más adelante no cambia ni las cuentas existentes ni las que recibieron un valor anterior.
La pantalla de configuración muestra los valores como propios del usuario. Con las claves sin definir, una cuenta nueva empieza con la eliminación
automática desactivada, a los 30 días, conservando las publicaciones fijadas, marcadas y marcadas como favoritas, y las publicaciones directas. El valor predeterminado no se muestra
a las personas ajenas a la instancia. Cuando hay un máximo definido, una cuenta nueva recibe el valor predeterminado recortado al máximo, como se
describe más abajo, de modo que empieza dentro de él.

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### Un máximo para conservar publicaciones

Con `auto_delete_max_age_days` por encima de `0`, ninguna publicación local se conserva más de ese número de días después de su publicación.
Para toda cuenta local sin el permiso `keep_posts_longer`:

- La eliminación automática está siempre activada: un usuario no puede desactivarla ni elegir más días que el máximo (menos está
  bien). La pantalla de configuración y `PUT /api/v1/preferences/auto_delete` rechazan ambas cosas.
- Solo conservan publicaciones las excepciones que permiten las claves `auto_delete_max_allow_*`. Por defecto solo lo hacen las publicaciones fijadas, todas
  ellas; con `auto_delete_max_kept_pins` por debajo de 5, solo conservan sus publicaciones esa cantidad de las fijadas más recientemente.
- Los usuarios que desactivaron la eliminación automática, o nunca la definieron, también están sujetos al máximo.

Activar el máximo, reducirlo o cambiar las excepciones no reescribe las configuraciones de nadie. En cambio, cada hora el
trabajo `content.auto_delete_statuses` encuentra las cuentas cuyas propias configuraciones conservan publicaciones que el máximo no conserva (porque
el máximo se activó o se redujo, o la cuenta perdió `keep_posts_longer`). Avisa a cada una de ellas una vez, con una
notificación y un correo electrónico que están activados por defecto, y desde cuándo se aplica el máximo: `auto_delete_grace_days` después
del aviso. Hasta entonces las configuraciones propias de la cuenta siguen en vigor; a partir de entonces sus publicaciones se rigen por el máximo. Una cuenta
que vuelve a estar dentro del máximo (su usuario cambia las configuraciones, obtiene `keep_posts_longer`, o el máximo desaparece o
sube) deja de estar sujeta, y un cambio posterior vuelve a avisarla con un nuevo periodo de gracia.

Activar el máximo avisa en el plazo de una hora a todas las cuentas a las que afecta. Las eliminaciones mantienen su ritmo: como máximo 100 publicaciones
de una cuenta por hora, cada una enviada como un `Delete` a los servidores a los que llegó la publicación, así que una cuenta con muchas publicaciones antiguas
tarda horas o días en vaciarse. Un usuario que cambie sus configuraciones mientras tanto detiene el resto.

`keep_posts_longer` es un permiso de miembro: dáselo a un rol para que quienes lo tengan conserven sus publicaciones el tiempo que elijan
y desactiven la eliminación automática. El rol integrado `admin` lo tiene. Un [plan de mecenas](supporters.md) puede incluirlo, de modo que
conservar las publicaciones más tiempo puede ser un beneficio del plan; cuando termina la suscripción, el máximo se aplica de nuevo tras el periodo
de gracia.

El máximo es público: `GET /api/v1/instance` lo devuelve en `auto_delete_limit`, y la página de información lo muestra, para que
la gente lo sepa antes de registrarse. Los cambios de estas claves requieren `manage_settings` y se registran en el registro de auditoría como cualquier otro
configuración. En el cliente web están en la página «Eliminación de publicaciones» de «Configuración del servidor», que pregunta antes de guardar un
máximo que se activa o se endurece.

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### Textos (una clave, un valor)

| Clave | Límite | Descripción |
| --- | --- | --- |
| `name` | 100 caracteres, una línea (no se permiten solo espacios en blanco) | El nombre de la instancia (nombre para mostrar). Consulta [nombre y dominio](#name-and-domain) |
| `short_description` | 500 caracteres | Una descripción breve |
| `description` | 20000 caracteres | La descripción |
| `terms` | 100000 caracteres | Los términos del servicio (Markdown) |
| `privacy_policy` | 100000 caracteres | La política de privacidad (Markdown) |
| `commerce_disclosure` | 100000 caracteres | La información del vendedor para los planes de pago, como el aviso que exige la ley japonesa (Markdown; [mecenas](supporters.md)) |
| `preview_image_alt` | 1500 caracteres | El texto alternativo de la imagen `preview_image` (`og:image:alt`). Sin definir, se usa «Imagen de {instance name}» |
| `thumbnail_alt` | 1500 caracteres | El texto alternativo de la imagen `thumbnail` |

- Cada texto tiene un valor. Un administrador lo escribe una vez en el idioma que quiera, y los lectores de todos los idiomas ven el
  mismo valor; los lectores de otros idiomas recurren a la traducción automática o similar. Las etiquetas y las descripciones de los campos
  de administración se traducen al idioma del usuario como cualquier otro texto de pantalla; solo el valor que escribe un administrador no se traduce.
- El servidor no conoce el idioma de un texto, así que el cliente web y las pantallas que genera el servidor muestran los textos con
  `dir="auto"` y `lang=""` (el valor HTML para un idioma desconocido) en lugar de heredar el idioma de la pantalla.
- Definir un texto vacío lo elimina.
- Además de la CLI, los textos pueden cambiarse en las pantallas de administración del cliente web (nombre, descripción breve y descripción en
  la página General de «Configuración del servidor», `/admin/settings`; los términos en la primera página de «Términos y políticas»,
  `/admin/documents`, la política de privacidad en `/admin/documents/privacy-policy`, y la información del vendedor y similares en
  `/admin/documents/commerce-disclosure`) y con la API de cliente (`GET /api/v1/admin/instance_texts`, `PUT /api/v1/admin/instance_texts/{key}`).

  Ambos requieren el permiso de rol `manage_settings`, y los cambios se registran en el registro de auditoría (`text_changed`).

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### Términos y política de privacidad

- Escribe `terms`, `privacy_policy` y `commerce_disclosure` en Markdown (CommonMark más las tablas, el tachado
  y los enlaces automáticos de GitHub Flavored Markdown). Un salto de línea dentro de un párrafo sigue siendo un salto de línea, y los encabezados `#` empiezan un nivel por debajo del
  título de la página (`h2`).
- El servidor los convierte a HTML y lo sanea (`GET /api/v1/instance/documents/{kind}` anónimo), y las páginas
  del cliente web `/about/terms`, `/about/privacy` y `/about/commerce` los muestran. Se eliminan el HTML en bruto, las imágenes y los enlaces que no sean `http`,
  `https` y `mailto`. Las páginas no necesitan iniciar sesión (son páginas del cliente web, así que necesitan JavaScript).
  `/about/commerce` es la URL que hay que dar al proveedor de pagos.
- Un documento que escribes se enlaza desde la pantalla de registro, el pie de cada pantalla que genera el servidor y el cliente web (la tercera columna, el menú, la configuración y la pantalla de inicio de sesión).
  Un documento que no está escrito no se enlaza, y su API responde 404.
- Mientras estén escritos los términos del servicio o la política de privacidad, la pantalla de registro los lista antes del botón de crear
  con una casilla obligatoria, «He leído y acepto los documentos anteriores». El servidor rechaza un registro sin ella,
  también el que se hace mediante un proveedor de inicio de sesión. Sin ninguno de los dos documentos, la pantalla no pide ninguna aceptación.

<!-- source: name-and-domain 1e14aa3e6c58 -->
### Nombre y dominio

Una instancia tiene un dominio y un nombre, por separado.

- El **dominio** es la configuración `host` (y `account_domain`, el dominio de los identificadores de cuenta). No puede cambiarse mientras la
  instancia está en funcionamiento. Los identificadores (`@alice@example.com`), las URL, los ID de ActivityPub y `domain` en `GET /api/v1/instance` siguen siendo
  el dominio aunque haya un nombre.
- El **nombre** es el texto `name`. Aparece dondequiera que se nombra la instancia a las personas: la barra lateral y «Acerca de este servidor» del
  cliente web, el título de la pestaña del navegador, las pantallas de inicio de sesión, registro, confirmación de correo electrónico, estado de la cuenta y consentimiento de autorización
  que genera el servidor, los correos electrónicos (confirmación de la dirección y avisos a los moderadores), `og:site_name` de las vistas previas de
  enlaces, `name` / `short_name` del Web App Manifest, `metadata.nodeName` de NodeInfo y `name` en `GET /api/v1/instance`.
- El nombre es el mismo en todos los idiomas. Cuando no hay nombre, se muestra en su lugar el dominio (`host`).
- Un cambio surte efecto desde la siguiente solicitud, sin reiniciar. El título de la pestaña y la barra lateral de un cliente web abierto se actualizan cuando
  guardas en la pantalla de administración.

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### Contacto y miniatura

El contacto de los operadores (`contact_account`, `contact_email`) y la imagen que representa la instancia (`thumbnail`
y el texto `thumbnail_alt`) aparecen en los lugares siguientes. Lo que no esté definido no aparece.

- «Acerca de este servidor» del cliente web (`/about`). La miniatura y la cuenta de contacto aparecen también en la primera pantalla (`/`)
  para las personas que no han iniciado sesión.
- `contact` (`account` y `email`) y `thumbnail` (`url`, `width`, `height`, `alt`) en `GET /api/v1/instance`.
- `metadata` en NodeInfo: `maintainer` según la convención de Misskey (`name` es el nombre para mostrar de la cuenta de contacto, o su
  nombre de usuario si no tiene; `email` es `contact_email`), `staffAccounts` según la convención de Pleroma y Akkoma (el ID de actor de
  la cuenta de contacto), `thumbnail` (`url` y `alt`) y `nodeDescription` a partir de la descripción breve `short_description`.

Cuando la cuenta de contacto está suspendida o eliminada, la cuenta no se muestra y solo se muestra la dirección de correo electrónico.
Cámbialos en la página General de la pantalla de administración del cliente web («Configuración del servidor», `/admin/settings`), o con la CLI o la API de cliente.

<!-- source: one-person-server 6ba524c58a4f -->
### Servidor de una sola persona

Un servidor solo de su operador, el hogar propio del operador en el fediverso, es un servidor de una sola persona: activa `single_user`. En el cliente web,
un administrador lo activa y desactiva con **Convertir en un servidor de una sola persona** en la página **Registro** de **Configuración del servidor**,
que explica por qué el servidor lo rechazó cuando hay más de una cuenta local.

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- Solo puede activarse mientras cuenta exactamente una cuenta local. Cuenta toda cuenta local que no esté eliminada,
  incluidas las pendientes de aprobación, desactivadas, programadas para eliminación, suspendidas o limitadas, ya que cada una podría volver como
  una segunda persona. Una cuenta cuyo registro fue rechazado no cuenta. Con cualquier otro número, `my instance set`
  falla indicando el recuento, y la API de administración responde `422` con el tipo `/api/v1/problems/single-user-requires-one-account`.
  El actor de la instancia del servidor no es una cuenta y no cuenta nunca.
- Puede desactivarse en cualquier momento. Activarlo o desactivarlo no elimina nada y no cambia el valor guardado de ninguna otra configuración:
  lo que retiene se aplica de nuevo tal como estaba cuando se desactiva. Como toda configuración, el cambio va al registro de auditoría.

Mientras está activado:

- **El registro está cerrado.** `registration_mode` se lee como `closed` sea lo que sea que guarde, así que la página de registro, el registro mediante un
  proveedor de inicio de sesión y `my admin create` no crean ninguna cuenta. Iniciar sesión con un proveedor de inicio de sesión sigue funcionando.
- **Solo la línea de tiempo de inicio.** `local_timeline_visibility` y `federated_timeline_visibility` se leen como `off`. Las líneas de tiempo de etiquetas y
  de listas permanecen.
- **Se oculta lo que necesita una comunidad.** Las líneas de tiempo local y federada y sus streams, las sugerencias de cuentas,
  las invitaciones de usuarios y de administradores, la revisión de registros y los bloqueos de registro responden `404` con el tipo
  `/api/v1/problems/single-user-mode`.
- **No hay publicaciones locales nuevas.** En un servidor de una sola persona, una publicación para «Este servidor» no llega a nadie salvo a los seguidores, así que una nueva
  publicación no puede ser local: la API de cliente responde `422` y la deja fuera de lo que una cuenta puede publicar. Una visibilidad
  predeterminada de «Este servidor» se lee como «Seguidores» y no puede guardarse, y una sala de chat de voz para las personas de este servidor se anuncia
  en una publicación para los seguidores. Las publicaciones que ya son locales siguen siendo locales.
- **El servidor dice lo que es.** `single_user` en `GET /api/v1/instance` es `true`, con el modo de registro efectivo
  y las líneas de tiempo efectivas y la cuenta del operador como `single_user_account`, y NodeInfo responde `openRegistrations: false` con
  `metadata.singleUser: true`.
- **La portada es el perfil del operador.** Las personas que no han iniciado sesión y abren la dirección del servidor ven el perfil
  de su única cuenta, con la forma de iniciar sesión y enlaces a la página de información, los términos del servicio y la política de privacidad
  que publica el servidor. El cliente web oculta lo que el servidor no ofrece, de su navegación, sus menús y sus
  pantallas de administración: las líneas de tiempo local y federada, el registro, las cuentas sugeridas, las invitaciones, la revisión de
  registros, los bloqueos de registro, la visibilidad «Este servidor» y las configuraciones de registro, invitaciones, líneas de tiempo
  públicas, página de inicio y cuentas sugeridas.

La federación, la moderación, la búsqueda de texto completo, el programa de mecenas, los documentos y las pantallas de administración funcionan como en cualquier
servidor.

<!-- source: usage 68ad8384438f -->
## Uso

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- Escribe las opciones antes de la clave y el valor.
- Con `-owner <username>`, el valor de `preview_image` o `thumbnail` se lee como la ruta de un archivo de imagen y se sube como
  medio de esa cuenta local. El worker procesa la imagen antes de usarla. Se recomienda una imagen apaisada de 1200x630.
- Un cambio registra una revisión (`instance_*_revisions`) y actualiza el valor actual (`instance_settings` / `instance_texts`)
  en una sola transacción. Un cambio desde la CLI se registra sin actor (`BY` es `cli` en el historial).
- Definir de nuevo el mismo valor añade igualmente una revisión.
- Códigos de salida: 0 si todo va bien, 1 por un valor rechazado o un error de la base de datos (no cambia nada), 2 por argumentos incorrectos.

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### Cambiar la dirección de la federación

- Restringirla conserva las relaciones de seguimiento, las publicaciones importadas y los registros de entrega, y no envía ningún `Undo` ni `Reject`. El flujo simplemente
  se detiene, y volver a `both` (u otro modo) lo reanuda tal como estaba.
- Las publicaciones y reacciones hechas mientras estaba restringida no se entregan con carácter retroactivo cuando la amplías de nuevo.
- En el lado remoto, los seguidores de los usuarios locales no reciben nada mientras el modo es `inbound`, y los seguimientos de los usuarios locales no se
  actualizan mientras es `outbound` u `off`.
- Las eliminaciones (de publicaciones o de cuentas) hechas mientras el modo es `off` no se envían. En `inbound` y `outbound`, la retirada
  de lo que ya se envió (`Delete` y similares) va a los destinatarios originales y a los seguidores de aquel momento.
- Los cambios se registran en `instance_setting_revisions` y en el registro de auditoría (`target_kind = setting`).

Para cortar también las relaciones, usa la CLI `my admin federation sever`.

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- Pone fin a los seguimientos pendientes y activos entre cuentas locales y remotas como terminaciones por moderación
  (`follow_terminations`, `reason = moderation`). Un cambio de modo nunca lo llama automáticamente.
- `Reject` (`in`) o `Undo` (`out`) se envía al lado remoto solo cuando el modo actual envía esa dirección. En un modo que
  no lo hace (como `off`), el seguimiento termina solo en el lado local y no se avisa al lado remoto.
- Es seguro ejecutarlo repetidamente. Imprime en la salida estándar el número de seguimientos que terminó.

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## Pantallas de administración y API de cliente

Toda configuración que puede cambiar la CLI puede cambiarse también en las pantallas de administración del cliente web y con la API de cliente. Ambas necesitan
el permiso de rol `manage_settings` (solo lo tiene el rol integrado `admin`). Como con la CLI, un cambio se conserva como una revisión
y se registra por clave en el registro de auditoría (`setting_changed`, el destino es la configuración y el actor es ese usuario). Consulta el
registro de auditoría en la pantalla de moderación («Registro de auditoría», `/moderation/audit-log`).

- **Cliente web** («Configuración del servidor», `/admin/settings`): la pantalla tiene una página por tema, enlazada bajo su encabezado. Cada
  página tiene un formulario y un botón de guardar, y al guardar se envía solo lo que cambiaste en esa página; las configuraciones de las demás páginas
  permanecen como están.

  | Página | Secciones y configuraciones |
  | --- | --- |
  | General (`/admin/settings`) | Nombre, Descripciones (`name`, `short_description`, `description`); Contacto y miniatura (`contact_account`, `contact_email`, `thumbnail` y el texto `thumbnail_alt`) |
  | Icono y logotipo (`/admin/settings/branding`) | El icono, el logotipo y el logotipo para fondos oscuros, cada uno guardado con su propio botón ([Logotipo e icono](#logo-and-icon)) |
  | Registro (`/admin/settings/registration`) | Registro (`registration_mode`, `login_providers_only`); Invitaciones de miembros (`invite_creators`, `member_invite_max_active`, `member_invite_max_uses`, `member_invite_max_lifetime_seconds`); Aplicaciones (`user_app_registration`, `oauth_metadata_document_clients`, `dynamic_client_registration`) |
  | Contenido (`/admin/settings/content`) | Idioma (`default_locale`, elegido entre los idiomas que tiene la aplicación); Líneas de tiempo (`local_timeline_visibility`, `federated_timeline_visibility`); Publicaciones (`require_media_description`); Página de inicio (`public_counts`); Cuentas sugeridas (`featured_accounts`) |
  | Eliminación de publicaciones (`/admin/settings/auto-delete`) | Valor predeterminado para las cuentas nuevas (las configuraciones `auto_delete_default_*`); Máximo (`auto_delete_max_age_days`, las configuraciones `auto_delete_max_allow_*`, `auto_delete_max_kept_pins`, `auto_delete_grace_days`). Guardar un máximo que se activa o se endurece (menos días, menos excepciones o publicaciones fijadas) pregunta antes, ya que avisa a las cuentas a las que afecta por primera vez ([Un máximo para conservar publicaciones](#a-maximum-for-keeping-posts)) |
  | Federación (`/admin/settings/federation`) | Federación (`federation_mode`, `authorized_fetch`, `remote_media_policy`, `remote_media_cache_ttl_seconds` (elige entre 1 hora, 1 día, 7 días, 30 días o 365 días), `link_cards`, `link_cards_ignore_robots_txt`); Servidores limitados y suspendidos (`domain_actions_visibility`, `domain_actions_rationale_visibility`); Revisión externa (las configuraciones `hook_*`, consulta [Hooks de decisión](decision-hooks.md)) |
  | Búsqueda y rastreadores (`/admin/settings/discovery`) | Buscadores e IA (`search_indexing`, `indexable_default`, `ai_usage`, `security_contact`); Búsqueda de publicaciones (`search_visibilities`, solo mientras el servidor tiene búsqueda de texto completo) |
  | Chat de voz (`/admin/settings/voice`) | `voice_room_starters`, `voice_public_summary`, `voice_participants_retention_days`; la página aparece en la lista solo mientras el servidor ofrece chat de voz |

  En la página General, una imagen se sube cuando la eliges y pasa a ser la miniatura cuando guardas. Quién puede ver los
  costos de funcionamiento, los ingresos y la lista de mecenas se define en las pantallas «Finanzas» y «Mecenas».

  `max_status_length`, `preview_image`, `appeal_window_days` y los periodos de retención (`*_retention_days`), `remote_media_cache_max_bytes` y `oauth_client_rate_limits` no aparecen en la pantalla;
  cámbialos con la CLI o la API de cliente. Las configuraciones de traducción tienen una pantalla propia, más abajo.
- **Cliente web** («Traducción», `/admin/translation`): los proveedores que tiene este servidor, su orden y lo que cada uno puede
  recibir, la traducción del navegador, los límites mensuales y cuánto tiempo se conservan las traducciones, con los caracteres enviados
  este mes y la tasa de aciertos de la caché. Cada proceso del servidor suma su recuento de aciertos de caché a la tasa como máximo una vez por minuto,
  así que las solicitudes más recientes pueden tardar un minuto en aparecer, y un proceso que se detiene puede dejar sin contar su último minuto. Actualiza tu política de privacidad ([plantilla](privacy-policy-template.md#the-services-you-choose))
  antes de activar un proveedor del servidor: el texto enviado a un proveedor no puede retirarse. Las claves de API de los proveedores y la dirección de un
  [plugin traductor](plugins.md#translating-posts) están en la [configuración del despliegue](../reference/configuration.md);
  ninguna configuración ni ninguna respuesta de la API de cliente las contiene.
- **Límites de traducción.** Lo que cuenta son los caracteres enviados, en puntos de código: los marcadores que sustituyen a menciones,
  enlaces y similares no cuentan, ni tampoco una traducción que el servidor ya conserva, una publicación que ya está en el idioma
  del lector o la traducción del navegador. Cada llamada a un proveedor cuenta, también la que falla, ya que el proveedor puede haber
  recibido el texto, así que el número puede diferir de lo que factura un proveedor. Al superar un límite, la solicitud se rechaza con un `429`
  y no se llama a ningún proveedor; el cliente web usa entonces la traducción del navegador del lector, cuando lo permites y el navegador
  puede traducir la publicación. Los meses son en UTC.
- **API de cliente**: `GET /api/v1/admin/settings` devuelve cada configuración con su valor actual, su valor predeterminado, si está definida
  y su tipo y los valores aceptados (opciones o rango). `PATCH /api/v1/admin/settings` cambia configuraciones en conjunto con
  `{"values": {"<key>": "<value>", ...}}`. Todos los valores son cadenas, normalizadas y guardadas como con la CLI. Si una sola clave es
  desconocida o un valor no es válido, la respuesta es un 422 que nombra la clave y no cambia nada. Las claves cambiadas se registran como
  revisiones y en el registro de auditoría en la misma transacción.
- Las invitaciones, que funcionan en los modos `open`, `approval` e `invite`, se crean en la pantalla de moderación («Invitaciones»,
  `/moderation/invites`, permiso `manage_invites`; consulta [códigos de invitación](registration.md#invite-codes)), y los miembros las crean
  según lo permita `invite_creators` ([invitaciones de miembros](registration.md#member-invites)).

<!-- source: rules b1076bb81e0c -->
## Reglas

Las reglas de la instancia aparecen en `rules` de `GET /api/v1/instance`, y las denuncias las citan por ID.

- Una regla tiene una frase de una línea (hasta 300 caracteres) y un complemento opcional (hasta 1000 caracteres). Como con los textos, el
  administrador las escribe en el idioma que quiera y todos los idiomas ven el mismo valor (`dir="auto"` y `lang=""`).
  Puede haber como máximo 100 reglas activas.
- Cada cambio (una frase revisada, una reordenación) se conserva como una revisión (`instance_rule_revisions`), y el ID de una regla no
  cambia.
- Una regla retirada desaparece de la lista de reglas activas. No puede recuperarse, pero las denuncias pueden seguir refiriéndose a ella.

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- La API de administración es `/api/v1/admin/rules` (listar, añadir, obtener, revisar, mover, retirar) y requiere el permiso de rol `manage_rules`.
- En la pantalla de administración del cliente web («Reglas», `/admin/rules`, `manage_rules`), la lista muestra las reglas en vigor en
  orden, y «Subir» y «Bajar» las reordenan; el filtro sobre la lista muestra en su lugar las reglas retiradas. «Añadir una
  regla» abre una página con una frase y un complemento. Elige la frase de una regla para abrir su página, donde la editas y
  la retiras (tras un cuadro de diálogo de confirmación). Cada cambio se registra en el registro de auditoría (`rule_added`, `rule_revised`,
  `rule_moved`, `rule_retired`).
- Los códigos de salida son los mismos que para las configuraciones (0 si todo va bien, 1 por un valor rechazado o un error de la base de datos, 2 por argumentos incorrectos).

<!-- source: link-card-domains c8f674387fa8 -->
## Dominios de tarjetas de enlace

Las tarjetas de enlace de las publicaciones (`link_cards`) provienen de un rastreador: el servidor obtiene la página enlazada y su imagen. Una regla
detiene ese rastreador en un dominio y sus subdominios, y deja intacto todo lo demás del dominio. Tiene uno de dos
modos. Una regla es una decisión del operador, y se aplica además de lo que los propios sitios enlazados rechazan ([vistas previas de enlaces](link-previews.md#cards-of-links-in-posts)).

| Modo | Qué obtiene | Tarjetas que ya tiene |
| --- | --- | --- |
| Detener la obtención (`no_fetch`) | Nada del dominio, ni páginas ni imágenes, tampoco cuando una redirección lleva allí | Ocultas |
| Detener solo las imágenes (`no_images`) | Páginas, pero ninguna imagen del dominio | Se muestran sin su imagen |

- Una regla afecta solo a las tarjetas de enlace. El dominio sigue federado y los medios de sus publicaciones se obtienen como siempre: para aislar un servidor, usa
  una [acción sobre dominios](moderation.md), que también detiene sus tarjetas de enlace. Una regla no figura en la lista de acciones sobre dominios ni se exporta con ellas.
- Una regla cubre el dominio y sus subdominios, y una imagen se comprueba contra los hosts de la página y de la imagen. Se aplica toda regla que cubra un host, así que una regla sobre un subdominio no
  relaja la regla de su dominio superior. Un dominio se escribe y se normaliza como en una acción sobre dominios: un nombre de host sin esquema ni puerto, en cualquier grafía de un dominio internacionalizado.
- Un cambio surte efecto de inmediato, también sobre las tarjetas que el servidor ya tiene, y las imágenes en proxy de esas tarjetas dejan de servirse mientras las imágenes están detenidas.
- Quitar una regla devuelve las tarjetas que el servidor ya tiene a como estaban. Un enlace que se omitió obtiene su tarjeta la próxima vez que una publicación la necesite; no se vuelve a obtener nada en bloque.
  Una tarjeta obtenida de nuevo mientras las imágenes estaban detenidas queda sin imagen hasta su siguiente nueva obtención.
- En la pantalla de administración del cliente web («Dominios de tarjetas de enlace», `/admin/link-card-domains`, `manage_settings`), «Añadir una regla» define una regla con una nota. La lista muestra 40 reglas por página y puede buscarse por dominio; al elegir un dominio se abre la página de su regla, donde cambias su modo y su nota, y la quitas (tras un cuadro de diálogo de confirmación).
- La API de administración es `/api/v1/admin/link_card_domains` (listar, definir; un `DELETE` con el dominio lo quita) y requiere el permiso de rol `manage_settings`. Un servidor conserva como máximo 1000 reglas, cada una con una nota de hasta 5000 caracteres.
- Cada cambio se registra en el registro de auditoría (`link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed`).

<!-- source: theme 380636a0e583 -->
## Tema

El tema de todo el servidor es un conjunto de sustituciones de los tokens de diseño del cliente web más un fragmento opcional de CSS (CSS personalizado). Se
aplica al cliente web de todos los espectadores y a las pantallas que genera el servidor (inicio de sesión, registro, términos, etcétera), tanto en
modo claro como oscuro.

- Puedes sustituir todos los tokens de `app/ui/src/theme.css` salvo los colores primitivos (`--palette-*`). La lista, los valores predeterminados y los
  rangos están en [app/ui/README.md](../../app/ui/README.md#server-themes). Un color es `#rrggbb` para el modo claro y para el oscuro
  (`color-overlay` también puede ser `#rrggbbaa`); los demás tokens toman un valor de la forma de su tipo (longitud, número,
  duración, curva de aceleración, lista de nombres de fuentes, sombra) dentro de su rango. Cuando sustituyas `font-sans` o `font-mono`, mantén
  `"my Noto Color Emoji"` en primer lugar; de lo contrario, los emojis del cliente web se dibujan con la fuente del sistema operativo.
- Al guardar, el servidor comprueba la forma y el rango de los valores, el contraste de cada par de tokens de color (los tokens que no
  sustituyes usan sus valores predeterminados; WCAG 2.2 AA tanto en modo claro como oscuro) y la longitud del CSS personalizado (64 KiB). Si alguna comprobación
  falla, no se guarda nada.
- El servidor no lee el CSS personalizado. Lo coloca en la misma hoja de estilos después de las reglas de los tokens, fuera de cualquier `@layer`. Se ejecuta
  bajo la misma CSP que el cliente web, así que no pueden cargarse imágenes, fuentes ni `@import` de otro origen.
  **El servidor no comprueba los efectos del CSS personalizado (contraste, desplazamiento horizontal, tamaño de las áreas táctiles, indicación del foco,
  idiomas de derecha a izquierda); eso es responsabilidad del administrador.** Los nombres de clase y la estructura de la página cambian entre
  versiones, así que solo los tokens son estables en los que apoyarse.
- Un tema se conserva como revisiones (`instance_theme_revisions`), y la hoja de estilos de la revisión actual se sirve en
  `/theme/{revision ID}.css` (`immutable`, en caché durante mucho tiempo). El shell HTML tiene una caché compartida (60 segundos más 300
  segundos de revalidación), así que tarda unos minutos después de guardar en que todos lo reciban. Una página que ya está abierta conserva
  el aspecto anterior hasta que se recarga.
- `<meta name="theme-color">` (claro y oscuro) en el shell y `theme_color` / `background_color` (claro) del Web App
  Manifest toman el valor de `color-surface` del tema.
- Guardar una revisión sin tokens ni CSS personalizado restablece el tema al valor predeterminado, y la hoja de estilos deja de cargarse.
- Puedes cambiarlo en la pantalla de administración del cliente web («Tema», `/admin/theme`), con la API de cliente (`GET` / `PUT
  /api/v1/admin/theme`) y con la CLI. La pantalla de administración y la API de cliente requieren el permiso de rol `manage_settings`, y
  los cambios se registran en el registro de auditoría (`setting_changed`, destino `theme`).

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## Logotipo e icono

Además de la miniatura y la imagen de vista previa, la instancia puede tener su propio icono, logotipo y logotipo oscuro, cada uno subido como un SVG.
Hasta que se define uno, se usan las imágenes integradas de mycroblog, y borrar uno devuelve la imagen integrada.

| Imagen | Forma | Dónde aparece |
| --- | --- | --- |
| `icon` | Cuadrada (con un margen del 1 %) | El favicon, el icono de pantalla de inicio de iOS (`apple-touch-icon`), los iconos de la aplicación instalada (Web App Manifest), la tarjeta del servidor en la página de inicio y junto a su nombre en el panel lateral del cliente web, y dondequiera que un cliente muestre el servidor como una imagen cuadrada |
| `logo` | De 1 a 8 veces más ancho que alto | La cabecera del cliente web y la parte superior de las páginas de inicio de sesión, registro y consentimiento en lugar del logotipo de texto integrado y el nombre del servidor (el logotipo debe contener el nombre, que es su texto alternativo), la versión HTML de todos los correos electrónicos y la imagen de vista previa de enlaces cuando `preview_image` no está definido |
| `logo_dark` | Como `logo` | Fondos oscuros; sin él, también se usa `logo` allí |

- **El servidor sanea el SVG.** Lo lee como XML y escribe un documento nuevo solo con los elementos permitidos (formas, grupos,
  `defs`, `symbol`, `use`, degradados, `clipPath`, `mask`, `pattern`, `style`, `title`, `desc`) y los atributos permitidos (geometría,
  transformaciones, atributos de presentación, `id`, `class`, `style`). Se eliminan los scripts, `foreignObject`, las imágenes incrustadas, los enlaces,
  las animaciones, los filtros (sombras y desenfoques), los controladores de eventos, las referencias a cualquier cosa fuera del archivo y las URL `data:`,
  y la respuesta lista lo que se eliminó. Solo se guarda el SVG saneado.
- Un SVG se rechaza (no se guarda nada) cuando no es XML bien formado en UTF-8, tiene un `DOCTYPE` o declaraciones de entidades, no tiene
  `viewBox`, supera 256 KiB (128 KiB tras sanear), tiene más de 5000 elementos, se anida a más de 32 niveles, tiene más de 200 elementos
  `use`, tiene una forma incorrecta para su tipo, contiene `text` (convierte antes el texto en contornos en tu editor) o tiene CSS que
  podría cargar algo (`@import`, `@font-face`, `url()` distinto de `url(#id)`, secuencias de escape). `@media (prefers-color-scheme: dark)`
  dentro de un `<style>` está permitido, de modo que un icono puede cambiar en modo oscuro.
- Algunos lugares no admiten SVG (la pantalla de inicio de iOS, los favicons de versiones antiguas de Safari, las vistas previas de enlaces, el correo electrónico). Para ellos, el cliente que sube el
  SVG dibuja también versiones PNG a partir del SVG saneado y las envía con él: 32, 48, 180, 192 y 512 px y un icono maskable de 512 px
  para `icon`; un logotipo de correo de 96 px de alto y una imagen de vista previa de enlace de 1200 × 630 para `logo`; un logotipo de correo de 96 px de alto para
  `logo_dark`. El servidor comprueba que cada una sea un PNG del tamaño correcto (como máximo 1 MiB; el icono de 180 px, el icono maskable y la
  vista previa de enlace deben ser opacos) y lo vuelve a escribir sin metadatos. Si algún archivo es incorrecto, no se guarda nada. Las versiones opacas
  del icono se dibujan sobre un color de fondo, `#rrggbb`, que por defecto es el `color-surface` claro del tema. El servidor
  no comprueba que las versiones PNG muestren la misma imagen que el SVG.
- Los archivos se guardan en el almacén de blobs bajo `instance/brand/<revision>/` y se sirven en
  `https://<host>/brand/<revision>/<version>.svg|.png` con `Cache-Control: public, max-age=31536000, immutable`, ya que el
  contenido de una revisión nunca cambia. Un SVG se sirve con `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` y `X-Content-Type-Options: nosniff`, así que incluso abierto por sí solo no ejecuta ni carga nada.
  Los archivos de revisiones anteriores permanecen, así que los correos electrónicos ya enviados conservan su logotipo.
- Con un icono definido, el shell HTML enlaza sus favicons PNG, su SVG y su `apple-touch-icon` de 180 px, el Web App Manifest lista
  sus iconos de 512 px, 192 px, maskable y SVG en ese orden (otros servidores del fediverso toman el primero como icono de este servidor),
  y `/apple-touch-icon.png` devuelve la versión de 180 px. El ETag del shell cambia con las imágenes, y la caché compartida del shell
  dura unos minutos como con el tema. Una aplicación instalada recoge un icono nuevo en Android cuando Chrome comprueba de nuevo el manifiesto;
  en los equipos de escritorio la aplicación conserva su icono anterior hasta que se vuelve a instalar.
- `GET /api/v1/instance` devuelve `icon`, `logo` y `logo_dark` con la URL del SVG, la URL y el tamaño de cada versión PNG
  y la revisión; cada uno se omite mientras se usa la imagen integrada.
- Defínelos en el cliente web en **Configuración del servidor** > **Icono y logotipo** (`/admin/settings/branding`, permiso de rol `manage_settings`). Elige un archivo SVG
  para el icono, el logotipo o el logotipo para fondos oscuros: el servidor lo comprueba, el navegador dibuja las versiones PNG y el
  formulario las previsualiza (una pestaña del navegador, la pantalla de inicio, el icono de la aplicación, un correo electrónico sobre fondo claro y oscuro, y la vista previa
  de enlace) con la lista de lo que el servidor eliminó. Si el servidor rechaza el archivo, el motivo aparece bajo el botón con cómo
  corregirlo. Para el icono, define también allí el color de fondo de sus versiones opacas. **Guardar** envía el SVG con todas las versiones
  PNG; **Usar el icono integrado**, **Usar el logotipo integrado** y **Quitar el logotipo para fondos oscuros** los borran. El
  cliente web conserva las imágenes que ha mostrado para usarlas sin conexión.
- La API de cliente hace lo mismo, con el permiso de rol `manage_settings`:
  `POST /api/v1/admin/instance/brand/sanitize` comprueba un SVG y devuelve el SVG saneado con las versiones PNG que hay que dibujar (no
  guarda nada), `PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` guarda el SVG con todas las versiones PNG
  (`multipart/form-data`, una parte por versión), `DELETE` en la misma ruta borra uno, y `GET /api/v1/admin/instance/brand` devuelve las imágenes
  actuales con el color de fondo del icono. Cada cambio se conserva como una revisión (`instance_brand_revisions`) y se registra en el
  registro de auditoría (`setting_changed`, destino `brand_icon`, `brand_logo` o `brand_logo_dark`).
- La CLI no puede definirlos, ya que las versiones PNG las dibuja el cliente, pero puede mostrarlos y borrar uno defectuoso:

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## Emojis personalizados

Los emojis personalizados locales se muestran como imágenes cuando alguien escribe `:shortcode:` en el cuerpo o la advertencia de contenido de una publicación, o en el
nombre para mostrar, la biografía o los campos del perfil. También llegan a Mastodon, Misskey, GoToSocial y Akkoma mediante la etiqueta `Emoji` de las
Notes y los actores que envías.

- Un shortcode tiene de 2 a 30 letras, dígitos o `_`. Distingue mayúsculas de minúsculas y no puede cambiarse una vez registrado el emoji.
  Los alias (hasta 10) y una categoría (hasta 64 caracteres) se usan para buscar y agrupar en el selector de emojis.
- La imagen es PNG, GIF o WebP, de como máximo `custom_emoji_max_bytes` (256 KiB por defecto) y de como máximo 1024 px por lado. El
  servidor crea derivados que caben en recuadros de 64, 128 y 256 px y no conserva el original. Un APNG se convierte en una imagen fija
  de su primer fotograma. GoToSocial por defecto no obtiene emojis de más de 100 KiB, así que se recomiendan imágenes pequeñas.
- Un emoji desactivado no aparece en ningún sitio (publicaciones, perfiles, federación, selector), y `:shortcode:` vuelve a mostrarse como texto. Un emoji
  configurado para ocultarse del selector solo falta en el selector y aún puede usarse escribiéndolo. Un emoji que vino de
  otro servidor solo puede desactivarse o copiarse como emoji de este servidor.
- Copiar obtiene la imagen del emoji de otro servidor con los mismos límites que el proxy de medios (no se obtienen imágenes de dominios con
  `reject_media`), ejecuta las mismas comprobaciones anteriores y lo registra como un emoji local nuevo. El shortcode es el
  del original o el que indiques. La imagen se reduce al recuadro de 256 px, y una imagen que no puede obtenerse o no pasa las comprobaciones
  no deja nada.
- La eliminación no se puede deshacer. Los emojis de publicaciones pasadas también vuelven a ser texto, y registrar de nuevo el mismo shortcode crea un
  emoji distinto. Las imágenes que copiaron los servidores federados no pueden eliminarse.
- La pantalla de administración («Emoji personalizados», `/admin/custom-emojis`), la API de cliente (`/api/v1/admin/custom_emojis`) y la CLI hacen
  lo mismo. En la pantalla, «Registrar un emoji personalizado» abre el formulario de registro (`/admin/custom-emojis/new`), y al elegir
  un emoji de la lista se abre su página (`/admin/custom-emojis/{id}`), donde cambias sus configuraciones, sustituyes su imagen, lo activas o
  desactivas, lo eliminas o copias uno de otro servidor; la propia lista activa y desactiva un emoji con una sola pulsación. La pantalla de administración y la API de cliente requieren el permiso de rol `manage_custom_emojis` (solo lo tiene el
  `admin` integrado). Registrar, copiar, sustituir la imagen, cambiar configuraciones y alias, activar y desactivar, y eliminar se
  registran en el registro de auditoría (destino `custom_emoji`) se hagan como se hagan (una operación desde la CLI se registra como una operación
  de la CLI).

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- Un usuario con el permiso de rol `create_custom_emojis` (una función de usuario) puede solicitar la creación de un emoji desde «Emojis
  personalizados» en la configuración (`/settings/custom-emojis`), cambiar los alias, la categoría y la visibilidad en el selector de los emojis que
  creó, y eliminarlos. Sustituir una imagen también es una solicitud. Una solicitud no aparece en ningún sitio (selector, publicaciones, reacciones,
  federación) hasta que alguien con `manage_custom_emojis` la aprueba. Apruébala o recházala en la página «Solicitudes» de «Emojis
  personalizados» en la pantalla de administración (`/admin/custom-emojis/requests`), con la API de cliente (`/api/v1/admin/custom_emoji_requests`) o con la CLI anterior. Las personas con
  `manage_custom_emojis` reciben una notificación de cada solicitud (`admin_custom_emoji_request`), y quien la solicita recibe una notificación de la
  aprobación o el rechazo. Solicitar, retirar, aprobar y rechazar se registran en el registro de auditoría (destino
  `custom_emoji_request`).
- Una solicitud de creación pendiente reserva su shortcode. No es posible una solicitud o un registro con el mismo shortcode hasta que
  la pendiente se rechace o se retire. La imagen de un emoji aprobado se copia como una subida del actor de la instancia, así que
  permanece aunque se elimine la cuenta de quien lo solicitó.
- Un creador puede cambiar sus emojis solo mientras tenga `create_custom_emojis`. Los emojis permanecen si se quita el rol.
  Los administradores pueden cambiar y eliminar todos los emojis sin importar quién los creó.
- Cuando el rol procede de un plan de mecenas, el número de emojis que un usuario puede crear cada mes está limitado por el límite
  mensual de emojis del plan. Cuenta las solicitudes de creación que el usuario envió en el mes natural actual (UTC) y que están
  aprobadas o pendientes; las solicitudes rechazadas y retiradas no cuentan, y eliminar un emoji no libera un hueco. Una solicitud
  cuenta en el mes en que se envió, aunque se apruebe en el siguiente. El recuento se reinicia a las 00:00 UTC del primer
  día de cada mes, y la página de configuración muestra al usuario cuándo, en su propia zona horaria. Sustituir la imagen de un emoji y los
  emojis que registran los administradores no cuentan. Un rol que un administrador asignó a mano, y un servidor donde los mecenas
  están desactivados, no tienen límite.
