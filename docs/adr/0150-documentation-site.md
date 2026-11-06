# ADR-0150: ドキュメントサイト（運営者・利用者・開発者向けの公開文書）

- Status: Proposed
- Date: 2026-11-06
- Deciders: @rosylilly
- Issue: #2321, #2331

## Decision（決定）

推奨案である。覆る条件は §10 に書く。

### 1. 結論

運営者・利用者・API を使う開発者に向けた公開のドキュメントサイトを、このリポジトリの中で Astro Starlight により構築し、静的なファイルとして配信する。

- 原稿はリポジトリの中の Markdown のままにする。サイトのために原稿を別のリポジトリや別の場所へ移さない
- サイトはビルドの道具であって原稿の置き場ではない。サイトのディレクトリに手で書いた本文を置かない（トップページなどサイトの骨組みを除く）
- 理由: `context-map.yaml` の `userdocs`・`docs:pr-impact`・`agentkit docs inventory`・`go:config-docs`・`docs/lint.yaml` の規則は、
  コードと同じ PR で文書を直すことを強制している（ADR-0000）。原稿をリポジトリの外へ出すと、この保証がすべて切れる

### 2. 置き場所とツールチェーン

- サイトは Bun のワークスペース `app/docs`（パッケージ名 `@my/docs`、`AGPL-3.0-only`）に置く。Web クライアントと同じ Bun・TypeScript・`bun fmt` を使い、
  新しい言語のツールチェーンを足さない
- 依存は Astro・`@astrojs/starlight`・§5 の OpenAPI 用プラグインに限る。いずれも MIT であり、AGPL-3.0 と両立する（要検証: 採用時の版のライセンス）
- ADR-0025 以降が定める Web クライアントのスタックはこのサイトに及ばない。サイトは Web クライアントのコードを import しない
- タスクは `tasks/docs.yml` に `docs:site-build`（ビルド）と `docs:site-dev`（手元のプレビュー）を足す
- `tools/agentkit/context-map.yaml` に `app/docs` の項目を足す

### 3. 原稿の範囲

| 区分 | 原稿 | サイトでの扱い |
| --- | --- | --- |
| 運営者向けガイド | `docs/operations/` | 載せる |
| リファレンス | `docs/reference/`（`commands.md` を除く） | 載せる。`configuration.md` は生成物のまま載せる |
| 利用者向けガイド | 新設の `docs/guide/`（§8） | 載せる |
| API を使う開発者向け | `api/openapi.yaml`、`pkg/mykit`・`pkg/mybot` の README、新設の `docs/developers/` | 載せる（§5） |
| 貢献者向け | `CONTRIBUTING.md`、`docs/agents/`、`docs/reference/commands.md` | 載せない。GitHub で読む |
| 設計文書 | `docs/adr/`、`docs/architecture/`、`docs/requirements/` | 載せない。v0 の間は日本語であり、利用者と運営者の文書は設計文書を引用しない規則（AGENTS.md）と揃える |

- サイトに載せる文書は、`docs/lint.yaml` で利用者・運営者向けの規則（設計文書と Issue を引用しない、`maintainer-terms` を含まない）が掛かるものに限る。
  新設の `docs/guide/` と `docs/developers/` もその規則の対象に加える

### 4. 原稿の取り込み

- ビルドの前段で、§3 の原稿を Starlight のコンテンツのディレクトリ（`app/docs/src/content/docs/`）へ写す。写したものは生成物であり、git に入れない
- 写すときに次を変える。原稿そのものは GitHub でもそのまま読める形を保つ
  - 先頭の `# 見出し` を frontmatter の `title` にする
  - 原稿どうしの相対リンク（`../reference/configuration.md#voice`）を、サイトの URL へ書き換える。リンクと `#anchor` が解決するかは引き続き `agentkit docs lint` が検査する
  - サイトに載せない文書へのリンクは、`blob/<ref>/...` の GitHub の URL へ書き換える
- サイドバーは `docs/README.md` の一覧の順と見出しから作る。目次を二か所に持たない
- 写しの手順は `app/docs` の中の小さなスクリプトに置く。Starlight の `docsLoader` は `src/content/docs/` を読む前提であり（要検証）、
  Astro の `glob()` ローダーで `docs/` を直接読む方法は Starlight のルーティングとサイドバーで動くか確かめていないためである（§11）

### 5. API を使う開発者向けの文書

- クライアント API のリファレンスは、`api/openapi.yaml`（`api/spec/` の bundle）から `starlight-openapi` で生成する。
  公開の文書であることは `api/redocly.yaml` の `no-design-references-*` がすでに保証している
