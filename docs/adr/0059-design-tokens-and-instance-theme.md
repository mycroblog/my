# ADR-0059: デザイントークンの体系とサーバー単位のテーマ・ロゴ・アイコン

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #649, #664, #1691, #1692, #1693

## Decision（決定）

`theme.css` をトークンの唯一の定義にし、管理者は `theme.css` のトークン（§2 の固定のものを除く）を上書きでき、さらに任意の CSS を足せる。
テーマはサーバーが同じオリジンのスタイルシートとして配る。サーバーが保証するのは、トークンの上書きの値の形と、色のトークンのコントラストである。任意の CSS の結果は管理者の責任とする。

管理者は、SVG でサーバーのアイコンとロゴも設定できる。SVG はサーバーがサニタイズし、SVG を受け付けない場所のための PNG は管理者のブラウザーで描き、instance モジュールが BlobStore に置いて本体のオリジンの版付きのパスで配る（§8〜§12）。

### 1. トークンの体系の強化

- `theme.css` をトークンの唯一の定義にする。上書きできるトークンは `@theme`（`inline` でない）に置き、ユーティリティが `var(--*)` を参照する状態を保つ。
  上書きは CSS 変数の値を替えるだけで効く。上書きできるトークンを `@theme inline` にしないことを `theme.css` の注記と検査で守る。
- `app/ui/scripts` の生成器（Bun）が、`theme.css` と `tokens.ts` から次を作る。コミットするのは Go の `theme_tokens.gen.go` だけで、`scripts/task.sh web:tokens` が古いと失敗する（ADR-0081）。クライアントの一覧はコミットせず、読むタスクの前に作る。
  - Go の `internal/domain/instance` の生成ファイル: 上書きできるトークンの一覧、それぞれの値の種類（色・長さ・フォントファミリー・影・時間・緩急・数）と既定値
    （色はライト / ダークを `#rrggbb` に解決したもの）、コントラストの組と最小値。
    ドメインは標準ライブラリだけで、値の種類ごとの書式の検査と、WCAG 2.2 の相対輝度とコントラスト比（`app/ui/src/color.ts` と同じ式）を計算する。
  - クライアントの `@my/ui` の同じ一覧（管理画面の入力・プレビュー・検査が使う）。コントラストの計算は `src/color.ts` に置き、検査スクリプトと管理画面が共有する。
- 部品の側では、色以外のトークン（文字・余白・角丸・影・動き）も `theme.css` の値と app/ui/README.md の役割の表を正とし、
  `scripts/task.sh web:tokens` は、任意の値の角丸・影・動きの時間（`rounded-[3px]`、`shadow-[…]`、`duration-[…]`）と、README にない余白の段階も拒む。
  部品がトークンだけを使うことで、トークンの上書きが画面全体に一様に効く。
- Storybook の `Foundations/Theme sample` に、上書きの見本（色と余白・角丸を変えたテーマ）を当てた表示を置く。

### 2. 上書きできる範囲

- 上書きできるのは、`theme.css` の `@theme` にあるトークンのうち、下に挙げる固定のもの以外である。指定しないトークンは既定値のまま。
  - 意味の色 `--color-*`（`overlay`、状態の色 `danger` / `success` / `warning` とその `-fg` を含む）: ライトとダークそれぞれに `#rrggbb`。`overlay` だけは `#rrggbbaa` も受け付ける。
    `#rrggbb` は sRGB の中にあることが書式で決まり、色域の検査が要らず、管理画面の色の入力（`<input type="color">` の値）と同じ形になる。
  - 文字: `--font-sans` / `--font-mono` / `--font-serif`（フォントファミリーの名前の並び）、`--text-*` の大きさと行の高さ。
  - 余白の単位 `--spacing`、角丸 `--radius-*`、影 `--elevation-*`、動きの時間と緩急。
