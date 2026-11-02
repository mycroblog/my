# ADR-0134: Dependabot の Go モジュールの対象を全モジュールに広げる

- Status: Accepted
- Date: 2026-11-02
- Deciders: @rosylilly
- Issue: #1964

## Context（背景・課題）

[ADR-0046](0046-dependabot-vulnerability-detection.md) は、`gomod` の版更新の対象を `/` と `/tools` と決めている。リポジトリの Go モジュールはほかに `pkg/mykit`・`pkg/mybot`・その `integration`・`tools/agentkit` がある。
`integration` のモジュールは `replace` でルートのモジュールを取り込むので、ルートの更新だけでは `go.mod` が古いままになり、マージキューの `mykit:warm` が失敗する（`go: updates to go.mod needed`、#1961）。これまでは手で `go mod tidy` のコミットを足していた。

決定者の指示: 「mykit などの go.mod が dependabot に見えてないのが原因ぽいので、そっちを直してもらえますか？」

Dependabot の実装（`dependabot-core` の `go_modules`、2026-11-02 に確認）では、リポジトリの中を指す相対パスの `replace` は置き換えず、`go mod tidy` を走らせる。`allow` の `dependency-type: all` は `gomod` で間接依存も対象にする（[Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)、2026-11-02 に確認）。

## Decision Drivers（判断基準）

1. 依存更新の PR が、手を加えなくてもマージキューを通る
2. 全モジュールの版がそろう
3. PR の数を増やさない

## Considered Options（選択肢）

1. **`directories` に全モジュールを足し、間接依存も対象にする**
2. **`directories` に全モジュールを足すだけにする**
   - `integration` の依存はほとんど間接依存なので、既定の直接依存だけでは上がらず、失敗が残る。
3. **Dependabot の PR に `go mod tidy` のコミットを足すワークフローを置く**
   - 確実だが、書き込みの権限とトークンが要り、`pull_request_target` を使わない方針とも合わない。

## Decision（決定）

選択肢 1 を採る。`gomod` の 1 つのブロックの `directories` に全モジュールを並べ、`allow` を `dependency-type: all` にする。`groups` は 1 つのブロックの中でディレクトリをまたいで 1 つの PR にまとまる。ADR-0046 のうち、`gomod` の対象のディレクトリの部分だけをこの ADR で置き換える。

## Consequences（影響）

- 依存更新の PR に、間接依存の更新も入り、大きくなる。
- `pkg/mykit`・`pkg/mybot` の依存の更新も提案される。MIT のライセンスの範囲は既存の検査が見る。
- 各ディレクトリの更新は同じ基点から作られるので、`integration` の `go mod tidy` は更新前のルートを見る。全モジュールが同じ最新版に上がれば整合するが、版がずれると `mykit:warm` がまだ失敗する。そのときはこれまでどおり `go mod tidy` のコミットを足す。何度も起きるなら選択肢 3 を見直す。
- 効果は次の Dependabot の実行で確かめる。

## Open Questions / 要検証

- なし
