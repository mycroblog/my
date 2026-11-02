# ADR-0128: Aurora DSQL に対応しない

- Status: Accepted
- Date: 2026-11-02
- Deciders: @rosylilly
- Issue: #1850

## Context（背景・課題）

M6（AWS のテンプレートと各系統のアダプタ、#29）の DB の既定を決めるには、Amazon Aurora DSQL に対応するかを先に決める必要がある。関連する要件は NFR-COST-1（アイドル時のコンピュートをほぼゼロにする）、NFR-COST-2（RDB 1 つで全機能が動く）、G2（同じコードで各系統が動く）である。

今の `db = postgres` は DSQL では動かない。次の事実は AWS の DSQL ユーザーガイドと料金表（2026-11-02 に確認）と、この ADR の時点の `main` のコードから取った。実際のクラスタでは確かめていない。調査の詳細は [#1846 の報告](https://github.com/mycroblog/my/issues/1846#issuecomment-6494456875)にある。

| 分野 | DSQL | このサーバー |
| --- | --- | --- |
| 分離レベル | Repeatable Read 固定、楽観的同時実行制御。`FOR UPDATE` も待たず、競合はコミット時などに `40001` | `READ COMMITTED` で始める（`postgres.go`）。ロックを待ってから新しいスナップショットで読み直す処理がある（`content/projection.go`、`queue.sql` の claim） |
| ロック | AWS のガイドに advisory lock の記載がない。AWS のサンプルと Atlas の文書は使えないとしている | `pg_advisory_xact_lock` が 8 つのクエリ、10 の呼び出し元。`my schema apply` にも `pg_advisory_lock` がある |
| 制約違反の扱い | 同時の書き込みの敗者が `23505` になるとは限らない（要検証） | `uow` の契約は「敗者は再試行しないエラー」で、`isUniqueViolation` が 31 か所、一意制約の違反を読み直しの再試行にする箇所が 11 か所以上ある |
| トランザクションの上限 | 3,000 行、10 MiB、5 分 | `projection.Service.compare` は射影の再構築を 1 トランザクションで行う（`my projection verify` と定期ジョブ）。投稿の削除の連鎖も 1 トランザクションで上限を超えうる |
| スキーマの適用 | インデックスは `ASYNC`、1 トランザクションに DDL は 1 文。Atlas の DSQL ドライバは Pro 版だけ | Atlas CE のライブラリが `CREATE INDEX`（300 個、うち 53 個が `DESC`）を 1 トランザクションで流す。`InspectSchema` が読むカタログには、DSQL のガイドが対象外とする `pg_opclass` などが含まれる |
| スキーマの変更 | `ALTER COLUMN TYPE`・`SET NOT NULL` はなく、主キーは変えられない。`ADD COLUMN` の文法に `DEFAULT`・`NOT NULL` がない（説明と食い違う。要検証） | [ADR-0087](0087-schema-changes-after-launch.md) の拡張と縮小の手順で、列の追加と制約の変更を使う |
| 型 | `bytea` は索引に使えない | `bytea` の主キー 4 つ、一意索引 6 つ |
| DB キュー | `SKIP LOCKED` は文法にない | `PickJob` が使う。`queue = sqs` でも順序キーのあるジョブの claim は advisory lock を使う |
| 認証 | IAM トークン（既定 15 分）、接続は最長 60 分 | 仕組みがない |

DSQL は外部キー、シーケンス、`jsonb`、`DROP COLUMN`、式インデックス、部分インデックスに対応しているが、このうち今のスキーマが使うのは部分インデックス（6 個）だけで、上の表の壁は変わらない。
[ADR-0126](0126-embedded-schema-apply.md) で計画の SQL を `my` が持つので書き換えの実装先はあるが、カタログを読めるかは未確認である。

## Decision Drivers（判断基準）

1. **整合性の壊れにくさ**: 分離レベルに依存する処理の誤りは、テストをすり抜けてデータを静かに壊す。
2. **日々の開発の検査で守れること**: PR と Merge Queue の検査（pgmem、ADR-0041）で退行を検出できること。
3. **v0.1.0 に向けた開発の余力**: M5 と M4.3 の完了を遅らせないこと。
4. **AWS でのアイドル費用**（NFR-COST-1）: 使っていないサーバーが費用をほぼ生まないこと。
5. **対象の広さ**（G2）: AWS の運用者が選べる DB の幅。

## Considered Options（選択肢）

1. **A. `postgres` アダプタの「DSQL モード」で受ける**: 接続先ごとにロック、分離レベル、スキーマの適用を切り替える。
2. **B. 対応しない**: AWS の DB は Aurora Serverless v2（0 ACU）、RDS、外部の PostgreSQL（Neon など）にする。
3. **C. `dsql` アダプタを別に作る**: クエリとリポジトリを DSQL 向けに分ける。

## Decision（決定）

**B を採る。Aurora DSQL に対応しない。** 決定者が選んだ（「推奨案の B でいいです。DSQL に対応しない」）。

- 最大の理由は、壁が SQL の方言ではなく同時実行の意味にあることである。`READ COMMITTED` とロックの待ちを前提にした処理は、構文を置き換えても正しくならない。ロックを読み直しだけに替えると write skew が起き、すべてを書き込みの競合に替えるには各処理の見直しが要る（Driver 1）。
- pgmem は楽観的同時実行制御を再現しない。DSQL だけの退行は、AWS の実クラスタを使う nightly でしか見つからない（Driver 2）。一方、`postgres` アダプタへの変更はすべて、この 2 つの意味の両方で正しいことを求められ続ける。
- 費用の利点は本物だが（下の比較）、外部の PostgreSQL という別の手段がある。整合性と開発の余力を上回らない。

範囲は次のとおりとする。

- `db = postgres` の接続先として DSQL を検出して拒むことはしない。運用文書と設計文書の対応する DB に載せず、動作を保証しない。
- ADR の本文は書き換えないので、次の DSQL についての記述はこの ADR で置き換える。
  - [ADR-0003](0003-database-abstraction.md) の対応表の「検証次第で Aurora DSQL」、Open Questions の DSQL の検証、DSQL の nightly の契約テスト、DSQL から来た「1 トランザクションの変更は 3,000 行まで」の上限。一括処理を短いトランザクションのジョブに分ける方針は変えない。
  - [ADR-0023](0023-declarative-schema-management.md) の DSQL の未検証の記述、[ADR-0034](0034-remote-content-scope-and-retention.md) と [ADR-0078](0078-postgres-uuid-columns.md) の課金の例と M6 の対象、[ADR-0050](0050-stream-pubsub-across-processes.md) の `LISTEN` の可否、[ADR-0120](0120-external-queue-wake-up-signals.md) の見直す条件、[ADR-0126](0126-embedded-schema-apply.md) の書き換えの案。
- ADR-0078 の `uuid` の決定は、PostgreSQL の利点でも成り立つので変えない。

### 見直す条件

次のいずれかで、新しい ADR で見直す。

- AWS の運用者の実測で、Aurora Serverless v2 と外部の PostgreSQL のどちらも費用か運用の要件を満たさない。
- DSQL が `READ COMMITTED` 相当の分離か、advisory lock か、待つ行ロックに対応する。
- このサーバーの同時実行の設計が、DSQL と別の理由で書き込みの競合を軸にした形に変わる。

見直すときは、先に実クラスタで次を確かめる: `READ COMMITTED` の `BEGIN` と advisory lock が返すエラー、Atlas CE のカタログの読み取りと偽の差分、`CREATE INDEX ASYNC` の `DESC` と `COLLATE "C"`、`ADD COLUMN` の `DEFAULT` と `NOT NULL`、同時の書き込みの敗者のエラー、`40001` の率、実際の DPU、接続プールと Idle への移行の関係。

## Consequences（影響）

- 良い影響
  - `postgres` アダプタは PostgreSQL の意味だけを前提にでき、`READ COMMITTED` とロックの設計を保てる。
  - M6 の AWS の DB の既定を決められ、#29 の残りが進む。
  - `bytea` のキーの型の変更、ロックの行の表、DDL の書き換え、DSQL の nightly が不要になる。
- 悪い影響・トレードオフ
  - AWS の中で、低い負荷で動き続けるサーバーの費用が DSQL より高くなりうる。Aurora Serverless v2 は接続があると 0 ACU にならない。運用者は費用か、外部の DB か、AWS の外への通信を受け入れる。
  - v0.1.0 の後に見直すと、`bytea` のキーの型の変更が ADR-0087 の 2 段階の移行になる。
  - 同時に検討していた RDS と Aurora の IAM データベース認証も、今は実装しない。静的なパスワードをなくす必要が出たら、別の Issue にする。
- 後続の作業
  - M6 の AWS のテンプレートの DB の既定を、Aurora Serverless v2 と外部の PostgreSQL の実測で決める（#29）。

## Pros and Cons of the Options（詳細比較・任意）

### 費用の比較

東京リージョン、データ 5 GB、1 か月 730 時間の試算。一覧の価格で、I/O、バックアップ、通信を除く。単価は [AWS Price List](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AuroraDSQL/current/ap-northeast-1/index.json) から取り、合計はこの ADR の計算である。

| 構成 | 単価 | 誰も使わない月 | 動き続ける月 |
| --- | --- | --- | --- |
| Aurora DSQL | $10 / 100 万 DPU、$0.40 / GB-month（1 GB まで無料） | 約 $1.60 | 仕事の量に比例。このサーバーの DPU は未測定 |
| Aurora Serverless v2 | $0.15 / ACU 時、$0.12 / GB-month | 約 $0.60（停止中。再開に約 15 秒以上） | 0.5 ACU に留まるとして約 $55（要検証） |
| RDS `db.t4g.micro`（Single-AZ、20 GB） | $0.025 / 時、$0.138 / GB-month | 約 $21 | 約 $21 |

定期ジョブや常時の接続で停止しない Serverless v2 と比べると、DSQL の差は大きい。これが A の最も強い根拠である。

### A. `postgres` アダプタの DSQL モード

- 良い: AWS の中だけで、仕事の量に比例した費用にできる。コードの大半（1,394 のクエリ）を共有できる。ロックは既に `LockPair` などのポートのメソッドなので、差し替える場所がある。
- 悪い: 8 つのロックの処理と、一意制約の違反を前提にする処理の見直し（`:execrows` の `ON CONFLICT DO NOTHING` 69 個の呼び出し元は未監査）。既定 5 回、最大 100 ms の再試行は、楽観的同時実行の競合の多さに合わない。一括処理の上限対策。スキーマの適用の分岐（ロック、拡張、作業用スキーマ、カタログ）と、ADR-0087 の手順が使えない変更への対処。すべてで 2 つの意味を保ち続ける必要がある。

### B. 対応しない

- 良い: 上の作業がすべて不要で、整合性の設計を保てる。Driver 1〜3 に最もよく合う。
- 悪い: AWS で動き続ける小さなサーバーの費用で不利になる（Driver 4）。選べる DB が 1 つ減る（Driver 5）。

### C. 別のアダプタ

- 良い: DSQL の意味を PostgreSQL の設計から分離でき、`postgres` アダプタの制約にならない。
- 悪い: `internal/adapter/db/postgres` の 109 ファイルとクエリの複製を、機能を足すたびに保つことになる。方言は SQLite と PostgreSQL に DSQL が加わって 3 つになる。費用は 1 つの配置先の利点に対して大きすぎる。

## 出典

2026-11-02 確認:
[DSQL release notes](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/release-notes.html) /
[Migration guide](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-migration-guide.html) /
[Supported SQL](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-sql-features.html) /
[Data types](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-data-types.html) /
[Concurrency control](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-concurrency-control.html) /
[CREATE INDEX](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-index-syntax-support.html) /
[ALTER TABLE](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/alter-table-syntax-support.html) /
[System tables](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-systems-tables.html) /
[Quotas](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html) /
[Aurora Serverless v2 auto-pause](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-serverless-v2-auto-pause.html) /
[AWS samples: Rails](https://github.com/aws-samples/aurora-dsql-samples/tree/main/ruby/rails) /
[Atlas: Aurora DSQL](https://atlasgo.io/guides/dsql/automatic-migrations)
