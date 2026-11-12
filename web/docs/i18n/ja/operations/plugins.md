<!-- source: plugins 4a60f7abbec9 -->
# プラグイン

プラグインは自分で動かす別のプログラムやサービスで、サーバーはネットワーク越しに限られた 1 つの仕事をそこに任せます。
サーバーはアドレスに接続するだけで、プラグインを起動、停止、監視することはありません。サイドカーとして、systemd のもとで、
または別のマシンで動かしてください。

既定では何も任せません。プラグインのアドレスがないサーバーは、接続を開かず、プロセスも起動しません。
ビルドタグ [`no_plugins`](../reference/build-tags.md) を使うと、プラグインクライアントがバイナリから取り除かれます。

![プラグインの位置づけ: my プロセスは 6 つのポートをプラグインに任せ、5 つの判定フックを問い合わせます。どれも、自分で動かすプラグインに接続する 1 つのプラグインクライアントを通ります。図の下部は、呼び出しが失敗したときにそれぞれがどうするかを示しています。下の一覧が各部分を説明しています。](images/plugins.svg)

図の上から順に説明します。

- **my プロセス**は、`role` にかかわらず、プラグインに 2 種類の呼び出しをします。
  - **任せるポート**: プラグインが組み込みの部分の代わりに仕事をします。それぞれ、設定が `plugin` を指定したときだけ有効になります。
    `translator`（インスタンス設定 `translation_providers = plugin`）、`billing`（`billing_sources` に `plugin` を含める）、
    `mailer`（`mail = plugin`）、`media_image`、`media_video`、`media_audio`（`media_image = plugin` など）、
    `key_manager`（`key_manager = plugin`）、`search`（`search = plugin`）です。
  - **[判定フック](decision-hooks.md)**: プラグインが何かを受け入れるかどうかを答えます。フックのモード `hook_<hook>_mode` が
    `off` でない限り問い合わせます。ほかのサーバーからの投稿には `inbound.status`、ほかのサーバーからのフォローには
    `inbound.follow`、登録には `signup.check`、アップロードには `media.check`、ローカルのアカウントの投稿には `local.status`、
    ほかのサーバーのアカウントのプロフィールには `inbound.actor`、ローカルのアカウントのプロフィールの変更には `local.profile` を使います。
