<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# Paiements de soutien avec PAY.JP

Avec `payments = payjp`, les utilisateurs locaux souscrivent à des formules de soutien mensuelles ou annuelles sur la page de paiement de ce serveur, et le rôle de la formule de soutien est accordé et retiré selon l’état du paiement.
Les clés de déploiement sont décrites dans la [référence de la configuration](../reference/configuration.md#payments). Pour le programme dans son ensemble (formules de soutien, rôles, liste des membres), consultez [supporters.md](supporters.md).
Un administrateur active le programme et crée les formules de soutien dans l’administration. Ce paramètre permet seulement au serveur de communiquer avec PAY.JP.

Un serveur ne peut utiliser qu’un seul prestataire. Pour passer de Stripe à PAY.JP (ou l’inverse), demandez aux souscripteurs existants de souscrire de nouveau auprès du nouveau prestataire.

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## Ce que fait le serveur chez PAY.JP

PAY.JP propose une API v1 et une API v2. La v2 n’a pas encore d’API de facturation récurrente : le serveur utilise donc la facturation récurrente de l'**API v1** (Plan et Subscription). La v1 n’a ni page de paiement hébergée ni portail client : ce serveur sert donc des pages à leur place.

[Comment fonctionne un souscription](supporters.md#how-a-subscription-works) montre dans un schéma comment s’articulent la page de paiement, le webhook, la réconciliation, le rôle de la formule de soutien et les revenus.

- **Plans** : pour chaque prix, le serveur crée un Plan dont l’ID est `my_<plan ID>_<month|year>_<amount>` (`metadata.my_plan_id`). Un Plan PAY.JP ne peut pas changer de montant : changer le montant crée donc un nouveau Plan, et l’ancien Plan n’est pas supprimé mais reçoit `metadata.my_archived=1`. Les souscriptions sur l’ancien Plan se renouvellent à son montant.
  Ne modifiez ni ne supprimez ces Plans dans l’administration de PAY.JP. La seule devise est le yen japonais, et les montants vont de 50 à 9 999 999 yens.
- **Souscrire** : le serveur envoie le navigateur de l’utilisateur vers sa propre page de paiement (`/payments/pages/payjp/checkout`). La page affiche les champs de carte de payjp.js (l’iframe de PAY.JP) et lance l’authentification 3-D Secure à la création du jeton (le type iframe).
  Le serveur n’accepte qu’un jeton dont le 3-D Secure est terminé (`three_d_secure_status` vaut `verified` ou `attempted`), ajoute la carte au client comme carte par défaut (la première fois avec l’ID `my_<the server's subscription ID>`), et crée un souscription avec l’ID d’souscription du serveur dans `metadata.my_subscription_id`. Le premier prélèvement a lieu à la création de la souscription.
  L’URL de la page est de courte durée et porte un état signé par le serveur (valable de 1 à 2 heures à partir de sa création).
  Un utilisateur qui quitte la page et recommence, sur la même formule de soutien ou une autre, obtient une nouvelle URL pour la formule de soutien qu’il demande maintenant. Une URL antérieure continue de fonctionner jusqu’à son expiration, et la première page payée crée l’unique souscription : les autres y mènent. Si une page antérieure a déjà été payée, l’utilisateur est dirigé vers l’écran de retour et n’obtient pas de nouvelle URL.
  Une URL signée ne peut pas être retirée : une page reste donc payable après la fermeture de son souscription en attente (sa formule de soutien retirée, son compte supprimé). Un paiement dessus n’achète rien : le serveur supprime la souscription et rembourse intégralement ses prélèvements (PAY.JP accepte un remboursement pendant 180 jours après la vente).
  L’API v1 n’accepte pas de clé d’idempotence : pendant qu’une soumission de la page crée la souscription, le serveur marque donc le paiement comme en cours dans la base de données ; une seconde soumission (un double clic, un autre onglet ou un autre processus serveur derrière le même équilibreur de charge) attend jusqu’à 30 secondes puis rejoint la souscription créé par la première. La marque expire au bout de 5 minutes si le processus qui la tenait s’est arrêté.
- **La page de la carte et de l’historique des paiements** (à la place du portail client de Stripe) : depuis son écran d’souscription, un souscripteur ouvre `/payments/pages/payjp/portal` pour changer de carte et consulter les paiements récents (l’URL expire au bout d’une heure). Le formulaire commence par l’adresse e-mail du client PAY.JP, qui est l’adresse saisie au premier paiement, et non un changement ultérieur de l’adresse du compte.
  Changer de carte reprend un souscription arrêté par un paiement échoué et prélève immédiatement. L’annulation et le changement de formule de soutien se font sur l’écran d’souscription de ce serveur.
- **Échec de paiement** : PAY.JP met en pause (`paused`) un souscription dont le prélèvement de renouvellement a échoué, et **ne réessaie pas**. Le serveur traite la pause comme un retard de paiement et conserve le rôle pendant un délai de grâce de 7 jours à partir de la pause. Si l’souscripteur change de carte dans ce délai, la souscription reprend ; sinon, le rôle est retiré au bout de 7 jours (côté PAY.JP, la souscription reste en pause et n’est pas prélevé).
- **Changements de formule de soutien** : une montée en gamme prélève la différence pour le reste de la période en cours, au prorata, sous forme de prélèvement unique (aucun prélèvement sous 50 yens), remplace la formule de soutien, et prélève le montant complet de la nouvelle formule de soutien à la fin de la période (le début de la période ne bouge pas). Si le prélèvement de la différence est refusé, la formule de soutien ne change pas.
  Une descente en gamme, et un passage de l’annuel au mensuel, prennent effet à la fin de la période avec `next_cycle_plan`.
- **Relecture de l’état** : le serveur traite un webhook comme un signal, et relit la souscription et les mouvements d’argent depuis l’API PAY.JP pour déterminer l’état.
  De plus, une réconciliation tous les `payments_reconcile_interval` relit la liste des événements, les mouvements d’argent et un balayage de tous les souscriptions, et répare tout webhook que le serveur a manqué.
- **Revenus** : pour chaque paiement, le serveur enregistre le montant brut, les frais (calculés à partir du `fee_rate` du prélèvement, arrondis à l’inférieur en dessous d’un yen) et le net. Un remboursement et une rétrofacturation enregistrent les frais restitués comme des frais négatifs.
  Quand une période se clôture et qu’un relevé est émis, la différence entre les frais du relevé et le calcul du serveur est enregistrée comme un mouvement de frais (son ID est celui du relevé), afin que le total des frais de la période corresponde au relevé.
  Un relevé couvre les frais de tout le compte : **utilisez donc un compte PAY.JP dédié à ce serveur**.
- **Dates** : les dates des revenus sont comptées à l’heure du Japon.

Le serveur envoie au plus 20 requêtes par seconde (2 en mode test). Cela reste dans la limite de débit de PAY.JP (pour les requêtes avec clé secrète, 30 par seconde en mode live et 2 en mode test). Sur `429`, le serveur attend 1 seconde, en doublant à chaque fois, et réessaie jusqu’à 3 fois.
En mode test, une seule réconciliation est lente (environ 8 minutes à la limite de 1 000 requêtes). Une tâche de continuation prend le relais : aucune action n’est nécessaire.

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## Données de carte et PCI DSS

Les numéros de carte, les dates d’expiration et les codes de sécurité ne sont saisis que dans l’iframe de PAY.JP, qu’affiche payjp.js. La page et le script de ce serveur n’y touchent jamais. Le serveur ne reçoit que l’ID du jeton créé par payjp.js, ainsi que le nom du titulaire et l’adresse e-mail dont le 3-D Secure a besoin.
Le serveur ne stocke que des ID d’objets PAY.JP (`cus_`, `sub_`, `ch_`, ID de Plan, ID d’événement), des montants, des périodes et des états. Il ne stocke ni numéros de carte, ni quatre derniers chiffres, ni dates d’expiration (la page de la carte ne fait que lire depuis PAY.JP les quatre derniers chiffres, la date d’expiration et l’adresse e-mail du client, et les afficher). Les corps de webhooks et les réponses de PAY.JP ne sont conservés ni dans les journaux ni dans la base de données.

La page de paiement est servie depuis l’origine de ce serveur : elle est donc conçue en tenant compte de la gestion des scripts des pages de paiement de PCI DSS v4 (exigence 6.4.3) et de la détection des altérations (exigence 11.6.1).

- La page ne charge que `https://js.pay.jp/v2/pay.js` et un petit script avec un nonce intégré à la page. Elle ne porte ni le bundle du client web, ni aucun script d’analyse ou de publicité.
- La page porte toujours sa propre Content-Security-Policy (`script-src` uniquement le nonce et `https://js.pay.jp`, `frame-src` uniquement `https://js.pay.jp` et `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` et `X-Content-Type-Options: nosniff`. Aucun paramètre ne modifie la CSP.
- Le modèle et le script sont intégrés au binaire du serveur et ne peuvent pas être modifiés à l’exécution. Les soumissions de formulaire ne sont acceptées que depuis la même origine.

Il en résulte que le périmètre de l’opérateur devrait être celui d’un commerçant qui laisse la saisie du paiement à l’iframe de PAY.JP (SAQ A). Vérifiez le périmètre qui s’applique et le type de questionnaire d’auto-évaluation auprès des consignes de PAY.JP et des exigences de vos réseaux de cartes.
Pour cela, il est utile de conserver une trace de ces points : les URL des pages de paiement (`/payments/pages/payjp/checkout` et `/portal`), la liste des scripts que la page charge (les deux ci-dessus), la CSP de la page, et le fait que le trafic vers le serveur utilise TLS (la configuration du reverse proxy).

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

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. Enregistrer le webhook

Sous « Settings > Webhook » dans le tableau de bord PAY.JP, ajoutez une destination. Enregistrez séparément le mode test et le mode live.

- URL : `https://<server domain>/payments/webhooks/payjp`
- Mode : le même que les clés

Un webhook PAY.JP identifie son expéditeur par le **jeton d’origine** du compte (`whook_...`, dans l’en-tête `X-Payjp-Webhook-Token`). Transmettez le jeton affiché dans le tableau de bord dans la variable d’environnement `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (ou `_FILE`).
Le point de terminaison compare le jeton en temps constant, et rejette avec `400` un événement de test reçu avec une clé live (et inversement). Il n’y a ni signature ni horodatage : toute personne qui connaît le jeton peut donc envoyer une notification, mais le serveur n’écrit jamais d’état à partir du contenu d’une notification ; il ne fait que relire l’état depuis PAY.JP.

Le serveur utilise ces événements (les autres types sont acceptés mais non enregistrés).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP ne renvoie une notification que toutes les 3 minutes, au plus 3 fois, et une notification au-delà n’arrive pas tant que vous ne la renvoyez pas depuis le tableau de bord. La réconciliation répare ce qui a été manqué : **il est donc recommandé de définir `payments_reconcile_interval` sur 1 heure**.
L’état du prestataire (le mode, la date du dernier webhook reçu et le résultat de la dernière réconciliation) est affiché dans la section « Prestataire de paiement » de `/admin/supporters` dans l’administration.

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

<!-- source: 7-operation fbdd81461f80 -->
## 7. Exploitation

- Un échec de réconciliation de type `permission` signifie une mauvaise clé secrète.
- `duplicate_charge` dans les avertissements de la section « Prestataire de paiement » signifie que le serveur n’a pas pu finir de rembourser un paiement pour un souscription fermé. La ligne de journal `a second subscription could not be canceled and refunded` nomme la souscription, et chaque réconciliation réessaie, y compris après la suppression de la souscription. Remboursez le prélèvement dans le tableau de bord si cela persiste : la tentative suivante le trouve remboursé et efface l’avertissement, tant que PAY.JP liste encore les prélèvements de la souscription supprimé (voir [Points à vérifier](#items-to-verify)). Un prélèvement de plus de 180 jours ne peut pas être remboursé : le serveur le journalise et continue.
- Les autres remboursements se traitent dans le tableau de bord PAY.JP. Un remboursement intégral et une rétrofacturation retirent le rôle et suppriment la souscription immédiatement, et un remboursement partiel ne retire pas le rôle.
- **L’annulation d’une rétrofacturation** n’est pas enregistrée automatiquement, car PAY.JP n’émet ni événement ni API pour cela. Quand cela arrive, enregistrez le montant annulé sur l’écran des revenus de la page des coûts (`finance`). Le rôle n’est pas rétabli automatiquement.
- **Les frais de formule de soutien payante de PAY.JP et les frais de virement** ne sont pas déduits des revenus. Enregistrez-les comme des coûts dans `finance`.
- Quand les réconciliations atteignent souvent la limite de requêtes, augmentez `payments_reconcile_max_requests` ou allongez `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Changements de formule de soutien

Une montée en gamme prélève d’abord la différence au prorata et ne déplace la formule de soutien qu’une fois ce prélèvement réussi. Quand le prélèvement est refusé, la formule de soutien reste et l’utilisateur voit une erreur avec le moyen de mettre à jour sa carte, sans délai de grâce ni notification de paiement échoué, et un nouvel essai avec une bonne carte prélève une seule fois. Si la formule de soutien ne se déplace pas après le prélèvement, un nouvel essai ne prélève pas de nouveau. Une différence inférieure au minimum de 50 yens, et un passage au prix annuel de la même formule de soutien, qui n’a pas de différence positive, ne prélèvent rien maintenant et déplacent la formule de soutien immédiatement ; le nouveau prix est prélevé à la fin de la période, et un échec à ce moment est un renouvellement échoué.

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## Points à vérifier

Les comportements suivants ne sont tranchés ni par la documentation de PAY.JP ni par le mode test.

- Comment sont calculés les frais restitués par une rétrofacturation, et si le relevé d’une période correspond aux frais du serveur (le mode test a montré des frais arrondis à l’inférieur, et des frais de remboursement calculés à partir de son montant).
- Ce que fait une vraie rétrofacturation : PAY.JP ne documente aucun moyen d’en provoquer une en mode test, ni de corps pour `dispute.created`. Un envoi de test de l’événement depuis le tableau de bord reçoit un `200` et ne change rien.
- Jusqu’où remonte la liste des événements.
- Si la suppression d’un Plan affecte les souscriptions existants (le serveur n’en supprime jamais), et la longueur maximale d’un ID de Plan.
- Si le rapport de CSP au chargement de la page de paiement importe : `pay.js` applique un style en ligne contre `style-src 'self'`, alors que les champs de carte et le 3-D Secure fonctionnent dans Chromium. Les autres navigateurs n’ont pas été vérifiés.
- Si payjp-cli peut transférer les événements v1.
- Qu’un prélèvement sur la carte enregistrée d’un client ne demande pas de 3-D Secure sauf si la requête le demande. Un prélèvement refusé est listé par `GET /v1/charges` avec `paid: false`, et le serveur ignore celui qui n’est pas payé.
- Que `GET /v1/charges` avec `subscription` liste encore les prélèvements d’un souscription supprimé. Le serveur rembourse avant de supprimer : seul un essai après un remboursement définitivement échoué repose donc là-dessus. La référence de l’API documente le filtre et ne dit rien des souscriptions supprimés. Si la liste revient vide, le serveur signale un échec au lieu d’un remboursement, et `duplicate_charge` demeure même après le remboursement manuel du prélèvement.
