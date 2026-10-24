## 12. 投稿フォーム

`features/compose` が新しい投稿と返信のフォームを受け持つ（FR-101〜105、FR-110）。

- **置き場所**: 外枠が `ComposeProvider` と、`Sheet` の `ComposeEntry` を 1 つ置く。フォームは常にシートで、レールのボタン（`useOpenCompose`）、`md` 未満の浮かぶボタン（`ComposeEntry` の `trigger`）、返信と引用のボタンが開く。`Sheet` は `lg` から画面の中央のパネルになる。
  シートは、フォームに要る `GET /instance` と `GET /preferences` の応答が届いて（または失敗して）から開き、開いたときに本文へ直接フォーカスを移す。
  読み込み中に開くとダイアログ自身にフォーカスが移り、React Aria がその 500 ms 後に（iOS の VoiceOver への対策として）フォーカスが body にあればダイアログへ戻すので、
  閉じた直後でトリガーへフォーカスを戻す途中のポップオーバーからフォーカスを奪う。
  ほかの機能の返信ボタンは `useReply()` の関数に投稿を渡す。フォームが返信先を示し、返信先の作者とメンション（閲覧者を除く）を本文の先頭に入れ、シートを開くか本文にフォーカスを移す。サインインしていない間はどちらも出さない（`useReply()` は `null`）。
- **道具**（`components/ComposeTools.tsx`）: 本文の下の 1 行に、画像・投票・CW・公開範囲・引用できる人・言語のボタンを同じ高さで並べ、同じ行の終わりに送信のボタンを置く（320 px のシートでも送信が最初の画面に入る）。絵文字のボタンは道具の行に置かず、本文の右上に置く（下の「絵文字のピッカー」）。
  アイコンだけのボタン（画像・投票・CW）は `aria-label` と同じ文言のツールチップを持ち（G12）、投票と CW はトグル（`aria-pressed`）にする。どのボタンも今の値を伝える。
  名前と値を見せる 3 つの操作（公開範囲、引用できる人、言語）も、同じ遅延と位置のツールチップに「<名前>：<今の値>」（公開範囲は「公開範囲: <名前>」、引用は「引用: <選択肢>」、言語は「言語: <言語名>」）を出す。
  ツールチップは `aria-label` や見えている値の代わりではなく（アクセシブルネームは変えない）、ホバーとキーボードフォーカスで開いて Esc で閉じ、選択肢のリストやポップオーバーが開いている間は開かない。
  `Select` は `tooltip` の prop（`toolbar` の形用）でトリガーのボタンを `TooltipTrigger` に入れ、リストが開いている間は `isDisabled` にする。投稿の編集のダイアログ（§13）の言語も同じ。
  画像は添付した枚数と上限・形式を説明（`aria-describedby`）にし、公開範囲は短い名前を見せて「公開範囲: <名前>」を名前、その範囲を見られる人を説明にし、引用できる人（FR-114、ADR-0069 §3。「誰でも」・「フォロワー」・「自分だけ」）と言語は `Select` の `toolbar` の形で選んだ値を見せる（名前は値とラベル）。
  CW の入力欄は本文の上に、画像の上限と形式の文は添付した画像の下に出す。
