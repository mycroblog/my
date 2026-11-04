# ADR-0068: 利用料金の自動取り込み（BillingSource ポート）

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #875, #1469

## Decision（決定）

AWS は Cost Explorer を 1 日 1 回（約 $0.31/月。決定者が NFR-COST の範囲として受け入れた）、Google Cloud は BigQuery の export を 1 日 1 回（無料枠の範囲で $0）読み、予算の Pub/Sub 通知を簡易な方法として残す。どちらも請求月の粒度で取得する。値が変わったときだけ版のイベントを足し、最新の版を投影に持つ。

### 1. `BillingSource` ポート

ポートは `internal/app/finance` に置く（ADR-0067 §1。`finance` だけが使う）。型は標準ライブラリだけで、SDK の型を出さない。

```go
type BillingSource interface {
    // Name is the source's identifier: "aws" or "gcp".
    Name() string
    // Location is the time zone of the source's billing periods (UTC, America/Los_Angeles).
    Location() *time.Location
    // Fetch returns the costs of the given billing months as the provider reports them now.
    Fetch(ctx context.Context, req FetchRequest) (Snapshot, error)
}

type FetchRequest struct {
    Months      []YearMonth // billing months in Location, oldest first, at most 2
    MaxRequests int         // billed API requests this call may make; 0 forbids any
}

type Snapshot struct {
    Lines       []CostLine
    Requests    int   // billed API requests made (Cost Explorer pages, BigQuery queries)
    BytesBilled int64 // BigQuery bytes billed; 0 for other sources
}

type CostLine struct {
    Month      YearMonth   // the provider's billing month
    Covered    DateRange   // [StartsOn, EndsOn] in Location: the days the amount covers
    Service    string      // provider's service name, e.g. "Amazon Simple Storage Service"
    ChargeType ChargeType  // usage, credit, refund, tax, support, fee, adjustment, other
    Category   string      // ADR-0067 category (compute, database, storage, network, other, ...)
    Amount     string      // decimal in major units, unrounded, may be negative
    Currency   string      // ISO 4217
    Final      bool        // the provider (or the source's rule, §3) treats the month as closed
}
```

- 1 つの `(Month, Service, ChargeType)` に 1 行を返す。打ち消しの行・クレジットは adapter が合計する（ADR-0067 §5）。返さなかったキーは、その月にその費用がないことを表す。
- `Covered` は、確定した月はその暦月の全体、見込みの月は月の初日から事業者のデータが揃った最後の日まで（AWS は前日、Google Cloud は `MAX(usage_end_time)` の日）。見込みの額を月の全体に広げると、まだ来ていない日に費用が載り、週の集計が過去の週を少なく出すため。
- 区分 `Category` は adapter が既知のサービス名の表で決め、知らないサービスと税は `other` にする。表示の事業者名は `Name()` から `finance` が決める（`AWS`・`Google Cloud`）。
- 失敗は `finance` が定義する種類の値で返す（§7）。`MaxRequests` を超える呼び出しは行わず `ErrRequestCap` を返す。
- **契約テスト**: `pkg/testkit.BillingSourceContract(t, factory)` を置く。`factory` は、契約が渡す事業者のデータの筋書き（月・サービス・費用の種類・金額・見込みか確定か・打ち消しの行・ページの数・権限の不足）を受け、それを返す fake の上で動く `BillingSource` を作る。
  契約は、行のキーの一意性、`Covered` が `Month` の中にあること、金額が丸めずに 10 進で返ること、打ち消しの行とクレジットの合計、見込みと確定の印、`Requests` が `MaxRequests` を超えないこと、`MaxRequests = 0` で何も呼ばないこと、失敗の種類の分類、`ctx` の取り消しを確かめる。
  テスト用の `fake` adapter（#879）と `aws`（Cost Explorer の HTTP を `httptest` で返す）・`gcp`（BigQuery REST と Pub/Sub の fake）がこれを通す。実アカウントは使わない。

### 2. 取り込みの周期と冪等性

