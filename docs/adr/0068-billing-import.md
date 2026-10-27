# ADR-0068: 利用料金の自動取り込み（BillingSource ポート）

- Status: Accepted（§4 の要求の数え方（`billing_import_runs` の `requests` の合計と、それに続くハンドラの手順）を ADR-0105（利用料金の取り込みの要求の予約）が置き換える）
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #875

## Context（背景・課題）

FR-904 は、AWS と Google Cloud の利用料金を定期的に取り込んで費用に載せ、月の途中の見込みと締めの後の確定を区別し、同じアカウントのほかの用途をタグ・ラベル・プロジェクトで除くことを求める。
取り込んだ費用を載せる側（`finance` モジュール、期間の区切り、金額の表現）は [ADR-0067](0067-finance-cost-records.md) が決めており、§5 の「取り込みとの契約」をこの ADR が守る。

- 取り込んだ費用の 1 件は、事業者の請求期間を事業者のタイムゾーンのままの暦の日付の範囲 `[starts_on, ends_on]` で持ち、その範囲の日数に累積の丸めで日割りして載せる。
- 値か見込み・確定の別が変わったら新しいイベント（見込み・確定・確定後の修正）として記録し、集計には最新の値を使う。過去の行は直さない。
- 打ち消しの行・クレジット・返金は取り込みの側で合計してから記録し、金額は負の値も持てる。10 進の金額は、記録する 1 件ごとに最近接偶数への丸めで通貨の最小単位にする（§6）。

事業者の API の事実は、spike #874 の[調査報告](https://github.com/mycroblog/my/issues/874#issuecomment-6321153767)（2026-10-22 に公式の文書と料金のページで確認。課金のある API は呼んでいない）による。要点は次のとおり。

- AWS には Cost Explorer `GetCostAndUsage` がある。ページ付けされた要求 1 回ごとに $0.01、期間は UTC の暦の日付と暦月、金額は USD だけ、期間ごとに `Estimated` が付く。締めの後も返金・クレジット・サポート料で値が変わり、請求書の額が最終。行の明細は Data Exports（CUR 2.0 / FOCUS 1.2）で S3 に出せる。
- Google Cloud には実際の費用を返す API がなく、公式の経路は Cloud Billing の BigQuery への export である。請求月 `invoice.month` は米国太平洋時間で切られ、確定の印はなく、訂正は打ち消しの行として追加される。予算（Budget）の Pub/Sub 通知は予算の範囲の累計だけを見込みの値で届ける。
- `cloud.google.com/go/bigquery` は依存の `apache/arrow` がリポジトリの `go-licenses check` を通らない。`google.golang.org/api/bigquery/v2`（REST、BSD-3-Clause）は go.mod にあるモジュールで、`CGO_ENABLED=0` で build でき、`go-licenses` を通る。

決定者の回答（#875、原文）:

> 1. Cost Explorer の約 $0.31/月を「ほぼ $0」（NFR-COST）の範囲として受け入れるか。 : 受け入れる
> 2. Google Cloud の予算の Pub/Sub 通知を、簡易な選択肢として ADR に残すか。 : 残す
> 3. 実アカウントでの確認（#882）をお願いできるか。 : M6 として設定

決定者の回答（PR #888、原文）:

> 1. 暫定値: 取り直しを「翌月15日まで」、Google Cloud の確定を「翌月5日」とする値を、#882 の実アカウントでの確認までの暫定値として受け入れるか。 : 受け入れる
> 2. AWS の既定の指標: UnblendedCost と NetUnblendedCost のどちらにするか : NetUnblendedCost
> 3. 予算の Pub/Sub 通知の実装: 別 issue
> 4. ADR-0067 への追加: 集計 API の期間に estimated: true を足すことを、ADR-0068 に含めてよいか。 : 含めて良い
> 5. AWS の月の要求数の上限: 既定の100回（$1.00）は妥当か。 : 1日4回取り直せるとして120回で設定するのがいいのではないか
>
> ADR Accepted

決めること: ポートの形と契約テスト、AWS と Google Cloud の取得の方法と粒度、周期・冪等性・API の費用の上限、見込み・確定・修正のデータモデル、絞り込み、設定と認証、失敗の扱いと管理画面、build tag。
実装の #879（枠組み）・#880（`aws`）・#881（`gcp` の BigQuery）・#889（`gcp` の予算の Pub/Sub）がこの ADR を待っている。

