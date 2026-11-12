<!-- source: plugins 4a60f7abbec9 -->
# Plugins

Un plugin es un programa o servicio aparte que tú ejecutas y al que el servidor entrega una tarea concreta a través de la red.
El servidor solo se conecta a una dirección: no inicia, detiene ni vigila el plugin. Ejecútalo como sidecar, bajo systemd
o en otra máquina.

De forma predeterminada no se delega nada. Un servidor sin dirección de plugin no abre ninguna conexión ni inicia ningún proceso.
La etiqueta de compilación [`no_plugins`](../reference/build-tags.md) quita el cliente de plugins del binario.

![Dónde encajan los plugins: el proceso my entrega seis puertos a un plugin y consulta cinco hooks de decisión, todo a través de un único cliente de plugins que se conecta a los plugins que ejecutas; la parte inferior del diagrama muestra qué hace cada uno cuando una llamada falla. La lista siguiente describe cada parte.](images/plugins.svg)

De arriba abajo en el diagrama:

- **El proceso my**, con cualquier `role`, hace dos tipos de llamadas a un plugin.
  - **Puertos que entrega**: un plugin hace una tarea en lugar de una pieza integrada, y cada uno está activo solo cuando una configuración
    nombra `plugin`: `translator` (la configuración de la instancia `translation_providers = plugin`), `billing` (`billing_sources` incluye
    `plugin`), `mailer` (`mail = plugin`), `media_image`, `media_video` y `media_audio` (`media_image = plugin` y así
    sucesivamente), `key_manager` (`key_manager = plugin`) y `search` (`search = plugin`).
  - **[Hooks de decisión](decision-hooks.md)**: un plugin responde si se acepta algo, y se le consulta salvo que el modo del hook
    `hook_<hook>_mode` sea `off`: `inbound.status` para publicaciones de otros servidores, `inbound.follow` para seguimientos de
    otros servidores, `signup.check` para registros, `media.check` para subidas, `local.status` para publicaciones de cuentas locales,
    `inbound.actor` para los perfiles de cuentas de otros servidores y `local.profile` para cambios de perfil de cuentas
    locales.
