<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# Aperçus de liens (OGP / X Card / fediverse:creator)

Les aperçus que d’autres services affichent des pages de ce serveur, ce que le serveur renvoie pour eux, et les étapes pour les vérifier à la main. Les cartes que ce serveur affiche pour les liens de ses propres publications
sont décrites dans [Cartes des liens dans les publications](#cards-of-links-in-posts) plus bas ; la façon d’arrêter de les récupérer depuis un domaine est décrite dans les [paramètres de l’instance](instance-settings.md#link-card-domains).

<!-- source: what-the-server-returns fe60560f1b91 -->
## Ce que le serveur renvoie

`my serve` renvoie le HTML de la page d’accueil (`https://<host>/`), de la page d’informations sur l’instance (`https://<host>/about`),
des pages des documents publiés (`/about/terms`, `/about/privacy`, `/about/commerce`), de la page consacrée à l’outil de récupération des cartes de lien (`/about/crawler`), des profils
(`https://<host>/@alice`) et des publications (`https://<host>/@alice/status/<post ID>`) en intégrant des balises meta dans l’enveloppe du
client web.
Une requête de HTML est toujours traitée comme celle d’un visiteur non connecté ; un aperçu n’affiche donc que ce qu’un visiteur non connecté peut voir.

| Page | Réponse |
| --- | --- |
| La page d’accueil (`/`) et la page d’informations sur l’instance (`/about`) | 200. L’enveloppe avec des balises meta qui présentent l’instance (voir « La page d’accueil » ci-dessous) |
| La page consacrée à l’outil de récupération des cartes de lien (`/about/crawler`) | 200. L’enveloppe avec des balises meta qui présentent l’instance, intitulée « Outil de récupération des cartes de lien \| nom de l’instance » |
| Les pages des documents publiés (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. L’enveloppe avec des balises meta qui présentent l’instance, intitulée « nom du document \| nom de l’instance ». La page d’un document non publié reçoit l’enveloppe générique |
| Une publication `public`, le profil d’un compte local actif | 200. L’enveloppe avec des balises meta |
| Une publication `unlisted` | 200. L’enveloppe avec des balises meta, plus `<meta name="robots" content="noindex, noarchive">` et `X-Robots-Tag: noindex, noarchive` |
| Une publication `local`, `private`, `direct`, supprimée ou inexistante, et le profil et les publications d’un compte suspendu, désactivé, supprimé ou limité | 404. L’enveloppe générique et `X-Robots-Tag: noindex, noarchive`. Le corps et les en-têtes sont les mêmes quelle que soit la raison |
| `/@<old username>/status/<post ID>` | 301, vers l’URL avec le nom d’utilisateur actuel |
| La page d’un hashtag (`/tags/<tag>`) | 200. L’enveloppe générique, avec le lien vers le flux Atom du hashtag (voir « L’enveloppe générique » ci-dessous) |
| Toute autre page du client web (explorer, les pages publiques des coûts et des soutiens, la connexion, etc.) | 200. L’enveloppe générique, avec les balises meta de l’instance (voir « L’enveloppe générique » ci-dessous) |

Chaque page porte aussi les déclarations destinées aux moteurs de recherche et aux robots d’IA (balises meta telles que `noindex` et `noai`, et
`X-Robots-Tag`, `Content-Usage` et `tdm-reservation`), selon les paramètres de l’instance et ceux de son propriétaire
([moteurs de recherche et robots d’IA](crawlers.md)). Le `noindex` du tableau ci-dessus est toujours ajouté, quels que soient les paramètres.
`noindex` n’empêche pas les aperçus de liens.

Une requête avec un `Accept` ActivityPub ne reçoit pas l’enveloppe ; c’est la partie ActivityPub qui y répond (un profil répond avec l’`id` de
l’acteur, et une publication qu’un visiteur non connecté peut voir répond par un 302 vers son `id`). Chaque réponse porte `Vary: Accept`.

<!-- source: meta-tags 1a514282794e -->
### Balises meta

| Balise | Contenu |
| --- | --- |
| `og:title`, `<title>` | « Nom affiché (@nom d’utilisateur@domaine du compte) ». Le nom d’utilisateur si le nom affiché est vide. Pour un article, son titre |
| `og:type` | `profile` pour un profil, `article` pour une publication et un article |
| `og:url`, `<link rel="canonical">` | L’URL canonique (l’URL avec le nom d’utilisateur actuel) |
| `og:description` | Le texte brut du corps de la publication (la biographie pour un profil) sans balises, coupé à 200 graphèmes. Pour une publication avec un avertissement de contenu, uniquement le texte de l’avertissement. Pour un article, son résumé, y compris lorsqu’il est marqué comme sensible ; le corps d’un article ne figure jamais dans la page |
| `og:image`, `og:image:alt` / `width` / `height` | L’image choisie dans l’ordre ci-dessous |
| `og:site_name` | Le nom de l’instance (le texte `name`, identique dans toutes les langues ; `host` lorsqu’il n’y en a pas ; [nom et domaine](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | La langue de la publication. Lorsqu’elle n’est pas définie (`und`), la langue de publication par défaut de l’auteur, et à défaut le `default_locale` de l’instance. Elle ne change pas avec `Accept-Language`. `og:locale` a la forme `language_TERRITORY` : une langue sans région reçoit sa région habituelle (`ja` → `ja_JP`, `en` → `en_US`, `pt` → `pt_BR`), et `<html lang>` conserve la balise de langue telle quelle |
| `article:published_time`, `article:author` | Publications uniquement. L’heure de la publication (UTC) et l’URL du profil de l’auteur |
| `profile:username` | Profils uniquement. `username@account domain` |
| `twitter:card` | `summary_large_image` lorsque l’image est plus large qu’environ 2:1 (largeur / hauteur >= 1.7) et d’au moins 300x157, sinon `summary` |
| `fediverse:creator` | `@username@account domain`. Mastodon l’affiche comme « auteur » |
| `<link rel="alternate" type="application/activity+json">` | L’`id` AP (l’acteur ou l’objet de la publication) |
| `<link rel="alternate" type="application/atom+xml">` | Uniquement les profils qui ont un flux Atom : l’adresse du flux (`https://<host>/@<username>.atom`), intitulée comme `og:title`. Un profil dont le propriétaire a désactivé le flux, ainsi qu’un compte limité ou qui a migré, n’en ont pas |

Les balises meta de la page d’accueil présentent l’instance elle-même.

- `og:type` vaut `website`, `og:title`, `<title>` et `og:site_name` sont le nom de l’instance, et `og:url` et `<link rel="canonical">` valent `https://<host>/`.
- `og:description` est le texte d’instance `short_description` coupé à 200 graphèmes. Il est omis lorsqu’il n’y en a pas.
- La langue (`og:locale`, `<html lang dir>`) est le `default_locale` de l’instance. Le nom et la description sont la même valeur dans toutes les langues.
- `og:image` est l’image de l’instance : le numéro 3 ci-dessous, ou le numéro 4, 5 ou 6 s’il n’y en a pas.
- Elle n’appartient à aucun compte ; `fediverse:creator` et l’`alternate` ActivityPub ne sont donc pas produits.
- La page d’informations sur l’instance (`/about`) a les mêmes balises meta. `og:title` et `<title>` sont « À propos de {nom de l’instance} » (`server:preview.about_title`), et `og:url` et `<link rel="canonical">` valent `https://<host>/about`.
- Les pages des documents publiés (`/about/terms`, `/about/privacy`, `/about/commerce`) ont elles aussi les mêmes balises meta. `og:title` et `<title>` sont « {nom du document} | {nom de l’instance} » (`server:preview.document_title`), et `og:url` et `<link rel="canonical">` sont l’adresse de cette page. La page d’un document non publié reçoit l’enveloppe générique.
- La page consacrée à l’outil de récupération des cartes de lien (`/about/crawler`) a les mêmes balises meta, avec la même forme de titre et d’adresse.

L’enveloppe générique, que renvoient toutes les autres pages et le 404 d’une publication masquée, porte les mêmes balises que la page d’accueil
sans adresse propre : `og:type` `website`, le nom de l’instance comme `og:site_name` et `og:title`, `short_description` comme `og:description`, `og:locale`, l’image de l’instance et `twitter:card`. Elle n’a ni `og:url` ni `<link rel="canonical">`, de sorte qu’une seule copie sert tous les chemins, et que le 404 d’une publication masquée reste identique à toute autre page.
La page d’un hashtag ajoute seulement `<link rel="alternate" type="application/atom+xml">` vers le flux du hashtag (`https://<host>/tags/<tag>.atom`), intitulé « #tag sur nom de l’instance » (`server:feed.tag_title`), qu’une publication ait déjà utilisé ou non le hashtag. Un lien partagé vers un hashtag ou vers la page publique des coûts affiche la carte de l’instance.

Les flux eux-mêmes sont des documents Atom 1.0 des 20 publications publiques locales les plus récentes qui ne sont ni des réponses ni des boosts, avec `ETag`, `Last-Modified` et `Cache-Control: public, max-age=60`. L’entrée d’un article est son titre et son résumé, avec le lien vers l’article ; son corps reste sur la page de l’article. Chaque profil qui a un flux a aussi un flux réservé aux articles, `https://<host>/@<username>/articles.atom`, intitulé « Articles de nom (@handle) » (`server:feed.articles_title`), qui suit les mêmes règles ; la page du profil ne lie que son flux principal. Le serveur conserve chaque flux rendu dans son magasin clé-valeur pendant une minute ; un changement atteint donc un flux en une minute environ. Un flux porte les mêmes `X-Robots-Tag` et déclarations d’IA que son profil ; le flux d’un hashtag est toujours `noindex`, et déclare le refus d’IA de n’importe lequel de ses auteurs. Un profil sans flux, et un hashtag qu’aucune publication n’a utilisé, répondent 404.

`og:image` est choisie dans cet ordre.

1. La première image jointe à la publication, ou la couverture d’un article (son texte alternatif va dans `og:image:alt`). Elle n’est pas utilisée pour une publication ou un article marqué comme sensible, ni pour une publication avec un avertissement de contenu.
2. L’image d’en-tête du profil, ou à défaut l’avatar (`og:image:alt` vaut « Image d’en-tête de {name} » ou « Avatar de {name} »).
3. L’image d’aperçu de l’instance (le paramètre d’instance `preview_image`, sous « Aperçus de liens » dans Paramètres → Général de l’administration ; `og:image:alt` est le texte `preview_image_alt`, ou « Image de {nom de l’instance} » s’il n’y en a pas).
4. La miniature de l’instance (le paramètre d’instance `thumbnail`, l’image de la page Paramètres → Général ; `og:image:alt` est le texte `thumbnail_alt`, ou « Image de {nom de l’instance} » s’il n’y en a pas).
5. La version pour aperçu de lien du logo propre à l’instance ([logo et icône](instance-settings.md#logo-and-icon)), un PNG de 1200x630 à `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` vaut « Image de {nom de l’instance} »).
6. L’image par défaut fournie (`https://<host>/preview-image.png`, un PNG de 1200x630 du logotype mycroblog sur fond blanc, sans le nom de l’instance ; `og:image:alt` vaut « Le logo de mycroblog »).

Les numéros 3 à 6 sont l’image de l’instance. Une page qui affiche un aperçu a donc toujours `og:image` et `og:image:alt`.
Les images de 1 à 4 sont la variante `large` du média traité, et la largeur et la hauteur sont les valeurs issues du traitement.
Si `preview_image` n’est pas définie, est encore en cours de traitement, que son traitement a échoué ou qu’elle n’est pas une image, la miniature est utilisée, puis le logo, puis l’image fournie. Les paramètres d’instance sont lus à chaque requête : `my instance set preview_image ...` prend donc effet sans redémarrage (pour la procédure, voir les [paramètres de l’instance](instance-settings.md)).

L’image fournie fait partie du binaire (environ 20 Ko). Elle est servie avec `Cache-Control: no-cache` et un ETag.

<!-- source: caching e6ace7ad3fd0 -->
### Mise en cache

- La réponse porte `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Une suppression, un changement de visibilité, ou la publication ou la dépublication d’un document reste dans les caches partagés au plus pendant cette durée.
- L’ETag est construit à partir des versions du gabarit, des ressources et des paramètres de l’instance, de la langue, de la version (révision) de la publication, de la visibilité effective, de la version du profil et de l’image choisie (pour la page d’accueil, la page d’informations, les pages de documents et la page de l’outil de récupération : l’adresse de la page, son titre, la description de l’instance et l’image choisie ; pour l’enveloppe générique : le nom de l’instance, sa description et l’image de l’instance).
  `If-None-Match` reçoit `304`.
- Si vous placez un CDN devant, incluez `Accept` dans la clé de cache (afin que le JSON AP et le HTML ne soient pas confondus).

<!-- source: manual-check bf2c7a86a5de -->
## Vérification manuelle

Vérifiez chaque service sur un appareil réel après une mise à niveau ou une modification des paramètres d’aperçu. Il vous faut une instance joignable de l’extérieur en HTTPS (les
robots d’aperçu ne peuvent pas atteindre `localhost`).

<!-- source: preparation 9fd604dc1529 -->
### Préparation

1. Avec un compte de test, préparez ces publications.
   - Une publication `public` avec une image en paysage (par exemple 1200x630) et un texte alternatif
   - Une publication `public` sans image (depuis un compte qui a une image d’en-tête)
   - Une publication `public` dont le corps dépasse 200 caractères et contient des émoji (comme 👨‍👩‍👧‍👦)
   - Une publication `unlisted`
   - Une publication `private`
   - Une publication d’un compte dont le nom affiché contient `"><script>alert(1)</script>`
2. Vérifiez le HTML de chaque URL.

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - Le statut est 200 et les balises meta du tableau ci-dessus sont présentes. La publication `unlisted` a `noindex`.
   - Une publication `private` et un ID inexistant donnent le même corps 404 avec `X-Robots-Tag: noindex, noarchive` (et aussi `noai,
     noimageai` lorsque les paramètres refusent l’IA).

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - Le `<script>` du nom affiché est échappé en `&lt;script&gt;`.

<!-- source: x 73a2dffdfdbb -->
### X

1. Collez l’URL de la publication avec une image dans l’écran de rédaction (ou dans un message privé).
2. Vérifiez que la grande carte image (`summary_large_image`) affiche le titre, le domaine et l’image.
3. Pour l’URL de la publication sans image, vérifiez que la carte affiche l’image d’en-tête. Pour un compte sans image d’en-tête
   ni avatar, vérifiez que la grande carte affiche l’image par défaut (`preview_image`, ou l’image fournie lorsqu’elle n’est
   pas définie).
4. Pour un compte avec seulement un avatar carré, vérifiez que la carte est la petite carte image (`summary`).

X met les cartes en cache. Pour vérifier une modification, collez l’URL avec une requête (comme `?v=2`) afin qu’elle compte comme une autre URL.

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. Depuis un compte d’une autre instance, faites une publication qui contient l’URL de la publication avec une image.
2. Vérifiez qu’une carte d’aperçu apparaît sous la publication avec le titre, la description et l’image.
3. Vérifiez que le compte de test (`fediverse:creator`) est affiché comme « auteur » sous la carte et que vous pouvez accéder à son
   profil.

Mastodon met la carte en cache sur chaque serveur qui l’a récupérée. Pour la refaire, publiez avec une nouvelle URL (avec une requête).

<!-- source: discord 4df04642bb64 -->
### Discord

1. Collez l’URL de la publication avec une image dans un salon.
2. Vérifiez que l’embed affiche le nom du site (le nom de l’instance), le titre, la description et une grande image.
3. Pour une publication de plus de 200 caractères, vérifiez que la description se termine par `…` et qu’aucun émoji n’est coupé en plein milieu.

<!-- source: slack 9f1e524968f3 -->
### Slack

1. Collez l’URL de la publication avec une image dans un canal.
2. Vérifiez que l’aperçu déplié affiche le nom du site, le titre, la description et l’image.
3. Vérifiez qu’un aperçu apparaît aussi pour l’URL d’une publication `unlisted` (`noindex` concerne les moteurs de recherche et n’empêche pas
   les aperçus).

Slack met aussi les aperçus en cache. Pour vérifier une modification, ajoutez une requête à l’URL.

<!-- source: line ef9e1e682603 -->
### LINE

1. Envoyez l’URL de la publication avec une image dans une conversation.
2. Vérifiez qu’un aperçu avec le titre, la description et l’image apparaît.

LINE met aussi les aperçus en cache. Pour vérifier une modification, ajoutez une requête à l’URL.

<!-- source: what-must-not-be-visible 187905fe778d -->
### Ce qui ne doit pas être visible

Sur chaque service, vérifiez que l’URL d’une publication `private` et l’URL d’une publication inexistante donnent le même aperçu générique
(seulement le nom de l’instance, ou rien) et qu’aucun corps, nom affiché ou image n’apparaît.

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## Cartes des liens dans les publications

Pour afficher une carte pour un lien dans une publication, la tâche de ce serveur récupère la page liée et son image, et respecte ce que le site lié en dit.
Ses requêtes sont `my-linkcard/<version> (+https://<host>/)` pour la page, l’image et `robots.txt`. Toutes les autres requêtes du serveur, y compris celles du proxy média, restent `my/<version> (+https://<host>/)`.
Les requêtes n’envoient ni cookies ni `Referer`, et rien n’indique quel compte ou quelle publication a lié la page.

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### La page destinée aux exploitants de sites

`https://<host>/about/crawler` indique aux exploitants des sites liés ce que le robot demande, ce qu’il envoie, à quelle fréquence, comment s’en exclure, et qu’ils peuvent vous demander d’arrêter de récupérer depuis leur domaine. N’importe qui peut la lire sans compte. La section « Moteurs de recherche et IA » de la page d’informations de l’instance (`/about`) y renvoie ; le `User-Agent` ne nomme que `https://<host>/`.

Elle indique ce que fait ce serveur, d’après `link_cards` et `link_cards_ignore_robots_txt`, et elle présente le contact de l’instance (`contact_account`, `contact_email`). Définissez un contact, afin qu’un site ait un endroit où écrire : sans contact, la page indique que ce serveur n’en publie aucun.
Lorsque `link_cards_ignore_robots_txt` est activé, la page indique qu’un site ne peut pas refuser ce serveur dans `robots.txt`.

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

Le nom dans `robots.txt` est `my-linkcard`, quelle que soit la casse. Sans groupe de ce nom, le groupe `*` s’applique, comme le dit la RFC 9309. `Allow`, `Disallow`, `*` et `$` sont lus, la correspondance la plus longue l’emportant et `Allow` l’emportant en cas d’égalité ; `Crawl-delay` et les autres enregistrements ne sont pas lus.
`robots.txt` est lu jusqu’à 512 Kio en 10 secondes, et ce qui a été lu d’un fichier plus volumineux est utilisé. Le serveur ne conserve que les règles pour `my-linkcard`, par origine (schéma, hôte et port).

| Réponse pour `robots.txt` | Ce que fait le serveur | Conservé pendant |
| --- | --- | --- |
| 200 | Respecte le fichier | 24 heures |
| 4xx autre que 429 | Pas de fichier : récupère | 24 heures |
| 429, 5xx, un échec réseau ou une redirection que le serveur refuse | Ne récupère rien depuis l’origine | 1 heure |
| Plus de 5 redirections | Pas de fichier : récupère | 24 heures |

- L’URL de la page est vérifiée avant d’être demandée, quelle que soit la visibilité de la publication. Il en va de même pour chaque URL vers laquelle elle redirige, d’après le `robots.txt` de l’origine de cette URL, et une redirection vers une URL refusée s’arrête là : le serveur ne la demande pas et ne crée aucune carte.
- Une URL que `robots.txt` interdit n’est pas récupérée de nouveau pendant 24 heures, comme tout autre échec d’une page. Lorsque le fichier n’a simplement pas pu être lu, le serveur réessaie la page une heure plus tard, jusqu’à la limite des tentatives de la tâche.
- Une image est vérifiée de la même façon d’après sa propre origine. Une image interdite laisse une carte sans image. Lorsque le fichier n’a simplement pas pu être lu, une image enregistrée précédemment reste, et une nouvelle image est omise jusqu’à la prochaine récupération.
- Une nouvelle récupération, qu’une publication peut provoquer 14 jours après la dernière, supprime la carte lorsque `robots.txt` interdit son URL. Lorsque le fichier n’a simplement pas pu être lu, la carte reste.
- Le serveur ne parcourt pas ses cartes pour les revérifier : un site qui change ses règles perd donc une carte à la prochaine récupération de cette carte.

`link_cards_ignore_robots_txt` ([paramètres de l’instance](instance-settings.md#settings-one-key-one-value)) désactive tout cela : le serveur ne récupère ni n’utilise `robots.txt`. Il se nomme toujours `my-linkcard`, et les sites liés ne peuvent alors pas refuser les récupérations avec `robots.txt` : il ne reste que les directives de leurs pages et une demande qui vous est adressée pour une [règle sur leur domaine](instance-settings.md#link-card-domains), comme [la page destinée aux exploitants de sites](#the-page-for-site-operators) le leur indique.

<!-- source: directives-of-a-page a854ef2ae8ea -->
### Directives d’une page

Le serveur lit aussi les directives de la page qu’il a récupérée : `<meta name="robots">` et `<meta name="my-linkcard">` dans le `<head>`, et `X-Robots-Tag` sans nom de robot ou avec `my-linkcard:`. Les directives destinées à un autre robot ne s’appliquent pas. Lorsqu’il y en a plusieurs, la plus stricte l’emporte.
Elles s’appliquent quelle que soit la valeur de `link_cards_ignore_robots_txt`.

| Directive | Carte |
| --- | --- |
| `nosnippet`, `max-snippet:0` | Pas de description |
| `noimageindex`, `max-image-preview:none` | Pas d’image, qui n’est pas récupérée |
| `noindex`, `nofollow`, `noarchive`, `none` et les autres | Aucun changement |

Le titre, le nom du site et l’auteur restent présents quelle que soit la directive ; un site qui n’en veut pas refuse la récupération dans `robots.txt`.
Les directives ne sont connues qu’une fois la page récupérée : elles n’empêchent donc pas la requête de la page. Une nouvelle récupération applique les directives que la page a alors.

<!-- source: limits-per-host 41838af70d25 -->
### Limites par hôte

Le robot est ménagé envers chaque hôte, quelle que soit la valeur de `link_cards_ignore_robots_txt`. Un hôte est le nom dans l’URL, sans le port : un site lié et l’hôte de ses images ont donc des limites distinctes.

- Il effectue une seule récupération à la fois depuis un hôte, et la suivante commence au moins 1 seconde après la fin de la précédente. Une récupération est une requête avec ses redirections et, lorsqu’il faut le lire, le `robots.txt` dont elle a besoin. Une page et son image sont deux récupérations, y compris sur un même hôte.
- Un `429` ou un `503` arrête les récupérations depuis l’hôte qui a répondu, y compris lorsqu’il a répondu à une redirection, une image ou `robots.txt`. Elles s’arrêtent pendant la durée de son `Retry-After`, en secondes ou sous forme de date, d’au moins 1 seconde et d’au plus 1 heure. Un `Retry-After` absent ou illisible vaut 1 minute.
- Une tâche qui doit attendre son tour revient plus tard. Aucun worker n’attend, et elle ne consomme aucune des tentatives de la tâche : même une rafale de liens vers un seul site aboutit donc à toutes les cartes et à aucune tâche en échec. Les tâches d’un même hôte se réveillent à raison d’environ une par seconde.
- Une URL dont l’hôte a arrêté le robot garde sa carte, n’est pas retenue comme échouée, et est récupérée à la fin de la pause. Après 5 réponses de ce genre pour une URL en 24 heures, la page compte comme échouée comme toute autre qui n’est pas un `200`, et une image est omise de la carte.
- Une carte qui attend son image conserve ce qu’elle a lu de la page jusqu’à 2 heures, afin que la page ne soit pas redemandée.
- Les [règles sur un domaine](instance-settings.md#link-card-domains) et les actions sur les domaines passent en premier : rien n’attend pour un hôte depuis lequel rien n’est récupéré.

Les limites vivent dans le magasin clé-valeur et expirent toutes : un worker qui s’arrête au milieu d’une récupération libère son hôte en 2 minutes.
Ces valeurs ne sont pas des paramètres. Pour voir les limites à l’œuvre :

- `my.link_card.host_pauses` compte les pauses selon le statut qu’a répondu l’hôte, et chaque pause consigne `link card host paused` au niveau `INFO` avec `host`, `status` et `pause_seconds`.
- `my.job.duration` avec `my.job.type="content.fetch_link_card"` et `my.outcome="postponed"` compte les exécutions qui sont revenues plus tard. Un grand nombre signifie beaucoup de liens vers peu de sites ou une longue pause, et non un échec.
