<!-- source: federation-relays bbd5bb98dde3 -->
# Relais de fédération

Un relais est un serveur qui transmet les publications publiques des serveurs qui l’ont rejoint aux autres serveurs inscrits. Même une petite instance qui a peu d’abonnements obtient un fil fédéré et des fils de hashtags bien remplies, et ses propres publications publiques portent plus loin. Une instance qui ne rejoint aucun relais ne gagne ni lignes ni tâches.

<!-- source: join-a-relay 147b07e65534 -->
## Rejoindre un relais

Utilisez « Relais » dans la zone d’administration (`/admin/relays`). Il faut la permission `manage_settings`, que le rôle intégré `admin` possède. « Rejoindre un relais » ouvre le formulaire qui en enregistre un. La liste affiche l’état de chaque relais avec un bouton pour le quitter ou le rejoindre à nouveau ; choisir un relais ouvre sa page, avec ses adresses, ce qu’il a apporté durant les dernières 24 heures et le bouton qui le supprime. La CLI est `my admin relay` (`my admin relay -help`) :

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. Enregistrez l’URL du relais. Il existe deux protocoles :
   - **Mastodon** : l’**URL de la boîte de réception** du relais (par exemple `https://relay.example/inbox`). De nombreux relais, dont pub-relay et Activity-Relay, l’acceptent.
   - **LitePub** : l’**URL de l’acteur** du relais (par exemple `https://relay.example/relay`). C’est le protocole des relais Pleroma et Akkoma.
   Si vous ne choisissez pas de protocole (« Détecter automatiquement »), le serveur utilise LitePub lorsque l’URL sert un document d’acteur, et Mastodon sinon.
2. Après l’enregistrement, l’acteur de l’instance envoie un `Follow` au relais, et l’état est « En attente de la réponse du relais » (`pending`). Lorsque le relais accepte, l’état devient « Rejoint » (`accepted`) ; lorsqu’il refuse, « Rejeté par le relais » (`rejected`). Certains relais mettent du temps à répondre, car un opérateur approuve chaque demande à la main.
3. Si vous désactivez « Recevoir les publications du relais » lors de l’enregistrement, le serveur ne fait qu’envoyer. Pour changer cela, quittez le relais et enregistrez-le à nouveau.

Vous ne pouvez pas enregistrer le relais d’un serveur soumis à une action de domaine (`suspend`). Si le serveur d’un relais rejoint est suspendu par la suite, l’état ne change pas, mais rien n’est envoyé à ce relais ni reçu de lui, et la zone d’administration affiche un avertissement.

<!-- source: what-the-server-sends 1db52c508f9c -->
## Ce que le serveur envoie

- Seuls la création, la modification et la suppression de publications **publiques** (`public`) locales sont envoyées. Les publications `unlisted`, réservées aux abonnés, directes ou locales uniquement (`local`), ainsi que les boosts, ne sont jamais envoyés. Pour garder une publication hors des relais, publiez-la en `unlisted` ou en `local`. Il n’existe pas de refus par utilisateur.
- Les documents des publications publiques portent une signature Linked Data (`RsaSignature2017`), afin que Mastodon et Misskey puissent accepter des publications qu’un relais transmet telles quelles.
- Le nombre de livraisons augmente du nombre de relais rejoints multiplié par le nombre de publications publiques (les livraisons vers un relais sont regroupées en une seule).

<!-- source: what-the-server-receives fa99c4ec4217 -->
## Ce que le serveur reçoit

- Une publication publique portée par un relais rejoint est récupérée de nouveau depuis son origine et vérifiée avant d’être ingérée. Elle s’affiche ensuite dans le fil fédéré et les fils de hashtags, et dans les fils d’accueil des personnes qui suivent l’un de ses hashtags. Elle ne s’affiche dans aucun autre fil d’accueil et ne crée aucune notification.
- Au plus `relay_max_activities_per_hour` publications par heure (1 000 par défaut) sont ingérées par relais. Tout ce qui dépasse la limite est écarté sans être récupéré.
  La page du relais dans la zone d’administration et `my admin relay list` indiquent combien ont été ingérées au cours des 24 dernières heures et combien ont été écartées au-delà de la limite. Si beaucoup sont écartées, relevez la limite ou reconsidérez le relais.
- Une publication n’est pas ingérée lorsque son domaine ou son compte fait l’objet d’un `suspend` (une restriction de fédération) ou d’un `limit` (une restriction qui la retire des lieux publics).
- Une publication arrivée uniquement par un relais est retirée de la vue publique après le paramètre d’instance `relay_status_retention_days` (7 par défaut). Une publication à laquelle une personne locale a répondu, qu’elle a boostée, mise en favori ou en marque-page, ainsi que les publications d’auteurs qu’une personne locale suit, reçoivent la même conservation que les autres publications distantes (`remote_status_retention_days`).
- L’ingestion coûte une récupération depuis l’origine par publication (plus une récupération de l’acteur pour un auteur inconnu). À la limite de 1 000 par heure, cela représente au plus 24 000 récupérations par jour.

Les réglages se trouvent dans la [référence de configuration](../reference/configuration.md#relay_max_activities_per_hour). Le résultat de l’ingestion est aussi exporté sous la métrique `my.relay.activities` (voir [Exécuter le serveur](running.md)).

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## Quitter, rejoindre à nouveau et supprimer

- « Quitter » envoie un `Undo` du `Follow` et fait passer l’état à « Quitté » (`left`). Rien n’est ensuite envoyé au relais ni reçu de lui. Les publications déjà ingérées sont retirées à la fin de leur durée de conservation.
- « Rejoindre à nouveau » envoie un nouveau `Follow` à un relais qui a rejeté le serveur ou que le serveur a quitté.
- « Supprimer » retire l’enregistrement d’un relais qui a été quitté ou rejeté. Les modifications et suppressions de publications déjà envoyées au relais continuent de lui être livrées après sa suppression.

L’enregistrement, l’adhésion, le départ et la suppression, ainsi que l’acceptation ou le refus du relais, sont consignés dans le journal d’audit avec le type de cible « Relais » (l’acceptation et le refus comme actions distantes).