- **絵文字のピッカー**（`features/emoji`、ADR-0091、ADR-0051 §4）:
  - **置き場所**: 本文の `TextArea` の右上（書字方向の終わり側、`inset-inline-end`）に、アイコンだけの絵文字のボタン（`EmojiPickerButton`。`aria-label` と同じ文言のツールチップ）を、カスタム絵文字の有無に関わらず置く。
    `TextArea` と `TextField` の `endAction` に渡し、入力欄は終わり側に余白を取って文字がボタンの下に回り込まないようにする。投稿の編集のダイアログの本文、プロフィールの編集の表示名（`TextField`）と自己紹介にも同じボタンを置き、CW と投票の選択肢には置かない。
    ボタンはポップオーバー（APG の dialog）に `@my/ui` の `EmojiPicker` を開き、検索欄にフォーカスを移す。
  - **データ**: Unicode 絵文字（`features/emoji/unicode` の基本のデータと、表示中のロケールと `en` の名前とキーワード）は `hooks/unicodeEmoji.ts` が `import()` で読み、ページの間ロケールごとに 1 度だけ組み立てて持つ。
    読み始めるのはボタンにポインタが乗ったとき・フォーカスしたとき・ポップオーバーを開いたときで、初期のチャンクには入らない。届くまではポップオーバーにスピナーを出し、読めなかったときは文と、カスタム絵文字だけのピッカーを出す（開き直すと読み直す）。
    カスタム絵文字は `GET /custom_emojis` の query。絵文字のフォントの分割（ADR-0095）は、グリッドに描いた絵文字の分を開いたときに読む。
  - **構成**: 節は、よく使う → カスタム絵文字（ADR-0051 のカテゴリごと、カテゴリの無いものが先。無いサーバーでは節とボタンを出さない）→ Unicode の 9 グループ（`emoji-test.txt` の順）。
    `EmojiPicker`（ADR-0091 §3・§5・§6）はデータを持たず、アプリが渡す節（ID・見出し・アイコン・絵文字の列）を、検索欄と肌の色のメニューボタン、節ごとのアイコンのボタンの列（React Aria の `Toolbar`。押すとその節の見出しまでスクロールして最初の絵文字にフォーカスを移し、グリッドの上端にある節のボタンに `aria-current="true"`）、すべての節を続けた 1 つのグリッド（APG の grid。Tab で検索欄 → 肌の色 → ボタンの列 → グリッドと移り、グリッドの中は roving tabindex で、矢印キーで書字方向に沿って動き、Home / End で行の端、Ctrl + Home / Ctrl + End でグリッドの端、Page Up / Page Down で見えている行の数だけ動き、Enter・Space・押下で選ぶ）で出す。
    検索は NFKC・小文字・カタカナをひらがなにそろえ、名前（カスタム絵文字は shortcode）の完全一致 → 名前の前方一致 → キーワード（別名、Unicode 絵文字は表示中のロケールと `en` の CLDR のキーワードと `en` の名前）の前方一致 → 部分一致 の順に 1 つの節に並べ（`searchEmojis`。ADR-0092 の補完も使う）、件数は入力が止まってから `role="status"` で告げる。
    各絵文字の名前は、カスタム絵文字は `:shortcode:`、Unicode 絵文字は表示中のロケールの CLDR の短い名前（肌の色の付いたものは annotationsDerived の名前）にする。動く絵文字は利用者の設定と `prefers-reduced-motion` に従って止める（`CustomEmoji` と同じ）。
    グリッドは TanStack Virtual で仮想化する（ADR-0097）。節の見出しの行と、グリッドの幅と見本のセルの大きさから決めた N 列の絵文字の行を 1 本の行の並びにし、見える行と前後の 4 行、それにフォーカスのあるセルの行（`rangeExtractor`）だけを描く。
    総数と位置は `aria-rowcount` / `aria-colcount` / `aria-rowindex` / `aria-colindex` で伝え、描いた行は節ごとの `rowgroup`（`aria-label` は見出し）にまとめる。`aria-current` はスクロールの位置から導く。グリッドのキー操作とフォーカスは `EmojiPicker` が持ち（React Aria の `GridList` は使わない）、押下・フォーカスのリング・ホバーは React Aria の `usePress`・`useFocusRing`・`useHover` で受ける。
  - **入れ方**: 対象の入力欄のカーソルの位置（選択していればその範囲）に、Unicode 絵文字は選んだ肌の色の文字そのもの（空白を足さない）を、カスタム絵文字は `:shortcode:` を入れる（サーバーが前後に英数字とコロンのない shortcode だけを読むので、必要なら前後に空白を足す）。
    選ぶとポップオーバーを閉じて入力欄にフォーカスを戻し、カーソルを入れた絵文字の後ろに置く。Shift を押しながら選ぶ（`keepOpen`。タッチでは渡らない）と閉じず、フォーカスはグリッドに残り、次の絵文字はその後ろに入る。
  - **保存**: よく使う絵文字は端末ごとに localStorage（`my:emoji:recent`。版 2、`hooks/frequentEmojis.ts`）に 32 個まで持つ。Unicode 絵文字は肌の色を外した基の列、カスタム絵文字は `:shortcode:` を 1 つの列に混ぜ、使った回数を 1 週間で半分になるように減衰させた値の順に並べる（減衰はどの絵文字にも同じ率で効くので、使ったときに並べた順がそのまま次に使うまで正しい）。
    まだ無いときは、リアクションの既定の群（👍 ❤️ 😆 …）を出す。版 1（最近使ったカスタム絵文字の shortcode の配列）は読み替えず、既定として読む。ADR-0092 の補完（`hooks/emojiSuggestions.ts`）も、この順でよく使う絵文字を先に並べる。
    肌の色は localStorage（`my:emoji:skin-tone`）に保存し、異体のある絵文字はその色で表示して入れる（複数の人の絵文字は全員に同じ色）。
  - **リアクション**（ADR-0091 §7、ADR-0052、§13）も同じピッカー（節の組み立て・データの読み込み・よく使う・肌の色を `hooks/emojiPickerContent.ts` で共有し、ポップオーバーの中身は `components/EmojiPickerPanel.tsx`）を使い、選んだ絵文字を入れる代わりにリアクションする。
