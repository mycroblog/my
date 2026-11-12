<!-- source: search-engines 6816d50f42dd -->
# Moteurs de recherche

Comment exploiter chaque moteur de recherche que la recherche plein texte des publications peut utiliser, quelle taille il lui faut, et jusqu’où il trouve le japonais et les autres langues écrites sans espaces. Pour un moteur qui n’est pas décrit ici, comme Groonga ou un index construit avec Sudachi, [un plugin de recherche](plugins.md#searching-posts) de votre cru peut tenir l’index à la place. [La recherche plein texte des publications](search.md) décrit ce que contient l’index, l’activation de la recherche, la reconstruction de l’index et le changement de moteur, quel que soit le moteur.

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` conserve l’index dans un cluster Elasticsearch de version 8 ou ultérieure, autogéré ou sur Elastic Cloud Hosted. Il a été vérifié avec Elasticsearch 9.5 et 8.19. Le serveur possède les champs de l’index, qu’il écrit, filtre, trie et pagine. Vous pouvez posséder ses réglages : l’analyse, les shards et les réplicas. Sans réglages de votre part, les réglages intégrés n’exigent aucun plugin et trouvent le japonais comme [décrit plus bas](#how-the-built-in-analysis-finds-japanese).

<!-- source: setting-it-up 3e02c35ff319 -->
### Mise en place

1. Lancez Elasticsearch. Un seul nœud suffit pour la plupart des serveurs. Pour un démarrage rapide sur le même hôte :

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   Avec la sécurité activée, ce qui est le cas par défaut, Elasticsearch sert du HTTPS avec un certificat de sa propre autorité. Copiez cette autorité hors du conteneur pour `search_elasticsearch_ca_file` :

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. Créez une clé d’API qui ne peut gérer que les index du serveur. `manage`, `read` et `write` sur l’alias et les index qui le sous-tendent sont tout ce dont elle a besoin :

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   La valeur `encoded` de la réponse est la clé.

3. Définissez la configuration du serveur et redémarrez-le :

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. Exécutez `my search rebuild` pour ajouter les publications existantes, comme dans [l’activation](search.md#turning-it-on).

Les paramètres :

| Paramètre | Ce qu’il fait |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | L’URL de l’API REST. Utilisez `https:` dès que les requêtes traversent un réseau que d’autres partagent ; `http:` est accepté pour un réseau privé ou un cluster dont la sécurité est désactivée. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | La clé d’API, qui est le meilleur choix. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | Un utilisateur et un mot de passe pour l’authentification basique à la place d’une clé. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | Le nom de l’alias, `my-statuses` par défaut. Donnez à chaque serveur qui partage un cluster son propre nom, et accordez à sa clé ce nom suivi de `*`. |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | Un fichier JSON de vos propres réglages de l’index, comme dans [ajuster l’analyse](#tuning-the-analysis-of-elasticsearch). |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | L’autorité de certification d’un cluster HTTPS dont le certificat n’est pas approuvé par le système. |

Les identifiants sont lus uniquement depuis l’environnement, comme des secrets, et n’apparaissent jamais dans les journaux ni les erreurs du serveur. Une erreur du cluster atteint les journaux sous la forme du chemin de la requête, de son statut et du type de l’erreur, comme `index_not_found_exception`, jamais avec le texte d’une publication ou d’une requête. Ce n’est que lorsque le cluster refuse de créer l’index que l’erreur cite sa raison, qui ne nomme que les réglages et les champs.

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### L’index et ses mises à niveau

Le serveur crée et maintient lui-même l’index. Ne le créez pas à la main et ne laissez rien d’autre y écrire.

- Au démarrage, le serveur cherche l’alias. S’il n’y en a pas, il crée un index nommé d’après l’alias, la version de ses champs et un hachage de ses réglages, comme `my-statuses-v1-1f0c2e9a7b3d4c5e`, et fait pointer l’alias vers lui. La version et le hachage sont conservés dans le `_meta` de l’index.
- Le serveur ne peut pas démarrer tant que le cluster est injoignable, refuse sa clé ou refuse les réglages. `my config check` vérifie la configuration et que le fichier de réglages est un unique objet JSON, mais pas ce qu’Elasticsearch en pense : vérifiez donc la connexion et les réglages en démarrant le serveur.
- Un index portant le nom de l’alias que le serveur n’a pas créé, comme un index qu’Elasticsearch a créé de lui-même, interrompt le démarrage avec une erreur qui le nomme. Supprimez cet index, ou choisissez un autre `search_elasticsearch_index`.
- Lorsque les réglages diffèrent de ceux avec lesquels l’index a été créé, parce que vous avez modifié le fichier de réglages ou qu’une nouvelle version de `my` a changé les réglages intégrés, le serveur crée un nouvel index vide avec les nouveaux réglages, fait pointer l’alias vers lui, supprime l’ancien et consigne un avertissement qui demande `my search rebuild`. Exécutez-le. Tant que la reconstruction n’est pas terminée, la recherche ne trouve que les publications écrites depuis. Lorsque Elasticsearch refuse les nouveaux réglages, le serveur ne démarre pas et l’ancien index reste.
- Lorsqu’une nouvelle version de `my` change les champs, le serveur met l’index à niveau au démarrage. Un changement qui ne fait qu’ajouter des champs est fait sur place, et la recherche continue de fonctionner ; les notes de version indiquent quand les publications déjà dans l’index ont besoin de `my search rebuild --keep` pour gagner les nouveaux champs. Tout autre changement des champs demande un nouvel index vide et une reconstruction, comme ci-dessus. Les notes de version indiquent quand une version fait cela.
- Un index créé par une version plus récente de `my` empêche une version plus ancienne de démarrer. Pour revenir en arrière à travers un tel changement, supprimez l’index de l’alias dans Elasticsearch (trouvez-le avec `GET _alias/my-statuses`), démarrez la version plus ancienne, qui crée le sien, et exécutez `my search rebuild`.
- `my search clear` fait pointer l’alias vers un nouvel index vide et supprime l’ancien en une seule étape, ce qui est plus rapide que de supprimer chaque document et rend l’espace disque.

<!-- source: sizing 2b1d9b604953 -->
### Dimensionnement

L’index ne conserve que ce dont une recherche a besoin et aucune copie du texte (`_source` est désactivé). Avec les réglages intégrés, les publications courtes occupent environ 80 à 150 octets de disque chacune une fois qu’Elasticsearch a fusionné ses segments, et les publications plus longues au vocabulaire plus large davantage : prévoyez donc 150 à 300 octets par publication, soit quelques centaines de mégaoctets pour un million de publications. Vérifiez la taille réelle avec `GET _cat/indices/my-statuses*?v`.

Les réglages intégrés donnent à l’index un seul shard primaire, qui contient des dizaines de millions de publications, et un réplica une fois que le cluster a un deuxième nœud (`auto_expand_replicas: 0-1`), si bien qu’un nœud unique reste au vert. Définissez `number_of_shards` et `number_of_replicas` dans un fichier de réglages pour les changer.

Un nœud avec 1 Go de heap Java (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) et environ 2 Go de mémoire au total dessert un serveur petit ou moyen. Donnez à Elasticsearch la moitié de la mémoire de la machine comme heap et laissez le reste au cache de fichiers, et donnez-lui davantage lorsque les recherches ralentissent. Les réplicas demandent plus de nœuds et le même espace disque en plus pour chaque réplica.

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### Comment une requête utilise l’analyse

Chaque publication est indexée dans un seul champ texte, quelle que soit sa langue, et la même analyse s’applique à toutes. La langue d’une publication ne choisit pas un autre analyseur.

- Le texte d’une publication est analysé avec l’analyseur nommé `my_text`.
- Une requête est découpée en mots aux espaces, y compris l’espace idéographique. Chaque mot est analysé avec `my_text_search` lorsque les réglages le définissent, et avec `my_text` sinon, et doit apparaître dans la publication comme une expression : ses jetons dans le même ordre et les uns à côté des autres. Chaque mot doit correspondre. Rien dans une requête n’est interprété comme le langage de requête d’Elasticsearch.
- Les résultats sont ordonnés du plus récent au plus ancien, et non par pertinence : les options de score ne changent donc rien.

Un analyseur décide donc de ce qu’est un mot. Un analyseur qui fait un jeton de chaque caractère trouve n’importe quelle suite de caractères. Un analyseur qui fait des mots de dictionnaire trouve les mots entiers et leurs autres formes.

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### Comment l’analyse intégrée trouve le japonais

Le `my_text` intégré est le tokenizer standard d’Elasticsearch avec les filtres `cjk_width` et `lowercase`, tous intégrés. Le tokenizer fait un jeton de chaque kanji et de chaque hiragana, de chaque suite de katakana, et de chaque mot des langues qui utilisent des espaces.

- Un mot en kanji ou en hiragana est trouvé partout où ses caractères se tiennent ensemble, même à l’intérieur d’un mot plus long : `検索` et `本` trouvent `全文検索の本文です`, et `東京 天気` trouve `東京都の天気は晴れ`. Les caractères qui n’apparaissent que séparément ne correspondent pas.
- Une suite de katakana est mise en correspondance en entier : `カタカナ` trouve `カタカナとABC`, mais `カタ` ne le trouve pas, et `ツリー` ne trouve pas `スカイツリー`.
- Les lettres latines pleine largeur et les katakana demi-largeur correspondent à leurs formes usuelles : `abc` trouve `ＡＢＣ`, et `カタカナ` trouve `ｶﾀｶﾅ`.
- Les hiragana ne trouvent pas les kanji de même lecture : `ぜんぶん` ne trouve pas `全文`. Les autres formes d’un mot ne sont pas trouvées : `走る` ne trouve pas `走った`.
- Une requête d’un seul caractère très courant, comme `の`, trouve un très grand nombre de publications.

Le chinois et le coréen sont trouvés de la même façon, par leurs caractères. Les mots des langues écrites avec des espaces sont trouvés en entier, sans racinisation : `run` ne trouve pas `running`.

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### Ajuster l’analyse d’Elasticsearch

Placez les réglages de l’index dans un fichier JSON et nommez-le dans `search_elasticsearch_index_settings_file`. Le serveur utilise le fichier entier comme `settings` de chaque index qu’il crée, à la place des réglages intégrés : il doit donc contenir tout ce dont l’index a besoin :

- L’analyseur `my_text`, que le champ texte utilise. Sans lui, Elasticsearch refuse de créer l’index et le serveur ne démarre pas.
- Facultativement l’analyseur `my_text_search`, qui analyse alors les mots des requêtes.
- Tout autre réglage d’index, comme `number_of_shards` et `number_of_replicas`. Ce que le fichier omet prend les valeurs par défaut du cluster.
- Seulement une analyse qu’Elasticsearch possède : un plugin que le fichier nomme doit être installé sur chaque nœud.

Le serveur lit le fichier au démarrage. Le modifier donne au prochain démarrage un nouvel index vide, comme dans [l’index et ses mises à niveau](#the-index-and-its-upgrades) : exécutez donc `my search rebuild` après chaque modification. Essayez d’abord une modification sur un autre cluster ou avec un autre `search_elasticsearch_index`.

Des mots de dictionnaire avec le plugin `analysis-kuromoji`, que vous installez sur chaque nœud (`bin/elasticsearch-plugin install analysis-kuromoji`) ou qu’Elastic Cloud Hosted propose comme extension :

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      }
    }
  }
}
```

Chaque suite d’un ou deux caractères, avec la seule analyse intégrée à Elasticsearch :

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "max_ngram_diff": 1,
  "analysis": {
    "tokenizer": {
      "my_grams": { "type": "ngram", "min_gram": 1, "max_gram": 2, "token_chars": ["letter", "digit"] }
    },
    "analyzer": {
      "my_text": { "type": "custom", "tokenizer": "my_grams", "filter": ["cjk_width", "lowercase"] }
    }
  }
}
```

Comparaison, mesurée sur 20 000 publications, moitié en anglais et moitié en japonais, sur Elasticsearch 9.5 :

| | Intégré | Kuromoji | Un et deux caractères |
| --- | --- | --- | --- |
| Plugin | Aucun | `analysis-kuromoji` | Aucun |
| Taille de l’index | 1,5 Mo | 1,4 Mo | 3,7 Mo |
| Partie d’un mot en kanji : `京都` dans `東京都` | Trouvé | Non trouvé | Trouvé |
| Partie d’un mot en katakana : `ツリー` dans `スカイツリー` | Non trouvé | Trouvé | Trouvé |
| Partie d’un mot en katakana : `コンピュータ` dans `コンピューター` | Non trouvé | Non trouvé | Trouvé |
| Autre forme d’un mot : `走る` trouve `走った` | Non trouvé | Trouvé | Non trouvé |
| Partie d’un mot anglais : `run` dans `running` | Non trouvé | Non trouvé | Trouvé |

Kuromoji découpe le japonais en mots de son dictionnaire et ramène chacun à sa forme de base : il trouve donc un mot sous n’importe laquelle de ses formes, ainsi qu’un mot que le dictionnaire connaît à l’intérieur d’un composé. Il ne trouve pas une partie d’un mot que le dictionnaire garde entier, et la façon dont il découpe un mot nouveau ou un nom dépend du dictionnaire.
Les suites d’un et deux caractères trouvent n’importe quelle partie de n’importe quel mot, dans toutes les langues, y compris des parties qui ne sont pas des mots : `run` trouve aussi `brunch`. L’index est environ deux fois et demie plus gros, et une recherche en lit davantage.

Les recherches ont pris quelques millisecondes avec chacun d’eux sur ce petit index, et les écarts augmentent avec la taille de l’index.

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` conserve l’index dans un cluster OpenSearch que vous exploitez, ou dans un domaine d’Amazon OpenSearch Service. Le serveur parle à l’API REST du cluster. Il possède les champs de l’index et les requêtes qu’il envoie, et vous pouvez posséder le reste : les analyseurs, les shards, les réplicas et tout autre réglage d’index. Sans réglages de votre part, il utilise des analyseurs intégrés à OpenSearch et n’exige aucun plugin.

