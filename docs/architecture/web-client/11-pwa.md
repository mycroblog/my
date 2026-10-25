## 11. PWA

Service Worker は `app/web/sw/sw.ts` にあり、`vite-plugin-pwa` の `injectManifest` が Workbox のモジュールと一緒に `dist/sw.js` にビルドする（ADR-0028）。
Web App Manifest と HTML は Go が返すので、プラグインはどちらも作らず、登録のスクリプトも差し込まない。

- **キャッシュの規則**（`sw/routing.ts` の `classify`。単体テストは `sw/routing.test.ts`）
  - **ビルドの成果物**: ハッシュ付きのアセット・アイコンを事前にキャッシュする。Vite の `index.html`（`vite preview` 用の代わりの外枠）とサーバーが描画する画面の CSS は含めない。
  - **汎用の外枠**: `/home` の外枠をビルドの版から作った revision で事前にキャッシュし、オフラインの外枠にする。外枠は訪問者ごとに変わらない（ADR-0028）。
    `/` の外枠はランディングページのリンクプレビューを持ち、ほかのアドレスに返せないので、オフラインの外枠にしない。
  - **遷移**: ネットワークだけに問い合わせる（3 秒でタイムアウト）。失敗かタイムアウトのときだけ、事前にキャッシュした汎用の外枠を返す。遷移の応答は保存しないので、ある URL の外枠（OGP・404・`noindex`）を別の URL に返すことはない。
  - **Service Worker が応えないパス**: `/api`・`/oauth`・`/auth`・`/.well-known`・`/nodeinfo`・`/ap`・`/media`・`/uploads`・`/theme`・`/licenses`（サーバーの予約パス）と `/manifest.webmanifest` への遷移。
    サーバーが描画する画面（§8）はネットワークが要り、SPA の外枠に置き換えてはいけない。
  - **クライアント API**: `/api` への要求には応えない（ネットワークだけになり、何もキャッシュしない）。`Authorization` 付きの要求はどのパスでもキャッシュしない。
  - **公開メディア**: 同じオリジンの `/media/` の画像だけを、キャッシュ優先（最大 300 件、30 日、200 の応答だけ）にする。動画は範囲要求の部分応答を再生できないので対象にしない。
  - **サーバーのテーマ**: 同じオリジンの `/theme/` のスタイルシートを、キャッシュ優先（最大 5 件、200 の応答だけ）にする。URL が版を含み内容が変わらないので、オフラインの外枠も最後に読んだテーマで表示する。
- **登録と更新**（`src/lib/pwa.ts`）: ビルドしたアプリだけが `/sw.js` をスコープ `/` で登録する。外枠の CSP が Trusted Types を要求するので、URL は `/sw.js` だけを通すポリシーで作る。
  - 最初の Service Worker は有効になるとすぐページを制御する（`clientsClaim`）。新しい版は待機し、`UpdatePrompt` が「新しい版があります」のトースト（`timeout: null`、「更新する」の操作付き）を出す。
    利用者が押したときだけ `SKIP_WAITING` を送り、新しい版がページを制御したら読み直す。ページが表示に戻るたびに更新を確かめる。
  - デプロイで古いチャンクが消えたとき（`vite:preloadError`）は読み直す。10 秒以内に繰り返したら読み直さず、エラー境界に任せる。
- **ログアウト**: セッションが終わると、`lib/auth/userData.ts` が `clearCaches()` を呼ぶ（§10）。事前のキャッシュも含めてすべてのキャッシュと、メディアの期限のメタデータを消す（ADR-0028）。
  消したあともオンラインでは同じように動き、新しい版の Service Worker が事前にキャッシュし直すまでオフラインの外枠は使えない。