- 有効な事業者ごとに、`Scheduler`（ADR-0004）の定期ジョブ `finance.billing_import`（`Every` 24 時間、名前 `finance.billing_import.<source>`）を載せる。冪等キーは既存の `schedule:<name>:<slot>` で、1 日に 1 本だけ積まれる。事業者のデータの更新は 24 時間に 1 回程度なので、それより多く呼んでも値は変わらない。
- ハンドラは、事業者の API を DB のトランザクションの外で呼び（呼ぶ前の予約は §4）、結果を 1 つの短いトランザクションで記録する（ADR-0003）。
  - キーごとに、最新の版と金額（最小単位に丸めた後）・通貨・確定の別・`Covered` が同じなら何も書かない。違えば版を 1 行足す。前の回にあって今回ないキーは、金額 0 の版を足す。
  - 版の表は `(source, billing_month, service, charge_type, run_key)` で一意にし、同じジョブの再実行（at-least-once）が重ねて書いても 1 行になる。
- 取り直す月（`FetchRequest.Months`）は、事業者のタイムゾーンでの今日から決める: 今月と、今日が月の 15 日以前なら先月。15 日を過ぎても先月が確定していなければ、先月の末日まで先月を含める。それより古い月は取り直さない。
  15 日は Data Exports の「月が終わった後の最初の 2 週間は前月分が更新されうる」（spike）と、サポート料が 6〜7 日に載ることに合わせた暫定値で、`internal/app/finance` の定数に持ち、設定にはしない。#882（M6）の実アカウントでの確認で見直す。
- 管理画面の「今すぐ取り込む」は、同じジョブを冪等キー `manual:<source>:<UTC の時>` で積む。1 時間に 1 回までで、費用の上限（§4）に数える。定期の 1 回とジョブの再試行と手動の取り込みを合わせて、1 日に 4 回まで事業者を呼べる（§4）。

### 3. 見込み・確定・修正

- 版の種類 `kind` は `estimate`（見込み）・`final`（確定）・`correction`（確定後の修正）。キーの最新の版が `final` か `correction` で、新しい値が違えば `correction`、`Final` が初めて立てば `final`、それまでは `estimate` にする。
- AWS は期間ごとの `Estimated = false` を `Final` にする。先月がいつ `false` になるかは文書になく（**要検証**、#882 の A-2）、15 日を過ぎても `true` のままなら月末まで取り直す（§2）。
- Google Cloud は確定の印がないので、月 M を「M+1 月の 5 日（太平洋時間）以降の取り込み」で `Final` にする。請求書の発行日は確かめられておらず（**要検証**、#882 の G-3）、5 日は暫定値で、15 日と同じく定数に持ち、#882（M6）で見直す。確定の後も 15 日までは取り直し、違いは `correction` になる。
- 集計（ADR-0067 §4・§5）は、キーごとの最新の版の金額を `Covered` の日数に日割りで載せる。ADR-0067 §9 の集計の応答（`GET /admin/cost_totals`・`GET /instance/cost_totals`）は、見込みの版を含む期間に `estimated: true` を付け、管理画面は事業者ごとに月の見込み・確定・修正を示す（FR-904）。

### 4. 費用の上限

