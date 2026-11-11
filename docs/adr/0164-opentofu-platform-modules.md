# ADR-0164: OpenTofu のプラットフォームごとの公開モジュール

- Status: Accepted
- Date: 2026-11-11
- Deciders: @rosylilly
- Issue: #2832

## Decision（決定）

v0.1.0 のタグの前に、`deploy/opentofu` の公開の単位を「役割ごとのモジュールを利用者が組み立てる」形から「プラットフォームごとに 1 つのモジュールを呼ぶ」形に変える。v0.1.0 の前なので、今の構成との互換と state の移し替えの手順は作らない（ADR-0023 の互換の約束は v0.1.0 から）。

見直す条件: 利用者の多くが 1 つのデプロイで複数のクラウドの部品を混ぜる（GCP のサービスに AWS の S3 など）と分かったとき。そのときは §5 の内部モジュールを公開に戻すことを考える。

### 1. 公開するモジュール

- 公開するのは `deploy/opentofu/mycroblog-gcp`（Cloud Run）と `deploy/opentofu/mycroblog-aws`（ECS Fargate）の 2 つだけにする。デプロイはどちらか 1 つのブロックを Git のソースと版の `ref` で呼ぶ。
- プラットフォームを変数（`platform = "gcp"`）で切り替える 1 つのモジュールにはしない。`source` は動的に決められないので、1 つのモジュールにすると両方のクラウドのプロバイダを常に宣言することになり（§6）、途中でプラットフォームを移る利用者もいないからである。
- VPS（Docker / microk8s）は OpenTofu の対象にしない。ADR-0010 §3 の Compose と Kustomize のままとする。

### 2. 入力の形: 役割ごとのセクションと `kind`

- 入力は役割ごとの object にする。`ingress`・`database`・`queue`・`media`・`kv`・`stream`・`mail`・`kek`・`network` の各セクションで、`kind` によって中身を選ぶ。
- どのセクションにも、プラットフォームの推奨構成を既定値として置く。何も書かなければ推奨構成になる。必須の入力は、どちらのプラットフォームでも `host`・`media.bucket_name`・`mail.from` と、それぞれのプラットフォームでしか決まらないものだけにする。GCP では SMTP の接続先、AWS では `network.egress` と `ingress.certificate_arn` がこれに当たる。
- 1 つの `kind` だけが使う項目は、同じセクションの optional な属性に置く。HCL には union 型がないので、`kind` と組み合わせの誤りは変数の `validation` で plan の前に止める。例: `stream.kind = valkey` なのに `kv.kind = db`、`kv.kind = memorystore` なのに VPC がない。
- 推奨する既定値:

| セクション | GCP | AWS |
| --- | --- | --- |
| `ingress.kind` | `load-balancer`（他に `domain-mapping`・`cloudflare`） | `load-balancer`（他に `cloudfront`・`cloudflare`。ALB は常に置く） |
| `database.kind` | `external`（Neon を手で作る。他に `cloud-sql`） | `external`（他に `rds`） |
| `queue.kind` | `cloud-tasks`（他に `db`） | なし（常に `db`。§4） |
| `media.kind` | `gcs`（他に `r2`・`s3`） | `s3`（他に `r2`・`s3-compatible`） |
| `kv.kind` | `db`（他に `memorystore`・`external`） | `db`（他に `elasticache`・`external`） |
| `stream.kind` | `pubsub`（他に `postgres`・`valkey`） | `sns`（他に `postgres`・`valkey`） |
| `mail` | SMTP だけ | `ses`（他に `smtp`） |
| `kek.kind` | `secret-manager`（他に `kms`） | `secrets-manager`（他に `kms`） |
| `network.kind` | `none`（他に `vpc`・`existing`） | `vpc`（他に `existing`） |

- Neon を OpenTofu で作る経路は公開モジュールに入れない（§6）。Neon は手で作り、`database.kind = external` で使う。
- AWS の `mail` の既定値は SES にする。新しいアカウントの SES は、AWS が本番の利用を認めるまで検証済みのアドレスにしか送れない。これは利用者向けの文書で補う。

### 3. ネットワーク

