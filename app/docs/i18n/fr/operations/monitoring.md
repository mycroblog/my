<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# Surveiller le serveur de l’extérieur

Comment pointer une sonde de disponibilité vers le serveur, comment éviter que la surveillance n’augmente le coût de fonctionnement, et comment distinguer le mode maintenance ([Maintenance](maintenance.md)) d’une panne. Les chemins des sondes se trouvent dans [la section sur les contrôles de santé du guide d’exploitation](running.md#health-checks).

Les informations de tiers ci-dessous portent la date à laquelle elles ont été confirmées et leur source. Les services changent leurs offres et leurs écrans ; consultez la page liée avant de vous fier à une limite.

<!-- source: which-url-to-monitor 1c5761757c52 -->
## Quelle URL surveiller

| Chemin | Pour | Touche la base de données | En mode de maintenance |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | Moniteurs de disponibilité, pages d’état, client web | Au plus une fois toutes les 60 s par processus | `503` avec `Retry-After` et `{"status":"maintenance"}` |
| `GET /healthz`, `GET /health/live` | La sonde de vivacité de la plateforme | Jamais | `200` `{"status":"maintenance"}` |
| `GET /readyz`, `GET /health/ready` | La sonde de disponibilité du répartiteur de charge ou de Kubernetes | À chaque requête (les requêtes simultanées partagent une seule vérification) | `200` `{"status":"maintenance"}` |

Faites pointer le moniteur externe vers **`/api/v1/instance/health`**, pas vers `/readyz` (`/health/ready`) : celui-ci interroge la base de données à chaque requête ; un moniteur à court intervalle garde donc éveillée une base de données qui descend à zéro. Les sondes sont destinées à votre plateforme.
Les modules [`monitoring-gcp`](../../deploy/opentofu/modules/internal/monitoring-gcp/README.md) et [`monitoring-aws`](../../deploy/opentofu/modules/internal/monitoring-aws/README.md) vérifient `/health/live` (`monitoring-gcp`) ou `/readyz` (`monitoring-aws`) depuis le moniteur propre à la plateforme, à un long intervalle, et leurs README indiquent ce que cela coûte.

Réponses de `/api/v1/instance/health` (sans authentification, `Cache-Control: no-store`) :

| État | HTTP | Corps |
| --- | --- | --- |
| Sain | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| Indisponible (la base de données est inaccessible ou le schéma n’est pas appliqué) | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| Mode de maintenance | `503` avec `Retry-After` | `{"status":"maintenance"}` |

`/healthz` et `/readyz` répondent `200` avec `{"status":"ok"}` lorsque tout va bien, et `/readyz` répond `503` avec un document de problème lorsque la base de données n’est pas prête. `/health/live` et `/health/ready` sont les mêmes vérifications sous d’autres noms.

**Sur Cloud Run, appelez les chemins qui ne se terminent pas par `z`.** Cloud Run réserve certains chemins qui se terminent par `z` et y répond par son propre `404` avant qu’une requête n’atteigne le conteneur : `/healthz` en fait partie, et rien ne promet que `/readyz` continuera de répondre. Depuis l’extérieur d’un déploiement Cloud Run (un moniteur de disponibilité, `curl`), utilisez `/health/live`, `/health/ready` ou `/api/v1/instance/health`. Les sondes du conteneur lui-même ne sont pas concernées.
Le corps ne porte jamais de raison, de nom d’hôte ni de version. La cause se trouve dans le journal du serveur.

`checked_at` est l’heure de la vérification sur laquelle la réponse repose. Le serveur exécute la vérification de la base de données au plus une fois par minute et par processus et réutilise le résultat ; un moniteur qui interroge plus souvent que toutes les 60 s voit donc le même `checked_at` et n’y gagne rien. Avec plusieurs processus, chacun exécute sa propre vérification.

Lorsque [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) est défini, ces trois chemins répondent toujours sans l’en-tête ; ils ne montrent donc pas que le CDN l’a perdu. Surveillez aussi une page passant par le CDN, comme `https://<host>/about`, si un moniteur est votre premier signe de problème.

<!-- source: keep-the-monitoring-cheap d7956dee97ca -->
## Garder la surveillance peu coûteuse

- **Bases de données qui descendent à zéro.** Si l’intervalle du moniteur est plus court que le temps que la base de données attend avant de s’arrêter, la base de données ne s’arrête jamais.
  Neon suspend un calcul après 5 minutes sans activité ; la formule Free ne peut pas le désactiver
  (confirmé le 2026-10-25, [Neon : Scale to zero](https://neon.com/docs/introduction/scale-to-zero)).
  En règle générale, un intervalle de 5 minutes ou plus permet à une base de données inactive de s’arrêter entre les vérifications, et chaque vérification la réveille pendant la durée de la vérification plus le temps d’inactivité.
  Un serveur fédéré reçoit des envois d’autres serveurs jour et nuit ; sa base de données est donc éveillée une bonne partie de la journée de toute façon ;
  le surcoût se limite aux heures calmes.
- **Préférez une répartition moins coûteuse si le coût au repos compte le plus.** Surveillez `/health/live` (qui ne touche jamais la base de données) à un court intervalle, et `/api/v1/instance/health`, qui s’appuie sur la base de données, à un long intervalle (15 minutes ou plus). `/health/live` répond aussi `200` pendant le mode de maintenance ; vérifiez donc `"status":"ok"` dans le corps.
- **Les requêtes elles-mêmes sont légères.** Une vérification toutes les 5 minutes représente environ 8 640 requêtes par mois, une toutes les minutes environ 43 200. Cela tient dans l’offre gratuite de Cloud Run (2 millions de requêtes, 180 000 vCPU-secondes et 360 000 GiB-secondes par mois ; durée arrondie à 100 ms ; confirmé le 2026-10-23, [tarifs de Cloud Run](https://cloud.google.com/run/pricing)).

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## Distinguer le mode maintenance d’une panne

Pendant le mode maintenance ([guide de maintenance](maintenance.md)), le point de terminaison de santé répond `503`, ce que la plupart des sondes traitent comme une panne. Le code de statut seul ne permet pas de les distinguer, décidez donc d’après le corps :

- Considérez `"status":"ok"` dans le corps comme un état sain, et tout le reste comme une panne. Une sonde par mot-clé ou par valeur JSON le fait.
- Pour éviter les alertes pendant une maintenance planifiée, définissez la fenêtre de maintenance de la sonde si le service en propose une (voir le tableau ci-dessous), ou mettez la sonde en pause à la main avant de commencer.
- L’offre Free d’UptimeRobot n’a pas de fenêtre de maintenance : mettez la sonde en pause ou acceptez l’alerte.

<!-- source: setting-up-the-monitors e91307d78a94 -->
## Configurer les sondes

Toutes vérifient la même chose : `GET https://<your-host>/api/v1/instance/health` est sain lorsque la réponse est `200` et que le corps contient `"status":"ok"`.

| Service | Type de sonde | Sain lorsque | Fenêtre de maintenance | Confirmé |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | Le corps contient `"status":"ok"` | Pas dans Free (Solo et au-dessus) | 2026-10-25, [tarifs](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status, ou Keyword | `2xx`, ou le corps contient `"status":"ok"` | Non vérifié | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` et `[BODY].status == ok` | `maintenance` dans la configuration | 2026-10-25, [README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword, ou JSON Query | Le corps contient `"status":"ok"`, ou `status` vaut `ok` | Intégrée | 2026-10-23, [README](https://github.com/louislam/uptime-kuma) |
| Upptime | `sites` dans `.upptimerc.yml` | `200` et le corps contient `"status":"ok"` | Aucune | 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

Créez une sonde de type **Keyword** : URL `https://<your-host>/api/v1/instance/health`, mot-clé `"status":"ok"`, alerte lorsque le mot-clé **n’existe pas**. L’offre Free autorise 50 sondes à un intervalle de 5 minutes, et inclut les sondes par mot-clé. « Custom headers & statuses » et les fenêtres de maintenance commencent à l’offre Solo (payante, intervalle de 60 secondes) (confirmé le 2026-10-25, [tarifs](https://uptimerobot.com/pricing.md)). Une sonde par mot-clé suffit ici, car le contrôle porte sur le corps.

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

Deux formes fonctionnent (confirmé le 2026-10-25, [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/), [Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)) :

- Une sonde **HTTP status** : elle considère une réponse `2xx` comme disponible, donc `200` est disponible et `503` (indisponible ou maintenance) ouvre un incident.
- Une sonde **Keyword** : « Alert us when » est « URL doesn’t contain keyword », mot-clé `"status":"ok"`. La correspondance ne distingue pas les majuscules des minuscules.

La fréquence des contrôles et les fenêtres de maintenance de l’offre Free ne sont pas vérifiées : les pages officielles ne les indiquent pas pour l’offre Free. Les chiffres de l’offre gratuite (10 sondes et heartbeats ensemble, une page de statut) proviennent de la [page de tarifs](https://betterstack.com/pricing), confirmés le 2026-10-23.

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

Dans `config.yaml` (intervalle par défaut `60s` ; confirmé le 2026-10-25, [README](https://github.com/TwiN/gatus)) :

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus possède une section `maintenance` de premier niveau (`start` au format `hh:mm`, `duration`, `timezone` valant `UTC` par défaut, `every` sous forme de liste de jours de la semaine, vide pour tous les jours) pour les travaux planifiés. Le README dit seulement qu’elle sert à éviter les alertes pendant les fenêtres de maintenance ; vérifiez le comportement sur votre version.

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

Ajoutez une sonde de type **HTTP(s) - Keyword** avec le mot-clé `"status":"ok"`, ou **HTTP(s) - Json Query** avec la requête `status` et la valeur attendue `ok`. Les codes de statut acceptés valent par défaut `200-299`, donc le `503` de maintenance et le `503` de panne sont tous deux considérés comme indisponibles. Uptime Kuma possède une fonction de maintenance pour les fenêtres des sondes (le [wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) la mentionne ; ses détails ne sont pas vérifiés). Les types de sondes ont été confirmés le 2026-10-23 dans le code source ([monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720), [EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)).

<!-- source: upptime 90c96c946d06 -->
### Upptime

Dans `.upptimerc.yml` (confirmé le 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/)) :

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

Par défaut, `20x` et `30x` comptent comme disponibles ; définissez `expectedStatusCodes: [200]` pour n’accepter que `200`. Le contrôle s’exécute depuis GitHub Actions selon un calendrier commun à tous les sites (`workflowSchedule.uptime`, toutes les 5 minutes par défaut, ce qui est aussi le minimum autorisé par GitHub Actions). Le nom de l’option indique qu’elle n’est pas recommandée, mais c’est le moyen de tester le corps.

<!-- source: a-status-page 64712ed6d8a5 -->
## Une page de statut

Définissez `status_page_url` (`MY_STATUS_PAGE_URL`, une URL `https:` ; voir la [référence de configuration](../reference/configuration.md#status_page_url)) sur la page publique de votre sonde. Le serveur affiche le lien dans `GET /api/v1/instance`, dans les pages de connexion et d’erreur qu’il génère, dans le client web (y compris lorsque le serveur ne répond pas) et sur la page de maintenance, afin qu’une personne qui ne peut pas joindre le serveur puisse en comprendre la raison. Il est lu depuis la configuration, pas depuis la base de données, donc il fonctionne lorsque la base de données est en panne. Une modification prend effet après un redémarrage.

Options pour la page elle-même (ce serveur n’importe pas l’état d’une page de statut, publiez donc aussi les annonces sous forme d’avis pour tout le serveur) :

- La page de statut hébergée du service de surveillance que vous avez choisi (l’offre Free d’UptimeRobot a une page de statut sans personnalisation ; l’offre gratuite de Better Stack en a une), les pages que Gatus et Uptime Kuma servent eux-mêmes, ou le site statique d’Upptime sur GitHub Pages.
- Atlassian Statuspage, qui est un lieu de publication et non une sonde. Un composant a l’un de cinq états : Operational, Under maintenance, Degraded performance, Partial outage, Major outage (valeurs d’API `operational`, `under_maintenance`, `degraded_performance`, `partial_outage`, `major_outage` ; confirmé le 2026-10-23, [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/), [Statuspage API](https://developer.statuspage.io/)). La façon de modifier automatiquement l’état d’un composant à partir de l’alerte d’une sonde n’est pas vérifiée (elle exige une intégration propre à chaque service, configurée sur un compte réel).