- AWS: 課金される要求（Cost Explorer のページ 1 つ = $0.01）を、UTC の日ごとと UTC の暦月ごとに数えて止める。
  - `billing_aws_max_requests_per_day`（既定 4）と `billing_aws_max_requests_per_month`（既定 120 = $1.20）。120 は「1 日 4 回取り直せる」× 30 日で、1 日の上限は 1 日で月の分を使い切らないためにある。
  - 数えるのは、記録された試行（§5 の `billing_import_runs` の、その UTC の日・その UTC の暦月の `occurred_at` の行）の `requests` と、記録されていない予約（`billing_import_reservations`）の `requests` の合計。定期の取り込み・ジョブの再試行・手動の取り込みをすべて含む。
  - 試行は事業者を呼ぶ前に、事業者ごとに 1 つずつ走るトランザクション（PostgreSQL は事業者ごとのトランザクションのアドバイザリロック、SQLite は書き込みのトランザクションが 1 つずつ走ること）で次を順に決める。
    1. 同じ `run_key` の試行がすでに成功していれば、何も呼ばず、何も記録せずにジョブを成功で終える。
    2. 同じ事業者に、まだ記録されていない予約で `ReservationHold`（15 分）より新しいものがあれば、呼ばずに 1 分後に再試行する。
    3. 日と月の残り（記録された試行の `requests` と、記録されていない予約の `requests` の合計を上限から引いたもの）の小さいほうを求め、0 なら呼ばずに `request_cap` の失敗として記録する。
    4. 残りがあれば、その数を `billing_import_reservations` に書き、その数を `MaxRequests` として渡す。上限のない事業者は `requests` を 0 で予約する。
  - 事業者の呼び出しはトランザクションの外で行い、試行の記録（`billing_import_runs` の行）は §2 のとおり別のトランザクションで書く。
  - 予約は、同じ `id` の試行が記録されるまで、その試行の要求の代わりに日と月の合計に入る。記録された後は試行の `requests` だけを数える。
  - 記録されないまま `ReservationHold` を過ぎた予約（記録の前にプロセスが止まった試行）は、ほかの試行を待たせないが、上限の合計には入り続ける。`ReservationHold` はジョブが走りうる時間（キューのリース）より長くする。
  - adapter はページを 1 つ取るたびに数え、残りが 0 になったら次のページを取らずに `ErrRequestCap` を返す（取ったページの行は捨て、記録は失敗にする。途中のページだけで記録すると、返らなかったキーが 0 に見えるため）。
  - `request_cap` はジョブを成功で終える（再試行しない）。日の上限なら翌 UTC 日の定期の取り込みが、月の上限なら翌月の初日の定期の取り込みが再び試す。それまでの版はそのまま集計に残り、管理画面は上限に達したことと今日・今月の要求数を示す。
  - 1 日 1 回・1 ページなら 31 回 × $0.01 で約 $0.31/月で、31 日の月に毎日 4 回呼んでも月の上限で 120 回（$1.20）に止まる。小さなアカウントで 2 か月 × 2 GroupBy の応答が 1 ページに収まるかは **要検証**（#882 の A-2）。
- Google Cloud（BigQuery）: 1 回のクエリの `maximumBytesBilled` を `billing_gcp_max_bytes_billed`（既定 1 GiB）で必ず付け、超えるスキャンはクエリごと失敗させる。1 日 1 回・最低 10 MB の課金で月 0.3 GB ほどで、毎月最初の 1 TiB の無料枠（請求先アカウントで共有）に収まる。
- Google Cloud（予算の Pub/Sub）: 1 日数十通で Pub/Sub の無料枠に収まり、上限を持たない。

### 5. データモデル（ADR-0020）

`finance` が所有する。ADR-0067 §2 の表のほかに次を持つ。

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `billing_import_runs` | イベント | `id`, `source`（`enum(aws, gcp)`）, `run_key`（ジョブの冪等キー）, `outcome`（`enum(succeeded, failed)`）, `error_kind`（`enum(none, permission, not_enabled, not_found, throttled, request_cap, bytes_cap, invalid_response, unavailable)`）, `requests`（`int`）, `bytes_billed`（`bigint`）, `occurred_at` |
| `billing_import_reservations` | イベント | `id`（試行の `billing_import_runs.id`）, `source`, `run_key`, `requests`, `reserved_at`。呼ぶ前の予約（§4） |
| `imported_cost_revisions` | イベント（版） | `id`, `source`, `billing_month`（`string(7)`、`YYYY-MM`）, `service`（`string(200)`）, `charge_type`（`enum(usage, credit, refund, tax, support, fee, adjustment, other)`）, `category`（ADR-0067 の enum）, `starts_on`・`ends_on`（`date`）, `amount_minor`（`bigint`、負も可）, `currency`（`string(3)`）, `kind`（`enum(estimate, final, correction)`）, `run_id`（`billing_import_runs.id`）, `run_key`, `recorded_at`。`(source, billing_month, service, charge_type, run_key)` で一意 |
| `imported_cost_heads` | 投影 | `(source, billing_month, service, charge_type)`（PK）→ `revision_id`。版を足すトランザクションで同期に更新する（ADR-0020 規約 7） |

