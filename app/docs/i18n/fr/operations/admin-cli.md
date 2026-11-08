<!-- source: administrators-and-roles 1baeba6d67b4 -->
# Administrateurs et rôles

Comment créer le premier administrateur, gérer les rôles en ligne de commande, annoncer et mener à bien la fin du service, retraiter les images téléversées, reconstruire l’index de recherche et exporter la page de maintenance.

<!-- source: prerequisites b7ae62128809 -->
## Prérequis

- La configuration (`host`, `database`, `database_url`, `kek`, etc.) est définie comme le décrit la [référence de configuration](../reference/configuration.md). `my config check` la vérifie.
- Le schéma est appliqué à la base de données (`my schema apply` ; consultez [database.md](database.md)).
- `host` et `account_domain` sont définitifs. La première commande qui utilise la base de données les enregistre, et toute commande ultérieure refuse de s’exécuter lorsque la configuration diffère (consultez [Paramètres que vous ne pouvez plus modifier ensuite](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: creating-an-administrator e5fec2e3d287 -->
## Créer un administrateur

```sh
my admin create -username alice -email alice@example.com
```

- Depuis un terminal, la commande demande deux fois le mot de passe, sans l’afficher.
- Lorsque l’entrée standard n’est pas un terminal, la première ligne de l’entrée standard est le mot de passe (par exemple `my admin create ... < password.txt`).
  Le mot de passe ne peut pas être donné en argument de ligne de commande ni en variable d’environnement, car ceux-ci se retrouvent dans les listes de processus et l’historique du shell.
- Lorsqu’une commande ponctuelle n’a ni terminal ni entrée standard, comme une [tâche Cloud Run](deploy-cloud-run.md#9-create-the-first-administrator), ajoutez `-password-link`. La commande ne lit alors aucun mot de passe : elle en définit un aléatoire que personne ne connaît et affiche un lien après la ligne `created admin`, qui permet de définir le mot de passe une seule fois, pendant une heure, comme `my admin account password-reset-link` (le journal d’audit l’enregistre sous `password_reset_link_issued`).
  Quiconque peut lire la sortie de la commande peut utiliser le lien, et sur une telle plateforme cette sortie reste dans les journaux de la plateforme jusqu’à ce que le lien soit utilisé ou expire. Ouvrez le lien immédiatement. S’il expire, émettez-en un autre avec `my admin account password-reset-link @alice`. Si l’émission du lien échoue, l’administrateur existe déjà : la commande l’indique et se termine avec le code 1, et la même commande donne un nouveau lien.
- Un mot de passe comporte de 8 à 128 caractères. Il n’apparaît jamais dans la sortie, les journaux ni les messages d’erreur.
- Une seule transaction crée l’utilisateur, le mot de passe, l’adresse e-mail principale, le compte local (avec ses clés) et l’attribution du rôle `admin` (sans attributeur, ce qui signifie la CLI).
  Les rôles intégrés sont d’abord créés s’ils manquent.
- En cas de succès, la commande affiche `created admin @alice@<account_domain> (user <id>, account <id>)` et se termine avec le code 0.
- Sur un [serveur d’une seule personne](instance-settings.md#one-person-server), la commande ne crée aucun compte et se termine avec le code 1.
  Tant que [la connexion est limitée aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers), elle avertit aussi que le mot de passe ne permet pas de se connecter jusqu’à la fin de la limite.

<!-- source: running-it-again 197692f7f103 -->
## Relancer la commande

- Lorsque le nom d’utilisateur ou l’adresse e-mail (tous deux insensibles à la casse) est déjà pris, la commande ne modifie rien, indique lequel est pris et se termine avec le code 1.
  Elle ne modifie jamais le mot de passe ni les rôles de l’utilisateur existant.
- Une exécution échouée n’écrit rien ; vous pouvez donc corriger la cause et relancer la même commande.
- Le code de sortie 2 signale une erreur d’utilisation.

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## Attribuer et révoquer des rôles

Les rôles contiennent des permissions. Les rôles intégrés sont `admin` (`position` 1000, toutes les permissions) et
`moderator` (`position` 100, avec `manage_reports`, `manage_accounts` et `view_audit`).
`my serve` vérifie les rôles intégrés à chaque démarrage (dans un processus de n’importe quel `role`), et `my admin role` les vérifie à chaque exécution.
Chacun crée ce qui manque et donne à `admin` toutes les permissions de la version en cours d’exécution.
Lorsqu’une mise à niveau ajoute une permission, aucune étape manuelle n’est nécessaire. Un `moderator` conserve les permissions et la position définies par l’administrateur.

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- Les permissions appartiennent à l’un des trois groupes.
  - Modération : `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit`.
  - Administration du serveur : `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers`.
  - Fonctionnalités des membres : `view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`.

  `list` affiche une colonne par groupe (`MODERATION`, `SERVER`, `MEMBER`). Le client web affiche « Modération » et « Administration » pour les deux premiers groupes et ne compte les fonctionnalités des membres dans aucun des deux (consultez [moderation.md](moderation.md)).
- Une modification prend effet à la requête suivante de l’utilisateur. Les notifications destinées aux modérateurs vont aux personnes qui détiennent la permission au moment de la création de la notification.
- Les attributions et les révocations sont écrites dans le journal d’audit sous `role_granted` et `role_revoked` (type de cible `role`). L’acteur d’une opération CLI est `cli` (`my admin audit list -target-kind role`).
- Attribuer un rôle que l’utilisateur possède déjà, ou révoquer un rôle qu’il n’a pas, ne modifie ni n’enregistre rien.
- Le dernier détenteur de `admin` ne peut pas le perdre (code de sortie 1). Attribuez d’abord `admin` à un autre utilisateur.
- L’API d’administration est `GET /api/v1/admin/roles`, `GET /api/v1/admin/accounts/{id}/roles` et `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`, et elle exige la permission `manage_roles`.
  Via l’API, vous ne pouvez pas attribuer ni révoquer un rôle supérieur à la `position` la plus élevée parmi vos propres rôles (403). Révoquer le dernier `admin` renvoie 409.
- Le personnel n’agit que sur les rangs inférieurs. Via l’API, les mesures de modération et leur levée, la décision sur les recours, la levée des suppressions d’e-mail, la révélation des données privées d’un compte, l’émission d’un lien de réinitialisation de mot de passe et le retrait de l’authentification à deux facteurs exigent que la `position` la plus élevée parmi vos rôles soit supérieure à celle de chaque rôle de la cible ; un rang supérieur ou égal, votre propre compte inclus, renvoie 403. Un modérateur ne peut pas agir sur un administrateur, et un administrateur ne peut pas agir sur un autre. Ces commandes (`my admin account ...`) n’ont pas une telle limite ; utilisez-les lorsque des administrateurs ne sont pas d’accord.
- Le seul administrateur ne peut être ni verrouillé ni retiré : `disable_login`, `suspend` et `ban` le visant sont refusés, via l’API (409) et via ces commandes (code de sortie 1), et il ne peut pas supprimer son propre compte (409). Un autre détenteur de `admin` ne compte que tant qu’il peut se connecter : pas après la suppression de son compte, ni lorsqu’il est suspendu, banni ou sous `disable_login`.

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## Annoncer la fin du service

Pour fermer le serveur, annoncez d’abord la fermeture. [service-closure.md](service-closure.md) décrit toute la procédure, de l’annonce à la destruction des données.
Le serveur continue de fonctionner normalement pendant l’annonce. `closure` dans `GET /api/v1/instance` et `metadata.serviceClosure` dans NodeInfo indiquent l’état et les dates.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` n’a pas de borne inférieure. `-export-until` (la date limite pour récupérer les données) doit être égal ou postérieur à `-closes-at`, et vaut par défaut 30 jours après la date de fermeture.
- Lorsque la date de fermeture est dans moins de 30 jours, la commande affiche un avertissement sur la sortie d’erreur standard (l’annonce est tout de même enregistrée).
- Modifier les dates est enregistré comme une nouvelle annonce, et une annonce faite après un retrait est une nouvelle annonce. Une fois le serveur fermé, vous ne pouvez ni annoncer ni retirer (code de sortie 1).
- L’annonce, la modification des dates et le retrait sont écrits dans le journal d’audit sous `service_closure_announced` et `service_closure_withdrawn` (type de cible `instance` ; consultez `my admin audit list -target-kind instance`).
- L’API d’administration est `GET /api/v1/admin/closure`, `PUT` / `DELETE /api/v1/admin/closure/announcement`, `GET /api/v1/admin/closure/check` et `POST /api/v1/admin/closure/close`. Elle exige la permission `manage_service_closure` (parmi les rôles intégrés, seul `admin` la possède).
- Pendant l’annonce, le serveur n’accepte ni nouveaux utilisateurs ni nouveaux paiements.
  - Le mode d’inscription effectif est `closed` (la valeur de `registration_mode` ne change pas et reprend effet lorsque l’annonce est retirée). Ni les inscriptions ni les inscriptions sur invitation ne fonctionnent, et les demandes en attente ne peuvent pas être approuvées (`409`).
  - L’annonce rejette toutes les demandes en attente (enregistrées comme décidées par `service_closure`) et annule toutes les souscriptions de soutien en cours à la fin de leur période.
    Les souscripteurs reçoivent une notification (`supporter_service_closure`) et un e-mail. Les nouvelles souscriptions renvoient `409`. Retirer l’annonce n’annule ni les rejets ni les annulations.
  - Chaque annonce, modification de dates et retrait envoie la notification `service_closure` à tous les utilisateurs locaux. Tout cela s’exécute sous forme de tâches ; n’arrêtez donc pas le worker.
  - Chaque utilisateur disposant d’une adresse e-mail confirmée reçoit un e-mail pour l’annonce, une modification de dates, un retrait, 7 jours et 1 jour avant la date de fermeture, la fermeture et 7 jours avant la date limite d’export. Les utilisateurs ne peuvent pas s’y opposer (consultez [email.md](email.md)).
  - Après la date de fermeture, les personnes qui détiennent `manage_service_closure` reçoivent la notification `admin_service_closure_due`. Le serveur ne se ferme pas de lui-même.

Après la date de fermeture, fermez le serveur. La fermeture est irréversible.

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` et `close` affichent le nombre de comptes locaux, le nombre de boîtes de réception de destination (les boîtes de réception partagées connues), une estimation des activités `Delete` à envoyer (leur produit), le nombre de souscriptions de soutien en cours,
  le nombre de migrations inachevées (`Move`) et le nombre de comptes sous conservation (`retention_holds`).
  Lorsqu’il reste des migrations ou qu’une conservation existe, la commande avertit sur la sortie d’erreur standard. Exportez les enregistrements des comptes sous conservation avec `my admin account export-moderation` et conservez-les avant de détruire les données.
- `close` refuse sans poser de question lorsqu’il n’y a pas d’annonce et avant la date de fermeture (code de sortie 1). Toute réponse autre que `y`, ou une saisie qui diffère du domaine du serveur (`host` dans la configuration, insensible à la casse), ne modifie rien et se termine avec le code 1.
- La fermeture est écrite dans le journal d’audit sous `service_closure_closed`. Dans la même transaction, elle met fin à toutes les souscriptions de soutien en cours (motif `service_closed`) et met en file d’attente les tâches qui annulent immédiatement les souscriptions auprès du prestataire de paiement (`CancelNow`).
- Après la fermeture, l’API client ne laisse passer que ces requêtes : la lecture de votre propre compte et de vos paramètres, les exports (`/exports`, `/csv_exports`), les sessions (`/sessions`, `/authorized_apps`), `GET /instance`, et l’administration de la fermeture et de la file de tâches.
  Tout le reste est refusé avec `403` (`/api/v1/problems/service-closed`). Les flux s’arrêtent et Web Push n’envoie rien. La connexion et la réinitialisation du mot de passe fonctionnent toujours.
- Après la fermeture, le worker remet un `Delete` de l’acteur de chaque compte local aux pairs de la fédération, 100 à la fois (les comptes migrés en dernier). Il les envoie même lorsque `federation_mode` vaut `off` ou `inbound`, et jamais vers un domaine suspendu.
  `my admin closure status` (et `progress` dans `GET /api/v1/admin/closure`) indique combien de comptes ont un `Delete` enregistré, combien de remises restent à développer et le nombre de remises dans chaque état (`pending`, `succeeded`, `permanent_failure`, `skipped`, `dead`). Gardez le worker en marche pendant la remise.
  Lorsque toutes les remises sont terminées, une vérification qui s’exécute toutes les 10 minutes enregistre `delivery_completed` (`service_closure_delivery_completed` dans le journal d’audit), et les acteurs locaux répondent alors `410`.
  Le serveur ne relance pas de lui-même les remises `dead` ; relancez-les avec `my job retry`.

Une fois la date limite d’export passée, écrivez la page de fermeture statique que votre domaine sert lorsque le serveur est arrêté. La commande lit la base de données ; exécutez-la donc avant de détruire la base de données.

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- Sortie : `<dir>/<locale>.html` pour chaque locale et `<dir>/index.html` avec toutes les locales l’une après l’autre. Les pages utilisent le même modèle que `my maintenance export` et affichent le nom du serveur, la date de fermeture, le message de l’opérateur dans l’annonce et `status_page_url`.
  La sortie contient aussi des exemples de configuration pour le frontal : `nginx.conf`, `Caddyfile` et `cloudfront-function.js`. Ils répondent `410` pour `/ap/*`, `/inbox`, WebFinger, host-meta, NodeInfo et `/@*`, et servent la page de fermeture pour tout le reste.
- La commande refuse (code de sortie 1) tant que le serveur n’est pas fermé. Avant `delivery_completed`, elle affiche un avertissement et écrit tout de même les pages. [service-closure.md](service-closure.md) décrit comment basculer le frontal vers la page statique.

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## Retraiter les images téléversées

Chaque image téléversée conserve trois tailles, et une animation conserve aussi ses versions animées. Une nouvelle version de `my` peut modifier ces tailles ou ces formats, et une image existante garde ses fichiers jusqu’à ce que vous la retraitiez :

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` prend l’heure du changement, par exemple l’heure de votre mise à niveau, au format RFC 3339. Seules les images dernièrement traitées avant cette heure sont prises, et une heure qui n’est pas encore passée est refusée.
- Le worker effectue le travail, 100 images par minute, chacune dans une tâche distincte (`media.reprocess_walk` et `media.reprocess` dans la [référence des tâches](../reference/jobs.md)). Gardez un worker en marche ; la page d’administration des tâches (`/admin/jobs`) montre la file.
- Le relancer avec la même heure est sans danger. Les images déjà traitées sont laissées de côté, et celles que la première exécution n’a pas atteintes sont reprises.
- Les originaux ne sont pas conservés ; chaque image est donc refaite à partir de sa version grande taille. Les détails que la version grande taille a perdus ne reviennent pas, et chaque exécution avec une heure ultérieure réencode les images une fois de plus et perd un peu de qualité. Exécutez-la une fois par changement.
- Les fichiers vidéo et audio ne sont pas réencodés et sont laissés de côté. Une image que les limites actuelles n’acceptent plus conserve sa version actuelle, et le worker consigne un avertissement.

La nouvelle version d’une image a de nouvelles URL. Les anciens fichiers restent pendant `media_replaced_retention` (30 jours par défaut, [référence de configuration](../reference/configuration.md#media_replaced_retention)), de sorte que les pages ouvertes et les caches continuent de fonctionner, puis la tâche horaire `media.purge_replaced` les supprime. Les autres serveurs ne sont pas informés des nouvelles URL : un serveur qui détient encore une ancienne URL et n’a pas de copie propre cesse d’afficher l’image après ce délai. Prévoyez le stockage supplémentaire jusqu’à la disparition des anciens fichiers.

<!-- source: rebuilding-the-search-index 9061821656ac -->
## Reconstruire l’index de recherche

`my search` gère l’index de la recherche plein texte des publications, sur un serveur doté d’un moteur de recherche. Avec `search = none`, chaque commande l’indique, ne fait rien et se termine avec le code 0.

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

Relancez `my search rebuild` pour reprendre une reconstruction interrompue. Elle repart de la dernière publication traitée et ne vide pas de nouveau l’index. Tant qu’une reconstruction est inachevée, l’autre mode est refusé avec le code de sortie 1. [Recherche plein texte des publications](search.md) indique quand utiliser chacun.

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## Exporter la page de maintenance

`my maintenance export` écrit la page du mode maintenance sous forme de HTML statique,
afin qu’un répartiteur de charge, un CDN ou un reverse proxy la serve tant que `my serve` lui-même ne peut pas s’exécuter (une image en cours de remplacement, une révision qui ne démarre pas).
Elle utilise le même modèle que le mode maintenance ; les deux affichent donc les mêmes mots et le même aspect.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Sortie : `<dir>/<locale>.html` pour chaque locale de `-lang`, pour les frontaux qui choisissent une page selon `Accept-Language`,
  et `<dir>/index.html` avec toutes les locales l’une après l’autre, pour ceux qui ne le peuvent pas.
  Les pages ont du CSS en ligne, aucun script et aucune ressource externe ; leur Content-Security-Policy se trouve dans un élément `<meta>`.
- Textes : les paramètres `maintenance_server_name`, `maintenance_message`, `maintenance_until` et `status_page_url`
  ([référence de configuration](../reference/configuration.md)).
- Sans `-offline`, elle lit aussi le nom du serveur (lorsque `maintenance_server_name` n’est pas défini), sa locale par défaut (lorsque `-lang` n’est pas donné)
  et les surcharges de jetons de son thème. Le CSS personnalisé du thème n’est jamais inclus. Lorsque la base de données ne peut pas être lue,
  elle affiche un avertissement, écrit les pages à partir de la configuration et du thème par défaut, et se termine avec le code 0.
- Relancez-la chaque fois que ces paramètres ou le thème changent, et avant chaque maintenance, afin que les pages du frontal restent à jour.
