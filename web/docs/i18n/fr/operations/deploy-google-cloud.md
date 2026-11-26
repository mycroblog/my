<!-- source: deploying-on-google-cloud 03b87618467f -->
# Déployer sur Google Cloud

Comment exécuter `my` sur Google Cloud avec le module OpenTofu `mycroblog-gcp` : un service Cloud Run, et un backend pour chaque rôle
que vous choisissez par le `kind` de sa section. Chaque section prend par défaut la recommandation du module, qui n’exécute aucune instance
tant que personne n’utilise le serveur, sauf pour un tick qui la réveille chaque minute. Un serveur coûte de l’argent : la configuration recommandée avec une
base de données sur Neon revient à environ 37 $ par mois avant le stockage et le trafic, et [Coûts](#costs) donne le chiffre de chaque choix.

Ce que tout déploiement OpenTofu partage, c’est-à-dire le bloc et ses sections, les secrets, la console et les mises à niveau, se trouve dans
[Déployer avec OpenTofu](deploy-opentofu.md). Cette page ajoute ce dont Google Cloud a besoin.

<!-- source: what-it-creates 7b13c8bbc968 -->
## Ce qu’il crée

Un seul bloc de `mycroblog-gcp` crée ce qui suit. Le premier kind de chaque ligne est le recommandé et celui par défaut ;
[la référence des modules](../../deploy/opentofu/README.md#sections) liste chaque champ.

| Rôle | Section | Recommandé | Autres kinds |
| --- | --- | --- | --- |
| Serveur | `service`, `jobs` | Un service Cloud Run avec `role = all`, un job de migration, un job de console tant que vous en avez besoin, un compte de service et un accès public | |
| Trafic | `ingress` | `load-balancer` : un Application Load Balancer externe global avec un certificat géré par Google | `domain-mapping`, `cloudflare`, et un hôte de médias en cache sur Cloudflare à côté de n’importe lequel |
| Base de données | `database` | `external` : un secret pour l’URL d’un PostgreSQL que vous exécutez, comme Neon | `cloud-sql` : une instance, sa base de données, son utilisateur et son mot de passe |
| Jobs | `queue` | `cloud-tasks` : une file Cloud Tasks, le job Cloud Scheduler du tick et un compte de service qui appelle le serveur | `db` : les jobs dans la base de données |
| Médias | `media` | `gcs` : un bucket Cloud Storage privé | `r2`, `s3` |
| Caches et limites de débit | `kv` | `db` : dans la base de données, sans rien à créer | `memorystore`, `external` (Valkey) |
| Mises à jour en temps réel | `stream` | `pubsub` : un topic, et un rôle personnalisé avec lequel le serveur gère ses propres abonnements | `postgres`, `valkey` |
| E-mail | `mail` | Tout service SMTP, avec un secret pour son mot de passe | |
| KEK | `kek` | `secret-manager` : un secret généré, monté comme fichier | `kms` : une clé dans Cloud KMS |
| Réseau | `network` | `none` : rien à rattacher | `vpc`, `existing` |
| Métriques, traces, erreurs | `observability` | Cloud Monitoring, Cloud Trace et Error Reporting, qui lisent ce que Cloud Run leur envoie. Le module active leurs API | `settings`, comme Sentry |
| Alertes | `monitoring` | Un contrôle de disponibilité, des métriques basées sur les journaux et des politiques d’alerte : [Supervision et alertes](#monitoring-and-alerts) | `null`, aucune |

Le module génère aussi la clé VAPID, et crée vides les secrets que vous seul pouvez remplir.

`examples/gcp-cloud-run-neon` est une façon de commencer : le fournisseur et un bloc avec chaque kind recommandé, qui fonctionne avec
n’importe quelle base de données externe, Neon ou non. Ajoutez à son bloc les sections que vous changez.

<!-- source: before-you-start 9869afda9a07 -->
## Avant de commencer

- Ce dont [le premier déploiement](deploy-opentofu.md#first-deployment) a besoin, avec `gcloud` comme CLI de la plateforme, et
  `gcloud auth application-default login` pour le fournisseur.
- Un projet Google Cloud avec facturation, utilisé pour cette seule instance, et un compte dessus avec le rôle Propriétaire. Le rôle
  personnalisé de `stream = pubsub` va sur le projet ; il atteindrait donc les abonnements de tout le reste de ce projet.

<!-- source: first-deployment 894b6af2cee9 -->
## Premier déploiement

Faites ici les deux premières étapes, puis suivez [le premier déploiement](deploy-opentofu.md#first-deployment) avec les notes de la
troisième.

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. Activer les API

Chaque déploiement a besoin de celles-ci :

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

Ajoutez l’API de chaque choix qui en demande une :

| Choix | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` ou `existing` | `compute.googleapis.com`, `servicenetworking.googleapis.com`, `networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`, `serviceconsumermanagement.googleapis.com`, et celles du réseau |

`translation` active sa propre API, et `observability` aussi. [Importer les coûts d’utilisation du cloud](billing-import.md) liste ce dont
`billing` a besoin.

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. Préparer la base de données

- **`external`** : créez la base de données, et gardez son URL de connexion directe et, s’il y en a une, son URL poolée pour
  `secrets_to_fill` : [La base de données](#the-database) dit laquelle va où. Le module ne la crée jamais ; son mot de passe reste donc
  hors de l’état OpenTofu.
- **`cloud-sql`** : rien à faire. Le module crée l’instance et écrit son URL, avec un mot de passe généré, dans le secret.

<!-- source: 3-deploy 4470affd40e4 -->
### 3. Déployer

Suivez [le premier déploiement](deploy-opentofu.md#first-deployment). Sur Google Cloud :

- **Variables** : `mail_from`, `mail_smtp_host` et `mail_smtp_username` de l’exemple configurent l’[e-mail](email.md). Le port
  est 587, puisque Google Cloud bloque le port sortant 25.
- **Secrets** : la KEK est `my-kek` et la clé VAPID `my-vapid-key`, chacun commençant par `name`, et
  `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` les copie. La KEK parvient au serveur sous forme de
  fichier monté, nommé par `MY_KEK_FILE`, et les autres secrets sous forme de variables d’environnement.
- **Application** : le job de migration vient d’abord, avec ce dont il dépend, ce qui inclut l’ingress, puisque les paramètres du serveur
  le nomment. La sonde de démarrage du service attend `/health/ready`, qui répond `503` jusqu’à ce que le schéma existe.
- **DNS** : ajoutez les enregistrements de `tofu output dns_records`, comme [Comment le trafic atteint le service](#how-traffic-reaches-the-service)
  le dit pour chaque kind. Tant que le certificat n’est pas actif, `https://{host}` échoue avec une erreur TLS ; la fédération et les navigateurs
  ne peuvent donc pas encore atteindre le serveur. Rien d’autre ne l’attend : Cloud Tasks et Cloud Scheduler appellent l’URL par défaut du service,
  `https://<name>-<project number>.<region>.run.app`. **Ne désactivez pas l’URL par défaut**, sinon les jobs s’arrêtent. Cloud Run ne compte
  ces deux appelants comme internes que depuis le même projet et sur cette URL, ce qui permet à un répartiteur de charge de la fermer à
  internet.
- **Le premier administrateur** : le lien reste dans Cloud Logging, où toute personne qui peut lire les journaux du projet peut l’utiliser.

<!-- source: the-console-and-upgrades a40a1df56865 -->
## La console et les mises à niveau

[La console](deploy-opentofu.md#the-console) et les [mises à niveau](deploy-opentofu.md#upgrading) exécutent des exécutions des jobs Cloud Run
`<name>-console` et `<name>-migrate` depuis l’endroit où vous appliquez, par `gcloud`. Ils ont besoin d’identifiants autorisés à exécuter
les jobs et à lire leurs journaux, `roles/run.developer` et `roles/logging.viewer`, que Propriétaire inclut. Une application interrompue
laisse l’exécution en cours sur Cloud Run, et l’application suivante attend qu’elle se termine.

Pour [vérifier d’abord le SQL](deploy-opentofu.md#checking-the-sql-first) :

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## La base de données

Le module recommande `external`, qui n’a besoin d’aucun réseau et ne facture rien sur Google Cloud : une base de données qui s’arrête au repos,
comme Neon, garde un serveur calme peu coûteux. `cloud-sql` garde tout dans un seul projet, à coût fixe. Déplacer des données de l’une
à l’autre est [un changement de backend](deploy-opentofu.md#changing-a-backend).

<!-- source: an-external-postgresql d6604bfb9e2f -->
### Un PostgreSQL externe

`database = { kind = "external" }`, la valeur par défaut, accepte tout PostgreSQL que Cloud Run peut atteindre : Neon, un autre PostgreSQL
hébergé, ou un que vous exécutez, par internet ou sur une adresse privée via [un réseau](#networks). Le module crée le
secret `my-database-url`, pour l’URL **directe**, que le job de migration et la console utilisent toujours : `my schema apply` prend
un verrou de niveau session, qu’un pooler en mode transaction ne conserve pas.

`database = { kind = "external", pooled = true }` ajoute le secret `my-database-url-pooled`, pour une URL **poolée**, que le
service utilise alors. Un pooler convient au serveur, comme le dit [Par un pooler de connexions](database.md#through-a-connection-pooler).
Sans pooler, le service utilise aussi l’URL directe ; `service.max_instances` fois le pool de chaque instance doit donc rester
sous la limite de connexions de la base de données. Le pool est par défaut de 4 connexions par instance, pour les 80 requêtes qu’une instance
accepte à la fois : augmentez-le avec `pool_max_conns` dans l’URL que le service utilise, par exemple `...?sslmode=require&pool_max_conns=8`
([Attendre une connexion à la base de données](running.md#waiting-for-a-database-connection)).

**Sur Neon**, créez un projet dans une région proche de votre région Cloud Run. Sa chaîne de connexion directe a l’hôte simple, et sa
chaîne poolée l’hôte avec `-pooler`, qui est le pooler de Neon et convient au serveur : définissez `pooled = true`. Comptez sur l’offre Launch,
puisque le tick par défaut garde le calcul éveillé ; l’offre Free fonctionne avec [les compromis décrits sous Coûts](#where-care-reduces-it).

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` crée une instance PostgreSQL 18, `db-f1-micro` par défaut, avec des sauvegardes quotidiennes et une
récupération à un instant donné conservée 7 jours, sa base de données et son utilisateur, et un mot de passe généré qui n’atteint ni le plan ni
l’état. `tier`, `disk_size_gb`, `retained_backups` et `deletion_protection` la modifient. Elle n’a jamais été appliquée : considérez le
premier déploiement comme son test.

- **Sans réseau**, Cloud Run l’atteint par la socket du connecteur Cloud SQL, et l’adresse publique de l’instance n’admet
  rien d’autre.
- **Avec un réseau**, elle n’a qu’une adresse privée, et un `pg_dump` doit s’exécuter depuis l’intérieur du réseau.
- **Elle n’a pas de pooler** : `pooled` est refusé, et `service.max_instances` et `pool_max_conns` bornent les connexions. Gardez le
  pool petit sur `db-f1-micro`, qui a 0,6 Go de mémoire.
- **Elle ne descend jamais à zéro** ; elle est donc facturée quel que soit le trafic, et le SLA de Google ne couvre pas une offre à cœur partagé.

<!-- source: the-other-backends 91ef8e2dea11 -->
## Les autres backends

Chacun a sa recommandation pour valeur par défaut, et [un changement de `kind`](deploy-opentofu.md#changing-a-backend) passe à un autre.

- **Jobs** : `cloud-tasks` pousse chaque job vers le service, qui peut descendre à zéro. `queue.tick_schedule`, chaque minute par
  défaut, réveille le serveur et la base de données pour les jobs périodiques. `db` interroge la base de données depuis un processus qui reste actif ;
  il a donc besoin de `service.min_instances` valant 1 ou plus et de `service.cpu_always_allocated = true`.
- **Médias** : `gcs` crée un bucket privé, `media.bucket_name`, dans la région. `r2` crée un bucket R2 dans
  `cloudflare.account_id`, et `s3` utilise tout stockage compatible S3 à son `endpoint` ; tous deux lisent une clé d’accès dans les secrets
  que liste `secrets_to_fill`. Pour partager un bucket avec d’autres instances, `existing_bucket = true` utilise le vôtre et `prefix`
  (`tenant-a/`) commence chaque nom d’objet ; avec `gcs`, le compte de service n’atteint alors que les objets qui s’y trouvent, ce qui exige
  un accès uniforme au niveau du bucket.
- **Caches et limites de débit** : `db` les garde dans la base de données. `memorystore` crée Memorystore pour Valkey, qui a besoin d’un
  réseau et n’a pas de mot de passe : tout ce qui est sur le réseau peut l’atteindre. `external` lit l’URL d’un Valkey que vous exécutez dans un secret.
- **Mises à jour en temps réel** : `pubsub` atteint chaque instance. `postgres` utilise `LISTEN` et `NOTIFY` sur la connexion directe de la
  base de données, et `valkey` le Valkey de `kv`.
- **La KEK** : `secret-manager` la génère une fois, pour que vous la copiiez. `kms` la garde dans Cloud KMS, qu’elle ne quitte jamais ; une
  sauvegarde de base de données divulguée et une configuration divulguée ensemble n’exposent donc encore aucun secret. La changer sur une instance qui contient des données est
  [un déménagement](deploy-opentofu.md#the-kek).

<!-- source: networks 9d28311e11a6 -->
## Réseaux

La configuration recommandée n’en a besoin d’aucun : une base de données externe, Cloud Tasks, Cloud Storage et Pub/Sub sont tous atteints par
internet avec l’identité propre du service ; Cloud Run ne se rattache donc à rien.

Il en faut un pour Memorystore, qui n’a que des adresses privées, pour donner une adresse privée à Cloud SQL, ou pour atteindre une base de données à
vous sur un tel réseau. `network = { kind = "vpc" }` crée un VPC, ses sous-réseaux et l’accès aux services privés, et
`network = { kind = "existing", ... }` se rattache à un VPC que vous avez, que vous décrivez par `network`, `subnetwork`,
`private_services_range` et `memorystore_connection_policy` :

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Le service et les jobs se rattachent par Direct VPC egress, et seules les plages privées passent par le réseau ; internet
garde donc son propre chemin, sans Cloud NAT. Avec `vpc`, `nat = true` envoie tout par Cloud NAT à la place, pour une adresse source
statique. Un réseau peut retarder d’une minute ou plus le démarrage d’une instance, ce qu’un serveur qui descend à zéro rencontre
souvent. Le VPC que crée le module n’a jamais été appliqué.

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## Comment le trafic atteint le service

`ingress.kind` en décide. Le module recommande le répartiteur de charge, pour la production. Le mappage de domaine convient pour essayer un
déploiement à peu de frais.

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| Frais mensuels | Environ 18 $ pour les règles de transfert, plus des frais par Go. Vérifiez la [page des tarifs](https://cloud.google.com/vpc/network-pricing) : ce chiffre n’a pas pu y être confirmé | Aucun |
| Statut | La méthode recommandée par Google Cloud | Une fonctionnalité en préversion que Google Cloud ne recommande pas pour la production, à cause de la latence |
| Régions | Toutes | `asia-northeast1`, `us-central1` et quelques autres |
| DNS | Un enregistrement `A` de `host` vers son adresse. Laissez désactivé tout proxy de votre fournisseur DNS pour lui | Les enregistrements que liste `dns_records` |
| Certificat | Géré par Google, actif une fois que le DNS pointe vers l’adresse, ce qui peut prendre un moment | Géré par Google, jusqu’à 24 heures |
| Propriété du domaine | Rien au-delà du DNS | La personne qui applique doit d’abord être un [propriétaire vérifié](https://cloud.google.com/run/docs/mapping-custom-domains) du domaine |
| URL `run.app` | Refusée depuis internet, de sorte que personne ne contourne le répartiteur de charge | Accessible à tous : un mappage de domaine ne peut pas ajouter d’en-tête, l’origine reste donc ouverte |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` et l’adresse du répartiteur de charge | `169.254.0.0/16` |

Cloud Tasks et Cloud Scheduler n’arrivent par aucun des deux : ils appellent directement l’URL `run.app` par défaut.

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` place Cloudflare devant `host`, avec le cache contourné pour tout, et
`ingress.media_host` ajoute un hôte de médias en cache à côté de n’importe quel kind. Les deux ont besoin de `cloudflare = { zone_id = "..." }`, le fournisseur
lit son jeton dans `CLOUDFLARE_API_TOKEN`, et le module crée les enregistrements ; `dns_records` est donc vide. Ils n’ont été exécutés que
sur des fournisseurs simulés.

- **Votre offre décide de l’origine.** Cloud Run route selon `Host`, et Cloudflare ne remplace l’en-tête Host que sur l’offre
  Enterprise. Sur toute autre offre, le module crée un mappage de domaine pour chaque hôte, avec les compromis du tableau ci-dessus, et
  Cloudflare s’y connecte. Que son certificat soit émis et renouvelé derrière le proxy n’est pas vérifié ; commencez donc avec
  `cloudflare.proxied = false`. Sur l’offre Enterprise, `cloudflare.host_header_override = true` se connecte plutôt à l’URL
  `run.app`, sans mappage de domaine.
- **Un en-tête ferme l’origine.** Une règle de transformation d’en-tête de requête ajoute une valeur que le module génère, et le serveur
  refuse avec un `403` une requête qui ne l’a pas, hors vérifications de santé et `POST` des jobs eux-mêmes, qui vérifient leurs jetons.
  Ainsi, personne ne contourne Cloudflare par l’URL `run.app`, et personne ne peut choisir l’adresse de client que voient les limites de débit.
- **Rien n’ajoute l’en-tête tant qu’un enregistrement n’est pas proxifié**, de sorte que le serveur refuse alors tout sauf ses vérifications de santé :
  avec `proxied = false` pour le certificat d’un mappage de domaine, activez le proxy une fois le certificat actif.
- **La valeur se retrouve dans l’état**, comme le dit [Secrets](deploy-opentofu.md#secrets), et dans le secret `my-origin-secret`.
  [Le guide du CDN](cdn.md#rotate-the-value) explique comment la renouveler.
- **Désactivez Bot Fight Mode** dans le tableau de bord, et n’activez pas « I'm Under Attack » : un défi sur `/ap/`, `/.well-known/`
  ou `/api/` casse la fédération, puisque les autres serveurs ne sont pas des navigateurs. Les deux couvrent toute la zone, qu’aucune règle ne peut exempter.
  Le module n’ajoute aucun défi et désactive Browser Integrity Check.
- **Email Address Obfuscation peut rester activé.** Les pages du serveur marquent elles-mêmes le contenu dont il doit laisser les adresses intactes.
- **Activez Always Use HTTPS** dans le tableau de bord si vous voulez que `http://` soit redirigé. Il couvre toute la zone ; le module
  vous le laisse donc.
- **Délais** : Cloudflare répond `524` lorsque l’origine n’envoie rien pendant 125 secondes. Les jobs ne sont pas concernés, puisqu’ils ne
  passent pas par Cloudflare. Les flux envoient un battement de cœur toutes les 30 secondes, sous la limite d’inactivité de 900 secondes. Que Cloudflare
  mette ou non les flux en tampon sur un hôte proxifié n’est pas vérifié.
- **Vidéo** : les conditions de Cloudflare restreignent la diffusion de vidéos et d’une quantité disproportionnée de gros fichiers sur les offres Free, Pro et
  Business, et il peut agir sur le contenu. Décidez-le avant de mettre de la vidéo en cache sur l’hôte de médias.

**Un fichier supprimé reste en cache.** L’hôte de médias suit le `Cache-Control` du serveur ; un fichier local supprimé peut donc rester visible
jusqu’à un an, et un fichier distant pendant `remote_media_cache_ttl_seconds`. Le serveur ne le purge pas. Lorsque quelque chose doit
disparaître, purgez-le à la main, sous **Caching > Configuration > Custom Purge** dans le tableau de bord, ou :

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

L’URL d’un proxy contient son `sig` dans la chaîne de requête, qui fait partie de ce qui est mis en cache ; donnez donc l’URL entière.

<!-- source: monitoring-and-alerts eb28e653de36 -->
## Supervision et alertes

`monitoring` ajoute un contrôle de disponibilité de `/health/live`, des métriques basées sur les journaux et des politiques d’alerte portant les noms de
[la procédure d’exploitation](alerts.md), qui notifient les adresses de `notification_emails` (`alert_emails` dans l’exemple) ou les canaux
de `notification_channels`. La section n’a pas de valeur par défaut : nommez qui est notifié, ou définissez `monitoring = null` pour tout
omettre. Elle n’a jamais été appliquée ; essayez-la donc une fois en faisant échouer un contrôle. Chaque exécution du contrôle réveille une instance :
`uptime_period`, 15 minutes par défaut, est la fréquence.

<!-- source: traces b1f84a2f93b9 -->
### Traces

Cloud Trace montre la trace que Cloud Run enregistre pour chaque requête, et les lignes de journal du serveur portent son `trace_id`, puisque le
module fait confiance à l’ingress (`MY_TRUSTED_PROXIES`). Le module définit `logs_exporter = gcp`, donc chaque ligne remplit aussi les
champs de trace que Cloud Logging relie à Cloud Trace (`logging.googleapis.com/trace`), et chaque ligne d’accès s’affiche comme une requête.
`observability.logs = "settings"` laisse `logs_exporter` à `settings`. Non vérifié : que Cloud Logging relie une ligne par
l’ID de trace seul qu’elle écrit, et que Cloud Run remplace toujours le `traceparent` d’une requête publique.
Les spans propres du serveur demanderaient un collector OpenTelemetry à ses côtés, que le module n’exécute pas.

<!-- source: limits 3d4721415f18 -->
## Limites

- **Téléversements** : Cloud Run refuse une requête HTTP/1 de plus de 32 Mio, et le serveur parle HTTP/1.1 ; le module définit donc
  `MY_MEDIA_MAX_AV_BYTES` à 30 Mio, au lieu des 100 Mio par défaut, afin qu’un téléversement plus gros reçoive la réponse du serveur lui-même.
  La limite propre de Cloudflare, 100 Mo sur Free et Pro, est au-dessus.
- **Flux** : `service.request_timeout_seconds`, 15 minutes par défaut, met fin à un flux ouvert, et le client se reconnecte.
- **Connexions à la base de données** : Google limite une instance Cloud Run à 100 connexions vers Cloud SQL.
- **Adresses des clients** : les limites de débit reposent sur `MY_TRUSTED_PROXIES`, que le module définit à partir de l’ingress. Aucun document que nous
  ayons lu ne dit ce que Cloud Run ajoute à `X-Forwarded-For` pour une requête directe ni quelle adresse de pair un conteneur voit ;
  `169.254.0.0/16` est donc une hypothèse. La forme documentée du répartiteur de charge est `[<supplied>,]<client-ip>,<lb-ip>`. Vérifiez les
  adresses dans les premiers journaux de requêtes : une adresse de client dans `169.254` ou de Google signifie que le paramètre est faux pour votre projet.
- **Accès public** : le service accorde `roles/run.invoker` à `allUsers`, ce qu’une politique d’organisation de partage restreint au domaine
  refuse. Un tel projet a besoin d’une exception.
- **Noms qui restent pris** : Cloud SQL et Cloud Tasks refusent le nom d’une instance ou d’une file supprimée pendant environ une semaine, et un
  rôle personnalisé supprimé garde son ID un certain temps ; un déploiement refait après un `destroy` a donc besoin d’un autre `name`.

<!-- source: costs ea658d24d79d -->
## Coûts

Ces chiffres sont un guide de ce qu’il faut attendre et de l’endroit où le soin réduit la facture, pas des limites auxquelles le déploiement est tenu. Ce
sont des prix catalogue approximatifs pour une région, d’octobre et novembre 2026 : vérifiez-les sur les pages de tarifs de
[Cloud Run](https://cloud.google.com/run/pricing), [Cloud SQL](https://cloud.google.com/sql/pricing) et
[Neon](https://neon.com/pricing). Le calcul de Cloud Run est gratuit par compte de facturation jusqu’à 180 000 vCPU-secondes et 360 000
GiB-secondes par mois.

Ce que paie tout déploiement, environ 0 $ au repos :

| Élément | Au repos | Ce qui s’y ajoute |
| --- | --- | --- |
| Cloud Run | 0 $ : aucune instance ne tourne, et le CPU n’est facturé que pendant les requêtes | Chaque flux ouvert garde une instance facturée ; voir ci-dessous |
| Cloud Tasks | 0 $ | 1 million d’opérations gratuites, puis 0,40 $ par million. Un job prend environ 2 opérations (le mettre en file et le livrer) |
| Cloud Scheduler | 0 $ : le tick est un job, et 3 sont gratuits par compte de facturation | 0,10 $ par job au-delà |
| Pub/Sub | 0 $ : 10 GiB gratuits | Seulement pendant que des flux sont ouverts |
| Secret Manager | Quelques centimes | Quelques versions gratuites, puis quelques centimes par version |
| Cloud Storage | Environ 0,02 $ par GiB et par mois | 5 GiB ne sont gratuits que dans `us-east1`, `us-west1` et `us-central1`. Les médias sortent par Cloud Run |
| Monitoring | 0 $ dans les limites gratuites | |

Ce que chaque choix ajoute :

| Section | Choix | Par mois |
| --- | --- | --- |
| `ingress` | `load-balancer` | Environ 18 $, non confirmé sur la page des tarifs, plus des frais par Go |
| | `domain-mapping` | 0 $ |
| | `cloudflare` | Votre offre Cloudflare, et l’origine : 0 $ avec un mappage de domaine |
| `database` | `external` sur l’offre Launch de Neon | Environ 19 $ avec le tick par défaut, plus 0,35 $ par Go-mois |
| | `external` sur l’offre Free de Neon | 0 $, dans ses limites : [ci-dessous](#where-care-reduces-it) |
| | `external` ailleurs | Ce que coûte cette base de données |
| | `cloud-sql` | Environ 9,40 $ pour `db-f1-micro` et 10 GiB dans `us-central1`, plus les sauvegardes, quel que soit le trafic |
| `queue` | `db` | Une instance facturée en continu, avec son CPU toujours alloué |
| `kv` | `memorystore` | Environ 23 $ pour le plus petit nœud, quel que soit le trafic |
| `kek` | `kms` | Quelques centimes, pour la clé et ses opérations |
| `network` | `vpc` avec `nat = true` | Cloud NAT, à l’heure et au Go |

La configuration recommandée sur l’offre Launch de Neon, le répartiteur de charge et Neon, revient à environ 37 $ par mois ; avec Cloud SQL, à
environ 27 $.

**Le tick garde Neon éveillé.** Chaque tick lit et écrit dans la base de données, et Neon n’arrête un calcul qu’après 5 minutes sans
activité ; avec le `* * * * *` par défaut, le calcul ne s’arrête donc jamais. Cela fait 730 heures par mois, soit 182,5 CU-heures à 0,25 CU,
ce qui représente les 19 $ ci-dessus. Le tick reste à chaque minute pour que les jobs périodiques s’exécutent à l’heure et qu’un signal de réveil perdu soit
récupéré rapidement. Cloud SQL facture la même chose quel que soit le tick.

**Un flux ouvert garde une instance facturée.** Avec la facturation à la requête, une instance est facturée tant qu’elle traite une requête,
et les mises à jour en temps réel sont une seule longue requête. Un client qui garde son flux ouvert tout le mois coûte une instance en
continu : environ 60 $ par mois à 1 vCPU et 512 Mio, après l’allocation gratuite. Davantage de flux partagent l’instance, jusqu’à
`service.concurrency`. Si les flux restent ouverts la plupart du temps, comparez avec la facturation à l’instance
(`service.cpu_always_allocated`), ou avec une petite VM et [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 427be9d46d32 -->
### Là où le soin réduit la facture

- **Le mappage de domaine** à la place du répartiteur de charge supprime les 18 $, avec
  [ses compromis](#how-traffic-reaches-the-service). Il convient pour essayer un déploiement.
- **L’offre Free de Neon** supprime les 19 $, et une petite instance peut tourner dessus, avec ces compromis :
  - Elle autorise 100 CU-heures par mois, que le tick par défaut épuise après environ 17 jours. Restez dans cette limite avec un
    calendrier plus long, tel que `queue = { tick_schedule = "*/10 * * * *" }` : éveillé au plus la moitié du temps, environ 91 CU-heures, avant que
    quoi que ce soit d’autre ne le réveille. Les jobs périodiques s’exécutent alors avec jusqu’à 10 minutes de retard, et un signal de réveil perdu est récupéré plus tard.
  - Un serveur fédéré reçoit des livraisons en continu, ce qui réveille aussi la base de données ; une instance occupée la dépasse donc
    quel que soit le calendrier.
  - Elle contient 1 Go par projet et un historique de 6 heures à partir duquel restaurer, et son calcul s’arrête toujours après 5 minutes d’inactivité ;
    la première requête après cela l’attend donc.