- 実行の記録は試行ごとに 1 行（ジョブの再試行も 1 行ずつ）。管理画面の状態は事業者ごとの最新の行と最新の成功の行から出す。失敗も行で残し、NULL 可の時刻の列で状態を表さない（ADR-0020 規約 4）。
- 同じジョブの試行が重なって書いても版は 1 行になる: 版は `run_key` を持ち、`(source, billing_month, service, charge_type, run_key)` の一意制約にぶつかった行は書かない（upsert の do nothing）。別の日のジョブが同じ値を取り込んだときは、最新の版との比較（§2）で書かない。
- 絞り込みの設定（§6）を変えると、同じキーの値が変わり `correction` か `estimate` の版として記録される。過去の確定した月は取り直さないので、変更より前の月の値は変わらない。

### 6. AWS adapter（`internal/adapter/billing/aws`）

- `GetCostAndUsage` を `Granularity = MONTHLY`、`TimePeriod` は取り直す最も古い月の初日から明日（UTC、End は含まない）、`GroupBy` は上限の 2 つで `SERVICE` と `RECORD_TYPE`、`Metrics` は `billing_aws_metric`（`NetUnblendedCost` か `UnblendedCost`、既定 `NetUnblendedCost`）で呼ぶ。`NetUnblendedCost` は割引の後の額で、請求書に近い。
  SDK に paginator がないので `NextPageToken` を自分で回し、ページを数える。リージョンは `us-east-1`（エンドポイント `ce.us-east-1.amazonaws.com`）に固定する。
- `RECORD_TYPE` を `ChargeType` に写す: Usage・DiscountedUsage・SavingsPlanCoveredUsage → `usage`、Credit → `credit`、Refund → `refund`、Tax → `tax`、Support fee → `support`、RIFee・SavingsPlanRecurringFee・SavingsPlanUpfrontFee など → `fee`、ほかは `other`。既定の指標が請求書と一致するかは #882 で確かめる。
- 金額は応答の 10 進の文字列（`"39.1603300457"`）をそのまま返し、通貨は常に `USD`。
- 権限は `ce:GetCostAndUsage`（Resource `*`）だけ。Organizations の連結アカウントから呼ぶときは、管理アカウントの「Linked Account Access」が要る。Cost Explorer を有効にしていないアカウントは `DataUnavailableException` を返す。
- 絞り込み `billing_aws_filter`: `none`（既定）・`linked_account`（`billing_aws_linked_accounts`、`LINKED_ACCOUNT` の `Dimensions`）・`tag`（`billing_aws_tag` の `key=value`、`Filter.Tags` の `EQUALS`）。
  - いちばん正確なのは「このサーバー専用の AWS アカウント」を絞らずに取ること。設定の説明に書く。
  - タグはコスト配分タグとして有効にしないと請求のデータに出ない。付けてから有効化の画面に出るまで最大 24 時間、有効化にさらに最大 24 時間かかり、過去分は `StartCostAllocationTagBackfill` で遡る。タグを付けられない税・サポート料は絞り込みから漏れうる（落ちるかは **要検証**、#882 の A-3）。
- 依存は `github.com/aws/aws-sdk-go-v2/service/costexplorer`（Apache-2.0）。core・`config`・`sts` は go.mod にある。

### 7. Google Cloud adapter（`internal/adapter/billing/gcp`）

