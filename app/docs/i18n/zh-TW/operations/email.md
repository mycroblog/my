<!-- source: sending-email 941ecea58cd9 -->
# 寄送電子郵件

本頁說明伺服器會寄送哪些電子郵件、如何設定寄送方式，以及如何處理寄送失敗和取消訂閱。設定項目列在[設定參考](../reference/configuration.md#mail)中。電子郵件確認和密碼重設在使用者端的流程，請參閱[註冊與電子郵件確認](registration.md)。

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## 寄送哪些郵件，以及如何設定

伺服器永遠不會在請求的處理中寄送電子郵件。它會在觸發郵件的事實所在的同一個交易中，為每種用途將一個任務加入佇列，再由任務 worker 寄出。寄送失敗不會反映在註冊或重設的回應中：使用者會看到「我們已寄出確認電子郵件」，如果沒有收到，可以再要求一次。

| 類別 | 電子郵件 | 任務類型 | 使用者可以取消訂閱嗎？ |
| --- | --- | --- | --- |
| 交易型（`transactional`） | 電子郵件地址確認 | `identity.verification_email` | 否 |
| 交易型 | 密碼重設 | `identity.password_reset_email` | 否 |
| 交易型 | 密碼變更或刪除的通知 | `identity.password_notice_email` | 否 |
| 交易型 | 帳號停用的通知，附上刪除日期 | `identity.deactivation_email` | 否 |
| 交易型 | 服務終止（公告、日期變更、取消、關閉日前 7 天和前 1 天、關閉，以及匯出期限前 7 天） | `identity.closure_email` | 否 |
| 通知型（`notification`） | 給版主的通知（預設開啟，依通知種類設定） | `notification.moderator_email` | 是 |
| 通知型（`notification`） | 15 分鐘仍未讀的通知：提及、追蹤、轉發和其他使用者的其他通知，以及版主的通知（預設關閉，依種類設定） | `notification.email` | 是 |

給支持者的通知郵件（`notification.supporter_email`）也透過同一個任務佇列寄送。

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### 哪些通知會寄送電子郵件

每位使用者可以在設定的 **通知** 頁面（網頁用戶端的 `/settings/notifications`），依通知種類選擇是否要在應用程式中接收，以及是否要以電子郵件接收。只有在伺服器會寄送電子郵件時，那裡才會出現「電子郵件」欄。在應用程式中關閉的種類根本不會建立，因此也不會寄送電子郵件或推播。

| 種類 | 預設寄送電子郵件 | 寄送時機 |
| --- | --- | --- |
| 其他使用者的通知（提及、新追蹤者、追蹤請求、追蹤請求被接受、轉發、喜歡、表情符號回應、引用、投票結束、你追蹤的帳號搬家、語音聊天） | 關閉 | 通知發生 15 分鐘後，僅在它仍未讀（未標為已讀、未關閉）且仍可看到，而使用者仍設定以電子郵件接收該種類時才寄送。每則通知一封郵件 |
| 版主的通知（對帳號的審核處置、檢舉處理完畢、申訴有了決定、因停權而失去的追蹤、自訂表情符號申請有了決定） | 關閉 | 同上 |
| 給版主的通知（新的檢舉、等待核准的註冊、申訴、已停用的 webhook、自訂表情符號申請、受限帳號搬家、關閉日已過、新的與重複發生的錯誤），寄給角色具備相應權限的人 | 開啟 | 立即寄送，不等待已讀 |
| 服務終止，以及支持者方案的通知 | 一律 | 立即寄送；無法關閉 |

通知郵件只會寄到使用者已確認的主要地址，且永遠不會寄到郵件已被停止的地址（見下文）。其他使用者通知的郵件主旨與其 Web Push 訊息的文字相同，郵件會連結到貼文、個人檔案或通知頁面。

服務終止郵件會寄給每一位主要地址已確認的使用者。漏收一封可能導致資料遺失，因此它們以交易型郵件寄送，不帶取消訂閱連結，也不會因垃圾郵件投訴而停止（硬退信則會停止）。公告、日期變更、取消和關閉，各自會在其紀錄所在的同一個交易中將 `identity.closure_email_fanout` 加入佇列。每小時執行的 `identity.queue_closure_reminders` 會決定何時寄送關閉日前 7 天、前 1 天，以及匯出期限前 7 天的提醒。公告服務終止時已經過了時間的提醒不會寄送，因為公告郵件已寫明這些日期。分送任務會為每位使用者在 `service_closure_emails` 記錄一列，並且只為成功記錄的使用者將 `identity.closure_email` 加入佇列，因此不論這個任務執行幾次，每位使用者每封郵件都只會收到一次。寄送之前，任務會確認郵件仍然有效（不是已取消的公告，也不是之後已變更的日期）。

設定 `mail` 決定電子郵件的寄送方式。

| `mail` | 行為 |
| --- | --- |
| `none`（預設） | 不寄送任何郵件，並將 `mail not sent (mail = none)` 連同訊息類別寫入伺服器日誌。使用 `profile = development` 時，這一行還會包含地址、主旨和內文（含連結）。適用於不使用電子郵件的架構，以及開發 |
| `smtp` | 以 SMTP submission 寄送。必須使用 STARTTLS，並在驗證身分和寄送前驗證憑證 |
| `ses` | 以 Amazon SES v2 API（使用原始內容的 `SendEmail`）寄送。透過 AWS SDK 的預設憑證鏈（例如 IAM 角色）驗證身分，不保存 SMTP 密碼 |
| `plugin` | 將每則訊息交給你執行的[外掛](#a-plugin-mail--plugin)，適用於只提供 HTTP API 的郵件服務 |

`smtp` 的設定為 `mail_from`、`mail_smtp_host`、`mail_smtp_port`（預設 587）、`mail_smtp_username` 和 `mail_smtp_password`。`ses` 的設定為 `mail_from`、`mail_ses_region` 和 `mail_ses_configuration_set`。`plugin` 的設定為 `mail_from` 和 `plugin_mailer_address`，以及[其目的地](plugins.md#the-destination)的其餘設定。`mail_from` 是單純的地址，例如 `noreply@social.example`。`From` 會成為 `"<server name>" <mail_from>`，而 `mail_from` 的網域也是 `Message-ID` 的網域，以及 SPF、DKIM 和 DMARC 所套用的網域（見下文）。設定了實例設定 `contact_email` 時，`Reply-To` 就是它（[實例設定](instance-settings.md)）。每封郵件都帶有 `Auto-Submitted: auto-generated`。`smtp` 和 `ses` 會建立相同的訊息。不使用 SMTPUTF8，因此地址以 ASCII 寄送，國際化網域會轉換為 Punycode。本地部分（`@` 之前）不是 ASCII 的地址無法寄送，其任務會失敗。

<!-- source: examples-by-provider 7ab129a02c43 -->
## 各服務供應商的範例

請以環境變數或檔案（`MY_MAIL_SMTP_PASSWORD` 或 `MY_MAIL_SMTP_PASSWORD_FILE`）傳入機密。不要寫在設定檔中。

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES（API）

在 AWS 上執行時，`mail = ses` 會使用 SES v2 API。憑證來自 AWS SDK 的預設憑證鏈（ECS 的任務角色、環境變數、共用設定檔），因此沒有需要設定的機密。

1. 在 SES 中將 `mail_from` 的網域驗證為身分（identity），並啟用 Easy DKIM（見下文）。
2. 申請正式環境存取權以離開沙盒。在沙盒中只能寄送給已驗證的地址。
3. 允許執行角色對 `mail_from` 的身分（若使用 configuration set，也包括該 configuration set）執行 `ses:SendEmail`。

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

設定 `mail_ses_configuration_set` 時，每封郵件都會使用該 configuration set。SES 會以自己的值覆寫郵件的 `Message-ID` 和 `Date`。SES 永遠不會在寄送當下同步拒絕收件者。它會接受寄送到帳號隱藏清單（account suppression list）上地址的郵件，然後不投遞，退信則會稍後以通知（SNS）送達。若要記錄退信和投訴，請設定下文的 SES 通知，讓它們透過 SNS 送達。

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### 外掛（`mail = plugin`）

對於只提供 HTTP API 的郵件服務，請執行種類為 `mailer` 的[外掛](plugins.md)，由它呼叫該服務。

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

使用這個值時必須設定 `plugin_mailer_address`。訊息可能包含機密（例如驗證連結），因此這個種類會拒絕 `http://`，即使是 loopback 也一樣：請使用 Unix socket 或 `https://`。伺服器啟動時不會呼叫外掛，即使外掛停止運作也能啟動，郵件會在任務中等待。

使用預設的 `plugin_mailer_protocol = json` 時，外掛只需要一個 HTTP 伺服器和一個 JSON 函式庫（[不使用函式庫的外掛](plugins.md#a-plugin-with-no-library)）。`Send` 最長 30 秒，或依 `plugin_mailer_timeout` 而定，須少於 5 分鐘。請求最大 2 MiB，回應最大 64 KiB。

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

伺服器在呼叫外掛之前會檢查訊息，因此無法透過這些欄位注入標頭。請求包含訊息的各個部分，由外掛組成其服務所需的格式：

- **外掛要加上三個標頭**：每則訊息唯一、位於 `envelopeFrom` 網域上的 `Message-ID`；`Date`；以及防止自動回覆形成迴圈的 `Auto-Submitted: auto-generated`。如果服務會自行寫入 `Message-ID` 和 `Date`，這兩個就由服務負責。
- `From` 由 `fromName` 和 `envelopeFrom` 組成。`fromName`、`replyTo`、`html` 和 `headers` 可能不存在。`headers` 只包含兩個 `List-Unsubscribe` 標頭。
- 地址為 UTF-8。如果外掛的服務需要 ASCII，外掛要將網域轉換為 Punycode，對於無法寄送的本地部分則回應 `OUTCOME_INVALID`。
- 由於失敗的任務會重試，同一則訊息可能會送來兩次。不要將訊息寫入日誌。

`outcome` 說明訊息的處理結果：

| `outcome` | 意義 | 伺服器 |
| --- | --- | --- |
| `OUTCOME_SENT` | 服務已接收訊息 | 完成任務 |
| `OUTCOME_INVALID` | 無法以目前的內容寄送 | 立即讓任務失敗 |
| `OUTCOME_RECIPIENT_REJECTED` | 地址不存在 | 立即讓任務失敗，並[停止寄送到該地址](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | 服務因其他原因永久拒絕 | 立即讓任務失敗 |
| `OUTCOME_TEMPORARY` | 服務暫時拒絕，或沒有回應 | 重試任務 |
| 其他任何值，或沒有值 | | 重試任務 |

HTTP 錯誤，或 Connect 的 `{"code":"unavailable"}` 加上相應的狀態，也屬於暫時性失敗。`enhancedStatus` 是選填的：拒絕的增強狀態碼，例如 `5.1.1`。伺服器會保留拒絕的 5 類代碼和 `OUTCOME_TEMPORARY` 的 4 類代碼，其他一律忽略。它不會保留外掛的任何文字，因此不要在這裡放原因：原因應該寫在外掛自己的日誌中。

伺服器只能從這個回應得知退信。它沒有接收外掛服務後續退信和投訴的端點，因此服務之後才回報的退信不會停止寄送到該地址。

請用 `my` 執行檔測試外掛：

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

第一個指令只確認外掛有回應，不會寄送任何郵件。第二個指令會寄送一則真正的訊息到 `--mail-to`，並向外掛送出一個沒有收件者的請求，外掛必須拒絕它。外掛收到訊息後伺服器就看不到它了，因此請查看實際送達的郵件：它的標頭必須包含 `Message-ID`、`Date` 和 `Auto-Submitted: auto-generated`。請參閱[測試外掛](plugins.md#test-a-plugin)。

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### 接收 SES 通知（SNS）

SES 的退信和投訴會送到 SNS 主題，再由該主題上的 HTTPS 訂閱送到伺服器的 `POST /mail/events/ses`。只有在設定了 `mail_ses_sns_topic_arn` 時，這個端點才會開放；否則它會回應 `404`。

1. 在與 SES 身分相同的區域建立標準 SNS 主題。
2. 將主題屬性 `SignatureVersion` 設為 `2`（SHA256withRSA）。不接受預設值 `1` 的簽章，伺服器會回應 `400`。
3. 在主題的存取政策中，允許 `ses.amazonaws.com` 從你自己的帳號執行 `SNS:Publish`（SES 主控台建立主題時會自動加上）。
4. 用以下任一方式讓 SES 通知指向這個主題（伺服器會讀取內文中的 `notificationType` 和 `eventType` 兩者）。
   - 身分通知：將這個主題用於 `mail_from` 身分的「Bounce」和「Complaint」回饋通知。
   - configuration set 的事件目的地：將這個主題加入 `mail_ses_configuration_set` 中 configuration set 的目的地，事件類型為 `BOUNCE` 和 `COMPLAINT`。
5. 將 `MY_MAIL_SES_SNS_TOPIC_ARN` 設為主題的 ARN，用 `my config check` 確認後重新啟動。
6. 在主題上建立協定為 `https`、端點為 `https://<server host>/mail/events/ses` 的訂閱。**不要啟用 raw message delivery**（沒有簽章的內文會被拒絕）。

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

伺服器收到訂閱確認（`SubscriptionConfirmation`）時，會向其 `SubscribeURL` 送出 `GET` 以確認訂閱，並記錄 `SNS subscription of the SES notifications confirmed`。請用 `aws sns list-subscriptions-by-topic` 確認訂閱已不再是 `PendingConfirmation`。

端點在使用訊息之前，會檢查以下所有項目。只要有任何一項不符合，就會回應 `400` 且不記錄任何內容。

- `TopicArn` 等於 `mail_ses_sns_topic_arn`。
- `SignatureVersion` 為 `2`，且簽章有效。
- 簽章的憑證 URL（`SigningCertURL`）為 `https`，且其主機是主題所在區域的 `sns.<region>.amazonaws.com`。訂閱確認的 `SubscribeURL` 也必須是相同的主機。

憑證是透過防範 SSRF 的對外 HTTP 用戶端取得，並在 KV 儲存中快取 24 小時。內文最多讀取 1 MiB，超過的會收到 `413`。當無法取得憑證、訂閱確認失敗，或無法寫入紀錄時，端點會回應 `503`，交由 SNS 重試。維護模式期間也會回應 `503`。

以下是它會記錄的通知。它會將收件地址與主要地址比對（不分大小寫，Punycode 網域也會與其 Unicode 形式相符）；沒有相符時，不做任何事。

| 通知 | 記錄為 |
| --- | --- |
| `bounceType` 為 `Permanent` 的退信（任何子類型，例如 `General`、`NoEmail`、`Suppressed` 或 `OnAccountSuppressionList`） | `hard_bounce` |
| 投訴（`complaintFeedbackType` 為 `not-spam` 者除外） | `complaint` |
| `Transient` 或 `Undetermined` 的退信，以及投遞或寄送等其他通知 | 不記錄（回應 `200`） |

SES 的 `feedbackId` 是紀錄的唯一鍵，因此不論 SNS 送了幾次相同的通知，都只會有一筆紀錄。

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES（SMTP 介面）

無法提供 IAM 憑證時（例如在 AWS 以外執行），請以 `mail = smtp` 使用 SES 的 SMTP 介面。在這種架構下，只有寄送當下對收件者的拒絕會被記錄為停止寄送（[停止寄送到無法收信的地址](#suppression-of-addresses-that-cannot-receive-email)）。

1. 在 SES 中將 `mail_from` 的網域驗證為身分，並啟用 Easy DKIM（見下文）。
2. 申請正式環境存取權以離開沙盒。在沙盒中只能寄送給已驗證的地址。
3. 在 SES 的「SMTP settings」中建立 SMTP 憑證（與 IAM 存取金鑰不同）。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES 會以自己的值覆寫 `Message-ID` 和 `Date`。

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### 一般的 SMTP submission 伺服器

適用於你自己的 SMTP 伺服器（例如 Postfix），或電子郵件投遞服務供應商的 submission 伺服器。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

伺服器不會寄送給不提供 STARTTLS，或其憑證無法針對 `mail_smtp_host` 中的名稱驗證的伺服器。憑證資訊只會在 STARTTLS 之後送出。變更設定後，請用 `my config check` 確認並重新啟動。

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF、DKIM 與 DMARC

Gmail 和 Yahoo 要求每天寄送大量郵件的寄件者設定 SPF、DKIM、DMARC 和一鍵取消訂閱。即使寄送量很少，沒有這些設定的郵件也比較容易被歸類為垃圾郵件。請在 `mail_from` 網域（例如 `social.example`）的 DNS 中，依服務供應商提供的值設定所有項目。

- **SPF**：在網域上放一筆 TXT 紀錄，授權你用來寄送的服務供應商（例如使用 SES 的自訂 MAIL FROM 網域時，就在該網域上放服務供應商指定的 `include:`）。
- **DKIM**：將服務供應商發行的公開金鑰放到 DNS，並在寄送端啟用簽署。SES 的話是 Easy DKIM 的三筆 CNAME 紀錄。伺服器本身不簽署，由服務供應商簽署。
- **DMARC**：位於 `_dmarc.social.example` 的 TXT 紀錄。**先從 `p=none` 開始**，用彙總報告（`rua=`）確認 SPF 和 DKIM 已對齊（與 From 網域相符），再逐步收緊為 `quarantine` 和 `reject`。

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

要在 SES 上對齊 SPF，請設定自訂 MAIL FROM 網域（預設的 MAIL FROM 是 `amazonses.com`，與 From 網域不符）。只要 DKIM 對齊，DMARC 就會通過。

<!-- source: look-at-send-failures ab9c4f356d3b -->
## 查看寄送失敗

請在任務頁面（`/admin/jobs/list`，權限 `manage_jobs`；請參閱[任務佇列與失敗的任務](running.md#the-job-queue-and-failed-dead-jobs)）查看寄送失敗。依任務類型篩選，就能只看電子郵件任務（在管理 API 中為 `GET /api/v1/admin/jobs?type=identity.verification_email`）。類型列在上方的表格中。

- **暫時性失敗**（連線錯誤、逾時、4xx 回應）會重試。交易型郵件會嘗試 6 次（從 30 秒開始的指數退避，總共約 15 分鐘）；通知型郵件使用預設策略（10 次，約 4 小時）。嘗試次數用完後，任務會成為失敗的任務（dead job）。錯誤的 `mail_smtp_host`、驗證失敗和被封鎖的網路都會出現在這裡。修正原因後，請用 `my job retry <id>` 或在任務畫面上重新執行。
- **永久拒絕**（針對收件者或內容的 5xx）不會重試，會立即失敗。
- 使用 `mail = ses` 時，SES 的 `MessageRejected`（內容被拒絕）屬於永久拒絕。`TooManyRequestsException`（寄送速率限制）、`SendingPausedException`、`MailFromDomainNotVerifiedException`、`AccountSuspendedException`，以及沒有送達 SES 的請求屬於暫時性失敗，修正帳號或權限後，重試就會成功。錯誤文字中只會保留 SES 的錯誤代碼。
- 使用 `mail = plugin` 時，由回應的 [`outcome`](#a-plugin-mail--plugin) 決定。無法連線或回應錯誤的外掛屬於暫時性失敗，並計入 [`my.plugin.failures`](running.md)。錯誤文字中只會保留結果和增強狀態碼，因此失敗的原因要看外掛的日誌。
- 從請求起算超過時間限制（重設為 15 分鐘，確認為 1 小時）的交易型郵件，會在不寄送的情況下完成。這是為了避免舊的連結延遲送達；不算失敗。
- 任務執行時如果沒有寄送的理由（地址已停止寄送、在這段期間已完成確認，或使用者失去了該通知的權限或設定），就會在不寄送的情況下完成。

任務的 payload 不包含電子郵件地址、內文或權杖，錯誤文字也一樣。只會留下 ID、任務類型和 SMTP 回應碼。成功的寄送可以從每次執行的結構化 `job` 日誌行（`my.job.type`）追蹤。

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## 停止寄送到無法收信的地址

當 SMTP 伺服器表示收件地址不存在（對 `RCPT TO` 回應 5xx，且擴充狀態為 `5.1.x`）時，伺服器會將該地址記錄為硬退信，並停止寄送給它。使用 `mail = plugin` 時，`OUTCOME_RECIPIENT_REJECTED` 會記錄同樣的硬退信，其他情況則不會記錄。使用 `mail = smtp` 時，只會記錄這種寄送當下的拒絕。SMTP 伺服器接受郵件之後才退回的非同步退信，以及投訴回饋迴圈，都不會被接收。使用 `mail = ses` 時，SES 永遠不會同步拒絕收件者，因此寄送當下不會記錄任何內容；永久退信和投訴是從 SES 通知記錄的（[接收 SES 通知（SNS）](#receive-ses-notifications-sns)）。

| 種類 | 停止寄送的郵件 |
| --- | --- |
| `hard_bounce` | 確認郵件以外的所有郵件。密碼重設郵件也會停止 |
| `complaint`（被回報為垃圾郵件） | 通知和公告郵件。交易型郵件仍會寄送 |

- 版主可以在管理畫面中帳號的管理頁面上看到停止寄送的種類和來源（`GET /api/v1/admin/accounts/{id}` 中的 `email_suppression`）。
- **解除**的方式有兩種。當使用者要求確認郵件並再次確認地址時，硬退信造成的停止寄送就會解除（即使地址處於停止寄送狀態，確認郵件仍會寄出）。使用者可以透過用戶端 API（`POST /api/v1/accounts/email/verification`，每小時最多 3 次）或在登入狀態頁面上提出要求。當你確認可以寄送時（例如信箱已修好），版主可以用 `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（權限 `manage_accounts`）解除。這會在稽核日誌中記錄 `email_suppression_lifted`（[審核](moderation.md)）。
- 對於密碼重設郵件被停止寄送的使用者，管理員可以發行重設連結並交給對方（`my admin account password-reset-link @alice`）。
- 使用者會在設定畫面上看到郵件無法寄達其主要地址，以及從何時開始。

<!-- source: unsubscribing 2806dbccc409 -->
## 取消訂閱

通知郵件帶有取消訂閱連結，以及 `List-Unsubscribe` 和 `List-Unsubscribe-Post: List-Unsubscribe=One-Click` 標頭（RFC 8058）。電子郵件用戶端的「取消訂閱」按鈕會使用它們。

- 取消訂閱的類別與通知設定中使用者可以選擇是否寄送電子郵件的各列一一對應，例如 `mention` 或 `admin_report`（給版主的通知每種一個，版主的通知則為 `moderation`）。
- 連結為 `https://{host}/mail/unsubscribe?token=...`，不需要登入。權杖依每封郵件發行，資料庫中只保存其雜湊值，90 天後失效。
  - `GET` 只會顯示確認畫面，不會變更任何內容（以免預先載入連結的掃描程式停止郵件寄送）。按鈕會送出相同的 `POST`。
  - `POST`（一鍵）會關閉通知設定中該列的電子郵件，僅此而已。在權杖失效之前，再次送出會得到相同的結果。
- **交易型郵件無法取消訂閱**。取消確認、重設和密碼變更通知，會使與安全性相關的通知無法送達。這些郵件也不帶這些標頭。
- 使用者也可以在設定的 **通知** 頁面切換相同的設定。

<!-- source: the-html-version 2ca008b1a48a -->
## HTML 版本

每封郵件都是 `multipart/alternative`，以 `text/plain` 和 `text/html`（皆為 UTF-8）提供相同的內容。在 `development` 設定檔中使用 `mail = none` 時，只會將文字版本寫入日誌。

- 有一個外層版面，包含段落、按鈕和署名（伺服器名稱、URL、聯絡方式、寄送這封郵件的原因，以及取消訂閱連結）。`<html lang dir>` 來自收件者的語系，`ar` 等語言會以由右至左的方式排版。
- 不會載入外部字型或樣式表。顏色使用預設的設計 token，淺色和深色依照 `color-scheme` 和 `prefers-color-scheme`。管理員的主題覆寫不會影響郵件。
- 不會載入圖片，但設定了實例自己的標誌時例外（[標誌與圖示](instance-settings.md#logo-and-icon)）：這時 HTML 版本會以標誌開頭（`https://<host>/brand/...`，在電子郵件用戶端採用深色配色時改用深色標誌），每位收件者都是相同的 URL，因此不會透露是誰開啟了郵件。不載入外部圖片的用戶端會改為顯示伺服器名稱（圖片的替代文字）。沒有標誌，或伺服器 URL 不是 `https` 時，郵件不含任何圖片。
- 只有 `https` URL 會變成連結。URL 為 `http` 時（例如 `web_origin` 為 `http://{host}` 的開發環境），URL 會以文字呈現，而不是按鈕或取消訂閱連結，署名中的伺服器 URL 也會省略。
- 版面是執行檔的一部分，因此伺服器在執行時不會執行任何 JavaScript。
