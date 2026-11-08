<!-- source: plugins 4a60f7abbec9 -->
# 外掛

外掛是由你執行的獨立程式或服務，伺服器會透過網路把一項範圍明確的任務交給它。伺服器只會連線到一個位址：它不會啟動、停止或監看外掛。請以 sidecar、在 systemd 底下，或在另一台機器上執行它。

預設不會委派任何任務。沒有外掛位址的伺服器不會開啟任何連線，也不會啟動任何程序。建置標籤 [`no_plugins`](../reference/build-tags.md) 會把外掛用戶端從執行檔中移除。

![外掛的位置：my 程序把六個 port 交給外掛，並詢問五個決策 hook，全都透過一個連線到你所執行外掛的外掛用戶端；圖的下方顯示呼叫失敗時各自的處理方式。下方清單說明每個部分。](images/plugins.svg)

由上而下說明這張圖：

- **my 程序**不論 `role` 為何，都會對外掛進行兩種呼叫。
  - **交出的 port**：外掛代替內建的部分執行一項任務，只有在設定指定 `plugin` 時才會啟用：`translator`（實例設定 `translation_providers = plugin`）、`billing`（`billing_sources` 包含 `plugin`）、`mailer`（`mail = plugin`）、`media_image`、`media_video` 和 `media_audio`（`media_image = plugin` 等）、`key_manager`（`key_manager = plugin`），以及 `search`（`search = plugin`）。
  - **[決策 hook](decision-hooks.md)**：外掛回答是否接受某件事，除非 hook 的模式 `hook_<hook>_mode` 為 `off`，否則都會詢問它：`inbound.status` 用於其他伺服器的貼文，`inbound.follow` 用於其他伺服器的追蹤，`signup.check` 用於註冊，`media.check` 用於上傳，`local.status` 用於本站帳號的貼文，`inbound.actor` 用於其他伺服器上帳號的個人檔案，`local.profile` 則用於本站帳號的個人檔案變更。
