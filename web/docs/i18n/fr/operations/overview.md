<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# Vue d’ensemble d’un serveur mycroblog

La première page à lire avant de lancer un serveur. mycroblog est un microblog qui se fédère
avec le reste du fediverse via ActivityPub ; `my` est son logiciel serveur, un binaire unique qui contient le serveur, le worker
de tâches, le client web et les commandes qui s’en occupent. Cette page présente de quoi se compose un serveur, ce que
vous choisissez par la configuration, comment un petit serveur devient un grand serveur, et où lire ensuite.

<!-- source: what-a-server-is-made-of 51fb6c4e4a24 -->
## De quoi un serveur est fait

![L’architecture d’un serveur mycroblog : les clients atteignent les processus my en HTTPS ; les processus utilisent les composants choisis par la configuration et les services extérieurs au serveur que vous activez. Les sections ci-dessous décrivent chaque composant.](images/architecture.svg)

Du haut du schéma vers le bas :

- **Qui se connecte.** Les personnes utilisent le client web dans leur navigateur (il peut être installé comme une application), les applications et les bots utilisent
  l’API cliente, et les autres serveurs du fédivers livrent et récupèrent des publications via ActivityPub. Tous atteignent le serveur en
  HTTPS.
- **Devant le serveur.** Par défaut, `my` parle en HTTP simple ; un proxy inverse ou un répartiteur de charge termine donc le HTTPS devant lui.
  Sur une seule machine, `my` peut servir lui-même le HTTPS à la place ([servir le HTTPS depuis le serveur](running.md#serving-https-from-the-server)).
  Un CDN devant les fichiers de médias est facultatif.
- **Les processus `my`.** Chaque processus exécute `my serve`, et son [`role`](../reference/configuration.md#role) dit ce qu’il
  exécute : `web` le serveur HTTP, `worker` le worker de jobs, qui livre les publications, récupère des données d’autres serveurs, envoie des e-mails,
  traite les médias et exécute les jobs périodiques, et `all`, la valeur par défaut, les deux dans un seul processus.
- **Les composants que vous choisissez.** Chaque composant au milieu du schéma a un paramètre qui choisit son adaptateur, listé dans
  [la section suivante](#parts-you-choose-by-configuration).
- **Les services extérieurs au serveur.** Le CAPTCHA, le chat vocal, les paiements, les plugins et l’import des coûts du cloud restent chacun désactivés
  tant que vous ne les configurez pas, et le serveur appelle alors le service que vous nommez.

<!-- source: parts-you-choose-by-configuration 34d13fdad524 -->
## Les composants que vous choisissez par la configuration

Chaque composant est un paramètre de la [référence de la configuration](../reference/configuration.md), défini dans un fichier TOML ou comme une
variable d’environnement (`MY_` et la clé en majuscules, comme `MY_DATABASE`). La première option de chaque liste est la valeur par défaut.
Lorsqu’un composant conserve des données, choisissez-le avant le premier démarrage : les données ne se déplacent pas d’elles-mêmes lorsque le paramètre change.

| Composant | Paramètre | Options | À lire ensuite |
| --- | --- | --- | --- |
| Base de données | [`database`](../reference/configuration.md#database) | `sqlite` (un fichier), `postgres` (PostgreSQL et services compatibles) | [Base de données](database.md), [sauvegarde](backup.md) |
| File de jobs | [`queue`](../reference/configuration.md#queue) | `db` (une table dans la base de données), `inline` (exécute chaque job aussitôt dans le même processus, pour les tests et le développement), `cloudtasks` (Google Cloud Tasks réveille le serveur pour chaque job ; exige PostgreSQL) | [La file de jobs](running.md#the-job-queue-and-failed-dead-jobs) |
| Magasin clé-valeur, pour les caches, les limites de débit et les réponses conservées un jour pour les écritures renvoyées | [`kv`](../reference/configuration.md#kv) | `db` (une table dans la base de données), `memory` (chaque processus de son côté), `valkey` (Valkey ou un autre serveur au protocole Redis, partagé par tous les processus) | [`kv`](../reference/configuration.md#kv) |
| Stockage des médias | [`blob`](../reference/configuration.md#blob) | `fs` (un répertoire), `s3` (Amazon S3, Cloudflare R2, MinIO, Garage et autres stockages compatibles S3), `gcs` (Google Cloud Storage) | [Fichiers Docker Compose](running.md#docker-compose-files), [sauvegarder les médias](backup.md#media-in-a-bucket) |
| Traitement des médias | [`media_image`](../reference/configuration.md#media_image), [`media_video`](../reference/configuration.md#media_video), [`media_audio`](../reference/configuration.md#media_audio), un pour chaque type de fichier | `native` (dans le serveur), `plugin` (un plugin à vous convertit les fichiers de ce type) | [Convertir des médias](plugins.md#converting-media) |
| Flux en temps réel entre les processus | [`stream`](../reference/configuration.md#stream) | `hub` (dans un seul processus), `postgres` (`LISTEN` / `NOTIFY`), `valkey`, `sns` (Amazon SNS et SQS), `pubsub` (Google Cloud Pub/Sub) | [`stream`](../reference/configuration.md#stream) |
| Gestionnaire de clés, qui chiffre les clés privées des comptes | [`key_manager`](../reference/configuration.md#key_manager) | `local` (une clé de chiffrement de clés que vous conservez), `gcpkms` (Google Cloud KMS), `awskms` (AWS KMS), `plugin` | [Gérer la clé](key-management.md) |
| E-mail | [`mail`](../reference/configuration.md#mail) | `none`, `smtp`, `ses` (Amazon SES), `plugin` | [Envoyer des e-mails](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`, `turnstile` (Cloudflare Turnstile), `hcaptcha`, `recaptcha` (Google reCAPTCHA v2) | [CAPTCHA](captcha.md) |
| Chat vocal | [`voice`](../reference/configuration.md#voice) | `off`, `p2p` (de navigateur à navigateur, avec TURN depuis `voice_turn` : `none`, `coturn` ou `cloudflare`), `livekit`, `sora` | [Chat vocal](voice.md) |
| Paiements des soutiens | [`payments`](../reference/configuration.md#payments) | `none`, `stripe`, `payjp` (PAY.JP) | [Soutiens](supporters.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md) |
| Plugins | `plugin_<kind>_address` | aucun par défaut ; une adresse pour chaque type : `translator`, `search`, `billing`, `mailer`, `media_image`, `media_video`, `media_audio`, `key_manager`, et les hooks de décision `inbound.status`, `local.status`, `inbound.follow`, `signup.check`, `media.check`, `inbound.actor`, `local.profile` | [Plugins](plugins.md), [hooks de décision](decision-hooks.md) |
| Traduction des publications | `translator_<provider>_*`, [`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | aucune (seulement la traduction propre au navigateur) ; `libretranslate`, `deepl`, `google` (Google Cloud Translation), `aws` (Amazon Translate), ou un plugin à vous devant le service que vous choisissez | [Traduire des publications](translation.md), [un plugin de traduction](plugins.md#translating-posts) |
| Recherche en texte intégral des publications | [`search`](../reference/configuration.md#search) | `none` ; ou `elasticsearch`, `opensearch` ou `meilisearch` ([moteurs de recherche](search-engines.md)) ; ou `plugin`, un plugin à vous devant un autre moteur | [Recherche en texte intégral](search.md), [un plugin de recherche](plugins.md#searching-posts) |
| CDN pour les médias | [`media_origin`](../reference/configuration.md#media_origin) | non défini (le serveur sert les médias depuis sa propre origine), ou l’origine de votre CDN | [Servir les médias par un CDN](cdn.md) |
| Coûts du cloud | [`billing_sources`](../reference/configuration.md#billing_sources) | aucun, ou l’un quelconque de `aws` (AWS Cost Explorer), `gcp` (l’export de facturation ou le budget de Google Cloud), `cloudflare` (les frais d’utilisation de Cloudflare), `plugin` | [Importer les coûts du cloud](billing-import.md) |
| Métriques | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`, `prometheus`, `otlp` (OpenTelemetry), `emf` (CloudWatch) | [Métriques](running.md#metrics), [alertes](alerts.md) |
| Traces | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`, `otlp` | [Traces](running.md#traces) |
| Signalements d’erreurs | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none` (la ligne de journal et les groupes d’erreurs seulement), `sentry` (Sentry, GlitchTip, Datadog), `otlp` | [Signalement des erreurs](error-reporting.md) |

Le binaire publié et l’image de conteneur contiennent tous les adaptateurs du tableau.

`my config check` signale d’un coup tous les problèmes de la configuration, avant que vous démarriez le serveur
([liste de contrôle de production](production-checklist.md)).

<!-- source: from-a-small-server-to-a-large-one ed28f8cf5385 -->
## D’un petit serveur à un grand

![Trois étapes d’un petit serveur à un grand : un seul processus sur SQLite et le système de fichiers, puis PostgreSQL et un stockage d’objets derrière un CDN, puis des processus web et worker séparés avec un broker de flux. La liste ci-dessous décrit chaque étape.](images/growth.svg)

1. **Commencer petit.** Un seul processus avec `role = all`, sur SQLite, avec les médias sur le système de fichiers et les valeurs par défaut pour
   le reste : la file d’attente et le magasin clé-valeur dans la base de données, et le flux en temps réel à l’intérieur du processus. Les fichiers Docker
   Compose de `deploy/docker` démarrent cette configuration sur une seule machine ([déployer avec Docker Compose](deploy-docker-compose.md)).
   Elle convient à un serveur personnel ou petit. `stream = hub` n’atteint que le processus dans lequel il s’exécute ; cette configuration reste donc à un
   seul processus.
2. **Sortir les données.** PostgreSQL pour la base de données, un bucket compatible S3 ou Google Cloud Storage pour les médias,
   et un CDN devant les médias. Le serveur reste un seul processus, mais il ne dépend plus d’un seul disque, et les
   processus de l’étape suivante peuvent partager les données. Décidez-le avant le premier démarrage si vous le pouvez : déplacer plus tard les données
   de SQLite et du système de fichiers est un travail à part entière ([ce qu’il faut sauvegarder](backup.md#what-to-back-up) liste
   ce qui doit être déplacé).
3. **Passer à l’échelle.** Exécutez des processus `role = web` pour les requêtes et `role = worker` pour les jobs, autant que chacun
   en demande, et répartissez les workers par type de job ([répartir les workers](running.md#splitting-workers-by-job-type)).
   Avec plus d’un processus, le flux en temps réel a besoin d’un broker qui atteint chaque processus (`postgres`, `valkey`,
   `sns` ou `pubsub`), et le magasin clé-valeur doit être partagé (`db` ou `valkey`), puisque `memory` garde séparés, pour chaque processus, les limites
   de débit et les réponses aux écritures renvoyées. La file des jobs reste dans
   PostgreSQL, ou passe à Google Cloud Tasks ([Cloud Tasks](running.md#cloud-tasks)).

<!-- source: ways-to-deploy eeb6d182d4dc -->
## Façons de déployer

- **[Docker Compose](deploy-docker-compose.md)** sur une seule machine : la première étape, ou la deuxième avec PostgreSQL et un bucket.
- **[Google Cloud](deploy-google-cloud.md)** : `role = all` sur Cloud Run, PostgreSQL comme Neon, médias dans Cloud Storage, Cloud
  Tasks pour les jobs et Pub/Sub pour le flux, sans aucune instance en marche tant que personne n’utilise le serveur.
- **[AWS](deploy-aws.md)** : `role = all` sur ECS Fargate, PostgreSQL comme Neon, médias dans S3, jobs dans la base de données et SNS
  pour le flux.

Sur les deux clouds, un module OpenTofu crée tout le déploiement : [Déployer avec OpenTofu](deploy-opentofu.md) donne les étapes qu’ils partagent.

<!-- source: where-to-go-next 48f37c91b820 -->
## Où aller ensuite

Lisez les guides de l’opérateur dans cet ordre :

1. **Premiers pas :** [exécuter le serveur](running.md) et [le déployer](#ways-to-deploy), puis parcourez la
   [liste de contrôle de production](production-checklist.md) avant de l’ouvrir aux utilisateurs.
2. **Configurer le serveur :** ses [réglages](instance-settings.md), l’[inscription](registration.md),
   les [fournisseurs de connexion externes](login-providers.md), l’[e-mail](email.md), le [CAPTCHA](captcha.md), le [CDN](cdn.md),
   les [aperçus de liens](link-previews.md) et les [moteurs de recherche et robots d’IA](crawlers.md).
3. **Activer des fonctionnalités :** la [modération](moderation.md), les [relais](relays.md), les [webhooks](webhooks.md),
   les [plugins](plugins.md) et les [hooks de décision](decision-hooks.md), le [chat vocal](voice.md), la [recherche en texte intégral](search.md),
   les [soutiens](supporters.md) et leurs paiements, et l’[importation des coûts cloud](billing-import.md).
4. **L’exploiter au quotidien :** la [supervision](monitoring.md), les [alertes](alerts.md), le [rapport d’erreurs](error-reporting.md),
   la [maintenance](maintenance.md), la [sauvegarde](backup.md), la [base de données](database.md),
   la [clé de chiffrement de clés](key-management.md) et les [administrateurs et rôles](admin-cli.md).
5. **Aspects juridiques et fermeture :** les modèles de [politique de confidentialité](privacy-policy-template.md) et de
   [registre des traitements](processing-record-template.md), et la [fermeture du service](service-closure.md).

La [référence de configuration](../reference/configuration.md) liste chaque réglage, et la [référence des jobs](../reference/jobs.md)
chaque job que les workers exécutent.
