# ADR-0067: 費用と収入の記録・集計・公開

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #873, #883, #914, #925, #1206, #2612

## Decision（決定）

### 1. モジュール

モジュール `finance`（`internal/domain/finance`・`internal/app/finance`）を置き、費用の項目・改定・終了・取り消しと、取り込んだ費用（ADR-0068）の行と、収入の行（§11）を所有する。
`instance` は設定・規約・テーマなどサーバーが出す中身を持つモジュールで、取り込みのポート（`BillingSource`）と定期ジョブを足すと責務が混ざる。

- 依存の向き（ADR-0002）: 公開範囲の設定は `internal/interface/clientapi` がインスタンス設定から読む。`internal/app/finance` からの読み取りはポート `SupporterDirectory` にし、監査ログは `AuditRecorded` のドメインイベントで `moderation` に渡す。ほかのモジュールから `finance` を呼ぶ向きは「加入のモジュール → `finance`」で、`RecordIncome`・`KnownIncome` を呼ぶ（§11）。
- 期間の展開・按分・集計は `internal/domain/finance` の純粋な関数にし、表駆動のテストで確かめる。
- 集計は読み取りのときに計算し、期間ごとの合計の投影を持たない。項目が数十件の規模では全項目を読んで展開しても軽く、公開の API は 60 秒のキャッシュを持つ。
- `finance` は overview §2 のモジュールの表、data-model §4.0 の所有の表、`.golangci.yml` の depguard に載せる。

### 2. データモデル（ADR-0020）

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `cost_items` | リソース | `id`, `schedule`（`enum(once, weeks, months, years)`）, `interval`（`int`。`once` は 1、ほかは 1〜99。`CHECK`）, `starts_on`（`date`。単発は支払日）, `created_by_user_id`（NULL 可 = CLI）, `created_at` |
| `cost_item_label_revisions` | イベント（版） | `id`, `cost_item_id`, `name`（`string(100)`）, `provider`（`string(100)`）, `provider_normalized`, `category`（enum。下記）, `memo`（`text`。公開しない）, `revised_by_user_id`（NULL 可）, `revised_at` |
| `cost_item_price_revisions` | イベント（版） | `id`, `cost_item_id`, `effective_on`（`date`。この日以降の支払いに効く）, `amount_minor`（`bigint`、0 以上）, `currency`（`string(3)`。ISO 4217）, `revised_by_user_id`（NULL 可）, `revised_at`。最初の行の `effective_on` は `starts_on` |
| `cost_item_end_changes` | イベント | `id`, `cost_item_id`, `ends_on`（`date`。NULL 可 = 無期限に戻す）, `changed_by_user_id`（NULL 可）, `changed_at` |
| `cost_item_cancellations` | イベント | `id`, `cost_item_id`（一意）, `canceled_by_user_id`（NULL 可）, `canceled_at` |
| `cost_item_heads` | 投影 | `cost_item_id`（PK）, `label_revision_id`, `ends_on`（NULL 可 = 無期限）。行がある = 取り消されていない |

- 区分 `category` は `enum(compute, database, storage, network, domain, email, software, other)` の固定の一覧にし、表示名はカタログで翻訳する（ADR-0009）。事業者は自由な文字列で、内訳は `provider_normalized`（NFKC・小文字）でまとめる。各行に出す表記は §20 が決める。
- 「終了した」「取り消した」を NULL 可の時刻の列で表さない（ADR-0020 規約 4）。終了は `cost_item_end_changes` の最新の行、取り消しは `cost_item_cancellations` の行の有無で表す。
  `ends_on` の NULL は data-model §1.2 の「無期限」に当たる。
- **取り消し**は、記録の誤りや不要になった項目を集計から外す操作で、過去の期間からも消える。**終了**は解約で、終了日より後の支払いだけが消える。取り消しを戻す操作は持たない（作り直す）。
- 改定（`cost_item_price_revisions`）は値上げ・値下げにも誤記の訂正にも使う。誤記は `effective_on = starts_on` で改定すれば全期間が直る。同じ `effective_on` の改定が複数あれば、最後に記録した行が効く。
  `effective_on` と `revised_at` は、効く日（業務の日付）と記録した時刻で、`moderation_actions` の開始・期限と同じく、イベントの日時属性は `revised_at` の 1 つである。
- 支払いの周期と開始日は項目の不変の事実にする。周期を変えるときは、項目を終了して新しい項目を作る。1 回だけの支払いを抜く例外（スキップ）は持たない。
- 取り込んだ費用の行（事業者・請求期間・サービス・見込みと確定・取り込みの世代）のテーブルは ADR-0068 §5 が `finance` の中に決める。ここでは §5 の境界の契約だけを決める。収入の行は §18 にある。
- 論理型 `date`（暦の日付 `YYYY-MM-DD`）を data-model §1.2 に置く。SQLite は `TEXT` と形の `CHECK`、PostgreSQL は `date`（data-model §1.2）。

### 3. 支払いの周期

- `once`: `starts_on` に 1 回払う。
- `weeks` / `months` / `years` と間隔 N: k 回目（k = 0, 1, …）の支払日は、**開始日から数えて**決める。前の支払日から足していかない（月末で日がずれていかない）。
  - `weeks`: `starts_on + 7·N·k` 日。日単位の周期は持たない。
  - `months`: `starts_on` の月から N·k か月後の、`starts_on` の日。その月にその日がなければ月の最終日（1/31 開始の毎月は 2/28 か 2/29、3/31、4/30 …）。
  - `years`: N·k 年後の同じ月日。2/29 開始は、うるう年でない年は 2/28。
- 終了日 `ends_on` があれば、支払日が `ends_on` 以下の回だけを数える（終了日を含む）。
- 各回の金額と通貨は、その支払日以前で最も新しい `effective_on` の改定の値にする。

