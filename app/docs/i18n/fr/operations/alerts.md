<!-- source: alerts-and-runbook 2c201450ab32 -->
# Alertes et runbook

Sur quoi les métriques propres du serveur doivent vous alerter, quelle est l’urgence de chaque alerte, et ce qu’il faut vérifier
et faire lorsque l’une se déclenche. La surveillance du serveur de l’extérieur (moniteurs de disponibilité et page d’état) est décrite dans [Surveiller le serveur de l’extérieur](monitoring.md) ;
mettez-la aussi en place, car aucune métrique n’arrive d’un serveur arrêté.

<!-- source: getting-the-metrics 58d8055fe44d -->
## Obtenir les métriques

Le serveur exporte ses métriques lorsque `metrics_exporter` est défini ([référence de configuration](../reference/configuration.md#metrics_exporter)) ;
le [guide d’exploitation](running.md) liste chaque métrique et ses étiquettes. Les alertes ci-dessous sont écrites pour Prometheus, et les noms sont
ceux que Prometheus voit : les points deviennent des tirets bas, et les suffixes d’unité et de compteur sont ajoutés
(`my.queue.oldest_ready_age` devient `my_queue_oldest_ready_age_seconds`, `my.delivery.attempts` devient `my_delivery_attempts_total`).

| Cible | Paramètre | Alertes |
| --- | --- | --- |
| Autohébergé (Docker Compose, Kubernetes, une VM) | `metrics_exporter = prometheus` | Prometheus collecte `GET /metrics` sur `metrics_listen`, Alertmanager achemine les alertes. Les règles ci-dessous fonctionnent telles quelles |
| Toute cible avec un OpenTelemetry Collector | `metrics_exporter = otlp` | Le Collector transmet au backend de votre choix. Un backend compatible Prometheus qui ajoute les suffixes d’unité et de type (comme le fait par défaut le récepteur OTLP de Prometheus) voit les mêmes noms |
| Google Cloud (Cloud Run) | `metrics_exporter = none` | Politiques d’alerte Cloud Monitoring sur un contrôle de disponibilité, les métriques de la plateforme et des métriques basées sur les journaux (voir [Cloud Run](#google-cloud-cloud-run)) |
| AWS (ECS) | `metrics_exporter = emf` | Alarmes CloudWatch sur les métriques que CloudWatch Logs extrait (voir [AWS ECS](#aws-ecs)) |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus et Alertmanager

Les règles se trouvent dans [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml). Copiez le fichier à côté de votre
`prometheus.yml` et chargez-le :

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` vaut par défaut `127.0.0.1:9464`, que seul un Prometheus sur le même hôte atteint. Dans Docker Compose ou Kubernetes, définissez
  `MY_METRICS_LISTEN=0.0.0.0:9464` et gardez le port sur le réseau privé : l’écouteur n’a aucune authentification.
- Collectez chaque processus (chaque processus `role = web` et `role = worker` a son propre écouteur). Les jauges de la file, des médias, de la taille de la base de données,
  des serveurs distants, de l’import de facturation et de l’instantané portent la même valeur pour toute l’instance dans chaque processus : les règles en prennent donc le maximum.
- Chaque règle a une étiquette `severity` : **`page`** exige quelqu’un immédiatement (des utilisateurs sont affectés ou le seront bientôt), **`ticket`** peut attendre les heures ouvrées.
  Acheminez-les vers des destinataires différents dans Alertmanager. Le `runbook_url` de chaque règle renvoie à sa section ci-dessous.
- Les seuils sont des points de départ pour un petit serveur. Observez les valeurs pendant une semaine et ajustez les seuils et les durées `for` à votre trafic.
  `MyRemoteMediaCacheOverLimit`, `MyMediaStorageLarge` et `MyDatabaseStorageLarge` portent des tailles que vous devez régler selon vos propres limites.
- Pendant le mode maintenance, le serveur ne sert aucune métrique et répond `503` : `MyTargetDown` et les alertes HTTP se déclenchent donc.
  Mettez les alertes en sourdine dans Alertmanager pour la durée de la fenêtre avant de commencer ([Maintenance](maintenance.md#steps)).

Après avoir modifié les règles, vérifiez le fichier et exécutez ses tests unitaires (`alerts.test.yml`) avec `promtool`, fourni avec Prometheus :

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud (Cloud Run)

Avec `metrics_exporter = none`, construisez les alertes à partir de ce que Cloud Run et Cloud Logging ont déjà :

- **Erreurs HTTP et latence** : le nombre de requêtes Cloud Run par classe de code de réponse et la métrique de latence des requêtes du service.
- **Erreurs** : les lignes `error report` vont dans Error Reporting, qui les regroupe et peut vous notifier des erreurs nouvelles et récurrentes ([Rapports d’erreurs](error-reporting.md#google-cloud-cloud-run)).
- **Tâches et requêtes** : le serveur écrit une ligne de journal par requête HTTP (`msg` = `http request`) et par exécution de tâche (`msg` = `job`), avec les
  mêmes étiquettes que les métriques. Faites-en des métriques basées sur les journaux, par exemple un compteur de `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  pour les tâches en échec, ou une distribution de `jsonPayload.duration_seconds` filtrée sur `jsonPayload."my.surface"="client_api"` pour la latence.
- **Arriéré de la file** : avec `queue = cloudtasks`, les tâches sont dans la base de données, donc l’écran des tâches et les lignes de journal des exécutions de tâches montrent l’arriéré ;
  la métrique de profondeur de file de Cloud Tasks compte les signaux en attente de remise.
- **Base de données** : les métriques d’utilisation du disque et de connexions de Cloud SQL, ou les alertes d’utilisation de votre fournisseur de base de données.
- **Coût** : un budget Cloud Billing avec des seuils d’alerte ([Coût et budget](#cost-and-budget)).

Le module [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) les crée sous forme de politiques portant les noms du tableau ci-dessous, et liste chaque alerte qu’il couvre différemment ou qu’il ne peut pas exprimer. Il n’a jamais été appliqué. Une métrique que seul le trafic produit présente des trous sur un service qui descend à zéro : une alerte sur son absence se déclenche donc à tort ; c’est son contrôle de disponibilité qui remarque un serveur inactif mort.

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

Avec `metrics_exporter = emf`, CloudWatch Logs transforme les lignes du serveur en métriques dans l’espace de noms `metrics_emf_namespace`, avec les noms
à points (`my.job.duration`, `my.queue.oldest_ready_age`). Les métriques de durée des requêtes et des tâches proviennent des lignes de journal des requêtes et des tâches, et les
dimensions sont les étiquettes que vous listez dans `metrics_emf_dimensions` ; incluez `my.outcome` pour alerter sur les tâches en échec et `http.response.status_code`
pour alerter sur les 5xx. Les jauges de toute l’instance (file, médias, taille de la base de données, serveurs distants, imports de facturation, instantané) sont écrites par la tâche périodique d’instantané. Les métriques d’exécution Go (`go.*`)
ne sont pas écrites en EMF ; utilisez à la place les métriques de CPU et de mémoire du service ECS.

- **Erreurs HTTP** : les métriques de 5xx et de nombre de requêtes de l’Application Load Balancer.
- **Arriéré de la file** : `my.queue.oldest_ready_age`, que la tâche d’instantané écrit.
- **Base de données** : les métriques de stockage et de connexions de RDS ou Aurora, ou les alertes d’utilisation de votre fournisseur de base de données.
- **E-mail** (`mail = ses`) : les métriques de réputation SES (taux de bounce et de plainte) dans CloudWatch.
- **Coût** : un budget AWS Budgets avec des alertes ([Coût et budget](#cost-and-budget)).

Le module [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) les crée sous forme d’alarmes portant les noms du tableau ci-dessous, avec la même liste. Il n’a jamais été appliqué.

<!-- source: the-alerts d76842ca02b8 -->
## Les alertes

Gravité : **page** = agir immédiatement, **ticket** = pendant les heures ouvrées. « – » dans la colonne de la règle signifie qu’il n’existe pas encore de métrique pour elle ; vérifiez-la comme décrit.

| Domaine | Alerte | Signal | Seuil | Gravité | Causes probables |
| --- | --- | --- | --- | --- | --- |
| File | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | plus de 5 min pendant 10 min | ticket | Trop peu de workers pour un pic (une publication populaire, un relais), des tâches lentes, un worker arrêté |
| File | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | plus de 30 min pendant 5 min | page | Aucun worker ne tourne (planté, non déployé, `role = web` seulement), aucun worker n’exécute un type de tâche, workers bloqués sur la base de données |
| File | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | plus de 10 par heure pendant 15 min | ticket | Un bug dans une tâche, un adaptateur mal configuré (e-mail, blob, facturation), un service distant qui refuse |
| File | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | partie file plus de 5 min, partie totaux plus de 1 h, pendant 15 min | ticket | Le magasin KV est injoignable, les requêtes de base de données de l’instantané échouent ou dépassent le délai |
| Fédération | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_delivery_attempts_total` avec `my_outcome` `retry` ou `failed` | plus de 50 % des tentatives pendant 30 min | ticket | Beaucoup d’abonnés sur des serveurs arrêtés, un grand serveur arrêté, des problèmes de réseau sortant |
| Fédération | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | idem | plus de 95 % des tentatives pendant 15 min | page | Réseau sortant ou DNS cassé, egress bloqué, clés du serveur ou `host` erronés |
| Fédération | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 5 de plus que la veille | ticket | Des serveurs qui ont fermé ou qui bloquent celui-ci, un problème de réseau de ce côté qui a duré des jours |
| Fédération | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | plus de 25 % des POST pendant 30 min | ticket | Échec de la vérification des signatures (décalage d’horloge, échec de récupération des clés), un spammeur ou un domaine suspendu qui réessaie, limites de débit |
| Fédération | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | plus de 10 % des POST pendant 30 min | ticket | L’horloge du serveur est décalée, la récupération des clés des expéditeurs échoue, un serveur a changé ses clés |
| Fédération | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | plus de 5 % des POST pendant 15 min | ticket | La base de données en panne, un bug dans la boîte de réception |
| Erreurs | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | plus de 10 en 15 min et 3 × la moyenne de la veille, pendant 15 min | ticket | Un nouveau bug issu d’une version, une dépendance qui échoue d’une nouvelle façon, une configuration cassée |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | 5xx de `http_server_request_duration_seconds_count` | plus de 5 % pendant 10 min | page | La base de données ou le KV en panne, une mauvaise version, le mode maintenance |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | idem | plus de 1 % pendant 30 min | ticket | Une route en échec, un adaptateur qui échoue de temps en temps |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | p95 de l’API cliente (hors streaming) | plus de 300 ms pendant 30 min | ticket | Base de données saturée ou éloignée, une requête lente, trop peu de CPU, démarrages à froid |
| Ressources | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 pendant 5 min | page | Le processus est arrêté, en boucle de plantage ou en mode maintenance ; le chemin de collecte est cassé |
| Ressources | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | toute augmentation pendant 5 min | page | La base de données est injoignable, ou le schéma n’est pas appliqué |
| Ressources | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 3 redémarrages en une heure | ticket | Tué pour manque de mémoire (OOM), un panic, une sonde de liveness en échec |
| Ressources | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | plus de 90 % pendant 15 min | ticket | `GOMEMLIMIT` trop bas pour la charge, beaucoup de flux ouverts, une fuite |
| Ressources | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | plus de 90 % pendant 15 min | ticket | Pool trop petit pour le trafic, requêtes lentes qui retiennent des connexions |
| Ressources | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`, `db_client_connection_timeouts_total` | attente de plus de 0,5 s par seconde, ou tout dépassement de délai, pendant 10 min | ticket | Comme ci-dessus ; limite de connexions propre à la base de données atteinte |
| Ressources | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | plus de la limite de votre formule moins une marge (exemple 8 Gio) pendant 1 h | ticket | Croissance des publications et du contenu distant en cache ; il est temps de relever la formule ou de raccourcir la rétention |
| Ressources | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | plus de 80 % pendant 15 min | ticket | Beaucoup de flux et de connexions ouverts, un `ulimit -n` bas |
| Ressources | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | plus de 5 % des opérations d’un adaptateur pendant 15 min | ticket | Valkey, le répertoire de blobs ou le backend de flux en échec |
| Ressources | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | plus de 1,2 × `remote_media_cache_max_bytes` pendant 3 h | ticket | Le nettoyage horaire ne s’exécute pas ou échoue |
| Ressources | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | plus de votre taille prévue (exemple 50 Gio) pendant 1 h | ticket | Croissance des téléversements ; il est temps de relever la formule ou le budget |
| E-mail | [`MyEmailFailing`](#myemailfailing) | `my_job_duration_seconds_count` des tâches d’e-mail avec `my_outcome="dead"` | toute occurrence en une heure | ticket | Paramètres SMTP ou SES erronés, un compte suspendu ou sans quota, des adresses refusées définitivement |
| E-mail | [`MyEmailRetrying`](#myemailretrying) | idem, `my_outcome="retry"` | plus de 50 % des exécutions pendant 15 min | ticket | Le serveur de messagerie est injoignable ou refuse la connexion, une limite de débit d’envoi |
| Coût | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 pendant 6 h | ticket | Permissions manquantes, export de facturation non configuré, plafond de requêtes atteint, fournisseur indisponible |
| Coût | Budget dépassé | – | – | – | Trafic, stockage ou une mauvaise configuration qui coûte plus que prévu ([ci-dessous](#cost-and-budget)) |

<!-- source: where-to-look 9ef1d37c4f05 -->
## Où regarder

Ces outils reviennent dans la plupart des entrées du runbook ci-dessous.

- **L’écran des tâches** (Administration → Tâches, `/admin/jobs`, exige `manage_jobs`) : les compteurs par état (en file, en cours, en nouvelle tentative, en échec) et par type, ainsi que
  la plus ancienne tâche en attente ; sa page **Tâches** (`/admin/jobs/list`) liste les tâches d’un état, et la page de chaque tâche montre sa dernière erreur et les tentatives qui n’ont pas abouti. Les tâches en attente ne sont listées que pour les files conservées dans la base de données
  (`queue = db` et `cloudtasks`) ; les tâches en échec et les tentatives sont listées pour toutes les files.
- **La commande `my job`** sur un hôte qui dispose de la configuration du serveur (`queue = db` et `cloudtasks`) :
  `my job list` liste les tâches en échec, de la plus récente à la plus ancienne ; `my job retry <id>...` les remet dans la file ; `my job delete <id>...` les abandonne.
  Les tâches peuvent être réexécutées sans risque.
- **L’écran des serveurs distants** (Modération → Serveurs distants, `/moderation/instances`, exige `manage_domains`) : les serveurs que les envois n’ont pas pu atteindre,
  et ceux dont les envois sont suspendus, avec la raison et le moment. « Reprendre les envois » marque de nouveau un serveur comme disponible.
- **Les journaux** : des lignes JSON sur l’erreur standard. Chaque requête HTTP se termine par une ligne dont le `msg` est `http request`, chaque exécution de tâche par une ligne dont le `msg` est `job`
  (`INFO` en cas de succès, `WARN` pour une nouvelle tentative, `ERROR` pour une tâche définitivement en échec, avec `error`, `job_id` et `my.job.type`).
  Les lignes à l’intérieur d’une requête portent son `request_id`, que le client voit comme `X-Request-Id`.
  - Docker Compose : `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging : `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights : ``filter msg = "job" and `my.outcome` = "dead"``
- **Rapports d’erreurs** : les lignes `error report` et l’écran Erreurs (Administration → Erreurs, `/admin/errors`, exige `manage_errors`) ([Rapports d’erreurs](error-reporting.md)).
- **Traces**, lorsque `traces_exporter = otlp` : les lignes de journal portent `trace_id` ; ouvrez-le dans votre backend de traces pour voir où une requête lente a passé son temps.
- **Le mode maintenance** ([Maintenance](maintenance.md)) : répond à chaque requête par une page de maintenance et ne touche aucun adaptateur. Utilisez-le pendant que vous réparez la base de données
  ou le magasin KV.

<!-- source: runbook e60e811fcff0 -->
## Runbook

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

Des tâches (envois, notifications, e-mails, diffusion sur les fils) attendent un worker pendant plus de 5 minutes.

À vérifier :

- L’écran des tâches : quels types sont en file, et si beaucoup sont en nouvelle tentative. Un pic d’un seul type (`federation.deliver` après une publication avec beaucoup d’abonnés,
  activités de relais) se résorbe de lui-même ; vérifiez que l’âge diminue.
- Que les processus worker tournent (`role = all` ou `role = worker`) et que leurs journaux montrent des lignes `job`.
- `MyDBPoolSaturated` ou `MyAPILatencyHigh` en même temps : les workers attendent la base de données.

Correctif :

- Exécutez davantage de processus worker (`role = worker`), ou donnez plus de CPU à ceux qui existent.
- Si un type de tâche est lent, trouvez-le dans les lignes de journal `job` (`duration_seconds`) et cherchez les erreurs de ce type.

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

La plus ancienne tâche prête attend depuis plus de 30 minutes : rien n’est traité, ou presque rien.

À vérifier :

- Qu’au moins un processus tourne avec `role = all` ou `role = worker`, et qu’il n’est pas en boucle de plantage (`MyProcessRestarting`).
- Les journaux du worker pour des erreurs qui se répètent à chaque tâche (base de données, KV, un panic).
- `MyNotReady` : un worker qui ne peut pas atteindre la base de données ne peut pas prendre de tâches.
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)` : si un type attend et pas les autres, vérifiez qu’un worker l’exécute ([Répartir les workers par type de tâche](running.md#splitting-workers-by-job-type)).

Correctif :

- Démarrez ou redémarrez les workers. Les tâches qu’un worker arrêté détenait retournent dans la file lorsque leur bail expire ; elles peuvent être réexécutées sans risque.
- Si la base de données est la cause, suivez [MyNotReady](#mynotready).

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

Plus de 10 tâches par heure échouent définitivement : elles ont épuisé leurs tentatives, ou la tâche a refusé de s’exécuter de nouveau.

À vérifier :

- L’écran des tâches, filtré sur En échec : les types et la dernière erreur. Les tentatives d’une tâche montrent chaque erreur en chemin.
- Les mêmes erreurs dans les lignes de journal `job` avec `my.outcome` = `dead`.

Correctif :

- Une erreur de configuration (e-mail, blob, import de facturation, destination d’un webhook) : corrigez-la, puis relancez les tâches depuis l’écran des tâches (« Relancer ») ou avec `my job retry <id>...`.
- Les envois vers des serveurs disparus échouent définitivement après environ 4 jours de tentatives ; c’est attendu et ne demande rien.
- Les tâches en échec que personne ne relance sont supprimées après `dead_job_retention` (30 jours par défaut). Supprimez celles que vous ne relancerez pas avec `my job delete`.
- Une erreur qui ressemble à un bug : signalez-la avec le type de tâche et l’erreur (elle ne contient aucune donnée personnelle).

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

Les jauges de la file et du stockage proviennent d’un instantané unique que les processus partagent par le magasin KV. Il est ancien : les alertes de la file et du stockage
voient donc d’anciennes valeurs et peuvent rester silencieuses.

À vérifier :

- Les journaux pour des erreurs concernant l’instantané des métriques ou le magasin KV.
- `MyAdapterErrors` pour `kv.valkey` ou `kv.db`, et la santé de la base de données.

Correctif :

- Réparez le magasin KV ou la connexion à la base de données. L’instantané est recalculé à la prochaine collecte ; la tâche périodique d’instantané s’exécute aussi toutes les 15 minutes.

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

Plus de la moitié des tentatives d’envoi vers d’autres serveurs échouent. Certains échecs sont normaux : des serveurs disparaissent, et le serveur continue de les réessayer pendant environ 4 jours.

À vérifier :

- L’écran des serveurs distants : si les échecs se concentrent sur quelques serveurs (ils sont arrêtés ; rien à faire) ou sont répartis sur beaucoup (soupçonnez ce serveur).
- `http_client_request_duration_seconds_count` par `error_type` : délais dépassés, erreurs DNS ou `forbidden_destination` (la protection SSRF a refusé la destination).

Correctif :

- Échecs sur quelques serveurs : aucun. Après des échecs sur 7 jours différents, les envois vers ce serveur sont suspendus, et le serveur est sondé chaque jour pour les reprendre.
- Échecs partout : suivez [MyDeliveryAllFailing](#mydeliveryallfailing).

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

Presque chaque envoi échoue. C’est presque toujours du côté de ce serveur.

À vérifier :

- Depuis l’hôte ou le conteneur du serveur, que le HTTPS sortant fonctionne (DNS, pare-feu sortant, NAT, un proxy).
- Les lignes de journal `job` de `federation.deliver` pour l’erreur : erreurs TLS, délais dépassés ou `401` de nombreux serveurs (ils ne peuvent pas vérifier la signature de ce serveur).
- Après un changement de domaine ou de clé : `host` doit rester le même une fois que le serveur s’est fédéré, et la KEK doit être celle avec laquelle les clés ont été scellées
  ([gestion de la KEK](key-management.md)).

Correctif :

- Réparez le réseau sortant. Les tentatives échouées sont réessayées avec un backoff (1 minute, 5 minutes, 30 minutes, 2 heures, etc.) : les envois se rattrapent donc d’eux-mêmes.
- Si des envois ont épuisé leurs tentatives, relancez les tâches `federation.deliver` en échec depuis l’écran des tâches.
- Si des serveurs distants ont été suspendus pendant la panne, ils reprennent au prochain envoi réussi, à la prochaine requête signée ou au sondage quotidien ; vous pouvez aussi appuyer sur
  « Reprendre les envois » dans l’écran des serveurs distants.

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

Les envois vers 5 serveurs ou plus ont été suspendus au cours du dernier jour. Un serveur est suspendu après que les envois vers lui ont échoué sur 7 jours différents : quelques suspensions par semaine
sont donc normales, car des serveurs ferment. Beaucoup à la fois signifient que beaucoup de serveurs ont échoué pendant des jours en même temps. `my_federation_instances` avec
`my_federation_instance_state` = `available` compte les serveurs vers lesquels on envoie encore mais qui ont échoué un jour ou l’autre ; un saut à cet endroit précède les suspensions.

La métrique n’a pas d’étiquette par serveur (les étiquettes ne portent jamais un domaine distant). À vérifier :

- L’écran des serveurs distants : l’onglet « Suspendus » liste les serveurs suspendus, avec le moment et la raison. S’ils ont quelque chose en commun (un hébergeur, un logiciel)
  ou s’ils sont dispersés.
- `MyDeliveryFailuresHigh` et `MyDeliveryAllFailing` sur la dernière semaine : un problème de réseau de ce côté qui a duré des jours suspend beaucoup de serveurs.

Correctif :

- Serveurs disparus : rien. Les serveurs suspendus sont sondés chaque jour et reprennent d’eux-mêmes lorsqu’ils répondent.
- Après un problème de ce côté : réparez-le, puis appuyez sur « Reprendre les envois » pour les serveurs qui vous importent, ou attendez le sondage quotidien.
- Un serveur dont l’opérateur signale qu’il bloque celui-ci : rien de ce côté ne peut aider. Pour cesser volontairement de se fédérer avec un serveur, utilisez une mesure sur le domaine
  (Modération → Mesures sur les domaines).

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

Plus d’un quart des activités que d’autres serveurs envoient en POST aux boîtes de réception sont refusées.

À vérifier :

- `my_inbox_activities_total{my_outcome="rejected"}` par `my_inbox_rejection` : `signature` (suivez [MyInboxSignatureFailures](#myinboxsignaturefailures)),
  `suspended` (un domaine suspendu qui continue de réessayer), `federation_off` (la fédération est désactivée), `rate_limited` (une IP cliente ou un serveur au-delà de la
  limite de débit) ou `invalid` (des activités sans `id`, `type`, `actor` ou `object` valide).
- Les lignes de journal `http request` des routes de la boîte de réception (`my.surface` = `activitypub`) portent la même répartition par `http.response.status_code`
  (`401`, `403`, `429`, `400`) et l’adresse du client, lorsque vous avez besoin de savoir d’où viennent les requêtes.

Correctif :

- Les refus d’un domaine suspendu, ou avec la fédération désactivée, ne demandent rien : ils sont refusés comme prévu.
- Requêtes limitées en débit venant d’un seul serveur : un spammeur ou un pic ; suspendez le domaine si cela continue.
- Activités invalides venant d’un seul serveur : le logiciel de ce serveur envoie quelque chose que ce serveur ne lit pas ; signalez-le avec le logiciel et la version du serveur.

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

Plus de 10 % des activités que d’autres serveurs envoient en POST aux boîtes de réception sont refusées parce que leur signature ou leur preuve d’intégrité est absente ou ne se vérifie pas,
ou parce que la clé de l’expéditeur n’a pas pu être récupérée.

À vérifier :

- L’horloge du serveur : les signatures sont refusées en dehors d’une fenêtre de temps. Vérifiez NTP sur l’hôte ou le runtime de conteneurs.
- La récupération des clés des expéditeurs : `MyDeliveryAllFailing` et `http_client_request_duration_seconds_count{my_http_purpose="json"}` par `error_type`
  (le réseau sortant, le DNS, la protection SSRF).
- Si les refus viennent de nombreux serveurs ou d’un seul (les lignes `http request` avec le statut `401`) : un serveur qui a changé ses clés, ou qui signe
  d’une façon que ce serveur n’accepte pas, n’affecte que ce serveur.

Correctif :

- Corrigez l’horloge ou le réseau sortant. Les autres serveurs réessaient leurs envois : rien n’est donc perdu si c’est corrigé en un jour environ.
- Un seul serveur en échec : signalez-le à l’opérateur de ce serveur, ou à ce projet avec le logiciel et la version du serveur.

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

La boîte de réception répond `500` à plus de 5 % des activités. Les expéditeurs réessaient, mais les activités arrivent en retard.

**Vérifiez** les lignes de journal `http request` et d’erreur des routes de la boîte de réception pour l’erreur, et la santé de la base de données (`MyNotReady`, `MyDBPoolWaiting`).

**Corrigez** la base de données ou signalez l’erreur comme un bug. Rien n’est à rejouer : les expéditeurs réessaient.

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

Le serveur signale des erreurs (panics, erreurs derrière les 5xx, tâches en échec) à plus de trois fois son rythme habituel.

À vérifier :

- Les lignes `error report` de la dernière heure, regroupées par `my.error.fingerprint` : les plus fréquentes, leur `my.error.where` et la ligne d’en-tête de `message`.
  Sur Cloud Run, Google Cloud Error Reporting montre les mêmes groupes.
- Si une version, un changement de configuration ou une dépendance défaillante (`MyNotReady`, `MyAdapterErrors`) a commencé en même temps.
- Les lignes de requête ou de tâche avec le même `request_id` ou `job_id`.

Correctif :

- Une dépendance défaillante ou une erreur de configuration : corrigez-la ; les tâches en échec peuvent ensuite être relancées depuis l’écran des tâches.
- Une mauvaise version : revenez à l’image précédente.
- Un bug : signalez-le avec l’en-tête et la pile de la ligne. Les deux sont expurgés des données personnelles.

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

Plus de 5 % de toutes les requêtes répondent 5xx. Les utilisateurs voient des erreurs.

À vérifier :

- Si vous êtes en mode maintenance (`503` attendu ; mettez l’alerte en sourdine la prochaine fois).
- `MyNotReady` (la base de données), `MyAdapterErrors` (KV, blob, flux) et une version récente.
- Les lignes `http request` avec `http.response.status_code` ≥ 500 par `http.route`, et les lignes d’erreur avec le même `request_id`.

Correctif :

- Réparez la dépendance défaillante. Si la base de données demande une intervention, passez en mode maintenance pour que les utilisateurs voient une page claire plutôt que des erreurs.
- Une mauvaise version : revenez à l’image précédente.
- Informez les utilisateurs ([Informer les utilisateurs d’un incident](#telling-users-about-an-incident)) si cela dure.

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

Plus de 1 % des requêtes répondent 5xx sur une demi-heure. Généralement une route ou un adaptateur.

**Vérifiez** les lignes `http request` avec un 5xx par `http.route`, et les lignes d’erreur de ces requêtes. **Corrigez** la dépendance, ou signalez l’erreur comme un bug.

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

Le 95e centile de l’API cliente dépasse 300 ms (l’objectif pour une page de fil d’accueil de 20 publications, à chaud). Le point d’accès de streaming est exclu.

À vérifier :

- `http_server_request_duration_seconds` par `http_route` : une route lente, ou toutes.
- La base de données : `MyDBPoolWaiting`, le CPU de la base de données et son journal des requêtes lentes, la distance réseau entre le serveur et la base de données.
- Le processus : la limitation du CPU, `MyMemoryNearLimit` (un GC sous pression mémoire), `go_schedule_duration_seconds` (des goroutines qui attendent un CPU).
- Les traces des requêtes lentes, si elles sont activées.

Correctif :

- Donnez plus de ressources à la base de données ou au serveur, ou rapprochez-les.
- Les démarrages à froid après une descente à zéro augmentent le centile sur un serveur calme ; c’est attendu.

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus ne peut pas collecter un processus.

À vérifier :

- Que le processus tourne : `docker compose ps`, `kubectl get pods` ou la console de la plateforme. Ses dernières lignes de journal disent pourquoi il s’est arrêté.
- S’il est en mode maintenance (aucune métrique n’est servie ; attendu).
- Que `metrics_exporter = prometheus` et que `metrics_listen` est joignable depuis Prometheus.

Correctif :

- Redémarrez le processus. S’il ne démarre pas, le journal dit pourquoi : une erreur de configuration (`my config check` l’indique), la base de données injoignable, un schéma non appliqué.

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

Le contrôle de disponibilité échoue : la base de données est injoignable, ou ses tables ne correspondent pas à cette version. Le répartiteur de charge cesse d’envoyer du trafic au processus,
et le point d’accès de santé externe signale `unavailable`.

À vérifier :

- Les lignes de journal du contrôle de disponibilité : une erreur de connexion (réseau, identifiants, base de données arrêtée ou à court de connexions), ou les tables manquantes.
- La page d’état et la console du fournisseur de base de données : est-elle en service, à court de stockage ou en maintenance ?
- Après une mise à niveau : le schéma a-t-il été appliqué avant le démarrage de la nouvelle version ([Initialiser la base de données](database.md)) ?

Correctif :

- Base de données arrêtée ou injoignable : rétablissez-la, ou basculez. Pendant votre intervention, le mode maintenance montre aux utilisateurs une page claire.
- Base de données à court de stockage : relevez la formule ou libérez de l’espace ([MyDatabaseStorageLarge](#mydatabasestoragelarge)).
- Schéma non appliqué : exécutez `my schema apply` de la même version, puis les processus deviennent prêts d’eux-mêmes.
- Données perdues ou corrompues : restaurez depuis votre sauvegarde, et gardez la KEK avec laquelle les clés de la sauvegarde ont été scellées ([gestion de la KEK](key-management.md)).

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

Un processus a redémarré 3 fois en une heure.

À vérifier :

- La raison du redémarrage indiquée par la plateforme : `OOMKilled` (mémoire), une sonde de liveness en échec, ou le code de sortie.
- Les dernières lignes de journal avant chaque redémarrage, pour un panic ou une erreur de configuration fatale.

Correctif :

- Mémoire insuffisante : définissez `GOMEMLIMIT` à environ 90 % de la limite de mémoire du conteneur, ou donnez plus de mémoire au conteneur ([MyMemoryNearLimit](#mymemorynearlimit)).
- Un panic : signalez-le avec les lignes de journal qui l’entourent.
- Le journal dit que la base de données appartient à un autre `host` ou `account_domain` : définissez dans la configuration les valeurs enregistrées, ou faites pointer `database_url` vers la bonne base de données. Ces valeurs ne peuvent pas changer ([Paramètres impossibles à modifier ensuite](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

Le runtime Go utilise plus de 90 % de `GOMEMLIMIT`. Près de la limite, le ramasse-miettes s’exécute beaucoup plus souvent, ce qui coûte du CPU et de la latence,
et au-delà, la plateforme peut tuer le processus. Cette alerte nécessite `GOMEMLIMIT` ; sans lui, surveillez plutôt les métriques de mémoire du conteneur.

**Vérifiez** `http_server_active_requests` et les flux ouverts (`my_stream_connections`), et si la mémoire continue de croître alors que la charge est stable (une fuite).

**Corrigez** en augmentant la mémoire du conteneur et `GOMEMLIMIT` avec elle, ou en exécutant davantage de processus. Signalez une croissance régulière qui ne suit pas la charge.

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

Plus de 90 % des connexions de base de données d’un processus sont utilisées. Les requêtes vont bientôt attendre une connexion (`MyDBPoolWaiting`).

**Vérifiez** si le trafic a augmenté ou si les requêtes sont devenues plus lentes (le journal des requêtes lentes de la base de données, `my_adapter_operation_duration_seconds`).

Correctif :

- Augmentez la taille du pool dans `database_url` (pour PostgreSQL, `pool_max_conns`), en gardant le total de tous les processus sous la limite de connexions de la base de données.
- Placez un pooler de connexions devant la base de données si beaucoup de processus la partagent.

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

Du travail attend une connexion de base de données libre, ou a renoncé à attendre après [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (les délais d’attente ne concernent que PostgreSQL) : les requêtes répondent `503`, et les tâches s’exécutent de nouveau plus tard. Suivez [MyDBPoolSaturated](#mydbpoolsaturated) ;
vérifiez aussi que la base de données elle-même n’est pas à sa limite de connexions.

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

La base de données occupe plus que prévu. `my_db_size_bytes` est la taille de la base PostgreSQL (`pg_database_size`, ce que comptent les limites de stockage des fournisseurs)
ou du fichier SQLite (son journal d’écriture anticipée exclu). C’est de la croissance, non une panne. Fixez le seuil de la règle à une marge sous la limite de stockage
de votre formule de base de données, ou du volume qui contient le fichier SQLite, qui doit aussi garder de la place pour le journal d’écriture anticipée et pour les sauvegardes.

Correctif :

- Relevez la formule ou le volume.
- Les publications distantes sont supprimées après le paramètre d’instance `remote_status_retention_days` ; l’abaisser ralentit la croissance.
- Les lignes supprimées libèrent de l’espace que les deux bases de données réutilisent, mais la taille baisse rarement : PostgreSQL conserve l’espace de ses tables, et SQLite conserve les pages
  libres dans le fichier jusqu’à un `VACUUM` (exécuté avec `sqlite3` pendant que le serveur est arrêté).

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

Le processus a ouvert plus de 80 % de sa limite de descripteurs de fichiers. Au-delà, les nouvelles connexions échouent.

**Corrigez** en relevant la limite (`ulimit -n`, le `nofile` du runtime de conteneurs), ou en exécutant davantage de processus. Vérifiez `my_stream_connections` et
`http_client_open_connections` pour savoir ce qui les retient.

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

Plus de 5 % des opérations d’un adaptateur échouent : `kv.valkey` (Valkey), `kv.db` (la table KV), `blob.fs` (le répertoire des médias),
`queue.db` (la table de la file) ou un `stream.<backend>` (les notifications entre processus).

**Vérifiez** les lignes d’erreur de cet adaptateur, et le service qui se trouve derrière : Valkey en service et joignable, le volume des médias monté, accessible en écriture et non plein.

**Corrigez** le service. Les opérations de tâches échouées sont réessayées ; les notifications de flux échouées ne retardent que les mises à jour en direct.

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

Les copies de médias distants restent au-dessus de `remote_media_cache_max_bytes`. Le nettoyage horaire supprime normalement les copies expirées et les moins récemment utilisées.
Fixez le seuil de la règle à votre valeur de `remote_media_cache_max_bytes`.

**Vérifiez** l’écran des tâches et les journaux pour des échecs des tâches de nettoyage, et `MyAdapterErrors` pour le stockage de blobs.

**Corrigez** la cause des échecs, ou réduisez la taille du cache ou la politique de médias distants de l’instance.

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

Les fichiers médias occupent plus que prévu. C’est de la croissance, non une panne. Fixez le seuil de la règle à ce que permettent votre formule de stockage ou votre budget.

**Corrigez** en relevant la formule ou le budget, ou en abaissant les limites de téléversement (`media_max_image_bytes`, `media_max_av_bytes`).

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

Des tâches d’e-mail ont définitivement échoué au cours de la dernière heure : vérification, réinitialisation du mot de passe, avis de mot de passe, notifications ou e-mails de fermeture.
Les utilisateurs qui attendent un e-mail de vérification ou de réinitialisation ne peuvent pas terminer.

**Vérifiez** l’écran des tâches filtré sur les types de tâches d’e-mail (ils se terminent par `_email`), et les dernières erreurs. [Envoi d’e-mails](email.md#look-at-send-failures)
liste les erreurs et leur signification.

Correctif :

- Hôte, port ou identifiants SMTP erronés, ou problème de compte ou de permission SES : corrigez-le, puis relancez les tâches en échec.
  Les e-mails de vérification et de réinitialisation plus anciens que la durée de vie de leur lien se terminent sans envoi lorsqu’ils sont réessayés : demandez donc à ces utilisateurs d’en demander un nouveau.
- Les adresses refusées définitivement sont supprimées ; rien à faire.

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

La plupart des envois d’e-mails échouent et attendent une nouvelle tentative : le serveur de messagerie est injoignable, refuse la connexion ou limite le débit. Les nouvelles tentatives durent environ 15 minutes pour
les e-mails transactionnels et environ 4 heures pour les notifications avant d’échouer définitivement. Suivez [MyEmailFailing](#myemailfailing) avant qu’elles n’échouent.

<!-- source: cost-and-budget 5a63bdd2754d -->
### Coût et budget

Le serveur n’alerte pas sur le coût. Définissez un budget avec des alertes chez le fournisseur, qui voit les charges en premier :

- AWS : un budget de coût AWS Budgets avec des alertes, par exemple à 80 % et 100 % du montant mensuel, filtré par le tag d’allocation des coûts de ce serveur.
- Google Cloud : un budget Cloud Billing pour le projet, avec des seuils d’alerte à 50 %, 90 % et 100 %. Les notifications du même budget peuvent alimenter
  l’import des coûts ([Importer les charges d’utilisation du cloud](billing-import.md)).

Les charges importées et l’estimation du mois en cours apparaissent dans Administration → Coûts → Imports de facturation. Les imports échoués y sont consignés et réessayés par l’import quotidien suivant ;
[`MyBillingImportFailing`](#mybillingimportfailing) vous prévient avant que les chiffres du mois ne deviennent périmés.

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

Le plus récent import des charges d’utilisation d’une source de facturation a échoué, et aucun import ultérieur n’a réussi depuis 6 heures. Les imports s’exécutent une fois par jour ; la limitation de débit et
l’indisponibilité sont réessayées dans les heures, et les autres échecs attendent l’import du lendemain : l’écran des coûts montre donc des chiffres plus anciens d’ici là.

À vérifier :

- Administration → Coûts → Imports de facturation : le dernier import de la source et son erreur.
- `my_billing_imports_total{my_outcome="failed"}` par `error_type`, ou la ligne de journal `WARN` dont le `msg` est `billing import failed`, avec `source` et
  `error_kind`. [Importer les charges d’utilisation du cloud](billing-import.md) liste les types et la façon de corriger chacun.

Correctif :

- Corrigez la cause (permissions, export de facturation, plafond de requêtes), puis appuyez sur « Importer maintenant » dans Administration → Coûts → Imports de facturation au lieu d’attendre le lendemain.

Sans Prometheus (Cloud Run avec `metrics_exporter = none`), créez plutôt une alerte basée sur les journaux sur la ligne `billing import failed`.

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## Informer les utilisateurs d’un incident

1. **Accusez réception** de l’alerte pour que les autres personnes d’astreinte sachent que quelqu’un s’en occupe.
2. **Publiez sur la page d’état** (`status_page_url`, voir [Surveiller le serveur de l’extérieur](monitoring.md#a-status-page)) dès que des utilisateurs sont affectés :
   ce qui est en panne, depuis quand, et quand vous publierez de nouveau. La page d’état reste accessible lorsque ce serveur ne l’est pas, et le client web y renvoie.
3. **Publiez une annonce** si les utilisateurs peuvent encore atteindre le serveur (dégradé, non arrêté) : Administration → Annonces, rédigez le texte et publiez-le.
   Les annonces s’affichent au-dessus du fil d’accueil de chacun. Utilisez plutôt le mode maintenance avec un `maintenance_message` si le serveur doit s’arrêter.
4. **Mettez à jour** les deux à l’heure promise, même si rien n’a changé.
5. **Clôturez** : marquez l’incident comme résolu sur la page d’état, retirez l’annonce (ou publiez un court suivi), et notez ce qui s’est passé
   et ce que vous avez changé pendant que c’est frais.
