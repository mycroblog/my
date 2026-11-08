<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# Vue d’ensemble d’un serveur mycroblog

La première page à lire avant de lancer un serveur. mycroblog est un microblog qui se fédère
avec le reste du fediverse via ActivityPub ; `my` est son logiciel serveur, un binaire unique qui contient le serveur, le worker
de tâches, le client web et les commandes qui s’en occupent. Cette page présente de quoi se compose un serveur, ce que
vous choisissez par la configuration, comment un petit serveur devient un grand serveur, et où lire ensuite.

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## De quoi se compose un serveur

![L’architecture d’un serveur mycroblog : les clients atteignent les processus my en HTTPS ; les processus utilisent les composants choisis par la configuration et les services extérieurs au serveur que vous activez. Les sections ci-dessous décrivent chaque composant.](images/architecture.svg)

De haut en bas du schéma :

- **Qui se connecte.** Les personnes utilisent le client web dans leur navigateur (il peut être installé comme une application), les applications et les bots utilisent
  l’API cliente, et les autres serveurs du fediverse remettent et récupèrent des publications via ActivityPub. Tous atteignent le serveur en
  HTTPS.
- **Devant le serveur.** `my` parle en HTTP simple : un proxy inverse ou un répartiteur de charge termine donc le HTTPS devant lui.
  Un CDN devant les fichiers média est facultatif.