### 4. 計上のしかたと按分

- **現金主義**（`cash`）: 各回の金額を支払日の期間に載せる。
- **按分**（`amortized`、既定）: 周期が 1 か月より長い費用（`months` の N ≥ 2、`years`、`weeks` の N ≥ 5）の各回の金額を、その回が覆う期間（支払日から次の支払日の前日まで。終了日の後も、払った最後の回は覆う期間の終わりまで）に日割りで載せる。
  それ以外（単発と 1 か月以下の周期）は、按分でも支払日に載せる。
- 日割りの端数は**累積の丸め**で決める。金額 A（最小単位）を D 日に按分するとき、最初の d 日の合計を `floor(A·d / D)` とし、ある期間に載る額は、その期間の終わりと始まりの累積の差にする。
  どの期間の分け方でも各日の額の合計が A に一致し、週・月・年の合計がそれぞれ A を超えも欠けもしない。
- 例: 2026-04-10 に 1,650 円で払う年 1 回のドメインは、2026-04-10〜2027-04-09 の 365 日に載り、2026 年 4 月には 21 日分の `floor(1650·21/365) = 94` 円が載る。月ごとの額はちょうど 1/12 にはならず、月の日数で変わる（月割りを採らない理由は「採らなかった案」）。
- 管理画面と公開のページは、どちらの見方かを選べ、既定は按分にする。「月にいくらかかるか」を示すのは按分で、現金主義は通帳と照らし合わせるときに使う。

### 5. 期間の区切りと取り込みとの契約

- 手入力の費用は日付（時刻なし）だけを持つ。支払日はカードの明細や請求書に書かれた暦の日付で、時刻とタイムゾーンを持たない。ADR-0009 §3 の「サーバーは UTC で保持」は時刻の値の規則で、暦の日付には当たらない。
- 期間は暦の日付の範囲で、タイムゾーンで区切らない。週は ISO 8601 の週（月曜〜日曜、ラベルは `2026-W17` と期間の日付）、月と年は暦のとおりにする。週・月・年はそれぞれ独立に日付で区切るので、年をまたぐ週は年の集計では 2 つの年に分かれる。
- 「今週」「今月」を決める今日の日付だけは、閲覧者の端末のタイムゾーンで Web クライアントが決め、API には期間を日付で渡す。
- **取り込みとの契約**（ADR-0068 が守る）:
  - 取り込んだ費用の 1 件は、事業者の請求期間（ふつうは事業者のタイムゾーンでの暦月）を暦の日付の範囲 `[starts_on, ends_on]` で持ち、その期間の金額を持つ。日付は事業者のタイムゾーンのまま持ち、変換しない（AWS は UTC、Google Cloud は米国太平洋時間。「経緯」の出典）。
    事業者の暦月を閲覧者やインスタンスのタイムゾーンで切り直すと、請求書の金額と合わなくなる。月の境目の数時間の利用が日本時間の暦月とずれることは、画面に請求期間の基準を示して受け入れる。
  - 現金主義でも按分でも、金額をその範囲の日数に日割り（累積の丸め、§4）で載せる。請求期間が暦月なら、月と年の集計ではその月にそのまま入り、週の集計では日数に比例して分かれる。
    事業者が日ごとの値を返すなら 1 日の範囲で持てばよい。どの粒度で取るかは ADR-0068 が決める。
  - 同じ事業者・請求期間・サービス・費用の種類の金額は、取り込んで値か見込み・確定の別が変わったときに新しいイベント（見込み・確定・確定後の修正）として記録し、集計には最新の値を使う。過去の行は直さない。
    Google Cloud の打ち消しの行やクレジット・返金は、取り込みの側で合計してから記録する。取り込んだ金額は負の値（クレジット・返金）も持てる。
- 画面と公開のページは、この区切りの規則（日付の扱い・週の始まり・事業者ごとの請求の日付の基準・按分の規則）を集計の近くに示す（FR-902）。

### 6. 通貨

- DB は通貨の最小単位の整数（`amount_minor`、`bigint`）と ISO 4217 の通貨コードで持つ。小数の桁は ISO 4217 の表（JPY は 0、USD は 2）を `internal/domain/finance` に持つ。float は使わない。
- 取り込みの 10 進の金額（AWS の文字列、Google Cloud の `NUMERIC` に変えた合計）は、記録する 1 件（請求期間 × サービス × 費用の種類）ごとに、最近接偶数への丸めで最小単位にする。
- 合計は通貨ごとに出し、換算しない。円とドルで払うサーバーの合計は「12,300 円 + $45.67」になる。為替の換算は、為替の日付と出典を記録する仕組みが要り、公開した合計が為替で動くので持たない。

### 7. 権限と画面

- 権限 `manage_finance` を置く。ADR-0058 §4 の区分はサーバー管理で、組み込みのロールでは `admin` だけが持つ。費用の一覧・登録・変更・終了・取り消し・集計の閲覧に要る。集計は `view_finance` でも ADR-0070 §5 の範囲で読める。
- 管理画面は ADR-0058 §1 の表に次の行を持つ: サーバー管理 | 費用（`/admin/finance`。項目の一覧とフォーム、週・月・年の集計） | `manage_finance`。
  公開範囲（§8）はサーバーの設定（`/admin/settings`、`manage_settings`）に置き、費用の画面はいまの公開範囲を示して、`manage_settings` を持つ閲覧者にはその設定へリンクする。
- 監査ログ（ADR-0058）: `target_kind` に `cost_item` を持ち、作成・ラベルの変更・改定・終了の変更・取り消しを記録する。`target_label` は項目の名前。公開範囲の変更は、既存の設定の変更として記録する。

