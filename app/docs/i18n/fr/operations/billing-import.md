<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# Importer les coûts d’utilisation du cloud

Toutes les 24 heures, le serveur importe les frais d’utilisation des prestataires listés dans le paramètre [`billing_sources`](../reference/configuration.md#billing_sources), ou d'[un plugin](#a-plugin-billing_sources--plugin) pour un prestataire dont il n’a pas d’adaptateur, et les ajoute aux coûts de l’instance. La valeur par défaut est vide, ce qui n’importe rien.
Il importe les frais du mois de facturation en cours et du précédent (jusqu’au 15 du mois, ou jusqu’à ce que le mois précédent soit définitif), à raison d’une ligne par service et par type de frais (utilisation, crédit, remboursement, taxe, support, frais, autre).
Tant que le prestataire n’a pas clos un mois, les valeurs sont enregistrées comme des estimations ; après la clôture, comme définitives ; et quand une valeur change après être devenue définitive, comme une correction. Les enregistrements du passé ne sont jamais réécrits.
L’état des imports (le dernier succès, le type du dernier échec et le nombre de requêtes de ce mois) est affiché sur la page des coûts de l’administration et par `GET /api/v1/admin/billing_imports`.
Les métriques comptent chaque tentative par source, résultat et type d’échec (`my.billing.imports`) et signalent les sources dont la tentative la plus récente a échoué (`my.billing.import.failing`) ;
[Alertes et runbook](alerts.md#mybillingimportfailing) contient l’alerte correspondante.

Aucun identifiant n’est conservé dans la configuration de ce serveur. Donnez à l’environnement d’exécution ce que lit la chaîne d’identifiants par défaut du SDK de chaque prestataire (pour AWS, le rôle d’exécution, les variables d’environnement ou les fichiers d’identifiants partagés ; pour Google Cloud, les Application Default Credentials).

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## Avec les modules OpenTofu

Les modules [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) et [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) définissent `billing_sources` et réalisent les parties de ce guide que permet l’API d’un prestataire. `billing-gcp` ne fonctionne qu’avec le module d’application de Google Cloud : sur AWS, les modules n’importent donc que les coûts d’AWS. `billing-aws` fonctionne avec les modules d’application d’AWS et avec celui de Cloud Run, où les deux peuvent figurer dans les `backends`. Aucun des deux n’a encore été appliqué sur un vrai compte.

| Étape de ce guide | Avec les modules |
| --- | --- |
| La politique IAM, et le rôle de « Calling from Cloud Run with web identity » | `billing-aws` |
| Activer Cost Explorer, Linked Account Access, les tags de répartition des coûts | À la main, dans la console AWS |
| Le dataset de l’export, et les rôles de « IAM roles » | `billing-gcp` |
| Activer l’export (« Standard usage cost ») | À la main, dans la console Google Cloud, avec un rôle sur le compte de facturation |
| Le topic et l’abonnement pull d’un budget, et le rôle d’abonné | `billing-gcp` avec `method = "budget"` |
| Créer le budget et le connecter au topic | À la main, dans la console Google Cloud, avec un rôle sur le compte de facturation |
| Activer l’API BigQuery ou Pub/Sub | À la main, comme dans [Déployer sur Cloud Run](deploy-cloud-run.md#1-enable-the-apis) |
| Un plugin | Pas par ces modules. Sans module de facturation dans les `backends`, définissez `MY_BILLING_SOURCES = plugin` et les paramètres du [plugin](#a-plugin-billing_sources--plugin) dans l'`env` du module d’application ; à côté d’un module de facturation, le module d’application refuse `MY_BILLING_SOURCES` dans `env` : ajoutez donc un backend de votre cru qui renvoie `billing_sources = ["plugin"]` ([les champs d’un backend](../../deploy/opentofu/README.md)) |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS (`billing_sources = aws`)

Le serveur appelle `GetCostAndUsage` d’AWS Cost Explorer une fois par mois de facturation (un mois calendaire UTC), groupé par service (`SERVICE`) et par type d’enregistrement (`RECORD_TYPE`). Le point de terminaison est toujours `us-east-1`.
Les montants sont en USD. Tant que Cost Explorer marque un mois comme estimé (`Estimated`), le serveur enregistre le montant jusqu’à la veille comme une estimation, et dès qu’il ne le fait plus, comme définitif.
Un binaire construit avec le build tag `no_billing_aws` refuse `aws` (voir [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits a555f91984c3 -->
### Coût et limites

L’API Cost Explorer facture 0,01 $ par requête paginée. Un petit compte a besoin d’une requête d’une page par jour, soit environ 0,31 $ par mois.
Pour éviter les frais inattendus, le serveur compte les requêtes par jour UTC (`billing_aws_max_requests_per_day`, 4 par défaut) et par mois calendaire UTC (`billing_aws_max_requests_per_month`, 120 par défaut = 1,20 $), et s’arrête sans appeler dès qu’une limite est atteinte.
Les imports planifiés, les nouvelles tentatives de tâches et « Importer maintenant » dans l’administration comptent tous. Un import qui atteint une limite est enregistré comme un échec de type `request_cap`, et l’import du lendemain (pour la limite quotidienne) ou du premier jour du mois suivant (pour la limite mensuelle) réessaie.
Avant d’appeler, un import met de côté les requêtes que les limites laissent, de sorte que des imports exécutés en même temps ne puissent pas dépasser ensemble les limites.
Les imports d’un même prestataire s’exécutent un à la fois : un import qui démarre alors qu’un autre n’est pas terminé réessaie une minute plus tard, et une tâche qui a déjà importé avec succès n’appelle rien quand il s’exécute de nouveau.
Un import qui s’est arrêté avant d’avoir fini (le processus s’est terminé) compte les requêtes qu’il avait mises de côté, et l’import suivant l’attend pendant 15 minutes au plus.

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### Activer Cost Explorer

1. Ouvrez Cost Explorer dans la console Billing and Cost Management et activez-le. Il ne peut pas être désactivé une fois activé. Il faut jusqu’à 24 heures pour que les données soient prêtes.
2. Un compte où il n’est pas activé renvoie `DataUnavailableException`, et l’import échoue avec `not_enabled`.

<!-- source: iam-permissions 9f1c23f3cb5c -->
### Permissions IAM

La seule permission nécessaire est `ce:GetCostAndUsage` (ressource `*`).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

Comment transmettre les identifiants dans chaque environnement d’exécution :

| Environnement d’exécution | Identifiants |
| --- | --- |
| AWS ECS | Attachez cette politique au rôle de la tâche. Aucune configuration n’est nécessaire |
| Google Cloud (Cloud Run) | Créez le rôle IAM de « Calling from Cloud Run with web identity » ci-dessous et définissez son ARN dans `billing_aws_role_arn`. Aucune clé AWS n’est nécessaire |
| Votre propre serveur | Transmettez la clé d’accès d’un utilisateur IAM ayant cette politique dans `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (ou un fichier d’identifiants partagé et `AWS_PROFILE`). Transmettez-la depuis un magasin de secrets |

Des permissions manquantes ou des identifiants invalides échouent avec `permission`.

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### Appeler depuis Cloud Run avec une identité web

Quand `billing_aws_role_arn` est défini, le serveur obtient auprès du serveur de métadonnées un jeton d’identité Google du compte de service de Cloud Run (l’audience est l’ARN du rôle) et l’échange contre des identifiants temporaires de ce rôle avec `AssumeRoleWithWebIdentity`.

1. Dans Google Cloud, notez l’ID unique (le `uniqueId` numérique) du compte de service du service Cloud Run : `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`.
2. Dans AWS IAM, créez un rôle avec la politique des permissions ci-dessus, et faites en sorte que sa politique d’approbation fasse confiance à `accounts.google.com`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` correspond au `azp` du jeton (l’ID du compte de service), et `accounts.google.com:oaud` à l'`aud` du jeton (l’audience que demande le serveur, qui est l’ARN du rôle).
3. Définissez `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`.

Cette configuration n’a pas encore été vérifiée sur un vrai compte.

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### Appeler depuis un compte lié d’Organizations

Le compte de gestion d’AWS Organizations peut voir les coûts de chaque compte membre de l’organisation. Pour appeler avec les identifiants d’un compte membre (lié),
activez « Linked Account Access » dans les préférences de Cost Explorer du compte de gestion. Quand il est désactivé, l’appel est refusé quelles que soient les permissions IAM, et l’import échoue avec `not_enabled` ou `permission`.

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### Choisir les coûts à importer (filtrage)

Choisissez avec `billing_aws_filter`.

| `billing_aws_filter` | Coûts importés | Paramètres supplémentaires |
| --- | --- | --- |
| `none` (par défaut) | Tous les coûts que les identifiants peuvent voir | Aucun |
| `linked_account` | Les coûts des comptes membres que vous nommez (`LINKED_ACCOUNT`) | `billing_aws_linked_accounts` (ID de compte à 12 chiffres, séparés par des virgules) |
| `tag` | Les coûts des ressources qui portent le tag de répartition des coûts que vous nommez | `billing_aws_tag` (`key=value`, par exemple `app=my`) |

La configuration la plus précise est un compte AWS dédié à ce serveur (il peut être un compte membre d’Organizations), importé avec `none` (en appelant avec les identifiants de ce compte) ou `linked_account` (en appelant depuis le compte de gestion).
Les taxes, les frais de support et certains crédits ne peuvent pas porter de tag, et peuvent sortir d’un filtre par tag (pas encore vérifié sur un vrai compte).

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### Activer un tag de répartition des coûts (`billing_aws_filter = tag`)

Un tag sur une ressource n’apparaît pas dans les données de facturation tant que vous ne l’avez pas activé comme tag de répartition des coûts.

1. Placez le même tag (par exemple `app=my`) sur les ressources de ce serveur (services ECS, RDS, buckets S3, équilibreurs de charge, etc.).
2. Dans « Cost allocation tags » de la console Billing and Cost Management, sélectionnez la clé du tag et activez-la. Il faut jusqu’à 24 heures pour qu’un tag apparaisse dans cette liste après que vous l’avez attaché, et jusqu’à 24 heures de plus après l’activation pour qu’il apparaisse dans les données de facturation.
3. Pour refléter les coûts de la période précédant l’activation, utilisez « Backfill » sur le même écran (l’API `StartCostAllocationTagBackfill`).
4. Définissez `MY_BILLING_AWS_FILTER=tag` et `MY_BILLING_AWS_TAG=app=my`.

<!-- source: metric 3e034dd351bd -->
### Métrique

`billing_aws_metric` vaut soit `NetUnblendedCost` (par défaut, plus proche de la facture), qui est après remises (Savings Plans, instances réservées, prix négociés), soit `UnblendedCost`, qui est avant elles.
Les valeurs de Cost Explorer et la facture peuvent différer à cause du moment des mises à jour et des arrondis. Quand elles diffèrent, la facture a raison.

<!-- source: kinds-of-failure 2a6f9042f460 -->
### Types d’échec

| Type | Réponse d’AWS | Comment corriger |
| --- | --- | --- |
| `permission` | `AccessDeniedException`, identifiants invalides | Vérifiez les permissions IAM et la manière dont les identifiants sont transmis |
| `not_enabled` | `DataUnavailableException` | Activez Cost Explorer. Pour un compte lié, activez Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer n’a plus les données de facturation de la période à importer |
| `throttled` | `LimitExceededException` | La tâche réessaie après une pause (cela compte dans la limite de requêtes) |
| `unavailable` | 5xx, défaillance réseau | La tâche réessaie après une pause (cela compte dans la limite de requêtes) |
| `invalid_response` | Une réponse illisible, `ValidationException`, et similaires | Vérifiez les paramètres (tag, ID de compte) |
| `request_cap` | (aucun appel n’est effectué) | Attendez que le jour ou le mois limitant passe, ou augmentez la limite |

Une nouvelle tentative ne peut pas corriger un échec autre que `throttled` et `unavailable` : le serveur l’enregistre donc et s’arrête, et l’import planifié du lendemain réessaie.

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud (`billing_sources = gcp`)

Google Cloud n’a aucune API qui renvoie les coûts réels : le serveur agrège donc l’export Cloud Billing vers BigQuery (coût d’utilisation standard) une fois par jour avec un `jobs.query` de BigQuery (`billing_gcp_method = bigquery`, la valeur par défaut).
Quand vous ne pouvez pas configurer l’export, il existe aussi une méthode simple qui n’importe qu’un total approximatif à partir des notifications Pub/Sub d’un budget (voir [Importer à partir des notifications de budget](#importing-from-budget-notifications-billing_gcp_method--budget)).
Le mois de facturation est l'`invoice.month` de l’export, un mois calendaire à l’heure du Pacifique américain (avec heure d’été). Les dates sont enregistrées à l’heure du Pacifique telles quelles.
Les montants sont dans la devise du compte de facturation, sommés en `NUMERIC` par mois de facturation, service (`service.description`) et type de frais (`cost_type`), et les crédits (`credits[]`) deviennent une ligne de crédit par service. Les lignes de correction de l’export qui annulent des lignes antérieures font partie de la somme.
L’export n’a aucune marque de finalité : l’import qui s’exécute à partir du 5 (heure du Pacifique) du mois suivant traite donc le mois comme définitif, et jusque-là l’enregistre comme une estimation du premier jour du mois au jour de la dernière utilisation. Une fois définitif, l’import relit encore le mois jusqu’au 15 et enregistre toute différence comme une correction.
Un binaire construit avec le build tag `no_billing_gcp` refuse `gcp` (voir [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### Coût et limites

Un import est une requête, et BigQuery facture les octets que chaque requête analyse (au moins 10 Mo). Un petit compte analyse environ 0,3 Go par mois, ce qui tient dans le premier 1 TiB par mois de l’offre gratuite (partagé avec les autres usages du compte de facturation) : cela coûte donc 0 $.
Pour éviter les analyses inattendues, chaque requête porte `maximumBytesBilled` (`billing_gcp_max_bytes_billed`, 1 GiB par défaut). Une requête qui le dépasserait n’est pas facturée et échoue, et elle est enregistrée comme un échec de type `bytes_cap`. L’administration affiche les octets facturés.

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### Activer l’export vers BigQuery

L’opérateur configure cela une fois, dans la console (aucun moyen de le faire par une API n’a été trouvé).

1. Dans un projet lié au compte de facturation, créez un dataset BigQuery (par exemple `billing_export`). Choisissez un emplacement multirégion (`US` ou `EU`). Les données du mois en cours et du précédent sont complétées rétroactivement (avec une seule région, les données commencent le jour où vous activez l’export). L’emplacement ne peut pas être changé ensuite.
   Ne définissez aucune expiration sur les tables. Les lignes qui expirent ne reviennent pas.
2. Sous « Billing > Billing export > BigQuery export », activez « Standard usage cost » et sélectionnez le dataset de l’étape 1. La configuration exige « Billing Account Costs Manager » ou « Billing Account Administrator » sur le compte de facturation et le rôle BigQuery User sur le projet.
3. Au bout de quelques heures à un jour, la table `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` est créée (les tirets de l’ID deviennent `_`).
4. Définissez `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`.

Quand le dataset ou la table est introuvable, l’import échoue avec `not_found`.

<!-- source: iam-roles f5628acd0d94 -->
### Rôles IAM

Accordez ces deux rôles au compte de service du serveur qui importe.

| Rôle | Où l’accorder | Usage |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | Le projet qui exécute la requête (`billing_gcp_query_project`, ou le projet de la table si vide) | `bigquery.jobs.create`. Les octets analysés sont facturés à ce projet |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | Le dataset de l’export | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

Quand l’API BigQuery n’est pas activée dans le projet de la requête, l’import échoue avec `not_enabled`, et quand un rôle manque, avec `permission`.

L’authentification se fait par les Application Default Credentials (ADC).

| Environnement d’exécution | Identifiants |
| --- | --- |
| Google Cloud (Cloud Run) | ADC avec le compte de service du service. Aucune clé et aucune configuration |
| AWS ECS | Placez un fichier de configuration d’identifiants de Workload Identity Federation (le type AWS) et transmettez son chemin dans `GOOGLE_APPLICATION_CREDENTIALS` (pas encore vérifié sur un vrai compte) |
| Votre propre serveur | Workload Identity Federation, ou une clé JSON de compte de service transmise dans `GOOGLE_APPLICATION_CREDENTIALS`. Transmettez une clé depuis un magasin de secrets |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### Choisir les coûts à importer (filtrage)

Par défaut, le serveur importe tous les coûts du compte de facturation. La configuration la plus précise est un compte de facturation dédié à ce serveur, importé sans filtre.

| Paramètre | Coûts importés |
| --- | --- |
| `billing_gcp_projects` (ID de projets, séparés par des virgules) | Les coûts des projets que vous nommez (`project.id`) |
| `billing_gcp_label` (`key=value`, par exemple `app=my`) | Les coûts des ressources qui portent le label que vous nommez (`labels`) |

Quand vous définissez les deux, le serveur n’importe que les coûts qui correspondent aux deux.
Les coûts au niveau de la facture, comme la taxe, peuvent ne porter ni projet ni label, et sortent alors du filtre (pas encore vérifié sur un vrai compte).

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### Types d’échec

| Type | Réponse de BigQuery | Comment corriger |
| --- | --- | --- |
| `permission` | 401, 403 (`accessDenied`) | Vérifiez les rôles du compte de service et la manière dont les identifiants sont transmis |
| `not_enabled` | 403 (`accessNotConfigured` et similaires) | Activez l’API BigQuery dans le projet de la requête |
| `not_found` | 404 | Vérifiez `billing_gcp_table` et la configuration de l’export |
| `bytes_cap` | 400 (`bytesBilledLimitExceeded`) | L’analyse a dépassé `billing_gcp_max_bytes_billed`. Augmentez la limite si la table est grande |
| `throttled` | 429, 403 (`rateLimitExceeded`, `quotaExceeded`) | La tâche réessaie après une pause |
| `unavailable` | 5xx, défaillance réseau, une requête qui ne se termine pas | La tâche réessaie après une pause |
| `invalid_response` | Une réponse illisible, des lignes qui mélangent des devises au sein d’un mois, tout autre 400 | Vérifiez la table d’export (est-ce bien le coût d’utilisation standard ?) |

Une nouvelle tentative ne peut pas corriger un échec autre que `throttled` et `unavailable` : le serveur l’enregistre donc et s’arrête, et l’import planifié du lendemain réessaie.

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### Importer à partir des notifications de budget (`billing_gcp_method = budget`)

C’est une méthode simple pour quand vous ne pouvez pas configurer l’export BigQuery. Une fois par jour, le serveur lit, depuis un abonnement pull, les notifications qu’un budget Cloud Billing envoie vers un topic Pub/Sub.
Elle utilise le pull plutôt que le push : une notification ne réveille donc jamais le serveur (Cloud Run, par exemple).

Cette méthode n’importe **qu’un total approximatif fondé sur des estimations**.

- Le `costAmount` d’une notification est le coût cumulé estimé de la période du budget, sans ventilation par service ni par type de frais. Le serveur n’importe qu’une ligne par mois et par budget (le nom du service est le nom d’affichage du budget, le type de frais est l’utilisation, et la catégorie est autre). Les crédits et les taxes sont dans le total selon la configuration du budget.
- Comme avec BigQuery, un mois devient définitif dans l’import qui s’exécute à partir du 5 (heure du Pacifique) du mois suivant. Une notification de budget ne sait rien de la clôture de la facture : même une valeur traitée comme définitive peut donc différer du montant de la facture.
- Un mois estimé est enregistré comme le coût du premier jour du mois au jour (heure du Pacifique) où la dernière notification a été publiée.
- Le filtrage est décidé par le périmètre du budget (projets, services, une clé et une valeur de label), et `billing_gcp_projects` et `billing_gcp_label` ne sont pas utilisés.

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### Préparer le budget, le topic et l’abonnement

1. Créez un topic et un abonnement pull que seul ce serveur lit. Voyez « How notifications are read » ci-dessous pour les raisons de la durée de rétention et de l’expiration.

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. Dans la console, sous « Billing > Budgets & alerts », créez un budget. Rendez sa période **mensuelle** (la notification d’un budget qui n’est pas mensuel échoue avec `invalid_response`).
   Sélectionnez le projet de ce serveur comme périmètre, et sous « Manage notifications > Connect a Pub/Sub topic to this budget » sélectionnez le topic de l’étape 1.
   Créer le budget exige « Billing Account Costs Manager » ou « Billing Account Administrator » sur le compte de facturation, et `pubsub.topics.setIamPolicy` et `pubsub.topics.list` sur le projet du topic.
   La première notification met quelques heures à arriver, puis plusieurs arrivent par jour.
3. Accordez au compte de service du serveur qui importe `roles/pubsub.subscriber` (Pub/Sub Subscriber) sur l’abonnement. Il sert à tirer les messages et à les acquitter (`pubsub.subscriptions.consume`).

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. Définissez `MY_BILLING_GCP_METHOD=budget` et `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`.

L’authentification est la même que pour BigQuery, les Application Default Credentials (voir le tableau de [Rôles IAM](#iam-roles) ci-dessus).
Le build tag `no_billing_gcp` retire aussi cette méthode. La bibliothèque cliente Pub/Sub est partagée avec `stream = pubsub` : elle ne quitte donc le binaire que si vous définissez aussi `no_pubsub` (voir [build-tags.md](../reference/build-tags.md)).

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### Comment les notifications sont lues

- Un import répète Pull jusqu’à ce qu’aucun nouveau message n’arrive, et prend, pour chaque budget et chaque mois, la notification dont l’heure de publication est la plus récente. Cela absorbe les livraisons Pub/Sub en double, les notifications répétées avec la même valeur et les réordonnancements : recevoir plusieurs fois la même notification ne compte donc qu’une fois dans le total.
  Le nombre de requêtes comptées pour un import est le nombre de Pull, normalement deux : un qui renvoie des messages et un qui revient vide. Les acquittements faits à la fin (`Acknowledge`, `ModifyAckDeadline`) ne sont pas comptés.
- Le serveur n’acquitte (ack) qu’une notification qu’une notification plus récente du même budget et du même mois a remplacée, et une notification d’un mois qui n’est plus importé (le mois précédent, une fois enregistré comme définitif). La dernière notification d’un mois importé n’est pas acquittée mais rendue à l’abonnement, de sorte que l’import suivant relit la même notification, et la valeur ne change pas un jour sans nouvelle notification.
- Définissez donc la rétention des messages de l’abonnement à **au moins 14 jours (31 jours recommandés)**. La dernière notification du budget d’un mois arrive vers le 2 du mois suivant, et le mois précédent est relu jusqu’au 15. Un mois dont les notifications ont expiré est enregistré comme un coût de 0.
- Par défaut, un abonnement expire après 31 jours sans utilisation. Définissez l’expiration sur « never » pour qu’il survive à une période pendant laquelle les imports étaient arrêtés.
- N’envoyez pas d’autres messages au topic. Un message qui ne peut pas être lu comme une notification de budget fait échouer cet import avec `invalid_response`, et est acquitté pour que l’import suivant ne reste pas bloqué dessus.
- Coût : quelques dizaines de messages par jour tiennent dans l’offre gratuite de Pub/Sub (10 GiB par mois), cela coûte donc 0 $. Le serveur ne fixe aucune limite.

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### Types d’échec (notifications de budget)

| Type | Réponse de Pub/Sub | Comment corriger |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Vérifiez `roles/pubsub.subscriber` sur l’abonnement et la manière dont les identifiants sont transmis |
| `not_enabled` | `PERMISSION_DENIED` (`SERVICE_DISABLED`) | Activez l’API Pub/Sub dans le projet de l’abonnement |
| `not_found` | `NOT_FOUND` | Vérifiez `billing_gcp_budget_subscription` et si l’abonnement a expiré |
| `throttled` | `RESOURCE_EXHAUSTED` | La tâche réessaie après une pause |
| `unavailable` | `UNAVAILABLE`, `INTERNAL` et similaires, défaillance réseau | La tâche réessaie après une pause |
| `invalid_response` | Un message qui ne peut pas être lu comme une notification de budget, un budget qui n’est pas mensuel, deux budgets ayant le même nom d’affichage | Rendez le budget mensuel, donnez aux budgets des noms d’affichage distincts, et n’envoyez aucun autre message au topic |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## Un plugin (`billing_sources = plugin`)

Pour un prestataire dont le serveur n’a pas d’adaptateur, comme Azure, Hetzner ou Sakura, exécutez un [plugin](plugins.md) du type `billing` et laissez-le lire les données du prestataire. Le serveur a une seule source `plugin`. Un plugin couvrant plusieurs prestataires les agrège lui-même et les distingue par les noms de ses services.

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

Les trois paramètres sont obligatoires avec cette source, et le serveur démarre sans appeler le plugin, même s’il est arrêté.

- `plugin_billing_location` est le fuseau horaire qui découpe les mois de facturation du plugin.
- `plugin_billing_provider` est le nom du prestataire dans les totaux, sur la page des coûts et dans le journal d’audit.

Le serveur demande au plugin le mois en cours et, comme pour les autres sources, le précédent jusqu’au 15 ou jusqu’à ce qu’il soit définitif. Les plafonds des autres sources ne s’appliquent pas : `max_requests` vaut `2147483647`, ce qui signifie aucun plafond, et le plugin décide de ce qu’il peut dépenser. Il signale les requêtes et les octets qui lui ont été facturés, que le serveur enregistre et affiche.

Ce type autorise le TCP simple, avec un avertissement au démarrage, puisqu’il ne transporte que des coûts. Un jeton bearer exige `https://` ou un socket Unix : voir [la destination d’un plugin](plugins.md#the-destination).

<!-- source: the-call 6a2bcce82ab2 -->
### L’appel

Avec `plugin_billing_protocol = json`, la valeur par défaut, un plugin n’a besoin que d’un serveur HTTP et d’une bibliothèque JSON ([Un plugin sans bibliothèque](plugins.md#a-plugin-with-no-library)). `Fetch` dure au plus 2 minutes, ou `plugin_billing_timeout`, inférieur à 5 minutes.

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- Une ligne est un mois, un service et un type de frais, avec ses lignes compensatoires, crédits et remboursements sommés. `service` a au plus 200 caractères et aucun caractère de contrôle.
- `coveredStart` et `coveredEnd` sont à l’intérieur du mois : le mois entier quand `final` est vrai, jusqu’au dernier jour avec des données avant cela sinon.
- `amount` est une décimale non arrondie en unités principales, qui peut être négative. `currency` est un code ISO 4217.
- `chargeType` est `CHARGE_TYPE_` suivi de `USAGE`, `CREDIT`, `REFUND`, `TAX`, `SUPPORT`, `FEE`, `ADJUSTMENT` ou `OTHER`.
- `category` est `CATEGORY_` suivi de `COMPUTE`, `DATABASE`, `STORAGE`, `NETWORK`, `DOMAIN`, `EMAIL`, `SOFTWARE` ou `OTHER`.
- `requests` est au plus `maxRequests`. `bytesBilled` est une chaîne décimale, pour un prestataire qui facture à l’octet.
- Une clé que `OUTCOME_OK` ne renvoie pas n’a aucun coût dans son mois : le serveur enregistre donc une révision de 0 pour elle. Un plugin qui n’a pas pu lire un prestataire doit signaler un échec, et non omettre ses lignes.
- Les appels peuvent se répéter : une nouvelle tentative d’un import échoué est le même appel de nouveau.

`maxRequests` à 0 interdit toute requête facturée. Le serveur n’effectue aucun appel quand il enverrait 0, mais `my plugin test --kind billing` le fait, avec le champ omis comme le permet la correspondance de proto3, et attend `OUTCOME_REQUEST_CAP` sans ligne et sans requête.

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### Types d’échec

Un plugin signale un échec dans `outcome`. Le serveur conserve les `requests` d’un échec, ignore ses lignes et laisse les imports précédents tels quels, comme avec les autres sources.

| Résultat | Type | Nouvelle tentative |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | Non |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | Non |
| `OUTCOME_NOT_FOUND` | `not_found` | Non |
| `OUTCOME_THROTTLED` | `throttled` | Oui |
| `OUTCOME_REQUEST_CAP` | `request_cap` | Non |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | Non |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | Non |
| `OUTCOME_UNAVAILABLE` | `unavailable` | Oui |
| Un résultat manquant ou inconnu, ou une ligne ou un compteur qui enfreint les règles ci-dessus | `invalid_response` | Non |
| Une erreur HTTP ou Connect, un délai dépassé, une réponse de plus de 4 MiB, aucune réponse | `unavailable` | Oui |

Ce qui n’est pas réessayé attend l’import quotidien suivant. La raison d’un échec se trouve dans le journal du plugin, puisque le serveur abandonne le message d’une erreur, et elle compte aussi dans [`my.plugin.failures`](running.md).

<!-- source: test-it 10a2c62a53d8 -->
### Le tester

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

La première n’effectue aucune requête facturée si le plugin respecte le contrat. La seconde effectue aussi une vraie récupération, qui peut coûter de l’argent auprès des prestataires du plugin. Voir [Tester un plugin](plugins.md#test-a-plugin).
