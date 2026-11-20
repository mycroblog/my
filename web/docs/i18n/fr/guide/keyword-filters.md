<!-- source: keyword-filters 466c3876a3d1 -->
# Filtres de mots-clés

Un filtre de mots-clés replie ou masque les publications qui contiennent l’un de ses mots, quel qu’en soit l’auteur : les spoilers d’une série que vous n’avez pas encore vue, un sujet dont vous avez besoin de vous reposer. Le serveur évalue les publications, donc toutes les applications que vous utilisez affichent le même résultat. Vous seul voyez vos filtres, et personne n’en est informé.

<!-- source: adding-a-filter 455f52213244 -->
## Ajouter un filtre

Ouvrez « Filtres de mots-clés » sous « Personnes et filtres » dans les paramètres, puis choisissez « Ajouter un filtre » :

1. **Nom :** ce qu’une publication repliée affiche à sa place (« Correspond au filtre “Finale de la saison” »), 50 caractères au maximum.
2. **Publications concernées :** « Replier derrière le nom du filtre » n’affiche que ce nom à la place de la publication, avec « Afficher » pour la lire.
   « Masquer complètement » exclut la publication, ainsi que sa notification.
3. **Fin :** « Jamais », au bout de l’une des durées proposées, de 30 minutes à une semaine, ou « À une date et une heure ».
4. **Mots :** un ou plusieurs mots ou expressions. « Ajouter un mot » en ajoute un autre, jusqu’à 100 par filtre. « Mot entier » à côté d’un mot
   le fait correspondre uniquement en tant que mot entier (voir [Comment les mots correspondent](#how-words-match)).
5. **Où il s’applique :** « Accueil et listes », « Notifications », « Fils publics » (les fils local, fédéré et de hashtag),
   « Discussions » et « Profils ». Un nouveau filtre s’applique partout ; décochez les endroits que vous ne voulez pas.

Choisissez « Ajouter ». Un filtre de la liste l’ouvre pour modifier chacun de ces éléments, et « Supprimer… » le supprime avec ses mots après
confirmation. Un filtre s’applique aussitôt à toutes les publications, y compris les anciennes ; les fils se rouvrent à l’écran pour afficher le changement.

La recherche, vos favoris et vos marque-pages, ainsi que vos messages directs ne sont jamais filtrés. Vos propres publications et
boosts non plus, ni les publications qu’ils citent.

<!-- source: what-a-filtered-post-looks-like 52e0483be326 -->
## À quoi ressemble une publication filtrée

Une publication qu’un filtre de repli concerne n’affiche que « Correspond au filtre » avec les noms des filtres qui l’ont retenue, et « Afficher ».
Son auteur, son texte et ses images restent cachés, et un lecteur d’écran ne lit que cette ligne, jusqu’à ce que vous choisissiez « Afficher » ;
« Masquer » la replie de nouveau, tout comme le fait de revenir sur la page.

Une notification concernant une telle publication affiche la même ligne à la place du texte de la publication. Lorsque seule une publication citée correspond, seule
la citation est repliée, même avec « Masquer », avec son propre « Afficher ». Un boost est filtré selon la publication qu’il booste.

Une publication que vous ouvrez vous-même, depuis un lien ou une notification, est repliée plutôt que masquée, afin que le lien affiche toujours
quelque chose.

<!-- source: when-a-filter-ends d70431d92b82 -->
## Quand un filtre prend fin

Un filtre qui a une fin cesse de s’appliquer à ce moment-là. Il reste dans votre liste, marqué « Expiré », jusqu’à ce que vous le supprimiez. Pour
le réutiliser, ouvrez-le et choisissez une nouvelle fin, ou « Jamais ». Pendant que vous modifiez un filtre qui a une fin, « Ne pas changer » laisse la fin telle
qu’elle est.

<!-- source: how-words-match aeca692b00c3 -->
## Comment les mots correspondent

Un mot correspond au texte d’une publication, à son avertissement de contenu (et au texte qu’il cache), aux choix de son sondage et aux descriptions de ses images. Les liens correspondent par le texte qu’ils affichent, pas par l’adresse qui se trouve derrière. Une expression comportant des espaces correspond à ces mots dans cet ordre ; toute suite d’espaces ou de sauts de ligne dans une publication correspond à une seule espace.

Les majuscules et les minuscules, les formes demi-chasse et pleine chasse, ainsi que les katakana et les hiragana correspondent tous entre eux :

| Mot | Correspond aussi à |
| --- | --- |
| `Finale` | `finale`, `FINALE`, `ｆｉｎａｌｅ` |
| `ネタバレ` | `ねたばれ`, `ﾈﾀﾊﾞﾚ` |
| `season finale` | `Season  Finale`, et les deux mots de part et d’autre d’un saut de ligne |

Un mot correspond partout où il apparaît, même à l’intérieur d’un mot plus long. « Mot entier » le fait correspondre uniquement là où aucune lettre, aucun chiffre ni aucun `_` ne le touche de part et d’autre :

| Mot | Sans « Mot entier » | Avec « Mot entier » |
| --- | --- | --- |
| `cat` | `cat`, `cats`, `concatenate` | `cat`, `cat!`, `#cat`, mais pas `cats` |
| `vote` | `vote`, `voters`, `devoted` | `vote`, `Vote.`, mais pas `voters` |

Certaines écritures notent les mots sans espaces entre eux : le chinois, le japonais (kanji, hiragana et katakana), le thaï, le lao, le khmer et le birman, entre autres. Un filtre ne peut pas y déterminer où finit un mot ; leurs lettres ne comptent donc jamais comme se touchant, et « Mot entier » fonctionne comme s’il était désactivé : `猫` correspond aussi à `子猫` et à `猫舌`, et `選挙` correspond aussi à `選挙速報`. Choisissez des mots plus longs dans ces langues pour éviter de correspondre à plus que ce que vous voulez, par exemple `最終回のネタバレ` plutôt que `最終回`. À côté d’une lettre d’une autre écriture, « Mot entier » s’applique toujours : `AI` avec « Mot entier » correspond à `AIの話` mais pas à `AIDS`.

Un filtre n’a ni caractères génériques ni expressions régulières. Ses mots correspondent dès que l’un d’eux correspond ; pour replier des publications uniquement lorsque deux mots apparaissent ensemble, écrivez-les comme une seule expression.

<!-- source: limits 2f4588a30027 -->
## Limites

Vous pouvez avoir jusqu’à 100 filtres, avec jusqu’à 100 mots chacun et 1 000 mots au total. Un mot compte au plus 100 caractères.

L’archive de « Exporter », sous « Compte et données » dans les paramètres, contient vos filtres, y compris ceux qui ont expiré, dans
`keyword_filters.json`.
