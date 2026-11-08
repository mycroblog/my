<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# S’inscrire et se connecter

Comment créer un compte sur un serveur mycroblog, un serveur qui fait tourner `my`, et s’y connecter. Chaque serveur est géré par ses propres opérateurs ; ce qu’un serveur
propose (qui peut s’inscrire, quels fils les visiteurs peuvent lire, si le chat vocal est activé) dépend donc d’eux. La page
« À propos » du serveur et les documents qu’elle référence lors de l’inscription vous indiquent ses règles ; ce guide décrit uniquement le comportement de `my`.

<!-- source: creating-an-account 965073ac4cf5 -->
## Créer un compte

Choisissez « S’inscrire » dans la navigation (ou « Créer un compte » sur la page de connexion). Le formulaire demande :

- **Nom d’utilisateur** : des lettres (a-z), des chiffres et des tirets bas. Votre identifiant est `@username@this-server`, et vous ne pouvez pas modifier le nom d’utilisateur par la suite.
- **Adresse e-mail** : vous vous connectez avec elle.
- **Mot de passe** : le formulaire indique la longueur autorisée sous le champ.
- **Code d’invitation**, lorsque le serveur en demande un ou en accepte un.
- **Pourquoi souhaitez-vous nous rejoindre ? (facultatif)**, lorsque le serveur examine les nouveaux comptes.
- Une case pour confirmer que vous avez lu les conditions et les politiques du serveur, lorsque le serveur en publie.

Ce que vous voyez dépend de la façon dont le serveur accepte les nouveaux comptes :

| Le serveur | Ce que vous faites |
| --- | --- |
| Est ouvert | Inscrivez-vous. Un code d’invitation est facultatif. |
| Examine les nouveaux comptes | Inscrivez-vous et indiquez pourquoi vous souhaitez rejoindre le serveur. Un modérateur approuve le compte avant que vous puissiez l’utiliser. Un code d’invitation vous permet d’utiliser le compte immédiatement. |
| Accepte uniquement les invitations | Saisissez le code d’invitation qui vous a été donné, ou ouvrez le lien d’invitation, qui le renseigne. |
| Est fermé | La page indique que le serveur n’accepte pas de nouveaux comptes. |
| Inscrit uniquement par des fournisseurs de connexion | Vous vous inscrivez avec un fournisseur proposé par le serveur (par exemple « Se connecter avec GitHub ») au lieu d’un mot de passe. |

Si le serveur envoie des e-mails, il envoie un lien pour confirmer votre adresse e-mail. Ouvrez-le et choisissez « Confirmer l’adresse e-mail ».
Tant que le compte n’est pas approuvé et que l’adresse n’est pas confirmée, vous pouvez vous connecter, mais vous ne voyez que ce que vous attendez
(« Votre compte n’est pas encore prêt »).

Après l’inscription, une page d’accueil vous permet de choisir un avatar et un nom d’affichage (« Continuer » les enregistre, « Plus tard » passe cette étape).
Votre fil d’accueil affiche alors une liste « Pour commencer » : « Configurer votre profil », « Suivre quelqu’un » et « Rédiger votre première
publication ». « Masquer ces étapes » la ferme sur ce navigateur. « Comptes suggérés » liste des personnes que vous pouvez suivre, sauf sur un serveur
d’une seule personne, où « Trouver des gens » ouvre la recherche à la place.

<!-- source: signing-in fcfbfa76f1d0 -->
## Se connecter

Choisissez « Se connecter » dans la navigation. Le client web vous envoie vers la page de connexion du serveur et vous ramène une fois connecté.
Sur cette page, vous pouvez :

- Saisir votre adresse e-mail et votre mot de passe.
- Choisir « Se connecter avec une clé d’accès », une fois que vous avez ajouté une clé d’accès sous « Clés d’accès » dans les paramètres.
- Choisir un bouton sous « Ou connectez-vous avec », lorsque le serveur propose des fournisseurs de connexion et que vous avez associé un compte de ce fournisseur sous « Comptes associés » dans les paramètres.
- Choisir « Mot de passe oublié ? » pour recevoir un lien qui définit un nouveau mot de passe. Définir un nouveau mot de passe vous déconnecte partout. Un serveur qui n’envoie pas d’e-mails ne peut pas envoyer le lien : demandez à ses administrateurs.

Si vous avez activé l’authentification à deux facteurs, la page demande ensuite le code à six chiffres de votre application d’authentification, ou l’un
de vos codes de récupération. Après trop de tentatives échouées, la page vous fait attendre quelques minutes et indique combien de temps.

<!-- source: an-account-that-cannot-be-used-yet 8e6bc1791f14 -->
### Un compte qui ne peut pas encore être utilisé

Après la connexion, vous pouvez voir une page à la place de l’application :

- Votre inscription attend l’approbation d’un modérateur, ou n’a pas été approuvée.
- Vous devez confirmer votre adresse e-mail. « Renvoyer le lien » envoie un nouveau lien.
- Votre compte est restreint par un modérateur. La page liste les mesures en vigueur et vous permet de contester une fois (« Contester cette mesure »). Les mêmes mesures sont listées sous « État du compte » dans les paramètres.
- Vous avez désactivé le compte. « Réactiver votre compte » le rétablit jusqu’à la date de sa suppression ; « Supprimer le compte » le supprime immédiatement.

<!-- source: signing-out fb81956ae999 -->
## Se déconnecter

Choisissez « Se déconnecter » dans la section « Compte » de l’application. La page « Navigateurs connectés » des paramètres liste les navigateurs
connectés à votre compte et les déconnecte.

<!-- source: settings 35cf43b05962 -->
## Paramètres

Les paramètres sont regroupés en « Profil et communauté », « Affichage et notifications », « Relations », « Sécurité », « Applications »
et « Compte et données ». Sous « Sécurité », vous gérez votre mot de passe, l’authentification à deux facteurs, les clés d’accès, les comptes associés
et les navigateurs connectés.
