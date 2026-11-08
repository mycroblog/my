<!-- source: error-reporting 1ccf0c1c98a4 -->
# Signalement d’erreurs

Comment le serveur vous informe de ses propres bogues et défaillances, où lire les rapports sur chaque plateforme, et comment les envoyer à un outil de suivi d’erreurs.
Les métriques vous disent que quelque chose ne va pas ; un rapport d’erreur vous dit ce qui a échoué, avec sa pile. La [référence de la configuration](../reference/configuration.md) liste tous les paramètres.

Trois choses arrivent à chaque rapport, quelle que soit votre configuration :

1. Une ligne `error report` est écrite sur l’erreur standard, avec la pile ([La ligne de journal](#the-log-line)).
2. Le rapport est compté dans un groupe d’erreurs dans la base de données. L’écran d’administration affiche les groupes, et les personnes ayant la permission `manage_errors` sont notifiées des groupes nouveaux et récurrents ([Groupes d’erreurs et notifications](#error-groups-and-notifications)).
3. Si vous définissez `errors_exporter`, le rapport part aussi vers un service externe ([Envoyer les rapports à un service](#sending-reports-to-a-service)).

Rien ne quitte le serveur tant que vous ne définissez pas `errors_exporter`.

<!-- source: what-is-reported 8e5ac9d14c36 -->
## Ce qui est signalé

| Signalé | Non signalé (les métriques et les autres lignes de journal le couvrent) |
| --- | --- |
| Une panique dans un gestionnaire HTTP, une tâche ou une tâche de fond | Les réponses 4xx (entrée invalide, autorisation, limites de débit) et le mode maintenance |
| Une erreur derrière une réponse 5xx de l’API client, de l’API d’administration, des pages de connexion, d’ActivityPub, d’OAuth ou des médias | Une requête dont le client est parti |
| Une tâche qui a définitivement échoué (`dead`) | Une nouvelle tentative de tâche. Elle est signalée quand elle n’a plus de tentatives |
| Un échec au démarrage : une configuration invalide, ou une base de données ou un adaptateur qui ne peut pas être mis en place | Un contrôle de disponibilité échoué |
| Une erreur que le client web n’a pas gérée, envoyée au serveur par le navigateur | Les échecs causés par d’autres serveurs : nouvelles tentatives et abandons de livraison, signatures rejetées, récupérations distantes échouées |
| | Le bruit propre au navigateur : un réseau perdu, les erreurs des extensions du navigateur et les chunks qu’un nouveau déploiement a supprimés |

La défaillance d’un service extérieur (e-mail, stockage, paiements, voix) n’est signalée que si elle fait échouer définitivement une tâche ou répondre une requête en 5xx. Une défaillance temporaire apparaît plutôt dans les métriques `my.adapter.*` ([Exécuter le serveur](running.md#metrics)).

Une panique dans une tâche de fond arrête le processus après avoir été signalée, de sorte que le serveur ne continue jamais à tourner dans un état défaillant. Votre plateforme le redémarre.

<!-- source: the-log-line 6b570d0bfd09 -->
## La ligne de journal

La ligne a `msg` = `error report` et `level` `ERROR` (`severity` vaut `CRITICAL` quand la défaillance arrête le processus).
Ses champs, et ce que le nettoyage leur fait, sont décrits dans [Exécuter le serveur](running.md#error-reports). En bref :

- `my.error.fingerprint` est identique pour toutes les occurrences d’une même erreur. Recherchez ou groupez par ce champ.
- `my.error.where` indique où cela s’est produit, par exemple `client_api GET /api/v1/statuses/{id}`.
- `request_id`, `job_id`, `trace_id` et `span_id` permettent de retrouver les autres lignes de la même requête ou de la même tâche.
- `message` est en dernier sur la ligne : le type et le message de l’erreur, un saut de ligne, puis la pile Go.

Aucune autre ligne ne porte de pile.

<!-- source: google-cloud-cloud-run 925c99683713 -->
### Google Cloud (Cloud Run)

Cloud Error Reporting lit la ligne sans aucune configuration et liste les erreurs dans la console sous Error Reporting. C’est gratuit ; vous ne payez que Cloud Logging.

- Il groupe les erreurs par le type et les premières trames de la pile. Il ne peut pas utiliser l’empreinte du serveur : un groupe d’Error Reporting peut donc différer d’un groupe de l’écran d’administration.
- Il conserve les groupes 30 jours, et vous notifie des nouvelles erreurs et des erreurs résolues qui se reproduisent, au plus 20 notifications par heure pour le projet. Configurez son canal de notification dans la console Google Cloud.
- Pour retrouver ce qui s’est passé autour d’un rapport, recherchez dans Cloud Logging `jsonPayload."request_id"="<id>"`, ou la même chose avec `trace_id`.

<!-- source: aws 6ca6594afd7e -->
### AWS

CloudWatch Logs lit la ligne comme du JSON simple.

- Activez la détection d’anomalies de journaux pour le groupe de journaux. Elle signale un motif de ligne jamais vu auparavant et peut déclencher une alarme vers SNS. Elle ne lit que les 1 500 premiers caractères d’une ligne, ce qui explique que `message` soit le dernier champ. Elle ne conserve pas d’état « résolu ».
- Pour compter les rapports, créez un filtre de métrique comme `{ $.msg = "error report" }`. N’utilisez aucune dimension tirée de `my.error.fingerprint` : chaque valeur d’une dimension est facturée comme une métrique à part entière. `my.error.source` et `my.error.kind` sont sûrs.
- Retrouvez le contexte avec Logs Insights : ``filter `request_id` = "<id>"``.

<!-- source: self-hosted 90de7a419474 -->
### Auto-hébergé

Lisez la ligne dans le journal du conteneur : `docker compose logs my | jq -c 'select(.msg == "error report")'`.
Avec `metrics_exporter = prometheus`, l’alerte `MyErrorReportsIncreasing` ([Alertes et runbook](alerts.md#myerrorreportsincreasing)) prévient quand les rapports augmentent fortement ; le compteur qui la sous-tend est `my.errors.reported`.
Pour des notifications qui n’exigent aucun service extérieur, utilisez l’écran d’administration et les webhooks ci-dessous. Pour un outil de suivi, utilisez [`errors_exporter`](#sending-reports-to-a-service).

<!-- source: error-groups-and-notifications 8f740a328309 -->
## Groupes d’erreurs et notifications

Le serveur compte les rapports par empreinte dans la base de données. Cela n’exige aucun paramètre ni service supplémentaire, et écrit peu : le premier rapport d’une empreinte est écrit immédiatement, et les suivants sont additionnés en mémoire et écrits une fois par minute. Un arrêt brutal du processus peut perdre les compteurs de la dernière minute ; les lignes de journal ne sont pas perdues ainsi.
Les compteurs de plus de 90 jours sont supprimés chaque jour. Si c’est la base de données qui a échoué, le rapport reste uniquement dans la ligne de journal.

<!-- source: the-administration-screen 09092782a7c1 -->
### L’écran d’administration

Les personnes dont le rôle a `manage_errors` (le rôle intégré `admin` l’a) voient « Erreurs » dans le menu d’administration (`/admin/errors`).

- La liste affiche les groupes, les groupes ouverts par défaut. Filtrez par état (ouvert, résolu, en sourdine, tous les états) et par source (le serveur, le client web). Chaque ligne indique le type d’erreur, l’endroit où elle s’est produite, les premières lignes du message, les compteurs des dernières 24 heures et des 7 derniers jours, ainsi que la première et la dernière date et version.
- La page d’un groupe affiche la dernière occurrence (date, version, rôle du processus, ID de requête, ID de trace, les types, le message et la pile), les occurrences par heure sur les 90 derniers jours, et l’historique de ses changements d’état. L’ID de requête et l’ID de trace ont un bouton « Copier » : collez-les dans votre recherche de journaux.
- « Marquer comme résolue » sert pour une erreur que vous avez corrigée. Si elle se reproduit sur une autre version du serveur, le groupe se rouvre et notifie de nouveau. La même erreur sur la version qui a été résolue est seulement comptée, afin que les dernières requêtes de l’ancienne version ne le rouvrent pas.
- « Mettre en sourdine » continue de compter mais n’envoie ni notification ni événement de webhook. « Retirer la sourdine » rouvre le groupe.

Les mêmes actions sont disponibles via `GET /api/v1/admin/error_groups` et `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`).

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### Notifications et webhooks

- Les personnes ayant `manage_errors` reçoivent la notification `admin_error_group` pour un nouveau groupe d’erreurs et pour une récurrence. Au plus 10 arrivent par heure pour l’ensemble du serveur ; le reste de l’heure est regroupé en une seule notification indiquant que d’autres erreurs ont été signalées. Elles peuvent aussi la recevoir par e-mail ([Modération](moderation.md#notifications-for-moderators-and-the-pending-counts)).
- Les événements de webhook `error_group.created` et `error_group.regressed` portent les mêmes groupes : vous pouvez donc les envoyer à Slack, à Discord ou à un outil d’automatisation ([Webhooks du serveur](webhooks.md), [Référence des webhooks](../reference/webhooks.md)). Le webhook exige `manage_webhooks`, et les événements exigent `manage_errors`.
- Un groupe en sourdine ne notifie jamais.

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### Erreurs du client web

Le client web envoie les erreurs que ses écrans n’ont pas gérées à `POST /api/v1/client_errors` sur votre propre serveur : aucun tiers ne les voit et aucun paramètre de sécurité du contenu ne change. Le serveur répond `204` à tout et abandonne ce qu’il n’accepte pas : un corps de plus de 16 KiB, une pile sans fichier de votre propre client web, et plus de 10 rapports par minute depuis une même adresse IP. La même erreur est envoyée une fois par affichage de page, et au plus 20 par affichage de page.
Un rapport envoyé sans session connectée est compté et journalisé mais ne notifie personne à lui seul, de sorte qu’un script anonyme ne peut pas remplir votre boîte de réception. Le groupe notifie quand le navigateur d’une personne connectée le signale aussi.
Les piles sont celles du build minifié et ne sont pas retraduites vers les sources : lisez donc d’abord la route et le message. La version est celle du serveur qui a servi la page. Une page ouverte hors ligne, ou après 3 secondes sans réponse, provient de la copie du navigateur, qui indique la version du moment où le navigateur a stocké le client web : une version plus ancienne avec le même client web.

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## Envoyer les rapports à un service

Définissez `errors_exporter` pour envoyer aussi chaque rapport à un outil de suivi d’erreurs. La ligne de journal et les groupes dans la base de données restent actifs. Un rapport est envoyé une fois et jamais renvoyé, et rien n’attend le service.

| Valeur | Envoie vers |
| --- | --- |
| `none` (par défaut) | Nulle part |
| `sentry` | Tout service compatible Sentry à `errors_sentry_dsn` |
| `otlp` | Un collecteur ou un backend OpenTelemetry, sous forme d’enregistrements de journal |

`errors_environment` (par défaut `production`) nomme le déploiement sur chaque rapport, par exemple `staging`.

Les deux exporteurs envoient les mêmes champs nettoyés que la ligne de journal, avec l’empreinte et les ID de trace et de span, et rien sur l’hôte : ni nom d’hôte, ni adresse IP, ni utilisateur, ni corps de requête, ni en-tête, ni cookie.

Le serveur limite ce qu’il envoie pour que le service ne soit pas inondé :

- La même empreinte part au plus une fois par minute et par processus, et l’ensemble des rapports au plus 60 par minute. Le reste demeure dans le journal et dans les groupes.
- Jusqu’à 1 000 rapports attendent d’être envoyés. Un rapport qui trouve la file pleine est abandonné. À l’arrêt du serveur, celui-ci attend jusqu’à 2 secondes que la file se vide.
- Quand le service répond `429` ou demande de ralentir (`Retry-After`, `X-Sentry-Rate-Limits`), le serveur cesse d’envoyer pendant cette durée.
- Le serveur ne suit jamais une redirection du service, afin que la clé du DSN et le rapport n’aillent nulle part ailleurs : une redirection compte comme `failed`. Utilisez le DSN que le service affiche, qui pointe lui-même vers son point d’entrée.

Surveillez le compteur `my.errors.exported` : `my.outcome` vaut `success`, `dropped` (au-delà d’une limite, la file était pleine, ou le service a demandé de ralentir) ou `failed` (le service était injoignable ou a refusé le rapport) ([Exécuter le serveur](running.md#metrics)).

<!-- source: sentry-glitchtip-and-datadog 12c935ec3c05 -->
### Sentry, GlitchTip et Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

Le serveur parle lui-même le protocole Sentry : aucun SDK Sentry n’intervient et aucune variable d’environnement `SENTRY_*` n’est lue. Il définit l’empreinte, de sorte que le service groupe par elle, ainsi que les tags `service` (`my`), `my.role`, `my.error.where` et `error.fingerprint`. La version est celle du serveur et l’environnement est `errors_environment`.

- **Sentry** (hébergé ou auto-hébergé) : copiez le DSN depuis les clés client du projet.
- **GlitchTip** : copiez le DSN du projet. Il est open source et tourne sur un petit serveur. Vérifiez quelles règles d’alerte votre version propose pour les problèmes nouveaux et récurrents.
- **Datadog Error Tracking** : utilisez le DSN Sentry de votre site Datadog, `https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`. Datadog exige le tag `service`, que le serveur définit toujours. Datadog stocke ces événements comme des journaux et les groupe dans Error Tracking. Si vous envoyez plutôt la ligne de journal à Datadog avec un agent ou un forwarder (Error Tracking for Logs), la pile se trouve dans `message` ; associez-la à `error.stack` avec un pipeline de journaux, faute de quoi Datadog ne la voit pas comme une pile. L’exporteur `sentry` n’exige aucune correspondance de ce type.

<!-- source: otlp-backends 778540dabdc8 -->
### Backends OTLP

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

Chaque rapport est un enregistrement de journal OpenTelemetry nommé `exception`, avec la sévérité `ERROR` (`FATAL` quand il arrête le processus), `exception.type`, `exception.message`, `exception.stacktrace`, `my.error.fingerprint`, `my.error.where`, `my.error.source`, `my.error.kind`, ainsi que les ID de trace et de span. Il circule via OTLP/HTTP (protobuf, gzip) avec la même ressource que les métriques et les traces.

Que le backend transforme ces enregistrements en erreurs groupées dépend du backend : certains construisent leurs vues d’erreurs à partir des seules données de trace, et Datadog a besoin de `error.stack`. Pour Sentry, GlitchTip et Datadog, utilisez `sentry`. Utilisez `otlp` quand vous voulez les enregistrements à côté de vos autres journaux. Le build tag `no_otel` exclut `otlp` ([Build tags](../reference/build-tags.md)).

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## Ce que le nettoyage garantit et ne garantit pas

Le nettoyage s’exécute une fois, à la création du rapport : la ligne de journal, la base de données et chaque exporteur ne reçoivent donc que le texte nettoyé.

- Dans le message, les URL, les adresses e-mail, les identifiants `@user@host`, les adresses IP, les UUID et ULID, et les nombres de 10 chiffres ou plus sont remplacés par `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` et `<n>`, et le message est tronqué à 1 KiB.
- Dans la pile, les valeurs des arguments de fonction sont remplacées par `(...)` et la pile est tronquée à 16 KiB. Les positions de fichier restent, relatives à l’arborescence des sources, de sorte que les chemins de la machine de build n’apparaissent pas.
- Aucun rapport ne contient de corps de requête, d’en-tête, de query, de cookie, d’ID de compte, de nom d’utilisateur, d’adresse IP ni de User-Agent.

Le nettoyage repose sur des motifs : il ne garantit donc pas qu’un message ne contienne aucune donnée personnelle. Un nom, un texte libre ou un nombre de moins de 10 chiffres que cite un message d’erreur peut subsister. Le champ `error` des lignes ordinaires `http request` et `job` n’est pas nettoyé de cette façon. Considérez les journaux et l’outil de suivi d’erreurs comme des lieux susceptibles de contenir des fragments de contenu d’utilisateurs, et appliquez-leur le même contrôle d’accès et la même rétention qu’au reste de vos journaux. Dès que vous définissez `errors_exporter`, les règles de localisation, de rétention et d’accès de ce service s’appliquent aux rapports.
