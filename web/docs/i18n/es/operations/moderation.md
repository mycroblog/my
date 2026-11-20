<!-- source: account-and-domain-actions-and-handling-reports 7f21d5c6c925 -->
# Acciones sobre cuentas y dominios, y gestión de denuncias

Cómo actúan los operadores y moderadores sobre cuentas y dominios, deciden apelaciones, gestionan denuncias, comprueban qué llegó a un servidor remoto y leen el registro de auditoría.
Todo lo de aquí, salvo la gestión de cuentas y la comprobación de entregas (solo mediante la API de administración), se puede hacer mediante la API de administración (`/api/v1/admin/...`, ámbitos `admin:read` / `admin:write`) y la CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`).
La API de administración exige permisos de rol: la gestión de cuentas, las acciones y las apelaciones necesitan `manage_accounts`, las acciones de dominio necesitan `manage_domains` y las denuncias necesitan `manage_reports`.
La CLI se conecta directamente a la base de datos con la misma configuración que el servidor (`-config` o variables de entorno) y no registra qué usuario la manejó (`issued_by_user_id` y campos similares son NULL, y el actor en el [registro de auditoría](#audit-log) es `cli`).

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## De una denuncia a una acción y una apelación

![El recorrido de una denuncia: llega, un moderador la revisa, puede seguir una acción con un aviso al usuario, y el usuario puede apelar; la columna derecha muestra qué envía cada paso a otros servidores. La lista siguiente describe cada paso.](images/moderation-lifecycle.svg)

De arriba abajo en el diagrama:

1. **Llega una denuncia.** La presenta un usuario local, otro servidor envía un `Flag` (descartado cuando su dominio tiene
   `reject_reports`, consulta [acciones de dominio](#domain-actions)), o un [gancho de decisión](decision-hooks.md) responde `flag`
   ([denuncias que presentó una revisión externa](#reports-that-an-external-review-filed)). Un usuario local que denuncia una
   cuenta de otro servidor puede elegir reenviar la denuncia, lo que envía de inmediato un `Flag` a ese servidor.
2. **Revisión.** La denuncia está `open`, y los moderadores reciben una notificación `admin_report`. Al resolverla pasa a
   `resolved` con el resultado `actioned` o `dismissed`, y un denunciante local recibe una notificación `report_resolved`.
   Reabrirla la devuelve a `open`. Reenviar una denuncia sobre una cuenta de otro servidor envía un `Flag` a ese
   servidor ([denuncias](#reports)).
3. **Acción.** Un moderador emite una [acción sobre la cuenta](#account-actions) desde la denuncia, lo que resuelve la denuncia como
   `actioned`; también se puede emitir una acción sin denuncia. La acción tiene un tipo, un motivo y una hora de fin, y
   termina cuando se levanta o caduca. Con `notify_target`, el usuario recibe una notificación `moderation_notice`
   ([lo que ve el usuario](#what-the-user-sees)). Los otros servidores reciben un `Delete` de las publicaciones en el caso de `delete_statuses`, un
   `Update` del actor cuando empieza o termina un `suspend` o `ban` de una cuenta local, y un `Reject` / `Undo` de los
   seguimientos que corta el `suspend` de una cuenta remota.
4. **Apelación.** El usuario local apela la acción. La apelación está `pending`, y los moderadores reciben una notificación
   `admin_appeal`. La decisión es `upheld` o `overturned`, y el usuario recibe una notificación `appeal_decided`.
   Revocar la decisión levanta la acción y cancela la eliminación de un `ban`, de modo que un `suspend` o `ban` revocado de una cuenta
   local envía un `Update` del actor ([apelaciones](#appeals)).

Las asignaciones, las notas, los avisos y las apelaciones se quedan en este servidor; solo las actividades de la columna derecha salen de él.

<!-- source: moderation-and-server-administration-in-the-web-client 035f2c60c4a9 -->
## Moderación y administración del servidor en el cliente web

El cliente web divide las pantallas del personal en dos.
La navegación muestra «Moderación» y «Administración» solo a las personas cuyo rol tiene al menos un permiso de ese grupo. Cada pantalla muestra solo un motivo a las personas sin el permiso, y el servidor también rechaza sus solicitudes.

| Grupo | Permisos | Pantallas |
| --- | --- | --- |
| Moderación (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Denuncias (`/moderation/reports`), Apelaciones (`/moderation/appeals`), Registros (`/moderation/registrations`), Revisiones automáticas (`/moderation/hook-decisions`), Cuentas (`/moderation/accounts`), Bloqueos de registro (`/moderation/signup-blocks`), Invitaciones (`/moderation/invites`), Acciones de dominio (`/moderation/domain-actions`), Servidores remotos (`/moderation/instances`), Registro de auditoría (`/moderation/audit-log`) |
| Administración del servidor (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Ajustes del servidor (`/admin/settings`), Reglas (`/admin/rules`), Términos y políticas (`/admin/documents`), Dominios de tarjetas de enlace (`/admin/link-card-domains`), la cola de jobs y los jobs fallidos (`/admin/jobs`), Errores (`/admin/errors`, `manage_errors`), Mediciones de UX (`/admin/ux-measurements`, `manage_errors`), Emoji personalizados (`/admin/custom-emojis`), Anuncios (`/admin/announcements`), Webhooks (`/admin/webhooks`), Costes e ingresos (`/admin/finance`, `manage_finance`), Colaboradores (`/admin/supporters`, `manage_supporters`). `manage_login_recovery` (recuperación de inicio de sesión: quitar la autenticación en dos pasos y emitir un enlace de restablecimiento de contraseña) y `manage_roles` (conceder y revocar los roles de una cuenta) aparecen como secciones de la página Gestión de una cuenta (`/moderation/accounts/{id}/manage`, que también necesita `manage_accounts`) |

- La insignia de «Moderación» es el total de denuncias abiertas, registros pendientes de aprobación y apelaciones pendientes de revisión ([notificaciones para moderadores y recuentos pendientes](#notifications-for-moderators-and-the-pending-counts)). «Administración» no muestra recuento.
- Cada grupo se abre en una lista de sus pantallas que los permisos de tu rol abren, cada una con una línea de lo que contiene y, para una cola, cuántas cosas esperan allí (Denuncias cuenta las denuncias abiertas, Apelaciones las que esperan revisión, Registros las que esperan aprobación, Emoji personalizados las solicitudes que esperan revisión, Errores los errores abiertos); un grupo ninguna de cuyas pantallas está abierta para ti dice por qué.
  La dirección de una pantalla que tu rol no abre lo dice, con un enlace a la lista del grupo.
- El `moderator` integrado tiene solo `manage_reports`, `manage_accounts` y `view_audit`, y no ve «Administración». Tampoco ve las pantallas Acciones de dominio y Servidores remotos (`manage_domains`),
  así que para delegarlas, crea un rol con ese permiso y concédelo ([conceder y revocar roles](admin-cli.md#granting-and-revoking-roles)).
- Las rutas de la API de administración siguen siendo `/api/v1/admin/...` sea cual sea el grupo, y los scopes son el mismo par `admin:read` / `admin:write`. Los grupos se separan por etiquetas en la referencia de la API (`admin-moderation`, `admin-server`).
  Un servicio ajeno al servidor puede llamar a las operaciones de un permiso con un scope de permiso en su lugar ([dejar que otro servicio use la API de administración](#letting-another-service-use-the-admin-api)).
- Hay un tercer grupo de permisos, las «funciones de usuario» (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`, `create_articles`, `create_events`), que vive fuera de las pantallas del personal. Quien tiene solo estos permisos no ve ni «Moderación» ni «Administración»,
  y para quien tiene `view_finance` la navegación muestra «Finanzas» (`/finance`) después de Configuración.
  Su API está en el espacio de nombres de usuario (`GET /api/v1/finance/cost_totals` y `GET /api/v1/finance/income_totals`, scope `read:finance`) y no necesita ningún scope `admin:*`. Lo que muestra lo definen los `finance_viewer_*` de los [ajustes de la instancia](instance-settings.md).

<!-- source: letting-another-service-use-the-admin-api d767e08e639c -->
### Dejar que otro servicio use la API de administración

Un servicio que ejecutas junto al servidor, uno que reparte invitaciones o registra costes, por ejemplo, puede llamar a la API de administración para un permiso sin un plugin.