- `network` をセクションにし、`kind` で選ぶ。
  - GCP: `none`（既定。どのバックエンドも公開の端点を持つ構成）、`vpc`（作る）、`existing`（利用者の VPC につなぐ）。
    - `kv.kind = memorystore` は VPC を要し、`none` では validation で止める。
    - `database.kind = cloud-sql` は、VPC があれば private address、なければ connector のソケットでつなぐ。どちらになるかは `network.kind` から決め、別の項目を置かない。
    - `network.nat`（固定の送信元アドレス）は `vpc` のときだけ使える。
  - AWS: ECS は常に VPC を要するので、`vpc`（既定。作る）と `existing` だけにする。
    - 送信の経路 `network.egress = public-ip | nat` は必須で、既定値を置かない。どちらも時間で課金され、費用を利用者が選ぶべきだからである。

### 4. スケールの口

公開モジュールにしても、scale to zero と、ワーカーだけを増やすことは利用者が選べるようにする。facade が固定するのは、どのモジュールを組み合わせるかと配線だけで、量は利用者が決める。

- **Web のサービス**: `service` セクションに、インスタンスの数と大きさを置く。
  - GCP は `min_instances`・`max_instances`・`cpu`・`memory_mib`・`concurrency`・`cpu_always_allocated` を置き、既定は `min_instances = 0`（scale to zero）とする。
  - AWS は `desired_count`・`cpu`・`memory_mib` を置く。
- **ワーカー**: `workers` は今の契約（`modules/workers`）のまま、キーごとに `job_types` と量を持つ map にする。GCP ではワーカーごとに Cloud Run のサービスと Cloud Tasks のキューが 1 つでき、`min_instances`・`max_instances`・`concurrency`・`max_concurrent_dispatches`・`max_dispatches_per_second` を持てる。AWS ではワーカーごとに ECS のサービスができ、`instances`・`cpu`・`memory_mib`・`concurrency` を持てる。
  - 利用者はこれまで同じ値を app と queue（と monitoring）に渡していたが、facade が 1 か所から渡す。
  - これで、`service` は小さく scale to zero のまま、配送のワーカーだけを大きくできる。例: `workers = { delivery = { job_types = ["federation.*"], max_instances = 20 } }`。
- **scale to zero を壊す組み合わせは validation で止めるか、出力で知らせる。**
  - GCP の `queue.kind = db` はプロセスが起きている前提なので、`service.min_instances >= 1` と `cpu_always_allocated` を要する。満たさなければ plan を止める。
  - DB を起こし続けるものは止めずに、変数の説明と文書で費用を示す。対象は `queue.tick_schedule`（既定は毎分）と `monitoring.uptime_period`。Neon の自動停止の前提は deployment-targets の Google Cloud の節にある。
  - `kv.kind = memorystore` と `database.kind = cloud-sql` は常時課金になる。これも止めずに説明で示す。
- **AWS には scale to zero を置かない。** ECS の常駐プロセスがキューをポーリングし、ALB からのリクエストでタスクを起こす仕組みがないからである（要検証）。自動スケーリング（Application Auto Scaling）は今のモジュールにもなく、この ADR の範囲の外とする。

### 5. 内部モジュール

- 今の `deploy/opentofu/modules/*` は `deploy/opentofu/modules/internal/*` に移す。公開モジュールは相対パス（`../modules/internal/...`）で呼ぶ。同じ Git パッケージの中で解決されるので、利用者が固定する `ref` は 1 つで済む。
- 内部モジュールは利用者向けの文書で案内せず、互換を約束しない。Git のパスを書けば呼べてしまうが、それはサポートの外とする。
- バックエンドの契約（`backends` の形、`tofu:backend-fields`）は内部の契約として残す。公開モジュールの中の組み立ては次の 3 段にする。
  1. `locals` で入力を判断に変える（どの `kind` か、名前、VPC が要るか、`service_ingress`）。
  2. `kind` ごとに module ブロックを 1 つずつ書き、`lifecycle { enabled = ... }` で役割ごとに 1 つだけを有効にする。無効にした module への参照は null になる。ただし無効でも入力の検査（`nullable = false` と validation）は走るので、使わないブロックには形の正しい仮の値を渡す。
  3. 有効なブロックだけを集めて、app の `backends` に渡す。
- サービスの名前は `local` で先に決め、ingress と monitoring に渡す。こうすると ingress が app に依存せず、今の example で利用者が手で解いていた循環参照（ingress が app のサービス名を読む）がなくなる。`service_ingress` も `ingress.kind` から決める。

