# ADR-0015: テスト戦略とデモ環境

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #14, #453, #1674, #1712, #1720

## Decision（決定）

### 1. テストのレベル

検査の場所は ADR-0079 が決める。「本検査」は Merge Queue の `check`、「遅い検査」はリリースの関門・nightly・手動で回す `slow-suite.yml` である。

| レベル | 対象 | 実行環境 | 実行タイミング |
| --- | --- | --- | --- |
| Unit | domain / application（フェイクのポート使用） | ローカル・CI | 本検査 |
| Contract | 各ポートの全アダプタ（同一スイート） | SQLite in-memory / pgmem（§3）/ PG（コンテナ）/ Garage（S3 互換。`pkg/testkit/s3test`）/ Floci（SNS・SQS。ADR-0050 §5）/ Cloud Tasks エミュレータ | 本検査は SQLite・pgmem（§3）・メモリの偽物で回す。コンテナの契約（`postgres-contract`・`stream-contract`・`blob-contract`・`voice-contract`・`mail-integration`）は遅い検査。クラウド実体が必要なものは nightly |
| Build matrix | `CGO_ENABLED=0` で linux/amd64・linux/arm64 をビルド、ビルドタグでアダプタを除外してもビルドできる | CI | `CGO_ENABLED=0` のビルドは本検査。ビルドタグの組み合わせと arm64 のイメージは遅い検査 |
| API conformance | OpenAPI スキーマとサーバー実装・生成クライアントの一致 | CI | 本検査 |
| Federation fixtures | 実サーバー由来の JSON を入力にした受信・生成の回帰テスト | CI | 本検査 |
| Federation live | docker compose で Mastodon / GoToSocial 等と実際に連合 | CI | 遅い検査 |
| E2E (web) | Playwright。`en`, `ja`, 疑似ロケール `en-XA`, `ar-XB` | CI | 遅い検査（全体）。部分集合は手動。PR と main への push では回さない |
| Deploy smoke | 各ターゲットへデプロイしてヘルスチェック + 投稿 | 実クラウド | nightly / リリース前 |

### 2. 検査の原則

- `scripts/task.sh check` = lint + 型検査 + Unit + Contract（ローカル完結分）+ i18n 検査 + docs 検査。外部ネットワーク不要。
- テストは決定的に: `Clock` / `IDGenerator` / `HTTPClient` を注入し、ネットワークはフィクスチャで置き換える。
- カバレッジは指標としてのみ使い、閾値で CI を落とさない（domain 層は高く保つ）。

### 3. PostgreSQL のテストは pgmem で動かす

- PostgreSQL を使うテストは、テスト用の共通のヘルパー `pkg/testkit/pgtest` でサーバーとデータベースを得る
  - サーバーはテストのバイナリごとに 1 つで、最初にデータベースを求められたときに起動する
  - テストごとのデータベースは、スキーマを当てたテンプレートの複製（`CREATE DATABASE ... TEMPLATE`）で作る
  - サーバーは環境変数で選ぶ。`TEST_POSTGRES_URL` を指定するとその実サーバー、指定しなければ `TEST_POSTGRES` が `pgmem`（既定）なら pgmem、`container` なら testcontainers-go のコンテナ
- 本検査の契約テスト（`scripts/task.sh go:test`、ジョブ `server-test`）は pgmem で回し、Docker を前提にしない
- コンテナの PostgreSQL での契約テスト（`scripts/task.sh go:test-postgres`、ジョブ `postgres-contract`）は、遅い検査で回す
  - 同じテストを、`TEST_POSTGRES=container` で流す
  - ADR-0003 の契約テストの PostgreSQL の頻度は、この節が決める
- スキーマの検査（`scripts/task.sh db:check`）は、本物の PostgreSQL と Atlas で本検査に回す
- pgmem のテストごとの fork と `Reset` は使わない
  - 時間はテンプレートの複製とほとんど変わらない（CI の `go test ./...` で 52 秒と 59 秒）
  - それなのにメモリは 1 桁増える
- pgmem は、公開から 3 日を過ぎた `v1.18.x` に `go.mod` で固定する。PostgreSQL のメジャーは、本番で使う `postgres:18` に合わせる
- pgmem はテストのヘルパーだけが使い、本番のバイナリ（`cmd/my`）にはリンクしない

### 4. L4 の課題テストは nightly で回す

