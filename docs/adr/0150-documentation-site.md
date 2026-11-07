# ADR-0150: ドキュメントサイト（運営者・利用者・開発者向けの公開文書）

- Status: Accepted
- Date: 2026-11-06
- Deciders: @rosylilly
- Issue: #2321, #2331, #2417

## Decision（決定）

覆る条件は §10 に書く。

### 1. 結論

運営者・利用者・API を使う開発者に向けた公開のドキュメントサイトを、このリポジトリの中で Astro Starlight により構築し、静的なファイルとして配信する。

- 原稿はリポジトリの中の Markdown のままにする。サイトのために原稿を別のリポジトリや別の場所へ移さない
- サイトはビルドの道具であって原稿の置き場ではない。サイトのディレクトリに手で書いた本文を置かない（トップページなどサイトの骨組みと、§9 の翻訳を除く）
- 理由: `context-map.yaml` の `userdocs`・`docs:pr-impact`・`agentkit docs inventory`・`go:config-docs`・`docs/lint.yaml` の規則は、
  コードと同じ PR で文書を直すことを強制している（ADR-0000）。原稿をリポジトリの外へ出すと、この保証がすべて切れる

### 2. 置き場所とツールチェーン

- サイトは Bun のワークスペース `app/docs`（パッケージ名 `@my/docs`、`AGPL-3.0-only`）に置く。Web クライアントと同じ Bun・TypeScript・`bun fmt` を使い、
  新しい言語のツールチェーンを足さない
- 依存は Astro・`@astrojs/starlight`・§5 の OpenAPI 用プラグインと、その周りの小さなパッケージ（Markdown の処理系 `@astrojs/markdown-satteri`、見出しのアンカーを作る `github-slugger`、リンクの検査と `llms.txt` のプラグイン）に限る。
  いずれも MIT か ISC であり、AGPL-3.0 と両立する（要検証: 採用時の版のライセンス）
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
| プラグインの作者向け | 新設の `docs/plugins/`。`overview.md` のほかは `api/plugin/` の `.proto` のコメントから生成する（`tools/protodoc`） | 載せる。API の開発者向けとは別の章にする |
| 貢献者向け | `CONTRIBUTING.md`、`docs/agents/`、`docs/reference/commands.md` | 載せない。GitHub で読む |
| 設計文書 | `docs/adr/`、`docs/architecture/`、`docs/requirements/` | 載せない。v0 の間は日本語であり、利用者と運営者の文書は設計文書を引用しない規則（AGENTS.md）と揃える |

- サイトに載せる文書は、`docs/lint.yaml` で利用者・運営者向けの規則（設計文書と Issue を引用しない、`maintainer-terms` を含まない）が掛かるものに限る。
  新設の `docs/guide/`・`docs/developers/`・`docs/plugins/` もその規則の対象に加える。`docs/plugins/` の生成物も同じ規則に掛かるので、`.proto` のコメントもこれに従う

### 4. 原稿の取り込み

- ビルドの前段で、§3 の原稿を Starlight のコンテンツのディレクトリ（`app/docs/src/content/docs/`）へ写す。写したものは生成物であり、git に入れない
- 写すときに次を変える。原稿そのものは GitHub でもそのまま読める形を保つ
  - 先頭の `# 見出し` を frontmatter の `title` にする
  - 原稿どうしの相対リンク（`../reference/configuration.md#voice`）を、サイトの URL へ書き換える。リンクと `#anchor` が解決するかは引き続き `agentkit docs lint` が検査する
  - サイトに載せない文書へのリンクは、`blob/<ref>/...` の GitHub の URL へ書き換える
- サイドバーは `docs/README.md` の一覧の順と見出しから作る。目次を二か所に持たない
- 写しの手順は `app/docs` の中の小さなスクリプトに置く。Starlight の `docsLoader` は `src/content/docs/` を読む前提であり（要検証）、
  Astro の `glob()` ローダーで `docs/` を直接読む方法は Starlight のルーティングとサイドバーで動くか確かめていないためである（Open Questions）

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
- サポート中のマイナー版、サポートを終えたマイナー版、`next` を載せる。サポートを終えた版は消さず、全ページに「サポート終了」の帯と `/latest/` の同じページへのリンクを付けて残す。
  古い版を動かし続ける運営者が、手元の版の文書を読めるようにするためである

