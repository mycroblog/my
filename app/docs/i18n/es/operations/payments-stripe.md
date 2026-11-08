<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# Pagos de mecenas con Stripe

Con `payments = stripe`, los usuarios locales se suscriben a planes mensuales o anuales en la página de pago alojada de Stripe (Checkout), y el rol del plan se concede y se retira según cambia el estado del pago.
Las claves de despliegue se describen en la [referencia de configuración](../reference/configuration.md#payments). Para el programa en conjunto (planes, roles, la lista de miembros), consulta [supporters.md](supporters.md).
Un administrador habilita el programa y crea los planes en la administración. Esta configuración solo permite que el servidor hable con Stripe.

Un servidor solo puede usar un proveedor. Para cambiar más adelante a otro proveedor, pide a los suscriptores existentes que se suscriban de nuevo con el nuevo.

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## Qué hace el servidor en Stripe

[Cómo funciona una suscripción](supporters.md#how-a-subscription-works) muestra en un diagrama cómo encajan la página de pago, el webhook, la conciliación, el rol del plan y los ingresos.

- **Planes y precios**: para cada plan, el servidor crea un Product mediante la API (ID `my_plan_<plan ID>`, con `metadata.my_plan_id`) y un Price por intervalo y moneda (`lookup_key` es `my_<plan ID>_<month|year>_<currency>`).
  Un Price de Stripe no puede cambiar su importe, así que cambiar el importe crea un Price nuevo y archiva el anterior. Las suscripciones del Price anterior se renuevan con su importe. El descuento por pago anual se expresa en el importe del Price anual; no se usan cupones.
  No edites ni elimines estos Products y Prices en el panel.
  Un plan puede usar las monedas de la [lista de cargos mínimos de Stripe](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts), y sus importes mensual y anual deben alcanzar ese mínimo cada uno: USD, AED, ARS, AUD, BRL, CAD, CHF, COP, CZK, DKK, EUR, GBP, HKD, HUF, IDR, ILS, INR, JPY, KRW, MXN, MYR, NOK, NZD, PHP, PLN, RON, RUB, SEK, SGD, THB y ZAR. El servidor rechaza cualquier otra moneda.
  La lista vale para un cargo en la moneda de liquidación de la cuenta. Un cargo que Stripe convierte debe alcanzar el mínimo de la moneda de liquidación después de la conversión, y el servidor no lo comprueba, así que elige un importe de plan bastante por encima del mínimo cuando su moneda difiere de la de tu cuenta bancaria.
- **Suscribirse**: el servidor crea una sesión de Checkout con `mode=subscription` y envía el navegador del usuario a su URL (`checkout.stripe.com`). La página de pago caduca después de una hora.
  El ID de suscripción del servidor se guarda en `client_reference_id`, en `metadata.my_subscription_id` y en el `metadata.my_subscription_id` de la suscripción de Stripe. El cálculo automático de impuestos (Stripe Tax) no está habilitado.
- **Empezar de nuevo**: como máximo una página de una suscripción pendiente puede cobrar un pago. Un usuario que vuelve a empezar con el mismo plan e intervalo regresa a la página abierta, que se abre otra vez vacía, salvo que le queden menos de 15 minutos. Cualquier otro inicio recibe una página nueva, y el servidor hace caducar la anterior, que entonces muestra el mensaje de caducidad de Stripe. Si una página anterior ya se pagó, el usuario no recibe ninguna página: va a la pantalla de retorno, que muestra la suscripción. Si el servidor no puede hacer caducar una página anterior, el inicio falla y el usuario puede intentarlo de nuevo.
  Una suscripción pendiente que se cierra (su plan se retiró, su cuenta se eliminó) hace caducar su página del mismo modo.
- **Una segunda suscripción**: dos pagos aún pueden coincidir, cuando se envían ambas páginas en el mismo instante. La suscripción que llega primero al servidor es la del usuario. El servidor cancela la otra de inmediato y reembolsa por completo todas sus facturas pagadas, renovaciones incluidas, ya que no dio nada por ningún periodo, con el motivo `duplicate`, y registra `a second subscription was canceled and refunded`. Un pago de una suscripción pendiente que se ha cerrado se trata del mismo modo, también cuando llegó al servidor justo antes del cierre. El usuario recibe el reembolso de Stripe y ve una sola suscripción. Stripe no devuelve su comisión sobre el cargo reembolsado, y esa comisión aparece en los ingresos.
  Un cargo en disputa no se reembolsa, ni tampoco un pago que no se había liquidado cuando el servidor lo miró (un método de adeudo, por ejemplo). Una suscripción que alguien ya canceló en el panel se deja como está, con sus cargos, y nunca pasa a ser la suscripción del usuario.
  **Antes de actualizar un servidor desde una versión que no incluye esto**, busca en Stripe clientes con dos suscripciones cuyo `metadata.my_subscription_id` sea el mismo. La primera conciliación cancela la que no se usa y reembolsa todo lo que cobró. Cancela en el panel las que prefieras resolver a mano, y el servidor las deja en paz.
- **Portal del cliente**: en el primer uso, el servidor crea mediante la API una configuración de portal (`metadata.my=1`) que habilita solo actualizar los métodos de pago, el historial de facturas y la cancelación al final del periodo. Los cambios de plan se hacen en las pantallas de este servidor, no en el portal.
  Una cancelación en el portal aparece en este servidor como cancelación programada, y el usuario puede deshacerla desde la pantalla del servidor. Se rechaza un cambio de plan hasta que se deshaga.
- **Una fecha de cancelación fijada a mano**: el portal establece el `cancel_at` de la suscripción, no `cancel_at_period_end`, y el servidor lee ambos. Un `cancel_at` que fijes en el panel o mediante la API cuenta igual cuando no es posterior al final del periodo actual. Cuando es anterior a ese final, el servidor muestra esa fecha como el final de la suscripción, y el usuario conserva el rol hasta entonces. Un `cancel_at` posterior todavía no es una cancelación: la suscripción sigue activa hasta el periodo que contiene la fecha.
- **Cambios de plan**: una mejora, y un cambio de mensual a anual, factura la diferencia prorrateada de inmediato (`proration_behavior=always_invoice`) y cambia solo cuando se paga esa factura (`payment_behavior=pending_if_incomplete`). Una rebaja, y un cambio de anual a mensual, cambian al final del periodo mediante un Subscription Schedule.
- **Un cambio de plan fallido**: cuando falla el pago de un cambio o la tarjeta pide autenticación, el servidor anula esa factura, lo que descarta el cambio, y el usuario ve un error. La suscripción se queda en su plan y activa, sin periodo de gracia ni notificación de pago fallido. El usuario puede actualizar la tarjeta en el portal y volver a intentarlo, y se le cobra una vez. Si el pago aún se está procesando, el servidor lo deja, y el plan cambia cuando se completa. Las renovaciones no se ven afectadas: una renovación fallida pasa al periodo de gracia como se describe más abajo.
- **Volver a leer el estado**: el servidor trata un webhook como una señal y vuelve a leer la suscripción, la sesión de Checkout y los movimientos de dinero desde la API de Stripe para decidir el estado.
  Además, una conciliación cada `payments_reconcile_interval` (6 horas por defecto) lee la lista de eventos, las sesiones de Checkout sin terminar, las transacciones de saldo y un barrido de todas las suscripciones, y repara cualquier webhook que el servidor perdió.
- **Ingresos**: el servidor registra el importe bruto, la comisión y el neto de cada transacción de saldo. Las comisiones como el 0,7 % de Billing aparecen como transacciones `stripe_fee` separadas y se restan de los ingresos.
  `stripe_fee` abarca las comisiones de toda la cuenta, así que **usa una cuenta de Stripe dedicada a este servidor**. En una cuenta que se usa para otras ventas, esas comisiones también se restan de los ingresos.
- **Fechas**: las fechas de los ingresos se cuentan en la zona horaria de la cuenta de Stripe (la zona horaria de «Settings > Business > Account details» en el panel, `settings.dashboard.timezone` en la API). Cuando no se puede leer, el servidor cuenta en UTC y la vuelve a leer cada hora.

Cada proceso del servidor envía como máximo 20 solicitudes por segundo (10 en modo de prueba). [Los límites de Stripe](https://docs.stripe.com/rate-limits) son por cuenta: 100 solicitudes por segundo en modo real, 25 en un sandbox, y 25 por segundo para cada endpoint. Los procesos de un mismo servidor los comparten, igual que cualquier otra cosa que use la cuenta.
Cuando Stripe devuelve `429`, el servidor espera lo que diga `Retry-After` si son 30 segundos o menos (1 segundo sin la cabecera) y reintenta hasta 2 veces; más allá, deja el reintento al trabajo.

Stripe también asigna las solicitudes de lectura por cuenta: una media de 500 por transacción en 30 días, y al menos 10 000 al mes sea cual sea el número de transacciones. Con el intervalo predeterminado, la conciliación queda muy por debajo de eso en un servidor pequeño. Una conciliación que no encuentra nada aun así lee unas cuantas páginas, así que el intervalo más corto, de 15 minutos (2880 ejecuciones en 30 días), se acerca a los 10 000 en una cuenta con pocas transacciones.
Stripe limita cada suscripción a 10 facturas nuevas por minuto y 20 al día, y un cambio de plan facturado de inmediato genera una factura nueva.

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## Datos de tarjeta y PCI DSS

Los métodos de pago se introducen solo en el Checkout y en el portal del cliente de Stripe, y el cliente web de este servidor no carga ni el script de Stripe ni un iframe.
El servidor guarda solo IDs de objetos de Stripe (`cus_`, `sub_`, `price_`, `prod_`, `cs_`, `txn_`), importes, periodos y estados. No guarda números de tarjeta, últimos cuatro dígitos, fechas de caducidad ni direcciones de facturación. Los cuerpos de los webhooks y las respuestas de Stripe no se guardan ni en los registros ni en la base de datos.
Por eso, se espera que el alcance de PCI DSS del operador sea el de un comercio que usa una página de pago alojada (SAQ A). Comprueba el alcance que se aplica y el tipo de cuestionario de autoevaluación en la guía del panel de Stripe («Settings > Compliance»).

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. Preparar la cuenta de Stripe

1. Crea una cuenta de Stripe e introduce los datos de tu negocio en el panel. Si vendes en Japón, registra la URL de tu página de divulgación comercial en «Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions», y aparecerá un enlace debajo de Checkout.
2. Comprueba la zona horaria en «Settings > Business > Account details». Las fechas de los ingresos se cuentan en ella.
3. En «Settings > Billing > Subscriptions and emails > Manage failed payments», elige el comportamiento de los reintentos (Smart Retries) y qué ocurre cuando se agotan los reintentos.
   El periodo de gracia antes de retirar el rol (7 días desde el primer fallo) lo cuenta este servidor, así que las configuraciones de reintento de Stripe no lo afectan. Cuando se agotan los reintentos, se recomienda «Cancel the subscription».
4. Para enviar recibos a los suscriptores, habilita el recibo de pago correcto en «Settings > Customer emails».

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. Crear una clave de API

En «Developers > API keys», se recomienda crear una **clave restringida** (`rk_test_...` / `rk_live_...`). Una clave secreta (`sk_test_...` / `sk_live_...`) también funciona.
El prefijo de la clave establece el modo, y una clave de prueba no cobra nada. No se puede usar una clave publicable (`pk_`).

Da a una clave restringida estos permisos. Una clave restringida con los permisos de esta tabla ejecutó en un sandbox la suscripción, los cambios de plan, la cancelación, la conciliación y la anulación de facturas. Con ella no se han ejecutado los reembolsos, la caducidad de una página ni las disputas, y nadie ha comprobado que la tabla sea el conjunto mínimo que funciona.

| Recurso | Permiso | APIs usadas |
| --- | --- | --- |
| Products | Write | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Write | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | Write | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (parte de Subscriptions) | Write | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Write | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | Write | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | Read | Se leen mediante las transacciones de saldo y los cargos |
| Balance transaction sources | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account (información de una cuenta fuera de Connect) | Read | `GET /v1/account` (zona horaria) |

Sin el permiso para escribir reembolsos, el servidor igualmente cancela una segunda suscripción, y `duplicate_charge` sigue en las advertencias (consulta [Registrar el endpoint del webhook](#3-register-the-webhook-endpoint)) hasta que pueda reembolsar. Una clave creada antes de que este permiso figurara en la lista necesita que se lo añadas.

Pasa la clave en la variable de entorno `MY_PAYMENTS_STRIPE_SECRET_KEY` (o la ruta de un archivo en `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`). No puede escribirse en el archivo de configuración.
Si la clave se filtra, rótala en el panel y vuelve a desplegar con la clave nueva.

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. Registrar el endpoint del webhook

En «Developers > Webhooks > Add endpoint», registra lo siguiente. El servidor no crea el endpoint mediante la API.

- URL: `https://<server domain>/payments/webhooks/stripe`
- Eventos de: «Your account» (no las cuentas conectadas)
- Estilo de carga: snapshot. El servidor lee solo `id`, `type`, `livemode`, `created`, `data.object.id` y `data.object.object` del cuerpo, así que la versión de API del endpoint no importa.
- Eventos que enviar (16 tipos):

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

Pasa el secreto de firma del endpoint (`whsec_...`) en la variable de entorno `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` (o `_FILE`).

El endpoint verifica la firma (HMAC-SHA256 en `Stripe-Signature`) y la marca de tiempo (dentro de 5 minutos en ambos sentidos), y rechaza con `400` un evento de prueba recibido con una clave real (y al revés).
Registra cada evento una sola vez por ID, encola un trabajo y devuelve `2xx`. Cuando no puede escribir en la base de datos devuelve `503`, y Stripe vuelve a enviar el evento. El cuerpo está limitado a 1 MiB.
Mientras rotas el secreto de firma, Stripe firma con el antiguo y con el nuevo, así que basta con poner el secreto nuevo en la configuración.

El estado del proveedor (el modo, cuándo llegó el último webhook, y el resultado y el tipo de fallo de la última conciliación) se muestra en la sección «Proveedor de pagos» de `/admin/supporters` en la administración, y mediante la API de administración `GET /api/v1/admin/payment_status` (permiso `manage_supporters`). La sección explica cómo resolver cada tipo de fallo de conciliación.
`warnings` lista `no_webhook` (nunca se ha registrado ningún webhook) y `reconciliation_corrected` (la última conciliación reparó una suscripción). Sospecha de un webhook mal configurado cuando aparezca cualquiera de los dos.
También lista `duplicate_charge`: una segunda suscripción aún no se ha cancelado y reembolsado. Consulta [Operación](#7-operation).
«Conciliar ahora» en la sección (`POST /api/v1/admin/payment_reconciliation/runs`) concilia de inmediato en lugar de esperar a la ejecución programada. Está limitado a una vez por hora UTC (una segunda llamada en la misma hora devuelve `429`), y el registro de auditoría guarda `payment_reconcile_queued`.

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. Configurar el servidor

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifica la configuración. Un binario compilado con la etiqueta de compilación `no_payments_stripe` rechaza `payments = stripe` (consulta [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. Probar en modo de prueba

Empieza con una clave de prueba (`rk_test_` / `sk_test_`, o una clave de sandbox). El modo de prueba no cobra nada.
Las claves de prueba y las reales tienen Products, Prices, clientes, suscripciones y endpoints de webhook separados. Para pasar a producción, registra de nuevo el endpoint en modo real, sustituye la clave y el secreto de firma por los reales y vuelve a desplegar. Los Prices de los planes se crean de nuevo en la cuenta real.

Tarjetas de prueba ([documentación de Stripe](https://docs.stripe.com/testing)):

| Número | Comportamiento |
| --- | --- |
| `4242 4242 4242 4242` | Se completa |
| `4000 0000 0000 0341` | Se puede asociar, pero el cargo falla (para comprobar el fallo de pago y el inicio del periodo de gracia) |
| `4000 0000 0000 9995` | Falla por fondos insuficientes |
| `4000 0000 0000 0259` | Se crea una disputa después del pago |

Usa cualquier fecha de caducidad futura y cualquier código de seguridad de 3 dígitos.

Para probar la renovación y el fallo de pago, usa un [test clock](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage). Un cliente creado por Checkout no tiene reloj al crearse, así que suscríbete primero y luego crea un reloj para ese cliente mediante la API, pasando `customer` a `POST /v1/test_helpers/test_clocks` (versión de API `2026-05-27.dahlia` o posterior).
Stripe lo permite solo cuando `frozen_time` no está en el pasado, la cuenta no tiene Automations configuradas y el cliente tiene como máximo tres suscripciones. Después, el cliente no puede salir del reloj, y eliminar el reloj elimina al cliente y sus suscripciones. Avanza el tiempo en «Billing > Test clocks» del panel para provocar el cargo de renovación y el fallo.

Un reloj no puede mostrarlo todo:

- **El final del periodo de gracia**: el servidor compara el tiempo real con la hora que Stripe indica para el fallo (`finalized_at`), que es simulada y va por delante del tiempo real. La suscripción entra en el periodo de gracia, y avanzar más el reloj no lo termina. El rol se queda hasta que el tiempo real pase 7 días después del fallo simulado, o hasta que Stripe cancele la suscripción cuando se agoten sus reintentos, si la cuenta está configurada para hacerlo.
- **El barrido de todas las suscripciones**: la lista de suscripciones de Stripe omite las suscripciones de un reloj salvo que la solicitud nombre el reloj o el cliente, y el barrido del servidor no nombra ninguno. Solo los webhooks y la lista de eventos llevan sus cambios al servidor.

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. Probar en local (Stripe CLI)

La [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) reenvía los eventos de Stripe a un servidor de tu equipo. No hace falta una URL pública.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Pon el `whsec_...` que muestra `stripe listen` en `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` e inicia el servidor (haz coincidir el puerto con la configuración de `listen`).

Con Docker Compose, define `MY_PAYMENTS=stripe`, `MY_PAYMENTS_STRIPE_SECRET_KEY` y `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` en `deploy/docker/.env` y ejecuta `docker compose up -d`.

En otra terminal, puedes lanzar eventos sueltos.

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

Los objetos que crea `stripe trigger` no llevan ningún ID de suscripción de este servidor, así que ninguna suscripción cambia de estado (el servidor los vuelve a leer e ignora lo que no es una suscripción propia). Para comprobar todo el flujo de suscripción, suscríbete desde el cliente web y paga con una tarjeta de prueba.

<!-- source: 7-operation f70cd2967b9f -->
## 7. Operación

- Un fallo de conciliación del tipo `permission` significa una clave incorrecta o permisos que faltan. La línea de registro del servidor (`payment reconciliation failed`) incluye el mensaje de Stripe, que nombra el permiso que hay que añadir.
- Cuando las conciliaciones alcanzan a menudo el límite de solicitudes, sube `payments_reconcile_max_requests` o alarga `payments_reconcile_interval`. Cada lectura cuenta para la asignación de lecturas de Stripe (consulta [Qué hace el servidor en Stripe](#what-the-server-does-at-stripe)), así que prefiere el intervalo más largo en una cuenta con pocas transacciones.
- Cuando la conciliación lleva parada más de 30 días, el servidor no puede ponerse al día desde la lista de eventos de Stripe (que se remonta 30 días). Entonces el barrido de todas las suscripciones repara el estado.
- `duplicate_charge` en las advertencias significa que un usuario pagó dos veces y el servidor no pudo terminar de cancelar y reembolsar la segunda suscripción. La línea de registro `a second subscription could not be canceled and refunded` nombra ambas suscripciones y el tipo de fallo, y aparece de nuevo en cada conciliación, que lo intenta otra vez. Con el tipo `permission`, añade el permiso para escribir reembolsos. De lo contrario, reembolsa el cargo en el panel: el siguiente intento no encuentra nada que reembolsar y borra la advertencia. «Conciliar ahora» lo intenta de inmediato.
- Los demás reembolsos y contracargos se gestionan en el panel de Stripe. Un reembolso total y un contracargo retiran el rol y cancelan la suscripción de inmediato, y un reembolso parcial no retira el rol. Ganar una disputa no restablece el rol automáticamente.

<!-- source: items-to-verify 8960333fa514 -->
## Puntos por verificar

Estos puntos no se han probado con Stripe, ni en un sandbox ni en producción, y se basan en la documentación de Stripe.

- Una renovación de una suscripción con un Price archivado, que debería quedarse en ese Price.
- `status_transitions.finalized_at` en una renovación real. El servidor lo toma como el momento en que el pago falló por primera vez, y cuenta el periodo de gracia desde él.
- Si una transacción `stripe_fee` puede asignarse a una suscripción. El servidor no lo intenta: resta estas comisiones de los ingresos en conjunto.
- Si `settings.dashboard.timezone` de `GET /v1/account` se lee con una clave restringida. Una lectura fallida no detiene el servidor, que cuenta en UTC, así que un servidor que funciona no lo muestra. Comprueba que las fechas de los ingresos caen en la zona horaria de la cuenta.
- Que `GET /v1/invoice_payments` filtra por `payment[payment_intent]`, que vincula un pago con su suscripción.
- Los reembolsos totales y parciales y los contracargos, desde el panel hasta el estado en el servidor.
- Que el barrido de todas las suscripciones repara una suscripción cuyo estado perdió el servidor.
- Qué hace una mejora en una suscripción que paga con un método que las actualizaciones pendientes no listan (los adeudos directos ACH, SEPA y Bacs). Mantén esos métodos desactivados en el panel hasta que se confirme.
- Que `cancel_at_period_end=true` sustituye un `cancel_at` de un periodo posterior.
- Volver a una página abierta. El servidor compara lo que devuelve `GET /v1/checkout/sessions/{id}` (`line_items`, `success_url`, `cancel_url`, `customer`, `customer_email`) con la solicitud nueva, y cualquier diferencia crea una página nueva. Si Stripe reescribe `success_url` o `customer_email` al guardarlos, el usuario recibe una página nueva cada vez y pierde lo que escribió, pero paga una sola vez.
- Hacer caducar una sesión justo cuando se completa su pago. La documentación de Stripe dice que una sesión que no está abierta devuelve un error, y entonces el servidor lee la sesión. El código exacto del error y el estado mientras hay una autenticación de tarjeta en curso no están documentados.
- Que una sesión no pueda pagarse dos veces desde dos pestañas. Una sesión contiene una suscripción y se completa una vez, pero la documentación no lo indica para dos envíos en el mismo instante.
- Si una autenticación fallida en la página de pago deja una suscripción `incomplete` que nombra la suscripción del servidor. El servidor la vincularía y mantendría al usuario pendiente, y un pago en otra página se reembolsaría como el segundo.
- Un reembolso de una segunda suscripción, con una clave restringida cuyos permisos «Charges» y «Refunds» son los que indica la tabla, y los movimientos de dinero que lo siguen.
