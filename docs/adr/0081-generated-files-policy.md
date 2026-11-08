# ADR-0081: 開発基盤: タスク・生成物・ファイルの構成・整形・依存の更新

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1052, #527, #1603, #1964, #1978

## Decision（決定）

Go が読む生成物だけをコミットする。タスクは go-task で実行する。多くの機能が追記するファイルは機能ごとに分ける。
TypeScript は `bun fmt` で整形する。依存は Dependabot で検知し更新し、Go の更新は `go.work` でまとめる。

### 1. 生成物の扱い

| 生成物 | 扱い | 作る時点 |
| --- | --- | --- |
| `api/openapi.yaml`、`clientapi/*.gen.go` | コミットする（Go が読む） | `task api:generate` |
| sqlc の生成物 | コミットする（Go が読む） | `task go:sqlc` |
| `internal/i18n/messages_gen.go`、`internal/i18n/testdata/golden.json` | コミットする（Go が読む） | `task i18n:gen` |
| `internal/domain/instance/theme_tokens.gen.go` | コミットする（Go が読む） | `task web:tokens-gen` |
| `emoji_table.gen.go`、`ai_crawlers.gen.go` | コミットする（外のデータを固定した版で取り込む。入力がリポジトリにない） | 手動 |
| `docs/reference/configuration.md` | コミットする（Go のテストが生成する人向けの文書。衝突は少ない） | `task go:config-docs` |
| `docs/reference/cli.md` | コミットする（Go のテストが生成する人向けの文書。衝突は少ない） | `task go:cli-docs` |
| `@my/ui` のスクリーンショットの基準画像 | コミットする（派生物ではなく期待値。ADR-0110） | CI のコンテナ |
| `tools/i18n/src/runtime/messages.gen.ts` | **コミットしない** | 型検査・ビルド・テストの前（`deps`） |
| `pkg/mykit/ts/src/schema.gen.ts` | コミットする（TS だけが読むが、配るパッケージの一部。ADR-0093） | `task api:generate` |
| `app/web/src/routeTree.gen.ts` | **コミットしない** | 同上 |
| `app/ui/src/themeTokens.gen.ts` | **コミットしない** | 同上 |
| `server` の疑似ロケール（`locales/{en-XA,ar-XB}/server/`） | コミットする（Go が読む） | `task i18n:gen` |
| `client` の疑似ロケール（`locales/{en-XA,ar-XB}/client/`） | **コミットしない** | 開発の起動・E2E・クライアントのビルドの前 |
| ADR の索引（`docs/adr/README.md` の表） | **持たない**（§3） | — |

- コミットしない生成物は `.gitignore` に入れる。生成のタスクは `sources` と `generates` を宣言する（§4）。
  使う側のタスク（`web:typecheck`・`web:build`・`web:test`・`i18n:test` など）は、生成のタスクを `deps` に持つ。入力が変わっていなければ、生成は飛ぶ。
- `go build` と `go test` は、Go だけで（Bun なしで）動く（ADR-0016）。
- ルートの木（`routeTree.gen.ts`）は、Vite を通さず `@tanstack/router-generator`（MIT）の `Generator` と `getConfig` を直接呼んで作る。
  `vite.config.ts` と同じ設定（`target: "react"`、`autoCodeSplitting: true`）で回すと、200 ms で、コミットしていたファイルとバイト単位で同じものができた（1.167.40、2026-10-23）。版を固定して直接の依存にする。
- エディタの型検査のために、`task generate` を一度回す手順を CONTRIBUTING.md に書く。開発の起動（`task dev`）も生成を `deps` に持つ。
- コミットする生成物の鮮度は、Merge Queue の `server-static`（ADR-0079）で検査する。2 つの PR の入力を重ねると古くなりうるからである。

### 2. 疑似ロケール

Go が整形するのは `client` 以外の名前空間だけである（Go のコードに `client:` のキーの参照はなく、`internal/i18n/testdata/cases.json` の 11 件もすべて `server`）。そこで、名前空間で分ける。

- `server` の疑似ロケールは、Go が読む生成物としてコミットを続ける。`internal/i18n`・`internal/interface/authpages`・`internal/platform` のテストが `i18n.WithPseudoLocales` で読むので、`go test ./...` を Bun なしで回すために要る。衝突は 14 日で各 3 回と少ない。
- `client` の疑似ロケールはコミットしない。読むのは TS だけで、疑似ロケールの衝突（各 36 回）はすべてこちらで起きている。
- `locales.go` の `go:embed` は、Go が読む名前空間（`client` 以外）だけを埋め込む。`client` の疑似ロケールがあってもなくても、Go のビルドとテストは同じに動く。
- 疑似ロケールを Go で実行時に作る案は採らない。FormatJS の疑似ロケールの変換は、`@formatjs/cli-lib` のネイティブのバイナリ（`cli-native-linux-x64`）の中にあり、Go で同じ結果を作るには移植と照合の仕組みが要るからである。