- プラグインのプロトコル（`api/plugin/`）は、`docs/operations/plugins.md` と `docs/developers/` のガイドで説明する。protobuf からのリファレンスの生成は最初の版では行わない
- `pkg/mykit`・`pkg/mybot` の API の詳細は、Go は pkg.go.dev、TypeScript は npm の README と型定義に任せる。サイトには使い始めのガイドとリンクを置く
- `docs/developers/` には、認証（OAuth・サードパーティーのアプリ）、ページング、エラー（RFC 9457）、レート制限、ストリームなど、操作を横断する説明を置く。
  `docs/reference/third-party-apps.md` は内容に応じてここへ移す

### 6. 版

- サイトは版ごとに分ける。`/latest/` は最新のリリースタグ、`/v0.1/` などはそのマイナー版の最新のタグ、`/next/` は `main` を指す
- 版ごとのビルドは、そのタグの作業ツリーからビルドする。古い版の原稿の写しをリポジトリに置かない（「ファイルは現在を書く」原則、ADR-0000）。そのため `starlight-versions` は使わない（採らなかった案）
- 配信のたびに、載せる版をそれぞれのタグからビルドして束ねる。静的アセットの配信は 1 回の配信で全体を置き換えるためである（要検証）。載せる版は少数なので、毎回のビルドで足りる
- 版の切り替えはサイトのヘッダーの小さな選択肢で行う。載せる版の一覧は `app/docs` の中の 1 ファイルに置き、配信の手順とヘッダーが同じものを読む
- 載せる版は、サポート中のマイナー版と `next` に限る。サポートを終えた版は「サポート終了」の帯を付けて残すか消すかを §11 で決める

### 7. 配信

- 配信先は Cloudflare Workers の静的アセットとする。UI Preview（`ui-preview-deploy.yml`、ADR-0079 §14〜§16）がすでに同じアカウントと仕組みを使っており、
  シークレットと運用の手順を増やさない。PR ごとの Preview も同じ仕組みで出せる。サーバーの配信先として Cloudflare Workers を対象にしないこと（AGENTS.md）とは別の話である
- PR が §3 の原稿か `app/docs` を変えたとき、UI Preview と同じ形で `/next/` だけのビルドを PR の Preview に置く。PR のビルドは信頼しないデータとして扱い、配信の側では実行しない（`ui-preview-deploy.yml` と同じ）
- `main` への push で `/next/` を更新し、リリース（`release.yml`）で `/latest/` とそのマイナー版を更新する。リリースの手順は `docs/agents/releasing.md` に足す
- Merge Queue の `check` は、`app/docs` か §3 の原稿が変わったときにサイトをビルドし、ビルドが通ることを検査する（ADR-0079 の変更の文脈に足す）
- 検索は Starlight の組み込み（Pagefind）を使い、外部の検索サービスを使わない
- 解析のスクリプトやトラッカーを置かない

### 8. 利用者向けガイド

- 利用者向けガイド（Web クライアントの使い方、公開範囲、通報、アカウントの移転など）を `docs/guide/` に新設する。最初は英語で書く
- サーバーごとの規約や案内は各サーバーの運営者が自分のサーバーで出すものであり、このサイトには `my` そのものの使い方だけを書く
- Web クライアントの中のヘルプ（画面の説明・ツールチップ）は `locales` で多言語化する既存の仕組みのままとし、このサイトはその補足とする

### 9. 言語

- 最初の版は英語だけで公開する。英語の文書だけを置き日本語の写しを置かない規則（AGENTS.md の Documentation）に従う
- Starlight の i18n は、ルートのロケールを `en` として最初から設定しておく。翻訳を足すときに URL を変えないためである
- 翻訳を入れるかどうか、入れるならどの区分（利用者向けガイドだけか）と、その翻訳の管理（ADR-0009 の翻訳のプラットフォームに載せるか）は §11 で決める

### 10. 覆す条件

- Starlight の取り込み（§4）が、原稿をリポジトリに置いたままでは保てないと分かったとき（生成物の写しでは足りず、原稿を `app/docs` に移す必要が出たとき）
- Cloudflare Workers の静的アセットの無料枠や条件が変わり、費用がかかるようになったとき。GitHub Pages に移す
- `starlight-openapi` が `api/openapi.yaml` の OpenAPI 3.1 の部分集合を描画できないとき。Redocly か Scalar の静的出力を別のページとして置く

### 11. 未決のこと

- ドメイン（`docs.<project domain>` など）
- 翻訳（§9）
- サポートを終えた版の扱い（§6）

## 経緯