## Decision Drivers（判断基準）

1. 取り込んだ額が請求書と同じ期間・同じ額に収まり、見込みと確定を区別できること（FR-904、ADR-0067 §5）
2. 運用の費用がほぼ $0 で、想定外の課金が上限で止まること（NFR-COST）
3. 何度取り込んでも重複せず、失敗しても次の回で追いつくこと（ADR-0004 の at-least-once、ADR-0020）
4. 運営者の設定が少なく、鍵を置かずに済む実行環境では鍵を置かないこと（ADR-0010、ADR-0019）
5. 依存が AGPL と両立し、`CGO_ENABLED=0` で build でき、使わない adapter を build tag で外せること（ADR-0002、ADR-0011）
6. ポートが事業者に依存せず、ほかの事業者の adapter を後から足せること

## Considered Options（選択肢）

論点ごとに比べる。各論点の 1 つめを採用する。

- **AWS の取得**: A1 Cost Explorer `GetCostAndUsage` / A2 Data Exports（CUR 2.0 / FOCUS 1.2）を S3 から読む / A3 AWS Budgets の `DescribeBudget`
- **取得の粒度**: G1 請求月（AWS は `MONTHLY`、Google Cloud は `invoice.month`）/ G2 日ごと（AWS は `DAILY`、Google Cloud は利用の日付）
- **Google Cloud の取得**: B1 BigQuery への標準の使用料金の export を `google.golang.org/api/bigquery/v2` の `jobs.query` で集計し、予算の Pub/Sub 通知を簡易な方法として選べるようにする / B2 予算の Pub/Sub 通知だけ / B3 BigQuery を `cloud.google.com/go/bigquery` で読む
- **記録の形**: E1 値が変わったときだけ版のイベントを足し、最新の版を投影に持つ / E2 キーごとに 1 行の現在値（ADR-0020 規約 5）を上書きする / E3 取り込みごとに全行を保存する
- **設定と認証**: S1 デプロイの設定（ADR-0010）と SDK の既定の認証の連鎖 / S2 管理画面で設定し、鍵を `Sealer` で暗号化して DB に持つ

## Decision（決定）

**A1・G1・B1・E1・S1 を採用する。** AWS は Cost Explorer を 1 日 1 回（約 $0.31/月。決定者が NFR-COST の範囲として受け入れた）、Google Cloud は BigQuery の export を 1 日 1 回（無料枠の範囲で $0）読み、予算の Pub/Sub 通知を簡易な方法として残す。

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
- ハンドラは、事業者の API を DB のトランザクションの外で呼び、結果を 1 つの短いトランザクションで記録する（ADR-0003）。
  - キーごとに、最新の版と金額（最小単位に丸めた後）・通貨・確定の別・`Covered` が同じなら何も書かない。違えば版を 1 行足す。前の回にあって今回ないキーは、金額 0 の版を足す。
  - 版の表は `(source, billing_month, service, charge_type, run_key)` で一意にし、同じジョブの再実行（at-least-once）が重ねて書いても 1 行になる。
- 取り直す月（`FetchRequest.Months`）は、事業者のタイムゾーンでの今日から決める: 今月と、今日が月の 15 日以前なら先月。15 日を過ぎても先月が確定していなければ、先月の末日まで先月を含める。それより古い月は取り直さない。
  15 日は Data Exports の「月が終わった後の最初の 2 週間は前月分が更新されうる」（spike）と、サポート料が 6〜7 日に載ることに合わせた暫定値（決定者の回答 1）で、`internal/app/finance` の定数に持ち、設定にはしない。#882（M6）の実アカウントでの確認で見直す。
- 管理画面の「今すぐ取り込む」は、同じジョブを冪等キー `manual:<source>:<UTC の時>` で積む。1 時間に 1 回までで、費用の上限（§4）に数える。定期の 1 回とジョブの再試行と手動の取り込みを合わせて、1 日に 4 回まで事業者を呼べる（§4）。

### 3. 見込み・確定・修正

