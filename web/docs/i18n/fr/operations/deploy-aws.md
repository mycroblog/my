<!-- source: deploying-on-aws d7e8643cd93d -->
# Déployer sur AWS

Comment exécuter `my` sur AWS avec le module OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) : ECS sur Fargate derrière
un Application Load Balancer, les jobs dans la base de données, S3, SNS et l’e-mail par Amazon SES. Rien ne descend à zéro : comptez
environ 42 $ par mois au repos avant la base de données, et lisez [Coûts](#costs).

**Rien de tout cela n’a jamais été appliqué sur AWS.** Les vérifications le planifient sur des fournisseurs simulés et sans identifiants, ce qui montre que
la configuration est cohérente, pas qu’AWS l’accepte ni que le serveur démarre. Considérez un premier déploiement comme un test.

Ce que tout déploiement OpenTofu partage, c’est-à-dire les entrées du module, ses secrets, la console et les mises à niveau, se trouve dans
[Déployer avec OpenTofu](deploy-opentofu.md). Cette page ajoute ce dont AWS a besoin.

<!-- source: what-it-creates 1ca642b05ab0 -->
## Ce qu’il crée

Un seul bloc de `mycroblog-aws` crée tout cela. Chaque ligne est une section du module, et sa valeur par défaut est la configuration
recommandée ; [la référence du module](../../deploy/opentofu/README.md#sections-of-mycroblog-aws) liste chaque champ.

| Élément | Section | Par défaut | Autres kinds |
| --- | --- | --- | --- |
| Serveur | `service`, `jobs` | Un cluster et un service ECS sur Fargate avec `role = all`, des définitions de tâches pour la migration et, tant que vous en avez besoin, la console, et un groupe de journaux `/ecs/<name>` | |
| Réseau | `network` | Un VPC sur deux zones ou plus, avec un sous-réseau public et un privé dans chacune | Un VPC à vous : [Réseaux](#networks) |
| Trafic | `ingress` | Un Application Load Balancer, toujours présent, avec votre certificat | CloudFront ou Cloudflare devant lui : [Comment le trafic atteint le service](#how-traffic-reaches-the-service) |
| Jobs | | Aucun : les tâches restent actives et prennent les jobs dans la base de données | |
| Base de données | `database` | Un secret pour l’URL d’un PostgreSQL que vous exécutez, comme Neon | Amazon RDS : [Choisir la base de données](#2-choose-the-database) |
| Médias | `media` | Un bucket S3 privé | Cloudflare R2, ou tout stockage compatible S3 |
| Caches et limites de débit | `kv` | Aucun : la base de données les contient | ElastiCache Serverless pour Valkey, ou un Valkey que vous exécutez |
| Mises à jour en temps réel | `stream` | Un topic SNS. Le serveur crée et supprime ses propres files SQS | La base de données, ou le Valkey de `kv` |
| E-mail | `mail` | L’identité SES du domaine de `mail.from` : [E-mail par Amazon SES](#email-through-amazon-ses) | Tout service SMTP |
| Secrets | | Secrets Manager : une clé VAPID générée, et les secrets que vous seul pouvez remplir, vides | |
| KEK | `kek` | Un secret généré dans Secrets Manager | Une clé dans AWS KMS, qui ne le quitte jamais |
| Métriques, traces, erreurs | `observability` | Métriques et rapports d’erreurs dans CloudWatch, traces vers X-Ray par un collector dans chaque tâche : [Traces vers X-Ray](#traces-to-x-ray) | Vos propres exporteurs, par `settings` |
| Alertes | `monitoring` | Un canary CloudWatch Synthetics et des alarmes : [Supervision et alertes](#monitoring-and-alerts) | Aucune, avec `null` |

`translation = {}` ajoute Amazon Translate, `search = { domain_name = "..." }` un domaine Amazon OpenSearch Service que vous
avez créé, et `billing = {}` l’importation du coût d’utilisation depuis Cost Explorer : [Traduire des publications](translation.md#amazon-translate),
[Moteurs de recherche](search-engines.md) et [Importer les coûts d’utilisation du cloud](billing-import.md).

<!-- source: before-you-start 9d5614a650e1 -->
## Avant de commencer

- Ce dont [le premier déploiement](deploy-opentofu.md#first-deployment) a besoin, avec `aws` comme CLI de la plateforme.
- Un compte AWS, utilisé pour cette seule instance, et des identifiants autorisés à créer tout ce qui précède, comme
  `AdministratorAccess`.
- Une région. SES envoie depuis elle, et le module la tire du fournisseur `aws`.
- Recommandé : une zone hébergée publique de `host` dans Route 53, où le module crée alors l’enregistrement de `host`, y compris à
  l’apex d’une zone.

<!-- source: first-deployment 26b562a3db03 -->
## Premier déploiement

Faites ici les trois premières étapes, puis suivez [le premier déploiement](deploy-opentofu.md#first-deployment) avec les notes de la
quatrième.

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. Demander les certificats

Le répartiteur de charge a besoin d’un certificat validé pour `host` dans AWS Certificate Manager, dans la région. Le module le prend sous forme d’
ARN, car le valider pendant l’application la retiendrait jusqu’à ce que vous ajoutiez des enregistrements que seule l’application affiche.

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

Avec `ingress.kind = cloudfront`, demandez-en un second dans `us-east-1`, la seule région dont CloudFront accepte un certificat ; dans `us-east-1`,
un seul certificat sert aux deux. Avec `ingress.media_host`, le certificat doit couvrir aussi cet hôte.

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. Choisir la base de données

`database.kind` la choisit. Dans les deux cas, les tâches interrogent la base de données pour les jobs environ une fois par seconde ; elle ne se repose donc jamais.

**`external`, la valeur par défaut et la recommandée**, lit l’URL d’un PostgreSQL que vous exécutez, comme [Neon](https://neon.com),
dans un secret que vous remplissez. Le module ne crée jamais la base de données ; son mot de passe reste donc hors de l’état OpenTofu. Sur Neon,
créez un projet dans une région proche de la vôtre et copiez ses chaînes de connexion :

- La chaîne **directe** (l’hôte sans `-pooler`) est obligatoire, dans le secret `<name>-database-url`. La migration et la
  console l’utilisent toujours : `my schema apply` prend un verrou de niveau session, qu’un pooler en mode transaction ne conserve pas.
- La chaîne **poolée** (l’hôte avec `-pooler`) est facultative, pour le service : définissez `database = { kind = "external", pooled = true }`,
  et le module crée le secret `<name>-database-url-pooled`. Voir
  [Par un pooler de connexions](database.md#through-a-connection-pooler).

Comptez sur l’offre Launch de Neon : le calcul ne s’arrêtant jamais, les heures de calcul de l’offre Free sont épuisées en un mois.

**`rds`** crée Amazon RDS pour PostgreSQL dans les sous-réseaux privés, avec un mot de passe que le module génère et écrit lui-même dans
`<name>-database-url` ; il n’y a donc rien à remplir :

```hcl
  database = { kind = "rds" }
```

C’est par défaut du `db.t4g.micro` avec 20 Go (`instance_class`, `allocated_storage_gb`), avec 7 jours de sauvegardes automatiques et la
protection contre la suppression. Elle n’a pas d’adresse publique ; seules les tâches l’atteignent : un `pg_dump` doit s’exécuter depuis l’intérieur du VPC, et
avant une mise à niveau, un instantané en tient lieu,
`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`. Elle n’a pas d’URL poolée.

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. Écrire le bloc

`examples/aws-ecs-neon` est une façon de commencer : le fournisseur `aws` et un bloc avec les valeurs par défaut, dont les variables sont listées dans
`terraform.tfvars.example`. Quel que soit votre point de départ, le bloc a besoin de ceci :

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` n’a pas de valeur par défaut, car les deux sorties sont facturées à l’heure : lisez [Réseaux](#networks).
- `ingress.route53_zone_id` nomme la zone hébergée de `host`, si vous en avez une.
- `media.bucket_name` doit être unique dans tout AWS.
- `monitoring` nomme qui est notifié par les alarmes, ou vaut `null`.

Ajoutez la section de la base de données que vous avez choisie, et toute autre que vous changez.

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. Déployer

Suivez [le premier déploiement](deploy-opentofu.md#first-deployment). Sur AWS, là où les commandes de cette page utilisent le `name`
par défaut, `my` :

- **Secrets** : ils sont dans Secrets Manager, nommés `<name>-...`. `tofu output kek` donne la commande qui copie la KEK, et
  `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` copie la clé VAPID. La
  KEK parvient au serveur sous la forme `MY_KEK`, puisque Fargate ne monte aucun fichier. Une tâche lit ses secrets à son démarrage : après en avoir changé
  un, exécutez `aws ecs update-service --cluster my --service my --force-new-deployment`, et la même chose pour chaque worker,
  `my-<key>`.
- **Application** : la définition de tâche de la migration vient d’abord, avec ce dont elle dépend, et les services seulement une fois la migration
  réussie. La vérification du répartiteur de charge interroge `/readyz`, qui répond `503` jusqu’à ce que le schéma existe.
- **DNS** : `tofu output dns_records` liste l’enregistrement de `host`, sauf si Route 53 ou Cloudflare le crée, ainsi que les trois enregistrements DKIM
  de SES. À l’apex d’une zone sans Route 53, l’enregistrement de `host` a besoin de ce que votre fournisseur DNS appelle un `ALIAS` ou un
  `ANAME`, comme le dit `tofu output dns_note`.
- **E-mail** : SES n’envoie qu’aux adresses que vous avez vérifiées tant qu’AWS n’a pas accordé l’accès à la production ; personne d’autre ne peut donc encore
  s’inscrire : lisez [E-mail par Amazon SES](#email-through-amazon-ses).
- **Alertes** : chaque adresse de `monitoring.notification_emails` reçoit un e-mail d’abonnement de SNS, qu’elle doit confirmer.
- **Le premier administrateur** : le lien reste dans CloudWatch Logs, dans `/ecs/<name>`, où toute personne qui peut lire les journaux du compte
  peut l’utiliser.

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## La console et les mises à niveau sur AWS

[La console](deploy-opentofu.md#the-console) et les [mises à niveau](deploy-opentofu.md#upgrading) exécutent des tâches Fargate ponctuelles depuis l’endroit où
vous appliquez, par `aws`, et lisent leur sortie dans CloudWatch Logs. Une application interrompue laisse la tâche en cours, et l’application suivante
attend qu’elle se termine. Lors d’une mise à niveau, chaque définition de tâche reçoit une nouvelle révision avec l’image, que rien n’exécute jusqu’à ce qu’un
service la prenne ; chaque service attend alors le démarrage de ses nouvelles tâches, et pour le service par défaut la réussite de la vérification du
répartiteur de charge, avant que les anciennes partent, et le disjoncteur de déploiement ramène une révision dont les tâches ne démarrent pas à celle
d’avant.

Les identifiants de l’endroit où vous appliquez, ou du job de CI qui applique, ont besoin, en plus de ce dont l’application elle-même a besoin :

- pour la migration : `ecs:RunTask`, `ecs:DescribeTasks`, `ecs:ListTasks`, `logs:FilterLogEvents` et `iam:PassRole` sur les
  deux rôles des tâches ;
- pour la console, en plus : `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource` et
  `ecs:RegisterTaskDefinition`, puisque la console prend d’abord l’image du service en cours d’exécution.

Pour [vérifier d’abord le SQL](deploy-opentofu.md#checking-the-sql-first), activez la console tant que le bloc nomme encore la version
actuelle, puis augmentez `ref`, appliquez seule la définition de tâche de la migration, et exécutez avec elle `my upgrade -dry-run` :

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## Supervision et alertes

`monitoring` ajoute un canary CloudWatch Synthetics de `/readyz`, des métriques dans CloudWatch et des alarmes portant les noms de
[la procédure d’exploitation](alerts.md#aws-ecs), qui notifient les adresses de `notification_emails` ou les topics SNS de
`notification_topic_arns`. L’entrée n’a pas de valeur par défaut : nommez qui est notifié, ou définissez `monitoring = null` pour tout omettre.
Chaque exécution du canary est facturée, et `uptime_period_minutes`, 15 par défaut, est la fréquence de ses exécutions : lisez [Coûts](#costs).

<!-- source: choosing-the-backends b83753b1f860 -->
## Choisir les backends

Chaque section prend par défaut le kind recommandé ; [changez le `kind` d’une section](deploy-opentofu.md#changing-a-backend) pour
un autre. La base de données est [plus haut](#2-choose-the-database).

| Section | Recommandé | Autres kinds, et quand |
| --- | --- | --- |
| `media` | `s3`, un bucket dans la région, avec `versioning` désactivé | `r2` dans `cloudflare.account_id`, ou `s3-compatible` à `endpoint`, chacun avec une clé d’accès dans des secrets que vous remplissez. Pour partager un bucket, `existing_bucket = true` utilise le vôtre et `prefix` (`tenant-a/`) commence chaque nom d’objet ; avec `s3`, le rôle des tâches n’atteint alors que les objets qui s’y trouvent |
| `kv` | `db`, qui ne coûte rien de plus | `elasticache`, qui est facturé au repos et vit dans les sous-réseaux privés, ou `external`, une URL Valkey dans un secret que vous remplissez |
| `stream` | `sns`, qui atteint chaque tâche | `postgres`, par la connexion directe de la base de données, ou `valkey`, le Valkey de `kv` |
| `mail` | `ses`, sans mot de passe | `smtp`, avec `smtp.host`, `smtp.port` (587) et `smtp.username`, dont le mot de passe va dans un secret que vous remplissez |
| `kek` | `secrets-manager`, que vous copiez une fois | `kms`, une clé qui ne quitte jamais AWS KMS. Sur une instance qui contient des données, [la changer](deploy-opentofu.md#the-kek) est un déménagement |
| `observability` | CloudWatch et X-Ray | `traces = "none"`, ou `"settings"` pour tout signal que vous envoyez ailleurs, comme Sentry |

`architecture = "arm64"` exécute les tâches sur Graviton, qui coûte moins ; l’image publiée existe pour les deux.

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## E-mail par Amazon SES

`mail.kind = "ses"`, la valeur par défaut, envoie avec l’API SES dans la région du déploiement, depuis le domaine de `mail.from`. Le
module crée l’identité du domaine avec Easy DKIM et autorise les tâches à utiliser `ses:SendEmail` sur elle ; il n’y a donc pas de mot de passe, et
`mail.configuration_set` crée un jeu de configuration et l’applique à chaque message, ou avec
`existing_configuration_set = true` en nomme un que des instances partagent ; chaque instance n’agit alors que sur les notifications des
e-mails qu’elle a envoyés. Avant que SES envoie à vos utilisateurs, ajoutez les
enregistrements que liste `tofu output dns_records` et sortez du bac à sable :
[Envoyer des e-mails avec Amazon SES](email-ses.md) donne les étapes, et comment le serveur apprend les rebonds et les plaintes.

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## Traces vers X-Ray

`observability.traces = "x-ray"` est la valeur par défaut, car le collector convient à la configuration recommandée sans frais supplémentaires. C’est le
collector AWS Distro for OpenTelemetry, qui s’exécute à côté du serveur dans chaque tâche, n’écoute que sur la boucle locale de la tâche et
écrit dans X-Ray avec le rôle des tâches (`xray:PutTraceSegments`). Il prend 32 unités de CPU et 128 Mio sur chaque tâche, qu’AWS
facture selon la seule taille de la tâche ; la tâche du service par défaut de 256 unités de CPU et 1 024 Mio coûte donc la même chose et donne au
serveur 224 unités de CPU et 896 Mio. La valeur par défaut de `traces_sample_ratio`, 1 %, enregistre environ 30 000 traces par mois pour 100 000
requêtes par jour, en dessous des 100 000 traces par mois que X-Ray enregistre gratuitement.

Ce qui coûte vraiment, c’est l’image du collector, tirée à chaque démarrage d’une tâche, et la mémoire du serveur. Définissez `traces = "none"` lorsque
vous ne lisez pas les traces, ou pour une tâche trop petite pour épargner 128 Mio : le plan refuse une taille qui laisse au serveur moins de
128 unités de CPU ou 512 Mio, dans `service`, `jobs` ou un worker. Le collector n’est pas essentiel : s’il s’arrête, le serveur continue
sans traces. Que X-Ray prenne ses spans n’est pas vérifié.

<!-- source: networks 36dc33a6cbef -->
## Réseaux

ECS a toujours besoin d’un VPC. `network.kind = "vpc"`, le recommandé, en crée un dans `cidr_block` (`10.0.0.0/16`) sur
`availability_zones`, au moins deux, dont le répartiteur de charge et RDS ont besoin : un sous-réseau public et un privé dans chaque zone, et une
passerelle internet. Il est en IPv4 seulement ; le serveur n’atteint donc aucun pair uniquement IPv6. `network.kind = "existing"` prend à la place un VPC à
vous : `vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs` et `private_subnet_ids`.

Une tâche a besoin d’une sortie vers internet, pour l’image, la base de données, la fédération et les API AWS, et `network.egress` la choisit.
Le trafic entrant n’atteint une tâche que par le répartiteur de charge, dans les deux cas :

| | `public-ip` (recommandé) | `nat` |
| --- | --- | --- |
| Sous-réseaux des tâches | Publics, avec une adresse IPv4 publique sur chaque tâche | Privés |
| Chaque heure | 0,005 $ par adresse de tâche | Une passerelle NAT, environ 0,045 $, et son adresse |
| Chaque Go | Rien pour l’adresse | Environ 0,045 $ par la passerelle, médias et fédération compris |
| Avec `existing` | Rien de plus | Vos sous-réseaux privés doivent déjà atteindre internet |

Utilisez `public-ip` sauf si une de vos politiques interdit les sous-réseaux publics : il coûte bien moins et n’est pas plus ouvert. `vpc` crée une seule
passerelle NAT, dans la première zone : lorsque cette zone tombe, les sous-réseaux privés perdent internet jusqu’à son retour. RDS et
ElastiCache vivent dans les sous-réseaux privés dans les deux cas, et n’ont besoin d’aucune sortie.

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## Comment le trafic atteint le service

Le répartiteur de charge est toujours présent, avec le certificat de `ingress.certificate_arn`, et les tâches n’admettent que lui.
`ingress.kind` décide de ce qui se tient devant lui :

| | `load-balancer` (recommandé) | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` pointe vers | Le répartiteur de charge | Une distribution CloudFront | Cloudflare, qui se connecte au répartiteur de charge |
| En cache | Rien | `/media/`, selon le `Cache-Control` du serveur | Rien sur `host` ; `media_host` ajoute un hôte en cache |
| Enregistrements | Route 53 les crée avec `route53_zone_id`, ou `dns_records` les liste | Route 53 avec `route53_zone_id`, ou `dns_records` | Cloudflare le crée |
| Exige | | `cloudfront_certificate_arn`, dans `us-east-1`. `cloudfront_price_class` choisit les emplacements de périphérie | `cloudflare = { zone_id = "..." }`, et le jeton dans `CLOUDFLARE_API_TOKEN` |
| Ferme l’origine par | Le groupe de sécurité des tâches | Le groupe de sécurité du répartiteur de charge n’admet que CloudFront, et son listener refuse une requête sans un en-tête que CloudFront envoie | Un en-tête que la règle de Cloudflare ajoute, que le serveur vérifie |

Derrière CloudFront ou Cloudflare, la valeur de l’en-tête se retrouve dans l’état OpenTofu, qui doit rester privé. Un enregistrement du
répartiteur de charge antérieur à CloudFront doit disparaître d’abord : un enregistrement existant du même nom fait échouer l’application. Les
adresses de CloudFront viennent d’AWS au moment du plan ; appliquez donc de temps en temps, faute de quoi une périphérie sur une nouvelle adresse partage les limites de débit de ses visiteurs.

`ingress.media_host` ajoute un hôte de médias en cache sur Cloudflare, à côté de `load-balancer` ou `cloudflare`, avec
`cloudflare.zone_id`. Un fichier supprimé reste en cache jusqu’à ce que vous le purgiez : lisez [Servir les médias par un CDN](cdn.md). Avec Cloudflare
devant quoi que ce soit, désactivez Bot Fight Mode dans son tableau de bord, puisqu’un défi sur `/ap/`, `/.well-known/` ou `/api/` casse la
fédération, et que le module ne le gère pas. Les kinds Cloudflare n’ont été exécutés que sur des fournisseurs simulés.

<!-- source: limits 42ed50837851 -->
## Limites

- **Santé** : l’unique vérification du répartiteur de charge interroge `/readyz`, qui échoue aussi tant que la base de données est indisponible : après 2,5 minutes
  d’échecs, ECS remplace la tâche, et continue de le faire jusqu’au retour de la base de données. Il n’y a pas de vérification de conteneur ; un worker,
  qui n’a pas de répartiteur de charge, ne signale donc une mauvaise image que dans ses journaux.
- **Téléversements** : le répartiteur de charge ne fixe aucune limite au corps d’une requête ; ce sont donc les paramètres `media_max_*` du serveur qui décident.
- **Flux** : un flux envoie un battement de cœur toutes les 30 secondes, sous le délai d’inactivité du répartiteur de charge de 60. Il reste ouvert jusqu’à ce que
  sa tâche parte, et le client se reconnecte alors.
- **Adresses des clients** : les limites de débit reposent sur ce que le serveur fait confiance, ce que le module définit à partir de l’ingress. Vérifiez les adresses des clients
  dans les premiers journaux de requêtes : une adresse des sous-réseaux du répartiteur de charge, ou de CloudFront, signifie qu’elle est fausse.
- **Le disque** disparaît avec la tâche ; le plan refuse donc SQLite et les médias sur le système de fichiers.
- **Les secrets** sous une clé KMS gérée par le client ont aussi besoin de `kms:Decrypt`, que le module n’accorde pas.

<!-- source: costs 19e0e43bc201 -->
## Coûts

Rien ne descend à zéro sur ECS : la tâche, le répartiteur de charge et ses adresses publiques sont facturés tout le mois. Ces chiffres viennent de
la liste de prix d’AWS pour Tokyo, d’octobre et novembre 2026, sauf si une ligne dit autre chose, et n’ont pas été comparés à une facture.

| Élément | Par mois | Choix |
| --- | --- | --- |
| Une tâche de 0,25 vCPU et 1 Go | 13,26 $ sur x86, 10,61 $ sur arm64 | `service`, `architecture`. Chaque worker ajoute ses propres tâches |
| Répartiteur de charge | 17,74 $, plus 0 à 5,84 $ pour chaque unité de capacité utilisée tout le mois | Toujours présent |
| Les adresses publiques du répartiteur de charge | 7,30 $, pour deux zones | Une par zone |
| La sortie | 3,65 $ par tâche avec `public-ip` ; environ 36,50 $ et 0,045 $ par Go avec `nat`, au tarif de l’Ohio | `network.egress` |
| Base de données | Environ 19 $ sur l’offre Launch de Neon à 0,25 CU, puisque le calcul ne s’arrête jamais ; environ 21 $ avec `rds`, `db.t4g.micro` et 20 Go | `database` |
| Caches et limites de débit | 0 $ avec `db` ; avec `elasticache`, environ 6,13 $, ou environ 61 $ selon un autre document d’AWS, en Virginie du Nord | `kv` |
| Le canary | Environ 3,34 $ toutes les 15 minutes ; environ 51,72 $ toutes les minutes | `monitoring.uptime_period_minutes` |
| Traces | 0 $ jusqu’à 100 000 traces par mois | `observability.traces` |

**Au repos, environ 42 $ avec `public-ip` et environ 75 $ avec `nat`**, plus les unités de capacité, avant la base de données, le stockage, le trafic
et le canary. Non lus, donc dans aucune ligne : CloudFront, Secrets Manager, CloudWatch Logs, S3, SNS et SQS, les données sortant d’AWS, et tout
niveau gratuit. Les tâches ponctuelles coûtent leur taille pour les minutes où elles s’exécutent, et une mise à niveau exécute une seconde tâche de service pendant quelques minutes.
