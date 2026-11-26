<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# Déployer avec OpenTofu

Ce que partage tout déploiement avec les modules racine OpenTofu : [Déployer sur Google Cloud](deploy-google-cloud.md) et
[Déployer sur AWS](deploy-aws.md) ajoutent ce dont leur plateforme a besoin. Les deux modules ont besoin
d’[OpenTofu](https://opentofu.org) 1.11 ou version ultérieure. Terraform n’est pas pris en charge.

<!-- source: the-root-modules 4e0bf528bd0e -->
## Les modules racine

Un seul bloc d’un module racine déploie tout le serveur : le service, ses jobs et un backend pour chaque rôle.

| Module racine | Plateforme | Exemple à copier |
| --- | --- | --- |
| `mycroblog-gcp` | Cloud Run sur Google Cloud | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | ECS sur Fargate, sur AWS. Il n’a jamais été appliqué | `examples/aws-ecs-neon` |

Un exemple est le fournisseur de la plateforme et un seul bloc, `module "mycroblog"`, dont la source est une source Git épinglée à une
version :

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

L’épinglage est obligatoire : un module non épinglé pourrait changer sous vos pieds les ressources de la KEK. L’image du serveur prend par défaut la
version que nomme `ref` ; `ref` seul nomme donc ce qui s’exécute, et l’augmenter est une mise à niveau. Le module n’existe qu’à partir de la première
version qui le contient.

<!-- source: sections 728259c0b297 -->
### Sections

Chaque rôle est une section du bloc, un objet dont `kind` choisit le backend, comme `database = { kind = "external" }`.
Chaque champ a une valeur par défaut, la configuration recommandée ; vous n’écrivez donc que ce que vous changez. Une combinaison qui ne peut pas fonctionner échoue
avant le plan, avec un message qui nomme les entrées. [La référence des modules](../../deploy/opentofu/README.md#sections)
liste les sections, les kinds et les champs de chaque module.

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### Autres paramètres et secrets

`settings` accepte tout autre paramètre de la [référence de configuration](../reference/configuration.md) par sa clé, et le
module le transmet au service, aux workers et aux jobs. `secrets` accepte les clés des paramètres secrets : pour chacune, le
module crée un secret vide `<name>-<key>`, avec des tirets à la place des tirets bas, le transmet au serveur et le liste pour que vous le remplissiez.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

Une clé qu’une section définit ou que le module fixe fait échouer le plan, tout comme une clé que le module n’accepte pas, telle que `listen` :
l’erreur en donne la raison. Une section nulle, comme `translation`, et un signal de `observability` réglé sur `settings`
ne définissent rien ; `settings` peut donc nommer leurs clés. Les rapports d’erreurs vers Sentry, par exemple :

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

Les paramètres modifiés à la main sur la plateforme ne durent pas : la prochaine application remet ce que dit le bloc.

<!-- source: secrets d58c587dc144 -->
### Secrets

Aucune valeur de secret que vous donnez n’atteint le plan ni l’état OpenTofu. Le module génère ce qu’il peut grâce à des attributs en
écriture seule : la KEK, la clé VAPID et le mot de passe d’une base de données qu’il crée. La seule exception est le secret d’origine de
`ingress.kind = cloudflare`, que la règle de Cloudflare détient et qui se retrouve donc dans l’état, lequel doit rester privé.

Les secrets que vous seul avez, comme l’URL d’une base de données externe, un mot de passe SMTP, une clé d’accès ou les clés de
`secrets`, le module les crée vides. `tofu output secrets_to_fill` liste chacun, avec ce qu’il doit contenir et la commande qui
l’ajoute. Un serveur dont un secret n’a pas de valeur ne démarre pas. Le serveur lit ses secrets au démarrage d’une instance ou d’une tâche ;
une nouvelle valeur prend donc effet au démarrage suivant.

**OpenTofu génère la KEK une seule fois et jamais plus.** Le secret et sa version ont `prevent_destroy`, et la version qui
écrirait une nouvelle valeur est fixée dans le module : une nouvelle KEK aléatoire rend illisible tout ce que l’ancienne a chiffré. Pour
la renouveler, suivez [Renouveler la KEK](key-management.md#rotate-the-kek), en ajoutant la nouvelle valeur avec la CLI de la plateforme, que
OpenTofu ne suit pas.

<!-- source: first-deployment 9ae06fd14fc1 -->
## Premier déploiement

Il vous faut `tofu` 1.11 ou ultérieur, `jq` et la CLI de la plateforme (`gcloud` ou `aws`) là où vous appliquez, car une application exécute la migration depuis
cet endroit ; un domaine pour [`host`](../reference/configuration.md#host), qui ne peut pas changer ensuite
([Paramètres que vous ne pouvez pas changer ensuite](production-checklist.md#settings-you-cannot-change-later)) ; et une version de `my` tirée des
[versions publiées](running.md#get-the-image). Il n’y a pas de tag `latest`. Faites d’abord ce que demande la page de la plateforme, comme créer
la base de données.

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. Copier l’exemple et définir les variables

Copiez l’exemple, avec son fichier de verrouillage des fournisseurs, depuis la version que vous déployez dans un répertoire vide :

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Définissez `ref` sur cette version** dans `main.tf` ; la copie d’une version stable l’a déjà correctement. Pour une préversion, définissez à la fois
`ref` et l’image, la version sans son `v`, car le module d’une préversion n’a pas pour image par défaut la sienne :
`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, et `image = { version = "0.1.0-rc.4" }` dans le bloc.

Remplissez `terraform.tfvars`, et ajoutez au bloc de `main.tf` toute section que vous changez. Décidez dès maintenant de
[`account_domain`](../reference/configuration.md#account_domain), sous la forme `settings = { account_domain = "example.org" }`, si
les identifiants utilisent un autre domaine.

Gardez ce répertoire : il contient vos paramètres et, par défaut, l’état. L’état ne contient aucune valeur de secret mais il est le seul
enregistrement de ce qui a été créé : conservez-le dans un backend distant, comme un bucket `gcs` ou `s3`, si plus d’une personne applique.

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. Créer les secrets et ajouter leurs valeurs

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` récupère le module et ses scripts à cette version, sous le nom de votre bloc : `migrate.sh`, qu’une application
exécute, et `console.sh`, qui exécute [la console](#the-console).

Appliquez d’abord les secrets seuls, ce qui les crée et génère la KEK et la clé VAPID, avec le module du type de KEK :

| Module racine | Appliquer |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

Avec `kek = { kind = "kms" }`, ciblez `module.mycroblog.module.kek_kms` à la place du secret de la KEK.

**Copiez la KEK maintenant**, et gardez la copie à l’écart de la base de données, par exemple dans un gestionnaire de mots de passe : sans elle, une
[sauvegarde](backup.md) ne peut pas être restaurée et les clés des comptes sont perdues. `tofu output kek` donne la commande. Copiez de la même manière la clé
VAPID, `<name>-vapid-key` : sa perte met fin à tous les abonnements Web Push.

Exécutez ensuite chaque commande de `tofu output secrets_to_fill` avec sa valeur. Ajoutez-les toutes avant l’étape suivante.

<!-- source: 3-apply b9f715c7b55d -->
### 3. Appliquer

```sh
tofu apply
```

Cela crée le job de migration avec ce dont il dépend, y exécute `my upgrade` par `migrate.sh`, et l’attend :
`my config check`, `my db upgrade -check`, `my schema apply` et `my db upgrade`, qui s’arrête à la première qui échoue. Sa
sortie apparaît dans celle de l’application. La première commande qui utilise la base de données y enregistre `host` et `account_domain`. Ce n’est qu’alors
que l’application crée les services, dont la vérification de santé échoue tant que le schéma n’existe pas.

Si la migration échoue, l’application s’arrête avant les services. Corrigez la cause, souvent un secret sans valeur, et exécutez
`tofu apply` de nouveau : il exécute la migration de nouveau.

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. Pointer le domaine vers le serveur

`tofu output dns_records` liste les enregistrements à ajouter chez votre fournisseur DNS, et est vide lorsque le module les crée.

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. Créer le premier administrateur

La console n’a ni terminal ni entrée standard ; `my admin create` affiche donc un lien qui définit le mot de passe au lieu d’en
lire un :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Ouvrez le lien immédiatement et définissez le mot de passe. Il ne fonctionne qu’une fois, pendant une heure, et **reste dans les journaux de la plateforme jusque-là**,
où toute personne qui peut les lire peut l’utiliser. S’il expire, exécutez `my admin account password-reset-link @alice` par la
console. Activez ensuite l’authentification à deux facteurs.

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. Parcourir la liste de contrôle

Parcourez la [liste de contrôle de production](production-checklist.md), en commençant par l’[e-mail](production-checklist.md#email) : SPF,
DKIM et DMARC sur le domaine de `mail.from`.

<!-- source: the-console 1b3d6dfa9c3c -->
## La console

La console exécute une commande `my` avec l’image, les paramètres et les secrets du service. Elle n’existe que tant que
`console_enabled` est activé ; désactivez-le donc quand vous avez terminé :

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` attend la commande, affiche sa sortie, qu’il relit dans les journaux de la plateforme, et se termine avec son code de
sortie. Il utilise l’image que le service exécute : après une mise à niveau qui s’est arrêtée à la migration, c’est la version précédente, à laquelle
la base de données convient encore.

La commande n’a ni entrée standard ni terminal ; une commande qui y lit un mot de passe ou un fichier ne peut donc pas s’exécuter de cette
manière. Ce qu’elle affiche reste dans les journaux : n’exécutez donc pas une commande dont vous n’écririez pas la sortie à cet endroit.

<!-- source: upgrading 4be8a01734ce -->
## Mettre à niveau

Lisez d’abord [Mettre à niveau](database.md#upgrading) et les notes de version, et avancez d’une version mineure à la fois. Une mise à niveau est une
application de la nouvelle version :

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Lisez le plan : le module d’une nouvelle version peut aussi changer des ressources. L’application fait ce qui suit et s’arrête à la première étape qui échoue :

1. Donne la nouvelle image aux jobs de migration et de console (définitions de tâches sur AWS), avec le reste du plan dont les services ne dépendent pas.
2. Exécute `my upgrade` dans le job de migration par `migrate.sh`, depuis l’endroit où vous appliquez, et l’attend, comme lors du
   [premier déploiement](#3-apply). Il attend d’abord toute exécution qu’une application interrompue a laissée en cours.
3. Fait passer chaque worker à la nouvelle image, puis le service. Une nouvelle révision ne reçoit du trafic qu’une fois sa vérification de santé réussie.

**L’endroit où vous appliquez** a besoin de `jq`, de la CLI de la plateforme et d’identifiants autorisés à exécuter le job et à lire ses journaux, que la
page de la plateforme liste. Il en va de même pour un job de CI qui applique.

**Lorsqu’une étape échoue**, les services restent sur leurs révisions, et les notes de version ou la sortie de la migration en donnent la raison. Corrigez
la cause et exécutez `tofu apply` de nouveau avec la même version : il exécute la migration de nouveau, et les étapes terminées n’ont
plus rien à faire. Une application de la même image n’exécute rien. Pour revenir en arrière après un changement de schéma, restaurez la sauvegarde : voir
[Revenir en arrière](database.md#rolling-back). Une application d’une version antérieure s’arrête à la migration, puisque cette version refuse
un schéma plus récent, et ne change aucun service.

Pour essayer une version candidate, ou pour garder l’image d’une version antérieure sous un module plus récent, définissez vous-même la version, le
tag de l’image sans le `v` de la version :

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### Vérifier d’abord le SQL

Le plan ne montre pas le SQL de la migration. Pour le lire avant l’application, appliquez seul le job de migration avec la nouvelle
image, exécutez-y `my upgrade -dry-run`, qui ne change rien, et lisez ses journaux. La page de la plateforme donne les commandes.

<!-- source: destructive-changes 0bea88d60224 -->
### Changements destructeurs

Lorsque les notes de version demandent `-allow-destructive`, nommez cette version dans le bloc :

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` ne reçoit `-allow-destructive` que lors de l’application de cette version ; une valeur laissée en place ne permet donc à aucune version ultérieure
de supprimer des données.

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### Épingler l’image par son condensat

La [liste de contrôle de production](production-checklist.md#operations) recommande un condensat, tiré des notes de version :

```hcl
  image = { digest = "sha256:..." }
```

Un condensat nomme une seule image, quel que soit le tag qui l’accompagne ; changez-le donc dans la même modification que `ref` : un condensat resté de la
version précédente garde l’image de cette version sous la nouvelle.

<!-- source: the-maintenance-mode 57bd5487b045 -->
### Le mode maintenance

Le [mode maintenance](maintenance.md) demande trois applications, puisque les services changent avant la mise à niveau et après elle. Ajoutez-le
aux paramètres du bloc et appliquez, faites la sauvegarde, augmentez `ref` et appliquez, puis retirez-le et appliquez :

```hcl
  settings = { maintenance = "true" }
```

Sans lui, l’ancienne révision continue de servir pendant la migration, ce qui
[n’est pas garanti de fonctionner](database.md#the-server-during-an-upgrade).

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## Répartir les jobs en workers

Lorsqu’un type de job retient les autres, donnez-lui son propre service. Décidez de ce qu’il faut répartir d’après les métriques, comme le dit
[Répartir les workers par type de job](running.md#splitting-workers-by-job-type). `workers` accepte une table indexée par une clé qui nomme chaque
worker, avec ses `job_types`, et le module en tire chaque file, route et paramètre de la répartition ; vous n’en écrivez donc aucun.
Pour éviter que les livraisons retardent le reste :

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

Pour garder rapides les e-mails et les notifications :

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
    }
  }
```

Le plan refuse une répartition des types qui doivent rester ensemble. Omettez les types des paramètres que vous n’avez pas définis, comme le dit cette
page : un type que le serveur n’a pas enregistré empêche le démarrage, et le plan ne peut pas voir lesquels sont concernés. Le service par défaut
exécute chaque type qu’aucun worker ne nomme, et aucun de ceux qu’un worker nomme.

Un worker accepte aussi la taille de son service, chacune valant celle du service par défaut lorsqu’elle n’est pas nommée :
[la référence des modules](../../deploy/opentofu/README.md#workers) liste les champs de chaque plateforme. Un worker ne reçoit aucun
trafic de l’extérieur, et une mise à niveau le fait passer après la migration et avant le service par défaut. Chaque worker peut apporter ce que
coûte un service, et les connexions de ses instances comptent dans la limite de la base de données.

<!-- source: changing-a-backend df886f733f8e -->
## Changer de backend

Changez le `kind` de sa section. Chaque kind définit ce dont il a besoin sur le serveur, et le plan dit ce qui change. Changer la
base de données signifie aussi déplacer ses données, puisqu’une nouvelle base est vide : faites une [sauvegarde](backup.md) de l’ancienne et restaurez-la dans
la nouvelle pendant que rien n’écrit.

<!-- source: the-kek 62356f591998 -->
### La KEK

Changer `kek.kind` tient en une ligne, mais sur une instance qui contient des données, ce n’est pas gratuit : la base de données contient des secrets enveloppés avec la
clé actuelle, dont les clés de chaque compte ; une instance qui la perd ne peut donc plus les lire. Suivez
[Passer de la KEK locale à un KMS et inversement](key-management.md#move-between-the-local-kek-and-a-kms). Du secret vers un KMS, gardez
la KEK actuelle comme `kek_previous` pendant que les secrets migrent : ajoutez `secrets = ["kek_previous"]`, et remplissez le secret
`<name>-kek-previous` avec la KEK actuelle. Exécutez `my kek rewrap` par [la console](#the-console), puis retirez-la lorsque
`my kek status` ne compte plus de clés `local:`. Gardez la KEK aussi longtemps que vous gardez des sauvegardes de la base de données antérieures au rewrap.

Sur une nouvelle instance sans données, rien n’a besoin d’être déplacé.
