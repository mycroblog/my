<!-- source: supporter-payments-with-stripe e517db9d077d -->
# Paiements de soutien avec Stripe

Avec `payments = stripe`, les utilisateurs locaux souscrivent à des formules mensuelles ou annuelles sur la page de paiement hébergée de Stripe (Checkout), et le rôle de la formule est attribué et retiré à mesure que l’état du paiement change. Lorsqu’un administrateur les active, les utilisateurs peuvent aussi envoyer sur la même page des [pourboires](#tips) uniques.
Les clés de déploiement sont décrites dans la [référence de configuration](../reference/configuration.md#payments). Pour le programme dans son ensemble (formules, rôles, liste des membres), voir [supporters.md](supporters.md).
Un administrateur active le programme et crée les formules dans l’administration. Ce réglage permet seulement au serveur de dialoguer avec Stripe.

Un serveur ne peut utiliser qu’un seul prestataire. Pour passer plus tard à un autre prestataire, demandez aux souscripteurs existants de souscrire de nouveau auprès du nouveau.

<!-- source: what-the-server-does-at-stripe 17a592e3f643 -->
## Ce que fait le serveur chez Stripe

[Comment fonctionne une souscription](supporters.md#how-a-subscription-works) montre dans un schéma comment la page de paiement, le webhook, le rapprochement, le rôle de la formule et les revenus s’articulent.

- **Formules et prix** : pour chaque formule, le serveur crée un Product par l’API (ID `my_plan_<plan ID>`, avec `metadata.my_plan_id`) et un Price par intervalle et par devise (`lookup_key` vaut `my_<plan ID>_<month|year>_<currency>`).
  Un Price de Stripe ne peut pas changer de montant ; changer le montant crée donc un nouveau Price et archive l’ancien. Les souscriptions sur l’ancien Price se renouvellent à son montant. La remise pour un paiement annuel est exprimée dans le montant du Price annuel ; aucun coupon n’est utilisé.
  Ne modifiez ni ne supprimez ces Products et Prices dans le tableau de bord.
  Une formule peut utiliser les devises de la [liste des frais minimaux de Stripe](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts), et ses montants mensuel et annuel doivent chacun atteindre ce minimum : USD, AED, ARS, AUD, BRL, CAD, CHF, COP, CZK, DKK, EUR, GBP, HKD, HUF, IDR, ILS, INR, JPY, KRW, MXN, MYR, NOK, NZD, PHP, PLN, RON, RUB, SEK, SGD, THB et ZAR. Le serveur refuse toute autre devise.
  La liste vaut pour un débit dans la devise de règlement du compte. Un débit que Stripe convertit doit atteindre le minimum de la devise de règlement après la conversion, et le serveur ne le vérifie pas ; choisissez donc le montant d’une formule bien au-dessus du minimum lorsque sa devise diffère de celle de votre compte bancaire.
- **Souscrire** : le serveur crée une session Checkout avec `mode=subscription` et envoie le navigateur de l’utilisateur à son URL (`checkout.stripe.com`). La page de paiement expire au bout d’une heure.
  L’ID de souscription du serveur va dans `client_reference_id`, `metadata.my_subscription_id` et le `metadata.my_subscription_id` de la souscription Stripe. Le calcul automatique des taxes (Stripe Tax) n’est pas activé.
- **Recommencer** : au plus une page d’une souscription en attente peut recevoir un paiement. Un utilisateur qui recommence sur la même formule et le même intervalle revient à la page ouverte, qui s’ouvre de nouveau vide, sauf s’il lui reste moins de 15 minutes. Tout autre démarrage reçoit une nouvelle page, et le serveur fait expirer la précédente, qui affiche alors le message d’expiration de Stripe. Si une page précédente a déjà été payée, l’utilisateur ne reçoit aucune page : il va à l’écran de retour, qui affiche la souscription. Si le serveur ne peut pas faire expirer une page précédente, le démarrage échoue et l’utilisateur peut réessayer.
  Une souscription en attente qui se ferme (sa formule retirée, son compte supprimé) fait expirer sa page de la même façon.
- **Une deuxième souscription** : deux paiements peuvent encore se rencontrer, lorsque les deux pages sont soumises au même moment. La souscription qui atteint le serveur en premier est celle de l’utilisateur. Le serveur annule l’autre immédiatement et rembourse intégralement toutes ses factures payées, renouvellements compris, puisqu’il n’a rien fourni pour aucune période, avec le motif `duplicate`, et journalise `a second subscription was canceled and refunded`. Un paiement pour une souscription en attente qui s’est fermée est traité de la même façon, y compris lorsqu’il est arrivé au serveur juste avant la fermeture. L’utilisateur reçoit le remboursement de Stripe et voit une seule souscription. Stripe ne rend pas ses frais sur le débit remboursé, et ces frais apparaissent dans les revenus.
  Un débit en litige n’est pas remboursé, ni un paiement qui n’avait pas abouti lorsque le serveur a regardé (une méthode de prélèvement, par exemple). Une souscription que quelqu’un a déjà annulée dans le tableau de bord est laissée telle quelle, avec ses débits, et ne devient jamais la souscription de l’utilisateur.
  **Avant de mettre à niveau un serveur depuis une version qui n’a pas cela**, cherchez dans Stripe les clients ayant deux souscriptions dont le `metadata.my_subscription_id` est identique. Le premier rapprochement annule celle qui n’est pas utilisée et rembourse tout ce qu’elle a débité. Annulez dans le tableau de bord toutes celles que vous préféreriez régler à la main, et le serveur les laisse tranquilles.
- **Portail client** : à la première utilisation, le serveur crée par l’API une configuration de portail (`metadata.my=1`) qui n’active que la mise à jour des moyens de paiement, l’historique des factures et l’annulation à la fin de la période. Les changements de formule se font sur les écrans de ce serveur, pas dans le portail.
  Une annulation dans le portail apparaît sur ce serveur comme programmée, et l’utilisateur peut l’annuler depuis l’écran du serveur. Un changement de formule est refusé tant qu’elle n’est pas annulée.
- **Une date d’annulation fixée à la main** : le portail définit le `cancel_at` de la souscription, pas `cancel_at_period_end`, et le serveur lit les deux. Un `cancel_at` que vous définissez dans le tableau de bord ou par l’API compte de la même façon lorsqu’il n’est pas postérieur à la fin de la période en cours. Lorsqu’il est antérieur à cette fin, le serveur affiche cette date comme la fin de la souscription, et l’utilisateur garde le rôle jusque-là. Un `cancel_at` plus tardif n’est pas encore une annulation : la souscription reste active jusqu’à la période qui contient la date.
- **Changements de formule** : une montée en gamme, et un passage du mensuel à l’annuel, facturent immédiatement la différence au prorata (`proration_behavior=always_invoice`) et ne basculent que lorsque cette facture est payée (`payment_behavior=pending_if_incomplete`). Une descente en gamme, et un passage de l’annuel au mensuel, basculent à la fin de la période par un Subscription Schedule.
- **Un changement de formule qui échoue** : lorsque le paiement d’un changement échoue ou que la carte demande une authentification, le serveur annule cette facture, ce qui abandonne le changement, et l’utilisateur voit une erreur. La souscription reste sur sa formule et active, sans délai de grâce ni notification d’échec de paiement. L’utilisateur peut mettre à jour la carte dans le portail et réessayer, et n’est débité qu’une fois. Si le paiement est encore en cours de traitement, le serveur le laisse, et la formule bascule lorsqu’il réussit. Les renouvellements ne sont pas affectés : un renouvellement échoué passe au délai de grâce comme décrit ci-dessous.
- **Relire l’état** : le serveur traite un webhook comme un signal et relit la souscription, la session Checkout et les mouvements d’argent depuis l’API de Stripe pour décider de l’état.
  En outre, un rapprochement tous les `payments_reconcile_interval` (6 heures par défaut) lit la liste des événements, les sessions Checkout non terminées, les transactions de solde et un balayage de toutes les souscriptions, et répare tout webhook que le serveur a manqué.
- **Revenus** : le serveur enregistre le montant brut, les frais et le net de chaque transaction de solde. Les frais tels que les 0,7 % de Billing apparaissent comme des transactions `stripe_fee` distinctes et sont soustraits des revenus.
  `stripe_fee` couvre les frais de l’ensemble du compte ; **utilisez donc un compte Stripe dédié à ce serveur**. Sur un compte utilisé pour d’autres ventes, ces frais sont eux aussi soustraits des revenus.
- **Pourboires** : voir [Pourboires](#tips).
- **Dates** : les dates des revenus sont comptées dans le fuseau horaire du compte Stripe (le fuseau horaire sous « Settings > Business > Account details » dans le tableau de bord, `settings.dashboard.timezone` dans l’API). Lorsqu’il ne peut pas être lu, le serveur compte en UTC et le relit toutes les heures.

Chaque processus du serveur envoie au plus 20 requêtes par seconde (10 en mode test). [Les limites de Stripe](https://docs.stripe.com/rate-limits) s’appliquent par compte : 100 requêtes par seconde en mode réel, 25 dans un bac à sable, et 25 par seconde pour chaque point de terminaison. Les processus d’un même serveur les partagent, tout comme tout ce qui utilise le compte.
Lorsque Stripe renvoie `429`, le serveur attend aussi longtemps que l’indique `Retry-After` si c’est 30 secondes ou moins (1 seconde sans l’en-tête) et réessaie jusqu’à 2 fois ; au-delà, il laisse la nouvelle tentative à la tâche.

Stripe alloue aussi les requêtes de lecture par compte : une moyenne de 500 par transaction sur 30 jours, et au moins 10 000 par mois quel que soit le nombre de transactions. À l’intervalle par défaut, le rapprochement reste très en dessous sur un petit serveur. Un rapprochement qui ne trouve rien lit tout de même quelques pages ; l’intervalle le plus court de 15 minutes (2 880 exécutions en 30 jours) approche donc les 10 000 sur un compte comptant peu de transactions.
Stripe limite chaque souscription à 10 nouvelles factures par minute et 20 par jour, et un changement de formule facturé immédiatement crée une nouvelle facture.

<!-- source: tips 5c0435583f71 -->
## Pourboires

Un pourboire (voir [pourboires](supporters.md#tips) pour ce qu’ils sont et comment les activer) est une session Checkout avec `mode=payment` que le serveur crée pour le paiement unique :

- **La page** : une ligne d’article au prix fixé dans la requête, nommée « Pourboire pour <server name> » dans la langue de l’utilisateur, au montant et dans la devise choisis par l’utilisateur. Le serveur ne crée pour elle ni Product, ni Price, ni client : l’adresse e-mail de l’utilisateur ne fait que préremplir la page. La page n’accepte que les cartes, de sorte que chaque page terminée est payée ; elle n’utilise pas Adaptive Pricing, de sorte que l’utilisateur paie dans la devise de vos montants ; et son bouton indique « Payer », pas « Faire un don ». La page expire au bout d’une heure, comme celle d’une souscription.
- **Nos ID** : l’ID de pourboire du serveur va dans `client_reference_id`, `metadata.my_tip_id` et le `metadata.my_tip_id` du PaymentIntent du paiement, que vous pouvez rechercher dans le tableau de bord.
- **Montants** : un pourboire peut utiliser les devises et leurs minimums des formules (voir [Ce que fait le serveur chez Stripe](#what-the-server-does-at-stripe)), jusqu’au plus gros débit de Stripe de huit chiffres (999 999,99 dollars américains, ou 99 999 999 yens).
- **Le résultat** : le serveur relit la session lorsque son événement `checkout.session.completed` ou `checkout.session.expired` arrive, lorsque l’utilisateur revient de la page, et dans le rapprochement. Une session terminée dont le `payment_status` est `paid` est un pourboire payé, avec son PaymentIntent et le montant qu’il a débité.
- **Remboursements et litiges** : remboursez un pourboire dans le tableau de bord. Le remboursement, un litige et un litige gagné arrivent par les événements et les transactions de solde que les souscriptions utilisent, et nomment le PaymentIntent du pourboire, ce qui mène le serveur au pourboire.
- **Tests** : en mode test, payez la page d’un pourboire avec les cartes de test de [Essayer en mode test](#5-try-it-in-test-mode) ; `4000 0000 0000 0259` en fait un litige.

La clé restreinte de [Créer une clé d’API](#2-create-an-api-key) n’a besoin d’aucune autre permission pour les pourboires. Ils n’ont pas encore été exécutés dans un bac à sable.

<!-- source: card-data-and-pci-dss 5fd1cd5551e0 -->
## Données de carte et PCI DSS

Les moyens de paiement ne sont saisis que dans Checkout et le portail client de Stripe, et le client web de ce serveur ne charge ni le script de Stripe ni d’iframe.
Le serveur ne stocke que les ID d’objets Stripe (`cus_`, `sub_`, `price_`, `prod_`, `cs_`, `pi_`, `txn_`), les montants, les périodes et les états. Il ne stocke ni numéros de carte, ni quatre derniers chiffres, ni dates d’expiration, ni adresses de facturation. Les corps des webhooks et les réponses de Stripe ne sont conservés ni dans les journaux ni dans la base de données.
Pour cette raison, le périmètre PCI DSS de l’exploitant devrait être celui d’un commerçant utilisant une page de paiement hébergée (SAQ A). Vérifiez le périmètre qui s’applique et le type de questionnaire d’auto-évaluation dans les indications du tableau de bord de Stripe (« Settings > Compliance »).

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. Préparer le compte Stripe

1. Créez un compte Stripe et saisissez les informations de votre entreprise dans le tableau de bord. Si vous vendez au Japon, enregistrez l’URL de votre page d’information commerciale sous « Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions », et un lien apparaît sous Checkout.
2. Vérifiez le fuseau horaire sous « Settings > Business > Account details ». Les dates des revenus y sont comptées.
3. Sous « Settings > Billing > Subscriptions and emails > Manage failed payments », choisissez le comportement des nouvelles tentatives (Smart Retries) et ce qui se passe lorsqu’elles sont épuisées.
   La période de grâce avant le retrait du rôle (7 jours à compter du premier échec) est comptée par ce serveur ; les paramètres de nouvelle tentative de Stripe ne l’affectent donc pas. Lorsque les nouvelles tentatives sont épuisées, « Cancel the subscription » est recommandé.
4. Pour envoyer des reçus aux souscripteurs, activez le reçu de paiement réussi sous « Settings > Customer emails ».

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. Créer une clé d’API

Sous « Developers > API keys », il est recommandé de créer une **clé restreinte** (`rk_test_...` / `rk_live_...`). Une clé secrète (`sk_test_...` / `sk_live_...`) fonctionne aussi.
Le préfixe de la clé définit le mode, et une clé de test ne facture rien. Une clé publiable (`pk_`) ne peut pas être utilisée.

Donnez à une clé restreinte ces permissions. Une clé restreinte avec les permissions de ce tableau a permis de faire fonctionner la souscription, les changements de formule, l’annulation, le rapprochement et l’annulation de factures dans un sandbox. Les remboursements, l’expiration d’une page et les litiges n’ont pas été exécutés avec elle, et personne n’a vérifié que le tableau constitue l’ensemble minimal qui fonctionne.

| Ressource | Permission | API utilisées |
| --- | --- | --- |
| Products | Write | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Write | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | Write | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (part of Subscriptions) | Write | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Write | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | Write | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | Read | Lu via les transactions de solde et les paiements |
| Balance transaction sources | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account (information of an account outside Connect) | Read | `GET /v1/account` (fuseau horaire) |

Sans la permission d’écrire des remboursements, le serveur annule tout de même une seconde souscription, et `duplicate_charge` reste dans les avertissements (consultez [Enregistrer le point de terminaison du webhook](#3-register-the-webhook-endpoint)) jusqu’à ce qu’il puisse rembourser. Une clé créée avant que cette permission ne figure dans la liste doit la recevoir en plus.

Transmettez la clé dans la variable d’environnement `MY_PAYMENTS_STRIPE_SECRET_KEY` (ou le chemin d’un fichier dans `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`). Elle ne peut pas être écrite dans le fichier de configuration.
Si la clé fuit, faites-la pivoter dans le tableau de bord et redéployez avec la nouvelle clé.

<!-- source: 3-register-the-webhook-endpoint bf645f89a942 -->
## 3. Enregistrer le point de terminaison du webhook

Sous « Developers > Webhooks > Add endpoint », enregistrez ce qui suit. Le serveur ne crée pas le point de terminaison par l’API.

- URL : `https://<server domain>/payments/webhooks/stripe`
- Événements de : « Your account » (pas les comptes connectés)
- Style de charge utile : snapshot. Le serveur ne lit que `id`, `type`, `livemode`, `created`, `data.object.id` et `data.object.object` du corps ; la version d’API du point de terminaison n’a donc pas d’importance.
- Événements à envoyer (16 types ; les pourboires n’ont besoin d’aucun autre type) :

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

Transmettez le secret de signature du point de terminaison (`whsec_...`) dans la variable d’environnement `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` (ou `_FILE`).

Le point de terminaison vérifie la signature (HMAC-SHA256 dans `Stripe-Signature`) et l’horodatage (à 5 minutes près dans un sens ou dans l’autre), et rejette avec `400` un événement de test reçu avec une clé réelle (et l’inverse).
Il enregistre chaque événement une fois par ID, met une tâche en file et renvoie `2xx`. Lorsqu’il ne peut pas écrire dans la base de données, il renvoie `503`, et Stripe renvoie l’événement. Le corps est limité à 1 MiB.
Pendant que vous renouvelez le secret de signature, Stripe signe avec l’ancien et le nouveau ; mettre le nouveau secret dans la configuration suffit donc.

L’état du prestataire (le mode, la date d’arrivée du dernier webhook, et le résultat et le type d’échec du dernier rapprochement) est affiché dans la section « Prestataire de paiement » de `/admin/supporters` dans l’administration, et par l’API d’administration `GET /api/v1/admin/payment_status` (permission `manage_supporters`). La section explique comment corriger chaque type d’échec de rapprochement.
`warnings` liste `no_webhook` (aucun webhook n’a jamais été enregistré) et `reconciliation_corrected` (le dernier rapprochement a réparé une souscription). Soupçonnez un webhook mal configuré lorsque l’un des deux apparaît.
Il liste aussi `duplicate_charge` : une deuxième souscription n’est pas encore annulée ni remboursée. Voir [Exploitation](#7-operation).
« Rapprocher maintenant » dans la section (`POST /api/v1/admin/payment_reconciliation/runs`) rapproche immédiatement au lieu d’attendre l’exécution planifiée. Il est limité à une fois par heure UTC (un deuxième appel dans la même heure renvoie `429`), et le journal d’audit enregistre `payment_reconcile_queued`.

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. Configurer le serveur

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` vérifie la configuration. Un binaire compilé avec le build tag `no_payments_stripe` refuse `payments = stripe` (consultez [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. Essayer en mode test

Commencez avec une clé de test (`rk_test_` / `sk_test_`, ou une clé de sandbox). Le mode test ne facture rien.
Les clés de test et les clés live ont des Products, Prices, clients, souscriptions et points de terminaison de webhook distincts. Pour passer en production, enregistrez de nouveau le point de terminaison en mode live, remplacez la clé et le secret de signature par ceux du mode live, puis redéployez. Les Prices des formules sont recréés dans le compte live.

Cartes de test ([documentation de Stripe](https://docs.stripe.com/testing)) :

| Numéro | Comportement |
| --- | --- |
| `4242 4242 4242 4242` | Réussit |
| `4000 0000 0000 0341` | Peut être rattachée mais le paiement échoue (pour vérifier l’échec de paiement et le début de la période de grâce) |
| `4000 0000 0000 9995` | Échoue pour fonds insuffisants |
| `4000 0000 0000 0259` | Un litige est créé après le paiement |

Utilisez n’importe quelle date d’expiration future et n’importe quel code de sécurité à 3 chiffres.

Pour essayer le renouvellement et l’échec de paiement, utilisez une [horloge de test](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage). Un client créé par Checkout n’a pas d’horloge à sa création ; souscrivez donc d’abord, puis créez une horloge pour ce client via l’API, en passant `customer` à `POST /v1/test_helpers/test_clocks` (version d’API `2026-05-27.dahlia` ou ultérieure).
Stripe ne l’autorise que si `frozen_time` n’est pas dans le passé, si le compte n’a aucune Automation configurée et si le client a au plus trois souscriptions. Le client ne peut ensuite plus quitter l’horloge, et supprimer l’horloge supprime le client et ses souscriptions. Faites avancer le temps sous « Billing > Test clocks » dans le tableau de bord pour déclencher le paiement de renouvellement et l’échec.

Une horloge ne peut pas tout montrer :

- **La fin de la période de grâce** : le serveur compare le temps réel avec l’heure que Stripe indique pour l’échec (`finalized_at`), qui est simulée et en avance sur le temps réel. La souscription entre en période de grâce, et faire avancer davantage l’horloge n’y met pas fin. Le rôle reste jusqu’à ce que le temps réel dépasse de 7 jours l’échec simulé, ou jusqu’à ce que Stripe annule la souscription lorsque ses nouvelles tentatives sont épuisées, si le compte est configuré pour le faire.
- **Le balayage de toutes les souscriptions** : la liste des souscriptions de Stripe omet les souscriptions placés sur une horloge, sauf si la requête nomme l’horloge ou le client, et le balayage du serveur ne nomme ni l’un ni l’autre. Seuls les webhooks et la liste des événements apportent leurs changements au serveur.

<!-- source: 6-try-it-locally-stripe-cli 115b10f5518c -->
## 6. Essayer en local (Stripe CLI)

La [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) transmet les événements Stripe à un serveur sur votre machine. Aucune URL publique n’est nécessaire.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Mettez le `whsec_...` qu’affiche `stripe listen` dans `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` et démarrez le serveur (faites correspondre le port à votre réglage `listen`).

Avec Docker Compose, définissez `MY_PAYMENTS=stripe`, `MY_PAYMENTS_STRIPE_SECRET_KEY` et `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` dans `deploy/docker/.env`, puis exécutez `docker compose up -d`.

Dans un autre terminal, vous pouvez déclencher des événements isolés.

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

Les objets que crée `stripe trigger` ne portent aucun ID de souscription ni de pourboire de ce serveur ; rien ne change donc d’état (le serveur les relit et ignore ce qui n’est pas sa propre souscription ou son propre pourboire). Pour vérifier tout le parcours d’une souscription, ou un pourboire, souscrivez ou donnez un pourboire depuis le client web et payez avec une carte de test.

<!-- source: 7-operation f70cd2967b9f -->
## 7. Exploitation

- Un échec de rapprochement de type `permission` signifie une mauvaise clé ou des permissions manquantes. La ligne de journal du serveur (`payment reconciliation failed`) contient le message de Stripe, qui nomme la permission à ajouter.
- Lorsque les rapprochements atteignent souvent la limite de requêtes, augmentez `payments_reconcile_max_requests` ou allongez `payments_reconcile_interval`. Chaque lecture compte dans l’allocation de lectures de Stripe (consultez [Ce que le serveur fait chez Stripe](#what-the-server-does-at-stripe)) ; préférez donc l’intervalle plus long sur un compte comptant peu de transactions.
- Lorsque le rapprochement est arrêté depuis plus de 30 jours, le serveur ne peut pas rattraper son retard à partir de la liste des événements de Stripe (qui remonte à 30 jours). Le balayage de toutes les souscriptions répare alors l’état.
- `duplicate_charge` dans les avertissements signifie qu’un utilisateur a payé deux fois et que le serveur n’a pas pu terminer l’annulation et le remboursement de la seconde souscription. La ligne de journal `a second subscription could not be canceled and refunded` nomme les deux souscriptions et le type d’échec, et réapparaît à chaque rapprochement, qui réessaie. Avec le type `permission`, ajoutez la permission d’écrire des remboursements. Sinon, remboursez le paiement dans le tableau de bord : la tentative suivante ne trouve plus rien à rembourser et efface l’avertissement. « Rapprocher maintenant » réessaie immédiatement.
- Les autres remboursements et rétrofacturations sont gérés dans le tableau de bord de Stripe. Un remboursement intégral et une rétrofacturation retirent aussitôt le rôle et annulent la souscription, et un remboursement partiel ne retire pas le rôle. Gagner un litige ne rétablit pas automatiquement le rôle.

<!-- source: items-to-verify 19584d53548f -->
## Points à vérifier

Ceux-ci n’ont pas été essayés avec Stripe, ni dans un bac à sable ni en réel, et reposent sur la documentation de Stripe.

- Un renouvellement d’une souscription sur un Price archivé, qui devrait rester sur ce Price.
- `status_transitions.finalized_at` lors d’un vrai renouvellement. Le serveur le prend pour le moment où le paiement a échoué pour la première fois, et compte le délai de grâce à partir de là.
- Si une transaction `stripe_fee` peut être rattachée à une souscription. Le serveur ne l’essaie pas : il soustrait ces frais des revenus dans leur ensemble.
- Si `settings.dashboard.timezone` de `GET /v1/account` est lu avec une clé restreinte. Une lecture échouée n’arrête pas le serveur, qui compte en UTC ; un serveur qui fonctionne ne le montre donc pas. Vérifiez que les dates des revenus tombent dans le fuseau horaire du compte.
- Que `GET /v1/invoice_payments` filtre par `payment[payment_intent]`, ce qui relie un paiement à sa souscription.
- Les remboursements totaux et partiels et les rétrofacturations, du tableau de bord à l’état sur le serveur.
- Que le balayage de toutes les souscriptions répare une souscription dont le serveur a manqué l’état.
- Ce que fait une montée en gamme pour une souscription payée avec un moyen que les mises à jour en attente ne listent pas (les prélèvements ACH, SEPA et Bacs). Gardez ces moyens désactivés dans le tableau de bord jusqu’à ce que cela soit confirmé.
- Que `cancel_at_period_end=true` remplace un `cancel_at` d’une période ultérieure.
- Le retour à une page ouverte. Le serveur compare ce que renvoie `GET /v1/checkout/sessions/{id}` (`line_items`, `success_url`, `cancel_url`, `customer`, `customer_email`) avec la nouvelle requête, et toute différence crée une nouvelle page. Si Stripe réécrit `success_url` ou `customer_email` lorsqu’il les stocke, l’utilisateur reçoit une nouvelle page à chaque fois et perd ce qu’il a saisi, mais ne paie qu’une fois.
- Faire expirer une session au moment où son paiement se termine. La documentation de Stripe indique qu’une session qui n’est pas ouverte renvoie une erreur, et le serveur relit alors la session. Le code exact de l’erreur et le statut pendant qu’une authentification de carte est en cours ne sont pas documentés.
- Qu’une session ne puisse pas être payée deux fois depuis deux onglets. Une session contient une souscription et se termine une seule fois, mais la documentation ne l’indique pas pour deux soumissions au même moment.
- Si une authentification échouée sur la page de paiement laisse une souscription `incomplete` qui nomme la souscription du serveur. Le serveur la relierait et garderait l’utilisateur en attente, et un paiement sur une autre page serait remboursé comme étant le second.
- Un remboursement d’une deuxième souscription, avec une clé restreinte dont les permissions « Charges » et « Refunds » sont celles que le tableau indique, et les mouvements d’argent qui le suivent.
- Un pourboire depuis sa page jusqu’à son remboursement et son litige, avec une clé restreinte. En particulier : que `allowed_payment_method_types=["card"]` écarte tout autre moyen de la page, qu’une session créée avec Adaptive Pricing désactivé débite la devise demandée, et que Stripe prend un débit unique de huit chiffres dans chaque devise du tableau.
- Si Stripe accepte les pourboires pour le fonctionnement d’un service, et pour un entrepreneur individuel au Japon. Interrogez Stripe avant de les activer.
