<!-- source: closing-the-service 30c438995ba1 -->
# Cerrar el servicio

Qué hacer, en orden, cuando dejas de ejecutar el servidor: permitir que los usuarios se lleven sus datos, eliminar las cuentas que quedan en otros servidores,
y después detener el servidor y pasar a una página estática de cierre.
Cada comando se describe con más detalle en [administradores y roles](admin-cli.md#announcing-the-end-of-the-service).

<!-- source: the-sequence 4557c35b6851 -->
## La secuencia

![Las etapas del cierre del servicio de arriba abajo: lo que hace el operador a la izquierda y el estado del servidor a la derecha, con la fecha de cierre y la fecha límite de exportación entre ambos. La tabla de abajo describe cada etapa.](images/service-closure.svg)

La tabla sigue el diagrama de arriba abajo; la columna Servidor empieza con el estado que muestra `my admin closure status`.
La fecha de cierre (`-closes-at`) termina el período de aviso, y la fecha límite de exportación (`-export-until`, 30 días después de la fecha de cierre
salvo que se indique otra) termina la etapa 5.

| Etapa | Servidor | Qué hace el operador |
| --- | --- | --- |
| 1. Anunciar | `announced`. Funciona con normalidad. Se detienen los registros, las invitaciones y las suscripciones nuevas de mecenas | `my admin closure announce`, publicar un aviso en la página de estado |
| 2. Período de aviso | `announced`. Funciona con normalidad. Los usuarios pueden exportar sus datos y mudarse a otro servidor | Reembolsar a los mecenas anuales, revisar las migraciones y las retenciones |
| 3. Cerrar (después de la fecha de cierre) | `closed`. Los usuarios solo pueden iniciar sesión y exportar. El servidor entrega un `Delete` por cada cuenta | `my admin closure check` y `close` |
| 4. Entrega completa | Registra `delivery_completed`; los actores locales también devuelven `410` | Confirmar con `my admin closure status`. No detener los procesos de trabajo |
| 5. Hasta la fecha límite de exportación | `delivery_completed`. Permanece cerrado y sigue ofreciendo la exportación | Exportar los registros sujetos a retención |
| 6. Página estática | Detenido. El front end sirve la página estática, y `410` para la federación | `my admin closure export-html`, cambiar el front end, detener `my serve` |
| 7. Descartar | Ninguno. Seguir sirviendo la página estática y `410` durante aproximadamente un año | Descartar la base de datos, los blobs y las copias de seguridad. Conservar el dominio durante aproximadamente un año |

Puedes combinar cualquier etapa con el modo de mantenimiento ([Mantenimiento](maintenance.md)). Durante el mantenimiento, las exportaciones y la entrega de actividades `Delete` se detienen.

<!-- source: 1-announce b34cb9474667 -->
## 1. Anunciar

Decide la fecha de cierre (`-closes-at`) y la fecha límite para llevarse los datos (`-export-until`, 30 días después de la fecha de cierre si se omite), y anúncialas.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- No hay un límite inferior para la fecha de cierre, pero aparece una advertencia cuando el tiempo entre el anuncio y la fecha de cierre es inferior a 30 días. Deja tiempo a los usuarios para exportar y mudarse.
- Para cambiar la fecha, ejecuta `announce` de nuevo; para cancelarlo, ejecuta `my admin closure withdraw`. Ambos envían a cada usuario una notificación `service_closure`.
- Una vez anunciado, el modo de registro efectivo pasa a `closed`, las solicitudes pendientes se rechazan y cada suscripción de mecenas vigente se cancela al final de su período (los suscriptores reciben una notificación y un correo electrónico).
  Retirar el anuncio no deshace los rechazos ni las cancelaciones.
- Las notificaciones las crean trabajos, así que no detengas los procesos de trabajo.
- En la página de estado ([monitorización y página de estado](monitoring.md)), publica la fecha de cierre, la fecha límite de exportación y cómo exportar y mudarse.
  La página de estado sobrevive al servidor, así que también orienta a quienes la visiten después del cierre.

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. Período de aviso

- Los usuarios pueden exportar sus datos (la exportación de datos y los archivos CSV) y mudarse a otro servidor (`Move`) desde la pantalla de configuración.
  Un banner en cada pantalla y una notificación muestran la fecha de cierre, la fecha límite de exportación y el camino hasta esa pantalla.
- **Mecenas anuales**: el anuncio cancela las suscripciones al final del período, pero en una suscripción cuyo período termina después de la fecha de cierre, el tiempo en que no se puede usar ya está pagado.
  El servidor no reembolsa, así que reembolsa a cada suscriptor en la pantalla del proveedor de pagos (en Stripe, la pantalla de pagos del panel; en PAY.JP, la pantalla de pagos de la consola de administración) ([pagos de mecenas (Stripe)](payments-stripe.md), [pagos de mecenas (PAY.JP)](payments-payjp.md)).
- Cuando pasa la fecha de cierre, las personas con `manage_service_closure` reciben una notificación `admin_service_closure_due`. El servidor no se cierra por sí solo.

<!-- source: 3-close c851780cf430 -->
## 3. Cerrar

Cuando haya pasado la fecha de cierre, ejecuta las comprobaciones antes de cerrar.

```sh
my admin closure check
```

- **Las migraciones sin terminar** producen una advertencia. Espera a que terminen antes de cerrar, porque después del cierre nadie puede iniciar un `Move`.
- **Las retenciones (`retention_holds`)** en cualquier cuenta producen una advertencia.
  La obligación que hay detrás de una retención no termina al cerrar el servidor, así que exporta los registros sujetos a retención y guárdalos fuera del servidor (lista las retenciones con `my admin retention list`).

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

Cuando las comprobaciones hayan terminado, cierra. No se puede deshacer. Confirma escribiendo `y` y el nombre de dominio (usa `-confirm-domain` en la automatización).

```sh
my admin closure close
```

Tras el cierre, los usuarios solo pueden iniciar sesión y exportar sus datos. Las suscripciones de mecenas restantes se cancelan de inmediato,
y se entrega a otros servidores un `Delete` (Actor) por cada cuenta local (incluso cuando `federation_mode` es `off` o `inbound`).
El buzón de entrada (inbox) y los GET de publicaciones y colecciones devuelven `410 Gone`.

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. Confirmar que la entrega está completa

**No detengas `my serve` ni los procesos de trabajo (procesos cuyo `role` es `all` o `worker`; con una cola de tipo push, la cola de Cloud Tasks) hasta que el estado sea `delivery_completed`.**
Los procesos de trabajo entregan las actividades `Delete`, y mientras lo hacen, el servidor sigue devolviendo el documento y la clave del actor local para que los receptores puedan verificar la firma del `Delete`.
Si te detienes antes de tiempo, las cuentas permanecen en otros servidores.

```sh
my admin closure status
```

- `accounts with a Delete` es el número de cuentas para las que se registró un `Delete`, `fan-outs pending` es el trabajo de distribución que queda, y `deliveries <state>` es el número de entregas por estado.
- Las entregas se reintentan según la política habitual (hasta 9 veces durante unos 4 días). Cuando todas han terminado, la comprobación que se ejecuta cada 10 minutos registra `delivery_completed` (`state: delivery_completed`).
  A partir de entonces los actores locales y el actor de la instancia también devuelven `410`.
- Una entrega que pasó a `dead` no se reintenta automáticamente. Para reintentarla, usa `my job retry`.

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. Hasta la fecha límite de exportación

Incluso después de completarse la entrega, el servidor sigue funcionando cerrado hasta `export_until` y sigue sirviendo las exportaciones de los usuarios.
El servidor nunca elimina datos por sí solo, así que sigue funcionando pasada la fecha límite hasta que lo detengas.
Si todavía no has exportado los registros sujetos a retención (§3), hazlo ahora.

<!-- source: 6-switching-to-the-static-page 9795c9aea7ae -->
## 6. Cambiar a la página estática

Después de la fecha límite de exportación, exporta la página de cierre. **Esto lee la base de datos, así que ejecútalo antes de detener `my serve` y descartar la base de datos.**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- Qué escribe:
  - `<dir>/<locale>.html` y `<dir>/index.html`, que lista todos los idiomas en orden. Usan la misma plantilla y el mismo CSS que la página de mantenimiento (`my maintenance export`) y muestran
    el nombre del servidor (`maintenance_server_name`, o si no el nombre de la instancia, o si no el nombre de host), la fecha de cierre, el mensaje del operador del anuncio y un enlace a la página de estado (`status_page_url`).
    El CSS es solo en línea, no hay scripts ni recursos externos, y la CSP está en una etiqueta `<meta>`. Se incorporan las sobrescrituras de colores del tema; el CSS personalizado no se incluye.
  - Ejemplos de configuración del front end `nginx.conf`, `Caddyfile` y `cloudfront-function.js`. Cada uno devuelve `410` sin cuerpo para las rutas de federación y descubrimiento (`/ap/*`, `/inbox`, `/.well-known/webfinger`,
    `/.well-known/host-meta`, `/.well-known/host-meta.json`, `/.well-known/nodeinfo`, `/nodeinfo/*`) y para los perfiles (`/@*`),
    y para cualquier otra ruta devuelve la página del primer idioma de `Accept-Language` (o `index.html` si no hay ninguno).
    Un idioma llega a la página de la configuración regional en la que lo mostraría la aplicación: `zh`, `zh-SG` y `zh-Hans` obtienen `zh-CN.html`, y `zh-Hant`, `zh-HK` y `zh-MO` obtienen `zh-TW.html`.
- El comando se niega si el servidor no se ha cerrado (código de salida 1). Antes de `delivery_completed`, avisa de que las entregas continúan (y aun así escribe los archivos).

Cambia el front end. Pon solo las páginas (`*.html`) en la ubicación pública, y adapta a tu entorno el nombre de host, el directorio y las rutas de los certificados del ejemplo.

| Front end | Cómo configurarlo |
| --- | --- |
| nginx | Pon las páginas en `/srv/closure` e incluye `nginx.conf` desde el bloque `http`. Las páginas también se sirven con `410` |
| Caddy | Pon las páginas en `/srv/closure` y usa el `Caddyfile`. Caddy obtiene el certificado, y las páginas también se sirven con `410` |
| CloudFront | Pon las páginas directamente bajo un bucket de S3, haz de ese bucket el origen del comportamiento de caché predeterminado y asocia `cloudfront-function.js` como una función de CloudFront de tipo viewer-request (`cloudfront-js-2.0`). Pon `Accept-Language` en la clave de caché de la política de caché. Las páginas se sirven con `200` (una función viewer-request no puede cambiar el estado de la respuesta del origen) |

Después del cambio, comprueba que `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` devuelve `410` y
que `curl -i https://social.example/` devuelve la página de cierre. Después, detén `my serve` y los procesos de trabajo.
Para la monitorización de disponibilidad, detén los monitores de comprobación de estado o apúntalos a la página de cierre.

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. Descartar la base de datos, los blobs y las copias de seguridad

Tras detenerlo, descarta la base de datos, los blobs (contenido multimedia y archivos de exportación), el almacén KV y las colas, junto con sus copias de seguridad e instantáneas. Si tienes un [motor de búsqueda](search.md#closing-the-service), descarta también su índice y sus instantáneas: contienen el texto de las publicaciones.
Antes de descartar nada, confirma una vez más que exportaste y guardaste los registros sujetos a retención. Descartar no se puede deshacer.

**Conserva el dominio y sigue sirviendo la página estática y `410` durante aproximadamente un año.** Otros servidores ven el `410`, dejan de recuperar y de entregar, y vacían sus cachés.
Los URI federados usan UUID, así que hay poco riesgo de que se vuelva a crear un actor con el mismo `id`,
pero si otra persona se queda con el dominio, el identificador (`acct:alice@social.example`) y las URL de los perfiles pasan a ser suyos y pueden usarse para suplantar a tus usuarios.
Sigue renovando el dominio y el certificado, y piénsalo de nuevo antes de soltar el dominio incluso después de que pase el año.
