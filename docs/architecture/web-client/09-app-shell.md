## 9. アプリの外枠

`app/web/src` 直下（`shell`）と `routes/__root.tsx` が、すべての画面に共通する部分を 1 か所で受け持つ。機能とルートはこれを作り直さない。

- **CSP に収める**: 外枠の CSP（`require-trusted-types-for 'script'` と nonce のない `style-src 'self'`）を破るライブラリの動作は止めておく。
  Zod はスキーマを作るときに `eval` でのコンパイルを試すので、`"zod"` の import は Vite の alias で `lib/zod.ts`（`jitless` にした Zod）に向ける。
  React Aria の `usePress` が `<head>` に差し込む `<style>` は、`main.tsx` が最初に同じ id の空の `<template>` を置いて止め（`lib/csp.ts`）、その規則は `index.css` に置く。
  Trusted Types の下では `innerHTML` に文字列を入れられないので、`PostContent`（A8）はサニタイズ済みの本文を、そのまま通すだけの Trusted Types のポリシー `my-server-sanitized-html` で TrustedHTML にしてから入れる。
  文字列を TrustedHTML にするのはこのポリシーだけで、受け取るのは `SanitizedHtml` だけにする。
  `DOMParser.parseFromString` も文字列を拒むので、投稿を文字だけで引用する場所（通知の行など）は、同じポリシーを通して解析する `@my/ui` の `sanitizedHtmlText` で本文の文字を取り出す。
  このポリシーがそのまま通してよいのは、渡る HTML がすべてサーバーのサニタイザ（Go の許可リスト）を通ったものだからである。連合で届くリモートの HTML（M3）も、クライアントに渡す前に同じサニタイザを通す。
  - 外枠の CSP は `trusted-types my-server-sanitized-html my-service-worker` で、作れるポリシーをクライアントの 2 つ（`PostContent` の HTML と、`lib/pwa.ts` の Service Worker の URL）に限る。
    どちらもそれぞれ 1 つのモジュールで、最初に使うときに 1 回だけ作るので、`'allow-duplicates'` は付けない。ほかのスクリプト（依存ライブラリを含む）は自分のポリシーを作れない。
    Vite・React・React Aria・Zod（`jitless`）はポリシーを作らない。ポリシーを作るコードを足すときは、この一覧と `internal/interface/web/shell.go` の CSP を一緒に変える。
  - 止められない動作が 1 つ残る: iOS / iPadOS の WebKit では、React Aria の `usePreventScroll`（モーダルの overlay と、Select / Menu のようなモーダルの popover が使う）が、開いている間 `<head>` の先頭に `* { overscroll-behavior: contain }` の `<style>` を差し込む。
    これを止める手段はなく、外枠の CSP がこの `<style>` を拒むので、iOS では overlay を開くたびに CSP の違反が 1 件報告される。この報告は受け入れ、CSP（ADR-0019 §9）は変えない。
    代わりに `@my/ui` の Dialog / Sheet / Popover / Select / Menu は、スクロールする面に `overscroll-contain` を自分で付け、overlay の中のスクロールがページに伝わらないようにする。overlay の中に別のスクロールする領域を作るときも、その領域に `overscroll-contain` を付ける。
    Chromium はこの経路を通らないので、`e2e/csp.spec.ts` はこの違反を検出しない。
- **起動**（`main.tsx`）: このブラウザで最後に表示したロケール（localStorage の `my:display:locale`）か、なければサーバーが外枠に書いた `<html lang>`（インスタンスのロケール）から、
  カタログのある最も近いロケールを決め（`ja-JP` → `ja`、ない言語は `en`）、`<html>` の `lang` / `dir` をそれに合わせる（`lib/locale.tsx` の `loadStartI18n`）。
  次に `<script type="application/json" id="config">` のランタイム設定を Zod で検証して読む（`lib/config`、`useConfig()`）。読めなければアプリの代わりに起動エラーの画面を出す。
  読めたら、表示のロケール・QueryClient・セッション・API クライアント（`lib/services.ts`）を作り、リフレッシュトークンの Cookie からセッションを再開し始める（§10）。
- **表示のロケールとタイムゾーン**（`lib/locale.tsx`、FR-007）: ページを開いている間に変わるので、React の外のストア（`AppLocale`、`useSyncExternalStore` で読む）に置く。
  サインインした利用者の設定（`GET /preferences`）を読むと、外枠（`AppLayout` が呼ぶ `features/settings` の `useUserDisplaySettings`）がその `locale` と `time_zone` をストアに渡す。
  ストアはそのロケールのカタログを読み込んでから `<html>` の `lang` / `dir` を変え、i18n のプロバイダを描画し直す。読み込みが前後したときは最後に頼んだロケールが勝つ。
  タイムゾーンは `react-intl` の `timeZone` になる。ロケールが変わると、サーバーが `Accept-Language` で訳した内容を取り直すために、Query をすべて無効にする。
  ストアは決めたロケールを `my:display:locale` に残す。設定を読むまでと、設定が読めないとき（429 を含む）とサインインしていないときは、最後に表示したロケール（初めてのブラウザではサーバーのロケール）のままになり、`<html lang>` も戻さない。
  このキーはこのブラウザでの表示のしかたで、利用者のデータではないので、セッションが終わっても消さない。次の利用者の設定が読めればそれに替わる。タイムゾーンは残さず、設定を読むまではブラウザのものになる。表示言語の選択肢は、カタログのあるロケール（疑似ロケールは、今それで表示しているときだけ）を、それぞれの言語の名前で並べる。
  タイムゾーンの選択肢は、表示中のロケールの CLDR の名前（`Intl.DisplayNames` にタイムゾーンの種類はないので、`Intl.DateTimeFormat` の `timeZoneName: "longGeneric"`。名前のないゾーンは UTC からの差だけ）に、
  画面を開いた時点の UTC からの差を付けて「（UTC+09:00）日本標準時」と出し、その差と名前の順に並べる。IANA の名前（`Asia/Tokyo`）は、同じ名前のゾーンを見分けるための選択肢の説明（`SelectItem` の `description`）にする。
- **端末のタイムゾーン**（FR-007）: タイムゾーンをまだ選んでいない利用者（`time_zone_chosen` が偽。新しいアカウント）の日時は端末（ブラウザ）のタイムゾーンで出し、
  `useUserDisplaySettings` が最初に開いた画面で、そのゾーン（`Intl.DateTimeFormat().resolvedOptions().timeZone`）を `PATCH /preferences` で保存する。
  外のシステム（ブラウザのゾーンとサーバーの設定）に合わせる処理なので Effect に置き、後始末で要求を中止する。失敗したら選ばないままにして、次に開いたときにまた試す。
  設定の画面は、選んでいるゾーンが端末のゾーンと違うとき（`Asia/Calcutta` と `Asia/Kolkata` のような別名は同じとみなす）、選択肢の下に端末のゾーンの名前を書いた注記と「この端末のタイムゾーンにする」ボタンを出す。
  ボタンを押すとフォーカスをタイムゾーンの選択肢に移してから保存する（注記はボタンごと消える）。
- **表示のテーマ**（`lib/theme.ts`）: 「システムに合わせる / ライト / ダーク」から選び、`<html data-theme>`（`light` / `dark`、システムに合わせるときは属性なし）にする。トークンの切り替えは [app/ui/README.md](../../../app/ui/README.md) の「Light and dark」にある。
  表示言語の `my:display:locale` と同じく、このブラウザでの表示のしかたなので、利用者の設定（`GET /preferences`）ではなく localStorage の `my:display:theme` に残し、サインインしていなくても効き、セッションが終わっても消さない。
  外枠は `<head>` のスタイルシートより前で、同じオリジンのクラシックスクリプト `public/theme.js`（`/theme.js`、`defer` / `async` なし）を読み込み、最初の描画の前に属性を付ける。CSP がインラインのスクリプトを許さず、外枠はどの閲覧者にも同じもの（ADR-0028）なので、サーバーは選択を外枠に書けない。
  `my serve` はビルドにこのファイルがあるときだけタグを書き（`internal/interface/web`）、Service Worker はほかのビルドのファイルと一緒にプリキャッシュする。
  属性を状態とし、`useTheme()` が `useSyncExternalStore` で読む。選ぶとすぐに属性と保存先を変え、ほかのタブでの選択も `storage` イベントで追う。保存できないとき（ストレージが使えない）もそのページには効く。
  選ぶ場所は「表示と投稿」の設定とメニューのシート（ADR-0039）で、どちらも同じ `ThemeRadioGroup`（APG の radio group。矢印キーで移ると選ぶ）にする。サーバーが描画する画面（§8）は JavaScript を使わないので、OS の設定に従う。
- **サーバーのテーマ**（[ADR-0059](../../adr/0059-design-tokens-and-instance-theme.md)、FR-605）: 管理者が保存したテーマ（トークンの上書きとカスタム CSS）は、`my serve` が版ごとの同じオリジンのスタイルシート `/theme/{版}.css`（`text/css`、`nosniff`、`immutable`）にして返す。
  外枠はビルドのスタイルシートの後にこれを `<link rel="stylesheet">` で読むので、最初の描画からテーマが当たる。規則は `@layer` の外にあり、Tailwind の `@layer theme` の既定値とユーティリティに勝つ。CSP（`style-src 'self'`）は変えない。
  外枠は閲覧者ごとに変わらず、ETag はテーマの版を含む。テーマがなければ `<link>` を出さない。
  `<meta name="theme-color">` は `media="(prefers-color-scheme: light)"` と `dark` の 2 つで、値はテーマ（なければ既定）の `color-surface`。Web App Manifest の `theme_color` / `background_color` はそのライトの値にする。
  `data-theme` で OS と違うテーマを選んだときのブラウザの枠の色は OS に従う。
