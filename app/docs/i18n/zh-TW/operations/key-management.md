<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# 管理金鑰加密金鑰（KEK）

伺服器會用一把金鑰加密金鑰（KEK）加密它存放在資料庫中的機密，而這把 KEK 本身絕不會存放在資料庫裡。本頁說明如何產生、保管、備份和輪替 KEK，如何改為把它保存在 KMS 中，以及 KEK 遺失或外洩時該怎麼做。[設定參考](../reference/configuration.md)列出了所有設定。

<!-- source: how-it-works 37074c6237d0 -->
## 運作方式

![金鑰階層：金鑰加密金鑰保存在 key_manager 指定的位置，絕不放在資料庫中；它包裝為每個機密產生的資料加密金鑰，再由資料加密金鑰加密機密。資料庫在五個密封欄位中保存金鑰 ID、包裝後的資料加密金鑰和加密後的機密。下方的清單說明每一層。](images/key-hierarchy.svg)

- 存放在資料庫中的機密（例如 actor 的私鑰）會以 AES-256-GCM 加密，使用的是為每個機密產生的資料加密金鑰（DEK）。DEK 會用 **金鑰加密金鑰（KEK）** 包裝後，與密文存放在一起（信封加密）。
- KEK 不存放在資料庫中，所以光是資料庫傾印外洩，並不會讓機密曝光。
- 每份密文都帶有包裝其 DEK 的 KEK 的 **金鑰 ID**。使用 `key_manager = local` 時，金鑰 ID 是 `local:` 加上由 KEK 推導出的 16 位十六進位數字，所以同一把 KEK 永遠得到同一個 ID。你不需要自行指定 ID。
- 密文會綁定它所存放的列 ID 和欄位名稱。把它複製到其他列或欄位後就無法解密。
- 密封欄位（名稱依 `my kek status` 的輸出）有：`local_account_private_keys.private_key_enc`（actor 的私鑰）、`totp_enrollments.secret_enc`（兩步驟驗證的密鑰）、`login_provider_secrets.sealed_secret`（登入服務提供者的用戶端密鑰）、`webhook_secrets.sealed_secret`（Webhook 簽章密鑰），以及 `instance_secrets.sealed_secret`（媒體代理的簽章金鑰）。
- 通行金鑰登入或透過外部服務提供者登入的狀態也以同樣方式密封，但不會存放：由瀏覽器保存，最多 30 分鐘。
- 媒體代理 URL 的簽章金鑰（HMAC-SHA256）是伺服器第一次啟動時產生的隨機金鑰，會像其他機密一樣加密後存放在資料庫中。輪替 KEK 時它不會改變，所以媒體代理 URL 在輪替前後都能繼續使用，而 `my kek rewrap` 會把它與其他機密一起重新包裝。
- `key_manager` 可以是 `local`（預設：使用 `kek` 設定）、`gcpkms`（KEK 留在 Google Cloud KMS 中；請參閱 [Google Cloud KMS](#google-cloud-kms)）、`awskms`（KEK 留在 AWS KMS 中；請參閱 [AWS KMS](#aws-kms)），或 `plugin`（KEK 由你自己的程式保管；請參閱[外掛](#a-plugin)）。

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## 遺失 KEK 會發生什麼事

沒有 KEK，即使還原了資料庫，也無法解密 actor 的私鑰。私鑰一旦遺失，其他伺服器就再也無法驗證該 actor 的簽章，**這個 actor 在聯邦宇宙中的身分就會毀損**（追蹤者的關係無法轉移）。請像對待資料庫一樣慎重地對待 KEK。

![遺失 KEK 會失去什麼：在沒有 KEK 的情況下還原之後，所有未密封的內容仍然可以讀取，每個密封的值都無法開啟，actor 也會失去身分。下方的段落說明相同的內容。](images/kek-loss.svg)

未密封的內容仍然可以讀取：貼文、帳號、追蹤關係以及資料庫的其他部分，還有儲存空間中的媒體檔案。每個密封的值都會以 `unknown key-encryption key` 錯誤失敗：actor 的私鑰、兩步驟驗證的密鑰、登入服務提供者的用戶端密鑰、Webhook 簽章密鑰，以及媒體代理的簽章金鑰。如果設定了另一把 KEK 來取代包裝這些機密的那一把，或包裝它們的 KMS 金鑰或外掛的金鑰被停用或銷毀，也會發生同樣的情況。

<!-- source: generate-a-kek 44b338b7440b -->
## 產生 KEK

把 32 個隨機位元組以標準 base64 編碼。

```sh
openssl rand -base64 32
```

沒有 `openssl` 的環境也可以用 `head -c 32 /dev/urandom | base64`。不要把產生的值留在終端機的歷史記錄或工單中。

<!-- source: store-and-deliver-it 9300565c2a3c -->
## 保管與傳遞

KEK 是機密設定，所以只能透過環境變數 `MY_KEK`，或透過 `MY_KEK_FILE`（存放該值的檔案路徑）傳遞。把它寫在設定檔（TOML）中會導致無法啟動。

| 目標 | 傳遞方式的範例 |
| --- | --- |
| 自行架設（二進位檔 / systemd） | 放在只有擁有者能讀取的檔案中（`chmod 600`），並讓 `MY_KEK_FILE` 指向它 |
| Docker / Compose | 把 Docker secret 掛載到 `/run/secrets/my_kek`，並設定 `MY_KEK_FILE=/run/secrets/my_kek`。`deploy/docker` 的檔案會從 `secrets/kek` 這樣做（[Docker Compose 檔案](running.md#docker-compose-files)）：要輪替時，替換該檔案並在 `.env` 中設定 `MY_KEK_PREVIOUS` |
| Kubernetes / microk8s | 把 Secret 掛載為檔案，並讓 `MY_KEK_FILE` 指向它 |
| Cloud Run | 把 Secret Manager 的密鑰公開為檔案或環境變數 |
| AWS ECS | 從 Secrets Manager 或 SSM Parameter Store（SecureString）傳遞 |

- 讓每個程序（例如 web 和 worker）都使用同一把 KEK。
- KEK 絕不會出現在日誌或錯誤訊息中（只會出現金鑰 ID）。
- `my config check` 會驗證 KEK 的格式（32 位元組的 base64），並把值顯示為 `(redacted)`。在 `production` 設定檔中，若 KEK 不是隨機的（例如全為零或重複同一個位元組），它也會失敗，而 `my serve` 會為此記錄警告（請參閱[正式環境檢查清單](production-checklist.md)）。

<!-- source: back-it-up 29e7bdb3b114 -->
## 備份

**資料庫和 KEK 都要備份，而且要分開備份**。

- 把 KEK 保存在資料庫備份以外的地方（密碼管理工具、離線媒體、另一個帳號中的機密儲存庫等）。如果兩者放在同一個地方，那個地方一旦外洩，資料庫加密就失去意義。
- 輪替期間，只要資料庫中還有以 `kek_previous` 的金鑰包裝的密文，就要把這些金鑰也納入備份。
- 把 KEK 納入你的還原程序，並定期測試還原（[備份與還原](backup.md#recovery-drill)）。還原資料庫後，請把 KEK（以及 `kek_previous`）設定成備份當時使用的相同值。如果 KEK 不同，任何讀取機密的操作都會以 `unknown key-encryption key` 錯誤失敗，錯誤中會指出金鑰 ID。

<!-- source: rotate-the-kek eb4af3189053 -->
## 輪替 KEK

`kek_previous` 中的 KEK 只用於解密，新的機密一律以 `kek` 加密。為了讓新舊程序並存的滾動部署絕不會產生某些程序無法讀取的密文，請分兩個步驟切換。

![分五個步驟輪替本機 KEK，並列出每一步之後 kek 與 kek_previous 的值：新的 KEK 先放進 kek_previous，接著與舊的 KEK 交換位置，my kek rewrap 搬移已存放的機密，最後舊的 KEK 從設定中移除。下方的步驟說明相同的內容。](images/kek-rotation.svg)

1. 產生新的 KEK 並備份。
2. 以 `MY_KEK=<old KEK>` 和 `MY_KEK_PREVIOUS=<new KEK>` 重新啟動每個程序。這時每個程序都能用新的 KEK 解密。
3. 以 `MY_KEK=<new KEK>` 和 `MY_KEK_PREVIOUS=<old KEK>` 重新啟動每個程序。從此之後，機密會以新的 KEK 包裝，而舊 KEK 的密文仍然可以讀取。
4. 用新的 KEK 重新包裝已存放的機密：以與步驟 3 相同的設定執行一次 `my kek rewrap`（請參閱[重新包裝已存放的機密](#re-wrap-the-stored-secrets)）。當它以 0 結束時，資料庫中就不再有任何機密需要舊的 KEK。
5. 在步驟 3 之後至少等待 30 分鐘，讓以舊 KEK 發出的短效登入權杖過期，然後以 `MY_KEK=<new KEK>` 且不設定 `kek_previous` 重新啟動每個程序。只要你還保留步驟 4 之前取得的資料庫備份，就要保留舊 KEK 的備份：還原那些備份時會再次需要它。

`kek_previous` 可以接受多把以逗號分隔的金鑰。同一把金鑰列出兩次，或列出與 `kek` 相同的金鑰，都會導致無法啟動。

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### 重新包裝已存放的機密

`my kek rewrap` 只會替換每個機密中包裝後的資料加密金鑰，並保留其密文；它絕不會寫出或顯示任何機密。

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` 會依欄位和金鑰 ID 各顯示一行，以 `(current)` 標示 `kek` 的金鑰 ID，最後說明是否還有機密需要 `kek_previous` 的金鑰。如果某個金鑰 ID 既不是 `kek`，也不在 `kek_previous` 中，就表示設定中少了一把 KEK。
- `my kek rewrap` 會以每批 100 個讀取機密，並在一個短交易中寫入每一批。它會依欄位顯示重新包裝了多少機密，以及有多少已經以 `kek` 包裝。它可以在伺服器服務期間執行，隨時停止或重新執行都是安全的：它會略過已經以 `kek` 包裝的機密，而在這期間被伺服器改寫的機密會維持伺服器寫入的樣子。
- 無法重新包裝的機密會保持不變，並以其列 ID 回報，指令會以 1 結束。原因可能是它的 KEK 沒有設定（訊息會指出金鑰 ID：把那把 KEK 加到 `kek_previous` 後再執行一次），或是存放的值已損毀或是從其他列複製而來。只要 `my kek status` 仍然計算到某把 KEK 下的機密，就不要把它從 `kek_previous` 中移除。

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

使用 `key_manager = gcpkms` 時，由對稱式 Cloud KMS 金鑰（用途為 `ENCRYPT_DECRYPT` 的 CryptoKey）包裝資料加密金鑰，KEK 絕不會離開 Cloud KMS。即使資料庫傾印和設定同時外洩，機密仍不會曝光：解密需要使用該金鑰的權限。

- 把 `kms_key` 設為金鑰的資源名稱，不含版本：`projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`。超過 248 位元組的名稱在啟動時會被拒絕。已存放機密的金鑰 ID 是 `gcpkms:` 加上這個名稱。
- 伺服器以 Application Default Credentials 進行驗證：在 Cloud Run 上是該服務的服務帳號；其他環境則是 `GOOGLE_APPLICATION_CREDENTIALS`。
- 在金鑰上（而不是在專案上）授予該服務帳號 `roles/cloudkms.cryptoKeyEncrypterDecrypter`，供每個程序（例如 web 和 worker）以及 `my kek status` 和 `my kek rewrap` 指令使用。沒有這個角色時，讀取機密會失敗，錯誤會指出該角色；金鑰版本不存在或已停用時也會同樣失敗，而且在設定修正之前不會重試這些錯誤。配額錯誤或 Cloud KMS 無法使用屬於暫時性問題，需要該機密的工作會重試。
- 伺服器會把 Cloud KMS 解密的資料加密金鑰保留 `kms_dek_cache_ttl`（預設 `10m`），所以大多數的機密讀取都不會呼叫 Cloud KMS。
- 建置標籤 `no_gcpkms` 會把這個轉接器排除在二進位檔之外（[建置標籤](../reference/build-tags.md)）。

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### 在 Cloud KMS 中輪替金鑰

在 Cloud KMS 中輪替（手動，或在金鑰上設定輪替週期）會新增一個主要版本，並讓舊版本保持啟用。新的機密會以新版本包裝，伺服器會用包裝每個機密的那個版本來解密，而金鑰 ID 不會改變，所以 **輪替不需要 `my kek rewrap`**。

**只要還有機密需要舊版本，就不要停用或銷毀它**：以已停用或已銷毀的版本包裝的機密將再也無法解密。Cloud KMS 不會告訴你某個版本包裝了哪些機密，所以要汰除金鑰材料時，請先把機密搬到新的金鑰：

1. 建立新的金鑰（可以在同一個金鑰環中），並在其上授予服務帳號該角色。
2. 把 `kms_key` 設為新的金鑰、`kms_key_previous` 設為舊的金鑰，然後重新啟動每個程序。舊的金鑰此後只用於解密。
3. 以相同的設定執行 `my kek rewrap`。當 `my kek status` 不再計算到舊金鑰 ID 下的機密時，移除 `kms_key_previous` 並重新啟動。
4. 直到這時才停用舊金鑰的版本，並且只要你還保留步驟 3 之前取得的資料庫備份，就要讓它們保持可還原：還原那些備份時會再次需要它們。

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

使用 `key_manager = awskms` 時，由對稱式 AWS KMS 金鑰（金鑰規格 `SYMMETRIC_DEFAULT`、金鑰用途 `ENCRYPT_DECRYPT`、客戶管理）包裝資料加密金鑰，KEK 絕不會離開 AWS KMS。即使資料庫傾印和設定同時外洩，機密仍不會曝光：解密需要使用該金鑰的權限。

- 把 `kms_key` 設為金鑰的 ARN（`arn:aws:kms:{region}:{account}:key/{key-id}`）、別名（`alias/{name}` 或其 ARN）或金鑰 ID。伺服器會在啟動時把名稱解析為金鑰的 ARN，已存放機密的金鑰 ID 是 `awskms:` 加上該 ARN。因此，把別名指向另一把金鑰，在下次啟動之前不會有任何改變，而啟動後也只影響新的機密：已存放的機密仍會指名舊的金鑰，所以請把它設在 `kms_key_previous` 中（請參閱[汰除 AWS KMS 金鑰](#retire-an-aws-kms-key)）。AWS 受管金鑰（`alias/aws/...`）會被拒絕，因為只有擁有它的 AWS 服務才能使用。
- 伺服器以 AWS SDK 的預設憑證鏈進行驗證：在 ECS 上是任務的角色；在 EC2 上是執行個體設定檔；其他環境則是環境變數（`AWS_ACCESS_KEY_ID` 等）或共用設定檔。區域取自金鑰的 ARN；使用別名或金鑰 ID 時，請設定 `AWS_REGION`。
- 在金鑰上允許 `kms:Encrypt`、`kms:Decrypt` 和 `kms:DescribeKey`，供每個程序（例如 web 和 worker）以及 `my kek status` 和 `my kek rewrap` 指令使用。IAM 角色的政策範例：

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  即使 `kms_key` 是別名，資源也要寫金鑰的 ARN。金鑰政策也必須允許該帳號的 IAM 政策授予存取權（預設的金鑰政策就是如此）。
- 當金鑰不存在、已停用或等待刪除，或不是對稱加密金鑰時，伺服器會無法啟動。之後，若缺少權限或金鑰被停用，讀取機密會失敗，錯誤會指出原因，而且在設定修正之前不會重試這些錯誤。節流（`ThrottlingException`）或 AWS KMS 無法使用屬於暫時性問題，需要該機密的工作會重試。
- 每次呼叫的加密內容會包含機密的列 ID 和欄位名稱（`{"my-aad": ...}`，以 base64url 編碼）。CloudTrail 會以明文記錄它；其中不含任何機密。
- 費用：AWS 會對每把客戶管理金鑰收取月費（撰寫本文時為 USD 1，金鑰材料的前兩次輪替各另加相同金額），並對超出每月免費額度的請求按次收費（請參閱 [AWS KMS 定價](https://aws.amazon.com/kms/pricing/)）。伺服器會把 AWS KMS 解密的資料加密金鑰保留 `kms_dek_cache_ttl`（預設 `10m`），所以一台有十個持續傳送的 actor 的伺服器，每個程序每月約發出 43,000 次請求，超出免費額度的部分只要幾美分。
- 建置標籤 `no_awskms` 會把這個轉接器排除在二進位檔之外（[建置標籤](../reference/build-tags.md)）。

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### 汰除 AWS KMS 金鑰

AWS KMS 的自動金鑰輪替會保留金鑰的 ARN 及其先前的金鑰材料，所以金鑰 ID 不會改變，**輪替不需要 `my kek rewrap`**。

要停止使用某把金鑰，或把別名指向另一把金鑰時，請先把機密搬到新的金鑰。**只要還有機密需要舊金鑰，就不要停用它或排定刪除它**：以它包裝的機密將再也無法解密。

1. 建立新的金鑰，並允許該角色使用它。
2. 把 `kms_key` 設為新的金鑰、`kms_key_previous` 設為舊的金鑰（使用其 ARN，而不是別名），然後重新啟動每個程序。舊的金鑰此後只用於解密。
3. 以相同的設定執行 `my kek rewrap`。當 `my kek status` 不再計算到舊金鑰 ID 下的機密時，移除 `kms_key_previous` 並重新啟動。
4. 直到這時才停用舊金鑰，並且只要你還保留步驟 3 之前取得的資料庫備份，就要保留它：還原那些備份時會再次需要它。

<!-- source: a-plugin 43023e60bac9 -->
## 外掛

使用 `key_manager = plugin` 時，由你自己的[外掛](plugins.md)保管 KEK（放在 Vault、HSM 或它選擇的任何地方），並為伺服器包裝資料加密金鑰。

**外掛與 KEK 位於同一個信任邊界內**。它會看到每把明文的資料加密金鑰，所以控制它的人，或控制它所在主機的人，都能讀取伺服器的所有機密，包括 actor 的私鑰。請給它與 KEK 相同等級的保護。這也是為什麼只有設定檔和環境變數能變更它的位址。

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- 位址是 `unix://` 或 `https://`。伺服器會拒絕 `http://`（即使是 loopback 也一樣），因為金鑰會以明文傳送。目的地的其他設定請參閱[外掛](plugins.md#the-destination)。
- 在讓伺服器指向外掛之前，先執行 `my plugin test --kind key_manager <address>`（[測試外掛](plugins.md#test-a-plugin)）。它只會包裝自己臨時產生、用完即丟的金鑰。
- 已存放機密的金鑰 ID 是 `plugin:` 加上外掛為其 KEK 指定的 ID，長度為 1 到 248 位元組的可見 ASCII 字元。
- **保留已存放機密仍然指名的每一把 KEK**。外掛中的輪替會在下一次包裝時以新的 ID 呈現。先前包裝的機密仍會指名舊的 ID，外掛必須仍然能開啟它們。`my kek rewrap` 會把它們搬到目前的 ID，`my kek status` 則會計算剩下的數量。
- 外掛和 KMS 絕不會同時運作：使用外掛時會拒絕 `kms_key` 和 `kms_key_previous`，而使用 `key_manager = gcpkms` 或 `awskms` 時也會拒絕 `plugin_key_manager_address`。
- 建置標籤 `no_plugins` 會把這個轉接器排除在二進位檔之外（[建置標籤](../reference/build-tags.md)）。

<!-- source: when-the-server-starts 5faeda92e43b -->
### 伺服器啟動時

每個程序，以及每個會開啟資料庫的 `my` 指令（例如 `my kek status`），都會在做任何事之前先執行以下步驟。這是唯一一種失敗時會讓啟動中止的外掛。

1. 向外掛詢問其目前 KEK 的 ID。
2. 用一把用完即丟的隨機金鑰執行自我測試：包裝再解開必須取回原本的金鑰，以不同的驗證資料解開必須回答 `OUTCOME_DECRYPT`，同一把金鑰的兩次包裝結果必須不同，而包裝後的值不得包含該金鑰。

如果外掛沒有回應或回答 `503`，伺服器會在最多 30 秒內重試，以便配合比伺服器晚啟動的 sidecar。超過之後，程序會以錯誤結束。錯誤的回答，以及其他任何失敗（例如權杖被拒絕），都會立即中止啟動：自我測試的目的就是擋下忽略驗證資料或直接把金鑰原樣傳回的外掛，而等待並不能修好這種外掛。

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### 外掛無法連線時

伺服器會採取失敗即關閉的做法。無論發生什麼問題（呼叫被拒絕或逾時，或回答為空、未知或格式錯誤），需要該金鑰的操作都會以錯誤失敗。它絕不會以明文存放機密，絕不會改用本機 KEK 或其他金鑰，也絕不會把失敗當成結果。以這種方式失敗的任務會稍後重試，而請求則會對提出請求的人回報失敗。每次失敗的呼叫都會依種類和 Connect 代碼計入 [`my.plugin.failures`](running.md)，日誌中不會包含任何金鑰、驗證資料或包裝後的值，也不會包含外掛所回應的任何內容。

包裝永遠不會被快取，而剛剛才密封的值在快取中也沒有任何內容，所以以下操作會立即停止：

- 建立帳號，因為這會產生 actor 的金鑰。
- 設定兩步驟驗證，以及註冊通行金鑰或用通行金鑰登入，因為這些流程會把狀態密封保存。
- 透過外部服務提供者登入，以及儲存服務提供者的用戶端密鑰。
- 註冊 Webhook，以及 `my kek rewrap`。

只有在程序仍保有先前讀取時取得的資料加密金鑰期間（最多 `kms_dek_cache_ttl`，預設 `10m`），讀取已存放的機密才能運作。超過這段時間，或對於尚未讀取過的機密，以下操作也會停止：

- 聯邦傳送和簽章擷取，因為它們需要 actor 的私鑰。外掛恢復後，傳送會透過其任務的重試補上進度。
- 登入時檢查兩步驟驗證碼。
- Webhook 傳送。

重新啟動會如上所述中止，所以 **不要靠重新啟動伺服器來修復外掛的中斷**：仍在運作的伺服器會繼續提供快取涵蓋的內容，而重新啟動的伺服器在外掛回應之前什麼都無法提供。不需要機密的操作（例如閱讀時間軸和發文）會照常進行。媒體代理的簽章金鑰會在啟動時讀取並保留在記憶體中。更新後的用戶端憑證或 CA 檔案也不需要重新啟動：檔案變更後，伺服器會重新讀取它們（[外掛](plugins.md#authentication)）。

<!-- source: the-contract 6183eb7a7dfa -->
### 介面約定

RPC 是 `my.plugin.key_manager.v1.KeyManagerService` 的 RPC，預設期限為 5 秒，每個方向的訊息最多 128 KiB。

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` 是外掛自己的 ID，不含 `plugin:`。`UnwrapKey` 可能會指名比目前更舊的 ID。
- `wrapped` 為 1 到 65,535 位元組，且不得包含該金鑰。同一把金鑰的兩次包裝結果會不同。
- `aad` 是包含列 ID 和欄位名稱的驗證資料，除非它與 `WrapKey` 收到的相同，否則 `UnwrapKey` 必須失敗。這正是防止機密被複製到其他列的機制。
- **絕不要記錄 `dek`、`aad` 或 `wrapped`**。

| `outcome` | 意義 |
| --- | --- |
| `OUTCOME_OK` | 呼叫成功 |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey`：外掛沒有持有 `keyId` 的 KEK |
| `OUTCOME_DECRYPT` | `UnwrapKey`：`wrapped` 或 `aad` 未通過驗證 |

當你無法回答時（例如金鑰儲存庫無法連線），請回傳 HTTP 或 Connect 錯誤。這時不要回答 `OUTCOME_DECRYPT` 或 `OUTCOME_UNKNOWN_KEY`：對伺服器來說，這兩者表示值本身有問題。伺服器本身不會重試呼叫，所以外掛可能會從某個任務再次收到同一個呼叫。

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## 在本機 KEK 與 KMS 之間遷移

金鑰 ID 的開頭是包裝它的金鑰管理器種類（`local:`、`gcpkms:`、`awskms:`、`plugin:`）。伺服器會以目前的 `key_manager` 包裝新的機密，並以每個已存放機密的金鑰 ID 所指名的金鑰管理器來解開它，所以在本機 KEK 與 KMS 之間遷移，就是一次跨種類的輪替。

- 使用 KMS 時，`kek` 和 `kek_previous` 只用於解密：它們會開啟仍以本機 KEK 包裝的機密，而每個新的機密都會以 `kms_key` 包裝。`kms_key_previous` 列出同一個 KMS 中較早的金鑰，它們也只用於解密。
- 使用 `key_manager = local` 時，若設定了 `kms_key` 和 `kms_key_previous`，它們只用於解密：它們會開啟仍以 KMS 包裝的機密。
- 從本機 KEK 遷移到 KMS：
  1. 依照 [Google Cloud KMS](#google-cloud-kms) 或 [AWS KMS](#aws-kms) 的說明建立金鑰並授予權限。
  2. 以 `key_manager = gcpkms` 或 `awskms`、把 `kms_key` 設為該金鑰，並把目前的 KEK 從 `kek` 移到 `kek_previous`，重新啟動每個程序。此後機密會以 KMS 包裝，而以本機 KEK 包裝的機密仍然可以開啟。
  3. 以相同的設定執行 `my kek rewrap`。
  4. 當 `my kek status` 不再計算到 `local:` 金鑰 ID 下的機密時，移除 `kek_previous` 並重新啟動。只要你還保留步驟 3 之前取得的資料庫備份，就要保留 KEK 的備份。
- 從 KMS 遷回本機 KEK 時，反過來做同樣的事：設定 `key_manager = local` 和新的 `kek`，並保留 `kms_key` 的設定（此後只用於解密），直到 `my kek rewrap` 搬完每個機密，且 `my kek status` 不再計算到 `gcpkms:` 或 `awskms:` 金鑰 ID 下的機密為止。這時 `kms_key` 和 `kms_key_previous` 中的金鑰必須全部屬於同一個 KMS。
- 要從一家雲端的 KMS 遷移到另一家的 KMS，請先遷移到本機 KEK，再遷移到另一個 KMS；沒有直接遷移的方式。
- 外掛也以同樣的方式遷移，而且只能經由本機 KEK：
  - 遷移到外掛時，以 `key_manager = plugin`、`plugin_key_manager_address`，並把目前的 KEK 從 `kek` 移到 `kek_previous`，重新啟動每個程序，然後執行 `my kek rewrap`。當 `my kek status` 不再計算到 `local:` 金鑰 ID 下的機密時，移除 `kek_previous` 並重新啟動。
  - 從外掛遷出時，以 `key_manager = local`、新的 `kek`，並保留 `plugin_key_manager_address`（此後只用於解密），重新啟動每個程序，然後執行 `my kek rewrap`。當 `my kek status` 不再計算到 `plugin:` 金鑰 ID 下的機密時，移除該位址並重新啟動。在那之前，外掛仍必須通過[啟動時的檢查](#when-the-server-starts)，其中包括一次包裝。
- 伺服器會把 KMS 或外掛解密的資料加密金鑰保留 `kms_dek_cache_ttl`（預設 `10m`，最多 `kms_dek_cache_entries` 個，預設 `1024`），所以使用中的機密在每個週期、每個程序大約只會呼叫它一次。任一設定為 `0` 都會停用快取。

<!-- source: if-the-kek-leaks be6e788e4cea -->
## KEK 外洩時

如果只有 KEK 外洩，請依上述方式輪替。如果 KEK 和資料庫可能都已外洩，輪替就無法保護你：資料庫中 actor 的私鑰已經可以被讀取，所以必須重新產生 actor 金鑰本身。

<!-- source: web-push-vapid-key aa32321d1e4a -->
## Web Push VAPID 金鑰

Web Push 以伺服器的 VAPID 金鑰（P-256，RFC 8292）簽章。這是與 KEK 分開的機密，也不會存放在資料庫中。

- 用 `my config vapid-key` 建立金鑰對，並透過 `MY_VAPID_PRIVATE_KEY` 或 `MY_VAPID_PRIVATE_KEY_FILE` 把私鑰（`vapid_private_key = ...` 的值）提供給每個程序。伺服器會從私鑰推導出公鑰並交給用戶端，所以你不需要設定公鑰。
- 如果沒有設定，Web Push 會停用，用戶端也不會提供這項功能。在 `production` 設定檔中，`my config check` 和 `my serve` 啟動時會對此發出警告。
- 瀏覽器會把訂閱綁定到公鑰，所以變更金鑰會中斷對現有訂閱的傳送。使用者在設定中重新開啟推播通知時，用戶端會以新的金鑰重新訂閱。除非金鑰外洩，否則不要變更它；如果外洩了，請切換到新的金鑰，並請使用者重新開啟通知。