<!-- source: setting-it-up-1 11097d66bccc -->
### Mise en place

1. Lancez OpenSearch 2 ou 3. Un seul nœud suffit pour commencer ; le serveur n’atteint jamais que l’adresse que vous lui donnez.
2. Définissez [`search`](../reference/configuration.md#search) sur `opensearch` et [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) sur l’URL de l’API REST, comme `https://search.internal.example.org:9200`. Un chemin est conservé, pour un cluster derrière un proxy inverse.
3. Donnez au serveur un moyen d’accès :
   - Avec le plugin de sécurité, un utilisateur avec [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) et [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), envoyés avec l’authentification HTTP Basic. Utilisez `https` lorsque le cluster n’est pas sur le même hôte ou sur un réseau privé de confiance. L’utilisateur doit pouvoir créer, lire le mapping de et supprimer les index dont le nom commence par [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), gérer cet alias, et lire, écrire et supprimer leurs documents.
   - Sur Amazon OpenSearch Service avec une politique d’accès IAM, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) défini sur `es`, ce qui signe chaque requête avec AWS Signature Version 4. Les identifiants proviennent de la chaîne par défaut du SDK AWS (le rôle d’exécution, l’environnement, les fichiers partagés), et la région de [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) ou de `AWS_REGION`. Le rôle a besoin de `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut` et `es:ESHttpDelete` sur le domaine. Un domaine avec contrôle d’accès précis et un utilisateur maître interne prend à la place l’utilisateur et le mot de passe.
   - Amazon OpenSearch Serverless n’est pas pris en charge : il n’a pas de suppression par requête, dont a besoin la suppression des publications d’un auteur ou d’une visibilité.