- **PR の側**: UI を変える PR は、変えた画面の L1 の作業ごとに最短の手順 R を書き、4 つの観点で変更を見直す。L4 は測らない。
- **nightly の側**: nightly の workflow に、L4 のジョブを置く。
  - 対象は、前回の成功した実行から、L1 の文書か、その画面の `context-map.yaml` の範囲が変わった画面の作業である。1 晩に測る作業の数に上限を設ける（実装で決め、workflow のコメントに書く）。
  - `demo` のサーバー（§8）で `my serve` を起動し、エージェント（Claude Code を非対話で動かす）が、Playwright の ARIA スナップショットだけを手がかりに作業を行う。
    スクリーンショットは渡さない。結果は作業ごとの手数・戻った回数・lostness を JSON の成果物として残す。
  - lostness が 0.4 を超えた作業は、画面ごとに 1 つの issue（同じ題名があれば追記）にまとめる。issue には作業・たどった道筋・L1 の最短の手順を書く。
  - Claude API への認証は ADR-0079 の Workload Identity Federation で行い、API キーを置かない。ジョブは `main` だけがデプロイできる environment `ux-nightly` で動く。
- lostness の式としきい値 0.4 は ADR-0110 §7 が決める。
- 見直す条件:
  - nightly の L4 の issue が、同じ画面で何晩も続けて出る（PR の時点で拾うべき問題が多い）場合。単独の作業の PR だけ、手元で L4 を回す形を足す。
  - 費用が、上限を設けても運用の目標に合わない場合。対象を、L1 が変わった画面だけに絞る。

### 5. 筋書きのシード

- 筋書き（scenario）をリポジトリに置く。`app/web/demo/` に、データだけの `scenario.ts`、それを適用する `seed.ts`、画像の `assets/` を置く。
- シードは `bin/my` のコマンドと、`@mycroblog/mykit` の client / admin の API だけでデータを作る。**SQL は書かない。** 筋書きの項目を作るコマンドか API がなければ、その項目のためにコマンドか API を足す（足せなければ項目を落とす）。
  通知・タイムライン・投影は、フォロー・返信・リアクション・メンションなどの操作の結果として生まれ、直に入れない。
- 筋書きの中身:
  - 人物: 管理者・モデレーター・一般の利用者を数人。言語は日本語と英語を混ぜ、右から左の文字の投稿を 1 つ含める（`dir="auto"` を確かめるため。ADR-0009）。
  - 投稿: 画像の付いた投稿、返信の連なり（スレッド）、公開範囲の異なる投稿、リアクション、ブースト。
  - 関係: フォロー（承認の要る鍵のアカウントへのフォローの申請を含む）、それによる通知。
  - モデレーション: 未処理の通報 1 つ、ルール。
  - ロールとバッジ: 管理者が作ったロール、それを表に出すバッジ。
  - サポーター: 有効にしたサポーターの制度、プラン（月と年の価格）、§6 の fake で申し込んだサポーター 1 人、費用の項目（管理の API で手で入れる）。
  - 招待（ADR-0106）、お知らせ。
- **決定的**: 同じ筋書きは同じ順で同じ中身を作る。ID と時刻はサーバーが決めるので、画面の確かめは ID と時刻に依存しない。
- **冪等**: 作る要求はそれぞれ、筋書きの項目の名前から作った `Idempotency-Key`（`demo:<項目>`）を付け、人物は利用者名で引く。同じ DB に 2 回流しても増えない。
- 画像は、このリポジトリのために作ったもの（またはシードがコードで描くもの）だけを使い、ライセンスはリポジトリと同じにする。第三者の写真を置かない。
- 投稿の中の URL は `.test` か `.invalid` のホストだけにし、リンクのプレビューも外へ出ない（§7）。
- 人物のパスワードは筋書きに固定で書く。§6 の実行時の確かめにより、この筋書きを入れられるのは `profile = development` のサーバーだけである。

### 6. 試験用のビルドタグ `e2e_fakes` と fake の決済

- **タグの意味**: `no_*` が adapter を外すのに対し、`e2e_fakes` は試験のための fake を**足す**。既定のビルド（タグなし）にも、`no_*` だけのビルドにも fake は入らない。
  後で fake を足すときも同じタグとこの節の確かめに入れる。