- **El cliente de plugins** hace todas las llamadas. Cada tipo tiene un destino, `plugin_<kind>_address`, y un plazo,
  `plugin_<kind>_timeout`, y ejecuta como máximo 8 llamadas a la vez (2 para cada tipo de multimedia). Se conecta mediante un socket Unix, `https://` o,
  solo para [algunos tipos](#plain-tcp), `http://`.
- **Tus plugins** son programas aparte que tú ejecutas, como sidecar, bajo systemd o en otra máquina. El servidor solo
  se conecta a ellos.
- **Cuando una llamada falla o agota su plazo** ([detalles](#when-a-call-fails)):
  - `billing`, `mailer` y los tipos de multimedia: el trabajo que hizo la llamada se ejecuta de nuevo más tarde.
  - `translator`: el servidor consulta al siguiente proveedor y, cuando no queda ninguno, el lector ve que la traducción falló.
  - `key_manager` falla cerrado: lo que necesita una clave falla y el servidor no se inicia sin el plugin.
  - `search`: el trabajo de una escritura se ejecuta de nuevo más tarde y una búsqueda falla, de modo que la persona ve que la búsqueda no está disponible.
  - Un hook de decisión hace lo que dice `hook_<hook>_on_failure`: `pass`, `retry` o, para registros, `require_approval`.

<!-- source: kinds 73da92ef7563 -->
## Tipos

Cada tipo tiene su propio destino, en las configuraciones `plugin_<kind>_*`. Un punto en el nombre de un tipo se convierte en `_` en la
clave, de modo que `inbound.status` tiene `plugin_inbound_status_address`.

| Tipo | Qué transporta | TCP sin cifrar (`http://`) | Protocolo predeterminado | Plazo predeterminado |
| --- | --- | --- | --- | --- |
| `translator` | El texto de una publicación que un lector pidió traducir | **Rechazado** | `json` | 10 s |
| `search` | El texto, el autor, la visibilidad y el idioma de las publicaciones `public` y `local`, y las consultas de las personas que han iniciado sesión | **Rechazado** | `json` | 10 s |
| `billing` | Los costos de la instancia | Permitido | `json` | 2 min |
| `mailer` | Direcciones y secretos como enlaces de confirmación | **Rechazado** | `json` | 30 s |
| `media_image` | Imágenes subidas, con sus metadatos, que pueden contener una ubicación | Permitido | `proto` | 4 min |
| `media_video` | Videos subidos, con sus metadatos, que pueden contener una ubicación | Permitido | `proto` | 4 min |
| `media_audio` | Archivos de audio subidos, con sus metadatos | Permitido | `proto` | 4 min |
| `key_manager` | Claves de cifrado de datos en texto plano | **Rechazado** | `json` | 5 s |
| `inbound.status` | El texto de las publicaciones entrantes | **Rechazado** | `json` | 3 s |
| `local.status` | El texto de las publicaciones locales | **Rechazado** | `json` | 1 s |
| `local.profile` | El perfil público de las cuentas locales, sin imágenes | Permitido | `json` | 1 s |
| `inbound.follow` | Las dos partes de un seguimiento | Permitido | `json` | 3 s |
| `inbound.actor` | Los perfiles públicos de cuentas de otros servidores | Permitido | `json` | 3 s |
| `signup.check` | Direcciones de correo electrónico y direcciones IP | **Rechazado** | `json` | 2 s |
| `media.check` | Una pequeña versión procesada del archivo multimedia, nunca el original | Permitido | `json` | 10 s |

Las configuraciones que hacen que el servidor use un destino pertenecen a cada función: los últimos cinco tipos de la tabla son los
[hooks de decisión](decision-hooks.md). Esta página cubre lo que comparten.

El servidor envía como máximo el límite fijo de un tipo por mensaje y lee como máximo otro. Una llamada que supera el límite falla y
el trabajo que la hizo se reintenta.

<!-- source: the-destination f397f6e80550 -->
## El destino

Las configuraciones existen solo en el archivo de configuración y en el entorno
([referencia de configuración](../reference/configuration.md)). Ninguna configuración de la instancia, página de administración ni API puede mostrarlos
o cambiarlos, de modo que una cuenta de administrador robada no puede redirigir publicaciones ni claves. Un cambio surte efecto al reiniciar.

`plugin_<kind>_address` tiene una de tres formas. El servidor comprueba la forma al iniciarse sin resolver el nombre, y
resuelve el nombre en cada conexión.

- `unix:///run/my/mailer.sock`: un socket Unix.
- `https://mailer.internal:8443`: TLS sobre TCP. El servidor siempre verifica el certificado, frente a
  `plugin_<kind>_tls_ca_file` o las raíces del sistema, y ninguna configuración lo desactiva.
- `http://billing.internal:5000`: TCP sin cifrar.

<!-- source: authentication 2fe4df9369ac -->
### Autenticación

Elige una:

- **Un socket Unix:** lo protegen los permisos del archivo del socket.
- **mTLS:** `plugin_<kind>_tls_cert_file` y `plugin_<kind>_tls_key_file`, con una dirección `https://`.
- **Un token de portador:** `plugin_<kind>_token` (o `MY_PLUGIN_<KIND>_TOKEN_FILE`), enviado como `Authorization: Bearer`, solo por
  `https://` o un socket Unix. Un token con una dirección `http://` es un error de configuración.

Una dirección `https://` sin token ni certificado de cliente se inicia con una advertencia, ya que el plugin no puede distinguir
al servidor de cualquier otro que llegue a él.

El servidor lee los archivos de `plugin_<kind>_tls_ca_file`, `plugin_<kind>_tls_cert_file` y
`plugin_<kind>_tls_key_file` al iniciarse, y un archivo ausente o ilegible lo detiene. Después los vuelve a mirar
en busca de cambios como máximo cada 5 segundos, cuando abre una conexión nueva, de modo que un certificado de corta duración o una CA que un
gestor de certificados como Vault, cert-manager o SPIFFE renueva en el mismo lugar funciona sin reiniciar. Las conexiones ya
abiertas conservan el certificado con el que empezaron hasta que se cierran. Mientras una renovación está a medio escribir, como un nuevo
certificado cuya clave aún no está en su sitio, el servidor conserva los archivos que leyó por última vez y lo intenta de nuevo en la siguiente
conexión. El token se lee solo al iniciarse.

<!-- source: plain-tcp cbb88078173e -->
### TCP sin cifrar

`mailer`, `key_manager`, `translator`, `search`, `inbound.status`, `local.status` y `signup.check` rechazan `http://`: el servidor no se inicia. Eso vale
también para el bucle local (`http://127.0.0.1`), ya que una dirección de bucle local no impide que otros procesos del equipo lean el
tráfico. Pon ese sidecar en un socket Unix en un volumen compartido.

Los demás tipos aceptan `http://`, y el servidor registra al iniciarse una advertencia que nombra el tipo y la dirección. Nada
cifra ni autentica ese tráfico, y no se puede usar ningún token de portador sobre él. Mantener cerrada esa red es tu
responsabilidad: lo que enumera la tabla anterior la cruza en claro.

<!-- source: protocols eb5a03718f1b -->
## Protocolos

`plugin_<kind>_protocol` indica lo que envía el servidor, y debe ser lo que acepta el plugin.

| Valor | Lo que envía el servidor | Para |
| --- | --- | --- |
| `json` | El protocolo unario de Connect con `application/json`, sobre HTTP/1.1 en conexiones sin cifrar | Un plugin construido con una biblioteca de Connect, o solo con un servidor HTTP y una biblioteca JSON |
| `proto` | El protocolo de Connect con `application/proto` | Un plugin construido con una biblioteca de Connect |
| `grpc` | gRPC sobre HTTP/2 | Un plugin que solo habla gRPC |

Un plugin responde con `415` al protocolo equivocado. Los tipos de multimedia transmiten en ambos sentidos, lo que requiere HTTP/2, así que aceptan `proto`
o `grpc` y `json` es un error de configuración.

Las definiciones del protocolo son los archivos `.proto` de `api/plugin/` del repositorio. Hay un paquete para cada
tipo de puerto y no comparten tipos; todos los hooks de decisión comparten un solo paquete, `my.plugin.hook.v1`.
[El protocolo de plugins](../plugins/overview.md) tiene la referencia de cada paquete, generada a partir de esos archivos.

<!-- source: when-a-call-fails 1caa41ab429e -->
## Cuando una llamada falla

Una llamada espera su turno entre las llamadas de su tipo que se ejecutan a la vez, y la espera cuenta en su plazo y en la
métrica [`my.plugin.slot_wait`](running.md). Una llamada que falla cuenta en la métrica [`my.plugin.failures`](running.md) por tipo y código de Connect. El servidor descarta el
mensaje que un plugin pone en un error, así que los motivos de los fallos de un plugin están en el propio registro del plugin. El servidor
no reintenta una llamada por sí mismo: lo hace el trabajo que la hizo, así que un plugin debe tolerar la misma llamada dos veces.

Un plugin caído cuando el servidor se inicia no detiene nada, salvo `key_manager`: el servidor no se inicia sin él, y
[lo que se detiene sin él](key-management.md#when-the-plugin-is-unreachable) es parte de su configuración que conviene leer primero.

Una llamada tiene el plazo de `plugin_<kind>_timeout`, y un plugin lo recibe como `Connect-Timeout-Ms` (`grpc-timeout` en
gRPC). Un plugin puede ignorar esa cabecera: el servidor deja de esperar de todos modos. El plazo debe mantenerse por debajo de 30 segundos para los
tipos por los que una solicitud puede esperar (`key_manager`, `translator`, `search`, `signup.check`, `media.check`, `local.status`, `inbound.actor` y `local.profile`), y
por debajo de 5 minutos, lo que puede durar un trabajo, para los demás.

<!-- source: test-a-plugin 5d8195bb27f5 -->
## Probar un plugin

`my plugin test` ejecuta el contrato de un tipo contra un plugin en funcionamiento e informa de cada caso. No lee ninguna configuración,
así que solo necesita el binario `my`.

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

El código de salida es 1 cuando falla un caso. Las opciones significan lo que significan las configuraciones: `--protocol`, `--token-file`, `--tls-ca-file`,
`--tls-cert-file`, `--tls-key-file` y `--timeout`. Aplica las mismas reglas que el servidor, así que `--kind mailer` rechaza una
dirección `http://`. El plugin que se prueba puede ser uno real, así que un caso que hace más que la propia llamada se ejecuta solo cuando
una opción lo pide. Un tipo cuyo contrato la versión aún no tiene informa de `reachable` y un `SKIP`.

`--kind billing` comprueba solo que el plugin se niega a gastar nada cuando no se permite ninguna solicitud facturada. `--billed` también
hace una obtención real del mes actual y del anterior, y comprueba la respuesta como lo hace una importación; puede costar dinero en
los proveedores que hay detrás del plugin. [Importar los costos de uso de la nube](billing-import.md#a-plugin-billing_sources--plugin) tiene el
contrato del tipo.

`--kind mailer` no hace ninguna llamada del RPC salvo que `--mail-to` nombre un destinatario. Con él, el comando envía un mensaje real allí,
desde `--mail-from` (de forma predeterminada, la misma dirección), y comprueba que el plugin rechaza una solicitud sin destinatario.
[Enviar correo electrónico](email.md#a-plugin-mail--plugin) tiene el contrato del tipo.

`--kind translator` envía unas pocas frases fijas en inglés, traduciéndolas al japonés (`--translate-from` y `--translate-to`
eligen otro par para un plugin que solo tiene algunos). Un servicio detrás del plugin que cobre por carácter también cobra estas,
unos 200 caracteres. Comprueba la forma de las respuestas y que el plugin rechaza el idioma `qaa`, un código que ISO 639
reserva para uso privado. No puede juzgar las palabras, así que lee tú mismo una traducción del plugin.
[Traducir publicaciones](#translating-posts) tiene el contrato del tipo.

`--kind search` busca, y escribe y elimina solo documentos propios: unos pocos con ID que ninguna publicación tiene, de un
autor que no es ninguna cuenta, con una palabra hecha para la ejecución, que elimina al final. Comprueba que una consulta sin
palabras no encuentra nada, que una consulta en el lenguaje de consulta de un motor o en japonés no falla, que el plugin rechaza un
cursor que no emitió, y que los documentos se encuentran de más reciente a más antiguo, página a página, se reemplazan y se eliminan por publicación y por
autor. `--clear-index` ejecuta además los casos que eliminan todos los documentos de una visibilidad y todo el índice: úsalo solo
contra un índice de pruebas. [Buscar publicaciones](#searching-posts) tiene el contrato del tipo.

`--kind media_image`, `--kind media_video` y `--kind media_audio` envían solo pequeños archivos de prueba que ellos mismos crean:
una foto, un emoji y un GIF animado, un MP4 con video y pista de sonido, o un MP3. Cada uno oculta una ubicación o un
título en sus metadatos, y un caso falla cuando eso vuelve en una salida. Comprueban cada salida como lo hace el servidor.
[Convertir archivos multimedia](#converting-media) tiene el contrato de los tipos.

`--kind key_manager` ejecuta las comprobaciones que el servidor ejecuta en cada inicio, y comprueba que el plugin rechaza un valor modificado y una
KEK que no tiene. Envía solo claves que se inventa y descarta, así que es seguro contra un almacén de claves real, que puede facturar
las llamadas. Comprueba que las respuestas tienen la forma correcta, no cómo guarda el plugin su KEK.
[Gestionar la clave](key-management.md#a-plugin) tiene el contrato del tipo.

`probe` es el tipo más pequeño, y el servidor no le delega nada. Sírvelo primero, para comprobar el transporte, el
protocolo y la autenticación antes de escribir el resto del plugin.

<!-- source: translating-posts c9d31b2b9157 -->
## Traducir publicaciones

Un plugin del tipo `translator` permite que el servidor traduzca una publicación cuando un lector lo pide, a través de un servicio que elijas
distinto de los [proveedores integrados](translation.md) (LibreTranslate, DeepL, Google Cloud Translation y Amazon
Translate). Sin un proveedor en el servidor, la única traducción es la integrada en el navegador del lector, que existe
solo en Chrome, Edge y Opera de escritorio: los lectores en un dispositivo móvil, en Firefox o en Safari no tienen botón de traducir hasta que
actives uno.

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. Define `plugin_translator_address` y `plugin_translator_name`, y reinicia. El nombre es obligatorio, ya que los lectores confían en una
   traducción según de dónde viene. La dirección nunca sale de la configuración.
2. [Actualiza tu política de privacidad](privacy-policy-template.md#the-services-you-choose) y nombra el servicio que hay detrás del plugin
   como servicio de traducción: lo que el plugin transmite es lo que recibe.
3. Actívalo con la [configuración de la instancia](instance-settings.md) `translation_providers = plugin`. Un cambio surte efecto de inmediato.

Tener una dirección no activa nada: hasta que la configuración nombra `plugin`, el servidor no envía nada y los lectores no ven ningún botón.

El plugin es un proveedor como cualquier otro. [`translation_provider_visibilities`](instance-settings.md) decide lo que puede
recibir, los límites mensuales cuentan lo que se le envía, y una traducción que hizo permanece en la caché del servidor como cualquier otra.
Con `translation_reports_to_server` activado, también recibe lo que los moderadores piden traducir en una denuncia, con las mismas visibilidades.
Puede ir antes o después de otro proveedor en el orden de `translation_providers`.

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### Lo que recibe el plugin

El plugin recibe el texto de una publicación cuando un lector pide una traducción: su cuerpo, advertencia de contenido, opciones de encuesta y
descripciones de imágenes, además del idioma del texto y el idioma al que traducir. No recibe la dirección de la publicación,
su autor, el lector, las menciones, las etiquetas, los enlaces, los emoji personalizados ni el código: los sustituyen marcadores numerados.

De forma predeterminada son las publicaciones `public`, `local` y `unlisted`. **Las publicaciones `private` y `direct` llegan al plugin solo cuando
lo permites**, con `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`. Entonces el plugin, y cualquier
servicio que haya detrás, lee texto que el autor destinó a pocas personas, así que permítelo solo si controlas adónde va el texto,
y dilo en tu política de privacidad. El servidor no puede retirar el texto que ha enviado.

El servidor rechaza `http://` para este tipo, también en el bucle local, porque el texto es el texto de publicaciones. Usa un socket Unix o
`https://`.

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### Un LLM detrás de un plugin

El servidor no tiene proveedor de LLM y no ofrece ninguno, a propósito. Cualquiera puede escribir una publicación, así que el texto que se traduce es una forma de
dar instrucciones a un LLM (una inyección de prompt), y los lectores toman una traducción por lo que dice la publicación. La sanitización protege
el HTML de una página: no puede impedir que una traducción diga algo que la publicación no dice.

El servidor no puede ver lo que llama un plugin. **Si tu plugin usa un LLM, este riesgo y sus consecuencias son de tu
exclusiva responsabilidad.**

<!-- source: the-contract ef518f876dd2 -->
### El contrato

El RPC es `Translate` de `my.plugin.translator.v1.TranslatorService`:

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` es una etiqueta BCP 47, o `und` cuando el servidor no conoce el idioma. El plugin entonces lo detecta y lo informa
  en `detectedLanguage`, o responde `OUTCOME_UNSUPPORTED_LANGUAGE`. `detectedLanguage` puede faltar cuando el plugin no tiene
  nada que añadir, y el servidor ignora un valor que no sea una etiqueta de idioma.
- `translations` tiene una entrada por fragmento, en el orden de los fragmentos, ninguna vacía. Un fragmento puede ser solo
  marcadores, y su traducción son entonces esos marcadores.
- `⟦1⟧` representa un elemento o abre un par que cierra `⟦/1⟧`. Cada marcador de un fragmento vuelve exactamente una vez en su
  traducción, sin cambios. Los elementos pueden moverse como exija el orden de las palabras, pero un par se abre antes de cerrarse, y los pares se anidan.
- Una traducción es texto plano. El servidor escapa cualquier marcado que contenga y sanitiza el resultado antes de guardarlo o mostrarlo.

| `outcome` | Significado | Qué hace el servidor |
| --- | --- | --- |
| `OUTCOME_OK` | Las traducciones están en la respuesta | Las comprueba, las guarda y responde al lector |
| `OUTCOME_TEMPORARY` | El servicio falló o no respondió | Consulta al siguiente proveedor |
| `OUTCOME_PROVIDER_LIMIT` | El servicio informa de su propio límite de frecuencia o cuota | Consulta al siguiente proveedor |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | El servicio no tiene el idioma de origen o de destino, o no puede detectarlo | Consulta al siguiente proveedor y se lo dice al lector si ninguno lo tiene |

Una respuesta que incumple estas reglas cuenta como `OUTCOME_TEMPORARY` y no se guarda nada de ella, igual que un `outcome` ausente o desconocido
y una llamada fallida. Cuando ningún proveedor responde, el lector ve que la traducción falló y la publicación se queda como está, salvo que
`translation_browser` esté activado y el navegador del lector pueda traducir la publicación: el cliente web la traduce entonces allí.
Los caracteres de cada llamada cuentan para los límites mensuales, también los de una llamada que falla, ya que el servicio puede haberlos recibido.

El servidor deja de esperar tras `plugin_translator_timeout`, y tras 20 segundos diga lo que diga esa configuración. Todos los
proveedores de una solicitud tienen 45 segundos en total. No reintenta una
llamada fallida, y los mismos fragmentos pueden volver para otro lector.

<!-- source: searching-posts bd9dfba5a3e7 -->
## Buscar publicaciones

Un plugin del tipo `search` mantiene el índice de la [búsqueda de texto completo de publicaciones](search.md) en un motor que el servidor
no soporta de forma integrada, como Groonga o PGroonga, un índice creado con Sudachi, Typesense, Manticore o un servicio
de búsqueda alojado. Para el servidor es un motor más: las mismas publicaciones entran en el índice, los mismos trabajos lo mantienen al día
y el servidor sigue decidiendo qué puede ver cada persona.

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. Ejecuta el plugin y compruébalo con [`my plugin test --kind search`](#test-a-plugin).
2. Define `search = plugin` y `plugin_search_address`, y sigue [Activar la búsqueda](search.md#turning-it-on), como con cualquier
   motor: el índice se llena a partir de entonces, y `my search rebuild` añade las publicaciones existentes.
3. Si el plugin envía las publicaciones a un servicio de otra empresa, nombra ese servicio en tus
   [documentos de privacidad](search.md#your-privacy-documents).

El servidor no abre ninguna conexión al iniciarse, así que un plugin caído no detiene nada salvo la búsqueda. Para pasar al
plugin o dejarlo, sigue [Desactivar o cambiar el motor](search.md#switching-off-or-changing-the-engine).

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### Lo que recibe el plugin

- **Las publicaciones del índice.** Por cada publicación que contiene el índice, el plugin recibe su texto (el cuerpo, la advertencia
  de contenido y las descripciones de imágenes, como texto plano), su ID, el ID de cuenta de su autor, su visibilidad, su idioma
  y cuándo se publicó. Son solo publicaciones `public` y `local`, como dice [`search_visibilities`](instance-settings.md),
  de autores que permiten la búsqueda. Las publicaciones `local` están pensadas solo para las cuentas de este servidor: el plugin, y cualquier
  servicio que haya detrás, también las lee. Las publicaciones `unlisted`, `private` y `direct` nunca le llegan.
- **Las consultas.** Cuando una persona que ha iniciado sesión busca publicaciones, el plugin recibe lo que escribió, con el cursor de la
  página y el número de resultados. No recibe quién buscó.
- **Las eliminaciones:** de una publicación, de todas las publicaciones de una cuenta, de todas las publicaciones de una visibilidad y de todo el índice.

El plugin devuelve solo IDs de publicaciones. El servidor lee de nuevo cada publicación de su base de datos y muestra solo lo que la persona puede
ver ahora, así que un plugin no puede mostrar una publicación a quien no pueda verla, y un índice que va retrasado solo encuentra menos. El
servidor tampoco muestra nunca a nadie los errores de un plugin.

El servidor rechaza `http://` para este tipo, también en el bucle local, porque transporta el texto de las publicaciones y lo que la gente busca.
Usa un socket Unix o `https://`.

<!-- source: the-contract-1 d500365750c4 -->
### El contrato

El servicio es `my.plugin.search.v1.SearchService`, con un RPC por operación:

| RPC | Qué hace |
| --- | --- |
| `Replace` | Añade el documento de una publicación, o reemplaza el documento con el mismo `statusId` |
| `Remove` | Elimina el documento de una publicación |
| `RemoveByAuthor` | Elimina todos los documentos de una cuenta |
| `RemoveByVisibility` | Elimina todos los documentos de una visibilidad, `VISIBILITY_PUBLIC` o `VISIBILITY_LOCAL` |
| `Clear` | Elimina todos los documentos; el índice vuelve a admitir documentos después |
| `Search` | Devuelve una página de los IDs de los documentos que encuentra una consulta, de más reciente a más antiguo |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **Una consulta es texto plano.** El plugin la divide en palabras y encuentra los documentos con todas las palabras. Nunca lee el
  texto como el lenguaje de consulta de su motor: un operador, un comodín, unas comillas o un nombre de campo en ella son solo texto. Una consulta
  sin palabras no encuentra nada. Cómo encuentra el plugin palabras en japonés y otros idiomas que se escriben sin espacios es decisión suya,
  y es la razón de tener uno.
- **Una página va de más reciente a más antigua.** Los `statusIds` van en orden descendente, cada uno como máximo una vez, `limit` de ellos como máximo, y cada uno es un
  ID de publicación tal como lo envió el servidor (36 caracteres en minúsculas). Los IDs son UUIDv7, así que su orden como cadenas es el orden
  del tiempo.
- **El cursor es del plugin.** `nextCursor` está vacío en la última página y, si no, son como máximo 256 bytes de ASCII
  imprimible sin espacios, como el último ID de la página. El servidor lo devuelve sin cambios para la página siguiente, que continúa
  después de ese ID aunque hayan entrado y salido documentos. Una página que no es la última puede contener menos de `limit` IDs. Un cursor
  que el plugin no emitió recibe `OUTCOME_INVALID_CURSOR`.
- **Una escritura se ve en la siguiente búsqueda**, y cada llamada puede llegar dos veces: eliminar lo que no está tiene éxito. Un motor que
  muestra las escrituras solo tras un refresco hace que el plugin lo espere.

| `outcome` | Significado | Qué hace el servidor |
| --- | --- | --- |
| `OUTCOME_OK` | La llamada hizo lo que pide | Aplica la política de visibilidad a los IDs de una página |
| `OUTCOME_FAILED` | El motor falló o no respondió | Reintenta una escritura con su trabajo; hace fallar una búsqueda, que la persona puede intentar de nuevo |
| `OUTCOME_INVALID_CURSOR` | Solo `Search`: el cursor no es uno que emitió el plugin | Avisa al cliente de que su cursor es incorrecto |

Un `outcome` ausente o desconocido, una llamada fallida y una página que incumple estas reglas cuentan como `OUTCOME_FAILED`, y no se muestra nada
de tal página. Una llamada dura como máximo `plugin_search_timeout` (10 segundos de forma predeterminada), y 10 segundos diga lo que diga ese
configuración.

<!-- source: converting-media 36df3dbdb21b -->
## Convertir archivos multimedia

Un plugin de multimedia hace la parte pesada del procesamiento de las subidas, de modo que pueda ejecutarse en otra máquina. Hay tres tipos,
y eliges cada uno por separado:

- `media_image` recodifica las imágenes en sus variantes.
- `media_video` reempaqueta videos: archivos con una pista de video, con o sin sonido.
- `media_audio` reempaqueta archivos solo de audio.

El servidor sigue haciendo el resto por sí mismo. Detecta el formato de cada subida, distingue los videos de los archivos de audio, comprueba cada
subida frente a los límites cuando llega y hace las vistas previas de los archivos multimedia remotos. Un tipo cuya configuración sigue en `native` se
procesa en el servidor.

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

Define `media_<kind> = plugin` y `plugin_media_<kind>_address` para cada tipo que entregues, y reinicia. Un plugin puede
encargarse de más de un tipo: dale la misma dirección para cada uno. Cada tipo ejecuta como máximo dos llamadas a la vez, y cada llamada dura como máximo
`plugin_media_<kind>_timeout` (4 minutos de forma predeterminada), de modo que un video lento no retiene las imágenes.

- **Quitar los metadatos es tarea del plugin.** Las subidas llegan al plugin tal como se enviaron, con sus datos EXIF y
  etiquetas, que pueden contener dónde se tomó una foto. El servidor comprueba el formato y el tamaño de cada salida, pero no puede
  saber que no queda ningún rastro de metadatos. `my plugin test --kind media_image` comprueba que la ubicación de sus archivos de prueba
  no vuelve; comprueba el resto en el plugin.
- **El TCP sin cifrar envía las subidas sin cifrar.** Los tipos de multimedia aceptan `http://`, con una advertencia al iniciarse. Las subidas, con sus
  metadatos, cruzan entonces la red en claro, y nada autentica a ninguno de los dos extremos: cualquiera en esa red puede leerlas
  o responder en nombre del plugin. Usa un socket Unix o `https://` salvo que la red sea solo tuya.
- **El servidor no confía en la salida.** Detecta el formato de cada salida por su contenido, nunca por lo que declare el
  plugin, y la sirve con el tipo de contenido y la extensión de archivo de ese formato. Decodifica cada salida con
  los mismos límites que una subida y comprueba que cabe en su tamaño. Una salida mayor de lo declarado o de esos
  límites, de un formato que el plugin no declaró, o de un formato que la variante no admite, hace fallar la subida como un archivo
  que el servidor no puede procesar.
- **Un fallo es un retraso.** Cuando el plugin está caído o falla, el trabajo se reintenta más tarde, como con el procesador integrado. Una
  acción que necesita de inmediato la imagen de un emoji personalizado, como copiar un emoji remoto o aceptar una solicitud de uno, falla
  mientras falla el plugin.

<!-- source: the-contract-2 812c987c276c -->
### El contrato

El servicio es `my.plugin.media.v1.MediaService`, con los RPC `Process` para una imagen y `Remux` para un archivo de video o audio.
Ambos transmiten en ambos sentidos, así que el plugin necesita una biblioteca de Connect o gRPC y HTTP/2: `proto` o `grpc`, sobre TLS con ALPN
`h2`, o sobre HTTP/2 sin cifrar (h2c) en un socket Unix o TCP sin cifrar.

Una llamada es semidúplex:

1. El servidor envía un `head`: los límites y, para una imagen, su propósito, los recuadros de sus tres tamaños y su punto
   focal. El archivo sigue en mensajes `chunk` de 1 MiB como máximo. Entonces el servidor cierra su lado.
2. El plugin responde con un `head` cuyo `outcome` es el resultado. Con `OUTCOME_OK`, el head de `Process` contiene además las
   dimensiones de la imagen tras su orientación EXIF y el blurhash de la salida fija pequeña.
3. Por cada salida, el plugin envía un `part`, que nombra la salida y declara su formato, dimensiones y longitud,
   seguido de mensajes `chunk` de 1 MiB como máximo.

`Process` devuelve las salidas fijas pequeña, mediana y grande, y las tres animadas de una imagen de más de un
fotograma. Las salidas pequeña y mediana son WebP; la salida fija grande es un JPEG o un PNG (siempre un PNG para un emoji personalizado),
y la animada grande, un GIF. `Remux` devuelve el archivo en su propio contenedor sin metadatos, y la imagen de portada que
incrusta, si la hay. El servidor lee como máximo el doble del límite de imagen de `Process`, y de `Remux` el límite de video para el
archivo y el límite de imagen para su portada.

| `outcome` | Significado | Qué hace el servidor |
| --- | --- | --- |
| `OUTCOME_OK` | Siguen las salidas | Las comprueba y las guarda |
| `OUTCOME_TEMPORARY` | El plugin falló | Reintenta el trabajo más tarde |
| `OUTCOME_UNSUPPORTED_FORMAT`, `OUTCOME_UNSUPPORTED_CODEC`, `OUTCOME_TOO_LARGE`, `OUTCOME_DIMENSIONS`, `OUTCOME_CORRUPT`, `OUTCOME_TOO_LONG` | El archivo no es aceptable | Marca la subida como fallida |

Un `outcome` ausente o desconocido y una llamada fallida cuentan como `OUTCOME_TEMPORARY`.

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## Un plugin sin biblioteca

Con `json`, un plugin necesita solo un servidor HTTP y una biblioteca JSON. Un RPC es un `POST` a
`/<package>.<Service>/<Method>` con `Content-Type: application/json`, y el plugin responde `200` con un objeto JSON:

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

El JSON sigue el mapeo estándar de proto3:

- Un `int64` es una cadena decimal, los `bytes` son base64 y un enum es su nombre.
- Un campo con su valor cero puede faltar, en ambos sentidos. Un plugin debe leer un campo ausente como cero.
- El servidor ignora los campos que no conoce.
- Un resultado es el `outcome` de la respuesta. Un `outcome` ausente o desconocido es un fallo, nunca un éxito.
- Un plugin que falla responde con un error HTTP, o con `{"code":"unavailable"}` de Connect con el estado correspondiente. El trabajo se reintenta.

`Connect-Timeout-Ms` y `Connect-Protocol-Version` pueden ignorarse.