### 6. プロバイダ

- `google` と `aws` は利用者がルートで設定する（project・region）。公開モジュールはその値を使う。GCP の `project_id` と `region` は、省略するとプロバイダの値を使う。
- `random` と `archive` は設定が要らないので、公開モジュールが宣言する。
- `cloudflare` は公開モジュールが宣言する。Cloudflare の `kind` を選ばなければ、どのリソースもこのプロバイダを使わず、`CLOUDFLARE_API_TOKEN` がなくても plan が通る。
  - 2026-11-11 に OpenTofu 1.12.6 と cloudflare 5.x で確かめた。`lifecycle.enabled = false` のリソースだけを持つモジュールを、トークンなしで plan した。
- `neon` は宣言しない。同じ日に neon 0.18.0 で確かめたところ、`count = 0` のリソースしかなくても、API key を要求して plan が失敗した。公開モジュールに入れると、Neon を使わない利用者全員に key を求めることになる。
- プロバイダの alias（`configuration_aliases`）は使わない。使うと、全利用者に `providers = {...}` を書かせることになる。CloudFront の us-east-1 の証明書は、作らずに ARN で受け取る。

### 7. 生成と、利用者が入れる secret

- 公開モジュールの中で生成できるものは、すべて生成する。
  - KEK、VAPID 鍵: 今と同じく ephemeral resource と write-only 属性を使い、plan と state に残さない。
  - origin secret（Cloudflare・CloudFront）: `random_password` で作る。Cloudflare のルールと CloudFront のヘッダーが値を持つので、生成しなくても state には残る。生成すれば手順が 1 つ減る。
  - Cloud SQL・RDS のパスワード: 今のモジュールのとおり。
- secret の名前は `name`（既定 `my`）から決める（`<name>-kek`、`<name>-database-url` など）。名前を個別に上書きする入力は置かない。
- 利用者しか持たない値もある。外部 DB の URL、SMTP のパスワード、R2・S3 互換ストアのアクセスキー、外部 Valkey の URL と、§11 の `secrets` で名前を挙げた secret である。公開モジュールはこれらの secret を空で作り、出力 `secrets_to_fill` で「何を入れるか」と、それを入れる `gcloud` / `aws` のコマンドを示す。値を変数で受け取って書く経路は置かない。ephemeral な変数では、書くたびに毎回その値を渡す必要があり、手順がかえって増えるからである。

### 8. 出力

- 利用者が apply の後に読むものは、公開モジュールの出力にまとめる。
  - `secrets_to_fill`
  - `dns_records`: ingress・メディアのホスト・SES の検証のレコード
  - `kek`: KEK の場所と、控えを取るコマンド
  - `console`（`console.sh` が読む）
  - `version`: 今のサーバーの版と、最後にマイグレーションを実行した版
- `console.sh` は、`.terraform/modules/<ブロック名>/deploy/opentofu/` から公開モジュールの出力を読む。アップグレードは §9 のとおり apply で行い、`upgrade.sh` は公開の手順から外す。

### 9. アップグレードを apply で行う

**版を変えて apply すれば、マイグレーションからサービスの差し替えまでが、`upgrade.sh` と同じ順序で実行される。** 版の差分は plan に出る。今の `ignore_changes`（GCP の image）と SSM のパラメータ（AWS）による「apply は image を変えない」形はやめる。

- **入力**: `image` を `image = { repository, version, digest }` に分ける。
  - `repository` の既定値は `ghcr.io/mycroblog/my`。
  - `version` の既定値は **公開モジュール自身の版** にする。リリースのワークフローが、タグの時点で公開モジュールに版を書き込む。こうすると、利用者は `source` の `?ref=v0.2.0` を上げるだけで、モジュールとイメージが同じ版に揃って上がる。README の「すべてのブロックを同じ版に」という約束が、利用者の作業ではなくなる。
  - `version` を明示するのは、rc を試すときと、モジュールだけを先に上げるときである。`digest` は `nightly@sha256:...` のように digest で固定するときに使う。
