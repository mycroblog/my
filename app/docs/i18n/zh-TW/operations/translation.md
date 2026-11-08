<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# 使用翻譯服務翻譯貼文

當讀者提出要求時，伺服器可以透過你設定的翻譯服務翻譯貼文：LibreTranslate、DeepL、Google Cloud Translation 或 Amazon Translate。伺服器擁有某個服務的金鑰時，該服務就會內建可用，再由[實例設定](instance-settings.md) `translation_providers` 啟用它。

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate 是開放原始碼的機器翻譯，你可以自行架設，也可以使用 [libretranslate.com](https://libretranslate.com) 的託管服務。用[設定](../reference/configuration.md)中的兩個項目讓伺服器指向它：

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` 是 LibreTranslate 伺服器的基礎 URL，使用 `http:` 或 `https:`，可以加上路徑。私有位址上的伺服器也能使用，例如與本伺服器並列的容器，或 `http://10.0.0.5:5000`。伺服器不會跟隨它回傳的重新導向，因此金鑰不會被送到其他地方。
- `translator_libretranslate_api_key` 是機密資訊，只會從環境變數讀取（`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` 或 `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`）。對方伺服器不需要金鑰時請不要設定；自行架設的伺服器通常不需要。金鑰絕不會顯示給用戶端，也不會寫入日誌。

接著把 `libretranslate` 依你希望嘗試的順序加入實例設定 `translation_providers`，例如只設定 `translation_providers = libretranslate`，或在後面接上另一個服務的 ID 作為備援。

伺服器會在第一次翻譯前向 LibreTranslate 取得支援的語言清單，之後每小時更新一次，不會送出清單中沒有的語言組合的貼文。貼文沒有標示語言時，由 LibreTranslate 自動偵測。請在你架設的伺服器上安裝讀者所需的語言模型。

**翻譯品質因語言而異**。LibreTranslate 的模型對某些語言組合表現良好，對其他組合則較粗糙；從訓練資料較少的語言翻譯時，可能會誤解原意。啟用前請先試試你的社群使用的語言；如果需要，在 `translation_providers` 中把商用服務排在它後面。

**文字會送到哪裡**。由你自己架設的伺服器會把貼文文字留在你自己的機器上，不會有其他人收到。這時你可以用 `translation_provider_visibilities` 允許它處理 `private` 與 `direct` 貼文，例如 `libretranslate=public|local|unlisted|private|direct`，而[隱私權政策](privacy-policy-template.md#the-services-you-choose)中也不必列出任何外部翻譯服務。託管的 LibreTranslate（無論是 libretranslate.com 還是其他人架設的）會像其他服務一樣收到文字：請閱讀它的條款、保留預設的可見範圍，並在政策中寫明它。

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL 透過 DeepL API 方案的金鑰，經由其 API 進行翻譯。伺服器只會送出貼文的文字，以及代表提及、連結與裝飾的標記（以 XML 元素表示，DeepL 會保留其位置），不會送出貼文的其他內容。

1. 從你的 DeepL 帳號取得驗證金鑰。免費方案的金鑰以 `:fx` 結尾。
2. 在[部署設定](../reference/configuration.md)中設定它，然後重新啟動伺服器：

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   免費方案的金鑰會呼叫 `https://api-free.deepl.com`，其他金鑰則呼叫 `https://api.deepl.com`。若要經由代理伺服器，請設定 `MY_TRANSLATOR_DEEPL_URL`。金鑰是機密資訊：伺服器絕不會把它寫入日誌、放進錯誤訊息，或在設定與用戶端 API 的回應中顯示它，而且只會把它送到該位址，不跟隨重新導向。
3. 用[實例設定](instance-settings.md) `translation_providers` 啟用這個服務，可以單獨使用，也可以依你的順序排列：

   ```sh
   my instance set translation_providers deepl
   ```

伺服器第一次翻譯時會詢問 DeepL 支援哪些語言，之後每天再詢問一次。貼文的語言會對應到 DeepL 最接近的變體：英文對應 `en-US`、葡萄牙文對應 `pt-BR`、台灣中文對應 `zh-Hant`。DeepL 不支援的語言會交給下一個服務，或告知讀者無法翻譯。

**額度**。每個方案都有每月字元額度，付費方案還可以在你的 DeepL 帳號中設定費用上限。當 DeepL 回報額度用盡或達到速率限制時，伺服器會嘗試 `translation_providers` 中的下一個服務。若已沒有其他服務，讀者會看到這則貼文目前無法翻譯；如果你允許，Web 用戶端會改用瀏覽器的翻譯功能。伺服器自己的每月上限（`translation_user_monthly_characters` 與 `translation_server_monthly_characters`）會在任何呼叫之前生效，因此請把它們設得比 DeepL 額度低。DeepL 計算字元的方式可能與它們不同。

**資料**。啟用前請先閱讀 [DeepL 的條款](https://www.deepl.com/en/pro-license)：

- 在付費方案中，DeepL 只會在翻譯所需的期間保留文字與譯文；調查錯誤時最多保留 72 小時。
- 在免費方案中，DeepL 可能永久保留文字與譯文，而且其條款禁止透過它傳送個人資料。貼文通常包含個人資料，因此免費方案只適合用來試用這個服務。絕對不要在 `translation_provider_visibilities` 中允許它處理 `private` 或 `direct` 貼文，並在你的隱私權政策中說明 DeepL 會保留它收到的內容。

在付費方案中，你可以逐一為各服務允許 `private` 與 `direct` 貼文：

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**標示來源**。DeepL 的條款要求每則譯文都要顯示其名稱或標誌。Web 用戶端會在每則來自 DeepL 的譯文旁顯示「DeepL」並連結到 `https://www.deepl.com`，所以你不需要另外加上任何東西。

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) 的服務 ID 是 `google`。伺服器會以一般翻譯模型呼叫它的 `TranslateText` 方法，並以 HTML 傳送文字，讓提及、連結以及其他必須維持原樣的部分回到原本的位置。

1. 在支付翻譯費用的專案中啟用 Cloud Translation API（`translate.googleapis.com`）：`gcloud services enable translate.googleapis.com --project PROJECT`。
2. 在該專案上授予伺服器的服務帳戶 `roles/cloudtranslate.user`：`gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`。
3. 把 `MY_TRANSLATOR_GOOGLE_PROJECT` 設為專案 ID。`MY_TRANSLATOR_GOOGLE_LOCATION` 預設為 `global`；也可以指定其他提供一般模型的 Cloud Translation 位置，例如 `us-central1`。
4. 重新啟動伺服器，並把 `google` 加入實例設定 `translation_providers`。

伺服器與它使用的其他 Google Cloud 服務一樣，以應用程式預設憑證（Application Default Credentials）進行驗證：在 Cloud Run 上使用該服務的服務帳戶；在其他環境則使用 `GOOGLE_APPLICATION_CREDENTIALS` 指定的金鑰檔案。不會使用 API 金鑰。伺服器啟動時不會進行任何呼叫，因此 API 未啟用或缺少角色時，要等到讀者要求翻譯才會顯現：這時伺服器會嘗試下一個服務，若已沒有其他服務，讀者會看到翻譯失敗。Google 回報專案的額度或速率限制時，也會改用下一個服務。

Google 在每月免費額度之後依傳送的字元數計費；請參閱 [Cloud Translation 定價](https://cloud.google.com/translate/pricing)。伺服器自己的每月上限也會計算字元數，但與 Google 的計費方式並不完全相同，因此也請在 Cloud Billing 中設定預算警示。

Google [表示](https://cloud.google.com/translate/data-usage)它只會將文字用於提供翻譯，只在記憶體中短暫保留，不會用來訓練模型，也不會分享給他人。伺服器呼叫的是 Cloud Translation 的全域端點，在這裡 Google 不會把處理限制在單一區域。

Google 的[標示來源要求](https://cloud.google.com/translate/attribution)規定譯文必須標示為機器翻譯，並在旁邊顯示連結到 `https://translate.google.com` 的「powered by Google Translate」標章。Web 用戶端會在這個服務的每則譯文旁顯示「Google Translate」名稱與該連結。Google 只以下載方式提供標章圖片，因此伺服器不會為這個服務提供標誌給用戶端；請確認讀者看到的內容符合 Google 對你的實例的要求。

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

Amazon Translate 是服務 `aws`。伺服器會和使用其他 AWS 服務時一樣，以 AWS SDK 預設憑證鏈的憑證呼叫它的 `TranslateText` 操作：任務或執行個體的執行角色、環境變數 `AWS_ACCESS_KEY_ID` 與 `AWS_SECRET_ACCESS_KEY`，或共用憑證檔案。伺服器的設定中不需要放入任何金鑰。

1. 為伺服器執行時使用的角色或使用者授予 IAM 權限 `translate:TranslateText`。對於語言不明的貼文，Amazon Translate 會使用 Amazon Comprehend 偵測語言；AWS 為 Amazon Translate 提供的受管政策會同時授予 `comprehend:DetectDominantLanguage`，所以也請一併授予。
2. 把 `translator_aws_region`（`MY_TRANSLATOR_AWS_REGION`）設為 Amazon Translate 的區域，例如 `us-east-1`。偵測語言時，該區域也必須提供 Amazon Comprehend。
3. 重新啟動伺服器，然後以 `translation_providers = aws` 啟用這個服務，可以單獨使用，也可以依你偏好的順序與其他服務排列（[實例設定](instance-settings.md)）。

Amazon Translate 依傳送的字元數計費，前 12 個月有免費方案（[定價](https://aws.amazon.com/translate/pricing/)）。伺服器只會傳送貼文中的文字：提及、主題標籤、連結與程式碼都留在伺服器上，各自以 HTML 標記中的簡短預留位置傳送。請把 `translation_user_monthly_characters` 與 `translation_server_monthly_characters` 的每月上限控制在你願意支付的範圍內。

**除非該帳戶受到 AWS Organizations 的 [AI 服務選擇退出政策](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html)涵蓋，否則 Amazon 可能會儲存文字並用來改善其服務，而且可能在其他區域進行**。沒有選擇退出時，Amazon 就不只是你處理這些文字的服務供應商：請在你的隱私權政策中說明這一點（[隱私權政策範本](privacy-policy-template.md#the-services-you-choose)），並且不要在 `translation_provider_visibilities` 中為 `aws` 允許 `private` 或 `direct`。預設情況下，這個服務只會收到 `public`、`local` 與 `unlisted` 貼文。

以建置標籤 `no_translator_aws` 建置的伺服器不含這個服務，若設定了 `translator_aws_region` 就會拒絕啟動（[建置標籤](../reference/build-tags.md)）。
