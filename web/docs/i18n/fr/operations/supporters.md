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

<!-- source: enabling-the-program b07e6c07d73b -->
## Activer le programme

1. Définissez la clé de déploiement `payments` sur votre fournisseur de paiement et transmettez les clés du fournisseur et le secret du webhook. Les étapes propres à chaque fournisseur se trouvent dans [payments-stripe.md](payments-stripe.md) et [payments-payjp.md](payments-payjp.md). Le programme ne peut pas être activé tant que la clé vaut `none`.
2. Configurez le serveur pour qu’il envoie des e-mails (`mail` autre que `none`). Seuls les utilisateurs avec une adresse e-mail confirmée peuvent s’abonner ; un serveur qui n’envoie pas d’e-mails ne peut donc pas activer le programme.
3. Sur la page « Mentions de commerce » de « Conditions et politiques » dans l’administration (`/admin/documents/commerce-disclosure`), rédigez en Markdown les mentions de commerce (pour le Japon, l’avis exigé par la loi sur les transactions commerciales spécifiées), le texte d’instance `commerce_disclosure`. Il est publié à `/about/commerce`, et vous pouvez enregistrer cette URL dans le tableau de bord du fournisseur.
   Le programme peut être activé avec le texte vide, mais l’administration affiche un avertissement.