- **未保存の変更**: プロフィールの編集のように、まとめて保存するフォームは、保存していない変更があるまま別の画面へ移るとき（検索条件とハッシュだけの変化は除く）に、TanStack Router の `useBlocker`（`withResolver`）で遷移を止め、
  `ConfirmDialog`（`alertdialog`）で確かめる。フォーカスは「編集を続ける」から始まり、Esc もそれと同じく画面に残る。再読み込みとタブを閉じるときは、ブラウザ自身の確認（`beforeunload`）になる。
  変更は保存済みの値と比べて求めるので、手で元に戻した変更や保存した後は確かめない。設定を変えたとたんに保存する画面（表示と投稿）は対象にしない。
- **古い投稿の自動削除**（`/settings/auto-delete`、設定のメニューの「古い投稿の自動削除」、FR-118、`features/settings` の `AutoDeleteSettings`）: `GET /preferences/auto_delete` の設定を、有効にするスイッチ、削除する投稿の古さ（`Select`。7 日〜2 年と、保存されている値が選択肢になければそれも）、
  残す投稿の例外（ピン留め・自分のブックマーク・自分のお気に入り・メディア付き・投票付きの `Checkbox` と、残す公開範囲の `Checkbox` をそれぞれ `fieldset` にまとめる）、ブーストとお気に入りの数の閾値（`Select`。「この条件では残さない」と 1〜100 回）で編集する。
  まとめて保存するフォームで、「保存する」が設定全体を `PUT /preferences/auto_delete` で送り、応答でキャッシュを置き換えて、保存した内容を `StatusMessage` に出す。保存していない変更は上の「未保存の変更」の確かめの対象にする。削除はサーバーの 1 時間ごとのジョブが行うので、保存しても投稿の一覧は取り直さない。
- **ミュートしたドメイン**（`/settings/domain-mutes`、設定のメニューの「ミュートしたドメイン」、FR-304、`features/settings` の `DomainMutes`）: ミュートが何を隠すか（ホーム以外のタイムラインの投稿・ホームのブースト・通知。フォローしているアカウントはホームに残る）の説明、
  ドメインをミュートするフォーム（`TextField` とブラウザの制約検証）、ミュートしているドメインの一覧（ドメインの順、「さらに表示」）の 3 つ。`POST /domain_mutes` が正規化したドメインを `StatusMessage` に出し、断られたらサーバーの `detail` を `role="alert"` で出して入力を残す。
  各行はドメインを U ラベルで（A ラベルと違えば A ラベルも、どちらも `dir="ltr"`）出し、「ミュートを解除」（アクセシブルネームにドメインを含める）で `DELETE /domain_mutes/{domain}` を送る。解除した行は一覧から消し、フォーカスを次の行のボタンへ移す（`useFocusAfterRemoval`）。
  ミュートと解除は投稿の一覧と通知（未読数を含む）を古くする（S1）。ミュートは一覧も取り直す（ドメインの順に入る）。
- **パスワード**（`/settings/password`、設定のメニューの「パスワード」、FR-002・FR-004、`features/settings` の `PasswordSettings`）: loader が、パスワードがあるか（`GET /accounts/password`）とパスキーの一覧を読む。
  パスワードがある利用者には 2 つの区画（`h2` の `section`）を出す。「パスワードを変更」は、変更が何をするか（ほかのブラウザーとアプリからログアウトし、パスキーと二要素認証は残る）の説明と、
  現在のパスワードと新しいパスワード（`autocomplete` は `current-password` / `new-password`、長さはブラウザーの制約検証）の `TextField` のフォーム。`POST /accounts/password` を送り、
  間違った現在のパスワードはその欄のエラー、ほかの失敗は `role="alert"` で出す。「パスワードを削除」は、パスキーだけでログインするようになることの説明と、`ConfirmDialog`（`danger`）で確かめるボタン。
  確かめるとパスキーでの確認（下記）のあとに `POST /accounts/password/remove` を送り、失敗はダイアログに出す。パスキーのない利用者にはボタンの代わりに先にパスキーを追加するよう出し、パスキーに対応していないブラウザではその旨を出す。
  パスワードのない利用者には「パスワードを設定」の区画だけを出し、新しいパスワードの欄をパスキーでの確認のあとに `POST /accounts/password` で送る。
  変更・削除・設定の結果は、画面の先頭の 1 つの `StatusMessage` に出す（削除や設定で区画が入れ替わっても、同じ場所の `StatusMessage` が読み上げられる）。成功したらフォームを空にし、パスワードがあるかを取り直してから結果を出す。
  サーバーは要求したトークンの系列を残すので、この画面のセッションは続く。パスワードを忘れたときのリセットはサーバーのログインの画面が行う（§8）。
- **パスキーでの確認**（`features/settings/api/passkeys` の `confirmWithPasskey`）: 本人を確かめる操作（パスキーの追加、パスワードの変更・設定・削除、アカウントの削除）を、パスワードの代わりにパスキーで確かめる。
  `POST /passkeys/confirmation` の options でブラウザの `navigator.credentials.get()` を開き、その答えと `confirmation_token` を操作の要求の `passkey` に入れる。パスワードのない利用者の操作と、パスワードの削除がこれを使う。
  閉じられた確認・使えなかったブラウザ・検証できなかったパスキー・期限切れは、それぞれの文言（`settings.passkey_confirmation.*`）で操作の失敗として出す（E2）。
- **パスキー**（`/settings/passkeys`、設定のメニューの「パスキー」、FR-004、`features/settings` の `PasskeySettings`）: パスキーが何をするか（パスワードの代わりになり、二要素認証のコードは引き続き求められ、パスキーがあればパスワードを削除できる）の説明、
  パスキーを追加するフォーム（名前と現在のパスワードの `TextField`。パスワードのない利用者には名前だけで、登録済みのパスキーで確かめる）、登録済みのパスキーの一覧（古い順）の 3 つ。追加は `POST /passkeys/registration` がパスワードかパスキーを確かめてから、ブラウザの `navigator.credentials.create()` を開き、
  その答えを `POST /passkeys` へ送る（`lib/webauthn` が JSON とブラウザのオブジェクトを変換する）。間違ったパスワードはその欄のエラー、閉じられた確認や検証できなかったパスキーなどは `role="alert"` で出す。
  パスキーに対応していないブラウザではフォームの代わりにその旨を出す。各行は名前（`dir="auto"`）と追加した日時を出し、「削除」（アクセシブルネームに名前を含める）を `ConfirmDialog`（`danger`）で確かめてから `DELETE /passkeys/{id}` を送る。
  削除した行は一覧から消し、フォーカスを次の行のボタンへ移す（`useFocusAfterRemoval`）。パスワードのない利用者の最後のパスキーはサーバーが断り（409 `last-sign-in-method`）、その理由をダイアログに出す。パスキーでのログインはサーバーのログインの画面が行う（§8）。
- **プロバイダ**（`App.tsx`）: 外側から、ランタイム設定、表示のロケール、i18n（`react-intl` と React Aria の `I18nProvider`）、QueryClient（`lib/query`）、セッションと API クライアント（`lib/auth/SessionProvider`、§10）、`AnnouncerProvider` / `ToastProvider`、React Aria の `RouterProvider`、ルーターの順に置く。
- **文言**（`lib/i18n`）: クライアントの文言は `locales/<locale>/client/<area>.json`（領域ごとのファイル、ADR-0082）にある。`vite-plugins/client-catalogs.ts` がロケールごとに 1 つのモジュールにまとめ、ロケールごとに別のチャンクで読み込む。足りない文言は `ja-JP` → `ja` → `en` の順に補う。
  ID の型は `tools/i18n` が生成する `MessageArgs` から作る（`ClientMessageKey`）。文字列にするときは `useT()` の `t("client:…", 引数)` を使い、ID と引数の型を検査する。
- **ルーティング**（`router.ts`）: TanStack Router のファイルルート。Vite のプラグインが `routes/` から `routeTree.gen.ts` を生成する（自動コード分割）。生成物はコミットしない（ADR-0081）。
  `tsc`・Oxlint・knip・テストの前には、`scripts/task.sh web:routes-gen`（`app/web/scripts/routes.ts`）が Vite を通さず同じ設定で生成する。
  - ルーターは遷移を `startTransition` の中で確定するので、次の画面を読み込む間も今の画面が残る（E4）。読み込みが `defaultPendingMs` を超えたときだけ `RoutePending` を出す。
  - `-` で始まるファイルとディレクトリはルートにならない。外枠の部品は `routes/-layout/` に置く。
  - プロフィールのアドレスはサーバーの外枠と同じ `/@username`（リモートのアカウントは `/@username@domain`）で、ルートは接頭辞付きのパラメータ（`routes/@{$acct}.tsx`）にする。
    パラメータの `@` を `%40` にしないように、ルーターは `pathParamsAllowedCharacters: ["@"]` にする。複数の機能が指す画面のアドレスは `lib/paths.ts` で作る。
  - プロフィールの下（`/@{acct}/…`）の第 1 区間は、次の予約語だけにする（[ADR-0053](../../adr/0053-status-page-url.md)）。ID やタグ名などの可変の値は第 1 区間に置かず、語の下に置く。
    語は `[a-z][a-z0-9_-]*` で、1 件を指すページは単数、一覧は複数にする。ルートのファイルは `routes/@{$acct}_.<語>….tsx`。
    語を足すときはこの表を更新する（ADR は要らない）。サーバーが役割を持つのは `status` だけで（外枠の meta タグと、ActivityPub の `Accept` での `id` へのリダイレクト）、ほかの語のアドレスには汎用の外枠が返る。

    | 語 | ページ | 状態 |
    | --- | --- | --- |
    | `status` | 投稿の詳細（`status/{id}`、§13）と、投稿の下のページ（`status/{id}/history` の編集履歴など） | 実装済み |
    | `followers` | フォロワー一覧 | 実装済み |
    | `following` | フォロー一覧 | 実装済み |
    | `media` | メディア付きの投稿 | 予約 |
    | `favorite` | お気に入り | 予約 |
    | `with_replies` | 返信を含む投稿 | 予約 |
    | `tagged` | ハッシュタグで絞った投稿（`tagged/{tag}`） | 予約 |
    | `featured` | ピン留めした投稿 | 予約 |

