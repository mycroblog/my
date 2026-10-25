# Contributing

人間のコントリビュータもコーディングエージェントも、同じルールで作業します。詳細は [AGENTS.md](AGENTS.md) と
[ADR-0014](docs/adr/0014-task-management-and-agent-workflow.md) を参照してください。

## セットアップ

ツールのバージョンは `.tool-versions` で固定しています（[asdf](https://asdf-vm.com/) または [mise](https://mise.jdx.dev/)）。

```sh
asdf install   # or: mise install
scripts/task.sh generate   # エディタの型検査のため、コミットしない生成物を作る
scripts/task.sh check
```

TypeScript だけが読む生成物（`app/web/src/routeTree.gen.ts`・`tools/i18n/src/runtime/messages.gen.ts`・
`app/ui/src/themeTokens.gen.ts`・`client` の疑似ロケール）はコミットしません（[ADR-0081](docs/adr/0081-generated-files-policy.md)）。
それらを読むタスク（型検査・lint・ビルド・テスト・開発サーバー）は生成を先に回すので、clone の直後でもそのまま動きます。
エディタはタスクを通さないので、clone のあとに一度 `scripts/task.sh generate` を回します。入力が変わっていない生成は飛びます。

タスクは [go-task](https://taskfile.dev/) で、`Taskfile.yml` と `tasks/<area>.yml` に書いてあります（[ADR-0080](docs/adr/0080-task-runner-go-task.md)）。`scripts/task.sh` が `tools/go.mod` で固定した版を `bin/task` にビルドして実行します。PATH 上の `task` は使いません。タスクの一覧は `scripts/task.sh --list` で出ます。

`scripts/task.sh check` はコンテキスト（docs・Go・i18n・API・`app/ui`・`app/web`・Docker・agentkit・`pkg/mykit` と `pkg/mybot`）ごとのタスク（`docs:check`・`go:check`・`i18n:check`・`api:check`・`ui:check`・`web:check`・`docker:check`・`agentkit:check`・`mykit:check`）を、各コンテキストの中の検査も含めて並列に実行します（`CHECKS="go web"` で一部だけ）。並列数は `TASK_CONCURRENCY` で制限でき、手元では `scripts/task.sh` が CPU 数の半分を既定にします。
Go のテストは偽サーバー（pstest・`httptest` など）を `127.0.0.1:0` で listen し、ポートは OS が選びます。pgmem への接続は TCP ではなく Unix ソケットを通ります（`pkg/testkit/pgtest`）。
テストを書くときは偽サーバーへの接続を増やさないよう、クライアントを使い回し、Google Cloud の gRPC クライアントには `option.WithGRPCConnectionPool(1)` を渡します（既定は最大 4 接続）。
テストが `listen tcp 127.0.0.1:0: bind: address already in use` で落ちるのは、同じマシンで並列に流したテストの接続（TIME_WAIT は 60 秒残ります）が
エフェメラルポートの範囲（`/proc/sys/net/ipv4/ip_local_port_range`）を使い切ったときです。WSL2 の mirrored ネットワーキングでは、Windows とポートを分け合うため
この範囲が 4,096 ポート（44620〜48715）しかなく、複数の `check` を同時に流すと使い切ります。`TASK_CONCURRENCY` で並列数を下げるか、同時に流す `check` の数を減らします。
mirrored では範囲を広げてはいけません。WSL が Linux に割り当てたこの範囲の外のポートを OS が選ぶと、外への接続（`git`・`gh` など）がタイムアウトします。
変更したコンテキストとその依存先だけを回すには `scripts/task.sh check:changed`（比較の基準は `BASE=origin/main`）を使います。CI も同じ判定（`scripts/changed-contexts.sh`）で変更のないコンテキストのジョブを飛ばし、必須チェック `check` が各ジョブの結果を集めます（[CI](#ci)）。
これらはネットワーク不要のチェック（Markdown lint、ADR 索引、Go の `go vet` / golangci-lint / テスト /
`CGO_ENABLED=0` ビルド / 依存ライセンス検査 / sqlc 生成物の検査、i18n カタログ検査、クライアント API の spec の lint と生成物の検査、`Dockerfile` の Go イメージと `.tool-versions` の一致、Web クライアントの型検査・ビルド・依存ライセンス検査）をすべて実行します。初回のみ Go モジュールとツール、`tools/i18n`・`tools/mail`・`app/` の npm パッケージのダウンロードが必要です。

- Go ツール（go-licenses, sqlc, oapi-codegen）は `tools/go.mod` の `tool` ディレクティブでバージョン固定しています。
- 依存関係の既知の脆弱性は GitHub の Dependabot Alerts で検知し、版更新の PR は Dependabot が作ります（`.github/dependabot.yml`、ADR-0046）。
- Docker Compose 構成のスモークテスト（イメージのビルド、スキーマ適用、管理者作成、WebFinger・Actor・NodeInfo の確認）は Docker が必要なため `scripts/task.sh check` には含めず、`scripts/task.sh docker:smoke` で実行します（CI では Docker の構成・`cmd/`・`internal/platform/` を変える PR と nightly で実行。linux/arm64 のイメージのビルドは Docker の構成を変える PR と nightly）。
- Mastodon・GoToSocial・Misskey とのライブの相互運用テストは `scripts/task.sh check` に含めず、`scripts/task.sh docker:federation-live` で実行します（CI では nightly。手順は [deploy/docker/federation-live/README.md](deploy/docker/federation-live/README.md)）。
- ライセンスは AGPL-3.0-only ですが、`api/spec/`・`api/openapi.yaml`・`pkg/mykit/`・`pkg/mybot/` は MIT です（ADR-0093 §2）。
  これらのファイルの SPDX ヘッダは `SPDX-License-Identifier: MIT`、それ以外の新しいソースは `AGPL-3.0-only` で、`scripts/task.sh api:license-scope`（`api:check` に含む）が検査します。
  MIT の範囲の依存は MIT・Apache-2.0・BSD・ISC などの寛容なライセンスに限ります（GPL 系は入れません）。
- 依存ライブラリのライセンスは AGPL-3.0 と両立するもの（`Taskfile.yml` の `GO_ALLOWED_LICENSES`。`tools/i18n`・`tools/mail`・`app/` の npm パッケージも同じ一覧で検査）に限ります。
  増やす場合は PR で理由を説明してください。

## 進め方

1. GitHub Issue を選ぶ（`agent:ready` は受け入れ基準が明確で着手可能なもの）。
2. ブランチ `<type>/<issue>-<slug>` を切る。`main` に直接 push しない。
3. 触れたコンテキストの check（`scripts/task.sh go:check` など）か `scripts/task.sh check:changed` を通してコミット（Conventional Commits、英語）。共有の設定を変えたときは `scripts/task.sh check`。
4. PR テンプレートに従って PR を作成する。
5. PR の CI が緑になったら、メンテナかオーケストレーター（エージェントを束ねて PR をマージする役）が
   `gh pr merge <PR> -R mycroblog/my --auto --merge` で Merge Queue に入れる。`main` には Merge Queue からだけマージする。
6. 衝突や `check` の失敗でキューから外れたら、`main` を取り込み、コミットする生成ファイル（Go が読むもの。`scripts/task.sh generate` の出力）は
   手で衝突を解かずに作り直し、`scripts/task.sh check:changed` を通して push する（もう一度キューに入れるのは 5. と同じ人）。
   `main` の取り込みは、PR が衝突したときかキューから外れたときだけにする（キューが最新の `main` に重ねて検査する）。

## CI

CI（`.github/workflows/`）は、どこで何を検査するかを次のように分けています（[ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md)）。
`ci.yml` の各ジョブは、ツールを準備したあと、検査を並列のステップで回します（[ADR-0083](docs/adr/0083-ci-steps-not-tasks.md)）。各ステップは検査の名前を持ち、タスクを 1 つ呼ぶ（`scripts/task.sh go:vet`）ので、Actions の画面で検査ごとのログと所要時間が見え、落ちた検査は手元でも同じタスクで回せます。
手元の `scripts/task.sh check` と同じ検査が CI のどこかのステップで回ることは、`scripts/task.sh docs:ci-coverage` が確かめます。

| いつ | ワークフロー | 回すもの |
| --- | --- | --- |
| PR | `ci.yml` | `preflight`。その PR のファイルや lockfile だけで結果が決まる検査で、markdownlint・shellcheck・gofmt・Go と npm の依存ライセンス・Dockerfile と `.tool-versions` の版・Playwright のイメージの版・デザイントークン・CSS の `100vh`。ui の文脈を変えたら `@my/ui` のストーリーテスト（`ui-test.yml`）も回す。Go か `tools/agentkit` を変えたら、golangci-lint を PR が変えた行に絞って回す（`lint`）。Docker の構成（`Dockerfile`・`.dockerignore`・`deploy/`）を変えたら、Docker Compose のスモークテストを回す（`docker-smoke`）。必須チェック `check` が結果を集める。ラベル `ci:full` を付けた PR では Merge Queue と同じジョブも回す（ラベルの付け外しで回し直す） |
| Merge Queue | `ci.yml` | マージグループ（PR を `main` と先に並んだ PR に重ねたもの）で本検査を回す。変えた文脈（`scripts/changed-contexts.sh`）に応じて、go・schema・api・i18n なら `server-static`（tidy・vet・ビルド・Go が読む生成物・両方言のスキーマ）と `server-test`（Go のテスト。PostgreSQL は pgmem）、ui・web・api・i18n なら `client-static`（tsc・Oxlint・knip・Vite のビルド・i18n のカタログ）と `client-test`（Vitest を 2 シャード）、常に `repo`（agentkit・文脈の地図・ADR の番号・CI が `check` の検査をすべて回すこと）。golangci-lint とビルドタグの組み合わせは回さない（遅い検査で回す）。最大 6 ジョブで、スキップしたジョブは成功として数える。`check` が通ればキューがマージする |
| `main` への push | `main.yml` | 検査はせず、PR と Merge Queue が復元するキャッシュだけを作る（[ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md)）。Go のキャッシュには、復元するジョブが読むもの（モジュール・パッケージとツールのビルド・テストのバイナリ・vet と golangci-lint の結果）だけを、同じ設定（`CGO_ENABLED=0`）で順に作る。Go の版・`go.sum`・`tools/go.sum`・日付をキーに空から作り直し、Bun のキャッシュは lockfile をキーにする。どちらもそのキーのキャッシュがまだないときだけ作る |
| リリースの関門・nightly・手動 | `slow-suite.yml` | 遅い検査（[ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md)）。Web の E2E の全マトリクス（8 シャード）、ライブの相互運用テスト（`federation-live.yml`）、本物の PostgreSQL でのテスト、`stream.Bus` の契約のブローカーのコンテナでのテスト、スモークテストと linux/arm64 のイメージのビルド、golangci-lint の全体とビルドタグの組み合わせ、CodeQL のコードスキャン（`codeql.yml`。結果は Security タブに出る）。PR と Merge Queue では回さず、`check` にも入らない |
| リリース（手動） | `release.yml` | 指定したコミットで遅い検査を回し、通ったときだけタグと GitHub Release の下書きを作る |
| nightly（03:00 JST） | `nightly.yml` | `main` の先頭で遅い検査を回す。前に成功した nightly から `main` が動いていなければ飛ばす |

- PR の CI はコンパイル・型・テストを回しません。push の前に手元で `scripts/task.sh check:changed` を通してください。
  Merge Queue で落ちた原因を PR で調べるときや、外からの貢献の PR を早めに確かめるときは、PR にラベル `ci:full` を付けます。
- PR では Web の E2E は自動では回りません。Web クライアントに触れる PR は、手元で `scripts/task.sh web:e2e` を回すか、PR のブランチに対して
  `gh workflow run web-e2e.yml -R mycroblog/my --ref <branch>`（全マトリクスは `-f matrix=full` を足す。Actions タブからも実行できる）で回します。
- `slow-suite.yml`・`nightly.yml`・`web-e2e.yml`・`federation-live.yml`・`postgres-contract.yml`・`stream-contract.yml`・`mail-integration.yml`・`codeql.yml` も
  `workflow_dispatch` で手で回せます。

## スキーマの変更

DB スキーマは方言ごとの目標スキーマファイルを正とし、[Atlas](https://atlasgo.io/) Community Edition で適用します
（[ADR-0023](docs/adr/0023-declarative-schema-management.md)）。マイグレーション SQL は書きません。

- 目標スキーマはモジュールごとのファイル `schema/<dialect>/<NN>-<module>.sql` です（[schema/README.md](schema/README.md)）。
  表を持つモジュールのファイルを、`schema/sqlite/` と `schema/postgres/` の両方で、同じ PR で一緒に編集します。
  テーブルは [論理データモデル](docs/architecture/data-model/README.md) に従い、論理型は [§1.2](docs/architecture/data-model/01-conventions.md#12-論理型) で方言の型に対応づけます。
- 拡張機能・シーケンス・ビュー・トリガーなど Atlas Community Edition が扱えないものは、目標スキーマに書きません。
  PostgreSQL の拡張機能は `schema/postgres.bootstrap.sql`（冪等な準備 SQL）に書きます。
- Atlas は `scripts/install-atlas.sh` がバージョンとチェックサムを固定して `bin/atlas` に入れます（`scripts/task.sh db:atlas`。
  `db:*` タスクが自動で実行します）。`atlas login` が必要な機能は使いません。

```sh
scripts/task.sh db:apply                      # SQLite: 目標スキーマを my.db に適用（開発用 DB はインメモリ）
scripts/task.sh db:diff                       # 適用される SQL を表示するだけ
scripts/task.sh db:reset                      # DB の中身をすべて消してから適用（データが消える）
scripts/task.sh db:check                      # db:reset のあと差分が空であることを検査（CI と同じ）
scripts/task.sh db:check-all                  # SQLite と、db:up のコンテナの PostgreSQL の両方で db:check

scripts/task.sh db:up DB_DIALECT=postgres     # PostgreSQL コンテナ（DB `my` と開発用 DB `my_dev`）を起動
scripts/task.sh db:apply DB_DIALECT=postgres  # db:diff / db:reset / db:check も同様
scripts/task.sh db:down                       # コンテナとデータを削除
```

- 接続先は `DB_URL`（対象 DB）と `DB_DEV_URL`（Atlas が差分の正規化に使う空の DB）で変えられます。
  PostgreSQL の URL は `search_path=public` を付けたスキーマ単位の URL にします。
- コンテナ以外の PostgreSQL に適用するときは、準備 SQL の実行方法も `PG_BOOTSTRAP` で指定します
  （例: `PG_BOOTSTRAP='psql -v ON_ERROR_STOP=1 -q "postgres://..."'`）。開発用 DB にも準備 SQL を先に流しておきます。
- 差分の適用に失敗する変更は、開発用の DB なら `scripts/task.sh db:reset` で作り直します。`v0.1.0` 以降の PR は、データを失わずに直前の `main` から更新できるスキーマ変更にします（[ADR-0087](docs/adr/0087-schema-changes-after-launch.md)）。
- CI は両方言について、空の DB への適用と、適用後に差分が残らないことを検査します（`scripts/task.sh db:check-all`）。

## クライアント API

- パス 1 つ・`components` の項目 1 つを 1 ファイルにした `api/spec/`（OpenAPI 3.1）が正です。`scripts/task.sh api:generate` が Redocly CLI でまとめた `api/openapi.yaml` を作り、生成器はそれを読みます。`api/openapi.yaml` は手で編集しません（[ADR-0066](docs/adr/0066-openapi-split-and-bundle.md)）。
- Go の strict server は oapi-codegen（`tools/go.mod` で固定）で `internal/interface/clientapi/api.gen.go` に生成し、生成物はコミットします（[ADR-0029](docs/adr/0029-api-code-generation.md)）。
- 規約とエンドポイントの追加手順は [docs/architecture/client-api.md](docs/architecture/client-api.md) にあります。

```sh
scripts/task.sh api:generate  # api/spec をまとめた api/openapi.yaml を作り、そこから生成し直す
scripts/task.sh api:lint      # api/spec を Redocly CLI で lint（使える 3.1 の部分集合を含む。設定は api/redocly.yaml）
scripts/task.sh api:check     # api:spec-check（ルートが api/spec のすべてのファイルを列挙する）、api:lint と、バンドルと生成物が最新でなければ失敗する api:generate-check（check に含まれる）
```

## Web クライアント

Web クライアント（`app/web`）は Vite でビルドし、`my serve` がバイナリに埋め込んだビルドと HTML の外枠を配ります
（[ADR-0028](docs/adr/0028-html-shell-link-previews-and-pwa.md)。設計規約は [docs/architecture/web-client/README.md](docs/architecture/web-client/README.md)）。

- `scripts/task.sh web:build` は `app/web/dist` にビルドし、`internal/interface/web/dist` にコピーします。Go はそこを `//go:embed all:dist` で埋め込みます。
  コミットするのはプレースホルダの `.gitkeep` だけなので、Go のビルドとテストに Bun は要りません（そのバイナリはスクリプトのない外枠を返します）。
- `scripts/task.sh go:bin` は `web:build` を先に実行し、Web クライアント入りの単一バイナリ `bin/my` を作ります。Docker イメージも同じビルドを埋め込みます。
  `GO_TAGS` に[ビルドタグ](docs/reference/build-tags.md)を渡すと、使わないアダプタをバイナリから外せます。

開発中は、Vite の開発サーバーを立て、`my serve` にその origin を渡します。ブラウザは `my serve` を開きます。
外枠・ランタイム設定・API・ActivityPub は本番と同じく Go が返し、スクリプトだけを Vite から HMR 付きで読み込みます。

```sh
scripts/task.sh db:apply                       # SQLite の my.db にスキーマを適用
scripts/task.sh web:dev                        # Vite の開発サーバー（http://localhost:5173 固定。生成物を先に作る）
MY_HOST=localhost:8080 MY_DATABASE_URL=my.db MY_KEK="$(openssl rand -base64 32)" \
  go run ./cmd/my serve -dev-vite-origin http://localhost:5173   # 別の端末で。http://localhost:8080 を開く
```

- `-dev-vite-origin` を付けると、外枠はキャッシュされず（`no-store`）、CSP はその origin のスクリプト・WebSocket・Vite が差し込む `<style>` を許す開発用のものになります。
  本番の CSP（nonce なしの `script-src 'self'`）での動作は、`scripts/task.sh go:bin` で作ったバイナリで確認します。
- ランタイム設定の `apiOrigin` は、開発時と、`MY_WEB_ORIGIN` を `host` の `http` の origin（`http://localhost:8080` など）にしたときだけ `http://{host}` になります。

## SQL とデータベースのテスト

- クエリは方言ごとに `queries/<dialect>/*.sql` に書き、[sqlc](https://sqlc.dev/) で Go コードを生成します（[ADR-0003](docs/adr/0003-database-abstraction.md)）。
  設定は `sqlc.yaml`、sqlc のバージョンは `tools/go.mod` で固定しています。生成物はコミットし、`scripts/task.sh check` が古くないかを検査します。

```sh
scripts/task.sh go:sqlc        # sqlc.yaml から生成し直す
scripts/task.sh go:sqlc-check  # 生成物が最新でなければ失敗（check に含まれる）
```

- DB アダプタは `pkg/testkit` のコントラクトテスト（例: `testkit.UnitOfWorkContract`）を SQLite と PostgreSQL の両方で通します。
  テスト用の DB には目標スキーマ `schema/<dialect>/` のファイルを名前の順に直接流します（空の DB に対する Atlas の適用と同じ SQL になります）。
- PostgreSQL のテストは `pkg/testkit/pgtest` でサーバーを得ます（[ADR-0041](docs/adr/0041-postgres-tests-on-pgmem.md)）。
  既定では、テストのプロセスの中で pgmem（メモリ上で動く PostgreSQL）を起動するので、Docker は要りません。
  テストのバイナリごとにサーバーを 1 つ起動し、テストごとにスキーマを当てたテンプレートのデータベースを複製します。
- 本物の PostgreSQL で流すときは、次のどちらかを使います。CI は nightly で `scripts/task.sh go:test-postgres` を流します。
  - `scripts/task.sh go:test-postgres`: `TEST_POSTGRES=container` で testcontainers-go のコンテナ（`TEST_POSTGRES_IMAGE`、既定は `postgres:18-alpine`）を起動します。
    Docker がないと、`TEST_POSTGRES_REQUIRED=1` なので失敗します。
  - `TEST_POSTGRES_URL=postgres://...`（DB を作成できるユーザー）: 既存のサーバーを使います
- Valkey のテスト（`kv/valkey` など）は `pkg/testkit/valkeytest` でサーバーを得ます。Docker があれば testcontainers-go で
  `valkey/valkey` のコンテナ（`TEST_VALKEY_IMAGE` で変更可）を起動し、なければ飛ばします。`TEST_VALKEY_URL=valkey://...` は既存のサーバーを使います。
  CI の `go:test` は `TEST_VALKEY_REQUIRED=1` で流すので、Docker がないと失敗します。
- Amazon SNS / SQS のアダプタ（`stream/sns`）のテストは、`pkg/testkit/awstest` のメモリの偽物で流します。AWS のエミュレータ
  [Floci](https://github.com/floci-io/floci) でも流すときは、次のどちらかを使います。CI は nightly で `scripts/task.sh go:test-stream` を流します。
  - `scripts/task.sh go:test-stream`: `TEST_AWS=container` で testcontainers-go のコンテナ（`TEST_AWS_IMAGE`、既定は `floci/floci:2.1.0`）を起動します。
    Docker がないと、`TEST_AWS_REQUIRED=1` なので失敗します。
  - `TEST_AWS_ENDPOINT=http://localhost:4566`: 起動済みのエミュレータを使います
- Google Cloud Pub/Sub のアダプタ（`stream/pubsub`）のテストは、既定ではプロセスの中の偽サーバー pstest で流すので、Docker は要りません。
  `scripts/task.sh go:test-stream`（`TEST_STREAM=container`）は Pub/Sub エミュレータのコンテナ（`TEST_PUBSUB_IMAGE` で変更可）を起動して同じコントラクトを流し、
  Docker がないと `TEST_PUBSUB_REQUIRED=1` なので失敗します。CI は nightly で流します。
- `mail = smtp` の結合テスト（`internal/adapter/mail/smtp` と `internal/platform` の `Mailpit` を名に持つテスト）は、`pkg/testkit/mailpittest` が
  テストごとに起動する Mailpit のコンテナ（`TEST_MAILPIT_IMAGE` で変更可）を相手にし、`TEST_MAIL=container` のときだけ流れます（`scripts/task.sh go:test-mail`）。
  CI は nightly で流します。

## 設計判断

アーキテクチャに関わる変更は ADR を書きます: `scripts/task.sh docs:adr-new TITLE="..." SLUG=...`。
ADR を `Accepted` にするのはメンテナです。

## 言語

- 設計ドキュメント（ADR・要件）: v0 の間は日本語
- コード・コメント・コミットメッセージ・PR タイトル: 英語
- Issue・PR 本文: 日本語 / 英語どちらでも可

## 翻訳・メッセージ

利用者向けの文言は `locales/<locale>/<namespace>/<area>.json`（FormatJS JSON 形式）に追加し、`en` と `ja` を同時に更新します（[ADR-0009](docs/adr/0009-internationalization.md)）。
キーは、その領域のファイルに書きます（[ADR-0082](docs/adr/0082-conflict-free-file-layout.md)）。領域はキーの先頭の区切りで、`admin` だけは 2 つ目の区切りまでです。
`settings.profile.title` は `locales/en/client/settings.json` に、`admin.report.title` は `locales/en/client/admin/report.json` に入ります。
新しい領域のキーは、そのファイルを新しく作って書きます（ファイルを束ねる側の変更は要りません）。違うファイルに書いたキーは `scripts/task.sh i18n:check` が報告します。
編集したら `scripts/task.sh i18n:gen` で疑似ロケール（`en-XA` / `ar-XB`）と型付きアクセサを再生成し、Go が読むもの（`server` の疑似ロケール・
`internal/i18n/messages_gen.go`・`internal/i18n/testdata/golden.json`）をコミットします。`client` の疑似ロケールと TS のアクセサはコミットしません。
サーバーが描画する画面を疑似ロケールで見るときは `my serve -pseudo-locales` で起動し、ブラウザの言語を `en-XA` / `ar-XB` にします（このフラグがなければ選べません）。