### 7. 配信

- 配信先は Cloudflare Workers の静的アセットとする。UI Preview（`ui-preview-deploy.yml`、ADR-0079 §14〜§16）がすでに同じアカウントと仕組みを使っており、
  シークレットと運用の手順を増やさない。PR ごとの Preview も同じ仕組みで出せる。サーバーの配信先として Cloudflare Workers を対象にしないこと（AGENTS.md）とは別の話である
- PR が §3 の原稿か `app/docs` を変えたとき、UI Preview と同じ形で `/next/` だけのビルドを PR の Preview に置く。PR のビルドは信頼しないデータとして扱い、配信の側では実行しない（`ui-preview-deploy.yml` と同じ）
- 独自ドメインは §11 の `mycroblog.dev` とする
- `main` への push で `/next/` を更新し、リリース（`release.yml`）で `/latest/` とそのマイナー版を更新する。リリースの手順は `docs/agents/releasing.md` に足す
- Merge Queue の `check` は、`app/docs` か §3 の原稿が変わったときにサイトをビルドし、ビルドが通ることを検査する（ADR-0079 の変更の文脈に足す）
- 検索は Starlight の組み込み（Pagefind）を使い、外部の検索サービスを使わない
- 解析のスクリプトやトラッカーを置かない

### 8. 利用者向けガイド

- 利用者向けガイド（Web クライアントの使い方、公開範囲、通報、アカウントの移転など）を `docs/guide/` に新設する。最初は英語で書く
- サーバーごとの規約や案内は各サーバーの運営者が自分のサーバーで出すものであり、このサイトには `my` そのものの使い方だけを書く
- Web クライアントの中のヘルプ（画面の説明・ツールチップ）は `locales` で多言語化する既存の仕組みのままとし、このサイトはその補足とする

### 9. 言語

英語の原稿を正とし、その翻訳をエージェントが書いてリポジトリに置く。人の手で訳し続けるには原稿の量が多すぎ（2026-11 時点で約 1.2MB）、
翻訳の基盤も使わない（ADR-0009 §8）ためである。最初の言語は日本語とする。

- 訳す範囲
  - `docs/guide/` のすべて、`docs/operations/` のすべて、トップページ（`app/docs/src/content/docs/index.mdx`）を訳す
  - `docs/reference/`（設定・コマンド・メトリクスなどの名前が大半で、一部は生成物）と `docs/developers/`（クライアントの開発者が英語で読む）は訳さない。
    ロケールのそのページは Starlight の代替（英語のページと「まだ翻訳がない」旨の案内）とする
  - サイドバーの見出しは、ページの見出しは訳したページの題から、区分の見出しは `app/docs/i18n/<locale>/sidebar.json` から付ける
- 置き場所: `app/docs/i18n/<locale>/` に、原稿の `docs/` 以下と同じ道で置く（`docs/guide/posting.md` の訳は `app/docs/i18n/ja/guide/posting.md`）。
  `docs/` の外に置くので、`docs/` を英語だけにする規則（AGENTS.md の Documentation）はそのままである。
  §4 の写しの手順がロケールのページ（`src/content/docs/<locale>/`）を組み立てる。Starlight の i18n は、英語をルートのロケール、訳を `/<locale>/` とする
- 翻訳の単位と古さの追跡
  - 原稿を見出しごとの節に分け、訳した節の前に、その節（英語の見出しのアンカー）と訳したときの英語の本文のハッシュを書いた印を置く（`<!-- source: <アンカー> <ハッシュ> -->`）。
    トップページ（MDX）は全体をひとつの節とし、frontmatter の最初の行にハッシュを書く
  - 英語の本文が変わった節は古い訳として扱う。前の訳をそのまま表示し、ページの先頭に古い節を英語のページへのリンクとともに並べる。訳のない節は英語のまま表示し、同じく並べる
  - 訳し直すのは、ハッシュが変わった節と訳のない節だけである
  - 古い訳と訳のない節は CI を落とさない。英語の原稿を直す PR に翻訳の作業を課さないためである
