# ADR-0120: 外部のジョブキュー（Cloud Tasks / SQS）は DB のジョブを起こす合図だけを運ぶ

- Status: Proposed
- Date: 2026-10-29
- Deciders: @rosylilly
- Issue: #1837

## Context（背景・課題）

M6 の Google Cloud（Cloud Run・min instances 0）と AWS（Lambda + Lambda Web Adapter）のターゲットは、常駐する worker を持たず、
ジョブを Cloud Tasks の HTTP ターゲットと SQS の event source mapping から受ける（[deployment-targets.md](../architecture/deployment-targets.md)）。
[ADR-0004](0004-job-queue-abstraction.md) はポート（`JobQueue`・pull runner と push receiver・`Scheduler`）とセマンティクス（at-least-once・リトライ・`dead_jobs`）を決め、
アダプタ `queue/sqs`・`queue/cloudtasks` を挙げているが、両方ともパッケージの宣言しかなく、`queue = sqs` / `cloudtasks` は起動時に失敗する。

ジョブのモデル（`internal/app/queue`）は、どのアダプタにも次を求めている。外部のキューはどれもそのままでは満たさない。

| 求めること | Cloud Tasks | SQS 標準キュー | SQS FIFO キュー |
| --- | --- | --- | --- |
| `OrderingKey` のジョブを 1 件ずつ、投入順に動かす（配送・supporters・voice・データの取り込みが使う） | 順序なし | 順序なし | `MessageGroupId` ごとに順序あり |
| `IdempotencyKey` は未完了のジョブの間で一意 | タスク名で重複排除（削除後も最大 24 時間は同じ名前を使えない） | なし | `MessageDeduplicationId` で 5 分だけ |
| `RunAt` の遅延 | `scheduleTime` で最大 30 日 | メッセージごとに最大 15 分 | **メッセージごとの遅延を指定できない**（キュー単位だけ） |
| 待ちのジョブの一覧（[ADR-0064](0064-queue-job-dashboard.md) の `JobInspector`） | `ListTasks`（呼び出しに費用） | 覗くと可視性タイムアウトが変わる | 同左 |

SQS の値は 2026-10-29 に [SendMessage](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_SendMessage.html) で確認した（`DelaySeconds` は 0〜900 秒、FIFO ではキュー単位だけ、重複排除は 5 分、本文は最大 1 MiB）。
Cloud Tasks の値は [ADR-0004](0004-job-queue-abstraction.md) の Context の表による。

ほかに、次も決まっていない。

