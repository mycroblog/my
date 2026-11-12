<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# 搜尋引擎與 AI 爬蟲

本頁說明伺服器會向搜尋引擎和 AI 爬蟲表明什麼、可以改變它的設定，以及如何確認伺服器實際提供的內容。

robots.txt 和 meta 標籤只是聲明，爬蟲可能遵守也可能不遵守；它們並不是技術上的封鎖。如果需要封鎖，請使用 WAF 或速率限制。

<!-- source: settings 13dfb1b04948 -->
## 設定

| 範圍 | 設定 | 變更位置 | 未設定時 |
| --- | --- | --- | --- |
| 伺服器 | `search_indexing`（`allow` / `deny`） | 管理介面伺服器設定中「搜尋與爬蟲」頁面的「搜尋引擎與 AI」區塊、`my instance set`、`PATCH /api/v1/admin/settings`（[實例設定](instance-settings.md)） | `deny` |
| 伺服器 | `ai_usage`（`allow` / `deny_training` / `deny`） | 同上 | `deny` |
| 伺服器 | `indexable_default`（新帳號的 `indexable`） | 同上 | `false` |
| 伺服器 | `security_contact`（回報弱點的聯絡方式） | 同上 | 空白（不提供 `security.txt`） |
| 個人 | `indexable`（出現在搜尋結果中） | 個人檔案設定中的「搜尋引擎與 AI」、`PATCH /api/v1/accounts/update_credentials` | 註冊時的 `indexable_default` |
| 個人 | `ai_opt_out`（拒絕 AI 訓練與使用） | 同上 | `false` |

- 頁面的聲明採用伺服器設定與頁面擁有者設定中較嚴格的一方。個人無法允許比伺服器更多的事。
- 只有在 `search_indexing = allow` 的伺服器上、`indexable = true` 的帳號，其個人檔案和 `public` 貼文才會告知搜尋引擎可以建立索引。想要被搜尋到的伺服器，要同時變更 `search_indexing` 和 `indexable_default`。在你變更 `indexable_default` 之前就已註冊的人，在他們自己選擇 `indexable` 之前不會被納入。
- 變更後的設定會在 `/robots.txt`、頁面和媒體回應中生效，不需要重新啟動。

<!-- source: what-the-server-returns 31584e03601b -->
## 伺服器回傳的內容

| 回應 | 拒絕搜尋時 | 拒絕 AI 時 |
| --- | --- | --- |
| 個人檔案和 `public` 貼文的頁面 | `<meta name="robots">` 和 `X-Robots-Tag` 中的 `noindex, noarchive` | `noai, noimageai`、`Content-Usage: train-ai=n`（`deny` 時為 `train-ai=n, ai-use=n`），以及 `tdm-reservation: 1`（標頭和 meta） |
| `unlisted` 貼文、隱藏或不存在頁面的 404 頁面、遠端帳號的頁面、應用程式畫面（`/home`、`/settings` 等） | 不論設定為何一律 `noindex, noarchive` | 依照伺服器設定 |
| `/`、`/about`、`/about/crawler` | 依照伺服器設定 | 依照伺服器設定 |
| `/media/`（由這台伺服器提供時） | 依照伺服器設定。沒有個別擁有者的聲明 | 依照伺服器設定。沒有個別擁有者的聲明 |
| 用戶端 API、ActivityPub 文件、`/.well-known/`、`/nodeinfo/`、登入和 OAuth 畫面 | `X-Robots-Tag: noindex` | 不加入（robots.txt 就足夠了） |

`noindex` 不會阻止連結預覽（請參閱[連結預覽](link-previews.md)）。

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` 是根據伺服器設定和內建的 AI 爬蟲列表產生的（`text/plain; charset=utf-8`、`Cache-Control: public, max-age=3600`、ETag）。營運者無法加入自己的規則。

- 如果 `ai_usage` 為 `deny_training` 或 `deny`，訓練用爬蟲（`GPTBot`、`ClaudeBot`、`CCBot`、`Google-Extended`、`Applebot-Extended` 等）會收到 `Disallow: /`。
- 如果 `ai_usage` 為 `deny_training` 或 `deny`，一個帶有 `Disallow: /` 的 `User-agent: __usage_ai-training` 群組會針對所有路徑表明不希望被用於 AI 訓練，不論擷取內容的是哪個爬蟲（Internet-Draft `draft-kelley-aipref-training-preference-signal`）。它指定的是用途而不是爬蟲，所以不會阻止任何擷取。`ai_usage = allow` 時會省略這個群組，也就是不表明任何偏好，因為其 `Allow: /` 會針對所有路徑表明贊成訓練，包括設定了 `ai_opt_out` 的人的頁面。
- 如果 `ai_usage` 為 `deny`，AI 搜尋、助理和代理程式的爬蟲（`OAI-SearchBot`、`ChatGPT-User`、`Claude-User`、`PerplexityBot` 等）也會收到 `Disallow: /`。
- 所有爬蟲（`User-agent: *`）只會被拒絕 `/api/`、`/oauth/`、`/auth/`、`/ap/` 和 `/media/proxy/`，設定則以 `Content-Usage`（IETF AI Preferences 草案）和 `Content-Signal`（Content Signals）行表明。拒絕搜尋不會產生 `Disallow: /`：封鎖擷取會讓爬蟲讀不到 `noindex`，也會擋下遵守 robots.txt 的連結預覽擷取程式。
- 不會拒絕 `/.well-known/` 和 `/nodeinfo`，讓伺服器列表和統計能繼續運作。
- 當 `account_domain` 與主機分開時，由主機那一側提供它。帳號名稱網域的 robots.txt 由營運者自行處理。

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| 路徑 | 提供時機 | 內容 |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` 不是 `allow` 時 | `[{"location": "/*", "tdm-reservation": 1}]`（TDM Reservation Protocol）。`allow` 時為 404 |
| `/.well-known/security.txt` | 設定了 `security_contact` 時 | `Contact`、`Expires`（回應時間的 30 天後）、`Preferred-Languages`（`default_locale` 和 `en`）、`Canonical`（RFC 9116） |

