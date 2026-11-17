<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# 翻訳サービスで投稿を翻訳する

サーバーは、読む人が求めたときに、あなたが設定した翻訳サービス（LibreTranslate、DeepL、Google Cloud Translation、Amazon Translate）を通して投稿を翻訳できます。サーバーにキーがあればそのプロバイダーが組み込まれ、[インスタンスの設定](instance-settings.md) `translation_providers` で有効になります。

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate はオープンソースの機械翻訳で、自分で動かすことも、[libretranslate.com](https://libretranslate.com) のホスティングサービスとして使うこともできます。[設定](../reference/configuration.md)の 2 つの項目で、サーバーにその場所を指定します。

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` は LibreTranslate サーバーのベース URL で、`http:` か `https:` で始まり、パスを付けることもできます。隣のコンテナや `http://10.0.0.5:5000` のような、プライベートアドレスのサーバーも使えます。サーバーはそこからのリダイレクトをたどらないので、キーがほかの場所に送られることはありません。
- `translator_libretranslate_api_key` はシークレットで、環境変数（`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` または `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`）からだけ読み込まれます。キーを必要としないサーバーなら設定しないでください。自分で動かすサーバーでは、たいてい不要です。キーがクライアントに見せられたり、ログに書かれたりすることはありません。

次に、インスタンスの設定 `translation_providers` に `libretranslate` を、試したい順に加えます。たとえば `translation_providers = libretranslate` だけにするか、そのあとにほかのプロバイダーの ID を続けて、そちらにフォールバックさせます。

サーバーは最初の翻訳の前と、その後 1 時間ごとに LibreTranslate へ言語の一覧を問い合わせ、一覧にない言語の組み合わせの投稿は送りません。投稿に言語がないときは、LibreTranslate が言語を判定します。自分で動かすサーバーには、読む人が必要とする言語のモデルをインストールしてください。

**翻訳の品質は言語によって異なります。** LibreTranslate のモデルは、ある言語の組み合わせではよく、ほかの組み合わせでは粗く、学習データの少ない言語からの翻訳では意味を取り違えることがあります。有効にする前にコミュニティで使われている言語で試し、必要なら `translation_providers` でそのあとに商用のプロバイダーを置いてください。

**テキストの送り先。** 自分で動かすサーバーなら、投稿のテキストはあなたのマシンの中にとどまり、ほかの誰にも渡りません。その場合は `translation_provider_visibilities` で `private` と `direct` の投稿を許可してもかまいません（たとえば `libretranslate=public|local|unlisted|private|direct`）。また、[プライバシーポリシー](privacy-policy-template.md#the-services-you-choose)には翻訳のための外部サービスを載せません。ホスティングされた LibreTranslate（libretranslate.com でもほかの誰かのものでも）は、ほかのプロバイダーと同じようにテキストを受け取ります。その規約を読み、公開範囲は既定値のままにし、ポリシーにそのサービスを載せてください。

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL は、DeepL API プランのキーを使って API で翻訳します。サーバーが送るのは、投稿のテキストと、メンション・リンク・装飾の代わりとなるマーカー（XML 要素として送り、DeepL はその位置を保ちます）だけで、投稿のほかの部分は送りません。

1. DeepL のアカウントで認証キーを取得します。無料プランのキーは `:fx` で終わります。
2. [デプロイの設定](../reference/configuration.md)にキーを設定し、サーバーを再起動します。

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   サーバーは、無料プランのキーなら `https://api-free.deepl.com` を、それ以外のキーなら `https://api.deepl.com` を呼び出します。プロキシを通すときは `MY_TRANSLATOR_DEEPL_URL` を設定します。キーはシークレットです。サーバーはキーをログに書いたり、エラーに含めたり、設定やクライアント API の応答で見せたりすることはなく、そのアドレスにだけ、リダイレクトをたどらずに送ります。
3. [インスタンスの設定](instance-settings.md) `translation_providers` で、単独で、または好みの順にプロバイダーを有効にします。

   ```sh
   my instance set translation_providers deepl
   ```

サーバーは最初の翻訳のときと、その後 1 日ごとに、DeepL が翻訳できる言語を問い合わせます。投稿の言語は、DeepL がもつもっとも近い変種に当てはめます。英語は `en-US`、ポルトガル語は `pt-BR`、台湾の中国語は `zh-Hant` です。DeepL にない言語は次のプロバイダーに回されるか、翻訳できないことが読む人に伝えられます。

**クォータ。** 各プランには月ごとの文字数のクォータがあり、有料プランでは DeepL のアカウントで費用の上限を設定できます。DeepL がクォータやレート制限を報告すると、サーバーは `translation_providers` の次のプロバイダーを試します。残りがなければ、読む人には投稿をいまは翻訳できなかったことが表示され、許可していれば Web クライアントはブラウザの翻訳を使います。サーバー自身の月ごとの上限（`translation_user_monthly_characters` と `translation_server_monthly_characters`）は呼び出しの前に適用されるので、DeepL のクォータより低く設定してください。DeepL の文字数の数え方は、これらと異なることがあります。

**データ。** 有効にする前に [DeepL の規約](https://www.deepl.com/en/pro-license)を読んでください。

- 有料プランでは、DeepL はテキストと翻訳を翻訳に必要な間だけ保持し、エラーを調査するときも最長 72 時間までです。
- 無料プランでは、DeepL はテキストと翻訳を無期限に保持することがあり、その規約では個人データを送ることが禁じられています。投稿にはたいてい個人データが含まれるので、無料プランはプロバイダーを試すためだけに使ってください。`translation_provider_visibilities` で `private` や `direct` の投稿を決して許可せず、DeepL が受け取ったものを保持することをプライバシーポリシーに書いてください。

有料プランでは、プロバイダーごとに `private` と `direct` の投稿を許可できます。

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**帰属表示。** DeepL の規約では、各翻訳とともにその名前かロゴを表示することが求められます。Web クライアントは、DeepL による翻訳のそれぞれの隣に `https://www.deepl.com` へのリンク付きで「DeepL」と表示するので、あなたが付け加えるものはありません。

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) のプロバイダー ID は `google` です。サーバーは汎用の翻訳モデルでその `TranslateText` メソッドを呼び出し、メンションやリンクなど変えてはならない部分が元の位置に戻ってくるよう、テキストを HTML として送ります。

1. 翻訳の費用を払うプロジェクトで Cloud Translation API（`translate.googleapis.com`）を有効にします:
   `gcloud services enable translate.googleapis.com --project PROJECT`。
2. そのプロジェクトで、サーバーのサービスアカウントに `roles/cloudtranslate.user` を付与します:
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`。
3. `MY_TRANSLATOR_GOOGLE_PROJECT` にプロジェクト ID を設定します。`MY_TRANSLATOR_GOOGLE_LOCATION` の既定値は `global` で、汎用モデルを提供する Cloud Translation のほかのロケーション（たとえば `us-central1`）も指定できます。
4. サーバーを再起動し、インスタンスの設定 `translation_providers` に `google` を加えます。

サーバーは、使っているほかの Google Cloud のサービスと同じく、アプリケーションのデフォルト認証情報（Application Default Credentials）で認証します。Cloud Run ではサービスのサービスアカウント、それ以外では `GOOGLE_APPLICATION_CREDENTIALS` が指すキーファイルです。API キーは使いません。サーバーは起動時に呼び出しを行わないので、API やロールが足りないことは、読む人が翻訳を求めたときに初めてわかります。そのときサーバーは次のプロバイダーを試し、残りがなければ読む人には翻訳に失敗したことが表示されます。Google が報告するプロジェクトのクォータやレート制限でも、次のプロバイダーに移ります。

Google は、月ごとの無料枠を超えた分を送った文字数で課金します。[Cloud Translation の料金](https://cloud.google.com/translate/pricing)を参照してください。サーバー自身の月ごとの上限も文字数を数えますが、Google の請求とまったく同じではないので、Cloud Billing で予算アラートも設定してください。

Google は、テキストを翻訳の提供にだけ使い、メモリにごく短い間だけ保持し、モデルの学習には使わず、共有もしないと[述べています](https://cloud.google.com/translate/data-usage)。サーバーは Cloud Translation のグローバルエンドポイントを呼び出し、そこでは Google は処理を 1 つのリージョンに限定しません。

Google の[帰属表示の要件](https://cloud.google.com/translate/attribution)では、翻訳を機械翻訳として示し、その隣に `https://translate.google.com` へリンクする「powered by Google Translate」のバッジを表示することが求められます。Web クライアントは、このプロバイダーによる翻訳のそれぞれの隣に、そのリンク付きで「Google Translate」という名前を表示します。Google はバッジの画像をダウンロードでしか提供していないので、サーバーはこのプロバイダーのロゴをクライアントに渡しません。読む人に見えるものがあなたのインスタンスについて Google の要件を満たしているか確認してください。

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

Amazon Translate はプロバイダー `aws` です。サーバーは、ほかの AWS のサービスと同じく AWS SDK のデフォルトのチェーンの認証情報で、その `TranslateText` 操作を呼び出します。タスクやインスタンスの実行ロール、環境変数 `AWS_ACCESS_KEY_ID` と `AWS_SECRET_ACCESS_KEY`、または共有の認証情報ファイルです。サーバーの設定にキーを入れることはありません。

1. サーバーを動かすロールまたはユーザーに、IAM の権限 `translate:TranslateText` を与えます。言語のわからない投稿では、Amazon Translate は Amazon Comprehend で言語を判定します。Amazon Translate 用の AWS のマネージドポリシーは `comprehend:DetectDominantLanguage` も一緒に付与しているので、これも付与してください。
2. `translator_aws_region`（`MY_TRANSLATOR_AWS_REGION`）に Amazon Translate のリージョン（たとえば `us-east-1`）を設定します。言語の判定には、Amazon Comprehend も動いているリージョンが必要です。
3. サーバーを再起動し、`translation_providers = aws` で、単独で、またはほかのプロバイダーの中で好みの順にプロバイダーを有効にします（[インスタンスの設定](instance-settings.md)）。

Amazon Translate は送った文字数で課金し、最初の 12 か月には無料枠があります（[料金](https://aws.amazon.com/translate/pricing/)）。サーバーが送るのは投稿の言葉だけです。メンション、ハッシュタグ、リンク、コードはサーバーにとどまり、それぞれ HTML のマークアップの短いプレースホルダーとして送られます。`translation_user_monthly_characters` と `translation_server_monthly_characters` の月ごとの上限は、払ってもよい範囲に収めてください。

**AWS Organizations の [AI サービスのオプトアウトポリシー](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html)がアカウントに適用されていない限り、Amazon はテキストを保存し、自社のサービスの改善に使うことがあり、それが別のリージョンで行われる可能性もあります。** オプトアウトしていなければ、Amazon はそのテキストについて単なるあなたのサービス提供者ではありません。そのことをプライバシーポリシーに書き（[プライバシーポリシーのテンプレート](privacy-policy-template.md#the-services-you-choose)）、`translation_provider_visibilities` で `aws` に `private` や `direct` を許可しないでください。既定では、このプロバイダーが受け取るのは `public`、`local`、`unlisted` の投稿だけです。

ビルドタグ `no_translator_aws` でビルドしたサーバーにはこのプロバイダーがなく、`translator_aws_region` が設定されていると起動を拒否します（[ビルドタグ](../reference/build-tags.md)）。
