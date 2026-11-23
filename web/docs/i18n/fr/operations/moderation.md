<!-- source: account-and-domain-actions-and-handling-reports 7f21d5c6c925 -->
# Mesures sur les comptes et les domaines, et traitement des signalements

Comment les exploitants et les modérateurs agissent sur les comptes et les domaines, tranchent les contestations, traitent les signalements, vérifient ce qui a atteint un serveur distant et lisent le journal d’audit.
Tout ce qui figure ici, sauf la gestion des comptes et la vérification des envois (API d’administration uniquement), peut se faire par l’API d’administration (`/api/v1/admin/...`, périmètres `admin:read` / `admin:write`) et la CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`).
L’API d’administration exige des permissions de rôle : la gestion des comptes, les mesures et les contestations exigent `manage_accounts`, les mesures sur les domaines exigent `manage_domains`, et les signalements exigent `manage_reports`.
La CLI se connecte directement à la base de données avec la même configuration que le serveur (`-config` ou variables d’environnement) et n’enregistre pas quel utilisateur l’a utilisée (`issued_by_user_id` et les champs similaires valent NULL, et l’acteur dans le [journal d’audit](#audit-log) est `cli`).

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## D’un signalement à une mesure et à une contestation

![Le parcours d’un signalement : il arrive, un modérateur l’examine, une mesure peut suivre avec un avis à l’utilisateur, et l’utilisateur peut la contester ; la colonne de droite montre ce que chaque étape envoie aux autres serveurs. La liste ci-dessous décrit chaque étape.](images/moderation-lifecycle.svg)

Du haut du schéma vers le bas :

1. **Un signalement arrive.** Un utilisateur local en dépose un, un autre serveur envoie un `Flag` (abandonné lorsque son domaine a `reject_reports`, voir [mesures sur les domaines](#domain-actions)), ou un [point de décision](decision-hooks.md) répond `flag` ([signalements déposés par un examen externe](#reports-that-an-external-review-filed)). Un utilisateur local qui signale un compte d’un autre serveur peut choisir de transmettre le signalement, ce qui envoie aussitôt un `Flag` à ce serveur.
2. **Examen.** Le signalement est `open`, et les modérateurs reçoivent une notification `admin_report`. Le résoudre le fait passer à `resolved` avec le résultat `actioned` ou `dismissed`, et un auteur de signalement local reçoit une notification `report_resolved`. Le rouvrir le remet à `open`. Transmettre un signalement concernant un compte d’un autre serveur envoie un `Flag` à ce serveur ([signalements](#reports)).
3. **Mesure.** Un modérateur prend une [mesure sur un compte](#account-actions) depuis le signalement, ce qui résout le signalement comme `actioned` ; une mesure peut aussi être prise sans signalement. La mesure a un type, un motif et une heure de fin, et prend fin lorsqu’elle est levée ou qu’elle expire. Avec `notify_target`, l’utilisateur reçoit une notification `moderation_notice` ([ce que voit l’utilisateur](#what-the-user-sees)). Les autres serveurs reçoivent un `Delete` des publications pour `delete_statuses`, un `Update` de l’acteur lorsque le `suspend` ou le `ban` d’un compte local commence ou se termine, et un `Reject` / `Undo` des abonnements que coupe le `suspend` d’un compte distant.
4. **Contestation.** L’utilisateur local conteste la mesure. La contestation est `pending`, et les modérateurs reçoivent une notification `admin_appeal`. La décision est `upheld` ou `overturned`, et l’utilisateur reçoit une notification `appeal_decided`. Annuler la mesure la lève et annule la suppression d’un `ban`, donc un `suspend` ou un `ban` levé par une contestation acceptée sur un compte local envoie un `Update` de l’acteur ([contestations](#appeals)).

Les attributions, les notes, les avis et les contestations restent sur ce serveur ; seules les activités de la colonne de droite le quittent.

<!-- source: moderation-and-server-administration-in-the-web-client 035f2c60c4a9 -->
## Modération et administration du serveur dans le client web

Le client web sépare les écrans du personnel en deux.
La navigation n’affiche « Modération » et « Administration » qu’aux personnes dont le rôle a au moins une permission de ce groupe. Chaque écran n’affiche qu’une raison aux personnes sans la permission, et le serveur rejette aussi leurs requêtes.

| Groupe | Permissions | Écrans |
| --- | --- | --- |
| Modération (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Signalements (`/moderation/reports`), Contestations (`/moderation/appeals`), Inscriptions (`/moderation/registrations`), Examens automatisés (`/moderation/hook-decisions`), Comptes (`/moderation/accounts`), Blocages d’inscription (`/moderation/signup-blocks`), Invitations (`/moderation/invites`), Actions de domaine (`/moderation/domain-actions`), Serveurs distants (`/moderation/instances`), Journal d’audit (`/moderation/audit-log`) |
| Administration du serveur (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Paramètres du serveur (`/admin/settings`), Règles (`/admin/rules`), Conditions et politiques (`/admin/documents`), Domaines de cartes de lien (`/admin/link-card-domains`), la file de jobs et les jobs en échec (`/admin/jobs`), Erreurs (`/admin/errors`, `manage_errors`), Mesures d’expérience utilisateur (`/admin/ux-measurements`, `manage_errors`), Émojis personnalisés (`/admin/custom-emojis`), Annonces (`/admin/announcements`), Webhooks (`/admin/webhooks`), Coûts et revenus (`/admin/finance`, `manage_finance`), Soutiens (`/admin/supporters`, `manage_supporters`). `manage_login_recovery` (récupération de connexion : retirer l’authentification à deux facteurs et émettre un lien de réinitialisation du mot de passe) et `manage_roles` (attribuer et révoquer les rôles d’un compte) apparaissent comme des sections de la page Gestion d’un compte (`/moderation/accounts/{id}/manage`, qui exige aussi `manage_accounts`) |

- Le badge sur « Modération » est le total des signalements ouverts, des inscriptions en attente d’approbation et des contestations en attente d’examen ([notifications pour les modérateurs et compteurs en attente](#notifications-for-moderators-and-the-pending-counts)). « Administration » n’affiche aucun nombre.
- Chaque groupe s’ouvre sur une liste de ses écrans que les permissions de votre rôle ouvrent, chacun avec une ligne sur ce qu’il contient et, pour une file, combien d’éléments y attendent (Signalements compte les signalements ouverts, Contestations celles en attente d’examen, Inscriptions celles en attente d’approbation, Émojis personnalisés les demandes en attente d’examen, Erreurs les erreurs ouvertes) ; un groupe dont aucun écran ne vous est ouvert en indique la raison.
  L’adresse d’un écran que votre rôle n’ouvre pas l’indique, avec un lien vers la liste du groupe.
- Le `moderator` intégré n’a que `manage_reports`, `manage_accounts` et `view_audit`, et ne voit pas « Administration ». Il ne voit pas non plus les écrans Actions de domaine et Serveurs distants (`manage_domains`) ;
  pour les déléguer, créez un rôle avec cette permission et attribuez-le ([attribuer et révoquer des rôles](admin-cli.md#granting-and-revoking-roles)).
- Les chemins de l’API d’administration restent `/api/v1/admin/...` quel que soit le groupe, et les portées sont la même paire `admin:read` / `admin:write`. Les groupes sont séparés par des balises dans la référence de l’API (`admin-moderation`, `admin-server`).
  Un service extérieur au serveur peut appeler les opérations d’une permission avec une portée de permission à la place ([laisser un autre service utiliser l’API d’administration](#letting-another-service-use-the-admin-api)).
- Il existe un troisième groupe de permissions, les « fonctions utilisateur » (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`, `create_articles`, `create_events`), qui vit en dehors des écrans du personnel. Une personne qui n’a que ces permissions ne voit ni « Modération » ni « Administration »,
  et pour une personne avec `view_finance`, la navigation affiche « Finances » (`/finance`) après Paramètres.
  Son API est dans l’espace de noms utilisateur (`GET /api/v1/finance/cost_totals` et `GET /api/v1/finance/income_totals`, portée `read:finance`) et n’exige aucune portée `admin:*`. Ce qu’elle affiche est défini par `finance_viewer_*` dans les [paramètres de l’instance](instance-settings.md).

<!-- source: letting-another-service-use-the-admin-api d767e08e639c -->
### Laisser un autre service utiliser l’API d’administration

Un service que vous exécutez à côté du serveur, un qui distribue des invitations ou enregistre des coûts, par exemple, peut appeler l’API d’administration pour une permission sans plugin.