- **順序**: apply の依存で、`upgrade.sh` と同じ順序にする。
  1. マイグレーションのジョブが新しい版で `my upgrade` を実行し、成功を待つ。`my upgrade` は `config check`・`db upgrade -check`・`schema apply`・`db upgrade` を 1 つのプロセスで順に実行する、新しいサブコマンドである。
  2. ワーカーのサービスが新しい版になる。
  3. 既定のサービスが新しい版になる。
  - どれかが失敗すると apply はそこで止まり、まだ動いていないものは前の版のままになる。もう一度 apply すれば、終わっていない段から続く。
- **マイグレーションの実行**: 両方のプラットフォームで同じ形にする。`terraform_data`（`triggers_replace` に image）の `local-exec` で、同じ版の `deploy/opentofu` にあるスクリプトを実行する。スクリプトはマイグレーションのジョブ（GCP は Cloud Run のジョブ、AWS は ECS の一回限りのタスク）で `my upgrade` を実行し、終わるまで待ち、失敗すれば 0 以外で終わる。
  - 失敗すると `terraform_data` は tainted になり、その後のリソースは変わらない。同じ版でもう一度 apply すると、tainted のリソースが置き換えられて、マイグレーションがもう一度実行される。やり直しのための入力は要らない。
  - 成功した後は、同じ版の apply は何もしない。
  - apply を中断したり、タイムアウトしたりしても、プラットフォームの実行は止まらずに走り続ける。そのためスクリプトは、同じジョブで実行中のものがあれば、その終わりを待ってから始める。マイグレーションは一度に 1 つしか走らない。
  - サービスは `terraform_data` に依存し、新しいリビジョン（GCP）や新しいタスク定義（AWS の `wait_for_steady_state` と circuit breaker の自動ロールバック）が ready になるまでプロバイダが待つ。
  - apply する環境に、GCP は gcloud、AWS は aws CLI と jq と、その認証が要る。CI から apply する利用者は、その環境にも用意する。
  - 最初のデプロイでも、マイグレーションが走ってからサービスができる。`upgrade.sh --migrate-only` は要らなくなる。
  - AWS は未検証のままである（§1）。
- **`allow_destructive`**: `upgrade.allow_destructive_for = "0.3.0"` のように、版を名指しして許す。名指しした版へ上げる apply だけで効くので、宣言的な設定に残っても次のアップグレードで破壊的な変更を黙って通さない。
- **やり直し**: マイグレーションが失敗したら、原因を直して同じ版で apply する。
- **下げる**: 版を下げた apply では、古い版の `db upgrade -check` が新しいスキーマを拒む。マイグレーションが失敗し、サービスには触らない。版を下げる手順は今と同じく、バックアップからの復元である。
- **メンテナンスモード**: apply の外に残す。前後で 2 回サービスを変える必要があり、1 回の apply では表せないからである。リリースノートがメンテナンスモードを求める版は、`env` に `MY_MAINTENANCE = "true"` を足して apply し、版を上げて apply し、外して apply する、という 3 回の apply を文書に書く。
- **console**: ジョブの image も同じ版に揃う。実行は今と同じく `console.sh` で行う。

### 10. どの口をセクションにするか

サーバーの差し替え可能な口は §2 の 9 つより多い（翻訳、検索、音声、決済、captcha、メトリクス・トレース・エラー、メディア処理、各種のプラグインなど）。口ごとにセクションを足すと公開モジュールは際限なく大きくなるので、次の規則で決める。

- **セクションにするのは、その選択肢を選ぶと公開モジュールがプラットフォームに何かをしなければならない口だけ**にする。
  - 何かとは、リソースを作る、権限を与える、API を有効にする、ネットワークにつなぐ、値を生成する、のいずれかである。
  - 例: Google Cloud Translation と Amazon Translate（API の有効化と IAM の付与）は `translation`。Cloud Monitoring・Cloud Trace・Error Reporting、CloudWatch の EMF（権限と出力先）は `observability`。Amazon OpenSearch Service の IAM 署名は `search`。
  - 同じ口でも、何もしなくてよい選択肢はセクションに入れない。例えば DeepL は `translation` の `kind` にしない。
