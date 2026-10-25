# デプロイターゲットと低コスト構成

> 価格・制限値は **2026-10-16 時点の公式ドキュメント / 公式料金ページ** の値です。各数値の出典は節末の「出典」に URL で示します。
> 公式資料で確認できないものは **要検証** とし、理由を併記しています。料金は改定されるので、最終的には各社の公式ページを正としてください。
> 価格は特記がない限り USD・税抜・代表リージョン（GCP は us-central1、AWS は us-east-1）の値です。M6 で実測し、本ファイルを更新します。

## サマリ

| ターゲット | コンピュート | RDB | キュー | Blob | スケジューラ | 想定固定費 |
| --- | --- | --- | --- | --- | --- | --- |
| **Google Cloud** | Cloud Run（min instances 0） | 外部 Neon / Turso、または Cloud SQL | Cloud Tasks（push） | GCS（または R2） | Cloud Scheduler | ほぼ $0（Cloud SQL を使うと最小構成でも約 $9/月〜） |
| **AWS** | Lambda（Function URL）+ Lambda Web Adapter | Aurora Serverless v2（0 ACU）/ Aurora DSQL / RDS / 外部 | SQS（event source） | S3 + CloudFront | EventBridge Scheduler | ほぼ $0（DB 次第。Aurora は常時接続があると 0 ACU にならない） |
| **Self-hosted** | Docker Compose / microk8s | SQLite（+ Litestream）/ PostgreSQL | DB キュー | ローカル FS / MinIO / 外部 S3 互換 | プロセス内 cron | VM 代（$4〜6/月 クラス、要検証） |

## 設計に効く事実（要点）

1. **Aurora Serverless v2 の 0 ACU は「ユーザー接続が 1 本もない」ときだけ** 一時停止する。常駐ワーカーの DB ポーリングやコネクションプール、RDS Proxy があると停止しない。復帰は通常約 15 秒、24 時間超の停止後は 30 秒以上。
   → 連合サーバーは受信が途切れにくく、「ほぼ $0」になる前提は弱い。AWS の最安 DB 構成として Aurora DSQL / 外部 DB を併記する。
2. **Aurora DSQL は外部キーとシーケンスをサポートしている**。残る主な制約は、1 トランザクションあたり 3,000 行・10 MiB・5 分、DDL と DML の分離（1 トランザクション 1 DDL）、分離レベルは Repeatable Read 固定、楽観的同時実行制御（コミット時に SQLSTATE 40001）。
3. **Neon Free は 100 CU-hours/プロジェクト/月**。最小 0.25 CU で 400 時間/月（約 16.7 日）分で、常時起動（約 730 時間）には足りない。5 分無通信で scale to zero（Free では無効化不可）。
4. **Turso Free** は 100 DB・5 GB・月 5 億 rows read・月 1,000 万 rows written。上限超過のクエリは `BLOCKED` で失敗する（課金で継続されない）。なお料金ページに「Turso is joining Supabase」との告知があり、プラン体系が変わる可能性がある。
5. **SQS の遅延上限は 15 分、Cloud Tasks の予約上限は 30 日**。SQS のメッセージ上限は **1 MiB**。
6. **AWS Lambda Web Functions**（2026-10 発表の新形態。HTTP サーバーをそのまま動かせる）は現時点で **Node.js のみ** 対応で、Go サーバーには使えない。Go では引き続き Lambda Web Adapter（Event Functions）を使う。

## Cloudflare（実行環境としては対象外）

- [ADR-0001](../adr/0001-server-language-and-runtime.md) により、Cloudflare Workers（と D1 / Cloudflare Queues）は v0 の実行環境の対象外。
- ただし **R2（S3 互換 API）と CDN は他ターゲットと組み合わせて使える**。R2 は転送量（egress）課金がなく、メディア配信のコストを下げる候補。メディアの前に CDN を置くには `media_origin` を設定する（[メディア配信に CDN を置く](../operations/cdn.md)）。
- Cloudflare Containers 上での実行は将来の検討事項（需要があれば別 ADR）。

### R2（Blob 用途のみ）

| 項目 | 値 |
| --- | --- |
| 無料枠（Standard のみ） | 10 GB-month/月、Class A 100 万リクエスト/月、Class B 1,000 万リクエスト/月 |
| Standard ストレージ | $0.015 / GB-month |
| Class A（書き込み・List 等）/ Class B（読み取り） | $4.50 / 100 万、$0.36 / 100 万 |
| Egress（インターネットへの転送） | 無料（S3 API・r2.dev 経由とも） |
| S3 API のリージョン | `auto`（空文字と `us-east-1` は `auto` の別名） |
| 署名付き URL | AWS SigV4。有効期限 1 秒〜7 日。**S3 API ドメイン（`<ACCOUNT_ID>.r2.cloudflarestorage.com`）でのみ有効で、カスタムドメインでは使えない** |
| 非対応の S3 機能（抜粋） | ACL（`x-amz-acl` 等）、一部チェックサムヘッダ（`x-amz-checksum-algorithm` 等）は未実装 |