- **Unicode 絵文字の表示**（ADR-0095）: ピッカー・補完・リアクション・投稿の本文・名前の Unicode 絵文字は、どれも `--font-sans` の先頭の `my Noto Color Emoji`（`@my/ui/emoji.css`）で描き、OS のフォントで描けるかを canvas で判定して隠すことはしない（フォントは `EMOJI_VERSION` のすべてを描くことを生成のときに確かめる）。
  フォントは `emoji-test.txt` のグループごとに分けてあり、画面に出た絵文字のグループの分だけを読む。読み込み中は `font-display: swap` で OS の絵文字で描き、届いたら入れ替わる。このため、字の幅に合わせて位置を測る層（ADR-0092 の強調の層）は、`document.fonts` の `loadingdone` の後にも測り直す。
- **下書き**（`hooks/draftStore.ts`）: フォームの外のストア（`useSyncExternalStore`）に置き、シート・返信ボタンが同じ下書きを共有する。
  リロードやタブの異常終了で失わないように、アカウントごとのキー（`my:user:compose:<アカウント ID>`）で sessionStorage に保存する。このタブだけのもので、タブを閉じるかセッションが終わると消える（§10）。
  アップロードの途中のファイルは保存しない（再開できない）。アップロード済みの画像は ID と説明を保存し、リロード後はサーバーのサムネイルを表示する。
- **既定値**: 公開範囲・引用できる人・言語は、利用者が選ぶまで `GET /preferences` の既定値に従う（state にコピーせず、表示のたびに求める）。返信の公開範囲の既定は、既定値と返信先のうち狭いほう（ADR-0017。`local` への返信は `local`）。
  公開範囲は 5 段階を説明付きのラジオグループ（ポップオーバーの中）で選ぶ。
  制限で公開範囲に上限があるとき（data-model/05-states-and-policies.md §5.4）は、閲覧者の `CredentialAccount` の `posting`（サーバーがドメインの規則から求めたもの）の `visibilities` にない選択肢を無効にし、ラジオグループの説明で上限を示す。
  上限より広い選択（制限の前に選んだもの、既定値）は、上限（`visibility_ceiling`）として表示して送る。上限は投稿の間にも変わり得るので、結果の公開範囲が送ったものより狭いときは、投稿の後にトーストで知らせる。
  `local` の投稿がほかのサーバーのアカウントにメンションするときは警告する（ADR-0017）。本文が 400 ms 変わらなくなってから、ドメイン付きのメンションを `GET /accounts/lookup`（`resolve=true`。サーバーが投稿のメンションを解決するのと同じく、未知のハンドルは WebFinger で解決する。data-model/04-tables/04-content.md §4.4）で引き、
  見つかったほかのサーバーのアカウント（`acct` にドメインがある）を `GET /accounts/relationships` で調べ、閲覧者をフォローしていない相手には投稿が届かないこと、ほかのサーバーのフォロワーにはそのメンションがメンションではなくただのリンクとして見えること（ADR-0035）を本文の下に示す。
  解決できないハンドル（404）はサーバーもテキストのままにするので示さない。解決の上限に達した（429）ときも示さない。
  言語の選択肢の名前は `Intl.DisplayNames` で表示中のロケールにする（カタログに言語名を持たない）。
  引用できる人は利用者が選んだときだけ `quote_policy` を送り、選ばなければサーバーが既定値を使う。既定値は設定の「新しい投稿を引用できる人」で変える。