- 版の種類 `kind` は `estimate`（見込み）・`final`（確定）・`correction`（確定後の修正）。キーの最新の版が `final` か `correction` で、新しい値が違えば `correction`、`Final` が初めて立てば `final`、それまでは `estimate` にする。
- AWS（A1）は期間ごとの `Estimated = false` を `Final` にする。先月がいつ `false` になるかは文書になく（**要検証**、#882 の A-2）、15 日を過ぎても `true` のままなら月末まで取り直す（§2）。
- Google Cloud（B1）は確定の印がないので、月 M を「M+1 月の 5 日（太平洋時間）以降の取り込み」で `Final` にする。請求書の発行日は確かめられておらず（**要検証**、#882 の G-3）、5 日は暫定値（決定者の回答 1）で、15 日と同じく定数に持ち、#882（M6）で見直す。確定の後も 15 日までは取り直し、違いは `correction` になる。
- 集計（ADR-0067 §4・§5）は、キーごとの最新の版の金額を `Covered` の日数に日割りで載せる。ADR-0067 §9 の集計の応答（`GET /admin/cost_totals`・`GET /instance/cost_totals`）は、見込みの版を含む期間に `estimated: true` を付け（この ADR が足す項目。決定者の回答 4）、管理画面は事業者ごとに月の見込み・確定・修正を示す（FR-904）。

### 4. 費用の上限

- AWS: 課金される要求（Cost Explorer のページ 1 つ = $0.01）を、UTC の日ごとと UTC の暦月ごとに数えて止める。
  - `billing_aws_max_requests_per_day`（既定 4）と `billing_aws_max_requests_per_month`（既定 120 = $1.20）。120 は「1 日 4 回取り直せる」× 30 日（決定者の回答 5）で、1 日の上限は 1 日で月の分を使い切らないためにある。
  - 数えるのは、§5 の `billing_import_runs` の `requests` の、その UTC の日・その UTC の暦月の `occurred_at` の行の合計。定期の取り込み・ジョブの再試行・手動の取り込みをすべて含む。
  - ハンドラは呼ぶ前に、日と月の残りの小さいほうを `MaxRequests` に渡す。adapter はページを 1 つ取るたびに数え、残りが 0 になったら次のページを取らずに `ErrRequestCap` を返す（取ったページの行は捨て、記録は失敗にする。途中のページだけで記録すると、返らなかったキーが 0 に見えるため）。
  - 残りが 0 なら呼ばずに `request_cap` の失敗として記録し、ジョブを成功で終える（再試行しない）。日の上限なら翌 UTC 日の定期の取り込みが、月の上限なら翌月の初日の定期の取り込みが再び試す。それまでの版はそのまま集計に残り、管理画面は上限に達したことと今日・今月の要求数を示す。
  - 1 日 1 回・1 ページなら 31 回 × $0.01 で約 $0.31/月で、31 日の月に毎日 4 回呼んでも月の上限で 120 回（$1.20）に止まる。小さなアカウントで 2 か月 × 2 GroupBy の応答が 1 ページに収まるかは **要検証**（#882 の A-2）。
- Google Cloud（BigQuery）: 1 回のクエリの `maximumBytesBilled` を `billing_gcp_max_bytes_billed`（既定 1 GiB）で必ず付け、超えるスキャンはクエリごと失敗させる。1 日 1 回・最低 10 MB の課金で月 0.3 GB ほどで、毎月最初の 1 TiB の無料枠（請求先アカウントで共有）に収まる。
- Google Cloud（予算の Pub/Sub）: 1 日数十通で Pub/Sub の無料枠に収まり、上限を持たない。
- AWS の API の呼び出しは、プロセスが要求の後・記録の前に止まると数えられない。上限はそのぶん緩くなるが、1 回の実行は日の残り（最大 4）までしか呼ばないので、ずれは 1 回につき 4 要求（$0.04）までに収まる。

### 5. データモデル（ADR-0020）

