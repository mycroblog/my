## 15. 検索とほかのサーバーのアカウントのフォロー

`features/search` が、ハンドルか URL でアカウントを探す画面（FR-305）と、全文検索があるサーバーで投稿を言葉で探すタブ（FR-309、[ADR-0149](../../adr/0149-full-text-search.md)）を受け持つ。フォローのボタンとフォローの一覧は `features/profile`（§13 の外のプロフィールの画面）にある。

- **画面**（`/search`、ナビゲーションの「検索」）: 見出しが名前を与える `role="search"` のフォームに、ラベル付きの入力欄（`type="search"`）と送信ボタンを置く。
  入力欄は `@username@domain`・`username@domain`・アクターかプロフィールの `https` の URL を受け付け、前後の空白を除いて `GET /accounts/lookup` に送る。
  サインイン中は `resolve=true` を付け、サーバーが知らないアカウントを相手のサーバーから取ってこさせる。サインインしていない間は付けず（解決にはトークンが要る）、このサーバーが知っているアカウントだけが見つかることを説明に書き、サインインのリンクを出す。
- **名前の検索**（FR-307、[screens/search.md](screens/search.md)）: サインイン中は、入力が 300 ms 止まるたびに（IME の変換中は待ち、`onCompositionEnd` から数える）、入力（先頭の `@` を除き、URL でないもの）を `GET /accounts/search`（`limit=20`）に送り、
  名前が合うアカウントを、フォームの下の区画（`h2`「アカウント（件数）」）に `features/profile` の `AccountFollowItem`（プロフィールへのリンクとフォローのボタン）で並べる。
  件数は視覚的に隠した `role="status"` で読み上げる。次の入力の結果が来るまで、前の結果を出したままにする。
- **投稿の検索**（FR-309、[screens/search.md](screens/search.md)）: `lib/fullTextSearch.ts` が `GET /instance` の `full_text_search` を読む。`usePostSearchAvailable`（全文検索があり、サインイン中）のときだけ、ルートが `SearchTabs` で「アカウント」「投稿」のタブ（`?type=posts`、S3）を出す。どちらでもないときは今までのアカウントの検索だけで、`?type=posts` は無視する。タブの有無で入力欄が作り直されないよう、ルートの loader がセッションの再開と `GET /instance` を待つ。検索そのものは loader に置かない。`defaultPreload: "intent"` の先読みが検索の回数の制限を使うからである。
  検索は送ったときだけ走る（R7）。検索語は `?q=` に置き、履歴を置き換える。IME の変換中は `onCompositionStart`・`onCompositionEnd` と `isComposing`・`keyCode` 229（Safari）で送信を止める。
  結果は `api/statusSearch.ts` の無限クエリ（`["get", "/statuses/search", { params: { query: { q } } }, "pages"]`）で、`staleTime: Infinity`・`retry: false` にする。書き込みが無効化の印を付けるか、利用者が同じ言葉をもう一度送るまで読み直さない。`isPostListQuery` に入れるので、ブロックやミュートで無効化される。
  ページは短くても空でも `next_cursor` がある間は続き、`Feed` が近づいた分を読む。描画は `features/timeline` の `StatusFeed`（`TimelineFeed` と同じ部品と `StatusCard`）で、ハイライトは受け取らない。最初のページの失敗は `RegionError`、後のページの失敗は読み込み済みの投稿の下に出す。429 は `lib/rateLimit.ts` が `Retry-After` の時刻に取り直すのでボタンを出さない。404 の `search-unavailable` は `refetchInstanceOnSearchUnavailable` が `GET /instance` を取り直し、タブが消える。
- **`indexable` の文言**: `client:settings.profile.indexable` とその説明は `useSearchScope`（`none`・`public`・`local`・`public_local`）を ICU の `select` に渡す。
- **送信**: 解決は相手のサーバーに届き、回数に制限がある（30 回 / 5 分）ので、画面を開いたときに走る query ではなく、利用者が送る mutation にする（R7）。
  見つかったアカウントをプロフィールの query のキャッシュに入れ、検索語を `?q=` にして今の履歴を置き換えてから、プロフィール（`/@{acct}`）に移る。戻ると、同じ検索語が入った画面に戻る（S3）。
  見つからない（404）・制限された（429）ときはクライアントのカタログの文言、それ以外はサーバーの理由を、入力欄のエラー（`aria-invalid` と `aria-describedby`）に出し、`useNotice` の `inPlace` で読み上げる。フォーカスは入力欄に残る。エラーは、それを起こした検索語のあいだだけ出し、入力を変えると消える。
- **フォローの承認待ち**: ほかのサーバーのアカウントのフォローは、相手のサーバーが `Accept` を返すまで `requested` のままになる（[federation.md](../federation.md)）。
  フォローのボタンは「リクエスト中」になり、その下に、承認を待っているサーバー（`Account.domain`）を示す文を出して、ボタンの説明（`aria-describedby`）にする。押すと、ローカルのリクエストと同じく取り消しを確かめる。
- **フォローの一覧**: ほかのサーバーのアカウントも、ローカルのアカウントと同じ行に `@username@domain` のハンドルで並び、そのプロフィール（`/@username@domain`）へリンクする。
- **テスト**: `features/search/search.test.tsx` が、キーボードでの検索からプロフィールへの移動、URL での検索、404・429・その他の失敗、サインインしていないときの検索、承認待ちのフォローと取り消し、フォローの一覧のリモートのアカウントを MSW で検査する。
  `features/search/statusSearch.test.tsx` が、全文検索のないサーバーとサインインしていない人の画面、送ったときだけの検索、IME の変換中、短いページと空のページの続き、空の説明、503・429・404 を MSW で検査する。
  E2E（`e2e/search.spec.ts`）は、キーボードだけで検索からフォローまでを通し、承認待ちの表示・axe・横スクロール・ARIA スナップショットを検査する。全文検索のあるサーバーは `search = fake` の要るテスト用のビルドだけにあるので、`e2e/demo-search.spec.ts` がデモのサーバー（nightly の `demo-e2e`）で、検索・空の説明・IME の変換・管理の設定を検査する。