- **組み立て**: `internal/platform/adapters_payments_fake.go`（`//go:build e2e_fakes`）だけが `internal/adapter/payment/fake` を import し、`payments = fake` の adapter に登録する。
  設定の側（`internal/platform/config`）も、`fake` をこのタグのビルドでだけ選択肢に加える。タグのないビルドで `payments = fake` を設定すると、ほかの外したタグと同じく起動の前の検査で拒む。
- **申し込みの画面**: fake は ADR-0074 §9 の `PageServer` を実装し、PAY.JP と同じく自分のオリジンの `/payments/pages/fake/` に短命のページを出す。
  ページはプランと金額、「試験用のビルドの決済で、お金は動かない」の表示、「支払う」「失敗させる」「取り消す」の操作を持つ。「支払う」は fake の側に加入と確定した支払い（`Final`）を記録して `SuccessURL` へ送る。
  `Mode()` は `test` を返すので、画面は ADR-0074 §5 のテストのモードの表示を出す。事業者の側の状態はプロセスのメモリにだけある（§8 の、起動のたびに作り直す環境の前提）。
- **本番に入れない確かめ**。支払い済みと記録できる fake が本番の binary に入れば、誰でもお金を払わずにサポーターのロールを得られる。どれか一つが破れても残りが止める:
  1. **実行時**: `payments = fake` は `profile = development` でなければ起動を拒む。起動のときに、試験用のビルドであることを警告のログに出す。
  2. **静的な検査**: `scripts/go-build-tags.sh`（`scripts/task.sh go:build-tags`、`go:check` の一部）は、タグのない `cmd/my` と代表の組み合わせのどれもが `internal/adapter/payment/fake` に依存しないことを確かめる。
  3. **コンテナのイメージ**: `Dockerfile` は `GO_TAGS` に `e2e_fakes` があればビルドを失敗させる。イメージは試験用のビルドを作らない。
  4. **リリース**: 公開する binary とイメージを作るジョブは、成果物の Go のビルド情報（`go version -m` の `build -tags=`。Go 1.27.3 で確かめた）に `e2e_fakes` がないことを確かめるスクリプトを通す。
- ADR-0074 §8 の「fake は本番の binary に入らない」は保たれる。変わるのは、試験用のビルドに限って fake を組み立てられることだけである。

### 7. 外部のサービスを使わない

デモ環境の設定は次に固定し、秘密を要さない（鍵 `kek` はその場で作る）。

| 項目 | 値 |
| --- | --- |
| `profile` | `development` |
| `database`・`blob` | `sqlite`（起動のたびに新しいファイル）・`fs` |
| `mail` | `smtp` で Mailpit へ（`mail_smtp_host = localhost`、`mail_smtp_port = 1025`、下の「メール」） |
| `federation_mode`（インスタンスの設定） | `off`（ADR-0006。外へ配送も取得もしない） |
| `voice` | `p2p`（STUN・TURN なし。同じマシンのブラウザどうしで繋がる） |
| `payments` | `fake`（§6） |
| Web Push | `vapid_private_key` をその場で作り（`my config vapid-key`）、`push_allowed_destinations` に fake のオリジンだけを書く（§9、§11） |
| CAPTCHA・外部 IdP・エラーと計測の送り先 | 設定しない（無効） |

**メール**:

- Mailpit は、開発の構成と同じ固定のイメージ（`deploy/docker/mailpit/compose.yaml` のタグとダイジェスト）で、同じ設定（`scripts/mailpit-cert.sh` の開発用の CA の証明書で STARTTLS を必須にする）で動かす。
  `mail = smtp` は STARTTLS と証明書の検証を省かないので、`my serve` は `SSL_CERT_FILE` でその CA を信頼する。`SSL_CERT_FILE` はシステムのルート証明書を置き換え、サーバーは外の HTTPS に繋がらなくなるが、デモ環境は外へ出ないので（`federation_mode = off`）かえって都合がよい。
- これで、メールのリンクを開く作業（パスワードの再設定、メールアドレスの確認、メールで送る招待）を、手元でも L4 でも利用者と同じ道筋で通せる。
- **L4 のエージェントは、Mailpit の Web の画面（`http://127.0.0.1:8025`）を別のタブで開いてメールを読む**。利用者がメールのアプリを開いてリンクを押すのと同じ形で、こちらのメールの文面とリンクの分かりやすさも作業に含めて確かめられるからである。
  ハーネスが REST API でリンクを取って渡す形は採らない。メールに気づいて開く段とメールの文面を飛ばし、作業の一部を測らなくなるからである。
  Mailpit の画面の中の手数は Mailpit の作りに左右されるので、ハーネスは Mailpit のオリジンでの手数を別に記録し、作業の手数と lostness から除く（L1 の最短の手順では「メールを開いてリンクを押す」を 1 手と数える）。

