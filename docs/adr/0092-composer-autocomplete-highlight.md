# ADR-0092: 投稿フォームの補完（@ アカウント・# ハッシュタグ・: 絵文字）と入力中の強調表示

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1041

## Decision（決定）

**専用の補完の API を正規化した前方一致の B-tree の索引で引き、UI は React Aria の `Autocomplete` に §3 の上書きをして使い、強調表示は `textarea` の後ろに重ねた層で塗る。**
IME とアクセシビリティを最も確実に守れるのが「ネイティブの `textarea` を保ち、その外に候補と強調を足す」形で、コストを抑えられるのが拡張なしの B-tree の前方一致だからである。

### 1. サーバーの補完の API（client API、ADR-0016）

**アカウント: `GET /api/v1/accounts/search`**（スコープ `read:accounts`、トークン必須）

- 引数: `q`（`@` を除いた 1〜100 文字。`user`・`user@dom`・表示名の先頭）、`limit`（既定 8、最大 20）。ページングはしない。
- 対象: このサーバーが知っていて、使えるハンドルを持つアカウント（ローカル・外。インスタンスのアクターを除く）。凍結・削除・消去中のアカウント、停止したドメインのアカウントは出さない。
- 照合: 次の語の**前方一致**。
  - ユーザー名（`username_normalized`）。`q` に `@` があれば `username@domain` を、ドメインの部分も前方一致で照合する。
  - 表示名を NFKC + 小文字化し、`:shortcode:` を除いた全体と、空白で区切った各語（ADR-0009 §6 と同じ正規化。CJK の表示名は語に分けないので、先頭からの前方一致になる）。
- 並び: 完全一致 → 閲覧者がフォローしている → ローカル → フォロワーの多い順。
  Mastodon も完全一致を先頭に置き、フォローの関係があるアカウントを先にしてからフォロワーの数などで並べる（2026-10-23 に [`AccountSearchService`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/services/account_search_service.rb) と [`Account::Search`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/models/concerns/account/search.rb) で確認）。
- 除外: 閲覧者がブロックしている・閲覧者をブロックしている・閲覧者がミュートしている（期限内）アカウント。Mastodon のアカウントの検索はブロックとミュートを除かない（同じ版で確認）が、補完ではメンションしても届かない・見たくない相手なので除く。
- `indexable`（FR-412、ADR-0057）は補完に効かせない。メンションの補完は、投稿から辿れるハンドルを打つ手間を省くだけで、検索エンジンや全文検索への掲載ではないため。Mastodon もアカウントの検索で `indexable` を見ず、`discoverable` は自己紹介の全文検索にだけ効かせる（2026-10-23 に上の `Account::Search` の `searchable_text` で確認）。
- 解決はしない（`resolve` を持たない）。Mastodon の Web の補完も `resolve: false`・`limit: 4` で、200 ms の間引きと前の問い合わせの取り消しをしている（2026-10-23 に [`actions/compose.js`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/javascript/mastodon/actions/compose.js) の `fetchComposeSuggestionsAccounts` で確認）。
  `q` が完全な `user@domain` で候補に完全一致がないときは、クライアントが既存の `GET /accounts/lookup`（`resolve=true`）を 1 回だけ呼び、見つかれば候補の先頭に足す。
  解決の上限と 429、連合の向き（`off` / `outbound` では `resolve` を送らない、ADR-0006）は既存の lookup の規則をそのまま使う。

**ハッシュタグ: `GET /api/v1/tags/search`**（スコープ `read:statuses`、トークン必須）

- 引数: `q`（`#` を除いた 1〜100 文字。`NormalizeTag` で正規化）、`limit`（既定 8、最大 20）。
- 対象: `public` / `local` / `unlisted` の投稿に今付いているタグだけ（`private` / `direct` だけのタグは、名前から投稿の存在が漏れるので出さない）。
- 並び: 完全一致 → 使われている投稿の数の多い順 → 最近使われた順。
- 閲覧者が使ったタグを先にするのは、クライアントが端末ごとに最近使ったタグ（localStorage、`my:compose:recent-tags`、16 個まで。ADR-0091 の「よく使う」と同じく端末ごと）を持ち、候補の中で先に並べることで行う。サーバーには持たせない。

#### データ（ADR-0020）

- `account_search_terms`（**投影**、`account` モジュールが所有）: アカウントごとに照合する語の行（`account_id`・`term`・`kind`（`username` / `display_name`）・`is_local`）。
  プロフィールの版・ハンドルの確認・ライフサイクルのイベントから同期で更新する（ADR-0020 規約 7）。凍結・削除で行を消す。
