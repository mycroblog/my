<!-- source: database-initializing-upgrading-and-recreating b4189f003273 -->
# Base de données : initialisation, mise à niveau et recréation

Comment les opérateurs créent la base de données, la mettent à niveau vers une nouvelle version et, uniquement sur les instances de développement et de test, l’effacent.

Le schéma se trouve dans `schema/`, un répertoire par module avec un fichier par base de données (`schema/<NN>-<module>/sqlite.sql` et `postgres.sql`). Le binaire du serveur contient le schéma de sa version, et
`my schema apply` l’applique : chaque déploiement l’exécute comme tâche ponctuelle avant le serveur, avec l’image du serveur et les
paramètres de base de données du serveur, de sorte que l’image du serveur est la seule image dont une instance a besoin.
`my serve` n’applique jamais le schéma. Démarré sur une base de données qui n’en a pas, le serveur répond `503` sur `/readyz` (voir [Démarrer le serveur](running.md#start-the-server)).

Les données qu’une modification du schéma doit reprendre (une colonne scindée en deux, une valeur déplacée vers une nouvelle table) sont réécrites par la commande
`my db upgrade` de l’image du serveur, qui s’exécute une fois par version après l’application du schéma. L’ordre complet figure dans [Mettre à niveau](#upgrading).

<!-- source: initializing-applying-the-schema 3b0a5f0defa1 -->
## Initialisation (application du schéma)

`my schema apply` n’exécute que les instructions qui amènent la base de données au schéma de sa version : sur une base vide, il crée tout,
sur une base à jour, il ne fait rien. Il peut être exécuté sans risque autant de fois que nécessaire, et chaque mise à niveau l’exécute de nouveau.

<!-- source: with-my-schema-apply df4290f9f1c4 -->
### Avec `my schema apply`

`my schema apply` lit les paramètres de base de données de la configuration du serveur (`database`, `database_url` et `database_dev_url`,
depuis `-config <file>` ou l’environnement ; il n’a besoin d’aucun autre paramètre) et amène la base de données au schéma de la version du binaire :

```sh
my schema apply -dry-run   # print the SQL statements it would run, and change nothing
my schema apply            # run them
```

- Il affiche les instructions SQL qu’il exécute, et les exécute dans une seule transaction : si l’une échoue, aucune ne subsiste.
  Une base de données qui correspond déjà au schéma est laissée inchangée (`ok: the database matches the schema; nothing to apply`).
- Un plan qui supprime une table ou une colonne est refusé, et rien ne s’exécute, sauf si `-allow-destructive` est indiqué. Ne le passez que
  lorsque les notes de version l’exigent, après une sauvegarde. Reconstruire une table SQLite pour la modifier ne compte pas comme une suppression.
- Sur PostgreSQL, il construit le schéma auquel il se compare dans un schéma temporaire de la même base de données et le supprime à la fin,
  y compris en cas d’échec ; le rôle doit avoir le privilège de créer un schéma à cet endroit. S’il ne l’a pas, définissez `database_dev_url` vers une autre base où il l’a
  ([`database_dev_url`](../reference/configuration.md#database_dev_url)). Les exécutions sur une même base de données s’attendent mutuellement : deux exécutions simultanées réussissent donc toutes les deux.
- Il prend en charge `database = sqlite` et `database = postgres`, et se termine avec le code `1` lorsqu’il n’a rien appliqué à cause d’une erreur ou d’une suppression refusée.

À partir de `v0.1.0`, une mise à niveau entre des versions prises en charge ne contient jamais de différence qui fasse perdre des données ; les versions antérieures à `v0.1.0` peuvent supprimer des colonnes
et des tables, et exigent `-allow-destructive`. Lisez le SQL avec `-dry-run` avant chaque mise à niveau (voir [Mettre à niveau](#upgrading)).

<!-- source: through-a-connection-pooler dd56827cda63 -->
### Via un pooler de connexions

`my schema apply` doit atteindre directement la base de données. Il détient un verrou de niveau session, qu’un pooler en mode transaction (le pooling par transaction de PgBouncer, la chaîne de connexion poolée de Neon) ne conserve pas : à travers l’un d’eux, deux exécutions simultanées se sont bloquées au lieu d’échouer. Donnez-lui une URL directe comme `database_url`. `pool_max_conns` et les autres paramètres `pool_*` peuvent y rester, ainsi que dans `database_dev_url` et `stream_postgres_url` : les commandes qui n’ouvrent aucun pool les ignorent. `my db upgrade`, `my db upgrade -check` et la console prennent la même URL.

`my serve` peut utiliser un pooler en mode transaction, à condition que :

- le pooler gère les requêtes préparées au niveau du protocole, que le serveur utilise pour chaque requête : PgBouncer 1.21 ou ultérieur avec `max_prepared_statements` supérieur à 0, ce qu’a la chaîne de connexion poolée de Neon. Derrière un pooler qui ne les gère pas, des requêtes échouent au hasard avec `prepared statement ... does not exist` ou `already exists` : ajoutez `default_query_exec_mode=exec` à l’URL, ce qui les évite au prix d’un aller-retour de plus par requête.
- `stream_postgres_url` contienne une URL directe lorsque `stream = postgres`, puisque `LISTEN` appartient à une session. Un listener sur une URL poolée se connecte puis ne reçoit rien.
- l’URL ne comporte ni `search_path` ni autre paramètre que le pooler refuse à la connexion : PgBouncer ferme une telle connexion, sauf si `ignore_startup_parameters` le liste.

<!-- source: docker-compose 7971a448363a -->
### Docker Compose

Dans [`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml), trois services ponctuels de l’image du serveur s’exécutent dans cet ordre, chacun seulement
après la réussite du précédent, et le serveur (`my`) démarre en dernier :

| Service | Exécute |
| --- | --- |
| `upgrade-check` | `my db upgrade -check` (réussit sur une base vide) |
| `schema` | `my schema apply`, sur `/data/my.db` ou, avec la couche `compose.postgres.yaml`, sur son PostgreSQL ([fichiers Docker Compose](running.md#docker-compose-files)) |
| `upgrade` | `my db upgrade` |

```sh
cd deploy/docker
docker compose up -d             # pulls the image of MY_VERSION; the three one-off services run in order, then my starts
curl -fsS localhost:8080/readyz  # ready when it answers {"status":"ok"}
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- Si l’un des trois échoue, `my` ne démarre pas. Lisez sa sortie avec `docker compose logs upgrade-check` (ou `schema`, `upgrade`).
- La création des administrateurs est décrite dans [Administrateurs et rôles](admin-cli.md).

<!-- source: other-platforms 5f4cdac7fe7a -->
### Autres plateformes

Sans Compose, exécutez `my schema apply` de l’image du serveur comme tâche ponctuelle (un Job ou un init container Kubernetes, un job Cloud Run, etc.),
avec les mêmes paramètres de base de données que le serveur :

```sh
# SQLite: mount the volume that holds the database file at /data
docker run --rm -v my_data:/data -e MY_DATABASE=sqlite -e MY_DATABASE_URL=/data/my.db my schema apply

# PostgreSQL
docker run --rm -e MY_DATABASE=postgres -e MY_DATABASE_URL='postgres://my:...@db.example/my' my schema apply
```

<!-- source: upgrading 4996c9dc9019 -->
## Mettre à niveau

Une mise à niveau vers une nouvelle version exécute toujours ces étapes dans cet ordre, avec l’image du serveur de la **nouvelle** version :

1. **Sauvegardez** la base de données (et, avec le stockage de blobs sur système de fichiers, les médias). Voir [Sauvegarder](#backing-up).
2. **`my db upgrade -check`**. Il ne change rien. Il se termine avec le code `1` lorsque le nouveau schéma supprimerait des données que la base de données n’a pas encore
   reprises, et nomme la version par laquelle passer d’abord ; arrêtez-vous alors et suivez [Sauter des versions](#skipping-releases).
   Une base vide réussit.
3. **`my schema apply`**, comme dans [Initialisation](#initializing-applying-the-schema). Vérifiez d’abord le SQL avec `-dry-run`.
4. **`my db upgrade`**. Il exécute chaque étape de mise à niveau des données de la version que la base de données n’a pas encore enregistrée, reconstruit les
   projections que ces étapes nomment, et enregistre chaque étape. Réexécuté, il ne fait rien. Si une étape échoue, il se termine avec le code `1` ; les étapes terminées
   restent enregistrées, et une nouvelle exécution reprend à partir de celle qui a échoué.
5. **Démarrez le nouveau serveur.** Il répond `503` sur `/readyz` et consigne les étapes manquantes jusqu’à ce que chaque étape de mise à niveau des données de sa version soit enregistrée.

`my serve` n’exécute jamais `my db upgrade -check`, `my schema apply` ni `my db upgrade` ; ce sont des tâches ponctuelles. N’en exécutez qu’une à la fois sur une base de données.

La plupart des versions n’ont aucune étape de mise à niveau des données : `my db upgrade -check` affiche `ok: the database can be upgraded to this release` et
`my db upgrade` affiche `ok: no data upgrade step to run`. Exécutez-les quand même toutes les deux, afin que la procédure ne dépende pas de la lecture de chaque note de version.
La section **Upgrading** des notes de version liste les étapes de chaque version.

<!-- source: the-server-during-an-upgrade ef479675745b -->
### Le serveur pendant une mise à niveau

Les mises à niveau sans interruption ne sont pas garanties. Le serveur précédent continue de fonctionner sur le nouveau schéma pendant le court laps de temps entre les étapes 3 et 5,
mais il ne peut pas participer aux étapes de mise à niveau des données. Passez en [mode maintenance](maintenance.md) pour la mise à niveau : en mode maintenance,
`my serve` n’ouvre aucune connexion à la base de données, de sorte que la sauvegarde est cohérente et que rien n’écrit pendant l’exécution des étapes.
Limitez la fenêtre de maintenance à quelques heures ([combien de temps](maintenance.md#how-long)).

![Les étapes d’une mise à niveau de haut en bas, et ce que répond /readyz à chacune : 200 avec status ok avant, 200 avec status maintenance du mode maintenance jusqu’au démarrage de la nouvelle version, puis de nouveau 200 avec status ok. La liste ci-dessous décrit chaque étape.](images/upgrade.svg)

De haut en bas du schéma :

- **Version précédente.** Le serveur fonctionne normalement, et `/readyz` répond `200` avec `{"status":"ok"}`.
- **Mode maintenance.** Redéployez avec `MY_MAINTENANCE=true`. À partir de là et jusqu’au démarrage de la nouvelle version, `/readyz` répond `200` avec
  `{"status":"maintenance"}` : le répartiteur de charge conserve donc les instances tandis que rien n’atteint la base de données.
- **1. Sauvegardez** la base de données et les médias.
- **2 à 4 : les tâches ponctuelles de la nouvelle image.** `my db upgrade -check` ne change rien ; `my schema apply` s’exécute après que vous avez
  lu son `-dry-run` ; `my db upgrade` exécute les étapes de mise à niveau des données.
- **5. Démarrez la nouvelle version** sans `MY_MAINTENANCE`. `/readyz` répond `200` avec `{"status":"ok"}` dès que chaque étape de mise à niveau des données
  de la version est enregistrée, et `503` tant qu’il en manque une (ou que le schéma n’est pas appliqué).

Avec Docker Compose, `docker compose up` remplace le conteneur en mode maintenance avant l’exécution des tâches ponctuelles : rien ne répond donc
sur `/readyz` pendant les étapes 2 à 4 ([Docker Compose](#docker-compose-1)).

<!-- source: checking-the-sql-with--dry-run ac385f82c7d5 -->
### Vérifier le SQL avec `-dry-run`

Avant que l’étape 3 n’applique quoi que ce soit, exécutez `my schema apply -dry-run` et lisez ce qu’il exécuterait.
Les ajouts (`CREATE TABLE`, `ADD COLUMN`, `CREATE INDEX`) sont ce que contiennent la plupart des versions. Une table de projection peut aussi être reconstruite,
`my db upgrade` la remplissant alors de nouveau à partir de ses événements. Un `DROP` de toute autre chose, ou un changement de type, devrait figurer dans la section
**Breaking changes** des notes de version ; si ce n’est pas le cas, arrêtez-vous et signalez-le avant d’appliquer. `my schema apply` refuse un plan qui supprime une table
ou une colonne, sauf si `-allow-destructive` est indiqué.

<!-- source: docker-compose-1 c4c380ec8daa -->
### Docker Compose

`docker compose up -d` exécute lui-même les étapes 2 à 5, dans l’ordre (voir le tableau dans [Initialisation](#docker-compose)). Si l’une d’elles échoue, le nouveau serveur ne démarre pas.

```sh
cd deploy/docker

# 1. Switch to the maintenance mode. --no-deps keeps the one-off services from running.
MY_MAINTENANCE=true docker compose up -d --no-deps --force-recreate my

# 2. Back up the volume (database and media) while the maintenance mode holds no connection.
docker run --rm -v my_data:/data -v "$PWD":/backup busybox tar czf /backup/my-data-$(date +%Y%m%d%H%M).tar.gz -C /data .

# 3. Set MY_VERSION in .env to the new version (0.2.0, or its digest form 0.2.0@sha256:... from the notes), and pull it.
docker compose pull

# 4. Check the SQL that the schema service will run.
docker compose run --rm --no-deps upgrade-check
docker compose run --rm --no-deps schema schema apply -dry-run

# 5. Upgrade: upgrade-check, schema, upgrade, then my starts again without the maintenance mode.
docker compose up -d
curl -fsS localhost:8080/readyz
```

`docker compose up` remplace le conteneur `my` avant l’exécution des services ponctuels, et ne démarre le nouveau qu’après la réussite des trois.
Si l’un échoue, `docker compose up` le signale et le serveur reste arrêté : placez entre-temps devant lui la [page de maintenance exportée](maintenance.md#docker-compose).
Lisez `docker compose logs <service>`, puis corrigez la cause et exécutez de nouveau `docker compose up -d`, ou [revenez en arrière](#rolling-back).
Le nom de volume `my_data` est le nom du projet Compose (`my`) suivi du nom du volume (`data`).

`MY_VERSION` est la seule chose qu’une mise à niveau modifie, et rien ne la modifie à votre place : il n’y a pas de tag `latest`, et `0.1` ne bouge qu’au sein de `v0.1`. Avec `MY_VERSION=0.1`, l’étape 3 se réduit à `docker compose pull`, qui récupère le dernier correctif.
Pour compiler plutôt depuis les sources, extrayez le tag de la nouvelle version et utilisez `docker compose -f compose.yaml -f compose.build.yaml build` à l’étape 3, et les mêmes options `-f` aux autres étapes.

<!-- source: kubernetes b423ca2c88f9 -->
### Kubernetes

Exécutez les étapes 2 à 4 comme un seul Job avant de déployer la nouvelle image. Les init containers s’exécutent l’un après l’autre, chacun seulement après la réussite du précédent,
et le conteneur principal s’exécute en dernier : un échec arrête donc le Job avant l’étape suivante. Donnez aux conteneurs la même configuration qu’au Deployment.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-upgrade-v0-2-0
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      initContainers:
        - name: upgrade-check
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade, -check]
          envFrom: [{ secretRef: { name: my-config } }]   # the Deployment's settings
        - name: schema
          image: ghcr.io/mycroblog/my:0.2.0
          args: [schema, apply]
          envFrom: [{ secretRef: { name: my-config } }]
      containers:
        - name: upgrade
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade]
          envFrom: [{ secretRef: { name: my-config } }]
```

```sh
kubectl set env deployment/my MY_MAINTENANCE=true && kubectl rollout status deployment/my
# back up, and check the SQL with a one-off Pod of the new image: my schema apply -dry-run
kubectl apply -f my-upgrade-v0-2-0.yaml
kubectl wait --for=condition=complete --timeout=30m job/my-upgrade-v0-2-0
kubectl set image deployment/my my=ghcr.io/mycroblog/my:0.2.0
kubectl set env deployment/my MY_MAINTENANCE-
kubectl rollout status deployment/my
```

Si le Job échoue, `kubectl logs job/my-upgrade-v0-2-0 -c upgrade-check` (ou `-c schema`, `-c upgrade`) montre pourquoi, et le Deployment reste sur l’image précédente.
Avec SQLite sur un volume, montez le même volume dans le Job et arrêtez d’abord les pods du Deployment : SQLite n’admet qu’un seul écrivain.

<!-- source: cloud-run a6c3de3c814f -->
### Cloud Run

Avec les modules OpenTofu, le script de [Déployer sur Cloud Run](deploy-cloud-run.md#upgrading) exécute ces étapes. À la main :

Les jobs Cloud Run exécutent un conteneur chacun : créez donc un job par étape et exécutez-les dans l’ordre avec `--wait`, qui échoue lorsque le job échoue.
Donnez aux jobs les mêmes variables d’environnement et secrets qu’au service.

```sh
gcloud run services update my --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
# back up (see below)

gcloud run jobs deploy my-upgrade-check --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade,-check --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-schema --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=schema,apply --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-upgrade --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade --set-env-vars=... --set-secrets=...

gcloud run jobs execute my-upgrade-check --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-schema --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-upgrade --region=asia-northeast1 --wait &&
  gcloud run deploy my --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 --remove-env-vars=MY_MAINTENANCE
```

Pour vérifier d’abord le SQL, déployez le job de schéma avec `--args=schema,apply,-dry-run`, exécutez-le et lisez ses journaux avant de le redéployer pour l’application.

<!-- source: backing-up b597d3139c2b -->
### Sauvegarder

Faites la sauvegarde après que le serveur a cessé d’écrire (en mode maintenance, ou arrêté) et avant toute étape de la mise à niveau.
[Sauvegarde et restauration](backup.md) couvre chaque élément en détail ; pour une mise à niveau :

- **SQLite** : copiez le fichier de base de données avec ses fichiers `-wal` et `-shm`, tant qu’aucun processus ne l’a ouvert.
  Avec Compose, le `tar` du volume ci-dessus couvre aussi les médias dans `/data/blobs`.
- **PostgreSQL** : `pg_dump` au format personnalisé, que lit `pg_restore`.

  ```sh
  pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
  ```

  La sauvegarde propre à une base de données gérée (une sauvegarde à la demande Cloud SQL, un instantané RDS) convient aussi ; faites-la au même moment.
- **Médias** : le stockage de blobs sur système de fichiers est un répertoire à copier avec la base de données.
- **La KEK** n’est pas dans la base de données. Conservez-la là où elle se trouve déjà ; une base de données restaurée a besoin de la même KEK.

<!-- source: rolling-back 59a40002cd86 -->
### Revenir en arrière

Il n’existe aucune migration inverse. Pour revenir en arrière, restaurez la sauvegarde faite avant la mise à niveau, puis démarrez la version **précédente** :

1. Arrêtez le nouveau serveur (ou passez-le en mode maintenance).
2. Restaurez la sauvegarde : avec Compose, extrayez l’archive dans le volume vidé
   (`docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'rm -rf /data/* && tar xzf /backup/<archive> -C /data'`) ;
   avec PostgreSQL, `pg_restore --clean --if-exists --dbname=<url> <dump>`.
3. Démarrez l’image de la version précédente (avec Compose, remettez `MY_VERSION` à la version précédente, puis `docker compose up -d`).

Tout ce que la nouvelle version a écrit après la mise à niveau est perdu : revenez donc en arrière peu après la mise à niveau, avant que l’instance ne se fédère beaucoup.

<!-- source: skipping-releases 9c7038296a6d -->
### Sauter des versions

Chaque version prend en charge les mises à niveau depuis la même version mineure (par exemple `v0.2.1` vers `v0.2.4`) et depuis la dernière version corrective de la version
mineure précédente (la dernière `v0.1.x` vers `v0.2.y`). Les versions correctives ne suppriment jamais de colonnes ni de tables : au sein d’une version mineure, n’importe quel ordre convient.

Pour franchir plus d’une version mineure, passez successivement par la dernière version corrective de chaque version mineure, en exécutant toute la procédure à chaque fois.
De `v0.1.3` à `v0.4.0`, par exemple : la dernière `v0.1.x`, puis la dernière `v0.2.x`, puis la dernière `v0.3.x`, puis `v0.4.0`.
`my db upgrade -check` arrête un saut qui ferait perdre des données et nomme la version par laquelle passer ; rien n’a changé à ce stade.

<!-- source: checking-and-rebuilding-projections 40b848afd40d -->
## Vérifier et reconstruire les projections

Certaines tables sont des projections : le serveur les dérive des événements et des ressources de leur module (les restrictions d’un compte à partir de ses
actions de modération, par exemple). Le worker vérifie chaque projection une fois par jour et corrige toute différence qu’il trouve. Pour en vérifier ou en corriger une à la main :

```sh
my projection list              # each projection with its module and tables
my projection verify [<name>]   # derive every projection, or the one named, and print the rows that differ; changes nothing
my projection rebuild [<name>]  # derive them the same way and write the difference
```

- `<name>` est une projection ou l’une de ses tables.
- `verify` affiche les lignes que les tables contiennent mais que les sources ne dérivent pas (`-`) et les lignes que les sources dérivent mais qui manquent aux tables (`+`),
  et se termine avec le code `1` si certaines diffèrent.
- `rebuild` écrit au plus 500 lignes par transaction, de sorte que les lecteurs ne voient jamais une projection vidée. Exécutez-le lorsque l’instance est calme :
  une ligne qui change pendant la reconstruction garde sa nouvelle valeur, et un `verify` ultérieur signale toute différence restante.

<!-- source: recreating-erasing-every-row 9ff993b99448 -->
## Recréer (effacer toutes les lignes)

Recréer signifie vider la base de données et appliquer de nouveau le schéma. **À partir de `v0.1.0`, la recréation est réservée aux instances de développement et de test.
Ne recréez jamais une instance publique qui se fédère avec d’autres serveurs.**

Toutes les lignes sont perdues et ne peuvent pas être récupérées. Les clés de compte enveloppées avec la clé de chiffrement de clés (KEK) sont perdues elles aussi : un compte recréé sous le
même nom d’hôte reçoit donc une clé différente et est un Actor différent pour les autres serveurs, et ses relations d’abonnement distantes ne reviennent pas.
Une instance publique qui ne peut pas appliquer une modification du schéma [revient en arrière](#rolling-back) depuis sa sauvegarde à la place.

Il n’existe aucune commande `my` qui efface une base de données : supprimez-la avec les outils de la base de données, puis appliquez de nouveau le schéma.
La base vide n’a ni `host` ni `account_domain` enregistrés : le prochain démarrage enregistre donc les valeurs configurées.

<!-- source: docker-compose-2 eba2326609d1 -->
### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # stop the server
docker run --rm -v my_data:/data busybox rm -f /data/my.db /data/my.db-wal /data/my.db-shm
docker compose up -d                    # the one-off services apply the schema to the empty database, then my starts
```

- Les médias téléversés et traités (`/data/blobs`) restent. Pour les effacer avec la base de données, supprimez le volume nommé avec `docker compose down -v`, puis exécutez
  `docker compose up -d`. Sans `-v`, le volume est conservé.
- Pour en garder d’abord une copie, arrêtez le serveur et copiez `my.db` (avec ses fichiers `-wal` et `-shm`) depuis le volume.

<!-- source: postgresql 679b0e3a5b1f -->
### PostgreSQL

Supprimez la base de données et créez-la de nouveau, vide, avec le même propriétaire (ou avec la console de la base de données gérée), puis exécutez `my schema apply` :

```sh
# stop every server and job that uses the database first
dropdb --maintenance-db="postgres://postgres:...@db.example/postgres" my
createdb --maintenance-db="postgres://postgres:...@db.example/postgres" --owner=my my
my schema apply
```
