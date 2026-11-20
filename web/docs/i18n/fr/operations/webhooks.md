<!-- source: server-webhooks e73d0e0966b7 -->
# Webhooks du serveur

Les webhooks envoient les signalements, les demandes d’inscription, les créations de comptes et les modifications de profil, les mesures de modération, les contestations, les mesures sur les domaines, les publications publiques locales et les actions de gestion des émojis personnalisés vers un relais Slack ou Discord, un système de tickets ou un outil d’automatisation comme n8n. Un serveur sans webhook configuré ne stocke aucune ligne supplémentaire et n’exécute aucune tâche supplémentaire. Ce que reçoit un destinataire (en-têtes, signature, événements, corps) est décrit dans la [référence des webhooks](../reference/webhooks.md).

<!-- source: set-up-a-webhook 20c889b84876 -->
## Configurer un webhook

Toute personne ayant la permission `manage_webhooks` peut configurer des webhooks sur l’écran d’administration « Webhooks » (`/admin/webhooks`).
Le rôle intégré `admin` l’a. « Configurer un webhook » ouvre le formulaire ; choisir un webhook dans la liste ouvre sa page, où
vous le testez, le désactivez et le réactivez, le modifiez, voyez ses livraisons des 14 derniers jours et en renvoyez une, lui donnez un nouveau
secret et le supprimez après avoir saisi le nom d’hôte de son URL. La page indique lorsque les livraisons échouent de façon répétée, et lorsque le
serveur a désactivé le webhook. L’historique affiche l’événement de chaque livraison, son état, ses tentatives et sa dernière tentative, ainsi que le corps
qu’il a envoyé, qui peut contenir des données personnelles et n’est montré qu’à une personne qui a aussi la permission dont son événement a besoin (ci-dessous).

1. Créez un webhook avec une URL de destination, une description et les événements auxquels s’abonner. Chaque événement exige aussi sa propre
   permission en plus de `manage_webhooks` : `manage_reports` pour les signalements, `manage_accounts` pour les inscriptions, les comptes,
   les mesures de modération et les contestations, `manage_domains` pour les actions de domaine, `manage_custom_emojis` pour les émojis personnalisés, et
   `manage_errors` pour les groupes d’erreurs.
   Les publications publiques n’exigent aucune permission supplémentaire. Vous ne pouvez pas sélectionner un événement dont vous n’avez pas la permission.
2. Le secret (`whsec_...`) est affiché une seule fois, à la création du webhook. Définissez-le sur le récepteur. Il ne peut pas être affiché de nouveau.
3. Sur la page du webhook, choisissez « Envoyer un test » pour envoyer `webhook.test`, et vérifiez dans ses livraisons qu’il est bien arrivé.

La création, la modification, la suppression, l’activation, la désactivation et le renouvellement du secret d’un webhook sont enregistrés dans le journal d’audit.
La cible est affichée sous la forme de la description et du nom d’hôte de l’URL ; l’URL elle-même n’est pas enregistrée.

<!-- source: rotate-the-secret fc0bedd73653 -->
## Renouveler le secret

« Créer un nouveau secret » crée un nouveau secret et l’affiche une seule fois. Pendant les 24 heures suivantes, les messages sont aussi signés avec l’ancien secret
(`webhook-signature` porte deux signatures), de sorte que vous pouvez faire passer le récepteur au nouveau secret dans cette fenêtre.
Les secrets sont scellés avec `kernel.Sealer` avant d’être stockés dans la base de données ; la clé est la
[KEK](key-management.md).

<!-- source: failures-and-redelivery 65aba0d2089e -->
## Échecs et nouvelles livraisons

- Si la destination répond autre chose que `2xx`, ne répond pas dans les 15 secondes ou est injoignable, le message est réessayé après 5 secondes, 5 minutes, 30 minutes, 2 heures, 5 heures, 10 heures, puis 10 heures (8 tentatives au total, environ 27 heures). Une livraison qui les a toutes épuisées devient une tâche morte, et « Renvoyer » dans les livraisons la renvoie avec le même `webhook-id`.
- Si la destination répond `410 Gone`, le webhook est désactivé immédiatement.
- Le serveur désactive une destination qui n’a pas réussi depuis 72 heures après son premier échec et qui a échoué 20 fois de suite ou plus. Le journal d’audit reçoit une entrée `webhook_disabled` par `system`, et toutes les personnes disposant de `manage_webhooks` reçoivent une notification dans l’application (et un e-mail, si elles ont choisi de recevoir les notifications par e-mail).
- La liste de l’écran d’administration affiche le nombre d’échecs consécutifs et la raison pour laquelle le serveur a désactivé un webhook (`gone` ou `failing`). Réactivez le webhook après avoir corrigé la destination. Les messages de la période où il était désactivé ne sont pas renvoyés.
- Les messages et les livraisons sont supprimés après 14 jours (valeur fixe). Les corps contiennent des données personnelles, comme le commentaire d’un signalement.

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## Destinations sur un réseau interne

La livraison passe par la protection SSRF des requêtes HTTP sortantes. Par défaut, elle n’atteint que des adresses publiques en `https` (ports 443 et 80). Pour envoyer vers une destination interne, comme un n8n sur le même hôte ou un chat dans votre VPC, listez des noms d’hôte, des adresses IP et des CIDR, séparés par des virgules, dans le paramètre `webhook_allowed_destinations` (variable d’environnement `MY_WEBHOOK_ALLOWED_DESTINATIONS` ; voir la [référence de configuration](../reference/configuration.md#webhook_allowed_destinations)).

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- Seules les destinations listées, et uniquement pour la livraison des webhooks, échappent à la protection. La fédération, les médias et les aperçus de liens ne changent pas. Une destination exemptée peut aussi utiliser `http` et n’importe quel port.
- Un nom d’hôte est autorisé quelle que soit l’adresse vers laquelle il se résout. Un CIDR est autorisé lorsque l’URL désigne une adresse IP de cette plage. Si vous utilisez un nom d’hôte qui se résout dans la plage, listez aussi ce nom d’hôte.
- Seul un opérateur ayant accès au serveur peut modifier ce paramètre. Les permissions de l’écran d’administration ne suffisent pas à atteindre un réseau interne.
- Une tentative vers une destination que la protection a refusée affiche `forbidden_destination` dans les livraisons.
- Le paramètre de développement `http_allow_private_networks` lève la protection pour tous les usages, webhooks compris.
