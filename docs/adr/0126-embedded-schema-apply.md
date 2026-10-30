# ADR-0126: スキーマの適用を my に組み込み、Atlas の Go ライブラリで my schema apply する

- Status: Accepted
- Date: 2026-10-30
- Deciders: @rosylilly
- Issue: #1888

## Context（背景・課題）

[ADR-0023](0023-declarative-schema-management.md) は、目標スキーマ `schema/<dialect>/` を Atlas Community Edition の CLI（`atlas schema apply`）で適用し、
サーバーの単一バイナリには Atlas を組み込まないと決めている。そのため今の配布物は次の形になっている。

- サーバーのイメージとは別に、Atlas の CLI とスキーマのファイルを入れた `schema` イメージ（`Dockerfile` の `schema` ステージ）がある。
- Docker Compose の構成は、`upgrade-check`（`my db upgrade -check`）・`schema`（`atlas schema apply`）・`upgrade`（`my db upgrade`）の 3 つの一回限りのサービスを、2 つのイメージで順に動かす（[ADR-0087](0087-schema-changes-after-launch.md)）。
- PostgreSQL では、運用者が準備 SQL（`schema/postgres.bootstrap.sql`）を `psql` などで別に流し、差分の計算に使う空の開発用 DB（`--dev-url`）を自分で用意する。

`v0.1.0` でメンテナのインスタンスの運用を始めると、この手順を版ごとに繰り返すことになる。M6 のデプロイテンプレート（GCP / AWS / microk8s）も、
どれも 2 つのイメージと開発用 DB の準備を書くことになる。ワーカーとサーバーを同じバイナリで切り替える（NFR-PERF-2）のと同じく、
スキーマの適用も `my` の 1 つのイメージで済めば、Self-hosted の最小構成（NFR-COST-3）の手順が短くなる。
スキーマは既に `schema/schema.go` でバイナリに埋め込んであり、目標スキーマとバイナリの版がずれることもない。

Atlas の CLI はアプリケーションに組み込めないので、差分を計算して適用する部分を Go のライブラリで持てるかを確かめた（2026-10-30、手元の試作。下の Pros and Cons）。

## Decision Drivers（判断基準）

1. 今の目標スキーマ（SQLite・PostgreSQL それぞれ 623 文）を書き換えずに、正しく適用でき、2 回目の適用で差分が出ない（収束する）こと
2. CI の `db:lint`（[ADR-0087](0087-schema-changes-after-launch.md) §3、Atlas の `migrate lint`）が確かめた差分と、本番で流れる差分が、同じエンジンで計算されること
3. 運用者の手順とイメージが減ること（NFR-COST-3）
4. `CGO_ENABLED=0` でビルドでき、AGPL-3.0 と両立するライセンスであること
5. 依存の API が安定していて、版の更新で壊れにくいこと

## Considered Options（選択肢）

1. **Atlas の Go ライブラリ（`ariga.io/atlas`、Apache-2.0）を `my` に組み込む**: 目標スキーマを開発用 DB に流して読み取り（inspect）、対象 DB を読み取って差分を取り（`SchemaDiff`）、計画を作って流す（`PlanChanges` / `ApplyChanges`）。CLI の `atlas schema apply` と同じエンジン。
2. **sqldef（`github.com/sqldef/sqldef/v3`、MIT）を `my` に組み込む**: 目標スキーマを sqldef 自前の構文解析器（PostgreSQL では pg_query も）で読み、対象 DB から書き出した DDL と比べる。開発用 DB は要らない。
3. **Atlas の CLI をサーバーのイメージに同梱し、`my schema apply` から実行する**: `my` は CLI を呼ぶだけにする。
4. **今のまま**（別の `schema` イメージで Atlas の CLI を動かす）。

## Decision（決定）

**選択肢 1 を採る。** 試作で、今の目標スキーマを書き換えずに両方の方言で適用でき、2 回目の差分が 0 になった（判断基準 1）。
CI の `db:lint` と同じエンジンなので、レビューで確かめた差分がそのまま本番で流れる（判断基準 2）。
選択肢 2 は判断基準 1 を満たさない。選択肢 3 は判断基準 3 を一部しか満たさず（開発用 DB と準備 SQL は残る）、バイナリの外に実行ファイルを持つ。

ADR-0023 のうち、「サーバーの単一バイナリには Atlas を組み込まない。スキーマの適用はデプロイ手順の一部として、Atlas の CLI で行う」を、この ADR が置き換える。
目標スキーマを正とすること、Community Edition の範囲の機能だけを使うこと、`atlas login` の要る機能に依存しないことは、ADR-0023 のまま変えない。

### 1. コマンド

```text
my schema apply [-dry-run] [-allow-destructive] [-config <file>]
```