- 長さと数には、画面が使えなくならない範囲の上限と下限を生成ファイルに持たせる（範囲の値は実装（#664）で決め、app/ui/README.md に書く）。
- 次のトークンは固定で、上書きできない（`app/ui/src/tokens.ts` の `fixedTokens`）。
  - 原色 `--palette-*`: 意味の色の実装の詳細で、名前と段階が版で変わりうるため。原色を替えたいときは、意味の色を直接上書きする。
  - `--shadow-*`: Tailwind が値をユーティリティに写すため。`--elevation-*` を上書きする。
  - `--color-badge-*`: バッジの色は ADR-0070 が決める。サーバーのテーマはバッジのトークンを上書きしない。
  - `--color-avatar-*`: 組のコントラストがこの値でだけ保たれるため。
  - `--color-hand`・`--color-hand-fg`: 音声チャットの挙手のマンゴーの色で、ロゴのスパークと同じくブランドの印であるため。
  - `--radius-avatar`: アバターの辺に対する割合であるため。
  - `--animate-*`: `theme.css` のキーフレームを名指しする一括指定であるため。部品（`--ease-celebrate` など）を上書きする。
- ロゴ・ファビコン・PWA のアイコンの差し替えはトークンの上書きに含めず、§8〜§12 で扱う。

### 3. 任意の CSS

- 管理者はトークンの上書きに加えて、任意の CSS を 1 つ保存できる。長さには上限（実装で決める。例 64 KiB）を設ける。
- 任意の CSS は、トークンの規則の後に `@layer` の外で同じスタイルシートに入るので、部品のスタイルにも勝つ。
- **CSP は緩めない**。任意の CSS も外枠と同じ CSP の下で動くので、別のオリジンの `@import`・画像・フォントは読めない（`style-src` / `img-src` / `font-src` が `'self'`）。
  使えるのは同じオリジンの資源だけで、そのことを管理画面と運用の文書に書く。
- サーバーは CSS の中身を解釈・サニタイズしない。CSS は独立した `text/css`（`X-Content-Type-Options: nosniff`）の応答で返し、HTML に埋め込まないので、HTML の文脈に出ることはない。
  任意の CSS を書けるのはサーバーの管理者（サーバーそのものを運用する者）だけで、利用者ごとの CSS は許さない。
- 任意の CSS が当たるのは、クラス名や DOM の構造に依存する部分である。これらは安定した API ではなく、版の変更で壊れうる。安定して上書きできるのはトークンだけである。

### 4. 検証

- **サーバーが検証すること**（管理 API の `PUT` が上書きの全体を置き換えるときに、Go のドメインが検査する。満たさなければ保存せず、422 の Problem に理由を並べて返す。ADR-0016）:
  - 上書きのトークンの名前が生成した一覧にあること、値がトークンの種類の書式と範囲に収まること。値は CSS に入るので、書式の検査が注入も防ぐ（`;`・`}`・`url(`・コメントを含む値は書式に合わない）。
  - 色のトークンの上書きを既定値と合わせ、生成した組のすべてをライトとダークで検査し、1 つでも最小値を下回れば拒む（上書きしたトークンが既定のトークンとの組で落ちる場合も含む）。
    #664 の受け入れ基準「コントラストが足りない上書きは保存できない」はこの検査である。
  - 任意の CSS の長さの上限。
- **サーバーが検証しないこと（管理者の責任）**: 任意の CSS が色・余白・表示に与える結果（コントラスト、横スクロール、タップ領域、フォーカスの表示、RTL）。
  管理画面は任意の CSS の欄に、この責任と、品質ゲート（ADR-0110）が既定のテーマに対してだけ働くことを示す。
- 管理画面は同じ一覧と計算で、入力中に色の組ごとの比を示し、下回る組があるうちは保存のボタンの前に理由を示す（保存の判定はサーバーが正）。
- 保存は ADR-0020 に従い、テーマのリビジョン（イベント。任意の CSS の本文を持つ）と、そのリビジョンのトークンの行（トークン・テーマ・値。トークンとテーマは CHECK で限る）と、
  現在のリビジョンの投影に分ける。トークンを JSON の列に入れない。テーブルの形は data-model.md に書く。
  権限は `manage_settings`（ADR-0058 のサーバー管理）、変更は監査ログ（ADR-0058）に残す。