- **引用**（FR-114、ADR-0069 §5）: ほかの機能の引用ボタンは `useQuote()` の関数に投稿を渡す。下書きは引用する投稿の ID（`quoteId`）を持ち、フォームは本文の上に「<名前> さんの投稿を引用」と引用する投稿（`QuoteCard`）と「引用をやめる」を出し、送信の本文に `quoted_status_id` を入れる。
- **連合の向きの注記**（FR-413、[ADR-0076](../../adr/0076-federation-direction.md)）: インスタンスが外へ送らないモード（`off` / `inbound`）のとき、ほかのサーバーの投稿への返信か、ほかのドメインの `@user@domain` へのメンションを含む下書きに、本文の下の警告（`Text`、`tone="warning"`）で「連合先には届きません」を出す（`useComposeForm` の `staysLocal`）。
  外のアカウントを取り込まないモード（`off` / `outbound`）では、メンションの検索とメンション先の確認が `resolve` を送らない（知っているアカウントだけを引く）。
  モードを狭める前から外の投稿を引用している下書きはフォームでは止めず、送るとサーバーが 403 `federation-restricted` を返し、送信中の一覧にその理由が出る。
- **投票**（FR-112、`components/PollEditor.tsx`、`hooks/poll.ts`）: 道具の行の投票のトグルが、本文と添付の下に投票の `fieldset`「投票」を出す。添付のある投稿には付けられず（トグルが無効）、投票のある投稿には添付できない（画像のボタンが無効）。
  選択肢は 2 つの空の欄から始まり、「選択肢を追加」で 4 つまで足し、3 つ目からは欄ごとの削除のボタン（アイコンとツールチップ）で外す。各欄は 50 文字まで。期間（5 分〜7 日の `Select`。既定は 1 日）、複数選択と締め切りまで票数を隠すのチェックボックスを持つ。
  空の選択肢があるか、前後の空白を除いて同じ選択肢が 2 つあるときは送信できず、同じ選択肢は欄の下に理由を出す。投票は下書きに入り、本文と一緒に `POST /statuses` の `poll` で送る。投票のある投稿は編集できない（API）。
- **文字数**: `GET /instance` の `limits.max_post_characters` から CW の文字数を引いたものを本文の上限にし、`TextArea` の文字数カウンタで示す。数え方はサーバーと同じコードポイント。上限を超えると送信できない。
- **補完**（ADR-0092 §1〜§3・§5、FR-119）: 入力中の `@` のアカウント・`#` のハッシュタグ・`:` の絵文字を、入力欄の下の候補から入れる。
  - **部品**: `features/compose` の `CompletionTextArea`・`CompletionTextField`（`components/CompletionFields.tsx`。`index.ts` から出し、投稿の編集とプロフィールも使う）が、`@my/ui` の `TextAreaAutocomplete`・`TextFieldAutocomplete` に `hooks/useTextCompletion.ts` の起点・語の見つけ方（`lib/textTokens.ts` の `activeWord`）・候補の関数・文言と、候補の中身を渡す。
    入力欄は `textbox` のまま `aria-autocomplete="list"`・`aria-controls`・`aria-activedescendant` を持ち、候補は `listbox` の `option`、件数・読み込み中（300 ms を超えたとき）・「候補はありません」は `role="status"` で告げる。キー（上下・Enter・Tab・Esc）と IME の扱いは `@my/ui` の README の `TextAreaAutocomplete` の項のとおり。
  - **対象**: 投稿フォームと投稿の編集のダイアログの本文、プロフィールの自己紹介と補足情報の値は `@`・`#`・`:`、CW とプロフィールの表示名は `:` だけ（ADR-0092 §5 の表）。投票の選択肢と補足情報のラベルには付けない。
  - **問い合わせ**: `@` と `#` は 1 文字から、入力が 200 ms 止まってから `GET /accounts/search`・`GET /tags/search`（`api/completion.ts`）を引く。`q` ごとに `staleTime` 60 秒で持ち、文字を消して戻っても問い合わせない。新しい語が前の語の要求を取り消す（`cancelQueries`）。
    `@` の `q` が完全な `user@domain` で、結果に同じ `acct` がないときは、`GET /accounts/lookup` を引き、見つかれば先頭に足す。外のアカウントを取り込まないモード（`off` / `outbound`、ADR-0076）では `resolve` を送らない。
    失敗（429 を含む）は候補を閉じるだけで知らせない。
  - **最近使ったタグ**: 投稿の送信（と編集の保存）で本文のハッシュタグを localStorage（`my:compose:recent-tags`、`hooks/recentTags.ts`）の先頭に入れ、16 個まで持つ（NFKC・小文字で同じものは 1 つ）。`#` の候補は、そのうち打った語で始まるものを新しい順に先に、続けてサーバーの候補を並べ、8 件まで出す。
    セッションが終わると消す（利用者が書いたものなので）。
  - **絵文字**: `:` の後に 2 文字から、サーバーに問い合わせず、すぐに出す。`features/emoji` の `useEmojiSuggestions` が、ピッカーと同じ `GET /custom_emojis` の query と Unicode 絵文字のデータ（初めてのときは読み込みを待つ。読めなければカスタム絵文字だけ）を、ピッカーの検索と同じ `searchEmojis`（shortcode・名前の前方一致 → 別名・キーワードの前方一致 → 部分一致）で照らし、よく使う絵文字（`my:emoji:recent`）を使った順に先に並べ、8 件まで出す。
    Unicode 絵文字は選んだ肌の色（`my:emoji:skin-tone`）の文字、カスタム絵文字は `:shortcode:` を入れる。
  - **入れ方**: 入力中の語を候補の値（ローカルのアカウントは `@user`、外は `@user@domain`、`#Tag`、`:shortcode:` か絵文字の文字）で置き換え、後ろに空白がなければ半角空白を 1 つ足す（取り消しの履歴に残す `execCommand("insertText")`）。
  - **候補の見た目**: アカウントはアバター・表示名（`dir="auto"`、カスタム絵文字は画像）・`@acct`、タグは `#名前` と使われている投稿の数（端末の最近使ったタグでサーバーが知らないものは数なし）、絵文字は画像（代替テキストは隣の文字と重なるので読ませない）か文字と、`:shortcode:` か CLDR の名前。
  - **テスト**: `features/compose/completion.test.tsx` が、各起点の候補と確定、lookup の条件と `resolve`、CW と表示名で `:` だけが開くこと、最近使ったタグの保存と並び、よく使う絵文字と肌の色、失敗で閉じることを MSW で検査する。E2E（`e2e/compose.spec.ts`）が候補の流れを axe と ARIA スナップショットで検査する。
