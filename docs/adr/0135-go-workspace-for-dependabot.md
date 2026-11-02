# ADR-0135: Dependabot の Go の更新を go.work でまとめる

- Status: Proposed
- Date: 2026-11-02
- Deciders: @rosylilly
- Issue: #1978

## Context（背景・課題）

`integration` のモジュール（`pkg/mykit/integration`・`pkg/mybot/integration`）は `replace` でルートのモジュールを取り込む。Dependabot がルートだけを更新すると、これらの `go.mod` が古いまま残り、マージキューの `mykit:warm` が `go: updates to go.mod needed` で失敗する。`tools/agentkit` も、ルートの更新で `scripts/agentkit-deps-check.sh` に落ちうる。

[ADR-0134](0134-dependabot-all-go-modules.md) は全ディレクトリを `gomod` に並べ、`allow: dependency-type: all` で間接依存も上げた。結果は、ビルドを壊す 106 件の更新のグループ（#1966）と、ディレクトリごとに散らばるメジャー更新の PR（#1967〜#1971）だった。間接依存の `github.com/agentable/go-intl` を v0.5.1 に上げると、`github.com/kaptinlin/messageformat-go/mf1` の最新版 v0.8.6（v0.2.17 を要求）がコンパイルできなくなる。

決定者の指示（#1978）: 「go.work 導入で良いのでは」

`dependabot-core` の `go_modules`（2026-11-02 に `file_fetcher.rb`・`file_parser.rb`・`file_updater.rb`・`file_updater/go_mod_updater.rb` を読んで確認）の動きは次のとおり。

- `directory: /` に `go.work` があるとワークスペースとして扱い、`use` のモジュールの `go.mod`・`go.sum` と、あれば `go.work.sum` を取得する。`use` は対象のディレクトリの中だけを指せる（`../` は捨てる）ので、`go.work` はルートに置くしかない。
- `use` の各モジュールで `go get <dep>@<version>` を実行し、`go work sync` のあと、各モジュールで `go mod tidy` を実行する。変更のあったファイルをまとめて 1 つの更新にする。
- 更新の候補は `use` の全モジュールの `require` の和集合で、`replace` されたものは除く。
- 返すファイルは取得したものだけなので、`go.work.sum` をコミットしなければ PR に入らない。`go.work` 自体も PR に入らない。
- GitHub の公式文書と Changelog に `go.work` の記述は見つからない（2026-11-02 に確認。Dependabot の対応ファイルの節と options reference を、ページの末尾を除いて読んだ）。対応は `dependabot-core` #14909（2026-05-05 にマージ）と issue #6012 のコメントだけが根拠で、GitHub の本番の動きは **要検証**。

## Decision Drivers（判断基準）

1. Dependabot の Go の更新が、手を加えなくても 1 つの PR でマージキューを通る
2. 公開する `pkg/mykit`・`pkg/mybot` の `go.mod` と依存が、サーバーから独立したままで正しい（ADR-0093 §1・§2）
3. 検査が、各モジュールを単独で見た結果のままである
4. ツールの版がサーバーの版に引きずられない（`tools/go.mod`）
5. 開発の手順への影響が小さい

## Considered Options（選択肢）

1. **ルートに `go.work` を置き、ルート・`integration`・`tools/agentkit` を入れ、検査は `GOWORK=off` で動かす**
2. **全モジュールを `go.work` に入れる**
   - 全モジュールの版がそろい、`pkg/mykit` にも更新が来る。ただし `go work sync` がサーバーの版を公開する `go.mod` に持ち込む（下の検証）。
3. **`pkg/mykit`・`pkg/mybot` は `go.work` から外し、`directories` に残す**
   - ライブラリにも版更新が来る。ただし `github.com/google/uuid` はサーバーでは間接依存で上がらないので、`pkg/mykit` だけが上げると `integration` が未整理になり、もとの失敗が残る。
4. **`go.work` を置かず、Dependabot の PR に手で `go mod tidy` のコミットを足す**
   - リポジトリの構成も Go の動きも変わらず、最も単純。ただし更新のたびに人か担当のエージェントの手が要り、`tools/agentkit` の分も同じ手間がかかる。これまでこの運用で、#1961 が止まった。
5. **`go mod tidy` のコミットを足すワークフローを置く**
   - 確実だが、書き込みの権限とトークンが要り、`pull_request_target` を使わない方針とも合わない（ADR-0134 の選択肢 3）。
6. **ADR-0134 のまま**
   - ビルドが壊れる。

## Decision（決定）

推奨案は選択肢 1。決定者の判断を待つ。