- `billing_gcp_method = bigquery`（既定）:
  - 運営者が一度だけ、請求先アカウントに紐づくプロジェクトにマルチリージョン（US か EU）のデータセットを有効期限なしで作り、コンソールで標準の使用料金の export を有効にする（API での設定は見つかっておらず **要検証**。手順は docs に書く）。テーブルは `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`。
  - `google.golang.org/api/bigquery/v2` の `jobs.query`（標準 SQL、`maximumBytesBilled` 付き、クエリのパラメータで月を渡す）で `invoice.month` × `service.description` × `cost_type` ごとに `SUM(CAST(cost AS NUMERIC))` とクレジットの合計（`credits[]` を `UNNEST`、`type` ごとではなく `credit` 1 つに）と `MAX(usage_end_time)` を出す。FLOAT64 の誤差を避けて `NUMERIC` で合計し、文字列で受ける。
  - `cost_type` を写す: `regular` → `usage`、`tax` → `tax`、`adjustment`・`rounding error` → `adjustment`。クレジットは `credit` の行にする。打ち消しの行は合計に入る。
  - 通貨は行の `currency`（請求先アカウントの通貨）。月の中で通貨が混ざる行は `invalid_response` にする。
  - 絞り込み: `billing_gcp_projects`（`project.id` の一覧）と `billing_gcp_label`（`key=value`、`labels` の `UNNEST`）。指定がなければ請求先アカウントのすべて。税など請求書の単位の行が `project.id` を持つかは **要検証**（#882 の G-2）で、持たなければプロジェクトの絞り込みで税が落ちることを設定の説明に書く。
  - 権限: クエリを実行するプロジェクト（`billing_gcp_query_project`）の `roles/bigquery.jobUser` と、export のデータセットの `roles/bigquery.dataViewer`。
  - `google.golang.org/api`（BSD-3-Clause）のほかに依存を足さない。`cloud.google.com/go/bigquery` は使わない（「採らなかった案」）。
- `billing_gcp_method = budget`（簡易な方法。実装は #889）:
  - 運営者が予算を 1 つ作り、Pub/Sub のトピックと pull のサブスクリプション（`billing_gcp_budget_subscription`）をつなぐ。定期ジョブがサブスクリプションを読み、`costIntervalStart` ごとに最新の `costAmount` を 1 行（サービス名は予算の名前、`usage`、区分 `other`）として返す。push にしないので通知で Cloud Run を起こさない。
  - 累計値なので取りこぼしても次の通知で追いつく。重複・順序の入れ替わりは、届いた時刻の新しいものを採ることで吸収する。サブスクリプションは 31 日使わないと失効し、保持は既定 7 日なので、ジョブが毎日読む。
  - 値は見込みだけで、サービスの内訳もない。確定は `bigquery` と同じ「M+1 月の 5 日」の規則で立て、管理画面に「予算の通知による概算」と示す。ラベルの絞り込みは予算で 1 つのキーと値だけ。
  - `cloud.google.com/go/pubsub/v2`（go.mod にある）を使う。
- 期間は `invoice.month` のとおり米国太平洋時間（`America/Los_Angeles`）の暦月で、`Covered` の日付も太平洋時間のまま返す（ADR-0067 §5）。

### 8. 設定と認証（ADR-0010、ADR-0019）

デプロイの設定（TOML と `MY_*`）に置き、`make config-docs` の表に載せる。インスタンス設定（DB）には置かない。

| キー | 内容 |
| --- | --- |
| `billing_sources` | 取り込む事業者の一覧（`aws`, `gcp`）。既定は空（取り込まない） |
| `billing_aws_metric`, `billing_aws_filter`, `billing_aws_tag`, `billing_aws_linked_accounts`, `billing_aws_max_requests_per_day`, `billing_aws_max_requests_per_month`, `billing_aws_role_arn` | §4・§6 と下の認証 |
| `billing_gcp_method`, `billing_gcp_table`（`project.dataset.table`）, `billing_gcp_query_project`, `billing_gcp_projects`, `billing_gcp_label`, `billing_gcp_max_bytes_billed`, `billing_gcp_budget_subscription` | §4・§7 |

- このサーバーの秘密の設定（`MY_*` の secret）は足さない。認証は既存の SNS・Pub/Sub の adapter と同じく SDK の既定の連鎖（AWS は `config.LoadDefaultConfig`、Google Cloud は ADC）に任せ、鍵が要る環境では、その連鎖が読む環境変数とファイルをシークレットストアから渡す（ADR-0019 §6）。

