<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# Cloud Run へのデプロイ

OpenTofu のモジュール [`mycroblog-gcp`](../../deploy/opentofu/README.md) で、Google Cloud 上に `my` を動かす方法を説明します。使うのは Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage、Pub/Sub で、PostgreSQL のデータベースは Neon に置き、グローバル外部アプリケーションロードバランサーの後ろに構成します。誰も使っていないあいだはインスタンスが 1 つも動きません。ただし、サーバーを毎分起こす tick だけは動きます。サーバーには費用がかかります。ストレージ、トラフィック、開いたままのストリームを除いて月に約 $37 を見込んでください。工夫で減らせる部分は[費用](#costs)に書いてあります。

モジュールには OpenTofu 1.11 以降が必要です。Terraform には対応していません。

<!-- source: what-it-creates cae1e55a1722 -->
## 作られるもの

`mycroblog-gcp` の 1 つのブロックで、これらすべてが作られます。表の各行はモジュールのセクションで、既定値がこのガイドでデプロイするものです。ほかの種類は[モジュールの README](../../deploy/opentofu/README.md#sections)にあります。

| 部分 | セクション | リソース |
| --- | --- | --- |
| サーバー | `service`、`jobs` | `role = all` の Cloud Run サービス、マイグレーションのジョブ、必要なあいだだけのコンソールのジョブ、サービスアカウント、公開アクセス |
| トラフィック | `ingress` | 証明書付きのグローバル外部アプリケーションロードバランサー。ほかの種類はドメインマッピングと Cloudflare で、Cloudflare のメディアホストはどの種類とも併用できます |
| ジョブ | `queue` | Cloud Tasks のキュー、Cloud Scheduler のジョブ、サーバーを呼び出すサービスアカウント |
| メディア | `media` | 非公開のバケット |
| リアルタイム更新 | `stream` | トピックとカスタムロール。サブスクリプションはサーバー自身が作成し、削除します |
| データベース | `database` | その URL のシークレット。データベースは手作業で作ります |
| メール | `mail` | SMTP パスワードのシークレット。SMTP サービスならどれでも使えます |
| シークレット | | 生成される VAPID 鍵と、あなたにしか入れられない値のための空のシークレット |
| KEK | `kek` | ファイルとしてマウントされる、生成されたシークレット。もう一つの種類は Cloud KMS です |
| メトリクス、トレース、エラー | `observability` | Cloud Run が送る情報を読む、Cloud Monitoring、Cloud Trace、Error Reporting の API |

シークレットの値が OpenTofu のプランやステートに現れることはありません。ほかから持ってくる値はあなたが自分で追加し、KEK と VAPID 鍵はモジュールが書き込み専用の属性で生成します。

<!-- source: before-you-start 10846ce6f3b1 -->
## 始める前に

- 適用を行う場所に `tofu` 1.11 以降、`gcloud`、`jq`。バージョンが変わる適用では、その場所から `gcloud` を通してマイグレーションが実行されます。
- 課金を有効にした、このインスタンス専用の Google Cloud プロジェクトと、そのプロジェクトのオーナーのロールを持つアカウント。
- [`host`](../reference/configuration.md#host) に使うドメイン。あとから変えられません。[あとから変えられない設定](production-checklist.md#settings-you-cannot-change-later)を読んでください。
- [リリース](running.md#get-the-image)の `my` のバージョン。`latest` タグはありません。

<!-- source: first-deployment 996fcc6eb35e -->
## 最初のデプロイ

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. API を有効にする

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` はロードバランサー用です。

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. Neon にデータベースを作る

Launch プランを見込んでください。既定の tick では、コンピュートが止まることがありません。Free プランでも動きますが、[費用](#costs)に書いたトレードオフがあります。

Neon のコンソールで、Cloud Run のリージョンに近いリージョンにプロジェクトを作り、その接続文字列をコピーします。モジュールはこれを作らないので、パスワードが OpenTofu のステートに入りません。プロジェクトには 2 つの接続文字列があり、デプロイではそれぞれを使うべき場所で使います。

- **ダイレクト**のもの（`-pooler` のないホスト）は必須です。マイグレーションのジョブとコンソールは常にこちらを使います。`my schema apply` はセッションレベルのロックを持ちますが、トランザクションモードのプーラーはそれを保てないためです。
- **プール**のもの（`-pooler` のあるホスト）は、サービス用の任意のものです。Neon のプーラーはサーバーに必要なことを処理できます。[コネクションプーラー経由](database.md#through-a-connection-pooler)を参照してください。`database = { pooled = true }` を設定すると、モジュールがそのための 2 つ目のシークレットを作ります。

プールの文字列がなければ、サービスもダイレクトのものを使い、データベースの接続数の上限の範囲で動きます。その上限は `service.max_instances` で抑えられます。Cloud Run のインスタンスが同時に受ける 80 リクエストのために、インスタンスあたり 4 という既定のプールを増やすには、サービスが使う文字列に `pool_max_conns` を足します。例: `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`。[データベース接続の待機](running.md#waiting-for-a-database-connection)を参照してください。

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. 例をコピーして変数を設定する

デプロイするリリースから、プロバイダーのロックファイルを含めて、空のディレクトリにコピーして始めます。

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` は `google` プロバイダーと 1 つのブロック `module "mycroblog"` です。**`ref` をそのリリースにしてください。** ブロックは `?ref=` でリリースを指していて、安定版のリリースのコピーなら最初から正しくなっています。サーバーのイメージも同じリリースなので、`ref` だけで動くものが決まります。プレリリースでは両方を自分で設定します。`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf` と、ブロックに `image = { version = "0.1.0-rc.4" }`（先頭の `v` を除いたバージョン）です。プレリリースのモジュールは、自分のイメージを既定にしないためです。モジュールはそれを含む最初のリリースからしか存在しないので、それより前のタグにはコピーする例がありません。このディレクトリは残しておいてください。ステートと設定がここにあります。

`terraform.tfvars` を埋めます。`mail_from`、`mail_smtp_host`、`mail_smtp_username` は[メール](email.md)の設定で、Google Cloud は外向きのポート 25 を塞いでいるため、ポートは 587 です。ほかを変えるには、`ingress = { kind = "domain-mapping" }` のように、`main.tf` のブロックにそのセクションを足します。すべてのセクションと、それが拒むものは[モジュールの README](../../deploy/opentofu/README.md#sections)にあります。

`settings` には、[設定リファレンス](../reference/configuration.md)のほかの設定をキーで渡せます。`secrets` にはシークレットな設定のキーを渡せて、それぞれに値を入れるための空のシークレットが作られます。セクションやモジュールが設定するキーはプランが失敗し、`listen` のように Cloud Run ではモジュールが受け付けないキーも失敗します。エラーが理由を説明します。ハンドルに別のドメインを使うなら、[`account_domain`](../reference/configuration.md#account_domain) をいま `settings = { account_domain = "example.org" }` のように決めてください。

**ステート**は既定ではこのディレクトリに残ります。シークレットの値は入っていませんが、何を作ったかを示す唯一の記録です。複数人で運用するなら、`backend "gcs"` ブロックでバケットに置いてください。

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. シークレットを作り値を追加する

シークレットを作り、KEK と VAPID 鍵を生成します。

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` は、そのリリースのスクリプトも取得します。適用が実行する `migrate.sh` と、あとの手順で使う `console.sh` です。

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**いますぐ KEK をコピー**して、データベースとは別の場所、たとえばパスワードマネージャーに保管してください。これがないと[バックアップ](backup.md)を復元できず、アカウントの鍵が失われます。コマンドは `tofu output kek` で確認できます。VAPID 鍵も同じようにしてください。失うと、すべての Web Push の購読が終わります。

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

次に、ほかから持ってくる値を追加します。`tofu output secrets_to_fill` は、モジュールが空のまま作った各シークレットについて、何を入れるかと追加するコマンドを一覧にします。読めないシークレットがあるインスタンスは起動しないので、サービスより前に追加してください。

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK は `MY_KEK_FILE` が指す、マウントされたファイルとしてサーバーに届きます。ほかは、インスタンスの起動時に解決される環境変数なので、新しいバージョンは次のインスタンスから有効になります。確実にするには、新しいリビジョンをデプロイしてください。

**OpenTofu が KEK を生成するのは一度だけで、二度とありません。** シークレットとそのバージョンには `prevent_destroy` があり、新しい値を書き込むバージョン番号はモジュールの中で固定されています。KEK を新しい乱数にすると、古い KEK で暗号化されたものがすべて読めなくなるからです。ローテーションするには[KEK のローテーション](key-management.md#rotate-the-kek)に従い、新しい値を `gcloud secrets versions add` で追加します。そこで追加したものを OpenTofu は追跡しません。

最初から KEK を Cloud KMS に置くには、この手順の前に `kek = { kind = "kms" }` を設定し、`kek_secret_manager` の代わりに `module.mycroblog.module.kek_kms` を対象にして、`cloudkms.googleapis.com` を有効にします。

<!-- source: 5-apply 2076866c0f3b -->
### 5. 適用する

```sh
tofu apply
```

これで残りがすべて作られます。順序はサービスの起動プローブが必要とするものです。プローブは `/health/ready` を待ち、スキーマができるまでそれは `503` を返します。まずマイグレーションのジョブが、依存するものとともに作られます。その中にはロードバランサーのアドレスも含まれます。サーバーの設定がそれを名指すためです。次に、適用はジョブの実行として `my upgrade` を、あなたのマシンから `gcloud` を通して実行し、終わるのを待ちます。実行されるのは `my config check`、`my db upgrade -check`、`my schema apply`、`my db upgrade` で、最初に失敗したところで止まります。その出力は適用の出力に表示されます。データベースを使う最初のコマンドが、`host` と `account_domain` をそこに記録します。サービスが作られるのはそのあとです。

マイグレーションが失敗すると、適用はサービスの前で止まります。原因（多くは値のないシークレット）を直して、もう一度 `tofu apply` を実行してください。マイグレーションがもう一度実行されます。

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. ドメインをロードバランサーに向ける

`tofu output dns_records` は、`host` からロードバランサーのアドレスへの `A` レコードを 1 つ一覧にします。DNS プロバイダーで追加してください。そのプロバイダーに独自のプロキシがあるなら、このレコードではオフにしてください。

証明書は Google が管理し、DNS がそのアドレスを指してから有効になるので、時間がかかることがあります。それまで `https://{host}` は TLS エラーになるため、連合もブラウザーもまだサーバーに届きません。ほかにそれを待つものはありません。Cloud Tasks と Cloud Scheduler は、サービスの既定の `run.app` URL、`https://<service>-<project number>.<region>.run.app` を呼びます。**既定の URL を無効にしないでください。** ジョブが止まります。その URL はインターネットからのリクエストを拒みます。Cloud Run がこの 2 つを内部からとみなすのは、同じプロジェクトからで、その URL に対してだけであり、カスタムドメインではそうならないためです。

ほかに選べるものは[トラフィックがサービスに届く経路](#how-traffic-reaches-the-service)を参照してください。

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. 最初の管理者を作る

Cloud Run のジョブには端末も標準入力もないので、`my admin create` はパスワードを読み取る代わりに、パスワードを設定するリンクを表示します。[コンソール](#the-console)をオンにして、次を実行します。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

リンクをすぐに開いて、パスワードを設定してください。リンクは 1 回だけ、1 時間のあいだ使えます。**それまでは Cloud Logging に残る**ので、プロジェクトのログを読める人なら誰でも使えます。期限が切れたら、コンソールで `my admin account password-reset-link @alice` を実行してください。そのあと、二要素認証をオンにします。

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. チェックリストを確認する

[本番運用前のチェックリスト](production-checklist.md)を確認してください。最初は[メール](production-checklist.md#email)で、`mail_from` のドメインの SPF、DKIM、DMARC です。

<!-- source: the-console 52df6f8c43ac -->
## コンソール

コンソールは、サービスと同じイメージ、設定、シークレットで、`my` のコマンドを 1 つ実行します。`console_enabled` がオンのあいだだけ存在するので、終わったらオフにしてください。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` はコマンドを待ち、その出力と終了コードを表示して、そのコードで終了します。ジョブの出力は Cloud Logging に送られるので、`roles/logging.viewer` が必要です。使うイメージはサービスが動かしているものです。マイグレーションで止まったアップグレードのあとなら、データベースがまだ合っている前のリリースです。

コマンドには標準入力も端末もないので、そこからパスワードやファイルを読むコマンドは、この方法では実行できません。出力はログに残るので、ログに書きたくない出力のコマンドは実行しないでください。

<!-- source: upgrading fe1bed77d3af -->
## アップグレード

先に[アップグレード](database.md#upgrading)とリリースノートを読み、マイナーバージョンを 1 つずつ上げてください。アップグレードは新しいリリースの適用です。プランには、すべてのサービスとジョブが新しいイメージを使う様子が表示され、適用はそのどれかが配信を始める前にマイグレーションを実行します。

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

イメージは `ref` が指すリリースなので、`ref` を変えることがアップグレードのすべてです。適用する前にプランを読んでください。新しいリリースのモジュールが、リソースも変えることがあります。適用は次を行い、最初に失敗した手順で止まります。

1. マイグレーションとコンソールのジョブに新しいイメージを渡し、サービスが依存しないプランの残りも適用します。
2. 適用を行う場所から、マイグレーションのジョブの実行として `my upgrade` を実行し、終わるのを待ちます。実行されるのは `my config check`、`my db upgrade -check`、`my schema apply`、`my db upgrade` です。その出力は適用の出力に表示されます。
3. 各ワーカーの新しいリビジョンを、続いてサービスの新しいリビジョンをデプロイします。リビジョンがトラフィックを受けるのは、起動プローブが `/health/ready` で通ったときだけです。

**適用を行う場所**には、`gcloud` と `jq`、そしてジョブを実行してそのログを読める認証情報（`roles/run.developer` と `roles/logging.viewer`。オーナーに含まれます）が必要です。適用する CI のジョブも同じです。

**手順が失敗したとき**は、サービスは元のリビジョンのままで、リリースノートかマイグレーションの出力に理由が書かれています。原因を直して、同じバージョンのままもう一度 `tofu apply` を実行してください。マイグレーションがもう一度実行され、完了した手順にはやることが残っていません。中断された適用や、接続が切れた適用は、実行を Cloud Run 上に走らせたままにします。次の適用は、自分の実行を始める前に、それが終わるのを待ちます。スキーマが変わったあとで戻すには、バックアップを復元してください。[ロールバック](database.md#rolling-back)を参照してください。前のバージョンでの適用は、そのリリースが新しいスキーマを拒むので、マイグレーションで止まり、どのサービスも変えません。

リリース候補を試すとき、またはより新しいモジュールのもとで以前のリリースのイメージを保つときは、バージョンを自分で設定します。これはリリースの `v` を除いた、イメージのタグです。

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### 先に SQL を確認する

プランにはマイグレーションの SQL が表示されません。適用の前にそれを読むには、マイグレーションのジョブだけに新しいイメージを渡し、そこで何も変更しない `my upgrade -dry-run` を実行します。

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### 破壊的な変更

リリースノートが `-allow-destructive` を求めているときは、そのバージョンをブロックに書きます。

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` に `-allow-destructive` が渡されるのはそのバージョンの適用のときだけなので、値を残したままにしても、あとのリリースでデータが削除されることはありません。

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### ダイジェストでイメージを固定する

[本番運用前のチェックリスト](production-checklist.md#operations)は、リリースノートにあるダイジェストの使用を勧めています。

```hcl
  image = { digest = "sha256:..." }
```

ダイジェストは、隣のタグが何であれ、1 つのイメージを指します。そのため `ref` と同じ編集で変えてください。前のリリースのダイジェストが残っていると、新しいバージョンのもとでもそのリリースのイメージのままになります。

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### メンテナンスモード

[メンテナンスモード](maintenance.md)には 3 回の適用が必要です。サービスがアップグレードの前と後で変わるためです。ブロックの設定に追加して適用し、バックアップを取り、`ref` を上げて適用し、それから取り除いて適用します。

```hcl
  settings = { maintenance = "true" }
```

これがないと、マイグレーションのあいだも古いリビジョンが配信を続けますが、それは[動くことが保証されていません](database.md#the-server-during-an-upgrade)。`gcloud` で手作業で変えた設定は残りません。次の適用が、ブロックに書かれたものに戻します。

<!-- source: monitoring-and-alerts 930234cd0dde -->
## 監視とアラート

モジュールは、`/health/live` の稼働チェック、ログベースのメトリクス、[Runbook](alerts.md) の名前を持つアラートポリシーを追加します。通知は `monitoring.notification_emails`（例では `alert_emails`）のアドレスか、`notification_channels` のチャネルに届きます。この入力に既定値はありません。通知する相手を指定するか、`monitoring = null` にして全部を除いてください。まだ一度も適用されていないので、一度チェックを失敗させて試してください。チェックは実行のたびにインスタンスを起こすので、[tick のトレードオフ](#costs)に加わります。その頻度が `uptime_period` で、既定は 15 分です。

<!-- source: changing-a-backend c20598caf230 -->
## バックエンドを変える

そのセクションの `kind` を変えます。各 kind は、サーバーに必要なものを設定し、何が変わるかはプランが示します。データベースを変えるときは、新しいものは空なので、データも移すことになります。古いものの[バックアップ](backup.md)を取り、何も書き込まれていないあいだに新しいものへ復元してください。たとえば、Neon の代わりに Cloud SQL を使うなら次のようにします。

```hcl
  database = { kind = "cloud-sql" }
```

Google Cloud の外のストアを使うには、`media.kind` を `s3`（`endpoint` を指定した S3 互換のサービス）か `r2`（`cloudflare.account_id` を指定）にします。どちらも、`secrets_to_fill` が一覧にするシークレットからアクセスキーを読みます。

<!-- source: when-you-need-a-network 0a4359c002ed -->
## ネットワークが必要なとき

このガイドには不要です。Neon、Cloud Tasks、Cloud Storage、Pub/Sub はすべて、サービス自身の ID でインターネット経由で届くので、Cloud Run は何にも接続せず、支払うものも面倒を見るものもありません。

必要になるのは、デプロイに Memorystore を入れるときです。Memorystore にはプライベートアドレスしかありません。また、Cloud SQL にパブリックアドレスの代わりにプライベートアドレスを与えるときも必要です。`network = { kind = "vpc" }` でネットワークが作られ、`network = { kind = "existing", ... }` は既存の VPC に接続します。その VPC は `network`、`subnetwork`、`private_services_range`、`memorystore_connection_policy` で指定します。

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

ネットワークを通るのはプライベートの範囲だけなので、サービスはこれまでどおり、Cloud NAT なしでインターネットに出ます。`nat = true` にすると、代わりにすべてが Cloud NAT を通り、送信元アドレスが固定されます。先に Service Networking API、Network Connectivity API、Compute Engine API を有効にしてください。ネットワークはインスタンスのコールドスタートを遅くし、Memorystore はサーバーがアイドルのあいだも課金されます。

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## ジョブをワーカーに分ける

ある種類のジョブがほかを遅らせるときは、それに専用のサービスを与えます。何を分けるかは、[ジョブの種類でワーカーを分ける](running.md#splitting-workers-by-job-type)にあるとおり、メトリクスから決めます。値は `workers` の 1 つで、サービス、キュー、監視に渡されます。キュー、URL、ルート、設定はすべてそのキーから作られるので、自分で書くものはありません。

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

一緒にしておくべき種類を分けるプランは拒否されます。また、すべてのプロセスに届くブローカーが必要で、`stream.kind = pubsub` がそれにあたります。

そのページにある 2 つのよくある構成は、次のようになります。配信がほかを遅らせないようにするには、上の値を使います。メールと通知を速く保つには次のようにします。

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
```

そのページにあるとおり、設定していない機能の種類は外してください。サーバーが登録していない種類があると起動が止まり、プランはそれがどれか見分けられません。`workers` があると、これらの種類のタスクは専用のキューに入るので、既定のサービスが空いていても手伝いません。

各ワーカーは `job_types` と、`min_instances`、`max_instances`、`cpu`、`memory_mib`、`concurrency`、`max_concurrent_dispatches`、`max_dispatches_per_second` を取ります。名指ししなかったものは、サービスかキューの値になります。ワーカーのサービスは外からのトラフィックを受けません。アップグレードでは、各ワーカーがマイグレーションのあと、既定のサービスの前に移され、それを知らせるものはありません。止まるのは手順が失敗したときだけです。各ワーカーにはサービスと同じ費用がかかりえて、そのインスタンスの接続はデータベースの上限に数えられます。

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## トラフィックがサービスに届く経路

これは `ingress.kind` で決まります。既定はロードバランサーで、本番向けです。ドメインマッピングは、デプロイを安く試すためのものです。`ingress = { kind = "domain-mapping" }` を設定します。

| | `load-balancer`（既定） | `domain-mapping` |
| --- | --- | --- |
| 月額 | 転送ルールで約 $18 に、GB あたりの料金が加わります。[料金のページ](https://cloud.google.com/vpc/network-pricing)で確認してください。この金額はそこでは確認できませんでした | なし |
| 位置づけ | Google Cloud が推奨する方法 | レイテンシーのため、Google Cloud が本番には勧めていない Preview の機能 |
| リージョン | すべて | `asia-northeast1`、`us-central1`、ほか数か所 |
| DNS | `tofu output dns_records` の `A` レコード | 同じ出力が一覧にするレコード |
| 証明書 | Google が管理。DNS がそのアドレスを指すと有効になります | Google が管理。最大 24 時間 |
| ドメインの所有権 | DNS のほかには何も不要 | あなたと、適用するほかのすべてのアカウントが、適用する前に[確認済みの所有者](https://cloud.google.com/run/docs/mapping-custom-domains)になっている必要があります |
| `run.app` URL | インターネットからは拒否されるので、ロードバランサーを迂回できません | 誰でも届きます。ドメインマッピングはヘッダーを足せないので、オリジンは開いたままです |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` とロードバランサーのアドレス | `169.254.0.0/16` |

Cloud Tasks と Cloud Scheduler は、どちらも経由せず、既定の `run.app` URL を直接呼びます。

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` は、すべてのキャッシュを回避して `host` の前に Cloudflare を置きます。`ingress.media_host` は、どの種類とも併用できる、キャッシュされるメディアホストを追加します。どちらも `cloudflare = { zone_id = "..." }` が必要で、トークンは `CLOUDFLARE_API_TOKEN` に入れます。どちらもモックしたプロバイダーでしか動かしていません。

- **オリジンはプランで決まります。** Cloud Run は `Host` でルーティングしますが、Cloudflare が Host ヘッダーの上書きを提供するのは Enterprise プランだけです。ほかのプランでは、オリジンが自分でそのホストを受け付ける必要があるため、モジュールは各ホストのドメインマッピングを作り、Cloudflare はそれに接続します。上の表のトレードオフがあります。その証明書がプロキシの後ろで発行され、更新されるかは確認できていないので、`cloudflare.proxied = false` で始めてください。Enterprise プランでは `cloudflare.host_header_override = true` にすると、ドメインマッピングなしで `run.app` URL に接続します。
- **ヘッダーがオリジンを閉じます。** モジュールはリクエストヘッダーを変換するルールを追加し、サーバーは、ヘルスチェックと、トークンを確認するジョブ自身の `POST` を除いて、ヘッダーのないリクエストを `403` で拒否します。そのため `run.app` URL を通って Cloudflare を迂回することはできず、呼び出し側の Cloudflare Worker が、レート制限に見えるクライアントのアドレスを選ぶこともできません。
- **値はステートに入ります。** モジュールがそれを生成してシークレット `my-origin-secret` に書き込みますが、Cloudflare プロバイダーにはルールのヘッダー値の書き込み専用の形式がないので、ステートは非公開にしてください。ローテーションの仕組みは [CDN のガイド](cdn.md#rotate-the-value)にあります。
- **レコードがプロキシされていないあいだは、何もヘッダーを足しません。** そのためサーバーはヘルスチェック以外をすべて拒否します。ドメインマッピングの証明書のために `proxied = false` にしたときは、証明書が有効になってからプロキシをオンにしてください。
- **Bot Fight Mode はオフにしてください。** これはゾーン全体に及び、どのルールでも飛ばせません。`/ap/`、`/.well-known/`、`/api/` へのチャレンジは、ほかのサーバーがブラウザーではないため、連合を壊します。ジョブは Cloudflare を通りません。モジュールはチャレンジを追加せず、Browser Integrity Check をオフにしますが、Bot Fight Mode は管理しません。同じ理由で「I'm Under Attack」もオンにしないでください。
- **Email Address Obfuscation はオンのままで構いません。** これは HTML 中のメールアドレスを `[email protected]` に書き換えて、元に戻すスクリプトを足しますが、サーバーの Content-Security-Policy がそのスクリプトを拒否します。確認メールの送信先を伝えるページなど、サーバー自身のページは、アドレスに手を加えないよう内容に印を付けています。
- **Always Use HTTPS** は、`http://` をリダイレクトしたいならダッシュボードでオンにしてください。ゾーン全体に及ぶので、モジュールはあなたに任せています。
- **タイムアウト**: オリジンが 125 秒間何も送らないと、Cloudflare は `524` を返します。ジョブは `run.app` URL を直接呼ぶので影響を受けません。ストリームは 30 秒ごとにハートビートを送り、900 秒のアイドル上限より短くなっています。通常のプロキシされたホストで Cloudflare がストリームをバッファするかどうかは確認できていません。
- **アップロード**: Free と Pro では 100 MB で、Cloud Run の 32 MiB より大きいため、32 MiB が上限のままです。
- **動画**: Cloudflare の規約は、Free、Pro、Business で動画や大きなファイルを不釣り合いに多く配信することを制限しており、コンテンツをリダイレクトするなどの措置を取ることがあります。メディアホストで動画をキャッシュする前に、これを判断してください。

**削除したファイルはキャッシュに残ります。** メディアホストはサーバーの `Cache-Control` に従うので、削除したローカルのファイルは最大 1 年、リモートのファイルは `remote_media_cache_ttl_seconds` のあいだ見えたままになることがあります。サーバーはこれをパージしません。何かを消す必要があるときは、ダッシュボードの **Caching > Configuration > Custom Purge** か、次のコマンドで手作業でパージしてください。

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

プロキシの URL はクエリ文字列に `sig` を持ち、それもキャッシュされるものの一部なので、URL 全体を指定してください。

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## KEK のバックエンドを変える

`kek.kind` を変えるのは `main.tf` の 1 行ですが、データのあるインスタンスでは無料ではありません。データベースには現在の鍵で包まれたシークレットが入っていて、鍵を失ったインスタンスはそれを読めなくなり、すべてのアカウントの鍵もその中にあります。

[ローカルの KEK と KMS の間で移す](key-management.md#move-between-the-local-kek-and-a-kms)に従ってください。シークレットから Cloud KMS へ移すときは、シークレットを移すあいだ、現在の KEK を `kek_previous` として残します。`secrets = ["kek_previous"]` を追加し、シークレット `my-kek-previous` に現在の KEK を入れます。[コンソール](#the-console)で `my kek rewrap` を実行し、`my kek status` が `local:` の鍵を数えなくなったら取り除きます。再ラップより前のデータベースのバックアップを持っているあいだは、KEK も保管しておいてください。

データのない新しいインスタンスでは、移すものはありません。

<!-- source: limits 3ff113a57279 -->
## 制限

- **アップロード**: Cloud Run は 32 MiB を超える HTTP/1 のリクエストを拒否し、サーバーは HTTP/1.1 で話します。そのためモジュールは `MY_MEDIA_MAX_AV_BYTES` を、既定の 100 MiB ではなく 30 MiB にして、それより大きなアップロードにはサーバー自身が応答するようにしています。
- **ストリーム**: リクエストのタイムアウト（既定で 15 分）で開いたストリームが終わり、クライアントは再接続します。
- **クライアントのアドレス**: レート制限は、モジュールがイングレスから設定する `MY_TRUSTED_PROXIES` に頼っています。直接のリクエストに Cloud Run が `X-Forwarded-For` に何を足すのか、コンテナがどのピアアドレスを見るのかを書いた、私たちの読んだ文書はなく、コンテナの契約もどちらにも触れていないので、`169.254.0.0/16` は推測です。外部ロードバランサーの文書化された形式は `[<supplied>,]<client-ip>,<lb-ip>` です。最初のリクエストのログでアドレスを確認してください。クライアント IP が `169.254` のアドレスや Google のアドレスなら、その設定はあなたのプロジェクトでは間違っています。
- **公開アクセス**: サービスは `allUsers` に `roles/run.invoker` を与えますが、ドメイン制限共有の組織ポリシーはこれを拒否します。そのようなプロジェクトには例外が必要です。

<!-- source: costs b8be3997d723 -->
## 費用

サーバーには費用がかかります。ここの数字は、何を見込むか、どこで工夫すれば減らせるかの目安であり、デプロイが守る上限ではありません。2026-10 と 2026-11 時点の、1 つのリージョンのおおよその定価です。[Cloud Run](https://cloud.google.com/run/pricing) と [Neon](https://neon.com/pricing) の料金のページで確認してください。コンピュートは、請求アカウントごとに月 180,000 vCPU 秒と 360,000 GiB 秒まで無料です。

| 部分 | アイドル時 | 増える要因 |
| --- | --- | --- |
| ロードバランサー | 月に約 $18。料金のページでは確認できていません | GB あたりの料金。[トラフィックがサービスに届く経路](#how-traffic-reaches-the-service)を参照 |
| Neon の Launch プラン | 既定の tick で月に約 $19 | ストレージは GB・月あたり $0.35。代わりに `database.kind = cloud-sql` にすると、小さなマシンで固定費は月に約 $9.40 |
| Cloud Run | $0。min instances は 0 で、CPU はリクエスト中だけ課金されます | 開いたストリームごとにインスタンスが課金されます。下を参照 |
| Cloud Tasks | $0 | 100 万オペレーションまで無料、以降は 100 万あたり $0.40。ジョブ 1 つで約 2 オペレーション |
| Cloud Scheduler | $0。請求アカウントごとに 3 ジョブまで無料 | それを超えるとジョブあたり $0.10 |
| Cloud Storage | 月に 1 GiB あたり約 $0.02 | 5 GiB が無料なのは `us-east1`、`us-west1`、`us-central1` だけ。メディアは Cloud Run を通して出ます |
| Pub/Sub | $0。10 GiB まで無料 | ストリームが開いているあいだだけ |
| Secret Manager | 数セント | 数バージョンまで無料、以降はバージョンあたり数セント |

**tick が Neon を起こし続けます。** tick は毎回データベースを読み書きし、Neon がコンピュートを止めるのは 5 分間アクティビティがないときだけなので、既定の `* * * * *` ではコンピュートが止まりません。月に 730 時間、0.25 CU で 182.5 CU 時間になり、これが上の $19 です。定期ジョブが時間どおりに動き、失われた起こす合図がすぐ回復されるよう、tick は毎分のままです。

**開いたストリームはインスタンスを課金させ続けます。** リクエストベースの課金では、インスタンスは何かのリクエストを処理しているあいだ課金され、リアルタイム更新は 1 本の長いリクエストです。ストリームを 1 か月開いたままにするクライアントが 1 つあると、インスタンス 1 つが 24 時間分かかります。無料枠のあとで、1 vCPU と 512 MiB なら月に約 $60 です。ストリームが増えると、`concurrency` までは同じインスタンスを共有します。ストリームがほとんどずっと開いているなら、インスタンスベースの課金や、小さな VM と [Docker Compose](deploy-docker-compose.md) と比べてください。

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### 工夫で減らせる部分

- **ドメインマッピング**をロードバランサーの代わりに使うと $18 がなくなりますが、[上のトレードオフ](#how-traffic-reaches-the-service)があります。デプロイを試すのに向いています。
- **Neon の Free プラン**で $19 がなくなり、小さなインスタンスならそれで動かせますが、次のトレードオフがあります。
  - 月に 100 CU 時間までで、既定の tick なら約 17 日で使い切ります。`queue = { tick_schedule = "*/10 * * * *" }` のように、間隔の長いスケジュールにすれば収まります。ほかが起こす前の時点で、起きている時間は最大でも半分、約 91 CU 時間です。そのぶん定期ジョブは最大 10 分遅れて動き、失われた起こす合図の回復も遅くなります。
  - 連合するサーバーは配信を 1 日中受け取り、それがデータベースも起こすので、忙しいインスタンスはどんなスケジュールでも枠を超えます。
  - 1 プロジェクトあたり 1 GB で、復元できる履歴は 6 時間です。コンピュートは 5 分アイドルになると必ず止まるので、そのあとの最初のリクエストはその起動を待ちます。