- `tag_usage`（**投影**、`feed_tag_entries` を持つ `timeline` モジュールが所有）: タグごとの `public` / `local` / `unlisted` の投稿の数と最後に使われた日時。`feed_tag_entries` と同じ投影の更新で増減する。数が 0 のタグは行を消す。
- 索引: 両方とも `term`（`tags` は既存の `name_normalized`）の B-tree で、`term >= q AND term < q || U+10FFFF` の範囲で引く。PostgreSQL では列を `COLLATE "C"` にしてバイト順にし、SQLite は既定の `BINARY` を使う。
  拡張（`pg_trgm`・FTS5）は使わない。FTS5 の trigram は 3 文字未満の部分文字列に一致しない（[SQLite FTS5 §4.3.4](https://www.sqlite.org/fts5.html)、2026-10-23 確認）ので、1〜2 文字から出す補完には合わない。
- 読み取りの量: 範囲の先頭から索引順に最大 100 行を読み、アプリケーション層で並べ替える。ブロック・ミュート・フォローの関係は、その候補の ID だけを `graph` の `internal/app` に問い合わせる（モジュールをまたぐ唯一の経路、ADR-0002）。
- 部分一致（表示名の途中の語でない位置）は扱わない。汎用の検索は別の ADR で決める。

#### レート制限とキャッシュ

- 両方の操作はアカウント単位の既存のレート制限に入れる。補完の問い合わせは、デバウンス 200 ms と次項のクライアントのキャッシュで、1 回の投稿でおよそ数回〜十数回に収まる見込みで、専用の枠は設けない。
  利用の計測で既存の枠を圧迫すると分かったら、操作ごとの枠を足す。
- 応答は閲覧者ごとに違うので `Cache-Control: private, max-age=30` にする。クライアントは TanStack Query で `q` ごとに 60 秒の `staleTime` を持ち、文字を消して戻ったときに問い合わせない。

### 2. カスタム絵文字と Unicode 絵文字の補完

- `:` の後に 2 文字以上の shortcode の文字（`[A-Za-z0-9_]`）が続き、`:` の前が英数字でも `:` でもないとき（サーバーの `ScanShortcodes` と同じ条件）に開く。
- 候補はクライアントの手元のデータから出し、サーバーに問い合わせない: `GET /custom_emojis`（ピッカーと同じ query）と、[ADR-0091](0091-composer-emoji-picker.md) が決める Unicode 絵文字のデータ（CLDR の名前とキーワード。表示中のロケールと `en`）。
- 照合は、カスタム絵文字は shortcode と別名、Unicode 絵文字は CLDR の名前とキーワードに対して、前方一致を先、部分一致を後にし、[ADR-0091](0091-composer-emoji-picker.md) の「よく使う」（端末ごとの localStorage）にある絵文字を先に並べる。最大 8 件。
- カスタム絵文字は `:shortcode:` を、Unicode 絵文字は文字そのもの（肌の色を持つ絵文字は ADR-0091 で選んだ肌の色を当てたもの）を入れる。

### 3. 補完の UI

- **ロール**: `textarea` に `role="combobox"` を付けない。ARIA in HTML は `textarea` に `textbox` 以外のロールを許さない（[ARIA in HTML](https://www.w3.org/TR/html-aria/)、2026-10-23 確認）。
  暗黙の `textbox` のまま、`aria-autocomplete="list"`・`aria-controls`（候補の `listbox`）・`aria-activedescendant` を付ける（いずれも WAI-ARIA 1.2 の `textbox` が持てる属性、[WAI-ARIA 1.2 §5.4 textbox](https://www.w3.org/TR/wai-aria-1.2/#textbox)、2026-10-23 確認）。
  `textbox` は `aria-expanded` を持てないので、開いたことと件数は `role="status"` の告知（「候補 5 件」）で伝える。候補は `role="listbox"` の `role="option"` で、選ばれた候補に `aria-selected="true"` を付ける（APG の combobox の listbox の部分）。
- **作り方**: React Aria Components の `Autocomplete`（`react-aria-components` 1.21.1 に安定版として入っている）を `TextArea` と `ListBox`（仮想フォーカス）に使う。公式の文書は `inputValue` に入力の一部を渡して `@` のメンションのような行内の補完を作る例を `TextArea` で示している
  （[React Aria Autocomplete](https://react-aria.adobe.com/Autocomplete)、2026-10-23 確認）。
  リポジトリが固定する `react-aria` 3.52.1 の `useAutocomplete`（`dist/private/autocomplete/useAutocomplete.mjs`、2026-10-23 確認）は、入力に `role` を付けず、`aria-autocomplete="list"`・`aria-controls`・`aria-activedescendant` を付け、
  `isComposing` の間はキーを処理せず、仮想フォーカスのある候補を Enter で選び、Tab は候補に渡さない。上の要件に合う。一方で次の点は合わないので、`TextAreaAutocomplete` で上書きする。
  - 入力に `spellCheck="false"`・`autoCorrect="off"`・`enterKeyHint="go"` を付ける。投稿の本文ではスペルチェックと自動修正を保ち、改行のキーを `go` にしないため、`TextArea` の側で戻す。
  - IME の判定は `isComposing` だけで、`keyCode === 229` を見ない（下の Safari の問題）。`onKeyDown` の前に 229 を見て止める。
  - 候補の一覧がある間は上下の矢印をすべて候補に渡すので、候補を閉じたら `ListBox` を外し、本文の行の移動を妨げない。
  上書きで足りることは spike（#1152）で確かめた（Playwright の Chromium・Firefox・WebKit、IME は合成イベントで模した）。部品は `@my/ui` の `TextAreaAutocomplete` にし、API を呼ぶ部分は `features/compose` が渡す。
- **キーボード**: 開いている間、上下の矢印で候補を移り（端で止まる）、Enter と Tab で確定し、Esc で閉じる。IME の変換中（下の IME の項）は、どのキーでも確定しない。開いたときは先頭の候補を選んだ状態にする。
  Esc は候補を閉じるだけで、投稿のシート（`Sheet`）を閉じない（イベントの伝播を止める）。開いていないときのキーは変えない。Ctrl / Cmd + Enter の送信は開いていても送信にする。
- **確定**: 入力中の語（`@` / `#` / `:` からカーソルまで）を候補の値（`@user` か `@user@domain`、`#Tag`、`:shortcode:`）で置き換え、後ろに空白がなければ半角空白を 1 つ足してカーソルをその後に置く。
  ローカルのアカウントは `@user`、ほかのサーバーのアカウントは `@user@domain`（Punycode でなく、`acct` のまま）にする。置き換えは、語の範囲を選んで `document.execCommand('insertText')` で入れ、取り消しの履歴に残す。`setRangeText` と `value` の代入は取り消しの履歴に残らない。`execCommand` は非推奨で標準の外だが、`textarea` の取り消しの履歴を保って文字を入れる方法がほかにないので使い、失敗（`false`）したら `setRangeText` に戻す（2026-10-23 に [MDN execCommand](https://developer.mozilla.org/en-US/docs/Web/API/Document/execCommand) で確認）。
- **IME**: `compositionstart` から `compositionend` までは候補を開かず、問い合わせず、キーを処理しない（`KeyboardEvent.isComposing` と `keyCode === 229` の両方を見る）。
  `compositionend` の後に入力中の語を読み直して開く。Safari は変換の確定の Enter の `keydown` を `compositionend` の後に `isComposing: false`・`keyCode: 229` で送る（UI Events に反する）ので、`keyCode === 229` の `keydown` は確定に使わない。
  WebKit の main では直っている（2026-10-23 に [WebKit bug 165004](https://bugs.webkit.org/show_bug.cgi?id=165004) で確認。bug 311717 の重複として解決）が、直る前の Safari が残るので、229 の判定を保つ。
  全角の `＠`・`＃`・`：` は起点にしない（サーバーも読まない）。
- **位置**: 候補はカーソルの行の直下に、入力欄と同じ幅で出す。カーソルの行の位置は、テキストエリアと同じスタイルの鏡の要素（強調表示の層と兼ねる、§4）で求める。
  画面の下に入らないときは行の上に出す（React Aria の `Popover` の反転）。モバイルのソフトウェアキーボードは `visualViewport` の高さを境にする。320 px のシート（ADR-0110）でも入力欄の幅に収まる。
- **問い合わせ**: 語が 1 文字（`:` は 2 文字）以上になってから、入力が 200 ms 止まったら問い合わせる。新しい問い合わせは前のものを取り消す（TanStack Query の `signal`）。
  読み込み中は 300 ms を超えたら「読み込み中」を、結果がなければ「候補はありません」を候補の場所に出し、どちらも告知する。失敗（429 を含む）は候補を閉じるだけで知らせない。
- **候補の見た目**: アカウントはアバター・表示名（`dir="auto"`、カスタム絵文字は画像）・`@user@domain`、タグは `#名前` と使われている投稿の数、絵文字は画像か文字と `:shortcode:`。文言は `client*` の名前空間に足す（ADR-0009）。

### 4. 入力中の強調表示

- **方法**: `textarea` の後ろに `aria-hidden` の層を置き、同じ文字を同じフォント・字間・行の高さ・内側の余白・`white-space: pre-wrap`・`overflow-wrap`・書字方向で並べる。
  層の文字は透明にして区間の背景だけを塗り、文字はネイティブの `textarea` が描く（二重に描かないので、ずれても文字は読める）。`textarea` の背景を透明にし、`scroll` で層の位置を合わせ、`ResizeObserver` で幅を合わせる。
- **強調するもの**: メンション・ハッシュタグ・URL・手元のデータにある `:shortcode:`（画像にはしない）と、文字数の上限を超えた部分。メンションの解決の結果では見た目を変えない（打つたびの解決になるため。外のメンションの注記は web-client §12 の既存の警告が受け持つ）。
- **色だけに頼らない**: 強調は飾りで、意味は文字（`@`・`#`・`:`）がすでに伝えているので、色を足しても WCAG 1.4.1 に反しない。上限を超えた部分は文字数カウンタ（ADR-0110 の読み上げ）が伝える。
  強制カラーモードでは背景が消えるので、`forced-colors:` で下線にする。色は新しいデザインシステムのトークン（#988）にライトとダークの組で足す。
- **書字方向**: 層と `textarea` に同じ `dir="auto"` を付け（ADR-0009 §5）、論理プロパティだけを使う。`ar-XB` のスクリーンショットで位置を確かめる（ADR-0110）。
- **規則の一致**: Web に `lib/textTokens.ts` を置き、`ParseText` と `ScanShortcodes` の規則を写す（`domainMentionPattern` もこれに寄せる）。
  共通のテストベクタ（入力の文字列と、期待する区間の種類・位置の一覧）を `pkg/testkit/fixtures/text/` の JSON に置き、Go の `ParseText`・`ScanShortcodes` のテストと Vitest の `textTokens` のテストの両方が読む。
  規則を変える PR は両方のテストを同時に直さなければ通らない。

### 5. 対象の入力欄

| 入力欄 | `@` | `#` | `:` | 強調表示 |
| --- | --- | --- | --- | --- |
| 投稿の本文（新規・返信・編集のダイアログ） | ○ | ○ | ○ | ○ |
| CW | × | × | ○ | `:shortcode:` だけ |
| プロフィールの自己紹介・補足情報の値 | ○ | ○ | ○ | ○ |
| プロフィールの表示名 | × | × | ○ | `:shortcode:` だけ |

サーバーが読む規則がある欄にだけ、その規則の分だけ付ける。どれも `@my/ui` の同じ部品（`TextAreaAutocomplete`、強調の層を含む）に、起点の種類を渡して使う。投票の選択肢は付けない（50 文字で効果が小さい）。

### 6. 機能要件と設計文書

- `docs/requirements/01-functional.md` の要件は次のとおり。
  - **FR-119**（S）: 投稿フォームの補完と強調: 投稿の本文・CW・プロフィールの自己紹介と表示名で、`@` のアカウント（フォロー中を先に、ブロック・ミュートを除く）・`#` のハッシュタグ（使われた数の順）・`:` の絵文字を候補から入れられ、入力中のメンション・ハッシュタグ・URL・絵文字・上限を超えた部分を強調表示する（ADR-0092）。
  - **FR-306**（S）: アカウントとハッシュタグの前方一致の検索（ユーザー名・`user@domain`・表示名の語、ローカルとこのサーバーが知る外のアカウント）。
- `docs/architecture/web-client/12-compose.md` の「補完と強調表示」の項に §3・§4 の振る舞いを書く。
- `docs/architecture/client-api.md`・`docs/architecture/data-model/` に 2 つの操作と 2 つの投影を書く。

#### 見直す条件

- ネイティブの `textarea` と層では主要なブラウザ（Chrome・Safari・Firefox、iOS・Android）で強調がずれる場面（折り返し・スクロール・ズーム）が避けられないと分かったら、`contenteditable` の編集器を改めて比べる。
- 補完の利用の計測で、前方一致の 100 行の上限のためにフォロー中の相手が出ない苦情が多いと分かったら、閲覧者のフォロー先に限った索引を足す。
- 汎用の全文検索を別の ADR で入れることになり、その索引が 1〜2 文字の前方一致も十分に速く引けるなら、補完の API をその上に寄せる。

## 経緯

投稿フォームの本文は `@my/ui` の `TextArea` のプレーンテキストで、補完も強調表示もなかった（[web-client §12](../architecture/web-client/12-compose.md)）。`@` のアカウント・`#` のハッシュタグ・`:` のカスタム絵文字を補完し、入力している間からメンションとハッシュタグを目で分かるようにする。関連する要件は FR-101、FR-103、FR-109、FR-111、FR-305、FR-412、FR-413。

サーバーには `GET /accounts/lookup`（`acct` の完全一致、`resolve=true` で外のアカウントを WebFinger で解決。FR-305）しかなかった。本文の読み取りはサーバーの `ParseText`（`internal/app/content/text.go`、FR-103）と `ScanShortcodes`（`internal/domain/content/custom_emoji.go`、ADR-0051）が持ち（プロフィールも `RenderProfileText` で同じ規則）、強調はこれと一致しなければ、強調したのにリンクにならないずれが出る。打つたびに問い合わせが飛ぶので、near-zero cost（NFR-COST）とレート制限（client-api §4.2、アカウント単位 1,500 / 5 分）の中に収める必要があった。

日本語の入力を壊さないこと（IME の変換中に候補を開かず確定の Enter を奪わない、取り消し・スペルチェック・モバイルの入力をブラウザの標準のまま保つ）、WAI-ARIA と APG に沿い ADR-0110 の検査を通ること、ADR-0025 の許可リストの中で独自の編集器を持たないことを基準に選んだ。

## 採らなかった案

- 汎用の検索の API（`GET /search`、SQLite FTS5 の trigram・PostgreSQL の `pg_trgm`）: 部分一致ができ検索画面と共有できるが、3 文字未満の語に一致しないか索引が効きにくい。投稿の全文検索まで含むと設計が大きく、`indexable` の扱いが検索と補完で違うのに 1 つの API に混ざる。
- サーバーに API を足さず、クライアントの手元のデータだけから出す: サーバーの変更がないが、フォローしていない相手・知らないタグが出ず、フォロー中の一覧を全部取るとフォローの多い人で重い。
- `@my/ui` に独自のフック（`useTextAutocomplete`）を書き、React Aria の `useListBox` と `aria-activedescendant` を結ぶ: IME と Enter を細かく制御できるが、仮想フォーカスと告知を自分で保守する。spike で上書きで足りたので要らなかった。
- `textarea` に `role="combobox"` を付ける: APG のとおりで `aria-expanded` を使えるが、ARIA in HTML に反し、axe などの検査と支援技術の扱いが不確かになる。
- `contenteditable` の編集器（自作、Lexical・ProseMirror・Tiptap など）: 区間を要素として描けてずれがなく、メンションをチップにもできるが、IME・取り消し・貼り付け・モバイルの入力をすべて編集器が担う。Lexical などは ADR-0025 の許可リストの外で、読み上げも自分で保証する必要がある。
- 強調表示をしない: 何もしなくて済むが、入力中に目で分かるようにする目的を満たさない。

## Consequences（影響）

- 良い影響
  - ネイティブの `textarea` を保つので、IME・取り消し・スペルチェック・自動修正・音声入力・支援技術の扱いがブラウザの標準のまま残る。
  - 補完の索引は B-tree だけで、SQLite と PostgreSQL の両方で拡張なしに同じ問い合わせが動く。読み取りは 1 回最大 100 行で、費用が小さい。
  - 共通のテストベクタで、強調した部分と投稿のリンクのずれを CI で防げる。既存の `domainMentionPattern` の写しもこれに寄る。
  - 同じ部品を投稿・編集・CW・プロフィールで使うので、振る舞いが揃う。
- 悪い影響・トレードオフ
  - 層を重ねる方式は、フォント・折り返し・スクロールの同期を CSS と JS で保つ必要があり、ブラウザの違いでわずかにずれることがある（文字はネイティブが描くので読めなくはならない）。
  - 前方一致だけなので、表示名の途中の語でない部分（「田中」で「山田田中」）は出ない。部分一致は汎用の検索に任せる。
  - 索引順に 100 行で打ち切るので、1 文字のような短い語では、フォロー中の相手や多くのフォロワーを持つ相手が候補に入らないことがある。語が伸びれば入る。
  - 「最近やりとりした相手」を先にする並びは入れない（安く引ける出どころがない）。
  - 閲覧者が使ったタグは端末ごとで、ほかの端末と共有しない。
  - `account_search_terms` と `tag_usage` の 2 つの投影が増え、プロフィールの更新と投稿の書き込みが少し重くなる。
  - Enter で候補を確定するので、候補が開いている間に改行するには Esc を先に押す必要がある。
  - `indexable = false` のアカウントも、ハンドルと表示名の前方一致で補完に出る。
  - 実機と支援技術での確認はしていない。バグ報告が届いたら #1738 を開き直して確かめる。

## Open Questions / 要検証

- 補完の問い合わせの回数が既存のアカウント単位のレート制限に収まるか。**要検証**（実装後の計測）。
