# ADR-0116: サーバーのロゴとアイコンを SVG で設定する

- Status: Accepted
- Date: 2026-10-28
- Deciders: @rosylilly
- Issue: #1691

## Context（背景・課題）

決定者の指示は「サーバーの管理者が、今あるバナーの画像に加えて、独自のロゴと独自のアイコンを設定できるようにする。どちらも SVG でアップロードする」である。
サーバー単位のテーマ（FR-605、ADR-0059）は色・文字・任意の CSS を扱い、§2 で「ロゴ・ファビコン・PWA のアイコンの差し替えはこの ADR に含めない（画像の扱いは別の Issue で決める）」とした。
この ADR がその決定である。関わる要件は FR-601（PWA）・FR-602（管理画面）・FR-605・NFR-SEC-6（アップロードの安全性）。

今の実装は次のとおり。

- **バナー**: インスタンスの設定 `thumbnail`（about・ランディング・`GET /instance`・NodeInfo の画像）と `preview_image`（リンクプレビューの既定画像、ADR-0028）は、
  どちらも media モジュールにアップロードしたメディアの ID（`KindMedia`）を持つ。media は ADR-0019 §8 の許可リスト（JPEG / PNG / GIF / WebP …）で中身を判定し、**SVG を拒む**。
- **アイコン**: `app/web/public` の `icon.svg`・`icon-maskable.svg` と、そこから `scripts/web-icons.sh`（ImageMagick）で描いてコミットした
  `icon-192.png`・`icon-512.png`・`icon-maskable-512.png`・`apple-touch-icon.png`（180 px）がビルドに入る。
  `internal/interface/web` は、ビルドにあるファイルだけを外枠の `<link rel="icon" type="image/svg+xml">`・`<link rel="apple-touch-icon">` と
  Web App Manifest の `icons` に出す（`build.go` の `appIcons`、`shell.go`、`handler.go`）。インスタンスごとに替える経路はない。
- **ロゴ**: `@my/ui` の `Wordmark` が `app/ui/src/logos` の 3 つの SVG（ライト・ダーク・単色）を `<img>` で出し、`BrandMark` がその横にサーバーの名前を文字で出す。
  サーバーが描画する画面は `server-pages.css` の `.site-logo` が背景画像で同じ SVG を使う。
- **メール**: `app/mail/frame.mjml`（ADR-0086 §2）は外部のフォント・画像を読まず、署名にサーバーの名前を文字で出す。
- **リンクプレビュー**: `og:image` は投稿の画像 → プロフィールの画像 → `preview_image` → Go が描く組み込みの 1200 × 630 の PNG（`preview_image.go`）の順。

制約は次のとおり。

