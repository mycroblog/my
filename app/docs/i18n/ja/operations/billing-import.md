<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# クラウドの利用料金の取り込み

サーバーは、設定 [`billing_sources`](../reference/configuration.md#billing_sources) に並べた事業者の利用料金を、またはアダプターを持たない事業者については [プラグイン](#a-plugin-billing_sources--plugin) の利用料金を、24 時間ごとに取り込み、インスタンスの費用に加えます。既定値は空で、何も取り込みません。
取り込むのは当月と前月（月の 15 日まで、または前月が確定するまで）の請求月の料金で、サービスと料金の種類（利用、クレジット、返金、税、サポート、手数料、その他）ごとに 1 行になります。
事業者が月を締めるまでは値を見積もりとして記録し、締めたあとは確定として記録します。確定後に値が変わったときは訂正として記録します。過去の記録を書き換えることはありません。
取り込みの状態（最後に成功した日時、最後の失敗の種類、今月のリクエスト数）は、管理画面の費用のページと `GET /api/v1/admin/billing_imports` で確認できます。
メトリクスは、すべての試行を取り込み元・結果・失敗の種類ごとに数え（`my.billing.imports`）、最新の試行が失敗した取り込み元を報告します（`my.billing.import.failing`）。
これらに対するアラートは [アラートとランブック](alerts.md#mybillingimportfailing) にあります。

このサーバーの設定には認証情報を保持しません。各事業者の SDK の既定の認証情報チェーンが読み取るものを実行環境に与えてください（AWS では実行ロール、環境変数、または共有の認証情報ファイル。Google Cloud では Application Default Credentials）。

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## OpenTofu のモジュールを使う

モジュール [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) と [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) は、`billing_sources` を設定し、このガイドの手順のうち事業者の API で行える部分を行います。`billing-gcp` は Google Cloud のアプリのモジュールとだけ組み合わせられるため、AWS ではモジュールが取り込むのは AWS の費用だけです。`billing-aws` は AWS のアプリのモジュールとも Cloud Run のアプリのモジュールとも組み合わせられ、Cloud Run では両方を `backends` に置けます。どちらのモジュールも、実際のアカウントにはまだ適用されていません。

| このガイドの手順 | モジュールを使う場合 |
| --- | --- |
| IAM のポリシーと、「Cloud Run から Web ID で呼び出す」のロール | `billing-aws` |
| Cost Explorer、Linked Account Access、コスト配分タグを有効にする | 手作業で、AWS のコンソールから |
| エクスポートのデータセットと、「IAM のロール」のロール | `billing-gcp` |
| エクスポート（「Standard usage cost」）を有効にする | 手作業で、Google Cloud のコンソールから、請求先アカウントのロールを持って |
| 予算のトピックと pull サブスクリプション、およびサブスクライバーのロール | `method = "budget"` を指定した `billing-gcp` |
| 予算を作成してトピックにつなぐ | 手作業で、Google Cloud のコンソールから、請求先アカウントのロールを持って |
| BigQuery または Pub/Sub の API を有効にする | 手作業で、[Cloud Run へのデプロイ](deploy-cloud-run.md#1-enable-the-apis) のとおりに |
| プラグイン | これらのモジュールでは行いません。`backends` に利用料金のモジュールがないときは、アプリのモジュールの `env` に `MY_BILLING_SOURCES = plugin` と [プラグイン](#a-plugin-billing_sources--plugin) の設定を指定します。利用料金のモジュールと並べるときは、アプリのモジュールが `env` の `MY_BILLING_SOURCES` を拒否するため、`billing_sources = ["plugin"]` を返す独自のバックエンドを追加します（[バックエンドのフィールド](../../deploy/opentofu/README.md)） |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS (`billing_sources = aws`)

サーバーは、請求月（UTC の暦月）ごとに 1 回、AWS Cost Explorer の `GetCostAndUsage` を、サービス（`SERVICE`）とレコードの種類（`RECORD_TYPE`）でグループ化して呼び出します。エンドポイントは常に `us-east-1` です。
金額は USD です。Cost Explorer が月を見積もり（`Estimated`）としている間は、前日までの金額を見積もりとして記録し、そうでなくなったら確定として記録します。
ビルドタグ `no_billing_aws` を付けてビルドしたバイナリは `aws` を拒否します（[build-tags.md](../reference/build-tags.md) を参照してください）。

<!-- source: cost-and-limits a555f91984c3 -->
### 費用と上限

Cost Explorer の API は、ページ分割されたリクエストごとに $0.01 を請求します。小さなアカウントでは 1 日に 1 ページのリクエストが 1 回で済み、月に約 $0.31 です。
思わぬ請求を防ぐため、サーバーはリクエストを UTC の 1 日ごと（`billing_aws_max_requests_per_day`、既定値 4）と UTC の暦月ごと（`billing_aws_max_requests_per_month`、既定値 120 = $1.20）に数え、上限に達すると呼び出さずに止まります。
定期の取り込み、ジョブの再試行、管理画面の「今すぐ取り込む」はすべて数えられます。上限に達した取り込みは種類 `request_cap` の失敗として記録され、翌日（1 日の上限の場合）または翌月の 1 日（月の上限の場合）の取り込みがもう一度試みます。
取り込みは、呼び出す前に上限の残りのリクエストを確保するため、同時に走る取り込みが合わせて上限を超えることはありません。
1 つの事業者の取り込みは 1 つずつ走ります。別の取り込みが終わっていないときに始まった取り込みは 1 分後にもう一度試み、すでに取り込みに成功したジョブはもう一度走っても何も呼び出しません。
終わる前に止まった（プロセスが終了した）取り込みは確保したリクエストを数えに入れ、次の取り込みはそれを最大 15 分待ちます。

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### Cost Explorer を有効にする

1. Billing and Cost Management のコンソールで Cost Explorer を開き、有効にします。一度有効にすると無効にはできません。データの準備には最大 24 時間かかります。
2. 有効にしていないアカウントは `DataUnavailableException` を返し、取り込みは `not_enabled` で失敗します。

<!-- source: iam-permissions 9f1c23f3cb5c -->
### IAM の権限

必要な権限は `ce:GetCostAndUsage`（リソース `*`）だけです。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

実行環境ごとの認証情報の渡し方は次のとおりです。

| 実行環境 | 認証情報 |
| --- | --- |
| AWS ECS | このポリシーをタスクロールに付けます。設定は不要です |
| Google Cloud (Cloud Run) | 下の「Cloud Run から Web ID で呼び出す」の IAM ロールを作成し、その ARN を `billing_aws_role_arn` に設定します。AWS のキーは不要です |
| 自前のサーバー | このポリシーを持つ IAM ユーザーのアクセスキーを `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`（または共有の認証情報ファイルと `AWS_PROFILE`）で渡します。シークレットストアから渡してください |

権限が足りないとき、または認証情報が無効なときは `permission` で失敗します。

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### Cloud Run から Web ID で呼び出す

`billing_aws_role_arn` を設定すると、サーバーはメタデータサーバーから Cloud Run のサービスアカウントの Google ID トークン（オーディエンスはロールの ARN）を取得し、`AssumeRoleWithWebIdentity` でそのロールの一時的な認証情報と交換します。

1. Google Cloud で、Cloud Run のサービスのサービスアカウントの一意の ID（数字の `uniqueId`）を控えます: `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`。
2. AWS IAM で、上の権限のポリシーを持つロールを作成し、その信頼ポリシーで `accounts.google.com` を信頼します。

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` はトークンの `azp`（サービスアカウントの ID）に、`accounts.google.com:oaud` はトークンの `aud`（サーバーが要求するオーディエンスで、ロールの ARN）に対応します。
3. `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>` を設定します。

この構成は、実際のアカウントではまだ検証されていません。

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### Organizations のリンクされたアカウントから呼び出す

AWS Organizations の管理アカウントは、組織のすべてのメンバーアカウントの費用を見られます。メンバー（リンクされた）アカウントの認証情報で呼び出すには、
管理アカウントの Cost Explorer の設定で「Linked Account Access」をオンにします。オフのときは IAM の権限にかかわらず呼び出しが拒否され、取り込みは `not_enabled` または `permission` で失敗します。

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### 取り込む費用を選ぶ（絞り込み）

`billing_aws_filter` で選びます。

| `billing_aws_filter` | 取り込む費用 | 追加の設定 |
| --- | --- | --- |
| `none`（既定値） | 認証情報で見られるすべての費用 | なし |
| `linked_account` | 指定したメンバーアカウントの費用（`LINKED_ACCOUNT`） | `billing_aws_linked_accounts`（12 桁のアカウント ID をカンマ区切りで） |
| `tag` | 指定したコスト配分タグを持つリソースの費用 | `billing_aws_tag`（`key=value`、例: `app=my`） |

最も正確なのは、このサーバー専用の AWS アカウント（Organizations のメンバーアカウントでもかまいません）を用意し、`none`（そのアカウントの認証情報で呼び出す）または `linked_account`（管理アカウントから呼び出す）で取り込む構成です。
税、サポート料金、一部のクレジットはタグを持てないため、タグでの絞り込みから漏れることがあります（実際のアカウントではまだ検証されていません）。

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### コスト配分タグを有効にする（`billing_aws_filter = tag`）

リソースに付けたタグは、コスト配分タグとして有効にするまで請求データに現れません。

1. このサーバーのリソース（ECS のサービス、RDS、S3 のバケット、ロードバランサーなど）に同じタグ（例: `app=my`）を付けます。
2. Billing and Cost Management のコンソールの「Cost allocation tags」で、タグのキーを選んで有効にします。タグを付けてからこの一覧に表示されるまで最大 24 時間、有効にしてから請求データに表示されるまでさらに最大 24 時間かかります。
3. 有効にする前の期間の費用を反映するには、同じ画面の「Backfill」（API `StartCostAllocationTagBackfill`）を使います。
4. `MY_BILLING_AWS_FILTER=tag` と `MY_BILLING_AWS_TAG=app=my` を設定します。

<!-- source: metric 3e034dd351bd -->
### 指標

`billing_aws_metric` は、割引（Savings Plans、リザーブドインスタンス、交渉価格）を適用したあとの `NetUnblendedCost`（既定値で、請求書に近い値）か、適用する前の `UnblendedCost` のどちらかです。
Cost Explorer の値と請求書は、更新のタイミングと端数処理のために異なることがあります。異なるときは請求書が正しい値です。

<!-- source: kinds-of-failure 2a6f9042f460 -->
### 失敗の種類

| 種類 | AWS の応答 | 対処 |
| --- | --- | --- |
| `permission` | `AccessDeniedException`、無効な認証情報 | IAM の権限と認証情報の渡し方を確認します |
| `not_enabled` | `DataUnavailableException` | Cost Explorer を有効にします。リンクされたアカウントでは Linked Account Access を有効にします |
| `not_found` | `BillExpirationException` | 取り込む期間の請求データが Cost Explorer にもうありません |
| `throttled` | `LimitExceededException` | ジョブが間を置いて再試行します（リクエストの上限に数えられます） |
| `unavailable` | 5xx、ネットワークの障害 | ジョブが間を置いて再試行します（リクエストの上限に数えられます） |
| `invalid_response` | 読み取れない応答、`ValidationException` など | 設定（タグ、アカウント ID）を確認します |
| `request_cap` | （呼び出しません） | 上限の対象の日または月が過ぎるのを待つか、上限を引き上げます |

`throttled` と `unavailable` 以外の失敗は再試行しても直らないため、サーバーは記録して止まり、翌日の定期の取り込みがもう一度試みます。

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud (`billing_sources = gcp`)

Google Cloud には実際の費用を返す API がないため、サーバーは BigQuery への Cloud Billing のエクスポート（標準の使用料金）を 1 日に 1 回、BigQuery の `jobs.query` で集計します（`billing_gcp_method = bigquery`、既定値）。
エクスポートを設定できないときのために、予算の Pub/Sub の通知からおおよその合計だけを取り込む簡易な方法もあります（[予算の通知から取り込む](#importing-from-budget-notifications-billing_gcp_method--budget) を参照してください）。
請求月はエクスポートの `invoice.month` で、米国太平洋時間（夏時間あり）の暦月です。日付は太平洋時間のまま記録します。
金額は請求先アカウントの通貨で、請求月・サービス（`service.description`）・料金の種類（`cost_type`）ごとに `NUMERIC` として合計し、クレジット（`credits[]`）はサービスごとに 1 行のクレジットになります。以前の行を打ち消すエクスポートの訂正の行も合計に含めます。
エクスポートには確定を示す印がないため、翌月の 5 日（太平洋時間）以降に走る取り込みでその月を確定として扱い、それまでは月の初日から最後の利用日までの見積もりとして記録します。確定したあとも、取り込みは 15 日までその月をもう一度読み、差があれば訂正として記録します。
ビルドタグ `no_billing_gcp` を付けてビルドしたバイナリは `gcp` を拒否します（[build-tags.md](../reference/build-tags.md) を参照してください）。

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### 費用と上限

1 回の取り込みは 1 つのクエリで、BigQuery は各クエリがスキャンしたバイト数（最小 10 MB）を請求します。小さなアカウントでスキャンするのは月に約 0.3 GB で、無料枠の月 1 TiB（請求先アカウントのほかの用途と共有）に収まるため、費用は $0 です。
思わぬスキャンを防ぐため、すべてのクエリに `maximumBytesBilled`（`billing_gcp_max_bytes_billed`、既定値 1 GiB）を付けます。これを超えるクエリは請求されずに失敗し、種類 `bytes_cap` の失敗として記録されます。管理画面には請求されたバイト数が表示されます。

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### BigQuery へのエクスポートを有効にする

運営者がコンソールで一度だけ設定します（API で設定する方法は見つかっていません）。

1. 請求先アカウントにリンクされたプロジェクトで、BigQuery のデータセット（例: `billing_export`）を作成します。ロケーションはマルチリージョン（`US` または `EU`）を選びます。当月と前月のデータがさかのぼって入ります（単一のリージョンでは、データは有効にした日から始まります）。ロケーションはあとから変更できません。
   テーブルに有効期限を設定しないでください。期限切れになった行は戻りません。
2. 「Billing > Billing export > BigQuery export」で「Standard usage cost」を有効にし、手順 1 のデータセットを選びます。設定には、請求先アカウントの「Billing Account Costs Manager」または「Billing Account Administrator」と、プロジェクトの BigQuery User のロールが必要です。
3. 数時間から 1 日後に、テーブル `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` が作成されます（ID のハイフンは `_` になります）。
4. `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` を設定します。

データセットまたはテーブルが見つからないときは、取り込みは `not_found` で失敗します。

<!-- source: iam-roles f5628acd0d94 -->
### IAM のロール

取り込むサーバーのサービスアカウントに、次の 2 つのロールを与えます。

| ロール | 付与する場所 | 用途 |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | クエリを実行するプロジェクト（`billing_gcp_query_project`、空のときはテーブルのプロジェクト） | `bigquery.jobs.create`。スキャンしたバイト数はこのプロジェクトに請求されます |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | エクスポートのデータセット | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

クエリのプロジェクトで BigQuery の API が有効になっていないときは `not_enabled` で、ロールが足りないときは `permission` で取り込みが失敗します。

認証は Application Default Credentials (ADC) です。

| 実行環境 | 認証情報 |
| --- | --- |
| Google Cloud (Cloud Run) | サービスのサービスアカウントによる ADC。キーも設定も不要です |
| AWS ECS | Workload Identity Federation（AWS の種類）の認証情報の構成ファイルを置き、そのパスを `GOOGLE_APPLICATION_CREDENTIALS` で渡します（実際のアカウントではまだ検証されていません） |
| 自前のサーバー | Workload Identity Federation、または `GOOGLE_APPLICATION_CREDENTIALS` で渡すサービスアカウントの JSON キー。キーはシークレットストアから渡してください |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### 取り込む費用を選ぶ（絞り込み）

既定では、サーバーは請求先アカウントのすべての費用を取り込みます。最も正確なのは、このサーバー専用の請求先アカウントを用意し、絞り込まずに取り込む構成です。

| 設定 | 取り込む費用 |
| --- | --- |
| `billing_gcp_projects`（プロジェクト ID をカンマ区切りで） | 指定したプロジェクトの費用（`project.id`） |
| `billing_gcp_label`（`key=value`、例: `app=my`） | 指定したラベルを持つリソースの費用（`labels`） |

両方を設定したときは、両方に一致する費用だけを取り込みます。
税など請求書の単位の費用は、プロジェクトもラベルも持たないことがあり、その場合は絞り込みから漏れます（実際のアカウントではまだ検証されていません）。

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### 失敗の種類

| 種類 | BigQuery の応答 | 対処 |
| --- | --- | --- |
| `permission` | 401、403（`accessDenied`） | サービスアカウントのロールと認証情報の渡し方を確認します |
| `not_enabled` | 403（`accessNotConfigured` など） | クエリのプロジェクトで BigQuery の API を有効にします |
| `not_found` | 404 | `billing_gcp_table` とエクスポートの設定を確認します |
| `bytes_cap` | 400（`bytesBilledLimitExceeded`） | スキャンが `billing_gcp_max_bytes_billed` を超えました。テーブルが大きいときは上限を引き上げます |
| `throttled` | 429、403（`rateLimitExceeded`、`quotaExceeded`） | ジョブが間を置いて再試行します |
| `unavailable` | 5xx、ネットワークの障害、終わらないクエリ | ジョブが間を置いて再試行します |
| `invalid_response` | 読み取れない応答、1 か月の中で通貨が混ざる行、そのほかの 400 | エクスポートのテーブル（標準の使用料金のものか）を確認します |

`throttled` と `unavailable` 以外の失敗は再試行しても直らないため、サーバーは記録して止まり、翌日の定期の取り込みがもう一度試みます。

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### 予算の通知から取り込む（`billing_gcp_method = budget`）

BigQuery へのエクスポートを設定できないときのための簡易な方法です。サーバーは 1 日に 1 回、Cloud Billing の予算が Pub/Sub のトピックに送る通知を pull サブスクリプションから読みます。
push ではなく pull を使うため、通知がサーバー（たとえば Cloud Run）を起こすことはありません。

この方法で取り込むのは、**見積もりに基づくおおよその合計だけ**です。

- 通知の `costAmount` は予算の期間の累計の見積もり費用で、サービスや料金の種類ごとの内訳はありません。サーバーが取り込むのは月と予算ごとに 1 行だけです（サービス名は予算の表示名、料金の種類は利用、カテゴリはその他）。クレジットと税は、予算の設定に従って合計に含まれます。
- BigQuery と同じく、月は翌月の 5 日（太平洋時間）以降に走る取り込みで確定します。予算の通知は請求書の締めについて何も知らないため、確定として扱った値でも請求額と異なることがあります。
- 見積もりの月は、月の初日から最後の通知が発行された日（太平洋時間）までの費用として記録します。
- 絞り込みは予算のスコープ（プロジェクト、サービス、1 つのラベルのキーと値）で決まり、`billing_gcp_projects` と `billing_gcp_label` は使いません。

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### 予算、トピック、サブスクリプションを用意する

1. このサーバーだけが読むトピックと pull サブスクリプションを作成します。保持期間と有効期限の理由は、下の「通知の読み方」を参照してください。

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. コンソールの「Billing > Budgets & alerts」で予算を作成します。期間は **月次** にします（月次でない予算の通知は `invalid_response` で失敗します）。
   スコープにこのサーバーのプロジェクトを選び、「Manage notifications > Connect a Pub/Sub topic to this budget」で手順 1 のトピックを選びます。
   予算の作成には、請求先アカウントの「Billing Account Costs Manager」または「Billing Account Administrator」と、トピックのプロジェクトの `pubsub.topics.setIamPolicy` と `pubsub.topics.list` が必要です。
   最初の通知が届くまで数時間かかり、その後は 1 日に数回届きます。
3. 取り込むサーバーのサービスアカウントに、サブスクリプションの `roles/pubsub.subscriber` (Pub/Sub Subscriber) を与えます。メッセージの pull と確認応答（`pubsub.subscriptions.consume`）に使います。

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. `MY_BILLING_GCP_METHOD=budget` と `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget` を設定します。

認証は BigQuery と同じ Application Default Credentials です（上の [IAM のロール](#iam-roles) の表を参照してください）。
ビルドタグ `no_billing_gcp` はこの方法も取り除きます。Pub/Sub のクライアントライブラリは `stream = pubsub` と共有しているため、バイナリから取り除かれるのは `no_pubsub` も指定したときだけです（[build-tags.md](../reference/build-tags.md) を参照してください）。

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### 通知の読み方

- 1 回の取り込みは新しいメッセージが届かなくなるまで Pull を繰り返し、予算と月ごとに発行時刻が最も新しい通知を採ります。これにより Pub/Sub の重複配信、同じ値の通知の繰り返し、順序の入れ替わりを吸収するため、同じ通知を何度受け取っても合計には 1 回だけ数えられます。
  1 回の取り込みで数えるリクエスト数は Pull の回数で、通常はメッセージを返す 1 回と空で戻る 1 回の 2 回です。最後に行う確認応答（`Acknowledge`、`ModifyAckDeadline`）は数えません。
- サーバーが確認応答（ack）するのは、同じ予算と月のより新しい通知に置き換えられた通知と、もう取り込まない月（確定として記録したあとの前月）の通知だけです。取り込む月の最新の通知は ack せずにサブスクリプションに戻すため、次の取り込みは同じ通知をもう一度読み、新しい通知のない日にも値は変わりません。
- そのため、サブスクリプションのメッセージの保持期間は **14 日以上（推奨は 31 日）** にしてください。月の予算の最後の通知は翌月の 2 日ごろに届き、前月は 15 日までもう一度読まれます。通知が期限切れになった月は費用 0 として記録されます。
- 既定では、サブスクリプションは 31 日使われないと期限切れになります。取り込みを止めていた期間を乗り切れるよう、有効期限を「never」にしてください。
- トピックにほかのメッセージを送らないでください。予算の通知として読めないメッセージはその取り込みを `invalid_response` で失敗させ、次の取り込みがそこで止まらないよう ack されます。
- 費用: 1 日に数十件のメッセージは Pub/Sub の無料枠（月 10 GiB）に収まるため、費用は $0 です。サーバーは上限を設けません。

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### 失敗の種類（予算の通知）

| 種類 | Pub/Sub の応答 | 対処 |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`、`UNAUTHENTICATED` | サブスクリプションの `roles/pubsub.subscriber` と認証情報の渡し方を確認します |
| `not_enabled` | `PERMISSION_DENIED`（`SERVICE_DISABLED`） | サブスクリプションのプロジェクトで Pub/Sub の API を有効にします |
| `not_found` | `NOT_FOUND` | `billing_gcp_budget_subscription` と、サブスクリプションが期限切れになっていないかを確認します |
| `throttled` | `RESOURCE_EXHAUSTED` | ジョブが間を置いて再試行します |
| `unavailable` | `UNAVAILABLE`、`INTERNAL` など、ネットワークの障害 | ジョブが間を置いて再試行します |
| `invalid_response` | 予算の通知として読めないメッセージ、月次でない予算、表示名が同じ 2 つの予算 | 予算を月次にし、予算ごとに別の表示名を付け、トピックにほかのメッセージを送らないようにします |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## プラグイン (`billing_sources = plugin`)

Azure、Hetzner、さくらなど、サーバーがアダプターを持たない事業者については、種類 `billing` の [プラグイン](plugins.md) を動かし、事業者のデータを読ませます。サーバーが持つ `plugin` の取り込み元は 1 つです。複数の事業者を扱うプラグインは、自分でそれらを集約し、サービスの名前で区別します。

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

この取り込み元では 3 つの設定が必須です。サーバーはプラグインを呼び出さずに起動するため、プラグインが停止していても起動します。

- `plugin_billing_location` は、プラグインの請求月を区切るタイムゾーンです。
- `plugin_billing_provider` は、合計、費用のページ、監査ログに表示する事業者の名前です。

サーバーはプラグインに当月を、またほかの取り込み元と同じく前月を 15 日まで、または確定するまで問い合わせます。ほかの取り込み元の上限は適用されません。`max_requests` は `2147483647` で上限なしを意味し、いくら使ってよいかはプラグインが決めます。プラグインは請求されたリクエストとバイト数を報告し、サーバーはそれを記録して表示します。

この種類は費用だけを運ぶため、起動時に警告を出したうえで平文の TCP を許します。ベアラートークンには `https://` または Unix ソケットが必要です。[プラグインの接続先](plugins.md#the-destination) を参照してください。

<!-- source: the-call 6a2bcce82ab2 -->
### 呼び出し

`plugin_billing_protocol = json`（既定値）では、プラグインに必要なのは HTTP サーバーと JSON ライブラリだけです（[ライブラリを使わないプラグイン](plugins.md#a-plugin-with-no-library)）。`Fetch` は最長 2 分、または 5 分未満の `plugin_billing_timeout` まで続きます。

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- 1 行は 1 つの月・サービス・料金の種類で、相殺する行、クレジット、返金を合計したものです。`service` は 200 文字以内で、制御文字を含みません。
- `coveredStart` と `coveredEnd` は月の中に収まります。`final` が true のときは月全体、それより前はデータのある最後の日までです。
- `amount` は主単位の丸めていない 10 進数で、負の値もありえます。`currency` は ISO 4217 のコードです。
- `chargeType` は `CHARGE_TYPE_` に `USAGE`、`CREDIT`、`REFUND`、`TAX`、`SUPPORT`、`FEE`、`ADJUSTMENT`、`OTHER` のいずれかを続けたものです。
- `category` は `CATEGORY_` に `COMPUTE`、`DATABASE`、`STORAGE`、`NETWORK`、`DOMAIN`、`EMAIL`、`SOFTWARE`、`OTHER` のいずれかを続けたものです。
- `requests` は `maxRequests` 以下です。`bytesBilled` は、バイト数で請求する事業者のための 10 進数の文字列です。
- `OUTCOME_OK` が返さないキーはその月に費用がないものとして、サーバーは 0 の改訂を記録します。事業者を読めなかったプラグインは、行を省くのではなく失敗を報告しなければなりません。
- 呼び出しは繰り返されることがあります。失敗した取り込みの再試行は、同じ呼び出しをもう一度行うものです。

`maxRequests` が 0 のときは、請求されるリクエストをすべて禁じます。サーバーは 0 を送ることになるときは呼び出しませんが、`my plugin test --kind billing` は proto3 のマッピングが許すとおりフィールドを省いて呼び出し、行もリクエストもない `OUTCOME_REQUEST_CAP` を期待します。

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### 失敗の種類

プラグインは失敗を `outcome` で報告します。ほかの取り込み元と同じく、サーバーは失敗の `requests` を記録し、その行を無視し、それまでの取り込みはそのままにします。

| 結果 | 種類 | 再試行 |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | しない |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | しない |
| `OUTCOME_NOT_FOUND` | `not_found` | しない |
| `OUTCOME_THROTTLED` | `throttled` | する |
| `OUTCOME_REQUEST_CAP` | `request_cap` | しない |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | しない |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | しない |
| `OUTCOME_UNAVAILABLE` | `unavailable` | する |
| 結果がない、または不明な結果、上の規則に反する行や数 | `invalid_response` | しない |
| HTTP または Connect のエラー、期限切れ、4 MiB を超える応答、応答なし | `unavailable` | する |

再試行しないものは、次の毎日の取り込みを待ちます。サーバーはエラーのメッセージを捨てるため、失敗の理由はプラグインのログにあります。失敗は [`my.plugin.failures`](running.md) にも数えられます。

<!-- source: test-it 10a2c62a53d8 -->
### テストする

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

1 つ目は、プラグインが契約に従っていれば請求されるリクエストを行いません。2 つ目は実際の取得も行うため、プラグインの事業者で料金がかかることがあります。[プラグインをテストする](plugins.md#test-a-plugin) を参照してください。
