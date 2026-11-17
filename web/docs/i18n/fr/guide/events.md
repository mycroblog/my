<!-- source: events f9b49d649309 -->
# Événements

Les événements permettent aux membres d’un serveur d’organiser un rassemblement : une date, un lieu de rendez-vous et la liste des personnes qui viennent. Les événements restent sur le serveur qui les héberge : seules les personnes disposant d’un compte sur ce serveur peuvent les voir ou y répondre, et ils ne sont jamais envoyés à d’autres serveurs.

<!-- source: finding-events aad251dc685d -->
## Trouver des événements

Choisissez « Événements » dans la navigation. « Événements à venir » liste les événements que vous pouvez voir, les plus proches en premier. Sur les grands écrans, « À venir » dans la colonne latérale affiche les prochains, et un profil montre les événements que cette personne organise. Personne n’est notifié d’un nouvel événement : vous en prenez connaissance grâce à ces listes, à une invitation ou à un lien partagé par quelqu’un.

Chaque événement affiche sa date dans votre fuseau horaire. Lorsque l’événement se situe dans un autre fuseau horaire, son heure locale et le lieu qui donne son nom à ce fuseau suivent, par exemple « sam., 9:00 – 11:00 PM (Tokyo) ». Un événement dont les organisateurs choisissent encore parmi des dates candidates indique « Date à définir ».

<!-- source: answering-an-event 7601baf062e3 -->
## Répondre à un événement

Ouvrez un événement et choisissez « Je participe », « Peut-être » ou « Je ne participe pas » sous « Votre réponse ». La réponse est enregistrée immédiatement, et vous pouvez la modifier jusqu’au début de l’événement. Répondre « Je participe » ou « Peut-être » ajoute votre nom à la liste des « Participants », que lisent toutes les personnes qui peuvent voir l’événement. Lorsque l’événement a une capacité et qu’il est complet, vous pouvez tout de même répondre « Peut-être ».

« Ajouter à l’agenda (.ics) » enregistre l’événement sous forme de fichier à importer dans votre application d’agenda. Il ne contient que les lieux que vous pouvez voir.

<!-- source: choosing-a-date-together 2712560194b1 -->
## Choisir une date ensemble

Un événement dont la date n’est pas encore fixée a un « Sondage de dates » : un tableau des dates candidates, une par ligne, avec le nombre de personnes « Disponible », ayant répondu « Peut-être » et « Indisponible ». Choisissez votre réponse dans chaque ligne sous « Votre réponse » ; elle est enregistrée immédiatement, et vous pouvez la modifier jusqu’à ce que la date soit fixée. Votre nom et vos réponses sont visibles de toutes les personnes qui peuvent voir l’événement, et « Qui a répondu » les liste pour une date. Vous pouvez répondre « Je participe » sous « Votre réponse » pour l’événement une fois sa date fixée.

L’organisateur et les co-organisateurs ajoutent une date avec « Ajouter une date candidate », en « Retirer » une (un sondage en garde au moins une) et choisissent « Fixer cette date » pour fixer la date de l’événement. Le sondage cesse alors d’accepter des réponses, et les personnes qui y ont répondu ainsi que les personnes invitées sont notifiées.

<!-- source: places 142a326683d9 -->
## Lieux

Un événement peut avoir un lieu pour se retrouver en personne, un lien en ligne (une diffusion ou une réunion sur un autre site) et un salon vocal sur ce serveur. Par défaut, le lieu physique et le lien en ligne ne sont montrés qu’aux participants que les organisateurs approuvent ; d’ici là, la page indique que le lieu est montré aux participants approuvés, et le lieu n’est pas envoyé du tout à votre navigateur. Un salon vocal est montré à toutes les personnes qui peuvent voir l’événement, et s’ouvre au début de celui-ci : dès lors, « Rejoindre le salon vocal » sur la page de l’événement ouvre le salon, où vous rejoignez l’appel. Si le salon n’a pas pu s’ouvrir, la page en indique la raison.

Les liens vers des cartes, des réunions en ligne et les liens d’une description s’ouvrent dans un nouvel onglet et nomment le site vers lequel ils mènent : ils quittent ce serveur, qui ne les vérifie jamais.

<!-- source: organizing-an-event d6a7b15b234f -->
## Organiser un événement

Choisissez « Nouvel événement » dans la liste des événements. L’éditeur demande :

- **Titre**, et une **Description** facultative (texte brut ; ses liens deviennent cliquables).
- **Fuseau horaire de l’événement**, **Début** et **Fin**. Les heures sont celles du fuseau horaire de l’événement, qui est au départ le vôtre. Un événement dure au plus 7 jours et commence dans l’année. Pour laisser les gens choisir la date, sélectionnez « Demander aux personnes de répondre à des dates candidates » et saisissez à la place jusqu’à 20 candidates, chacune avec son début et sa fin.
- **Lieux** : « Se retrouver sur place » (un nom, une adresse et un lien de carte facultatif) et « Lien en ligne » (un nom et une URL), ainsi que **Qui voit le lieu et le lien** : « Uniquement les participants que vous approuvez » ou « Toutes les personnes qui voient l’événement ». Lorsque le chat vocal est activé et que vous pouvez créer des salons, « Salon vocal » ouvre un salon pour l’événement à son début, avec **Qui peut parler sans demander**.
- **Qui voit l’événement** : tout le monde sur ce serveur, les abonnés de l’organisateur, les personnes que l’organisateur suit, les abonnés mutuels de l’organisateur, ou uniquement les personnes invitées. Les personnes que vous invitez et celles qui ont répondu le voient toujours.
- **Capacité**, le nombre de personnes qui peuvent répondre « Je participe » ; laissez vide pour ne pas fixer de limite.

Sur la page de l’événement, l’organisateur et les co-organisateurs approuvent les participants (« Approuver » et « Révoquer l’approbation »), invitent des personnes de ce serveur sous « Invitations » et modifient l’événement avec « Modifier l’événement ». L’organisateur seul change qui voit l’événement, nomme jusqu’à trois « Co-organisateurs » et annule l’événement avec « Annuler l’événement », éventuellement en indiquant un motif que l’événement affiche ensuite. Un événement annulé reste lisible mais n’accepte plus de réponses.

Les modérateurs du serveur peuvent aussi annuler un événement ; sa page l’indique alors, avec leur motif. Certains serveurs ne laissent que certaines personnes créer des événements : lorsque vous n’y êtes pas autorisé, la création affiche « Vous ne pouvez pas créer d’événements sur ce serveur ».

Si vous avez répondu « Je participe » ou « Peut-être », des rappels arrivent un jour et une heure avant le début de l’événement. Pour toutes les notifications d’événements (invitations, modifications, annulations et rappels), consultez [Notifications](notifications.md).