- 設定（`db`・`database_url`）はサーバーと同じものを読む。適用するのは、そのバイナリに埋め込んだ目標スキーマ（`schema.SQLite` / `schema.Postgres`）。
- 手順:
  1. PostgreSQL では、準備 SQL（`schema.PostgresBootstrap`）を対象 DB に流す。運用者が `psql` を使う手順はなくなる。
  2. 開発用 DB を用意し、目標スキーマを流して読み取る。
     - SQLite はインメモリの DB（接続ごとに別の DB になるので、接続は 1 つ）。
     - PostgreSQL は、対象 DB の中に実行ごとに名前の違う一時スキーマを作り、`search_path` をそのスキーマ（と拡張機能のある `public`）にした 1 つの接続で目標スキーマを流して読み取る。
       読み取った結果のスキーマ名を対象のスキーマ（`public`）に置き換えてから差分を取る。一時スキーマは、失敗したときも含めて消す。
       対象 DB に `CREATE` の権限がない環境のために、別の空の DB を指す設定（例: `database_dev_url`）も受ける。
  3. 対象 DB を読み取り、差分を取り、計画を作る。差分がなければ何もしない（何度実行してもよい）。
  4. 計画を 1 つのトランザクションで流す。途中で失敗したら全体を戻す。
     - PostgreSQL は DDL もトランザクションの中で戻る。
     - SQLite は Atlas の `sqlite.OpenTx` を使う。対象 DB はサーバーと同じ接続の設定（`foreign_keys(1)`）で開き、`OpenTx` が外部キーの強制を外してトランザクションを開き、
       コミットの前に `PRAGMA foreign_key_check` の違反が増えていないことを確かめる（増えていたら戻す）。表の作り直し（新しい表を作り、行を写し、古い表を消して名前を替える）もこの中で行う。
  5. `-dry-run` は計画の SQL を表示するだけで流さない。
- **破壊的な差分は既定で拒む。** 差分（`schema.Change`）に表・列の削除（`DropTable`・`DropColumn` など）が含まれるときは、`-allow-destructive` がない限り何も流さずに失敗する。
  計画の SQL の文字列では判定しない（SQLite の表の作り直しは、データを失わなくても `DROP TABLE` を含む）。
  ADR-0087 §2 の B・C（投影の作り直し・contract）を運用者が意図して流すときだけ付ける。CI の `db:lint` の検査はそのまま残す。
- 同時に複数が動いても壊れないように、PostgreSQL では適用の間 advisory lock（セッション単位）を取る。SQLite は書き込みのロックで直列になる。
- `my serve` は今までどおりスキーマを適用しない。起動のたびの差分計算（scale-to-zero の起動の遅れ）と、複数のインスタンスの同時の適用を避けるため。

### 2. 置き場所

- Atlas の方言のドライバは DB の SDK と同じく外部への依存なので、`internal/adapter/db` の方言ごとのアダプタに置く。`no_sqlite` / `no_postgres` のビルドタグで、その方言の Atlas のドライバも外れる。
- `cmd/my` がコマンドを組み立てる。ドメイン・アプリケーション層は Atlas を import しない（[ADR-0002](0002-modular-monolith-hexagonal.md)）。

### 3. CLI とライブラリの版

- CI の `db:lint`・`db:lint-test` は Atlas の CLI（`migrate diff` / `migrate lint`）のまま使う。
- `go.mod` の `ariga.io/atlas` の版と `scripts/install-atlas.sh` の `ATLAS_VERSION` を同じにし、`scripts/task.sh check` でずれを検査する。Dependabot が片方だけを上げても通らない。

### 4. デプロイと文書

- `Dockerfile` の `schema` ステージと `schema` イメージをなくす。Docker Compose の `schema` サービスはサーバーのイメージで `my schema apply` を動かす。
- 開発用のタスク（`db:apply`・`db:diff`・`db:reset`）は `my schema apply` を使う。
- `docs/operations/database.md` の初期化と更新の手順を `my schema apply` に書き換える。

### 見直す条件

- Atlas のライブラリの API（`sql/schema`・`sql/migrate`・方言のドライバ）が Community Edition の範囲から外される、または互換性を大きく壊す変更が続く。
- sqldef が今の目標スキーマを書き換えずに読め、PostgreSQL でも収束するようになり、開発用 DB を不要にする利点が上回る。

## Consequences（影響）