- 見出しのアンカー: 訳したページの見出しには、英語の見出しのアンカーを付ける（Markdown の見出しの属性 `{#id}`）。原稿のリンクと `#anchor` は訳でもそのまま使え、
  リンクの検査（`starlight-links-validator`）もロケールのページで同じに効く
- 誰がいつ訳すか
  - エージェントが手順書（`docs/agents/translating-docs.md`）に従って訳す。手順書はエージェントの種類を問わない。Claude Code にはこれを包むスキルを置く
  - 訳す作業は手で始める。CI・ワークフロー・ビルドから翻訳の API を呼ばない。シークレットを増やさず、ビルドを再現できるままにするためである
  - リリースの手順（`docs/agents/releasing.md`）に、タグの前に訳し直す手順を置く。ただしリリースの関門にはしない。版ごとのサイトはタグの作業ツリーからビルドするので（§6）、タグの時点の訳がその版で公開される
- 道具: `app/docs/scripts/translations.ts`（`scripts/task.sh docs:site-translations`）が、訳すべき節の一覧（`status`）、節ごとの英語の本文と印（`todo`）、
  原稿が引用する UI の用語とロケールのカタログ（`locales/<locale>/`）での訳（`glossary`）、訳の検査（`check`）を出す。
  §4 の写しの手順と同じモジュール（`app/docs/src/translations.ts`）で節を読むので、両者の節の分け方は一致する
- 訳の検査（`docs:site-translations-check`、サイトの文脈の `check` の一部）
  - 印が読めること、節が英語と同じ深さの見出しで始まること、原稿のどの文書も訳していないファイルがないことを確かめる
  - 古くない節については、コードブロック・コード片・リンク先が英語の本文と同じであることを確かめる。設定のキー・コマンド・リンクを訳で壊さないためである
- UI の用語: 原稿は UI を英語の文言で引用する（"Write a post"）。訳はロケールのカタログの同じキーの文言（「投稿を書く」）を使う。`glossary` がその対応を出す
- 表示
  - 訳したページにはすべて、機械翻訳であり英語のページが正である旨の帯を、英語のページへのリンクとともに付ける
  - 法的文書のテンプレート（`docs/operations/privacy-policy-template.md`・`docs/operations/processing-record-template.md`）の訳には、使う前に法律の専門家の確認が要る旨を加える
  - 帯と案内の文言は、サイトの UI 文字列（`app/docs/src/content/i18n/`）として英語と各ロケールで持つ
- 人による確認は記録しない。訳を直した人の手直しは、その節を訳し直すと上書きされうる
- 訳は生成物ではない（ADR-0081）。生成器が同じ入力から同じ出力を作るものではなく、エージェントが書いてコミットする原稿の一種として扱う。
  ただし `agentkit docs lint` の対象から外す。リンクは英語の原稿の位置からの相対で書かれており、そのリンク先は訳の検査が英語と同じであることを保証する
- 言語を足すときは、`translations.ts` のロケールの一覧と `astro.config.ts` の `locales` に足し、サイトの UI 文字列を足して訳す

### 10. 覆す条件

- Starlight の取り込み（§4）が、原稿をリポジトリに置いたままでは保てないと分かったとき（生成物の写しでは足りず、原稿を `app/docs` に移す必要が出たとき）
- Cloudflare Workers の静的アセットの無料枠や条件が変わり、費用がかかるようになったとき。GitHub Pages に移す
- `starlight-openapi` が `api/openapi.yaml` の OpenAPI 3.1 の部分集合を描画できないとき。Redocly か Scalar の静的出力を別のページとして置く

### 11. 公開の場所と v0.1.0 との関係

- サイトは `mycroblog.dev` の根に置く。トップページがプロジェクトの紹介を兼ね、版は `https://mycroblog.dev/latest/`・`/v0.1/`・`/next/` のように道の先頭で分ける
- このサイトの作業はすべて `v0.1.0` のタグの前提条件とする。運営者が v0.1.0 で実運用を始めるとき（`00-vision.md` の M5）、版の合った文書を公開の場で読めるようにするためである

## 経緯

v0.1.0 の公開（M5、`00-vision.md`）に向けて、運営者向けの文書（`docs/operations/`・`docs/reference/`）は揃いつつあるが、読む場所は GitHub の Markdown しかない。
API の仕様は `api/openapi.yaml` として公開できる形にあるが、読める形で公開されていない。利用者向けの文書はまだない。

