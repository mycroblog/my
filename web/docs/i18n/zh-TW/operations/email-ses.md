<!-- source: sending-email-through-amazon-ses 6deee2054246 -->
# 透過 Amazon SES 寄送電子郵件

如何讓 Amazon SES 準備好寄送伺服器的電子郵件：用 DNS 紀錄驗證寄件者的網域、離開新帳號一開始所在的沙盒，並讓帳號維持良好的狀態。OpenTofu 模組 [`mycroblog-aws`](deploy-aws.md#email-through-amazon-ses) 預設透過 SES 寄送，並建立身分與權限；紀錄和正式環境存取權的申請則是你自己要做的步驟。設定本身列在[寄送電子郵件](email.md#amazon-ses-api)。

**AWS 部署從未被實際套用過**，因此這些步驟來自 AWS 的文件，尚未在本專案的伺服器上完整照做過。

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. 加入網域的紀錄

SES 從已驗證的身分寄送。`mycroblog-aws` 會在部署所在的區域，為 `mail.from` 的網域（`noreply@social.example` 的話就是網域 `social.example`）建立身分，並啟用 Easy DKIM。套用之後，輸出 `dns_records` 會列出要在你的 DNS 服務供應商加入的紀錄，以及 `host` 的紀錄：

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

這三筆 CNAME 紀錄會公開 SES 簽署時使用的金鑰。SES 會自行檢查，一旦解析成功就將身分標為已驗證，通常在一小時內，最久不超過 72 小時；在那之前它會拒絕寄送。SES 主控台的 **Identities** 下會顯示身分的狀態。

也請加入 SPF 和 DMARC 紀錄，收件端除了 DKIM 之外也會檢查它們：[SPF、DKIM 與 DMARC](email.md#spf-dkim-and-dmarc) 說明了各自要放什麼。SES 簽署的 DKIM 與 `mail.from` 的網域對齊，所以光靠這些紀錄 DMARC 就會通過；也對齊 SPF 的自訂 MAIL FROM 網域則是選用的。

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. 離開沙盒

每個新帳號一開始都在 SES 沙盒中，且每個區域各自獨立。在沙盒中，SES 只會寄送給你自己驗證過的地址和網域，且 24 小時內最多 200 封、每秒一封。沙盒中的伺服器可以寄測試郵件給自己的營運者，但其他人的註冊會失敗。

若要寄送給所有人，請為部署所在的區域申請正式環境存取權：

1. 在該區域開啟 SES 主控台，於 **Account dashboard** 選擇 **Request production access**（或 **Get set up**）。
2. 郵件類型選擇 **Transactional**，填入伺服器的 URL，並說明它會寄送什麼：註冊與新地址的確認、密碼重設，以及各使用者自行開啟、也可以用一鍵取消訂閱關閉的通知。
3. 說明你如何處理退信和投訴：一旦下文的通知送達，伺服器就會停止寄送給退信或投訴的地址。

AWS 會以電子郵件回覆，通常在一天內，也可能在核准之前要求更多資訊。核准後，主控台會顯示你的寄送配額，並隨著你的使用而成長。

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. 留意退信與投訴

SES 會監看退信或被投訴的郵件所占的比例，並對比率持續偏高的帳號先進行審查，再暫停。伺服器是透過 Amazon SNS 從 SES 的通知得知退信或投訴，並停止寄送給該地址。`mycroblog-aws` 不會建立這個主題：請依[接收 SES 通知](email.md#receive-ses-notifications-sns)的說明設定，並透過模組的 `settings` 把它的 ARN 交給伺服器：

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

SES 主控台的 **Reputation metrics** 會顯示帳號的退信率和投訴率。

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## 從其他區域寄送

模組會在部署所在的區域建立身分，伺服器也在那裡寄送。若要透過另一個區域的 SES，或改用 SMTP 服務寄送，請設定 `mail = { kind = "smtp", ... }`，並使用該服務的 SMTP 介面：設定請見 [Amazon SES（SMTP 介面）](email.md#amazon-ses-smtp-interface)。