`finance` が所有する。ADR-0067 §2 の表に次を足す。

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `billing_import_runs` | イベント | `id`, `source`（`enum(aws, gcp)`）, `run_key`（ジョブの冪等キー）, `outcome`（`enum(succeeded, failed)`）, `error_kind`（`enum(none, permission, not_enabled, not_found, throttled, request_cap, bytes_cap, invalid_response, unavailable)`）, `requests`（`int`）, `bytes_billed`（`bigint`）, `occurred_at` |
| `imported_cost_revisions` | イベント（版） | `id`, `source`, `billing_month`（`string(7)`、`YYYY-MM`）, `service`（`string(200)`）, `charge_type`（`enum(usage, credit, refund, tax, support, fee, adjustment, other)`）, `category`（ADR-0067 の enum）, `starts_on`・`ends_on`（`date`）, `amount_minor`（`bigint`、負も可）, `currency`（`string(3)`）, `kind`（`enum(estimate, final, correction)`）, `run_id`（`billing_import_runs.id`）, `run_key`, `recorded_at`。`(source, billing_month, service, charge_type, run_key)` で一意 |
| `imported_cost_heads` | 投影 | `(source, billing_month, service, charge_type)`（PK）→ `revision_id`。版を足すトランザクションで同期に更新する（規約 7） |

- 実行の記録は試行ごとに 1 行（ジョブの再試行も 1 行ずつ）。管理画面の状態は事業者ごとの最新の行と最新の成功の行から出す。失敗も行で残し、NULL 可の時刻の列で状態を表さない（規約 4）。
- 同じジョブの試行が重なって書いても版は 1 行になる: 版は `run_key` を持ち、`(source, billing_month, service, charge_type, run_key)` の一意制約にぶつかった行は書かない（upsert の do nothing）。別の日のジョブが同じ値を取り込んだときは、最新の版との比較（§2）で書かない。
- 絞り込みの設定（§6）を変えると、同じキーの値が変わり `correction` か `estimate` の版として記録される。過去の確定した月は取り直さないので、変更より前の月の値は変わらない。

### 6. AWS adapter（`internal/adapter/billing/aws`）

- `GetCostAndUsage` を `Granularity = MONTHLY`、`TimePeriod` は取り直す最も古い月の初日から明日（UTC、End は含まない）、`GroupBy` は上限の 2 つで `SERVICE` と `RECORD_TYPE`、`Metrics` は `billing_aws_metric`（`NetUnblendedCost` か `UnblendedCost`、既定 `NetUnblendedCost`。決定者の回答 2）で呼ぶ。`NetUnblendedCost` は割引の後の額で、請求書に近い。
  SDK に paginator がないので `NextPageToken` を自分で回し、ページを数える。リージョンは `us-east-1`（エンドポイント `ce.us-east-1.amazonaws.com`）に固定する。
- `RECORD_TYPE` を `ChargeType` に写す: Usage・DiscountedUsage・SavingsPlanCoveredUsage → `usage`、Credit → `credit`、Refund → `refund`、Tax → `tax`、Support fee → `support`、RIFee・SavingsPlanRecurringFee・SavingsPlanUpfrontFee など → `fee`、ほかは `other`。既定の指標が請求書と一致するかは #882 で確かめる。
- 金額は応答の 10 進の文字列（`"39.1603300457"`）をそのまま返し、通貨は常に `USD`。
- 権限は `ce:GetCostAndUsage`（Resource `*`）だけ。Organizations の連結アカウントから呼ぶときは、管理アカウントの「Linked Account Access」が要る。Cost Explorer を有効にしていないアカウントは `DataUnavailableException` を返す。
- 絞り込み `billing_aws_filter`: `none`（既定）・`linked_account`（`billing_aws_linked_accounts`、`LINKED_ACCOUNT` の `Dimensions`）・`tag`（`billing_aws_tag` の `key=value`、`Filter.Tags` の `EQUALS`）。
  - いちばん正確なのは「このサーバー専用の AWS アカウント」を絞らずに取ること。設定の説明に書く。
  - タグはコスト配分タグとして有効にしないと請求のデータに出ない。付けてから有効化の画面に出るまで最大 24 時間、有効化にさらに最大 24 時間かかり、過去分は `StartCostAllocationTagBackfill` で遡る。タグを付けられない税・サポート料は絞り込みから漏れうる（落ちるかは **要検証**、#882 の A-3）。
- 新しい依存は `github.com/aws/aws-sdk-go-v2/service/costexplorer`（Apache-2.0）。core・`config`・`sts` は go.mod にある。

### 7. Google Cloud adapter（`internal/adapter/billing/gcp`）

