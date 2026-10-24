### 4.12 finance

サーバーの維持費用と収入（FR-901・FR-902・FR-904・FR-917、[ADR-0067](../../../adr/0067-finance-cost-records.md)・[ADR-0068](../../../adr/0068-billing-import.md)・[ADR-0073](../../../adr/0073-income-records-and-publication.md)・[ADR-0098](../../../adr/0098-cost-provider-display.md)）。`internal/app/finance` が所有します。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `cost_items` | リソース | `id`, `schedule`（`enum(once, weeks, months, years)`）, `interval`（`int`）, `starts_on`（`date`。単発は支払日）, `created_by_user_id`（NULL 可 = CLI）, `created_at` | `CHECK`: `once` は `interval = 1`、ほかは 1〜99 |
| `cost_item_label_revisions` | イベント（版） | `id`, `cost_item_id`, `name`（`string(100)`）, `provider`（`string(100)`）, `provider_normalized`, `category`（`enum(compute, database, storage, network, domain, email, software, other)`）, `memo`（`text`。なしは `''`。公開しない）, `revised_by_user_id`（NULL 可）, `revised_at` | インデックス `(cost_item_id, id DESC)` |
| `cost_item_price_revisions` | イベント（版） | `id`, `cost_item_id`, `effective_on`（`date`）, `amount_minor`（`bigint`）, `currency`（`string(3)`。ISO 4217）, `revised_by_user_id`（NULL 可）, `revised_at` | `amount_minor >= 0`。インデックス `(cost_item_id, id)` |
| `cost_item_end_changes` | イベント | `id`, `cost_item_id`, `ends_on`（`date`。NULL 可 = 無期限）, `changed_by_user_id`（NULL 可）, `changed_at` | インデックス `(cost_item_id, id DESC)` |
| `cost_item_cancellations` | イベント | `id`, `cost_item_id`, `canceled_by_user_id`（NULL 可）, `canceled_at` | 一意 `(cost_item_id)` |
| `cost_item_heads` | 投影 | `cost_item_id`（PK）, `label_revision_id`, `ends_on`（`date`。NULL 可 = 無期限） | 行がある = 取り消されていない |
| `billing_import_runs` | イベント | `id`, `source`（`enum(aws, gcp)`）, `run_key`（ジョブの冪等キー）, `outcome`（`enum(succeeded, failed)`）, `error_kind`（`enum(none, permission, not_enabled, not_found, throttled, request_cap, bytes_cap, invalid_response, unavailable)`）, `requests`（`int`）, `bytes_billed`（`bigint`）, `occurred_at` | `CHECK (outcome = 'succeeded') = (error_kind = 'none')`。インデックス `(source, occurred_at)`, `(run_key)` |
| `imported_cost_revisions` | イベント（版） | `id`, `source`, `billing_month`（`string(7)`。`YYYY-MM`）, `service`（`string(200)`）, `charge_type`（`enum(usage, credit, refund, tax, support, fee, adjustment, other)`）, `category`（`cost_item_label_revisions` と同じ enum）, `starts_on`・`ends_on`（`date`）, `amount_minor`（`bigint`。負も可）, `currency`（`string(3)`）, `kind`（`enum(estimate, final, correction)`）, `run_id`, `run_key`, `recorded_at` | 一意 `(source, billing_month, service, charge_type, run_key)`。`starts_on <= ends_on` |
| `imported_cost_heads` | 投影 | `(source, billing_month, service, charge_type)`（PK）, `revision_id` | 版を足すトランザクションで同期に更新する |
| `income_movements` | イベント | `id`, `provider`（`enum(stripe)`）, `provider_id`（`string(255)`。Stripe は balance transaction の ID）, `kind`（`enum(payment, refund, chargeback, chargeback_reversal, fee)`）, `parent_movement_id`（NULL 可。返金・チャージバック・その取り消しの元の `payment` の行）, `subscription_id`（NULL 可 = 加入に結び付かない手数料。加入への論理参照で外部キーを付けない）, `occurred_at`, `occurred_on`（`date`。事業者のレポートのタイムゾーンでの日付）, `covers_starts_on`・`covers_ends_on`（`date`。NULL 可。`payment` の請求の期間）, `gross_minor`・`fee_minor`・`net_minor`（`bigint`。負も可）, `currency`（`string(3)`）, `recorded_at` | 一意 `(provider, provider_id)`。`net_minor = gross_minor - fee_minor`。`covers_*` は両方 NULL か `payment` で `starts_on <= ends_on`。`parent_movement_id` は返金・チャージバック・取り消しだけ。インデックス `(occurred_on)`, `(subscription_id, occurred_at)` |

- 作成は項目・最初のラベルの版・`starts_on` から効く最初の価格の版（と終了日を渡したときは終了の変更）を 1 つのトランザクションで書き、`finance.cost_item_changed` を発行します。
  ラベルの変更と終了の変更も `finance.cost_item_changed` を発行し、その射影が `cost_item_heads` を書きます。取り消しは `finance.cost_item_canceled` を発行し、射影が行を消します。
  価格の改定は投影を持たず、読み取りが項目の改定をすべて読みます。
