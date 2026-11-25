<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# OpenTofu でのデプロイ

OpenTofu のルートモジュールを使うすべてのデプロイに共通することを説明します。[Google Cloud へのデプロイ](deploy-google-cloud.md)と [AWS へのデプロイ](deploy-aws.md)が、それぞれのプラットフォームに必要なことを補います。どちらのモジュールにも、[OpenTofu](https://opentofu.org) 1.11 以降が必要です。Terraform には対応していません。

<!-- source: the-root-modules 4e0bf528bd0e -->
## ルートモジュール

ルートモジュールの 1 つのブロックで、サービス、そのジョブ、役割ごとのバックエンドを含む、サーバー全体をデプロイします。

| ルートモジュール | プラットフォーム | コピーする例 |
| --- | --- | --- |
| `mycroblog-gcp` | Google Cloud の Cloud Run | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | AWS の ECS on Fargate。まだ一度も適用されたことがありません | `examples/aws-ecs-neon` |

例は、プラットフォームのプロバイダーと、1 つのブロック `module "mycroblog"` でできています。そのソースは、リリースに固定した Git のソースです。

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

固定は必須です。固定しないモジュールは、あなたの知らないうちに KEK のリソースを変えてしまうかもしれません。サーバーのイメージは、`ref` が指すリリースを既定にするので、`ref` だけで動くものが決まり、それを上げることがアップグレードです。モジュールは、それを含む最初のリリースから存在します。

<!-- source: sections 728259c0b297 -->
### セクション

役割はそれぞれブロックのセクションで、`kind` がバックエンドを選ぶオブジェクトです。たとえば `database = { kind = "external" }` です。どのフィールドにも推奨の構成が既定値としてあるので、変えるものだけを書きます。動かない組み合わせは、計画の前に、入力を名指しするメッセージで失敗します。各モジュールのセクション、種類、フィールドは[モジュールのリファレンス](../../deploy/opentofu/README.md#sections)にあります。

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### そのほかの設定とシークレット

`settings` は、[設定リファレンス](../reference/configuration.md)にあるそのほかの設定を、キーを指定して受け取り、モジュールがそれをサービス、ワーカー、ジョブに渡します。`secrets` はシークレットの設定のキーを受け取ります。それぞれについて、モジュールは空のシークレット `<name>-<key>`（アンダースコアはハイフンにします）を作ってサーバーに渡し、埋めるものとして一覧にします。

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

セクションが設定するキー、モジュールが固定するキー、`listen` のようにモジュールが受け付けないキーは、計画が失敗し、エラーがその理由を伝えます。`translation` のように null のセクションや、`settings` にした `observability` のシグナルは何も設定しないので、`settings` でそのキーを指定できます。たとえば Sentry へのエラー報告は次のとおりです。

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

プラットフォーム上で手作業で変えた設定は続きません。次の適用で、ブロックに書かれたものに戻ります。

<!-- source: secrets d58c587dc144 -->
### シークレット

あなたが渡したシークレットの値が、OpenTofu の計画やステートに入ることはありません。モジュールは、書き込み専用の属性を通して、生成できるものを生成します。KEK、VAPID 鍵、モジュールが作るデータベースのパスワードです。唯一の例外は `ingress.kind = cloudflare` のオリジンシークレットで、Cloudflare のルールが持つので、ステートに入ります。そのステートは非公開に保ってください。

外部データベースの URL、SMTP のパスワード、アクセスキー、`secrets` のキーのように、あなただけが持つシークレットは、モジュールが空で作ります。`tofu output secrets_to_fill` が、それぞれに入れるものと、追加するコマンドを一覧にします。値のないシークレットがあるサーバーは起動しません。サーバーはシークレットをインスタンスやタスクの起動時に読むので、新しい値は次の起動から効きます。

**OpenTofu が KEK を生成するのは 1 度だけで、それきりです。** シークレットとそのバージョンには `prevent_destroy` が付き、新しい値を書くバージョンはモジュールの中で固定されています。KEK を新しいランダムな値にすると、古い KEK で暗号化されたものがすべて読めなくなるからです。ローテーションするには、[KEK のローテーション](key-management.md#rotate-the-kek)に従い、OpenTofu が追跡しないプラットフォームの CLI で新しい値を追加します。

<!-- source: first-deployment 9ae06fd14fc1 -->
## 最初のデプロイ

適用する場所には、`tofu` 1.11 以降、`jq`、プラットフォームの CLI（`gcloud` または `aws`）が必要です。適用はそこからマイグレーションを実行するからです。そのほかに、[`host`](../reference/configuration.md#host)のためのドメイン（あとから変えられません。[あとから変えられない設定](production-checklist.md#settings-you-cannot-change-later)を参照）と、[リリース](running.md#get-the-image)にある `my` のバージョンが必要です。`latest` タグはありません。最初に、データベースを作るなど、プラットフォームのページが求めることを済ませてください。

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. 例をコピーして変数を設定する

デプロイするリリースから、例をプロバイダーのロックファイルごと、空のディレクトリにコピーします。

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` の **`ref` をそのリリースにします**。安定版のリリースのコピーなら、すでに正しくなっています。プレリリースでは、`ref` とイメージの両方を設定します。イメージは先頭の `v` を除いたバージョンです。プレリリースのモジュールは、自分のイメージを既定にしないからです。`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf` を実行し、ブロックに `image = { version = "0.1.0-rc.4" }` を書きます。

`terraform.tfvars` を埋め、変えたいセクションを `main.tf` のブロックに足します。ハンドルに別のドメインを使うなら、[`account_domain`](../reference/configuration.md#account_domain)をここで決め、`settings = { account_domain = "example.org" }` と書いてください。

このディレクトリは残しておいてください。あなたの設定と、既定ではステートが入っています。ステートにシークレットの値は入りませんが、何が作られたかの唯一の記録です。複数の人が適用するなら、`gcs` や `s3` のバケットのようなリモートバックエンドに保管してください。

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. シークレットを作り値を追加する

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` は、そのリリースのモジュールとそのスクリプトを、ブロックの名前の下に取得します。適用が実行する `migrate.sh` と、[コンソール](#the-console)を実行する `console.sh` です。

最初に、シークレットだけを適用します。これでシークレットが作られ、KEK と VAPID 鍵が生成されます。KEK の種類のモジュールも一緒に指定します。

| ルートモジュール | 適用 |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

`kek = { kind = "kms" }` では、KEK のシークレットの代わりに `module.mycroblog.module.kek_kms` を指定します。

**今すぐ KEK をコピーし**、そのコピーはデータベースとは別に、たとえばパスワードマネージャーに保管してください。これがないと[バックアップ](backup.md)を復元できず、アカウントの鍵も失われます。コマンドは `tofu output kek` にあります。VAPID 鍵 `<name>-vapid-key` も同じようにコピーしてください。失うと、すべての Web Push の購読が終わります。

そのあと、`tofu output secrets_to_fill` の各コマンドを、値を付けて実行します。次の手順の前に、すべて追加してください。

<!-- source: 3-apply b9f715c7b55d -->
### 3. 適用する

```sh
tofu apply
```

これでマイグレーションのジョブが、依存するものとともに作られ、`migrate.sh` を通してその中で `my upgrade` が実行され、終わるのを待ちます。実行されるのは `my config check`、`my db upgrade -check`、`my schema apply`、`my db upgrade` で、最初に失敗したところで止まります。その出力は適用の出力に現れます。データベースを使う最初のコマンドが、そこに `host` と `account_domain` を記録します。サービスが作られるのはそのあとで、そのヘルスチェックはスキーマができるまで失敗します。

マイグレーションが失敗すると、適用はサービスの前で止まります。原因（多くは値のないシークレット）を直して、もう一度 `tofu apply` を実行してください。マイグレーションがもう一度実行されます。

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. ドメインをサーバーに向ける

`tofu output dns_records` が、DNS プロバイダーで追加するレコードを一覧にします。モジュールがレコードを作る場合は空です。

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. 最初の管理者を作る

コンソールにはターミナルも標準入力もないので、`my admin create` は、パスワードを読む代わりに、パスワードを設定するリンクを表示します。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

リンクはすぐに開いて、パスワードを設定してください。リンクは 1 時間のあいだ 1 回だけ使え、**それまでプラットフォームのログに残ります**。ログを読める人なら誰でも使えます。期限が切れたら、コンソールで `my admin account password-reset-link @alice` を実行してください。そのあと、2 要素認証をオンにします。

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. チェックリストを確認する

[本番運用のチェックリスト](production-checklist.md)を確認します。最初は[メール](production-checklist.md#email)で、`mail.from` のドメインの SPF、DKIM、DMARC です。

<!-- source: the-console 1b3d6dfa9c3c -->
## コンソール

コンソールは、サービスと同じイメージ、設定、シークレットで、`my` のコマンドを 1 つ実行します。存在するのは `console_enabled` がオンのあいだだけなので、終わったらオフにしてください。

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` はコマンドの終わりを待ち、プラットフォームのログから読み戻した出力を表示して、そのコマンドの終了コードで終わります。使うのはサービスが動かしているイメージです。マイグレーションで止まったアップグレードのあとでは、それは前のリリースで、データベースはまだそれに合っています。

コマンドには標準入力もターミナルもないので、パスワードやファイルをそこから読むコマンドは、この方法では実行できません。表示したものはログに残るので、ログに書きたくない出力を持つコマンドは実行しないでください。

<!-- source: upgrading 4be8a01734ce -->
## アップグレード

まず[アップグレード](database.md#upgrading)とリリースノートを読み、マイナーバージョンを 1 つずつ上げてください。アップグレードは、新しいリリースの適用です。

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

計画を読んでください。新しいリリースのモジュールは、リソースも変えるかもしれません。適用は次のことを行い、最初に失敗した手順で止まります。

1. マイグレーションとコンソールのジョブ（AWS ではタスク定義）に新しいイメージを与え、サービスが依存しない計画のそのほかの部分も適用します。
2. 適用する場所から `migrate.sh` を通して、マイグレーションのジョブで `my upgrade` を実行し、[最初のデプロイ](#3-apply)と同じように終わるのを待ちます。その前に、中断された適用が残した実行があれば、それが終わるのを待ちます。
3. 各ワーカーを新しいイメージに移し、そのあとサービスを移します。新しいリビジョンがトラフィックを受けるのは、そのヘルスチェックが通ってからです。

**適用する場所**には、`jq`、プラットフォームの CLI、ジョブを実行してそのログを読める認証情報が必要です。それはプラットフォームのページに一覧があります。適用する CI のジョブも同じです。

**手順が失敗したとき**は、サービスは自分のリビジョンのままで、その理由はリリースノートかマイグレーションの出力にあります。原因を直し、同じバージョンでもう一度 `tofu apply` を実行してください。マイグレーションがもう一度実行され、完了した手順はすることが残っていません。同じイメージの適用は何も実行しません。スキーマが変わったあとで戻るには、バックアップを復元します。[ロールバック](database.md#rolling-back)を参照してください。以前のバージョンの適用は、そのリリースが新しいスキーマを拒否するので、マイグレーションで止まり、どのサービスも変えません。

リリース候補を試すとき、または新しいモジュールのもとで以前のリリースのイメージを保つときは、バージョンを自分で設定します。リリースの `v` を除いた、イメージのタグです。

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### 先に SQL を確認する

計画には、マイグレーションの SQL が表示されません。適用の前にそれを読むには、マイグレーションのジョブだけを新しいイメージで適用し、その中で何も変えない `my upgrade -dry-run` を実行して、ログを読みます。コマンドはプラットフォームのページにあります。

<!-- source: destructive-changes 0bea88d60224 -->
### 破壊的な変更

リリースノートが `-allow-destructive` を求めているときは、そのバージョンをブロックに書きます。

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` に `-allow-destructive` が渡されるのは、そのバージョンの適用のときだけなので、値を残しておいても、あとのリリースがデータを削除することはありません。

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### ダイジェストでイメージを固定する

[本番運用のチェックリスト](production-checklist.md#operations)は、リリースノートにあるダイジェストの利用を勧めています。

```hcl
  image = { digest = "sha256:..." }
```

ダイジェストは、横のタグが何と言っていても 1 つのイメージを指すので、`ref` と同じ編集で変えてください。前のリリースのダイジェストを残すと、新しいバージョンのもとでも、そのリリースのイメージのままになります。

<!-- source: the-maintenance-mode 57bd5487b045 -->
### メンテナンスモード

[メンテナンスモード](maintenance.md)には 3 回の適用が要ります。サービスがアップグレードの前にも後にも変わるからです。ブロックの設定に追加して適用し、バックアップを取り、`ref` を上げて適用し、そのあと取り除いて適用します。

```hcl
  settings = { maintenance = "true" }
```

これを使わないと、マイグレーションのあいだも古いリビジョンが配信を続けますが、それが[動く保証はありません](database.md#the-server-during-an-upgrade)。

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## ジョブをワーカーに分ける

ある種類のジョブがほかを止めてしまうときは、それ専用のサービスを与えます。何を分けるかは、[ジョブの種類でワーカーを分ける](running.md#splitting-workers-by-job-type)にあるとおり、メトリクスから決めてください。`workers` は、各ワーカーを名指しするキーのマップを受け取り、それぞれに `job_types` を持ちます。分割に必要なキュー、ルート、設定はすべてモジュールがそこから作るので、自分では何も書きません。配信がほかを遅らせないようにするには、次のとおりです。

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

メールと通知を速く保つには、次のとおりです。

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
    }
  }
```

一緒にしておくべき種類を分けると、計画が拒否します。そのページにあるとおり、設定していない機能の種類は外してください。サーバーが登録していない種類があると起動が止まり、計画にはそれがどれか見えません。既定のサービスは、どのワーカーも名指ししない種類をすべて動かし、ワーカーが名指しした種類は動かしません。

ワーカーには、サービスのサイズも指定できます。名指ししないものは、既定のサービスと同じです。各プラットフォームのフィールドは[モジュールのリファレンス](../../deploy/opentofu/README.md#workers)にあります。ワーカーは外からのトラフィックを受けず、アップグレードではマイグレーションのあと、既定のサービスの前に移ります。ワーカーごとに、サービスと同じ費用がかかることがあり、そのインスタンスの接続はデータベースの上限に数えられます。

<!-- source: changing-a-backend df886f733f8e -->
## バックエンドを変える

そのセクションの `kind` を変えます。各種類が、サーバーに必要なものを設定し、計画が何が変わるかを示します。新しいデータベースは空なので、データベースを変えるときは、データも移す必要があります。古いデータベースの[バックアップ](backup.md)を取り、何も書き込まれていないあいだに、新しいデータベースへ復元してください。

<!-- source: the-kek 62356f591998 -->
### KEK

`kek.kind` を変えるのは 1 行ですが、データのあるインスタンスでは無料ではありません。データベースには現在の鍵で包まれたシークレットが入っていて、すべてのアカウントの鍵もその中にあるので、それを失ったインスタンスは読めなくなります。[ローカルの KEK と KMS の間で移す](key-management.md#move-between-the-local-kek-and-a-kms)に従ってください。シークレットから KMS へ移すときは、シークレットを移すあいだ、現在の KEK を `kek_previous` として残します。`secrets = ["kek_previous"]` を追加し、シークレット `<name>-kek-previous` に現在の KEK を入れます。[コンソール](#the-console)で `my kek rewrap` を実行し、`my kek status` が `local:` の鍵を数えなくなったら取り除きます。再ラップより前のデータベースのバックアップを持っているあいだは、KEK も保管しておいてください。

データのない新しいインスタンスでは、移すものはありません。
