<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publier et choisir qui voit une publication

<!-- source: writing-a-post 5f24a2f4b092 -->
## Rédiger une publication

Choisissez « Écrire une publication » pour ouvrir le formulaire « Nouvelle publication ». Saisissez votre texte dans « Votre publication » et choisissez « Publier ». Le formulaire indique combien de
caractères il vous reste. Outre le texte, vous pouvez :

- Joindre des images, une vidéo ou un fichier audio avec « Ajouter des images, une vidéo ou un fichier audio ». Décrivez chaque pièce jointe pour les personnes qui ne peuvent pas la voir ou l’entendre ; certains serveurs exigent une description pour chaque pièce jointe, et « Exiger des descriptions de médias » dans les paramètres « Affichage et publication » l’exige de vos propres publications. La localisation et les autres métadonnées sont retirées des fichiers.
- « Ajouter un sondage », avec des options et une durée, ainsi que les choix « Autoriser plusieurs choix » et « Masquer les votes jusqu’à la fin du sondage ».
- « Ajouter un avertissement de contenu ». Les lecteurs voient d’abord l’avertissement et choisissent « Afficher le contenu ».
- Choisir « Masquer les images jusqu’à ce que les lecteurs les affichent » pour marquer les médias comme sensibles.
- Définir la langue de la publication, ainsi que qui peut y répondre, la booster et la citer (voir [Qui peut répondre, booster et citer](#who-can-reply-boost-and-quote)).
- Répondre à une publication (« Répondre ») ou en citer une (« Citer »).

Une publication qui n’a pas pu être envoyée reste sous « Publications en cours d’envoi », où vous pouvez la « Renvoyer », la « Modifier » ou l’« Abandonner ».
Un serveur peut refuser une publication après une revue automatisée. Lorsque le refus peut être contesté, la publication propose « Contester ».

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Brouillons et publications programmées

Choisissez « Enregistrer le brouillon » pour conserver sur le serveur ce que vous écrivez et vider le formulaire. Un brouillon enregistré s’ouvre sur tout appareil où vous vous connectez : le lien « Brouillons et publications programmées » sous le formulaire mène à la page qui les liste, où « Modifier » rouvre un brouillon dans le formulaire. Lorsque le formulaire contient déjà autre chose, une confirmation vous est demandée avant qu’il soit remplacé. Enregistrer un brouillon ouvert depuis cette page le remplace ; le publier le publie et le retire de vos brouillons. Si le même brouillon a été enregistré sur un autre appareil depuis que vous l’avez ouvert, rien n’est écrasé ni publié : enregistrez de nouveau pour conserver votre version comme nouveau brouillon, ou publiez de nouveau pour publier votre version comme nouvelle publication.

Pour publier plus tard, choisissez « Programmer la publication » (l’horloge parmi les outils du formulaire), choisissez une date et une heure, puis « Programmer ». La date et l’heure sont dans votre fuseau horaire, celui de vos préférences, et le formulaire le nomme. Une heure passée, trop proche ou trop lointaine est refusée avant que quoi que ce soit soit envoyé ; le serveur fixe à quelle distance minimale et maximale vous pouvez programmer. La publication est publiée à cette heure, ou peu après.

La page « Brouillons et publications programmées » liste d’abord vos publications programmées, avec l’heure de publication de chacune, puis vos brouillons. Une publication programmée peut être déplacée avec « Changer l’heure » ou redevenir un brouillon avec « Annuler la programmation », et tout brouillon peut être supprimé. Vous seul voyez vos brouillons.

Lorsqu’une publication programmée ne peut pas être publiée à l’heure prévue, par exemple parce que la publication à laquelle elle répond a disparu, elle reste parmi vos brouillons avec le motif, et vous recevez une notification qui mène à vos brouillons, afin que vous puissiez la corriger et la programmer de nouveau.

<!-- source: who-can-see-a-post be247913f548 -->
## Qui peut voir une publication

L’outil « Qui peut la voir » du formulaire ouvre les choix pour cette publication. La valeur par défaut des nouvelles publications est « Qui peut la voir »
sous « Nouvelles publications » dans les paramètres « Affichage et publication ».

| Choix | Qui peut la voir |
| --- | --- |
| Public | Tout le monde, ici et sur les autres serveurs. Elle est affichée sur les fils publics et les hashtags. |
| Voisins | Les personnes connectées à ce serveur, et vos abonnés sur d’autres serveurs. Elle est affichée sur le fil local de ce serveur. |
| Public discret | Toute personne disposant du lien ou de votre profil, mais elle n’est affichée ni sur les fils publics ni sur les hashtags. |
| Abonnés seulement | Uniquement vos abonnés et les personnes que vous mentionnez. |
| Direct | Uniquement les personnes que vous mentionnez. |

![Qui peut voir une publication sur ce serveur et sur les autres serveurs, pour chacun des cinq choix. La liste ci-dessous dit la même chose.](images/visibility.svg)

Ligne par ligne, sur ce serveur puis sur les autres serveurs :

- **Public** : tout le monde, y compris les personnes qui ne sont pas connectées, et la publication est affichée sur les fils local et fédéré. Sur les autres serveurs, tout le monde peut la lire, et elle est envoyée à vos abonnés et aux personnes que vous mentionnez.
- **Voisins** : les personnes connectées à ce serveur, et la publication est affichée sur le fil local. Les autres serveurs ne la reçoivent que pour vos abonnés qui s’y trouvent, qui la voient comme une publication « Abonnés seulement » ; elle n’est envoyée à personne d’autre.
- **Public discret** : toute personne disposant du lien ou de votre profil, mais la publication n’est affichée ni sur les fils publics ni sur les hashtags. Sur les autres serveurs aussi, toute personne disposant du lien peut la lire, et elle est envoyée à vos abonnés et aux personnes que vous mentionnez.
- **Abonnés seulement** : vos abonnés et les personnes que vous mentionnez, sur ce serveur et sur les autres serveurs.
- **Direct** : uniquement les personnes que vous mentionnez, sur ce serveur et sur les autres serveurs.

Sur un serveur d’une seule personne, où personne d’autre n’est connecté, « Voisins » ne signifie rien de plus que « Abonnés seulement » : les nouvelles publications
ne peuvent pas l’avoir, et une valeur par défaut « Voisins » fonctionne comme « Abonnés seulement ». « Qui peut la voir » et la valeur par défaut dans les paramètres
l’omettent, et une réponse à une publication pour les voisins est « Abonnés seulement ». Les publications qui l’ont déjà
la conservent.

Un blocage l’emporte toujours : une personne qui vous bloque, ou que vous bloquez, ne voit pas votre publication, quel que soit votre choix.

<!-- source: neighbors 2581a57320f5 -->
### « Voisins »

Une publication pour les voisins reste sur votre serveur. Les personnes qui n’y sont pas connectées ne peuvent pas la lire, même avec le
lien, et elle n’apparaît jamais sur le fil fédéré. Les autres serveurs ne la reçoivent que pour vos abonnés qui s’y trouvent, et ils
la montrent à ces abonnés comme une publication réservée aux abonnés.

- Si vous mentionnez une personne d’un autre serveur qui ne vous suit pas, elle ne reçoit pas la publication, et vos abonnés de ce serveur voient la mention comme un simple lien. Le formulaire vous en avertit.
- Les comptes de votre serveur peuvent booster une publication pour les voisins, et le boost n’est affiché que sur ce serveur.
- Une réponse à une publication pour les voisins commence aussi comme « Voisins » ; vous pouvez la modifier.

<!-- source: other-things-to-know b36265de1760 -->
### Autres choses à savoir

- Vous ne pouvez pas changer qui peut voir une publication après l’avoir publiée. Une modification conserve qui peut la voir et la publication à laquelle elle répond.
- Un modérateur peut limiter la portée des publications d’un compte. Le formulaire indique alors « Votre compte peut publier au plus comme … » et un choix plus large est publié au niveau le plus large autorisé.
- Toute personne qui peut voir une publication « Public », « Voisins » ou « Public discret » peut la booster. Seul vous pouvez booster vos propres publications « Abonnés seulement », et seuls vos abonnés voient ce boost. Les publications « Direct » ne peuvent pas être boostées.
- L’auteur d’une publication peut choisir qui peut y répondre et qui peut la booster ; les publications de certains autres serveurs portent aussi un tel choix. Une réponse ou un boost hors de ce choix est refusé, avec un message tel que « L’auteur de cette publication ne vous permet pas d’y répondre. ». L’auteur, les personnes que la publication mentionne et l’auteur de la publication à laquelle elle répond peuvent toujours répondre.
- L’auteur peut aussi permettre à certaines personnes de répondre ou de booster seulement une fois qu’il l’a approuvé. Une telle réponse ou un tel boost attend : vous êtes notifié (« … a demandé à répondre à votre publication »), et il reste à l’écart de votre publication jusqu’à ce que vous l’approuviez. Une réponse en attente venant d’un autre serveur ne figure pas dans la conversation sous votre publication ni dans son nombre de réponses ; elle apparaît quand même sur le profil de son auteur et sur sa propre page. Un boost en attente n’est pas compté. L’approuver l’ajoute à votre publication ; le refuser le tient à l’écart, et l’autre serveur en est informé dans les deux cas.
- Une réponse ou un boost venant d’un autre serveur qui sort entièrement de votre choix est tenu à l’écart de votre publication de la même façon, et ne vous notifie de rien.
- Lorsque vous répondez à une publication, ou la boostez, dont l’auteur approuve à la main les réponses ou les boosts, la vôtre attend cette approbation. D’ici là, vous seul voyez votre réponse, sous la publication et sur sa propre page, marquée « En attente d’approbation » ; les autres n’en voient rien, et rien n’est envoyé aux autres serveurs. Votre boost n’est pas effectué tant qu’il n’est pas approuvé. Vous êtes notifié lorsque l’auteur l’approuve ou le refuse (« … a approuvé votre réponse, boost ou citation »). Supprimer une réponse en attente, ou annuler un boost en attente, retire la demande.

<!-- source: who-can-reply-boost-and-quote 5d5ece5f0cb2 -->
## Qui peut répondre, booster et citer

« Qui peut répondre, booster et citer » dans le menu « Qui peut la voir » ouvre les choix pour cette publication, chacun avec sa valeur,
par exemple « Qui peut booster : Abonnés, d’autres avec votre approbation ». Pour les réponses, les boosts et les citations, choisissez
qui peut le faire immédiatement : « Tout le monde », « Abonnés », « Personnes que vous mentionnez » ou « Vous seul ». Sous chacun, « Permettre à d’autres avec votre
approbation » permet à tous les autres de demander au lieu d’être refusés.

- Les personnes qu’une publication mentionne peuvent toujours y répondre. Une publication « Abonnés seulement » ne peut être boostée et citée que par vous, et une publication « Direct » par personne, quel que soit votre choix.
- Les valeurs par défaut des nouvelles publications, avec les mêmes choix, se trouvent sous « Qui peut répondre, booster et citer » dans les paramètres « Affichage et publication ».
- Les demandes qui vous attendent sont listées dans « En attente d’approbation », ouvert par la ligne en haut des notifications. « Approuver » ajoute la réponse, le boost ou la citation à votre publication, et la conversation en dessous affiche aussitôt une réponse approuvée ; « Refuser » la tient à l’écart.
- Sur la publication de quelqu’un d’autre, « Répondre » n’est pas affiché et « Booster » ne peut pas être actionné lorsque son auteur ne vous le permet pas. Lorsque l’auteur les approuve à la main, le formulaire le dit avant que vous répondiez, et après que vous avez boosté, la publication indique « Boost en attente d’approbation ».

<!-- source: after-posting e11fd06a76e8 -->
## Après la publication

Sous une publication, vous trouvez « Répondre », « Booster », « Citer », « Favori », « Réagir » et « Plus ». « Plus » propose « Copier le lien », « Ouvrir
la page d’origine » (pour une publication d’un autre serveur), « Partager » et « Marque-page » ; vos marque-pages se trouvent sur la page « Marque-pages »,
et vous seul les voyez. Sur votre propre publication, il propose aussi « Modifier » (pas pour une publication avec un sondage), « Supprimer et réécrire… » et
« Supprimer… ». Les deux suppressions demandent d’abord confirmation. « Supprimer… » retire la publication pour tout le monde, avec ses boosts et ses favoris ;
« Supprimer et réécrire… » fait de même et ouvre le formulaire avec son texte, ses images et son sondage comme nouvelle publication.

Une publication modifiée affiche « · modifiée » après son heure ; choisissez-le pour ouvrir « Historique des modifications », de la version la plus récente à la plus ancienne, avec les
changements marqués.

Un sondage accepte un vote, ou plusieurs lorsque son auteur le permet : choisissez, puis « Voter ». Les résultats s’affichent une fois que vous avez voté ou que le
sondage est terminé, et pour son auteur à tout moment. « Réagir » ouvre un sélecteur d’émojis (recherche, catégories, teinte de peau et émojis
personnalisés de ce serveur) ; chaque personne donne à une publication un favori ou une réaction. Sur la page même de la publication, un émoji liste
les personnes qui ont réagi avec lui. Le formulaire comporte aussi un bouton « Ajouter un émoji ».

Une publication dans une autre langue que la vôtre propose « Traduire » lorsque le serveur dispose d’un service de traduction ou que votre navigateur peut
traduire ; « Afficher l’original » revient en arrière. Derrière un avertissement de contenu, la ligne se trouve dans le bandeau de l’avertissement. Si le serveur envoie
le texte des publications à un service de traduction, « Qui peut la voir » vous indique pour quels choix.

Les images, les GIF, les vidéos et les fichiers audio s’affichent dans la publication. Choisissez une image pour ouvrir la visionneuse : faites glisser ou utilisez les flèches, zoomez et
lisez sa description. Un média marqué comme sensible reste masqué sous une ligne telle que « Image sensible », jusqu’à ce que vous choisissiez « Afficher ». Un lien dans une
publication s’affiche sous forme de carte.

<!-- source: writing-an-article 3bc498986329 -->
## Rédiger un article

« Écrire un article », sous le formulaire de nouvelle publication, ouvre l’éditeur d’articles, une page à part. Donnez à l’article un titre, un
résumé si vous le souhaitez (laissé vide, il est tiré du début du corps), une image de couverture avec sa description, et le
corps, écrit en Markdown : `#` ouvre un titre, `**text**` met en gras, et « Gras » et « Lien » au-dessus du corps font de même pour
le texte que vous sélectionnez (Ctrl+B et Ctrl+K, ou Cmd sur un Mac). Déposez ou collez des images dans le corps, ou choisissez-les avec « Ajouter des
images » : chacune est téléversée et placée à l’endroit du curseur sous la forme `![](media:…)` ; écrivez sa description entre les crochets. Seules les images que vous téléversez s’affichent dans un article ; une image d’une autre adresse devient un lien avec sa description.

- Les hashtags vont dans leur propre champ, séparés par des espaces ; l’article est listé sous eux, et un `#` dans le corps reste du texte. Les mentions dans le corps renvoient vers les personnes mais ne les notifient pas.
- « Aperçu » montre l’article tel que les lecteurs le verront, rendu par le serveur exactement comme il sera publié, avec la taille du corps et le nombre de ses images par rapport aux limites de ce serveur.
- « Marquer comme sensible » montre d’abord aux lecteurs le titre et le résumé, et replie la couverture et le corps. Un article ne peut pas être « Direct ».
- Pendant que vous écrivez, l’article est enregistré comme brouillon à chaque pause (« Brouillon enregistré »), de sorte qu’il s’ouvre sur tout appareil depuis « Brouillons et publications programmées », où « Modifier » le ramène dans l’éditeur. Si le brouillon a été modifié entre-temps sur un autre appareil, rien n’est écrasé : votre texte reste dans l’éditeur, et « Enregistrer comme nouveau brouillon » le conserve comme un autre brouillon.
- « Publier » publie l’article maintenant et ouvre sa page ; « Programmer » le publie à l’heure que vous choisissez, comme pour une publication.
- Pour modifier un article que vous avez publié, choisissez « Modifier » dans son menu « Plus d’actions ». L’éditeur s’ouvre avec ce que vous avez écrit ; « Enregistrer les modifications » le remplace, et la version précédente reste dans son « Historique des modifications ». Qui peut voir un article ne change pas.

Un serveur peut ne laisser que certaines personnes écrire de nouveaux articles ; « Écrire un article » n’est alors pas affiché aux autres, qui peuvent toujours
modifier les articles qu’elles ont publiés.

<!-- source: reading-articles f2811fffd462 -->
## Lire des articles

Un article est une longue publication avec un titre, un résumé et souvent une image de couverture, écrite sur ce serveur ou sur un autre. Les fils et les profils affichent un article sous forme de carte avec sa couverture, le mot « Article », son titre et son résumé ; sélectionnez le titre pour lire l’article entier sur sa propre page, dont le titre est l’en-tête. Les réponses, boosts, citations, favoris et réactions fonctionnent comme sur toute publication. Les articles n’ont pas de « Traduire ».

Un article que son auteur a marqué comme sensible affiche son titre et son résumé, et cache le reste : sur la carte, « Afficher les médias sensibles » montre sa couverture, et sur sa page « Afficher l’article » montre sa couverture et son texte. L’onglet « Articles » d’un profil ne liste que les articles de cette personne, les plus récents en premier ; pour quelqu’un d’un autre serveur, seulement les articles que ce serveur a reçus.

<!-- source: direct-messages e262b7a83b69 -->
## Messages directs

« Messages directs » dans la navigation réunit vos publications « Direct » et celles que vous recevez, une ligne par
conversation, celle dont la publication est la plus récente en premier. Chaque ligne indique qui participe à la conversation, quand sa publication la plus récente a été
écrite, et le début de cette publication. Le nombre à côté de « Messages directs » est le nombre de conversations qui contiennent une publication que vous n’avez pas
encore lue ; chacune d’elles porte un point devant ses noms.

Choisissez le début d’une publication pour ouvrir le fil de la conversation ; cela marque la conversation comme lue sur tous vos appareils. Une
réponse à une publication « Direct » est toujours « Direct » aussi, de sorte qu’elle n’atteint personne que la conversation n’atteignait pas. Sur
les téléphones, « Messages directs » se trouve dans « Menu ».

<!-- source: deleting-old-posts-automatically ab68788c9a84 -->
## Supprimer automatiquement les anciennes publications

« Supprimer automatiquement les anciennes publications », sous « Compte et données » dans les paramètres, supprime vos publications dès qu’elles dépassent
l’âge que vous choisissez, comme si vous les aviez supprimées vous-même : elles disparaissent ici et des serveurs auxquels elles ont été envoyées, et ne peuvent pas être restaurées. Les publications que vous gardez
(épinglées, mises en marque-page ou en favori par vous, avec des médias ou un sondage, les articles, selon qui peut les voir, ou avec beaucoup de boosts ou de
favoris) restent. Vos boosts de publications d’autres personnes ne sont jamais supprimés. Le serveur vérifie toutes les heures et supprime au
plus 100 de vos publications à chaque fois ; de nombreuses anciennes publications mettent donc plusieurs heures à disparaître.

Un nouveau compte commence avec les réglages que le serveur a choisis pour les nouveaux comptes ; ils sont les vôtres à partir de là, et vous pouvez les
modifier comme n’importe quel autre réglage.

Un serveur peut aussi conserver les publications au plus un certain nombre de jours. La page « À propos » du serveur l’indique avant l’inscription. Sous un tel
maximum, la suppression automatique est toujours activée pour vous, l’âge que vous choisissez ne peut pas dépasser le maximum, et seuls les
types de publications que le serveur autorise peuvent être conservés (par défaut, seulement les publications épinglées ; vos articles sont donc supprimés aussi,
sauf si le serveur autorise à les conserver). Certains serveurs permettent à certaines personnes de conserver leurs publications plus longtemps, par exemple comme avantage d’une formule de soutien.

Lorsqu’un maximum commence à s’appliquer à vous (le serveur l’introduit ou le baisse, ou vous perdez le droit de conserver des publications plus longtemps),
vous recevez une notification, qui mène à cette page, et un e-mail sauf si vous l’avez désactivé, indiquant à partir de quand il s’applique.
D’ici là, vos réglages restent en vigueur. La page indique la date et, sous « Publications qui seraient supprimées », combien disparaissent alors.
