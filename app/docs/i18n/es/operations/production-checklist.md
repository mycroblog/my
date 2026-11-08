<!-- source: production-checklist e5f6c0c5b10a -->
# Lista de comprobación para producción

Repasa esta lista antes de abrir el servidor a los usuarios y a otros servidores, y de nuevo después de cualquier cambio en el despliegue.
Algunas opciones de configuración no se pueden cambiar una vez que el servidor ha federado, y algunos errores, como perder la clave de cifrado de claves,
no se pueden deshacer en absoluto.

Cada punto indica por qué importa, cómo verificarlo y dónde están los detalles.
Empieza por la parte automatizada:

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## Qué comprueba `my config check`

En el perfil `production` (el valor por defecto de [`profile`](../reference/configuration.md#profile)), `my config check`
primero valida toda la configuración y después comprueba que la configuración válida es adecuada para una instancia de producción.
Los problemas se enumeran en dos grupos, bajo un resumen en el idioma de la configuración regional del proceso, cada uno con el detalle técnico
en inglés:

- Los **errores** hacen que `my config check` termine con código 1. Son valores de configuración que rompen una instancia de producción o debilitan su seguridad:
  - Un `host` o `account_domain` al que otros servidores no pueden llegar: con un puerto, `localhost`, un nombre sin punto, o un nombre bajo
    un dominio de primer nivel reservado para uso local o pruebas (`.localhost`, `.local`, `.test`, `.example`, `.invalid`, `.internal`, `.home.arpa`).
  - Un `web_origin` sobre `http`.
  - Un `database_url` de SQLite que mantiene la base de datos en memoria.
  - Un `kek` que no es aleatorio, como todo ceros o un byte repetido.
  - Una clave de sitio o una clave secreta de `captcha` que es una de las claves de prueba que publican los servicios de CAPTCHA.
  - `worker_job_types`, `worker_job_types_except` o una ruta de cola que no nombra ningún tipo de trabajo (en todos los perfiles; `my job types` los enumera).
  - Con `-instance`, un `worker_job_concurrency` que el pool de la base de datos no puede atender.
- Las **advertencias** dejan el código de salida en 0. Son valores de configuración habituales en desarrollo que pueden ser deliberados en producción:
  - `mail = none`.
  - Una base de datos SQLite o el almacén de blobs `fs` en un directorio temporal (`/tmp`, `/var/tmp`, `/dev/shm` o `$TMPDIR`).
  - `queue = inline`.
  - Falta de `vapid_private_key`.
  - `stream = hub` o `kv = memory` con un `role` distinto de `all`: solo funcionan dentro de un único proceso.
  - Ajustes de voz que probablemente no eran los deseados, como `voice = p2p` sin STUN ni TURN.
  - Un [plugin](plugins.md) en una dirección `http://`, o en una dirección `https://` sin token ni certificado de cliente.
  - Una clave en modo de prueba del proveedor de pagos.
  - `origin_secret` establecido con un `trusted_proxies` vacío, y `origin_secret_previous` que sigue establecido tras una rotación.
  - Con `-instance`: los textos de instancia `name`, `short_description`, `terms` y `privacy_policy`, la configuración de instancia
    `contact_account`, `contact_email` y `security_contact` cuando no están establecidos, y un servidor sin reglas.
    Cuando no se puede acceder a la base de datos, eso es una advertencia más y el resto de la comprobación se sigue ejecutando.

`my serve` registra los mismos hallazgos como advertencias al arrancar, pero arranca de todos modos.
El perfil `development` omite estas comprobaciones; úsalo para el desarrollo local y las pruebas, nunca para una instancia que usen otras personas.

Ejecuta `my config check` en el mismo entorno que el servidor (las mismas variables de entorno, archivos de secretos y archivo de configuración),
por ejemplo como un paso del despliegue antes de que la nueva versión reciba tráfico.

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## Ajustes que no puedes cambiar después

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### El dominio del servidor y el dominio de los identificadores

- **Por qué**: `host` forma parte de cada id de ActivityPub del servidor, y otros servidores guardan esos ids.
  `account_domain` es el dominio de los identificadores (`@alice@example.org`). El primer `my serve` o comando `my` que usa la
  base de datos registra ambos en ella, y a partir de entonces `my serve` y esos comandos se niegan a ejecutarse cuando la configuración difiere:
  un cambio después de federar rompe todos los seguimientos y todas las referencias a las publicaciones y cuentas del servidor.
  Establece ambos antes del primer arranque. Un error detectado antes de federar se corrige
  [recreando la base de datos](database.md#recreating-erasing-every-row), lo que borra todas las filas.
- **Decide**: si los identificadores usan el dominio del servidor o uno aparte (por ejemplo, identificadores en `example.org`
  y el servidor en `social.example.org`). Un dominio de identificadores aparte debe redirigir su WebFinger, host-meta y NodeInfo
  al servidor.
- **Verifica**: `my config check` muestra `host` y `account_domain` e informa de un dominio al que otros servidores no pueden llegar.
  `my config check -instance` también falla cuando difieren de los valores de la base de datos, y no registra nada.
  Con un dominio de identificadores aparte, `curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"`
  responde `301` a `https://<host>/.well-known/webfinger?...`.
- **Detalles**: [ejecutar el servidor](running.md) (los endpoints de federación y cómo servir los identificadores desde otro dominio),
  [`host`](../reference/configuration.md#host) y [`account_domain`](../reference/configuration.md#account_domain).

<!-- source: reachable-over-https 1cf53c613f7b -->
### Accesible por `https`

- **Por qué**: otros servidores obtienen los ids en `https://<host>/` por el puerto 443, y los navegadores envían las cookies de inicio de sesión solo por `https`.
- **Verifica**: `curl -sI https://<host>/healthz` responde `200` desde fuera de tu red con un certificado válido para `host`,
  y `my config check` no informa de ningún error sobre `web_origin`. `http://<host>/` redirige a `https`, algo que hace el proxy inverso,
  balanceador de carga o CDN delante del servidor (con Docker Compose, la capa `compose.caddy.yaml` es un proxy así).
- **Detalles**: [ejecutar el servidor](running.md), [`web_origin`](../reference/configuration.md#web_origin).

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### Inaccesible esquivando el CDN

- **Por qué**: con un CDN delante y la dirección propia del servidor todavía pública (`*.run.app` en Cloud Run), cualquiera puede llamar al servidor esquivando el WAF y la caché,
  y quien llame desde la propia plataforma del CDN puede elegir la IP de cliente que ven los límites de frecuencia. Solo hace falta en ese caso: no cuando un cortafuegos o la plataforma ya limitan quién puede conectarse,
  y sin un CDN no hay nada que esquivar.
- **Verifica**: `origin_secret` y `origin_secret_header` están establecidos y el CDN establece la cabecera en todos los nombres de host. `curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/`
  responde `403`, su `/healthz` responde `200` y `https://<host>/` responde `200`.
- **Detalles**: [Rechazar solicitudes que no han pasado por el CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), incluido cómo recuperarse cuando deja a todos fuera.

<!-- source: secrets ec71f1f20574 -->
## Secretos

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### La clave de cifrado de claves (KEK)

- **Por qué**: la KEK cifra las claves privadas de todos los actores. Sin ella el servidor no puede firmar nada, y las claves no se pueden
  recuperar de la base de datos: habría que recrear todas las cuentas y otros servidores dejarían de confiar en él.
- **Verifica**:
  - Se generó de forma aleatoria (`openssl rand -base64 32`); `my config check` informa de una clave que no es aleatoria.
  - Se guarda en un lugar distinto de la base de datos y sus copias de seguridad, por ejemplo en un gestor de contraseñas o en el
    almacén de secretos de la nube, y al menos otra persona que gestione el servidor puede acceder a ella.
  - Llega al servidor solo mediante `MY_KEK` o `MY_KEK_FILE` (el archivo de configuración rechaza los secretos). Con Docker Compose, es
    el archivo `secrets/kek` y no está en `.env` ([Archivos de Docker Compose](running.md#docker-compose-files)).
- **Detalles**: [gestión de claves](key-management.md).

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### La clave VAPID de Web Push

- **Por qué**: sin ella el servidor no envía notificaciones Web Push. Cambiarla más adelante termina todas las suscripciones, así que genérala una sola vez
  y consérvala como la KEK.
- **Verifica**: `my config check` avisa cuando `vapid_private_key` no está establecida. Genera una con `my config vapid-key`.
- **Detalles**: [gestión de claves](key-management.md) (la clave VAPID de Web Push).

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### Los demás secretos quedan fuera del repositorio y de la imagen

- **Por qué**: un secreto en un repositorio, una imagen de contenedor o un archivo de configuración se copia a todos los lugares a los que van esos elementos.
- **Verifica**: `database_url`, `mail_smtp_password`, `captcha_secret_key`, las claves del proveedor de pagos, `valkey_url` y cualquier otro
  ajuste marcado como secreto en la referencia de configuración proceden de variables de entorno o de archivos `<NAME>_FILE` que rellena el
  almacén de secretos de la plataforma (secretos de Docker o Kubernetes, Secret Manager, Parameter Store). El archivo de configuración no puede contenerlos:
  `my config check` rechaza un secreto en él. Con Docker Compose, proceden de `.env`, que solo tú puedes leer (`chmod 600`). Busca los valores en el repositorio y en la imagen para asegurarte de que no queda ninguna copia.
  `my config check` muestra `(redacted)` para cada secreto establecido.
- **Detalles**: [referencia de configuración](../reference/configuration.md) (la columna Secret), [CAPTCHA](captcha.md),
  [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md).

<!-- source: instance-settings baae9cc3e09e -->
## Ajustes de instancia

La configuración de instancia está en la base de datos y se pueden cambiar mientras el servidor funciona, en las pantallas de administración del servidor
o con `my instance set`. `my config check -instance` avisa de los textos, contactos y reglas que no están establecidos.

- **Nombre, descripción corta y descripción**: se muestran en la página de inicio, la página de información (`/about`), las vistas previas de enlaces
  y NodeInfo. Verifícalo en `/about`.
- **Contactos**: `contact_account` (la cuenta con la que los usuarios contactan sobre el servidor) y `contact_email` (la dirección a la que escriben los usuarios
  y otros servidores; los correos del servidor también dirigen a ella las respuestas). Verifícalo en `/about`.
- **Contacto para vulnerabilidades**: `security_contact`, un URI `https:` o `mailto:`. Si está establecido, el servidor responde en
  `/.well-known/security.txt`. Verifícalo con `curl -s https://<host>/.well-known/security.txt`.
- **Términos del servicio y política de privacidad**: `terms` y `privacy_policy`. La política de privacidad debe nombrar los servicios que reciben
  datos personales, como el servicio de CAPTCHA y el proveedor de correo. Verifica los enlaces en `/about` y en la pantalla de registro.
- **Reglas del servidor**: lo que los usuarios aceptan al registrarse y lo que hacen cumplir los moderadores. Añádelas con `my instance rule create`
  o en las pantallas de administración, y verifícalas en `/about`.
- **Modo de registro**: `registration_mode` es `approval` hasta que lo cambies; elige `open`, `approval`, `invite` o `closed`
  según cómo se unan las personas. Con `mail = none`, los registros `open` también esperan la aprobación de un moderador.
- **Buscadores e IA**: `search_indexing`, `ai_usage` e `indexable_default` rechazan ambos hasta que los permitas.
  Verifica `/robots.txt` y `/.well-known/tdmrep.json`.
- **Página de estado**: establece [`status_page_url`](../reference/configuration.md#status_page_url) (un ajuste, no un ajuste de instancia,
  para que el enlace se muestre durante el mantenimiento) con una página de estado en un servicio distinto del servidor.

**Detalles**: [configuración de la instancia](instance-settings.md), [registro](registration.md),
[buscadores y rastreadores de IA](crawlers.md), [CAPTCHA](captcha.md) (qué escribir en la política de privacidad),
[plantilla de política de privacidad](privacy-policy-template.md), [plantilla del registro de actividades de tratamiento](processing-record-template.md),
[monitorización](monitoring.md) (la página de estado).

<!-- source: email 40c18999c46f -->
## Correo electrónico

- **Por qué**: el correo electrónico verifica direcciones y envía restablecimientos de contraseña y notificaciones. Con `mail = none` ningún correo llega a los usuarios.
- **Verifica**:
  - `mail` es `smtp`, `ses` o `plugin`, y `mail_from` es una dirección de un dominio que controlas; `my config check` avisa de `mail = none`.
  - El dominio de `mail_from` tiene registros SPF, DKIM y DMARC, y el proveedor firma con DKIM. Sin ellos, el correo acaba en spam.
  - Regístrate con una cuenta de prueba y recibe el correo de verificación; solicita un restablecimiento de contraseña y recibe también ese correo.
    Comprueba en las cabeceras del mensaje recibido que aparecen `spf=pass`, `dkim=pass` y `dmarc=pass`.
  - Con SES, la cuenta ha salido del sandbox y las notificaciones de rebotes y quejas llegan al servidor.
- **Detalles**: [correo electrónico](email.md), [registro](registration.md) (verificación del correo electrónico).

<!-- source: administrators 2c05e6644d88 -->
## Administradores

- **Por qué**: la cuenta de un administrador puede cambiar toda la configuración, leer datos privados y suspender a cualquiera.
- **Verifica**:
  - Todos los administradores activaron la autenticación en dos pasos (Configuración, Autenticación en dos pasos en el cliente web)
    y conservan los códigos de recuperación.
  - Solo las personas que lo necesitan tienen el rol `admin`; los moderadores tienen `moderator` o un rol con solo los permisos que usan.
    Enuméralos con `my admin role` y revísalos en las pantallas de administración.
- **Detalles**: [administradores y roles](admin-cli.md), [moderación](moderation.md) (permisos y recuperación de inicio de sesión).

<!-- source: operations 56bd643c2241 -->
## Operaciones

- **Copias de seguridad y recuperación**: la base de datos, el almacén de blobs (`blob_fs_dir` o el bucket) y la KEK tienen copias de seguridad programadas,
  en un lugar distinto del servidor, y se ha ensayado una restauración en un servidor nuevo: el servidor restaurado arranca y las cuentas
  pueden iniciar sesión y publicar. Una copia de seguridad que nunca se ha restaurado no se sabe si funciona. La base de datos y los archivos multimedia no deben estar en un
  directorio temporal; `my config check` avisa cuando lo están.
  Consulta [gestión de claves](key-management.md) (copia de seguridad de la KEK) y [base de datos](database.md).
- **Monitorización y alertas**: un monitor externo comprueba `/healthz` y avisa a alguien que pueda actuar, y se vigilan los trabajos fallidos (dead) de la cola de trabajos.
  Consulta [monitorización](monitoring.md) y [ejecutar el servidor](running.md) (comprobaciones de salud, métricas y la cola de trabajos).
- **La imagen**: la instancia ejecuta una imagen publicada fijada por digest (`ghcr.io/mycroblog/my:0.1.0@sha256:...`, de las notas de la versión), no una
  que cambia al reiniciar, y [se ha verificado su procedencia](running.md#verify-an-image). No existe la etiqueta `latest`.
  Una actualización cambia esa versión a propósito y sigue [Actualización](database.md#upgrading), una versión menor cada vez.
- **Código fuente (AGPL-3.0)**: el servidor debe ofrecer su código fuente a todas las personas que lo usan. Deja
  [`source_url`](../reference/configuration.md#source_url) en su valor por defecto para una versión sin modificar; apúntalo a tu propio
  código fuente cuando ejecutes un fork o una versión modificada. Verifica el enlace en `/about`.
  Consulta [ejecutar el servidor](running.md) (publicación del código fuente).
- **Mantenimiento**: aprende a poner el servidor en modo de mantenimiento antes de necesitarlo. Consulta [mantenimiento](maintenance.md).

<!-- source: federation a5ab186ec04b -->
## Federación

- **Por qué**: otros servidores encuentran las cuentas mediante WebFinger y NodeInfo y después obtienen el actor. Si cualquiera de estos pasos falla desde fuera,
  el servidor es invisible para el fediverso.
- **Verifica** desde una máquina fuera de tu red:
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` devuelve la cuenta,
    siguiendo la redirección a `host` cuando los dominios difieren (`curl -L`).
  - `curl -s https://<host>/.well-known/nodeinfo` enlaza `/nodeinfo/2.1`, que nombra el software y su código fuente.
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` redirige al id del actor,
    y el actor en ese id es JSON con una `publicKey`.
  - Desde una cuenta de Mastodon u otro servidor, busca `@<user>@<account_domain>`, síguela y comprueba que llega una publicación nueva.
  - Un CDN delante del servidor no guarda en caché `/ap/*` ni `/@*` a menos que respete `Vary: Accept`.
- **Detalles**: [ejecutar el servidor](running.md) (los endpoints de federación), [CDN](cdn.md), [relés](relays.md).
