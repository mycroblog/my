<!-- source: search-engines 6816d50f42dd -->
# Motores de búsqueda

Cómo ejecutar cada motor de búsqueda que puede usar la búsqueda de texto completo de publicaciones, qué tamaño necesita y hasta dónde encuentra el japonés y otros idiomas que se escriben sin espacios. Para un motor que no figura aquí, como Groonga o un índice creado con Sudachi, [un plugin de búsqueda](plugins.md#searching-posts) propio puede mantener el índice en su lugar. [La búsqueda de texto completo de publicaciones](search.md) explica qué contiene el índice, cómo activar la búsqueda, cómo reconstruir el índice y cómo cambiar de motor, sea cual sea el motor.

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` guarda el índice en un clúster de Elasticsearch de versión 8 o posterior, autogestionado o en Elastic Cloud Hosted. Se ha comprobado con Elasticsearch 9.5 y 8.19. El servidor es el dueño de los campos del índice, sobre los que escribe, filtra, ordena y pagina. Tú puedes ser el dueño de su configuración: el análisis, los shards y las réplicas. Sin configuración propia, la integrada no necesita ningún plugin y encuentra japonés como se [describe más abajo](#how-the-built-in-analysis-finds-japanese).

<!-- source: setting-it-up 3e02c35ff319 -->
### Puesta en marcha

1. Ejecuta Elasticsearch. Un solo nodo basta para la mayoría de los servidores. Para un inicio rápido en el mismo equipo:

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   Con la seguridad activada, que es lo predeterminado, Elasticsearch sirve HTTPS con un certificado de su propia autoridad. Copia esa autoridad fuera del contenedor para `search_elasticsearch_ca_file`:

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. Crea una clave de API que solo pueda gestionar los índices del servidor. Le bastan `manage`, `read` y `write` sobre el alias y los índices que hay detrás:

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   El valor `encoded` de la respuesta es la clave.

3. Define la configuración del servidor y reinícialo:

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. Ejecuta `my search rebuild` para añadir las publicaciones existentes, como en [activar la búsqueda](search.md#turning-it-on).

Los ajustes:

| Ajuste | Qué hace |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | La URL de la API REST. Usa `https:` siempre que las solicitudes crucen una red que compartan otras personas; `http:` se acepta para una red privada o un clúster con la seguridad desactivada. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | La clave de API, que es la mejor opción. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | Un usuario y una contraseña para la autenticación básica en lugar de una clave. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | El nombre del alias, `my-statuses` de forma predeterminada. Da a cada servidor que comparta un clúster su propio nombre y concede a su clave ese nombre seguido de `*`. |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | Un archivo JSON con tu propia configuración del índice, como en [ajustar el análisis](#tuning-the-analysis-of-elasticsearch). |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | La autoridad de certificación de un clúster HTTPS cuyo certificado el sistema no reconoce. |

Las credenciales se leen solo del entorno, como secretos, y nunca aparecen en los registros ni en los errores del servidor. Un error del clúster llega a los registros como la ruta de la solicitud, su estado y el tipo del error, como `index_not_found_exception`, nunca con el texto de una publicación ni de una consulta. Solo cuando el clúster se niega a crear el índice, el error cita su motivo, que no nombra más que la configuración y los campos.

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### El índice y sus actualizaciones

El servidor crea y mantiene el índice por sí mismo. No lo crees a mano ni dejes que nada más escriba en él.

- Al iniciarse, el servidor busca el alias. Si no hay ninguno, crea un índice con el nombre del alias, la versión de sus campos y un hash de su configuración, como `my-statuses-v1-1f0c2e9a7b3d4c5e`, y hace que el alias apunte a él. La versión y el hash se guardan en el `_meta` del índice.
- El servidor no puede iniciarse mientras el clúster esté inaccesible, rechace su clave o rechace la configuración. `my config check` comprueba la configuración y que el archivo de configuración sea un único objeto JSON, no lo que Elasticsearch opine de él, así que comprueba la conexión y la configuración iniciando el servidor.
- Un índice con el nombre del alias que el servidor no creó, como uno que Elasticsearch hizo por sí mismo, impide el inicio con un error que lo nombra. Elimina ese índice o elige otro `search_elasticsearch_index`.
- Cuando la configuración difiere de aquella con la que se creó el índice, porque editaste el archivo de configuración o una nueva versión de `my` cambió la integrada, el servidor crea un índice nuevo y vacío con la configuración nueva, hace que el alias apunte a él, elimina el anterior y registra una advertencia que pide `my search rebuild`. Ejecútalo. Hasta que termine la reconstrucción, la búsqueda solo encuentra las publicaciones escritas desde entonces. Cuando Elasticsearch rechaza la configuración nueva, el servidor no se inicia y el índice anterior se conserva.
- Cuando una nueva versión de `my` cambia los campos, el servidor actualiza el índice al iniciarse. Un cambio que solo añade campos se hace en el mismo lugar y la búsqueda sigue funcionando; las notas de la versión indican cuándo las publicaciones que ya están en el índice necesitan `my search rebuild --keep` para obtener los campos nuevos. Cualquier otro cambio de los campos requiere un índice nuevo y vacío y una reconstrucción, como arriba. Las notas de la versión indican cuándo una versión hace esto.
- Un índice creado por una versión más reciente de `my` impide que una más antigua se inicie. Para volver atrás tras un cambio así, elimina el índice del alias en Elasticsearch (búscalo con `GET _alias/my-statuses`), inicia la versión anterior, que crea el suyo, y ejecuta `my search rebuild`.
- `my search clear` hace que el alias apunte a un índice nuevo y vacío y elimina el anterior en un solo paso, lo que es más rápido que eliminar cada documento y devuelve el espacio en disco.

<!-- source: sizing 2b1d9b604953 -->
### Dimensionamiento

El índice guarda solo lo que necesita una búsqueda y ninguna copia del texto (`_source` está desactivado). Con la configuración integrada, las publicaciones cortas ocupan unos 80 a 150 bytes de disco cada una una vez que Elasticsearch ha fusionado sus segmentos, y las más largas con un vocabulario más amplio, más; por eso conviene prever de 150 a 300 bytes por publicación: unos cientos de megabytes para un millón de publicaciones. Comprueba el tamaño real con `GET _cat/indices/my-statuses*?v`.

La configuración integrada da al índice un shard primario, que aloja decenas de millones de publicaciones, y una réplica cuando el clúster tiene un segundo nodo (`auto_expand_replicas: 0-1`), de modo que un solo nodo se mantiene en verde. Define `number_of_shards` y `number_of_replicas` en un archivo de configuración para cambiarlos.

Un nodo con 1 GB de heap de Java (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) y unos 2 GB de memoria en total atiende un servidor pequeño o mediano. Dale a Elasticsearch la mitad de la memoria del equipo como heap y deja el resto para la caché de archivos, y dale más cuando las búsquedas se vuelvan lentas. Las réplicas necesitan más nodos y el mismo disco otra vez por cada réplica.

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### Cómo usa el análisis una consulta

Cada publicación se indexa en un único campo de texto, sea cual sea su idioma, y el mismo análisis se aplica a todas. El idioma de una publicación no elige otro analizador.

- El texto de una publicación se analiza con el analizador llamado `my_text`.
- Una consulta se divide en palabras en los espacios, incluido el espacio ideográfico. Cada palabra se analiza con `my_text_search` cuando la configuración lo define, y con `my_text` en caso contrario, y debe aparecer en la publicación como una frase: sus tokens en el mismo orden y uno junto a otro. Deben coincidir todas las palabras. Nada de una consulta se lee como el lenguaje de consulta de Elasticsearch.
- Los resultados se ordenan de más reciente a más antiguo, no por relevancia, así que las opciones de puntuación no influyen.

Por tanto, un analizador decide qué es una palabra. Un analizador que hace un token de cada carácter encuentra cualquier serie de caracteres. Uno que hace palabras de diccionario encuentra palabras completas y sus otras formas.

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### Cómo encuentra japonés el análisis integrado

El `my_text` integrado es el tokenizador estándar de Elasticsearch con los filtros `cjk_width` y `lowercase`, todos integrados. El tokenizador hace un token de cada kanji y cada hiragana, de cada serie de katakana y de cada palabra de los idiomas que usan espacios.

- Una palabra de kanji o hiragana se encuentra dondequiera que sus caracteres estén juntos, incluso dentro de una palabra más larga: `検索` y `本` encuentran `全文検索の本文です`, y `東京 天気` encuentra `東京都の天気は晴れ`. Los caracteres que solo aparecen separados no coinciden.
- Una serie de katakana se compara completa: `カタカナ` encuentra `カタカナとABC`, pero `カタ` no lo encuentra, y `ツリー` no encuentra `スカイツリー`.
- Las letras latinas de ancho completo y el katakana de ancho medio coinciden con sus formas habituales: `abc` encuentra `ＡＢＣ`, y `カタカナ` encuentra `ｶﾀｶﾅ`.
- El hiragana no encuentra el kanji de la misma lectura: `ぜんぶん` no encuentra `全文`. No se encuentran otras formas de una palabra: `走る` no encuentra `走った`.
- Una consulta de un solo carácter muy común, como `の`, encuentra muchísimas publicaciones.

El chino y el coreano se encuentran del mismo modo, por sus caracteres. Las palabras de los idiomas que se escriben con espacios se encuentran completas, sin lematización: `run` no encuentra `running`.

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### Ajustar el análisis de Elasticsearch

Pon la configuración del índice en un archivo JSON y nómbralo en `search_elasticsearch_index_settings_file`. El servidor usa todo el archivo como los `settings` de cada índice que crea, en lugar de los integrados, así que debe contener todo lo que el índice necesita:

- El analizador `my_text`, que usa el campo de texto. Sin él, Elasticsearch se niega a crear el índice y el servidor no se inicia.
- Opcionalmente, el analizador `my_text_search`, que entonces analiza las palabras de las consultas.
- Cualquier otro ajuste del índice, como `number_of_shards` y `number_of_replicas`. Lo que el archivo omite toma los valores predeterminados del clúster.
- Solo análisis que Elasticsearch tenga: un plugin que nombre el archivo debe estar instalado en todos los nodos.

El servidor lee el archivo al iniciarse. Editarlo da al siguiente inicio un índice nuevo y vacío, como en [el índice y sus actualizaciones](#the-index-and-its-upgrades), así que ejecuta `my search rebuild` después de cada cambio. Prueba antes un cambio en otro clúster o con otro `search_elasticsearch_index`.

Palabras de diccionario con el plugin `analysis-kuromoji`, que instalas en todos los nodos (`bin/elasticsearch-plugin install analysis-kuromoji`) o que Elastic Cloud Hosted ofrece como extensión:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      }
    }
  }
}
```

Cada serie de uno o dos caracteres, solo con el análisis integrado en Elasticsearch:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "max_ngram_diff": 1,
  "analysis": {
    "tokenizer": {
      "my_grams": { "type": "ngram", "min_gram": 1, "max_gram": 2, "token_chars": ["letter", "digit"] }
    },
    "analyzer": {
      "my_text": { "type": "custom", "tokenizer": "my_grams", "filter": ["cjk_width", "lowercase"] }
    }
  }
}
```

Cómo se comparan, medido con 20 000 publicaciones, la mitad en inglés y la mitad en japonés, en Elasticsearch 9.5:

| | Integrado | Kuromoji | Uno y dos caracteres |
| --- | --- | --- | --- |
| Plugin | Ninguno | `analysis-kuromoji` | Ninguno |
| Tamaño del índice | 1,5 MB | 1,4 MB | 3,7 MB |
| Parte de una palabra en kanji: `京都` en `東京都` | Encontrado | No encontrado | Encontrado |
| Parte de una palabra en katakana: `ツリー` en `スカイツリー` | No encontrado | Encontrado | Encontrado |
| Parte de una palabra en katakana: `コンピュータ` en `コンピューター` | No encontrado | No encontrado | Encontrado |
| Otra forma de una palabra: `走る` encuentra `走った` | No encontrado | Encontrado | No encontrado |
| Parte de una palabra en inglés: `run` en `running` | No encontrado | No encontrado | Encontrado |

Kuromoji divide el japonés en las palabras de su diccionario y reduce cada una a su forma base, de modo que encuentra una palabra en cualquiera de sus formas, y una palabra que el diccionario conoce dentro de un compuesto. No encuentra una parte de una palabra que el diccionario mantiene entera, y cómo divide una palabra nueva o un nombre depende del diccionario.

Las series de uno y dos caracteres encuentran cualquier parte de cualquier palabra, en todos los idiomas, incluidas partes que no son palabras: `run` también encuentra `brunch`. El índice es unas dos veces y media más grande y una búsqueda lee más de él.

Las búsquedas tardaron unos pocos milisegundos con cada uno de ellos en este índice pequeño, y las diferencias crecen con el tamaño del índice.

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` guarda el índice en un clúster de OpenSearch que tú ejecutas, o en un dominio de Amazon OpenSearch Service. El servidor habla con la API REST del clúster. Es el dueño de los campos del índice y de las consultas que envía, y tú puedes ser el dueño del resto: los analizadores, los shards, las réplicas y cualquier otro ajuste del índice. Sin configuración propia, usa analizadores integrados en OpenSearch y no necesita ningún plugin.

