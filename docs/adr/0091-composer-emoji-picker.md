# ADR-0091: 投稿フォームの絵文字ピッカー（Unicode 絵文字の全集合・カテゴリのタブ・よく使う・肌の色）

- Status: Accepted（§2 の表示（B1 OS のフォント）を ADR-0095（絵文字を Noto Color Emoji で描く）が、§3 の仮想化しない描画を ADR-0097（絵文字ピッカーのグリッドを TanStack Virtual で仮想化する）が置き換える）
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1040

## Context（背景・課題）

投稿フォームの絵文字の入力を、Mastodon の Web のピッカーのように、入力欄から開けて、カテゴリのタブ・よく使う絵文字・検索・肌の色の選択を持つものにしたい（決定者の要望、#1040）。
関係する要件は FR-101（投稿の作成）、FR-116（絵文字リアクション）、カスタム絵文字の ADR-0051 §4、リアクションの ADR-0052 §5、i18n の ADR-0009、品質ゲートの ADR-0027 である。

いまの仕組み（[web-client §12](../architecture/web-client/12-compose.md)・[§13](../architecture/web-client/13-status-detail-and-actions.md)）:

- `features/emoji` の `EmojiPickerButton` は、`GET /custom_emojis` がピッカーに出す絵文字を返すときだけ、道具の行（`ComposeTools`）に出る。カスタム絵文字の無いサーバーではボタンが無い。
- `@my/ui` の `EmojiPicker` は、検索欄とカテゴリの見出しで分けたグリッド（React Aria の `GridList`、APG の grid）で、データを持たない。最近使った絵文字は localStorage（`my:emoji:recent`）に 16 個まで。
- Unicode 絵文字は、リアクションのピッカーの固定の「よく使う群」（`EmojiPicker` の `unicode`）だけで、全集合・カテゴリ・名前での検索・肌の色がない。投稿では OS の入力に任せている。
- サーバーは Unicode の `emoji-test.txt`（`tasks/go.yml` の `EMOJI_VERSION`、現在 18.0.0）から、リアクションの正規化の表を生成している（`scripts/emoji-table`、ADR-0052 §1）。

決める必要があるのは、Unicode 絵文字のデータの出所・ライセンス・置き場所と大きさ、表示の方法、ピッカーの構成、開き方と置き場所、入れ方、アクセシビリティ、リアクションとの共有である。
Issue #1041（[ADR-0092](0092-composer-autocomplete-highlight.md)、`:` からの絵文字の補完）が同じデータを使うので、データの形はここで先に決める。

## Decision Drivers（判断基準）

1. **サーバーと版がそろうこと**: ピッカーで選べる Unicode 絵文字は、サーバーがリアクションで受け入れて正規化できるもの（ADR-0052 §1）と同じ集合にする。
2. **i18n とアクセシビリティ**: 各絵文字に表示中のロケールの名前があり、その名前とキーワードで検索できる（ADR-0009）。APG の grid とキー操作、件数の告知、320 px（ADR-0027）。
3. **依存とライセンス**: ADR-0025 / ADR-0031 の許可の外のライブラリを足さない。データのライセンスが AGPL-3.0 と両立する（ADR-0011）。
4. **初期の読み込みに効かないこと**: ピッカーを開くまでデータを読まない。ロケールごとに分ける。
5. **作る量と保守**: Unicode の版の更新が 1 つの手順で済む。サーバーの API とスキーマを増やさない。

## Considered Options（選択肢）

論点ごとに選択肢を並べる。

### A. Unicode 絵文字のデータの出所

