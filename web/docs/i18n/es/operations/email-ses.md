<!-- source: sending-email-through-amazon-ses 6deee2054246 -->
# Enviar correo con Amazon SES

Cómo preparar Amazon SES para enviar el correo del servidor: verificar el dominio del remitente con registros DNS, salir del sandbox
en el que empieza una cuenta nueva y mantener la cuenta en buen estado. El módulo de OpenTofu [`mycroblog-aws`](deploy-aws.md#email-through-amazon-ses) envía por SES de forma
predeterminada y crea la identidad y el permiso; los registros y la solicitud de acceso a producción son pasos tuyos.
Los ajustes en sí están en [Enviar correo](email.md#amazon-ses-api).

**El despliegue en AWS nunca se ha aplicado**, así que estos pasos proceden de la documentación de AWS y no se han seguido de principio
a fin en un servidor de este proyecto.

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. Añadir los registros del dominio

SES envía desde una identidad verificada. `mycroblog-aws` crea la identidad del dominio de `mail.from` (para
`noreply@social.example`, el dominio `social.example`) con Easy DKIM, en la región del despliegue. Tras el apply, la
salida `dns_records` enumera los registros que hay que añadir en tu proveedor de DNS, con el registro de `host`:

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

Los tres registros CNAME publican las claves con las que firma SES. SES las comprueba por sí mismo y marca la identidad como verificada cuando
resuelven, normalmente en una hora y como máximo en 72 horas; hasta entonces se niega a enviar. La consola de SES muestra el estado
de la identidad en **Identities**.

Añade también registros SPF y DMARC, que los receptores comprueban junto a DKIM: [SPF, DKIM y DMARC](email.md#spf-dkim-and-dmarc) dice
qué poner en cada uno. El DKIM firmado por SES se alinea con el dominio de `mail.from`, así que DMARC pasa solo con estos registros; un
dominio MAIL FROM personalizado, que alinea también SPF, es opcional.

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. Salir del sandbox

Toda cuenta nueva empieza en el sandbox de SES, por separado en cada región. En el sandbox, SES envía solo a direcciones y
dominios que hayas verificado tú mismo, y como máximo 200 mensajes en 24 horas y uno por segundo. Un servidor en el sandbox puede enviar a su
propio operador un mensaje de prueba, pero el registro de cualquier otra persona falla.

Para enviar a todo el mundo, solicita el acceso a producción para la región del despliegue:

1. Abre la consola de SES en esa región y, en **Account dashboard**, elige **Request production access** (o **Get set up**).
2. Elige **Transactional** como tipo de correo, indica la URL del servidor y describe lo que envía: confirmaciones de
   registro y de una dirección nueva, restablecimientos de contraseña, y las notificaciones que cada usuario activa y puede desactivar con una
   baja de un clic.
3. Explica cómo gestionas los rebotes y las quejas: el servidor deja de enviar a una dirección que rebota o se queja cuando las
   notificaciones de abajo le llegan.

AWS responde por correo, normalmente en un día, y puede pedir más información antes de conceder el acceso. Una vez concedido, la consola muestra
tu cuota de envío, que crece con tu uso.

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. Vigilar los rebotes y las quejas

SES vigila la proporción de mensajes que rebotan o provocan quejas, y pone en revisión, y luego pausa, una cuenta cuyas tasas
se mantienen altas. El servidor se entera de un rebote o de una queja por las notificaciones de SES a través de Amazon SNS y deja de enviar a esa
dirección. `mycroblog-aws` no crea el topic: configúralo como dice [Recibir las notificaciones de SES](email.md#receive-ses-notifications-sns),
y dale al servidor su ARN mediante los `settings` del módulo:

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

La consola de SES muestra las tasas de rebote y de quejas de la cuenta en **Reputation metrics**.

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## Enviar desde otra región

El módulo crea la identidad en la región del despliegue, y el servidor envía allí. Para enviar por SES desde otra
región, o por un servicio SMTP en su lugar, define `mail = { kind = "smtp", ... }` con la interfaz SMTP de ese servicio:
[Amazon SES (interfaz SMTP)](email.md#amazon-ses-smtp-interface) tiene los ajustes.