1. Inscrivez un compte pour le service et accordez-lui un rôle avec uniquement les permissions dont le service a besoin (pour les invitations, un rôle avec `manage_invites` ; [attribuer et révoquer des rôles](admin-cli.md#granting-and-revoking-roles)).
2. Connecté en tant que ce compte, enregistrez une application dans **Paramètres → Applications → Vos applications** dont les portées incluent la portée de permission, `admin:` et la permission : `admin:manage_invites`.
3. Émettez un jeton pour le compte à partir de l’application, avec cette portée, et donnez au service le jeton d’accès et le jeton d’actualisation.

- Le jeton n’atteint que les opérations des permissions que ses portées nomment, et seulement tant que les rôles du compte les conservent ; le journal d’audit enregistre ce qu’il fait comme des actions du compte.
- Le jeton d’accès dure une heure. Le service l’actualise avec le jeton d’actualisation et conserve à chaque fois le nouveau jeton d’actualisation : un ancien jeton réutilisé révoque le jeton.
- `manage_roles`, `manage_oauth_apps`, `manage_login_recovery`, `manage_login_providers` et `manage_service_closure` n’ont aucune portée de permission, et aucune application ne peut en demander une à un membre du personnel sur l’écran de consentement.
- Pour arrêter le service, révoquez les jetons de l’application ou supprimez l’application, retirez le rôle, ou désactivez l’application dans Administration → Applications OAuth.

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

<!-- source: appeals 3366a552f16e -->
## Contestations

Un utilisateur local peut contester une mesure prise contre lui.
Un modérateur examine la contestation et soit la maintient (`uphold`), soit l’annule (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

L’utilisateur conteste avec `POST /api/v1/moderation/actions/{id}/appeal` (portée `write:accounts`, corps `{"text": "..."}`).
Un utilisateur sous `disable_login`, `suspend` ou `ban` ne peut pas obtenir de jeton d’accès et conteste par le formulaire propre à chaque mesure sur la page de connexion (`POST /auth/appeal`).
Tant que [la connexion est limitée aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers), un tel utilisateur n’atteint le formulaire que par un fournisseur lié.
La page affiche le formulaire et la date limite pour une mesure qui peut être contestée, le contenu et le résultat pour une mesure déjà contestée, et la raison pour une mesure dont la date limite est passée ou qui ne peut pas être contestée. Pour une contestation rejetée, elle affiche la raison.
Dans l’API d’administration : `GET /api/v1/admin/appeals` (filtre avec `state`), `GET .../{id}`, et `POST .../{id}/uphold` et `POST .../{id}/overturn` (le corps `{"public_reason": "..."}` est facultatif). Elles exigent la permission `manage_accounts`.
Dans le client web, Modération → Contestations (`/moderation/appeals`) liste les contestations en attente d’examen, ou toutes ; la page d’une contestation (`/moderation/appeals/{id}`) affiche ce que l’utilisateur a écrit et la mesure, avec la raison à communiquer à l’utilisateur et « Maintenir » et « Annuler », chacun demandant d’abord confirmation.

- Une mesure ne peut être contestée qu’une fois, et le texte compte de 1 à 2 000 caractères. La date limite est le nombre de jours du paramètre d’instance `appeal_window_days` après la mesure (180 jours par défaut, de 20 à 365 ; voir [paramètres de l’instance](instance-settings.md)).
  Une mesure passée la date limite et une seconde contestation donnent `422` (`/api/v1/problems/appeal-window-closed` et `/api/v1/problems/already-appealed`).
- Un `ban` peut être contesté jusqu’à ce que le compte soit supprimé (30 jours après la mesure, ou jusqu’à la date limite `appeal_window_days` si elle est plus courte). Un `ban` d’un compte supprimé donne `422` (`/api/v1/problems/not-appealable`). Une mesure levée ou expirée peut encore être contestée dans le délai.
- Lorsqu’un utilisateur conteste, les personnes avec `manage_accounts` reçoivent une notification (`admin_appeal`). Lorsqu’elle est examinée, l’utilisateur reçoit une notification (`appeal_decided`) et peut voir le résultat et `public_reason`. L’examinateur n’est pas montré.
- Annuler une mesure la lève dans la même transaction (le même effet que `lift`, avec un motif de levée vide). Annuler `delete_statuses` ne ramène pas les publications supprimées. Une mesure déjà levée avant l’examen reste telle quelle.
- Annuler un `ban` annule aussi la suppression programmée du compte dans la même transaction (le journal d’audit enregistre `account_deletion_canceled`), met fin à la suspension et envoie le `Update` du profil d’origine aux autres serveurs. Les sessions et jetons révoqués ne reviennent pas ; l’utilisateur se reconnecte donc.
  Si le compte a été supprimé avant l’examen, il ne peut pas être annulé (`422`, `/api/v1/problems/not-appealable` ; le maintien reste possible).
- Un examen n’a lieu qu’une fois. Réexaminer avec le même résultat ne change rien, et le résultat opposé donne `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management aa223fe467e1 -->
## Gestion des comptes

La recherche de comptes, l’affichage de leurs détails et la révélation des adresses IP et des adresses e-mail se font par l’API d’administration et exigent la permission `manage_accounts`.

Dans le client web, un compte a trois pages, avec des liens entre elles sous le titre de la page :

- **Examen** (`/moderation/accounts/{id}`) : l’état du compte et ses restrictions, son inscription et qui l’a invité, le formulaire d’une nouvelle mesure (ouvert depuis un signalement avec « Agir sur ce compte » ; le formulaire propose aussi les mesures sur les publications du signalement, et la mesure résout le signalement), les mesures les plus récentes prises contre lui avec leurs motifs, avec « Toutes les mesures » vers sa page Mesures lorsqu’il peut y en avoir davantage, et ses signalements, les plus récents d’abord, le reste sous « Afficher plus ».
- **Mesures** (`/moderation/accounts/{id}/actions`) : chaque mesure prise contre le compte, ou celles en vigueur, une page à la fois. Chaque mesure ouvre sa propre page (`/moderation/accounts/{id}/actions/{action id}`) avec tous ses détails et, tant qu’elle est en vigueur, « Lever » avec un motif pour les modérateurs. La page d’un bannissement n’a pas de « Lever » : seule l’annulation de sa contestation l’annule.
- **Gestion** (`/moderation/accounts/{id}/manage`) : pour un compte local, ses rôles (`manage_roles`), sa suppression d’e-mail, le retrait de l’authentification à deux facteurs et l’émission d’un lien de réinitialisation du mot de passe (`manage_login_recovery`), ainsi que ses adresses e-mail et IP, montrées seulement après « Afficher », avec ses connexions et les comptes qui ont utilisé les mêmes adresses IP, chaque liste des plus récentes aux plus anciennes avec le reste sous « Afficher plus » (chaque appui lit une page, enregistrée dans le journal d’audit), et un lien vers ceux du même domaine d’e-mail ; pour un compte distant, la récupération de ses publications depuis son serveur (`manage_domains`). Chaque section n’apparaît qu’aux personnes qui ont sa permission.

Le personnel n’agit que sur les rangs inférieurs : émettre ou lever une mesure, décider d’une contestation, lever une suppression d’e-mail, révéler des données privées, émettre un lien de réinitialisation du mot de passe et retirer l’authentification à deux facteurs exigent que la `position` la plus élevée de vos rôles soit supérieure à celle de chaque rôle de la cible.
Un rang supérieur ou égal, y compris votre propre compte, renvoie `403` avec le type de problème `/api/v1/problems/target-outranks-actor` ; un modérateur ne peut donc pas bannir un administrateur et un administrateur ne peut pas en bannir un autre. Les commandes `my admin` n’ont pas une telle limite.
Une mesure qui empêche le seul administrateur pouvant se connecter de se connecter (`disable_login`, `suspend`, `ban`) est refusée quel que soit son auteur, et cet administrateur ne peut pas supprimer son propre compte (`409` avec `/api/v1/problems/last-administrator`). Voir [admin-cli.md](admin-cli.md) pour les rôles.

- `GET /api/v1/admin/accounts` renvoie les comptes locaux et distants, les plus récents d’abord (l’Actor de l’instance est exclu).
  Vous pouvez filtrer par préfixe de nom d’utilisateur (`username`), domaine (`domain`), local ou distant (`origin`), état (`state`), et, pour les comptes locaux, adresse e-mail (`email`), domaine d’e-mail (`email_domain`) et adresse IP utilisée pour s’inscrire ou se connecter (`ip`).
  Vous ne pouvez donner qu’un seul de `email`, `email_domain` et `ip`. La liste n’inclut ni adresses IP ni adresses e-mail.
- L’état est le premier qui s’applique dans cette liste : `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` et `pending` (examen de l’inscription), `limited` (`limit`), `active`.
  L’état et les restrictions (`restrictions`) proviennent uniquement des mesures prises contre le compte lui-même et n’incluent pas les actions de domaine.
- `GET /api/v1/admin/accounts/{id}` renvoie l’état et les restrictions et, pour un compte local, la demande d’inscription et son examen, la confirmation de l’e-mail, les rôles, et jusqu’à 20 mesures et signalements, les plus récents d’abord. `GET /api/v1/admin/reports?target_account_id={id}` parcourt page par page tous les signalements concernant un compte.
- `POST /api/v1/admin/accounts/{id}/reveal` renvoie l’adresse e-mail d’un compte local, les adresses IP et les User-Agents de l’inscription et des connexions (20 entrées, les plus récentes d’abord), et les autres comptes qui ont utilisé la même adresse IP (jusqu’à 20 par adresse IP).
  `GET /api/v1/admin/accounts/{id}/sign_ins` et `GET /api/v1/admin/accounts/{id}/same_ip_accounts?ip=` les parcourent tous page par page.
  Chaque appel de ces trois enregistre `revealed_private_data` dans le journal d’audit, et les données ne sont renvoyées que si l’enregistrement a pu être écrit. Les adresses IP sont supprimées après la période de conservation (180 jours par défaut) ; les origines de connexion plus anciennes ne peuvent donc pas être montrées.
- `two_factor_enabled` dans `GET /api/v1/admin/accounts/{id}` indique si l’utilisateur d’un compte local utilise l’authentification à deux facteurs (TOTP).
  Un utilisateur qui a perdu à la fois l’application d’authentification et les codes de récupération peut se connecter avec le seul mot de passe une fois que vous retirez l’authentification à deux facteurs avec `POST /api/v1/admin/accounts/{id}/two_factor/disable`.
  Seuls les utilisateurs ayant la permission `manage_login_recovery` peuvent le faire, et par défaut seul le rôle d’administrateur intégré l’a (elle relève de l’administration du serveur ; le client web n’affiche la section qu’à eux).
  Le retrait enregistre `two_factor_disabled` dans le journal d’audit. Vérifiez que la personne est bien la propriétaire du compte par une autre voie, comme l’e-mail, avant de le retirer.
  Lorsqu’un administrateur sans autre administrateur l’a perdue, l’opérateur la retire avec la commande CLI `my admin account disable-two-factor @alice` (l’acteur du journal d’audit est la CLI).
- Sur un serveur qui envoie des e-mails, un utilisateur qui a oublié son mot de passe reçoit par e-mail un lien pour en définir un nouveau depuis « Mot de passe oublié ? » sur la page de connexion.
  Pour un serveur qui n’envoie pas d’e-mails (`mail = none`), ou un utilisateur que les e-mails n’atteignent pas, un administrateur émet le lien et le transmet.
  Dans les écrans d’administration, émettez-le avec « Réinitialiser le mot de passe » sur la page Gestion du compte (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`).
  Seuls les utilisateurs ayant la même permission que pour retirer l’authentification à deux facteurs, `manage_login_recovery`, peuvent l’émettre, et par défaut seul le rôle d’administrateur intégré l’a (quiconque détient le lien peut entrer dans le compte ; le `manage_accounts` d’un modérateur ne suffit donc pas).
  Le lien émis est montré une seule fois sur-le-champ et peut être copié. Le serveur ne stocke qu’un hachage du lien ; vous ne pouvez donc pas le voir ensuite ; émettez-le de nouveau si besoin.
  Lorsqu’aucun administrateur n’est disponible ou que les écrans d’administration ne le sont pas, l’opérateur l’émet avec la commande CLI `my admin account password-reset-link @alice` (l’acteur du journal d’audit est la CLI).
  Tant que [la connexion est limitée aux fournisseurs de connexion](login-providers.md#limit-sign-in-to-login-providers), les deux émettent quand même le lien et indiquent qu’il ne fonctionne pas avant la fin de la limite.
  Le lien fonctionne pendant 1 heure et une seule fois, et cesse de fonctionner si le mot de passe change ensuite par une autre voie. Les deux façons de l’émettre enregistrent `password_reset_link_issued` dans le journal d’audit.
  Quiconque détient le lien peut définir le mot de passe ; vérifiez donc la personne par une autre voie comme l’e-mail et ne donnez le lien qu’à elle.
  Définir le nouveau mot de passe révoque toutes les sessions et tous les jetons d’application de l’utilisateur. Les clés d’accès et l’authentification à deux facteurs restent.
- `email_suppression` dans `GET /api/v1/admin/accounts/{id}` indique que l’envoi vers l’adresse principale d’un compte local est arrêté.
  Un `hard_bounce` (la destination n’existe pas) arrête tous les e-mails sauf l’e-mail de confirmation, et un `complaint` (signalé comme spam) n’arrête que les e-mails de notification et d’annonce. Les e-mails de réinitialisation du mot de passe s’arrêtent aussi ; émettez donc le lien ci-dessus et transmettez-le si besoin.
  Lorsque l’utilisateur demande un e-mail de confirmation et confirme de nouveau l’adresse, la suppression due à un rebond définitif est levée.
  Lorsque vous avez vérifié que l’envoi est correct, par exemple parce que l’utilisateur a réparé sa boîte aux lettres, levez-la avec `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permission `manage_accounts`). Elle enregistre `email_suppression_lifted` dans le journal d’audit.
- Une recherche lit jusqu’à 400 comptes pour remplir une page. Lorsque peu de comptes correspondent, même une page courte ou vide peut porter `next_cursor` ; continuez donc la lecture.

<!-- source: domain-actions d71386f1cd2d -->
## Actions de domaine

| Type | Effet |
| --- | --- |
| `suspend` | Arrête toute livraison, récupération et réception (la réception reçoit `403` avant la vérification de la signature). Les comptes et publications existants sont masqués des fils et des profils. Coupe les abonnements entre les utilisateurs locaux et les comptes du domaine dans les deux sens (rien n’est envoyé à l’autre côté) |
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

Dans l’API d’administration : `POST /api/v1/admin/domain_actions` (le corps contient `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` et `expires_in` en secondes),
`GET /api/v1/admin/domain_actions` (filtre avec `domain` et `active`), `GET .../{id}`, `POST .../{id}/lift`,
et `GET .../impact?domain=`, qui compte les abonnements qu’un `suspend` du domaine mettrait fin.

Dans le client web, ouvrez Modération → Actions de domaine (`/moderation/domain-actions`). La liste affiche le domaine, la raison publique, le type, la fin et l’état de chaque action, en vigueur ou toutes, de chaque domaine ou de celui que vous recherchez ;
« Nouvelle action » ouvre le formulaire (`/moderation/domain-actions/new`), et choisir un domaine ouvre la page de l’action avec ses motifs et ses dates, où « Lever » y met fin.
Un `suspend` demande d’abord confirmation : la confirmation indique combien d’abonnements entre les utilisateurs locaux et les comptes du domaine il met fin définitivement, et vous demande de saisir le domaine.
Le lien « Actions de domaine » sur la page d’un serveur distant ne liste que les actions de ce domaine, et « Nouvelle action » y démarre le formulaire avec le domaine renseigné.

- Les domaines sont enregistrés normalisés en étiquette A IDNA (minuscules, sans point final). `例え.jp` et `xn--r8jz45g.jp` sont le même domaine.
- Une action s’applique aussi aux sous-domaines (une action sur `spam.example` s’applique aussi à `a.spam.example`).
- Une action est appliquée à la projection des restrictions (`domain_restrictions`) et aux restrictions des fils dans la transaction qui l’enregistre, et prend effet à partir de la prochaine livraison, récupération ou réception.
- Une action avec une heure de fin (`-duration` / `expires_in`, dix ans au plus) prend fin d’elle-même à ce moment. L’expiration n’est pas une « levée » ; les listes l’affichent donc comme `expired`.
- Lever ne supprime que les restrictions de cette action. Les autres actions sur le même domaine restent. Lever une action déjà levée ne change rien.
- Les listes sont des plus récentes aux plus anciennes, et `-active` (`active=true`) n’affiche que les actions en vigueur.

<!-- source: importing-and-exporting-mastodon-csv f9b0965f8f91 -->
### Importer et exporter du CSV Mastodon

Vous pouvez échanger la liste des actions de domaine avec Mastodon au format CSV des blocages de domaine de Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`).
Vous pouvez importer un CSV exporté par Mastodon, et Mastodon peut importer un CSV exporté par ce serveur.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

Dans l’API d’administration : `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (le corps contient `csv`), et `POST .../import` (le corps contient `blocks`, les lignes choisies, et `private_note`).
Une ligne pour le propre domaine de ce serveur est ignorée (`own_domain`), et une action de domaine sur lui est refusée.
Dans le client web, ouvrez Modération → Actions de domaine et choisissez « CSV de Mastodon » (`/moderation/domain-actions/import`) : « Enregistrer en CSV » enregistre le fichier, et l’import affiche un aperçu d’un fichier choisi avant d’importer les lignes que vous conservez ; un `suspend` parmi elles demande d’abord confirmation.

| Ligne CSV | Action sur ce serveur |
| --- | --- |
| `#severity` vaut `suspend` (ou est vide) | `suspend` |
| `#severity` vaut `silence` | `limit` |
| `#severity` vaut `noop` | Aucune |
| `#reject_media` / `#reject_reports` est vrai | Ajoute respectivement `reject_media` / `reject_reports` |

- Un import se fait en deux étapes. Vérifiez d’abord dans l’aperçu quelles actions chaque ligne émettrait, choisissez les lignes à conserver, puis importez (la CLI ne change rien tant que vous n’ajoutez pas `-apply`, et `-only` choisit des lignes par domaine).
- Une ligne dont le domaine ou un domaine parent a déjà une action du même type en vigueur n’émet pas ce type (si chaque type est déjà là, c’est « déjà appliqué »). Une seconde ligne pour le même domaine, une ligne invalide et une ligne qui n’émettrait rien (`noop` seul) sont aussi ignorées. Importer de nouveau le même CSV n’ajoute pas d’actions.
- Un CSV sans en-tête `#domain` est lu comme une liste d’un domaine par ligne, et chaque domaine reçoit `suspend` (comme Mastodon). Un import accepte jusqu’à 5 000 lignes.
- Les actions importées durent « jusqu’à levée » et portent le commentaire public et l’obfuscation de la ligne ainsi que la note privée que vous avez donnée. Elles apparaissent dans le journal d’audit comme les actions émises une à une (l’acteur est la personne pour l’API et `cli` pour la CLI).
- Un export écrit une ligne par domaine pour les actions en vigueur. Un domaine avec un `suspend` reçoit `suspend`, un domaine avec un `limit` reçoit `silence`, et un domaine avec aucun des deux reçoit `noop`. Un domaine avec seulement `force_sensitive`, que les blocages de domaine de Mastodon n’ont pas, n’est pas exporté. Les notes privées et les heures de fin ne sont pas exportées.

<!-- source: subscribing-to-blocklists d49a93c3c44f -->
### S’abonner à des listes de blocage

Au lieu d’importer un CSV à la main, vous pouvez vous abonner à une liste de blocage de domaines qu’un autre serveur ou une autre personne publie à une URL `https`. Le serveur récupère la liste juste après votre abonnement, chaque jour ensuite, et lorsque vous le demandez, et transforme ses lignes en mesures sur les domaines : soit sous forme de brouillons qu’un modérateur accepte ou rejette (le mode `draft`, par défaut), soit directement à chaque récupération (le mode `automatic`).

```sh
my admin domain subscription add -format mastodon_csv -title "Shared blocklist" https://lists.example/blocklist.csv
my admin domain subscription list
my admin domain subscription drafts <subscription id>
my admin domain subscription accept -keys issue:suspend:spam.example <subscription id>
my admin domain subscription reject -all <subscription id>
my admin domain subscription update -mode automatic <subscription id>
my admin domain subscription fetch <subscription id>               # fetch now and print the outcome
my admin domain subscription fetches <subscription id>
my admin domain subscription remove [-lift-actions] <subscription id>
```

Dans l’API d’administration : `GET` et `POST /api/v1/admin/domain_list_subscriptions`, `GET`, `PATCH` et `DELETE .../{id}` (`lift_actions=true` lève les mesures de l’abonnement), `POST .../{id}/fetch`, `GET .../{id}/fetches`, `GET .../{id}/drafts`, et `POST .../{id}/drafts/accept` et `.../drafts/reject` (le corps contient `keys`, ou rien pour tous les brouillons). Toutes exigent `manage_domains`.

Dans le client web, ouvrez Modération → Mesures sur les domaines et choisissez « Abonnements aux listes de blocage » (`/moderation/domain-actions/subscriptions`). La liste montre le mode et la dernière récupération de chaque abonnement, avec les échecs consécutifs ; « Ajouter un abonnement » ouvre le formulaire du nom, de l’URL, du format et des réglages ci-dessous, et s’abonner ouvre la page de l’abonnement. Cette page a quatre sections : les brouillons, que vous acceptez ou rejetez un par un ou tous ensemble (accepter des brouillons qui suspendent un domaine demande d’abord confirmation) ; les récupérations, avec « Récupérer maintenant » et le résultat de chaque récupération ; les réglages ; et « Se désabonner », qui demande s’il faut lever les mesures émises par l’abonnement. La notification `admin_domain_list_failing` renvoie vers la liste.

| Format | Ce qu’est la liste |
| --- | --- |
| `mastodon_csv` | Le CSV des blocages de domaines de Mastodon, lu comme le lit un import |
| `mastodon_json` | La réponse de `GET /api/v1/instance/domain_blocks` d’un serveur Mastodon qui publie ses blocages de domaines. Les domaines qu’il masque (avec `*`) sont ignorés, et un serveur qui ne publie pas la liste (`401`, `403`, `404`) fait échouer la récupération |
| `plain_text` | Un domaine par ligne, chacun un `suspend` |

Ce qu’émet une ligne suit le même tableau qu’un import, modifié par les réglages de l’abonnement : `suspend_as` (`suspend`, `limit` ou `ignore` ; `suspend` par défaut), `silence_as` (`limit` ou `ignore` ; `limit` par défaut), si les `reject_media` et `reject_reports` de la ligne s’appliquent (les deux par défaut), et si les mesures copient le commentaire public de la ligne (pas par défaut).

- Une récupération passe par la même protection que les récupérations des autres serveurs : une URL, ou une redirection, qui mène à une adresse privée échoue. Elle lit au plus 2 MiB et 5 000 lignes ; une liste plus grande fait échouer la récupération. Le serveur renvoie l’`ETag` et le `Last-Modified` de la liste, de sorte qu’une liste inchangée coûte peu, et qu’une liste identique à la dernière lue ne change rien.
- Une récupération qui échoue ne change aucune mesure. Elle est enregistrée avec la raison de l’échec (`http_status`, `network`, `forbidden_destination`, `too_large`, `too_many_entries` ou `malformed`) et affichée dans la liste des abonnements et leurs récupérations. Après 3 échecs consécutifs, les personnes dont le rôle a `manage_domains` reçoivent la notification `admin_domain_list_failing`.
- Un abonnement ne lève ni ne remplace jamais une mesure qu’il n’a pas émise. Si une mesure du même type couvre déjà le domaine, sur lui ou sur un domaine parent, quelle que soit la personne qui l’a émise, l’abonnement n’émet rien à cet endroit. Lorsque deux abonnements listent le même domaine, la mesure de celui qui l’a émise en premier est en vigueur ; si celui-ci la lève, le suivant émet la sienne.
- Lorsque la liste retire un domaine, le mode automatique lève, à la récupération suivante, la mesure que l’abonnement a émise dessus. Si une récupération levait plus de 10 mesures et plus de la moitié de celles de l’abonnement, les levées attendent plutôt sous forme de brouillons, puisqu’une liste cassée ou vide ressemble à cela.
- Une gravité que le serveur ne connaît pas (ni `suspend`, ni `silence`, ni `noop`) attend toujours sous forme de brouillon, comme un `suspend` selon les réglages de l’abonnement.
- Les brouillons listent ce que la liste ajouterait (`issue`) et retirerait (`lift`), chacun avec une clé telle que `issue:suspend:spam.example`. Un brouillon rejeté ne revient pas tant que la liste ne change pas la ligne du domaine (un `issue`) ou que la mesure ne se termine pas (un `lift`).
- Si un modérateur lève une mesure qu’un abonnement a émise, l’abonnement ne la réémet pas tant que la liste ne change pas la ligne du domaine.
- Changer les réglages les applique à la récupération que le changement déclenche. L’URL et le format ne changent pas ; abonnez-vous de nouveau pour les changer.
- Supprimer un abonnement conserve les mesures qu’il a émises, qui comptent alors comme celles d’un modérateur, sauf si vous les levez avec lui.
- Le journal d’audit enregistre l’abonnement, la modification, la suppression, les brouillons acceptés et rejetés, et les récupérations demandées à la main. Les mesures du mode automatique sont enregistrées comme les autres mesures sur les domaines avec `system` comme acteur, et sont envoyées aux webhooks sous `domain_moderation_action.created` et `.lifted`, dont le `data` contient le `subscription_id`.

<!-- source: severed-follows 51be435d8635 -->
### Abonnements rompus

Un `suspend` de domaine et un `suspend` de compte distant coupent les abonnements entre les utilisateurs locaux et les comptes suspendus dans les deux sens.

- Pour un `suspend` de domaine, une tâche (`moderation.sever_domain`) qui s’exécute après l’enregistrement de la mesure les coupe un utilisateur local à la fois. Tant que la tâche n’est pas terminée, les abonnements apparaissent encore dans les listes. Les comptes des sous-domaines sont aussi coupés.
- Pour un `suspend` de compte distant, les abonnements sont coupés dans la transaction qui enregistre la mesure. Un second `suspend` d’un compte déjà suspendu ne coupe rien.
- Chaque utilisateur qui a perdu des abonnements approuvés reçoit une notification (`relationships_severed`) qui indique combien d’abonnés et d’abonnements ont été perdus. Un utilisateur qui n’a perdu que des demandes d’abonnement n’en reçoit aucune.
- Lever la mesure ne rétablit pas les abonnements coupés. Les utilisateurs se réabonnent.
- Traiter à nouveau la même mesure (par exemple lorsqu’une tâche se réexécute) n’ajoute ni enregistrements de relations rompues ni notifications.

<!-- source: federating-only-with-allowed-servers 84209876856e -->
### Fédérer uniquement avec des serveurs autorisés

Le paramètre d’instance `federation_scope` décide avec quels serveurs ce serveur fédère : `open` (par défaut) fédère avec tout serveur qu’aucune action de domaine ne suspend,
et `allowlist` uniquement avec les domaines de la liste d’autorisation et leurs sous-domaines, pour une organisation ou un serveur personnel qui ne parle qu’avec des serveurs qu’il connaît.

Tant que `federation_scope` vaut `allowlist` :

- Un serveur sans autorisation est traité comme un domaine suspendu : ses livraisons reçoivent `403` avant la vérification de leurs signatures, rien ne lui est livré ni n’est récupéré chez lui, et les relais n’apportent rien de lui.
- Une autorisation couvre ses sous-domaines : autoriser `friend.example` autorise aussi `social.friend.example`.
- Les actions de domaine s’appliquent toujours aux serveurs autorisés : un domaine autorisé avec un `suspend` reste suspendu, et un domaine avec un `limit` ou `reject_media` reste limité.
- Les GET ActivityPub exigent une signature d’un serveur autorisé, quoi que dise `authorized_fetch`, afin qu’un serveur sans autorisation ne puisse pas lire les publications publiques. Les serveurs et robots qui ne signent pas leurs GET reçoivent `401`.
- NodeInfo indique `metadata.federationScope`, afin que les autres serveurs puissent le savoir.

Tant qu’il vaut `open`, la liste d’autorisation ne change rien, et elle est conservée pour la prochaine fois que vous basculez.

```sh
my admin domain allow add -comment "partner" friend.example
my admin domain allow list
my admin domain allow remove friend.example
my admin federation scope                   # prints the setting
my admin federation scope allowlist         # shows what the switch would end, and changes nothing
my admin federation scope -apply allowlist  # switches
my admin federation scope open
```

Dans l’API d’administration : `GET /api/v1/admin/domain_allows`, `POST /api/v1/admin/domain_allows` (le corps contient `domain` et `comment` ; un domaine déjà autorisé répond `200` et conserve son autorisation), et `DELETE /api/v1/admin/domain_allows/{domain}`, avec la permission `manage_domains`.
`GET /api/v1/admin/settings/federation_scope/impact` compte ce que la bascule mettrait fin, avec la permission `manage_settings`, et `PATCH /api/v1/admin/settings` change `federation_scope` comme tout autre paramètre.
La liste d’autorisation contient au plus 5 000 domaines, et un commentaire, qui n’est jamais publié, au plus 5 000 caractères. Le propre domaine de ce serveur ne peut pas être autorisé.

Passer à `allowlist`, et retirer une autorisation pendant qu’elle est active, met fin définitivement à des abonnements :

- Avant de basculer, lisez combien d’abonnements approuvés entre les utilisateurs locaux et les serveurs sans autorisation la bascule mettrait fin, combien d’utilisateurs locaux en perdent un, et les serveurs qui en ont le plus (l’impact ci-dessus, ou `my admin federation scope allowlist` sans `-apply`).
- La bascule coupe les abonnements avec chaque serveur que la liste d’autorisation n’autorise pas, dans les deux sens et sans en informer ces serveurs, comme le fait un `suspend` de domaine : un job (`moderation.sever_allowlist`) les coupe un utilisateur local à la fois, et chaque utilisateur qui a perdu des abonnements approuvés reçoit une notification (`relationships_severed`) avec les nombres.
- Retirer une autorisation alors que `federation_scope` vaut `allowlist` coupe de la même façon les abonnements avec les comptes de ce domaine et de ses sous-domaines qu’aucune autre autorisation ne couvre. Retirer l’autorisation d’un sous-domaine dont le domaine parent est autorisé ne coupe rien.
- Revenir à `open`, ou autoriser de nouveau le domaine, ne ramène pas les abonnements coupés. Les utilisateurs suivent de nouveau.
- Définir `allowlist` alors qu’il vaut déjà `allowlist` ne coupe rien de plus.

Dans le client web, ouvrez Modération → Actions de domaine et choisissez « Domaines autorisés » (`/moderation/domain-actions/allowed`) pour lire,
ajouter et retirer des autorisations ; en retirer une demande d’abord confirmation, puisque cela coupe le domaine tant que `federation_scope` vaut `allowlist`. Le
paramètre lui-même est « Serveurs » sur la page Fédération des paramètres du serveur (`/admin/settings/federation`, avec la
permission `manage_settings`) : choisir « Serveurs autorisés uniquement » et enregistrer affiche les abonnements que la bascule mettrait fin, les comptes
locaux qui en perdent un, et les serveurs qui en ont le plus, et n’enregistre qu’une fois que vous avez confirmé.

L’ajout et le retrait d’une autorisation figurent dans le journal d’audit (`domain_allow_added` et `domain_allow_removed`), et la bascule est le `setting_changed` de `federation_scope`.

<!-- source: publishing-the-domains-you-act-on e8402b3a2c93 -->
### Publier les domaines sur lesquels vous agissez

Vous pouvez publier les actions `limit` et `suspend` en vigueur par `GET /api/v1/instance/domain_actions`.
Le paramètre d’instance `domain_actions_visibility` (`off` / `signed_in` / `everyone`, `off` par défaut) décide qui voit la liste, et
`domain_actions_rationale_visibility` (les mêmes valeurs, `off` par défaut) décide qui voit les commentaires publics ([paramètres de l’instance](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Avec `off`, l’API renvoie `404`, et avec `signed_in` elle renvoie `401` à une requête sans jeton d’accès.
- Il y a une entrée par action, des plus récentes aux plus anciennes. Une entrée contient le domaine, le SHA-256 du domaine (hexadécimal) et le type, et le commentaire public pour les personnes autorisées à le voir. Les notes privées ne sont jamais renvoyées.
- Pour une action avec `-obfuscate` (`obfuscate`), la moitié centrale des caractères du domaine (hors points) est remplacée par `*` (`bad.example` devient `bad.*****le`).
  Quiconque peut comparer le SHA-256 d’un domaine qu’il connaît pour vérifier s’il s’agit d’un domaine obfusqué.
- Les actions `reject_media`, `reject_reports` et `force_sensitive`, et les actions levées ou expirées, ne sont pas publiées.
- La page d’information de l’instance du client web (`https://<host>/about`) affiche cette liste selon les mêmes paramètres. Avec `off`, la section n’est pas affichée, et avec `signed_in`, les personnes non connectées voient une note indiquant que seuls les utilisateurs connectés peuvent la voir.

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

<!-- source: checking-what-reached-a-remote-server 6c1ec9c8cef1 -->
## Vérifier ce qui a atteint un serveur distant

Lorsqu’un utilisateur demande si une publication, un abonnement ou une acceptation a atteint un autre serveur, listez les livraisons vers ce serveur avec l’API d’administration.
Elle affiche chaque livraison d’une activité vers une boîte de réception du serveur, des plus récentes aux plus anciennes, avec chaque tentative et ce que le serveur a répondu, y compris les livraisons réussies.
Elle exige la permission `manage_domains`.
Dans le client web, la section **Livraisons** de l’écran du serveur distant (`/moderation/instances/{domain}`) affiche la même liste, avec ses filtres par **ID d’activité** et **Compte local**, et **Afficher** pour chaque champ masqué sous **Détails** d’une livraison masquée ; l’écran indique lesquels sont enregistrés dans le journal d’audit avant leur exécution.
La liste Serveurs distants n’affiche que les serveurs que les livraisons n’ont pas pu atteindre. Pour ouvrir tout autre serveur, saisissez son domaine, un identifiant sur lui ou une URL sur lui dans **Ouvrir un serveur** au-dessus de la liste,
ou utilisez **Ouvrir le serveur du compte** sur la page Examen d’un compte distant ou sur un signalement concernant un compte distant, ou le domaine dans une entrée du journal d’audit concernant un domaine.

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "https://example.com/api/v1/admin/instances/misskey.io/deliveries"
```

- Chaque livraison a le type d’activité (`Create`, `Follow`, `Accept`, etc.), la date d’enregistrement de l’activité et la date d’ajout de la boîte de réception, et ses `attempts`, des plus anciennes aux plus récentes.
  Chaque tentative a son `outcome` (`succeeded`, `retryable_failure`, `permanent_failure`, `skipped`), le `signature_scheme` et le `status_code` de la réponse, l’`error` d’un échec sans réponse ou la raison pour laquelle rien n’a été envoyé, et l’heure.
  Une tentative `succeeded` avec `202` signifie que l’autre serveur a accepté l’activité ; qu’il l’affiche ensuite dépend de ce serveur.
- Les tentatives sont conservées 30 jours ; l’historique remonte donc à 30 jours.
- Pour restreindre la liste, ajoutez `activity_uri=<the activity's ID>`, ou `account_id=<a local account's ID>` pour les livraisons d’un compte.
  Une requête avec `account_id` est enregistrée dans le journal d’audit sous `account_deliveries_viewed`, avec le compte et le serveur ; les autres requêtes ne sont pas enregistrées.
- Le corps de l’activité n’est jamais affiché.

Certaines livraisons sont masquées : leur expéditeur, l’URI de l’activité et la boîte de réception sont omis, et `mask` en indique la raison.

| `mask` | L’activité concerne |
| --- | --- |
| `private`, `direct`, `local` | une publication avec cette visibilité |
| `approves_followers` | un abonnement, son annulation, une acceptation ou un refus d’un compte local qui approuve ses abonnés |
| `unknown` | une publication ou un compte qui a disparu, de sorte que ce que c’était n’est plus connu |

Une boîte de réception partagée ne nomme aucun destinataire et est toujours affichée ; une boîte de réception personnelle masquée dans l’erreur d’une tentative se lit `[masked]`.
Le type, les heures et chaque tentative d’une livraison masquée sont affichés ; vous pouvez donc toujours voir si elle a atteint le serveur.
Pour voir les champs masqués d’une livraison, utilisez `POST /api/v1/admin/instances/{domain}/deliveries/{id}/reveal` ; chaque appel est enregistré dans le journal d’audit sous `delivery_revealed` avant que quoi que ce soit soit affiché.

<!-- source: reports 5791186f6b24 -->
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

Dans le client web, **Modération → Signalements** (`/moderation/reports`) liste les signalements ouverts, les plus récents d’abord, avec qui a déposé chacun, sa catégorie, la date et qui s’en occupe.
Ses filtres (état, catégorie et qui s’en occupe : vous ou personne) et l’ordre (la colonne **Déposé** : les plus récents ou les plus anciens en attente d’abord) restent dans l’adresse de la page, et **Précédent** et **Suivant** parcourent les pages.
La page d’un signalement (`/moderation/reports/{id}`) affiche son état, sa catégorie, les règles citées (une règle retirée depuis l’indique), qui l’a déposé et qui s’en occupe, s’il est allé au serveur du compte, et combien d’autres signalements et mesures le compte a ;
puis le compte ; le commentaire ; les publications signalées, une publication supprimée avec la date de suppression ; ce qui s’est passé dans le salon de chat vocal ou les modifications de l’événement ; et les notes.
À partir de `wide`, la page de la liste d’où vous venez se tient à côté. **M’assigner** et **Désassigner**, **Résoudre** (**Mesure prise** ou **Aucune mesure nécessaire**) et **Rouvrir** sont sous son titre, **Ajouter une note** sous les notes.

Dans l’API d’administration : `GET /api/v1/admin/reports` (filtre avec `state`, `category` et `assignee` : `me`, `none` ou l’ID d’un compte local ; `order=oldest` liste d’abord le signalement en attente depuis le plus longtemps au lieu du plus récent), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (corps `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (corps `{"body": "..."}`), `POST .../{id}/reopen` et `POST .../{id}/forward` (corps `{"comment": "..."}`, facultatif).

- Une assignation (`assign`) ne peut aller qu’à un utilisateur local dont le rôle a `manage_reports`. Dans l’API d’administration, vous vous assignez vous-même.
- Le résultat d’une résolution (`resolve`) est `actioned` (une mesure a été prise contre la cible) ou `dismissed` (aucune mesure n’était nécessaire). Résoudre de nouveau un signalement résolu ne change pas le résultat.
- Lorsque vous émettez une mesure à partir d’un signalement (`report_id` / `-report` de la mesure), le signalement est résolu comme `actioned` dans la même opération.
- Une note (`note`) est une note interne que seuls les modérateurs voient, de 2 000 caractères au plus. Elle n’est montrée ni à la cible ni à l’auteur du signalement. Elle ne peut être ni modifiée ni supprimée.
- Rouvrir (`reopen`) remet un signalement résolu à `open` (l’assignation reste). Un signalement ouvert ne peut pas être rouvert (`409` dans l’API d’administration).
- Transférer (`forward`, **Envoyer à leur serveur…** à la fin de la page du signalement, qui demande d’abord confirmation) envoie un signalement concernant un compte d’un autre serveur à ce serveur, comme un `Flag` de l’acteur de votre instance avec le compte et les publications signalées qui existent encore. Le seul texte qu’il porte est ce que vous écrivez pour les modérateurs de ce serveur lorsque vous le transférez (`-comment`, `comment` dans l’API d’administration, ou **Texte pour leurs modérateurs** sur la page du signalement ; jusqu’à 1 000 caractères, aucun par défaut). Le commentaire de l’auteur du signalement et la note d’un [examen externe](#reports-that-an-external-review-filed) ne quittent jamais votre serveur de cette façon, et rien sur l’auteur du signalement n’est envoyé. Un signalement que son auteur a choisi de transférer en le déposant est parti aussitôt, avec le commentaire de son auteur. Le transfert fonctionne pour un signalement déposé sur ce serveur, par une personne ou par un [examen externe](#reports-that-an-external-review-filed), ouvert ou résolu, qui n’a pas encore été transféré ; un signalement reçu d’un autre serveur ou concernant un salon de chat vocal ou un événement ne peut pas être transféré (`409` `report-not-forwardable` dans l’API d’administration, et les détails du signalement n’affichent aucun bouton). Un signalement n’est envoyé qu’une fois : le transférer de nouveau, avec n’importe quel texte, ou transférer un signalement que son auteur avait transféré en le déposant, ne change rien. `forwarded` dans l’API d’administration indique s’il a été envoyé, `forwardable` si vous pouvez l’envoyer maintenant.
- Les détails du signalement (`show`, `GET .../{id}`) renvoient les notes, des plus anciennes aux plus récentes, avec le nombre d’autres signalements concernant la cible et le nombre de mesures contre la cible (y compris celles levées et expirées). La liste ne renvoie pas les notes.
- Les assignations, résolutions, réouvertures, notes et transferts sont conservés sous forme d’événements (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), et `report_states` contient l’état actuel.
- Les modérateurs peuvent traduire les publications signalées et le commentaire de l’auteur du signalement sur la page du signalement, ou avec `POST .../{id}/statuses/{status_id}/translation` et `POST .../{id}/comment/translation` (corps `{"target_language": "en"}`).
  Le texte n’est envoyé à un fournisseur de traduction que tant que le [paramètre d’instance](instance-settings.md) `translation_reports_to_server` est activé, et alors uniquement à un fournisseur autorisé pour la visibilité de la publication : l’accès d’un modérateur n’élargit rien ; une publication directe ne va donc qu’à un fournisseur autorisé pour `direct`, et le commentaire compte comme `private`.
  La traduction du commentaire n’est pas conservée, pas plus que celle d’une publication supprimée, et les caractères ne comptent que dans la limite mensuelle du serveur. La traduction intégrée du navigateur, lorsque `translation_browser` est activé, fonctionne sur les publications signalées quoi que dise `translation_reports_to_server` ; le commentaire, qui ne nomme aucune langue, n’est traduit que par le serveur.
- Un auteur de signalement ne voit de la cible que ce qu’il verrait du compte autrement : tant qu’elle est suspendue ou le bloque, son signalement reste listé sans le compte. Les modérateurs voient toujours toute la cible.
- Un auteur de signalement local reçoit une notification (`report_resolved`) à la première résolution. Rouvrir puis résoudre de nouveau n’ajoute aucune notification.
  Un auteur de signalement peut lister ses propres signalements avec `GET /api/v1/reports` et ne voit que si chacun est ouvert ou résolu. Le résultat, la mesure prise, la raison donnée à la cible et la personne assignée ne sont pas montrés.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Signalements déposés par un examen externe

Un [point de décision](decision-hooks.md) qui répond `flag` dépose un signalement avec l’acteur de votre instance comme auteur. Le signalement l’indique : `hook_origin` dans l’API d’administration et le webhook `report.created`, une ligne dans les détails du signalement, et `automated` dans `my admin report show`. Son commentaire est la note du plugin, et sa catégorie provient du motif. Il n’est pas transmis lorsqu’il est déposé, et le résoudre ne notifie personne. Lorsque la cible est sur un autre serveur, vous pouvez le transmettre vous-même (`forward`) : la note du plugin reste sur votre serveur, et seul le texte que vous écrivez part. Annuler la décision laisse le signalement tel quel.

<!-- source: voice-chat-rooms d2dd76116ac4 -->
### Salons de chat vocal

- Un utilisateur local peut signaler un salon de chat vocal ouvert qu’il peut voir (`POST /api/v1/reports` avec `voice_room_id`, et `account_id` défini sur l’hôte du salon).
  Le signalement cible l’hôte à ce moment-là et renvoie aux paramètres du salon (nom et description) tels qu’ils étaient alors. Il ne renvoie à aucune publication et n’est jamais transféré à un autre serveur.
- Les détails du signalement (`GET /api/v1/admin/reports/{id}`) affichent le salon (`voice_room`) : chaque modification de ses paramètres, qui y a participé et quand cette personne est partie, qui en a été retiré, et comment il s’est terminé.
  Les enregistrements de qui a participé sont conservés 90 jours après la fin du salon (`voice_participants_retention_days`) et aussi longtemps qu’un signalement renvoie au salon ([conservation des enregistrements](#record-retention-and-holds)).
  Il n’y a ni enregistrements audio ni discussion textuelle ; ces enregistrements sont donc la seule preuve.
- Un titulaire de `manage_reports` peut mettre fin à un salon ouvert pour tous les participants avec `POST /api/v1/admin/voice_rooms/{id}/end` (corps `{"reason": "..."}`, facultatif, jusqu’à 500 caractères).
  La raison n’est destinée qu’aux modérateurs ; les participants voient seulement que le salon a pris fin. Cela fonctionne aussi lorsque le chat vocal est désactivé, et le journal d’audit enregistre `voice_room_ended`.
  Dans le client web, la page du signalement d’un salon ouvert comporte « Mettre fin au salon », qui demande d’abord confirmation, avec la raison.

<!-- source: events cbc3e19bfdb6 -->
### Événements

- Un utilisateur local peut signaler un événement qu’il peut voir, y compris un événement annulé (`POST /api/v1/reports` avec `event_id`, et `account_id` défini sur l’organisateur de l’événement).
  Le signalement cible l’organisateur et renvoie aux paramètres de l’événement (titre, description et lieux) tels que l’auteur du signalement les a vus. Un événement encore en sondage de date peut être signalé et annulé comme n’importe quel autre. Il ne renvoie à aucune publication et n’est jamais transféré à un autre serveur.
- Les détails du signalement (`GET /api/v1/admin/reports/{id}`) affichent l’événement (`event`) : sa date, s’il a été annulé et comment, et chaque modification de ses paramètres, des plus anciennes aux plus récentes, avec tous ses lieux, y compris ceux que seuls les participants approuvés voyaient.
  `reported_revision_id` est la version que l’auteur du signalement a vue ; un titre ou une adresse modifié après le signalement reste donc sous vos yeux.
- Un titulaire de `manage_reports` peut annuler un événement avec `POST /api/v1/admin/events/{id}/cancel` (corps `{"reason": "..."}`, facultatif, jusqu’à 500 caractères).
  La raison est montrée avec l’annulation à toutes les personnes qui voient l’événement. Les utilisateurs qui ont répondu qu’ils viennent ou peut-être, et les utilisateurs invités, sont notifiés, comme pour l’annulation par l’organisateur lui-même, et le journal d’audit enregistre `event_canceled`.
  Un événement déjà annulé répond `409` `event-closed`. Dans le client web, la page du signalement d’un événement comporte « Annuler l’événement », qui demande d’abord confirmation, avec la raison.
- Suspendre ou supprimer l’organisateur annule les événements de l’organisateur qui n’ont pas encore commencé, y compris ceux encore en sondage de date (`gathering.cancel_organized`), et notifie leurs participants. Ces annulations ne nomment personne comme auteur de l’annulation, et lever la suspension ne les rétablit pas. Les événements qui ont déjà commencé ne sont pas annulés.

<!-- source: notifications-for-moderators-and-the-pending-counts 0d1c327abe83 -->
## Notifications pour les modérateurs et compteurs en attente

Les nouveaux signalements, les inscriptions en attente d’approbation et les contestations parviennent aux personnes qui ont la permission sous forme de notifications dans l’application.

| Notification | Quand elle arrive | Qui la reçoit |
| --- | --- | --- |
| `admin_report` | Lorsque la cible du signalement n’a aucun autre signalement ouvert (un deuxième signalement ou un suivant concernant le même compte ne notifie pas) | Les personnes dont le rôle a `manage_reports`, sauf l’auteur du signalement et la cible du signalement |
| `admin_sign_up` | Lorsqu’une inscription démarre en attente d’approbation (mode d’approbation, ou `open` sur une configuration qui ne peut pas envoyer d’e-mails) | Les personnes dont le rôle a `manage_accounts` |
| `admin_appeal` | Lorsqu’une contestation est faite contre une mesure | Les personnes dont le rôle a `manage_accounts`, sauf la personne qui a contesté |
| `admin_custom_emoji_request` | Lorsque la création d’un émoji personnalisé ou le remplacement de son image est demandé | Les personnes dont le rôle a `manage_custom_emojis`, sauf le demandeur |
| `admin_error_group` | Lorsque le serveur ou le client web signale une erreur d’un nouveau type, ou qu’un groupe d’erreurs qui avait été résolu se reproduit sur une autre version du serveur | Les personnes dont le rôle a `manage_errors` |
| `admin_domain_list_failing` | Lorsque la récupération d’une liste de blocage abonnée échoue pour la troisième fois de suite | Les personnes dont le rôle a `manage_domains` |

- `admin_error_group` arrive au plus 10 fois par heure pour tout le serveur. Le reste de l’heure se replie en une seule notification
  qui indique combien de groupes d’erreurs de plus il y a eu. Un groupe en sourdine ne notifie jamais. Une erreur que le client web signale pour un
  visiteur non connecté ne notifie qu’une fois que le navigateur d’un utilisateur connecté la signale aussi, et sa récurrence ne notifie personne.
  Les groupes d’erreurs sont lus et résolus, mis en sourdine ou réactivés par `GET /api/v1/admin/error_groups` et
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`), qui exigent `manage_errors`.
- Qui reçoit une notification est décidé par sa permission au moment où la notification est créée. Une personne qui perd ensuite la permission ne voit plus ce type dans sa liste ni dans son compteur de non lues. Si la permission revient, elle le revoit.
- Chaque type de notification ci-dessus arrive aussi par e-mail, activé par défaut. Chaque personne active ou désactive chaque type dans l’application et par
  e-mail sur la page **Notifications** des paramètres (`/settings/notifications` dans le client web), qui ne liste que les
  types que son rôle reçoit. Un type désactivé dans l’application n’est pas créé pour elle ; il n’est donc ni envoyé en push ni par e-mail ; les compteurs de
  `GET /api/v1/admin/counts` ne changent pas, car ils comptent les signalements, les inscriptions et les contestations eux-mêmes.
  Avec les signalements ouverts, les compteurs renvoient aussi la date de dépôt du plus ancien signalement ouvert (`oldest_open_report_filed_at`).
  Le serveur met en file le job d’e-mail dans la transaction qui enregistre la notification, et le job envoie l’e-mail juste après la
  validation, sans attendre que la notification soit lue. Si la personne a perdu la permission ou désactivé l’e-mail de ce type
  au moment de l’envoi, ou si son adresse principale n’est pas confirmée ou que ses e-mails sont arrêtés, rien n’est envoyé.
  L’e-mail ne contient que le type de notification et un lien vers l’écran des notifications, et rien du contenu du signalement ou du compte. Son lien de désabonnement ne désactive que l’e-mail de ce type.
  Avec `mail = none`, aucun e-mail n’est envoyé, et `sends_email` dans `GET /api/v1/instance` vaut `false`.

`GET /api/v1/admin/counts` renvoie le nombre de signalements ouverts (`open_reports`, exige `manage_reports`), le nombre d’inscriptions en attente d’approbation (`pending_registrations`) et le nombre de contestations en attente d’examen (`pending_appeals`) (les deux exigent `manage_accounts`), le nombre de demandes d’émojis personnalisés en attente d’une décision (`pending_custom_emoji_requests`, exige `manage_custom_emojis`), et le nombre de groupes d’erreurs ouverts, nouveaux, régressés ou réactivés (`open_error_groups`, exige `manage_errors`).
Les éléments pour lesquels vous n’avez pas la permission ne sont pas renvoyés.

<!-- source: audit-log 8334cc0280f5 -->
## Journal d’audit

Les opérations des modérateurs et des administrateurs sont enregistrées à raison d’une ligne chacune dans le journal d’audit (`audit_log_entries`) dans la même transaction que l’opération.
Les opérations en échec et celles qui n’ont rien changé ne sont pas enregistrées : lever une mesure déjà levée, résoudre un signalement résolu, assigner à la même personne, retirer une règle déjà retirée, déplacer à la même position, attribuer un rôle que l’utilisateur a déjà, et révoquer un rôle que l’utilisateur n’a pas.

| Type de cible | Opérations |
| --- | --- |
| `account` | `account_action_issued` et `account_action_lifted` (émettre et lever une mesure contre un compte), `account_deletion_canceled` (la suppression annulée parce qu’une contestation contre un `ban` a été annulée), `revealed_private_data` (adresses IP et adresses e-mail révélées), `two_factor_disabled` (authentification à deux facteurs retirée), `password_reset_link_issued` (un lien de réinitialisation du mot de passe émis), `email_suppression_lifted` (suppression d’e-mail levée), `account_moved` (un compte local restreint a déménagé vers un compte local ; les restrictions ne sont pas reportées sur la destination, c’est donc à vous de décider de les y appliquer), `account_statuses_refetch_queued` (nouvelle récupération des publications d’un compte distant), `account_deliveries_viewed` (les livraisons d’un compte local vers un serveur listées ; le nom de la cible est l’identifiant et le serveur) |
| `status` | `status_refetch_queued` (nouvelle récupération d’une publication distante ; le nom de la cible est l’URI de la publication) |
| `domain` | `domain_action_issued` et `domain_action_lifted` (émettre et lever une action de domaine), `domain_statuses_refetch_queued` (nouvelle récupération des publications des comptes du domaine), `instance_marked_available` (effacement des échecs d’un serveur), `delivery_revealed` (l’expéditeur masqué, l’URI de l’activité et la boîte de réception d’une livraison vers le serveur affichés) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (assigner, désassigner, résoudre et rouvrir un signalement, ajouter une note, et le transférer) |
| `user` | `registration_approved`, `registration_rejected` (examen de l’inscription) |
| `setting` | `setting_changed` (paramètres de l’instance), `text_changed` (textes de l’instance) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (règles de l’instance) |
| `role` | `role_granted`, `role_revoked` (attribuer et révoquer un rôle pour un utilisateur), `role_created`, `role_updated`, `role_deleted` (créer, modifier et supprimer un rôle) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (examen de la contestation ; l’annuler enregistre aussi `account_action_lifted` pour la mesure, et `account_deletion_canceled` pour un `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (ajouter et lever un blocage d’inscription) |
| `hook_decision` | `hook_decision_applied` (le serveur a agi sur le verdict d’un examen externe et laissé un état ; l’acteur est le système), `hook_decision_overridden` (un modérateur a annulé la décision, aussi en annulant sa contestation), `hook_appeal_upheld` (un modérateur a maintenu la contestation d’une publication refusée, et la décision reste) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (enregistrer un émoji personnalisé, remplacer son image, copier un émoji d’un autre serveur, le modifier, l’activer, le désactiver et le supprimer) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (créer, modifier, supprimer, activer et désactiver un webhook, et régénérer son secret) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn`, `announcement_deleted` (créer, modifier, publier et retirer une annonce, et supprimer un brouillon) |
| `invite` | `invite_created`, `invite_revoked` (créer et révoquer une invitation ; le nom de la cible est constitué des 4 premiers caractères du code) |
| `billing_source` | `billing_import_queued` (un import de facturation a été lancé à la main ; le nom de la cible est le nom du fournisseur) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (enregistrer un coût, changer son nom et autres, réviser son montant, changer sa date de fin et le supprimer ; le nom de la cible est le nom de l’élément) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (activer et désactiver la facturation ; le nom de la cible est le nom du fournisseur de paiement), et `supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled` (activer les pourboires, changer leurs montants et les désactiver ; le nom de la cible est la devise et les montants en unités mineures) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (créer, modifier et retirer une formule de soutien ; le nom de la cible est le nom de la formule) |
| `supporter_subscription` | `supporter_subscription_ended` (un administrateur a mis fin aussitôt à un abonnement ; le nom de la cible est le nom de la formule et l’identifiant de l’utilisateur) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (soumettre, retirer, approuver et rejeter une demande d’émoji personnalisé ; le nom de la cible est le `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (la réconciliation avec le fournisseur de paiement a été lancée à la main ; le nom de la cible est le nom du fournisseur) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (désactiver et réactiver un client OAuth ; le désactiver révoque tous ses jetons ; le nom de la cible est le nom du client et `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (refuser et lever le refus d’un hôte de Client ID Metadata Document ; le refuser révoque les jetons des clients de cet hôte ; le nom de la cible est l’hôte) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (annoncer la fermeture du service ou en changer la date, retirer l’annonce, fermer, et terminer la livraison de la fermeture ; le nom de la cible est `service_closure`) |
| `voice_room` | `voice_room_ended` (un modérateur a mis fin à un salon de chat vocal ouvert ; le nom de la cible est l’ID du salon, puisque le nom disparaît lorsque le compte qui l’a écrit est effacé ; les détails du signalement affichent le nom) |
| `event` | `event_canceled` (un modérateur a annulé un événement ; le nom de la cible est l’ID de l’événement, puisque le titre disparaît lorsque le compte qui l’a écrit est effacé ; les détails du signalement affichent le titre) |
| `domain_allow` | `domain_allow_added`, `domain_allow_removed` (ajouter un domaine à la liste d’autorisation de la fédération et l’en retirer ; voir [fédérer uniquement avec des serveurs autorisés](#federating-only-with-allowed-servers) ; le nom de la cible est le domaine) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (définir une règle de l’outil de récupération des cartes de lien sur un domaine, et la retirer ; voir [domaines de cartes de lien](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (ajouter, modifier, supprimer, activer et désactiver un fournisseur de connexion externe, et remplacer son secret client ; le nom de la cible est le nom d’affichage du fournisseur) |
| `domain_list_subscription` | `domain_list_subscription_created`, `domain_list_subscription_updated`, `domain_list_subscription_deleted` (s’abonner à une liste de blocage, changer ses paramètres et la retirer), `domain_list_draft_accepted` et `domain_list_draft_rejected` (accepter un brouillon, ce qui enregistre aussi l’action de domaine, et en rejeter un), `domain_list_fetch_queued` (une récupération demandée à la main) ; le nom de la cible est le titre de l’abonnement ; voir [s’abonner à des listes de blocage](#subscribing-to-blocklists) |

La liste indique si la cible existe encore. Les comptes, signalements, contestations et blocages d’inscription qui ont disparu après leur période de conservation, les autorisations retirées, ainsi que les rôles, émojis personnalisés et webhooks supprimés, n’affichent que le nom au moment de l’enregistrement et aucun lien vers un écran.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

Dans l’API d’administration : `GET /api/v1/admin/audit_log` (filtre avec `actor_account_id`, `action`, `target_kind` et `target_id`). Elle exige la permission `view_audit`.
Dans le client web, ouvrez Modération → Journal d’audit (`/moderation/audit-log`) : chaque entrée se lit comme une phrase de qui a fait quoi à quoi, des plus récentes aux plus anciennes, la personne et la cible liées à leurs écrans.
Filtrez-le par qui l’a fait (vous, ou la personne d’une entrée via « Ses autres opérations »), par type de cible puis d’opération, et par une cible (« Autres opérations sur ceci » d’une entrée).

- Une ligne est l’acteur (l’utilisateur pour `user`, ou `cli` pour la CLI), l’opération, le type et l’ID de la cible, le nom d’affichage de la cible, l’ID de l’enregistrement d’origine et l’heure.
  L’ID de la cible est l’ID du compte, du signalement, de la contestation, de l’utilisateur, de la règle, du rôle ou du blocage d’inscription ; pour une action de domaine, c’est le domaine ; pour les paramètres et les textes, c’est la clé.
- Le nom d’affichage de la cible (`@alice`, `@bob@example.com`, un domaine, une clé de paramètre, le texte d’une règle ; pour un texte sa clé, pour un rôle sa clé et l’identifiant de l’utilisateur, pour un blocage d’inscription son type et sa valeur) est une copie prise au moment de l’enregistrement, et reste donc lisible après la disparition de la cible.
  Chaque ligne de l’API renvoie aussi si la cible existe encore (`target_exists`), qui vaut `false` pour les comptes, signalements, contestations et blocages d’inscription disparus. Le journal d’audit web n’affiche que le nom d’affichage, sans lien, pour une cible disparue.
- Le texte des motifs et des notes n’est pas copié. Retrouvez-le par l’ID de l’enregistrement d’origine (la mesure, l’assignation, la résolution, la réouverture, la note, l’examen, ou la version d’un paramètre ou d’une règle).
- La liste va des plus récentes aux plus anciennes, avec une pagination par clé (`-before <entry id>` dans la CLI).
- Il n’existe ni API ni CLI pour modifier ou supprimer le journal d’audit. Les anciennes lignes sont supprimées après la [période de conservation](#record-retention-and-holds).

<!-- source: record-retention-and-holds 9861d8570a91 -->
## Conservation des enregistrements et conservations obligatoires

Après la durée de conservation de chaque type d’enregistrement, des tâches horaires (`moderation.purge_records`, `identity.purge_origins`, `voice.purge_records` et `gathering.purge_records`) retirent les enregistrements de modération.
Vous pouvez modifier les durées dans les paramètres d’instance, mais pas en dessous de la limite inférieure ([paramètres de l’instance](instance-settings.md)). Une durée modifiée prend effet à partir de la prochaine exécution. Une conservation obligatoire levée, avec son motif, est retirée une fois que la durée de conservation des mesures de modération s’est écoulée depuis sa levée.

| Enregistrement | Paramètre | Valeur par défaut | Limite inférieure | Point de départ |
| --- | --- | --- | --- | --- |
| Adresse IP et User-Agent de connexion | `session_origins_retention_days` | 180 jours | 90 jours | La connexion |
| Adresse IP d’inscription | `registration_origins_retention_days` | 180 jours | 90 jours | L’inscription |
| Signalements (commentaire, publications visées, notes, attributions, résolutions, réouvertures) | `reports_retention_days` | 365 jours | 180 jours | La dernière résolution |
| Mesures contre des comptes (motif, note, publications visées, levée) et contestations | `moderation_actions_retention_days` | 365 jours | 180 jours | La fin de la mesure |
| Journal d’audit | `audit_log_retention_days` | 730 jours | 365 jours | L’enregistrement |
| Blocages d’inscription | Aucun (ne peut pas être modifié) | 30 jours | s. o. | La première des deux dates entre la levée et l’expiration |
| Qui a participé à un salon de chat vocal (participations, intervenants et auditeurs, mains levées, invitations, retraits) | `voice_participants_retention_days` | 90 jours | 30 jours | La fin du salon |
| Les décisions des [hooks de décision](decision-hooks.md#the-decisions) | `hook_decisions_retention_days` | 90 jours | 30 jours | La décision |
| Les réponses, approbations, invitations et votes sur les dates candidates d’un événement | `event_records_retention_days` | 90 jours | 30 jours | La première des deux dates entre la fin de l’événement (pour un événement dont la date n’a jamais été fixée, la fin de sa dernière date candidate) et son annulation |

- Les signalements ouverts ne sont jamais retirés. Un signalement auquel une mesure fait référence (`report_id` de la mesure) reste jusqu’à ce que cette mesure soit retirée.
- La fin d’une mesure est la première des deux dates entre la levée et l’expiration. Pour une mesure qui a fait l’objet d’une contestation, si l’examen survient après cela, la fin est le moment de l’examen, et la mesure est conservée tant que la contestation attend son examen.
  `warn` et `delete_statuses` n’ont aucun effet durable ; leur fin est donc le moment où elles ont été émises. Une mesure sans fin (un `ban`, un `suspend` sans heure de fin, etc.) n’est retirée que lorsqu’elle est levée.
- Les publications auxquelles un signalement ou une mesure fait référence ne sont pas physiquement supprimées par la tâche de purge, avec leurs médias joints, tant que ce signalement ou cette mesure n’est pas retiré, même si l’auteur les a supprimées (elles disparaissent de la vue). Les publications distantes restent aussi au-delà de la durée de conservation (y compris le paramètre d’instance `remote_status_max_retention_days`).
- Un blocage d’inscription qui n’a ni levée ni heure de fin reste aussi longtemps qu’il est en vigueur.
- Les enregistrements de qui a participé à un salon de chat vocal visé par un signalement restent jusqu’à ce que ce signalement soit retiré. Le salon lui-même (son nom, sa description, ses hôtes et la manière dont il s’est terminé) n’est jamais retiré.
- Les réponses, votes et invitations d’un événement visé par un signalement restent jusqu’à ce que ce signalement soit retiré, tout comme ceux qui nomment un compte sous conservation obligatoire. L’événement lui-même (ses réglages et lieux, sa date et ses dates candidates, ses co-organisateurs et son annulation) n’est jamais retiré ; une fois ses réponses disparues, il ne compte plus personne comme participant.
- Une exécution de tâche ne retire qu’un nombre limité de lignes par type (100 à la fois, 20 fois, pour les mesures, les signalements et les blocages d’inscription ; 1 000 lignes à la fois, 20 fois, pour le journal d’audit). L’exécution suivante retire le reste.

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
