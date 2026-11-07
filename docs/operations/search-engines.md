# Search engines

How to run each search engine that the full-text search of posts can use, how large it needs to be, and how far it finds Japanese and other languages written without spaces. [Full-text search of posts](search.md) covers what the index holds, turning the search on, rebuilding the index, and switching engines, whatever the engine.

## Elasticsearch

Written by the Elasticsearch backend.

## OpenSearch

`search = opensearch` keeps the index in an OpenSearch cluster that you run, or in a domain of Amazon OpenSearch Service. The server talks to the cluster's REST API and needs no plugin: every analyzer it uses comes with OpenSearch.

### Setting it up

1. Run OpenSearch 2 or 3. One node is enough to start; the server only ever reaches the address you give it.
2. Set [`search`](../reference/configuration.md#search) to `opensearch` and [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) to the URL of the REST API, such as `https://search.internal.example.org:9200`. A path is kept, for a cluster behind a reverse proxy.
3. Give the server a way in:
   - With the security plugin, a user with [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) and [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), sent with HTTP Basic authentication. Use `https` when the cluster is not on the same host or a private network you trust. The user needs to create indexes and aliases named after [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), and to read, write, and delete their documents.
   - On Amazon OpenSearch Service with an IAM access policy, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) set to `es`, which signs every request with AWS Signature Version 4. The credentials come from the AWS SDK's default chain (the execution role, the environment, the shared files), and the region from [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) or `AWS_REGION`. The role needs `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut`, and `es:ESHttpDelete` on the domain. A domain with fine-grained access control and an internal master user takes the user and password instead.
   - Amazon OpenSearch Serverless is not supported: it has no delete by query, which removing the posts of an author or of a visibility needs.
4. Start the server. It creates the index at start and refuses to start when it cannot reach the cluster or create the index.
5. Run `my search rebuild` to index the posts that are already there ([rebuilding the index](search.md#rebuilding-the-index)).

The URL, the user, and the AWS credentials never appear in the server's logs or errors. An error of the engine reaches the log as the operation, the HTTP status, and the type of the engine's error only, since the engine's own message can quote a post or a query.

### The index and its upgrades

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), `my-statuses` by default, names an alias. The documents live in the index named after it with the version of the server's mapping, such as `my-statuses-v1`, which the server creates with its settings and mapping and points the alias at. Every write goes through the alias and fails while it is missing, so OpenSearch never creates an index of its own with a guessed mapping. Do not create an index of that name yourself: the server refuses to start when the name is an index rather than an alias, or when the alias points at an index it did not name.

A release whose mapping or analysis differs raises the version, and its release notes say so. At start, it creates the new index, such as `my-statuses-v2`, and moves the alias to it. The new index starts empty, so after the upgrade:

1. Run `my search rebuild`. Until it ends, search works but finds only what was posted or rebuilt since the upgrade.
2. Delete the old index (`DELETE /my-statuses-v1`) when the rebuild has ended. The server leaves it in place and logs a warning that names it.

A server of an older release that starts against an alias already moved to a newer index uses that index as it is.

### Sizing

- **Memory**: the node needs a JVM heap of at least 512 MB (`-Xms512m -Xmx512m`) and as much again for the operating system; give it 1 GB for a small server, and more as the index grows. The heap is best kept at half of the node's memory.
- **Disk**: the index takes roughly the size of the text of the indexed posts, more for languages written without spaces, whose text is indexed as single characters and pairs. Keep at least 15% of the disk free: by default OpenSearch stops placing shards on a node above 85% and blocks writes to its indexes above 95%.
- **Shards and replicas**: the index has the cluster's default number of shards, one unless you changed it, which serves millions of posts. The default of one replica leaves a single-node cluster yellow; it works, and `PUT /my-statuses-v1/_settings` with `{"index": {"number_of_replicas": 0}}` makes it green. On a cluster of several nodes, keep a replica.
- **Load**: every search is one query of the index, newest first by post ID. Writes are one request per post, and a rebuild writes no faster than [its rate](search.md#rebuilding-the-index).

### Japanese and other languages

Every post, whatever its language, is analyzed the same way: the text is split into words at spaces and punctuation, lowercased, and full-width Latin letters and half-width katakana are folded to their usual forms. Chinese, Japanese, and Korean text, which has no spaces, is split into characters, and each pair of neighbouring characters is indexed as well as each character (the built-in `cjk_bigram` filter of OpenSearch).

- A query finds the posts that contain every word of it. In Japanese, a word of two characters or more finds the posts that contain each pair of its characters: `東京` finds `東京都の天気`, and `全文検索` finds `全文検索の本文です`. A single character, such as `本`, finds every post with that character.
- The pairs do not know where words end, so a query can find a post where its characters only happen to stand side by side: `京都` finds `東京都` as well. A query of katakana written in half-width or full-width letters finds both.
- Readings are not matched: `ぜんぶん` does not find `全文`, and inflected forms are not matched to their dictionary forms, in Japanese or in English (`running` does not find `run`).

A morphological analyzer such as the `analysis-kuromoji` plugin would split Japanese into words instead. Amazon OpenSearch Service has it on every domain, but a cluster you run needs it installed (`opensearch-plugin install analysis-kuromoji`), so the server does not use it: the same index works on any OpenSearch cluster.

## Meilisearch

Written by the Meilisearch backend.