4. Facultativement, [ajustez l’analyse](#tuning-the-analysis-of-opensearch) avec un fichier de réglages.
5. Démarrez le serveur. Il crée l’index au démarrage et refuse de démarrer lorsqu’il ne peut pas atteindre le cluster ou créer l’index.
6. Exécutez `my search rebuild` pour indexer les publications qui sont déjà là ([reconstruire l’index](search.md#rebuilding-the-index)).

L’URL, l’utilisateur, les identifiants AWS et le contenu du fichier de réglages n’apparaissent jamais dans les journaux ni les erreurs du serveur. Une erreur du moteur atteint le journal sous la forme de l’opération, du statut HTTP et du seul type d’erreur du moteur, puisque le message propre du moteur peut citer une publication ou une requête. La seule exception est un refus de création de l’index, dont le message nomme le réglage fautif.

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### L’index et ses mises à niveau

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), `my-statuses` par défaut, nomme un alias. Les documents vivent dans un index que le serveur crée et vers lequel il fait pointer l’alias, nommé d’après l’alias avec la version du mapping du serveur et un hachage des réglages de l’index, comme `my-statuses-v1-3f2a9c41b07e`. Le mapping consigne les deux dans son `_meta` (`my_mapping_version` et `my_settings_hash`). Chaque écriture passe par l’alias et échoue tant qu’il manque : OpenSearch ne crée donc jamais un index de lui-même avec un mapping deviné. Ne créez pas vous-même un index de ce nom : le serveur refuse de démarrer lorsque le nom est un index plutôt qu’un alias, ou lorsque l’alias pointe vers un index qu’il n’a pas créé.

Au démarrage, le serveur compare l’index derrière l’alias avec sa propre version de mapping et ses propres réglages. Lorsque l’un des deux diffère (une version dont le mapping a changé, ce que ses notes de version indiquent, ou un fichier de réglages modifié), il crée un nouvel index et y déplace l’alias. Le nouvel index démarre vide, donc :

1. Exécutez `my search rebuild`. Tant qu’il n’est pas terminé, la recherche fonctionne mais ne trouve que ce qui a été publié ou reconstruit depuis le changement.
2. Supprimez l’ancien index (`DELETE /my-statuses-v1-…`) lorsque la reconstruction est terminée. Le serveur le laisse en place et consigne un avertissement qui le nomme.

Démarrez chaque processus du serveur avec le même fichier de réglages : des processus avec des fichiers différents déplacent l’alias d’avant en arrière. Un serveur d’une version plus ancienne qui démarre sur un index d’un mapping plus récent utilise cet index tel quel.

`my search clear`, et `my search rebuild` sans `--keep`, qui commence par lui, créent un nouvel index vide avec le même mapping et les mêmes réglages, y déplacent l’alias et suppriment l’ancien en une seule requête.

<!-- source: sizing-1 4ad4bbcd9a2b -->
### Dimensionnement

- **Mémoire** : le nœud a besoin d’un heap JVM d’au moins 512 Mo (`-Xms512m -Xmx512m`) et d’autant de nouveau pour le système d’exploitation ; donnez-lui 1 Go pour un petit serveur, et davantage à mesure que l’index grossit. Le mieux est de garder le heap à la moitié de la mémoire du nœud.
- **Disque** : l’index occupe à peu près la taille du texte des publications indexées avec les réglages intégrés, davantage pour les langues écrites sans espaces, dont le texte est indexé en caractères isolés et en paires, et encore davantage avec des n-grammes. Gardez au moins 15 % du disque libre : par défaut, OpenSearch cesse de placer des shards sur un nœud au-delà de 85 % et bloque les écritures dans ses index au-delà de 95 %.
- **Shards et réplicas** : sans fichier de réglages, l’index a les valeurs par défaut du cluster, un shard et un réplica sauf si vous les avez changées. Un shard dessert des millions de publications. Un réplica laisse un cluster à nœud unique en jaune ; cela fonctionne, et `"index": {"number_of_replicas": 0}` dans un fichier de réglages le rend vert. Sur un cluster de plusieurs nœuds, gardez un réplica. Le nombre de réplicas peut aussi être changé sur l’index en service (`PUT /my-statuses/_settings`), ce qui n’exige aucune reconstruction ; le serveur continue d’utiliser l’index tant que le fichier de réglages ne change pas.
- **Charge** : chaque recherche est une requête de l’index, du plus récent au plus ancien par ID de publication. Les écritures sont une requête par publication, et une reconstruction n’écrit pas plus vite que [son débit](search.md#rebuilding-the-index).

<!-- source: japanese-and-other-languages a48cff99706b -->
### Le japonais et les autres langues

Avec les réglages intégrés, chaque publication, quelle que soit sa langue, est analysée de la même façon : le texte est découpé en mots aux espaces et à la ponctuation, mis en minuscules, et les lettres latines pleine largeur et les katakana demi-largeur sont ramenés à leurs formes usuelles. Le texte chinois, japonais et coréen, qui n’a pas d’espaces, est découpé en caractères, et chaque paire de caractères voisins est indexée en plus de chaque caractère (le filtre `cjk_bigram` intégré d’OpenSearch).

- Une requête trouve les publications qui contiennent chacun de ses mots. En japonais, un mot de deux caractères ou plus trouve les publications qui contiennent chaque paire de ses caractères : `東京` trouve `東京都の天気`, et `全文検索` trouve `全文検索の本文です`. Un caractère seul, comme `本`, trouve toutes les publications qui contiennent ce caractère.
- Les paires ne savent pas où les mots se terminent : une requête peut donc trouver une publication dont les caractères se trouvent simplement côte à côte : `京都` trouve aussi `東京都`. Une requête en katakana écrits en demi-largeur ou en pleine largeur trouve les deux.
- Les lectures ne sont pas mises en correspondance : `ぜんぶん` ne trouve pas `全文`, et les formes fléchies ne sont pas rapprochées de leurs formes de dictionnaire, en japonais comme en anglais (`running` ne trouve pas `run`).

Pour trouver le japonais par mots, utilisez un analyseur morphologique tel que kuromoji dans un [fichier de réglages](#tuning-the-analysis-of-opensearch).

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### Ajuster l’analyse d’OpenSearch

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) nomme un fichier JSON dont l’objet devient les `settings` de chaque index que le serveur crée : `analysis`, `index.number_of_shards`, `index.number_of_replicas` et tout autre réglage d’index. Il ne contient que des réglages ; le serveur possède le mapping, et refuse un fichier avec `mappings`.

Le fichier doit définir deux analyseurs, que le mapping nomme :

| Analyseur | Analyse |
| --- | --- |
| `my_text` | Le texte de chaque publication : son corps, son avertissement de contenu et les textes alternatifs de ses médias |
| `my_text_search` | Le texte de chaque requête |

Ils peuvent avoir la même définition. Chaque requête est une requête `match` du champ `text` avec `"operator": "and"` : le texte de la requête passe par `my_text_search`, et une publication est trouvée lorsque son texte contient chaque terme qui en sort, du plus récent au plus ancien par ID de publication, sans score. Une requête dont l’analyseur ne laisse aucun terme ne trouve rien. La correspondance d’expression, floue et par préfixe n’est pas utilisée : un analyseur décide donc seul de ce qu’est une correspondance. La langue d’une publication est stockée mais ne choisit pas d’analyseur : chaque publication passe par `my_text`.

Lorsque le serveur démarre avec un fichier dont les réglages diffèrent de ceux de l’index, ne serait-ce que d’un réplica, il crée un nouvel index vide, et l’index a besoin d’une reconstruction ([l’index et ses mises à niveau](#the-index-and-its-upgrades)). La mise en forme et l’ordre des clés ne comptent pas. Un fichier qu’OpenSearch refuse, comme un fichier sans les deux analyseurs ou avec un plugin que le cluster n’a pas, interrompt le démarrage avec la raison d’OpenSearch.

Kuromoji découpe le japonais en mots avec un dictionnaire et rapproche les formes fléchies de leurs formes de dictionnaire. Amazon OpenSearch Service a le plugin sur chaque domaine ; un cluster que vous exploitez doit l’avoir installé (`opensearch-plugin install analysis-kuromoji`, par exemple dans un Dockerfile `FROM opensearchproject/opensearch`). Un fichier avec kuromoji :

```json
{
  "index": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      }
    }
  }
}
```

Un mot que le dictionnaire découpe autrement que la requête n’est pas trouvé : kuromoji peut donc manquer ce que trouvent les paires intégrées, comme une partie d’un nom composé ou un mot nouveau. Un tokenizer n-gramme, intégré à OpenSearch, trouve n’importe quelle suite de caractères au prix d’un index plus gros et de correspondances à l’intérieur de mots plus longs ; avec `"min_gram": 2` et `"max_gram": 3` (et `"index": {"max_ngram_diff": 1}`), une requête d’un seul caractère ne trouve rien.

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` conserve l’index dans un serveur [Meilisearch](https://www.meilisearch.com/docs), autohébergé ou Meilisearch Cloud. C’est un processus unique, sans cluster à exploiter, ce qui convient à un petit serveur, et l’image officielle segmente le japonais, le chinois et le coréen sans plugin. Le serveur a été vérifié avec Meilisearch 1.56.

<!-- source: running-meilisearch 4f7d5e17c255 -->
### Exploiter Meilisearch

Lancez l’image officielle avec une clé maîtresse d’au moins 16 octets, sur un réseau que seul `my` peut atteindre, et gardez son répertoire de données sur un volume persistant :

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` refuse de démarrer sans clé maîtresse et désactive l’aperçu de recherche intégré. Meilisearch parle en HTTP simple : placez-le derrière TLS, ou gardez-le sur un réseau privé, lorsque `my` l’atteint par autre chose qu’une liaison locale ou privée.

<!-- source: settings 74547d71e962 -->
### Paramètres

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

Définissez la clé par l’environnement (`MY_SEARCH_MEILISEARCH_API_KEY`, ou `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), jamais dans le fichier de configuration. Ce peut être la clé maîtresse, mais une clé qui ne peut faire que ce dont `my` a besoin est plus sûre. Créez-en une avec la clé maîtresse :

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Utilisez la `key` de la réponse comme `search_meilisearch_api_key`. La clé n’est envoyée que dans l’en-tête `Authorization` : les journaux et les erreurs du serveur montrent le code d’erreur de Meilisearch (comme `invalid_api_key`) et jamais la clé, une publication ou une requête. Tous [les paramètres](../reference/configuration.md#search_meilisearch_url) figurent dans la référence de configuration.

Plusieurs serveurs peuvent partager un même serveur Meilisearch avec chacun un `search_meilisearch_index` différent, et une clé limitée à son propre index.

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### L’index et ses réglages

Le serveur crée l’index lorsqu’il n’existe pas. La première fois qu’il utilise l’index après un démarrage, il compare les réglages qu’il enverrait avec ceux qu’il a appliqués en dernier, qu’il consigne sous forme de hachage dans un document de l’index qu’aucune recherche ne trouve, et ne les envoie que s’ils diffèrent : un redémarrage sans changement n’envoie rien. Les réglages sont :

- **Ce que le serveur définit lui-même :** `searchableAttributes` (les champs texte des publications), `displayedAttributes` (tous), `filterableAttributes` (l’ID de la publication, l’auteur et la visibilité) et `sortableAttributes` (l’ID de la publication). La recherche en dépend, et un fichier de réglages qui en définit un est refusé.
- **Ce que vous pouvez ajuster :** tout le reste, dans le [fichier de réglages](#tuning-meilisearch). Sans fichier, ou pour ce que le fichier omet, le serveur utilise ses valeurs par défaut : des `rankingRules` de `sort`, `words`, `typo`, `proximity`, `attribute` et `exactness` ; aucune tolérance aux fautes de frappe ; les langues des champs japonais, chinois et coréen dans `localizedAttributes` ; et les valeurs par défaut propres à Meilisearch pour le reste (`stopWords`, `synonyms`, `dictionary`, `separatorTokens`, `nonSeparatorTokens`, `proximityPrecision`, `pagination`, `faceting`, `distinctAttribute`, `searchCutoffMs`, `prefixSearch`). Les réglages que vous modifiez à la main sur le moteur ne subsistent que jusqu’au prochain envoi de ses réglages par le serveur.

**Un changement des réglages n’exige pas `my search rebuild`.** Meilisearch réindexe lui-même les documents qu’il contient, en arrière-plan, et les publications restent dans l’index. Cela prend seulement du temps, à peu près autant que leur indexation. Le démarrage du serveur ne l’attend pas : le serveur envoie les réglages et continue. Pendant ce temps, les recherches répondent avec les réglages précédents, et les écritures de l’index attendent derrière la réindexation, puisque Meilisearch exécute les tâches d’un index dans l’ordre ; une tâche dont l’écriture ne se termine pas dans ses 10 secondes réessaie plus tard, de sorte que les publications écrites pendant la réindexation atteignent l’index après elle. `my search clear` attend les réglages du nouvel index vide, qui ne prennent aucun temps.

Une reconstruction n’est nécessaire que lorsque l’index démarre vide :

- Vous changez `search_meilisearch_index`.
- Vous démarrez un nouveau serveur Meilisearch vide, par exemple au lieu de mettre à niveau l’ancien.
- Les notes de version d’une version de `my` indiquent qu’elle écrit les documents différemment.

Les écritures sont des tâches que Meilisearch exécute dans l’ordre. Le serveur attend la tâche de chaque écriture, et une tâche qui échoue ou ne se termine pas dans le temps de la tâche fait réessayer la tâche : un Meilisearch arrêté ou occupé retarde donc l’index sans perdre de modification. Une tâche de réglages qui échoue, par exemple à cause d’une valeur du fichier de réglages que Meilisearch refuse, fait échouer les écritures jusqu’à ce qu’elle réussisse ; `GET /tasks?types=settingsUpdate` sur le moteur montre son erreur. `my search clear` supprime l’index et le recrée avec les réglages.

Pour mettre à niveau Meilisearch lui-même, suivez son guide de mise à niveau (un dump, ou sa mise à niveau sans dump), ou démarrez la nouvelle version vide et exécutez `my search rebuild`.

<!-- source: what-a-search-sends 575b89133606 -->
### Ce qu’une recherche envoie

La requête d’une recherche est fixe, quoi que dise le fichier de réglages : `matchingStrategy: all`, pour qu’une publication ait chaque mot ; `sort: status_id:desc`, du plus récent au plus ancien ; un `filter` sur l’ID de la publication pour la page suivante ; une `limit` de la page plus un ; `attributesToRetrieve` de l’ID de la publication ; et `showMatchesPosition`, qui signale une requête sans mot. Les guillemets doubles d’une expression et le signe moins devant un mot, que Meilisearch lirait comme des opérateurs, sont retirés : une requête est donc toujours composée de mots simples.

<!-- source: tuning-meilisearch df827448e06c -->
### Ajuster Meilisearch

Ajustez le moteur avec un fichier JSON de [réglages d’index Meilisearch](https://www.meilisearch.com/docs/reference/api/settings), nommé par `search_meilisearch_settings_file`. Le serveur le vérifie au démarrage, lorsque `search = meilisearch` : il doit exister et être un objet JSON, ne doit pas définir les quatre réglages que le serveur définit lui-même, et ses `rankingRules`, s’il en a, doivent commencer par `sort`, ce qui garde les résultats du plus récent au plus ancien. Meilisearch vérifie les valeurs lorsque le serveur les envoie. Le serveur ne consigne jamais le contenu du fichier.

Un exemple qui restreint les langues au japonais, traite `・` comme un séparateur, rend la proximité moins coûteuse à indexer et active la tolérance aux fautes de frappe pour les mots longs :

```json
{
  "localizedAttributes": [
    {"attributePatterns": ["text_ja"], "locales": ["jpn"]}
  ],
  "separatorTokens": ["・"],
  "proximityPrecision": "byAttribute",
  "typoTolerance": {"enabled": true, "minWordSizeForTypos": {"oneTypo": 8, "twoTypos": 12}},
  "synonyms": {"ご飯": ["ごはん"], "ごはん": ["ご飯"]}
}
```

Ce que coûte un changement :

- Chaque modification du fichier, au démarrage suivant, fait réindexer à Meilisearch chaque document, comme ci-dessus : aucune donnée n’est perdue et rien n’exige de reconstruction, mais les écritures attendent la fin. Modifiez-le à un moment calme, et suivez la tâche avec `GET /tasks?types=settingsUpdate`.
- `localizedAttributes` et les jetons décident de la façon dont le texte est découpé en mots : une liste de langues plus étroite peut segmenter une publication d’une autre langue comme si elle était de la mauvaise (voir plus bas) : gardez donc les champs japonais, chinois et coréen, sauf si le serveur n’a aucune publication dans ces langues.
- La tolérance aux fautes de frappe et les synonymes font trouver à une recherche plus que les mots exacts. Les règles de classement après `sort` ne changent rien, puisque deux publications ne partagent jamais un ID ; les omettre ne fait qu’épargner un peu de travail à Meilisearch.

<!-- source: sizing-2 1d581fac5316 -->
### Dimensionnement

Meilisearch conserve l’index dans des fichiers mappés en mémoire sur le disque : il fonctionne avec moins de mémoire que l’index, et devient plus rapide à mesure qu’une plus grande partie de l’index tient en mémoire.

- **Mémoire.** 1 Go est un bon départ pour un petit serveur ; donnez-en davantage à mesure que l’index grossit, idéalement autant que la taille de l’index sur le disque. L’indexation utilise par défaut jusqu’aux deux tiers de la mémoire de la machine ; sur une machine partagée avec d’autres services, plafonnez-la avec `MEILI_MAX_INDEXING_MEMORY` (comme `512Mb`).
- **Disque.** L’index est plusieurs fois plus gros que le texte qu’il contient, parce qu’il conserve les listes de mots de chaque champ. Mesurez-le après la première reconstruction avec `GET /stats` (`databaseSize`), et gardez un espace libre bien supérieur : la réindexation et les mises à niveau ont besoin de place pour travailler.
- **CPU.** Un ou deux cœurs suffisent pour un petit serveur. Une reconstruction est la charge la plus lourde ; ralentissez-la avec `my search rebuild --rate`.

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### Le japonais et les autres langues

Le serveur place le texte d’une publication dans le champ de sa langue : japonais (`ja`, y compris les balises comme `ja-JP`), chinois (`zh`), coréen (`ko`), ou un champ pour toutes les autres langues et pour l’absence de langue. Il indique à Meilisearch la langue des trois premières, et Meilisearch segmente chacune en mots avec un dictionnaire :

- **Le japonais** est segmenté par Lindera avec le dictionnaire UniDic, que le texte contienne des kana ou uniquement des kanji. `検索`, `全文検索` et `全文　検索` trouvent tous `全文検索の本文です`, et `東京` trouve `東京都庁舎見学`.
- **Le chinois** est segmenté par jieba, et **le coréen** par Lindera avec le dictionnaire ko-dic.
- **Les autres langues** sont découpées aux espaces et à la ponctuation ; Meilisearch détecte lui-même l’écriture et la langue du texte.

Une requête est découpée en mots de la même façon, et une publication est trouvée lorsqu’elle a chaque mot de la requête, le dernier aussi comme début d’un mot plus long (`テキ` trouve `テキスト`). Seuls les 10 premiers mots d’une requête comptent.

Ce que la recherche ne peut pas trouver :

- **Une chaîne qui n’est pas un mot de la segmentation du dictionnaire.** La recherche trouve des mots, non des sous-chaînes : `京都` ne trouve pas `東京都`, et une partie que le dictionnaire garde à l’intérieur d’un mot plus long, comme `庁舎` dans `東京都庁舎見学`, n’est pas trouvée non plus.
- **Les lectures.** Les hiragana trouvent le même mot en katakana (`かたかな` trouve `カタカナ`), mais une lecture ne trouve pas les kanji (`ぜんぶん` ne trouve pas `全文`).
- **Les orthographes proches.** La tolérance aux fautes de frappe est désactivée : chaque mot doit donc être exact.
- **Les publications japonaises sans langue.** Une publication dont la langue est absente ou erronée est segmentée d’après ce que Meilisearch détecte, et un texte en kanji uniquement est alors souvent pris pour du chinois et découpé différemment : une telle publication peut être trouvée par `京都` pour `東京都`, ou ne pas l’être par `東京`. Une publication porte la langue que son auteur a choisie, ou ce que le serveur de l’auteur a envoyé.

Le serveur normalise les formes de compatibilité d’Unicode dans les publications et les requêtes avant qu’elles n’atteignent Meilisearch : les katakana demi-largeur (`ｶﾀｶﾅ`) et les lettres et chiffres pleine largeur (`ＡＢＣ`) trouvent donc leurs formes usuelles et sont trouvés par elles. Les émoji sont aussi des mots : `👍` trouve les publications qui en contiennent.

La recherche n’a besoin d’aucun plugin, dictionnaire ni image à elle pour rien de tout cela : l’image officielle contient les dictionnaires du japonais, du chinois et du coréen.
