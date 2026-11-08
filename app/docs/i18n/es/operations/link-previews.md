<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# Vistas previas de enlaces (OGP / X Card / fediverse:creator)

Las vistas previas que otros servicios muestran de las páginas de este servidor, lo que el servidor devuelve para ellas y los pasos para comprobarlas a mano. Las tarjetas que este servidor muestra para los enlaces de sus propias publicaciones
están en [Tarjetas de enlaces en las publicaciones](#cards-of-links-in-posts) más abajo; cómo dejar de obtenerlas de un dominio se explica en la [configuración de la instancia](instance-settings.md#link-card-domains).

<!-- source: what-the-server-returns bb9326f9729a -->
## Qué devuelve el servidor

`my serve` devuelve el HTML de la página de inicio (`https://<host>/`), la página de información de la instancia (`https://<host>/about`),
las páginas de los documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`), la página sobre el rastreador de tarjetas de enlaces (`/about/crawler`), los perfiles
(`https://<host>/@alice`) y las publicaciones (`https://<host>/@alice/status/<post ID>`) incrustando metaetiquetas en la estructura base del
cliente web.
Una petición de HTML se trata siempre como sin sesión iniciada, de modo que una vista previa muestra solo lo que puede ver un visitante sin sesión.

| Página | Respuesta |
| --- | --- |
| La página de inicio (`/`) y la página de información de la instancia (`/about`) | 200. La estructura base con metaetiquetas que presentan la instancia (consulta «La página de inicio» más abajo) |
| La página sobre el rastreador de tarjetas de enlaces (`/about/crawler`) | 200. La estructura base con metaetiquetas que presentan la instancia, con el título «Rastreador de tarjetas de enlaces \| nombre de la instancia» |
| Las páginas de documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. La estructura base con metaetiquetas que presentan la instancia, con el título «nombre del documento \| nombre de la instancia». La página de un documento que no está publicado recibe la estructura base genérica |
| Una publicación `public`, el perfil de una cuenta local activa | 200. La estructura base con metaetiquetas |
| Una publicación `unlisted` | 200. La estructura base con metaetiquetas, más `<meta name="robots" content="noindex, noarchive">` y `X-Robots-Tag: noindex, noarchive` |
| Una publicación `local`, `private`, `direct`, eliminada o inexistente, y el perfil y las publicaciones de una cuenta suspendida, desactivada, eliminada o limitada | 404. La estructura base genérica y `X-Robots-Tag: noindex, noarchive`. El cuerpo y las cabeceras son iguales sea cual sea el motivo |
| `/@<old username>/status/<post ID>` | 301, a la URL con el nombre de usuario actual |

Cada página lleva además las declaraciones para los motores de búsqueda y los rastreadores de IA (metaetiquetas como `noindex` y `noai`, y
`X-Robots-Tag`, `Content-Usage` y `tdm-reservation`), según la configuración de la instancia y los de su propietario
([motores de búsqueda y rastreadores de IA](crawlers.md)). El `noindex` de la tabla anterior se añade siempre, sean cuales sean las configuraciones.
`noindex` no impide las vistas previas de enlaces.

Una petición con un `Accept` de ActivityPub no recibe la estructura base; la responde la parte de ActivityPub (un perfil responde con el
`id` del actor, y una publicación que un visitante sin sesión puede ver responde con un 302 a su `id`). Cada respuesta lleva `Vary: Accept`.

<!-- source: meta-tags 00a4a8115b9e -->
### Metaetiquetas

| Etiqueta | Contenido |
| --- | --- |
| `og:title`, `<title>` | «Nombre visible (@usuario@dominio de la cuenta)». El nombre de usuario si el nombre visible está vacío |
| `og:type` | `profile` para un perfil, `article` para una publicación |
| `og:url`, `<link rel="canonical">` | La URL canónica (la URL con el nombre de usuario actual) |
| `og:description` | El texto sin formato del cuerpo de la publicación (la biografía en un perfil) sin etiquetas, recortado a 200 grafemas. En una publicación con advertencia de contenido, solo el texto de la advertencia de contenido |
| `og:image`, `og:image:alt` / `width` / `height` | La imagen elegida en el orden indicado más abajo |
| `og:site_name` | El nombre de la instancia (el texto `name`, igual en todos los idiomas; `host` si no hay; [nombre y dominio](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | El idioma de la publicación. Cuando no está definido (`und`), el idioma de publicación por defecto del autor y, si no lo hay, el `default_locale` de la instancia. No cambia con `Accept-Language` |
| `article:published_time`, `article:author` | Solo publicaciones. La hora de la publicación (UTC) y la URL del perfil del autor |
| `profile:username` | Solo perfiles. `username@account domain` |
| `twitter:card` | `summary_large_image` cuando la imagen es más ancha que aproximadamente 2:1 (ancho / alto >= 1.7) y de al menos 300x157; en caso contrario, `summary` |
| `fediverse:creator` | `@username@account domain`. Mastodon lo muestra como «autor» |
| `<link rel="alternate" type="application/activity+json">` | El `id` de AP (el actor o el objeto de la publicación) |

Las metaetiquetas de la página de inicio presentan la propia instancia.

- `og:type` es `website`, `og:title`, `<title>` y `og:site_name` son el nombre de la instancia, y `og:url` y
  `<link rel="canonical">` son `https://<host>/`.
- `og:description` es el texto de la instancia `short_description` recortado a 200 grafemas. Se omite cuando no hay.
- El idioma (`og:locale`, `<html lang dir>`) es el `default_locale` de la instancia. El nombre y la descripción son el mismo valor
  en todos los idiomas.
- `og:image` es el número 3 de la lista siguiente (la imagen de vista previa de la instancia), o el número 4 o 5 si no hay.
- No pertenece a ninguna cuenta, así que `fediverse:creator` y el `alternate` de ActivityPub no se generan.
- La página de información de la instancia (`/about`) tiene las mismas metaetiquetas. `og:title` y `<title>` son «Acerca de {nombre de la instancia}»
  (`server:preview.about_title`), y `og:url` y `<link rel="canonical">` son `https://<host>/about`.
- Las páginas de documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`) también tienen las mismas metaetiquetas.
  `og:title` y `<title>` son «{nombre del documento} | {nombre de la instancia}» (`server:preview.document_title`), y `og:url` y
  `<link rel="canonical">` son la dirección de esa página. La página de un documento que no está publicado recibe la estructura base genérica.
- La página sobre el rastreador de tarjetas de enlaces (`/about/crawler`) tiene las mismas metaetiquetas, con la misma forma de título y de dirección.

`og:image` se elige en este orden.

1. La primera imagen adjunta a la publicación (su texto alternativo va en `og:image:alt`). No se usa en una publicación marcada como sensible ni
   con advertencia de contenido.
2. La imagen de cabecera del perfil o, si no hay, el avatar (`og:image:alt` es «Imagen de cabecera de {name}» o «Avatar de {name}»).
3. La imagen de vista previa de la instancia (la configuración de la instancia `preview_image`; `og:image:alt` es el texto `preview_image_alt` o, si no hay, «Imagen
   de {nombre de la instancia}»).
4. La versión para vista previa de enlaces del logotipo propio de la instancia ([logotipo e icono](instance-settings.md#logo-and-icon)), un PNG de 1200x630
   en `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` es «Imagen de {nombre de la instancia}»).
5. La imagen por defecto incluida (`https://<host>/preview-image.png`, un PNG de 1200x630. Es una imagen genérica sin el nombre
   de la instancia, y `og:image:alt` describe la imagen).

Así, una página que muestra una vista previa siempre tiene `og:image` y `og:image:alt`.
Las imágenes de 1 a 3 son la variante `full` del archivo multimedia procesado, y el ancho y el alto son los valores del procesamiento.
Si `preview_image` aún se está procesando, falló el procesamiento o no es una imagen, se usa el número 4 (o el 5 si no hay logotipo).
la configuración de la instancia se leen en cada petición, así que `my instance set preview_image ...` surte efecto sin reiniciar (para el
procedimiento, consulta la [configuración de la instancia](instance-settings.md)).

La imagen por defecto se dibuja con la biblioteca estándar en la primera petición tras el arranque y se mantiene en el proceso (unos 10 KB).
Se sirve con `Cache-Control: no-cache` y un ETag.

<!-- source: caching 5bc906452b10 -->
### Caché

- La respuesta lleva `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Una eliminación, un cambio de visibilidad o
  la publicación o retirada de un documento permanece en las cachés compartidas como máximo ese tiempo.
- El ETag se construye a partir de las versiones de la plantilla, de los recursos y de la configuración de la instancia, el idioma, la versión
  (revisión) de la publicación, la visibilidad efectiva, la versión del perfil y la imagen elegida (para la página de inicio, la página
  de información, las páginas de documentos y la página sobre el rastreador: la dirección de la página, su título, la descripción de la instancia y la imagen elegida).
  `If-None-Match` recibe `304`.
- Si pones una CDN delante, incluye `Accept` en la clave de caché (para que el JSON de AP y el HTML no se mezclen).

<!-- source: manual-check bf2c7a86a5de -->
## Comprobación manual

Comprueba cada servicio en un dispositivo real después de una actualización o de un cambio en las configuraciones de vista previa. Necesitas una instancia accesible desde el exterior por HTTPS (los
bots de vista previa no pueden llegar a `localhost`).

<!-- source: preparation 9fd604dc1529 -->
### Preparación

1. Con una cuenta de pruebas, prepara estas publicaciones.
   - Una publicación `public` con una imagen apaisada (por ejemplo, de 1200x630) y texto alternativo
   - Una publicación `public` sin imagen (de una cuenta que tenga imagen de cabecera)
   - Una publicación `public` cuyo cuerpo tenga más de 200 caracteres y contenga emojis (como 👨‍👩‍👧‍👦)
   - Una publicación `unlisted`
   - Una publicación `private`
   - Una publicación de una cuenta cuyo nombre visible contenga `"><script>alert(1)</script>`
2. Comprueba el HTML de cada URL.

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - El estado es 200 y están las metaetiquetas de la tabla anterior. La publicación `unlisted` tiene `noindex`.
   - Una publicación `private` y un ID inexistente dan el mismo cuerpo 404 con `X-Robots-Tag: noindex, noarchive` (y también `noai,
     noimageai` cuando las configuraciones rechazan la IA).

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - El `<script>` del nombre visible aparece escapado como `&lt;script&gt;`.

<!-- source: x 73a2dffdfdbb -->
### X

1. Pega la URL de la publicación con imagen en la pantalla de redacción (o en un mensaje directo).
2. Comprueba que la tarjeta de imagen grande (`summary_large_image`) muestra el título, el dominio y la imagen.
3. Para la URL de la publicación sin imagen, comprueba que la tarjeta muestra la imagen de cabecera. Para una cuenta sin imagen de cabecera
   ni avatar, comprueba que la tarjeta grande muestra la imagen por defecto (`preview_image`, o la imagen incluida cuando
   no está definida).
4. Para una cuenta con solo un avatar cuadrado, comprueba que la tarjeta es la de imagen pequeña (`summary`).

X guarda las tarjetas en caché. Para comprobar un cambio, pega la URL con una consulta (como `?v=2`) para que cuente como una URL distinta.

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. Desde una cuenta de otra instancia, haz una publicación que contenga la URL de la publicación con imagen.
2. Comprueba que aparece una tarjeta de vista previa bajo la publicación con el título, la descripción y la imagen.
3. Comprueba que la cuenta de prueba (`fediverse:creator`) aparece como «autor» bajo la tarjeta y que puedes ir a su
   perfil.

Mastodon guarda la tarjeta en caché en cada servidor que la obtuvo. Para generarla de nuevo, publica con una URL nueva (con una consulta).

<!-- source: discord 4df04642bb64 -->
### Discord

1. Pega la URL de la publicación con imagen en un canal.
2. Comprueba que el contenido incrustado muestra el nombre del sitio (el nombre de la instancia), el título, la descripción y una imagen grande.
3. Para una publicación de más de 200 caracteres, comprueba que la descripción termina con `…` y que ningún emoji queda cortado por la mitad.

<!-- source: slack 9f1e524968f3 -->
### Slack

1. Pega la URL de la publicación con imagen en un canal.
2. Comprueba que la vista previa desplegada muestra el nombre del sitio, el título, la descripción y la imagen.
3. Comprueba que también aparece una vista previa para la URL de una publicación `unlisted` (`noindex` es para los motores de búsqueda y no
   impide las vistas previas).

Slack también guarda las vistas previas en caché. Para comprobar un cambio, añade una consulta a la URL.

<!-- source: line ef9e1e682603 -->
### LINE

1. Envía en un chat la URL de la publicación con imagen.
2. Comprueba que aparece una vista previa con el título, la descripción y la imagen.

LINE también guarda las vistas previas en caché. Para comprobar un cambio, añade una consulta a la URL.

<!-- source: what-must-not-be-visible 187905fe778d -->
### Lo que no debe ser visible

En cada servicio, comprueba que la URL de una publicación `private` y la URL de una publicación inexistente dan la misma vista previa genérica
(solo el nombre de la instancia, o nada) y que no aparece ningún cuerpo, nombre visible ni imagen.

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## Tarjetas de enlaces en las publicaciones

Para mostrar una tarjeta de un enlace en una publicación, un trabajo de este servidor obtiene la página enlazada y su imagen, y respeta lo que el sitio enlazado indica al respecto.
Sus peticiones son `my-linkcard/<version> (+https://<host>/)` para la página, la imagen y `robots.txt`. Todas las demás peticiones del servidor, incluidas las del proxy multimedia, siguen siendo `my/<version> (+https://<host>/)`.
Las peticiones no envían cookies ni `Referer`, y nada indica qué cuenta o publicación enlazó la página.

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### La página para operadores de sitios

`https://<host>/about/crawler` informa a los operadores de los sitios enlazados de qué solicita el rastreador, qué envía, con qué frecuencia y cómo excluirse, y de que pueden pedirte que dejes de obtener contenido de su dominio. Cualquiera puede leerla sin una cuenta. La sección «Motores de búsqueda e IA» de la página de información de la instancia (`/about`) enlaza a ella; el `User-Agent` nombra solo `https://<host>/`.

Dice lo que hace este servidor, a partir de `link_cards` y `link_cards_ignore_robots_txt`, y muestra el contacto de la instancia (`contact_account`, `contact_email`). Define un contacto, para que un sitio tenga dónde preguntar: sin él, la página dice que este servidor no publica ninguno.
Con `link_cards_ignore_robots_txt` activado, la página dice que un sitio no puede rechazar a este servidor en `robots.txt`.

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

El nombre en `robots.txt` es `my-linkcard`, en mayúsculas o minúsculas. Sin un grupo con ese nombre, se aplica el grupo `*`, como indica la RFC 9309. Se leen `Allow`, `Disallow`, `*` y `$`, gana la coincidencia más larga y `Allow` gana un empate; `Crawl-delay` y otros registros no se leen.
`robots.txt` se lee hasta 512 KiB en 10 segundos, y se usa lo que se leyó de un archivo mayor. El servidor guarda solo las reglas de `my-linkcard`, por origen (esquema, host y puerto).

| Respuesta para `robots.txt` | Qué hace el servidor | Se conserva durante |
| --- | --- | --- |
| 200 | Respeta el archivo | 24 horas |
| 4xx distinto de 429 | Sin archivo: obtiene la página | 24 horas |
| 429, 5xx, un fallo de red o una redirección que el servidor rechaza | No obtiene nada del origen | 1 hora |
| Más de 5 redirecciones | Sin archivo: obtiene la página | 24 horas |

- La URL de la página se comprueba antes de solicitarla, sea cual sea la visibilidad de la publicación. Lo mismo ocurre con cada URL a la que redirige, contra el `robots.txt` del origen de esa URL, y una redirección a una URL rechazada termina ahí: el servidor no la solicita y no crea tarjeta.
- Una URL que `robots.txt` no permite no se vuelve a obtener durante 24 horas, como cualquier otro fallo de una página. Cuando el archivo solo no se pudo leer, el servidor vuelve a intentar la página una hora después, hasta el límite de intentos del trabajo.
- Una imagen se comprueba de la misma manera contra su propio origen. Una imagen no permitida deja una tarjeta sin imagen. Cuando el archivo solo no se pudo leer, se mantiene una imagen registrada antes y una imagen nueva se omite hasta la siguiente nueva obtención.
- Una nueva obtención, que una publicación puede provocar 14 días después de la última, elimina la tarjeta cuando `robots.txt` no permite su URL. Cuando el archivo solo no se pudo leer, la tarjeta se mantiene.
- El servidor no recorre sus tarjetas para volver a comprobarlas, así que un sitio que cambia sus reglas pierde una tarjeta en la siguiente nueva obtención de esa tarjeta.

`link_cards_ignore_robots_txt` ([configuración de la instancia](instance-settings.md#settings-one-key-one-value)) desactiva todo esto: el servidor ni obtiene ni usa `robots.txt`. Sigue identificándose como `my-linkcard`, y los sitios enlazados ya no pueden rechazar las obtenciones con `robots.txt`: solo quedan las directivas de sus páginas y una petición a ti de una [regla sobre su dominio](instance-settings.md#link-card-domains), como les indica [la página para operadores de sitios](#the-page-for-site-operators).

<!-- source: directives-of-a-page a854ef2ae8ea -->
### Directivas de una página

El servidor también lee las directivas de la página que obtuvo: `<meta name="robots">` y `<meta name="my-linkcard">` en el `<head>`, y `X-Robots-Tag` sin nombre de bot o con `my-linkcard:`. Las directivas para otro bot no se aplican. Cuando hay varias, gana la más estricta.
Se aplican sea cual sea el valor de `link_cards_ignore_robots_txt`.

| Directiva | Tarjeta |
| --- | --- |
| `nosnippet`, `max-snippet:0` | Sin descripción |
| `noimageindex`, `max-image-preview:none` | Sin imagen, que no se obtiene |
| `noindex`, `nofollow`, `noarchive`, `none` y las demás | Sin cambios |

El título, el nombre del sitio y el autor se mantienen con cualquier directiva; un sitio que no los quiere rechaza la obtención en `robots.txt`.
Las directivas solo se conocen una vez obtenida la página, así que no impiden la petición de la página. Una nueva obtención aplica las directivas que la página tenga entonces.

<!-- source: limits-per-host 41838af70d25 -->
### Límites por host

El rastreador es amable con cada host, sea cual sea el valor de `link_cards_ignore_robots_txt`. Un host es el nombre de la URL, sin el puerto, de modo que un sitio enlazado y el host de sus imágenes tienen límites separados.

- Hace una obtención a la vez de un host, y la siguiente empieza al menos 1 segundo después de que terminara la anterior. Una obtención es una petición con sus redirecciones y, cuando hay que leerlo, el `robots.txt` que necesita. Una página y su imagen son dos obtenciones, también en un mismo host.
- Un `429` o un `503` detiene las obtenciones del host que respondió, también cuando respondió a una redirección, a una imagen o a `robots.txt`. Se detienen durante su `Retry-After`, en segundos o como fecha, durante al menos 1 segundo y como máximo 1 hora. Un `Retry-After` ausente o ilegible equivale a 1 minuto.
- Un trabajo que tiene que esperar su turno vuelve más tarde. Ningún worker espera, y no consume ninguno de los intentos del trabajo, de modo que incluso una ráfaga de enlaces a un mismo sitio termina con todas las tarjetas y sin ningún trabajo fallido. Los trabajos de un mismo host se reactivan a razón de uno por segundo, aproximadamente.
- Una URL cuyo host detuvo al rastreador conserva su tarjeta, no se recuerda como fallida y se obtiene cuando termina la pausa. Tras 5 respuestas así para una URL en 24 horas, la página cuenta como fallida igual que cualquier otra que no sea un `200`, y una imagen se omite de la tarjeta.
- Una tarjeta que espera su imagen conserva durante hasta 2 horas lo que leyó de la página, para no volver a solicitarla.
- Las [reglas sobre un dominio](instance-settings.md#link-card-domains) y las acciones sobre dominios van primero: nada espera a un host del que no se obtiene nada.

Los límites viven en el almacén clave-valor y todos caducan: un worker que se detiene en mitad de una obtención libera su host en 2 minutos.
Estas cifras no son configuraciones. Para ver los límites en funcionamiento:

- `my.link_card.host_pauses` cuenta las pausas por el estado con que respondió el host, y cada pausa registra `link card host paused` en `INFO` con `host`, `status` y `pause_seconds`.
- `my.job.duration` con `my.job.type="content.fetch_link_card"` y `my.outcome="postponed"` cuenta las ejecuciones que volvieron más tarde. Un número alto significa muchos enlaces a pocos sitios o una pausa larga, no un fallo.