### 5. 配信

- `my serve` が、現在のテーマを 1 つのスタイルシートにして、リビジョンを含むパス（例 `/theme/{revision}.css`）で返す。中身は次の順である。
  1. トークンの上書きの `:root { … }` の規則。色は `light-dark(#…, #…)` にする。名前は生成した一覧から、値は検査済みのものだけから組み立てる。
  2. 任意の CSS。

  どちらも `@layer` の外に置くので、Tailwind が `@layer theme` に出す既定値と部品のユーティリティに勝つ。
  内容はリビジョンで決まるので `immutable` で長期キャッシュする。上書きも任意の CSS もないときは `<link>` を出さない。
- SPA の外枠と、サーバーが描画する画面は、ビルドのスタイルシートの後に `<link rel="stylesheet">` で読む。スタイルシートは描画を止めるので最初の描画から当たり、`data-theme` と `light-dark()` の仕組みはそのまま効く。
  CSP（`style-src 'self'`）は変えない。外枠は閲覧者ごとに変わらない（インスタンスごとに 1 つ）ので、共有キャッシュも ETag（テーマのリビジョンを足す）もそのまま使える。
  保存から全員の外枠に届くまでは外枠のキャッシュ（最大 60 秒と再検証の 300 秒）の分だけ遅れる。
- Service Worker は `/theme/` のスタイルシートを実行時にキャッシュし（版付きなのでキャッシュ優先）、オフラインの外枠でもテーマが当たる。
- `theme-color`: 外枠は `media="(prefers-color-scheme: light)"` と `dark` の 2 つの `<meta name="theme-color">` に、テーマの `surface` のライト / ダークの値を書く。
  Web App Manifest の `theme_color` / `background_color` はテーマを 1 つしか持てないので、ライトの `surface` にする。`Options.ThemeColor` / `BackgroundColor` の既定値もトークンの既定の `surface` に合わせる。
  `data-theme` で OS と違うテーマを選んだときのブラウザの枠の色は OS の設定に従う（受け入れる制限）。
  任意の CSS で背景を変えても `theme-color` には反映しない（`surface` のトークンだけが元になる）。
- `GET /instance` は、テーマのリビジョンとスタイルシートの URL、トークンの上書きの値（トークン・ライト・ダーク）を返す。管理画面の初期値と、外枠を使わないクライアント（§6 の Tauri）のためである。
  管理 API は任意の CSS の本文も返す。

### 6. Tauri のアプリ

- Tauri のアプリ（ADR-0028、FR-603）はビルドした UI を同梱し、接続するサーバーが実行時に変わるので、外枠の `<link>` は使えない。
  サーバーのスタイルシートを取得し、構築できるスタイルシート（`new CSSStyleSheet()` の `replaceSync` と `document.adoptedStyleSheets`）で当てる。
  CSSOM による変更は web-client の D10 の禁止（`style` 属性・`cssText`）に当たらない。最初の描画の前に当てるため、前回の内容をアプリの保存領域に残して起動時に先に当てる。
  WebView での動作の検証は Tauri の作業で行い、急がない。

### 7. テスト

- 品質ゲート（ADR-0110）のスクリーンショット・axe・横スクロールの検査は、既定のテーマに対して行う。
- テーマについての E2E は小さく保つ: 見本のテーマ（色・余白のトークンの上書きと短い任意の CSS）を保存した `my serve` で、外枠とサーバーの画面がスタイルシートを読み、
  トークンと任意の CSS が当たっていることだけを確かめる。
- 管理 API の検査（書式・範囲・コントラスト・長さ）は Go の単体テストで網羅する。

### 8. アイコンとロゴ

