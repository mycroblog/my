<!-- source: instance-settings 4eb467f0dbca -->
# Paramètres de l’instance

Les paramètres qui définissent la façon dont votre instance est exploitée : mode d’inscription, longueur des publications,
langue par défaut, qui peut voir les fils, nom de l’instance, description, conditions, règles, thème et émojis personnalisés.

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## Configuration et paramètres de l’instance

| | Configuration | Paramètres de l’instance |
| --- | --- | --- |
| Quoi | Comment le déploiement est construit : rôles, adaptateurs, points de terminaison, noms d’hôte, secrets | Comment l’instance est exploitée : mode d’inscription, limite de longueur des publications, langue par défaut, qui peut voir les fils, nom de l’instance, description et conditions |
| Où | Variables d’environnement `MY_*` et fichier de configuration ([référence de configuration](../reference/configuration.md)) | La base de données. Chaque modification est conservée comme une révision |
| Modification | Prend effet au redémarrage du processus | Prend effet immédiatement avec `my instance set`, les écrans d’administration du client web ou l’API client ([écrans d’administration et API client](#admin-screens-and-the-client-api)) |
| Si non défini | La valeur par défaut du schéma (le serveur ne démarre pas si la clé est obligatoire) | La valeur par défaut définie dans le code |

- **Aucune clé n’est dans les deux.** Aucune ne remplace l’autre. Vous ne pouvez pas donner un paramètre d’instance par une variable d’environnement ou
  le fichier de configuration, et vous ne pouvez pas stocker une valeur de configuration dans la base de données.
- Une valeur fixée par déploiement et qui change la structure du processus relève de la configuration ; une valeur qui change pendant que
  l’instance fonctionne est un paramètre d’instance.

<!-- source: prerequisites 12391490eea0 -->
## Prérequis

- La configuration est fournie comme décrit dans la [référence de configuration](../reference/configuration.md) (`my config check`).
- Le schéma est appliqué à la base de données (`my schema apply` ; voir [Base de données](database.md)).

<!-- source: keys ae25d36239c2 -->
## Clés

<!-- source: settings-one-key-one-value 3a7617a2d2aa -->
### Paramètres (une clé, une valeur)

| Clé | Type | Valeur par défaut | Description |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | Le mode d’inscription ([inscription](registration.md)). Tant qu’une fermeture du service est annoncée et après sa fermeture, et sur un [serveur d’une seule personne](#one-person-server), la valeur effective est `closed`. La valeur enregistrée ne change pas, et annuler l’annonce ou désactiver `single_user` la rétablit. `GET /instance` et NodeInfo montrent la valeur effective |
| `max_status_length` | entier 1-100000 | `500` | La longueur maximale d’une publication, en caractères |
| `default_locale` | balise de langue BCP 47 | `en` | La langue par défaut. C’est la langue initiale de l’interface des nouveaux utilisateurs, et le repli lorsqu’aucun catalogue ne correspond à la préférence d’un utilisateur |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | Qui peut voir le fil local (`GET /api/v1/timelines/local`) : personne, les utilisateurs connectés, ou tout le monde. `off` répond 404 et `signed_in` répond 401 aux visiteurs qui ne sont pas connectés ; dans les deux cas la navigation le masque. Sauf si la valeur est `everyone`, la page d’accueil n’affiche aucune publication d’exemple. Sur un [serveur d’une seule personne](#one-person-server), il se lit comme `off` |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | Qui peut voir le fil fédéré (`GET /api/v1/timelines/federated`). Les valeurs ont le même sens que pour le fil local, et il se lit aussi comme `off` sur un serveur d’une seule personne |
| `preview_image` | ID de média (vide = non défini) | vide | L’image d’aperçu de lien (`og:image`) pour les pages sans image propre, sous « Aperçus de liens » dans Paramètres → Général. Lorsqu’elle n’est pas définie, la miniature, puis le logo, puis l’image fournie est utilisée ([aperçus de liens](link-previews.md)) |
| `contact_account` | nom d’utilisateur d’un compte local (vide = non défini) | vide | Le compte à contacter pour joindre les exploitants. Voir [contact et miniature](#contact-and-thumbnail) |
| `contact_email` | adresse e-mail (vide = non défini) | vide | L’adresse e-mail à contacter pour joindre les exploitants. C’est aussi le `Reply-To` et la signature des e-mails que le serveur envoie |
| `thumbnail` | ID de média (vide = non défini) | vide | Une image qui représente l’instance (miniature), sur les pages À propos et d’accueil, et dans les aperçus de liens lorsque `preview_image` n’est pas définie |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | S’il faut conserver des copies des médias distants. `none` : le proxy de médias relaie chaque requête. `avatars` : les avatars, les en-têtes et les émojis personnalisés sont mis en cache dans le stockage de blobs pendant `remote_media_cache_ttl_seconds`. `all` : tous les médias distants le sont. La taille totale du cache est limitée par le réglage de configuration `remote_media_cache_max_bytes` |
| `remote_media_cache_ttl_seconds` | entier 60-31536000 (secondes) | `86400` | Combien de temps les médias distants récupérés peuvent être conservés. C’est aussi le `Cache-Control: max-age` des réponses du proxy de médias ; les CDN et les navigateurs mettent donc en cache pendant cette durée |
| `remote_media_cache_max_bytes` | entier à partir de 1 (octets) | `1073741824` | La limite de taille des copies de médias distants que conserve le proxy de médias. Le nettoyage horaire supprime les copies expirées, puis les moins récemment consultées au-delà de cette limite |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch (mode sécurisé). Lorsque `true`, un GET ActivityPub sans signature HTTP vérifiable reçoit `401` pour les objets et les collections, et un document minimal qui porte la clé pour l’acteur d’un utilisateur. L’acteur de l’instance renvoie toujours son document complet, de sorte que tout serveur peut récupérer la clé pour vérifier une signature |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | Le sens de la fédération. `off` : aucune fédération. `inbound` : vous suivez et lisez des comptes distants, mais les publications locales, les profils et les réactions ne sont pas envoyés, et les abonnements venant de l’extérieur sont refusés (`Reject`). `outbound` : vous acceptez les abonnements venant de l’extérieur et envoyez les publications locales, mais les publications et les réactions distantes ne sont pas prises en compte. `both` : envoyer et recevoir. Un changement prend effet immédiatement et exige la permission `manage_settings` |
| `federation_scope` | `open` / `allowlist` | `open` | Avec quels serveurs ce serveur se fédère. `open` : chaque serveur qu’aucune mesure sur les domaines ne suspend. `allowlist` : uniquement les domaines de la liste d’autorisation et leurs sous-domaines ; tout autre serveur est traité comme suspendu, et les GET ActivityPub exigent une signature d’un serveur autorisé quoi que dise `authorized_fetch`. Basculer vers `allowlist` met fin définitivement aux abonnements avec les serveurs qu’elle coupe ; voir [ne fédérer qu’avec des serveurs autorisés](moderation.md#federating-only-with-allowed-servers). Un changement prend effet immédiatement et exige la permission `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | Enregistrement dynamique de clients (RFC 7591). Tant que `true`, `POST /oauth/register` accepte l’enregistrement de clients publics sans authentification. Lorsque `false`, le point de terminaison refuse chaque requête avec `403` (`access_denied`) ; les clients déjà enregistrés continuent de fonctionner. Une tâche périodique supprime les clients enregistrés dynamiquement qui ne sont pas utilisés, selon les réglages de configuration `dynamic_client_unredeemed_retention` et `dynamic_client_idle_retention`. Un changement prend effet immédiatement et exige `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | Qui peut voir la liste des domaines visés par des mesures (`GET /api/v1/instance/domain_actions`) : personne, les utilisateurs connectés, ou tout le monde ([modération](moderation.md#publishing-the-domains-you-act-on)) |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | Qui peut voir les commentaires publics de cette liste. Une personne qui ne peut pas voir la liste ne les voit jamais |
| `appeal_window_days` | entier 20-365 (jours) | `180` | Combien de jours après une mesure la personne concernée peut contester ([modération](moderation.md#appeals)). Le changer modifie aussi l’échéance des mesures déjà prises |
| `session_origins_retention_days` | entier 90-3650 (jours) | `180` | Combien de jours après une connexion conserver l’adresse IP et le User-Agent de la connexion ([modération](moderation.md#record-retention-and-holds)) |
| `registration_origins_retention_days` | entier 90-3650 (jours) | `180` | Combien de jours après l’inscription conserver l’adresse IP de l’inscription |
| `reports_retention_days` | entier 180-3650 (jours) | `365` | Combien de jours après la dernière résolution conserver un signalement. Un signalement auquel une mesure fait référence est conservé aussi longtemps que la mesure |
| `moderation_actions_retention_days` | entier 180-3650 (jours) | `365` | Combien de jours après la fin d’une mesure (la levée, l’expiration ou l’examen d’une contestation, selon ce qui arrive en dernier) conserver les mesures contre les comptes et les contestations |
| `audit_log_retention_days` | entier 365-3650 (jours) | `730` | Combien de jours après son enregistrement conserver une ligne du journal d’audit |
| `remote_status_retention_days` | entier 1-3650 (jours) | `90` | Combien de jours une publication distante est conservée après sa publication lorsque rien sur cette instance n’y fait référence (une réponse locale, un boost, un favori, un marque-page, une notification, ou une discussion avec une publication locale) |
| `remote_status_max_retention_days` | entier 0-3650 (jours) | `0` | Le nombre maximal de jours pendant lesquels n’importe quelle publication distante est conservée après sa publication, même lorsque cette instance y fait référence. Les réponses locales, les boosts, les favoris et les marque-pages restent et l’affichent comme supprimée. `0` ne fixe aucune limite |
| `relay_status_retention_days` | entier 1-3650 (jours) | `7` | Combien de jours est conservée une publication distante que seul un relais a apportée, lorsque rien sur cette instance n’y fait référence et qu’aucun compte local ne suit son auteur |
| `oauth_client_rate_limits` | paires `client_id=requests` séparées par des virgules | vide | Plafonds sur les requêtes de l’API cliente des clients OAuth, pour retenir un client qui surcharge le serveur : les requêtes que tous ses utilisateurs ensemble peuvent faire en 5 minutes. Un changement met jusqu’à une minute à s’appliquer |
| `search_indexing` | `allow` / `deny` | `deny` | Si les moteurs de recherche peuvent indexer l’instance. Même avec `allow`, seuls les profils des comptes `indexable` et les publications `public` sont indexés ([moteurs de recherche et robots d’IA](crawlers.md)) |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | La déclaration aux services d’IA. `deny_training` refuse l’entraînement ; `deny` refuse l’entraînement et l’usage par l’IA (recherche par IA, assistants, agents) |
| `indexable_default` | `true` / `false` | `false` | La valeur par défaut de `indexable` pour les nouveaux comptes. La changer ne modifie pas les comptes existants |
| `auto_delete_default_enabled` | `true` / `false` | `false` | Si les nouveaux comptes démarrent avec la suppression automatique de leurs anciennes publications activée. Avec les autres clés `auto_delete_default_*`, c’est la politique que reçoit un nouveau compte ([suppression automatique des anciennes publications](#automatic-deletion-of-old-posts)) |
| `auto_delete_default_min_age_days` | entier 1-3650 (jours) | `30` | Combien de jours après sa publication la politique d’un nouveau compte supprime une publication |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | Si la politique d’un nouveau compte conserve les publications que son auteur a épinglées, mises en marque-page ou en favori |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | Si la politique d’un nouveau compte conserve les publications avec des médias ou un sondage |
| `auto_delete_default_keep_articles` | `true` / `false` | `true` | Si la politique d’un nouveau compte conserve les [articles](#articles) |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | Si la politique d’un nouveau compte conserve les publications de cette visibilité |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | Si la politique d’un nouveau compte conserve les publications directes |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | entier 0-1000000 | `0` | La politique d’un nouveau compte conserve une publication boostée ou mise en favori au moins ce nombre de fois. `0` ne fixe aucun seuil |
| `auto_delete_max_age_days` | entier 0-3650 (jours) | `0` | Le nombre maximal de jours après sa publication pendant lesquels n’importe quelle publication locale est conservée ([maximum](#a-maximum-for-keeping-posts)). `0` ne fixe aucun maximum |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | Si les publications épinglées peuvent encore être conservées sous le maximum |
| `auto_delete_max_kept_pins` | entier 1-5 | `5` | Combien d’épingles conservent des publications sous le maximum, les plus récemment épinglées en premier. `5`, le plus grand nombre d’épingles d’un compte, conserve chaque publication épinglée |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | Si les publications que l’auteur a mises en marque-page ou en favori, ou avec des médias ou un sondage, peuvent encore être conservées sous le maximum |
| `auto_delete_max_allow_articles` | `true` / `false` | `true` | Si les articles peuvent encore être conservés sous le maximum |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | Si les publications peuvent encore être conservées selon leur visibilité sous le maximum |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | Si les publications peuvent encore être conservées selon leurs boosts et leurs favoris sous le maximum |
| `auto_delete_grace_days` | entier 0-365 (jours) | `30` | Combien de jours un compte auquel le maximum s’applique nouvellement conserve ses propres réglages après en avoir été informé. Un changement ne déplace pas les dates des comptes déjà informés |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | Quelles publications couvre la recherche plein texte d’un serveur doté d’un moteur de recherche. Les publications `unlisted`, `private` et `direct` ne sont jamais couvertes, et seuls les comptes `indexable` le sont. Retirer une visibilité retire ses publications de l’index. En ajouter une ne trouve que ses nouvelles publications, jusqu’à ce que vous [reconstruisiez l’index](search.md#changing-search_visibilities). Sans effet avec `search = none` |
| `security_contact` | URI `https:` ou `mailto:` (vide = non défini) | vide | Où signaler les problèmes de sécurité de ce serveur : sa configuration et son exploitation, ses comptes et ses données. Lorsqu’il est défini, le serveur sert `/.well-known/security.txt` (RFC 9116). C’est votre contact, pas celui du logiciel : transmettez au projet un signalement de vulnérabilité du logiciel lui-même comme le décrit sa [politique de sécurité](../../SECURITY.md) |
| `require_media_description` | `true` / `false` | `false` | Si chaque fichier média (image, vidéo ou audio) joint à une publication locale exige une description. Lorsque `true`, une publication ou une modification avec un fichier dont la description est absente ou ne contient que des espaces est rejetée avec 422. Il est publié comme `limits.require_media_description` dans `GET /api/v1/instance`, et le client web rend le champ de description obligatoire. Il est distinct du paramètre par utilisateur `require_media_description`, qui ne vérifie qu’avant l’envoi |
| `link_cards` | `true` / `false` | `true` | S’il faut afficher des cartes pour les liens dans les publications. Lorsque `true`, le serveur récupère la page liée pour construire une carte. Lorsque `false`, il cesse de récupérer et masque aussi les cartes déjà construites. Pour ne l’arrêter que sur certains domaines, voir [domaines des cartes de liens](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | Si les récupérations de cartes de liens ignorent `robots.txt`. Lorsque `false`, le serveur obéit au `robots.txt` des sites liés ; un site qui interdit `my-linkcard` ou `*` n’obtient donc aucune carte ([aperçus de liens](link-previews.md#cards-of-links-in-posts)). `/about/crawler` indique aux sites liés de quel robot il s’agit. Lorsque `true`, le serveur ne récupère ni n’utilise `robots.txt`, et un site lié ne peut plus refuser ces récupérations avec lui ; le serveur continue de se nommer `my-linkcard`. Activez-le uniquement lorsque trop de cartes manquent. Les cartes construites pendant qu’il était activé sont vérifiées à leur prochaine récupération une fois qu’il est désactivé. Les directives d’une page et les domaines des cartes de liens s’appliquent dans les deux cas |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | S’il faut accepter les clients Client ID Metadata Document. Lorsque `true`, pour une demande d’autorisation dont le `client_id` est une URL `https`, le serveur récupère les métadonnées à cette URL et les traite comme le client (sans enregistrement préalable). La récupération ne suit aucune redirection, lit au plus 5 Ko et réutilise la réponse au plus 24 heures selon ses directives de cache. Lorsque `false`, il cesse de récupérer et refuse les demandes d’autorisation, les échanges de jetons et les actualisations de ces clients |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | Quelle part des coûts d’exploitation (enregistrés sur `/admin/finance/costs`, et définis sur `/admin/finance/publication`) publier. `private` : rien n’est publié (`GET /api/v1/instance/cost_totals` renvoie 404). `totals` : uniquement le total par période et par devise. `breakdown` : les totaux et une répartition par fournisseur et par catégorie, montrés à tout le monde sur les pages d’informations (`/about` et `/about/costs`) et dans l’API. `breakdown` révèle sur quel fournisseur (par exemple, quel cloud) vous fonctionnez. À chaque niveau, les noms des éléments, les mémos et les paiements individuels restent privés. Les périodes jusqu’à celle qui contient aujourd’hui (UTC) incluse sont publiées |
| `finance_viewer_item_names` | `true` / `false` | `false` | Si les utilisateurs ayant la permission `view_finance` voient les noms des éléments de coût et les noms de services des coûts importés sur l’écran des finances (`/finance`, `GET /api/v1/finance/cost_totals`). Les totaux et la répartition par fournisseur et par catégorie sont toujours montrés, quel que soit ce réglage |
| `finance_viewer_item_memos` | `true` / `false` | `false` | Si les utilisateurs ayant `view_finance` voient les mémos des éléments de coût sur l’écran des finances. Si le réglage des noms ou celui des mémos vaut `true`, chaque période affiche des lignes d’éléments (fournisseur, catégorie et montant de cette période, plus le nom et le mémo lorsqu’ils sont activés). Les personnes ayant `manage_finance` voient tout quoi qu’il en soit |
| `finance_viewer_income` | `totals` / `plans` | `totals` | Si les utilisateurs ayant `view_finance` ne voient que les totaux des revenus (`totals`) ou aussi le nombre de souscripteurs par formule (`plans`). Le montant par formule et qui a payé ne sont jamais montrés. Si `income_visibility` vaut `plans`, les nombres publiés sont montrés quelle que soit cette valeur. Les revenus se lisent sur l’écran des finances du client web (`/finance`) et dans `GET /api/v1/finance/income_totals` |
| `income_visibility` | `private` / `totals` / `plans` | `private` | Quelle part des revenus de soutien (le montant après les frais du prestataire de paiement) publier. Elle se choisit séparément de `finance_visibility`. `private` : rien n’est publié (`GET /api/v1/instance/income_totals` renvoie 404). `totals` : uniquement le total des revenus par période et par devise. `plans` : aussi le nombre actuel de souscripteurs par formule, montré à tout le monde. Pour une formule comptant peu de souscripteurs, le nombre peut permettre de deviner qui souscrit. Le solde (revenus moins coûts) n’est montré que lorsque ni cette valeur ni `finance_visibility` n’est `private` ; sinon le solde révélerait le côté que vous avez masqué. À chaque niveau, les paiements individuels, les souscripteurs, le montant par formule, ainsi que les montants bruts et les frais restent privés. Les périodes jusqu’à celle qui contient aujourd’hui (UTC) incluse sont publiées |
| `funding_meter_visibility` | `off` / `percent` / `amounts` | `off` | Ce que publie la jauge de financement : jusqu’où les revenus de ce mois (après les frais du prestataire de paiement) vont vers l’objectif mensuel, montrée sur la page À propos, sur la liste des soutiens (`/about/supporters`) et dans `GET /api/v1/instance` (`funding_meter`). Elle se définit avec l’objectif sur `/admin/finance/publication`, qui désactive les choix qu’excluent les visibilités actuelles et en explique la raison. `off` : rien. `percent` : uniquement le pourcentage, arrondi à la baisse à 1 %, si le mois est couvert, et si des coûts estimés sont dans l’objectif. Il ne peut être choisi que tant que `finance_visibility` et `income_visibility` valent tous deux `private`. `amounts` : aussi les revenus et l’objectif. Il ne peut être choisi que tant qu’aucun des deux n’est `private`. Tant qu’un seul des deux est publié, la jauge ne peut pas être montrée : les montants publiés et le pourcentage révéleraient le côté masqué. Changer l’une ou l’autre visibilité de sorte que la valeur choisie ne soit plus autorisée met la jauge sur `off`, et ce changement est enregistré et audité comme n’importe quel autre. Le mois est le mois calendaire contenant aujourd’hui (UTC), et seuls les revenus dans la devise de l’objectif comptent |
| `funding_meter_target_amount` | décimal supérieur à zéro, ou vide | vide | L’objectif mensuel de la jauge de financement, dans l’unité principale de `funding_meter_target_currency` (`1650` yens, `12.50` dollars), avec au plus autant de décimales que la devise. Vide : l’objectif est les coûts de ce mois, ce qui les exige dans une seule devise ; dans un mois dont les coûts sont en plusieurs devises, ou nuls, la jauge n’est pas montrée |
| `funding_meter_target_currency` | code ISO 4217, ou vide | vide | La devise de `funding_meter_target_amount`, comme `JPY`. Vide : la devise des coûts de ce mois |
| `funding_meter_rail` | `true` / `false` | `false` | Si le client web montre aussi la jauge de financement dans la colonne de droite de la mise en page large, en plus de la page À propos, tant que `funding_meter_visibility` n’est pas `off` |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | Qui peut voir la liste des soutiens (`GET /api/v1/instance/supporters` et `/about/supporters`). `off` : personne (l’API répond 404 et `/about` n’a aucun lien). `signed_in` : uniquement les personnes connectées à ce serveur (l’API répond 401 sinon). `everyone` : tout le monde. La liste ne montre que les comptes et les formules des souscripteurs actifs qui ont choisi d’être listés ; les montants, la facturation mensuelle ou annuelle, les dates et les nombres ne sont jamais montrés. La page de la liste et la réponse de l’API sont toujours `noindex, noarchive`, quel que soit `search_indexing` |
| `user_app_registration` | `true` / `false` | `true` | Si les utilisateurs connectés peuvent enregistrer leurs propres applications OAuth (publiques ou confidentielles) avec l’API cliente (`POST /api/v1/oauth_apps`). Lorsque `false`, l’enregistrement répond 403. Lister, modifier et supprimer des applications enregistrées, réémettre des secrets, s’émettre un jeton à soi-même et l’autorisation par d’autres utilisateurs continuent de fonctionner |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | Qui peut ouvrir des salons de chat vocal, tant que le réglage de configuration `voice` n’est pas `off` ([configuration](../reference/configuration.md#voice)). `everyone` : chaque utilisateur local connecté qui n’est ni suspendu ni limité. `permitted` : uniquement les détenteurs de la permission `start_voice_rooms`, que seul le rôle intégré `admin` possède par défaut et que le rôle d’une formule de soutien peut avoir |
| `voice_public_summary` | `true` / `false` | `false` | Si la page d’un salon de chat vocal dont la visibilité est `local` montre son nom, sa description, son hôte et s’il est encore ouvert aux visiteurs qui ne sont pas connectés à ce serveur et aux utilisateurs distants, ainsi que dans son aperçu de lien. Les participants ne sont jamais montrés. Lorsque `false`, ces visiteurs voient seulement que le salon est destiné aux utilisateurs locaux de ce serveur |
| `event_creators` | `everyone` / `permitted` | `everyone` | Qui peut créer des événements ([événements](#events)). `everyone` : chaque utilisateur local connecté qui n’est ni suspendu ni limité. `permitted` : uniquement les détenteurs de la permission `create_events`, que seul le rôle intégré `admin` possède par défaut et que le rôle d’une formule de soutien peut avoir |
| `public_counts` | `true` / `false` | `true` | Si la page d’accueil (`/`, non connecté) et `GET /api/v1/instance/overview` montrent combien de membres compte le serveur, combien de publications publiques ses utilisateurs ont faites chacun des 7 derniers jours, et quand le serveur a démarré. Seules les publications publiques des utilisateurs de ce serveur comptent ; les nombres sont actualisés toutes les quelques minutes. Lorsque `false`, l’API les omet |
| `featured_accounts` | noms d’utilisateur d’au plus 20 comptes locaux, séparés par des espaces ou des virgules (vide = aucun) | vide | Les comptes que les suggestions de personnes à suivre montrent en premier, dans cet ordre, avant les personnes de ce serveur qui ont publié récemment. Un compte dont le propriétaire a désactivé « Suggérer mon compte et permettre de le trouver par une partie de son nom » n’est pas suggéré. Voir [suggestions](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | Si l’inscription et la connexion sont limitées aux fournisseurs de connexion tant qu’au moins un est activé : les mots de passe et les clés d’accès ne permettent plus de se connecter, et les utilisateurs sans fournisseur lié ne peuvent plus se connecter une fois leurs sessions terminées. Sans fournisseur activé, il ne s’applique pas. `my instance set` affiche combien d’utilisateurs actifs n’ont aucun lien avec un fournisseur activé. Voir [limiter la connexion aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers). |
| `invite_creators` | `everyone` / `permitted` | `permitted` | Qui peut créer ses propres invitations en plus du personnel ayant `manage_invites` ([invitations des membres](registration.md#member-invites)). `everyone` : chaque utilisateur local connecté qui n’est ni suspendu ni limité. `permitted` : uniquement les détenteurs de la permission `create_invites`, que seul le rôle intégré `admin` possède par défaut et que le rôle d’une formule de soutien peut avoir. En mode `approval`, une invitation utilisable approuve immédiatement le nouvel utilisateur ; `everyone` permet donc à chaque utilisateur de contourner l’examen |
| `member_invite_max_active` | entier 0-100 | `5` | Combien d’invitations utilisables (non révoquées, non expirées, non épuisées) un membre peut détenir à la fois ; `0` ne permet aux membres d’en créer aucune. Les invitations du personnel ne comptent pas |
| `member_invite_max_uses` | entier 1-1000 | `10` | Combien de personnes peuvent s’inscrire avec une invitation créée par un membre |
| `member_invite_max_lifetime_seconds` | entier 3600-31536000 (secondes) | `2592000` (30 jours) | Combien de temps au plus une invitation créée par un membre peut fonctionner. L’abaisser ne raccourcit pas les invitations existantes |
| `translation_providers` | ID de fournisseurs séparés par des virgules | vide | Les fournisseurs de traduction côté serveur qui sont activés, le premier étant essayé en premier. Lorsqu’il est vide, rien n’est envoyé à aucun fournisseur. Un ID pour lequel ce serveur n’a aucun fournisseur ne fait rien. Les [fournisseurs de traduction](translation.md) intégrés ont les ID `libretranslate`, `deepl`, `google` et `aws`, et un [plugin de traduction](plugins.md#translating-posts) l’ID `plugin` |
| `translation_provider_visibilities` | paires `provider=visibility\|visibility` séparées par des virgules | vide | Ce que chaque fournisseur peut recevoir. Un fournisseur non nommé reçoit les publications `public`, `local` et `unlisted` ; `provider=` sans rien après n’en reçoit aucune. `private` et `direct` ne vont qu’à un fournisseur nommé avec elles ; en autoriser un n’autorise donc pas le suivant |
| `translation_browser` | booléen | `false` | Si les lecteurs peuvent utiliser la traduction intégrée à leur navigateur, qui n’envoie rien à ce serveur ni à un fournisseur. Lorsqu’un fournisseur côté serveur est aussi activé, le fournisseur passe en premier, et le navigateur traduit ce que le serveur refuse : une publication ou une langue qu’aucun fournisseur ne prend, une limite mensuelle, ou un fournisseur qui échoue |
| `translation_user_monthly_characters` | entier 0-1000000000000 | `50000` | Combien de caractères les requêtes d’un utilisateur peuvent envoyer aux fournisseurs de traduction au cours d’un mois UTC. À `0`, rien n’est envoyé. Il n’existe pas de valeur illimitée : définissez un grand nombre |
| `translation_server_monthly_characters` | entier 0-1000000000000 | `500000` | Combien de caractères l’ensemble du serveur peut envoyer aux fournisseurs de traduction au cours d’un mois UTC. À `0`, rien n’est envoyé. Il n’existe pas de valeur illimitée : définissez un grand nombre |
| `translation_retention_days` | entier 1-3650 (jours) | `30` | Combien de jours une traduction est conservée après avoir été faite. La lire ne prolonge pas cette durée, et elle disparaît avec la publication |
| `translation_reports_to_server` | booléen | `false` | Si les modérateurs peuvent envoyer le texte d’un signalement, les publications signalées et le commentaire de l’auteur du signalement, aux fournisseurs côté serveur. Chaque fournisseur ne reçoit toujours que les visibilités que `translation_provider_visibilities` lui autorise, quelle que soit la personne qui demande : une publication directe signalée ne va qu’à un fournisseur autorisé pour `direct`, et le commentaire compte comme `private`. La traduction du commentaire n’est jamais conservée, pas plus que celle d’une publication supprimée, et les caractères ne comptent que dans `translation_server_monthly_characters`. Lorsque `false`, rien d’un signalement n’atteint un fournisseur |
| `event_records_retention_days` | entier 30-3650 (jours) | `90` | Combien de jours après la fin ou l’annulation d’un événement conserver qui y a répondu, qui a été approuvé, qui a été invité et qui a voté sur ses dates candidates. Un événement dont la date n’a jamais été fixée se termine avec sa dernière date candidate. L’événement lui-même (ses réglages, lieux, date et dates candidates, et annulation) est conservé. Les enregistrements d’un événement signalé sont conservés aussi longtemps que le signalement, et les conservations obligatoires s’appliquent comme pour les autres enregistrements ([conservation des enregistrements](moderation.md#record-retention-and-holds)) |
| `voice_participants_retention_days` | entier 30-3650 (jours) | `90` | Combien de jours après la fin d’un salon de chat vocal conserver les enregistrements de qui y a participé : les participations, les intervenants et auditeurs, les mains levées, les invitations et les retraits. Le salon lui-même (son nom, sa description, ses hôtes et la manière dont il s’est terminé) est conservé. Les enregistrements d’un salon signalé sont conservés aussi longtemps que le signalement, et les conservations obligatoires s’appliquent comme pour les autres enregistrements |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | Jusqu’où chaque [hook de décision](decision-hooks.md#modes) agit tant qu’il a un plugin. `off` ne passe à `enforce` que par `observe`. |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | Ce que fait le hook lorsqu’il applique et que son plugin échoue ([détails](decision-hooks.md#when-the-plugin-fails)). |
| `hook_inbound_status_unfollowed_direct` | booléen | `false` | Si `inbound.status` envoie aussi les publications directes d’auteurs qu’aucun compte local ne suit ([détails](decision-hooks.md#what-inboundstatus-reviews)). La page d’informations (`/about`) l’indique tant que le hook est utilisé. |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | Ce que fait `signup.check` lorsqu’il applique et que son plugin échoue. |
| `hook_signup_check_reject` | booléen | `false` | Si un `reject` de `signup.check` [refuse l’inscription](decision-hooks.md#sign-ups) pendant qu’il applique. Lorsqu’il est désactivé, la réponse est seulement enregistrée, comme dans `observe`. |
| `hook_local_status_account_age_days` | entier 0-3650 (jours) | `7` | `local.status` [examine les publications et leurs modifications](decision-hooks.md#local-posts) des comptes plus jeunes que cette durée ; `0` examine chaque compte. La page d’informations (`/about`) l’indique tant que le hook est utilisé. |
| `hook_local_profile_account_age_days` | entier 0-3650 (jours) | `7` | `local.profile` [examine les modifications de profil](decision-hooks.md#local-profiles) des comptes plus jeunes que cette durée ; `0` examine chaque compte. La page d’informations (`/about`) l’indique tant que le hook est utilisé. |
| `hook_decisions_retention_days` | entier 30-3650 (jours) | `90` | Combien de jours conserver les [décisions des hooks](decision-hooks.md#the-decisions). |
| `single_user` | booléen | `false` | Si le serveur est un [serveur d’une seule personne](#one-person-server). Il ne peut être activé que tant que le serveur a exactement un compte local, et désactivé à tout moment |
| `max_drafts` | entier 1-10000 | `100` | Combien de brouillons non programmés un utilisateur peut conserver sur le serveur. L’abaisser ne supprime aucun brouillon : un utilisateur au-dessus n’en enregistre plus de nouveau |
| `max_scheduled_posts` | entier 1-10000 | `300` | Combien de publications programmées d’un utilisateur peuvent attendre. L’abaisser n’annule aucune programmation |
| `max_scheduled_posts_per_day` | entier 1-10000 | `25` | Combien de publications programmées d’un utilisateur peuvent attendre pour un jour UTC |
| `scheduled_post_min_lead_minutes` | entier 1-1440 (minutes) | `5` | Combien de minutes à l’avance, au minimum, une publication peut être programmée |
| `scheduled_post_max_lead_days` | entier 1-3660 (jours) | `366` | Combien de jours à l’avance, au maximum, une publication peut être programmée |
| `article_authoring` | `everyone` / `permission` | `everyone` | Qui peut écrire de nouveaux [articles](#articles) : chaque utilisateur local, ou seulement les utilisateurs dont les rôles ont la permission `create_articles` |
| `max_article_bytes` | entier 1000-100000 (octets) | `100000` | Le nombre maximal d’octets du HTML que produit le corps d’un article |
| `max_article_images` | entier 0-100 | `20` | Le nombre maximal d’images des propres téléversements de l’auteur dans le corps d’un article, en plus de sa couverture |

- Les valeurs sont normalisées avant d’être enregistrées (`TRUE` devient `true`, `0750` devient `750`, `ja_jp` devient `ja-JP`, les ID de médias sont mis en minuscules).
  Une valeur invalide est rejetée et rien n’est écrit.
- `preview_image` et `thumbnail` n’acceptent que le média d’une image qui existe (y compris une image encore en cours de traitement).
  Tant que le traitement n’est pas terminé, et à la place d’une image dont le traitement a échoué, `preview_image` se rabat sur l’image par défaut.
  Dès qu’un média a été défini dans une révision quelconque, le nettoyage des téléversements non rattachés ne le supprime pas.
- `contact_account` n’accepte que le nom d’utilisateur d’un utilisateur local non supprimé (enregistré en minuscules). `contact_email`
  n’accepte qu’une adresse de la forme `alice@example.com`.
- `featured_accounts` n’accepte que les noms d’utilisateur d’utilisateurs locaux non supprimés ; un `@` initial est retiré, les noms
  sont mis en minuscules, un nom répété n’est conservé qu’une fois, et ils sont enregistrés séparés par des espaces simples.

<!-- source: suggestions-of-accounts b7d362466e33 -->
### Suggestions de comptes

Les utilisateurs connectés voient des suggestions de personnes à suivre (sur le fil d’accueil vide après l’inscription, et avec
`GET /api/v1/accounts/suggestions`). Les suggestions sont les comptes de `featured_accounts` dans leur ordre, puis les
comptes de ce serveur qui ont publié une publication publique ou locale au cours des 30 derniers jours, celui qui a publié en dernier en premier. Le
compte de l’utilisateur lui-même, les comptes que l’utilisateur suit, bloque ou met en sourdine, ou par lesquels il est bloqué, ainsi que les comptes suspendus ou
désactivés sont omis.

Les nouveaux comptes sont suggérés et peuvent être trouvés par une partie de leur nom dans la recherche de comptes. Un utilisateur qui désactive « Suggérer mon
compte et permettre de le trouver par une partie de son nom » dans les paramètres du profil sort des deux : la recherche ne trouve alors le compte que par son
nom d’utilisateur complet, ou pour les personnes qui le suivent.

<!-- source: automatic-deletion-of-old-posts 9eb8a72df28e -->
### Suppression automatique des anciennes publications

Les utilisateurs peuvent faire supprimer automatiquement leurs propres publications quelques jours après les avoir publiées (Paramètres, « Supprimer les anciennes publications »).
La politique de chaque utilisateur lui est propre : si elle est activée, après combien de jours, et quelles publications elle conserve (les publications épinglées, mises en marque-page ou en favori, les publications avec des médias ou un sondage, les articles, les publications de certaines visibilités, et les publications comptant au moins un certain nombre de boosts ou de favoris). Supprimer une publication de cette manière revient à ce que son auteur la supprime : les serveurs auxquels elle a été envoyée reçoivent un `Delete`.
Seules les publications des comptes locaux sont supprimées de cette manière, jamais les boosts.

Les clés `auto_delete_default_*` sont la politique avec laquelle chaque nouveau compte local démarre, quelle que soit la façon dont il est créé (le formulaire d’inscription, un fournisseur de connexion externe, une invitation ou `my admin`). Le compte la reçoit à sa création et en est ensuite propriétaire : modifier les clés plus tard ne change ni les comptes existants ni ceux qui ont reçu une valeur antérieure.
L’écran des paramètres montre les valeurs comme étant celles de l’utilisateur. Avec les clés non définies, un nouveau compte démarre avec la suppression automatique désactivée, après 30 jours, en conservant les publications épinglées, mises en marque-page ou en favori, les articles et les publications directes. La valeur par défaut n’est pas montrée aux personnes extérieures à l’instance. Lorsqu’un maximum est défini, un nouveau compte reçoit la valeur par défaut ramenée au maximum, comme décrit ci-dessous, de sorte qu’il démarre dans cette limite.

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### Un maximum de conservation des publications

Avec `auto_delete_max_age_days` supérieur à `0`, aucune publication locale n’est conservée plus de ce nombre de jours après sa publication.
Pour chaque compte local sans la permission `keep_posts_longer` :

- La suppression automatique est toujours activée : un utilisateur ne peut pas la désactiver, et ne peut pas choisir plus de jours que le maximum (moins, c’est
  possible). L’écran des paramètres et `PUT /api/v1/preferences/auto_delete` refusent les deux.
- Seules les exceptions que les clés `auto_delete_max_allow_*` autorisent conservent des publications. Par défaut, seules les publications épinglées le font, toutes ;
  avec `auto_delete_max_kept_pins` inférieur à 5, seules les épingles les plus récentes, en ce nombre, conservent leurs publications.
- Les utilisateurs qui ont désactivé la suppression automatique, ou ne l’ont jamais définie, sont aussi soumis au maximum.

Activer le maximum, l’abaisser ou modifier les exceptions ne réécrit les paramètres de personne. À la place, chaque heure, la
tâche `content.auto_delete_statuses` trouve les comptes dont les propres paramètres conservent des publications que le maximum ne conserve pas (parce que
le maximum a été activé ou abaissé, ou que le compte a perdu `keep_posts_longer`). Elle en informe chacun une seule fois, avec une
notification et un e-mail activés par défaut, et indique à partir de quand le maximum s’applique : `auto_delete_grace_days` après
l’avis. D’ici là, les propres paramètres du compte restent en vigueur ; ensuite, ses publications suivent le maximum. Un compte
qui rentre de nouveau dans le maximum (son utilisateur modifie ses paramètres, il obtient `keep_posts_longer`, ou le maximum disparaît ou
augmente) n’est plus concerné, et une modification ultérieure l’en informe de nouveau avec un nouveau délai de grâce.

Activer le maximum informe en l’espace d’une heure chaque compte qu’il concerne. Les suppressions gardent leur rythme : au plus 100 publications
d’un compte par heure, chacune envoyée comme un `Delete` aux serveurs auxquels la publication est parvenue ; un compte comptant beaucoup d’anciennes publications
met donc des heures ou des jours à se vider. Un utilisateur qui modifie ses paramètres entre-temps arrête le reste.

`keep_posts_longer` est une permission de membre : donnez-la à un rôle pour permettre à ses titulaires de conserver leurs publications aussi longtemps qu’ils le choisissent
et de désactiver la suppression automatique. Le rôle intégré `admin` l’a. Une [formule de soutien](supporters.md) peut la porter, de sorte que
conserver ses publications plus longtemps peut être un avantage de la formule ; lorsque la souscription se termine, le maximum s’applique de nouveau après le délai de
grâce.

Le maximum est public : `GET /api/v1/instance` le renvoie dans `auto_delete_limit`, et la page À propos l’affiche, afin que les gens
le connaissent avant de s’inscrire. Les modifications de ces clés exigent `manage_settings` et sont enregistrées dans le journal d’audit comme tout autre
paramètre. Dans le client web, elles se trouvent sur la page « Suppression des publications » de « Paramètres du serveur », qui demande confirmation avant d’enregistrer un
maximum qui est activé ou rendu plus strict.

<!-- source: drafts-and-scheduled-posts 240f795223ab -->
### Brouillons et publications programmées

Les utilisateurs enregistrent des brouillons de publications sur le serveur, continuent de les écrire sur un autre appareil et les publient immédiatement ou à l’heure de leur choix. Seul leur auteur lit un brouillon ; les brouillons ne sont jamais fédérés, ne figurent pas dans l’export des données d’un compte et disparaissent avec le compte lorsqu’il est effacé. Les cinq réglages `max_drafts` et `scheduled_post_*` ci-dessus les bornent ; un enregistrement ou une programmation au-delà d’une limite est refusé avec un message qui la nomme.

- Une publication programmée est publiée par la tâche `content.publish_scheduled_draft` à son heure ou peu après, y compris lorsque l’heure est plus lointaine que la limite de délai du service de file : la tâche revient et attend de nouveau. Définissez-les avec `my instance set`, par exemple `my instance set max_scheduled_posts 100`.
- La publication est vérifiée lorsqu’elle est publiée, et pas seulement lorsqu’elle est programmée : un utilisateur qu’une mesure de modération empêche de publier à ce moment-là, ou dont la publication répond à une publication disparue ou la cite, n’obtient aucune publication. Le brouillon reste, et l’utilisateur reçoit une notification qui en explique la raison (`scheduled_status_failed`, que les utilisateurs peuvent désactiver dans leurs réglages de notification, et qui n’est jamais envoyée par e-mail). L’examen automatisé des publications locales, tant qu’il est utilisé, examine la publication lorsqu’elle est publiée.
- Une publication est horodatée au moment de sa publication, et la suppression automatique des anciennes publications compte à partir de là.
- Les fichiers médias qu’un brouillon joint sont conservés tant que le brouillon l’est ; supprimer le brouillon les rend non rattachés, et ils partent avec le nettoyage des téléversements non rattachés. La couverture du brouillon d’un article et les téléversements que son corps montre sont conservés de la même façon.
- Les brouillons et les publications programmées incluent les [articles](#articles), qui sont programmés et comptés comme des publications. Le brouillon d’un article peut ne pas avoir de titre pendant que son auteur écrit ; il est vérifié comme un nouvel article lorsqu’il est publié, et un utilisateur qui n’a plus le droit d’écrire de nouveaux articles à ce moment-là n’obtient aucun article.

<!-- source: articles fbbf778b92f0 -->
### Articles

Un article est une longue publication avec un titre, un résumé et une image de couverture, dont l’auteur écrit le corps en Markdown. Les articles sont des publications à tous les autres égards : ils ont une visibilité, peuvent être modifiés avec leur historique conservé, boostés, mis en favori, cités, recevoir des réponses, être signalés et supprimés, et la modération agit sur eux comme sur les publications. Le serveur rend le corps une seule fois, lorsqu’il est écrit : le HTML brut qu’il contient n’est pas rendu, et seules les images des propres téléversements de l’auteur s’affichent comme des images ; toute autre image devient un lien, de sorte que les lecteurs ne chargent jamais d’images depuis d’autres sites. Les fils, les notifications et la recherche montrent le titre, le résumé et la couverture ; le corps s’affiche là où l’article est ouvert.

- Par défaut, chaque utilisateur local peut écrire des articles. Avec `article_authoring` défini sur `permission`, seuls les utilisateurs dont les rôles ont la permission `create_articles` peuvent en écrire de nouveaux ; les articles déjà écrits restent, et leurs auteurs peuvent toujours les modifier. Une mesure de modération qui empêche un compte de publier l’empêche aussi d’écrire et de modifier des articles. Une [formule de soutien](supporters.md) peut donner `create_articles` comme avantage.
- `max_article_bytes` borne le HTML d’un corps, pas le Markdown : les serveurs qui reçoivent des articles comptent le HTML, et 100 000 octets, le maximum possible, est ce qu’ils conservent en entier. Un corps en japonais d’environ 30 000 caractères y tient. `max_article_images` borne les images d’un corps. L’éditeur montre les deux avant la publication de l’article.
- Un article n’est jamais une publication directe, et n’a ni sondage, ni pièces jointes autres que ses images, ni avertissement de contenu : un article sensible montre son titre et son résumé et replie le corps et la couverture.
- La suppression automatique des anciennes publications conserve les articles sauf si l’utilisateur le désactive (`auto_delete_default_keep_articles` pour les nouveaux comptes, `auto_delete_max_allow_articles` sous un maximum).
- `GET /api/v1/instance` renvoie les limites dans `limits.articles` et le réglage de rédaction dans `article_authoring`. Définissez-les avec `my instance set`, par exemple `my instance set article_authoring permission`.

<!-- source: events 07dccecf6936 -->
### Événements

Les utilisateurs locaux planifient des événements avec une date, un fuseau horaire et jusqu’à trois lieux : un salon de chat vocal de ce serveur, un lieu de rendez-vous en personne et un lien. Au lieu d’une date, un organisateur peut proposer jusqu’à 20 dates candidates : les utilisateurs qui voient l’événement répondent à chacune par oui, peut-être ou non, et l’organisateur ou un co-organisateur en confirme ensuite une, ce qui fixe la date. D’ici là, personne ne répond s’il viendra. Les événements sont réservés aux utilisateurs de ce serveur : ils ne sont jamais fédérés, et les utilisateurs distants et les visiteurs qui ne sont pas connectés n’en voient aucun.

- Qui peut en créer un est défini par `event_creators`. Un organisateur doit aussi pouvoir ouvrir des salons de chat vocal (`voice_room_starters` et le réglage de configuration `voice`) pour tenir un événement dans l’un d’eux.
- Le salon de chat vocal d’un événement s’ouvre à son début, hébergé par l’organisateur, et attend jusqu’à 30 minutes le premier participant. Il prend le titre de l’événement et la visibilité de l’événement (chaque utilisateur local, les abonnés, les personnes que l’organisateur suit, les abonnés mutuels, ou les invités). Les co-organisateurs, les personnes qui ont répondu qu’elles viennent ou viendront peut-être, et les invités qui n’ont pas répondu sont invités au salon, afin qu’ils continuent de le voir lorsqu’un co-hôte le reprend. Le salon ne crée aucune publication de début, et la notification de début de l’événement est sa seule notification.
- Un salon qui ne peut pas s’ouvrir (le chat vocal est désactivé, l’organisateur ne peut plus ouvrir de salons ou en héberge un autre, ou autant de salons sont ouverts que le réglage de configuration `voice_max_open_rooms` le permet) laisse l’événement tel quel : l’organisateur est notifié, et la page de l’événement indique que le salon n’a pas pu s’ouvrir. Le serveur le vérifie lorsque la date est fixée ou modifiée et aussi un jour avant le début, et notifie l’organisateur de ce qui garderait le salon fermé. L’organisateur est aussi notifié lorsque personne n’est venu dans le salon. Annuler l’événement, ou retirer son salon de chat vocal, termine le salon et conserve l’événement.
- L’organisateur choisit qui voit l’événement : chaque utilisateur local, les abonnés de l’organisateur, les personnes que l’organisateur suit, les abonnés mutuels, ou uniquement les invités. Les invités et les personnes qui y ont répondu, ou ont répondu à ses dates candidates, le voient toujours, et un blocage entre un utilisateur et l’organisateur ou un co-organisateur le masque pour cet utilisateur.
- L’adresse et le lien d’un événement ne sont montrés qu’aux participants que l’organisateur ou un co-organisateur a approuvés, sauf si l’organisateur les montre à toutes les personnes qui voient l’événement. Les liens doivent être `http` ou `https`, et le serveur ne les ouvre jamais.
- Un utilisateur qui voit un événement peut l’enregistrer sous forme de fichier iCalendar (`.ics`) pour l’ajouter à son propre agenda. Le fichier ne nomme que les lieux que cet utilisateur voit ; une adresse réservée aux participants approuvés n’y apparaît donc jamais. Il n’existe aucune adresse d’abonnement au calendrier.
- Un utilisateur crée au plus 10 événements par heure, et un événement envoie au plus 100 invitations par heure.
- Les utilisateurs peuvent signaler un événement, et les modérateurs peuvent en annuler un ; suspendre ou supprimer un organisateur annule les événements de l’organisateur qui n’ont pas commencé ([modération](moderation.md#events)). Qui a répondu et qui a été invité est conservé pendant `event_records_retention_days` après l’événement, tout comme les votes sur ses dates candidates.

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### Textes (une clé, une valeur)

| Clé | Limite | Description |
| --- | --- | --- |
| `name` | 100 caractères, une ligne (des espaces seuls ne sont pas autorisés) | Le nom de l’instance (nom d’affichage). Voir [nom et domaine](#name-and-domain) |
| `short_description` | 500 caractères | Une courte description |
| `description` | 20000 caractères | La description |
| `terms` | 100000 caractères | Les conditions d’utilisation (Markdown) |
| `privacy_policy` | 100000 caractères | La politique de confidentialité (Markdown) |
| `commerce_disclosure` | 100000 caractères | Les mentions du vendeur pour les formules payantes, comme l’avis qu’exige la loi japonaise (Markdown ; [soutiens](supporters.md)) |
| `preview_image_alt` | 1500 caractères | Le texte alternatif de l’image `preview_image` (`og:image:alt`). Lorsqu’il n’est pas défini, « Image of {instance name} » est utilisé |
| `thumbnail_alt` | 1500 caractères | Le texte alternatif de l’image `thumbnail` |

- Chaque texte a une seule valeur. Un administrateur l’écrit une fois dans la langue qu’il souhaite, et les lecteurs de toutes les langues voient la
  même valeur ; les lecteurs d’autres langues s’appuient sur la traduction automatique ou un moyen similaire. Les libellés et les descriptions des champs d’administration
  sont traduits dans la langue de la personne qui les consulte comme n’importe quel autre texte d’écran ; seule la valeur qu’un administrateur écrit ne l’est pas.
- Le serveur ne connaît pas la langue d’un texte ; le client web et les écrans rendus par le serveur affichent donc les textes avec
  `dir="auto"` et `lang=""` (la valeur HTML pour une langue inconnue) au lieu d’hériter de la langue de l’écran.
- Définir un texte vide le supprime.
- En plus de la CLI, les textes peuvent être modifiés sur les écrans d’administration du client web (le nom, la courte description et la description sur
  la page Général de « Paramètres du serveur », `/admin/settings` ; les conditions sur la première page de « Conditions et politiques »,
  `/admin/documents`, la politique de confidentialité sur `/admin/documents/privacy-policy`, et les mentions commerciales et autres sur
  `/admin/documents/commerce-disclosure`) et avec l’API client (`GET /api/v1/admin/instance_texts`, `PUT /api/v1/admin/instance_texts/{key}`).
  Les deux exigent la permission de rôle `manage_settings`, et les modifications sont enregistrées dans le journal d’audit (`text_changed`).

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### Conditions et politique de confidentialité

- Écrivez `terms`, `privacy_policy` et `commerce_disclosure` en Markdown (CommonMark plus les tableaux, le texte barré
  et les liens automatiques de GitHub Flavored Markdown). Un saut de ligne à l’intérieur d’un paragraphe reste un saut de ligne, et les titres `#` commencent un niveau en dessous du
  titre de la page (`h2`).
- Le serveur les convertit en HTML et le nettoie (`GET /api/v1/instance/documents/{kind}` anonyme), et les pages du client web
  `/about/terms`, `/about/privacy` et `/about/commerce` les affichent. Le HTML brut, les images et les liens autres que `http`,
  `https` et `mailto` sont supprimés. Les pages ne demandent aucune connexion (ce sont des pages du client web ; elles exigent donc JavaScript).
  `/about/commerce` est l’URL à donner au fournisseur de paiement.
- Un document que vous rédigez est lié depuis l’écran d’inscription, le pied de page de chaque écran rendu par le serveur et le client web (la troisième colonne, le menu, les paramètres et l’écran de connexion).
  Un document qui n’est pas rédigé n’est pas lié, et son API répond 404.
- Tant que les conditions d’utilisation ou la politique de confidentialité sont rédigées, l’écran d’inscription les liste avant le bouton de création
  avec une case à cocher obligatoire, « J’ai lu et j’accepte les documents ci-dessus ». Le serveur refuse une inscription sans elle,
  y compris une inscription par un fournisseur de connexion. Sans l’un ni l’autre document, l’écran ne demande aucun accord.

<!-- source: name-and-domain 1e14aa3e6c58 -->
### Nom et domaine

Une instance a un domaine et un nom, séparément.

- Le **domaine** est la configuration `host` (et `account_domain`, le domaine des identifiants). Il ne peut pas être modifié pendant que l’
  instance fonctionne. Les identifiants (`@alice@example.com`), les URL, les ID ActivityPub et `domain` dans `GET /api/v1/instance` restent
  le domaine même lorsqu’il y a un nom.
- Le **nom** est le texte `name`. Il apparaît partout où l’instance est nommée aux personnes : la barre latérale et « À propos de ce serveur » du
  client web, le titre de l’onglet du navigateur, les écrans de connexion, d’inscription, de confirmation d’e-mail, d’état du compte et de consentement à l’autorisation
  rendus par le serveur, les e-mails (confirmation de l’adresse e-mail et notifications aux modérateurs), `og:site_name` des aperçus
  de liens, `name` / `short_name` du Web App Manifest, `metadata.nodeName` de NodeInfo et `name` dans `GET /api/v1/instance`.
- Le nom est le même dans toutes les langues. Lorsqu’il n’y a pas de nom, le domaine (`host`) est affiché à la place.
- Une modification prend effet à partir de la requête suivante, sans redémarrage. Le titre de l’onglet et la barre latérale d’un client web ouvert se mettent à jour lorsque
  vous enregistrez sur l’écran d’administration.

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### Contact et vignette

Le contact des opérateurs (`contact_account`, `contact_email`) et l’image qui représente l’instance (`thumbnail`
et le texte `thumbnail_alt`) apparaissent aux endroits ci-dessous. Tout ce qui n’est pas défini n’apparaît pas.

- « À propos de ce serveur » du client web (`/about`). La vignette et le compte de contact apparaissent aussi sur le premier écran (`/`)
  pour les personnes qui ne sont pas connectées.
- `contact` (`account` et `email`) et `thumbnail` (`url`, `width`, `height`, `alt`) dans `GET /api/v1/instance`.
- `metadata` dans NodeInfo : `maintainer` selon la convention de Misskey (`name` est le nom d’affichage du compte de contact, ou son
  nom d’utilisateur s’il n’en a pas ; `email` est `contact_email`), `staffAccounts` selon la convention de Pleroma et d’Akkoma (l’ID d’acteur du
  compte de contact), `thumbnail` (`url` et `alt`), et `nodeDescription` issu de la courte description `short_description`.

Lorsque le compte de contact est suspendu ou supprimé, le compte n’est pas affiché et seule l’adresse e-mail l’est.
Modifiez-les sur la page Général de l’écran d’administration du client web (« Paramètres du serveur », `/admin/settings`), ou avec la CLI ou l’API client.

<!-- source: one-person-server 6ba524c58a4f -->
### Serveur d’une seule personne

Un serveur réservé à son opérateur, le chez-soi de l’opérateur dans le fédivers, est un serveur d’une seule personne : activez `single_user`. Dans le client web,
un administrateur l’active et le désactive avec **Faire de ce serveur un serveur d’une seule personne** sur la page **Inscription** de **Paramètres du serveur**,
qui indique pourquoi le serveur a refusé lorsqu’il y a plus d’un compte local.

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- Il ne peut être activé que lorsqu’exactement un compte local compte. Chaque compte local qui n’est pas supprimé compte,
  y compris ceux en attente d’approbation, désactivés, programmés pour suppression, suspendus ou limités, puisque chacun pourrait revenir comme
  une seconde personne. Un compte dont l’inscription a été rejetée ne compte pas. Avec tout autre nombre, `my instance set`
  échoue en indiquant le nombre, et l’API d’administration répond `422` avec le type `/api/v1/problems/single-user-requires-one-account`.
  L’acteur d’instance du serveur n’est pas un compte et ne compte jamais.
- Il peut être désactivé à tout moment. L’activer ou le désactiver ne supprime rien et ne change la valeur stockée d’aucun autre paramètre :
  ce qu’il retient s’applique de nouveau tel quel une fois désactivé. Comme tout paramètre, la modification va dans le journal d’audit.

Tant qu’il est activé :

- **L’inscription est fermée.** `registration_mode` se lit comme `closed` quelle que soit sa valeur stockée ; la page d’inscription, l’inscription par un
  fournisseur de connexion et `my admin create` ne créent donc aucun compte. La connexion avec un fournisseur de connexion fonctionne toujours.
- **Seulement le fil d’accueil.** `local_timeline_visibility` et `federated_timeline_visibility` se lisent comme `off`. Les fils de hashtags et de listes
  restent.
- **Ce dont une communauté a besoin est masqué.** Les fils local et fédéré et leurs flux, les suggestions de comptes,
  les invitations des utilisateurs et des administrateurs, l’examen des inscriptions et les blocages d’inscription répondent `404` avec le type
  `/api/v1/problems/single-user-mode`.
- **Pas de nouvelles publications locales.** Sur un serveur d’une seule personne, une publication pour « Ce serveur » n’atteint que les abonnés ; une nouvelle
  publication ne peut donc pas être locale : l’API client répond `422` et l’omet de ce qu’un compte peut publier. Une visibilité par défaut
  « Ce serveur » se lit comme « Abonnés » et ne peut pas être enregistrée, et un salon de chat vocal pour les personnes de ce serveur s’annonce
  dans une publication pour les abonnés. Les publications déjà locales restent locales.
- **Le serveur dit ce qu’il est.** `single_user` dans `GET /api/v1/instance` vaut `true`, avec le mode d’inscription et les fils effectifs
  et le compte de l’opérateur comme `single_user_account`, et NodeInfo répond `openRegistrations: false` avec
  `metadata.singleUser: true`.
- **La page d’accueil est le profil de l’opérateur.** Les personnes qui ne sont pas connectées et ouvrent l’adresse du serveur voient le profil
  de son unique compte, avec le moyen de se connecter et des liens vers la page À propos, les conditions d’utilisation et la politique de confidentialité
  que le serveur publie. Le client web masque ce que le serveur ne propose pas, dans sa navigation, ses menus et ses
  écrans d’administration : les fils local et fédéré, l’inscription, les comptes suggérés, les invitations, l’examen des
  inscriptions, les blocages d’inscription, la visibilité « Ce serveur », et les paramètres de l’inscription, des invitations, des fils
  publics, de la page d’accueil et des comptes suggérés.

La fédération, la modération, la recherche en texte intégral, le programme de soutien, les documents et les écrans d’administration fonctionnent comme sur n’importe quel
serveur.

<!-- source: usage 68ad8384438f -->
## Utilisation

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- Écrivez les options avant la clé et la valeur.
- Avec `-owner <username>`, la valeur de `preview_image` ou de `thumbnail` est lue comme le chemin d’un fichier image et téléversée comme
  média de ce compte local. Le worker traite l’image avant son utilisation. Une image paysage de 1200x630 est recommandée.
- Une modification enregistre une révision (`instance_*_revisions`) et met à jour la valeur courante (`instance_settings` / `instance_texts`)
  dans une seule transaction. Une modification depuis la CLI est enregistrée sans acteur (`BY` vaut `cli` dans l’historique).
- Définir de nouveau la même valeur ajoute quand même une révision.
- Codes de sortie : 0 en cas de succès, 1 pour une valeur rejetée ou une erreur de base de données (rien ne change), 2 pour des arguments incorrects.

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### Modifier le sens de la fédération

- Le restreindre conserve les relations d’abonnement, les publications importées et les enregistrements de livraison, et n’envoie aucun `Undo` ni `Reject`. Le flux
  s’arrête simplement, et revenir à `both` (ou à un autre mode) le reprend tel qu’il était.
- Les publications et réactions faites pendant la restriction ne sont pas livrées rétroactivement lorsque vous élargissez de nouveau.
- Côté distant, les abonnés des utilisateurs locaux ne reçoivent rien tant que le mode est `inbound`, et les abonnements des utilisateurs locaux ne sont pas
  mis à jour tant qu’il est `outbound` ou `off`.
- Les suppressions (de publications ou de comptes) effectuées pendant que le mode est `off` ne sont pas envoyées. En `inbound` et `outbound`, le retrait
  de ce qui avait déjà été envoyé (`Delete` et similaires) va aux destinataires d’origine et aux abonnés de l’époque.
- Les modifications sont enregistrées dans `instance_setting_revisions` et dans le journal d’audit (`target_kind = setting`).

Pour couper aussi les relations, utilisez la CLI `my admin federation sever`.

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- Elle met fin aux abonnements en attente et actifs entre comptes locaux et distants comme des terminaisons par modération
  (`follow_terminations`, `reason = moderation`). Un changement de mode ne l’appelle jamais automatiquement.
- `Reject` (`in`) ou `Undo` (`out`) n’est envoyé au côté distant que si le mode courant envoie ce sens. Dans un mode qui
  ne le fait pas (comme `off`), l’abonnement se termine du côté local uniquement et le côté distant n’en est pas informé.
- On peut l’exécuter à répétition sans risque. Elle affiche sur la sortie standard le nombre d’abonnements auxquels elle a mis fin.

<!-- source: admin-screens-and-the-client-api 887d153efcf9 -->
## Écrans d’administration et API cliente

Chaque réglage que la CLI peut modifier peut aussi l’être sur les écrans d’administration du client web et avec l’API cliente. Les deux exigent la permission de rôle `manage_settings` (seul le rôle intégré `admin` l’a). Comme avec la CLI, un changement est conservé comme une révision et enregistré par clé dans le journal d’audit (`setting_changed`, la cible est le réglage et l’acteur est cet utilisateur). Consultez le journal d’audit sur l’écran de modération (« Journal d’audit », `/moderation/audit-log`).

- **Client web** (« Paramètres du serveur », `/admin/settings`) : l’écran a une page par sujet, liée sous son titre. Chaque page a un formulaire et un bouton d’enregistrement, et l’enregistrement n’envoie que ce que vous avez modifié sur cette page ; les réglages des autres pages restent tels quels.

  | Page | Sections et réglages |
  | --- | --- |
  | Général (`/admin/settings`) | Nom, Descriptions (`name`, `short_description`, `description`) ; Contact et vignette (`contact_account`, `contact_email`, `thumbnail`, et le texte `thumbnail_alt`) |
  | Icône et logo (`/admin/settings/branding`) | L’icône, le logo et le logo pour fonds sombres, chacun enregistré avec son propre bouton ([Logo et icône](#logo-and-icon)) |
  | Inscription (`/admin/settings/registration`) | Inscription (`registration_mode`, `login_providers_only`) ; Invitations des membres (`invite_creators`, `member_invite_max_active`, `member_invite_max_uses`, `member_invite_max_lifetime_seconds`) ; Applications (`user_app_registration`, `oauth_metadata_document_clients`, `dynamic_client_registration`) |
  | Contenu (`/admin/settings/content`) | Langue (`default_locale`, choisie parmi les langues dont l’application dispose) ; Fils (`local_timeline_visibility`, `federated_timeline_visibility`) ; Publications (`require_media_description`) ; Page d’accueil (`public_counts`) ; Comptes suggérés (`featured_accounts`) |
  | Suppression des publications (`/admin/settings/auto-delete`) | Valeur par défaut des nouveaux comptes (les réglages `auto_delete_default_*`) ; Maximum (`auto_delete_max_age_days`, les réglages `auto_delete_max_allow_*`, `auto_delete_max_kept_pins`, `auto_delete_grace_days`). Enregistrer un maximum qui est activé ou rendu plus strict (moins de jours, moins d’exceptions ou d’épingles) demande d’abord confirmation, puisqu’il notifie les comptes qu’il retient nouvellement ([Un maximum de conservation des publications](#a-maximum-for-keeping-posts)) |
  | Fédération (`/admin/settings/federation`) | Fédération (`federation_mode`, `federation_scope` (enregistrer `allowlist` montre d’abord les abonnements que la bascule terminerait et demande confirmation, voir [ne fédérer qu’avec des serveurs autorisés](moderation.md#federating-only-with-allowed-servers)), `authorized_fetch`, `remote_media_policy`, `remote_media_cache_ttl_seconds` (à choisir parmi 1 heure, 1 jour, 7 jours, 30 jours ou 365 jours), `link_cards`, `link_cards_ignore_robots_txt`) ; Serveurs limités et suspendus (`domain_actions_visibility`, `domain_actions_rationale_visibility`) ; Examen externe (les réglages `hook_*`, voir [Hooks de décision](decision-hooks.md)) |
  | Recherche et robots (`/admin/settings/discovery`) | Moteurs de recherche et IA (`search_indexing`, `indexable_default`, `ai_usage`, `security_contact`) ; Recherche de publications (`search_visibilities`, uniquement tant que le serveur dispose de la recherche plein texte) |
  | Chat vocal (`/admin/settings/voice`) | `voice_room_starters`, `voice_public_summary`, `voice_participants_retention_days` ; la page n’est listée que tant que le serveur propose le chat vocal |

  Sur la page Général, une image est téléversée lorsque vous la choisissez et devient la miniature lorsque vous enregistrez. Qui peut voir les coûts d’exploitation, les revenus et la liste des soutiens se définit sur les écrans « Finances » et « Soutiens ».

  `max_status_length`, `preview_image`, `appeal_window_days`, ainsi que les durées de conservation (`*_retention_days`), `remote_media_cache_max_bytes` et `oauth_client_rate_limits` n’apparaissent pas sur l’écran ; modifiez-les avec la CLI ou l’API cliente. Les réglages de traduction ont un écran à part, ci-dessous.
- **Client web** (« Traduction », `/admin/translation`) : les fournisseurs dont ce serveur dispose, leur ordre et ce que chacun peut recevoir, la traduction du navigateur, les limites mensuelles, et la durée de conservation des traductions, avec les caractères envoyés ce mois-ci et le taux de réussite du cache. Chaque processus du serveur ajoute son nombre de réussites du cache au taux au plus une fois par minute ; les requêtes les plus récentes peuvent donc mettre une minute à apparaître, et un processus qui s’arrête peut laisser sa dernière minute non comptée. Mettez à jour votre politique de confidentialité ([modèle](privacy-policy-template.md#the-services-you-choose)) avant d’activer un fournisseur côté serveur : le texte envoyé à un fournisseur ne peut pas être rappelé. Les clés d’API des fournisseurs et l’adresse d’un [plugin de traduction](plugins.md#translating-posts) se trouvent dans la [configuration du déploiement](../reference/configuration.md) ; aucun réglage ni aucune réponse de l’API cliente ne les contient.
- **Limites de traduction.** Ce qui compte, ce sont les caractères envoyés, en points de code : les marqueurs qui remplacent les mentions, les liens et autres ne comptent pas, pas plus qu’une traduction que le serveur conserve déjà, une publication déjà dans la langue du lecteur, ou la traduction du navigateur. Chaque appel à un fournisseur compte, y compris celui qui échoue, puisque le fournisseur a pu recevoir le texte ; le nombre peut donc différer de ce que facture un fournisseur. Au-delà d’une limite, la requête est refusée avec un `429` et aucun fournisseur n’est appelé ; le client web utilise alors la traduction du navigateur du lecteur, lorsque vous l’autorisez et que le navigateur peut traduire la publication. Les mois sont en UTC.
- **API cliente** : `GET /api/v1/admin/settings` renvoie chaque réglage avec sa valeur actuelle, sa valeur par défaut, s’il est défini, et son type et ses valeurs acceptées (choix ou plage). `PATCH /api/v1/admin/settings` modifie des réglages ensemble avec `{"values": {"<key>": "<value>", ...}}`. Toutes les valeurs sont des chaînes, normalisées et enregistrées comme avec la CLI. Si une seule clé est inconnue ou qu’une valeur est invalide, la réponse est un 422 qui nomme la clé et rien ne change. Les clés modifiées sont enregistrées comme des révisions et dans le journal d’audit dans la même transaction.
- Les invitations, qui fonctionnent dans les modes `open`, `approval` et `invite`, sont créées sur l’écran de modération (« Invitations », `/moderation/invites`, permission `manage_invites` ; voir [codes d’invitation](registration.md#invite-codes)), et par les membres selon ce que permet `invite_creators` ([invitations des membres](registration.md#member-invites)).

<!-- source: rules b1076bb81e0c -->
## Règles

Les règles de l’instance apparaissent dans `rules` de `GET /api/v1/instance`, et les signalements les citent par ID.

- Une règle comporte une phrase d’une ligne (jusqu’à 300 caractères) et un complément facultatif (jusqu’à 1 000 caractères). Comme pour les textes,
  l’administrateur les écrit dans la langue qu’il souhaite et toutes les langues voient la même valeur (`dir="auto"` et `lang=""`).
  Il peut y avoir au plus 100 règles actives.
- Chaque modification (une phrase révisée, un réordonnancement) est conservée comme une révision (`instance_rule_revisions`), et l’ID d’une règle
  ne change pas.
- Une règle retirée disparaît de la liste des règles actives. Elle ne peut pas être rétablie, mais les signalements peuvent toujours y faire référence.

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- L’API d’administration est `/api/v1/admin/rules` (lister, ajouter, obtenir, réviser, déplacer, retirer) et exige la permission de rôle `manage_rules`.
- Sur l’écran d’administration du client web (« Règles », `/admin/rules`, `manage_rules`), la liste affiche les règles en vigueur dans
  l’ordre, et « Monter » et « Descendre » les réordonnent ; le filtre au-dessus de la liste affiche à la place les règles retirées. « Ajouter une
  règle » ouvre une page avec une phrase et un complément. Choisissez la phrase d’une règle pour ouvrir sa page, où vous la modifiez et la
  retirez (après une boîte de dialogue de confirmation). Chaque modification est enregistrée dans le journal d’audit (`rule_added`, `rule_revised`,
  `rule_moved`, `rule_retired`).
- Les codes de sortie sont les mêmes que pour les paramètres (0 en cas de succès, 1 pour une valeur rejetée ou une erreur de base de données, 2 pour des arguments incorrects).

<!-- source: link-card-domains c8f674387fa8 -->
## Domaines des cartes de lien

Les cartes de lien des publications (`link_cards`) proviennent d’un robot : le serveur récupère la page liée et son image. Une règle
arrête ce robot sur un domaine et ses sous-domaines, et laisse tout le reste du domaine intact. Elle a l’un de deux
modes. Une règle est le choix de l’opérateur, et elle s’applique en plus de ce que les sites liés refusent eux-mêmes ([aperçus de liens](link-previews.md#cards-of-links-in-posts)).

| Mode | Ce qu’il récupère | Cartes déjà présentes |
| --- | --- | --- |
| Arrêter la récupération (`no_fetch`) | Rien du domaine, ni pages ni images, même lorsqu’une redirection y mène | Masquées |
| Arrêter les images seulement (`no_images`) | Les pages, mais aucune image du domaine | Affichées sans leur image |

- Une règle ne porte que sur les cartes de lien. Le domaine reste fédéré, et les médias de ses publications sont récupérés comme d’habitude : pour couper un serveur, utilisez
  une [action de domaine](moderation.md), qui arrête aussi ses cartes de lien. Une règle n’est ni listée avec les actions de domaine ni exportée avec elles.
- Une règle couvre le domaine et ses sous-domaines, et une image est vérifiée par rapport aux hôtes de la page et de l’image. Toute règle qui couvre un hôte s’applique ; une règle sur un sous-domaine
  n’assouplit donc pas la règle de son parent. Un domaine s’écrit et se normalise comme pour une action de domaine : un nom d’hôte sans schéma ni port, sous n’importe quelle graphie d’un domaine internationalisé.
- Une modification prend effet immédiatement, y compris sur les cartes que le serveur possède déjà, et les images mandatées de ces cartes cessent d’être servies tant que les images sont arrêtées.
- Retirer une règle rétablit telles quelles les cartes que le serveur possède déjà. Un lien qui avait été ignoré obtient sa carte la prochaine fois qu’une publication en a besoin ; rien n’est récupéré à nouveau en masse.
  Une carte récupérée à nouveau pendant que les images étaient arrêtées reste sans image jusqu’à sa prochaine récupération.
- Sur l’écran d’administration du client web (« Domaines des cartes de lien », `/admin/link-card-domains`, `manage_settings`), « Ajouter une règle » définit une règle avec une note. La liste affiche 40 règles par page et peut être recherchée par domaine ; choisir un domaine ouvre la page de sa règle, où vous modifiez son mode et sa note, et la supprimez (après une boîte de dialogue de confirmation).
- L’API d’administration est `/api/v1/admin/link_card_domains` (lister, définir ; un `DELETE` avec le domaine le supprime) et exige la permission de rôle `manage_settings`. Un serveur conserve au plus 1 000 règles, chacune avec une note d’au plus 5 000 caractères.
- Chaque modification est enregistrée dans le journal d’audit (`link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed`).

<!-- source: theme 380636a0e583 -->
## Thème

Le thème du serveur est un ensemble de remplacements des jetons de conception du client web plus un élément optionnel de CSS (CSS personnalisé). Il
s’applique au client web de chaque visiteur et aux écrans que rend le serveur (connexion, inscription, conditions, etc.), en
clair comme en sombre.

- Vous pouvez remplacer chaque jeton de design sauf les couleurs primitives (`--palette-*`). Une couleur est `#rrggbb` pour le clair et pour le sombre
  (`color-overlay` peut aussi être `#rrggbbaa`) ; les autres jetons prennent une valeur de la forme de leur type (longueur, nombre,
  durée, courbe d’accélération, liste de noms de polices, ombre) dans sa plage. Lorsque vous remplacez `font-sans` ou `font-mono`, gardez
  `"my Noto Color Emoji"` en premier ; sinon les émojis du client web sont dessinés avec la police du système.
- Lorsque vous enregistrez, le serveur vérifie la forme et la plage des valeurs, le contraste de chaque paire de jetons de couleur (les jetons que vous ne
  remplacez pas utilisent leurs valeurs par défaut ; WCAG 2.2 AA en clair comme en sombre) et la longueur du CSS personnalisé (64 Kio). Si une vérification
  échoue, rien n’est enregistré.
- Le serveur ne lit pas le CSS personnalisé. Il le place dans la même feuille de style après les règles de jetons, en dehors de tout `@layer`. Il s’exécute
  sous la même CSP que le client web ; les images, polices et `@import` d’une autre origine ne peuvent donc pas être chargés.
  **Le serveur ne vérifie pas les effets du CSS personnalisé (contraste, défilement horizontal, taille des cibles tactiles, indication du focus,
  langues s’écrivant de droite à gauche) ; c’est la responsabilité de l’administrateur.** Les noms de classes et la structure des pages changent d’une
  version à l’autre ; seuls les jetons sont donc stables.
- Un thème est conservé sous forme de révisions (`instance_theme_revisions`), et la feuille de style de la révision courante est servie à
  `/theme/{revision ID}.css` (`immutable`, mise en cache longtemps). L’enveloppe HTML a un cache partagé (60 secondes plus 300
  secondes de revalidation) ; il faut donc quelques minutes après l’enregistrement pour que tout le monde l’obtienne. Une page déjà ouverte conserve
  l’ancien aspect jusqu’à son rechargement.
- `<meta name="theme-color">` (clair et sombre) dans l’enveloppe et `theme_color` / `background_color` (clair) du Web App
  Manifest prennent la valeur du `color-surface` du thème.
- Enregistrer une révision sans jetons et sans CSS personnalisé rétablit le thème par défaut, et la feuille de style n’est plus chargée.
- Vous pouvez le modifier sur l’écran d’administration du client web (« Thème », `/admin/theme`), avec l’API client (`GET` / `PUT
  /api/v1/admin/theme`) et avec la CLI. L’écran d’administration et l’API client exigent la permission de rôle `manage_settings`, et les
  modifications sont enregistrées dans le journal d’audit (`setting_changed`, cible `theme`).

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## Logo et icône

En plus de la vignette et de l’image d’aperçu, l’instance peut avoir sa propre icône, son logo et son logo sombre, chacun téléversé sous forme d’un SVG.
Tant qu’aucun n’est défini, les images mycroblog intégrées sont utilisées, et en effacer un rétablit l’image intégrée.

| Image | Forme | Où elle apparaît |
| --- | --- | --- |
| `icon` | Carrée (à 1 % près) | Le favicon, l’icône d’écran d’accueil d’iOS (`apple-touch-icon`), les icônes de l’application installée (Web App Manifest), la carte du serveur sur la page d’accueil et à côté de son nom dans le panneau latéral du client web, et partout où un client affiche le serveur sous forme d’image carrée |
| `logo` | De 1 à 8 fois plus large que haute | L’en-tête du client web et le haut des pages de connexion, d’inscription et de consentement à la place du logotype intégré et du nom du serveur (le logo doit contenir le nom, qui est son texte alternatif), la version HTML de chaque e-mail, et l’image d’aperçu de lien lorsque `preview_image` n’est pas définie |
| `logo_dark` | Comme `logo` | Fonds sombres ; sans lui, `logo` y est utilisé aussi |

- **Le serveur assainit le SVG.** Il le lit comme du XML et écrit un nouveau document à partir des seuls éléments autorisés (formes, groupes,
  `defs`, `symbol`, `use`, dégradés, `clipPath`, `mask`, `pattern`, `style`, `title`, `desc`) et attributs (géométrie,
  transformations, attributs de présentation, `id`, `class`, `style`). Les scripts, `foreignObject`, images intégrées, liens,
  animations, filtres (ombres et flous), gestionnaires d’événements, références à quoi que ce soit hors du fichier et URL `data:` sont supprimés,
  et la réponse liste ce qui a été supprimé. Seul le SVG assaini est stocké.
- Un SVG est refusé (rien n’est enregistré) lorsqu’il n’est pas du XML bien formé en UTF-8, comporte un `DOCTYPE` ou des déclarations d’entités, n’a pas de
  `viewBox`, dépasse 256 Kio (128 Kio après assainissement), compte plus de 5 000 éléments, est imbriqué sur plus de 32 niveaux, compte plus de 200 éléments
  `use`, n’a pas la bonne forme pour son type, contient `text` (convertissez d’abord le texte en contours dans votre éditeur), ou a du CSS qui
  pourrait charger quelque chose (`@import`, `@font-face`, `url()` autre que `url(#id)`, échappements). `@media (prefers-color-scheme: dark)`
  dans un `<style>` est autorisé, de sorte qu’une icône peut changer en mode sombre.
- Certains endroits n’acceptent pas de SVG (l’écran d’accueil d’iOS, les favicons des anciens Safari, les aperçus de liens, les e-mails). Pour eux, le client qui téléverse le
  SVG dessine aussi des versions PNG à partir du SVG assaini et les envoie avec lui : 32, 48, 180, 192 et 512 px et une icône maskable de 512 px
  pour `icon` ; un logo d’e-mail de 96 px de haut et une image d’aperçu de lien de 1200 × 630 pour `logo` ; un logo d’e-mail de 96 px de haut pour
  `logo_dark`. Le serveur vérifie que chacune est un PNG de la bonne taille (1 Mio au plus ; l’icône de 180 px, l’icône maskable et l’
  aperçu de lien doivent être opaques) et la réécrit sans métadonnées. Si un fichier est incorrect, rien n’est enregistré. Les versions opaques
  de l’icône sont dessinées sur une couleur de fond, `#rrggbb`, qui vaut par défaut le `color-surface` clair du thème. Le serveur ne
  vérifie pas que les versions PNG montrent la même image que le SVG.
- Les fichiers sont stockés dans le magasin de blobs sous `instance/brand/<revision>/` et servis à
  `https://<host>/brand/<revision>/<version>.svg|.png` avec `Cache-Control: public, max-age=31536000, immutable`, puisque le
  contenu d’une révision ne change jamais. Un SVG est servi avec `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` et `X-Content-Type-Options: nosniff`, de sorte que même ouvert seul il n’exécute rien et ne charge rien.
  Les fichiers des révisions antérieures restent, de sorte que les e-mails déjà envoyés conservent leur logo.
- Lorsqu’une icône est définie, l’enveloppe HTML lie ses favicons PNG, son SVG et son `apple-touch-icon` de 180 px, le Web App Manifest liste
  ses icônes de 512 px, de 192 px, maskable et SVG dans cet ordre (les autres serveurs du fédivers prennent la première comme icône de ce serveur),
  et `/apple-touch-icon.png` renvoie la version de 180 px. L’ETag de l’enveloppe change avec les images, et le cache partagé de l’enveloppe
  dure quelques minutes comme avec le thème. Une application installée récupère une nouvelle icône sur Android lorsque Chrome vérifie le manifeste la fois suivante ;
  sur les ordinateurs, l’application conserve son ancienne icône jusqu’à ce qu’elle soit installée de nouveau.
- `GET /api/v1/instance` renvoie `icon`, `logo` et `logo_dark` avec l’URL du SVG, l’URL et la taille de chaque version PNG,
  et la révision ; chacun est omis tant que l’image intégrée est utilisée.
- Définissez-les dans le client web sous **Paramètres du serveur** > **Icône et logo** (`/admin/settings/branding`, permission de rôle `manage_settings`). Choisissez un fichier
  SVG pour l’icône, le logo ou le logo pour fonds sombres : le serveur le vérifie, le navigateur dessine les versions PNG, et le
  formulaire les prévisualise (un onglet de navigateur, l’écran d’accueil, l’icône de l’application, un e-mail sur fond clair et sombre, et l’aperçu
  de lien) avec la liste de ce que le serveur a supprimé. Si le serveur refuse le fichier, la raison s’affiche sous le bouton avec la façon de
  le corriger. Pour l’icône, définissez aussi la couleur de fond de ses versions opaques. **Enregistrer** envoie le SVG avec chaque version
  PNG ; **Utiliser l’icône intégrée**, **Utiliser le logo intégré** et **Supprimer le logo pour fonds sombres** les effacent. Le
  client web conserve les images qu’il a affichées pour une utilisation hors ligne.
- L’API client fait de même, avec la permission de rôle `manage_settings` :
  `POST /api/v1/admin/instance/brand/sanitize` vérifie un SVG et renvoie le SVG assaini avec les versions PNG à dessiner (elle
  n’enregistre rien), `PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` enregistre le SVG avec chaque version PNG
  (`multipart/form-data`, une partie par version), `DELETE` sur le même chemin en efface un, et `GET /api/v1/admin/instance/brand` renvoie les images
  courantes avec la couleur de fond de l’icône. Chaque modification est conservée comme une révision (`instance_brand_revisions`) et enregistrée dans le
  journal d’audit (`setting_changed`, cible `brand_icon`, `brand_logo` ou `brand_logo_dark`).
- La CLI ne peut pas les définir, puisque les versions PNG sont dessinées par le client, mais elle peut les afficher et effacer une image défectueuse :

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## Émojis personnalisés

Les émojis personnalisés locaux sont affichés comme des images lorsque quelqu’un écrit `:shortcode:` dans le corps ou l’avertissement de contenu d’une publication, ou dans le
nom d’affichage, la biographie ou les champs de profil. Ils parviennent aussi à Mastodon, Misskey, GoToSocial et Akkoma par la balise `Emoji` des
Notes et des acteurs que vous envoyez.

- Un shortcode compte de 2 à 30 lettres, chiffres ou `_`. Il est sensible à la casse et ne peut pas être modifié après l’enregistrement de l’émoji.
  Les alias (jusqu’à 10) et une catégorie (jusqu’à 64 caractères) servent à la recherche et au regroupement dans le sélecteur d’émojis.
- L’image est un PNG, un GIF ou un WebP, d’au plus `custom_emoji_max_bytes` (256 Kio par défaut) et d’au plus 1 024 px de chaque côté. Le
  serveur produit des dérivés qui tiennent dans des cadres de 64, 128 et 256 px et ne conserve pas l’original. Un APNG devient une image fixe
  de sa première image. GoToSocial ne récupère par défaut pas les émojis de plus de 100 Kio ; de petites images sont donc recommandées.
- Un émoji désactivé n’apparaît nulle part (publications, profils, fédération, sélecteur), et `:shortcode:` est de nouveau affiché comme du texte. Un émoji
  réglé pour être masqué du sélecteur n’est simplement pas dans le sélecteur et peut toujours être utilisé en l’écrivant. Un émoji venant
  d’un autre serveur ne peut qu’être désactivé ou copié comme émoji de ce serveur.
- La copie récupère l’image de l’émoji d’un autre serveur selon les mêmes limites que le proxy de médias (les images de domaines avec
  `reject_media` ne sont pas récupérées), exécute les mêmes vérifications que ci-dessus et l’enregistre comme nouvel émoji local. Le shortcode est celui de
  l’original ou celui que vous donnez. L’image est réduite au cadre de 256 px, et une image qui ne peut pas être récupérée ou qui échoue aux vérifications
  ne laisse rien derrière elle.
- La suppression ne peut pas être annulée. Les émojis des publications passées redeviennent aussi du texte, et enregistrer de nouveau le même shortcode crée un
  émoji différent. Les images que des serveurs fédérés ont copiées ne peuvent pas être supprimées.
- L’écran d’administration (« Émojis personnalisés », `/admin/custom-emojis`), l’API client (`/api/v1/admin/custom_emojis`) et la CLI font les
  mêmes choses. Sur l’écran, « Enregistrer un émoji personnalisé » ouvre le formulaire d’enregistrement (`/admin/custom-emojis/new`), et choisir
  un émoji dans la liste ouvre sa page (`/admin/custom-emojis/{id}`), où vous modifiez ses paramètres, remplacez son image, l’activez ou le
  désactivez, le supprimez, ou copiez celui d’un autre serveur ; la liste elle-même active et désactive un émoji d’une seule pression. L’écran d’administration et l’API client exigent la permission de rôle `manage_custom_emojis` (seul le rôle intégré `admin`
  la possède). L’enregistrement, la copie, le remplacement de l’image, la modification des paramètres et des alias, l’activation et la désactivation, et la suppression sont
  enregistrés dans le journal d’audit (cible `custom_emoji`) quelle que soit la manière dont ils sont faits (une opération depuis la CLI est enregistrée comme une opération
  CLI).

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- Un utilisateur ayant la permission de rôle `create_custom_emojis` (une fonctionnalité utilisateur) peut demander la création d’un émoji depuis « Émoji
  personnalisé » dans les paramètres (`/settings/custom-emojis`), modifier les alias, la catégorie et la visibilité dans le sélecteur des émojis qu’il a
  créés, et les supprimer. Remplacer une image est aussi une demande. Une demande n’apparaît nulle part (sélecteur, publications, réactions,
  fédération) tant que quelqu’un ayant `manage_custom_emojis` ne l’a pas approuvée. Approuvez-la ou rejetez-la sur la page « Demandes » de « Émojis
  personnalisés » dans l’écran d’administration (`/admin/custom-emojis/requests`), avec l’API client (`/api/v1/admin/custom_emoji_requests`) ou avec la CLI ci-dessus. Les personnes ayant
  `manage_custom_emojis` sont notifiées d’une demande (`admin_custom_emoji_request`), et le demandeur est notifié de
  l’approbation ou du rejet. La demande, le retrait, l’approbation et le rejet sont enregistrés dans le journal d’audit (cible
  `custom_emoji_request`).
- Une demande de création en attente réserve son shortcode. Une demande ou un enregistrement avec le même shortcode est impossible tant que
  la demande en attente n’est pas rejetée ou retirée. L’image d’un émoji approuvé est copiée comme un téléversement de l’acteur de l’instance, de sorte qu’elle
  reste même si le compte du demandeur est supprimé.
- Un créateur ne peut modifier ses émojis que tant qu’il détient `create_custom_emojis`. Les émojis restent si le rôle est retiré.
  Les administrateurs peuvent modifier et supprimer tous les émojis, quel que soit le créateur.
- Lorsque le rôle provient d’une formule de soutien, le nombre d’émojis qu’un utilisateur peut créer chaque mois est limité par la limite
  mensuelle d’émojis de la formule. Elle compte les demandes de création que l’utilisateur a soumises au cours du mois calendaire courant (UTC) et qui sont
  approuvées ou en attente ; les demandes rejetées et retirées ne comptent pas, et supprimer un émoji ne libère pas de place. Une demande
  compte dans le mois où elle a été soumise, même si elle est approuvée le mois suivant. Le compteur est remis à zéro à 00:00 UTC le premier
  jour de chaque mois, et la page des paramètres indique à l’utilisateur quand, dans son propre fuseau horaire. Le remplacement de l’image d’un émoji et les
  émojis que les administrateurs enregistrent ne comptent pas. Un rôle qu’un administrateur a attribué à la main, et un serveur où les soutiens
  sont désactivés, n’ont aucune limite.