- 良い影響
  - 運用者に要るのはサーバーのイメージ 1 つになる。Compose の 3 つの一回限りのサービスはどれも同じイメージで動き、PostgreSQL の準備 SQL と開発用 DB の用意も `my` がする。
  - バイナリに埋め込んだスキーマを適用するので、版と目標スキーマの組み合わせを運用者が取り違えない。
  - 計画の SQL を `my` が持つので、[#1850](https://github.com/mycroblog/my/issues/1850)（Aurora DSQL）の「Atlas CE の差分を DSQL の形に書き換えて流す」案を、`my` の中で実装できる。
- 悪い影響・トレードオフ
  - バイナリが大きくなる。Atlas と一緒に `hashicorp/hcl/v2`（MPL-2.0。`GO_ALLOWED_LICENSES` に含まれる）・`zclconf/go-cty` などが入る。この増分は判断に入れない。
  - CLI の `atlas schema apply` が `file://` と `--dev-url` で行っている処理（目標スキーマの読み込み、開発用 DB の扱い）を `my` が自前で持つ。CLI と振る舞いがずれないことは、テストで確かめ続ける必要がある。
  - Atlas の版の更新は、`go.mod` と CLI の 2 か所を一緒に上げる手間がかかる。
- 後続の作業（Issue 化する）
  - `my schema apply` の実装（§1・§2・§3）
  - デプロイの構成・開発用のタスク・運用文書を `my schema apply` に移し、`schema` イメージをなくす（§4）

## Pros and Cons of the Options（詳細比較・任意）

2026-10-30 に手元で試作した（Go 1.27、`CGO_ENABLED=0`、PostgreSQL 18 のコンテナ、Atlas CLI Community Edition v1.3.0 と比べた）。試作のコードはリポジトリに入れていない。
文の数はその日の `main` の目標スキーマのもの。

| 項目 | 1. Atlas ライブラリ v1.3.0 | 2. sqldef v3.11.28 |
| --- | --- | --- |
| `CGO_ENABLED=0` | ビルドできる。cgo の `mattn/go-sqlite3` はテストでしか使われず、リンクされない | ビルドできる。PostgreSQL の構文解析は WASM 版の pg_query（wazero）に替わる |
| ドライバ | 既存の `modernc.org/sqlite` と `pgx` の `*sql.DB` をそのまま渡せる | SQLite は `modernc.org/sqlite`。PostgreSQL は自分で `lib/pq` の接続を開く |
| SQLite の目標スキーマ | 623 文をそのまま適用。2 回目の差分 0 | 24 文を読めない（`key`・`value`・`text`・`interval`・`comment`・`change`・`sensitive` という列名、`CHECK` の `GLOB` と `date(x) IS x`）。列名を引用符で囲んでも 15 文が残る |
| PostgreSQL の目標スキーマ | 623 文をそのまま適用（開発用 DB は同じサーバーの別の DB でも、対象 DB の一時スキーマでも）。2 回目の差分 0 | 7 文を読めない。読めた文だけでも、2 回目に 61 文の差分（`bytea` の列の `ALTER TYPE`、`CHECK` の作り直し）が出て収束しない |
| 生成する SQL | — | `CHECK ((a = 'x') = (b IS NOT NULL))` の括弧を落として `a = 'x' = b is not null` を出し、PostgreSQL が構文エラーで拒む |
| 組み込みの API | `sql/schema`・`sql/migrate`・方言のドライバは公開のパッケージ | 入口の `sqldef.Run` は `log.Fatal` / `os.Exit` を呼ぶので使えず、`schema.GenerateIdempotentDDLs`・`database.RunDDLs` を直接呼ぶ。互換性を壊す v4 が予告されている |
| 開発用 DB | 要る（SQLite はインメモリ、PostgreSQL は一時スキーマか別の DB） | 要らない |
| CI の `db:lint` との関係 | 同じエンジン | 別のエンジン（差分の計算がずれうる） |

選択肢 1 の §1 の手順（一時スキーマ・トランザクション・破壊的な差分の判定）を試作し、同じ日に次を確かめた。

| 確かめたこと | 結果 |
| --- | --- |
| PostgreSQL: 空の DB に、一時スキーマを開発用 DB にして適用 | CLI の `schema diff`（`file://schema/postgres`）が差分なし。一時スキーマは残らない。2 回目の計画は空 |
| PostgreSQL: 行のある表に、列（`NULL` 可・`DEFAULT` 付き）・`CHECK`・索引を足す | 計画は CLI と同じ（スキーマ名 `public` を付けるかだけが違う）。行は残り、適用後の CLI の差分なし |
| PostgreSQL / 破壊的な差分の判定: 列の削除を含む目標スキーマ | `-allow-destructive` なしでは何も流さずに失敗し、付けると流れて CLI の差分なし |
| SQLite: 空の DB に適用 | CLI の差分なし。2 回目の計画は空 |
| SQLite: 行のある表の作り直し（既存の列に `CHECK` を足し、`DEFAULT` を外す） | 計画は CLI と 1 文ずつ同じ。行は残り、`foreign_key_check` の違反なし、適用後の CLI の差分なし |
| SQLite: 既存の行が満たさない `CHECK` での作り直し | 行の写しで失敗し、全体が戻る（元の表と行がそのまま、作りかけの表は残らず、`foreign_keys` は on に戻る） |
| SQLite: 外部キーで参照される表の作り直し | 参照する側の行は残り、違反なし |
| SQLite: 既存の行が違反する外部キーを足す | `foreign key mismatch` で戻る。CLI は URL に `_fk=1` がないと外部キーの強制が外れたまま流し、違反する行を残す。`my` は対象 DB をサーバーと同じ設定で開くので、常に確かめる |

選択肢 2 の最大の利点は開発用 DB が要らないことだが、sqldef に合わせて目標スキーマの列名と `CHECK` を書き換え、上流の不具合が直るのを待つ必要がある。
選択肢 3 は実装が最も小さいが、開発用 DB と準備 SQL の扱いは運用者に残り、イメージの中に別の実行ファイルの版を持ち続ける。

## Open Questions / 要検証

なし。
