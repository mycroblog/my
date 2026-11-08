<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# Pagos de mecenas con PAY.JP

Con `payments = payjp`, los usuarios locales se suscriben a planes mensuales o anuales en la página de pago de este servidor, y el rol del plan se concede y se retira según cambia el estado del pago.
Las claves de despliegue se describen en la [referencia de configuración](../reference/configuration.md#payments). Para el programa en su conjunto (planes, roles, la lista de miembros), consulta [supporters.md](supporters.md).
Un administrador activa el programa y crea los planes en la administración. Esta configuración solo permite que el servidor hable con PAY.JP.

Un servidor solo puede usar un proveedor. Para cambiar de Stripe a PAY.JP (o a la inversa), pide a los suscriptores existentes que se suscriban de nuevo con el nuevo proveedor.

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## Qué hace el servidor en PAY.JP

PAY.JP tiene las API v1 y v2. La v2 aún no tiene una API de cobro recurrente, así que el servidor usa el cobro recurrente de la **API v1** (Plan y Subscription). La v1 no tiene ni página de pago alojada ni portal de clientes, así que este servidor ofrece páginas en su lugar.

[Cómo funciona una suscripción](supporters.md#how-a-subscription-works) muestra en un diagrama cómo encajan la página de pago, el webhook, la conciliación, el rol del plan y los ingresos.

- **Planes**: para cada precio el servidor crea un Plan cuyo ID es `my_<plan ID>_<month|year>_<amount>` (`metadata.my_plan_id`). Un Plan de PAY.JP no puede cambiar su importe, así que cambiar el importe crea un Plan nuevo, y el Plan antiguo no se elimina sino que recibe `metadata.my_archived=1`. Las suscripciones del Plan antiguo se renuevan a su importe.
  No edites ni elimines estos Plans en la administración de PAY.JP. La única moneda es el yen japonés, y los importes van de 50 a 9.999.999 yenes.
- **Suscripción**: el servidor envía el navegador del usuario a su propia página de pago (`/payments/pages/payjp/checkout`). La página muestra los campos de tarjeta de payjp.js (el iframe de PAY.JP) y ejecuta la autenticación 3-D Secure al crear el token (el tipo iframe).
  El servidor acepta solo un token cuyo 3-D Secure se haya completado (`three_d_secure_status` es `verified` o `attempted`), añade la tarjeta al cliente como tarjeta predeterminada (la primera vez con el ID `my_<the server's subscription ID>`) y crea una suscripción con el ID de suscripción del servidor en `metadata.my_subscription_id`. El primer cobro se realiza al crear la suscripción.
  La URL de la página es de corta duración y lleva un estado firmado por el servidor (válido de 1 a 2 horas desde su creación).
  Un usuario que abandona la página y empieza de nuevo, con el mismo plan u otro, recibe una URL nueva para el plan que pidió ahora. Una URL anterior sigue funcionando hasta que caduca, y la página que se pague primero crea la única suscripción: las demás pasan a ella. Si una página anterior ya se pagó, el usuario va a la pantalla de retorno y no recibe una URL nueva.
  Una URL firmada no se puede retirar, así que una página sigue siendo pagable después de que se cierre su suscripción pendiente (su plan retirado, su cuenta eliminada). Un pago en ella no compra nada: el servidor elimina la suscripción y reembolsa sus cobros por completo (PAY.JP acepta un reembolso durante 180 días desde la venta).
  La API v1 no admite clave de idempotencia, así que mientras un envío de la página crea la suscripción, el servidor marca el pago como en curso en la base de datos: un segundo envío (un doble clic, otra pestaña u otro proceso del servidor detrás del mismo balanceador de carga) espera hasta 30 segundos y luego pasa a la suscripción que creó el primero. La marca caduca a los 5 minutos si el proceso que la tenía se detuvo.
- **La página de tarjeta e historial de pagos** (en lugar del portal de clientes de Stripe): desde su pantalla de suscripción, un suscriptor abre `/payments/pages/payjp/portal` para cambiar la tarjeta y consultar los pagos recientes (la URL caduca en una hora). El formulario empieza con la dirección de correo electrónico del cliente de PAY.JP, que es la dirección introducida en el primer pago, no un cambio posterior de la dirección de la cuenta.
  Cambiar la tarjeta reanuda una suscripción detenida por un pago fallido y cobra de inmediato. Cancelar y cambiar de plan se hace en la pantalla de suscripción de este servidor.
- **Fallo de pago**: PAY.JP pone en pausa (`paused`) una suscripción cuyo cobro de renovación falló, y **no reintenta**. El servidor trata la pausa como un pago atrasado y conserva el rol durante un período de gracia de 7 días desde el momento de la pausa. Si el suscriptor cambia la tarjeta en ese tiempo, la suscripción se reanuda; si no, el rol se retira pasados 7 días (en PAY.JP la suscripción sigue en pausa y no se cobra).
- **Cambios de plan**: una mejora de plan cobra la diferencia por el resto del período actual, prorrateada, como un cobro único (sin cobro por debajo de 50 yenes), reemplaza el plan y cobra el importe completo del nuevo plan al final del período (el inicio del período no se mueve). Si el cobro de la diferencia es rechazado, el plan no cambia.
  Una rebaja de plan, y un cambio de anual a mensual, se aplican al final del período con `next_cycle_plan`.
- **Lectura del estado de vuelta**: el servidor trata un webhook como una señal y lee de vuelta la suscripción y los movimientos de dinero desde la API de PAY.JP para decidir el estado.
  Además, una conciliación cada `payments_reconcile_interval` lee la lista de eventos, los movimientos de dinero y un barrido de todas las suscripciones, y repara cualquier webhook que el servidor haya perdido.
- **Ingresos**: por cada pago el servidor registra el importe bruto, la comisión (calculada a partir del `fee_rate` del cobro, redondeando a la baja por debajo de un yen) y el neto. Un reembolso y un contracargo registran la comisión devuelta como una comisión negativa.
  Cuando se cierra un período y se emite un extracto, la diferencia entre la comisión del extracto y el cálculo del servidor se registra como un movimiento de comisión (su ID es el ID del extracto), de modo que el total de comisiones del período coincida con el extracto.
  Un extracto abarca las comisiones de toda la cuenta, así que **usa una cuenta de PAY.JP dedicada a este servidor**.
- **Fechas**: las fechas de los ingresos se cuentan en hora de Japón.

El servidor envía como máximo 20 peticiones por segundo (2 en modo de prueba). Eso se mantiene dentro del límite de frecuencia de PAY.JP (para las peticiones con clave secreta, 30 por segundo en modo real y 2 en modo de prueba). Ante un `429` el servidor espera 1 segundo, duplicándolo cada vez, y reintenta hasta 3 veces.
En modo de prueba una sola conciliación es lenta (unos 8 minutos con el límite de 1.000 peticiones). Un trabajo de continuación toma el relevo, así que no hace falta ninguna acción.

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## Datos de tarjeta y PCI DSS

Los números de tarjeta, las fechas de caducidad y los códigos de seguridad se introducen solo dentro del iframe de PAY.JP, que muestra payjp.js. La página y el script de este servidor nunca los tocan. El servidor recibe solo el ID del token que creó payjp.js y el nombre del titular y la dirección de correo electrónico que necesita 3-D Secure.
El servidor guarda solo IDs de objetos de PAY.JP (`cus_`, `sub_`, `ch_`, IDs de Plan, IDs de evento), importes, períodos y estados. No guarda números de tarjeta, últimos cuatro dígitos ni fechas de caducidad (la página de tarjeta solo lee de PAY.JP los últimos cuatro dígitos, la fecha de caducidad y la dirección de correo electrónico del cliente, y los muestra). Los cuerpos de los webhooks y las respuestas de PAY.JP no se conservan ni en los registros ni en la base de datos.

La página de pago se sirve desde el origen de este servidor, por lo que está construida teniendo en cuenta la gestión de scripts de páginas de pago de PCI DSS v4 (requisito 6.4.3) y la detección de manipulaciones (requisito 11.6.1).

- La página carga solo `https://js.pay.jp/v2/pay.js` y un pequeño script con un nonce incrustado en la página. No lleva ni el paquete del cliente web ni ningún script de analítica o publicidad.
- La página lleva siempre su propia Content-Security-Policy (`script-src` solo el nonce y `https://js.pay.jp`, `frame-src` solo `https://js.pay.jp` y `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` y `X-Content-Type-Options: nosniff`. Ninguna configuración cambia la CSP.
- La plantilla y el script están incrustados en el binario del servidor y no se pueden cambiar en tiempo de ejecución. Los envíos de formularios se aceptan solo desde el mismo origen.

Como resultado, se espera que el alcance del operador sea el de un comerciante que deja la introducción del pago al iframe de PAY.JP (SAQ A). Comprueba el alcance que te corresponde y el tipo de cuestionario de autoevaluación conforme a la guía de PAY.JP y a los requisitos de tus marcas de tarjeta.
Al hacerlo, conviene conservar un registro de estos puntos: las URL de las páginas de pago (`/payments/pages/payjp/checkout` y `/portal`), la lista de scripts que carga la página (los dos anteriores), la CSP de la página y que el tráfico hacia el servidor usa TLS (la configuración del proxy inverso).

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. Preparar la cuenta de PAY.JP

1. Crea una cuenta de PAY.JP y empieza en modo de prueba. Cobrar en modo real requiere una solicitud de producción y una revisión.
2. Consulta «API settings > 3-D Secure» en el panel de PAY.JP. El servidor pide 3-D Secure cada vez que crea un token, así que también puedes optar por exigir 3-D Secure en toda creación de tokens.
3. La página de la divulgación comercial se define en la administración de este servidor (consulta [supporters.md](supporters.md)).

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. Claves y token del webhook

En «API» del panel de PAY.JP, consulta estas dos claves. El prefijo de una clave determina el modo, y las claves de prueba no cobran nada. Ambas claves deben estar en el mismo modo.

| Clave | Ejemplo | Uso |
| --- | --- | --- |
| Clave secreta | `sk_test_...` / `sk_live_...` | El servidor llama a la API con ella. Es también el origen de la clave que firma las URL de las páginas |
| Clave pública | `pk_test_...` / `pk_live_...` | payjp.js en las páginas de pago y de tarjeta |

Pasa la clave secreta en la variable de entorno `MY_PAYMENTS_PAYJP_SECRET_KEY` (o la ruta de un archivo en `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`). No se puede escribir en el archivo de configuración.
Si la clave secreta se filtra, vuelve a emitirla en el panel y vuelve a desplegar con la nueva clave. Las URL de las páginas de pago y de tarjeta que estaban abiertas antes del nuevo despliegue dejan de funcionar.

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. Registrar el webhook

En «Settings > Webhook» del panel de PAY.JP, añade un destino. Registra por separado el modo de prueba y el modo real.

- URL: `https://<server domain>/payments/webhooks/payjp`
- Modo: el mismo que las claves

Un webhook de PAY.JP identifica a su remitente con el **token de origen** de la cuenta (`whook_...`, en la cabecera `X-Payjp-Webhook-Token`). Pasa el token que muestra el panel en la variable de entorno `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (o `_FILE`).
El endpoint compara el token en tiempo constante, y rechaza con `400` un evento de prueba recibido con una clave real (y a la inversa). No hay firma ni marca de tiempo, así que cualquiera que conozca el token puede enviar una notificación, pero el servidor nunca escribe estado a partir del contenido de una notificación; solo lee el estado de vuelta desde PAY.JP.

El servidor usa estos eventos (los otros tipos se aceptan pero no se registran).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP reenvía una notificación solo cada 3 minutos, como máximo 3 veces, y una notificación pasado ese punto no llega hasta que la reenvías desde el panel. La conciliación repara lo que se perdió, así que **se recomienda establecer `payments_reconcile_interval` en 1 hora**.
El estado del proveedor (el modo, cuándo llegó el último webhook y el resultado de la última conciliación) se muestra en la sección «Proveedor de pagos» de `/admin/supporters` en la administración.

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. Configurar el servidor

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifica la configuración. Un binario compilado con la etiqueta de compilación `no_payments_payjp` rechaza `payments = payjp` (consulta [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. Probar en modo de prueba

Empieza con claves de prueba. El modo de prueba y el modo real tienen Plans, clientes, suscripciones, webhooks y tokens separados. Para pasar a producción, registra el webhook en modo real, sustituye las claves y el token por los reales y vuelve a desplegar. Los Plans de los planes se crean de nuevo en la cuenta real.

Tarjetas de prueba ([documentación de PAY.JP](https://pay.jp/docs/testcard)):

| Número | Comportamiento |
| --- | --- |
| `4242 4242 4242 4242` | Se completa con éxito |
| `4000 0000 0008 0319` | La tarjeta se puede registrar pero el cobro es rechazado (una suscripción fallida y una reanudación fallida tras un cambio de tarjeta) |
| `4000 0000 0000 4012` | La tarjeta se puede registrar pero el cobro falla por caducidad |
| `4000 0000 0000 0002` | El registro de la tarjeta (la creación del token) es rechazado |

Usa una fecha de caducidad futura; el código de seguridad y el nombre son arbitrarios. En modo de prueba, la pantalla de autenticación de prueba de 3-D Secure de PAY.JP te permite elegir el resultado.

Para probar una renovación con rapidez, establece el `trial_end` de la suscripción unos minutos más adelante con la API. El cobro de renovación se ejecuta algunos minutos después de esa hora, no a esa hora. Otra actualización de `trial_end` mientras la primera está pendiente reinicia la espera.
Para probar un fallo de pago y el período de gracia, suscríbete, cambia la tarjeta en la página de tarjeta por la tarjeta que se rechaza (`4000 0000 0008 0319`) y luego deja que la suscripción se renueve.

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. Probar en local

Los webhooks de PAY.JP van a una URL pública, así que para recibirlos en un servidor de tu equipo, expón el puerto mediante un túnel (cloudflared, ngrok y similares) y registra esa URL como destino del modo de prueba.
Se espera que `payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` de payjp-cli también reenvíe eventos, pero no se ha verificado si puede reenviar eventos de la v1.

Abre las páginas de pago y de tarjeta en el origen de este servidor (también funcionan en `http://localhost:8080`, pero el comportamiento de payjp.js y de 3-D Secure varía según las configuraciones del navegador).

<!-- source: 7-operation fbdd81461f80 -->
## 7. Operación

- Un fallo de conciliación de tipo `permission` significa una clave secreta incorrecta.
- `duplicate_charge` en las advertencias de la sección «Proveedor de pagos» significa que el servidor no pudo terminar de reembolsar un pago de una suscripción cerrada. La línea de registro `a second subscription could not be canceled and refunded` nombra la suscripción, y cada conciliación lo intenta de nuevo, también después de que se elimine la suscripción. Reembolsa el cobro en el panel si el aviso continúa: el siguiente intento lo encuentra reembolsado y borra la advertencia, siempre que PAY.JP siga listando los cobros de la suscripción eliminada (consulta [Puntos por verificar](#items-to-verify)). Un cobro de más de 180 días no se puede reembolsar: el servidor lo registra y continúa.
- Los demás reembolsos se gestionan en el panel de PAY.JP. Un reembolso total y un contracargo retiran el rol y eliminan la suscripción de inmediato, y un reembolso parcial no retira el rol.
- **La cancelación de un contracargo** no se registra automáticamente, porque PAY.JP no emite ni un evento ni una API para ello. Cuando ocurra uno, registra el importe cancelado en la pantalla de ingresos de la página de costos (`finance`). El rol no se restablece automáticamente.
- **Las comisiones del plan de pago de PAY.JP y las comisiones de transferencia** no se restan de los ingresos. Regístralas como costos en `finance`.
- Cuando las conciliaciones alcancen a menudo el límite de peticiones, aumenta `payments_reconcile_max_requests` o alarga `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Cambios de plan

Una mejora de plan cobra primero la diferencia prorrateada y mueve el plan solo después de que ese cobro se realice con éxito. Cuando el cobro es rechazado, el plan se mantiene y el usuario ve un error con la forma de actualizar la tarjeta, sin período de gracia ni notificación de pago fallido, y volver a intentarlo con una tarjeta válida cobra una sola vez. Si el plan no se mueve después del cobro, volver a intentarlo no cobra de nuevo. Una diferencia por debajo del mínimo de 50 yenes, y un cambio al precio anual del mismo plan, que no tiene una diferencia positiva, no cobran nada ahora y se mueven de inmediato; el nuevo precio se cobra al final del período, y un fallo entonces es una renovación fallida.

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## Puntos por verificar

La documentación de PAY.JP y el modo de prueba no aclaran los siguientes comportamientos.

- Cómo se calcula la comisión devuelta por un contracargo, y si el extracto de un período coincide con las comisiones del servidor (el modo de prueba mostró una comisión redondeada a la baja, y la comisión de un reembolso calculada a partir de su importe).
- Qué hace un contracargo real: PAY.JP no documenta ninguna forma de provocar uno en modo de prueba ni ningún cuerpo para `dispute.created`. Un envío de prueba del evento desde el panel se responde con `200` y no cambia nada.
- Hasta cuándo hacia atrás llega la lista de eventos.
- Si eliminar un Plan afecta a las suscripciones existentes (el servidor nunca elimina ninguno), y la longitud máxima de un ID de Plan.
- Si importa el informe de CSP al cargar la página de pago: `pay.js` aplica un estilo en línea contra `style-src 'self'`, mientras que los campos de tarjeta y 3-D Secure funcionan en Chromium. No se comprobaron otros navegadores.
- Si payjp-cli puede reenviar eventos de la v1.
- Que un cobro de la tarjeta guardada de un cliente no pide 3-D Secure a menos que lo pida la petición. Un cobro rechazado aparece en `GET /v1/charges` con `paid: false`, y el servidor ignora los que no están pagados.
- Que `GET /v1/charges` con `subscription` siga listando los cobros de una suscripción eliminada. El servidor reembolsa antes de eliminar, así que solo un intento posterior a un reembolso que falló definitivamente depende de esto. La referencia de la API documenta el filtro y no dice nada sobre las suscripciones eliminadas. Si la lista vuelve vacía, el servidor informa de un fallo en lugar de un reembolso, y `duplicate_charge` permanece incluso después de reembolsar el cobro a mano.
