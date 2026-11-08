<!-- source: administrators-and-roles 1baeba6d67b4 -->
# Administradores y roles

Cómo crear el primer administrador, gestionar los roles desde la línea de comandos, anunciar y llevar a cabo el cierre del servicio, procesar de nuevo las imágenes subidas, reconstruir el índice de búsqueda y exportar la página de mantenimiento.

<!-- source: prerequisites b7ae62128809 -->
## Requisitos previos

- La configuración (`host`, `database`, `database_url`, `kek`, etc.) está definida como describe la [referencia de configuración](../reference/configuration.md). `my config check` la verifica.
- El esquema está aplicado en la base de datos (`my schema apply`; consulta [database.md](database.md)).
- `host` y `account_domain` son definitivos. El primer comando que usa la base de datos los registra, y todos los comandos posteriores se niegan a ejecutarse si la configuración difiere (consulta [Settings you cannot change later](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: creating-an-administrator e5fec2e3d287 -->
## Crear un administrador

```sh
my admin create -username alice -email alice@example.com
```

- Desde un terminal, el comando pide la contraseña dos veces, sin mostrarla en pantalla.
- Cuando la entrada estándar no es un terminal, la primera línea de la entrada estándar es la contraseña (por ejemplo, `my admin create ... < password.txt`).
  La contraseña no se puede indicar como argumento de la línea de comandos ni como variable de entorno, porque acabarían en las listas de procesos y en el historial del shell.
- Cuando un comando puntual no tiene ni terminal ni entrada estándar, como un [trabajo de Cloud Run](deploy-cloud-run.md#9-create-the-first-administrator), añade `-password-link`. Entonces el comando no lee ninguna contraseña: asigna una aleatoria que nadie llega a conocer e imprime un enlace después de la línea `created admin`, que permite definir la contraseña una sola vez, durante una hora, igual que `my admin account password-reset-link` (el registro de auditoría lo anota como `password_reset_link_issued`).
  Quien pueda leer la salida del comando puede usar el enlace, y en una plataforma así esa salida permanece en los registros de la plataforma hasta que el enlace se usa o caduca. Abre el enlace de inmediato. Si caduca, emite otro con `my admin account password-reset-link @alice`. Si falla la emisión del enlace, el administrador ya existe: el comando lo indica y termina con el código 1, y el mismo comando da un enlace nuevo.
- Una contraseña tiene de 8 a 128 caracteres. Nunca aparece en la salida, en los registros ni en un mensaje de error.
- Una sola transacción crea el usuario, la contraseña, la dirección de correo electrónico principal, la cuenta local (con sus claves) y la concesión del rol `admin` (sin quien lo concede, lo que significa la CLI).
  Los roles integrados se crean antes si faltan.
- Si todo va bien, el comando imprime `created admin @alice@<account_domain> (user <id>, account <id>)` y termina con el código 0.
- En un [servidor de una sola persona](instance-settings.md#one-person-server) el comando no crea ninguna cuenta y termina con el código 1.
  Mientras el [inicio de sesión esté limitado a los proveedores de acceso](login-providers.md#limit-sign-in-to-login-providers), además avisa de que la contraseña no puede iniciar sesión hasta que termine la limitación.

<!-- source: running-it-again 197692f7f103 -->
## Ejecutarlo de nuevo

- Cuando el nombre de usuario o la dirección de correo electrónico (ambos sin distinguir mayúsculas de minúsculas) ya están en uso, el comando no cambia nada, imprime cuál está en uso y termina con el código 1.
  Nunca cambia la contraseña ni los roles del usuario existente.
- Una ejecución fallida no escribe nada, así que puedes corregir la causa y ejecutar el mismo comando otra vez.
- El código de salida 2 indica un error de uso.

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## Conceder y revocar roles

Los roles contienen permisos. Los roles integrados son `admin` (`position` 1000, todos los permisos) y
`moderator` (`position` 100, con `manage_reports`, `manage_accounts` y `view_audit`).
`my serve` comprueba los roles integrados cada vez que arranca (en un proceso de cualquier `role`), y `my admin role` los comprueba cada vez que se ejecuta.
Cada uno crea lo que falta y da a `admin` todos los permisos de la versión en ejecución.
Cuando una actualización añade un permiso, no hace falta ningún paso manual. Un `moderator` conserva los permisos y la posición que fijó el administrador.

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- Los permisos pertenecen a uno de tres grupos.
  - Moderación: `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit`.
  - Administración del servidor: `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers`.
  - Funciones para miembros: `view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`.

  `list` imprime una columna por grupo (`MODERATION`, `SERVER`, `MEMBER`). El cliente web muestra «Moderación» y «Administración» para los dos primeros grupos y no cuenta las funciones para miembros en ninguno de los dos (consulta [moderation.md](moderation.md)).
- Un cambio surte efecto en la siguiente petición del usuario. Las notificaciones para moderadores van a las personas que tienen el permiso en el momento en que se crea la notificación.
- Las concesiones y las revocaciones se escriben en el registro de auditoría como `role_granted` y `role_revoked` (tipo de destino `role`). El actor de una operación de la CLI es `cli` (`my admin audit list -target-kind role`).
- Conceder un rol que el usuario ya tiene, o revocar uno que no tiene, no cambia ni registra nada.
- El último titular de `admin` no puede perderlo (código de salida 1). Concede antes `admin` a otro usuario.
- La API de administración es `GET /api/v1/admin/roles`, `GET /api/v1/admin/accounts/{id}/roles` y `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`, y requiere el permiso `manage_roles`.
  A través de la API, no puedes conceder ni revocar un rol por encima del `position` más alto entre tus propios roles (403). Revocar el último `admin` devuelve 409.
- El personal solo actúa sobre rangos inferiores. A través de la API, las acciones de moderación y su levantamiento, la resolución de apelaciones, el levantamiento de supresiones de correo electrónico, la revelación de los datos privados de una cuenta, la emisión de un enlace de restablecimiento de contraseña y la eliminación de la autenticación en dos pasos exigen que el `position` más alto entre tus roles sea superior al de cada rol del destinatario; un rango superior o igual, incluida tu propia cuenta, devuelve 403. Un moderador no puede actuar sobre un administrador, y un administrador no puede actuar sobre otro. Estos comandos (`my admin account ...`) no tienen ese límite, así que úsalos cuando los administradores discrepen.
- No se puede bloquear el acceso ni eliminar al único administrador: `disable_login`, `suspend` y `ban` sobre él se rechazan, tanto a través de la API (409) como con estos comandos (código de salida 1), y tampoco puede eliminar su propia cuenta (409). Otro titular de `admin` solo cuenta mientras pueda iniciar sesión: no después de eliminar su cuenta, ni mientras esté suspendido, vetado o con `disable_login`.

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## Anunciar el cierre del servicio

Para cerrar el servidor, anuncia primero el cierre. [service-closure.md](service-closure.md) describe todo el procedimiento, desde el anuncio hasta la eliminación de los datos.
El servidor sigue funcionando con normalidad durante el anuncio. `closure` en `GET /api/v1/instance` y `metadata.serviceClosure` en NodeInfo muestran el estado y las fechas.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` no tiene límite inferior. `-export-until` (el plazo para sacar los datos) debe ser igual o posterior a `-closes-at`, y por defecto es 30 días después de la fecha de cierre.
- Cuando faltan menos de 30 días para la fecha de cierre, el comando imprime un aviso en la salida de error estándar (el anuncio se registra igualmente).
- Cambiar las fechas se registra como un anuncio nuevo, y un anuncio hecho después de una retirada también es nuevo. Una vez cerrado el servidor, no se puede anunciar ni retirar (código de salida 1).
- Anunciar, cambiar las fechas y retirar se escriben en el registro de auditoría como `service_closure_announced` y `service_closure_withdrawn` (tipo de destino `instance`; consulta `my admin audit list -target-kind instance`).
- La API de administración es `GET /api/v1/admin/closure`, `PUT` / `DELETE /api/v1/admin/closure/announcement`, `GET /api/v1/admin/closure/check` y `POST /api/v1/admin/closure/close`. Requiere el permiso `manage_service_closure` (entre los roles integrados, solo `admin` lo tiene).
- Durante el anuncio, el servidor no acepta usuarios nuevos ni pagos nuevos.
  - El modo de registro efectivo es `closed` (el valor de `registration_mode` no cambia y vuelve a tener efecto cuando se retira el anuncio). No funcionan ni los registros ni los registros con invitación, y las solicitudes pendientes no se pueden aprobar (`409`).
  - Al anunciar se rechazan todas las solicitudes pendientes (se registran como decididas por `service_closure`) y se cancelan, al final de su periodo, todas las suscripciones de mecenas en curso.
    Los suscriptores reciben una notificación (`supporter_service_closure`) y un correo electrónico. Las suscripciones nuevas devuelven `409`. Retirar el anuncio no deshace los rechazos ni las cancelaciones.
  - Cada anuncio, cambio de fechas y retirada envía la notificación `service_closure` a todos los usuarios locales. Todo esto se ejecuta como trabajos, así que no detengas el worker.
  - Cada usuario con una dirección de correo electrónico confirmada recibe un correo electrónico por el anuncio, un cambio de fechas, una retirada, 7 días y 1 día antes de la fecha de cierre, el cierre y 7 días antes del plazo de exportación. Los usuarios no pueden desactivarlo (consulta [email.md](email.md)).
  - Tras la fecha de cierre, las personas que tienen `manage_service_closure` reciben la notificación `admin_service_closure_due`. El servidor no se cierra solo.

Después de la fecha de cierre, cierra el servidor. El cierre no se puede deshacer.

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` y `close` muestran el número de cuentas locales, el número de buzones de destino (los buzones compartidos conocidos), una estimación de las actividades `Delete` que se enviarán (su producto), el número de suscripciones de mecenas en curso,
  el número de migraciones sin terminar (`Move`) y el número de cuentas bajo retención (`retention_holds`).
  Cuando quedan migraciones o existe una retención, el comando avisa en la salida de error estándar. Exporta los registros de las cuentas bajo retención con `my admin account export-moderation` y guárdalos antes de eliminar los datos.
- `close` se niega sin preguntar cuando no hay anuncio y antes de la fecha de cierre (código de salida 1). Cualquier respuesta distinta de `y`, o una entrada que difiera del dominio del servidor (`host` en la configuración, sin distinguir mayúsculas de minúsculas), no cambia nada y termina con el código 1.
- El cierre se escribe en el registro de auditoría como `service_closure_closed`. En la misma transacción termina todas las suscripciones de mecenas en curso (motivo `service_closed`) y encola los trabajos que cancelan de inmediato las suscripciones en el proveedor de pagos (`CancelNow`).
- Tras el cierre, la API de cliente solo admite estas peticiones: leer tu propia cuenta y configuración, las exportaciones (`/exports`, `/csv_exports`), las sesiones (`/sessions`, `/authorized_apps`), `GET /instance` y la administración del cierre y de la cola de trabajos.
  Todo lo demás se rechaza con `403` (`/api/v1/problems/service-closed`). Los flujos se detienen y Web Push no envía nada. Iniciar sesión y restablecer la contraseña siguen funcionando.
- Tras el cierre, el worker entrega un `Delete` del actor de cada cuenta local a los pares de la federación, de 100 en 100 (las cuentas migradas, al final). Los envía incluso cuando `federation_mode` es `off` o `inbound`, y nunca a un dominio suspendido.
  `my admin closure status` (y `progress` en `GET /api/v1/admin/closure`) muestra cuántas cuentas tienen un `Delete` registrado, cuántas entregas quedan por expandir y el número de entregas en cada estado (`pending`, `succeeded`, `permanent_failure`, `skipped`, `dead`). Mantén el worker en ejecución durante la entrega.
  Cuando todas las entregas han terminado, una comprobación que se ejecuta cada 10 minutos registra `delivery_completed` (`service_closure_delivery_completed` en el registro de auditoría), y los actores locales pasan a responder `410`.
  El servidor no vuelve a ejecutar por sí mismo las entregas `dead`; vuelve a ejecutarlas con `my job retry`.

Una vez pasado el plazo de exportación, escribe la página estática de cierre que tu dominio sirve cuando el servidor está detenido. El comando lee la base de datos, así que ejecútalo antes de eliminarla.

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- Salida: `<dir>/<locale>.html` para cada idioma y `<dir>/index.html` con todos los idiomas uno tras otro. Las páginas usan la misma plantilla que `my maintenance export` y muestran el nombre del servidor, la fecha de cierre, el mensaje del operador del anuncio y `status_page_url`.
  La salida incluye además ejemplos de configuración para el front end: `nginx.conf`, `Caddyfile` y `cloudfront-function.js`. Responden `410` para `/ap/*`, `/inbox`, WebFinger, host-meta, NodeInfo y `/@*`, y sirven la página de cierre para todo lo demás.
- El comando se niega (código de salida 1) a menos que el servidor esté cerrado. Antes de `delivery_completed` imprime un aviso y escribe las páginas igualmente. [service-closure.md](service-closure.md) describe cómo cambiar el front end a la página estática.

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## Procesar de nuevo las imágenes subidas

Cada imagen subida conserva tres tamaños, y una animación también sus versiones en movimiento. Una nueva versión de `my` puede cambiar esos tamaños o formatos, y una imagen existente conserva sus archivos hasta que la procesas de nuevo:

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` toma la hora del cambio, como la hora a la que actualizaste, en RFC 3339. Solo se toman las imágenes procesadas por última vez antes de esa hora, y una hora que aún no ha pasado se rechaza.
- El worker hace el trabajo, 100 imágenes por minuto, cada una en un trabajo propio (`media.reprocess_walk` y `media.reprocess` en la [referencia de trabajos](../reference/jobs.md)). Mantén un worker en ejecución; la página de trabajos de administración (`/admin/jobs`) muestra la cola.
- Ejecutarlo de nuevo con la misma hora es seguro. Las imágenes ya procesadas se dejan como están, y se recogen las que la primera ejecución no alcanzó.
- Los originales no se conservan, así que cada imagen se vuelve a generar a partir de su versión grande. El detalle que la versión grande perdió no se recupera, y cada ejecución con una hora posterior vuelve a codificar las imágenes y pierde un poco de calidad. Ejecútalo una sola vez por cambio.
- Los archivos de video y de audio no se vuelven a codificar y se dejan como están. Una imagen que los límites actuales ya no aceptan conserva su versión actual, y el worker registra un aviso.

La nueva versión de una imagen tiene URL nuevas. Los archivos antiguos se conservan durante `media_replaced_retention` (30 días por defecto, [referencia de configuración](../reference/configuration.md#media_replaced_retention)), de modo que las páginas abiertas y las cachés siguen funcionando, y después el trabajo horario `media.purge_replaced` los elimina. No se avisa a otros servidores de las URL nuevas: un servidor que aún conserva una URL antigua y no tiene copia propia deja de mostrar la imagen pasado ese tiempo. Cuenta con el almacenamiento adicional hasta que desaparezcan los archivos antiguos.

<!-- source: rebuilding-the-search-index 9061821656ac -->
## Reconstruir el índice de búsqueda

`my search` gestiona el índice de la búsqueda de texto completo de las publicaciones, en un servidor con un motor de búsqueda. Con `search = none`, cada comando lo indica, no hace nada y termina con el código 0.

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

Ejecuta `my search rebuild` de nuevo para reanudar una reconstrucción interrumpida. Continúa desde la última publicación procesada y no vuelve a vaciar el índice. Mientras haya una sin terminar, el otro modo se rechaza con el código de salida 1. [Full-text search of posts](search.md) indica cuándo usar cada uno.

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## Exportar la página de mantenimiento

`my maintenance export` escribe la página del modo de mantenimiento como HTML estático,
para que la sirvan un balanceador de carga, una CDN o un proxy inverso mientras `my serve` no puede ejecutarse (una imagen que se está sustituyendo, una revisión que no arranca).
Usa la misma plantilla que el modo de mantenimiento, así que ambos muestran las mismas palabras y el mismo aspecto.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Salida: `<dir>/<locale>.html` para cada idioma de `-lang`, para los front ends que eligen una página según `Accept-Language`,
  y `<dir>/index.html` con todos los idiomas uno tras otro, para los que no pueden.
  Las páginas llevan CSS en línea, ningún script y ningún recurso externo; su Content-Security-Policy está en un elemento `<meta>`.
- Textos: la configuración `maintenance_server_name`, `maintenance_message`, `maintenance_until` y `status_page_url`
  ([referencia de configuración](../reference/configuration.md)).
- Sin `-offline`, también lee el nombre de la instancia (cuando `maintenance_server_name` no está definido), su idioma por defecto (cuando no se indica `-lang`)
  y las sustituciones de tokens de su tema. El CSS personalizado del tema nunca se incluye. Cuando no se puede leer la base de datos,
  imprime un aviso, escribe las páginas a partir de la configuración y el tema por defecto, y termina con 0.
- Ejecútalo de nuevo cada vez que cambien esa configuración o el tema, y antes de cada mantenimiento, para que las páginas del front end estén al día.