- **push の受け口の認証**: Cloud Run の web は未認証で公開する。Lambda Web Adapter は HTTP でないイベント（SQS・EventBridge）を `POST /events`（`AWS_LWA_PASS_THROUGH_PATH`）でアプリに渡すが、
  Function URL からの同じパスへの要求と見分ける方法は文書にない（2026-10-29 に [Non-HTTP Event Triggers](https://aws.github.io/aws-lambda-web-adapter/features/non-http-events.html) で確認）
- **outbox のリレー**: `queue/db` の `Outbox` は外部のキューのために `outbox_events` に書き、`Relay` が移す。常駐しないターゲットで誰がいつリレーを動かすか
- **Scheduler のトリガー**: `queue.Scheduler` は worker と all のロールのプロセスの中で動き、同じ周期の 2 回目の投入を **プロセスのメモリ** で防ぐ（冪等キー `schedule:<name>:<slot>` は未完了の間だけ効く）。
  外から 1 分ごとに起こされる短命のプロセスでは、完了した後の同じ周期にもう一度ジョブを積んでしまう

今決める理由: M6 のデプロイのテンプレートと、#1838〜#1841 の実装の前提になる。v0.1.0 の後はスキーマの変更に移行が要る（[ADR-0087](0087-schema-changes-after-launch.md)）ので、表を足すならその前に決めたい。

## Decision Drivers（判断基準）

1. **ジョブのモデルをアダプタで変えない**: 順序キー・冪等キー・リトライ・`dead_jobs`・試行の記録・ダッシュボードが、`queue = db` と同じに動く。ハンドラは同じコードのまま（ADR-0004）
2. **偽の push でジョブを作らせない・早めさせない**: 受け口はインターネットから届く（Cloud Run・Function URL）。認証を破られても、積まれていないジョブを動かせない
3. **ほぼ $0 の構成を壊さない**（NFR-COST）: 常駐のプロセスと DB のポーリングを要さない（Aurora Serverless v2 の 0 ACU、Neon の scale to zero）。Cloud Scheduler の無料枠（請求先アカウントあたり 3 ジョブ）に収まる
4. **秘密と個人情報をクラウドのキューに置かない**: ペイロードは DB にだけ置く（[ADR-0086](0086-email-delivery.md) の判断基準 1）
5. **実装と検査の量**: 外部のキューごとの差を小さな部分に閉じ、コントラクトテストを共有する

## Considered Options（選択肢）

1. **A. DB の `jobs` を正とし、外部のキューは「このジョブを見よ」という合図（ジョブの ID）だけを運ぶ**。
   受け口はジョブを ID で DB から claim し、順序・冪等・リース・リトライは `queue/db` がそのまま行う
2. **B. ジョブそのもの（型・ペイロード・キー）を外部のキューに積み、足りない性質を DB の補助の表で補う**。
   冪等キーと順序キーの持ち主を DB の表に記録し、受け口が実行の前後に確かめる
3. **C. ジョブそのものを外部のキューに積み、足りない性質は諦める**。順序キーを無視し、冪等キーはタスク名・FIFO の重複排除の範囲だけで効かせる

## Decision（決定）

> Proposed の推奨案。Accepted にするのは人間の Decider のみ。

**推奨案は A。** `queue = cloudtasks` / `sqs` は「DB のキュー + 合図の運び手」で、外部のキューはジョブを持たない。

### 1. 合図（wake-up signal）

- ジョブは `queue = db` と同じく `jobs` に入る。`EnqueueTx` は呼び手のトランザクションで `jobs` に書く（`outbox_events` とリレーは使わない）。
  `Enqueue` も `jobs` に書いてから合図を送る
- **合図はジョブの ID と、見るべき時刻だけを持つ**。ペイロード・型・キーは載せない。`traceparent` はジョブの行にあるので運ばない（[ADR-0063](0063-distributed-tracing.md) §2 の伝播は DB の列で足りる）
- 合図はコミットの後に送る（uow のコミットのフック。`queue.AfterCommit` と同じ仕組み）。送れなかった合図・届かなかった合図は §3 の掃除が拾うので、送信の失敗はログに出すだけにする
- 新しいポート `queue.Waker`（`Wake(ctx, []Wake{JobID, At})`）を `internal/app/queue` に置き、`queue/cloudtasks` と `queue/sqs` が実装する。
  `At` が遅延の上限（SQS 15 分、Cloud Tasks 30 日）を超えるときは上限で送る（既存の `queue.NextVisible`）
- Cloud Tasks のタスクには名前を付けない（名前の重複排除は削除後 24 時間も効き、同じジョブの次の合図を拒むため）。SQS は標準キューを使う
- 合図の重複は害がない（§2 の claim が 1 回にする）。合図の数はジョブの投入・リトライの数と同じで、外部のキューの課金もそれに比例する

### 2. push receiver

- 受け口は合図のジョブを **ID で claim** する。`queue.Source` に `ClaimJob(ctx, id, worker, now, lease)` を足し、`queue/db` が実装する。
  結果に応じて次のようにする

  | claim の結果 | すること |
  | --- | --- |
  | 取れた | ハンドラを動かし、pull runner と同じ結果の処理（`Complete` / `Release` / `Bury`・試行の記録・エラーの報告）をする。`Release` したらその `RunAt` で合図を送る |
  | まだ `RunAt` 前（遅延の上限で早く届いた） | `RunAt` で合図を送り直す（再投入のチェーン） |
  | 順序キーの前のジョブが残っている | 何もしない。前のジョブが `Complete` / `Bury` したとき、runner が同じキーの次のジョブに合図を送る |
  | 他の worker がリース中・行がない（完了済み・dead） | 何もしない |

- 結果を DB に記録できたら成功（Cloud Tasks には 2xx、SQS にはそのメッセージを `batchItemFailures` に入れない）を返す。**リトライはアプリが `RunAt` で行い、外部のキューの再試行には頼らない**。
  外部のキューの再試行は、受け口が落ちた・DB に届かなかったときにだけ働く（5xx・`batchItemFailures`）
- ハンドラの context はリースで終わる。リースはプラットフォームの上限（Cloud Tasks の dispatch deadline、Lambda のタイムアウト）より短くし、設定の検査で確かめる
- Lambda の SQS のイベントは複数のメッセージをまとめて渡す。受け口は 1 件ずつ順に処理し、残り時間がリースに足りなくなったら残りを `batchItemFailures` で返す
  （`ReportBatchItemFailures` を使う。2026-10-29 に [Handling errors for an SQS event source](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-errorhandling.html) で確認）

### 3. 掃除（合図の取りこぼしを拾う）

- 定期のジョブ `queue.wake` が、期限の来た（`run_at <= now - 猶予`）ジョブのうち、リースがない・切れていて順序キーで止められていないものに合図を送り直す。
  猶予（既定 2 分）は送ったばかりの合図との重なりを減らすため。重なっても §2 で 1 回になる
- これでコミットとの間のプロセスの停止、合図の送信の失敗、リース中に落ちた worker のジョブを拾う。取りこぼしたジョブの遅れは掃除の周期（§4 のトリガーの周期、1 分）と猶予で決まる

### 4. Scheduler の外部トリガー

- 受け口に、合図とは別の **tick** を足す。tick は (1) `queue.Scheduler.Tick` で期限の来た周期のジョブを積み、(2) §3 の掃除をする
- 周期の 2 回目の投入は、プロセスのメモリでなく KV（`KeyValueStore.CompareAndSwap`、TTL は周期）で防ぐ。外部トリガーを使うときは `kv` を `db` か `valkey` にすることを設定の検査で求める
- Google Cloud: Cloud Scheduler の HTTP ターゲットで tick を 1 分ごとに 1 本（無料枠の 3 本のうち 1 本）。AWS: EventBridge Scheduler が同じ SQS のキューに tick のメッセージを送る（要検証: EventBridge Scheduler の SQS のターゲットとその料金）
- 外部トリガーを使うプロセスはプロセス内の Scheduler を動かさない。Self-hosted（`queue = db`、常駐の worker）は今のまま、プロセス内の Scheduler と pull runner を使い、Kubernetes の CronJob は要らない

### 5. 受け口と認証

- **Cloud Tasks・Cloud Scheduler**: 内部のエンドポイント `POST /_internal/jobs`（合図）と `POST /_internal/tick` で、アプリが OIDC の ID トークンを検証する。
  署名（Google の JWKS）・`iss`・`aud`（設定。既定は受け口の URL で、Cloud Tasks の `OidcToken.audience` の既定と同じ。2026-10-29 に [OidcToken](https://docs.cloud.google.com/tasks/docs/reference/rest/v2/OidcToken) で確認）・
  `email` が設定のサービスアカウントで `email_verified` が真であることを確かめる。JWKS の取得とキャッシュは `internal/adapter/oidc` を使う。
  Cloud Run の Invoker ロールでの認証は、worker を別のサービスに分けたときに足せる追加の守りとし、アプリの検証を省かない
- **Lambda**: Lambda Web Adapter の pass-through のパスを、テンプレートが作る推測できない値（設定 `queue_sqs_events_path`、`AWS_LWA_PASS_THROUGH_PATH` と同じ値）にする。
  そのパス以外への非 HTTP の形の要求は受けない。Function URL から届く要求を見分ける仕組みが Lambda Web Adapter にあれば併用する（要検証）
- どちらでも、**合図は DB にあるジョブの ID を指すだけ** なので、認証を破られても新しいジョブを作れず、`RunAt` 前のジョブを早められない（§2 の表）。できるのは DB の読み取りを増やすことだけで、レート制限で抑える
- メンテナンスモード（[ADR-0084](0084-maintenance-mode.md)）では、両方の受け口が 503 を返し、外部のキューの再試行に任せる（今の表の「キューの push の受け口」の行のとおり）

### 6. 設定とビルド

- `queue = cloudtasks`: キューのパス（`projects/<p>/locations/<l>/queues/<q>`）・受け口の URL・OIDC のサービスアカウント・audience。認証は ADC
- `queue = sqs`: キューの URL・`queue_sqs_events_path`。認証は SDK の既定の資格情報の連鎖
- どちらも `db` は `postgres` か `libsql`（`jobs` を持つ DB）。ビルドタグ `no_cloudtasks` / `no_sqs`
- **SQS の pull（常駐の worker が SQS を長くポーリングする形）は作らない**。常駐の worker があるなら `queue = db` で足り、SQS を挟む利点がない

### この推奨を覆す条件

- ジョブの数が、DB の `jobs` の書き込み（1 ジョブに INSERT・claim・DELETE）を支えられないほど多いと分かった場合（B・C はこれを外部のキューに逃がせる）
- 対象の DB（Aurora DSQL など）が ID での claim のトランザクションに耐えない場合（#1846）
- Lambda Web Adapter の pass-through のパスを推測できない値にできない場合（§5 の守りを組み直す）

## Consequences（影響）

- 良い: 順序キー・冪等キー・リトライ・`dead_jobs`・試行の記録・待ちのジョブの一覧が、すべてのキューで同じになる。ハンドラは分岐しない
- 良い: [ADR-0064](0064-queue-job-dashboard.md) が外部のキューでは出せないとした待ちのジョブの一覧が出る。`docs/operations/running.md` と `docs/operations/alerts.md` の「Cloud Tasks・SQS では一覧が出ない」の記述を直す
- 良い: ペイロードと秘密がクラウドのキューに出ない。偽の合図の害が DB の読み取りに限られる
- 良い: 外部のキューのアダプタは「合図を送る」だけの小さなものになり、受け口・掃除・tick は `internal/app/queue` で共有する
- 悪い: ジョブごとに DB への書き込みが残る（`queue = db` と同じ）。B・C は外部のキューにジョブを置くので、表 `outbox_events` の行は移すまでの間だけで済む。
  ただし B・C も outbox を通るジョブは DB に一度書くので、差は INSERT・DELETE の 1 組と claim の UPDATE になる
- 悪い: 合図の取りこぼしは掃除の周期（1 分 + 猶予）まで遅れる。ジョブの実行は「早くとも `RunAt` 以降」（ADR-0004）のままで、保証の範囲は変わらない
- 悪い: 順序キーで止められたジョブは前のジョブの完了の合図を待つ。前のジョブの runner が合図を送る前に落ちると、掃除の周期だけ遅れる
- 悪い: 外部トリガーは KV に周期の記録を置くので、`kv = memory` では使えない
- 後続の作業（#1838〜#1841 の Scope をこの ADR に合わせて書き直す）:
  - `queue.Waker`・`Source.ClaimJob`・push receiver・掃除のジョブ・tick の受け口（`internal/app/queue` と `queue/db`、コントラクトテストの追加）
  - `queue/cloudtasks`（`Waker` と OIDC の検証）、`queue/sqs`（`Waker` と Lambda のイベントの受け口）
  - `queue/db` の `Outbox` の `outbox_events` とリレーは使う人がいなくなる。消すかは実装の PR で決め、消すならスキーマから表を除く（v0.1.0 の前）
  - [ADR-0004](0004-job-queue-abstraction.md) の「pull runner（DB キュー、SQS 常駐ワーカー）」と、[overview.md](../architecture/overview.md) §3 のキューの行を、この ADR が Accepted になったら更新する
  - `docs/operations/` のキュー・メンテナンス・バックアップの記述（`backup.md` の「SQS・Cloud Tasks のジョブは DB に戻らない」は、A では当てはまらなくなる）

## Pros and Cons of the Options（詳細比較・任意）

### A. DB の `jobs` を正とし、合図だけを運ぶ（推奨）

- 良い: 判断基準 1・2・4 を満たす。合図の受け口は「ID の行を claim する」だけで、DB が順序と冪等を決める
- 良い: 常駐のプロセスも DB のポーリングも要らない（判断基準 3）。DB に触るのはジョブが動くときと掃除のときだけ
- 悪い: ジョブの量の上限が DB で決まる。外部のキューの「無限に積める」利点を使わない

### B. ジョブを外部に積み、DB の補助の表で補う

- 良い: ジョブの本体が DB に残らず、量が DB に縛られない
- 悪い: 冪等キーと順序キーの持ち主の表、`RunAt` の再投入のチェーン、待ちの一覧の代わりが要り、結局 DB への書き込みはジョブごとに残る。
  補助の表と外部のキューの間の不整合（片方だけ成功）を扱う仕組みが A より大きい
- 悪い: ペイロードがクラウドのキューに置かれる。受け口の認証が破られると任意のジョブを作れる

### C. 足りない性質を諦める

- 良い: 実装が最も小さい
- 悪い: 順序キーに頼る supporters・voice・データの取り込みが、Google Cloud・AWS でだけ壊れる。ターゲットごとに正しさが変わり、検査で見つけにくい。
  受け口の認証が破られると任意のジョブを作れる

## Open Questions / 要検証

- Lambda Web Adapter が pass-through の要求に付けるヘッダーと、Function URL の要求が同じパスに届いたときの扱い（§5）
- EventBridge Scheduler の SQS のターゲットとその料金（§4）
- Cloud Tasks の dispatch deadline と、ジョブのリースの既定（5 分）との関係。長いハンドラ（データの取り込み・サービスの終了）のリースが上限に収まるか
- 掃除のジョブの問い合わせ（期限の来た・リースのない・順序キーで止められていないジョブ）が、大きな `jobs` で索引に乗るか
