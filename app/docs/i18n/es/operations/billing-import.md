<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# Importación de los costos de uso de la nube

El servidor importa cada 24 horas los cargos de uso de los proveedores indicados en la configuración [`billing_sources`](../reference/configuration.md#billing_sources), o de [un plugin](#a-plugin-billing_sources--plugin) para un proveedor del que no tiene adaptador, y los suma a los costos de la instancia. El valor por defecto está vacío, lo que no importa nada.
Importa los cargos del mes de facturación actual y del anterior (hasta el día 15 del mes, o hasta que el mes anterior sea definitivo), como una línea por servicio y tipo de cargo (uso, crédito, reembolso, impuesto, soporte, tarifa, otro).
Hasta que el proveedor cierra un mes, los valores se registran como estimaciones; después de cerrarlo, como definitivos; y cuando un valor cambia una vez definitivo, como una corrección. Los registros del pasado nunca se reescriben.
El estado de las importaciones (el último éxito, el tipo del último fallo y el número de solicitudes de este mes) se muestra en la página de costos de la administración y con `GET /api/v1/admin/billing_imports`.
Las métricas cuentan cada intento por origen, resultado y tipo de fallo (`my.billing.imports`) e informan de los orígenes cuyo intento más reciente falló (`my.billing.import.failing`);
[Alertas y guía de actuación](alerts.md#mybillingimportfailing) describe la alerta sobre ellas.

La configuración de este servidor no guarda credenciales. Da al entorno de ejecución lo que lee la cadena de credenciales por defecto del SDK de cada proveedor (para AWS, el rol de ejecución, las variables de entorno o los archivos de credenciales compartidos; para Google Cloud, las Application Default Credentials).

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## Con los módulos de OpenTofu

Los módulos [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) y [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) definen `billing_sources` y hacen las partes de esta guía que permite la API de un proveedor. `billing-gcp` solo funciona con el módulo de aplicación de Google Cloud, así que en AWS los módulos importan únicamente los costos de AWS. `billing-aws` funciona con los módulos de aplicación de AWS y con el de Cloud Run, donde ambos pueden estar en `backends`. Ninguno se ha aplicado todavía en una cuenta real.

| Paso de esta guía | Con los módulos |
| --- | --- |
| La política de IAM y el rol de «Llamada desde Cloud Run con identidad web» | `billing-aws` |
| Activar Cost Explorer, Linked Account Access y las etiquetas de asignación de costos | A mano, en la consola de AWS |
| El conjunto de datos de la exportación y los roles de «Roles de IAM» | `billing-gcp` |
| Activar la exportación («Standard usage cost») | A mano, en la consola de Google Cloud, con un rol en la cuenta de facturación |
| El tema y la suscripción pull de un presupuesto, y el rol de suscriptor | `billing-gcp` con `method = "budget"` |
| Crear el presupuesto y conectarlo al tema | A mano, en la consola de Google Cloud, con un rol en la cuenta de facturación |
| Activar la API de BigQuery o de Pub/Sub | A mano, como en [Despliegue en Cloud Run](deploy-cloud-run.md#1-enable-the-apis) |
| Un plugin | No con estos módulos. Sin ningún módulo de facturación en `backends`, define `MY_BILLING_SOURCES = plugin` y la configuración de [el plugin](#a-plugin-billing_sources--plugin) en el `env` del módulo de aplicación; junto a un módulo de facturación, el módulo de aplicación rechaza `MY_BILLING_SOURCES` en `env`, así que añade un backend propio que devuelva `billing_sources = ["plugin"]` ([los campos de un backend](../../deploy/opentofu/README.md)) |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS (`billing_sources = aws`)

El servidor llama a `GetCostAndUsage` de AWS Cost Explorer una vez por mes de facturación (un mes natural en UTC), agrupando por servicio (`SERVICE`) y tipo de registro (`RECORD_TYPE`). El endpoint es siempre `us-east-1`.
Los importes están en USD. Mientras Cost Explorer marca un mes como estimado (`Estimated`), el servidor registra como estimación el importe hasta el día anterior, y cuando deja de hacerlo, como definitivo.
Un binario compilado con la etiqueta de compilación `no_billing_aws` rechaza `aws` (consulta [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits a555f91984c3 -->
### Costo y límites

La API de Cost Explorer cobra 0,01 $ por cada solicitud paginada. Una cuenta pequeña necesita una solicitud de una página al día, unos 0,31 $ al mes.
Para evitar cargos inesperados, el servidor cuenta las solicitudes por día UTC (`billing_aws_max_requests_per_day`, por defecto 4) y por mes natural UTC (`billing_aws_max_requests_per_month`, por defecto 120 = 1,20 $), y se detiene sin llamar cuando se alcanza un límite.
Cuentan las importaciones programadas, los reintentos de trabajos y «Importar ahora» en la administración. Una importación que alcanza un límite se registra como un fallo del tipo `request_cap`, y la importación del día siguiente (para el límite diario) o del primer día del mes siguiente (para el límite mensual) lo intenta de nuevo.
Antes de llamar, una importación reserva las solicitudes que dejan los límites, de modo que las importaciones que se ejecutan a la vez no pueden superarlos en conjunto.
Las importaciones de un proveedor se ejecutan de una en una: una importación que empieza mientras otra no ha terminado lo intenta de nuevo un minuto después, y un trabajo que ya importó con éxito no llama a nada cuando se ejecuta otra vez.
Una importación que se detuvo antes de terminar (el proceso salió) cuenta las solicitudes que había reservado, y la siguiente importación la espera hasta 15 minutos.

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### Activar Cost Explorer

1. Abre Cost Explorer en la consola de Billing and Cost Management y actívalo. No se puede desactivar una vez activado. Los datos tardan hasta 24 horas en estar listos.
2. Una cuenta donde no está activado devuelve `DataUnavailableException`, y la importación falla con `not_enabled`.

<!-- source: iam-permissions 9f1c23f3cb5c -->
### Permisos de IAM

El único permiso necesario es `ce:GetCostAndUsage` (recurso `*`).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

Cómo pasar las credenciales en cada entorno de ejecución:

| Entorno de ejecución | Credenciales |
| --- | --- |
| AWS ECS | Adjunta esta política al rol de la tarea. No hace falta ninguna configuración |
| Google Cloud (Cloud Run) | Crea el rol de IAM de «Llamada desde Cloud Run con identidad web» que aparece más abajo y define su ARN en `billing_aws_role_arn`. No hacen falta claves de AWS |
| Tu propio servidor | Pasa la clave de acceso de un usuario de IAM con esta política en `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (o un archivo de credenciales compartido y `AWS_PROFILE`). Pásala desde un almacén de secretos |

Si faltan permisos o las credenciales no son válidas, se falla con `permission`.

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### Llamada desde Cloud Run con identidad web

Cuando `billing_aws_role_arn` está definido, el servidor obtiene del servidor de metadatos un token de ID de Google de la cuenta de servicio de Cloud Run (la audiencia es el ARN del rol) y lo intercambia por credenciales temporales de ese rol con `AssumeRoleWithWebIdentity`.

1. En Google Cloud, anota el ID único (el `uniqueId` numérico) de la cuenta de servicio del servicio de Cloud Run: `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`.
2. En AWS IAM, crea un rol con la política de los permisos anteriores y haz que su política de confianza confíe en `accounts.google.com`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` corresponde al `azp` del token (el ID de la cuenta de servicio), y `accounts.google.com:oaud` al `aud` del token (la audiencia que solicita el servidor, que es el ARN del rol).
3. Define `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`.

Esta configuración aún no se ha verificado en una cuenta real.

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### Llamada desde una cuenta vinculada de Organizations

La cuenta de administración de AWS Organizations puede ver los costos de todas las cuentas miembro de la organización. Para llamar con las credenciales de una cuenta miembro (vinculada),
activa «Linked Account Access» en las preferencias de Cost Explorer de la cuenta de administración. Mientras está desactivado, la llamada se rechaza sean cuales sean los permisos de IAM, y la importación falla con `not_enabled` o `permission`.

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### Elegir qué costos importar (filtrado)

Elige con `billing_aws_filter`.

| `billing_aws_filter` | Costos importados | Configuración adicional |
| --- | --- | --- |
| `none` (por defecto) | Todos los costos que pueden ver las credenciales | Ninguno |
| `linked_account` | Los costos de las cuentas miembro que indiques (`LINKED_ACCOUNT`) | `billing_aws_linked_accounts` (IDs de cuenta de 12 dígitos, separados por comas) |
| `tag` | Los costos de los recursos que llevan la etiqueta de asignación de costos que indiques | `billing_aws_tag` (`key=value`, por ejemplo `app=my`) |

La configuración más precisa es una cuenta de AWS dedicada a este servidor (puede ser una cuenta miembro de Organizations), importada con `none` (llamando con las credenciales de esa cuenta) o `linked_account` (llamando desde la cuenta de administración).
Los impuestos, las tarifas de soporte y algunos créditos no pueden llevar etiqueta, y pueden quedar fuera de un filtro por etiqueta (aún no verificado en una cuenta real).

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### Activar una etiqueta de asignación de costos (`billing_aws_filter = tag`)

Una etiqueta de un recurso no aparece en los datos de facturación hasta que la activas como etiqueta de asignación de costos.

1. Pon la misma etiqueta (por ejemplo `app=my`) en los recursos de este servidor (servicios de ECS, RDS, buckets de S3, balanceadores de carga, etcétera).
2. En «Cost allocation tags» de la consola de Billing and Cost Management, selecciona la clave de la etiqueta y actívala. Una etiqueta tarda hasta 24 horas en aparecer en esta lista tras adjuntarla, y hasta otras 24 horas tras la activación en aparecer en los datos de facturación.
3. Para reflejar los costos del periodo anterior a la activación, usa «Backfill» en la misma pantalla (la API `StartCostAllocationTagBackfill`).
4. Define `MY_BILLING_AWS_FILTER=tag` y `MY_BILLING_AWS_TAG=app=my`.

<!-- source: metric 3e034dd351bd -->
### Métrica

`billing_aws_metric` es `NetUnblendedCost` (por defecto, más cercana a la factura), que es el costo tras los descuentos (Savings Plans, instancias reservadas, precios negociados), o `UnblendedCost`, que es el costo antes de ellos.
Los valores de Cost Explorer y la factura pueden diferir por el momento de la actualización y el redondeo. Cuando difieren, la factura es la que vale.

<!-- source: kinds-of-failure 2a6f9042f460 -->
### Tipos de fallo

| Tipo | Respuesta de AWS | Cómo solucionarlo |
| --- | --- | --- |
| `permission` | `AccessDeniedException`, credenciales no válidas | Comprueba los permisos de IAM y cómo se pasan las credenciales |
| `not_enabled` | `DataUnavailableException` | Activa Cost Explorer. Para una cuenta vinculada, activa Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer ya no tiene los datos de facturación del periodo que se va a importar |
| `throttled` | `LimitExceededException` | El trabajo reintenta tras una pausa (cuenta para el límite de solicitudes) |
| `unavailable` | 5xx, fallo de red | El trabajo reintenta tras una pausa (cuenta para el límite de solicitudes) |
| `invalid_response` | Una respuesta ilegible, `ValidationException` y similares | Comprueba la configuración (etiqueta, IDs de cuenta) |
| `request_cap` | (no se hace ninguna llamada) | Espera a que pase el día o el mes que limita, o sube el límite |

Reintentar no puede solucionar un fallo que no sea `throttled` ni `unavailable`, así que el servidor lo registra y se detiene, y la importación programada del día siguiente lo intenta de nuevo.

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud (`billing_sources = gcp`)

Google Cloud no tiene ninguna API que devuelva los costos reales, así que el servidor agrega una vez al día la exportación de Cloud Billing a BigQuery (costo de uso estándar) con un `jobs.query` de BigQuery (`billing_gcp_method = bigquery`, el valor por defecto).
Cuando no puedes configurar la exportación, hay también un método sencillo que importa solo un total aproximado a partir de las notificaciones de Pub/Sub de un presupuesto (consulta [Importar desde las notificaciones de un presupuesto](#importing-from-budget-notifications-billing_gcp_method--budget)).
El mes de facturación es el `invoice.month` de la exportación, un mes natural en hora del Pacífico de EE. UU. (con horario de verano). Las fechas se registran en hora del Pacífico tal cual.
Los importes están en la moneda de la cuenta de facturación, sumados como `NUMERIC` por mes de facturación, servicio (`service.description`) y tipo de cargo (`cost_type`), y los créditos (`credits[]`) pasan a ser una línea de crédito por servicio. Las filas de corrección de la exportación que anulan filas anteriores forman parte de la suma.
La exportación no tiene ninguna marca de cierre, así que la importación que se ejecuta a partir del día 5 (hora del Pacífico) del mes siguiente trata el mes como definitivo, y hasta entonces lo registra como una estimación desde el primer día del mes hasta el día del último uso. Una vez definitivo, la importación sigue leyendo el mes hasta el día 15 y registra cualquier diferencia como una corrección.
Un binario compilado con la etiqueta de compilación `no_billing_gcp` rechaza `gcp` (consulta [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### Costo y límites

Una importación es una consulta, y BigQuery factura los bytes que analiza cada consulta (como mínimo 10 MB). Una cuenta pequeña analiza unos 0,3 GB al mes, lo que cabe en el primer 1 TiB mensual del nivel gratuito (compartido con los demás usos de la cuenta de facturación), así que cuesta 0 $.
Para evitar análisis inesperados, cada consulta lleva `maximumBytesBilled` (`billing_gcp_max_bytes_billed`, por defecto 1 GiB). Una consulta que lo superara no se factura y falla, y se registra como un fallo del tipo `bytes_cap`. La administración muestra los bytes facturados.

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### Activar la exportación a BigQuery

El operador lo configura una sola vez, en la consola (no se ha encontrado ninguna forma de configurarlo mediante una API).

1. En un proyecto vinculado a la cuenta de facturación, crea un conjunto de datos de BigQuery (por ejemplo `billing_export`). Elige una ubicación multirregión (`US` o `EU`). Se rellenan con datos retroactivos el mes actual y el anterior (con una sola región, los datos empiezan el día en que lo activas). La ubicación no se puede cambiar después.
   No establezcas caducidad en las tablas. Las filas que caducan no vuelven.
2. En «Billing > Billing export > BigQuery export», activa «Standard usage cost» y selecciona el conjunto de datos del paso 1. La configuración requiere «Billing Account Costs Manager» o «Billing Account Administrator» en la cuenta de facturación y el rol BigQuery User en el proyecto.
3. Tras unas horas o un día, se crea la tabla `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` (los guiones del ID pasan a ser `_`).
4. Define `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`.

Cuando no se encuentra el conjunto de datos o la tabla, la importación falla con `not_found`.

<!-- source: iam-roles f5628acd0d94 -->
### Roles de IAM

Da a la cuenta de servicio del servidor que importa estos dos roles.

| Rol | Dónde concederlo | Uso |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | El proyecto que ejecuta la consulta (`billing_gcp_query_project`, o el proyecto de la tabla cuando está vacío) | `bigquery.jobs.create`. Los bytes analizados se facturan a este proyecto |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | El conjunto de datos de la exportación | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

Cuando la API de BigQuery no está activada en el proyecto de la consulta, la importación falla con `not_enabled`, y cuando falta un rol, con `permission`.

La autenticación usa Application Default Credentials (ADC).

| Entorno de ejecución | Credenciales |
| --- | --- |
| Google Cloud (Cloud Run) | ADC con la cuenta de servicio del servicio. Sin clave y sin configuración |
| AWS ECS | Coloca un archivo de configuración de credenciales de Workload Identity Federation (el tipo de AWS) y pasa su ruta en `GOOGLE_APPLICATION_CREDENTIALS` (aún no verificado en una cuenta real) |
| Tu propio servidor | Workload Identity Federation, o una clave JSON de cuenta de servicio pasada en `GOOGLE_APPLICATION_CREDENTIALS`. Pasa la clave desde un almacén de secretos |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### Elegir qué costos importar (filtrado)

Por defecto el servidor importa todos los costos de la cuenta de facturación. La configuración más precisa es una cuenta de facturación dedicada a este servidor, importada sin filtros.

| Configuración | Costos importados |
| --- | --- |
| `billing_gcp_projects` (IDs de proyecto, separados por comas) | Los costos de los proyectos que indiques (`project.id`) |
| `billing_gcp_label` (`key=value`, por ejemplo `app=my`) | Los costos de los recursos que llevan la etiqueta que indiques (`labels`) |

Cuando defines ambos, el servidor importa solo los costos que coinciden con los dos.
Los costos a nivel de factura, como los impuestos, pueden no llevar ni proyecto ni etiqueta, y entonces quedan fuera del filtro (aún no verificado en una cuenta real).

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### Tipos de fallo

| Tipo | Respuesta de BigQuery | Cómo solucionarlo |
| --- | --- | --- |
| `permission` | 401, 403 (`accessDenied`) | Comprueba los roles de la cuenta de servicio y cómo se pasan las credenciales |
| `not_enabled` | 403 (`accessNotConfigured` y similares) | Activa la API de BigQuery en el proyecto de la consulta |
| `not_found` | 404 | Comprueba `billing_gcp_table` y la configuración de la exportación |
| `bytes_cap` | 400 (`bytesBilledLimitExceeded`) | El análisis superó `billing_gcp_max_bytes_billed`. Sube el límite si la tabla es grande |
| `throttled` | 429, 403 (`rateLimitExceeded`, `quotaExceeded`) | El trabajo reintenta tras una pausa |
| `unavailable` | 5xx, fallo de red, una consulta que no termina | El trabajo reintenta tras una pausa |
| `invalid_response` | Una respuesta ilegible, filas que mezclan monedas dentro de un mes, cualquier otro 400 | Comprueba la tabla de exportación (¿es el costo de uso estándar?) |

Reintentar no puede solucionar un fallo que no sea `throttled` ni `unavailable`, así que el servidor lo registra y se detiene, y la importación programada del día siguiente lo intenta de nuevo.

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### Importar desde las notificaciones de un presupuesto (`billing_gcp_method = budget`)

Es un método sencillo para cuando no puedes configurar la exportación a BigQuery. Una vez al día, el servidor lee desde una suscripción pull las notificaciones que un presupuesto de Cloud Billing envía a un tema de Pub/Sub.
Usa pull en lugar de push, de modo que una notificación nunca despierta al servidor (Cloud Run, por ejemplo).

Este método importa **solo un total aproximado basado en estimaciones**.

- El `costAmount` de una notificación es el costo acumulado estimado del periodo del presupuesto, sin desglose por servicio ni tipo de cargo. El servidor importa solo una línea por mes y presupuesto (el nombre del servicio es el nombre visible del presupuesto, el tipo de cargo es uso y la categoría es otra). Los créditos y los impuestos están en el total según esté configurado el presupuesto.
- Igual que con BigQuery, un mes pasa a ser definitivo en la importación que se ejecuta a partir del día 5 (hora del Pacífico) del mes siguiente. Una notificación de presupuesto no sabe nada del cierre de la factura, así que incluso un valor tratado como definitivo puede diferir del importe de la factura.
- Un mes estimado se registra como el costo desde el primer día del mes hasta el día (hora del Pacífico) en que se publicó la última notificación.
- El filtrado lo decide el alcance del presupuesto (proyectos, servicios, una clave y un valor de etiqueta), y no se usan `billing_gcp_projects` ni `billing_gcp_label`.

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### Preparar el presupuesto, el tema y la suscripción

1. Crea un tema y una suscripción pull que lea solo este servidor. Consulta «Cómo se leen las notificaciones» más abajo para saber el porqué del periodo de retención y de la caducidad.

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. En la consola, en «Billing > Budgets & alerts», crea un presupuesto. Haz que su periodo sea **mensual** (una notificación de un presupuesto que no sea mensual falla con `invalid_response`).
   Selecciona el proyecto de este servidor como alcance y, en «Manage notifications > Connect a Pub/Sub topic to this budget», selecciona el tema del paso 1.
   Crear el presupuesto requiere «Billing Account Costs Manager» o «Billing Account Administrator» en la cuenta de facturación, y `pubsub.topics.setIamPolicy` y `pubsub.topics.list` en el proyecto del tema.
   La primera notificación tarda unas horas en llegar y, después, llegan varias al día.
3. Da a la cuenta de servicio del servidor que importa `roles/pubsub.subscriber` (Pub/Sub Subscriber) en la suscripción. Se usa para extraer mensajes y confirmarlos (`pubsub.subscriptions.consume`).

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. Define `MY_BILLING_GCP_METHOD=budget` y `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`.

La autenticación son las mismas Application Default Credentials que para BigQuery (consulta la tabla de [Roles de IAM](#iam-roles) más arriba).
La etiqueta de compilación `no_billing_gcp` elimina también este método. La biblioteca cliente de Pub/Sub se comparte con `stream = pubsub`, así que solo sale del binario cuando defines también `no_pubsub` (consulta [build-tags.md](../reference/build-tags.md)).

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### Cómo se leen las notificaciones

- Una importación repite Pull hasta que no llega ningún mensaje nuevo, y toma, para cada presupuesto y mes, la notificación con la hora de publicación más reciente. Esto absorbe las entregas duplicadas de Pub/Sub, las notificaciones repetidas con el mismo valor y los cambios de orden, así que recibir la misma notificación muchas veces cuenta una sola vez en el total.
  El número de solicitudes contadas por una importación es el número de Pull, normalmente dos: uno que devuelve mensajes y otro que vuelve vacío. Las confirmaciones hechas al final (`Acknowledge`, `ModifyAckDeadline`) no se cuentan.
- El servidor confirma (ack) solo una notificación que ha sustituido otra más reciente del mismo presupuesto y mes, y una notificación de un mes que ya no se importa (el mes anterior, una vez registrado como definitivo). La última notificación de un mes importado no se confirma sino que se devuelve a la suscripción, de modo que la siguiente importación vuelve a leer la misma notificación y el valor no cambia en un día sin ninguna notificación nueva.
- Por eso, define la retención de mensajes de la suscripción en **al menos 14 días (se recomiendan 31 días)**. La última notificación del presupuesto de un mes llega hacia el día 2 del mes siguiente, y el mes anterior se vuelve a leer hasta el día 15. Un mes cuyas notificaciones han caducado se registra como un costo de 0.
- Por defecto una suscripción caduca tras 31 días sin uso. Define la caducidad en «never» para que sobreviva a un periodo en el que se detuvieron las importaciones.
- No envíes otros mensajes al tema. Un mensaje que no se puede leer como notificación de presupuesto hace fallar esa importación con `invalid_response`, y se confirma para que la siguiente importación no se quede atascada en él.
- Costo: unas pocas decenas de mensajes al día caben en el nivel gratuito de Pub/Sub (10 GiB al mes), así que cuesta 0 $. El servidor no establece ningún límite.

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### Tipos de fallo (notificaciones de presupuesto)

| Tipo | Respuesta de Pub/Sub | Cómo solucionarlo |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Comprueba `roles/pubsub.subscriber` en la suscripción y cómo se pasan las credenciales |
| `not_enabled` | `PERMISSION_DENIED` (`SERVICE_DISABLED`) | Activa la API de Pub/Sub en el proyecto de la suscripción |
| `not_found` | `NOT_FOUND` | Comprueba `billing_gcp_budget_subscription` y si la suscripción ha caducado |
| `throttled` | `RESOURCE_EXHAUSTED` | El trabajo reintenta tras una pausa |
| `unavailable` | `UNAVAILABLE`, `INTERNAL` y similares, fallo de red | El trabajo reintenta tras una pausa |
| `invalid_response` | Un mensaje que no se puede leer como notificación de presupuesto, un presupuesto que no es mensual, dos presupuestos con el mismo nombre visible | Haz el presupuesto mensual, da a los presupuestos nombres visibles distintos y no envíes otros mensajes al tema |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## Un plugin (`billing_sources = plugin`)

Para un proveedor del que el servidor no tiene adaptador, como Azure, Hetzner o Sakura, ejecuta un [plugin](plugins.md) del tipo `billing` y deja que lea los datos del proveedor. El servidor tiene un único origen `plugin`. Un plugin con varios proveedores los agrega por sí mismo y los distingue por los nombres de sus servicios.

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

Los tres valores de configuración son obligatorios con este origen, y el servidor arranca sin llamar al plugin, incluso cuando este está caído.

- `plugin_billing_location` es la zona horaria que delimita los meses de facturación del plugin.
- `plugin_billing_provider` es el nombre del proveedor en los totales, en la página de costos y en el registro de auditoría.

El servidor pide al plugin el mes actual y, como con los demás orígenes, el anterior hasta el día 15 o hasta que sea definitivo. Los límites de los demás orígenes no se aplican: `max_requests` es `2147483647`, lo que significa sin límite, y el plugin decide lo que puede gastar. Informa de las solicitudes y de los bytes que se le facturaron, que el servidor registra y muestra.

El tipo admite TCP sin cifrar, con una advertencia al arrancar, ya que solo transporta costos. Un token de portador requiere `https://` o un socket Unix: consulta [el destino de un plugin](plugins.md#the-destination).

<!-- source: the-call 6a2bcce82ab2 -->
### La llamada

Con `plugin_billing_protocol = json`, el valor por defecto, un plugin solo necesita un servidor HTTP y una biblioteca JSON ([Un plugin sin biblioteca](plugins.md#a-plugin-with-no-library)). `Fetch` dura 2 minutos como máximo, o `plugin_billing_timeout`, por debajo de 5 minutos.

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- Una línea es un mes, un servicio y un tipo de cargo, con sus líneas compensatorias, créditos y reembolsos sumados. `service` tiene como máximo 200 caracteres y ningún carácter de control.
- `coveredStart` y `coveredEnd` están dentro del mes: el mes entero cuando `final` es true, hasta el último día con datos antes.
- `amount` es un decimal sin redondear en unidades mayores, que puede ser negativo. `currency` es un código ISO 4217.
- `chargeType` es `CHARGE_TYPE_` y uno de `USAGE`, `CREDIT`, `REFUND`, `TAX`, `SUPPORT`, `FEE`, `ADJUSTMENT`, `OTHER`.
- `category` es `CATEGORY_` y una de `COMPUTE`, `DATABASE`, `STORAGE`, `NETWORK`, `DOMAIN`, `EMAIL`, `SOFTWARE`, `OTHER`.
- `requests` es como máximo `maxRequests`. `bytesBilled` es una cadena decimal, para un proveedor que factura por bytes.
- Una clave que `OUTCOME_OK` no devuelve no tiene costo en su mes, así que el servidor registra para ella una revisión de 0. Un plugin que no pudo leer un proveedor debe informar de un fallo, no omitir sus líneas.
- Las llamadas pueden repetirse: un reintento de una importación fallida es la misma llamada otra vez.

`maxRequests` 0 prohíbe cualquier solicitud facturada. El servidor no hace ninguna llamada cuando enviaría 0, pero `my plugin test --kind billing` sí la hace, con el campo omitido como permite la asignación de proto3, y espera `OUTCOME_REQUEST_CAP` sin líneas y sin solicitudes.

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### Tipos de fallo

Un plugin informa de un fallo en `outcome`. El servidor conserva las `requests` de un fallo, ignora sus líneas y deja las importaciones anteriores como están, igual que con los demás orígenes.

| Resultado | Tipo | Se reintenta |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | No |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | No |
| `OUTCOME_NOT_FOUND` | `not_found` | No |
| `OUTCOME_THROTTLED` | `throttled` | Sí |
| `OUTCOME_REQUEST_CAP` | `request_cap` | No |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | No |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | No |
| `OUTCOME_UNAVAILABLE` | `unavailable` | Sí |
| Un resultado ausente o desconocido, o una línea o un recuento que incumple las reglas anteriores | `invalid_response` | No |
| Un error HTTP o de Connect, un plazo agotado, una respuesta de más de 4 MiB, ninguna respuesta | `unavailable` | Sí |

Lo que no se reintenta espera a la siguiente importación diaria. El motivo de un fallo está en el registro del plugin, ya que el servidor descarta el mensaje de un error, y cuenta también en [`my.plugin.failures`](running.md).

<!-- source: test-it 10a2c62a53d8 -->
### Probarlo

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

La primera no hace ninguna solicitud facturada si el plugin cumple el contrato. La segunda hace además una obtención real, que puede costar dinero en los proveedores del plugin. Consulta [Probar un plugin](plugins.md#test-a-plugin).