### 8. 入口

- **手元**: `scripts/task.sh web:demo` が、`scripts/task.sh docker:mailpit` で Mailpit を起動し（すでに動いていればそのまま使う。Docker が要る）、`GO_TAGS=e2e_fakes` で `bin/my` を作り、`.tmp/demo/` に新しい DB を作ってスキーマを当て、`my serve` を起動してシードを流し、URL と人物のログインを表示する。
  起動のたびに DB を作り直す（fake の状態はメモリにだけあり、残した DB と食い違うため）。決定者の手での確認はこれを使う。
- **L4**: `app/web/e2e/server.ts` に、`voice` と同じく別の DB とポートの `demo` のサーバーを足し、L4（`app/web/l4/run.ts`）はそれを起動する。
  作業は筋書きの人物でログインして始め、シードで測れるようになった作業の `unavailable` を外す。nightly の L4 のジョブも `e2e_fakes` で `bin/my` を作り、Mailpit を同じイメージのダイジェストで動かす。
  Mailpit は証明書を作ってから起動しなければならないので、ジョブの `services:` ではなく、`scripts/mailpit-cert.sh` の後のステップで `deploy/docker/mailpit/compose.yaml` から起動する（`services:` のコンテナはチェックアウトの前に起動し、証明書を読めない）。
- **E2E**: 既存の spec は、最小のフィクスチャの `main` のサーバーのままにする（前提を変えない）。本物のデータが要る新しい spec は `demo` のサーバーを使える。E2E と L4 の binary は CI の中だけで使い、公開しない。
- **画面のストーリー**（ADR-0079）: MSW のフィクスチャは、人物・投稿の文・画像を `scenario.ts` から読んでよい（できるところから）。サーバーの応答を録って生成したフィクスチャはコミットしない（ADR-0081）。

### 9. fake のプッシュサービス（`e2e_fakes`）

- `e2e_fakes` のビルドの `my` にだけ入るサブコマンド（`my dev push-service`）として置く。単一の binary（overview §6）を保ち、§6 の 4 つの確かめ（実行時・静的な検査・コンテナ・リリース）にそのまま入れる。
  実行時は、`profile = development` の設定でなければ起動を拒む。
- `localhost` の別のポートで **HTTPS** で待つ（endpoint を `https` に限る領域の規則を変えない）。証明書は Mailpit と同じ開発用の CA（`scripts/mailpit-cert.sh`）で作り、`my serve` は §7 の `SSL_CERT_FILE` でそれを信頼する。
- 振る舞い（状態はメモリにだけ置く）:
  - **購読の発行**: `applicationServerKey` を受け、P-256 の鍵の対と 16 バイトの `auth` を作り、`PushSubscriptionJSON`（endpoint・`p256dh`・`auth`）を返す。
  - **受信**（RFC 8030 §5）: `TTL` と `Content-Encoding: aes128gcm` を要し、`Authorization: vapid t=…, k=…` の JWT を検証する（ES256 の署名、`k` が購読の `applicationServerKey` と一致、`aud` が fake のオリジン、`exp` が 24 時間以内の未来、`sub` がある。RFC 8292 §2・§3）。
    発行した鍵で RFC 8291 の本文を復号し、平文と `Urgency`・`Topic` を記録して `201` と `Location` を返す。検証に落ちれば `400`・`401`・`403`、上限を超えれば `413` を返す。
  - **失効**: 購読を失効させる操作を持ち、以後の受信に `410` を返す（サーバーの失効の扱いを確かめるため）。
  - **読み出し**: 購読ごとに受け取った平文と検証の結果を返す。
- 受信と検証のコードは `internal/adapter/webpush` の暗号の実装と別に書く（同じ誤りを両側で持たないため）。テストは RFC 8291 付録 A の例で両側を確かめる。

### 10. Web Push のブラウザの側

