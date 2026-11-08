<!-- source: production-checklist e5f6c0c5b10a -->
# 正式上線檢查清單

在你向使用者和其他伺服器開放伺服器之前，以及每次變更部署之後，請逐項檢查這份清單。有些設定在伺服器開始聯邦之後就無法變更，而有些錯誤（例如遺失金鑰加密金鑰）則完全無法挽回。

每個項目都會說明它為什麼重要、如何確認，以及詳細說明在哪裡。請從自動檢查的部分開始：

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## `my config check` 會檢查什麼

在 `production` 設定檔（[`profile`](../reference/configuration.md#profile) 的預設值）中，`my config check` 會先驗證每一項設定，再檢查有效的設定是否適合正式環境的實例。問題會分成兩組列出，前面附有以行程語系撰寫的摘要，每一項都附有英文的技術細節：

- **錯誤**會讓 `my config check` 以 1 結束。這些設定會讓正式環境的實例無法運作，或削弱其安全性：
  - 其他伺服器無法連到的 `host` 或 `account_domain`：帶有連接埠、`localhost`、沒有點的名稱，或位於保留給本機或測試用途的頂級網域（`.localhost`、`.local`、`.test`、`.example`、`.invalid`、`.internal`、`.home.arpa`）之下的名稱。
  - 使用 `http` 的 `web_origin`。
  - 將資料庫放在記憶體中的 SQLite `database_url`。
  - 不是隨機產生的 `kek`，例如全為零或重複同一個位元組。
  - 屬於 CAPTCHA 服務公開的測試金鑰之一的 `captcha` 網站金鑰或密鑰。
  - 沒有指定任何任務類型的 `worker_job_types`、`worker_job_types_except` 或佇列路由（在所有設定檔中；`my job types` 會列出任務類型）。
  - 使用 `-instance` 時，資料庫連線池無法支應的 `worker_job_concurrency`。
- **警告**會讓結束代碼維持為 0。這些設定在開發時很常見，在正式環境中也可能是刻意的：
  - `mail = none`。
  - 位於暫存目錄（`/tmp`、`/var/tmp`、`/dev/shm` 或 `$TMPDIR`）中的 SQLite 資料庫或 `fs` blob 儲存區。
  - `queue = inline`。
  - 沒有 `vapid_private_key`。
  - 在 `all` 以外的 `role` 下使用 `stream = hub` 或 `kv = memory`：它們只能在單一行程內運作。
  - 可能並非有意的語音設定，例如沒有 STUN 或 TURN 的 `voice = p2p`。
  - 位於 `http://` 位址，或位於 `https://` 位址但既沒有 token 也沒有用戶端憑證的[外掛](plugins.md)。
  - 付款服務商的測試模式金鑰。
  - 設定了 `origin_secret` 但 `trusted_proxies` 為空，以及輪替後仍保留設定的 `origin_secret_previous`。
  - 使用 `-instance` 時：未設定的實例文字 `name`、`short_description`、`terms` 和 `privacy_policy`，未設定的實例設定 `contact_account`、`contact_email` 和 `security_contact`，以及沒有規則的伺服器。無法連線到資料庫時，這會多算一則警告，其餘的檢查仍會執行。

`my serve` 在啟動時會將相同的檢查結果記錄為警告，但仍會啟動。`development` 設定檔會略過這些檢查；請將它用於本機開發和測試，絕不要用於其他人使用的實例。

請在與伺服器相同的環境（相同的環境變數、密鑰檔案和設定檔）中執行 `my config check`，例如作為部署的一個步驟，在新版本開始接收流量之前執行。

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## 之後無法變更的設定

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### 伺服器的網域與帳號名稱的網域

- **原因**：`host` 是伺服器每個 ActivityPub id 的一部分，而其他伺服器會儲存這些 id。`account_domain` 是帳號名稱（`@alice@example.org`）的網域。第一個使用資料庫的 `my serve` 或 `my` 指令會將兩者記錄到資料庫中，從那之後，只要設定不同，`my serve` 和這些指令都會拒絕執行：在聯邦之後變更，會破壞每一個追蹤，以及每一個對這台伺服器貼文和帳號的參照。請在第一次啟動之前設定好兩者。在聯邦之前發現的錯誤，可以透過[重新建立資料庫](database.md#recreating-erasing-every-row)來修正，但這會清除所有資料列。
- **決定**：帳號名稱要使用伺服器的網域，還是另一個網域（例如帳號名稱使用 `example.org`，伺服器使用 `social.example.org`）。另外的帳號名稱網域必須將其 WebFinger、host-meta 和 NodeInfo 重新導向到伺服器。
- **確認**：`my config check` 會顯示 `host` 和 `account_domain`，並回報其他伺服器無法連到的網域。當它們與資料庫中的值不同時，`my config check -instance` 也會失敗，而且不會記錄任何東西。使用另外的帳號名稱網域時，`curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` 會以 `301` 回應，導向 `https://<host>/.well-known/webfinger?...`。
- **詳細說明**：[執行伺服器](running.md)（聯邦端點，以及從另一個網域提供帳號名稱）、[`host`](../reference/configuration.md#host) 和 [`account_domain`](../reference/configuration.md#account_domain)。

<!-- source: reachable-over-https 1cf53c613f7b -->
### 可透過 `https` 連線

- **原因**：其他伺服器會透過連接埠 443 在 `https://<host>/` 擷取 id，而瀏覽器只會透過 `https` 傳送登入用的 Cookie。
- **確認**：從你的網路外部執行 `curl -sI https://<host>/healthz`，會以對 `host` 有效的憑證回應 `200`，而且 `my config check` 沒有回報關於 `web_origin` 的錯誤。`http://<host>/` 會重新導向到 `https`，這由伺服器前方的反向代理、負載平衡器或 CDN 處理（使用 Docker Compose 時，`compose.caddy.yaml` 層就是這樣的代理）。
- **詳細說明**：[執行伺服器](running.md)、[`web_origin`](../reference/configuration.md#web_origin)。

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### 無法繞過 CDN 連線

- **原因**：前方有 CDN，而伺服器自己的位址仍然公開（Cloud Run 上的 `*.run.app`）時，任何人都可以繞過 WAF 和快取直接呼叫伺服器，而在 CDN 自家平台上的呼叫者還能自行指定速率限制所看到的用戶端 IP。只有在這種情況下才需要：防火牆或平台已經限制誰能連線時不需要，而沒有 CDN 時也就沒有什麼可以繞過。
- **確認**：已設定 `origin_secret` 和 `origin_secret_header`，而且 CDN 會在每個主機名稱上設定該標頭。`curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/` 回應 `403`，其 `/healthz` 回應 `200`，而 `https://<host>/` 回應 `200`。
- **詳細說明**：[拒絕未經過 CDN 的請求](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)，包括所有人都被鎖在外面時如何復原。

<!-- source: secrets ec71f1f20574 -->
## 密鑰

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### 金鑰加密金鑰（KEK）

- **原因**：KEK 會加密每個 actor 的私密金鑰。沒有它，伺服器就無法簽署任何東西，而且這些金鑰也無法從資料庫復原：每個帳號都必須重新建立，其他伺服器也不會再信任它們。
- **確認**：
  - 它是隨機產生的（`openssl rand -base64 32`）；`my config check` 會回報不是隨機產生的金鑰。
  - 它存放在資料庫及其備份以外的地方，例如密碼管理工具或雲端的密鑰儲存服務，而且至少有另一位營運伺服器的人能取得它。
  - 它只透過 `MY_KEK` 或 `MY_KEK_FILE` 傳給伺服器（設定檔會拒絕密鑰）。使用 Docker Compose 時，它是 `secrets/kek` 檔案，而且不在 `.env` 中（[Docker Compose 檔案](running.md#docker-compose-files)）。
- **詳細說明**：[金鑰管理](key-management.md)。

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### Web Push 的 VAPID 金鑰

- **原因**：沒有它，伺服器就不會送出任何 Web Push 通知。之後變更它會終止所有訂閱，所以請只產生一次，並像 KEK 一樣妥善保管。
- **確認**：未設定 `vapid_private_key` 時，`my config check` 會發出警告。請用 `my config vapid-key` 產生。
- **詳細說明**：[金鑰管理](key-management.md)（Web Push 的 VAPID 金鑰）。

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### 其他密鑰不要放進儲存庫和映像檔

- **原因**：放在儲存庫、容器映像檔或設定檔中的密鑰，會被複製到這些東西去的每一個地方。
- **確認**：`database_url`、`mail_smtp_password`、`captcha_secret_key`、付款服務商的金鑰、`valkey_url`，以及設定參考中標示為密鑰的所有其他設定，都來自由平台的密鑰儲存服務（Docker 或 Kubernetes secrets、Secret Manager、Parameter Store）填入的環境變數或 `<NAME>_FILE` 檔案。設定檔不能存放它們：`my config check` 會拒絕設定檔中的密鑰。使用 Docker Compose 時，它們來自只有你能讀取的 `.env`（`chmod 600`）。請在儲存庫和映像檔中搜尋這些值，確認沒有殘留任何副本。`my config check` 對每個已設定的密鑰都會印出 `(redacted)`。
- **詳細說明**：[設定參考](../reference/configuration.md)（Secret 欄）、[CAPTCHA](captcha.md)、[Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md)。

<!-- source: instance-settings baae9cc3e09e -->
## 實例設定

實例設定存放在資料庫中，可以在伺服器執行期間，透過伺服器的管理畫面或 `my instance set` 變更。`my config check -instance` 會針對未設定的文字、聯絡方式和規則發出警告。

- **名稱、簡短說明和說明**：顯示在首頁、資訊頁面（`/about`）、連結預覽和 NodeInfo 中。請在 `/about` 上確認。
- **聯絡方式**：`contact_account`（使用者就伺服器相關事宜聯絡的帳號）和 `contact_email`（使用者和其他伺服器寫信的地址；伺服器寄出的電子郵件也會將回覆送到這裡）。請在 `/about` 上確認。
- **弱點聯絡方式**：`security_contact`，一個 `https:` 或 `mailto:` URI。設定後，伺服器會回應 `/.well-known/security.txt`。請用 `curl -s https://<host>/.well-known/security.txt` 確認。
- **服務條款和隱私權政策**：`terms` 和 `privacy_policy`。隱私權政策必須列出會收到個人資料的服務，例如 CAPTCHA 服務和郵件服務商。請在 `/about` 和註冊畫面上確認連結。
- **伺服器規則**：使用者註冊時同意的內容，也是版主執行的依據。請用 `my instance rule create` 或在管理畫面中新增，並在 `/about` 上確認。
- **註冊模式**：在你變更之前，`registration_mode` 為 `approval`；請依照你希望大家加入的方式，選擇 `open`、`approval`、`invite` 或 `closed`。使用 `mail = none` 時，`open` 的註冊也會等待版主核准。
- **搜尋引擎與 AI**：在你允許之前，`search_indexing`、`ai_usage` 和 `indexable_default` 會拒絕兩者。請確認 `/robots.txt` 和 `/.well-known/tdmrep.json`。
- **狀態頁面**：將 [`status_page_url`](../reference/configuration.md#status_page_url)（是一般設定而不是實例設定，所以維護期間也會顯示連結）設定為架設在伺服器以外服務上的狀態頁面。

**詳細說明**：[實例設定](instance-settings.md)、[註冊](registration.md)、[搜尋引擎與 AI 爬蟲](crawlers.md)、[CAPTCHA](captcha.md)（隱私權政策中要寫什麼）、[隱私權政策範本](privacy-policy-template.md)、[處理活動紀錄範本](processing-record-template.md)、[監控](monitoring.md)（狀態頁面）。

<!-- source: email 40c18999c46f -->
## 電子郵件

- **原因**：電子郵件用來驗證地址，並寄送密碼重設和通知。使用 `mail = none` 時，使用者不會收到任何電子郵件。
- **確認**：
  - `mail` 為 `smtp`、`ses` 或 `plugin`，而 `mail_from` 是你所掌控網域上的地址；`my config check` 會針對 `mail = none` 發出警告。
  - `mail_from` 的網域有 SPF、DKIM 和 DMARC 記錄，而且服務商會以 DKIM 簽署。沒有這些，郵件會被歸到垃圾郵件。
  - 以測試帳號註冊並收到驗證郵件；申請密碼重設並同樣收到郵件。檢查收到的郵件標頭中是否有 `spf=pass`、`dkim=pass` 和 `dmarc=pass`。
  - 使用 SES 時，帳號已脫離沙箱，而且退信和投訴通知會送達伺服器。
- **詳細說明**：[電子郵件](email.md)、[註冊](registration.md)（電子郵件驗證）。

<!-- source: administrators 2c05e6644d88 -->
## 管理員

- **原因**：管理員的帳號可以變更所有設定、讀取私人資料，並將任何人停權。
- **確認**：
  - 每位管理員都已開啟兩步驟驗證（網頁用戶端中的「設定」→「兩步驟驗證」），並保管好復原碼。
  - 只有需要的人才擁有 `admin` 角色；版主擁有 `moderator`，或只含他們會用到之權限的角色。請用 `my admin role` 列出，並在管理畫面中檢視。
- **詳細說明**：[管理員與角色](admin-cli.md)、[審核](moderation.md)（權限和登入復原）。

<!-- source: operations 56bd643c2241 -->
## 營運

- **備份與復原**：資料庫、blob 儲存區（`blob_fs_dir` 或 bucket）和 KEK 都依排程備份到與伺服器分開的地方，而且已經演練過還原到全新的伺服器：還原後的伺服器能啟動，帳號也能登入和發文。從未還原過的備份，不能確定它可用。資料庫和媒體不得放在暫存目錄中；放在那裡時 `my config check` 會發出警告。請參閱[金鑰管理](key-management.md)（備份 KEK）和[資料庫](database.md)。
- **監控與警示**：外部監控會檢查 `/healthz`，並通知能夠處理的人，而且有人在留意任務佇列中的 dead 任務。請參閱[監控](monitoring.md)和[執行伺服器](running.md)（健康檢查、指標和任務佇列）。
- **映像檔**：實例執行的是以 digest 固定的正式發布映像檔（`ghcr.io/mycroblog/my:0.1.0@sha256:...`，取自該版本的發布說明），而不是重新啟動時會改變的映像檔，而且[已驗證其來源](running.md#verify-an-image)。沒有 `latest` 標籤。升級時會刻意變更該版本，並依照[升級](database.md#upgrading)的步驟，一次升一個次要版本。
- **原始碼（AGPL-3.0）**：伺服器必須向所有使用者提供其原始碼。未修改的正式版本，請將 [`source_url`](../reference/configuration.md#source_url) 保持預設值；執行分支或修改版本時，請將它指向你自己的原始碼。請在 `/about` 上確認連結。請參閱[執行伺服器](running.md)（公開原始碼）。
- **維護**：在需要之前，就先了解如何讓伺服器進入維護模式。請參閱[維護](maintenance.md)。

<!-- source: federation a5ab186ec04b -->
## 聯邦

- **原因**：其他伺服器會透過 WebFinger 和 NodeInfo 找到帳號，然後擷取 actor。只要其中任何一項從外部無法運作，聯邦宇宙就看不到這台伺服器。
- 從你的網路外部的機器**確認**：
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` 會回傳該帳號；網域不同時，請跟隨到 `host` 的重新導向（`curl -L`）。
  - `curl -s https://<host>/.well-known/nodeinfo` 會連結到 `/nodeinfo/2.1`，其中列出軟體名稱及其原始碼。
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` 會重新導向到 actor 的 id，而該 id 上的 actor 是帶有 `publicKey` 的 JSON。
  - 從 Mastodon 或其他伺服器上的帳號搜尋 `@<user>@<account_domain>`、追蹤它，並看到新貼文送達。
  - 伺服器前方的 CDN 不會快取 `/ap/*` 和 `/@*`，除非它會遵守 `Vary: Accept`。
- **詳細說明**：[執行伺服器](running.md)（聯邦端點）、[CDN](cdn.md)、[中繼站](relays.md)。
