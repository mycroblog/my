# ADR-0162: チップ（1 回の支払い）の受け付けと記録

- Status: Proposed
- Date: 2026-11-09
- Deciders: @rosylilly
- Issue: #2612, #2611

## Decision（決定）

§1〜§8 は決定者が決めたことである。§9 以降は推奨案で、spike #2611 の[調査報告](https://github.com/mycroblog/my/issues/2611)と今のコードから形を決めた。

チップ（FR-918）は、サポーターの機能（ADR-0072）が有効で決済の事業者が設定されたサーバーで、ローカルの利用者がサーバーの運営に 1 回だけ払うお礼である。
加入（ADR-0072）と同じモジュール `supporters` が持ち、事業者とのやりとりは `PaymentProvider`（ADR-0074）を広げて通し、収入は `income_movements`（ADR-0067）に載る。

### 1. チップが何であるか

- チップは**サーバーが提供しているサービスへのお礼**で、寄付ではない。画面と決済の画面は「<サーバー名> の運営へのチップ。見返りはありません」と書く。
  寄付・税の控除・目的のための募金と読める文言を、画面にも決済の画面の品目にも運営者向けの docs の例文にも使わない。
- Stripe の決済の画面に `submit_type=donate` を使わない（ボタンが「寄付する」になる）。PAY.JP v2 の `submit_type` も寄付の値を使わない。
- 理由: Stripe はチップを「提供した商品やサービスに対して払われるもの」として認め、寄付は特定の慈善の目的に結び付くものに限り、日本では個人への寄付を扱わない（[チップや寄付の受け付けに関する要件](https://support.stripe.com/questions/requirements-for-accepting-tips-or-donations)、2026-11-09 に確認）。

### 2. 名前

- 英語は **Tip**、日本語は **チップ**。UI・API・コード・docs で同じ語を使う（`locales/en`・`locales/ja`）。
- 「Support」は「Supporter」（加入、ADR-0072）と紛れるので使わない。

### 3. 範囲

- **プランがなくても使える**: サポーターの機能が有効（ADR-0072 §2）で事業者が設定されていれば、プランを 1 つも作らずにチップだけを有効にできる。チップだけで運営費を受けるサーバーのため。
- **見返りはない**: 特典もロールも付けない。お礼は支払いの完了のページに出すだけである。
- **運営者にだけ届く**: ほかの利用者へのチップ、残高・コイン、目標の額、キャンペーンを持たない。利用者の間の送金は Stripe が扱わず（"personal or peer-to-peer money transmission"）、資金決済法の為替取引・前払式支払手段に当たりうる。目標と募集は Stripe の資金調達・クラウドファンディングの制限に当たる（spike #2611 の「Notes for operators」）。
- **モバイルアプリに導線を出さない**: 加入と同じく（FR-911、ADR-0072 §14）、Web クライアントが Tauri のアプリとして動くときは、チップの画面・ボタン・案内を出さない。

### 4. 事業者と運営者の責任

- **Stripe と PAY.JP の両方で作る。** PAY.JP のヘルプは 投げ銭・心づけ・応援機能 を「取り扱いが困難な商材」に挙げる（[商材によって審査が難しい／スムーズなものはありますか？](https://help.pay.jp/ja/articles/4506410)、2026-11-09 に確認）が、`my` はそれを理由に止めない。事業者の審査と規約に従うのは運営者の責任で、運営者向けの docs にそう書く。
- **`my` は使ってよいかの確認を求めない。** チップを有効にする管理画面は、短く次を示し、各事業者の条件を説明する運営者向けの docs にリンクする（法的な助言ではなく情報として）。
  - dispute（チャージバック）ごとの手数料（Stripe の日本では 1 件 ¥1,500、[料金](https://stripe.com/jp/pricing)、2026-11-09 に確認）。
  - チップの返金とチャージバックは収入から差し引かれること。

### 5. 事業者の方式

- **Stripe**: `mode=payment` の Checkout Session（ホスト型）。
- **PAY.JP**: チップは **API v2** の Checkout Session（`mode=payment`）で受け、支払いは `payment_flow_id` で引き、返金は `POST /v2/payment_refunds`、dispute は `/v2/payment_disputes` で読み、作成の要求に `Idempotency-Key` を付ける。
  既存の加入は v1 の定期課金 API のまま（ADR-0074 §10）。PAY.JP は移行の期間に同じ鍵で v1 と v2 を併用することを認めている（[API v2 への移行](https://docs.pay.jp/v2/guide/v2-migration)、2026-11-09 に確認）。

### 6. 金額

- 金額は運営者が選んだ候補からだけ選べる。利用者は額を入力しない。
- 候補は **¥100〜¥100,000、US$1〜US$1,000** の間で、事業者の 1 回の支払いの最低額より上でなければならない。最低額を知らない通貨の候補は受けない。
- 下限は、最低額ぎりぎりのチップが dispute の手数料で大きく損をするため（¥500 のチップが dispute で ¥2,000 の負担になる）。

### 7. カードの試し打ちへの備え

- チップを払えるのは、サインインしたローカルの利用者で、主アドレスのメールの確認が済んだ人だけ（ADR-0072 §6 の加入の条件のうち、続いている加入がないことを除いたもの）。
- 金額は §6 の候補だけ。利用者ごととサーバー全体のレート制限を掛け、急増を見る計測値を出す（§17）。

### 8. 収入の出どころ

- `income_movements` に出どころの列 `source`（`subscription`・`one_time`・`unknown`）を足し、チップを `my` の外で受けた支払いと見分ける（ADR-0067 §18）。
- 収入の行は今と同じく利用者を持たない（ADR-0067 §12）。値の決め方は §16。

### 9. ポート（推奨案）

ADR-0074 §1 の `PaymentProvider` は加入の形のまま変えず、1 回の支払いを任意のインターフェースとして `internal/app/supporters` に足す（§9 の `PageServer` と同じ形）。adapter がこれを実装しているかが、チップを受けられるかの報告になる。

```go
// OneTimePayments is a provider that takes one-time payments (tips, ADR-0162).
type OneTimePayments interface {
    // OneTimeLimits reports the smallest and largest one-time charge the provider takes in
    // currency, in minor units; ok is false for a currency it does not take.
    OneTimeLimits(currency string) (min, max int64, ok bool)
    // StartOneTimeCheckout creates a hosted checkout of one payment and returns its page.
    StartOneTimeCheckout(ctx context.Context, req OneTimeRequest) (Checkout, error)
    // GetOneTimeCheckout reports whether a one-time checkout was paid and what was charged.
    GetOneTimeCheckout(ctx context.Context, ref CheckoutRef) (OneTimeResult, error)
    // ExpireOneTimeCheckout makes an open one-time checkout unpayable and reports what it is
    // now: expired, or paid when the customer got there first.
    ExpireOneTimeCheckout(ctx context.Context, ref CheckoutRef) (OneTimeResult, error)
}

type OneTimeRequest struct {
    LocalID     string // our tip id; stored on the checkout and the payment as my_tip_id
    AmountMinor int64  // one of the operator's choices (§6)
    Currency    string // ISO 4217
    Description string // the line item, localized: "Tip for <server>"
    Email       string // prefills the checkout; no provider customer is created or reused
    Locale      string
    SuccessURL  string // our origin; the provider appends its checkout id
    CancelURL   string
    Key         string // idempotency key
}

type OneTimeResult struct {
    Ref         CheckoutRef
    Status      OneTimeStatus // open, paid, expired
    LocalID     string
    Payment     PaymentRef    // paid: Stripe PaymentIntent, PAY.JP v2 PaymentFlow
    AmountMinor int64         // paid: as charged
    Currency    string
}
```

- `MoneyMovement` に `Payment PaymentRef` を足す。事業者の支払いの参照（Stripe は PaymentIntent、PAY.JP v1 は charge、v2 は PaymentFlow）で、支払いが持ち、その返金・チャージバック・取り消しが同じ値を受け継ぐ。`supporters` はこれでチップに行き着く。加入の支払いにも分かる限り入れる。
- 加入の `CheckoutResult` は変えない。`OneTimeStatus` の `paid` は、決済が済んで金額が確定したことを言う。カードだけを受ける（§10・§11）ので、「完了したが未払い」は起きない。
- `Change`（ADR-0074 §1）に `OneTimeCheckout CheckoutRef` を足し、`Resolve` は 1 回の支払いの画面の通知をそこに返す（adapter は Stripe の `mode`、PAY.JP の API の版で見分ける）。加入の `CheckoutRef` には入れない。
- 顧客を作らず、`supporter_customers` の顧客も使わない。メールだけを渡す。チップの記録を消しても（§18）事業者の側にこちらの利用者に結び付く顧客が残らないため。
- `my` はチップの返金の操作を持たない。運営者は事業者のダッシュボードで返金し、返金はお金の動きとして記録される（§15）。
- **契約テスト**: `pkg/testkit.PaymentProviderContract` に、`OneTimePayments` を実装する adapter だけが通す項目を足す: 作成の URL のホスト、支払い・期限切れ・`ExpireOneTimeCheckout` と支払いの行き違い、全額の返金、一部の返金、チャージバック、その取り消し（ある事業者だけ）、返金・チャージバック・取り消しの `MoneyMovement.Payment` が支払いのものと等しいこと、`OneTimeLimits` と範囲の外の金額の `invalid_request`。fake・`stripe`・`payjp` の adapter が通す。

### 10. Stripe adapter（推奨案）

- `StartOneTimeCheckout`: `mode=payment`、`payment_method_types=["card"]`、`line_items` に `price_data`（`currency`・`unit_amount`・`product_data.name` に `Description`）を 1 つ、`client_reference_id` と `metadata.my_tip_id` と `payment_intent_data.metadata.my_tip_id` に `LocalID`、`customer_email`、`locale`、`success_url`（`?checkout={CHECKOUT_SESSION_ID}`）、`cancel_url`、`expires_at` は 1 時間後（Stripe は 30 分〜24 時間）。`submit_type` は付けない。`Idempotency-Key` は `Key`。
  Price も Product も作らない（`EnsurePlan` は要らない）。
- カードだけにするのは、コンビニなどの遅れて決まる支払いでは `checkout.session.completed` が `payment_status=unpaid` で届き、`async_payment_*` のイベントを足す必要があるため（[Fulfill orders](https://docs.stripe.com/checkout/fulfillment)、2026-11-09 に確認）。
- `GetOneTimeCheckout` と `ExpireOneTimeCheckout`: `GET /v1/checkout/sessions/{id}` と `POST .../expire`（ADR-0074 §2 の `ExpireCheckout` と同じ 400 の扱い）。`paid` は `status=complete` かつ `payment_status=paid`。`Payment` はセッションの `payment_intent`。
- `OneTimeLimits`: 下限は `currency.go` の最低額の表（1 回の支払いも同じ表、[Supported currencies](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts)）。上限は Stripe の 1 回の上限（8 桁、要検証）。
- お金の動き: 今の `movement` は、invoice に結び付かない charge を `Subscription` なしの `payment` にしている。そこに `Payment` として PaymentIntent を入れる。返金と dispute は元の charge の PaymentIntent を受け継ぐ。新しい webhook のイベントは要らない（ADR-0074 §3 の一覧で足りる）。

### 11. PAY.JP adapter: v2 で 1 回の支払い（推奨案）

- 同じ adapter `internal/adapter/payment/payjp` の中に v2 のクライアントを足す（`https://api.pay.jp/v2`、同じ秘密鍵。認証と本文の形は v2 の文書に合わせる。要検証）。v1 のクライアントと要求の速さの上限（ADR-0074 §10）を分け合う。
- `StartOneTimeCheckout`: `POST /v2/checkout/sessions`（`mode: payment`、`line_items` に `price_data`（JPY）、`client_reference_id` と `metadata.my_tip_id`、`payment_flow_data.metadata.my_tip_id`、`customer_email`、`payment_method_types` はカード、`success_url`・`cancel_url`、`expires_at` は 1 時間後、`locale`）に `Idempotency-Key`。返る `url` のホストを ADR-0074 §7 と同じく確かめる（ホストの名前は要検証）。
  v1 の自前のページ（ADR-0074 §11）は使わない。カードは PAY.JP のホスト型の画面で入力され、3D セキュアは初回の利用で自動で行われる（[3D セキュア認証](https://docs.pay.jp/v2/guide/payments/methods/card/3ds)、2026-11-09 に確認）。
- `GetOneTimeCheckout`・`ExpireOneTimeCheckout`: セッションを読み（`open`・`complete`・`expired`）、`complete` なら `payment_flow_id` の PaymentFlow を読んで成功していれば `paid`。PAY.JP は「Webhook で結果を受信して初めて支払いが完了した」と案内するので、`complete` でも PaymentFlow が成功でなければ `open` と答える。失効は `POST /v2/checkout/sessions/{id}/expire`。
- `OneTimeLimits("JPY")` は 50 と 9,999,999（[支払い方法](https://docs.pay.jp/v2/guide/payments/methods)、2026-11-09 に確認）、ほかの通貨は false。
- **お金の動き**: v2 の支払いの記録（Payment Transactions の `payment`・`refund`・`chargeback`・`chargeback_cancel`、[支払いの取引](https://docs.pay.jp/v2/guide/payments-management/payment-transaction)）を `ListMoney` の別のカーソルで読み、`payment`・`refund`・`chargeback`・`chargeback_reversal` にする。`Payment` は PaymentFlow の ID。
  v1 と違い、チャージバックの取り消しが記録に出るので `chargeback_reversal` を返す。手数料が取引ごとに出るか、v1 の取引明細（`statement`）に v2 の手数料が入るかは要検証で、入るなら ADR-0074 §13 の手数料の補正は v2 の取引の手数料も合計に足す（二重に引かないため）。
- **dispute の終わり**: v2 の dispute のイベントは `dispute.created` だけなので、照合（§14）が開いている dispute を `GET /v2/payment_disputes` で読み、`lost`・`cancel` を知る。お金の動きは上の取引から入る。
- **webhook**: ADR-0074 §12 の同じ経路 `POST /payments/webhooks/payjp` で受け、`ParseWebhook` が v1 と v2 の通知を本文の形とオブジェクトの種類で見分ける。v2 は `data` がリソースそのもの。v2 の種類の写し: `checkout.session.completed`・`.expired` は 1 回の支払いの画面、`payment_flow.succeeded`・`refund.*`・`dispute.created` は `money`、ほかは `ignored`。
  1 つのエンドポイントに v1 と v2 の両方が届くか、トークンが同じかは要検証で、分かれるなら経路とトークンの設定を 1 つ足す（Open Questions）。

### 12. チップの記録（データモデル、ADR-0020。推奨案）

`supporters` が持つ。

| テーブル | 分類 | カラム | 制約・規則 |
| --- | --- | --- | --- |
| `supporter_tip_changes` | イベント | `id`, `state`（`enum(enabled, disabled)`）, `currency`（`string(3)`）, `changed_by_user_id`（NULL 可 = CLI）, `changed_at` | 最新の行が今の状態。行がなければ無効。`enabled` の行は子の `supporter_tip_amounts` を 1〜6 行持つ |
| `supporter_tip_amounts` | イベント（子） | `change_id`, `amount_minor`（`bigint`） | PK `(change_id, amount_minor)`。§6 の範囲はアプリケーションが確かめる |
| `supporter_tips` | リソース | `id`（事業者に載せるこちらの ID）, `user_id`, `amount_minor`, `currency`, `provider`, `created_at` | インデックス `(user_id, id DESC)` |
| `supporter_tip_checkouts` | イベント | `tip_id`（PK）, `provider`, `provider_checkout_id`, `expires_at`, `created_at` | 一意 `(provider, provider_checkout_id)`。1 つのチップに画面は 1 つ |
| `supporter_tip_payments` | イベント | `tip_id`（PK）, `provider`, `provider_payment_id`, `amount_minor`, `currency`, `paid_at` | 一意 `(provider, provider_payment_id)`。お金の動きの `Payment` からチップを引く |
| `supporter_tip_outcomes` | イベント | `id`, `tip_id`, `kind`（`enum(expired, refunded, charged_back, dispute_won)`）, `occurred_at` | 一意 `(tip_id, kind)` |
| `supporter_open_tips` | 投影 | `user_id`（PK）, `tip_id`（一意）, `expires_at` | 支払いも期限切れもまだないチップ。利用者ごとに 1 つ。インデックス `(expires_at)` |

- 状態を NULL 可の時刻の列で表さない（ADR-0020 規約 4）。チップの状態は、支払いの行があれば `paid`、なく `expired` の結果があれば `expired`、どちらもなければ `open` で、支払いの後の `refunded`・`charged_back`・`dispute_won` は結果の行の有無で決まる。
- 金額の候補は、有効・無効の変更と同じ行の版で持つ。候補は 1 つの通貨で、6 つまで。
- `supporter_open_tips` は同期の投影で、記録から作り直せる（`my projection verify`）。
- お金の額（総額・手数料・net）はここに持たない。ADR-0067 の `income_movements` が持つ。

### 13. 支払いの開始と「支払いは 1 回まで」（推奨案）

1. 利用者が候補の金額を選び、確認の画面（§19）を経て `POST` する。サーバーは §7 の条件、チップが有効なこと、金額が今の候補にあること、レート制限を確かめる。
2. 開いているチップ（`supporter_open_tips`）があれば、まずそれを `ExpireOneTimeCheckout` で失効させる。失効の時に支払い済みなら、支払いとして記録し、新しい画面を作らずに完了のページへ送る（2 つのタブから 2 回払わないため）。失効できなければ開始を失敗にする。
3. 1 つのトランザクションでチップの行と投影の行を書き、トランザクションの外で `StartOneTimeCheckout`（冪等キー `tip:<tip_id>`）を呼び、画面の行を短いトランザクションで書いて、リダイレクトの URL を返す。
4. 戻ったページは `GET` でチップの状態を読み、まだ `open` なら `supporters.sync_tip` を積んで取り直す（webhook を待たない）。`paid` になったらお礼を出す。
5. 期限（1 時間）を過ぎた開いているチップは、5 分ごとの `supporters.sync_due`（ADR-0072）と同じジョブが取り直し、`expired` か `paid` にする。

### 14. webhook と照合（推奨案）

- ジョブ `supporters.apply_payment_event`（ADR-0074 §3）は、`Change.OneTimeCheckout` があれば `GetOneTimeCheckout` で読み、チップの記録に当てる（`SyncTip`）。加入の `SyncCheckout` には渡さない。
- 加入の `SyncCheckout` が完了した画面のこちらの加入を見つけられないとき、今は何も残さずに捨てている（`internal/app/supporters/core.go` の `LockSubscription` の `ErrNotFound`）。警告のログを出すようにする。
- 照合（ADR-0074 §4）の 2 段め「未完了の申し込み」に、期限の切れていない開いているチップの画面を足す（`Syncer.OpenTipCheckouts`）。1 時間で期限が切れ、利用者ごとに 1 つなので件数は少ない。
- PAY.JP v2 では、2 段めの後に、`charged_back` で `dispute_won` のないチップの dispute を読む（§11）。
- 3 段め「お金の動き」は変えない。1 回の支払いの動きも `ListMoney` から入り、§15 と §16 に回る。

### 15. 結果（推奨案）

- `paid`: `GetOneTimeCheckout` が `paid` を返したとき、`supporter_tip_payments` に書く。返った金額と通貨がチップの行と違えば記録はするが警告のログを出す。
- `expired`: 期限切れか失効を事業者が答えたとき。
- `refunded`: `Payment` がチップの支払いに当たる返金で、額が支払いの総額以上のもの（ADR-0072 §7 の全額の判断と同じ `ParentGross` の比較）。一部の返金は収入の行だけで、結果にしない（一部の返金を重ねて全額になっても `refunded` にしない）。
- `charged_back`: チャージバックの動き。`dispute_won`: その取り消しの動き（Stripe の `dispute_reversal`、PAY.JP v2 の `chargeback_cancel`）。PAY.JP v1 の支払いはチップに使わないので、取り消しを知る手段がない v1 の制限（ADR-0074 §13）はチップに及ばない。
- 結果はロールも状態も動かさない（特典がないため）。`manage_finance` の個々の動きの画面と、`manage_supporters` のチップの一覧に出すためにある。
- `RecordMoney` は、今は加入を持たない動きを取り消しの判断の前に飛ばしている。`Payment` がチップの支払いに当たる動きを結果の判断に回すように変える。

### 16. 収入の計上と公開（推奨案）

- `supporters` が `IncomeMovement` に `Source` を入れる: 加入がこちらの加入に当たれば `subscription`、`Payment` がこちらのチップの支払いに当たれば `one_time`、どちらでもなければ `unknown`（`my` の外の支払いと、加入に結び付かない手数料）。
  返金・チャージバック・取り消しは、元の支払いの行の `source` を受け継ぐ（`finance.RecordIncome` が元の行から写す）。チップの記録を消した後（§18）の返金も `one_time` になる。
- チップの支払いは `Covers` を持たないので、按分でも現金主義でも、支払った月（`occurred_on`）に全額が載る（ADR-0067 §15 の「`Covers` を持たない支払い」の規則のまま）。
- `income_visibility = plans` と `view_finance` の人数（ADR-0067 §17）では、プランごとの行の後に「チップ」の行を、チップが有効なときだけ、人数なしで出す。合計にチップが含まれることを示すだけで、チップの額も人数も出さない（少ない人数から誰が払ったかが推測できるため）。
- `manage_finance` の `/admin/finance` と `GET /admin/income_totals` は、期間ごとの net を `source` ごとにも出す。個々の動き（`GET /admin/income_movements`）は `source` を持ち、`one_time` の行は払った人のアカウントを出す（記録が残っている間。§18）。個々のチップはこの画面と API だけに出す。

### 17. 権限・画面・API・計測（推奨案）

- 権限: チップの有効化と金額の候補は `manage_supporters`（ADR-0072 §2）。管理画面 `/admin/supporters` に「チップ」の区画を置き、§4 の案内と、事業者が `OneTimePayments` を実装していないときの理由を出す。
  有効化・候補の変更・無効化は監査ログ（ADR-0058）に `supporter_tips_enabled`・`supporter_tips_revised`・`supporter_tips_disabled` で残す。
- サポーターの機能を無効にする（ADR-0072 §2）と、チップも受けない。開いているチップの画面は期限まで残り、払われれば記録される。
- API（[client-api.md](../architecture/client-api.md) の手順で細部を決める）:
  - 公開: `GET /instance` に `tips_enabled`、`GET /instance/tip_amounts`（候補の金額と通貨。無効なら `404`）。
  - 利用者（タグ `supporters`、スコープ `write:supporters` / `read:supporters`）: `POST /supporter/tips`（`{amount}`。条件を満たさなければ `422`、レート制限で `429`）がリダイレクトの URL を返す。`GET /supporter/tips/{id}`（本人のチップの状態だけ）。
  - 管理（`admin-server`、`manage_supporters`）: `GET/PUT /admin/supporter_tips`（有効化と候補）。
- Web: `/about` のサポーターの区画と `/settings/supporter` からチップの画面（`/settings/supporter/tip`）へ。完了のページはお礼だけを出す。
- **レート制限**（ADR-0019 §5 の表に足す。要検証: 運用で調整）: 開始の `POST` を利用者ごとに 5 回 / 時・20 回 / 日、サーバー全体で 100 回 / 時。超えた全体の制限は管理画面の支払いの状態にも出す。
- **計測**: `my_supporter_tip_checkouts_total`（`my_outcome` は `started`・`rate_limited`・`paid`・`expired`）を出し、`docs/operations/` の計測値と警告の表に「開始の急増」と「支払いに至らない開始の多さ」の警告の例を載せる。

### 18. アカウントの削除と消去（推奨案）

- アカウントの削除（BAN の削除の予約を含む）では、開いているチップの画面を失効させる。
- アカウントの消去（ADR-0020 規約 6）は、その利用者のチップの記録（`supporter_tips` と子の行、投影）を消す。収入の行は利用者を持たないので額と `source` だけが残り、集計は変わらない。data-model §8 の消去の表に足す。
- 事業者の側には、チップの支払いと決済の画面に入れたメールが残る（事業者の記録で、`my` は消せない）。プライバシーポリシーの例文と運営者向けの docs に書く。

### 19. 運営者の docs と確認の画面（推奨案）

- 確認の画面（`POST` の前）に、金額・1 回だけでいますぐ支払われること・見返りがないこと・特商法の表記（ADR-0072 §13）へのリンクを出す。
- `docs/operations/supporters.md` にチップの節を足す: 有効にする手順、Stripe と PAY.JP の条件（§1・§4、spike #2611 の各事業者の項）、dispute の手数料、特商法の表記に足す事項（チップが何か・金額・支払いの時期・提供するものはないこと・返金の扱い）、消費税・所得税の扱いは税理士に確かめること。どれも法的な助言ではない情報として書く。
- Stripe の日本で個人事業主の運営者へのチップが受け付けられるかは、分からないまま docs に「Stripe に確かめる」と書く（Open Questions）。

### この決定を覆す条件

- Stripe か PAY.JP が、サーバーの運営へのチップを規約で認めないと明らかにしたら、その事業者の adapter から `OneTimePayments` を外す（ポートと記録の形はそのまま）。
- カードの試し打ちが §17 の制限を越えて起きたら、CAPTCHA（ADR-0106）を開始に足すか、制限を絞る。
- PAY.JP が v2 の定期課金 API を出して加入も v2 に移ったら（ADR-0074 の覆す条件）、§11 の v1 と v2 の見分けは要らなくなる。
- 運営者が任意の金額を求める声が多ければ、§6 の範囲の中で利用者が額を入れる形（Stripe の `custom_unit_amount`）を検討する。

## 経緯

M4.2 の加入（FR-910〜FR-917、ADR-0072・ADR-0074）は定期の支払いだけを扱い、プランを作らないサーバーには運営費を受ける手段がなかった。そこでチップ（FR-918）を、加入と同じ有効化・事業者・収入の記録の上に足すことにした。
`PaymentProvider` は加入の形で、`CheckoutRequest` は定期の価格と加入の ID を、`CheckoutResult` はサブスクリプションだけを、`MoneyMovement` はサブスクリプションだけを持つ。`income_movements` は `subscription_id` に NULL を許すが、NULL はチップと `my` の外の支払いの両方を意味してしまう。

spike #2611 は事業者の規約と API を文書だけで調べた（2026-11-09、テストモードの試行はしていない）。Stripe はサービスへのチップを認め、寄付は目的に結び付くものに限り日本では個人への寄付を扱わない。PAY.JP は 投げ銭・心づけ・応援機能 を審査の難しい商材とする。今のコードでは、Stripe の adapter はチップの支払いを加入のない収入として記録するがチップに結び付けず、`SyncCheckout` は完了した画面を捨て、PAY.JP の adapter は加入のない charge を返金・dispute ごと捨てる。
報告は PAY.JP では受けず Stripe だけで受け、運営者に使ってよいかを確かめさせる案を勧めた。決定者はこれを見直し、両方の事業者で作り、確認は求めずに案内だけを出し、PAY.JP は v1 にない ホスト型の画面・冪等キー・dispute の一覧を持つ v2 で受けることにした。日本語の名前は、Stripe の審査が寄付と読みうる語を避けてチップにした。

ポートを加入のメソッドの分岐で広げず別のインターフェースにしたのは、加入のメソッドの契約（価格・サブスクリプション・ロール）を変えずに済み、チップを受けない事業者が型で分かるためである。結果をお金の動きから導くのは、加入の取り消しの判断（ADR-0072 §7）と同じ経路で、webhook の取りこぼしを照合の 3 段めが直すためである。

## 採らなかった案

- 日本語の名前をカンパにする: 気軽な語で、運営費を皆で出し合う意味に合う。ただ日本語ではしばしば寄付の意味で使われ、Stripe の日本の審査が寄付（個人への寄付は扱わない）と読みうる。
- 「Support」「サポート」: 意味は広い。ただ加入の「Supporter」と紛れる。
- チップを Stripe だけにし、PAY.JP では受けない（spike #2611 の推奨）: PAY.JP の審査で断られる運営者を生まない。ただ国内の決済だけを使う運営者がチップを使えず、審査の判断は運営者と事業者の間のことである。
- 有効にする前に、運営者に事業者の条件を満たすことを確かめさせ監査ログに残す（spike #2611 の option B）: 運営者に条件を読ませる。ただ `my` には確かめる手段がなく、チェックの欄は保証にならず、手順だけが増える。
- 決済の画面で贈与の合意を取る（option C）: 会社の運営者が寄付として受けられる。ただ寄付と読める文言が増え、個人の運営者の助けにならない。
- 法人の運営者だけに許す（option D）: 日本の規則では最も安全。ただチップだけで運営する個人のサーバーという目的に反する。
- PAY.JP v1 の charge を自前のページで作る: 加入と同じ API の版で済む。ただホスト型の画面がなく、カードの入力のページと 3D セキュアと冪等の代わりの仕組み（ADR-0074 §11）をチップにも持つことになる。
- `StartCheckout`・`GetCheckout` に 1 回の支払いの分岐を足す: メソッドが増えない。ただ加入の契約（`Price`・`Subscription`・`ResumeCheckout`）がチップに意味を持たず、チップを受けない事業者を型で表せない。
- Stripe の Payment Links: 作る要求が要らない。ただ使い回すリンクで、サインインした利用者とチップの ID を結び付けられず、冪等もない。
- `custom_unit_amount` で利用者が額を入れる: 柔軟。ただ小さい額のカードの試し打ちに使われやすく、§6 の候補だけの決定に反する。
- 収入の出どころを `supporters` の表（`provider_movement_id` ごと）に持つ: `income_movements` の列が増えない。ただ収入の集計が `finance` だけで出どころを分けられず、決定は列を足すことにした。
- チップの人数か額を公開の集計に出す: 透明。ただチップは件数が少なく、額と時期から誰が払ったかが推測できる。
- チップの払い手に事業者の顧客を作るか、加入の顧客を使う: 事業者のダッシュボードで払い手ごとに見られる。ただ消去の後も事業者の側にこちらの利用者に結び付く記録が増え、チップには要らない。
- チップの記録を消去の後も残す: 返金がチップに行き着き続ける。ただ誰が払ったかを消去の後も持つことになる。収入の `source` は元の行から受け継ぐので、消しても集計は変わらない。

## Consequences（影響）

- 良い影響
  - プランのないサーバーも、加入と同じ有効化・事業者・照合・収入の記録の上でチップを受けられる。新しい事業者の設定も秘密も要らない（PAY.JP v2 の webhook が分かれる場合を除く）。
  - 加入のメソッドと契約は変わらず、チップを受けるかは adapter が任意のインターフェースを実装するかで決まる。
  - チップの返金・チャージバックが `MoneyMovement.Payment` でチップに行き着き、収入は `source` で `my` の外の支払いと分かれる。
  - チップは特典もロールも持たないので、結果が誰かの権限を動かすことはない。
- 悪い影響・トレードオフ
  - PAY.JP の adapter が v1 と v2 の 2 つのクライアント・2 つの webhook の形・2 つのお金の動きの経路を持つ。v2 の文書は v1 ほど確かめられておらず、要検証が多い。
  - PAY.JP の審査でチップが断られる運営者が出うる。`my` は止めないので、運営者が docs を読まなければ契約上の問題を抱えうる。
  - Stripe の日本の dispute の手数料（¥1,500）は小さいチップより大きく、チャージバック 1 件でそのチップの何倍もの損になる。
  - 確認と開始の画面・API・管理の区画・計測値・i18n の文言・契約テストの項目が増える。
  - チップの記録を消去で消すので、消去の後の返金・チャージバックはチップの結果にならず、収入の行だけになる。
- 後続の作業（マイルストーン One-time Tips の Issue）
  - ポートの `OneTimePayments` と `MoneyMovement.Payment`、fake と契約テスト。
  - `stripe` adapter の 1 回の支払い。
  - `payjp` adapter の v2 のクライアント・webhook・お金の動き・dispute の照合。
  - `supporters` のチップの記録・開始・同期・照合・結果・消去、`income_movements.source`。
  - API・Web の画面・管理画面・監査ログ・レート制限・計測値と警告。
  - `docs/operations/supporters.md` のチップの節、`docs/reference/` の計測値・ジョブの一覧。

## Open Questions / 要検証

- PAY.JP の 1 つの webhook のエンドポイントに v1 と v2 の両方のイベントが届くか、トークンが同じか、本文の形でどう見分けるか（§11）。分かれるなら経路と `payments_payjp_v2_webhook_token` を足す。
- Stripe の日本で、個人事業主の運営者へのサービスへのチップが受け付けられるか。どちらでも運営者向けの docs に書く（§19）。
- PAY.JP v2 の認証と本文の形、Checkout Session の URL のホスト、作成に `Idempotency-Key` が効くこと（§11）。
- PAY.JP v1 の取引明細に v2 の手数料が入るか、v2 の取引ごとに手数料が出るか（§11）。
- Stripe の 1 回の支払いの上限の額（§10）と、入金の通貨が違うときの最低額の換算（ADR-0074 §2 と同じ）。
- 金額の範囲を JPY と USD のほかの通貨（EUR など）にどう広げるか。決めるまでは JPY と USD だけを候補に受ける。
