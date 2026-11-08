<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# Gestión de la clave de cifrado de claves (KEK)

El servidor cifra los secretos que guarda en la base de datos con una clave de cifrado de claves (KEK) que nunca almacena allí.
Esta página explica cómo generar, guardar, respaldar y rotar la KEK, cómo mantenerla en un KMS en su lugar y qué hacer si se pierde o se filtra.
La [referencia de configuración](../reference/configuration.md) enumera todos las configuraciones.

<!-- source: how-it-works 37074c6237d0 -->
## Cómo funciona

![La jerarquía de claves: la clave de cifrado de claves, guardada donde indica key_manager y nunca en la base de datos, envuelve una clave de cifrado de datos creada para cada secreto, que cifra el secreto; la base de datos guarda el ID de la clave, la clave de cifrado de datos envuelta y el secreto cifrado en cinco columnas selladas. La lista siguiente describe cada capa.](images/key-hierarchy.svg)

- Los secretos guardados en la base de datos, como las claves privadas de los actores, se cifran con AES-256-GCM usando una clave de cifrado de datos (DEK) generada para cada secreto.
  La DEK se envuelve con una **clave de cifrado de claves (KEK)** y se guarda junto al texto cifrado (cifrado de sobre).
- La KEK no se guarda en la base de datos, así que un volcado filtrado de la base de datos por sí solo no expone los secretos.
- Cada texto cifrado lleva el **ID de clave** de la KEK que envolvió su DEK. Con `key_manager = local`, el ID de clave es `local:` seguido de 16 dígitos hexadecimales derivados de la KEK,
  de modo que la misma KEK da siempre el mismo ID. Nunca asignas los ID.
- Un texto cifrado queda vinculado al ID de fila y al nombre de columna donde se guarda. Copiarlo a otra fila o columna lo hace imposible de descifrar.
- Las columnas selladas, con los nombres que imprime `my kek status`, son `local_account_private_keys.private_key_enc` (claves privadas de los actores),
  `totp_enrollments.secret_enc` (secretos de la autenticación en dos factores), `login_provider_secrets.sealed_secret` (secretos de cliente de los proveedores de inicio de sesión),
  `webhook_secrets.sealed_secret` (secretos de firma de webhooks) e `instance_secrets.sealed_secret` (la clave de firma del proxy de medios).
- El estado de un inicio de sesión con passkey o mediante un proveedor externo se sella de la misma manera pero no se guarda: el navegador lo conserva durante 30 minutos como máximo.
- La clave de firma (HMAC-SHA256) de las URL del proxy de medios es una clave aleatoria que el servidor genera en su primer arranque y guarda en la base de datos, cifrada como los demás secretos.
  No cambia al rotar la KEK, de modo que las URL del proxy de medios siguen funcionando a través de las rotaciones, y `my kek rewrap` la vuelve a envolver junto con los demás secretos.