<!-- source: information-page 0f89d6c3c3ce -->
### 資訊頁面

實例資訊頁面（`/about`）會根據 `GET /api/v1/instance` 中的 `search_indexing` 和 `ai_usage`，顯示是否允許搜尋和 AI，以及這些聲明的意義。說明中會將這些聲明對照日本著作權法第 30 條之 4（這類聲明不一定會被視為法律上的拒絕）和歐盟 DSM 指令第 4 條（以機器可讀的方式保留文字與資料探勘的權利）。它不保證任何法律效力。

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## 這台伺服器自己的擷取

這些設定是關於誰來爬取這台伺服器。伺服器本身也會以 `my-linkcard` 的身分爬取其他網站，用來產生貼文中連結的卡片：如[連結預覽](link-previews.md#cards-of-links-in-posts)所述，它會遵守 `robots.txt` 和被連結頁面的指示，並限制對每個主機的請求數。這個爬蟲以 `link_cards`、`link_cards_ignore_robots_txt` 設定，並可透過[連結卡片網域](instance-settings.md#link-card-domains)按網域設定。

這些網站的營運者可以在 `/about` 連結的 `https://<host>/about/crawler` 閱讀相關說明（[給網站營運者的頁面](link-previews.md#the-page-for-site-operators)）。開啟 `link_cards_ignore_robots_txt` 時，他們無法用 `robots.txt` 拒絕，頁面也會說明這一點：他們只能用自己頁面的指示來限制卡片，或請你為他們的網域設定規則。

<!-- source: limits 93da7e422ee8 -->
## 限制

- 個人的 `ai_opt_out` 只會在這台伺服器提供的頁面上表明。媒體的鍵值中沒有擁有者，而且有些架構會讓 CDN 直接提供媒體，所以媒體只帶有伺服器整體的聲明。其他伺服器所提供頁面上的聲明交由那些伺服器處理（不會送出自訂的 ActivityPub 擴充）。
- `ai_usage = deny` 也表明拒絕 ChatGPT 或類似服務的使用者讓服務讀取這台伺服器頁面的行為。
- 遠端帳號的 `indexable`（Mastodon 的 `toot:indexable`）會被接收並儲存。在這台伺服器上，遠端帳號的頁面一律為 `noindex`。
- 在有搜尋引擎的伺服器上，`indexable` 也決定一個人的貼文是否會出現在這台伺服器自己的[全文搜尋](search.md)中。在那裡，它涵蓋 `search_visibilities` 的可見範圍，其中可以包含 `local`。`search_indexing` 對該搜尋沒有影響，而告知搜尋引擎的內容仍如本頁所述。

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## 從物件儲存或 CDN 提供媒體

當物件儲存或 CDN 直接提供媒體時，`/media/` 的回應不會經過這台伺服器，也不會帶有上表中的標頭。伺服器整體的聲明仍會透過 `/robots.txt` 套用到所有內容；若也要加上這些標頭，請在 CDN 的回應標頭規則中，為 `/media/*` 加入以下內容（`ai_usage = deny` 的範例）。

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

變更設定時，請一併變更 CDN 規則使其一致。`/media/proxy/` 由這台伺服器提供，所以不需要規則。

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## AI 爬蟲列表

這份列表是從 [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt)（MIT）的 `robots.json` 產生，並內建在二進位檔中。它不會在執行時擷取，所以較新的列表會隨升級一起提供。

<!-- source: verify 1633f03abedb -->
## 確認

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- 在沒有任何設定的伺服器上，`/robots.txt` 中有 `User-agent: GPTBot`、`User-agent: __usage_ai-training` 和 `Content-Usage: search=n, train-ai=n, ai-use=n`，個人檔案則帶有 `X-Robots-Tag: noindex, noarchive, noai, noimageai`。
- 在 `search_indexing = allow` 和 `ai_usage = allow` 時，選擇了 `indexable` 的帳號，其個人檔案不帶有 `X-Robots-Tag`，而 `/.well-known/tdmrep.json` 為 404。
