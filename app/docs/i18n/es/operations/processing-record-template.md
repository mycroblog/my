<!-- source: record-of-processing-activities-template 75f7037cf953 -->
# Plantilla del registro de actividades de tratamiento

El registro de actividades de tratamiento que el artículo 30(1) del RGPD exige llevar al responsable del tratamiento, rellenado hasta donde decide el software.

**Esto es una plantilla y no asesoramiento jurídico.** Describe lo que hace el software con su configuración predeterminada y no decide si el RGPD, o este registro, se aplica a ti. Haz que lo revisen para tu jurisdicción.

El artículo 30(5) exime a una organización de menos de 250 empleados, salvo que su tratamiento pueda entrañar un riesgo para los derechos y libertades de las personas, no sea ocasional o incluya categorías especiales o datos penales. Ejecutar una instancia mantiene cuentas de forma continua, así que no des por supuesta la exención sin comprobarla. Llevar el registro también te ayuda a responder solicitudes y a redactar la [política de privacidad](privacy-policy-template.md).

<!-- source: how-to-use-it 32e30db3b6f3 -->
## Cómo usarla

1. Copia todo lo que hay desde [Registro](#record) en tus propios registros. Es un documento interno: no lo publiques con los textos de la instancia, y entrégalo a una autoridad de control cuando lo solicite.
2. Sustituye cada `{{...}}`. Un valor entre llaves, como `{{180 days}}`, es el valor predeterminado del software. Confírmalo con tu instancia. La [plantilla de política de privacidad](privacy-policy-template.md#the-periods-and-their-settings) nombra el ajuste de cada plazo.
3. Elimina una actividad, o una línea que empiece por `{{IF ...}}`, que no corresponda a tu configuración. [Los servicios que eliges](privacy-policy-template.md#the-services-you-choose) indica qué ajuste sigue cada servicio.
4. Comprueba que no queda nada: `grep -n '{{' <your file>` no imprime ninguna línea.
5. Revísala cuando cambies un ajuste o un servicio, y fecha cada revisión.

El artículo 30 no pide la base jurídica, y el registro tiene una línea para ella porque la necesitas para la política. Trata las entradas que hay allí como preguntas por resolver.

Si ejecutas una instancia por cuenta de otra persona, puedes ser su encargado del tratamiento. El artículo 30(2) pide entonces un registro distinto, que esta plantilla no cubre.

<!-- source: record ed52538cf508 -->
## Registro

<!-- source: controller-article-301a 9720484d93ff -->
### Responsable (artículo 30(1)(a))

| | |
| --- | --- |
| Responsable | {{NOMBRE, DIRECCIÓN POSTAL, CORREO ELECTRÓNICO}} |
| Corresponsable | {{NOMBRE Y CONTACTO, O «ninguno»}} |
| Representante en la Unión | {{NOMBRE Y CONTACTO, O «no aplica»}} |
| Delegado de protección de datos | {{NOMBRE Y CONTACTO, O «ninguno»}} |
| Instancia | {{NOMBRE Y URL DE LA INSTANCIA}} |
| Software | my, versión {{VERSIÓN}} |
| Última revisión | {{FECHA, POR QUIÉN}} |

<!-- source: recipients-used-in-this-record 8f1748f022f5 -->
### Destinatarios usados en este registro

Estos nombres se usan en las actividades que siguen. Enumera cada uno de los que uses.

| Destinatario | Nombre, contacto y contrato | País |
| --- | --- | --- |
| Alojamiento (servidor, base de datos, almacenamiento de blobs, cola, gestión de claves) | {{...}} | {{...}} |
| Proxy inverso o balanceador de carga | {{...}} | {{...}} |
| {{IF cdn}} CDN | {{...}} | {{...}} |
| {{IF mail}} Proveedor de correo | {{...}} | {{...}} |
| {{IF captcha}} Servicio de CAPTCHA | {{...}} | {{...}} |
| {{IF login providers}} Proveedores de inicio de sesión | {{...}} | {{...}} |
| {{IF payments}} Proveedor de pagos (Stripe o PAY.JP) | {{...}} | {{...}} |
| {{IF error reports}} Rastreador de errores | {{...}} | {{...}} |
| {{IF telemetry}} Servicio de métricas y trazas | {{...}} | {{...}} |
| {{IF voice}} Servidor multimedia de voz, STUN y TURN | {{...}} | {{...}} |
| {{IF webhooks}} Receptores de webhooks | {{...}} | {{...}} |
| {{IF follow review}} Servicio de revisión de seguimientos | {{...}} | {{...}} |
| {{IF sign-up review}} Servicio de revisión de registros | {{...}} | {{...}} |
| {{IF post review}} Servicio de revisión de publicaciones | {{...}} | {{...}} |
| {{IF profile review}} Servicio de revisión de perfiles | {{...}} | {{...}} |
| {{IF media review}} Servicio de revisión de archivos multimedia | {{...}} | {{...}} |
| {{IF translation}} Proveedores de traducción | {{...}} | {{...}} |
| {{IF external review of posts}} Revisión externa de publicaciones de otros servidores | {{...}} | {{...}} |
| Almacenamiento de copias de seguridad | {{...}} | {{...}} |
| Servidores federados | Otros servidores de la red, gestionados por otros responsables, que cambian con el tiempo. `my` mantiene la lista de servidores conocidos | Cualquier país |
| {{IF relays}} Relés | {{CADA RELÉ AL QUE TE UNISTE}} | {{...}} |
| Servicios push | Elegidos por el navegador de cada usuario | Depende del navegador |
| Aplicaciones | Autorizadas por cada usuario | Depende de la aplicación |

<!-- source: transfers-article-301e 82e1298af5f2 -->
### Transferencias (artículo 30(1)(e))

- La entrega federada envía datos personales a servidores en el extranjero, en países que no se conocen de antemano y no pueden limitarse. {{TU VALORACIÓN: si esto es una transferencia con arreglo al capítulo V del RGPD y con qué base. Esta plantilla lo deja abierto.}}
- Transferencias a los servicios anteriores: {{PARA CADA UNO, EL TERCER PAÍS Y LA DECISIÓN DE ADECUACIÓN O LA GARANTÍA}}.

<!-- source: 1-accounts-and-signing-in 865bacb0785b -->
### 1. Cuentas e inicio de sesión

| | |
| --- | --- |
| Fines | Crear cuentas, iniciar sesión, recuperar el acceso, decidir los registros |
| Interesados | Personas con una cuenta, personas que solicitan una |
| Datos personales | Direcciones de correo electrónico, hash de la contraseña, secreto de autenticación en dos pasos y códigos de recuperación con hash, claves de acceso, el identificador en un proveedor de inicio de sesión, preferencias (idioma, zona horaria, valores predeterminados de publicación), motivo del registro, invitación y decisión, roles, tokens de sesión y de aplicación con hash |
| Destinatarios | Alojamiento, proveedor de correo, proveedores de inicio de sesión, personal con `manage_accounts` |
| Supresión | Supresión de la cuenta: 7 días después de eliminarla. Una cuenta desactivada se elimina tras 30 días. Sesiones, tokens y códigos: 30 días después de caducar. Las invitaciones y las concesiones de roles, el URI de la cuenta y el nombre de usuario permanecen. Una cuenta que nadie elimina permanece |
| Base jurídica | {{p. ej. contrato}} |

<!-- source: 2-protecting-sign-in-and-registration 66d632cbf187 -->
### 2. Protección del inicio de sesión y del registro

| | |
| --- | --- |
| Fines | Prevenir abusos, encontrar cuentas de una misma persona durante la moderación, limitar la frecuencia, detener bots |
| Interesados | Personas con una cuenta, solicitantes, visitantes de las páginas de inicio de sesión |
| Datos personales | Dirección IP de registro, dirección IP y User-Agent del inicio de sesión, dirección IP (y, en las solicitudes de restablecimiento de contraseña, la dirección de correo electrónico) en los contadores de límite de frecuencia, bloqueos de registro (dominio de correo electrónico, dirección con hash, rango de IP). Con CAPTCHA: dirección IP, información del navegador y del dispositivo, cookies. Con una revisión de registros: la dirección de correo electrónico y la dirección IP de cada solicitante sin invitación |
| Destinatarios | Alojamiento, personal con `manage_accounts` (cada revelación queda en el registro de auditoría), servicio de CAPTCHA, servicio de revisión de registros |
| Supresión | Direcciones IP de registro `{{180 days}}`, orígenes de inicio de sesión `{{180 days}}`, contadores en el plazo de dos veces la ventana del límite, bloqueos de registro 30 días después de levantarse o caducar. Se detiene con una retención legal |
| Base jurídica | {{p. ej. intereses legítimos}} |

<!-- source: 3-publishing-and-social-features 5e271f0279f9 -->
### 3. Publicación y funciones sociales

| | |
| --- | --- |
| Fines | Publicar perfiles y publicaciones, y las interacciones que las personas piden |
| Interesados | Personas con una cuenta, personas a las que mencionan |
| Datos personales | Perfil y sus versiones, publicaciones y sus versiones, archivos multimedia, favoritos, reacciones, impulsos, marcadores, publicaciones fijadas, votos en encuestas, seguimientos, bloqueos, silenciados, listas. Las publicaciones pueden contener cualquier dato que una persona escriba, incluidas categorías especiales. `{{IF post review}}` El texto de una publicación rechazada que su autor envía con una apelación |
| Destinatarios | Alojamiento, CDN, servicio de revisión de archivos multimedia (una pequeña copia procesada de cada subida), otros usuarios, la web pública para el contenido público, servidores federados (actividad 4), motores de búsqueda y rastreadores de IA según `search_indexing` y `ai_usage`, `{{the operator of the full-text search engine, if another company runs it}}`, aplicaciones que la persona autorizó, `{{IF post review}}` el servicio de revisión de publicaciones para las publicaciones `public`, `local` y `unlisted` de cuentas nuevas y sus ediciones (su texto tal como se escribió y tal como se muestra, advertencia de contenido, idioma, marca de contenido sensible, opciones de encuesta, descripciones de imágenes, etiquetas y número de menciones, su dirección y las direcciones de las publicaciones a las que responden o que citan, la dirección y el nombre de usuario del autor, y cuándo se creó la cuenta; sin archivos multimedia), `{{IF profile review}}` el servicio de revisión de perfiles para los cambios de perfil de cuentas nuevas (el nombre visible, la biografía tal como se escribió y tal como se muestra, los campos del perfil, si cambió el avatar o la cabecera, la dirección y el nombre de usuario de la cuenta, y cuándo se creó la cuenta; sin imágenes) |
| Supresión | Cuando la persona la elimina, por su política de eliminación automática, o al suprimir la cuenta. `{{IF auto_delete_max_age_days}}` Como máximo `{{auto_delete_max_age_days}}` días después de la publicación, para las cuentas sin `keep_posts_longer`. Subidas adjuntas a nada `{{24 hours}}`. Versiones antiguas de imágenes después de que una versión más reciente las sustituya `{{30 days}}`. El registro del URI de una publicación eliminada permanece. `{{IF post review}}` Una apelación se va con su decisión (`hook_decisions_retention_days`, conservada mientras puede apelarse o espera revisión) o al suprimir la cuenta |
| Base jurídica | {{p. ej. contrato}} |

<!-- source: 4-federation 795a56004c37 -->
### 4. Federación

| | |
| --- | --- |
| Fines | Entregar la actividad local a otros servidores y recibir la de ellos |
| Interesados | Personas con una cuenta, personas de otros servidores |
| Datos personales | Enviados: documento del actor, publicaciones, seguimientos, bloqueos, impulsos, «me gusta», reacciones, votos en encuestas, citas, migraciones, comentarios de denuncia reenviados (sin el denunciante), cambios del ciclo de vida. Conservados: copias de las actividades enviadas y recibidas, intentos de entrega |
| Destinatarios | Servidores federados, relés, servicio de revisión de seguimientos, servicio de revisión de perfiles |
| Supresión | Aquí: recibidas 30 días y sus documentos 7 días. Enviadas 30 días, y la entrega de una publicación hasta 30 días después de que se elimine la publicación o su autor. Allí: no está bajo tu control. Se envía `Delete`, y se espera, pero no se obliga, a que otros servidores lo sigan |
| Base jurídica | {{p. ej. contrato, intereses legítimos}} |

<!-- source: 5-storing-data-of-people-on-other-servers 3708a84995bb -->
### 5. Almacenamiento de datos de personas de otros servidores

| | |
| --- | --- |
| Fines | Mostrar perfiles y publicaciones remotos a los usuarios locales |
| Interesados | Personas de otros servidores, que normalmente no tienen ninguna relación contigo |
| Datos personales | Documentos de actor, versiones de perfil, identificadores, publicaciones, referencias a archivos multimedia, copias de archivos multimedia, interacciones |
| Destinatarios | Alojamiento, CDN, usuarios locales, páginas públicas, `{{IF external review of posts}}` la revisión externa de publicaciones de otros servidores para las publicaciones y revisiones que se relacionan con una cuenta local, incluidas las publicaciones solo para seguidores (`private`), y las publicaciones directas solo cuando una cuenta local sigue al autor, salvo que `hook_inbound_status_unfollowed_direct` esté activado (su texto e idiomas, advertencia de contenido, marca de contenido sensible, hora de publicación, dirección y URL, la dirección de la publicación a la que responden, opciones de encuesta, etiquetas, direcciones y descripciones de los adjuntos, el autor y su servidor, la visibilidad, el número de cuentas nombradas y de las locales entre ellas, y si una cuenta local sigue al autor) |
| Supresión | Publicaciones `{{90 days}}` cuando nada de aquí se refiere a ellas, `{{7 days}}` cuando solo las trajo un relé, y `{{no limit}}` para cualquier otra publicación. Copias de archivos multimedia `{{not kept}}`. Comprobaciones de identificador 90 días. Todo lo de una cuenta 7 días después de que su servidor la elimine, o hasta 30 días cuando solo lo indicó un `410 Gone` y el servidor no vuelve a responder. Su URI y su dominio permanecen |
| Base jurídica | {{p. ej. intereses legítimos}} |

<!-- source: 6-notifications ad34beea6ab2 -->
### 6. Notificaciones

| | |
| --- | --- |
| Fines | Informar a las personas de la actividad y de su cuenta, por correo electrónico y Web Push |
| Interesados | Personas con una cuenta, personas nombradas en una notificación |
| Datos personales | Notificaciones y marcas de lectura, configuración de notificaciones, dirección de correo electrónico, el texto de cada correo, registros de rebotes y quejas, dirección y claves de Web Push, suscripciones a publicaciones |
| Destinatarios | Alojamiento, proveedor de correo, servicios push (el cuerpo del mensaje va cifrado y lo conservan hasta 48 horas) |
| Supresión | Al suprimir la cuenta, o con la publicación a la que se refieren. Web Push: también cuando la persona o el servicio push lo termina. El proveedor de correo: {{SU PLAZO}} |
| Base jurídica | {{p. ej. contrato, intereses legítimos}} |

<!-- source: 7-moderation-and-audit d72a42b6312e -->
### 7. Moderación y auditoría

| | |
| --- | --- |
| Fines | Gestionar denuncias, acciones y apelaciones, demostrar lo que hizo el personal, retenciones por solicitudes legales |
| Interesados | Personas denunciadas, denunciantes, personal, personas nombradas en un comentario o una nota |
| Datos personales | Denuncias (comentario, publicaciones, reglas, notas, asignaciones, resoluciones), acciones (motivo, nota, publicaciones), apelaciones, registro de auditoría (actor, acción, el nombre del objetivo), retenciones y su motivo, publicaciones conservadas como prueba. Pueden incluir datos penales o categorías especiales si una denuncia o una publicación los contiene |
| Destinatarios | Alojamiento, personal con el permiso correspondiente, la persona afectada a través de sus propios registros de moderación (nunca la identidad del denunciante), el servidor de la cuenta denunciada para una denuncia reenviada |
| Supresión | Denuncias `{{365 days}}` desde la última resolución, o con una acción que se refiera a ellas. Acciones y apelaciones `{{365 days}}` desde el fin de la acción. Registro de auditoría `{{730 days}}`. Las denuncias abiertas y las acciones sin fin permanecen. Una retención liberada se elimina tras el plazo de las acciones |
| Base jurídica | {{p. ej. intereses legítimos, obligación legal}} |

<!-- source: 8-requests-from-people 762c1e8bc31c -->
### 8. Solicitudes de las personas

| | |
| --- | --- |
| Fines | Responder a solicitudes de acceso y portabilidad, e importaciones |
| Interesados | Personas con una cuenta |
| Datos personales | Archivos de exportación (actor, publicaciones, registros de moderación propios), archivos CSV, importaciones y sus resultados, la correspondencia de una solicitud |
| Destinatarios | Alojamiento, la persona |
| Supresión | Archivos e importaciones 7 días. Correspondencia: {{PLAZO}} |
| Base jurídica | {{p. ej. obligación legal, contrato}} |

La exportación propia del usuario no contiene direcciones de correo electrónico, direcciones IP, notificaciones ni preferencias. Para una respuesta completa, usa `my admin account export-moderation` para los registros de moderación y [revelar](moderation.md#account-management) para las direcciones de correo electrónico e IP, y lee el resto de la base de datos. La exportación de moderación omite la identidad de los denunciantes: comprueba que un comentario no revela ninguna antes de entregarlo.

<!-- source: 9-voice-chat e0bf1be42da8 -->
### 9. Chat de voz

Elimina esta actividad cuando `voice = off`.

| | |
| --- | --- |
| Fines | Gestionar salas de chat de voz y moderarlas |
| Interesados | Anfitriones y participantes |
| Datos personales | Nombre de la sala, descripción, anfitriones y cómo terminó la sala; quién participó, habló, levantó la mano, fue invitado o fue expulsado. Las direcciones IP y el audio pasan por el servidor multimedia de voz o el servidor TURN, o llegan directamente a otros participantes con `voice = p2p`. El servidor no graba audio |
| Destinatarios | Alojamiento, servidor multimedia de voz, STUN y TURN, participantes |
| Supresión | Participantes `{{90 days}}` desde el fin de la sala, o mientras dure una denuncia sobre ella. Las salas permanecen, con el nombre y la descripción vaciados al suprimir al autor. En los servicios: {{SU PLAZO}} |
| Base jurídica | {{p. ej. contrato, intereses legítimos}} |

<!-- source: 10-supporter-payments 99d64d001780 -->
### 10. Pagos de mecenas

Elimina esta actividad cuando `payments = none`.

| | |
| --- | --- |
| Fines | Cobrar suscripciones, conceder roles, contabilidad |
| Interesados | Suscriptores |
| Datos personales | ID de objetos del proveedor, importes, períodos, estados, reembolsos, contracargos, la elección de aparecer en la lista. Dirección de correo electrónico enviada al proveedor. Ningún número de tarjeta, últimos cuatro dígitos ni fecha de caducidad |
| Destinatarios | Alojamiento, proveedor de pagos, personas que pueden ver la lista de mecenas, personal con el permiso correspondiente |
| Supresión | Recibos de webhook 45 días. El vínculo con el cliente y la aparición en la lista, al suprimir la cuenta. Los registros de suscripciones y pagos no tienen un fin automático: {{PLAZO Y LA LEY QUE LO EXIGE}}. En el proveedor: {{SU PLAZO, Y SI ELIMINAS AL CLIENTE}} |
| Base jurídica | {{p. ej. contrato, obligación legal}} |

<!-- source: 11-running-the-service 9c89577f9cee -->
### 11. Funcionamiento del servicio

| | |
| --- | --- |
| Fines | Encontrar fallos, restaurar tras un fallo, conectar tus propias herramientas |
| Interesados | Todos los anteriores |
| Datos personales | Registros (rutas que pueden nombrar un nombre de usuario o una publicación, texto de errores), registros del proxy con direcciones IP, informes de errores depurados por patrón, registros de trabajos que contienen ID, cuerpos de webhooks, copias de seguridad de todo |
| Destinatarios | Alojamiento, proxy inverso, rastreador de errores, servicio de métricas y trazas, receptores de webhooks, almacenamiento de copias de seguridad |
| Supresión | Trabajos fallidos y registros de ejecución `{{30 days}}`. Mensajes de webhook 14 días. Recuentos horarios de errores 90 días. Registros de la aplicación y del proxy: {{PLAZO}}. El último informe de cada grupo de errores: lo sustituye el siguiente, sin fin automático. Copias de seguridad: {{PLAZO}}. En el rastreador de errores y en los receptores: {{SU PLAZO}} |
| Base jurídica | {{p. ej. intereses legítimos}} |

<!-- source: 12-translating-posts 17a9b7a81356 -->
### 12. Traducción de publicaciones

Elimina esta actividad cuando `translation_providers` esté vacío.

| | |
| --- | --- |
| Fines | Traducir una publicación cuando un lector lo pide, {{IF translation of reports}} traducir una denuncia cuando un moderador lo pide, y mantener la traducción dentro de sus límites |
| Interesados | Autores de publicaciones, aquí y en otros servidores, y personas a las que nombra el texto. Lectores, por el recuento de caracteres. {{IF translation of reports}} Denunciantes, por el texto de sus comentarios |
| Datos personales | Lo que contenga el texto de una publicación: su cuerpo, advertencia de contenido, opciones de encuesta y descripciones de imágenes. La dirección de la publicación, su autor, el lector, las menciones y los enlaces se sustituyen u omiten antes del envío. Conservados: las traducciones, y los caracteres por cuenta y mes |
| Destinatarios | Proveedores de traducción, como tus encargados del tratamiento: publicaciones `public`, `local` y `unlisted` de forma predeterminada. {{IF translation to private and direct}} También publicaciones `private` y `direct`, para {{LOS PROVEEDORES QUE PERMITISTE}}. {{IF translation of reports}} Los comentarios de las denuncias, para los proveedores con permiso para publicaciones `private`, y las publicaciones que nombran las denuncias, cuando un moderador lo pide |
| Supresión | Traducciones `{{30 days}}` desde la traducción, y con la publicación cuando se elimina. Caracteres por cuenta hasta el final del mes siguiente, o al suprimir la cuenta. En los proveedores: lo que se envió no puede retirarse. {{SUS PLAZOS}} |
| Base jurídica | {{p. ej. intereses legítimos}} |

<!-- source: security-measures-article-301g 756bb9e562e3 -->
### Medidas de seguridad (artículo 30(1)(g))

Lo que hace el software:

- Las contraseñas se almacenan como hashes Argon2id. Los tokens de sesión, de acceso y de un solo uso, y los códigos de recuperación, se almacenan como hashes.
- Las claves privadas de firma, los secretos de autenticación en dos pasos y los secretos de proveedores y de webhooks se cifran con AES-256-GCM bajo una clave de cifrado de claves guardada fuera de la base de datos ([gestión de claves](key-management.md)).
- Las direcciones de correo electrónico y las IP se muestran solo al personal con `manage_accounts` mediante una acción explícita que registra el registro de auditoría.
- Las solicitudes de inicio de sesión, registro y restablecimiento de contraseña tienen límite de frecuencia. Los permisos del personal pasan por roles.
- Las solicitudes salientes que los servidores remotos pueden influir pasan por una protección contra el acceso a direcciones internas.

Lo que haces tú:

- TLS en el proxy inverso: {{CONFIGURACIÓN}}
- Cifrado en reposo de la base de datos, del almacenamiento de blobs y de las copias de seguridad: {{MEDIDAS}}
- Quién puede acceder al host, a la base de datos y a la KEK, y cómo se revisa el acceso: {{MEDIDAS}}
- Cuentas del personal, roles y autenticación en dos pasos: {{POLÍTICA}}
- Actualizaciones del servidor y de su host: {{POLÍTICA}}
- Gestión de una violación de la seguridad de los datos personales: {{PROCEDIMIENTO Y CONTACTO}}