- `billing_gcp_method = bigquery`（既定、B1）:
  - 運営者が一度だけ、請求先アカウントに紐づくプロジェクトにマルチリージョン（US か EU）のデータセットを有効期限なしで作り、コンソールで標準の使用料金の export を有効にする（API での設定は見つかっておらず **要検証**。手順は docs に書く）。テーブルは `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`。
  - `google.golang.org/api/bigquery/v2` の `jobs.query`（標準 SQL、`maximumBytesBilled` 付き、クエリのパラメータで月を渡す）で `invoice.month` × `service.description` × `cost_type` ごとに `SUM(CAST(cost AS NUMERIC))` とクレジットの合計（`credits[]` を `UNNEST`、`type` ごとではなく `credit` 1 つに）と `MAX(usage_end_time)` を出す。FLOAT64 の誤差を避けて `NUMERIC` で合計し、文字列で受ける。
  - `cost_type` を写す: `regular` → `usage`、`tax` → `tax`、`adjustment`・`rounding error` → `adjustment`。クレジットは `credit` の行にする。打ち消しの行は合計に入る。
  - 通貨は行の `currency`（請求先アカウントの通貨）。月の中で通貨が混ざる行は `invalid_response` にする。
  - 絞り込み: `billing_gcp_projects`（`project.id` の一覧）と `billing_gcp_label`（`key=value`、`labels` の `UNNEST`）。指定がなければ請求先アカウントのすべて。税など請求書の単位の行が `project.id` を持つかは **要検証**（#882 の G-2）で、持たなければプロジェクトの絞り込みで税が落ちることを設定の説明に書く。
  - 権限: クエリを実行するプロジェクト（`billing_gcp_query_project`）の `roles/bigquery.jobUser` と、export のデータセットの `roles/bigquery.dataViewer`。
  - 新しい依存はない（`google.golang.org/api` は go.mod にある、BSD-3-Clause）。`cloud.google.com/go/bigquery` は使わない（B3 の Pros and Cons）。
- `billing_gcp_method = budget`（簡易な方法、#875 の決定者の回答 2。実装は #889、PR #888 の決定者の回答 3）:
  - 運営者が予算を 1 つ作り、Pub/Sub のトピックと pull のサブスクリプション（`billing_gcp_budget_subscription`）をつなぐ。定期ジョブがサブスクリプションを読み、`costIntervalStart` ごとに最新の `costAmount` を 1 行（サービス名は予算の名前、`usage`、区分 `other`）として返す。push にしないので通知で Cloud Run を起こさない。
  - 累計値なので取りこぼしても次の通知で追いつく。重複・順序の入れ替わりは、届いた時刻の新しいものを採ることで吸収する。サブスクリプションは 31 日使わないと失効し、保持は既定 7 日なので、ジョブが毎日読む。
  - 値は見込みだけで、サービスの内訳もない。確定は B1 と同じ「M+1 月の 5 日」の規則で立て、管理画面に「予算の通知による概算」と示す。ラベルの絞り込みは予算で 1 つのキーと値だけ。
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
- 管理画面 `/admin/finance`（`manage_finance`、ADR-0067 §7）に事業者ごとの取り込みの状態を出す: 最後の成功の時刻、最後の試行の結果と失敗の種類（`client*` のメッセージで直し方を案内する）、今月の要求数と上限（AWS）・課金されたバイト（BigQuery）、月ごとの見込み・確定・修正、請求期間の基準（UTC・太平洋時間）。API は `GET /admin/billing_imports` と `POST /admin/billing_imports/{source}/runs`（今すぐ取り込む）。手動の取り込みは監査ログ（ADR-0037）に残す。

### 10. build tag

- `no_billing_aws` は `internal/adapter/billing/aws` と `service/costexplorer` を、`no_billing_gcp` は `internal/adapter/billing/gcp` と `bigquery/v2` を外し、それぞれ `billing_sources` の `aws`・`gcp` を拒む（[build-tags.md](../reference/build-tags.md) の「Adding a tag」の手順）。
- `no_billing_gcp` を付けない build は、予算の方法のために Pub/Sub のクライアントを含む。`no_pubsub` は `stream = pubsub` を外すだけなので、Pub/Sub のクライアントを外すには両方を付ける。
- 代表の組み合わせに足す: self-hosted の組に両方、AWS の組に `no_billing_gcp`、Google Cloud の組に `no_billing_aws`。