| 実行環境 | AWS（Cost Explorer） | Google Cloud（BigQuery・Pub/Sub） |
| --- | --- | --- |
| Lambda | 実行ロールに `ce:GetCostAndUsage` | Workload Identity Federation（AWS の種類）の credential configuration を `GOOGLE_APPLICATION_CREDENTIALS` で渡す（**要検証**、#882 の G-4）。サービスアカウントの JSON 鍵も読めるが勧めない |
| Cloud Run | `billing_aws_role_arn` を設定すると、adapter がメタデータサーバーから Google の ID トークンを取り、`stscreds.NewWebIdentityRoleProvider` で `AssumeRoleWithWebIdentity` する（**要検証**、#882 の A-4）。静的なアクセスキーも読める | サービスのサービスアカウントで ADC。鍵は要らない |
| Self-hosted | 静的なアクセスキー（`AWS_ACCESS_KEY_ID` などか共有の認証ファイル）。IAM Roles Anywhere は **要検証** | サービスアカウントの JSON 鍵か Workload Identity Federation |

- どちらも運営者が設定したクラウドの API なので、SDK の HTTP クライアントを使い、`kernel.HTTPClient` は通さない（AGENTS.md の Design rules）。

### 9. 失敗の扱いと管理画面

- adapter は事業者の失敗を種類に分ける: `permission`（AWS の `AccessDeniedException`、BigQuery・Pub/Sub の 403）、`not_enabled`（`DataUnavailableException`、Linked Account Access なし）、`not_found`（データセット・テーブル・サブスクリプションの 404）、`throttled`（`LimitExceededException`、429）、`bytes_cap`（`maximumBytesBilled` 超え）、`invalid_response`、`unavailable`（5xx・ネットワーク）。`request_cap` は `finance` が決める。
- `throttled` と `unavailable` はジョブのリトライ（ADR-0004 の指数バックオフ）に任せ、再試行の要求も上限に数える。それ以外は設定か権限の誤りで、再試行しても直らないので記録してジョブを成功で終え、翌日の定期ジョブが再び試す。どの失敗も `billing_import_runs` の行とログ（`slog`）に残す。
- 管理画面 `/admin/finance`（`manage_finance`、ADR-0067 §7）に事業者ごとの取り込みの状態を出す: 最後の成功の時刻、最後の試行の結果と失敗の種類（`client*` のメッセージで直し方を案内する）、今月の要求数と上限（AWS）・課金されたバイト（BigQuery）、月ごとの見込み・確定・修正、請求期間の基準（UTC・太平洋時間）。API は `GET /admin/billing_imports` と `POST /admin/billing_imports/{source}/runs`（今すぐ取り込む）。手動の取り込みは監査ログ（ADR-0058）に残す。

### 10. build tag

- `no_billing_aws` は `internal/adapter/billing/aws` と `service/costexplorer` を、`no_billing_gcp` は `internal/adapter/billing/gcp` と `bigquery/v2` を外し、それぞれ `billing_sources` の `aws`・`gcp` を拒む（[build-tags.md](../reference/build-tags.md) の「Adding a tag」の手順）。
- `no_billing_gcp` を付けない build は、予算の方法のために Pub/Sub のクライアントを含む。`no_pubsub` は `stream = pubsub` を外すだけなので、Pub/Sub のクライアントを外すには両方を付ける。
- 代表の組み合わせ: self-hosted の組に両方、AWS の組に `no_billing_gcp`、Google Cloud の組に `no_billing_aws`。

### この決定を覆す条件

- #882 で Cost Explorer の応答が 1 日 1 ページに収まらず、月の費用が $1 を超えると分かったら、AWS を Data Exports に変える。
- #882 で Cost Explorer の値が請求書と恒常的に合わない（指標の選び方で直らない）と分かったら、`bill/InvoiceId` で確定が分かる Data Exports に変える。
- 週の集計に日ごとの精度が要ると分かったら、AWS だけ `DAILY` を検討する。Google Cloud は請求月と利用の日付がずれるので請求月のままにする。
- BigQuery の export を用意できない運営者が多いと分かったら、予算の方法を既定にすることを検討する。
- 事業者の課金が要求の数ではなくなったとき、または事業者の API が冪等キーや予算の上限を持つようになったときは、予約の規則を見直す。
- 止まった試行の予約が上限を使い、取り込みが止まることが運用で多いと分かったら、予約を古くなったら数えない規則を検討する。

## 経緯

