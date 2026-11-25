<!-- source: deploying-on-aws d7e8643cd93d -->
# AWS へのデプロイ

OpenTofu のモジュール [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) で、AWS 上に `my` を動かす方法を説明します。Application Load Balancer の後ろの ECS on Fargate、データベースの中のジョブ、S3、SNS、Amazon SES を通したメールで構成します。ゼロまでスケールするものはありません。データベースを除いて、アイドル時でも月に約 $42 を見込み、[費用](#costs)を読んでください。

**これは AWS にまだ一度も適用されたことがありません。** チェックは、モックしたプロバイダーと認証情報なしで計画を立てるだけなので、設定に矛盾がないことは分かっても、AWS がそれを受け付けることや、サーバーが起動することは分かりません。最初のデプロイはテストとして扱ってください。

OpenTofu のデプロイに共通するもの、つまりモジュールの入力、シークレット、コンソール、アップグレードは、[OpenTofu でのデプロイ](deploy-opentofu.md)にあります。このページは、AWS に必要なことを補います。

<!-- source: what-it-creates 1ca642b05ab0 -->
## 作られるもの

`mycroblog-aws` の 1 つのブロックで、これらすべてが作られます。各行はモジュールのセクションで、既定値が推奨の構成です。すべてのフィールドは[モジュールのリファレンス](../../deploy/opentofu/README.md#sections-of-mycroblog-aws)にあります。

| 部分 | セクション | 既定 | ほかの種類 |
| --- | --- | --- | --- |
| サーバー | `service`、`jobs` | `role = all` の Fargate 上の ECS クラスターとサービス、マイグレーションと（必要なあいだだけ）コンソールのタスク定義、ロググループ `/ecs/<name>` | |
| ネットワーク | `network` | 2 つ以上のゾーンにまたがる VPC。各ゾーンにパブリックとプライベートのサブネットがあります | あなたの VPC: [ネットワーク](#networks) |
| トラフィック | `ingress` | あなたの証明書を持つ Application Load Balancer。常にあります | その前に置く CloudFront か Cloudflare: [トラフィックがサービスに届く経路](#how-traffic-reaches-the-service) |
| ジョブ | | なし。タスクは動き続け、データベースからジョブを受け取ります | |
| データベース | `database` | Neon など、自分で動かす PostgreSQL の URL を入れるシークレット | Amazon RDS: [データベースを選ぶ](#2-choose-the-database) |
| メディア | `media` | 非公開の S3 バケット | Cloudflare R2、または S3 互換の任意のストア |
| キャッシュとレート制限 | `kv` | なし。データベースが保ちます | ElastiCache Serverless for Valkey、または自分で動かす Valkey |
| リアルタイム更新 | `stream` | SNS トピック。サーバーが自分の SQS キューを作って消します | データベース、または `kv` の Valkey |
| メール | `mail` | `mail.from` のドメインの SES ID: [Amazon SES を通したメール](#email-through-amazon-ses) | 任意の SMTP サービス |
| シークレット | | Secrets Manager: 生成された VAPID 鍵と、あなたにしか埋められない、空のシークレット | |
| KEK | `kek` | Secrets Manager の、生成されたシークレット | AWS KMS の鍵。鍵はそこから出ません |
| メトリクス、トレース、エラー | `observability` | CloudWatch のメトリクスとエラー報告、すべてのタスクにあるコレクター経由の X-Ray へのトレース: [X-Ray へのトレース](#traces-to-x-ray) | `settings` を通した、あなた自身のエクスポーター |
| アラート | `monitoring` | CloudWatch Synthetics のカナリアとアラーム: [監視とアラート](#monitoring-and-alerts) | `null` でなし |

`translation = {}` は Amazon Translate を、`search = { domain_name = "..." }` はあなたが作った Amazon OpenSearch Service のドメインを、`billing = {}` は Cost Explorer からの利用料の取り込みを追加します。それぞれ[投稿を翻訳する](translation.md#amazon-translate)、[検索エンジン](search-engines.md)、[クラウド利用料の取り込み](billing-import.md)を参照してください。

<!-- source: before-you-start 9d5614a650e1 -->
## 始める前に

- [最初のデプロイ](deploy-opentofu.md#first-deployment)に必要なもの。プラットフォームの CLI は `aws` です。
- このインスタンス専用の AWS アカウントと、上のすべてを作れる認証情報（`AdministratorAccess` など）。
- リージョン。SES はそこから送信し、モジュールは `aws` プロバイダーからそれを受け取ります。
- 推奨: Route 53 にある `host` のパブリックホストゾーン。そうすると、モジュールが `host` のレコードをそこに作り、ゾーンの頂点（apex）でも作れます。

<!-- source: first-deployment 26b562a3db03 -->
## 最初のデプロイ

最初の 3 つの手順はここで行い、そのあと 4 つ目の補足を見ながら[最初のデプロイ](deploy-opentofu.md#first-deployment)に従います。

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. 証明書を要求する

ロードバランサーには、そのリージョンの AWS Certificate Manager にある、`host` の検証済みの証明書が必要です。モジュールは ARN としてそれを受け取ります。適用のなかで検証すると、適用が表示するレコードをあなたが追加するまで、適用が止まってしまうからです。

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

`ingress.kind = cloudfront` では、もう 1 つを `us-east-1` で要求します。CloudFront が証明書を受け取るのはそのリージョンだけです。`us-east-1` なら、1 つの証明書で両方をまかなえます。`ingress.media_host` を使うときは、証明書がそのホストも含む必要があります。

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. データベースを選ぶ

`database.kind` で選びます。どちらでも、タスクは約 1 秒に 1 回データベースにジョブをポーリングするので、データベースは休みません。

**`external`（既定で推奨）** は、[Neon](https://neon.com) のような、あなたが動かす PostgreSQL の URL を、あなたが埋めるシークレットから読みます。モジュールはデータベースを作らないので、そのパスワードが OpenTofu のステートに入ることはありません。Neon では、近いリージョンにプロジェクトを作り、その接続文字列をコピーします。

- **直接**接続のもの（`-pooler` のないホスト）は必須で、シークレット `<name>-database-url` に入れます。マイグレーションとコンソールは常にこれを使います。`my schema apply` はセッションレベルのロックを持ち、トランザクションモードの接続プールはそれを保てないからです。
- **プール**のもの（`-pooler` のあるホスト）は任意で、サービス用です。`database = { kind = "external", pooled = true }` にすると、モジュールがシークレット `<name>-database-url-pooled` を作ります。[接続プールを通す](database.md#through-a-connection-pooler)を参照してください。

Neon の Launch プランを見込んでください。コンピュートが止まらないので、Free プランのコンピュート時間は、その月のうちに尽きます。

**`rds`** は、プライベートサブネットに Amazon RDS for PostgreSQL を作ります。パスワードはモジュールが生成して `<name>-database-url` に自分で書き込むので、埋めるものはありません。

```hcl
  database = { kind = "rds" }
```

既定では 20 GB の `db.t4g.micro`（`instance_class`、`allocated_storage_gb`）で、自動バックアップを 7 日間保持し、削除保護が付きます。公開アドレスはなく、届くのはタスクだけなので、`pg_dump` は VPC の内側から実行する必要があり、アップグレードの前には、代わりにスナップショットを取ります。`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade` です。プールの URL はありません。

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. ブロックを書く

`examples/aws-ecs-neon` は始め方の一例です。`aws` プロバイダーと、既定値の 1 つのブロックで、その変数は `terraform.tfvars.example` に一覧があります。どこから始めても、ブロックには次のものが必要です。

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` に既定値はありません。外への 2 つの経路はどちらも時間で課金されるからです。[ネットワーク](#networks)を読んでください。
- `ingress.route53_zone_id` には、あれば `host` のホストゾーンを指定します。
- `media.bucket_name` は、AWS 全体で一意である必要があります。
- `monitoring` には、アラームの通知先を指定するか、`null` にします。

選んだデータベースのセクションと、変えたいほかのセクションを足してください。

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. デプロイする

[最初のデプロイ](deploy-opentofu.md#first-deployment)に従います。AWS では次のとおりです。このページのコマンドは、既定の `name` である `my` を使います。

- **シークレット**: Secrets Manager にあり、`<name>-...` という名前です。`tofu output kek` に KEK をコピーするコマンドがあり、`aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` で VAPID 鍵をコピーできます。Fargate はファイルをマウントしないので、KEK は `MY_KEK` としてサーバーに届きます。タスクはシークレットを起動時に読みます。シークレットを変えたら `aws ecs update-service --cluster my --service my --force-new-deployment` を実行し、各ワーカー（`my-<key>`）にも同じことをします。
- **適用**: 最初にマイグレーションのタスク定義が、依存するものとともに作られ、サービスはマイグレーションが成功してから作られます。ロードバランサーのチェックは `/readyz` を確認し、スキーマができるまでそれは `503` を返します。
- **DNS**: `tofu output dns_records` は、Route 53 や Cloudflare が作る場合を除いて `host` のレコードと、SES の 3 つの DKIM レコードを一覧にします。Route 53 を使わないゾーンの頂点では、`host` のレコードに、DNS プロバイダーが `ALIAS` や `ANAME` と呼ぶものが必要です。`tofu output dns_note` にあるとおりです。
- **メール**: AWS が本番アクセスを許可するまで、SES は確認済みのアドレスにしか送信しないので、ほかの人はまだサインアップできません。[Amazon SES を通したメール](#email-through-amazon-ses)を読んでください。
- **アラート**: `monitoring.notification_emails` の各アドレスには、SNS から購読のメールが届くので、それを承認する必要があります。
- **最初の管理者**: リンクは CloudWatch Logs の `/ecs/<name>` に残り、アカウントのログを読める人なら誰でも使えます。

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## AWS でのコンソールとアップグレード

[コンソール](deploy-opentofu.md#the-console)と[アップグレード](deploy-opentofu.md#upgrading)は、適用する場所から `aws` を通して、単発の Fargate タスクを実行し、その出力を CloudWatch Logs から読みます。適用が中断されるとタスクは動き続け、次の適用はそれが終わるのを待ちます。アップグレードでは、すべてのタスク定義がイメージを含む新しいリビジョンを得ますが、サービスがそれを取り込むまで何も実行されません。各サービスは、新しいタスクが起動するのを待ち、既定のサービスではさらにロードバランサーのチェックに通るのを待ってから、古いタスクを外します。タスクが起動しないリビジョンは、デプロイのサーキットブレーカーが前のものに戻します。

適用する場所の認証情報（または適用する CI のジョブのもの）には、適用自体に必要なものに加えて、次のものが必要です。

- マイグレーション: `ecs:RunTask`、`ecs:DescribeTasks`、`ecs:ListTasks`、`logs:FilterLogEvents`、タスクの 2 つのロールへの `iam:PassRole`。
- コンソールには、さらに `ecs:DescribeServices`、`ecs:DescribeTaskDefinition`、`ecs:ListTagsForResource`、`ecs:RegisterTaskDefinition`。コンソールは最初に、動いているサービスのイメージを取るからです。

[先に SQL を確認する](deploy-opentofu.md#checking-the-sql-first)には、ブロックがまだ現在のリリースを指しているうちにコンソールをオンにし、そのあと `ref` を上げて、マイグレーションのタスク定義だけを適用し、それで `my upgrade -dry-run` を実行します。

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## 監視とアラート

`monitoring` は、`/readyz` の CloudWatch Synthetics のカナリア、CloudWatch のメトリクス、そして[運用手順書](alerts.md#aws-ecs)と同じ名前のアラームを追加します。通知先は `notification_emails` のアドレスか、`notification_topic_arns` の SNS トピックです。この入力に既定値はありません。誰に通知するかを指定するか、`monitoring = null` にして全体を外してください。カナリアは実行のたびに課金され、その間隔が `uptime_period_minutes` で、既定では 15 です。[費用](#costs)を読んでください。

<!-- source: choosing-the-backends b83753b1f860 -->
## バックエンドを選ぶ

どのセクションも既定は推奨の種類です。別のものにするには、[セクションの `kind` を変えます](deploy-opentofu.md#changing-a-backend)。データベースは[上](#2-choose-the-database)にあります。

| セクション | 推奨 | ほかの種類と、その使いどころ |
| --- | --- | --- |
| `media` | `s3`。リージョンのバケットで、`versioning` はオフ | `cloudflare.account_id` の `r2`、または `endpoint` の `s3-compatible`。どちらも、あなたが埋めるシークレットにアクセスキーを入れます。バケットを共有するには、`existing_bucket = true` であなたのバケットを使い、`prefix`（`tenant-a/`）がすべてのオブジェクト名の先頭になります。`s3` ではタスクのロールがその下のオブジェクトにしか届かなくなります |
| `kv` | `db`。追加の費用はありません | `elasticache`。アイドル時も課金され、プライベートサブネットに置かれます。または `external`。あなたが埋めるシークレットに入れる Valkey の URL |
| `stream` | `sns`。すべてのタスクに届きます | `postgres`。データベースの直接接続を使います。または `valkey`。`kv` の Valkey |
| `mail` | `ses`。パスワードは不要 | `smtp`。`smtp.host`、`smtp.port`（587）、`smtp.username` を指定し、パスワードはあなたが埋めるシークレットに入れます |
| `kek` | `secrets-manager`。1 度だけコピーします | `kms`。AWS KMS から出ない鍵。データのあるインスタンスでは、[変更](deploy-opentofu.md#the-kek)は移行です |
| `observability` | CloudWatch と X-Ray | `traces = "none"`、または、Sentry など、ほかへ送るシグナルのための `"settings"` |

`architecture = "arm64"` にすると、タスクは Graviton で動き、費用が下がります。リリースのイメージは両方に対応しています。

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## Amazon SES を通したメール

既定の `mail.kind = "ses"` は、デプロイのリージョンの SES API で、`mail.from` のドメインから送信します。モジュールは、そのドメインの ID を Easy DKIM 付きで作り、タスクにその `ses:SendEmail` を許可するので、パスワードはありません。`mail.configuration_set` は構成セットを作ってすべてのメッセージに適用します。`existing_configuration_set = true` にすると、インスタンスで共有する構成セットを名指しでき、各インスタンスは、自分が送ったメールの通知にだけ動作します。SES があなたのユーザーに送信する前に、`tofu output dns_records` が一覧にするレコードを追加し、サンドボックスを抜けてください。手順と、サーバーがバウンスや苦情をどう知るかは、[Amazon SES でメールを送る](email-ses.md)にあります。

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## X-Ray へのトレース

`observability.traces = "x-ray"` は既定です。コレクターが、追加の費用なしで推奨の構成に収まるからです。これは AWS Distro for OpenTelemetry のコレクターで、すべてのタスクでサーバーの隣で動き、タスクのループバックだけで待ち受け、タスクのロール（`xray:PutTraceSegments`）で X-Ray に書き込みます。各タスクから 32 CPU ユニットと 128 MiB を使いますが、AWS はタスクのサイズだけで課金するので、256 CPU ユニットと 1,024 MiB の既定のサービスタスクは同じ費用のまま、サーバーには 224 CPU ユニットと 896 MiB が残ります。既定の `traces_sample_ratio` の 1% では、1 日 10 万リクエストで月に約 3 万トレースが記録され、X-Ray が無料で記録する月 10 万トレースの範囲に収まります。

実際に費用がかかるのは、タスクの起動のたびに取得するコレクターのイメージと、サーバーのメモリです。トレースを読まないとき、または 128 MiB を割けないほど小さなタスクでは、`traces = "none"` にしてください。サーバーに残るのが 128 CPU ユニットか 512 MiB を下回るサイズは、`service`、`jobs`、ワーカーのどれでも、計画が拒否します。コレクターは必須ではなく、止まってもサーバーはトレースなしで動き続けます。X-Ray がそのスパンを受け取るかどうかは確認できていません。

<!-- source: networks 36dc33a6cbef -->
## ネットワーク

ECS には常に VPC が必要です。推奨の `network.kind = "vpc"` は、`cidr_block`（`10.0.0.0/16`）に、`availability_zones`（ロードバランサーと RDS のために 2 つ以上）にまたがって VPC を作ります。各ゾーンにパブリックとプライベートのサブネットがあり、インターネットゲートウェイもあります。IPv4 だけなので、サーバーは IPv6 のみのピアには届きません。`network.kind = "existing"` は、代わりにあなたの VPC を使い、`vpc_id`、`vpc_cidr`、`public_subnet_ids`、`public_subnet_cidrs`、`private_subnet_ids` を指定します。

タスクには、イメージ、データベース、連合、AWS の API のために、インターネットへ出る経路が必要で、`network.egress` がそれを選びます。内向きのトラフィックは、どちらでもロードバランサーを通してしかタスクに届きません。

| | `public-ip`（推奨） | `nat` |
| --- | --- | --- |
| タスクのサブネット | パブリック。各タスクにパブリックな IPv4 アドレスが付きます | プライベート |
| 1 時間あたり | タスクのアドレスごとに $0.005 | NAT ゲートウェイ 1 つで約 $0.045、と、そのアドレス |
| 1 GB あたり | アドレスには何もかかりません | ゲートウェイ経由で約 $0.045。メディアと連合も含みます |
| `existing` のとき | 追加なし | あなたのプライベートサブネットが、すでにインターネットに届いている必要があります |

あなたの方針がパブリックサブネットを禁じていないかぎり、`public-ip` を使ってください。費用がはるかに安く、開き方も変わりません。`vpc` は NAT ゲートウェイを最初のゾーンに 1 つ作るので、そのゾーンが落ちると、復旧するまでプライベートサブネットはインターネットを失います。RDS と ElastiCache はどちらでもプライベートサブネットにあり、外へ出る経路は要りません。

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## トラフィックがサービスに届く経路

ロードバランサーは、`ingress.certificate_arn` の証明書付きで常にあり、タスクはそれだけを受け入れます。その前に何を置くかは `ingress.kind` で決まります。

| | `load-balancer`（推奨） | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` の向き先 | ロードバランサー | CloudFront ディストリビューション | Cloudflare。ロードバランサーに接続します |
| キャッシュ | なし | `/media/`。サーバーの `Cache-Control` に従います | `host` ではなし。`media_host` がキャッシュされるホストを追加します |
| レコード | Route 53 が `route53_zone_id` で作るか、`dns_records` が一覧にします | `route53_zone_id` の Route 53、または `dns_records` | Cloudflare が作ります |
| 必要なもの | | `us-east-1` の `cloudfront_certificate_arn`。`cloudfront_price_class` がエッジのロケーションを選びます | `cloudflare = { zone_id = "..." }` と、`CLOUDFLARE_API_TOKEN` のトークン |
| オリジンを閉じる方法 | タスクのセキュリティグループ | ロードバランサーのセキュリティグループは CloudFront だけを受け入れ、リスナーは CloudFront が送るヘッダーのないリクエストを拒否します | Cloudflare のルールが足すヘッダー。サーバーが確認します |

CloudFront や Cloudflare の後ろでは、ヘッダーの値が OpenTofu のステートに入るので、ステートは非公開に保ってください。CloudFront より前からあるロードバランサーのレコードは、先に消す必要があります。同じ名前のレコードが残っていると、適用が失敗します。CloudFront のアドレスは、計画のときに AWS から取得するので、ときどき適用し直してください。そうしないと、新しいアドレスのエッジが、そのビューアーのレート制限を共有してしまいます。

`ingress.media_host` は、`load-balancer` や `cloudflare` と併用できる、Cloudflare 上のキャッシュされるメディアホストを、`cloudflare.zone_id` とともに追加します。削除したファイルは、パージするまでキャッシュに残ります。[CDN を通してメディアを配信する](cdn.md)を読んでください。Cloudflare を何かの前に置くときは、ダッシュボードで Bot Fight Mode をオフにしてください。`/ap/`、`/.well-known/`、`/api/` へのチャレンジは連合を壊し、モジュールはそれを管理しません。Cloudflare の種類は、モックしたプロバイダーでしか動かしていません。

<!-- source: limits 42ed50837851 -->
## 制限

- **ヘルス**: ロードバランサーの唯一のチェックは `/readyz` を確認し、データベースが落ちているあいだはこれも失敗します。失敗が 2.5 分続くと ECS がタスクを置き換え、データベースが戻るまでそれを繰り返します。コンテナのチェックはないので、ロードバランサーのないワーカーは、不良なイメージでの失敗がログにしか現れません。
- **アップロード**: ロードバランサーはリクエストのボディに上限を設けないので、サーバーの `media_max_*` の設定が決めます。
- **ストリーム**: ストリームは 30 秒ごとにハートビートを送り、ロードバランサーのアイドルタイムアウトの 60 秒より短くなっています。タスクが去るまで開いたままで、そのあとクライアントが再接続します。
- **クライアントのアドレス**: レート制限は、サーバーが信頼するものに頼っていて、それはモジュールがイングレスから設定します。最初のリクエストのログでクライアントのアドレスを確認してください。ロードバランサーのサブネットや CloudFront のアドレスなら、設定が間違っています。
- **ディスク**はタスクとともに消えるので、計画は SQLite と、ファイルシステム上のメディアを拒否します。
- カスタマーマネージドの KMS キーで暗号化した**シークレット**には `kms:Decrypt` も必要で、モジュールはこれを許可しません。

<!-- source: costs 19e0e43bc201 -->
## 費用

ECS にはゼロまでスケールするものがありません。タスク、ロードバランサー、そのパブリックアドレスは、1 か月ずっと課金されます。ここの数字は、行に断りがなければ、2026-10 と 2026-11 時点の AWS の東京の価格表によるもので、請求書とは照らし合わせていません。

| 部分 | 月額 | 選択 |
| --- | --- | --- |
| 0.25 vCPU と 1 GB のタスク | x86 で $13.26、arm64 で $10.61 | `service`、`architecture`。各ワーカーは、それぞれのタスクを追加します |
| ロードバランサー | $17.74 に、1 か月使い続けた容量ユニットごとに $0 から $5.84 | 常にあります |
| ロードバランサーのパブリックアドレス | 2 ゾーンで $7.30 | ゾーンごとに 1 つ |
| 外への経路 | `public-ip` ではタスクごとに $3.65。`nat` ではオハイオの料金で約 $36.50 と GB あたり $0.045 | `network.egress` |
| データベース | Neon の Launch プランで 0.25 CU なら約 $19（コンピュートが止まらないため）。`rds`、`db.t4g.micro`、20 GB なら約 $21 | `database` |
| キャッシュとレート制限 | `db` なら $0。`elasticache` では約 $6.13、AWS の別の文書によれば北バージニアで約 $61 | `kv` |
| カナリア | 15 分ごとで約 $3.34、毎分なら約 $51.72 | `monitoring.uptime_period_minutes` |
| トレース | 月 10 万トレースまでは $0 | `observability.traces` |

**アイドル時は、`public-ip` で約 $42、`nat` で約 $75** で、これに容量ユニットが加わります。データベース、ストレージ、トラフィック、カナリアは含みません。確認していないので、どの行にも入っていないものは、CloudFront、Secrets Manager、CloudWatch Logs、S3、SNS と SQS、AWS の外へ出るデータ、無料枠です。単発のタスクは、実行した分のあいだ、そのサイズの費用がかかり、アップグレードは数分のあいだ、2 つ目のサービスのタスクを動かします。
