## 8. サーバーが描画する画面

ログイン（二要素認証を使う利用者に認証コードを求める 2 段目の画面 `/auth/sign_in/two_factor` を含む）・登録・メール確認・パスワードリセット（リンクの要求 `/auth/forgot_password` と、リンクが開く新しいパスワードの設定 `/auth/reset_password`）・通知のメールの配信停止（`/mail/unsubscribe`。`GET` は確認だけ、`POST` は RFC 8058 のワンクリックで、ログインも CSRF のトークンも要らない、ADR-0086 §4）・アカウントの状態・OAuth の認可同意の画面は、Go の `html/template`（`internal/interface/authpages`）が描画し、JavaScript なしで動く（ADR-0030）。CAPTCHA を有効にしたときだけ、ログイン・登録・パスワードリセットのリンクの要求の画面はサービスのウィジェットのスクリプトを読み、そのページの CSP にサービスのオリジンを足す（ADR-0042）。ログインの画面は、Web クライアントのビルドが作るスクリプト（`src/server-pages/passkey-sign-in.ts`、CSP は `script-src 'self'`）を読み、パスキーに対応したブラウザでだけ「パスキーでログイン」のボタンを出す（FR-004）。ボタンのフォームは JavaScript がないと隠れたままで、メールアドレスとパスワードのフォームはスクリプトなしで動く。

- スタイルは Vite のビルドの別のエントリ `app/web/src/server-pages.css` にある。Tailwind と `@my/ui/theme.css` を読み込み、テンプレートが使う少数のクラス（`page`・`card`・`field`・`hint`・`field-error`・`alert`・`notice`・`button` など）を
  トークンのユーティリティの `@apply` で定義する。テンプレートからクラスを集めないので、Tailwind はソースを走査しない（`source(none)`）。
- `my serve` はビルドのマニフェストのキー `src/server-pages.css` からファイル名を引き（`web.Handler.Stylesheet`）、画面の `<link rel="stylesheet">` にする。SPA の外枠はこの CSS を読み込まない。
  サーバーのテーマ（§9 の「サーバーのテーマ」）があれば、その後にテーマのスタイルシートも読む（`authpages.Options.ThemeStylesheet`）。この CSS はユーティリティを持たないので、テーマのトークンは `@apply` で作った規則と `theme.css` の基本の規則を通して効く。
  Vite の開発サーバーを使うときはビルドがないので、画面にスタイルは付かない。
- CSP は `style-src 'self'` で、インラインのスタイルとスクリプトを置かない。
- ダークモードは `prefers-color-scheme` に従う（トークンの `light-dark()` と `<meta name="color-scheme" content="light dark">`）。フォーカスのリングは `theme.css` の `:focus-visible`、
  タップ領域は `pointer-coarse:` で 44 px 以上、方向は論理プロパティ（`px-`・`ps-`・`pe-`）で書く（D4・D7・Y4）。
- フォームは `<label for>`、`autocomplete`（`username`・`current-password`・`new-password`・`one-time-code`）、ヒントとエラーの `aria-describedby`、エラーの `aria-invalid` を持ち、ブラウザの制約検証（`required`・`minlength`）を使う。
- 利用規約・プライバシーポリシー・特定商取引法に基づく表記（#489）の本文は、サーバーが描画せず、SPA のルート `/about/terms`・`/about/privacy`・`/about/commerce` が描く。
  本文はクライアント API の匿名の `GET /instance/documents/{kind}` が返す（管理者が Markdown で書いた文書を、サーバーが HTML にしてサニタイズしたもの、`internal/adapter/markdown`）。`GET /instance` の `terms_url` / `privacy_policy_url` / `commerce_disclosure_url` はこの 3 つのパスを指す。
  3 つとも `routes/about_.<名前>.tsx` の情報ページ（`InfoLayout`、`routes/-layout/DocumentPage.tsx`）で、サインインしていなくても読める。`h1` は文書の名前、本文は `PostContent`（`lang=""`・`dir="auto"`）で出し、`h2` に `id` がなければクライアントが連番の `id` を足して目次（`h2[id]`）にする。本文の前に最終改定日（`revised_at`）を出す。loader は `GET /instance/documents/{kind}` を Query のキャッシュ経由で読み（S7）、公開していない文書（404）は見つからない画面になる。サーバーはこの 3 つのアドレスに、公開している文書なら文書の名前を題名にしたインスタンスのリンクプレビューの外枠を、公開していない文書なら汎用の外枠を返す（ADR-0028）。アプリの中のこの 3 つへのリンクは、`GET /instance` の絶対 URL のパスだけを取り出して同じタブの SPA の移動にする（`features/documents` の `DocumentLink`。Y12 の外へ出るリンクではない）。
  文書の画面は JavaScript を前提にする（SPA のほかの画面と同じ）。サーバーが公開している文書は、すべてのサーバー描画の画面の `main` の後のフッター（`nav`「規約とポリシー」）と、登録のフォームの作成のボタンの前の同意の文（ボタンの `aria-describedby`）からこの SPA のパスへリンクする。公開していない文書はリンクしない。
- サーバーが描画するページのリンクは、ページの間では同じタブで開く（フッターの文書のリンクも）。入力の途中で読むリンク、つまり登録のフォームの同意の文の文書と、ボットの確認の説明の後のサービスのプライバシーポリシーと利用規約（ほかのサイト）は、
  入力を失わないように `target="_blank" rel="noopener noreferrer"` で新しいタブで開き、末尾に `.visually-hidden` の「（新しいタブで開きます）」（`server:auth.link.new_tab`）を置く（Y12）。