- `key_manager` es `local` (el valor por defecto: usa la configuración `kek`), `gcpkms` (la KEK permanece en Google Cloud KMS; consulta [Google Cloud KMS](#google-cloud-kms)),
  `awskms` (la KEK permanece en AWS KMS; consulta [AWS KMS](#aws-kms)) o `plugin` (la KEK permanece en un programa tuyo; consulta [Un plugin](#a-plugin)).

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## Qué ocurre si pierdes la KEK

Sin la KEK, restaurar la base de datos no te permite descifrar las claves privadas de los actores. Una vez perdida una clave privada, los demás servidores ya no pueden verificar las firmas de ese actor,
y **la identidad del actor en el fediverso queda rota** (las relaciones de los seguidores no se pueden trasladar). Trata la KEK con el mismo cuidado que la base de datos.

![Lo que se pierde con la KEK: tras una restauración sin ella, todo lo que no está sellado sigue siendo legible, todo valor sellado falla al abrirse y los actores pierden su identidad. El párrafo siguiente dice lo mismo.](images/kek-loss.svg)

Lo que no está sellado sigue siendo legible: las publicaciones, las cuentas, los seguimientos y el resto de la base de datos, y los archivos multimedia del almacenamiento.
Todo valor sellado falla con un error `unknown key-encryption key`: las claves privadas de los actores, los secretos de la autenticación en dos factores,
los secretos de cliente de los proveedores de inicio de sesión, los secretos de firma de webhooks y la clave de firma del proxy de medios.
Lo mismo ocurre cuando se define otra KEK en lugar de la que envolvió los secretos, o cuando la clave del KMS o la clave del plugin que los envolvió está desactivada o destruida.

<!-- source: generate-a-kek 44b338b7440b -->
## Generar una KEK

Codifica 32 bytes aleatorios con base64 estándar.

```sh
openssl rand -base64 32
```

Donde `openssl` no está disponible, `head -c 32 /dev/urandom | base64` también sirve. No dejes el valor generado en el historial de tu terminal ni en tickets.

<!-- source: store-and-deliver-it 9300565c2a3c -->
## Guardarla y entregarla

La KEK es una configuración secreta, así que solo puede pasarse mediante la variable de entorno `MY_KEK`, o mediante `MY_KEK_FILE`, la ruta de un archivo que contiene el valor. Ponerla en el archivo de configuración (TOML) impide el arranque.

| Destino | Ejemplo de cómo entregarla |
| --- | --- |
| Autoalojado (binario / systemd) | Ponla en un archivo que solo el propietario pueda leer (`chmod 600`) y apunta `MY_KEK_FILE` a él |
| Docker / Compose | Monta un secreto de Docker en `/run/secrets/my_kek` y define `MY_KEK_FILE=/run/secrets/my_kek`. Los archivos de `deploy/docker` lo hacen desde `secrets/kek` ([archivos de Docker Compose](running.md#docker-compose-files)): para rotar, sustituye ese archivo y define `MY_KEK_PREVIOUS` en `.env` |
| Kubernetes / microk8s | Monta un Secret como archivo y apunta `MY_KEK_FILE` a él |
| Cloud Run | Expón un secreto de Secret Manager como archivo o como variable de entorno |
| AWS ECS | Pásala desde Secrets Manager o SSM Parameter Store (SecureString) |

- Da a todos los procesos, como web y worker, la misma KEK.
- La KEK nunca aparece en los registros ni en los mensajes de error (solo el ID de clave).
- `my config check` valida el formato de la KEK (base64 de 32 bytes) e imprime el valor como `(redacted)`. En el perfil `production` falla también con una KEK
  que no es aleatoria, como todo ceros o un byte repetido, y `my serve` registra una advertencia sobre ella (consulta la [lista de comprobación de producción](production-checklist.md)).

<!-- source: back-it-up 29e7bdb3b114 -->
## Respaldarla

**Respalda la base de datos y la KEK, por separado.**

- Guarda la KEK en un lugar distinto de donde están las copias de seguridad de la base de datos (un gestor de contraseñas, un medio sin conexión, un almacén de secretos en otra cuenta, etcétera).
  Si ambas están en un mismo lugar, una filtración de ese lugar hace inútil el cifrado de la base de datos.
- Durante una rotación, incluye las claves de `kek_previous` en la copia de seguridad mientras siga habiendo en la base de datos texto cifrado envuelto con ellas.
- Incluye la KEK en tu procedimiento de restauración y prueba las restauraciones con regularidad ([Copia de seguridad y restauración](backup.md#recovery-drill)). Tras restaurar una base de datos, define la KEK (y `kek_previous`) que estaba en uso al hacer la copia con los mismos valores.
  Con una KEK distinta, todo lo que lee un secreto falla con un error `unknown key-encryption key` que nombra el ID de clave.

<!-- source: rotate-the-kek eb4af3189053 -->
## Rotar la KEK

Una KEK en `kek_previous` se usa solo para descifrar, y los secretos nuevos se cifran siempre con `kek`.
Para que un despliegue gradual, donde procesos antiguos y nuevos se ejecutan a la vez, nunca produzca texto cifrado que algún proceso no pueda leer, hazlo en dos pasos.

![Una rotación de la KEK local en cinco pasos, con los valores de kek y kek_previous después de cada uno: la nueva KEK entra primero en kek_previous, luego intercambia el lugar con la antigua, my kek rewrap mueve los secretos guardados y la KEK antigua sale de la configuración. Los pasos siguientes dicen lo mismo.](images/kek-rotation.svg)

1. Genera una KEK nueva y respáldala.
2. Reinicia todos los procesos con `MY_KEK=<old KEK>` y `MY_KEK_PREVIOUS=<new KEK>`. Ahora todos los procesos pueden descifrar con la KEK nueva.
3. Reinicia todos los procesos con `MY_KEK=<new KEK>` y `MY_KEK_PREVIOUS=<old KEK>`. A partir de ahora los secretos se envuelven con la KEK nueva, y el texto cifrado de la KEK antigua sigue siendo legible.
4. Vuelve a envolver los secretos guardados con la KEK nueva: ejecuta `my kek rewrap` una vez, con la misma configuración que en el paso 3 (consulta [Volver a envolver los secretos guardados](#re-wrap-the-stored-secrets)).
   Cuando termina con 0, ningún secreto de la base de datos necesita ya la KEK antigua.
5. Espera al menos 30 minutos después del paso 3, para que hayan caducado los tokens de inicio de sesión de corta duración emitidos con la KEK antigua, y reinicia todos los procesos con `MY_KEK=<new KEK>` y sin `kek_previous`.
   Conserva la copia de seguridad de la KEK antigua mientras conserves copias de seguridad de la base de datos hechas antes del paso 4: restaurar una de ellas la necesita de nuevo.

`kek_previous` acepta varias claves separadas por comas. Indicar dos veces la misma clave, o indicar la misma clave que `kek`, impide el arranque.

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### Volver a envolver los secretos guardados

`my kek rewrap` sustituye solo la clave de cifrado de datos envuelta de cada secreto y conserva su texto cifrado; nunca escribe ni imprime un secreto.

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` imprime una línea por columna e ID de clave, marca el ID de clave de `kek` con `(current)` y termina indicando si algún secreto necesita todavía una clave de `kek_previous`.
  Un ID de clave que no es `kek` ni está en `kek_previous` significa que falta una KEK en la configuración.
- `my kek rewrap` lee los secretos en lotes de 100 y escribe cada lote en una transacción corta. Imprime, por columna, cuántos secretos volvió a envolver y cuántos ya estaban envueltos con `kek`.
  Puede ejecutarse mientras el servidor está atendiendo, y es seguro detenerlo o volver a ejecutarlo en cualquier momento: omite los secretos ya envueltos con `kek`, y un secreto que el servidor reescribió entretanto se deja como lo escribió el servidor.
- Un secreto que no se puede volver a envolver se deja sin cambios y se informa con su ID de fila, y el comando termina con 1. O bien su KEK no está configurada
  (el mensaje nombra el ID de clave: añade esa KEK a `kek_previous` y ejecútalo de nuevo), o bien el valor guardado está dañado o se copió de otra fila.
  No quites una KEK de `kek_previous` mientras `my kek status` siga contando secretos bajo ella.

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

Con `key_manager = gcpkms`, una clave simétrica de Cloud KMS (una CryptoKey con el propósito `ENCRYPT_DECRYPT`) envuelve las claves de cifrado de datos, y la KEK nunca sale de Cloud KMS.
Un volcado filtrado de la base de datos junto con una configuración filtrada siguen sin exponer los secretos: descifrarlos requiere el permiso para usar la clave.

- Define `kms_key` con el nombre de recurso de la clave, sin versión:
  `projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`. Los nombres de más de 248 bytes se rechazan al arrancar.
  El ID de clave de los secretos guardados es `gcpkms:` seguido de este nombre.
- El servidor se autentica con Application Default Credentials: en Cloud Run, la cuenta de servicio del servicio; en otros lugares, `GOOGLE_APPLICATION_CREDENTIALS`.
- Concede a esa cuenta de servicio `roles/cloudkms.cryptoKeyEncrypterDecrypter` sobre la clave (no sobre el proyecto), para todos los procesos, como web y worker, y para los comandos `my kek status` y `my kek rewrap`.
  Sin él, leer un secreto falla con un error que nombra el rol; una versión de clave ausente o desactivada falla igualmente, y esos errores no se reintentan hasta que se corrija la configuración.
  Un error de cuota o un Cloud KMS no disponible es temporal, y el trabajo que necesitaba el secreto se reintenta.
- El servidor conserva durante `kms_dek_cache_ttl` (por defecto `10m`) una clave de cifrado de datos que Cloud KMS descifró, de modo que la mayoría de las lecturas de un secreto no llaman a Cloud KMS.
- La etiqueta de compilación `no_gcpkms` deja el adaptador fuera del binario ([Etiquetas de compilación](../reference/build-tags.md)).

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### Rotar la clave en Cloud KMS

Una rotación en Cloud KMS (a mano, o con un periodo de rotación en la clave) añade una nueva versión primaria y mantiene activadas las versiones antiguas.
Los secretos nuevos se envuelven con la versión nueva, el servidor descifra cada secreto con la versión que lo envolvió y el ID de clave no cambia, así que **una rotación no necesita `my kek rewrap`**.

**No desactives ni destruyas una versión antigua mientras haya secretos que aún la necesiten**: un secreto envuelto con una versión desactivada o destruida ya no se puede descifrar.
Cloud KMS no indica qué secretos envolvió una versión, así que, para retirar material de clave, traslada primero los secretos a una clave nueva:

1. Crea una clave nueva (puede estar en el mismo llavero) y concede a la cuenta de servicio el rol sobre ella.
2. Reinicia todos los procesos con `kms_key` definido con la clave nueva y `kms_key_previous` con la antigua. La clave antigua ahora solo descifra.
3. Ejecuta `my kek rewrap`, con la misma configuración. Cuando `my kek status` ya no cuente secretos bajo el ID de la clave antigua, quita `kms_key_previous` y reinicia.
4. Solo entonces desactiva las versiones de la clave antigua, y mantenlas recuperables mientras conserves copias de seguridad de la base de datos hechas antes del paso 3: restaurar una de ellas las necesita de nuevo.

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

Con `key_manager = awskms`, una clave simétrica de AWS KMS (especificación de clave `SYMMETRIC_DEFAULT`, uso de clave `ENCRYPT_DECRYPT`, administrada por el cliente) envuelve las claves de cifrado de datos, y la KEK nunca sale de AWS KMS.
Un volcado filtrado de la base de datos junto con una configuración filtrada siguen sin exponer los secretos: descifrarlos requiere el permiso para usar la clave.

- Define `kms_key` con el ARN de la clave (`arn:aws:kms:{region}:{account}:key/{key-id}`), un alias (`alias/{name}` o su ARN) o el ID de clave.
  Al arrancar, el servidor resuelve el nombre al ARN de la clave, y el ID de clave de los secretos guardados es `awskms:` seguido de ese ARN.
  Por tanto, apuntar un alias a otra clave no cambia nada hasta el siguiente arranque, y entonces solo para los secretos nuevos: los secretos ya guardados siguen nombrando la clave antigua, así que defínela en `kms_key_previous` (consulta [Retirar una clave de AWS KMS](#retire-an-aws-kms-key)).
  Las claves administradas por AWS (`alias/aws/...`) se rechazan; solo el servicio de AWS que es propietario de una puede usarla.
- El servidor se autentica con la cadena de credenciales por defecto del SDK de AWS: en ECS, el rol de la tarea; en EC2, el perfil de instancia; en otros lugares, el entorno (`AWS_ACCESS_KEY_ID` y similares) o los archivos de configuración compartidos.
  La región es la del ARN de la clave; para un alias o un ID de clave, define `AWS_REGION`.
- Permite `kms:Encrypt`, `kms:Decrypt` y `kms:DescribeKey` sobre la clave, para todos los procesos, como web y worker, y para los comandos `my kek status` y `my kek rewrap`. Una política del rol de IAM:

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  El recurso es el ARN de la clave incluso cuando `kms_key` es un alias. La política de la clave debe permitir además que las políticas de IAM de la cuenta concedan acceso (la política de clave por defecto lo hace).
- El servidor no arranca cuando la clave no existe, está desactivada o pendiente de eliminación, o no es una clave de cifrado simétrica. Más adelante, un permiso ausente o una clave desactivada hace fallar la lectura de un secreto con un error que nombra la causa,
  y esos errores no se reintentan hasta que se corrija la configuración. La limitación de tasa (`ThrottlingException`) o un AWS KMS no disponible es temporal, y el trabajo que necesitaba el secreto se reintenta.
- El contexto de cifrado de cada llamada contiene el ID de fila y el nombre de columna del secreto (`{"my-aad": ...}`, en base64url). CloudTrail lo registra en texto claro; no contiene ningún secreto.
- Costo: AWS cobra una tarifa mensual por cada clave administrada por el cliente (1 USD en el momento de escribir esto, más lo mismo por cada una de las dos primeras rotaciones de su material de clave) y una tarifa por solicitud por encima de un nivel gratuito mensual
  (consulta los [precios de AWS KMS](https://aws.amazon.com/kms/pricing/)). El servidor conserva durante `kms_dek_cache_ttl` (por defecto `10m`) una clave de cifrado de datos que AWS KMS descifró,
  de modo que un servidor con diez actores que entregan todo el tiempo hace unas 43 000 solicitudes al mes por proceso, unos pocos céntimos por encima del nivel gratuito.
- La etiqueta de compilación `no_awskms` deja el adaptador fuera del binario ([Etiquetas de compilación](../reference/build-tags.md)).

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### Retirar una clave de AWS KMS

La rotación automática de claves en AWS KMS conserva el ARN de la clave y su material de clave anterior, así que el ID de clave no cambia y **una rotación no necesita `my kek rewrap`**.

Para dejar de usar una clave, o para apuntar un alias a otra clave, traslada primero los secretos a la clave nueva. **No desactives la clave antigua ni programes su eliminación mientras haya secretos que aún la necesiten**: un secreto envuelto con ella ya no se puede descifrar.

1. Crea la clave nueva y permite al rol usarla.
2. Reinicia todos los procesos con `kms_key` definido con la clave nueva y `kms_key_previous` con la antigua (su ARN, no el alias). La clave antigua ahora solo descifra.
3. Ejecuta `my kek rewrap`, con la misma configuración. Cuando `my kek status` ya no cuente secretos bajo el ID de la clave antigua, quita `kms_key_previous` y reinicia.
4. Solo entonces desactiva la clave antigua, y consérvala mientras conserves copias de seguridad de la base de datos hechas antes del paso 3: restaurar una de ellas la necesita de nuevo.

<!-- source: a-plugin 43023e60bac9 -->
## Un plugin

Con `key_manager = plugin`, un [plugin](plugins.md) tuyo guarda la KEK, en Vault, en un HSM o donde elija, y envuelve las claves de cifrado de datos para el servidor.

**El plugin está en el mismo límite de confianza que la KEK.** Ve en texto claro todas las claves de cifrado de datos, así que quien lo controle, o controle el host donde se ejecuta, puede leer todos los secretos del servidor, incluidas las claves privadas de los actores.
Dale la protección que das a la KEK. Por eso también ninguna configuración salvo el archivo de configuración y el entorno puede cambiar su dirección.

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- La dirección es `unix://` o `https://`. El servidor rechaza `http://`, también en loopback, ya que las claves viajarían en texto claro. Las demás configuraciones del destino están en [Plugins](plugins.md#the-destination).
- Ejecuta `my plugin test --kind key_manager <address>` antes de apuntar el servidor a un plugin ([Probar un plugin](plugins.md#test-a-plugin)). Envuelve solo claves que inventa y descarta.
- El ID de clave de los secretos guardados es `plugin:` seguido del ID que da el plugin a su KEK, que es de 1 a 248 bytes de ASCII visible.
- **Conserva todas las KEK que algún secreto guardado siga nombrando.** Una rotación en el plugin se muestra como un ID nuevo en el siguiente envoltorio. Los secretos envueltos antes siguen nombrando el ID antiguo, y el plugin debe poder abrirlos todavía. `my kek rewrap` los traslada al ID actual, y `my kek status` cuenta los que quedan.
- Un plugin y un KMS nunca sirven a la vez: `kms_key` y `kms_key_previous` se rechazan con un plugin, y también `plugin_key_manager_address` con `key_manager = gcpkms` o `awskms`.
- La etiqueta de compilación `no_plugins` deja el adaptador fuera del binario ([Etiquetas de compilación](../reference/build-tags.md)).

<!-- source: when-the-server-starts 5faeda92e43b -->
### Cuando el servidor arranca

Todos los procesos, y todos los comandos de `my` que abren la base de datos, como `my kek status`, hacen esto antes que nada. Es el único tipo de plugin cuyo fallo impide un arranque.

1. Pide al plugin el ID de su KEK actual.
2. Ejecuta una autoprueba con una clave aleatoria que descarta: un envoltorio y un desenvoltorio deben devolver la clave, un desenvoltorio con datos autenticados distintos debe responder `OUTCOME_DECRYPT`, dos envoltorios de la clave deben diferir, y el valor envuelto no debe contener la clave.

Un plugin que no responde, o que responde `503`, se prueba de nuevo durante 30 segundos como máximo, para un sidecar que arranca después del servidor. Pasado ese tiempo el proceso termina con un error. Una respuesta incorrecta, y cualquier otro fallo, como un token rechazado, lo detiene de inmediato: la autoprueba existe para detener a un plugin que ignora los datos autenticados o deja pasar las claves tal cual, y esperar no arregla uno así.

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### Cuando no se puede llegar al plugin

El servidor falla en cerrado. Sea lo que sea lo que vaya mal, una llamada rechazada o con el tiempo agotado, o una respuesta vacía, desconocida o mal formada, la operación que necesitaba la clave falla con un error. Nunca guarda un secreto en texto claro, nunca cambia a la KEK local ni a otra clave, y nunca trata un fallo como un resultado. Un trabajo que falla así se reintenta más tarde, y una solicitud falla para la persona que la hizo. Cada llamada fallida cuenta en [`my.plugin.failures`](running.md) por tipo y código de Connect, y el registro no contiene ninguna clave, dato autenticado ni valor envuelto, ni nada de lo que dijo el plugin.

El envoltorio nunca se guarda en caché, y un valor sellado hace un momento no tiene nada en la caché, así que esto se detiene de inmediato:

- Crear una cuenta, que genera las claves del actor.
- Configurar la autenticación en dos factores, y registrar una passkey o iniciar sesión con una, cuyas ceremonias mantienen su estado sellado.
- Iniciar sesión mediante un proveedor externo, y guardar el secreto de cliente de un proveedor.
- Registrar un webhook, y `my kek rewrap`.

Leer un secreto guardado solo funciona mientras el proceso conserva su clave de cifrado de datos de una lectura anterior, durante `kms_dek_cache_ttl` como máximo (por defecto `10m`). Pasado ese tiempo, o para un secreto que aún no ha leído, esto se detiene también:

- La entrega de federación y las obtenciones firmadas, que necesitan las claves privadas de los actores. La entrega se pone al día mediante los reintentos de sus trabajos, una vez que el plugin vuelve.
- Comprobar un código de dos factores al iniciar sesión.
- La entrega de webhooks.

Un reinicio se detiene como se describió arriba, así que **no reinicies el servidor para arreglar una caída del plugin**: un servidor que está en marcha sigue sirviendo lo que cubre su caché, y uno que se reinicia no sirve nada hasta que el plugin responda. Lo que no necesita ningún secreto, como leer cronologías y publicar, continúa. La clave de firma del proxy de medios se lee al arrancar y permanece en memoria. Un certificado de cliente o un archivo de CA renovado tampoco necesita reinicio: el servidor vuelve a leer los archivos cuando cambian ([Plugins](plugins.md#authentication)).

<!-- source: the-contract 6183eb7a7dfa -->
### El contrato

Las RPC son las de `my.plugin.key_manager.v1.KeyManagerService`, con un plazo por defecto de 5 segundos y mensajes de 128 KiB como máximo en cada sentido.

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` es el ID propio del plugin, sin `plugin:`. `UnwrapKey` puede nombrar un ID más antiguo que el actual.
- `wrapped` es de 1 a 65 535 bytes y no debe contener la clave. Dos envoltorios de una clave difieren.
- `aad` son datos autenticados que contienen ID de fila y nombres de columna, y `UnwrapKey` debe fallar a menos que sean los que recibió `WrapKey`. Eso es lo que impide que un secreto se copie a otra fila.
- **Nunca registres `dek`, `aad` ni `wrapped`.**

| `outcome` | Significado |
| --- | --- |
| `OUTCOME_OK` | La llamada tuvo éxito |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey`: el plugin no tiene la KEK de `keyId` |
| `OUTCOME_DECRYPT` | `UnwrapKey`: `wrapped` o `aad` no se autentican |

Responde con un error HTTP o de Connect cuando no puedas responder, por ejemplo cuando el almacén de claves está fuera de alcance. No respondas entonces `OUTCOME_DECRYPT` ni `OUTCOME_UNKNOWN_KEY`: para el servidor, dicen que el valor en sí es incorrecto. El servidor no reintenta una llamada por sí mismo, así que un plugin puede recibir la misma llamada de nuevo desde un trabajo.

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## Pasar de la KEK local a un KMS y viceversa

Un ID de clave empieza con el tipo de gestor de claves que la envolvió (`local:`, `gcpkms:`, `awskms:`, `plugin:`). El servidor envuelve los secretos nuevos con el `key_manager` actual
y desenvuelve cada secreto guardado con el gestor de claves que nombra su ID de clave, así que pasar de la KEK local a un KMS es una rotación entre tipos.

- Con un KMS, `kek` y `kek_previous` solo descifran: abren los secretos todavía envueltos con la KEK local, y todo secreto nuevo se envuelve con `kms_key`.
  `kms_key_previous` enumera claves anteriores del mismo KMS, que también solo descifran.
- Con `key_manager = local`, `kms_key` y `kms_key_previous`, cuando están definidos, solo descifran: abren los secretos todavía envueltos con el KMS.
- De la KEK local a un KMS:
  1. Crea la clave y concede los permisos como en [Google Cloud KMS](#google-cloud-kms) o [AWS KMS](#aws-kms).
  2. Reinicia todos los procesos con `key_manager = gcpkms` o `awskms`, `kms_key` definido con la clave y la KEK actual movida de `kek` a `kek_previous`.
     Los secretos ahora se envuelven con el KMS, y los envueltos con la KEK local siguen abriéndose.
  3. Ejecuta `my kek rewrap` con la misma configuración.
  4. Cuando `my kek status` ya no cuente secretos bajo un ID de clave `local:`, quita `kek_previous` y reinicia. Conserva la copia de seguridad de la KEK mientras conserves copias de seguridad de la base de datos hechas antes del paso 3.
- De un KMS de vuelta a la KEK local, haz lo mismo a la inversa: define `key_manager = local` y una `kek` nueva, y mantén definido `kms_key`, que ahora solo descifra,
  hasta que `my kek rewrap` haya trasladado todos los secretos y `my kek status` ya no cuente secretos bajo un ID de clave `gcpkms:` o `awskms:`.
  Las claves de `kms_key` y `kms_key_previous` deben pertenecer entonces todas al mismo KMS.
- Para pasar del KMS de una nube al de otra, pasa primero a la KEK local y luego al otro KMS; no hay un paso directo.
- Un plugin se mueve de la misma manera, solo a través de la KEK local:
  - Hacia él, reinicia todos los procesos con `key_manager = plugin`, `plugin_key_manager_address` y la KEK actual movida de `kek` a `kek_previous`, y luego ejecuta `my kek rewrap`. Cuando `my kek status` ya no cuente secretos bajo un ID de clave `local:`, quita `kek_previous` y reinicia.
  - Desde él, reinicia todos los procesos con `key_manager = local`, una `kek` nueva y `plugin_key_manager_address` mantenido, que ahora solo descifra, y luego ejecuta `my kek rewrap`. Cuando `my kek status` ya no cuente secretos bajo un ID de clave `plugin:`, quita la dirección y reinicia. Hasta entonces el plugin debe seguir superando [las comprobaciones del arranque](#when-the-server-starts), que incluyen un envoltorio.
- El servidor conserva durante `kms_dek_cache_ttl` (por defecto `10m`, como máximo `kms_dek_cache_entries`, por defecto `1024`) una clave de cifrado de datos que el KMS o el plugin descifró,
  de modo que un secreto en uso llega a él aproximadamente una vez por periodo y proceso. `0` en cualquiera de las dos configuraciones desactiva la caché.

<!-- source: if-the-kek-leaks be6e788e4cea -->
## Si la KEK se filtra

Si solo se filtró la KEK, rótala como se explica arriba. Si pueden haberse filtrado tanto la KEK como la base de datos, la rotación no te protege:
las claves privadas de los actores en la base de datos ya son legibles, así que hay que regenerar las propias claves de los actores.

<!-- source: web-push-vapid-key aa32321d1e4a -->
## Clave VAPID de Web Push

Web Push se firma con la clave VAPID del servidor (P-256, RFC 8292). Es un secreto distinto de la KEK y no se guarda en la base de datos.

- Crea un par de claves con `my config vapid-key`, y da a todos los procesos la clave privada (el valor de `vapid_private_key = ...`) mediante `MY_VAPID_PRIVATE_KEY` o `MY_VAPID_PRIVATE_KEY_FILE`.
  El servidor deriva la clave pública de la privada y se la entrega a los clientes, así que no la configuras.
- Si no la defines, Web Push queda desactivado y los clientes no lo ofrecen. En el perfil `production`, `my config check` y el arranque de `my serve` avisan de ello.
- Los navegadores vinculan una suscripción a la clave pública, así que cambiar la clave detiene la entrega a las suscripciones existentes. Cuando un usuario vuelve a activar las notificaciones push en la configuración, el cliente se suscribe de nuevo con la clave nueva.
  No cambies la clave salvo que se haya filtrado; si se filtra, cambia a una clave nueva y pide a los usuarios que vuelvan a activar las notificaciones.