### 8. 公開範囲

- インスタンス設定のキー `finance_visibility`（`enum(private, totals, breakdown)`、未設定は `private`）を `instance` に置く。ほかの設定と同じく `manage_settings` で守り、サーバーの設定の画面から変える。何を公開するかはサーバー全体の決定なので、費用を記録する人（`manage_finance`）ではなく設定を任された人が決める。
  - `private`: 公開の API は `404` を返し、`/about` に費用を出さない。
  - `totals`: 期間ごとの通貨ごとの合計だけを出す。
  - `breakdown`: 合計に加え、事業者ごと・区分ごとの内訳を出す。
- どの段階でも、項目の名前・メモ・個々の支払い・改定の履歴は公開しない。名前とメモにはバケット名・リージョン・プランのような構成の細部が入りやすい。
  内訳でも事業者と区分は出るので、どのクラウドで動いているかは知られる。設定の画面で `breakdown` を選ぶところにこのことを書く。
- 公開する場所: 匿名で読める API（§9）と、`/about` の「運営の費用」の区画と、そこからリンクする `/about/costs` のページ。区画は `finance_visibility` か `income_visibility` のどちらかが `private` でなければ出し、収入と収支を含む（§17）。公開するのは今日を含む期間までで、それより後の予定の支払いは出さない。

### 9. API の形（[client-api.md](../architecture/client-api.md)）

- 管理（タグ `admin-server`、スコープ `admin:read` / `admin:write` と `manage_finance`）:
  - `GET /admin/cost_items`（キーセットページング）・`POST /admin/cost_items`・`GET /admin/cost_items/{id}`
  - `PATCH /admin/cost_items/{id}`（名前・事業者・区分・メモ）・`POST /admin/cost_items/{id}/price_revisions`（改定日・金額）・`PUT /admin/cost_items/{id}/end`（終了日。`null` で無期限）・`DELETE /admin/cost_items/{id}`
  - `GET /admin/cost_totals?granularity=week|month|year&from=YYYY-MM-DD&to=YYYY-MM-DD&basis=amortized|cash`: 期間ごとの通貨ごとの合計と、事業者・区分・通貨ごとの内訳、`rules`（週の始まり、事業者ごとの請求の日付の基準）を返す。見込みの印 `estimated` は ADR-0068 §3 が決める
- 公開（タグ `instance`、`security: []`、`Cache-Control: public, max-age=60`）: `GET /instance/cost_totals` は管理と同じ問い合わせを受け、`finance_visibility` で内訳を省く。`private` では `404`。
  `GET /instance` は `finance_visibility` を返し、Web クライアントはそれで `/about` の区画を出し分ける。
- 金額は JSON で `{"amount": "12.34", "currency": "USD"}`（主単位の 10 進の文字列）にする。数値にすると JavaScript の倍精度で丸まる。入力の小数の桁が通貨の桁より多ければ `422` にする。日付は `YYYY-MM-DD` の文字列にする。
- 1 回の集計で返す期間の数に上限を持つ（週 104・月 60・年 20。超えたら `422`）。

### 10. i18n（ADR-0009）

- 金額は Web クライアントが `Intl.NumberFormat`（`style: "currency"`、`currency` は API の値）で書式化し、文字列としてメッセージに渡す。通貨はデータで決まるので ICU の number skeleton には書けない。Go は金額を書式化しない。
- 期間のラベルは `Intl.DateTimeFormat`（月・年）と `formatRange`（週の日付の範囲）で書く。周期（「3 か月ごと」）・区分・計上のしかた・区切りの規則の説明は `client*` 名前空間の ICU メッセージ（`plural`）にする。
- 事業者と項目の名前は運営者の入力で、ロケールごとに持たず `dir="auto"` で出す。

### 11. 収入の持ち主と経路

- 収入の行は `finance` が持つ。費用と同じモジュールで期間・按分・通貨の関数（`internal/domain/finance` の `Spread`・`Summarize`）を共有し、収支を 1 か所で計算する。
- 事業者からの取り込みは、ADR-0074 の照合と webhook のジョブ（加入のモジュール `supporters`、ADR-0072）が `PaymentProvider` からお金の動きを取り、`internal/app/finance` の `RecordIncome(ctx, []IncomeMovement)` を呼んで渡す。
  `internal/app` どうしの呼び出しは ADR-0002 の唯一のモジュール間の経路で、向きは「加入のモジュール → `finance`」だけにする。
- `finance` から加入のモジュールへの読み取り（プランごとの人数、§17。管理画面の加入者の表示、§16）は、`internal/app/finance` のポート `SupporterDirectory` にし、`internal/platform` が加入のモジュールで実装を組み立てる（ADR-0070 §4 の `CustomEmojiQuota` と同じ形）。import の循環を作らない。
- 課金が無効なら、`SupporterDirectory` は空を返し、収入の行は作られない。

**事業者によらない契約**（ADR-0074 の `MoneyMovement` をこの形に写す）:

```go
type IncomeMovement struct {
    Provider       string    // "stripe" (the providers ADR-0074 enables)
    ProviderID     string    // Stripe: balance transaction id; unique per provider
    Kind           IncomeKind // payment, refund, chargeback, chargeback_reversal, fee
    ParentID       string    // refund / chargeback / reversal: ProviderID of the payment; "" otherwise
    SubscriptionID string    // our subscription id (ADR-0072); "" for fees not tied to one subscription
    Source         IncomeSource // subscription, one_time (a tip, ADR-0162), unknown
    OccurredAt     time.Time // when the provider booked it (UTC)
    OccurredOn     Date      // OccurredAt as a date in the provider's reporting time zone (§13)
    Covers         DateRange // payment: the billing period it pays for; others: zero
    Gross, Fee, Net string   // decimal in major units of Currency; Net = Gross - Fee; negative for refunds
    Currency       string    // ISO 4217 (§14)
}
```

