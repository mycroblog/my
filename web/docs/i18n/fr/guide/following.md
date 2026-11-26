<!-- source: following-people-and-reading-timelines 32771133806e -->
# Suivre des personnes et lire les fils

<!-- source: finding-people 56dd27ea715d -->
## Trouver des personnes

« Rechercher » trouve des personnes par leur nom ou leur identifiant, ainsi que des hashtags ; pour un compte d’un autre serveur, saisissez `@username@server` ou l’adresse de son profil. Lorsque vous êtes connecté, le serveur recherche de lui-même, sur son propre serveur, un compte qu’il ne connaît pas encore ; les visiteurs non connectés ne trouvent que les comptes que le serveur connaît déjà. Certains serveurs ne recherchent pas du tout les comptes d’autres serveurs, et seuls les comptes qu’ils connaissent déjà sont alors trouvés. L’onglet « Publications » recherche dans le texte des publications lorsque le serveur le propose ; il trouve les publications publiques, ainsi que les publications pour les voisins lorsque vous êtes connecté, et uniquement celles des auteurs qui l’autorisent (les auteurs peuvent s’en exclure dans les paramètres de leur profil).

Sur le profil d’un compte d’un autre serveur, la page n’affiche que les publications reçues par ce serveur et renvoie vers le profil d’origine (« Voir sur … »).

<!-- source: following 4910790105e4 -->
## Suivre

Un profil comporte le bouton « Suivre ». Selon le compte, il affiche autre chose :

| Bouton | Signification |
| --- | --- |
| Suivre | Vous suivez le compte immédiatement. |
| Demander à suivre | Le compte approuve ses abonnés (« Abonnés soumis à approbation »). Le bouton indique alors « Demande envoyée », et « Retirer la demande » l’annule. |
| Suivre en retour | Le compte vous suit. |
| Abonné | Vous suivez le compte. Le choisir demande « Ne plus suivre … ? ». |

Lorsque vous suivez un compte d’un autre serveur, le profil indique « En attente de l’acceptation de votre demande d’abonnement par … » tant que ce serveur n’a pas répondu. Certains serveurs ne suivent pas les comptes d’autres serveurs ; le profil l’indique alors.

Sur la page d’un autre serveur, les boutons qui permettent de suivre un compte ou de répondre à une publication peuvent vous amener sur ce serveur : saisissez votre identifiant (`@username@server`) là où cette page le demande. Ce serveur ouvre alors « Ouvrir depuis un autre serveur », après votre connexion, avec l’adresse du compte ou de la publication. « Afficher le compte » ouvre son profil ici, où « Suivre » le suit ; « Répondre à la publication » ouvre ici la publication, avec une réponse déjà commencée. Rien n’est recherché avant que vous n’appuyiez sur le bouton.

Après avoir suivi un compte, la cloche « M’avertir des nouvelles publications » vous notifie de chacune de ses nouvelles publications.

Lorsque vous approuvez vos abonnés, les personnes qui souhaitent vous suivre vous envoient une demande. La ligne « N demandes d’abonnement » au-dessus de vos notifications les ouvre, et vous « Approuvez » ou « Refusez » chacune d’elles. Vos abonnés et les comptes que vous suivez figurent sur la page de profil, qui en indique le nombre.

« Qui peut voir vos abonnements et vos abonnés » sous « Profil » dans les paramètres décide qui peut ouvrir ces deux listes : « Tout le monde » (par défaut), « Abonnés » (les personnes de ce serveur dont vous avez accepté l’abonnement) ou « Vous seul ». Vous voyez toujours les deux listes, et tout le monde voit toujours combien elles comptent d’éléments. Les autres serveurs ne reçoivent que les nombres, sauf si vous choisissez « Tout le monde ». Une liste que vous ne pouvez pas ouvrir l’indique à la place des comptes.

<!-- source: timelines ecf964fa1eb9 -->
## Fils

| Fil | Ce qu’il affiche |
| --- | --- |
| Accueil | Vos publications, ainsi que les publications et les boosts des personnes que vous suivez. Il affiche aussi les publications pour les voisins des personnes que vous suivez, ainsi que les publications publiques et les publications pour les voisins des hashtags que vous suivez, venant de personnes que vous ne suivez pas. Les nouvelles publications s’affichent en haut à mesure qu’elles arrivent. Pendant que vous lisez plus bas, le bouton « nouvelles publications » au-dessus du fil compte celles qui sont arrivées au-dessus de vous. Si vous désactivez « Afficher automatiquement les nouvelles publications » (sous « Fils » dans les paramètres « Affichage et publication »), les nouvelles publications attendent que vous appuyiez sur ce bouton. |
| Local | Les publications publiques, et les publications pour les voisins, des comptes de ce serveur. |
| Fédéré | Les publications publiques des comptes de ce serveur et des comptes d’autres serveurs que ce serveur connaît. Les publications pour les voisins et les publications « Public discret » n’y figurent pas. |

Les fils Local et Fédéré s’adressent à toutes les personnes qui utilisent le serveur, que vous les suiviez ou non. Les
opérateurs d’un serveur peuvent désactiver l’un ou l’autre, ou ne l’afficher qu’aux personnes connectées ; la page l’indique alors.
« Langues à afficher », sous « Fils » dans les paramètres « Affichage et publication », limite chaque fil aux langues
que vous choisissez ; les publications sans langue définie s’affichent quand même.

Les réponses ne s’affichent que là où vous pouvez suivre la conversation. L’accueil et vos listes affichent une réponse quand elle répond à vous, à une personne que vous suivez ou à une publication de son propre auteur ; les fils Local et Fédéré n’affichent une réponse que si elle poursuit le propre fil de son auteur. Ouvrez une publication pour voir toutes ses réponses ; les pages de hashtag et les profils affichent aussi les réponses.