- **URL と secret を渡すだけで使える外部のサービスは、§11 の汎用の口で受ける。** DeepL、LibreTranslate、Stripe、PAY.JP、captcha、Sentry、LiveKit Cloud、Sora、外部の Elasticsearch・Meilisearch などである。アダプタが増えても公開モジュールを変えずに済む。
- **プラグインのサーバー自体のデプロイは範囲の外**とする。プラグインは別のサービスで、作り方も言語も利用者が決めるからである。プラグインのアドレスとトークンは、§11 の汎用の口で渡す。
- インスタンス設定（`translation_providers` など、データベースに持つ設定）は設定ではないので、OpenTofu は扱わない。

### 11. 汎用の口: `settings` と `secrets`

今の `env` / `secret_env` を、次の 2 つで置き換える。

- `settings`（`map(string)`）: 設定のキー（`deepl` ではなく `translator_deepl_url` のような設定の名前）から値への map。公開モジュールが `MY_` の環境変数にしてサービス・ワーカー・ジョブに渡す。
- `secrets`（`set(string)`）: secret の設定のキーの集合。公開モジュールは `<name>-<キー>` の secret を空で作り、サーバーに渡し、§7 の `secrets_to_fill` に載せる。利用者が secret を自分で作って ID を渡す必要はない。

どちらも、公開モジュールに同梱する設定の一覧（§12）を使い、plan の前に次を検査する。

- 設定にないキー（綴りの誤りや、リリースで名前が変わったキー）は拒む。
- secret の設定を `settings` に書くことは拒み、secret でない設定を `secrets` に書くことも拒む。
- セクションが決める設定（`database`、`queue` など）を汎用の口で上書きすることは拒む。

### 12. 検査と保証

公開モジュールが口を作り切っていることは、レビューではなく次の検査で保証する。

- **設定の一覧の生成**: `go:config-docs` は、設定のスキーマから `docs/reference/configuration.md` と `deploy/docker/example.env` を作っている。これに加えて、公開モジュールが読む設定の一覧（キー、`MY_` の名前、secret かどうか）を JSON で生成する。古いと `go:test` が落ちる。§11 の validation はこの JSON を読む。
- **分類の検査（`tofu:coverage`）**: プラットフォームごとに、設定のすべてのキーを次のどれかに分類したファイルを、`deploy/opentofu` に置く。
  - `section`: どのセクションが決めるか
  - `fixed`: 公開モジュールが固定の値を入れる（例: Cloud Run の `media_max_av_bytes`）
  - `generic`: §11 の口
  - `excluded`: 理由を付けて、扱わない
  - キーは接頭辞で束ねてよい（`plugin_*` は `generic` など）。表は短く保つ。
  - 検査は、分類のないキー、2 つに当たるキー、もう設定にないキーで落ちる。設定のスキーマが変わると `docs/reference/configuration.md` が変わり、そのパスで tofu の検査が選ばれる（`scripts/changed-contexts.sh`）。そのため、**アダプタを足す PR は、その PR の中で公開モジュールでの扱いを決めることになる。**
- **配線の検査**: `section` と `fixed` に分類したキーは、`tofu test`（モックのプロバイダ）で確かめる。セクションの `kind` ごとに plan し、サービスの環境変数にそのキーの `MY_` の名前が出ることを見る。今の `tofu:env-check`（存在しない `MY_` の名前を拒む）は残す。
- **利用者の plan での保証**: §11 の validation により、利用者の誤り（綴り、平文の secret、セクションとの二重指定）は apply の前に止まる。

これで、次の 3 つを保証する。

1. サーバーのどの設定も、公開モジュールで設定できるか、扱わない理由が書かれている。
2. セクションの選択肢は、実際にサーバーへ届いている。
3. 汎用の口に渡した設定は、サーバーに無視されない。

## 経緯

`deploy/opentofu` は、役割ごとのモジュール（`<role>-<provider>-<service>`、約 30 個）を公開し、利用者が example を写して組み立てる形で始まった。部品を差し替えられることを優先したためである（ADR-0002 のアダプタの差し替えを、IaC でも同じ形で見せる）。

しかし、運用してみて次の問題が分かった。

- 1 つのデプロイを作るのに、何を組み合わせるかが分かりにくい。
- example は、コメントで書いた「差し替えの手順」と配線の約束を利用者に読ませている。
  - ingress と app の循環参照を output ごとに手で解く
  - `service_ingress` を ingress と一致させる
  - `workers` を queue と app の両方に同じ値で渡す
  - secrets の `referenced` と `generate_p256_keys`
