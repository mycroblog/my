<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# Cloud Run へのデプロイ

[`deploy/opentofu`](../../deploy/opentofu/README.md) の OpenTofu のモジュールを使って、Google Cloud で `my` を動かす方法です。Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage、Pub/Sub を使い、データベースには Neon の PostgreSQL を使い、グローバル外部アプリケーションロードバランサーの後ろに置きます。誰も使っていない間、サービスはインスタンスを 1 つも動かしません。ただし、毎分サーバーを起こすティックだけは例外です。サーバーにはお金がかかります。ストレージ、通信量、開いているストリームの分を除いて月におよそ $37 を見込み、工夫で減らせるところは[費用](#costs)を読んでください。

モジュールには OpenTofu 1.11 以降が必要です。Terraform はサポートしていません。

<!-- source: what-it-creates c95687df197f -->
## 作られるもの

| 部品 | モジュール | リソース |
| --- | --- | --- |
| サーバー | `app-gcp-cloud-run` | `role = all` の Cloud Run サービス、マイグレーションのジョブ、必要な間だけのコンソールのジョブ、サービスアカウント、公開アクセス |
| トラフィック | `ingress-gcp-load-balancer`、または `ingress-gcp-domain-mapping` か `ingress-cloudflare-app`、加えて任意で `ingress-cloudflare-media` | 証明書付きのグローバル外部アプリケーションロードバランサー、またはドメインマッピング、または Cloudflare と、メディア用のホスト |
| ジョブ | `queue-gcp-cloud-tasks` | Cloud Tasks のキュー、Cloud Scheduler のジョブ、サーバーを呼び出すサービスアカウント |
| メディア | `blob-gcp-gcs` | 非公開のバケット |
| リアルタイムの更新 | `stream-gcp-pubsub` | トピックとカスタムロール。サブスクリプションはサーバーが自分で作成・削除します |
| データベース | `db-external` | なし。データベースは手で作ります |
| メール | `mail-smtp` | なし。どの SMTP サービスでもかまいません |
| シークレット | `secrets-gcp-secret-manager` | あなたが値を入れるデータベースの URL と SMTP のパスワードのシークレット、生成される VAPID キー |
| KEK | `kek-gcp-secret-manager` または `kek-gcp-kms` | ファイルとしてマウントされる生成済みのシークレット、または Cloud KMS の鍵 |

シークレットの値が OpenTofu のプランや状態に入ることはありません。ほかから来る値はあなたが自分で追加し、KEK と VAPID キーはデプロイが書き込み専用の属性を通して生成します。

<!-- source: before-you-start 4c624a6b7316 -->
## 始める前に

- `tofu` 1.11 以降、`gcloud`、`jq`。
- このインスタンスだけに使う、課金が有効な Google Cloud のプロジェクトと、そのプロジェクトでオーナーのロールをもつアカウント。
- [`host`](../reference/configuration.md#host) のためのドメイン。あとから変えられません。[あとから変えられない設定](production-checklist.md#settings-you-cannot-change-later)を読んでください。
- [リリース](running.md#get-the-image)にある `my` のバージョン。`latest` タグはありません。

<!-- source: first-deployment 996fcc6eb35e -->
## 最初のデプロイ

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. API を有効にする

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` はロードバランサーのためのものです。

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. Neon にデータベースを作る

Launch プランを見込んでください。既定のティックでは、コンピュートが止まることはありません。Free プランでも動きますが、[費用](#costs)に書いたトレードオフがあります。

方法は 2 つあり、例では 1 つ目を使います。

- **`db-external` を使って手で作る。** Neon のコンソールで、Cloud Run のリージョンに近いリージョンにプロジェクトを作り、その接続文字列をコピーします。パスワードは OpenTofu の状態に入りません。
- **[`db-neon`](../../deploy/opentofu/modules/db-neon/README.md) を使う。** プロジェクトを作り、URL をシークレットに書き込んでくれます。**このときデータベースのパスワードは状態に入ります。** またプロバイダーはメンテナンスされていません。それが何を意味するかはその README にあり、`main.tf` の `db` ブロックのコメントに差し替え方があります。

プロジェクトには 2 つの接続文字列があり、デプロイはそれぞれを適した場所で使います。

- **直接接続**のもの（ホストに `-pooler` が付かないもの）は必須です。マイグレーションのジョブとコンソールは常にこれを使います。`my schema apply` はセッション単位のロックを保持し、トランザクションモードのプーラーではそれが保たれないからです。
- **プール接続**のもの（ホストに `-pooler` が付くもの）は任意で、サービス用です。Neon のプーラーはサーバーが必要とするものを扱えます。[接続プーラーを通す](database.md#through-a-connection-pooler)を参照してください。専用のシークレットに入れて `db` モジュールの `pooled_url_secret_id` に渡し、そのシークレットを `secrets` モジュールの `referenced` に加えます。

プール接続の文字列がなければ、サービスも直接接続のものを使い、データベースの接続数の上限に従います。その数は app モジュールの `max_instances` で抑えられます。Cloud Run のインスタンスが同時に受ける 80 件のリクエストのために、インスタンスあたり既定で 4 のプールを増やすには、サービスが使う文字列に `pool_max_conns` を加えます。たとえば `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8` です。[データベースの接続を待つ](running.md#waiting-for-a-database-connection)を参照してください。

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
### 3. 例をコピーして変数を設定する

デプロイするリリースから、プロバイダーのロックファイルも含めて、空のディレクトリにコピーしたものから始めます。

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**`ref` をそのリリースにします。** `main.tf` のどのモジュールブロックも `?ref=` でリリースを指定しており、安定版のリリースのコピーではすでに正しくなっています。プレリリースでは自分で設定してください: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`。モジュールはそれを含む最初のリリースからしか存在しないので、それより前のタグにはコピーする例がありません。`ref` と `image` には同じリリースを指定し、このディレクトリは残しておいてください。あなたの状態と設定が入っています。

`terraform.tfvars` を埋めます。`mail_from`、`mail_smtp_host`、`mail_smtp_port`（既定は 587。Google Cloud は外向きのポート 25 をブロックします）、`mail_smtp_username` で[メール](email.md)を設定します。`env` と `secret_env` には、[設定リファレンス](../reference/configuration.md)のそのほかの設定項目を、それぞれ値とシークレットの ID で指定します。2 か所で指定した設定項目があるとプランが失敗します。

メールを送らないなら、`main.tf` から `mail` モジュールとそのシークレットを削除します。`mail` は `none` のままになります。その前に[登録にとってそれが何を意味するか](production-checklist.md#email)を読んでください。

ハンドルに別のドメインを使うなら、[`account_domain`](../reference/configuration.md#account_domain) をいま `env` の `MY_ACCOUNT_DOMAIN` として決めてください。

**状態**は既定ではこのディレクトリに置かれます。シークレットの値は含みませんが、何が作られたかの唯一の記録です。複数の人が運用するなら、`backend "gcs"` ブロックでバケットに置いてください。

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. シークレットを作り、値を追加する

次のコマンドでシークレットを作り、KEK と VAPID キーを生成します。

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` は、次の手順で使う `upgrade.sh` と `console.sh` もそのリリースのものを取得します。

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**いますぐ KEK をコピーし**、そのコピーはデータベースとは別の場所、たとえばパスワードマネージャーに保管してください。KEK がないと[バックアップ](backup.md)を復元できず、アカウントの鍵が失われます。VAPID キーも同じようにしてください。これを失うと、すべての Web Push の購読が終わります。

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

次に、ほかから来る値を追加します。シークレットを読めないインスタンスは起動しないので、アプリより前に追加してください。

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK は、`MY_KEK_FILE` で指定されるマウントされたファイルとしてサーバーに届きます。ほかのものはインスタンスの起動時に解決される環境変数なので、新しいバージョンは次のインスタンスから有効になります。確実にするには新しいリビジョンをデプロイしてください。

**OpenTofu が KEK を生成するのは 1 回だけで、二度と生成しません。** シークレットとそのバージョンには `prevent_destroy` が付いており、新しい値を書き込むことになるバージョン番号はモジュールの中で固定されています。これを変えないでください。新しいランダムな KEK になると、古い KEK で暗号化したものがすべて読めなくなります。ローテーションするには [KEK をローテーションする](key-management.md#rotate-the-kek)に従い、新しい値は `gcloud secrets versions add` で追加します。そこで追加したものを OpenTofu は追跡しません。

最初から KEK を Cloud KMS に置くには、`main.tf` の `kek` ブロックをそのコメントのとおりに置き換え、`cloudkms.googleapis.com` を有効にします。自分の KEK や VAPID キーを持ち込むには、この手順の前に、`kek` モジュールに `create = false` を設定するか、`generate_p256_keys` からその鍵を削除します。ほかで作ったシークレットに必要なのはその ID だけです。app モジュールの `secret_env` か、それを使うモジュールに渡してください。

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. マイグレーションに必要なものを作る

サービスの起動プローブは `/readyz` を待ちますが、これはスキーマができるまで `503` を返すので、スキーマを先に作る必要があります。マイグレーションのジョブと、それが依存するものを作ります。サーバーの設定がロードバランサーのアドレスを指定しているので、そのアドレスも含まれます。

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. スキーマを適用する

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

`my config check`、`my db upgrade -check`、`my schema apply`、`my db upgrade` を、それぞれジョブの実行として行い、最初に失敗したところで止まります。データベースを使う最初のコマンドが、`host` と `account_domain` をデータベースに記録します。

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. 残りを作る

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. ドメインをロードバランサーに向ける

`tofu output dns_records` は、`host` からロードバランサーのアドレスへの `A` レコードを 1 つ表示します。これを DNS プロバイダーに追加してください。そのプロバイダーに独自のプロキシがあるなら、このレコードでは無効にしておきます。

証明書は Google マネージドで、DNS がそのアドレスを指してから初めて有効になり、それには時間がかかることがあります。それまでは `https://{host}` が TLS のエラーで失敗するので、連合もブラウザもまだサーバーに届きません。ほかにこれを待つものはありません。Cloud Tasks と Cloud Scheduler は、サービスの既定の `run.app` の URL、`https://<service>-<project number>.<region>.run.app` を呼び出します。**既定の URL を無効にしないでください。** 無効にするとジョブが止まります。その URL はインターネットからのリクエストを拒否し、Cloud Run がこの 2 つを内部からのものとして扱うのは、同じプロジェクトからでその URL に来たときだけで、カスタムドメインに来たときではありません。

ほかに選べるものについては[トラフィックがサービスに届く経路](#how-traffic-reaches-the-service)を参照してください。

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. 最初の管理者を作る

Cloud Run のジョブには端末も標準入力もないので、`my admin create` はパスワードを読み込む代わりに、パスワードを設定するリンクを表示します。[コンソール](#the-console)を有効にして、次を実行します。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

すぐにリンクを開いてパスワードを設定してください。リンクは 1 時間の間、1 回だけ使えます。**それまでリンクは Cloud Logging に残り**、プロジェクトのログを読める人なら誰でも使えます。期限が切れたら、コンソールから `my admin account password-reset-link @alice` を実行します。そのあと二要素認証を有効にしてください。

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. チェックリストを確認する

[本番運用のチェックリスト](production-checklist.md)を確認します。まず[メール](production-checklist.md#email)、つまり `mail_from` のドメインの SPF、DKIM、DMARC から始めてください。

<!-- source: the-console 5aebf704c6d7 -->
## コンソール

コンソールは、サービスのイメージ、設定、シークレットで `my` のコマンドを 1 つ実行します。`console_enabled` が有効な間だけ存在するので、終わったら無効にしてください。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` はコマンドの終了を待ち、その出力と終了コードを表示し、その終了コードで終わります。ジョブの出力は Cloud Logging に送られるので、`roles/logging.viewer` が必要です。`image` の値にかかわらず、サービスが動かしているイメージを使います。

コマンドには標準入力も端末もないので、そこからパスワードやファイルを読み込むコマンドはこの方法では実行できません。表示したものはログに残るので、そこに書かれては困る出力をするコマンドは実行しないでください。

<!-- source: upgrading 1a1e204c5173 -->
## アップグレード

まず[アップグレード](database.md#upgrading)とリリースノートを読み、マイナーバージョンを 1 つずつ上げてください。

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

スクリプトは次のことを行い、最初に失敗した手順で止まります。

1. `--maintenance` を付けると、サービスを[メンテナンスモード](maintenance.md)に切り替えます。
2. マイグレーションのジョブに新しいイメージを与えます。
3. `my config check`、`my db upgrade -check`、`my schema apply`、`my db upgrade` を、それぞれ前のものを待ってから実行します。
4. メンテナンスモードを外した新しいリビジョンをデプロイします。新しいリビジョンは、起動プローブが `/readyz` で成功したときにだけトラフィックを受けます。

`--maintenance` を付けないと、手順 2 と 3 の間も古いリビジョンが配信を続けますが、それが[動作する保証はありません](database.md#the-server-during-an-upgrade)。リリースノートで求められたときは `--allow-destructive` を加えます。[本番運用のチェックリスト](production-checklist.md#operations)が勧めるとおり、`--image ghcr.io/mycroblog/my:0.2.0@sha256:...` でダイジェストを固定してください。

そのあと、`terraform.tfvars` の `image` を同じ値に、`main.tf` の `ref` を同じリリースにして、`tofu init` と `tofu plan` を実行します。適用する前にそのプランを読んでください。新しいリリースのモジュールはリソースを変えることがあります。`tofu apply` は既存のサービスやジョブのイメージを決して変えないので、`image` を忘れても害はありませんが、新しいジョブや作り直したサービスはその値から始まります。

**手順が失敗したとき**、サービスは元のリビジョンのままで、求めていればメンテナンスモードのままです。原因を直してスクリプトをもう一度実行してください。手順は繰り返しても安全です。`my schema apply` がまだ実行されていないときに、アップグレードせずにメンテナンスモードを抜けるには、次を実行します。

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

スキーマが変わったあとで戻すには、バックアップを復元します。[ロールバック](database.md#rolling-back)を参照してください。

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## 監視とアラート

モジュール [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) は、`/readyz` の稼働時間チェック、[ランブック](alerts.md)の名前を付けたアラートポリシー、メールでの通知を追加します。このモジュールはまだ一度も適用されたことがないので、チェックを失敗させて一度試してください。先に `monitoring.googleapis.com` を有効にし、チェックにかかる費用をその README で読んでください。各実行はインスタンスを起こし、`/readyz` を通してデータベースも起こすので、[ティックのトレードオフ](#costs)に上乗せされます。

<!-- source: changing-a-backend ef4660f60191 -->
## バックエンドを変える

`main.tf` のモジュールのブロックを置き換えます。app モジュールはブロックが返すものをそのまま受け取るので、ほかは何も変わりません。データベースを変えるときは、新しいデータベースは空なので、データの移行も必要です。手順は [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) にあります。たとえば、自分のデータベースを使うなら次のようにします。

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

Google Cloud の外のストアを使うなら、`blob` ブロックを、S3 互換のサービスなら [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md) に、または [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md) に置き換えます。

<!-- source: when-you-need-a-network fb392704a1e7 -->
## ネットワークが必要なとき

このガイドではネットワークは必要ありません。Neon、Cloud Tasks、Cloud Storage、Pub/Sub にはどれもサービス自身の ID でインターネット越しに接続するので、Cloud Run は何にもアタッチせず、支払うものも管理するものもありません。

ネットワークが必要になるのは、プライベートアドレスしかもたない Memorystore をデプロイに加えるときや、Cloud SQL に公開アドレスの代わりにプライベートアドレスを与えるときです。[`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md) を加えるか、手持ちの VPC を[同じ形](../../deploy/opentofu/README.md#the-network-on-google-cloud)のオブジェクトとして記述し、同じものをアプリと、それを受け取る各モジュールに渡します。

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

アプリにも `network = module.network` を与え、その `backends` に `module.kv` を加えます。ネットワークを通るのはプライベートな範囲だけなので、サービスはこれまでどおりインターネットに接続でき、Cloud NAT は不要です。プライベートアドレス上のバックエンドに対してアプリが別のネットワークにある、またはネットワークがないと、プランが失敗します。先に Service Networking と Network Connectivity の API、そして Compute Engine の API を有効にしてください。ネットワークはインスタンスのコールドスタートを長くします。[プライベートネットワークが加えるもの](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds)を読んでください。

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## ジョブをワーカーに分ける

ある種類のジョブがほかを足止めしているときは、その種類に専用のサービスを与えます。何を分けるかは、[ジョブの種類ごとにワーカーを分ける](running.md#splitting-workers-by-job-type)にあるとおり、メトリクスから決めます。1 つの値をキューとアプリに渡し、すべてのキュー、URL、ルート、設定はそのキーから作られるので、それらを自分で書くことはありません。

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

`queue` ブロックと `app` ブロックに `workers = local.workers` を設定します。一緒にしておくべき種類を分けた場合や、キューとアプリに異なる値を与えた場合は、プランが拒否します。また、すべてのプロセスに届くブローカーが必要で、`stream-gcp-pubsub` はそれに当たります。`monitoring-gcp` を使うなら、同じ値をそれにも渡します。

そのページにある 2 つのよくある構成は次のようになります。配送がほかを遅らせないようにするには、上の値を使います。メールと通知を速く保つには次のようにします。

```hcl
locals {
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
}
```

そのページにあるとおり、設定していない機能の種類は除いてください。サーバーが登録していない種類があると起動が止まり、プランからはそれがどれかがわかりません。`workers` を使うと、これらの種類のタスクは専用のキューに送られるので、既定のサービスが空いていてもこれらの種類を手伝うことはありません。

すべてのフィールドと、ワーカーがアプリから何を受け継ぐかは[契約](../../deploy/opentofu/README.md#workers)にあります。ワーカーのサービスは外部からのトラフィックを受けません。`upgrade.sh` は、マイグレーションのあと、既定のサービスの前に各ワーカーを更新し、そのために何かを指示する必要はありません。止まるのは手順が失敗したときだけです。ワーカーはそれぞれサービスの分の費用がかかることがあり、そのインスタンスの接続はデータベースの上限に数えられます。

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## トラフィックがサービスに届く経路

これは `main.tf` の `ingress` ブロックで決まります。例では本番向けのロードバランサーを使います。その隣のコメントには、デプロイを安く試すためのドメインマッピングがあります。差し替えるには、そのブロックと `app` ブロックの `service_ingress` を変えます。2 つが食い違っていればプランが知らせます。

| | `ingress-gcp-load-balancer`（例） | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| 月額料金 | 転送ルールにおよそ $18、加えて GB ごとの料金。[料金のページ](https://cloud.google.com/vpc/network-pricing)を確認してください。この金額はそこでは確認できませんでした | なし |
| 位置づけ | Google Cloud が推奨する方法 | レイテンシのため Google Cloud が本番には推奨していないプレビューの機能 |
| リージョン | どこでも | `asia-northeast1`、`us-central1` ほか少数 |
| DNS | `tofu output dns_records` に出る `A` レコード | 同じ出力が一覧にするレコード |
| 証明書 | Google マネージド。DNS がアドレスを指すと有効になります | Google マネージド。最長 24 時間 |
| ドメインの所有権 | DNS のほかには何も不要 | 適用する前に、あなたと、適用するほかのすべてのアカウントが[確認済みの所有者](https://cloud.google.com/run/docs/mapping-custom-domains)である必要があります |
| `run.app` の URL | インターネットからは拒否されるので、誰もロードバランサーを迂回できません | 誰でも到達できます。ドメインマッピングはヘッダーを加えられないので、オリジンは開いたままです |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` とロードバランサーのアドレス | `169.254.0.0/16` |
| `app` ブロックの `service_ingress` | `internal-and-load-balancer`。`ingress` ブロックと食い違うとプランが失敗します | `all` |

Cloud Tasks と Cloud Scheduler はどちらも通らず、既定の `run.app` の URL を直接呼び出します。

サービス自身の URL を使う場合や、CDN などほかのものを前に置く場合は、`ingress` ブロックと `backends` にあるその行を削除し、`service_ingress` は必要な値のままにしておきます。これは常にあなたが明示的に選ぶもので、`all` にすると `run.app` の URL が誰にでも開かれます。`env` の `MY_TRUSTED_PROXIES` には、サーバーに接続してくるものを設定します。前に何を置いても、`https://{host}` で応答しなければなりません。

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` は、すべてのキャッシュをバイパスして `host` の前に Cloudflare を置きます。`ingress-cloudflare-media` は、どの ingress とも並べて、キャッシュするメディア用のホストを追加します。先に各モジュールの README を読んでください（[app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md)、[media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)）。トークンは `CLOUDFLARE_API_TOKEN` に入れます。これらのモジュールは、モックのプロバイダーでしか動かしたことがありません。

- **プランによってオリジンが決まります。** Cloud Run は `Host` でルーティングし、Cloudflare が Host ヘッダーの上書きを提供しているのは Enterprise プランだけです。それ以外のプランでは `*.run.app` のオリジンでプランが失敗するので、オリジン自身があなたのホストを受け付ける必要があります。そのホストの `ingress-gcp-domain-mapping` を `backends` の外に置いておき、`origin_host = "ghs.googlehosted.com"` を設定します。ホストごとに専用のマッピングが必要です。プロキシの後ろでその証明書が発行・更新されるかは確認されていないので、`proxied = false` から始めてください。
- **ヘッダーでオリジンを閉じます。** `ingress-cloudflare-app` はリクエストヘッダーを変換するルールを加え、サーバーはそのヘッダーのないリクエストを `403` で拒否します。例外はヘルスチェックと、トークンを確認するジョブ自身の `POST` です。そのため、誰も `run.app` の URL を通って Cloudflare を迂回できず、呼び出し側が自分の Cloudflare Worker から、レート制限が見るクライアントのアドレスを選ぶこともできません。
- **値は状態に入ります。** Cloudflare のプロバイダーには、ルールのヘッダーの値を書き込み専用にする形がありません。`openssl rand -hex 32` で値を生成し、`gcloud secrets versions add` でシークレットに追加し、同じ値を `TF_VAR_origin_secret` で渡し、状態は非公開にしておきます。ローテーションの方法は [CDN のガイド](cdn.md#rotate-the-value)にあります。
- **レコードがプロキシされていない間は、何もヘッダーを加えない**ので、サーバーはヘルスチェック以外のすべてを拒否します。ドメインマッピングの証明書のために `proxied = false` にしているなら、シークレットはそのあとで設定してください。
- **ダッシュボードで Bot Fight Mode を無効にしてください。** これはゾーン全体にかかり、どのルールでも飛ばせません。そして `/ap/`、`/.well-known/`、`/api/` へのチャレンジは連合を壊します。ほかのサーバーはブラウザではないからです。ジョブは Cloudflare を通りません。モジュールはチャレンジを加えず、Browser Integrity Check を無効にしますが、Bot Fight Mode は管理しません。同じ理由で「I'm Under Attack」も有効にしないでください。
- `http://` をリダイレクトしたいなら、**ダッシュボードで Always Use HTTPS を有効にしてください。** これはゾーン全体にかかるので、モジュールはあなたに任せています。
- **タイムアウト**: オリジンが 125 秒間何も送らないと、Cloudflare は `524` を返します。ジョブは `run.app` の URL を直接呼び出すので影響を受けません。ストリームは 30 秒ごとにハートビートを送り、900 秒のアイドルの上限を下回ります。通常のプロキシされたホストでストリームがバッファリングされるかは確認されていません。
- **アップロード**: Free と Pro では 100 MB で、Cloud Run の 32 MiB より大きいので、上限は Cloud Run の 32 MiB のままです。
- **動画**: Cloudflare の規約では、Free、Pro、Business で動画や不釣り合いな量の大きなファイルを配信することが制限されており、Cloudflare がそのコンテンツをリダイレクトしたり、ほかの措置をとったりすることがあります。メディア用のホストで動画をキャッシュする前に、このことを判断してください。

**削除したファイルはキャッシュに残ります。** メディア用のホストはサーバーの `Cache-Control` に従うので、削除したローカルのファイルは最長 1 年、リモートのものは `remote_media_cache_ttl_seconds` の間、見え続けることがあります。サーバーはキャッシュをパージしません。何かを消さなければならないときは、ダッシュボードの **Caching > Configuration > Custom Purge** で手動でパージするか、次を実行します。

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

プロキシの URL はクエリ文字列に `sig` を含み、それもキャッシュされるものの一部なので、URL 全体を指定してください。

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## KEK のバックエンドを変える

`kek` ブロックの置き換えは `main.tf` の 1 つのブロックですが、データのあるインスタンスでは簡単にはいきません。データベースには現在の鍵で包んだシークレットがあり、その鍵を失ったインスタンスはそれを読めなくなります。そしてすべてのアカウントの鍵がその中に含まれます。

[ローカルの KEK と KMS の間を移行する](key-management.md#move-between-the-local-kek-and-a-kms)に従ってください。シークレットから Cloud KMS に移るときは、シークレットが移る間、現在の KEK を `kek_previous` として残しておきます。そのために、`secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }` で app モジュールを通してそのシークレットを渡します。[コンソール](#the-console)から `my kek rewrap` を実行し、`my kek status` が `local:` の鍵を数えなくなったら削除します。KEK は、再ラップより前のデータベースのバックアップを保管している間は残しておいてください。

データのない新しいインスタンスでは、何も移す必要はありません。

<!-- source: limits 5bd276dd3a89 -->
## 制限

- **アップロード**: Cloud Run は 32 MiB を超える HTTP/1 のリクエストを拒否し、サーバーは HTTP/1.1 を話します。そのため例では、大きなアップロードにサーバー自身の応答が返るよう、`MY_MEDIA_MAX_AV_BYTES` を既定の 100 MiB ではなく 30 MiB に設定しています。
- **ストリーム**: リクエストのタイムアウト（既定は 15 分）で開いているストリームが終わり、クライアントは接続し直します。
- **クライアントのアドレス**: レート制限は `MY_TRUSTED_PROXIES` に頼っており、app モジュールは `ingress` モジュールが返すものからそれを設定します。直接のリクエストに対して Cloud Run が `X-Forwarded-For` に何を加えるか、コンテナからどんな相手のアドレスが見えるかは、私たちが読んだどの文書にも書かれておらず、コンテナの契約にもどちらも書かれていないので、`169.254.0.0/16` は推測です。外部ロードバランサーについて文書化されている形式は `[<supplied>,]<client-ip>,<lb-ip>` です。最初のリクエストのログでアドレスを確認してください。クライアントの IP が `169.254` のアドレスや Google のアドレスなら、その設定はあなたのプロジェクトには合っていません。
- **公開アクセス**: サービスは `allUsers` に `roles/run.invoker` を付与しますが、ドメイン制限共有の組織ポリシーはこれを拒否します。そのようなプロジェクトには例外が必要です。

<!-- source: costs 1ab8d07f6847 -->
## 費用

サーバーにはお金がかかります。ここの金額は、何を見込めばよいか、どこで工夫すれば減らせるかの目安で、デプロイがこれに収まることを保証するものではありません。1 つのリージョンについての 2026-10 と 2026-11 時点のおおよその定価です。[Cloud Run](https://cloud.google.com/run/pricing) と [Neon](https://neon.com/pricing) の料金のページで確認してください。コンピュートは、請求先アカウントごとに月 180,000 vCPU 秒と 360,000 GiB 秒まで無料です。

| 部品 | アイドル時 | 増える要因 |
| --- | --- | --- |
| ロードバランサー | 月およそ $18（料金のページでは確認できていません） | GB ごとの料金。[トラフィックがサービスに届く経路](#how-traffic-reaches-the-service)を参照 |
| Neon の Launch プラン | 既定のティックで月およそ $19 | ストレージは GB 月あたり $0.35。代わりに `db-gcp-cloud-sql` を使うと、より小さなマシンで月およそ $9.40 の固定費になります。内訳はその README にあります |
| Cloud Run | $0。最小インスタンス数は 0 で、CPU はリクエストの間だけ課金されます | 開いているストリームはそれぞれインスタンスを課金状態に保ちます。下を参照 |
| Cloud Tasks | $0 | 100 万オペレーションまで無料、その後は 100 万あたり $0.40。ジョブ 1 件でおよそ 2 オペレーション |
| Cloud Scheduler | $0。請求先アカウントごとに 3 ジョブまで無料 | それを超えるとジョブあたり $0.10 |
| Cloud Storage | 月 GiB あたりおよそ $0.02 | 5 GiB の無料枠は `us-east1`、`us-west1`、`us-central1` だけ。メディアは Cloud Run を通って出ていきます |
| Pub/Sub | $0。10 GiB まで無料 | ストリームが開いている間だけ |
| Secret Manager | 数セント | 少数のバージョンは無料、その後はバージョンあたり数セント |

**ティックが Neon を起こし続けます。** ティックのたびにデータベースを読み書きし、Neon がコンピュートを止めるのは 5 分間アクティビティがなかったときだけなので、既定の `* * * * *` ではコンピュートが止まることはありません。月 730 時間、0.25 CU で 182.5 CU 時間となり、これが上の $19 です。ティックが毎分のままなのは、定期ジョブを時間どおりに実行し、失われた起動の合図をすぐに取り戻すためです。

**開いているストリームはインスタンスを課金状態に保ちます。** リクエストベースの課金では、インスタンスは何かのリクエストを処理している間課金され、リアルタイムの更新は 1 つの長いリクエストです。1 か月中ストリームを開いたままにするクライアントが 1 つあると、インスタンス 1 つ分が 24 時間かかります。無料枠を除いて、1 vCPU と 512 MiB で月およそ $60 です。ほかのストリームは `concurrency` まで同じインスタンスを共有します。ストリームがほとんどの時間開いているなら、インスタンスベースの課金や、小さな VM と [Docker Compose](deploy-docker-compose.md) の構成と比べてください。

<!-- source: where-care-reduces-it 5711f378aadb -->
### 工夫で減らせるところ

- ロードバランサーの代わりに**ドメインマッピング**を使うと、[上に書いたトレードオフ](#how-traffic-reaches-the-service)はありますが、$18 がなくなります。デプロイを試すのに向いています。
- **Neon の Free プラン**を使うと $19 がなくなり、小さなインスタンスならそれで動かせますが、次のトレードオフがあります。
  - 月 100 CU 時間までで、既定のティックではおよそ 17 日で使い切ります。`tick_schedule = "*/10 * * * *"` のように間隔を長くすれば枠内に収まり、ほかに何も起こさなければ、起きている時間は最大で半分、およそ 91 CU 時間になります。その場合、定期ジョブは最大 10 分遅れて実行され、失われた起動の合図を取り戻すのも遅くなります。
  - 連合するサーバーは 24 時間配送を受け取り、それもデータベースを起こすので、忙しいインスタンスはスケジュールにかかわらず Free プランでは足りなくなります。
  - プロジェクトあたり 1 GB と、復元に使える 6 時間の履歴があり、コンピュートはアイドルが 5 分続くと必ず止まるので、そのあとの最初のリクエストはその起動を待ちます。
