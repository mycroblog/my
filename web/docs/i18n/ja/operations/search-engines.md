<!-- source: search-engines 6816d50f42dd -->
# 検索エンジン

投稿の全文検索で使える検索エンジンごとに、その動かし方、必要な規模、日本語などの分かち書きをしない言語をどこまで見つけられるかを説明します。ここにないエンジン、たとえば Groonga や Sudachi で作るインデックスは、代わりに自作の[検索プラグイン](plugins.md#searching-posts)でインデックスを保持できます。インデックスが保持する内容、検索を有効にする方法、インデックスの再構築、エンジンの切り替えは、エンジンにかかわらず[投稿の全文検索](search.md)で説明しています。

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` は、バージョン 8 以降の Elasticsearch クラスターにインデックスを保持します。自分で管理するクラスターでも、Elastic Cloud Hosted でもかまいません。Elasticsearch 9.5 と 8.19 で動作を確認しています。インデックスのフィールドはサーバーが管理し、サーバーがそこに書き込み、絞り込み、並べ替え、ページ送りをします。インデックスの設定（解析、シャード、レプリカ）は自分で管理できます。自分の設定がなければ、組み込みの設定はプラグインを必要とせず、[後述](#how-the-built-in-analysis-finds-japanese)のとおりに日本語を見つけます。

<!-- source: setting-it-up 3e02c35ff319 -->
### 設定する

1. Elasticsearch を動かします。ほとんどのサーバーでは 1 ノードで足ります。同じホストですぐに始めるには、次のようにします。

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   既定どおりセキュリティが有効だと、Elasticsearch は独自の認証局の証明書で HTTPS を提供します。`search_elasticsearch_ca_file` のために、その認証局をコンテナーからコピーします。

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. サーバーのインデックスだけを管理できる API キーを作成します。必要なのは、エイリアスとその背後のインデックスに対する `manage`、`read`、`write` だけです。

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   応答の `encoded` の値がキーです。

3. サーバーの設定を行い、再起動します。

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. [有効にする](search.md#turning-it-on)のとおり、`my search rebuild` を実行して既存の投稿を追加します。

設定は次のとおりです。

| 設定 | 内容 |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | REST API の URL。リクエストがほかの人と共有するネットワークを通るときは必ず `https:` を使います。`http:` は、プライベートネットワークやセキュリティを無効にしたクラスターのために受け付けます。 |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | API キー。こちらがおすすめです。 |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username)、[`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | キーの代わりに使う、Basic 認証のユーザーとパスワード。 |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | エイリアスの名前。既定は `my-statuses` です。クラスターを共有するサーバーにはそれぞれ別の名前を付け、そのキーにはその名前に `*` を付けたものを許可します。 |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | [解析を調整する](#tuning-the-analysis-of-elasticsearch)のとおりの、インデックスの独自の設定を書いた JSON ファイル。 |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | システムが信頼していない証明書を使う HTTPS クラスターの認証局。 |

認証情報はシークレットとして環境変数からだけ読まれ、サーバーのログやエラーに現れることはありません。クラスターのエラーは、リクエストのパス、ステータス、`index_not_found_exception` のようなエラーの種類としてログに記録され、投稿やクエリの本文が含まれることはありません。クラスターがインデックスの作成を拒否したときだけ、エラーはその理由を引用しますが、そこに出てくるのは設定とフィールドだけです。

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### インデックスとそのアップグレード

インデックスはサーバー自身が作成し、維持します。手動で作成したり、ほかのものに書き込ませたりしないでください。

- 起動時に、サーバーはエイリアスを探します。なければ、エイリアスの名前、フィールドのバージョン、設定のハッシュから名付けたインデックス（`my-statuses-v1-1f0c2e9a7b3d4c5e` など）を作成し、エイリアスをそこに向けます。バージョンとハッシュはインデックスの `_meta` に保存されます。
- クラスターに到達できない、キーを拒否される、設定を拒否されるといった場合、サーバーは起動できません。`my config check` が調べるのは設定と、設定ファイルが 1 つの JSON オブジェクトであることで、Elasticsearch がそれをどう判断するかではありません。接続と設定は、サーバーを起動して確認してください。
- サーバーが作成していない、エイリアスと同じ名前のインデックス（Elasticsearch が自分で作ったものなど）があると、その名前を示すエラーで起動が止まります。そのインデックスを削除するか、別の `search_elasticsearch_index` を選んでください。
- 設定ファイルを編集した、または新しいバージョンの `my` が組み込みの設定を変えたために、設定がインデックスを作成したときと異なる場合、サーバーは新しい設定で空のインデックスを作成してエイリアスをそこに向け、古いインデックスを削除し、`my search rebuild` を求める警告をログに記録します。これを実行してください。再構築が終わるまで、検索はそれ以降に書かれた投稿しか見つけません。Elasticsearch が新しい設定を拒否した場合、サーバーは起動せず、古いインデックスが残ります。
- 新しいバージョンの `my` がフィールドを変えると、サーバーは起動時にインデックスをアップグレードします。フィールドを追加するだけの変更はその場で行われ、検索は動き続けます。すでにインデックスにある投稿に新しいフィールドを持たせるために `my search rebuild --keep` が必要なときは、リリースノートに書かれます。それ以外のフィールドの変更には、上と同じく新しい空のインデックスと再構築が必要です。バージョンがこれを行うときは、リリースノートに書かれます。
- 新しいバージョンの `my` が作ったインデックスがあると、古いバージョンは起動できません。そのような変更をまたいで戻すには、Elasticsearch でエイリアスのインデックスを削除し（`GET _alias/my-statuses` で見つけられます）、古いバージョンを起動して自分のインデックスを作成させ、`my search rebuild` を実行します。
- `my search clear` は、エイリアスを新しい空のインデックスに向け、古いインデックスを一度に削除します。ドキュメントを 1 件ずつ削除するより速く、ディスク容量も戻ります。

<!-- source: sizing 2b1d9b604953 -->
### 規模の見積もり

インデックスは検索に必要なものだけを保持し、本文のコピーは持ちません（`_source` は無効です）。組み込みの設定では、Elasticsearch がセグメントをマージした後で、短い投稿は 1 件あたり約 80〜150 バイトのディスクを使い、長く語彙の多い投稿はそれ以上を使います。1 投稿あたり 150〜300 バイト、つまり 100 万投稿で数百 MB を見込んでください。実際の大きさは `GET _cat/indices/my-statuses*?v` で確認できます。

組み込みの設定では、インデックスのプライマリシャードは 1 つで、これに数千万件の投稿が入ります。クラスターに 2 つ目のノードができるとレプリカが 1 つ付き（`auto_expand_replicas: 0-1`）、1 ノードでも green のままです。これを変えるには、設定ファイルで `number_of_shards` と `number_of_replicas` を設定します。

Java ヒープ 1 GB（`ES_JAVA_OPTS=-Xms1g -Xmx1g`）、合計約 2 GB のメモリを持つノードで、小規模から中規模のサーバーをまかなえます。マシンのメモリの半分をヒープとして Elasticsearch に与え、残りをファイルキャッシュに残し、検索が遅くなったら増やしてください。レプリカには、さらにノードと、レプリカごとに同じだけのディスクが必要です。

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### クエリが解析をどう使うか

すべての投稿は、言語にかかわらず 1 つのテキストフィールドにインデックスされ、すべてに同じ解析が適用されます。投稿の言語によって別のアナライザーが選ばれることはありません。

- 投稿の本文は `my_text` という名前のアナライザーで解析されます。
- クエリは、全角スペースを含むスペースで語に分けられます。各語は、設定に `my_text_search` があればそれで、なければ `my_text` で解析され、フレーズとして投稿に現れる必要があります。つまり、そのトークンが同じ順序で隣り合っている必要があります。すべての語が一致しなければなりません。クエリの中のものが Elasticsearch のクエリ言語として解釈されることはありません。
- 結果は関連度ではなく新しい順に並ぶので、スコアリングのオプションは影響しません。

つまり、何が語になるかはアナライザーが決めます。1 文字ごとにトークンを作るアナライザーは、どんな文字の並びも見つけます。辞書の単語を作るアナライザーは、単語全体とその活用形などを見つけます。

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### 組み込みの解析が日本語をどう見つけるか

組み込みの `my_text` は、Elasticsearch の standard トークナイザーに `cjk_width` と `lowercase` のフィルターを組み合わせたもので、どれも組み込みです。このトークナイザーは、漢字とひらがなは 1 文字ごとに、カタカナは連続した並びごとに、スペースを使う言語は単語ごとにトークンを作ります。

- 漢字やひらがなの語は、その文字が並んでいればどこでも、長い語の中でも見つかります。`検索` と `本` は `全文検索の本文です` を見つけ、`東京 天気` は `東京都の天気は晴れ` を見つけます。離れてしか現れない文字は一致しません。
- カタカナの並びは全体で一致します。`カタカナ` は `カタカナとABC` を見つけますが、`カタ` は見つけません。`ツリー` は `スカイツリー` を見つけません。
- 全角の英字と半角カタカナは、通常の形と一致します。`abc` は `ＡＢＣ` を見つけ、`カタカナ` は `ｶﾀｶﾅ` を見つけます。
- ひらがなは同じ読みの漢字を見つけません。`ぜんぶん` は `全文` を見つけません。語のほかの形も見つかりません。`走る` は `走った` を見つけません。
- `の` のような、非常によく使われる 1 文字のクエリは、非常に多くの投稿を見つけます。

中国語と韓国語も同じように、文字で見つかります。スペースで書く言語の単語は、語幹処理なしで全体で見つかります。`run` は `running` を見つけません。

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### Elasticsearch の解析を調整する

インデックスの設定を JSON ファイルに書き、`search_elasticsearch_index_settings_file` でそのファイルを指定します。サーバーは、作成する各インデックスの `settings` として、組み込みの設定の代わりにファイル全体を使います。そのため、ファイルにはインデックスに必要なものをすべて書く必要があります。

- テキストフィールドが使うアナライザー `my_text`。これがないと、Elasticsearch はインデックスの作成を拒否し、サーバーは起動しません。
- 必要なら、クエリの語を解析するアナライザー `my_text_search`。
- `number_of_shards` や `number_of_replicas` など、そのほかのインデックスの設定。ファイルに書かなかったものはクラスターの既定値になります。
- Elasticsearch にある解析だけ。ファイルで指定するプラグインは、すべてのノードにインストールされている必要があります。

サーバーは起動時にファイルを読みます。ファイルを編集すると、[インデックスとそのアップグレード](#the-index-and-its-upgrades)のとおり次の起動で新しい空のインデックスが作られるので、変更のたびに `my search rebuild` を実行してください。変更は、まず別のクラスターか別の `search_elasticsearch_index` で試してください。

`analysis-kuromoji` プラグインを使った辞書の単語の例です。このプラグインはすべてのノードにインストールするか（`bin/elasticsearch-plugin install analysis-kuromoji`）、Elastic Cloud Hosted では拡張機能として提供されます。

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

Elasticsearch に組み込みの解析だけを使った、1 文字と 2 文字のすべての並びの例です。

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

半分が英語、半分が日本語の 20,000 件の投稿で、Elasticsearch 9.5 を使って比べた結果です。

| | 組み込み | Kuromoji | 1 文字と 2 文字 |
| --- | --- | --- | --- |
| プラグイン | なし | `analysis-kuromoji` | なし |
| インデックスの大きさ | 1.5 MB | 1.4 MB | 3.7 MB |
| 漢字の語の一部: `東京都` の中の `京都` | 見つかる | 見つからない | 見つかる |
| カタカナの語の一部: `スカイツリー` の中の `ツリー` | 見つからない | 見つかる | 見つかる |
| カタカナの語の一部: `コンピューター` の中の `コンピュータ` | 見つからない | 見つからない | 見つかる |
| 語のほかの形: `走る` で `走った` を見つける | 見つからない | 見つかる | 見つからない |
| 英単語の一部: `running` の中の `run` | 見つからない | 見つからない | 見つかる |

Kuromoji は日本語を辞書の単語に分け、それぞれを基本形に戻すので、語をどの形でも見つけ、辞書が知っている語なら複合語の中でも見つけます。辞書が 1 語として扱う語の一部は見つけず、新しい語や名前をどう分けるかは辞書によります。

1 文字と 2 文字の並びは、すべての言語で、どの語のどの部分も、語でない部分も見つけます。`run` は `brunch` も見つけます。インデックスは約 2.5 倍の大きさになり、検索で読む量も増えます。

この小さなインデックスでは、どの設定でも検索は数ミリ秒で終わりました。差はインデックスが大きくなるほど広がります。

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` は、自分で動かす OpenSearch クラスター、または Amazon OpenSearch Service のドメインにインデックスを保持します。サーバーはクラスターの REST API と通信します。インデックスのフィールドと送るクエリはサーバーが管理し、残り（アナライザー、シャード、レプリカ、そのほかのインデックスの設定）は自分で管理できます。自分の設定がなければ、OpenSearch に組み込みのアナライザーを使い、プラグインは必要ありません。

<!-- source: setting-it-up-1 11097d66bccc -->
### 設定する

1. OpenSearch 2 または 3 を動かします。始めるには 1 ノードで足ります。サーバーは指定したアドレスにしかアクセスしません。
2. [`search`](../reference/configuration.md#search) を `opensearch` に、[`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) を REST API の URL（`https://search.internal.example.org:9200` など）に設定します。リバースプロキシの背後のクラスターのために、パスはそのまま保たれます。
3. サーバーがアクセスする手段を用意します。
   - セキュリティプラグインがあるなら、[`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) と [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password) でユーザーを指定します。これは HTTP Basic 認証で送られます。クラスターが同じホストや信頼できるプライベートネットワークにないときは `https` を使ってください。ユーザーには、名前が [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index) で始まるインデックスの作成、マッピングの読み取り、削除、そのエイリアスの管理、そのドキュメントの読み取り、書き込み、削除の権限が必要です。
   - IAM のアクセスポリシーを使う Amazon OpenSearch Service では、[`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) を `es` に設定します。すべてのリクエストが AWS Signature Version 4 で署名されます。認証情報は AWS SDK の既定のチェーン（実行ロール、環境変数、共有ファイル）から、リージョンは [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) または `AWS_REGION` から取得します。ロールには、ドメインに対する `es:ESHttpGet`、`es:ESHttpPost`、`es:ESHttpPut`、`es:ESHttpDelete` が必要です。きめ細かなアクセス制御と内部マスターユーザーを使うドメインでは、代わりにユーザーとパスワードを使います。
   - Amazon OpenSearch Serverless には対応していません。投稿者や公開範囲ごとに投稿を取り除くのに必要な delete by query がないためです。
4. 必要なら、設定ファイルで[解析を調整](#tuning-the-analysis-of-opensearch)します。
5. サーバーを起動します。サーバーは起動時にインデックスを作成し、クラスターに到達できないときやインデックスを作成できないときは起動を拒否します。
6. `my search rebuild` を実行して、すでにある投稿をインデックスします（[インデックスの再構築](search.md#rebuilding-the-index)）。

URL、ユーザー、AWS の認証情報、設定ファイルの内容が、サーバーのログやエラーに現れることはありません。エンジン自身のメッセージは投稿やクエリを引用することがあるので、エンジンのエラーは操作、HTTP ステータス、エンジンのエラーの種類だけがログに記録されます。唯一の例外はインデックスの作成を拒否されたときで、そのメッセージは問題のある設定を示します。

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### インデックスとそのアップグレード

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index)（既定は `my-statuses`）はエイリアスの名前です。ドキュメントは、サーバーが作成してエイリアスを向けるインデックスに保存されます。このインデックスは、エイリアスの名前に、サーバーのマッピングのバージョンとインデックスの設定のハッシュを付けて名付けられます（`my-statuses-v1-3f2a9c41b07e` など）。マッピングはこの 2 つを `_meta`（`my_mapping_version` と `my_settings_hash`）にも記録します。書き込みはすべてエイリアスを通り、エイリアスがない間は失敗するので、OpenSearch が推測したマッピングで独自にインデックスを作ることはありません。その名前のインデックスを自分で作成しないでください。その名前がエイリアスではなくインデックスのとき、またはエイリアスがサーバーの作成していないインデックスを指しているとき、サーバーは起動を拒否します。

起動時に、サーバーはエイリアスの背後のインデックスを、自分のマッピングのバージョンと設定と比べます。どちらかが異なる場合（マッピングが変わったリリース。リリースノートにそう書かれます。または設定ファイルを編集した場合）、サーバーは新しいインデックスを作成し、エイリアスをそこに移します。新しいインデックスは空から始まるので、次のようにします。

1. `my search rebuild` を実行します。これが終わるまで、検索は動きますが、変更以降に投稿されたか再構築されたものしか見つけません。
2. 再構築が終わったら、古いインデックスを削除します（`DELETE /my-statuses-v1-…`）。サーバーは古いインデックスをそのまま残し、その名前を示す警告をログに記録します。

サーバーのすべてのプロセスを同じ設定ファイルで起動してください。ファイルの異なるプロセスがあると、エイリアスが行ったり来たりします。新しいマッピングのインデックスに対して古いリリースのサーバーが起動した場合は、そのインデックスをそのまま使います。

`my search clear` と、それから始まる `--keep` なしの `my search rebuild` は、同じマッピングと設定で新しい空のインデックスを作成し、エイリアスをそこに移し、古いインデックスを 1 回のリクエストで削除します。

<!-- source: sizing-1 4ad4bbcd9a2b -->
### 規模の見積もり

- **メモリ**: ノードには少なくとも 512 MB の JVM ヒープ（`-Xms512m -Xmx512m`）と、それと同じだけのオペレーティングシステム用のメモリが必要です。小規模なサーバーには 1 GB を与え、インデックスが大きくなるにつれて増やしてください。ヒープはノードのメモリの半分にしておくのが最適です。
- **ディスク**: 組み込みの設定では、インデックスはインデックスした投稿の本文とおおよそ同じ大きさになります。分かち書きをしない言語では、本文が 1 文字ずつと 2 文字の組としてインデックスされるので大きくなり、n-gram を使うとさらに大きくなります。ディスクの少なくとも 15% を空けておいてください。既定では、OpenSearch は使用率が 85% を超えたノードにシャードを置かなくなり、95% を超えるとそのインデックスへの書き込みを止めます。
- **シャードとレプリカ**: 設定ファイルがなければ、インデックスはクラスターの既定値を使います。変更していなければシャード 1 つとレプリカ 1 つです。シャード 1 つで数百万件の投稿をまかなえます。レプリカが 1 つあると、1 ノードのクラスターは yellow のままです。動作はしますが、設定ファイルに `"index": {"number_of_replicas": 0}` と書くと green になります。複数ノードのクラスターでは、レプリカを残してください。レプリカの数は稼働中のインデックスでも変えられ（`PUT /my-statuses/_settings`）、再構築は必要ありません。設定ファイルが変わらない限り、サーバーはそのインデックスを使い続けます。
- **負荷**: 検索は 1 回ごとにインデックスへの 1 つのクエリで、投稿 ID の新しい順です。書き込みは投稿ごとに 1 リクエストで、再構築は[その速度](search.md#rebuilding-the-index)より速くは書き込みません。

<!-- source: japanese-and-other-languages a48cff99706b -->
### 日本語とほかの言語

組み込みの設定では、すべての投稿が言語にかかわらず同じように解析されます。本文はスペースと句読点で語に分けられ、小文字にされ、全角の英字と半角カタカナは通常の形にそろえられます。スペースのない中国語、日本語、韓国語の本文は文字に分けられ、各文字に加えて隣り合う 2 文字の組もインデックスされます（OpenSearch に組み込みの `cjk_bigram` フィルター）。

- クエリは、そのすべての語を含む投稿を見つけます。日本語では、2 文字以上の語は、その文字の組をすべて含む投稿を見つけます。`東京` は `東京都の天気` を見つけ、`全文検索` は `全文検索の本文です` を見つけます。`本` のような 1 文字は、その文字を含むすべての投稿を見つけます。
- 2 文字の組は語の切れ目を知らないので、クエリの文字がたまたま隣り合っているだけの投稿も見つかることがあります。`京都` は `東京都` も見つけます。半角と全角のどちらで書いたカタカナのクエリも、両方を見つけます。
- 読みでは一致しません。`ぜんぶん` は `全文` を見つけません。活用形も、日本語でも英語でも辞書形とは一致しません（`running` は `run` を見つけません）。

日本語を単語で見つけるには、[設定ファイル](#tuning-the-analysis-of-opensearch)で kuromoji などの形態素解析器を使ってください。

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### OpenSearch の解析を調整する

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) は JSON ファイルを指定します。そのオブジェクトは、サーバーが作成するすべてのインデックスの `settings`（`analysis`、`index.number_of_shards`、`index.number_of_replicas`、そのほかのインデックスの設定）になります。このファイルには設定だけを書きます。マッピングはサーバーが管理し、`mappings` を含むファイルは拒否されます。

ファイルには、マッピングが指定する 2 つのアナライザーを定義する必要があります。

| アナライザー | 解析するもの |
| --- | --- |
| `my_text` | 各投稿の本文、注意書き、メディアの代替テキスト |
| `my_text_search` | 各クエリの本文 |

2 つが同じ定義でもかまいません。各クエリは、フィールド `text` に対する `"operator": "and"` 付きの `match` クエリです。クエリの本文は `my_text_search` を通り、出てきたすべての語を本文に持つ投稿が、スコアリングなしで投稿 ID の新しい順に見つかります。アナライザーが語を 1 つも残さないクエリは、何も見つけません。フレーズ、あいまい、前方一致の照合は使わないので、何が一致になるかはアナライザーだけが決めます。投稿の言語は保存されますが、アナライザーは選びません。すべての投稿は `my_text` を通ります。

インデックスと設定が異なる（レプリカ 1 つの違いでも）ファイルでサーバーが起動すると、新しい空のインデックスが作られ、インデックスの再構築が必要になります（[インデックスとそのアップグレード](#the-index-and-its-upgrades)）。書式やキーの順序は数に入りません。2 つのアナライザーがない、クラスターにないプラグインを使っているなど、OpenSearch が拒否するファイルでは、OpenSearch の理由とともに起動が止まります。

Kuromoji は日本語を辞書で単語に分け、活用形をその辞書形で照合します。Amazon OpenSearch Service ではすべてのドメインにこのプラグインがあります。自分で動かすクラスターにはインストールが必要です（`opensearch-plugin install analysis-kuromoji`。たとえば `FROM opensearchproject/opensearch` の Dockerfile で）。kuromoji を使うファイルの例です。

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

辞書がクエリと違う分け方をする語は見つからないので、kuromoji では、複合名詞の一部や新しい語など、組み込みの 2 文字の組なら見つかるものを見逃すことがあります。OpenSearch に組み込みの n-gram トークナイザーは、インデックスが大きくなり、長い語の中でも一致してしまう代わりに、どんな文字の並びも見つけます。`"min_gram": 2` と `"max_gram": 3`（と `"index": {"max_ngram_diff": 1}`）にすると、1 文字のクエリは何も見つけません。

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` は、自分でホストする [Meilisearch](https://www.meilisearch.com/docs) サーバー、または Meilisearch Cloud にインデックスを保持します。動かすクラスターのない単一のプロセスなので小規模なサーバーに向いており、公式イメージはプラグインなしで日本語、中国語、韓国語を分かち書きします。Meilisearch 1.56 で動作を確認しています。

<!-- source: running-meilisearch 4f7d5e17c255 -->
### Meilisearch を動かす

公式イメージを 16 バイト以上のマスターキーとともに、`my` だけが到達できるネットワークで動かし、データディレクトリは永続ボリュームに置きます。

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` にすると、マスターキーがなければ起動を拒否し、組み込みの検索プレビューを無効にします。Meilisearch は平文の HTTP で通信します。`my` がローカルやプライベートのリンク以外を通って到達するときは、TLS の背後に置くか、プライベートネットワークに置いてください。

<!-- source: settings 74547d71e962 -->
### 設定

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

キーは設定ファイルには書かず、環境変数（`MY_SEARCH_MEILISEARCH_API_KEY` または `MY_SEARCH_MEILISEARCH_API_KEY_FILE`）で設定します。マスターキーでもかまいませんが、`my` に必要なことだけができるキーのほうが安全です。マスターキーを使って、次のように作成します。

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

応答の `key` を `search_meilisearch_api_key` として使います。キーは `Authorization` ヘッダーでだけ送られます。サーバーのログやエラーには Meilisearch のエラーコード（`invalid_api_key` など）が表示され、キー、投稿、クエリが表示されることはありません。[すべての設定](../reference/configuration.md#search_meilisearch_url)は設定リファレンスにあります。

複数のサーバーが、それぞれ別の `search_meilisearch_index` と、自分のインデックスに限ったキーを使って、1 つの Meilisearch サーバーを共有できます。

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### インデックスとその設定

インデックスがなければ、サーバーが作成します。起動後に初めてインデックスを使うとき、サーバーは送ろうとする設定を前回適用した設定と比べ、異なるときだけ送ります。前回の設定は、どの検索にも見つからないインデックス内のドキュメントにハッシュとして記録されています。何も変えずに再起動しても、何も送りません。設定は次のとおりです。

- **サーバー自身が設定するもの:** `searchableAttributes`（投稿のテキストフィールド）、`displayedAttributes`（すべて）、`filterableAttributes`（投稿 ID、投稿者、公開範囲）、`sortableAttributes`（投稿 ID）。検索はこれらに依存しており、これらのどれかを設定する設定ファイルは拒否されます。
- **調整できるもの:** それ以外のすべてで、[設定ファイル](#tuning-meilisearch)に書きます。ファイルがないとき、またはファイルに書かなかったものには、サーバーの既定値を使います。`rankingRules` は `sort`、`words`、`typo`、`proximity`、`attribute`、`exactness`。タイポ許容はなし。`localizedAttributes` には日本語、中国語、韓国語のフィールドの言語。残り（`stopWords`、`synonyms`、`dictionary`、`separatorTokens`、`nonSeparatorTokens`、`proximityPrecision`、`pagination`、`faceting`、`distinctAttribute`、`searchCutoffMs`、`prefixSearch`）は Meilisearch 自身の既定値です。エンジン上で手動で変えた設定は、次にサーバーが設定を送るまでしか残りません。

**設定の変更に `my search rebuild` は必要ありません。** Meilisearch は保持しているドキュメントを自分でバックグラウンドでインデックスし直し、投稿はインデックスに残ります。時間がかかるだけで、その長さはおおよそ最初にインデックスしたときと同じです。サーバーの起動はこれを待たず、サーバーは設定を送って先に進みます。その間、検索は以前の設定で応答し、Meilisearch はインデックスのタスクを順に実行するので、インデックスへの書き込みはインデックスし直しの後ろで待ちます。書き込みが 10 秒以内に終わらないジョブは後で再試行するので、インデックスし直しの間に書かれた投稿は、それが終わった後にインデックスに入ります。`my search clear` は新しい空のインデックスの設定を待ちますが、これには時間がかかりません。

再構築が必要なのは、インデックスが空から始まるときだけです。

- `search_meilisearch_index` を変えたとき。
- たとえば古い Meilisearch サーバーをアップグレードする代わりに、新しい空の Meilisearch サーバーを起動したとき。
- `my` のあるバージョンのリリースノートに、ドキュメントの書き方を変えたと書かれているとき。

書き込みは、Meilisearch が順に実行するタスクです。サーバーは書き込みごとにそのタスクを待ち、タスクが失敗するかジョブの時間内に終わらないとジョブを再試行します。そのため、Meilisearch が停止していたり混んでいたりしても、インデックスが遅れるだけで変更は失われません。設定ファイルの値を Meilisearch が拒否したなどの理由で設定のタスクが失敗すると、それが成功するまで書き込みが失敗します。エンジンで `GET /tasks?types=settingsUpdate` を実行するとそのエラーが分かります。`my search clear` はインデックスを削除し、設定とともに作り直します。

Meilisearch 自体をアップグレードするには、そのアップグレードガイド（ダンプ、またはダンプなしのアップグレード）に従うか、新しいバージョンを空で起動して `my search rebuild` を実行します。

<!-- source: what-a-search-sends 575b89133606 -->
### 検索が送るもの

検索のクエリは、設定ファイルの内容にかかわらず固定です。投稿がすべての語を持つように `matchingStrategy: all`、新しい順にするために `sort: status_id:desc`、次のページのための投稿 ID に対する `filter`、ページの件数に 1 を足した `limit`、投稿 ID の `attributesToRetrieve`、そして語のないクエリを見分けるための `showMatchesPosition` です。Meilisearch が演算子として解釈するフレーズの二重引用符と語の前のマイナス記号は取り除かれるので、クエリは常にただの語の並びです。

<!-- source: tuning-meilisearch df827448e06c -->
### Meilisearch を調整する

`search_meilisearch_settings_file` で指定する [Meilisearch のインデックスの設定](https://www.meilisearch.com/docs/reference/api/settings)の JSON ファイルで、エンジンを調整します。`search = meilisearch` のとき、サーバーは起動時にこれを確認します。ファイルが存在して JSON オブジェクトであること、サーバー自身が設定する 4 つの設定を含まないこと、`rankingRules` があるならそれが `sort` で始まること（結果を新しい順に保つため）が必要です。値は、サーバーが送ったときに Meilisearch が確認します。サーバーがファイルの内容をログに記録することはありません。

言語を日本語に絞り、`・` を区切りとして扱い、近接度のインデックスを軽くし、長い語でタイポ許容を有効にする例です。

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

変更にかかるコストは次のとおりです。

- ファイルを変更するたびに、次の起動で、上のとおり Meilisearch がすべてのドキュメントをインデックスし直します。データは失われず再構築も必要ありませんが、それが終わるまで書き込みは待ちます。静かな時間帯に変更し、`GET /tasks?types=settingsUpdate` でタスクを見守ってください。
- `localizedAttributes` とトークンは、本文を語にどう分けるかを決めます。ロケールの一覧を狭くすると、ほかの言語の投稿を間違った言語として分かち書きすることがあります（下記を参照）。そのため、サーバーにそれらの投稿がまったくない場合を除き、日本語、中国語、韓国語のフィールドは残してください。
- タイポ許容と同義語を使うと、検索は正確な語以外のものも見つけます。同じ ID の投稿は 2 つとないので、`sort` の後のランキングルールは何も変えません。省いても Meilisearch の作業が少し減るだけです。

<!-- source: sizing-2 1d581fac5316 -->
### 規模の見積もり

Meilisearch はインデックスをディスク上のメモリマップトファイルに保持します。インデックスより少ないメモリでも動き、インデックスのうちメモリに収まる部分が多いほど速くなります。

- **メモリ。** 小規模なサーバーなら 1 GB から始めるのが妥当です。インデックスが大きくなるにつれて増やし、理想的にはインデックスのディスク上の大きさと同じだけ与えてください。インデックス処理は既定でマシンのメモリの 3 分の 2 まで使います。ほかのサービスと共有するマシンでは、`MEILI_MAX_INDEXING_MEMORY`（`512Mb` など）で上限を設けてください。
- **ディスク。** インデックスはすべてのフィールドの語のリストを保持するので、保持する本文の数倍の大きさになります。最初の再構築の後に `GET /stats`（`databaseSize`）で測り、それを十分に上回る空き容量を確保してください。インデックスし直しやアップグレードには作業のための余地が必要です。
- **CPU。** 小規模なサーバーなら 1〜2 コアで足ります。最も重い負荷は再構築です。`my search rebuild --rate` で遅くできます。

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### 日本語とほかの言語

サーバーは投稿の本文を、その言語のフィールドに入れます。日本語（`ja`。`ja-JP` のようなタグを含む）、中国語（`zh`）、韓国語（`ko`）、そしてそれ以外のすべての言語と言語なしのための 1 つのフィールドです。最初の 3 つについてはその言語を Meilisearch に伝え、Meilisearch はそれぞれを辞書で語に分けます。

- **日本語**は、かなを含むか漢字だけかにかかわらず、UniDic 辞書を使う Lindera で分かち書きされます。`検索`、`全文検索`、`全文　検索` はどれも `全文検索の本文です` を見つけ、`東京` は `東京都庁舎見学` を見つけます。
- **中国語**は jieba で、**韓国語**は ko-dic 辞書を使う Lindera で分かち書きされます。
- **ほかの言語**はスペースと句読点で分けられます。本文の文字体系と言語は Meilisearch 自身が判定します。

クエリも同じように語に分けられ、クエリのすべての語を持つ投稿が見つかります。最後の語は、より長い語の先頭としても一致します（`テキ` は `テキスト` を見つけます）。クエリの最初の 10 語だけが数に入ります。

検索で見つけられないものは次のとおりです。

- **辞書の分かち書きで語にならない文字列。** 検索が見つけるのは部分文字列ではなく語です。`京都` は `東京都` を見つけず、`東京都庁舎見学` の中の `庁舎` のように、辞書が長い語の中に含めている部分も見つかりません。
- **読み。** ひらがなは同じ語のカタカナを見つけます（`かたかな` は `カタカナ` を見つけます）が、読みで漢字は見つかりません（`ぜんぶん` は `全文` を見つけません）。
- **近い綴り。** タイポ許容が無効なので、すべての語が正確である必要があります。
- **言語のない日本語の投稿。** 言語がない、または間違っている投稿は Meilisearch の判定に従って分かち書きされ、漢字だけの本文はよく中国語と判定されて違う分け方をされます。そのような投稿は、`東京都` が `京都` で見つかったり、`東京` で見つからなかったりします。投稿は、投稿者が選んだ言語、または投稿者のサーバーが送った言語を持ちます。

サーバーは、投稿とクエリが Meilisearch に届く前に Unicode の互換形を正規化するので、半角カタカナ（`ｶﾀｶﾅ`）や全角の英字と数字（`ＡＢＣ`）は、通常の形を見つけ、通常の形からも見つかります。絵文字も語です。`👍` はそれを含む投稿を見つけます。

これらのどれにも、検索のための独自のプラグイン、辞書、イメージは必要ありません。公式イメージに日本語、中国語、韓国語の辞書が含まれています。
