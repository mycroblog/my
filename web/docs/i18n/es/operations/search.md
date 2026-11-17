<!-- source: full-text-search-of-posts fc16f55b56ea -->
# Búsqueda de texto completo de publicaciones

Qué contiene la búsqueda de texto completo de publicaciones, cómo activarla y cambiarla, cómo reconstruir su índice y qué exige a tus registros y copias de seguridad.

<!-- source: choosing-an-engine 9cfd51361e0a -->
## Elegir un motor

La búsqueda de texto completo de publicaciones necesita un motor de búsqueda que ejecutes o alquiles. [`search`](../reference/configuration.md#search) lo elige:

- `none`, el valor predeterminado: sin búsqueda de texto completo. El servidor no se conecta a nada, no encola ningún trabajo de búsqueda y el cliente no ofrece ninguna forma de buscar publicaciones. En un servidor así, cada comando `my search` lo indica y no hace nada.
- `elasticsearch`, `opensearch` o `meilisearch`: ese motor, con la configuración de su sección en [Motores de búsqueda](search-engines.md).
- `plugin`: un programa tuyo que mantiene el índice en cualquier otro motor, como Groonga o un índice creado con Sudachi, en `plugin_search_address` ([Buscar publicaciones con un plugin](plugins.md#searching-posts)).

La búsqueda de cuentas y de etiquetas funciona desde la base de datos y no depende de este ajuste.

El servidor es dueño únicamente de los campos que escribe, filtra, ordena y pagina. Cómo analiza el texto el motor, cómo ordena los resultados y qué tamaño tiene su índice lo ajustas tú, mediante un archivo de configuración del motor; [Motores de búsqueda](search-engines.md) describe qué encuentra cada motor en japonés con la configuración incorporada y cómo cambiarlo.

<!-- source: what-the-index-holds 5e0884482ea8 -->
## Qué contiene el índice

La base de datos sigue siendo la fuente de verdad, y el índice es una copia que puedes desechar y reconstruir. Un motor devuelve solo IDs de publicaciones: el servidor vuelve a leer cada publicación de la base de datos y aplica las reglas de visibilidad habituales, de modo que un índice atrasado puede dejar una publicación sin encontrar pero no puede mostrar una que no deba verse.

Una publicación está en el índice cuando se cumple todo esto:

- No está eliminada.
- Su visibilidad es `public` o `local`, y [`search_visibilities`](instance-settings.md) la incluye. Las publicaciones `unlisted`, `private` y `direct` no se indexan nunca, sea cual sea el ajuste.
- Su autor ha activado `indexable`, el ajuste descrito en [Motores de búsqueda y rastreadores de IA](crawlers.md).
- Su autor, y el dominio del autor, no tienen ninguna restricción que los oculte del descubrimiento.

El índice contiene el cuerpo, la advertencia de contenido y los textos alternativos de la versión actual de cada publicación, con su autor e idioma. No contiene encuestas, perfiles ni versiones anteriores.

`indexable_default` es `false` de forma predeterminada, así que un servidor que acaba de activar la búsqueda encuentra poco hasta que la gente elige `indexable` por sí misma.

<!-- source: turning-it-on 3d88170680a2 -->
## Activarla

1. Ejecuta el motor y define `search` y la configuración de su backend.
2. Si ejecutas los workers por separado, decide [su disposición](#keeping-search-from-delaying-other-jobs).
3. Configura [tus registros](#the-query-appears-in-urls).
4. Si el motor lo ejecuta otra persona, añádelo a tus [documentos de privacidad](#your-privacy-documents).
5. Reinicia el servidor y, a partir de entonces, las publicaciones nuevas entran en el índice.
6. Ejecuta `my search rebuild` para añadir las publicaciones existentes.

La búsqueda funciona durante la reconstrucción, y sus resultados son incompletos hasta que termina.

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## Reconstruir el índice

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

El comando termina en cuanto la reconstrucción está en marcha, y el worker hace el resto como trabajos `search.rebuild` ([referencia de trabajos](../reference/jobs.md#full-text-search)), así que mantén un worker en ejecución. Cada trabajo cubre 100 publicaciones.

- **Velocidad.** El valor predeterminado es de 20 publicaciones por segundo, con un máximo de 1000, y cuenta todas las publicaciones `public` y `local` examinadas, incluidas las que no cumplen los requisitos. Con el valor predeterminado, un millón de publicaciones tardan unas 14 horas. Deja margen para otros trabajos y para la base de datos, así que súbela solo si tu servidor tiene recursos de sobra.
- **Cuál ejecutar.** El modo predeterminado elimina todo lo que no debería estar en el índice: documentos que quedaron por una eliminación que nunca llegó, o por restaurar la base de datos desde una copia de seguridad. La búsqueda encuentra menos hasta que termina. Usa `--keep` después de [añadir una visibilidad](#changing-search_visibilities), cuando no hay nada que eliminar y la búsqueda debe seguir completa. `--keep` nunca elimina el documento de una publicación que ya no está en la base de datos.
- **¿Se interrumpió?** Ejecuta el mismo comando de nuevo. Continúa desde la última publicación procesada y no vacía el índice otra vez. Añade `--rate` para cambiar la velocidad. Una reconstrucción también continúa sola cuando se reinicia un worker.
- **El otro modo.** Mientras una reconstrucción está sin terminar, el comando en el otro modo se rechaza, porque no haría lo que dice. Para empezar de nuevo en cualquiera de los dos modos, ejecuta `my search clear` y después el comando que quieras.
- **Ejecutarlo dos veces** da el mismo índice. Las publicaciones creadas, editadas o eliminadas durante una reconstrucción terminan correctas, porque sus propios trabajos siguen ejecutándose y cada trabajo lee la publicación tal como está en ese momento.

`my search status` muestra el backend, `search_visibilities` y, de la última reconstrucción, su fase (`clearing`, `filling` o `finished`), las publicaciones procesadas y fallidas, la velocidad y el último error.

Cuando algo va mal:

- **El motor está caído.** Si fallan tres publicaciones seguidas, el trabajo se detiene donde está, registra el error y reintenta. Una reconstrucción que agotó los reintentos deja un trabajo fallido (`my job list`), y `updated` en el estado deja de avanzar. Cuando el motor vuelva, ejecuta el mismo comando para continuar.
- **El motor rechaza una publicación.** La publicación se registra como fallida y la reconstrucción sigue. El estado lista hasta 20 de ellas. Corrige la causa y ejecuta `my search rebuild --keep` para intentar de nuevo todas las publicaciones.

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## Cambiar `search_visibilities`

Dejar fuera una visibilidad elimina sus publicaciones del índice por sí solo. Añadir una encuentra solo las publicaciones nuevas, hasta que ejecutes `my search rebuild --keep`.

**Avisa a tus usuarios antes de añadir `local`.** `indexable` pasa entonces a cubrir las publicaciones `local` de todos los que ya lo activaron, y la reconstrucción añade sus publicaciones `local` anteriores, sin volver a preguntarles. La configuración del perfil dice qué cubre `indexable` en tu servidor, pero quienes lo eligieron hace tiempo no lo verán a menos que se lo digas.

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## Desactivar o cambiar el motor

Cuando el servidor pasa a `search = none` o a otro motor, pierde la conexión con el índice antiguo. Ese índice ya no se limpia, así que las publicaciones eliminadas después permanecen en él, texto incluido.

1. Detén el servidor y sus workers, para que nada escriba en el índice después del siguiente paso.
2. Ejecuta `my search clear`.
3. Cambia `search` y arranca el servidor.
4. Con un motor nuevo, ejecuta `my search rebuild`.

Si ya hiciste el cambio, o no puedes detener el servidor, elimina el índice antiguo en el propio motor. `my search clear` no puede alcanzarlo una vez cambiado el ajuste. Los trabajos encolados antes del cambio no hacen nada tras pasar a `none`.

**Volver desde `none`.** Ejecuta `my search rebuild` sin `--keep`: el servidor no guarda ningún registro de lo que se eliminó mientras no tenía motor, y solo vaciar el índice elimina esas publicaciones. Si una reconstrucción estaba sin terminar cuando lo dejaste, ejecuta antes `my search clear`, porque una reanudación no vacía el índice.

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## Evitar que la búsqueda retrase otros trabajos

De forma predeterminada un worker ejecuta un trabajo a la vez, así que un motor lento comparte el tiempo de ese worker con `federation.inbox`, aunque cada llamada al motor tenga un límite de 10 segundos. Un servidor con mucha actividad debería separarlos:

- Ejecuta un proceso worker con `worker_job_types = search.*` y los demás con `worker_job_types_except = search.*` ([Dividir los workers por tipo de trabajo](running.md#splitting-workers-by-job-type)).
- Con `queue = cloudtasks`, envía los tipos `search.*` a su propia cola con una [ruta de cola](running.md#routing-job-types-to-their-own-queues).

Un servidor pequeño puede dejarlos juntos, y los trabajos de búsqueda comparten entonces el tiempo con la federación.

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## La consulta aparece en las URL

`GET /api/v1/statuses/search` lleva en su URL lo que la gente busca, como `?q=`. La línea de registro `http request` del propio servidor contiene la ruta y la ruta de enrutamiento, no la cadena de consulta, y las trazas y los informes de error tampoco la incluyen. Sin embargo, lo que haya delante del servidor puede registrarla, y una búsqueda puede decir mucho de una persona. Revisa cada capa de tu configuración y mantén la consulta fuera de su registro o trata ese registro como datos personales, con el acceso y la retención correspondientes.

| Capa | ¿Registra la consulta? | Qué hacer |
| --- | --- | --- |
| Cloud Run | Sí. Sus registros de solicitudes se crean automáticamente, y `httpRequest.requestUrl` incluye la consulta. | Exclúyelos, como se indica más abajo. |
| Balanceador de carga de Google Cloud | Solo si activas el registro del servicio de backend. Está desactivado de forma predeterminada en los nuevos y usa el mismo campo. | Déjalo desactivado, o exclúyelos como se indica más abajo. |
| AWS Application Load Balancer | Solo si activas los registros de acceso, que están desactivados de forma predeterminada. La línea de solicitud contiene la URL tal como la envió el cliente. No tiene forma de omitir parte de una línea. | Déjalos desactivados, o dales una regla de ciclo de vida corta y acceso limitado. |
| CloudFront | Solo si activas el registro estándar, en el campo `cs-uri-query`. | Con el registro estándar v2, deja ese campo fuera al seleccionar los campos. |
| ECS | No tiene registro de solicitudes propio: sus registros contienen lo que escribe el servidor. | Nada. |
| Cloudflare | Solo si usas Logpush, en el campo `ClientRequestURI`. | Selecciona `ClientRequestPath` en su lugar. |
| Caddy | Solo con una directiva `log`, que `compose.caddy.yaml` no tiene. | Si añades una, elimina el parámetro, como se indica más abajo. |
| Otros proxies inversos | Normalmente sí: sus formatos predeterminados registran la línea de solicitud completa. | Registra la ruta sin la consulta para esta ruta, o desactiva su registro de acceso. |

En Google Cloud, esto mantiene ambos registros fuera del bucket `_Default` del proyecto. Añade la misma exclusión a cualquier sink propio:

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

En Caddy:

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## Copias de seguridad

El índice contiene el texto de las publicaciones, así que necesita el mismo cuidado que la base de datos: quién puede acceder a él, dónde se guarda y durante cuánto tiempo.

Una instantánea del motor es la parte que hay que vigilar. Una publicación eliminada del índice permanece en todas las instantáneas tomadas antes, igual que en las copias de seguridad de la base de datos, así que da a las instantáneas la misma retención que a tus [copias de seguridad de la base de datos](backup.md).

No necesitas instantáneas para recuperarte: `my search rebuild` vuelve a crear el índice desde la base de datos. Tras restaurar la base de datos desde una copia de seguridad, ejecútalo sin `--keep`, para que las publicaciones más recientes que la copia salgan del índice.

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## Tus documentos de privacidad

Si el motor se ejecuta en un servicio de otra empresa, esa empresa recibe el texto de las publicaciones cubiertas por `search_visibilities`, incluidas las publicaciones `local` si las incluyes. Nómbrala como destinataria en la [política de privacidad](privacy-policy-template.md) y en el [registro de actividades de tratamiento](processing-record-template.md). Un motor que ejecutas tú mismo no es un destinatario nuevo.

Indica también lo que decidiste sobre los [registros que contienen la consulta](#the-query-appears-in-urls), si los conservas.

<!-- source: closing-the-service 343ec69df255 -->
## Cerrar el servicio

El servidor no elimina datos por sí mismo cuando se cierra, así que descarta el índice y las instantáneas del motor junto con la base de datos y los archivos multimedia ([Cerrar el servicio](service-closure.md#7-discarding-the-db-blobs-and-backups)).