- **アイコン**（`icon`）: 正方形のマーク。ファビコン・PWA のアイコン・`apple-touch-icon`・サーバーを四角い絵で表す場所（ランディングとサーバー情報のカードのアバター、Tauri のサーバーの切り替え）に使う。
- **ロゴ**（`logo`）: 横長のワードマーク。アプリの外枠のヘッダー（`BrandMark`）・ランディング・サーバーが描画する画面（`.site-logo`）・メールのヘッダー・リンクプレビュー（§12）に使う。
  ロゴはサーバーの名前を含む絵として扱い、設定されているときは `BrandMark` は名前の文字を横に出さず、ロゴの `<img>` の `alt` をサーバーの名前にする。
- **ダークのロゴ**（`logo_dark`）: 任意。暗い地で使う。ないときは暗い地でも `logo` を使う。アイコンはライトとダークで 1 つ（地の色を管理者が選ぶ。§10）。
- 元の形式は SVG だけである。
- `thumbnail` / `preview_image`（バナー）はそのまま残す。役割が違う（大きな写真・イラスト）。
- 未設定のものは、組み込みの my の絵（`icon.svg` などのビルドのファイル、`Wordmark`）を使う。設定を消すと組み込みに戻る。

### 9. SVG のサニタイズと検査（サーバー）

`internal/domain/instance` の純粋な関数として、標準ライブラリの `encoding/xml` でトークンを読み、許可したものだけから**新しく書き出す**（入力をそのまま通さない）。
保存の API はサニタイズした結果だけを保存し、元のファイルは残さない。

- **拒む（422 の Problem）**: XML として壊れているもの、`<!DOCTYPE>` と実体の宣言（XXE と実体の展開を防ぐ）、ルートが `svg` でないもの、
  `viewBox` がないか数として読めないもの、サニタイズの前で 256 KiB・後で 128 KiB を超えるもの、要素が 5,000 個を超えるもの、`use` の展開の後の要素が 20,000 個を超えるもの、`viewBox` の数が 1,000,000 を超えるもの、入れ子が 32 段を超えるもの、
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
- **文字**: `text` / `tspan` は拒み、「文字をアウトライン（パス）にしてください」と知らせる。文字の形は閲覧者のフォントで変わり、ブラウザーで描く PNG（§10）と SVG で違う絵になるため。

### 10. PNG の版は管理者のブラウザーで描く

SVG を受け付けない場所のために、管理画面が次の PNG を描き、SVG と一緒にアップロードする。

| 版 | 元 | 大きさ | 使い道 |
| --- | --- | --- | --- |
| `favicon-32` / `favicon-48` | icon | 32 / 48 px | Safari 18 以前などのファビコン（`<link rel="icon" type="image/png" sizes>`） |
| `apple-touch-180` | icon | 180 px、不透明 | iOS のホーム画面 |
| `pwa-192` / `pwa-512` | icon | 192 / 512 px | Manifest の `purpose: "any"` とインストールの条件 |
| `maskable-512` | icon | 512 px、不透明 | Manifest の `purpose: "maskable"`（中央 80% の安全域に縮めて置く） |
| `logo-mail` / `logo-dark-mail` | logo / logo_dark | 高さ 96 px（表示は 48 px の 2 倍）、幅は比で、600 px まで | メールのヘッダー |
| `preview-1200x630` | logo | 1200 × 630 px、不透明 | リンクプレビューの既定画像（`preview_image` がないとき） |

- 不透明の版の地の色は、管理者が選ぶ（既定はテーマのライトの `surface`。§2 の上書きがあればその値）。ホーム画面とマスクで透明の部分がどう塗られるかを OS に任せないため。
- 管理画面の流れ: (1) SVG を選ぶ → (2) `POST /admin/instance/brand/sanitize` がサニタイズした SVG と落としたものの一覧を返す（保存しない） →
  (3) 管理画面がその SVG を `<img>`（`blob:` の URL。CSP の `img-src blob:` の範囲）で読み、各版を `<canvas>` に描いて `toBlob("image/png")` で PNG にし、各版とファビコン・ホーム画面・メールの見え方をプレビューする →
  (4) `PUT /admin/instance/brand/{icon|logo|logo_dark}` に SVG と PNG をまとめて送る。消去は同じパスの `DELETE` で行う。