FR-904 は、AWS と Google Cloud の利用料金を定期的に取り込んで費用に載せ、月の途中の見込みと締めの後の確定を区別し、同じアカウントのほかの用途をタグ・ラベル・プロジェクトで除くことを求める。載せる側（`finance`、期間の区切り、金額の表現）は ADR-0067 が決めており、§5 の「取り込みとの契約」をここで守る。判断の軸は、請求書と同じ期間・額に収まること、運用の費用がほぼ $0 で想定外の課金が上限で止まること、何度取り込んでも重複しないこと、鍵を置かずに済む環境では置かないことだった。

事業者の API の事実は spike #874 の[調査報告](https://github.com/mycroblog/my/issues/874#issuecomment-6321153767)（2026-10-22 に公式の文書と料金のページで確認。課金のある API は呼んでいない）による。

- AWS には Cost Explorer `GetCostAndUsage` がある。ページ付けされた要求 1 回ごとに $0.01、期間は UTC の暦の日付と暦月、金額は USD だけ、期間ごとに `Estimated` が付く。締めの後も返金・クレジット・サポート料で値が変わり、請求書の額が最終。行の明細は Data Exports（CUR 2.0 / FOCUS 1.2）で S3 に出せる。
- Google Cloud には実際の費用を返す API がなく、公式の経路は Cloud Billing の BigQuery への export である。請求月 `invoice.month` は米国太平洋時間で切られ、確定の印はなく、訂正は打ち消しの行として追加される。予算（Budget）の Pub/Sub 通知は予算の範囲の累計だけを見込みの値で届ける。
- `cloud.google.com/go/bigquery` は依存の `apache/arrow` がリポジトリの `go-licenses check` を通らない。`google.golang.org/api/bigquery/v2`（REST、BSD-3-Clause）は `CGO_ENABLED=0` で build でき、`go-licenses` を通る。

決定者は、Cost Explorer の約 $0.31/月を NFR-COST の範囲として受け入れ、予算の Pub/Sub 通知を簡易な選択肢として残し（実装は別 issue）、取り直しの「翌月 15 日まで」と確定の「翌月 5 日」を #882（M6）までの暫定値とし、既定の指標を `NetUnblendedCost`、月の要求数の上限を当初案の 100 回から「1 日 4 回取り直せる」120 回とした。

要求は当初、`billing_import_runs` の `requests` の合計だけで数えていた。記録の前の試行が数に入らないので、同じ事業者の試行が同時に走る（定期のジョブと手動の取り込み、再試行とリースの切れた二重の実行、複数のレプリカ）と、どれも同じ残りを読んで呼ぶので、合わせて日の上限を超えて課金されうる数え方だった。許していたずれ（1 回につき 4 要求、$0.04 まで）は 1 つの試行が止まった場合だけのものだった。#1552 が実装した予約の規則を決定者が採用し、#1469 で数え方を置き換えた（§4）。上限の値、`request_cap` の扱い、Google Cloud の上限は変えていない。

## 採らなかった案

