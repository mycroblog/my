<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Exécuter le serveur : démarrage, arrêt et contrôles de santé

Comment démarrer et arrêter le serveur, obtenir son image et le surveiller pendant son fonctionnement : métriques, traces, file de tâches et contrôles de santé.

<!-- source: start-the-server c19e1ea32f07 -->
## Démarrer le serveur

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` valide la configuration, construit les adaptateurs de base de données, de file de jobs et de gestion des clés, et accepte le HTTP sur `listen`.
Avec `tls = off`, la valeur par défaut, il y parle en HTTP/1.1 simple : placez devant lui un proxy inverse ou un répartiteur de charge qui termine le HTTPS (et le HTTP/2 ou HTTP/3 pour les clients) et transmet en HTTP/1.1.
Pour servir le HTTPS sans proxy, voir [Servir le HTTPS depuis le serveur](#serving-https-from-the-server).
Les mises à jour en temps réel (`GET /api/v1/streaming`, les flux des fils local et fédéré, et les flux des salons de chat vocal) sont des réponses de longue durée qui envoient un commentaire de présence toutes les 30 secondes (15 pour un salon vocal) et restent ouvertes tant qu’elles sont inactives.
Le proxy ne doit pas mettre ces réponses en mémoire tampon (nginx : le serveur envoie `X-Accel-Buffering: no`), et son délai d’inactivité ou de lecture doit être plus long que l’intervalle de présence.
Il n’applique pas le schéma ni n’exécute les étapes de mise à niveau des données : exécutez `my schema apply` puis `my db upgrade` avant de démarrer (voir [Base de données](database.md) ; les mises à niveau exécutent les deux dans le cadre de [Mettre à niveau](database.md#upgrading)). Tant que `my db upgrade` n’a pas enregistré chaque étape de la version, `/readyz` répond `503`.
Dans la configuration Docker Compose (`deploy/docker/compose.yaml` ; voir [Déployer avec Docker Compose](deploy-docker-compose.md)), les services ponctuels `schema` et `upgrade` exécutent `my schema apply` et `my db upgrade` avant le démarrage du serveur.
Si le schéma n’est pas appliqué, le processus démarre quand même et la disponibilité renvoie 503, et il ne sert rien d’autre tant que le schéma n’est pas là.

Le premier `my serve` ou la première commande `my` qui utilise la base de données y enregistre `host` et `account_domain`. Lorsque la configuration diffère des valeurs enregistrées, `my serve` journalise `start failed` avec les deux paires de valeurs et se termine avec le code 1, et les commandes affichent la même erreur. Les exceptions sont `my schema apply` et `my db backup`, qui ne vérifient pas, et le mode maintenance, qui ne lit pas la base de données. `my config check -instance` et `my db upgrade -check` vérifient mais n’enregistrent rien.
Un processus qui n’a pas pu lire la base de données à son démarrage n’est pas prêt tant qu’il ne le peut pas, et d’ici là il n’exécute aucun job et répond `503` avec `Retry-After` à chaque requête sauf les vérifications de santé (`/healthz`, `/readyz`, `/health/live`, `/health/ready`, `/api/v1/instance/health`). C’est aussi ce que fait un serveur démarré avant `my schema apply`. S’il trouve ensuite des valeurs différentes, il s’arrête avec la même erreur.

Dans le profil `production`, `my config check` vérifie aussi que les paramètres valides conviennent à une instance de production : les erreurs, comme un `host` que les autres serveurs ne peuvent pas atteindre ou une KEK qui n’est pas aléatoire, le font se terminer avec 1, et les avertissements, comme `mail = none`, non.
`my serve` journalise les deux comme des avertissements au démarrage mais démarre quand même ; exécutez donc `my config check` dans le cadre du déploiement.
La [liste de contrôle de production](production-checklist.md) liste les vérifications et ce qu’il faut vérifier à la main avant la mise en service.

| `role` | HTTP | Worker de jobs |
| --- | --- | --- |
| `all` (par défaut) | Toutes les routes | S’exécute |
| `web` | Toutes les routes | Ne s’exécute pas |
| `worker` | Vérifications de santé uniquement (pour les sondes de la plateforme) | S’exécute |
| `sfu` | Vérifications de santé et WebSocket du nœud média de `voice = sfu`, qui reçoit l’audio sur ses propres ports UDP et TCP ([Chat vocal](voice.md#run-media-nodes)) | Ne s’exécute pas |

Les journaux vont sur l’erreur standard en JSON Lines. Chaque ligne a `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) et `severity`, le même niveau sous la forme que lit Cloud Logging (`DEBUG`, `INFO`, `WARNING`, `ERROR`, et `CRITICAL` pour une erreur qui arrête le processus).
Les journaux écrits pendant le traitement d’une requête portent un `request_id`, et le même ID est renvoyé dans l’en-tête de réponse `X-Request-Id`.
Si le `X-Request-Id` de la requête compte au plus 128 caractères parmi les lettres, les chiffres et `-_.:`, il est utilisé tel quel, comme l’ID attribué par un répartiteur de charge.
Les journaux à l’intérieur d’une trace (requêtes et jobs) portent aussi `trace_id` et `span_id` (voir [Traces](#traces)).
Avec `logs_exporter = gcp`, pour Cloud Run, les lignes portent aussi les champs de Cloud Logging qui relient une ligne à sa trace dans Cloud Trace, et chaque ligne d’accès sa requête sous `httpRequest` ; les autres champs restent tels quels.

Avec `logs_exporter = otlp`, le serveur envoie les lignes comme enregistrements de journal OpenTelemetry par OTLP/HTTP au récepteur à `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` ou `OTEL_EXPORTER_OTLP_ENDPOINT` (par défaut `http://localhost:4318`), avec `OTEL_EXPORTER_OTLP_HEADERS`.
Chaque enregistrement porte les champs de sa ligne comme attributs, et la trace et le span de sa ligne : un collector OpenTelemetry auquel chaque hôte envoie rassemble ainsi les journaux de tous en un seul endroit, à côté de leurs traces (`traces_exporter = otlp`).
Les lignes écrites avant que le serveur ait démarré, et les enregistrements que le récepteur n’a pas acceptés, vont toujours sur la sortie d’erreur standard, suivis d’une ligne `otlp logs export failed`.
Une rafale de plus de 2 048 lignes pendant que le récepteur est lent abandonne les lignes qui ne tiennent pas dans la file d’envoi.
Un pipeline minimal du collector pour les journaux :

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  otlphttp:
    endpoint: http://loki:3100/otlp # any receiver of OTLP logs, such as Loki
service:
  pipelines:
    logs:
      receivers: [otlp]
      exporters: [otlphttp]
```

Chaque requête HTTP et chaque exécution de job écrit une ligne à sa fin. La ligne a les mêmes noms et valeurs que les étiquettes des [métriques](#metrics) ci-dessous ;
avec `metrics_exporter = none`, une plateforme peut donc construire à partir de ces lignes des métriques basées sur les journaux (compteurs et distributions).

| Ligne (`msg`) | Champs |
| --- | --- |
| `http request` (`INFO` ; les vérifications de santé utilisent `DEBUG`) | `http.request.method`, `http.route`, `my.surface` et `http.response.status_code` (un nombre) ont les mêmes valeurs que les étiquettes de `http.server.request.duration`. Aussi `duration_seconds` (un nombre de secondes), `url.path` (le chemin sans la chaîne de requête), `http.response.body.size` et `request_id`. `http.route` et `my.surface` sont absents pour les requêtes qui n’ont atteint aucune route (vérifications de santé, requêtes refusées avant le routage), et la métrique ne les compte pas. Une requête dont le client est parti avant que le serveur puisse répondre a l’état `499` au lieu d’un 5xx, sauf si un gestionnaire a paniqué ; le client n’a rien reçu, ce qui ne compte donc pas comme une erreur du serveur |
| `job` (`INFO` en cas de succès et pour `postponed`, `WARN` pour `retry`, `ERROR` pour `dead`) | `my.job.type` et `my.outcome` ont les mêmes valeurs que les étiquettes de `my.job.duration`. Aussi `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt`, et `error` en cas d’échec |
| `error report` (`ERROR` ; `CRITICAL` pour un échec qui arrête le processus) | Une ligne par erreur signalée ([Signalements d’erreurs](#error-reports)) |

Avec `metrics_exporter = none`, les lignes `http request` et `job` portent aussi les [métriques système](#system-metrics) des adaptateurs et du pool de la base de données.
Lorsque la requête ou le job a exécuté des opérations de file, de clé-valeur, de flux ou de blob : `my.adapter.operations.total`, `my.adapter.errors.total` et
`my.adapter.errors`, les erreurs par `my.adapter` (comme `{"kv.valkey": 2}`). `db.client.connection.usage`, les connexions utilisées sur le maximum du pool,
lorsqu’il en a un. Lorsqu’elles ne sont pas nulles, `my.db.client.connection.waits`, `db.client.connection.wait_time` (secondes) et `db.client.connection.timeouts` : l’augmentation
depuis la ligne précédente du processus ; leur somme sur les lignes est donc le total du processus.

<!-- source: error-reports b70c69ddef24 -->
### Rapports d’erreurs

Le guide [Rapports d’erreurs](error-reporting.md) explique où lire ces lignes sur chaque plateforme, l’écran Erreurs, les notifications, et l’envoi de rapports à un outil de suivi d’erreurs.

Le serveur signale les erreurs qui demandent l’attention de l’exploitant sous la forme d’une ligne `error report` chacune :

- une panique dans un gestionnaire HTTP, une tâche ou une tâche d’arrière-plan ;
- une erreur derrière une réponse 5xx de l’API cliente, de l’API d’administration, des pages de connexion, d’ActivityPub, d’OAuth ou des médias ;
- une tâche qui échoue définitivement (`dead`) : elle a épuisé ses nouvelles tentatives, ou a refusé de s’exécuter de nouveau. Les envois qui ont abandonné parce que l’autre serveur échouait sans cesse ne sont pas signalés ;
- un échec de démarrage : une configuration invalide, ou une base de données ou un adaptateur qui ne peut pas être mis en place.

Les réponses 4xx, les nouvelles tentatives de tâches, le mode de maintenance, les vérifications de disponibilité échouées, les requêtes dont le client est parti et les échecs causés par d’autres serveurs ne sont pas signalés ; les métriques et les autres lignes de journal les couvrent.

Lorsque le client d’une requête s’en va, les autres lignes d’erreur du serveur pour cette requête sont écrites à `INFO` au lieu de `ERROR`, puisque la plupart d’entre elles disent seulement que la requête a été annulée.

La ligne est sous la forme que lit Google Cloud Error Reporting : sur Cloud Run, les rapports apparaissent dans Error Reporting regroupés comme des erreurs sans aucune configuration. Sur les autres plateformes, c’est une simple ligne JSON :

| Champ | Valeur |
| --- | --- |
| `@type`, `serviceContext` | Le type d’événement Error Reporting, et `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 chiffres hexadécimaux identiques pour chaque occurrence de la même erreur : construits à partir de la source, du lieu, du type d’erreur et des fonctions du serveur dans la pile, sans numéros de ligne |
| `my.error.source`, `my.error.kind` | `server` (ou `web`), et `panic` ou `error` |
| `my.error.where` | Où cela s’est produit : `<surface> <method> <route>` pour une requête (comme `client_api GET /api/v1/statuses/{id}`), le type de tâche pour une tâche, le nom de la tâche pour une tâche d’arrière-plan, `startup` pour un échec de démarrage |
| `my.role`, `deployment.environment.name` | Le `role` du processus et `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Les ID permettant de retrouver les lignes de requête ou de tâche de la même erreur, lorsqu’elle en a |
| `context.httpRequest` | Pour une erreur de requête : `method`, `url` (la route, comme `/api/v1/statuses/{id}`, jamais le chemin demandé par le client) et `responseStatusCode`, le statut répondu (500) |
| `message` | En dernier sur la ligne : `<type>: <message>`, un saut de ligne, puis la pile Go |

Avant qu’un rapport quitte le processus, le message voit ses URL, adresses e-mail, identifiants `@user@host`, adresses IP, UUID et ULID, et nombres de 10 chiffres ou plus remplacés par `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` et `<n>`, et il est coupé à 1 KiB.
La pile conserve ses positions de fichier mais pas les valeurs d’arguments de ses lignes de fonction (elles sont affichées comme `(...)`), et est coupée à 16 KiB. Aucun rapport ne porte de corps de requête, d’en-tête, de requête, de cookie, de compte, de nom d’utilisateur, d’adresse IP ni de User-Agent.
Aucune autre ligne ne porte de pile ; Error Reporting ne trouve donc que les piles de ces lignes.

Un nom de champ est une clé unique qui contient `.` ; il n’est pas imbriqué. Mettez-le entre guillemets dans les filtres et extractions de Cloud Logging (par exemple `jsonPayload."http.route"`, et `EXTRACT(jsonPayload.duration_seconds)` pour une valeur de distribution).
Les champs utilisés comme libellés ne prennent leurs valeurs que dans un ensemble fini ; `url.path`, `request_id` et `job_id` ne sont jamais des libellés.

Avec `metrics_exporter = emf`, ces deux lignes sont les lignes EMF elles-mêmes. La ligne `http request` porte la valeur de `http.server.request.duration`, et la ligne `job` porte les valeurs de `my.job.duration` et `my.job.wait` (clés nommées d’après les métriques), plus la directive `_aws`, `service.name`, `service.version`, `my.role` et `service.namespace` (le `host`). Ces trois métriques ne sont écrites sur aucune autre ligne. Les champs que vous choisissez dans `metrics_emf_dimensions` sont écrits sous forme de chaînes, comme l’exige EMF (par exemple la dimension `http.response.status_code` vaut `"200"`).
Les journaux vont vers la sortie d’erreur standard et les autres lignes EMF vers la sortie standard. Le pilote de journaux `awslogs` d’ECS envoie les deux à CloudWatch Logs ; les métriques sont donc extraites des deux.

<!-- source: docker-compose-files eab514676a87 -->
## Fichiers Docker Compose

`deploy/docker/` contient ce qui déploie une instance. `compose.yaml` s’exécute seul, et les autres fichiers s’empilent par-dessus.

| Fichier | Ajoute | À utiliser lorsque |
| --- | --- | --- |
| `compose.yaml` | Le serveur et ses services ponctuels de base de données, sur SQLite et des fichiers de médias dans le volume `data` | Toujours |
| `compose.postgres.yaml` | PostgreSQL dans un conteneur, dans le volume `postgres` | Vous voulez PostgreSQL sur cette machine. Pour un serveur PostgreSQL ailleurs, omettez-le et définissez `MY_DATABASE` et `MY_DATABASE_URL` dans `.env` |
| `compose.s3.yaml` | Les médias dans un bucket compatible S3 ; avec `COMPOSE_PROFILES=garage` aussi un stockage [Garage](https://garagehq.deuxfleurs.fr/) à un nœud dans le volume `garage` | Les médias dépassent un disque, ou vous les voulez dans un bucket. Sans le profil, le stockage est l’externe défini par les paramètres `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, avec des certificats TLS sur les ports 80 et 443 | `MY_HOST` est un domaine public et rien d’autre ne termine le TLS sur cette machine |
| `compose.sfu.yaml` | Le chat vocal avec un nœud média : le serveur se scinde en `role = web`, `worker` et `sfu`, et Caddy envoie le WebSocket du nœud au nœud. Demande `compose.postgres.yaml` et `compose.caddy.yaml` | Vous voulez `voice = sfu` ([Chat vocal](voice.md#docker-compose)) |
| `compose.build.yaml` | Construit l’image à partir d’une copie du dépôt | Vous exécutez votre propre construction |

Choisissez la base de données et le stockage des médias avant le premier démarrage : les données ne se déplacent pas d’elles-mêmes lorsque les fichiers changent
([Sauvegarder](backup.md) montre ce qui doit être déplacé).

**Empilez les fichiers** avec des options `-f` dans cet ordre, ou nommez-les une fois dans `.env`, afin que chaque commande `docker compose` ultérieure
(`exec`, `logs`, `pull`, et celles des autres guides) utilise les mêmes fichiers :

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Un fichier nommé `compose.override.yaml` dans le répertoire n’est lu que lorsque ni `-f` ni `COMPOSE_FILE` ne nomment de fichiers. Avec
eux, listez-le aussi.

**Paramètres.** Copiez `example.env` vers `.env`. Compose lit `.env` pour renseigner les variables de ses fichiers, et transmet tout le
fichier aux conteneurs du serveur ; chaque paramètre de la [référence de la configuration](../reference/configuration.md) fonctionne donc
en étant défini à cet endroit. Le modèle les liste tous, avec une phrase pour chacun.

- La clé de chiffrement de clés reste hors de `.env`, dans le fichier `secrets/kek`, et les conteneurs la lisent comme le secret Docker
  `/run/secrets/my_kek`, de sorte qu’elle ne devient jamais une variable d’environnement. Une ligne `MY_KEK` dans `.env` arrête le serveur, qui
  refuse `MY_KEK` et `MY_KEK_FILE` ensemble ([gérer la clé](key-management.md)).
- Le serveur s’exécute sous l’utilisateur `65532`, et Compose monte le fichier avec les permissions qu’il a ; le fichier doit donc être
  lisible par d’autres. Conservez-le dans un répertoire que vous seul pouvez ouvrir, comme le font les commandes de `example.env`.
- Les autres secrets parviennent aux conteneurs sous forme de variables d’environnement. Pour en passer un sous forme de fichier à la place, montez-le avec un secret
  dans un `compose.override.yaml` à vous et définissez son `<NAME>_FILE`, comme `compose.yaml` le fait pour la clé.
- `.env` contient des secrets : `chmod 600 .env`, et conservez-en une copie avec vos sauvegardes.
- `MY_LISTEN` reste `:8080`, puisque les fichiers se connectent à ce port, sauf si le serveur sert lui-même le HTTPS
  ([Servir le HTTPS depuis le serveur](#serving-https-from-the-server)).

**TLS avec Caddy.** Définissez `MY_HOST` sur un domaine dont le DNS pointe vers la machine, avec les ports 80 et 443 accessibles depuis
Internet, et retirez `MY_WEB_ORIGIN` de `.env`. Caddy obtient et renouvelle le certificat tout seul à la première requête.
Le serveur fait confiance au `X-Forwarded-For` des plages d’adresses par défaut de Docker ; définissez `MY_TRUSTED_PROXIES` si les vôtres diffèrent.

**Le Garage inclus.** `compose.s3.yaml` avec `COMPOSE_PROFILES=garage` a besoin des valeurs que `example.env` liste en tête.
Le service ponctuel `garage-init` prépare le stockage à chaque `up`. Il reste à l’intérieur du réseau Compose, puisque le serveur
sert lui-même les médias. Un nœud, c’est un disque : sauvegardez le volume `garage` ou le bucket ([Sauvegarder](backup.md#media-in-a-bucket)).

<!-- source: serving-https-from-the-server 7b583c946805 -->
## Servir le HTTPS depuis le serveur

`my` peut servir lui-même le HTTPS, de sorte qu’une petite machine exécute tout le serveur sous forme d’un seul binaire avec un domaine et les ports 80 et 443,
sans proxy inverse. Avec `tls = acme`, il obtient ses certificats auprès d’une autorité de certification ACME comme Let’s
Encrypt. Avec `tls = files`, il sert un certificat et une clé que vous fournissez et que vous tenez à jour avec un autre outil, comme
certbot.

```sh
MY_TLS=acme
MY_TLS_ACME_AGREE_TERMS=true            # you agree to the terms of service of the certificate authority
# MY_TLS_ACME_EMAIL=ops@social.example  optional contact of the account
# MY_LISTEN=:443          the default with tls other than off
# MY_TLS_HTTP_LISTEN=:80  the default; empty opens no port 80
```

```sh
MY_TLS=files
MY_TLS_CERT_FILE=/etc/letsencrypt/live/social.example/fullchain.pem
MY_TLS_KEY_FILE=/etc/letsencrypt/live/social.example/privkey.pem
```

- **Ports.** Le serveur sert le HTTPS sur `listen` (`:443` par défaut dès que `tls` n’est pas `off`). Sur `tls_http_listen` (`:80`
  par défaut), il redirige par `301` les requêtes `GET` et `HEAD` vers le même chemin en `https`, et répond `400` à toute autre
  méthode, de sorte qu’aucun corps de requête ne circule en clair.
- **Hôtes.** Le serveur sert `host`, et l’hôte de `web_origin` lorsqu’il diffère. Une négociation pour tout autre nom
  échoue sans certificat, et ne fait jamais commander un certificat au serveur.
- **Connexions.** TLS 1.2 ou ultérieur avec les suites de chiffrement par défaut de Go, HTTP/2 et HTTP/1.1, au plus 64 KiB d’en-têtes
  de requête, et au plus 100 flux concurrents sur une connexion HTTP/2. Les délais sont ceux du HTTP simple. Le serveur
  ajoute toujours `Strict-Transport-Security` à chaque réponse ([En-têtes de sécurité](#security-headers)).
- **Rôles.** Seuls les rôles `web`, `all` et `sfu` écoutent avec TLS. Un nœud média (`role = sfu`) sert l’hôte de
  `voice_sfu_public_url` au lieu de `host`. Un `worker` sert ses vérifications de santé en HTTP simple sur
  `listen` (`:8080` par défaut) et n’ouvre pas de port 80.
- **Adresses des clients.** Sans proxy devant, le pair de la connexion est le client : laissez `trusted_proxies` vide.
  Ne le définissez que lorsqu’un CDN placé devant se connecte au serveur en `https`.
- **Mode maintenance.** `my serve -maintenance` ne lit ni `tls` ni les certificats et répond en HTTP simple sur
  `listen` (`:8080` sauf si vous le définissez) ; dirigez donc `MY_LISTEN` et votre pare-feu vers ce port pendant son exécution, ou conservez un
  proxy inverse pour la maintenance.

<!-- source: certificates-from-an-acme-certificate-authority 52037a75d641 -->
### Certificats d’une autorité de certification ACME

Avec `tls = acme`, le serveur commande un certificat pour chacun de ses hôtes qui n’en a pas, auprès de l’autorité de certification
à `tls_acme_directory` (Let’s Encrypt par défaut).

- **Avant de commencer.** `host` doit être un nom DNS qui pointe vers le serveur, et les ports 443 et 80 doivent être accessibles depuis
  Internet : l’autorité de certification vérifie que le serveur contrôle le nom en s’y connectant. Il répond sur
  `listen` (le défi TLS-ALPN-01) ou sur `tls_http_listen` (le défi HTTP-01) ; un `tls_http_listen` vide
  fonctionne donc avec le seul port 443. Les certificats génériques et la validation par enregistrements DNS ne sont pas disponibles.
- **Premier démarrage.** Le serveur commence à écouter sans attendre de certificat, puis les commande. Tant qu’un hôte n’a pas son
  certificat, une négociation pour lui échoue ; avec Let’s Encrypt, les premiers certificats arrivent généralement en moins d’une minute. Le journal
  indique `obtained a TLS certificate` pour chaque hôte, et `the TLS certificate could not be obtained` avec la raison lorsqu’une
  commande échoue.
- **Renouvellements.** Le serveur renouvelle chaque certificat à l’heure que l’autorité de certification suggère pour lui (ACME Renewal
  Information) : il choisit une fois une heure aléatoire dans la fenêtre suggérée, la conserve tant que la fenêtre reste, et redemande
  comme l’indique le `Retry-After` de l’autorité, au moins toutes les 12 heures, de sorte qu’une autorité qui doit remplacer des certificats
  plus tôt puisse déplacer la fenêtre. Lorsque l’autorité ne propose pas une telle fenêtre, le serveur renouvelle après les deux tiers de la
  durée de vie du certificat, ou après la moitié pour un certificat valable moins de 10 jours. Un renouvellement indique à
  l’autorité quel certificat il remplace, ce qui le maintient hors des limites de débit de Let’s Encrypt. Le certificat actuel
  reste en service jusqu’à l’arrivée de son renouvellement, et un renouvellement qui échoue attend comme une première commande.
- **Commandes en échec.** Après une commande en échec, le serveur attend avant la suivante pour le même hôte : 1 minute après le
  premier échec, 10 minutes après le deuxième, 100 minutes après le troisième, puis un jour, afin de rester dans les
  limites de l’autorité de certification. L’attente est conservée dans la base de données et survit aux redémarrages. Corrigez la cause (DNS, pare-feu,
  port) et la commande suivante part à la fin de l’attente.
- **Où ils sont conservés.** Le compte auprès de l’autorité de certification, les certificats et l’enregistrement de chaque commande
  sont conservés dans la base de données ; les clés privées sont scellées avec la clé de chiffrement de clés, comme les autres secrets
  ([gestion des clés](key-management.md)). Une sauvegarde de la base de données les inclut, et un serveur redémarré sert les
  certificats stockés aussitôt, sans commander. Changer la clé de chiffrement de clés les ré-enveloppe ; cela ne commande pas de nouveaux
  certificats.
- **Plusieurs processus.** Les processus qui partagent la base de données et le magasin `kv` se coordonnent : l’un commande le certificat d’
  un hôte pendant que les autres attendent puis le chargent, et le processus auquel l’autorité de certification se connecte répond à sa
  vérification. Utilisez `kv = db` ou `kv = valkey` ; `kv = memory` est refusé sauf si un seul processus exécute tout (`role = all`).
  Le processus qui obtient un certificat en informe les autres par le courtier [`stream`](../reference/configuration.md#stream), et ils le servent en
  quelques secondes ; chaque processus `web` avec `tls = acme` reste donc à l’écoute du courtier. Avec `stream = hub`, ou lorsqu’un
  avis est perdu, chaque processus relit quand même les certificats actuels dans la base de données toutes les 5 minutes, et chaque
  minute une fois que son certificat a atteint son heure de renouvellement, de sorte qu’un renouvellement les atteint bien avant l’expiration de l’ancien certificat.
- **Profils.** `tls_acme_profile` choisit un profil que l’autorité de certification propose, comme `shortlived` chez Let’s
  Encrypt. Vide, il prend sa valeur par défaut. Après un changement de profil, le prochain renouvellement le suit, et le certificat qu’il apporte
  est servi même s’il expire avant celui qu’il remplace.

Pour l’essayer sans dépenser les limites de l’environnement de production de Let’s Encrypt, commencez avec son environnement de test (staging),
qui émet des certificats auxquels les navigateurs ne font pas confiance, sous des limites bien plus élevées :

```sh
MY_TLS_ACME_DIRECTORY=https://acme-staging-v02.api.letsencrypt.org/directory
```

Une fois que le journal indique que les certificats ont été obtenus et que `curl -kv https://social.example/` montre un certificat issu de
`(STAGING)`, retirez le paramètre et redémarrez. Le serveur enregistre un nouveau compte auprès de l’environnement de production et
y commande ses certificats ; ceux du test restent dans la base de données et ne sont plus servis.

<!-- source: certificates-from-files 35f16ea23b33 -->
### Certificats issus de fichiers

Avec `tls = files`, `tls_cert_file` contient d’abord le certificat du serveur, puis ses intermédiaires (le `fullchain.pem` de certbot) ; `tls_key_file` contient sa clé. Le certificat doit couvrir `host`, et il sert aussi l’hôte de
`web_origin` lorsqu’il diffère et que le certificat le couvre.

Le serveur regarde chaque minute les dates de modification des deux fichiers et les recharge lorsque l’un change, sans
redémarrage. Une paire qui ne correspond pas encore (un nouveau certificat avant sa nouvelle clé) maintient le certificat précédent en
service jusqu’à la vérification suivante, et le journal en indique la raison.

<!-- source: commands-and-metrics de97a3993e6b -->
### Commandes et métriques

`my tls` affiche et contrôle les certificats avec la configuration du serveur, depuis toute machine qui atteint la
base de données (la [CLI d’administration](admin-cli.md#tls-certificates)) :

```sh
my tls status                                          # each host: certificate, renewal, latest orders
my tls renew -host social.example                      # order now, without waiting
my tls revoke -host social.example -reason key_compromise
```

- **`status`** affiche, pour chaque hôte, le certificat actuel (son numéro de série, son émetteur et son expiration), la date de son renouvellement (la
  fenêtre de l’autorité de certification, ou l’heure dérivée de sa durée de vie), le nombre de commandes consécutives qui n’en ont obtenu aucun, et
  les dernières commandes avec leurs erreurs. Avec `tls = files`, il affiche le certificat de `tls_cert_file`.
- **`renew`** commande un certificat maintenant, en ignorant l’heure de renouvellement et l’attente après des commandes en échec : exécutez-le une fois que vous
  avez corrigé la cause des commandes en échec. Le serveur en cours d’exécution répond à la vérification de l’autorité de certification ; il doit donc être
  démarré, et ses processus servent le nouveau certificat en quelques secondes, ou en 5 minutes avec `stream = hub`.
- **`revoke`** commande d’abord un certificat qui remplace l’actuel, puis demande à l’autorité de certification de révoquer
  l’actuel et l’enregistre ; chaque processus cesse de servir le certificat révoqué en quelques secondes, ou en 5
  minutes avec `stream = hub`. Utilisez `key_compromise` lorsque la clé privée peut être connue d’autres personnes, `superseded` ou
  `operator` sinon. Lorsque le remplaçant ne peut pas être commandé, la révocation a quand même lieu : les processus servent le
  certificat précédent de l’hôte s’il n’est pas révoqué, ou rien jusqu’à ce que le serveur en obtienne un nouveau.
- `renew` et `revoke` exigent `tls = acme` et un `kv` autre que `memory`, puisque les processus auxquels l’autorité de certification
  se connecte lisent les réponses à ses vérifications dans le magasin `kv`. Avec `tls = files`, ils sont refusés : renouvelez
  ou révoquez le certificat avec l’outil qui écrit les fichiers.

Avec [`metrics_exporter`](#metrics) défini, mettez en place une alerte sur `my.tls.certificate.expiry` sous 7 jours et sur
`my.tls.order.consecutive_failures` de 3 ou plus ([alertes](alerts.md#mytlscertificateexpiring)) : un renouvellement en échec se voit
des semaines avant l’expiration du certificat.

<!-- source: checks-and-running-it 983028d27675 -->
### Vérifications et exécution

`my config check` signale, avec `tls = acme`, des conditions non acceptées, un `host` qui est une adresse IP, `localhost`,
ou un nom sans point, un `tls_acme_directory` qui n’est pas une URL `https`, et `kv = memory` avec un `role` autre que
`all`, comme des erreurs. Avec `tls = files`, il signale comme des erreurs un certificat ou une clé illisible, une clé qui ne correspond pas au
certificat, et un certificat qui ne couvre pas `host`, a expiré ou n’est pas encore valide, et avertit lorsqu’il reste
moins de 14 jours. Il avertit lorsque `trusted_proxies` est défini avec `tls` autre que `off`. Avec `tls = off`, il avertit
lorsque `listen` est le port 80 ou 443 sur une adresse autre que loopback, puisque rien ne semble terminer le TLS devant le
serveur ; le serveur journalise le même avertissement une fois par démarrage lorsqu’un tel serveur, avec `trusted_proxies` vide, reçoit une requête HTTP simple
d’une adresse autre que loopback.

**systemd.** Les ports inférieurs à 1024 exigent une capacité pour un utilisateur autre que root. Donnez-la au service et à rien d’autre :

```ini
[Service]
User=my
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

Avec `tls = files`, l’utilisateur doit aussi pouvoir lire les fichiers de certificat. Avec certbot, ses répertoires `live` et `archive`
ne sont lisibles que par root : donnez au groupe du service l’accès (`chgrp` et `chmod g+rx` sur les répertoires, `g+r` sur
les clés), ou copiez les fichiers dans un hook de déploiement vers un répertoire que le service lit.

**Docker.** Docker 20.10 et les versions ultérieures laissent un conteneur ouvrir des ports inférieurs à 1024 sans capacité, de sorte que l’image fonctionne telle
quelle. L’image définit `MY_LISTEN=:8080` ; définissez donc `MY_LISTEN=:443` dans `.env`, et publiez les deux ports dans un
`compose.override.yaml` (`ports: ["80:80", "443:443"]`). Avec `tls = files`, montez les fichiers de certificat en lecture seule
là où pointent `MY_TLS_CERT_FILE` et `MY_TLS_KEY_FILE` ; le serveur s’exécute sous l’utilisateur `65532`, qui doit pouvoir les lire.
Avec `tls = acme`, il n’y a rien à monter : les certificats vivent dans la base de données.
Omettez `compose.caddy.yaml`, qui prendrait les mêmes ports. Cela ne vaut pas avec `network_mode: host`, où
le conteneur partage le réseau de la machine et a besoin de la même capacité qu’un service hors conteneur.

**Comparé à Caddy.** Servir le HTTPS depuis `my` renonce au HTTP/3, à la compression des réponses (le serveur envoie les fichiers du
client web compressés et ne compresse rien d’autre), à un journal d’accès propre (les lignes de journal `http request` du serveur subsistent), et aux certificats validés par
DNS, y compris les certificats génériques. Conservez `compose.caddy.yaml`, ou un autre proxy, lorsque vous avez besoin de l’un d’eux.

<!-- source: get-the-image 123f0112b538 -->
## Obtenir l’image

L’image du serveur est `ghcr.io/mycroblog/my`, publique, pour `linux/amd64` et `linux/arm64`, avec tous les adaptateurs.

| Tag | Pointe vers | À utiliser pour |
| --- | --- | --- |
| `0.1.0` | Cette version, définitivement | Nommer une version |
| `0.1` | Le dernier correctif de `v0.1` | Docker Compose, qui applique le schéma à chaque `up` |
| `0.1.0-rc.1` | Cette préversion | Essayer une version avant sa sortie |
| `nightly` | Le dernier commit de `main` qui a passé les tests nocturnes, déplacé chaque nuit où il change | Essayer des changements avant une préversion, sur une instance que vous pouvez jeter |

Il n’y a ni `latest` ni `0` : les deux franchiraient une version mineure, et chaque mise à niveau vers une nouvelle version mineure doit passer [par la précédente](database.md#skipping-releases). Nommez une version dans chaque commande ; `docker pull ghcr.io/mycroblog/my` seul échoue avec `manifest unknown`.
Une version corrective peut aussi avoir besoin de `my schema apply` et de `my db upgrade` ; en dehors de Docker Compose, suivez donc [Mise à niveau](database.md#upgrading) pour chaque version. En production, épinglez le condensat que donnent les notes de la version :

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

**`nightly` n’a aucun chemin de mise à niveau.** Les mises à niveau ne sont promises qu’entre versions publiées : d’une image `nightly` à la suivante, ou de `nightly` à une version, le schéma et les étapes de données de `my db upgrade` peuvent changer d’une façon par laquelle aucune version ne passe ; attendez-vous donc à recréer sa base de données. Ne l’exécutez jamais en production. `my version` indique de quel commit il s’agit, comme dans `v0.1.0-rc.5.0.20261109182600.g7a789720a1b2` : une version postérieure à la dernière publiée (`v0.1.0-rc.5`), l’heure du commit en UTC, et les 12 premiers caractères du commit après `g`. Pour conserver un build, épinglez son condensat (`ghcr.io/mycroblog/my:nightly@sha256:...`) : un build plus ancien perd le tag mais reste récupérable par son condensat.

Où l’exécuter :

- **Docker Compose** : `deploy/docker/compose.yaml` exécute l’image de `MY_VERSION` dans `.env` ([fichiers Docker Compose](#docker-compose-files)). Pour la construire plutôt depuis un checkout, ajoutez la surcharge `compose.build.yaml` (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`), et définissez `MY_VERSION` sur un nom à vous, puisqu’il nomme votre image locale et c’est ce qu’affiche `my version`.
- **Google Cloud et AWS** : les modules OpenTofu exécutent l’image de `ghcr.io` telle quelle, la version que nomme leur `ref` ([Déployer avec OpenTofu](deploy-opentofu.md#the-root-modules)).
- **Kubernetes** : récupérez-la depuis `ghcr.io` telle quelle.

<!-- source: verify-an-image 76b9a1293edb -->
### Vérifier une image

Chaque image publiée a une provenance signée via Sigstore, qui lie le condensat au workflow et au commit qui l’ont construite, ainsi qu’un SBOM. Avec la [CLI GitHub](https://cli.github.com/), et connecté à `ghcr.io` si la commande le demande :

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

La première commande échoue sauf si l’image a été construite par le workflow de publication de ce dépôt. Pour une image `nightly`, vérifiez-la plutôt avec `--signer-workflow mycroblog/my/.github/workflows/nightly.yml`. Elle affiche le commit, qui est celui du tag de la version ; le label `org.opencontainers.image.revision` de l’image nomme le même.

<!-- source: get-the-binary 2608f7bdd54d -->
## Obtenir le binaire

Pour exécuter `my` sans conteneur, chaque version joint le binaire de l’image, pour `linux/amd64` et `linux/arm64`,
sous la forme `my_<version>_linux_<arch>.tar.gz`. C’est un seul binaire statique contenant le client web ; l’archive contient aussi
`LICENSE`, les mentions de licence dans `licenses/` et un README. Il n’existe pas de build pour d’autres systèmes d’exploitation.

```sh
curl -fsSLO https://github.com/mycroblog/my/releases/download/v0.1.0/my_v0.1.0_linux_amd64.tar.gz
tar xzf my_v0.1.0_linux_amd64.tar.gz my
./my version
```

Configurez-le comme l’image est configurée ([référence de configuration](../reference/configuration.md)), et suivez
[Mettre à niveau](database.md#upgrading) pour chaque version : rien n’applique le schéma à votre place.

<!-- source: verify-a-binary 03daabe0c940 -->
### Vérifier un binaire

Chaque archive a une provenance signée via Sigstore, comme une image, et la version liste les SHA-256 des archives dans `SHA256SUMS` :

```sh
gh attestation verify my_v0.1.0_linux_amd64.tar.gz -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
sha256sum --ignore-missing -c SHA256SUMS
```

<!-- source: publish-the-source-code-agpl-30 2e87c44ffb2f -->
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

L’image porte la licence de `my` dans `/usr/share/doc/my/LICENSE`, et les mentions de licence du code et des données tiers contenus dans le binaire dans `/usr/share/doc/my/licenses/` :
`go-modules.md` pour les modules Go, `npm-packages.md` et `npm-packages-sw.md` pour les paquets npm du client web, et les licences de ses données et polices intégrées.
Si vous distribuez l’image ou le binaire, transmettez ces fichiers avec lui.

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

<!-- source: metrics bb10bda1b8a4 -->
## Métriques

Le paramètre `metrics_exporter` (par défaut `none`) exporte les métriques du service. Ses paramètres associés se trouvent dans la [référence de la configuration](../reference/configuration.md).

| `metrics_exporter` | Chemin | Adapté à |
| --- | --- | --- |
| `none` | Rien n’est exporté. L’instrumentation ne coûte presque rien. Les valeurs de toute l’instance vont quand même dans une ligne de journal `metrics snapshot` toutes les 15 minutes, et celles des adaptateurs et du pool dans les lignes de requêtes et de jobs | Google Cloud, où les métriques de la plateforme et les métriques basées sur les journaux en tiennent lieu ([Déployer sur Google Cloud](deploy-google-cloud.md#monitoring-and-alerts)) |
| `prometheus` | `GET /metrics` sur `metrics_listen` (par défaut `127.0.0.1:9464`). C’est un écouteur distinct du `listen` public, sans authentification, qui s’exécute pour chaque `role` | Auto-hébergé (collecté par un serveur compatible Prometheus ou KEDA) |
| `otlp` | Envoyé à un OpenTelemetry Collector via OTLP/HTTP. La destination provient des variables d’environnement OTel standard comme `OTEL_EXPORTER_OTLP_ENDPOINT` | Les configurations où vous pouvez exécuter un collecteur |
| `emf` | Lignes JSON au format CloudWatch Embedded Metric Format. Les métriques de requêtes et de jobs voyagent sur les lignes de journal de fin ci-dessus ; les autres sont écrites sur la sortie standard, une ligne par ensemble d’étiquettes, à la fin d’une requête ou d’un job | AWS ([Déployer sur AWS](deploy-aws.md#monitoring-and-alerts)) |

Les métriques sont listées ci-dessous. Dans Prometheus, `.` dans un nom devient `_` et un suffixe d’unité (`_seconds`, et `_total` pour un compteur) est ajouté.

| Métrique | Type | Étiquettes |
| --- | --- | --- |
| `http.server.request.duration` | Histogramme, secondes | `http.request.method`, `http.route` (le chemin du motif de route ; `other` si aucun ne correspond), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Compteur | `activity.type` (un type d’Activity connu, ou `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (uniquement avec `rejected` : `signature` pour une signature ou une preuve d’intégrité absente, invalide ou qui ne se vérifie pas, ou dont la clé n’a pas pu être récupérée ; `suspended` pour un domaine suspendu ; `federation_off` ; `rate_limited` ; `invalid` pour une activité sans `id`, `type`, `actor` ou `object` valide) |
| `my.relay.activities` | Compteur | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving` : si une activité transportée par un relais a été ingérée, et pourquoi elle a été abandonnée ; voir [Relais de fédération](relays.md)) |
| `my.move.activities` | Compteur | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon` : si un `Move` reçu a été enregistré, et pourquoi il a été abandonné) |
| `my.delivery.attempts` | Compteur | `my.outcome` (`success` / `retry` / `gone` / `failed` ; `gone` couvre un 410 et les livraisons ignorées parce que le serveur est indisponible ou suspendu) |
| `my.delivery.duration` | Histogramme, secondes | `my.outcome` |
| `my.ux.duration` | Histogramme, secondes | `my.ux.metric` (`lcp` / `inp` / `long_frame` / `transition` / `press` / `call_rtt` / `call_jitter`), `my.ux.name` (la route, le type d’interaction, l’action ou le transport de l’appel ; [Mesures d’expérience utilisateur](error-reporting.md#ux-measurements)) |
| `my.ux.call.packet_loss` | Histogramme, pourcentage | `my.ux.name` (le transport de l’appel) |
| `my.plugin.failures` | Compteur | `my.plugin.kind` (un type de [plugin](plugins.md)), `my.plugin.code` (le code Connect de l’échec) |
| `my.plugin.slot_wait` | Histogramme, secondes | `my.plugin.kind` (combien de temps un appel a attendu son tour parmi les appels de ce type qui s’exécutent en même temps) |
| `my.hook.decisions` | Compteur | `hook`, `mode`, `outcome` (les décisions qu’un [hook de décision](decision-hooks.md) a enregistrées) |
| `my.hook.failures` | Compteur | `hook`, `outcome` (`timeout`, `error`, `invalid` ou `breaker_open`) |
| `my.hook.duration` | Histogramme | `hook` (secondes par appel du plugin) |
| `my.job.duration` | Histogramme, secondes | `my.job.type` (un type de job enregistré, ou `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed` : `postponed` est une exécution qui n’a fait aucun travail et n’utilise aucune des tentatives du job, comme la récupération d’une carte de lien qui attend son hôte) |
| `my.job.wait` | Histogramme, secondes | `my.job.type` |
| `my.link_card.host_pauses` | Compteur | `http.response.status_code` (`429` / `503`). Un par pause des récupérations de cartes de lien auprès d’un hôte ; voir [Aperçus de liens](link-previews.md#limits-per-host) |
| `my.projection.drifts` | Compteur | `my.projection` (le nom d’une projection). Un par vérification planifiée qui a constaté que la projection avait dérivé ; voir [Vérifier et reconstruire les projections](database.md#checking-and-rebuilding-projections) |
| `my.translation.requests` | Compteur | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Un par demande de traduction d’une publication que le spectateur peut voir, ou de la publication ou du commentaire d’un signalement qu’un modérateur demande : satisfaite à partir des propres mots de l’auteur, d’une traduction conservée ou d’un fournisseur, ou refusée parce qu’aucun fournisseur ne peut recevoir la publication, qu’une limite mensuelle est atteinte, qu’aucun fournisseur ne traduit la langue, ou qu’aucun fournisseur n’a renvoyé de traduction utilisable. |
| `my.translation.lock_waits` | Compteur | Aucune. Une par demande qui a attendu une autre demande traduisant la même publication dans la même langue, au lieu de solliciter un fournisseur |
| `my.translation.characters` | Compteur | `my.provider` (l’ID d’un fournisseur de traduction). Les caractères des publications et des commentaires de signalements envoyés au fournisseur, sans marqueurs, y compris les appels qui échouent. C’est ce que comptent les [limites mensuelles](instance-settings.md#admin-screens-and-the-client-api), et cela peut différer de ce que le fournisseur facture |
| `my.timeline.delivery.lag` | Histogramme, secondes | Aucune (de la création d’une publication locale à son écriture dans les fils d’accueil et à la notification aux flux) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` pour les flux des fils local et fédéré, `voice` pour les flux des salons de chat vocal) |
| `my.stream.signals` | Compteur | `my.stream.kind` (`user` / `timeline`). Un par signal qu’un flux ouvert a traité, qu’il ait ou non envoyé quelque chose au client |
| `my.accounts.registered` | Compteur | Aucune |
| `my.statuses.created` | Compteur | `my.visibility` |
| `my.billing.imports` | Compteur | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (uniquement avec `failed` : le type d’échec, comme dans [Importer les frais d’utilisation du cloud](billing-import.md)). Chaque tentative compte, y compris les nouvelles tentatives et les échecs qui attendent l’import quotidien suivant |
| `my.supporter.tip_checkouts` | Compteur | `my.outcome` (`started` / `rate_limited` / `paid` / `expired`). Un par démarrage d’un pourboire qui a obtenu la page du fournisseur, par démarrage que les limites ont refusé, et par pourboire payé ou terminé sans paiement ; voir [pourboires](supporters.md#tips) pour les alertes qu’il sert |
| `my.voice.turn.credentials` | Compteur | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Un par identifiant TURN qu’un participant à un chat vocal a demandé en rejoignant un salon ou en renouvelant sa connexion : `reused` signifie que des identifiants Cloudflare émis plus tôt pour la même connexion ont été renvoyés, et `failed` que Cloudflare n’a pas pu être joint ou a refusé, de sorte que le participant a reçu une erreur et n’a pas pu se connecter |
| `my.voice.sfu.rooms`, `my.voice.sfu.sessions` | Jauge | Aucune (par nœud média de `voice = sfu` : les salons ayant des connexions sur le nœud et les sessions qui y sont connectées) |
| `my.voice.sfu.capacity.reserved`, `my.voice.sfu.capacity.connected` | Jauge | Aucune (par nœud média : la somme des maximums des salons placés sur lui et les sessions qui y sont connectées, toutes deux face à `voice_sfu_node_capacity`) |
| `my.voice.sfu.packets` | Compteur | `my.outcome` (`forwarded`, un par participant auquel un paquet audio a été envoyé ; ou pourquoi un paquet a été abandonné : `silent`, `not_allowed` pour un participant qui n’a pas le droit de parler ou est coupé, `queue_full` pour un participant dont la connexion n’a pas pu suivre) |
| `my.voice.sfu.bytes` | Compteur, octets | Aucune (l’audio que le nœud média a transmis) |
| `my.voice.sfu.renegotiations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.migrations` | Compteur | Aucune (nouvelles offres après la première, parce que les intervenants ont changé ; relances ICE que des clients ont demandées après un changement de réseau ; sessions qu’un nœud en arrêt a envoyées à un autre nœud) |
| `my.voice.sfu.ticket_refusals` | Compteur | `my.voice.sfu.reason` (`invalid` pour un ticket expiré, déjà utilisé ou inconnu, `other_node` pour un ticket d’un autre nœud) |
| `my.voice.sfu.client.rtt` | Histogramme, secondes | Aucune (le temps d’aller-retour jusqu’aux participants, d’après leurs rapports RTCP) |
| `my.voice.sfu.client.packet_loss` | Histogramme, pourcentage | Aucune (la perte sur le chemin vers les participants, d’après leurs rapports RTCP) |
| `my.errors.exported` | Compteur | `my.outcome` (`success` / `dropped` / `failed` : un par signalement d’erreur pour le service de `errors_exporter`. `dropped` signifie que le signalement n’a pas été envoyé parce que 1 000 signalements attendaient déjà ou que le service a demandé au serveur de ralentir, et le serveur attend aussi longtemps que le service le demande avant de renvoyer ; `failed` signifie que le service n’a pas pu être joint ou a refusé le signalement. Un signalement est envoyé une fois et jamais réessayé, et les signalements encore en attente 2 secondes après le début d’un arrêt sont perdus) |
| `my.queue.ready` | Jauge | `my.job.type` (le nombre de jobs exécutables dans la file de la base de données) |
| `my.queue.oldest_ready_age` | Jauge, secondes | `my.job.type` (depuis combien de temps attend le plus ancien job exécutable de chaque type dans la file de la base de données ; un type sans job exécutable n’a pas de valeur) |
| `my.queue.dead` | Jauge | Aucune |
| `my.instance.users`, `my.instance.statuses` | Jauge | Aucune (les nombres d’utilisateurs locaux et de publications, comme dans NodeInfo) |
| `my.media.stored_bytes` | Jauge, octets | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Jauge, octets | Aucune (la taille de la base de données : les relations du schéma courant pour PostgreSQL ; le fichier de la base de données sans son journal d’écriture anticipée pour SQLite) |
| `my.billing.import.failing` | Jauge | `my.billing.source` (1 lorsque le dernier import de la source a échoué, 0 sinon) |
| `my.federation.instances` | Jauge | `my.federation.instance.state` (`available` : les serveurs auxquels les livraisons vont encore mais qui ont échoué au moins un jour depuis leur dernière réponse ; `unavailable` : les serveurs dont les livraisons sont suspendues) |
| `my.metrics.snapshot.age` | Jauge, secondes | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Compteur | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Un par [signalement d’erreur](#error-reports) |
| `my.tls.certificate.expiry` | Jauge, secondes | `host` (le temps restant avant l’expiration du certificat que ce processus sert pour l’un de ses hôtes, avec `tls = acme` ou `tls = files` ; chaque processus signale le sien, de sorte qu’un processus resté avec un ancien certificat se voit. Voir [Commandes et métriques](#commands-and-metrics)) |
| `my.tls.order.attempts` | Compteur | `host`, `result` (`issued` / `failed`). Un par commande d’un certificat auprès de l’autorité de certification ACME, par le processus qui l’a passée, `my tls renew` et `my tls revoke` compris |
| `my.tls.order.consecutive_failures` | Jauge | `host` (les commandes depuis le certificat actuel de l’hôte qui n’en ont obtenu aucun : le nombre qui fixe l’attente avant la commande suivante) |
| `my.tls.rounds` | Compteur | Aucune. Un par tour du gestionnaire de certificats sur ses hôtes avec `tls = acme` ; un nombre qui cesse de croître signifie que le gestionnaire s’est arrêté |

Les vérifications de santé (`/healthz`, `/readyz`, `/health/live`, `/health/ready`) ne sont pas mesurées. Aucune étiquette ne contient de domaine distant, de compte, d’URL ni d’IP de client.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` et `my.billing.import.failing` sont des valeurs pour toute l’instance : chaque processus signale la même valeur à partir d’un instantané conservé dans le magasin clé-valeur.

<!-- source: system-metrics 973069859f11 -->
### Métriques système

Les métriques sur le processus, le runtime Go et les connexions aux ressources sont exportées par le même `metrics_exporter`.
Le CPU des conteneurs, la mémoire, le nombre d’instances et les requêtes entrantes sont laissés aux métriques de la plateforme ; celles-ci couvrent la saturation à l’intérieur du processus que la plateforme ne peut pas voir.
Ne les utilisez pas pour déclencher la mise à l’échelle ; utilisez-les pour les alertes et pour estimer le plafond d’instances.

| Métrique | Type | Étiquettes |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Jauge, octets | Seul `go.memory.used` a `go.memory.type` (`stack` / `other`). `go.memory.limit` n’apparaît que lorsque `GOMEMLIMIT` est défini |
| `go.memory.allocated`, `go.memory.allocations` | Compteur | Aucune |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Jauge | Aucune |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histogramme, secondes | Aucune (l’attente entre le moment où une goroutine devient exécutable et son exécution, et les pauses du GC) |
| `go.memory.gc.cycles` | Compteur | Aucune |
| `process.cpu.time` | Compteur observable, secondes (total depuis le démarrage) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Jauge, octets | Aucune (mémoire résidente, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Jauge | Aucune (les descripteurs de fichiers ouverts et la limite) |
| `process.uptime` | Jauge, secondes | Aucune |
| `db.client.connection.count` | Jauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Jauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Compteur observable (total depuis le démarrage ; secondes ou nombre) | `db.client.connection.pool.name`. `timeouts` n’existe que pour PostgreSQL |
| `my.adapter.operation.duration` | Histogramme, secondes | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (connexions `valkey`, et abonnés `stream.<stream>`) |
| `my.adapter.reconnects` | Compteur | `my.adapter` |
| `http.client.request.duration` | Histogramme, secondes | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots` / `domain_list`), `http.request.method`, `my.http.status_class` (`2xx` et ainsi de suite), `error.type` (`forbidden_destination` est un refus par la protection SSRF) |
| `http.client.open_connections` | UpDownCounter | Aucune |
| `my.readiness.failures` | Compteur | Aucune |
| `my.readiness.shared` | Compteur | Aucune. Un par requête de disponibilité qui a repris le résultat d’une vérification déjà en cours au lieu d’en démarrer une |
| `my.http.origin_refused` | Compteur | Aucune. Un par requête refusée parce qu’elle n’est pas passée par le CDN ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Les sondes de l’adresse de l’origine le font augmenter lentement ; un bond alors que le trafic s’arrête signifie que le CDN n’envoie plus l’en-tête. Le mode maintenance ne mesure rien, de sorte que seul son journal d’accès les montre |

- Les métriques du runtime Go (`go.*`) ne sont exportées que par `prometheus` et `otlp`. Avec `emf`, un job d’instantané périodique écrit les jauges du processus et du pool de connexions ainsi que les compteurs observables.
- Les attentes du pool et le temps CPU sont des compteurs observables : le pool et le noyau conservent des totaux depuis le démarrage, et ils sont lus au moment de la collecte. Dans Prometheus, ce sont des compteurs `_total` (comme `process_cpu_time_seconds_total`), dans OTLP ce sont des sommes monotones, et vous voyez leur croissance avec `rate()` de PromQL.
  `emf` n’a pas de compteurs totaux ; il écrit donc l’augmentation depuis l’écriture précédente (additionnez-les avec la statistique Sum de CloudWatch).
- Le `claim` de la file n’est mesuré que lorsqu’il prend un job ou échoue. L’interrogation d’un worker en attente n’enregistre rien.
- `db.client.connection.pending_requests` n’est pas exporté : les statistiques de pgx et de `database/sql` ne signalent pas combien attendent ; le nombre et la durée des attentes en tiennent donc lieu.
- Mettez en place une alerte sur le rapport entre la somme de `go.memory.used` et `go.memory.limit`, et sur la croissance de `db.client.connection.wait_time` (la limite du pool est atteinte).

<!-- source: memory-limit-gomemlimit 66320a6c7b76 -->
### Limite de mémoire (`GOMEMLIMIT`)

`GOMEMLIMIT` est une variable d’environnement du runtime Go que l’opérateur définit ; l’application ne la choisit pas.
Sans elle, Go s’exécute avec sa valeur par défaut (aucune limite), et le tas ne se vide pas de façon anticipée à l’approche de la limite de mémoire du conteneur.
Avec une limite de mémoire de conteneur, définissez-la à environ 90 % de cette limite (par exemple `GOMEMLIMIT=460MiB` pour un conteneur de 512 Mio). Les modules OpenTofu la définissent ainsi pour chaque service, d’après sa taille.
Ne définissez pas `GOMAXPROCS` : la valeur par défaut de Go suit la limite de processeur du conteneur.

<!-- source: traces 15eb90e8584a -->
## Traces

Le réglage `traces_exporter` (`none` par défaut) exporte des traces distribuées. Ses réglages associés se trouvent dans la [référence de configuration](../reference/configuration.md).

| Span | Nom | Attributs |
| --- | --- | --- |
| Requête HTTP (serveur) | `<method> <http.route>` (`<method>` pour une requête qui n’a atteint aucune route) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Un 5xx est un échec |
| Exécution d’une tâche (consommateur) | Le type de tâche (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Chaque nouvelle tentative est un span distinct |
| Transaction | `my.db.transaction` | `db.system.name`. Chaque nouvelle tentative est un span distinct |
| Requête (client) | Le nom de la requête sqlc (comme `InsertJobs` ; sinon le premier mot du SQL, comme `BEGIN`) | `db.system.name`, `db.operation.name` |
| Envoi (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| HTTP sortant (client) | `<method>` | `http.request.method`, `server.address` (uniquement le nom d’hôte de la destination), `http.response.status_code`. Un 4xx ou un 5xx est un échec |

Un span en échec a `error.type` (un nom fini tel que `timeout`, ou un code de statut). Les attributs ne contiennent jamais d’ID de compte, de publication ou de tâche, d’URL, de chemins ou de requêtes, d’IP clientes, de valeurs de User-Agent, de texte SQL ni de chaînes d’erreur.
`server.address` laisse les noms d’hôte des serveurs distants au récepteur ; c’est donc vous qui décidez combien de temps le récepteur les conserve et qui peut les consulter. Les vérifications d’état et les requêtes refusées par `origin_secret` ne créent aucun span.

- Une exécution de tâche n’est pas l’enfant du span qui l’a mise en file (généralement une requête HTTP). C’est la **racine d’une nouvelle trace** avec un lien vers le span qui l’a mise en file. Dans un visualiseur, suivez le lien depuis la trace de la requête.
- Un `traceparent` entrant ne devient le parent que lorsque le pair connecté figure dans `trusted_proxies` et, avec [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), que la requête porte le secret. Sinon, il est écarté et une nouvelle trace commence. `tracestate` est toujours écarté. Le HTTP sortant (fédération, WebFinger, médias, Web Push, etc.) ne porte aucun `traceparent`.
- Échantillonnage : une requête est enregistrée si son parent de proxy de confiance l’a été, ou si la cible du lien d’une tâche l’a été ; sinon elle est enregistrée selon le ratio `traces_sample_ratio` (0,01 par défaut). Abaisser le ratio ne laisse jamais sans leur trace les tâches qui prolongent une requête enregistrée.

| Cible | Réglages | Chemin |
| --- | --- | --- |
| Par défaut, petit auto-hébergement | `traces_exporter = none` | Aucun span n’est exporté. Suivez `request_id` et `job_id` dans les journaux. Si un proxy de confiance fournit un `traceparent`, son `trace_id` et son `span_id` sont ajoutés aux journaux |
| Auto-hébergement (Jaeger, Tempo, un OpenTelemetry Collector, etc.) | `traces_exporter = otlp`, `traces_flush = batch` | Envoyé par OTLP/HTTP au récepteur à `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (ou `OTEL_EXPORTER_OTLP_ENDPOINT`, `http://localhost:4318` par défaut). Les en-têtes d’authentification vont dans `OTEL_EXPORTER_OTLP_HEADERS`. Pour conserver de façon fiable les erreurs et les traces lentes, utilisez l’échantillonnage en queue du Collector |
| Google Cloud | `traces_exporter = none` | Cloud Trace montre la trace que Cloud Run enregistre pour chaque requête : [Déployer sur Google Cloud](deploy-google-cloud.md#traces) |
| AWS | `traces_exporter = otlp`, vers un collector dans chaque tâche | X-Ray : [Traces vers X-Ray](deploy-aws.md#traces-to-x-ray) |

<!-- source: profiles d19dd2bf22e2 -->
## Profils

Les [métriques système](#system-metrics) disent combien de mémoire et de CPU un processus utilise ; un profil du runtime Go dit ce qui les utilise.
`pprof_listen` nomme l’adresse d’un écouteur qui sert les profils sous `/debug/pprof/`, pour chaque `role`. Il est vide par défaut, ce qui n’ouvre aucun écouteur.

```sh
MY_PPROF_LISTEN=127.0.0.1:6060 my serve

go tool pprof -sample_index=inuse_space http://127.0.0.1:6060/debug/pprof/heap   # what holds the heap now
go tool pprof -sample_index=alloc_space http://127.0.0.1:6060/debug/pprof/heap   # what has allocated since the start
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'             # where the CPU goes, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'                       # every goroutine, by where it waits
go tool pprof 'http://127.0.0.1:6060/debug/pprof/mutex?seconds=30'               # where goroutines wait on locks, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/heap?debug=1' | grep '^# '               # the runtime's memory statistics
```

L’écouteur n’a pas d’authentification, un profil montre l’intérieur du processus, et en prendre un coûte du CPU : gardez-le sur loopback et atteignez-le par un tunnel (`ssh -L 6060:127.0.0.1:6060 <host>`, `kubectl port-forward`), puis désactivez-le lorsque vous avez terminé.
Le profil du tas ne couvre que le tas Go. La mémoire résidente du processus contient aussi les pages du binaire qui ont été lues, les piles et les propres structures du runtime ; les statistiques de mémoire les donnent.
Les profils de blocage et de mutex n’échantillonnent que pendant qu’une requête avec `seconds` les prend, et sont vides sans elle.

<!-- source: stop-the-server 72d71a40815a -->
## Arrêter le serveur

Sur SIGTERM ou SIGINT, le serveur s’arrête proprement.

1. Il cesse d’accepter de nouvelles connexions et de prendre des tâches.
2. Il attend jusqu’à 25 secondes que les requêtes en cours et les tâches en cours d’exécution se terminent.
3. Après 25 secondes, il annule le contexte des requêtes et des tâches restantes et ferme les connexions.
   Il laisse ensuite à une tâche annulée jusqu’à 3 secondes de plus pour enregistrer son résultat, et se termine avec le code 1 (28 secondes au plus en tout, dans le délai de grâce par défaut de 30 secondes de Kubernetes et d’ECS).
   Une tâche annulée est retentée. Une tâche qui ne s’arrête pas dans ces 3 secondes, ou qui est tuée en cours de route, est abandonnée et reprise par un worker lorsque son bail expire ; elle peut donc s’exécuter deux fois.

Un processus qui exécute un nœud média de `voice = sfu` (`role = sfu`, ou `all`) le draine d’abord : il cesse de prendre des salons, déplace
ses salons sur d’autres nœuds et dit à leurs clients de s’y reconnecter, puis attend jusqu’à 20 des 25 secondes qu’ils
partent avant d’attendre les requêtes. Donnez-lui le même délai de grâce d’au moins 30 secondes (la valeur par défaut de Docker est 10 :
définissez `stop_grace_period` ou `docker stop -t 30`). Voir [Chat vocal](voice.md#drain-a-node-for-a-deploy).

Une panique dans l’une des tâches d’arrière-plan du serveur déclenche le même arrêt, afin que le serveur ne continue jamais de s’exécuter dans un état défectueux.
La ligne de journal `serve failed` nomme la tâche, et le processus se termine avec le code 1 pour que la plateforme ou le gestionnaire de processus le redémarre.

Pour un travail sur la base de données, le magasin clé-valeur ou la file, redémarrez en mode maintenance (`MY_MAINTENANCE=true`) plutôt que de l’arrêter. Consultez [Maintenance](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 1fd5aaccbff5 -->
## La file de jobs et les jobs en échec (morts)

[Jobs](../reference/jobs.md) liste chaque type de job : ce qu’il fait, ce qui le déclenche, comment il réessaie et ce que fait son ré-exécution.

La page d’administration des jobs (Administration → Jobs, `/admin/jobs`, permission `manage_jobs`) comporte trois pages :

- **État de la file** (`/admin/jobs`) : d’abord combien de jobs ont échoué, avec un lien vers eux ; le nombre par état (en file, en cours, en attente de nouvelle tentative, en échec), chacun menant aux jobs de cet état ; depuis combien de temps attend le plus ancien job dû ; et un tableau des nombres par type et par état avec le temps pendant lequel chaque type fait attendre un job.
- **Liste des jobs** (`/admin/jobs/list`) : les jobs qui ne sont pas terminés (en file, en cours, en attente de nouvelle tentative), ou d’un état, et d’un type, une page à la fois, avec le type, le nombre de tentatives, l’heure à laquelle il est dû ou a échoué, l’état et la première ligne de la dernière erreur.
- **Un job** (`/admin/jobs/<id>`, depuis le type dans la liste) : **Exécuter de nouveau** pour un job en échec, qui demande d’abord confirmation, car une exécution peut répéter ce que le job a fait avant d’échouer ; chaque fait du job, son ID et ses clés chacun avec un bouton de copie ; sa dernière erreur et sa charge utile, chacune avec un bouton de copie ; un tableau des tentatives infructueuses, les plus récentes d’abord (résultat, heure de fin, worker et erreur) ; et un lien vers les jobs de son type.

L’API d’administration comprend `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` et `POST /api/v1/admin/jobs/{id}/retry`.
Un job terminé n’est plus dans la file ; sa page est donc introuvable.

- Les jobs en attente, en cours et en attente de nouvelle tentative ne peuvent être listés que pour les files dont le serveur détient les jobs : `queue = db`, `queue = cloudtasks` (les jobs sont aussi dans la base de données) et `queue = inline`. Les jobs en échec et l’historique des tentatives se trouvent dans la base de données de l’application pour toutes les files, et peuvent donc toujours être listés.
- L’historique des tentatives ne conserve que les tentatives infructueuses (les nouvelles tentatives et la tentative qui a finalement échoué). Suivez une exécution réussie par la ligne de journal structurée `job` (qui porte `job_id`).
  L’historique est supprimé par le nettoyage horaire une fois que `job_attempt_retention` (30 jours par défaut ; voir le [paramètre](../reference/configuration.md#job_attempt_retention)) s’est écoulé depuis la fin de la tentative.
  L’historique ne contient ni charge utile ni journal de gestionnaire, et un message d’erreur est tronqué à ses 2 000 premiers octets. Les gestionnaires de jobs tiennent les adresses e-mail, les adresses IP, les jetons et les corps de publications hors des messages d’erreur.
  Pour une vue limitée aux types de jobs d’e-mail, voir [Envoyer des e-mails](email.md).

Un job dont le gestionnaire a décidé qu’il avait échoué définitivement, et un job qui a épuisé ses nouvelles tentatives, devient un job mort dans la file de la base de données (`queue = db` et `queue = cloudtasks`).
`my job` les liste, les relance et les supprime. L’état « En échec » de la page des jobs et l’API d’administration peuvent aussi les lister et les relancer.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Si un job avec la même clé d’idempotence est déjà en attente, ce job tient lieu de celui qui est relancé. Les gestionnaires de jobs sont idempotents ; remettre le même job en file un nombre quelconque de fois est donc sans risque.
- Un ID inconnu est affiché et la commande se termine avec le code 1 ; les autres ID sont quand même traités. Le code de sortie 2 signifie une erreur d’utilisation.
- Un job mort que personne ne relance est supprimé par le nettoyage horaire une fois que `dead_job_retention` (30 jours par défaut ; voir le [paramètre](../reference/configuration.md#dead_job_retention)) s’est écoulé depuis l’échec.
- Les jobs morts de `queue = inline` vivent dans la mémoire du processus du serveur ; `my job` ne peut donc pas les gérer (code de sortie 1).

<!-- source: splitting-workers-by-job-type e3b4c4374dfb -->
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

<!-- source: running-several-jobs-at-once 2a1137a12e8f -->
### Exécuter plusieurs jobs à la fois

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) définit combien de jobs un processus exécute à la fois, de 1 à 128. Il fonctionne avec `queue = db` ; avec Cloud Tasks, c’est la plateforme qui en décide. La valeur par défaut est un job à la fois, et une valeur de 1 se comporte de même.

Augmentez-le pour les jobs qui passent leur temps à attendre d’autres serveurs, comme `federation.deliver`. Ceux-ci utilisent peu de mémoire et peu de connexions à la base de données par job, de sorte qu’un processus fait le travail de beaucoup : dans un test local où un job n’attendait qu’une réponse de 200 ms, un processus avec 32 emplacements égalait 32 processus à un emplacement chacun, en utilisant une petite part de la mémoire. Ces jobs ne faisaient rien d’autre ; mesurez donc votre propre instance. Associez-le à `worker_job_types`, et donnez à chaque type de job un processus avec son propre nombre.

Gardez ces limites à l’esprit :

- **Connexions à la base de données** : un job a besoin d’une connexion pour être réclamé, pour son propre travail et pour enregistrer son résultat. Le paramètre doit être inférieur au maximum du pool, sinon le serveur ne démarre pas. Le maximum est `pool_max_conns` dans `database_url`, par défaut le plus grand de 4 et du nombre de CPU, si bien qu’un vCPU en autorise 3 tant que vous ne l’augmentez pas. Vérifiez que votre base de données ou votre pooler de connexions accepte les connexions de tous vos processus.
- **SQLite** : il n’écrit qu’une transaction à la fois, et dans un test local il était plus lent à 128 qu’à 32.
- **Jobs gourmands en CPU** : les emplacements se partagent le CPU et la mémoire d’un seul processus. `media.process` attend son tour à une limite de la moitié des CPU par défaut, et l’attente compte dans le bail de 5 minutes du job ; un processus plein de `media.process` peut donc dépasser le bail et recommencer. Donnez à de tels jobs un processus avec peu d’emplacements.
- **Un serveur à la fois** : un processus établit au plus 8 connexions vers un même hôte ; un processus ne livre donc pas à un seul serveur plus vite que 8 connexions le permettent, quel que soit son nombre d’emplacements. Là où la plupart des livraisons vont vers quelques gros serveurs, davantage de processus aident là où davantage d’emplacements n’aident pas.
- **Un serveur bloqué retient des emplacements** : les emplacements qui attendent un serveur lent ne sont pas libres pour d’autres jobs ; gardez donc les livraisons séparées des e-mails et des notifications.
- **Les déploiements consomment des tentatives** : lorsque le serveur s’arrête, les jobs encore en cours après `ShutdownTimeout` (25 secondes par défaut) sont annulés, et chaque job annulé consomme une tentative. Plus il y a d’emplacements qui exécutent des jobs lents, plus chaque déploiement consomme de tentatives. Un job annulé dont le résultat n’est pas enregistré à temps attend l’expiration de son bail avant de s’exécuter de nouveau.
- **Un plantage emporte tous les emplacements** : si le processus manque de mémoire, tous ses jobs attendent l’expiration de leurs baux.

Pour décider d’augmenter, lisez `my.job.wait` par `my.job.type` : si les jobs d’un type attendent longtemps alors que `my.job.duration` est surtout du temps passé à attendre d’autres serveurs et que le CPU et la mémoire restent bas, davantage d’emplacements devraient aider. Si les attentes restent élevées après l’augmentation, regardez les connexions à la base de données et la limite par hôte ci-dessus avant d’augmenter de nouveau.

Tant qu’aucun job n’est dû, un emplacement interroge et les autres attendent ; un processus inactif coûte donc à la base de données ce que coûte un processus à un seul job.

<!-- source: waiting-for-a-database-connection 795c0fd7f769 -->
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
  - Sur Google Cloud, une instance accepte de nombreuses requêtes à la fois : [Un PostgreSQL externe](deploy-google-cloud.md#an-external-postgresql) dimensionne son pool.
- **Des `503` fréquents ou des `db.client.connection.timeouts`** signifient que le pool est trop petit ou que les requêtes sont devenues plus lentes ; un délai plus long ne fait que le masquer. Suivez [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks f14d9721ba24 -->
### Cloud Tasks

[Déployer sur Google Cloud](deploy-google-cloud.md) met en place tout ce qui suit avec des modules OpenTofu.

Avec `queue = cloudtasks`, les tâches restent dans la base de données (`database = postgres`), et Google Cloud Tasks ne fait que réveiller le serveur pour chacune d’elles :

- Pour chaque tâche qu’il stocke, relance ou remet en file, le serveur crée une tâche dans `queue_cloudtasks_queue` une fois la transaction validée. La tâche appelle `POST {queue_cloudtasks_url}/_internal/jobs` à l’heure à laquelle la tâche est due, avec l’ID de la tâche comme corps et un jeton d’ID OIDC de `queue_cloudtasks_service_account`. Une tâche due à plus de 30 jours est signalée à 30 jours, et de nouveau lorsque cette tâche arrive.
- Le processus qui reçoit la tâche exécute le travail et répond `204` une fois son résultat enregistré, y compris lorsque le travail a déjà été exécuté ou n’est pas dû. Une tâche qui échoue est relancée par le serveur à sa prochaine heure de tentative, pas par Cloud Tasks. Cloud Tasks ne relance une tâche que lorsque la réponse est `5xx` (la base de données n’a pas pu être jointe) ou `401` (un jeton que le serveur a refusé) ; son `retryConfig` n’a donc besoin d’aucun réglage.
- Aucun processus n’interroge la table et aucun processus n’exécute de lui-même les tâches périodiques. Un job Cloud Scheduler appelle `POST {queue_cloudtasks_url}/_internal/tick`, chaque minute (`* * * * *`) par défaut, avec un jeton OIDC du même compte de service (et l’audience de `queue_cloudtasks_audience`, lorsqu’elle est définie). Chaque tick met en file les tâches périodiques dont la période est venue, une fois pour toute l’instance (les périodes sont réservées dans le KV ; `kv` doit donc être `db` ou `valkey`), et signale de nouveau les tâches dues depuis plus de 2 minutes sans s’être exécutées, ce qui récupère un signal perdu.
- `/_internal/jobs` et `/_internal/tick` sont servis par les processus avec `role = all` ou `worker`, et n’acceptent qu’un jeton signé par Google pour `queue_cloudtasks_service_account` avec une adresse e-mail vérifiée, dont la seule audience est `queue_cloudtasks_audience` ou, par défaut, l’URL appelée. Toute autre requête reçoit un `401`.
- Une livraison exécute une tâche, pendant au plus le bail de 5 minutes. Gardez `queue_cloudtasks_dispatch_deadline` (10 minutes par défaut) et le délai d’attente des requêtes du service Cloud Run supérieurs à cette durée.
- Le compte de service propre du serveur a besoin de `roles/cloudtasks.enqueuer` sur la file et de `roles/iam.serviceAccountUser` sur `queue_cloudtasks_service_account`. Il s’authentifie avec les Application Default Credentials.

<!-- source: how-late-periodic-jobs-run 775fb52d1df8 -->
### Avec quel retard s’exécutent les tâches périodiques

Une tâche périodique s’exécute avec jusqu’à une période de son déclencheur après son heure, plus le temps que met la plateforme à la démarrer. C’est ce qui détermine le retard avec lequel arrivent les rappels et les notifications de début des événements et avec lequel s’ouvrent leurs salons de chat vocal, puisque `gathering.queue_due` les démarre : un rappel a un retard pouvant aller jusqu’à l’intervalle du tableau. Les valeurs sont celles que donnent le code de l’ordonnanceur et le déclencheur de chaque plateforme ; elles n’ont pas été mesurées sur un serveur en fonctionnement.

| Plateforme | Ce qui démarre les tâches périodiques | Retard attendu d’un rappel ou du début d’un événement |
| --- | --- | --- |
| Auto-hébergement et [Docker Compose](deploy-docker-compose.md) (`queue = db`) | L’ordonnanceur de chaque processus avec `role = all` ou `worker` vérifie une fois par minute, à partir du démarrage du processus, et met en file chaque tâche une fois par période pour tout le serveur | Jusqu’à 1 minute, plus environ une seconde pour qu’un worker prenne la tâche |
| [AWS](deploy-aws.md) (`queue = db`) | Le même ordonnanceur, dans chaque tâche | Jusqu’à 1 minute, plus environ une seconde |
| [Google Cloud](deploy-google-cloud.md) (`queue = cloudtasks`) | Cloud Scheduler appelle `/_internal/tick` selon `queue.tick_schedule`, chaque minute par défaut ; Cloud Tasks réveille ensuite le serveur pour chaque tâche | Jusqu’à 1 minute, plus le démarrage d’une instance lorsqu’aucune ne tourne (un démarrage à froid, généralement quelques secondes). Avec un `tick_schedule` plus long, jusqu’à cet intervalle ([Là où le soin le réduit](deploy-google-cloud.md#where-care-reduces-it)) |

Un rappel qui arriverait après le début de son événement n’est pas envoyé, pas plus qu’une notification de début après la fin de l’événement.
Avec un tick de plus de 5 minutes, les rappels les plus courts peuvent donc être perdus.

<!-- source: routing-job-types-to-their-own-queues f4098db6215c -->
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
Sur Google Cloud, `workers` donne à chaque worker sa propre file et son propre service, et à chaque service les mêmes routes et permissions :
[Répartir les jobs en workers](deploy-opentofu.md#splitting-jobs-into-workers).

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

<!-- source: health-checks e268d13fe7f2 -->
## Vérifications d’état

| Chemin | Type | Quand il renvoie 200 |
| --- | --- | --- |
| `GET /healthz`, `GET /health/live` | vivacité | Le processus peut répondre en HTTP. Il ne regarde pas les dépendances telles que la base de données (de sorte qu’une base de données lente ne redémarre pas le processus) |
| `GET /readyz`, `GET /health/ready` | disponibilité | La base de données est joignable et le schéma est appliqué |
| `GET /api/v1/instance/health` | Surveillance de disponibilité, pages d’état et clients (servi par le rôle `web`) | Comme la disponibilité. Le résultat est mis en cache dans le processus pendant 60 secondes |

- Cloud Run réserve certains chemins qui se terminent par `z`, dont `/healthz`, et y répond lui-même par un `404` ; derrière lui, appelez `/health/live` et `/health/ready`, qui répondent de la même façon.
- Un 200 de n’importe lequel d’entre eux a pour corps `{"status":"ok"}`.
- `/readyz` vérifie la base de données à chaque requête, et les requêtes simultanées partagent une seule vérification.
- `/api/v1/instance/health` renvoie 200 et `{"status":"ok","checked_at":"<RFC 3339>"}` lorsque tout va bien, et 503 et `{"status":"unavailable","checked_at":...}` en cas d’échec (`Cache-Control: no-store`).
  Chaque processus ne touche la base de données qu’une fois par minute au plus, quelle que soit la fréquence à laquelle il est surveillé ou sollicité, et il ne regarde ni le jeton d’accès ni la limite de débit. Faites pointer les moniteurs de disponibilité vers ce point de terminaison, pas vers la sonde (`/readyz` ou `/health/ready`).
  Pour la configuration par service, l’intervalle de surveillance et son coût, et le traitement de la maintenance, voir le [guide de surveillance de la disponibilité](monitoring.md).
- Les trois écrivent au niveau Debug dans le journal d’accès et ne créent aucun span de trace.
- Lorsque la disponibilité n’est pas assurée, la réponse est un 503 avec un corps RFC 9457 `application/problem+json`.
  Son `title` est traduit selon `Accept-Language` et ne contient aucun détail interne. La cause (une erreur de connexion, ou les noms des tables manquantes) va dans le journal.
- Une vérification de disponibilité est interrompue au bout de 3 secondes.
- La disponibilité exige aussi que la base de données contienne les `host` et `account_domain` configurés, ou que le processus ait pu les enregistrer.
- **Schéma appliqué** signifie que toutes les tables que déclarent avec `CREATE TABLE` les fichiers de schéma de la base de données (`schema/<NN>-<module>/<dialect>.sql`) existent
  (SQLite est vérifié par `sqlite_schema`, PostgreSQL par le `current_schema()` de la connexion). `my schema apply` applique tout le schéma ; une table manquante signifie donc que le schéma n’est pas appliqué ou qu’il est plus ancien que le binaire.
  Les différences de colonnes et d’index ne sont pas vérifiées ; `my schema apply -dry-run` affiche ce qui changerait.

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

<!-- source: web-client 0a0b62acc010 -->
## Client web

Un processus dont le `role` est `web` ou `all` sert le client web intégré au binaire.

| Chemin | Contenu | `Cache-Control` |
| --- | --- | --- |
| Tout `GET` qu’aucune autre route ne reconnaît (`/`, `/@alice`, `/@alice/status/{id}`, `/settings`, etc.) | La coquille HTML de la SPA. Les paramètres de l’instance sont transmis dans `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Fichiers de construction avec des noms hachés | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | Le manifeste de l’application web construit à partir des paramètres de l’instance | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` et les autres fichiers directement sous la construction | Servis tels quels | `no-cache`, ETag |
| `GET /licenses/*` | Les licences de ce que la construction embarque (avis des paquets npm, et `LICENSE-Unicode.txt` et `OFL.txt` pour les données non npm embarquées). Servis en `text/plain; charset=utf-8` quelle que soit l’extension | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Construits à partir des paramètres de l’instance (voir [Moteurs de recherche et robots d’IA](crawlers.md)) | `public, max-age=3600`, ETag |

- Les chemins sous `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (la feuille de style du thème du serveur) et `/licenses`, ainsi que `/robots.txt`, ne renvoient jamais la coquille ; si aucune autre route ne correspond, ils renvoient un problème 404.
- La coquille est servie après la négociation de contenu ActivityPub. Demander un chemin de coquille avec un `Accept` ActivityPub renvoie 404 (les pages de publication redirigent, comme décrit ci-dessous).
  La coquille porte toujours `Vary: Accept`. Lorsque vous placez un CDN devant, incluez le type d’`Accept` (ActivityPub ou HTML) dans la clé de cache, ou excluez du cache la coquille, `/ap/*` et `/@*`.
  Ne servez par le CDN que `/assets/*`.
- Les fichiers texte de la construction (scripts, feuilles de style, images SVG, avis de licence) sont intégrés compressés uniquement. Une requête qui accepte Brotli les reçoit avec `Content-Encoding: br`, une qui accepte gzip avec `Content-Encoding: gzip`, et toute autre requête les reçoit décompressés, toujours avec `Vary: Accept-Encoding` et un ETag pour chaque encodage.
  Un proxy placé devant n’a besoin d’aucune compression propre pour eux, et un CDN doit conserver `Vary: Accept-Encoding` (ou l’encodage) dans sa clé de cache pour `/assets/*`.
- La coquille et les écrans de connexion portent des en-têtes `Link` qui nomment les feuilles de style et le script d’entrée que la page charge en premier, à partir desquels un CDN comme Cloudflare (sa fonction Early Hints) peut produire ses propres indications précoces.
  Avec [`web_early_hints`](../reference/configuration.md#web_early_hints) = `true`, le serveur les envoie aussi en premier dans une réponse `103 Early Hints`, afin que le navigateur récupère les fichiers pendant que le serveur prépare la page. Il est désactivé par défaut, car un proxy qui ne sait pas transmettre un 103 peut faire échouer la requête :
  - Un Application Load Balancer externe de Google Cloud répond 502 ou 503 (`backend_interim_response_not_supported`) : laissez-le désactivé derrière un tel répartiteur, comme le fait le déploiement Cloud Run.
  - nginx le transmet à partir de la 1.29.0, avec `early_hints` défini.
  - Le `reverse_proxy` de Caddy le transmet ; une directive `encode` l’abandonne dans les versions antérieures à la 2.8.
  - Les navigateurs n’utilisent un 103 que pour une page vers laquelle ils naviguent, et Chrome et Edge seulement via HTTP/2 et HTTP/3.
- Un binaire construit sans le client web (un simple `go build`, sans construire d’abord le client) journalise un avertissement au démarrage et sert une coquille sans scripts.

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
