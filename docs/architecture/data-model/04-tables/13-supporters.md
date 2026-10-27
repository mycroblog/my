### 4.13 supporters

決済の事業者の事実を運ぶ経路の記録（FR-910・FR-911、[ADR-0074](../../../adr/0074-payment-provider-port.md) §3・§4）。`internal/app/supporters` が所有します。
加入・顧客・価格・加入の状態の表は [ADR-0072](../../../adr/0072-supporter-plans-and-roles.md) §15 が決めます。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `payment_webhook_receipts` | イベント | `id`, `provider`（`enum(stripe, payjp)`）, `event_id`（`string(255)`）, `event_type`（`string(100)`）, `object_id`（`string(255)`）, `received_at` | 一意 `(provider, event_id)`。インデックス `(provider, received_at)`, `(received_at)`。本文は持たない |
| `payment_reconcile_runs` | イベント | `id`, `provider`, `run_key`（ジョブの冪等キー）, `outcome`（`enum(succeeded, partial, failed)`）, `error_kind`（`enum(none, request_cap, permission, not_found, invalid_request, throttled, unavailable, invalid_signature, stale_signature, mode_mismatch, invalid_response)`）, `changes_until`, `changes_cursor`・`money_cursor`・`sweep_cursor`（`string(255)`。`''` は先頭）, `requests`・`checked_subscriptions`・`corrected_subscriptions`（`int`）, `occurred_at` | 一意 `(provider, run_key)`。`CHECK (outcome = 'succeeded') = (error_kind = 'none')`, `(outcome = 'partial') = (error_kind = 'request_cap')`。インデックス `(provider, occurred_at)` |

- webhook の受け口 `POST /payments/webhooks/{provider}` は、本文を 1 MiB まで読み、署名・時刻・モードを `PaymentProvider.ParseWebhook` で確かめ、1 つの短いトランザクションで `payment_webhook_receipts` の行とジョブ `supporters.payment_event`（冪等キー `payment_event:<provider>:<event_id>`、順序のキーは対象のオブジェクト）を書きます。
  同じイベントの行がもうあれば何も書きません。ジョブはイベントの本文の状態を使わず、事業者からサブスクリプション・申し込み・お金の動きを取り直して `Syncer`（`supporters.sync`）に渡すので、重複・順序の入れ替わり・取りこぼしは同じ扱いになります。
- 定期ジョブ `supporters.payment_reconcile`（`payments_reconcile_interval` ごと）は、試行ごとに `payment_reconcile_runs` の行を 1 つ足します。最新の `succeeded` か `partial` の行のカーソルから、変わったもの（`changes_until` の 5 分前から）・未完了の申し込み・お金の動き・全件の見回しの 4 段を読み、要求が `payments_reconcile_max_requests` に達した段でカーソルを記録して `partial` で止まり、同じトランザクションで続きのジョブ（冪等キー `continue:<run_id>`、1 分後）を積みます。
  見回しのカーソルは最後のページを読むと `''` に戻ります。カーソルは試行ごとの行に残し、上書きする現在値の列を持ちません。事業者の失敗は `failed` として記録し、次の照合は前の `succeeded` か `partial` の行から始めます。
- `payment_webhook_receipts` は 45 日を過ぎた行を毎日の定期ジョブ `supporters.payment_receipt_purge` が消します。

#### サポーターの課金・プラン・加入

