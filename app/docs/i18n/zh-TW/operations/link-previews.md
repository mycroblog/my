<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# 連結預覽（OGP / X Card / fediverse:creator）

本頁說明其他服務為這台伺服器的頁面顯示的預覽、伺服器為此回傳的內容，以及手動檢查的步驟。這台伺服器為自己貼文中的連結顯示的卡片，請參閱下方的[貼文中連結的卡片](#cards-of-links-in-posts)；如何停止從某個網域擷取卡片，請參閱[實例設定](instance-settings.md#link-card-domains)。

<!-- source: what-the-server-returns bb9326f9729a -->
## 伺服器回傳的內容

`my serve` 會把 meta 標籤嵌入網頁用戶端的外殼（shell），回傳以下頁面的 HTML：首頁（`https://<host>/`）、實例資訊頁面（`https://<host>/about`）、已公開文件的頁面（`/about/terms`、`/about/privacy`、`/about/commerce`）、連結卡片爬蟲的說明頁面（`/about/crawler`）、個人檔案（`https://<host>/@alice`）以及貼文（`https://<host>/@alice/status/<post ID>`）。對 HTML 的請求一律被視為未登入，因此預覽只會顯示未登入的訪客能看到的內容。

| 頁面 | 回應 |
| --- | --- |
| 首頁（`/`）和實例資訊頁面（`/about`） | 200。帶有介紹實例的 meta 標籤的外殼（見下方「首頁」） |
| 連結卡片爬蟲的說明頁面（`/about/crawler`） | 200。帶有介紹實例的 meta 標籤的外殼，標題為「連結卡片爬蟲 \| 實例名稱」 |
| 已公開文件的頁面（`/about/terms`、`/about/privacy`、`/about/commerce`） | 200。帶有介紹實例的 meta 標籤的外殼，標題為「文件名稱 \| 實例名稱」。未公開文件的頁面會得到通用的外殼 |
| `public` 貼文、有效的本站帳號的個人檔案 | 200。帶有 meta 標籤的外殼 |
| `unlisted` 貼文 | 200。帶有 meta 標籤的外殼，並加上 `<meta name="robots" content="noindex, noarchive">` 和 `X-Robots-Tag: noindex, noarchive` |
| `local`、`private`、`direct`、已刪除或不存在的貼文，以及被停權、已停用、已刪除或受限帳號的個人檔案和貼文 | 404。通用的外殼和 `X-Robots-Tag: noindex, noarchive`。不論原因為何，內文和標頭都相同 |
| `/@<old username>/status/<post ID>` | 301，導向使用目前使用者名稱的 URL |

每個頁面也會依實例設定和擁有者的設定，帶有對搜尋引擎和 AI 爬蟲的聲明（`noindex` 和 `noai` 等 meta 標籤，以及 `X-Robots-Tag`、`Content-Usage` 和 `tdm-reservation`）（[搜尋引擎與 AI 爬蟲](crawlers.md)）。上表中的 `noindex` 不論設定為何都一定會加上。`noindex` 不會阻止連結預覽。

帶有 ActivityPub `Accept` 的請求不會得到外殼；而是由 ActivityPub 端回應（個人檔案以行為者的 `id` 回應，未登入訪客能看到的貼文則以 302 導向它的 `id`）。所有回應都帶有 `Vary: Accept`。

<!-- source: meta-tags 00a4a8115b9e -->
### Meta 標籤

| 標籤 | 內容 |
| --- | --- |
| `og:title`、`<title>` | 「顯示名稱（@使用者名稱@帳號網域）」。顯示名稱為空時使用使用者名稱 |
| `og:type` | 個人檔案為 `profile`，貼文為 `article` |
| `og:url`、`<link rel="canonical">` | 標準 URL（使用目前使用者名稱的 URL） |
| `og:description` | 去除標籤後的貼文內文純文字（個人檔案為自我介紹），截斷為 200 個字素。有內容警告的貼文只使用內容警告的文字 |
| `og:image`、`og:image:alt` / `width` / `height` | 依下方順序選出的圖片 |
| `og:site_name` | 實例名稱（文字 `name`，所有語言都相同；沒有時使用 `host`；[名稱與網域](instance-settings.md#name-and-domain)） |
| `og:locale`、`<html lang dir>` | 貼文的語言。未設定（`und`）時使用作者的預設發文語言，再沒有則使用實例的 `default_locale`。不會隨 `Accept-Language` 改變 |
| `article:published_time`、`article:author` | 僅限貼文。貼文的時間（UTC）和作者的個人檔案 URL |
| `profile:username` | 僅限個人檔案。`username@account domain` |
| `twitter:card` | 圖片寬於約 2:1（寬 / 高 >= 1.7）且至少為 300x157 時為 `summary_large_image`，否則為 `summary` |
| `fediverse:creator` | `@username@account domain`。Mastodon 會把它顯示為「作者」 |
| `<link rel="alternate" type="application/activity+json">` | AP `id`（行為者或貼文的物件） |

首頁的 meta 標籤介紹的是實例本身。

- `og:type` 為 `website`，`og:title`、`<title>` 和 `og:site_name` 為實例名稱，`og:url` 和 `<link rel="canonical">` 為 `https://<host>/`。
- `og:description` 是截斷為 200 個字素的實例文字 `short_description`。沒有時則省略。
- 語言（`og:locale`、`<html lang dir>`）是實例的 `default_locale`。名稱和說明在所有語言中都是相同的值。
- `og:image` 是下方的第 3 項（實例預覽圖片），沒有時為第 4 或第 5 項。
- 它不屬於任何帳號，因此不會輸出 `fediverse:creator` 和 ActivityPub 的 `alternate`。
- 實例資訊頁面（`/about`）有相同的 meta 標籤。`og:title` 和 `<title>` 為「關於 {instance name}」（`server:preview.about_title`），`og:url` 和 `<link rel="canonical">` 為 `https://<host>/about`。
- 已公開文件的頁面（`/about/terms`、`/about/privacy`、`/about/commerce`）也有相同的 meta 標籤。`og:title` 和 `<title>` 為「{document name} | {instance name}」（`server:preview.document_title`），`og:url` 和 `<link rel="canonical">` 為該頁面的網址。未公開文件的頁面會得到通用的外殼。
- 連結卡片爬蟲的說明頁面（`/about/crawler`）有相同的 meta 標籤，標題和網址的形式也相同。

`og:image` 依以下順序選擇。

1. 貼文附加的第一張圖片（其替代文字會放入 `og:image:alt`）。標示為敏感內容或有內容警告的貼文不會使用。
2. 個人檔案的封面圖片，沒有則使用大頭貼（`og:image:alt` 為「{name} 的封面圖片」或「{name} 的大頭貼」）。
3. 實例預覽圖片（實例設定 `preview_image`；`og:image:alt` 為文字 `preview_image_alt`，沒有時為「{instance name} 的圖片」）。
4. 實例自己標誌的連結預覽版本（[標誌與圖示](instance-settings.md#logo-and-icon)），位於 `https://<host>/brand/<revision>/preview-1200x630.png` 的 1200x630 PNG（`og:image:alt` 為「{instance name} 的圖片」）。
5. 內建的預設圖片（`https://<host>/preview-image.png`，1200x630 的 PNG。它是不含實例名稱的通用圖片，`og:image:alt` 描述這張圖片）。

因此，會顯示預覽的頁面一定有 `og:image` 和 `og:image:alt`。第 1 到第 3 項的圖片是處理後媒體的 `full` 變體，寬度和高度是處理時得到的值。如果 `preview_image` 仍在處理中、處理失敗或不是圖片，則使用第 4 項（沒有標誌時為第 5 項）。實例設定在每次請求時都會讀取，因此 `my instance set preview_image ...` 不需重新啟動即可生效（步驟請參閱[實例設定](instance-settings.md)）。

預設圖片會在啟動後的第一次請求時以標準函式庫繪製，並保存在處理程序中（約 10 KB）。它會帶著 `Cache-Control: no-cache` 和 ETag 提供。

<!-- source: caching 5bc906452b10 -->
### 快取

- 回應帶有 `Cache-Control: public, max-age=60, stale-while-revalidate=300`。刪除、可見範圍的變更，或文件的公開與取消公開，最多會在共用快取中保留這麼久。
- ETag 由範本、資源檔和實例設定的版本、語言、貼文的版本（revision）、實際的可見範圍、個人檔案的版本，以及選出的圖片組成（首頁、資訊頁面、文件頁面和爬蟲說明頁面則是：頁面的網址、標題、實例說明和選出的圖片）。`If-None-Match` 會得到 `304`。
- 如果你在前方放置 CDN，請把 `Accept` 加入快取鍵（以免 AP JSON 和 HTML 混在一起）。

<!-- source: manual-check bf2c7a86a5de -->
## 手動檢查

在升級或變更預覽設定之後，請在實際裝置上檢查各個服務。你需要一個可以從外部透過 HTTPS 連線的實例（預覽機器人無法連到 `localhost`）。

<!-- source: preparation 9fd604dc1529 -->
### 準備

1. 用測試帳號準備以下貼文。
   - 附有橫向圖片（例如 1200x630）和替代文字的 `public` 貼文
   - 沒有圖片的 `public` 貼文（來自有封面圖片的帳號）
   - 內文超過 200 個字元並含有表情符號（例如 👨‍👩‍👧‍👦）的 `public` 貼文
   - `unlisted` 貼文
   - `private` 貼文
   - 來自顯示名稱含有 `"><script>alert(1)</script>` 的帳號的貼文
2. 檢查每個 URL 的 HTML。

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - 狀態為 200，且上表中的 meta 標籤都存在。`unlisted` 貼文帶有 `noindex`。
   - `private` 貼文和不存在的 ID 會得到相同的 404 內文，並帶有 `X-Robots-Tag: noindex, noarchive`（設定拒絕 AI 時，也會帶有 `noai, noimageai`）。

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - 顯示名稱中的 `<script>` 被跳脫為 `&lt;script&gt;`。

<!-- source: x 73a2dffdfdbb -->
### X

1. 把附有圖片的貼文 URL 貼到撰寫畫面（或私訊）中。
2. 確認大圖卡片（`summary_large_image`）顯示了標題、網域和圖片。
3. 對於沒有圖片的貼文 URL，確認卡片顯示了封面圖片。對於既沒有封面圖片也沒有大頭貼的帳號，確認大卡片顯示預設圖片（`preview_image`，未設定時為內建圖片）。
4. 對於只有正方形大頭貼的帳號，確認卡片是小圖卡片（`summary`）。

X 會快取卡片。若要檢查變更，請在 URL 加上查詢字串（例如 `?v=2`）再貼上，讓它被視為不同的 URL。

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. 從其他實例上的帳號，發布一則包含附有圖片的貼文 URL 的貼文。
2. 確認貼文下方出現了帶有標題、說明和圖片的預覽卡片。
3. 確認卡片下方以「作者」顯示測試帳號（`fediverse:creator`），且可以前往其個人檔案。

Mastodon 會在每台擷取過卡片的伺服器上快取卡片。若要重新產生，請以新的 URL（加上查詢字串）發文。

<!-- source: discord 4df04642bb64 -->
### Discord

1. 把附有圖片的貼文 URL 貼到頻道中。
2. 確認嵌入內容顯示了網站名稱（實例名稱）、標題、說明和大圖。
3. 對於超過 200 個字元的貼文，確認說明以 `…` 結尾，且沒有任何表情符號在中間被截斷。

<!-- source: slack 9f1e524968f3 -->
### Slack

1. 把附有圖片的貼文 URL 貼到頻道中。
2. 確認展開的預覽顯示了網站名稱、標題、說明和圖片。
3. 確認 `unlisted` 貼文的 URL 也會出現預覽（`noindex` 是給搜尋引擎的，不會阻止預覽）。

Slack 也會快取預覽。若要檢查變更，請在 URL 加上查詢字串。

<!-- source: line ef9e1e682603 -->
### LINE

1. 在聊天中傳送附有圖片的貼文 URL。
2. 確認出現了帶有標題、說明和圖片的預覽。

LINE 也會快取預覽。若要檢查變更，請在 URL 加上查詢字串。

<!-- source: what-must-not-be-visible 187905fe778d -->
### 不應該看到的內容

在每個服務上，確認 `private` 貼文的 URL 和不存在的貼文的 URL 都會得到相同的通用預覽（只有實例名稱，或什麼都沒有），且不會出現任何內文、顯示名稱或圖片。

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## 貼文中連結的卡片

為了替貼文中的連結顯示卡片，這台伺服器的任務會擷取被連結的頁面及其圖片，並遵守被連結網站對此的聲明。它對頁面、圖片和 `robots.txt` 的請求使用 `my-linkcard/<version> (+https://<host>/)`。伺服器的其他所有請求，包括媒體代理，仍使用 `my/<version> (+https://<host>/)`。這些請求不傳送 Cookie 也不傳送 `Referer`，也不會透露是哪個帳號或貼文連結了該頁面。

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### 給網站營運者的頁面

`https://<host>/about/crawler` 會告訴被連結網站的營運者，爬蟲請求什麼、傳送什麼、頻率多高、如何拒絕，以及他們可以請你停止從他們的網域擷取。任何人不需帳號都能閱讀。實例資訊頁面（`/about`）的「搜尋引擎與 AI」區塊會連結到它；`User-Agent` 只寫出 `https://<host>/`。

它會依據 `link_cards` 和 `link_cards_ignore_robots_txt` 說明這台伺服器的行為，並顯示實例的聯絡方式（`contact_account`、`contact_email`）。請設定聯絡方式，讓網站有地方可以詢問：沒有設定時，頁面會說明這台伺服器沒有公開聯絡方式。開啟 `link_cards_ignore_robots_txt` 時，頁面會說明網站無法透過 `robots.txt` 拒絕這台伺服器。

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

`robots.txt` 中的名稱是 `my-linkcard`，不分大小寫。沒有該名稱的群組時，依 RFC 9309 的規定套用 `*` 群組。會讀取 `Allow`、`Disallow`、`*` 和 `$`，以最長的比對為準，平手時 `Allow` 優先；不會讀取 `Crawl-delay` 和其他記錄。`robots.txt` 會在 10 秒內讀取最多 512 KiB，較大的檔案則使用已讀取的部分。伺服器只依來源（scheme、主機和連接埠）保存 `my-linkcard` 的規則。

| `robots.txt` 的回應 | 伺服器的行為 | 保存時間 |
| --- | --- | --- |
| 200 | 遵守該檔案 | 24 小時 |
| 429 以外的 4xx | 視為沒有檔案：進行擷取 | 24 小時 |
| 429、5xx、網路故障，或伺服器拒絕的重新導向 | 不從該來源擷取任何東西 | 1 小時 |
| 超過 5 次重新導向 | 視為沒有檔案：進行擷取 | 24 小時 |

- 不論貼文的可見範圍為何，頁面的 URL 都會在請求前先檢查。它重新導向到的每個 URL 也會以該 URL 來源的 `robots.txt` 檢查，重新導向到被拒絕的 URL 時就在此結束：伺服器不會請求它，也不會產生卡片。
- 被 `robots.txt` 禁止的 URL，與頁面的其他失敗一樣，24 小時內不會再次擷取。如果只是無法讀取該檔案，伺服器會在一小時後再次嘗試該頁面，直到達到任務的嘗試次數上限。
- 圖片也會以同樣的方式，依它自己的來源檢查。被禁止的圖片會讓卡片沒有圖片。如果只是無法讀取該檔案，先前記錄的圖片會保留，新的圖片則在下次重新擷取前不會加入。
- 重新擷取（貼文可能在上次擷取的 14 天後觸發）時，如果 `robots.txt` 禁止該 URL，卡片會被移除。如果只是無法讀取該檔案，卡片會保留。
- 伺服器不會逐一重新檢查自己的卡片，因此網站變更規則後，卡片會在該卡片下次重新擷取時才被移除。

`link_cards_ignore_robots_txt`（[實例設定](instance-settings.md#settings-one-key-one-value)）會關閉以上所有行為：伺服器既不擷取也不使用 `robots.txt`。它仍然自稱 `my-linkcard`，而被連結的網站就無法透過 `robots.txt` 拒絕擷取：只剩下其頁面中的指示，以及請你為[他們的網域設定規則](instance-settings.md#link-card-domains)，正如[給網站營運者的頁面](#the-page-for-site-operators)所告訴他們的。

<!-- source: directives-of-a-page a854ef2ae8ea -->
### 頁面的指示

伺服器也會讀取它所擷取頁面的指示：`<head>` 中的 `<meta name="robots">` 和 `<meta name="my-linkcard">`，以及沒有機器人名稱或帶有 `my-linkcard:` 的 `X-Robots-Tag`。給其他機器人的指示不適用。有多個指示時，以較嚴格的為準。不論 `link_cards_ignore_robots_txt` 為何，這些指示都適用。

| 指示 | 卡片 |
| --- | --- |
| `nosnippet`、`max-snippet:0` | 沒有說明 |
| `noimageindex`、`max-image-preview:none` | 沒有圖片，也不會擷取圖片 |
| `noindex`、`nofollow`、`noarchive`、`none` 及其他 | 沒有變化 |

在任何指示下，標題、網站名稱和作者都會保留；不希望顯示它們的網站，請在 `robots.txt` 中拒絕擷取。指示要在擷取頁面後才會知道，因此不會阻止對頁面的請求。重新擷取時會套用頁面當時的指示。

<!-- source: limits-per-host 41838af70d25 -->
### 每台主機的限制

不論 `link_cards_ignore_robots_txt` 為何，爬蟲對每台主機都很溫和。主機是指 URL 中的名稱，不含連接埠，因此被連結的網站和其圖片的主機各有獨立的限制。

- 它一次只對一台主機進行一次擷取，下一次擷取至少在上一次結束 1 秒後才開始。一次擷取是指一個請求及其重新導向，以及在需要讀取時它所需要的 `robots.txt`。頁面和它的圖片是兩次擷取，即使在同一台主機上也是。
- `429` 或 `503` 會停止從回應它的主機擷取，即使它是在回應重新導向、圖片或 `robots.txt` 時也一樣。停止的時間依它的 `Retry-After`（以秒數或日期表示），至少 1 秒、最多 1 小時。沒有或無法讀取 `Retry-After` 時為 1 分鐘。
- 必須等待輪到自己的任務會稍後再回來。沒有任何 worker 在等待，也不會用掉任務的嘗試次數，因此即使大量連結同時指向一個網站，最後所有卡片都會產生，也不會有失敗的任務。同一台主機的任務大約每秒喚醒一個。
- 主機使爬蟲暫停的 URL 會保留其卡片，不會被記為失敗，並在暫停結束時擷取。某個 URL 在 24 小時內收到 5 次這樣的回應後，該頁面會與其他非 `200` 的頁面一樣被視為失敗，圖片則不會加入卡片。
- 等待圖片的卡片會保留它從頁面讀取的內容最多 2 小時，以免再次請求該頁面。
- [網域規則](instance-settings.md#link-card-domains)和網域處置優先：不會為不擷取任何東西的主機等待。

這些限制保存在鍵值儲存中，而且都會過期：在擷取途中停止的 worker，會在 2 分鐘內釋放它的主機。這些數字不是設定。若要觀察限制的運作：

- `my.link_card.host_pauses` 依主機回應的狀態計算暫停次數，每次暫停都會以 `INFO` 層級記錄 `link card host paused`，帶有 `host`、`status` 和 `pause_seconds`。
- `my.job.duration` 中 `my.job.type="content.fetch_link_card"` 且 `my.outcome="postponed"` 的部分，會計算稍後再回來的執行次數。數字很大表示有許多連結指向少數網站或暫停時間很長，而不是失敗。
