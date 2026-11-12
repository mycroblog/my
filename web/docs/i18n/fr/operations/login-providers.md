<!-- source: external-login-providers 48a582ab87ec -->
# Fournisseurs de connexion externes

Un fournisseur de connexion externe permet aux utilisateurs de se connecter avec un compte qu’ils possèdent ailleurs : un fournisseur OpenID Connect
(Google Workspace, Microsoft Entra ID, Keycloak, etc.), GitHub, Discord ou un serveur Mastodon.
Vous configurez les fournisseurs avec la commande `my login-provider` sur un hôte qui dispose de la configuration du serveur, ou via l’API
d’administration (`/api/v1/admin/login_providers`) avec la permission `manage_login_providers`, que seul le rôle intégré
`admin` possède. La commande fonctionne avant qu’un administrateur puisse se connecter ; vous pouvez donc l’utiliser pour la première configuration.
Dans le client web, les administrateurs disposant de cette permission utilisent « Fournisseurs de connexion » dans le menu d’administration. La liste
affiche le type de chaque fournisseur, son état et si l’inscription est autorisée ; « Ajouter un fournisseur » ouvre le formulaire qui en crée un.
Choisir un fournisseur ouvre sa page, qui affiche son URL de rappel avec un bouton qui la copie et le nombre d’utilisateurs
associés, et où vous modifiez ses paramètres, lui donnez un nouveau secret client, l’activez, le désactivez et le supprimez.

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## Ce que fait la connexion externe

- Un fournisseur ne fait que prouver qui est quelqu’un chez ce fournisseur. Un compte n’est jamais associé à un utilisateur par adresse e-mail : un utilisateur
  associe un compte sous « Comptes associés » dans les paramètres, ou s’inscrit avec lui comme nouvel utilisateur. Une adresse e-mail qui correspond
  à un utilisateur existant n’associe jamais cet utilisateur.
- Un utilisateur associé saisit toujours le code à deux facteurs (TOTP) après s’être connecté avec un fournisseur, et un utilisateur dont le compte n’est
  pas actif ne peut pas se connecter.
