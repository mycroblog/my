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
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | A JSON file of your own settings of the index, as in [tuning the analysis](#tuning-the-analysis). |
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

### Tuning the analysis

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

Written by the OpenSearch backend.

## Meilisearch

Written by the Meilisearch backend.
