# ADR-0099: 決済の PAY.JP adapter（API v1 の定期課金）

- Status: Accepted
- Date: 2026-10-25
- Deciders: @rosylilly
- Issue: #1301

## Context（背景・課題）

M4.2（Supporters、Epic #925）の課金は、[ADR-0074](0074-payment-provider-port.md) のポート `PaymentProvider` と `stripe` adapter で動いている（FR-910・FR-911・FR-917）。
ADR-0074 は「2 つめの事業者が要るときは、同じ契約テストを通す adapter を足し、`payments_provider` の値と各表の `provider` の enum に足す」と決めている。
日本の運営者が国内の決済代行で課金できるように、2 つめの事業者として PAY.JP を足す。1 つのサーバーで有効な事業者は 1 つで、切り替えるときは既存の加入者に再加入してもらう（ADR-0074、#925 の回答 9）。

PAY.JP には API v1 と v2 がある。v2 は定期課金の API をまだ持たない（v1 は「提供あり」、v2 は「未対応 ※ 2026年対応予定」。[v2 のドキュメント](https://docs.pay.jp/v2/llms-full.txt)の「API v1 からの移行について」、2026-10-25 に確認）。

決定者の指示（#1301 の作業の依頼、原文）:

> 定期課金が v2 にないなら v1 での対応をお願いします

決定者の回答（PR #1303、原文）:

> 1. 推奨案で OK
> 2. 推奨案で OK
> 3. 費用記録でよい
> 4. よい。セキュリティと PCIDSS に注意
> 5. その方針でよい
>
> ADR Accepted

PR #1303 の確認事項は次のとおり。1 = 上げる変更は U1。2 = 手数料は M1。3 = PAY.JP の有料プランの利用料と振込の手数料は収入から引かず、運営者が費用として記録する。
4 = カードの入力のページは adapter がこのサーバーのオリジンで出す（F1）。5 = チャージバックの取り消しは自動で記録せず、運営者が `finance` で記録する。

事業者の事実は、2026-10-25 に [PAY.JP API v1 リファレンス](https://pay.jp/docs/api/)（`docs.pay.jp/v1/api/`）と v2 のドキュメントで確かめた。アカウントの作成と課金のある操作はしていない。Stripe との違いのうち、この決定に効くものは次のとおり。

| 項目 | Stripe（ADR-0074） | PAY.JP API v1 |
| --- | --- | --- |
| 決済の画面 | ホスト型の Checkout（`mode=subscription`） | **ホスト型の画面がない**。カードは payjp.js（PAY.JP の iframe）でブラウザがトークンにし、サーバーが顧客にカードを登録して定期課金を作る |
| 顧客ポータル | Customer Portal（支払い方法・請求書・解約） | **ない** |
| 3D セキュア | Checkout が行う | カードの登録には「トークン作成時の 3D セキュア」を勧めている。payjp.js の `createToken(element, {three_d_secure: true, card: {name, email}})` を iframe 型（`threeDSecureWorkflow: 'iframe'`）で呼ぶと、ページの上の iframe で認証し、認証の済んだトークンが返る。カードの名義と、メールか電話番号が要る（payjp.js は入力欄を出さない）。テストのモードでは全てのテストカードで 3D セキュアを試せる |
| 価格 | Product と金額を変えられない Price、`lookup_key` | Plan（`amount` 50〜9,999,999、`currency` は `jpy` だけ、`interval` は `month` か `year`）。ID を指定して作れ、更新できるのは `name` と `metadata` だけ |
| 状態 | `incomplete`・`active`・`past_due`・`unpaid`・`paused`・`canceled` など | `trial`・`active`・`canceled`（**期間の終わりの解約の予約**。終わるまで `/resume` で取り消せる）・`paused`（更新の課金の失敗で停止し、再開の要求まで課金しない。**再試行しない**。再開の要求がその場で課金する）。期間が終わった解約と削除したものは取得できなくなる。同じ顧客は同じ Plan を 2 つ購読できない |
| 作成 | 支払いが済まなければ `incomplete` | 作成のときに最初の課金をし、失敗すれば作成も失敗する（前払い） |
| プランの変更 | 品目の差し替え、Subscription Schedule | `plan` を変えると新しいプランの全額をすぐ課金する（`prorate=true` で旧プランの残りを返金）。`trial_end` に未来の時刻を渡すと即時の課金をしない。`next_cycle_plan` で次の更新からプランを変え、空にすると取り消す |
| 即時の解約 | `DELETE`（`prorate=false`） | `DELETE /v1/subscriptions/:id`（`prorate` の既定は false） |
| webhook の検証 | `Stripe-Signature` の HMAC と時刻 | **固定のトークン** `X-Payjp-Webhook-Token`（アカウント・モードごと）を比べるだけ。時刻も本文の署名もない。本文に `livemode` がある |
| webhook の再送 | ライブで最大 3 日 | v2 のドキュメントでは 3 分おきに最大 3 回（v1 も同じかは **要検証**） |
| イベントの一覧 | `GET /v1/events` は 30 日まで | `GET /v1/events`（`since`・`until`・`type`・`resource_id`・`object`、`limit` 100 と `offset`、新しい順）。保持期間は書かれていない（**要検証**） |
| お金の動き | balance transaction（`amount`・`fee`・`net`） | charge（`amount`・`amount_refunded`・`fee_rate`（％の文字列）・`subscription`・`term_id`）。**返金のオブジェクトはなく**、charge の `amount_refunded` が増え、`charge.refunded` のイベントが出る。手数料の額は区間（Term）ごとの取引明細（Statement）の項目（`fee`・`refund_fee_offset`・`chargeback`・`chargeback_fee_offset` など）にだけ出る。チャージバックは `dispute.created` のイベントだけで、API のオブジェクトがない |
| 冪等キー | `Idempotency-Key` | v1 のリファレンスに記載がない（v2 にはある） |
| レート制限 | ライブで読み取り 1 秒 100 件ほど | 秘密鍵の要求（支払い以外）がライブで 1 秒 30 件、テストで **1 秒 2 件**。支払いの作成・確定・返金はライブで 14 件、テストで 2 件。超えると `429`（`over_capacity`） |
| 一覧 | カーソル | `limit`（1〜100）と `offset`。新しい順 |
| 通貨 | 多数 | `jpy` だけ |
| 日付 | アカウントのタイムゾーン | 入金日などは日本時間（JST） |
| SDK | `stripe-go`（MIT） | `payjp-go`。リファレンスは `beta` ブランチを指す |
| 鍵 | `sk_`・`rk_`、`whsec_` | `sk_test_`・`sk_live_`（秘密鍵、Basic 認証のユーザー名）、`pk_test_`・`pk_live_`（公開鍵、payjp.js と 3D セキュアの開始に使う）、`whook_`（webhook のトークン） |

決めること: v1 の API をポートにどう写すか、v1 にない決済の画面と顧客ポータルの代わり、webhook の検証、お金の動きと手数料の写し、照合、設定と秘密、テストのしかた、build tag、ポートと契約テストに要る小さな変更。

## Decision Drivers（判断基準）

1. ADR-0072 の加入の状態とロールの規則、ADR-0074 のポートの形と経路（取り直して導く・照合）を変えずに、PAY.JP でも同じ結果になること（FR-911）
2. カード情報がこのサーバーを通らず、運営者の PCI の範囲が SAQ A 相当に収まること。Web クライアント（PWA）の CSP を緩めないこと（FR-910、ADR-0019 §9）
3. 偽の webhook でロールを得られないこと（ADR-0019）。固定トークンでも、受けた通知で状態を書かずに取り直すので、再生の攻撃で状態が変わらないこと
4. 3D セキュアの義務（日本の EC のクレジットカード決済）を満たすこと
5. 収入の額が事業者の記録と一致すること（FR-917、ADR-0073）
6. テストのモードのレート制限（1 秒 2 件）の内で照合とテストが回ること
7. 依存が増えず、`CGO_ENABLED=0` で build でき、使わないときは build tag で外せること（ADR-0010・ADR-0011）
8. 実装が小さいこと

## Considered Options（選択肢）

論点ごとに比べる。各論点の 1 つめを採用する。

- **API の版**: A1 v1 の定期課金 API（Plan・Subscription）/ A2 v2 の Setup Flow と Payment Flow で、こちらが毎期の課金を組む / A3 v2 の定期課金 API を待つ
- **カードの入力と顧客ポータルの代わり**: F1 adapter が自分の短命のページ（このサーバーのオリジン、署名した URL）を出し、payjp.js の iframe とトークン作成時の 3D セキュアを使う / F2 Web クライアント（SPA）に payjp.js を組み込む / F3 v1 の Checkout（`checkout.pay.jp` のボタンのスクリプト）
- **上げる変更（`now_prorated`）**: U1 差額を日割りで都度の課金にし、プランを `trial_end` = 今の期間の終わりで差し替えて期間を保つ / U2 PAY.JP の `prorate=true`（新しいプランの全額をすぐ課金し、旧プランの残りを返金する）
- **手数料**: M1 charge ごとに `fee_rate` から計算し、区間の取引明細との差を締めのときに `fee` の動きで補う / M2 取引明細の区間ごとの額だけを記録する
- **API の呼び出し**: R1 `net/http` で REST を直接呼ぶ / R2 `payjp-go`

## Decision（決定）

**A1・F1・U1・M1・R1 を採用する。** A1 は決定者の指示、F1・U1・M1 と §5 の費用の扱い・チャージバックの取り消しは決定者の回答（PR #1303）による。ポートの形は ADR-0074 §1 のまま使い、足すのは §1 の 2 つ（ページの受け口の任意のインターフェースと、契約テストの webhook の時刻の扱い）だけである。

### 1. ポートと契約テストに足すもの

- **ページの受け口**: `internal/app/supporters` に任意のインターフェースを足す。adapter が実装していれば、`internal/interface/payments` が `GET`・`POST /payments/pages/{provider}/...` をその `http.Handler` に渡す。
  `{provider}` が設定の事業者でないか、実装していなければ `404`。経路は webhook と同じく OpenAPI の client API に載せず、ADR-0019 §5 のレート制限の対象にする。

  ```go
  // PageServer is a provider whose checkout and portal pages our server serves, for a provider
  // without hosted ones (ADR-0099 §3). StartCheckout and PortalURL return URLs of these pages.
  type PageServer interface {
      // Pages handles the requests under PagesPath(provider).
      Pages() http.Handler
  }
  ```

- **契約テスト**: `PaymentUnderTest` に `TimedSignature bool` を足す。false の adapter（`payjp`）では、`ParseWebhook` の時刻の許容（`stale_signature`）と複数の署名・秘密の切り替えの項目を飛ばし、代わりに「トークンの誤り・欠け・テストとライブの食い違いを見分ける」ことを確かめる。
  決済の画面と顧客ポータルの URL の許可するホスト（`CheckoutHosts`）には、`SuccessURL`・`returnURL` のオリジンを渡す（§3）。ほかの契約の項目はそのまま通す。

### 2. `payjp` adapter（`internal/adapter/payment/payjp`）

- **呼び出し（R1）**: `net/http` で `https://api.pay.jp/v1` を呼ぶ（Basic 認証のユーザー名に秘密鍵、本文は `application/x-www-form-urlencoded`、応答は JSON）。使うのは十数の API で、`payjp-go` の `beta` ブランチを追わない。宛先は固定で、設定で変えられない（テストの `httptest` だけが変える）。
  外向きの HTTP は ADR-0074 §7 の H1 と同じく、`internal/platform` が渡すタイムアウトと計測の付いた `*http.Client` を使う。
- `Name()` は `payjp`。`Location()` は日本時間（`time.FixedZone`、UTC+9。日本に夏時間はない）で、PAY.JP の入金日などの日付の基準に合わせる（ADR-0073 §3）。`MinimumAmount("JPY")` は 50 で、ほかの通貨は false。
- **プラン**: Plan の ID を `my_<plan_id>_<interval>_<amount>` に決めて作る（`currency=jpy`、`metadata.my_plan_id`、`trial_days` と `billing_day` は使わない）。同じ ID があれば作らず、名前が違えば `name` を更新する。金額を変えると ID が変わるので新しい Plan になる。
  spec にない同じプランの Plan は、削除せずに `metadata.my_archived=1` を付けてアーカイブとする（削除が既存の定期課金に効くかは **要検証**、効かなくても削除しない）。`ProductRef` は Plan をまとめる名前 `my_<plan_id>` で、PAY.JP のオブジェクトはない。9,999,999 を越える額と JPY 以外は `invalid_request`。
- **スナップショット**（`GetSubscription`・`ListSubscriptions`）: `Price` は `plan.id`、`ScheduledPrice` は `next_cycle_plan.id`、期間は `current_period_start`・`current_period_end`、`LocalID` は `metadata.my_subscription_id`。状態は次のとおり写す。

  | PAY.JP | ポートの状態 | ほかの項目 |
  | --- | --- | --- |
  | `active`・`trial` | `active` | 試用は作らない。`trial` は §2 の上げる変更（U1）で期間を保つときにだけ現れる |
  | `canceled` | `active` | `CancelAtPeriodEnd = true`（PAY.JP の `canceled` は期間の終わりの解約の予約） |
  | `paused` | `past_due` | `LatestPaymentFailed = paused_at`。こちらは停止の命令を出さないので、停止は支払いの失敗による（ダッシュボードで止めたときも同じに扱う） |
  | 取得が `404` | `canceled` | 期間の終わった解約と削除は取得できなくなる。こちらが記録した ID だけを取り直すので、`404` は終わったサブスクリプションである |
  | 知らない値 | `unpaid` | 警告のログ（ADR-0074 §1） |

  PAY.JP は支払いに失敗した定期課金を再試行しない（[定期課金のガイド](https://docs.pay.jp/v1/subscription)、2026-10-25 に確認）。ADR-0072 §7 の猶予（7 日）の起点は `paused_at` で、その間に利用者が §3 のポータルでカードを直すと adapter が `/resume` で課金して `active` に戻る。直さなければ 7 日で `lapsed` になり、PAY.JP の側は停止のまま課金しない。
- **命令**（どれも呼ぶ前に定期課金を読み、もう望む状態なら呼ばない。v1 に冪等キーの記載がないので、これで二重の操作を防ぐ）:
  - `ChangePrice(now_prorated)`（U1）: 差額 = 切り捨て((新しい価格の 1 秒あたり − 今の価格の 1 秒あたり) × 今の期間の残りの秒数)。1 秒あたりは価格をその間隔の 1 期間（今の期間の始まりから 1 か月か 1 年）の秒数で割る。
    差額が 50 円以上なら、顧客に都度の課金（`POST /v1/charges`、`customer`・`amount`・`metadata` に `my_subscription_id`・`my_subscription`（PAY.JP の定期課金の ID）・`my_change`（呼び出し側の `key`）・`my_period_end`）をする。課金の前に、その顧客の直近 2 日の charge に同じ `my_change` があるかを確かめる。
    続けて定期課金の `plan` を新しいプランにし、`trial_end` を今の期間の終わりにする（即時の課金をさせず、次の課金は期間の終わりに新しいプランの全額）。差額の課金が断られたら、プランを変えずに `invalid_request` を返す（利用者に支払い方法を直すよう示す）。
    月払いから年払いへの同じ順位の変更は、年払いの 1 秒あたりが安いので差額がなく、年払いは今の期間の終わりから始まる。
  - `ChangePrice(period_end)`: `next_cycle_plan` を新しいプランにする。`CancelScheduledChange` は `next_cycle_plan` を空にする。
  - `CancelAtPeriodEnd`: `POST /v1/subscriptions/:id/cancel`。`ResumeCancellation`: 状態が `canceled` で期間の終わりの前なら `POST /v1/subscriptions/:id/resume`（期間の内なので課金しない）。
  - `CancelNow`: `DELETE /v1/subscriptions/:id`（`prorate=false`）。`404` は済んだものとする。
- **失敗の種類**（ADR-0074 §1）: `401` → `permission`、`404` → `not_found`、`400`・`402` → `invalid_request`、`429` → `throttled`、`5xx` とネットワーク → `unavailable`。
- **要求の速さ**: ライブで 1 秒 20 件、テストで 1 秒 2 件まで（PAY.JP の秘密鍵の制限 30 件・2 件の内）。`429` は指数バックオフで待つ（`Retry-After` の記載はない）。

### 3. 決済の画面と顧客ポータルの代わり（F1）

v1 にホスト型の画面がないので、adapter が Stripe のホスト型の画面の役をする短命のページを、このサーバーのオリジンの `/payments/pages/payjp/` に出す（§1 の `PageServer`）。ページは ADR-0030 のサーバー描画の画面（`authpages`）と同じく `html/template` で作り、Web クライアント（SPA）の外に置く。

- **署名した状態**: `StartCheckout` と `PortalURL` は、ページに要る状態（こちらの加入の ID・Plan・顧客・メール・ロケール・戻り先・有効期限）を JSON にし、秘密鍵から導いた鍵（HMAC-SHA256）で署名して URL に載せる。サーバーは DB に何も持たない。
  ページの URL のオリジンは `SuccessURL`（ポータルは `returnURL`）のオリジンで、adapter はそれ以外へ送らない（ADR-0074 §7 の URL の確かめに当たる）。URL は Stripe のセッションの URL と同じく短命の持参人の鍵で、`Referrer-Policy: no-referrer` と `Cache-Control: no-store` を付ける。
  有効期限は作成の時刻を 1 時間に切り捨てて 2 時間を足した時刻で、同じ `Key` は同じ時間帯に同じ申し込みを返す。
- **申し込み**（`StartCheckout` → ページ → `SuccessURL`）:
  1. `GET` のページは、プランの名前・金額・間隔、テストのモードの表示（ADR-0074 §5）、メール（主アドレスを入れておく）とカードの名義の欄、payjp.js の card element（PAY.JP の iframe）を出す。ページの小さなスクリプトが payjp.js でトークンを作り、フォームで送る。
     スクリプトは `createToken` に `three_d_secure: true` と名義・メールを渡し、iframe 型の 3D セキュアを経たトークンを受ける。
  2. `POST` は、トークンを取り直して（`GET /v1/tokens/:id`）`card.three_d_secure_status` が `verified` か `attempted` であることを確かめる。顧客を用意し（`CheckoutRequest.Customer` が空なら ID `my_<こちらの加入の ID>` の顧客を作る。あればそれを使う）、トークンのカードを顧客の既定のカードとして足す。
     続けて、`metadata.my_subscription_id` にこちらの加入の ID を入れた定期課金を作る（作る前に顧客の定期課金を読み、もうあれば作らない）。作成は最初の課金を伴う。
     成功すれば `SuccessURL` に `checkout=<CheckoutRef>` を付けて `303` で送る。3D セキュアの失敗・課金の失敗は、理由をページに出し、足したカードを外して入力からやり直させる。`CancelURL` へ戻るリンクを常に出す。
  - `CheckoutRef` は `<顧客>/<こちらの加入の ID>/<有効期限>`。`GetCheckout` は顧客の定期課金を読み、`metadata.my_subscription_id` が一致するものがあれば `completed`（その顧客と定期課金）、なく有効期限を過ぎていれば `expired`、ほかは `open` を返す。
- **ポータル**（`PortalURL`）: 既定のカードのブランド・下 4 桁・有効期限（表示するだけで保存しない）、定期課金の状態（停止していれば支払いに失敗したことと、カードを更新すると再開すること）、直近の支払いの履歴（`GET /v1/charges?customer=`、日付・金額・返金）、カードの変更を出す。
  カードの変更は申し込みと同じ 1・2 の手順（定期課金は作らない）で、成功したら新しいカードを既定にして古いカードを消し、停止している定期課金を `/resume` で再開する（その場で課金する）。解約はポータルに出さない（解約はこのサーバーの画面と API で行う、ADR-0072 §8）。
- **CSP**: ページにだけ次の CSP を付け、Web クライアントの CSP（ADR-0019 §9）は変えない。正確なオリジンは **要検証**（実装で payjp.js の文書に合わせる）。

  ```text
  default-src 'none'; script-src 'nonce-<n>' https://js.pay.jp; style-src 'nonce-<n>'; frame-src https://js.pay.jp https://api.pay.jp;
  connect-src https://api.pay.jp; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'
  ```

- **偽造の防止**: `POST` は `net/http.CrossOriginProtection` を通し、署名した状態をフォームにも載せる（ADR-0019 §4）。ページはカード情報を受けず、受けるのは payjp.js が作ったトークンの ID だけである。
- **i18n**: 文言はサーバーのメッセージ（`server.payments.*`、`locales/en` と `locales/ja`）で、ロケールは状態のロケールを使う（ADR-0009）。
- **PCI**: カードの番号は PAY.JP の iframe の中だけで入力され、このサーバーと Web クライアントのスクリプトは触れない。運営者の範囲は SAQ A 相当で、運営者が確かめる事項として docs に書く（ADR-0074 §5 と同じ）。
  決済のページがこのサーバーのオリジンに載るので（決定者の回答 4 の「セキュリティと PCIDSS に注意」）、PCI DSS v4 の決済のページのスクリプトの管理（要件 6.4.3）と改ざんの検知（要件 11.6.1）に沿って、次を守る。
  - ページが読むスクリプトは、payjp.js と、テンプレートに埋めた nonce 付きの自前の小さなスクリプトだけにする。外部のスクリプトは CSP の `script-src` で `https://js.pay.jp` だけを許す。Web クライアントの bundle・解析・広告のスクリプトを載せない。
  - ページのテンプレートとスクリプトは adapter のソースに `embed` で入れ、実行時に変えられない。CSP と `X-Content-Type-Options: nosniff`・`frame-ancestors 'none'` は常に付け、ページのテストで確かめる。
  - カードの番号・有効期限・CVC を受ける項目を、フォームにもサーバーにも持たない。サーバーが受けるのはトークンの ID・名義・メールだけで、ログに残すのは PAY.JP のオブジェクトの ID だけにする。
  - 運営者の docs に、SAQ A の範囲の確かめ方（このページと PAY.JP の iframe の分担、CSP、TLS）と、ページの CSP を変える設定を持たないことを書く。

### 4. webhook

- 経路は ADR-0074 §3 の `POST /payments/webhooks/payjp`。`ParseWebhook` は `X-Payjp-Webhook-Token` を設定のトークンと定数時間で比べ、欠けか違いは `invalid_signature`。PAY.JP の通知には時刻の署名がないので `stale_signature` を返さない。本文の `livemode` が鍵のモードと違えば `mode_mismatch`。
- 本文は `id`・`type`・`livemode`・`created`・`data.id`・`data.object`・`data.subscription`・`data.metadata.my_subscription` だけを読む。通知を受けても状態は書かず、ジョブが取り直す（ADR-0074 T1）ので、トークンを知る者が古い通知を再び送っても状態は変わらない。
- 種類の写し: `subscription.*`（`created`・`updated`・`deleted`・`paused`・`resumed`・`canceled`・`renewed`）と `charge.failed` は `subscription`、`charge.succeeded`・`charge.refunded`・`dispute.created`・`statement.created` は `money`、ほかは `ignored`。運営者はこの種類を PAY.JP の管理画面で登録する（docs）。
- PAY.JP の再送が短い（Context）ので、取りこぼしは照合（ADR-0074 §4）が拾う。docs で、PAY.JP では `payments_reconcile_interval` を 1 時間にすることを勧める。

### 5. お金の動き（M1）

- **支払い**: 確定した charge（`captured`）のうち、`subscription` を持つものと `metadata.my_subscription` を持つもの（§2 の差額）だけを返す。ID は charge の ID、`Gross` は `amount`、`Fee` は `amount × fee_rate ÷ 100` の端数を切り捨てた額（端数の扱いは **要検証**）、`OccurredAt` は `captured_at`。
  `Covers` は、定期課金の今の期間の始まりが charge の作成と 10 分以内ならその期間、ほかは作成の日から定期課金のプランの間隔の 1 期間、差額の課金は作成の日から `metadata.my_period_end` の前の日まで（日本時間の日付）。
- **返金**: `charge.refunded` のイベントごとに 1 つの動きにする。ID はイベントの ID、`ParentID` は charge、`ParentGross` は charge の `amount`、`Gross` は −（そのイベントの `amount_refunded` − 同じ charge の 1 つ前の `charge.refunded` のイベントの `amount_refunded`）。PAY.JP は返金で手数料を返す（明細の `refund_fee_offset`）ので、`Fee` は返金の額の手数料の負の額とする（返る額の計算は **要検証**）。
- **チャージバック**: `dispute.created` のイベントから、charge と額を読んで `chargeback` にする（イベントの本文の形は **要検証**）。PAY.JP にはチャージバックの取り消しを知らせるイベントと API がないので、`chargeback_reversal` は返さない。運営者が `finance` の画面で記録する（docs）。
- **手数料の補正**: `statement.created`（`type=sales`）ごとに、明細の `fee`・`refund_fee_offset`・`chargeback_fee_offset` の合計と、その区間（`term`）の charge から上の式で計算した手数料の合計を比べ、差があれば `fee` の動き（ID `<statement の ID>:fee`、サブスクリプションなし）を 1 つ返す。これで区間ごとの手数料の合計が PAY.JP の明細と一致する。
  PAY.JP の有料プランの利用料（`service_fee`）と振込の手数料（`transfer_fee`）は収入から引かず、運営者が費用（FR-901）として記録する。
- どの動きも額が後から変わらないので `Final = true`。`ListMoney` は Events API を時刻の窓（1 時間）に区切って `offset` で読み、上の種類のイベントから動きを作る（カーソルは窓と `offset`）。PAY.JP の一覧は新しい順なので窓の中の順は新しい順になるが、照合は `(provider, provider_id)` の一意で重なりを吸収する（ADR-0073 §2）。
  `GetMoney` は ID の形（`ch_`・イベント・`:fee`）で読み分ける。

### 6. 照合

- ADR-0074 §4 の 4 段をそのまま使う。1 の変わったもの（`ListChanges`）は Events API の §4 の種類を同じ窓で読み、2 の未完了の申し込みは `GetCheckout`、3 は §5、4 の見回しは `GET /v1/subscriptions`（`limit=100`、見回しを始めた時刻を `until` に固定して `offset` を進める。カーソルは `until` と `offset`）。
- 見回しの 1 ページで停止の時刻まで分かるので、支払いの失敗のために追加の要求は要らない。期間の終わった定期課金は一覧から消えるが、こちらが続いていると思う加入は Scheduler の期限（ADR-0072 §7）が取り直し、`404` が `canceled` になる。
- Events API の保持期間が分かるまで `ErrChangesWindow` は返さない。保持期間が照合の周期より短いと分かったら、「この決定を覆す条件」に従う。
- テストのモードの 1 秒 2 件では、1 回の照合の上限（既定 1,000 要求）に 8 分ほどかかる。続きのジョブ（ADR-0074 §4）がそのまま引き継ぐ。

### 7. 設定と秘密

ADR-0074 §5 の表に足す（デプロイの設定、秘密は環境変数か `_FILE`、`make config-docs` の表に載せる）。

| キー | 秘密 | 内容 |
| --- | --- | --- |
| `payments_provider` | | `none`（既定）・`stripe`・`payjp` |
| `payments_payjp_secret_key` | ✓ | 秘密鍵（`sk_test_`・`sk_live_`） |
| `payments_payjp_public_key` | | 公開鍵（`pk_test_`・`pk_live_`）。payjp.js と 3D セキュアの開始に使う。秘密鍵とモードが違えば設定の検査で拒む |
| `payments_payjp_webhook_token` | ✓ | webhook のトークン（`whook_`）。モードごとに違う |

- モードは秘密鍵の接頭辞で決める。§3 の状態の署名の鍵は秘密鍵から導く（秘密鍵を替えると、発行済みのページの URL は使えなくなる）。

### 8. build tag とデータモデル

- `no_payments_payjp` は `internal/adapter/payment/payjp` を外し、`payments_provider = payjp` を拒む（[build-tags.md](../reference/build-tags.md)）。依存を増やさないので、代表の組み合わせはどれも含めたまま build を確かめる。
- `schema/<dialect>/12-finance.sql`（`income_movements`）と `13-supporters.sql` の `provider` の値に `payjp` を足す（ADR-0074 の「2 つめの事業者」の手順）。表は増えない。FR-910 の「決済の事業者（Stripe）」を「（Stripe か PAY.JP）」にする。

### 9. テストのしかた

1. **契約テスト**（§1）: `payjp` adapter を、PAY.JP v1 の REST の JSON を返す `httptest` のサーバーの上で通す。fixture はリファレンスの例の形に合わせる。
2. **ページのテスト**: 申し込みとカードの変更を、`httptest` の PAY.JP の上で、署名の誤り・期限切れ・3D セキュアの失敗・課金の失敗・二度目の戻り（定期課金を二重に作らない）・CSP のヘッダーで確かめる。
3. **ローカルでの試し方**（docs）: テストの鍵を設定し、webhook は公開の URL（トンネル）か payjp-cli の `payjp listen --forward-to http://localhost:<port>/payments/webhooks/payjp`（v1 のイベントを転送できるかは **要検証**）で受ける。
   テストのカード（`4242424242424242` など）で申し込み、更新は定期課金の `trial_end` を数分後にして早める（リファレンスがテストの用途として案内している）。
4. **テストモードでの通しの確認**（決定者）: ADR-0074 §6 の 4 の筋書き（加入・ロール・プランの変更・支払いの失敗と猶予とカードの変更での再開・解約・全額と一部の返金・チャージバック・収入の記録と手数料の補正）。

### この決定を覆す条件

- PAY.JP が v2 の定期課金 API を出したら、v2 の adapter への移行を別の ADR で検討する（v1 と v2 は顧客の ID を共有するが、v2 で作った支払い方法は v1 で使えない）。v1 の終了が告知されたときも同じ。
- Events API の保持期間が短く照合で追いつけないと分かったら、`ListMoney` の支払いは `GET /v1/charges` から読み、返金は charge の `amount_refunded` の差（イベントの ID の代わりに累計の額を ID に含める）に変える。
- 取引明細の手数料と charge ごとの計算が区間をまたいでずれる（明細の区間と `term_id` が一致しない）と分かったら、M2 に変える。

## Consequences（影響）

- 良い影響
  - 国内の決済代行（JPY、日本時間の明細）で課金でき、ポートと ADR-0072 の規則を変えずに済む。足す型は任意の `PageServer` と契約テストの 1 項目だけである。
  - Web クライアントは事業者を知らず、CSP も変えない。PAY.JP のスクリプトは adapter の短命のページにだけ載る。
  - 依存を増やさない（`net/http` だけ）。
- 悪い影響・トレードオフ
  - カードの入力・3D セキュア・顧客ポータルのページを adapter が持つので、`stripe` adapter よりコードが多く、i18n の文言とページのテストが増える。カードの入力のページがこのサーバーのオリジンに載る。
  - webhook の検証が固定トークンなので、トークンが漏れると通知を偽造できる（状態は取り直すので書き換えられないが、ジョブと PAY.JP への要求を増やせる）。トークンの切り替えには PAY.JP 側の再発行と設定の変更を同時に行う。
  - PAY.JP は支払いの失敗で定期課金を止め、再試行しないので、猶予の内に利用者がカードを直さなければ戻らない。猶予の通知（ADR-0072 §16）がより大事になる。
  - 上げる変更（U1）は、Stripe と違って期間の始まりを動かさず、差額の課金が断られたら変更しない（Stripe では `past_due` になる）。
  - チャージバックの取り消しは自動で記録されない。手数料は区間の締めまで charge ごとの計算の額で、締めで補正される。
  - v1 は v2 への移行が求められている API で、いずれ移行の作業が要る。
- 後続で必要になる作業（Epic #925）
  - ポートの `PageServer` と `GET`・`POST /payments/pages/{provider}/...` の受け口、契約テストの `TimedSignature` と許可するホスト、`payments_provider = payjp` と §7 の設定、§8 の enum と FR-910
  - `payjp` adapter: §2 の REST の呼び出し・プラン・スナップショット・命令・失敗の種類・要求の速さ、§4 の webhook、§5 のお金の動き、§6 の一覧、契約テスト、`no_payments_payjp` と build-tags.md
  - `payjp` adapter のページ: §3 の申し込みとポータル、署名した状態、トークンの 3D セキュアの確かめ、CSP、`server.payments.*` の文言
  - docs: 運営者の手引き（鍵・webhook の登録と種類・照合の周期・3D セキュアの設定・PCI の範囲・チャージバックの取り消しと PAY.JP の利用料の記録・事業者の切り替えは再加入）
  - 決定者のテストモードでの確認（§9 の 4）と、要検証の項目の結果の記録

## Pros and Cons of the Options（詳細比較・任意）

### API の版: A1 v1（採用）/ A2 v2 でこちらが毎期の課金を組む / A3 v2 の定期課金を待つ

- A2 の最も強い論点: v2 は移行先で、Checkout v2 の `setup` モードでカードの登録までホスト型の画面で済み（スクリプトをこちらに載せない）、冪等キーと RFC 9457 のエラーがある。悪い点: 毎期の課金・失敗・期間・予約された変更をこちらのジョブと表で持つ定期課金のエンジンになり、ポートの「事業者の側の事実を取り直す」の前提（ADR-0074 T1）が崩れる。
- A3 の良い点: 作業が要らず、移行もない。悪い点: 時期が決まっておらず（「2026年対応予定」）、日本の運営者が今は使えない。

### カードの入力: F1 adapter のページ（採用）/ F2 SPA に組み込む / F3 v1 の Checkout

- F2 の最も強い論点: 画面が Web クライアントの見た目と部品（ADR-0025 以降）で揃い、React と client の i18n で書ける。悪い点: Web クライアント全体の CSP に PAY.JP のオリジンを足すことになり、事業者ごとのコードが SPA に入る。SPA のどの画面にも PAY.JP のスクリプトを載せうる範囲になる。
- F3 の良い点: PAY.JP の用意したボタンとモーダルで、フォームのコードが要らない。悪い点: 結局こちらのページにスクリプトを載せ、見た目と文言を変えにくく、3D セキュアの扱いを別に組む。

### 上げる変更: U1 差額の都度の課金（採用）/ U2 PAY.JP の `prorate`

- U2 の最も強い論点: PAY.JP の機能で、adapter のコードが少なく、月から年への変更は Stripe と同じく変更の日から年払いが始まる。悪い点: 旧プランの残りを返金として返すので、支払いの直後の変更では返金が支払いの全額に等しくなりえ、ADR-0072 §7 が全額の返金として加入を `revoked` にしてしまう。返金と手数料の返還の動きが変更のたびに増える。

### 手数料: M1 charge ごとに計算して締めで補正（採用）/ M2 明細の区間の額だけ

- M2 の良い点: PAY.JP の明細そのままで、計算の誤りがない。悪い点: 支払いごとの手数料と net（FR-917、ADR-0073 §2 の支払いごとの記録）が埋まらず、区間が締まるまで手数料が記録されない。

### API の呼び出し: R1 `net/http`（採用）/ R2 `payjp-go`

- R2 の良い点: 公式の SDK で、型とリトライがある。悪い点: リファレンスが `beta` ブランチを指し、版の追従が読みにくい。使う API は十数で、フォームの本文と JSON の応答は標準ライブラリで足りる。

## Open Questions / 要検証

- Plan の削除が既存の定期課金に効くか（効かなくても §2 は削除しない）と、Plan の ID の長さの上限。
- `fee_rate` からの手数料の端数の扱い、返金とチャージバックで返る手数料の計算（§5）。
- `dispute.created` のイベントの本文の形（§5）。
- Events API の保持期間と、v1 の webhook の再送の回数（§4・§6）。
- payjp.js（`https://js.pay.jp/v2/pay.js`）の iframe 型の 3D セキュアの CSP に要るオリジン（§3）。
- v1 で `Idempotency-Key` が使えるか（使えれば §2 の命令にも付ける）。
- payjp-cli が v1 のイベントを転送できるか（§9）。
