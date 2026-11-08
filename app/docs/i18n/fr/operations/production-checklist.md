<!-- source: production-checklist e5f6c0c5b10a -->
# Liste de contrôle de production

Parcourez cette liste avant d’ouvrir le serveur aux utilisateurs et aux autres serveurs, puis à nouveau après toute modification du déploiement. Certains paramètres ne peuvent plus être modifiés une fois que le serveur a fédéré, et certaines erreurs, comme la perte de la clé de chiffrement de clés, sont irréversibles.

Chaque élément indique pourquoi il compte, comment le vérifier et où trouver les détails. Commencez par la partie automatisée :

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## Ce que vérifie `my config check`

Dans le profil `production` (celui par défaut de [`profile`](../reference/configuration.md#profile)), `my config check` valide d’abord chaque paramètre, puis vérifie que les paramètres valides conviennent à une instance de production. Les problèmes sont listés en deux groupes, sous un résumé dans la langue de la locale du processus, chacun avec le détail technique en anglais :

- Les **erreurs** font quitter `my config check` avec le code 1. Ce sont des paramètres qui cassent une instance de production ou affaiblissent sa sécurité :
  - Un `host` ou un `account_domain` que les autres serveurs ne peuvent pas joindre : avec un port, `localhost`, un nom sans point, ou un nom sous un domaine de premier niveau réservé à l’usage local ou aux tests (`.localhost`, `.local`, `.test`, `.example`, `.invalid`, `.internal`, `.home.arpa`).
  - Un `web_origin` en `http`.
  - Un `database_url` SQLite qui garde la base de données en mémoire.
  - Un `kek` qui n’est pas aléatoire, par exemple composé uniquement de zéros ou d’un octet répété.
  - Une clé de site ou une clé secrète de `captcha` qui est l’une des clés de test publiées par les services de CAPTCHA.
  - `worker_job_types`, `worker_job_types_except` ou une route de file qui ne nomme aucun type de tâche (dans tous les profils ; `my job types` les liste).
  - Avec `-instance`, un `worker_job_concurrency` que le pool de la base de données ne peut pas servir.
- Les **avertissements** laissent le code de sortie à 0. Ce sont des paramètres habituels en développement qui peuvent être voulus en production :
  - `mail = none`.
  - Une base de données SQLite ou le stockage de blobs `fs` dans un répertoire temporaire (`/tmp`, `/var/tmp`, `/dev/shm` ou `$TMPDIR`).
  - `queue = inline`.
  - Pas de `vapid_private_key`.
  - `stream = hub` ou `kv = memory` avec un `role` autre que `all` : ils ne fonctionnent qu’au sein d’un seul processus.
  - Des paramètres de voix probablement non voulus, comme `voice = p2p` sans STUN ni TURN.
  - Un [plugin](plugins.md) à une adresse `http://`, ou à une adresse `https://` sans jeton ni certificat client.
  - Une clé en mode test du fournisseur de paiement.
  - `origin_secret` défini avec un `trusted_proxies` vide, et `origin_secret_previous` laissé défini après une rotation.
  - Avec `-instance` : les textes d’instance `name`, `short_description`, `terms` et `privacy_policy`, les paramètres d’instance `contact_account`, `contact_email` et `security_contact` lorsqu’ils ne sont pas définis, et un serveur sans règles. Lorsque la base de données est injoignable, cela fait un avertissement de plus et le reste du contrôle s’exécute quand même.

`my serve` consigne les mêmes constats sous forme d’avertissements au démarrage, mais démarre malgré tout. Le profil `development` ignore ces contrôles ; utilisez-le pour le développement local et les tests, jamais pour une instance que d’autres utilisent.

Exécutez `my config check` dans le même environnement que le serveur (mêmes variables d’environnement, mêmes fichiers de secrets, même fichier de configuration), par exemple comme étape du déploiement avant que la nouvelle version ne reçoive du trafic.

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## Paramètres que l’on ne peut plus changer ensuite

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### Le domaine du serveur et le domaine des identifiants

- **Pourquoi** : `host` fait partie de chaque identifiant ActivityPub du serveur, et les autres serveurs conservent ces identifiants. `account_domain` est le domaine des identifiants de comptes (`@alice@example.org`). Le premier `my serve` ou la première commande `my` qui utilise la base de données enregistre les deux dans celle-ci, et dès lors `my serve` et ces commandes refusent de s’exécuter lorsque la configuration diffère : un changement après la fédération casse chaque abonnement et chaque référence aux publications et aux comptes du serveur. Définissez les deux avant le premier démarrage. Une erreur découverte avant la fédération se corrige en [recréant la base de données](database.md#recreating-erasing-every-row), ce qui efface chaque ligne.
- **À décider** : si les identifiants utilisent le domaine du serveur ou un domaine distinct (par exemple des identifiants sur `example.org` et le serveur sur `social.example.org`). Un domaine d’identifiants distinct doit rediriger son WebFinger, son host-meta et son NodeInfo vers le serveur.
- **Vérifier** : `my config check` affiche `host` et `account_domain` et signale un domaine que les autres serveurs ne peuvent pas joindre. `my config check -instance` échoue aussi lorsqu’ils diffèrent des valeurs de la base de données, et n’enregistre rien. Avec un domaine d’identifiants distinct, `curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` répond `301` vers `https://<host>/.well-known/webfinger?...`.
- **Détails** : [exécuter le serveur](running.md) (les points de terminaison de fédération et la publication des identifiants depuis un autre domaine), [`host`](../reference/configuration.md#host) et [`account_domain`](../reference/configuration.md#account_domain).

<!-- source: reachable-over-https 1cf53c613f7b -->
### Joignable en `https`

- **Pourquoi** : les autres serveurs récupèrent les identifiants à `https://<host>/` sur le port 443, et les navigateurs n’envoient les cookies de connexion qu’en `https`.
- **Vérifier** : `curl -sI https://<host>/healthz` répond `200` depuis l’extérieur de votre réseau avec un certificat valide pour `host`, et `my config check` ne signale aucune erreur à propos de `web_origin`. `http://<host>/` redirige vers `https`, ce que font le proxy inverse, le répartiteur de charge ou le CDN placé devant le serveur (avec Docker Compose, la couche `compose.caddy.yaml` est un tel proxy).
- **Détails** : [exécuter le serveur](running.md), [`web_origin`](../reference/configuration.md#web_origin).

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### Non joignable en contournant le CDN

- **Pourquoi** : avec un CDN devant, et l’adresse propre du serveur toujours publique (`*.run.app` sur Cloud Run), n’importe qui peut appeler le serveur en contournant le WAF et le cache, et un appelant situé sur la plateforme même du CDN peut choisir l’IP client que voient les limites de débit. Nécessaire uniquement dans ce cas : pas lorsqu’un pare-feu ou la plateforme limite déjà qui peut se connecter, et sans CDN il n’y a rien à contourner.
- **Vérifier** : `origin_secret` et `origin_secret_header` sont définis et le CDN place l’en-tête sur chaque nom d’hôte. `curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/` répond `403`, son `/healthz` répond `200`, et `https://<host>/` répond `200`.
- **Détails** : [Refuser les requêtes qui ne sont pas passées par le CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), y compris comment s’en sortir lorsque cela enferme tout le monde dehors.

<!-- source: secrets ec71f1f20574 -->
## Secrets

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### La clé de chiffrement de clés (KEK)

- **Pourquoi** : la KEK chiffre les clés privées de chaque acteur. Sans elle, le serveur ne peut rien signer, et les clés ne peuvent pas être récupérées depuis la base de données : il faudrait recréer chaque compte et les autres serveurs ne lui feraient plus confiance.
- **Vérifier** :
  - Elle a été générée aléatoirement (`openssl rand -base64 32`) ; `my config check` signale une clé qui n’est pas aléatoire.
  - Elle est stockée ailleurs que dans la base de données et ses sauvegardes, par exemple dans un gestionnaire de mots de passe ou le coffre de secrets du cloud, et au moins une autre personne qui exploite le serveur peut y accéder.
  - Elle parvient au serveur uniquement par `MY_KEK` ou `MY_KEK_FILE` (le fichier de configuration refuse les secrets). Avec Docker Compose, c’est le fichier `secrets/kek`, et elle n’est pas dans `.env` ([fichiers Docker Compose](running.md#docker-compose-files)).
- **Détails** : [gestion des clés](key-management.md).

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### La clé VAPID de Web Push

- **Pourquoi** : sans elle, le serveur n’envoie aucune notification Web Push. La changer ensuite met fin à tous les abonnements, générez-la donc une seule fois et conservez-la comme la KEK.
- **Vérifier** : `my config check` avertit lorsque `vapid_private_key` n’est pas défini. Générez-en une avec `my config vapid-key`.
- **Détails** : [gestion des clés](key-management.md) (la clé VAPID de Web Push).

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### Les autres secrets restent hors du dépôt et de l’image

- **Pourquoi** : un secret dans un dépôt, une image de conteneur ou un fichier de configuration est copié partout où ceux-ci vont.
- **Vérifier** : `database_url`, `mail_smtp_password`, `captcha_secret_key`, les clés du fournisseur de paiement, `valkey_url` et tous les autres paramètres marqués comme secrets dans la référence de configuration proviennent de variables d’environnement ou de fichiers `<NAME>_FILE` que remplit le coffre de secrets de la plateforme (secrets Docker ou Kubernetes, Secret Manager, Parameter Store). Le fichier de configuration ne peut pas les contenir : `my config check` y refuse un secret. Avec Docker Compose, ils proviennent de `.env`, que vous seul pouvez lire (`chmod 600`). Recherchez les valeurs dans le dépôt et l’image pour vous assurer qu’il ne reste aucune copie. `my config check` affiche `(redacted)` pour chaque secret défini.
- **Détails** : [référence de configuration](../reference/configuration.md) (la colonne Secret), [CAPTCHA](captcha.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md).

<!-- source: instance-settings baae9cc3e09e -->
## Paramètres d’instance

Les paramètres d’instance résident dans la base de données et peuvent être modifiés pendant que le serveur fonctionne, dans les écrans d’administration du serveur ou avec `my instance set`. `my config check -instance` avertit des textes, des contacts et des règles qui ne sont pas définis.

- **Nom, description courte et description** : affichés sur la page d’accueil, la page d’informations (`/about`), les aperçus de liens et NodeInfo. Vérifiez sur `/about`.
- **Contacts** : `contact_account` (le compte que les utilisateurs contactent à propos du serveur) et `contact_email` (l’adresse à laquelle écrivent les utilisateurs et les autres serveurs ; les e-mails du serveur y dirigent aussi les réponses). Vérifiez sur `/about`.
- **Contact pour les vulnérabilités** : `security_contact`, un URI `https:` ou `mailto:`. Lorsqu’il est défini, le serveur répond à `/.well-known/security.txt`. Vérifiez avec `curl -s https://<host>/.well-known/security.txt`.
- **Conditions d’utilisation et politique de confidentialité** : `terms` et `privacy_policy`. La politique de confidentialité doit nommer les services qui reçoivent des données personnelles, comme le service de CAPTCHA et le fournisseur d’e-mail. Vérifiez les liens sur `/about` et sur l’écran d’inscription.
- **Règles du serveur** : ce que les utilisateurs acceptent en s’inscrivant, et ce que les modérateurs font respecter. Ajoutez-les avec `my instance rule create` ou dans les écrans d’administration, et vérifiez-les sur `/about`.
- **Mode d’inscription** : `registration_mode` vaut `approval` tant que vous ne le changez pas ; choisissez `open`, `approval`, `invite` ou `closed` selon la façon dont les gens rejoignent le serveur. Avec `mail = none`, les inscriptions `open` attendent aussi l’approbation d’un modérateur.
- **Moteurs de recherche et IA** : `search_indexing`, `ai_usage` et `indexable_default` refusent les deux tant que vous ne les autorisez pas. Vérifiez `/robots.txt` et `/.well-known/tdmrep.json`.
- **Page de statut** : définissez [`status_page_url`](../reference/configuration.md#status_page_url) (un paramètre, pas un paramètre d’instance, afin que le lien s’affiche pendant la maintenance) sur une page de statut hébergée par un service distinct du serveur.

**Détails** : [paramètres d’instance](instance-settings.md), [inscription](registration.md), [moteurs de recherche et robots d’IA](crawlers.md), [CAPTCHA](captcha.md) (ce qu’il faut écrire dans la politique de confidentialité), [modèle de politique de confidentialité](privacy-policy-template.md), [modèle de registre des activités de traitement](processing-record-template.md), [surveillance](monitoring.md) (la page de statut).

<!-- source: email 40c18999c46f -->
## E-mail

- **Pourquoi** : l’e-mail vérifie les adresses et envoie les réinitialisations de mot de passe et les notifications. Avec `mail = none`, aucun e-mail n’atteint les utilisateurs.
- **Vérifier** :
  - `mail` vaut `smtp`, `ses` ou `plugin`, et `mail_from` est une adresse sur un domaine que vous contrôlez ; `my config check` avertit pour `mail = none`.
  - Le domaine de `mail_from` a des enregistrements SPF, DKIM et DMARC, et le fournisseur signe avec DKIM. Sans eux, les messages atterrissent dans les indésirables.
  - Inscrivez-vous avec un compte de test et recevez l’e-mail de vérification ; demandez une réinitialisation de mot de passe et recevez-la aussi. Vérifiez dans les en-têtes du message reçu la présence de `spf=pass`, `dkim=pass` et `dmarc=pass`.
  - Avec SES, le compte est sorti du bac à sable, et les notifications de rebond et de plainte parviennent au serveur.
- **Détails** : [e-mail](email.md), [inscription](registration.md) (vérification de l’e-mail).

<!-- source: administrators 2c05e6644d88 -->
## Administrateurs

- **Pourquoi** : le compte d’un administrateur peut modifier chaque paramètre, lire des données privées et suspendre n’importe qui.
- **Vérifier** :
  - Chaque administrateur a activé l’authentification à deux facteurs (« Paramètres », « Authentification à deux facteurs » dans le client web) et conserve les codes de récupération.
  - Seules les personnes qui en ont besoin détiennent le rôle `admin` ; les modérateurs détiennent `moderator` ou un rôle avec uniquement les permissions qu’ils utilisent. Listez-les avec `my admin role` et passez-les en revue dans les écrans d’administration.
- **Détails** : [administrateurs et rôles](admin-cli.md), [modération](moderation.md) (permissions et récupération de connexion).

<!-- source: operations 56bd643c2241 -->
## Exploitation

- **Sauvegardes et restauration** : la base de données, le stockage de blobs (`blob_fs_dir` ou le bucket) et la KEK sont sauvegardés selon un calendrier, vers un emplacement distinct du serveur, et une restauration vers un serveur neuf a été répétée : le serveur restauré démarre et les comptes peuvent se connecter et publier. Une sauvegarde qui n’a jamais été restaurée n’est pas réputée fonctionner. La base de données et les médias ne doivent pas se trouver dans un répertoire temporaire ; `my config check` avertit lorsque c’est le cas. Voir [gestion des clés](key-management.md) (sauvegarder la KEK) et [base de données](database.md).
- **Surveillance et alertes** : une sonde externe contrôle `/healthz` et alerte une personne en mesure d’agir, et les tâches mortes de la file de tâches sont surveillées. Voir [surveillance](monitoring.md) et [exécuter le serveur](running.md) (contrôles de santé, métriques et file de tâches).
- **L’image** : l’instance exécute une image publiée épinglée par son empreinte (`ghcr.io/mycroblog/my:0.1.0@sha256:...`, tirée des notes de version), et non une image qui change à un redémarrage, et [sa provenance a été vérifiée](running.md#verify-an-image). Il n’existe pas de balise `latest`. Une mise à niveau change cette version volontairement et suit [Mise à niveau](database.md#upgrading), une version mineure à la fois.
- **Code source (AGPL-3.0)** : le serveur doit proposer son code source à toutes les personnes qui l’utilisent. Laissez [`source_url`](../reference/configuration.md#source_url) à sa valeur par défaut pour une version publiée non modifiée ; faites-le pointer vers votre propre source lorsque vous exécutez un fork ou une version modifiée. Vérifiez le lien sur `/about`. Voir [exécuter le serveur](running.md) (publier le code source).
- **Maintenance** : sachez comment placer le serveur en mode maintenance avant d’en avoir besoin. Voir [maintenance](maintenance.md).

<!-- source: federation a5ab186ec04b -->
## Fédération

- **Pourquoi** : les autres serveurs trouvent les comptes par WebFinger et NodeInfo, puis récupèrent l’acteur. Si l’un d’eux échoue depuis l’extérieur, le serveur est invisible pour le fediverse.
- **Vérifier** depuis une machine extérieure à votre réseau :
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` renvoie le compte, en suivant la redirection vers `host` lorsque les domaines diffèrent (`curl -L`).
  - `curl -s https://<host>/.well-known/nodeinfo` renvoie un lien vers `/nodeinfo/2.1`, qui nomme le logiciel et sa source.
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` redirige vers l’identifiant de l’acteur, et l’acteur à cet identifiant est du JSON avec une `publicKey`.
  - Depuis un compte sur Mastodon ou un autre serveur, recherchez `@<user>@<account_domain>`, suivez-le et voyez arriver une nouvelle publication.
  - Un CDN placé devant le serveur ne met pas en cache `/ap/*` et `/@*` à moins qu’il ne respecte `Vary: Accept`.
- **Détails** : [exécuter le serveur](running.md) (les points de terminaison de fédération), [CDN](cdn.md), [relais](relays.md).