- **強調表示**（ADR-0092 §4・§5、FR-119）: 入力中のメンション・ハッシュタグ・URL・`:shortcode:`・文字数の上限を超えた部分の背景を、入力欄の後ろの層に塗る。
  - **区間**: `lib/textHighlights.ts` の `textHighlights` が、`lib/textTokens.ts` の `textTokens`（サーバーの `ParseText`・`ScanShortcodes` と同じ規則）から作り、`@my/ui` の `TextArea`・`TextField`（と補完の `TextAreaAutocomplete`・`TextFieldAutocomplete`）の `highlights` に渡す。
    `:shortcode:` は、手元のカスタム絵文字（ピッカーと同じ `GET /custom_emojis` の query。`features/emoji` の `useCustomEmojiShortcodes`）の shortcode にあるものだけを塗る。
    上限を超えた部分は、上限のコードポイントの後ろから終わりまでで、トークンと重なればそちらを上限の色で塗る。本文の上限は上の「文字数」の値（`max_post_characters` から CW を引いたもの）、プロフィールの自己紹介と補足情報の値はそれぞれの上限。
    メンションの解決の結果では見た目を変えず、絵文字を画像にもしない。
  - **対象**: 投稿フォームと投稿の編集のダイアログの本文、プロフィールの自己紹介と補足情報の値はすべての種類、CW とプロフィールの表示名は `:shortcode:` だけ（ADR-0092 §5 の表）。投票の選択肢には付けない。
  - **層**（`@my/ui` の `TextAreaMirror`）: `aria-hidden` の層を入力欄と同じ箱の中の前に置き、入力欄の計算済みのフォント・字間・行の高さ・内側の余白・枠の幅を写し、`white-space: pre-wrap`（`input` は `pre` で縦の中央）・`overflow-wrap: break-word`・`dir="auto"` で同じ文字を並べる。
    層の文字は透明で、区間の `<span>` の背景だけを塗る。文字は入力欄が描き、層は描かない（二重に描かない）。入力欄の背景は透明にし、箱が入力欄の背景を塗る。入力欄も `dir="auto"` にする。
    入力欄の `scroll`（`input` はカーソルの移動の `selectionchange` の後も）でスクロールを、`ResizeObserver` で大きさを、`document.fonts` の `loadingdone`（Noto Color Emoji の `font-display: swap` の入れ替え）で位置を合わせ直す。
    層を持つ入力欄の行の高さは整数の px（`round(calc(var(--text-base--line-height) * 1em), 1px)`）にする。小数の px だと、スクロールした層が Chromium と WebKit で 1 px ずれる（#1152 の spike、決定者の決定）。
  - **見た目**: 色は種類ごとのトークン `highlight-mention`・`highlight-hashtag`・`highlight-url`・`highlight-shortcode`・`highlight-over`（ライトとダークの組。上の文字の `fg` と WCAG AA のコントラスト）。
    強制カラーモードでは背景が消えるので、区間の下に `CanvasText` の線（上限を超えた部分は破線）を引く。層は強制カラーから外れ（`forced-color-adjust: none`。透明の文字を透明のまま保つ）、強制カラーのときだけ入力欄の前に出る（Chromium が入力欄の文字の行の後ろに `Canvas` の色の板を塗り、後ろの線を隠すため）。
  - **テスト**: `lib/textHighlights.test.ts` が共通のテストベクタで区間が `textTokens` と一致することを、`compose.test.tsx` が本文の層の区間と `aria-hidden` を検査する。`@my/ui` の `TextHighlight` のストーリーが、折り返し・スクロール・RTL・1 行の欄の位置をスクリーンショット（3 幅 × ライト / ダーク × LTR / RTL）で、強制カラーモードの線を `forced-colors` のタグのスクリーンショットで確かめる。
