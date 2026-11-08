<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# Motores de búsqueda y rastreadores de IA

Qué comunica el servidor a los motores de búsqueda y a los rastreadores de IA, la configuración que lo cambia y cómo verificar lo que sirve.

robots.txt y las metaetiquetas son declaraciones que los rastreadores pueden respetar o no; no son un bloqueo técnico. Si necesitas bloquear, usa un WAF o limitación de frecuencia.

<!-- source: settings 13dfb1b04948 -->
## Configuración

| Ámbito | Configuración | Dónde cambiarlo | Si no se define |
| --- | --- | --- | --- |
| Servidor | `search_indexing` (`allow` / `deny`) | La sección «Motores de búsqueda e IA» de la página «Búsqueda y rastreadores» de la configuración del servidor en administración, `my instance set`, `PATCH /api/v1/admin/settings` ([Configuración de la instancia](instance-settings.md)) | `deny` |
| Servidor | `ai_usage` (`allow` / `deny_training` / `deny`) | Igual | `deny` |
| Servidor | `indexable_default` (el `indexable` de las cuentas nuevas) | Igual | `false` |
| Servidor | `security_contact` (dónde notificar vulnerabilidades) | Igual | Vacío (no se sirve `security.txt`) |
| Persona | `indexable` (aparecer en la búsqueda) | «Motores de búsqueda e IA» en la configuración del perfil, `PATCH /api/v1/accounts/update_credentials` | `indexable_default` en el registro |
| Persona | `ai_opt_out` (rechazar el entrenamiento y el uso por IA) | Igual | `false` |

- La declaración de una página es la más estricta entre la configuración del servidor y la configuración del propietario de la página. Una persona no puede permitir más de lo que permite el servidor.
- Se indica a los motores de búsqueda que solo pueden indexar los perfiles y las publicaciones `public` de cuentas con `indexable = true`, en un servidor con `search_indexing = allow`.
  Un servidor que quiere ser localizable por la búsqueda cambia tanto `search_indexing` como `indexable_default`. Las personas que se registraron antes de que cambiaras `indexable_default` no se incluyen hasta que elijan `indexable` por sí mismas.
- Un cambio de configuración surte efecto en `/robots.txt`, en las páginas y en las respuestas de contenido multimedia sin reiniciar.

<!-- source: what-the-server-returns 31584e03601b -->
## Qué devuelve el servidor

| Respuesta | Al rechazar la búsqueda | Al rechazar la IA |
| --- | --- | --- |
| Páginas de perfiles y publicaciones `public` | `noindex, noarchive` en `<meta name="robots">` y `X-Robots-Tag` | `noai, noimageai`, `Content-Usage: train-ai=n` (`train-ai=n, ai-use=n` para `deny`) y `tdm-reservation: 1` (encabezado y meta) |
| Publicaciones `unlisted`, páginas 404 de páginas ocultas o inexistentes, páginas de cuentas remotas, pantallas de la aplicación (`/home`, `/settings`, etc.) | `noindex, noarchive` sea cual sea la configuración | Sigue la configuración del servidor |
| `/`, `/about`, `/about/crawler` | Sigue la configuración del servidor | Sigue la configuración del servidor |
| `/media/` (cuando este servidor lo sirve) | Sigue la configuración del servidor. Sin declaración por propietario | Sigue la configuración del servidor. Sin declaración por propietario |
| La API de cliente, los documentos de ActivityPub, `/.well-known/`, `/nodeinfo/`, las pantallas de inicio de sesión y de OAuth | `X-Robots-Tag: noindex` | No se añade (robots.txt basta) |

`noindex` no impide las vistas previas de enlaces (consulta [Vistas previas de enlaces](link-previews.md)).

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` se construye a partir de la configuración del servidor y de la lista incluida de rastreadores de IA (`text/plain; charset=utf-8`, `Cache-Control: public, max-age=3600`, ETag).
Los operadores no pueden añadir reglas propias.

- Si `ai_usage` es `deny_training` o `deny`, los rastreadores de entrenamiento (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, `Applebot-Extended`, etc.) reciben `Disallow: /`.
- Si `ai_usage` es `deny_training` o `deny`, un grupo `User-agent: __usage_ai-training` con `Disallow: /` declara una preferencia contra el entrenamiento de IA para todas las rutas, sea cual sea el rastreador que recupere el contenido
  (el borrador de Internet `draft-kelley-aipref-training-preference-signal`). Nombra un uso, no un rastreador, así que no detiene ninguna recuperación. Con `ai_usage = allow` el grupo se omite, lo que no declara ninguna preferencia, porque su `Allow: /` declararía una preferencia a favor del entrenamiento para todas las rutas, incluidas las páginas de las personas que definieron `ai_opt_out`.
- Si `ai_usage` es `deny`, los rastreadores de búsqueda de IA, asistentes y agentes (`OAI-SearchBot`, `ChatGPT-User`, `Claude-User`, `PerplexityBot`, etc.) también reciben `Disallow: /`.
- A todos los rastreadores (`User-agent: *`) solo se les rechaza `/api/`, `/oauth/`, `/auth/`, `/ap/` y `/media/proxy/`, y la configuración se declara en líneas `Content-Usage` (el borrador de preferencias de IA del IETF) y
  `Content-Signal` (Content Signals). Rechazar la búsqueda no produce `Disallow: /`: bloquear las recuperaciones impediría que los rastreadores leyeran `noindex`,
  y también impediría que los recuperadores de vistas previas de enlaces que obedecen robots.txt funcionaran.
- `/.well-known/` y `/nodeinfo` no se rechazan, para que las listas de servidores y las estadísticas sigan funcionando.
- Cuando `account_domain` es distinto del host, lo sirve el lado del host. El robots.txt del dominio del identificador es asunto del propio operador.

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| Ruta | Se sirve cuando | Contenido |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` es distinto de `allow` | `[{"location": "/*", "tdm-reservation": 1}]` (TDM Reservation Protocol). 404 para `allow` |
| `/.well-known/security.txt` | `security_contact` está definido | `Contact`, `Expires` (30 días después del momento de la respuesta), `Preferred-Languages` (`default_locale` y `en`), `Canonical` (RFC 9116) |

