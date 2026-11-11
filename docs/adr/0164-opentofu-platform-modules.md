# ADR-0164: OpenTofu のプラットフォームごとの公開モジュール

- Status: Proposed
- Date: 2026-11-11
- Deciders: @rosylilly
- Issue: #2832

## Decision（決定）

推奨案。v0.1.0 のタグの前に、`deploy/opentofu` の公開の単位を「役割ごとのモジュールを利用者が組み立てる」形から「プラットフォームごとに 1 つのモジュールを呼ぶ」形に変える。v0.1.0 の前なので、今の構成との互換と state の移し替えの手順は作らない（ADR-0023 の互換の約束は v0.1.0 から）。

覆る条件: 利用者の多くが 1 つのデプロイで複数のクラウドの部品を混ぜる（GCP のサービスに AWS の S3 など）と分かったとき。そのときは §5 の内部モジュールを公開に戻すことを考える。

### 1. 公開するモジュール

- 公開するのは `deploy/opentofu/mycroblog-gcp`（Cloud Run）と `deploy/opentofu/mycroblog-aws`（ECS Fargate）の 2 つだけにする。デプロイはどちらか 1 つのブロックを Git のソースと版の `ref` で呼ぶ。
- プラットフォームを変数（`platform = "gcp"`）で切り替える 1 つのモジュールにはしない。`source` は動的に決められないので、1 つのモジュールにすると両方のクラウドのプロバイダを常に宣言することになり（§6）、途中でプラットフォームを移る利用者もいないからである。
- VPS（Docker / microk8s）は OpenTofu の対象にしない。ADR-0010 §3 の Compose と Kustomize のままとする。

### 2. 入力の形: 役割ごとのセクションと `kind`

- 入力は役割ごとの object にする。`ingress`・`database`・`queue`・`media`・`kv`・`stream`・`mail`・`kek`・`network` の各セクションで、`kind` によって中身を選ぶ。
- どのセクションにも、プラットフォームの推奨構成を既定値として置く。何も書かなければ推奨構成になる。必須の入力は、どちらのプラットフォームでも `host`・`image`・`media.bucket_name`・`mail.from` と、それぞれのプラットフォームでしか決まらないものだけにする。GCP では SMTP の接続先、AWS では `network.egress` と `ingress.certificate_arn` がこれに当たる。
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
  2. `kind` ごとに module ブロックを 1 つずつ書き、`lifecycle { enabled = ... }` で役割ごとに 1 つだけを有効にする。
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
- 利用者しか持たない値もある。外部 DB の URL、SMTP のパスワード、R2・S3 互換ストアのアクセスキー、外部 Valkey の URL である。公開モジュールはこれらの secret を空で作り、出力 `secrets_to_fill` で「何を入れるか」と、それを入れる `gcloud` / `aws` のコマンドを示す。値を変数で受け取って書く経路は置かない。ephemeral な変数では、書くたびに毎回その値を渡す必要があり、手順がかえって増えるからである。

### 8. 出力

- 利用者が apply の後に読むものは、公開モジュールの出力にまとめる。
  - `secrets_to_fill`
  - `dns_records`: ingress・メディアのホスト・SES の検証のレコード
  - `kek`: KEK の場所と、控えを取るコマンド
  - `console`（`console.sh` が読む）
  - `upgrade`（`upgrade.sh` が読む）
- `upgrade.sh` と `console.sh` は、`.terraform/modules/<ブロック名>/deploy/opentofu/` から公開モジュールの出力を読む。

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
- **scale の口を「プロファイル」（`small` / `large` など）にまとめる**: 選択がさらに簡単になる。採らなかったのは、プロファイルの名前が量を隠し、ワーカーだけを増やすような部分的な調整ができなくなるからである。

## Consequences（影響）

- 利用者のデプロイは、プロバイダのブロックと公開モジュールのブロック 1 つになる。必須の入力は数個で、配線は公開モジュールが持つ。
- `upgrade.sh`・`console.sh`・文書・`tofu test` の対象が、プラットフォームごとに 1 つの構成にまとまる。
- 内部モジュールの入力の名前は自由に変えられる。公開の互換は公開モジュールの変数だけになる。
- 悪い影響: 公開モジュールにない組み合わせ（GCP に SES など）は作れない。必要になれば `kind` を足すか、内部モジュールをサポートの外で使ってもらう。
- 悪い影響: 公開モジュールの変数が大きくなる。HCL に union 型がないため、`kind` ごとの項目が同じ object に並び、validation が増える。
- 悪い影響: 選ばなかった `kind` のブロックも module として書かれるので、`tofu init` は使わない内部モジュールも取得する。同じ Git パッケージなので、余分な取得はない。
- 悪い影響: AWS では scale to zero を選べない（§4）。
- ADR-0010 §3 の表の Google Cloud と AWS の行は、この ADR が Accepted になったら公開モジュールを指すように直す。
- 後続の作業（Issue にする）:
  - 公開モジュール `mycroblog-gcp` の実装と `tofu test`。内部モジュールの `modules/internal` への移動。
  - `mycroblog-aws` の実装。AWS は今も apply していないので、未検証のままであることも明記する。
  - `examples/` を公開モジュールを 1 回呼ぶだけの形に書き換え、`deploy-cloud-run.md` と `deploy/opentofu/README.md` を書き直す。
  - `upgrade.sh`・`console.sh` が公開モジュールの出力を読むようにする。
  - SES の sandbox と本番利用の申請についての文書。
  - `scripts/tofu-examples.sh` などの検査を、公開モジュールの相対ソースを許すように直す（example の相対ソースの禁止は残す）。

## Open Questions / 要検証

- `lifecycle.enabled = false` の module を参照したときに null になり、`[for b in [...] : b if b != null]` で除けること。実装の最初の `tofu test` で確かめる。
- `kv-gcp-memorystore`・`kv-aws-elasticache` が Valkey の URL を secret に書くかどうか。`stream.kind = valkey` の配線がこれに依存する。
- ECS で、リクエストを受けてタスクを 0 から起こす仕組みがないこと（§4、要検証）。あれば AWS にも scale to zero の口を足すかを考える。
- R2 のアクセスキーを公開モジュールで生成するか（採らなかった案を参照）。
- AWS の自動スケーリング（Application Auto Scaling）を `service` と `workers` の口に足すか。
