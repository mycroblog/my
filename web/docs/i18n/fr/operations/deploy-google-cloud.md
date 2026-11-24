<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# Déployer sur Cloud Run

Comment exécuter `my` sur Google Cloud avec le module OpenTofu [`mycroblog-gcp`](../../deploy/opentofu/README.md) : Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage et Pub/Sub, avec une base de données PostgreSQL sur Neon, derrière un Application Load Balancer externe global. Le service n’exécute aucune instance tant que personne ne l’utilise, à l’exception d’un tick qui réveille le serveur chaque minute. Un serveur coûte de l’argent : comptez environ 37 $ par mois avant le stockage, le trafic et les flux ouverts, et lisez [Coûts](#costs) pour ce que l’on peut réduire avec un peu de soin.

Le module exige OpenTofu 1.11 ou une version ultérieure. Terraform n’est pas pris en charge.

<!-- source: what-it-creates cae1e55a1722 -->
## Ce qu’il crée

Un seul bloc de `mycroblog-gcp` crée tout cela. Chaque ligne est une section du module, et sa valeur par défaut est ce que ce guide déploie ; [le README du module](../../deploy/opentofu/README.md#sections) liste les autres types.

| Partie | Section | Ressources |
| --- | --- | --- |
| Serveur | `service`, `jobs` | Un service Cloud Run avec `role = all`, un job de migration, un job de console tant que vous en avez besoin, un compte de service et un accès public |
| Trafic | `ingress` | Un Application Load Balancer externe global avec un certificat. Un mappage de domaine ou Cloudflare sont les autres types, et un hôte de médias sur Cloudflare peut se placer à côté de n’importe lequel |
| Jobs | `queue` | Une file Cloud Tasks, un job Cloud Scheduler et un compte de service qui appelle le serveur |
| Médias | `media` | Un bucket privé |
| Mises à jour en temps réel | `stream` | Un sujet et un rôle personnalisé. Le serveur crée et supprime ses propres abonnements |
| Base de données | `database` | Un secret pour son URL : vous créez la base de données à la main |
| E-mail | `mail` | Un secret pour le mot de passe SMTP : n’importe quel service SMTP |
| Secrets | | Une clé VAPID générée, et les secrets que vous seul pouvez remplir, vides |
| KEK | `kek` | Un secret généré monté comme fichier. Cloud KMS est l’autre type |
| Métriques, traces, erreurs | `observability` | Les API de Cloud Monitoring, Cloud Trace et Error Reporting, qui lisent ce que Cloud Run leur envoie |

Les valeurs des secrets n’atteignent jamais le plan ni l’état d’OpenTofu : vous ajoutez vous-même celles qui viennent d’ailleurs, et le module génère la KEK et la clé VAPID au moyen d’attributs en écriture seule.

<!-- source: before-you-start 10846ce6f3b1 -->
## Avant de commencer

- `tofu` 1.11 ou une version ultérieure, `gcloud` et `jq`, là où vous appliquez : une application qui change la version exécute la migration à partir de là, par `gcloud`.
- Un projet Google Cloud avec facturation, utilisé pour cette seule instance, et un compte sur ce projet avec le rôle Owner.
- Un domaine pour [`host`](../reference/configuration.md#host). Il ne peut pas être changé par la suite : lisez [Paramètres que vous ne pouvez pas changer ensuite](production-checklist.md#settings-you-cannot-change-later).
- Une version de `my` issue des [versions publiées](running.md#get-the-image). Il n’y a pas de balise `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Premier déploiement

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. Activer les API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` sert au répartiteur de charge.

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. Créer la base de données sur Neon

Attendez-vous à la formule Launch : avec le tick par défaut, le calcul ne s’arrête jamais. La formule Free fonctionne aussi, avec les compromis décrits sous [Coûts](#costs).

Dans la console Neon, créez un projet dans une région proche de votre région Cloud Run, et copiez ses chaînes de connexion. Le module ne le crée jamais ; son mot de passe reste donc hors de l’état d’OpenTofu. Un projet a deux chaînes de connexion, et le déploiement utilise chacune là où elle convient :

- La chaîne **directe** (l’hôte sans `-pooler`) est obligatoire. Le job de migration et la console l’utilisent toujours : `my schema apply` tient un verrou de niveau session, qu’un pooler en mode transaction ne conserve pas.
- La chaîne **poolée** (l’hôte avec `-pooler`) est facultative, pour le service. Le pooler de Neon gère ce dont le serveur a besoin : voir [Par un pooler de connexions](database.md#through-a-connection-pooler). Définissez `database = { pooled = true }`, et le module crée un second secret pour elle.

Sans chaîne poolée, le service utilise aussi la chaîne directe, à la limite de connexions de la base de données, que `service.max_instances` borne. Pour relever le pool de sa valeur par défaut de 4 par instance, pour les 80 requêtes qu’une instance Cloud Run accepte à la fois, ajoutez `pool_max_conns` à la chaîne que le service utilise, par exemple `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Voir [Attendre une connexion à la base de données](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. Copier l’exemple et définir les variables

Partez d’une copie dans un répertoire vide, avec le fichier de verrouillage des fournisseurs, de la version que vous déployez :

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` contient le fournisseur `google` et un seul bloc, `module "mycroblog"`. **Définissez `ref` sur cette version.** Le bloc nomme une version avec `?ref=`, que la copie d’une version stable a déjà correctement définie. L’image du serveur est la même version ; `ref` seul nomme donc ce qui s’exécute. Pour une préversion, définissez les deux vous-même : `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, et `image = { version = "0.1.0-rc.4" }` dans le bloc, la version sans son `v`, puisque le module d’une préversion n’utilise pas par défaut sa propre image. Le module n’existe qu’à partir de la première version qui le contient ; une balise antérieure n’a donc aucun exemple à copier. Conservez ce répertoire : il contient votre état et vos paramètres.

Remplissez `terraform.tfvars`. `mail_from`, `mail_smtp_host` et `mail_smtp_username` configurent l’[e-mail](email.md) ; le port est 587, car Google Cloud bloque le port sortant 25. Pour changer autre chose, ajoutez la section au bloc de `main.tf`, comme `ingress = { kind = "domain-mapping" }` : chaque section et ce qu’elle refuse figurent dans [le README du module](../../deploy/opentofu/README.md#sections).

`settings` accepte tout autre paramètre de la [référence de la configuration](../reference/configuration.md) par sa clé, et `secrets` les clés des paramètres secrets, chacun recevant un secret vide à remplir. Une clé qu’une section ou le module définit fait échouer le plan, tout comme une clé que le module n’accepte pas sur Cloud Run, comme `listen` ; l’erreur en indique la raison. Décidez dès maintenant de [`account_domain`](../reference/configuration.md#account_domain), sous la forme `settings = { account_domain = "example.org" }`, si les identifiants utilisent un autre domaine.

**L’état** reste par défaut dans ce répertoire. Il ne contient aucune valeur de secret, mais c’est le seul enregistrement de ce qui a été créé : conservez-le dans un bucket avec un bloc `backend "gcs"` si plus d’une personne exécute ceci.

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. Créer les secrets et ajouter leurs valeurs

Cela crée les secrets, et génère la KEK et la clé VAPID :

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` récupère aussi les scripts de cette version : `migrate.sh`, qu’une application exécute, et `console.sh`, qu’une étape ultérieure utilise :

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**Copiez la KEK maintenant**, et conservez la copie à part de la base de données, par exemple dans un gestionnaire de mots de passe. Sans elle, une [sauvegarde](backup.md) ne peut pas être restaurée, et les clés des comptes sont perdues. `tofu output kek` donne la commande. Faites de même avec la clé VAPID, dont la perte met fin à chaque abonnement Web Push :

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Ajoutez ensuite les valeurs qui viennent d’ailleurs. `tofu output secrets_to_fill` liste chaque secret que le module a créé vide, avec ce qui doit y figurer et la commande qui l’ajoute. Une instance dont un secret ne peut pas être lu ne démarre pas ; ajoutez-les donc avant le service :

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

La KEK parvient au serveur sous forme de fichier monté, nommé par `MY_KEK_FILE`. Les autres sont des variables d’environnement résolues au démarrage d’une instance ; une nouvelle version prend donc effet à la prochaine instance : déployez une nouvelle révision pour vous en assurer.

**OpenTofu génère la KEK une seule fois, et jamais plus.** Le secret et sa version ont `prevent_destroy`, et le numéro de version qui écrirait une nouvelle valeur est fixé dans le module : une nouvelle KEK aléatoire rend illisible tout ce que l’ancienne a chiffré. Pour la renouveler, suivez [Renouveler la KEK](key-management.md#rotate-the-kek), en ajoutant la nouvelle valeur avec `gcloud secrets versions add`. OpenTofu ne suit pas ce que vous y ajoutez.

Pour conserver la KEK dans Cloud KMS dès le départ, définissez `kek = { kind = "kms" }` avant cette étape, ciblez `module.mycroblog.module.kek_kms` à la place de `kek_secret_manager`, et activez `cloudkms.googleapis.com`.

<!-- source: 5-apply 2076866c0f3b -->
### 5. Appliquer

```sh
tofu apply
```

Cela crée tout le reste, dans un ordre dont la sonde de démarrage du service a besoin : la sonde attend `/health/ready`, qui répond `503` tant que le schéma n’existe pas. Le job de migration vient en premier, avec ce dont il dépend, ce qui inclut l’adresse du répartiteur de charge, puisque les paramètres du serveur la nomment. L’application exécute ensuite `my upgrade` comme une exécution du job, depuis votre machine par `gcloud`, et l’attend : `my config check`, `my db upgrade -check`, `my schema apply` et `my db upgrade`, qui s’arrête à la première qui échoue. Sa sortie apparaît dans celle de l’application. La première commande qui utilise la base de données y enregistre `host` et `account_domain`. Ce n’est qu’ensuite que l’application crée les services.

Si la migration échoue, l’application s’arrête avant les services. Corrigez la cause, souvent un secret sans valeur, et exécutez de nouveau `tofu apply` : il relance la migration.

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. Faire pointer le domaine vers le répartiteur de charge

`tofu output dns_records` liste un enregistrement `A`, de `host` vers l’adresse du répartiteur de charge. Ajoutez-le chez votre fournisseur DNS. Si ce fournisseur a un proxy propre, laissez-le désactivé pour cet enregistrement.

Le certificat est géré par Google et ne devient actif qu’une fois que le DNS pointe vers l’adresse, ce qui peut prendre un certain temps. D’ici là, `https://{host}` échoue avec une erreur TLS ; la fédération et les navigateurs ne peuvent donc pas encore atteindre le serveur. Rien d’autre n’attend ce moment : Cloud Tasks et Cloud Scheduler appellent l’URL `run.app` par défaut du service, `https://<service>-<project number>.<region>.run.app`. **Ne désactivez pas l’URL par défaut**, sinon les jobs s’arrêtent. Cette URL refuse les requêtes venant d’Internet, et Cloud Run ne compte ces deux appels comme internes que depuis le même projet et sur cette URL, pas sur un domaine personnalisé.

Voir [Comment le trafic atteint le service](#how-traffic-reaches-the-service) pour ce que vous pouvez choisir d’autre.

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. Créer le premier administrateur

Les jobs Cloud Run n’ont ni terminal ni entrée standard ; `my admin create` affiche donc un lien qui définit le mot de passe au lieu d’en lire un. Activez la [console](deploy-opentofu.md#the-console) et exécutez :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Ouvrez le lien aussitôt et définissez le mot de passe. Il ne fonctionne qu’une fois, pendant une heure. **Il reste dans Cloud Logging jusque-là**, où toute personne qui peut lire les journaux du projet peut l’utiliser. S’il expire, exécutez `my admin account password-reset-link @alice` par la console. Activez ensuite l’authentification à deux facteurs.

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. Parcourir la liste de contrôle

Parcourez la [liste de contrôle de production](production-checklist.md), en commençant par l’[e-mail](production-checklist.md#email) : SPF, DKIM et DMARC sur le domaine de `mail_from`.

<!-- source: the-console 52df6f8c43ac -->
## La console

La console exécute une commande `my` avec l’image, les paramètres et les secrets du service. Elle n’existe que tant que `console_enabled` est activé ; désactivez-la donc lorsque vous avez terminé :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` attend la commande, affiche sa sortie et son code de sortie, et se termine avec ce code. Il exige `roles/logging.viewer`, car la sortie d’un job va dans Cloud Logging. Il utilise l’image que le service exécute : après une mise à niveau qui s’est arrêtée à la migration, c’est la version précédente, à laquelle la base de données correspond encore.

La commande n’a ni entrée standard ni terminal ; une commande qui y lit un mot de passe ou un fichier ne peut donc pas s’exécuter ainsi. Ce qu’elle affiche reste dans les journaux ; n’exécutez donc pas une commande dont vous n’écririez pas la sortie à cet endroit.

<!-- source: upgrading fe1bed77d3af -->
## Mettre à niveau

Lisez d’abord [Mettre à niveau](database.md#upgrading) et les notes de version, et avancez d’une version mineure à la fois. Une mise à niveau est une application de la nouvelle version : le plan montre chaque service et chaque job qui prend son image, et l’application exécute la migration avant que l’un d’eux la serve.

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

L’image est la version que nomme `ref` ; changer `ref` suffit donc à mettre à niveau. Lisez le plan avant de l’appliquer : le module d’une nouvelle version peut aussi modifier des ressources. L’application fait ceci, et s’arrête à la première étape qui échoue :

1. Donne aux jobs de migration et de console la nouvelle image, avec le reste du plan dont les services ne dépendent pas.
2. Exécute `my upgrade` comme une exécution du job de migration, depuis l’endroit où vous appliquez, et l’attend : `my config check`, `my db upgrade -check`, `my schema apply` et `my db upgrade`. Sa sortie apparaît dans celle de l’application.
3. Déploie la nouvelle révision de chaque worker, puis du service. Une révision ne reçoit du trafic que lorsque sa sonde de démarrage réussit sur `/health/ready`.

**L’endroit où vous appliquez** a besoin de `gcloud` et `jq`, et d’identifiants qui peuvent exécuter le job et lire ses journaux (`roles/run.developer` et `roles/logging.viewer`, que Owner inclut). Il en va de même pour un job de CI qui applique.

**Lorsqu’une étape échoue**, les services restent sur leurs révisions, et les notes de version ou la sortie de la migration en indiquent la raison. Corrigez la cause et exécutez de nouveau `tofu apply`, avec la même version : il relance la migration, et les étapes terminées ne trouvent plus rien à faire. Une application qui a été interrompue, ou qui a perdu sa connexion, laisse l’exécution en cours sur Cloud Run ; l’application suivante attend qu’elle se termine avant de lancer la sienne. Pour revenir en arrière après un changement de schéma, restaurez la sauvegarde : voir [Revenir en arrière](database.md#rolling-back). Une application avec une version antérieure s’arrête à la migration, puisque cette version refuse un schéma plus récent, et ne change aucun service.

Pour essayer une version candidate, ou pour conserver l’image d’une version antérieure sous un module plus récent, définissez vous-même la version ; c’est la balise de l’image, sans le `v` de la version :

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### Vérifier d’abord le SQL

Le plan n’affiche pas le SQL de la migration. Pour le lire avant l’application, donnez au job de migration la nouvelle image seule, et exécutez-y `my upgrade -dry-run`, qui ne change rien :

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### Changements destructifs

Lorsque les notes de version exigent `-allow-destructive`, nommez cette version dans le bloc :

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` ne reçoit `-allow-destructive` que lors de l’application de cette version ; une valeur laissée en place ne permet donc à aucune version ultérieure de supprimer des données.

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### Épingler l’image par son condensat

La [liste de contrôle de production](production-checklist.md#operations) recommande un condensat, tiré des notes de version :

```hcl
  image = { digest = "sha256:..." }
```

Un condensat nomme une seule image, quelle que soit la balise à côté ; modifiez-le donc dans la même édition que `ref` : un condensat laissé de la version précédente conserve l’image de cette version sous la nouvelle version.

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### Le mode maintenance

Le [mode maintenance](maintenance.md) demande trois applications, puisque les services changent avant la mise à niveau et après. Ajoutez-le aux paramètres du bloc et appliquez, faites la sauvegarde, relevez `ref` et appliquez, puis retirez-le et appliquez :

```hcl
  settings = { maintenance = "true" }
```

Sans lui, l’ancienne révision continue de servir pendant la migration, ce qui [n’est pas garanti de fonctionner](database.md#the-server-during-an-upgrade). Les paramètres modifiés à la main avec `gcloud` ne durent pas : la prochaine application remet ce que dit le bloc.

<!-- source: monitoring-and-alerts 930234cd0dde -->
## Surveillance et alertes

Le module ajoute une vérification de disponibilité de `/health/live`, des métriques basées sur les journaux et des politiques d’alerte portant les noms du [guide d’exploitation](alerts.md), qui notifient les adresses de `monitoring.notification_emails` (`alert_emails` dans l’exemple) ou les canaux de `notification_channels`. L’entrée n’a pas de valeur par défaut : nommez qui est notifié, ou définissez `monitoring = null` pour tout omettre. Elle n’a jamais été appliquée ; essayez-la donc une fois en faisant échouer une vérification. Chaque exécution de la vérification réveille une instance, ce qui s’ajoute au [compromis du tick](#costs) : `uptime_period`, 15 minutes par défaut, en est la fréquence.

<!-- source: changing-a-backend c20598caf230 -->
## Changer de backend

Changez le `kind` de sa section. Chaque type définit ce dont il a besoin sur le serveur, et le plan indique ce qui change. Changer de base de données signifie aussi déplacer ses données, puisqu’une nouvelle base est vide : faites une [sauvegarde](backup.md) de l’ancienne et restaurez-la dans la nouvelle pendant que rien n’écrit. Par exemple, Cloud SQL à la place de Neon :

```hcl
  database = { kind = "cloud-sql" }
```

Pour un stockage extérieur à Google Cloud, définissez `media.kind` sur `s3`, pour tout service compatible S3 avec son `endpoint`, ou sur `r2`, avec `cloudflare.account_id`. Les deux lisent une clé d’accès dans les secrets que liste `secrets_to_fill`.

<!-- source: when-you-need-a-network 0a4359c002ed -->
## Quand vous avez besoin d’un réseau

Ce guide n’en exige aucun : Neon, Cloud Tasks, Cloud Storage et Pub/Sub sont tous atteints par Internet avec l’identité propre du service ; Cloud Run ne se rattache donc à rien et il n’y a rien à payer ni à surveiller.

Vous en avez besoin pour placer Memorystore dans le déploiement, puisqu’il n’a que des adresses privées, ou pour donner à Cloud SQL une adresse privée à la place de son adresse publique. `network = { kind = "vpc" }` en crée un, et `network = { kind = "existing", ... }` se rattache à un VPC que vous avez, que vous décrivez par `network`, `subnetwork`, `private_services_range` et `memorystore_connection_policy` :

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Seules les plages privées passent par le réseau ; le service atteint donc Internet comme avant, sans Cloud NAT. `nat = true` envoie tout par Cloud NAT à la place, pour une adresse source statique. Activez d’abord les API Service Networking et Network Connectivity, ainsi que l’API Compute Engine. Un réseau s’ajoute au démarrage à froid d’une instance, et Memorystore est facturé même lorsque le serveur est inactif.

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## Répartir les jobs entre des workers

Lorsqu’un type de job retarde les autres, donnez-lui son propre service. Décidez de ce qu’il faut répartir d’après les métriques, comme l’indique [Répartir les workers par type de job](running.md#splitting-workers-by-job-type). Une seule valeur, `workers`, va au service, à la file et à la surveillance, et chaque file, URL, route et paramètre provient de ses clés ; vous n’en écrivez donc aucun :

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

Le plan refuse une répartition de types qui doivent rester ensemble. Il exige aussi un courtier qui atteint tous les processus, ce que `stream.kind = pubsub` est.

Les deux dispositions courantes de cette page ressemblent à ceci. Pour éviter que les livraisons retardent le reste, utilisez la valeur ci-dessus. Pour garder rapides les e-mails et les notifications :

```hcl
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
```

Omettez les types des paramètres que vous n’avez pas définis, comme l’indique cette page : un type que le serveur n’a pas enregistré arrête le démarrage, et le plan ne peut pas voir lesquels c’est. Avec `workers`, le service par défaut n’aide pas pour ces types lorsqu’il est libre, puisque leurs tâches vont dans leur propre file.

Chaque worker accepte `job_types`, et `min_instances`, `max_instances`, `cpu`, `memory_mib`, `concurrency`, `max_concurrent_dispatches` et `max_dispatches_per_second`, chacun valant celui du service ou de la file lorsqu’il n’est pas nommé. Le service d’un worker ne reçoit aucun trafic de l’extérieur. Une mise à niveau déplace chacun après la migration et avant le service par défaut, et sans rien pour le lui dire : seule une étape qui échoue l’arrête. Chaque worker peut apporter ce que coûte le service, et les connexions de ses instances comptent dans la limite de la base de données.

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## Comment le trafic atteint le service

`ingress.kind` en décide. La valeur par défaut est le répartiteur de charge, destiné à la production. Le mappage de domaine sert à essayer un déploiement à peu de frais : définissez `ingress = { kind = "domain-mapping" }`.

| | `load-balancer` (par défaut) | `domain-mapping` |
| --- | --- | --- |
| Frais mensuels | Environ 18 $ pour les règles de transfert, plus un tarif par Go. Vérifiez la [page des tarifs](https://cloud.google.com/vpc/network-pricing) : ce chiffre n’a pas pu y être confirmé | Aucun |
| Statut | La méthode recommandée par Google Cloud | Une fonctionnalité en préversion que Google Cloud ne recommande pas pour la production, à cause de la latence |
| Régions | Toutes | `asia-northeast1`, `us-central1` et quelques autres |
| DNS | Un enregistrement `A`, issu de `tofu output dns_records` | Les enregistrements que la même sortie liste |
| Certificat | Géré par Google, actif une fois que le DNS pointe vers l’adresse | Géré par Google, jusqu’à 24 heures |
| Propriété du domaine | Rien au-delà du DNS | Vous, et tout autre compte concerné, devez être [propriétaire vérifié](https://cloud.google.com/run/docs/mapping-custom-domains) avant d’appliquer |
| URL `run.app` | Refusée depuis Internet ; personne ne contourne donc le répartiteur de charge | Accessible à tous : un mappage de domaine ne peut pas ajouter d’en-tête ; l’origine reste donc ouverte |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` et l’adresse du répartiteur de charge | `169.254.0.0/16` |

Cloud Tasks et Cloud Scheduler n’arrivent par aucun des deux : ils appellent directement l’URL `run.app` par défaut.

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` place Cloudflare devant `host`, avec le cache contourné pour tout, et `ingress.media_host` ajoute un hôte de médias mis en cache à côté de n’importe quel type. Les deux exigent `cloudflare = { zone_id = "..." }`, et le jeton va dans `CLOUDFLARE_API_TOKEN`. Ils n’ont été exécutés que sur des fournisseurs simulés.

- **Votre formule décide de l’origine.** Cloud Run route par `Host`, et Cloudflare ne propose le remplacement de l’en-tête Host que sur la formule Enterprise. Sur toute autre formule, l’origine doit accepter elle-même votre hôte ; le module crée donc un mappage de domaine de chaque hôte, avec les compromis du tableau ci-dessus, et Cloudflare s’y connecte. Si son certificat est émis et renouvelé derrière le proxy n’est pas vérifié ; commencez donc avec `cloudflare.proxied = false`. Sur la formule Enterprise, `cloudflare.host_header_override = true` se connecte plutôt à l’URL `run.app`, sans mappage de domaine.
- **Un en-tête ferme l’origine.** Le module ajoute une règle de transformation d’en-tête de requête, et le serveur refuse par un `403` une requête sans l’en-tête, hormis les vérifications de santé et les `POST` propres aux jobs, qui vérifient leurs jetons. Personne ne contourne donc Cloudflare par l’URL `run.app`, et le propre Cloudflare Worker d’un appelant ne peut pas choisir l’adresse de client que voient les limites de débit.
- **La valeur atterrit dans l’état.** Le module la génère et l’écrit dans le secret `my-origin-secret`, mais le fournisseur Cloudflare n’a pas de forme en écriture seule pour la valeur d’en-tête d’une règle ; gardez donc l’état privé. [Le guide du CDN](cdn.md#rotate-the-value) explique comment fonctionne le renouvellement.
- **Rien n’ajoute l’en-tête tant qu’un enregistrement n’est pas proxifié**, de sorte que le serveur refuse tout sauf ses vérifications de santé : avec `proxied = false` pour le certificat d’un mappage de domaine, activez le proxy une fois le certificat actif.
- **Désactivez Bot Fight Mode** dans le tableau de bord. Il couvre toute la zone et aucune règle ne peut l’ignorer, et un défi sur `/ap/`, `/.well-known/` ou `/api/` casse la fédération, car les autres serveurs ne sont pas des navigateurs. Les jobs ne passent pas par Cloudflare. Le module n’ajoute aucun défi et désactive Browser Integrity Check, mais il ne gère pas Bot Fight Mode. N’activez pas « I’m Under Attack » pour la même raison.
- **Email Address Obfuscation peut rester activé.** Il réécrit les adresses e-mail du HTML en `[email protected]` et ajoute un script pour les restaurer, que la Content-Security-Policy du serveur refuse. Les propres pages du serveur, comme celle qui indique où est allé l’e-mail de confirmation, marquent leur contenu pour qu’il laisse les adresses tranquilles.
- **Activez Always Use HTTPS** dans le tableau de bord si vous voulez que `http://` soit redirigé. Il couvre toute la zone ; le module vous le laisse donc.
- **Délais** : Cloudflare répond `524` lorsque l’origine n’envoie rien pendant 125 secondes. Les jobs ne sont pas concernés, puisqu’ils appellent directement l’URL `run.app`. Les flux envoient un signal de présence toutes les 30 secondes, sous la limite d’inactivité de 900 secondes. S’il met en mémoire tampon les flux sur un hôte proxifié ordinaire n’est pas vérifié.
- **Téléversements** : 100 Mo sur Free et Pro, au-dessus des 32 MiB de Cloud Run, qui restent la limite.
- **Vidéo** : les conditions de Cloudflare restreignent la diffusion de vidéo et d’une quantité disproportionnée de gros fichiers sur Free, Pro et Business, et il peut rediriger le contenu ou prendre d’autres mesures. Décidez-le avant de mettre de la vidéo en cache sur l’hôte de médias.

**Un fichier supprimé reste en cache.** L’hôte de médias suit le `Cache-Control` du serveur ; un fichier local supprimé peut donc rester visible jusqu’à un an, et un fichier distant pendant `remote_media_cache_ttl_seconds`. Le serveur ne le purge pas. Lorsque quelque chose doit disparaître, purgez-le à la main, sous **Caching > Configuration > Custom Purge** dans le tableau de bord, ou :

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Une URL de proxy contient sa `sig` dans la chaîne de requête, qui fait partie de ce qui est mis en cache ; donnez donc l’URL complète.

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## Changer le backend de la KEK

Changer `kek.kind` tient en une ligne de `main.tf`, mais sur une instance qui contient des données, ce n’est pas gratuit. La base de données contient des secrets enveloppés avec la clé actuelle ; une instance qui la perd ne peut donc plus les lire, et les clés de chaque compte en font partie.

Suivez [Passer de la KEK locale à un KMS](key-management.md#move-between-the-local-kek-and-a-kms). Du secret à Cloud KMS, conservez la KEK actuelle comme `kek_previous` pendant que les secrets se déplacent : ajoutez `secrets = ["kek_previous"]`, et remplissez le secret `my-kek-previous` avec la KEK actuelle. Exécutez `my kek rewrap` par [la console](deploy-opentofu.md#the-console), puis retirez-la lorsque `my kek status` ne compte plus de clés `local:`. Conservez la KEK aussi longtemps que vous conservez des sauvegardes de la base de données antérieures à la ré-enveloppe.

Sur une nouvelle instance sans données, rien n’a besoin d’être déplacé.

<!-- source: limits 3ff113a57279 -->
## Limites

- **Téléversements** : Cloud Run refuse une requête HTTP/1 au-dessus de 32 MiB, et le serveur parle HTTP/1.1. Le module définit donc `MY_MEDIA_MAX_AV_BYTES` à 30 MiB, contre 100 MiB par défaut, afin qu’un téléversement plus volumineux reçoive la propre réponse du serveur.
- **Flux** : le délai d’expiration des requêtes, 15 minutes par défaut, met fin à un flux ouvert, et le client se reconnecte.
- **Adresses des clients** : les limites de débit reposent sur `MY_TRUSTED_PROXIES`, que le module définit à partir de l’ingress. Aucun document que nous ayons lu n’indique ce que Cloud Run ajoute à `X-Forwarded-For` pour une requête directe ni quelle adresse de pair voit un conteneur, et le contrat du conteneur n’en dit rien ; `169.254.0.0/16` est donc une hypothèse. La forme documentée du répartiteur de charge externe est `[<supplied>,]<client-ip>,<lb-ip>`. Vérifiez les adresses dans les premiers journaux de requêtes : une IP de client qui est une adresse `169.254` ou une adresse Google signifie que le paramètre est erroné pour votre projet.
- **Accès public** : le service accorde `roles/run.invoker` à `allUsers`, ce qu’une politique d’organisation de partage limité au domaine refuse. Un tel projet a besoin d’une exception.

<!-- source: costs b8be3997d723 -->
## Coûts

Un serveur coûte de l’argent. Ces chiffres sont un repère de ce à quoi s’attendre et de l’endroit où un peu de soin réduit les coûts, pas des limites auxquelles le déploiement est tenu. Ce sont des tarifs catalogue approximatifs pour une région, de 2026-10 et 2026-11. Vérifiez-les sur les pages de tarifs de [Cloud Run](https://cloud.google.com/run/pricing) et de [Neon](https://neon.com/pricing). Le calcul est gratuit par compte de facturation jusqu’à 180 000 vCPU-secondes et 360 000 GiB-secondes par mois.

| Partie | Inactif | Ce qui s’y ajoute |
| --- | --- | --- |
| Répartiteur de charge | Environ 18 $ par mois, non confirmé sur la page des tarifs | Un tarif par Go ; voir [Comment le trafic atteint le service](#how-traffic-reaches-the-service) |
| Neon, formule Launch | Environ 19 $ par mois, avec le tick par défaut | Le stockage à 0,35 $ par Go-mois. `database.kind = cloud-sql` a à la place un coût fixe d’environ 9,40 $ par mois, pour une machine plus petite |
| Cloud Run | 0 $ : le minimum d’instances est 0 et le CPU n’est facturé que pendant les requêtes | Chaque flux ouvert maintient une instance facturée ; voir ci-dessous |
| Cloud Tasks | 0 $ | 1 million d’opérations gratuites, puis 0,40 $ par million. Un job en demande environ 2 |
| Cloud Scheduler | 0 $ : 3 jobs gratuits par compte de facturation | 0,10 $ par job au-delà |
| Cloud Storage | Environ 0,02 $ par Gio et par mois | 5 Gio sont gratuits seulement dans `us-east1`, `us-west1` et `us-central1`. Les médias sortent par Cloud Run |
| Pub/Sub | 0 $ : 10 Gio gratuits | Seulement tant que des flux sont ouverts |
| Secret Manager | Quelques centimes | Quelques versions gratuites, puis quelques centimes par version |

**Le tick garde Neon éveillé.** Chaque tick lit et écrit dans la base de données, et Neon n’arrête un calcul qu’après 5 minutes sans activité ; avec le `* * * * *` par défaut, le calcul ne s’arrête donc jamais. Cela fait 730 heures par mois, soit 182,5 CU-heures à 0,25 CU, ce qui correspond aux 19 $ ci-dessus. Le tick reste à chaque minute afin que les jobs périodiques s’exécutent à l’heure, et qu’un signal de réveil perdu soit rattrapé rapidement.

**Un flux ouvert maintient une instance facturée.** Avec la facturation à la requête, une instance est facturée tant qu’elle traite une requête, et les mises à jour en temps réel sont une longue requête. Un client qui garde son flux ouvert tout le mois coûte une instance en continu : environ 60 $ par mois à 1 vCPU et 512 MiB, après le quota gratuit. D’autres flux se partagent l’instance, jusqu’à `concurrency`. Si les flux restent ouverts la plupart du temps, comparez avec la facturation à l’instance, ou avec une petite VM et [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### Où un peu de soin réduit les coûts

- **Le mappage de domaine** à la place du répartiteur de charge supprime les 18 $, avec [les compromis ci-dessus](#how-traffic-reaches-the-service). Il convient pour essayer un déploiement.
- **La formule Free de Neon** supprime les 19 $, et une petite instance peut fonctionner dessus, avec ces compromis :
  - Elle autorise 100 CU-heures par mois, que le tick par défaut consomme au bout d’environ 17 jours. Restez dans cette limite avec un calendrier plus espacé, comme `queue = { tick_schedule = "*/10 * * * *" }` : au plus la moitié du temps éveillé, environ 91 CU-heures, avant que quoi que ce soit d’autre le réveille. Les jobs périodiques s’exécutent alors jusqu’à 10 minutes en retard, et un signal de réveil perdu est rattrapé plus tard.
  - Un serveur fédéré reçoit des livraisons en continu, ce qui réveille aussi la base de données ; une instance occupée la dépasse donc quel que soit le calendrier.
  - Elle contient 1 Go par projet et un historique de 6 heures à partir duquel restaurer, et son calcul s’arrête toujours après 5 minutes d’inactivité ; la première requête après cela l’attend donc.