- **Les processus `my`.** Chaque processus exécute `my serve`, et son [`role`](../reference/configuration.md#role) indique ce qu’il
  exécute : `web` le serveur HTTP, `worker` le worker de tâches, qui distribue les publications, récupère des données auprès d’autres serveurs, envoie des e-mails,
  traite les médias et exécute les tâches périodiques, et `all`, la valeur par défaut, les deux dans un même processus.
- **Les composants que vous choisissez.** Chaque composant au milieu du schéma a un paramètre qui choisit son adaptateur, listé dans
  [la section suivante](#parts-you-choose-by-configuration).
- **Les services extérieurs au serveur.** CAPTCHA, chat vocal, paiements, plugins et import des coûts cloud restent chacun désactivés
  tant que vous ne les configurez pas ; le serveur appelle alors le service que vous indiquez.

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## Les composants que vous choisissez par la configuration

Chaque composant est un paramètre de la [référence de configuration](../reference/configuration.md), défini dans un fichier TOML ou comme
variable d’environnement (`MY_` suivi de la clé en majuscules, par exemple `MY_DATABASE`). La première option de chaque liste est la valeur par défaut.
Lorsqu’un composant conserve des données, choisissez-le avant le premier démarrage : les données ne se déplacent pas d’elles-mêmes quand le paramètre change.

| Composant | Paramètre | Options | À lire ensuite |
| --- | --- | --- | --- |
| Base de données | [`database`](../reference/configuration.md#database) | `sqlite` (un fichier), `postgres` (PostgreSQL et services compatibles) | [Base de données](database.md), [sauvegarde](backup.md) |
| File de tâches | [`queue`](../reference/configuration.md#queue) | `db` (une table de la base de données), `inline` (exécute chaque tâche aussitôt dans le même processus, pour les tests et le développement), `cloudtasks` (Google Cloud Tasks réveille le serveur pour chaque tâche ; nécessite PostgreSQL) | [La file de tâches](running.md#the-job-queue-and-failed-dead-jobs) |
| Magasin clé-valeur, pour les caches et les limites de débit | [`kv`](../reference/configuration.md#kv) | `db` (une table de la base de données), `memory` (chaque processus pour lui-même), `valkey` (Valkey ou un autre serveur au protocole Redis, partagé par tous les processus) | [`kv`](../reference/configuration.md#kv) |
| Stockage des médias | [`blob`](../reference/configuration.md#blob) | `fs` (un répertoire), `s3` (Amazon S3, Cloudflare R2, MinIO, Garage et autres stockages compatibles S3), `gcs` (Google Cloud Storage) | [Fichiers Docker Compose](running.md#docker-compose-files), [sauvegarder les médias](backup.md#media-in-a-bucket) |
| Traitement des médias | [`media_image`](../reference/configuration.md#media_image), [`media_video`](../reference/configuration.md#media_video), [`media_audio`](../reference/configuration.md#media_audio), un pour chaque type de fichier | `native` (dans le serveur), `plugin` (un plugin de votre choix convertit les fichiers de ce type) | [Convertir les médias](plugins.md#converting-media) |
| Flux en temps réel entre les processus | [`stream`](../reference/configuration.md#stream) | `hub` (à l’intérieur d’un seul processus), `postgres` (`LISTEN` / `NOTIFY`), `valkey`, `sns` (Amazon SNS et SQS), `pubsub` (Google Cloud Pub/Sub) | [`stream`](../reference/configuration.md#stream) |
| Gestionnaire de clés, qui chiffre les clés privées des comptes | [`key_manager`](../reference/configuration.md#key_manager) | `local` (une clé de chiffrement de clés que vous conservez), `gcpkms` (Google Cloud KMS), `awskms` (AWS KMS), `plugin` | [Gérer la clé](key-management.md) |
| E-mail | [`mail`](../reference/configuration.md#mail) | `none`, `smtp`, `ses` (Amazon SES), `plugin` | [Envoyer des e-mails](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`, `turnstile` (Cloudflare Turnstile), `hcaptcha`, `recaptcha` (Google reCAPTCHA v2) | [CAPTCHA](captcha.md) |
| Chat vocal | [`voice`](../reference/configuration.md#voice) | `off`, `p2p` (de navigateur à navigateur, avec TURN via `voice_turn` : `none`, `coturn` ou `cloudflare`), `livekit`, `sora` | [Chat vocal](voice.md) |
| Paiements des soutiens | [`payments`](../reference/configuration.md#payments) | `none`, `stripe`, `payjp` (PAY.JP) | [Soutiens](supporters.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md) |
| Plugins | `plugin_<kind>_address` | aucun par défaut ; une adresse pour chaque type : `translator`, `search`, `billing`, `mailer`, `media_image`, `media_video`, `media_audio`, `key_manager`, et les hooks de décision `inbound.status`, `local.status`, `inbound.follow`, `signup.check`, `media.check`, `inbound.actor`, `local.profile` | [Plugins](plugins.md), [hooks de décision](decision-hooks.md) |
| Traduction des publications | `translator_<provider>_*`, [`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | aucun (seulement la traduction propre au navigateur) ; `libretranslate`, `deepl`, `google` (Google Cloud Translation), `aws` (Amazon Translate), ou un plugin de votre choix devant le service que vous choisissez | [Traduire les publications](translation.md), [un plugin de traduction](plugins.md#translating-posts) |
| Recherche plein texte des publications | [`search`](../reference/configuration.md#search) | `none` ; ou `elasticsearch`, `opensearch` ou `meilisearch` ([moteurs de recherche](search-engines.md)) ; ou `plugin`, un plugin de votre choix devant un autre moteur | [Recherche plein texte](search.md), [un plugin de recherche](plugins.md#searching-posts) |
| CDN pour les médias | [`media_origin`](../reference/configuration.md#media_origin) | non défini (le serveur sert les médias depuis sa propre origine), ou l’origine de votre CDN | [Servir les médias via un CDN](cdn.md) |
| Coûts cloud | [`billing_sources`](../reference/configuration.md#billing_sources) | aucun, ou l’un de `aws` (AWS Cost Explorer), `gcp` (l’export de facturation ou le budget de Google Cloud), `plugin` | [Importer les coûts cloud](billing-import.md) |
| Métriques | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`, `prometheus`, `otlp` (OpenTelemetry), `emf` (CloudWatch) | [Métriques](running.md#metrics), [alertes](alerts.md) |
| Traces | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`, `otlp` | [Traces](running.md#traces) |
| Rapports d’erreurs | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none` (la ligne de journal et les groupes d’erreurs uniquement), `sentry` (Sentry, GlitchTip, Datadog), `otlp` | [Rapports d’erreurs](error-reporting.md) |

Le binaire publié et l’image de conteneur contiennent tous les adaptateurs du tableau. Un binaire que vous compilez vous-même peut omettre
ceux que vous n’utilisez pas grâce aux [build tags](../reference/build-tags.md), et refuse alors leurs paramètres au démarrage.

`my config check` signale d’un coup tous les problèmes de la configuration, avant que vous ne démarriez le serveur
([liste de contrôle de production](production-checklist.md)).

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## D’un petit serveur à un grand serveur

![Trois étapes d’un petit serveur à un grand serveur : un processus sur SQLite et le système de fichiers, puis PostgreSQL et un stockage objet derrière un CDN, puis des processus web et worker séparés avec un broker de flux. La liste ci-dessous décrit chaque étape.](images/growth.svg)

1. **Commencer petit.** Un processus avec `role = all`, sur SQLite, avec les médias sur le système de fichiers et les valeurs par défaut pour le
   reste : la file et le magasin clé-valeur dans la base de données, et le flux en temps réel à l’intérieur du processus. Les fichiers
   Docker Compose de `deploy/docker` démarrent cette configuration sur une seule machine ([déployer avec Docker Compose](deploy-docker-compose.md)).
   Elle convient à un serveur personnel ou petit. `stream = hub` n’atteint que le processus dans lequel il s’exécute : cette configuration reste donc à un seul
   processus.
2. **Sortir les données.** PostgreSQL pour la base de données, un bucket compatible S3 ou Google Cloud Storage pour les médias,
   et un CDN devant les médias. Le serveur reste un seul processus, mais il ne dépend plus d’un seul disque, et les
   processus de l’étape suivante peuvent partager les données. Décidez-le avant le premier démarrage si possible : déplacer ensuite les données
   de SQLite et du système de fichiers est un chantier à part ([ce qu’il faut sauvegarder](backup.md#what-to-back-up) liste
   ce qui doit être déplacé).
3. **Passer à l’échelle.** Exécutez des processus avec `role = web` pour les requêtes et `role = worker` pour les tâches, autant que chacun
   en demande, et répartissez les workers par type de tâche ([répartir les workers](running.md#splitting-workers-by-job-type)).
   Avec plus d’un processus, le flux en temps réel a besoin d’un broker qui atteint tous les processus (`postgres`, `valkey`,
   `sns` ou `pubsub`), et le magasin clé-valeur doit être partagé (`db` ou `valkey`), puisque `memory` garde séparées les limites
   de débit de chaque processus. La file de tâches reste dans
   PostgreSQL, ou passe à Google Cloud Tasks ([Cloud Tasks](running.md#cloud-tasks)).

Le [déploiement sur Cloud Run](deploy-cloud-run.md) est un exemple de configuration située entre la deuxième et la troisième étape :
`role = all` sur Cloud Run, PostgreSQL sur Neon, les médias dans Cloud Storage, Cloud Tasks pour les tâches et Pub/Sub pour le
flux, sans aucune instance en cours d’exécution tant que personne n’utilise le serveur.

<!-- source: where-to-go-next 2f1d25c01f8a -->
## Où aller ensuite

Lisez les guides de l’opérateur dans cet ordre :

1. **Démarrer :** [lancer le serveur](running.md) ou [le déployer sur Cloud Run](deploy-cloud-run.md), puis parcourez la
   [liste de contrôle de production](production-checklist.md) avant de l’ouvrir aux utilisateurs.
2. **Configurer l’instance :** ses [paramètres](instance-settings.md), l’[inscription](registration.md),
   les [fournisseurs de connexion externes](login-providers.md), l’[e-mail](email.md), le [CAPTCHA](captcha.md), le [CDN](cdn.md),
   les [aperçus de liens](link-previews.md), et les [moteurs de recherche et robots d’IA](crawlers.md).
3. **Activer des fonctionnalités :** la [modération](moderation.md), les [relais](relays.md), les [webhooks](webhooks.md),
   les [plugins](plugins.md) et les [hooks de décision](decision-hooks.md), le [chat vocal](voice.md), la [recherche plein texte](search.md),
   les [soutiens](supporters.md) et leurs paiements, et l’[import des coûts cloud](billing-import.md).
4. **L’exploiter au quotidien :** la [supervision](monitoring.md), les [alertes](alerts.md), les [rapports d’erreurs](error-reporting.md),
   la [maintenance](maintenance.md), la [sauvegarde](backup.md), la [base de données](database.md),
   la [clé de chiffrement de clés](key-management.md), et les [administrateurs et rôles](admin-cli.md).
5. **Juridique et fermeture :** les modèles de [politique de confidentialité](privacy-policy-template.md) et de
   [registre des traitements](processing-record-template.md), et la [fermeture du service](service-closure.md).

La [référence de configuration](../reference/configuration.md) liste tous les paramètres, et la [référence des tâches](../reference/jobs.md)
toutes les tâches que les workers exécutent.