4. Dans « Rôles » dans l’administration, créez le rôle à associer à la formule. Il ne doit avoir aucun titulaire, uniquement des permissions de membre (groupe `member`), et une `position` inférieure à celle de tout rôle du personnel, `moderator` compris.
   Les permissions de membre sont les avantages qu’une formule peut donner : `create_custom_emojis` (demander des émojis personnalisés), `view_finance` (lire les finances du serveur), `start_voice_rooms` (ouvrir des salons de chat vocal alors que seuls les utilisateurs autorisés le peuvent), `create_invites` (inviter des personnes alors que seuls les utilisateurs autorisés le peuvent, dans les limites des [invitations des membres](registration.md#member-invites)), `keep_posts_longer` (conserver des publications au-delà du [maximum de la suppression automatique](instance-settings.md#automatic-deletion-of-old-posts), et désactiver la suppression automatique alors que le maximum est actif), `create_articles` (écrire des articles alors que seuls les utilisateurs autorisés le peuvent ; voir [articles](instance-settings.md#articles)) et `create_events` (créer des [événements](instance-settings.md#events) alors que seuls les utilisateurs autorisés le peuvent).
   Lorsque l’abonnement prend fin, le rôle disparaît, et les avantages avec lui : les invitations que le soutien a créées cessent de fonctionner jusqu’à ce qu’il s’abonne de nouveau, et le maximum de la suppression automatique s’applique de nouveau à ses publications après son délai de grâce.
5. Dans « Soutiens » dans l’administration (permission `manage_supporters`), ouvrez la page « Formules » (`/admin/supporters/plans`) et choisissez « Créer une formule ». Définissez le nom, la description, le prix mensuel, la remise pour un paiement annuel, la limite mensuelle de création d’émojis personnalisés (elle se réinitialise le premier jour de chaque mois, UTC ; voir [émojis personnalisés](instance-settings.md#custom-emoji)), l’ordre et la visibilité, et choisissez le rôle : le formulaire liste les rôles qui peuvent recevoir une formule, ou, sans la permission `manage_roles`, demande la clé du rôle.
   Le serveur crée les prix mensuel et annuel chez le fournisseur (ne créez pas de prix dans le tableau de bord du fournisseur). Tant que les prix n’existent pas, la formule est « en préparation » et n’accepte aucun abonnement.
6. Sur la première page de « Soutiens » (`/admin/supporters`), choisissez « Activer le programme ». La page indique ce qui manque tant que le programme ne peut pas être activé, et affiche le fournisseur de paiement de la configuration, que seule la configuration change.

Dès qu’une formule existe, son rôle ne peut plus être attribué ni révoqué à la main, et vous modifiez ses permissions et sa position sur la page de la formule (choisissez la formule sur la page « Formules »). Une formule plus élevée (un ordre plus grand) se place au-dessus des rôles des formules inférieures, a toutes leurs permissions et a une limite d’émojis au moins égale à la leur.

<!-- source: setup-at-the-provider 5821be7cae27 -->
## Configuration chez le fournisseur

- **Désactivation** : un abonné ne peut pas désactiver son compte tant que l’abonnement est actif, y compris un abonnement qui doit se terminer à la fin de sa période. L’écran des paramètres le mène à l’abonnement, afin que personne ne soit facturé alors qu’il est masqué.
- **Nouvelles tentatives (relances de paiement)** : sept jours après l’échec d’un paiement, le serveur retire le rôle (indépendamment des nouvelles tentatives du fournisseur). Définissez la période de nouvelles tentatives du fournisseur à au moins 7 jours et réglez l’action finale sur « annuler ». Lorsque le paiement réussit après le délai de grâce, le rôle revient.
- **Vérifier le fournisseur** : la section « Fournisseur de paiement » de la première page de « Soutiens » affiche le mode du fournisseur (test ou production), la dernière livraison de webhook et la dernière réconciliation avec la raison de son arrêt, et avertit lorsque le webhook semble non configuré. « Réconcilier maintenant » lance aussitôt une réconciliation, au plus une fois par heure.
- **Portail client** : le portail est ouvert pour les moyens de paiement et les factures. Désactivez les changements de formule dans le portail et n’autorisez que l’annulation à la fin de la période. Les changements de formule et les annulations se font sur les propres écrans du serveur.
- **Remboursements** : le serveur ne rembourse qu’un paiement qui n’a rien acheté : un second abonnement pour un même paiement, et un paiement pour un abonnement en attente qui s’est clos. Le guide du fournisseur ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) décrit les deux. Tout autre remboursement incombe à l’opérateur, dans le tableau de bord du fournisseur. Un remboursement total et une contestation de paiement retirent aussitôt le rôle et annulent aussitôt l’abonnement du fournisseur. Un remboursement partiel ne retire pas le rôle. Gagner une contestation de paiement ne rétablit pas le rôle ; l’utilisateur s’abonne de nouveau.

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

<!-- source: the-list-of-members df4c915636dc -->
## La liste des membres

- Le paramètre d’instance `supporter_list_visibility` (la section « Liste des soutiens » de la première page de « Soutiens », pour un administrateur avec `manage_settings` ; voir [instance-settings.md](instance-settings.md)) choisit qui peut voir la liste des abonnés actuels (`/about/supporters`). Avec la valeur par défaut `off`, ni la liste ni le lien depuis `/about` n’apparaissent.
- La liste n’affiche que les abonnés qui ont choisi d’y figurer sur la page « Badges et liste des soutiens » de leurs paramètres (personne n’y figure par défaut). Elle affiche le nom de la formule et l’avatar, le nom d’affichage et l’identifiant de chaque personne, et jamais les montants, le caractère mensuel ou annuel du paiement, les dates ou les nombres.
- Le choix d’y figurer subsiste lorsque l’abonné change de formule, et disparaît lorsque l’abonnement prend fin (le rôle est retiré). Une personne qui s’abonne de nouveau commence sans y figurer.
- La page de la liste et son API envoient toujours `noindex, noarchive`, quoi que dise le paramètre des moteurs de recherche (`search_indexing`).

<!-- source: changing-prices-hiding-plans-and-retiring-plans fdeeb52e52bb -->
## Changer les prix, masquer des formules et retirer des formules

- Lorsque vous changez le prix mensuel ou la remise, le nouveau prix s’applique aux nouveaux abonnements et aux changements de formule. Les abonnés existants renouvellent à leur prix d’origine. Il n’existe aucune opération pour faire passer les abonnés existants à un nouveau prix. Pour les faire passer, créez une nouvelle formule et retirez l’ancienne.
- Masquer une formule arrête les nouveaux abonnements et les changements vers cette formule. Les abonnements existants continuent.
- Retirer une formule (« Retirer la formule… » sur sa page, après avoir saisi son nom) est irréversible. Ses abonnements sont annulés à la fin de leur période (ceux en attente se clôturent aussitôt et leur page de paiement cesse d’accepter des paiements, tout paiement qui arrive quand même étant remboursé ; et ceux dont le paiement est en retard au-delà du délai de grâce sont annulés aussitôt). Lorsque le dernier abonnement prend fin, le rôle redevient un rôle ordinaire.

<!-- source: accounts-and-subscriptions 480ba9a583b1 -->
## Comptes et abonnements

- Supprimer un compte et le bannir (ce qui programme sa suppression) annulent aussitôt ses abonnements, sans remboursement (un paiement pour un abonnement encore en attente n’a rien acheté, et est remboursé). Annuler un bannissement ne rétablit pas les abonnements.
- Suspendre un compte programme l’annulation à la fin de la période. Lever la suspension ne la rétablit pas automatiquement ; l’utilisateur annule l’annulation avant la fin de la période ou s’abonne de nouveau.
- Sur la page « Abonnements » de « Soutiens » (`/admin/supporters/subscriptions`, filtrée par formule et par état), un administrateur peut ouvrir un abonnement et y mettre fin aussitôt avec « Terminer maintenant… », après avoir saisi l’identifiant de l’abonné (pas de remboursement, sauf pour un paiement d’abonnement en attente, qui n’a rien acheté ; cela est écrit dans le journal d’audit).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Désactiver le programme et changer de fournisseur

- Le programme ne peut pas être désactivé tant qu’une souscription est en cours (en attente, active, programmée pour être annulée, en retard de paiement ou au-delà du délai de grâce). Retirez d’abord toutes les formules et désactivez le programme une fois les souscriptions terminés.
- Pour changer de fournisseur, désactivez le programme, modifiez `payments`, et réactivez le programme avec le nouveau fournisseur. Les souscripteurs existants souscrivent de nouveau. Tant que le fournisseur configuré diffère de celui avec lequel le programme a été activé, le serveur refuse les nouvelles souscriptions et les changements, et l’administration affiche un avertissement.

<!-- source: tips d34b97b3cfbe -->
## Pourboires

Un pourboire est un paiement unique d’un utilisateur local pour le fonctionnement du serveur, sans contrepartie : il ne donne aucun rôle, badge ni autre avantage, et l’utilisateur ne voit qu’un remerciement une fois le paiement effectué. Les pourboires sont désactivés par défaut, n’exigent aucune formule et vont uniquement à l’opérateur : les utilisateurs ne peuvent pas se faire de pourboires entre eux, et il n’y a ni objectifs, ni campagnes, ni soldes.

- **Ce qu’il faut** : le programme de soutien activé sur le fournisseur configuré, et un fournisseur qui accepte les paiements uniques. La section **Pourboires** de la première page de l’écran **Soutiens** (permission `manage_supporters`) indique si les utilisateurs peuvent donner un pourboire maintenant et avertit lorsque le fournisseur n’accepte aucun paiement unique ; `GET /api/v1/admin/supporter_tips` indique la même chose : si votre fournisseur les accepte (`one_time_payments`), le montant minimal et maximal qu’il autorise dans chaque devise (`limits`), et si les utilisateurs peuvent donner un pourboire maintenant (`accepting`). Stripe les accepte (voir [Pourboires avec Stripe](payments-stripe.md#tips)), PAY.JP les accepte sur sa page hébergée de l’API v2 (voir [Pourboires avec PAY.JP](payments-payjp.md#tips)), et le fournisseur de test des constructions de test aussi.
- **Montants** : vous choisissez de 1 à 6 montants dans une seule devise, le yen ou le dollar américain, chacun de ¥100 à ¥100 000 ou de 1 US$ à 1 000 US$ et au moins égal au plus petit débit unique du fournisseur. Les utilisateurs en choisissent un et ne saisissent jamais de montant ; le plancher évite qu’un petit pourboire coûte plus qu’il ne rapporte lorsqu’il est contesté.
- **Activer, modifier et désactiver** : dans la section **Pourboires**, cochez **Accepter les pourboires**, choisissez la devise, écrivez les montants un par ligne et sélectionnez **Enregistrer les pourboires** ; décochez la case et enregistrez pour les désactiver. L’API est `PUT /api/v1/admin/supporter_tips` avec `{"enabled": true, "amounts": [{"amount": "500", "currency": "JPY"}, ...]}`, ou `{"enabled": false}`. Chaque changement est écrit dans le journal d’audit (`supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled`). Désactiver le programme de soutien arrête aussi les pourboires ; un paiement déjà ouvert reste payable jusqu’à son terme, et un paiement sur celui-ci est quand même enregistré.
- **Qui peut donner un pourboire** : les utilisateurs de ce serveur avec une adresse e-mail confirmée dont le compte n’est ni suspendu ni programmé pour suppression. Un abonnement n’est pas nécessaire. Les démarrages sont limités à 5 par heure et 20 par jour pour chaque utilisateur, et à 100 par heure pour tout le serveur, ce qui arrête le test de cartes (l’essai de cartes volées avec de petits paiements) ; `server_limit_reached` indique que la limite du serveur a été atteinte au cours de la dernière heure.
- **Payer** : les utilisateurs accèdent à la page du pourboire (`/settings/supporter/tip`) depuis la section **Pourboires** de la page À propos, sous la jauge de financement dans la colonne latérale, et depuis la page des formules de soutien ; un visiteur qui n’est pas connecté se voit proposer de se connecter d’abord. La page confirme le montant, qu’il est payé une seule fois aussitôt, et que rien n’est donné en retour, avec un lien vers vos mentions de commerce, avant la page du fournisseur. La page hébergée du fournisseur prend le paiement unique, par carte uniquement, et reste payable pendant une heure. Un utilisateur a au plus un pourboire ouvert : en démarrer un autre rend d’abord la page précédente impayable, et lorsque la précédente a été payée entre-temps, l’utilisateur est envoyé vers son remerciement au lieu de payer deux fois. Dans l’application Tauri, les pourboires n’apparaissent pas.
- **Revenus** : un pourboire payé est comptabilisé une fois dans le mois où il a été payé, que ce soit l’événement du fournisseur, le retour depuis la page ou la réconciliation qui le signale en premier. Ses remboursements et ses contestations de paiement sont comptabilisés dans leur propre mois et se soustraient des revenus. Les totaux des administrateurs affichent séparément le net des abonnements, les pourboires et l’autre argent du compte du fournisseur, et leur liste des mouvements d’argent indique qui a payé chaque pourboire tant que le compte existe. Les totaux publics, `/finance` et `/about/costs` n’affichent jamais un pourboire individuel, le montant des pourboires ni leur nombre ; avec `income_visibility` défini sur `plans`, la liste des formules se termine par une ligne « Pourboires » sans nombre.
- **Remboursements et litiges** : le serveur n’a aucune opération de remboursement pour les pourboires. Remboursez dans le tableau de bord du fournisseur ; un remboursement total, une contestation de paiement et un litige gagné sont enregistrés avec le pourboire, et un remboursement partiel seulement dans les revenus. Chaque litige coûte aussi les frais de litige du fournisseur (avec Stripe au Japon, ¥1 500 au moment de la rédaction), quel que soit le montant du pourboire.
- **Comptes** : supprimer un compte, ou le bannir, rend impayable la page de son pourboire ouvert. Effacer le compte supprime ses pourboires ; leurs revenus restent, sans l’utilisateur. Le fournisseur conserve le paiement et l’adresse e-mail qui a prérempli sa page, que le serveur ne peut pas supprimer : dites-le dans votre politique de confidentialité.

La métrique `my.supporter.tip_checkouts` compte les démarrages (`started`), les démarrages que les limites ont refusés (`rate_limited`), et les pourboires qui ont été payés (`paid`) ou se sont terminés sans paiement (`expired`). Deux exemples d’alertes :

- Une rafale de démarrages : `sum(rate(my_supporter_tip_checkouts_total{my_outcome=~"started|rate_limited"}[15m]))` bien au-dessus de son niveau habituel pendant 15 minutes.
- De nombreux démarrages jamais payés : `sum(increase(my_supporter_tip_checkouts_total{my_outcome="expired"}[1h])) / sum(increase(my_supporter_tip_checkouts_total{my_outcome="started"}[1h]))` au-dessus de 0,8 avec au moins 20 démarrages.

Il vous appartient de vérifier auprès du fournisseur s’il accepte les pourboires, selon son examen et ses conditions. Formulez chaque texte des pourboires, vos mentions de commerce comprises, comme un pourboire pour faire fonctionner le service, jamais comme un don, une déduction ou une collecte de fonds pour une cause.

<!-- source: what-operators-must-check-not-yet-verified 4981ac7a2d03 -->
## Ce que les exploitants doivent vérifier (non encore vérifié)

mycroblog ne juge aucun de ces points. Vérifiez-les au regard des lois de votre pays et des conditions de votre prestataire.

- Les éléments que doivent contenir les mentions commerciales (le nom, l’adresse et le numéro de téléphone du vendeur, le prix, quand et comment payer, quand le service est fourni, comment résilier, etc.) et la manière dont elles doivent être publiées.
- Que l’écran de confirmation finale (article 12-6 de la loi sur les transactions commerciales spécifiées) indique la quantité, le prix, le moment du paiement, le moment de la livraison, ainsi que la manière de résilier et son effet, en prenant ensemble la page de paiement du prestataire et l’écran de confirmation de `my`.
- Le traitement de la taxe à la consommation, et si vous émettez des factures qualifiées.
- Pour les pourboires : si votre prestataire accepte les pourboires pour le fonctionnement d’un service (PAY.JP range le « tipping » parmi les biens qu’il approuve difficilement, et Stripe au Japon peut ne pas les accepter pour un entrepreneur individuel ; interrogez le prestataire), ce que les mentions commerciales en disent (ce qu’est un pourboire, les montants, qu’il est payé immédiatement, que rien n’est fourni en retour, et comment les remboursements sont traités), ainsi que leur taxe à la consommation et leur impôt sur le revenu, qu’un expert-comptable devrait confirmer.
