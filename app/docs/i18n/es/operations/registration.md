<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# Registro de usuarios (modos de registro, invitaciones, aprobación, confirmación del correo electrónico)

Cómo se registran los usuarios y confirman su dirección de correo electrónico, los modos de registro, los códigos de invitación, la aprobación de solicitudes, el restablecimiento de contraseñas y los bloqueos de registro.

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## Del registro a la aplicación

![Los caminos de un usuario nuevo: el formulario de registro o un proveedor de inicio de sesión, las comprobaciones que pueden rechazar el registro, el estado que da cada modo de registro, y la aprobación y la confirmación del correo electrónico antes de llegar a la aplicación. La lista de abajo dice lo mismo.](images/sign-up-flow.svg)

De arriba abajo en el diagrama:

- **Inicio.** Un visitante crea una cuenta en la página de registro (`/auth/sign_up`) con un nombre de usuario, una dirección de correo electrónico y una
  contraseña; la página no ofrece este formulario mientras [el registro esté limitado a proveedores de inicio de sesión](#registration-limited-to-login-providers).
  O bien el visitante inicia sesión con un [proveedor de inicio de sesión](login-providers.md) cuya cuenta todavía no está vinculada a un usuario; cuando
  el proveedor permite registros (`-allow-signup`), el visitante recibe el mismo formulario sin el campo de contraseña.
- **Comprobaciones, en orden.** El modo de registro (`closed` rechaza todos los registros), el límite por red (5 cada 30 minutos),
  el CAPTCHA (cuando `captcha` no es `none`), los [bloqueos de registro](#sign-up-blocks) (`reject` rechaza,
  `require_approval` deja el registro pendiente de aprobación), el código de invitación (debe poder usarse, y el modo `invite` exige uno), el plugin de
  [`signup.check`](decision-hooks.md#sign-ups) para un registro sin invitación, y que el nombre de usuario y la dirección de correo electrónico
  no estén en uso. Un registro que no supera una comprobación se rechaza con un error en el formulario y no se escribe nada.
- **Estado justo después de registrarse.** El usuario nuevo inicia sesión de inmediato. En el modo `open` el registro queda aprobado,
  o pendiente de aprobación con `mail = none`; en el modo `approval` queda pendiente de aprobación, o aprobado si se usa una invitación válida;
  en el modo `invite` queda aprobado. Un bloqueo `require_approval` o una retención por `signup.check` lo deja pendiente en cualquier modo.
- **Hasta que la cuenta esté lista.** Un registro pendiente necesita la aprobación de un moderador (`my registration approve`, o
  las pantallas de administración). La dirección de correo electrónico debe confirmarse con el enlace de un correo si `mail = smtp`; una dirección que
  el proveedor de inicio de sesión verificó queda confirmada de inmediato, y con `mail = none` no hace falta confirmación. Un registro
  rechazado no queda aprobado, y la cuenta no puede usar la aplicación.
- **La aplicación.** Una cuenta lista pasa al paso de bienvenida (un nombre para mostrar y un avatar) y después a la cronología de inicio.

Las secciones siguientes describen cada paso.

<!-- source: the-sign-up-page 111fbe5ecf64 -->
## La página de registro

Los usuarios se registran en `https://<host>/auth/sign_up` con un nombre de usuario, una dirección de correo electrónico y una contraseña, salvo que [el registro esté limitado a proveedores de inicio de sesión](#registration-limited-to-login-providers).
Tras registrarse, ese navegador inicia sesión de inmediato.
La página funciona sin JavaScript y, como la página de inicio de sesión, está protegida con un token CSRF y rechaza las solicitudes de otro origen.
Si eliges CAPTCHA como protección contra bots, las páginas de registro y de inicio de sesión muestran su widget y necesitan JavaScript ([CAPTCHA](captcha.md)).

El modo de registro es el ajuste de instancia `registration_mode`. Elígelo con la CLI o en el cliente web, en la página «Registro» de «Configuración del servidor» (`/admin/settings/registration`) ([configuración de la instancia](instance-settings.md)). Un cambio se aplica desde el siguiente registro.
Mientras el servidor anuncia su cierre y después de que cierre, el registro se trata como `closed` sea cual sea el ajuste ([CLI de administración](admin-cli.md#announcing-the-end-of-the-service)).
Lo mismo ocurre en un [servidor de una sola persona](instance-settings.md#one-person-server), donde tampoco se ofrecen las invitaciones, la revisión de registros ni los bloqueos de registro, y ninguna vía crea una cuenta, tampoco `my admin create`.

```sh
my instance set registration_mode invite
```

| Modo | Página | Estado justo después de registrarse |
| --- | --- | --- |
| `open` | Cualquiera puede registrarse; el código de invitación es opcional | Aprobado, salvo con `mail = none`, donde queda pendiente de la aprobación de un moderador (véase más abajo). Una invitación solo registra quién invitó al usuario |
| `approval` (predeterminado) | Pregunta además «¿Por qué quieres unirte?» (opcional, hasta 1000 caracteres); el código de invitación es opcional | Pendiente de aprobación; aprobado de inmediato con una invitación válida |
| `invite` | El código de invitación es obligatorio | Aprobado |
| `closed` | Solo dice que no se aceptan registros | Ninguno |

- En todos los modos salvo `closed`, la página tiene un campo para el código de invitación, que se rellena con el enlace de una invitación (`?invite=<code>`). Un código que no funciona (desconocido, revocado, caducado, agotado, o de un miembro que ya no puede invitar) se rechaza en todos los modos, no se ignora, para que un código mal escrito no pase inadvertido.
- Un bloqueo de registro que exige aprobación mantiene un registro pendiente de aprobación incluso con una invitación ([bloqueos de registro](#sign-up-blocks)).
- Si hay un plugin configurado para el gancho de decisión [`signup.check`](decision-hooks.md#sign-ups), un registro sin invitación también lo revisa ese plugin después del CAPTCHA y de los bloqueos, y puede dejarlo pendiente de aprobación o rechazarlo.

- Los registros desde la misma red (dirección IP; `/64` en IPv6) se limitan a 5 cada 30 minutos. Pasado ese límite, el servidor devuelve `429` con `Retry-After`.
- Si el nombre de usuario o la dirección de correo electrónico ya están en uso, la página muestra un error en ese campo y no escribe nada (tampoco se consume una invitación).
- Cuando configuras [proveedores de inicio de sesión externos](login-providers.md) (OpenID Connect, GitHub, Discord o Mastodon) y dejas que los usuarios nuevos se registren con uno, las páginas de inicio de sesión y de registro muestran un botón por cada proveedor habilitado, con el nombre para mostrar del proveedor tal como lo escribiste (un proveedor de GitHub muestra «Iniciar sesión con GitHub» y uno de Discord «Iniciar sesión con Discord», en el idioma del visitante).
  Quien tiene una cuenta en el proveedor que no está vinculada a ningún usuario de este servidor recibe el mismo formulario de registro sin el campo de contraseña, y el usuario nuevo inicia sesión solo con el proveedor.
  El modo de registro, el código de invitación, el motivo, el CAPTCHA, los bloqueos de registro, `signup.check` y el límite por red se aplican igual que en el formulario con contraseña; en el modo `closed`, o cuando el proveedor no permite registros, la página de inicio de sesión indica que no hay ninguna cuenta vinculada.
  Una dirección que el proveedor marca como verificada (`email_verified`; en GitHub, la dirección principal cuando está verificada; en Discord, la dirección cuando Discord la marca como verificada) queda confirmada de inmediato si el usuario la conserva; cualquier otra dirección, y todas las direcciones de un usuario que se registra con un servidor de Mastodon, que no da ninguna, se confirman por correo como se explica más abajo.
  Una dirección de correo electrónico que coincide con la de un usuario existente nunca vincula a ese usuario: el formulario dice que la dirección ya está en uso.
  Un usuario con autenticación de dos factores sigue introduciendo el código después de iniciar sesión con un proveedor.
  Un proveedor de GitHub pide a GitHub el ámbito `user:email`, y también `read:org` cuando indicas organizaciones permitidas; entonces el servidor comprueba en cada inicio de sesión que el usuario pertenece a una de ellas y rechaza el inicio de sesión en caso contrario. Registra `https://<host>/auth/external/<provider ID>/callback` como URL de devolución de llamada de tu aplicación OAuth de GitHub.
  Un proveedor de Discord pide a Discord los ámbitos `identify` y `email`, y también `guilds` cuando indicas servidores permitidos; entonces el servidor comprueba en cada inicio de sesión que el usuario pertenece a uno de ellos. Registra la misma URL de devolución de llamada en tu aplicación de Discord.
- Un usuario con la sesión iniciada vincula una cuenta de un proveedor en «Cuentas vinculadas» de la configuración, tras confirmar con la contraseña, una clave de acceso u otra cuenta vinculada, y solo con los proveedores que permiten vínculos.
  El navegador debe haber iniciado sesión como el mismo usuario en las páginas de inicio de sesión; de lo contrario, el servidor pide iniciar sesión primero.
  Se rechaza una cuenta de un proveedor que ya ha vinculado otro usuario, y un usuario vincula como máximo una cuenta por proveedor.
  Un usuario puede desvincular una cuenta a menos que sea la última forma de iniciar sesión (sin contraseña, sin claves de acceso y sin otra cuenta vinculada).
- Un usuario sin contraseña y sin claves de acceso confirma las operaciones que normalmente las piden (establecer una contraseña, añadir una clave de acceso, eliminar o trasladar la cuenta, vincular otra cuenta) iniciando sesión de nuevo en un proveedor vinculado.
  A un proveedor OpenID Connect se le pide que autentique de nuevo al usuario (`max_age=0` y `prompt=login`), y se rechaza una autenticación anterior.
  La confirmación sirve una vez, solo para esa operación y en un plazo de 10 minutos; iniciar sesión con una cuenta que no es la vinculada cuenta como un intento erróneo, con el mismo límite que una contraseña incorrecta.
- En cada registro e inicio de sesión, el servidor guarda la dirección IP del cliente (y, en el inicio de sesión, el User-Agent). Los registros se eliminan automáticamente tras 180 días
  (la configuración de instancia `session_origins_retention_days` y `registration_origins_retention_days`, de al menos 90 días). Los registros de una cuenta con una retención legal se conservan ([conservación de registros y retenciones](moderation.md#record-retention-and-holds)).
  Indica este periodo de conservación en tu política de privacidad ([plantilla de política de privacidad](privacy-policy-template.md)).

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## Cuándo puede un usuario usar la aplicación

Los usuarios que esperan aprobación, fueron rechazados o no han confirmado su dirección de correo electrónico pueden iniciar sesión igualmente en la página de inicio de sesión. También pueden los usuarios que desactivaron su propia cuenta, hasta la fecha de su eliminación: allí ven esa fecha y pueden reactivar o eliminar la cuenta ([moderación](moderation.md#accounts-that-their-owners-deactivated)).
Tras iniciar sesión, ven qué están esperando (la aprobación de un moderador o la confirmación de la dirección de correo electrónico), o que su registro no fue aprobado, y no pueden avanzar más.
Solo los usuarios aprobados pueden usar la aplicación (la autorización OAuth y la emisión de tokens de la API de clientes) y, en las configuraciones que exigen confirmar el correo electrónico, solo los que han confirmado su dirección.

Un usuario nuevo que puede usar la aplicación de inmediato pasa de la página de registro directamente a la aplicación, a un paso de bienvenida que pide
un nombre para mostrar y un avatar. Después, la cronología de inicio muestra una lista de primeros pasos y sugerencias de personas a las que
seguir ([sugerencias](instance-settings.md#suggestions-of-accounts)). «Continuar» en la página que confirma una dirección de correo electrónico
lleva al mismo paso de bienvenida.

Un administrador creado con `my admin create` cuenta como aprobado y confirmado.

<!-- source: email-confirmation 6c1b7759c2cf -->
## Confirmación del correo electrónico

El ajuste `mail` ([referencia de configuración](../reference/configuration.md#mail)) selecciona cómo se envía el correo.
Los ejemplos de SMTP, la configuración de DNS y cómo interpretar los fallos de entrega están en [envío de correo electrónico](email.md).

- `mail = smtp`: el servidor envía un correo de confirmación al registrarse. El enlace es válido durante 24 horas y funciona una sola vez.
  Al abrir el enlace aparece una página con un botón de confirmar, y la dirección queda confirmada cuando el usuario lo pulsa (así los escáneres que precargan los enlaces de los correos no consumen el token).
  Después de iniciar sesión, el usuario puede pedir que se le reenvíe el correo hasta 3 veces por hora.
  La conexión con el servidor SMTP siempre se cifra con STARTTLS, y el servidor verifica el certificado antes de autenticarse y enviar. Nunca envía a un servidor que no ofrece STARTTLS.
- `mail = none` (predeterminado): el servidor no envía correo y escribe `mail not sent (mail = none)` con la categoría del mensaje en el registro del servidor. Solo con `profile = development` la línea incluye además la dirección y el cuerpo, con el enlace de confirmación.
  Como el correo nunca llega al usuario, el servidor pide la aprobación de un moderador en lugar de la confirmación del correo electrónico.
  Así, un registro en el modo `open` también empieza pendiente de aprobación, y una dirección de correo electrónico sin confirmar no impide usar la aplicación.

El token de confirmación no se registra en ningún sitio salvo esta línea del registro de desarrollo (los registros de acceso no guardan las cadenas de consulta). La base de datos guarda solo el SHA-256 del token.

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## Registro limitado a proveedores de inicio de sesión

Mientras el ajuste de instancia `login_providers_only` está activado y hay al menos un [proveedor de inicio de sesión](login-providers.md#limit-sign-in-to-login-providers) habilitado, la página de registro no tiene formulario con contraseña y ofrece solo los proveedores que permiten registros; un código de invitación del enlace pasa al formulario de registro del proveedor. Se rechaza un registro con contraseña, sea cual sea el contenido del formulario. Si ningún proveedor permite registros, el registro se considera `closed`. Si no hay ningún proveedor habilitado, el ajuste no se aplica.

<!-- source: password-reset-and-change bec2baef5ab7 -->
## Restablecimiento y cambio de contraseña

Mientras se aplica el ajuste de instancia `login_providers_only`, se rechazan los enlaces de restablecimiento y establecer o cambiar una contraseña ([qué hace el límite](login-providers.md#limit-sign-in-to-login-providers)).

- `mail = smtp`: cuando un usuario introduce una dirección de correo electrónico en «¿Has olvidado tu contraseña?» de la página de inicio de sesión, el servidor envía un enlace para establecer una contraseña nueva al usuario cuya dirección principal es esa.
  La respuesta de la página es la misma exista o no la cuenta. El enlace es válido durante 1 hora y funciona una sola vez, y deja de funcionar si la contraseña cambia por otra vía mientras tanto.
  Las solicitudes se limitan a 5 cada 30 minutos por dirección IP de cliente, y el correo a 3 por hora por dirección (una solicitud que supera un límite no envía nada y recibe la misma respuesta). Cuando el CAPTCHA está habilitado, este formulario también se verifica.
- `mail = none`: la página dice que no puede enviar un enlace e indica al usuario que lo pida a un administrador. El administrador emite un enlace en la página de gestión de la cuenta de las pantallas de administración, o con `my admin account password-reset-link @alice`, y se lo da al usuario ([moderación](moderation.md)).
- Un usuario con la sesión iniciada puede cambiar la contraseña en «Contraseña» de la configuración introduciendo la contraseña actual (las entradas erróneas se limitan a 5 por hora y usuario).
- Un restablecimiento o un cambio revoca las demás sesiones y los tokens de aplicaciones del usuario (un cambio desde la configuración conserva solo la aplicación que lo hizo). Las claves de acceso y la autenticación de dos factores se mantienen.
  Un inicio de sesión que espera su código de dos factores y una autorización de aplicación sin completar también dejan de funcionar: el usuario los empieza de nuevo.
  Con `mail = smtp`, el servidor también envía al usuario un correo que avisa del cambio de contraseña.

<!-- source: invite-codes 5f98f9eb7d6f -->
## Códigos de invitación

Las invitaciones funcionan en los modos `open`, `approval` e `invite`. El personal crea invitaciones con la CLI, las pantallas de moderación y la API de administración que se describen abajo; los miembros también pueden crear las suyas ([invitaciones de miembros](#member-invites)).
En el modo `closed` no se puede crear ninguna invitación y ninguna funciona.

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` muestra el código (16 caracteres) y el enlace a la página de registro con el código ya rellenado (`https://<host>/auth/sign_up?invite=<code>`).
- Los códigos se comparan sin distinguir mayúsculas de minúsculas, espacios ni guiones.
- Aunque varias personas se registren a la vez, un código nunca se usa más allá de su límite (una restricción de unicidad en `invite_redemptions`).
- Un código revocado, caducado o agotado da siempre el mismo error (una invitación inutilizable).
- Además de la CLI, puedes crear, listar y revocar invitaciones en las pantallas de moderación del cliente web («Invitaciones», `/moderation/invites`) y mediante la API de clientes
  (`GET` / `POST /api/v1/admin/invites` y `POST /api/v1/admin/invites/{id}/revoke`). Ambas necesitan el permiso de rol `manage_invites`
  (el rol integrado `moderator` no lo tiene, así que para delegarlo crea un rol con ese permiso y concédelo).
  «Crear una invitación» en la lista abre el formulario (`/moderation/invites/new`), que muestra el código y el enlace de registro de la invitación nueva.
  La lista permite copiar el enlace de una invitación válida y revocarla.
- La lista muestra todas las invitaciones, las del personal y las de los miembros, con quién la creó (`created_by`) y cómo (`creator_kind`: `staff` o `member`). Quienes tienen `manage_invites` pueden revocar cualquiera de ellas.
- La página de revisión de la cuenta en las pantallas de moderación (`GET /api/v1/admin/accounts/{id}`, permiso `manage_accounts`) muestra la invitación con la que se registró la cuenta y quién la creó, es decir, la persona que invitó al usuario.
- Crear y revocar una invitación se registra en el registro de auditoría (`invite_created`, `invite_revoked`) igual desde la pantalla, la API y la CLI. El nombre del destino son los 4 primeros caracteres del código (`ABCD…`); el código completo no se copia al registro de auditoría.
  Revocar una invitación ya revocada no cambia nada, así que no se registra.

<!-- source: member-invites db3bff74ed69 -->
## Invitaciones de miembros

Los miembros pueden crear invitaciones propias, además del personal.
Quién puede hacerlo lo decide el ajuste de instancia `invite_creators` ([configuración de la instancia](instance-settings.md)):

- `permitted` (predeterminado): solo los usuarios cuyos roles tienen el permiso `create_invites`. Por defecto solo lo tiene el rol integrado `admin`. Puedes añadirlo a un rol que concedas a mano, o al rol de un [plan de apoyo](supporters.md), para que los mecenas puedan invitar a otras personas.
- `everyone`: todos los usuarios locales con la sesión iniciada.

En cualquier caso, un usuario cuya cuenta está suspendida o limitada, o cuyo registro no está aprobado, no puede crear invitaciones, y nadie puede mientras el modo de registro sea `closed`.
Quienes tienen `manage_invites` siguen creando invitaciones del personal, a las que no se aplica ninguno de los límites siguientes.

Un miembro crea, lista, copia y revoca sus propias invitaciones en la configuración del cliente web («Tus invitaciones», `/settings/invites`), o mediante la API de clientes (`GET` / `POST /api/v1/invites` y `POST /api/v1/invites/{id}/revoke`).
La configuración lista la página solo para un usuario que puede crear invitaciones ahora; un usuario que ya no puede sigue abriéndola desde su dirección para copiar y revocar las invitaciones que creó.
La lista indica, de cada invitación, si es válida, está revocada, caducada, agotada, o inutilizable porque su creador ya no puede invitar.

Los límites, que también son ajustes de instancia, se aplican solo a las invitaciones de miembros.
Los administradores los cambian, igual que `invite_creators`, en «Invitaciones de los miembros» de la página «Registro» de la configuración del servidor (`/admin/settings/registration`):

| Ajuste | Significado | Predeterminado | Rango |
| --- | --- | --- | --- |
| `member_invite_max_active` | Cuántas invitaciones válidas (no revocadas, caducadas ni agotadas) puede tener un miembro a la vez | `5` | 0-100 (0: los miembros no pueden crear invitaciones) |
| `member_invite_max_uses` | Cuántas personas pueden registrarse con una invitación | `10` | 1-1000 |
| `member_invite_max_lifetime_seconds` | Cuánto tiempo puede funcionar una invitación, en segundos | `2592000` (30 días) | 3600 (1 hora) a 31 536 000 (365 días) |

- Un miembro elige los usos y la duración dentro de los límites; si no elige, una invitación permite 1 registro y funciona durante 7 días (o menos, si un límite es más bajo). No se puede crear una invitación sin límite de usos o de tiempo.
- Bajar un límite no cambia las invitaciones existentes; se aplica a la siguiente invitación que se cree.
- Cada registro con la invitación de un miembro comprueba que su creador todavía puede crear invitaciones. Si el creador perdió `create_invites` (por ejemplo, porque terminó su plan), `invite_creators` volvió a `permitted`, o la cuenta se suspendió, se limitó o se eliminó, la invitación no funciona y la página de registro dice que no es válida.
  La invitación en sí no cambia, así que vuelve a funcionar, dentro de su caducidad, cuando el creador puede volver a invitar.
- Crear y revocar una invitación de un miembro se registra en el registro de auditoría como `invite_created` e `invite_revoked`, con el miembro como autor.

En el modo `approval`, una invitación válida aprueba de inmediato al usuario nuevo: quien lo invitó sustituye la revisión de los moderadores.
Con `invite_creators = everyone`, todos los usuarios pueden por tanto dejar entrar a personas sin revisión, lo que debilita el modo de aprobación.
Si dependes de la revisión, mantén `permitted` y da `create_invites` solo a personas de confianza, o mantén bajos `member_invite_max_active` y `member_invite_max_uses`.

<!-- source: approving-and-rejecting e68676f67aab -->
## Aprobar y rechazar

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- El argumento es el nombre de usuario de una cuenta local (la `@` inicial puede omitirse).
- Solo se pueden decidir los registros pendientes de aprobación. Con un registro ya decidido, o un usuario creado sin la página de registro (`my admin create`), el comando no cambia nada y termina con el código 1.
- El código de salida 2 indica un argumento incorrecto.

Las mismas decisiones pueden tomarse desde las pantallas de administración (la API de clientes). Todas necesitan el permiso `manage_accounts`.

| Operación | Qué hace |
| --- | --- |
| `GET /api/v1/admin/registrations` | Registros pendientes de aprobación (el que más espera, primero). Devuelve el ID del registro, el motivo para unirse, la dirección de correo electrónico y si está confirmada, y la cuenta local |
| `POST /api/v1/admin/registrations/{id}/approve` | Aprueba. El cuerpo `{"note": "..."}` es opcional. La respuesta muestra la nota como `decision_note`, y la respuesta sobre un registro ya decidido no incluye la dirección de correo electrónico |
| `POST /api/v1/admin/registrations/{id}/reject` | Rechaza. El cuerpo es el mismo que el de `approve` |

- Es el mismo caso de uso que la CLI, y registra la decisión en `registration_decisions` y en el registro de auditoría (`registration_approved`, `registration_rejected`). El autor de una decisión por CLI es `cli`; el autor de una decisión por API es el moderador que decidió.
- Decidir un registro ya decidido no cambia nada y devuelve el estado actual (si dos personas deciden a la vez, solo se registra una decisión). Un usuario creado sin la página de registro da 404.

<!-- source: sign-up-blocks b70eb18cd52f -->
## Bloqueos de registro

Los bloqueos rechazan los registros de la página de registro, o los pasan a aprobación, según el dominio del correo electrónico, la dirección de correo electrónico o el rango de IP.

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- Un registro que coincide con un bloqueo con `-severity reject` (el valor predeterminado) se rechaza, y uno que coincide con `require_approval` empieza pendiente de aprobación sea cual sea el modo de registro. Si coinciden varios, se aplica el más estricto. La página de registro no muestra el motivo del rechazo.
- `email_hash` se compara con la dirección en minúsculas, sin la parte de la parte local que empieza en el primer `+`. Un rango de IP se compara con la IP de registro (el mismo valor que `registration_origins`) y nunca se aplica a un registro cuya IP se desconoce.
- Los bloqueos caducados y levantados no tienen efecto. Añadirlos y levantarlos se registra en el registro de auditoría (`signup_block_added`, `signup_block_lifted`).
- Cuando expulsas una cuenta local con `-block-email` (`block_email: true` en la API), el servidor también crea un bloqueo `email_hash` para la dirección de correo electrónico de ese usuario.

Para una lista de miles de entradas, como los dominios de los servicios de correo desechable, añádelas de una vez con `import` desde un archivo (`-` para la entrada estándar). Todos los valores reciben el mismo tipo, gravedad, comentario y duración.

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- La lista tiene un valor por línea (la misma forma que `<value>` de `add`), o filas CSV cuya primera columna es el valor. Se omiten las líneas en blanco y las que empiezan por `#`. Una importación admite hasta 10 000 entradas.
- El comando muestra cuántas se añadieron y, de cada fila que no añadió, el número de línea, el valor y el motivo. El motivo es `invalid` (no es un valor válido de ese tipo), `duplicate` (la misma forma normal que una fila anterior de la misma lista) o `already_blocked` (hay en vigor un bloqueo del mismo tipo y valor igual o más estricto).
- Cada bloqueo añadido se registra en el registro de auditoría (`signup_block_added`) como con `add`, y se puede levantar uno a uno.
- La importación escribe en transacciones de unos cientos, así que si falla a medias, lo añadido antes se conserva. Importar la misma lista otra vez añade solo el resto.
- El software no incluye ninguna lista ni descarga una desde una URL periódicamente. Tú descargas y actualizas las listas públicas (como disposable-email-domains en GitHub) y vuelves a importar la lista actualizada.

Las mismas operaciones están disponibles en las pantallas de administración (la API de clientes). Todas necesitan el permiso `manage_accounts`.
En el cliente web, abre Moderación → Bloqueos de registro (`/moderation/signup-blocks`). La lista muestra el valor, el tipo, el estado y el final de cada bloqueo, en vigor o todos;
«Añadir un bloqueo» abre el formulario (`/moderation/signup-blocks/new`), «Añadir desde una lista» importa una lista y muestra las líneas que omitió (`/moderation/signup-blocks/import`),
y al elegir un valor se abre la página del bloqueo con su efecto y su comentario, donde «Levantar» lo termina.

| Operación | Qué hace |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | Lista los bloqueos (los más recientes primero). Filtra con `kind` y `active` |
| `POST /api/v1/admin/signup_blocks` | Añade un bloqueo. El cuerpo tiene `kind`, `value`, `severity`, `comment` y `expires_in` (segundos, como máximo diez años) |
| `POST /api/v1/admin/signup_blocks/import` | Añade bloqueos desde una lista. El cuerpo tiene `kind`, `text` (la lista), `severity`, `comment` y `expires_in`. La respuesta tiene `added`, el número añadido, y `skipped`, las filas no añadidas (`line`, `value`, `reason`) |
| `GET /api/v1/admin/signup_blocks/{id}` | Devuelve un bloqueo |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | Levanta un bloqueo. Un bloqueo ya levantado se queda como está |
