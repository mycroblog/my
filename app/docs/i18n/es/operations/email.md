<!-- source: sending-email 941ecea58cd9 -->
# Envío de correo electrónico

Qué correos electrónicos envía el servidor, cómo configurar el envío y cómo tratar los fallos y la baja de las suscripciones.
Las configuraciones están en la [referencia de configuración](../reference/configuration.md#mail).
Los flujos de cara al usuario de la confirmación del correo electrónico y del restablecimiento de contraseña están en [registro y confirmación del correo electrónico](registration.md).

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## Qué se envía y cómo configurarlo

El servidor nunca envía correo electrónico dentro de una solicitud. Encola un trabajo para cada finalidad en la misma transacción que el hecho
que desencadena el correo, y un worker de trabajos lo envía. Un fallo al enviar no se ve en la respuesta a un registro ni a un restablecimiento:
al usuario se le dice «hemos enviado un correo de confirmación», y lo pide de nuevo si no llega.

| Clase | Correo electrónico | Tipo de trabajo | ¿Puede el usuario darse de baja? |
| --- | --- | --- | --- |
| Transaccional (`transactional`) | Confirmación de la dirección de correo electrónico | `identity.verification_email` | No |
| Transaccional | Restablecimiento de contraseña | `identity.password_reset_email` | No |
| Transaccional | Aviso de un cambio de contraseña o de una eliminación | `identity.password_notice_email` | No |
| Transaccional | Aviso de la desactivación de la cuenta, con la fecha de su eliminación | `identity.deactivation_email` | No |
| Transaccional | Cierre del servicio (el anuncio, un cambio de fecha, una cancelación, 7 días y 1 día antes de la fecha de cierre, el cierre y 7 días antes del plazo de exportación) | `identity.closure_email` | No |
| Notificación (`notification`) | Aviso a moderadores (activado por defecto, por tipo de aviso) | `notification.moderator_email` | Sí |
| Notificación (`notification`) | Una notificación que permaneció sin leer durante 15 minutos: menciones, seguimientos, impulsos y las demás notificaciones de otros usuarios, y avisos de los moderadores (desactivado por defecto, por tipo) | `notification.email` | Sí |

Los correos de notificación para mecenas (`notification.supporter_email`) pasan por la misma cola de trabajos.

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### Qué notificaciones se envían por correo electrónico

Cada usuario elige, por tipo de notificación, si llega a la aplicación y por correo electrónico, en la página **Notificaciones** de
la configuración (`/settings/notifications` en el cliente web). La columna de correo electrónico aparece allí solo mientras el servidor envía
correo electrónico. Un tipo desactivado en la aplicación no se crea en absoluto, así que tampoco se envía por correo ni como notificación push.

| Tipo | Correo electrónico por defecto | Cuándo se envía |
| --- | --- | --- |
| Notificaciones de otros usuarios (menciones, nuevos seguidores, solicitudes de seguimiento, solicitudes de seguimiento aceptadas, impulsos, favoritos, reacciones con emoji, citas, encuestas terminadas, cuentas que sigues que se mudan, chats de voz) | Desactivado | 15 minutos después de la notificación, solo si sigue sin leer (no marcada como leída, no descartada) y sigue siendo visible, y el usuario sigue recibiendo ese tipo por correo electrónico. Un correo por notificación |
| Avisos de los moderadores (una acción de moderación sobre la cuenta, una denuncia resuelta, una apelación resuelta, seguimientos perdidos por una suspensión, una solicitud de emoji personalizado resuelta) | Desactivado | Igual que arriba |
| Avisos a moderadores (nuevas denuncias, registros pendientes de aprobación, apelaciones, webhooks desactivados, solicitudes de emoji personalizado, cuentas limitadas que se mudan, el paso de la fecha de cierre, errores nuevos y recurrentes), a las personas cuyo rol tiene el permiso | Activado | De inmediato, sin esperar a que se lea |
| Cierre del servicio y avisos de los planes de apoyo | Siempre | De inmediato; no se pueden desactivar |

Los correos de notificación van solo a la dirección principal del usuario una vez confirmada, y nunca a una dirección cuyos correos
estén detenidos (consulta más abajo). El asunto del correo de una notificación de otro usuario es el mismo texto que su mensaje de Web Push,
y el correo enlaza a la publicación, al perfil o a las notificaciones.

Los correos de cierre del servicio van a todos los usuarios cuya dirección principal está confirmada. Perderse uno puede significar perder datos, así que se envían
como correo transaccional, no llevan enlace para darse de baja y una queja no los detiene (un rebote definitivo sí).
El anuncio, un cambio de fecha, una cancelación y el cierre encolan cada uno `identity.closure_email_fanout` en la misma
transacción que su registro. El trabajo horario `identity.queue_closure_reminders` decide cuándo enviar los recordatorios de 7 días y
1 día antes de la fecha de cierre y de 7 días antes del plazo de exportación. Un recordatorio cuyo momento ya había pasado cuando se
anunció el cierre no se envía, porque el correo del anuncio indica las fechas.
El trabajo de difusión registra una fila de `service_closure_emails` por usuario y encola `identity.closure_email` solo para los usuarios que
pudo registrar, de modo que, por muchas veces que se ejecute el trabajo, cada usuario recibe cada correo una sola vez. Antes de enviar, el trabajo comprueba que el correo
sigue siendo válido (que no es de un anuncio cancelado ni de una fecha que ha cambiado desde entonces).

La configuración `mail` elige cómo se envía el correo electrónico.

| `mail` | Comportamiento |
| --- | --- |
| `none` (por defecto) | No envía nada y escribe `mail not sent (mail = none)` en el registro del servidor con la categoría del mensaje. Con `profile = development`, la línea contiene además las direcciones, el asunto y el cuerpo, enlaces incluidos. Para configuraciones que no usan correo electrónico y para desarrollo |
| `smtp` | Envía por SMTP submission. Requiere STARTTLS y verifica el certificado antes de autenticarse y enviar |
| `ses` | Envía con la API v2 de Amazon SES (`SendEmail` con contenido sin procesar). Se autentica con la cadena de credenciales por defecto del SDK de AWS (un rol de IAM, por ejemplo) y no tiene contraseña SMTP |
| `plugin` | Entrega cada mensaje a un [plugin](#a-plugin-mail--plugin) que tú ejecutas, para un servicio de correo que solo ofrece una API HTTP |

Las configuraciones de `smtp` son `mail_from`, `mail_smtp_host`, `mail_smtp_port` (por defecto 587), `mail_smtp_username` y `mail_smtp_password`. Los
configuraciones de `ses` son `mail_from`, `mail_ses_region` y `mail_ses_configuration_set`. Las configuraciones de `plugin` son `mail_from`
y `plugin_mailer_address` con el resto de [su destino](plugins.md#the-destination).
`mail_from` es una dirección sin nombre, como `noreply@social.example`. `From` pasa a ser `"<server name>" <mail_from>`, y el dominio
de `mail_from` es también el dominio de `Message-ID` y aquel al que se aplican SPF, DKIM y DMARC (consulta más abajo).
`Reply-To` es la configuración de la instancia `contact_email` cuando está definida ([configuración de la instancia](instance-settings.md)).
Todos los correos llevan `Auto-Submitted: auto-generated`.
`smtp` y `ses` construyen el mismo mensaje. No se usa SMTPUTF8, así que las direcciones se envían en ASCII y los dominios internacionalizados se
convierten a Punycode. No se puede enviar a una dirección cuya parte local (antes de la `@`) no sea ASCII, y su trabajo falla.

<!-- source: examples-by-provider 7ab129a02c43 -->
## Ejemplos por proveedor

Pasa los secretos por variable de entorno o por archivo (`MY_MAIL_SMTP_PASSWORD` o `MY_MAIL_SMTP_PASSWORD_FILE`). No los pongas en el
archivo de configuración.

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES (API)

Cuando ejecutas en AWS, `mail = ses` usa la API v2 de SES. Las credenciales vienen de la cadena por defecto del SDK de AWS (el rol de tarea de
ECS, las variables de entorno, el archivo de configuración compartido), así que no hay ningún secreto que configurar.

1. En SES, verifica el dominio de `mail_from` como identidad y activa Easy DKIM (consulta más abajo).
2. Solicita acceso de producción para salir del sandbox. En el sandbox solo puedes enviar a direcciones verificadas.
3. Permite `ses:SendEmail` al rol de ejecución sobre la identidad de `mail_from` (y sobre el conjunto de configuración, si usas uno).

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

Cuando `mail_ses_configuration_set` está definido, todos los correos usan ese conjunto de configuración. SES sobrescribe el `Message-ID` y el `Date` del
correo con sus propios valores.
SES nunca rechaza a un destinatario de forma síncrona en el momento del envío. Acepta un envío a una dirección de la lista de supresión de la cuenta y
luego no lo entrega, y el rebote llega más tarde como una notificación (SNS).
Para registrar rebotes y quejas, configura las notificaciones de SES de más abajo para que lleguen a través de SNS.

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### Un plugin (`mail = plugin`)

Para un servicio de correo que solo ofrece una API HTTP, ejecuta un [plugin](plugins.md) del tipo `mailer` y deja que llame al servicio.

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

`plugin_mailer_address` es obligatorio con este valor. Un mensaje puede contener un secreto, como un enlace de verificación, así que el tipo
rechaza `http://`, también en loopback: usa un socket Unix o `https://`. El servidor arranca sin llamar al plugin, incluso cuando
este está caído, y el correo espera en su trabajo.

Con `plugin_mailer_protocol = json`, el valor por defecto, un plugin solo necesita un servidor HTTP y una biblioteca JSON
([Un plugin sin biblioteca](plugins.md#a-plugin-with-no-library)). `Send` dura 30 segundos como máximo, o `plugin_mailer_timeout`,
por debajo de 5 minutos. Una solicitud es de 2 MiB como máximo y una respuesta de 64 KiB como máximo.

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

El servidor comprueba un mensaje antes de llamar al plugin, así que no se puede inyectar una cabecera a través de ninguno de estos campos. La solicitud
contiene las partes del mensaje, y el plugin construye lo que necesita su servicio:

- **El plugin añade tres cabeceras:** un `Message-ID` único por mensaje, en el dominio de `envelopeFrom`; un `Date`; y
  `Auto-Submitted: auto-generated`, que evita que las respuestas automáticas entren en bucle. Un servicio que escribe su propio `Message-ID` y
  `Date` cubre esas dos.
- `From` es `fromName` y `envelopeFrom`. `fromName`, `replyTo`, `html` y `headers` pueden faltar. `headers` contiene solo las
  dos cabeceras `List-Unsubscribe`.
- Las direcciones son UTF-8. Un plugin cuyo servicio necesita ASCII convierte el dominio a Punycode y responde `OUTCOME_INVALID` para una
  parte local que no puede enviar.
- El mismo mensaje puede llegar dos veces, ya que un trabajo que falla se reintenta. No escribas un mensaje en un registro.

`outcome` indica qué ocurrió con el mensaje:

| `outcome` | Significado | El servidor |
| --- | --- | --- |
| `OUTCOME_SENT` | El servicio aceptó el mensaje | Completa el trabajo |
| `OUTCOME_INVALID` | No se puede enviar tal como está | Hace fallar el trabajo de inmediato |
| `OUTCOME_RECIPIENT_REJECTED` | La dirección no existe | Hace fallar el trabajo de inmediato y [suprime la dirección](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | El servicio lo rechazó definitivamente, por otro motivo | Hace fallar el trabajo de inmediato |
| `OUTCOME_TEMPORARY` | El servicio lo rechazó por ahora, o no respondió | Reintenta el trabajo |
| Cualquier otro, o ninguno | | Reintenta el trabajo |

Un error HTTP, o el `{"code":"unavailable"}` de Connect con el estado correspondiente, es también un fallo temporal.
`enhancedStatus` es opcional: el código de estado mejorado de un rechazo, como `5.1.1`. El servidor conserva un código de clase 5 de un
rechazo y de clase 4 de `OUTCOME_TEMPORARY`, e ignora cualquier otro. No conserva ningún texto de un plugin, así que no pongas ahí ningún motivo:
un motivo pertenece al registro del propio plugin.

El servidor solo se entera de un rebote por esta respuesta. No tiene ningún endpoint para los rebotes y quejas posteriores del servicio de un plugin,
así que un rebote que el servicio comunique después no suprime la dirección.

Prueba el plugin con el binario `my`:

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

El primero solo comprueba que el plugin responde, y no envía nada. El segundo envía un mensaje real a `--mail-to`, y envía al
plugin una solicitud sin destinatario, que este debe rechazar. El servidor no puede ver el mensaje una vez que lo tiene el plugin, así que mira
el que llega: sus cabeceras deben contener `Message-ID`, `Date` y `Auto-Submitted: auto-generated`.
Consulta [Probar un plugin](plugins.md#test-a-plugin).

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### Recibir las notificaciones de SES (SNS)

Los rebotes y las quejas de SES van a un tema de SNS, y una suscripción HTTPS a ese tema los entrega al
`POST /mail/events/ses` del servidor.
El endpoint se abre solo cuando `mail_ses_sns_topic_arn` está definido; en caso contrario responde `404`.

1. Crea un tema estándar de SNS en la misma región que la identidad de SES.
2. Define el atributo del tema `SignatureVersion` en `2` (SHA256withRSA). Una firma con el valor por defecto `1` no se acepta y el
   servidor responde `400`.
3. En la política de acceso del tema, permite a `ses.amazonaws.com` el `SNS:Publish` desde tu propia cuenta (la consola de SES lo añade
   cuando crea el tema).
4. Dirige las notificaciones de SES al tema, de cualquiera de las dos maneras (el servidor lee tanto `notificationType` como `eventType` en el cuerpo).
   - Notificaciones de identidad: usa este tema para las notificaciones de comentarios «Bounce» y «Complaint» de la identidad `mail_from`.
   - Destino de eventos de un conjunto de configuración: añade este tema como destino del conjunto de configuración de `mail_ses_configuration_set`,
     para los tipos de evento `BOUNCE` y `COMPLAINT`.
5. Define `MY_MAIL_SES_SNS_TOPIC_ARN` con el ARN del tema, comprueba con `my config check` y reinicia.
6. Crea una suscripción al tema con el protocolo `https` y el endpoint `https://<server host>/mail/events/ses`.
   **No actives la entrega de mensajes sin procesar** (un cuerpo sin firma se rechaza).

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

Cuando el servidor recibe la confirmación de la suscripción (`SubscriptionConfirmation`), envía un `GET` a su `SubscribeURL` para
confirmar la suscripción y registra `SNS subscription of the SES notifications confirmed`.
Comprueba con `aws sns list-subscriptions-by-topic` que la suscripción ya no está en `PendingConfirmation`.

Antes de usar un mensaje, el endpoint comprueba todo lo siguiente. Si algo falla, responde `400` y no registra nada.

- `TopicArn` es igual a `mail_ses_sns_topic_arn`.
- `SignatureVersion` es `2` y la firma es válida.
- La URL del certificado de la firma (`SigningCertURL`) es `https` y su host es `sns.<region>.amazonaws.com` para la región
  del tema. El `SubscribeURL` de una confirmación de suscripción debe tener el mismo host.

El certificado se obtiene con el cliente HTTP saliente que protege contra SSRF y se guarda en caché en el almacén KV durante 24 horas. El
cuerpo se lee hasta 1 MiB, y uno mayor recibe `413`.
Cuando no se puede obtener el certificado, falla la confirmación de la suscripción o no se puede escribir el registro, el endpoint
responde `503` y deja que SNS reintente. También responde `503` durante el modo de mantenimiento.

Estas son las notificaciones que registra. Compara la dirección del destinatario con la dirección principal (sin distinguir mayúsculas de minúsculas, y un
dominio en Punycode coincide también en su forma Unicode); si ninguna coincide, no hace nada.

| Notificación | Se registra como |
| --- | --- |
| Un rebote cuyo `bounceType` es `Permanent` (cualquier subtipo, como `General`, `NoEmail`, `Suppressed` u `OnAccountSuppressionList`) | `hard_bounce` |
| Una queja (salvo aquellas cuyo `complaintFeedbackType` es `not-spam`) | `complaint` |
| Un rebote `Transient` o `Undetermined`, y otras notificaciones como entrega o envío | No se registra (responde `200`) |

El `feedbackId` de SES es la clave única de un registro, así que, por muchas veces que SNS envíe la misma notificación, hay un solo registro.

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES (interfaz SMTP)

Cuando no puedes dar credenciales de IAM, por ejemplo cuando ejecutas fuera de AWS, usa la interfaz SMTP de SES con `mail = smtp`. En esta
configuración solo se registra como supresión el rechazo del destinatario en el momento del envío
([supresión de direcciones que no pueden recibir correo electrónico](#suppression-of-addresses-that-cannot-receive-email)).

1. En SES, verifica el dominio de `mail_from` como identidad y activa Easy DKIM (consulta más abajo).
2. Solicita acceso de producción para salir del sandbox. En el sandbox solo puedes enviar a direcciones verificadas.
3. Crea las credenciales SMTP en «SMTP settings» de SES (son distintas de las claves de acceso de IAM).

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES sobrescribe `Message-ID` y `Date` con sus propios valores.

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### Un servidor SMTP submission general

Úsalo para tu propio servidor SMTP, como Postfix, o para el servidor de submission de un proveedor de entrega de correo.

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

El servidor no envía a un servidor que no ofrece STARTTLS, o cuyo certificado no se puede verificar para el nombre de
`mail_smtp_host`. Las credenciales se envían solo después de STARTTLS.
Tras cambiar las configuraciones, comprueba con `my config check` y reinicia.

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF, DKIM y DMARC

Gmail y Yahoo exigen SPF, DKIM, DMARC y baja con un solo clic a los remitentes que envían muchos correos al día. Incluso con un volumen
pequeño, es más probable que un correo sin ellos se clasifique como spam.
Define todos ellos en el DNS del dominio de `mail_from` (por ejemplo `social.example`), con los valores que te dé tu proveedor.

- **SPF**: pon un registro TXT en el dominio que autorice al proveedor por el que envías (por ejemplo, si usas un dominio MAIL FROM personalizado
  de SES, el `include:` que especifique el proveedor, en ese dominio).
- **DKIM**: pon en el DNS la clave pública que emite el proveedor y activa la firma por parte del remitente. Para SES son los tres
  registros CNAME de Easy DKIM. El servidor en sí no firma, así que firma el proveedor.
- **DMARC**: un registro TXT en `_dmarc.social.example`. **Empieza con `p=none`**, comprueba con los informes agregados (`rua=`) que SPF
  y DKIM están alineados (coinciden con el dominio de From) y después endurece a `quarantine` y `reject`.

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

Para alinear SPF en SES, define un dominio MAIL FROM personalizado (el MAIL FROM por defecto es `amazonses.com`, que no coincide con el dominio
de From). Si DKIM está alineado, DMARC pasa.

<!-- source: look-at-send-failures ab9c4f356d3b -->
## Revisar los fallos de envío

Revisa los fallos de envío en la página de trabajos (`/admin/jobs/list`, permiso `manage_jobs`; consulta
[la cola de trabajos y los trabajos fallidos](running.md#the-job-queue-and-failed-dead-jobs)).
Filtra por tipo de trabajo para ver solo los trabajos de correo electrónico (en la API de administración, `GET /api/v1/admin/jobs?type=identity.verification_email`).
Los tipos están en la tabla anterior.

- **Los fallos temporales** (errores de conexión, tiempos de espera agotados, respuestas 4xx) se reintentan. El correo transaccional se intenta 6 veces (un
  backoff exponencial desde 30 segundos, unos 15 minutos en total); el correo de notificación usa la política por defecto (10 veces, unas
  4 horas). Cuando se agotan los intentos, el trabajo pasa a ser un trabajo fallido (trabajo muerto). Un `mail_smtp_host` incorrecto, una
  autenticación fallida y una red bloqueada aparecen aquí. Una vez corregida la causa, vuelve a ejecutarlo con `my job retry <id>` o en la
  pantalla de trabajos.
- **Los rechazos permanentes** (un 5xx por el destinatario o por el contenido) no se reintentan y fallan de inmediato.
- Con `mail = ses`, el `MessageRejected` de SES (se rechazó el contenido) es un rechazo permanente. `TooManyRequestsException`
  (el límite de tasa de envío), `SendingPausedException`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException` y
  las solicitudes que no llegan a SES son fallos temporales, y un reintento tiene éxito una vez corregida la cuenta o los permisos.
  En el texto del error solo se conserva el código de error de SES.
- Con `mail = plugin`, decide el [`outcome`](#a-plugin-mail--plugin) de la respuesta. Un plugin al que no se puede llegar, o que
  responde con un error, es un fallo temporal, y cuenta en [`my.plugin.failures`](running.md). En el texto del error solo se conservan el resultado y el código de
  estado mejorado, así que el motivo de un fallo está en el registro del plugin.
- Un correo transaccional que supera un límite de tiempo desde su solicitud (15 minutos para un restablecimiento, 1 hora para una confirmación) se
  completa sin enviarse. Esto evita que lleguen enlaces antiguos tarde; no es un fallo.
- Cuando el trabajo se ejecuta y no hay motivo para enviar (la dirección está suprimida, se ha confirmado entretanto, o el usuario
  perdió el permiso o la configuración de la notificación), se completa sin enviar.

La carga de un trabajo no contiene dirección de correo electrónico, cuerpo ni token, y tampoco el texto de un error. Solo quedan los ID, el tipo de trabajo y el código de
respuesta SMTP.
Un envío correcto se puede seguir en la línea de registro estructurada `job` de cada ejecución (`my.job.type`).

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## Supresión de direcciones que no pueden recibir correo electrónico

Cuando el servidor SMTP dice que la dirección del destinatario no existe (un 5xx a `RCPT TO` con un estado extendido `5.1.x`), el
servidor registra la dirección como rebote definitivo y deja de enviarle.
Con `mail = plugin`, `OUTCOME_RECIPIENT_REJECTED` registra el mismo rebote definitivo, y nada más lo hace.
Con `mail = smtp`, solo se registra este rechazo en el momento del envío. Los rebotes asíncronos que vuelven después de que el servidor SMTP
aceptara el correo, y los bucles de retroalimentación de quejas, no se reciben.
Con `mail = ses`, SES nunca rechaza a un destinatario de forma síncrona, así que no se registra nada en el momento del envío; los rebotes permanentes y las
quejas se registran a partir de las notificaciones de SES
([Recibir las notificaciones de SES (SNS)](#receive-ses-notifications-sns)).

| Tipo | Correo electrónico que se detiene |
| --- | --- |
| `hard_bounce` | Todo excepto el correo de confirmación. El correo de restablecimiento de contraseña también se detiene |
| `complaint` (denunciado como spam) | Los correos de notificación y de anuncios. Los correos transaccionales se siguen enviando |

- Los moderadores ven el tipo y el origen de una supresión en la página de gestión de la cuenta de las pantallas de administración (`email_suppression` en
  `GET /api/v1/admin/accounts/{id}`).
- Hay dos maneras de **levantarla**. Cuando el usuario pide un correo de confirmación y confirma de nuevo la dirección, se levanta la supresión por
  rebote definitivo (el correo de confirmación se envía incluso mientras la dirección está suprimida). El usuario lo pide a través de la
  API de cliente (`POST /api/v1/accounts/email/verification`, como máximo 3 por hora) o en la página de estado
  de inicio de sesión. Cuando has comprobado que es
  seguro enviar, por ejemplo porque se arregló el buzón, un moderador la levanta con
  `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permiso `manage_accounts`). Registra
  `email_suppression_lifted` en el registro de auditoría ([moderación](moderation.md)).
- Para un usuario cuyo correo de restablecimiento de contraseña está detenido, un administrador emite un enlace de restablecimiento y se lo entrega
  (`my admin account password-reset-link @alice`).
- En la pantalla de configuración, los usuarios ven que el correo no llega a su dirección principal, y desde cuándo.

<!-- source: unsubscribing 2806dbccc409 -->
## Darse de baja

Los correos de notificación llevan un enlace para darse de baja y las cabeceras `List-Unsubscribe` y `List-Unsubscribe-Post:
List-Unsubscribe=One-Click` (RFC 8058). El botón «darse de baja» de los clientes de correo las usa.

- Las clases de baja se corresponden una a una con las filas de las configuraciones de notificaciones cuyo correo puede elegir el usuario, como
  `mention` o `admin_report` (una por tipo de aviso a moderadores, y `moderation` para los avisos de los moderadores).
- El enlace es `https://{host}/mail/unsubscribe?token=...` y no requiere iniciar sesión. Se emite un token por correo, solo su hash se
  guarda en la base de datos, y caduca a los 90 días.
  - `GET` solo muestra una pantalla de confirmación y no cambia nada (para que los escáneres que precargan enlaces no detengan el correo).
    El botón envía el mismo `POST`.
  - `POST` (un clic) desactiva el correo de esa fila de las configuraciones de notificaciones, y nada más. Hasta que el token
    caduca, enviarlo de nuevo da el mismo resultado.
- **Del correo transaccional no se puede uno dar de baja.** La confirmación, el restablecimiento y el aviso de un cambio de contraseña
  impedirían que llegaran avisos relevantes para la seguridad. Estos correos tampoco llevan tales cabeceras.
- Los usuarios pueden cambiar la misma configuración en la página **Notificaciones** de la configuración.

<!-- source: the-html-version 2ca008b1a48a -->
## La versión HTML

Todos los correos son `multipart/alternative` con el mismo contenido como `text/plain` y `text/html` (ambos en UTF-8). Con `mail = none`
en el perfil `development`, solo la versión de texto se escribe en el registro.

- Hay un diseño exterior, con párrafos, botones y una firma (nombre del servidor, URL, contacto, el motivo del correo
  y el enlace para darse de baja). `<html lang dir>` viene de la configuración regional del destinatario, y los idiomas como `ar` se maquetan de derecha a
  izquierda.
- No carga fuentes ni hojas de estilo externas. Los colores son los tokens de diseño por defecto, y el modo claro y oscuro siguen
  `color-scheme` y `prefers-color-scheme`. Los cambios de tema del administrador no afectan al correo.
- No carga imágenes, salvo el logotipo propio de la instancia cuando hay uno ([logotipo e icono](instance-settings.md#logo-and-icon)):
  la versión HTML empieza entonces con el logotipo (`https://<host>/brand/...`, el logotipo oscuro en su lugar donde el cliente de correo sigue
  un esquema de color oscuro), la misma URL para todos los destinatarios, así que no revela nada sobre quién abrió el correo. Los clientes que no cargan
  imágenes externas muestran en su lugar el nombre del servidor (el texto alternativo de la imagen). Sin logotipo, o cuando la URL del servidor no es `https`,
  el correo no tiene ninguna imagen.
- Solo las URL `https` se convierten en enlaces. Cuando una URL es `http`, como en una configuración de desarrollo donde `web_origin` es `http://{host}`, la URL
  se escribe como texto en lugar de un botón o un enlace para darse de baja, y se omite la URL del servidor en la firma.
- El diseño forma parte del binario, así que no se ejecuta JavaScript en el servidor en tiempo de ejecución.