Une page de hashtag (par exemple `#cats`) affiche les publications publiques et les publications pour les voisins portant ce hashtag.

<!-- source: following-hashtags 8fffb811899b -->
## Suivre des hashtags

Une page de hashtag comporte le bouton « Suivre » lorsque vous êtes connecté. Après avoir suivi un hashtag, votre fil d’accueil affiche aussi,
la prochaine fois que vous l’ouvrez, les publications publiques et les publications pour les voisins portant ce hashtag, venant de personnes que vous ne suivez pas.
Une telle publication indique « Hashtag suivi #cats » au-dessus d’elle, et le hashtag mène à sa page, où « Abonné » permet de ne plus le suivre.
Les réponses arrivent aussi ; les boosts non, pas plus que les publications des personnes que vous bloquez ou mettez en sourdine, ni celles des comptes et des serveurs que
le serveur tient à l’écart de ses fils publics. L’accueil n’affiche que les publications que ce serveur a déjà : suivre un hashtag ne
va pas chercher de publications sur d’autres serveurs.

Vous pouvez suivre jusqu’à 100 hashtags. « Hashtags suivis », sous « Relations » dans les paramètres, les liste, du plus récent
au plus ancien, et vous permet de « Ne plus suivre » chacun.

<!-- source: featured-hashtags ba77c9628ab1 -->
## Hashtags mis en avant

Un profil peut mettre en avant jusqu’à dix hashtags, dans l’ordre choisi par son titulaire, chacun avec le nombre de publications « Public » et
« Public discret » du titulaire qui le portent, et la date de la dernière. Choisissez les vôtres dans « Hashtags mis en avant » sous « Profil » dans les
paramètres : saisissez un hashtag et appuyez sur Entrée pour l’ajouter après les autres, et appuyez sur le × d’un hashtag pour le retirer ; chaque modification est
enregistrée aussitôt. Un hashtag que vous n’avez pas encore utilisé peut aussi être mis en avant. Les autres serveurs qui affichent votre profil reçoivent la même liste.
Sur le profil d’un autre serveur, les nombres portent sur les publications que ce serveur a reçues, et peuvent donc être inférieurs à ceux de
ce serveur.

Un hashtag mis en avant ouvre les publications de cette personne portant ce hashtag, les plus récentes d’abord, que vous pouvez voir sur son profil
(« Publications de Alice avec #cats ») ; les boosts n’y figurent pas. « Toutes les publications avec #cats » ouvre la page du hashtag avec
les publications de tout le monde.

Tant que vous ne suivez personne, votre fil d’accueil affiche des personnes suggérées par le serveur ; « Rechercher » les affiche avant que vous ne saisissiez quoi que ce soit.

<!-- source: private-notes 881c1ea7d56c -->
## Notes privées

Sur le profil d’une autre personne, « Ajouter une note que vous seul pouvez voir » conserve une note sur ce compte, jusqu’à 2 000 caractères : comment
vous vous êtes rencontrés, ce que la personne aime. Le compte n’en est pas informé, et la note ne quitte pas ce serveur. Appuyez sur la note pour la modifier ;
l’enregistrer vide la supprime. L’archive de « Exporter », sous « Compte et données » dans les paramètres, contient vos notes, et une note disparaît lorsque
vous ou l’autre compte êtes supprimés.

<!-- source: following-in-a-feed-reader 50caaa7dbc56 -->
## Suivre dans un lecteur de flux

Les personnes sans compte sur le fédivers peuvent suivre un profil ou un hashtag dans un lecteur de flux : ajoutez l’adresse du
profil suivie de `.atom` (par exemple `https://example.com/@alice.atom`), ou celle de la page du hashtag
(`https://example.com/tags/cats.atom`). La plupart des lecteurs de flux trouvent aussi le flux à partir de l’adresse même de la page. Un flux contient
les 20 publications publiques les plus récentes des comptes de ce serveur, sans réponses ni boosts ; le flux d’un hashtag contient celles qui portent ce
hashtag. Une publication avec un avertissement de contenu n’affiche que l’avertissement, et son texte et ses médias restent sur la page de la publication. Un article
affiche son titre et son résumé, avec un lien vers l’article. Pour ne suivre que les articles d’un profil, ajoutez `/articles.atom`
à l’adresse du profil à la place (`https://example.com/@alice/articles.atom`) ; il suit les mêmes règles.

Vos propres publications figurent dans les flux selon « Proposer un flux Atom de vos publications publiques » sous « Profil » dans les paramètres. Tant que
vous ne l’avez pas modifié, ce réglage suit « Inclure votre profil et vos publications publiques dans la recherche » : vos publications figurent dans les flux tant que celui-ci est
activé. Une fois que vous l’avez modifié, changer le réglage de recherche le laisse tel quel. Lorsqu’il est désactivé, votre profil n’a pas de flux
et vos publications ne figurent dans le flux d’aucun hashtag. Un lecteur de flux peut mettre une minute ou deux à voir un changement. Les comptes qu’un
modérateur a limités ou suspendus, et les comptes qui ont déménagé, n’ont pas de flux.

<!-- source: lists 13e17a3ab8e9 -->
## Listes

« Listes » place des personnes que vous suivez dans une liste, afin que vous lisiez leurs publications séparément de votre fil d’accueil. Vous seul voyez vos listes, et personne n’est informé que vous l’avez placé dans l’une d’elles. Vous ne pouvez ajouter que des personnes que vous suivez.

<!-- source: hiding-what-you-do-not-want-to-read 29d63089083e -->
## Masquer ce que vous ne voulez pas lire

Mettez en sourdine des comptes ou des serveurs entiers, ou bloquez des comptes : voir [Bloquer et mettre en sourdine](safety.md).