- 普通にインスタンスを立てる利用者は、1 つのデプロイで複数のクラウドを混ぜない。必要なのは「安いものに差し替えられる」程度の選択で、プラットフォームごとの推奨構成を既定にし、選択式で変えられれば足りる。

IaC のツールを Pulumi に変える案も考えた。これは採らなかった案に書く。問題はツールではなく公開の単位にある、と判断した。

v0.1.0 の前で、互換を保つ相手がいないので、今変える。

関連: NFR-PORT（移植性）、ADR-0010 §3（デプロイテンプレートと OpenTofu だけを使うこと）、ADR-0004 §23（キューの分割とワーカー）、[deployment-targets](../architecture/deployment-targets.md)（費用と scale to zero の前提）。

## 採らなかった案

- **今の構成のまま example を充実させる**: 部品を自由に組み合わせられ、モジュールの数だけ構成を作れる。採らなかったのは、配線の約束を利用者が読み解く必要が残り、利用者の構成が example からずれると `upgrade.sh` と文書のサポートが難しくなるからである。
- **`platform` を変数で切り替える 1 つのモジュール**: 入口が 1 つで済む。採らなかった理由は §1。両方のクラウドのプロバイダを常に宣言し、使わないプロバイダの取得と設定を全員に求めるからである。
- **Pulumi（Go）に移る**: サーバーと同じ言語で書け、Go の型（ジョブの種類など）を直接使える。条件分岐や合成も普通のコードで書け、union 型もない不便もない。採らなかったのは次の理由である。
  - 利用者が言語のツールチェーンと state のバックエンドを用意する必要がある。
  - 配る側にとって、コードの自由はサポートの範囲を広げる。
  - 標準の導線が Pulumi Cloud（プロプライエタリな SaaS）に寄っていて、ADR-0010 §3 の方針とぶつかる。
  - 今の OpenTofu の機能（`lifecycle.enabled`、ephemeral、write-only 属性）とテストを作り直す費用が大きい。
- **外から入れる secret の値を ephemeral な変数で受け取り、write-only 属性で書く**: state に残さずに、1 回の apply で secret まで揃う。採らなかったのは §7 のとおり、値が要る apply のたびに毎回渡す必要があるからである。
- **Neon の作成を公開モジュールに含める**: DB まで 1 回の apply で揃う。採らなかったのは §6 のとおりで、プロバイダが使わない利用者にも key を求め、パスワードが state に残り、プロバイダは上流で保守されていない。
- **R2 のアクセスキーを Cloudflare の API トークンとして公開モジュールで作る**: 利用者が埋める secret が 2 つ減る。この ADR では採らず、Open Questions に残す。トークンの値が state に残り、トークンを作る権限を OpenTofu の実行者に渡すことになるからである。
- **アップグレードを `upgrade.sh` に残す**: 順序もメンテナンスモードも 1 本のスクリプトで扱え、今動いている。採らなかったのは、apply とスクリプトの 2 つの道具で同じリソースを変えることになるからである。image を `ignore_changes` にするため、plan に版の差分が出ず、CI の apply とも噛み合わない。
- **GCP で、`google_cloud_run_v2_job` の `run_execution_token` を使う**: トークンを変えたジョブの作成・更新は実行を 1 回作り、その成功を待つ。スクリプトも gcloud も要らない。spike #2837（2026-11-11、google 7.46.2、OpenTofu 1.12.6）で、次のとおり採れないと分かった。
  - 実行が失敗しても、ジョブはトークンを受け取って ready になる。そのため、同じトークンの次の apply は何も実行せず、サービスを新しい版にする。
  - タイムアウトや中断の後も実行は走り続け、その最中の apply もサービスを動かす。
  - 一度使ったトークンは実行の名前が衝突して ready にならない。版から作ったトークンでは、版を下げてまた上げると詰まる。
  - プロバイダは実行の結果を返さないので、postcondition でも止められない。