- 周期と開始日は項目の不変の事実です。周期を変えるときは、項目を終了して新しい項目を作ります。改定の `effective_on` と終了日は開始日より前にできません（`finance.ValidateDates`）。
- 支払日は開始日から数えます（`finance.Recurrence.Occurrence`）。月ごとはその月にない日を月の最終日に、年ごとの 2/29 はうるう年でない年の 2/28 にします。終了日は含みます。
  各回の価格は、その日以前で最も新しい `effective_on` の改定で、同じ日の改定は最後に記録した行（ID の最も大きい行）です。
- 週・月・年の集計は投影に持たず、読み取りのたびに取り消されていない項目をすべて読んで `internal/domain/finance` の純粋な関数（`Payments`・`Entries`・`Spread`・`Summarize`）で計算します（ADR-0067 §1）。
  按分（既定）は 1 か月より長い周期（N ≥ 2 の月、年、N ≥ 5 の週）の各回を、その回が覆う日（次の支払日の前日まで）に累積の丸めで日割りし、現金主義と 1 か月以下の周期は支払日に載せます。
  期間は暦の日付で区切り、週は ISO 8601（月曜〜日曜）です。合計は通貨ごとで、内訳は `provider_normalized`・区分・通貨でまとめます。
- 金額は通貨の最小単位の整数で、小数の桁は `internal/domain/finance` の ISO 4217 の表（`finance.Currency.MinorUnits`。JPY は 0、USD は 2）にあります。表は ISO 4217 の現行の通貨と基金のコードのうち、小数の桁を持つもの（N.A. の貴金属・SDR・試験用のコードを除く）です。
- `provider_normalized` は事業者の NFKC・小文字の形（`finance.NormalizeProvider`）で、表記の揺れ（全角・大文字）を内訳で 1 つにまとめます。まとめるキーで、表示には使いません。
  内訳の行と項目の行に出す事業者は、期間ごと・`provider_normalized` ごとに 1 つの表記で、その期間に支払いが載る項目のうち、いまのラベルの版の `revised_at` が最も新しい（同じなら版の `id` が大きい）項目の `provider` です（ADR-0098）。
  取り込んだ費用の表記は取り込みの事業者の名前（`AWS`・`Google Cloud`）で、同じ期間・同じ `provider_normalized` に手入力の項目があればその表記を優先します。
- 利用料金の取り込み（ADR-0068）は、`billing_sources` の事業者ごとの定期ジョブ `finance.billing_import`（24 時間ごと）が `BillingSource` ポートで事業者の請求月の費用を読み、試行ごとに `billing_import_runs` の行を 1 つ足します。
  成功した試行は、キー `(source, billing_month, service, charge_type)` ごとに最新の版と金額・通貨・確定の別・`starts_on`〜`ends_on` を比べ、違うキーだけ版を足して `imported_cost_heads` を同じトランザクションで更新します。取り直した月で事業者が返さなくなったキーは金額 0 の版になります。
  版の種類は、キーの最新の版が `final` か `correction` なら `correction`、事業者が月を確定と返せば `final`、それまでは `estimate` です（`finance.NextRevisionKind`）。同じジョブの再試行は `run_key` の一意制約で版を重ねません。
- 取り直す月は事業者のタイムゾーンでの今月と、15 日まで、または確定するまでの前月です（`finance.RefetchMonths`）。AWS の課金される要求は `billing_import_runs.requests` を UTC の日と暦月で合計して上限（`billing_aws_max_requests_per_day`・`_per_month`）で止め、上限に達した試行は事業者を呼ばずに `request_cap` として記録します。
- 集計は、取り込んだ費用の各キーの最新の版を `starts_on`〜`ends_on` の日数に、現金主義でも按分でも日割りで載せ、`estimate` の版が載る期間を見込みとします（ADR-0068 §3）。

- 収入（ADR-0073）は、決済の事業者で額が確定したお金の動きを 1 件 1 行で `income_movements` に記録します。加入のモジュール（`supporters`）の webhook と照合のジョブが `finance.RecordIncome` を呼び、
  `(provider, provider_id)` の一意で同じ動きは何度渡されても 1 行です（挿入の衝突は何もしない）。返金・チャージバック・その取り消しは元の支払いの行を `parent_movement_id` に持ち、元の行がまだなければ記録せずにジョブの再試行に任せます。
  加入の ID は事業者の加入をこちらの加入に写したもので、こちらの加入でなければ NULL です。行は利用者を持たず、加入者のアカウントは管理画面だけが `finance.SupporterDirectory`（`internal/platform` が `supporters` で実装する）で引きます。
- 収入の集計は投影に持たず、期間に載りうる行（期間の日の行、期間に掛かる請求の期間の支払い、期間の終わりまでの返金などのうち元の支払いの請求の期間が期間に掛かるもの）を読んで `finance.IncomeTotals` で計算します。
  按分では 31 日より長い請求の期間の支払いを日割りし、その返金・チャージバック・取り消しは、元の支払いの日割りのうち自分の日より前の分を自分の日にまとめて載せ、残りを元の支払いと同じ日割りで打ち消します（ADR-0073 §5）。
  総額と net を同じ規則で載せ、手数料は期間の総額 − net です。収支は同じ期間・同じ見方の費用の合計を通貨ごとに引きます（`finance.Balance`）。
