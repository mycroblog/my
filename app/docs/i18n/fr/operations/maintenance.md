<!-- source: maintenance 46ba28c229dd -->
# Maintenance

Comment mettre le serveur hors service pour intervenir sur la base de données, le KV ou la file de tâches, et ce qu’il faut afficher pendant ce temps.

Il y a deux outils :

- **Le mode maintenance** (`maintenance = true`, `MY_MAINTENANCE=true` ou `my serve -maintenance`). `my serve` démarre sans ouvrir aucun adaptateur
  et répond à chaque requête par un `503`, un `Retry-After` et une réponse que chaque client comprend. Utilisez-le chaque fois que `my serve` lui-même peut s’exécuter.
- **La page exportée** (`my maintenance export`). Du HTML statique issu du même modèle, qu’un répartiteur de charge, un CDN ou un proxy inverse sert
  lorsque `my serve` ne peut plus du tout s’exécuter (image en cours de remplacement, révision qui ne démarre pas). Voir [Exporter la page de maintenance](admin-cli.md#exporting-the-maintenance-page).

<!-- source: what-the-maintenance-mode-answers 05dd30158a8e -->
## Ce que répond le mode maintenance

Le processus ne lit que l’adresse d’écoute, `host`, `account_domain`, `web_origin`, les proxys (`trusted_proxies`, `trusted_proxy_header`),
le [contrôle de l’origine](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) (`origin_secret`, `origin_secret_previous`, `origin_secret_header`), `status_page_url` et les paramètres `maintenance_*` ([référence de configuration](../reference/configuration.md)).
Les paramètres de la base de données, de la file, du magasin de blobs, du KV, de l’e-mail et du flux ne sont ni lus ni vérifiés : ils peuvent donc être erronés ou injoignables pendant ce temps.
Quel que soit le `role`, il ne sert que du HTTP : pas de tâches, pas de tâches périodiques, pas de broker de flux.
`my config check -maintenance` vérifie exactement ce que lit le mode maintenance.

Lorsque [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) est défini, une requête qui exige l’en-tête et ne l’a pas reçoit le `403` de ce contrôle au lieu de la réponse du tableau ci-dessous.

| Requête | Statut | Corps |
| --- | --- | --- |
| `GET /healthz`, `GET /readyz` | `200` | `{"status":"maintenance"}` |
| `GET /api/v1/instance/health` | `503` | `{"status":"maintenance"}` |
| Le reste de l’API cliente (`/api/*`) et les points d’accès OAuth qui répondent en JSON | `503` | `application/problem+json` de type `/api/v1/problems/maintenance`, avec `message`, `until` et `status_page_url` s’il est défini |
| ActivityPub (selon le chemin, `Accept` ou `Content-Type`), WebFinger, host-meta, NodeInfo, les webhooks de paiement, les appels de Cloud Tasks et de Cloud Scheduler (`/_internal/*`) | `503` | `{"error":"maintenance"}` |
| Tout le reste (pages, client web, ses ressources, envois de formulaires) | `503` | La page de maintenance, ou le problème lorsque `Accept` demande du JSON |

Chaque réponse porte `Retry-After` et `Cache-Control: no-store`. `Retry-After` compte les secondes jusqu’à `maintenance_until` tant qu’il est dans le futur
(au plus 86 400), et vaut sinon `maintenance_retry_after` (une durée Go, `10m` par défaut).

La page et le problème affichent :

| Paramètre | Affiché comme |
| --- | --- |
| `maintenance_server_name` | Le nom du serveur. Le nom d’hôte s’il n’est pas défini (le nom propre de l’instance se trouve dans la base de données) |
| `maintenance_message` | Un avis en texte brut, de 2 000 caractères au plus, identique dans toutes les langues |
| `maintenance_until` | Le moment où l’intervention devrait se terminer (RFC 3339). La page l’affiche en UTC ; le client web dans le fuseau horaire de l’utilisateur |
| `status_page_url` | Un lien vers la page d’état ([Supervision](monitoring.md#a-status-page)) |

Les paramètres `maintenance_*` peuvent rester définis en fonctionnement normal : ils ne sont utilisés que lorsque `maintenance` vaut true.
Les textes de la page suivent `Accept-Language` (anglais sinon).

Le client web affiche un avis de maintenance en plein écran lorsque l’API répond avec le problème de maintenance,
garde l’utilisateur connecté et réessaie après `Retry-After`.

<!-- source: how-long a7b6a44f0269 -->
## Combien de temps

Limitez une maintenance **à quelques heures**. Les autres serveurs réessaient leurs remises en cas de `503`, mais aucun d’eux ne lit `Retry-After`,
et lorsque leurs tentatives sont épuisées, les activités envoyées entre-temps sont perdues pour ce serveur.
La fenêtre de nouvelles tentatives la plus courte parmi les principales implémentations est d’environ un jour (Akkoma environ 1 jour, Misskey environ 1,5 jour, Mastodon environ 2 à 3 jours, Pleroma environ 7 jours,
d’après leurs calendriers de nouvelles tentatives).
Les moteurs de recherche traitent un `503` bref comme une panne temporaire, mais Google déconseille de servir un `503` pour `robots.txt` pendant plus d’un jour ou deux.
Un arrêt de plus d’un jour ou deux n’est pas une maintenance : fermez plutôt le service ([Fermer le service](service-closure.md)).

<!-- source: steps 1427d2c6d499 -->
## Étapes

1. **Annoncez-la.** Publiez la fenêtre sur la page d’état (`status_page_url`) et comme annonce (Administration → Annonces) un peu à l’avance.
   Définissez `maintenance_message` et `maintenance_until` dans la configuration du déploiement, et actualisez les pages exportées
   (`my maintenance export`) si le frontal les sert.
2. **Arrêtez la file en mode push** (Cloud Tasks) — voir [ci-dessous](#push-queues). Sinon, chaque remise pendant la fenêtre
   est un `503` qui consomme l’une des tentatives de la tâche.
3. **Passez en mode maintenance** : redéployez avec `MY_MAINTENANCE=true` (selon la cible ci-dessous). Chaque réplique redémarre en mode maintenance ;
   le répartiteur de charge les conserve parce que `/readyz` répond `200`.
4. **Faites le travail.**
5. **Revenez en arrière** : redéployez sans `MY_MAINTENANCE` (ou avec `false`), vérifiez que `/readyz` et `/api/v1/instance/health` répondent `{"status":"ok"}`,
   puis reprenez la file en mode push et fermez l’annonce.

![Ce que le serveur répond avant, pendant et après le mode maintenance, selon le type de requête : comme d’habitude avant et après, et 503 avec Retry-After pendant, sauf /healthz et /readyz, qui restent à 200. La liste ci-dessous dit la même chose.](images/maintenance.svg)

De gauche à droite :

- **Avant**, pendant que vous annoncez la maintenance, chaque requête reçoit sa réponse habituelle : `/healthz`, `/readyz` et
  `/api/v1/instance/health` répondent `200` avec `{"status":"ok"}`.
- **En mode maintenance** (`MY_MAINTENANCE=true`) :
  - `/healthz` et `/readyz` répondent `200` avec `{"status":"maintenance"}`.
  - `/api/v1/instance/health` répond `503` avec `{"status":"maintenance"}`.
  - Le reste de l’API cliente et les points d’accès OAuth qui répondent en JSON reçoivent `503` avec le problème de maintenance.
  - ActivityPub, WebFinger et NodeInfo répondent `503` avec `{"error":"maintenance"}`.
  - Les pages et le client web reçoivent `503` avec la page de maintenance.
  - Chaque réponse porte `Retry-After` et `Cache-Control: no-store`.
- **Après** votre redéploiement sans `MY_MAINTENANCE`, chaque requête reçoit de nouveau sa réponse habituelle.

<!-- source: push-queues 0d0d1fc56a29 -->
### Files en mode push

- **Cloud Tasks** : mettez la file en pause, ce qui lui permet de continuer à accepter de nouvelles tâches mais cesse de les distribuer, et reprenez-la après le travail.
  Le nombre de tentatives d’une tâche est le `retryConfig.maxAttempts` de la file.

  ```sh
  gcloud tasks queues pause my-jobs --location=asia-northeast1
  gcloud tasks queues resume my-jobs --location=asia-northeast1
  ```

  Mettez aussi en pause la tâche Cloud Scheduler de `/_internal/tick` (`gcloud scheduler jobs pause` / `resume`), ou laissez ses appels échouer avec `503` entre-temps.
  Les tâches elles-mêmes sont dans la base de données : une tâche qui a épuisé ses tentatives ne perd que son signal, et le premier tick après le travail signale
  de nouveau chaque tâche due.

<!-- source: per-target 4e060394be4b -->
## Selon la cible

Les pages d’erreur des frontaux ci-dessous répondent aux erreurs du serveur avec la page exportée. Ne les appliquez qu’aux erreurs d’un serveur qui ne s’exécute pas
(`502` et `504`, et le `503` propre au répartiteur de charge lorsqu’il n’a aucun backend sain), ou n’activez la règle que lorsque `my serve` ne peut pas s’exécuter :
une règle sur chaque `503` remplacerait aussi les réponses propres du mode maintenance, comme le problème que lisent l’API cliente et le client web.

<!-- source: cloud-run b728619281de -->
### Cloud Run

Déployez une nouvelle révision avec la variable, puis retirez-la ensuite :

```sh
gcloud run services update my --region=asia-northeast1 \
  --update-env-vars=MY_MAINTENANCE=true,MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
gcloud run services update my --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE,MY_MAINTENANCE_UNTIL
```

Lorsque Cloud Run est exposé directement, sans répartiteur de charge, il n’y a rien devant lui : le mode maintenance est donc le seul outil.

**Lorsque `my serve` ne peut pas s’exécuter** derrière un Application Load Balancer externe global : téléversez le `index.html` exporté dans un bucket Cloud Storage
ajouté comme bucket backend, et ajoutez une politique de réponse d’erreur personnalisée qui répond aux `502` et `504` du backend avec lui et `overrideResponseCode: 503`.
Seul l’Application Load Balancer externe global dispose de réponses d’erreur personnalisées (pas les variantes régionales ou classiques), et la politique ne s’applique
qu’aux réponses du backend, pas à celles que Google Front End produit lui-même. Cette voie ne peut pas envoyer `Retry-After` : les en-têtes personnalisés de la source d’erreur ne sont pas ajoutés.

```yaml
# Part of the URL map (gcloud compute url-maps import).
defaultCustomErrorResponsePolicy:
  errorService: https://www.googleapis.com/compute/v1/projects/<project>/global/backendBuckets/my-maintenance
  errorResponseRules:
    - matchResponseCodes: ["502", "504"]
      path: /index.html
      overrideResponseCode: 503
```

<!-- source: aws-ecs 8c6aa496b517 -->
### AWS ECS

Enregistrez une révision de la définition de tâche du service avec `MY_MAINTENANCE=true` dans l’environnement du conteneur, mettez le service à jour avec elle,
puis faites de même sans la variable ensuite. Avec les modules OpenTofu, `deploy/opentofu/upgrade.sh --platform aws-ecs --maintenance`
fait passer le service en mode maintenance avant une mise à niveau, et le ramène avec la nouvelle révision.

**Lorsque `my serve` ne peut pas s’exécuter** derrière CloudFront : téléversez le `index.html` exporté dans un bucket S3 qui est une seconde origine
(la page d’erreur ne peut pas provenir de l’origine défaillante), et ajoutez des réponses d’erreur personnalisées pour `502` et `504` (et `503` uniquement tant que `my serve` ne peut pas s’exécuter)
avec `ResponsePagePath: /maintenance/index.html` et `ResponseCode: 503`.
CloudFront peut ajouter `Retry-After` au moyen d’une politique d’en-têtes de réponse avec un en-tête personnalisé, mais la politique s’applique à chaque réponse du comportement de cache :
attachez-la donc uniquement pour la maintenance et détachez-la ensuite.

La réponse fixe d’un Application Load Balancer ne peut pas porter la page exportée : son corps fait au plus 1 024 caractères et elle ne définit aucun en-tête comme `Retry-After`.
Utilisez-la seulement pour un court texte et le statut :

```json
{
  "Type": "fixed-response",
  "FixedResponseConfig": {
    "StatusCode": "503",
    "ContentType": "text/plain",
    "MessageBody": "This server is under maintenance. Please try again later. / メンテナンス中です。しばらくしてからお試しください。"
  }
}
```

Sans `Retry-After`, les autres serveurs réessaient selon leur propre calendrier, que la fenêtre indiquée plus haut prend déjà en compte.

<!-- source: docker-compose d564c0b39ae8 -->
### Docker Compose

`deploy/docker/compose.yaml` transmet les paramètres de `.env` au serveur, et prend aussi `MY_MAINTENANCE`, `MY_MAINTENANCE_MESSAGE` et `MY_MAINTENANCE_UNTIL` depuis le shell.
Définissez-les et recréez le service `my`, puis retirez `MY_MAINTENANCE` et recréez-le de nouveau après le travail.
`--no-deps` empêche l’exécution des services ponctuels de base de données (`upgrade-check`, `schema`, `upgrade`) ; une mise à niveau les exécute ([Mettre à niveau](database.md#upgrading)) :

```sh
MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z docker compose up -d --no-deps --force-recreate my
docker compose up -d --no-deps --force-recreate my
```

**Lorsque `my serve` ne peut pas s’exécuter** derrière nginx : servez les pages exportées depuis un répertoire, choisissez-en une selon `Accept-Language`,
et répondez aux `502` et `504` propres à nginx (le serveur est arrêté ou ne répond pas) avec elle, en `503`.
Les `503` propres au mode maintenance passent sans changement, puisque `proxy_intercept_errors` reste désactivé.

```nginx
map $http_accept_language $maintenance_page {
    default  en.html;
    ~*^ja    ja.html;
}

server {
    # ...
    location / {
        proxy_pass http://my:8080;
        error_page 502 504 =503 /maintenance/$maintenance_page;
    }
    location /maintenance/ {
        internal;
        alias /srv/maintenance/;   # the -out directory of my maintenance export
        add_header Retry-After 600 always;
        add_header Cache-Control no-store always;
    }
}
```

<!-- source: microk8s-kubernetes a831623b2454 -->
### microk8s (Kubernetes)

Définissez la variable sur le Deployment, ce qui déploie les pods, puis retirez-la ensuite :

```sh
microk8s kubectl set env deployment/my MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
microk8s kubectl rollout status deployment/my
microk8s kubectl set env deployment/my MY_MAINTENANCE- MY_MAINTENANCE_UNTIL-
```

La sonde de disponibilité sur `/readyz` continue de réussir en mode maintenance : le déploiement progressif aboutit donc et le Service continue d’acheminer vers les pods.

**Lorsque `my serve` ne peut pas s’exécuter** derrière le NGINX Ingress Controller (l’add-on `ingress` de microk8s) : servez les pages exportées depuis un petit
Deployment `nginx` (par exemple depuis un ConfigMap contenant `index.html`, `en.html` et `ja.html`) et faites-en le backend par défaut personnalisé,
le contrôleur lui confiant les réponses aux `502` et `504` du serveur (ajoutez `503` uniquement tant que `my serve` ne peut pas s’exécuter) :

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/custom-http-errors: "502,504"
    nginx.ingress.kubernetes.io/default-backend: my-maintenance   # a Service in the same namespace
```

<!-- source: monitoring-during-the-maintenance 91e64028447e -->
## Supervision pendant la maintenance

- `/healthz` et `/readyz` restent à `200` avec `{"status":"maintenance"}` : les répartiteurs de charge et Kubernetes conservent les instances.
- `/api/v1/instance/health` répond `503` avec `{"status":"maintenance"}` ([Supervision](monitoring.md)).
  Les moniteurs de disponibilité voient le serveur comme arrêté, sauf s’ils comparent le corps ou disposent d’une fenêtre de maintenance ; mettez-les en pause ou définissez la fenêtre.
