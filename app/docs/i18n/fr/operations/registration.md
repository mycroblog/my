<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# Inscription des utilisateurs (modes d’inscription, invitations, approbation, confirmation de l’e-mail)

Comment les utilisateurs s’inscrivent et confirment leur adresse e-mail, les modes d’inscription, les codes d’invitation, l’approbation des demandes, les réinitialisations de mot de passe et les blocages d’inscription.

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## De l’inscription à l’application

![Les parcours d’un nouvel utilisateur : le formulaire d’inscription ou un fournisseur de connexion, les contrôles qui peuvent refuser l’inscription, l’état que donne chaque mode d’inscription, puis l’approbation et la confirmation de l’e-mail avant l’application. La liste ci-dessous dit la même chose.](images/sign-up-flow.svg)

Du haut du schéma vers le bas :

- **Début.** Un visiteur crée un compte sur la page d’inscription (`/auth/sign_up`) avec un nom d’utilisateur, une adresse e-mail et un mot de passe, ce que la page ne propose pas lorsque [l’inscription est limitée aux fournisseurs de connexion](#registration-limited-to-login-providers). Ou bien le visiteur se connecte avec un [fournisseur de connexion](login-providers.md) dont le compte n’est pas encore associé à un utilisateur ; lorsque le fournisseur autorise les inscriptions (`-allow-signup`), le visiteur obtient le même formulaire sans le champ du mot de passe.
- **Contrôles, dans l’ordre.** Le mode d’inscription (`closed` refuse toute inscription), la limite par réseau (5 par 30 minutes), le CAPTCHA (lorsque `captcha` n’est pas `none`), les [blocages d’inscription](#sign-up-blocks) (`reject` refuse, `require_approval` met en attente d’approbation), le code d’invitation (il doit être utilisable, et le mode `invite` en exige un), le plugin de [`signup.check`](decision-hooks.md#sign-ups) pour une inscription sans invitation, et un nom d’utilisateur et une adresse e-mail qui ne sont pas déjà pris. Une inscription qui échoue à un contrôle est refusée avec une erreur sur le formulaire, et rien n’est écrit.
- **État juste après l’inscription.** Le nouvel utilisateur est connecté immédiatement. En mode `open`, l’inscription est approuvée, ou attend l’approbation avec `mail = none` ; en mode `approval`, elle attend l’approbation, ou est approuvée avec une invitation utilisable ; en mode `invite`, elle est approuvée. Un blocage `require_approval` ou une mise en attente par `signup.check` la laisse en attente quel que soit le mode.
- **Jusqu’à ce que le compte soit prêt.** Une inscription en attente a besoin de l’approbation d’un modérateur (`my registration approve`, ou les écrans d’administration). L’adresse e-mail doit être confirmée via le lien d’un message avec `mail = smtp` ; une adresse que le fournisseur de connexion a vérifiée est confirmée immédiatement, et avec `mail = none` aucune confirmation n’est nécessaire. Une inscription rejetée n’est pas approuvée, et le compte ne peut pas utiliser l’application.
- **L’application.** Un compte prêt passe à l’étape d’accueil (un nom d’affichage et un avatar), puis à la fil d’accueil.

Les sections ci-dessous décrivent chaque étape.

<!-- source: the-sign-up-page 111fbe5ecf64 -->
## La page d’inscription

Les utilisateurs s’inscrivent sur `https://<host>/auth/sign_up` avec un nom d’utilisateur, une adresse e-mail et un mot de passe, sauf si [l’inscription est limitée aux fournisseurs de connexion](#registration-limited-to-login-providers). Après l’inscription, ce navigateur est connecté immédiatement. La page fonctionne sans JavaScript et, comme la page de connexion, est protégée par un jeton CSRF et par le rejet des requêtes inter-origines. Lorsque vous choisissez un CAPTCHA pour la protection contre les robots, les pages d’inscription et de connexion affichent son widget et ont besoin de JavaScript ([CAPTCHA](captcha.md)).

Le mode d’inscription est le paramètre d’instance `registration_mode`. Choisissez-le avec la CLI ou dans le client web sur la page « Inscription » des « Paramètres du serveur » (`/admin/settings/registration`) ([paramètres d’instance](instance-settings.md)). Une modification s’applique à partir de l’inscription suivante. Tant que le serveur annonce sa fermeture et après sa fermeture, l’inscription est traitée comme `closed` quel que soit le paramètre ([CLI d’administration](admin-cli.md#announcing-the-end-of-the-service)). Il en va de même sur un [serveur d’une seule personne](instance-settings.md#one-person-server), où les invitations, l’examen des inscriptions et les blocages d’inscription ne sont pas proposés non plus, et où aucun moyen ne crée de compte, `my admin create` compris.

```sh
my instance set registration_mode invite
```

| Mode | Page | État juste après l’inscription |
| --- | --- | --- |
| `open` | Tout le monde peut s’inscrire ; un code d’invitation est facultatif | Approuvée, sauf avec `mail = none`, où elle attend l’approbation d’un modérateur (voir ci-dessous). Une invitation ne change rien d’autre que d’enregistrer qui a invité l’utilisateur |
| `approval` (par défaut) | Demande aussi « Pourquoi souhaitez-vous nous rejoindre ? » (facultatif, jusqu’à 1 000 caractères) ; un code d’invitation est facultatif | En attente d’approbation ; approuvée immédiatement avec une invitation utilisable |
| `invite` | Un code d’invitation est obligatoire | Approuvée |
| `closed` | Indique seulement que l’inscription n’est pas acceptée | Aucun |

- Dans tous les modes sauf `closed`, la page comporte un champ de code d’invitation, rempli à partir du lien d’une invitation (`?invite=<code>`). Un code qui ne fonctionne pas (inconnu, révoqué, expiré, épuisé, ou d’un membre qui ne peut plus inviter) est refusé dans tous les modes, et non ignoré, afin qu’un code mal saisi ne passe pas inaperçu.
- Un blocage d’inscription qui exige une approbation maintient une inscription en attente d’approbation même avec une invitation ([blocages d’inscription](#sign-up-blocks)).
- Avec un plugin configuré pour le point de décision [`signup.check`](decision-hooks.md#sign-ups), une inscription sans invitation est aussi examinée par ce plugin après le CAPTCHA et les blocages, et il peut la mettre en attente d’approbation ou la refuser.

- Les inscriptions depuis le même réseau (adresse IP ; `/64` pour IPv6) sont limitées à 5 par 30 minutes. Au-delà, le serveur renvoie `429` avec `Retry-After`.
- Si le nom d’utilisateur ou l’adresse e-mail est déjà pris, la page affiche une erreur sur ce champ et n’écrit rien (une invitation n’est pas consommée non plus).
- Lorsque vous configurez des [fournisseurs de connexion externes](login-providers.md) (OpenID Connect, GitHub, Discord ou Mastodon) et laissez les nouveaux utilisateurs s’inscrire avec l’un d’eux, les pages de connexion et d’inscription affichent un bouton pour chaque fournisseur activé, libellé avec le nom d’affichage du fournisseur exactement tel que vous l’avez saisi (un fournisseur GitHub affiche « Se connecter avec GitHub », et un fournisseur Discord « Se connecter avec Discord », dans la langue du visiteur). Une personne dont le compte chez le fournisseur n’est associé à aucun utilisateur de ce serveur obtient le même formulaire d’inscription sans le champ du mot de passe, et le nouvel utilisateur se connecte uniquement avec le fournisseur. Le mode d’inscription, le code d’invitation, la raison, le CAPTCHA, les blocages d’inscription, `signup.check` et la limite par réseau s’appliquent comme sur le formulaire avec mot de passe ; en mode `closed`, ou lorsque le fournisseur n’autorise pas les inscriptions, la page de connexion indique qu’aucun compte n’est associé. Une adresse que le fournisseur marque comme vérifiée (`email_verified` ; pour GitHub, l’adresse principale lorsqu’elle est vérifiée ; pour Discord, l’adresse lorsque Discord la marque comme vérifiée) est confirmée immédiatement si l’utilisateur la conserve ; toute autre adresse, ainsi que chaque adresse d’un utilisateur qui s’inscrit avec un serveur Mastodon, qui n’en fournit aucune, est confirmée par e-mail comme ci-dessous. Une adresse e-mail qui correspond à un utilisateur existant n’associe jamais cet utilisateur : le formulaire indique que l’adresse est déjà prise. Un utilisateur avec authentification à deux facteurs saisit quand même le code après s’être connecté avec un fournisseur. Un fournisseur GitHub demande à GitHub la portée `user:email`, et aussi `read:org` lorsque vous listez des organisations autorisées ; le serveur vérifie alors à chaque connexion que l’utilisateur appartient à l’une d’elles et refuse la connexion sinon. Enregistrez `https://<host>/auth/external/<provider ID>/callback` comme URL de rappel de votre application OAuth GitHub. Un fournisseur Discord demande à Discord les portées `identify` et `email`, et aussi `guilds` lorsque vous listez des serveurs autorisés ; le serveur vérifie alors à chaque connexion que l’utilisateur appartient à l’un d’eux. Enregistrez la même URL de rappel dans votre application Discord.
- Un utilisateur connecté associe un compte chez un fournisseur sous « Comptes associés » dans les paramètres, après confirmation par le mot de passe, une clé d’accès ou un autre compte associé, et uniquement pour les fournisseurs qui autorisent les associations. Le navigateur doit être connecté aux pages de connexion en tant que le même utilisateur ; sinon le serveur demande de se connecter d’abord. Un compte chez un fournisseur qu’un autre utilisateur a associé est refusé, et un utilisateur associe au plus un compte par fournisseur. Un utilisateur peut dissocier un compte sauf s’il s’agit du dernier moyen de se connecter (pas de mot de passe, pas de clé d’accès et pas d’autre compte associé).
- Un utilisateur sans mot de passe et sans clés d’accès confirme les opérations qui les demandent autrement (définir un mot de passe, ajouter une clé d’accès, supprimer ou déplacer le compte, associer un autre compte) en se connectant à nouveau chez un fournisseur associé. Un fournisseur OpenID Connect est invité à authentifier à nouveau l’utilisateur (`max_age=0` et `prompt=login`), et une authentification antérieure est refusée. La confirmation fonctionne une fois, pour cette opération seulement, dans un délai de 10 minutes ; se connecter avec un compte qui n’est pas celui associé compte comme une tentative erronée, soumise à la même limite qu’un mauvais mot de passe.
- À chaque inscription et chaque connexion, le serveur enregistre l’adresse IP du client (et, pour une connexion, le User-Agent). Les enregistrements sont supprimés automatiquement après 180 jours (les paramètres d’instance `session_origins_retention_days` et `registration_origins_retention_days`, d’au moins 90 jours). Les enregistrements d’un compte soumis à une conservation obligatoire sont gardés ([conservation et blocage des enregistrements](moderation.md#record-retention-and-holds)). Indiquez cette durée de conservation dans votre politique de confidentialité ([modèle de politique de confidentialité](privacy-policy-template.md)).

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## Quand un utilisateur peut utiliser l’application

Les utilisateurs qui attendent l’approbation, qui ont été rejetés ou qui n’ont pas confirmé leur adresse e-mail peuvent quand même se connecter sur la page de connexion. Il en va de même des utilisateurs qui ont désactivé leur propre compte, jusqu’à la date de sa suppression : ils y voient cette date, et peuvent réactiver ou supprimer le compte ([modération](moderation.md#accounts-that-their-owners-deactivated)). Après la connexion, ils voient ce qu’ils attendent (l’approbation d’un modérateur ou la confirmation de l’adresse e-mail), ou que leur inscription n’a pas été approuvée, et ne peuvent pas aller plus loin. Seuls les utilisateurs approuvés peuvent utiliser l’application (l’autorisation OAuth et l’émission de jetons de l’API cliente), et, sur les configurations qui exigent la confirmation de l’e-mail, seulement ceux qui ont confirmé leur adresse e-mail.

Un nouvel utilisateur qui peut utiliser l’application immédiatement passe de la page d’inscription directement dans l’application, à une étape d’accueil qui demande un nom d’affichage et un avatar. La fil d’accueil affiche ensuite une liste de premières étapes et des suggestions de personnes à suivre ([suggestions](instance-settings.md#suggestions-of-accounts)). « Continuer » sur la page qui confirme une adresse e-mail mène à la même étape d’accueil.

Un administrateur créé avec `my admin create` compte comme approuvé et confirmé.

<!-- source: email-confirmation 6c1b7759c2cf -->
## Confirmation de l’e-mail

Le paramètre `mail` ([référence de configuration](../reference/configuration.md#mail)) choisit la manière dont les e-mails sont envoyés. Les exemples SMTP, la configuration DNS et la manière de lire les échecs de distribution se trouvent dans [envoi d’e-mails](email.md).

- `mail = smtp` : le serveur envoie un e-mail de confirmation lors de l’inscription. Le lien est valable 24 heures et ne fonctionne qu’une fois. L’ouverture du lien affiche une page avec un bouton de confirmation, et l’adresse est confirmée lorsque l’utilisateur appuie dessus (afin que les scanners qui préchargent les liens des e-mails ne consomment pas le jeton). Après la connexion, l’utilisateur peut faire renvoyer l’e-mail jusqu’à 3 fois par heure. La connexion au serveur SMTP est toujours chiffrée avec STARTTLS, et le serveur vérifie le certificat avant de s’authentifier et d’envoyer. Il n’envoie jamais à un serveur qui ne propose pas STARTTLS.
- `mail = none` (par défaut) : le serveur n’envoie aucun e-mail et écrit `mail not sent (mail = none)` avec la catégorie du message dans le journal du serveur. Ce n’est qu’avec `profile = development` que la ligne contient aussi l’adresse et le corps, y compris le lien de confirmation. Comme les e-mails n’atteignent jamais l’utilisateur, le serveur demande l’approbation d’un modérateur au lieu de la confirmation par e-mail. Ainsi, une inscription en mode `open` commence aussi en attente d’approbation, et une adresse e-mail non confirmée ne bloque pas l’utilisation de l’application.

Le jeton de confirmation n’est enregistré nulle part ailleurs que dans cette ligne du journal de développement (les journaux d’accès n’enregistrent pas les chaînes de requête). La base de données ne stocke que le SHA-256 du jeton.

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## Inscription limitée aux fournisseurs de connexion

Tant que le paramètre d’instance `login_providers_only` est activé et qu’au moins un [fournisseur de connexion](login-providers.md#limit-sign-in-to-login-providers) est activé, la page d’inscription n’a pas de formulaire de mot de passe et ne propose que les fournisseurs qui autorisent les inscriptions ; un code d’invitation présent dans le lien est transmis au formulaire d’inscription du fournisseur. Une inscription avec un mot de passe est refusée, quel que soit le contenu du formulaire. Sans fournisseur qui autorise les inscriptions, l’inscription est considérée comme `closed`. Sans fournisseur activé, le paramètre ne s’applique pas.

<!-- source: password-reset-and-change bec2baef5ab7 -->
## Réinitialisation et changement du mot de passe

Tant que le paramètre d’instance `login_providers_only` s’applique, les liens de réinitialisation et la définition ou le changement d’un mot de passe sont refusés ([ce que fait la limite](login-providers.md#limit-sign-in-to-login-providers)).

- `mail = smtp` : lorsqu’un utilisateur saisit une adresse e-mail sous « Mot de passe oublié ? » sur la page de connexion, le serveur envoie un lien pour définir un nouveau mot de passe à l’utilisateur dont c’est l’adresse principale. La réponse de la page est la même, qu’un compte existe ou non. Le lien est valable 1 heure et ne fonctionne qu’une fois, et il cesse de fonctionner si le mot de passe change entre-temps par une autre voie. Les demandes sont limitées à 5 par 30 minutes par adresse IP client, et les e-mails à 3 par heure par adresse (une demande au-delà d’une limite n’envoie rien et obtient la même réponse). Lorsque le CAPTCHA est activé, ce formulaire est aussi vérifié.
- `mail = none` : la page indique qu’elle ne peut pas envoyer de lien et invite l’utilisateur à s’adresser à un administrateur. L’administrateur émet un lien sur la page de gestion du compte dans les écrans d’administration, ou avec `my admin account password-reset-link @alice`, et le transmet à l’utilisateur ([modération](moderation.md)).
- Un utilisateur connecté peut changer le mot de passe sous « Mot de passe » dans les paramètres en saisissant le mot de passe actuel (les saisies erronées sont limitées à 5 par heure et par utilisateur).
- Une réinitialisation ou un changement révoque les autres sessions et les jetons d’application de l’utilisateur (un changement depuis les paramètres ne conserve que l’application qui a fait le changement). Les clés d’accès et l’authentification à deux facteurs restent. Une connexion qui attend son code à deux facteurs, et une autorisation d’application pas encore terminée, cessent aussi de fonctionner : l’utilisateur les recommence. Avec `mail = smtp`, le serveur envoie aussi un e-mail à l’utilisateur pour indiquer que le mot de passe a changé.

<!-- source: invite-codes 5f98f9eb7d6f -->
## Codes d’invitation

Les invitations fonctionnent dans les modes `open`, `approval` et `invite`. Les invitations de l’équipe sont créées avec la CLI, les écrans de modération et l’API d’administration ci-dessous ; les membres peuvent aussi créer leurs propres invitations ([invitations des membres](#member-invites)). En mode `closed`, aucune invitation ne peut être créée et aucune ne fonctionne.

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` affiche le code (16 caractères) et le lien vers la page d’inscription avec le code renseigné (`https://<host>/auth/sign_up?invite=<code>`).
- Les codes sont comparés sans tenir compte de la casse, des espaces et des tirets.
- Même lorsque plusieurs personnes s’inscrivent en même temps, un code n’est jamais utilisé au-delà de sa limite (une contrainte d’unicité sur `invite_redemptions`).
- Un code révoqué, expiré ou épuisé donne la même erreur (une invitation inutilisable).
- Outre la CLI, vous pouvez créer, lister et révoquer des invitations dans les écrans de modération du client web (« Invitations », `/moderation/invites`) et via l’API cliente (`GET` / `POST /api/v1/admin/invites` et `POST /api/v1/admin/invites/{id}/revoke`). Les deux exigent la permission de rôle `manage_invites` (le rôle intégré `moderator` ne l’a pas, donc pour déléguer cette tâche, créez un rôle avec cette permission et attribuez-le). « Créer une invitation » dans la liste ouvre le formulaire (`/moderation/invites/new`), qui affiche le code et le lien d’inscription de la nouvelle invitation. La liste permet de copier le lien d’une invitation utilisable et de la révoquer.
- La liste affiche chaque invitation, celles de l’équipe et celles des membres, avec la personne qui l’a créée (`created_by`) et la manière (`creator_kind` : `staff` ou `member`). Les détenteurs de `manage_invites` peuvent révoquer n’importe laquelle.
- La page d’examen du compte dans les écrans de modération (`GET /api/v1/admin/accounts/{id}`, permission `manage_accounts`) affiche l’invitation avec laquelle le compte s’est inscrit et la personne qui l’a créée, c’est-à-dire celle qui a invité l’utilisateur.
- La création et la révocation d’une invitation sont consignées dans le journal d’audit (`invite_created`, `invite_revoked`), que ce soit depuis l’écran, l’API ou la CLI. Le nom de la cible correspond aux 4 premiers caractères du code (`ABCD…`) ; le code complet n’est pas copié dans le journal d’audit. Révoquer une invitation déjà révoquée ne change rien, et n’est donc pas consigné.

<!-- source: member-invites db3bff74ed69 -->
## Invitations des membres

Les membres peuvent créer leurs propres invitations, en plus de l’équipe. Qui peut le faire est défini par le paramètre d’instance `invite_creators` ([paramètres d’instance](instance-settings.md)) :

- `permitted` (par défaut) : uniquement les utilisateurs dont les rôles ont la permission `create_invites`. Seul le rôle intégré `admin` l’a par défaut. Vous pouvez l’ajouter à un rôle que vous attribuez à la main, ou au rôle d’une [formule de soutien](supporters.md), afin que les personnes qui soutiennent le serveur puissent inviter.
- `everyone` : chaque utilisateur local connecté.

Dans les deux cas, un utilisateur dont le compte est suspendu ou limité, ou dont l’inscription n’est pas approuvée, ne peut pas créer d’invitations, et personne ne le peut tant que le mode d’inscription est `closed`. Les détenteurs de `manage_invites` continuent de créer des invitations de l’équipe, auxquelles aucune des limites ci-dessous ne s’applique.

Un membre crée, liste, copie et révoque ses propres invitations dans les paramètres du client web (« Vos invitations », `/settings/invites`), ou via l’API cliente (`GET` / `POST /api/v1/invites` et `POST /api/v1/invites/{id}/revoke`). Les paramètres n’affichent la page que pour un utilisateur qui peut créer des invitations actuellement ; un utilisateur qui ne le peut plus l’ouvre quand même depuis son adresse pour copier et révoquer les invitations qu’il a créées. La liste indique, pour chaque invitation, si elle est utilisable, révoquée, expirée, épuisée, ou inutilisable parce que son créateur ne peut plus inviter.

Les limites, qui sont aussi des paramètres d’instance, ne s’appliquent qu’aux invitations des membres. Les administrateurs les modifient, ainsi que `invite_creators`, sous « Invitations des membres » sur la page « Inscription » des paramètres du serveur (`/admin/settings/registration`) :

| Paramètre | Signification | Par défaut | Plage |
| --- | --- | --- | --- |
| `member_invite_max_active` | Combien d’invitations utilisables (ni révoquées, ni expirées, ni épuisées) un membre peut détenir en même temps | `5` | 0-100 (0 : les membres ne peuvent pas créer d’invitations) |
| `member_invite_max_uses` | Combien de personnes peuvent s’inscrire avec une même invitation | `10` | 1-1 000 |
| `member_invite_max_lifetime_seconds` | Pendant combien de temps une invitation peut fonctionner, en secondes | `2592000` (30 jours) | 3 600 (1 heure) à 31 536 000 (365 jours) |

- Un membre choisit le nombre d’utilisations et la durée de vie dans les limites ; sans choix, une invitation permet 1 inscription et fonctionne pendant 7 jours (ou moins, lorsqu’une limite est plus basse). Une invitation sans limite d’utilisations ou de durée ne peut pas être créée.
- Abaisser une limite ne modifie pas les invitations existantes ; elle s’applique à la prochaine invitation créée.
- Chaque inscription avec l’invitation d’un membre vérifie que son créateur peut encore créer des invitations. Lorsque le créateur a perdu `create_invites` (par exemple, parce que sa formule a pris fin), que `invite_creators` est revenu à `permitted`, ou que le compte a été suspendu, limité ou supprimé, l’invitation ne fonctionne pas et la page d’inscription indique qu’elle n’est pas valide. L’invitation elle-même n’est pas modifiée, elle refonctionne donc, avant son expiration, lorsque le créateur peut de nouveau inviter.
- La création et la révocation par un membre sont consignées dans le journal d’audit sous les noms `invite_created` et `invite_revoked`, avec le membre comme acteur.

En mode `approval`, une invitation utilisable approuve immédiatement le nouvel utilisateur : la personne qui l’a invité tient lieu d’examen par les modérateurs. Avec `invite_creators = everyone`, chaque utilisateur peut donc faire entrer des gens sans examen, ce qui affaiblit le mode d’approbation. Si vous comptez sur l’examen, gardez `permitted` et ne donnez `create_invites` qu’à des personnes de confiance, ou gardez `member_invite_max_active` et `member_invite_max_uses` bas.

<!-- source: approving-and-rejecting e68676f67aab -->
## Approuver et rejeter

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- L’argument est le nom d’utilisateur d’un compte local (le `@` initial peut être omis).
- Seules les inscriptions en attente d’approbation peuvent être tranchées. Pour une inscription déjà tranchée, ou un utilisateur créé sans passer par la page d’inscription (`my admin create`), la commande ne change rien et se termine avec le code 1.
- Le code de sortie 2 signifie un argument erroné.

Les mêmes décisions peuvent être prises depuis les écrans d’administration (l’API cliente). Toutes exigent la permission `manage_accounts`.

| Opération | Ce qu’elle fait |
| --- | --- |
| `GET /api/v1/admin/registrations` | Les inscriptions en attente d’approbation (attente la plus longue en premier). Renvoie l’ID de l’inscription, la raison de l’inscription, l’adresse e-mail et si elle est confirmée, ainsi que le compte local |
| `POST /api/v1/admin/registrations/{id}/approve` | Approuve. Le corps `{"note": "..."}` est facultatif. La réponse affiche la note sous `decision_note`, et une réponse sur une inscription déjà tranchée ne contient pas d’adresse e-mail |
| `POST /api/v1/admin/registrations/{id}/reject` | Rejette. Le corps est le même que pour `approve` |

- C’est le même cas d’usage que la CLI, et il consigne la décision dans `registration_decisions` et dans le journal d’audit (`registration_approved`, `registration_rejected`). L’acteur d’une décision par la CLI est `cli` ; l’acteur d’une décision par l’API est le modérateur qui a tranché.
- Trancher une inscription déjà tranchée ne change rien et renvoie l’état actuel (si deux personnes tranchent en même temps, une seule décision est consignée). Un utilisateur créé sans passer par la page d’inscription donne 404.

<!-- source: sign-up-blocks b70eb18cd52f -->
## Blocages d’inscription

Les blocages rejettent les inscriptions depuis la page d’inscription, ou les basculent en approbation, selon le domaine de l’e-mail, l’adresse e-mail ou la plage d’IP.

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- Une inscription qui correspond à un blocage avec `-severity reject` (la valeur par défaut) est refusée, et une inscription qui correspond à `require_approval` commence en attente d’approbation quel que soit le mode d’inscription. Lorsque plusieurs correspondent, le plus strict s’applique. La page d’inscription n’affiche pas la raison d’un rejet.
- `email_hash` est comparé à l’adresse en minuscules, sans la partie de la partie locale à partir du premier `+`. Une plage d’IP est comparée à l’IP d’inscription (la même valeur que `registration_origins`) et ne s’applique jamais à une inscription dont l’IP est inconnue.
- Les blocages expirés et levés n’ont aucun effet. L’ajout et la levée sont consignés dans le journal d’audit (`signup_block_added`, `signup_block_lifted`).
- Lorsque vous bannissez un compte local avec `-block-email` (`block_email: true` dans l’API), le serveur crée aussi un blocage `email_hash` pour l’adresse e-mail de cet utilisateur.

Pour une liste de milliers d’entrées, comme les domaines des services d’e-mail jetables, ajoutez-les en masse avec `import` depuis un fichier (`-` pour l’entrée standard). Chaque valeur reçoit les mêmes type, gravité, commentaire et durée.

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- La liste comporte une valeur par ligne (la même forme que `<value>` de `add`), ou des lignes CSV dont la première colonne est la valeur. Les lignes vides et les lignes qui commencent par `#` sont ignorées. Un import accepte jusqu’à 10 000 entrées.
- La commande affiche combien ont été ajoutées et, pour chaque ligne non ajoutée, le numéro de ligne, la valeur et la raison. La raison est `invalid` (pas une valeur valide pour ce type), `duplicate` (la même forme normale qu’une ligne précédente de la même liste) ou `already_blocked` (un blocage du même type et de la même valeur, aussi strict ou plus strict, est en vigueur).
- Chaque blocage ajouté est consigné dans le journal d’audit (`signup_block_added`) comme avec `add`, et peut être levé un par un.
- L’import écrit par transactions de quelques centaines, donc s’il échoue en cours de route, ce qui a été ajouté auparavant reste. Importer à nouveau la même liste n’ajoute que le reste.
- Le logiciel ne fournit aucune liste et n’en récupère pas périodiquement depuis une URL. Vous récupérez et mettez à jour vous-même les listes publiques (comme disposable-email-domains sur GitHub) et importez à nouveau la liste mise à jour.

Les mêmes opérations sont disponibles depuis les écrans d’administration (l’API cliente). Toutes exigent la permission `manage_accounts`. Dans le client web, ouvrez Modération → Blocages d’inscription (`/moderation/signup-blocks`). La liste affiche la valeur, le type, l’état et la fin de chaque blocage, en vigueur ou tous ; « Ajouter un blocage » ouvre le formulaire (`/moderation/signup-blocks/new`), « Ajouter depuis une liste » importe une liste et affiche les lignes ignorées (`/moderation/signup-blocks/import`), et choisir une valeur ouvre la page du blocage avec son effet et son commentaire, où « Lever » y met fin.

| Opération | Ce qu’elle fait |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | Liste les blocages (les plus récents en premier). Filtrez avec `kind` et `active` |
| `POST /api/v1/admin/signup_blocks` | Ajoute un blocage. Le corps contient `kind`, `value`, `severity`, `comment` et `expires_in` (secondes, au plus dix ans) |
| `POST /api/v1/admin/signup_blocks/import` | Ajoute des blocages à partir d’une liste. Le corps contient `kind`, `text` (la liste), `severity`, `comment` et `expires_in`. La réponse contient `added`, le nombre ajouté, et `skipped`, les lignes non ajoutées (`line`, `value`, `reason`) |
| `GET /api/v1/admin/signup_blocks/{id}` | Renvoie un blocage |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | Lève un blocage. Un blocage déjà levé reste tel quel |