このリポジトリにはすでに、文書をコードと同じ PR で直す仕組み（ADR-0000、`agentkit docs`）がある。足りないのは公開の場であって原稿の管理ではない、という整理から、
原稿をリポジトリに残してビルドと配信だけを足す形にした。生成器は、i18n の組み込み・OpenAPI のプラグイン・静的な検索があり、
クライアントと同じ Bun と TypeScript で動く Astro Starlight を選んだ。

サイトを公開したあと、英語以外の読み手、とくに日本語の運営者と利用者に向けた訳が要るとなった（#2417）。原稿は運営者向けだけで約 780KB あり、
人が訳して保ち続けるのは難しい。そこで、英語を正としたままエージェントが節ごとに訳し、原稿が変わった節だけを訳し直す形にした。
訳す範囲、置き場所、手で始めること、古い訳を表示して案内すること、確認を記録しないこと、法的文書のテンプレートの扱い、リリースとの関係は、決定者が決めた。

## 採らなかった案

- 翻訳のプラットフォーム（Weblate・Crowdin）で文書を訳す: 翻訳者がコードを触らずに貢献できる。しかし ADR-0009 §8 で翻訳の基盤を使わないとしており、長い文章の訳をレビューし続ける人手もない
- ビルドや CI で翻訳の API を呼ぶ: 訳が常に新しい。しかしビルドが再現できなくなり、配信と PR の Preview にシークレットが要り、ビルドのたびに費用がかかる。
  古い訳を表示して案内する形（§9）で足りるため採らない
- 英語の原稿を直す PR で訳も直すことを CI で強制する: 訳が常に新しい。しかし文書を直すすべての PR に翻訳の作業と道具が要る
- 文書ごと（ファイルごと）に古さを追跡する: 道具が簡単になる。しかし 1 行の修正で長い文書（`docs/operations/running.md` など）を丸ごと訳し直すことになる
- 人による確認を節ごとに記録し、確認済みの節を守る: 訳の質を上げられる。しかし運用の文書のすべてを訳すと確認の量が多く、記録の仕組みも要る。機械翻訳の帯で足りるとした
- 訳を `docs/ja/` や原稿の隣（`running.ja.md`）に置く: GitHub でも訳を読める。しかし `docs/` を英語だけにする規則と `docs/lint.yaml` の各規則に例外が要り、原稿の一覧も二重になる

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
  - 訳は英語の原稿より遅れうる。古い節は案内とともに表示され、訳し直すまで古いままである
  - 機械翻訳の訳は誤りを含みうる。帯で英語のページが正であることを示す
- 後続の作業（Issue 化する）
  - `app/docs` の骨組み、原稿の取り込み、サイドバー
  - API リファレンスと `docs/developers/`
  - 版のビルドと配信（`main` と `release.yml`）
  - Merge Queue の検査への組み込み
  - `docs/lint.yaml` に `docs/guide/` と `docs/developers/` を足す
  - 利用者向けガイドの執筆
  - README と `docs/README.md` からサイトへの案内
  - `docs/operations/` の日本語訳（文書が多いため、`docs/README.md` の区分ごとなどに分けて進める）

## Open Questions / 要検証

- Starlight の `docsLoader` が `src/content/docs/` の外を読めるか、Astro の `glob()` ローダーで Starlight のルーティングとサイドバーが動くか
  （[Starlight Manual Setup](https://starlight.astro.build/manual-setup/)、[Astro Content Loader API](https://docs.astro.build/en/reference/content-loader-reference/)。2026-11-06 時点で未確認）。動くなら §4 の写しの手順を省ける
- `starlight-openapi` が、`api/openapi.yaml` の OpenAPI 3.1 の部分集合（`api/redocly.yaml`）を崩れずに描画できるか（[starlight-openapi](https://github.com/HiDeoo/starlight-openapi)。OpenAPI 3.1 に対応とあるが、この仕様では未確認）
- Cloudflare Workers の静的アセットの無料枠のファイル数とサイズの上限が、サポートを終えた版も残すサイトに足りるか（要検証）