- **サーバーの検査**: SVG をもう一度サニタイズし（§9）、PNG は中身で PNG と判定し、版ごとの寸法に一致すること・1 つ 1 MiB 以下を確かめ、
  標準ライブラリの `image/png` でデコードして再エンコードする（メタデータを落とす。ADR-0019 §8 と同じ）。揃わない版があれば何も保存しない。
- **サーバーは PNG が SVG と同じ絵かを確かめない**。送れるのは `manage_settings` を持つ管理者だけで、ずれても害はその管理者の表示に留まる（サーバーのオリジンでスクリプトが動く経路・外部への取得・解凍爆弾を作らないという安全性を下げない）。
- **ブラウザーで描く理由**: ブラウザーは SVG を最も忠実に描き、閲覧者がファビコンや PWA で見る SVG と同じ描画系で PNG ができる。
  サーバーに描画系を持たず、cgo も保守の止まった依存も足さない。サーバーが描く oksvg は `clipPath`・`mask`・`class`・`<style>` を扱わず、
  よくある書き出しの SVG で SVG と PNG が違う絵になる。描かない案は Safari 18 以前のファビコン・iOS のホーム画面・リンクプレビューで管理者のアイコンが出ない。
- **見直す条件**: CLI や API だけで PNG を含めて設定する需要がはっきりしたとき、または `clipPath`・`mask`・`<style>` を扱う保守された pure Go の SVG 描画のライブラリ（例えば tdewolff/canvas の SVG の読み込み）が使えると確かめられたときは、
  サーバーが pure Go で描く形に切り替える（版の表と保存の形は変えない）。

### 11. 保存（ADR-0020）

instance モジュールが持つ。テーブルの形は `docs/architecture/data-model/04-tables/09-instance.md` と `schema/09-instance/<dialect>.sql` に書く。

- **イベント（リビジョン）** `instance_brand_revisions`: `id`・`kind`（CHECK で `icon` / `logo` / `logo_dark`）・`action`（CHECK で `set` / `clear`）・`revised_by_user_id`（CLI からは NULL）・`revised_at`。
- **イベントの子** `instance_brand_files`: リビジョン・版（CHECK で `svg` と §10 の版）・BlobStore のキー・`content_type`・幅・高さ・バイト数・SHA-256。`clear` のリビジョンは子を持たない。
  地の色は `instance_brand_revisions` の列に `#rrggbb` で持つ（`icon` だけが使う。CHECK で書式を限る）。
- **投影** `instance_brands`: `kind` ごとの現在のリビジョン。`clear` で行を消す。
- BlobStore のキーはサーバーが作る（`instance/brand/<revision-id>/<版>.<拡張子>`、ADR-0019 §8）。置き換えた古いリビジョンのファイルは、リビジョンの履歴が残る間は消さない
  （履歴の保持と消去は既存の設定のリビジョンと同じ扱い。実装で決める）。BlobStore への書き込みはトランザクションの外で先に行い、DB に記録してから投影を切り替える（ADR-0003）。
- 権限は `manage_settings`（ADR-0058 のサーバー管理の権限。画面はサーバーの設定 `/admin/settings` に置く）。設定・消去は監査ログ（ADR-0058）に、種類とリビジョンを残す。
- テーマの CLI は `my instance theme get` と `my instance theme set` を持つ。
- CLI は `my instance brand show` と `my instance brand clear <kind>` を持つ（壊れた SVG を管理画面なしで外す復旧のため）。設定は PNG を描くので管理画面からだけ行う。

### 12. 配信（ブランドの画像）

- `my serve` が `/brand/<revision-id>/<版>.svg|.png` で返す（本体のオリジン。外枠の `img-src 'self'` のまま）。中身はリビジョンで決まるので `Cache-Control: public, max-age=31536000, immutable`。
- SVG の応答のヘッダー: `Content-Type: image/svg+xml`、`Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`、`X-Content-Type-Options: nosniff`。
  URL を直接開いても、スクリプト・外部の取得・フォームは動かない（サニタイズとの二重の守り）。PNG は `image/png` と同じ `nosniff`。
