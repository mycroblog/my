<!-- source: deploying-on-google-cloud 03b87618467f -->
# Google Cloud へのデプロイ

OpenTofu のモジュール `mycroblog-gcp` で、Google Cloud 上に `my` を動かす方法を説明します。Cloud Run のサービスと、役割ごとのバックエンドで構成し、バックエンドはセクションの `kind` で選びます。どのセクションも既定値はモジュールの推奨で、誰もサーバーを使っていないあいだはインスタンスが 1 つも動きません。ただし、サーバーを毎分起こす tick だけは動きます。サーバーには費用がかかります。推奨の構成で Neon にデータベースを置くと、ストレージとトラフィックを除いて月に約 $37 で、各選択肢の金額は[費用](#costs)に書いてあります。

OpenTofu のデプロイに共通するもの、つまりブロックとそのセクション、シークレット、コンソール、アップグレードは、[OpenTofu でのデプロイ](deploy-opentofu.md)にあります。このページは、Google Cloud に必要なことを補います。

<!-- source: what-it-creates 7b13c8bbc968 -->
## 作られるもの

`mycroblog-gcp` の 1 つのブロックで、次のものが作られます。各行の最初の種類が推奨で、既定値です。すべてのフィールドは[モジュールのリファレンス](../../deploy/opentofu/README.md#sections)にあります。

| 役割 | セクション | 推奨 | ほかの種類 |
| --- | --- | --- | --- |
| サーバー | `service`、`jobs` | `role = all` の Cloud Run サービス、マイグレーションのジョブ、必要なあいだだけ使うコンソールのジョブ、サービスアカウント、公開アクセス | |
| トラフィック | `ingress` | `load-balancer`: Google が管理する証明書を持つ、グローバル外部アプリケーションロードバランサー | `domain-mapping`、`cloudflare`、どれとも併用できる Cloudflare 上のキャッシュ付きメディアホスト |
| データベース | `database` | `external`: Neon など、自分で動かす PostgreSQL の URL を入れるシークレット | `cloud-sql`: インスタンス、そのデータベース、ユーザー、パスワード |
| ジョブ | `queue` | `cloud-tasks`: Cloud Tasks のキュー、tick の Cloud Scheduler ジョブ、サーバーを呼ぶサービスアカウント | `db`: データベースの中のジョブ |
| メディア | `media` | `gcs`: 非公開の Cloud Storage バケット | `r2`、`s3` |
| キャッシュとレート制限 | `kv` | `db`: データベースの中。作るものはありません | `memorystore`、`external`（Valkey） |
| リアルタイム更新 | `stream` | `pubsub`: トピックと、サーバーが自分のサブスクリプションを管理するためのカスタムロール | `postgres`、`valkey` |
| メール | `mail` | 任意の SMTP サービスと、そのパスワードのシークレット | |
| KEK | `kek` | `secret-manager`: 生成されるシークレットをファイルとしてマウント | `kms`: Cloud KMS の鍵 |
| ネットワーク | `network` | `none`: 接続するものはありません | `vpc`、`existing` |
| メトリクス、トレース、エラー | `observability` | Cloud Monitoring、Cloud Trace、Error Reporting。Cloud Run が送るものを読みます。モジュールがそれらの API を有効にします | `settings`（Sentry など） |
| アラート | `monitoring` | 死活監視のチェック、ログベースのメトリクス、アラートポリシー: [監視とアラート](#monitoring-and-alerts) | `null`（なし） |

モジュールは VAPID 鍵も生成し、あなたにしか埋められないシークレットは空のまま作ります。

`examples/gcp-cloud-run-neon` は始め方の一例です。プロバイダーと、推奨の種類をすべて備えた 1 つのブロックで、Neon かどうかにかかわらず、外部のどのデータベースでも動きます。変えたいセクションを、そのブロックに足してください。

<!-- source: before-you-start 9869afda9a07 -->
## 始める前に

- [最初のデプロイ](deploy-opentofu.md#first-deployment)に必要なもの。プラットフォームの CLI は `gcloud` で、プロバイダーのために `gcloud auth application-default login` を実行します。
- 課金が有効で、このインスタンス専用の Google Cloud プロジェクトと、そのオーナー（Owner）ロールを持つアカウント。`stream = pubsub` のカスタムロールはプロジェクトに付くため、同じプロジェクトにあるほかのもののサブスクリプションにも及びます。

<!-- source: first-deployment 894b6af2cee9 -->
## 最初のデプロイ

最初の 2 つの手順はここで行い、そのあと 3 つ目の補足を見ながら[最初のデプロイ](deploy-opentofu.md#first-deployment)に従います。

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. API を有効にする

どのデプロイにも、次のものが必要です。

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

API が必要になる選択肢ごとに、その API を足します。

| 選択 | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` または `existing` | `compute.googleapis.com`、`servicenetworking.googleapis.com`、`networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`、`serviceconsumermanagement.googleapis.com`、およびネットワークのもの |

`translation` は自分の API を自分で有効にし、`observability` も同じです。`billing` に必要なものは[クラウド利用料の取り込み](billing-import.md)にあります。

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. データベースを用意する

- **`external`**: データベースを作り、その直接接続の URL と、あれば接続プールの URL を、`secrets_to_fill` のために控えておきます。どちらをどこに入れるかは[データベース](#the-database)にあります。モジュールはデータベースを作らないので、そのパスワードが OpenTofu のステートに入ることはありません。
- **`cloud-sql`**: 何もしません。モジュールがインスタンスを作り、生成したパスワードを付けた URL をシークレットに書き込みます。

<!-- source: 3-deploy 4470affd40e4 -->
### 3. デプロイする

[最初のデプロイ](deploy-opentofu.md#first-deployment)に従います。Google Cloud では次のとおりです。

- **変数**: 例の `mail_from`、`mail_smtp_host`、`mail_smtp_username` で[メール](email.md)を設定します。Google Cloud は外向きのポート 25 を遮断するので、ポートは 587 です。
- **シークレット**: KEK は `my-kek`、VAPID 鍵は `my-vapid-key` で、どちらも `name` で始まります。`gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` でコピーできます。KEK は `MY_KEK_FILE` で指すマウントされたファイルとして、ほかのシークレットは環境変数としてサーバーに届きます。
- **適用**: 最初にマイグレーションのジョブが、依存するものとともに作られます。サーバーの設定がイングレスを名指しするので、そのイングレスも含まれます。サービスの起動プローブは `/health/ready` を待ち、スキーマができるまでそれは `503` を返します。
- **DNS**: `tofu output dns_records` のレコードを、[トラフィックがサービスに届く経路](#how-traffic-reaches-the-service)が種類ごとに説明するとおりに追加します。証明書が有効になるまで、`https://{host}` は TLS エラーで失敗するため、連合もブラウザーもまだサーバーに届きません。ほかに待つものはありません。Cloud Tasks と Cloud Scheduler は、サービスの既定の URL `https://<name>-<project number>.<region>.run.app` を呼びます。**既定の URL を無効にしないでください**。ジョブが止まります。Cloud Run は、この 2 つの呼び出し元を、同じプロジェクトからその URL で来たときだけ内部のものとして扱います。だからこそ、ロードバランサーがその URL をインターネットから閉じられます。
- **最初の管理者**: リンクは Cloud Logging に残り、プロジェクトのログを読める人なら誰でも使えます。

<!-- source: the-console-and-upgrades a40a1df56865 -->
## コンソールとアップグレード

[コンソール](deploy-opentofu.md#the-console)と[アップグレード](deploy-opentofu.md#upgrading)は、適用する場所から `gcloud` を通して、Cloud Run のジョブ `<name>-console` と `<name>-migrate` の実行として動きます。ジョブを実行してそのログを読める認証情報、つまり `roles/run.developer` と `roles/logging.viewer` が必要で、Owner にはどちらも含まれます。適用が中断されると、実行は Cloud Run 上で動き続け、次の適用はそれが終わるのを待ちます。

[先に SQL を確認する](deploy-opentofu.md#checking-the-sql-first)には、次のようにします。

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## データベース

モジュールが勧めるのは `external` です。ネットワークが要らず、Google Cloud では何も課金されません。Neon のように、アイドルのあいだ止まるデータベースなら、静かなサーバーを安く保てます。`cloud-sql` は、固定費で、すべてを 1 つのプロジェクトにまとめます。両者のあいだでデータを移すのは、[バックエンドの変更](deploy-opentofu.md#changing-a-backend)です。

<!-- source: an-external-postgresql d6604bfb9e2f -->
### 外部の PostgreSQL

既定の `database = { kind = "external" }` は、Cloud Run から届く任意の PostgreSQL を使います。Neon、ほかのホスト型 PostgreSQL、自分で動かすもののどれでも構わず、インターネット経由でも、[ネットワーク](#networks)を通したプライベートアドレスでも使えます。モジュールは、**直接**接続の URL のためにシークレット `my-database-url` を作ります。マイグレーションのジョブとコンソールは常にこれを使います。`my schema apply` はセッションレベルのロックを持ち、トランザクションモードの接続プールはそれを保てないからです。

`database = { kind = "external", pooled = true }` は、**接続プール**の URL のためにシークレット `my-database-url-pooled` を足し、サービスはこちらを使うようになります。接続プールはサーバーに向いています。理由は[接続プールを通す](database.md#through-a-connection-pooler)にあります。接続プールがないときは、サービスも直接接続の URL を使うので、`service.max_instances` とインスタンスごとのプールの積が、データベースの接続数の上限を下回る必要があります。プールは、インスタンスが同時に受ける 80 リクエストに対し、既定ではインスタンスあたり 4 接続です。増やすには、サービスが使う URL に `pool_max_conns` を付けます。たとえば `...?sslmode=require&pool_max_conns=8` です（[データベース接続を待つ](running.md#waiting-for-a-database-connection)）。

**Neon の場合**は、Cloud Run のリージョンに近いリージョンにプロジェクトを作ります。直接接続の接続文字列はそのままのホスト名で、プールのものは `-pooler` の付いたホスト名です。これは Neon の接続プールで、サーバーに向いているので、`pooled = true` にします。既定の tick がコンピュートを起こし続けるので、Launch プランを見込んでください。Free プランでも動きますが、[費用の下にあるトレードオフ](#where-care-reduces-it)があります。

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` は、PostgreSQL 18 のインスタンス（既定では `db-f1-micro`）を作ります。毎日のバックアップと、7 日間保持されるポイントインタイムリカバリを備え、そのデータベースとユーザー、そして計画にもステートにも現れない、生成されたパスワードが付きます。`tier`、`disk_size_gb`、`retained_backups`、`deletion_protection` で変えられます。まだ一度も適用されたことがないので、最初のデプロイをそのテストとして扱ってください。

- **ネットワークなし**では、Cloud Run は Cloud SQL コネクタのソケットを通して届き、インスタンスの公開アドレスはほかのものを受け付けません。
- **ネットワークあり**では、プライベートアドレスだけを持ち、`pg_dump` はネットワークの内側から実行する必要があります。
- **接続プールはありません**。`pooled` は拒否され、接続数は `service.max_instances` と `pool_max_conns` で抑えます。メモリが 0.6 GB の `db-f1-micro` では、プールを小さく保ってください。
- **ゼロまでスケールしない**ので、トラフィックにかかわらず課金され、Google の SLA は共有コアの tier を対象にしません。

<!-- source: the-other-backends 91ef8e2dea11 -->
## ほかのバックエンド

どれも推奨を既定値にしていて、[`kind` を変える](deploy-opentofu.md#changing-a-backend)と別のものへ移ります。

- **ジョブ**: `cloud-tasks` は、ゼロまでスケールできるサービスへ、ジョブを 1 つずつ送り込みます。`queue.tick_schedule`（既定では毎分）が、定期ジョブのためにサーバーとデータベースを起こします。`db` は、動き続けるプロセスからデータベースをポーリングするので、`service.min_instances` を 1 以上にし、`service.cpu_always_allocated = true` にする必要があります。
- **メディア**: `gcs` は、リージョンに非公開のバケット `media.bucket_name` を作ります。`r2` は `cloudflare.account_id` に R2 のバケットを作り、`s3` は `endpoint` にある S3 互換のストアならどれでも使います。どちらも、`secrets_to_fill` が一覧にするシークレットからアクセスキーを読みます。バケットをほかのインスタンスと共有するには、`existing_bucket = true` であなたのバケットを使い、`prefix`（`tenant-a/` など）がすべてのオブジェクト名の先頭になります。`gcs` ではサービスアカウントがその下のオブジェクトにしか届かなくなり、それにはバケットレベルの均一なアクセス制御が必要です。
- **キャッシュとレート制限**: `db` はデータベースに保ちます。`memorystore` は Memorystore for Valkey を作ります。ネットワークが必要で、パスワードはなく、ネットワーク上のものなら誰でも届きます。`external` は、あなたが動かす Valkey の URL をシークレットから読みます。
- **リアルタイム更新**: `pubsub` はすべてのインスタンスに届きます。`postgres` はデータベースの直接接続で `LISTEN` と `NOTIFY` を使い、`valkey` は `kv` の Valkey を使います。
- **KEK**: `secret-manager` は 1 度だけ生成し、あなたがコピーします。`kms` は Cloud KMS に保ち、鍵はそこから出ないので、データベースのダンプと設定が両方漏れても、シークレットは明らかになりません。データのあるインスタンスでこれを変えるのは[移行](deploy-opentofu.md#the-kek)です。

<!-- source: networks 9d28311e11a6 -->
## ネットワーク

推奨の構成には要りません。外部のデータベース、Cloud Tasks、Cloud Storage、Pub/Sub は、どれもサービス自身の ID でインターネット越しに届くので、Cloud Run は何にも接続しません。

必要になるのは、プライベートアドレスしか持たない Memorystore を使うとき、Cloud SQL にプライベートアドレスを与えるとき、あなたのデータベースにプライベートアドレスで届くときです。`network = { kind = "vpc" }` は VPC、そのサブネット、プライベートサービスアクセスを作り、`network = { kind = "existing", ... }` は既存の VPC に接続します。その VPC は `network`、`subnetwork`、`private_services_range`、`memorystore_connection_policy` で指定します。

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

サービスとジョブは Direct VPC egress で接続し、ネットワークを通るのはプライベートの範囲だけなので、インターネットは自分の経路のまま、Cloud NAT は使いません。`vpc` で `nat = true` にすると、固定の送信元アドレスのために、すべてが Cloud NAT を通ります。ネットワークがあるとインスタンスの起動が 1 分以上遅れることがあり、ゼロまでスケールするサーバーでは頻繁に起きます。モジュールが作る VPC は、まだ一度も適用されたことがありません。

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## トラフィックがサービスに届く経路

これは `ingress.kind` で決まります。モジュールは本番向けにロードバランサーを勧めます。ドメインマッピングは、デプロイを安く試すのに向いています。

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| 月額 | 転送ルールで約 $18 に、GB あたりの料金が加わります。[料金のページ](https://cloud.google.com/vpc/network-pricing)で確認してください。この金額はそこでは確認できませんでした | なし |
| 位置づけ | Google Cloud が推奨する方法 | レイテンシーのため、Google Cloud が本番には勧めていない Preview の機能 |
| リージョン | すべて | `asia-northeast1`、`us-central1`、ほか数か所 |
| DNS | `host` からそのアドレスへの `A` レコード 1 つ。DNS プロバイダーのプロキシはオフにしてください | `dns_records` が一覧にするレコード |
| 証明書 | Google が管理。DNS がそのアドレスを指すと有効になり、時間がかかることがあります | Google が管理。最大 24 時間 |
| ドメインの所有権 | DNS のほかには何も不要 | 適用する人が、先にそのドメインの[確認済みの所有者](https://cloud.google.com/run/docs/mapping-custom-domains)になっている必要があります |
| `run.app` URL | インターネットからは拒否されるので、ロードバランサーを迂回できません | 誰でも届きます。ドメインマッピングはヘッダーを足せないので、オリジンは開いたままです |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` とロードバランサーのアドレス | `169.254.0.0/16` |

Cloud Tasks と Cloud Scheduler は、どちらも経由せず、既定の `run.app` URL を直接呼びます。

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` は、すべてのキャッシュを回避して `host` の前に Cloudflare を置きます。`ingress.media_host` は、どの種類とも併用できる、キャッシュされるメディアホストを追加します。どちらも `cloudflare = { zone_id = "..." }` が必要で、プロバイダーはトークンを `CLOUDFLARE_API_TOKEN` から読み、モジュールがレコードを作るので、`dns_records` は空です。どちらもモックしたプロバイダーでしか動かしていません。

- **オリジンはプランで決まります。** Cloud Run は `Host` でルーティングしますが、Cloudflare が Host ヘッダーを上書きできるのは Enterprise プランだけです。ほかのプランでは、モジュールが各ホストのドメインマッピングを作り、Cloudflare はそれに接続します。上の表のトレードオフがあります。その証明書がプロキシの後ろで発行され、更新されるかは確認できていないので、`cloudflare.proxied = false` で始めてください。Enterprise プランでは `cloudflare.host_header_override = true` にすると、ドメインマッピングなしで `run.app` URL に接続します。
- **ヘッダーがオリジンを閉じます。** リクエストヘッダーを変換するルールが、モジュールの生成した値を足し、サーバーは、ヘルスチェックと、トークンを確認するジョブ自身の `POST` を除いて、それのないリクエストを `403` で拒否します。そのため `run.app` URL を通って Cloudflare を迂回することはできず、レート制限に見えるクライアントのアドレスを選ぶこともできません。
- **レコードがプロキシされていないあいだは、何もヘッダーを足しません**。そのためサーバーはヘルスチェック以外をすべて拒否します。ドメインマッピングの証明書のために `proxied = false` にしたときは、証明書が有効になってからプロキシをオンにしてください。
- **値はステートに入り**（[シークレット](deploy-opentofu.md#secrets)にあるとおりです）、シークレット `my-origin-secret` にも入ります。ローテーションの方法は [CDN のガイド](cdn.md#rotate-the-value)にあります。
- **Bot Fight Mode はオフにし**、ダッシュボードの「I'm Under Attack」はオンにしないでください。`/ap/`、`/.well-known/`、`/api/` へのチャレンジは、ほかのサーバーがブラウザーではないため、連合を壊します。どちらもゾーン全体に及び、どのルールでも除外できません。モジュールはチャレンジを追加せず、Browser Integrity Check をオフにします。
- **Email Address Obfuscation はオンのままで構いません。** サーバー自身のページは、アドレスに手を加えてはならない内容に印を付けています。
- **Always Use HTTPS** は、`http://` をリダイレクトしたいならダッシュボードでオンにしてください。ゾーン全体に及ぶので、モジュールはあなたに任せています。
- **タイムアウト**: オリジンが 125 秒間何も送らないと、Cloudflare は `524` を返します。ジョブは Cloudflare を通らないので影響を受けません。ストリームは 30 秒ごとにハートビートを送り、900 秒のアイドル上限より短くなっています。プロキシされたホストで Cloudflare がストリームをバッファするかどうかは確認できていません。
- **動画**: Cloudflare の規約は、Free、Pro、Business で動画や大きなファイルを不釣り合いに多く配信することを制限しており、コンテンツに対して措置を取ることがあります。メディアホストで動画をキャッシュする前に、これを判断してください。

**削除したファイルはキャッシュに残ります。** メディアホストはサーバーの `Cache-Control` に従うので、削除したローカルのファイルは最大 1 年、リモートのファイルは `remote_media_cache_ttl_seconds` のあいだ見えたままになることがあります。サーバーはこれをパージしません。何かを消す必要があるときは、ダッシュボードの **Caching > Configuration > Custom Purge** か、次のコマンドで手作業でパージしてください。

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

プロキシの URL はクエリ文字列に `sig` を持ち、それもキャッシュされるものの一部なので、URL 全体を指定してください。

<!-- source: monitoring-and-alerts eb28e653de36 -->
## 監視とアラート

`monitoring` は、`/health/live` の死活監視のチェック、ログベースのメトリクス、そして[運用手順書](alerts.md)と同じ名前のアラートポリシーを追加します。通知先は `notification_emails`（例では `alert_emails`）のアドレスか、`notification_channels` のチャネルです。このセクションに既定値はありません。誰に通知するかを指定するか、`monitoring = null` にして全体を外してください。まだ一度も適用されたことがないので、チェックをわざと失敗させて 1 度試してください。チェックは実行のたびにインスタンスを起こします。その間隔が `uptime_period` で、既定では 15 分です。

<!-- source: traces b1f84a2f93b9 -->
### トレース

Cloud Trace は、Cloud Run がリクエストごとに記録するトレースを表示し、モジュールがイングレスを信頼する（`MY_TRUSTED_PROXIES`）ので、サーバーのログ行は `trace_id` を持ちます。モジュールは `logs_exporter = gcp` を設定するので、各ログ行は Cloud Logging が Cloud Trace にリンクするトレースのフィールド（`logging.googleapis.com/trace`）も埋め、アクセスログの行はリクエストとして表示されます。`observability.logs = "settings"` にすると、`logs_exporter` は `settings` に任されます。確認できていないこと: Cloud Logging が、ログ行に書かれた trace ID だけでその行をリンクすること、および公開リクエストの `traceparent` を Cloud Run が必ず置き換えること。サーバー自身のスパンには、その隣で OpenTelemetry Collector を動かす必要があり、モジュールはそれを動かしません。

<!-- source: limits 3d4721415f18 -->
## 制限

- **アップロード**: Cloud Run は 32 MiB を超える HTTP/1 のリクエストを拒否し、サーバーは HTTP/1.1 で話します。そのためモジュールは `MY_MEDIA_MAX_AV_BYTES` を、既定の 100 MiB ではなく 30 MiB にして、それより大きなアップロードにはサーバー自身が応答するようにしています。Cloudflare 自身の上限（Free と Pro で 100 MB）はそれより上です。
- **ストリーム**: `service.request_timeout_seconds`（既定で 15 分）で開いたストリームが終わり、クライアントは再接続します。
- **データベース接続**: Google は、Cloud Run のインスタンスから Cloud SQL への接続を 100 までに制限しています。
- **クライアントのアドレス**: レート制限は、モジュールがイングレスから設定する `MY_TRUSTED_PROXIES` に頼っています。直接のリクエストに Cloud Run が `X-Forwarded-For` に何を足すのか、コンテナがどのピアアドレスを見るのかを書いた、私たちの読んだ文書はないので、`169.254.0.0/16` は推測です。ロードバランサーの文書化された形式は `[<supplied>,]<client-ip>,<lb-ip>` です。最初のリクエストのログでアドレスを確認してください。クライアントのアドレスが `169.254` のものや Google のものなら、その設定はあなたのプロジェクトでは間違っています。
- **公開アクセス**: サービスは `allUsers` に `roles/run.invoker` を与えますが、ドメイン制限共有の組織ポリシーはこれを拒否します。そのようなプロジェクトには例外が必要です。
- **使えないままになる名前**: Cloud SQL と Cloud Tasks は、削除したインスタンスやキューの名前を約 1 週間拒否し、削除したカスタムロールも ID をしばらく保ちます。そのため `destroy` のあとにデプロイし直すときは、別の `name` が必要です。

<!-- source: costs ea658d24d79d -->
## 費用

ここの数字は、何を見込むか、どこで工夫すれば減らせるかの目安であり、デプロイが守る上限ではありません。2026-10 と 2026-11 時点の、1 つのリージョンのおおよその定価です。[Cloud Run](https://cloud.google.com/run/pricing)、[Cloud SQL](https://cloud.google.com/sql/pricing)、[Neon](https://neon.com/pricing) の料金のページで確認してください。Cloud Run のコンピュートは、請求アカウントごとに月 180,000 vCPU 秒と 360,000 GiB 秒まで無料です。

どのデプロイでも支払うもの。アイドル時は約 $0 です。

| 部分 | アイドル時 | 増える要因 |
| --- | --- | --- |
| Cloud Run | $0。インスタンスは動かず、CPU はリクエスト中だけ課金されます | 開いたストリームごとにインスタンスが課金されます。下を参照 |
| Cloud Tasks | $0 | 100 万オペレーションまで無料、以降は 100 万あたり $0.40。ジョブ 1 つで約 2 オペレーション（キューへの登録と配送） |
| Cloud Scheduler | $0。tick は 1 ジョブで、請求アカウントごとに 3 ジョブまで無料 | それを超えるとジョブあたり $0.10 |
| Pub/Sub | $0。10 GiB まで無料 | ストリームが開いているあいだだけ |
| Secret Manager | 数セント | 数バージョンまで無料、以降はバージョンあたり数セント |
| Cloud Storage | 月に 1 GiB あたり約 $0.02 | 5 GiB が無料なのは `us-east1`、`us-west1`、`us-central1` だけ。メディアは Cloud Run を通して出ます |
| Monitoring | 無料枠の範囲内なら $0 | |

選択肢ごとに加わるもの。

| セクション | 選択 | 月額 |
| --- | --- | --- |
| `ingress` | `load-balancer` | 約 $18。料金のページでは確認できていません。GB あたりの料金が加わります |
| | `domain-mapping` | $0 |
| | `cloudflare` | あなたの Cloudflare のプランと、オリジン。ドメインマッピングなら $0 |
| `database` | Neon の Launch プランの `external` | 既定の tick で約 $19、ストレージは GB・月あたり $0.35 |
| | Neon の Free プランの `external` | 制限の範囲内なら $0: [下](#where-care-reduces-it)を参照 |
| | それ以外の `external` | そのデータベースの費用 |
| | `cloud-sql` | `us-central1` の `db-f1-micro` と 10 GiB で約 $9.40、それにバックアップ。トラフィックにかかわらずかかります |
| `queue` | `db` | 24 時間課金されるインスタンスで、CPU は常に割り当てられます |
| `kv` | `memorystore` | 最小のノードで約 $23。トラフィックにかかわらずかかります |
| `kek` | `kms` | 数セント。鍵とその操作の分です |
| `network` | `nat = true` の `vpc` | Cloud NAT。時間あたりと GB あたり |

Neon の Launch プランでの推奨の構成、つまりロードバランサーと Neon は月に約 $37 で、Cloud SQL なら約 $27 です。

**tick が Neon を起こし続けます。** tick は毎回データベースを読み書きし、Neon がコンピュートを止めるのは 5 分間アクティビティがないときだけなので、既定の `* * * * *` ではコンピュートが止まりません。月に 730 時間、0.25 CU で 182.5 CU 時間になり、これが上の $19 です。定期ジョブが時間どおりに動き、失われた起こす合図がすぐ回復されるよう、tick は毎分のままです。Cloud SQL は tick にかかわらず同じ額です。

**開いたストリームはインスタンスを課金させ続けます。** リクエストベースの課金では、インスタンスは何かのリクエストを処理しているあいだ課金され、リアルタイム更新は 1 本の長いリクエストです。ストリームを 1 か月開いたままにするクライアントが 1 つあると、インスタンス 1 つが 24 時間分かかります。無料枠のあとで、1 vCPU と 512 MiB なら月に約 $60 です。ストリームが増えると、`service.concurrency` までは同じインスタンスを共有します。ストリームがほとんどずっと開いているなら、インスタンスベースの課金（`service.cpu_always_allocated`）や、小さな VM と [Docker Compose](deploy-docker-compose.md) と比べてください。

<!-- source: where-care-reduces-it 427be9d46d32 -->
### 工夫で減らせる部分

- **ドメインマッピング**をロードバランサーの代わりに使うと $18 がなくなりますが、[トレードオフ](#how-traffic-reaches-the-service)があります。デプロイを試すのに向いています。
- **Neon の Free プラン**で $19 がなくなり、小さなインスタンスならそれで動かせますが、次のトレードオフがあります。
  - 月に 100 CU 時間までで、既定の tick なら約 17 日で使い切ります。`queue = { tick_schedule = "*/10 * * * *" }` のように、間隔の長いスケジュールにすれば収まります。ほかが起こす前の時点で、起きている時間は最大でも半分、約 91 CU 時間です。そのぶん定期ジョブは最大 10 分遅れて動き、失われた起こす合図の回復も遅くなります。
  - 連合するサーバーは配信を 1 日中受け取り、それがデータベースも起こすので、忙しいインスタンスはどんなスケジュールでも枠を超えます。
  - 1 プロジェクトあたり 1 GB で、復元できる履歴は 6 時間です。コンピュートは 5 分アイドルになると必ず止まるので、そのあとの最初のリクエストはその起動を待ちます。