### この決定を覆す条件

- #882 で Cost Explorer の応答が 1 日 1 ページに収まらず、月の費用が $1 を超えると分かったら、AWS を Data Exports（A2）に変える。
- #882 で Cost Explorer の値が請求書と恒常的に合わない（指標の選び方で直らない）と分かったら、`bill/InvoiceId` で確定が分かる A2 に変える。
- 週の集計に日ごとの精度が要ると分かったら、AWS だけ G2（`DAILY`）を検討する。Google Cloud は請求月と利用の日付がずれるので G1 のままにする。
- BigQuery の export を用意できない運営者が多いと分かったら、予算の方法を既定にすることを検討する。

## Consequences（影響）

- 良い影響
  - AWS は約 $0.31/月、Google Cloud は $0 で、請求書と同じ請求月の単位で取り込め、見込み・確定・修正が版のイベントとして残る。
  - 値が変わったときだけ版を足すので、毎日の取り込みでも行は月に数十件で済み、何度取り込んでも集計が重複しない。
  - ポートが請求月と 10 進の金額だけを扱うので、ほかの事業者（spike の一覧の Cloudflare・DigitalOcean など）の adapter を同じ契約テストで足せる。
  - 新しい依存は `service/costexplorer` 1 つで、どちらも build tag で外せる。
- 悪い影響・トレードオフ
  - 請求月の単位なので、月の途中の週の集計は日割りの近似になる。見込みの月は前日までの額を前日までの日数に広げるので、日ごとの使い方の偏りは出ない。
  - Cost Explorer は一度有効にすると無効にできず、呼び出しごとに課金される。上限を超えた月は取り込みが止まる。
  - Google Cloud は確定の印がなく、「翌月 5 日」の規則で確定を決めるので、5 日より後に来る調整は `correction` になる。予算の方法は見込みだけで内訳もない。
  - タグ・プロジェクトの絞り込みは、タグを付けられない税・サポート料を落としうる。専用のアカウント・プロジェクトを勧めることで受け入れる。
  - Google Cloud の export の設定はコンソールでの手作業で、最初の遡りに最大 5 日かかる。
  - 取り込みの状態の表と、管理 API と画面の項目が増える。
- 後続で必要になる作業（Epic #883）
  - #879（枠組み）: `BillingSource` ポートと型、15 日と 5 日の定数、`pkg/testkit.BillingSourceContract`、`fake` adapter、§5 の 3 つの表と `schema/<dialect>.sql`・sqlc、定期ジョブと取り直す月の決め方、版の種類の決め方、UTC の日と月の要求数の上限（`billing_import_runs` から数える）、失敗の種類、`billing_sources` と `billing_aws_max_requests_per_day`・`_per_month` の設定、`GET /admin/billing_imports`・`POST /admin/billing_imports/{source}/runs` と `/admin/finance` の状態の表示、ADR-0067 §9 の集計の `estimated` と区切りの規則（事業者ごとの請求の日付の基準）。overview §3 のポートの表と data-model の所有の表に足す
  - #880（`aws`）: §6 の adapter、`RECORD_TYPE` の写し、絞り込み、`billing_aws_*` の設定と Cloud Run からの web identity、`no_billing_aws`、IAM とタグの有効化の docs
  - #881（`gcp`）: §7 の `billing_gcp_method = bigquery`、`billing_gcp_*` の設定（`budget` を除く）、`no_billing_gcp`、export の設定と権限の docs
  - #889（`gcp` の予算）: §7 の `billing_gcp_method = budget`（Pub/Sub の pull、`costIntervalStart` ごとの最新の `costAmount`、`billing_gcp_budget_subscription`）と、予算・トピック・サブスクリプションの設定と権限の docs
  - #882（M6、決定者）: spike の手順 A-1〜A-4・G-1〜G-5 で、ページ数、`Estimated` が `false` になる日、指標の選び方、タグの絞り込みと税、請求書の発行日、スキャン量、税の行の `project.id`、環境をまたぐ認証を確かめ、暫定値の 15 日と 5 日を見直す

## Pros and Cons of the Options（詳細比較・任意）

### AWS: A1 Cost Explorer（採用）/ A2 Data Exports / A3 Budgets

