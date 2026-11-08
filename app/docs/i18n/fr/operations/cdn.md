<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# Placer un CDN devant la diffusion des médias

Un CDN placé devant le serveur (Cloudflare, Google Cloud CDN, CloudFront, etc.) peut diffuser les médias (`/media/` et `/media/proxy/`).
Les requêtes servies depuis le cache du CDN n’utilisent ni le processeur ni le transfert du serveur.
Ce guide décrit la configuration où le serveur est l’origine du CDN (origin pull). Elle s’applique à tous les stockages de blobs : avec `blob = s3` ou `blob = gcs`, le serveur
lit les fichiers dans le bucket privé et les sert sous `/media/` comme avec `blob = fs` ; le bucket n’est donc jamais l’origine du CDN.
Lorsque tout le serveur se trouve derrière le CDN, il peut aussi [refuser les requêtes qui ne sont pas passées par lui](#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: configure-it e915ce01da94 -->
## Configuration

Définissez `media_origin` (`MY_MEDIA_ORIGIN`) sur le nom d’hôte du CDN (par exemple `media.example.org`).

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- La valeur est une origine uniquement (`scheme://host[:port]`), sans chemin, sans requête ni barre oblique finale. `https` est obligatoire, et `http` n’est autorisé que pour les hôtes de bouclage tels que `localhost`. Une valeur invalide fait échouer le démarrage.
- S’il n’est pas défini, les médias sont servis depuis l’origine propre du serveur (`https://{host}`).
- Ces URL utilisent cette origine : `{media_origin}/media/...` pour les médias locaux et `{media_origin}/media/proxy/...` pour le proxy des médias distants.
  Toutes les URL émises par l’API client, par ActivityPub (`attachment`, `icon`, `image` et l’`icon` des émojis personnalisés) et par les aperçus de liens (OGP) sont concernées.
- La Content-Security-Policy du client web ajoute cette origine à `img-src` et `media-src`.
- Les URI fédérés, comme l’`id` d’un objet ActivityPub, restent sous `https://{host}/` quel que soit `media_origin`. Seul l’endroit où les fichiers sont servis change, pas les identifiants.
- Les URL sont construites à chaque lecture et ne sont pas stockées dans la base de données ; modifier `media_origin` change donc aussi les URL des médias existants.
  Les anciennes URL que des serveurs distants ont enregistrées continuent de fonctionner tant que `/media/` sur l’ancienne origine continue d’être servi.

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## Pointer le CDN vers le serveur

- Faites de l’origine (backend) le nom d’hôte public du serveur (`https://{host}`). Le CDN n’a besoin de récupérer sur le serveur que les chemins sous `/media/`.
- Le serveur ne restreint pas `/media/` et `/media/proxy/` selon le nom d’hôte. Il sert le `Host` que le CDN envoie, quel qu’il soit.
- `/media/` ne nécessite aucune authentification, et Authorized Fetch ne s’y applique pas. Le serveur répond telles quelles aux requêtes non signées du CDN.
- Pour les médias distants (`/media/proxy/{id}/{variant}?sig=...`), `sig` fait partie de l’URL. **Incluez la chaîne de requête dans la clé de cache.** Sinon, des URL aux signatures différentes partagent une même entrée.
- Laissez le `Cache-Control` de l’origine décider de la durée de mise en cache (ne le remplacez pas au niveau du CDN).

| Chemin | `Cache-Control` |
| --- | --- |
| `/media/{...}` (médias locaux) | `public, max-age=31536000, immutable` (un an) |
| `/media/proxy/{...}` (médias distants) | `public, max-age=` le nombre de secondes du paramètre du serveur `remote_media_cache_ttl_seconds` |

Les deux sont renvoyés avec `Cross-Origin-Resource-Policy: cross-origin`, afin que les pages d’autres origines puissent les lire.

Sur Cloud Run, `ingress-cloudflare-media` configure cela avec OpenTofu : consultez [Déployer sur Cloud Run](deploy-cloud-run.md#cloudflare).

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## Qui peut ouvrir les URL de médias

Les URL de médias ne sont soumises à aucun contrôle d’accès. Quiconque possède l’URL d’un fichier peut l’ouvrir, sans se connecter, quelle que soit la visibilité de la publication qui le joint.
Cela vaut avec ou sans CDN, et inclut les pièces jointes des publications réservées aux abonnés et des messages directs, ainsi que les URL de proxy des médias distants.

- Les URL sont difficiles à deviner : l’URL d’un fichier local contient des identifiants avec une partie aléatoire, et une URL de proxy porte une signature. Connaître une URL n’en révèle pas d’autres.
- Quiconque a vu une publication peut transmettre les URL de ses médias, et les URL continuent de fonctionner pour les autres.
- Les réponses sont marquées `public` : le CDN et tout cache partagé entre le serveur et le lecteur peuvent donc en conserver une copie (consultez le tableau ci-dessus pour la durée).
  C’est ce qui permet à un CDN de servir des médias sans interroger le serveur.

Informez vos utilisateurs qu’une publication non publique limite les personnes qui la voient, mais n’empêche pas quelqu’un qui possède le lien d’un fichier joint d’ouvrir ce fichier.

<!-- source: deletion-and-the-cache baac405df923 -->
## Suppression et cache

Après la suppression d’un média, le cache du CDN le conserve pendant `max-age`. Un fichier supprimé peut rester visible jusqu’à un an pour les médias locaux, et jusqu’à `remote_media_cache_ttl_seconds` pour le proxy.
Le serveur ne purge pas le cache du CDN. Si quelque chose doit disparaître immédiatement après une suppression ou une mesure de modération, purgez-le à la main dans la console ou l’API du CDN.

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

Le Service Worker du client web ne met pas en cache les médias d’une autre origine et laisse leur gestion au réseau du navigateur ; seul le cache du CDN s’applique donc.

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## Refuser les requêtes qui ne sont pas passées par le CDN

Un CDN ne protège le serveur que des requêtes qui passent par lui. Lorsque l’origine est aussi joignable à sa propre adresse, comme l’est `*.run.app` sur Cloud Run avec du trafic provenant d’Internet, n’importe qui peut l’appeler directement, en contournant le WAF et le cache.
Si les adresses du CDN figurent dans `trusted_proxies`, un appelant situé sur la plateforme serverless du CDN lui-même (par exemple un Cloudflare Worker qui lui appartient) se connecte lui aussi depuis une adresse de confiance et peut choisir l’adresse IP du client que voient les limites de débit.

Les trois paramètres ci-dessous y remédient. Le CDN ajoute un en-tête secret à chaque requête qu’il transmet, et le serveur répond `403` à toute requête qui en est dépourvue, avant de faire quoi que ce soit d’autre de la requête.

![Les chemins des requêtes avec la vérification d’origine activée : les navigateurs, les applications et les autres serveurs passent par le CDN, qui ajoute l’en-tête secret ; la plateforme et toute autre personne atteignent directement le serveur, sans cet en-tête ; la vérification d’origine du serveur laisse passer l’en-tête et les requêtes exemptées, et refuse le reste avec 403. La liste ci-dessous décrit chaque chemin.](images/cdn-request-flow.svg)

- Les navigateurs et les applications (pages, API client, OAuth et URL de médias) et les autres serveurs (remises et récupérations ActivityPub)
  atteignent le CDN à `host` et à l’hôte de `media_origin`. Le CDN répond lui-même aux fichiers `/media/` en cache, et
  définit `origin_secret_header` à `origin_secret` sur chaque requête qu’il transmet au serveur.
- La plateforme (sondes de santé, moniteurs de disponibilité, Cloud Tasks et Cloud Scheduler) et toute autre personne, comme un script ou un
  webhook qui vous appartient, atteignent directement l’adresse propre du serveur, sans l’en-tête.
- La vérification d’origine s’exécute en premier dans le serveur. Elle laisse passer une requête qui porte l’en-tête une fois avec `origin_secret` ou
  `origin_secret_previous` et, sans l’en-tête, uniquement les `GET` et `HEAD` des contrôles de santé et les `POST` vers `/_internal/`.
  Elle retire l’en-tête de toutes les requêtes et refuse les autres avec `403`.
- Une requête qui passe atteint le client web, l’API client, OAuth, ActivityPub, les contrôles de santé ou `/_internal/`,
  ou `/media/` et `/media/proxy/`, qui lisent le stockage des médias (`blob = fs`, `s3` ou `gcs`) ou relaient le fichier d’un
  autre serveur. Une requête refusée reçoit `403` avec `Cache-Control: no-store` et incrémente `my.http.origin_refused`.

Utilisez-la lorsque le serveur possède une adresse publique que la plateforme ne peut pas fermer à tout sauf au CDN. Vous n’en avez pas besoin lorsqu’un pare-feu ou la plateforme limite déjà qui peut se connecter, par exemple un serveur qui n’écoute que sur un réseau privé derrière votre répartiteur de charge.
Ni lorsque ce répartiteur de charge vérifie lui-même l’en-tête du CDN et n’admet que les adresses du CDN, comme peut le faire un AWS Application Load Balancer derrière CloudFront avec une règle d’écouteur et la liste de préfixes gérée `com.amazonaws.global.cloudfront.origin-facing`. Indiquez alors à la fois les adresses du répartiteur de charge et celles du CDN dans `trusted_proxies`, puisque chacun ajoute à `X-Forwarded-For`.

Elle ne peut pas aider lorsque c’est une fonctionnalité propre à la plateforme qui transmet le trafic sans ajouter d’en-tête que vous contrôlez, comme un mappage de domaine Cloud Run. Seul un CDN ou un répartiteur de charge capable d’ajouter un en-tête de requête le peut.

| Paramètre | Ce qu’il contient |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret) (`MY_ORIGIN_SECRET`) | La valeur, d’au moins 32 octets d’ASCII visible. Un secret, lu depuis l’environnement ou `MY_ORIGIN_SECRET_FILE` |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header) (`MY_ORIGIN_SECRET_HEADER`) | Le nom de l’en-tête, à vous. Obligatoire avec `origin_secret`. Les noms qu’un CDN refuse de définir ou que le serveur ou un proxy lit, comme ceux qui commencent par `cf-` ou `x-cf-`, sont invalides |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous) (`MY_ORIGIN_SECRET_PREVIOUS`) | Une seconde valeur acceptée, uniquement [pendant une rotation](#rotate-the-value) |

Si aucun n’est défini, rien n’est vérifié et le serveur se comporte comme si cette section n’existait pas.

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### Ce qui passe sans l’en-tête

| Requête | Raison |
| --- | --- |
| `GET` ou `HEAD` de `/healthz`, `/readyz`, `/api/v1/instance/health` | Les sondes de la plateforme et les moniteurs de disponibilité les appellent directement |
| `POST` vers les chemins `/_internal/` | Cloud Tasks et Cloud Scheduler les appellent sur l’URL propre du serveur. Ils portent leur propre jeton signé |

Tout le reste nécessite l’en-tête, y compris en mode maintenance : le client web et ses ressources, `/media/`, l’API client, OAuth et ActivityPub, y compris les remises des autres serveurs.
Tout ce qui appelle directement le serveur à sa propre adresse, au lieu de passer par le CDN, est donc refusé : un webhook qui vous appartient, un serveur de médias vocaux, un script. Faites-le pointer vers le nom d’hôte du CDN.

Le port des métriques (`metrics_listen`) est un écouteur distinct, non public et non vérifié.

Une requête qui ne porte pas le secret n’a aucune influence sur le reste du traitement, même lorsqu’elle passe : son adresse IP de client est celle du pair de la connexion, quoi que dise `X-Forwarded-For`, et ses `traceparent` et `X-Request-Id` sont ignorés.

<!-- source: set-it-up ee0a17ad2d33 -->
### Mise en place

Chaque nom d’hôte que le CDN transmet au serveur nécessite l’en-tête : `host`, l’hôte de `media_origin` et un `account_domain` distinct si le CDN le sert. Un hôte de médias placé derrière le même CDN le définit lui aussi, puisque `/media/` est refusé comme tout le reste.

1. Générez la valeur et choisissez un nom que rien d’autre n’utilise :

   ```sh
   openssl rand -hex 32
   ```

2. **Définissez d’abord l’en-tête sur le CDN.** Le serveur ignore un en-tête qu’il ne connaît pas ; cette étape ne change donc encore rien.
   - **Cloudflare** : une règle de transformation d’en-tête de requête (Rules, puis Transform Rules, puis Modify Request Header), sur toutes les formules. Faites-la correspondre à toutes les requêtes entrantes de la zone et choisissez **Set static** avec votre nom et votre valeur. Elle remplace une valeur envoyée par un client. Une règle limitée à certains noms d’hôte laisse les autres verrouillés.
   - **Amazon CloudFront** : un en-tête personnalisé d’origine sur l’origine de chaque distribution. Il remplace aussi la valeur d’un visiteur.
   - **Répartiteurs de charge Google Cloud** : un en-tête de requête personnalisé sur le service de backend.
   - Tout autre CDN ou proxy : l’en-tête doit être défini, et non ajouté, sur chaque requête vers le serveur.
3. Définissez `MY_ORIGIN_SECRET` et `MY_ORIGIN_SECRET_HEADER`, puis déployez. `my config check` les valide et répète ce qui est vérifié, mais il ne peut pas voir le CDN.
4. Vérifiez depuis l’extérieur, avec l’adresse de l’origine elle-même :

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

Indiquez aussi les adresses du CDN dans [`trusted_proxies`](../reference/configuration.md#trusted_proxies), sinon le serveur ne voit qu’un seul client et toutes les requêtes partagent les limites de débit du CDN ; `my config check` avertit à ce sujet.

Une requête refusée reçoit le même document de problème `403` que toute autre requête interdite, avec `Cache-Control: no-store`, quelle que soit la manière dont l’en-tête était erroné. Rien dans la réponse ne nomme l’en-tête.

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### Lorsque tout le monde est verrouillé

Une configuration erronée refuse toutes les requêtes alors que les contrôles de santé restent à `200` : la plateforme voit donc un serveur en bonne santé et un moniteur de disponibilité sur `/healthz` ne voit rien. Les signes sont les suivants :

- `403` pour chaque page passant par le CDN.
- La métrique `my.http.origin_refused` augmente tandis que les requêtes de `http.server.request.duration` cessent.
- Des lignes `http request` dans le journal avec le statut `403` et sans `http.route`. Le journal et les traces ne contiennent jamais l’en-tête, son nom ni sa valeur. Une requête refusée ne crée aucun span de trace.

Pour rétablir la situation, déployez sans la vérification : retirez `MY_ORIGIN_SECRET` et `MY_ORIGIN_SECRET_HEADER`. Les autres serveurs retentent leurs remises, mais les personnes n’ont pas pu joindre le serveur entre-temps.
Comparez ensuite le nom et la valeur du CDN avec les paramètres, vérifiez que la règle couvre chaque nom d’hôte, puis définissez-les de nouveau.

<!-- source: rotate-the-value b88f695fde31 -->
### Faire pivoter la valeur

Le CDN et le serveur ne peuvent pas changer au même instant, et chaque réplique du serveur redémarre à son rythme ; avec une seule valeur acceptée, un changement refuserait donc le trafic aussi longtemps qu’elles diffèrent. Le serveur accepte par conséquent deux valeurs pendant une rotation :

1. Définissez `MY_ORIGIN_SECRET` sur la nouvelle valeur et `MY_ORIGIN_SECRET_PREVIOUS` sur l’ancienne, puis déployez. Les deux valeurs passent.
2. Changez la valeur sur le CDN et attendez que le changement atteigne tous ses emplacements.
3. Retirez `MY_ORIGIN_SECRET_PREVIOUS` et déployez. `my config check` avertit tant qu’il est défini.

Le nom de l’en-tête ne pivote pas de cette manière. Pour le changer, répétez [la mise en place](#set-it-up) en désactivant un instant la vérification du serveur, ou définissez le nouveau nom sur le CDN à côté de l’ancien avant de le déployer.