<!-- source: setting-it-up-1 11097d66bccc -->
### Puesta en marcha

1. Ejecuta OpenSearch 2 o 3. Un nodo basta para empezar; el servidor solo llega a la dirección que le des.
2. Define [`search`](../reference/configuration.md#search) como `opensearch` y [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) como la URL de la API REST, por ejemplo `https://search.internal.example.org:9200`. Se conserva una ruta, para un clúster detrás de un proxy inverso.
3. Dale al servidor una forma de entrar:
   - Con el plugin de seguridad, un usuario con [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) y [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), enviados con autenticación básica HTTP. Usa `https` cuando el clúster no esté en el mismo equipo ni en una red privada de confianza. El usuario necesita crear, leer el mapeo de y eliminar los índices cuyos nombres empiezan por [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), gestionar ese alias y leer, escribir y eliminar sus documentos.
   - En Amazon OpenSearch Service con una política de acceso de IAM, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) definido como `es`, que firma cada solicitud con AWS Signature Version 4. Las credenciales provienen de la cadena predeterminada del SDK de AWS (el rol de ejecución, el entorno, los archivos compartidos) y la región, de [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) o de `AWS_REGION`. El rol necesita `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut` y `es:ESHttpDelete` sobre el dominio. Un dominio con control de acceso detallado y un usuario maestro interno toma en su lugar el usuario y la contraseña.
   - Amazon OpenSearch Serverless no es compatible: no tiene eliminación por consulta, que se necesita para quitar las publicaciones de un autor o de una visibilidad.