- `go.work` をルートに置き、`use` を `.`・`./pkg/mybot/integration`・`./pkg/mykit/integration`・`./tools/agentkit` にする。`go.work.sum` はコミットせず、無視する。
- `pkg/mykit`・`pkg/mybot`・`tools` は入れない。`go work sync` はワークスペースの版を各モジュールの `require` に書き戻すので、入れるとサーバーの版が移るため。
- `.github/dependabot.yml` の `gomod` は `directories: [/, /tools]` に戻し、`allow` を消す（既定の直接依存だけ）。`go` のグループで minor と patch を 1 つの PR にする。ADR-0046 の対象のディレクトリの記述と同じになる。
- **`go.work` を Dependabot だけが使う。** `scripts/task.sh`・`scripts/agentkit.sh`・`.github/actions/setup-go`・CodeQL のビルドが `GOWORK=off` にする。各モジュールは単独で検査する。
- `Dockerfile` は変えない。`.dockerignore` は許可リストなので `go.work` はビルドのコンテキストに入らず、`go.mod` だけでビルドする。
- `scripts/go-work-check.sh`（`go:mod-check`）で、`go.work` が読めることと `use` の一覧を検査する。ほかの検査は `GOWORK=off` なので、`go.work` の `go` の行が古くなっても気づかず、Dependabot の更新だけが止まるため。
- `github.com/agentable/go-intl` の `ignore` は足さない。間接依存なので候補にならず、直接依存の更新を全部当てても動かない（下の検証）。逆に、直接依存が新しい版を要求するようになれば、`ignore` では止まらない。
- ADR-0093 §1 の「ルートに `go.work` を置かずに」だけをこの ADR で置き換える。`integration` のモジュールが `replace` で結ぶ形と、`pkg/mykit` がサーバーを require しないことは変えない。

### 検証（2026-11-02、Go 1.27.3）

Dependabot の手順を手で再現した（cooldown は適用していない）。

| 確かめたこと | 結果 |
| --- | --- |
| ルートだけ `golang.org/x/net` を上げて `go mod tidy` | `mykit:warm` が `go: updates to go.mod needed` で失敗（もとの問題を再現） |
| 選択肢 1 で `golang.org/x/net` v0.61.0→v0.62.0 | `go.mod` の変更はルートと `integration` 2 つの x/net の 1 行ずつだけ。`mykit:warm`・`mykit:mod-check`・`mykit:mybot-mod-check`・`agentkit:deps-check`・`go:mod-check` が通る |
| 選択肢 1 で直接依存の minor と patch 17 件を一度に | ルートの `go.mod` で 25 行が変わる。`pkg/mykit`・`pkg/mybot`・`tools` は変わらず、`go-intl` は v0.2.17 のまま。`mykit:warm`・`go:check`・`mykit` の Go の検査と結合テスト・`agentkit:check` が通る |
| 選択肢 2 で、更新なしの `go work sync` と `go mod tidy` | `pkg/mykit/go.mod` の `oapi-codegen/runtime` が v1.7.0→v1.8.0 に上がり、`testify` が間接依存に増える。`pkg/mybot/go.mod` にも `go.yaml.in/yaml/v3` が増える |
| 選択肢 2 で、`pkg/mykit` に `stripe-go` の import を足す | ワークスペースではビルドが通る（`GOWORK=off` なら失敗する）。誤った依存が隠れる |
| 選択肢 1 で、更新なしの `go work sync` と `go mod tidy` | `integration` 2 つの `go.mod` に `lib/pq`・`objx` の間接依存が足されるだけで、再実行しても変わらない。この状態をコミットする |
| `/tools` の手順（親に `go.work` がある状態で `go get`・`go mod tidy`） | `go get` と `go mod tidy` はワークスペースを見ないので通る |
| ワークスペースで `go tool -modfile=tools/go.mod` | `-modfile cannot be used in workspace mode` で失敗する。`GOWORK=off` が必須の理由 |

### 覆す条件

- GitHub の Dependabot の実行で、`integration` が 1 つの PR に入らない、または `go work sync` が失敗する場合は、`go.work` を消して選択肢 4 に戻す。
- `go.work` を Dependabot だけに使う前提が崩れる（素の `go` の案内のほうが負担になる）場合も見直す。

## Consequences（影響）

- 良い影響
  - 直接依存の更新 1 つの PR に `integration` と `tools/agentkit` の変更が入り、手を足さずに整理された状態になる。
  - `pkg/mykit`・`pkg/mybot` の `go.mod` は Dependabot の版更新で動かず、利用者に見える要求の版はサーバーに引きずられない。
  - 間接依存の更新が無くなり、ADR-0134 の大きなグループは出ない。
- 悪い影響・トレードオフ
  - 素の `go` の動きが変わる。ルートでは動くが `-modfile` は拒まれ、`pkg/mykit`・`pkg/mybot`・`tools` では `GOWORK=off` なしに動かない（`pkg/mykit`・`pkg/mybot` の README の例にも付けた）。クローンして試す利用者にも影響する。
  - 素の `go` と検査で、選ばれる版がずれることがある。正は `GOWORK=off` の検査の結果。
  - `pkg/mykit`・`pkg/mybot` には Dependabot の版更新が来ない。依存は手で上げ、そのとき `integration` で `go mod tidy` する。脆弱性は Dependabot Alerts が見る。
  - `tools/go.mod` の `require` はすべて `// indirect`（`tool` のディレクティブの形）なので、直接依存だけの版更新では `/tools` は動かない。ツールの版は手で上げる。これは ADR-0134 より前からの状態で、#1961 も `/` だけだった。
  - Go の版を上げるときは `go.work` の `go` の行も上げる（`scripts/go-work-check.sh` が検査する）。
- 後続の作業
  - 次の Dependabot の実行で、`integration` と `tools/agentkit` が 1 つの PR に入ることを確かめる（要検証）。

## Open Questions / 要検証

- GitHub の本番の Dependabot が、この `go.work` で手で再現した手順どおりに動くか。次の実行で確かめる。
- ツールの版にも Dependabot の更新を出すか（`/tools` だけの別のブロックで `allow` に `tool` の 4 つのモジュールを名指しする案。PR は 1 つ増える）。
