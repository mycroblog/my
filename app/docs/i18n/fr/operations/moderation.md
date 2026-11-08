<!-- source: account-and-domain-actions-and-handling-reports 59d39b2e3f37 -->
# Mesures sur les comptes et les domaines, et traitement des signalements

Comment les opérateurs et les modérateurs agissent sur les comptes et les domaines, tranchent les contestations, traitent les signalements et lisent le journal d’audit. Tout ce qui figure ici, sauf la gestion des comptes (API d’administration uniquement), peut se faire via l’API d’administration (`/api/v1/admin/...`, portées `admin:read` / `admin:write`) et la CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`). L’API d’administration exige des permissions de rôle : la gestion des comptes, les mesures et les contestations nécessitent `manage_accounts`, les mesures sur les domaines nécessitent `manage_domains`, et les signalements nécessitent `manage_reports`. La CLI se connecte directement à la base de données avec la même configuration que le serveur (`-config` ou variables d’environnement) et n’enregistre pas quel utilisateur l’a utilisée (`issued_by_user_id` et les champs semblables valent NULL, et l’acteur dans le [journal d’audit](#audit-log) est `cli`).

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## D’un signalement à une mesure et à une contestation

![Le parcours d’un signalement : il arrive, un modérateur l’examine, une mesure peut suivre avec un avis à l’utilisateur, et l’utilisateur peut la contester ; la colonne de droite montre ce que chaque étape envoie aux autres serveurs. La liste ci-dessous décrit chaque étape.](images/moderation-lifecycle.svg)

Du haut du schéma vers le bas :

1. **Un signalement arrive.** Un utilisateur local en dépose un, un autre serveur envoie un `Flag` (abandonné lorsque son domaine a `reject_reports`, voir [mesures sur les domaines](#domain-actions)), ou un [point de décision](decision-hooks.md) répond `flag` ([signalements déposés par un examen externe](#reports-that-an-external-review-filed)). Un utilisateur local qui signale un compte d’un autre serveur peut choisir de transmettre le signalement, ce qui envoie aussitôt un `Flag` à ce serveur.
2. **Examen.** Le signalement est `open`, et les modérateurs reçoivent une notification `admin_report`. Le résoudre le fait passer à `resolved` avec le résultat `actioned` ou `dismissed`, et un auteur de signalement local reçoit une notification `report_resolved`. Le rouvrir le remet à `open`. Transmettre un signalement concernant un compte d’un autre serveur envoie un `Flag` à ce serveur ([signalements](#reports)).
3. **Mesure.** Un modérateur prend une [mesure sur un compte](#account-actions) depuis le signalement, ce qui résout le signalement comme `actioned` ; une mesure peut aussi être prise sans signalement. La mesure a un type, un motif et une heure de fin, et prend fin lorsqu’elle est levée ou qu’elle expire. Avec `notify_target`, l’utilisateur reçoit une notification `moderation_notice` ([ce que voit l’utilisateur](#what-the-user-sees)). Les autres serveurs reçoivent un `Delete` des publications pour `delete_statuses`, un `Update` de l’acteur lorsque le `suspend` ou le `ban` d’un compte local commence ou se termine, et un `Reject` / `Undo` des abonnements que coupe le `suspend` d’un compte distant.
4. **Contestation.** L’utilisateur local conteste la mesure. La contestation est `pending`, et les modérateurs reçoivent une notification `admin_appeal`. La décision est `upheld` ou `overturned`, et l’utilisateur reçoit une notification `appeal_decided`. Annuler la mesure la lève et annule la suppression d’un `ban`, donc un `suspend` ou un `ban` levé par une contestation acceptée sur un compte local envoie un `Update` de l’acteur ([contestations](#appeals)).

Les attributions, les notes, les avis et les contestations restent sur ce serveur ; seules les activités de la colonne de droite le quittent.

<!-- source: moderation-and-server-administration-in-the-web-client 3beb1c9760a0 -->
## Modération et administration du serveur dans le client web

Le client web répartit les écrans de l’équipe en deux groupes. La navigation n’affiche « Modération » et « Administration » qu’aux personnes dont le rôle a au moins une permission de ce groupe. Chaque écran n’affiche qu’une explication aux personnes sans la permission, et le serveur rejette aussi leurs requêtes.

| Groupe | Permissions | Écrans |
| --- | --- | --- |
| Modération (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Signalements (`/moderation/reports`), Contestations (`/moderation/appeals`), Comptes (`/moderation/accounts`), Inscriptions (`/moderation/registrations`), Blocages d’inscription (`/moderation/signup-blocks`), Invitations (`/moderation/invites`), Mesures sur les domaines (`/moderation/domain-actions`), Serveurs distants (`/moderation/instances`), Journal d’audit (`/moderation/audit-log`) |
| Administration du serveur (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Paramètres du serveur (`/admin/settings`), Règles (`/admin/rules`), Conditions et politiques (`/admin/documents`), la file de tâches et les tâches échouées (`/admin/jobs`), Émojis personnalisés (`/admin/custom-emojis`), Annonces (`/admin/announcements`), Webhooks (`/admin/webhooks`), coûts et import de facturation (`/admin/finance`, `manage_finance`). `manage_login_recovery` (récupération de connexion : suppression de l’authentification à deux facteurs et émission d’un lien de réinitialisation du mot de passe) et `manage_roles` (attribution et retrait des rôles d’un compte) apparaissent comme des sections de la page de gestion d’un compte (`/moderation/accounts/{id}/manage`, qui exige aussi `manage_accounts`) |

- Le badge sur « Modération » est le total des signalements ouverts, des inscriptions en attente d’approbation et des contestations en attente d’examen ([notifications pour les modérateurs et compteurs en attente](#notifications-for-moderators-and-the-pending-counts)). « Administration » n’affiche aucun compteur.
- Chaque groupe s’ouvre sur une vue d’ensemble ; les écrans eux-mêmes se trouvent dans le menu à côté. La vue d’ensemble de la modération a une tuile pour chaque file que compte votre rôle (signalements ouverts, inscriptions en attente d’approbation, contestations en attente de décision), chacune étant un lien vers sa file, une tuile indiquant depuis combien de temps le plus ancien signalement ouvert attend (un lien vers la liste des signalements, du plus ancien au plus récent), et liste les signalements ouverts à traiter en premier dans l’ensemble de la file : d’abord les contenus illégaux, puis ceux qui attendent depuis le plus longtemps, avec la durée d’attente de chacun et la personne qui s’en occupe. La liste des signalements se filtre par état, catégorie et responsable (vous ou personne) et affiche les plus récents ou les plus anciens en premier ; les filtres restent dans l’adresse de la page. La vue d’ensemble de l’administration montre la santé du serveur : les tâches échouées, depuis combien de temps la file de tâches fait attendre sa plus ancienne tâche, et les erreurs ouvertes, chaque tuile étant un lien vers son écran et marquée lorsqu’elle demande de l’attention.
- Le rôle intégré `moderator` n’a que `manage_reports`, `manage_accounts` et `view_audit`, et ne voit pas « Administration ». Il ne voit pas non plus les écrans Mesures sur les domaines et Serveurs distants (`manage_domains`), donc pour déléguer ceux-ci, créez un rôle avec cette permission et attribuez-le ([attribuer et retirer des rôles](admin-cli.md#granting-and-revoking-roles)).
- Les chemins de l’API d’administration restent `/api/v1/admin/...` quel que soit le groupe, et les portées sont toujours la même paire `admin:read` / `admin:write`. Les groupes sont séparés par des balises dans la référence de l’API (`admin-moderation`, `admin-server`).
- Il existe un troisième groupe de permissions, les « fonctionnalités utilisateur » (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`), qui se trouve en dehors des écrans de l’équipe. Une personne qui n’a que ces permissions ne voit ni « Modération » ni « Administration », et pour une personne avec `view_finance`, la navigation affiche « Finances » (`/finance`) après les paramètres. Son API se trouve dans l’espace de noms utilisateur (`GET /api/v1/finance/cost_totals` et `GET /api/v1/finance/income_totals`, portée `read:finance`) et n’exige aucune portée `admin:*`. Ce qu’elle affiche est défini par `finance_viewer_*` dans les [paramètres d’instance](instance-settings.md).

<!-- source: account-actions a61dac7c5921 -->
## Mesures sur les comptes