- **リンク**: 画面の間のリンクは `@my/ui` の `Link` に `href` を渡す。React Aria の `RouterProvider` が同じオリジンの `href` をルーターの遷移に変える。`#id` の `href`（本文へ移動するリンク）は、その要素にフォーカスを移す。
  投稿の本文とプロフィール（`PostContent`）の中のリンクは、サーバーが描画した HTML のもので、自インスタンスのページも `https://{host}/...`（ADR-0018。連合でも同じ HTML を配る）を指す。
  これを同じタブで開くと（修飾キーのない左クリックか Enter）、`PostContent` の `onLinkClick` に渡した `lib/contentLinks.ts` の `useContentLinkClick()` が、
  ホストが `apiOrigin` のホストと同じ（スキームは問わない）プロフィール・投稿・ハッシュタグのリンク（`/@{acct}`・`/@{acct}/status/{id}`・`/tags/{tag}`）をルーターの遷移に変える。
  そのため平文 HTTP のローカルの構成（`apiOrigin` が `http://localhost:8080`）でも、TLS のない `https://` に移らずに該当の画面が開く。修飾キー付きで新しいタブやウィンドウで開くことはブラウザに任せる。
  サーバーはどのリンクにも `target="_blank"` を書くので、`PostContent` は描画の後にリンクごとに開き方を決める（Y12）。上の 3 種類の自インスタンスの画面へのリンク（`ExternalLinkContext` の `isAppLink`、`App.tsx` の `ExternalLinks` が `contentLinkPath` で判定）は `target` を外して同じタブで開き、
  ほかのリンクは新しいタブで開いて、サーバーの `rel`（`nofollow noopener noreferrer`）に `noopener noreferrer` を保ち、「（新しいタブで開きます）」を末尾に足す。
  アプリの外へ出るリンク（ほかのサイト、ソースコードの公開先）は `ExternalLink` で新しいタブで開く（Y12）。
  それ以外のサーバー自身のパス（`/auth`・`/oauth`・`/api` などの予約パス、`lib/paths.ts` の `isServerPath`。登録の画面など）へのリンクはアプリのルートではないので、外枠の `navigate`（`App.tsx`）がルーターに渡さず、`leaveApp` でページごと開く。
