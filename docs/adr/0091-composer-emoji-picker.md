# ADR-0091: 絵文字ピッカーと絵文字の描画

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1040, #1145, #1176, #1202

## Decision（決定）

ピッカーは投稿・編集・プロフィール・リアクションで同じ `@my/ui` の `EmojiPicker` を使い、グリッドを TanStack Virtual で仮想化する。Unicode 絵文字のデータは Unicode と CLDR からリポジトリ内で生成する。Web クライアントのすべての場所で、Unicode 絵文字を自前で配る Noto Color Emoji で描く。「よく使う」と肌の色は端末ごと（localStorage）に持つ。開くボタンは本文の入力欄の右上に置く。

### 1. データ

- 生成器は `scripts/emoji-table` で、サーバーの表とクライアントのデータを **同じ `EMOJI_VERSION` の同じ `emoji-test.txt`** から一度に作る。ピッカーで選べる Unicode 絵文字を、サーバーがリアクションで受け入れて正規化できるもの（ADR-0051 §6）と同じ集合にするため。
  CLDR の版は `CLDR_VERSION` として同じタスクに固定し、取得したファイルのダイジェストを確かめる。タスクは `emoji:generate` にまとめ、版の更新は 2 つの変数を変えて実行するだけにする。
  CLDR は `unicode-org/cldr-json` のリリースの JSON（`cldr-annotations-full/annotations/<ロケール>/annotations.json` と `cldr-annotations-derived-full/annotationsDerived/<ロケール>/annotations.json`）を取る。
  CLDR 49（cldr-json 49.0.0、2026-10-22 公開）は、Emoji 18.0 の `emoji-test.txt` の絵文字（`Component` を除く）すべてに `en` と `ja` の名前を持つ（2026-10-23 に両方を取得して突き合わせて確認）。
  CLDR の annotations は VS16（U+FE0F）を外した列を鍵にするので、生成器は fully-qualified の列と VS16 を外した列の両方で引く。
- クライアントのデータは `app/web/src/features/emoji/unicode/` に生成してコミットする（`*.gen.json`）。
  - 基本のデータ（ロケールに依らない）: 書記素の列（fully-qualified）、グループ、並び順、絵文字の版、肌の色の異体の列。`Component` のグループ（肌の色の修飾子だけのものなど）は出さない。
  - ロケールごとのデータ: CLDR の短い名前とキーワード。`en` と `ja` を作り、`en-XA` / `ar-XB`（疑似ロケール）とほかのロケールは `en` に落とす。CLDR に名前が無い絵文字は `en`、それも無ければ `emoji-test.txt` の英語の名前を使う。
- グループの名前（9 個）と肌の色の選択の文言は、メッセージカタログ（`client` の名前空間）に足す（ADR-0009）。肌の色の 5 段階の名前は CLDR の修飾子の annotations を使う。
- データは `features/emoji` に置き、`@my/ui` に置かない。ロケールのデータは i18n のカタログに当たり、ADR-0025 が `@my/ui` に禁じるアプリの i18n への依存になる。`EmojiPicker` はデータを持たず、節（見出し・アイコン・絵文字の列）を受け取る。
- 読み込みは `import()` で、ピッカーを初めて開いたとき（とポインタが乗ったとき・フォーカスしたときの先読み）に、基本のデータと表示中のロケールのデータの 2 つのチャンクを取る。ADR-0092 の補完も同じモジュールを使う。
  大きさの上限は 2 つで gzip 後 100 KB とする。
  Emoji 18.0 と CLDR 49 から試しに生成した詰めた JSON は、基本のデータ（基の絵文字 1,923 個と肌の色の異体 2,040 個）が gzip 後 12.7 KB、
  `ja` の名前とキーワードが 37.7 KB、肌の色の異体の名前が 12.4 KB で、合わせて約 63 KB（2026-10-23 に計測。`en` もほぼ同じ）。
