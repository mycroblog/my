<!-- source: full-text-search-of-posts fc16f55b56ea -->
# 貼文全文搜尋

本頁說明貼文全文搜尋保存了哪些內容、如何開啟與變更它、如何重建索引，以及它對你的日誌和備份有什麼要求。

<!-- source: choosing-an-engine 9cfd51361e0a -->
## 選擇搜尋引擎

貼文全文搜尋需要一個由你自己執行或租用的搜尋引擎。由 [`search`](../reference/configuration.md#search) 選擇：

- `none`（預設）：不提供全文搜尋。伺服器不連線到任何引擎、不排入任何搜尋任務，用戶端也不會顯示搜尋貼文的方式。在這樣的伺服器上，每個 `my search` 指令都會說明這一點，且不做任何事。
- `elasticsearch`、`opensearch` 或 `meilisearch`：使用該引擎，並套用[搜尋引擎](search-engines.md)中對應區段的設定。
- `plugin`：由你自己的程式在 `plugin_search_address` 把索引保存在其他任何引擎中，例如 Groonga 或以 Sudachi 建立的索引（[以外掛搜尋貼文](plugins.md#searching-posts)）。

帳號與主題標籤的搜尋是透過資料庫進行，與這項設定無關。

伺服器只負責它寫入、篩選、排序和分頁所用的欄位。引擎如何分析文字、如何排序結果、索引有多大，都由你透過引擎的設定檔來調整；[搜尋引擎](search-engines.md)說明了各引擎在內建設定下能搜尋到哪些日文內容，以及如何變更。

<!-- source: what-the-index-holds 5e0884482ea8 -->
## 索引保存的內容

資料庫始終是真實資料的來源，索引只是可以丟棄並重建的副本。引擎只回傳貼文 ID：伺服器會從資料庫重新讀取每則貼文，並套用一般的可見範圍規則，因此落後的索引可能讓某則貼文搜尋不到，但不會顯示不該被看到的貼文。

符合以下所有條件的貼文才會被收入索引：

- 未被刪除。
- 可見範圍是 `public` 或 `local`，且包含在 [`search_visibilities`](instance-settings.md) 中。不論設定如何，`unlisted`、`private` 和 `direct` 的貼文都不會被收入索引。
- 作者已開啟 `indexable`，這項設定的說明請參閱[搜尋引擎與 AI 爬蟲](crawlers.md)。
- 作者及作者的網域，沒有受到使其無法被探索的限制。

索引保存每則貼文目前版本的內文、內容警告和替代文字，以及作者與語言。不保存投票、個人檔案或舊版本。

`indexable_default` 預設為 `false`，因此剛開啟搜尋的伺服器，在使用者自行選擇 `indexable` 之前，能搜尋到的內容很少。

<!-- source: turning-it-on 3d88170680a2 -->
## 開啟搜尋

1. 執行搜尋引擎，並設定 `search` 及其後端的設定。
2. 如果你將 worker 分開執行，請決定[它們的配置](#keeping-search-from-delaying-other-jobs)。
3. 設定[你的日誌](#the-query-appears-in-urls)。
4. 如果引擎由其他人營運，請把它加進你的[隱私權文件](#your-privacy-documents)。
5. 重新啟動伺服器，此後新的貼文就會進入索引。
6. 執行 `my search rebuild` 以加入既有的貼文。

重建期間搜尋仍可使用，但在重建結束前，結果會不完整。

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## 重建索引

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

指令會在重建開始後立即返回，其餘任務由 worker 以 `search.rebuild` 任務執行（[任務參考文件](../reference/jobs.md#full-text-search)），因此請保持 worker 執行中。每個任務處理 100 則貼文。

- **速率**。預設為每秒 20 則貼文，最多 1000 則，而且會計入所有檢查過的 `public` 和 `local` 貼文，包括不符合條件的。以預設速率處理一百萬則貼文約需 14 小時。這是為了給其他任務和資料庫留下餘裕，因此只有在伺服器資源有餘時才調高。
- **該執行哪一種**。預設模式會移除所有不該在索引中的內容：例如刪除通知沒有送達而留下的文件，或從備份還原資料庫後留下的文件。在重建結束前，能搜尋到的內容會變少。在[新增可見範圍](#changing-search_visibilities)之後，沒有需要移除的內容、且希望搜尋保持完整時，請使用 `--keep`。`--keep` 永遠不會移除已不在資料庫中的貼文所對應的文件。
- **中斷了**？再執行一次相同的指令。它會從最後處理的貼文接著進行，不會再清空索引。加上 `--rate` 可以變更速率。worker 重新啟動時，重建也會自動繼續。
- **另一種模式**。在重建尚未完成時，另一種模式的指令會被拒絕，因為它無法做到它所說的事。若要以任一模式重新開始，請先執行 `my search clear`，再執行你要的指令。
- **執行兩次**會得到相同的索引。重建期間建立、編輯或刪除的貼文最後都會是正確的，因為它們各自的任務會持續執行，而每個任務都會讀取貼文在當下的狀態。

`my search status` 會顯示後端、`search_visibilities`，以及最近一次重建的階段（`clearing`、`filling` 或 `finished`）、已處理和失敗的貼文數、速率和最後一個錯誤。

發生問題時：

- **引擎停止運作**。如果連續三則貼文失敗，任務會在當下停止、記錄錯誤並重試。用完重試次數的重建會留下一個失敗的任務（`my job list`），狀態中的 `updated` 也會停止變動。引擎恢復後，執行相同的指令即可繼續。
- **引擎拒絕某則貼文**。該貼文會被記錄為失敗，重建繼續進行。狀態最多列出 20 則。排除原因後，執行 `my search rebuild --keep` 以重新嘗試所有貼文。

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## 變更 `search_visibilities`

移除某個可見範圍，會自動把該範圍的貼文從索引中移除。新增可見範圍時，在你執行 `my search rebuild --keep` 之前，只能搜尋到新的貼文。

**新增 `local` 之前，請先告知你的使用者**。這樣一來，`indexable` 會涵蓋所有已開啟它的人的 `local` 貼文，重建也會加入他們先前的 `local` 貼文，而不會再次徵求他們的同意。個人檔案設定會說明 `indexable` 在你的伺服器上涵蓋哪些內容，但很久以前就選擇了它的人，除非你告訴他們，否則不會注意到。

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## 關閉或更換搜尋引擎

當伺服器改為 `search = none` 或換成其他引擎時，就會失去與舊索引的連線。舊索引不再被清理，因此之後刪除的貼文（包括其文字）會一直留在裡面。

1. 停止伺服器及其 worker，確保下一步之後沒有任何東西寫入索引。
2. 執行 `my search clear`。
3. 變更 `search`，並啟動伺服器。
4. 若換了新的引擎，執行 `my search rebuild`。

如果你已經切換過了，或無法停止伺服器，請在引擎本身刪除舊索引。設定變更後，`my search clear` 就無法觸及它。改為 `none` 之後，變更前已排入的任務不會做任何事。

**從 `none` 切換回來時**。請不加 `--keep` 執行 `my search rebuild`：伺服器在沒有引擎期間，不會記錄哪些內容被刪除，只有清空索引才能移除那些貼文。如果離開時有尚未完成的重建，請先執行 `my search clear`，因為接續進行的重建不會清空索引。

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## 避免搜尋拖慢其他任務

預設情況下，worker 一次只執行一個任務，因此即使每次呼叫引擎都有 10 秒的上限，緩慢的引擎仍會與 `federation.inbox` 共用該 worker 的時間。忙碌的伺服器應該將它們分開：

- 以 `worker_job_types = search.*` 執行一個 worker 處理程序，其他的則以 `worker_job_types_except = search.*` 執行（[依任務類型分開 worker](running.md#splitting-workers-by-job-type)）。
- 使用 `queue = cloudtasks` 時，用[佇列路由](running.md#routing-job-types-to-their-own-queues)把 `search.*` 類型送到專屬的佇列。

小型伺服器可以讓它們保持在一起，這時搜尋任務會與聯邦共用時間。

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## 查詢字串會出現在 URL 中

`GET /api/v1/statuses/search` 會把使用者搜尋的內容以 `?q=` 放在 URL 中。伺服器自己的 `http request` 日誌行只記錄路徑和路由，不記錄查詢字串，追蹤（trace）和錯誤回報也會將它排除。不過，位於伺服器前方的元件可能會記錄它，而搜尋內容可能透露一個人的許多事。請檢查你架構中的每一層，不是讓查詢字串不進入它的日誌，就是把該日誌視為個人資料，並相應地管理存取權限和保存期限。

| 層 | 會記錄查詢字串嗎？ | 該怎麼做 |
| --- | --- | --- |
| Cloud Run | 會。它的請求日誌會自動產生，且 `httpRequest.requestUrl` 包含查詢字串。 | 依下方說明排除它們。 |
| Google Cloud 負載平衡器 | 只有在你為後端服務開啟日誌記錄時才會。新建立的後端服務預設為關閉，且使用相同的欄位。 | 保持關閉，或依下方說明排除它們。 |
| AWS Application Load Balancer | 只有在你開啟存取日誌時才會，預設為關閉。請求行保存用戶端送出的原始 URL。它無法捨棄一行中的部分內容。 | 保持關閉，或為它們設定較短的生命週期規則並限制存取。 |
| CloudFront | 只有在你開啟標準日誌記錄時才會，記錄在 `cs-uri-query` 欄位中。 | 使用標準日誌記錄 v2 時，在選擇欄位時排除該欄位。 |
| ECS | 沒有自己的請求日誌：它的日誌保存的是伺服器所寫入的內容。 | 不需要處理。 |
| Cloudflare | 只有在你使用 Logpush 時才會，記錄在 `ClientRequestURI` 欄位中。 | 改為選擇 `ClientRequestPath`。 |
| Caddy | 只有在使用 `log` 指令時才會，而 `compose.caddy.yaml` 沒有使用它。 | 如果你加入了它，請依下方說明移除該參數。 |
| 其他反向代理 | 通常會：它們的預設格式會記錄整個請求行。 | 為這個路由只記錄不含查詢字串的路徑，或關閉它的存取日誌。 |

在 Google Cloud 上，以下設定會讓這兩種日誌都不進入專案的 `_Default` 儲存區。也請在你自己建立的每個接收器（sink）加入相同的排除條件：

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

在 Caddy 中：

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## 備份

索引保存了貼文的文字，因此需要與資料庫同樣小心對待：誰能存取它、保存在哪裡、保存多久。

需要特別留意的是引擎的快照。從索引中刪除的貼文，會留在刪除前拍攝的每個快照中，就像資料庫備份一樣，因此請為快照設定與[資料庫備份](backup.md)相同的保存期限。

復原並不需要快照：`my search rebuild` 會從資料庫重新建立索引。從備份還原資料庫之後，請不加 `--keep` 執行它，讓比備份更新的貼文離開索引。

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## 你的隱私權文件

如果引擎執行在其他公司的服務上，該公司會收到 `search_visibilities` 涵蓋的貼文文字（如果你把 `local` 包含在內，也包括這類貼文）。請在[隱私權政策](privacy-policy-template.md)和[處理活動紀錄](processing-record-template.md)中將它列為接收者。由你自己執行的引擎則不算是新的接收者。

如果你保留了[含有查詢字串的日誌](#the-query-appears-in-urls)，也請說明你對它們做了什麼決定。

<!-- source: closing-the-service 343ec69df255 -->
## 結束服務

伺服器在結束服務時不會自行刪除資料，因此請將索引和引擎的快照，連同資料庫和媒體一起銷毀（[結束服務](service-closure.md#7-discarding-the-db-blobs-and-backups)）。