- **レイアウト**（`routes/-layout/AppLayout.tsx`。見た目は `@my/ui` の `AppShell`・`AppNavigation`）: モバイルファーストで、`md` 未満は下端のタブバー、`md` から横のレール、`lg` から文言付きのサイドバー、`xl` から 3 列目（右帯）を出す。`md` からのレールには「投稿を書く」の `primary` ボタンが 1 つあり（`lg` から文言付き、`md` はアイコンだけ）、`md` 未満のタブバーは場所だけにして投稿は浮かぶボタンから開く（ADR-0039）。
  投稿フォームはページに置かず、常に `Sheet`（`lg` から画面の中央のパネル、§12）で開く。開くのは、レールのボタン、`md` 未満の浮かぶボタン、返信・引用のボタンで、ページに置くフォームは常に 1 つにする。
  浮かぶボタンは `md` 未満の、ルートの静的なデータ（TanStack Router の `staticData`、`composeButton: true`）を持つ画面（タイムラインとプロフィール）にだけ、タブバーの上に出す（G4）。外枠は今のマッチ（`useMatches`）からそれを読む。
  ほかの画面でもシートは置いたままにし、返信のボタンはそこで開く。浮かぶボタンはアイコンだけで、「投稿を書く」をアクセシブルネーム（`aria-label`）と `Tooltip` にする（G12）。レールのボタンは `md` ではアイコンだけで同じ名前を持つ。
  `main` は終わりに、タブバーとボタン（出ているとき、`data-compose-fab`）の高さぶんの余白を取り、`index.css` の `scroll-padding-block-end` もフォーカスした要素を同じだけ避ける。`md` 未満のトーストは、タブバーの上に、ボタンが出ているときはボタンの上に出す（`with-compose-fab:` の variant）。
  3 列目（右帯）は `AppShell` の `aside` で、`xl` の幅の画面でだけ置く（狭い画面には出さず、そこにある内容は別の画面かメニューのシートから開ける）。上から、通話の開いているルーム（`features/voice` の `VoiceRail`、[ADR-0100](../../adr/0100-voice-chat-rooms.md) §8）、サーバーについて（`ServerInfo`）の順に、それぞれ `AsidePanel`（見出しが名前の `aside`）として並べ、区画ごとに `react-error-boundary` で囲む。
  通話の区画は、サーバーが通話を有効にしていて（`GET /instance` の `voice_enabled`）開いているルームがあるときだけ出し、一覧の先頭の 3 件（並びは API のまま）と「すべてのルームを見る」（`/voice`）を出す。区画はカードなので、中のルームはカードにしない（`VoiceRoomCard` の `surface="plain"`）。通話の画面（`/voice`・`/voice/{id}`）では一覧と重なるので出さない。
  ルームの画面（`/voice/{id}`）がルームを表示している間は、通話の区画の代わりにそのルームの区画（`VoiceRoomAside`）をサーバーについての前に置く: ホストと共同ホストが通話に入っている間の「挙手している人 · N」（一人ずつ「許可」と「却下」。`xl` 未満では同じ一覧をドックの「挙手」ボタンのポップオーバーで開く）と、「ルームについて」（ホスト・見られる人・話せる人と、ホストには設定のダイアログを開くボタン）。
  **ミニプレイヤー**: 通話はアプリ全体の状態で、画面を移っても続く（`features/voice` の `VoiceCallProvider` をアプリの providers に置く。[#1345](https://github.com/mycroblog/my/issues/1345) の回答 45、X のスペースと同じ使い方）。
  通話の間は、すべての画面（アプリの外枠と管理の画面）の `main` の最後に、浮かぶ小さなカードのミニプレイヤー（`VoiceCallBar`。見た目は `@my/ui` の `MiniPlayer`、置き場所は `ShellDock`）を出す。ミニプレイヤーには、話者のアバターの重なり（話している人を先に、輪を付けて。最初の 3 人。飾りなので支援技術には読ませない）、ルーム名（ルームの画面へ戻るリンク）とその下の 1 行（接続している間は「{name}さんが話しています」、ほかにも話している人がいれば「{name}さんほか N 人が話しています」、誰も話していないときと接続中・再接続中は通話の状態）、話者のマイク（`CallDockButton` のトグル）・ブラウザが止めた音を出すボタン・退出（ホストは次のホストか解散を選ぶダイアログ）を並べる。名前と 1 行は狭い幅では省略して、操作を押し出さない。
  ただし、その通話のルームの画面がルームを表示している間（要約やエラーではなく、通話の操作を出している間。パスは末尾の `/` を除いて比べる）は、ミニプレイヤーを出さない。
  ミニプレイヤーは「参加中の通話」（en は "Voice chat call"。ルームの画面のドックの「通話の操作」と別の名前）という名前の `region`（`section` の `aria-label`）で、ランドマークの一覧に出る。`main` の最後の子なので、フォーカスの順はページの内容の後になる。
  **ルームの画面**（`features/voice` の `VoiceStage`）はステージにする: カードに、ライブのピル・人数と開いてからの時間（通話中は通話の状態も）・共有（Web Share API、なければリンクのコピー）・ホストには設定のボタン、ほかの人には「その他の操作」のメニュー（「ルームを通報」）、ページの見出しのルーム名・説明・ハッシュタグを置き、その下に話している人を大きな、聞いている人を小さなアバター（`@my/ui` の `StageAvatar`）の格子で並べる（ホスト・共同ホストを先に、役割・話している輪・マイクオフ・挙手・いま送ったリアクション）。アバターはメニューのボタンで、プロフィールと、ホストと共同ホストにはその人への操作を、話者の自分のアバターには「聞くだけにする」を出す。
  ページの最後はミニプレイヤーと同じ `ShellDock` のドックで、参加前は参加の知らせと「参加する」、通話中は `@my/ui` の `CallDock`（「通話の操作」の `toolbar`）に、話者のマイク（トグル）、聞き手の「話す」と挙手（トグル）、リアクション、ホストと共同ホストの招待と「挙手」（人数の付いたポップオーバー）、最後に退出を並べ、ブラウザが止めた音と失敗はその下に出す。見られる人と話せる人は本文に出さず、設定のダイアログと `xl` の「ルームについて」に出す。
  ミニプレイヤーは `position: sticky` で、ページの列の下端に留まる（ページが画面より短いときは内容のすぐ後に出る）: `md` 未満はタブバーの 1rem 上、`md` から画面の下端の 1rem 上。タブバーを覆わず、浮かぶ投稿のボタンが出ている間（`with-compose-fab:`、`xl` 未満）はミニプレイヤーの終わり側を 4rem 空けてボタンと重ならない。投稿フォームはモーダルの `Sheet` なのでミニプレイヤーの上に開く。
  `main` の最後にあるので、ページを最後まで送れば最後の項目はミニプレイヤーの上に出る。フォーカスした要素もミニプレイヤーを避ける（`index.css` の `:root:has([data-call-bar])` の `scroll-padding-block-end`）。ミニプレイヤーは知らせや折り返しで高くなるので、`MiniPlayer` が自分の高さを `ResizeObserver` で測って文書の `--call-bar-block-size` に書き、余白はそれから計算する。
  短い画面（`short:`、400% の拡大の 320 × 225 CSS px）では、ミニプレイヤーは上端のオフラインの帯（G17）と同じく粘着させず、ページの終わりに置いてページと一緒に流す。固定した要素（タブバーと浮かぶボタン）だけを画面の高さの半分に収める（WCAG 1.4.10）。失敗の知らせ（マイク・退出）はミニプレイヤーの中の 1 行で、`role="alert"` で告げる。管理の画面（`AdminLayout`）でも、タブバーがないので画面の下端の 1rem 上に出す。
  通話の間にタブを閉じる・再読み込みするときはブラウザの確認（`beforeunload`）を出し、ページが去るとき（`pagehide`。戻る / 進むのキャッシュ（bfcache）に入るときも）は `keepalive` の要求で退出する。
  ほかの参加者がいるルームのホストは次のホストを選ばずに退出できない（サーバーの `409`）ので、この退出は断られ、ホストはハートビートが切れてからの猶予（ADR-0100 §6）が終わるまでホストのまま残る。
  このタブのサインアウト（別のアカウントへの切り替えを含む）は、トークンを取り消す前に退出の要求を送ってから通話を終える（`Session` の `onBeforeSignOut`）。ほかのタブのサインアウトでも、利用者が変わったら通話を終え、参加と退出の状態を捨てる。
  メンテナンスの画面（`MaintenanceGate`）とサーバーの閉鎖の画面（`ClosedScreen`）はページを置き換えて帯を出さないので、それが出たら `keepalive` の要求で退出して通話を終える。
  `md` 未満で高さが 32rem 以下の画面（横向きの電話、400% の拡大の 320 × 225 CSS px）は `short:` の variant（`@my/ui/theme.css`）で、タブバーの項目を `min-h-12` にし、ボタンとの間を詰め、上端のオフラインの帯（G17）を粘着させずにページと一緒に流して、固定した要素を画面の高さの半分に収める（WCAG 1.4.10）。
  ナビゲーション（`routes/-layout/Navigation.tsx`）は 1 つの `nav` で、今いる区画のリンクに `aria-current="page"` を付ける。「プロフィール」の区画は自分のプロフィールとそのフォロワー / フォローの一覧で、
  同じ `/@{acct}` の下にある自分の投稿の詳細は含めない。画面の端に接する要素は `env(safe-area-inset-*)` を足し、高さは `svh` で指定する。
  - 項目は [ADR-0039](../../adr/0039-mobile-navigation-and-compose-entry.md) に従い、ローカルと連合のタイムラインは 1 つの画面のタブにせず、どの幅でもナビゲーションの別の項目・別の画面にする（決定者の指示、[#660](https://github.com/mycroblog/my/issues/660)）。
    利用者が並べ替えない限り、`md` 未満のタブバーは、ホーム・ローカル・連合・通知・検索・メニューの 6 つ（サーバーが提供しないタイムラインの項目は出さないので 4〜6 つ）で、
    `md` からのレールとサイドバーは、ホーム・ローカル・連合・通知・検索・プロフィール・ブックマーク・設定の 8 つにする。どちらも 1 つのリストの同じ順番で、各項目はその幅で出ないときに `display: none`（アクセシビリティツリーからも外れる）にする。
  - 利用者は、この 8 つの項目の順番と表示・非表示を「表示と投稿」の「ナビゲーション」の区画（`features/settings` の `NavigationSettings`）で選ぶ（[#662](https://github.com/mycroblog/my/issues/662)）。
    選んだものは表示のテーマと同じくこのブラウザでの表示のしかたなので、localStorage の `my:display:navigation`（並べた ID と隠した ID）に残し、すぐに反映し、セッションが終わっても消さない。ほかのタブでの変更も `storage` イベントで追う。
    残した値は今ある項目と照らし合わせて読み（知らない ID は捨て、ない項目は既定の順番の位置に足す）、「設定」は戻る道なので隠せない。
    タブバーは表示する項目の最初の 5 つ（`tabBarLimit`）とメニューにし、6 つ目からはレールとメニューのシートに出す。サーバーが提供しないタイムラインも最初の 5 つの中の位置を占めたままにするので、`GET /instance` を読む間に項目がタブバーとシートの間を移らない。
    スタッフの 2 つの区分は並べ替えの対象にせず、利用者の項目の後に置く。
    区画は WAI-ARIA APG の並べ替えのできるリスト（listbox の Rearrangeable の例）の考え方に従う。各項目は、表示するかのチェックボックスと、1 つ上・1 つ下へ動かすアイコンのボタン（名前とツールチップは「ホームを上へ移動」）を持ち、どれもふつうのタブ移動で届く。
    動かすとフォーカスは動かした項目の同じボタン（端に着いて押せなくなったときはもう一方のボタン）に残り、新しい位置を `useAnnouncer` で知らせる（「ホームを 8 項目中 2 番目に移動しました」）。各項目の説明は、電話でタブバーとメニューのどちらに出るか・隠していること・サーバーが提供していないことを言う。
    スタッフの画面は「モデレーション」と「サーバー管理」の 2 つの区分に分ける（[ADR-0058](../../adr/0058-moderation-admin-separation.md)、§14）。ロールがモデレーションの画面の権限（`manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit`）のどれかを持つ閲覧者には「モデレーション」（`/moderation`）を、
    サーバー管理の画面の権限（`manage_settings`・`manage_rules`・`manage_custom_emojis`・`manage_announcements`・`manage_webhooks`・`manage_roles`・`manage_jobs`・`manage_finance`）のどれかを持つ閲覧者には「管理」（`/admin`）を、設定の後にこの順で足す。どちらも、その区分に閲覧者の使える画面があるときだけ出すので、組み込みのモデレーターには「管理」が出ない。レールとサイドバーとメニューのシートに出し、タブバーには出さない。
    権限の 3 つめの区分「利用者の機能」（`member`。`view_finance` など、[ADR-0070](../../adr/0070-permission-split-emoji-finance.md)）はこの規則に数えないので、それだけを持つ閲覧者には「モデレーション」も「管理」も出ない。
    サーバーが通話を有効にしている（`GET /instance` の `voice_enabled`、ADR-0100 §2）ログイン中の閲覧者には、利用者の項目のすぐ後に「通話」（`/voice`）を足す。「会計」と同じく並べ替えの対象にせず、レールとサイドバーとメニューのシートに出し、タブバーには出さない。
    `view_finance` を持つ閲覧者には、利用者の項目の後、スタッフの区分の前に「会計」（`/finance`）を足す。スタッフの区分と同じく並べ替えの対象にせず、レールとサイドバーとメニューのシートに出し、タブバーには出さない。
    jsdom はスタイルを当てないので、統合テストでは両方の項目が文書にあり、どの幅で何が出るかは E2E（`e2e/shell.spec.ts`・`e2e/federated.spec.ts`）で確かめる。
  - **サインインしていないときのナビゲーション**: 上の 8 つの項目とスタッフの区分はセッションが要るので出さず、セッションなしで使える画面とサインイン・登録への導線だけを出す（[#781](https://github.com/mycroblog/my/issues/781)）。
    項目は、ローカル（`/local`）・連合（`/public`）・検索（`/search`）・サーバー情報（`/about`）・ログイン（`/login`）の 5 つと、サーバーが新しいアカウントを受け付けている（`GET /instance` の `registrations.mode` が `closed` 以外）ときの新規登録（サーバーが描画する `/auth/sign_up`、§8）で、この順に並べる。
    `md` 未満のタブバーは最初の 5 つとメニューにし、新規登録はレールとメニューのシートに出す。ローカルと連合は、サインインしているときと同じく、サーバーが提供するものだけを出す。並べ替えと非表示（下）はサインインしている利用者の設定なので、この項目には効かない。
    ログインは、今の画面を戻り先（`return_to`）にした `/login` へのリンクで、`/login` にいる間は `aria-current="page"` を持ち、その戻り先を変えない。新規登録はアプリの外のページなので `aria-current` を持たない。
    セッションの再開を待つ間（`unknown`）はリストに項目を出さず、サインインしている利用者に未ログインの項目を、未ログインの閲覧者に利用者の項目を一瞬でも見せない。
    オフラインで開いてセッションを再開できない（`unreachable`、§10）ときは、セッションが残っている見込みが高いので、利用者の項目を出す。
  - `lg` からのサイドバーは、リストの上にサーバーの名前（下の「サーバーの名前」）を、リストの下（`nav` の末尾）にサインインしているアカウントのアバター・表示名・ハンドルを出す。
    サーバーの名前はルートのアドレス（`/`）へのリンク（`routes/-layout/ServerNameLink.tsx`）で、サインインしていない人にはランディングページを、サインインしている利用者にはホームのタイムラインを開く。
    名前は見えている名前、説明（`aria-describedby`）は「このサーバーのトップページ」にし、ランディングページにいる間は `aria-current="page"` を持つ。`xl` からの 3 列目（`ServerInfo`）とメニューのシートも、サーバーの名前を同じリンクで出す。
    アカウントはそのプロフィールへのリンクで、名前は表示名とハンドル（それぞれ 1 行に省略、G7）、説明（`aria-describedby`）は「サインインしているアカウント」にする。
    場所はリストの「プロフィール」が示すので、このリンクは `aria-current` を持たない。どちらもリストの外に置くので、リストのリンクとその順番はどの幅でも変わらない。`lg` 未満では出さず、サインインしていないときと読み込み中はアカウントを出さない。
  - タブバーの各項目は文言の幅を基本にし、残りの幅を等分する（`flex: 1 1 auto`）。長い文言（「Notifications」）が短い文言（「Menu」）の余りを使う。
    文言は省略せず、収まらない翻訳（6 つの項目の 320 px）では単語の途中でも折り返して、全体を読めるようにする。
  - 「ローカル」（`/local`）と「連合」（`/public`）はそれぞれの画面へのリンクで、その画面でだけ `aria-current="page"` になる。どちらの画面も見出しの下にそのタイムラインだけを置き、もう一方へのタブを持たない。
  - 「メニュー」は場所ではなくボタンなので `aria-current` を持たない。押すと `Sheet`（モーダルのダイアログ）を開き、タブバーにないレールの項目（プロフィール・ブックマーク・設定、閲覧者によってモデレーションと管理）を並べて、今いる区画のリンクに `aria-current="page"` を付ける。項目の下に表示のテーマの切り替え（横に並べた `ThemeRadioGroup`）を置き、その下にサーバーの名前（ルートのアドレスへのリンク。サイドバーのない幅でもランディングページへ移れる）、インスタンスの情報ページへのリンク、規約とポリシーへのリンクを置く。
  - サーバーの利用規約とプライバシーポリシー（`GET /instance` の `terms_url` / `privacy_policy_url`、`features/documents` の `DocumentLinks`）は、`xl` からの 3 列目（`ServerInfo`）、メニューのシートの末尾（テーマの切り替えの下）、設定の画面の「規約とポリシー」の区画、サインインの画面（`/login`）にリンクする（`nav`「規約とポリシー」）。サーバーが公開していない文書は出さない。
  - 件数のバッジ（`@my/ui` の `Badge`）: 「通知」は未読数、「モデレーション」はモデレーターを待つ件数（`GET /admin/counts` の未解決の通報・承認待ちの登録・審査待ちの異議申し立ての合計、FR-712）を、アイコンの角に数字で出す。
    数字はアクセシビリティツリーから隠し、件数はリンクのアクセシブルネーム（見える名前で始まる「通知、未読 3 件」「モデレーション、未対応 3 件」、WCAG 2.5.3）で伝えるので、色だけで示さない（WCAG 1.4.1）。
    シートの「モデレーション」も同じ名前とバッジを持ち、タブバーの「メニュー」のボタンはシートの項目の件数を自分のバッジと名前（「メニュー、モデレーションに未対応 3 件」）で示す。「管理」には件数を出さない。
    件数は、その件数に要る権限（`manage_reports`・`manage_accounts`）を持つ閲覧者だけが読む。ストリームの `admin_report` / `admin_sign_up` / `admin_appeal` の通知と再接続、登録の決定と通報の処理で読み直す。読めないときはバッジを出さない。
    シートはパスの変わる遷移が始まったとき（ルーターの `onBeforeNavigate`）に閉じるので、新しい画面の `h1` がフォーカスを受け取れる。未保存の変更の確かめで止めた遷移は始まらないので、シートは開いたまま残る。
  - ローカル（`/local`）と連合（`/public`）のタイムラインは、インスタンスの設定で無効にでき、そのときは API が 404 を返す（FR-202）。タブバー・レール・サイドバーは `GET /instance` の `timelines` で、
    サーバーが提供するものだけを出す（`features/timeline` の `useTimelineAvailability`）。ローカルはサーバーが無効と答えるまで出し、連合はサーバーが有効と答えてから出すので、無効にしたサーバーでは読み込み中も失敗したときも出ない。
  - 最後に受け取った `timelines` は localStorage の `my:instance:timelines` に残し（インスタンスの設定で、利用者のデータではない）、ナビゲーションを置いたときに 1 回だけ読む。
    オフラインで開いたりリロードしたりして `GET /instance` が届かないときも、前と同じ項目を出す。サーバーの答えが届けばそちらを使う。
  - 無効にしたタイムラインのアドレスを直接開いたとき（古いリンクやブックマーク）は、「もう一度試す」ではなく無効であることを出す（`routes/local.tsx`・`routes/public.tsx`）。
  先頭に、フォーカスしたときだけ見える「本文へ移動」のリンクを置く。
- **ランディングページ**（`routes/index.tsx`、`features/landing`）: ルートのアドレス `/` は、サインインしていない人にはランディングページを、サインインしている利用者にはホームのタイムライン（`/home` へ置き換えの遷移）を出す。
  セッションを再開できるかを loader で確かめてから決める。サーバーに届かず再開できないとき（オフラインで開いたとき）も、このブラウザに残した投稿を読めるように `/home` へ移す。
  外枠の中（ナビゲーション付き）に出し、見出し（`h1`）は「{サーバーの名前} へようこそ」にする。中身は `GET /instance` から、サムネイル（`thumbnail`。情報ページと同じ `ServerThumbnail`）、要約（`short_description`）、
  サインインのボタン、登録の画面（サーバーが描画する `/auth/sign_up`、§8）へのリンク（登録の方式 `registrations.mode` で文言と説明を変え、`closed` ではリンクの代わりに受け付けていないことを出す）、
  説明（`description`。空行で段落に分ける）・最近の投稿・ルール・利用規約とプライバシーポリシーの区画（`DocumentsSection`）・インスタンスの情報ページ（`/about`）へのリンク・ソースコードへのリンク（情報ページと同じ `SourceCodeLink`）を順に出す。どれも管理者が書いた、どのロケールでも同じ 1 つの値の文なので、`dir="auto"` と `lang=""` で出す（Y10）。
  並びは情報ページと同じ考え方にする。サーバーが何かを先に置き、ランディングにしかないサインインと登録の導線はその要約の直後に、最近の投稿はサーバーが何かの最後（説明の後）に置く。
  次に利用者が従う決まり（ルール・規約・ポリシー）を続け、他のサーバーとクローラーへの接し方と運営者の連絡先は情報ページへのリンクに任せ、動かしているソフトウェア（ソースコード）を最後に置く。
  最近の投稿は、サーバーがローカルのタイムラインを提供しているときだけ、その最新の 4 件（`GET /timelines/local?limit=4`。タイムラインの無限の query とは別の query）を、操作のボタンのない投稿の feed（Y6）にし、ローカルのタイムラインへのリンクを付ける。
  読み込み中と失敗はその区画の中に出す。`GET /instance` が失敗しても、ページは外枠のランタイム設定の名前とサインインのボタンで開く。
  サーバーはこのアドレスに、インスタンスを紹介するリンクプレビュー（`og:type` は `website`。[リンクプレビュー](../../operations/link-previews.md)）付きの外枠を返す。
- **インスタンスの情報ページ**（`routes/about.tsx`、`features/about`、FR-718）: `/about` は、サインインしているかによらず同じ中身を、外枠の中に出す。見出し（`h1`）は「{サーバーの名前} について」にする。
  中身は `GET /instance` からサムネイル・要約・説明・ルール（ランディングページと同じ出し方）、利用規約とプライバシーポリシーの区画（`DocumentsSection`）、処分したドメインの一覧（`GET /instance/domain_actions`）、検索エンジンと AI への表明、運営の費用、サポーター、特商法の表記、運営者の連絡先、ソースコードの区画、ライセンスの区画を順に出す。
  利用者が従う決まり（ルール・規約・ポリシー）を続けて置き、次にサーバーが他のサーバーとクローラーにどう接するか、最後に運営者と動かしているソフトウェアを置く。
  サムネイル（`thumbnail`）はページの幅の画像で、代替テキストは管理者が書いたもの（無ければ見出しがサーバーを名指すので装飾として `alt=""`）にする。
  連絡先（`contact`）は「連絡先」の区画の説明リストで、アカウント（アバター・表示名（`dir="auto"`）・ハンドル（`dir="ltr"`）をプロフィールへの 1 つのリンクにする）とメールアドレス（`mailto:` のリンク、`dir="ltr"`）を、サーバーが返したものだけ出す。どちらも無ければ区画ごと出さない。
  ソースコードの区画は、AGPL-3.0 第 13 条（ADR-0011）により利用者がソースコードを入手できることと、公開先（`source_url`）へのリンク（`SourceCodeLink`）を出す。
  `https:` の URL なら「ソースコードを見る」（`ExternalLink`。新しいタブで開く、Y12）、`mailto:` なら「ソースコードをメールで請求する」にし、情報ページではその下に請求先のアドレス（`dir="ltr"`）を説明リストで出す。
  ライセンスの区画は、Web のアプリが同梱するほかの人のソフトウェア・データ・フォントのライセンスの表示へのリンクを、サーバーの設定によらずいつも出す。
  アプリと Service Worker に含まれる npm のパッケージ（Vite の `build.license` が作る `licenses/npm-packages.md`・`licenses/npm-packages-sw.md`）、Unicode の絵文字のデータと CLDR の名前（`LICENSE-Unicode.txt`、ADR-0091 §1）、Noto Color Emoji のフォント（`OFL.txt`、ADR-0095 §6）の 4 つである。
  どれもビルドの `dist/licenses/` のファイルをサーバーが `/licenses/` で平文（`text/plain`）として配るもので、アプリのルートではないので、リンクはアプリの外のページとして同じタブで開く（`lib/paths.ts` の `isServerPath`）。同梱するものを増やしたら、`vite.config.ts` とこの区画の両方に足す。
  処分したドメインの一覧は、インスタンス設定 `domain_actions_visibility` に従う。API が 404（誰にも見せない）を返すときは区画ごと出さず、401（ログインした利用者だけに見せる。サインインしていない閲覧者）を返すときは、一覧の代わりにそのことと `/login` へのリンクを出す。
  どちらも設定の答えなので再試行しない。項目はドメイン（伏せ字はサーバーが返したまま、`dir="ltr"`）・種類・公開コメント（`dir="auto"`。見せてよい相手にだけサーバーが返す）で、伏せ字のドメインがあるときは一覧の前にそのことを書く。
  一覧は 40 件ずつで、続きは「さらに表示」のボタンで読む。loader は `GET /instance` と一覧の最初のページを待つが、どちらの失敗もページの失敗にはせず、一覧の失敗はその区画の中に出す。
  運営の費用の区画は、`GET /instance` の `finance_visibility`（FR-903、[ADR-0067](../../adr/0067-finance-cost-records.md) §8）か `income_visibility`（FR-917、[ADR-0073](../../adr/0073-income-records-and-publication.md) §7）のどちらかが `private` でないときだけ出し、
  費用と収入のそれぞれについて何を公開しているか（費用は合計だけか、内訳までか。収入は合計だけか、プランごとの人数までか。項目の名前・メモ・個々の支払い・誰が払ったかは出さない）、どちらも公開しているときは収支も公開していること、`/about/costs` へのリンク（公開しているものを名前にする）を置く。
  サポーターの区画（`features/supporters` の `AboutSupportersSection`、ADR-0072 §14、ADR-0071 §4）は、`GET /instance` の `supporters_enabled` が真のとき、加入を受け付けているプラン（`GET /instance/supporter_plans`。名前と説明は `dir="auto"`・`lang=""`、月額と年額）と `/settings/supporter` へのリンクを、
  `supporter_list_visibility` が `off` でないとき `/about/supporters` へのリンクを出す。どちらもなければ区画を出さない。Tauri のアプリでは、プランと加入へのリンクを出さず、一覧へのリンクだけを出す（下の「Tauri のアプリ」）。
  `xl` からの 3 列目（`ServerInfo`）・メニューのシート（テーマの切り替えの下）・ランディングページからリンクする。
  サーバーはこのアドレスに、ランディングページと同じインスタンスのリンクプレビュー（タイトルは「{サーバーの名前} について」）付きの外枠を返す。
- **運営の費用のページ**（`routes/about_.costs.tsx`、`features/costs`、FR-903・FR-917、ADR-0067 §8、ADR-0073 §7）: `/about/costs` は、サインインしているかによらず同じ中身を、外枠の中に出す。見出し（`h1`）は「{サーバーの名前} の運営の費用」にする。
  計上のしかたと週・月・年のタブ（どちらも URL に置く。S3）を 1 組だけ置き、タブの中に「費用」と「収入」の区画（`h2` の `section`）を並べる。費用と収入は同じ期間と同じ計上のしかたで読む。今日の日付は端末のタイムゾーンでルートの loader が決める。
  「費用」は管理画面の費用の集計（下の「費用」）と同じ部品（`CostTotalsView`）で、新しい期間が先の合計の表と期間の区切りの規則を出し、`GET /instance/cost_totals` を読む。今の期間の事業者・区分ごとの内訳は `finance_visibility` が `breakdown` のときだけ出し、`totals` のときは合計だけを公開していることを書く。
  「収入」は `IncomeTotalsView` で、新しい期間が先の手数料を引いた収入の表と、応答が収支を持つとき（費用も公開しているとき）だけ収支の列（符号付き、通貨ごとに `Intl.ListFormat` で並べる）、応答が `plans` を持つときだけいまのプランごとの加入者の数（名前は `dir="auto"`・`lang=""`、人数は `plural`。期間ごとの数でもプランごとの金額でもないことを書く）、
  収入の数え方の規則（手数料と返金・事業者ごとの日付のタイムゾーン・選んだ計上のしかた・収支）を出し、`GET /instance/income_totals` を読む。費用を公開していないときは、そのため収支を公開していないことを書く。総額・手数料・個々の支払い・加入者は出さない。
  どちらかの API が 404（公開しない）を返すときはその区画を出さず、どちらも 404 のときは、タブの代わりにどちらも公開していないことと `/about` へのリンクを出す。これは設定の答えなので再試行しない。
  loader は `GET /instance` と 2 つの合計を待つが、どの失敗もページの失敗にはせず、合計の失敗はそれぞれの区画の中に出す。
- **サポーターの一覧**（`routes/about_.supporters.tsx`、`features/supporters` の `SupporterListView`、[ADR-0071](../../adr/0071-role-badges-and-supporter-list.md) §4）: `/about/supporters` は、インスタンス設定 `supporter_list_visibility` の範囲で、外枠の中に出す。見出し（`h1`）は「{サーバーの名前} のサポーター」にする。
  `GET /instance/supporters` を無限の query で読み、応答の `plans` の順にプランの名前の見出し（`h2`、`dir="auto"`・`lang=""`）の区画を作り、各人を情報ページの連絡先と同じ 1 つのプロフィールへのリンク（アバター・表示名（`dir="auto"`）・ハンドル（`dir="ltr"`））で並べる。
  一覧はプランの順、プランの中では新しい順に並ぶので、続きのページの人は同じ見出しの下に続く。読み込めた人のいないプランの見出しは出さない。続きは「さらに表示」で読む。金額・月払いか年払いか・日付・人数は出さない。
  API が 404（誰にも見せない）を返すときは、そのことと `/about` へのリンクを、401（サインインした人だけに見せる。サインインしていない閲覧者）を返すときは、そのことと戻り先をこのページにした `/login` へのリンクを出す。どちらも設定の答えなので再試行しない。
  loader は `GET /instance` と最初のページを待つが、どちらの失敗もページの失敗にはしない。サーバーはこのアドレスに、設定によらず `noindex, noarchive` の汎用の外枠を返す（ADR-0057）。
- **会計のページ**（`routes/finance.tsx`、`features/costs` の `FinanceCosts`、[ADR-0070](../../adr/0070-permission-split-emoji-finance.md) §5）: `/finance` は `view_finance` を持つ人（`manage_finance` を持つ人も読める）の、利用者の側の画面で、スタッフの画面（`/admin/*`）の外に置く。
  見出し（`h1`）は「会計」で、ナビゲーションの項目の画面なので見た目からは消す。`GET /finance/cost_totals` と `GET /finance/income_totals`（スコープ `read:finance`。`admin:*` を要さない）を読み、運営の費用のページと同じ部品（`CostTotalsPanel`・`CostTotalsView`・`IncomeTotalsView`）で、計上のしかたと週・月・年のタブ（URL に置く。S3）を 1 組置き、
  タブの中の「費用」の区画（`h2` の `section`）に合計の表、今の期間の事業者・区分ごとの内訳（設定によらずいつも）、期間の区切りの規則を、「収入」の区画に手数料を引いた収入と収支（いつも）の表、応答が `plans` を持つとき（`finance_viewer_income` か `income_visibility` が `plans`、または `manage_finance`）だけいまのプランごとの加入者の数、収入の数え方の規則を出す。総額・手数料・個々の支払い・加入者は出さない。
  インスタンス設定 `finance_viewer_item_names`・`finance_viewer_item_memos` のどちらかが有効なら、今の期間の項目の行（名前・事業者・区分・額・メモ。名前とメモはサーバーが返したときだけ。`dir="auto"`）を内訳の下に出し、画面の説明の下に、項目の名前とメモのどちらを見せているかを書く。`manage_finance` を持つ人にはサーバーがすべての行を返すので、この文を出さない。
  権限のない閲覧者には、要求を送らずに理由（権限の名前）を出す。loader は閲覧者を待ち、権限があるときだけ 2 つの合計を待つが、合計の失敗はページの失敗にはせず、それぞれの区画の中に出す。
- **自分のカスタム絵文字**（`/settings/custom-emojis`、`routes/settings.custom-emojis.tsx`、`features/custom-emojis` の `OwnCustomEmojis`、[ADR-0070](../../adr/0070-permission-split-emoji-finance.md) §1〜§4）: `create_custom_emojis` を持つ人の設定の画面で、ナビゲーションの項目を増やさず、設定のメニューにその人にだけ「あなたのカスタム絵文字」を出す。
  権限のない閲覧者には、要求を送らずに理由（権限の名前）を出す。loader は権限があるときだけ自分の絵文字（`GET /custom_emojis/mine`）と申請（`GET /custom_emoji_requests`）を待つ（スコープ `read:custom_emojis` / `write:custom_emojis`。`admin:*` を要さない）。
  「カスタム絵文字を申請する」「あなたの申請」「あなたが作ったカスタム絵文字」の 3 つの区画（`h2` の `section`）を並べる。
  - **申請**: 使った数と上限（「{上限} 個のうち {使った数} 個。審査中の申請を含む」、上限がなければそのこと）と、画像（`FileTrigger`）と shortcode のフォーム。管理画面の登録と同じく画像を `POST /media`（`purpose=emoji`）で送って処理を待ち、`POST /custom_emoji_requests`（`kind=create`）を送る。
    サーバーの拒否（422 `invalid-custom-emoji`。上限の `limit_reached` も）は `detail` をフォームの下（`role="alert"`）に出して入力を残す。成功したら入力を空にし、申請と自分の絵文字（使った数）に無効化の印を付けてから、結果を `StatusMessage` に出す。
  - **申請の一覧**: 新しい順。各申請は画像と見出し（`:shortcode:`、`dir="ltr"`）と、内容（新しい絵文字・画像の差し替え）・状態・申請した日時・終わった日時・却下の理由（`dir="auto"`）の説明リスト。審査中の申請は「取り下げる」を持ち、応答で行を置き換える。
  - **自分の絵文字**: 新しい順。各絵文字は管理画面の行と同じ説明リストで、「編集する」（カテゴリ・別名・ピッカーに出すかのダイアログ。有効・無効は管理者だけが変える）・「画像の差し替えを申請」（`FileTrigger` で選ぶとすぐに `kind=image` の申請を送る。承認まで絵文字は今の画像のまま）・「削除する」（`ConfirmDialog`）を持つ。
    変えた絵文字は応答で行を置き換え、消した絵文字は一覧から除いてフォーカスを次の行のボタンに移し、使った数・申請（画像の申請が失効する）・ピッカーの絵文字に無効化の印を付ける。
- **サポーターのプラン**（`/settings/supporter`、`routes/settings.supporter.tsx`、`features/supporters` の `SupporterSubscriptionSettings`、[ADR-0072](../../adr/0072-supporter-plans-and-roles.md) §6・§8・§13・§14）: 設定のメニューは、`GET /instance` の `supporters_enabled` が真のときだけ（Tauri のアプリでは出さない）「サポーターのプラン」を出す。
  loader は公開のプラン（`GET /instance/supporter_plans`）と自分の加入（`GET /supporter/subscription`。続いている加入と、加入できない理由 `ineligible_reason`）を待つが、失敗はページの失敗にせず本文に出す。どちらかが 404 なら、課金が無効でプランを提供していないことだけを出す。
  - **加入**: 続いている加入がなければ「プランを選ぶ」の区画に、プランの `RadioGroup`（名前は `dir="auto"`・`lang=""`、説明に月額と年額と管理者の説明）と支払いの間隔（月払い・年払い、選んだプランの額）の `RadioGroup` を出す。加入できない利用者には理由（メールの確認がまだ・このサーバーの利用者でない・凍結か削除の手続き中）を書き、「加入する」を出さない。
    「加入する」は最終の確認（`ConfirmDialog`、`primary`）を開き、プランの名前・間隔ごとの額と更新しても同じであること・支払いの時期（いますぐ、以降は毎月か毎年）・特典が始まる時期・解約の方法と効果（このページからいつでも、支払った期間の終わりまで特典が続く）を説明リストで、特商法の表記（`commerce_disclosure_url`）があればそのリンク（`DocumentLink`、同じタブ）を出す（§13）。
    「支払いへ進む」は `POST /supporter/checkout` を送り、応答の `redirect_url`（決済事業者のホスト型の決済画面）へ外枠の `leaveApp` で同じタブのままページごと移る。カード情報はこのアプリを通らない。断られたら（422 `supporter-ineligible`・`supporter-plan-unavailable`、409 `supporter-subscribed`）、`type` に対応する文言をダイアログに出す（E2）。
  - **戻り**: 決済画面は `?checkout=success`（済んだ）か `?checkout=canceled`（離れた）、顧客ポータルは `?portal=returned` を付けてこのページに戻す。検索条件は Zod で検証し、知らない値は無視する（S3）。`success` と `returned` では、loader が 1 度だけ `POST /supporter/subscription/sync`（事業者からの取り直しを積み、いまの加入を返す）を送って応答をキャッシュに書く。
    `success` で加入がまだ `pending` の間は、事業者の確認を待っていることを `StatusMessage` に出し、`GET /supporter/subscription` を 3 秒ごとに最大 20 回読み直す。確かめられたら礼を、`canceled` では請求されていないことを出す。
  - **自分の加入**: 「あなたの加入」の区画に、プラン（`dir="auto"`）・状態・更新の額（もとの価格で続けているときはその額）・次の更新か終了（`canceling`）の日時・特典の終わり（`past_due`）・予約した変更（変更先と額と日時）の説明リストと、状態が求めること（支払い待ち・解約の予約・支払いの失敗・猶予の切れ）の文を出す。
    操作は状態ごとに、`pending` は「支払いに進む」（同じプランと間隔でもう一度 `POST /supporter/checkout`）、`canceling` は「解約を取り消す」、予約した変更があれば「予約した変更を取り消す」、`active` と `past_due` は「解約する」（`ConfirmDialog`、`danger`。期間の終わりまで特典が続き、それ以降の請求がないこと）、`pending` 以外は「支払い方法と請求書」（`POST /supporter/portal` の URL へ `leaveApp` で移る）。
    `active` のときだけ「プランを変更する」の区画に同じ 2 つの `RadioGroup` を出し、上位のプランや年払いはすぐ（差額を日割りで請求）、下位のプランや月払いは期間の終わりに変わることを説明し、確認のダイアログに選んだ変更がどちらかを書いてから `POST /supporter/subscription/change` を送る。いまのプランと間隔のままならボタンを押せない。もとの価格で続けているときは、変えると変更先のいまの価格になることを書く。
    変更・解約・取り消しは `POST /supporter/subscription/{change,cancel,resume,cancel_scheduled_change}` で、応答の加入でキャッシュを置き換え、掲載の選択・サポーターの一覧・閲覧者のアカウント（ロールとバッジ）に無効化の印を付け、結果を区画の `StatusMessage` に出して `useAnnouncer` で知らせる。失敗は `type` に対応する文言で出す。
  - **掲載の選択**（ADR-0071 §4）: `/settings/profile` の「サポーターの一覧」の区画（`SupporterListingSettings`）で、`GET /preferences/supporter_listing` の `listed` を `Switch` にし、変えるとすぐ `PUT` で保存してサポーターの一覧に無効化の印を付ける（既定は載せない）。
    区画の説明に、一覧にはプランの見出しの下にアバター・名前・ハンドルだけが出て金額や日付は出ないこと、加入が終わると選択も消えることを書く。プランに加入していない利用者（API が 404）と、`supporter_list_visibility` が `off` のときは区画を出さない。特典の設定なので Tauri のアプリでも出す。
- **Tauri のアプリ**（`lib/platform.ts` の `isTauriApp()`、ADR-0072 §14）: Web クライアントが Tauri 2 のアプリ（ADR-0008）の WebView の中で動くかは、Tauri がページのスクリプトより前に置く `window.isTauri` が `true` かだけで決める（`@tauri-apps/api` の `isTauri()` が読むのと同じ値。依存は足さない）。ほかの手段（User-Agent、ビルドの旗）で見分けない。
  アプリでは、加入・プランの変更・解約・解約の取り消し・顧客ポータルの画面（`/settings/supporter`）、設定のメニューのその項目、`/about` のプランの区画と加入へのリンクを出さない。`/settings/supporter` を直接開いても、プランも加入も読まずに、この画面はアプリでは使えないことだけを出す。
  購入への導線も、Web で加入できることの案内も出さない。特典（プロフィールのバッジ、`/settings/custom-emojis`、`/finance`）と、サポーターの一覧とそのリンク、掲載の選択はアプリでも出す。新しく購入につながる画面やリンクを足すときは、この関数で出し分ける。
- **フォーカスとタイトル**（`routes/-layout/useViewChange.ts`、Y5）: ルーターが画面を描画するたびに、`main` の `h1` の文字列から `document.title` を `client:shell.document_title`（画面名とサーバー名）で作る。サーバーの名前が変わったときも作り直す（フォーカスは動かさない）。
  パスが変わったときだけ、その `h1` にフォーカスを移す。最初の画面と、検索条件やハッシュだけの変化（タブ・絞り込み）ではフォーカスを動かさない。画面の見出しは `PageHeading`（`tabindex="-1"` の `h1`）で書く。
  ナビゲーションの項目の画面で、見出しが場所を言うだけのもの（ホーム・ローカル・連合・通知・検索・ブックマーク・設定・会計・モデレーション・サーバー管理）は、
  どこにいるかをナビゲーションの現在の項目（`aria-current="page"`）が見せているので、`h1` を見た目から消す。ルートの `staticData.hideHeading` を真にすると、`PageHeading` が `sr-only`（Tailwind の視覚的に隠すクラス）を付け、
  `h1` はアクセシビリティツリー・フォーカスの移動・`document.title` のためにそのまま残る。同じルートの読み込み中（pending component）と失敗（`RouteError`）の見出しも隠し、失敗の理由は `RegionError` の `h2` が見せる。
  場所より多くを言う見出し（サーバーがタイムラインを提供していないときの 404 の「ローカルタイムラインは提供されていません」など）は `visuallyHidden={false}` で見せる。名前がパラメータで決まる画面（ハッシュタグ・プロフィール・投稿）、設定の下の画面（`SettingsHeading`。設定へ戻るリンクが見出しの後にある）、情報ページとランディングページは見出しを見せる。
- **サーバーの名前**（`lib/instanceName.ts` の `useInstanceName`）: サイドバー・3 列目の「このサーバーについて」・メニューのシート・タブのタイトル・ログインの画面は、サーバーを `GET /instance` の `name`（閲覧者の言語）で呼ぶ。
  読み込むまでと読めないときは、外枠のランタイム設定の `instanceName`（サーバーが既定の言語で書いた名前）にする。どちらも、名前がなければサーバーのドメインである。
  管理画面で名前を変えると、取り直した `GET /instance` で再読み込みなしに変わる。ハンドルとリンクは名前によらずドメイン（ランタイム設定の `accountDomain`）のままにする（[名前とドメイン](../../operations/instance-settings.md#name-and-domain)）。
- **エラー**（E1、G16）: 画面の失敗はルーターの `defaultErrorComponent`（`RouteError`）がレイアウトの中に出す。画面の `h1` はその画面の名前のままにし（ルートの `staticData.title` のメッセージ ID。名前がパラメータで決まる画面は自分の `errorComponent` から `heading` を渡す）、本文の位置に `RegionError` を出す。
  見出しは失敗の種類（`lib/errors.ts` の `errorKind`: 通信できない・サーバーの失敗・見つからない（404）・権限がない（403）・画面自体の不具合）ごとに分け、理由は `errorMessage(error)`（通信できないときはオフラインかどうかも言う。画面自体の不具合はカタログの文）にする。404 と 403 には「もう一度試す」を出さない。
  「もう一度試す」は、まずその失敗を起こした query だけをその場で取り直す。また失敗したら、エラーとボタンのフォーカスを残したまま新しい理由を `role="alert"` で出す。取れたら Query のエラー境界を戻して loader を再実行し、画面の `h1` にフォーカスを移す。
  レイアウト自体の失敗はルートの `errorComponent`（`RootError`）が画面全体に出す。合うルートのないアドレスは `NotFound` を出す。独立した区画（3 列目など）は `react-error-boundary` で囲む。
  見せるものがない画面（合うルートのないアドレス・見つからないアカウント・無効にしたタイムライン）の出口は `routes/-layout/HomeLink.tsx` の 1 つのリンクにし、サインインしている利用者にはホームのタイムライン（`/home`）、サインインしていない人にはランディングページ（`/`、「トップページへ」）を指す。
  オフラインでセッションを再開できないとき（`unreachable`）は、ナビゲーションと同じくホームのタイムラインを指す。
- **オフライン**（`lib/network.ts`、G17）: オンラインかどうかは TanStack Query の `onlineManager` 1 つに置き、Query のキャッシュとオフラインの帯が同じ状態を読む。
  ブラウザが `navigator.onLine` と `offline` / `online` のイベントでネットワークがないと言うとき（起動時も `navigator.onLine` から始める）と、
  API クライアントの要求が応答を受け取れなかったとき（`NetworkError`。次に応答が届くまで）にオフラインとする。
  - レイアウトが画面の上端に `OfflineBanner`（`role="status"`）を 1 つ置き、最初から空のまま置いておくので、オフラインになると読み上げられる。安全領域の上の余白は帯が自分で取るので、その余白を取る要素より前に置く。
  - オフラインの帯は、残したステータスページの URL（§11 の「ステータスページの URL」）があれば、文言の後にステータスページへの外へ出るリンク（「サーバーの状態を確かめる」、Y12）を出す。
    リンクは `OfflineBanner` の `action` で、ライブリージョンの外に置き、読み上げは文言だけにする。レート制限の帯には出さない。
  - query と mutation は `networkMode: "always"` にする。オフラインでも止めて待たずにすぐ `NetworkError` で失敗するので、区画は終わらないスピナーではなく読み込めない理由を出し、loader も待ち続けない。
    ブラウザがオフラインのときの `NetworkError` の文言は「オフラインのため、サーバーに接続できません」（`client:api.error.offline`）になる。`NetworkError` の再試行は待たずにすぐ行い、知らせを遅らせない（`retry` は既定のままにし、loader の `queryClient.query` は再試行しない）。
  - オンラインに戻ると、失敗していた query（表示中のもの）だけを取り直す（既定の `refetchOnReconnect` が失敗した query にだけ `"always"` を返す）。タイムラインは自分で `refetchOnReconnect: false` にしているので取り直さない（§10）。
    `NetworkError` で失敗した画面は、`RouteError` がオンラインに戻ったときに「もう一度試す」と同じことをして読み込み直す。
  - 通知の画面の loader は失敗しても画面を開き、一覧が見出しの下に自分の失敗を出す（`useNotifications`。読み込みの失敗はマウントで再試行しない）。
- **外枠の読み込み**: 外枠はどの画面でも、閲覧者（`GET /accounts/verify_credentials`、`useViewer`）・設定（`GET /preferences`、`useUserDisplaySettings`）・インスタンス（`GET /instance`、`useTimelineAvailability`）・
  通知の既読位置（`GET /notifications/marker`、`useUnreadNotificationCount`）を読む。どれが失敗しても画面全体のエラーにはせず、前に読めた値（Query のキャッシュ）か既定の値（閲覧者は読み込み中か `AccountSection` の区画のエラー、
  表示言語は上の「表示のロケール」、タイムラインは localStorage の値、未読数は 0）で表示を続ける。閲覧者は、前に読めたアカウントがあれば再取得が失敗してもサインイン中のままにする。
  これらの query の `retry` は `lib/query.ts` の `shellQueryRetry` にする（loader は再試行しないまま）。同じものを外枠と同時に読む投稿フォーム（`useComposeForm`）は `shellQueryReader` で、さらに `retryOnMount: false` にし、外枠がすでに知っている失敗をマウントのたびに取り直さない（429 の制限を使い増やさず、帯も消えて出直さない）。4xx（429 を含む）は再試行せず、それ以外（5xx・`NetworkError`）は 1 回だけ再試行するので、失敗は約 1 秒で区画に出る。
- **レート制限**（`lib/rateLimit.ts`）: サーバーのレート制限の 429 は `Retry-After`（秒数か HTTP の日付）を持つ。API クライアントはそれを `ApiError` の `retryAt`（時刻）にし、`rateLimitedUntil(error)` が 429 のときだけそれを返す。
  レイアウトの `useRateLimitRetry` が、画面で使っている query（observer のあるもの）のうち 429 で失敗して止まっているものを Query のキャッシュから探し、その最も遅い `retryAt` に、それらを取り直す（時刻が理由なので Effect のタイマー）。
  その間、オフラインの帯（`ConnectionBanner` が出す 1 つの `OfflineBanner`）に、リクエストが多すぎることと、読み込み直す時刻（秒まで）を出す。オフラインのときはオフラインの文言を優先する。
  `src/shell.test.tsx` が外枠の 4 つの要求の失敗（5xx と 429）・前の値の保持・再読み込み後の表示言語・429 の帯とその時刻での取り直し・アカウントの区画の再試行を MSW で検査し、
  `e2e/rate-limit.spec.ts` が 429 の帯の axe・横スクロール・ARIA スナップショットと、時刻での取り直しを検査する。
- **メンテナンス**（`lib/maintenance.ts`、`src/Maintenance.tsx`、[ADR-0084](../../adr/0084-maintenance-mode.md) §3）: メンテナンスモードのサーバーは、client API と OAuth の JSON の端点に `503` と Problem（`type: /api/v1/problems/maintenance`）と `Retry-After` を返す。
  API クライアントとセッションはこれを `MaintenanceError`（`ApiError` に、告知文 `notice`・終了予定 `until`・ステータスページ `statusPageUrl` を足したもの。形の正しいものだけ持つ）にし、
  ページに 1 つの `Maintenance`（React の外のストア、`lib/services.ts` が作る）に知らせる。この 503 ではトークンを捨てず、更新の失敗として扱わず、サインアウトしない（§10）。
  - 外枠（`App.tsx` の `MaintenanceGate`）は、ストアがメンテナンス中の間、ルーターの代わりに画面全体をメンテナンスの表示（`StandalonePage` の `main`）にする。
    見出し（`h1`。表示したときにフォーカスを移し、`document.title` も「メンテナンス中」にする）・説明・告知文（管理者の文なので `dir="auto"`・`lang=""`、改行を保つ）・
    終了予定（表示のロケールとタイムゾーンで `Intl` で整形）・ステータスページへのリンク（`ExternalLink`）を、サーバーが設定したものだけ出し、最後に「再読み込み」のボタンを置く。
  - `Retry-After` の時刻に `GET /instance` でサーバーに聞き直す（時刻が理由なので Effect のタイマー）。まだメンテナンス中なら新しい Problem の時刻でまた待ち、応答がないときや時刻が過ぎているときは 30 秒待つ。
    「再読み込み」はすぐに聞き直し、まだメンテナンス中ならそのことを `useAnnouncer` で知らせる。
  - メンテナンス中と知った後に API が 2xx を返したら、メンテナンスが終わったので、ページを 1 度だけ再読み込みする。セッションは Cookie から再開し、同じアドレスの画面と、その loader と Query が最初から読み込まれる。
  - キャッシュ済みの Service Worker が古いシェルを出しても、API の応答で同じ表示になる。
  `src/lib/api/client.test.ts` がトークンとセッションが残ること（要求・401 の後の更新・起動時の再開）と終わった後の再読み込みを、`src/maintenance.test.tsx` が表示・フォーカス・再読み込みのボタン・`Retry-After` の後の聞き直しを MSW で検査し、
  `e2e/maintenance.spec.ts` が表示の axe・横スクロールと、終わった後にサインインしたまま元の画面に戻ることを検査する。
- **サービスの終了の予告とクローズ**（`features/closure`、`lib/closure.ts`、[ADR-0088](../../adr/0088-service-shutdown-modes.md) §2・§4）: `GET /instance` の `closure` を読む。
  - 予告中（`state: announced`）は、`AppLayout` が `ConnectionBanner` の下に閉じられない帯（`@my/ui` の `NoticeBanner`、名前付きの `region`、ライブリージョンにはしない）を全画面に置く。サインインの画面・サーバーの情報のページ・管理画面の外枠にも出る。
    閉鎖日・クローズ後に持ち出せる期限・移転できること・運営者のメッセージ（`dir="auto"`・`lang=""`）と、データのエクスポートの画面（`/settings/account`）へのリンクを示す。短い画面では固定せず、ページと一緒に流れる。
  - クローズ後（`closed` / `delivery_completed`）、サインインしていない訪問者には、閉鎖したことと持ち出しの期限を同じ帯で示す。サインインした利用者（管理画面を除く）には、`AppLayout` がアプリの代わりに閉鎖の画面（`ClosedScreen`、`StandalonePage`）を出す。
    閉鎖日・期限・運営者のメッセージ、アーカイブの要求とダウンロード（設定の `DataExportsSection`）、CSV のダウンロード（`CsvExportsSection`）、サインアウトだけを置く。client API が断るほかの操作への導線は置かない。
  - ページが `403 service-closed` で失敗したときのエラーの見出しは「このサーバーは閉鎖されました」で、理由はサーバーの `detail` が示す（`errorKind` の `closed`）。
  - 移転の設定の画面（ADR-0089）が無い間、帯は移転できることを文で示すだけで、画面へのリンクは置かない。
  `e2e/service-closure.spec.ts` が予告の帯（サインインとサーバーの情報、キーボードでの到達）とクローズの画面の axe・横スクロールを検査する。
