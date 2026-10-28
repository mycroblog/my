# ADR-0041: PostgreSQL のテストを pgmem で動かす

- Status: Accepted（コンテナの PostgreSQL のテストを回す頻度を ADR-0079（CI の検査を Merge Queue に一本化する） が置き換える）
- Date: 2026-10-19
- Deciders: @rosylilly
- Issue: #453

## Context（背景・課題）

PostgreSQL を使うテスト（`internal/adapter/db/postgres`、`internal/adapter/kv/db`、`internal/adapter/queue/db`、
`internal/platform`）は、testcontainers-go で起動した `postgres:18-alpine` のコンテナで動かしている（ADR-0003、ADR-0015）。
その結果、`make go-test` の大半の時間をコンテナとデータベースの準備に使っている。並列のエージェントで負荷が高いと、reaper の起動失敗やタイムアウトでも落ちる（NFR-DEV-1, 2）。

pgmem（MIT）は、PostgreSQL 18.3（PGlite のフォーク）を WebAssembly にし、wasm2go で Go に変換したもの。
テストのプロセスの中で、本物のサーバーとしてメモリ上で動く。spike #453 で確かめたことは次のとおり。

- 4 つのパッケージの全テストが、テストの変更なしで通る。スキーマ（`pg_trgm` を含む）、`FOR UPDATE` / `SKIP LOCKED`、advisory lock、
  READ COMMITTED、デッドロック検出（40P01）も含む
- CI の `go test ./...` は 129 秒から 52〜59 秒になる。`db/postgres` だけなら 114 秒から 23〜28 秒になる
- `CGO_ENABLED=0` で動き、本番のバイナリには入らない
- 一方で、テストのバイナリは 36 MB から 88 MB に増え、キャッシュのないビルドは 20〜40 秒延びる
- テストごとに fork や `Reset` をすると、捨てたデータディレクトリが Go のヒープに残る。GC が回収するまで、1 プロセスで 9 GB を超える

## Decision Drivers（判断基準）

- 毎 PR の検査の速さと安定性（Docker に頼らない）
- 本物の PostgreSQL との互換性（ADR-0003 の契約テストの意味を保つ）
- 並列のエージェントと CI の runner の、メモリの上限
- 依存の条件（ライセンス、cgo、3 日ルール、保守の状況）

## Considered Options（選択肢）

1. 今のまま（testcontainers-go。テンプレートのデータベースを複製する）
2. pgmem のサーバーをテストのバイナリごとに 1 つ起動し、テンプレートのデータベースを複製する（`CREATE DATABASE ... TEMPLATE`）
3. pgmem のスナップショットから、テストごとに fork する（`Reset` で戻すものも含む）

## Decision（決定）

選択肢 2 を採る。

- PostgreSQL を使うテストは、テスト用の共通のヘルパー `pkg/testkit/pgtest` でサーバーとデータベースを得る
  - サーバーはテストのバイナリごとに 1 つで、最初にデータベースを求められたときに起動する
  - テストごとのデータベースは、スキーマを当てたテンプレートの複製（`CREATE DATABASE ... TEMPLATE`）で作る
  - サーバーは環境変数で選ぶ。`TEST_POSTGRES_URL` を指定するとその実サーバー、指定しなければ `TEST_POSTGRES` が `pgmem`（既定）なら pgmem、`container` なら testcontainers-go のコンテナ
- 毎 PR の契約テスト（`make go-test`、CI の go-test）は pgmem で回し、Docker を前提にしない
- コンテナの PostgreSQL での契約テスト（`make go-test-postgres`）は、main への push と nightly で回す
  - 同じテストを、`TEST_POSTGRES=container` で流す
  - ADR-0003 の「PostgreSQL（コンテナ）で毎 PR」を、この形に置き換える
- スキーマの検査（`make db-check`、CI の schema ジョブ）は、今のとおり本物の PostgreSQL と Atlas で毎 PR に回す
- pgmem のテストごとの fork と `Reset` は使わない
  - 時間は選択肢 2 とほとんど変わらない（CI の `go test ./...` で 52 秒と 59 秒）
  - それなのにメモリは 1 桁増える
- pgmem は、公開から 3 日を過ぎた `v1.18.x` に `go.mod` で固定する。PostgreSQL のメジャーは、本番で使う `postgres:18` に合わせる
- pgmem はテストのヘルパーだけが使い、本番のバイナリ（`cmd/my`）にはリンクしない

## Consequences（影響）

- 良い影響
  - 毎 PR の Go のテストが、約 2 倍速くなる
  - Docker のない手元やエージェントの環境でも、PostgreSQL のテストが飛ばされずに走る
  - reaper とコンテナの起動による失敗がなくなる
- 悪い影響・トレードオフ
  - テストのバイナリが大きくなり、キャッシュのないビルドが遅くなる
  - 公開から間もない、保守者が 1 人の依存を持つ
  - PGlite のフォークと上流の PostgreSQL の差は、main と nightly のコンテナのジョブまで見つからない
- 置き換え
  - ADR-0015 の Contract の行の PostgreSQL の頻度と、ADR-0003 の契約テストの節の PostgreSQL の頻度を、この ADR が置き換える。ほかの部分は両 ADR のまま

## Pros and Cons of the Options（詳細比較・任意）

| | 1. コンテナ | 2. pgmem のサーバーとテンプレート | 3. pgmem の fork / Reset |
| --- | --- | --- | --- |
| CI の `go test ./...` | 129 秒 | 59 秒 | 52〜56 秒 |
| 1 プロセスのピークのメモリ（`db/postgres`） | 0.5 GB とコンテナ | 約 1 GB | 9 GB 超（`GOMEMLIMIT=4GiB` で 4.4 GB、約 2 割遅くなる） |
| Docker | 要る | 要らない | 要らない |
| 本物の PostgreSQL | そのもの | PGlite のフォーク | PGlite のフォーク |

## Open Questions / 要検証

- pgmem の保守が止まったとき、どこまでフォークして持つか、コンテナに戻すか
