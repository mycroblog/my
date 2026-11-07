<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# Stripe によるサポーターの支払い

`payments = stripe` にすると、ローカルの利用者は Stripe がホストする支払いページ（Checkout）で月額または年額のプランに加入でき、支払いの状態が変わるのに合わせて、プランのロールが付与され、取り除かれます。
デプロイ時の設定は [設定のリファレンス](../reference/configuration.md#payments) で説明しています。プログラム全体（プラン、ロール、メンバーの一覧）については [supporters.md](supporters.md) を参照してください。
プログラムを有効にし、プランを作るのは管理画面で管理者が行います。この設定は、サーバーが Stripe とやりとりできるようにするだけです。

1 つのサーバーで使える事業者は 1 つだけです。あとで別の事業者に切り替えるには、既存の加入者に新しい事業者で加入し直してもらってください。

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## サーバーが Stripe で行うこと

[加入のしくみ](supporters.md#how-a-subscription-works) では、支払いページ、Webhook、照合、プランのロール、収入がどう組み合わさるかを図で示しています。

- **プランと価格**: サーバーは、プランごとに API で Product を 1 つ作り（ID は `my_plan_<plan ID>`、`metadata.my_plan_id` 付き）、期間と通貨ごとに Price を 1 つ作ります（`lookup_key` は `my_<plan ID>_<month|year>_<currency>`）。
  Stripe の Price は金額を変えられないので、金額を変えると新しい Price を作り、古いものをアーカイブします。古い Price の加入は、その金額で更新されます。年払いの割引は年額の Price の金額で表し、クーポンは使いません。
  これらの Product と Price をダッシュボードで編集したり削除したりしないでください。
  プランには [Stripe の最低請求額の一覧](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts) にある通貨を使え、月額と年額はそれぞれその最低額に達していなければなりません。USD、AED、ARS、AUD、BRL、CAD、CHF、COP、CZK、DKK、EUR、GBP、HKD、HUF、IDR、ILS、INR、JPY、KRW、MXN、MYR、NOK、NZD、PHP、PLN、RON、RUB、SEK、SGD、THB、ZAR です。サーバーはそれ以外の通貨を拒否します。
  この一覧は、アカウントの決済通貨での請求にあてはまります。Stripe が換算する請求は、換算後に決済通貨の最低額を満たす必要があり、サーバーはそれを確認しません。そのため、プランの通貨が銀行口座の通貨と違うときは、プランの金額を最低額より十分に高くしてください。
- **加入**: サーバーは `mode=subscription` で Checkout セッションを作り、利用者のブラウザーをその URL（`checkout.stripe.com`）に送ります。支払いページは 1 時間で期限切れになります。
  サーバーの加入 ID は、`client_reference_id`、`metadata.my_subscription_id`、そして Stripe の subscription の `metadata.my_subscription_id` に入ります。自動の税計算（Stripe Tax）は有効にしません。
- **やり直し**: 保留中の加入 1 つにつき、支払いを受け付けられるページは 1 つまでです。同じプランと期間でやり直した利用者は、残り時間が 15 分未満でなければ、開いているページに戻り、そのページは再び空の状態で開きます。それ以外のやり直しでは新しいページになり、サーバーは前のページを期限切れにします。前のページには Stripe の期限切れのメッセージが表示されます。前のページですでに支払いが済んでいた場合、利用者にはページは渡されず、加入を表示する戻り先の画面に移ります。サーバーが前のページを期限切れにできない場合、やり直しは失敗し、利用者はもう一度試せます。
  保留中の加入が閉じた場合（プランが廃止された、アカウントが削除された）も、同じようにそのページを期限切れにします。
- **2 つ目の加入**: 両方のページが同時に送信されると、2 つの支払いが重なることがまだあります。先にサーバーに届いた加入がその利用者のものになります。サーバーはもう一方をすぐに解約し、どの期間にも何も提供していないので、更新分も含めて支払い済みのすべての請求書を理由 `duplicate` で全額返金し、`a second subscription was canceled and refunded` を記録します。閉じた保留中の加入への支払いも、閉じる直前にサーバーに届いた場合も含めて、同じように扱います。利用者は Stripe からの返金を受け取り、加入は 1 つだけ表示されます。Stripe は返金した請求の手数料を返さないので、その手数料は収入に表れます。
  異議申し立て中の請求は返金せず、サーバーが確認した時点で決済が済んでいなかった支払い（たとえば口座引き落とし）も返金しません。誰かがすでにダッシュボードで解約した加入は、請求も含めてそのままにし、利用者の加入にはなりません。
  **この機能のないバージョンからサーバーをアップグレードする前に**、Stripe で、`metadata.my_subscription_id` が同じ 2 つの subscription を持つ顧客を探してください。最初の照合で、使われていないほうを解約し、それが請求したものをすべて返金します。手作業で精算したいものはダッシュボードで解約しておけば、サーバーはそれに手を付けません。
- **カスタマーポータル**: 最初に使うときに、サーバーは API でポータルの設定（`metadata.my=1`）を作ります。この設定で有効になるのは、支払い方法の更新、請求書の履歴、期間の終わりでの解約だけです。プランの変更はポータルではなく、このサーバーの画面で行います。
  ポータルでの解約は、このサーバーでは解約予定として表示され、利用者はサーバーの画面から取り消せます。取り消すまで、プランの変更は拒否されます。
- **手作業で設定した解約日**: ポータルは subscription の `cancel_at_period_end` ではなく `cancel_at` を設定し、サーバーは両方を読みます。ダッシュボードや API で設定した `cancel_at` も、現在の期間の終わりより後でなければ同じように扱います。期間の終わりより前であれば、サーバーはその日を加入の終わりとして表示し、利用者はその日までロールを保ちます。それより後の `cancel_at` はまだ解約ではありません。その日を含む期間まで、加入は有効のままです。
- **プランの変更**: アップグレードと、月額から年額への変更は、差額を日割りですぐに請求し（`proration_behavior=always_invoice`）、その請求書が支払われたときにだけ切り替わります（`payment_behavior=pending_if_incomplete`）。ダウングレードと、年額から月額への変更は、Subscription Schedule を使って期間の終わりに切り替わります。
- **プランの変更の失敗**: 変更の支払いが失敗したときや、カードが認証を求めたときは、サーバーはその請求書を無効にして変更を破棄し、利用者にはエラーが表示されます。加入は元のプランのまま有効で、猶予期間も支払い失敗の通知もありません。利用者はポータルでカードを更新してもう一度試せ、請求は 1 回だけです。支払いがまだ処理中であれば、サーバーはそのままにし、成功したときにプランが切り替わります。更新には影響しません。更新の失敗は、下で説明するとおり猶予期間に入ります。
- **状態の読み戻し**: サーバーは Webhook を合図として扱い、subscription、Checkout セッション、お金の動きを Stripe の API から読み戻して状態を決めます。
  さらに、`payments_reconcile_interval`（既定値は 6 時間）ごとの照合で、イベントの一覧、完了していない Checkout セッション、残高の取引、すべての subscription の走査を読み、サーバーが受け取りそこねた Webhook を補います。
- **収入**: サーバーは、各残高取引の総額、手数料、純額を記録します。Billing の 0.7% のような手数料は別の `stripe_fee` 取引として現れ、収入から差し引かれます。
  `stripe_fee` はアカウント全体の手数料を含むので、**このサーバー専用の Stripe アカウントを使ってください**。ほかの販売にも使っているアカウントでは、それらの手数料も収入から差し引かれます。
- **日付**: 収入の日付は、Stripe アカウントのタイムゾーン（ダッシュボードの「設定 > ビジネス > アカウントの詳細」のタイムゾーン、API では `settings.dashboard.timezone`）で数えます。読み取れないときは、サーバーは UTC で数え、1 時間ごとに読み直します。

各サーバープロセスが送るリクエストは毎秒 20 件まで（テストモードでは 10 件）です。[Stripe の制限](https://docs.stripe.com/rate-limits) はアカウントごとで、本番モードでは毎秒 100 件、サンドボックスでは 25 件、エンドポイントごとに毎秒 25 件です。1 つのサーバーのプロセスはこれを共有し、そのアカウントを使うほかのものも同じ制限を共有します。
Stripe が `429` を返したときは、`Retry-After` が 30 秒以下であればその時間だけ待ち（ヘッダーがなければ 1 秒）、2 回まで再試行します。それを超える場合、再試行はジョブに任せます。

Stripe は読み取りのリクエストもアカウントごとに割り当てています。30 日間で取引 1 件あたり平均 500 件で、取引の件数にかかわらず少なくとも月に 10,000 件です。既定の間隔では、小さなサーバーの照合はこれを大きく下回ります。何も見つからない照合でも数ページは読むので、最短の 15 分間隔（30 日で 2,880 回）では、取引の少ないアカウントで 10,000 件に近づきます。
Stripe は、subscription 1 つにつき新しい請求書を 1 分に 10 件、1 日に 20 件までに制限しており、すぐに請求するプランの変更では新しい請求書が作られます。

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## カードのデータと PCI DSS

支払い方法は Stripe の Checkout とカスタマーポータルでだけ入力し、このサーバーの Web クライアントは Stripe のスクリプトも iframe も読み込みません。
サーバーが保存するのは、Stripe のオブジェクト ID（`cus_`、`sub_`、`price_`、`prod_`、`cs_`、`txn_`）、金額、期間、状態だけです。カード番号、下 4 桁、有効期限、請求先住所は保存しません。Webhook の本文と Stripe の応答は、ログにもデータベースにも残しません。
そのため、運営者の PCI DSS の対象範囲は、ホストされた支払いページを使う加盟店のもの（SAQ A）になると見込まれます。あてはまる対象範囲と自己評価問診票の種類は、Stripe のダッシュボードの案内（「設定 > コンプライアンス」）で確認してください。

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. Stripe アカウントを用意する

1. Stripe アカウントを作り、ダッシュボードで事業の情報を入力します。日本で販売する場合は、「設定 > ビジネス > 公開情報 > 特定商取引法に基づく表記」に特定商取引法に基づく表記のページの URL を登録すると、Checkout の下にリンクが表示されます。
2. 「設定 > ビジネス > アカウントの詳細」でタイムゾーンを確認します。収入の日付はこのタイムゾーンで数えます。
3. 「設定 > Billing > サブスクリプションおよびメール > 支払い失敗の管理」で、再試行の動作（Smart Retries）と、再試行を使い切ったときの動作を選びます。
   ロールを取り除くまでの猶予期間（最初の失敗から 7 日）はこのサーバーが数えるので、Stripe の再試行の設定には左右されません。再試行を使い切ったときは「サブスクリプションをキャンセル」をおすすめします。
4. 加入者に領収書を送るには、「設定 > 顧客へのメール」で支払い成功時の領収書を有効にします。

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. API キーを作る

「開発者 > API キー」で、**制限付きのキー**（`rk_test_...` / `rk_live_...`）を作ることをおすすめします。シークレットキー（`sk_test_...` / `sk_live_...`）でも動きます。
キーの接頭辞でモードが決まり、テスト用のキーでは何も請求されません。公開可能キー（`pk_`）は使えません。

制限付きのキーには、次の権限を与えてください。この表の権限を持つ制限付きのキーで、加入、プランの変更、解約、照合、請求書の無効化をサンドボックスで実行しました。返金、ページの期限切れ、異議申し立てはこのキーでは実行しておらず、この表が動作する最小の組み合わせかどうかは誰も確認していません。

| リソース | 権限 | 使う API |
| --- | --- | --- |
| Products | 書き込み | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | 書き込み | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | 書き込み | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | 書き込み | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | 書き込み | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules（Subscriptions の一部） | 書き込み | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | 書き込み | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | 書き込み | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | 読み取り | 残高の取引と請求を通じて読みます |
| Balance transaction sources | 読み取り | `GET /v1/balance_transactions` |
| Events | 読み取り | `GET /v1/events` |
| Account（Connect 以外のアカウントの情報） | 読み取り | `GET /v1/account`（タイムゾーン） |

返金の書き込み権限がなくても、サーバーは 2 つ目の加入を解約しますが、返金できるようになるまで `duplicate_charge` が警告に残ります（[Webhook のエンドポイントを登録する](#3-register-the-webhook-endpoint) を参照）。この権限が一覧に載る前に作ったキーには、この権限を加える必要があります。

キーは環境変数 `MY_PAYMENTS_STRIPE_SECRET_KEY`（またはファイルのパスを `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`）で渡します。設定ファイルには書けません。
キーが漏れた場合は、ダッシュボードでローテーションし、新しいキーで再デプロイしてください。

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. Webhook のエンドポイントを登録する

「開発者 > Webhook > エンドポイントを追加」で、次の内容を登録します。サーバーは API でエンドポイントを作りません。

- URL: `https://<server domain>/payments/webhooks/stripe`
- イベントの送信元: 「お客様のアカウント」（連結アカウントではありません）
- ペイロードの形式: スナップショット。サーバーが読むのは本文の `id`、`type`、`livemode`、`created`、`data.object.id`、`data.object.object` だけなので、エンドポイントの API バージョンは問いません。
- 送信するイベント（16 種類）:

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

エンドポイントの署名シークレット（`whsec_...`）は、環境変数 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`（または `_FILE`）で渡します。

エンドポイントは署名（`Stripe-Signature` の HMAC-SHA256）とタイムスタンプ（前後 5 分以内）を検証し、本番用のキーで受け取ったテストのイベント（とその逆）を `400` で拒否します。
各イベントを ID で一度だけ記録し、ジョブをキューに入れて `2xx` を返します。データベースに書き込めないときは `503` を返し、Stripe がイベントを送り直します。本文の上限は 1 MiB です。
署名シークレットをローテーションしている間、Stripe は古いものと新しいものの両方で署名するので、新しいシークレットを設定に入れるだけで済みます。

事業者の状態（モード、最後に Webhook が届いた時刻、最後の照合の結果と失敗の種類）は、管理画面の `/admin/supporters` の「決済の事業者」の節と、管理 API の `GET /api/v1/admin/payment_status`（権限 `manage_supporters`）に表示されます。この節では、照合の失敗の種類ごとに直し方を説明しています。
`warnings` には、`no_webhook`（Webhook が一度も記録されていない）と `reconciliation_corrected`（最後の照合で加入を補正した）が載ります。どちらかが出たら、Webhook の設定の誤りを疑ってください。
`duplicate_charge` も載ります。2 つ目の加入がまだ解約・返金されていないことを表します。[運用](#7-operation) を参照してください。
この節の「今すぐ照合する」（`POST /api/v1/admin/payment_reconciliation/runs`）は、予定された実行を待たずにすぐ照合します。UTC の 1 時間に 1 回までに制限されており（同じ時間内の 2 回目の呼び出しは `429` を返します）、監査ログに `payment_reconcile_queued` が記録されます。

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. サーバーを設定する

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` で設定を検証できます。ビルドタグ `no_payments_stripe` でビルドしたバイナリは `payments = stripe` を拒否します（[build-tags.md](../reference/build-tags.md) を参照）。

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. テストモードで試す

テスト用のキー（`rk_test_` / `sk_test_`、またはサンドボックスのキー）から始めてください。テストモードでは何も請求されません。
テスト用のキーと本番用のキーでは、Product、Price、顧客、subscription、Webhook のエンドポイントが別々です。本番に移るには、本番モードでエンドポイントを登録し直し、キーと署名シークレットを本番用のものに置き換えて再デプロイします。プランの Price は本番のアカウントで新しく作られます。

テスト用のカード（[Stripe のドキュメント](https://docs.stripe.com/testing)）:

| 番号 | 動作 |
| --- | --- |
| `4242 4242 4242 4242` | 成功します |
| `4000 0000 0000 0341` | 登録はできますが請求は失敗します（支払いの失敗と猶予期間の始まりを確かめるため） |
| `4000 0000 0000 9995` | 残高不足で失敗します |
| `4000 0000 0000 0259` | 支払いのあとに異議申し立てが作られます |

有効期限には未来の任意の日付を、セキュリティコードには任意の 3 桁を使ってください。

更新と支払いの失敗を試すには、[テストクロック](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage) を使います。Checkout が作った顧客には作成時にクロックがないので、先に加入してから、API でその顧客のクロックを作ります。`POST /v1/test_helpers/test_clocks` に `customer` を渡してください（API バージョン `2026-05-27.dahlia` 以降）。
Stripe がこれを許すのは、`frozen_time` が過去でなく、アカウントに Automations が設定されておらず、顧客の subscription が 3 つまでのときだけです。顧客はあとからクロックを外せず、クロックを削除すると顧客とその subscription も削除されます。ダッシュボードの「Billing > テストクロック」で時間を進めると、更新の請求と失敗が起こります。

クロックでは確かめられないこともあります。

- **猶予期間の終わり**: サーバーは、実際の時刻と、Stripe が失敗について報告する時刻（`finalized_at`）を比べます。後者はシミュレートされた時刻で、実際の時刻より先に進んでいます。加入は猶予期間に入りますが、クロックをさらに進めても猶予期間は終わりません。ロールは、シミュレートされた失敗から実際の時刻で 7 日が過ぎるまで、またはアカウントがそう設定されていれば、再試行を使い切って Stripe が subscription を解約するまで残ります。
- **すべての subscription の走査**: Stripe の subscription の一覧は、リクエストでクロックか顧客を指定しない限り、クロック上の subscription を含めません。サーバーの走査はどちらも指定しません。それらの変更をサーバーに届けるのは、Webhook とイベントの一覧だけです。

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. ローカルで試す（Stripe CLI）

[Stripe CLI](https://docs.stripe.com/stripe-cli)（Apache-2.0）は、Stripe のイベントを手元のマシンのサーバーに転送します。公開の URL は要りません。

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

`stripe listen` が表示する `whsec_...` を `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` に入れて、サーバーを起動します（ポートは `listen` の設定に合わせてください）。

Docker Compose では、`deploy/docker/.env` に `MY_PAYMENTS=stripe`、`MY_PAYMENTS_STRIPE_SECRET_KEY`、`MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` を設定してから、`docker compose up -d` を実行します。

別のターミナルで、イベントを 1 つずつ発生させられます。

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

`stripe trigger` が作るオブジェクトには、このサーバーの加入 ID が入っていないので、どの加入の状態も変わりません（サーバーはそれらを読み戻し、自分の加入でないものは無視します）。加入の流れ全体を確かめるには、Web クライアントから加入し、テスト用のカードで支払ってください。

<!-- source: 7-operation f70cd2967b9f -->
## 7. 運用

- 種類が `permission` の照合の失敗は、キーの誤りか権限の不足を意味します。サーバーのログ行（`payment reconciliation failed`）には Stripe のメッセージが入っており、加えるべき権限が書かれています。
- 照合がリクエストの上限によく達するときは、`payments_reconcile_max_requests` を上げるか、`payments_reconcile_interval` を長くしてください。読み取りはどれも Stripe の読み取りの割り当てに数えられるので（[サーバーが Stripe で行うこと](#what-the-server-does-at-stripe) を参照）、取引の少ないアカウントでは間隔を長くするほうを選んでください。
- 照合が 30 日を超えて止まっていた場合、サーバーは Stripe のイベントの一覧（30 日前までさかのぼれます）からは追いつけません。その場合は、すべての subscription の走査が状態を補います。
- 警告の `duplicate_charge` は、利用者が 2 回支払い、サーバーが 2 つ目の加入の解約と返金を終えられなかったことを意味します。ログ行 `a second subscription could not be canceled and refunded` には両方の subscription と失敗の種類が書かれ、照合のたびに再試行されるので、そのたびにまた現れます。種類が `permission` の場合は、返金の書き込み権限を加えてください。それ以外の場合は、ダッシュボードで請求を返金してください。次の試行で返金するものが残っていないことがわかり、警告が消えます。「今すぐ照合する」ですぐに試せます。
- そのほかの返金とチャージバックは、Stripe のダッシュボードで扱います。全額返金とチャージバックでは、ロールが取り除かれ、加入がすぐに解約されます。一部返金ではロールは取り除かれません。異議申し立てに勝っても、ロールは自動では戻りません。

<!-- source: items-to-verify 8960333fa514 -->
## 確認が必要な項目

次のことは、サンドボックスでも本番でも Stripe に対して試しておらず、Stripe のドキュメントに基づいています。

- アーカイブした Price の加入の更新。その Price のままになるはずです。
- 実際の更新での `status_transitions.finalized_at`。サーバーはこれを支払いが最初に失敗した時刻とみなし、そこから猶予期間を数えます。
- `stripe_fee` の取引を加入に割り当てられるかどうか。サーバーは試みず、これらの手数料を収入全体から差し引きます。
- 制限付きのキーで `GET /v1/account` の `settings.dashboard.timezone` を読めるかどうか。読み取りに失敗してもサーバーは止まらず UTC で数えるので、動いているサーバーからはわかりません。収入の日付がアカウントのタイムゾーンになっていることを確かめてください。
- `GET /v1/invoice_payments` が `payment[payment_intent]` で絞り込めること。これで支払いをその加入に結び付けます。
- 全額返金、一部返金、チャージバックの、ダッシュボードでの操作からサーバーでの状態まで。
- すべての subscription の走査が、サーバーが状態を受け取りそこねた加入を補うこと。
- 保留中の更新が対象にしない支払い方法（ACH、SEPA、Bacs の口座引き落とし）で支払っている加入で、アップグレードが何をするか。これが確かめられるまで、ダッシュボードでこれらの支払い方法は無効にしておいてください。
- `cancel_at_period_end=true` が、後の期間にある `cancel_at` を置き換えること。
- 開いているページに戻ること。サーバーは、`GET /v1/checkout/sessions/{id}` が返すもの（`line_items`、`success_url`、`cancel_url`、`customer`、`customer_email`）を新しいリクエストと比べ、違いがあれば新しいページを作ります。Stripe が保存するときに `success_url` や `customer_email` を書き換えると、利用者は毎回新しいページを受け取って入力した内容を失いますが、支払いは 1 回だけです。
- 支払いが完了するのと同時にセッションを期限切れにすること。Stripe のドキュメントには、開いていないセッションはエラーを返すとあり、その場合サーバーはセッションを読みます。そのエラーの正確なコードと、カードの認証が進行中のときの状態は文書化されていません。
- 1 つのセッションに 2 つのタブから 2 回支払えないこと。セッションは 1 つの subscription を持ち、一度だけ完了しますが、同時に 2 回送信された場合についてはドキュメントに書かれていません。
- 支払いページでの認証の失敗が、サーバーの加入を指す `incomplete` の subscription を残すかどうか。残る場合、サーバーはそれを結び付けて利用者を保留のままにし、別のページでの支払いは 2 つ目として返金されることになります。
- 制限付きのキーの「Charges」と「Refunds」の権限が表のとおりの場合の、2 つ目の加入の返金と、それに続くお金の動き。