- **オフラインで読むホームのタイムライン**（FR-604、`features/timeline` の `api/savedHome.ts`）: Service Worker は API に答えないので（ADR-0045）、API の応答はアプリが残す。
  サインイン中にホームを表示している間、Query のキャッシュのホームのタイムラインが読み込みか書き込みに成功するたびに、新しい投稿（40 件まで）と読み込んだ時刻を localStorage の `my:user:home-timeline`（利用者のデータ、§2）に書く。
  フォロワー限定（`private`）とダイレクト（`direct`）の投稿と、それをブーストしたものは端末に残さない（#640 の決定）。編集で公開範囲が狭まった投稿は、キャッシュの書き換えで書き直すときに除かれる。
  - ホームの loader は、タイムラインが `NetworkError` で読めないとき、Query のキャッシュにあればそれを、なければ残した投稿を読み込んだ時刻のままキャッシュに入れて表示する。
    オフラインで開いてセッションを再開できない（`unreachable`、§10）ときも、残した投稿があれば「サインイン」の代わりにそれを表示し、オンラインに戻るとルートを読み直してセッションを再開する。
    残した投稿がなければ「サインイン」を出す。サーバーがセッションはないと答えたら、残した投稿を消してサインインへ送る（§10「サインインが要る画面」）。
  - オフラインの間（`useIsOffline`、§9）、ホームはフィードの上に「オフラインです。〜時点の投稿を表示しています」を読み込んだ時刻とともに出す。オフラインになったことは外枠の帯が読み上げるので、これはライブリージョンにしない。
  - 外枠は Service Worker が事前にキャッシュしたものを使う。メディアは公開の画像のキャッシュにあれば表示される。
- **ステータスページの URL**（FR-810、[ADR-0085](../../adr/0085-uptime-monitoring-status-page.md) §6、`lib/statusPage.ts` の `useStatusPageUrl`）: サーバーが答えないときは `GET /instance` を読めないので、ホームのタイムラインと同じくアプリが残す。
  `GET /instance` が答えるたびに、その `status_page_url` を localStorage の `my:instance:status-page`（インスタンスの設定で、利用者のデータではない）に書き、無ければ消す。
  `GET /instance` が答えるまでと読めないときは残した URL を使う。オフラインの帯（§9）・情報ページ（`/about` の「サーバーの状態」）・設定のメニューの下のリンクがこれを読み、URL が無ければ出さない。
- **Web Push**（FR-206）: 設定の「表示と投稿」の「プッシュ通知」（`features/settings` の `usePushNotifications`）で、この端末で受け取るかと通知の種類を選ぶ。インスタンスが `push_server_key` を返さないときは区画を出さない。
  - 有効にするとき（スイッチの操作の中）だけ `Notification.requestPermission()` で許可を求め、Service Worker の登録の `PushManager.subscribe`（`userVisibleOnly`、`applicationServerKey` はサーバーの鍵）で購読し、`PUT /push/subscription` で種類と一緒に登録する。
    別の鍵で作った購読が残っていれば解除してから購読し直す。種類を変えると登録し直し、無効にすると `DELETE /push/subscription` とブラウザの購読の解除を行う。
  - Push API のないブラウザ（iOS / iPadOS ではホーム画面に追加していない Safari を含む）と、サイトの通知をブロックしたブラウザでは、スイッチの代わりに理由を出す。
  - Service Worker（`sw/push.ts`）は、サーバーが利用者の言語で書いた題を、通知の ID を `tag` にして表示する（同じ通知の再送は表示を置き換える）。押すと、開いているアプリの窓（フォーカスのある窓、なければどれか）をメッセージのパスに移し、窓がなければ新しく開く。
    パスは同じオリジンのものだけを開き、それ以外と読めないメッセージは通知の画面を開く。
- **`my serve` の配信**: `sw.js` は長期キャッシュせず（`no-cache`）、Service Worker 自身に効く CSP（`default-src 'none'; connect-src 'self'`）で返す。既定の `default-src 'none'` のままだと Service Worker の `fetch` が止まる。
- **インストール**: マニフェストのアイコンは `icon.svg`・192 / 512 px の PNG・512 px の maskable の PNG、外枠は iOS 用に `apple-touch-icon.png`（180 px）を指す。
  PNG は SVG から `scripts/web-icons.sh`（ImageMagick）で作り、コミットする。インストールの条件は E2E が Chrome の `Page.getInstallabilityErrors` で検査する。
