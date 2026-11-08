<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# CAPTCHA 機器人防護（Turnstile / hCaptcha / reCAPTCHA）

CAPTCHA 預設為關閉：設定 [`captcha`](../reference/configuration.md#captcha) 為 `none`。此時登入、註冊和密碼重設頁面不需要 JavaScript 就能運作，也不會傳送任何資料給第三方。選擇 CAPTCHA 服務後，該服務的小工具會出現在三個表單上：登入（`/auth/sign_in`）、註冊（`/auth/sign_up`），以及申請密碼重設連結（`/auth/forgot_password`，僅限會寄送郵件的伺服器）。伺服器會透過服務的 siteverify API 驗證小工具產生的 token。你無法只保護其中部分表單。

<!-- source: choosing-a-service 22ddd3cc639f -->
## 選擇服務

| `captcha` | 服務 | 費用 | 使用者要做什麼 |
| --- | --- | --- | --- |
| `turnstile`（建議） | Cloudflare Turnstile | 免費，無上限 | 通常不需要做任何事；只在必要時出現核取方塊 |
| `hcaptcha` | hCaptcha | 有免費方案 | 核取方塊，必要時再加上圖片挑戰 |
| `recaptcha` | Google reCAPTCHA v2（核取方塊） | 每月 10,000 次評估以內免費（見下文） | 核取方塊，必要時再加上圖片或語音挑戰 |

不支援 reCAPTCHA v3（僅分數）和 reCAPTCHA Enterprise。

<!-- source: creating-keys 38cd93394638 -->
## 建立金鑰

每個服務都會給你一組金鑰：公開的**網站金鑰**（site key），以及只有伺服器持有的**密鑰**（secret key）。請將設定 `host` 的主機名稱（不含連接埠）註冊為網域。當 siteverify 回應中的主機名稱與 `host` 不符時，伺服器會拒絕該 token。

- **Turnstile**：在 Cloudflare 主控台的「Turnstile」下新增小工具，並註冊主機名稱。小工具模式請選擇「Managed」。
- **hCaptcha**：在 hCaptcha 主控台新增網站，並註冊主機名稱。密鑰位於帳號設定中。
- **reCAPTCHA**：在 Google reCAPTCHA 管理控制台中，選擇類型「Challenge (v2)」並搭配「I'm not a robot」核取方塊，然後註冊網域。金鑰屬於一個 Google Cloud 專案。

每個服務的測試金鑰（一定會成功的金鑰）很適合用來確認小工具會出現並產生 token。不過，使用測試金鑰時 siteverify 會回傳固定的主機名稱（Turnstile 為 `example.com`，hCaptcha 為 `dummy-key-pass` 等），所以在這台伺服器上登入和註冊會以「請再檢查一次」失敗。

<!-- source: configuring 3301db5c6286 -->
## 設定

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- 密鑰只能透過環境變數 `MY_CAPTCHA_SECRET_KEY`，或透過指向檔案的 `MY_CAPTCHA_SECRET_KEY_FILE` 傳入。日誌中會顯示 `[REDACTED]`。
- 如果你選擇了某個 `captcha` 卻缺少金鑰，`my config check` 會回報問題，伺服器也不會啟動。
- 伺服器在啟動時不會試用金鑰。當服務拒絕密鑰或網站金鑰時，第一次驗證會記錄 ERROR「the CAPTCHA service refused the secret key or the site key」。在你修正之前，請求會依照下方「服務無法連線時」的方式處理。
- 變更會在重新啟動伺服器時生效。

<!-- source: how-verification-works c5cc12dd3769 -->
## 驗證的運作方式

- 順序依序為速率限制、CAPTCHA 驗證，然後是密碼檢查、註冊處理或寄送重設郵件。超過速率限制的請求絕不會送到服務。
- 使用有效邀請碼的註冊也會經過驗證。
- 驗證失敗的登入，會算作該用戶端 IP 位址的一次登入失敗。它不會計入帳號，所以沒有人能把別人鎖在外面。驗證失敗的註冊會算作一次註冊嘗試（請參閱[註冊](registration.md)中的速率限制）。申請重設連結也同樣會算作一次申請嘗試。
- 對 Turnstile 而言，小工具帶有動作名稱（`sign_in` / `sign_up` / `password_reset`），伺服器會檢查回應中的動作是否相同。某個表單的 token 無法用在另一個表單上。
- 伺服器不會將使用者的 IP 位址（`remoteip`）傳送給服務。token 只能使用一次，所以伺服器不會重試。逾時時間為 10 秒。

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### 服務無法連線時

當服務無法連線、逾時、回傳 5xx，或拒絕金鑰時：

| 表單 | 處理方式 |
| --- | --- |
| 註冊和申請重設連結 | 以「請稍後再試」拒絕（`503`）。否則機器人可能會觸發寄信和註冊。 |
| 登入 | 不經 CAPTCHA 直接放行，伺服器會記錄 WARN「the CAPTCHA service could not check a sign-in; letting it through」。速率限制和密碼檢查仍然適用。 |

放行登入是為了避免服務中斷或金鑰錯誤時，所有人（包括管理員）都被鎖在外面。如果這則 WARN 持續出現，請檢查服務狀態和金鑰。

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### reCAPTCHA 免費方案

超過每月 10,000 次評估的免費額度後，reCAPTCHA 的 siteverify **一律會成功**（所以在那段期間 CAPTCHA 沒有任何防護作用）。伺服器無法偵測是否超量，所以請在 Google Cloud 控制台中檢查你的用量。

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## 頁面與 Content-Security-Policy

- 小工具的語言依照頁面的語系，主題則依照瀏覽器的配色（淺色或深色）。Turnstile 小工具會自行依照配色。hCaptcha 和 reCAPTCHA 則依瀏覽器送出的 client hint `Sec-CH-Prefers-Color-Scheme` 來選擇；不送出它的瀏覽器會使用淺色主題。
- 頁面會在小工具旁說明由哪個服務執行檢查、該服務會收到 IP 位址和瀏覽器資訊，並連結到該服務的隱私權政策（hCaptcha 和 reCAPTCHA 還會連結到服務條款）。對 reCAPTCHA 來說，這也兼作 Google 要求的聲明：「This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply.」
- 在沒有 JavaScript 的瀏覽器中，`<noscript>` 訊息會說明這項檢查需要 JavaScript。無法通過 CAPTCHA 的使用者既無法註冊也無法登入，你只能用其他方式協助他們，例如用 `my admin create` 建立帳號，或暫時設定 `captcha = none`。
- 只有在 CAPTCHA 啟用時，`Content-Security-Policy`（CSP）才會在登入、註冊和申請密碼重設頁面上放寬。絕不會加入 `'unsafe-inline'` 和 `'unsafe-eval'`。網頁用戶端、用戶端 API、同意畫面、電子郵件確認及其他頁面的 CSP 不會改變。

| `captcha` | 新增的來源 |
| --- | --- |
| `turnstile` | `script-src`、`frame-src`：`https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`、`frame-src`、`style-src`、`connect-src`：`https://hcaptcha.com`、`https://*.hcaptcha.com` |
| `recaptcha` | `script-src`：`https://www.google.com/recaptcha/`、`https://www.gstatic.com/recaptcha/`；`frame-src`：`https://www.google.com/recaptcha/`、`https://recaptcha.google.com/recaptcha/`；`connect-src`：`https://www.google.com/recaptcha/` |

如果伺服器前方的反向代理或 CDN 會覆寫 CSP，請為這三個頁面加入相同的來源。

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## 隱私權政策中要寫什麼

啟用 CAPTCHA 後，任何開啟登入、註冊或申請密碼重設頁面的人，其瀏覽器都會直接連線到該服務，服務會收到 IP 位址、瀏覽器資訊和 Cookie。實例的隱私權政策（`my instance set privacy_policy`，請參閱[實例設定](instance-settings.md)）由你撰寫；伺服器不會在其中加入任何內容。[隱私權政策範本](privacy-policy-template.md)中有一列是關於該服務的。請至少寫下以下內容。

- 你使用哪個服務（服務商名稱）以及原因：為了防止自動化的登入、註冊和密碼重設請求。
- 哪些資料會送到服務（IP 位址、瀏覽器和裝置資訊、Cookie、與小工具的互動），以及只有登入、註冊和申請密碼重設頁面會傳送這些資料。
- 該服務隱私權政策的連結（Turnstile：<https://www.cloudflare.com/privacypolicy/>，hCaptcha：<https://www.hcaptcha.com/privacy>，reCAPTCHA：<https://policies.google.com/privacy>）。
- 使用 reCAPTCHA 時，說明 Google 的隱私權政策和服務條款（<https://policies.google.com/terms>）也適用。
