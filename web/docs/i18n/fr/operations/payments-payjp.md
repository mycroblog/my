<!-- source: supporter-payments-with-payjp 6d397d24cf93 -->
# Paiements de soutien avec PAY.JP

Avec `payments = payjp`, les utilisateurs locaux souscrivent à des formules mensuelles ou annuelles sur la page de paiement de ce serveur, et le rôle de la formule est attribué et retiré à mesure que l’état du paiement change. Lorsque les pourboires sont activés, les utilisateurs paient aussi des pourboires uniques sur la page hébergée de PAY.JP (voir [Pourboires](#tips)).
Les clés de déploiement sont décrites dans la [référence de configuration](../reference/configuration.md#payments). Pour le programme dans son ensemble (formules, rôles, liste des membres), voir [supporters.md](supporters.md).
Un administrateur active le programme et crée les formules dans l’administration. Ce réglage permet seulement au serveur de dialoguer avec PAY.JP.

Un serveur ne peut utiliser qu’un seul prestataire. Pour passer de Stripe à PAY.JP (ou l’inverse), demandez aux souscripteurs existants de souscrire de nouveau auprès du nouveau prestataire.

<!-- source: what-the-server-does-at-payjp afde8e828e80 -->
## Ce que fait le serveur chez PAY.JP

PAY.JP propose l’API v1 et l’API v2. La v2 n’a pas encore d’API de facturation récurrente ; le serveur utilise donc la facturation récurrente de l’**API v1** (Plan et Subscription). La v1 n’a ni page de paiement hébergée ni portail client ; ce serveur sert donc des pages à leur place.
Les pourboires sont des paiements uniques sur l’**API v2**, qui dispose d’une page de paiement hébergée ; voir [Pourboires](#tips). PAY.JP autorise les deux versions sur un même compte avec les mêmes clés.

[Comment fonctionne une souscription](supporters.md#how-a-subscription-works) montre dans un schéma comment la page de paiement, le webhook, le rapprochement, le rôle de la formule et les revenus s’articulent.

- **Formules** : pour chaque prix, le serveur crée un Plan dont l’ID est `my_<plan ID>_<month|year>_<amount>` (`metadata.my_plan_id`). Un Plan PAY.JP ne peut pas changer de montant ; changer le montant crée donc un nouveau Plan, et l’ancien Plan n’est pas supprimé mais reçoit `metadata.my_archived=1`. Les souscriptions sur l’ancien Plan se renouvellent à son montant.
  Ne modifiez ni ne supprimez ces Plans dans l’administration de PAY.JP. La seule devise est le yen japonais, et les montants vont de 50 à 9 999 999 yens.
- **Souscrire** : le serveur envoie le navigateur de l’utilisateur à sa propre page de paiement (`/payments/pages/payjp/checkout`). La page affiche les champs de carte de payjp.js (l’iframe de PAY.JP) et lance l’authentification 3-D Secure lorsqu’elle crée le jeton (le type iframe).
  Le serveur n’accepte qu’un jeton dont le 3-D Secure est terminé (`three_d_secure_status` vaut `verified` ou `attempted`), ajoute la carte au client comme carte par défaut (la première fois avec l’ID `my_<the server's subscription ID>`), et crée une souscription avec l’ID de souscription du serveur dans `metadata.my_subscription_id`. Le premier débit a lieu à la création de la souscription.
  L’URL de la page est de courte durée et porte un état signé par le serveur (valable de 1 à 2 heures à partir de sa création).
  Un utilisateur qui quitte la page et recommence, sur la même formule ou sur une autre, reçoit une nouvelle URL pour la formule qu’il demande à ce moment-là. Une URL précédente continue de fonctionner jusqu’à son expiration, et la page payée en premier crée l’unique souscription : les autres y mènent. Si une page précédente a déjà été payée, l’utilisateur va à l’écran de retour et ne reçoit pas de nouvelle URL.
  Une URL signée ne peut pas être retirée ; une page reste donc payable après la fermeture de sa souscription en attente (sa formule retirée, son compte supprimé). Un paiement dessus n’achète rien : le serveur supprime la souscription et rembourse intégralement ses débits (PAY.JP accepte un remboursement pendant 180 jours après la vente).
  L’API v1 n’accepte aucune clé d’idempotence ; tant qu’une soumission de la page crée la souscription, le serveur marque donc le paiement comme en cours dans la base de données : une deuxième soumission (un double clic, un autre onglet, ou un autre processus du serveur derrière le même répartiteur de charge) attend jusqu’à 30 secondes puis mène à la souscription que la première a créée. La marque expire au bout de 5 minutes si le processus qui la détenait s’est arrêté.
- **La page de la carte et de l’historique des paiements** (à la place du portail client de Stripe) : depuis son écran de souscription, un souscripteur ouvre `/payments/pages/payjp/portal` pour changer la carte et consulter les paiements récents (l’URL expire au bout d’une heure). Le formulaire commence par l’adresse e-mail du client PAY.JP, qui est l’adresse saisie au premier paiement, et non un changement ultérieur de l’adresse du compte.
  Changer la carte reprend une souscription arrêtée par un paiement échoué et débite immédiatement. Résilier et changer de formule se font sur l’écran de souscription de ce serveur.
- **Échec de paiement** : PAY.JP met en pause (`paused`) une souscription dont le débit de renouvellement a échoué, et **ne réessaie pas**. Le serveur traite la pause comme un retard de paiement et conserve le rôle pendant un délai de grâce de 7 jours à compter de la pause. Si le souscripteur change la carte dans ce délai, la souscription reprend ; sinon, le rôle est retiré après 7 jours (côté PAY.JP, la souscription reste en pause et n’est pas débitée).
- **Changements de formule** : une montée en gamme débite la différence pour le reste de la période en cours, au prorata, comme un débit unique (aucun débit sous 50 yens), remplace la formule et débite le montant complet de la nouvelle formule à la fin de la période (le début de la période ne bouge pas). Si le débit de la différence est refusé, la formule ne change pas.
  Une descente en gamme, et un passage de l’annuel au mensuel, basculent à la fin de la période avec `next_cycle_plan`.
- **Relire l’état** : le serveur traite un webhook comme un signal et relit la souscription et les mouvements d’argent depuis l’API de PAY.JP pour décider de l’état.
  En outre, un rapprochement tous les `payments_reconcile_interval` lit la liste des événements, les mouvements d’argent et un balayage de toutes les souscriptions, et répare tout webhook que le serveur a manqué.
- **Revenus** : pour chaque paiement, le serveur enregistre le montant brut, les frais (calculés à partir du `fee_rate` du débit, arrondis à la baisse en dessous d’un yen) et le net. Un remboursement et une rétrofacturation enregistrent les frais restitués comme des frais négatifs.
  Lorsqu’un terme se clôt et qu’un relevé est émis, la différence entre les frais du relevé et le calcul du serveur (les frais des débits des souscriptions et des pourboires du terme) est enregistrée comme un mouvement de frais (son ID est celui du relevé), afin que le total des frais du terme corresponde au relevé.
  Un relevé couvre les frais de l’ensemble du compte ; **utilisez donc un compte PAY.JP dédié à ce serveur**.
- **Dates** : les dates des revenus sont comptées à l’heure du Japon.

Le serveur envoie au plus 20 requêtes par seconde (2 en mode test), API v1 et v2 confondues. Cela reste dans la limite de débit de PAY.JP (pour les requêtes à clé secrète, 30 par seconde en mode réel et 2 en mode test). Sur `429`, le serveur attend 1 seconde, en doublant à chaque fois, et réessaie jusqu’à 3 fois.
En mode test, un rapprochement isolé est lent (environ 8 minutes à la limite de 1 000 requêtes). Une tâche de continuation prend le relais ; aucune action n’est donc nécessaire.

<!-- source: tips b0917978ffa6 -->
## Pourboires

Les pourboires (voir [supporters.md](supporters.md#tips)) utilisent l’API v2 de PAY.JP avec les mêmes clés et le même jeton de webhook que les souscriptions. PAY.JP range le « tipping » parmi les biens qu’il approuve difficilement : savoir si votre compte peut prendre des pourboires est à régler avec PAY.JP, selon son examen et ses conditions.

- **Payer** : le serveur crée une Checkout Session (`mode=payment`) du montant unique, par carte uniquement, qui expire au bout d’une heure, et envoie l’utilisateur à sa page chez PAY.JP (une page `https` sur `pay.jp` ; le serveur refuse toute autre adresse). L’ID de pourboire du serveur se trouve dans le `client_reference_id` de la session et dans le `metadata.my_tip_id` de la session et de son PaymentFlow. La création porte un `Idempotency-Key`, de sorte qu’une nouvelle tentative reçoit la même session. PAY.JP lance 3-D Secure sur la page hébergée lorsque l’émetteur de la carte le demande.
  Un pourboire est payé lorsque sa session est `complete` et que son PaymentFlow a réussi. Lorsqu’un utilisateur démarre un autre pourboire, le serveur fait d’abord expirer la session ouverte (`POST /v2/checkout/sessions/{id}/expire`) ; si l’utilisateur l’a payée entre-temps, ce paiement est enregistré à la place.
- **Revenus** : l’argent des pourboires provient des transactions de paiement du compte de l’API v2 : un paiement (son ID est celui du PaymentFlow), un remboursement, une rétrofacturation et l’annulation d’une rétrofacturation, chacun avec les frais que PAY.JP indique. Un remboursement et une rétrofacturation enregistrent les frais restitués comme des frais négatifs, comme pour les souscriptions.
- **Litiges** : PAY.JP n’envoie un événement qu’à l’ouverture d’un litige. Chaque rapprochement lit aussi les litiges qui se sont terminés en votre faveur (statut `cancel`), et enregistre l’annulation d’une rétrofacturation que les transactions de paiement ne signalent pas elles-mêmes. Un litige clos sans rétrofacturation (une demande d’information) n’enregistre rien.
- **Remboursements** : remboursez un pourboire dans le tableau de bord de PAY.JP. Un remboursement total et une rétrofacturation sont enregistrés avec le pourboire ; un remboursement partiel seulement dans les revenus.

<!-- source: card-data-and-pci-dss 7be9976efb0b -->
## Données de carte et PCI DSS

Les numéros de carte, les dates d’expiration et les codes de sécurité ne sont saisis que dans l’iframe de PAY.JP, que montre payjp.js. La page et le script de ce serveur n’y touchent jamais. Le serveur ne reçoit que l’ID du jeton créé par payjp.js ainsi que le nom du titulaire et l’adresse e-mail dont 3-D Secure a besoin.
Le serveur ne stocke que les ID d’objets PAY.JP (`cus_`, `sub_`, `ch_`, les ID de Plan, les ID d’événements, ainsi que les Checkout Sessions, PaymentFlows, remboursements et litiges des pourboires), les montants, les périodes et les états. La carte d’un pourboire est saisie sur la propre page de PAY.JP, que ce serveur ne sert jamais. Il ne stocke ni numéros de carte, ni quatre derniers chiffres, ni dates d’expiration (la page de la carte ne fait que lire chez PAY.JP les quatre derniers chiffres, la date d’expiration et l’adresse e-mail du client, et les affiche). Les corps des webhooks et les réponses de PAY.JP ne sont conservés ni dans les journaux ni dans la base de données.

La page de paiement est servie depuis l’origine de ce serveur ; elle est donc conçue en tenant compte de la gestion des scripts des pages de paiement de PCI DSS v4 (exigence 6.4.3) et de la détection des altérations (exigence 11.6.1).

- La page ne charge que `https://js.pay.jp/v2/pay.js` et un petit script avec un nonce intégré à la page. Elle ne porte ni le bundle du client web ni aucun script d’analyse ou de publicité.
- La page porte toujours sa propre Content-Security-Policy (`script-src` uniquement le nonce et `https://js.pay.jp`, `frame-src` uniquement `https://js.pay.jp` et `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` et `X-Content-Type-Options: nosniff`. Aucun réglage ne modifie la CSP.
- Le gabarit et le script sont intégrés au binaire du serveur et ne peuvent pas être modifiés à l’exécution. Les envois de formulaire ne sont acceptés que depuis la même origine.

Par conséquent, le périmètre de l’exploitant devrait être celui d’un commerçant qui laisse la saisie du paiement à l’iframe de PAY.JP (SAQ A). Vérifiez le périmètre qui s’applique et le type de questionnaire d’auto-évaluation au regard des indications de PAY.JP et des exigences de vos réseaux de cartes.
Ce faisant, il est utile de conserver une trace de ces points : les URL des pages de paiement (`/payments/pages/payjp/checkout` et `/portal`), la liste des scripts que la page charge (les deux ci-dessus), la CSP de la page, et le fait que le trafic vers le serveur utilise TLS (la configuration du proxy inverse).

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. Préparer le compte PAY.JP

1. Créez un compte PAY.JP et commencez en mode test. Prélever en mode live exige une demande de passage en production et un examen.
2. Vérifiez « API settings > 3-D Secure » dans le tableau de bord PAY.JP. Le serveur demande le 3-D Secure à chaque création de jeton ; vous pouvez aussi choisir d’exiger le 3-D Secure pour toute création de jeton.
3. La page des mentions légales commerciales se définit dans l’administration de ce serveur (voir [supporters.md](supporters.md)).

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. Clés et jeton de webhook

Sous « API » dans le tableau de bord PAY.JP, repérez ces deux clés. Le préfixe d’une clé détermine le mode, et les clés de test ne prélèvent rien. Les deux clés doivent être dans le même mode.

| Clé | Exemple | Usage |
| --- | --- | --- |
| Clé secrète | `sk_test_...` / `sk_live_...` | Le serveur appelle l’API avec elle. C’est aussi la source de la clé qui signe les URL des pages |
| Clé publique | `pk_test_...` / `pk_live_...` | payjp.js sur les pages de paiement et de carte |

Transmettez la clé secrète dans la variable d’environnement `MY_PAYMENTS_PAYJP_SECRET_KEY` (ou le chemin d’un fichier dans `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`). Elle ne peut pas être écrite dans le fichier de configuration.
Si la clé secrète fuite, réémettez-la dans le tableau de bord et redéployez avec la nouvelle clé. Les URL des pages de paiement et de carte ouvertes avant le redéploiement cessent de fonctionner.

<!-- source: 3-register-the-webhook 836afecf8d0c -->
## 3. Enregistrer le webhook

Sous « Settings > Webhook » dans le tableau de bord de PAY.JP, ajoutez une destination. Enregistrez séparément le mode test et le mode réel.

- URL : `https://<server domain>/payments/webhooks/payjp`
- Mode : le même que celui des clés

Un webhook PAY.JP identifie son expéditeur par le **jeton d’origine** du compte (`whook_...`, dans l’en-tête `X-Payjp-Webhook-Token`). Transmettez le jeton affiché dans le tableau de bord dans la variable d’environnement `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (ou `_FILE`).
Le point de terminaison compare le jeton en temps constant, et rejette avec `400` un événement de test reçu avec une clé réelle (et l’inverse). Il n’y a ni signature ni horodatage ; toute personne qui connaît le jeton peut donc envoyer une notification, mais le serveur n’écrit jamais d’état à partir du contenu d’une notification ; il ne fait que relire l’état depuis PAY.JP.

Le serveur utilise ces événements (les autres types sont acceptés mais non enregistrés).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`
- Pour les pourboires (API v2) : `checkout.session.completed`, `checkout.session.expired`, `payment_flow.succeeded`, `refund.created`, `refund.updated`, `refund.failed` et `dispute.created`

Le serveur distingue les événements de l’API v1 et de l’API v2 par leur forme ; les deux versions vont donc à la même URL avec le même jeton.

PAY.JP ne renvoie une notification que toutes les 3 minutes, au plus 3 fois, et une notification au-delà n’arrive pas tant que vous ne la renvoyez pas depuis le tableau de bord. Le rapprochement répare ce qui a été manqué ; **il est donc recommandé de régler `payments_reconcile_interval` sur 1 heure**.
L’état du prestataire (le mode, la date d’arrivée du dernier webhook, et le résultat du dernier rapprochement) est affiché dans la section « Prestataire de paiement » de `/admin/supporters` dans l’administration.

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. Configurer le serveur

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` vérifie la configuration. Un binaire construit avec le build tag `no_payments_payjp` refuse `payments = payjp` (voir [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. Essayer en mode test

Commencez avec des clés de test. Le mode test et le mode live ont des Plans, des clients, des souscriptions, des webhooks et des jetons distincts. Pour passer en live, enregistrez le webhook en mode live, remplacez les clés et le jeton par ceux du live, et redéployez. Les Plans des formules de soutien sont créés à nouveau dans le compte live.

Cartes de test ([documentation PAY.JP](https://pay.jp/docs/testcard)) :

| Numéro | Comportement |
| --- | --- |
| `4242 4242 4242 4242` | Réussit |
| `4000 0000 0008 0319` | La carte peut être enregistrée mais le prélèvement est refusé (un souscription échoué, et une reprise échouée après un changement de carte) |
| `4000 0000 0000 4012` | La carte peut être enregistrée mais le prélèvement échoue pour cause d’expiration |
| `4000 0000 0000 0002` | L’enregistrement de la carte (la création du jeton) est refusé |

Utilisez une date d’expiration future ; le code de sécurité et le nom sont arbitraires. En mode test, l’écran d’authentification de test de PAY.JP pour le 3-D Secure vous permet de choisir le résultat.

Pour essayer rapidement un renouvellement, réglez le `trial_end` de la souscription quelques minutes plus tard avec l’API. Le prélèvement de renouvellement s’exécute quelques minutes après cette heure, et non à cette heure. Une autre mise à jour de `trial_end` alors que la première arrive à échéance relance l’attente.
Pour essayer un échec de paiement et le délai de grâce, souscrivez, changez la carte sur la page de la carte pour la carte refusée (`4000 0000 0008 0319`), puis laissez la souscription se renouveler.

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. Essayer en local

Les webhooks PAY.JP vont vers une URL publique : pour les recevoir sur un serveur de votre machine, exposez le port via un tunnel (cloudflared, ngrok et autres) et enregistrez cette URL comme destination du mode test.
`payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` de payjp-cli devrait transférer les événements également, mais la capacité à transférer les événements v1 n’a pas été vérifiée.

Ouvrez les pages de paiement et de carte sur l’origine de ce serveur (elles fonctionnent aussi sur `http://localhost:8080`, mais le comportement de payjp.js et du 3-D Secure varie selon les paramètres du navigateur).

<!-- source: 7-operation fae168baab1c -->
## 7. Exploitation

- Un échec de rapprochement de type `permission` signifie une clé secrète erronée.
- `duplicate_charge` dans les avertissements de la section « Prestataire de paiement » signifie que le serveur n’a pas pu terminer de rembourser un paiement pour une souscription fermée. La ligne de journal `a second subscription could not be canceled and refunded` nomme la souscription, et chaque rapprochement réessaie, y compris après la suppression de la souscription. Remboursez le débit dans le tableau de bord si cela persiste : la prochaine tentative le trouve remboursé et efface l’avertissement, tant que PAY.JP liste encore les débits de la souscription supprimée (voir [Points à vérifier](#items-to-verify)). Un débit de plus de 180 jours ne peut pas être remboursé : le serveur le journalise et continue.
- Les autres remboursements se traitent dans le tableau de bord de PAY.JP. Un remboursement total et une rétrofacturation retirent le rôle et suppriment immédiatement la souscription, et un remboursement partiel ne retire pas le rôle.
- **Une annulation de rétrofacturation** d’un débit de souscription n’est pas enregistrée automatiquement, car l’API v1 n’a ni événement ni API pour cela (pour un pourboire, si : voir [Pourboires](#tips)). Lorsqu’elle survient, enregistrez le montant annulé sur l’écran des revenus de la page des coûts (`finance`). Le rôle n’est pas rétabli automatiquement.
- **Les frais de formule payante et les frais de virement de PAY.JP** ne sont pas soustraits des revenus. Enregistrez-les comme des coûts dans `finance`.
- Lorsque les rapprochements atteignent souvent la limite de requêtes, relevez `payments_reconcile_max_requests` ou allongez `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Changements de formule de soutien

Une montée en gamme prélève d’abord la différence au prorata et ne déplace la formule de soutien qu’une fois ce prélèvement réussi. Quand le prélèvement est refusé, la formule de soutien reste et l’utilisateur voit une erreur avec le moyen de mettre à jour sa carte, sans délai de grâce ni notification de paiement échoué, et un nouvel essai avec une bonne carte prélève une seule fois. Si la formule de soutien ne se déplace pas après le prélèvement, un nouvel essai ne prélève pas de nouveau. Une différence inférieure au minimum de 50 yens, et un passage au prix annuel de la même formule de soutien, qui n’a pas de différence positive, ne prélèvent rien maintenant et déplacent la formule de soutien immédiatement ; le nouveau prix est prélevé à la fin de la période, et un échec à ce moment est un renouvellement échoué.

<!-- source: items-to-verify d5fb7fd9927f -->
## Points à vérifier

Les comportements suivants ne sont tranchés ni par la documentation de PAY.JP ni par le mode test.

- Comment sont calculés les frais restitués par une rétrofacturation, et si le relevé d’un terme correspond aux frais du serveur (le mode test a montré des frais arrondis à la baisse, et des frais de remboursement calculés à partir de son montant).
- Ce que fait une vraie rétrofacturation : PAY.JP ne documente aucun moyen d’en provoquer une en mode test ni aucun corps pour `dispute.created`. Un envoi de test de l’événement depuis le tableau de bord reçoit `200` et ne change rien.
- Jusqu’où remonte la liste des événements.
- Si supprimer un Plan affecte les souscriptions existantes (le serveur n’en supprime jamais), et la longueur maximale d’un ID de Plan.
- Si le rapport de CSP au chargement de la page de paiement importe : `pay.js` applique un style en ligne contre `style-src 'self'`, alors que les champs de carte et 3-D Secure fonctionnent dans Chromium. Les autres navigateurs n’ont pas été vérifiés.
- Si payjp-cli peut transmettre les événements v1.
- Qu’un débit sur la carte enregistrée d’un client ne demande pas de 3-D Secure sauf si la requête le demande. Un débit refusé est listé par `GET /v1/charges` avec `paid: false`, et le serveur ignore tout débit non payé.
- Pour les pourboires sur l’API v2, qui ont été conçus à partir des guides et du SDK officiel de PAY.JP et pas encore essayés en mode test : que PAY.JP envoie les événements de la v2 à la même destination avec le même jeton que ceux de la v1 ; que la page hébergée se trouve sur `pay.jp` ou l’un de ses sous-domaines ; que la création d’une Checkout Session respecte `Idempotency-Key` ; que le `resource_id` d’une transaction de paiement est le PaymentFlow d’un paiement, le remboursement d’un remboursement, et le litige d’une rétrofacturation et de son annulation ; que les listes sont triées des plus récentes aux plus anciennes avec `starting_after` et `ending_before` ; les signes du montant et des frais d’une transaction ; et que le relevé d’un terme de la v1 inclut les frais de la v2.
- Que `GET /v1/charges` avec `subscription` liste encore les débits d’une souscription supprimée. Le serveur rembourse avant de supprimer ; seule une tentative après un remboursement définitivement échoué s’appuie donc sur cela. La référence de l’API documente le filtre et ne dit rien des souscriptions supprimées. Si la liste revient vide, le serveur signale un échec au lieu d’un remboursement, et `duplicate_charge` reste même après que le débit a été remboursé à la main.
