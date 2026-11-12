<!-- source: closing-the-service 30c438995ba1 -->
# Fermer le service

Ce qu’il faut faire, dans l’ordre, lorsque vous cessez d’exploiter le serveur : laisser les utilisateurs récupérer leurs données, supprimer les comptes qui subsistent sur d’autres serveurs,
puis arrêter le serveur et passer à une page de fermeture statique.
Chaque commande est décrite plus en détail dans [administrateurs et rôles](admin-cli.md#announcing-the-end-of-the-service).

<!-- source: the-sequence 4557c35b6851 -->
## La séquence

![Les étapes de la fermeture du service de haut en bas : ce que fait l’opérateur à gauche, et l’état du serveur à droite, avec la date de fermeture et la date limite d’export entre les deux. Le tableau ci-dessous décrit chaque étape.](images/service-closure.svg)

Le tableau suit le schéma de haut en bas ; la colonne Serveur commence par l’état qu’affiche `my admin closure status`.
La date de fermeture (`-closes-at`) met fin à la période de préavis, et la date limite d’export (`-export-until`, 30 jours après la date de fermeture
sauf indication contraire) met fin à l’étape 5.

| Étape | Serveur | Ce que fait l’opérateur |
| --- | --- | --- |
| 1. Annoncer | `announced`. Fonctionne comme d’habitude. Les inscriptions, les invitations et les nouvelles souscriptions de soutien s’arrêtent | `my admin closure announce`, publier un avis sur la page d’état |
| 2. Période de préavis | `announced`. Fonctionne comme d’habitude. Les utilisateurs peuvent exporter leurs données et migrer vers un autre serveur | Rembourser les soutiens annuels, vérifier les migrations et les conservations légales |
| 3. Fermer (après la date de fermeture) | `closed`. Les utilisateurs peuvent seulement se connecter et exporter. Le serveur livre un `Delete` pour chaque compte | `my admin closure check` et `close` |
| 4. Livraison terminée | Enregistre `delivery_completed` ; les Actors locaux renvoient aussi `410` | Confirmer avec `my admin closure status`. Ne pas arrêter les workers |
| 5. Jusqu’à la date limite d’export | `delivery_completed`. Reste fermé et continue de proposer l’export | Exporter les enregistrements soumis à une conservation légale |
| 6. Page statique | Arrêté. Le frontal sert la page statique, et `410` pour la fédération | `my admin closure export-html`, basculer le frontal, arrêter `my serve` |
| 7. Mise au rebut | Aucun. Continuez de servir la page statique et `410` pendant environ un an | Mettre au rebut la base de données, les blobs et les sauvegardes. Conserver le domaine pendant environ un an |

Vous pouvez combiner n’importe quelle étape avec le mode maintenance ([Maintenance](maintenance.md)). Pendant la maintenance, les exports et la livraison des activités `Delete` s’arrêtent.

<!-- source: 1-announce b34cb9474667 -->
## 1. Annoncer

Fixez la date de fermeture (`-closes-at`) et la date limite pour récupérer les données (`-export-until`, 30 jours après la date de fermeture si elle est omise), puis annoncez-les.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- Il n’y a pas de borne inférieure pour la date de fermeture, mais un avertissement apparaît lorsque le délai entre l’annonce et la date de fermeture est inférieur à 30 jours. Laissez aux utilisateurs le temps d’exporter et de migrer.
- Pour changer la date, relancez `announce` ; pour l’annuler, exécutez `my admin closure withdraw`. L’une comme l’autre envoie à chaque utilisateur une notification `service_closure`.
- Une fois l’annonce faite, le mode d’inscription effectif devient `closed`, les demandes en attente sont rejetées, et chaque souscription de soutien en cours est annulée à la fin de sa période (les souscripteurs reçoivent une notification et un e-mail).
  Retirer l’annonce n’annule ni les rejets ni les annulations.
- Les notifications sont créées par des tâches ; n’arrêtez donc pas les workers.
- Sur la page d’état ([supervision et page d’état](monitoring.md)), publiez la date de fermeture, la date limite d’export, et la façon d’exporter et de migrer.
  La page d’état survit au serveur ; elle guide donc aussi les personnes qui la consultent après la fermeture.

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. Période de préavis

- Les utilisateurs peuvent exporter leurs données (l’export de données et les fichiers CSV) et migrer vers un autre serveur (`Move`) depuis l’écran des paramètres.
  Une bannière sur chaque écran et une notification indiquent la date de fermeture, la date limite d’export et le chemin vers cet écran.
- **Soutiens annuels** : l’annonce annule les souscriptions à la fin de la période, mais pour une souscription dont la période se termine après la date de fermeture, la durée pendant laquelle il ne peut pas être utilisé a déjà été payée.
  Le serveur ne rembourse pas ; remboursez donc chaque souscripteur dans l’écran du fournisseur de paiement (dans Stripe, l’écran des paiements du tableau de bord ; dans PAY.JP, l’écran des paiements de la console d’administration) ([paiements de soutien (Stripe)](payments-stripe.md), [paiements de soutien (PAY.JP)](payments-payjp.md)).
- Une fois la date de fermeture passée, les personnes disposant de `manage_service_closure` reçoivent une notification `admin_service_closure_due`. Le serveur ne se ferme pas tout seul.

<!-- source: 3-close c851780cf430 -->
## 3. Fermer

Une fois la date de fermeture passée, exécutez les vérifications avant de fermer.

```sh
my admin closure check
```

- **Les migrations non terminées** produisent un avertissement. Attendez qu’elles se terminent avant de fermer, car après la fermeture personne ne peut démarrer un `Move`.
- **Les conservations légales (`retention_holds`)** sur un compte quelconque produisent un avertissement.
  L’obligation qui motive une conservation ne prend pas fin lorsque vous fermez le serveur ; exportez donc les enregistrements soumis à une conservation et stockez-les hors du serveur (listez les conservations avec `my admin retention list`).

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

Lorsque les vérifications sont terminées, fermez. Cela ne peut pas être annulé. Confirmez en saisissant `y` et le nom de domaine (utilisez `-confirm-domain` dans l’automatisation).

```sh
my admin closure close
```

Après la fermeture, les utilisateurs peuvent seulement se connecter et exporter leurs données. Les souscriptions de soutien restantes sont annulées immédiatement,
et un `Delete` (Actor) pour chaque compte local est livré aux autres serveurs (même lorsque `federation_mode` vaut `off` ou `inbound`).
L’inbox et les GET des publications et des collections renvoient `410 Gone`.

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. Confirmer que la livraison est terminée

**N’arrêtez pas `my serve` ni les workers (les processus dont le `role` est `all` ou `worker` ; avec une file de type push, la file Cloud Tasks) tant que l’état n’est pas `delivery_completed`.**
Les workers livrent les activités `Delete`, et pendant ce temps le serveur continue de renvoyer le document et la clé de l’Actor local afin que les destinataires puissent vérifier la signature du `Delete`.
Si vous arrêtez trop tôt, les comptes subsistent sur d’autres serveurs.

```sh
my admin closure status
```

- `accounts with a Delete` est le nombre de comptes pour lesquels un `Delete` a été enregistré, `fan-outs pending` est le travail de diffusion restant, et `deliveries <state>` est le nombre de livraisons par état.
- Les livraisons sont retentées selon la politique habituelle (jusqu’à 9 fois sur environ 4 jours). Lorsque toutes sont terminées, la vérification qui s’exécute toutes les 10 minutes enregistre `delivery_completed` (`state: delivery_completed`).
  À partir de là, les Actors locaux et l’Actor de l’instance renvoient aussi `410`.
- Une livraison passée à l’état `dead` n’est pas retentée automatiquement. Pour la retenter, utilisez `my job retry`.

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. Jusqu’à la date limite d’export

Même une fois la livraison terminée, le serveur continue de fonctionner fermé jusqu’à `export_until` et continue de servir les exports des utilisateurs.
Le serveur ne supprime jamais de données de lui-même ; il continue donc de fonctionner au-delà de la date limite jusqu’à ce que vous l’arrêtiez.
Si vous n’avez pas encore exporté les enregistrements soumis à une conservation légale (§3), faites-le maintenant.

<!-- source: 6-switching-to-the-static-page 9795c9aea7ae -->
## 6. Passer à la page statique

Après la date limite d’export, exportez la page de fermeture. **Cela lit la base de données ; exécutez-le donc avant d’arrêter `my serve` et de mettre la base de données au rebut.**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- Ce qu’elle écrit :
  - `<dir>/<locale>.html`, et `<dir>/index.html`, qui liste toutes les langues dans l’ordre. Ils utilisent le même modèle et le même CSS que la page de maintenance (`my maintenance export`) et affichent
    le nom du serveur (`maintenance_server_name`, sinon le nom de l’instance, sinon le nom d’hôte), la date de fermeture, le message de l’opérateur issu de l’annonce, et un lien vers la page d’état (`status_page_url`).
    Le CSS est uniquement en ligne, il n’y a ni scripts ni ressources externes, et la CSP est dans une balise `<meta>`. Les remplacements de couleurs du thème sont intégrés ; le CSS personnalisé n’est pas inclus.
  - Des exemples de configuration du frontal : `nginx.conf`, `Caddyfile` et `cloudfront-function.js`. Chacun renvoie `410` sans corps pour les chemins de fédération et de découverte (`/ap/*`, `/inbox`, `/.well-known/webfinger`,
    `/.well-known/host-meta`, `/.well-known/host-meta.json`, `/.well-known/nodeinfo`, `/nodeinfo/*`) et pour les profils (`/@*`),
    et pour tout autre chemin renvoie la page de la première langue de `Accept-Language` (ou `index.html` s’il n’y en a pas).
    Une langue atteint la page de la langue dans laquelle l’application l’afficherait : `zh`, `zh-SG` et `zh-Hans` obtiennent `zh-CN.html`, et `zh-Hant`, `zh-HK` et `zh-MO` obtiennent `zh-TW.html`.
- La commande refuse de s’exécuter si le serveur n’est pas fermé (code de sortie 1). Avant `delivery_completed`, elle avertit que les livraisons se poursuivent (et écrit quand même les fichiers).

Basculez le frontal. Ne mettez que les pages (`*.html`) à l’emplacement public, et adaptez le nom d’hôte, le répertoire et les chemins de certificats de l’exemple à votre environnement.

| Frontal | Comment le configurer |
| --- | --- |
| nginx | Placez les pages dans `/srv/closure` et incluez `nginx.conf` depuis le bloc `http`. Les pages sont aussi servies avec `410` |
| Caddy | Placez les pages dans `/srv/closure` et utilisez le `Caddyfile`. Caddy obtient le certificat, et les pages sont aussi servies avec `410` |
| CloudFront | Placez les pages directement sous un bucket S3, faites de ce bucket l’origine du comportement de cache par défaut, et associez `cloudfront-function.js` comme CloudFront Function de requête du visiteur (`cloudfront-js-2.0`). Mettez `Accept-Language` dans la clé de cache de la politique de cache. Les pages sont servies avec `200` (une fonction de requête du visiteur ne peut pas modifier le statut de la réponse de l’origine) |

Après la bascule, vérifiez que `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` renvoie `410` et
que `curl -i https://social.example/` renvoie la page de fermeture. Arrêtez ensuite `my serve` et les workers.
Pour la surveillance de disponibilité, arrêtez les sondes de contrôle de santé ou faites-les pointer vers la page de fermeture.

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. Mettre au rebut la base de données, les blobs et les sauvegardes

Après l’arrêt, mettez au rebut la base de données, les blobs (médias et fichiers d’export), le magasin KV et les files, avec leurs sauvegardes et instantanés. Avec un [moteur de recherche](search.md#closing-the-service), mettez aussi au rebut son index et ses instantanés : ils contiennent le texte des publications.
Avant de mettre quoi que ce soit au rebut, confirmez une dernière fois que vous avez exporté et stocké les enregistrements soumis à une conservation légale. La mise au rebut ne peut pas être annulée.

**Conservez le domaine, et continuez de servir la page statique et `410` pendant environ un an.** Les autres serveurs voient le `410`, cessent de récupérer et de livrer, et vident leurs caches.
Les URI fédérées utilisent des UUID ; il y a donc peu de risque qu’un Actor avec le même `id` soit créé à nouveau,
mais si quelqu’un d’autre prend le domaine, l’identifiant (`acct:alice@social.example`) et les URL de profil deviennent les siens et peuvent servir à usurper l’identité de vos utilisateurs.
Continuez à renouveler le domaine et le certificat, et réfléchissez à nouveau avant de lâcher le domaine, même une fois l’année écoulée.