- **AWS で、タスクを実行して待つ Lambda を `aws_lambda_invocation` で呼ぶ**: apply する環境に CLI が要らない。採らなかったのは、Lambda のコードと権限を保守することになり、15 分の上限があるからである。未検証の AWS には重い。
- **口ごとにセクションを足す**: 利用者はどの口も同じ書き方で選べる。採らなかったのは、URL と secret だけのアダプタにまでセクションを作ると、公開モジュールがサーバーのアダプタの数だけ大きくなり、アダプタを足すたびに公開モジュールの変数を変えることになるからである（§10）。
- **`env` / `secret_env` を検査なしで残す**: 何でも渡せ、公開モジュールの保守がいらない。採らなかったのは、綴りの誤りや平文の secret を apply の前に止められず、どの口が扱われているかを誰も保証しないからである（§11・§12）。
- **scale の口を「プロファイル」（`small` / `large` など）にまとめる**: 選択がさらに簡単になる。採らなかったのは、プロファイルの名前が量を隠し、ワーカーだけを増やすような部分的な調整ができなくなるからである。

## Consequences（影響）

- 利用者のデプロイは、プロバイダのブロックと公開モジュールのブロック 1 つになる。必須の入力は数個で、配線は公開モジュールが持つ。
- `upgrade.sh`・`console.sh`・文書・`tofu test` の対象が、プラットフォームごとに 1 つの構成にまとまる。
- 内部モジュールの入力の名前は自由に変えられる。公開の互換は公開モジュールの変数だけになる。
- 悪い影響: 公開モジュールにない組み合わせ（GCP に SES など）は作れない。必要になれば `kind` を足すか、内部モジュールをサポートの外で使ってもらう。
- 悪い影響: 公開モジュールの変数が大きくなる。HCL に union 型がないため、`kind` ごとの項目が同じ object に並び、validation が増える。
- 悪い影響: 選ばなかった `kind` のブロックも module として書かれるので、`tofu init` は使わない内部モジュールも取得する。同じ Git パッケージなので、余分な取得はない。
- 悪い影響: AWS では scale to zero を選べない（§4）。
- 版を変えた apply がそのままアップグレードになる。plan に版の差分が出て、CI からの apply でもアップグレードできる（§9）。
- 悪い影響: 版を上げる apply は、マイグレーションの時間だけ長くなる。apply を中断すると、ジョブの実行は続く。
- サーバーにアダプタを足すと、同じ PR で公開モジュールでの扱い（セクション、汎用の口、扱わない）を決めることになる（§12）。デプロイの対応が後から漏れることはない。代わりに、アダプタを足す PR の手間が少し増える。
- 悪い影響: プラグインのサーバーは、利用者が別にデプロイする必要がある（§10）。
- 悪い影響: アップグレードは `local-exec` に頼り、apply する環境に gcloud（GCP）、aws CLI と jq（AWS）とその認証が要る。
- ADR-0010 §3 の表の Google Cloud と AWS の行は、公開モジュールを指す。
- 後続の作業（Issue にする）:
  - 公開モジュール `mycroblog-gcp` の実装と `tofu test`。内部モジュールの `modules/internal` への移動。
  - `mycroblog-aws` の実装。AWS は今も apply していないので、未検証のままであることも明記する。
  - `examples/` を公開モジュールを 1 回呼ぶだけの形に書き換え、`deploy-cloud-run.md` と `deploy/opentofu/README.md` を書き直す。
  - `console.sh` が公開モジュールの出力を読むようにする。
  - `my upgrade`（§9）をサーバーに足す。
  - リリースのワークフローで、公開モジュールに版を書き込む（§9）。
  - アップグレードの文書を apply の手順に書き直し、`upgrade.sh` を外す。
  - 設定の一覧の JSON の生成、分類のファイル、`tofu:coverage`、§11 の validation（§12）。
  - セクション `translation`・`observability`・`search` の内部モジュール（§10）。
  - SES の sandbox と本番利用の申請についての文書。
  - `scripts/tofu-examples.sh` などの検査を、公開モジュールの相対ソースを許すように直す（example の相対ソースの禁止は残す）。

## Open Questions / 要検証

- `kv-gcp-memorystore`・`kv-aws-elasticache` が Valkey の URL を secret に書くかどうか。`stream.kind = valkey` の配線がこれに依存する。
- ECS で、リクエストを受けてタスクを 0 から起こす仕組みがないこと（§4、要検証）。あれば AWS にも scale to zero の口を足すかを考える。
- R2 のアクセスキーを公開モジュールで生成するか（採らなかった案を参照）。
- AWS の自動スケーリング（Application Auto Scaling）を `service` と `workers` の口に足すか。