- A1 の良い点: 運営者の設定は IAM の権限 1 つで、`Estimated` が付き、`RECORD_TYPE` で費用の種類が分かる。悪い点: 1 ページ $0.01 の課金があり、確定の印が請求書ほど明確でない。
- A2 の最も強い論点: API の費用が $0 で、`bill/InvoiceId` で確定が分かり、行の明細が残る。悪い点: バケットとそのポリシー、export の作成、最初の配信を運営者が用意し、サーバーは `service/s3` と CSV か Parquet の読み込みと明細の集計を持つ。export 自体の料金・サービスプリンシパル名・最初の配信までの時間は **要検証**。
- A3 の良い点: 予算の監視は無料。悪い点: 合計だけでサービスの内訳と確定の印がなく、API の呼び出しの料金も文書にない（**要検証**）。

### 粒度: G1 請求月（採用）/ G2 日ごと

- G2 の最も強い論点: 週の集計が実際の利用の日に合う。悪い点: 行が日数倍になり、Cost Explorer のページが増えて費用が上がる。Google Cloud は遅れて報告された利用が翌月の請求に回るので、利用の日付で切ると請求書と合わない。確定も月の単位でしか分からない。

### Google Cloud: B1 BigQuery（REST）と予算の方法（採用）/ B2 予算だけ / B3 `cloud.google.com/go/bigquery`

- B1 の良い点: サービス・費用の種類の内訳と打ち消しの行まで取れ、無料枠の範囲で $0、新しい依存がない。悪い点: export の設定を運営者がコンソールで行い、確定は日の規則で決める。予算の方法を併せ持つので adapter が 2 つの方法を持つ。
- B2 の最も強い論点: 設定が最も軽く、データセットが要らない。悪い点: 見込みだけで内訳がなく、FR-904 の見込みと確定の区別を日の規則でしか満たせない。決定者の回答 2 に従い、B1 の中の簡易な方法として残す。
- B3 の良い点: 公式の高水準のクライアントで、結果の型の変換が楽。悪い点: `apache/arrow` など 11 個のモジュールが増え、arrow の `LICENSE.txt` で `go-licenses check` が失敗する（[spike の commit 980950d](https://github.com/mycroblog/my/commit/980950de15dc75fdb9d879ef6d0b1d1e6afaa78d)）。許可リストの見直しが要る。

### 記録: E1 変化したときの版と投影（採用）/ E2 現在値 / E3 取り込みごとの全行

- E2 の良い点: 表が 1 つで、集計が単純。悪い点: 見込みから確定、確定後の修正の履歴が消え、ADR-0067 §5 の「新しいイベントとして記録し過去の行は直さない」に反する。
- E3 の良い点: 取り込みごとの事業者の値をそのまま再現できる。悪い点: 毎日すべてのキーを書くので行が日数倍に増え、変化のない行がほとんどになる。変化は E1 でも版として残る。

### 設定: S1 デプロイの設定と SDK の連鎖（採用）/ S2 管理画面と DB の暗号化した鍵

- S2 の最も強い論点: 再デプロイなしに運営者が画面から有効にでき、絞り込みを変えられる。悪い点: Lambda・Cloud Run の実行ロール・サービスアカウントを使えず、長期の鍵を DB に置くことになる。ほかのクラウドの adapter（SNS・Pub/Sub）とも設定の場所が違う。

## Open Questions / 要検証

- Cost Explorer の 2 か月 × 2 GroupBy の応答が小さなアカウントで 1 ページに収まるか、先月の `Estimated` が `false` になる日、`NetUnblendedCost` が請求書と一致するか（#882 の A-2）。
- タグの絞り込みで税・クレジット・返金・サポート料の行が落ちるか、単独アカウントでのコスト配分タグの有効化と backfill（#882 の A-3）。
- Cloud Run から AWS への web identity、Lambda から Google Cloud への Workload Identity Federation の組み立て（#882 の A-4・G-4）。
- Google Cloud の請求書の発行日と締めの後の行の追加の頻度（暫定値の「翌月 5 日」と「15 日」）、export のテーブルのパーティションの列と 1 回のスキャン量、税の行の `project.id`（#882 の G-2・G-3）。
- Billing export の設定を API で行えるか。
