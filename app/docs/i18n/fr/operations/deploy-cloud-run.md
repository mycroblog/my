<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# Déployer sur Cloud Run

Comment exécuter `my` sur Google Cloud avec les modules OpenTofu de [`deploy/opentofu`](../../deploy/opentofu/README.md) : Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage et Pub/Sub, avec une base de données PostgreSQL sur Neon, derrière un répartiteur de charge d’application externe global. Le service n’exécute aucune instance tant que personne ne l’utilise, à l’exception d’un tick qui réveille le serveur chaque minute. Un serveur coûte de l’argent : comptez environ 37 $ par mois avant le stockage, le trafic et les flux ouverts, et lisez [Coûts](#costs) pour savoir ce que l’attention peut réduire.

Les modules nécessitent OpenTofu 1.11 ou une version ultérieure. Terraform n’est pas pris en charge.

<!-- source: what-it-creates c95687df197f -->
## Ce que cela crée

| Élément | Module | Ressources |
| --- | --- | --- |
| Serveur | `app-gcp-cloud-run` | Un service Cloud Run avec `role = all`, un job de migration, un job de console tant que vous en avez besoin, un compte de service et un accès public |
| Trafic | `ingress-gcp-load-balancer`, ou `ingress-gcp-domain-mapping` ou `ingress-cloudflare-app`, et éventuellement `ingress-cloudflare-media` | Un répartiteur de charge d’application externe global avec un certificat, ou un mappage de domaine, ou Cloudflare, et un hôte de médias |
| Tâches | `queue-gcp-cloud-tasks` | Une file Cloud Tasks, un job Cloud Scheduler et un compte de service qui appelle le serveur |
| Médias | `blob-gcp-gcs` | Un bucket privé |
| Mises à jour en temps réel | `stream-gcp-pubsub` | Un sujet et un rôle personnalisé. Le serveur crée et supprime ses propres abonnements |
| Base de données | `db-external` | Rien : vous créez la base de données à la main |
| E-mail | `mail-smtp` | Rien : n’importe quel service SMTP |
| Secrets | `secrets-gcp-secret-manager` | Les secrets de l’URL de la base de données et du mot de passe SMTP, que vous remplissez, et une clé VAPID générée |
| KEK | `kek-gcp-secret-manager` ou `kek-gcp-kms` | Un secret généré monté comme fichier, ou une clé Cloud KMS |

Les valeurs des secrets n’atteignent jamais le plan ni l’état d’OpenTofu : vous ajoutez vous-même celles qui viennent d’ailleurs, et le déploiement génère la KEK et la clé VAPID au moyen d’attributs en écriture seule.

<!-- source: before-you-start 4c624a6b7316 -->
## Avant de commencer

- `tofu` 1.11 ou une version ultérieure, `gcloud` et `jq`.
- Un projet Google Cloud avec facturation, utilisé uniquement pour cette instance, et un compte y disposant du rôle Owner.
- Un domaine pour [`host`](../reference/configuration.md#host). Il ne peut plus changer ensuite : lisez [Paramètres que l’on ne peut plus changer ensuite](production-checklist.md#settings-you-cannot-change-later).
- Une version de `my` parmi les [versions publiées](running.md#get-the-image). Il n’existe pas de balise `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Premier déploiement

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. Activer les API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` sert au répartiteur de charge.

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. Créer la base de données sur Neon

Prévoyez l’offre Launch : avec le tick par défaut, le calcul ne s’arrête jamais. L’offre Free fonctionne aussi, avec les compromis décrits sous [Coûts](#costs).

Il y a deux façons de faire, et l’exemple utilise la première :

- **À la main, avec `db-external`.** Dans la console Neon, créez un projet dans une région proche de votre région Cloud Run, et copiez ses chaînes de connexion. Le mot de passe reste hors de l’état d’OpenTofu.
- **Avec [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**, qui crée le projet et écrit les URL dans les secrets à votre place. **Le mot de passe de la base de données se retrouve alors dans l’état**, et le fournisseur n’est plus maintenu : son README explique ce que cela signifie, et un commentaire sur le bloc `db` dans `main.tf` montre le remplacement.

Un projet a deux chaînes de connexion, et le déploiement utilise chacune là où elle convient :

- La chaîne **directe** (l’hôte sans `-pooler`) est obligatoire. Le job de migration et la console l’utilisent toujours : `my schema apply` tient un verrou au niveau de la session, qu’un pooler en mode transaction ne conserve pas.
- La chaîne **poolée** (l’hôte avec `-pooler`) est facultative, pour le service. Le pooler de Neon gère ce dont le serveur a besoin : voir [Via un pooler de connexions](database.md#through-a-connection-pooler). Donnez-la à `pooled_url_secret_id` du module `db`, dans un secret distinct, ajouté à `referenced` du module `secrets`.

Sans chaîne poolée, le service utilise aussi la chaîne directe, à la limite de connexions de la base de données, que `max_instances` du module app borne. Pour relever le pool de sa valeur par défaut de 4 par instance, pour les 80 requêtes qu’une instance Cloud Run prend en même temps, ajoutez `pool_max_conns` à la chaîne que le service utilise, par exemple `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Voir [Attente d’une connexion à la base de données](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
### 3. Copier l’exemple et définir les variables

Partez d’une copie dans un répertoire vide, avec le fichier de verrouillage des fournisseurs, depuis la version que vous déployez :

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Réglez `ref` sur cette version.** Chaque bloc de module de `main.tf` nomme une version avec `?ref=`, ce que la copie d’une version stable a déjà correctement. Pour une préversion, réglez-le vous-même : `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`. Les modules n’existent qu’à partir de la première version qui les contient, donc une balise plus ancienne n’a pas d’exemple à copier. Gardez la même version dans `ref` et dans `image`, et conservez ce répertoire : il contient votre état et vos paramètres.

Remplissez `terraform.tfvars`. `mail_from`, `mail_smtp_host`, `mail_smtp_port` (587 par défaut ; Google Cloud bloque le port sortant 25) et `mail_smtp_username` configurent l’[e-mail](email.md). `env` et `secret_env` acceptent tout autre paramètre de la [référence de configuration](../reference/configuration.md), par valeur et par ID de secret. Un paramètre nommé à deux endroits fait échouer le plan.

Pour n’envoyer aucun e-mail, supprimez le module `mail` et son secret de `main.tf`, et `mail` reste à `none`. Lisez d’abord [ce que cela implique pour les inscriptions](production-checklist.md#email).

Décidez dès maintenant de [`account_domain`](../reference/configuration.md#account_domain), sous la forme `MY_ACCOUNT_DOMAIN` dans `env`, si les identifiants utilisent un autre domaine.

**L’état** reste par défaut dans ce répertoire. Il ne contient aucune valeur de secret, mais c’est la seule trace de ce qui a été créé : conservez-le dans un bucket avec un bloc `backend "gcs"` si plusieurs personnes exécutent ceci.

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. Créer les secrets et ajouter leurs valeurs

Cela crée les secrets, et génère la KEK et la clé VAPID :

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` récupère aussi `upgrade.sh` et `console.sh` à cette version, que les étapes suivantes utilisent :

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**Copiez la KEK maintenant**, et conservez la copie à l’écart de la base de données, par exemple dans un gestionnaire de mots de passe. Sans elle, une [sauvegarde](backup.md) ne peut pas être restaurée, et les clés des comptes sont perdues. Faites de même avec la clé VAPID, dont la perte met fin à tous les abonnements Web Push :

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Ajoutez ensuite les valeurs qui viennent d’ailleurs. Une instance dont un secret ne peut pas être lu ne démarre pas, ajoutez-les donc avant l’application :

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

La KEK parvient au serveur sous la forme d’un fichier monté, nommé par `MY_KEK_FILE`. Les autres sont des variables d’environnement résolues au démarrage d’une instance, donc une nouvelle version prend effet à la prochaine instance : déployez une nouvelle révision pour en être sûr.

**OpenTofu génère la KEK une seule fois et jamais plus.** Le secret et sa version ont `prevent_destroy`, et le numéro de version qui écrirait une nouvelle valeur est fixé dans le module : ne le changez pas, car une nouvelle KEK aléatoire rend illisible tout ce que l’ancienne a chiffré. Pour effectuer une rotation, suivez [Effectuer la rotation de la KEK](key-management.md#rotate-the-kek), en ajoutant la nouvelle valeur avec `gcloud secrets versions add`. OpenTofu ne suit pas ce que vous ajoutez là.

Pour conserver la KEK dans Cloud KMS dès le départ, remplacez le bloc `kek` dans `main.tf` comme son commentaire l’indique, et activez `cloudkms.googleapis.com`. Pour apporter votre propre KEK ou clé VAPID, définissez `create = false` sur le module `kek`, ou retirez la clé de `generate_p256_keys`, avant cette étape. Un secret créé ailleurs n’a besoin que de son ID : passez-le à `secret_env` du module app, ou au module qui l’utilise.

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. Créer ce dont la migration a besoin

La sonde de démarrage du service attend `/readyz`, qui répond `503` tant que le schéma n’existe pas, donc le schéma doit venir en premier. Créez le job de migration et ce dont il dépend, ce qui comprend l’adresse du répartiteur de charge, puisque les paramètres du serveur la nomment :

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. Appliquer le schéma

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

Il exécute `my config check`, `my db upgrade -check`, `my schema apply` et `my db upgrade`, chacun comme une exécution du job, et s’arrête au premier qui échoue. La première commande qui utilise la base de données y enregistre `host` et `account_domain`.

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. Créer le reste

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. Faire pointer le domaine vers le répartiteur de charge

`tofu output dns_records` liste un enregistrement `A`, de `host` vers l’adresse du répartiteur de charge. Ajoutez-le chez votre fournisseur DNS. Si ce fournisseur dispose de son propre proxy, laissez-le désactivé pour cet enregistrement.

Le certificat est géré par Google et ne devient actif qu’une fois que le DNS pointe vers l’adresse, ce qui peut prendre un certain temps. D’ici là, `https://{host}` échoue avec une erreur TLS, donc la fédération et les navigateurs ne peuvent pas encore joindre le serveur. Rien d’autre ne l’attend : Cloud Tasks et Cloud Scheduler appellent l’URL `run.app` par défaut du service, `https://<service>-<project number>.<region>.run.app`. **Ne désactivez pas l’URL par défaut**, sinon les tâches s’arrêtent. Cette URL refuse les requêtes venant d’Internet, et Cloud Run ne considère ces deux appels comme internes que depuis le même projet et sur cette URL, pas sur un domaine personnalisé.

Voir [Comment le trafic atteint le service](#how-traffic-reaches-the-service) pour ce que vous pouvez choisir d’autre.

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. Créer le premier administrateur

Les jobs Cloud Run n’ont ni terminal ni entrée standard, donc `my admin create` affiche un lien qui définit le mot de passe au lieu d’en lire un. Activez la [console](#the-console), et exécutez :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Ouvrez le lien tout de suite et définissez le mot de passe. Il fonctionne une fois, pendant une heure. **Il reste dans Cloud Logging jusque-là**, où toute personne qui peut lire les journaux du projet peut l’utiliser. S’il expire, exécutez `my admin account password-reset-link @alice` via la console. Activez ensuite l’authentification à deux facteurs.

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. Parcourir la liste de contrôle

Parcourez la [liste de contrôle de production](production-checklist.md), en commençant par l’[e-mail](production-checklist.md#email) : SPF, DKIM et DMARC sur le domaine de `mail_from`.

<!-- source: the-console 5aebf704c6d7 -->
## La console

La console exécute une commande `my` avec l’image, les paramètres et les secrets du service. Elle n’existe que tant que `console_enabled` est activé, désactivez-la donc lorsque vous avez terminé :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` attend la fin de la commande, affiche sa sortie et son code de sortie, et se termine avec ce code. Il nécessite `roles/logging.viewer`, car la sortie d’un job va dans Cloud Logging. Il utilise l’image que le service exécute, quoi que dise `image`.

La commande n’a ni entrée standard ni terminal, donc une commande qui y lit un mot de passe ou un fichier ne peut pas s’exécuter ainsi. Ce qu’elle affiche reste dans les journaux, n’exécutez donc pas une commande dont vous n’écririez pas la sortie à cet endroit.

<!-- source: upgrading 1a1e204c5173 -->
## Mise à niveau

Lisez d’abord [Mise à niveau](database.md#upgrading) et les notes de version, et avancez d’une version mineure à la fois.

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

Le script fait ceci, et s’arrête à la première étape qui échoue :

1. Il bascule le service en [mode maintenance](maintenance.md) avec `--maintenance`.
2. Il donne la nouvelle image au job de migration.
3. Il exécute `my config check`, `my db upgrade -check`, `my schema apply` et `my db upgrade`, chacun attendant le précédent.
4. Il déploie la nouvelle révision, sans le mode maintenance. Elle ne reçoit du trafic que lorsque sa sonde de démarrage réussit sur `/readyz`.

Sans `--maintenance`, l’ancienne révision continue de servir pendant les étapes 2 et 3, ce qui [n’est pas garanti de fonctionner](database.md#the-server-during-an-upgrade). Ajoutez `--allow-destructive` lorsque les notes de version l’exigent. Épinglez une empreinte avec `--image ghcr.io/mycroblog/my:0.2.0@sha256:...`, ce que recommande la [liste de contrôle de production](production-checklist.md#operations).

Définissez ensuite `image` dans `terraform.tfvars` à la même valeur, et `ref` dans `main.tf` à la même version, puis exécutez `tofu init` et `tofu plan`. Lisez ce plan avant de l’appliquer : les modules d’une nouvelle version peuvent modifier des ressources. `tofu apply` ne change jamais l’image d’un service ou d’un job existant, donc un `image` oublié ne fait aucun mal, mais cette valeur est celle dont partirait un nouveau job ou un service recréé.

**Lorsqu’une étape échoue**, le service reste sur sa révision, en mode maintenance si vous l’avez demandé. Corrigez la cause et exécutez à nouveau le script : les étapes peuvent être répétées sans danger. Pour quitter le mode maintenance sans mise à niveau, lorsque `my schema apply` n’a pas été exécuté :

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

Pour revenir en arrière après que le schéma a changé, restaurez la sauvegarde : voir [Retour en arrière](database.md#rolling-back).

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## Surveillance et alertes

Le module [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) ajoute un contrôle de disponibilité de `/readyz`, des politiques d’alerte portant les noms de [la procédure d’exploitation](alerts.md), et des notifications par e-mail. Il n’a jamais été appliqué, essayez-le donc une fois en faisant échouer un contrôle. Activez d’abord `monitoring.googleapis.com`, et lisez dans son README ce que coûte le contrôle : chaque exécution réveille une instance, et, via `/readyz`, la base de données, ce qui s’ajoute au [compromis du tick](#costs).

<!-- source: changing-a-backend ef4660f60191 -->
## Changer de backend

Remplacez le bloc du module dans `main.tf`. Le module app prend ce que renvoie le bloc, donc rien d’autre ne change. Changer la base de données implique aussi de déplacer ses données, puisqu’une nouvelle base est vide : [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) détaille les étapes. Par exemple, une base de données à vous :

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

Pour un stockage hors de Google Cloud, remplacez le bloc `blob` par [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md), pour tout service compatible S3, ou [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md).

<!-- source: when-you-need-a-network fb392704a1e7 -->
## Quand vous avez besoin d’un réseau

Ce guide n’en nécessite aucun : Neon, Cloud Tasks, Cloud Storage et Pub/Sub sont tous joints via Internet avec l’identité propre du service, donc Cloud Run ne se rattache à rien et il n’y a rien à payer ni à surveiller.

Vous en avez besoin pour mettre Memorystore dans le déploiement, car il n’a que des adresses privées, ou pour donner à Cloud SQL une adresse privée à la place de son adresse publique. Ajoutez [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md), ou décrivez un VPC que vous avez comme un objet de [la même forme](../../deploy/opentofu/README.md#the-network-on-google-cloud), et passez le même à l’application et à chaque module qui le prend :

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

Donnez aussi `network = module.network` à l’application, et ajoutez `module.kv` à ses `backends`. Seules les plages privées passent par le réseau, donc le service atteint Internet comme avant, sans Cloud NAT. Un backend sur des adresses privées avec une application sur un autre réseau, ou sur aucun, fait échouer le plan. Activez d’abord les API Service Networking et Network Connectivity, ainsi que l’API Compute Engine. Un réseau s’ajoute au démarrage à froid d’une instance : lisez [ce qu’ajoute le réseau privé](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds).

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## Répartir les tâches entre des workers

Lorsqu’un type de tâche retarde les autres, donnez-lui son propre service. Décidez de ce qu’il faut séparer d’après les métriques, comme l’explique [Répartir les workers par type de tâche](running.md#splitting-workers-by-job-type). Une seule valeur va à la file et à l’application, et chaque file, URL, route et paramètre découle de ses clés, vous n’en écrivez donc aucun :

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

Définissez `workers = local.workers` sur le bloc `queue` et sur le bloc `app`. Le plan refuse une séparation des types qui doivent rester ensemble, ainsi qu’une file et une application auxquelles on a donné des valeurs différentes. Il nécessite aussi un broker qui atteint chaque processus, ce qu’est `stream-gcp-pubsub`. La même valeur va à `monitoring-gcp`, si vous l’utilisez.

Les deux dispositions courantes de cette page ressemblent à ceci. Pour éviter que les livraisons ne retardent le reste, utilisez la valeur ci-dessus. Pour garder rapides les e-mails et les notifications :

```hcl
locals {
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
}
```

Omettez les types des paramètres que vous n’avez pas définis, comme le dit cette page : un type que le serveur n’a pas enregistré empêche le démarrage, et le plan ne peut pas voir lesquels c’est. Avec `workers`, le service par défaut n’aide pas pour ces types lorsqu’il est libre, puisque leurs tâches vont dans leur propre file.

Chaque champ, et ce qu’un worker reprend de l’application, se trouve dans [le contrat](../../deploy/opentofu/README.md#workers). Le service d’un worker ne reçoit aucun trafic de l’extérieur. `upgrade.sh` déplace chacun après la migration et avant le service par défaut, et sans rien pour le lui dire : seule une étape qui échoue l’arrête. Chaque worker peut apporter ce que coûte le service, et les connexions de ses instances comptent dans la limite de la base de données.

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## Comment le trafic atteint le service

Le bloc `ingress` de `main.tf` en décide. L’exemple utilise le répartiteur de charge, destiné à la production. Le mappage de domaine figure en commentaire à côté, pour essayer un déploiement à peu de frais : pour le substituer, modifiez ce bloc et `service_ingress` du bloc `app`, et le plan signale si les deux divergent.

| | `ingress-gcp-load-balancer` (l’exemple) | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| Frais mensuels | Environ 18 $ pour les règles de transfert, plus un tarif par Go. Vérifiez la [page de tarifs](https://cloud.google.com/vpc/network-pricing) : ce chiffre n’a pas pu y être confirmé | Aucun |
| Statut | La méthode recommandée par Google Cloud | Une fonctionnalité en préversion que Google Cloud ne recommande pas pour la production, à cause de la latence |
| Régions | Toutes | `asia-northeast1`, `us-central1` et quelques autres |
| DNS | Un enregistrement `A`, depuis `tofu output dns_records` | Les enregistrements que la même sortie liste |
| Certificat | Géré par Google, actif une fois que le DNS pointe vers l’adresse | Géré par Google, jusqu’à 24 heures |
| Propriété du domaine | Rien au-delà du DNS | Vous, et tout autre compte concerné, devez être [propriétaire vérifié](https://cloud.google.com/run/docs/mapping-custom-domains) avant d’appliquer |
| URL `run.app` | Refusée depuis Internet, donc personne ne contourne le répartiteur de charge | Accessible à tous : un mappage de domaine ne peut pas ajouter d’en-tête, donc l’origine reste ouverte |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` et l’adresse du répartiteur de charge | `169.254.0.0/16` |
| `service_ingress` du bloc `app` | `internal-and-load-balancer`. Un désaccord avec le bloc `ingress` fait échouer le plan | `all` |

Cloud Tasks et Cloud Scheduler n’arrivent ni par l’un ni par l’autre : ils appellent directement l’URL `run.app` par défaut.

Pour utiliser l’URL propre du service ou autre chose devant, comme un CDN, supprimez le bloc `ingress` et sa ligne dans `backends`, et gardez `service_ingress` à ce dont vous avez besoin : c’est toujours votre choix explicite, et `all` laisse l’URL `run.app` ouverte à tous. Définissez `MY_TRUSTED_PROXIES` dans `env` à ce qui se connecte au serveur. Ce que vous placez devant doit toujours répondre à `https://{host}`.

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` place Cloudflare devant `host`, avec le cache contourné pour tout, et `ingress-cloudflare-media` ajoute un hôte de médias mis en cache à côté de n’importe quel ingress. Lisez d’abord le README de chaque module ([app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md), [media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)). Le jeton va dans `CLOUDFLARE_API_TOKEN`. Ces modules n’ont fonctionné que sur des fournisseurs simulés.

- **Votre offre détermine l’origine.** Cloud Run route selon `Host`, et Cloudflare ne propose le remplacement de l’en-tête Host que dans l’offre Enterprise. Sur toute autre offre, une origine `*.run.app` fait échouer le plan, donc l’origine doit accepter elle-même votre hôte : gardez un `ingress-gcp-domain-mapping` de cet hôte, hors de `backends`, et définissez `origin_host = "ghs.googlehosted.com"`. Chaque hôte a besoin de son propre mappage. Le fait que son certificat soit émis et renouvelé derrière le proxy n’est pas vérifié, commencez donc avec `proxied = false`.
- **Un en-tête ferme l’origine.** `ingress-cloudflare-app` ajoute une règle de transformation d’en-tête de requête, et le serveur refuse avec un `403` une requête sans l’en-tête, à l’exception des contrôles de santé et des `POST` propres aux tâches, qui vérifient leurs jetons. Ainsi, personne ne contourne Cloudflare via l’URL `run.app`, et un Cloudflare Worker appartenant à un tiers ne peut pas choisir l’adresse client que voient les limites de débit.
- **La valeur se retrouve dans l’état.** Le fournisseur Cloudflare n’a pas de forme en écriture seule pour la valeur d’en-tête d’une règle. Générez-en une avec `openssl rand -hex 32`, ajoutez-la à un secret avec `gcloud secrets versions add`, passez la même valeur avec `TF_VAR_origin_secret`, et gardez l’état privé. [Le guide du CDN](cdn.md#rotate-the-value) explique comment la renouveler.
- **Rien n’ajoute l’en-tête tant qu’un enregistrement n’est pas proxifié**, donc le serveur refuse tout sauf ses contrôles de santé : avec `proxied = false` pour le certificat d’un mappage de domaine, configurez le secret après cela.
- **Désactivez Bot Fight Mode** dans le tableau de bord. Il couvre toute la zone et aucune règle ne peut l’ignorer, et un défi sur `/ap/`, `/.well-known/` ou `/api/` casse la fédération, car les autres serveurs ne sont pas des navigateurs. Les tâches ne passent pas par Cloudflare. Les modules n’ajoutent aucun défi et désactivent Browser Integrity Check, mais ils ne gèrent pas Bot Fight Mode. N’activez pas « I’m Under Attack » pour la même raison.
- **Activez Always Use HTTPS** dans le tableau de bord si vous voulez que `http://` soit redirigé. Il couvre toute la zone, donc les modules vous le laissent.
- **Délais d’attente** : Cloudflare répond `524` lorsque l’origine n’envoie rien pendant 125 secondes. Les tâches ne sont pas concernées, puisqu’elles appellent directement l’URL `run.app`. Les flux envoient un signal de présence toutes les 30 secondes, sous la limite d’inactivité de 900 secondes. Le fait qu’il mette ou non les flux en mémoire tampon sur un hôte proxifié ordinaire n’est pas vérifié.
- **Téléversements** : 100 Mo sur Free et Pro, au-dessus des 32 MiB de Cloud Run, qui reste la limite.
- **Vidéo** : les conditions de Cloudflare restreignent la diffusion de vidéo et d’une quantité disproportionnée de gros fichiers sur Free, Pro et Business, et il peut rediriger le contenu ou prendre d’autres mesures. Décidez-en avant de mettre de la vidéo en cache sur l’hôte de médias.

**Un fichier supprimé reste en cache.** L’hôte de médias suit le `Cache-Control` du serveur, donc un fichier local supprimé peut rester visible jusqu’à un an, et un fichier distant pendant `remote_media_cache_ttl_seconds`. Le serveur ne le purge pas. Lorsque quelque chose doit disparaître, purgez-le à la main, sous **Caching > Configuration > Custom Purge** dans le tableau de bord, ou :

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Une URL de proxy contient son `sig` dans la chaîne de requête, qui fait partie de ce qui est mis en cache, donnez donc l’URL entière.

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## Changer le backend de la KEK

Remplacer le bloc `kek` revient à un bloc dans `main.tf`, mais sur une instance contenant des données, ce n’est pas gratuit. La base de données contient des secrets enveloppés avec la clé actuelle, donc une instance qui la perd ne peut plus les lire, et les clés de chaque compte en font partie.

Suivez [Passer de la KEK locale à un KMS et inversement](key-management.md#move-between-the-local-kek-and-a-kms). Du secret vers Cloud KMS, conservez la KEK actuelle comme `kek_previous` pendant que les secrets sont déplacés, en passant son secret par le module app, avec `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }` ; exécutez `my kek rewrap` via [la console](#the-console), puis retirez-la lorsque `my kek status` ne compte plus de clés `local:`. Conservez la KEK aussi longtemps que vous gardez des sauvegardes de la base de données antérieures au rewrap.

Sur une nouvelle instance sans données, rien n’a besoin d’être déplacé.

<!-- source: limits 5bd276dd3a89 -->
## Limites

- **Téléversements** : Cloud Run refuse une requête HTTP/1 au-delà de 32 MiB, et le serveur parle HTTP/1.1. L’exemple règle donc `MY_MEDIA_MAX_AV_BYTES` à 30 MiB, contre 100 MiB par défaut, afin qu’un téléversement plus volumineux reçoive la réponse propre du serveur.
- **Flux** : le délai d’expiration de la requête, 15 minutes par défaut, met fin à un flux ouvert, et le client se reconnecte.
- **Adresses des clients** : les limites de débit s’appuient sur `MY_TRUSTED_PROXIES`, que le module app définit à partir de ce que renvoie le module `ingress`. Aucun document que nous avons lu ne dit ce que Cloud Run ajoute à `X-Forwarded-For` pour une requête directe, ni quelle adresse de pair voit un conteneur, et le contrat du conteneur n’en dit rien, donc `169.254.0.0/16` est une hypothèse. La forme documentée du répartiteur de charge externe est `[<supplied>,]<client-ip>,<lb-ip>`. Vérifiez les adresses dans les premiers journaux de requêtes : une IP client qui est une adresse `169.254` ou une adresse Google signifie que le paramètre est erroné pour votre projet.
- **Accès public** : le service accorde `roles/run.invoker` à `allUsers`, ce qu’une politique d’organisation de partage restreint au domaine refuse. Un tel projet a besoin d’une exception.

<!-- source: costs 1ab8d07f6847 -->
## Coûts

Un serveur coûte de l’argent. Ces chiffres donnent une idée de ce à quoi s’attendre et de l’endroit où l’attention réduit les coûts, et non des limites auxquelles le déploiement serait tenu. Ce sont des prix catalogue approximatifs pour une région, datant de 2026-10 et 2026-11. Vérifiez-les sur les pages de tarifs de [Cloud Run](https://cloud.google.com/run/pricing) et de [Neon](https://neon.com/pricing). Le calcul est gratuit par compte de facturation jusqu’à 180 000 vCPU-secondes et 360 000 GiB-secondes par mois.

| Élément | À l’arrêt | Ce qui s’y ajoute |
| --- | --- | --- |
| Répartiteur de charge | Environ 18 $ par mois, non confirmé sur la page de tarifs | Un tarif par Go ; voir [Comment le trafic atteint le service](#how-traffic-reaches-the-service) |
| Neon, offre Launch | Environ 19 $ par mois, avec le tick par défaut | Stockage à 0,35 $ par Go-mois. `db-gcp-cloud-sql` a en revanche un coût fixe d’environ 9,40 $ par mois, pour une machine plus petite : son README détaille le calcul |
| Cloud Run | 0 $ : le nombre minimal d’instances est 0 et le CPU n’est facturé que pendant les requêtes | Chaque flux ouvert maintient une instance facturée ; voir ci-dessous |
| Cloud Tasks | 0 $ | 1 million d’opérations gratuites, puis 0,40 $ par million. Une tâche en consomme environ 2 |
| Cloud Scheduler | 0 $ : 3 jobs gratuits par compte de facturation | 0,10 $ par job au-delà |
| Cloud Storage | Environ 0,02 $ par Gio et par mois | 5 Gio ne sont gratuits que dans `us-east1`, `us-west1` et `us-central1`. Les médias sortent par Cloud Run |
| Pub/Sub | 0 $ : 10 Gio gratuits | Seulement tant que des flux sont ouverts |
| Secret Manager | Quelques centimes | Quelques versions gratuites, puis quelques centimes par version |

**Le tick maintient Neon éveillé.** Chaque tick lit et écrit dans la base de données, et Neon n’arrête un calcul qu’après 5 minutes sans activité, donc avec le `* * * * *` par défaut, le calcul ne s’arrête jamais. Cela représente 730 heures par mois, soit 182,5 CU-heures à 0,25 CU, ce qui correspond aux 19 $ ci-dessus. Le tick reste à chaque minute afin que les tâches périodiques s’exécutent à l’heure, et qu’un signal de réveil perdu soit rattrapé rapidement.

**Un flux ouvert maintient une instance facturée.** Avec la facturation à la requête, une instance est facturée tant qu’elle traite une requête, et les mises à jour en temps réel forment une longue requête. Un client qui garde son flux ouvert tout le mois coûte une instance en continu : environ 60 $ par mois pour 1 vCPU et 512 MiB, après le quota gratuit. Davantage de flux se partagent l’instance, jusqu’à `concurrency`. Si les flux restent ouverts la plupart du temps, comparez avec la facturation à l’instance, ou avec une petite VM et [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 5711f378aadb -->
### Où l’attention réduit les coûts

- **Le mappage de domaine** à la place du répartiteur de charge supprime les 18 $, avec [les compromis ci-dessus](#how-traffic-reaches-the-service). Il convient pour essayer un déploiement.
- **L’offre Free de Neon** supprime les 19 $, et une petite instance peut y fonctionner, avec ces compromis :
  - Elle autorise 100 CU-heures par mois, que le tick par défaut épuise en 17 jours environ. Restez dans cette limite avec un calendrier plus espacé, comme `tick_schedule = "*/10 * * * *"` : éveillé au plus la moitié du temps, environ 91 CU-heures, avant que quoi que ce soit d’autre ne le réveille. Les tâches périodiques s’exécutent alors avec jusqu’à 10 minutes de retard, et un signal de réveil perdu est rattrapé plus tard.
  - Un serveur fédéré reçoit des livraisons en continu, ce qui réveille aussi la base de données, donc une instance occupée dépasse l’offre quel que soit le calendrier.
  - Elle contient 1 Go par projet et un historique de 6 heures à partir duquel restaurer, et son calcul s’arrête toujours après 5 minutes d’inactivité, donc la première requête qui suit l’attend.