4. Opcionalmente, [ajusta el análisis](#tuning-the-analysis-of-opensearch) con un archivo de configuración.
5. Inicia el servidor. Crea el índice al iniciarse y se niega a iniciarse cuando no puede llegar al clúster o crear el índice.
6. Ejecuta `my search rebuild` para indexar las publicaciones que ya existen ([reconstruir el índice](search.md#rebuilding-the-index)).

La URL, el usuario, las credenciales de AWS y el contenido del archivo de configuración nunca aparecen en los registros ni en los errores del servidor. Un error del motor llega al registro solo como la operación, el estado HTTP y el tipo del error del motor, ya que el mensaje propio del motor puede citar una publicación o una consulta. La única excepción es una creación rechazada del índice, cuyo mensaje nombra el ajuste responsable.

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### El índice y sus actualizaciones

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), `my-statuses` de forma predeterminada, nombra un alias. Los documentos viven en un índice que el servidor crea y al que apunta el alias, con el nombre del alias, la versión del mapeo del servidor y un hash de la configuración del índice, como `my-statuses-v1-3f2a9c41b07e`. El mapeo registra las mismas dos en su `_meta` (`my_mapping_version` y `my_settings_hash`). Toda escritura pasa por el alias y falla mientras falta, de modo que OpenSearch nunca crea un índice propio con un mapeo supuesto. No crees tú un índice con ese nombre: el servidor se niega a iniciarse cuando el nombre es un índice en lugar de un alias, o cuando el alias apunta a un índice que no creó.

Al iniciarse, el servidor compara el índice que hay detrás del alias con su propia versión de mapeo y su configuración. Cuando alguno difiere (una versión cuyo mapeo cambió, lo que indican sus notas de la versión, o un archivo de configuración editado), crea un índice nuevo y mueve el alias a él. El índice nuevo empieza vacío, así que:

1. Ejecuta `my search rebuild`. Hasta que termine, la búsqueda funciona pero solo encuentra lo publicado o reconstruido desde el cambio.
2. Elimina el índice anterior (`DELETE /my-statuses-v1-…`) cuando haya terminado la reconstrucción. El servidor lo deja en su lugar y registra una advertencia que lo nombra.

Inicia todos los procesos del servidor con el mismo archivo de configuración: los procesos con archivos distintos mueven el alias de un lado a otro. Un servidor de una versión anterior que se inicia con un índice de un mapeo más reciente usa ese índice tal cual.

`my search clear`, y `my search rebuild` sin `--keep`, que empieza con él, crean un índice nuevo y vacío con el mismo mapeo y la misma configuración, mueven el alias a él y eliminan el anterior en una sola solicitud.

<!-- source: sizing-1 4ad4bbcd9a2b -->
### Dimensionamiento

- **Memoria**: el nodo necesita un heap de JVM de al menos 512 MB (`-Xms512m -Xmx512m`) y otro tanto para el sistema operativo; dale 1 GB para un servidor pequeño y más a medida que crezca el índice. Lo mejor es mantener el heap en la mitad de la memoria del nodo.
- **Disco**: el índice ocupa aproximadamente el tamaño del texto de las publicaciones indexadas con la configuración integrada, más para los idiomas que se escriben sin espacios, cuyo texto se indexa como caracteres sueltos y pares, y más aún con n-gramas. Mantén libre al menos el 15 % del disco: de forma predeterminada, OpenSearch deja de colocar shards en un nodo por encima del 85 % y bloquea las escrituras en sus índices por encima del 95 %.
- **Shards y réplicas**: sin un archivo de configuración, el índice tiene los valores predeterminados del clúster, un shard y una réplica salvo que los hayas cambiado. Un shard atiende millones de publicaciones. Una réplica deja un clúster de un solo nodo en amarillo; funciona, y `"index": {"number_of_replicas": 0}` en un archivo de configuración lo pone en verde. En un clúster de varios nodos, conserva una réplica. El número de réplicas también puede cambiarse en el índice en vivo (`PUT /my-statuses/_settings`), lo que no requiere reconstrucción; el servidor sigue usando el índice mientras el archivo de configuración no cambie.
- **Carga**: cada búsqueda es una consulta del índice, de más reciente a más antigua por ID de publicación. Las escrituras son una solicitud por publicación, y una reconstrucción no escribe más rápido que [su ritmo](search.md#rebuilding-the-index).

<!-- source: japanese-and-other-languages a48cff99706b -->
### Japonés y otros idiomas

Con la configuración integrada, cada publicación, sea cual sea su idioma, se analiza del mismo modo: el texto se divide en palabras en los espacios y la puntuación, se pasa a minúsculas, y las letras latinas de ancho completo y el katakana de ancho medio se convierten a sus formas habituales. El texto en chino, japonés y coreano, que no tiene espacios, se divide en caracteres, y cada par de caracteres vecinos se indexa además de cada carácter (el filtro integrado `cjk_bigram` de OpenSearch).

- Una consulta encuentra las publicaciones que contienen todas sus palabras. En japonés, una palabra de dos caracteres o más encuentra las publicaciones que contienen cada par de sus caracteres: `東京` encuentra `東京都の天気`, y `全文検索` encuentra `全文検索の本文です`. Un solo carácter, como `本`, encuentra todas las publicaciones con ese carácter.
- Los pares no saben dónde terminan las palabras, así que una consulta puede encontrar una publicación en la que sus caracteres solo coinciden por estar uno junto a otro: `京都` encuentra también `東京都`. Una consulta de katakana escrita en letras de ancho medio o de ancho completo encuentra ambas.
- Las lecturas no se comparan: `ぜんぶん` no encuentra `全文`, y las formas flexionadas no se comparan con sus formas de diccionario, ni en japonés ni en inglés (`running` no encuentra `run`).

Para encontrar japonés por palabras, usa un analizador morfológico como kuromoji en un [archivo de configuración](#tuning-the-analysis-of-opensearch).

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### Ajustar el análisis de OpenSearch

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) nombra un archivo JSON cuyo objeto pasa a ser los `settings` de cada índice que crea el servidor: `analysis`, `index.number_of_shards`, `index.number_of_replicas` y cualquier otro ajuste del índice. Contiene solo configuración; el servidor es el dueño del mapeo y rechaza un archivo con `mappings`.

El archivo debe definir dos analizadores, que el mapeo nombra:

| Analizador | Analiza |
| --- | --- |
| `my_text` | El texto de cada publicación: su cuerpo, su advertencia de contenido y los textos alternativos de sus archivos multimedia |
| `my_text_search` | El texto de cada consulta |

Pueden tener la misma definición. Cada consulta es una consulta `match` del campo `text` con `"operator": "and"`: el texto de la consulta pasa por `my_text_search`, y se encuentra una publicación cuando su texto contiene todos los términos que resultan, de más reciente a más antigua por ID de publicación, sin puntuación. Una consulta cuyo analizador no deja ningún término no encuentra nada. No se usan la coincidencia de frases, difusa ni de prefijos, así que un analizador decide por sí solo qué es una coincidencia. El idioma de una publicación se guarda pero no elige un analizador: toda publicación pasa por `my_text`.

Cuando el servidor se inicia con un archivo cuya configuración difiere de la del índice, aunque sea en una réplica, crea un índice nuevo y vacío, y el índice necesita una reconstrucción ([el índice y sus actualizaciones](#the-index-and-its-upgrades)). El formato y el orden de las claves no cuentan. Un archivo que OpenSearch rechaza, como uno sin los dos analizadores o con un plugin que el clúster no tiene, impide el inicio con el motivo de OpenSearch.

Kuromoji divide el japonés en palabras con un diccionario y compara las formas flexionadas con sus formas de diccionario. Amazon OpenSearch Service tiene el plugin en todos los dominios; un clúster que ejecutas tú necesita que lo instales (`opensearch-plugin install analysis-kuromoji`, por ejemplo en un Dockerfile `FROM opensearchproject/opensearch`). Un archivo con kuromoji:

```json
{
  "index": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      }
    }
  }
}
```

Una palabra que el diccionario divide de forma distinta a la consulta no se encuentra, así que kuromoji puede perder lo que encuentran los pares integrados, como una parte de un sustantivo compuesto o una palabra nueva. Un tokenizador de n-gramas, integrado en OpenSearch, encuentra cualquier serie de caracteres a costa de un índice más grande y de coincidencias dentro de palabras más largas; con `"min_gram": 2` y `"max_gram": 3` (y `"index": {"max_ngram_diff": 1}`), una consulta de un solo carácter no encuentra nada.

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` guarda el índice en un servidor [Meilisearch](https://www.meilisearch.com/docs), autoalojado o Meilisearch Cloud. Es un único proceso sin clúster que ejecutar, lo que conviene a un servidor pequeño, y la imagen oficial segmenta japonés, chino y coreano sin plugin. El servidor se ha comprobado con Meilisearch 1.56.

<!-- source: running-meilisearch 4f7d5e17c255 -->
### Ejecutar Meilisearch

Ejecuta la imagen oficial con una clave maestra de al menos 16 bytes, en una red a la que solo pueda llegar `my`, y conserva su directorio de datos en un volumen persistente:

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` se niega a iniciarse sin una clave maestra y desactiva la vista previa de búsqueda integrada. Meilisearch habla HTTP sin cifrar: ponlo detrás de TLS, o mantenlo en una red privada, cuando `my` lo alcance por cualquier enlace que no sea local o privado.

<!-- source: settings 74547d71e962 -->
### Ajustes

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

Define la clave a través del entorno (`MY_SEARCH_MEILISEARCH_API_KEY` o `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), nunca en el archivo de configuración. Puede ser la clave maestra, pero una clave que pueda hacer solo lo que `my` necesita es más segura. Crea una con la clave maestra:

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Usa el `key` de la respuesta como `search_meilisearch_api_key`. La clave se envía solo en la cabecera `Authorization`: los registros y los errores del servidor muestran el código de error de Meilisearch (como `invalid_api_key`) y nunca la clave, una publicación ni una consulta. Toda [la configuración](../reference/configuration.md#search_meilisearch_url) está en la referencia de configuración.

Varios servidores pueden compartir un mismo servidor Meilisearch con un `search_meilisearch_index` distinto cada uno y una clave limitada a su propio índice.

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### El índice y su configuración

El servidor crea el índice cuando no existe. La primera vez que usa el índice tras un inicio, compara la configuración que enviaría con la que aplicó la última vez, que registra como un hash en un documento del índice que ninguna búsqueda encuentra, y la envía solo cuando difiere: un reinicio sin cambios no envía nada. La configuración es:

- **Lo que define el servidor por sí mismo:** `searchableAttributes` (los campos de texto de las publicaciones), `displayedAttributes` (todos), `filterableAttributes` (el ID de la publicación, el autor y la visibilidad) y `sortableAttributes` (el ID de la publicación). La búsqueda depende de ellos, y se rechaza un archivo de configuración que defina alguno.
- **Lo que puedes ajustar:** todo lo demás, en el [archivo de configuración](#tuning-meilisearch). Sin uno, o para lo que el archivo omite, el servidor usa sus valores predeterminados: `rankingRules` de `sort`, `words`, `typo`, `proximity`, `attribute` y `exactness`; sin tolerancia a erratas; los idiomas de los campos de japonés, chino y coreano en `localizedAttributes`; y los valores predeterminados propios de Meilisearch para el resto (`stopWords`, `synonyms`, `dictionary`, `separatorTokens`, `nonSeparatorTokens`, `proximityPrecision`, `pagination`, `faceting`, `distinctAttribute`, `searchCutoffMs`, `prefixSearch`). Lo que cambies a mano en el motor permanece solo hasta que el servidor envíe de nuevo los suyos.

**Un cambio de la configuración no necesita `my search rebuild`.** Meilisearch reindexa por sí mismo los documentos que contiene, en segundo plano, y las publicaciones permanecen en el índice. Solo lleva tiempo, aproximadamente tanto como llevó indexarlas. El inicio del servidor no espera a que termine: el servidor envía la configuración y continúa. Mientras tanto, las búsquedas responden con la configuración anterior y las escrituras del índice esperan detrás de la reindexación, ya que Meilisearch ejecuta en orden las tareas de un índice; un trabajo cuya escritura no termina en sus 10 segundos se reintenta más tarde, de modo que las publicaciones escritas durante la reindexación llegan al índice después. `my search clear` espera a la configuración del índice nuevo y vacío, que no tardan nada.

Una reconstrucción solo es necesaria cuando el índice empieza vacío:

- Cambias `search_meilisearch_index`.
- Inicias un servidor Meilisearch nuevo y vacío, por ejemplo en lugar de actualizar el anterior.
- Las notas de la versión de `my` indican que escribe los documentos de otra manera.

Las escrituras son tareas que Meilisearch ejecuta en orden. El servidor espera a la tarea de cada escritura, y una tarea que falla o no termina en el tiempo del trabajo hace que el trabajo se reintente, de modo que un Meilisearch caído u ocupado retrasa el índice sin perder ningún cambio. Una tarea de configuración que falla, por ejemplo por un valor del archivo de configuración que Meilisearch rechaza, hace fallar las escrituras hasta que tiene éxito; `GET /tasks?types=settingsUpdate` en el motor muestra su error. `my search clear` elimina el índice y lo vuelve a crear con la configuración.

Para actualizar el propio Meilisearch, sigue su guía de actualización (un volcado o su actualización sin volcado), o inicia la versión nueva vacía y ejecuta `my search rebuild`.

<!-- source: what-a-search-sends 575b89133606 -->
### Qué envía una búsqueda

La consulta de una búsqueda es fija, diga lo que diga el archivo de configuración: `matchingStrategy: all`, de modo que una publicación debe tener todas las palabras; `sort: status_id:desc`, de más reciente a más antigua; un `filter` sobre el ID de la publicación para la página siguiente; un `limit` de la página más uno; `attributesToRetrieve` del ID de la publicación; y `showMatchesPosition`, que detecta una consulta que no tiene ninguna palabra. Las comillas dobles de una frase y el signo menos antes de una palabra, que Meilisearch leería como operadores, se descartan, de modo que una consulta siempre son palabras simples.

<!-- source: tuning-meilisearch df827448e06c -->
### Ajustar Meilisearch

Ajusta el motor con un archivo JSON de [configuración de índice de Meilisearch](https://www.meilisearch.com/docs/reference/api/settings), nombrado por `search_meilisearch_settings_file`. El servidor lo comprueba al iniciarse, cuando `search = meilisearch`: debe existir y ser un objeto JSON, no debe definir los cuatro valores de configuración que define el propio servidor, y sus `rankingRules`, cuando las tiene, deben empezar por `sort`, lo que mantiene los resultados de más reciente a más antiguo. Meilisearch comprueba los valores cuando el servidor los envía. El servidor nunca registra el contenido del archivo.

Un ejemplo que reduce los idiomas al japonés, trata `・` como separador, abarata el costo de indexar la proximidad y activa la tolerancia a erratas para las palabras largas:

```json
{
  "localizedAttributes": [
    {"attributePatterns": ["text_ja"], "locales": ["jpn"]}
  ],
  "separatorTokens": ["・"],
  "proximityPrecision": "byAttribute",
  "typoTolerance": {"enabled": true, "minWordSizeForTypos": {"oneTypo": 8, "twoTypos": 12}},
  "synonyms": {"ご飯": ["ごはん"], "ごはん": ["ご飯"]}
}
```

Lo que cuesta un cambio:

- Cada cambio del archivo, en el siguiente inicio, hace que Meilisearch reindexe todos los documentos, como se dijo antes: no se pierde ningún dato y nada necesita una reconstrucción, pero las escrituras esperan hasta que termina. Hazlo en un momento tranquilo y observa la tarea con `GET /tasks?types=settingsUpdate`.
- `localizedAttributes` y los tokens deciden cómo se divide el texto en palabras: una lista de idiomas más estrecha puede segmentar una publicación en otro idioma como si fuera el equivocado (véase más abajo), así que conserva los campos de japonés, chino y coreano salvo que el servidor no tenga ninguna de esas publicaciones.
- La tolerancia a erratas y los sinónimos hacen que una búsqueda encuentre más que las palabras exactas. Las reglas de ordenación posteriores a `sort` no cambian nada, ya que no hay dos publicaciones con el mismo ID; omitirlas solo ahorra a Meilisearch un poco de trabajo.

<!-- source: sizing-2 1d581fac5316 -->
### Dimensionamiento

Meilisearch guarda el índice en archivos mapeados en memoria en disco: funciona con menos memoria que el índice y se vuelve más rápido cuanto más del índice cabe en memoria.

- **Memoria.** 1 GB es un buen punto de partida para un servidor pequeño; dale más a medida que crezca el índice, idealmente tanto como el tamaño del índice en disco. La indexación usa de forma predeterminada hasta dos tercios de la memoria del equipo; en un equipo compartido con otros servicios, limítala con `MEILI_MAX_INDEXING_MEMORY` (como `512Mb`).
- **Disco.** El índice es varias veces más grande que el texto que contiene, porque guarda las listas de palabras de cada campo. Mídelo tras la primera reconstrucción con `GET /stats` (`databaseSize`) y mantén un espacio libre muy superior: la reindexación y las actualizaciones necesitan espacio para trabajar.
- **CPU.** Uno o dos núcleos bastan para un servidor pequeño. Una reconstrucción es la carga más pesada; ralentízala con `my search rebuild --rate`.

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### Japonés y otros idiomas

El servidor pone el texto de una publicación en el campo de su idioma: japonés (`ja`, incluidas etiquetas como `ja-JP`), chino (`zh`), coreano (`ko`), o un campo para todos los demás idiomas y para la ausencia de idioma. Le indica a Meilisearch el idioma de los tres primeros, y Meilisearch segmenta cada uno en palabras con un diccionario:

- **El japonés** lo segmenta Lindera con el diccionario UniDic, tenga el texto kana o solo kanji. `検索`, `全文検索` y `全文　検索` encuentran todos `全文検索の本文です`, y `東京` encuentra `東京都庁舎見学`.
- **El chino** lo segmenta jieba, y **el coreano**, Lindera con el diccionario ko-dic.
- **Los demás idiomas** se dividen en los espacios y la puntuación; Meilisearch detecta por sí mismo la escritura y el idioma del texto.

Una consulta se divide en palabras del mismo modo, y se encuentra una publicación cuando tiene todas las palabras de la consulta, la última también como el comienzo de una palabra más larga (`テキ` encuentra `テキスト`). Solo cuentan las 10 primeras palabras de una consulta.

Lo que la búsqueda no puede encontrar:

- **Una cadena que no es una palabra de la segmentación del diccionario.** La búsqueda encuentra palabras, no subcadenas: `京都` no encuentra `東京都`, y una parte que el diccionario mantiene dentro de una palabra más larga, como `庁舎` en `東京都庁舎見学`, tampoco se encuentra.
- **Lecturas.** El hiragana encuentra la misma palabra en katakana (`かたかな` encuentra `カタカナ`), pero una lectura no encuentra kanji (`ぜんぶん` no encuentra `全文`).
- **Grafías cercanas.** La tolerancia a erratas está desactivada, así que cada palabra debe ser exacta.
- **Publicaciones en japonés sin idioma.** Una publicación cuyo idioma falta o es erróneo se segmenta según lo que detecte Meilisearch, y el texto solo en kanji suele tomarse entonces por chino y dividirse de otra manera: una publicación así puede encontrarse con `京都` por `東京都`, o no con `東京`. Una publicación lleva el idioma que eligió su autor, o el que envió el servidor del autor.

El servidor pliega las formas de compatibilidad de Unicode en las publicaciones y las consultas antes de que lleguen a Meilisearch, de modo que el katakana de ancho medio (`ｶﾀｶﾅ`) y las letras y dígitos de ancho completo (`ＡＢＣ`) encuentran sus formas habituales y son encontrados por ellas. Los emoji también son palabras: `👍` encuentra las publicaciones que lo tienen.

La búsqueda no necesita ningún plugin, diccionario ni imagen propia para nada de esto: la imagen oficial tiene los diccionarios de japonés, chino y coreano.