### 3. ADR の索引と番号

- README に表を持たず、運用の規則だけを残す。一覧は `scripts/agentkit.sh adr list`（番号・タイトル・Status。`--status Proposed` で絞る）で見る。
  GitHub では、ディレクトリの一覧とコード検索（`path:docs/adr "Status: Proposed"`）で見る。
- `scripts/adr-new.sh` は、作業ツリー・`docs/adr/aliases.yaml` の廃番・開いている PR（`gh pr list` と `gh pr diff --name-only`）にある ADR の番号の最大に 1 を足して番号を選ぶ。
  それでも重なったときに備え、Merge Queue の `repo` ジョブで番号の重複を検査する。重なったら、後から並んだ PR が番号を付け直す。

### 4. タスクの実行器: go-task

検査・生成・ビルドは go-task（Taskfile）で実行する。MIT ライセンスの Go 製の単一バイナリである（最新版は v3.54.0、2026-10-01。GitHub API で 2026-10-23 確認）。次の性質を使う
（[Dependencies](https://taskfile.dev/docs/guide/dependencies)・[Up-to-date checks](https://taskfile.dev/docs/guide/up-to-date)・[Output](https://taskfile.dev/docs/guide/output)、2026-10-23 確認）。

- `deps` は既定で並列に動く。`--concurrency`（`-C`）で同時の数を絞れる。`--parallel` で、コマンドラインに並べたタスクも並列に回せる。
- `sources` と `generates` で入力と出力を宣言でき、既定の判断はチェックサム（`method: checksum`）で行う。指紋は `.task/` に置く（`.gitignore` に入れる）。
  このリポジトリで測ると、Go の 1,492 ファイル（`**/*.go`）で 0.26〜0.38 秒、`app/` の TS と CSS の 907 ファイルで 0.11 秒だった（go-task 3.54.0、2026-10-23）。
- `includes` で Taskfile を分け、名前空間を付けられる。

### 5. Taskfile の構成

```text
Taskfile.yml              ルート。includes と check・check:changed・generate だけを書く
tasks/
  go.yml                  go:vet・go:lint・go:test・go:build・go:build-tags・go:licenses・go:mod・go:sqlc
  web.yml / ui.yml        web:*・ui:*（typecheck・lint・knip・build・test・ui-test・e2e）
  i18n.yml / api.yml      i18n:*・api:*（生成は sources / generates を宣言する）
  db.yml / docker.yml / docs.yml / agentkit.yml
  emoji.yml / mail.yml / mykit.yml / shell.yml
```

- 1 つのファイルに全部を書かず、領域ごとのファイルに分ける。新しい領域は新しいファイルとして足し、ルートには include を 1 行足すだけにする（§7）。
- `task check` は、すべての領域の検査を `deps` で並列に回す。NFR-DEV-1 はこの `task check` を指す。
- `task check:changed` は、`scripts/changed-contexts.sh` で文脈を選び、その検査を並列に回す。
- `task generate` は、すべての生成（§1）を回す。入力が変わっていない生成は飛ぶので、検査とビルドのタスクは、毎回 `generate` の該当のタスクを `deps` に持てる。
- CI のジョブはタスクにせず、ワークフローのステップで組む（ADR-0079）。

### 6. 実行の設定

- CI では `scripts/task.sh` が `--output interleaved` を付ける。各ステップが 1 つのタスクを動かして自分のログを持つので、接頭辞は要らない。
- **同時の数**: CI では制限しない（runner の 4 vCPU を使い切る）。手元では、`scripts/task.sh` が `TASK_CONCURRENCY` を CPU の数の半分にして呼ぶ。1 台のマシンで 4〜5 個のエージェントが同時に検査を回すからである。
  go-task は、設定ファイル（`.taskrc.yml` の `concurrency`）・環境変数 `TASK_CONCURRENCY`・`-C` の順に後のものを優先する（[Configuration Reference](https://taskfile.dev/reference/config/)、2026-10-23 確認）。手元で `TASK_CONCURRENCY=1` にすると、2 秒のタスク 3 つが 2.1 秒から 6.1 秒になり、直列に戻ることを確かめた。
- **出力**: 手元は `prefixed` にする。
- **失敗**: go-task は、タスクが失敗すると自分の終了コード（201）で終わる。CI と `scripts/task.sh` は `--exit-code`（`-x`）を付け、コマンドの終了コードをそのまま返す。
  並列の `deps` は、1 つが落ちても残りを最後まで回す（既定。`--failfast` は付けない）。1 回の実行で落ちた検査をすべて見せるためである。
- **版**: go-task の版は `tools/go.mod` の `tool` で固定し、`scripts/task.sh` が `bin/task` をビルドして呼ぶ（`scripts/agentkit.sh` と同じ形）。
  ADR と文書の `task <name>` は `scripts/task.sh <name>` を指す。スクリプトは PATH の `task` を使わない。Taskwarrior のコマンドも `task` なので、PATH の `task` が go-task とは限らず、go-task でも版がずれうるからである。
  CI もこのスクリプトで導入する。GitHub の `go-task/setup-task` は使わない（版の固定が `tools/go.mod` と二重になり、手元では使えないから）。
- `Makefile` は持たない。

### 7. 衝突しないファイルの構成: 規則

- 多くの機能が追記する一覧・カタログ・配線・設計文書は、機能（または節）ごとのファイルに分ける。1 つの PR が足すものは、新しいファイルか、その機能のファイルに入るようにする。
- 分けたファイルを束ねる側（索引・include・`embed`）は、ディレクトリを読む形にして、ファイルを足しても束ねる側を書き換えずに済むようにする。
  束ねる側にどうしても 1 行足す必要があるなら（Go の呼び出しなど）、その行は機能の名前の辞書順に並べる。
- 新しい共有のファイルを作るときは、この規則に合うかをレビューで確かめる。

### 8. 衝突しないファイルの構成: 対象

| 対象 | 形 | 束ねる側 |
| --- | --- | --- |
| ロケールのカタログ | `locales/<locale>/<namespace>/<area>.json`。`area` はキーの先頭の区切り（`settings`・`compose` など）。`admin` は 2 つ目の区切りまで（`admin.domain_actions` → `admin/domain_actions.json`） | `tools/i18n` のローダーと `locales.go` の `go:embed` がディレクトリを読む。キーは変えない |
| Web クライアントの設計文書 | `docs/architecture/web-client/<NN>-<slug>.md`（節ごと。`14-admin.md` など） | `web-client/README.md` に節の一覧と、節をまたぐ原則を置く |
| データモデルの設計文書 | `docs/architecture/data-model/<NN>-<slug>.md`（同上） | `data-model/README.md` |
| `internal/platform` の `NewApp` | `internal/platform/setup_<module>.go` ごとに、モジュールの配線を `(a *App) setUp<Module>(...)` にまとめる | `NewApp` はモジュールの `setUp*` を依存の順に呼ぶだけにする |
| スキーマ | `schema/<NN>-<module>/<dialect>.sql`（モジュールのディレクトリは Go のパッケージ。ADR-0153 §3） | Atlas には `scripts/schema-files.sh` が方言のファイルを 1 つのディレクトリに `<NN>-<module>.sql` として写して渡す（`db:check`・`db:lint`）。Go はパッケージ `schema` が各モジュールのパッケージを import し、名前の順に連結する。モジュールを足すときは `schema.go` に 1 行足す |

- カタログの分け方の実測: `en` の `client`（約 3,000 キー）をこの規則で分けると 70 ファイルになり、最大は `settings` の 352 キー、中央値は 30 キーだった。
  14 日間の衝突でカタログの中の変わった行は、`admin/report`・`admin/account`・`admin/accounts`・`account_status`・`admin/appeal` に集まっていた。そのため、`admin` は 2 つ目の区切りまで分ける。
- Atlas Community Edition 1.3.0 は、`file://` にディレクトリを渡すと、中の SQL ファイルを名前の順に dev データベースで実行して desired state を作る。
  下のディレクトリは読まず（`contains neither SQL nor HCL files`）、`schema diff` と `migrate diff` に `--to file://…` を 2 つ渡すと `the provided SQL state must be either a single schema file or a migration directory` で拒む（2026-11-08 に SQLite で確認）。そのため、モジュールごとのディレクトリから 1 つのディレクトリに写して渡す。
  スキーマを `CREATE TABLE` の区切りで 3 ファイルに分けて `schema inspect` で比べると、両方言とも 1 ファイルのときと同じ結果になった。
  名前の順を逆にすると、PostgreSQL では後のファイルの表を参照する索引で失敗した（2026-10-23）。番号の順は、`db-check` でそのまま検査できる。
  モジュールのファイルは自分で作るものだけを使う（外部キーはなく、索引と注釈は同じファイルの表の後に置く）ので、依存するモジュールだけを当てたテストの DB でも通る（`schema_test.go` が各モジュールで確かめる）。
- 翻訳のプラットフォームの候補（ADR-0009）は、どちらも分けたカタログを扱える。Weblate は Component discovery のアドオンが、名前付きのグループ（`language`・`component`）を持つ正規表現でファイルを探し、コンポーネントを自動で足し引きする（[Add-ons](https://docs.weblate.org/en/latest/admin/addons.html)）。
  Crowdin は `crowdin.yml` の `source` に `**` を、`translation` に `%locale%` を書ける（[Configuration file](https://support.crowdin.com/configuration-file/)）。いずれも 2026-10-23 確認。
- 節の参照: agentkit の `show web-client§14.2` と `data-model§4.4` は、節の番号から分けたファイルを引く。
- `AGENTS.md` と機能要件の表（`01-functional.md`）は分けない。規則や要件の追加は決定者の判断を伴い、並行して変わることが少ないからである。

### 9. TypeScript の整形: bun fmt

TypeScript と JavaScript のコードは `bun fmt` で整形し、CI で検査する。

- 範囲は `app/`、`pkg/mykit/ts`、`pkg/mybot/ts`、`tools/i18n` の `*.ts`・`*.tsx`・`*.js`・`*.mjs` である。CSS・JSON（`locales/`）・Markdown・YAML（`api/spec`）は範囲に入れない。
- 設定はリポジトリの根の `bunfig.toml` の `[fmt]` に置く。`bun fmt` は `[fmt]` を持つ一番近い `bunfig.toml` を読むので、各パッケージの `bunfig.toml`（`[install]` だけ）は `[fmt]` を持たない。
  - `printWidth = 120`。ほかは Prettier の既定（二重引用符、セミコロン、`trailingComma: "all"`、`arrowParens: "always"`、`objectWrap: "preserve"`）で、字下げと改行は `.editorconfig` から読む。
    120 は、コメントと文書を折り返している幅で、当時の行の 96.5 % が収まった。整形器はコメントを折り返さないので、広い幅ほど一度の差分は小さくても、コードとコメントの幅が揃う 120 を採る。
  - `sortImports`・`sortTailwindcss`・`sortPackageJson`・`jsdoc` は使わない（既定で切れている）。import の順と Tailwind のクラスは Oxlint と `better-tailwindcss` が受け持つ。
- 整形しないファイルは根の `.prettierignore` に挙げる（`.gitignore` のファイルも読まない）。API の仕様から作る `pkg/mykit/ts/src/schema.gen.ts` と、Oxlint の違反の見本（`app/oxlint/violations/`。各行の `expect:` のコメントが報告される行を指すので、配置が検査の中身である）と、`web:lint-rules` が動く間だけ `app/web/src` に置くその写しを外す。コミットしない生成物（§1）は `.gitignore` にあるので読まない。
- タスクは `web:fmt`（書き込む）と `web:fmt-check`（`bun fmt --check`。整形が要るファイルがあると失敗する）で、範囲の全部を一度に扱う。
  `web:check`・`i18n:check`・`mail:check`・`mykit:check` が `web:fmt-check` を持つので、`check:changed` はどのパッケージを変えても検査する。CI では、Pull Request の preflight と、Merge Queue の `repo` ジョブで動かす（ADR-0079）。
- TypeScript を変えたら、コミットの前に `scripts/task.sh web:fmt` を回す。整形したファイルの衝突は、解いたあとで `web:fmt` を回し直す。
- 一度の整形は整形だけのコミットにし、根の `.git-blame-ignore-revs` に挙げる。

### 10. 依存の脆弱性検知と更新の提案

NFR-SEC-8 の依存関係の既知の脆弱性の検知は、Dependabot で行う。`govulncheck` は週 1 回の定期実行も含めて持たない。

- **検知**: リポジトリの Dependabot Alerts（vulnerability alerts）で、Go モジュール（`go.mod`・`tools/go.mod`）、Bun（`app/bun.lock`・`tools/i18n/bun.lock`）、GitHub Actions、`Dockerfile` のベースイメージの既知の脆弱性を検知する。CI のジョブ・タスクとしての脆弱性検査は持たない。
- **更新の提案**: `.github/dependabot.yml` で版更新の PR を週 1 回作らせる。
  ecosystem は `gomod`（`/`・`/tools`。§11）、`bun`（`/app`・`/tools/i18n`。Dependabot は text 形式の `bun.lock` に対応する）、`github-actions`、`docker`（`/` の `Dockerfile` と `/pkg/mybot/examples`）。
- **cooldown**: すべての ecosystem で `cooldown.default-days: 3` を設定し、公開から 3 日未満の版への更新 PR を作らない。公開から 3 日未満の版を入れない決まり（`app/bunfig.toml`・`tools/i18n/bunfig.toml` の `minimumReleaseAge`）と揃える。
  cooldown は版更新だけに効き、脆弱性を直す security update には効かない。
- Dependabot の PR も通常の PR と同じく CI の `check` を通してから人間がマージする。
  `Dockerfile` の Go・Node.js・Bun のタグは `.tool-versions` と一致させる必要がある（`docker:dockerfile-check`）ので、タグの版は `ignore` にして手で上げ、Dependabot は `FROM` の digest だけを上げる。

### 11. Go の更新と go.work

- `go.work` をルートに置き、`use` を `.`・`./pkg/mybot/integration`・`./pkg/mykit/integration`・`./tools/agentkit` にする。`go.work.sum` はコミットせず、無視する。
- `pkg/mykit`・`pkg/mybot`・`tools` は入れない。`go work sync` はワークスペースの版を各モジュールの `require` に書き戻すので、入れるとサーバーの版が移るため。
- `.github/dependabot.yml` の `gomod` は `directories: [/, /tools]` にし、`allow` を持たない（既定の直接依存だけ）。`go` のグループで minor と patch を 1 つの PR にする。
- **`go.work` を Dependabot だけが使う。** `scripts/task.sh`・`scripts/agentkit.sh`・`.github/actions/setup-go`・CodeQL のビルドが `GOWORK=off` にする。各モジュールは単独で検査する。
- `Dockerfile` は変えない。`.dockerignore` は許可リストなので `go.work` はビルドのコンテキストに入らず、`go.mod` だけでビルドする。
- `scripts/go-work-check.sh`（`go:mod-check`）で、`go.work` が読めることと `use` の一覧を検査する。ほかの検査は `GOWORK=off` なので、`go.work` の `go` の行が古くなっても気づかず、Dependabot の更新だけが止まるため。
- `github.com/agentable/go-intl` の `ignore` は足さない。間接依存なので候補にならず、直接依存の更新を全部当てても動かない（下の検証）。逆に、直接依存が新しい版を要求するようになれば、`ignore` では止まらない。
- `integration` のモジュールが `replace` で結ぶ形と、`pkg/mykit` がサーバーを require しないことは変えない（ADR-0093 §1）。
- Dependabot の動き（`dependabot-core` の `go_modules` の `file_fetcher.rb`・`file_parser.rb`・`file_updater.rb`・`file_updater/go_mod_updater.rb`、2026-11-02 に読んで確認）:
  - `directory: /` に `go.work` があるとワークスペースとして扱い、`use` のモジュールの `go.mod`・`go.sum` と、あれば `go.work.sum` を取得する。`use` は対象のディレクトリの中だけを指せる（`../` は捨てる）ので、`go.work` はルートに置くしかない。
  - `use` の各モジュールで `go get <dep>@<version>` を実行し、`go work sync` のあと、各モジュールで `go mod tidy` を実行する。変更のあったファイルをまとめて 1 つの更新にする。
  - 更新の候補は `use` の全モジュールの `require` の和集合で、`replace` されたものは除く。返すファイルは取得したものだけなので、`go.work.sum` をコミットしなければ PR に入らない。`go.work` 自体も PR に入らない。
  - GitHub の公式文書と Changelog に `go.work` の記述は見つからない（2026-11-02 確認）。対応は `dependabot-core` #14909（2026-05-05 にマージ）と issue #6012 のコメントが根拠である。本番の動きは #1984 と #2110 で確かめた（ルート・`integration` 2 つ・`tools/agentkit`・`tools` の `go.mod` と `go.sum` が bot のコミット 1 つの PR に入る）。

検証（2026-11-02、Go 1.27.3）。Dependabot の手順を手で再現した（cooldown は適用していない）。

| 確かめたこと | 結果 |
| --- | --- |
| ルートだけ `golang.org/x/net` を上げて `go mod tidy` | `mykit:warm` が `go: updates to go.mod needed` で失敗（もとの問題を再現） |
| 採った構成で `golang.org/x/net` v0.61.0→v0.62.0 | `go.mod` の変更はルートと `integration` 2 つの x/net の 1 行ずつだけ。`mykit:warm`・`mykit:mod-check`・`mykit:mybot-mod-check`・`agentkit:deps-check`・`go:mod-check` が通る |
| 採った構成で直接依存の minor と patch 17 件を一度に | ルートの `go.mod` で 25 行が変わる。`pkg/mykit`・`pkg/mybot`・`tools` は変わらず、`go-intl` は v0.2.17 のまま。`mykit:warm`・`go:check`・`mykit` の Go の検査と結合テスト・`agentkit:check` が通る |
| 全モジュールを入れて、更新なしの `go work sync` と `go mod tidy` | `pkg/mykit/go.mod` の `oapi-codegen/runtime` が v1.7.0→v1.8.0 に上がり、`testify` が間接依存に増える。`pkg/mybot/go.mod` にも `go.yaml.in/yaml/v3` が増える |
| 全モジュールを入れて、`pkg/mykit` に `stripe-go` の import を足す | ワークスペースではビルドが通る（`GOWORK=off` なら失敗する）。誤った依存が隠れる |
| 採った構成で、更新なしの `go work sync` と `go mod tidy` | `integration` 2 つの `go.mod` に `lib/pq`・`objx` の間接依存が足されるだけで、再実行しても変わらない。この状態をコミットする |
| `/tools` の手順（親に `go.work` がある状態で `go get`・`go mod tidy`） | `go get` と `go mod tidy` はワークスペースを見ないので通る |
| ワークスペースで `go tool -modfile=tools/go.mod` | `-modfile cannot be used in workspace mode` で失敗する。`GOWORK=off` が必須の理由 |

### 12. 覆す条件

- Go が読む生成物（sqlc・API・i18n）の衝突が並行の作業を止めるほど多いなら、生成物を一切コミットしない形を考え直す（生成器を Go に移す。ADR-0016 の覆す条件と合わせて判断する）。
- コミットしない生成物の作り忘れで、手元やエディタの混乱が続くなら、TS の生成物も `go:generate` のように固定の手順で作る仕組みを考える。
- go-task の開発が止まる、またはライセンスが変わるなら、Make に戻す。Taskfile のタスクは名前と依存がそのまま make のターゲットに写せる。
- チェックサムによる飛ばしが原因で、古い生成物で検査が通る事故が起きるなら、生成のタスクを `method: none`（毎回実行）にする。
- 分けた設計文書を読むのに、エージェントが読むファイルの数と手間がかえって増えるなら、節の大きさを見直す。
- `bun fmt` の出力が Bun の版ごとに大きく変わり、Bun を上げるたびに大きな差分が出るなら、版を固定できる Prettier を考え直す。
- `bun fmt` の範囲の外のファイル（CSS・JSON・Markdown・YAML）も揃える必要が出たら、範囲を広げる。
- Dependabot Alerts が使えないホスティング（GitHub 以外への移転など）に移る場合は、`govulncheck` などの検査を CI に戻す。
- 呼ばれないコードの脆弱性の通知が多すぎてトリアージが回らない場合は、呼び出しを解析する検査を定期実行で足す。
- GitHub の Dependabot の実行で、`integration` が 1 つの PR に入らない、または `go work sync` が失敗する場合は、`go.work` を消して、Dependabot の PR に手で `go mod tidy` のコミットを足す形に戻す。
- `go.work` を Dependabot だけに使う前提が崩れる（素の `go` の案内のほうが負担になる）場合も見直す。

## 経緯

関係する要件は NFR-DEV-1・NFR-DEV-2・NFR-SEC-8・NFR-I18N。

並行の作業（workflow.md §9）では、14 日で PR のマージ 235 件に対し、ブランチに main をマージしたコミットが 138 件あった。衝突した PR は Merge Queue から外れ、CI が最初から回る（ADR-0079）。
衝突の多かった生成物は `messages.gen.ts`（42 回）、`client` の疑似ロケール（各 36 回）、ADR の索引（16 回）で、手書きのファイルは `web-client.md`（46 回）、`client.json`（各 36 回）、`app.go`（23 回）、`data-model.md`（22 回）、`schema/*.sql`（各 14 回）だった。
GitHub はサーバーでのマージにカスタムのマージドライバを使わないので、形式を工夫しても行が近ければ衝突は避けられない。そこで、生成物を Go が読むものに絞り（§1〜§3）、手書きのファイルを分けた（§7、§8）。`go build` を Bun なしで動かす ADR-0016 の基準は守る。

`Makefile`（418 行、ターゲット 68 個）は文脈の中を直列に回し、更新時刻の判断は `git checkout` で崩れ、生成の要否を判断できなかった。依存を表せて並列に回せ、中身で判断できる実行器として go-task に移った（§4〜§6）。
当初は CI のジョブも `task ci:<job>` で組んだが、検査ごとの時間とログが見えないので、CI はワークフローのステップに移した（ADR-0079）。

依存の検知は当初、ADR-0019 §10 の `govulncheck` を CI と日次実行で回していた。呼び出しグラフの解析の計算コストが高く、Go 以外を見られないので、Dependabot に移した（§10）。

Bun 1.4.4（2026-10-22 公開）が Prettier と互換の `bun fmt` を入れたので、整形器のなかった TypeScript に組み込んだ（§9）。当時 `*.ts` など 148,320 行のうち 120 文字を超える行が 5,216 あり、エージェントの癖で折り返しがばらついていた。一度の整形で変わる量は幅 120 で 786 ファイル（+21,701 / −5,135 行）だった。

Dependabot の Go の更新は、ルートだけを上げると `integration` の `go.mod` が古いまま残り、マージキューの `mykit:warm` が `go: updates to go.mod needed` で失敗した（#1961）。
まず全モジュールを `directories` に並べ `allow: dependency-type: all` で間接依存も上げたが、ビルドを壊す 106 件のグループ（#1966）と、ディレクトリごとに散らばるメジャー更新の PR（#1967〜#1971）になった。間接依存の `github.com/agentable/go-intl` を v0.5.1 に上げると、`github.com/kaptinlin/messageformat-go/mf1` の最新版 v0.8.6（v0.2.17 を要求）がコンパイルできなくなる。
そこで `go.work` でまとめる形にした（§11）。

## 採らなかった案

- すべての生成物をコミットし、形式を工夫する: 生成器なしで何でも動くが、衝突は形式では消えない。
- 生成物を一切コミットしない: 衝突がなくなるが、生成器を Go に移すか Go のビルドに Bun が要り、ADR-0016 と `go install` などの慣習に反する。
- Make のまま `make -j` と `mk/*.mk` にする: 新しい依存がないが、更新時刻の判断は崩れ、並列の出力と失敗の扱いを自分で組むことになる。
- Bazel・Pants・moon・Nx: 完全な依存のグラフとリモートキャッシュを持つが、ルールの保守と学習、CI の準備の重さがこの規模に合わない。
- agentkit にタスクの実行を持たせる: リポジトリの文脈を知っているが、エージェント向けの道具がビルドの要になり、並列とキャッシュを自前で作ることになる。
- ファイルを分けず、共有のファイルに触る issue を同時に走らせない: 何も移さずに済むが、並行の数がそのファイルで決まる。
- 対象のファイルだけ分け、規則にしない: 今の衝突は減るが、新しい一覧やカタログが 1 本のファイルとして生まれ、同じ問題を繰り返す。
- スキーマのファイルを方言ごとのディレクトリに残し、モジュールごとの Go のパッケージには生成した写しをコミットする: ADR-0023 §1 の置き場所を変えずに済むが、同じ SQL が 2 つずつあり、生成の検査が要る。
- スキーマのファイルを実行時に読む: 埋め込まずに済むが、Go のテストの結果のキャッシュは、テストが開いたファイルを大きさと更新時刻で見るので（Go 1.27 の `cmd/go/internal/test` の `hashOpen`、2026-11-08 確認）、チェックアウトのたびに外れる。
- モジュールのパッケージからシンボリックリンクで方言のディレクトリのファイルを指す: 置き場所は変わらないが、Go の埋め込みはシンボリックリンクを拒む（GODEBUG の `embedfollowsymlinks=1` が要る）。
- Prettier: 同じ書き方にできるが、依存を 1 つ足し、JavaScript で 1 回に数秒かかる。
- oxfmt: 同じ書き方にできるが、依存を 1 つ足し、0.x 版（2026-10-28 に 0.75.0）で出力が版ごとに変わりうる（要検証）。
- 整形器を入れない: 何も足さないが、幅と折り返しのばらつきが残る。
- `govulncheck` を週 1 回だけ残す、または PR ごとに回す: 呼ばれるコードだけを判定できるが、計算コストがかかり、Go 以外を見られない。
- `gomod` の `directories` に全モジュールを並べ、間接依存も対象にする: 全モジュールの版がそろうが、ビルドが壊れる。直接依存だけにすると `integration` が上がらず失敗が残る。
- 全モジュールを `go.work` に入れる: `pkg/mykit` にも更新が来るが、`go work sync` がサーバーの版を公開する `go.mod` に持ち込む（§11 の検証）。
- `pkg/mykit`・`pkg/mybot` を `go.work` から外し `directories` に残す: ライブラリにも更新が来るが、`github.com/google/uuid` はサーバーでは間接依存で上がらないので、`integration` が未整理になる。
- Dependabot の PR に手で `go mod tidy` のコミットを足す: 最も単純だが、更新のたびに手が要る。この運用で #1961 が止まった。
- `go mod tidy` のコミットを足すワークフローを置く: 確実だが、書き込みの権限とトークンが要り、`pull_request_target` を使わない方針とも合わない。

## Consequences（影響）

- 良い影響:
  - TS の生成物・`client` の疑似ロケール・ADR の索引による衝突（14 日で約 140 回）と、それらの鮮度の検査がなくなる。手書きのファイルの衝突（約 190 回分）の大部分が機能ごとのファイルに散り、workflow.md §9 の「共有のファイルに触る issue を同時に走らせない」の対象が減る。
  - 設計文書が節ごとのファイルになり、機能のカタログをまとめて消せる。
  - 文脈の中の検査も並列に回り、生成は入力が変わったときだけ走る。
  - TypeScript の差分に整形だけの変更が混ざらず、依存は増えない。
  - 新しい脆弱性の公開は main への変更が無くても通知され、Go 以外の依存も検知と更新提案の対象になる。
  - 直接依存の更新 1 つの PR に `integration` と `tools/agentkit` の変更が入り、`pkg/mykit`・`pkg/mybot` の利用者に見える要求の版はサーバーに引きずられない。
- 悪い影響・トレードオフ:
  - clone した直後や `git pull` の後は、`task generate`（か、生成を `deps` に持つタスク）を回すまで、エディタの TS の型検査が通らない。
  - API の TS の型やルートの木の変化は、PR の差分では見えない。レビューは入力（`api/spec/`・`app/web/src/routes/`）で行う。
  - Go が読む生成物（sqlc・API・i18n の Go 側）の衝突は残る。
  - README を見ても ADR の Status の一覧が分からない。
  - コントリビューターは make の代わりに go-task を使う。`.task/` のチェックサムが手元の状態を持つ。おかしくなったら `task --force` で回し直す。
  - CPU を食う Go の検査（golangci-lint と `go test`）を並列に回しても、4 vCPU では短くなる幅が小さいことがある。CI のジョブは ADR-0079 で静的な検査とテストに分けたままにする。
  - ファイルの数が増え、機能をまたぐ変更では開くファイルが増える。外から張られた文書のリンクは古くなる。
  - スキーマのファイルの番号の順を、外部キーの依存に合わせて保つ必要がある。番号のずれは `db-check` で見つかる。
  - `NewApp` の呼び出しの並びには、まだ 1 行ずつ追記が起きる。
  - `bun fmt` は Prettier と同じく、メソッドの連鎖などで 2 回目の整形で形が変わることがある。`web:fmt` を 2 回回すと落ち着く。GitHub の外の道具は `git config blame.ignoreRevsFile .git-blame-ignore-revs` が要る。
  - Bun の版を上げるときは、`web:fmt-check` が通るかを確かめ、変わったら整形だけのコミットを分ける。
  - Dependabot Alerts はモジュールの版で判定するので、呼ばれないコードの脆弱性も通知される。影響の有無は人間が判断する。通知は CI を失敗させないので、Alerts を見て対応する運用が要る。検知は GitHub の機能に依存し、フォークや別ホスティングでは働かない。
  - 素の `go` の動きが変わる。ルートでは動くが `-modfile` は拒まれ、`pkg/mykit`・`pkg/mybot`・`tools` では `GOWORK=off` なしに動かない（`pkg/mykit`・`pkg/mybot` の README の例にも付けた）。クローンして試す利用者にも影響する。素の `go` と検査で選ばれる版がずれることがあり、正は `GOWORK=off` の検査の結果である。
  - `pkg/mykit`・`pkg/mybot` には Dependabot の版更新が来ない。依存は手で上げ、そのとき `integration` で `go mod tidy` する。脆弱性は Dependabot Alerts が見る。
  - `tools/go.mod` の `require` はすべて `// indirect`（`tool` のディレクティブの形）なので、ツールの版は直接依存の更新の候補にならず、手で上げる。`tools/go.mod` の `// indirect` の行は、ほかのモジュールと同じ依存の更新の PR で動く（#1984、#2110）。
  - Go の版を上げるときは `go.work` の `go` の行も上げる（`scripts/go-work-check.sh` が検査する）。
- 後続: CSS・JSON・Markdown を `bun fmt` の範囲に入れるかを、要るときに Issue にする。

## Open Questions / 要検証

- ツールの版にも Dependabot の更新を出すか（`/tools` だけの別のブロックで `allow` に `tool` の 4 つのモジュールを名指しする案。PR は 1 つ増える）。
