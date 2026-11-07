# Search engines

How to run each search engine that the full-text search of posts can use, how large it needs to be, and how far it finds Japanese and other languages written without spaces. [Full-text search of posts](search.md) covers what the index holds, turning the search on, rebuilding the index, and switching engines, whatever the engine.

## Elasticsearch

`search = elasticsearch` keeps the index in an Elasticsearch cluster of version 8 or later, self-managed or on Elastic Cloud Hosted. It has been checked against Elasticsearch 9.5 and 8.19. The server owns the fields of the index, which it writes, filters, sorts, and pages on. You may own its settings: the analysis, the shards, and the replicas. Without settings of your own, the built-in ones need no plugin and find Japanese as [described below](#how-the-built-in-analysis-finds-japanese).

### Setting it up

1. Run Elasticsearch. A single node is enough for most servers. For a quick start on the same host:

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   With security on, which is the default, Elasticsearch serves HTTPS with a certificate of its own authority. Copy that authority out of the container for `search_elasticsearch_ca_file`:

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. Create an API key that can manage only the server's indexes. `manage`, `read`, and `write` on the alias and the indexes behind it are all it needs:

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   The response's `encoded` value is the key.

3. Set the server's configuration and restart it:

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. Run `my search rebuild` to add the existing posts, as in [turning it on](search.md#turning-it-on).

The settings:

| Setting | What it does |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | The URL of the REST API. Use `https:` whenever the requests cross a network that others share; `http:` is accepted for a private network or a cluster with security off. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | The API key, which is the better choice. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | A user and password for basic authentication instead of a key. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | The name of the alias, `my-statuses` by default. Give each server that shares a cluster its own name, and grant its key that name followed by `*`. |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | A JSON file of your own settings of the index, as in [tuning the analysis](#tuning-the-analysis-of-elasticsearch). |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | The certificate authority of an HTTPS cluster whose certificate the system does not trust. |

The credentials are read only from the environment, as secrets, and never appear in the server's logs or errors. An error of the cluster reaches the logs as the request's path, its status, and the type of the error, such as `index_not_found_exception`, never with the text of a post or a query. Only when the cluster refuses to create the index does the error quote its reason, which names nothing but the settings and the fields.

### The index and its upgrades

The server creates and maintains the index itself. Do not create it by hand, and do not let anything else write to it.

- At start, the server looks for the alias. When there is none, it creates an index named after the alias, the version of its fields, and a hash of its settings, such as `my-statuses-v1-1f0c2e9a7b3d4c5e`, and points the alias at it. The version and the hash are kept in the index's `_meta`.
- The server cannot start while the cluster is unreachable, refuses its key, or refuses the settings. `my config check` checks the configuration and that the settings file is one JSON object, not what Elasticsearch thinks of it, so check the connection and the settings by starting the server.
- An index of the name of the alias that the server did not create, such as one that Elasticsearch made by itself, stops the start with an error that names it. Delete that index, or choose another `search_elasticsearch_index`.
- When the settings differ from those the index was created with, because you edited the settings file or a new version of `my` changed the built-in ones, the server creates a new empty index with the new settings, points the alias at it, deletes the old one, and logs a warning that asks for `my search rebuild`. Run it. Until the rebuild ends, search finds only the posts written since. When Elasticsearch refuses the new settings, the server does not start and the old index stays.
- When a new version of `my` changes the fields, the server upgrades the index at start. A change that only adds fields is made in place, and search goes on working; the release notes say when the posts already in the index need `my search rebuild --keep` to gain the new fields. Any other change of the fields takes a new empty index and a rebuild, as above. The release notes say when a version does this.
- An index made by a newer version of `my` stops an older one from starting. To roll back across such a change, delete the alias's index in Elasticsearch (find it with `GET _alias/my-statuses`), start the older version, which creates its own, and run `my search rebuild`.
- `my search clear` points the alias at a new empty index and deletes the old one in one step, which is quicker than deleting each document and gives the disk space back.

### Sizing

The index keeps only what a search needs and no copy of the text (`_source` is off). With the built-in settings, short posts take about 80 to 150 bytes of disk each once Elasticsearch has merged its segments, and longer posts with a wider vocabulary more, so plan on 150 to 300 bytes a post: a few hundred megabytes for a million posts. Check the real size with `GET _cat/indices/my-statuses*?v`.

The built-in settings give the index one primary shard, which holds tens of millions of posts, and one replica once the cluster has a second node (`auto_expand_replicas: 0-1`), so a single node stays green. Set `number_of_shards` and `number_of_replicas` in a settings file to change them.

A node with 1 GB of Java heap (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) and about 2 GB of memory in all serves a small or medium server. Give Elasticsearch half of the machine's memory as heap and leave the rest to the file cache, and give it more when searches slow down. Replicas need more nodes and the same disk again for each replica.

### How a query uses the analysis

Every post is indexed into one text field, whatever its language, and the same analysis applies to all of them. The language of a post does not choose another analyzer.

- The text of a post is analyzed with the analyzer named `my_text`.
- A query is split into words at spaces, including the ideographic space. Each word is analyzed with `my_text_search` when the settings define it, and with `my_text` otherwise, and must appear in the post as a phrase: its tokens in the same order and next to each other. Every word must match. Nothing in a query is read as the query language of Elasticsearch.
- Results are ordered newest first, not by relevance, so scoring options make no difference.

So an analyzer decides what a word is. An analyzer that makes a token of each character finds any run of characters. One that makes dictionary words finds whole words and their other forms.

### How the built-in analysis finds Japanese

The built-in `my_text` is the standard tokenizer of Elasticsearch with the `cjk_width` and `lowercase` filters, all built in. The tokenizer makes a token of each kanji and each hiragana, of each run of katakana, and of each word of the languages that use spaces.

- A word of kanji or hiragana is found wherever its characters stand together, even inside a longer word: `検索` and `本` find `全文検索の本文です`, and `東京 天気` finds `東京都の天気は晴れ`. Characters that only appear apart do not match.
- A run of katakana is matched whole: `カタカナ` finds `カタカナとABC`, but `カタ` does not find it, and `ツリー` does not find `スカイツリー`.
- Full-width Latin letters and half-width katakana match their usual forms: `abc` finds `ＡＢＣ`, and `カタカナ` finds `ｶﾀｶﾅ`.
- Hiragana does not find the kanji of the same reading: `ぜんぶん` does not find `全文`. Other forms of a word are not found: `走る` does not find `走った`.
- A query of one very common character, such as `の`, finds a great many posts.

Chinese and Korean are found the same way, by their characters. Words in the languages written with spaces are found whole, without stemming: `run` does not find `running`.

### Tuning the analysis of Elasticsearch

Put the settings of the index in a JSON file and name it in `search_elasticsearch_index_settings_file`. The server uses the whole file as the `settings` of each index it creates, in place of the built-in ones, so it must hold everything the index needs:

- The analyzer `my_text`, which the text field uses. Without it, Elasticsearch refuses to create the index and the server does not start.
- Optionally the analyzer `my_text_search`, which then analyzes the words of the queries.
- Any other index setting, such as `number_of_shards` and `number_of_replicas`. What the file leaves out takes the cluster's defaults.
- Only analysis that Elasticsearch has: a plugin that the file names must be installed on every node.

The server reads the file at start. Editing it gives the next start a new empty index, as in [the index and its upgrades](#the-index-and-its-upgrades), so run `my search rebuild` after each change. Try a change on another cluster or another `search_elasticsearch_index` first.

Dictionary words with the `analysis-kuromoji` plugin, which you install on every node (`bin/elasticsearch-plugin install analysis-kuromoji`) or which Elastic Cloud Hosted offers as an extension:

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

Every run of one or two characters, with only the analysis built into Elasticsearch:

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

How they compare, measured on 20,000 posts, half English and half Japanese, on Elasticsearch 9.5:

| | Built-in | Kuromoji | One and two characters |
| --- | --- | --- | --- |
| Plugin | None | `analysis-kuromoji` | None |
| Size of the index | 1.5 MB | 1.4 MB | 3.7 MB |
| Part of a kanji word: `京都` in `東京都` | Found | Not found | Found |
| Part of a katakana word: `ツリー` in `スカイツリー` | Not found | Found | Found |
| Part of a katakana word: `コンピュータ` in `コンピューター` | Not found | Not found | Found |
| Another form of a word: `走る` finds `走った` | Not found | Found | Not found |
| Part of an English word: `run` in `running` | Not found | Not found | Found |

Kuromoji splits Japanese into the words of its dictionary and reduces each to its base form, so it finds a word in any of its forms, and a word that the dictionary knows inside a compound. It does not find a part of a word that the dictionary keeps whole, and how it splits a new word or a name depends on the dictionary.

Runs of one and two characters find any part of any word, in every language, including parts that are not words: `run` also finds `brunch`. The index is about two and a half times as large, and a search reads more of it.

Searches took a few milliseconds with each of them on this small index, and the differences grow with the size of the index.

## OpenSearch

`search = opensearch` keeps the index in an OpenSearch cluster that you run, or in a domain of Amazon OpenSearch Service. The server talks to the cluster's REST API. It owns the fields of the index and the queries it sends, and you may own the rest: the analyzers, the shards, the replicas, and any other index setting. Without settings of your own, it uses analyzers built into OpenSearch and needs no plugin.

### Setting it up

1. Run OpenSearch 2 or 3. One node is enough to start; the server only ever reaches the address you give it.
2. Set [`search`](../reference/configuration.md#search) to `opensearch` and [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) to the URL of the REST API, such as `https://search.internal.example.org:9200`. A path is kept, for a cluster behind a reverse proxy.
3. Give the server a way in:
   - With the security plugin, a user with [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) and [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), sent with HTTP Basic authentication. Use `https` when the cluster is not on the same host or a private network you trust. The user needs to create, read the mapping of, and delete the indexes whose names start with [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), to manage that alias, and to read, write, and delete their documents.
   - On Amazon OpenSearch Service with an IAM access policy, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) set to `es`, which signs every request with AWS Signature Version 4. The credentials come from the AWS SDK's default chain (the execution role, the environment, the shared files), and the region from [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) or `AWS_REGION`. The role needs `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut`, and `es:ESHttpDelete` on the domain. A domain with fine-grained access control and an internal master user takes the user and password instead.
   - Amazon OpenSearch Serverless is not supported: it has no delete by query, which removing the posts of an author or of a visibility needs.
4. Optionally, [tune the analysis](#tuning-the-analysis-of-opensearch) with a settings file.
5. Start the server. It creates the index at start and refuses to start when it cannot reach the cluster or create the index.
6. Run `my search rebuild` to index the posts that are already there ([rebuilding the index](search.md#rebuilding-the-index)).

The URL, the user, the AWS credentials, and the contents of the settings file never appear in the server's logs or errors. An error of the engine reaches the log as the operation, the HTTP status, and the type of the engine's error only, since the engine's own message can quote a post or a query. The one exception is a refused creation of the index, whose message names the setting at fault.

### The index and its upgrades

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), `my-statuses` by default, names an alias. The documents live in an index that the server creates and points the alias at, named after the alias with the version of the server's mapping and a hash of the index settings, such as `my-statuses-v1-3f2a9c41b07e`. The mapping records the same two in its `_meta` (`my_mapping_version` and `my_settings_hash`). Every write goes through the alias and fails while it is missing, so OpenSearch never creates an index of its own with a guessed mapping. Do not create an index of that name yourself: the server refuses to start when the name is an index rather than an alias, or when the alias points at an index it did not create.

At start, the server compares the index behind the alias with its own mapping version and settings. When either differs (a release whose mapping changed, which its release notes say, or an edited settings file), it creates a new index and moves the alias to it. The new index starts empty, so:

1. Run `my search rebuild`. Until it ends, search works but finds only what was posted or rebuilt since the change.
2. Delete the old index (`DELETE /my-statuses-v1-…`) when the rebuild has ended. The server leaves it in place and logs a warning that names it.

Start every process of the server with the same settings file: processes with different files move the alias back and forth. A server of an older release that starts against an index of a newer mapping uses that index as it is.

`my search clear`, and `my search rebuild` without `--keep`, which begins with it, create a new empty index with the same mapping and settings, move the alias to it, and delete the old one in one request.

### Sizing

- **Memory**: the node needs a JVM heap of at least 512 MB (`-Xms512m -Xmx512m`) and as much again for the operating system; give it 1 GB for a small server, and more as the index grows. The heap is best kept at half of the node's memory.
- **Disk**: the index takes roughly the size of the text of the indexed posts with the built-in settings, more for languages written without spaces, whose text is indexed as single characters and pairs, and more again with n-grams. Keep at least 15% of the disk free: by default OpenSearch stops placing shards on a node above 85% and blocks writes to its indexes above 95%.
- **Shards and replicas**: without a settings file the index has the cluster's defaults, one shard and one replica unless you changed them. One shard serves millions of posts. One replica leaves a single-node cluster yellow; it works, and `"index": {"number_of_replicas": 0}` in a settings file makes it green. On a cluster of several nodes, keep a replica. The number of replicas can also be changed on the live index (`PUT /my-statuses/_settings`), which needs no rebuild; the server goes on using the index as long as the settings file does not change.
- **Load**: every search is one query of the index, newest first by post ID. Writes are one request per post, and a rebuild writes no faster than [its rate](search.md#rebuilding-the-index).

### Japanese and other languages

With the built-in settings, every post, whatever its language, is analyzed the same way: the text is split into words at spaces and punctuation, lowercased, and full-width Latin letters and half-width katakana are folded to their usual forms. Chinese, Japanese, and Korean text, which has no spaces, is split into characters, and each pair of neighbouring characters is indexed as well as each character (the built-in `cjk_bigram` filter of OpenSearch).

- A query finds the posts that contain every word of it. In Japanese, a word of two characters or more finds the posts that contain each pair of its characters: `東京` finds `東京都の天気`, and `全文検索` finds `全文検索の本文です`. A single character, such as `本`, finds every post with that character.
- The pairs do not know where words end, so a query can find a post where its characters only happen to stand side by side: `京都` finds `東京都` as well. A query of katakana written in half-width or full-width letters finds both.
- Readings are not matched: `ぜんぶん` does not find `全文`, and inflected forms are not matched to their dictionary forms, in Japanese or in English (`running` does not find `run`).

To find Japanese by words, use a morphological analyzer such as kuromoji in a [settings file](#tuning-the-analysis-of-opensearch).

### Tuning the analysis of OpenSearch

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) names a JSON file whose object becomes the `settings` of every index the server creates: `analysis`, `index.number_of_shards`, `index.number_of_replicas`, and any other index setting. It holds settings only; the server owns the mapping, and refuses a file with `mappings`.

The file must define two analyzers, which the mapping names:

| Analyzer | Analyzes |
| --- | --- |
| `my_text` | The text of each post: its body, content warning, and the alternative texts of its media |
| `my_text_search` | The text of each query |

They may have the same definition. Each query is a `match` query of the field `text` with `"operator": "and"`: the query text goes through `my_text_search`, and a post is found when its text holds every term that comes out, newest first by post ID, with no scoring. A query whose analyzer leaves no term finds nothing. Phrase, fuzzy, and prefix matching are not used, so an analyzer decides on its own what a match is. The language of a post is stored but does not pick an analyzer: every post goes through `my_text`.

When the server starts with a file whose settings differ from those of the index, even by one replica, it creates a new empty index, and the index needs a rebuild ([the index and its upgrades](#the-index-and-its-upgrades)). Formatting and the order of the keys do not count. A file that OpenSearch refuses, such as one without the two analyzers or with a plugin the cluster does not have, stops the start with OpenSearch's reason.

Kuromoji splits Japanese into words with a dictionary and matches inflected forms by their dictionary forms. Amazon OpenSearch Service has the plugin on every domain; a cluster you run needs it installed (`opensearch-plugin install analysis-kuromoji`, for example in a Dockerfile `FROM opensearchproject/opensearch`). A file with kuromoji:

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

A word that the dictionary splits differently from the query is not found, so kuromoji can miss what the built-in pairs find, such as a part of a compound noun or a new word. An n-gram tokenizer, built into OpenSearch, finds any run of characters at the cost of a larger index and of matches inside longer words; with `"min_gram": 2` and `"max_gram": 3` (and `"index": {"max_ngram_diff": 1}`), a query of a single character finds nothing.

## Meilisearch

`search = meilisearch` keeps the index in a [Meilisearch](https://www.meilisearch.com/docs) server, self-hosted or Meilisearch Cloud. It is a single process with no cluster to run, which suits a small server, and the official image segments Japanese, Chinese, and Korean without a plugin. The server has been checked against Meilisearch 1.56.

### Running Meilisearch

Run the official image with a master key of at least 16 bytes, on a network that only `my` can reach, and keep its data directory on a persistent volume:

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` refuses to start without a master key and turns off the built-in search preview. Meilisearch speaks plain HTTP: put it behind TLS, or keep it on a private network, when `my` reaches it over anything but a local or private link.

### Settings

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

Set the key through the environment (`MY_SEARCH_MEILISEARCH_API_KEY`, or `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), never in the config file. It may be the master key, but a key that can do only what `my` needs is safer. Create one with the master key:

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Use the `key` of the response as `search_meilisearch_api_key`. The key is sent only in the `Authorization` header: the server's logs and errors show the Meilisearch error code (such as `invalid_api_key`) and never the key, a post, or a query. All [the settings](../reference/configuration.md#search_meilisearch_url) are in the configuration reference.

Several servers can share one Meilisearch server with a different `search_meilisearch_index` each, and a key limited to its own index.

### The index and its settings

The server creates the index when it does not exist. The first time it uses the index after a start, it compares the settings it would send with those it applied last, which it records as a hash in a document of the index that no search finds, and sends them only when they differ: a restart with nothing changed sends nothing. The settings are:

- **What the server sets itself:** `searchableAttributes` (the text fields of the posts), `displayedAttributes` (all), `filterableAttributes` (the post ID, the author, and the visibility), and `sortableAttributes` (the post ID). The search depends on them, and a settings file that sets one is refused.
- **What you can tune:** everything else, in the [settings file](#tuning-meilisearch). Without one, or for what the file leaves out, the server uses its defaults: `rankingRules` of `sort`, `words`, `typo`, `proximity`, `attribute`, and `exactness`; no typo tolerance; the languages of the Japanese, Chinese, and Korean fields in `localizedAttributes`; and Meilisearch's own defaults for the rest (`stopWords`, `synonyms`, `dictionary`, `separatorTokens`, `nonSeparatorTokens`, `proximityPrecision`, `pagination`, `faceting`, `distinctAttribute`, `searchCutoffMs`, `prefixSearch`). Settings you change by hand on the engine stay only until the server next sends its settings.

**A change of the settings needs no `my search rebuild`.** Meilisearch reindexes the documents it holds by itself, in the background, and the posts stay in the index. It only takes time, roughly as long as indexing them did. The start of the server does not wait for it: the server sends the settings and goes on. Meanwhile searches answer with the previous settings, and the writes of the index wait behind the reindex, since Meilisearch runs the tasks of an index in order; a job whose write does not finish within its 10 seconds retries later, so posts written during the reindex reach the index after it. `my search clear` waits for the settings of the new, empty index, which take no time.

A rebuild is needed only when the index starts empty:

- You change `search_meilisearch_index`.
- You start a new, empty Meilisearch server, for example rather than upgrading the old one.
- The release notes of a version of `my` say that it writes the documents differently.

Writes are tasks that Meilisearch runs in order. The server waits for the task of each write, and a task that fails or does not end within the time of the job makes the job retry, so a Meilisearch that is down or busy delays the index without losing a change. A settings task that fails, for example over a value of the settings file that Meilisearch refuses, fails the writes until it succeeds; `GET /tasks?types=settingsUpdate` on the engine shows its error. `my search clear` deletes the index and creates it again with the settings.

To upgrade Meilisearch itself, follow its upgrade guide (a dump, or its dumpless upgrade), or start the new version empty and run `my search rebuild`.

### What a search sends

The query of a search is fixed, whatever the settings file says: `matchingStrategy: all`, so that a post must have every word; `sort: status_id:desc`, newest first; a `filter` on the post ID for the next page; a `limit` of the page plus one; `attributesToRetrieve` of the post ID; and `showMatchesPosition`, which tells a query that has no word. The double quotes of a phrase and the minus sign before a word, which Meilisearch would read as operators, are dropped, so a query is always plain words.

### Tuning Meilisearch

Tune the engine with a JSON file of [Meilisearch index settings](https://www.meilisearch.com/docs/reference/api/settings), named by `search_meilisearch_settings_file`. The server checks it at start, when `search = meilisearch`: it must exist and be a JSON object, must not set the four settings the server sets itself, and its `rankingRules`, when it has them, must start with `sort`, which keeps the results newest first. Meilisearch checks the values when the server sends them. The server never logs the file's content.

An example that narrows the languages to Japanese, treats `・` as a separator, makes proximity cheaper to index, and turns typo tolerance on for long words:

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

What a change costs:

- Every change of the file, on the next start, makes Meilisearch reindex every document, as above: no data is lost and nothing needs a rebuild, but writes wait until it ends. Change it at a quiet time, and watch the task with `GET /tasks?types=settingsUpdate`.
- `localizedAttributes` and the tokens decide how text is split into words: a narrower list of locales can segment a post in another language as the wrong one (see below), so keep the Japanese, Chinese, and Korean fields unless the server has none of those posts.
- Typo tolerance and synonyms make a search find more than the exact words. The ranking rules after `sort` change nothing, since no two posts share an ID; leaving them out only saves Meilisearch a little work.

### Sizing

Meilisearch keeps the index in memory-mapped files on disk: it runs with less memory than the index, and gets faster the more of the index fits in memory.

- **Memory.** 1 GB is a reasonable start for a small server; give it more as the index grows, ideally as much as the index's size on disk. Indexing uses up to two-thirds of the machine's memory by default; on a machine shared with other services, cap it with `MEILI_MAX_INDEXING_MEMORY` (such as `512Mb`).
- **Disk.** The index is several times larger than the text it holds, because it keeps the word lists of every field. Measure it after the first rebuild with `GET /stats` (`databaseSize`), and keep free space well above it: reindexing and upgrades need room to work.
- **CPU.** One or two cores are enough for a small server. A rebuild is the heaviest load; slow it with `my search rebuild --rate`.

### Japanese and other languages

The server puts the text of a post in the field of its language: Japanese (`ja`, including tags such as `ja-JP`), Chinese (`zh`), Korean (`ko`), or one field for every other language and for no language. It tells Meilisearch the language of the first three, and Meilisearch segments each into words with a dictionary:

- **Japanese** is segmented by Lindera with the UniDic dictionary, whether the text has kana or is kanji only. `検索`, `全文検索`, and `全文　検索` all find `全文検索の本文です`, and `東京` finds `東京都庁舎見学`.
- **Chinese** is segmented by jieba, and **Korean** by Lindera with the ko-dic dictionary.
- **Other languages** are split at spaces and punctuation; Meilisearch detects the script and language of the text itself.

A query is split into words the same way, and a post is found when it has every word of the query, the last one also as the start of a longer word (`テキ` finds `テキスト`). Only the first 10 words of a query count.

What the search cannot find:

- **A string that is not a word of the dictionary's segmentation.** Search finds words, not substrings: `京都` does not find `東京都`, and a part that the dictionary keeps inside a longer word, such as `庁舎` in `東京都庁舎見学`, is not found either.
- **Readings.** Hiragana finds the same word in katakana (`かたかな` finds `カタカナ`), but a reading does not find kanji (`ぜんぶん` does not find `全文`).
- **Near spellings.** Typo tolerance is off, so every word must be exact.
- **Japanese posts without a language.** A post whose language is missing or wrong is segmented by what Meilisearch detects, and text in kanji only is then often taken for Chinese and split differently: such a post may be found by `京都` for `東京都`, or not by `東京`. A post carries the language its author chose, or what the author's server sent.

The server folds the compatibility forms of Unicode in posts and queries before they reach Meilisearch, so half-width katakana (`ｶﾀｶﾅ`) and full-width letters and digits (`ＡＢＣ`) find and are found by their usual forms. Emoji are words too: `👍` finds the posts that have it.

Search needs no plugin, dictionary, or image of its own for any of this: the official image has the dictionaries of Japanese, Chinese, and Korean.
