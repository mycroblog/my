# ADR-0003: RDB の抽象化と対応方言

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #3, #1022, #1850

## Decision（決定）

### 1. 対応方言（v0）

| 方言ファミリ | 具体的な実体 | アダプタ |
| --- | --- | --- |
| SQLite | ローカル SQLite ファイル | `sqlite` |
| PostgreSQL 系 | セルフホスト PG、Neon、Supabase、Cloud SQL、RDS / Aurora | `postgres` |

MySQL 系（MySQL、TiDB Serverless、PlanetScale 等）は v0 では対象外とし、v1 以降にコミュニティアダプタとして受け入れられる設計にする。Aurora DSQL には対応しない（§14）。

libSQL / Turso は v0.1.0 では対応せず、将来の候補とする。v0.1.0 の時点でアダプタの実装がなく、対応するときは Go クライアントの成熟度（Open Questions）を確かめて、改めて決める。決定者の決定（#2178）。

### 2. 抽象化

- アプリケーション層は **集約単位のリポジトリポート** と **`UnitOfWork`（`Tx(ctx, func(tx) error) error`）** にのみ依存する。
- 短い対話的トランザクションを許容する。ただし:
  - **トランザクション内で外部 I/O（HTTP・キュー投入・Blob 操作）をしない**。キュー投入は Transactional Outbox 経由（ADR-0004）。
  - 直列化失敗・競合エラー（SQLSTATE 40001）は `UnitOfWork` が有限回リトライする（PG の serializable 対策）。そのためトランザクション関数は再実行可能に書く。
  - 一括処理は短いトランザクションのジョブに分割する。
  - ジョブは at-least-once なので、書き込みは引き続き一意制約 + upsert（`INSERT ... ON CONFLICT`）・冪等キーで重複に強くする。
  - カウンタ（フォロワー数・お気に入り数など）は加算 SQL（`SET n = n + 1`）を使い、定期ジョブで再集計して補正する。
