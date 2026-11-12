<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# Déployer avec Docker Compose

Comment faire tourner `my` sur une seule machine avec Docker Compose : la configuration la plus petite, un processus serveur sur SQLite avec les fichiers médias dans un volume nommé. PostgreSQL, le stockage objet et TLS sont des fichiers à superposer plus tard ([fichiers Docker Compose](running.md#docker-compose-files)).

Vous avez besoin de Docker et de Docker Compose v2.

<!-- source: start-the-server ba6cb34daaa0 -->
## Démarrer le serveur

Les fichiers Compose se trouvent dans [`deploy/docker`](../../deploy/docker/compose.yaml) des sources. Prenez-les sur le tag de la version que vous exécutez, afin qu’ils correspondent à son image :

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose récupère l’image `ghcr.io/mycroblog/my` de `MY_VERSION`. Il n’existe pas de tag `latest`, la version doit donc être nommée : une version peut nécessiter des étapes qu’une mise à jour silencieuse ignorerait ([les tags de l’image](running.md#get-the-image)). Pour construire plutôt l’image à partir du checkout, ajoutez la surcharge : `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- Le service `schema` exécute `my schema apply` de l’image du serveur, qui amène la base de données d’un volume nommé au schéma de cette version, puis se termine ; le serveur (`role=all`) démarre une fois qu’il a réussi. Le schéma est appliqué de nouveau à chaque `up` : après une mise à jour, vous démarrez donc le serveur de la même façon.
- Le serveur écoute sur `127.0.0.1:8080` (le port est `MY_HTTP_PORT` dans `.env`). Il est prêt quand `curl localhost:8080/readyz` renvoie `{"status":"ok"}`.
- La clé de chiffrement de clés (KEK) parvient au serveur depuis `secrets/kek` comme secret Docker `/run/secrets/my_kek`, jamais comme variable d’environnement ([gérer la clé de chiffrement de clés](key-management.md)).
- Tous les autres paramètres de la [référence de la configuration](../reference/configuration.md) se définissent dans `.env`, dont le modèle les décrit.

<!-- source: create-an-administrator c3d30824f7c0 -->
## Créer un administrateur

Créez un administrateur, puis recherchez le compte avec WebFinger et récupérez son acteur. Le mot de passe est lu sur la première ligne de l’entrée standard.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## Se connecter depuis un navigateur

Avec le couple `MY_HOST=localhost:8080` et `MY_WEB_ORIGIN=http://localhost:8080` de `example.env`, la connexion, l’API client et les médias fonctionnent en HTTP simple sur `http://localhost:8080` (les identifiants ActivityPub restent `https://localhost:8080/...`). Si vous changez `MY_HTTP_PORT`, changez le port de `MY_HOST` et de `MY_WEB_ORIGIN` en conséquence.

1. Ouvrez `http://localhost:8080/login` dans un navigateur (ou appuyez sur **Se connecter** dans la barre supérieure du client web) ; cela ouvre l’écran de connexion du serveur.
2. Sur l’écran de connexion, saisissez l’adresse e-mail et le mot de passe de l’administrateur que vous venez de créer.
3. Vous revenez dans le client web, sur votre fil d’accueil.

- Le cookie de connexion est `Secure`. Chrome et Firefox l’acceptent sur `http://localhost` ; Safari (WebKit) ne le fait pas, utilisez donc Chrome ou Firefox en local.
- Les liens des hashtags et des mentions dans le corps des publications utilisent `https://`, puisque ce sont les URL fédérées.

<!-- source: going-public be2475df6407 -->
## Passer en public

- Définissez `MY_HOST` dans `.env` sur votre vrai domaine, supprimez `MY_WEB_ORIGIN` (il vaut par défaut `https://{MY_HOST}`) et terminez TLS devant le serveur : la couche `compose.caddy.yaml` exécute Caddy pour cela ([fichiers Docker Compose](running.md#docker-compose-files)). `MY_HOST` est enregistré dans la base de données au premier démarrage et ne peut plus être changé ensuite.
- Derrière un reverse proxy de votre choix, définissez `MY_TRUSTED_PROXIES` dans `.env` afin que les limites de débit, comme celle de la connexion, s’appliquent par IP client plutôt qu’au proxy ([`trusted_proxies`](../reference/configuration.md#trusted_proxies)).
- Avant d’ouvrir le serveur aux utilisateurs, parcourez la [liste de contrôle de production](production-checklist.md).
- `docker compose down` arrête la pile. `down -v` supprime aussi les volumes de données.

Pour chaque paramètre, consultez la [référence de la configuration](../reference/configuration.md) ; pour l’exploitation au quotidien, consultez [l’exécution du serveur](running.md).
