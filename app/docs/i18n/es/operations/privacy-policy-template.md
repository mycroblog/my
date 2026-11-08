<!-- source: privacy-policy-template 9966bbc6844c -->
# Plantilla de política de privacidad

Una política de privacidad para adaptar a tu servidor: describe qué conserva este servidor, por qué, durante cuánto tiempo y quién lo recibe.

**Esto es una plantilla y no es asesoramiento jurídico.** Describe lo que hace el software con su configuración por defecto. No puede saber cómo gestionas tu instancia, y no decide qué leyes se te aplican. Lo que debe decir una política depende de tu jurisdicción y de la de tus usuarios (por ejemplo, el RGPD en la UE, el RGPD del Reino Unido o la Ley de Protección de la Información Personal de Japón). Haz que la revisen para tu jurisdicción antes de publicarla, y considera las bases jurídicas y la declaración sobre transferencias de más abajo como cuestiones que debes resolver, no como respuestas.

El registro de actividades de tratamiento que la acompaña está en [Plantilla del registro de actividades de tratamiento](processing-record-template.md).

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## Cómo usarla

1. Copia [la plantilla](#the-template) en un archivo, `privacy-policy.md`.
2. Sustituye cada `{{...}}`. Un valor entre llaves, como `{{180 days}}`, es el valor por defecto del software: confírmalo en tu instancia ([Los periodos y su configuración](#the-periods-and-their-settings)) y quita las llaves.
3. Una fila o una frase que empieza con `{{IF ...}}` se aplica solo a esa configuración ([Los servicios que eliges](#the-services-you-choose)). Elimínala cuando no se aplique. Cuando se aplique, quita el marcador.
4. Comprueba que no queda nada: `grep -n '{{' privacy-policy.md` no imprime ninguna línea.
5. Publícala con `my instance set privacy_policy - < privacy-policy.md`, o en la página «Política de privacidad» de «Términos y políticas» (`/admin/documents/privacy-policy`) en la administración. Los lectores la encuentran en `/about/privacy` ([configuración de la instancia](instance-settings.md#terms-and-privacy-policy)).

Una política de privacidad escrita añade una casilla de aceptación obligatoria a la pantalla de registro. Esa casilla demuestra que alguien leyó el documento. No supongas que sea consentimiento para el tratamiento allí donde tu ley exija consentimiento.

El texto tiene un único valor para todos los idiomas. Si tienes lectores en varios idiomas, escribe los idiomas que necesites dentro de ese único texto.

Revisa la política cuando cambies un ajuste de los siguientes o un servicio, y después de cada actualización cuyas notas de la versión mencionen datos personales.

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## Los periodos y su configuración

`my instance list` muestra toda la configuración de la instancia y si cada ajuste tiene su valor por defecto. Un cambio en un periodo surte efecto en la siguiente ejecución de su trabajo horario. Los valores fuera del intervalo se rechazan.

| Registro | Valor por defecto | Ajuste de la instancia | Intervalo (días) | Cuenta desde |
| --- | --- | --- | --- | --- |
| Dirección IP y User-Agent del inicio de sesión | 180 días | `session_origins_retention_days` | 90-3650 | El inicio de sesión |
| Dirección IP del registro | 180 días | `registration_origins_retention_days` | 90-3650 | El registro |
| Denuncias | 365 días | `reports_retention_days` | 180-3650 | La última resolución |
| Acciones contra cuentas, y apelaciones | 365 días | `moderation_actions_retention_days` | 180-3650 | El fin de la acción |
| Registro de auditoría | 730 días | `audit_log_retention_days` | 365-3650 | El registro |
| Quién participó en una sala de chat de voz | 90 días | `voice_participants_retention_days` | 30-3650 | El fin de la sala |
| Las decisiones de una revisión externa, cuando usas una | 90 días | `hook_decisions_retention_days` | 30-3650 | La decisión |
| Publicaciones remotas a las que nada de aquí hace referencia | 90 días | `remote_status_retention_days` | 1-3650 | La publicación del mensaje |
| Cualquier publicación remota | Sin límite (`0`) | `remote_status_max_retention_days` | 0-3650 | La publicación del mensaje |
| Publicaciones remotas que solo trajo un relay | 7 días | `relay_status_retention_days` | 1-3650 | La publicación del mensaje |
| Copias de archivos multimedia remotos | 1 día, y ninguna se conserva mientras `remote_media_policy` sea `none` | `remote_media_cache_ttl_seconds`, `remote_media_policy` | 60-31536000 segundos | La obtención |
| Traducciones de publicaciones | 30 días | `translation_retention_days` | 1-3650 | La traducción. Leerla no prolonga el periodo, y desaparece cuando se elimina la publicación |

Estos son valores de configuración del despliegue ([referencia de configuración](../reference/configuration.md)), no de la configuración de la instancia:

| Registro | Valor por defecto | Ajuste |
| --- | --- | --- |
| Subidas que no están adjuntas a nada | 24 horas | `media_unattached_retention` |
| Versiones antiguas de imágenes, después de que las sustituya una versión más reciente | 30 días | `media_replaced_retention` |
| Trabajos que fallaron definitivamente | 30 días | `dead_job_retention` |
| Registros de ejecuciones de trabajos que no tuvieron éxito | 30 días | `job_attempt_retention` |

Estos periodos los fija el software, y ningún ajuste los cambia:

| Registro | Periodo |
| --- | --- |
| Borrado de una cuenta eliminada | 7 días después de la eliminación; hasta 30 días para una cuenta remota cuando solo lo indicó un `410 Gone` de su servidor y el servidor no vuelve a responder |
| Eliminación de una cuenta desactivada | 30 días después de la desactivación |
| Bloqueos de registro | 30 días después del levantamiento o de la caducidad; sin ninguno de los dos, mientras estén en vigor |
| Sesiones, tokens y códigos de autorización | 30 días después de que caduquen |
| Archivos de las exportaciones de datos, e importaciones de datos | 7 días |
| Registros de las actividades recibidas de otros servidores | 30 días, y el documento en sí 7 días |
| Registros de las actividades entregadas a otros servidores | 30 días; la entrega de una publicación hasta 30 días después de que se elimine la publicación o su autor |
| Comprobaciones de identificadores remotos | 90 días, conservando la más reciente de cada cuenta |
| Mensajes y entregas de webhooks | 14 días |
| Recibos de webhooks de pago | 45 días |
| Recuentos horarios de errores y avisos de errores | 90 días |

Una [retención](moderation.md#holds) detiene el borrado de una cuenta, de modo que todos los periodos anteriores pueden durar más para una cuenta bajo retención. Si usas retenciones, dilo, como hace la plantilla.

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## Registros sin fin automático

El servidor los elimina solo cuando se borra una cuenta, o nunca. Una ley puede pedirte que justifiques conservarlos, o que los elimines por otros medios, así que decide qué dice tu política sobre cada uno.

- Las filas de una cuenta activa: direcciones de correo electrónico, versiones anteriores del perfil, versiones anteriores de las publicaciones, notificaciones, seguimientos y la solicitud de registro de un usuario rechazado. Una cuenta que nadie elimina permanece.
- Lo que permanece tras el borrado: el URI y el nombre de usuario de la cuenta (para que no se pueda reutilizar ninguno), sus eventos del ciclo de vida, el registro de que una publicación fue eliminada, los registros de moderación hasta que termine su propio periodo, las invitaciones y las concesiones de roles, las migraciones de cuenta y las salas de chat de voz sin sus nombres ni descripciones.
- Los registros de mecenas (suscripciones, cambios de estado, pagos, reembolsos, contracargos), que se conservan por contabilidad y para conciliar con el proveedor. El servidor no elimina a un cliente en el proveedor de pagos: hazlo en el panel del proveedor si tus obligaciones lo permiten.
- Una acción sin fin (un `ban`, o un `suspend` sin hora de finalización), hasta que se levante. Las denuncias abiertas.
- Las cuentas remotas, hasta que su servidor las elimine.
- El último informe de cada grupo de errores, que el siguiente informe de ese grupo sustituye. Se depura por patrones, así que puede quedar un fragmento de texto de un usuario.
- Las copias de seguridad, tus registros y los registros de tu plataforma y de tu proxy. El servidor no los gestiona: la [copia de seguridad](backup.md#how-often-and-where) describe las generaciones que conservar, y una copia de seguridad restaurada devuelve datos borrados.

<!-- source: the-services-you-choose 81461216c966 -->
## Los servicios que eliges

Cada servicio que recibe datos personales aparece en la política solo si lo activaste. Rellena la tabla de la sección 6 a partir de esta lista, y lee la política y el contrato del propio servicio: lo que el servicio hace con los datos una vez que llegan no es algo que este documento pueda decirte.

| Marcador | Ajuste | Qué recibe el servicio |
| --- | --- | --- |
| Alojamiento | `database`, `blob`, `queue`, `key_manager` | Todo lo que contienen la base de datos y el almacenamiento de blobs, en la forma en que lo guarde el proveedor. Los datos de los trabajos en una cola en la nube contienen identificadores y categorías, sin direcciones, cuerpos de mensajes ni secretos. |
| `{{IF cdn}}` | `media_origin` | Las direcciones IP de los lectores y las peticiones de archivos multimedia ([CDN](cdn.md)). |
| Proxy | Tu proxy inverso o balanceador de carga, que el servidor necesita para TLS | Cada petición con su dirección IP, según el registro que configures. |
| `{{IF mail}}` | `mail` es `smtp` o `ses` | Direcciones de los destinatarios, asuntos y cuerpos. El correo electrónico de notificación lleva el mismo texto que la notificación ([correo electrónico](email.md)). |
| `{{IF captcha}}` | `captcha` no es `none` | La dirección IP, la información del navegador y del dispositivo, las cookies y las interacciones con el widget de todas las personas que abren la página de inicio de sesión, de registro o de solicitud de restablecimiento de contraseña, directamente desde su navegador. El servidor envía al servicio solo el token y no añade ninguna dirección IP ([CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)). |
| `{{IF login providers}}` | [Proveedores de acceso](login-providers.md) | El proveedor ve el inicio de sesión. El servidor recibe el identificador de la persona en el proveedor, lo conserva y, en el registro, usa la dirección de correo electrónico y el nombre de usuario que da el proveedor. |
| `{{IF stripe}}` | `payments = stripe` | La dirección de correo electrónico que rellena previamente Checkout, el plan y el ID de suscripción local. Stripe recoge los datos de pago en sus propias páginas ([Stripe](payments-stripe.md#card-data-and-pci-dss)). |
| `{{IF payjp}}` | `payments = payjp` | La dirección de correo electrónico y el plan. El navegador carga payjp.js en las páginas de pago del servidor, la tarjeta se introduce en el iframe de PAY.JP, y el nombre y la dirección de correo electrónico van a PAY.JP para 3-D Secure ([PAY.JP](payments-payjp.md#card-data-and-pci-dss)). |
| `{{IF error reports}}` | `errors_exporter` | Informes depurados sin cuenta, dirección IP, cuerpo ni cabecera. La depuración se basa en patrones, así que puede quedar un fragmento de texto de un usuario ([informe de errores](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)). |
| `{{IF telemetry}}` | `metrics_exporter`, `traces_exporter` | Métricas y spans sin cuentas ni direcciones IP de clientes. Los spans de las peticiones salientes nombran los nombres de host de los servidores remotos ([ejecución](running.md)). |
| `{{IF voice media server}}` | `voice` es `livekit` o `sora` | Las direcciones IP de los participantes, su audio mientras hablan, un ID de sesión y el ID de la sala. El ID de la cuenta no se envía ([chat de voz](voice.md)). |
| `{{IF turn}}` | `voice_stun_urls`, `voice_turn` | Los servidores STUN y TURN ven las direcciones IP de los participantes, y un servidor TURN retransmite su audio cifrado. |
| `{{IF webhooks}}` | [Webhooks del servidor](webhooks.md) | Tu receptor recibe los cuerpos de los eventos. Algunos llevan la dirección de correo electrónico de un solicitante o el comentario de una denuncia. |
| `{{IF follow review}}` | `plugin_inbound_follow_address` está definido y `hook_inbound_follow_mode` no es `off` | Las direcciones de las dos cuentas de cada seguimiento procedente de otro servidor, enviadas por el servidor cuando llega el seguimiento ([hooks de decisión](decision-hooks.md)). La decisión registrada conserva la dirección de la actividad y qué cuenta la envió. |
| `{{IF sign-up review}}` | `plugin_signup_check_address` está definido y `hook_signup_check_mode` no es `off` | La dirección de correo electrónico y la dirección IP de cada solicitante sin invitación, enviadas por el servidor cuando se registra ([hooks de decisión](decision-hooks.md#sign-ups)). La decisión registrada no contiene ninguna de las dos. Nombra el servicio en la sección 6, salvo que ejecutes tú mismo el plugin. |
| `{{IF post review}}` | `plugin_local_status_address` está definido y `hook_local_status_mode` no es `off` | El texto de cada publicación `public`, `local` y `unlisted` de una cuenta con menos antigüedad que `hook_local_status_account_age_days` (7 días por defecto; `0` es toda cuenta), tal como se escribió y tal como se muestra, con su advertencia de contenido, idioma, si está marcada como sensible, opciones de encuesta, descripciones de imágenes, etiquetas, cuántas cuentas menciona, la dirección que tendría, las direcciones de las publicaciones a las que responde y que cita, la dirección y el nombre de usuario del autor, y cuándo se creó la cuenta, enviados por el servidor cuando se hace la publicación y de nuevo con cada edición ([hooks de decisión](decision-hooks.md#local-posts)). Las publicaciones `private` y `direct` nunca se envían. La decisión registrada no contiene texto. Un autor que apela una publicación rechazada envía su texto, que el servidor conserva con la decisión. Nombra el servicio en la sección 6, salvo que ejecutes tú mismo el plugin. |
| `{{IF profile review}}` | `plugin_inbound_actor_address` está definido y `hook_inbound_actor_mode` no es `off` | Los perfiles públicos de cuentas de otros servidores, cada vez que uno cambia: el nombre visible, la biografía, los campos del perfil, las direcciones del avatar y de la imagen de cabecera (sin imágenes), la dirección, el servidor y el nombre de usuario de la cuenta, y si la cuenta es nueva aquí ([hooks de decisión](decision-hooks.md#profiles-from-other-servers)). No se envía nada de las cuentas de tu servidor. La decisión registrada conserva la dirección de la cuenta y un resumen criptográfico del perfil. |
| `{{IF profile review}}` | `plugin_local_profile_address` está definido y `hook_local_profile_mode` no es `off` | El nombre visible, la biografía tal como se escribió y tal como se muestra, los campos del perfil, si cambió el avatar o la cabecera, la dirección y el nombre de usuario de la cuenta, y cuándo se creó la cuenta, enviados por el servidor cada vez que una cuenta con menos antigüedad que `hook_local_profile_account_age_days` (7 días por defecto; `0` es toda cuenta) los cambia ([hooks de decisión](decision-hooks.md#local-profiles)). No se envía ninguna imagen. La decisión registrada no contiene nada de esto. Nombra el servicio en la sección 6, salvo que ejecutes tú mismo el plugin. |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` está definido y `hook_inbound_status_mode` no es `off` | Las publicaciones y revisiones que envían otros servidores y que se refieren a una cuenta de aquí, incluidas las publicaciones solo para seguidores (`private`): el texto en cada uno de sus idiomas, la advertencia de contenido, si está marcada como sensible, cuándo se publicó, su dirección y su URL, la dirección de la publicación a la que responde, opciones de encuesta, etiquetas, direcciones y descripciones de los adjuntos (sin archivos multimedia), su autor y el servidor del autor, su visibilidad, cuántas cuentas nombra y cuántas de ellas están aquí, y si alguien de aquí sigue al autor ([hooks de decisión](decision-hooks.md#what-is-sent-and-what-it-costs)). Las publicaciones directas solo se envían cuando alguien de aquí sigue al autor, o con `hook_inbound_status_unfollowed_direct` de cualquier autor. |
| `{{IF media review}}` | `plugin_media_check_address` está definido y `hook_media_check_mode` no es `off` | La imagen procesada más pequeña de cada subida, sin metadatos, nunca el original, con la finalidad del archivo y su tipo, tamaño y duración ([hooks de decisión](decision-hooks.md#what-mediacheck-reviews)). La decisión registrada conserva el ID del archivo y quién lo subió. Una subida rechazada no se conserva en absoluto: si debes preservar una o denunciarla, tu plugin tiene que hacerlo. Nombra el servicio en la sección 6, salvo que ejecutes tú mismo el plugin. |
| `{{IF translation}}` | `translation_providers` no está vacío | El texto de una publicación cuando un lector pide traducirla: su cuerpo, advertencia de contenido, opciones de encuesta y descripciones de imágenes. No se envían la dirección de la publicación, su autor, el lector, las menciones ni los enlaces. Por defecto un proveedor recibe las publicaciones `public`, `local` y `unlisted`. Funciona como tu proveedor de servicios, igual que el alojamiento, de modo que las publicaciones `local` le llegan aunque no se federan: conserva la frase sobre `local` en la sección 5. Un proveedor que conserva el texto para su propio uso no es un proveedor de servicios en este sentido, y la política debe decirlo. |
| `{{IF translation}}` con el plan gratuito de DeepL | `translation_providers` incluye `deepl` con una clave que termina en `:fx` | Lo mismo. DeepL puede conservar el texto y las traducciones de forma indefinida, y la política debe decirlo ([traducción](translation.md#deepl)). |
| `{{IF translation}}` con Amazon Translate | `translation_providers` incluye `aws` | Lo mismo. Salvo que la política de exclusión de servicios de IA de AWS Organizations cubra tu cuenta, Amazon puede almacenar el texto y usarlo para mejorar sus servicios, posiblemente en otra región, y la política debe decirlo ([traducción](translation.md#amazon-translate)). |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` permite a un proveedor `private` o `direct` | Lo mismo, para esas publicaciones. Usa este marcador solo cuando lo hayas permitido, por proveedor. Añade líneas a las secciones 2 y 5, y amplía tu promesa sobre esas publicaciones, así que anúncialo antes de cambiar el ajuste. |
| `{{IF translation of reports}}` | `translation_reports_to_server` | El comentario de una denuncia y las publicaciones que nombra, cuando un moderador pide traducirlos. Cada proveedor sigue recibiendo solo las visibilidades que tiene permitidas, y el comentario cuenta como `private`: si ningún proveedor tiene permitido `private`, los comentarios no llegan a ninguno. La traducción del comentario no se almacena. |
| `{{IF browser translation}}` | `translation_browser` | No llega nada a este servidor ni a un proveedor. El navegador traduce en el dispositivo del lector, y lo que haga el fabricante del navegador depende de ese navegador. |

Los servicios son elecciones tuyas, así que esta plantilla no dice dónde está establecido cada uno ni qué contrato lo cubre. Confirma ambas cosas tú mismo.

Dos cosas llegan a terceros sin un ajuste tuyo:

- **Web Push.** Cuando un usuario lo activa, su navegador elige el servicio de push y da su dirección al servidor. El cuerpo del mensaje está cifrado para ese navegador, así que el servicio de push no debería poder leerlo. Sí ve la dirección de la suscripción, el tamaño y la hora de cada mensaje y el contacto (asunto VAPID) de tu servidor, y retiene un mensaje hasta 48 horas mientras el navegador está desconectado.
- **Federación.** Consulta la sección 5 de la plantilla. `federation_mode` la desactiva o limita su dirección, y los relays los añades tú ([relays](relays.md)).

<!-- source: the-template 8b6bd2fc983a -->
## La plantilla

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