Une mesure est appliquée à la projection des restrictions (`account_restrictions`) dans la transaction qui l’enregistre, et prend effet dès la requête suivante.

| Type | Effet sur un compte local |
| --- | --- |
| `warn` | Aucune restriction (informe seulement l’utilisateur) |
| `force_sensitive` | Affiche toutes les pièces jointes comme sensibles |
| `limit` | Traite les publications (y compris passées) comme réservées aux abonnés et les garde hors des fils publics |
| `restrict_posting` | Interdit de publier, de répondre, de booster et de modifier |
| `restrict_interaction` | Interdit de suivre, de mettre en favori, de booster et de répondre |
| `disable_login` | Interdit l’usage de l’application et de l’API, et révoque les sessions et jetons actifs (la page de connexion permet quand même à l’utilisateur de voir la mesure) |
| `suspend` | Applique toutes les restrictions ci-dessus, et masque le profil et les publications à tout le monde sauf à l’utilisateur |
| `ban` | Applique les mêmes restrictions que `suspend` et supprime le compte après 30 jours. Elle ne peut pas être levée (elle n’est annulée que si une contestation faite pendant ces 30 jours est acceptée) |
| `delete_statuses` | Supprime les publications indiquées. Elles deviennent invisibles même pour l’utilisateur, et le serveur envoie aux autres serveurs le même `Delete` que lorsque l’utilisateur les supprime |
| `mark_statuses_sensitive` | Affiche les pièces jointes des publications indiquées comme sensibles |

Pour un compte distant, vous ne pouvez prendre que `limit`, `suspend` et `force_sensitive` (tout autre type donne `422` dans l’API d’administration et échoue dans la CLI). L’effet est local à ce serveur. Les seules choses envoyées à l’autre serveur sont le `Reject` / `Undo` des abonnements coupés par `suspend`.

| Type | Effet sur un compte distant |
| --- | --- |
| `force_sensitive` | Affiche toutes les pièces jointes comme sensibles |
| `limit` | Traite les publications (y compris passées) comme réservées aux abonnés, les garde hors des fils publics, et ne crée aucune notification pour les utilisateurs qui ne suivent pas le compte |
| `suspend` | Abandonne tout ce qui est reçu du compte et masque son profil et ses publications. Coupe les abonnements entre le compte et les utilisateurs locaux dans les deux sens (demandes d’abonnement comprises), et envoie à l’autre serveur un `Reject` des abonnements du compte et un `Undo` des abonnements des utilisateurs. La lever rétablit l’affichage et la réception depuis le compte (ce qui a été abandonné et les abonnements coupés pendant la suspension ne reviennent pas) |

