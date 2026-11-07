# Search engines

How to run each search engine that the full-text search of posts can use, how large it needs to be, and how far it finds Japanese and other languages written without spaces. [Full-text search of posts](search.md) covers what the index holds, turning the search on, rebuilding the index, and switching engines, whatever the engine.

## Elasticsearch

`search = elasticsearch` keeps the index in an Elasticsearch cluster of version 8 or later, self-managed or on Elastic Cloud Hosted. It has been checked against Elasticsearch 9.5 and 8.19. It needs no plugin: the analysis it uses is built into Elasticsearch, and so are Japanese and the other languages written without spaces, as [below](#japanese-and-other-languages-without-spaces).

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
   MY_SEARCH_ELASTICSEARCH_REPLICAS=0
   ```

4. Run `my search rebuild` to add the existing posts, as in [turning it on](search.md#turning-it-on).

The settings:

| Setting | What it does |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | The URL of the REST API. Use `https:` whenever the requests cross a network that others share; `http:` is accepted for a private network or a cluster with security off. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | The API key, which is the better choice. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | A user and password for basic authentication instead of a key. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | The name of the alias, `my-statuses` by default. Give each server that shares a cluster its own name, and grant its key that name followed by `*`. |
| [`search_elasticsearch_replicas`](../reference/configuration.md#search_elasticsearch_replicas) | How many replicas of the index the cluster keeps. Set `0` on a single node, whose index otherwise stays yellow because a replica has nowhere to go. |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | The certificate authority of an HTTPS cluster whose certificate the system does not trust. |

The credentials are read only from the environment, as secrets, and never appear in the server's logs or errors. An error of the cluster reaches the logs as the request's path, its status, and the type of the error, such as `index_not_found_exception`, never with the text of a post or a query.

### The index and its upgrades

The server creates and maintains the index itself. Do not create it by hand, and do not let anything else write to it.

- At start, the server looks for the alias. When there is none, it creates an index named after the alias and the version of its definition, such as `my-statuses-v1`, and points the alias at it. The definition's version is kept in the index's `_meta`.
- The server cannot start while the cluster is unreachable or refuses its key. `my config check` checks only the configuration, so check the connection by starting the server.
- An index of the name of the alias that the server did not create, such as one that Elasticsearch made by itself, stops the start with an error that names it. Delete that index, or choose another `search_elasticsearch_index`.
- When a new version of `my` changes the definition, the server upgrades the index at start. A change that only adds fields is made in place, and search goes on working; the release notes say when the posts already in the index need `my search rebuild --keep` to gain the new fields. A change of how the text is analyzed cannot be made to an existing index: the server points the alias at a new empty index, deletes the old one, and logs a warning that asks for `my search rebuild`. Run it after the upgrade. Until the rebuild ends, search finds only the posts written since. The release notes say when a version does this.
- An index made by a newer version of `my` stops an older one from starting. To roll back across such a change, delete the alias's index in Elasticsearch (find it with `GET _alias/my-statuses`), start the older version, which creates its own, and run `my search rebuild`.
- `my search clear` points the alias at a new empty index and deletes the old one in one step, which is quicker than deleting each document and gives the disk space back.

### Sizing

The index keeps only what a search needs and no copy of the text (`_source` is off). Short posts take about 150 bytes of disk each once Elasticsearch has merged its segments, and longer posts with a wider vocabulary more, so plan on 150 to 300 bytes a post: a few hundred megabytes for a million posts. Check the real size with `GET _cat/indices/my-statuses*?v`. The index has one primary shard, which holds tens of millions of posts.

A node with 1 GB of Java heap (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) and about 2 GB of memory in all serves a small or medium server. Give Elasticsearch half of the machine's memory as heap and leave the rest to the file cache, and give it more when searches slow down. Replicas need more nodes and the same disk again for each replica.

### Japanese and other languages without spaces

Every post is analyzed the same way, whatever its language, with the standard tokenizer of Elasticsearch, `cjk_width`, and `lowercase`, all built in. The tokenizer makes a token of each kanji and each hiragana, of each run of katakana, and of each word of the languages that use spaces. Each word of a query must appear in the post as a phrase of those tokens.

What this finds in Japanese:

- A word of kanji or hiragana is found wherever its characters stand together, even inside a longer word: `検索` and `本` find `全文検索の本文です`, and `東京 天気` finds `東京都の天気は晴れ`. Characters that only appear apart do not match.
- A run of katakana is matched whole: `カタカナ` finds `カタカナとABC`, but `カタ` does not find it.
- Full-width Latin letters and half-width katakana match their usual forms: `abc` finds `ＡＢＣ`, and `カタカナ` finds `ｶﾀｶﾅ`.
- Hiragana does not find the kanji of the same reading: `ぜんぶん` does not find `全文`.
- A query of one very common character, such as `の`, finds a great many posts. Results are ordered newest first, not by relevance.

Chinese and Korean are found the same way, by their characters. Words in the languages written with spaces are found whole, without stemming: `run` does not find `running`.

This choice needs no plugin and finds a word wherever its characters are, which a microblog's short and informal posts need. The `analysis-kuromoji` plugin, which splits Japanese into dictionary words, finds fewer posts for a partial word and is not used.

## OpenSearch

Written by the OpenSearch backend.

## Meilisearch

Written by the Meilisearch backend.
