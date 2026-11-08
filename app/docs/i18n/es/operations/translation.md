<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# Traducir publicaciones con un servicio de traducción

El servidor puede traducir una publicación cuando un lector lo pide, a través de un servicio de traducción que tú configuras: LibreTranslate,
DeepL, Google Cloud Translation o Amazon Translate. Un proveedor queda incorporado cuando el servidor tiene sus claves, y la
[configuración de la instancia](instance-settings.md) `translation_providers` lo activa.

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate es una traducción automática de código abierto que puedes ejecutar tú mismo o usar como servicio alojado en
[libretranslate.com](https://libretranslate.com). Apunta el servidor hacia él con dos valores de configuración de la
[configuración](../reference/configuration.md):

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` es la URL base del servidor de LibreTranslate, con `http:` o `https:` y una ruta
  opcional. Funciona un servidor en una dirección privada, como un contenedor junto a este o `http://10.0.0.5:5000`.
  El servidor no sigue las redirecciones que este devuelva, así que la clave no va a ningún otro sitio.
- `translator_libretranslate_api_key` es un secreto que se lee solo del entorno
  (`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` o `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`). Déjalo sin definir cuando el
  servidor no necesite clave; uno que alojes tú casi nunca la necesita. La clave nunca se muestra a los clientes ni se escribe en
  los registros.

Después añade `libretranslate` a la configuración de la instancia `translation_providers`, en el orden en que quieras que se pruebe, por ejemplo
`translation_providers = libretranslate` solo, o seguido del ID de otro proveedor para recurrir a él.

El servidor pide a LibreTranslate su lista de idiomas antes de la primera traducción y de nuevo cada hora, y
no envía una publicación en un par de idiomas que no figure en ella. Cuando una publicación no tiene idioma, LibreTranslate lo detecta.
En un servidor que ejecutes tú, instala los modelos de idioma que necesiten tus lectores.

**La calidad de la traducción varía según el idioma.** Los modelos de LibreTranslate son buenos para algunos pares y mediocres para otros,
y una traducción desde un idioma con pocos datos de entrenamiento puede perder el sentido. Prueba los idiomas que usa tu
comunidad antes de activarlo, y deja un proveedor comercial después de él en `translation_providers` si lo necesitas.

**Adónde va el texto.** Un servidor que ejecutas tú guarda el texto de las publicaciones en tus propias máquinas y nadie más
lo recibe. Entonces puedes permitirle publicaciones `private` y `direct` con `translation_provider_visibilities`, por ejemplo
`libretranslate=public|local|unlisted|private|direct`, y la
[política de privacidad](privacy-policy-template.md#the-services-you-choose) no cita ningún servicio externo de traducción. Un
LibreTranslate alojado, libretranslate.com o el de otra persona, recibe el texto como cualquier otro proveedor: lee sus condiciones, mantén las
visibilidades predeterminadas y nómbralo en la política.

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL traduce a través de su API con una clave de un plan de API de DeepL. El servidor envía el texto de una publicación con los marcadores
que sustituyen a menciones, enlaces y formato como elementos XML, que DeepL mantiene en su sitio, y nada más de la
publicación.

1. Obtén una clave de autenticación en tu cuenta de DeepL. Una clave del plan gratuito termina en `:fx`.
2. Defínela en la [configuración del despliegue](../reference/configuration.md) y reinicia el servidor:

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   El servidor llama a `https://api-free.deepl.com` para una clave del plan gratuito y a `https://api.deepl.com` para cualquier otra.
   Para pasar por un proxy, define `MY_TRANSLATOR_DEEPL_URL`. La clave es un secreto: el servidor nunca la registra, la incluye en un
   error ni la muestra en un ajuste o en una respuesta de la API de cliente, y la envía solo a esa dirección, sin seguir
   redirecciones.
3. Activa el proveedor con la [configuración de la instancia](instance-settings.md) `translation_providers`, solo o en tu orden:

   ```sh
   my instance set translation_providers deepl
   ```

El servidor pregunta a DeepL qué idiomas traduce la primera vez que traduce, y de nuevo cada día. El idioma de una publicación
pasa a la variante más cercana que tenga DeepL: el inglés a `en-US`, el portugués a `pt-BR`, el chino de Taiwán a `zh-Hant`. Un
idioma que DeepL no tiene pasa al siguiente proveedor, o se informa al lector de que no se puede traducir.

**Cuota.** Cada plan tiene una cuota mensual de caracteres, y un plan de pago puede tener un límite de costo que fijas en tu cuenta
de DeepL. Cuando DeepL informa de la cuota o de su límite de frecuencia, el servidor prueba el siguiente proveedor de `translation_providers`.
Si no queda ninguno, el lector ve que la publicación no se pudo traducir en este momento, y el cliente web usa la traducción
del navegador si lo permites. Los límites mensuales del propio servidor (`translation_user_monthly_characters` y
`translation_server_monthly_characters`) se aplican antes de cualquier llamada, así que fíjalos por debajo de tu cuota de DeepL. DeepL puede contar los
caracteres de forma distinta.

**Datos.** Lee [las condiciones de DeepL](https://www.deepl.com/en/pro-license) antes de activarlo:

- En un plan de pago, DeepL conserva el texto y las traducciones solo el tiempo que necesita para traducirlos, y como máximo 72
  horas cuando investiga un error.
- En el plan gratuito, DeepL puede conservar el texto y las traducciones para siempre, y sus condiciones prohíben enviar datos personales
  a través de él. Las publicaciones suelen contener datos personales, así que usa el plan gratuito solo para probar el proveedor. No le permitas nunca
  publicaciones `private` o `direct` en `translation_provider_visibilities`, y di en tu política de privacidad que DeepL conserva lo
  que recibe.

En un plan de pago puedes permitir publicaciones `private` y `direct`, proveedor por proveedor:

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**Atribución.** Las condiciones de DeepL exigen que su nombre o logotipo se muestre con cada traducción. El cliente web muestra
«DeepL» con un enlace a `https://www.deepl.com` junto a cada traducción suya, así que no tienes que añadir nada.

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) tiene el ID de proveedor `google`. El servidor llama a su método
`TranslateText` con el modelo de traducción general y envía el texto como HTML para que las menciones, los enlaces y las
demás partes que deben quedar tal cual vuelvan en su sitio.

1. Activa la API de Cloud Translation (`translate.googleapis.com`) en el proyecto que paga las traducciones:
   `gcloud services enable translate.googleapis.com --project PROJECT`.
2. Concede a la cuenta de servicio del servidor `roles/cloudtranslate.user` en ese proyecto:
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`.
3. Define `MY_TRANSLATOR_GOOGLE_PROJECT` con el ID del proyecto. `MY_TRANSLATOR_GOOGLE_LOCATION` es `global` de forma predeterminada; puede
   indicar otra ubicación de Cloud Translation que ofrezca el modelo general, como `us-central1`.
4. Reinicia el servidor y añade `google` a la configuración de la instancia `translation_providers`.

El servidor se autentica con Application Default Credentials, como los demás servicios de Google Cloud que usa: en
Cloud Run, la cuenta de servicio del servicio; en otros sitios, el archivo de claves que indica `GOOGLE_APPLICATION_CREDENTIALS`. No se usa
ninguna clave de API. El servidor no hace ninguna llamada al arrancar, así que una API o un rol que falte solo se nota cuando un lector pide una
traducción: el servidor prueba entonces el siguiente proveedor, y el lector ve que la traducción falló cuando no queda ninguno.
Una cuota o un límite de frecuencia del proyecto que informe Google también pasa al siguiente proveedor.

Google cobra por carácter enviado, después de una franja gratuita mensual; consulta los
[precios de Cloud Translation](https://cloud.google.com/translate/pricing). Los límites mensuales del propio servidor también cuentan los
caracteres, pero no exactamente como los factura Google, así que fija además una alerta de presupuesto en Cloud Billing.

Google [declara](https://cloud.google.com/translate/data-usage) que usa el texto solo para prestar la traducción,
lo guarda en memoria solo un breve tiempo, no lo usa para entrenar sus modelos y no lo comparte. El servidor llama al
extremo global de Cloud Translation, donde Google no limita el procesamiento a una sola región.

Los [requisitos de atribución](https://cloud.google.com/translate/attribution) de Google piden que una traducción se muestre como
traducida automáticamente y que junto a ella aparezca la insignia «powered by Google Translate», con un enlace a
`https://translate.google.com`. El cliente web muestra el nombre «Google Translate» con ese enlace junto a cada
traducción de este proveedor. Google ofrece las imágenes de la insignia solo como descarga, así que el servidor no da a los clientes ningún logotipo de
este proveedor; comprueba que lo que ven los lectores cumple los requisitos de Google para tu instancia.

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

Amazon Translate es el proveedor `aws`. El servidor llama a su operación `TranslateText` con las credenciales de la
cadena predeterminada del SDK de AWS, como en los demás servicios de AWS: el rol de ejecución de la tarea o de la instancia, las variables
de entorno `AWS_ACCESS_KEY_ID` y `AWS_SECRET_ACCESS_KEY`, o los archivos de credenciales compartidos. Ninguna clave va a la
configuración del servidor.

1. Da al rol o al usuario con el que se ejecuta el servidor el permiso de IAM `translate:TranslateText`. Para una publicación cuyo
   idioma se desconoce, Amazon Translate detecta el idioma con Amazon Comprehend; las políticas administradas de AWS para
   Amazon Translate conceden `comprehend:DetectDominantLanguage` junto con él, así que concédelo también.
2. Define `translator_aws_region` (`MY_TRANSLATOR_AWS_REGION`) con la región de Amazon Translate, como `us-east-1`. Detectar
   el idioma requiere una región donde también funcione Amazon Comprehend.
3. Reinicia el servidor y activa el proveedor con `translation_providers = aws`, solo o en tu orden de preferencia
   entre los demás ([configuración de la instancia](instance-settings.md)).

Amazon Translate cobra por carácter enviado, con una capa gratuita durante los primeros 12 meses
([precios](https://aws.amazon.com/translate/pricing/)). El servidor envía solo las palabras de una publicación: las menciones, las etiquetas,
los enlaces y el código se quedan en el servidor, y cada uno va como un marcador corto en formato HTML. Mantén los límites mensuales
de `translation_user_monthly_characters` y `translation_server_monthly_characters` dentro de lo que quieras pagar.

**Amazon puede almacenar el texto y usarlo para mejorar sus servicios, posiblemente en otra región, salvo que la
[política de exclusión de servicios de IA](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html) de AWS
Organizations cubra la cuenta.** Sin la exclusión, Amazon no es solo tu proveedor de servicio para ese texto: dilo en tu
política de privacidad ([plantilla de política de privacidad](privacy-policy-template.md#the-services-you-choose)) y no permitas
`private` ni `direct` para `aws` en `translation_provider_visibilities`. De forma predeterminada el proveedor recibe solo publicaciones `public`,
`local` y `unlisted`.

Un servidor compilado con la etiqueta de compilación `no_translator_aws` no tiene el proveedor y se niega a arrancar con
`translator_aws_region` definido ([etiquetas de compilación](../reference/build-tags.md)).