- SVG は文書であり、スクリプト・外部の資源の参照・イベントハンドラーを持てる。ADR-0019 §8 はこれを理由に SVG のアップロードを拒む。
  `<img>` や CSS の背景で読んだ SVG はスクリプトが無効で外部の資源も読まないが、この制限は SVG の URL を直接開いたときや `<iframe>` / `<object>` には当たらない
  （[MDN: SVG as an image](https://developer.mozilla.org/en-US/docs/Web/SVG/Guides/SVG_as_an_image)、2026-10-28 確認）。
- SVG を受け付けない場所がある（下の表）。そこにはラスター画像が要る。
- サーバーは `CGO_ENABLED=0` でビルドする（ADR-0001）。外枠の CSP は `img-src 'self' blob:`（と media のオリジン）で、緩めない（ADR-0019 §9、ADR-0028）。
- 設定はイベント（リビジョン）と投影に分け、問い合わせや制約のあるデータを JSON に入れない（ADR-0020）。管理の変更は監査ログに残す（ADR-0037）。

SVG の対応（どれも 2026-10-28 に一次資料で確認）:

| 使い道 | SVG | 根拠 |
| --- | --- | --- |
| ファビコン（`<link rel="icon">`） | Chrome / Edge 80+、Firefox 41+、**Safari は macOS / iOS とも 26.0 から**。Safari 18 以前は PNG が要る | [Can I use: SVG favicons](https://caniuse.com/link-icon-svg) |
| Web App Manifest の `icons` | 形式の制限はなく、SVG は `sizes: "any"` で書ける | [MDN: icons](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Manifest/Reference/icons) |
| Chrome のインストールの条件 | 192 px と 512 px のアイコンが要る（形式は書いていないので、PNG を必ず出す） | [web.dev: install criteria](https://web.dev/articles/install-criteria) |
| インストール済みの PWA のアイコンの更新 | Chrome は起動時に 24 時間ごとに manifest を確かめる。Android は WebAPK を作り直してアイコンを替えるが、**デスクトップはアイコンの変更に対応していない** | [web.dev: How Chrome handles updates to the web app manifest](https://web.dev/articles/manifest-updates) |
| iOS のホーム画面（`apple-touch-icon`） | **PNG のみ** | [Apple: Configuring Web Applications](https://developer.apple.com/library/archive/documentation/AppleApplications/Reference/SafariWebContent/ConfiguringWebApplications/ConfiguringWebApplications.html) |
| リンクプレビュー（`og:image` / X Card） | OGP は形式を決めていない（`og:image:type` で MIME を示すだけ）。この ADR はリンクプレビューに PNG だけを出す（組み込みの既定画像もすでに PNG）ので、各クローラーの SVG の扱いに依らない | [The Open Graph protocol](https://ogp.me/) |
| メール | Gmail は SVG を PNG に変換して出す（部分対応）。Apple Mail・Outlook は対応と記録。この ADR はメールにも PNG を出す | [Can I email: SVG images](https://www.caniemail.com/features/image-svg/)（最終テスト 2026-09-16） |
| 連合の相手が取るサーバーのアイコン | Misskey は manifest の `icons` の最初の `src` をアイコンに、最後の `<link rel="icon">`（なければ `/favicon.ico`）をファビコンにする | [`FetchInstanceMetadataService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/FetchInstanceMetadataService.ts) |

## Decision Drivers（判断基準）

1. **安全性を下げない**: SVG を受け付けても、サーバーのオリジンでスクリプトが動く経路・外部への取得・解凍爆弾を作らない（ADR-0019）。
2. **どこでも同じ見た目**: ファビコン・ホーム画面・PWA・リンクプレビュー・メールで、管理者が見たものと同じ絵が出る。
3. **`CGO_ENABLED=0` と依存の少なさ**: cgo も、保守の止まった依存も足さない（ADR-0001）。
4. **管理者の手間**: 管理者は SVG を 1 つ用意すればよく、大きさごとの PNG を自分で作らなくてよい。
5. **既存の仕組みに乗る**: ストレージは BlobStore の port（ADR-0005）、データは ADR-0020、権限は `manage_settings`、変更は監査ログ。未設定なら今の my の絵のまま。

## Considered Options（選択肢）

### A. 元の形式

1. **A1: SVG だけ**（決定者の指示）。
2. **A2: SVG と PNG のどちらでも**。PNG は既存の media の検査（ADR-0019 §8）がそのまま使え、サニタイズが要らない。ベクターの絵を持たない管理者も設定できる。
   一方で、PNG の元からはファビコンの SVG・どの大きさでも鮮明なロゴを作れず、形式ごとに表示の経路が 2 つになる。

### B. ラスター画像（PNG）の作り方

1. **B1: 管理者のブラウザーで描く**。管理画面がサニタイズ済みの SVG を `<img>` に読み、`<canvas>` に描いて `toBlob("image/png")` で PNG にし、SVG と一緒にアップロードする。
2. **B2: サーバーが pure Go で描く**。[oksvg](https://github.com/srwiley/oksvg) と [rasterx](https://github.com/srwiley/rasterx)（どちらも BSD-3-Clause で AGPL と両立、cgo なし）。
   長所は API・CLI のどちらから設定しても同じ PNG ができ、クライアントを信用しなくてよいこと。短所は、oksvg が SVG 2 の一部しか読まないこと
   （同梱の `doc/SVG_Element_List.txt` によれば `clipPath`・`mask`・`pattern`・`marker`・`filter`・`text`・`class` と多くのプレゼンテーション属性を扱わない）と、
   最後の push が oksvg 2023-05-29・rasterx 2022-07-30 で保守が止まっていること（GitHub API で 2026-10-28 確認）。
   MIT の [tdewolff/canvas](https://github.com/tdewolff/canvas) は保守されている（最後の push 2026-09-23）が、SVG を読み込める範囲はこの ADR では比べていない（見直す条件を参照）。
3. **B3: 描かない**。SVG だけを出し、SVG を受け付けない場所（Safari 18 以前のファビコン・`apple-touch-icon`・リンクプレビュー・Gmail）は組み込みの my の絵か名前の文字に落とす。

### C. どこに置き、どこから配るか

1. **C1: instance モジュールが BlobStore に置き、本体のオリジンの版付きのパスで配る**（ADR-0059 §5 のテーマのスタイルシートと同じ形）。
2. **C2: media モジュールのメディアにする**（`thumbnail` と同じく設定にメディアの ID を持つ）。media の許可リスト・再エンコード・配信のヘッダー（ADR-0019 §8）と衝突し、
   SVG だけ例外の経路を media に作ることになる。

## Decision（決定）

**A1 + B1 + C1** を採る。

### 1. アイコンとロゴ

- **アイコン**（`icon`）: 正方形のマーク。ファビコン・PWA のアイコン・`apple-touch-icon`・サーバーを四角い絵で表す場所（ランディングとサーバー情報のカードのアバター、Tauri のサーバーの切り替え）に使う。
- **ロゴ**（`logo`）: 横長のワードマーク。アプリの外枠のヘッダー（`BrandMark`）・ランディング・サーバーが描画する画面（`.site-logo`）・メールのヘッダー・リンクプレビュー（下記）に使う。
  ロゴはサーバーの名前を含む絵として扱い、設定されているときは `BrandMark` は名前の文字を横に出さず、ロゴの `<img>` の `alt` をサーバーの名前にする。
- **ダークのロゴ**（`logo_dark`）: 任意。暗い地で使う。ないときは暗い地でも `logo` を使う。アイコンはライトとダークで 1 つ（地の色を管理者が選ぶので、下記）。
- 既存の `thumbnail` / `preview_image`（バナー）はそのまま残す。役割が違う（大きな写真・イラスト）。
- 未設定のものは、今の組み込みの my の絵（`icon.svg` などのビルドのファイル、`Wordmark`）を使う。設定を消すと組み込みに戻る。

### 2. SVG のサニタイズと検査（サーバー）

`internal/domain/instance` の純粋な関数として、標準ライブラリの `encoding/xml` でトークンを読み、許可したものだけから**新しく書き出す**（入力をそのまま通さない）。
保存の API はサニタイズした結果だけを保存し、元のファイルは残さない。

- **拒む（422 の Problem）**: XML として壊れているもの、`<!DOCTYPE>` と実体の宣言（XXE と実体の展開を防ぐ）、ルートが `svg` でないもの、
  `viewBox` がないか数として読めないもの、サニタイズの前で 256 KiB・後で 128 KiB を超えるもの、要素が 5,000 個を超えるもの、入れ子が 32 段を超えるもの、
  アイコンで `viewBox` の縦横比が 1:1 から 1% 以上ずれるもの、ロゴで横 ÷ 縦が 1 未満か 8 を超えるもの、`text` / `tspan` を含むもの（下記）。
- **許す要素**: `svg`・`g`・`defs`・`symbol`・`use`・`title`・`desc`・`path`・`rect`・`circle`・`ellipse`・`line`・`polyline`・`polygon`・
  `linearGradient`・`radialGradient`・`stop`・`clipPath`・`mask`・`pattern`・`style`。そのほかの要素（`script`・`foreignObject`・`image`・`a`・`iframe`・`animate*`・`set`・`filter` と `fe*`・`text` など）は、
  `text` を除いて中身ごと落とし、落としたものを応答で知らせる。
- **許す属性**: 形と変換（`d`・`points`・`x`・`y`・`width`・`height`・`r`・`rx`・`ry`・`cx`・`cy`・`x1`〜`y2`・`fx`・`fy`・`transform`・`viewBox`・`preserveAspectRatio`）、
  プレゼンテーション属性の一覧（`fill`・`stroke` とその派生・`opacity`・`stop-color`・`stop-opacity`・`clip-path`・`mask`・`fill-rule`・`clip-rule`・`color`・`display`・`visibility` など）、
  グラデーションとパターンの属性、`id`・`class`・`style`、`xmlns` / `xmlns:xlink`。`on*` のイベントハンドラーとそのほかの属性は落とす。
- **参照**: `href` / `xlink:href` は `#` で始まる文書内の参照だけを許し、ほかは属性ごと落とす。`url(…)`（属性と CSS）も `url(#id)` だけを許す。
  `data:` の URL は許さない（埋め込みのラスター画像は SVG の利点を消し、中の画像の検査も要るため）。`use` は文書内の参照だけになるが、展開の爆発を防ぐため `use` は 200 個までにする。
- **CSS**（`style` 要素と `style` 属性）: `@import`・`@font-face`・`@namespace`、`url(#…)` 以外の `url(`、`expression(`、`\` のエスケープ、`<` を含むものは拒む（落とすと見た目が変わるため、知らせて拒む）。
  `@media (prefers-color-scheme: dark)` は許す（アイコンをダークで変えたい管理者のため）。CSS を解釈して直すことはしない。
- **文字**: `text` / `tspan` は拒み、「文字をアウトライン（パス）にしてください」と知らせる。文字の形は閲覧者のフォントで変わり、ブラウザーで描く PNG（§3）と SVG で違う絵になるため（判断基準 2）。

### 3. PNG の版は管理者のブラウザーで描く（B1）

SVG を受け付けない場所のために、管理画面が次の PNG を描き、SVG と一緒にアップロードする。

| 版 | 元 | 大きさ | 使い道 |
| --- | --- | --- | --- |
| `favicon-32` / `favicon-48` | icon | 32 / 48 px | Safari 18 以前などのファビコン（`<link rel="icon" type="image/png" sizes>`） |
| `apple-touch-180` | icon | 180 px、不透明 | iOS のホーム画面 |
| `pwa-192` / `pwa-512` | icon | 192 / 512 px | Manifest の `purpose: "any"` とインストールの条件 |
| `maskable-512` | icon | 512 px、不透明 | Manifest の `purpose: "maskable"`（中央 80% の安全域に縮めて置く） |
| `logo-mail` / `logo-dark-mail` | logo / logo_dark | 高さ 96 px（表示は 48 px の 2 倍）、幅は比で、600 px まで | メールのヘッダー |
| `preview-1200x630` | logo | 1200 × 630 px、不透明 | リンクプレビューの既定画像（`preview_image` がないとき） |

- 不透明の版の地の色は、管理者が選ぶ（既定はテーマのライトの `surface`。ADR-0059 の上書きがあればその値）。ホーム画面とマスクで透明の部分がどう塗られるかを OS に任せないため。
- 管理画面の流れ: (1) SVG を選ぶ → (2) `POST /admin/instance/brand/sanitize` がサニタイズした SVG と落としたものの一覧を返す（保存しない） →
  (3) 管理画面がその SVG を `<img>`（`blob:` の URL。CSP の `img-src blob:` の範囲）で読み、各版を `<canvas>` に描いて PNG にし、各版とファビコン・ホーム画面・メールの見え方をプレビューする →
  (4) `PUT /admin/instance/brand/{icon|logo|logo_dark}` に SVG と PNG をまとめて送る。
- **サーバーの検査**: SVG をもう一度サニタイズし（§2）、PNG は中身で PNG と判定し、版ごとの寸法に一致すること・1 つ 1 MiB 以下を確かめ、
  標準ライブラリの `image/png` でデコードして再エンコードする（メタデータを落とす。ADR-0019 §8 と同じ）。揃わない版があれば何も保存しない。
- **サーバーは PNG が SVG と同じ絵かを確かめない**。送れるのは `manage_settings` を持つ管理者だけで、ずれても害はその管理者の表示に留まる（判断基準 1 を下げない）。
- **B1 を選ぶ理由**: ブラウザーは SVG を最も忠実に描き、閲覧者がファビコンや PWA で見る SVG と同じ描画系で PNG ができる（判断基準 2）。
  サーバーに描画系を持たず、cgo も保守の止まった依存も足さない（判断基準 3）。B2 の oksvg は `clipPath`・`mask`・`class`・`<style>` を扱わず、
  よくある書き出しの SVG で SVG と PNG が違う絵になる。B3 は Safari 18 以前のファビコン・iOS のホーム画面・リンクプレビューで管理者のアイコンが出ず、決定者の目的を満たさない。
- **見直す条件**: CLI や API だけで PNG を含めて設定する需要がはっきりしたとき、または `clipPath`・`mask`・`<style>` を扱う保守された pure Go の SVG 描画のライブラリ（例えば tdewolff/canvas の SVG の読み込み）が使えると確かめられたときは、
  B2 に切り替える（版の表と保存の形は変えない）。

### 4. 保存（C1、ADR-0020）

instance モジュールが持つ。テーブルの形は実装で `docs/architecture/data-model/04-tables/09-instance.md` と `schema/<dialect>/09-instance.sql` に書く。

- **イベント（リビジョン）** `instance_brand_revisions`: `id`・`kind`（CHECK で `icon` / `logo` / `logo_dark`）・`action`（CHECK で `set` / `clear`）・`revised_by_user_id`（CLI からは NULL）・`revised_at`。
- **イベントの子** `instance_brand_files`: リビジョン・版（CHECK で `svg` と §3 の版）・BlobStore のキー・`content_type`・幅・高さ・バイト数・SHA-256。`clear` のリビジョンは子を持たない。
  地の色は `instance_brand_revisions` の列に `#rrggbb` で持つ（`icon` だけが使う。CHECK で書式を限る）。
- **投影** `instance_brands`: `kind` ごとの現在のリビジョン。`clear` で行を消す。
- BlobStore のキーはサーバーが作る（`instance/brand/<revision-id>/<版>.<拡張子>`、ADR-0019 §8）。置き換えた古いリビジョンのファイルは、リビジョンの履歴が残る間は消さない
  （履歴の保持と消去は既存の設定のリビジョンと同じ扱い。実装で決める）。BlobStore への書き込みはトランザクションの外で先に行い、DB に記録してから投影を切り替える（ADR-0003）。
- 権限は `manage_settings`（ADR-0058 のサーバー管理の権限。画面はサーバーの設定 `/admin/settings` に置く）。設定・消去は監査ログ（ADR-0037）に、種類とリビジョンを残す。
- CLI は `my instance brand show` と `my instance brand clear <kind>` を持つ（壊れた SVG を管理画面なしで外す復旧のため）。設定は PNG を描くので管理画面からだけ行う。

### 5. 配信

- `my serve` が `/brand/<revision-id>/<版>.svg|.png` で返す（本体のオリジン。外枠の `img-src 'self'` のまま）。中身はリビジョンで決まるので `Cache-Control: public, max-age=31536000, immutable`。
- SVG の応答のヘッダー: `Content-Type: image/svg+xml`、`Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`、`X-Content-Type-Options: nosniff`。
  URL を直接開いても、スクリプト・外部の取得・フォームは動かない（サニタイズとの二重の守り）。PNG は `image/png` と同じ `nosniff`。
- **クライアントは SVG を `<img>` か CSS の背景でだけ出す**。インラインの `<svg>` にも `dangerouslySetInnerHTML` にもしない（`@my/ui` の部品で一か所に閉じる）。
- **外枠**（`shell.go`）: アイコンがあれば `<link rel="icon" type="image/png" sizes="32x32">`（と 48）・`<link rel="icon" type="image/svg+xml">`（SVG）・
  `<link rel="apple-touch-icon" href="…/apple-touch-180.png">` をその URL で出し、組み込みのものを出さない。ロゴは外枠の JSON のデータブロックと `GET /instance` で渡す。
  外枠の ETag にブランドのリビジョンを足す。
- **固定のパス**: iOS などは `link` がなくても `/apple-touch-icon.png` を探し、ほかのクライアントは `/favicon.ico` を探すことがある。
  アイコンがあるときは `/apple-touch-icon.png` を管理者の 180 px の版で返す（`no-cache` と ETag で再検証、今の `cacheRevalidate` と同じ）。
- **Web App Manifest**: `icons` を `pwa-512`・`pwa-192`・`maskable-512`・SVG（`sizes: "any"`）の順にする（最初を PNG にするのは、Misskey が最初のアイコンをサーバーのアイコンにするため）。
  インストール済みの PWA は、Android では Chrome が manifest を確かめたときに替わるが、デスクトップの Chrome はアイコンを替えない（入れ直すまで古いアイコン）。
- **リンクプレビュー**: `og:image` の順に、`preview_image` の次・組み込みの画像の前として `preview-1200x630` を入れる。
- **メール**（ADR-0086 の例外）: ロゴがあるとき、外枠の MJML の先頭に `logo-mail`（ダークの地では `logo-dark-mail`、`prefers-color-scheme` のスタイルで切り替え）を
  `<img src="https://<host>/brand/…" width height alt="<サーバーの名前>">` で出す。URL は全員に同じで、受信者ごとの値を含めない（開封の追跡にしない）。
  外部の画像を読まないクライアントや設定では `alt` の名前が出る。ロゴがないときは今のまま画像を出さない。
- **`GET /instance`**: `icon` / `logo` / `logo_dark` に SVG の URL・各 PNG の URL と寸法・リビジョンを返す（なければ省く）。管理 API は地の色も返す。
- **Service Worker**: `/brand/` を実行時にキャッシュ優先で持つ（版付きのため）。オフラインの外枠でもロゴが出る。
- **連合**: Misskey は manifest の最初のアイコンと最後の `<link rel="icon">` からサーバーのアイコンを取る（Context の表）。外枠と manifest が管理者のアイコンを出すので、追加の仕組みは要らない。

### 既存の ADR との関係

- **ADR-0019 §8**（SVG のアップロードを拒む）: 利用者のメディアには引き続き当たる。この ADR は `manage_settings` の管理者のブランドの SVG だけを、§2 のサニタイズと §5 のヘッダーを条件に例外にする。
  ADR-0019 の状態の行にこの例外を書く。
- **ADR-0086 §2**（メールは外部の画像を読まない）: ロゴのある場合だけ、上記の 1 枚を例外にする。ADR-0086 の状態の行にこの例外を書く。
- **ADR-0059 §2**: 「ロゴ・ファビコン・PWA のアイコンは別に決める」の決定がこの ADR である。
- **ADR-0028**: `og:image` の順に 1 段を足し、マニフェストのアイコンをインスタンスの設定から作る。

## Consequences（影響）

- 管理者は SVG を 1 つ（ロゴは 2 つまで）選ぶだけで、ファビコンからメールまで自分の絵になる。PNG の各版を自分で作らなくてよい。
- サーバーに SVG の描画系も cgo も足さない。サニタイズは標準ライブラリだけで書ける。
- **悪い点**:
  - サニタイズの許可リストを保守し、テスト（既知の XSS の SVG の集合、XXE、`use` の展開、CSS の `url(` / `@import`）を持つ必要がある。
  - `text` を拒むので、文字を含む書き出しの SVG は管理者がアウトライン化してから上げる必要がある。`filter` を落とすので影やぼかしは消える（落としたものは知らせる）。
  - PNG は管理者のブラウザーの描画に依存し、ブラウザーごとに細部が違いうる。PNG と SVG の一致をサーバーは保証しない。
  - 設定は管理画面からだけで、CLI からは消すことしかできない。
  - メールに外部の画像を 1 枚足すので、外部の画像を読まないクライアントでは `alt` の文字になる。
  - BlobStore にリビジョンごとのファイルが溜まる（1 リビジョンで最大でおよそ 2 MB）。
- インストール済みのデスクトップの PWA は、アイコンを替えても入れ直すまで古いアイコンのままである（Chrome の制限）。
- **後続の作業**:
  1. #1692 サーバー: SVG のサニタイズとそのテスト、ブランドのリビジョン・ファイル・投影のスキーマ（全方言）と data-model の文書、
     管理 API（`sanitize`・`PUT`・`DELETE`）と `GET /instance` の項目、`/brand/` の配信とヘッダー、外枠・`/apple-touch-icon.png`・Manifest・`og:image`・メールの MJML の切り替え、
     監査ログ、`my instance brand show|clear`、運用の文書。
  2. #1693 Web / 管理画面（#1692 の後）: ブランドの画面（SVG の選択・サニタイズの結果と落としたものの表示・地の色・canvas での各版の描画とプレビュー・アップロード）、
     `Wordmark` / `BrandMark` / サーバーのカードのアバター / `.site-logo` / Service Worker の切り替え、ロケールの文言、Storybook の見本。
  - メンテナンスの HTML（ADR-0084）は画像を使わない今のままにする（名前の文字で足りる）。

## Pros and Cons of the Options（詳細比較・任意）

| | 長所 | 短所 |
| --- | --- | --- |
| A1（採用） | どの大きさでも鮮明。ファビコンとロゴを SVG のまま出せる。表示の経路が 1 つ | サニタイズが要る。ベクターの絵を持たない管理者は設定できない |
| A2 | 既存の media の検査を使える。PNG しか持たない管理者も設定できる | PNG の元からは SVG のファビコンや鮮明なロゴを作れない。経路が 2 つ |
| B1（採用） | ブラウザーの忠実な描画。サーバーに依存を足さない | クライアントの描画に依存し、CLI / API だけでは設定できない。PNG と SVG の一致を保証しない |
| B2 | API・CLI から同じ PNG ができる。クライアントを信用しない | oksvg / rasterx は保守が止まり、`clipPath`・`mask`・`<style>` を扱わず、絵が違いうる。描画の CPU とメモリをサーバーが持つ |
| B3 | 最も簡単 | Safari 18 以前のファビコン・iOS のホーム画面・リンクプレビューで管理者の絵が出ない |
| C1（採用） | テーマと同じ版付きの配信。SVG 専用のヘッダーを付けられる。media の規則を変えない | ブランド専用の保存と配信の経路が増える |
| C2 | 保存と配信を media と共有できる | media の許可リスト・再エンコード・ヘッダー（ADR-0019 §8）に SVG の例外を作る |

## Open Questions / 要検証

なし。
