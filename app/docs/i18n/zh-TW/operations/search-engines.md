<!-- source: search-engines 6816d50f42dd -->
# 搜尋引擎

本頁說明貼文全文搜尋可使用的各個搜尋引擎要如何執行、需要多大的規模，以及它們對日文和其他不以空格分詞的語言能找到什麼程度。對於這裡沒有列出的引擎，例如 Groonga 或以 Sudachi 建立的索引，可以改由你自己的[搜尋外掛](plugins.md#searching-posts)保存索引。不論使用哪個引擎，索引保存的內容、啟用搜尋、重建索引和切換引擎，請參閱[貼文全文搜尋](search.md)。

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` 會把索引保存在 8 版或更新版本的 Elasticsearch 叢集中，可以是自行管理的叢集，或 Elastic Cloud Hosted。已針對 Elasticsearch 9.5 和 8.19 進行過驗證。索引的欄位由伺服器負責，伺服器會依這些欄位寫入、篩選、排序和分頁。索引的設定則可以由你負責：分析、分片和副本。沒有你自己的設定時，內建設定不需要任何外掛，並會依[下文所述](#how-the-built-in-analysis-finds-japanese)的方式找到日文。

<!-- source: setting-it-up 3e02c35ff319 -->
### 設定方式

1. 執行 Elasticsearch。大多數伺服器使用單一節點就夠了。若要在同一台主機上快速開始：

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   在預設啟用安全功能的情況下，Elasticsearch 會以自己的憑證機構所簽發的憑證提供 HTTPS。請將該憑證機構從容器中複製出來，用於 `search_elasticsearch_ca_file`：

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. 建立一把只能管理伺服器索引的 API 金鑰。它只需要對別名及其背後的索引擁有 `manage`、`read` 和 `write`：

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   回應中的 `encoded` 值就是金鑰。

3. 設定伺服器的設定並重新啟動：

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. 如[啟用搜尋](search.md#turning-it-on)所述，執行 `my search rebuild` 加入既有的貼文。

設定項目：

| 設定 | 作用 |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | REST API 的 URL。只要請求會經過與他人共用的網路，就請使用 `https:`；私有網路或關閉安全功能的叢集可以使用 `http:`。 |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | API 金鑰，這是比較好的選擇。 |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username)、[`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | 以基本驗證代替金鑰時使用的使用者和密碼。 |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | 別名的名稱，預設為 `my-statuses`。共用同一個叢集的每個伺服器請使用各自的名稱，並授予其金鑰該名稱加上 `*` 的權限。 |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | 你自己的索引設定的 JSON 檔案，如[調整分析](#tuning-the-analysis-of-elasticsearch)所述。 |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | 系統不信任其憑證的 HTTPS 叢集的憑證機構。 |

憑證資訊只會從環境變數中以機密的形式讀取，永遠不會出現在伺服器的日誌或錯誤中。叢集的錯誤會以請求路徑、狀態和錯誤類型（例如 `index_not_found_exception`）出現在日誌中，絕不包含貼文或查詢的文字。只有在叢集拒絕建立索引時，錯誤才會引用其原因，而原因只會提到設定和欄位。

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### 索引及其升級

索引由伺服器自行建立和維護。請不要手動建立它，也不要讓其他任何東西寫入它。

- 啟動時，伺服器會尋找別名。沒有別名時，它會建立一個以別名、欄位版本和設定雜湊值命名的索引（例如 `my-statuses-v1-1f0c2e9a7b3d4c5e`），並讓別名指向它。版本和雜湊值保存在索引的 `_meta` 中。
- 當叢集無法連線、拒絕其金鑰或拒絕設定時，伺服器無法啟動。`my config check` 會檢查設定，以及設定檔是否為一個 JSON 物件，但不會檢查 Elasticsearch 是否接受它，因此請透過啟動伺服器來確認連線和設定。
- 如果有一個與別名同名、但不是由伺服器建立的索引（例如 Elasticsearch 自行建立的索引），啟動會以指出該索引名稱的錯誤停止。請刪除該索引，或選擇另一個 `search_elasticsearch_index`。
- 當設定與建立索引時的設定不同時（因為你編輯了設定檔，或新版的 `my` 變更了內建設定），伺服器會以新的設定建立一個新的空索引，讓別名指向它，刪除舊的索引，並記錄一則要求執行 `my search rebuild` 的警告。請執行它。在重建完成之前，搜尋只會找到之後寫入的貼文。當 Elasticsearch 拒絕新的設定時，伺服器不會啟動，舊的索引會保留。
- 當新版的 `my` 變更了欄位時，伺服器會在啟動時升級索引。只新增欄位的變更會就地進行，搜尋也會持續運作；當已在索引中的貼文需要 `my search rebuild --keep` 才能取得新欄位時，版本說明會註明。欄位的其他任何變更，都需要如上所述的新空索引和重建。版本說明會註明哪個版本會這樣做。
- 由較新版本的 `my` 建立的索引，會讓較舊的版本無法啟動。若要跨越這類變更回復到舊版，請在 Elasticsearch 中刪除別名的索引（用 `GET _alias/my-statuses` 找出它），啟動較舊的版本讓它建立自己的索引，然後執行 `my search rebuild`。
- `my search clear` 會在一個步驟中讓別名指向新的空索引並刪除舊的索引，這比逐一刪除文件更快，而且會釋放磁碟空間。

<!-- source: sizing 2b1d9b604953 -->
### 規模估算

索引只保存搜尋所需的內容，不保存文字的副本（`_source` 已關閉）。使用內建設定時，在 Elasticsearch 合併其區段後，短貼文每則約佔 80 到 150 位元組的磁碟空間，較長且用詞較廣的貼文則更多，因此請以每則貼文 150 到 300 位元組來規劃：一百萬則貼文約需數百 MB。請用 `GET _cat/indices/my-statuses*?v` 確認實際大小。

內建設定會讓索引有一個主分片（可容納數千萬則貼文），並在叢集有第二個節點時有一個副本（`auto_expand_replicas: 0-1`），因此單一節點的狀態會維持綠色。若要變更，請在設定檔中設定 `number_of_shards` 和 `number_of_replicas`。

擁有 1 GB Java heap（`ES_JAVA_OPTS=-Xms1g -Xmx1g`）、總共約 2 GB 記憶體的節點，足以服務小型或中型的伺服器。請把機器記憶體的一半作為 heap 給 Elasticsearch，其餘留給檔案快取，搜尋變慢時再增加。副本需要更多節點，而且每個副本都需要同樣大小的磁碟空間。

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### 查詢如何使用分析

每則貼文不論語言為何，都會被索引到同一個文字欄位，並套用相同的分析。貼文的語言不會選擇其他的分析器。

- 貼文的文字以名為 `my_text` 的分析器分析。
- 查詢會在空格（包括全形空格）處切分成詞。設定中定義了 `my_text_search` 時，每個詞以它分析，否則以 `my_text` 分析，而且必須以片語的形式出現在貼文中：其 token 依相同順序且彼此相鄰。每個詞都必須符合。查詢中的任何內容都不會被當作 Elasticsearch 的查詢語言解讀。
- 結果依由新到舊排序，而不是依相關性，因此評分選項不會造成任何差異。

所以，什麼是一個詞，是由分析器決定的。把每個字元都當作 token 的分析器，可以找到任何連續的字元。產生字典詞彙的分析器，則會找到完整的詞及其其他詞形。

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### 內建分析如何找到日文

內建的 `my_text` 是 Elasticsearch 的標準 tokenizer 加上 `cjk_width` 和 `lowercase` 篩選器，全部都是內建的。這個 tokenizer 會把每個漢字和每個平假名、每段連續的片假名，以及以空格分詞的語言中的每個詞，各自作為一個 token。

- 由漢字或平假名組成的詞，只要其字元相連出現就會被找到，即使是在較長的詞中：`検索` 和 `本` 會找到 `全文検索の本文です`，`東京 天気` 會找到 `東京都の天気は晴れ`。只分開出現的字元不會符合。
- 連續的片假名會整段比對：`カタカナ` 會找到 `カタカナとABC`，但 `カタ` 找不到它，`ツリー` 也找不到 `スカイツリー`。
- 全形拉丁字母和半形片假名會與其一般形式相符：`abc` 會找到 `ＡＢＣ`，`カタカナ` 會找到 `ｶﾀｶﾅ`。
- 平假名找不到讀音相同的漢字：`ぜんぶん` 找不到 `全文`。詞的其他詞形也找不到：`走る` 找不到 `走った`。
- 只有一個非常常見字元的查詢，例如 `の`，會找到非常多的貼文。

中文和韓文也以相同的方式依字元找到。以空格分詞的語言中的詞會整個比對，不做詞幹處理：`run` 找不到 `running`。

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### 調整 Elasticsearch 的分析

將索引的設定寫入 JSON 檔案，並在 `search_elasticsearch_index_settings_file` 中指定它。伺服器會用整個檔案代替內建設定，作為它建立的每個索引的 `settings`，因此檔案必須包含索引所需的一切：

- 文字欄位使用的分析器 `my_text`。沒有它的話，Elasticsearch 會拒絕建立索引，伺服器也不會啟動。
- 選擇性的分析器 `my_text_search`，有它時會用來分析查詢中的詞。
- 其他任何索引設定，例如 `number_of_shards` 和 `number_of_replicas`。檔案中省略的部分會使用叢集的預設值。
- 只能使用 Elasticsearch 具備的分析：檔案中指定的外掛必須安裝在每個節點上。

伺服器會在啟動時讀取這個檔案。編輯它會讓下次啟動時產生一個新的空索引（如[索引及其升級](#the-index-and-its-upgrades)所述），因此每次變更後都請執行 `my search rebuild`。請先在另一個叢集或另一個 `search_elasticsearch_index` 上試驗變更。

使用 `analysis-kuromoji` 外掛的字典詞彙，你需要在每個節點上安裝它（`bin/elasticsearch-plugin install analysis-kuromoji`），或使用 Elastic Cloud Hosted 提供的擴充功能：

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

只使用 Elasticsearch 內建的分析，對每段一或兩個字元的連續字元建立索引：

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

以下是在 Elasticsearch 9.5 上，以英文和日文各半的 20,000 則貼文測得的比較：

| | 內建 | Kuromoji | 一和兩個字元 |
| --- | --- | --- | --- |
| 外掛 | 無 | `analysis-kuromoji` | 無 |
| 索引大小 | 1.5 MB | 1.4 MB | 3.7 MB |
| 漢字詞的一部分：`東京都` 中的 `京都` | 找得到 | 找不到 | 找得到 |
| 片假名詞的一部分：`スカイツリー` 中的 `ツリー` | 找不到 | 找得到 | 找得到 |
| 片假名詞的一部分：`コンピューター` 中的 `コンピュータ` | 找不到 | 找不到 | 找得到 |
| 詞的其他詞形：`走る` 找到 `走った` | 找不到 | 找得到 | 找不到 |
| 英文單字的一部分：`running` 中的 `run` | 找不到 | 找不到 | 找得到 |

Kuromoji 會把日文切分成其字典中的詞，並將每個詞還原為基本形，因此可以找到任何詞形的詞，以及複合詞中字典認識的詞。它找不到字典視為一整個詞的詞中的一部分，而新詞或名稱如何切分則取決於字典。

一和兩個字元的連續字元可以找到任何語言中任何詞的任何部分，包括不是詞的部分：`run` 也會找到 `brunch`。索引大約大兩倍半，搜尋時也要讀取更多內容。

在這個小型索引上，三種設定的搜尋都只花了幾毫秒，差異會隨著索引的大小而擴大。

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` 會把索引保存在你執行的 OpenSearch 叢集，或 Amazon OpenSearch Service 的網域中。伺服器會與叢集的 REST API 溝通。索引的欄位和它送出的查詢由伺服器負責，其餘的部分可以由你負責：分析器、分片、副本和其他任何索引設定。沒有你自己的設定時，它會使用 OpenSearch 內建的分析器，不需要任何外掛。

<!-- source: setting-it-up-1 11097d66bccc -->
### 設定方式

1. 執行 OpenSearch 2 或 3。一開始使用一個節點就夠了；伺服器只會連到你提供的位址。
2. 將 [`search`](../reference/configuration.md#search) 設為 `opensearch`，並將 [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) 設為 REST API 的 URL，例如 `https://search.internal.example.org:9200`。路徑會保留，以便用於位於反向代理後方的叢集。
3. 提供伺服器存取的方式：
   - 使用安全外掛時，以 [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) 和 [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password) 指定使用者，以 HTTP Basic 驗證傳送。叢集不在同一台主機或你信任的私有網路上時，請使用 `https`。該使用者需要能建立名稱以 [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index) 開頭的索引、讀取其 mapping 並刪除它們，能管理該別名，並能讀取、寫入和刪除其中的文件。
   - 在使用 IAM 存取政策的 Amazon OpenSearch Service 上，將 [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) 設為 `es`，以 AWS Signature Version 4 簽署每個請求。憑證來自 AWS SDK 的預設憑證鏈（執行角色、環境變數、共用檔案），區域則來自 [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) 或 `AWS_REGION`。該角色需要對網域擁有 `es:ESHttpGet`、`es:ESHttpPost`、`es:ESHttpPut` 和 `es:ESHttpDelete`。使用精細存取控制和內部主要使用者的網域，則改用使用者和密碼。
   - 不支援 Amazon OpenSearch Serverless：它沒有 delete by query，而移除某位作者或某個可見範圍的貼文需要這項功能。
4. 可以選擇以設定檔[調整分析](#tuning-the-analysis-of-opensearch)。
5. 啟動伺服器。它會在啟動時建立索引，無法連到叢集或無法建立索引時會拒絕啟動。
6. 執行 `my search rebuild`，為既有的貼文建立索引（[重建索引](search.md#rebuilding-the-index)）。

URL、使用者、AWS 憑證和設定檔的內容，永遠不會出現在伺服器的日誌或錯誤中。引擎的錯誤只會以操作、HTTP 狀態和引擎錯誤的類型出現在日誌中，因為引擎自己的訊息可能會引用貼文或查詢。唯一的例外是索引建立被拒絕時，其訊息會指出有問題的設定。

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### 索引及其升級

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index)（預設為 `my-statuses`）指定的是別名。文件存放在伺服器建立並讓別名指向的索引中，其名稱由別名、伺服器 mapping 的版本和索引設定的雜湊值組成，例如 `my-statuses-v1-3f2a9c41b07e`。mapping 在其 `_meta` 中也記錄了這兩者（`my_mapping_version` 和 `my_settings_hash`）。每次寫入都會經過別名，別名不存在時寫入會失敗，因此 OpenSearch 永遠不會以猜測的 mapping 自行建立索引。請不要自行建立同名的索引：當該名稱是索引而不是別名，或別名指向不是由伺服器建立的索引時，伺服器會拒絕啟動。

啟動時，伺服器會將別名背後的索引與自己的 mapping 版本和設定比較。只要其中之一不同（mapping 有變更的版本，版本說明中會註明；或設定檔已被編輯），它就會建立一個新索引，並將別名移到新索引。新索引一開始是空的，因此：

1. 執行 `my search rebuild`。在它完成之前，搜尋可以運作，但只會找到變更之後發布或重建的內容。
2. 重建完成後，刪除舊的索引（`DELETE /my-statuses-v1-…`）。伺服器會保留它，並記錄一則指出其名稱的警告。

請以相同的設定檔啟動伺服器的每個程序：使用不同檔案的程序會讓別名來回移動。舊版的伺服器以較新 mapping 的索引啟動時，會直接使用該索引。

`my search clear`，以及以它開頭、不帶 `--keep` 的 `my search rebuild`，會在一個請求中以相同的 mapping 和設定建立新的空索引、將別名移到它，並刪除舊的索引。

<!-- source: sizing-1 4ad4bbcd9a2b -->
### 規模估算

- **記憶體**：節點需要至少 512 MB 的 JVM heap（`-Xms512m -Xmx512m`），另外還需要同樣大小給作業系統；小型伺服器請給它 1 GB，並隨索引成長增加。heap 最好維持在節點記憶體的一半。
- **磁碟**：使用內建設定時，索引大約佔用與已索引貼文文字相當的大小；不以空格分詞的語言會更多，因為其文字是以單一字元和字元配對建立索引；使用 n-gram 則還會更多。請保留至少 15% 的磁碟可用空間：預設情況下，OpenSearch 在節點超過 85% 時會停止在該節點上配置分片，超過 95% 時會封鎖對其索引的寫入。
- **分片與副本**：沒有設定檔時，索引會使用叢集的預設值，除非你變更過，否則是一個分片和一個副本。一個分片可以服務數百萬則貼文。一個副本會讓單一節點的叢集呈現黃色；它仍能運作，在設定檔中加上 `"index": {"number_of_replicas": 0}` 就會變成綠色。在多個節點的叢集上，請保留副本。副本數量也可以在運作中的索引上變更（`PUT /my-statuses/_settings`），不需要重建；只要設定檔沒有變更，伺服器就會繼續使用該索引。
- **負載**：每次搜尋都是對索引的一次查詢，依貼文 ID 由新到舊排序。寫入是每則貼文一個請求，重建的寫入速度不會超過[其速率](search.md#rebuilding-the-index)。

<!-- source: japanese-and-other-languages a48cff99706b -->
### 日文與其他語言

使用內建設定時，每則貼文不論語言為何都以相同方式分析：文字在空格和標點處切分成詞並轉為小寫，全形拉丁字母和半形片假名會轉為其一般形式。沒有空格的中文、日文和韓文文字會切分成字元，除了每個字元之外，每一對相鄰的字元也會建立索引（OpenSearch 內建的 `cjk_bigram` 篩選器）。

- 查詢會找到包含其中每個詞的貼文。在日文中，兩個字元以上的詞會找到包含其每一對字元的貼文：`東京` 會找到 `東京都の天気`，`全文検索` 會找到 `全文検索の本文です`。單一字元（例如 `本`）會找到所有包含該字元的貼文。
- 字元配對不知道詞的界線，因此查詢可能會找到其字元只是剛好相鄰的貼文：`京都` 也會找到 `東京都`。以半形或全形寫的片假名查詢，兩者都會找到。
- 不比對讀音：`ぜんぶん` 找不到 `全文`；無論日文或英文，活用形也不會與其辭書形比對（`running` 找不到 `run`）。

若要以詞為單位找到日文，請在[設定檔](#tuning-the-analysis-of-opensearch)中使用 kuromoji 等形態素分析器。

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### 調整 OpenSearch 的分析

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) 指定一個 JSON 檔案，其物件會成為伺服器建立的每個索引的 `settings`：`analysis`、`index.number_of_shards`、`index.number_of_replicas` 和其他任何索引設定。它只包含設定；mapping 由伺服器負責，含有 `mappings` 的檔案會被拒絕。

檔案必須定義 mapping 所指定的兩個分析器：

| 分析器 | 分析的對象 |
| --- | --- |
| `my_text` | 每則貼文的文字：內文、內容警告，以及媒體的替代文字 |
| `my_text_search` | 每個查詢的文字 |

兩者可以有相同的定義。每個查詢都是對欄位 `text` 帶有 `"operator": "and"` 的 `match` 查詢：查詢文字會經過 `my_text_search`，當貼文的文字包含所產生的每個詞項時就會被找到，依貼文 ID 由新到舊排序，不評分。分析器沒有產生任何詞項的查詢找不到任何東西。不使用片語、模糊和前綴比對，因此什麼算符合完全由分析器自行決定。貼文的語言會被儲存，但不會選擇分析器：每則貼文都會經過 `my_text`。

當伺服器以設定與索引不同的檔案啟動時（即使只差一個副本），它會建立新的空索引，索引需要重建（[索引及其升級](#the-index-and-its-upgrades)）。格式和鍵的順序不算在內。OpenSearch 拒絕的檔案（例如缺少這兩個分析器，或使用了叢集沒有的外掛）會讓啟動停止，並附上 OpenSearch 的原因。

Kuromoji 會以字典把日文切分成詞，並以辭書形比對活用形。Amazon OpenSearch Service 的每個網域都有這個外掛；你自己執行的叢集則需要安裝它（`opensearch-plugin install analysis-kuromoji`，例如在 `FROM opensearchproject/opensearch` 的 Dockerfile 中）。使用 kuromoji 的檔案：

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

字典切分方式與查詢不同的詞會找不到，因此 kuromoji 可能會漏掉內建字元配對能找到的內容，例如複合名詞的一部分或新詞。OpenSearch 內建的 n-gram tokenizer 能找到任何連續字元，代價是索引較大，以及會符合較長詞中的部分；使用 `"min_gram": 2` 和 `"max_gram": 3`（以及 `"index": {"max_ngram_diff": 1}`）時，單一字元的查詢找不到任何東西。

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` 會把索引保存在 [Meilisearch](https://www.meilisearch.com/docs) 伺服器中，可以自行架設，或使用 Meilisearch Cloud。它是單一程序，不需要執行叢集，適合小型伺服器，而且官方映像檔不需要外掛就能切分日文、中文和韓文。已針對 Meilisearch 1.56 進行過驗證。

<!-- source: running-meilisearch 4f7d5e17c255 -->
### 執行 Meilisearch

以至少 16 位元組的 master key 執行官方映像檔，放在只有 `my` 能連到的網路上，並將其資料目錄放在持久性 volume 上：

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` 在沒有 master key 時會拒絕啟動，並關閉內建的搜尋預覽。Meilisearch 使用純 HTTP：當 `my` 不是透過本機或私有連結連到它時，請把它放在 TLS 後方，或保持在私有網路中。

<!-- source: settings 74547d71e962 -->
### 設定

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

請透過環境變數（`MY_SEARCH_MEILISEARCH_API_KEY` 或 `MY_SEARCH_MEILISEARCH_API_KEY_FILE`）設定金鑰，絕不要寫在設定檔中。它可以是 master key，但只能做 `my` 所需操作的金鑰比較安全。請用 master key 建立一把：

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

將回應中的 `key` 用作 `search_meilisearch_api_key`。金鑰只會在 `Authorization` 標頭中傳送：伺服器的日誌和錯誤會顯示 Meilisearch 的錯誤代碼（例如 `invalid_api_key`），絕不會顯示金鑰、貼文或查詢。所有[設定](../reference/configuration.md#search_meilisearch_url)都列在設定參考中。

多個伺服器可以共用一個 Meilisearch 伺服器，各自使用不同的 `search_meilisearch_index`，以及只限於自己索引的金鑰。

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### 索引及其設定

索引不存在時，伺服器會建立它。啟動後第一次使用索引時，伺服器會比較它要傳送的設定與上次套用的設定（它以雜湊值記錄在索引中一份任何搜尋都找不到的文件裡），只有在不同時才傳送：沒有任何變更的重新啟動不會傳送任何東西。設定如下：

- **伺服器自行設定的項目**：`searchableAttributes`（貼文的文字欄位）、`displayedAttributes`（全部）、`filterableAttributes`（貼文 ID、作者和可見範圍），以及 `sortableAttributes`（貼文 ID）。搜尋依賴這些項目，設定其中任何一項的設定檔都會被拒絕。
- **你可以調整的項目**：其他所有項目，寫在[設定檔](#tuning-meilisearch)中。沒有設定檔，或對於檔案中省略的部分，伺服器會使用自己的預設值：`rankingRules` 為 `sort`、`words`、`typo`、`proximity`、`attribute` 和 `exactness`；不使用錯字容許；`localizedAttributes` 中為日文、中文和韓文欄位指定語言；其餘則使用 Meilisearch 自己的預設值（`stopWords`、`synonyms`、`dictionary`、`separatorTokens`、`nonSeparatorTokens`、`proximityPrecision`、`pagination`、`faceting`、`distinctAttribute`、`searchCutoffMs`、`prefixSearch`）。你在引擎上手動變更的設定，只會維持到伺服器下次傳送其設定為止。

**變更設定不需要執行 `my search rebuild`**。Meilisearch 會在背景自行為它保存的文件重新建立索引，貼文會留在索引中。這只是需要時間，大約與當初建立索引所花的時間相當。伺服器的啟動不會等待它：伺服器傳送設定後就繼續進行。在這段期間，搜尋會以先前的設定回應，而索引的寫入會排在重新索引之後等待，因為 Meilisearch 會依序執行一個索引的任務；寫入未能在 10 秒內完成的任務會稍後重試，因此在重新索引期間寫入的貼文會在重新索引之後進入索引。`my search clear` 會等待新的空索引的設定，這不需要任何時間。

只有在索引從空的開始時才需要重建：

- 你變更了 `search_meilisearch_index`。
- 你啟動了一個新的空 Meilisearch 伺服器，例如不升級舊的伺服器而改用新的。
- 某個 `my` 版本的版本說明表示它以不同方式寫入文件。

寫入是 Meilisearch 依序執行的任務。伺服器會等待每次寫入的任務，失敗或未在任務時間內結束的任務會讓任務重試，因此 Meilisearch 停止運作或忙碌時，只會延遲索引而不會遺失變更。失敗的設定任務（例如因為 Meilisearch 拒絕設定檔中的某個值）會讓寫入失敗，直到它成功為止；在引擎上執行 `GET /tasks?types=settingsUpdate` 可以看到其錯誤。`my search clear` 會刪除索引，並以設定重新建立它。

若要升級 Meilisearch 本身，請依照它的升級指南（dump，或其 dumpless upgrade），或以空的狀態啟動新版本並執行 `my search rebuild`。

<!-- source: what-a-search-sends 575b89133606 -->
### 搜尋送出的內容

不論設定檔怎麼寫，搜尋的查詢都是固定的：`matchingStrategy: all`，讓貼文必須包含每個詞；`sort: status_id:desc`，由新到舊；用於下一頁的貼文 ID `filter`；頁面大小加一的 `limit`；貼文 ID 的 `attributesToRetrieve`；以及 `showMatchesPosition`，用來判斷沒有詞的查詢。Meilisearch 會當作運算子解讀的片語雙引號和詞前面的減號會被移除，因此查詢永遠是單純的詞。

<!-- source: tuning-meilisearch df827448e06c -->
### 調整 Meilisearch

以 `search_meilisearch_settings_file` 指定一個 [Meilisearch 索引設定](https://www.meilisearch.com/docs/reference/api/settings)的 JSON 檔案來調整引擎。`search = meilisearch` 時，伺服器會在啟動時檢查它：它必須存在且是一個 JSON 物件，不能設定伺服器自行設定的四個項目，而如果有 `rankingRules`，必須以 `sort` 開頭，以維持由新到舊的結果。Meilisearch 會在伺服器傳送時檢查這些值。伺服器永遠不會記錄檔案的內容。

以下範例把語言限縮為日文、將 `・` 視為分隔符號、降低建立鄰近度索引的成本，並對長詞開啟錯字容許：

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

變更的代價：

- 對檔案的每次變更，都會在下次啟動時讓 Meilisearch 如上所述重新為每份文件建立索引：不會遺失資料，也不需要重建，但寫入會等到它結束。請在流量少的時候變更，並用 `GET /tasks?types=settingsUpdate` 監看任務。
- `localizedAttributes` 和 token 決定文字如何切分成詞：較窄的語系清單可能會把另一種語言的貼文當成錯誤的語言切分（見下文），因此除非伺服器上沒有日文、中文和韓文的貼文，否則請保留這些欄位。
- 錯字容許和同義詞會讓搜尋找到比完全相同的詞更多的結果。`sort` 之後的排序規則不會造成任何改變，因為沒有兩則貼文會有相同的 ID；省略它們只是讓 Meilisearch 少做一點工作。

<!-- source: sizing-2 1d581fac5316 -->
### 規模估算

Meilisearch 把索引保存在磁碟上的記憶體對應檔案中：它可以在記憶體比索引小的情況下執行，而索引能放進記憶體的部分越多，速度就越快。

- **記憶體**。小型伺服器從 1 GB 開始是合理的；隨著索引成長再增加，理想上與索引在磁碟上的大小相當。建立索引時預設最多使用機器記憶體的三分之二；在與其他服務共用的機器上，請用 `MEILI_MAX_INDEXING_MEMORY`（例如 `512Mb`）設定上限。
- **磁碟**。索引比它保存的文字大好幾倍，因為它保存了每個欄位的詞彙清單。請在第一次重建後用 `GET /stats`（`databaseSize`）測量，並保留遠大於它的可用空間：重新索引和升級都需要作業空間。
- **CPU**。小型伺服器使用一或兩個核心就夠了。重建是最重的負載；請用 `my search rebuild --rate` 放慢它。

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### 日文與其他語言

伺服器會把貼文的文字放在其語言的欄位中：日文（`ja`，包括 `ja-JP` 等標籤）、中文（`zh`）、韓文（`ko`），或是給其他所有語言和沒有語言的貼文共用的一個欄位。它會告訴 Meilisearch 前三者的語言，Meilisearch 再以字典把每一種切分成詞：

- **日文**由 Lindera 搭配 UniDic 字典切分，不論文字含有假名或只有漢字。`検索`、`全文検索` 和 `全文　検索` 都會找到 `全文検索の本文です`，`東京` 會找到 `東京都庁舎見学`。
- **中文**由 jieba 切分，**韓文**由 Lindera 搭配 ko-dic 字典切分。
- **其他語言**在空格和標點處切分；Meilisearch 會自行偵測文字的書寫系統和語言。

查詢也以相同方式切分成詞，當貼文包含查詢中的每個詞時就會被找到，最後一個詞也可以是較長詞的開頭（`テキ` 會找到 `テキスト`）。查詢只有前 10 個詞會被計算。

搜尋找不到的內容：

- **不是字典切分出的詞的字串**。搜尋找的是詞，而不是子字串：`京都` 找不到 `東京都`，而字典保留在較長詞中的部分，例如 `東京都庁舎見学` 中的 `庁舎`，也找不到。
- **讀音**。平假名會找到以片假名書寫的同一個詞（`かたかな` 會找到 `カタカナ`），但讀音找不到漢字（`ぜんぶん` 找不到 `全文`）。
- **相近的拼寫**。錯字容許是關閉的，因此每個詞都必須完全相同。
- **沒有語言的日文貼文**。語言不存在或錯誤的貼文會依 Meilisearch 偵測的結果切分，只有漢字的文字常常會被當成中文而以不同方式切分：這樣的貼文可能會被 `京都` 找到（原文為 `東京都`），或無法被 `東京` 找到。貼文帶有的語言，是作者選擇的語言，或作者的伺服器所傳送的語言。

伺服器會在貼文和查詢送到 Meilisearch 之前，將 Unicode 的相容形式正規化，因此半形片假名（`ｶﾀｶﾅ`）以及全形字母和數字（`ＡＢＣ`）可以找到其一般形式，也能被其一般形式找到。表情符號也是詞：`👍` 會找到含有它的貼文。

這一切都不需要搜尋專用的外掛、字典或映像檔：官方映像檔已內含日文、中文和韓文的字典。
