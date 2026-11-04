# ADR-0011: ライセンス: AGPL-3.0 と MIT の範囲

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #12

## Decision（決定）

### 1. リポジトリのライセンス

**リポジトリは AGPL-3.0（SPDX: `AGPL-3.0-only`）とする。例外は `pkg/mykit/`・`pkg/mybot/` と、生成元の `api/spec/` とそのバンドル `api/openapi.yaml` で、これらは MIT とする（ADR-0093 §2）。**

- 改変版をネットワーク越しに提供する事業者にもソース公開を求め、コミュニティへの還元を担保する。Fediverse の主要実装と同じ立場をとる。
- サーバーの実装（`internal/`・`cmd/`・`app/web` など）は AGPL-3.0-only である。

### 2. ライセンス文とヘッダ

- ルートに `LICENSE`（GNU 公式テキスト）を置く。ソースファイルには SPDX ヘッダ（`// SPDX-License-Identifier: AGPL-3.0-only`）を付ける。
- MIT の範囲は、それぞれのディレクトリに MIT の `LICENSE` を置き、ルートの `README.md` のライセンスの節に例外の範囲を書く。SPDX ヘッダは `MIT`（`// SPDX-License-Identifier: MIT`、YAML は `# SPDX-License-Identifier: MIT`）で、生成物の見出しも `MIT` にする。

### 3. ソースの入手先

- AGPL 第 13 条に従い、サーバーは利用者にソースの入手先を示す（NodeInfo の `software.repository`、Web クライアントのフッター / about ページ）。
  ソース URL は設定で変更でき、改変版の運営者が自分のリポジトリを指せるようにする。

## 経緯

OSS として配布する（NFR-DIST-1）ネットワークサービスで、利用者が自由にカスタマイズ・セルフホストすることを想定する。Fediverse の主要サーバー実装（Mastodon、Misskey、GoToSocial、Pleroma/Akkoma）は AGPL 系を採用している。

当初はリポジトリ全体を AGPL-3.0 とした。API クライアントライブラリを出すときに、生成物の元の仕様が AGPL のままだと生成物の扱いが曖昧になるので、ライブラリと仕様の範囲だけを MIT にした（ADR-0093 §2）。

## 採らなかった案

- Apache-2.0（全体）: 改変版をネットワーク越しに提供する事業者にソース公開を求められない。
- MIT（全体）: 同じ理由で還元を担保できない。

## Consequences（影響）

- 依存ライブラリは AGPL-3.0 と両立するもの（MIT / BSD / Apache-2.0 / MPL-2.0 等）に限る。依存ライセンスの検査を CI に入れる（別 Issue）。
- 企業による採用の障壁になり得る点は受け入れる。
- 将来、ほかの部分を MIT / Apache-2.0 で出したくなった場合、外部コントリビュータの同意が必要になる。
  外部コントリビュートを受け入れる前に、DCO / CLA の扱いと合わせて決める（別 Issue）。
