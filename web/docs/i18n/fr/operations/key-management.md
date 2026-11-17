<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# Gérer la clé de chiffrement de clés (KEK)

Le serveur chiffre les secrets qu’il conserve dans la base de données avec une clé de chiffrement de clés (KEK) qu’il n’y stocke jamais.
Cette page explique comment générer, stocker, sauvegarder et renouveler la KEK, comment la conserver dans un KMS à la place, et que faire si elle est perdue ou divulguée.
La [référence de configuration](../reference/configuration.md) liste tous les paramètres.

<!-- source: how-it-works 37074c6237d0 -->
## Fonctionnement

![La hiérarchie des clés : la clé de chiffrement de clés, conservée là où key_manager l’indique et jamais dans la base de données, enveloppe une clé de chiffrement de données créée pour chaque secret, qui chiffre le secret ; la base de données contient l’ID de clé, la clé de chiffrement de données enveloppée et le secret chiffré dans cinq colonnes scellées. La liste ci-dessous décrit chaque couche.](images/key-hierarchy.svg)

- Les secrets stockés dans la base de données, comme les clés privées des acteurs, sont chiffrés avec AES-256-GCM à l’aide d’une clé de chiffrement de données (DEK) générée pour chaque secret.
  La DEK est enveloppée avec une **clé de chiffrement de clés (KEK)** et stockée à côté du texte chiffré (chiffrement par enveloppe).
- La KEK n’est pas stockée dans la base de données ; un export de la base de données divulgué ne suffit donc pas à exposer les secrets.
- Chaque texte chiffré porte l’**ID de clé** de la KEK qui a enveloppé sa DEK. Avec `key_manager = local`, l’ID de clé est `local:` suivi de 16 chiffres hexadécimaux dérivés de la KEK,
  de sorte que la même KEK donne toujours le même ID. Vous n’attribuez jamais d’ID.
- Un texte chiffré est lié à l’ID de ligne et au nom de colonne où il est stocké. Le copier dans une autre ligne ou colonne le rend indéchiffrable.
- Les colonnes scellées, nommées comme les affiche `my kek status`, sont `local_account_private_keys.private_key_enc` (clés privées des acteurs),
  `totp_enrollments.secret_enc` (secrets d’authentification à deux facteurs), `login_provider_secrets.sealed_secret` (secrets client des fournisseurs de connexion),
  `webhook_secrets.sealed_secret` (secrets de signature des webhooks) et `instance_secrets.sealed_secret` (la clé de signature du proxy de médias).
- L’état d’une connexion par clé d’accès ou par un fournisseur externe est scellé de la même façon mais n’est pas stocké : le navigateur le conserve pendant 30 minutes au plus.
- La clé de signature (HMAC-SHA256) des URL du proxy de médias est une clé aléatoire que le serveur génère à son premier démarrage et stocke dans la base de données, chiffrée comme les autres secrets.
  Elle ne change pas lorsque vous renouvelez la KEK ; les URL du proxy de médias continuent donc de fonctionner d’une rotation à l’autre, et `my kek rewrap` la ré-enveloppe avec les autres secrets.
