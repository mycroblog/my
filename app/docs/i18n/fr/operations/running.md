<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Exécuter le serveur : démarrage, arrêt et contrôles de santé

Comment démarrer et arrêter le serveur, obtenir son image et le surveiller pendant son fonctionnement : métriques, traces, file de tâches et contrôles de santé.

<!-- source: start-the-server 0d5f8898a2f5 -->
## Démarrer le serveur

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` valide la configuration, construit les adaptateurs de base de données, de file de tâches et de gestion des clés, et accepte le HTTP sur `listen`.
Il parle en HTTP/1.1 simple, sans TLS : placez devant lui un reverse proxy ou un répartiteur de charge qui termine le HTTPS (et le HTTP/2 ou HTTP/3 pour les clients) et transmet en HTTP/1.1.
Les mises à jour en temps réel (`GET /api/v1/streaming`, les flux des fils local et fédéré, et les flux des salons vocaux) sont des réponses de longue durée qui envoient un commentaire de battement de cœur toutes les 30 secondes (15 pour un salon vocal) et restent ouvertes lorsqu’elles sont inactives.
Le proxy ne doit pas mettre ces réponses en mémoire tampon (nginx : le serveur envoie `X-Accel-Buffering: no`), et son délai d’inactivité ou de lecture doit être supérieur à l’intervalle du battement de cœur.
Il n’applique pas le schéma : appliquez-le avec `my schema apply` avant de démarrer (consultez [Base de données](database.md) ; les mises à niveau l’exécutent dans le cadre de [Mise à niveau](database.md#upgrading)).
Dans la configuration Docker Compose (`deploy/docker/compose.yaml` ; consultez [Déployer avec Docker Compose](deploy-docker-compose.md)), un service ponctuel `schema` exécute `my schema apply` avant le démarrage du serveur.
Si le schéma n’est pas appliqué, le processus démarre tout de même et la disponibilité renvoie 503, et il ne sert rien d’autre tant que le schéma n’est pas là.

Le premier `my serve` ou la première commande `my` qui utilise la base de données y enregistre `host` et `account_domain`. Lorsque la configuration diffère des valeurs enregistrées, `my serve` consigne `start failed` avec les deux paires de valeurs et se termine avec le code 1, et les commandes affichent la même erreur. Les exceptions sont `my schema apply` et `my db backup`, qui ne vérifient pas, et le mode maintenance, qui ne lit pas la base de données. `my config check -instance` et `my db upgrade -check` vérifient mais n’enregistrent rien.
Un processus qui n’a pas pu lire la base de données au démarrage n’est pas prêt tant qu’il ne le peut pas, et d’ici là il n’exécute aucune tâche et répond `503` avec `Retry-After` à toute requête sauf les trois contrôles de santé ci-dessous (`/healthz`, `/readyz`, `/api/v1/instance/health`). C’est aussi ce que fait un serveur démarré avant `my schema apply`. S’il trouve alors des valeurs différentes, il s’arrête avec la même erreur.

Dans le profil `production`, `my config check` vérifie aussi que les paramètres valides conviennent à un serveur de production : les erreurs, comme un `host` que les autres serveurs ne peuvent pas joindre ou une KEK qui n’est pas aléatoire, le font se terminer avec le code 1, et les avertissements, comme `mail = none`, non.
`my serve` consigne les deux comme avertissements au démarrage mais démarre quand même ; exécutez donc `my config check` comme étape du déploiement.
La [liste de contrôle de production](production-checklist.md) énumère les vérifications et ce qu’il faut contrôler à la main avant la mise en service.

| `role` | HTTP | Worker de tâches |
| --- | --- | --- |
| `all` (par défaut) | Toutes les routes | S’exécute |
| `web` | Toutes les routes | Ne s’exécute pas |
| `worker` | Contrôles de santé uniquement (pour les sondes de la plateforme) | S’exécute |

Les journaux vont vers la sortie d’erreur standard au format JSON Lines. Chaque ligne a `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) et `severity`, le même niveau sous la forme que lit Cloud Logging (`DEBUG`, `INFO`, `WARNING`, `ERROR`, et `CRITICAL` pour une erreur qui arrête le processus).
Les journaux écrits pendant le traitement d’une requête portent un `request_id`, et le même ID est renvoyé dans l’en-tête de réponse `X-Request-Id`.
Si le `X-Request-Id` de la requête fait au plus 128 caractères parmi des lettres, des chiffres et `-_.:`, il est utilisé tel quel, comme l’ID attribué par un répartiteur de charge.
Les journaux à l’intérieur d’une trace (requêtes et tâches) portent aussi `trace_id` et `span_id` (consultez [Traces](#traces)).

Chaque requête HTTP et chaque exécution de tâche écrit une ligne lorsqu’elle se termine. La ligne a les mêmes noms et valeurs que les étiquettes des [métriques](#metrics) ci-dessous ;
sur Cloud Run avec `metrics_exporter = none`, vous pouvez donc construire des métriques basées sur les journaux de Cloud Logging (compteurs et distributions) à partir de ces lignes.

| Ligne (`msg`) | Champs |
| --- | --- |
| `http request` (`INFO` ; les contrôles de santé utilisent `DEBUG`) | `http.request.method`, `http.route`, `my.surface` et `http.response.status_code` (un nombre) ont les mêmes valeurs que les étiquettes de `http.server.request.duration`. Aussi `duration_seconds` (un nombre de secondes), `url.path` (le chemin sans la chaîne de requête), `http.response.body.size` et `request_id`. `http.route` et `my.surface` sont absents pour les requêtes qui n’ont atteint aucune route (contrôles de santé, requêtes refusées avant le routage), et la métrique ne les compte pas. Une requête dont le client est parti avant que le serveur puisse répondre a le statut `499` au lieu d’un 5xx, sauf si un gestionnaire a paniqué ; le client n’a rien reçu, elle ne compte donc pas comme une erreur du serveur |
| `job` (`INFO` en cas de succès et pour `postponed`, `WARN` pour `retry`, `ERROR` pour `dead`) | `my.job.type` et `my.outcome` ont les mêmes valeurs que les étiquettes de `my.job.duration`. Aussi `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt`, et `error` en cas d’échec |
| `error report` (`ERROR` ; `CRITICAL` pour un échec qui arrête le processus) | Une ligne par erreur signalée ([Rapports d’erreurs](#error-reports)) |

<!-- source: error-reports 3d780744461c -->
### Rapports d’erreurs

Le guide [Signalement des erreurs](error-reporting.md) indique où lire ces lignes sur chaque plateforme, l’écran Erreurs, les notifications et l’envoi des rapports vers un outil de suivi des erreurs.

Le serveur signale les erreurs qui requièrent l’attention de l’opérateur sous la forme d’une ligne `error report` chacune :

- une panique dans un gestionnaire HTTP, une tâche ou une tâche d’arrière-plan ;
- une erreur derrière une réponse 5xx de l’API client, de l’API d’administration, des pages de connexion, d’ActivityPub, d’OAuth ou des médias ;
- une tâche qui échoue définitivement (`dead`) : elle a épuisé ses nouvelles tentatives, ou a refusé de s’exécuter de nouveau. Les remises abandonnées parce que l’autre serveur ne cessait d’échouer ne sont pas signalées ;
- un échec au démarrage : une configuration invalide, ou une base de données ou un adaptateur qui ne peut pas être configuré.

Les réponses 4xx, les nouvelles tentatives de tâches, le mode maintenance, les contrôles de disponibilité échoués, les requêtes dont le client est parti et les échecs causés par d’autres serveurs ne sont pas signalés ; les métriques et les autres lignes de journal les couvrent.

Lorsque le client d’une requête s’en va, les autres lignes d’erreur du serveur pour cette requête sont écrites en `INFO` au lieu de `ERROR`, puisque la plupart d’entre elles indiquent seulement que la requête a été annulée.

La ligne est dans la forme que lit Google Cloud Error Reporting : sur Cloud Run, les rapports apparaissent dans Error Reporting regroupés comme des erreurs sans aucune configuration. Sur les autres plateformes, c’est une simple ligne JSON :

| Champ | Valeur |
| --- | --- |
| `@type`, `serviceContext` | Le type d’événement Error Reporting, et `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 chiffres hexadécimaux identiques pour chaque occurrence de la même erreur : construits à partir de la source, de l’endroit, du type d’erreur et des fonctions du serveur dans la pile, sans numéros de ligne |
| `my.error.source`, `my.error.kind` | `server` (ou `web`), et `panic` ou `error` |
| `my.error.where` | Où cela s’est produit : `<surface> <method> <route>` pour une requête (comme `client_api GET /api/v1/statuses/{id}`), le type de tâche pour une tâche, le nom de la tâche pour une tâche d’arrière-plan, `startup` pour un échec au démarrage |
| `my.role`, `deployment.environment.name` | Le `role` du processus et `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Les ID permettant de retrouver les lignes de requête ou de tâche de la même erreur, lorsqu’elle en a |
| `message` | En dernier sur la ligne : `<type>: <message>`, un saut de ligne et la pile Go |

Avant qu’un rapport ne quitte le processus, le message voit les URL, adresses e-mail, identifiants `@user@host`, adresses IP, UUID et ULID, et nombres de 10 chiffres ou plus remplacés par `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` et `<n>`, et est tronqué à 1 Kio.
La pile conserve ses positions de fichier mais pas les valeurs d’arguments de ses lignes de fonction (elles sont affichées sous la forme `(...)`), et est tronquée à 16 Kio. Aucun rapport ne porte de corps de requête, d’en-tête, de requête, de cookie, de compte, de nom d’utilisateur, d’adresse IP ni de User-Agent.
Aucune autre ligne ne porte de pile ; Error Reporting ne trouve donc que les piles de ces lignes.

Un nom de champ est une clé unique qui contient `.` ; elle n’est pas imbriquée. Mettez-la entre guillemets dans les filtres et extractions de Cloud Logging (par exemple `jsonPayload."http.route"`, et `EXTRACT(jsonPayload.duration_seconds)` pour une valeur de distribution).
Les champs utilisés comme étiquettes ne prennent que des valeurs d’un ensemble fini ; `url.path`, `request_id` et `job_id` ne sont jamais des étiquettes.

Avec `metrics_exporter = emf`, ces deux lignes sont les lignes EMF elles-mêmes. La ligne `http request` porte la valeur de `http.server.request.duration`, et la ligne `job` porte les valeurs de `my.job.duration` et `my.job.wait` (clés nommées d’après les métriques),
ainsi que la directive `_aws`, `service.name`, `service.version` et `my.role`. Ces trois métriques ne sont écrites sur aucune autre ligne. Les champs que vous choisissez dans `metrics_emf_dimensions` sont écrits sous forme de chaînes, comme l’exige EMF (par exemple la dimension `http.response.status_code` vaut `"200"`).
Les journaux vont vers la sortie d’erreur standard et les autres lignes EMF vers la sortie standard. Le pilote de journaux `awslogs` d’ECS envoie les deux à CloudWatch Logs, de sorte que les métriques sont extraites des deux.

<!-- source: docker-compose-files df45a902f45b -->
## Fichiers Docker Compose

`deploy/docker/` contient ce qui déploie un serveur. `compose.yaml` s’exécute seul, et les autres fichiers s’y superposent.

| Fichier | Ajoute | À utiliser lorsque |
| --- | --- | --- |
| `compose.yaml` | Le serveur et ses services de base de données ponctuels, sur SQLite et des fichiers de médias dans le volume `data` | Toujours |
| `compose.postgres.yaml` | PostgreSQL dans un conteneur, dans le volume `postgres` | Vous voulez PostgreSQL sur cette machine. Pour un serveur PostgreSQL ailleurs, omettez-le et définissez `MY_DATABASE` et `MY_DATABASE_URL` dans `.env` |
| `compose.s3.yaml` | Les médias dans un bucket compatible S3 ; avec `COMPOSE_PROFILES=garage`, aussi un magasin [Garage](https://garagehq.deuxfleurs.fr/) à un nœud dans le volume `garage` | Les médias dépassent un disque, ou vous voulez les placer dans un bucket. Sans le profil, le magasin est l’externe des paramètres `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, avec des certificats TLS sur les ports 80 et 443 | `MY_HOST` est un domaine public et rien d’autre ne termine le TLS sur cette machine |
| `compose.build.yaml` | Construit l’image à partir d’un checkout | Vous exécutez votre propre build |

Choisissez la base de données et le stockage des médias avant le premier démarrage : les données ne se déplacent pas d’elles-mêmes lorsque les fichiers changent
([Sauvegarde](backup.md) indique ce qui doit être déplacé).

**Superposez les fichiers** avec des options `-f` dans cet ordre, ou nommez-les une fois dans `.env`, afin que chaque commande `docker compose` ultérieure
(`exec`, `logs`, `pull`, et celles des autres guides) utilise les mêmes fichiers :

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Un fichier nommé `compose.override.yaml` dans le répertoire n’est lu que lorsque ni `-f` ni `COMPOSE_FILE` ne nomment de fichiers. Avec
eux, listez-le aussi.

**Paramètres.** Copiez `example.env` vers `.env`. Compose lit `.env` pour remplir les variables de ses fichiers, et transmet tout le
fichier aux conteneurs du serveur ; chaque paramètre de la [référence de configuration](../reference/configuration.md) fonctionne donc en
étant défini là. Le modèle les liste tous, avec une phrase pour chacun.

- La clé de chiffrement de clés reste hors de `.env`, dans le fichier `secrets/kek`, et les conteneurs la lisent comme le secret Docker
  `/run/secrets/my_kek`, de sorte qu’elle ne devient jamais une variable d’environnement. Une ligne `MY_KEK` dans `.env` arrête le serveur, qui
  refuse `MY_KEK` et `MY_KEK_FILE` ensemble ([gestion de la clé](key-management.md)).
- Le serveur s’exécute sous l’utilisateur `65532`, et Compose monte le fichier avec les permissions qu’il a ; le fichier doit donc être
  lisible par les autres. Gardez-le dans un répertoire où vous seul pouvez entrer, comme le font les commandes de `example.env`.
- Les autres secrets parviennent aux conteneurs sous forme de variables d’environnement. Pour en transmettre un sous forme de fichier, montez-le avec un secret
  dans un `compose.override.yaml` qui vous appartient et définissez son `<NAME>_FILE`, comme le fait `compose.yaml` pour la clé.
- `.env` contient des secrets : `chmod 600 .env`, et gardez-en une copie avec vos sauvegardes.
- `MY_LISTEN` reste `:8080`, puisque les fichiers se connectent à ce port.

**TLS avec Caddy.** Définissez `MY_HOST` sur un domaine dont le DNS pointe vers la machine, avec les ports 80 et 443 joignables depuis
Internet, et retirez `MY_WEB_ORIGIN` de `.env`. Caddy obtient et renouvelle le certificat de lui-même à la première requête.
Le serveur fait confiance au `X-Forwarded-For` des plages d’adresses par défaut de Docker ; définissez `MY_TRUSTED_PROXIES` si les vôtres diffèrent.

**Le Garage intégré.** `compose.s3.yaml` avec `COMPOSE_PROFILES=garage` nécessite les valeurs que `example.env` liste en haut.
Le service ponctuel `garage-init` prépare le magasin à chaque `up`. Il reste à l’intérieur du réseau Compose, puisque le serveur
sert lui-même les médias. Un nœud est un disque : sauvegardez le volume `garage` ou le bucket ([Sauvegarde](backup.md#media-in-a-bucket)).

<!-- source: get-the-image 238d00ea9826 -->
## Obtenir l’image

L’image du serveur est `ghcr.io/mycroblog/my`, publique, pour `linux/amd64` et `linux/arm64`, avec tous les adaptateurs.

| Tag | Pointe vers | À utiliser pour |
| --- | --- | --- |
| `0.1.0` | Cette version, définitivement | Nommer une version |
| `0.1` | Le dernier correctif de `v0.1` | Docker Compose, qui applique le schéma à chaque `up` |
| `0.1.0-rc.1` | Cette préversion | Essayer une version avant sa sortie |

Il n’y a ni `latest` ni `0` : les deux franchiraient une version mineure, et chaque mise à niveau vers une nouvelle version mineure doit passer [par la précédente](database.md#skipping-releases). Nommez une version dans chaque commande ; `docker pull ghcr.io/mycroblog/my` seul échoue avec `manifest unknown`.
Une version corrective peut aussi nécessiter `my schema apply` et `my db upgrade` ; en dehors de Docker Compose, suivez donc [Mise à niveau](database.md#upgrading) pour chaque version. En production, épinglez le condensé que donnent les notes de version :

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

Où l’exécuter :

- **Docker Compose** : `deploy/docker/compose.yaml` exécute l’image de `MY_VERSION` dans `.env` ([Fichiers Docker Compose](#docker-compose-files)). Pour la construire plutôt à partir d’un checkout, ajoutez la surcharge `compose.build.yaml` (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) et définissez `MY_VERSION` sur un nom qui vous appartient, puisqu’il nomme votre image locale et est ce qu’affiche `my version`.
- **Kubernetes** : récupérez-la depuis `ghcr.io` telle quelle.
- **Cloud Run** : utilisez directement l’image `ghcr.io`. Un déploiement résout le tag en un condensé.
- **AWS (ECS, EKS)** : copiez l’image vers Amazon ECR, ou utilisez un pull through cache ECR, qui nécessite des identifiants ghcr.io.
- **Une image plus petite** : `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .` dans un checkout en construit une avec moins d’adaptateurs ([build tags](../reference/build-tags.md)). C’est votre propre build ; il n’est donc pas couvert par les vérifications ci-dessous.

<!-- source: verify-an-image b696c257662d -->
### Vérifier une image

Chaque image publiée a une provenance signée via Sigstore, qui lie le condensé au workflow et au commit qui l’ont construite, et un SBOM. Avec la [CLI GitHub](https://cli.github.com/), et connecté à `ghcr.io` si la commande le demande :

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

La première commande échoue sauf si l’image a été construite par le workflow de publication de ce dépôt. Elle affiche le commit, qui est celui du tag de la version ; l’étiquette de l’image `org.opencontainers.image.revision` désigne le même.

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
## Publier le code source (AGPL-3.0)

`my` est sous licence AGPL-3.0-only. La section 13 de l’AGPL-3.0 exige de quiconque exécute le serveur et laisse des utilisateurs interagir avec lui via un réseau
qu’il offre à ces utilisateurs un moyen d’obtenir le code source correspondant du programme en cours d’exécution. Cela s’applique à toute personne qui interagit avec le serveur, pas seulement à celles qui ont un compte.

Indiquez où le code source est publié avec le paramètre `source_url` (variable d’environnement `MY_SOURCE_URL` ; consultez la [référence de configuration](../reference/configuration.md#source_url)).

| Valeur | Signification |
| --- | --- |
| Non défini | La valeur par défaut, `https://github.com/mycroblog/my`. Utilisez-la lorsque vous exécutez `my` sans modification : le `GET /instance` d’une version désigne alors la source de cette version, `https://github.com/mycroblog/my/tree/v0.1.0`, et NodeInfo le dépôt |
| Une URL `https:` | Un endroit où le code source peut être lu (le dépôt d’un fork, le tag ou l’archive d’une version particulière, etc.) |
| Un URI `mailto:` | Une adresse e-mail à laquelle demander le code source (par exemple `mailto:source@example.org` ; `?subject=...` est autorisé) |

Toute autre valeur (`http:`, `git@...`, un chemin relatif, une URL avec des informations d’utilisateur ou un `#`, un `mailto:` sans adresse, etc.) fait échouer `my config check`, et le serveur ne démarre pas.

La valeur apparaît à ces endroits :

- `source_url` dans le `GET /instance` de l’API client
- La section « Code source » de la page d’informations du serveur du client web (`/about`) et un lien sur la page d’accueil (`/`)
- `metadata.sourceCode` dans NodeInfo (`/nodeinfo/2.1`). Une URL `https:` est aussi publiée sous `software.repository`
  (le schéma NodeInfo 2.1 n’autorise qu’une URL `http(s)` dans `software.repository` ; une valeur `mailto:` n’apparaît donc que dans `metadata.sourceCode`)

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### Lorsque vous forkez ou modifiez `my`

Si vous exécutez un `my` modifié (vous appliquez des correctifs, construisez un fork ou remplacez des dépendances), l’URL par défaut ne pointe plus vers le code source de ce qui s’exécute.
Remplacez `source_url` par l’endroit où l’on peut trouver le code source de la version en cours d’exécution, avec vos modifications. Pointer vers le commit, le tag ou l’archive qui correspond à la version en cours d’exécution, plutôt que vers l’état le plus récent d’un dépôt, permet aux utilisateurs d’obtenir de façon fiable le même code source.
Si vous exécutez une version officielle non modifiée, gardez la valeur par défaut.

<!-- source: when-you-use-mailto 11155a8b71ed -->
### Lorsque vous utilisez `mailto:`

- Soyez prêt à remettre rapidement le code source correspondant (y compris les scripts nécessaires pour le construire) lorsque quelqu’un le demande. Se contenter de nommer une adresse, sans être prêt à livrer, ne remplit pas l’obligation.
- Utilisez une adresse qui fonctionne encore après que vous cessez d’exploiter le serveur ou que la personne responsable change. Une adresse d’exploitation partagée vaut mieux qu’une boîte personnelle.
- Assurez-vous que le filtrage antispam ne supprime pas les demandes.
- Utilisez une adresse que vous acceptez de rendre publique : n’importe qui peut la voir sur la page d’informations, sur la page d’accueil et dans NodeInfo.

<!-- source: metrics 2dea3a7f8f83 -->
## Métriques

Le paramètre `metrics_exporter` (`none` par défaut) exporte les métriques du service. Ses paramètres associés figurent dans la [référence de configuration](../reference/configuration.md).

| `metrics_exporter` | Chemin | Adapté à |
| --- | --- | --- |
| `none` | Rien n’est exporté. L’instrumentation ne coûte presque rien | Cloud Run (utilisez plutôt les métriques de la plateforme et les métriques basées sur les journaux) |
| `prometheus` | `GET /metrics` sur `metrics_listen` (`127.0.0.1:9464` par défaut). C’est un écouteur distinct du `listen` public, sans authentification, et il s’exécute pour chaque `role` | Auto-hébergement (interrogé par un serveur compatible Prometheus ou KEDA) |
| `otlp` | Envoyé à un OpenTelemetry Collector via OTLP/HTTP. La destination provient des variables d’environnement OTel standard comme `OTEL_EXPORTER_OTLP_ENDPOINT` | Configurations où vous pouvez exécuter un collecteur |
| `emf` | Lignes JSON CloudWatch Embedded Metric Format. Les métriques de requêtes et de tâches voyagent sur les lignes de journal de clôture ci-dessus ; les autres sont écrites sur la sortie standard, une ligne par ensemble d’étiquettes, lorsqu’une requête ou une tâche se termine | AWS ECS |

Les métriques sont listées ci-dessous. Dans Prometheus, `.` dans un nom devient `_` et un suffixe d’unité (`_seconds`, et `_total` pour un compteur) est ajouté.

| Métrique | Type | Étiquettes |
| --- | --- | --- |
| `http.server.request.duration` | Histogramme, secondes | `http.request.method`, `http.route` (le chemin du motif de route ; `other` si aucun ne correspond), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Compteur | `activity.type` (un type d’Activity connu, ou `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (uniquement avec `rejected` : `signature` pour une signature ou une preuve d’intégrité manquante, invalide ou qui ne se vérifie pas, ou dont la clé n’a pas pu être récupérée ; `suspended` pour un domaine suspendu ; `federation_off` ; `rate_limited` ; `invalid` pour une activité sans `id`, `type`, `actor` ou `object` valide) |
| `my.relay.activities` | Compteur | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving` : si une activité transportée par un relais a été ingérée, et pourquoi elle a été abandonnée ; consultez [Relais de fédération](relays.md)) |
| `my.move.activities` | Compteur | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon` : si un `Move` reçu a été enregistré, et pourquoi il a été abandonné) |
| `my.delivery.attempts` | Compteur | `my.outcome` (`success` / `retry` / `gone` / `failed` ; `gone` couvre un 410 et les remises ignorées parce que le serveur est indisponible ou suspendu) |
| `my.delivery.duration` | Histogramme, secondes | `my.outcome` |
| `my.plugin.failures` | Compteur | `my.plugin.kind` (un type de [plugin](plugins.md)), `my.plugin.code` (le code Connect de l’échec) |
| `my.plugin.slot_wait` | Histogramme, secondes | `my.plugin.kind` (combien de temps un appel a attendu son tour parmi les appels de ce type qui s’exécutent simultanément) |
| `my.hook.decisions` | Compteur | `hook`, `mode`, `outcome` (les décisions qu’un [hook de décision](decision-hooks.md) a enregistrées) |
| `my.hook.failures` | Compteur | `hook`, `outcome` (`timeout`, `error`, `invalid` ou `breaker_open`) |
| `my.hook.duration` | Histogramme | `hook` (secondes par appel du plugin) |
| `my.job.duration` | Histogramme, secondes | `my.job.type` (un type de tâche enregistré, ou `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed` : `postponed` est une exécution qui n’a fait aucun travail et n’utilise aucune des tentatives de la tâche, comme la récupération d’une carte de lien qui attend son hôte) |
| `my.job.wait` | Histogramme, secondes | `my.job.type` |
| `my.link_card.host_pauses` | Compteur | `http.response.status_code` (`429` / `503`). Un par pause des récupérations de cartes de liens auprès d’un hôte ; consultez [Aperçus de liens](link-previews.md#limits-per-host) |
| `my.translation.requests` | Compteur | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Un par demande de traduction d’une publication que le lecteur peut voir, ou de la publication ou du commentaire d’un signalement qu’un modérateur demande : servie à partir des propres mots de l’auteur, d’une traduction conservée ou d’un fournisseur, ou refusée parce qu’aucun fournisseur ne peut recevoir la publication, qu’une limite mensuelle est atteinte, qu’aucun fournisseur ne traduit la langue, ou qu’aucun fournisseur n’a renvoyé de traduction exploitable. |
| `my.translation.characters` | Compteur | `my.provider` (l’ID d’un fournisseur de traduction). Les caractères des publications et des commentaires de signalements envoyés au fournisseur, sans marqueurs, y compris les appels qui échouent. C’est ce que comptent les [limites mensuelles](instance-settings.md#admin-screens-and-the-client-api), et cela peut différer de ce que le fournisseur facture |
| `my.timeline.delivery.lag` | Histogramme, secondes | Aucune (de la création d’une publication locale à son écriture dans les fils d’accueil et à la notification des flux) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` pour les flux des fils local et fédéré, `voice` pour les flux des salons vocaux) |
| `my.accounts.registered` | Compteur | Aucune |
| `my.statuses.created` | Compteur | `my.visibility` |
| `my.billing.imports` | Compteur | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (uniquement avec `failed` : le type d’échec, comme dans [Importer les frais d’utilisation du cloud](billing-import.md)). Chaque tentative compte, y compris les nouvelles tentatives et les échecs qui attendent l’import quotidien suivant |
| `my.voice.turn.credentials` | Compteur | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Un par identifiant TURN qu’un participant à un chat vocal a demandé en rejoignant un salon ou en renouvelant sa connexion : `reused` signifie que des identifiants Cloudflare émis plus tôt pour la même connexion ont été renvoyés, et `failed` signifie que Cloudflare n’a pas pu être joint ou a refusé, de sorte que le participant a reçu une erreur et n’a pas pu se connecter |
| `my.errors.exported` | Compteur | `my.outcome` (`success` / `dropped` / `failed` : un par rapport d’erreur pour le service de `errors_exporter`. `dropped` signifie que le rapport n’a pas été envoyé parce que 1 000 rapports attendaient déjà ou que le service a demandé au serveur de ralentir, et le serveur attend aussi longtemps que le service le demande avant de renvoyer ; `failed` signifie que le service n’a pas pu être joint ou a refusé le rapport. Un rapport est envoyé une fois et jamais retenté, et les rapports encore en attente 2 secondes après le début d’un arrêt sont perdus) |
| `my.queue.ready` | Jauge | `my.job.type` (le nombre de tâches exécutables dans la file de la base de données) |
| `my.queue.oldest_ready_age` | Jauge, secondes | `my.job.type` (depuis combien de temps attend la plus ancienne tâche exécutable de chaque type dans la file de la base de données ; un type sans tâche exécutable n’a pas de valeur) |
| `my.queue.dead` | Jauge | Aucune |
| `my.instance.users`, `my.instance.statuses` | Jauge | Aucune (les nombres d’utilisateurs et de publications locaux, comme dans NodeInfo) |
| `my.media.stored_bytes` | Jauge, octets | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Jauge, octets | Aucune (la taille de la base de données : `pg_database_size` pour PostgreSQL ; le fichier de base de données sans son journal d’écriture anticipée pour SQLite) |
| `my.billing.import.failing` | Jauge | `my.billing.source` (1 lorsque l’import le plus récent de la source a échoué, 0 sinon) |
| `my.federation.instances` | Jauge | `my.federation.instance.state` (`available` : serveurs vers lesquels les remises vont encore mais qui ont échoué au moins un jour depuis leur dernière réponse ; `unavailable` : serveurs dont les remises sont en pause) |
| `my.metrics.snapshot.age` | Jauge, secondes | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Compteur | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Un par [rapport d’erreur](#error-reports) |

Les contrôles de santé (`/healthz`, `/readyz`) ne sont pas mesurés. Aucune étiquette ne contient de domaine distant, de compte, d’URL ni d’adresse IP de client.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` et `my.billing.import.failing` sont des valeurs pour tout le serveur : chaque processus rapporte la même valeur à partir d’un instantané conservé dans le magasin clé-valeur.

<!-- source: system-metrics 8b5d40a4eb98 -->
### Métriques système

Les métriques sur le processus, le runtime Go et les connexions aux ressources sont exportées via le même `metrics_exporter`.
Le processeur, la mémoire, le nombre d’instances et les requêtes entrantes des conteneurs sont laissés aux métriques de la plateforme ; celles-ci couvrent la saturation à l’intérieur du processus que la plateforme ne peut pas voir.
Ne les utilisez pas pour déclencher la mise à l’échelle ; utilisez-les pour les alertes et pour estimer le plafond d’instances.

| Métrique | Type | Étiquettes |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Jauge, octets | Seul `go.memory.used` a `go.memory.type` (`stack` / `other`). `go.memory.limit` n’apparaît que lorsque `GOMEMLIMIT` est défini |
| `go.memory.allocated`, `go.memory.allocations` | Compteur | Aucune |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Jauge | Aucune |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histogramme, secondes | Aucune (l’attente entre le moment où il devient exécutable et celui où il s’exécute, et les pauses du GC) |
| `go.memory.gc.cycles` | Compteur | Aucune |
| `process.cpu.time` | Observable Counter, secondes (total depuis le démarrage) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Jauge, octets | Aucune (mémoire résidente, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Jauge | Aucune (descripteurs de fichiers ouverts et limite) |
| `process.uptime` | Jauge, secondes | Aucune |
| `db.client.connection.count` | Jauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Jauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (total depuis le démarrage ; secondes ou nombre) | `db.client.connection.pool.name`. `timeouts` n’existe que pour PostgreSQL |
| `my.adapter.operation.duration` | Histogramme, secondes | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (connexions `valkey` et abonnés `stream.<stream>`) |
| `my.adapter.reconnects` | Compteur | `my.adapter` |
| `http.client.request.duration` | Histogramme, secondes | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`), `http.request.method`, `my.http.status_class` (`2xx`, etc.), `error.type` (`forbidden_destination` est un refus de la protection SSRF) |
| `http.client.open_connections` | UpDownCounter | Aucune |
| `my.readiness.failures` | Compteur | Aucune |
| `my.http.origin_refused` | Compteur | Aucune. Un par requête refusée parce qu’elle n’est pas passée par le CDN ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Les sondes de l’adresse de l’origine l’incrémentent lentement ; un saut pendant que le trafic s’arrête signifie que le CDN n’envoie plus l’en-tête. Le mode maintenance ne mesure rien, seul son journal d’accès les montre |

- Les métriques du runtime Go (`go.*`) ne sont exportées que par `prometheus` et `otlp`. Avec `emf`, une tâche d’instantané périodique écrit les jauges et compteurs observables du processus et du pool de connexions.
- Les attentes du pool et le temps processeur sont des Observable Counters : le pool et le noyau conservent des totaux depuis le démarrage, et ils sont lus au moment de la collecte. Dans Prometheus, ce sont des compteurs `_total` (comme `process_cpu_time_seconds_total`), dans OTLP ce sont des sommes monotones, et vous voyez leur croissance avec `rate()` de PromQL.
  `emf` n’a pas de compteurs totaux ; il écrit donc l’augmentation depuis l’écriture précédente (additionnez-les avec la statistique Sum de CloudWatch).
- `claim` de la file n’est mesuré que lorsqu’il prend une tâche ou échoue. L’interrogation d’un worker en attente n’enregistre rien.
- `db.client.connection.pending_requests` n’est pas exporté : les statistiques de pgx et de `database/sql` ne rapportent pas combien de requêtes attendent ; le nombre et la durée des attentes en tiennent donc lieu.
- Alertez sur le ratio de la somme de `go.memory.used` à `go.memory.limit`, et sur la croissance de `db.client.connection.wait_time` (la limite du pool est atteinte).

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### Limite de mémoire (`GOMEMLIMIT`)

`GOMEMLIMIT` est une variable d’environnement du runtime Go que l’opérateur définit ; l’application ne la choisit pas.
Sans elle, Go s’exécute avec sa valeur par défaut (aucune limite), et le tas ne se vide pas de façon anticipée à l’approche de la limite de mémoire du conteneur.
Sur les configurations avec une limite de mémoire de conteneur (Cloud Run, ECS, Kubernetes), définissez-la à environ 90 % de cette limite (par exemple `GOMEMLIMIT=460MiB` pour un conteneur de 512 Mio).
Ne définissez pas `GOMAXPROCS` : la valeur par défaut de Go suit la limite de processeur du conteneur.

<!-- source: traces e274793f5885 -->
## Traces

Le paramètre `traces_exporter` (`none` par défaut) exporte les traces distribuées. Ses paramètres associés figurent dans la [référence de configuration](../reference/configuration.md).

| Span | Nom | Attributs |
| --- | --- | --- |
| Requête HTTP (serveur) | `<method> <http.route>` (`<method>` pour une requête qui n’a atteint aucune route) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Un 5xx est un échec |
| Exécution de tâche (consommateur) | Le type de tâche (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Chaque nouvelle tentative est un span distinct |
| Transaction | `my.db.transaction` | `db.system.name`. Chaque nouvelle tentative est un span distinct |
| Requête (client) | Le nom de la requête sqlc (comme `InsertJob` ; sinon le premier mot du SQL, comme `BEGIN`) | `db.system.name`, `db.operation.name` |
| Remise (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| HTTP sortant (client) | `<method>` | `http.request.method`, `server.address` (le nom d’hôte de destination uniquement), `http.response.status_code`. Un 4xx ou 5xx est un échec |

Un span échoué a `error.type` (un nom fini tel que `timeout`, ou un code de statut). Les attributs ne contiennent jamais d’ID de compte, de publication ou de tâche, d’URL, de chemins ou de requêtes, d’adresses IP de clients, de valeurs User-Agent, de texte SQL ni de chaînes d’erreur.
`server.address` laisse les noms d’hôte des serveurs distants au récepteur ; vous décidez donc combien de temps le récepteur les conserve et qui peut les consulter. Les contrôles de santé et les requêtes refusées par `origin_secret` ne créent aucun span.

- Une exécution de tâche n’est pas un enfant du span qui l’a mise en file d’attente (généralement une requête HTTP). C’est la **racine d’une nouvelle trace** avec un lien vers le span de mise en file d’attente. Dans un visualiseur, suivez le lien depuis la trace de la requête.
- Un `traceparent` entrant ne devient le parent que si le pair qui se connecte figure dans `trusted_proxies` et, avec [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), si la requête porte le secret. Sinon, il est écarté et une nouvelle trace commence. `tracestate` est toujours écarté. Le HTTP sortant (fédération, WebFinger, médias, Web Push, etc.) ne porte aucun `traceparent`.
- Échantillonnage : une requête est enregistrée si son parent de proxy de confiance l’a été, ou si la cible du lien d’une tâche l’a été ; sinon elle est enregistrée au ratio `traces_sample_ratio` (0,01 par défaut). Abaisser le ratio ne laisse jamais sans leur trace les tâches qui prolongent une requête enregistrée.

| Cible | Paramètres | Chemin |
| --- | --- | --- |
| Par défaut, petit auto-hébergement | `traces_exporter = none` | Aucun span n’est exporté. Suivez `request_id` et `job_id` dans les journaux. Si un proxy de confiance fournit un `traceparent`, son `trace_id` et son `span_id` sont ajoutés aux journaux |
| Auto-hébergé (Jaeger, Tempo, un OpenTelemetry Collector, etc.) | `traces_exporter = otlp`, `traces_flush = batch` | Envoyé via OTLP/HTTP au récepteur à `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (ou `OTEL_EXPORTER_OTLP_ENDPOINT`, `http://localhost:4318` par défaut). Les en-têtes d’authentification vont dans `OTEL_EXPORTER_OTLP_HEADERS`. Pour conserver de façon fiable les erreurs et les traces lentes, utilisez l’échantillonnage tail du Collector |
| Google Cloud (Cloud Run) | `none`, ou `otlp` avec `unit` pour activer les traces | Voir ci-dessous |
| AWS (ECS) | `none`, ou `otlp` avec `batch` pour activer les traces | Voir ci-dessous |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

Même avec `none`, la trace de requête que Cloud Run crée automatiquement et le `trace_id` dans les journaux partagent le même ID (lorsque vous faites confiance avec `trusted_proxies` à la connexion en amont de Cloud Run).
Le fait que Cloud Run écrase toujours le `traceparent` des requêtes publiques n’est pas encore vérifié.
Les journaux n’ont pas la forme du champ de trace que Cloud Logging reconnaît (`logging.googleapis.com/trace`).

Pour envoyer les spans de l’application à Cloud Trace, exécutez un **OpenTelemetry Collector en sidecar** dans le même service. Le serveur ne détient aucun identifiant Google ; le sidecar envoie au point de terminaison OTLP de Cloud Trace avec les Application Default Credentials.

- Serveur (le conteneur d’entrée) : `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar : un OpenTelemetry Collector qui reçoit avec un récepteur `otlp` en HTTP sur `localhost:4318` et envoie avec l’exportateur `googlecloud`, ou l’exportateur `otlphttp` (`https://telemetry.googleapis.com`, avec l’extension `googleclientauth`). Accordez au compte de service la permission d’écrire dans Cloud Trace.
- À la fin de chaque requête ou tâche enregistrée, `unit` vide les spans accumulés vers le sidecar avant de retourner. Pour que le sidecar puisse les envoyer alors qu’aucun processeur n’est alloué, utilisez la facturation par instance pour une configuration avec traces activées.
- Le coût est celui des ressources du sidecar et du volume de spans (tarifs de Cloud Trace). Choisissez le ratio avec `traces_sample_ratio`.

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

Pour envoyer vers X-Ray, exécutez l’**AWS Distro for OpenTelemetry (ADOT) Collector** comme conteneur sidecar dans la tâche. Le serveur n’a pas de signature SigV4 ; le Collector signe et envoie au point de terminaison OTLP de X-Ray (`https://xray.<region>.amazonaws.com/v1/traces`).

- Serveur : `MY_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. Dans la configuration du Collector, recevez avec un récepteur `otlp` sur `localhost:4318` et envoyez à X-Ray avec un exportateur `otlphttp` qui utilise l’extension `sigv4auth`.
- Accordez au rôle de la tâche la permission d’écrire dans X-Ray.
- Le coût est la part du sidecar dans les ressources de la tâche, et le nombre de traces que X-Ray enregistre (les 100 000 premières par mois sont gratuites).

<!-- source: stop-the-server 37d4c0cc3d97 -->
## Arrêter le serveur

Sur SIGTERM ou SIGINT, le serveur s’arrête proprement.

1. Il cesse d’accepter de nouvelles connexions et de prendre des tâches.
2. Il attend jusqu’à 25 secondes que les requêtes en cours et les tâches en cours d’exécution se terminent.
3. Après 25 secondes, il annule le contexte des requêtes et des tâches restantes et ferme les connexions.
   Il laisse ensuite à une tâche annulée jusqu’à 3 secondes de plus pour enregistrer son résultat, et se termine avec le code 1 (28 secondes au plus en tout, dans le délai de grâce par défaut de 30 secondes de Kubernetes et d’ECS).
   Une tâche annulée est retentée. Une tâche qui ne s’arrête pas dans ces 3 secondes, ou qui est tuée en cours de route, est abandonnée et reprise par un worker lorsque son bail expire ; elle peut donc s’exécuter deux fois.

Une panique dans l’une des tâches d’arrière-plan du serveur déclenche le même arrêt, afin que le serveur ne continue jamais de s’exécuter dans un état défectueux.
La ligne de journal `serve failed` nomme la tâche, et le processus se termine avec le code 1 pour que la plateforme ou le gestionnaire de processus le redémarre.

Pour un travail sur la base de données, le magasin clé-valeur ou la file, redémarrez en mode maintenance (`MY_MAINTENANCE=true`) plutôt que de l’arrêter. Consultez [Maintenance](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## La file de tâches et les tâches échouées (mortes)

[Tâches](../reference/jobs.md) liste chaque type de tâche : ce qu’elle fait, ce qui la déclenche, comment elle est retentée et ce que fait son exécution répétée.

La page d’administration des tâches (`/admin/jobs`, permission `manage_jobs`) comporte trois pages :

- **État de la file** (`/admin/jobs`) : le nombre par état et par type et état, combien de temps chaque type fait attendre une tâche, et depuis combien de temps attend la plus ancienne tâche en attente.
- **Tâches** (`/admin/jobs/list`) : un tableau des tâches dans un état (en attente, en attente d’exécution, en cours, en attente de nouvelle tentative, échouée), une page à la fois, avec le type, l’état, l’heure prévue ou d’échec et le nombre de tentatives. Les tâches échouées ont **Relancer**.
- **Une tâche** (`/admin/jobs/<id>`, depuis le type dans le tableau) : tous les faits de la tâche, sa dernière erreur, ses clés et sa charge utile, l’historique des tentatives infructueuses (résultat, erreur, worker, et heures de début et de fin de chaque tentative), et **Relancer** pour une tâche échouée.

L’API d’administration est `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` et `POST /api/v1/admin/jobs/{id}/retry`.
Une tâche terminée n’est plus dans la file ; sa page est donc introuvable.

- Les tâches en attente, en cours d’exécution et en attente de nouvelle tentative ne peuvent être listées que pour les files dont le serveur détient les tâches : `queue = db`, `queue = cloudtasks` (les tâches sont aussi dans la base de données) et `queue = inline`. Les tâches échouées et l’historique des tentatives se trouvent dans la base de données de l’application pour toutes les files ; elles peuvent donc toujours être listées.
- L’historique des tentatives ne conserve que les tentatives infructueuses (les nouvelles tentatives et la tentative qui a finalement échoué). Suivez une exécution réussie via la ligne de journal structurée `job` (qui porte `job_id`).
  L’historique est supprimé par le nettoyage horaire une fois que `job_attempt_retention` (30 jours par défaut ; consultez le [paramètre](../reference/configuration.md#job_attempt_retention)) s’est écoulé depuis la fin de la tentative.
  L’historique ne contient ni charge utile ni journal de gestionnaire, et un message d’erreur est tronqué à ses 2 000 premiers octets. Les gestionnaires de tâches gardent les adresses e-mail, les adresses IP, les jetons et les corps de publications hors des messages d’erreur.
  Pour une vue limitée aux types de tâches d’e-mail, consultez [Envoi d’e-mails](email.md).

Une tâche dont le gestionnaire a décidé qu’elle a échoué définitivement, et une tâche qui a épuisé ses nouvelles tentatives, devient une tâche morte dans la file de la base de données (`queue = db` et `queue = cloudtasks`).
`my job` les liste, les relance et les supprime. L’état « Échec » de la page des tâches et l’API d’administration peuvent aussi les lister et les relancer.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Si une tâche ayant la même clé d’idempotence attend déjà, cette tâche tient lieu de celle qui est relancée. Les gestionnaires de tâches sont idempotents ; remettre la même tâche en file un nombre quelconque de fois est donc sans danger.
- Un ID inconnu est affiché et la commande se termine avec le code 1 ; les autres ID sont tout de même traités. Le code de sortie 2 signale une erreur d’utilisation.
- Une tâche morte que personne ne relance est supprimée par le nettoyage horaire une fois que `dead_job_retention` (30 jours par défaut ; consultez le [paramètre](../reference/configuration.md#dead_job_retention)) s’est écoulé depuis l’échec.
- Les tâches mortes de `queue = inline` vivent dans la mémoire du processus du serveur ; `my job` ne peut donc pas les gérer (code de sortie 1).

<!-- source: splitting-workers-by-job-type 89ced36a3c80 -->
### Répartir les workers par type de tâche

Par défaut, chaque worker exécute chaque type de tâche, dans l’ordre où les tâches sont arrivées à échéance, une tâche à la fois par processus (consultez [Exécuter plusieurs tâches à la fois](#running-several-jobs-at-once)). Une rafale de remises retarde alors les e-mails et les notifications, et tout worker que vous ajoutez partage son temps entre tous. Avec `queue = db`, vous pouvez donner à certains types leurs propres workers :

- [`worker_job_types`](../reference/configuration.md#worker_job_types) : ce processus n’exécute que ces types.
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except) : ce processus exécute tous les types sauf ceux-ci.

Chacun est une liste séparée par des virgules de [types de tâches](../reference/jobs.md) (`my job types` les affiche ; `my config check` refuse un nom qui n’en correspond à aucun), écrits sous forme de nom (`federation.deliver`) ou de préfixe (`federation.*`). Définissez-les par processus, par exemple avec `MY_WORKER_JOB_TYPES`. Vous ne pouvez pas définir les deux sur un même processus. Un nom ou un préfixe qui ne correspond à aucun type de tâche arrête le démarrage, ce qui détecte aussi un type qu’une version a renommé, ou un type qui n’existe qu’avec un paramètre que vous n’avez pas défini. Ils ne fonctionnent pas avec `role = web`, ni avec `queue = cloudtasks` ou `inline`, où le démarrage échoue aussi : là, un signal nomme la tâche à exécuter, de sorte qu’un processus ne peut pas choisir ses tâches.

Au démarrage, un processus doté de l’un des paramètres consigne les types qu’il ignore. Il ne peut pas voir les paramètres des autres processus ; **assurez-vous donc qu’un processus exécute chaque type**, faute de quoi ces tâches attendent indéfiniment. Le nettoyage périodique et l’e-mail sont aussi des types de tâches. Chaque processus démarre tout de même les tâches périodiques ; le processus qui les exécute dépend de ces paramètres.

Décidez de ce qu’il faut répartir d’après les métriques, pas d’après les noms de types. `my.job.wait` (Prometheus : `my_job_wait_seconds`) indique combien de temps les tâches ont attendu par type avant de démarrer :

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` indique le temps de worker qu’utilise chaque type, ce qui correspond aussi à peu près au nombre de processus dont il a besoin. Un type qui utilise la majeure partie du temps et fait attendre les autres est celui à déplacer.

Trois dispositions courantes, avec le nombre de processus de votre choix :

| Disposition | Processus |
| --- | --- |
| Empêcher les remises de retarder le reste | `worker_job_types = federation.fanout,federation.deliver` pour les remises, et `worker_job_types_except = federation.fanout,federation.deliver` pour tout le reste. Ajoutez des processus au premier groupe pendant une rafale. |
| Empêcher un moteur de recherche de retarder le reste | `worker_job_types = search.*` pour les tâches de recherche, et `worker_job_types_except = search.*` pour tout le reste ([Empêcher la recherche de retarder les autres tâches](search.md#keeping-search-from-delaying-other-jobs)). |
| Garder les e-mails et les notifications rapides | `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started` pour ceux-ci, et un worker sans paramètre pour tout, ce qui aide aussi pour ces types lorsqu’il est libre. Omettez les types dont vous n’avez pas défini les paramètres, faute de quoi le démarrage s’arrête : `notification.push` nécessite `vapid_private_key`, et `notification.email`, `notification.moderator_email` et `notification.supporter_email` nécessitent `mail`. Consultez [Tâches qui dépendent de paramètres](../reference/jobs.md#jobs-that-depend-on-settings). |

Un préfixe tel que `identity.*` est le module de code qui possède les types, pas un type de travail. `identity.*` prend aussi les exports de données, les imports et les tâches d’expiration, et il manque `instance.notify_announcement` et `voice.notify_started` ; une disposition pour un type de travail nomme donc chaque type.

Gardez-les ensemble, sur des workers ayant le même paramètre. Les tâches d’une même clé d’ordre s’exécutent dans l’ordre quel que soit leur type ; si rien n’exécute la tâche précédente, la suivante attend aussi, et si le groupe de la tâche précédente est engorgé, la tâche suivante l’attend :

| Types | Ce qu’ils maintiennent dans l’ordre |
| --- | --- |
| `federation.fanout`, `federation.deliver` | Les remises d’un J’aime et de son annulation |
| `voice.apply_media_event`, `voice.apply` | Les événements d’un même salon vocal |
| `supporters.sync`, `supporters.command` | Les modifications d’un même abonnement |

Lors d’une mise à niveau :

- Un processus doté de `worker_job_types` n’exécute pas les types qu’une nouvelle version ajoute. Consultez les notes de version et les types ignorés dans le journal. Un processus doté de `worker_job_types_except` les exécute, ce qui en fait l’endroit le plus sûr pour tout le reste.
- Un type renommé arrête le démarrage ; lisez donc d’abord les notes de version.

Pour déployer une répartition, démarrez d’abord les nouveaux workers, puis ajoutez le paramètre aux anciens. Pour l’annuler, retirez le paramètre d’un groupe avant d’arrêter l’autre. Les tâches restent dans la file, rien n’est donc perdu, et un type qu’aucun processus n’exécute pendant un moment ne fait qu’attendre.

**Détecter un type qu’aucun processus n’exécute.** Les tâches d’un tel type attendent ; `my.queue.oldest_ready_age` croît donc pour ce type, et les alertes `MyQueueBacklog` et `MyQueueStalled` se déclenchent sur le type qui attend le plus longtemps. Pour voir lequel, ou pour alerter par type :

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

La page d’administration des tâches montre la même attente par type. Un worker doté de l’un des paramètres laisse aussi de côté une tâche dont le type n’a aucun gestionnaire (elle compte sous `other` dans la métrique) ; sans worker dépourvu de paramètre, une telle tâche attend donc au lieu de devenir une tâche morte.

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### Exécuter plusieurs tâches à la fois

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) définit combien de tâches un processus exécute à la fois, de 1 à 128. Il fonctionne avec `queue = db` ; avec Cloud Tasks, la plateforme en décide. La valeur par défaut est une tâche à la fois, et un paramètre de 1 se comporte de la même manière.

Augmentez-le pour les tâches qui passent leur temps à attendre d’autres serveurs, comme `federation.deliver`. Elles utilisent peu de mémoire et peu de connexions à la base de données par tâche ; un seul processus fait donc le travail de plusieurs : lors d’un test local où une tâche n’attendait qu’une réponse pendant 200 ms, un processus avec 32 emplacements égalait 32 processus d’un emplacement chacun, en utilisant une petite partie de la mémoire. Ces tâches ne faisaient rien d’autre ; mesurez donc votre propre serveur. Associez-le à `worker_job_types`, et donnez à chaque type de tâche un processus avec son propre nombre.

Gardez ces limites à l’esprit :

- **Connexions à la base de données** : une tâche a besoin d’une connexion pour être réclamée, pour son propre travail et pour enregistrer son résultat. Avec `database = postgres`, le paramètre doit être inférieur au maximum du pool, faute de quoi le serveur ne démarre pas. Le maximum est `pool_max_conns` dans `database_url`, par défaut le plus grand de 4 et du nombre de processeurs ; un vCPU en autorise donc 3 tant que vous ne l’augmentez pas. Vérifiez que votre base de données ou votre gestionnaire de connexions accepte les connexions de tous vos processus.
- **SQLite** : il écrit une transaction à la fois. Au-delà de 32, le serveur consigne un avertissement, et lors d’un test local SQLite était plus lent à 128 qu’à 32.
- **Tâches gourmandes en processeur** : les emplacements partagent le processeur et la mémoire d’un processus. `media.process` attend son tour à une limite de la moitié des processeurs par défaut, et l’attente compte dans le bail de 5 minutes de la tâche ; un processus rempli de `media.process` peut donc dépasser le bail et recommencer. Donnez à de telles tâches un processus avec peu d’emplacements.
- **Un serveur à la fois** : un processus établit au plus 8 connexions vers un même hôte ; un processus ne remet donc à un serveur donné pas plus vite que ne le permettent 8 connexions, quel que soit son nombre d’emplacements. Lorsque la plupart des remises vont à quelques grands serveurs, davantage de processus aident là où davantage d’emplacements ne le font pas.
- **Un serveur bloqué retient des emplacements** : les emplacements qui attendent un serveur lent ne sont pas libres pour d’autres tâches ; séparez donc les remises des e-mails et des notifications.
- **Les déploiements consomment des tentatives** : lorsque le serveur s’arrête, les tâches encore en cours après `ShutdownTimeout` (25 secondes par défaut) sont annulées, et chaque tâche annulée consomme une tentative. Plus les emplacements exécutent de tâches lentes, plus chaque déploiement consomme de tentatives. Une tâche annulée dont le résultat n’est pas enregistré à temps attend l’expiration de son bail avant de s’exécuter de nouveau.
- **Un plantage emporte tous les emplacements** : si le processus manque de mémoire, toutes ses tâches attendent l’expiration de leurs baux.

Pour décider s’il faut l’augmenter, lisez `my.job.wait` par `my.job.type` : si les tâches d’un type attendent longtemps alors que `my.job.duration` est surtout du temps passé à attendre d’autres serveurs et que le processeur et la mémoire restent bas, davantage d’emplacements devraient aider. Si les attentes restent élevées après l’avoir augmenté, examinez les connexions à la base de données et la limite par hôte ci-dessus avant de l’augmenter de nouveau.

Tant qu’aucune tâche n’est due, un emplacement interroge et les autres attendent ; un processus inactif coûte donc à la base de données ce que coûte un processus à une seule tâche.

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
### Attendre une connexion à la base de données

Avec `database = postgres`, chaque instruction et chaque transaction attend d’abord une connexion libre du pool du processus.
[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (`60s` par défaut) borne cette attente, et rien d’autre : une longue requête ou
transaction qui a déjà sa connexion se poursuit, quelle que soit sa durée. `0` attend sans limite. SQLite n’a aucune limite de connexions à attendre ; le définir là fait donc échouer le démarrage.

Lorsque l’attente expire :

- Une requête reçoit la réponse `503 Service Unavailable` avec `Retry-After: 5`.
- Une tâche s’exécute de nouveau 30 secondes plus tard et ne consomme aucune tentative ; une base de données occupée ne transforme donc jamais une tâche en tâche morte.
- La métrique `db.client.connection.timeouts` la compte, et l’alerte [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) se déclenche.

La valeur par défaut est le délai d’écriture de 60 secondes du serveur : une requête qui a attendu plus longtemps ne pourrait de toute façon pas envoyer sa réponse ; la valeur par défaut ne fait donc échouer aucune requête qui aurait encore pu réussir.
Choisissez une valeur d’après le fonctionnement de votre serveur :

- **Abaissez-la** pour délester rapidement : les clients réessaient après `Retry-After` au lieu de garder leurs requêtes ouvertes. Gardez-la supérieure à vos transactions normales les plus lentes, faute de quoi une courte rafale se met à recevoir des `503`.
- **Dimensionnez d’abord le pool.** Un processus a besoin d’à peu près autant de connexions qu’il exécute de travaux à la fois. `pool_max_conns` dans `database_url` vaut par défaut le plus grand de 4 et du nombre de processeurs, et tous les processus ensemble doivent rester sous la limite de connexions de la base de données.
  - Avec [`worker_job_concurrency`](#running-several-jobs-at-once), les emplacements attendent aussi des connexions, et le paramètre doit rester inférieur au maximum du pool.
  - Sur Cloud Run, un service accepte 80 requêtes simultanées par instance par défaut. Avec un pool de 4, une rafale de requêtes lentes fait la queue ici ; augmentez donc `pool_max_conns`, abaissez la simultanéité du service, ou les deux.
- **Des `503` fréquents ou des `db.client.connection.timeouts`** signifient que le pool est trop petit ou que les requêtes sont devenues plus lentes ; un délai plus long ne fait que le masquer. Suivez [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[Déployer sur Cloud Run](deploy-cloud-run.md) configure tout ce qui suit avec des modules OpenTofu.

Avec `queue = cloudtasks`, les tâches restent dans la base de données (`database = postgres`), et Google Cloud Tasks ne fait que réveiller le serveur pour chacune d’elles :

- Pour chaque tâche qu’il stocke, retente ou remet en file, le serveur crée une tâche dans `queue_cloudtasks_queue` une fois la transaction validée. La tâche appelle
  `POST {queue_cloudtasks_url}/_internal/jobs` à l’heure d’échéance de la tâche, avec l’ID de la tâche comme corps et un jeton d’ID OIDC de
  `queue_cloudtasks_service_account`. Une tâche due dans plus de 30 jours est signalée à 30 jours, et de nouveau lorsque cette tâche arrive.
- Le processus qui reçoit la tâche exécute la tâche et répond `204` une fois son résultat enregistré, y compris lorsque la tâche a déjà été exécutée ou n’est pas due.
  Une tâche qui échoue est retentée par le serveur à sa prochaine heure de tentative, pas par Cloud Tasks. Cloud Tasks ne retente une tâche que lorsque la
  réponse est `5xx` (la base de données n’a pas pu être jointe) ou `401` (un jeton que le serveur a refusé) ; son `retryConfig` n’a donc besoin d’aucun paramètre.
- Aucun processus n’interroge la table et aucun processus n’exécute de lui-même les tâches périodiques. Créez une tâche Cloud Scheduler qui appelle
  `POST {queue_cloudtasks_url}/_internal/tick` chaque minute (`* * * * *`) avec un jeton OIDC du même compte de service (et l’audience de
  `queue_cloudtasks_audience`, lorsqu’elle est définie). Chaque tick met en file les tâches périodiques dont la période est arrivée, une fois pour tout le serveur (les périodes sont réclamées dans le KV,
  `kv` doit donc être `db` ou `valkey`), et signale de nouveau les tâches dues depuis plus de 2 minutes sans s’être exécutées, ce qui récupère un signal perdu.
- `/_internal/jobs` et `/_internal/tick` sont servis par les processus avec `role = all` ou `worker`, et n’acceptent qu’un jeton signé par Google pour
  `queue_cloudtasks_service_account` avec une adresse e-mail vérifiée, dont l’audience est `queue_cloudtasks_audience` ou, par défaut, l’URL appelée. Toute autre requête reçoit un `401`.
- Une remise exécute une tâche, pour au plus le bail de 5 minutes. Gardez `queue_cloudtasks_dispatch_deadline` (10 minutes par défaut) et le délai d’attente des requêtes du
  service Cloud Run supérieurs à cette durée.
- Le compte de service du serveur lui-même a besoin de `roles/cloudtasks.enqueuer` sur la file et de `roles/iam.serviceAccountUser` sur `queue_cloudtasks_service_account`.
  Il s’authentifie avec les Application Default Credentials.

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
### Acheminer des types de tâches vers leurs propres files

Avec `queue = cloudtasks`, un worker ne peut pas choisir les tâches qu’il exécute : `worker_job_types` ne fonctionne qu’avec `queue = db`, car la plateforme décide qui reçoit un signal.
Pour donner à certains types de tâches leur propre capacité, envoyez leurs signaux vers leur propre file, et définissez la capacité de cette file sur la plateforme.
Rien ne change tant que vous ne définissez pas de route.

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes) : `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`.
Une route sans `@` ni URL appelle `queue_cloudtasks_url`.

Le côté gauche est un type de tâche de la [référence des tâches](../reference/jobs.md) ou un préfixe. Un nom l’emporte sur un préfixe, et un préfixe plus long sur un plus court.
Les types qui ne correspondent à aucune route, y compris ceux qu’ajoute une version ultérieure, utilisent la file par défaut (`queue_cloudtasks_queue`), et continuent donc de s’exécuter.
`my config check` vérifie la forme des routes ; un côté gauche qui ne correspond à aucun type de tâche arrête `my serve` au démarrage.

**Donnez à chaque processus les mêmes routes, `role = web` compris.** Un signal est envoyé par le processus qui stocke la tâche : un processus web pour la plupart de ce que font les utilisateurs, comme la
première tâche d’une publication, et un worker pour les tâches qu’une tâche stocke, retente, la tâche suivante d’une clé d’ordre et le balayage des signaux perdus.
Lorsque chaque rôle a ses propres paramètres, comme les services Cloud Run, transmettez la même valeur à tous :

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

Un processus avec d’autres routes ne lève aucune erreur et ne perd aucune tâche : ses signaux vont vers une autre file, et les tâches s’y exécutent, en dehors de la capacité que vous avez définie.
`my.queue.oldest_ready_age` ne le montre pas, car ces tâches n’attendent pas. Pour comparer les processus, chacun consigne au démarrage une ligne avec ses `routes` et leur `routes_hash` :
le hachage doit être le même partout. Pour voir où vont les signaux, lisez les métriques propres à la plateforme de chaque file.

**Permissions.** Les identifiants de chaque processus doivent pouvoir envoyer vers chaque file des routes : `roles/cloudtasks.enqueuer`.
Un envoi qui échoue est consigné et non retenté sur une autre file, afin qu’un type que vous limitez ne déborde pas vers les autres. Ces tâches démarrent en retard, une fois que le balayage renvoie leurs signaux après 2 minutes.

**Capacité.** N’importe quel récepteur exécute n’importe quelle tâche ; vous choisissez donc jusqu’où les séparer :

| Plateforme | Un seul récepteur | Récepteurs distincts |
| --- | --- | --- |
| Google Cloud | Un service. Chaque file a ses propres `maxConcurrentDispatches` et `maxDispatchesPerSecond` ; la limite d’instances du service est partagée | Un service par file, nommé par l’URL dans la route, chacun avec sa propre limite d’instances et sa simultanéité. Chacun définit `queue_cloudtasks_url` sur sa propre URL |

Une file nécessite la même configuration que celle par défaut : le rôle sur la file.
Seule la file par défaut a besoin du tick.

**Ce qu’ajoute une file.** Chaque file est une chose de plus à créer, surveiller et payer, et son importance dépend de la taille de votre serveur.
Répartissez ce que vous avez vu attendre dans `my.job.wait`, pas tout.

**Le balayage.** Chaque minute, le balayage cherche les tâches dues depuis plus de 2 minutes sans s’être exécutées, jusqu’à 500 par route et 500 pour la file par défaut, et les signale de nouveau.
Une file qui ne suit pas reçoit donc jusqu’à 500 signaux en double par minute. Ils n’exécutent pas une tâche deux fois, mais ce sont des messages que vous payez, et ils cessent une fois l’arriéré résorbé.

**Les clés d’ordre** tiennent toujours : la base de données décide de ce qui peut s’exécuter. Les tâches d’une même clé peuvent se trouver sur des files différentes, mais une file bloquée retarde aussi les tâches suivantes de ses clés.

**Ajouter, déplacer et supprimer une file.**

1. Créez la file et son récepteur, et donnez à chaque processus la permission d’y envoyer.
2. Modifiez les routes sur chaque processus. Tant qu’elles diffèrent, les signaux vont vers l’une ou l’autre file et les tâches s’exécutent aux deux endroits.
3. Pour supprimer une file, retirez-la d’abord des routes partout, et attendez qu’elle soit vide avant de la supprimer. Un signal dans une file supprimée est perdu, pas sa tâche : le balayage signale de nouveau la tâche une fois qu’elle est due depuis 2 minutes.
   Une tâche Cloud Tasks peut porter le signal d’une tâche jusqu’à 30 jours à l’avance ; une tâche due plus tard que l’attente est donc simplement signalée par le balayage lorsque son heure arrive.

<!-- source: health-checks 06a8834ade0d -->
## Contrôles de santé

| Chemin | Type | Quand il renvoie 200 |
| --- | --- | --- |
| `GET /healthz` | liveness | Le processus peut répondre en HTTP. Il ne regarde pas les dépendances telles que la base de données (une base de données lente ne redémarre donc pas le processus) |
| `GET /readyz` | readiness | La base de données est joignable et le schéma est appliqué |
| `GET /api/v1/instance/health` | Surveillance de disponibilité, pages d’état et clients (servi par le rôle `web`) | Identique à la disponibilité. Le résultat est mis en cache dans le processus pendant 60 secondes |

- Un 200 de `/healthz` ou `/readyz` a pour corps `{"status":"ok"}`.
- `/readyz` vérifie la base de données à chaque requête, et les requêtes simultanées partagent une seule vérification.
- `/api/v1/instance/health` renvoie 200 et `{"status":"ok","checked_at":"<RFC 3339>"}` en bonne santé, et 503 et `{"status":"unavailable","checked_at":...}` en cas d’échec (`Cache-Control: no-store`).
  Chaque processus ne sollicite la base de données qu’une fois par minute au plus, quelle que soit la fréquence de la surveillance ou des requêtes, et il ne regarde ni le jeton d’accès ni la limite de débit. Faites pointer les moniteurs de disponibilité vers ce point de terminaison, pas vers la sonde (`/readyz`).
  Pour la configuration par service, l’intervalle de surveillance et le coût, et la manière dont la maintenance est traitée, consultez le [guide de surveillance de la disponibilité](monitoring.md).
- Les trois journalisent au niveau Debug dans le journal d’accès et ne créent aucun span de trace.
- Lorsque la disponibilité n’est pas assurée, la réponse est 503 avec un corps RFC 9457 `application/problem+json`.
  Son `title` est traduit selon `Accept-Language` et ne contient aucun détail interne. La cause (une erreur de connexion, ou les noms des tables manquantes) va dans le journal.
- Une vérification de disponibilité est interrompue au bout de 3 secondes.
- La disponibilité exige aussi que la base de données contienne les `host` et `account_domain` configurés, ou que le processus ait pu les enregistrer.
- **Schéma appliqué** signifie que chaque table que les fichiers de schéma de la base de données (`schema/<NN>-<module>/<dialect>.sql`) déclarent avec `CREATE TABLE` existe
  (SQLite est vérifié via `sqlite_schema`, PostgreSQL via le `current_schema()` de la connexion). `my schema apply` applique tout le schéma ; une table manquante signifie donc que le schéma n’est pas appliqué ou est plus ancien que le binaire.
  Les différences de colonnes et d’index ne sont pas vérifiées ; `my schema apply -dry-run` affiche ce qui changerait.

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## Requêtes qui contournent le CDN

Lorsque le serveur est derrière un CDN et que sa propre adresse reste publique, définissez `origin_secret` et `origin_secret_header` afin qu’il refuse ce qui n’est pas passé par le CDN. Les contrôles de santé restent joignables. Consultez [Refuser les requêtes qui ne sont pas passées par le CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: security-headers 8c5e82848149 -->
## En-têtes de sécurité

Chaque réponse porte `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy: same-site` et `X-Frame-Options`,
et, pour les réponses qui ne sont pas des documents, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`.
Un gestionnaire les remplace pour les réponses qui nécessitent d’autres valeurs, comme le JSON ActivityPub et les médias publics.
`Permissions-Policy` refuse la caméra, la géolocalisation, le paiement et l’USB. Il refuse aussi le microphone, sauf lorsque le chat vocal est activé (`voice` n’est pas `off`) :
il autorise alors le microphone uniquement pour les pages de l’origine propre du serveur (`microphone=(self)`), afin que les navigateurs puissent parler dans un appel.
Le HTML du client web reçoit une CSP de document qui n’utilise pas de nonce (`script-src 'self'`, etc.).
Elle exige Trusted Types (`require-trusted-types-for 'script'`) et limite les politiques pouvant être créées aux deux du client (`trusted-types my-server-sanitized-html my-service-worker`).

<!-- source: web-client f35b79dd2cc3 -->
## Client web

Un processus dont le `role` est `web` ou `all` sert le client web intégré au binaire.

| Chemin | Contenu | `Cache-Control` |
| --- | --- | --- |
| Tout `GET` qu’aucune autre route ne reconnaît (`/`, `/@alice`, `/@alice/status/{id}`, `/settings`, etc.) | La coquille HTML de la SPA. Les paramètres du serveur sont transmis dans `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Fichiers de build avec des noms hachés | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | Le manifeste de l’application web construit à partir des paramètres du serveur | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` et les autres fichiers directement sous le build | Servis tels quels | `no-cache`, ETag |
| `GET /licenses/*` | Les licences de ce que le build embarque (avis des paquets npm, et `LICENSE-Unicode.txt` et `OFL.txt` pour les données embarquées non npm). Servies en `text/plain; charset=utf-8` quelle que soit l’extension | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Construits à partir des paramètres du serveur (consultez [Moteurs de recherche et robots d’IA](crawlers.md)) | `public, max-age=3600`, ETag |

- Les chemins sous `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (la feuille de style du thème du serveur) et `/licenses`, ainsi que `/robots.txt`, ne renvoient jamais la coquille ; si aucune autre route ne correspond, ils renvoient un problème 404.
- La coquille est servie après la négociation de contenu ActivityPub. Demander un chemin de la coquille avec un `Accept` ActivityPub renvoie 404 (les pages de publications redirigent, comme décrit plus bas).
  La coquille porte toujours `Vary: Accept`. Lorsque vous placez un CDN devant, incluez le type d’`Accept` (ActivityPub ou HTML) dans la clé de cache, ou excluez du cache la coquille, `/ap/*` et `/@*`.
  Ne servez que `/assets/*` depuis le CDN.
- Un binaire compilé sans le client web (un simple `go build`, sans construire d’abord le client) consigne un avertissement au démarrage et sert une coquille sans scripts.

<!-- source: federation-endpoints 7aaee8c9da42 -->
## Points de terminaison de fédération

Un processus dont le `role` est `web` ou `all` répond à ces points de terminaison, que les autres serveurs du fédivers utilisent pour trouver et afficher vos comptes.

| Chemin | Contenu |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033). `resource` est `acct:alice@{account_domain}` (`acct:alice@{host}` fonctionne aussi), un `id` d’Actor, ou l’URL `/@alice`. `subject` est toujours l’identifiant du côté `account_domain` |
| `GET /.well-known/host-meta`, `/.well-known/host-meta.json` | Le modèle d’URI WebFinger (XRD / JRD) |
| `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.1` | NodeInfo 2.1. `metadata.sourceCode` est le paramètre `source_url`, et `software.repository` est aussi défini pour une URL `https:` (consultez [Publier le code source](#publish-the-source-code-agpl-30)) |
| `GET /ap/actors/{uuid}` | L’Actor (`publicKey` est le `#main-key` RSA). Une requête qui préfère le HTML reçoit un 303 vers `/@{username}` |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | Collections |
| `POST /ap/actors/{uuid}/inbox`, `POST /ap/inbox` | La boîte de réception de chaque acteur local et la boîte de réception partagée. Les requêtes doivent être signées en HTTP |
| `GET /@{username}` | Un 302 vers l’`id` de l’Actor pour un `Accept` ActivityPub ; sinon le HTML du client web (voir ci-dessous) |
| `GET /@{username}/status/{uuid}` | Pour un `Accept` ActivityPub, un 302 vers l’`id` (`/ap/objects/{uuid}`) de la publication si un visiteur non connecté peut la voir, et 404 sinon. Sinon le HTML du client web |

- L’identifiant de l’Actor du serveur est `{account_domain}@{account_domain}` (la même forme que Mastodon).
- Un compte qui n’existe pas ou ne peut pas être affiché (suspendu, désactivé, supprimé ou distant) reçoit toujours le même 404 (`Cache-Control: no-store`).
- Les réponses anonymes portent `Cache-Control: public, max-age=60`. Les réponses qui varient selon `Accept` portent `Vary: Accept`.
  Pour un CDN qui ne gère pas `Vary: Accept`, excluez `/ap/*` et `/@*` du cache.

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### Domaines distincts pour l’identifiant et la remise

Lorsque `account_domain` diffère de `host`, les requêtes vers `/.well-known/webfinger`, `/.well-known/host-meta` et `/.well-known/nodeinfo` du côté `account_domain` sont redirigées vers `host` avec un 301 (avec `Access-Control-Allow-Origin: *`).
Si vous acheminez les requêtes de `account_domain` vers ce serveur, le serveur renvoie lui-même cette redirection.
Si un autre serveur ou reverse proxy répond à sa place, redirigez vers `https://{host}` en conservant le chemin et la chaîne de requête.
