<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publier et choisir qui voit une publication

<!-- source: writing-a-post b3b7ce030983 -->
## Rédiger une publication

Choisissez « Écrire une publication » pour ouvrir le formulaire « Nouvelle publication ». Saisissez votre texte dans « Votre publication » et choisissez « Publier ». Le formulaire indique combien de caractères il vous reste. Outre le texte, vous pouvez :

- Joindre des images, une vidéo ou un fichier audio avec « Ajouter des images, une vidéo ou un fichier audio ». Décrivez chaque pièce jointe pour les personnes qui ne peuvent pas la voir ou l’entendre ; certains serveurs exigent une description pour chaque pièce jointe. La localisation et les autres métadonnées sont retirées des fichiers.
- « Ajouter un sondage », avec des options et une durée, ainsi que les choix « Autoriser plusieurs choix » et « Masquer les votes jusqu’à la fin du sondage ».
- « Ajouter un avertissement de contenu ». Les lecteurs voient d’abord l’avertissement et choisissent « Afficher le contenu ».
- Choisir « Masquer les images jusqu’à ce que les lecteurs les affichent » pour marquer les médias comme sensibles.
- Définir la langue de la publication, ainsi que qui peut y répondre, la booster et la citer (voir [Qui peut répondre, booster et citer](#who-can-reply-boost-and-quote)).
- Répondre à une publication (« Répondre ») ou en citer une (« Citer »).

Une publication qui n’a pas pu être envoyée reste sous « Publications en cours d’envoi », où vous pouvez la « Renvoyer », la « Modifier » ou l’« Abandonner ». Un serveur peut refuser une publication après une revue automatisée. Lorsque le refus peut être contesté, la publication propose « Contester ».

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Brouillons et publications programmées

Choisissez « Enregistrer le brouillon » pour conserver sur le serveur ce que vous écrivez et vider le formulaire. Un brouillon enregistré s’ouvre sur tout appareil où vous vous connectez : le lien « Brouillons et publications programmées » sous le formulaire mène à la page qui les liste, où « Modifier » rouvre un brouillon dans le formulaire. Lorsque le formulaire contient déjà autre chose, une confirmation vous est demandée avant qu’il soit remplacé. Enregistrer un brouillon ouvert depuis cette page le remplace ; le publier le publie et le retire de vos brouillons. Si le même brouillon a été enregistré sur un autre appareil depuis que vous l’avez ouvert, rien n’est écrasé ni publié : enregistrez de nouveau pour conserver votre version comme nouveau brouillon, ou publiez de nouveau pour publier votre version comme nouvelle publication.

Pour publier plus tard, choisissez « Programmer la publication » (l’horloge parmi les outils du formulaire), choisissez une date et une heure, puis « Programmer ». La date et l’heure sont dans votre fuseau horaire, celui de vos préférences, et le formulaire le nomme. Une heure passée, trop proche ou trop lointaine est refusée avant que quoi que ce soit soit envoyé ; le serveur fixe à quelle distance minimale et maximale vous pouvez programmer. La publication est publiée à cette heure, ou peu après.

La page « Brouillons et publications programmées » liste d’abord vos publications programmées, avec l’heure de publication de chacune, puis vos brouillons. Une publication programmée peut être déplacée avec « Changer l’heure » ou redevenir un brouillon avec « Annuler la programmation », et tout brouillon peut être supprimé. Vous seul voyez vos brouillons.

Lorsqu’une publication programmée ne peut pas être publiée à l’heure prévue, par exemple parce que la publication à laquelle elle répond a disparu, elle reste parmi vos brouillons avec le motif, et vous recevez une notification qui mène à vos brouillons, afin que vous puissiez la corriger et la programmer de nouveau.

<!-- source: who-can-see-a-post 267cf73d0612 -->
## Qui peut voir une publication

Le bouton « Visibilité » du formulaire ouvre « Qui peut voir cette publication ? » pour cette publication. La valeur par défaut des nouvelles publications est le
paramètre « Qui voit vos publications par défaut » sous « Affichage, publication et fils » dans les paramètres.

| Choix | Qui peut la voir |
| --- | --- |
| Public | Tout le monde, ici et sur les autres serveurs. Elle est affichée sur les fils publics. |
| Ce serveur | Les personnes connectées à ce serveur, et vos abonnés sur d’autres serveurs. Elle est affichée sur le fil local de ce serveur. |
| Non répertoriée | Toute personne disposant du lien, mais elle n’est pas affichée sur les fils publics. |
| Abonnés | Uniquement vos abonnés et les personnes que vous mentionnez. |
| Personnes mentionnées | Uniquement les personnes que vous mentionnez. |

![Qui peut voir une publication sur ce serveur et sur les autres serveurs, pour chacune des cinq visibilités. La liste ci-dessous dit la même chose.](images/visibility.svg)

Ligne par ligne, sur ce serveur puis sur les autres serveurs :

- **Public** : tout le monde, y compris les personnes qui ne sont pas connectées, et la publication est affichée sur les fils local et fédéré. Sur les autres serveurs, tout le monde peut la lire, et elle est envoyée à vos abonnés et aux personnes que vous mentionnez.
- **Ce serveur** : les personnes connectées à ce serveur, et la publication est affichée sur le fil local. Les autres serveurs ne la reçoivent que pour vos abonnés qui s’y trouvent, qui la voient comme une publication « Abonnés » ; elle n’est envoyée à personne d’autre.
- **Non répertoriée** : toute personne disposant du lien, mais la publication n’est pas affichée sur les fils publics. Sur les autres serveurs aussi, toute personne disposant du lien peut la lire, et elle est envoyée à vos abonnés et aux personnes que vous mentionnez.
- **Abonnés** : vos abonnés et les personnes que vous mentionnez, sur ce serveur et sur les autres serveurs.
- **Personnes mentionnées** : uniquement les personnes que vous mentionnez, sur ce serveur et sur les autres serveurs.

Sur un serveur d’une seule personne, où personne d’autre n’est connecté, « Ce serveur » ne signifie rien de plus que « Abonnés » : les nouvelles publications
ne peuvent pas l’avoir, et une valeur par défaut « Ce serveur » fonctionne comme « Abonnés ». Le menu de visibilité et la valeur par défaut dans « Affichage,
publication et fils » l’omettent, et une réponse à une publication « Ce serveur » est destinée aux « Abonnés ». Les publications qui l’ont déjà
la conservent.

Un blocage l’emporte toujours : une personne qui vous bloque, ou que vous bloquez, ne voit pas votre publication, quel que soit votre choix.

<!-- source: this-server d39e8d3e2df6 -->
### « Ce serveur »

Une publication réglée sur « Ce serveur » reste sur votre serveur. Les personnes qui n’y sont pas connectées ne peuvent pas la lire, même avec le
lien, et elle n’apparaît jamais sur le fil fédéré. Les autres serveurs ne la reçoivent que pour vos abonnés qui s’y trouvent, et ils
la montrent à ces abonnés comme une publication réservée aux abonnés.

- Si vous mentionnez sur un autre serveur une personne qui ne vous suit pas, elle ne reçoit pas la publication, et vos abonnés qui s’y trouvent voient la mention comme un simple lien. Le formulaire vous en avertit.
- Les comptes de votre serveur peuvent booster une publication « Ce serveur », et le boost n’est affiché que sur ce serveur.
- Une réponse à une publication « Ce serveur » commence elle aussi en « Ce serveur » ; vous pouvez la modifier.

<!-- source: other-things-to-know d1f569ac2d1d -->
### À savoir aussi

- Vous ne pouvez pas changer la visibilité d’une publication après l’avoir publiée. Lorsque vous modifiez une publication, la visibilité et la publication à laquelle elle répond restent inchangées.
- Un modérateur peut limiter la portée des publications d’un compte. Le formulaire indique alors « Votre compte peut publier au plus en … » et un choix plus large est publié au niveau le plus large autorisé.
- Toute personne qui peut voir une publication publique, « Ce serveur » ou non répertoriée peut la booster. Vous seul pouvez booster vos propres publications « Abonnés », et seuls vos abonnés voient ce boost. Les publications « Personnes mentionnées » ne peuvent pas être boostées.
- L’auteur d’une publication peut choisir qui peut y répondre et qui peut la booster ; les publications de certains autres serveurs portent aussi un tel choix. Une réponse ou un boost en dehors de ce choix est refusé, avec un message tel que « L’auteur de cette publication ne vous permet pas d’y répondre. » L’auteur, les personnes que la publication mentionne et l’auteur de la publication à laquelle elle répond peuvent toujours répondre.
- L’auteur peut aussi permettre à certaines personnes de répondre ou de booster seulement une fois qu’il l’approuve. Une telle réponse ou un tel boost attend : vous êtes notifié (« … a demandé à répondre à votre publication »), et il reste en dehors de votre publication jusqu’à ce que vous l’approuviez. Une réponse en attente venant d’un autre serveur ne figure pas dans la conversation sous votre publication ni dans son nombre de réponses ; elle apparaît toujours sur le profil de son auteur et sur sa propre page. Un boost en attente n’est pas compté. L’approuver l’ajoute à votre publication ; le refuser le laisse en dehors, et l’autre serveur en est informé dans les deux cas.
- Une réponse ou un boost venant d’un autre serveur qui sort entièrement de votre choix est tenu à l’écart de votre publication de la même façon, et ne vous notifie de rien.
- Lorsque vous répondez à une publication dont l’auteur approuve les réponses ou les boosts à la main, ou que vous la boostez, la vôtre attend cette approbation. D’ici là, vous seul voyez votre réponse, sous la publication et sur sa propre page, marquée comme en attente ; les autres n’en voient rien, et rien n’est envoyé aux autres serveurs. Votre boost n’est pas effectué avant d’être approuvé. Vous êtes notifié lorsque l’auteur l’approuve ou le refuse (« … a approuvé votre réponse, boost ou citation »). Supprimer une réponse en attente, ou annuler un boost en attente, retire la demande.
- Vous pouvez épingler jusqu’à cinq de vos publications à votre profil avec « Épingler au profil ». Une publication « Personnes mentionnées » ne peut pas être épinglée.

<!-- source: who-can-reply-boost-and-quote c9ea034f7416 -->
## Qui peut répondre, booster et citer

Le bouton « Qui peut répondre, booster et citer » parmi les outils du formulaire ouvre les choix pour cette publication. Pour les réponses, les boosts et les citations, choisissez séparément qui peut le faire immédiatement : « Tout le monde », « Vos abonnés et les personnes que vous mentionnez », « Les personnes que vous mentionnez » ou « Vous seul ». Sous chacun, « Les autres peuvent répondre avec votre approbation » (et de même pour les boosts et les citations) permet à tous les autres de demander plutôt que d’être refusés. Dès qu’un choix admet moins que tout le monde, le formulaire en montre le résultat au-dessus de ses outils, par exemple « Réponses : tout le monde. Boosts : vous seul. Citations : tout le monde. », avant que vous publiiez.

- Les personnes qu’une publication mentionne peuvent toujours y répondre. Une publication « Abonnés » ne peut être boostée et citée que par vous, et une publication « Personnes mentionnées » par personne, quel que soit votre choix.
- Les valeurs par défaut des nouvelles publications se trouvent sous « Qui peut répondre à vos nouvelles publications, les booster et les citer » dans « Affichage, publication et fils » dans les paramètres.
- Pour les modifier pour une publication déjà envoyée, choisissez « Changer qui peut répondre, booster et citer » dans son menu « Plus d’actions ». Chaque choix est enregistré immédiatement. Les réponses, boosts et citations que vous aviez approuvés restent, et les demandes que le nouveau choix admet sont approuvées.
- Les demandes qui vous attendent sont listées sous « Demandes d’approbation » dans les paramètres, à côté de « Demandes d’abonnement » ; la notification de chacune y renvoie avec « Examiner les demandes ». « Approuver » ajoute la réponse, le boost ou la citation à votre publication, et la conversation en dessous affiche immédiatement une réponse approuvée ; « Refuser » la laisse en dehors.
- Sur la publication de quelqu’un d’autre, « Répondre » n’est pas affiché et « Booster » ne peut pas être activé lorsque son auteur ne vous le permet pas. Lorsque l’auteur les approuve à la main, le formulaire l’indique avant que vous répondiez, et après que vous avez boosté, la publication indique « Votre boost attend l’approbation de l’auteur. »

<!-- source: after-posting dc553fa0ead9 -->
## Après la publication

Sous une publication, vous trouvez « Répondre », « Booster », « Citer », « Favori » et « Marque-page ». Le menu « Plus d’actions » contient « Copier le lien » et, pour vos propres publications, « Modifier », « Épingler au profil » et « Supprimer ». Une publication modifiée affiche « Modifiée », et ses versions précédentes se trouvent sur sa page « Historique des modifications ». « Supprimer » retire la publication pour tout le monde, avec ses boosts et ses favoris.

Pour la publication d’une autre personne, le même menu propose de suivre son auteur, de le mettre en sourdine, de le bloquer ou de le signaler, ainsi que « Message direct ». Vos marque-pages sont listés sur la page « Marque-pages » ; vous seul les voyez.

Lorsque le serveur propose la traduction, ou que votre navigateur peut traduire, les publications dans d’autres langues ont « Traduire » parmi leurs actions, et dans le menu « Plus d’actions », qui le contient lorsque la publication est trop étroite pour un bouton de plus. Pendant qu’une publication affiche sa traduction, une ligne sous le texte indique de quelle langue elle a été traduite et par qui, avec « Afficher l’original » pour revenir en arrière. Si le serveur envoie le texte des publications à un service de traduction, le sélecteur de visibilité vous indique pour quels choix.

<!-- source: writing-an-article 0dfa0a80bb8e -->
## Écrire un article

« Écrire un article » sous le formulaire de nouvelle publication ouvre l’éditeur d’articles, une page à part. Donnez à l’article un titre, un résumé si vous le souhaitez (laissé vide, il est composé à partir du début du corps), une image de couverture avec sa description, et le corps, écrit en Markdown : `#` commence un titre, `**text**` met en gras, et « Gras » et « Lien » au-dessus du corps font la même chose pour le texte que vous sélectionnez (Ctrl+B et Ctrl+K, ou Cmd sur un Mac). Déposez ou collez des images dans le corps, ou choisissez-les avec « Ajouter des images » : chacune est téléversée et placée à l’emplacement du curseur sous la forme `![](media:…)` ; écrivez sa description entre les crochets. Seules les images que vous téléversez s’affichent dans un article ; une image provenant d’une autre adresse devient un lien avec sa description.

- Les hashtags vont dans leur propre champ, séparés par des espaces ; l’article est listé sous eux, et un `#` dans le corps reste du texte. Les mentions dans le corps renvoient vers les personnes mais ne les notifient pas.
- « Aperçu » montre l’article tel que les lecteurs le verront, rendu par le serveur exactement comme il sera publié, avec la taille du corps et le nombre de ses images au regard des limites de ce serveur.
- « Marquer comme sensible » montre d’abord aux lecteurs le titre et le résumé, et replie la couverture et le corps. Un article ne peut pas être uniquement « Personnes mentionnées ».
- Pendant que vous écrivez, l’article est enregistré comme brouillon à chaque pause (« Brouillon enregistré »), de sorte qu’il s’ouvre sur tout appareil depuis « Brouillons et publications programmées », où « Modifier » le ramène dans l’éditeur. Si le brouillon a été modifié entre-temps sur un autre appareil, rien n’est écrasé : votre texte reste dans l’éditeur, et « Enregistrer comme nouveau brouillon » le conserve comme un autre brouillon.
- « Publier » publie l’article maintenant et ouvre sa page ; « Programmer » le publie à l’heure que vous choisissez, comme pour une publication.
- Pour modifier un article que vous avez publié, choisissez « Modifier » dans son menu « Plus d’actions ». L’éditeur s’ouvre avec ce que vous avez écrit ; « Enregistrer les modifications » le remplace, et la version précédente reste dans son « Historique des modifications ». La visibilité d’un article ne change pas.

Un serveur peut ne laisser que certaines personnes écrire de nouveaux articles ; « Écrire un article » n’est alors pas affiché aux autres, qui peuvent toujours modifier les articles qu’elles ont publiés.

<!-- source: reading-articles f2811fffd462 -->
## Lire des articles

Un article est une longue publication avec un titre, un résumé et souvent une image de couverture, écrite sur ce serveur ou sur un autre. Les fils et les profils affichent un article sous forme de carte avec sa couverture, le mot « Article », son titre et son résumé ; sélectionnez le titre pour lire l’article entier sur sa propre page, dont le titre est l’en-tête. Les réponses, boosts, citations, favoris et réactions fonctionnent comme sur toute publication. Les articles n’ont pas de « Traduire ».

Un article que son auteur a marqué comme sensible affiche son titre et son résumé, et cache le reste : sur la carte, « Afficher les médias sensibles » montre sa couverture, et sur sa page « Afficher l’article » montre sa couverture et son texte. L’onglet « Articles » d’un profil ne liste que les articles de cette personne, les plus récents en premier ; pour quelqu’un d’un autre serveur, seulement les articles que ce serveur a reçus.

<!-- source: direct-messages 59062702f4be -->
## Messages directs

« Messages directs » dans la navigation rassemble vos publications « Personnes mentionnées » et celles que vous recevez, une ligne par conversation, celle dont la publication est la plus récente en premier. Chaque ligne indique qui participe à la conversation, quand sa publication la plus récente a été écrite et le début de cette publication. Le nombre à côté de « Messages directs » est le nombre de conversations qui comportent une publication que vous n’avez pas encore lue ; chacune a un point avant ses noms.

Choisissez le début d’une publication pour ouvrir le fil de la conversation ; cela marque la conversation comme lue sur tous vos appareils. Une réponse à une publication « Personnes mentionnées » est toujours elle aussi « Personnes mentionnées », de sorte qu’elle n’atteint personne que la conversation n’atteignait pas. Sur les téléphones, « Messages directs » se trouve dans « Menu », sauf si vous le déplacez dans la barre d’onglets sous « Affichage, publication et fils » dans les paramètres.

<!-- source: deleting-old-posts-automatically e581c8d27380 -->
## Supprimer automatiquement les anciennes publications

« Supprimer les anciennes publications » dans les paramètres supprime vos publications une fois qu’elles sont plus anciennes que l’âge que vous choisissez, comme si vous les aviez supprimées vous-même : elles disparaissent ici et des serveurs auxquels elles ont été envoyées, et ne peuvent pas être restaurées. Les publications que vous conservez (épinglées, mises en marque-page ou en favori par vous, avec des médias ou un sondage, des articles, de certaines visibilités, ou avec beaucoup de boosts ou de favoris) restent. Vos boosts de publications d’autres personnes ne sont jamais supprimés. Le serveur vérifie toutes les heures et supprime au plus 100 de vos publications à chaque fois ; de nombreuses anciennes publications mettent donc plusieurs heures à disparaître.

Un nouveau compte commence avec les réglages que le serveur a choisis pour les nouveaux comptes ; ils sont à vous dès lors, et vous pouvez les modifier comme tout autre réglage.

Un serveur peut aussi conserver les publications pendant un nombre de jours maximal. La page « À propos » du serveur l’indique avant que vous vous inscriviez. Sous un tel maximum, la suppression automatique est toujours activée pour vous, l’âge que vous choisissez ne peut pas dépasser le maximum, et seuls les types de publications que le serveur autorise peuvent être conservés (par défaut uniquement les publications épinglées ; vos articles sont donc supprimés eux aussi, à moins que le serveur n’autorise à les conserver). Certains serveurs permettent à certaines personnes de conserver leurs publications plus longtemps, par exemple comme avantage d’une formule de soutien.

Lorsqu’un maximum commence à s’appliquer à vous (le serveur l’instaure ou l’abaisse, ou vous perdez le droit de conserver des publications plus longtemps), vous recevez une notification, et un e-mail sauf si vous l’avez désactivé, indiquant à partir de quand il s’applique. D’ici là, vos propres réglages restent en vigueur. La page des paramètres affiche la date et, avec son aperçu, celles de vos publications qui seront alors supprimées.