- E2E のフィクスチャと L4 の init page（`app/web/l4/mcp.ts`）が、Playwright の `addInitScript` で `PushManager.prototype.subscribe`・`getSubscription`・`permissionState` を差し替える。
  差し替えは `exposeBinding` で Node の側から fake に購読の発行を頼み、`PushSubscription` と同じ形（`endpoint`・`options.applicationServerKey`・`getKey`・`toJSON`・`unsubscribe`）を返す。
  ページから fake へ直に `fetch` しないので、画面の CSP（`connect-src 'self'`）を変えない。アプリのコードには何も足さない。
- 通知の許可は `browserContext.grantPermissions(["notifications"])` で先に与える。
- **配達**: Node の側が fake の受け取った平文を読み、Playwright の `CDPSession`（`browserContext.newCDPSession(page)`、Chromium だけ）で
  `ServiceWorker.enable` の `ServiceWorker.workerRegistrationUpdated` から `registrationId` を得て、`ServiceWorker.deliverPushMessage({ origin, registrationId, data })` を送る。
  Service Worker は本物の `push` の処理で通知を出し、確かめは `registration.getNotifications()` で行う。
  （2026-10-28 に `ChromeDevTools/devtools-protocol` の `browser_protocol.json` と playwright.dev の CDPSession で確認。ServiceWorker のドメインは experimental）

### 11. Web Push の SSRF ガード

- `httpclient.Options` に、Webhook（ADR-0060）・外部 IdP（ADR-0106 §5）と同じ形の送信先の許可リストを Web Push の送信（`HTTPPurposeWebPush`）だけに足す。設定の鍵は `push_allowed_destinations`（`webhook_allowed_destinations` と同じ書式）。
- 設定の検査は、`profile = development` でなければこの鍵を拒み、起動の時に警告のログを出す（`http_allow_private_networks` と同じ扱い）。
  ビルドタグでは限らない（`e2e_fakes` でないビルドでも、開発用のプロファイルなら使える）。鍵は fake を足すのではなく、送信先を開くだけだからである。ほかの用途（取得・配送・リンクのプレビュー・メディア）のガードは変わらない。
- デモ環境の設定（§7）は、`vapid_private_key` をその場で作り（`my config vapid-key`）、許可リストに fake のオリジンだけを書く。

### 12. Web Push で確かめられること

- **E2E**（§8 の `demo` のサーバー）: 通知の設定で Web Push をオンにする → サーバーに購読ができる → 別の人物が API で返信する → fake が VAPID を検証して平文を受け取る → 配達して `getNotifications()` に返信の通知がある。
  種類をオフにした行の通知は届かないこと、fake が `410` を返すとサーバーが購読を消すことも確かめる。
- **L4**: Web Push を設定する作業を 1 つ足す。作業は「この端末で Web Push をオンにする」で、
  成否は、サーバーにその人物の購読があることと、作業の後にハーネスが別の人物の返信を起こし、その通知を fake が受け取って復号できたことで判定する。
  **測れないこと**: エージェントは OS の通知もブラウザの許可の確認も見えない（スナップショットに出ない）。許可は先に与えるので許可の確認の分かりやすさは測らず、通知を押して開く作業（`notificationclick`）も L4 と E2E では通さない（CDP に通知を押す操作がない。2026-10-29 に `browser_protocol.json` で確認）。`notificationclick` は `app/web/sw/push.test.ts` の単体テストで確かめる。
  `getNotifications()` はハーネスの事後の確かめにだけ使う。
- 本物の FCM・Mozilla autopush・Apple の push service との相互運用は確かめない（それは本番と手での確認の範囲に残る）。

### 13. headless の Chromium での配達

ページの target の CDPSession から ServiceWorker のドメインを使え、`ServiceWorker.deliverPushMessage` が `push` の処理を起こし、`showNotification` と `getNotifications` が働くことを、固定した Playwright のイメージの Chromium の新しい headless で確かめた（`app/web/e2e/web-push.ts`）。
働かなくなれば §10 の配達を外し、fake が受け取って VAPID を検証し復号できたところまでを確かめる（L4 の判定はもともと fake の受信で行うので変わらない。E2E は fake の受信と復号を確かめるところで止める）。

### 14. Web Push の見直す条件

- Chromium か Playwright の更新で CDP の配達が使えなくなり、代わりの方法がない場合（§13 の、fake の受信と復号までの形に縮める）。
- 本物のブラウザの購読の振る舞い（鍵の変更・ブラウザの側の失効）の退行が、差し替えのせいで見逃される場合（ブラウザのフラグで宛先を変える形か、本物の push service での手での確認を足す）。
- Web Push の利用が小さく、fake と差し替えの保守が見合わない場合（デモ環境では Web Push を出さない形に戻す）。