- 渡すのは事業者の側で額が確定したものだけにする。Stripe は balance transaction ができたとき（`fee` と `net` はそのときに決まる）。見込みの値は持たない（ADR-0068 §3 の見込みと確定の版は要らない）。
- 契約テスト（ADR-0074 のポートの契約に足す）: 各事業者の adapter が、決済・一部と全額の返金・チャージバック・その取り消し・独立した手数料の筋書きで、`Net = Gross − Fee`、符号、`ParentID`、`Covers`、`OccurredOn` の日付の基準を満たす `IncomeMovement` を返すことを確かめる。

### 12. 収入の記録の単位と net

- お金の動き 1 件を 1 行のイベントにする（§18 の `income_movements`）。`(provider, provider_movement_id)` で一意にし、同じ動きを webhook・照合・再試行で何度渡しても 1 行になる（ADR-0003。挿入の衝突は何もしない）。
  Stripe の balance transaction は作られた後に変わらないので、版を持たない。事業者の側の訂正は、新しい動き（`adjustment`）として来る。
- 総額・手数料・net の 3 つを持ち、公開と `view_finance` には net だけを出す。総額と手数料は `manage_finance` の管理画面で、手数料率の確かめに使う。
- 種類ごとの値:

| 種類 | 総額 | 手数料 | net | Stripe |
| --- | --- | --- | --- | --- |
| `payment` | 支払いの額 | 決済ごとの手数料 | 総額 − 手数料 | charge の balance transaction |
| `refund` | −返金の額 | 返金にかかる手数料（Stripe は 0。元の決済の手数料は返らない） | 総額 − 手数料 | `refund` の balance transaction |
| `chargeback` | −取り戻された額 | dispute の手数料（¥1,500） | 総額 − 手数料 | `adjustment`（dispute）の balance transaction |
| `chargeback_reversal` | 戻った額 | 0 | 総額 | 正の `adjustment` |
| `fee` | 0 | 加入に結び付かない手数料 | −手数料 | `stripe_fee` |

- **Stripe の Billing の手数料**は収入から差し引く: Billing の 0.7%・Stripe Tax・Radar の手数料は決済ごとの `fee` に入らず、別の `stripe_fee` の balance transaction として載る。
  ADR-0074 の照合のジョブがこれも取り込み、`fee` の種類の行（`subscription_id` を持たない）としてその日の収入から引く。加入ごとに割り当てられるかは **要検証** だが、割り当てなくても期間の net は正しい。
- 出どころ: 各行は `source` で、こちらの加入の動き（`subscription`）、チップの動き（`one_time`、ADR-0162）、どちらでもない動き（`unknown`）を見分ける。`subscription_id` の NULL だけではチップと `my` の外の支払いを見分けられないため。
- 個人ごとの支払いの扱い: `income_movements` は `subscription_id` だけを持ち、利用者の ID・名前・メールアドレスを持たない。誰の加入かは `SupporterDirectory` を通して管理画面だけが引く。アカウントの消去で加入のモジュールが利用者との結び付きを消せば、収入の行は額だけを残し、集計は変わらない。

### 13. 収入の日付と期間

- 期間は §5 と同じく暦の日付で区切り（ISO 週・暦月・暦年）、週・月・年を独立に日付で区切る。
- お金の動きは時刻を持つので、`OccurredAt` を**事業者のレポートのタイムゾーン**での日付にして `occurred_on` に持つ。ADR-0068 が AWS を UTC、Google Cloud を太平洋時間のまま持つのと同じく、月の収入を事業者のダッシュボードとレポートの月の値に合わせるため。
  - Stripe: アカウントのタイムゾーン（ダッシュボードの設定）。adapter が API（`Account.settings.dashboard.timezone`）で読む。API で読めなければ UTC にする。API で読めるかは **要検証**。
  - ADR-0074 の `PaymentProvider` に、ADR-0068 の `BillingSource.Location()` と同じ `Location() *time.Location` を持たせ、adapter が日付にする。`occurred_at` も残し、管理画面の 1 件の表示に使う。
- 集計の近くに示す区切りの規則（§5）は、収入の日付の基準（事業者のレポートのタイムゾーン）を含む。
- 公開するのは今日を含む期間までで、§8 と同じく後の期間は出さない。年払いの按分（§15）で来年に載る分も、その期間が来るまで出さない。

### 14. 収入の通貨

- §6 と同じく、通貨の最小単位の整数と ISO 4217 の通貨コードで持ち、合計は通貨ごとに出し、換算しない。事業者の 10 進の金額は、1 件ごとに最近接偶数への丸めで最小単位にする（`MinorUnitsOf`）。
- Stripe は balance transaction の `currency`（入金の通貨）で記録する。プランの価格の通貨と違えば Stripe が換算した後の額になる。

### 15. 収入の計上のしかたと年払い

- §4 と同じ 2 つの見方を出し、既定は按分にする。収支は収入と費用を**同じ見方**で並べる。
- **現金主義**（`cash`）: 各行の net を `occurred_on` に載せる。
- **按分**（`amortized`）:
  - `payment` で、`Covers` が 1 か月より長い（年払い）ものは、net を `Covers` の日数に日割り（§4 の累積の丸め、`Spread`）で載せる。月払いと、`Covers` を持たない支払いは `occurred_on` に載せる。
    §4 が年払いの費用を按分するのと同じ規則で、年払いの加入者がいる月だけ収入が跳ねることを防ぎ、按分の費用と並べたときに「月にいくら足りるか」を示す。
  - `refund`・`chargeback`・`chargeback_reversal` で、元の支払い（`parent_movement_id`）が按分されているものは、net を元の支払いと同じ `Covers` に日割りし、**`occurred_on` より前の日の分をまとめて `occurred_on` に載せ**、残りの日は元の支払いと同じ日割りで打ち消す。
    返金した月にそれまでに載った分が差し引かれ、公開した過去の期間の額は動かず、全額の返金なら後の月の按分は 0 になる。返金の総額は net の合計に一致する。
  - それ以外（元の支払いが按分されていない返金、`fee`、返らない手数料）は `occurred_on` に載せる。