- **プラグインクライアント**がすべての呼び出しを行います。種類ごとに宛先 `plugin_<kind>_address` が 1 つと期限
  `plugin_<kind>_timeout` があり、同時に実行する呼び出しは最大 8 件（メディアの各種類は 2 件）です。接続には Unix ソケット、`https://`、
  [一部の種類](#plain-tcp)に限り `http://` を使います。
- **自分のプラグイン**は、自分で動かす別のプログラムで、サイドカーとして、systemd のもとで、または別のマシンで動かします。サーバーは
  それに接続するだけです。
- **呼び出しが失敗したとき、または時間切れになったとき**（[詳細](#when-a-call-fails)）:
  - `billing`、`mailer`、メディアの種類: 呼び出しをしたジョブが後でもう一度実行されます。
  - `translator`: サーバーは次のプロバイダーに問い合わせ、残っていなければ、読む人には翻訳が失敗したことが表示されます。
  - `key_manager` は安全側に倒れます。鍵が必要なものはすべて失敗し、プラグインがないとサーバーは起動しません。
  - `search`: 書き込みのジョブは後でもう一度実行され、検索は失敗するので、利用者には検索が使えないことが表示されます。
  - 判定フックは `hook_<hook>_on_failure` の指定に従います。`pass`、`retry`、登録の場合は `require_approval` です。

<!-- source: kinds 73da92ef7563 -->
## 種類

種類ごとに、設定 `plugin_<kind>_*` のもとに独自の宛先があります。種類の名前のドットはキーでは `_` になるので、
`inbound.status` の宛先は `plugin_inbound_status_address` です。

| 種類 | 運ぶもの | 平文の TCP（`http://`） | 既定のプロトコル | 既定の期限 |
| --- | --- | --- | --- | --- |
| `translator` | 読む人が翻訳を求めた投稿の本文 | **拒否** | `json` | 10 秒 |
| `search` | `public` と `local` の投稿の本文、投稿者、公開範囲、言語と、ログインしている人のクエリ | **拒否** | `json` | 10 秒 |
| `billing` | インスタンスのコスト | 許可 | `json` | 2 分 |
| `mailer` | アドレスと、確認用リンクなどの秘密の情報 | **拒否** | `json` | 30 秒 |
| `media_image` | アップロードされた画像とそのメタデータ（位置情報を含むことがあります） | 許可 | `proto` | 4 分 |
| `media_video` | アップロードされた動画とそのメタデータ（位置情報を含むことがあります） | 許可 | `proto` | 4 分 |
| `media_audio` | アップロードされた音声ファイルとそのメタデータ | 許可 | `proto` | 4 分 |
| `key_manager` | 平文のデータ暗号化鍵 | **拒否** | `json` | 5 秒 |
| `inbound.status` | 届いた投稿の本文 | **拒否** | `json` | 3 秒 |
| `local.status` | ローカルの投稿の本文 | **拒否** | `json` | 1 秒 |
| `local.profile` | ローカルのアカウントの公開プロフィール（画像を除く） | 許可 | `json` | 1 秒 |
| `inbound.follow` | フォローの両者 | 許可 | `json` | 3 秒 |
| `inbound.actor` | ほかのサーバーのアカウントの公開プロフィール | 許可 | `json` | 3 秒 |
| `signup.check` | メールアドレスと IP アドレス | **拒否** | `json` | 2 秒 |
| `media.check` | メディアを処理した小さな版（元のファイルは送りません） | 許可 | `json` | 10 秒 |

サーバーに宛先を使わせる設定は、それぞれの機能に属します。表の最後の 5 つの種類は[判定フック](decision-hooks.md)です。
このページでは、それらに共通することを説明します。

サーバーは、1 つのメッセージで種類ごとに決まった上限までを送り、同じく上限までを読みます。上限を超える呼び出しは失敗し、
その呼び出しをしたジョブが再試行します。

<!-- source: the-destination f397f6e80550 -->
## 宛先

これらの設定は、設定ファイルと環境変数にだけあります
（[設定リファレンス](../reference/configuration.md)）。インスタンス設定、管理画面、API のどれからも表示や変更はできないので、
管理者アカウントが盗まれても、投稿や鍵の送り先を変えられることはありません。変更は再起動で反映されます。

`plugin_<kind>_address` は次の 3 つの形のいずれかです。サーバーは起動時に名前を解決せずに形を確認し、
接続のたびに名前を解決します。

- `unix:///run/my/mailer.sock`: Unix ソケット。
- `https://mailer.internal:8443`: TCP 上の TLS。サーバーは常に、`plugin_<kind>_tls_ca_file` またはシステムのルート証明書に対して
  証明書を検証し、これを無効にする設定はありません。
- `http://billing.internal:5000`: 平文の TCP。

<!-- source: authentication 2fe4df9369ac -->
### 認証

次のいずれかを選びます。

- **Unix ソケット:** ソケットのファイルのパーミッションで保護します。
- **mTLS:** `https://` のアドレスとともに `plugin_<kind>_tls_cert_file` と `plugin_<kind>_tls_key_file` を使います。
- **Bearer トークン:** `plugin_<kind>_token`（または `MY_PLUGIN_<KIND>_TOKEN_FILE`）を `Authorization: Bearer` として送ります。
  使えるのは `https://` か Unix ソケットだけです。`http://` のアドレスでトークンを使うと設定エラーになります。

トークンもクライアント証明書もない `https://` のアドレスでは、警告を出して起動します。プラグインは、サーバーとそこに到達できるほかの誰かを
区別できないためです。

サーバーは `plugin_<kind>_tls_ca_file`、`plugin_<kind>_tls_cert_file`、`plugin_<kind>_tls_key_file` のファイルを起動時に読み、
ファイルがないか読めないと起動が止まります。その後は、新しい接続を開くときに、最大 5 秒に 1 回ファイルの変更を確認します。
そのため、Vault、cert-manager、SPIFFE などの証明書マネージャーがその場で更新する短命の証明書や CA も、再起動なしで使えます。
すでに開いている接続は、閉じるまで開始時の証明書を使い続けます。新しい証明書はあるのに鍵がまだ置かれていないなど、更新が書きかけの間は、
サーバーは最後に読んだファイルを使い続け、次の接続でもう一度試します。トークンは起動時にだけ読まれます。

<!-- source: plain-tcp cbb88078173e -->
### 平文の TCP

`mailer`、`key_manager`、`translator`、`search`、`inbound.status`、`local.status`、`signup.check` は `http://` を拒否し、サーバーは起動しません。
これはループバック（`http://127.0.0.1`）でも同じです。ループバックアドレスでも、ホスト上のほかのプロセスが通信を読むのは防げないためです。
このようなサイドカーは、共有ボリューム上の Unix ソケットに置いてください。

ほかの種類は `http://` を受け付け、サーバーは起動時に種類とアドレスを示す警告をログに記録します。その通信は何によっても暗号化も認証もされず、
Bearer トークンも使えません。そのネットワークを閉じておくのは自分の責任です。上の表に挙げたものが、平文でそこを通ります。

<!-- source: protocols eb5a03718f1b -->
## プロトコル

`plugin_<kind>_protocol` はサーバーが送るものを指定し、プラグインが受け付けるものと一致している必要があります。

| 値 | サーバーが送るもの | 用途 |
| --- | --- | --- |
| `json` | `application/json` を使う Connect の unary プロトコル。平文の接続では HTTP/1.1 | Connect のライブラリ、または HTTP サーバーと JSON ライブラリだけで作ったプラグイン |
| `proto` | `application/proto` を使う Connect のプロトコル | Connect のライブラリで作ったプラグイン |
| `grpc` | HTTP/2 上の gRPC | gRPC しか話さないプラグイン |

プラグインは、違うプロトコルには `415` で応答します。メディアの種類は双方向にストリームし、それには HTTP/2 が必要なので、`proto`
か `grpc` を使います。`json` は設定エラーです。

プロトコルの定義は、リポジトリの `api/plugin/` 以下の `.proto` ファイルです。ポートの種類ごとにパッケージが 1 つあり、型は共有しません。
判定フックはすべて 1 つのパッケージ `my.plugin.hook.v1` を共有します。
[プラグインのプロトコル](../plugins/overview.md)に、それらのファイルから生成したすべてのパッケージのリファレンスがあります。

<!-- source: when-a-call-fails 1caa41ab429e -->
## 呼び出しが失敗したとき

呼び出しは、同じ種類の同時に実行される呼び出しの中で自分の順番を待ちます。その待ち時間は期限と、メトリクス
[`my.plugin.slot_wait`](running.md) に含まれます。失敗した呼び出しは、種類と Connect のコードごとにメトリクス [`my.plugin.failures`](running.md) に数えられます。
サーバーはプラグインがエラーに入れたメッセージを捨てるので、プラグインの失敗の理由はプラグイン自身のログにあります。サーバーは
呼び出しを自分では再試行しません。再試行するのは呼び出しをしたジョブなので、プラグインは同じ呼び出しを 2 回受けても対処できる必要があります。

サーバーの起動時にプラグインが停止していても、何も止まりません。例外は `key_manager` で、これがないとサーバーは起動しません。
[これがないと止まるもの](key-management.md#when-the-plugin-is-unreachable)は、その設定の一部として最初に読んでください。

呼び出しには `plugin_<kind>_timeout` の期限があり、プラグインはそれを `Connect-Timeout-Ms`（gRPC では `grpc-timeout`）として受け取ります。
プラグインはこのヘッダーを無視してもかまいません。いずれにしてもサーバーは待つのをやめます。期限は、リクエストが待つことのある種類
（`key_manager`、`translator`、`search`、`signup.check`、`media.check`、`local.status`、`inbound.actor`、`local.profile`）では 30 秒未満、
それ以外ではジョブが実行できる時間である 5 分未満にする必要があります。

<!-- source: test-a-plugin 5d8195bb27f5 -->
## プラグインをテストする

`my plugin test` は、動いているプラグインに対してある種類の契約を実行し、ケースごとに結果を報告します。設定は読まないので、
必要なのは `my` のバイナリだけです。

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

ケースが失敗すると終了コードは 1 になります。フラグの意味は設定と同じです: `--protocol`、`--token-file`、`--tls-ca-file`、
`--tls-cert-file`、`--tls-key-file`、`--timeout`。サーバーと同じ規則を適用するので、`--kind mailer` は `http://` のアドレスを拒否します。
テストするプラグインは本物でもかまわないので、呼び出しそのもの以上のことをするケースは、フラグで求めたときだけ実行されます。
そのバージョンにまだ契約がない種類は、`reachable` と `SKIP` を報告します。

`--kind billing` は、課金されるリクエストが許されていないときに、プラグインが何も費用をかけることを拒否するかだけを確認します。`--billed` を付けると、
今月と前月の分を実際に取得し、インポートと同じように応答を確認します。プラグインの背後のプロバイダーで費用がかかることがあります。
[クラウドの利用料金のインポート](billing-import.md#a-plugin-billing_sources--plugin)に、この種類の契約があります。

`--kind mailer` は、`--mail-to` で受信者を指定しない限り RPC を呼び出しません。指定すると、コマンドは `--mail-from`（既定は同じアドレス）から
そこへ実際に 1 通のメッセージを送り、受信者のないリクエストをプラグインが拒否することを確認します。
[メールの送信](email.md#a-plugin-mail--plugin)に、この種類の契約があります。

`--kind translator` は、決まった英語の文をいくつか日本語に翻訳させます（一部の言語の組しか持たないプラグインには、`--translate-from` と `--translate-to`
で別の組を選びます）。プラグインの背後のサービスが文字数で課金するなら、これらも課金されます。約 200 文字です。応答の形と、ISO 639 が
私用のために空けているコード `qaa` の言語をプラグインが拒否することを確認します。訳語の良し悪しは判断できないので、プラグインの翻訳は自分で読んでください。
[投稿の翻訳](#translating-posts)に、この種類の契約があります。

`--kind search` は検索を行い、自分のドキュメントだけを書き込み、取り除きます。どの投稿にもない ID で、どのアカウントでもない投稿者の、
実行のために作った語を含むドキュメントをいくつか作り、最後に取り除きます。語のないクエリが何も見つけないこと、エンジンのクエリ言語や
日本語のクエリが失敗しないこと、自分が発行していないカーソルをプラグインが拒否すること、ドキュメントが新しい順にページごとに見つかり、
置き換えられ、投稿ごとと投稿者ごとに取り除かれることを確認します。`--clear-index` を付けると、ある公開範囲のすべてのドキュメントと
インデックス全体を取り除くケースも実行します。これはテスト用のインデックスに対してだけ使ってください。[投稿の検索](#searching-posts)に、この種類の契約があります。

`--kind media_image`、`--kind media_video`、`--kind media_audio` は、自分で作った小さなテストファイルだけを送ります。
写真、絵文字、アニメーション GIF、映像と音声のトラックを持つ MP4、または MP3 です。どれもメタデータに位置情報やタイトルを隠しており、
それが出力に戻ってくるとケースは失敗します。すべての出力をサーバーと同じように確認します。
[メディアの変換](#converting-media)に、これらの種類の契約があります。

`--kind key_manager` は、サーバーが起動のたびに行う確認を実行し、変更された値と、自分が持っていない KEK をプラグインが拒否することを確認します。
送るのはその場で作って捨てる鍵だけなので、呼び出しに課金することのある本物の鍵ストアに対しても安全です。確認するのは応答の形が正しいことで、
プラグインが KEK をどう保持しているかではありません。
[鍵の管理](key-management.md#a-plugin)に、この種類の契約があります。

`probe` は最も小さな種類で、サーバーはこれに何も任せません。残りのプラグインを書く前に、まずこれを提供して、トランスポート、
プロトコル、認証を確認してください。

<!-- source: translating-posts c9d31b2b9157 -->
## 投稿の翻訳

種類 `translator` のプラグインを使うと、[組み込みのプロバイダー](translation.md)（LibreTranslate、DeepL、Google Cloud Translation、
Amazon Translate）以外の自分で選んだサービスを通して、読む人が求めたときにサーバーが投稿を翻訳できます。サーバー側のプロバイダーがなければ、
翻訳は読む人のブラウザーに組み込まれたものだけです。これはデスクトップの Chrome、Edge、Opera にしかないので、スマートフォン、Firefox、Safari で
読む人には、プロバイダーを有効にするまで翻訳ボタンが表示されません。

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. `plugin_translator_address` と `plugin_translator_name` を設定し、再起動します。読む人は翻訳を出どころによって信頼するので、
   名前は必須です。アドレスが設定の外に出ることはありません。
2. [プライバシーポリシーを更新](privacy-policy-template.md#the-services-you-choose)し、プラグインの背後のサービスを翻訳サービスとして
   挙げてください。プラグインが渡すのは、プラグインが受け取ったものです。
3. [インスタンス設定](instance-settings.md) `translation_providers = plugin` で有効にします。変更はすぐに反映されます。

アドレスがあるだけでは何も有効になりません。設定が `plugin` を指定するまで、サーバーは何も送らず、読む人にはボタンが表示されません。

このプラグインは、ほかと同じ 1 つのプロバイダーです。[`translation_provider_visibilities`](instance-settings.md) が受け取ってよいものを決め、
月ごとの上限は送られたものを数え、プラグインが作った翻訳はほかと同じようにサーバーのキャッシュに残ります。
`translation_reports_to_server` を有効にすると、モデレーターが通報の中で翻訳を求めたものも、同じ公開範囲に従って受け取ります。
`translation_providers` の順序の中で、ほかのプロバイダーの前にも後にも置けます。

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### プラグインが受け取るもの

プラグインは、読む人が翻訳を求めたときに投稿のテキストを受け取ります。本文、注意書き、投票の選択肢、画像の説明と、
テキストの言語と翻訳先の言語です。投稿のアドレス、投稿者、読む人、メンション、ハッシュタグ、リンク、カスタム絵文字、コードは受け取りません。
それらの代わりに番号付きのマーカーが入ります。

既定では、対象は `public`、`local`、`unlisted` の投稿です。**`private` と `direct` の投稿がプラグインに届くのは、許可したときだけです。**
許可するには `translation_provider_visibilities = plugin=public|local|unlisted|private|direct` とします。そうすると、プラグインと
その背後のサービスは、投稿者が少数の人に向けたテキストを読むことになるので、テキストの行き先を自分で管理できる場合にだけ許可し、
プライバシーポリシーにそう書いてください。サーバーは、一度送ったテキストを取り戻せません。

テキストは投稿のテキストなので、サーバーはこの種類で `http://` を、ループバックでも拒否します。Unix ソケットか
`https://` を使ってください。

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### プラグインの背後の LLM

サーバーは LLM のプロバイダーを持たず、提供もしません。これは意図的なものです。投稿は誰でも書けるので、翻訳するテキストは LLM に指示を与える
手段（プロンプトインジェクション）になり、読む人は翻訳を投稿の内容だと受け取ります。サニタイズが守るのはページの HTML で、
投稿が言っていないことを翻訳が言うのを止めることはできません。

サーバーには、プラグインが何を呼び出しているかは見えません。**プラグインが LLM を使うなら、このリスクとその結果は自分の責任です。**

<!-- source: the-contract ef518f876dd2 -->
### 契約

RPC は `my.plugin.translator.v1.TranslatorService` の `Translate` です。

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

- `sourceLanguage` は BCP 47 のタグで、サーバーが言語を知らないときは `und` です。その場合、プラグインは言語を判定して
  `detectedLanguage` で報告するか、`OUTCOME_UNSUPPORTED_LANGUAGE` で応答します。プラグインに付け加えることがなければ `detectedLanguage` は
  なくてもかまいません。言語タグでない値は、サーバーが無視します。
- `translations` には断片ごとに 1 つ、断片の順に、空でない項目が入ります。断片がマーカーだけのこともあり、その翻訳はそのマーカーになります。
- `⟦1⟧` は 1 つの項目を表すか、`⟦/1⟧` で閉じる組を開きます。断片の各マーカーは、その翻訳にそのままの形でちょうど 1 回現れます。
  項目は語順に合わせて動いてもかまいませんが、組は閉じる前に開いている必要があり、組は入れ子になります。
- 翻訳はプレーンテキストです。サーバーはその中のマークアップをエスケープし、保存や表示の前に結果をサニタイズします。

| `outcome` | 意味 | サーバーの動作 |
| --- | --- | --- |
| `OUTCOME_OK` | 翻訳が応答に含まれている | 確認して保存し、読む人に応答する |
| `OUTCOME_TEMPORARY` | サービスが失敗した、または応答しなかった | 次のプロバイダーを試す |
| `OUTCOME_PROVIDER_LIMIT` | サービスが独自のレート制限や割り当ての超過を報告した | 次のプロバイダーを試す |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | サービスが翻訳元や翻訳先の言語を持たない、または判定できない | 次のプロバイダーを試し、どれも持たなければ読む人に伝える |

これらの規則に反する応答は、`outcome` がない、または不明な場合や呼び出しが失敗した場合と同じく `OUTCOME_TEMPORARY` として扱われ、
何も保存されません。どのプロバイダーも応答しないと、読む人には翻訳が失敗したことが表示され、投稿はそのままです。ただし
`translation_browser` が有効で、読む人のブラウザーが投稿を翻訳できるときは、Web クライアントがそこで翻訳します。
サービスが受け取った可能性があるので、各呼び出しの文字数は、失敗した呼び出しのものも月ごとの上限に数えられます。

サーバーは `plugin_translator_timeout` の後に、またその設定にかかわらず 20 秒の後に、待つのをやめます。1 つのリクエストのすべての
プロバイダーには、合わせて 45 秒が与えられます。失敗した呼び出しは再試行せず、同じ断片がほかの読む人のためにもう一度送られることがあります。

<!-- source: searching-posts bd9dfba5a3e7 -->
## 投稿の検索

種類 `search` のプラグインは、Groonga や PGroonga、Sudachi で作ったインデックス、Typesense、Manticore、ホスティングされた検索サービスなど、
サーバーに組み込みの対応がないエンジンに、[投稿の全文検索](search.md)のインデックスを保持します。サーバーにとっては、もう 1 つのエンジンです。
同じ投稿がインデックスに入り、同じジョブがインデックスを最新に保ち、各人が何を見てよいかは引き続きサーバーが決めます。

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. プラグインを動かし、[`my plugin test --kind search`](#test-a-plugin) で確認します。
2. `search = plugin` と `plugin_search_address` を設定し、ほかのエンジンと同じように[有効にする](search.md#turning-it-on)に従います。
   インデックスはそれ以降に埋まっていき、`my search rebuild` で既存の投稿が追加されます。
3. プラグインが投稿をほかの会社のサービスに送るなら、そのサービスを
   [プライバシーに関する文書](search.md#your-privacy-documents)に書いてください。

サーバーは起動時に接続を開かないので、プラグインが停止していても止まるのは検索だけです。プラグインへの移行やプラグインからの移行は、
[エンジンを無効にする・変更する](search.md#switching-off-or-changing-the-engine)に従ってください。

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### プラグインが受け取るもの

- **インデックスの投稿。** インデックスが保持する各投稿について、プラグインはそのテキスト（本文、注意書き、画像の説明をプレーンテキストで）、
  ID、投稿者のアカウント ID、公開範囲、言語、公開日時を受け取ります。対象は、[`search_visibilities`](instance-settings.md) のとおり
  `public` と `local` の投稿だけで、検索を許可している投稿者のものに限られます。`local` の投稿はこのサーバーのアカウントだけに
  向けたものですが、プラグインとその背後のサービスもそれを読みます。`unlisted`、`private`、`direct` の投稿が届くことはありません。
- **クエリ。** ログインしている人が投稿を検索すると、プラグインは入力された内容を、ページのカーソルと結果の件数とともに受け取ります。
  誰が検索したかは受け取りません。
- **削除。** 投稿 1 件、あるアカウントのすべての投稿、ある公開範囲のすべての投稿、インデックス全体の削除です。

プラグインが返すのは投稿 ID だけです。サーバーは各投稿をデータベースから読み直し、その人がいま見てよいものだけを表示するので、
プラグインが見てはいけない人に投稿を見せることはできず、インデックスが遅れていても見つかるものが減るだけです。また、
サーバーがプラグインのエラーを誰かに表示することはありません。

投稿のテキストと人々が検索する内容を運ぶので、サーバーはこの種類で `http://` を、ループバックでも拒否します。Unix ソケットか
`https://` を使ってください。

<!-- source: the-contract-1 d500365750c4 -->
### 契約

サービスは `my.plugin.search.v1.SearchService` で、操作ごとに RPC が 1 つあります。

| RPC | 内容 |
| --- | --- |
| `Replace` | 投稿のドキュメントを追加する、または同じ `statusId` のドキュメントを置き換える |
| `Remove` | 投稿のドキュメントを取り除く |
| `RemoveByAuthor` | あるアカウントのすべてのドキュメントを取り除く |
| `RemoveByVisibility` | ある公開範囲（`VISIBILITY_PUBLIC` または `VISIBILITY_LOCAL`）のすべてのドキュメントを取り除く |
| `Clear` | すべてのドキュメントを取り除く。インデックスはその後もドキュメントを受け付ける |
| `Search` | クエリが見つけたドキュメントの ID を 1 ページ分、新しい順に返す |

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

- **クエリはプレーンテキストです。** プラグインはクエリを語に分け、すべての語を持つドキュメントを見つけます。テキストをエンジンの
  クエリ言語として解釈することはありません。その中の演算子、ワイルドカード、引用符、フィールド名はただのテキストです。語のないクエリは
  何も見つけません。日本語などの分かち書きをしない言語で語をどう見つけるかはプラグインが選ぶことで、プラグインを使う理由でもあります。
- **ページは新しい順です。** `statusIds` は降順で、それぞれ最大 1 回、最大 `limit` 件で、どれもサーバーが送ったとおりの投稿 ID
  （36 文字の小文字）です。ID は UUIDv7 なので、文字列としての順序が時間の順序になります。
- **カーソルはプラグインのものです。** `nextCursor` は最後のページでは空で、それ以外では空白を含まない印字可能な ASCII で最大 256 バイト、
  たとえばページの最後の ID です。サーバーは次のページのためにそれをそのまま送り返し、次のページは、ドキュメントの追加や削除があっても
  その ID の後から続きます。最後でないページの ID が `limit` 件より少なくてもかまいません。プラグインが発行していないカーソルには
  `OUTCOME_INVALID_CURSOR` を返します。
- **書き込みは次の検索に反映されます。** また、どの呼び出しも 2 回来ることがあります。ないものの削除は成功します。リフレッシュの後でしか
  書き込みが反映されないエンジンでは、プラグインがそれを待ちます。

| `outcome` | 意味 | サーバーの動作 |
| --- | --- | --- |
| `OUTCOME_OK` | 呼び出しが求めたことを行った | ページの ID に閲覧ポリシーを適用する |
| `OUTCOME_FAILED` | エンジンが失敗した、または応答しなかった | 書き込みはそのジョブで再試行し、検索は失敗させる（利用者はもう一度試せる） |
| `OUTCOME_INVALID_CURSOR` | `Search` のみ: カーソルがプラグインの発行したものではない | カーソルが間違っていることをクライアントに伝える |

`outcome` がない、または不明な場合、呼び出しが失敗した場合、これらの規則に反するページは `OUTCOME_FAILED` として扱われ、
そのようなページのものは何も表示されません。呼び出しは最大で `plugin_search_timeout`（既定は 10 秒）続き、その設定にかかわらず最大 10 秒です。

<!-- source: converting-media 36df3dbdb21b -->
## メディアの変換

メディアのプラグインは、アップロードの処理の重い部分を行い、それを別のマシンで動かせるようにします。種類は 3 つあり、
それぞれ別に選べます。

- `media_image` は、画像を各バリアントに再エンコードします。
- `media_video` は、動画（音声の有無にかかわらず、映像トラックを持つファイル）を再多重化します。
- `media_audio` は、音声だけのファイルを再多重化します。

残りは引き続きサーバー自身が行います。各アップロードの形式を判定し、動画と音声ファイルを見分け、届いた時点で各アップロードを
上限と照らし合わせ、リモートのメディアのプレビューを作ります。設定が `native` のままの種類は、サーバー内で処理されます。

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

任せる種類ごとに `media_<kind> = plugin` と `plugin_media_<kind>_address` を設定し、再起動します。1 つのプラグインが複数の種類を
受け持ってもかまいません。その場合はどの種類にも同じアドレスを指定します。各種類は同時に最大 2 件の呼び出しを実行し、各呼び出しは最大で
`plugin_media_<kind>_timeout`（既定は 4 分）続くので、遅い動画が画像を足止めすることはありません。

- **メタデータの除去はプラグインの仕事です。** アップロードは、送られたときのまま、写真を撮った場所を含むことのある EXIF データやタグとともに
  プラグインに届きます。サーバーはすべての出力の形式と大きさを確認しますが、メタデータの痕跡がすべて消えたかどうかは分かりません。
  `my plugin test --kind media_image` はテストファイルの位置情報が戻ってこないことを確認します。残りはプラグインで確認してください。
- **平文の TCP では、アップロードが暗号化されずに送られます。** メディアの種類は、起動時の警告とともに `http://` を受け付けます。その場合、
  アップロードはメタデータとともに平文でネットワークを通り、どちらの端も認証されません。そのネットワークにいる人は誰でもそれを読んだり、
  プラグインの代わりに応答したりできます。ネットワークが自分だけのものでない限り、Unix ソケットか `https://` を使ってください。
- **サーバーは出力を信頼しません。** 各出力の形式は、プラグインの申告ではなく内容から判定し、その形式の Content-Type とファイル拡張子で
  提供します。各出力はアップロードと同じ上限のもとでデコードし、大きさに収まっていることを確認します。申告やそれらの上限より大きい出力、
  プラグインが申告していない形式の出力、バリアントが受け付けない形式の出力は、サーバーが処理できないファイルとしてアップロードを失敗させます。
- **失敗は遅延です。** プラグインが停止しているか失敗すると、組み込みの処理と同じく、ジョブが後で再試行します。リモートの絵文字のコピーや
  絵文字のリクエストの承認など、カスタム絵文字の画像がすぐに必要な操作は、プラグインが失敗している間は失敗します。

<!-- source: the-contract-2 812c987c276c -->
### 契約

サービスは `my.plugin.media.v1.MediaService` で、RPC は画像のための `Process` と、動画や音声ファイルのための `Remux` です。
どちらも双方向にストリームするので、プラグインには Connect か gRPC のライブラリと HTTP/2 が必要です。`proto` か `grpc` を、ALPN
`h2` の TLS 上、または Unix ソケットや平文の TCP 上の平文の HTTP/2（h2c）で使います。

呼び出しは半二重です。

1. サーバーが `head` を送ります。上限と、画像の場合はその用途、3 つの大きさの枠、焦点です。ファイルは最大 1 MiB の
   `chunk` メッセージで続きます。その後、サーバーは自分の側を閉じます。
2. プラグインは、`outcome` に結果を入れた `head` で応答します。`OUTCOME_OK` の場合、`Process` の head には、EXIF の向きを適用した後の
   画像の寸法と、小さな静止画の出力の blurhash も入ります。
3. プラグインは出力ごとに、出力の名前と、その形式、寸法、長さの申告を含む `part` を送り、
   続いて最大 1 MiB の `chunk` メッセージを送ります。

`Process` は、小、中、大の静止画の出力と、複数のフレームを持つ画像ならその 3 つの動く出力を返します。小と中の出力は WebP で、大の静止画は
JPEG か PNG（カスタム絵文字では常に PNG）、大の動く出力は GIF です。`Remux` は、メタデータを除いたファイルを元のコンテナーのまま返し、
埋め込まれたカバー画像があればそれも返します。サーバーが読むのは、`Process` からは画像の上限の 2 倍まで、`Remux` からはファイルについては
動画の上限まで、カバーについては画像の上限までです。

| `outcome` | 意味 | サーバーの動作 |
| --- | --- | --- |
| `OUTCOME_OK` | 出力が続く | 確認して保存する |
| `OUTCOME_TEMPORARY` | プラグインが失敗した | 後でジョブを再試行する |
| `OUTCOME_UNSUPPORTED_FORMAT`、`OUTCOME_UNSUPPORTED_CODEC`、`OUTCOME_TOO_LARGE`、`OUTCOME_DIMENSIONS`、`OUTCOME_CORRUPT`、`OUTCOME_TOO_LONG` | ファイルを受け付けられない | アップロードを失敗として記録する |

`outcome` がない、または不明な場合と、呼び出しが失敗した場合は `OUTCOME_TEMPORARY` として扱われます。

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## ライブラリを使わないプラグイン

`json` を使うなら、プラグインに必要なのは HTTP サーバーと JSON ライブラリだけです。RPC は `Content-Type: application/json` 付きの
`/<package>.<Service>/<Method>` への `POST` で、プラグインは `200` と JSON オブジェクトで応答します。

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

JSON は proto3 の標準のマッピングに従います。

- `int64` は 10 進の文字列、`bytes` は base64、列挙型はその名前です。
- ゼロ値のフィールドは、どちらの方向でも省略されることがあります。プラグインは、ないフィールドをゼロとして読む必要があります。
- サーバーは知らないフィールドを無視します。
- 結果は応答の `outcome` です。`outcome` がない、または不明な場合は失敗であり、成功になることはありません。
- 失敗したプラグインは、HTTP のエラー、または対応するステータスとともに Connect の `{"code":"unavailable"}` で応答します。ジョブが再試行します。

`Connect-Timeout-Ms` と `Connect-Protocol-Version` は無視してもかまいません。
