# Search engines

How to run each search engine that the full-text search of posts can use, how large it needs to be, and how far it finds Japanese and other languages written without spaces. [Full-text search of posts](search.md) covers what the index holds, turning the search on, rebuilding the index, and switching engines, whatever the engine.

## Elasticsearch

Written by the Elasticsearch backend.

## OpenSearch

Written by the OpenSearch backend.

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
```

Set the key through the environment (`MY_SEARCH_MEILISEARCH_API_KEY`, or `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), never in the config file. It may be the master key, but a key that can do only what `my` needs is safer. Create one with the master key:

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.delete", "indexes.create", "indexes.get", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Use the `key` of the response as `search_meilisearch_api_key`. The key is sent only in the `Authorization` header: the server's logs and errors show the Meilisearch error code (such as `invalid_api_key`) and never the key, a post, or a query. All [the settings](../reference/configuration.md#search_meilisearch_url) are in the configuration reference.

Several servers can share one Meilisearch server with a different `search_meilisearch_index` each, and a key limited to its own index.

### The index and its settings

The server creates the index when it does not exist, and sends the settings it needs the first time it uses the index after each start: the fields to search, filter, and sort on, the language of each text field, the order of the results, and no typo tolerance. Do not change these settings by hand: the next start puts them back. When a new version of `my` changes them, Meilisearch reindexes the documents it holds by itself in the background, and nothing needs a rebuild. Until it finishes, the writes of the index wait behind it, and their jobs retry when they run out of time.

A rebuild (`my search rebuild`, see [rebuilding the index](search.md#rebuilding-the-index)) is needed when:

- You change `search_meilisearch_index`: the new index starts empty.
- You start a new, empty Meilisearch server, for example rather than upgrading the old one.
- The release notes of a version of `my` say that it writes the documents differently.

Writes are tasks that Meilisearch runs in order. The server waits for the task of each write, and a task that fails or does not end within the time of the job makes the job retry, so a Meilisearch that is down or busy delays the index without losing a change. The tasks Meilisearch keeps are listed by `GET /tasks`, with the error of any that failed.

To upgrade Meilisearch itself, follow its upgrade guide (a dump, or its dumpless upgrade), or start the new version empty and run `my search rebuild`.

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
