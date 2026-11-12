<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# Moteurs de recherche et robots d’IA

Ce que le serveur indique aux moteurs de recherche et aux robots d’IA, les paramètres qui le modifient et la manière de vérifier ce qu’il sert.

Le fichier robots.txt et les balises meta sont des déclarations que les robots peuvent respecter ou non ; ce ne sont pas un blocage technique. Si vous devez bloquer, utilisez un WAF ou une limitation de débit.

<!-- source: settings 13dfb1b04948 -->
## Paramètres

| Portée | Paramètre | Où le modifier | Valeur par défaut |
| --- | --- | --- | --- |
| Serveur | `search_indexing` (`allow` / `deny`) | La section « Moteurs de recherche et IA » de la page « Recherche et robots » des paramètres d’administration du serveur, `my instance set`, `PATCH /api/v1/admin/settings` ([Paramètres du serveur](instance-settings.md)) | `deny` |
| Serveur | `ai_usage` (`allow` / `deny_training` / `deny`) | Identique | `deny` |
| Serveur | `indexable_default` (le `indexable` des nouveaux comptes) | Identique | `false` |
| Serveur | `security_contact` (où signaler les vulnérabilités) | Identique | Vide (aucun `security.txt` n’est servi) |
| Personne | `indexable` (apparaître dans la recherche) | « Moteurs de recherche et IA » dans les paramètres du profil, `PATCH /api/v1/accounts/update_credentials` | `indexable_default` à l’inscription |
| Personne | `ai_opt_out` (refuser l’entraînement et l’utilisation par l’IA) | Identique | `false` |

- La déclaration d’une page est la plus stricte entre le paramètre du serveur et celui du propriétaire de la page. Une personne ne peut pas autoriser plus que le serveur.
- Les moteurs de recherche sont informés qu’ils peuvent indexer uniquement les profils et les publications `public` des comptes avec `indexable = true`, sur un serveur avec `search_indexing = allow`.
  Un serveur qui souhaite être référencé modifie à la fois `search_indexing` et `indexable_default`. Les personnes inscrites avant que vous ne modifiiez `indexable_default` ne sont pas incluses tant qu’elles n’ont pas choisi elles-mêmes `indexable`.
- Un paramètre modifié prend effet dans `/robots.txt`, les pages et les réponses de médias sans redémarrage.

<!-- source: what-the-server-returns 31584e03601b -->
## Ce que le serveur renvoie

| Réponse | Lorsque la recherche est refusée | Lorsque l’IA est refusée |
| --- | --- | --- |
| Pages de profils et de publications `public` | `noindex, noarchive` dans `<meta name="robots">` et `X-Robots-Tag` | `noai, noimageai`, `Content-Usage: train-ai=n` (`train-ai=n, ai-use=n` pour `deny`) et `tdm-reservation: 1` (en-tête et meta) |
| Publications `unlisted`, pages 404 pour les pages masquées ou inexistantes, pages de comptes distants, écrans de l’application (`/home`, `/settings`, etc.) | `noindex, noarchive` quel que soit le paramètre | Suit le paramètre du serveur |
| `/`, `/about`, `/about/crawler` | Suit le paramètre du serveur | Suit le paramètre du serveur |
| `/media/` (lorsque ce serveur le sert) | Suit le paramètre du serveur. Aucune déclaration par propriétaire | Suit le paramètre du serveur. Aucune déclaration par propriétaire |
| L’API client, les documents ActivityPub, `/.well-known/`, `/nodeinfo/`, les écrans de connexion et d’OAuth | `X-Robots-Tag: noindex` | Non ajouté (robots.txt suffit) |

`noindex` n’empêche pas les aperçus de liens (consultez [Aperçus de liens](link-previews.md)).

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` est construit à partir des paramètres du serveur et de la liste fournie des robots d’IA (`text/plain; charset=utf-8`, `Cache-Control: public, max-age=3600`, ETag).
Les opérateurs ne peuvent pas ajouter de règles qui leur soient propres.

- Si `ai_usage` vaut `deny_training` ou `deny`, les robots d’entraînement (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, `Applebot-Extended`, etc.) reçoivent `Disallow: /`.
- Si `ai_usage` vaut `deny_training` ou `deny`, un groupe `User-agent: __usage_ai-training` avec `Disallow: /` exprime une préférence contre l’entraînement de l’IA pour tous les chemins, quel que soit le robot qui récupère le contenu
  (l’Internet-Draft `draft-kelley-aipref-training-preference-signal`). Il désigne un usage, pas un robot, et n’empêche donc aucune récupération. Avec `ai_usage = allow`, le groupe est omis, ce qui n’exprime aucune préférence, car son `Allow: /` exprimerait une préférence en faveur de l’entraînement pour tous les chemins, y compris les pages des personnes qui ont activé `ai_opt_out`.
- Si `ai_usage` vaut `deny`, les robots de la recherche par IA, des assistants et des agents (`OAI-SearchBot`, `ChatGPT-User`, `Claude-User`, `PerplexityBot`, etc.) reçoivent également `Disallow: /`.
- Tous les robots (`User-agent: *`) ne se voient refuser que `/api/`, `/oauth/`, `/auth/`, `/ap/` et `/media/proxy/`, et les paramètres sont exprimés dans des lignes `Content-Usage` (le brouillon IETF AI Preferences) et
  `Content-Signal` (Content Signals). Refuser la recherche ne produit pas `Disallow: /` : bloquer les récupérations empêcherait les robots de lire `noindex`
  et empêcherait aussi les outils d’aperçu de liens qui respectent robots.txt.
- `/.well-known/` et `/nodeinfo` ne sont pas refusés, afin que les listes de serveurs et les statistiques continuent de fonctionner.
- Lorsque `account_domain` est distinct de l’hôte, c’est l’hôte qui le sert. Le robots.txt du domaine de l’identifiant relève de l’opérateur.

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| Chemin | Servi lorsque | Contenu |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` vaut autre chose que `allow` | `[{"location": "/*", "tdm-reservation": 1}]` (TDM Reservation Protocol). 404 pour `allow` |
| `/.well-known/security.txt` | `security_contact` est défini | `Contact`, `Expires` (30 jours après l’heure de la réponse), `Preferred-Languages` (`default_locale` et `en`), `Canonical` (RFC 9116) |