- 公開配信は public bucket + カスタムドメイン、非公開オブジェクトの一時共有は S3 API ドメインの署名付き URL、と使い分ける必要がある（`BlobStore.publicUrl` / `signedUrl` の実装に影響）。
- 要検証: aws-sdk-go-v2 の既定チェックサム動作と R2 の互換性（R2 のドキュメントは SDK ごとの設定例を載せているが、本プロジェクトの使い方で問題ないかは spike で確認する）。

出典（2026-10-16 確認）:
[R2 pricing](https://developers.cloudflare.com/r2/pricing/) /
[R2 S3 API compatibility](https://developers.cloudflare.com/r2/api/s3/api/) /
[R2 presigned URLs](https://developers.cloudflare.com/r2/api/s3/presigned-urls/)

## Google Cloud

- **構成**: Cloud Run サービス（Web）+ 同一イメージのワーカー用エンドポイント。Cloud Tasks が HTTP でワーカーを叩く（push 型）。
- **RDB**: Cloud SQL は最小構成でも常時課金のため、低コスト構成では Neon（Postgres）や Turso（libSQL）などの外部サーバーレス DB を推奨。
  Cloud Run + SQLite は永続ディスクがないため非推奨。
- **キュー**: 遅延実行・レート制御・タスク名による重複排除がある Cloud Tasks を第一候補。Pub/Sub は将来オプション。

### Cloud Run

| 項目 | リクエストベース課金 | インスタンスベース課金 |
| --- | --- | --- |
| 無料枠（請求先アカウント単位・毎月リセット、us-central1 の Tier 1 価格で割引として適用） | 180,000 vCPU 秒、360,000 GiB 秒、200 万リクエスト | 240,000 vCPU 秒、450,000 GiB 秒 |
| 課金単位 | 処理中のインスタンス時間（100 ms 単位で切り上げ） | インスタンスの起動から終了まで（最低 1 分） |
| アイドル時 | **min instances に指定したインスタンスのみ** アイドル料金がかかる。min instances 0 ならアイドル課金なし | インスタンスが存在する間は課金 |

- 本プロジェクトの既定はリクエストベース課金 + min instances 0（コールドスタートを許容）。
- 無料の外向き転送は北米発 1 GiB/月のみ。連合配送の外向き通信量は M6 で計測する。

### Cloud Tasks

| 項目 | 値 |
| --- | --- |
| 予約の最大先送り（遅延上限） | 現在時刻から **30 日** |
| 最大タスクサイズ | 1 MiB（バッチ投入は 10 MiB / 100 タスクまで） |
| タスクの最大保持期間 | 31 日 |
| タスク名による重複排除 | 削除済みタスクと同名の投入を **最大 24 時間** 拒否 |
| キューあたりの配信レート上限 | 500 タスク/秒（システム上限。超える場合はキューを分ける） |
| HTTP ターゲットの dispatch deadline | 既定 10 分、15 秒〜30 分 |
| 料金 | 課金オペレーション（API 呼び出しと push 配信試行。32 KB 単位でチャンク計数）月 100 万まで無料、以降 $0.40 / 100 万 |
| 認証 | タスクまたはキュー単位で OIDC（ID トークン）を付与できる。Cloud Run に対しては ID トークンを使い、サービスアカウントに Cloud Run Invoker ロールを付与する |

- 配送のリトライ 1 回が 1 オペレーションになるため、連合配送の失敗が多いとオペレーション数が増える（M6 で計測）。
- dispatch deadline の上限 30 分は Cloud Run のリクエストタイムアウトと揃えて設定する。

### Cloud Scheduler

- 課金はジョブ単位で $0.10 / ジョブ / 31 日（実行回数では課金されない。一時停止中のジョブも数える）。
- 無料枠は **請求先アカウントあたり 3 ジョブ/月**（プロジェクト単位ではない）。定期ジョブは 1 本のトリガーから複数種のジョブを enqueue する設計にして本数を抑える（ADR-0004 の方針どおり）。

### Cloud SQL（参考: 最小料金）

| 構成（us-central1） | 料金 |
| --- | --- |
| db-f1-micro（共有 vCPU、0.6 GiB RAM） | $0.0105 / 時間 ≒ 約 $7.7 / 月（730 時間換算） |
| db-g1-small（共有 vCPU、1.7 GiB RAM） | $0.035 / 時間 ≒ 約 $25.6 / 月 |
| SSD ストレージ | $0.000232877 / GiB 時間 ≒ 約 $0.17 / GiB 月 |

- 共有コア（db-f1-micro / db-g1-small）は Cloud SQL の SLA 対象外。インスタンスは起動している秒数で課金される。
- 例: db-f1-micro + SSD 10 GiB で約 $9.4/月。要検証: 最小ストレージ容量とアイドル時の IPv4 アドレス料金（料金表に「IPv4 addresses while idle $0.01/時間」があるが、どの条件で発生するかは本調査では確認しきれていない）。

### Cloud Storage（GCS）

| 項目 | 値 |
| --- | --- |
| 無料枠（Always Free） | 5 GB-month の Standard ストレージ（us-east1 / us-west1 / us-central1 のみ）、Class A 5,000 回、Class B 50,000 回、北米発の外向き転送 100 GB/月 |
| Standard ストレージ（us-central1） | $0.000027397 / GiB 時間 ≒ 約 $0.020 / GiB 月 |
| Class A / Class B（Standard、フラット名前空間） | $0.005 / 1,000 回、$0.0004 / 1,000 回 |
| インターネットへの転送（アジア・豪州以外宛、0〜10 TiB） | $0.12 / GiB（Cloud CDN 経由なら GCS 側の転送料は免除、キャッシュフィル料金は別） |
| S3 互換 | XML API（`https://storage.googleapis.com`）+ HMAC キーで S3 向けツール・ライブラリを利用できる |
| 署名 | `AWS4-HMAC-SHA256`（`x-amz-*` 拡張）と HMAC キーによる署名をサポート。署名付き URL は XML API エンドポイントでのみ有効で、有効期限は最大 604,800 秒（7 日） |
| HMAC キー | サービスアカウントあたり最大 10 個。作成後、使えるまで最大 60 秒 |

- 要検証: aws-sdk-go-v2 で生成した署名付き URL・マルチパートアップロード・チェックサムヘッダが GCS の XML API でそのまま動くか（公式ドキュメントは署名方式の互換を明記しているが、SDK の既定動作との組み合わせは spike で確認する）。

出典（2026-10-16 確認）:
[Cloud Run pricing](https://cloud.google.com/run/pricing) /
[Google Cloud Free Program の無料枠](https://docs.cloud.google.com/free/docs/free-cloud-features) /
[Cloud Tasks quotas and limits](https://docs.cloud.google.com/tasks/docs/quotas) /
[Cloud Tasks pricing](https://cloud.google.com/tasks/pricing) /
[Cloud Tasks: HTTP target tasks（認証）](https://docs.cloud.google.com/tasks/docs/creating-http-target-tasks) /
[Cloud Tasks REST: Task（dispatchDeadline）](https://docs.cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues.tasks) /
[Cloud Scheduler pricing](https://cloud.google.com/scheduler/pricing) /
[Cloud SQL pricing](https://cloud.google.com/sql/pricing) /
[Cloud Storage pricing](https://cloud.google.com/storage/pricing) /
[Cloud Storage interoperability](https://docs.cloud.google.com/storage/docs/interoperability) /
[Cloud Storage signatures](https://docs.cloud.google.com/storage/docs/authentication/signatures) /
[Cloud Storage signed URLs](https://docs.cloud.google.com/storage/docs/access-control/signed-urls) /
[HMAC keys](https://docs.cloud.google.com/storage/docs/authentication/hmackeys)

## AWS

- **構成**: Lambda（Web 用関数 + SQS コンシューマ関数、同一コード）。Lambda Web Adapter で通常の HTTP サーバーとして書ける。
- **RDB**: Aurora Serverless v2 は 0 ACU まで自動一時停止できるが、ユーザー接続が残っていると停止しない（下記）。Aurora DSQL は PostgreSQL 互換で外部キーも使えるが、
  トランザクションの行数・サイズ・時間に上限があるため、コントラクトテストで可否を判断する。
- **キュー**: SQS の遅延は最大 15 分。→ 再投入チェーンで長いバックオフを実現。

### Lambda

| 項目 | 値 |
| --- | --- |
| 無料枠（Event Functions） | 月 100 万リクエスト + 400,000 GB 秒（Provisioned Concurrency を有効にした関数には適用されない） |
| 料金（参考） | $0.20 / 100 万リクエスト、$0.0000166667 / GB 秒（x86、us-east-1 の第 1 段階） |
| 関数タイムアウト | 最大 900 秒（15 分） |
| 同期呼び出しのペイロード | リクエスト・レスポンスとも 6 MB（ストリーミング時は 6 MB 超の部分が 2 MBps に制限） |

- **Lambda Web Adapter**: Lambda Runtime API と HTTP をつなぐ Rust 製のアダプタで、Go を含むマネージドランタイム上の通常の HTTP サーバーをそのまま Lambda で動かせる。zip の場合はレイヤーとして追加し `AWS_LAMBDA_EXEC_WRAPPER=/opt/bootstrap` を設定する。Function URL では複数値ヘッダ（同名ヘッダの複数指定）を返せない点に注意（AWS Compute Blog）。
  - 要検証: アダプタ本体のリポジトリ（GitHub の awslabs）は本調査の出典範囲外のため、最新版の設定項目・バージョンは spike で確認する。
- **Lambda Web Functions**（2026-10 発表）: HTTP サーバーを直接動かす新形態。1 環境で複数リクエストを並行処理し、scale to zero する。ただし **現時点で Node.js（`nodejs24.x`）のみ**、提供リージョンは 5 つ（東京を含む）で、Go サーバーには使えない。対応言語が広がったら再検討する。
- **AWS Free Tier の変更**: 新規アカウントは「最大 $200 のクレジット（6 か月）」の Free plan / Paid plan 方式になり、30 以上のサービスの「Always free」枠は両プランで継続する。SQS の料金ページは「全顧客」に月 100 万リクエスト無料と明記している。
  - 要検証: Lambda の無料枠（上表）が Always free に含まれるか。Lambda の料金ページは無料枠を記載しているが、Free Tier ページの一覧（動的表示）からは確認できなかった。

### SQS

| 項目 | 値 |
| --- | --- |
| 遅延（delay queue / メッセージタイマー） | 0 秒〜 **最大 15 分** |
| メッセージサイズ | 最大 **1 MiB**（1,048,576 バイト）。1 リクエストに 1〜10 メッセージ、合計 1 MiB まで |
| メッセージ保持期間 | 最大 14 日 |
| 可視性タイムアウト | 既定 30 秒、最大 12 時間 |
| 料金 | 全顧客に月 100 万リクエストの無料枠。ペイロード 64 KB ごとに 1 リクエストとして課金 |

### EventBridge Scheduler

- 無料枠は月 1,400 万回の呼び出し、超過分は $1.00 / 100 万回。

### Aurora Serverless v2（0 ACU / 自動一時停止）

| 項目 | 値 |
| --- | --- |
| 最小容量 | 0 ACU（自動一時停止）。それ以前からのクラスタは最小 0.5 ACU のため、0 に変更が必要 |
| 一時停止までの無通信時間 | 300 秒（既定・最小）〜 86,400 秒 |
| 復帰時間 | 通常 **約 15 秒**。24 時間を超えて停止していると深いスリープに入り **30 秒以上** |
| 停止中の課金 | インスタンス料金は 0。ストレージ等クラスタ側の料金は継続 |
| 一時停止しない条件（抜粋） | ユーザー接続が 1 本でも開いている / RDS Proxy を併用 / 論理レプリケーション有効 / Global Database のプライマリ / zero-ETL 連携 |
| ACU 単価（参考） | Aurora Standard で $0.12 / ACU 時間（us-east-1、料金ページの計算例）。常時 0.5 ACU なら約 $44/月 |

- 連合サーバーは外部からの受信が断続的に続くため、**実運用で 0 ACU に落ちる時間は短い可能性が高い**。また復帰中の約 15 秒は Inbox 受信や API がタイムアウトしうる（接続タイムアウトを 15〜30 秒以上にする必要がある）。
- 新規アカウントの Free plan では Aurora PostgreSQL serverless を最大 4 ACU・1 GiB ストレージで使える（クレジット消費）。

### Aurora DSQL

| 項目 | 値 |
| --- | --- |
| 料金・無料枠 | DPU（分散処理単位）+ ストレージ（GB-month）。**月 100,000 DPU と 1 GB ストレージが無料**。アイドル時は DPU 課金なし |
| 外部キー | **サポート**（`NO ACTION` / `RESTRICT` / `CASCADE` / `SET NULL` / `SET DEFAULT`、遅延可能制約も可）。参照整合性の確認で追加の読み取りが発生する |
| シーケンス / identity 列 | サポート。`CACHE` の明示が必須で、値は `1` か `65536` 以上のみ。型は `bigint` |
| 分離レベル | PostgreSQL の Repeatable Read 固定（スナップショット分離） |
| 同時実行制御 | 楽観的（OCC）。競合はコミット時に SQLSTATE `40001`（OC000 データ競合 / OC001 スキーマ競合）。`FOR UPDATE` もロックではなくコミット時判定 |
| トランザクション上限 | 変更行数 3,000 行、変更データ 10 MiB、実行時間 5 分 |
| DDL | DDL と DML は別トランザクション、1 トランザクションに DDL は 1 文まで。インデックスは `CREATE INDEX ASYNC` |
| 非対応（抜粋） | 一時テーブル、`TRUNCATE`、PL/pgSQL（SQL 関数は可）、トリガー相当はアプリ側で実装、データベースは 1 クラスタ 1 つ（`postgres`）、照合順序は `C` のみ |
| その他の上限 | 接続は最長 60 分、テーブル 1,000、スキーマ 10、テーブルあたりインデックス 24、主キー / インデックスのキーサイズ 1 KiB、行サイズ 2 MiB |

- 要検証: `SELECT ... FOR UPDATE SKIP LOCKED` の可否（公式ページで明示を見つけられなかった）。不可なら DSQL 上で DB キュー（`queue/db`）は使えず、AWS では SQS を必須にする。
- 1 行に更新が集中するカウンタ（`SET n = n + 1`）は OCC の競合を起こしやすい。DSQL では非正規化カウンタを別テーブルに分散するか、定期再集計に寄せる。

出典（2026-10-16 確認）:
[AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/) /
[Lambda quotas](https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html) /
[Lambda Web Functions](https://docs.aws.amazon.com/lambda/latest/dg/web-functions.html) /
[Lambda Web Function runtimes](https://docs.aws.amazon.com/lambda/latest/dg/web-functions-runtimes.html) /
[What's New: Lambda Web Functions](https://aws.amazon.com/about-aws/whats-new/2026/10/aws-lambda-web-functions/) /
[AWS Compute Blog: Lambda Web Adapter と response streaming](https://aws.amazon.com/blogs/compute/using-response-streaming-with-aws-lambda-web-adapter-to-optimize-performance/) /
[AWS Free Tier](https://aws.amazon.com/free/) /
[SQS delay queues](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-delay-queues.html) /
[SQS message quotas](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/quotas-messages.html) /
[SQS pricing](https://aws.amazon.com/sqs/pricing/) /
[EventBridge pricing](https://aws.amazon.com/eventbridge/pricing/) /
[Aurora Serverless v2 auto-pause](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-serverless-v2-auto-pause.html) /
[Aurora pricing](https://aws.amazon.com/rds/aurora/pricing/) /
[Aurora DSQL pricing](https://aws.amazon.com/rds/aurora/dsql/pricing/) /
[Aurora DSQL: Migrating from PostgreSQL](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-migration-guide.html) /
[Aurora DSQL quotas](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html) /
[Aurora DSQL foreign keys](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-foreign-key-constraints.html) /
[Aurora DSQL CREATE SEQUENCE](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-sequence-syntax-support.html) /
[Aurora DSQL concurrency control](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-concurrency-control.html)

要検証（本調査で未確認）: S3 / CloudFront の料金・無料枠、Lambda Function URL の追加料金の有無、Lambda Web Functions の無料枠の有無。

## 外部サーバーレス DB

### Neon（PostgreSQL）

| 項目（Free プラン） | 値 |
| --- | --- |
| 月額 | $0（期限なし、カード不要） |
| プロジェクト | 100 |
| コンピュート | **100 CU-hours / プロジェクト / 月**（0.25 CU = 1 GB RAM なら 400 時間/月）、最大 2 CU まで自動スケール |
| Scale to zero | 5 分無通信で停止。**Free では無効化できない** |
| ストレージ | 1 GB / プロジェクト（全プロジェクト合計 20 GB まで） |
| 外向き転送 | 5 GB / プロジェクト / 月 |
| 有料（Launch） | $0.106 / CU 時間、$0.35 / GB-month、転送 500 GB/プロジェクト込み（超過 $0.10/GB） |

- 常時起動（約 730 時間/月）だと 0.25 CU でも 182.5 CU-hours になり Free 枠を超える。**5 分以上アクセスが途切れる時間がどれだけあるか** が Free で収まるかの分かれ目になる（M6 で計測）。
- 要検証: Free 枠を使い切った後の挙動（停止か、制限か）を公式ページで明示的に確認できなかった。

### Turso（libSQL）

| 項目（Free プラン） | 値 |
| --- | --- |
| 月額 | $0（カード不要） |
| データベース数 | 100 |
| ストレージ | 5 GB |
| 読み取り / 書き込み | 月 5 億 rows read / 月 1,000 万 rows written |
| Point-in-Time Restore | 1 日 |
| 上限超過時 | クォータのあるプランでは、超過したクエリは `BLOCKED` エラーで失敗する |
| 有料（Developer） | $4.99/月〜（年払い時の表示）、9 GB・25 億 rows read・2,500 万 rows written 込み |

- rows read は「返した行」ではなく **スキャンした行** で数える（集約関数・インデックスのない検索・JOIN で増える。`SELECT 1` でも 1 行）。中断したトランザクションの書き込みも rows written に数える。
  → タイムライン取得はインデックスで走査行数を抑え、カウンタは集計テーブルで持つ（ADR-0003 / ADR-0013 の方針と整合）。
- 料金ページに「Turso is joining Supabase」との告知がある。プラン体系・提供継続性は今後変わりうるため、Turso 依存の機能は libSQL / SQLite 互換の範囲に留める。
- 要検証: 料金ページの月払い価格（表示は年払い割引込み）、ドキュメントのプラン名（Starter / Scaler）と料金ページのプラン名（Free / Developer / Scaler）の対応。

出典（2026-10-16 確認）:
[Neon pricing](https://neon.com/pricing) /
[Neon plans](https://neon.com/docs/introduction/plans) /
[Turso pricing](https://turso.tech/pricing) /
[Turso usage & billing](https://docs.turso.tech/help/usage-and-billing)

## Self-hosted（Docker / microk8s）

- **最小構成**: アプリ 1 コンテナ（`role=all`）+ SQLite ファイル（ボリューム）+ ローカル FS。リバースプロキシ（Caddy 等）で TLS。
- **拡張構成**: PostgreSQL、`role=web` と `role=worker` を分離、MinIO または外部 S3 互換ストレージ。
- **microk8s**: Kustomize ベースのマニフェスト（または Helm chart）を `deploy/k8s` に提供。SQLite 構成時はレプリカ 1 固定。
- **バックアップ**: SQLite は Litestream で S3 互換へ継続レプリケーション、Postgres は `pg_dump` / マネージドのスナップショット。

## KV（任意の Valkey）

- 既定の `kv = db` は RDB のテーブルに置き、追加の常駐ミドルウェアを要らない（NFR-COST-2）。どのターゲットの既定の構成も、Valkey を前提にしない。
- レート制限やキャッシュの書き込みが DB の負荷になったら、`kv = valkey` で Valkey（BSD-3-Clause の Redis 互換のサーバー）か Redis 互換のサービスに移す。
  ADR-0003 と ADR-0019 の「Redis」は、このアダプタで満たす。接続は `valkey_url`（`valkeys://` で TLS、パスで DB 番号）、
  非公開の CA は `valkey_tls_ca_file`、キーの接頭辞は `valkey_key_prefix` で設定する（[設定のリファレンス](../reference/configuration.md)）。
- クラスタのモードのサーバーでも動く。KV のキーは 1 つのハッシュスロット（ハッシュタグ `{kv}`）に集まるので、KV をクラスタで分散はしない。
- 候補: Google Cloud は Memorystore for Valkey、AWS は ElastiCache for Valkey（サーバーレスを含む）、Self-hosted は `valkey/valkey` のコンテナ。
  どれも常時課金で、ほぼ $0 の構成には入らない（料金は要検証。M6 で計測する）。

## ストリーム（プロセス間の知らせ）

リアルタイム更新（`GET /api/v1/streaming`）の知らせをプロセスの間で運ぶブローカーは、設定 `stream` で選ぶ（[ADR-0050](../adr/0050-stream-pubsub-across-processes.md)、[overview §4.4](overview.md#44-リアルタイム更新sse)）。
設定は推測せず、既定は `hub`（プロセスの中だけ）。`role` が `all` 以外のプロセスが `hub` で起動すると、警告をログに出す。
どのブローカーも、ストリームを開いていないプロセスは送るだけで購読しない（scale to zero を壊さない）。

| ターゲット | `stream` | 設定 | 追加の固定費 |
| --- | --- | --- | --- |
| Self-hosted（SQLite、または 1 プロセスの `role = all`） | `hub` | なし | なし |
| Self-hosted（PostgreSQL で複数プロセス・レプリカ） | `postgres`（`db = postgres` のときだけ）。`kv = valkey` で Valkey を動かしているなら `valkey` も可 | `database_url` がトランザクションモードのプーラー（PgBouncer の transaction pooling、Neon のプール接続、Supabase の 6543 番）を通るときは、直接の接続先を `stream_postgres_url` に | なし（`LISTEN` の接続が 1 プロセスに 1 本） |
| Google Cloud（Cloud Run） | `pubsub` | `stream_pubsub_topic`（`projects/<p>/topics/<t>`）、`stream_pubsub_subscription_prefix` | なし（無料枠の中なら $0） |
| AWS（Lambda） | `sns` | `stream_sns_topic_arn`、`stream_sqs_queue_prefix`。認証とリージョン以外の設定は AWS SDK の既定の連鎖（実行ロール） | なし（無料枠の中なら $0） |

- **トピック** は各ブローカーとも 1 インスタンスに 1 つで、デプロイのテンプレートが作る（標準トピック）。プロセスごとのキュー・subscription はサーバーが作り、消す。
  消し損ねた分は、定期ジョブ `stream.sweep`（15 分ごと）が、KV の生存の印のないプロセスのものを消す。`stream_sqs_queue_prefix` と `stream_pubsub_subscription_prefix` は、そのインスタンスだけが使う値にする
- **Neon**: アイドルの `LISTEN` の接続はコンピュートを起こし続けないが、scale to zero でリスナーは切られる。ストリームのないプロセスは listen しないので、切られるのはストリームがないときだけ
- **Aurora DSQL**: `LISTEN` / `NOTIFY` を使えるかを公式の文書で確認できない（サポートする SQL の一覧にない）ので、`postgres` は使わない
- 未決（ADR-0050 の Open Questions）: Lambda でストリームを既定で有効にするか（下記の実行時間）と、`db = postgres` で `stream` の既定を `postgres` にするか。決まるまでは、どのターゲットも `stream` を明示的に設定する

### コスト（2026-10-19 確認）

- SNS（us-east-1）: API リクエストは月 100 万まで無料、以降 $0.50 / 100 万（64 KB ごとに 1 リクエスト）。SQS への配信は課金されない。知らせ 1 件は SNS 1 リクエスト
- SQS（us-east-1）: 月 100 万リクエスト無料、以降 $0.40 / 100 万。空の受信も 1 リクエストで、listen 中のプロセス 1 つで約 129,600 回 / 月（20 秒のロングポーリング）。常時開いたストリーム約 7 本分までが無料枠
- Lambda: SSE の接続 1 本が実行環境 1 つを最大 15 分（関数のタイムアウト）占有する。ブローカーとは別に、実行時間がストリームの本数 × 時間で増える（512 MB で 1 本を 1 か月開き続けると約 131 万 GB 秒で、無料枠 40 万 GB 秒を超える）
- Pub/Sub: スループット $40 / TiB、毎月 10 GiB まで無料（リクエストごとに最低 1 KB で数える）。subscription 3 つなら知らせ約 268 万件 / 月までが無料枠で、以降 100 万件ごとに約 $0.15。保持を 1 日以下にするので保存料はかからない。subscription の作成・削除は無料
- Cloud Run（リクエストベース課金）: CPU はリクエストの処理中だけ割り当てられる。SSE のストリームはリクエストなので、「ストリームがある間だけ受ける」形になる。subscription はインスタンスにつき 1 つ
- Valkey: ElastiCache Serverless for Valkey は最小 100 MB の課金で約 $6.13 / 月（$0.084 / GB 時間、us-east-1）、Memorystore for Valkey は最小の shared-core-nano でも約 $23 / 月（$0.0318 / 時間）。クラウドの既定にはしない

### 権限

- AWS（実行ロール）:
  - トピックに `sns:Publish`、`sns:Subscribe`（条件 `sns:Protocol` = `sqs`、`sns:Endpoint` をキューの ARN の接頭辞 `arn:aws:sqs:<region>:<account>:<stream_sqs_queue_prefix>-*` に限る）、`sns:Unsubscribe`、`sns:ListSubscriptionsByTopic`（sweep）
  - キュー `arn:aws:sqs:<region>:<account>:<stream_sqs_queue_prefix>-*` に `sqs:CreateQueue`、`sqs:TagQueue`、`sqs:SetQueueAttributes`、`sqs:GetQueueAttributes`、`sqs:ReceiveMessage`、`sqs:DeleteMessage`、`sqs:DeleteQueue`
  - `*` に `sqs:ListQueues`（sweep）
  - キューはトピックのアカウントとリージョンに作る
- Google Cloud（サービスアカウントのカスタムロール。`roles/pubsub.editor` は広すぎる）:
  - トピックに `pubsub.topics.publish`、`pubsub.topics.attachSubscription`
  - プロジェクトに `pubsub.subscriptions.create`、`pubsub.subscriptions.list`（sweep）、`pubsub.subscriptions.consume`、`pubsub.subscriptions.delete`（subscription の名前は前もって決まらないので、プロジェクトに付ける）

出典（2026-10-19 確認）:
[SNS pricing](https://aws.amazon.com/sns/pricing/) /
[SQS pricing](https://aws.amazon.com/sqs/pricing/) /
[SNS → SQS subscription](https://docs.aws.amazon.com/sns/latest/dg/subscribe-sqs-queue-to-sns-topic.html) /
[SNS API permissions](https://docs.aws.amazon.com/sns/latest/dg/sns-access-policy-language-api-permissions-reference.html) /
[Lambda pricing](https://aws.amazon.com/lambda/pricing/) /
[Pub/Sub pricing](https://cloud.google.com/pubsub/pricing) /
[Pub/Sub access control](https://docs.cloud.google.com/pubsub/docs/access-control) /
[Pub/Sub subscription properties](https://docs.cloud.google.com/pubsub/docs/subscription-properties) /
[Cloud Run billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings) /
[Neon: LISTEN / NOTIFY](https://neon.com/guides/pub-sub-listen-notify) /
[Aurora DSQL supported SQL](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-sql-features.html) /
[ElastiCache pricing](https://aws.amazon.com/elasticache/pricing/) /
[Memorystore for Valkey pricing](https://cloud.google.com/memorystore/valkey/pricing)

## 信頼するプロキシとクライアント IP

ログイン・登録・匿名のクライアント API・Inbox のレート制限は、クライアント IP（IPv6 は `/64`）ごとに数える（ADR-0019 §5）。
サーバーはリクエストごとに 1 回、`internal/platform` の HTTP ミドルウェアでクライアント IP を決め、リクエストの context に入れる（`kernel.RequestClientIP`）。

- 接続元が設定 `trusted_proxies`（CIDR かアドレスのカンマ区切り）に含まれないときは、接続元がクライアント IP で、転送ヘッダは読まない。既定は空で、誰も信頼しない。
- 含まれるときは、設定 `trusted_proxy_header` のヘッダ（`x-forwarded-for` が既定、`forwarded` は RFC 7239 の `for`）を右から見て、
  `trusted_proxies` に含まれない最初のアドレスを採る。それより左はクライアントが書いた値なので使わない。
  同名ヘッダが複数行あれば順につないだ 1 つのリストとして読む。
- 右から見てアドレスでない値（`unknown`、難読化した識別子、`for` のない要素、壊れた値）に当たったら、その値を書いた信頼するプロキシのアドレスを採る。
  全部が信頼するプロキシなら最も左のアドレスを採る。
- 読むヘッダは片方だけ。両方を読むと、片方しか書かないプロキシの背後でクライアントがもう片方を偽造できる。

設定しないとプロキシの背後では全リクエストがプロキシの IP に見え、ログインの IP 制限（5 回 / 分）などをインスタンス全体で共有する。
逆に、プロキシ以外も接続できるアドレスや、ヘッダに追記せずクライアントの値をそのまま渡すプロキシを入れると、クライアントが IP を偽って制限を逃れられる。

| ターゲット | 接続元 | `trusted_proxies` | `trusted_proxy_header` |
| --- | --- | --- | --- |
| Docker Compose（`deploy/docker`、ホストのリバースプロキシ） | Compose ネットワークのゲートウェイ（ポートは `127.0.0.1` にだけ公開） | ネットワークのサブネット（`docker network inspect <project>_default`。既定の割り当て範囲全体なら `172.16.0.0/12, 192.168.0.0/16`） | プロキシが追記するヘッダ。Caddy の `reverse_proxy` と Traefik は `X-Forwarded-For`、nginx は `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` で追記する |
| Cloud Run（直接公開） | Google Front End からのリンクローカルアドレス | `169.254.0.0/16` | `x-forwarded-for` |
| Cloud Run（外部アプリケーションロードバランサの背後） | 同上 | `169.254.0.0/16` に加えてロードバランサの転送ルールの IP | `x-forwarded-for` |
| AWS Application Load Balancer（EC2 / ECS） | ロードバランサのノード（ALB のサブネットのプライベート IP） | ALB を置いたサブネット（または VPC）の CIDR | `x-forwarded-for`（ALB の `routing.http.xff_header_processing.mode` は既定の `append` のままにする） |

- Google Cloud のロードバランサと Cloud Run は `X-Forwarded-For` を `<クライアントが送った値>, <クライアント IP>[, <ロードバランサの IP>]` の形にする。
  要検証: Cloud Run のコンテナから見た接続元が `169.254.0.0/16` に収まること（M6 のデプロイ検証でアクセスログの接続元を確認し、ずれていれば表を直す）。
- AWS ALB の `append` モードはクライアント IP を `X-Forwarded-For` の右端に追記する。`preserve` は追記しない（クライアントの値がそのまま届く）ので使わない。
- 要検証: Lambda Web Adapter 経由（Function URL / ALB → Lambda）では接続元がアダプタの `127.0.0.1` になる。`trusted_proxies = 127.0.0.1` で
  `X-Forwarded-For` を採れるか、ヘッダが前段で追記されたものかを、Lambda のデプロイテンプレートの作業で確認する。

## コスト見積りの観点（M6 で埋める）

| 観点 | 計測方法 |
| --- | --- |
| 1 投稿あたりの配送リクエスト数 | フォロワーのドメイン分布 × shared inbox 利用率（Cloud Tasks / SQS のオペレーション数に直結） |
| 受信アクティビティ量 | 連合タイムラインの流量（リレー非接続時） |
| DB のアイドル時間 | 5 分以上アクセスが途切れる時間の合計（Neon の scale to zero、Aurora の自動一時停止が効くか） |
| DB 読み書き行数 | タイムライン 1 回取得あたりの 読み取り行数・コンピュート時間（Neon の CU-hours、Turso の rows read、DSQL の DPU） |
| ストレージ | ローカルメディア総量、リモートメディアのキャッシュ方針別 |
| 転送量 | メディア配信（CDN 経由 / 直接）、連合配送の外向き通信 |