- 例: 2026-04-10 に 12,000 円の年払い（決済の手数料 3.6% の 432 円を引いて net 11,568 円、`Covers` 2026-04-10〜2027-04-09 の 365 日）を、2026-06-01 に全額返金（net −12,000 円。決済の手数料は返らない）。
  - 按分では、4 月と 5 月（52 日）に元の支払いの `floor(11568·52/365) = 1,648` 円が載る。
  - 6 月には、返金の 4/10〜5/31 の分（約 −1,710 円）がまとめて載り、6 月以降の日ごとには元の支払い（約 +31.7 円）と返金（約 −32.9 円）の日割りが並ぶ。
  - 返らない手数料 432 円は、返金の日以降の日割りの差として残る。どの期間の分け方でも、全期間の合計は 11,568 − 12,000 = −432 円になる。
  - 現金主義では、4 月に 11,568 円、6 月に −12,000 円が載る。

### 16. 収入の権限と管理画面

- 収入の記録は事業者からの取り込みだけで、手入力・変更・取り消しの操作を持たない。監査ログ（ADR-0058）には何も足さない（スタッフの操作がない）。公開範囲の変更は既存の設定の変更として残る。
- `manage_finance` を持つ人は、`/admin/finance` の収入の区画で、期間ごとの通貨ごとの総額・手数料・net の合計、収支、個々のお金の動き（日時・種類・総額・手数料・net・事業者の ID・加入者のアカウント）を見る。
  個々の動きと加入者のアカウントは、この画面と管理 API だけに出す。
- `view_finance` を持つ人（ADR-0070 §5）は `/finance` で、期間ごとの通貨ごとの net の合計と収支を見る。`finance_viewer_income = plans` か、`income_visibility = plans`（公開に出ている範囲は見せる）なら、§17 の人数を足す。総額・手数料・個々の動き・加入者・プランごとの金額は見せない。

### 17. 収入の公開範囲と公開先

- インスタンス設定のキー `income_visibility`（`enum(private, totals, plans)`、未設定は `private`）を `instance` に置く。`finance_visibility` と同じく `manage_settings` で守り、`/admin/settings` の費用の区画の隣に置く。
  - `private`: 公開の API は `404` を返し、`/about` に収入を出さない。
  - `totals`: 期間ごとの通貨ごとの net の合計を出す。
  - `plans`: 合計に加え、プランごとのいまの加入者の数を出す。プランごとの金額は出さない。
- どの段階でも、個々の支払い、加入者、プランごとの金額、総額と手数料は出さない。
- **プランごとの人数**: 問い合わせの時点で、プランのロールが付いている加入（ADR-0072 の状態で「有効」「解約の予約」「猶予の内の支払いの遅れ」）の数をプランごとに数える。`SupporterDirectory.PlanCounts(ctx)` が返す（プランの ID・表示名・人数。廃止したプランは人数が 0 なら省く）。
  期間ごとの人数を出さないので、加入と解約の時期が期間の表から読めない。加入者の少ないプランでは人数から誰かが推測できることを、設定の画面で `plans` を選ぶところに書く（ADR-0070 の Consequences と同じ）。
- **収支**: 収入と費用の両方が公開されているとき（`income_visibility` と `finance_visibility` がどちらも `private` でない）だけ、期間ごとの通貨ごとに「収入の net − 費用」を出す。片方だけが公開なら収支を出さない（収支から非公開の側が逆算できるため）。
  通貨が揃わなければ通貨ごとの収支が並ぶ（§6）。
- **公開先**: `/about` の「運営の費用」の区画に収入と収支を載せ、区画は `finance_visibility` か `income_visibility` のどちらかが `private` でなければ出す。`/about/costs` のページに、費用の表と並べて収入と収支の表を出す（経路は変えない）。ページは公開されている側の API だけを呼ぶ。

### 18. 収入のデータモデル（ADR-0020）

`finance` が所有する。§2 と ADR-0068 §5 の表のほかに次を持つ。

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `income_movements` | イベント | `id`, `provider`（`enum(stripe, payjp, fake)`。事業者を足すときに値を足す）, `provider_movement_id`（`string(255)`）, `kind`（`enum(payment, refund, chargeback, chargeback_reversal, fee)`）, `parent_movement_id`（NULL 可。`refund`・`chargeback`・`chargeback_reversal` は元の `payment` の行）, `subscription_id`（NULL 可 = `fee`。ADR-0072 の加入の ID への論理参照で、外部キーを付けない）, `source`（`enum(subscription, one_time, unknown)`。加入の支払い・チップ（ADR-0162）の支払い・どちらでもない動き（`my` の外で受けた支払いと、加入に結び付かない手数料）。値の決め方は ADR-0162 §16）, `occurred_at`, `occurred_on`（`date`）, `covers_starts_on`・`covers_ends_on`（`date`、NULL 可。`payment` だけが持つ。`CHECK`）, `gross_minor`・`fee_minor`・`net_minor`（`bigint`、負も可。`net_minor = gross_minor − fee_minor` の `CHECK`）, `currency`（`string(3)`）, `recorded_at`。一意 `(provider, provider_id)`。インデックス `(occurred_on)` |