- **画像・動画・音声**: 1 投稿 4 件まで。画像は JPEG・PNG・GIF・WebP、動画は MP4・QuickTime・WebM、音声は MP3・Ogg・M4A・WebM（申告された型で選び、ほかは知らせて外す）。選んだファイルは `prepareMedia` が用意する。
  画像は `prepareImage` が `createImageBitmap`（EXIF の向きを適用）と `OffscreenCanvas` で 3840 × 2160 ピクセルの面積以内に縮め、描き直して EXIF などのメタデータを捨ててから送る。
  GIF はアニメーションを保つためそのまま送る（GIF に EXIF はなく、残りはサーバーが除く）。
  動画と音声はそのまま送り、メタデータはサーバーが除く（ADR-0054 §6）。動画は文書に入れない `<video>` で 0.5 秒（短ければ半分）の位置のフレームを `OffscreenCanvas` に描き、1920 × 1080 ピクセルの面積以内の JPEG をポスター（`thumbnail`）として一緒に送る。ブラウザーが復号できない動画はポスターなしで送る。
  `POST /media`（202）の後は、`GET /media/{id}` の query が `processing` の間だけポーリングする。
  どの添付にも代替テキストの入力欄を必ず出し（Y9）、本文と一緒に `POST /statuses` の `media[].description` で送る。サーバーが代替テキストを必須にしている（`GET /instance` の `limits.require_media_description`）ときは、説明の入力欄を必須にし、空か空白だけの欄があればブラウザの制約検証で送らずにその欄へフォーカスを移す（サーバーも 422 で拒む）。
  そうでなく、利用者の設定 `require_media_description` が真で説明のない添付があるときは、送る前に `alertdialog` で確かめる。
- **送信**: 送信は下書きを「送信中」の一覧（outbox）に移してフォームを空にし、一覧が送った内容を表示する（S8）。成功したら一覧から消し、トーストで知らせ、タイムラインの query を無効にする。
  失敗したら、フォームが空ならそこへ下書きを戻し、空でなければ一覧に残して「もう一度送る」「編集する」「破棄する」を出す。一覧も保存するので、送信中にリロードしても失わない。
  1 つの下書きの試行はすべて同じ `Idempotency-Key` を送り、応答が届かなかった再送で二重に投稿しない。下書きを編集するとキーを作り直す。
- **テスト**: `features/compose/compose.test.tsx` がフォームの流れを MSW で検査し、引用の流れは `features/status/quote-actions.test.tsx`（§13）が検査する。jsdom には `matchMedia`・`createImageBitmap`・`OffscreenCanvas` がないので、必要なテストが差し替える。
  jsdom の `FormData` と `Blob` は Node.js の fetch が本文にできないので、アップロードのテストは Node.js のものに差し替える。
  E2E（`e2e/compose.spec.ts`）は、Playwright の要求の差し替えでクライアント API とトークンの更新に答え、サインイン済みの状態で axe・横スクロール・ARIA スナップショット・送信の失敗と再送を検査する。