- **クライアントは SVG を `<img>` か CSS の背景でだけ出す**。インラインの `<svg>` にも `dangerouslySetInnerHTML` にもしない（`@my/ui` の部品で一か所に閉じる）。
- **外枠**（`shell.go`）: アイコンがあれば `<link rel="icon" type="image/png" sizes="32x32">`（と 48）・`<link rel="icon" type="image/svg+xml">`（SVG）・
  `<link rel="apple-touch-icon" href="…/apple-touch-180.png">` をその URL で出し、組み込みのものを出さない。ロゴは外枠の JSON のデータブロックと `GET /instance` で渡す。
  外枠の ETag にブランドのリビジョンを足す。
- **固定のパス**: iOS などは `link` がなくても `/apple-touch-icon.png` を探し、ほかのクライアントは `/favicon.ico` を探すことがある。
  アイコンがあるときは `/apple-touch-icon.png` を管理者の 180 px の版で返す（`no-cache` と ETag で再検証、`cacheRevalidate` と同じ）。
- **Web App Manifest**: `icons` を `pwa-512`・`pwa-192`・`maskable-512`・SVG（`sizes: "any"`）の順にする（最初を PNG にするのは、Misskey が最初のアイコンをサーバーのアイコンにするため）。
  インストール済みの PWA は、Android では Chrome が manifest を確かめたときに替わるが、デスクトップの Chrome はアイコンを替えない（入れ直すまで古いアイコン）。
- **リンクプレビュー**: `og:image` の順に、`preview_image` の次・組み込みの画像の前として `preview-1200x630` を入れる。
- **メール**（ADR-0086 の例外）: ロゴがあるとき、外枠の MJML の先頭に `logo-mail`（ダークの地では `logo-dark-mail`、`prefers-color-scheme` のスタイルで切り替え）を
  `<img src="https://<host>/brand/…" width height alt="<サーバーの名前>">` で出す。URL は全員に同じで、受信者ごとの値を含めない（開封の追跡にしない）。
  外部の画像を読まないクライアントや設定では `alt` の名前が出る。ロゴがないときは画像を出さない。
- **メンテナンスの HTML**（ADR-0088）: 画像を使わない（名前の文字で足りる）。
- **`GET /instance`**: `icon` / `logo` / `logo_dark` に SVG の URL・各 PNG の URL と寸法・リビジョンを返す（なければ省く）。管理 API は地の色も返す。
- **Service Worker**: `/brand/` を実行時にキャッシュ優先で持つ（版付きのため）。オフラインの外枠でもロゴが出る。
- **連合**: Misskey は manifest の最初のアイコンと最後の `<link rel="icon">` からサーバーのアイコンを取る。外枠と manifest が管理者のアイコンを出すので、追加の仕組みは要らない。

### 13. 既存の ADR との関係

- テーマは Accepted ADR の決定を置き換えない。ADR-0028 の「外枠を閲覧者ごとに変えない・CSP の nonce を使わない」と ADR-0019 §9 の CSP の範囲で、外枠に同じオリジンのスタイルシートを 1 つ足す。
  ADR-0110 の品質ゲートは既定のテーマに対して働き、管理者のテーマについては色のトークンのコントラストだけをサーバーが保証する。
- **ADR-0019 §8**（SVG のアップロードを拒む）: 利用者のメディアには引き続き当たる。`manage_settings` の管理者のブランドの SVG だけを、§9 のサニタイズと §12 のヘッダーを条件に例外にする。
- **ADR-0086 §2**（メールは外部の画像を読まない）: ロゴのある場合だけ、§12 の 1 枚を例外にする。
- **ADR-0028**: `og:image` の順に 1 段を足し、マニフェストのアイコンをインスタンスの設定から作る。

## 経緯