- SQL の扱い（Go）: **sqlc を方言ごとに生成**（`queries/sqlite/<module>/*.sql`, `queries/postgres/<module>/*.sql`。モジュールごとの出力は ADR-0153 §3）し、方言別アダプタがリポジトリポートを実装する。
  方言差（FTS、JSON 関数、日付関数、プレースホルダ）はアダプタ内に閉じ込める。根拠は [#17 の調査報告](https://github.com/mycroblog/my/issues/17#issuecomment-6209364365)。
- ドライバ: SQLite は `modernc.org/sqlite`（cgo 不要）、PostgreSQL は `pgx/v5`。
- トランザクションの設定:
  - SQLite は `BEGIN IMMEDIATE`（`_txlock=immediate`）+ `busy_timeout` + WAL を必須とする。遅延ロックのままだと書き込みの競合でリトライが多発する。
  - PostgreSQL は READ COMMITTED を既定とし、直列化失敗・デッドロックは `UnitOfWork` のリトライで吸収する。
- スキーマは方言ごとに `schema/<dialect>/` を持つ（ADR-0023、ADR-0081）。
- **コントラクトテスト**: リポジトリポートごとに単一のテストスイートを書き、SQLite（インメモリ / 一時ファイル）・PostgreSQL で実行する。PostgreSQL の実行環境と頻度は ADR-0015 と ADR-0079 が決める。

### 3. 型・ID・時刻の共通規約

- ID: UUIDv7 を文字列（36 文字）として扱い、`kernel.IDGenerator`（`internal/adapter/system`）で生成する。同じプロセスの中では、時計が小さく巻き戻っても ID は増え続ける（`uuid.NewV7()` は秒をまたいで巻き戻るとシステムの時刻からやり直すので、続けて行った操作の順に並ばなくなる）。時系列ソート可能で、クライアント API のキーセットページングに使える。
  保存の型は、PostgreSQL では `uuid` 型（§5）、SQLite では `TEXT`（§7）にする。
- 時刻: UTC・ミリ秒精度。SQLite は `INTEGER`（Unix ミリ秒、`STRICT` テーブル）、PostgreSQL は `timestamptz`。ドライバ既定の文字列化は、文字列の順序が時刻の順序と一致しないので使わない。
- 真偽値: 整数 0/1（SQLite）/ `boolean`（PG）をアダプタで吸収。JSON: SQLite は TEXT、PG は `jsonb`。
- 文字列比較・照合順序に依存したロジックを書かない（ユーザー名は保存時に正規化した小文字カラムを別に持つ）。

### 4. KeyValueStore ポート

公開鍵・リモート文書のキャッシュ、レート制限、短命トークン用に KV ポートを用意する。
既定は RDB 上のテーブル実装（`kv-db`）。任意でメモリ（`kv/memory`）/ Valkey（`kv/valkey`）。

### 5. PostgreSQL の `uuid` 型の対象の列

PostgreSQL の UUIDv7 の列を `uuid` 型にし、SQLite と Go の型は変えない。

- 対象は、**値がいつもこのサーバーが生成した UUID である列**。主キーの `id`、それを指す `*_id` の列、UUIDv7 だけを持つカーソルの列
- 次は `text` のまま（`COLLATE "C"` も付けたまま）
  - UUID でない識別子: AP の URI、外部の事業者の ID（`provider_*_id`、決済の webhook の `event_id` / `object_id`）、OAuth の `client_id` と冪等キー（クライアントが決める値）
  - 種類の違う値が入る列: 監査ログの `target_id`（設定のキーなども入る）、リアクションの `emoji_key`（Unicode の絵文字かカスタム絵文字の ID）、
    ジョブの `run_key`、課金の取り込みのカーソル（`changes_cursor` など）
- 判断に迷う列は、値の出どころで決める。生成元が `kernel.IDGenerator` だけなら `uuid`、それ以外が 1 つでもあれば `text`
- `uuid` の列と `text` の列を比べるところ（`emoji_key` と `custom_emoji_id` の `CHECK` など）は `uuid_col::text` で比べる。`uuid` の文字列への変換は小文字の正規形なので、保存してある文字列と一致する

### 6. 並び順

- PostgreSQL の `uuid` の比較は 16 バイトの符号なしのバイト順（`memcmp`）。UUIDv7 は先頭の 48 ビットがミリ秒の時刻（ビッグエンディアン）なので、バイト順 = 時刻順 = 小文字の正規形の文字列のバイト順
- そのため `ORDER BY id`、`id < :cursor` のキーセットページング、`id >= UUIDv7Floor(t)` の読み取り窓は、`text COLLATE "C"` のときと意味も結果も変わらない
  - 確認: PostgreSQL 18.6 で 200 万行の UUIDv7 を `ORDER BY id` と `ORDER BY id::text COLLATE "C"` で並べ、順位の食い違いは 0 行（2026-10-23）
- PostgreSQL には `uuid` の `max` / `min` の集約関数が無い（18.6 で `function max(uuid) does not exist` を確認）。`MAX(id)` / `MIN(id)` は使わず、
  `ORDER BY id DESC LIMIT 1`・`DISTINCT ON` などで書く。集約関数を自前で定義することはしない（ADR-0023 の目標スキーマに書けないものを増やさない）

### 7. SQLite の ID

- SQLite は `TEXT` の 36 文字。SQLite は小さな構成向けで、大きさの差の効き目が小さい。`sqlite3` の CLI で読めることを優先する
- `BLOB` にしないので、SQLite の生成コードとクエリは PostgreSQL の型の選択に影響されない

### 8. Go の型と sqlc

- アプリケーション層とドメインの ID は `string`（36 文字の小文字の正規形）。ドメインに ID の型は持たない
- sqlc は PostgreSQL の `uuid` を `string` に対応させる（`sqlc.yaml` の `overrides` に `db_type: uuid`・`go_type: string`、NULL 可は `*string`）。
  pgx/v5 は `string` を `uuid` の引数に渡せ、`uuid` の列を `string` に読める（`pgtype.UUIDCodec` の text の経路。pgx v5 で確認、2026-10-23）。
  そのため `pgtype.UUID` も Go 1.27 の `uuid.UUID`（`driver.Valuer` / `sql.Scanner` を実装しない、#17）も、アダプタの外に出ない
- 方言の差は `schema/<dialect>/`・`queries/<dialect>/`・`sqlc.yaml` に閉じ込める。リポジトリのポートと契約テスト（`pkg/testkit`）のシグネチャは方言に依らない
- 「無し」を空文字 `''` で表す ID の引数・列は `uuid` に入らないので、次のようにする
  - クエリの任意の引数は、NULL 可の引数（`sqlc.narg`）にする
  - 列（リモートのリンクカードの画像の `media_files.owner_account_id` など）は、任意の関連として NULL 可にする（ADR-0020 規約 4 の「任意の関連」）

### 9. 外へ出す形と、不正な ID の入力

- 外へ出す ID は 36 文字の小文字の正規形。client API の `Id`（`format: uuid`）、AP の `id`、表示用 URL はこの形で、連合の URI は変えない（ADR-0023 §9、ADR-0018）
- PostgreSQL の `uuid` は大文字・ハイフン無し・`{…}` の形も受け付けて同じ値にする（18.6 で確認）。これを通すと同じ投稿に複数の URL ができるので、
  **受け付ける形は 36 文字の小文字の正規形の UUIDv7 だけ** にする。判定は `kernel` の 1 つの関数 `kernel.ValidID` に置き、外から来た ID を DB に渡す前に使う
  - 使うのは、client API・AP の入口と、アプリケーション層で受信した文書から自サーバーの URI の ID を取り出すところ（`internal/app/federation` の引用の承認の `quoteRequestStatus` など）
  - client API の path の ID: 正規形でなければ、存在しない ID と同じ `404`（ADR-0018 の、存在しないときと権限が無いときを同じにする方針に合わせる）
  - client API のクエリ・本文の ID（カーソル、`ids` での絞り込み、返信先・添付の参照など）: 正規形でなければ、その欄に存在しない ID を渡したときと同じ応答（カーソルは壊れたカーソルと同じ `400` の `invalid_parameter`）
  - AP: 受信した文書の中の自サーバーの URI は `^/ap/(actors|objects|activities)/{uuid}` で厳密にパースし、合わなければ自サーバーの URI として扱わない。正規表現の `{uuid}` は小文字の正規形だけにする
- アダプタに不正な形の ID が届くのは、上の検査の漏れ（バグ）として扱う。PostgreSQL では SQLSTATE `22P02` のエラーになり `500` になる。漏れは契約テストと client API のテスト（不正な形の ID を入れたときの応答）で見つける

### 10. `uuid` の大きさと速さ（簡単な計測）

PostgreSQL 18.6（`postgres:18-alpine`、WSL2 上の Docker）で、投稿に似た表（`id`・`account_id`・短い本文、200 万行、アカウント 1 万）を作り、`VACUUM ANALYZE` の後に測った（2026-10-23）。
速さは 1 台の手元の値で、傾向だけを見る。

| | `text COLLATE "C"` | `uuid` | 差 |
| --- | --- | --- | --- |
| 表 | 208 MB | 130 MB | −37% |
| 主キーの索引 | 113 MB | 60 MB | −47% |
| `(account_id, id)` の索引 | 321 MB | 137 MB | −57% |
| 200 万行の INSERT（索引あり） | 11.5 秒 | 9.6 秒 | −16% |
| 7 日の読み取り窓の件数（`id >= floor`） | 424 ms | 376 ms | −11% |
| 投稿とアカウントの結合（全件） | 79 ms | 62 ms | −22% |
| 主キーでの 1 行の取得 | 19.6 µs | 19.1 µs | ほぼ同じ |
| アカウントごとのキーセットのページ（41 行） | 232 µs | 222 µs | ほぼ同じ |

ID の列が多い表（関係・投影・イベント）ほど効く。読み取りの量で課金される DB では、索引の大きさの差がそのまま読み取り量の差になる（要検証: 実際の課金への効き目は M6 の計測で確かめる）。

### 11. `uuid` の対象とテスト

- Neon・Supabase・Cloud SQL・RDS / Aurora は PostgreSQL そのものなので、`uuid` は組み込みの型として使える
- ID の列は `bytea` の 16 バイトにはしない
- pgmem（PGlite のフォーク、PostgreSQL 18.3）も組み込みの `uuid` をそのまま持つ。契約テストでスキーマを通す

### 12. `uuid` への移行

移行は書かなかった。`v0.1.0` より前の変更なので、スキーマを書き換えて DB を作り直して入れた（ADR-0023 §4）。

### 13. `uuid` の決定を見直す条件

- 対象の DB のどれかで、`uuid` 型の列の索引や `uuid` と `text` の比較が使えない・遅いと分かった場合
- 外部キーや結合で `uuid` と `text` が混ざる列が多く、キャストがクエリの大半に要ると分かった場合（そのときは `text COLLATE "C"` に戻す）

### 14. Aurora DSQL に対応しない

- 最大の理由は、壁が SQL の方言ではなく同時実行の意味にあることである。`READ COMMITTED` とロックの待ちを前提にした処理は、構文を置き換えても正しくならない。ロックを読み直しだけに替えると write skew が起き、すべてを書き込みの競合に替えるには各処理の見直しが要る。
- pgmem は楽観的同時実行制御を再現しない。DSQL だけの退行は、AWS の実クラスタを使う nightly でしか見つからない。一方、`postgres` アダプタへの変更はすべて、この 2 つの意味の両方で正しいことを求められ続ける。
- 費用の利点は本物だが（採らなかった案を参照）、外部の PostgreSQL という別の手段がある。整合性と開発の余力を上回らない。AWS の DB は Aurora Serverless v2（0 ACU）、RDS、外部の PostgreSQL（Neon など）にする。

範囲は次のとおりとする。

- `database = postgres` の接続先として DSQL を検出して拒むことはしない。運用文書と設計文書の対応する DB に載せず、動作を保証しない。
- §5 の `uuid` の決定は、PostgreSQL の利点でも成り立つので変えない。

次のいずれかで、新しい判断として見直す。

- AWS の運用者の実測で、Aurora Serverless v2 と外部の PostgreSQL のどちらも費用か運用の要件を満たさない。
- DSQL が `READ COMMITTED` 相当の分離か、advisory lock か、待つ行ロックに対応する。
- このサーバーの同時実行の設計が、DSQL と別の理由で書き込みの競合を軸にした形に変わる。

見直すときは、先に実クラスタで次を確かめる: `READ COMMITTED` の `BEGIN` と advisory lock が返すエラー、Atlas CE のカタログの読み取りと偽の差分、`CREATE INDEX ASYNC` の `DESC` と `COLLATE "C"`、`ADD COLUMN` の `DEFAULT` と `NOT NULL`、同時の書き込みの敗者のエラー、`40001` の率、実際の DPU、接続プールと Idle への移行の関係。

## 経緯

運営者が「維持コストの安いもの、あるいは慣れたもの」から RDB を選べるようにしたい（G3）が、方言を増やすほどテストと保守の費用が増える。無料枠や低価格の選択肢（SQLite、Neon、Supabase）と慣れた PostgreSQL を覆い、at-least-once のジョブ（ADR-0004）と `CGO_ENABLED=0`（ADR-0002）に合うことを基準に、SQLite 系と PostgreSQL 系の 2 ファミリに決めた。Cloudflare D1 は ADR-0002 により対象外である。

PostgreSQL の ID の型は当初、`COLLATE "C"` の `text` か `uuid` 型かを未決にしていた。`text` の 1 値 37 バイトが `uuid` で 16 バイトになり、表と索引が小さくなり、不正な値を DB が拒む。型の変更は実運用のデータが溜まると移し替えが要るので、v0.x の公開の前に `uuid` に決めた（#1022、実装は #1049）。外へ出す ID の形・並び順・アプリケーション層を変えないことを条件にした。

Aurora DSQL は当初「検証次第で」候補に残し、トランザクションあたり 3,000 行の上限もそこから来ていた。`bytea` を ID に使わない判断も、DSQL で索引に使えないことが当時の理由だった。M6 の AWS の DB の既定を決めるために調べた結果（2026-11-02、AWS の DSQL ユーザーガイドと料金表、この時点の `main` のコード。実クラスタでは未確認。[#1846 の報告](https://github.com/mycroblog/my/issues/1846#issuecomment-6494456875)）、次の壁があり対応しないと決めた。Repeatable Read 固定と楽観的同時実行制御（`FOR UPDATE` も待たない）、advisory lock がないこと（8 つのクエリ、10 の呼び出し元と `my schema apply` が使う）、同時の書き込みの敗者が `23505` になるとは限らないこと（要検証）、3,000 行・10 MiB・5 分の上限、Atlas CE のスキーマ適用と ADR-0023 の拡張と縮小の手順が使えないこと、`bytea` の主キーと一意索引、`SKIP LOCKED` がないこと、IAM トークンの認証。

## 採らなかった案

- PostgreSQL のみ: 方言は最少だが、無料枠の選択肢（SQLite）を覆えない。
- SQLite 系 + PostgreSQL 系 + MySQL 系: 選択肢は広がるが、テストと保守の費用が方言の数だけ増える。MySQL 系は v1 以降のコミュニティアダプタに回す。
- ORM の方言吸収に任せる: 検討した案。
- 生 SQL を方言別に手書き: 方言分岐とスキャン定型が全メソッドに散る（[#17 の調査報告](https://github.com/mycroblog/my/issues/17#issuecomment-6209364365)）。
- 書き込みはアトミックバッチのみ: 短い対話的トランザクションより実装が素直でなくなる。
- PostgreSQL の ID を `text COLLATE "C"` のまま: 変更が無く 2 方言のクエリがほとんど同じ形を保てる。正しさの問題も無い。大きさと速さの差は小さな構成では見えないが、v0.x の後では変えにくい。
- `uuid` + SQLite は `BLOB` の 16 バイト: SQLite も小さくなるが、`sqlc` の生成型が `[]byte` になりアダプタで変換が要る。小さな構成で得るものに比べて手間が大きい。
- `uuid` + ドメインの ID の型（`[16]byte`）: 型で正規形を保証できるが、全モジュールの書き換えになる。入口の検査（§9）で同じ保証が得られる。
- DSQL を `postgres` アダプタの「DSQL モード」で受ける: AWS の中だけで仕事の量に比例した費用にでき、これが最も強い根拠である（東京リージョン、データ 5 GB、1 か月 730 時間の試算で、誰も使わない月は DSQL 約 $1.60、Aurora Serverless v2 約 $0.60、RDS `db.t4g.micro` 約 $21。動き続ける月は Serverless v2 が 0.5 ACU で約 $55（要検証）、DSQL は未測定。[AWS Price List](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AuroraDSQL/current/ap-northeast-1/index.json)、2026-11-02）。ただし 8 つのロックの処理、一意制約の違反を前提にする処理、一括処理の上限、スキーマの適用の分岐を見直し、2 つの意味を保ち続ける必要がある。
- `dsql` アダプタを別に作る: DSQL の意味を PostgreSQL の設計から分離できるが、`internal/adapter/db/postgres` の 109 ファイルとクエリの複製を保ち、方言が 3 つになる。1 つの配置先の利点に対して大きすぎる。

## Consequences（影響）

- 運営者は SQLite（$0）から始め、規模に応じて PostgreSQL に移行できる（v0 期間はデータ移行ツールは提供しない）。
- 対話的トランザクションを使えるため実装が素直になる。一方、再実行可能なトランザクション関数とトランザクション外での外部 I/O という規律が必要（AGENTS.md に明記）。
- sqlc を方言別に生成するので、クエリが方言の数だけ重複する。コントラクトテストで挙動の一致を担保する。
- 方言 2 ファミリ × コントラクトテストの CI 時間が増える。
- PostgreSQL の表と索引が 4〜6 割ほど小さくなり、結合と範囲の読み取りが 1〜2 割速くなる。DB が UUID でない値を拒み、外から受け取る ID の形の検査が 1 か所にまとまる。
- 方言でスキーマの型が変わり、PostgreSQL のクエリだけに `::text` のキャスト、`MAX(id)` の書き換え、`sqlc.narg` が入る。`psql` で ID の列を文字列の関数（`LIKE` など）で扱うときにキャストが要る。
- `postgres` アダプタは PostgreSQL の意味だけを前提にでき、`READ COMMITTED` とロックの設計を保てる。
- AWS の中で、低い負荷で動き続けるサーバーの費用が DSQL より高くなりうる。Aurora Serverless v2 は接続があると 0 ACU にならない。運用者は費用か、外部の DB か、AWS の外への通信を受け入れる。
- `v0.1.0` の後に DSQL を見直すと、`bytea` のキーの型の変更が ADR-0023 §5 の 2 段階の移行になる。
- RDS と Aurora の IAM データベース認証も今は実装しない。静的なパスワードをなくす必要が出たら、別の Issue にする。
- 後続: M6 の AWS のテンプレートの DB の既定を、Aurora Serverless v2 と外部の PostgreSQL の実測で決める（#29）。

## Open Questions / 要検証

- 読み取り専用の経路（`BEGIN IMMEDIATE` を取らない読み取りトランザクション / 非トランザクション読み取り）の API
- 将来 libSQL / Turso に対応するときの、Go クライアントの成熟度（cgo 要否、リモート接続でのトランザクション）
- `uuid` と `text` が混ざる比較・結合（§5 の `text` のままの列との結合）が、実際のクエリでどれだけあるか。多すぎれば §13 に当たる
- 読み取り量で課金される DB での、実際の課金への効き目（M6 の計測）
