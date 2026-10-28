# ADR-0118: デモ環境の Web Push: 自前の fake のプッシュサービスと CDP での配達

- Status: Proposed
- Date: 2026-10-28
- Deciders: @rosylilly
- Issue: #1720

## Context（背景・課題）

ADR-0117 のデモ環境（筋書きのシード、試験用のビルドタグ `e2e_fakes`、Mailpit、外部のサービスなし。L4 と `web:demo` が使う）は、Web Push（FR-206、FR-210、ADR-0109）を**設定しない**（ADR-0117 §3 の表）。
決定者は、デモ環境で Web Push を模すことを検討するよう求めた。今は次の理由で、Web Push を本物のサーバーとブラウザで通して確かめる場がない。

- ブラウザの購読の宛先（endpoint）は、ブラウザの事業者のプッシュサービス（Chromium なら FCM）である。CI の headless のブラウザはそこに届かず、`PushManager.subscribe` は失敗する（要検証: headless の Chromium での失敗の仕方）。
  ADR-0117 の判断基準 3（外部のサービスと秘密を要さない）にも反する。
- サーバーの送信（`internal/adapter/webpush`: RFC 8030 の POST、RFC 8291 の暗号化、RFC 8292 の VAPID）は SSRF ガード付きの `kernel.HTTPClient`（ADR-0019 §1）を通り、ループバックとホスト名 `localhost` を拒む。
  ガードを外す `http_allow_private_networks` はあるが、すべての用途の外向きの HTTP を開く。
- 購読の endpoint は領域の規則で `https` に限る（`notification.ErrInvalidPushEndpoint`）。
- Web の側は、購読の作成（`usePushNotifications`）が単体テストで `PushManager` を差し替えて確かめるだけで、Service Worker の `push` の処理（`app/web/sw/push.ts`）と通知の表示は本物のブラウザで通っていない。

## Decision Drivers（判断基準）

1. **本番を弱めないこと。** SSRF ガードの例外と fake は、本番の binary と `profile = production` に決して効かない（ADR-0117 の判断基準 1 と同じ強さ）。
2. 本番と同じ経路を通すこと。サーバーの暗号化と VAPID の署名、送信のジョブ、失効（404・410）の扱い、Service Worker の `push` の処理を、本物のコードで通す。
3. 外部のサービスと秘密を要さないこと（ADR-0117 の判断基準 3）。nightly・手元・fork で同じに動く。
4. 依存と保守が小さいこと。新しい言語の実行環境やサービスを増やさない。
5. L4 と E2E の両方で使えること（L4 のブラウザは Playwright MCP が動かす。ADR-0114）。

## Considered Options（選択肢）