決定者の指示は、デザイントークンの管理を強化し、トークン上書きによる独自テーマをサーバーで適用できるようにすることだった（FR-601、FR-602、FR-605）。トークンは `app/ui/src/theme.css` が Tailwind CSS v4（ADR-0025）の `@theme` で持ち、色は `--palette-*`（OKLCH の原色）と `light-dark(<ライト>, <ダーク>)` で原色を指す `--color-*` の 2 層である。テーマの切り替えは `<html data-theme>` で、`public/theme.js` が最初の描画の前に付ける（web-client §9）。
外枠は全閲覧者に共通で 60 秒の共有キャッシュがあり、CSP は nonce のない `style-src 'self'` と Trusted Types なので（ADR-0028、ADR-0019 §9）、インラインのスタイル・nonce・閲覧者ごとの外枠を足さずに、JavaScript のないサーバーの画面（web-client §8）にも最初の描画から当たる形を選んだ。

管理者の自由度は決定者の判断で、色だけでなく余白・角丸・文字、さらに任意の CSS まで広げた。一方で色のトークンの上書きはコントラスト AA を満たさないと保存できない（ADR-0110）とし、任意の CSS の結果は管理者の責任と切り分けた。他のサーバーでは（2026-10-20 に一次資料で確認）、Mastodon が任意の CSS を同じオリジンのスタイルシートで配り検査はなく、Misskey は変数による上書きができるがコントラストの検査はなく、Pleroma / Akkoma と GoToSocial はファイルを置いて再起動する。

