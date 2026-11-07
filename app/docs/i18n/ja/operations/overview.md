<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# mycroblog サーバーの概要

サーバーを運用する前に最初に読むページです。mycroblog は ActivityPub でフェディバースのほかのサーバーと連合するマイクロブログで、`my` はそのサーバーソフトウェアです。これは 1 つのバイナリで、サーバー、ジョブワーカー、Web クライアント、そしてそれらを管理するコマンドを含みます。このページでは、サーバーが何でできているか、設定で何を選ぶか、小さなサーバーがどのように大きなサーバーへ育つか、次に何を読めばよいかを説明します。

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## サーバーの構成

![mycroblog サーバーの構成図。クライアントは HTTPS で my のプロセスにつながり、プロセスは設定で選んだ部品と、有効にしたサーバー外のサービスを使います。各部品は以下の節で説明します。](images/architecture.svg)

図の上から順に説明します。

- **接続してくるもの。** 人はブラウザで Web クライアントを使い（アプリとしてインストールすることもできます）、アプリやボットはクライアント API を使い、フェディバースのほかのサーバーは ActivityPub で投稿を配送・取得します。いずれも HTTPS でサーバーにつながります。
- **サーバーの前段。** `my` は平文の HTTP を話すので、その前に置いたリバースプロキシやロードバランサーで HTTPS を終端します。メディアファイルの前に CDN を置くこともできます（任意）。
- **`my` のプロセス。** 各プロセスは `my serve` を実行し、その [`role`](../reference/configuration.md#role) で何を動かすかが決まります。`web` は HTTP サーバー、`worker` はジョブワーカー（投稿の配送、ほかのサーバーからの取得、メールの送信、メディアの処理、定期ジョブの実行を行います）、既定値の `all` は 1 つのプロセスでその両方を動かします。
- **選ぶ部品。** 図の中央にある各部品には、そのアダプターを選ぶ設定が 1 つずつあります。[次の節](#parts-you-choose-by-configuration)に一覧があります。
- **サーバー外のサービス。** CAPTCHA、通話、支払い、プラグイン、クラウド費用の取り込みは、それぞれ設定するまで無効で、設定するとサーバーは指定したサービスを呼び出します。

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## 設定で選ぶ部品

どの部品も[設定リファレンス](../reference/configuration.md)の設定項目で、TOML ファイルか環境変数（`MY_` にキーを大文字にしたもの。例: `MY_DATABASE`）で設定します。各一覧の最初の選択肢が既定値です。データを保持する部品は、最初の起動の前に選んでください。設定を変えても、データが自動で移ることはありません。

| 部品 | 設定 | 選択肢 | 次に読むもの |
| --- | --- | --- | --- |
| データベース | [`database`](../reference/configuration.md#database) | `sqlite`（ファイル）、`postgres`（PostgreSQL と互換サービス） | [データベース](database.md)、[バックアップ](backup.md) |
| ジョブキュー | [`queue`](../reference/configuration.md#queue) | `db`（データベースのテーブル）、`inline`（各ジョブを同じプロセスですぐに実行します。テストと開発用）、`cloudtasks`（ジョブごとに Google Cloud Tasks がサーバーを起こします。PostgreSQL が必要） | [ジョブキュー](running.md#the-job-queue-and-failed-dead-jobs) |
| キャッシュとレート制限のためのキーバリューストア | [`kv`](../reference/configuration.md#kv) | `db`（データベースのテーブル）、`memory`（プロセスごとに独立）、`valkey`（Valkey またはほかの Redis プロトコルのサーバー。全プロセスで共有） | [`kv`](../reference/configuration.md#kv) |
| メディアの保存先 | [`blob`](../reference/configuration.md#blob) | `fs`（ディレクトリ）、`s3`（Amazon S3、Cloudflare R2、MinIO、Garage など S3 互換のストア）、`gcs`（Google Cloud Storage） | [Docker Compose のファイル](running.md#docker-compose-files)、[メディアのバックアップ](backup.md#media-in-a-bucket) |
| メディアの処理 | ファイルの種類ごとに 1 つずつ [`media_image`](../reference/configuration.md#media_image)、[`media_video`](../reference/configuration.md#media_video)、[`media_audio`](../reference/configuration.md#media_audio) | `native`（サーバー内）、`plugin`（あなたのプラグインがその種類のファイルを変換します） | [メディアの変換](plugins.md#converting-media) |
| プロセス間のリアルタイムストリーム | [`stream`](../reference/configuration.md#stream) | `hub`（1 つのプロセス内）、`postgres`（`LISTEN` / `NOTIFY`）、`valkey`、`sns`（Amazon SNS と SQS）、`pubsub`（Google Cloud Pub/Sub） | [`stream`](../reference/configuration.md#stream) |
| アカウントの秘密鍵を暗号化する鍵管理 | [`key_manager`](../reference/configuration.md#key_manager) | `local`（あなたが保管する鍵暗号化鍵）、`gcpkms`（Google Cloud KMS）、`awskms`（AWS KMS）、`plugin` | [鍵の管理](key-management.md) |
| メール | [`mail`](../reference/configuration.md#mail) | `none`、`smtp`、`ses`（Amazon SES）、`plugin` | [メールの送信](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`、`turnstile`（Cloudflare Turnstile）、`hcaptcha`、`recaptcha`（Google reCAPTCHA v2） | [CAPTCHA](captcha.md) |
| 通話 | [`voice`](../reference/configuration.md#voice) | `off`、`p2p`（ブラウザ同士。TURN は `voice_turn` で `none`、`coturn`、`cloudflare` のいずれか）、`livekit`、`sora` | [通話](voice.md) |
| サポーターの支払い | [`payments`](../reference/configuration.md#payments) | `none`、`stripe`、`payjp`（PAY.JP） | [サポーター](supporters.md)、[Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md) |
| プラグイン | `plugin_<kind>_address` | 既定ではなし。種類ごとに 1 つのアドレス: `translator`、`search`、`billing`、`mailer`、`media_image`、`media_video`、`media_audio`、`key_manager`、および判定フックの `inbound.status`、`local.status`、`inbound.follow`、`signup.check`、`media.check`、`inbound.actor`、`local.profile` | [プラグイン](plugins.md)、[判定フック](decision-hooks.md) |
| 投稿の翻訳 | `translator_<provider>_*`、[`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | なし（ブラウザ自身の翻訳だけ）、`libretranslate`、`deepl`、`google`（Google Cloud Translation）、`aws`（Amazon Translate）、または選んだサービスの前に置くあなたのプラグイン | [投稿の翻訳](translation.md)、[翻訳プラグイン](plugins.md#translating-posts) |
| 投稿の全文検索 | [`search`](../reference/configuration.md#search) | `none`、または `elasticsearch`、`opensearch`、`meilisearch`（[検索エンジン](search-engines.md)）、またはほかのエンジンの前に置くあなたのプラグインである `plugin` | [全文検索](search.md)、[検索プラグイン](plugins.md#searching-posts) |
| メディアの CDN | [`media_origin`](../reference/configuration.md#media_origin) | 未設定（サーバーが自身のオリジンからメディアを配信します）、または CDN のオリジン | [CDN からメディアを配信する](cdn.md) |
| クラウド費用 | [`billing_sources`](../reference/configuration.md#billing_sources) | なし、または `aws`（AWS Cost Explorer）、`gcp`（Google Cloud の請求データのエクスポートまたは予算）、`plugin` のいずれか（複数可） | [クラウド費用の取り込み](billing-import.md) |
| メトリクス | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`、`prometheus`、`otlp`（OpenTelemetry）、`emf`（CloudWatch） | [メトリクス](running.md#metrics)、[アラート](alerts.md) |
| トレース | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`、`otlp` | [トレース](running.md#traces) |
| エラーレポート | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none`（ログ行とエラーグループだけ）、`sentry`（Sentry、GlitchTip、Datadog）、`otlp` | [エラーレポート](error-reporting.md) |

リリースされたバイナリとコンテナイメージには、表のアダプターがすべて含まれています。自分でビルドするバイナリでは、使わないアダプターを[ビルドタグ](../reference/build-tags.md)で外せます。その場合、起動時にそのアダプターの設定は拒否されます。

`my config check` は、サーバーを起動する前に、設定の問題をまとめてすべて報告します（[本番運用のチェックリスト](production-checklist.md)）。

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## 小さなサーバーから大きなサーバーへ

![小さなサーバーから大きなサーバーへの 3 つの段階。SQLite とファイルシステムの上の 1 つのプロセス、次に PostgreSQL と CDN の後ろのオブジェクトストレージ、次にストリームのブローカーを使う web と worker の別々のプロセス。下の一覧が各段階を説明しています。](images/growth.svg)

1. **小さく始める。** `role = all` の 1 つのプロセスで、SQLite を使い、メディアはファイルシステムに置き、そのほかは既定値のままにします。キューとキーバリューストアはデータベースに、リアルタイムストリームはプロセス内に置かれます。`deploy/docker` の Docker Compose のファイルで、この構成を 1 台のマシンで起動できます（[Docker Compose でのデプロイ](deploy-docker-compose.md)）。個人や小規模のサーバーに向いています。`stream = hub` は自分が動いているプロセスにしか届かないので、この構成は 1 つのプロセスのままです。
2. **データを外に出す。** データベースに PostgreSQL、メディアに S3 互換のバケットか Google Cloud Storage を使い、メディアの前に CDN を置きます。サーバーはまだ 1 つのプロセスですが、1 つのディスクには依存しなくなり、次の段階のプロセス同士でデータを共有できます。できれば最初の起動の前に決めてください。あとから SQLite とファイルシステムからデータを移すのは、それだけで 1 つの作業になります（移すべきものは[バックアップする対象](backup.md#what-to-back-up)に一覧があります）。
3. **スケールアウトする。** リクエスト用に `role = web`、ジョブ用に `role = worker` のプロセスを、それぞれ必要な数だけ動かし、ワーカーをジョブの種類ごとに分けます（[ワーカーの分割](running.md#splitting-workers-by-job-type)）。プロセスが複数あるときは、リアルタイムストリームにすべてのプロセスに届くブローカー（`postgres`、`valkey`、`sns`、`pubsub`）が必要です。また、`memory` ではレート制限がプロセスごとに別々になるので、キーバリューストアは共有するもの（`db` か `valkey`）にしてください。ジョブキューは PostgreSQL のままにするか、Google Cloud Tasks に移します（[Cloud Tasks](running.md#cloud-tasks)）。

[Cloud Run へのデプロイ](deploy-cloud-run.md)は、2 番目と 3 番目の段階の間にある構成の例です。Cloud Run の上の `role = all`、Neon の PostgreSQL、Cloud Storage のメディア、ジョブには Cloud Tasks、ストリームには Pub/Sub を使い、誰もサーバーを使っていない間はインスタンスが 1 つも動きません。

<!-- source: where-to-go-next 2f1d25c01f8a -->
## 次に読むもの

運用者向けのガイドは次の順に読んでください。

1. **始める:** [サーバーを動かす](running.md)か[Cloud Run にデプロイし](deploy-cloud-run.md)、利用者に公開する前に[本番運用のチェックリスト](production-checklist.md)を確認します。
2. **インスタンスを設定する:** [設定](instance-settings.md)、[登録](registration.md)、[外部のログインプロバイダー](login-providers.md)、[メール](email.md)、[CAPTCHA](captcha.md)、[CDN](cdn.md)、[リンクプレビュー](link-previews.md)、[検索エンジンと AI クローラー](crawlers.md)。
3. **機能を有効にする:** [モデレーション](moderation.md)、[リレー](relays.md)、[Webhook](webhooks.md)、[プラグイン](plugins.md)と[判定フック](decision-hooks.md)、[通話](voice.md)、[全文検索](search.md)、[サポーター](supporters.md)とその支払い、[クラウド費用の取り込み](billing-import.md)。
4. **日々運用する:** [監視](monitoring.md)、[アラート](alerts.md)、[エラーレポート](error-reporting.md)、[メンテナンス](maintenance.md)、[バックアップ](backup.md)、[データベース](database.md)、[鍵暗号化鍵](key-management.md)、[管理者とロール](admin-cli.md)。
5. **法務とサービスの終了:** [プライバシーポリシー](privacy-policy-template.md)と[処理活動の記録](processing-record-template.md)のテンプレート、[サービスの終了](service-closure.md)。

[設定リファレンス](../reference/configuration.md)にはすべての設定項目が、[ジョブリファレンス](../reference/jobs.md)にはワーカーが実行するすべてのジョブが載っています。
