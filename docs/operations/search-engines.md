# Search engines

How to run each search engine that the full-text search of posts can use, how large it needs to be, and how far it finds Japanese and other languages written without spaces. [Full-text search of posts](search.md) covers what the index holds, turning the search on, rebuilding the index, and switching engines, whatever the engine.

## Elasticsearch

Written by the Elasticsearch backend.

## OpenSearch

`search = opensearch` keeps the index in an OpenSearch cluster that you run, or in a domain of Amazon OpenSearch Service. The server talks to the cluster's REST API. It owns the fields of the index and the queries it sends, and you may own the rest: the analyzers, the shards, the replicas, and any other index setting. Without settings of your own, it uses analyzers built into OpenSearch and needs no plugin.

### Setting it up

1. Run OpenSearch 2 or 3. One node is enough to start; the server only ever reaches the address you give it.
2. Set [`search`](../reference/configuration.md#search) to `opensearch` and [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) to the URL of the REST API, such as `https://search.internal.example.org:9200`. A path is kept, for a cluster behind a reverse proxy.
3. Give the server a way in:
   - With the security plugin, a user with [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) and [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), sent with HTTP Basic authentication. Use `https` when the cluster is not on the same host or a private network you trust. The user needs to create, read the mapping of, and delete the indexes whose names start with [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), to manage that alias, and to read, write, and delete their documents.
   - On Amazon OpenSearch Service with an IAM access policy, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) set to `es`, which signs every request with AWS Signature Version 4. The credentials come from the AWS SDK's default chain (the execution role, the environment, the shared files), and the region from [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) or `AWS_REGION`. The role needs `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut`, and `es:ESHttpDelete` on the domain. A domain with fine-grained access control and an internal master user takes the user and password instead.
   - Amazon OpenSearch Serverless is not supported: it has no delete by query, which removing the posts of an author or of a visibility needs.
4. Optionally, [tune the analysis](#tuning-the-analysis) with a settings file.
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

To find Japanese by words, use a morphological analyzer such as kuromoji in a [settings file](#tuning-the-analysis).

### Tuning the analysis

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

Written by the Meilisearch backend.
