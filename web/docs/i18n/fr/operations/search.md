<!-- source: full-text-search-of-posts fc16f55b56ea -->
# Recherche en texte intégral des publications

Ce que contient la recherche en texte intégral des publications, comment l’activer et la modifier, comment reconstruire son index, et ce qu’elle demande à vos journaux et à vos sauvegardes.

<!-- source: choosing-an-engine 9cfd51361e0a -->
## Choisir un moteur

La recherche en texte intégral des publications exige un moteur de recherche que vous exécutez ou que vous louez. [`search`](../reference/configuration.md#search) le choisit :

- `none`, la valeur par défaut : pas de recherche en texte intégral. Le serveur ne se connecte à rien, ne met aucune tâche de recherche en file, et le client n’offre aucun moyen de rechercher des publications. Sur un tel serveur, chaque commande `my search` le dit et ne fait rien.
- `elasticsearch`, `opensearch` ou `meilisearch` : ce moteur, avec les paramètres de sa section dans [Moteurs de recherche](search-engines.md).
- `plugin` : un programme de votre cru qui tient l’index dans tout autre moteur, comme Groonga ou un index construit avec Sudachi, à `plugin_search_address` ([Rechercher des publications avec un plugin](plugins.md#searching-posts)).

La recherche de comptes et de hashtags fonctionne à partir de la base de données et ne dépend pas de ce paramètre.

Le serveur ne possède que les champs qu’il écrit, filtre, trie et pagine. La façon dont le moteur analyse le texte, classe les résultats et la taille de son index vous appartiennent, via un fichier de paramètres du moteur ; [Moteurs de recherche](search-engines.md) décrit ce que chaque moteur trouve en japonais avec les paramètres intégrés et comment les modifier.

<!-- source: what-the-index-holds 5e0884482ea8 -->
## Ce que contient l’index

La base de données reste la source de vérité, et l’index est une copie que vous pouvez jeter et reconstruire. Un moteur ne renvoie que des ID de publications : le serveur relit chaque publication dans la base de données et applique les règles de visibilité habituelles ; un index en retard peut donc laisser une publication introuvable, mais ne peut pas en montrer une qui ne devrait pas être vue.

Une publication est dans l’index quand toutes ces conditions sont réunies :

- Elle n’est pas supprimée.
- Sa visibilité est `public` ou `local`, et [`search_visibilities`](instance-settings.md) l’inclut. Les publications `unlisted`, `private` et `direct` ne sont jamais indexées, quel que soit le paramètre.
- Son auteur a activé `indexable`, le paramètre décrit dans [Moteurs de recherche et robots d’IA](crawlers.md).
- Son auteur, et le domaine de son auteur, n’ont aucune restriction qui les masque de la découverte.

L’index contient le corps, l’avertissement de contenu et les textes alternatifs de la version actuelle de chaque publication, avec son auteur et sa langue. Il ne contient ni les sondages, ni les profils, ni les versions plus anciennes.

`indexable_default` vaut `false` par défaut : un serveur qui vient d’activer la recherche trouve donc peu de choses tant que les personnes n’ont pas choisi `indexable` elles-mêmes.

<!-- source: turning-it-on 3d88170680a2 -->
## L’activer

1. Exécutez le moteur, et définissez `search` et les paramètres de son backend.
2. Si vous exécutez les workers séparément, décidez de [leur répartition](#keeping-search-from-delaying-other-jobs).
3. Configurez [vos journaux](#the-query-appears-in-urls).
4. Si le moteur est exploité par quelqu’un d’autre, ajoutez-le à vos [documents de confidentialité](#your-privacy-documents).
5. Redémarrez le serveur : dès lors, les nouvelles publications entrent dans l’index.
6. Exécutez `my search rebuild` pour ajouter les publications existantes.

La recherche fonctionne pendant la reconstruction, et ses résultats sont incomplets jusqu’à sa fin.

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## Reconstruire l’index

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

La commande rend la main dès que la reconstruction est en cours, et le worker fait le reste sous forme de tâches `search.rebuild` ([référence des tâches](../reference/jobs.md#full-text-search)) : gardez donc un worker en marche. Chaque tâche couvre 100 publications.

- **Débit.** La valeur par défaut est de 20 publications par seconde, au plus 1 000, et elle compte chaque publication `public` et `local` examinée, y compris celles qui ne sont pas éligibles. Au débit par défaut, un million de publications prennent environ 14 heures. Il laisse de la place aux autres tâches et à la base de données : ne l’augmentez que si votre serveur a de la marge.
- **Laquelle exécuter.** La commande par défaut retire tout ce qui ne devrait pas se trouver dans l’index : les documents laissés par une suppression qui n’est jamais arrivée, ou par la restauration de la base de données depuis une sauvegarde. La recherche trouve moins de choses jusqu’à sa fin. Utilisez `--keep` après [l’ajout d’une visibilité](#changing-search_visibilities), quand rien n’est à retirer et que la recherche doit rester complète. `--keep` ne retire jamais le document d’une publication qui n’est plus dans la base de données.
- **Interrompue ?** Exécutez de nouveau la même commande. Elle reprend à la dernière publication traitée et ne vide pas de nouveau l’index. Ajoutez `--rate` pour changer le débit. Une reconstruction reprend aussi d’elle-même quand un worker redémarre.
- **L’autre mode.** Tant qu’une reconstruction n’est pas terminée, la commande de l’autre mode est refusée, car elle ne ferait pas ce qu’elle annonce. Pour repartir de zéro dans l’un ou l’autre mode, exécutez `my search clear` puis la commande voulue.
- **L’exécuter deux fois** donne le même index. Les publications créées, modifiées ou supprimées pendant une reconstruction finissent correctes, car leurs propres tâches continuent de s’exécuter et chaque tâche lit la publication telle qu’elle est à cet instant.

`my search status` affiche le backend, `search_visibilities` et, pour la dernière reconstruction, sa phase (`clearing`, `filling` ou `finished`), les publications traitées et en échec, le débit et la dernière erreur.

Quand quelque chose se passe mal :

- **Le moteur est en panne.** Si trois publications de suite échouent, la tâche s’arrête là où elle en est, enregistre l’erreur et réessaie. Une reconstruction qui a épuisé ses tentatives laisse une tâche en échec (`my job list`), et `updated` dans l’état cesse de bouger. Une fois le moteur rétabli, exécutez la même commande pour continuer.
- **Le moteur refuse une publication.** La publication est enregistrée comme en échec et la reconstruction continue. L’état en liste jusqu’à 20. Corrigez la cause, puis exécutez `my search rebuild --keep` pour réessayer toutes les publications.

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## Modifier `search_visibilities`

Retirer une visibilité supprime d’elle-même ses publications de l’index. En ajouter une ne trouve que les nouvelles publications, jusqu’à ce que vous exécutiez `my search rebuild --keep`.

**Prévenez vos utilisateurs avant d’ajouter `local`.** `indexable` couvre alors les publications `local` de toutes les personnes qui l’ont déjà activé, et la reconstruction ajoute leurs publications `local` antérieures, sans le leur redemander. Les paramètres du profil disent ce que `indexable` couvre sur votre serveur, mais les personnes qui l’ont choisi il y a longtemps ne le verront pas si vous ne leur dites pas.

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## Désactiver ou changer le moteur

Quand le serveur passe à `search = none` ou à un autre moteur, il perd sa connexion à l’ancien index. Cet index n’est plus nettoyé : les publications supprimées ensuite y restent, texte compris.

1. Arrêtez le serveur et ses workers, afin que rien n’écrive dans l’index après l’étape suivante.
2. Exécutez `my search clear`.
3. Changez `search`, puis démarrez le serveur.
4. Avec un nouveau moteur, exécutez `my search rebuild`.

Si vous avez déjà basculé, ou si vous ne pouvez pas arrêter le serveur, supprimez l’ancien index dans le moteur lui-même. `my search clear` ne peut plus l’atteindre une fois le paramètre changé. Les tâches mis en file avant le changement ne font rien après un passage à `none`.

**Revenir de `none`.** Exécutez `my search rebuild` sans `--keep` : le serveur ne garde aucune trace de ce qui a été supprimé pendant qu’il n’avait pas de moteur, et seul le vidage de l’index retire ces publications. Si une reconstruction n’était pas terminée quand vous êtes parti, exécutez d’abord `my search clear`, car une reprise ne vide pas l’index.

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## Éviter que la recherche retarde les autres tâches

Par défaut, un worker exécute une tâche à la fois : un moteur lent partage donc le temps de ce worker avec `federation.inbox`, même si chaque appel du moteur a une limite de 10 secondes. Un serveur chargé devrait les séparer :

- Exécutez un processus worker avec `worker_job_types = search.*` et les autres avec `worker_job_types_except = search.*` ([Répartir les workers par type de tâche](running.md#splitting-workers-by-job-type)).
- Avec `queue = cloudtasks`, envoyez les types `search.*` vers leur propre file avec une [route de file](running.md#routing-job-types-to-their-own-queues).

Un petit serveur peut les laisser ensemble, et les tâches de recherche partagent alors le temps avec la fédération.

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## La requête apparaît dans les URL

`GET /api/v1/statuses/search` transporte dans son URL ce que les gens recherchent, sous la forme `?q=`. La ligne de journal `http request` du serveur contient le chemin et la route, pas la query string, et les traces et les rapports d’erreurs l’omettent aussi. Tout ce qui se trouve devant le serveur peut toutefois l’enregistrer, et une recherche peut en dire long sur une personne. Vérifiez chaque couche de votre installation, et soit gardez la requête hors de son journal, soit traitez ce journal comme des données personnelles, avec un accès et une rétention à l’avenant.

| Couche | Enregistre-t-elle la requête ? | Que faire |
| --- | --- | --- |
| Cloud Run | Oui. Ses journaux de requêtes sont créés automatiquement, et `httpRequest.requestUrl` inclut la query. | Les exclure, comme ci-dessous. |
| Équilibreur de charge Google Cloud | Seulement si vous activez la journalisation du service de backend. Elle est désactivée par défaut sur les nouveaux, et utilise le même champ. | La laisser désactivée, ou les exclure, comme ci-dessous. |
| AWS Application Load Balancer | Seulement si vous activez les journaux d’accès, désactivés par défaut. La ligne de requête contient l’URL telle que le client l’a envoyée. Il n’a aucun moyen de supprimer une partie d’une ligne. | Les laisser désactivés, ou leur donner une règle de cycle de vie courte et un accès limité. |
| CloudFront | Seulement si vous activez la journalisation standard, dans le champ `cs-uri-query`. | Avec la journalisation standard v2, omettez ce champ en sélectionnant les champs. |
| ECS | Aucun journal de requêtes propre : ses journaux contiennent ce que le serveur écrit. | Rien. |
| Cloudflare | Seulement si vous utilisez Logpush, dans le champ `ClientRequestURI`. | Sélectionnez plutôt `ClientRequestPath`. |
| Caddy | Seulement avec une directive `log`, que `compose.caddy.yaml` n’a pas. | Si vous en ajoutez une, retirez le paramètre, comme ci-dessous. |
| Autres reverse proxies | Généralement oui : leurs formats par défaut journalisent toute la ligne de requête. | Journaliser le chemin sans la query pour cette route, ou désactiver leur journal d’accès. |

Sur Google Cloud, cela garde les deux journaux hors du bucket `_Default` du projet. Ajoutez la même exclusion à tout sink de votre cru :

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

Dans Caddy :

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## Sauvegardes

L’index contient le texte des publications : il demande donc les mêmes précautions que la base de données — qui peut y accéder, où il est conservé et combien de temps.

Un instantané du moteur est la partie à surveiller. Une publication supprimée de l’index reste dans tous les instantanés pris auparavant, comme dans les sauvegardes de la base de données ; donnez donc aux instantanés la même rétention qu’à vos [sauvegardes de la base de données](backup.md).

Vous n’avez pas besoin d’instantanés pour restaurer : `my search rebuild` refait l’index à partir de la base de données. Après avoir restauré la base de données depuis une sauvegarde, exécutez-le sans `--keep`, afin que les publications plus récentes que la sauvegarde quittent l’index.

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## Vos documents de confidentialité

Si le moteur tourne sur le service d’une autre entreprise, celle-ci reçoit le texte des publications couvertes par `search_visibilities`, y compris les publications `local` si vous les incluez. Nommez-la comme destinataire dans la [politique de confidentialité](privacy-policy-template.md) et dans le [registre des activités de traitement](processing-record-template.md). Un moteur que vous exécutez vous-même n’est pas un nouveau destinataire.

Indiquez aussi ce que vous avez décidé au sujet des [journaux qui contiennent la requête](#the-query-appears-in-urls), si vous les conservez.

<!-- source: closing-the-service 343ec69df255 -->
## Fermer le service

Le serveur ne supprime pas lui-même les données quand il ferme : jetez donc l’index et les instantanés du moteur avec la base de données et les médias ([Fermer le service](service-closure.md#7-discarding-the-db-blobs-and-backups)).
