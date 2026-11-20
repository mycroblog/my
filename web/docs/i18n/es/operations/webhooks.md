<!-- source: server-webhooks e73d0e0966b7 -->
# Webhooks del servidor

Los webhooks envían denuncias, solicitudes de registro, creaciones de cuentas y cambios de perfil, acciones de
moderación, apelaciones, acciones sobre dominios, publicaciones públicas locales y acciones de gestión de emoji
personalizados a un relé de Slack o Discord, a un sistema de tickets o a una herramienta de automatización como n8n. Un
servidor sin webhooks configurados no guarda filas adicionales ni ejecuta trabajos adicionales. Lo que recibe un
receptor (encabezados, firma, eventos, cuerpo) se describe en la [referencia de webhooks](../reference/webhooks.md).

<!-- source: set-up-a-webhook 20c889b84876 -->
## Configurar un webhook

Cualquiera con el permiso `manage_webhooks` puede configurar webhooks en la pantalla de administración «Webhooks» (`/admin/webhooks`).
El rol integrado `admin` lo tiene. «Configurar un webhook» abre el formulario; al elegir un webhook de la lista se abre su página, donde
lo pruebas, lo desactivas y activas, lo cambias, ves sus entregas de los últimos 14 días y reenvías una, le das un nuevo
secreto y lo eliminas tras escribir el nombre de host de su URL. La página dice cuándo las entregas siguen fallando y cuándo
el servidor desactivó el webhook. El historial muestra el evento, el estado, los intentos y el último intento de cada entrega, y el cuerpo
que envió, que puede contener datos personales y se muestra solo a quien también tiene el permiso que necesita su evento (abajo).

1. Crea un webhook con una URL de destino, una descripción y los eventos a los que suscribirse. Cada evento también necesita su propio
   permiso además de `manage_webhooks`: `manage_reports` para denuncias, `manage_accounts` para registros, cuentas,
   acciones de moderación y apelaciones, `manage_domains` para acciones de dominio, `manage_custom_emojis` para emoji personalizados, y
   `manage_errors` para grupos de errores.
   Las publicaciones públicas no necesitan ningún permiso adicional. No puedes seleccionar un evento cuyo permiso no tienes.
2. El secreto (`whsec_...`) se muestra una vez, cuando se crea el webhook. Defínelo en el receptor. No puede mostrarse de nuevo.
3. En la página del webhook, elige «Enviar una prueba» para enviar `webhook.test`, y comprueba en sus entregas que llegó.

Crear, cambiar, eliminar, activar, desactivar y rotar el secreto de un webhook se anotan en el registro de auditoría.
El destino se muestra como la descripción y el nombre de host de la URL; la URL en sí no se anota.

<!-- source: rotate-the-secret fc0bedd73653 -->
## Rotar el secreto

«Crear un secreto nuevo» crea un secreto nuevo y lo muestra una vez. Durante las 24 horas siguientes los mensajes también se firman con el secreto antiguo
(`webhook-signature` lleva dos firmas), así que puedes cambiar el receptor al secreto nuevo en esa ventana.
Los secretos se sellan con `kernel.Sealer` antes de almacenarse en la base de datos; la clave es la
[KEK](key-management.md).

<!-- source: failures-and-redelivery 65aba0d2089e -->
## Fallos y reenvío

- Si el destino responde con algo distinto de `2xx`, no responde en 15 segundos o no se puede alcanzar, el mensaje se reintenta a los 5 segundos, 5 minutos, 30 minutos, 2 horas, 5 horas, 10 horas y 10 horas (8 intentos en total, unas 27 horas). Una entrega que los ha agotado todos se convierte en un trabajo muerto, y «Enviar de nuevo» en las entregas la envía otra vez con el mismo `webhook-id`.
- Si el destino responde `410 Gone`, el webhook se desactiva de inmediato.
- El servidor desactiva un destino que no ha tenido éxito en 72 horas desde su primer fallo y ha fallado 20 veces seguidas o más. El registro de auditoría recibe una entrada `webhook_disabled` de `system`, y todas las personas con `manage_webhooks` reciben una notificación en la aplicación (y un correo electrónico, si eligieron recibir las notificaciones por correo electrónico).
- La lista de la pantalla de administración muestra el número de fallos seguidos y el motivo por el que el servidor desactivó un webhook (`gone` o `failing`). Vuelve a activar el webhook después de arreglar el destino. Los mensajes del tiempo en que estuvo desactivado no se envían de nuevo.
- Los mensajes y las entregas se eliminan a los 14 días (fijo). Los cuerpos contienen datos personales, como el comentario de una denuncia.

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## Destinos en una red interna

La entrega pasa por la protección SSRF del HTTP saliente. De forma predeterminada solo llega a direcciones públicas por
`https` (puertos 443 y 80). Para enviar a un destino interno, como un n8n en el mismo host o un chat dentro de tu VPC,
enumera nombres de host, direcciones IP y CIDR, separados por comas, en el ajuste `webhook_allowed_destinations`
(variable de entorno `MY_WEBHOOK_ALLOWED_DESTINATIONS`; consulta la [referencia de configuración](../reference/configuration.md#webhook_allowed_destinations)).

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- Solo los destinos enumerados, y solo para la entrega de webhooks, quedan exentos de la protección. La federación, el contenido multimedia y las vistas previas de enlaces no cambian. Un destino exento también puede usar `http` y cualquier puerto.
- Un nombre de host se permite sea cual sea la dirección a la que se resuelva. Un CIDR se permite cuando la URL indica una dirección IP de ese rango. Si usas un nombre de host que se resuelve dentro del rango, enumera también el nombre de host.
- Solo un operador con acceso al servidor puede cambiar el ajuste. Los permisos de la pantalla de administración por sí solos no pueden llegar a una red interna.
- Un intento hacia un destino que la protección rechazó muestra `forbidden_destination` en las entregas.
- El ajuste de desarrollo `http_allow_private_networks` levanta la protección para cualquier propósito, webhooks incluidos.