- ライセンス: Unicode のデータ（`emoji-test.txt`）と CLDR は Unicode License v3（SPDX `Unicode-3.0`、OSI 承認。2026-10-23 に [SPDX](https://spdx.org/licenses/Unicode-3.0.html) と [unicode.org/license.html](https://unicode.org/license.html) で確認）で、
  複製に著作権と許諾の表示を添えることを条件に改変・再配布を許すので AGPL-3.0 と両立する。生成したデータの隣に表示の文（`LICENSE-Unicode.txt`）を置き、Web のライセンス表示にも載せる。
  npm の依存ではないので `web:licenses` の許可リスト（`NPM_ALLOWED_LICENSES`）は変えない。

### 2. 表示

- Unicode 絵文字は、ピッカー・補完・リアクション・投稿の本文のどれも、同じ `--font-sans` の Noto Color Emoji で描く（§8〜§13）。ピッカーで見た形と投稿で見える形が一致する。
- OS が描けない版の絵文字を canvas で判定して隠すことはしない。フォントが `EMOJI_VERSION` のすべてを描くことは、生成のときに確かめる（§8）。

### 3. ピッカーの構成

- **並び**: よく使う → カスタム絵文字（ADR-0051 のカテゴリごとの見出し。カテゴリの無いものが先）→ Unicode の 9 グループ（`emoji-test.txt` の順: 顔と感情・人と体・動物と自然・食べ物と飲み物・旅行と場所・活動・物・記号・旗）。
  カスタム絵文字が無いサーバーでは、その節とタブを出さない。
- **カテゴリのタブ**: グリッドの上に、節ごとのアイコンのボタンの列（React Aria の `Toolbar`）を置く。アイコンは `Icons.tsx` の Phosphor（ADR-0025。絵文字そのものは OS ごとに形と色が変わり、強制カラーモードで潰れるので使わない）、名前は節の見出しと同じ文言でツールチップを持つ。
  1 つのグリッドを続けてスクロールする形なので、APG の tabs（パネルを切り替える）ではなく、押すとその節の見出しまでグリッドをスクロールしてその節の最初の絵文字にフォーカスを移すボタンにする。
  見えている節のボタンに `aria-current="true"` を付け、スクロールに合わせて移す（§18）。スクロールの滑らかさは §15 に従う。
- **よく使う**: 回数と新しさ（使った時刻で減衰させた回数）で並べ、32 個まで持つ。Unicode 絵文字（肌の色を外した基の列）とこのサーバーのカスタム絵文字（shortcode）を同じ列に混ぜる。
  まだ無いときは、リアクションの「よく使う群」を既定の列として出す。
- **検索**: 検索欄の文字列を NFKC・小文字・カタカナをひらがなにそろえてから、カスタム絵文字の shortcode と別名、Unicode 絵文字の表示中のロケールと `en` の名前とキーワード、絵文字そのものと比べる。
  並びは「名前の完全一致 → 名前の前方一致 → キーワードの前方一致 → 部分一致」の順、同じ順位の中は既定の並び（カスタム絵文字が先）。検索中はタブを無効にし、結果だけの 1 つの節にする。
  結果の件数は検索が止まってから `role="status"` で告げる（「N 件の絵文字」）。
- **肌の色**: 検索欄の隣に、今の肌の色の手（✋ と選んだ修飾子）を見せるボタンを置き、`Menu`（単一選択）で 6 つ（既定の黄色と 5 段階）から選ぶ。選んだ色は保存し、異体のある絵文字はその色で表示して入れる。
  複数の人の絵文字は全員に同じ色を付ける（人ごとの色の組み合わせは選ばせない）。
- **性能**: Unicode の全集合（肌の色の異体を除いて 1,923 個）と多数のカスタム絵文字を出すので、グリッドは §14〜§19 のとおり仮想化する。

### 4. 置き場所と開き方

- 本文の `TextArea` の右上（書字方向の終わり側。論理プロパティ `inset-inline-end`）に、アイコンだけの絵文字のボタン（`aria-label` と同じ文言のツールチップ）を置く。カスタム絵文字の有無に関わらず出す。
  本文の入力が右上のボタンの下に回り込まないように、入力欄の終わり側に余白を取る。道具の行に絵文字のボタンを置かないので、320 px のシートの道具の行が 1 つ短くなる（web-client §12 の「送信が最初の画面に入る」を楽にする）。
- 同じボタンを、投稿の編集のダイアログの本文、プロフィールの編集の表示名と自己紹介に置く。CW と投票の選択肢には置かず、ADR-0092 の `:` からの補完と OS の入力に任せる（1 行の欄にボタンを並べると 320 px で欄が狭くなる）。
- ピッカーは `@my/ui` の新しいデザインの `Popover`（#991）で開き、検索欄にフォーカスを移す。幅は画面の幅から余白を引いた幅と 22 rem の小さいほうにし、列の数は §14 の計算で決める。
  ADR-0110 の `md` 未満のシートの中で開くので、シートの上に重なるポップオーバーとして画面に収まることを E2E の 320 px で確かめる。

### 5. 入れ方

- Unicode 絵文字はその文字（肌の色を付けたもの）を、カスタム絵文字は `:shortcode:` を、カーソルの位置（選択があればその範囲）に入れる。カスタム絵文字の前後の空白の規則は web-client §12 のとおり。Unicode 絵文字には空白を足さない。
- 選ぶとピッカーを閉じて入力欄にフォーカスを戻す。Shift を押しながら選ぶ（Shift+Enter・Shift+クリック）と、閉じずに続けて選べ、フォーカスはグリッドに残る。
  タッチの端末には続けて選ぶ操作を設けず、1 つ選ぶごとに閉じる。

### 6. アクセシビリティ

- 各絵文字の名前は、Unicode 絵文字は表示中のロケールの CLDR の短い名前（肌の色の付いたものは CLDR の annotationsDerived の名前）、カスタム絵文字は `:shortcode:`。
- グリッドは APG の grid の操作（Tab の順、矢印キーは書字方向に沿う、Home / End、Enter・Space・押下で選ぶ）を §17 のとおり持つ。
- ADR-0110 の層（Testing Library のキー操作、Storybook の axe、3 幅 × ライト / ダーク × LTR / RTL のスクリーンショット、E2E の 320 px と 4 ロケール）を通す。

### 7. リアクション

リアクションのピッカー（ADR-0051 §10、web-client §13）も同じピッカーにし、全集合とよく使う・検索・肌の色を持つ。
データがサーバーと同じ `EMOJI_VERSION` から作られるので、選べる Unicode 絵文字はすべてサーバーが受け入れる。リモートのカスタム絵文字は出さない。

### 8. フォントの出所と版

Noto Color Emoji を COLRv1 と OpenType-SVG の 2 つの形式で自前で分割して配り、ブラウザーが `tech()` で選ぶ。対象のブラウザー（Chrome・Firefox・Safari、Tauri の WebView の iOS の WKWebView と Android の WebView）で同じ形にする。
`font-src 'self'`（ADR-0019 §9、ADR-0059）を変えず、第三者のオリジンに閲覧を知らせない。絵文字を含まない画面ではフォントを読まず、読むのは画面にある絵文字の分だけにする。

- **COLRv1**: `googlefonts/noto-emoji` のリリース `v2026-09-24-unicode18_0`（2D v2.057、Unicode 18.0）の `2D/fonts/Noto-COLRv1.ttf`（5.0 MB）を使う。
- **OpenType-SVG**: noto-emoji は SVG のフォントをビルドしない。`google/fonts` の `ofl/notocoloremoji/NotoColorEmoji-Regular.ttf`（v2.055、25 MB）を使う。このファイルは `COLR`・`CPAL`・`SVG` の表を併せ持つので、`COLR` と `CPAL` を外す。
- **確認したこと（2026-10-24）**: どちらのファイルも、Emoji 18.0 の `emoji-test.txt` の fully-qualified の 3,963 列をすべて 1 つのグリフに組む。HarfBuzz（`uharfbuzz`）で整形して確かめた。
  Google Fonts は、Chrome と Firefox の User-Agent に COLRv1 の表の WOFF2 を、Safari の User-Agent に SVG の表の WOFF2 を返す。どちらも 11 個に分けている。
  確認に使ったリリースは [v2026-09-24-unicode18_0](https://github.com/googlefonts/noto-emoji/releases/tag/v2026-09-24-unicode18_0) と [google/fonts の METADATA.pb](https://github.com/google/fonts/blob/e7db6ca603fd4862010383ff17ffd6cc4ffac1c0/ofl/notocoloremoji/METADATA.pb) である。
- 2 つのファイルはコミット（noto-emoji は `e20cbc2bbec1926686be9f9bee7d1d2cfa1fea0e`）と SHA-256 で固定し、§1 の `EMOJI_VERSION`・`CLDR_VERSION` と同じタスクで取得する。
  生成器は、`EMOJI_VERSION` の `emoji-test.txt` のすべての fully-qualified の列が両方のフォントで 1 つのグリフになることを確かめ、そうでなければ失敗する。
  `EMOJI_VERSION` を上げるのは、それを描ける Noto のリリースが出てからにする。

### 9. 分割と生成

- `emoji-test.txt` のグループごとに分ける。`Flags` は、国旗（地域指示記号の対、`U+1F1E6..1F1FF`）とそれ以外に分ける。
  各列はそのグループの分割に入れ、分割の `unicode-range` は、その分割の列に現れるすべての符号位置にする。ZWJ（`U+200D`）・`U+FE0F`・肌の色の修飾子は、複数の分割に重なって入る。
  重なり方は Google Fonts と同じである。Google Fonts の分割も `U+200D` を複数の `unicode-range` に入れている。分割をまたぐ ZWJ の列が別々のグリフに割れないように、列を丸ごと 1 つの分割に入れる。
- `fontTools.subset`（`pyftsubset` と同じもの）で、分割ごとに GSUB の閉包を保ったまま部分集合を作り、WOFF2 にする。
  COLRv1 の版は `SVG` の表を、SVG の版は `COLR` と `CPAL` の表を落とす。
- **大きさ（2026-10-24 に試しに生成して計測）**:

  | 形式 | 合計 | 最大の分割 | 顔と感情の分割 | Google Fonts の配信（11 分割の合計） |
  | --- | --- | --- | --- | --- |
  | COLRv1 | 1.96 MB | 国旗 693 KB | 75 KB | 2.08 MB |
  | SVG | 3.72 MB | 国旗 1,152 KB | 153 KB | 3.95 MB |

  COLRv1 は、分けないときの WOFF2 が 1.90 MB で、分割で増えるのは 3% である。
- **道具**: fontTools（MIT）、brotli（MIT）、SVG の部分集合に要る lxml（BSD-3-Clause）を、生成のときだけ使う。どれも AGPL-3.0 と両立する。
  リポジトリの道具に Python は無い（`.tool-versions`）。そこで、`scripts/emoji-fonts` を `uv` のロックファイルで版を固定した使い捨ての環境で動かす。
  生成した WOFF2 と `@font-face` の CSS はコミットする。`scripts/task.sh check` とビルドに Python は要らない。サーバーには何も足さない。
- 置き場所は `@my/ui`（`app/ui/src/fonts/emoji/` と `app/ui/src/emoji.css`、`@my/ui/emoji.css` として公開）にする。タイポグラフィは `@my/ui` が持ち、Storybook のスクリーンショットも同じフォントで撮れる。
  Vite が URL にハッシュを付けて `dist/assets/` に出す。
- **`emoji.css` を読み込む場所**: Web クライアントの `app/web/src/index.css` と Storybook の `app/ui/src/storybook.css` が、`theme.css` の後に `emoji.css` を読み込む。
  - `theme.css` は `emoji.css` を読み込まない。`--font-sans` と `--font-mono` の先頭に `"my Noto Color Emoji"` を置くのは `theme.css` のままにする（§10）。フォントの並びとサーバー単位のテーマのトークンを 1 か所に保つため。
  - サーバーが描画する画面（`server-pages.css`）は `emoji.css` を読み込まず、OS の絵文字のフォントで描く。ファミリー名は `--font-sans` に残るが、face が無いので飛ばされる。

### 10. `@font-face` とフォントの並び

- **ファミリー**: 分割ごとに、`font-family: "my Noto Color Emoji"` の `@font-face` を 1 つ置く。OS に入っている同じ名前の CBDT の Noto（Linux・Android）と区別するため、ファミリー名を変える。
- **`src`**: `src: url(<colrv1>.woff2) format("woff2") tech(color-COLRv1), url(<svg>.woff2) format("woff2") tech(color-SVG)` とする。
  ブラウザーは、描ける最初の項目を選ぶ（Firefox は COLRv1 に対応するので最初の項目を選ぶ）。
  対応する版は次のとおりである（2026-10-24 に MDN の [browser-compat-data](https://github.com/mdn/browser-compat-data/blob/8371d582c202efe1f21622ab8cfac6ab00a7b209/css/at-rules/font-face.json) と caniuse の [colr-v1](https://github.com/Fyrd/caniuse/blob/664a78d75bcf581e1c6279144ad66b797a1635ab/features-json/colr-v1.json) で確認）。

  | 機能 | Chrome | Firefox | Safari | 備考 |
  | --- | --- | --- | --- | --- |
  | `src` の `tech()` | 108 | 107 | 17 | |
  | COLRv1 | 98 | 107 | TP まで未対応 | iOS の Safari も 27.2 まで未対応。Android の Chrome・WebView は対応 |

- **`unicode-range`**: 次の符号位置は、本文の文字として使われるので外す。`#`・`*`・`0`〜`9`（キーキャップの基）と、`©`・`®`・`™`・`‼`・`⁉`・`↔` など `emoji-test.txt` で既定がテキストの表示の記号のうち、本文の記号として使われるものである。
  外した字の絵文字の表示（`#️⃣`、`©️`）は、OS の絵文字のフォントで描く。外す字の一覧は、生成器の定数 `TEXT_CODE_POINTS` として 1 か所に置く。
- **並び**: `--font-sans` と `--font-mono` の先頭に `"my Noto Color Emoji"` を置く。`unicode-range` が絵文字の符号位置だけなので、ほかの字には効かない。
  先頭に置かないと、`☀`・`♠`・`❤` のように CJK のフォントや Segoe UI が字形を持つ字が、そちらのフォントで描かれる。末尾の OS の絵文字のフォント（`"Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji"`）は、フォントを読めないときのために残す。
  ADR-0009 の「フォントは OS フォントを優先し」は、絵文字の符号位置についてだけこの ADR が置き換える。本文の文字は OS のフォントのままにする。
- **`font-display: swap`**: 読み込み中は OS の絵文字で描き、届いたら入れ替える。`block` にすると、遅い回線で絵文字が最大 3 秒見えない。
  入れ替えで行の高さが変わらないように、`@font-face` に `ascent-override`・`descent-override` を付けて OS の絵文字に近づける。
- **キャッシュ**: Service Worker（ADR-0028）は `**/*.woff2` を precache するが、絵文字の WOFF2 を `globIgnores` で外す。2 つの形式の約 5.7 MB を、インストールのたびに取らないためである。
  代わりに、実行時に cache-first で持つ。URL にハッシュが付くので、内容が変わらない限り再取得しない。ほかの WOFF2 の precache はそのままにする。
- **使えないとき**: フォントを読めない場合は、OS の絵文字に落ちる。この場合、新しい絵文字が豆腐になり得る。
  Safari 16 以前（`tech()` を知らない）も OS の絵文字に落ちることを想定するが、振る舞いは要検証である（Open Questions）。

### 11. ピッカーでのフォントの読み込み

- ピッカー・補完・リアクション・投稿の本文は、同じ `--font-sans` で Noto の形になる。
- フォントを読めない環境で豆腐になる絵文字があるのは、§10 の「使えないとき」に書いた例外として受け入れる。
- ピッカーを初めて開いたときに読む分割は、ブラウザーで違う。Firefox は描いた行（§15）の絵文字のグループの分割を読む。Chromium は `U+FE0F` を `unicode-range` に持つ分割（§9。country-flags と component を除く 9 個）をまとめて読むので、11 分割のうち 9 分割、COLRv1 で約 1.35 MB を読む。カスタム絵文字の節が無ければ、初めの行に `U+FE0F` を含む `❤️` と `☺️` が必ず入るためである。
  すべての分割は COLRv1 で約 2 MB、SVG で約 3.7 MB である。
- この値は、`emoji.css` と本物のフォントを読む静的なページに、初めて開いたときの行の近似（よく使うの既定の 12 個と顔と感情、計 10 行 × 8 列）を置いて計測した（2026-11-04。Firefox は同じ内容で 4 分割）。本物のピッカーの画面では計測しておらず、WebKit は未計測である。

### 12. ADR-0092 の強調表示との関係

ADR-0092 §4 の強調表示の層は、`textarea` と同じフォント・字間・行の高さで文字を並べる。`--font-sans` の先頭の `@font-face` は両方に同じく効く。
ただし、`font-display: swap` の入れ替えの前後で字の幅が変わる。このため、層の位置合わせは、フォントの読み込み（`document.fonts` の `loadingdone`）の後にも測り直す。

### 13. フォントのライセンスと表示

- Noto Color Emoji は SIL Open Font License 1.1 で、Reserved Font Name を定めていない。部分集合を作っても名前を変える義務は無い（2026-10-24 に noto-emoji の [2D/fonts/LICENSE](https://github.com/googlefonts/noto-emoji/blob/e20cbc2bbec1926686be9f9bee7d1d2cfa1fea0e/2D/fonts/LICENSE) と google/fonts の [OFL.txt](https://github.com/google/fonts/blob/e7db6ca603fd4862010383ff17ffd6cc4ffac1c0/ofl/notocoloremoji/OFL.txt) で確認）。
- OFL 1.1 は、各複製に著作権の表示と許諾の文を含めれば、ソフトウェアと一緒に配ることを許す。フォントはプログラムと結合せず、別のファイルとして配るので、AGPL-3.0 のリポジトリと両立する。
- WOFF2 の隣に `OFL.txt`（著作権の表示を含む）を置き、ビルドの成果物にも含める。§1 の Unicode のライセンスの表示と同じ場所に載せる。

### 14. グリッドの行の並び

- グリッドは 1 本の行の並びにする。節ごとに、見出しの行を 1 つと、その節の絵文字を N 個ずつに切った絵文字の行を続ける。検索中は「検索結果」の見出しの行と結果の絵文字の行だけになる（§3）。
- 列の数 N は、グリッドの内側の幅から計算する。`ResizeObserver` でグリッドの幅と、画面に出さない見本のセル 2 つ（`aria-hidden`、同じクラス）の大きさと間隔を測り、`N = floor((幅 + 間隔) / (セルの大きさ + 間隔))`（1 以上）とする。
  セルの大きさはフォントの大きさの設定と `pointer-coarse:` で変わる（§4・web-client D4 の 40 px / 44 px）ので、固定の値を使わずに測る。幅を測れない環境（jsdom）では N = 8（22 rem のポップオーバーの列の数）とする。
  ADR-0110 の「部品はコンテナクエリ」は、部品が画面ではなく自分の幅に応じることを求めるもので、この計算も部品の幅に応じる。
- 行の高さは固定にする。絵文字の行はセルの高さと間隔、見出しの行は見本の見出しの高さで、どちらも同じ測り方で得る。行ごとの `measureElement` は使わない。
  高さが決まっているので、スクロールの位置から描く範囲と、見出しの位置が計算だけで決まる。
- グリッドの高さの上限は `max-h-64` で、中身が短ければ縮む。

### 15. 仮想化

- `@tanstack/react-virtual` の `useVirtualizer` で、グリッドの要素をスクロールの要素にする。行は絶対配置（`translateY`）で、高さの合計を持つ入れ物の中に置く。
- `overscan` は 4 行（`Feed` と同じ）。見える行（上限の高さで 6 行ほど）と前後の 4 行ずつを描く。
- **フォーカスのあるセルの行は表示範囲の外でも描く**。`rangeExtractor` で、既定の範囲（`defaultRangeExtractor`）に今のセルの行を足す（`Feed` と同じ。web-client Y6 と同じ考え）。
  ホイールやタッチでスクロールしてフォーカスのあるセルが見えなくなっても、その要素は DOM に残り、フォーカスは失われない。スクロールして戻ると同じ要素がそのまま見える。
- 仮想化を読むフックは、`Feed` と同じく `"use no memo"` で React Compiler から外す（web-client R4）。仮想化の結果はそのフックの戻り値として読み、ほかのコードはコンパイラの対象のままにする。
- スクロールの滑らかさは CSS の `scroll-behavior` ではなく、TanStack Virtual の `scrollToIndex` の `behavior` で決める。CSS で滑らかにすると、仮想化の側が目標の位置に着いたかを確かめる処理と食い違うためである。
  カテゴリのボタンによる移動だけを、`prefers-reduced-motion: no-preference` のときに `smooth` にする（web-client D6）。キー操作による移動は即座にスクロールする。

### 16. ARIA の構造

- グリッドは `role="grid"`（`aria-label` は `labels.grid`）に `aria-rowcount`（見出しの行を含む行の総数）と `aria-colcount`（N）を付ける。
  APG の grid は、DOM に無い行や列があるときに総数を `aria-rowcount` / `aria-colcount`、位置を `aria-rowindex` / `aria-colindex` で示すことを求める（2026-10-24 に [APG の Grid Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/grid/) で確認）。
- 描いた行は節ごとに `role="rowgroup"`（`aria-label` は節の見出し）にまとめる。見出しの行が範囲の外で DOM に無くても、節の名前が伝わるように `aria-labelledby` ではなく `aria-label` にする。
  行の入れ物（高さの合計を持つ要素）は `role="presentation"` にする。
- 見出しの行は `role="row"`（`aria-rowindex`）の中の `role="rowheader"`（`aria-colspan` は N）にする。見出しはフォーカスを受けない（APG の「機能の無い見出しのセル」）。
- 絵文字の行は `role="row"`（`aria-rowindex`）、各絵文字は `role="gridcell"`（`aria-colindex`、`aria-label` は §6 の名前）にする。カスタム絵文字の画像は中に置く。
- 一致する絵文字が無いときは、1 つの行とセルに文言（`labels.empty`）を出す。
- 件数の告知は §3 の `role="status"` のまま。

### 17. キー操作とフォーカス（roving tabindex）

- グリッドの中で Tab で止まるのは 1 つのセルだけにする。初めは最初の絵文字、その後は最後にフォーカスのあったセルが `tabindex="0"` になる。Tab の順は 検索欄 → 肌の色 → カテゴリのボタンの列 → グリッド。
- 今のセルは、節と絵文字の鍵で持つ。列の数が変わって行の切り方が変わっても、同じ絵文字に留まる。検索の文字列が変わって今のセルが結果に無くなったら、最初の絵文字を今のセルとみなす（state を Effect で直さず、描画の中で導く。web-client R6）。
- 矢印キーで DOM のフォーカスをセルに移す。移した先の行が描かれていなければ、今のセルを先に変えて（`rangeExtractor` がその行を描く）、描かれた後にフォーカスを移す。どちらも `focus({ preventScroll: true })` で移し、`scrollToIndex`（`align: "auto"`）でその行を見える位置に入れる。
- キーと動き（左右は書字方向に沿う。RTL では入れ替わる。React Aria の `useLocale` の向きを使う）:

  | キー | 動き |
  | --- | --- |
  | → / ← | 並びの次 / 前の絵文字。行の端では次 / 前の行へ、節の境では次 / 前の節へ続く（APG の layout grid の任意の折り返し）。最初と最後で止まる |
  | ↓ / ↑ | 次 / 前の絵文字の行の同じ列（見出しの行は飛ばす）。その行が短ければ、その行の最後の絵文字。最後 / 最初の行で止まる。節の最初の行へ上がるときは、その節の見出しの行も見える位置に入れる |
  | Home / End | 今の行の最初 / 最後の絵文字（APG） |
  | Ctrl + Home / Ctrl + End | グリッドの最初 / 最後の絵文字（APG） |
  | Page Down / Page Up | 見えている高さに入る行の数（1 以上）だけ下 / 上の行の同じ列。行き先が見出しなら、その向きの次の絵文字の行。端を越えるときは最後 / 最初の絵文字の行 |
  | Enter / Space / 押下 | 選ぶ。Shift を押しながら（Shift+Enter・Shift+クリック）は閉じずに続ける。タッチでは続けない（§5） |

  文字の入力による移動（typeahead）は持たない。名前で探すのは検索欄が受け持つ。
- 押下は React Aria の `usePress` で受ける。Shift の有無と、押したのがタッチかどうかを `PressEvent` から読み、Enter と Space の扱いも React Aria にそろう。見た目の状態（フォーカスのリング・ホバー・押下）は `useFocusRing`・`useHover` の `data-*` で付ける。
- **roving tabindex を選ぶ理由**:
  - ピッカーの検索欄とグリッドは別の Tab の止まり場所で、文字の入力は検索欄、選ぶのはグリッドと分けている。検索欄から矢印キーで動かす形にすると、IME の変換の候補を矢印キーで選ぶ操作と競合し、変換中かどうか（`isComposing`）でキーの意味を変えることになる。
  - DOM のフォーカスがセルにあれば、支援技術はフォーカスの移動として読み上げる。`aria-activedescendant` は、iOS の VoiceOver などの対応がフォーカスの移動より弱く（要検証。手動の検査は ADR-0110 の手動の層で行う）、ADR-0110 の手動の検査の対象（VoiceOver・TalkBack・NVDA）の全部で確かめる手間が大きい。
  - Shift で続けて選ぶとき、フォーカスを選んだ絵文字に残す（§5）のは、DOM のフォーカスがそのまま表す。
  - 範囲の外でも今のセルの行を描くことは、どちらの方式でも要る。roving tabindex で増えるものはない。

### 18. カテゴリのボタンと `aria-current`

- カテゴリのボタン（§3）を押すと、その節の最初の絵文字を今のセルにしてフォーカスを移し、`scrollToIndex` でその節の見出しの行をグリッドの上端に合わせる（`align: "start"`）。
  最初の絵文字の行は、スクロールが着く前でも `rangeExtractor` で描かれているので、すぐにフォーカスを移せる。
- `aria-current="true"` は、スクロールの位置から導く。グリッドの上端の帯（高さの 10%）より上にある最後の見出しの行の節を、今の節とする。行の位置は計算で決まるので、`IntersectionObserver` は使わない。
  スクロールの位置が変わると仮想化が描き直すので、`aria-current` もスクロールに合わせて移る。検索中はボタンを無効にし、`aria-current` を付けない。

### 19. React Aria に残すもの・使わないもの

- 使う: 検索欄（`@my/ui` の `TextField` の `type="search"`）、肌の色の `MenuTrigger` / `Menu`、カテゴリのボタンの `Toolbar`、`Button`・`Tooltip`、呼び出し側の `Popover`、セルの `usePress`・`useFocusRing`・`useHover`、書字方向の `useLocale`。
- 使わない: `GridList`・`GridListSection`・`GridListHeader`・`GridListItem`。グリッドのキー操作・フォーカス・ARIA の属性は §16・§17 のとおり `EmojiPicker` が持つ。
- 仮想化は `EmojiPicker` の内側に閉じる。props と、`searchEmojis`・`skinToneVariant` などの公開する関数と、呼び出し側（投稿フォーム・編集・プロフィール・リアクション）はこれに左右されない。

### 覆る条件

- CLDR の版が `EMOJI_VERSION` に追いつかない期間が長く、名前の無い絵文字が目立つ場合は、`emojibase-data` を見直す。
- 端末をまたいで「よく使う」と肌の色をそろえる要望が多い場合、または Tauri のアプリで端末が増える場合は、サーバーの利用者の設定（`GET /preferences` に肌の色、よく使う絵文字は新しい現在値の表）を見直す。
- 仮想化してもピッカーを開くのが遅い（ミドルレンジの Android で 100 ms を超える）場合は、全集合をタブごとに分けて描く（タブを APG の tabs にする）形を見直す。
- TanStack Virtual の版の更新で、`rangeExtractor` や `scrollToIndex` の振る舞いが変わり、フォーカスを保てなくなった場合は、React Aria の `Virtualizer` と自作の `Layout` を見直す。
- React Aria Components の `GridLayout` が節を配置するようになった場合は、キー操作を React Aria に戻せるかを見直す。
- Safari が COLRv1 に対応したら、SVG の版をやめて COLRv1 だけにする（2026-10-24 時点で caniuse は Safari TP まで未対応）。
- `google/fonts` が SVG の表を持つファイルを配らなくなったら、noto-emoji の SVG の素材から nanoemoji（Apache-2.0）で作るかを見直す。
- 初めてピッカーを開いたときの転送量（SVG で全分割は約 3.7 MB）が、モバイルの回線で受け入れられないと分かったら、分割を細かくするか、ピッカーの描き方（§11）を見直す。

## 経緯

投稿フォームの絵文字の入力を、Mastodon の Web のピッカーのように、入力欄から開けて、カテゴリのタブ・よく使う絵文字・検索・肌の色の選択を持つものにする要望から始まった（#1040）。関係する要件は FR-101（投稿の作成）と FR-116（絵文字リアクション）。ADR-0092 の補完が同じデータを使うので、データの形を先に決めた。Mastodon v4.7.3 は `emoji-mart` と `emojibase-data` 16.0.3 を、Misskey 2026.10.0 は自前のピッカーと `@misskey-dev/emoji-data` を使う（2026-10-23 に各 `package.json` で確認）。

表示は、当初は OS のフォントで描き、OS が描けない版の絵文字を canvas で判定して隠すとした。その後、端末で形がそろわないこと、古い OS で新しい絵文字が豆腐になること、ADR-0110 のスクリーンショットが OS に左右されることから、すべての場所を Noto Color Emoji で描くと決めた（#1145）。Safari が COLRv1 を描けないので、COLRv1 と OpenType-SVG の両方を配る。

`emoji.css` は当初 `theme.css` から読むとしたが、`theme.css` はサーバーが描画する画面の `server-pages.css` も読み込む。#1176 はその画面のフォントと読み込む量を変えないことを求めたので、実装（PR #1182）に合わせて使う側が読み込む形にした。

グリッドは、当初はまず仮想化せずに描いて計測するとした。実装の後、開いたピッカーは数千の要素を持ち、CI の E2E で axe と横スクロールの検査が 30 秒の上限を超えて失敗した（`web-e2e.yml`、run 40094615925、40096646446）。Playwright の計測でも、開くまで CPU 4× で 2.6 秒、6× で 3.9 秒かかった（#1150）。そこで TanStack Virtual で仮想化すると決めた（#1202）。`@tanstack/react-virtual` は `@my/ui` の依存にすでにあり（`Feed`）、ADR-0025 の許可リストに載っている。

## 採らなかった案

- `emojibase-data`: MIT で生成済みでロケールも多いが、版の更新がパッケージの公開に縛られてサーバーの `EMOJI_VERSION` とずれ得る。`ja/compact.json` の 1 ファイルで gzip 後 84 KB あり、ADR-0025 の表にない依存でもある。
- `emoji-mart` などピッカーのライブラリごとの採用: 最も早く Mastodon 風の UI になるが、UI が React Aria と `@my/ui` の部品ではなく、ADR-0110 の品質ゲート（APG の grid、RTL、`ar-XB`、デザイントークン）を外から満たせない。ADR-0025 / ADR-0025 の外の依存になる。
- 画像（Twemoji など）: 形がそろうが、画像の配信（数千枚または大きなスプライト）、帰属表示（Twemoji の画像は CC-BY 4.0）、本文とのずれが生じる。
- 「よく使う」と肌の色をサーバーの設定に持つ: 端末をまたぐが、API とスキーマが増え、使った絵文字の履歴をサーバーが持つ。
- 開くボタンを道具の行に置いたままにする: 変更は小さいが、320 px の道具の行がきついままで、本文から離れる。
- COLRv1 だけを配る: 配る量が最小（1.96 MB）で出所も 1 つだが、Safari と iOS（Tauri を含む）は OS の絵文字のままになる。
- OS のフォントのまま: 転送量も作業も無いが、端末で形が違い、古い OS で豆腐になる。
- Google Fonts から読む: 生成と保守が要らないが、`font-src 'self'`（ADR-0019 §9、ADR-0059）を緩める必要があり、閲覧のたびに Google に要求が届き、版を固定できない。
- CBDT（`NotoColorEmoji.ttf`）: 10.7 MB と最も大きく、macOS で描けるのは Chrome だけ（2026-10-23 に noto-emoji の README で確認）。
- `theme.css` から `emoji.css` を読む: 読み込みを忘れないが、サーバーが描画する画面も絵文字のフォントを持つ。
- サーバーが描画する画面に別の theme を作る: 画面は確実に分かれるが、トークンとサーバー単位のテーマの規則（`theme_tokens.gen.go` の生成元）が 2 つに割れる。
- React Aria の `Virtualizer` に節のあるグリッドの `Layout` を自作する: `GridList` のキー操作をそのまま使えるが、React Aria Components 1.21.1 の `GridLayout` は節を配置せず、`Layout` の抽象（`getVisibleLayoutInfos`・`getLayoutInfo`・`getContentSize`、`getKeyAbove` などの位置の問い合わせ）を書いて React Aria の内部に合わせて保守することになる。
- 節をタブで分けて描く: 仮想化が要らないが、最大の節だけで数百の絵文字があり、1 本のグリッドを続けてスクロールする形を崩す。
- 画面の外の節に `content-visibility: auto` を付ける: 変更は小さいが、要素の数も axe の検査の時間も変わらない。
- `aria-activedescendant`: フォーカスを動かさずに済むが、支援技術の対応にばらつきがあり（要検証）、検索欄から動かす形は IME と競合する。指す先のセルを範囲の外でも描くことは変わらない。

## Consequences（影響）

- 良い影響
  - カスタム絵文字の無いサーバーでも、投稿とリアクションで Unicode 絵文字を名前とキーワード（日本語を含む）で探せる。
  - サーバーとクライアントの絵文字の集合が 1 つの版でそろい、版の更新が 1 つの手順になる。
  - 新しい npm の依存を足さない。ADR-0092 の補完が同じデータと検索を使える。
  - 端末・OS の版に関わらず、絵文字が同じ形になる。古い OS でも `EMOJI_VERSION` の絵文字が豆腐にならない（`tech()` に対応するブラウザーでは）。
  - Storybook と E2E のスクリーンショット（ADR-0110）が、CI のコンテナの OS の絵文字に左右されない。第三者のオリジンに閲覧を知らせない。
  - サーバーが描画する画面は、絵文字のフォントを読み込まず、見た目も変わらない。トークンとフォントの並びは `theme.css` 1 か所で済む。
  - 開いたピッカーの要素の数が、全集合の大きさに依らず、見える行と前後の数行の分（百数十のセル）になる。描いていない行があっても、総数と位置が支援技術に伝わる。
- 悪い影響・トレードオフ
  - 生成器と生成したデータ（ロケールごとの JSON）を自前で保守する。CLDR の構造が変わると生成器を直す。
  - 「よく使う」と肌の色は端末ごとで、別の端末やブラウザーでは引き継がれない。
  - 本文の右上のボタンのぶん、本文の入力欄の幅が狭くなる。
  - 初めて開くときに 2 つのチャンクを取るので、遅い回線ではピッカーが開くまで待たせる（読み込み中の表示が要る）。
  - 絵文字を含む画面で、その分割を読む転送量が増える。顔と感情の分割は COLRv1 で 75 KB、SVG で 153 KB である。
  - `font-display: swap` の入れ替えで、絵文字の形が一度変わる。
  - OS の絵文字に慣れた利用者には、見慣れない形になる。利用者の設定で OS の絵文字に戻す選択肢は持たない。
  - フォントの出所が 2 つ（noto-emoji と google/fonts）あり、版（2.057 と 2.055）が違い得る。細部の形が形式によって違うことがある。
  - リポジトリに約 5.7 MB のバイナリが入り、Unicode の版を上げるたびに増える。生成に Python の道具（fontTools・brotli・lxml・uharfbuzz）が要る。
  - 通知（Web Push）・`document.title`・OS のネイティブの UI（`select` の一覧など）は OS のフォントで描かれ、Noto にならない。
  - Web クライアントの新しい入口（新しいエントリのスタイルシートや、別の Storybook）で Noto Color Emoji を使うには、そのスタイルシートが `@my/ui/emoji.css` を読み込む必要がある。読み忘れても壊れはしないが、絵文字が OS のフォントになる。
  - サーバー単位のテーマで `font-sans` / `font-mono` を上書きすると、`"my Noto Color Emoji"` を先頭に残さない限り、Web クライアントの絵文字も OS のフォントになる（[app/ui/README.md](../../app/ui/README.md#server-themes)・[運用の手引き](../operations/instance-settings.md#theme)）。
  - グリッドのキー操作・フォーカスの移動・ARIA の属性を自前で持つ。`GridList` が持っていた細かな扱い（typeahead など）は持たない。テスト（Testing Library のキー操作と Storybook の実ブラウザ）で固める。
  - React Compiler から外すフックが `Feed` のほかにもう 1 つある（web-client R4）。
  - ブラウザーのページ内検索（Ctrl+F）は、描いていない絵文字を見つけない。名前で探すのはピッカーの検索欄が受け持つ。
  - WKWebView で OpenType-SVG を、Android の WebView で COLRv1 を描く速さは実機で測っていない（#1150 は Playwright の計測で閉じ、Tauri のアプリは v0 の外）。ピッカーの分割の読み込みは、仮想化の後の本物の画面では計測しておらず（#1213 は必要になるまで閉じた）、§11 の値は静的なページでの計測で、WebKit は未計測である。Chromium は、仮想化しても初回に 11 分割のうち 9 分割を読む。

## Open Questions / 要検証

- `@font-face` の `src` の `tech()` を知らない Safari 16 以前で、`src` 全体が無効になって OS の絵文字に落ちるか、COLRv1 の項目を読んで何も描かないか（要検証。古い Safari の実機か BrowserStack が要る）。
- `aria-activedescendant` と roving tabindex の、iOS の VoiceOver と Android の TalkBack での読み上げの差（要検証。ADR-0110 の手動の層で、§17 の roving tabindex の読み上げを確かめる）。
