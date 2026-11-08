<!-- source: supporters-paid-plans ea0039323475 -->
# Mecenas (planes de pago)

Solo cuando un administrador lo activa, los usuarios locales pueden suscribirse a un plan de pago en la web y conservar el rol vinculado al plan mientras sigan pagando. El programa está desactivado por defecto y no aparece ningún punto de entrada a planes ni suscripciones hasta que se activa.
La clave de despliegue `payments` se describe en la [referencia de configuración](../reference/configuration.md#payments).

**mycroblog no garantiza el cumplimiento de ninguna ley, incluida la Ley japonesa de Transacciones Comerciales Específicas.** Los operadores deben comprobar el contenido de la divulgación comercial, la pantalla de confirmación final y el tratamiento del impuesto sobre el consumo y de las facturas conforme a las leyes de su propio país (consulta «Qué deben comprobar los operadores» más abajo).

<!-- source: how-a-subscription-works 13d7fb3772b2 -->
## Cómo funciona una suscripción

![Flujo de una suscripción: el usuario paga en el proveedor de pagos, los eventos del proveedor hacen que el servidor lea el estado de vuelta, y el servidor concede o retira el rol del plan y registra el ingreso. Los pasos se enumeran a continuación.](images/supporter-flow.svg)

De arriba abajo en el diagrama:

- **Suscripción.** El usuario elige un plan y un intervalo (mes o año) en `/settings/supporter`. Tras la confirmación final, el servidor (`my serve`) crea la página de pago en el proveedor indicado en `payments` (`stripe` o `payjp`), y el usuario paga en esa página.
- **Eventos.** El proveedor envía sus eventos al endpoint de webhook `POST /payments/webhooks/{provider}`, que registra cada evento una sola vez y encola un trabajo. Un evento es solo una señal: el servidor nunca toma el estado de su contenido.
- **Lectura de vuelta.** El trabajo `supporters.apply_payment_event`, y la conciliación `supporters.reconcile_payments` cada `payments_reconcile_interval`, leen de vuelta la suscripción y los movimientos de dinero desde la API del proveedor. La conciliación repara cualquier evento que no haya llegado.
- **El estado y el rol.** El servidor deriva el estado de la suscripción a partir de lo que ha leído y de cada plazo vencido (por ejemplo, el período de gracia de 7 días), y con ello concede o retira el rol del plan.
- **Ingresos.** Los movimientos de dinero se convierten en registros de ingresos (importe bruto, comisión e importe neto de cada pago, reembolso, contracargo y comisión). Sus totales se muestran en la pantalla de finanzas (`/finance`, para quienes tienen `view_finance`) y, en la medida que permita `income_visibility`, en `/about/costs` (consulta [instance-settings.md](instance-settings.md)).

![Los estados de una suscripción: active, canceling y past_due conservan el rol del plan; pending, lapsed, expired, revoked y abandoned no. Las transiciones se enumeran a continuación.](images/supporter-states.svg)

Los estados de una suscripción, tal como los muestran la administración y `my admin supporters subscriptions`:

- `pending`: iniciada, aún sin pagar. Sin rol. El primer pago la pasa a `active`, y una suscripción que termina antes de su primer pago pasa a `abandoned`.
- `active`: pagada. Conserva el rol del plan, que cada renovación mantiene.
- `past_due`: un pago ha fallado. Conserva el rol durante 7 días desde el fallo; un pago en ese tiempo la devuelve a `active`.
- `lapsed`: han pasado 7 días desde el fallo. Se retira el rol. Un pago posterior la devuelve a `active` y el rol vuelve.
- `canceling`: cancelada al final del período. Conserva el rol hasta entonces y pasa a `expired`.
- `expired`, `revoked` (un reembolso total o un contracargo) y `abandoned` son finales y no conservan ningún rol.

<!-- source: enabling-the-program fe86b2e59df5 -->
## Activar el programa

1. Establece la clave de despliegue `payments` con tu proveedor de pagos y pasa las claves del proveedor y el secreto del webhook. Los pasos específicos de cada proveedor están en [payments-stripe.md](payments-stripe.md) y [payments-payjp.md](payments-payjp.md). El programa no se puede activar mientras la clave sea `none`.
2. Configura el servidor para enviar correo electrónico (`mail` distinto de `none`). Solo los usuarios con una dirección de correo electrónico confirmada pueden suscribirse, así que un servidor que no envía correo no puede activar el programa.
3. En la página «Divulgaciones comerciales» de «Términos y políticas» en la administración (`/admin/documents/commerce-disclosure`), escribe en Markdown la divulgación comercial (para Japón, el aviso exigido por la Ley de Transacciones Comerciales Específicas), el texto de instancia `commerce_disclosure`. Se publica en `/about/commerce`, y puedes registrar esa URL en el panel del proveedor.
   El programa se puede activar con el texto vacío, pero la administración muestra una advertencia.
4. En «Roles» en la administración, crea el rol que vincular al plan. No debe tener titulares, solo permisos de miembro (grupo `member`), y una `position` inferior a la de cualquier rol del personal, `moderator` incluido.
   Los permisos de miembro son las ventajas que puede dar un plan: `create_custom_emojis` (solicitar emojis personalizados), `view_finance` (leer las finanzas del servidor), `start_voice_rooms` (abrir salas de chat de voz mientras solo pueden hacerlo los usuarios con permiso), `create_invites` (invitar a personas mientras solo pueden hacerlo los usuarios con permiso, dentro de los límites de las [invitaciones de miembros](registration.md#member-invites)) y `keep_posts_longer` (conservar publicaciones más allá del [máximo del borrado automático](instance-settings.md#automatic-deletion-of-old-posts), y desactivar el borrado automático mientras el máximo esté activo).
   Cuando la suscripción termina, el rol desaparece y con él las ventajas: las invitaciones que hizo el mecenas dejan de funcionar hasta que se suscriba de nuevo, y el máximo del borrado automático vuelve a aplicarse a sus publicaciones tras su período de gracia.
5. En «Mecenas» en la administración (permiso `manage_supporters`), abre la página «Planes» (`/admin/supporters/plans`) y elige «Crear un plan». Define el nombre, la descripción, el precio mensual, el descuento por pago anual, el límite mensual de creación de emojis personalizados (se restablece el primer día de cada mes, UTC; consulta [emojis personalizados](instance-settings.md#custom-emoji)), el orden y la visibilidad, y elige el rol.
   El servidor crea los precios mensual y anual en el proveedor (no crees precios en el panel del proveedor). Hasta que existan los precios, el plan está «en preparación» y no acepta suscripciones.
6. En la página «Programa» de «Mecenas» (`/admin/supporters`), marca «Activar el programa de mecenas» y elige «Guardar». El mismo formulario define quién puede leer la lista de mecenas.

Una vez que existe un plan, su rol ya no se puede conceder ni revocar a mano, y sus permisos y posición se cambian en la página del plan (elige el plan en la página «Planes»). Un plan superior (un orden mayor) queda por encima de los roles de los planes inferiores, tiene todos sus permisos y un límite de emojis al menos igual al de ellos.

<!-- source: setup-at-the-provider 74efcb74cf80 -->
## Configuración en el proveedor

- **Desactivación**: un suscriptor no puede desactivar su cuenta mientras la suscripción esté vigente, incluida una programada para terminar al final de su período. La pantalla de configuración lo lleva a la suscripción, para que nadie sea cobrado estando oculto.
- **Reintentos (dunning)**: siete días después de que falle un pago, el servidor retira el rol (con independencia de los reintentos del proveedor). Establece el período de reintentos del proveedor en al menos 7 días y haz que la acción final sea «cancelar». Si el pago se realiza con éxito después del período de gracia, el rol vuelve.
- **Portal de clientes**: el portal se abre para métodos de pago y facturas. Desactiva los cambios de plan en el portal y permite solo la cancelación al final del período. Los cambios de plan y las cancelaciones se hacen en las pantallas del propio servidor.
- **Reembolsos**: el servidor reembolsa solo un pago que no compró nada: una segunda suscripción de un mismo pago y un pago de una suscripción pendiente que se cerró. La guía del proveedor ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) describe ambos. Cualquier otro reembolso corresponde al operador, en el panel del proveedor. Un reembolso total y un contracargo retiran el rol de inmediato y cancelan de inmediato la suscripción del proveedor. Un reembolso parcial no retira el rol. Ganar un contracargo no restablece el rol; el usuario se suscribe de nuevo.

<!-- source: what-users-see d1216c59368a -->
## Lo que ven los usuarios

- En la web, el usuario abre «Planes de apoyo» (`/settings/supporter`), elige un plan y un intervalo de pago (mes o año), pasa por una pantalla de confirmación final y continúa a la página de pago del proveedor. Los datos de la tarjeta nunca pasan por este servidor. La página de pago vuelve a esta pantalla, que entonces lee de nuevo el estado de la suscripción.
  Mientras está suscrito, el usuario puede cambiar de plan desde la misma pantalla (una mejora de plan y un cambio de mensual a anual se aplican una vez pagada su diferencia prorrateada, mientras que una rebaja de plan y un cambio de anual a mensual se aplican al final del período), cancelar un cambio programado, cancelar la suscripción (las ventajas duran hasta el final del período) y deshacer una cancelación, y abrir el portal de clientes para métodos de pago y facturas.
  Cuando falla el pago de un cambio, o la tarjeta pide una autenticación que el cambio no puede solicitar, el plan se mantiene y el usuario ve un error con la forma de actualizar la tarjeta. No inicia el período de gracia ni envía la notificación de pago fallido; una renovación fallida sí lo hace.
- Solo pueden suscribirse los usuarios de este servidor que tengan una dirección de correo electrónico confirmada y una cuenta que no esté suspendida ni programada para su eliminación. Un usuario puede tener un plan a la vez.
- `/about` muestra una sección con los planes públicos. En la aplicación Tauri no aparecen la pantalla de suscripción, la sección de planes de `/about` ni los enlaces a ellas. Las ventajas (insignia, emojis personalizados, finanzas) también funcionan en la aplicación.

<!-- source: notifications-to-subscribers 91f6ad77e546 -->
## Notificaciones a los suscriptores

Se avisa a los suscriptores una sola vez de cada uno de los siguientes hechos, mediante notificación, Web Push y correo electrónico (a la dirección principal, en el idioma del usuario). A un suscriptor cuya cuenta está desactivada no se le avisa de nada, ni entonces ni al volver.

- Fallo de pago (`supporter_payment_failed`): cuando la suscripción pasa a vencida (`past_due`). Indica el final del período de gracia (en UTC en el correo) y un enlace a la pantalla donde se puede corregir el método de pago (`/settings/supporter`).
- Fin de la suscripción (`supporter_subscription_ended`): cuando una suscripción sale de un estado con ventajas porque se agotó el período de gracia, expiró, o terminó con un reembolso o un contracargo. El mensaje indica el motivo. No se avisa de un fin causado por eliminar la cuenta.
- Retirada del plan (`supporter_plan_retired`): a los suscriptores de un plan retirado (salvo los pendientes). Indica que la suscripción no se renovará.
- Anuncio del fin del servicio (`supporter_service_closure`): a los suscriptores cuya suscripción estaba cancelada al final de su período cuando empezó el anuncio (salvo los pendientes). Indica que la suscripción no se renovará. Durante el anuncio no se aceptan nuevas suscripciones.

El texto de Web Push y de las notificaciones no lleva a la pantalla de pago. Solo el cliente web pone en una notificación un enlace a la pantalla de suscripción; la aplicación Tauri no. Los enlaces de los correos abren el cliente web en un navegador.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` realiza parte de las operaciones de la administración (`my admin supporters -help`).

- `status`: si el programa está activado, qué necesita antes de poder activarse (un proveedor, correo electrónico) y el número de suscripciones en curso.
- `enable`, `disable`: activan y desactivan el programa. La desactivación se rechaza mientras haya alguna suscripción en curso.
- `plans`: la lista de planes (incluidos los retirados), con el número de planes en preparación y retirados y de suscripciones.
- `subscriptions`: la lista de suscripciones (filtra con `-plan` y `-state`). `cancel <subscription ID>` cancela una suscripción de inmediato (sin reembolso, salvo un pago de una suscripción pendiente, que no compró nada).

Activar, desactivar y cancelar se escriben en el registro de auditoría como operaciones de la CLI. Crear, cambiar y retirar planes se hace en la administración o mediante la API de administración.

<!-- source: the-list-of-members f00ca3b391a3 -->
## La lista de miembros

- El ajuste de instancia `supporter_list_visibility` (en la pantalla «Mecenas»; consulta [instance-settings.md](instance-settings.md)) elige quién puede ver la lista de suscriptores actuales (`/about/supporters`). Con el valor por defecto `off`, no aparecen ni la lista ni el enlace desde `/about`.
- La lista muestra solo a los suscriptores que eligieron aparecer en la página «Insignias y lista de mecenas» de su configuración (por defecto nadie aparece). Muestra el nombre del plan y el avatar, el nombre visible y el identificador de cada persona, y nunca importes, si el pago es mensual o anual, fechas ni recuentos.
- La elección de aparecer se mantiene cuando el suscriptor cambia de plan y desaparece cuando la suscripción termina (se retira el rol). Quien se suscribe de nuevo empieza sin aparecer.
- La página de la lista y su API envían siempre `noindex, noarchive`, sea cual sea el ajuste de motores de búsqueda (`search_indexing`).

<!-- source: changing-prices-hiding-plans-and-retiring-plans a65c92b1c371 -->
## Cambiar precios, ocultar planes y retirar planes

- Cuando cambias el precio mensual o el descuento, el nuevo precio se aplica a las nuevas suscripciones y a los cambios de plan. Los suscriptores existentes renuevan a su precio original. No hay ninguna operación para pasar a los suscriptores existentes a un precio nuevo. Para hacerlo, crea un plan nuevo y retira el antiguo.
- Ocultar un plan detiene las nuevas suscripciones y los cambios a ese plan. Las suscripciones existentes continúan.
- Retirar un plan no se puede deshacer. Sus suscripciones se cancelan al final de su período (las pendientes se cierran de inmediato y su página de pago deja de aceptar pagos, y se reembolsa cualquier pago que aún llegue; y las que tienen el pago atrasado más allá del período de gracia se cancelan de inmediato). Cuando termina la última suscripción, el rol vuelve a ser un rol ordinario.

<!-- source: accounts-and-subscriptions 8c5e10df3c18 -->
## Cuentas y suscripciones

- Eliminar una cuenta y expulsarla (lo que programa su eliminación) cancelan de inmediato sus suscripciones, sin reembolso (un pago de una suscripción aún pendiente no compró nada, y se reembolsa). Revertir una expulsión no restablece las suscripciones.
- Suspender una cuenta programa la cancelación al final del período. Levantar la suspensión no la restablece automáticamente; el usuario cancela la cancelación antes del final del período o se suscribe de nuevo.
- En la página «Suscripciones» de «Mecenas» (`/admin/supporters/subscriptions`), un administrador puede abrir una suscripción y terminarla de inmediato con «Terminar ahora» (sin reembolso, salvo un pago de una suscripción pendiente, que no compró nada; se escribe en el registro de auditoría).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Desactivar el programa y cambiar de proveedor

- El programa no se puede desactivar mientras haya alguna suscripción en curso (pendiente, activa, programada para cancelarse, vencida o pasado el período de gracia). Retira primero todos los planes y desactiva el programa cuando las suscripciones hayan terminado.
- Para cambiar de proveedor, desactiva el programa, cambia `payments` y vuelve a activar el programa con el nuevo proveedor. Los suscriptores existentes se suscriben de nuevo. Mientras el proveedor configurado difiera de aquel con el que se activó el programa, el servidor rechaza nuevas suscripciones y cambios, y la administración muestra una advertencia.

<!-- source: what-operators-must-check-not-yet-verified 659a7e51f9d3 -->
## Qué deben comprobar los operadores (aún sin verificar)

mycroblog no juzga ninguno de estos puntos. Compruébalos conforme a las leyes de tu país y los términos de tu proveedor.

- Los elementos que debe contener la divulgación comercial (nombre, dirección y teléfono del vendedor, el precio, cuándo y cómo pagar, cuándo se presta el servicio, cómo cancelar, etc.) y cómo debe publicarse.
- Que la pantalla de confirmación final (artículo 12-6 de la Ley de Transacciones Comerciales Específicas) muestre la cantidad, el precio, el momento del pago, el momento de la entrega y la forma de cancelar y su efecto, tomando conjuntamente la página de pago del proveedor y la pantalla de confirmación de `my`.
- El tratamiento del impuesto sobre el consumo y si emites facturas cualificadas.
