<!-- source: moving-your-account 95d8d22a22bb -->
# Migrer votre compte

Vous pouvez migrer votre compte vers un autre compte, sur le même serveur ou sur un autre. Vos abonnés suivent le nouveau
compte, et l’ancien compte indique qu’il a migré. Cela fonctionne aussi dans l’autre sens, lorsque vous arrivez sur ce
serveur depuis un autre.

<!-- source: what-moves-and-what-stays a45d57421940 -->
## Ce qui déménage et ce qui reste

| Déménage | Reste sur l’ancien compte |
| --- | --- |
| Vos abonnés | Vos publications, les comptes que vous suivez, vos blocages, vos mises en sourdine et vos marque-pages |

![L’ancien compte à gauche et le nouveau compte à droite : un alias, le déménagement, vos abonnés qui passent au nouveau compte, et ce qui reste sur l’ancien compte. La liste ci-dessous dit la même chose.](images/account-move.svg)

De haut en bas du schéma :

1. **Ajouter un alias.** Sur le nouveau compte, ajoutez l’ancien compte sous « Alias ».
2. **Déménager le compte.** Sur l’ancien compte, choisissez « Déménager le compte » et nommez le nouveau compte.
3. **Ce qui déménage.** Vos abonnés suivent le nouveau compte. L’ancien compte indique « Ce compte a déménagé vers … » : vous pouvez toujours vous y connecter, lire et exporter vos données.
4. **Ce qui reste ici.** Vos publications, les comptes et les hashtags que vous suivez, ainsi que vos blocages, mises en sourdine et marque-pages restent sur l’ancien compte. Vos publications ne déménagent pas. Emportez le reste sous forme de fichiers CSV, avec « Exporter » sur l’ancien compte et « Importer » sur le nouveau ([emporter vos abonnements](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 8470ed8a1f13 -->
## Déménager depuis ce compte

1. Sur le nouveau compte, ajoutez ce compte comme alias : « Alias », sous « Compte et données » dans les paramètres, puis « Ajouter un alias ». « Déménager le compte » affiche l’identifiant de ce compte à y saisir.
2. Sur ce compte, ouvrez « Déménager le compte » sous « Compte et données » dans les paramètres, qui indique ce qui déménage et ce qui reste. Sous « Compte vers lequel vous déménagez », saisissez l’identifiant du nouveau compte (par exemple `@name@example.com`) ou l’adresse de son profil, puis choisissez « Déménager le compte ».
3. Confirmez que c’est bien vous avec votre mot de passe, une clé d’accès ou un fournisseur lié. Un déménagement que le serveur refuse en indique la raison.

Après le déménagement, l’ancien compte :

- Indique où il a déménagé, sur cette page et sur son profil. Vous pouvez toujours vous y connecter, lire, exporter vos données et supprimer le compte, mais pas publier, booster, réagir, suivre ni voter.
- Ne peut pas déménager de nouveau pendant 30 jours.

« Annuler le déménagement », sur la même page, rétablit l’ancien compte : il peut de nouveau publier et suivre, et les autres serveurs en sont
informés. Les abonnés qui ont déjà déménagé restent avec le nouveau compte.

<!-- source: deactivating-or-deleting-this-account 599007a2d409 -->
## Désactiver ou supprimer ce compte

Sous « Compte et données » dans les paramètres, « Désactiver le compte » masque le compte et vous déconnecte partout : reconnectez-vous
avant la date indiquée par la page pour le rétablir, faute de quoi il est supprimé à cette date. « Supprimer le compte » le supprime aussitôt et
définitivement. « Exportez » d’abord vos données.

<!-- source: moving-to-this-account 4b769e76357c -->
## Déménager vers ce compte

Ajoutez le compte depuis lequel vous déménagez comme alias sous « Alias » dans les paramètres de ce compte (« Ajouter un alias »). Un alias seul
ne change rien ; il permet seulement à l’autre compte de déménager ses abonnés vers vous. Vous pouvez avoir 5 alias, et
« Retirer » en supprime un.

<!-- source: taking-your-follows-with-you 09ca5e285436 -->
## Emporter vos abonnements

« Exporter » et « Importer », sous « Compte et données » dans les paramètres, exportent et importent des fichiers CSV au format utilisé par Mastodon.
Vous pouvez exporter « Abonnements », « Hashtags suivis », « Comptes bloqués », « Comptes en sourdine », « Serveurs en sourdine » et « Marque-pages », et
les importer sur le nouveau compte. Mastodon n’a pas de fichier de hashtags suivis : seul un serveur qui exécute le même logiciel que
celui-ci importe ce fichier. « Fusionner » conserve ce que vous avez et ajoute ce que liste le fichier ; « Remplacer » rend la liste identique à ce que
liste le fichier. « Aperçu » montre ce qu’un fichier ferait avant que vous ne choisissiez « Importer » ; l’import s’exécute ensuite en arrière-plan.

« Demander une archive », sous « Exporter », prépare une archive ZIP de vos publications, de votre profil, de vos filtres de mots-clés, de vos notes privées, ainsi que de vos
abonnements, hashtags suivis, blocages, mises en sourdine, serveurs en sourdine et marque-pages, à télécharger à cet endroit une fois prête.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Quand un compte que vous suivez migre

Vous en êtes notifié (« … a migré vers un autre compte »). Lorsque vous pouvez suivre le nouveau compte, vous le suivez aussitôt, ou votre
demande attend son approbation. Vous ne le suivez pas si vous l’avez bloqué, s’il vous a bloqué ou si vous avez mis son serveur en sourdine ;
la notification indique lequel de ces cas s’applique.