1. **A. デモ環境では Web Push を出さない**（今の ADR-0117 §3 のまま）。
2. **B. 第三者の模擬サービス**: [marc1706/web-push-testing](https://github.com/marc1706/web-push-testing) を動かす。
   （[GoogleChrome/web-push-testing-service](https://github.com/GoogleChromeLabs/web-push-testing-service) は 2020-09 から更新がなく、アーカイブ済み。本物のブラウザを Selenium で動かす作りで、対象外とする。2026-10-28 に GitHub の API で確認）
3. **C. 自前の fake のプッシュサービス**を `e2e_fakes` のビルドに置き、ブラウザの側は init script で購読を差し替え、配達は CDP で行う（推奨案）。

ブラウザの側（B・C に共通）:

- **B1. init script で `PushManager` を差し替える**（E2E と L4 のブラウザだけ）。
- **B2. ブラウザのフラグで宛先を変える**: Chromium の GCM の宛先を変えるスイッチは GCM・MCS のプロトコル自体を模す必要があり（要検証）、Firefox の `dom.push.serverURL` は autopush の WebSocket のプロトコルを模す必要がある。L4 は Chromium で動く。

SSRF ガード（B・C に共通）:

- **G1. 送信（`HTTPPurposeWebPush`）だけの許可リスト**を足し、`profile = development` でなければ起動を拒む。
- **G2. 既存の `http_allow_private_networks = true`** をデモ環境で使う。

## Decision（決定）

> Proposed の間は推奨案。決めるのは決定者。

**推奨案: C・B1・G1。** ADR-0117 §3 の表の「Web Push」の行を本 ADR が置き換える（Accepted になるとき ADR-0117 の Status に注記する）。

### 1. fake のプッシュサービス（`e2e_fakes`）

- `e2e_fakes` のビルドの `my` にだけ入るサブコマンド（仮名 `my dev push-service`）として置く。単一の binary（overview §6）を保ち、ADR-0117 §2 の 4 つの確かめ（実行時・静的な検査・コンテナ・リリース）にそのまま入れる。
  実行時は、`profile = development` の設定でなければ起動を拒む。
- `localhost` の別のポートで **HTTPS** で待つ（endpoint を `https` に限る領域の規則を変えない）。証明書は Mailpit と同じ開発用の CA（`scripts/mailpit-cert.sh`）で作り、`my serve` は ADR-0117 §3 の `SSL_CERT_FILE` でそれを信頼する。
- 振る舞い（状態はメモリにだけ置く）:
  - **購読の発行**: `applicationServerKey` を受け、P-256 の鍵の対と 16 バイトの `auth` を作り、`PushSubscriptionJSON`（endpoint・`p256dh`・`auth`）を返す。
  - **受信**（RFC 8030 §5）: `TTL` と `Content-Encoding: aes128gcm` を要し、`Authorization: vapid t=…, k=…` の JWT を検証する（ES256 の署名、`k` が購読の `applicationServerKey` と一致、`aud` が fake のオリジン、`exp` が 24 時間以内の未来、`sub` がある。RFC 8292 §2・§3）。
    発行した鍵で RFC 8291 の本文を復号し、平文と `Urgency`・`Topic` を記録して `201` と `Location` を返す。検証に落ちれば `400`・`401`・`403`、上限を超えれば `413` を返す。
  - **失効**: 購読を失効させる操作を持ち、以後の受信に `410` を返す（サーバーの失効の扱いを確かめるため）。
  - **読み出し**: 購読ごとに受け取った平文と検証の結果を返す。
- 受信と検証のコードは `internal/adapter/webpush` の暗号の実装と別に書く（同じ誤りを両側で持たないため）。テストは RFC 8291 付録 A の例で両側を確かめる。

### 2. ブラウザの側（B1）

- E2E のフィクスチャと L4 の init page（`app/web/l4/mcp.ts`）が、Playwright の `addInitScript` で `PushManager.prototype.subscribe`・`getSubscription`・`permissionState` を差し替える。
  差し替えは `exposeBinding` で Node の側から fake に購読の発行を頼み、`PushSubscription` と同じ形（`endpoint`・`options.applicationServerKey`・`getKey`・`toJSON`・`unsubscribe`）を返す。
  ページから fake へ直に `fetch` しないので、画面の CSP（`connect-src 'self'`）を変えない。アプリのコードには何も足さない。
- 通知の許可は `browserContext.grantPermissions(["notifications"])` で先に与える。
- **配達**: Node の側が fake の受け取った平文を読み、Playwright の `CDPSession`（`browserContext.newCDPSession(page)`、Chromium だけ）で
  `ServiceWorker.enable` の `ServiceWorker.workerRegistrationUpdated` から `registrationId` を得て、`ServiceWorker.deliverPushMessage({ origin, registrationId, data })` を送る。
  Service Worker は本物の `push` の処理で通知を出し、確かめは `registration.getNotifications()` で行う。
  （2026-10-28 に `ChromeDevTools/devtools-protocol` の `browser_protocol.json` と playwright.dev の CDPSession で確認。ServiceWorker のドメインは experimental。headless の Chromium で `showNotification` と `getNotifications` が働くか、ページの target の CDPSession から ServiceWorker のドメインを使えるかは要検証）

### 3. SSRF ガード（G1）

- `httpclient.Options` に、Webhook（ADR-0060）・外部 IdP（ADR-0106 §5）と同じ形の送信先の許可リストを Web Push の送信（`HTTPPurposeWebPush`）だけに足す。設定の鍵は仮名 `push_allowed_destinations`（`webhook_allowed_destinations` と同じ書式）。
- 設定の検査は、`profile = development` でなければこの鍵を拒み、起動の時に警告のログを出す（`http_allow_private_networks` と同じ扱い）。ほかの用途（取得・配送・リンクのプレビュー・メディア）のガードは変わらない。
- デモ環境の設定（ADR-0117 §3）は、`vapid_private_key` をその場で作り（`my config vapid-key`）、許可リストに fake のオリジンだけを書く。

### 4. 確かめられるようになること

- **E2E**（ADR-0117 §4 の `demo` のサーバー）: 通知の設定で Web Push をオンにする → サーバーに購読ができる → 別の人物が API で返信する → fake が VAPID を検証して平文を受け取る → 配達して `getNotifications()` に返信の通知がある。
  種類をオフにした行の通知は届かないこと、fake が `410` を返すとサーバーが購読を消すことも確かめる。
- **L4**: 「返信だけ Web Push で受け取る」のような、Web Push を設定する作業を測れる（今は Web Push の列が出ないので測れない）。成否はサーバーの購読の `alerts` と、作業の後に起こした通知が fake に届いたかで判定する。
  **測れないこと**: エージェントは OS の通知もブラウザの許可の確認も見えない（スナップショットに出ない）。許可は先に与えるので許可の確認の分かりやすさは測らず、通知を押して開く作業（`notificationclick`）も L4 と E2E では通さない（CDP に通知を押す操作がない。要検証）。
  `getNotifications()` はハーネスの事後の確かめにだけ使う。
- 本物の FCM・Mozilla autopush・Apple の push service との相互運用は確かめない（それは本番と手での確認の範囲に残る）。

推奨を覆す条件: headless の Chromium で `deliverPushMessage` か `getNotifications` が働かない（要検証の結果）なら §2 の配達を外し、fake で受信と復号までを確かめる形に縮める。
Web Push の利用が v0 で小さいと決定者が判断するなら A を採る。

## Consequences（影響）

- 良い影響: サーバーの暗号化・VAPID・送信のジョブ・失効の扱いと、Service Worker の `push` の処理を、本物のコードで E2E と L4 から通せる。外部のサービスも秘密も要らない。
- 良い影響: 自前の fake が RFC 8291・8292 をこちらの実装と別に検証するので、相互運用の誤りの一部を CI で見つけられる。
- 悪い影響: fake と init script の差し替えは保守の対象で、差し替えた `PushManager` はブラウザの本物の購読の振る舞い（鍵の変更、ブラウザの側の失効）を再現しない。
- 悪い影響: 設定の鍵と `e2e_fakes` のコードが増える。§3 の許可リストは本番では使えないが、鍵の存在そのものは設定の説明（`docs/reference/configuration.md`）に載る。
- 悪い影響: 配達は Chromium の CDP の experimental なドメインに頼る。Playwright・Chromium の更新で壊れうる。
- 後続の作業（決定の後に Issue にする）:
  1. fake のプッシュサービス（`e2e_fakes` のサブコマンド、HTTPS、VAPID の検証、RFC 8291 の復号、失効と読み出し）と、その証明書の作成。
  2. SSRF ガードの Web Push の許可リストと、`profile = development` の検査、設定の説明。
  3. `web:demo` と `app/web/e2e/server.ts` の `demo` のサーバーで VAPID の鍵と fake を起動し、ADR-0117 の Status に本 ADR の注記を足す。
  4. E2E のフィクスチャ（init script・`exposeBinding`・CDP の配達）と Web Push の spec。
  5. L4 の init page に同じ差し替えと配達を入れ、Web Push を設定する L1 の作業を足す。

## Pros and Cons of the Options（詳細比較・任意）

- **A（出さない）**: 何も増えず、本番を弱める余地もない。Web Push は単体テストで暗号（RFC 8291 の例）と購読の作成を確かめている。
  ただし、送信のジョブから Service Worker の通知までを通す確かめがなく、Web Push の設定の画面（ADR-0109 の表の列）を L4 で測れない。
- **B（marc1706/web-push-testing）**: 購読の発行・VAPID の検証（`jsonwebtoken` で ES256）・`aes128gcm` と `aesgcm` の復号・失効・読み出しを既に持ち、自前のコードが要らない。
  ライセンスは MIT、最新のリリースは 1.2.2（2026-01-01）、最後の push は 2026-03-30（2026-10-28 に GitHub の API とソースで確認）。
  ただし Node（`>=20`）と npm の依存（express ほか）をデモの実行に足し（Bun で動くかは要検証）、HTTP だけで待つので、`https` に限る endpoint の規則を変えるか TLS の中継を前に置く必要がある。
  `aud` と `exp` の範囲の検査は `jsonwebtoken` の既定に任され、RFC 8292 の要求を満たすかは要検証。保守者は 1 人である。
- **B2（ブラウザのフラグ）**: 差し替えがなく、ブラウザの本物の購読の流れを通せる。しかし fake が GCM・MCS か autopush のプロトコルを話す必要があり、作るものが C よりずっと大きい。
- **G2（`http_allow_private_networks`）**: 設定の鍵を増やさず、すぐ使える。しかし取得・リンクのプレビュー・メディアまでループバックに届くようになり、デモ環境の SSRF の振る舞いが本番と大きく違う。必要な範囲より広い。

## Open Questions / 要検証

- headless の Chromium（Playwright の同梱版）で、`ServiceWorker.deliverPushMessage` が `push` の処理を起こし、`showNotification` と `getNotifications` が働くか。ページの target の CDPSession で ServiceWorker のドメインを使えるか。
- 許可リストを `profile = development` に加えて `e2e_fakes` のビルドにも限るか（推奨案は profile だけ。`http_allow_private_networks` と揃える）。
- L4 で Web Push の作業を何本足すか。