## 経緯

エージェントが自律的に作業するには、変更の正しさを機械的に判定できる必要がある（NFR-DEV-1, 2）。アダプタの差し替え可能性（NFR-PORT-3）と連合互換性（G4）も継続的に保証するため、§1 と §2 を決めた。頻度は、E2E を main への push だけに移し（ADR-0079）、検査を PR・Merge Queue・main・nightly に振り分け（ADR-0079）、Merge Queue に一本化した（ADR-0079）順に変わった。

コンテナの PostgreSQL は `go test` の大半の時間を準備に使い、並列のエージェントでは reaper の起動失敗やタイムアウトで落ちた。spike #453 で pgmem（MIT、PGlite のフォークを WebAssembly にし wasm2go で Go に変換したもの）を試すと、4 つのパッケージの全テストが変更なしで通り（スキーマ、`FOR UPDATE` / `SKIP LOCKED`、advisory lock、READ COMMITTED、デッドロック検出 40P01 を含む）、CI の `go test ./...` は 129 秒から 52〜59 秒になった。

ADR-0110 §7 は L4 を PR の前に行うと決めていたが、並列の作業（[workflow §9](../agents/workflow.md#9-parallel-batches)）では PR が同じ領域を変え続け、最初の並列の作業（#1643）では L4 を測れなかった。測る対象が main に実際にある画面であること、並列の作業の速さを落とさないこと、費用に上限があることから、§4 にした。API キーは ADR-0079 で Workload Identity Federation に替わった。

本物のサーバーを動かして画面を確かめる場面が増えたが、サーバーのデータがほとんどなく、L4 の多くの作業が測れなかった。v0 の公開の前の決定者の手での確認も、人が使っている途中のサーバーを要る。そこで §5 から §8 を決めた。基準は、支払い済みと記録できる fake が本番の binary に決して入らないこと、データが領域の規則（ADR-0020）を通ること、外部のサービスと秘密を要さないこと、決定的で冪等であること、本物の `my serve` を測ることである。メールは決定者の問いを受けて、ログへの出力から Mailpit に替えた。

デモ環境は当初 Web Push を設定しなかったが、決定者の求めで検討した。ブラウザの購読の宛先（FCM）に CI の headless のブラウザは届かず、サーバーの送信は SSRF ガードがループバックを拒み、endpoint は領域の規則（`notification.ErrInvalidPushEndpoint`）で `https` に限り、Service Worker の `push` の処理は本物のブラウザで通っていなかった。FR-206、FR-210、ADR-0086 を本番を弱めず本番と同じ経路で通すため、§9 から §14 を決めた。決定者はプッシュサービスを自前の偽物、SSRF ガードの例外を開発用プロファイルに限るもの、fake の置き場所を `e2e_fakes` のサブコマンドとした。

## 採らなかった案

- PostgreSQL をコンテナのまま: 本物そのものだが、129 秒かかり Docker を要し、起動の失敗で落ちる。
- pgmem のスナップショットからテストごとに fork（`Reset` を含む）: CI で 52〜56 秒と少し速いが、1 プロセスのピークのメモリが 9 GB を超える（`GOMEMLIMIT=4GiB` で 4.4 GB、約 2 割遅くなる）。
- L4 を PR の前に手元で測る: 問題をマージの前に拾え作者が直せるが、並列の作業では PR ごとに起動と実行が要り、main にない状態を測る。
- 決まった手順を E2E に書く: 決定的で費用がなく PR で回せるが、決まった道筋をなぞるだけで lostness を測れない。手順の数の回帰の検査としては L3 に足せる。
- シードを SQL やダンプで入れる: 速く ID と時刻まで固定できるが、イベント・投影・現在値の整合をシードが手で保ち、スキーマの変更で壊れる。
- 試験用のビルドだけの `my demo seed`: 速く型で守られるが、API の検証と認可を通らず、API の欠けを見つける機会を失う。
- fake を常に binary に入れ `profile` で止める: ビルドの種類が増えないが、設定の誤り一つで支払いの偽装に届く。
- デモ用の別の binary: 最も強い隔離だが、単一の binary の構成（overview §6）から外れ、コマンドとフラグを二重に保ち、L4 が本物の `my serve` を測らなくなる。
- Stripe のテストモード・stripe-mock: 本物の事業者の画面を通せるが、鍵とネットワークが要り fork と手元で動かない。stripe-mock は申し込みの画面を持たない。
- `mail = none`: Docker が要らず最も軽いが、リンクはログにしか出ず、利用者の道筋でメールを開く作業を測れない。
- デモ環境で Web Push を出さない: 何も増えないが、送信のジョブから Service Worker の通知までを通す確かめがなく、Web Push の設定の画面を L4 で測れない。
- [marc1706/web-push-testing](https://github.com/marc1706/web-push-testing): 必要な機能を持ち MIT で自前のコードが要らないが、Node と npm の依存を足し、HTTP だけで待ち、`aud` と `exp` の検査は要検証で、保守者は 1 人（2026-10-28 に GitHub の API とソースで確認）。[web-push-testing-service](https://github.com/GoogleChromeLabs/web-push-testing-service) はアーカイブ済み。
- ブラウザのフラグで宛先を変える: 差し替えなしで本物の購読の流れを通せるが、GCM・MCS か autopush のプロトコルを話す必要があり（要検証）、作るものがずっと大きい。
- `http_allow_private_networks = true` をデモ環境で使う: 鍵を増やさずすぐ使えるが、取得・リンクのプレビュー・メディアまでループバックに届き、必要な範囲より広い。

## Consequences（影響）

- コントラクトテストのおかげで、利用者が自作したアダプタも同じ基準で検証できる（`testkit` を公開パッケージにする）。
- nightly ジョブ用のクラウドアカウント・シークレット管理が必要（M6）。
- pgmem で Go のテストが約 2 倍速くなり、Docker のない環境でも PostgreSQL のテストが飛ばされず、reaper とコンテナの起動による失敗がなくなる。
- テストのバイナリが 36 MB から 88 MB に増え、キャッシュのないビルドが 20〜40 秒延びる。公開から間もない、保守者が 1 人の依存を持つ。PGlite のフォークと上流の PostgreSQL の差は、遅い検査のコンテナのジョブまで見つからない。
- L4 で並列の作業の速さが落ちず、測るのは main の実際の画面で、迷いやすさが決定者の批評（L5）の前に issue として並ぶ。問題が見つかるのはマージの翌日で、直すのは別の PR になる。
- nightly に LLM の API の費用が増える。エージェントの振る舞いはぶれるので、1 晩の結果だけで判断せず、続けて出る問題を重く見る。
- L4 がデータのある画面の作業とメールのリンクを開く作業を測れ、決定者は 1 つのコマンドで使われている途中のサーバーを手元で見られる。外部のサービスも秘密も要らないので、fork や新しい貢献者の手元でも同じ環境が立つ。
- シードが client / admin の API と `@mycroblog/mykit` を通るので、API とライブラリの使い方の確かめにもなる。筋書きは API の変更に合わせて保守が要り、壊れれば nightly の L4 で翌日に分かる。
- ビルドの種類が増え、試験用のビルドと公開するビルドを取り違える余地が生まれる。§6 の 4 つの確かめで防ぐが、確かめのスクリプトとタグの名前は保守の対象になる。
- fake の申し込みの画面は Stripe・PAY.JP の画面と違うので、事業者の画面の中の迷いやすさは L4 では測れない。
- `web:demo` と L4 のジョブは Mailpit のために Docker を要る。Mailpit のイメージの更新は、開発の構成・統合テストと同じダイジェストを揃えて行う。
- サーバーの暗号化・VAPID・送信のジョブ・失効の扱いと Service Worker の `push` の処理を、本物のコードで E2E と L4 から通せる。自前の fake が RFC 8291・8292 をこちらの実装と別に検証するので、相互運用の誤りの一部を CI で見つけられる。
- fake と init script の差し替えは保守の対象で、差し替えた `PushManager` はブラウザの本物の購読の振る舞い（鍵の変更、ブラウザの側の失効）を再現しない。
- 設定の鍵と `e2e_fakes` のコードが増える。§11 の許可リストは本番では使えないが、鍵の存在そのものは設定の説明（`docs/reference/configuration.md`）に載る。
- 配達は Chromium の CDP の experimental なドメインに頼る。Playwright・Chromium の更新で壊れうる。

## Open Questions / 要検証

- pgmem の保守が止まったとき、どこまでフォークして持つか、コンテナに戻すか