- **外掛用戶端**負責所有呼叫。每個種類有一個目的地 `plugin_<kind>_address` 和一個期限 `plugin_<kind>_timeout`，同時最多執行 8 個呼叫（每個媒體種類為 2 個）。它透過 Unix socket、`https://`，或者（僅限[部分種類](#plain-tcp)）`http://` 連線。
- **你的外掛**是你執行的獨立程式，可以是 sidecar、在 systemd 底下，或在另一台機器上。伺服器只會連線到它們。
- **呼叫失敗或逾時時**（[詳細說明](#when-a-call-fails)）：
  - `billing`、`mailer` 和媒體種類：發出呼叫的任務稍後會再次執行。
  - `translator`：伺服器會詢問下一個供應者，沒有剩下的供應者時，讀者會看到翻譯失敗。
  - `key_manager` 採取失效封閉（fail closed）：需要金鑰的操作都會失敗，沒有外掛時伺服器不會啟動。
  - `search`：寫入的任務稍後會再次執行，而搜尋會失敗，使用者會看到搜尋無法使用。
  - 決策 hook 會依照 `hook_<hook>_on_failure` 的設定處理：`pass`、`retry`，或者（註冊時）`require_approval`。

<!-- source: kinds 73da92ef7563 -->
## 種類

每個種類在 `plugin_<kind>_*` 設定下都有自己的目的地。種類名稱中的點在鍵中會變成 `_`，因此 `inbound.status` 的設定是 `plugin_inbound_status_address`。

| 種類 | 傳送的內容 | 純 TCP（`http://`） | 預設協定 | 預設期限 |
| --- | --- | --- | --- | --- |
| `translator` | 讀者要求翻譯的貼文文字 | **拒絕** | `json` | 10 秒 |
| `search` | `public` 和 `local` 貼文的文字、作者、可見範圍和語言，以及已登入使用者的查詢 | **拒絕** | `json` | 10 秒 |
| `billing` | 實例的費用 | 允許 | `json` | 2 分鐘 |
| `mailer` | 地址和確認連結等機密 | **拒絕** | `json` | 30 秒 |
| `media_image` | 上傳的圖片及其中繼資料（可能包含位置） | 允許 | `proto` | 4 分鐘 |
| `media_video` | 上傳的影片及其中繼資料（可能包含位置） | 允許 | `proto` | 4 分鐘 |
| `media_audio` | 上傳的音訊檔案及其中繼資料 | 允許 | `proto` | 4 分鐘 |
| `key_manager` | 明文的資料加密金鑰 | **拒絕** | `json` | 5 秒 |
| `inbound.status` | 傳入貼文的文字 | **拒絕** | `json` | 3 秒 |
| `local.status` | 本站貼文的文字 | **拒絕** | `json` | 1 秒 |
| `local.profile` | 本站帳號的公開個人檔案，不含圖片 | 允許 | `json` | 1 秒 |
| `inbound.follow` | 追蹤關係的雙方 | 允許 | `json` | 3 秒 |
| `inbound.actor` | 其他伺服器上帳號的公開個人檔案 | 允許 | `json` | 3 秒 |
| `signup.check` | 電子郵件地址和 IP 位址 | **拒絕** | `json` | 2 秒 |
| `media.check` | 處理過的小尺寸媒體版本，絕不是原始檔 | 允許 | `json` | 10 秒 |

讓伺服器使用某個目的地的設定屬於各自的功能：表格的最後五個種類是[決策 hook](decision-hooks.md)。本頁說明它們共通的部分。

伺服器每則訊息最多傳送該種類固定的上限，並最多讀取另一個上限。超過上限的呼叫會失敗，由發出呼叫的任務重試。

<!-- source: the-destination f397f6e80550 -->
## 目的地

這些設定只存在於設定檔和環境變數中（[設定參考](../reference/configuration.md)）。沒有任何實例設定、管理頁面或 API 能顯示或變更它們，因此即使管理員帳號被盜，也無法改變貼文或金鑰的去向。變更會在重新啟動後生效。

`plugin_<kind>_address` 有三種形式。伺服器在啟動時檢查形式而不解析名稱，並在每次連線時解析名稱。

- `unix:///run/my/mailer.sock`：Unix socket。
- `https://mailer.internal:8443`：透過 TCP 的 TLS。伺服器一律依據 `plugin_<kind>_tls_ca_file` 或系統的根憑證驗證憑證，沒有任何設定可以關閉這項驗證。
- `http://billing.internal:5000`：純 TCP。

<!-- source: authentication 2fe4df9369ac -->
### 驗證

選擇其中一種：

- **Unix socket**：由 socket 檔案的權限保護。
- **mTLS**：`plugin_<kind>_tls_cert_file` 和 `plugin_<kind>_tls_key_file`，搭配 `https://` 位址。
- **Bearer 權杖**：`plugin_<kind>_token`（或 `MY_PLUGIN_<KIND>_TOKEN_FILE`），以 `Authorization: Bearer` 傳送，只能透過 `https://` 或 Unix socket。權杖搭配 `http://` 位址屬於設定錯誤。

既沒有權杖也沒有用戶端憑證的 `https://` 位址會在啟動時發出警告，因為外掛無法分辨伺服器和其他能連到它的人。

伺服器會在啟動時讀取 `plugin_<kind>_tls_ca_file`、`plugin_<kind>_tls_cert_file` 和 `plugin_<kind>_tls_key_file` 的檔案，檔案不存在或無法讀取時會停止啟動。之後在開啟新連線時，最多每 5 秒檢查一次檔案是否有變更，因此由 Vault、cert-manager 或 SPIFFE 等憑證管理工具就地更新的短期憑證或 CA，不需要重新啟動就能使用。已經開啟的連線會繼續使用開始時的憑證，直到連線關閉。在更新只寫了一半的期間（例如新憑證的金鑰還沒放好），伺服器會繼續使用上次讀取的檔案，並在下一次連線時再試一次。權杖只在啟動時讀取。

<!-- source: plain-tcp cbb88078173e -->
### 純 TCP

`mailer`、`key_manager`、`translator`、`search`、`inbound.status`、`local.status` 和 `signup.check` 會拒絕 `http://`：伺服器不會啟動。loopback（`http://127.0.0.1`）也一樣，因為 loopback 位址無法阻止主機上的其他程序讀取流量。請把這類 sidecar 放在共用 volume 中的 Unix socket 上。

其他種類接受 `http://`，伺服器會在啟動時記錄一則指出種類和位址的警告。這些流量沒有任何加密或驗證，也無法透過它使用 bearer 權杖。保持該網路封閉是你的責任：上表列出的內容會以明文通過它。

<!-- source: protocols eb5a03718f1b -->
## 協定

`plugin_<kind>_protocol` 指定伺服器傳送的內容，它必須是外掛所接受的協定。

| 值 | 伺服器傳送 | 適用於 |
| --- | --- | --- |
| `json` | 使用 `application/json` 的 Connect unary 協定，在純連線上透過 HTTP/1.1 | 以 Connect 函式庫建立的外掛，或只用 HTTP 伺服器和 JSON 函式庫建立的外掛 |
| `proto` | 使用 `application/proto` 的 Connect 協定 | 以 Connect 函式庫建立的外掛 |
| `grpc` | 透過 HTTP/2 的 gRPC | 只支援 gRPC 的外掛 |

外掛對錯誤的協定會回應 `415`。媒體種類是雙向串流，需要 HTTP/2，因此它們使用 `proto` 或 `grpc`，`json` 屬於設定錯誤。

協定的定義是儲存庫 `api/plugin/` 底下的 `.proto` 檔案。每種 port 各有一個套件，彼此不共用型別；決策 hook 全都共用一個套件 `my.plugin.hook.v1`。[外掛協定](../plugins/overview.md)收錄了從這些檔案產生的每個套件的參考文件。

<!-- source: when-a-call-fails 1caa41ab429e -->
## 呼叫失敗時

呼叫會在同一種類同時執行的呼叫之間等待輪到自己，等待時間會計入它的期限和指標 [`my.plugin.slot_wait`](running.md)。失敗的呼叫會依種類和 Connect 代碼計入指標 [`my.plugin.failures`](running.md)。伺服器會捨棄外掛放在錯誤中的訊息，因此外掛失敗的原因要看外掛自己的日誌。伺服器本身不會重試呼叫：重試由發出呼叫的任務負責，因此外掛必須能處理同一個呼叫來兩次的情況。

伺服器啟動時外掛停止運作，不會讓任何東西停下來，`key_manager` 除外：沒有它伺服器不會啟動，而[沒有它時會停止的功能](key-management.md#when-the-plugin-is-unreachable)是設定它之前要先閱讀的部分。

呼叫的期限是 `plugin_<kind>_timeout`，外掛會以 `Connect-Timeout-Ms`（gRPC 為 `grpc-timeout`）收到它。外掛可以忽略這個標頭：伺服器無論如何都會停止等待。對於請求可能需要等待的種類（`key_manager`、`translator`、`search`、`signup.check`、`media.check`、`local.status`、`inbound.actor` 和 `local.profile`），期限必須少於 30 秒；其他種類則必須少於 5 分鐘，也就是任務可以執行的時間。

<!-- source: test-a-plugin 5d8195bb27f5 -->
## 測試外掛

`my plugin test` 會對執行中的外掛執行某個種類的契約，並回報每個案例。它不讀取任何設定，因此只需要 `my` 執行檔。

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

有案例失敗時，結束代碼為 1。旗標的意義與設定相同：`--protocol`、`--token-file`、`--tls-ca-file`、`--tls-cert-file`、`--tls-key-file` 和 `--timeout`。它套用與伺服器相同的規則，因此 `--kind mailer` 會拒絕 `http://` 位址。受測的外掛可能是真正在使用的外掛，因此做了呼叫本身以外事情的案例，只有在旗標要求時才會執行。如果這個版本還沒有某個種類的契約，會回報 `reachable` 和一個 `SKIP`。

`--kind billing` 只檢查在不允許任何計費請求時，外掛會拒絕產生任何花費。`--billed` 還會實際擷取本月和上個月的資料，並像匯入時一樣檢查回應；這可能會在外掛背後的供應商產生費用。[匯入雲端使用費用](billing-import.md#a-plugin-billing_sources--plugin)收錄了這個種類的契約。

`--kind mailer` 除非 `--mail-to` 指定了收件者，否則不會呼叫 RPC。指定時，指令會從 `--mail-from`（預設為相同地址）寄送一則真正的訊息到那裡，並檢查外掛會拒絕沒有收件者的請求。[寄送電子郵件](email.md#a-plugin-mail--plugin)收錄了這個種類的契約。

`--kind translator` 會傳送幾個固定的英文句子，翻譯成日文（對於只支援部分語言的外掛，可用 `--translate-from` 和 `--translate-to` 選擇其他語言組合）。外掛背後依字元計費的服務也會對這些內容計費，大約 200 個字元。它會檢查回應的格式，以及外掛是否拒絕語言 `qaa`（ISO 639 保留給私人用途的代碼）。它無法判斷用字，因此請自己閱讀外掛的翻譯結果。[翻譯貼文](#translating-posts)收錄了這個種類的契約。

`--kind search` 會進行搜尋，並且只寫入和移除它自己的文件：幾份 ID 不屬於任何貼文、作者不屬於任何帳號、包含為這次執行產生的詞的文件，並在最後移除它們。它會檢查沒有詞的查詢找不到任何東西、以引擎查詢語言撰寫或以日文撰寫的查詢不會失敗、外掛會拒絕不是它發行的 cursor，以及文件會由新到舊逐頁被找到、可以被取代，也可以依貼文和依作者移除。`--clear-index` 還會執行移除某個可見範圍所有文件和整個索引的案例：請只對測試用的索引使用它。[搜尋貼文](#searching-posts)收錄了這個種類的契約。

`--kind media_image`、`--kind media_video` 和 `--kind media_audio` 只會傳送它們自己產生的小型測試檔案：一張照片、一個表情符號和一張 GIF 動畫、一個帶有影像和音軌的 MP4，或一個 MP3。每個檔案都在中繼資料中藏有位置或標題，如果它出現在輸出中，案例就會失敗。它們會像伺服器一樣檢查每個輸出。[轉換媒體](#converting-media)收錄了這些種類的契約。

`--kind key_manager` 會執行伺服器每次啟動時執行的檢查，並檢查外掛會拒絕被竄改的值和它不持有的 KEK。它只傳送自行產生、用完即丟的金鑰，因此對真正的金鑰儲存也很安全，但金鑰儲存可能會對呼叫計費。它檢查的是回應的格式是否正確，而不是外掛如何保管它的 KEK。[管理金鑰](key-management.md#a-plugin)收錄了這個種類的契約。

`probe` 是最小的種類，伺服器不會委派任何任務給它。請先提供它，在撰寫外掛的其餘部分之前，先確認傳輸、協定和驗證。

<!-- source: translating-posts c9d31b2b9157 -->
## 翻譯貼文

種類為 `translator` 的外掛，可以讓伺服器在讀者要求時，透過[內建供應者](translation.md)（LibreTranslate、DeepL、Google Cloud Translation 和 Amazon Translate）以外你所選擇的服務翻譯貼文。沒有伺服器端的供應者時，唯一的翻譯是讀者瀏覽器內建的翻譯，而它只存在於桌面版的 Chrome、Edge 和 Opera：在手機、Firefox 或 Safari 上的讀者，在你啟用某個供應者之前都不會看到翻譯按鈕。

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. 設定 `plugin_translator_address` 和 `plugin_translator_name`，然後重新啟動。名稱是必填的，因為讀者會依翻譯的來源決定是否信任它。位址永遠不會離開設定。
2. [更新你的隱私權政策](privacy-policy-template.md#the-services-you-choose)，將外掛背後的服務列為翻譯服務：外掛轉交出去的，就是它收到的內容。
3. 以[實例設定](instance-settings.md) `translation_providers = plugin` 啟用它。變更會立即生效。

只有位址不會啟用任何功能：在設定指定 `plugin` 之前，伺服器不會傳送任何內容，讀者也看不到按鈕。

外掛與其他供應者一樣是一個供應者。[`translation_provider_visibilities`](instance-settings.md) 決定它可以收到什麼，每月上限會計算傳送給它的內容，而它產生的翻譯也和其他翻譯一樣保留在伺服器的快取中。開啟 `translation_reports_to_server` 時，它也會依相同的可見範圍，收到版主在檢舉中要求翻譯的內容。在 `translation_providers` 的順序中，它可以排在其他供應者之前或之後。

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### 外掛收到的內容

讀者要求翻譯時，外掛會收到貼文的文字：內文、內容警告、投票選項和圖片描述，以及文字的語言和要翻譯成的語言。它不會收到貼文的位址、作者、讀者、提及、主題標籤、連結、自訂表情符號或程式碼：這些會以編號的標記代替。

預設是 `public`、`local` 和 `unlisted` 貼文。**`private` 和 `direct` 貼文只有在你允許時才會送到外掛**，設定方式為 `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`。這時外掛以及它背後的任何服務，都會讀到作者只想給少數人看的文字，因此只有在你能掌控文字的去向時才允許，並在你的隱私權政策中說明。伺服器無法收回已經傳送的文字。

由於內容是貼文的文字，伺服器會拒絕這個種類使用 `http://`，loopback 也一樣。請使用 Unix socket 或 `https://`。

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### 外掛背後的 LLM

伺服器沒有 LLM 供應者，也刻意不提供。任何人都能撰寫貼文，因此要翻譯的文字可以成為對 LLM 下指令的途徑（提示注入），而讀者會把翻譯當成貼文的內容。清理（sanitizing）保護的是頁面的 HTML：它無法阻止翻譯說出貼文沒有說的話。

伺服器看不到外掛呼叫了什麼。**如果你的外掛使用 LLM，這項風險及其後果都由你自行負責**。

<!-- source: the-contract ef518f876dd2 -->
### 契約

RPC 是 `my.plugin.translator.v1.TranslatorService` 的 `Translate`：

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` 是 BCP 47 標籤，伺服器不知道語言時則為 `und`。這時外掛要偵測語言並在 `detectedLanguage` 中回報，或回應 `OUTCOME_UNSUPPORTED_LANGUAGE`。外掛沒有要補充的資訊時，`detectedLanguage` 可以不存在，伺服器會忽略不是語言標籤的值。
- `translations` 每個片段一個項目，依片段的順序排列，都不能是空的。片段可能只包含標記，這時它的翻譯就是那些標記。
- `⟦1⟧` 代表一個項目，或開啟一組由 `⟦/1⟧` 關閉的配對。片段中的每個標記都必須原封不動地在翻譯中恰好出現一次。項目可以依語序需要移動位置，但配對必須先開啟再關閉，且配對可以巢狀。
- 翻譯是純文字。伺服器會跳脫其中的任何標記語言，並在儲存或顯示之前清理結果。

| `outcome` | 意義 | 伺服器的處理 |
| --- | --- | --- |
| `OUTCOME_OK` | 回應中包含翻譯 | 檢查、儲存，並回應讀者 |
| `OUTCOME_TEMPORARY` | 服務失敗或沒有回應 | 嘗試下一個供應者 |
| `OUTCOME_PROVIDER_LIMIT` | 服務回報了自己的速率限制或配額 | 嘗試下一個供應者 |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | 服務不支援來源或目標語言，或無法偵測語言 | 嘗試下一個供應者，沒有任何供應者支援時告知讀者 |

違反這些規則的回應，與不存在或未知的 `outcome` 以及失敗的呼叫一樣，視為 `OUTCOME_TEMPORARY`，其內容不會儲存。沒有任何供應者回應時，讀者會看到翻譯失敗，貼文維持原樣；但如果 `translation_browser` 已開啟，且讀者的瀏覽器能翻譯這則貼文，網頁用戶端就會在瀏覽器中翻譯。每次呼叫的字元數都會計入每月上限，失敗的呼叫也一樣，因為服務可能已經收到了它們。

伺服器會在 `plugin_translator_timeout` 之後停止等待，而且不論該設定為何，最多 20 秒。同一個請求的所有供應者合計有 45 秒。伺服器不會重試失敗的呼叫，而相同的片段可能會因為另一位讀者再次送來。

<!-- source: searching-posts bd9dfba5a3e7 -->
## 搜尋貼文

種類為 `search` 的外掛，會把[貼文全文搜尋](search.md)的索引保存在伺服器沒有內建支援的引擎中，例如 Groonga 或 PGroonga、以 Sudachi 建立的索引、Typesense、Manticore，或託管的搜尋服務。對伺服器而言，它只是另一個引擎：相同的貼文會進入索引，相同的任務會讓它保持最新，而且仍由伺服器決定每個人可以看到什麼。

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. 執行外掛，並用 [`my plugin test --kind search`](#test-a-plugin) 檢查它。
2. 設定 `search = plugin` 和 `plugin_search_address`，並和任何引擎一樣依照[啟用搜尋](search.md#turning-it-on)操作：索引從那時起開始累積，`my search rebuild` 會加入既有的貼文。
3. 如果外掛會把貼文傳送給其他公司的服務，請在你的[隱私權文件](search.md#your-privacy-documents)中列出該服務。

伺服器在啟動時不會開啟連線，因此外掛停止運作時，只有搜尋會停止。要改用外掛或不再使用外掛，請依照[關閉或更換引擎](search.md#switching-off-or-changing-the-engine)操作。

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### 外掛收到的內容

- **索引中的貼文**。對於索引中的每則貼文，外掛會收到它的文字（內文、內容警告和圖片描述，皆為純文字）、ID、作者的帳號 ID、可見範圍、語言，以及發布時間。依照 [`search_visibilities`](instance-settings.md) 的設定，這些只有允許搜尋的作者的 `public` 和 `local` 貼文。`local` 貼文原本只給這個伺服器的帳號看：外掛以及它背後的任何服務也會讀到它們。`unlisted`、`private` 和 `direct` 貼文永遠不會送到外掛。
- **查詢**。已登入的使用者搜尋貼文時，外掛會收到他輸入的內容，以及頁面的 cursor 和結果數量。它不會收到是誰搜尋的。
- **移除**：移除一則貼文、一個帳號的所有貼文、某個可見範圍的所有貼文，以及整個索引。

外掛只會回傳貼文 ID。伺服器會從資料庫重新讀取每則貼文，只顯示該使用者目前可以看到的內容，因此外掛無法把貼文顯示給不該看到它的人，而落後的索引只會找到較少的結果。伺服器也永遠不會向任何人顯示外掛的錯誤。

由於這個種類傳送的是貼文的文字和使用者搜尋的內容，伺服器會拒絕它使用 `http://`，loopback 也一樣。請使用 Unix socket 或 `https://`。

<!-- source: the-contract-1 d500365750c4 -->
### 契約

服務是 `my.plugin.search.v1.SearchService`，每個操作一個 RPC：

| RPC | 作用 |
| --- | --- |
| `Replace` | 加入一則貼文的文件，或取代 `statusId` 相同的文件 |
| `Remove` | 移除一則貼文的文件 |
| `RemoveByAuthor` | 移除一個帳號的所有文件 |
| `RemoveByVisibility` | 移除某個可見範圍（`VISIBILITY_PUBLIC` 或 `VISIBILITY_LOCAL`）的所有文件 |
| `Clear` | 移除所有文件；之後索引仍會接受新文件 |
| `Search` | 由新到舊回傳查詢找到的文件 ID 的其中一頁 |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **查詢是純文字**。外掛把它切分成詞，找出包含每個詞的文件。它永遠不會把文字當成引擎的查詢語言解讀：其中的運算子、萬用字元、引號或欄位名稱都只是文字。沒有詞的查詢找不到任何東西。外掛如何在日文和其他不以空格分詞的語言中找出詞，由外掛自行決定，而這正是使用外掛的理由。
- **頁面由新到舊排列**。`statusIds` 依遞減順序排列，每個最多出現一次，最多 `limit` 個，每個都是伺服器傳送時的貼文 ID（36 個小寫字元）。ID 是 UUIDv7，因此其字串順序就是時間順序。
- **cursor 屬於外掛**。最後一頁的 `nextCursor` 為空，其他頁則是最多 256 位元組、不含空格的可列印 ASCII，例如該頁的最後一個 ID。伺服器會原封不動地送回它以取得下一頁，即使期間有文件增減，下一頁也會從該 ID 之後接續。不是最後一頁的頁面，ID 數量可能少於 `limit`。不是外掛發行的 cursor 會得到 `OUTCOME_INVALID_CURSOR`。
- **寫入會反映在下一次搜尋中**，而且每個呼叫都可能來兩次：移除不存在的東西也算成功。如果引擎要在重新整理後才會反映寫入，外掛就要等待重新整理完成。

| `outcome` | 意義 | 伺服器的處理 |
| --- | --- | --- |
| `OUTCOME_OK` | 呼叫完成了要求的事 | 對一頁的 ID 套用檢視政策 |
| `OUTCOME_FAILED` | 引擎失敗或沒有回應 | 由寫入的任務重試；搜尋則失敗，使用者可以再試一次 |
| `OUTCOME_INVALID_CURSOR` | 僅限 `Search`：cursor 不是外掛發行的 | 告知用戶端其 cursor 錯誤 |

不存在或未知的 `outcome`、失敗的呼叫，以及違反這些規則的頁面，都視為 `OUTCOME_FAILED`，這種頁面的內容不會顯示。一個呼叫最多持續 `plugin_search_timeout`（預設 10 秒），而且不論該設定為何，最多 10 秒。

<!-- source: converting-media 36df3dbdb21b -->
## 轉換媒體

媒體外掛負責上傳處理中繁重的部分，讓它可以在另一台機器上執行。共有三個種類，每個種類可以分別選擇：

- `media_image` 將圖片重新編碼為各種變體。
- `media_video` 重新封裝（remux）影片：帶有影像軌的檔案，不論有沒有聲音。
- `media_audio` 重新封裝只有音訊的檔案。

其餘的任務仍由伺服器自己處理。它會偵測每個上傳檔案的格式、區分影片和音訊檔案、在上傳到達時依上限檢查，並製作遠端媒體的預覽。設定維持 `native` 的種類會在伺服器中處理。

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

為每個要交出的種類設定 `media_<kind> = plugin` 和 `plugin_media_<kind>_address`，然後重新啟動。一個外掛可以負責多個種類：為每個種類設定相同的位址即可。每個種類同時最多執行兩個呼叫，每個呼叫最多持續 `plugin_media_<kind>_timeout`（預設 4 分鐘），因此處理緩慢的影片不會耽誤圖片。

- **移除中繼資料是外掛的任務**。上傳檔案會以送來時的原樣到達外掛，包含其 EXIF 資料和標籤，其中可能含有照片的拍攝地點。伺服器會檢查每個輸出的格式和大小，但無法判斷中繼資料是否已完全清除。`my plugin test --kind media_image` 會檢查其測試檔案的位置不會再出現；其餘部分請在外掛中確認。
- **純 TCP 會以未加密的方式傳送上傳檔案**。媒體種類接受 `http://`，並在啟動時發出警告。這時上傳檔案及其中繼資料會以明文通過網路，而且兩端都沒有驗證：該網路上的任何人都可以讀取它們，或冒充外掛回應。除非網路只屬於你，否則請使用 Unix socket 或 `https://`。
- **伺服器不信任輸出**。它會從內容偵測每個輸出的格式，絕不依據外掛的宣告，並以該格式的內容類型和副檔名提供。它會在與上傳相同的上限下解碼每個輸出，並檢查其大小是否相符。如果輸出比宣告的大或超過這些上限、格式不是外掛宣告的格式，或是該變體不接受的格式，上傳就會以伺服器無法處理的檔案失敗。
- **失敗只會造成延遲**。外掛停止運作或失敗時，任務會在稍後重試，與內建處理器相同。需要立即取得自訂表情符號圖片的操作（例如複製遠端表情符號，或接受表情符號申請），在外掛失敗期間會失敗。

<!-- source: the-contract-2 812c987c276c -->
### 契約

服務是 `my.plugin.media.v1.MediaService`，RPC 為處理圖片的 `Process`，以及處理影片或音訊檔案的 `Remux`。兩者都是雙向串流，因此外掛需要 Connect 或 gRPC 函式庫和 HTTP/2：使用 `proto` 或 `grpc`，透過 ALPN 為 `h2` 的 TLS，或在 Unix socket 或純 TCP 上透過明文 HTTP/2（h2c）。

呼叫是半雙工的：

1. 伺服器傳送 `head`：上限，以及圖片的用途、三種尺寸的方框和焦點。接著以每則最多 1 MiB 的 `chunk` 訊息傳送檔案。然後伺服器關閉自己這一端。
2. 外掛以 `head` 回應，其 `outcome` 即為結果。為 `OUTCOME_OK` 時，`Process` 的 head 還包含套用 EXIF 方向後的圖片尺寸，以及小型靜態輸出的 blurhash。
3. 對每個輸出，外掛傳送一個 `part`，指明輸出並宣告其格式、尺寸和長度，接著是每則最多 1 MiB 的 `chunk` 訊息。

`Process` 回傳小、中、大三種靜態輸出，以及多影格圖片的三種動態輸出。小和中的輸出為 WebP；大的靜態輸出為 JPEG 或 PNG（自訂表情符號一律為 PNG），大的動態輸出為 GIF。`Remux` 回傳以原本容器封裝、不含中繼資料的檔案，以及其中嵌入的封面圖片（如果有的話）。伺服器從 `Process` 最多讀取圖片上限的兩倍，從 `Remux` 則對檔案讀取最多影片上限、對封面讀取最多圖片上限。

| `outcome` | 意義 | 伺服器的處理 |
| --- | --- | --- |
| `OUTCOME_OK` | 接著傳送輸出 | 檢查並儲存 |
| `OUTCOME_TEMPORARY` | 外掛失敗 | 稍後重試任務 |
| `OUTCOME_UNSUPPORTED_FORMAT`、`OUTCOME_UNSUPPORTED_CODEC`、`OUTCOME_TOO_LARGE`、`OUTCOME_DIMENSIONS`、`OUTCOME_CORRUPT`、`OUTCOME_TOO_LONG` | 檔案無法接受 | 將上傳標記為失敗 |

不存在或未知的 `outcome` 以及失敗的呼叫，都視為 `OUTCOME_TEMPORARY`。

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## 不使用函式庫的外掛

使用 `json` 時，外掛只需要 HTTP 伺服器和 JSON 函式庫。RPC 是以 `Content-Type: application/json` 對 `/<package>.<Service>/<Method>` 發出的 `POST`，外掛以 `200` 和一個 JSON 物件回應：

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

JSON 遵循 proto3 的標準對應：

- `int64` 是十進位字串，`bytes` 是 base64，列舉則是其名稱。
- 值為零值的欄位可能不存在，雙向皆然。外掛必須把不存在的欄位當作零值讀取。
- 伺服器會忽略它不認識的欄位。
- 結果是回應的 `outcome`。不存在或未知的 `outcome` 一律視為失敗，絕不視為成功。
- 失敗的外掛要回應 HTTP 錯誤，或 Connect 的 `{"code":"unavailable"}` 加上相應的狀態。任務會重試。

`Connect-Timeout-Ms` 和 `Connect-Protocol-Version` 可以忽略。