1. **A1 自前生成**: Unicode の `emoji-test.txt`（並び・グループ・肌の色の異体・絵文字の版）と CLDR の annotations / annotationsDerived（ロケールごとの名前とキーワード）から、リポジトリ内の生成器でクライアント用のデータを作る。
2. **A2 `emojibase-data`**: Unicode と CLDR から作られた、ロケールごとの JSON の npm パッケージを使う。
3. **A3 ピッカーのライブラリごと採用**: `emoji-mart` など、データと UI を持つライブラリを使う。
   Mastodon v4.7.3 の Web は、ピッカーに `emoji-mart`（`emoji-mart-lazyload`）を、絵文字のデータに `emojibase-data` 16.0.3 を使う
   （2026-10-23 に [package.json](https://github.com/mastodon/mastodon/blob/8ff11c2bb7b9f0714e947b10fa3629c6542c7121/package.json) で確認）。
   Misskey 2026.10.0 は自前のピッカー（`MkEmojiPicker.vue`）と自前のデータ・画像のパッケージ（`@misskey-dev/emoji-data`・`@misskey-dev/emoji-assets` 17.0.3）を使う
   （2026-10-23 に [packages/frontend/package.json](https://github.com/misskey-dev/misskey/blob/de89118a225fbe0833e42ab94e8d5af1e5211208/packages/frontend/package.json) で確認）。

### B. 表示

1. **B1 OS のフォント**: 文字のまま描く。OS が描けない新しい絵文字はピッカーから隠す。
2. **B2 画像（Twemoji など）**: 絵文字の画像セットを配る。

### C. 「よく使う」と肌の色の保存先

1. **C1 端末ごと（localStorage）**
2. **C2 サーバーの利用者の設定（`GET /preferences`）**: 端末をまたいで同じ。

### D. 開き方

1. **D1 本文の入力欄の右上（書字方向の終わり側）のアイコン**: Mastodon と同じ。カスタム絵文字が無いサーバーでも出す。
2. **D2 道具の行のまま**: カスタム絵文字の有無に関わらず出すように変えるだけ。

## Decision（決定）

**A1・B1・C1・D1 を採用する。** ピッカーは投稿・編集・プロフィール・リアクションで同じ `@my/ui` の `EmojiPicker` を使う。

### 1. データ（A1）

- 生成器は `scripts/emoji-table` を広げ（または同じタスクから呼ぶ別の Go のコマンドにし）、サーバーの表とクライアントのデータを **同じ `EMOJI_VERSION` の同じ `emoji-test.txt`** から一度に作る（判断基準 1）。
  CLDR の版は `CLDR_VERSION` として同じタスクに固定し、取得したファイルのダイジェストを確かめる（今の `go:emoji-table` と同じ）。タスクは `emoji:generate` のような名前にまとめ、版の更新は 2 つの変数を変えて実行するだけにする。
  CLDR は `unicode-org/cldr-json` のリリースの JSON（`cldr-annotations-full/annotations/<ロケール>/annotations.json` と `cldr-annotations-derived-full/annotationsDerived/<ロケール>/annotations.json`）を取る。
  CLDR 49（cldr-json 49.0.0、2026-10-22 公開）は、Emoji 18.0 の `emoji-test.txt` の絵文字（`Component` を除く）すべてに `en` と `ja` の名前を持つ（2026-10-23 に両方を取得して突き合わせて確認）。
  CLDR の annotations は VS16（U+FE0F）を外した列を鍵にするので、生成器は fully-qualified の列と VS16 を外した列の両方で引く。
- クライアントのデータは `app/web/src/features/emoji/unicode/` に生成してコミットする（`*.gen.json`）。
  - 基本のデータ（ロケールに依らない）: 書記素の列（fully-qualified）、グループ、並び順、絵文字の版、肌の色の異体の列。`Component` のグループ（肌の色の修飾子だけのものなど）は出さない。
  - ロケールごとのデータ: CLDR の短い名前とキーワード。`en` と `ja` を作り、`en-XA` / `ar-XB`（疑似ロケール）とほかのロケールは `en` に落とす。CLDR に名前が無い絵文字は `en`、それも無ければ `emoji-test.txt` の英語の名前を使う。
- グループの名前（9 個）と肌の色の選択の文言は、メッセージカタログ（`client` の名前空間）に足す（ADR-0009）。肌の色の 5 段階の名前は CLDR の修飾子の annotations を使う。
- データは `features/emoji` に置き、`@my/ui` に置かない。ロケールのデータは i18n のカタログに当たり、ADR-0031 が `@my/ui` に禁じるアプリの i18n への依存になる。`EmojiPicker` は今と同じくデータを持たず、節（見出し・アイコン・絵文字の列）を受け取る。
- 読み込みは `import()` で、ピッカーを初めて開いたとき（とポインタが乗ったとき・フォーカスしたときの先読み）に、基本のデータと表示中のロケールのデータの 2 つのチャンクを取る。ADR-0092 の補完も同じモジュールを使う。
  大きさの上限は 2 つで gzip 後 100 KB とし、実装の PR で計測して示す。
  Emoji 18.0 と CLDR 49 から試しに生成した詰めた JSON は、基本のデータ（基の絵文字 1,923 個と肌の色の異体 2,040 個）が gzip 後 12.7 KB、
  `ja` の名前とキーワードが 37.7 KB、肌の色の異体の名前が 12.4 KB で、合わせて約 63 KB（2026-10-23 に計測。`en` もほぼ同じ）。
- ライセンス: Unicode のデータ（`emoji-test.txt`）と CLDR は Unicode License v3（SPDX `Unicode-3.0`、OSI 承認。2026-10-23 に [SPDX](https://spdx.org/licenses/Unicode-3.0.html) と [unicode.org/license.html](https://unicode.org/license.html) で確認）で、
  複製に著作権と許諾の表示を添えることを条件に改変・再配布を許すので AGPL-3.0 と両立する。生成したデータの隣に表示の文（`LICENSE-Unicode.txt` など）を置き、Web のライセンス表示にも載せる。
  npm の依存ではないので `web:licenses` の許可リスト（`NPM_ALLOWED_LICENSES`）は変えない。

A2 を選ばない理由: `emojibase-data` は MIT で AGPL-3.0 と両立し、中身は同じ Unicode と CLDR だが、版の更新がパッケージの公開に縛られ、サーバーの `EMOJI_VERSION` とずれ得る（判断基準 1）。
大きさも `ja/compact.json` の 1 ファイルで gzip 後 84 KB あり、A1 の 3 つのチャンクの合計より大きい。ADR-0025 の表にない依存を足すことにもなる。
A3 を選ばない理由: UI が React Aria と `@my/ui` の部品ではなく、ADR-0027 の品質ゲート（APG の grid、RTL、`ar-XB`、デザイントークン）を外から満たせない。ADR-0025 / ADR-0031 の外の依存になる。

### 2. 表示（B1）

- Unicode 絵文字は OS のフォントの文字で描く。投稿の本文の絵文字も OS のフォントで描いているので、ピッカーで見た形と投稿で見える形が一致する。
- OS が描けない絵文字（豆腐になるもの）はピッカーと補完から隠す。ピッカーを開いたとき、新しい版から順に、版ごとの代表の 1 字を canvas に描いて色の付いた絵文字として描けるかを調べ、描ける最も新しい版より新しい絵文字を出さない。
  canvas が使えない（指紋採取の対策の拡張機能などで例外になる）ときは、すべてを出す。結果はそのタブの間だけ持つ。
  同じ方法を `emoji-picker-element` が使い（[determineEmojiSupportLevel.js](https://github.com/nolanlawson/emoji-picker-element/blob/5d1c8bfc21e737c03a3cb14e06b4e472e2275ff1/src/picker/utils/determineEmojiSupportLevel.js)）、
  Mastodon v4.7.3 もその判定を移して使う（[mode.ts](https://github.com/mastodon/mastodon/blob/8ff11c2bb7b9f0714e947b10fa3629c6542c7121/app/javascript/mastodon/features/emoji/mode.ts)。どちらも 2026-10-23 に確認）。
- B2 は形が端末をまたいでそろうが、画像の配信（数千枚または大きなスプライト）、帰属表示（Twemoji の画像は CC-BY 4.0。2026-10-23 に [jdecked/twemoji の LICENSE-GRAPHICS](https://github.com/jdecked/twemoji/blob/b6b55fef1e8636b540a6d016a4729ca8cdf2e60b/LICENSE-GRAPHICS) で確認）、投稿の本文との見た目のずれが生じる。採らない。

### 3. ピッカーの構成

- **並び**: よく使う → カスタム絵文字（ADR-0051 のカテゴリごとの見出し。カテゴリの無いものが先）→ Unicode の 9 グループ（`emoji-test.txt` の順: 顔と感情・人と体・動物と自然・食べ物と飲み物・旅行と場所・活動・物・記号・旗）。
  カスタム絵文字が無いサーバーでは、その節とタブを出さない。
- **カテゴリのタブ**: グリッドの上に、節ごとのアイコンのボタンの列（React Aria の `Toolbar`）を置く。アイコンは `Icons.tsx` の Phosphor（ADR-0043。絵文字そのものは OS ごとに形と色が変わり、強制カラーモードで潰れるので使わない）、名前は節の見出しと同じ文言でツールチップを持つ。
  1 つのグリッドを続けてスクロールする形なので、APG の tabs（パネルを切り替える）ではなく、押すとその節の見出しまでグリッドをスクロールしてその節の最初の絵文字にフォーカスを移すボタンにする。
  見えている節のボタンに `aria-current="true"` を付け、スクロールに合わせて移す（見出しの交差の監視）。スクロールの動きは `motion-safe:` のときだけ滑らかにする。
- **よく使う**: 今の「最近使った」を、回数と新しさ（使った時刻で減衰させた回数）で並べる「よく使う」に置き換え、32 個まで持つ。Unicode 絵文字（肌の色を外した基の列）とこのサーバーのカスタム絵文字（shortcode）を同じ列に混ぜる。
  まだ無いときは、今のリアクションの「よく使う群」を既定の列として出す。
- **検索**: 検索欄の文字列を NFKC・小文字・カタカナをひらがなにそろえてから、カスタム絵文字の shortcode と別名、Unicode 絵文字の表示中のロケールと `en` の名前とキーワード、絵文字そのものと比べる。
  並びは「名前の完全一致 → 名前の前方一致 → キーワードの前方一致 → 部分一致」の順、同じ順位の中は既定の並び（カスタム絵文字が先）。検索中はタブを無効にし、結果だけの 1 つの節にする。
  結果の件数は検索が止まってから `role="status"` で告げる（「N 件の絵文字」）。
- **肌の色**: 検索欄の隣に、今の肌の色の手（✋ と選んだ修飾子）を見せるボタンを置き、`Menu`（単一選択）で 6 つ（既定の黄色と 5 段階）から選ぶ。選んだ色は保存し、異体のある絵文字はその色で表示して入れる。
  複数の人の絵文字は全員に同じ色を付ける（人ごとの色の組み合わせは選ばせない）。
- **性能**: Unicode の全集合（肌の色の異体を除いて 1,923 個）と多数のカスタム絵文字を出す。
  React Aria Components 1.21.1（`react-stately` 3.50.0）の `Virtualizer` の `GridLayout` は節（`section` と `header`）を配置せず、節を扱うのは 1 列の `ListLayout` だけである
  （2026-10-23 に `app/node_modules` の `react-stately/dist/private/layout/GridLayout.mjs` と `ListLayout.mjs` で確認）。
  `@tanstack/react-virtual` で描かない行は、`GridList` が矢印キーでフォーカスを移す先の要素が無くなるので、キー操作を React Aria から引き取ることになり使わない。
  そこで、まず仮想化せずにすべてを描いて、ピッカーを開くまでの時間を計測する。遅ければ、React Aria Components が公開する `Layout` を継承した「節のあるグリッド」のレイアウトを `@my/ui` に書いて `Virtualizer` に渡す。

### 4. 置き場所と開き方（D1）

- 本文の `TextArea` の右上（書字方向の終わり側。論理プロパティ `inset-inline-end`）に、アイコンだけの絵文字のボタン（`aria-label` と同じ文言のツールチップ）を置く。カスタム絵文字の有無に関わらず出す。
  本文の入力が右上のボタンの下に回り込まないように、入力欄の終わり側に余白を取る。道具の行から絵文字のボタンを外すので、320 px のシートの道具の行が 1 つ短くなる（§12 の「送信が最初の画面に入る」を楽にする）。
- 同じボタンを、投稿の編集のダイアログの本文、プロフィールの編集の表示名と自己紹介に置く。CW と投票の選択肢には置かず、ADR-0092 の `:` からの補完と OS の入力に任せる（1 行の欄にボタンを並べると 320 px で欄が狭くなる）。
- ピッカーは `@my/ui` の新しいデザインの `Popover`（#991）で開き、検索欄にフォーカスを移す。幅は画面の幅から余白を引いた幅と 22 rem の小さいほうにし、列の数はコンテナクエリで決める（ADR-0027）。
  ADR-0039 の `md` 未満のシートの中で開くので、シートの上に重なるポップオーバーとして画面に収まることを E2E の 320 px で確かめる。

### 5. 入れ方

- Unicode 絵文字はその文字（肌の色を付けたもの）を、カスタム絵文字は `:shortcode:` を、カーソルの位置（選択があればその範囲）に入れる。カスタム絵文字の前後の空白の規則は今のまま（§12）。Unicode 絵文字には空白を足さない。
- 選ぶとピッカーを閉じて入力欄にフォーカスを戻す（今と同じ）。Shift を押しながら選ぶ（Shift+Enter・Shift+クリック）と、閉じずに続けて選べ、フォーカスはグリッドに残る。
  タッチの端末には続けて選ぶ操作を設けず、1 つ選ぶごとに閉じる。

### 6. アクセシビリティ

- 各絵文字の名前は、Unicode 絵文字は表示中のロケールの CLDR の短い名前（肌の色の付いたものは CLDR の annotationsDerived の名前）、カスタム絵文字は `:shortcode:`。
- グリッドは今の APG の grid の操作（Tab で検索欄 → タブの列 → グリッド、矢印キーは書字方向に沿う、Home / End、Enter・Space・押下で選ぶ）を保つ。
- ADR-0027 の層（Testing Library のキー操作、Storybook の axe、3 幅 × ライト / ダーク × LTR / RTL のスクリーンショット、E2E の 320 px と 4 ロケール）を通す。

### 7. リアクション

リアクションのピッカー（ADR-0052 §5、§13）も同じピッカーにし、固定の「よく使う群」を全集合とよく使う・検索・肌の色に置き換える。
データがサーバーと同じ `EMOJI_VERSION` から作られるので、選べる Unicode 絵文字はすべてサーバーが受け入れる。リモートのカスタム絵文字は今と同じく出さない。

### 覆る条件

- CLDR の版が `EMOJI_VERSION` に追いつかない期間が長く、名前の無い絵文字が目立つ場合は、A2（`emojibase-data`）を見直す。
- 端末をまたいで「よく使う」と肌の色をそろえる要望が多い場合、または Tauri のアプリで端末が増える場合は、C2（`GET /preferences` に肌の色、よく使う絵文字は新しい現在値の表）を見直す。
- 仮想化してもピッカーを開くのが遅い（ミドルレンジの Android で 100 ms を超える）場合や、節のあるグリッドのレイアウトの保守が重い場合は、全集合をタブごとに分けて描く（タブを APG の tabs にする）形を見直す。

## Consequences（影響）

- 良い影響
  - カスタム絵文字の無いサーバーでも、投稿とリアクションで Unicode 絵文字を名前とキーワード（日本語を含む）で探せる。
  - サーバーとクライアントの絵文字の集合が 1 つの版でそろい、版の更新が 1 つの手順になる。
  - 新しい npm の依存を足さない。ADR-0092 の補完が同じデータと検索を使える。
- 悪い影響・トレードオフ
  - 生成器と生成したデータ（ロケールごとの JSON）を自前で保守する。CLDR の構造が変わると生成器を直す。
  - 「よく使う」と肌の色は端末ごとで、別の端末やブラウザーでは引き継がれない。
  - OS が描けない新しい絵文字は隠すので、古い OS の利用者は新しい絵文字を選べない（他人の投稿では豆腐のまま見える）。判定の canvas の描画がピッカーを初めて開くときに少し時間を使う。
  - 本文の右上のボタンのぶん、本文の入力欄の幅が狭くなる。
  - 初めて開くときに 2 つのチャンクを取るので、遅い回線ではピッカーが開くまで待たせる（読み込み中の表示が要る）。
- 後続で必要になる作業（Issue 化する）
  1. データの生成: `scripts/emoji-table` の拡張（または同じタスクの別のコマンド）、`CLDR_VERSION` とダイジェスト、`features/emoji/unicode/` の生成物、Unicode のライセンスの表示、版の更新の手順（`docs/reference/commands.md`）。
  2. `@my/ui` の `EmojiPicker`: 節とタブの列とスクロールの同期、肌の色の `Menu`、開くまでの時間の計測（遅ければ節のあるグリッドの `Layout`）、件数の告知、Storybook とテスト。
  3. 投稿フォームの配置: 本文の右上のボタン、道具の行からの削除、編集のダイアログとプロフィール、よく使う（`my:emoji:recent` の形の変更。ADR-0012 のとおり移行はしない）、OS の対応の判定。
  4. リアクションのピッカーの置き換え（§13）。
  5. 文書の更新: [web-client §12](../architecture/web-client/12-compose.md) の「道具」と「カスタム絵文字のピッカー」を「絵文字のピッカー」に書き換え（置き場所・データ・構成・入れ方）、§13 のリアクションのピッカー、`tools/agentkit/context-map.yaml` の `features/emoji` の項目。
     ADR-0051 §4 の「最近使った絵文字はクライアントの端末ごとに持つ」はこの ADR の「よく使う」と矛盾しない（端末ごとのまま）ので、ADR-0051 は置き換えない。

## Pros and Cons of the Options（詳細比較・任意）

| | 良い点 | 悪い点 |
| --- | --- | --- |
| A1 自前生成 | サーバーと同じ版。依存なし。必要な項目だけで小さくできる（`ja` で約 63 KB） | 生成器の保守。CLDR の取り込みを自分で書く |
| A2 `emojibase-data` | 生成済みでロケールが多い（`ja` を含む）。MIT | 版がパッケージの公開に縛られ、サーバーとずれ得る。ADR-0025 の外の依存。`ja/compact.json` だけで gzip 後 84 KB |
| A3 ピッカーのライブラリ | 最も早く Mastodon 風の UI になる | React Aria・`@my/ui`・ADR-0027 の品質ゲートの外。ADR-0025 / ADR-0031 の外の依存 |
| B1 OS のフォント | 配信なし。投稿の本文と同じ形 | 端末で形が違う。新しい絵文字を隠す判定が要る |
| B2 画像 | どの端末でも同じ形 | 配信の重さ、帰属表示、本文とのずれ |
| C1 端末ごと | サーバーの変更なし。今の仕組みの延長 | 端末をまたがない |
| C2 サーバー | 端末をまたぐ | API・スキーマ（ADR-0020 の現在値の表）が増える。使った絵文字の履歴をサーバーが持つ |
| D1 入力欄の右上 | Mastodon と同じで見つけやすい。道具の行が空く | 入力欄が狭くなる。§12 の配置の変更 |
| D2 道具の行 | 変更が小さい | 320 px の道具の行がきついまま。本文から離れる |

## Open Questions / 要検証

- 仮想化せずにすべてを描いたとき、ミドルレンジの Android でピッカーを開くまでの時間（要検証。実機での計測が要る）。
- OS が描けない絵文字の canvas での判定の、Tauri の WebView（iOS / Android）での振る舞い（要検証。実機が要る）。