- 投影を持たず、集計は読み取りのときに計算する（§1）。数百人 × 年 12 回の規模で、期間の範囲の行を読んで按分しても軽い。
- 元の支払いより先に返金が来たとき（照合の順序）は、ADR-0074 のジョブが元の支払いを事業者から取り直して先に渡す。`RecordIncome` は元の行がない返金を受けず、ジョブの再試行に任せる。
- 状態（返金済み・按分中）を列に持たない。返金済みかは子の行の有無と合計で、按分は `covers_*` と見方で決まる（ADR-0020 規約 4）。

### 19. 収入の API の形（[client-api.md](../architecture/client-api.md)）

§9 と同じ問い合わせ（`granularity`・`from`・`to`・`basis`）と金額の表現（主単位の 10 進の文字列と通貨）、期間の数の上限を使う。

- 公開（タグ `instance`、`security: []`、`Cache-Control: public, max-age=60`）: `GET /instance/income_totals` は期間ごとの通貨ごとの net の合計、`plans` ならプランごとの人数、収支（§17 の条件のとき）、`rules`（日付の基準・按分の規則）を返す。`private` では `404`。
  `GET /instance` は `income_visibility` を返す。
- `view_finance`（タグ `finance`、スコープ `read:finance`、ADR-0070 §5）: `GET /finance/income_totals` は同じ形で、収支を常に、人数を §16 の条件で返す。`Cache-Control: private, no-store`。
- 管理（タグ `admin-server`、`admin:read` と `manage_finance`）: `GET /admin/income_totals`（総額・手数料・net の合計と収支）と `GET /admin/income_movements`（キーセットページング。期間・種類で絞り込む。加入者のアカウントを含む）。
- 金額と期間のラベルの書式化は §10 のとおり Web クライアントが `Intl.*` で行う。収入・収支・手数料・種類・人数（`plural`）の文言は `client*` 名前空間に置く。

### 20. 費用の内訳の事業者の表記

内訳は `provider_normalized`・区分・通貨でまとめ（§2）、各行に出す事業者の表記だけを次の規則で決める。正規化した形は表記に使わない。

- 集計の期間ごとに、`provider_normalized` が同じ内訳の行（区分・通貨が違う行を含む）は、同じ 1 つの表記を出す。
- 表記は、その期間に支払い（按分では日割りの分）が載る費用の項目のうち、いまのラベルの版（`cost_item_heads.label_revision_id`）の `revised_at` が最も新しい項目の `provider` である。
  `revised_at` が同じなら、ラベルの版の `id` が大きいほうを採る。
- 取り込んだ費用（ADR-0068）の表記は、取り込みが使う事業者の名前（`aws` は `AWS`、`gcp` は `Google Cloud`。`rules.sources[].provider` と同じ）である。
  同じ期間・同じ `provider_normalized` に手入力の項目があれば、手入力の項目の表記を優先する。取り込んだ費用だけの行は、取り込みの事業者の名前を出す。
- 行の並びは `provider_normalized`・区分・通貨の順にする。表記で並べ替えない。
- `GET /finance/cost_totals` の項目の行（ADR-0070 §5）の `provider` も、その期間の同じ `provider_normalized` の内訳の行と同じ表記にする。項目の行を内訳の行と見比べられるようにするためである。

### 21. 事業者の表記の API とデータ

- `CostBreakdownLine.provider` と `FinanceCostItemLine.provider` は §20 の表記を返す。正規化した事業者を返す項目を持たない。
  正規化した形は表示に使わず、まとめるキーはサーバーの中だけに持つ。
- 表記のための表やカラムは持たない。集計は `cost_item_heads` から読むいまのラベルの版の `provider`・`revised_at`・`id` を使い、読み取りのときに計算する（§1）。
- 3 つの集計の API（公開・会計の閲覧・管理）は同じ規則で表記を決める。公開の API の内訳でも、出すのは項目の名前ではなく事業者の表記なので、§8 の公開の範囲は変わらない。
- Web クライアントは、事業者の表記を運営者の入力として `dir="auto"` で出す（§10）。

### この決定を覆す条件

- 取り込みの行が増え、読み取りのときの集計が公開のページの応答時間や DB の費用（NFR-COST）を押し上げると計測で分かったら、期間ごとの合計の投影に変える。
- 複数の通貨で払う運営者の多くが 1 つの合計を求めたら、為替を記録して換算する ADR を書く。
- 日割りの月の額が「1/12 ずつ」と違うことが運営者に分かりにくいと分かったら、月と年の集計だけ月割りにすることを検討する（週の集計との食い違いを受け入れる）。
- 運営者が事業者の外の収入（銀行振込の寄付など）も載せたいと求めたら、手入力の収入の項目（費用の項目と同じ形）を別の ADR で足す。
- 返金の按分の打ち消し（§15）が運営者に分かりにくいと分かったら、返金は全額を返金の日に載せる形にし、按分の表で後の月に返金済みの収入が残ることを受け入れる。
- アカウントのタイムゾーンを API で読めても、UTC の日付との違いが運営者に分かりにくいと分かったら、すべて UTC の日付にする。
- プランの人数の推移を見たいという要望が多ければ、期間ごとの人数を、加入者の少ないプランを伏せる規則とともに検討する。
- 運営者が同じ事業者を期間ごとに違う表記で入れ、期間をまたぐと表記が変わることが分かりにくいと分かったら、期間に関わらず最新のラベルの版の表記を出す形（「採らなかった案」）に変える。

## 経緯