- `notify_target` (`-no-notify`) n’a aucun sens pour un compte distant (aucune notification ne l’atteint).
- Un utilisateur local qui a perdu des abonnements à cause d’un `suspend` reçoit une notification (`relationships_severed`) qui indique combien d’abonnés et d’abonnements ont été perdus ([abonnements rompus](#severed-follows)).
- La prise et la levée d’une mesure sont consignées dans le [journal d’audit](#audit-log), comme pour les mesures sur les comptes locaux.

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

Dans l’API d’administration : `POST /api/v1/admin/accounts/{id}/actions` (le corps contient `kind`, `reason_category`, `public_reason`, `private_note`, `report_id`, `notify_target`, `expires_in` en secondes et `status_ids`), `GET /api/v1/admin/accounts/{id}/actions` (filtrez avec `active`), `GET /api/v1/admin/account_actions/{id}`, et `POST .../{id}/lift` (le corps `{"reason": "..."}` est facultatif).

- Désignez un compte avec `@username` (local), `@username@domain` (distant) ou l’ID du compte dans la CLI, et avec l’ID du compte dans l’API d’administration.
- Plusieurs mesures s’empilent (l’union de leurs restrictions). Lever une mesure ne retire que ses propres restrictions, et une mesure ayant une heure de fin se termine d’elle-même à ce moment (affichée comme `expired` dans les listes). Une heure de fin est au plus à dix ans (`expires_in` jusqu’à 315360000) ; une heure plus lointaine est refusée avec `422`.
- Les sessions et jetons que `disable_login`, `suspend` et `ban` ont révoqués ne reviennent pas lorsque vous levez la mesure. L’utilisateur se reconnecte.
- Un `suspend` ou un `ban` d’un compte local envoie un `Update` de l’acteur avec un profil vidé aux serveurs des abonnés et aux serveurs connus pour montrer la suspension (Mastodon l’affiche comme « suspendu »). La levée et l’expiration envoient un `Update` avec le profil d’origine. L’expiration est envoyée par une tâche qui s’exécute toutes les 5 minutes, elle peut donc avoir jusqu’à 5 minutes de retard.
- `delete_statuses` et `mark_statuses_sensitive` prennent de 1 à 100 publications non supprimées du compte cible via `-status` (`status_ids`). Elles ne peuvent pas avoir d’heure de fin. Lever `delete_statuses` ne rétablit pas les publications supprimées. Lever `mark_statuses_sensitive` rétablit l’affichage d’origine.
- Un `ban` ne peut pas avoir d’heure de fin et ne peut pas être levé. Il n’est annulé que si une contestation que l’utilisateur a faite dans les 30 jours précédant la suppression est acceptée ([contestations](#appeals)). En cas de doute, utilisez un `suspend` avec une heure de fin. Trente jours après la mesure, une tâche qui s’exécute toutes les heures supprime le compte (le compte et les publications disparaissent de la vue, et le serveur envoie un `Delete` aux autres serveurs).
- `report_id` (`-report`) ne peut désigner qu’un signalement concernant le compte cible. Un signalement ouvert est résolu comme `actioned` en même temps que la mesure (la résolution figure aussi dans le journal d’audit). Un signalement résolu ne change pas.
- `public_reason` est le motif montré à l’utilisateur et `private_note` est une note réservée aux modérateurs. `notify_target` (vrai par défaut ; faux avec `-no-notify` dans la CLI) décide si l’utilisateur est informé.

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### Ce que voit l’utilisateur

- Une mesure avec `notify_target` vrai parvient à l’utilisateur sous la forme d’une notification (`moderation_notice`). Elle contient le type, la catégorie du motif, `public_reason`, l’heure de fin et l’état. Le type et la catégorie reviennent aussi sous forme de noms d’affichage dans la langue de l’interface de l’utilisateur.
- L’utilisateur peut consulter les mesures qui le visent avec l’API de première partie `GET /api/v1/moderation/actions` (filtrable avec `active`), de la plus récente à la plus ancienne. Les mesures avec `notify_target` faux et les mesures levées ou expirées sont aussi listées.
- `private_note`, le signalement, la personne qui a pris la mesure et le motif de la levée ne sont jamais montrés à l’utilisateur.
- Un utilisateur peut contester chaque mesure une fois ([contestations](#appeals)). La liste des mesures et la mesure dans une notification indiquent si elle peut être contestée (`appealable`), l’échéance (`appeal_deadline`), ainsi que la contestation et son issue (`appeal`).
- Un utilisateur soumis à `disable_login`, `suspend` ou `ban` ne peut pas utiliser l’application ni l’API mais peut quand même se connecter sur la page de connexion (`/auth/sign_in`). Il y voit le type, le motif et l’heure de fin de la mesure en vigueur (pour `ban`, le nombre de jours avant la suppression du compte), et peut la contester depuis cette page ([contestations](#appeals)). Le point de terminaison d’autorisation (`/oauth/authorize`) répond `403` et n’émet aucun jeton. Un utilisateur soumis à `ban` peut se connecter jusqu’à la suppression du compte, et la suppression révoque aussi cette session. Un utilisateur dont la propre suppression est programmée, et un compte qui a été désactivé, ne peuvent pas se connecter, même sur la page de connexion.

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### Comptes que leurs propriétaires ont désactivés

Un utilisateur peut désactiver son propre compte depuis les paramètres du compte, après avoir saisi à nouveau son mot de passe ou un code à deux facteurs, ou confirmé avec une clé d’accès (un utilisateur sans aucun de ces éléments se reconnecte chez un fournisseur de connexion associé). Le compte n’est pas une mesure de modération et les modérateurs ne peuvent désactiver personne :

- Le profil et les publications sont masqués à tout le monde sauf aux modérateurs, et les autres serveurs reçoivent un `Update` de l’acteur marqué `suspended`, de sorte qu’ils peuvent afficher le compte comme suspendu. Chaque session et chaque jeton de l’utilisateur prennent fin. Là où le serveur envoie des e-mails, un e-mail indique la date de la suppression ; aucun autre e-mail n’est envoyé avant.
- Rien n’atteint le compte : les mentions et les messages directs sont abandonnés, aucune notification, aucun e-mail ni Web Push n’est généré, et les nouveaux abonnés sont refusés. Les favoris et les boosts sont comptés. Les abonnements existants restent.
- L’utilisateur se connecte à une page du serveur, qui montre la date de la suppression et lui permet de réactiver ou de supprimer le compte. La réactivation rétablit tout. La désactivation suivante doit attendre 24 heures.
- Trente jours après la désactivation, la tâche horaire supprime le compte. Une réactivation après cette date est refusée même si la tâche ne s’est pas encore exécutée. Le nom d’utilisateur n’est pas libéré.

Les modérateurs voient l’état `deactivated` dans la liste des comptes (`state=deactivated`), et peuvent toujours lire les signalements concernant le compte et agir dessus. Une mesure de modération ne change pas la date de suppression, sauf un `ban`, qui la remplace par la sienne : dès lors l’utilisateur ne peut plus réactiver. Une suspension n’envoie rien aux autres serveurs tant que le compte est désactivé, et elle reste en vigueur après que l’utilisateur a réactivé. Un utilisateur ne peut pas désactiver un compte tant que la modération l’empêche de se connecter, tant que sa suppression est programmée, après un déplacement, tant que son inscription attend un examen, tant qu’une souscription payante est encore en cours (y compris une souscription réglée pour se terminer à la fin de sa période), ou lorsqu’il est le seul administrateur pouvant se connecter. Un administrateur désactivé ne compte pas comme tel. Si la date d’un administrateur désactivé arrive alors qu’aucun autre administrateur ne peut se connecter, la tâche horaire ne supprime pas le compte et consigne un avertissement. Cet administrateur ne peut plus se connecter non plus, créez donc un autre administrateur avec `my admin create` ; l’exécution suivante supprime alors le compte.

<!-- source: appeals 8a6f1b4c288d -->
## Contestations

Un utilisateur local peut contester une mesure prise contre lui. Un modérateur examine la contestation et la rejette (`uphold`, la mesure est maintenue) ou l’accepte (`overturn`, la mesure est levée).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

L’utilisateur conteste avec `POST /api/v1/moderation/actions/{id}/appeal` (portée `write:accounts`, corps `{"text": "..."}`). Un utilisateur soumis à `disable_login`, `suspend` ou `ban` ne peut pas obtenir de jeton d’accès et conteste via le formulaire propre à chaque mesure sur la page de connexion (`POST /auth/appeal`). Tant que la [connexion est limitée aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers), un tel utilisateur n’atteint le formulaire que via un fournisseur associé. La page affiche le formulaire et l’échéance pour une mesure qui peut être contestée, le contenu et l’issue pour une mesure déjà contestée, et la raison pour une mesure dont l’échéance est passée ou qui ne peut pas être contestée. Pour une contestation rejetée, elle affiche la raison. Dans l’API d’administration : `GET /api/v1/admin/appeals` (filtrez avec `state`), `GET .../{id}`, et `POST .../{id}/uphold` et `POST .../{id}/overturn` (le corps `{"public_reason": "..."}` est facultatif). Elles exigent la permission `manage_accounts`.

- Une mesure peut être contestée une fois, et le texte compte de 1 à 2 000 caractères. L’échéance est le nombre de jours du paramètre d’instance `appeal_window_days` après la mesure (180 jours par défaut, de 20 à 365 ; voir [paramètres d’instance](instance-settings.md)). Une mesure après l’échéance et une seconde contestation donnent `422` (`/api/v1/problems/appeal-window-closed` et `/api/v1/problems/already-appealed`).
- Un `ban` peut être contesté jusqu’à la suppression du compte (30 jours après la mesure, ou jusqu’à l’échéance `appeal_window_days` si elle est plus courte). Un `ban` d’un compte supprimé donne `422` (`/api/v1/problems/not-appealable`). Une mesure levée ou expirée peut encore être contestée dans le délai.
- Lorsqu’un utilisateur conteste, les personnes disposant de `manage_accounts` reçoivent une notification (`admin_appeal`). Lorsqu’elle est examinée, l’utilisateur reçoit une notification (`appeal_decided`) et peut voir l’issue et `public_reason`. Le réviseur n’est pas montré.
- Accepter la contestation lève la mesure dans la même transaction (même effet que `lift`, avec un motif de levée vide). Accepter la contestation de `delete_statuses` ne rétablit pas les publications supprimées. Une mesure déjà levée avant l’examen reste telle quelle.
- Accepter la contestation d’un `ban` annule aussi la suppression programmée du compte dans la même transaction (le journal d’audit consigne `account_deletion_canceled`), met fin à la suspension, et envoie l’`Update` du profil d’origine aux autres serveurs. Les sessions et jetons révoqués ne reviennent pas, l’utilisateur se reconnecte donc. Si le compte a été supprimé avant l’examen, elle ne peut pas être acceptée (`422`, `/api/v1/problems/not-appealable` ; la rejeter reste possible).
- Un examen n’a lieu qu’une fois. Examiner à nouveau avec la même issue ne change rien, et l’issue opposée donne `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management 99149899a198 -->
## Gestion des comptes

La recherche de comptes, la consultation de leurs détails et l’affichage des adresses IP et e-mail se font via l’API d’administration et exigent la permission `manage_accounts`.

Dans le client web, un compte a trois pages, avec des liens entre elles sous le titre de la page :

- **Examen** (`/moderation/accounts/{id}`) : l’état et les restrictions du compte, le formulaire d’une nouvelle mesure, les mesures les plus récentes qui le visent avec leurs motifs, et ses 20 signalements les plus récents. Les signalements plus anciens ne se trouvent que dans la liste des signalements (`/moderation/reports`), qui n’a pas de filtre par compte.
- **Mesures** (`/moderation/accounts/{id}/actions`) : chaque mesure sur le compte, une page à la fois, avec « Lever » pour celles en vigueur. Chaque mesure ouvre sa propre page (`/moderation/accounts/{id}/actions/{action id}`) avec tous ses détails.
- **Gestion** (`/moderation/accounts/{id}/manage`, comptes locaux uniquement) : les rôles du compte (`manage_roles`), sa suppression d’envoi d’e-mails, la suppression de l’authentification à deux facteurs et l’émission d’un lien de réinitialisation du mot de passe (`manage_login_recovery`), ainsi que ses adresses e-mail et IP. Chaque section n’apparaît qu’aux personnes disposant de la permission correspondante.

L’équipe n’agit que sur des rangs inférieurs : prendre ou lever une mesure, trancher une contestation, lever une suppression d’envoi d’e-mails, afficher des données privées, émettre un lien de réinitialisation du mot de passe et supprimer l’authentification à deux facteurs exigent que la `position` la plus élevée parmi vos rôles soit supérieure à celle de chaque rôle de la cible. Un rang supérieur ou égal, votre propre compte compris, renvoie `403` avec le type de problème `/api/v1/problems/target-outranks-actor`, de sorte qu’un modérateur ne peut pas bannir un administrateur et qu’un administrateur ne peut pas en bannir un autre. Les commandes `my admin` n’ont pas cette limite. Une mesure qui empêche le seul administrateur pouvant se connecter de se connecter (`disable_login`, `suspend`, `ban`) est refusée quelle que soit la personne qui la prend, et cet administrateur ne peut pas supprimer son propre compte (`409` avec `/api/v1/problems/last-administrator`). Voir [admin-cli.md](admin-cli.md) pour les rôles.

- `GET /api/v1/admin/accounts` renvoie les comptes locaux et distants, du plus récent au plus ancien (l’acteur de l’instance est exclu). Vous pouvez filtrer par préfixe de nom d’utilisateur (`username`), domaine (`domain`), local ou distant (`origin`), état (`state`) et, pour les comptes locaux, adresse e-mail (`email`), domaine de l’e-mail (`email_domain`) et adresse IP utilisée pour s’inscrire ou se connecter (`ip`). Vous ne pouvez donner qu’un seul parmi `email`, `email_domain` et `ip`. La liste n’inclut ni adresses IP ni adresses e-mail.
- L’état est le premier qui s’applique dans cette liste : `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` et `pending` (examen de l’inscription), `limited` (`limit`), `active`. L’état et les restrictions (`restrictions`) proviennent uniquement des mesures visant le compte lui-même et n’incluent pas les mesures sur les domaines.
- `GET /api/v1/admin/accounts/{id}` renvoie l’état et les restrictions et, pour un compte local, la demande d’inscription et son examen, la confirmation de l’e-mail, les rôles, ainsi que jusqu’à 20 mesures et signalements, du plus récent au plus ancien.
- `POST /api/v1/admin/accounts/{id}/reveal` renvoie l’adresse e-mail d’un compte local, les adresses IP et User-Agents de l’inscription et des connexions (20 entrées, de la plus récente à la plus ancienne), et les autres comptes qui ont utilisé la même adresse IP (jusqu’à 20 par adresse IP). Chaque appel consigne `revealed_private_data` dans le journal d’audit, et les données ne sont renvoyées que si l’enregistrement a pu être écrit. Les adresses IP sont supprimées après la durée de conservation (180 jours par défaut), donc les origines de connexion plus anciennes ne peuvent pas être affichées.
- `two_factor_enabled` dans `GET /api/v1/admin/accounts/{id}` indique si l’utilisateur d’un compte local utilise l’authentification à deux facteurs (TOTP). Un utilisateur qui a perdu à la fois l’application d’authentification et les codes de récupération peut se connecter avec le seul mot de passe une fois que vous avez supprimé l’authentification à deux facteurs avec `POST /api/v1/admin/accounts/{id}/two_factor/disable`. Seuls les utilisateurs disposant de la permission `manage_login_recovery` peuvent le faire, et par défaut seul le rôle d’administrateur intégré l’a (elle relève de l’administration du serveur ; le client web n’affiche la section que pour eux). La suppression consigne `two_factor_disabled` dans le journal d’audit. Vérifiez par une autre voie, comme l’e-mail, que la personne est bien la propriétaire du compte avant de la supprimer. Lorsqu’un administrateur sans autre administrateur l’a perdue, l’opérateur la supprime avec la commande CLI `my admin account disable-two-factor @alice` (l’acteur du journal d’audit est la CLI).
- Sur un serveur qui envoie des e-mails, un utilisateur qui a oublié son mot de passe reçoit par e-mail un lien pour en définir un nouveau depuis « Mot de passe oublié ? » sur la page de connexion. Pour un serveur qui n’envoie pas d’e-mails (`mail = none`), ou un utilisateur que les e-mails n’atteignent pas, un administrateur émet le lien et le remet. Dans les écrans d’administration, émettez-le avec « Réinitialisation du mot de passe » sur la page de gestion du compte (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`). Seuls les utilisateurs disposant de la même permission que pour supprimer l’authentification à deux facteurs, `manage_login_recovery`, peuvent l’émettre, et par défaut seul le rôle d’administrateur intégré l’a (quiconque détient le lien peut accéder au compte, donc le `manage_accounts` d’un modérateur ne suffit pas). Le lien émis est affiché une fois sur place et peut être copié. Le serveur ne stocke qu’une empreinte du lien, vous ne pouvez donc pas le revoir plus tard ; émettez-le à nouveau si nécessaire. Lorsqu’aucun administrateur n’est disponible ou que les écrans d’administration ne le sont pas, l’opérateur l’émet avec la commande CLI `my admin account password-reset-link @alice` (l’acteur du journal d’audit est la CLI). Tant que la [connexion est limitée aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers), les deux continuent d’émettre le lien et indiquent qu’il ne fonctionne pas avant la fin de la limite. Le lien fonctionne pendant 1 heure et une seule fois, et cesse de fonctionner si le mot de passe change ensuite par une autre voie. Les deux façons de l’émettre consignent `password_reset_link_issued` dans le journal d’audit. Quiconque détient le lien peut définir le mot de passe, vérifiez donc la personne par une autre voie comme l’e-mail et ne donnez le lien qu’à elle. Définir le nouveau mot de passe révoque toutes les sessions et tous les jetons d’application de l’utilisateur. Les clés d’accès et l’authentification à deux facteurs restent.
- `email_suppression` dans `GET /api/v1/admin/accounts/{id}` indique que l’envoi vers l’adresse principale d’un compte local est arrêté. Un `hard_bounce` (la destination n’existe pas) arrête tous les e-mails sauf l’e-mail de confirmation, et un `complaint` (signalé comme indésirable) n’arrête que les e-mails de notification et d’annonce. Les e-mails de réinitialisation du mot de passe s’arrêtent aussi, émettez donc le lien ci-dessus et remettez-le si nécessaire. Lorsque l’utilisateur demande un e-mail de confirmation et confirme à nouveau l’adresse, la suppression due à un rebond définitif est levée. Lorsque vous avez vérifié que l’envoi est possible, par exemple parce que l’utilisateur a corrigé sa boîte aux lettres, levez-la avec `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permission `manage_accounts`). Elle consigne `email_suppression_lifted` dans le journal d’audit.
- Une recherche lit jusqu’à 400 comptes pour remplir une page. Lorsque peu de comptes correspondent, même une page courte ou vide peut porter `next_cursor`, continuez donc à lire.

<!-- source: domain-actions 13dd3d4d3a0f -->
## Mesures sur les domaines

| Type | Effet |
| --- | --- |
| `suspend` | Arrête toute livraison, récupération et réception (la réception reçoit `403` avant la vérification de la signature). Les comptes et publications existants sont masqués des fils et des profils. Coupe les abonnements entre les utilisateurs locaux et les comptes du domaine dans les deux sens (rien n’est envoyé de l’autre côté) |
| `limit` | Garde les publications hors des fils publics et de la recherche, et ne crée aucune notification pour les utilisateurs qui ne suivent pas le compte |
| `reject_media` | Ne récupère pas les médias |
| `reject_reports` | Abandonne les signalements (`Flag`) du domaine |
| `force_sensitive` | Affiche les médias comme sensibles |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

Dans l’API d’administration : `POST /api/v1/admin/domain_actions` (le corps contient `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` et `expires_in` en secondes), `GET /api/v1/admin/domain_actions` (filtrez avec `domain` et `active`), `GET .../{id}`, et `POST .../{id}/lift`.

Dans le client web, ouvrez Modération → Mesures sur les domaines (`/moderation/domain-actions`). La liste affiche le domaine, le type, l’état et la fin de chaque mesure, en vigueur ou toutes ; « Nouvelle mesure » ouvre le formulaire (`/moderation/domain-actions/new`), et choisir un domaine ouvre la page de la mesure avec ses commentaires et ses dates, où « Lever » y met fin. Le lien « Mesures sur les domaines » sur la page d’un serveur distant ne liste que les mesures de ce domaine, et « Nouvelle mesure » y démarre le formulaire avec le domaine déjà renseigné.

- Les domaines sont enregistrés normalisés en A-label IDNA (minuscules, sans point final). `例え.jp` et `xn--r8jz45g.jp` sont le même domaine.
- Une mesure s’applique aussi aux sous-domaines (une mesure sur `spam.example` s’applique aussi à `a.spam.example`).
- Une mesure est appliquée à la projection des restrictions (`domain_restrictions`) et aux restrictions de fil dans la transaction qui l’enregistre, et prend effet dès la livraison, la récupération ou la réception suivante.
- Une mesure avec une heure de fin (`-duration` / `expires_in`, au plus dix ans) se termine d’elle-même à ce moment. L’expiration n’est pas une « levée », donc les listes l’affichent comme `expired`.
- La levée ne retire que les restrictions de cette mesure. Les autres mesures sur le même domaine restent. Lever une mesure déjà levée ne change rien.
- Les listes vont de la plus récente à la plus ancienne, et `-active` (`active=true`) n’affiche que les mesures en vigueur.

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### Importer et exporter un CSV Mastodon

Vous pouvez transmettre la liste des mesures sur les domaines vers Mastodon et depuis Mastodon au format CSV des blocages de domaines de Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`). Vous pouvez importer un CSV exporté par Mastodon, et Mastodon peut importer un CSV exporté par ce serveur.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

Dans l’API d’administration : `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (le corps contient `csv`), et `POST .../import` (le corps contient `blocks`, les lignes choisies, et `private_note`). Une ligne pour le propre domaine de ce serveur est ignorée (`own_domain`), et une mesure sur domaine le visant est refusée. Dans le client web, ouvrez Modération → Mesures sur les domaines et choisissez « CSV de Mastodon » (`/moderation/domain-actions/import`) : « Exporter le CSV » enregistre le fichier, et l’import prévisualise un fichier collé ou choisi avant d’importer les lignes que vous conservez.

| Ligne du CSV | Mesure sur ce serveur |
| --- | --- |
| `#severity` vaut `suspend` (ou est vide) | `suspend` |
| `#severity` vaut `silence` | `limit` |
| `#severity` vaut `noop` | Aucune |
| `#reject_media` / `#reject_reports` est vrai | Ajoute respectivement `reject_media` / `reject_reports` |

- Un import se déroule en deux étapes. Vérifiez d’abord dans l’aperçu quelles mesures chaque ligne prendrait, choisissez les lignes à conserver, puis importez (la CLI ne change rien tant que vous n’ajoutez pas `-apply`, et `-only` choisit les lignes par domaine).
- Une ligne dont le domaine ou un domaine parent a déjà une mesure du même type en vigueur ne prend pas ce type (si tous les types sont déjà là, elle est « already applied »). Une seconde ligne pour le même domaine, une ligne invalide et une ligne qui ne prendrait aucune mesure (`noop` seul) sont aussi ignorées. Importer à nouveau le même CSV n’ajoute pas de mesures.
- Un CSV sans en-tête `#domain` est lu comme une liste d’un domaine par ligne, et chaque domaine reçoit `suspend` (comme Mastodon). Un import accepte jusqu’à 5 000 lignes.
- Les mesures importées durent « until lifted » et portent le commentaire public et l’obfuscation de la ligne ainsi que la note privée que vous avez donnée. Elles apparaissent dans le journal d’audit comme les mesures prises une par une (l’acteur est la personne pour l’API et `cli` pour la CLI).
- Un export écrit une ligne par domaine pour les mesures en vigueur. Un domaine avec un `suspend` reçoit `suspend`, un domaine avec un `limit` reçoit `silence`, et un domaine sans l’un ni l’autre reçoit `noop`. Un domaine avec seulement `force_sensitive`, que les blocages de domaines de Mastodon n’ont pas, n’est pas exporté. Les notes privées et les heures de fin ne sont pas exportées.

<!-- source: severed-follows 51be435d8635 -->
### Abonnements rompus

Un `suspend` de domaine et un `suspend` de compte distant coupent les abonnements entre les utilisateurs locaux et les comptes suspendus dans les deux sens.

- Pour un `suspend` de domaine, une tâche (`moderation.sever_domain`) qui s’exécute après l’enregistrement de la mesure les coupe un utilisateur local à la fois. Tant que la tâche n’est pas terminée, les abonnements apparaissent encore dans les listes. Les comptes des sous-domaines sont aussi coupés.
- Pour un `suspend` de compte distant, les abonnements sont coupés dans la transaction qui enregistre la mesure. Un second `suspend` d’un compte déjà suspendu ne coupe rien.
- Chaque utilisateur qui a perdu des abonnements approuvés reçoit une notification (`relationships_severed`) qui indique combien d’abonnés et d’abonnements ont été perdus. Un utilisateur qui n’a perdu que des demandes d’abonnement n’en reçoit aucune.
- Lever la mesure ne rétablit pas les abonnements coupés. Les utilisateurs se réabonnent.
- Traiter à nouveau la même mesure (par exemple lorsqu’une tâche se réexécute) n’ajoute ni enregistrements de relations rompues ni notifications.

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### Publier les domaines visés par des mesures

Vous pouvez publier les mesures `limit` et `suspend` en vigueur via `GET /api/v1/instance/domain_actions`. Le paramètre d’instance `domain_actions_visibility` (`off` / `signed_in` / `everyone`, `off` par défaut) décide qui voit la liste, et `domain_actions_rationale_visibility` (mêmes valeurs, `off` par défaut) décide qui voit les commentaires publics ([paramètres d’instance](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Avec `off`, l’API renvoie `404`, et avec `signed_in` elle renvoie `401` à une requête sans jeton d’accès.
- Il y a une entrée par mesure, de la plus récente à la plus ancienne. Une entrée contient le domaine, le SHA-256 du domaine (hexadécimal) et le type, ainsi que le commentaire public pour les personnes autorisées à le voir. Les notes privées ne sont jamais renvoyées.
- Pour une mesure avec `-obfuscate` (`obfuscate`), la moitié centrale des caractères du domaine (hors points) est remplacée par `*` (`bad.example` devient `bad.*****le`). N’importe qui peut comparer le SHA-256 d’un domaine qu’il connaît pour vérifier s’il s’agit d’un domaine obfusqué.
- Les mesures `reject_media`, `reject_reports` et `force_sensitive`, ainsi que les mesures levées ou expirées, ne sont pas publiées.
- La page d’informations de l’instance du client web (`https://<host>/about`) affiche cette liste selon les mêmes paramètres. Avec `off`, la section n’est pas affichée, et avec `users` les personnes non connectées voient une note indiquant que seuls les utilisateurs connectés peuvent la voir.

<!-- source: refetching-remote-posts 883740b40c9a -->
## Récupérer à nouveau les publications distantes

Vous pouvez récupérer à nouveau depuis l’autre serveur les publications distantes stockées. Une récupération n’a lieu que lorsque vous la demandez via l’API d’administration, la CLI ou le client web, jamais périodiquement.

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

Dans l’API d’administration : `POST /api/v1/admin/statuses/{id}/refetch`, `POST /api/v1/admin/accounts/{id}/refetch_statuses` et `POST /api/v1/admin/instances/{domain}/refetch_statuses`. Elles exigent la permission `manage_domains`. Dans le client web, utilisez « Récupérer à nouveau les publications » sur la page d’examen du compte (`/moderation/accounts/{id}`, comptes distants) et sur l’écran du serveur distant (`/moderation/instances/{domain}`).

- Une publication est traitée comme un `Update` reçu. S’il existe une version plus récente, le serveur ajoute une version ; il met à jour les décomptes de votes des sondages ; et il ne fait rien si rien n’a changé. Une publication pour laquelle l’autre serveur renvoie 410 Gone ou un `Tombstone` est supprimée. Une publication qui ne peut pas être récupérée (404 compris) est laissée telle quelle.
- Pour un compte, le serveur récupère à nouveau jusqu’à 1 000 de ses publications stockées non supprimées les plus récentes, 100 à la fois, à une minute d’intervalle.
- Pour un domaine, le serveur récupère à nouveau en partant des comptes les plus récents du domaine, jusqu’à 20 des publications les plus récentes par compte et jusqu’à 1 000 publications au total, 5 comptes à la fois, à une minute d’intervalle.
- Il ne récupère rien depuis un domaine soumis à un `suspend`, ni depuis un domaine `unavailable` parce que des livraisons ont échoué. Un échec de récupération de la même URI est mémorisé pendant 1 heure, et une nouvelle récupération pendant ce délai ne la récupère pas.
- Votre demande est consignée dans le journal d’audit sous `status_refetch_queued`, `account_statuses_refetch_queued` ou `domain_statuses_refetch_queued`.

<!-- source: reports e8a3f112b323 -->
## Signalements

Les signalements (`Flag`) reçus d’autres serveurs entrent dans la liste des signalements comme ouverts (`open`).

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

Dans l’API d’administration : `GET /api/v1/admin/reports` (filtrez avec `state`, `category` et `assignee` : `me`, `none` ou l’ID d’un compte local ; `order=oldest` liste d’abord le signalement qui attend depuis le plus longtemps plutôt que le plus récent), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`, `POST .../{id}/resolve` (corps `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (corps `{"body": "..."}`), `POST .../{id}/reopen` et `POST .../{id}/forward` (corps `{"comment": "..."}`, facultatif).

- Une attribution (`assign`) ne peut aller qu’à un utilisateur local dont le rôle a `manage_reports`. Dans l’API d’administration, vous vous l’attribuez à vous-même.
- Le résultat d’une résolution (`resolve`) est `actioned` (une mesure a été prise contre la cible) ou `dismissed` (aucune mesure n’était nécessaire). Résoudre à nouveau un signalement résolu ne change pas le résultat.
- Lorsque vous prenez une mesure depuis un signalement (`report_id` / `-report` de la mesure), le signalement est résolu comme `actioned` dans la même opération.
- Une note (`note`) est une note interne que seuls les modérateurs voient, de 2 000 caractères au plus. Elle n’est montrée ni à la cible ni à l’auteur du signalement. Elle ne peut être ni modifiée ni supprimée.
- La réouverture (`reopen`) remet un signalement résolu à `open` (l’attribution reste). Un signalement ouvert ne peut pas être rouvert (`409` dans l’API d’administration).
- La transmission (`forward`, le bouton **Transmettre à son serveur** des détails du signalement) envoie un signalement concernant un compte d’un autre serveur à ce serveur, sous la forme d’un `Flag` de l’acteur de votre instance avec le compte et les publications signalées qui existent encore. Le seul texte qu’il porte est ce que vous écrivez pour les modérateurs de ce serveur lorsque vous le transmettez (`-comment`, `comment` dans l’API d’administration, ou **Texte pour leurs modérateurs** sur la page du signalement ; jusqu’à 1 000 caractères, aucun par défaut). Le commentaire de l’auteur du signalement et la note d’un [examen externe](#reports-that-an-external-review-filed) ne quittent jamais votre serveur de cette façon, et rien concernant l’auteur du signalement n’est envoyé. Un signalement que son auteur a choisi de transmettre en le déposant est parti aussitôt, avec le commentaire de l’auteur. La transmission fonctionne pour un signalement déposé sur ce serveur, par une personne ou par un [examen externe](#reports-that-an-external-review-filed), ouvert ou résolu, qui n’a pas encore été transmis ; un signalement reçu d’un autre serveur ou concernant un salon de chat vocal ne peut pas être transmis (`409` `report-not-forwardable` dans l’API d’administration, et les détails du signalement n’affichent aucun bouton). Un signalement n’est envoyé qu’une fois : le transmettre à nouveau, avec n’importe quel texte, ou transmettre un signalement que son auteur avait transmis en le déposant, ne change rien. `forwarded` dans l’API d’administration indique s’il a été envoyé, `forwardable` si vous pouvez l’envoyer maintenant.
- Les détails du signalement (`show`, `GET .../{id}`) renvoient les notes, de la plus ancienne à la plus récente, avec le nombre d’autres signalements concernant la cible et le nombre de mesures prises contre la cible (y compris levées et expirées). La liste ne renvoie pas les notes.
- Les attributions, résolutions, réouvertures, notes et transmissions sont conservées sous forme d’événements (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), et `report_states` contient l’état actuel.
- Les modérateurs peuvent traduire les publications signalées et le commentaire de l’auteur du signalement sur la page du signalement, ou avec `POST .../{id}/statuses/{status_id}/translation` et `POST .../{id}/comment/translation` (corps `{"target_language": "en"}`). Le texte n’est envoyé à un fournisseur de traduction que tant que le [paramètre d’instance](instance-settings.md) `translation_reports_to_server` est activé, et alors uniquement à un fournisseur autorisé pour la visibilité de la publication : l’accès d’un modérateur n’élargit rien, donc une publication directe n’est envoyée qu’à un fournisseur autorisé pour `direct`, et le commentaire compte comme `private`. La traduction du commentaire n’est pas conservée, pas plus que celle d’une publication supprimée, et les caractères ne comptent que dans la limite mensuelle du serveur. La traduction intégrée du navigateur, lorsque `translation_browser` est activé, fonctionne sur les signalements quelle que soit la valeur de `translation_reports_to_server`.
- Un auteur de signalement ne voit de la cible que ce qu’il verrait autrement du compte : tant qu’il est suspendu ou qu’il le bloque, son signalement reste listé sans le compte. Les modérateurs voient toujours toute la cible.
- Un auteur de signalement local reçoit une notification (`report_resolved`) à la première résolution. Rouvrir puis résoudre à nouveau n’ajoute aucune notification. Un auteur de signalement peut lister ses propres signalements avec `GET /api/v1/reports` et ne voit que si chacun est ouvert ou résolu. Le résultat, la mesure prise, le motif donné à la cible et la personne responsable ne sont pas montrés.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Signalements déposés par un examen externe

Un [point de décision](decision-hooks.md) qui répond `flag` dépose un signalement avec l’acteur de votre instance comme auteur. Le signalement l’indique : `hook_origin` dans l’API d’administration et le webhook `report.created`, une ligne dans les détails du signalement, et `automated` dans `my admin report show`. Son commentaire est la note du plugin, et sa catégorie provient du motif. Il n’est pas transmis lorsqu’il est déposé, et le résoudre ne notifie personne. Lorsque la cible est sur un autre serveur, vous pouvez le transmettre vous-même (`forward`) : la note du plugin reste sur votre serveur, et seul le texte que vous écrivez part. Annuler la décision laisse le signalement tel quel.

<!-- source: voice-chat-rooms 7fd142752b07 -->
### Salons de chat vocal

- Un utilisateur local peut signaler un salon de chat vocal ouvert qu’il peut voir (`POST /api/v1/reports` avec `voice_room_id`, et `account_id` réglé sur l’hôte du salon). Le signalement vise l’hôte à ce moment-là et renvoie aux paramètres du salon (nom et description) tels qu’ils étaient alors. Il ne renvoie à aucune publication et n’est jamais transmis à un autre serveur.
- Les détails du signalement (`GET /api/v1/admin/reports/{id}`) montrent le salon (`voice_room`) : chaque modification de ses paramètres, qui y a participé et quand cette personne est partie, qui en a été retiré, et comment il s’est terminé. Les enregistrements des participants sont conservés 90 jours après la fin du salon (`voice_participants_retention_days`) et aussi longtemps qu’un signalement renvoie au salon ([conservation des enregistrements](#record-retention-and-holds)). Il n’y a ni enregistrements audio ni chat texte, ces enregistrements sont donc la seule preuve.
- Un détenteur de `manage_reports` peut mettre fin à un salon ouvert pour tous les participants avec `POST /api/v1/admin/voice_rooms/{id}/end` (corps `{"reason": "..."}`, facultatif, jusqu’à 500 caractères). Le motif est réservé aux modérateurs ; les participants voient seulement que le salon est terminé. Cela fonctionne aussi lorsque le chat vocal est désactivé, et le journal d’audit consigne `voice_room_ended`.

<!-- source: notifications-for-moderators-and-the-pending-counts 7b83fa4097b2 -->
## Notifications pour les modérateurs et compteurs en attente

Les nouveaux signalements, les inscriptions en attente d’approbation et les contestations parviennent aux personnes disposant de la permission sous forme de notifications dans l’application.

| Notification | Quand elle arrive | Qui la reçoit |
| --- | --- | --- |
| `admin_report` | Lorsque la cible du signalement n’a pas d’autre signalement ouvert (un deuxième signalement ou suivant concernant le même compte ne notifie pas) | Les personnes dont le rôle a `manage_reports`, sauf l’auteur du signalement et la cible du signalement |
| `admin_sign_up` | Lorsqu’une inscription commence en attente d’approbation (mode d’approbation, ou `open` sur une configuration qui ne peut pas envoyer d’e-mails) | Les personnes dont le rôle a `manage_accounts` |
| `admin_appeal` | Lorsqu’une contestation est faite contre une mesure | Les personnes dont le rôle a `manage_accounts`, sauf la personne qui a contesté |
| `admin_custom_emoji_request` | Lorsque la création d’un émoji personnalisé ou le remplacement de son image est demandé | Les personnes dont le rôle a `manage_custom_emojis`, sauf le demandeur |
| `admin_error_group` | Lorsque le serveur ou le client web signale une erreur d’un nouveau type, ou qu’un groupe d’erreurs résolu se reproduit sur une autre version du serveur | Les personnes dont le rôle a `manage_errors` |

- `admin_error_group` arrive au plus 10 fois par heure pour l’ensemble du serveur. Le reste de l’heure est regroupé en une seule notification qui indique combien de groupes d’erreurs supplémentaires il y a eu. Un groupe mis en sourdine ne notifie jamais. Une erreur que le client web signale pour un visiteur non connecté ne notifie que lorsque le navigateur d’un utilisateur connecté la signale aussi, et sa récurrence ne notifie personne. Les groupes d’erreurs sont lus et résolus, mis en sourdine ou rétablis via `GET /api/v1/admin/error_groups` et `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`), qui exigent `manage_errors`.
- La personne qui reçoit une notification est déterminée par sa permission au moment où la notification est créée. Une personne qui perd ensuite la permission ne voit plus ce type dans sa liste ni dans son compteur de non lus. Si la permission revient, elle le voit de nouveau.
- Chaque type de notification ci-dessus arrive aussi par e-mail, activé par défaut. Chaque personne active ou désactive chaque type dans l’application et par e-mail sur la page **Notifications** des paramètres (`/settings/notifications` dans le client web), qui ne liste que les types que son rôle reçoit. Un type désactivé dans l’application n’est pas créé pour elle, il n’est donc ni poussé ni envoyé par e-mail ; les compteurs de `GET /api/v1/admin/counts` ne changent pas, car ils comptent les signalements, inscriptions et contestations eux-mêmes. Avec les signalements ouverts, les compteurs renvoient aussi la date de dépôt du plus ancien signalement ouvert (`oldest_open_report_filed_at`). Le serveur met la tâche d’e-mail en file dans la transaction qui enregistre la notification, et la tâche envoie l’e-mail juste après la validation, sans attendre que la notification soit lue. Si la personne a perdu la permission ou désactivé l’e-mail de ce type au moment de l’envoi, ou si son adresse principale n’est pas confirmée ou que ses e-mails sont arrêtés, rien n’est envoyé. L’e-mail ne contient que le type de notification et un lien vers l’écran des notifications, et rien du contenu du signalement ou du compte. Son lien de désabonnement ne désactive que l’e-mail de ce type. Avec `mail = none`, aucun e-mail n’est envoyé, et `sends_email` dans `GET /api/v1/instance` vaut `false`.

`GET /api/v1/admin/counts` renvoie le nombre de signalements ouverts (`open_reports`, exige `manage_reports`), le nombre d’inscriptions en attente d’approbation (`pending_registrations`) et le nombre de contestations en attente d’examen (`pending_appeals`) (les deux exigent `manage_accounts`). Les éléments pour lesquels vous n’avez pas la permission ne sont pas renvoyés.

<!-- source: audit-log 86955f4182c0 -->
## Journal d’audit

Les opérations des modérateurs et des administrateurs sont consignées à raison d’une ligne chacune dans le journal d’audit (`audit_log_entries`) dans la même transaction que l’opération. Les opérations échouées et les opérations qui n’ont rien changé ne sont pas consignées : lever une mesure déjà levée, résoudre un signalement résolu, attribuer au même responsable, retirer une règle déjà retirée, déplacer à la même position, attribuer un rôle que l’utilisateur a déjà, et retirer un rôle que l’utilisateur n’a pas.

| Type de cible | Opérations |
| --- | --- |
| `account` | `account_action_issued` et `account_action_lifted` (prise et levée d’une mesure contre un compte), `account_deletion_canceled` (la suppression annulée parce qu’une contestation d’un `ban` a été reconnue fondée), `revealed_private_data` (adresses IP et adresses e-mail affichées), `two_factor_disabled` (authentification à deux facteurs supprimée), `password_reset_link_issued` (un lien de réinitialisation du mot de passe émis), `email_suppression_lifted` (arrêt des e-mails levé), `account_moved` (un compte local restreint déplacé vers un compte local ; les restrictions ne sont pas reportées sur la destination, c’est donc à vous de décider si vous les y appliquez), `account_statuses_refetch_queued` (nouvelle récupération des publications d’un compte distant) |
| `status` | `status_refetch_queued` (nouvelle récupération d’une publication distante ; le nom de la cible est l’URI de la publication) |
| `domain` | `domain_action_issued` et `domain_action_lifted` (prise et levée d’une mesure sur un domaine), `domain_statuses_refetch_queued` (nouvelle récupération des publications des comptes du domaine), `instance_marked_available` (effacement des échecs d’un serveur) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (attribution, retrait d’attribution, résolution et réouverture d’un signalement, ajout d’une note, et transmission) |
| `user` | `registration_approved`, `registration_rejected` (examen de l’inscription) |
| `setting` | `setting_changed` (paramètres d’instance), `text_changed` (textes d’instance) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (règles de l’instance) |
| `role` | `role_granted`, `role_revoked` (attribution et retrait d’un rôle à un utilisateur), `role_created`, `role_updated`, `role_deleted` (création, modification et suppression d’un rôle) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (examen d’une contestation ; l’annulation consigne aussi `account_action_lifted` pour la mesure, et `account_deletion_canceled` pour un `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (ajout et levée d’un blocage d’inscription) |
| `hook_decision` | `hook_decision_applied` (le serveur a agi sur le verdict d’un examen externe et a laissé l’état ; l’acteur est le système), `hook_decision_overridden` (un modérateur a annulé la décision, y compris en acceptant sa contestation), `hook_appeal_upheld` (un modérateur a rejeté la contestation d’une publication refusée, et la décision reste) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (enregistrement d’un émoji personnalisé, remplacement de son image, copie d’un émoji d’un autre serveur, modification, activation, désactivation et suppression) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (création, modification, suppression, activation et désactivation d’un webhook, et régénération de son secret) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn` (création, modification, publication et retrait d’une annonce) |
| `invite` | `invite_created`, `invite_revoked` (création et révocation d’une invitation ; le nom de la cible correspond aux 4 premiers caractères du code) |
| `billing_source` | `billing_import_queued` (un import de facturation a été lancé à la main ; le nom de la cible est le nom du fournisseur) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (enregistrement d’un coût, modification de son nom et semblables, révision de son montant, modification de sa date de fin, et suppression ; le nom de la cible est le nom de l’élément) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (activation et désactivation de la facturation ; le nom de la cible est le nom du fournisseur de paiement) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (création, modification et retrait d’une formule de soutien ; le nom de la cible est le nom de la formule) |
| `supporter_subscription` | `supporter_subscription_ended` (un administrateur a mis fin immédiatement à une souscription ; le nom de la cible est le nom de la formule et l’identifiant de l’utilisateur) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (dépôt, retrait, approbation et rejet d’une demande d’émoji personnalisé ; le nom de la cible est le `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (le rapprochement avec le fournisseur de paiement a été lancé à la main ; le nom de la cible est le nom du fournisseur) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (désactivation et rétablissement d’un client OAuth ; la désactivation révoque tous ses jetons ; le nom de la cible est le nom du client et `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (refus et annulation du refus d’un hôte de Client ID Metadata Document ; le refus révoque les jetons des clients de cet hôte ; le nom de la cible est l’hôte) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (annonce de la fermeture du service ou changement de la date, retrait de l’annonce, fermeture, et fin de la livraison de la fermeture ; le nom de la cible est `service_closure`) |
| `voice_room` | `voice_room_ended` (un modérateur a mis fin à un salon de chat vocal ouvert ; le nom de la cible est l’ID du salon, car le nom disparaît lorsque le compte qui l’a écrit est effacé ; les détails du signalement montrent le nom) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (définition d’une règle du robot des cartes de lien sur un domaine, et son retrait ; voir [domaines des cartes de lien](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (ajout, modification, suppression, activation et désactivation d’un fournisseur de connexion externe, et remplacement de son secret client ; le nom de la cible est le nom d’affichage du fournisseur) |

La liste indique si la cible existe encore. Les comptes, signalements, contestations et blocages d’inscription qui ont disparu après leur durée de conservation, ainsi que les rôles, émojis personnalisés et webhooks supprimés, n’affichent que le nom au moment de l’enregistrement et aucun lien vers un écran.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

Dans l’API d’administration : `GET /api/v1/admin/audit_log` (filtrez avec `actor_account_id`, `action`, `target_kind` et `target_id`). Elle exige la permission `view_audit`.

- Une ligne comprend l’acteur (l’utilisateur pour `user`, ou `cli` pour la CLI), l’opération, le type et l’ID de la cible, le nom d’affichage de la cible, l’ID de l’enregistrement d’origine et l’heure. L’ID de la cible est l’ID du compte, du signalement, de la contestation, de l’utilisateur, de la règle, du rôle ou du blocage d’inscription ; pour une mesure sur un domaine, c’est le domaine ; pour les paramètres et les textes, c’est la clé.
- Le nom d’affichage de la cible (`@alice`, `@bob@example.com`, un domaine, une clé de paramètre, le texte d’une règle ; pour un texte sa clé, pour un rôle sa clé et l’identifiant de l’utilisateur, pour un blocage d’inscription son type et sa valeur) est une copie prise au moment de l’enregistrement, il reste donc lisible une fois la cible disparue. Chaque ligne de l’API renvoie aussi si la cible existe encore (`target_exists`), qui vaut `false` pour les comptes, signalements, contestations et blocages d’inscription disparus. Le journal d’audit du web n’affiche que le nom d’affichage, sans lien, pour une cible disparue.
- Le texte des motifs et des notes n’est pas copié. Retrouvez-le via l’ID de l’enregistrement d’origine (la mesure, l’attribution, la résolution, la réouverture, la note, l’examen, ou la version d’un paramètre ou d’une règle).
- La liste va de la plus récente à la plus ancienne avec une pagination par clé (`-before <entry id>` dans la CLI).
- Il n’existe ni API ni CLI pour modifier ou supprimer le journal d’audit. Les anciennes lignes sont supprimées après la [durée de conservation](#record-retention-and-holds).

<!-- source: record-retention-and-holds bd806b3bbda7 -->
## Conservation des enregistrements et conservations obligatoires

Après la durée de conservation de chaque type d’enregistrement, des tâches horaires (`moderation.purge_records`, `identity.purge_origins` et `voice.purge_records`) suppriment les enregistrements de modération. Vous pouvez modifier les durées dans les paramètres d’instance, mais pas en dessous de la limite inférieure ([paramètres d’instance](instance-settings.md)). Une durée modifiée prend effet à l’exécution suivante. Une conservation obligatoire levée, avec son motif, est supprimée une fois écoulée, depuis sa levée, la durée de conservation des mesures de modération.

| Enregistrement | Paramètre | Par défaut | Limite inférieure | Point de départ |
| --- | --- | --- | --- | --- |
| Adresse IP et User-Agent de connexion | `session_origins_retention_days` | 180 jours | 90 jours | La connexion |
| Adresse IP d’inscription | `registration_origins_retention_days` | 180 jours | 90 jours | L’inscription |
| Signalements (commentaire, publications visées, notes, attributions, résolutions, réouvertures) | `reports_retention_days` | 365 jours | 180 jours | La dernière résolution |
| Mesures contre des comptes (motif, note, publications visées, levée) et contestations | `moderation_actions_retention_days` | 365 jours | 180 jours | La fin de la mesure |
| Journal d’audit | `audit_log_retention_days` | 730 jours | 365 jours | L’enregistrement |
| Blocages d’inscription | Aucun (ne peut pas être modifié) | 30 jours | n/a | Le premier de la levée et de l’expiration |
| Qui a participé à un salon de chat vocal (participations, intervenants et auditeurs, mains levées, invitations, retraits) | `voice_participants_retention_days` | 90 jours | 30 jours | La fin du salon |
| Les décisions des [points de décision](decision-hooks.md#the-decisions) | `hook_decisions_retention_days` | 90 jours | 30 jours | La décision |

- Les signalements ouverts ne sont jamais supprimés. Un signalement auquel une mesure renvoie (`report_id` de la mesure) reste jusqu’à la suppression de cette mesure.
- La fin d’une mesure est le premier de la levée et de l’expiration. Pour une mesure qui a été contestée, si l’examen arrive plus tard, la fin est l’heure de l’examen, et la mesure est conservée tant que la contestation attend un examen. `warn` et `delete_statuses` n’ont pas d’effet durable, donc la fin est l’heure où elles ont été prises. Une mesure sans fin (un `ban`, un `suspend` sans heure de fin, etc.) n’est pas supprimée avant d’être levée.
- Les publications vers lesquelles un signalement ou une mesure renvoie ne sont pas physiquement supprimées par la tâche de purge, avec leurs médias joints, avant la suppression de ce signalement ou de cette mesure, même si l’auteur les a supprimées (elles disparaissent de la vue). Les publications distantes restent aussi au-delà de la durée de conservation (y compris le paramètre d’instance `remote_status_max_retention_days`).
- Un blocage d’inscription qui n’a ni levée ni heure de fin reste tant qu’il est en vigueur.
- Les enregistrements de qui a participé à un salon de chat vocal visé par un signalement restent jusqu’à la suppression de ce signalement. Le salon lui-même (son nom, sa description, ses hôtes et la manière dont il s’est terminé) n’est jamais supprimé.
- Une exécution de tâche ne supprime qu’un nombre limité de lignes par type (100 à la fois, 20 fois, pour les mesures, signalements et blocages d’inscription ; 1 000 lignes à la fois, 20 fois, pour le journal d’audit). L’exécution suivante supprime le reste.

<!-- source: holds f3bf57c2c258 -->
### Conservations obligatoires

Tant qu’une ordonnance judiciaire interdit l’effacement (loi japonaise sur les plateformes de diffusion d’informations, article 16) ou qu’une demande de divulgation est en cours de traitement, vous pouvez arrêter l’effacement par compte.

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- Pour un compte soumis à une conservation obligatoire, ses mesures et contestations, les signalements le concernant et ceux qu’il a faits, les journaux d’audit correspondants, les adresses IP de connexion et d’inscription, ses publications, et les enregistrements des salons de chat vocal auxquels il a participé ou dans lesquels il a agi ne sont pas supprimés après la durée de conservation. Un blocage d’inscription créé à partir du bannissement de ce compte est aussi conservé au-delà de 30 jours après la levée ou l’expiration.
- Vous pouvez placer plusieurs conservations obligatoires sur un même compte. Lorsque vous les levez toutes, les enregistrements dont la durée est dépassée sont supprimés à l’exécution suivante. Lever une conservation déjà levée ne change rien.
- Les conservations obligatoires et leur levée ne se font qu’avec la CLI, et l’acteur dans l’enregistrement est la CLI.
- Les données d’un compte supprimé (un compte dont le `ban` a atteint sa date, un compte que l’utilisateur a supprimé, ou un compte distant que son serveur a supprimé) sont effacées 7 jours après la suppression par la tâche horaire `account.erase_deleted`. Les publications d’un compte distant partent aussi, y compris celles que des utilisateurs locaux ont mises en marque-page ou auxquelles ils ont répondu. Un compte distant qui a été marqué comme supprimé uniquement parce qu’une récupération a répondu `410 Gone`, sans son propre `Delete` signé, est d’abord récupéré une fois de plus. Un second `410` l’efface. Si son serveur le sert de nouveau, il revient avec ses publications, et rien n’est effacé. Toute autre réponse (`404`, ou aucune réponse) attend une exécution ultérieure, et 30 jours après la suppression le compte est effacé de toute façon. Un compte soumis à une conservation obligatoire n’est pas effacé. Les publications vers lesquelles un signalement ou une mesure renvoie restent jusqu’à la suppression de cet enregistrement. L’URI et le nom d’utilisateur du compte ainsi que les enregistrements de mesures subsistent. Les noms et descriptions que le compte a donnés à ses salons de chat vocal sont vidés ; les salons restent.

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### Exporter pour les demandes de divulgation

Pour une demande de divulgation (RGPD, article 15 ; loi japonaise sur la protection des informations personnelles, article 33), répondez en exportant en JSON tous les enregistrements de modération d’un compte.

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- L’export contient les mesures contre le compte (y compris les notes privées, la levée, les contestations et les examens), les signalements concernant le compte (commentaire, publications visées, règles citées, notes des modérateurs, attributions, résolutions, réouvertures), les signalements faits par le compte (seulement si chacun est ouvert ou résolu), et les journaux d’audit correspondants.
- L’identité des auteurs de signalements (le compte qui signale, et l’URI et le serveur d’origine d’un signalement reçu) n’est pas exportée. Avant de remettre l’export, vérifiez que le commentaire d’un signalement ne dit rien qui identifie l’auteur du signalement.
- Un modérateur est exporté par ID d’utilisateur. Placez d’abord une conservation obligatoire afin que les enregistrements ne soient pas supprimés pendant que vous traitez la demande.
