<!-- source: backup-and-restore 845c6a2e72dd -->
# Sauvegarde et restauration

Ce qu’il faut sauvegarder, comment réaliser chaque sauvegarde pendant que l’instance continue de fonctionner, comment restaurer une instance à partir de celles-ci et comment répéter une restauration
avant d’en avoir besoin. Les mises à niveau font aussi l’objet d’une sauvegarde ; [Mise à niveau](database.md#upgrading) détaille les étapes qui l’entourent.

<!-- source: what-to-back-up 916727936cca -->
## Ce qu’il faut sauvegarder

![Ce qu’il faut sauvegarder et l’ordre de restauration : la base de données et les médias sont sauvegardés ensemble, la KEK à part, et une restauration remet en place la base de données, les médias et la configuration avec la même KEK avant le démarrage du serveur. La liste ci-dessous dit la même chose.](images/backup-restore.svg)

Du haut du schéma vers le bas :

- **La base de données** (`database_url` : SQLite ou PostgreSQL) est sauvegardée en premier, avec `my db backup` ou `pg_dump`, chaque jour et avant chaque mise à niveau.
- **Les médias** (`blob_fs_dir`, ou le bucket du magasin de blobs `s3` ou `gcs`) sont sauvegardés juste après, sous forme de copie, de versions du bucket ou de réplica.
  Les deux sauvegardes forment une même génération, conservée ensemble, à l’écart du serveur et chiffrée.
- **La KEK** (`MY_KEK` ou `MY_KEK_FILE`) est sauvegardée une fois et après chaque rotation, avec `kek_previous`, et jamais au même endroit que les sauvegardes de la base de données.
  Avec `key_manager = gcpkms` ou `awskms`, la KEK reste dans le KMS : gardez la clé de `kms_key` activée aussi longtemps que vous conservez des sauvegardes qui en ont besoin
  ([Mettre hors service une clé AWS KMS](key-management.md#retire-an-aws-kms-key)).
- **La configuration** (l’environnement, `MY_CONFIG`, `.env` et les secrets) est copiée avec les fichiers de déploiement lorsqu’elle change, sans la KEK.
- **Une restauration** se fait dans un environnement vide, dans cet ordre : la base de données, puis les médias avec les mêmes clés avant le démarrage du serveur,
  puis la configuration avec la même KEK, `kek_previous` et `host` ; ensuite le serveur démarre et vous vérifiez `/readyz`, `my projection verify` et les clés publiques des acteurs
  ([Restauration](#restoring)). Sans la KEK, le serveur ne démarre pas, et avec une autre, la lecture d’un secret échoue avec `unknown key-encryption key`
  ([Sans la bonne KEK](#without-the-right-kek)).
- **Aucune sauvegarde n’est nécessaire** pour le magasin clé-valeur avec `kv = memory` ou `valkey` (caches et limites de débit), les tâches (elles sont dans la base de données),
  les journaux et les métriques, ni l’index d’un moteur de recherche, que `my search rebuild` recrée.

| Quoi | Où cela se trouve | Sauvegarde | Perdu sans sauvegarde |
| --- | --- | --- | --- |
| La base de données | le fichier SQLite (`database_url`) ou la base de données PostgreSQL | [SQLite](#sqlite) ou [PostgreSQL](#postgresql) | tout : comptes, publications, abonnements, paramètres |
| Les médias | le répertoire `blob_fs_dir` du magasin de blobs `fs`, ou le bucket `blob_s3_bucket` du magasin de blobs `s3` ou `blob_gcs_bucket` du magasin de blobs `gcs` | [Médias](#media) ou [Médias dans un bucket](#media-in-a-bucket) | les images, vidéos et fichiers audio téléversés, ainsi que les fichiers distants en cache |
| La KEK | là où vous conservez `MY_KEK` ou le fichier de `MY_KEK_FILE` (`secrets/kek` avec Compose ; jamais dans la base de données) | [La KEK](key-management.md#back-it-up), séparément de la base de données | la clé privée de chaque acteur et avec elle l’identité de l’acteur dans le fédivers, ainsi que les autres secrets que stocke la base de données |
| La configuration | les variables d’environnement, le fichier de configuration (`MY_CONFIG`), les secrets (`MY_VAPID_PRIVATE_KEY`, clés de messagerie et de paiement), `.env` avec Compose | copiez-les avec vos fichiers de déploiement | rien dans les données, mais l’instance ne peut pas démarrer de la même façon ; modifier `host` casse chaque URL fédérée |

Ce qui n’a pas besoin de sauvegarde :

- **Le magasin clé-valeur** (`kv`). Avec `kv = db`, il fait partie de la base de données. Avec `kv = memory` ou `kv = valkey`, il contient des caches, des compteurs de limite de débit
  et des états de courte durée, comme une connexion qui attend son second facteur. Après une restauration avec un Valkey vide, les caches se remplissent à nouveau,
  les limites de débit repartent de zéro, et une connexion ou un appel vocal en cours doit recommencer.
- **La file de tâches** (`queue`). Avec `queue = db` et `queue = cloudtasks`, les tâches font partie de la base de données et reviennent avec elle.
  Les tâches de Cloud Tasks ne portent qu’un signal pour une tâche de la base de données ; après une restauration, le serveur signale à nouveau les tâches échues en quelques minutes.
- **Les journaux et les métriques**, que le serveur ne relit pas.
- **L’index d’un moteur de recherche**, si vous en avez un. `my search rebuild` le recrée à partir de la base de données. Si vous faites des instantanés du moteur, ils contiennent le texte des publications, y compris celles supprimées après l’instantané ; traitez-les donc comme des sauvegardes de la base de données ([Recherche en texte intégral](search.md#backups)).

La clé VAPID de Web Push est un secret de la configuration. Une nouvelle clé interrompt les notifications push vers tous les abonnements existants
([Clé VAPID de Web Push](key-management.md#web-push-vapid-key)) ; conservez-la donc avec la configuration.

<!-- source: how-often-and-where 17e55765e9a0 -->
## À quelle fréquence et où

- **Base de données** : au moins une fois par jour, et avant chaque mise à niveau. Conservez plusieurs générations, par exemple 7 sauvegardes quotidiennes, 4 hebdomadaires et 3 mensuelles,
  afin qu’une erreur remarquée tardivement puisse encore être annulée. Un PostgreSQL géré avec restauration à un instant donné couvre les jours que sa rétention conserve.
- **Médias** : avec la base de données, à chaque fois (voir [L’ordre de la base de données et des médias](#the-order-of-the-database-and-the-media)).
- **KEK et configuration** : lorsqu’elles changent. Une rotation de la KEK est un changement : sauvegardez la nouvelle KEK avant de l’utiliser, et conservez la précédente
  tant que `kek_previous` la liste.
- **Où** : à l’écart du serveur et de son disque, dans un autre domaine de défaillance (une autre machine, une autre région, un autre fournisseur ou compte).
  Une sauvegarde sur le disque du serveur lui-même ne survit pas à la perte de ce disque.
- **Chiffrement** : la sauvegarde de la base de données contient des adresses e-mail, des empreintes de mots de passe et des messages directs ; chiffrez-la au repos
  (un bucket chiffré, ou chiffrez le fichier avec `age` ou `gpg` avant le téléversement). Les secrets qu’elle contient, comme les clés privées des acteurs,
  sont chiffrés avec la KEK, donc **ne conservez jamais la KEK au même endroit que les sauvegardes de la base de données** : quiconque obtient les deux peut lire tous les secrets.
- **Rétention** : une sauvegarde conserve des données que des utilisateurs ont supprimées ou que vous avez effacées sur demande. Supprimez les anciennes générations selon un calendrier que votre politique de
  confidentialité indique.

<!-- source: backing-up 0c38c68033e4 -->
## Sauvegarder

Le serveur peut continuer de fonctionner pendant une sauvegarde. Pour en réaliser une qui corresponde exactement à un instant donné, par exemple avant une mise à niveau,
passez d’abord en [mode maintenance](maintenance.md) : en mode maintenance, `my serve` n’ouvre aucune connexion à la base de données et n’écrit rien.

<!-- source: sqlite 7105c29de4c9 -->
### SQLite

`my db backup <file>` écrit une copie cohérente de la base de données SQLite dans un nouveau fichier pendant que le serveur continue de fonctionner.
La copie contient toutes les écritures validées avant le démarrage de la commande, y compris celles qui sont encore dans le fichier `-wal`, et aucune après.
C’est un fichier unique, plus petit que l’original car les pages libres sont omises. La commande refuse un fichier qui existe déjà,
et ne fonctionne qu’avec `database = sqlite`.

```sh
my db backup /backup/my-$(date +%Y%m%d%H%M).db
```

Avec Docker Compose, exécutez-la dans le conteneur du serveur. La copie est écrite à l’intérieur du conteneur ; écrivez-la donc dans le volume de données
et sortez-la de là :

```sh
cd deploy/docker
docker compose exec -T my my db backup /data/backup.db
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'mv /data/backup.db /backup/my-$(date +%Y%m%d%H%M).db'
```

Ne copiez pas le fichier de la base de données avec `cp` ou `tar` pendant que le serveur fonctionne : le fichier seul ne contient pas les écritures encore présentes dans le fichier `-wal`,
et une copie réalisée au milieu d’une écriture peut être corrompue. Une copie simple convient lorsqu’aucun processus n’a la base de données ouverte
(serveur arrêté, ou en mode maintenance) ; copiez alors aussi les fichiers `-wal` et `-shm`.

<!-- source: postgresql 45b14a5a4970 -->
### PostgreSQL

Utilisez `pg_dump` au format personnalisé, que `pg_restore` lit. Il lit un seul instantané, de sorte que le serveur peut continuer de fonctionner.

```sh
pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
```

Avec la couche Compose `compose.postgres.yaml`, exécutez-le dans le conteneur de la base de données (nommez la couche avec `-f` ou dans `COMPOSE_FILE`) :

```sh
cd deploy/docker
docker compose exec -T postgres pg_dump --format=custom -U my my > my-$(date +%Y%m%d%H%M).dump
```

Utilisez le `pg_dump` de la même version majeure que le serveur ou d’une version plus récente.

Un PostgreSQL géré dispose de ses propres sauvegardes, qui sont le meilleur choix pour une grande base de données :

- **Cloud SQL** et **Amazon RDS** réalisent des sauvegardes quotidiennes automatiques et, avec la restauration à un instant donné activée, restaurent à n’importe quelle seconde
  de la période de rétention. Activez les deux, réglez la rétention pour couvrir les générations souhaitées, et réalisez une sauvegarde à la demande
  (une sauvegarde à la demande de Cloud SQL, un instantané RDS) avant chaque mise à niveau.
- Leur restauration crée une nouvelle instance (ou en écrase une) : faites ensuite pointer `database_url` vers elle.
- Ces sauvegardes restent dans le compte du fournisseur. Exportez de temps en temps un `pg_dump` vers un emplacement extérieur à celui-ci également,
  afin que la perte du compte n’entraîne pas celle des données.

<!-- source: media a2445f4d5501 -->
### Médias

Le magasin de blobs `fs` conserve chaque fichier sous `blob_fs_dir` (`/data/blobs` avec Compose) : les téléversements dans `objects/`, et les fichiers en cours d’écriture dans `tmp/`.
Copiez le répertoire, à l’exception de `tmp/`, avec n’importe quel outil qui copie des fichiers (`tar`, `rsync`, un instantané du volume ou du disque).
Un fichier est écrit sous `tmp/` puis renommé dans `objects/` en une seule étape ; une copie réalisée pendant que le serveur fonctionne ne contient donc jamais la moitié d’un fichier dans `objects/`.

```sh
docker run --rm -v my_data:/data -v "$PWD":/backup busybox \
  tar czf /backup/blobs-$(date +%Y%m%d%H%M).tar.gz -C /data --exclude=blobs/tmp blobs
```

`rsync` vers l’hôte de sauvegarde garde la copie petite après la première fois, puisque les fichiers médias ne changent jamais une fois écrits.

<!-- source: media-in-a-bucket 882f3a0f9a2b -->
### Médias dans un bucket

Les magasins de blobs `s3` et `gcs` conservent chaque fichier sous forme d’objet du bucket `blob_s3_bucket` ou `blob_gcs_bucket`, nommé par sa clé : les médias traités sous `media/`,
les téléversements tels que reçus sous `uploads/`, et les fichiers distants en cache sous `remote/`. Le bucket reste privé : le serveur lit les
fichiers et les sert sous `/media/`. Un objet n’apparaît qu’une fois écrit entièrement ; une copie réalisée pendant que le serveur fonctionne ne contient donc jamais
la moitié d’un fichier. Protégez le bucket en trois couches, de la moins chère à la plus sûre.

**Gestion des versions.** Activez la gestion des versions des objets du bucket lorsque le service l’offre (Amazon S3, Google Cloud Storage, MinIO et d’autres).
Un objet supprimé ou écrasé reste alors comme version non courante, et le bucket peut revenir à l’instant d’une sauvegarde de la base de données.

```sh
aws s3api put-bucket-versioning --bucket my-media --versioning-configuration Status=Enabled
```

**Règles de cycle de vie.** Avec la gestion des versions, chaque fichier supprimé conserve son coût de stockage jusqu’à ce qu’une règle le retire. Ajoutez une règle qui supprime les versions non courantes
après l’âge de votre plus ancienne sauvegarde de la base de données (35 jours pour les générations de [À quelle fréquence et où](#how-often-and-where) sans les mensuelles),
afin que chaque sauvegarde de la base de données que vous conservez retrouve ses fichiers et qu’un fichier supprimé par un utilisateur ne reste pas plus longtemps que ne le dit votre politique de confidentialité.
Ajoutez une règle qui abandonne les téléversements multipart incomplets au bout d’un jour : le serveur abandonne lui-même un téléversement échoué d’un gros fichier, mais un processus
qui s’arrête au milieu de l’un d’eux laisse ses parties derrière lui.

```json
{
  "Rules": [
    {
      "ID": "noncurrent-versions",
      "Status": "Enabled",
      "Filter": {},
      "NoncurrentVersionExpiration": { "NoncurrentDays": 35 },
      "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 1 }
    }
  ]
}
```

```sh
aws s3api put-bucket-lifecycle-configuration --bucket my-media --lifecycle-configuration file://lifecycle.json
```

**Une copie dans un autre compte ou une autre région.** La gestion des versions protège contre une erreur, pas contre la perte du bucket ou du compte qui le possède,
et les identifiants du serveur peuvent supprimer des objets. Conservez une copie que ces identifiants ne peuvent pas atteindre :

- **Réplication.** Amazon S3 réplique un bucket versionné vers un bucket d’une autre région ou d’un autre compte (Cross-Region ou Same-Region Replication,
  avec la destination appartenant à l’autre compte). Activez aussi la gestion des versions pour la destination, et ne répliquez pas les suppressions
  (laissez la réplication des marqueurs de suppression désactivée), afin qu’une suppression sur l’instance n’atteigne pas la copie.
  D’autres services ont leur propre réplication, comme les buckets bi-régionaux de Cloud Storage et la réplication de buckets de MinIO.
- **Une copie planifiée.** Là où il n’y a pas de réplication, ou pour copier vers un autre fournisseur, copiez le bucket selon le calendrier de la sauvegarde de la base de données,
  juste après celle-ci, avec des identifiants qui peuvent seulement lire le bucket de l’instance et seulement écrire dans le bucket de sauvegarde :

  ```sh
  rclone copy --checksum source:my-media backup:my-media-backup
  ```

  `rclone copy` et `aws s3 sync` sans `--delete` ajoutent de nouveaux objets et n’en retirent jamais un de la copie. Les fichiers ne changent jamais une fois écrits,
  de sorte qu’après la première exécution, chaque copie ne transfère que les nouveaux. Supprimez les anciens objets de la copie selon le calendrier de votre politique de rétention,
  ou activez-y aussi la gestion des versions et une règle de cycle de vie.

Les identifiants restent hors de la copie : le bucket ne contient aucun secret, mais la configuration nomme la clé dans `blob_s3_access_key_id`
et `blob_s3_secret_access_key`, qui vont avec la configuration. Le magasin de blobs `gcs` ne conserve aucune clé : il utilise le compte de service
de la machine ou de Cloud Run.

<!-- source: the-order-of-the-database-and-the-media 52b56171d6d9 -->
### L’ordre de la base de données et des médias

Sauvegardez **la base de données d’abord, puis les médias**, et conservez les deux ensemble comme une seule génération. Pour un bucket, la copie ou la réplication
qui s’exécute après la sauvegarde de la base de données est la sauvegarde des médias ; avec la gestion des versions, ce sont les versions à l’instant de la sauvegarde de la base de données.

La base de données nomme les fichiers médias qu’elle utilise, et la copie des médias est réalisée un instant plus tard. Pendant cet instant :

- De nouveaux téléversements peuvent arriver dans la copie des médias sans ligne dans la copie de la base de données. Ce sont des fichiers que rien n’utilise ; ils prennent de la place sans faire de mal.
- Un fichier supprimé après la sauvegarde de la base de données (le média d’une publication supprimée, un téléversement qui n’a jamais été attaché) est dans la copie de la base de données mais pas dans la copie des médias.
  Après une restauration, ce fichier répond `404`, et la publication qui l’affichait montre une image manquante.

L’autre ordre est pire : chaque fichier téléversé entre la sauvegarde des médias et celle de la base de données manquerait après une restauration,
y compris les médias des nouvelles publications. Réaliser les deux en mode maintenance évite l’un et l’autre écart.

<!-- source: the-kek-and-the-configuration b3d65ec5edb5 -->
### La KEK et la configuration

La KEK n’est pas dans la base de données. Sauvegardez-la une fois, séparément des sauvegardes de la base de données, comme le décrit [Gérer la clé de chiffrement de clés](key-management.md#back-it-up),
et à nouveau après chaque rotation. Conservez une copie de la configuration (l’environnement, le fichier de configuration, `.env` et les fichiers de secrets)
avec vos fichiers de déploiement, sans la KEK.

<!-- source: restoring 4db94bba2bc4 -->
## Restauration

Une sauvegarde contient le `host` et le `account_domain` de son instance. Démarrez la base de données restaurée avec les mêmes valeurs : avec d’autres, le serveur refuse de démarrer.

Restaurez dans un environnement vide (une nouvelle machine, de nouveaux volumes ou une nouvelle base de données), dans cet ordre : la base de données, les médias (le répertoire ou le bucket),
puis la configuration avec **la même KEK** (et le même `kek_previous`) que celle utilisée lors de la sauvegarde. Démarrez ensuite le serveur comme d’habitude.

<!-- source: sqlite-1 62820e765e7f -->
### SQLite

Arrêtez le serveur, mettez le fichier de sauvegarde à la place du fichier de base de données, et supprimez les anciens fichiers `-wal` et `-shm` s’il en reste.
Avec Docker Compose, dans le volume `my_data` (un volume existant, ou un nouveau sur une autre machine) :

```sh
cd deploy/docker
docker compose down          # keeps the volume; on a new machine, `docker compose up --no-start` creates it
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c '
  rm -rf /data/my.db /data/my.db-wal /data/my.db-shm /data/blobs &&
  cp /backup/my-202601010000.db /data/my.db &&
  tar xzf /backup/blobs-202601010000.tar.gz -C /data &&
  chown -R 65532:65532 /data'
docker compose up -d
```

L’image du serveur s’exécute sous l’utilisateur `65532`, qui doit posséder les fichiers du volume (le `chown` ci-dessus).

<!-- source: postgresql-1 4a3134816771 -->
### PostgreSQL

Restaurez dans une nouvelle base de données vide, préparée comme pour une nouvelle instance, mais sans appliquer le schéma : le dump le contient.

1. Créez le rôle et la base de données.
2. Restaurez le dump. En superutilisateur, les tables restaurées conservent leur propriétaire (le rôle doit exister) ; en tant que propriétaire de la base de données, ajoutez `--no-owner`.

   ```sh
   pg_restore --exit-on-error --dbname="postgres://postgres:...@db.example/my" my-202601010000.dump
   ```

3. Avec la sauvegarde propre à une base de données gérée, restaurez-la plutôt à la manière du fournisseur, et faites pointer `database_url` vers l’instance restaurée.

Avec la couche Compose `compose.postgres.yaml`, démarrez uniquement la base de données, sur un nouveau volume, et restaurez dedans. Le conteneur a créé le rôle et la base de données :

```sh
cd deploy/docker
docker compose up -d --wait postgres
docker compose exec -T postgres pg_restore --exit-on-error --no-owner -U my -d my < my-202601010000.dump
docker compose up -d
```

Pour restaurer par-dessus une base de données existante (annuler une mise à niveau), utilisez `pg_restore --clean --if-exists`, comme le montre [Revenir en arrière](database.md#rolling-back).

<!-- source: media-1 4292939246df -->
### Médias

Extrayez la copie des médias dans `blob_fs_dir` avant le démarrage du serveur, avec pour propriétaire l’utilisateur sous lequel le serveur s’exécute.

<!-- source: media-in-a-bucket-1 2e8d8c7703b4 -->
### Médias dans un bucket

Restaurez les objets dans un bucket vide avant le démarrage du serveur, puis définissez `blob_s3_bucket` (ainsi que le point de terminaison, la région et les identifiants) ou `blob_gcs_bucket` sur celui-ci.
Les clés doivent rester les mêmes : la base de données nomme chaque fichier par sa clé.

- À partir d’une copie dans un autre bucket, recopiez-la :

  ```sh
  rclone copy --checksum backup:my-media-backup target:my-media-restored
  ```

- À partir des versions du bucket de l’instance elle-même, copiez les objets tels qu’ils étaient à l’instant de la sauvegarde de la base de données vers un nouveau bucket.
  `rclone` lit les versions de cet instant avec `--s3-version-at` :

  ```sh
  rclone copy --s3-version-at "2026-01-01 00:00:00" source:my-media target:my-media-restored
  ```

- À partir d’un réplica, faites pointer `blob_s3_bucket` ou `blob_gcs_bucket` vers le réplica, ou recopiez-le vers un bucket du compte d’origine.

Les identifiants du serveur doivent pouvoir lire, écrire et supprimer des objets du bucket restauré (`s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`
et `s3:AbortMultipartUpload` sur Amazon S3 ; le rôle `roles/storage.objectUser` sur le bucket dans Google Cloud Storage).

<!-- source: starting-and-checking f656b59cc0ed -->
### Démarrage et vérification

Démarrez le serveur de la manière habituelle. Avec Docker Compose, `docker compose up -d` exécute d’abord `my db upgrade -check`, le schéma, puis `my db upgrade` ;
sur une sauvegarde de la même version, ils ne changent rien. Vérifiez ensuite :

1. `/readyz` répond `200`, et le serveur n’enregistre aucune erreur `unknown key-encryption key`.
2. `my projection verify` ne signale aucune différence ([Vérifier et reconstruire les projections](database.md#checking-and-rebuilding-projections)).
3. Vous pouvez vous connecter, et les dernières publications et leurs médias s’affichent.
4. **Les clés des acteurs n’ont pas changé** : le `publicKey.publicKeyPem` d’un acteur
   (`curl -H 'Accept: application/activity+json' https://<host>/ap/actors/<id>`) est le même qu’avant la restauration.
   Les autres serveurs vérifient les signatures de l’instance avec cette clé ; une clé inchangée signifie donc que la fédération reprend là où elle s’était arrêtée.
5. Une publication, un abonnement ou un désabonnement parvient à un autre serveur (un compte de test sur une autre instance le voit).

Les écritures réalisées après la sauvegarde sont perdues : les publications, abonnements et téléversements depuis lors n’existent plus sur cette instance, alors que d’autres serveurs peuvent encore avoir
les activités qu’ils ont reçues. Cela ne peut pas être réparé d’ici ; restaurez à partir de la sauvegarde la plus récente dont vous disposez.

Avec un moteur de recherche, exécutez `my search rebuild` après la restauration, sans `--keep`, afin que les publications plus récentes que la sauvegarde quittent l’index ([Recherche en texte intégral](search.md#backups)).

<!-- source: without-the-right-kek 2a5031eadf18 -->
### Sans la bonne KEK

- **Pas de KEK** : le serveur ne démarre pas. `my serve` et les autres commandes `my` qui lisent la configuration s’arrêtent avec
  `The required setting MY_KEK is not set.` et `kek: required when key_manager is local`.
- **Une autre KEK** : le serveur démarre et répond à `/readyz`, mais tout ce qui lit un secret chiffré avec l’ancienne KEK échoue avec
  `unknown key-encryption key` et l’ID de clé de l’ancienne KEK (`local key manager: key "local:…" is not configured`) : les envois vers d’autres serveurs et les récupérations signées (les clés privées des acteurs),
  les connexions avec authentification à deux facteurs, les signatures de webhooks et les fournisseurs de connexion externes. Les URL du proxy de médias émises auparavant cessent aussi de fonctionner.
  Les mots de passe et les publications ne sont pas chiffrés avec la KEK, de sorte que la connexion par mot de passe et la lecture des publications fonctionnent toujours, ce qui peut masquer le problème au début :
  surveillez le journal pour détecter l’erreur.
- Trouvez la KEK dont l’ID de clé est nommé par l’erreur, définissez-la comme `kek` (ou dans `kek_previous`) et redémarrez. Il n’existe aucun moyen de récupérer un secret sans sa KEK ;
  si la KEK est définitivement perdue, les clés des acteurs doivent être régénérées, et les autres serveurs voient chaque acteur comme une nouvelle clé pour le même compte.

<!-- source: recovery-drill f9adfe0e0d28 -->
## Exercice de reprise

On ne sait qu’une sauvegarde fonctionne qu’une fois qu’elle a été restaurée. Répétez une restauration avant que l’instance devienne publique, et à nouveau après un changement dans la façon dont les
sauvegardes sont réalisées (au moins tous les quelques mois) :

1. Réalisez une sauvegarde de l’instance en production comme ci-dessus : la base de données, puis les médias.
2. Sur une autre machine ou dans un autre projet Compose, restaurez-la avec la KEK et la configuration issues de l’endroit où vous les conservez
   (et non copiées depuis le serveur en production), et avec `host` inchangé pour que les URL des acteurs correspondent.
3. Ne laissez pas la copie se fédérer : gardez-la hors d’internet, ou activez le mode maintenance une fois les vérifications ci-dessous réussies.
4. Parcourez [Démarrage et vérification](#starting-and-checking) : connexion, publications et médias, et la `publicKey` de quelques acteurs comparée à celle de l’instance en production.
5. Démarrez-la une fois sans la KEK et une fois avec une autre, et vérifiez qu’elle échoue comme le décrit [Sans la bonne KEK](#without-the-right-kek).
6. Notez la durée de chaque étape : c’est la durée pendant laquelle une vraie restauration maintient l’instance à l’arrêt.
