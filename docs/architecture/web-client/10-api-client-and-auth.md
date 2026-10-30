## 10. API クライアントと認証

API の呼び出しとトークンは `lib/api`・`lib/auth` が 1 か所で扱う（ADR-0029、ADR-0030）。機能はこれを作り直さない。
トークンの付与と 401 の後の更新・問題の詳細の解釈・ストリーミングの読み取りと再接続は、公開の API クライアントライブラリ `@mycroblog/mykit`（`pkg/mykit/ts`、[ADR-0093](../../adr/0093-api-client-library-mykit.md) §3）に任せ、
`app/web` には文言の整形（`client*` のメッセージ）・`lib/network` と `lib/maintenance` への報告・openapi-react-query との接続を残す。
`app/web` はワークスペースの依存（`workspace:*`）として `@mycroblog/mykit` のソースを読む（`tsconfig` の `customConditions` の `@mycroblog/source` と、同じファイルを指す Vite の `resolve.alias`）。Web クライアントのための変更は、ライブラリの公開の API の変更になる。

- **型**（S9）: `@mycroblog/mykit` の `components`（`import type { components } from "@mycroblog/mykit"`）を使う。openapi-typescript が、`api/spec/` をまとめた `api/openapi.yaml`（`scripts/task.sh api:generate` が作る）から `pkg/mykit/ts/src/schema.gen.ts` を生成し、コミットする。
  機能は API の型を自分の `features/<x>/api` から export して使う（`lib/api` と `@mycroblog/mykit` の値を import できるのは `features/*/api` と `lib/` だけ。ほかは `@mycroblog/mykit` の型だけを import できる。S1）。