- AWS を Data Exports で読む: API の費用が $0 で、`bill/InvoiceId` で確定が分かり明細が残る。バケットと export の用意を運営者に求め、サーバーは `service/s3` と CSV か Parquet の読み込みを持つことになる。料金・サービスプリンシパル名・最初の配信までの時間は **要検証**。
- AWS Budgets の `DescribeBudget`: 予算の監視は無料。合計だけでサービスの内訳と確定の印がなく、呼び出しの料金も文書にない（**要検証**）。
- 日ごとの粒度: 週の集計が実際の利用の日に合う。行が日数倍になり Cost Explorer の費用が上がる。Google Cloud は遅れた利用が翌月の請求に回るので請求書と合わず、確定も月の単位でしか分からない。
- Google Cloud を予算の通知だけにする: 設定が最も軽い。見込みだけで内訳がなく、見込みと確定の区別を日の規則でしか満たせないので、簡易な方法として残した。
- `cloud.google.com/go/bigquery`: 公式の高水準のクライアントで型の変換が楽。`apache/arrow` など 11 個のモジュールが増え、arrow の `LICENSE.txt` で `go-licenses check` が失敗する（[spike の commit 980950d](https://github.com/mycroblog/my/commit/980950de15dc75fdb9d879ef6d0b1d1e6afaa78d)）。
- キーごとに 1 行の現在値（ADR-0020 規約 5）: 表が 1 つで単純。見込みから確定、修正の履歴が消え、ADR-0067 §5 に反する。
- 取り込みごとに全行を保存: 事業者の値をそのまま再現できる。行が日数倍に増え、変化のない行がほとんどになる。
- 管理画面で設定し、鍵を `Sealer` で暗号化して DB に持つ: 再デプロイなしに有効にできる。実行ロールを使えず長期の鍵を DB に置くことになり、ほかのクラウドの adapter とも設定の場所が違う。
- 要求を記録の後に数える: 表が増えない。同時の試行のずれに上限がない。
- 事業者ごとのロックを呼び出しの間ずっと持つ: 上限は超えない。呼び出しの間トランザクションかロックを持つ接続を開いたままにするので ADR-0003 と両立しない。

## Consequences（影響）

- 良い: AWS は約 $0.31/月、Google Cloud は $0 で、請求書と同じ請求月の単位で取り込め、見込み・確定・修正が版のイベントとして残る。
- 良い: 値が変わったときだけ版を足すので、毎日の取り込みでも行は月に数十件で済み、何度取り込んでも集計が重複しない。
- 良い: ポートが請求月と 10 進の金額だけを扱うので、ほかの事業者（spike の一覧の Cloudflare・DigitalOcean など）の adapter を同じ契約テストで足せる。依存は build tag で外せる。
- 良い: 同時の試行が何個あっても、課金される要求は日と月の上限を超えない。成功したジョブの再実行は事業者を呼ばない。
- 悪い: 請求月の単位なので、月の途中の週の集計は日割りの近似になる。見込みの月は前日までの額を前日までの日数に広げるので、日ごとの使い方の偏りは出ない。
- 悪い: Cost Explorer は一度有効にすると無効にできず、呼び出しごとに課金される。上限を超えた月は取り込みが止まる。
- 悪い: Google Cloud は確定の印がなく、「翌月 5 日」の規則で確定を決めるので、5 日より後に来る調整は `correction` になる。予算の方法は見込みだけで内訳もない。
- 悪い: タグ・プロジェクトの絞り込みは、タグを付けられない税・サポート料を落としうる。専用のアカウント・プロジェクトを勧めることで受け入れる。
- 悪い: Google Cloud の export の設定はコンソールでの手作業で、最初の遡りに最大 5 日かかる。
- 悪い: 記録の前にプロセスが止まった試行の予約は、実際に呼んだ数より多くても上限に数えられ、その日・その月の残りが減る。
- 悪い: 同じ事業者の試行は 1 つずつしか呼べず、重なった試行は 1 分ずつ遅れる。止まった試行があると、次の取り込みは最大 15 分待つ。`billing_import_reservations` の行は試行ごとに増える。
- 後続: #882（M6、決定者）で spike の手順 A-1〜A-4・G-1〜G-5 により、ページ数、`Estimated` が `false` になる日、指標の選び方、タグの絞り込みと税、請求書の発行日、スキャン量、税の行の `project.id`、環境をまたぐ認証を確かめ、暫定値の 15 日と 5 日を見直す。

## Open Questions / 要検証

- Cost Explorer の 2 か月 × 2 GroupBy の応答が小さなアカウントで 1 ページに収まるか、先月の `Estimated` が `false` になる日、`NetUnblendedCost` が請求書と一致するか（#882 の A-2）。
- タグの絞り込みで税・クレジット・返金・サポート料の行が落ちるか、単独アカウントでのコスト配分タグの有効化と backfill（#882 の A-3）。
- Cloud Run から AWS への web identity、Lambda から Google Cloud への Workload Identity Federation の組み立て（#882 の A-4・G-4）。
- Google Cloud の請求書の発行日と締めの後の行の追加の頻度（暫定値の「翌月 5 日」と「15 日」）、export のテーブルのパーティションの列と 1 回のスキャン量、税の行の `project.id`（#882 の G-2・G-3）。
- Billing export の設定を API で行えるか。
