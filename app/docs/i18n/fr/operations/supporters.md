<!-- source: supporters-paid-plans ea0039323475 -->
# Soutiens (formules payantes)

Uniquement lorsqu’un administrateur l’active, les utilisateurs locaux peuvent souscrire à une formule payante sur le web et conserver le rôle associé à la formule tant qu’ils continuent de payer. Le programme est désactivé par défaut, et aucune formule ni aucun point d’entrée de souscription n’apparaît nulle part tant qu’il n’est pas activé.
La clé de déploiement `payments` est décrite dans la [référence de configuration](../reference/configuration.md#payments).

**mycroblog ne garantit la conformité à aucune loi, y compris la loi japonaise sur les transactions commerciales spécifiées.** Les opérateurs doivent vérifier le contenu des mentions commerciales, l’écran de confirmation finale et le traitement de la taxe sur la consommation et des factures au regard des lois de leur propre pays (voir « Ce que les opérateurs doivent vérifier » ci-dessous).

<!-- source: how-a-subscription-works 13d7fb3772b2 -->
## Fonctionnement d’une souscription

![Le déroulement d’une souscription : l’utilisateur paie chez le fournisseur de paiement, les événements du fournisseur font relire l’état par le serveur, et le serveur accorde ou retire le rôle de la formule et enregistre le revenu. Les étapes sont listées ci-dessous.](images/supporter-flow.svg)

Du haut du schéma vers le bas :

- **Souscrire.** L’utilisateur choisit une formule et une périodicité (mensuelle ou annuelle) sur `/settings/supporter`. Après la confirmation finale, le serveur (`my serve`) crée la page de paiement chez le fournisseur défini par `payments` (`stripe` ou `payjp`), et l’utilisateur paie sur cette page.
- **Événements.** Le fournisseur envoie ses événements au point de terminaison de webhook `POST /payments/webhooks/{provider}`, qui enregistre chaque événement une seule fois et met une tâche en file. Un événement n’est qu’un signal : le serveur ne tire jamais l’état de son contenu.
- **Relecture.** La tâche `supporters.apply_payment_event`, et le rapprochement `supporters.reconcile_payments` toutes les `payments_reconcile_interval`, relisent la souscription et les mouvements d’argent depuis l’API du fournisseur. Le rapprochement répare tout événement qui n’est pas arrivé.
- **L’état et le rôle.** Le serveur déduit l’état de la souscription de ce qu’il a lu et de chaque échéance passée (le délai de grâce de 7 jours, par exemple), et accorde ou retire avec lui le rôle de la formule.
- **Revenus.** Les mouvements d’argent deviennent les enregistrements de revenus (montant brut, frais et net de chaque paiement, remboursement, rétrofacturation et frais). Leurs totaux sont affichés sur l’écran des finances (`/finance`, pour les personnes ayant `view_finance`) et, dans la mesure où `income_visibility` le permet, sur `/about/costs` (voir [instance-settings.md](instance-settings.md)).

![Les états d’une souscription : active, canceling et past_due conservent le rôle de la formule ; pending, lapsed, expired, revoked et abandoned ne le conservent pas. Les transitions sont listées ci-dessous.](images/supporter-states.svg)

Les états d’une souscription, tels que les affichent l’administration et `my admin supporters subscriptions` :

- `pending` : démarré, pas encore payé. Aucun rôle. Le premier paiement le fait passer à `active`, et une souscription qui se termine avant son premier paiement devient `abandoned`.
- `active` : payé. Il conserve le rôle de la formule, que chaque renouvellement maintient.
- `past_due` : un paiement a échoué. Il conserve le rôle pendant 7 jours à compter de l’échec ; un paiement dans ce délai le fait repasser à `active`.
- `lapsed` : 7 jours se sont écoulés après l’échec. Le rôle est retiré. Un paiement après cela le fait repasser à `active`, et le rôle revient.
- `canceling` : annulé à la fin de la période. Il conserve le rôle jusque-là, puis devient `expired`.
- `expired`, `revoked` (un remboursement intégral ou une rétrofacturation) et `abandoned` sont définitifs et ne conservent aucun rôle.

<!-- source: enabling-the-program fe86b2e59df5 -->
## Activer le programme

1. Définissez la clé de déploiement `payments` sur votre fournisseur de paiement et transmettez les clés du fournisseur et le secret de webhook. Les étapes propres à chaque fournisseur se trouvent dans [payments-stripe.md](payments-stripe.md) et [payments-payjp.md](payments-payjp.md). Le programme ne peut pas être activé tant que la clé vaut `none`.
2. Configurez le serveur pour qu’il envoie des e-mails (`mail` autre que `none`). Seuls les utilisateurs ayant une adresse e-mail confirmée peuvent souscrire ; un serveur qui n’envoie pas d’e-mails ne peut donc pas activer le programme.
3. Sur la page « Mentions commerciales » de « Conditions et politiques » dans l’administration (`/admin/documents/commerce-disclosure`), rédigez en Markdown les mentions commerciales (pour le Japon, l’avis exigé par la loi sur les transactions commerciales spécifiées), le texte d’instance `commerce_disclosure`. Il est publié sur `/about/commerce`, et vous pouvez enregistrer cette URL dans le tableau de bord du fournisseur.
   Le programme peut être activé avec le texte vide, mais l’administration affiche un avertissement.
4. Dans « Rôles » dans l’administration, créez le rôle à associer à la formule. Il ne doit avoir aucun titulaire, seulement des permissions de membre (groupe `member`), et une `position` inférieure à celle de tous les rôles du personnel, `moderator` compris.
   Les permissions de membre sont les avantages qu’une formule peut offrir : `create_custom_emojis` (demander des émojis personnalisés), `view_finance` (lire les finances du serveur), `start_voice_rooms` (ouvrir des salons de chat vocal tant que seuls les utilisateurs autorisés le peuvent), `create_invites` (inviter des personnes tant que seuls les utilisateurs autorisés le peuvent, dans les limites des [invitations de membres](registration.md#member-invites)) et `keep_posts_longer` (conserver les publications au-delà du [maximum de la suppression automatique](instance-settings.md#automatic-deletion-of-old-posts), et désactiver la suppression automatique tant que le maximum est activé).
   Lorsque la souscription se termine, le rôle disparaît, et les avantages aussi : les invitations créées par le soutien cessent de fonctionner jusqu’à ce qu’il souscrive de nouveau, et le maximum de la suppression automatique s’applique de nouveau à ses publications après son délai de grâce.
5. Dans « Soutiens » dans l’administration (permission `manage_supporters`), ouvrez la page « Formules » (`/admin/supporters/plans`) et choisissez « Créer une formule ». Définissez le nom, la description, le prix mensuel, la remise pour un paiement annuel, la limite mensuelle de création d’émojis personnalisés (elle est remise à zéro le premier jour de chaque mois, en UTC ; voir [émojis personnalisés](instance-settings.md#custom-emoji)), l’ordre et la visibilité, et choisissez le rôle.
   Le serveur crée les prix mensuel et annuel chez le fournisseur (ne créez pas de prix dans le tableau de bord du fournisseur). Tant que les prix n’existent pas, la formule est « en préparation » et n’accepte aucune souscription.
6. Sur la page « Programme » de « Soutiens » (`/admin/supporters`), cochez « Activer le programme de soutien » et choisissez « Enregistrer ». Le même formulaire définit qui peut lire la liste des soutiens.

Dès qu’une formule existe, son rôle ne peut plus être accordé ni retiré à la main, et vous modifiez ses permissions et sa position sur la page de la formule (choisissez la formule sur la page « Formules »). Une formule supérieure (un ordre plus grand) se situe au-dessus des rôles des formules inférieures, a toutes leurs permissions et a une limite d’émojis au moins égale à la leur.

<!-- source: setup-at-the-provider 74efcb74cf80 -->
## Configuration chez le fournisseur

- **Désactivation** : un souscripteur ne peut pas désactiver son compte tant que la souscription est active, y compris une souscription programmée pour se terminer à la fin de sa période. L’écran des paramètres le conduit vers la souscription, afin que personne ne soit facturé tout en étant masqué.
- **Nouvelles tentatives (relance)** : sept jours après l’échec d’un paiement, le serveur retire le rôle (indépendamment des nouvelles tentatives du fournisseur). Réglez la période de nouvelles tentatives du fournisseur sur au moins 7 jours et faites de l’action finale « annuler » (« cancel »). Lorsque le paiement réussit après le délai de grâce, le rôle revient.
- **Portail client** : le portail est ouvert pour les moyens de paiement et les factures. Désactivez les changements de formule dans le portail et n’autorisez que l’annulation à la fin de la période. Les changements de formule et les annulations se font sur les écrans propres au serveur.
- **Remboursements** : le serveur ne rembourse qu’un paiement qui n’a rien acheté : une seconde souscription pour un même paiement, et un paiement pour une souscription en attente qui s’est fermée. Le guide du fournisseur ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) décrit les deux. Tout autre remboursement relève de l’opérateur, dans le tableau de bord du fournisseur. Un remboursement intégral et une rétrofacturation retirent immédiatement le rôle et annulent immédiatement la souscription chez le fournisseur. Un remboursement partiel ne retire pas le rôle. Gagner une rétrofacturation ne rétablit pas le rôle ; l’utilisateur souscrit de nouveau.

<!-- source: what-users-see d1216c59368a -->
## Ce que voient les utilisateurs

- Sur le web, un utilisateur ouvre « Formules de soutien » (`/settings/supporter`), choisit une formule et une périodicité de paiement (mensuelle ou annuelle), passe par un écran de confirmation finale, puis accède à la page de paiement du fournisseur. Les données de carte ne transitent jamais par ce serveur. La page de paiement revient à cet écran, qui relit alors l’état de la souscription.
  Une fois souscripteur, l’utilisateur peut changer de formule depuis le même écran (une montée en gamme et un passage de mensuel à annuel s’appliquent une fois leur différence au prorata payée, tandis qu’une descente en gamme et un passage d’annuel à mensuel s’appliquent à la fin de la période), annuler un changement programmé, annuler la souscription (les avantages durent jusqu’à la fin de la période) et annuler une annulation, et ouvrir le portail client pour les moyens de paiement et les factures.
  Lorsque le paiement d’un changement échoue, ou que la carte demande une authentification que le changement ne peut pas demander, la formule reste en place et l’utilisateur voit une erreur avec la marche à suivre pour mettre à jour la carte. Cela ne démarre pas le délai de grâce et n’envoie pas la notification d’échec de paiement ; un renouvellement qui échoue, lui, le fait.
- Seuls les utilisateurs de ce serveur qui ont une adresse e-mail confirmée et un compte ni suspendu ni programmé pour suppression peuvent souscrire. Un utilisateur ne peut détenir qu’une formule à la fois.
- `/about` affiche une section avec les formules publiques. Dans l’application Tauri, l’écran de souscription, la section des formules de `/about` et les liens vers eux n’apparaissent pas. Les avantages (badge, émojis personnalisés, finances) fonctionnent aussi dans l’application.

<!-- source: notifications-to-subscribers 91f6ad77e546 -->
## Notifications aux souscripteurs

Les souscripteurs sont informés une fois de chacun des événements suivants, par notification, Web Push et e-mail (à l’adresse principale, dans la langue de l’utilisateur). Un souscripteur dont le compte est désactivé n’est informé de rien, ni à ce moment-là ni à son retour.

- Échec de paiement (`supporter_payment_failed`) : lorsque la souscription devient en retard de paiement (`past_due`). Elle indique la fin du délai de grâce (en UTC dans l’e-mail) et un lien vers l’écran où le moyen de paiement peut être corrigé (`/settings/supporter`).
- Fin de la souscription (`supporter_subscription_ended`) : lorsqu’une souscription quitte un état avec avantages parce que le délai de grâce s’est écoulé, qu’il a expiré, ou qu’il s’est terminé par un remboursement ou une rétrofacturation. Le message indique la raison. Une fin causée par la suppression du compte n’est pas annoncée.
- Retrait d’une formule (`supporter_plan_retired`) : aux souscripteurs d’une formule retirée (sauf celles en attente). Elle indique que la souscription ne sera pas renouvelée.
- Annonce de la fin du service (`supporter_service_closure`) : aux souscripteurs dont la souscription a été annulée à la fin de sa période lorsque l’annonce a commencé (sauf celles en attente). Elle indique que la souscription ne sera pas renouvelée. Aucune nouvelle souscription n’est acceptée pendant l’annonce.

Le texte des notifications Web Push et des notifications ne mène pas à l’écran de paiement. Seul le client web met dans une notification un lien vers l’écran de souscription ; l’application Tauri ne le fait pas. Les liens dans les e-mails ouvrent le client web dans un navigateur.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` réalise une partie des opérations de l’administration (`my admin supporters -help`).

- `status` : si le programme est activé, ce qu’il lui faut avant de pouvoir l’être (un fournisseur, l’envoi d’e-mails) et le nombre de souscriptions en cours.
- `enable`, `disable` : activent et désactivent le programme. La désactivation est refusée tant qu’une souscription est en cours.
- `plans` : la liste des formules (y compris les formules retirées), avec le nombre de formules en préparation et retirées et de souscriptions.
- `subscriptions` : la liste des souscriptions (filtrez avec `-plan` et `-state`). `cancel <subscription ID>` annule une souscription immédiatement (sans remboursement, sauf pour un paiement de souscription en attente, qui n’a rien acheté).

L’activation, la désactivation et l’annulation sont écrites dans le journal d’audit comme des opérations CLI. La création, la modification et le retrait de formules se font dans l’administration ou par l’API d’administration.

<!-- source: the-list-of-members f00ca3b391a3 -->
## La liste des membres

- Le paramètre d’instance `supporter_list_visibility` (sur l’écran « Soutiens » ; voir [instance-settings.md](instance-settings.md)) choisit qui peut voir la liste des souscripteurs actuels (`/about/supporters`). Avec la valeur par défaut `off`, ni la liste ni le lien depuis `/about` n’apparaissent.
- La liste n’affiche que les souscripteurs qui ont choisi d’y figurer sur la page « Badges et liste des soutiens » de leurs paramètres (personne n’y figure par défaut). Elle affiche le nom de la formule et l’avatar, le nom d’affichage et l’identifiant de chaque personne, et jamais les montants, le fait que le paiement soit mensuel ou annuel, les dates ni les totaux.
- Le choix de figurer dans la liste est conservé lorsque le souscripteur change de formule, et disparaît lorsque la souscription se termine (le rôle est retiré). Une personne qui souscrit de nouveau repart sans figurer dans la liste.
- La page de la liste et son API envoient toujours `noindex, noarchive`, quoi que dise le paramètre des moteurs de recherche (`search_indexing`).

<!-- source: changing-prices-hiding-plans-and-retiring-plans a65c92b1c371 -->
## Modifier les prix, masquer des formules et en retirer

- Lorsque vous modifiez le prix mensuel ou la remise, le nouveau prix s’applique aux nouvelles souscriptions et aux changements de formule. Les souscripteurs existants se renouvellent à leur prix d’origine. Il n’existe aucune opération pour faire passer les souscripteurs existants à un nouveau prix. Pour les faire passer, créez une nouvelle formule et retirez l’ancienne.
- Masquer une formule arrête les nouvelles souscriptions et les changements vers cette formule. Les souscriptions existants se poursuivent.
- Le retrait d’une formule ne peut pas être annulé. Ses souscriptions sont annulées à la fin de leur période ((celles en attente se ferment immédiatement) et leur page de paiement cesse d’accepter des paiements, tout paiement qui arrive encore étant remboursé ; et celles dont le paiement est en retard au-delà du délai de grâce sont annulés immédiatement). Lorsque le dernière souscription se termine, le rôle redevient un rôle ordinaire.

<!-- source: accounts-and-subscriptions 8c5e10df3c18 -->
## Comptes et souscriptions

- Supprimer un compte et le bannir (ce qui programme sa suppression) annulent immédiatement ses souscriptions, sans remboursement (un paiement pour une souscription encore en attente n’a rien acheté, et est remboursé). Annuler un bannissement ne rétablit pas les souscriptions.
- Suspendre un compte programme l’annulation à la fin de la période. Lever la suspension ne la rétablit pas automatiquement ; l’utilisateur annule l’annulation avant la fin de la période ou souscrit de nouveau.
- Sur la page « Souscriptions » de « Soutiens » (`/admin/supporters/subscriptions`), un administrateur peut ouvrir une souscription et y mettre fin immédiatement avec « Terminer maintenant » (sans remboursement, sauf pour un paiement de souscription en attente, qui n’a rien acheté ; l’opération est écrite dans le journal d’audit).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Désactiver le programme et changer de fournisseur

- Le programme ne peut pas être désactivé tant qu’une souscription est en cours (en attente, active, programmée pour être annulée, en retard de paiement ou au-delà du délai de grâce). Retirez d’abord toutes les formules et désactivez le programme une fois les souscriptions terminés.
- Pour changer de fournisseur, désactivez le programme, modifiez `payments`, et réactivez le programme avec le nouveau fournisseur. Les souscripteurs existants souscrivent de nouveau. Tant que le fournisseur configuré diffère de celui avec lequel le programme a été activé, le serveur refuse les nouvelles souscriptions et les changements, et l’administration affiche un avertissement.

<!-- source: what-operators-must-check-not-yet-verified 659a7e51f9d3 -->
## Ce que les opérateurs doivent vérifier (non encore vérifié)

mycroblog ne juge aucun de ces points. Vérifiez-les au regard des lois de votre pays et des conditions de votre fournisseur.

- Les éléments que les mentions commerciales doivent contenir (le nom, l’adresse et le numéro de téléphone du vendeur, le prix, le moment et le mode de paiement, le moment où le service est fourni, la façon de résilier, etc.) et la façon dont elles doivent être publiées.
- Que l’écran de confirmation finale (article 12-6 de la loi sur les transactions commerciales spécifiées) affiche la quantité, le prix, le moment du paiement, le moment de la livraison, ainsi que la façon d’annuler et son effet, en tenant ensemble la page de paiement du fournisseur et l’écran de confirmation de `my`.
- Le traitement de la taxe sur la consommation, et si vous émettez des factures qualifiées.
