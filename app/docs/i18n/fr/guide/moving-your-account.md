<!-- source: moving-your-account 95d8d22a22bb -->
# Migrer votre compte

Vous pouvez migrer votre compte vers un autre compte, sur le même serveur ou sur un autre. Vos abonnés suivent le nouveau
compte, et l’ancien compte indique qu’il a migré. Cela fonctionne aussi dans l’autre sens, lorsque vous arrivez sur ce
serveur depuis un autre.

<!-- source: what-moves-and-what-stays 4ea7b64d67bd -->
## Ce qui migre et ce qui reste

| Migre | Reste sur l’ancien compte |
| --- | --- |
| Vos abonnés | Vos publications, les comptes que vous suivez, vos blocages, vos mises en sourdine et vos marque-pages |

![L’ancien compte à gauche et le nouveau compte à droite : un alias, la migration, vos abonnés qui passent au nouveau compte, et ce qui reste sur l’ancien compte. La liste ci-dessous dit la même chose.](images/account-move.svg)

De haut en bas du schéma :

1. **Ajoutez un alias.** Sur le nouveau compte, ajoutez l’ancien compte sous « Alias ».
2. **Migrer le compte.** Sur l’ancien compte, choisissez « Migrer le compte » et indiquez le nouveau compte.
3. **Ce qui migre.** Vos abonnés suivent le nouveau compte. L’ancien compte affiche « Votre compte a migré » : vous pouvez toujours vous y connecter, y lire et y exporter vos données.
4. **Ce qui reste ici.** Vos publications, les comptes que vous suivez, ainsi que vos blocages, mises en sourdine et marque-pages restent sur l’ancien compte. Vos publications ne migrent pas. Emportez le reste vous-même : exportez vos abonnements, blocages, mises en sourdine et marque-pages sous forme de fichiers CSV avec « Import et export » sur l’ancien compte, puis importez-les sur le nouveau ([emporter vos abonnements](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 7836944c7996 -->
## Migrer depuis ce compte

1. Sur le nouveau compte, ajoutez ce compte comme alias : « Alias », sous « Compte et données » dans les paramètres, avec « Ajouter un alias ». La page de migration affiche l’adresse de ce compte, que vous saisissez à cet endroit.
2. Sur ce compte, ouvrez « Migrer le compte » sous « Compte et données » dans les paramètres. Saisissez l’identifiant du nouveau compte (un identifiant tel que `@name@example.com`, ou l’adresse du compte) et choisissez « Continuer ».
3. Le client indique ce qui migre et ce qui reste. Saisissez votre mot de passe pour confirmer (ou, si vous n’en avez pas, confirmez avec une clé d’accès, ou en vous reconnectant auprès d’un fournisseur lié) et choisissez « Migrer le compte ».

Après la migration, l’ancien compte :

- Affiche « Votre compte a migré ». Vous pouvez toujours vous connecter, lire, exporter vos données et supprimer le compte, mais vous ne pouvez plus publier, booster, réagir, suivre ni voter.
- Est présenté sur son profil comme ayant migré vers le nouveau compte.
- Ne peut plus migrer pendant 30 jours.

« Annuler la migration » rétablit l’ancien compte : il peut de nouveau publier et suivre, et les autres serveurs en sont
informés. Les abonnés qui ont déjà migré restent avec le nouveau compte.

<!-- source: moving-to-this-account adb43ba4eef3 -->
## Migrer vers ce compte

Ajoutez le compte depuis lequel vous migrez comme alias sous « Alias » dans les paramètres de ce compte (« Ajouter un alias »). Un alias seul
ne change rien : il permet seulement à l’autre compte de migrer ses abonnés vers vous. Vous ne pouvez avoir que quelques alias, et
« Supprimer » en supprime un.

<!-- source: taking-your-follows-with-you ab2475388452 -->
## Emporter vos abonnements

« Import et export », sous « Compte et données » dans les paramètres, exporte et importe des fichiers CSV au format utilisé par Mastodon.
Vous pouvez exporter « Abonnements », « Comptes bloqués », « Comptes en sourdine », « Domaines en sourdine » et « Marque-pages », puis les importer sur le
nouveau compte. « Fusionner » conserve ce que vous avez et ajoute ce que le fichier énumère ; « Remplacer » fait en sorte que la liste soit exactement ce que le fichier
énumère. Les comptes et les publications d’autres serveurs sont recherchés en arrière-plan, ce qui peut prendre un certain temps.

« Archive et suppression » prépare une archive ZIP de vos publications, de votre profil, ainsi que de vos abonnements, blocages, mises en sourdine et marque-pages.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Quand un compte que vous suivez migre

Vous en êtes notifié (« … a migré vers un autre compte »). Lorsque vous pouvez suivre le nouveau compte, vous le suivez aussitôt, ou votre
demande attend son approbation. Vous ne le suivez pas si vous l’avez bloqué, s’il vous a bloqué ou si vous avez mis son serveur en sourdine ;
la notification indique lequel de ces cas s’applique.