ロゴとアイコンは、バナーの画像に加えて SVG で設定したいという決定者の指示で決めた（FR-601、FR-602、FR-605、NFR-SEC-6）。SVG はスクリプト・外部の資源の参照・イベントハンドラーを持てる文書で、`<img>` や CSS の背景で読めば無効だが、URL を直接開いたときや `<iframe>` / `<object>` には当たらない（[MDN](https://developer.mozilla.org/en-US/docs/Web/SVG/Guides/SVG_as_an_image)）。ラスター画像が要る場所は次のとおりで（2026-10-28 に一次資料で確認）、§10 の版はここから決めた。

- ファビコンの SVG は Chrome / Edge 80+、Firefox 41+、Safari 26.0 から（[Can I use](https://caniuse.com/link-icon-svg)）。
- Chrome のインストールの条件は 192 px と 512 px のアイコンで、形式は書かれていない（[web.dev](https://web.dev/articles/install-criteria)）。Chrome は起動時に 24 時間ごとに manifest を確かめ、Android は WebAPK を作り直すが、デスクトップはアイコンの変更に対応しない（[web.dev](https://web.dev/articles/manifest-updates)）。
- iOS の `apple-touch-icon` は PNG のみ（[Apple](https://developer.apple.com/library/archive/documentation/AppleApplications/Reference/SafariWebContent/ConfiguringWebApplications/ConfiguringWebApplications.html)）。
- OGP は形式を決めていない（[ogp.me](https://ogp.me/)）ので、クローラーの扱いに依らないよう PNG だけを出す。Gmail は SVG を PNG に変換して出す（[Can I email](https://www.caniemail.com/features/image-svg/)、最終テスト 2026-09-16）ので、メールにも PNG を出す。

トークンの上書きが `@layer` の外の `:root` の規則で Tailwind のユーティリティに効くことは、`app/web/e2e/instance-theme.spec.ts` で確かめている。

## 採らなかった案

トークンとテーマ:

- JSON / TypeScript の定義から CSS と Go を生成する（Style Dictionary など）: 言語に依らない定義にできるが、`@theme` の書き方の再現に新しい依存か自作の変換器が要る。
- 上書きをアクセントの色だけ、または意味の色だけにする: 壊れにくく組の検査もそのまま使えるが、管理者が変えたい余白・文字に足りない。
- 原色まで上書きする: 段階ごと替えられるが、名前と段階が実装の詳細で、版の変更で上書きの意味が変わる。
- コントラストをクライアントだけで検査する、または警告だけにする: サーバーの実装が少なく自由だが、API を直接呼べば AA を満たさないテーマを保存でき、ADR-0110 のゲートを破れる。
- `GET /instance` の値をクライアントが起動後に当てる: サーバーの変更は少ないが、最初の描画で既定の見た目が見え、サーバーの画面には当たらない。
- 外枠の `<style>` に埋め込み CSP に `'sha256-…'` を足す: リクエストは増えないが、テーマの版ごとに CSP が変わり `style-src` を緩める。
- 任意の CSS を許さない: 品質ゲートの保証が保たれ版の変更で崩れないが、トークンにない変更ができない。

ロゴとアイコン:

- SVG と PNG のどちらでも受ける: PNG は既存の media の検査が使え、PNG しか持たない管理者も設定できるが、ファビコンの SVG や鮮明なロゴを作れず、表示の経路が 2 つになる。
- サーバーが pure Go で描く（[oksvg](https://github.com/srwiley/oksvg) と [rasterx](https://github.com/srwiley/rasterx)）: API・CLI から同じ PNG ができクライアントを信用しなくてよいが、`clipPath`・`mask`・`pattern`・`marker`・`filter`・`text`・`class` を扱わず、最後の push が 2023 年と 2022 年で保守が止まっている（2026-10-28 確認）。
- 描かない: 最も簡単だが、SVG を受け付けない場所に管理者の絵が出ない。
- media モジュールのメディアにする: 保存と配信を共有できるが、media の許可リスト・再エンコード・ヘッダー（ADR-0019 §8）に SVG の例外を作ることになる。

## Consequences（影響）

- 管理者はコードを変えずに、色・文字・余白・角丸と任意の CSS でサーバーの見た目を変えられ（FR-605）、SVG を 1 つ（ロゴは 2 つまで）選ぶだけでファビコンからメールまで自分の絵になる。
- 色のトークンの上書きは、どの組み合わせでもライト / ダークの両方で AA が保たれる。トークンの上書きは名前にだけ依存するので、部品の作り替えで壊れない。
- CSP・外枠のキャッシュ・JavaScript なしのサーバーの画面がそのまま動き、最初の描画からテーマが当たる。サーバーに SVG の描画系も cgo も足さない。
- **悪い点**:
  - 任意の CSS と、文字・余白のトークンの上書きは、コントラスト・横スクロール（web-client の D3）・タップ領域（D4）・RTL を壊しうる。サーバーはそれを検査しない。
  - 任意の CSS は部品のクラス名と DOM に依存するので、版を上げると効かなくなったり崩れたりしうる。リリースノートで知らせる以上の互換は約束しない。
  - 1 つの色を変えると別の組が落ちることがあり（`accent` を明るくすると `accent-fg` も替える必要がある）、管理画面にそれを分かりやすく示す必要がある。
  - 外枠のキャッシュの分だけ、保存してから全員に届くまで数分遅れる。ブラウザの枠の色（`theme-color`）は `data-theme` の選択と任意の CSS に追従しない。
  - サニタイズの許可リストを保守し、テスト（既知の XSS の SVG の集合、XXE、`use` の展開、CSS の `url(` / `@import`）を持つ必要がある。
  - `text` を拒むので、文字を含む書き出しの SVG は管理者がアウトライン化してから上げる必要がある。`filter` を落とすので影やぼかしは消える（落としたものは知らせる）。
  - PNG は管理者のブラウザーの描画に依存し、ブラウザーごとに細部が違いうる。PNG と SVG の一致をサーバーは保証しない。設定は管理画面からだけで、CLI からは消すことしかできない。
  - メールに外部の画像を 1 枚足すので、外部の画像を読まないクライアントでは `alt` の文字になる。BlobStore にリビジョンごとのファイルが溜まる（1 リビジョンで最大でおよそ 2 MB）。
  - インストール済みのデスクトップの PWA は、アイコンを替えても入れ直すまで古いアイコンのままである（Chrome の制限）。
- 後続: Tauri のアプリでの適用（§6）は Tauri の作業の Issue に含める。

## Open Questions / 要検証

- Tauri の WebView（WKWebView・Android WebView）で、CSP の下で `adoptedStyleSheets` が使えることは、Tauri の作業で確かめる（要検証）。