- `key_manager` vaut `local` (la valeur par défaut : il utilise le paramètre `kek`), `gcpkms` (la KEK reste dans Google Cloud KMS ; voir [Google Cloud KMS](#google-cloud-kms)),
  `awskms` (la KEK reste dans AWS KMS ; voir [AWS KMS](#aws-kms)) ou `plugin` (la KEK reste auprès d’un programme à vous ; voir [Un plugin](#a-plugin)).

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## Que se passe-t-il si vous perdez la KEK

Sans la KEK, restaurer la base de données ne vous permet pas de déchiffrer les clés privées des acteurs. Une fois une clé privée perdue, les autres serveurs ne peuvent plus vérifier les signatures de cet acteur,
et **l’identité de l’acteur dans le fédivers est rompue** (les relations d’abonnement ne peuvent pas être reportées). Traitez la KEK avec le même soin que la base de données.

![Ce qui est perdu avec la KEK : après une restauration sans elle, tout ce qui n’est pas scellé reste lisible, chaque valeur scellée ne peut pas être ouverte, et les acteurs perdent leur identité. Le paragraphe ci-dessous dit la même chose.](images/kek-loss.svg)

Ce qui n’est pas scellé reste lisible : les publications, les comptes, les abonnements et le reste de la base de données, ainsi que les fichiers médias dans le stockage.
Chaque valeur scellée échoue avec une erreur `unknown key-encryption key` : les clés privées des acteurs, les secrets d’authentification à deux facteurs,
les secrets client des fournisseurs de connexion, les secrets de signature des webhooks et la clé de signature du proxy de médias.
La même chose se produit lorsqu’une autre KEK est définie à la place de celle qui a enveloppé les secrets, ou lorsque la clé KMS ou la clé du plugin qui les a enveloppés est désactivée ou détruite.

<!-- source: generate-a-kek 44b338b7440b -->
## Générer une KEK

Encodez 32 octets aléatoires en base64 standard.

```sh
openssl rand -base64 32
```

Là où `openssl` n’est pas disponible, `head -c 32 /dev/urandom | base64` fonctionne aussi. Ne laissez pas la valeur générée dans l’historique de votre terminal ni dans des tickets.

<!-- source: store-and-deliver-it 9300565c2a3c -->
## La stocker et la fournir

La KEK est un paramètre secret ; elle ne peut donc être transmise que par la variable d’environnement `MY_KEK`, ou par `MY_KEK_FILE`, le chemin d’un fichier contenant la valeur. La placer dans le fichier de configuration (TOML) empêche le démarrage.

| Cible | Exemple de façon de la fournir |
| --- | --- |
| Auto-hébergé (binaire / systemd) | Placez-la dans un fichier que seul le propriétaire peut lire (`chmod 600`) et faites pointer `MY_KEK_FILE` vers lui |
| Docker / Compose | Montez un secret Docker sur `/run/secrets/my_kek` et définissez `MY_KEK_FILE=/run/secrets/my_kek`. Les fichiers de `deploy/docker` le font à partir de `secrets/kek` ([Fichiers Docker Compose](running.md#docker-compose-files)) : pour renouveler, remplacez ce fichier et définissez `MY_KEK_PREVIOUS` dans `.env` |
| Kubernetes / microk8s | Montez un Secret sous forme de fichier et faites pointer `MY_KEK_FILE` vers lui |
| Cloud Run | Exposez un secret de Secret Manager sous forme de fichier ou de variable d’environnement |
| AWS ECS | Transmettez-la depuis Secrets Manager ou SSM Parameter Store (SecureString) |

- Donnez à chaque processus, comme web et worker, la même KEK.
- La KEK n’apparaît jamais dans les journaux ni dans les messages d’erreur (seul l’ID de clé y figure).
- `my config check` valide le format de la KEK (base64 de 32 octets) et affiche la valeur sous la forme `(redacted)`. Dans le profil `production`, il échoue aussi sur une KEK
  qui n’est pas aléatoire, comme des zéros uniquement ou un octet répété, et `my serve` journalise un avertissement à son sujet (voir la [liste de contrôle de production](production-checklist.md)).

<!-- source: back-it-up 29e7bdb3b114 -->
## La sauvegarder

**Sauvegardez la base de données et la KEK, séparément.**

- Conservez la KEK ailleurs que là où se trouvent les sauvegardes de la base de données (un gestionnaire de mots de passe, un support hors ligne, un coffre de secrets dans un autre compte, etc.).
  Si les deux sont au même endroit, une fuite de cet endroit rend le chiffrement de la base de données inutile.
- Pendant une rotation, incluez les clés de `kek_previous` dans la sauvegarde aussi longtemps que des textes chiffrés enveloppés avec elles restent dans la base de données.
- Incluez la KEK dans votre procédure de restauration et testez régulièrement les restaurations ([Sauvegarde et restauration](backup.md#recovery-drill)). Après la restauration d’une base de données, définissez la KEK (et `kek_previous`) qui était utilisée au moment de la sauvegarde avec les mêmes valeurs.
  Avec une KEK différente, tout ce qui lit un secret échoue avec une erreur `unknown key-encryption key` qui nomme l’ID de clé.

<!-- source: rotate-the-kek eb4af3189053 -->
## Renouveler la KEK

Une KEK dans `kek_previous` n’est utilisée que pour le déchiffrement, et les nouveaux secrets sont toujours chiffrés avec `kek`.
Pour qu’un déploiement progressif, où d’anciens et de nouveaux processus s’exécutent côte à côte, ne produise jamais un texte chiffré qu’un processus ne peut pas lire, procédez en deux temps.

![Une rotation de la KEK locale en cinq étapes, avec les valeurs de kek et kek_previous après chacune : la nouvelle KEK va d’abord dans kek_previous, puis échange sa place avec l’ancienne, my kek rewrap déplace les secrets stockés, et l’ancienne KEK quitte la configuration. Les étapes ci-dessous disent la même chose.](images/kek-rotation.svg)

1. Générez une nouvelle KEK et sauvegardez-la.
2. Redémarrez chaque processus avec `MY_KEK=<old KEK>` et `MY_KEK_PREVIOUS=<new KEK>`. Chaque processus peut maintenant déchiffrer avec la nouvelle KEK.
3. Redémarrez chaque processus avec `MY_KEK=<new KEK>` et `MY_KEK_PREVIOUS=<old KEK>`. À partir de maintenant, les secrets sont enveloppés avec la nouvelle KEK, et les textes chiffrés de l’ancienne KEK restent lisibles.
4. Ré-enveloppez les secrets stockés avec la nouvelle KEK : exécutez `my kek rewrap` une fois, avec la même configuration qu’à l’étape 3 (voir [Ré-envelopper les secrets stockés](#re-wrap-the-stored-secrets)).
   Lorsqu’elle se termine avec le code 0, plus aucun secret de la base de données n’a besoin de l’ancienne KEK.
5. Attendez au moins 30 minutes après l’étape 3, afin que les jetons de connexion de courte durée émis avec l’ancienne KEK aient expiré, puis redémarrez chaque processus avec `MY_KEK=<new KEK>` et sans `kek_previous`.
   Conservez la sauvegarde de l’ancienne KEK aussi longtemps que vous conservez des sauvegardes de la base de données réalisées avant l’étape 4 : en restaurer une en a de nouveau besoin.

`kek_previous` accepte plusieurs clés séparées par des virgules. Lister deux fois la même clé, ou lister la même clé que `kek`, empêche le démarrage.

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### Ré-envelopper les secrets stockés

`my kek rewrap` ne remplace que la clé de chiffrement de données enveloppée de chaque secret et conserve son texte chiffré ; il n’écrit ni n’affiche jamais un secret.

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` affiche une ligne par colonne et par ID de clé, marque l’ID de clé de `kek` avec `(current)`, et se termine en indiquant si un secret a encore besoin d’une clé de `kek_previous`.
  Un ID de clé qui n’est ni `kek` ni dans `kek_previous` signifie qu’une KEK manque dans la configuration.
- `my kek rewrap` lit les secrets par lots de 100 et écrit chaque lot dans une courte transaction. Il affiche, par colonne, combien de secrets il a ré-enveloppés et combien étaient déjà enveloppés avec `kek`.
  Il peut s’exécuter pendant que le serveur fonctionne, et on peut l’arrêter ou le relancer sans risque à tout moment : il ignore les secrets déjà enveloppés avec `kek`, et un secret que le serveur a réécrit entre-temps est laissé tel que le serveur l’a écrit.
- Un secret qui ne peut pas être ré-enveloppé est laissé inchangé et signalé avec son ID de ligne, et la commande se termine avec le code 1. Soit sa KEK n’est pas configurée
  (le message nomme l’ID de clé : ajoutez cette KEK à `kek_previous` et relancez), soit la valeur stockée est endommagée ou a été copiée depuis une autre ligne.
  Ne retirez pas une KEK de `kek_previous` tant que `my kek status` compte encore des secrets sous celle-ci.

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

Avec `key_manager = gcpkms`, une clé Cloud KMS symétrique (une CryptoKey avec l’usage `ENCRYPT_DECRYPT`) enveloppe les clés de chiffrement de données, et la KEK ne quitte jamais Cloud KMS.
Un export de la base de données divulgué et une configuration divulguée ensemble n’exposent toujours pas les secrets : les déchiffrer exige l’autorisation d’utiliser la clé.

- Définissez `kms_key` sur le nom de ressource de la clé, sans version :
  `projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`. Les noms de plus de 248 octets sont rejetés au démarrage.
  L’ID de clé des secrets stockés est `gcpkms:` suivi de ce nom.
- Le serveur s’authentifie avec les Application Default Credentials : sur Cloud Run, le compte de service du service ; ailleurs, `GOOGLE_APPLICATION_CREDENTIALS`.
- Accordez à ce compte de service `roles/cloudkms.cryptoKeyEncrypterDecrypter` sur la clé (et non sur le projet), pour chaque processus, comme web et worker, et pour les commandes `my kek status` et `my kek rewrap`.
  Sans cela, la lecture d’un secret échoue avec une erreur qui nomme le rôle ; une version de clé manquante ou désactivée échoue de la même façon, et ces erreurs ne sont pas retentées tant que la configuration n’est pas corrigée.
  Une erreur de quota ou un Cloud KMS indisponible est temporaire, et le travail qui avait besoin du secret est retenté.
- Le serveur conserve pendant `kms_dek_cache_ttl` (`10m` par défaut) une clé de chiffrement de données que Cloud KMS a déchiffrée, de sorte que la plupart des lectures d’un secret n’appellent pas Cloud KMS.
- Le tag de compilation `no_gcpkms` exclut l’adaptateur du binaire ([Tags de compilation](../reference/build-tags.md)).

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### Renouveler la clé dans Cloud KMS

Une rotation dans Cloud KMS (à la main, ou avec une période de rotation sur la clé) ajoute une nouvelle version primaire et laisse les anciennes versions activées.
Les nouveaux secrets sont enveloppés avec la nouvelle version, le serveur déchiffre chaque secret avec la version qui l’a enveloppé, et l’ID de clé ne change pas ; **une rotation n’exige donc pas de `my kek rewrap`**.

**Ne désactivez ni ne détruisez une ancienne version tant que des secrets en ont encore besoin** : un secret enveloppé avec une version désactivée ou détruite ne peut plus être déchiffré.
Cloud KMS n’indique pas quels secrets une version a enveloppés ; pour retirer du matériel de clé, déplacez donc d’abord les secrets vers une nouvelle clé :

1. Créez une nouvelle clé (elle peut se trouver dans le même trousseau de clés) et accordez le rôle sur celle-ci au compte de service.
2. Redémarrez chaque processus avec `kms_key` défini sur la nouvelle clé et `kms_key_previous` défini sur l’ancienne. L’ancienne clé ne fait plus que déchiffrer.
3. Exécutez `my kek rewrap`, avec la même configuration. Lorsque `my kek status` ne compte plus de secrets sous l’ID de l’ancienne clé, retirez `kms_key_previous` et redémarrez.
4. Seulement ensuite, désactivez les versions de l’ancienne clé, et gardez-les restaurables aussi longtemps que vous conservez des sauvegardes de la base de données réalisées avant l’étape 3 : en restaurer une en a de nouveau besoin.

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

Avec `key_manager = awskms`, une clé AWS KMS symétrique (spécification de clé `SYMMETRIC_DEFAULT`, usage de clé `ENCRYPT_DECRYPT`, gérée par le client) enveloppe les clés de chiffrement de données, et la KEK ne quitte jamais AWS KMS.
Un export de la base de données divulgué et une configuration divulguée ensemble n’exposent toujours pas les secrets : les déchiffrer exige l’autorisation d’utiliser la clé.

- Définissez `kms_key` sur l’ARN de la clé (`arn:aws:kms:{region}:{account}:key/{key-id}`), un alias (`alias/{name}` ou son ARN), ou l’ID de clé.
  Au démarrage, le serveur résout le nom en ARN de la clé, et l’ID de clé des secrets stockés est `awskms:` suivi de cet ARN.
  Faire pointer un alias vers une autre clé ne change donc rien avant le prochain démarrage, et alors seulement pour les nouveaux secrets : les secrets déjà stockés continuent de nommer l’ancienne clé ; définissez-la donc dans `kms_key_previous` (voir [Mettre hors service une clé AWS KMS](#retire-an-aws-kms-key)).
  Les clés gérées par AWS (`alias/aws/...`) sont rejetées ; seul le service AWS qui en est propriétaire peut l’utiliser.
- Le serveur s’authentifie avec la chaîne d’identifiants par défaut du SDK AWS : sur ECS, le rôle de la tâche ; sur EC2, le profil d’instance ; ailleurs, l’environnement (`AWS_ACCESS_KEY_ID` et similaires) ou les fichiers de configuration partagés.
  La région est celle de l’ARN de la clé ; pour un alias ou un ID de clé, définissez `AWS_REGION`.
- Autorisez `kms:Encrypt`, `kms:Decrypt` et `kms:DescribeKey` sur la clé, pour chaque processus, comme web et worker, et pour les commandes `my kek status` et `my kek rewrap`. Une politique du rôle IAM :

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  La ressource est l’ARN de la clé même lorsque `kms_key` est un alias. La politique de clé doit aussi permettre aux politiques IAM du compte d’accorder l’accès (la politique de clé par défaut le permet).
- Le serveur ne démarre pas lorsque la clé n’existe pas, est désactivée ou en attente de suppression, ou n’est pas une clé de chiffrement symétrique. Plus tard, une autorisation manquante ou une clé désactivée fait échouer la lecture d’un secret avec une erreur qui nomme la cause,
  et ces erreurs ne sont pas retentées tant que la configuration n’est pas corrigée. Une limitation de débit (`ThrottlingException`) ou un AWS KMS indisponible est temporaire, et le travail qui avait besoin du secret est retenté.
- Le contexte de chiffrement de chaque appel contient l’ID de ligne et le nom de colonne du secret (`{"my-aad": ...}`, en base64url). CloudTrail l’enregistre en clair ; il ne contient aucun secret.
- Coût : AWS facture des frais mensuels pour chaque clé gérée par le client (1 USD au moment de la rédaction, plus autant pour chacune des deux premières rotations de son matériel de clé) et des frais par requête au-delà d’un quota gratuit mensuel
  (voir [la tarification d’AWS KMS](https://aws.amazon.com/kms/pricing/)). Le serveur conserve pendant `kms_dek_cache_ttl` (`10m` par défaut) une clé de chiffrement de données qu’AWS KMS a déchiffrée,
  de sorte qu’un serveur avec dix acteurs qui livrent en continu fait environ 43 000 requêtes par mois et par processus, quelques centimes au-delà du quota gratuit.
- Le tag de compilation `no_awskms` exclut l’adaptateur du binaire ([Tags de compilation](../reference/build-tags.md)).

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### Mettre hors service une clé AWS KMS

La rotation automatique des clés dans AWS KMS conserve l’ARN de la clé et son matériel de clé antérieur ; l’ID de clé ne change donc pas et **une rotation n’exige pas de `my kek rewrap`**.

Pour cesser d’utiliser une clé, ou pour faire pointer un alias vers une autre clé, déplacez d’abord les secrets vers la nouvelle clé. **Ne désactivez pas l’ancienne clé et ne planifiez pas sa suppression tant que des secrets en ont encore besoin** : un secret enveloppé avec elle ne peut plus être déchiffré.

1. Créez la nouvelle clé et autorisez le rôle à l’utiliser.
2. Redémarrez chaque processus avec `kms_key` défini sur la nouvelle clé et `kms_key_previous` défini sur l’ancienne (son ARN, pas l’alias). L’ancienne clé ne fait plus que déchiffrer.
3. Exécutez `my kek rewrap`, avec la même configuration. Lorsque `my kek status` ne compte plus de secrets sous l’ID de l’ancienne clé, retirez `kms_key_previous` et redémarrez.
4. Seulement ensuite, désactivez l’ancienne clé, et gardez-la aussi longtemps que vous conservez des sauvegardes de la base de données réalisées avant l’étape 3 : en restaurer une en a de nouveau besoin.

<!-- source: a-plugin 43023e60bac9 -->
## Un plugin

Avec `key_manager = plugin`, un [plugin](plugins.md) à vous conserve la KEK, dans Vault, dans un HSM ou là où il le choisit, et enveloppe les clés de chiffrement de données pour le serveur.

**Le plugin se trouve dans la même frontière de confiance que la KEK.** Il voit chaque clé de chiffrement de données en clair ; quiconque le contrôle, ou contrôle l’hôte sur lequel il s’exécute, peut donc lire tous les secrets du serveur, y compris les clés privées des acteurs.
Donnez-lui la protection que vous donnez à la KEK. C’est aussi pourquoi aucun paramètre autre que le fichier de configuration et l’environnement ne peut modifier son adresse.

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- L’adresse est `unix://` ou `https://`. Le serveur refuse `http://`, y compris sur la boucle locale, car les clés circuleraient en clair. Les autres paramètres de la destination sont dans [Plugins](plugins.md#the-destination).
- Exécutez `my plugin test --kind key_manager <address>` avant de faire pointer le serveur vers un plugin ([Tester un plugin](plugins.md#test-a-plugin)). Il n’enveloppe que des clés qu’il invente et jette.
- L’ID de clé des secrets stockés est `plugin:` suivi de l’ID que le plugin donne à sa KEK, qui compte de 1 à 248 octets d’ASCII visible.
- **Conservez chaque KEK qu’un secret stocké nomme encore.** Une rotation dans le plugin apparaît comme un nouvel ID au prochain enveloppement. Les secrets enveloppés avant continuent de nommer l’ancien ID, et le plugin doit encore pouvoir les ouvrir. `my kek rewrap` les déplace vers l’ID courant, et `my kek status` compte ceux qui restent.
- Un plugin et un KMS ne servent jamais ensemble : `kms_key` et `kms_key_previous` sont refusés avec un plugin, et `plugin_key_manager_address` l’est avec `key_manager = gcpkms` ou `awskms`.
- Le tag de compilation `no_plugins` exclut l’adaptateur du binaire ([Tags de compilation](../reference/build-tags.md)).

<!-- source: when-the-server-starts 5faeda92e43b -->
### Au démarrage du serveur

Chaque processus, et chaque commande `my` qui ouvre la base de données, comme `my kek status`, fait ceci avant tout le reste. C’est le seul type de plugin dont l’échec empêche un démarrage.

1. Il demande au plugin l’ID de sa KEK courante.
2. Il exécute un autotest avec une clé aléatoire qu’il jette : un enveloppement et un désenveloppement doivent rendre la clé, un désenveloppement avec des données authentifiées différentes doit répondre `OUTCOME_DECRYPT`, deux enveloppements de la clé doivent différer, et la valeur enveloppée ne doit pas contenir la clé.

Un plugin qui ne répond pas, ou répond `503`, est réessayé pendant 30 secondes au plus, pour un sidecar qui démarre après le serveur. Passé ce délai, le processus se termine avec une erreur. Une mauvaise réponse, et tout autre échec comme un jeton refusé, l’arrête immédiatement : l’autotest existe pour arrêter un plugin qui ignore les données authentifiées ou laisse passer les clés, et attendre n’en répare pas un.

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### Lorsque le plugin est injoignable

Le serveur échoue de manière fermée. Quoi qu’il arrive, un appel refusé ou expiré, ou une réponse vide, inconnue ou mal formée, l’opération qui avait besoin de la clé échoue avec une erreur. Il ne stocke jamais un secret en clair, ne bascule jamais vers la KEK locale ou une autre clé, et ne traite jamais un échec comme un résultat. Une tâche qui échoue ainsi est retentée plus tard, et une requête échoue pour la personne qui l’a faite. Chaque appel échoué est compté dans [`my.plugin.failures`](running.md) par type et par code Connect, et le journal ne contient ni clé, ni donnée authentifiée, ni valeur enveloppée, ni rien de ce que le plugin a dit.

L’enveloppement n’est jamais mis en cache, et une valeur scellée il y a un instant n’a rien dans le cache ; ceci s’arrête donc immédiatement :

- La création d’un compte, qui génère les clés de l’acteur.
- La configuration de l’authentification à deux facteurs, et l’enregistrement d’une clé d’accès ou la connexion avec l’une d’elles, dont les cérémonies conservent leur état scellé.
- La connexion par un fournisseur externe, et l’enregistrement du secret client d’un fournisseur.
- L’enregistrement d’un webhook, et `my kek rewrap`.

La lecture d’un secret stocké ne fonctionne que tant que le processus conserve sa clé de chiffrement de données d’une lecture précédente, pendant `kms_dek_cache_ttl` au plus (`10m` par défaut). Passé ce délai, ou pour un secret qu’il n’a pas encore lu, ceci s’arrête aussi :

- La livraison fédérée et les récupérations signées, qui ont besoin des clés privées des acteurs. La livraison rattrape son retard grâce aux nouvelles tentatives de ses tâches, une fois le plugin de retour.
- La vérification d’un code à deux facteurs lors de la connexion.
- La livraison des webhooks.

Un redémarrage s’arrête comme décrit plus haut ; **ne redémarrez donc pas le serveur pour réparer une panne du plugin** : un serveur qui est en marche continue de servir ce que son cache couvre, et celui qui redémarre ne sert rien tant que le plugin ne répond pas. Ce qui n’exige aucun secret, comme la lecture des fils et la publication, continue. La clé de signature du proxy de médias est lue au démarrage et reste en mémoire. Un certificat client ou un fichier d’autorité de certification renouvelé n’exige pas non plus de redémarrage : le serveur relit les fichiers dès qu’ils changent ([Plugins](plugins.md#authentication)).

<!-- source: the-contract 6183eb7a7dfa -->
### Le contrat

Les RPC sont celles de `my.plugin.key_manager.v1.KeyManagerService`, avec un délai par défaut de 5 secondes et des messages d’au plus 128 Kio dans chaque sens.

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` est l’ID propre du plugin, sans `plugin:`. `UnwrapKey` peut nommer un ID plus ancien que le courant.
- `wrapped` fait de 1 à 65 535 octets et ne doit pas contenir la clé. Deux enveloppements d’une clé diffèrent.
- `aad` est une donnée authentifiée qui contient des ID de lignes et des noms de colonnes, et `UnwrapKey` doit échouer si elle n’est pas celle que `WrapKey` a reçue. C’est ce qui empêche de copier un secret vers une autre ligne.
- **Ne journalisez jamais `dek`, `aad` ni `wrapped`.**

| `outcome` | Signification |
| --- | --- |
| `OUTCOME_OK` | L’appel a réussi |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey` : le plugin ne détient pas la KEK de `keyId` |
| `OUTCOME_DECRYPT` | `UnwrapKey` : `wrapped` ou `aad` ne s’authentifie pas |

Répondez par une erreur HTTP ou Connect lorsque vous ne pouvez pas répondre, par exemple lorsque le magasin de clés est hors d’atteinte. Ne répondez alors pas `OUTCOME_DECRYPT` ni `OUTCOME_UNKNOWN_KEY` : pour le serveur, ils disent que la valeur elle-même est mauvaise. Le serveur ne retente pas lui-même un appel ; un plugin peut donc recevoir le même appel de nouveau depuis une tâche.

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## Passer de la KEK locale à un KMS, et inversement

Un ID de clé commence par le type de gestionnaire de clés qui l’a enveloppée (`local:`, `gcpkms:`, `awskms:`, `plugin:`). Le serveur enveloppe les nouveaux secrets avec le `key_manager` courant
et désenveloppe chaque secret stocké avec le gestionnaire de clés que son ID de clé nomme ; passer de la KEK locale à un KMS est donc une rotation entre types.

- Avec un KMS, `kek` et `kek_previous` ne font que déchiffrer : ils ouvrent les secrets encore enveloppés avec la KEK locale, et chaque nouveau secret est enveloppé avec `kms_key`.
  `kms_key_previous` liste les clés antérieures du même KMS, qui ne font elles aussi que déchiffrer.
- Avec `key_manager = local`, `kms_key` et `kms_key_previous`, lorsqu’ils sont définis, ne font que déchiffrer : ils ouvrent les secrets encore enveloppés avec le KMS.
- De la KEK locale vers un KMS :
  1. Créez la clé et accordez les autorisations comme dans [Google Cloud KMS](#google-cloud-kms) ou [AWS KMS](#aws-kms).
  2. Redémarrez chaque processus avec `key_manager = gcpkms` ou `awskms`, `kms_key` défini sur la clé, et la KEK courante déplacée de `kek` vers `kek_previous`.
     Les secrets sont maintenant enveloppés avec le KMS, et ceux enveloppés avec la KEK locale s’ouvrent toujours.
  3. Exécutez `my kek rewrap` avec la même configuration.
  4. Lorsque `my kek status` ne compte plus de secrets sous un ID de clé `local:`, retirez `kek_previous` et redémarrez. Conservez la sauvegarde de la KEK aussi longtemps que vous conservez des sauvegardes de la base de données réalisées avant l’étape 3.
- D’un KMS vers la KEK locale, faites la même chose en sens inverse : définissez `key_manager = local` et une nouvelle `kek`, et gardez `kms_key` défini, qui ne fait alors que déchiffrer,
  jusqu’à ce que `my kek rewrap` ait déplacé chaque secret et que `my kek status` ne compte plus de secrets sous un ID de clé `gcpkms:` ou `awskms:`.
  Les clés de `kms_key` et `kms_key_previous` doivent alors toutes appartenir au même KMS.
- Pour passer du KMS d’un cloud à celui d’un autre, passez d’abord à la KEK locale puis à l’autre KMS ; il n’y a pas de passage direct.
- Un plugin se déplace de la même façon, uniquement par la KEK locale :
  - Vers lui, redémarrez chaque processus avec `key_manager = plugin`, `plugin_key_manager_address`, et la KEK courante déplacée de `kek` vers `kek_previous`, puis exécutez `my kek rewrap`. Lorsque `my kek status` ne compte plus de secrets sous un ID de clé `local:`, retirez `kek_previous` et redémarrez.
  - Depuis lui, redémarrez chaque processus avec `key_manager = local`, une nouvelle `kek`, et `plugin_key_manager_address` conservé, qui ne fait alors que déchiffrer, puis exécutez `my kek rewrap`. Lorsque `my kek status` ne compte plus de secrets sous un ID de clé `plugin:`, retirez l’adresse et redémarrez. D’ici là, le plugin doit toujours réussir [les vérifications au démarrage](#when-the-server-starts), qui comprennent un enveloppement.
- Le serveur conserve pendant `kms_dek_cache_ttl` (`10m` par défaut, au plus `kms_dek_cache_entries`, `1024` par défaut) une clé de chiffrement de données que le KMS ou le plugin a déchiffrée,
  de sorte qu’un secret en cours d’utilisation l’atteint environ une fois par période et par processus. `0` dans l’un ou l’autre paramètre désactive le cache.

<!-- source: if-the-kek-leaks be6e788e4cea -->
## Si la KEK est divulguée

Si seule la KEK a été divulguée, renouvelez-la comme ci-dessus. Si la KEK et la base de données ont pu être divulguées toutes les deux, la rotation ne vous protège pas :
les clés privées des acteurs dans la base de données sont déjà lisibles ; les clés des acteurs elles-mêmes doivent donc être régénérées.

<!-- source: web-push-vapid-key aa32321d1e4a -->
## Clé VAPID de Web Push

Web Push est signé avec la clé VAPID du serveur (P-256, RFC 8292). C’est un secret distinct de la KEK, qui n’est pas stocké dans la base de données.

- Créez une paire de clés avec `my config vapid-key`, et donnez à chaque processus la clé privée (la valeur de `vapid_private_key = ...`) par `MY_VAPID_PRIVATE_KEY` ou `MY_VAPID_PRIVATE_KEY_FILE`.
  Le serveur dérive la clé publique de la clé privée et la transmet aux clients ; vous ne la configurez donc pas.
- Si vous ne la définissez pas, Web Push est désactivé et les clients ne le proposent pas. Dans le profil `production`, `my config check` et le démarrage de `my serve` vous en avertissent.
- Les navigateurs lient un abonnement à la clé publique ; changer la clé interrompt donc la livraison vers les abonnements existants. Lorsqu’un utilisateur réactive les notifications push dans les paramètres, le client se réabonne avec la nouvelle clé.
  Ne changez pas la clé sauf si elle a été divulguée ; si elle l’est, passez à une nouvelle clé et demandez aux utilisateurs de réactiver les notifications.