<!-- source: information-page 0f89d6c3c3ce -->
### Página de información

La página de información de la instancia (`/about`) muestra si se permiten la búsqueda y la IA, y qué significa la declaración, a partir de `search_indexing` y `ai_usage` en `GET /api/v1/instance`.
La explicación sitúa las declaraciones frente al artículo 30-4 de la Ley de Propiedad Intelectual de Japón (es posible que tales declaraciones no se traten como un rechazo jurídico) y al artículo 4 de la Directiva DSM de la UE (una reserva legible por máquina de la minería de textos y datos).
No garantiza ningún efecto jurídico.

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## Las recuperaciones propias de este servidor

Esta configuración trata de lo que rastrea este servidor. El servidor también rastrea otros para las tarjetas de los enlaces de las publicaciones, como `my-linkcard`: obedece `robots.txt` y las directivas de las páginas enlazadas y limita sus peticiones por host, como describen las [vistas previas de enlaces](link-previews.md#cards-of-links-in-posts).
Ese rastreador se configura con `link_cards`, `link_cards_ignore_robots_txt` y, por dominio, con los [dominios de tarjetas de enlaces](instance-settings.md#link-card-domains).

Los operadores de esos sitios lo leen en `https://<host>/about/crawler`, al que enlaza `/about` ([la página para operadores de sitios](link-previews.md#the-page-for-site-operators)). Con `link_cards_ignore_robots_txt` activado, no pueden excluirse con `robots.txt`, y la página lo dice: solo pueden limitar la tarjeta con las directivas de sus páginas, o pedirte una regla para su dominio.

<!-- source: limits 93da7e422ee8 -->
## Límites

- El `ai_opt_out` de una persona se declara solo en las páginas que sirve este servidor. El contenido multimedia no tiene propietario en su clave, y algunas configuraciones dejan que una CDN lo sirva directamente, así que el contenido multimedia lleva solo la declaración de todo el servidor.
  Las declaraciones en páginas servidas por otros servidores quedan en manos de esos servidores (no se envía ninguna extensión personalizada de ActivityPub).
- `ai_usage = deny` también declara un rechazo de las acciones en las que un usuario de ChatGPT o un servicio similar le hace leer una página de este servidor.
- El `indexable` de una cuenta remota (el `toot:indexable` de Mastodon) se recibe y se almacena. Las páginas de cuentas remotas son siempre `noindex` en este servidor.
- `indexable` también decide si las publicaciones de una persona están en la propia [búsqueda de texto completo](search.md) de este servidor, en un servidor con un motor de búsqueda. Allí abarca las visibilidades de `search_visibilities`, que pueden incluir `local`. `search_indexing` no tiene efecto en esa búsqueda, y lo que se comunica a los motores de búsqueda sigue siendo lo que dice esta página.

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## Servir contenido multimedia desde almacenamiento de objetos o una CDN

Cuando el almacenamiento de objetos o una CDN sirve el contenido multimedia directamente, las respuestas de `/media/` no pasan por este servidor y no llevan los encabezados de la tabla anterior.
La declaración de todo el servidor sigue aplicándose a todo mediante `/robots.txt`; para añadir también los encabezados, añade lo siguiente a `/media/*` en las reglas de encabezados de respuesta de la CDN (un ejemplo para `ai_usage = deny`).

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

Cuando cambies la configuración, cambia también las reglas de la CDN para que coincidan. `/media/proxy/` lo sirve este servidor, así que no necesita ninguna regla.

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## La lista de rastreadores de IA

La lista se genera a partir de `robots.json` de [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) y se incluye en el binario. No se recupera en tiempo de ejecución, así que una lista más reciente llega con una actualización.

<!-- source: verify 1633f03abedb -->
## Verificar

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- En un servidor sin configuración propia, `/robots.txt` tiene `User-agent: GPTBot`, `User-agent: __usage_ai-training` y `Content-Usage: search=n, train-ai=n, ai-use=n`,
  y los perfiles llevan `X-Robots-Tag: noindex, noarchive, noai, noimageai`.
- Con `search_indexing = allow` y `ai_usage = allow`, el perfil de una cuenta que eligió `indexable` no lleva `X-Robots-Tag`, y `/.well-known/tdmrep.json` es 404.