課金の有効化・プラン・事業者の価格・顧客・加入・加入の状態・猶予・掲載の選択（[ADR-0072](../../../adr/0072-supporter-plans-and-roles.md) §15）。投影はすべて同期で、`my projection` の `supporters` が記録から作り直します。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `supporter_program_changes` | イベント | `id`, `state`（`enum(enabled, disabled)`）, `provider`（`enum('', stripe, payjp)`。有効にしたときの事業者）, `changed_by_user_id`（NULL 可 = CLI）, `changed_at` | 最新の行が今の状態。行がなければ無効。`CHECK (state = 'enabled') = (provider <> '')` |
| `supporter_plans` | リソース | `id`, `role_id`, `currency`（`string(3)`）, `created_by_user_id`（NULL 可）, `created_at` | 一意 `(role_id)` |
| `supporter_plan_revisions` | イベント（版） | `id`, `plan_id`, `name`（`string(50)`）, `description`（`string(1000)`）, `rank`（`int`、0〜1,000,000）, `monthly_amount_minor`（`bigint`、1 以上）, `annual_discount_percent`（`int`、0〜90）, `emoji_limit`（`int`、0〜10,000）, `visibility`（`enum(public, hidden)`）, `revised_by_user_id`（NULL 可）, `revised_at` | インデックス `(plan_id, id DESC)` |
| `supporter_plan_retirements` | イベント | `plan_id`（PK）, `retired_by_user_id`（NULL 可）, `retired_at` | |
| `supporter_plan_heads` | 投影 | `plan_id`（PK）, `revision_id`, `rank`, `visibility` | 廃止していないプランだけ。一意 `(rank)` |
| `supporter_plan_prices` | リソース | `id`, `plan_id`, `interval`（`enum(month, year)`）, `amount_minor`, `currency`, `provider`, `provider_price_id`, `created_at` | 一意 `(provider, provider_price_id)`。同じプラン・間隔・金額の価格を何本でも持てる（以前の金額に戻すと、事業者が作った新しい価格が古い価格と並ぶ） |
| `supporter_plan_price_heads` | 投影 | `plan_id`, `interval`, `price_id` | PK `(plan_id, interval)`。新しい加入と変更に使う価格。アーカイブされていない最新の価格 |
| `supporter_plan_price_archivals` | イベント | `price_id`（PK）, `archived_at` | |
| `supporter_customers` | リソース | `id`, `user_id`, `provider`, `provider_customer_id`, `created_at` | 一意 `(provider, user_id)`, `(provider, provider_customer_id)` |
| `supporter_subscriptions` | リソース | `id`（事業者に載せるこちらの ID）, `user_id`, `plan_id`, `price_id`（開始のとき）, `provider`, `created_at` | インデックス `(user_id, id DESC)` |
| `supporter_checkouts` | イベント | `id`, `subscription_id`, `provider`, `provider_checkout_id`, `expires_at`, `created_at` | 一意 `(provider, provider_checkout_id)`。インデックス `(subscription_id, id DESC)`, `(provider, expires_at)` |
| `supporter_subscription_links` | イベント | `subscription_id`（PK）, `provider`, `provider_subscription_id`, `linked_at` | 一意 `(provider, provider_subscription_id)` |
| `supporter_subscription_transitions` | イベント（版） | `id`, `subscription_id`, `state`（`enum(pending, active, canceling, past_due, lapsed, expired, revoked, abandoned)`）, `plan_id`, `price_id`, `period_ends_at`（`pending`・`abandoned` だけ NULL）, `reason`（`enum(started, synced, grace_expired, reversed, plan_retired, account_closed, account_suspended, staff, service_closed)`）, `occurred_at` | インデックス `(subscription_id, id DESC)` |
| `supporter_grace_periods` | イベント | `transition_id`（PK。`past_due` の遷移）, `failed_at`, `ends_at`（`failed_at` + 7 日） | |
| `supporter_scheduled_changes` | イベント | `transition_id`（PK）, `price_id`, `effective_at` | 行がなければ予約なし |
| `supporter_subscription_reversals` | イベント | `id`, `subscription_id`, `kind`（`enum(full_refund, partial_refund, chargeback)`）, `provider`, `provider_reversal_id`, `occurred_at` | 一意 `(provider, provider_reversal_id)` |
| `supporter_subscription_heads` | 投影 | `subscription_id`（PK）, `transition_id`, `user_id`, `plan_id`, `state` | インデックス `(plan_id, state)`, `(user_id)` |
| `supporter_live_subscriptions` | 投影 | `user_id`（PK）, `subscription_id` | 終端でない加入だけ。利用者ごとに 1 つ |
| `supporter_subscription_deadlines` | 投影 | `subscription_id`（PK）, `kind`（`enum(checkout, period_end, grace)`）, `due_at` | インデックス `(due_at)` |
| `supporter_subscription_reads` | 現在値 | `subscription_id`（PK）, `read_at` | 当てた最新のスナップショットを事業者から読み始めた時刻 |
| `supporter_listings` | 現在値 | `user_id`（PK）, `created_at` | 参加者の一覧に出る選択（ADR-0071 §6） |

- 状態は、webhook・決済画面からの戻り・Scheduler の期限・定期の照合のどれも、事業者からサブスクリプションを取り直して `supporters.Derive` で導きます（`Syncer`、ジョブ `supporters.sync`）。
  導いた状態か、プラン・価格・期間の終わり・猶予・予約された変更が直前の遷移と違うときだけ遷移の行を足し、同じトランザクションで投影と期限、プランのロールの付与・剥奪・入れ替え（`identity` の管理されたロールの操作）を書きます。ロールが外れる遷移は `supporter_listings` の行も消します。
  スナップショットには読み始めた時刻を付け、加入の行をロックしたトランザクションで `supporter_subscription_reads` の時刻より前に読んだものは当てません。同時に読んだ古い状態が、先に当てた新しい状態を上書きしないためです。
- 掲載の選択（`supporter_listings`）は、加入中（`active`・`canceling`・`past_due`）の本人が `PUT /preferences/supporter_listing` で入れ、取り消すと消えます。入れる処理は加入の行をロックして状態を確かめるので、ロールが外れる遷移と入れ違っても加入の終わった人の行は残りません。
  一覧（`GET /instance/supporters`）は、行と加入中の加入の投影と廃止していないプランの投影を結合し、プランの順位の順・載せた新しい順に読みます。
- 期限は、`pending` が開始から 24 時間（`checkout`）、`active`・`canceling` が期間の終わり（`period_end`）、`past_due` が猶予の終わり（`grace`）です。5 分ごとのジョブ `supporters.deadlines` が期限の来た行に `supporters.sync` を積みます。
- 事業者への命令（すぐ解約・期間の終わりの解約）は、遷移のトランザクションで積むジョブ `supporters.command` がコミットの後に送り、取り直したスナップショットを当てます。全額の返金とチャージバック、アカウントの削除（BAN の予約を含む）、管理者による解約は、こちらの状態を先に終端にしてから事業者をすぐ解約します。凍結は期間の終わりの解約を予約します。
- プランの作成と、条件の版を足す変更は、同じトランザクションでジョブ `supporters.ensure_prices`（冪等キー `ensure_prices:<版の ID>`）を積みます。ジョブは事業者に月と年の価格を作らせ（`EnsurePlan`）、`supporter_plan_prices` と今の価格、アーカイブを記録します。今の価格は、事業者が今の条件に返した価格で、プランと間隔ごとにアーカイブされていない最新の価格（作った順）と一致します。金額を以前の金額に戻しても新しい価格として記録するので、プランは毎回使える状態になります。既存の加入はもとの価格のまま続きます。
- 決済画面の行（`supporter_checkouts`）は ADR-0072 §15 の表にない追加で、`pending` の加入の取り直しと定期の照合（`Syncer.OpenCheckouts`）が読みます。