M4.1 はサーバーの維持費用を記録し（FR-901）、週・月・年で集計し（FR-902）、公開範囲を選んで公開する（FR-903）。月ごとだけでなく、年ごとのドメイン更新費用、週単位や単発の支払いも書けることが求められた。
取り込み（FR-904）の方法は ADR-0068 に任せ、ここでは取り込んだ費用を載せる側の形と期間の区切りの契約を決めた。判断の軸は、集計を誰が見ても同じ結果で再現でき端数の合計が支払った額とずれないこと、運営者が実際の支払いをそのまま書けること、公開を選ばない限り何も出さないことだった。

事業者の請求期間は spike #874 の[調査報告](https://github.com/mycroblog/my/issues/874#issuecomment-6321153767)（2026-10-22 に公式の文書で確認）による。

- AWS Cost Explorer: 期間は UTC の暦の日付と暦月（"All times are in Universal Coordinated Time (UTC)"、[GetCostAndUsage](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsage.html)）。金額は USD。期間ごとに `Estimated` が付き、締めの後もクレジット・返金・サポート料で翌月の半ばまで値が変わる。
- Google Cloud（BigQuery への請求データの export）: 請求月 `invoice.month` は米国太平洋時間（夏時間あり）で切られる（"Pacific Time to calculate the billing period for all customers"、[Understand your bills and charges](https://docs.cloud.google.com/identity/docs/how-to/understand-your-bills-and-charges)）。金額は請求先アカウントの通貨。確定の印はなく、訂正は打ち消しの行として追加される。

M4.2（Supporters、Epic #925）で、課金のプランの支払いを収入として記録し（FR-917）、`view_finance` を持つ人に見せ（FR-916）、費用と同じように公開範囲を選べることが求められた。公開するのは手数料を引いた額で、返金はその月で差し引き、費用と並べて収支を出し、公開範囲は費用とは別のキーにする、誰がいくら払ったかは見せない、と決まった。
当初「ほかのモジュールは `finance` を呼ばない」としていたが、期間・按分・通貨の関数を共有して収支を 1 か所で計算するため、加入のモジュールから `finance` を呼ぶ向きを許した。`/about` の区画も、費用か収入のどちらかが公開なら出すようにした。

Stripe の事実は spike #909 の[調査報告](https://github.com/mycroblog/my/issues/909#issuecomment-6326974258)（2026-10-22 に公式の文書で確認）による。決済・返金・dispute はそれぞれ balance transaction になり、`amount`（総額）・`fee`・`net` = `amount` − `fee` を入金の通貨で持つ。返金で処理の手数料は返らず、dispute は ¥1,500 の手数料が勝っても返らない。Billing の 0.7%・Stripe Tax・Radar の手数料は別の `stripe_fee` の取引として載る。Paddle は利用規約（AUP）の懸念で外した。

チップ（ADR-0162、#2612）の支払いも加入のない収入として入るので、`subscription_id` の NULL がチップと `my` の外の支払いの両方を意味しないよう、出どころの列 `source` を `income_movements` に足した（§12・§18）。

内訳は当初 `provider_normalized` をそのまま事業者として返していたので、`ConoHa` が `conoha`、`Amazon SES` が `amazon ses`、取り込んだ `Google Cloud` が `google cloud` と出た（#1206）。まとめ方は変えず、表記を入力どおりにした（§20）。

## 採らなかった案

- `instance` に置く: 公開範囲も `/about` も `instance` にあり、モジュール間の呼び出しが要らない。設定とお知らせのモジュールに、周期の計算と外部の請求の取り込みが入るので採らない。
- 集計を投影に持つ: 匿名のアクセスが多くても合計を読むだけで済む。終わりのない費用の補充、改定のたびの作り直し、再構築と検証（`projection.verify`）が要るので採らない。
- 現金主義だけ: 説明が要らないが、更新月だけ費用が跳ね「月にいくら」が分からない（FR-902 も両方を求める）。按分だけは通帳と照らし合わせられない。
- 月割り（1/12 ずつ）: 運営者の直感に合う。週に載せる規則が別に要り、週の合計を足しても月の合計にならず、半端な月の規則も増える。
- インスタンスのタイムゾーンで区切る: 時刻を持つ取り込みを 1 つの基準にそろえられる。支払日は時刻を持たないので変換が増えるだけで、請求書の日付とも合わない。閲覧者のタイムゾーンは閲覧者ごとに公開の額が変わり、UTC は日本の運営者の支払日が前日に動く。
- 週の始まりをロケールに合わせる: 見慣れた週になるが、公開した週の合計がロケールで変わる。
- 金額を 10 進の文字列で持つ: 細かい金額を丸めずに持てる。SQLite に 10 進の型がなく SQL で合計できない。請求書も最小単位で締まるので、1 行あたり最小単位の半分までの誤差を受け入れる。
- 運営者が入れた為替で換算する: 「月にいくら」を 1 つの数で示せるが、為替の日付と出典の記録、改定の扱いが要り、過去の公開した額が変わる。
- 閲覧と変更の権限を分ける（費用について）: 確かめるだけの人に変更を許さない。1 人か少人数のサーバーで使われる見込みが低いので `manage_finance` 1 つで始めた。閲覧だけの権限はプランのロールのために ADR-0070 が足した。
- `manage_settings` で費用も守る: 権限は増えないが、設定を任せた人に支払いの額も見せる。公開範囲だけ `manage_finance` で守る案は、設定の定義にキーごとの権限が要り、外に何を出すかを決める権限が 2 つに分かれる。
- 公開か非公開かの 2 段階で項目まで出す: 単純で最も透明。名前とメモから構成の細部が漏れ、合計だけ出す選択肢がない（FR-903 も 3 段階を求める）。
- 収入の行を加入のモジュールが持つ: 個人との結び付きが閉じ、`finance` を誰も呼ばずに済む。按分の関数を借りるか複製することになり、収支の計算が 2 つのモジュールにまたがる。
- 収入を期間ごとの合計の現在値で持つ: 行が少ない。見方と区切りごとに合計が要り、返金の繰り上げを表せず、二重の取り込みで額がずれる。
- 収入の日付を UTC にする: adapter の設定も要検証も要らない。日本の運営者の 5/1 午前の支払いが 4 月に載り、Stripe のダッシュボードの月の値と合わない。インスタンスのタイムゾーンは手入力の費用と揃うが、設定が増え事業者の明細と合わない。
- 収入は現金主義だけ: 入金と 1 件ずつ一致する。按分の費用と並べると、年払いの更新月だけ黒字に、ほかの月は赤字に見える。
- 返金は按分でも全額を返金の日に載せる: 説明が短い。返金済みの収入が按分の表に出続け、後の月の分が二重に引かれたように見える。
- 収入の公開を 2 段階にする: 人数から個人を推測させる設定がない。`view_finance` に人数まで見せる設定（ADR-0070）に対して公開だけが狭くなり、費用と同じように粒度を選べない。
- `GET /instance/cost_totals` に収入を足す: 1 回の要求で揃う。`finance_visibility = private` で `404` を返す §9 の契約と、収入だけを公開する組み合わせがぶつかる。
- 期間ごとの人数を出す: 支援者の推移が分かる。過去の時点を復元する問い合わせが要り、参加者の一覧（FR-914）と合わせると誰がいつ解約したかが読める。
- 表記を期間に関わらず最新のラベルの版から選ぶ: どの期間でも同じ表記になる。期間に載らない項目の表記が出て、集計の対象の外を読む必要がある。
- 表記を金額の最も大きい項目から選ぶ: それらしいが、改定や按分で表記が入れ替わり、直しても出るとは限らない。
- 完全一致でまとめる: 規則が要らないが、`ConoHa` と `conoha` が別の行になり、表記の揺れで内訳が割れる。

## Consequences（影響）

- 良い: 単発・週・月・年の支払い、月末と 2/29、値上げ、解約、誤記の訂正を、開始日から数える支払日と日付で効く改定の 1 つの規則で表せる。
- 良い: 按分の端数が累積の丸めで決まるので、どの期間の分け方でも合計が支払った額に一致し、週・月・年の表が矛盾しない。
- 良い: 日付だけで区切るので、公開した額を誰が見ても同じになる。読み取りのときに計算するので、投影の再構築・検証（ADR-0020 規約 1）が要らない。
- 良い: 収入と費用が同じ期間・見方・通貨の規則で並び、収支を 1 つの関数で計算できる。返金が公開した過去の期間を動かさず、全期間の合計が net の合計に一致する。
- 良い: 収入の行は事業者の ID で一意なので、webhook・照合・再試行がどの順で何度来ても同じ結果になる。`finance` は利用者の ID を持たず、個人ごとの支払いが管理画面と管理 API の外に出る経路がない。契約が事業者によらないので、事業者を足しても `finance` は変わらない。
- 良い: `ConoHa`・`Amazon SES`・`Cloudflare R2` が入力どおりに出て、表記の揺れは 1 行にまとまり、直した表記がすぐ出る。
- 悪い: 按分の月の額は月の日数で変わり、年払い 1,200 円が毎月ちょうど 100 円にはならない。
- 悪い: 通貨ごとの合計しか出ないので、複数の通貨で払うサーバーは全体を 1 つの数で示せない。加入の通貨と費用の通貨が違うと収支も通貨ごとに分かれる。
- 悪い: 1 回だけの支払いを抜く例外がないので、1 か月だけ無料だった月は、その日に効く改定を 2 つ（0 円と元の額）入れて表す。
- 悪い: 取り込みの事業者ごとに請求の日付の基準が違い（UTC と太平洋時間）、同じ日付でも同じ瞬間を指さない。基準を画面に示すことで受け入れる。
- 悪い: 費用を記録する人（`manage_finance`）と公開範囲を決める人（`manage_settings`）が分かれ、`manage_finance` だけを持つ人は公開範囲を変えられない。
- 悪い: 按分の表では、返金した年払いの元の支払いと返金の日割りが後の月に並ぶ。表の行を見る `manage_finance` には説明が要る。
- 悪い: 日付の基準が事業者のタイムゾーンなので、日本の運営者の Stripe のタイムゾーンが UTC のままだと、月の境目の支払いが前の月に載る。設定の確かめを運営者向けの docs に書く。
- 悪い: Stripe から銀行への入金にかかる費用（あれば）は収入から引かれず、運営者が費用の項目として記録する。
- 悪い: Stripe の `stripe_fee` はアカウント全体の手数料なので、同じアカウントをほかの販売にも使うとその手数料も引かれる。運営者向けの docs で専用のアカウントを勧める（ADR-0068 の絞り込みに当たる仕組みは持たない）。
- 悪い: 収支は両方が公開のときだけ出るので、収入だけを公開したサーバーは「足りているか」を示せない。`plans` の人数は加入者の少ないプランで個人を推測させうる。
- 悪い: 同じ事業者でも、期間ごとに載る項目が違えば期間ごとに違う表記が出うる（先月は `conoha` の項目だけ、今月は `ConoHa` の項目もある）。まとめるキーが API に出ないので、クライアントは行を `provider` の文字列で見分ける（1 つの期間の中では `provider_normalized` ごとに表記が 1 つ）。

## Open Questions / 要検証

- Stripe のアカウントのタイムゾーンを API で読めるか（§13。読めなければ UTC）。`stripe_fee` を加入ごとに割り当てられるか（spike #909 §13。割り当てられなくてもこの形は変わらない）。