v0.1.0 の公開（M5、`00-vision.md`）に向けて、運営者向けの文書（`docs/operations/`・`docs/reference/`）は揃いつつあるが、読む場所は GitHub の Markdown しかない。
API の仕様は `api/openapi.yaml` として公開できる形にあるが、読める形で公開されていない。利用者向けの文書はまだない。

このリポジトリにはすでに、文書をコードと同じ PR で直す仕組み（ADR-0000、`agentkit docs`）がある。足りないのは公開の場であって原稿の管理ではない、という整理から、
原稿をリポジトリに残してビルドと配信だけを足す形にした。生成器は、i18n の組み込み・OpenAPI のプラグイン・静的な検索があり、
クライアントと同じ Bun と TypeScript で動く Astro Starlight を推す。

## 採らなかった案

- 文書を別のリポジトリに置く: サイトの変更をサーバーのリリースと切り離せ、コントリビューターも分けられる。しかし `userdocs` と `docs:pr-impact` による「コードと同じ PR で文書を直す」保証が切れ、版もずれるため採らない
- 原稿を `app/docs/src/content/docs/` に移す: Starlight の標準の形で、写しの手順が要らない。しかし `docs/lint.yaml`・`context-map.yaml`・`docs/README.md` の仕組みと既存のリンクを作り直すことになり、
  GitHub で原稿を読む形も崩れる。§4 の写しの手順が保てないときの退路とする（§10）
- Hugo: Go の単一バイナリでピン留めが簡単で、ビルドも速い。しかしテーマと i18n、OpenAPI の描画を自前で揃える手間が大きい
- MkDocs Material: 機能と見た目が揃っている。しかし Python のツールチェーンをひとつ足すことになる（ADR-0081 のピン留めの対象が増える）
- VitePress: Bun でも動き軽い。しかし OpenAPI の描画と i18n の扱いで Starlight ほどの用意がない（要検証）
- Docusaurus: 版の仕組みと i18n が組み込みである。しかし React と Webpack 系の重い依存を Web クライアントとは別系統で持つことになり、版の仕組みは古い版の写しをリポジトリに置く形である
- `starlight-versions`: Starlight の中で版を扱える。しかし古い版の文書をリポジトリに写して残す形であり、「ファイルは現在を書く」原則に反する。開発も 1.0 前である（要検証）
- GitHub Pages: シークレットが要らず最も手軽である。しかし PR ごとの Preview がなく、UI Preview と別の配信の仕組みをもうひとつ持つことになる。§10 の退路とする
- 各サーバーが自分の文書を `my` から配信する: 運営者の手元の版と一致する。しかし利用者向け以外は運営者がサーバーを立てる前に読むものであり、公開のサイトが要る

## Consequences（影響）

- 良い影響
  - 運営者・利用者・開発者が、GitHub を開かずに版の合った文書を読める
  - 文書の正しさの検査（`agentkit docs`）はそのまま効き、サイトのビルドが壊れたリンクと描画の失敗も拾う
  - API のリファレンスが `api/spec/` の変更とともに更新される
- 悪い影響・トレードオフ
  - 原稿をサイトへ写す手順（見出し・リンクの書き換え・サイドバー）を保守する必要がある
  - Bun のワークスペースに Astro 系の依存が増え、Dependabot の更新が増える
  - Cloudflare のアカウントへの依存が UI Preview に加えてもうひとつ増える
  - 利用者向けガイドは新しく書く量が多い
- 後続の作業（Issue 化する）
  - `app/docs` の骨組み、原稿の取り込み、サイドバー
  - API リファレンスと `docs/developers/`
  - 版のビルドと配信（`main` と `release.yml`）
  - Merge Queue の検査への組み込み
  - `docs/lint.yaml` に `docs/guide/` と `docs/developers/` を足す
  - 利用者向けガイドの執筆
  - README と `docs/README.md` からサイトへの案内

## Open Questions / 要検証

- Starlight の `docsLoader` が `src/content/docs/` の外を読めるか、Astro の `glob()` ローダーで Starlight のルーティングとサイドバーが動くか
  （[Starlight Manual Setup](https://starlight.astro.build/manual-setup/)、[Astro Content Loader API](https://docs.astro.build/en/reference/content-loader-reference/)。2026-11-06 時点で未確認）。動くなら §4 の写しの手順を省ける
- `starlight-openapi` が、`api/openapi.yaml` の OpenAPI 3.1 の部分集合（`api/redocly.yaml`）を崩れずに描画できるか（[starlight-openapi](https://github.com/HiDeoo/starlight-openapi)。OpenAPI 3.1 に対応とあるが、この仕様では未確認）
- Cloudflare Workers の静的アセットの無料枠のファイル数とサイズの上限が、版を重ねたサイトに足りるか（要検証）
- §11 の未決のこと