- Les nouveaux utilisateurs ne peuvent s’inscrire avec un fournisseur que si vous l’autorisez pour ce fournisseur (`-allow-signup`) et que le
  [mode d’inscription](registration.md) leur permet de s’inscrire ; le code d’invitation, le CAPTCHA, les blocages d’inscription et la limite de réseau
  s’appliquent comme sur le formulaire par mot de passe. Une personne qui s’inscrit ainsi n’a pas de mot de passe et se connecte uniquement avec le fournisseur.
  Le [schéma des parcours d’inscription](registration.md#from-sign-up-to-the-app) montre côte à côte les deux façons de s’inscrire.

<!-- source: kinds c9123566b5c5 -->
## Types

| Type | `-issuer` | Conditions |
| --- | --- | --- |
| `oidc` | L’URL de l’émetteur (`https`), exactement telle que les jetons d’ID du fournisseur la nomment, comme `https://accounts.google.com` | `-claim <claim>=<value>` : uniquement les utilisateurs dont le jeton d’ID contient la revendication avec l’une des valeurs données pour elle |
| `github` | Aucun : le serveur connaît les points de terminaison de github.com | `-org <organization>` : uniquement les membres de l’une des organisations (ajoutez `-scope read:org`) |
| `discord` | Aucun : le serveur connaît les points de terminaison de discord.com | `-guild <server ID>` : uniquement les membres de l’un des serveurs Discord (guildes) ; sans cette option, tous les utilisateurs de Discord sont admis |
| `mastodon` | Le domaine web du serveur, comme `mastodon.example` (Mastodon 4.3 ou ultérieur) | Aucune |

Pour Google Workspace, limitez les utilisateurs à votre organisation avec `-claim hd=<your domain>`. Ne vous fiez pas au domaine de
l’adresse e-mail.

<!-- source: examples 0d19ab463197 -->
## Exemples

<!-- source: google b99294b8539d -->
### Google

Dans la console Google Cloud, créez un client OAuth de type « Web application ». Pour le limiter à votre organisation, définissez le
type d’utilisateur de l’écran de consentement OAuth sur « Internal » (disponible lorsque le projet appartient à une organisation Google Workspace),
et ajoutez aussi la condition `hd` : le type d’utilisateur restreint qui Google laisse entrer, et la condition fait vérifier à ce
serveur la revendication `hd` de chaque jeton d’ID.

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak et autres fournisseurs OpenID Connect

L’émetteur est l’URL du realm, comme `https://sso.example.com/realms/main`, exactement telle que les jetons d’ID du fournisseur la nomment.
Créez un client confidentiel avec le flux standard, et les portées `openid`, `email` et `profile`. Si le fournisseur
se trouve sur un réseau privé, listez-le dans `login_provider_allowed_destinations` ([ci-dessous](#providers-on-an-internal-network)).
Utilisez `-claim` pour exiger une revendication, comme une revendication de groupe, avec l’une des valeurs données.

<!-- source: discord 00e9659d1eae -->
### Discord

Créez une application dans le [Discord Developer Portal](https://discord.com/developers/applications), ouvrez « OAuth2 », et
donnez au serveur l’ID client et le secret client. Le serveur demande les portées `identify` et `email`, et aussi
`guilds` lorsque vous listez des serveurs avec `-guild`. Un utilisateur est identifié par son ID Discord numérique ; le nom d’utilisateur et le nom d’affichage
ne font que suggérer le nom d’utilisateur sur le formulaire d’inscription. L’adresse e-mail n’est considérée comme vérifiée que lorsque Discord indique qu’elle est
vérifiée ; sinon l’utilisateur la confirme par e-mail comme n’importe quelle autre adresse.

Avec `-guild`, le serveur vérifie à chaque connexion, inscription et association que l’utilisateur appartient à l’un des serveurs listés
et refuse l’utilisateur dans le cas contraire. Sans cette option, tout utilisateur de Discord peut se connecter. Un ID de guilde est un nombre d’au plus 20 chiffres : dans
Discord, activez « Developer Mode » (User Settings, Advanced), puis faites un clic droit sur l’icône du serveur et choisissez « Copy Server
ID ».

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

Ajoutez l’URL de rappel sous « OAuth2 », « Redirects » dans l’application, une fois que `add` l’a affichée.

<!-- source: github a7181ea6edd6 -->
### GitHub

Créez une OAuth App (Settings, Developer settings, OAuth Apps) et donnez-lui l’ID client et le secret. Le serveur demande
la portée `user:email`, et aussi `read:org` lorsque vous listez des organisations avec `-org`. Avec `-org`, le serveur vérifie
à chaque connexion que l’utilisateur appartient à l’une des organisations ; les membres d’autres organisations sont donc refusés.

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## Configurer un fournisseur

L’URI de redirection dont le fournisseur a besoin est l’URL de rappel du fournisseur, `https://<host>/auth/external/<id>/callback`,
qui contient l’ID que le serveur donne au fournisseur. Donc :

1. Enregistrez un client OAuth chez le fournisseur (une « OAuth App » sur GitHub) avec n’importe quelle URI de redirection pour l’instant, et notez l’ID
   client et le secret client qu’il vous donne.
2. Exécutez `my login-provider create` avec eux. La commande affiche l’URL de rappel.
3. Remplacez l’URI de redirection du client chez le fournisseur par cette URL de rappel.

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

Un fournisseur `mastodon` est un serveur Mastodon que vous nommez ; les utilisateurs d’autres serveurs ne peuvent pas se connecter avec lui. Le serveur
doit être en Mastodon 4.3 ou ultérieur : `add` lit ses métadonnées de serveur d’autorisation
(`/.well-known/oauth-authorization-server`) et refuse un serveur qui n’en a pas. Sans `-client-id`, `add` enregistre un client
sur le serveur lui-même, avec l’URL de rappel et la portée `profile`, et stocke l’ID client et le secret qu’il obtient ; il n’y a donc
rien à faire sur le serveur :

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

Si le serveur ne laisse pas d’autres enregistrer des clients, `add` échoue. Créez plutôt une application avec la portée `profile` et l’URL de rappel
dans les paramètres de développement du serveur, et donnez son ID client et son secret comme pour les autres types
(l’URL de rappel contient l’ID du fournisseur ; suivez donc les trois étapes ci-dessus).

Les utilisateurs se connectent avec la seule portée `profile`, qui lit leur compte et rien d’autre. Mastodon ne donne pas d’adresse e-mail,
de sorte qu’une personne qui s’inscrit avec lui confirme l’adresse par e-mail. Le formulaire d’inscription suggère le nom d’utilisateur du compte.
Tant que le domaine du serveur, ou un domaine parent, fait l’objet d’une action de domaine ([actions de domaine](moderation.md#domain-actions)), son bouton disparaît et
personne ne se connecte, ne s’inscrit ni n’associe un compte avec lui. Le serveur d’un fournisseur ne change pas ; configurez un nouveau fournisseur
pour un autre serveur.

Le secret client est stocké chiffré avec la clé de chiffrement de clés ([gestion des clés](key-management.md)) et n’est plus jamais affiché,
ni par la commande ni par l’API d’administration.

Un nouveau fournisseur est activé. Par défaut, les utilisateurs peuvent y associer leur compte existant (`-allow-link`), mais personne ne peut s’inscrire
avec lui : ajoutez `-allow-signup` pour permettre aux nouveaux utilisateurs de s’inscrire avec lui, ce que le mode d’inscription limite toujours
([inscription](registration.md)).

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## Modifier, renouveler, désactiver et supprimer

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` ne modifie que les paramètres qu’il nomme. `-scope`, `-claim` et `-org` remplacent toute la liste. Le type d’un fournisseur
  ne change pas ; configurez plutôt un nouveau fournisseur.
- `rotate-secret` remplace le secret client par celui que vous saisissez. Renouvelez-le d’abord chez le fournisseur, puis ici.
- `disable` conserve le fournisseur et les comptes qui lui sont associés, mais personne ne se connecte, ne s’inscrit ni n’associe de compte avec lui, et la
  page de connexion ne l’affiche pas, jusqu’à ce que vous exécutiez `enable`. `disable` et `delete` affichent combien d’utilisateurs associés ne peuvent plus
  se connecter avec le fournisseur, et ce que fait maintenant la [limitation aux fournisseurs de connexion](#limit-sign-in-to-login-providers) tant que son
  paramètre est activé.
- `delete` supprime le fournisseur définitivement.

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### Utilisateurs qui se connectent uniquement par un fournisseur

Désactiver ou supprimer un fournisseur ne touche pas aux utilisateurs. Un utilisateur qui a un mot de passe ou une clé d’accès, ou qui a un autre compte
associé, se connecte de cette façon, sauf si [la connexion est limitée aux fournisseurs de connexion](#limit-sign-in-to-login-providers) et qu’un autre
fournisseur reste activé. Un utilisateur qui n’a rien de tout cela ne peut plus du tout se connecter : `disable` et `delete` affichent
combien d’utilisateurs associés perdent le fournisseur, et le client web affiche ce nombre dans la confirmation. `enable` rétablit les
utilisateurs d’un fournisseur désactivé ; un fournisseur supprimé ne peut pas revenir.

Pour récupérer un tel utilisateur, émettez un lien de réinitialisation de mot de passe sur la page de gestion du compte dans les écrans d’administration, ou avec
`my admin account password-reset-link @alice`, et donnez-le à l’utilisateur ([inscription](registration.md#password-reset-and-change)).
L’utilisateur se connecte ensuite avec le nouveau mot de passe.

Chaque modification est enregistrée dans le journal d’audit sous le nom d’affichage du fournisseur, en tant que CLI ou en tant qu’administrateur qui l’a effectuée
par l’API d’administration.

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## Limiter la connexion aux fournisseurs de connexion

Le paramètre d’instance `login_providers_only` (désactivé par défaut) fait des fournisseurs le seul moyen de se connecter et de s’inscrire,
tant qu’au moins un fournisseur est activé. Activez-le sur la page « Inscription » de « Paramètres du serveur » dans le client web, ou avec la CLI :

```sh
my instance set login_providers_only true
```

La confirmation dans le client web et la sortie de la commande indiquent combien d’utilisateurs locaux actifs n’ont aucune association avec un
fournisseur activé. Le client web indique aussi lorsque cela vous inclut. Il avertit et ne refuse pas : c’est vous qui décidez.

Tant que la limitation s’applique :

- Les mots de passe et les clés d’accès ne connectent personne, et la page de connexion n’affiche que les fournisseurs. Une connexion commencée avec un
  mot de passe ou une clé d’accès et qui attend son code à deux facteurs est refusée aussi.
- Personne ne s’inscrit avec un mot de passe. La page d’inscription affiche les fournisseurs qui autorisent les inscriptions (`-allow-signup`), selon
  le [mode d’inscription](registration.md) comme d’habitude. Sans un tel fournisseur, l’inscription apparaît comme `closed`, y compris pour les clients
  et dans NodeInfo.
- Personne ne demande ni n’utilise un lien de réinitialisation de mot de passe. Les liens restent valides jusqu’à leur expiration, et fonctionnent si la limitation prend fin à temps.
- Les utilisateurs ne peuvent ni définir ni modifier un mot de passe, ni ajouter une clé d’accès, car aucun des deux ne permettrait de se connecter. Ils peuvent toujours les supprimer.
- `my admin account password-reset-link` et les écrans d’administration émettent toujours un lien, et indiquent qu’il ne fonctionne pas avant la fin de la
  limitation. `my admin create` crée toujours un administrateur, et avertit que le mot de passe ne permet pas de se connecter.
- Un utilisateur ne peut pas dissocier son dernier compte chez un fournisseur activé.

Ce qui continue de fonctionner :

- Les sessions, les jetons d’accès des applications et les jetons d’actualisation restent valides. La limitation n’en met aucun fin.
- Un mot de passe ou une clé d’accès confirme toujours des opérations dans une session. C’est ainsi qu’un utilisateur existant passe à un fournisseur : une fois
  connecté, il ouvre « Comptes associés » dans les paramètres, confirme avec le mot de passe ou la clé d’accès, et se connecte chez le fournisseur.
- La connexion, l’inscription et l’association chez un fournisseur, ainsi que le code à deux facteurs après la connexion par un fournisseur.

Chaque refus est formulé de la même façon quels que soient l’adresse e-mail, le mot de passe ou le jeton envoyés ; il ne dit donc rien d’aucun compte.

**Les utilisateurs sans association ne peuvent plus se connecter une fois leurs sessions terminées, tant que vous n’agissez pas.** Un compte n’est jamais associé par adresse
e-mail ; la connexion d’un fournisseur ne les trouve donc pas non plus. Demandez-leur d’associer un compte pendant qu’ils sont connectés. Un utilisateur qui
a perdu sa session a besoin que vous mettiez fin à la limitation pendant un moment, ou que vous le teniez à l’écart. Cela comprend un utilisateur sous `disable_login`,
`suspend` ou `ban`, qui n’a sans association aucune page de connexion depuis laquelle [contester](moderation.md).
L’association exige aussi que le navigateur soit connecté aux pages de connexion : un utilisateur dont l’application est toujours connectée, mais dont la session
de navigateur est terminée, ne peut pas s’associer tant que la limitation s’applique, et doit attendre que vous agissiez. **Annoncez le changement à vos utilisateurs avant
d’activer la limitation,** afin qu’ils associent un compte tant que leurs sessions durent.

<!-- source: end-the-limit 77f5c71d013b -->
### Mettre fin à la limitation

La limitation suit le paramètre et les fournisseurs d’une requête à l’autre, sans redémarrage.

- Un administrateur qui est verrouillé dehors exécute ceci sur un hôte qui dispose de la configuration du serveur :

  ```sh
  my instance set login_providers_only false
  ```

- Lorsqu’aucun fournisseur n’est activé, la limitation ne s’applique pas : désactiver ou supprimer le dernier fournisseur rouvre la connexion par mot de passe et par clé d’accès,
  quoi que dise le paramètre. Les utilisateurs qui n’ont ni mot de passe ni clé d’accès ont toujours besoin d’un
  [lien de réinitialisation](registration.md#password-reset-and-change).
- Avec le paramètre activé et aucun fournisseur activé, configurer ou activer un fournisseur déclenche immédiatement la limitation ; un nouveau fournisseur est
  activé dès qu’il est configuré. Le client web avertit d’abord, en indiquant combien d’utilisateurs actifs n’ont aucune association avec ce fournisseur et
  si cela vous inclut. Il n’avertit que pour l’action qui déclencherait la limitation. `my login-provider create`, `enable`,
  `disable` et `delete` affichent ce que fait maintenant la limitation tant que le paramètre est activé.
- Lorsqu’une panne chez le fournisseur empêche tout le monde d’entrer, utilisez la même commande.

<!-- source: re-authentication 8ec1742f8811 -->
## Réauthentification

Les opérations qui demandent le mot de passe ou une clé d’accès (définir un mot de passe, ajouter une clé d’accès, supprimer ou déplacer le compte,
associer un autre compte) demandent à un utilisateur qui n’a ni l’un ni l’autre de se reconnecter chez un fournisseur associé ([inscription](registration.md)).
Un fournisseur OpenID Connect est invité à authentifier de nouveau l’utilisateur (`max_age=0` et `prompt=login`) et le serveur
refuse une authentification antérieure ; un serveur Mastodon est interrogé avec `force_login=true`. GitHub et Discord n’ont aucun moyen de
demander une connexion récente ; un utilisateur qui y est toujours connecté est donc confirmé sans rien saisir. Le serveur vérifie toujours
que le compte est celui qui est associé, mais c’est une preuve plus faible que la personne est devant le clavier. Préférez un fournisseur OpenID
Connect, ou faites conserver un mot de passe ou une clé d’accès aux utilisateurs, lorsque cela compte.

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## Fournisseurs sur un réseau interne

Le serveur joint les fournisseurs par sa protection HTTP sortante, qui refuse les adresses de boucle locale et privées. Pour utiliser un
fournisseur sur votre propre réseau, comme un Keycloak à côté du serveur, listez son hôte ou sa plage d’adresses dans
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations).