1. Registra una cuenta para el servicio y concédele un rol con solo los permisos que el servicio necesita (para invitaciones, un rol con `manage_invites`; [conceder y revocar roles](admin-cli.md#granting-and-revoking-roles)).
2. Con sesión iniciada como esa cuenta, registra una aplicación en **Configuración → Aplicaciones → Tus aplicaciones** cuyos scopes incluyan el scope del permiso, `admin:` y el permiso: `admin:manage_invites`.
3. Emite un token a la cuenta desde la aplicación, con ese scope, y da al servicio el token de acceso y el token de renovación.

- El token llega solo a las operaciones de los permisos que nombran sus scopes, y solo mientras los roles de la cuenta los conservan; el registro de auditoría anota lo que hace como acciones de la cuenta.
- El token de acceso dura una hora. El servicio lo renueva con el token de renovación y conserva el token de renovación nuevo cada vez: uno antiguo usado de nuevo revoca el token.
- `manage_roles`, `manage_oauth_apps`, `manage_login_recovery`, `manage_login_providers` y `manage_service_closure` no tienen scope de permiso, y ninguna aplicación puede pedir uno a un miembro del personal en la pantalla de consentimiento.
- Para detener el servicio, revoca los tokens de la aplicación o elimina la aplicación, quita el rol o desactiva la aplicación en Administración → Aplicaciones OAuth.

<!-- source: account-actions a61dac7c5921 -->
## Acciones sobre cuentas

Una acción se aplica a la proyección de restricciones (`account_restrictions`) en la transacción que la registra, y surte efecto desde la siguiente petición.

| Tipo | Efecto en una cuenta local |
| --- | --- |
| `warn` | Sin restricción (solo informa al usuario) |
| `force_sensitive` | Muestra todos los archivos adjuntos como sensibles |
| `limit` | Trata las publicaciones (incluidas las pasadas) como solo para seguidores y las mantiene fuera de las líneas de tiempo públicas |
| `restrict_posting` | Prohíbe publicar, responder, impulsar y editar |
| `restrict_interaction` | Prohíbe seguir, marcar como favorito, impulsar y responder |
| `disable_login` | Prohíbe el uso de la aplicación y de la API, y revoca las sesiones y los tokens activos (la página de inicio de sesión aún deja al usuario ver la acción) |
| `suspend` | Aplica todas las restricciones anteriores, y oculta el perfil y las publicaciones a todos salvo al usuario |
| `ban` | Aplica las mismas restricciones que `suspend` y elimina la cuenta a los 30 días. No se puede levantar (solo se cancela cuando se revoca una apelación hecha durante esos 30 días) |
| `delete_statuses` | Elimina las publicaciones indicadas. Pasan a ser invisibles incluso para el usuario, y el servidor envía a otros servidores el mismo `Delete` que cuando el usuario las elimina |
| `mark_statuses_sensitive` | Muestra los archivos adjuntos de las publicaciones indicadas como sensibles |

Para una cuenta remota solo puedes emitir `limit`, `suspend` y `force_sensitive` (cualquier otro tipo da `422` en la API de administración y falla en la CLI).
El efecto es local a este servidor. Lo único que se envía al otro servidor son el `Reject` / `Undo` de los seguimientos cortados por `suspend`.

| Tipo | Efecto en una cuenta remota |
| --- | --- |
| `force_sensitive` | Muestra todos los archivos adjuntos como sensibles |
| `limit` | Trata las publicaciones (incluidas las pasadas) como solo para seguidores, las mantiene fuera de las líneas de tiempo públicas y no crea notificaciones para los usuarios que no siguen la cuenta |
| `suspend` | Descarta todo lo recibido de la cuenta y oculta su perfil y sus publicaciones. Corta los seguimientos entre la cuenta y los usuarios locales en ambos sentidos (también las solicitudes de seguimiento), y envía al otro servidor un `Reject` de los seguimientos de la cuenta y un `Undo` de los seguimientos de los usuarios. Levantarla devuelve la visualización y el servidor vuelve a recibir de la cuenta (lo descartado y los seguimientos cortados durante la suspensión no vuelven) |

- `notify_target` (`-no-notify`) no tiene sentido para una cuenta remota (ninguna notificación le llega).
- Un usuario local que perdió seguimientos por un `suspend` recibe una notificación (`relationships_severed`) que indica cuántos seguidores y seguidos se perdieron ([seguimientos cortados](#severed-follows)).
- Emitir y levantar se registran en el [registro de auditoría](#audit-log), igual que las acciones sobre cuentas locales.

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

En la API de administración: `POST /api/v1/admin/accounts/{id}/actions` (el cuerpo tiene `kind`, `reason_category`, `public_reason`, `private_note`, `report_id`, `notify_target`, `expires_in` en segundos y `status_ids`),
`GET /api/v1/admin/accounts/{id}/actions` (filtra con `active`), `GET /api/v1/admin/account_actions/{id}` y `POST .../{id}/lift` (el cuerpo `{"reason": "..."}` es opcional).

- Nombra una cuenta con `@username` (local), `@username@domain` (remota) o el ID de la cuenta en la CLI, y con el ID de la cuenta en la API de administración.
- Varias acciones se acumulan (la unión de sus restricciones). Levantar una quita solo las restricciones de esa acción, y una acción con hora de fin termina sola a esa hora (se muestra como `expired` en las listas). La hora de fin está como máximo a diez años (`expires_in` hasta 315360000); una más lejana se rechaza con `422`.
- Las sesiones y los tokens que revocaron `disable_login`, `suspend` y `ban` no vuelven al levantar la acción. El usuario inicia sesión de nuevo.
- Un `suspend` o `ban` de una cuenta local envía un `Update` del actor con un perfil vaciado a los servidores de los seguidores y a los servidores conocidos para mostrar la suspensión (Mastodon la muestra como «suspendida»).
  Levantarlo y la caducidad envían un `Update` con el perfil original. La caducidad la envía un trabajo que se ejecuta cada 5 minutos, así que puede retrasarse hasta 5 minutos.
- `delete_statuses` y `mark_statuses_sensitive` toman de 1 a 100 publicaciones no eliminadas de la cuenta objetivo mediante `-status` (`status_ids`). No pueden tener hora de fin.
  Levantar `delete_statuses` no devuelve las publicaciones eliminadas. Levantar `mark_statuses_sensitive` restablece la visualización original.
- Un `ban` no puede tener hora de fin y no se puede levantar. Solo se cancela cuando se revoca una apelación que el usuario hizo durante los 30 días anteriores a la eliminación ([apelaciones](#appeals)). En caso de duda, usa un `suspend` con hora de fin.
  Treinta días después de la acción, un trabajo que se ejecuta cada hora elimina la cuenta (la cuenta y las publicaciones desaparecen de la vista, y el servidor envía un `Delete` a otros servidores).
- `report_id` (`-report`) solo puede nombrar una denuncia sobre la cuenta objetivo. Una denuncia abierta se resuelve como `actioned` junto con la acción (la resolución también queda en el registro de auditoría). Una denuncia ya resuelta no cambia.
- `public_reason` es el motivo que se muestra al usuario y `private_note` es una nota solo para moderadores. `notify_target` (verdadero por defecto; falso con `-no-notify` en la CLI) decide si se informa al usuario.

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### Lo que ve el usuario

- Una acción con `notify_target` verdadero llega al usuario como una notificación (`moderation_notice`). Lleva el tipo, la categoría del motivo, `public_reason`, la hora de fin y el estado. El tipo y la categoría también vuelven como nombres visibles en el idioma de la interfaz del usuario.
- El usuario puede consultar las acciones dirigidas a él con el `GET /api/v1/moderation/actions` propio (filtra con `active`), las más nuevas primero. También se listan las acciones con `notify_target` falso y las acciones levantadas o caducadas.
- `private_note`, la denuncia, quién tomó la acción y el motivo de levantarla nunca se muestran al usuario.
- Un usuario puede apelar cada acción una vez ([apelaciones](#appeals)). La lista de acciones y la acción en una notificación incluyen si se puede apelar (`appealable`), el plazo (`appeal_deadline`), y la apelación y su resultado (`appeal`).
- Un usuario bajo `disable_login`, `suspend` o `ban` no puede usar la aplicación ni la API, pero sí puede iniciar sesión en la página de inicio de sesión (`/auth/sign_in`). Allí ve el tipo, el motivo y la hora de fin de la acción vigente
  (para `ban`, el número de días hasta que se elimine la cuenta), y puede apelar desde esa página ([apelaciones](#appeals)). El endpoint de autorización (`/oauth/authorize`) responde con `403` y no emite ningún token.
  Un usuario bajo `ban` puede iniciar sesión hasta que se elimine la cuenta, y la eliminación revoca también esa sesión. Un usuario cuya propia eliminación está programada, y una cuenta que ha sido desactivada, no pueden iniciar sesión ni siquiera en la página de inicio de sesión.

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### Cuentas que sus propietarios desactivaron

Un usuario puede desactivar su propia cuenta desde la configuración de la cuenta, tras volver a introducir su contraseña o un
código de dos factores, o confirmar con una passkey (un usuario que no tiene ninguna de estas cosas vuelve a iniciar sesión en un
proveedor de inicio de sesión vinculado). La cuenta no es una acción de moderación y los moderadores no pueden desactivar a nadie:

- El perfil y las publicaciones se ocultan a todos salvo a los moderadores, y los otros servidores reciben un `Update` del
  actor marcado como `suspended`, por lo que pueden mostrar la cuenta como suspendida. Terminan todas las sesiones y tokens del usuario.
  Donde el servidor envía correo, un correo electrónico indica la fecha de la eliminación; antes de ella no se envía ningún otro correo.
- Nada llega a la cuenta: las menciones y los mensajes directos se descartan, no se crean notificaciones, correos ni Web Push,
  y se rechazan los seguidores nuevos. Los favoritos y los impulsos se cuentan. Los seguimientos existentes se mantienen.
- El usuario inicia sesión en una página del servidor, que muestra la fecha de la eliminación y le permite reactivar o eliminar la
  cuenta. Reactivar lo devuelve todo. La siguiente desactivación tiene que esperar 24 horas.
- Treinta días después de la desactivación, el trabajo horario elimina la cuenta. Una reactivación posterior a esa fecha se rechaza
  aunque el trabajo aún no se haya ejecutado. El nombre de usuario no se libera.

Los moderadores ven el estado `deactivated` en la lista de cuentas (`state=deactivated`), y todavía pueden leer las denuncias sobre
la cuenta y actuar sobre ella. Una acción de moderación no cambia la fecha de la eliminación, salvo un `ban`, que
la reemplaza por la suya: a partir de ese momento el usuario no puede reactivar. Una suspensión no envía nada a otros servidores mientras la
cuenta está desactivada, y sigue en vigor después de que el usuario reactive. Un usuario no puede desactivar una cuenta mientras
la moderación le impida iniciar sesión, mientras su eliminación esté programada, después de que se haya mudado, mientras su registro esté pendiente
de revisión, mientras siga en curso una suscripción de pago (incluida una programada para terminar al final de su período), ni cuando
sea el único administrador que puede iniciar sesión. Un administrador desactivado no cuenta como uno que pueda. Si la fecha de un
administrador desactivado llega mientras ningún otro administrador puede iniciar sesión, el trabajo horario no elimina la
cuenta y registra una advertencia. Ese administrador tampoco puede iniciar sesión, así que crea otro administrador con
`my admin create`; la siguiente ejecución elimina entonces la cuenta.

<!-- source: appeals 3366a552f16e -->
## Apelaciones

Un usuario local puede apelar una acción contra él.
Un moderador revisa la apelación y la mantiene (`uphold`) o la revoca (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

El usuario apela con `POST /api/v1/moderation/actions/{id}/appeal` (scope `write:accounts`, cuerpo `{"text": "..."}`).
Un usuario bajo `disable_login`, `suspend` o `ban` no puede obtener un token de acceso y apela mediante el formulario por acción de la página de inicio de sesión (`POST /auth/appeal`).
Mientras [el inicio de sesión está limitado a los proveedores de inicio de sesión](login-providers.md#limit-sign-in-to-login-providers), ese usuario llega al formulario solo mediante un proveedor vinculado.
La página muestra el formulario y el plazo para una acción que puede apelarse, el contenido y el resultado para una acción ya apelada, y el motivo para una acción cuyo plazo ha pasado o que no puede apelarse. Para una apelación rechazada muestra el motivo.
En la API de administración: `GET /api/v1/admin/appeals` (filtra con `state`), `GET .../{id}`, y `POST .../{id}/uphold` y `POST .../{id}/overturn` (el cuerpo `{"public_reason": "..."}` es opcional). Necesitan el permiso `manage_accounts`.
En el cliente web, Moderación → Apelaciones (`/moderation/appeals`) enumera las apelaciones que esperan revisión, o todas; la página de una apelación (`/moderation/appeals/{id}`) muestra lo que escribió el usuario y la acción, con el motivo que comunicar al usuario y «Mantener» y «Revocar», cada una con confirmación previa.

- Una acción puede apelarse una vez, y el texto tiene de 1 a 2000 caracteres. El plazo es el número de días del ajuste de la instancia `appeal_window_days` después de la acción (180 días por defecto, de 20 a 365; consulta [ajustes de la instancia](instance-settings.md)).
  Una acción fuera de plazo y una segunda apelación dan `422` (`/api/v1/problems/appeal-window-closed` y `/api/v1/problems/already-appealed`).
- Un `ban` puede apelarse hasta que se elimina la cuenta (30 días después de la acción, o hasta el plazo de `appeal_window_days` si es menor). Un `ban` de una cuenta eliminada da `422` (`/api/v1/problems/not-appealable`). Una acción levantada o caducada aún puede apelarse dentro del plazo.
- Cuando un usuario apela, las personas con `manage_accounts` reciben una notificación (`admin_appeal`). Cuando se revisa, el usuario recibe una notificación (`appeal_decided`) y puede ver el resultado y `public_reason`. No se muestra al revisor.
- Revocar levanta la acción en la misma transacción (el mismo efecto que `lift`, con un motivo de levantamiento vacío). Revocar `delete_statuses` no recupera las publicaciones eliminadas. Una acción ya levantada antes de la revisión permanece como está.
- Revocar un `ban` también cancela la eliminación programada de la cuenta en la misma transacción (el registro de auditoría anota `account_deletion_canceled`), termina la suspensión y envía el `Update` del perfil original a otros servidores. Las sesiones y los tokens revocados no vuelven, así que el usuario inicia sesión de nuevo.
  Si la cuenta se eliminó antes de la revisión, no puede revocarse (`422`, `/api/v1/problems/not-appealable`; mantenerla sigue siendo posible).
- Una revisión ocurre una vez. Revisar de nuevo con el mismo resultado no cambia nada, y el resultado contrario da `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management aa223fe467e1 -->
## Gestión de cuentas

Buscar cuentas, ver sus detalles y revelar direcciones IP y direcciones de correo se hace mediante la API de administración y necesita el permiso `manage_accounts`.

En el cliente web, una cuenta tiene tres páginas, con enlaces entre ellas bajo el título de la página:

- **Revisión** (`/moderation/accounts/{id}`): el estado y las restricciones de la cuenta, su registro y quién la invitó, el formulario de una acción nueva (abierto desde una denuncia con «Actuar sobre esta cuenta», el formulario también ofrece las acciones sobre las publicaciones de la denuncia, y la acción resuelve la denuncia), las acciones más recientes sobre ella con sus motivos, con «Todas las acciones» hacia su página Acciones cuando puede haber más, y sus denuncias, las más recientes primero, el resto bajo «Mostrar más».
- **Acciones** (`/moderation/accounts/{id}/actions`): todas las acciones sobre la cuenta, o las vigentes, una página cada vez. Cada acción abre su propia página (`/moderation/accounts/{id}/actions/{action id}`) con todos sus detalles y, mientras está vigente, «Levantar» con un motivo para los moderadores. La página de un veto no tiene «Levantar»: solo revocar su apelación lo deshace.
- **Gestión** (`/moderation/accounts/{id}/manage`): para una cuenta local, sus roles (`manage_roles`), su supresión de correo, quitar la autenticación en dos pasos y emitir un enlace de restablecimiento de contraseña (`manage_login_recovery`), y sus direcciones de correo e IP, mostradas solo tras «Mostrar», con sus inicios de sesión y las cuentas que usaron las mismas direcciones IP, cada lista con la más reciente primero y el resto bajo «Mostrar más» (cada pulsación lee una página, anotada en el registro de auditoría), y un enlace a las del mismo dominio de correo; para una cuenta remota, obtener de nuevo sus publicaciones de su servidor (`manage_domains`). Cada sección aparece solo a las personas con su permiso.

El personal actúa solo sobre rangos inferiores: emitir o levantar una acción, resolver una apelación, levantar una supresión de correo, revelar datos privados, emitir un enlace de restablecimiento de contraseña y quitar la autenticación en dos pasos exigen que la `position` más alta entre tus roles esté por encima de la de cada rol del destinatario.
Un rango superior o igual, incluida tu propia cuenta, devuelve `403` con el tipo de problema `/api/v1/problems/target-outranks-actor`, así que un moderador no puede vetar a un administrador y un administrador no puede vetar a otro. Los comandos `my admin` no tienen tal límite.
Una acción que impide iniciar sesión al único administrador que puede hacerlo (`disable_login`, `suspend`, `ban`) se rechaza quienquiera que la emita, y ese administrador no puede eliminar su propia cuenta (`409` con `/api/v1/problems/last-administrator`). Consulta [admin-cli.md](admin-cli.md) para los roles.

- `GET /api/v1/admin/accounts` devuelve cuentas locales y remotas, las más recientes primero (se excluye el Actor de la instancia).
  Puedes filtrar por prefijo de nombre de usuario (`username`), dominio (`domain`), local o remota (`origin`), estado (`state`) y, para cuentas locales, dirección de correo (`email`), dominio de correo (`email_domain`) y la dirección IP usada para registrarse o iniciar sesión (`ip`).
  Solo puedes dar uno de `email`, `email_domain` e `ip`. La lista no incluye direcciones IP ni direcciones de correo.
- El estado es el primero que se aplica de esta lista: `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` y `pending` (revisión del registro), `limited` (`limit`), `active`.
  El estado y las restricciones (`restrictions`) proceden solo de acciones contra la propia cuenta y no incluyen las acciones de dominio.
- `GET /api/v1/admin/accounts/{id}` devuelve el estado y las restricciones y, para una cuenta local, la solicitud de registro y su revisión, la confirmación del correo, los roles y hasta 20 acciones y denuncias, las más recientes primero. `GET /api/v1/admin/reports?target_account_id={id}` recorre todas las denuncias sobre una cuenta.
- `POST /api/v1/admin/accounts/{id}/reveal` devuelve la dirección de correo de una cuenta local, las direcciones IP y los User-Agent del registro y de los inicios de sesión (20 entradas, las más recientes primero) y otras cuentas que usaron la misma dirección IP (hasta 20 por dirección IP).
  `GET /api/v1/admin/accounts/{id}/sign_ins` y `GET /api/v1/admin/accounts/{id}/same_ip_accounts?ip=` recorren todos ellos.
  Cada llamada a estas tres anota `revealed_private_data` en el registro de auditoría, y los datos se devuelven solo si se pudo escribir la anotación. Las direcciones IP se eliminan tras el periodo de conservación (180 días por defecto), así que los orígenes de inicio de sesión más antiguos no pueden mostrarse.
- `two_factor_enabled` en `GET /api/v1/admin/accounts/{id}` muestra si el usuario de una cuenta local usa autenticación en dos pasos (TOTP).
  Un usuario que perdió tanto la aplicación de autenticación como los códigos de recuperación puede iniciar sesión solo con la contraseña una vez que quitas la autenticación en dos pasos con `POST /api/v1/admin/accounts/{id}/two_factor/disable`.
  Solo los usuarios con el permiso `manage_login_recovery` pueden hacerlo, y por defecto solo lo tiene el rol integrado de administrador (pertenece a la administración del servidor; el cliente web muestra la sección solo a ellos).
  La eliminación anota `two_factor_disabled` en el registro de auditoría. Verifica por otra vía, como el correo, que la persona es la titular de la cuenta antes de quitarla.
  Cuando un administrador sin otros administradores la perdió, el operador la quita con el comando de la CLI `my admin account disable-two-factor @alice` (el actor del registro de auditoría es la CLI).
- En un servidor que envía correo, un usuario que olvidó la contraseña recibe por correo un enlace para establecer una nueva desde «¿Olvidaste tu contraseña?» en la página de inicio de sesión.
  Para un servidor que no envía correo (`mail = none`), o un usuario al que el correo no llega, un administrador emite el enlace y se lo entrega.
  En las pantallas de administración, emítelo con «Restablecer la contraseña» en la página Gestión de la cuenta (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`).
  Solo los usuarios con el mismo permiso que para quitar la autenticación en dos pasos, `manage_login_recovery`, pueden emitirlo, y por defecto solo lo tiene el rol integrado de administrador (quien tenga el enlace puede entrar en la cuenta, así que el `manage_accounts` de un moderador no basta).
  El enlace emitido se muestra una vez en el momento y puede copiarse. El servidor almacena solo un hash del enlace, así que no puedes verlo después; emítelo de nuevo si lo necesitas.
  Cuando no hay ningún administrador disponible o las pantallas de administración no lo están, el operador lo emite con el comando de la CLI `my admin account password-reset-link @alice` (el actor del registro de auditoría es la CLI).
  Mientras [el inicio de sesión está limitado a los proveedores de inicio de sesión](login-providers.md#limit-sign-in-to-login-providers), ambos siguen emitiendo el enlace y dicen que no funciona hasta que termine la limitación.
  El enlace funciona durante 1 hora y una sola vez, y deja de funcionar si la contraseña cambia después por otra vía. Cualquiera de las dos formas de emitirlo anota `password_reset_link_issued` en el registro de auditoría.
  Quien tenga el enlace puede establecer la contraseña, así que verifica a la persona por otra vía, como el correo, y da el enlace solo a ella.
  Establecer la contraseña nueva revoca todas las sesiones y los tokens de aplicación del usuario. Las passkeys y la autenticación en dos pasos permanecen.
- `email_suppression` en `GET /api/v1/admin/accounts/{id}` muestra que el envío a la dirección principal de una cuenta local está detenido.
  Un `hard_bounce` (el destino no existe) detiene todo el correo salvo el de confirmación, y un `complaint` (denunciado como spam) detiene solo el correo de notificaciones y anuncios. El correo de restablecimiento de contraseña también se detiene, así que emite el enlace de arriba y entrégalo si hace falta.
  Cuando el usuario pide un correo de confirmación y confirma la dirección de nuevo, se levanta la supresión por hard bounce.
  Cuando hayas verificado que el envío es correcto, por ejemplo porque el usuario arregló el buzón, levántala con `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permiso `manage_accounts`). Anota `email_suppression_lifted` en el registro de auditoría.
- Una búsqueda lee hasta 400 cuentas para llenar una página. Cuando coinciden pocas cuentas, incluso una página corta o vacía puede llevar `next_cursor`, así que sigue leyendo.

<!-- source: domain-actions d71386f1cd2d -->
## Acciones de dominio

| Tipo | Efecto |
| --- | --- |
| `suspend` | Detiene toda entrega, obtención y recepción (la recepción recibe `403` antes de verificar la firma). Las cuentas y publicaciones existentes se ocultan de las cronologías y los perfiles. Corta los seguimientos entre usuarios locales y las cuentas del dominio en ambas direcciones (no se envía nada al otro lado) |
| `limit` | Mantiene las publicaciones fuera de las cronologías públicas y de la búsqueda, y no crea notificaciones para los usuarios que no siguen a la cuenta |
| `reject_media` | No obtiene multimedia |
| `reject_reports` | Descarta las denuncias (`Flag`) del dominio |
| `force_sensitive` | Muestra la multimedia como sensible |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

En la API de administración: `POST /api/v1/admin/domain_actions` (el cuerpo tiene `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` y `expires_in` en segundos),
`GET /api/v1/admin/domain_actions` (filtra con `domain` y `active`), `GET .../{id}`, `POST .../{id}/lift`,
y `GET .../impact?domain=`, que cuenta los seguimientos que terminaría un `suspend` del dominio.

En el cliente web, abre Moderación → Acciones de dominio (`/moderation/domain-actions`). La lista muestra el dominio, el motivo público, el tipo, el fin y el estado de cada acción, vigentes o todas, de todos los dominios o del que busques;
«Nueva acción» abre el formulario (`/moderation/domain-actions/new`), y al elegir un dominio se abre la página de la acción con sus motivos y fechas, donde «Levantar» la termina.
Un `suspend` pregunta antes: la confirmación dice cuántos seguimientos entre usuarios locales y las cuentas del dominio termina para siempre, y pide que escribas el dominio.
El enlace «Acciones de dominio» de la página de un servidor remoto enumera solo las acciones de ese dominio, y «Nueva acción» allí inicia el formulario con el dominio rellenado.

- Los dominios se registran normalizados a la etiqueta A de IDNA (minúsculas, sin punto final). `例え.jp` y `xn--r8jz45g.jp` son el mismo dominio.
- Una acción también se aplica a los subdominios (una acción sobre `spam.example` se aplica también a `a.spam.example`).
- Una acción se aplica a la proyección de restricciones (`domain_restrictions`) y a las restricciones de cronología en la transacción que la registra, y surte efecto desde la siguiente entrega, obtención o recepción.
- Una acción con hora de fin (`-duration` / `expires_in`, como máximo diez años) termina por sí sola a esa hora. La caducidad no es un «levantamiento», así que las listas la muestran como `expired`.
- Levantar quita solo las restricciones de esa acción. Las otras acciones sobre el mismo dominio permanecen. Levantar una acción ya levantada no cambia nada.
- Las listas van con la más reciente primero, y `-active` (`active=true`) muestra solo las acciones vigentes.

<!-- source: importing-and-exporting-mastodon-csv f9b0965f8f91 -->
### Importar y exportar CSV de Mastodon

Puedes pasar la lista de acciones de dominio desde y hacia Mastodon en el formato CSV de los bloqueos de dominio de Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`).
Puedes importar un CSV que exportó Mastodon, y Mastodon puede importar un CSV que exportó este servidor.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

En la API de administración: `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (el cuerpo tiene `csv`) y `POST .../import` (el cuerpo tiene `blocks`, las filas elegidas, y `private_note`).
Una fila para el propio dominio de este servidor se omite (`own_domain`), y se rechaza una acción de dominio sobre él.
En el cliente web, abre Moderación → Acciones de dominio y elige «CSV de Mastodon» (`/moderation/domain-actions/import`): «Guardar como CSV» guarda el archivo, y la importación previsualiza un archivo elegido antes de importar las filas que conservas; un `suspend` entre ellas pregunta antes.

| Fila del CSV | Acción en este servidor |
| --- | --- |
| `#severity` es `suspend` (o vacío) | `suspend` |
| `#severity` es `silence` | `limit` |
| `#severity` es `noop` | Ninguna |
| `#reject_media` / `#reject_reports` es verdadero | Añade `reject_media` / `reject_reports` respectivamente |

- Una importación tiene dos pasos. Primero comprueba en la vista previa qué acciones emitiría cada fila, elige las filas que conservar y luego importa (la CLI no cambia nada hasta que añades `-apply`, y `-only` elige filas por dominio).
- Una fila cuyo dominio o un dominio padre ya tiene una acción del mismo tipo en vigor no emite ese tipo (si todos los tipos ya están, es «ya aplicada»). También se omiten una segunda fila para el mismo dominio, una fila no válida y una fila que no emitiría nada (solo `noop`). Importar de nuevo el mismo CSV no añade acciones.
- Un CSV sin cabecera `#domain` se lee como una lista con un dominio por línea, y cada dominio recibe `suspend` (igual que Mastodon). Una importación admite hasta 5000 filas.
- Las acciones importadas duran «hasta que se levanten» y llevan el comentario público y la ofuscación de la fila y la nota privada que diste. Aparecen en el registro de auditoría como las acciones emitidas una a una (el actor es la persona para la API y `cli` para la CLI).
- Una exportación escribe una fila por dominio para las acciones en vigor. Un dominio con un `suspend` recibe `suspend`, uno con un `limit` recibe `silence`, y uno con ninguno recibe `noop`. Un dominio con solo `force_sensitive`, que los bloqueos de dominio de Mastodon no tienen, no se exporta. Las notas privadas y las horas de fin no se exportan.

<!-- source: subscribing-to-blocklists d49a93c3c44f -->
### Suscribirse a listas de bloqueo

En lugar de importar un CSV a mano, puedes suscribirte a una lista de bloqueo de dominios que otro servidor o persona publica en una
URL `https`. El servidor obtiene la lista justo después de suscribirte, todos los días a partir de entonces y cuando lo pides, y convierte
sus filas en acciones de dominio: como borradores que un moderador acepta o rechaza (el modo `draft`, el predeterminado), o directamente en
cada obtención (el modo `automatic`).

```sh
my admin domain subscription add -format mastodon_csv -title "Shared blocklist" https://lists.example/blocklist.csv
my admin domain subscription list
my admin domain subscription drafts <subscription id>
my admin domain subscription accept -keys issue:suspend:spam.example <subscription id>
my admin domain subscription reject -all <subscription id>
my admin domain subscription update -mode automatic <subscription id>
my admin domain subscription fetch <subscription id>               # fetch now and print the outcome
my admin domain subscription fetches <subscription id>
my admin domain subscription remove [-lift-actions] <subscription id>
```

En la API de administración: `GET` y `POST /api/v1/admin/domain_list_subscriptions`, `GET`, `PATCH` y `DELETE .../{id}`
(`lift_actions=true` levanta las acciones de la suscripción), `POST .../{id}/fetch`, `GET .../{id}/fetches`, `GET .../{id}/drafts`,
y `POST .../{id}/drafts/accept` y `.../drafts/reject` (el cuerpo lleva `keys`, o nada para todos los borradores). Todas
necesitan `manage_domains`.

En el cliente web, abre Moderación → Medidas de dominio y elige «Suscripciones a listas de bloqueo»
(`/moderation/domain-actions/subscriptions`). La lista muestra el modo y la última obtención de cada suscripción, con los fallos
consecutivos; «Añadir suscripción» abre el formulario del nombre, la URL, el formato y los ajustes de más abajo, y al suscribirte se abre la
página de la suscripción. Esa página tiene cuatro secciones: los borradores, que aceptas o rechazas uno a uno o todos a la vez (aceptar
borradores que suspenden un dominio pide confirmación antes); las obtenciones, con «Obtener ahora» y el resultado de cada obtención; los ajustes; y «Cancelar
suscripción», que pregunta si se levantan las acciones que emitió la suscripción. La notificación `admin_domain_list_failing`
enlaza con la lista.

| Formato | Qué es la lista |
| --- | --- |
| `mastodon_csv` | El CSV de bloqueos de dominio de Mastodon, leído como lo lee una importación |
| `mastodon_json` | La respuesta de `GET /api/v1/instance/domain_blocks` de un servidor Mastodon que publica sus bloqueos de dominio. Los dominios que ofusca (con `*`) se omiten, y un servidor que no publica la lista (`401`, `403`, `404`) hace fallar la obtención |
| `plain_text` | Un dominio por línea, cada uno un `suspend` |

Lo que emite una fila sigue la misma tabla que una importación, modificada por los ajustes de la suscripción: `suspend_as` (`suspend`,
`limit` o `ignore`; `suspend` por defecto), `silence_as` (`limit` o `ignore`; `limit` por defecto), si se aplican el `reject_media` y el
`reject_reports` de la fila (ambos por defecto) y si las acciones copian el comentario público de la fila (por defecto no).

- Una obtención pasa por la misma protección que las obtenciones de otros servidores: una URL, o una redirección, que lleve a una dirección privada
  falla. Lee como máximo 2 MiB y 5000 filas; una lista mayor hace fallar la obtención. El servidor devuelve el `ETag` y el
  `Last-Modified` de la lista, así que una lista sin cambios cuesta poco, y una lista idéntica a la última leída no cambia nada.
- Una obtención que falla no cambia ninguna acción. Se registra con el motivo del fallo (`http_status`, `network`,
  `forbidden_destination`, `too_large`, `too_many_entries` o `malformed`) y se muestra en la lista de suscripciones y en sus
  obtenciones. Tras 3 fallos consecutivos, las personas cuyo rol tiene `manage_domains` reciben la notificación
  `admin_domain_list_failing`.
- Una suscripción nunca levanta ni sustituye una acción que no emitió. Si una acción del mismo tipo ya cubre el dominio,
  en él o en un dominio padre, la haya emitido quien sea, la suscripción no emite nada allí. Cuando dos suscripciones enumeran el mismo
  dominio, rige la acción de la que la emitió primero; si esa la levanta, la siguiente emite la suya.
- Cuando la lista quita un dominio, el modo automático levanta, en la siguiente obtención, la acción que la suscripción emitió sobre él. Si una
  obtención levantara más de 10 acciones y más de la mitad de las de la suscripción, los levantamientos esperan como borradores, ya que una
  lista rota o vacía tiene ese aspecto.
- Una gravedad que el servidor no conoce (ni `suspend`, ni `silence`, ni `noop`) siempre espera como borrador, como un `suspend`
  bajo los ajustes de la suscripción.
- Los borradores enumeran lo que la lista añadiría (`issue`) y quitaría (`lift`), cada uno con una clave como `issue:suspend:spam.example`.
  Un borrador rechazado no vuelve hasta que la lista cambia la fila del dominio (un `issue`) o termina la acción (un `lift`).
- Si un moderador levanta una acción que emitió una suscripción, la suscripción no la vuelve a emitir hasta que la lista cambia
  la fila del dominio.
- Cambiar los ajustes los aplica en la obtención que inicia el cambio. La URL y el formato no cambian; suscríbete
  de nuevo para cambiarlos.
- Quitar una suscripción conserva las acciones que emitió, que pasan entonces a contar como de un moderador, salvo que las levantes con ella.
- El registro de auditoría anota las suscripciones, los cambios, las eliminaciones, los borradores aceptados y rechazados, y las obtenciones pedidas a mano.
  Las acciones del modo automático se anotan como las demás acciones de dominio con `system` como actor, y se envían a los
  webhooks como `domain_moderation_action.created` y `.lifted`, cuyo `data` lleva el `subscription_id`.

<!-- source: severed-follows 51be435d8635 -->
### Seguimientos cortados

Un `suspend` de dominio y un `suspend` de cuenta remota cortan los seguimientos entre los usuarios locales y las cuentas suspendidas en ambos sentidos.

- En un `suspend` de dominio, un trabajo (`moderation.sever_domain`) que se ejecuta después de registrar la acción los corta de un usuario local en un usuario local. Hasta que el trabajo termina, los seguimientos siguen apareciendo en las listas. También se cortan las cuentas de los subdominios.
- En un `suspend` de cuenta remota, los seguimientos se cortan en la transacción que registra la acción. Un segundo `suspend` de una cuenta ya suspendida no corta nada.
- Cada usuario que perdió seguimientos aprobados recibe una notificación (`relationships_severed`) que indica cuántos seguidores y seguidos se perdieron. Un usuario que solo perdió solicitudes de seguimiento no recibe ninguna.
- Levantar la acción no devuelve los seguimientos cortados. Los usuarios vuelven a seguir.
- Procesar de nuevo la misma acción (por ejemplo cuando un trabajo se vuelve a ejecutar) no añade más registros de relaciones cortadas ni notificaciones.

<!-- source: federating-only-with-allowed-servers 84209876856e -->
### Federar solo con los servidores permitidos

El ajuste de la instancia `federation_scope` decide con qué servidores federa este servidor: `open` (el valor predeterminado) federa con todos los servidores que ninguna acción de dominio suspende,
y `allowlist` solo con los dominios de la lista de permitidos y sus subdominios, para una organización o un servidor personal que habla solo con servidores que conoce.

Mientras `federation_scope` es `allowlist`:

- Un servidor sin permiso se trata como un dominio suspendido: sus entregas reciben `403` antes de comprobar sus firmas, no se le entrega nada ni se obtiene nada de él, y los relays no traen nada de él.
- Un permiso cubre sus subdominios: permitir `friend.example` permite también `social.friend.example`.
- Las acciones de dominio se siguen aplicando a los servidores permitidos: un dominio permitido con un `suspend` sigue suspendido, y uno con un `limit` o `reject_media` sigue limitado.
- Los GET de ActivityPub necesitan una firma de un servidor permitido, diga lo que diga `authorized_fetch`, para que un servidor sin permiso no pueda leer las publicaciones públicas. Los servidores y rastreadores que no firman sus GET reciben `401`.
- NodeInfo declara `metadata.federationScope`, para que otros servidores puedan saberlo.

Mientras es `open`, la lista de permitidos no cambia nada, y se conserva para la próxima vez que cambies.

```sh
my admin domain allow add -comment "partner" friend.example
my admin domain allow list
my admin domain allow remove friend.example
my admin federation scope                   # prints the setting
my admin federation scope allowlist         # shows what the switch would end, and changes nothing
my admin federation scope -apply allowlist  # switches
my admin federation scope open
```

En la API de administración: `GET /api/v1/admin/domain_allows`, `POST /api/v1/admin/domain_allows` (el cuerpo tiene `domain` y `comment`; un dominio ya permitido responde `200` y conserva su permiso) y `DELETE /api/v1/admin/domain_allows/{domain}`, con el permiso `manage_domains`.
`GET /api/v1/admin/settings/federation_scope/impact` cuenta lo que terminaría el cambio, con el permiso `manage_settings`, y `PATCH /api/v1/admin/settings` cambia `federation_scope` como cualquier otro ajuste.
La lista de permitidos contiene como máximo 5000 dominios, y un comentario, que nunca se publica, como máximo 5000 caracteres. El propio dominio de este servidor no puede permitirse.

Cambiar a `allowlist`, y quitar un permiso mientras está activada, termina seguimientos para siempre:

- Antes de cambiar, lee cuántos seguimientos aprobados entre usuarios locales y servidores sin permiso terminaría el cambio, cuántos usuarios locales pierden alguno y los servidores con más de ellos (el impacto de arriba, o `my admin federation scope allowlist` sin `-apply`).
- El cambio corta los seguimientos con todos los servidores que la lista de permitidos no permite, en ambas direcciones y sin avisar a esos servidores, como hace un `suspend` de dominio: un job (`moderation.sever_allowlist`) los corta usuario local por usuario local, y cada usuario que perdió seguimientos aprobados recibe una notificación (`relationships_severed`) con los recuentos.
- Quitar un permiso mientras `federation_scope` es `allowlist` corta los seguimientos con las cuentas de ese dominio y de sus subdominios que ningún otro permiso cubre, de la misma manera. Quitar el permiso de un subdominio cuyo dominio padre está permitido no corta nada.
- Volver a `open`, o permitir de nuevo el dominio, no recupera los seguimientos cortados. Los usuarios vuelven a seguir.
- Definir `allowlist` cuando ya es `allowlist` no corta nada más.

En el cliente web, abre Moderación → Acciones de dominio y elige «Dominios permitidos» (`/moderation/domain-actions/allowed`) para leer,
añadir y quitar permisos; quitar uno pregunta antes, ya que aísla el dominio mientras `federation_scope` es `allowlist`. El
ajuste en sí es «Servidores» en la página Federación de los ajustes del servidor (`/admin/settings/federation`, con el
permiso `manage_settings`): elegir «Solo servidores permitidos» y guardar muestra los seguimientos que terminaría el cambio, las cuentas
locales que pierden alguno y los servidores con más de ellos, y guarda solo cuando confirmas.

Añadir y quitar un permiso están en el registro de auditoría (`domain_allow_added` y `domain_allow_removed`), y el cambio es el `setting_changed` de `federation_scope`.

<!-- source: publishing-the-domains-you-act-on e8402b3a2c93 -->
### Publicar los dominios sobre los que actúas

Puedes publicar las acciones `limit` y `suspend` en vigor mediante `GET /api/v1/instance/domain_actions`.
El ajuste de la instancia `domain_actions_visibility` (`off` / `signed_in` / `everyone`, `off` por defecto) decide quién ve la lista, y
`domain_actions_rationale_visibility` (los mismos valores, `off` por defecto) decide quién ve los comentarios públicos ([ajustes de la instancia](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Con `off` la API devuelve `404`, y con `signed_in` devuelve `401` a una solicitud sin token de acceso.
- Hay una entrada por acción, la más reciente primero. Una entrada tiene el dominio, el SHA-256 del dominio (hex) y el tipo, y el comentario público para las personas autorizadas a verlo. Las notas privadas nunca se devuelven.
- Para una acción con `-obfuscate` (`obfuscate`), la mitad central de los caracteres del dominio (salvo los puntos) se sustituye por `*` (`bad.example` pasa a `bad.*****le`).
  Cualquiera puede comparar el SHA-256 de un dominio que conoce para comprobar si es uno ofuscado.
- Las acciones `reject_media`, `reject_reports` y `force_sensitive`, y las acciones levantadas o caducadas, no se publican.
- La página de información de la instancia del cliente web (`https://<host>/about`) muestra esta lista bajo los mismos ajustes. Con `off` la sección no se muestra, y con `signed_in` las personas que no han iniciado sesión ven una nota que dice que solo los usuarios con sesión iniciada pueden verla.

<!-- source: refetching-remote-posts 883740b40c9a -->
## Volver a obtener publicaciones remotas

Puedes volver a obtener del otro servidor las publicaciones remotas almacenadas.
Una nueva obtención ocurre solo cuando la pides mediante la API de administración, la CLI o el cliente web, nunca periódicamente.

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

En la API de administración: `POST /api/v1/admin/statuses/{id}/refetch`, `POST /api/v1/admin/accounts/{id}/refetch_statuses` y `POST /api/v1/admin/instances/{domain}/refetch_statuses`. Necesitan el permiso `manage_domains`.
En el cliente web, usa «Volver a descargar publicaciones» en la página de Revisión de la cuenta (`/moderation/accounts/{id}`, cuentas remotas) y en la pantalla del servidor remoto (`/moderation/instances/{domain}`).

- Una publicación se trata como un `Update` recibido. Si hay una versión más nueva, el servidor añade una versión; actualiza los recuentos de votos de las encuestas; y no hace nada si nada cambió. Una publicación para la que el otro servidor devuelve 410 Gone o un `Tombstone` se elimina. Una publicación que no se puede obtener (404 incluido) se deja como está.
- Para una cuenta, el servidor vuelve a obtener hasta 1.000 de sus publicaciones almacenadas no eliminadas más nuevas, de 100 en 100, con un minuto de separación.
- Para un dominio, el servidor vuelve a obtener empezando por las cuentas más nuevas del dominio, hasta las 20 publicaciones más nuevas por cuenta y hasta 1.000 publicaciones en total, de 5 cuentas en 5 cuentas, con un minuto de separación.
- No obtiene de un dominio con un `suspend`, ni de un dominio `unavailable` porque fallaron las entregas. Una obtención fallida del mismo URI se recuerda durante 1 hora, y una nueva obtención en ese tiempo no la intenta.
- Tu petición se registra en el registro de auditoría como `status_refetch_queued`, `account_statuses_refetch_queued` o `domain_statuses_refetch_queued`.

<!-- source: checking-what-reached-a-remote-server 6c1ec9c8cef1 -->
## Comprobar qué llegó a un servidor remoto

Cuando un usuario pregunta si una publicación, un seguimiento o una aceptación llegó a otro servidor, enumera las entregas a ese servidor con la API de administración.
Muestra cada entrega de una actividad a un inbox del servidor, la más reciente primero, con cada intento y lo que respondió el servidor, incluidas las entregas que tuvieron éxito.
Necesita el permiso `manage_domains`.
En el cliente web, la sección **Entregas** de la pantalla del servidor remoto (`/moderation/instances/{domain}`) muestra la misma lista, con sus filtros por **ID de actividad** y **Cuenta local**, y **Mostrar** para cada campo oculto en **Detalles** de una entrega enmascarada; la pantalla dice cuáles de ellos se anotan en el registro de auditoría antes de ejecutarse.
La lista de servidores remotos muestra solo los servidores a los que fallaron las entregas. Para abrir cualquier otro servidor, escribe su dominio, un identificador en él o una URL en él en **Abrir un servidor**, encima de la lista,
o usa **Abrir el servidor de la cuenta** en la página Revisión de una cuenta remota o en una denuncia sobre una cuenta remota, o el dominio de una entrada del registro de auditoría sobre un dominio.

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "https://example.com/api/v1/admin/instances/misskey.io/deliveries"
```

- Cada entrega tiene el tipo de actividad (`Create`, `Follow`, `Accept`, etc.), cuándo se registró la actividad y cuándo se añadió el inbox, y sus `attempts`, el más antiguo primero.
  Cada intento tiene su `outcome` (`succeeded`, `retryable_failure`, `permanent_failure`, `skipped`), el `signature_scheme` y el `status_code` de la respuesta, el `error` de un fallo sin respuesta o por qué no se envió nada, y la hora.
  Un intento `succeeded` con `202` significa que el otro servidor aceptó la actividad; si luego la muestra depende de ese servidor.
- Los intentos se conservan 30 días, así que el historial llega 30 días atrás.
- Para acotar la lista, añade `activity_uri=<the activity's ID>`, o `account_id=<a local account's ID>` para las entregas de una cuenta.
  Una consulta con `account_id` se anota en el registro de auditoría como `account_deliveries_viewed`, con la cuenta y el servidor; las otras consultas no se anotan.
- El cuerpo de la actividad nunca se muestra.

Algunas entregas están enmascaradas: su remitente, URI de actividad e inbox se omiten, y `mask` dice por qué.

| `mask` | La actividad trata de |
| --- | --- |
| `private`, `direct`, `local` | una publicación con esa visibilidad |
| `approves_followers` | un seguimiento, su deshacer, una aceptación o un rechazo de una cuenta local que aprueba a sus seguidores |
| `unknown` | una publicación o una cuenta que ya no existe, así que ya no se sabe qué era |

Un inbox compartido no nombra a ningún destinatario y siempre se muestra; un inbox personal enmascarado en el error de un intento se lee `[masked]`.
El tipo, las horas y todos los intentos de una entrega enmascarada se muestran, así que aún puedes ver si llegó al servidor.
Para ver los campos enmascarados de una entrega, usa `POST /api/v1/admin/instances/{domain}/deliveries/{id}/reveal`; cada llamada se anota en el registro de auditoría como `delivery_revealed` antes de mostrar nada.

<!-- source: reports 5791186f6b24 -->
## Denuncias

Las denuncias (`Flag`) recibidas de otros servidores entran en la lista de denuncias como abiertas (`open`).

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

En el cliente web, **Moderación → Denuncias** (`/moderation/reports`) enumera las denuncias abiertas, las más recientes primero, con quién presentó cada una, su categoría, cuándo y quién la gestiona.
Sus filtros (estado, categoría y quién la gestiona: tú o nadie) y el orden (la columna **Presentada**: las más recientes o las que llevan más tiempo esperando primero) permanecen en la dirección de la página, y **Anterior** y **Siguiente** recorren las páginas.
La página de una denuncia (`/moderation/reports/{id}`) muestra su estado, categoría, reglas citadas (una regla retirada desde entonces lo dice), quién la presentó y quién la gestiona, si fue al servidor de la cuenta y cuántas otras denuncias y acciones tiene la cuenta;
luego la cuenta; el comentario; las publicaciones denunciadas, una eliminada con cuándo se eliminó; lo que ocurrió en la sala de chat de voz o los cambios del evento; y las notas.
Desde `wide` la página de la lista de la que vienes está a su lado. **Asignármela** y **Quitar la asignación**, **Resolver** (**Se tomó una acción** o **No hizo falta ninguna acción**) y **Reabrir** están bajo su título, **Añadir una nota** bajo las notas.

En la API de administración: `GET /api/v1/admin/reports` (filtra con `state`, `category` y `assignee`: `me`, `none` o el ID de una cuenta local; `order=oldest` enumera primero la denuncia que lleva más tiempo esperando en lugar de la más reciente), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (cuerpo `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (cuerpo `{"body": "..."}`), `POST .../{id}/reopen` y `POST .../{id}/forward` (cuerpo `{"comment": "..."}`, opcional).

- Una asignación (`assign`) solo puede ir a un usuario local cuyo rol tenga `manage_reports`. En la API de administración te asignas a ti mismo.
- El resultado de una resolución (`resolve`) es `actioned` (se tomó una acción contra el destinatario) o `dismissed` (no hizo falta ninguna acción). Resolver de nuevo una denuncia resuelta no cambia el resultado.
- Cuando emites una acción desde una denuncia (`report_id` / `-report` de la acción), la denuncia se resuelve como `actioned` en la misma operación.
- Una nota (`note`) es una nota interna que solo ven los moderadores, de hasta 2000 caracteres. No se muestra ni al destinatario ni al denunciante. No puede editarse ni eliminarse.
- Reabrir (`reopen`) devuelve una denuncia resuelta a `open` (la asignación permanece). Una denuncia abierta no puede reabrirse (`409` en la API de administración).
- Reenviar (`forward`, **Enviar a su servidor…** al final de la página de la denuncia, que pregunta antes) envía una denuncia sobre una cuenta de otro servidor a ese servidor, como un `Flag` del actor de tu instancia con la cuenta y las publicaciones denunciadas que aún existen. El único texto que lleva es lo que escribes para los moderadores de ese servidor al reenviarla (`-comment`, `comment` en la API de administración, o **Texto para sus moderadores** en la página de la denuncia; hasta 1000 caracteres, ninguno por defecto). El comentario del denunciante y la nota de una [revisión externa](#reports-that-an-external-review-filed) nunca salen de tu servidor de esta manera, y no se envía nada del denunciante. Una denuncia que su denunciante eligió reenviar al presentarla salió de inmediato, con el comentario del denunciante. Reenviar funciona con una denuncia presentada en este servidor, por una persona o por una [revisión externa](#reports-that-an-external-review-filed), abierta o resuelta, que aún no se reenvió; una denuncia recibida de otro servidor o sobre una sala de chat de voz o un evento no puede reenviarse (`409` `report-not-forwardable` en la API de administración, y los detalles de la denuncia no muestran ningún botón). Una denuncia se envía una vez: reenviarla de nuevo, con cualquier texto, o reenviar una que su denunciante había reenviado al presentarla, no cambia nada. `forwarded` en la API de administración dice si se envió, `forwardable` si puedes enviarla ahora.
- Los detalles de la denuncia (`show`, `GET .../{id}`) devuelven las notas, las más antiguas primero, con el número de otras denuncias sobre el destinatario y el número de acciones contra el destinatario (incluidas las levantadas y caducadas). La lista no devuelve notas.
- Las asignaciones, resoluciones, reaperturas, notas y reenvíos se conservan como eventos (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), y `report_states` contiene el estado actual.
- Los moderadores pueden traducir las publicaciones denunciadas y el comentario del denunciante en la página de la denuncia, o con `POST .../{id}/statuses/{status_id}/translation` y `POST .../{id}/comment/translation` (cuerpo `{"target_language": "en"}`).
  El texto va a un proveedor de traducción solo mientras el [ajuste de la instancia](instance-settings.md) `translation_reports_to_server` está activado, y entonces solo a un proveedor al que se permite la visibilidad de la publicación: el acceso de un moderador no amplía nada, así que una publicación directa va solo a un proveedor al que se permite `direct`, y el comentario cuenta como `private`.
  La traducción del comentario no se conserva, ni tampoco la de una publicación eliminada, y los caracteres cuentan solo contra el límite mensual del servidor. La traducción integrada del navegador, cuando `translation_browser` está activado, funciona con las publicaciones denunciadas diga lo que diga `translation_reports_to_server`; el comentario, que no nombra ningún idioma, solo lo traduce el servidor.
- Un denunciante ve del destinatario solo lo que vería de la cuenta en otro caso: mientras está suspendida o lo bloquea, su denuncia sigue en la lista sin la cuenta. Los moderadores siempre ven todo el destinatario.
- Un denunciante local recibe una notificación (`report_resolved`) en la primera resolución. Reabrir y resolver de nuevo no añade ninguna notificación.
  Un denunciante puede enumerar sus propias denuncias con `GET /api/v1/reports` y ve solo si cada una está abierta o resuelta. El resultado, la acción tomada, el motivo dado al destinatario y la persona asignada no se muestran.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Denuncias que presentó una revisión externa

Un [gancho de decisión](decision-hooks.md) que responde `flag` presenta una denuncia con el actor de tu instancia como denunciante. La denuncia
lo indica: `hook_origin` en la API de administración y en el webhook `report.created`, una línea en los detalles de la denuncia, y `automated` en
`my admin report show`. Su comentario es la nota del plugin, y su categoría proviene del motivo. No se reenvía cuando
se presenta, y resolverla no notifica a nadie. Cuando el objetivo está en otro servidor, puedes reenviarla
tú mismo (`forward`): la nota del plugin se queda en tu servidor, y solo sale el texto que tú escribas. Cancelar la decisión
deja la denuncia como está.

<!-- source: voice-chat-rooms d2dd76116ac4 -->
### Salas de chat de voz

- Un usuario local puede denunciar una sala de chat de voz abierta que puede ver (`POST /api/v1/reports` con `voice_room_id`, y `account_id` definido como el anfitrión de la sala).
  La denuncia apunta al anfitrión en ese momento y señala los ajustes de la sala (nombre y descripción) tal como eran entonces. No señala ninguna publicación y nunca se reenvía a otro servidor.
- Los detalles de la denuncia (`GET /api/v1/admin/reports/{id}`) muestran la sala (`voice_room`): cada cambio de sus ajustes, quién participó y cuándo se fue, quién fue expulsado de ella y cómo terminó.
  Los registros de quién participó se conservan 90 días después de que la sala termina (`voice_participants_retention_days`) y mientras una denuncia señale la sala ([conservación de registros](#record-retention-and-holds)).
  No hay grabaciones ni chat de texto, así que estos registros son la única prueba.
- Quien tiene `manage_reports` puede terminar una sala abierta para todos los participantes con `POST /api/v1/admin/voice_rooms/{id}/end` (cuerpo `{"reason": "..."}`, opcional, hasta 500 caracteres).
  El motivo es solo para los moderadores; los participantes ven solo que la sala terminó. Funciona también mientras el chat de voz está desactivado, y el registro de auditoría anota `voice_room_ended`.
  En el cliente web, la página de la denuncia de una sala abierta tiene «Terminar la sala», que pregunta antes, con el motivo.

<!-- source: events cbc3e19bfdb6 -->
### Eventos

- Un usuario local puede denunciar un evento que puede ver, incluso uno cancelado (`POST /api/v1/reports` con `event_id`, y `account_id` definido como el organizador del evento).
  La denuncia apunta al organizador y señala los ajustes del evento (título, descripción y lugares) tal como los vio el denunciante. Un evento que aún sondea una fecha puede denunciarse y cancelarse como cualquier otro. No señala ninguna publicación y nunca se reenvía a otro servidor.
- Los detalles de la denuncia (`GET /api/v1/admin/reports/{id}`) muestran el evento (`event`): su fecha, si se canceló y cómo, y cada cambio de sus ajustes, el más antiguo primero, con todos sus lugares, también los que solo veían los asistentes aprobados.
  `reported_revision_id` es la versión que vio el denunciante, así que un título o una dirección cambiados después de la denuncia siguen ante ti.
- Quien tiene `manage_reports` puede cancelar un evento con `POST /api/v1/admin/events/{id}/cancel` (cuerpo `{"reason": "..."}`, opcional, hasta 500 caracteres).
  El motivo se muestra con la cancelación a todos los que ven el evento. Los usuarios que respondieron que van o quizá y los usuarios invitados reciben notificación, como en la propia cancelación del organizador, y el registro de auditoría anota `event_cancelled`.
  Un evento ya cancelado responde `409` `event-closed`. En el cliente web, la página de la denuncia de un evento tiene «Cancelar el evento», que pregunta antes, con el motivo.
- Suspender o eliminar al organizador cancela los eventos del organizador que aún no han empezado, incluidos los que aún sondean una fecha (`gathering.cancel_organized`), y notifica a sus asistentes. Estas cancelaciones no nombran a nadie como quien las canceló, y levantar la suspensión no las restaura. Los eventos que ya empezaron no se cancelan.

<!-- source: notifications-for-moderators-and-the-pending-counts 0d1c327abe83 -->
## Notificaciones para moderadores y recuentos pendientes

Las denuncias nuevas, los registros pendientes de aprobación y las apelaciones llegan a las personas con el permiso como notificaciones en la aplicación.

| Notificación | Cuándo llega | Quién la recibe |
| --- | --- | --- |
| `admin_report` | Cuando el destinatario de la denuncia no tiene otra denuncia abierta (una segunda denuncia o posterior sobre la misma cuenta no notifica) | Personas cuyo rol tiene `manage_reports`, salvo el denunciante y el destinatario de la denuncia |
| `admin_sign_up` | Cuando un registro empieza pendiente de aprobación (modo de aprobación, o `open` en una configuración que no puede enviar correo) | Personas cuyo rol tiene `manage_accounts` |
| `admin_appeal` | Cuando se presenta una apelación contra una acción | Personas cuyo rol tiene `manage_accounts`, salvo la persona que apeló |
| `admin_custom_emoji_request` | Cuando se solicita crear un emoji personalizado o sustituir su imagen | Personas cuyo rol tiene `manage_custom_emojis`, salvo el solicitante |
| `admin_error_group` | Cuando el servidor o el cliente web informa de un error de un tipo nuevo, o un grupo de errores que se resolvió ocurre de nuevo en otra versión del servidor | Personas cuyo rol tiene `manage_errors` |
| `admin_domain_list_failing` | Cuando falla por tercera vez seguida la obtención de una lista de bloqueo suscrita | Personas cuyo rol tiene `manage_domains` |

- `admin_error_group` llega como máximo 10 veces por hora para todo el servidor. El resto de la hora se pliega en una notificación
  que dice cuántos grupos de errores más hubo. Un grupo silenciado nunca notifica. Un error que el cliente web notifica por un
  visitante que no ha iniciado sesión notifica solo cuando el navegador de un usuario con sesión iniciada también lo notifica, y su reaparición no notifica a nadie.
  Los grupos de errores se leen y se resuelven, silencian o dejan de silenciarse mediante `GET /api/v1/admin/error_groups` y
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`), que necesitan `manage_errors`.
- Quién recibe una notificación lo decide su permiso cuando se crea la notificación. Una persona que pierde después el permiso ya no ve ese tipo en su lista ni en su recuento de no leídas. Si el permiso vuelve, lo ve de nuevo.
- Cada tipo de notificación de arriba también llega por correo, activado por defecto. Cada persona activa o desactiva cada tipo en la aplicación y por
  correo en la página **Notificaciones** de la configuración (`/settings/notifications` en el cliente web), que enumera solo los
  tipos que recibe su rol. Un tipo desactivado en la aplicación no se crea para ella, así que ni se envía por push ni por correo; los recuentos de
  `GET /api/v1/admin/counts` no cambian, porque cuentan las propias denuncias, registros y apelaciones.
  Con las denuncias abiertas, los recuentos también devuelven cuándo se presentó la denuncia abierta más antigua (`oldest_open_report_filed_at`).
  El servidor encola el job de correo en la transacción que registra la notificación, y el job envía el correo justo después del
  commit, sin esperar a que se lea la notificación. Si la persona ha perdido el permiso o ha desactivado el correo de ese tipo
  en el momento del envío, o su dirección principal no está confirmada o sus correos están detenidos, no se envía nada.
  El correo contiene solo el tipo de notificación y un enlace a la pantalla de notificaciones, y nada del contenido de la denuncia ni de la cuenta. Su enlace de baja desactiva solo el correo de ese tipo.
  Con `mail = none`, no se envía correo, y `sends_email` en `GET /api/v1/instance` es `false`.

`GET /api/v1/admin/counts` devuelve el número de denuncias abiertas (`open_reports`, necesita `manage_reports`), el número de registros pendientes de aprobación (`pending_registrations`) y el número de apelaciones pendientes de revisión (`pending_appeals`) (ambos necesitan `manage_accounts`), el número de solicitudes de emoji personalizados pendientes de decisión (`pending_custom_emoji_requests`, necesita `manage_custom_emojis`) y el número de grupos de errores abiertos, nuevos, regresados o que dejaron de silenciarse (`open_error_groups`, necesita `manage_errors`).
Los elementos para los que no tienes el permiso no se devuelven.

<!-- source: audit-log 4d53a2539898 -->
## Registro de auditoría

Las operaciones de moderadores y administradores se anotan, una fila cada una, en el registro de auditoría (`audit_log_entries`) en la misma transacción que la operación.
Las operaciones fallidas y las que no cambiaron nada no se anotan: levantar una acción ya levantada, resolver una denuncia resuelta, asignar a la misma persona, retirar una regla ya retirada, mover a la misma posición, conceder un rol que el usuario ya tiene y revocar un rol que el usuario no tiene.

| Tipo de destino | Operaciones |
| --- | --- |
| `account` | `account_action_issued` y `account_action_lifted` (emitir y levantar una acción contra una cuenta), `account_deletion_canceled` (la eliminación cancelada porque se revocó una apelación contra un `ban`), `revealed_private_data` (direcciones IP y direcciones de correo reveladas), `two_factor_disabled` (autenticación en dos pasos quitada), `password_reset_link_issued` (enlace de restablecimiento de contraseña emitido), `email_suppression_lifted` (supresión de correo levantada), `account_moved` (una cuenta local restringida se mudó a una cuenta local; las restricciones no se trasladan al destino, así que decides si aplicarlas allí), `account_statuses_refetch_queued` (nueva obtención de las publicaciones de una cuenta remota), `account_deliveries_viewed` (se enumeraron las entregas de una cuenta local a un servidor; el nombre del destino es el identificador y el servidor) |
| `status` | `status_refetch_queued` (nueva obtención de una publicación remota; el nombre del destino es la URI de la publicación) |
| `domain` | `domain_action_issued` y `domain_action_lifted` (emitir y levantar una acción de dominio), `domain_statuses_refetch_queued` (nueva obtención de las publicaciones de las cuentas del dominio), `instance_marked_available` (borrar los fallos de un servidor), `delivery_revealed` (se mostraron el remitente, la URI de actividad y el inbox enmascarados de una entrega al servidor) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (asignar, quitar la asignación, resolver y reabrir una denuncia, añadir una nota y reenviarla) |
| `user` | `registration_approved`, `registration_rejected` (revisión del registro) |
| `setting` | `setting_changed` (ajustes de la instancia), `text_changed` (textos de la instancia) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (reglas de la instancia) |
| `role` | `role_granted`, `role_revoked` (conceder y revocar un rol a un usuario), `role_created`, `role_updated`, `role_deleted` (crear, cambiar y eliminar un rol) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (revisión de apelación; revocar también anota `account_action_lifted` para la acción, y `account_deletion_canceled` para un `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (añadir y levantar un bloqueo de registro) |
| `hook_decision` | `hook_decision_applied` (el servidor actuó según el veredicto de una revisión externa y dejó estado; el actor es el sistema), `hook_decision_overridden` (un moderador canceló la decisión, también al revocar su apelación), `hook_appeal_upheld` (un moderador mantuvo la apelación de una publicación rechazada, y la decisión permanece) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (registrar un emoji personalizado, sustituir su imagen, copiar un emoji de otro servidor, cambiarlo, activarlo, desactivarlo y eliminarlo) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (crear, cambiar, eliminar, activar y desactivar un webhook, y regenerar su secreto) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn`, `announcement_deleted` (crear, cambiar, publicar y retirar un anuncio, y eliminar un borrador) |
| `invite` | `invite_created`, `invite_revoked` (crear y revocar una invitación; el nombre del destino son los 4 primeros caracteres del código) |
| `billing_source` | `billing_import_queued` (una importación de facturación se inició a mano; el nombre del destino es el nombre del proveedor) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (registrar un coste, cambiar su nombre y similares, revisar su importe, cambiar su fecha de fin y eliminarlo; el nombre del destino es el nombre del concepto) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (activar y desactivar la facturación; el nombre del destino es el nombre del proveedor de pagos), y `supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled` (activar las propinas, cambiar sus importes y desactivarlas; el nombre del destino es la moneda y los importes en unidades menores) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (crear, cambiar y retirar un plan de colaborador; el nombre del destino es el nombre del plan) |
| `supporter_subscription` | `supporter_subscription_ended` (un administrador terminó una suscripción de inmediato; el nombre del destino es el nombre del plan y el identificador del usuario) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (enviar, retirar, aprobar y rechazar una solicitud de emoji personalizado; el nombre del destino es el `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (la conciliación con el proveedor de pagos se inició a mano; el nombre del destino es el nombre del proveedor) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (desactivar y revocar un cliente OAuth; desactivar revoca todos sus tokens; el nombre del destino es el nombre del cliente y `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (denegar y revocar la denegación de un host de Client ID Metadata Document; denegar revoca los tokens de los clientes de ese host; el nombre del destino es el host) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (anunciar el cierre del servicio o cambiar la fecha, retirar el anuncio, cerrar y completar la entrega del cierre; el nombre del destino es `service_closure`) |
| `voice_room` | `voice_room_ended` (un moderador terminó una sala de chat de voz abierta; el nombre del destino es el ID de la sala, ya que el nombre desaparece cuando se borra la cuenta que lo escribió; los detalles de la denuncia muestran el nombre) |
| `event` | `event_cancelled` (un moderador canceló un evento; el nombre del destino es el ID del evento, ya que el título desaparece cuando se borra la cuenta que lo escribió; los detalles de la denuncia muestran el título) |
| `domain_allow` | `domain_allow_added`, `domain_allow_removed` (añadir un dominio a la lista de permitidos de la federación y quitarlo; consulta [federar solo con los servidores permitidos](#federating-only-with-allowed-servers); el nombre del destino es el dominio) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (definir una regla del rastreador de tarjetas de enlace sobre un dominio y quitarla; consulta [dominios de tarjetas de enlace](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (añadir, cambiar, eliminar, activar y desactivar un proveedor de inicio de sesión externo, y sustituir su secreto de cliente; el nombre del destino es el nombre para mostrar del proveedor) |
| `domain_list_subscription` | `domain_list_subscription_created`, `domain_list_subscription_updated`, `domain_list_subscription_deleted` (suscribirse a una lista de bloqueo, cambiar sus ajustes y quitarla), `domain_list_draft_accepted` y `domain_list_draft_rejected` (aceptar un borrador, lo que también anota la acción de dominio, y rechazar uno), `domain_list_fetch_queued` (una obtención pedida a mano); el nombre del destino es el título de la suscripción; consulta [suscribirse a listas de bloqueo](#subscribing-to-blocklists) |

La lista muestra si el destino aún existe. Las cuentas, denuncias, apelaciones y bloqueos de registro que desaparecieron tras su periodo de conservación, los permisos quitados y los roles, emoji personalizados y webhooks eliminados muestran solo el nombre en el momento del registro y ningún enlace a una pantalla.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

En la API de administración: `GET /api/v1/admin/audit_log` (filtra con `actor_account_id`, `action`, `target_kind` y `target_id`). Necesita el permiso `view_audit`.
En el cliente web, abre Moderación → Registro de auditoría (`/moderation/audit-log`): cada entrada se lee como una frase de quién hizo qué a qué, la más reciente primero, con la persona y el destino enlazados a sus pantallas.
Fíltralo por quién lo hizo (tú, o la persona de «Sus otras operaciones» de una entrada), por el tipo de destino y luego de operación, y por un destino (el «Otras operaciones sobre esto» de una entrada).

- Una fila es el actor (el usuario para `user`, o `cli` para la CLI), la operación, el tipo y el ID del destino, el nombre para mostrar del destino, el ID del registro original y la hora.
  El ID del destino es el ID de la cuenta, denuncia, apelación, usuario, regla, rol o bloqueo de registro; para una acción de dominio es el dominio; para ajustes y textos es la clave.
- El nombre para mostrar del destino (`@alice`, `@bob@example.com`, un dominio, una clave de ajuste, el texto de una regla; para un texto su clave, para un rol su clave y el identificador del usuario, para un bloqueo de registro su tipo y valor) es una copia tomada en el momento del registro, así que sigue siendo legible después de que el destino desaparezca.
  Cada fila de la API también devuelve si el destino aún existe (`target_exists`), que es `false` para las cuentas, denuncias, apelaciones y bloqueos de registro que ya no están. El registro de auditoría web muestra solo el nombre para mostrar, sin enlace, para un destino que ya no está.
- El texto de los motivos y las notas no se copia. Búscalo mediante el ID del registro original (la acción, asignación, resolución, reapertura, nota, revisión o la versión de un ajuste o regla).
- La lista va con la más reciente primero con paginación por clave (`-before <entry id>` en la CLI).
- No hay API ni CLI para actualizar o eliminar el registro de auditoría. Las filas antiguas se eliminan tras el [periodo de conservación](#record-retention-and-holds).

<!-- source: record-retention-and-holds 9861d8570a91 -->
## Conservación de registros y retenciones

Tras el período de retención de cada tipo de registro, trabajos horarios (`moderation.purge_records`, `identity.purge_origins`, `voice.purge_records` y `gathering.purge_records`) eliminan los registros de moderación.
Puedes cambiar los períodos en la configuración de la instancia, pero no por debajo del límite inferior ([configuración de la instancia](instance-settings.md)). Un período cambiado surte efecto desde la siguiente ejecución. Una retención liberada, con su motivo, se elimina cuando ha pasado desde su liberación el período de retención de las acciones de moderación.

| Registro | Configuración | Por defecto | Límite inferior | Cuenta desde |
| --- | --- | --- | --- | --- |
| Dirección IP y User-Agent del inicio de sesión | `session_origins_retention_days` | 180 días | 90 días | El inicio de sesión |
| Dirección IP del registro | `registration_origins_retention_days` | 180 días | 90 días | El registro |
| Denuncias (comentario, publicaciones objetivo, notas, asignaciones, resoluciones, reaperturas) | `reports_retention_days` | 365 días | 180 días | La última resolución |
| Acciones contra cuentas (motivo, nota, publicaciones objetivo, levantamiento) y apelaciones | `moderation_actions_retention_days` | 365 días | 180 días | El fin de la acción |
| Registro de auditoría | `audit_log_retention_days` | 730 días | 365 días | El registro |
| Bloqueos de registro | Ninguno (no se puede cambiar) | 30 días | n/a | Lo primero entre el levantamiento y la caducidad |
| Quién participó en una sala de chat de voz (participaciones, oradores y oyentes, manos levantadas, invitaciones, expulsiones) | `voice_participants_retention_days` | 90 días | 30 días | El fin de la sala |
| Las decisiones de los [ganchos de decisión](decision-hooks.md#the-decisions) | `hook_decisions_retention_days` | 90 días | 30 días | La decisión |
| Las respuestas, aprobaciones, invitaciones y votos sobre las fechas candidatas de un evento | `event_records_retention_days` | 90 días | 30 días | Lo primero entre el fin del evento (para un evento cuya fecha nunca se fijó, el fin de su última fecha candidata) y su cancelación |

- Las denuncias abiertas nunca se eliminan. Una denuncia a la que se refiere una acción (`report_id` de la acción) se mantiene hasta que se elimine esa acción.
- El fin de una acción es lo primero entre el levantamiento y la caducidad. Para una acción que fue apelada, si la revisión llega más tarde, el fin es la hora de la revisión, y la acción se conserva mientras la apelación espera revisión.
  `warn` y `delete_statuses` no tienen efecto duradero, así que el fin es la hora en que se emitieron. Una acción sin fin (un `ban`, un `suspend` sin hora de fin, etc.) no se elimina hasta que se levanta.
- Las publicaciones a las que apunta una denuncia o una acción no se eliminan físicamente por el trabajo de purga, con sus archivos multimedia adjuntos, hasta que se elimine esa denuncia o acción, aunque el autor las haya eliminado (desaparecen de la vista). Las publicaciones remotas también se mantienen más allá del período de retención (incluido la configuración de instancia `remote_status_max_retention_days`).
- Un bloqueo de registro que no tiene ni levantamiento ni hora de fin se mantiene mientras esté en vigor.
- Los registros de quién participó en una sala de chat de voz a la que apunta una denuncia se mantienen hasta que se elimine esa denuncia. La sala en sí (su nombre, descripción, anfitriones y cómo terminó) nunca se elimina.
- Las respuestas, votos e invitaciones de un evento al que apunta una denuncia se mantienen hasta que se elimine esa denuncia, y también los que nombran una cuenta bajo una retención. El evento en sí (sus ajustes y lugares, su fecha y fechas candidatas, sus coorganizadores y su cancelación) nunca se elimina; una vez desaparecidas sus respuestas, no cuenta a nadie como asistente.
- Una ejecución del trabajo elimina solo un número limitado de filas por tipo (100 cada vez, 20 veces, para acciones, denuncias y bloqueos de registro; 1.000 filas cada vez, 20 veces, para el registro de auditoría). La siguiente ejecución elimina el resto.

<!-- source: holds f3bf57c2c258 -->
### Retenciones

Mientras una orden judicial prohíbe la eliminación (artículo 16 de la Ley japonesa de Plataformas de Distribución de Información) o se está tramitando una solicitud de divulgación, puedes detener la eliminación por cuenta.

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- Para una cuenta bajo retención, sus acciones y apelaciones, las denuncias sobre ella y las que hizo, los registros de auditoría de estas, las direcciones IP de inicio de sesión y de registro, sus publicaciones y los registros de las salas de chat de voz en las que participó o actuó no se eliminan tras el período de retención.
  Un bloqueo de registro creado a partir de la expulsión de esa cuenta también se conserva pasados 30 días desde el levantamiento o la caducidad.
- Puedes poner varias retenciones sobre una misma cuenta. Cuando las liberas todas, los registros que superan su período se eliminan en la siguiente ejecución. Liberar una retención ya liberada no cambia nada.
- Las retenciones y su liberación se hacen solo con la CLI, y el actor del registro es la CLI.
- Los datos de una cuenta eliminada (una cuyo `ban` llegó a su fecha, una que el usuario eliminó o una cuenta remota que su servidor eliminó) se borran 7 días después de la eliminación mediante el trabajo horario `account.erase_deleted`. Las publicaciones de una cuenta remota también se van, incluidas las que usuarios locales guardaron como marcador o respondieron.
  Una cuenta remota que se marcó como eliminada solo porque una obtención respondió `410 Gone`, sin su propio `Delete` firmado, se vuelve a obtener una vez más antes. Un segundo `410` la borra. Si su servidor vuelve a servirla, regresa con sus publicaciones y no se borra nada. Cualquier otra respuesta (`404`, o ninguna respuesta) espera a una ejecución posterior, y 30 días después de la eliminación la cuenta se borra de todos modos.
  Una cuenta bajo retención no se borra. Las publicaciones a las que apunta una denuncia o una acción se mantienen hasta que se elimine ese registro. Se conservan el URI y el nombre de usuario de la cuenta y los registros de acciones.
  Los nombres y las descripciones que la cuenta dio a sus salas de chat de voz se vacían; las salas se mantienen.

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### Exportar para solicitudes de divulgación

Ante una solicitud de divulgación (artículo 15 del RGPD, artículo 33 de la Ley japonesa de Protección de la Información Personal), responde exportando en JSON todos los registros de moderación de una cuenta.

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- La exportación contiene las acciones contra la cuenta (incluidas las notas privadas, el levantamiento, las apelaciones y las revisiones), las denuncias sobre la cuenta (comentario, publicaciones objetivo, reglas citadas, notas de los moderadores, asignaciones, resoluciones, reaperturas), las denuncias que hizo la cuenta (solo si cada una está abierta o resuelta) y los registros de auditoría de estas.
- La identidad de los denunciantes (la cuenta denunciante, y el URI y el servidor de origen de una denuncia recibida) no se exporta. Antes de entregar la exportación, comprueba que el comentario de una denuncia no diga nada que identifique al denunciante.
- Un moderador se exporta por ID de usuario. Establece primero una retención para que los registros no se eliminen mientras tramitas la solicitud.