- **クライアント**（`lib/api/client.ts`）: `@mycroblog/mykit` の `createClient` の openapi-fetch のクライアントと、その上の openapi-react-query のクライアントを 1 つずつ作り（`Api` の `fetch` と `query`）、同じオリジン・トークン・ロケールでストリームを開く `stream` を添える。
  機能は `features/<x>/api` で `api.query.queryOptions(...)` から `queryOptions` を作る。キーは `[method, path, params]` になる。
  書き込みは、`features/<x>/api` が `api.fetch` で作る `mutationKey` / `mutationFn` を TanStack Query の `useMutation` に渡す。
  openapi-react-query のフック（`api.query.useQuery` / `api.query.useMutation` など）は呼ばない（`restrict/syntax`）。オブジェクトから読むフックは描画ごとに同じ関数だと示せないので、React Compiler が呼んだフックごと最適化を諦め、`vite build` が失敗する（R4、[Don't dynamically use Hooks](https://react.dev/reference/rules/react-calls-components-and-hooks#dont-dynamically-use-hooks)）。
  ページを `next_cursor` でたどる一覧（タイムライン）は、openapi-react-query に無限クエリの `queryOptions` がないので、TanStack Query の `infiniteQueryOptions` で `api.fetch` を呼んで作る。
  キーは同じ形の末尾に `"pages"` を足したもの（`["get", "/timelines/local", "pages"]`）にし、同じ操作の普通のクエリと重ならないようにする（`features/timeline/api`）。
  - 表示のロケール（§9）を要求ごとに `Accept-Language` で送り（`mykit` の `acceptLanguage` に、要求ごとに読む関数を渡す）、サーバーが problem の `title` / `detail` をそのロケールで返すようにする。
  - セッション（`lib/auth/session.ts`）が `mykit` の `TokenSource`（`getAccessToken()` と `refresh(refused)`）になり、`mykit` がそのアクセストークンを `Authorization: Bearer` で付ける。最初の要求は、起動時のセッションの再開（`session.restore()`）を待ってから送る（`getAccessToken()` が待つ）。
  - トークンを付けた要求が 401 になったら、`mykit` がセッションの `refresh` でトークンを 1 度だけ更新し、要求の写しを新しいトークンで送り直す。2 度目の 401 はそのまま失敗にする。
  - `mykit` の 429・503 の再試行は切る（`retry: { disabled: true }`）。レート制限とメンテナンスは画面が出す（§9）。投稿の `Idempotency-Key` は投稿フォームが下書きごとに付け（§12）、`mykit` はそれを保つ。
  - `app/web` のミドルウェアが、失敗した応答を `lib/errors.ts` の `ApiError`（`mykit` の `problemFromResponse` が読んだ RFC 9457 の problem に `message` を足したもの）に、通信の失敗（`mykit` の `NetworkError`）は `lib/errors.ts` の `NetworkError` にして投げる。`mykit` の `ProblemError` より先に投げるので、呼び出し側が受け取るのは `ApiError` だけになる。
    どちらも `message` がローカライズ済みで、そのまま表示できる（`NetworkError` は、ブラウザがオフラインならそう言う文言になる。§9 のオフライン）。応答が届いたか届かなかったかは `lib/network.ts` に伝える。problem でない本文（プロキシの HTML など）と `title` のない problem は、クライアントのカタログの文言（`client:api.error.status`）にする。
    生成された型は失敗をすべて problem と呼ぶので、表示する文言は `errorMessage(error)` で取る。
  - メンテナンスの Problem（`503`、`type: /api/v1/problems/maintenance`、ADR-0084 §3）は `MaintenanceError`（`ApiError` の一種）にし、`lib/maintenance.ts` に知らせる。その後の 2xx の応答も知らせ、メンテナンスの終わりにする（§9 のメンテナンス）。
    この 503 は 401 ではないのでトークンを更新しない。401 の後の更新が OAuth の端点でこの 503 に当たったときも、トークンを捨てずに `MaintenanceError` を投げる。
- **セッション**（`lib/auth/session.ts`）: アクセストークンはこのオブジェクトのメモリにだけ置き、localStorage・sessionStorage・IndexedDB・読める Cookie には書かない。
  リフレッシュトークンは HttpOnly の `__Host-my_rt` Cookie にあり、`POST /oauth/token/refresh`（`X-My-Token-Refresh: 1`、`credentials: "include"`）だけがそれを読む。
  - サーバーはリフレッシュトークンを厳格にローテーションし、同じトークンが 2 度使われるとトークン系列ごと失効させる。そこで、更新はオリジンで 1 つの Web Lock（`navigator.locks.request("my:oauth-refresh", …)`）の中で行う。
    更新したタブは新しいアクセストークンを BroadcastChannel（`my:oauth-session`）で他のタブに送る。ロックを待っていたタブは、その間に届いたトークンを使う。
    届いていなければ自分で更新するが、前の更新でローテーションされた Cookie を送るので、再利用にはならない。同じタブの中の同時の更新は 1 つの要求を共有する。
  - 認可コードの交換も同じロックの中で行い、起動時の再開と競合しない。
  - サインイン・サインアウト・セッションの終了（更新が 4xx で断られた）も BroadcastChannel で他のタブに伝える。
  - 利用者が変わったとき（サインイン、またはセッションのあったタブでの終了）、`lib/auth/userData.ts` が Query のキャッシュをすべて消す。セッションが終わったときは、`lib/pwa` の `clearCaches()` で Cache Storage もすべて消す（§11）。
    セッションが終わったときは、`lib/storage` で利用者のデータと宣言したキー（投稿の下書き（§12）とオフラインで読むホームのタイムライン（§11））も消す（§2「ブラウザの保存」）。サインインでは下書きを消さない（キーにアカウントの ID を含める）。ホームのタイムラインは前の利用者のものなので、サインインでも消す。
  - 起動時のセッションの再開がサーバーに届かなかったとき（オフラインで開いた）と、更新の端点がメンテナンスの Problem を返したとき（`MaintenanceError`。`lib/maintenance.ts` に知らせる）は、サインインしていないものとして動きつつ、`unreachable` を立てる。
    どちらも Cookie を消さず、他のタブにサインアウトを伝えない。次の `restore()`（ルートの loader・`requireSignIn`）がもう一度更新を試す。
    再開できたら利用者が変わったものとして扱い（Query のキャッシュを消す）、サーバーがセッションはないと答えたら、セッションが終わったものとして利用者のデータを消す。
  - 状態（`unknown` / `signed-in` / `signed-out`）は `useSessionStatus()` で読む。サーバーのデータである閲覧者のアカウントは Context に置かず、`features/session` の `useViewer()` が Query で取る（S2、S6）。
- **ログイン**（`lib/auth/signIn.ts`、Authorization Code + PKCE）: `/login` は画面を描かず、開くとすぐ（`?return_to=` を戻り先にして）新しい verifier（32 バイトの乱数）と `state` を作り、S256 の challenge を付けてサーバーの認可画面（`/oauth/authorize`）にページごと遷移する。
  アプリのログインの入口（上のバー・ランディングのマストヘッド・サインインを求める画面のリンク・サインインが要る画面への遷移）は、どれも間に画面を挟まずにサーバーのログインの画面を開く。ランディングのマストヘッドは `useSignIn` で直接、ほかは `lib/paths.ts` の `signInPath` が作る `/login?return_to=…` のリンクで始める。
  ブックマークや古いリンクの `/login` も同じように始まる。アカウントの作成は、サーバーのログインの画面から進める。
  `return_to` はこのアプリのパス（`/` で始まり `//` や `/\` で始まらない、S3）だけを受け取り、ほかは捨てて `/` に戻る。
  scope は指定せず、Web クライアントに登録されたすべてのスコープを受け取る。
  - verifier・`state`・戻り先は、往復の間だけ sessionStorage（このタブだけ。`my:oauth-pending`）に置き、コールバックが読んだ時点で消す。どれも資格情報ではない（コードは 1 回限り・短命で、verifier に結び付く）。
  - コールバック（`/login/callback`、`routes/login_.callback.tsx`）は `beforeLoad` で `state` を照合し、コードを交換してから、戻り先に `replace` で遷移する。コードは履歴にもアドレスバーにも残らない。
    失敗（拒否・`state` の不一致・交換の失敗・通信の失敗）は、そのルートの `errorComponent` が理由とやり直しのリンクを出す。
  - 戻り先（`return_to`）は、1 つの `/` で始まるパスだけを受け付ける（`//example.com` や `javascript:` は捨てる）。
- **リアルタイム更新**（`lib/sse`、S10、FR-207）: レイアウトの `useLiveUpdates`（`routes/-layout`）が、サインイン中だけ `lib/sse` の `useEventStream` で `GET /streaming` を 1 本開く。
  `useEventStream` は `Api` の `stream`（`mykit` の `runStream`）を呼ぶ。`EventSource` はヘッダを送れないので、`mykit` が `fetch` の本文を HTML Standard の手順で読む（`id` と `retry` は使わず、`Last-Event-ID` を送らない）。
  トークンはセッションから取り、401 のあとに 1 度だけ更新する。更新がサーバーに届かないときとメンテナンスに当たったときは、通信の失敗として接続し直す。
  ストリームの `fetch` も、応答が届いたか（`lib/network.ts`）とメンテナンスの Problem（`lib/maintenance.ts`）を、ほかの要求と同じく知らせる。
  - ストリームが終わったら約 1 秒後に、失敗したら失敗のたびに倍の間隔（最大 1 分、後半の半分にばらす）で接続し直す。429（`too-many-streams`）と 503 の後は `Retry-After` に従う。60 秒（サーバーの 30 秒ごとのハートビートの 2 回分）何も届かなければ切って接続し直す。
    サーバーが要求をずっと断るとき（更新しても治らない 401、スコープの足りない 403 など、408・429 以外の 4xx）は接続し直さずにやめ、次にセッションが変わるまで開かない。セッションが終わるか外枠が消えると、Effect の後始末が要求を中止する。
  - `mykit` が知らないイベントを「未知のイベント」（`type: "unknown"`）として渡すので、`useEventStream` はそれを捨てる。
- **通話のルームのストリーム**（`features/voice/api/call.ts`、ADR-0101 §3）: 通話の間だけ、アプリ全体の通話（`features/voice` の `VoiceCallProvider`、§9）が、参加のセッションごとに `GET /voice/rooms/{id}/stream?session=` を 1 本開く。
  アプリのストリーム（S10）とは別の購読で、`Api` の `fetch`（openapi-fetch の `parseAs: "stream"`。トークンの付与と 401 のあとの更新は、ほかの要求と同じ）で本文を読み、`mykit` の `EventStreamParser` でイベントに分ける。
  45 秒（サーバーの 15 秒ごとのハートビートの 3 回分）何も届かなければ切る。終わるか失敗したら、同じセッションで 1 秒後に（失敗が続くと最大 30 秒まで間隔を延ばして）開き直し、最初の `state` で追いつく。
  `404` はセッションが利用者のものでなくなったこと（ハートビートが切れた）なので、参加し直して新しいセッションで開く。`state` は部屋の Query のキャッシュに書き、ほかのイベントは通話の状態（`features/voice/hooks/voiceCall.ts`）と音声の接続（`features/voice/rtc`）に渡す。
  - サーバーは何も再送しないので、2 本目以降のストリームが開いたら、ホームの最初のページを読み直して新着を数え、未読数を読み直し、通知の一覧を古いことにする（再取得はしない）。
  - `update` は、まず Query のキャッシュの別の項目（`features/timeline/api/newStatuses.ts`）に待たせる。表示中のホームがそれをどう出すかは、利用者の設定 `timeline_auto_insert`（`GET /preferences`、既定は真（[ADR-0127](../../adr/0127-public-timeline-streaming.md) の 6）。設定の「表示・投稿・タイムライン」の `Switch` で選び、変えたとたんに保存する）で決まる（Y7、[ADR-0077](../../adr/0077-timeline-auto-insert.md)）。
    - 偽のとき: ホームの `NewItemsButton` が、読み込み済みの投稿にないものの数を出し、押されたら最初のページの先頭に差し込んで見出しにフォーカスを移す。
    - 真のとき: 待たせた投稿をすぐ最初のページの先頭に差し込む。先頭にいる（フィードの先頭の目印が `IntersectionObserver` で見えている）ときはそのまま見せる。
      先頭から離れているときは、差し込む前に画面の上端にかかる投稿の位置を測り、`useLayoutEffect` で差の分だけスクロールを足して、その投稿を同じ場所に保つ（安定版の Safari が `overflow-anchor` に対応しないので、フィードは `overflow-anchor: none` にして自分で補う）。
      先頭から離れている間は、離れてから差し込んだ件数を `NewItemsButton`（`sticky`）で出し、押されたら先頭へ移って見出しにフォーカスを移す。自分で先頭へ戻ったら件数を 0 にする。差し込んだ投稿は動きなしで現れる。
    - 真でも、フォーカスか文字の選択がフィードの中にある間は一時停止し、偽のときと同じく待たせて `NewItemsButton` で知らせる。フォーカスと選択がフィードの外に出たら、待たせた投稿を差し込む（WCAG 2.2.2）。
    - どちらでも、差し込みはフォーカスを動かさず、差し込んだ投稿を 1 件ずつ読ませない。件数は `NewItemsButton` の `role="status"` だけで知らせ、先頭にいて一時停止していない間の差し込みは知らせない。
    ホームを表示していない（ホームのクエリを使う部品がない）ときは誰も読んでいないので、待たせずにキャッシュの最初のページの先頭に差し込む。ほかの画面からホームを開くときも、ルートの loader が待っている投稿を同じように差し込む。`staleTime` の間に開き直した、取り直さないホームも、最初の描画から新着を出す。
    待つ投稿が読み込み済みの投稿とつながらないとき（再接続で 1 ページ分の新着が見つかった、または 200 件を超えた）は差し込まず、押したときに最初のページだけを取り直す。
    設定が真で、先頭にいて一時停止していないときは、押されるのを待たずに最初のページだけを取り直す。
    タイムラインは、ウィンドウのフォーカスやオンライン復帰では取り直さない（取り直すと先頭に投稿が差し込まれる）。
  - `notification` は読み取り位置（`/notifications/marker`）の未読数を 1 つ増やし、通知の一覧を古いことにする。表示中の一覧は変わらない。
  - `announcement` はお知らせの一覧（`GET /announcements` のキャッシュ）の同じ ID のお知らせを置き換えるか、表示の始まりの順に加え、`announcement_removed` は一覧から外す。キャッシュがなければ何もしない（次に開いたときに読む）。
    過去を含む一覧（`GET /announcements/history` のキャッシュ）は、持っているお知らせなら置き換え、持っていない新しいお知らせと `announcement_removed`（取り下げか期間の変更かを区別できない）では無効化の印を付ける。
    再接続したら両方の一覧に無効化の印を付ける。
- **お知らせ**（`features/announcements`、FR-726、[ADR-0125](../../adr/0125-announcement-title-notification-list.md)）: サーバーが今見せているお知らせ（`GET /announcements`）を、`xl` 未満ではホームのタイムラインの見出しの下（このブラウザに残した投稿を出している間は出さない）に、`xl` 以上では右帯の最上段（§9）に出す。
  どちらも、読み込み中・読み込めないときは何も出さない（本体の画面が主で、お知らせは添え物）。右帯があるかはアプリの外枠だけが画面の幅から決め、ホームは外枠の文脈（`useHasAside`）で知る（D2）。
  - ホームの上: 未読があるときだけ、見出し（`h2`「お知らせ」）と未読の件数の `Badge` のある区画（`section`）に新しい順で出す。各お知らせは `article` で、見出し（`h3`）はタイトル（`dir="auto"`。タイトルのない古いお知らせは表示の始まりの日付）、その下に日付。未読のお知らせは見出しの中に「未読」の印を文字で出す（色だけで示さない、WCAG 1.4.1）。区画の終わりに「すべてのお知らせ」（`/announcements`）。
  - 本文は管理者の Markdown をサーバーが HTML にしてサニタイズしたもので、`PostContent`（A8）で出す。掲載の終わりがあれば本文の下に出す。
  - 未読のお知らせは「既読にする」（`POST /announcements/{id}/read`、説明は見出し）を持つ。押すと応答でキャッシュを書き換え、ボタンが消えたらフォーカスをそのお知らせの見出しに移す（`useFocusHeadingAfterLift`）。未読がなくなると区画ごと消え、フォーカスは画面の見出しに移る。失敗は区画の中に `role="alert"` で出す。
  - 右帯: `AsidePanel`（見出しの隣に未読の数）に、未読を先に、その中は新しい順で最初の 3 件のタイトルを、一覧のページのそのお知らせ（`/announcements?id=`）へのリンクとして出す。「未読」の印はリンクの名前に含める。本文と「既読にする」は出さない。
  - 一覧のページ（`/announcements`、[screens/announcements.md](screens/announcements.md)）: `GET /announcements/history` をページごとに読み、「さらに表示」で続きを足す。`id` を付けて開くと（通知・Web Push・右帯から）、そのお知らせのあるページまで読み、見出しにフォーカスを移し（外枠が画面の見出しに移した後のフレームで）、未読で期間の中なら既読にする。どのページにもなければ取り下げられたことを一覧の上に言う。
  - ナビゲーションの「お知らせ」は、今見えている未読の数をバッジで出す（名前は「お知らせ、未読 2 件」）。
- **ログアウト**（設定画面の「ログアウト」）: アクセストークンを `POST /oauth/revoke` で失効させ（サーバーはトークン系列ごと失効させ、Cookie を消し、その系列を認可した認可サーバーの画面のセッションも終わらせる）、メモリのトークンを消して他のタブに伝え、ランディングページ（`/`）を開く（`/login` はすぐログインを始めるので開かない。アカウントの削除と、このブラウザのセッションの取り消しの後も同じ）。
  サーバーに届かなかったときも、このタブはログアウトし、セッションが残り得ることをトーストで知らせる。
  画面のセッションが終わるので、続けて「ログイン」を押すとパスワードを聞かれる（共用端末で次の人がそのままログインできない）。
- **サインインが要る画面**: ルートの `beforeLoad` に `lib/auth/requireSignIn.ts` の `requireSignIn` を置く。セッションの再開を待ち、サインインしていなければ、戻り先をその画面にして `/login` に送る（TanStack Router の Authenticated Routes）。
  画面を守るのは API で、これはサインインを先に促すだけである。サインインしていない閲覧者に、その画面の中で「サインイン」を出す作りにはしない。
  - サインインが要る画面: ホーム（`/home`）・通知（`/notifications`）・自分のプロフィール（`/profile`。サインインしていれば `/@{acct}` へ移る）・ブックマーク（`/bookmarks`）・設定（`/settings` とその下のすべて）・モデレーション（`/moderation` とその下）・管理（`/admin` とその下）。
    ホームだけは、オフラインで開いてセッションを再開できない（`unreachable`）ときにサインインへ送らずに開き、このブラウザに残した投稿を出す（§11）。サーバーがセッションはないと答えたら、残した投稿を消してサインインへ送る。
  - セッションなしで見られる画面: ランディング（`/`。サインインしていれば `/home` へ移る）・インスタンスの情報（`/about`）・ローカルと連合のタイムライン（`/local`・`/public`。サーバーが提供するとき）・ハッシュタグのタイムライン（`/tags/{tag}`）・検索（`/search`。このサーバーが知っているアカウントだけ、§15）・プロフィールとフォロー / フォロワーの一覧（`/@{acct}`）・投稿の詳細と編集履歴。ログイン（`/login`）は画面ではなく、すぐサーバーのログインの画面へ移る。
    これらの画面では、投稿への操作・返信・新しい投稿のボタンなど、セッションが要る操作を出さない（§12、§13）。サインインしていない閲覧者のナビゲーションは §9 の「サインインしていないときのナビゲーション」にする。
- **テスト**: `src/test/msw/http.ts` が OAuth のエンドポイント（既定ではリフレッシュ Cookie がない状態）の既定のハンドラを持ち、API の操作の既定のハンドラ（Bearer が必要）は機能ごとに置く（§7）。
  `renderApp` は main.tsx と同じくセッションを再開し、ページの外への遷移（認可画面）を `leftTo` に記録する（jsdom は別の文書を読み込めない）。
  `GET /streaming` の既定のハンドラは、何も送らずにすぐ終わるストリームを返す（MSW は Node.js の `fetch` をソケットで横取りするので、テストの終わりに中止された流れ続ける応答があると、後のテストの要求が本物のネットワークに出ることがある）。イベントを送るテストは `streamingServer()` の `current().send()` / `close()` を使う。
  jsdom には Web Locks がないので、タブ間の直列化のテストは `src/test/locks.ts` の代わりの実装を 2 つの `Session` に共有させる。BroadcastChannel は Node.js のものを使う。