<!-- source: information-page 0f89d6c3c3ce -->
### Page d’informations

La page d’informations du serveur (`/about`) indique si la recherche et l’IA sont autorisées, et ce que signifie la déclaration, à partir de `search_indexing` et `ai_usage` dans `GET /api/v1/instance`.
L’explication situe ces déclarations par rapport à l’article 30-4 de la loi japonaise sur le droit d’auteur (de telles déclarations peuvent ne pas être traitées comme un refus juridique) et à l’article 4 de la directive européenne DSM (une réserve lisible par machine pour la fouille de textes et de données).
Elle ne garantit aucun effet juridique.

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## Les récupérations de ce serveur

Ces paramètres concernent ce qui explore ce serveur. Le serveur explore aussi les autres serveurs pour les cartes des liens des publications, sous le nom `my-linkcard` : il respecte `robots.txt` et les directives des pages liées et limite ses requêtes par hôte, comme le décrivent les [aperçus de liens](link-previews.md#cards-of-links-in-posts).
Ce robot est configuré avec `link_cards`, `link_cards_ignore_robots_txt` et, par domaine, avec les [domaines des cartes de liens](instance-settings.md#link-card-domains).

Les opérateurs de ces sites en prennent connaissance sur `https://<host>/about/crawler`, vers lequel `/about` renvoie ([la page destinée aux opérateurs de sites](link-previews.md#the-page-for-site-operators)). Lorsque `link_cards_ignore_robots_txt` est activé, ils ne peuvent pas s’exclure avec `robots.txt`, et la page l’indique : ils peuvent seulement limiter la carte avec les directives de leurs pages, ou vous demander une règle pour leur domaine.

<!-- source: limits 93da7e422ee8 -->
## Limites

- Le `ai_opt_out` d’une personne n’est exprimé que sur les pages que ce serveur sert. Un média n’a pas de propriétaire dans sa clé, et certaines configurations laissent un CDN le servir directement ; les médias ne portent donc que la déclaration valable pour tout le serveur.
  Les déclarations sur les pages servies par d’autres serveurs relèvent de ces serveurs (aucune extension ActivityPub personnalisée n’est envoyée).
- `ai_usage = deny` exprime aussi un refus des actions par lesquelles un utilisateur de ChatGPT ou d’un service similaire lui fait lire une page de ce serveur.
- Le `indexable` d’un compte distant (le `toot:indexable` de Mastodon) est reçu et stocké. Les pages des comptes distants sont toujours en `noindex` sur ce serveur.
- `indexable` détermine aussi si les publications d’une personne figurent dans la [recherche plein texte](search.md) de ce serveur, sur un serveur doté d’un moteur de recherche. Il couvre alors les visibilités de `search_visibilities`, qui peuvent inclure `local`. `search_indexing` n’a aucun effet sur cette recherche, et ce qui est indiqué aux moteurs de recherche reste tel que cette page le décrit.

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## Servir les médias depuis un stockage d’objets ou un CDN

Lorsqu’un stockage d’objets ou un CDN sert directement les médias, les réponses `/media/` ne passent pas par ce serveur et ne portent pas les en-têtes du tableau ci-dessus.
La déclaration valable pour tout le serveur s’applique toujours à l’ensemble via `/robots.txt` ; pour ajouter aussi les en-têtes, ajoutez ce qui suit à `/media/*` dans les règles d’en-têtes de réponse du CDN (un exemple pour `ai_usage = deny`).

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

Lorsque vous modifiez les paramètres, adaptez les règles du CDN en conséquence. `/media/proxy/` est servi par ce serveur, il ne nécessite donc aucune règle.

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## La liste des robots d’IA

La liste est générée à partir de `robots.json` de [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) et intégrée au binaire. Elle n’est pas récupérée à l’exécution : une liste plus récente arrive donc avec une mise à niveau.

<!-- source: verify 1633f03abedb -->
## Vérifier

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- Sur un serveur sans paramètres, `/robots.txt` contient `User-agent: GPTBot`, `User-agent: __usage_ai-training` et `Content-Usage: search=n, train-ai=n, ai-use=n`,
  et les profils portent `X-Robots-Tag: noindex, noarchive, noai, noimageai`.
- Avec `search_indexing = allow` et `ai_usage = allow`, le profil d’un compte qui a choisi `indexable` ne porte aucun `X-Robots-Tag`, et `/.well-known/tdmrep.json` renvoie 404.
