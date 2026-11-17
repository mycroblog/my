<!-- source: search-engines 6816d50f42dd -->
# 搜索引擎

本页说明如何运行帖子全文搜索可以使用的每种搜索引擎、它需要多大的规模，以及它对日语和其他不用空格分词的语言能搜到什么程度。对于这里没有列出的引擎，例如 Groonga 或用 Sudachi 构建的索引，可以改由你自己的[搜索插件](plugins.md#searching-posts)维护索引。无论使用哪种引擎，索引保存什么、如何开启搜索、如何重建索引以及如何更换引擎，见[帖子全文搜索](search.md)。

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` 把索引保存在版本 8 或更高的 Elasticsearch 集群中，可以是自行管理的集群，也可以是 Elastic Cloud Hosted。它已在 Elasticsearch 9.5 和 8.19 上验证过。服务器管理索引的字段，用于写入、筛选、排序和分页。索引的设置可以由你管理：分析、分片和副本。没有你自己的设置时，内置设置无需任何插件，对日语的搜索方式见[下文](#how-the-built-in-analysis-finds-japanese)。

<!-- source: setting-it-up 3e02c35ff319 -->
### 设置

1. 运行 Elasticsearch。对大多数服务器来说，单个节点就足够了。要在同一台主机上快速开始：

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   安全功能开启时（这是默认设置），Elasticsearch 使用由它自己的证书颁发机构签发的证书提供 HTTPS。把该证书颁发机构从容器中复制出来，用于 `search_elasticsearch_ca_file`：

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. 创建一个只能管理服务器索引的 API 密钥。它只需要对别名及其背后的索引拥有 `manage`、`read` 和 `write` 权限：

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   响应中的 `encoded` 值就是密钥。

3. 设置服务器的配置并重启：

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. 按照[开启](search.md#turning-it-on)中的说明，运行 `my search rebuild` 加入现有的帖子。

设置项：

| 设置 | 作用 |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | REST API 的 URL。只要请求会经过他人共用的网络，就使用 `https:`；私有网络或关闭了安全功能的集群也接受 `http:`。 |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | API 密钥，这是更好的选择。 |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username)、[`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | 用于基本认证的用户名和密码，代替密钥。 |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | 别名的名称，默认为 `my-statuses`。共用一个集群的每台服务器都要使用自己的名称，并授予其密钥对该名称加 `*` 的权限。 |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | 你自己的索引设置的 JSON 文件，见[调整分析](#tuning-the-analysis-of-elasticsearch)。 |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | HTTPS 集群的证书颁发机构，用于系统不信任其证书的情况。 |

凭据只作为机密从环境中读取，绝不会出现在服务器的日志或错误中。集群的错误只以请求的路径、状态和错误类型（例如 `index_not_found_exception`）出现在日志中，绝不会带有帖子或查询的文本。只有在集群拒绝创建索引时，错误才会引用其原因，而原因只涉及设置和字段。

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### 索引及其升级

服务器自行创建和维护索引。不要手动创建它，也不要让其他任何程序写入它。

- 启动时，服务器查找别名。没有别名时，它会创建一个以别名、字段版本和设置哈希命名的索引，例如 `my-statuses-v1-1f0c2e9a7b3d4c5e`，并让别名指向它。版本和哈希保存在索引的 `_meta` 中。
- 集群无法访问、拒绝其密钥或拒绝其设置时，服务器无法启动。`my config check` 检查配置以及设置文件是否是一个 JSON 对象，但不检查 Elasticsearch 是否接受它，因此请通过启动服务器来检查连接和设置。
- 如果存在一个与别名同名、但不是服务器创建的索引（例如 Elasticsearch 自动创建的索引），启动会因一个指明它的错误而停止。删除该索引，或选择另一个 `search_elasticsearch_index`。
- 当设置与创建索引时的设置不同时（因为你编辑了设置文件，或新版本的 `my` 更改了内置设置），服务器会用新设置创建一个新的空索引，让别名指向它，删除旧索引，并记录一条要求运行 `my search rebuild` 的警告。请运行它。在重建结束之前，搜索只能找到此后写入的帖子。如果 Elasticsearch 拒绝新设置，服务器不会启动，旧索引保持不变。
- 当新版本的 `my` 更改了字段时，服务器会在启动时升级索引。只添加字段的更改会就地进行，搜索可以继续使用；发行说明会写明索引中已有的帖子何时需要运行 `my search rebuild --keep` 才能获得新字段。字段的其他任何更改都需要新的空索引和重建，如上所述。发行说明会写明哪个版本会这样做。
- 由较新版本的 `my` 创建的索引会阻止较旧版本启动。要跨越这种更改回滚，请在 Elasticsearch 中删除别名对应的索引（用 `GET _alias/my-statuses` 查找），启动较旧的版本（它会创建自己的索引），然后运行 `my search rebuild`。
- `my search clear` 在一步之内让别名指向一个新的空索引并删除旧索引，这比逐个删除文档更快，并且会释放磁盘空间。

<!-- source: sizing 2b1d9b604953 -->
### 规模

索引只保存搜索所需的内容，不保存文本的副本（`_source` 已关闭）。使用内置设置时，在 Elasticsearch 合并其段之后，每条短帖子大约占用 80 到 150 字节的磁盘空间，词汇更丰富的长帖子占用更多，因此请按每条帖子 150 到 300 字节来规划：一百万条帖子需要几百 MB。用 `GET _cat/indices/my-statuses*?v` 查看实际大小。

内置设置为索引提供一个主分片（可容纳数千万条帖子），并在集群有第二个节点后提供一个副本（`auto_expand_replicas: 0-1`），因此单节点集群保持绿色。要更改它们，请在设置文件中设置 `number_of_shards` 和 `number_of_replicas`。

一个拥有 1 GB Java 堆（`ES_JAVA_OPTS=-Xms1g -Xmx1g`）、总内存约 2 GB 的节点，足以服务中小型服务器。把机器内存的一半作为堆分配给 Elasticsearch，其余留给文件缓存，搜索变慢时再增加。副本需要更多节点，每个副本还需要同样多的磁盘空间。

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### 查询如何使用分析

无论语言如何，每条帖子都被索引到同一个文本字段中，并对所有帖子应用相同的分析。帖子的语言不会选择另一个分析器。

- 帖子的文本用名为 `my_text` 的分析器分析。
- 查询在空格（包括全角空格）处被拆分为多个词。如果设置中定义了 `my_text_search`，每个词用它分析，否则用 `my_text` 分析，并且必须作为短语出现在帖子中：其词元顺序相同且彼此相邻。每个词都必须匹配。查询中的任何内容都不会被当作 Elasticsearch 的查询语言解读。
- 结果按从新到旧排序，而不是按相关性排序，因此评分选项不起作用。

因此，由分析器决定什么是一个词。把每个字符都作为一个词元的分析器，可以找到任何连续的字符。生成词典词语的分析器，可以找到完整的词及其其他形式。

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### 内置分析如何搜索日语

内置的 `my_text` 是 Elasticsearch 的标准分词器，加上 `cjk_width` 和 `lowercase` 过滤器，全部是内置的。该分词器把每个汉字和每个平假名、每串连续的片假名，以及使用空格的语言中的每个词都作为一个词元。

- 由汉字或平假名构成的词，只要其字符连在一起，即使在更长的词中也能找到：`検索` 和 `本` 能找到 `全文検索の本文です`，`東京 天気` 能找到 `東京都の天気は晴れ`。只是分开出现的字符不匹配。
- 连续的片假名要整体匹配：`カタカナ` 能找到 `カタカナとABC`，但 `カタ` 找不到它，`ツリー` 也找不到 `スカイツリー`。
- 全角拉丁字母和半角片假名与其常规形式匹配：`abc` 能找到 `ＡＢＣ`，`カタカナ` 能找到 `ｶﾀｶﾅ`。
- 平假名找不到同音的汉字：`ぜんぶん` 找不到 `全文`。词的其他形式也找不到：`走る` 找不到 `走った`。
- 由一个非常常见的字符构成的查询（例如 `の`）会找到大量帖子。

中文和韩语也以同样的方式按字符查找。使用空格的语言中的词要整体匹配，不做词干提取：`run` 找不到 `running`。

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### 调整 Elasticsearch 的分析

把索引的设置写入一个 JSON 文件，并在 `search_elasticsearch_index_settings_file` 中指定它。服务器会把整个文件用作它创建的每个索引的 `settings`，代替内置设置，因此文件必须包含索引所需的一切：

- 分析器 `my_text`，供文本字段使用。没有它，Elasticsearch 会拒绝创建索引，服务器也不会启动。
- 可选的分析器 `my_text_search`，此时由它分析查询中的词。
- 其他任何索引设置，例如 `number_of_shards` 和 `number_of_replicas`。文件中没有的设置采用集群的默认值。
- 只能使用 Elasticsearch 具备的分析：文件中指定的插件必须安装在每个节点上。

服务器在启动时读取该文件。编辑它会让下一次启动得到一个新的空索引，如[索引及其升级](#the-index-and-its-upgrades)所述，因此每次更改后都要运行 `my search rebuild`。请先在另一个集群或另一个 `search_elasticsearch_index` 上尝试更改。

使用 `analysis-kuromoji` 插件按词典词语分析，该插件需要安装在每个节点上（`bin/elasticsearch-plugin install analysis-kuromoji`），Elastic Cloud Hosted 也以扩展的形式提供它：

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

只使用 Elasticsearch 内置的分析，索引每一串一个或两个字符：

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

在 Elasticsearch 9.5 上，用 20,000 条帖子（一半英语，一半日语）测得的比较结果：

| | 内置 | Kuromoji | 一个和两个字符 |
| --- | --- | --- | --- |
| 插件 | 无 | `analysis-kuromoji` | 无 |
| 索引大小 | 1.5 MB | 1.4 MB | 3.7 MB |
| 汉字词的一部分：`東京都` 中的 `京都` | 能找到 | 找不到 | 能找到 |
| 片假名词的一部分：`スカイツリー` 中的 `ツリー` | 找不到 | 能找到 | 能找到 |
| 片假名词的一部分：`コンピューター` 中的 `コンピュータ` | 找不到 | 找不到 | 能找到 |
| 词的其他形式：`走る` 找到 `走った` | 找不到 | 能找到 | 找不到 |
| 英语单词的一部分：`running` 中的 `run` | 找不到 | 找不到 | 能找到 |

Kuromoji 把日语拆分为其词典中的词，并把每个词还原为原形，因此能找到一个词的任何形式，也能找到复合词中词典认识的词。它找不到被词典视为整体的词的一部分，新词或名称如何拆分取决于词典。

一个和两个字符的组合能找到任何语言中任何词的任何部分，包括不是词的部分：`run` 也能找到 `brunch`。索引大约是两倍半大，搜索时也要读取更多内容。

在这个小索引上，三种方式的搜索都只需几毫秒，差异会随索引的增大而扩大。

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` 把索引保存在你运行的 OpenSearch 集群中，或 Amazon OpenSearch Service 的域中。服务器与集群的 REST API 通信。它管理索引的字段和它发送的查询，其余部分可以由你管理：分析器、分片、副本和其他任何索引设置。没有你自己的设置时，它使用 OpenSearch 内置的分析器，无需任何插件。

<!-- source: setting-it-up-1 11097d66bccc -->
### 设置

1. 运行 OpenSearch 2 或 3。一个节点就足以开始；服务器只会访问你提供的地址。
2. 把 [`search`](../reference/configuration.md#search) 设为 `opensearch`，把 [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) 设为 REST API 的 URL，例如 `https://search.internal.example.org:9200`。路径会被保留，以便使用位于反向代理后面的集群。
3. 为服务器提供访问方式：
   - 使用安全插件时，通过 [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) 和 [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password) 指定一个用户，以 HTTP Basic 认证发送。当集群不在同一台主机上，也不在你信任的私有网络中时，请使用 `https`。该用户需要能够创建名称以 [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index) 开头的索引、读取其映射并删除它们，管理该别名，以及读取、写入和删除其中的文档。
   - 在使用 IAM 访问策略的 Amazon OpenSearch Service 上，把 [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) 设为 `es`，这会用 AWS Signature Version 4 为每个请求签名。凭据来自 AWS SDK 的默认链（执行角色、环境、共享文件），区域来自 [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) 或 `AWS_REGION`。该角色需要对该域拥有 `es:ESHttpGet`、`es:ESHttpPost`、`es:ESHttpPut` 和 `es:ESHttpDelete` 权限。启用了精细访问控制并使用内部主用户的域，则改用用户名和密码。
   - 不支持 Amazon OpenSearch Serverless：它没有按查询删除的功能，而移除某个作者或某个可见范围的帖子需要这一功能。
4. 可选：用设置文件[调整分析](#tuning-the-analysis-of-opensearch)。
5. 启动服务器。它会在启动时创建索引，无法访问集群或无法创建索引时拒绝启动。
6. 运行 `my search rebuild`，索引已有的帖子（[重建索引](search.md#rebuilding-the-index)）。

URL、用户、AWS 凭据和设置文件的内容绝不会出现在服务器的日志或错误中。引擎的错误只以操作、HTTP 状态和引擎错误的类型出现在日志中，因为引擎自己的消息可能引用帖子或查询。唯一的例外是索引创建被拒绝的情况，其消息会指出出错的设置。

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### 索引及其升级

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index)（默认为 `my-statuses`）是一个别名的名称。文档保存在服务器创建并让别名指向的索引中，该索引以别名加上服务器映射的版本和索引设置的哈希命名，例如 `my-statuses-v1-3f2a9c41b07e`。映射在其 `_meta` 中记录同样的两项（`my_mapping_version` 和 `my_settings_hash`）。每次写入都经过别名，别名不存在时写入失败，因此 OpenSearch 永远不会用猜测的映射自行创建索引。不要自己创建同名的索引：如果该名称是索引而不是别名，或者别名指向不是服务器创建的索引，服务器会拒绝启动。

启动时，服务器把别名背后的索引与自己的映射版本和设置进行比较。只要有一项不同（映射发生变化的版本，发行说明会写明；或编辑过的设置文件），它就会创建一个新索引并把别名移到新索引上。新索引起初是空的，因此：

1. 运行 `my search rebuild`。在它结束之前，搜索可以使用，但只能找到更改之后发布或重建的内容。
2. 重建结束后，删除旧索引（`DELETE /my-statuses-v1-…`）。服务器会保留旧索引，并记录一条指明它的警告。

用同一个设置文件启动服务器的每个进程：使用不同文件的进程会让别名来回移动。较旧版本的服务器在遇到较新映射的索引时，会原样使用该索引。

`my search clear`，以及以它开始的不带 `--keep` 的 `my search rebuild`，会用相同的映射和设置创建一个新的空索引，把别名移过去，并在一个请求中删除旧索引。

<!-- source: sizing-1 4ad4bbcd9a2b -->
### 规模

- **内存**：节点至少需要 512 MB 的 JVM 堆（`-Xms512m -Xmx512m`），操作系统还需要同样多的内存；小型服务器请分配 1 GB，并随索引增长而增加。堆最好保持在节点内存的一半。
- **磁盘**：使用内置设置时，索引大约占用被索引帖子文本的大小；对于不用空格分词的语言，文本按单个字符和字符对索引，会占用更多；使用 n-gram 时还会更多。请至少保留 15% 的可用磁盘空间：默认情况下，OpenSearch 在节点使用率超过 85% 时停止向其分配分片，超过 95% 时阻止写入其索引。
- **分片和副本**：没有设置文件时，索引使用集群的默认值，除非你更改过，否则是一个分片和一个副本。一个分片可以服务数百万条帖子。一个副本会让单节点集群处于黄色状态；这不影响使用，在设置文件中写入 `"index": {"number_of_replicas": 0}` 即可变为绿色。在多节点集群上，请保留副本。副本数也可以在运行中的索引上更改（`PUT /my-statuses/_settings`），无需重建；只要设置文件不变，服务器就会继续使用该索引。
- **负载**：每次搜索是对索引的一次查询，按帖子 ID 从新到旧排列。写入是每条帖子一个请求，重建的写入速度不会超过[其速率](search.md#rebuilding-the-index)。

<!-- source: japanese-and-other-languages a48cff99706b -->
### 日语和其他语言

使用内置设置时，每条帖子无论语言如何都以相同方式分析：文本在空格和标点处拆分为词，转为小写，全角拉丁字母和半角片假名被转换为其常规形式。没有空格的中文、日语和韩语文本被拆分为字符，除了每个字符之外，每对相邻字符也会被索引（OpenSearch 内置的 `cjk_bigram` 过滤器）。

- 查询会找到包含其中每个词的帖子。在日语中，两个或更多字符的词，会找到包含其每对字符的帖子：`東京` 能找到 `東京都の天気`，`全文検索` 能找到 `全文検索の本文です`。单个字符（例如 `本`）会找到包含该字符的所有帖子。
- 字符对不知道词在哪里结束，因此查询可能找到其字符只是碰巧相邻的帖子：`京都` 也能找到 `東京都`。用半角或全角书写的片假名查询能找到两者。
- 读音不匹配：`ぜんぶん` 找不到 `全文`；无论日语还是英语，屈折形式都不会与其原形匹配（`running` 找不到 `run`）。

要按词搜索日语，请在[设置文件](#tuning-the-analysis-of-opensearch)中使用 kuromoji 等形态分析器。

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### 调整 OpenSearch 的分析

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) 指定一个 JSON 文件，其对象会成为服务器创建的每个索引的 `settings`：`analysis`、`index.number_of_shards`、`index.number_of_replicas` 以及其他任何索引设置。它只包含设置；映射由服务器管理，包含 `mappings` 的文件会被拒绝。

文件必须定义映射所指定的两个分析器：

| 分析器 | 分析对象 |
| --- | --- |
| `my_text` | 每条帖子的文本：正文、内容警告和媒体的替代文本 |
| `my_text_search` | 每个查询的文本 |

两者可以采用相同的定义。每个查询都是对字段 `text` 的 `match` 查询，带有 `"operator": "and"`：查询文本经过 `my_text_search`，当帖子的文本包含得出的每个词项时就会被找到，结果按帖子 ID 从新到旧排列，不评分。分析器没有产生任何词项的查询什么也找不到。不使用短语、模糊和前缀匹配，因此完全由分析器决定什么算匹配。帖子的语言会被保存，但不会选择分析器：每条帖子都经过 `my_text`。

当服务器用一个与索引设置不同的文件启动时，即使只差一个副本，它也会创建一个新的空索引，索引需要重建（[索引及其升级](#the-index-and-its-upgrades)）。格式和键的顺序不算差异。OpenSearch 拒绝的文件（例如缺少这两个分析器，或使用了集群没有的插件）会让启动停止，并给出 OpenSearch 的原因。

Kuromoji 用词典把日语拆分为词，并按原形匹配屈折形式。Amazon OpenSearch Service 的每个域都有该插件；你自己运行的集群需要安装它（`opensearch-plugin install analysis-kuromoji`，例如在 `FROM opensearchproject/opensearch` 的 Dockerfile 中）。使用 kuromoji 的文件：

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

词典拆分方式与查询不同的词找不到，因此 kuromoji 可能漏掉内置字符对能找到的内容，例如复合名词的一部分或新词。OpenSearch 内置的 n-gram 分词器能找到任何连续的字符，代价是索引更大，并且会匹配更长词中的部分；使用 `"min_gram": 2` 和 `"max_gram": 3`（以及 `"index": {"max_ngram_diff": 1}`）时，单个字符的查询什么也找不到。

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` 把索引保存在 [Meilisearch](https://www.meilisearch.com/docs) 服务器中，可以自行托管，也可以使用 Meilisearch Cloud。它是单个进程，无需运行集群，适合小型服务器，官方镜像无需插件即可对日语、中文和韩语分词。服务器已在 Meilisearch 1.56 上验证过。

<!-- source: running-meilisearch 4f7d5e17c255 -->
### 运行 Meilisearch

使用至少 16 字节的主密钥运行官方镜像，放在只有 `my` 能访问的网络上，并把它的数据目录放在持久卷上：

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` 在没有主密钥时拒绝启动，并关闭内置的搜索预览。Meilisearch 使用明文 HTTP：当 `my` 不是通过本地或私有链路访问它时，请把它放在 TLS 后面，或者让它留在私有网络中。

<!-- source: settings 74547d71e962 -->
### 设置

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

通过环境变量设置密钥（`MY_SEARCH_MEILISEARCH_API_KEY` 或 `MY_SEARCH_MEILISEARCH_API_KEY_FILE`），绝不要写在配置文件中。它可以是主密钥，但只能做 `my` 所需操作的密钥更安全。用主密钥创建一个：

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

把响应中的 `key` 用作 `search_meilisearch_api_key`。密钥只在 `Authorization` 响应头中发送：服务器的日志和错误显示 Meilisearch 的错误代码（例如 `invalid_api_key`），绝不会显示密钥、帖子或查询。所有[设置项](../reference/configuration.md#search_meilisearch_url)都在配置参考中。

多台服务器可以共用一个 Meilisearch 服务器，各自使用不同的 `search_meilisearch_index`，以及只能访问自己索引的密钥。

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### 索引及其设置

索引不存在时，服务器会创建它。启动后第一次使用索引时，服务器会把将要发送的设置与上次应用的设置进行比较（上次的设置以哈希的形式记录在索引中一个任何搜索都找不到的文档里），只在两者不同时才发送：没有任何更改的重启不会发送任何内容。设置包括：

- **服务器自行设置的部分：** `searchableAttributes`（帖子的文本字段）、`displayedAttributes`（全部）、`filterableAttributes`（帖子 ID、作者和可见范围）和 `sortableAttributes`（帖子 ID）。搜索依赖于它们，设置了其中任何一项的设置文件会被拒绝。
- **你可以调整的部分：** 其他所有设置，写在[设置文件](#tuning-meilisearch)中。没有设置文件时，或对于文件中没有写的设置，服务器使用自己的默认值：`rankingRules` 为 `sort`、`words`、`typo`、`proximity`、`attribute` 和 `exactness`；不启用拼写容错；`localizedAttributes` 中包含日语、中文和韩语字段的语言；其余设置使用 Meilisearch 自己的默认值（`stopWords`、`synonyms`、`dictionary`、`separatorTokens`、`nonSeparatorTokens`、`proximityPrecision`、`pagination`、`faceting`、`distinctAttribute`、`searchCutoffMs`、`prefixSearch`）。你在引擎上手动更改的设置，只会保留到服务器下一次发送其设置为止。

**更改设置不需要运行 `my search rebuild`。** Meilisearch 会在后台自行重新索引它保存的文档，帖子始终留在索引中。这只是需要时间，大致与当初索引它们所用的时间相当。服务器启动时不会等待它：服务器发送设置后就继续运行。在此期间，搜索按之前的设置响应，而索引的写入会排在重新索引之后，因为 Meilisearch 按顺序执行一个索引的任务；写入未能在 10 秒内完成的任务会稍后重试，因此重新索引期间写入的帖子会在其完成后进入索引。`my search clear` 会等待新的空索引的设置完成，这几乎不需要时间。

只有在索引从空开始时才需要重建：

- 你更改了 `search_meilisearch_index`。
- 你启动了一个新的空 Meilisearch 服务器，例如没有升级旧服务器而是换了新的。
- 某个版本的 `my` 的发行说明写明它以不同方式写入文档。

写入是 Meilisearch 按顺序执行的任务。服务器会等待每次写入的任务，失败或未能在任务时限内结束的任务会让该任务重试，因此宕机或繁忙的 Meilisearch 只会延迟索引，不会丢失任何更改。失败的设置任务（例如因为 Meilisearch 拒绝了设置文件中的某个值）会让写入一直失败，直到它成功为止；在引擎上执行 `GET /tasks?types=settingsUpdate` 可以看到其错误。`my search clear` 会删除索引，并用这些设置重新创建它。

要升级 Meilisearch 本身，请按照其升级指南操作（使用 dump，或其无 dump 升级），或者以空状态启动新版本，然后运行 `my search rebuild`。

<!-- source: what-a-search-sends 575b89133606 -->
### 搜索发送的内容

无论设置文件如何写，搜索的查询都是固定的：`matchingStrategy: all`，使帖子必须包含每个词；`sort: status_id:desc`，从新到旧；用于下一页的基于帖子 ID 的 `filter`；页面大小加一的 `limit`；只取帖子 ID 的 `attributesToRetrieve`；以及 `showMatchesPosition`，用于判断查询中是否没有任何词。短语的双引号和词前的减号会被 Meilisearch 当作运算符，因此会被去掉，查询始终是普通的词。

<!-- source: tuning-meilisearch df827448e06c -->
### 调整 Meilisearch

用一个 [Meilisearch 索引设置](https://www.meilisearch.com/docs/reference/api/settings)的 JSON 文件调整引擎，由 `search_meilisearch_settings_file` 指定。当 `search = meilisearch` 时，服务器在启动时检查它：文件必须存在且是一个 JSON 对象，不得设置服务器自行设置的四项设置，并且如果有 `rankingRules`，它必须以 `sort` 开头，以保持结果从新到旧排列。Meilisearch 会在服务器发送这些值时检查它们。服务器绝不会记录该文件的内容。

下面的示例把语言限定为日语，把 `・` 视为分隔符，降低邻近度的索引成本，并为长词开启拼写容错：

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

更改的代价：

- 每次更改该文件，下一次启动时都会让 Meilisearch 重新索引所有文档，如上所述：不会丢失数据，也不需要重建，但写入要等到它结束。请在空闲时更改，并用 `GET /tasks?types=settingsUpdate` 关注该任务。
- `localizedAttributes` 和各种 token 设置决定文本如何拆分为词：更窄的语言列表可能会把另一种语言的帖子当作错误的语言来分词（见下文），因此除非服务器上没有日语、中文和韩语帖子，否则请保留这些字段。
- 拼写容错和同义词会让搜索找到比原词更多的内容。`sort` 之后的排序规则不会改变任何结果，因为没有两条帖子共用一个 ID；去掉它们只会让 Meilisearch 少做一点工作。

<!-- source: sizing-2 1d581fac5316 -->
### 规模

Meilisearch 把索引保存在磁盘上的内存映射文件中：它可以在内存小于索引的情况下运行，索引能放进内存的部分越多，速度越快。

- **内存。** 对小型服务器来说，1 GB 是合理的起点；随着索引增长再增加，理想情况下与索引在磁盘上的大小相同。默认情况下，索引最多使用机器内存的三分之二；在与其他服务共用的机器上，请用 `MEILI_MAX_INDEXING_MEMORY`（例如 `512Mb`）设置上限。
- **磁盘。** 索引比它所保存的文本大好几倍，因为它保存每个字段的词列表。在第一次重建后用 `GET /stats`（`databaseSize`）测量，并保持远多于它的可用空间：重新索引和升级都需要空间。
- **CPU。** 对小型服务器来说，一到两个核心就足够了。重建是最重的负载；可以用 `my search rebuild --rate` 降低速度。

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### 日语和其他语言

服务器把帖子的文本放入其语言对应的字段：日语（`ja`，包括 `ja-JP` 等标签）、中文（`zh`）、韩语（`ko`），或者用于其他所有语言和无语言的一个字段。它会把前三种的语言告诉 Meilisearch，Meilisearch 用词典把它们分别拆分为词：

- **日语**由 Lindera 使用 UniDic 词典分词，无论文本包含假名还是只有汉字。`検索`、`全文検索` 和 `全文　検索` 都能找到 `全文検索の本文です`，`東京` 能找到 `東京都庁舎見学`。
- **中文**由 jieba 分词，**韩语**由 Lindera 使用 ko-dic 词典分词。
- **其他语言**在空格和标点处拆分；Meilisearch 会自行检测文本的文字和语言。

查询以同样的方式拆分为词，当帖子包含查询的每个词时就会被找到，最后一个词还可以作为更长词的开头匹配（`テキ` 能找到 `テキスト`）。只有查询的前 10 个词有效。

搜索找不到的内容：

- **不属于词典分词结果的字符串。** 搜索查找的是词，而不是子字符串：`京都` 找不到 `東京都`，词典保留在更长词内部的部分（例如 `東京都庁舎見学` 中的 `庁舎`）也找不到。
- **读音。** 平假名能找到用片假名写的同一个词（`かたかな` 能找到 `カタカナ`），但读音找不到汉字（`ぜんぶん` 找不到 `全文`）。
- **相近的拼写。** 拼写容错已关闭，因此每个词都必须完全一致。
- **没有语言的日语帖子。** 语言缺失或错误的帖子，按 Meilisearch 检测到的语言分词，只有汉字的文本常常被当作中文并以不同方式拆分：这样的帖子可能会被 `京都` 当作 `東京都` 找到，也可能无法被 `東京` 找到。帖子带有其作者选择的语言，或作者的服务器发送的语言。

服务器在帖子和查询到达 Meilisearch 之前，会对 Unicode 的兼容形式进行规范化，因此半角片假名（`ｶﾀｶﾅ`）以及全角字母和数字（`ＡＢＣ`）与其常规形式可以互相找到。表情符号也是词：`👍` 能找到包含它的帖子。

这些都不需要搜索专用的插件、词典或镜像：官方镜像自带日语、中文和韩语的词典。
