# デプロイターゲットとコストの目安

> 価格・制限値は節ごとに書いた日付の公式ドキュメント・料金ページの値で、特記がない限り USD・税抜・代表リージョン（GCP は us-central1、AWS は us-east-1）です。料金は改定されるので、各社の公式ページを正としてください。
> 公式資料で確認できないものは **要検証** としています。
> ここの数字は、見込みを立て、工夫して減らせるところを見つけるための目安で、選択肢を費用だけで退ける基準ではありません（[NFR-COST](../requirements/02-non-functional.md#コストnfr-cost)）。

## サマリ

| ターゲット | コンピュート | RDB | キュー | Blob | スケジューラ | 想定固定費 |
| --- | --- | --- | --- | --- | --- | --- |
| **Google Cloud** | Cloud Run（min instances 0） | 外部の Neon、または Cloud SQL | Cloud Tasks（push） | GCS（または R2） | Cloud Scheduler | 約 $37/月（ロードバランサ約 $18 + Neon Launch 約 $19）。ドメインマッピングや Neon Free で減らせる。Cloud SQL は最小構成で約 $9/月〜 |
| **AWS** | ECS Fargate + ALB | 外部 / RDS | DB キュー | S3 | プロセス内 cron | 約 $42/月 + DB（東京の最小構成。`app-aws-ecs` の README に内訳） |
| **Self-hosted** | Docker Compose | SQLite（+ Litestream）/ PostgreSQL | DB キュー | ローカル FS / MinIO / 外部 S3 互換 | プロセス内 cron | VM 代（$4〜6/月 クラス、要検証） |

## 設計に効く事実（2026-10-16 確認）

1. **Aurora Serverless v2 の 0 ACU は「ユーザー接続が 1 本もない」ときだけ** 一時停止する。常駐ワーカーの DB ポーリングやコネクションプール、RDS Proxy があると停止しない。復帰は通常約 15 秒、24 時間超の停止後は 30 秒以上（その間、Inbox の受信や API がタイムアウトしうる）。
   → 連合サーバーは受信が途切れにくく、一時停止はあまり見込めない。費用を抑える選択肢として外部 DB を併記する。
2. **Neon Free は 100 CU-hours/プロジェクト/月**。最小 0.25 CU で 400 時間/月（約 16.7 日）分で、常時起動（約 730 時間、182.5 CU-hours）には足りない。5 分無通信で scale to zero（Free では無効化不可）。想定するプランは Launch（$0.106 / CU 時間）で、Free はトレードオフを受け入れれば使える。
3. **Cloud Tasks**: 予約の先送りは現在時刻から **30 日** まで、タスク名による重複排除は削除済みタスクと同名の投入を **最大 24 時間** 拒否する、HTTP ターゲットの dispatch deadline は既定 10 分（15 秒〜30 分）、キューあたりの配信は 500 タスク/秒まで。課金は API 呼び出しと push の配信試行ごとで、月 100 万オペレーションまで無料、以降 $0.40 / 100 万。配送のリトライ 1 回が 1 オペレーションになる。
4. **Cloud Scheduler** の無料枠は **請求先アカウントあたり 3 ジョブ/月**、以降 $0.10 / ジョブ / 31 日（実行回数では課金されない）。
5. **署名付き URL の配信元**: R2 の署名付き URL は S3 API ドメイン（`<ACCOUNT_ID>.r2.cloudflarestorage.com`）でのみ有効で、カスタムドメインでは使えない。GCS は XML API（`https://storage.googleapis.com`）と HMAC キーで S3 互換に使え、署名付き URL は XML API のエンドポイントでのみ有効（最大 7 日）。公開の配信と一時の共有で返すホストが分かれる。R2 は転送量（egress）の課金がない。
6. **Cloud SQL** は起動している秒数で常時課金され、最小の db-f1-micro + SSD 10 GiB で約 $9.4/月。共有コアは SLA の対象外。

出典:
[Aurora Serverless v2 auto-pause](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-serverless-v2-auto-pause.html) /
[Aurora pricing](https://aws.amazon.com/rds/aurora/pricing/) /
[Neon pricing](https://neon.com/pricing) /
[Neon plans](https://neon.com/docs/introduction/plans) /
[Cloud Tasks quotas and limits](https://docs.cloud.google.com/tasks/docs/quotas) /
[Cloud Tasks pricing](https://cloud.google.com/tasks/pricing) /
[Cloud Scheduler pricing](https://cloud.google.com/scheduler/pricing) /
[R2 pricing](https://developers.cloudflare.com/r2/pricing/) /
[R2 presigned URLs](https://developers.cloudflare.com/r2/api/s3/presigned-urls/) /
[Cloud Storage interoperability](https://docs.cloud.google.com/storage/docs/interoperability) /
[Cloud Storage signed URLs](https://docs.cloud.google.com/storage/docs/access-control/signed-urls) /
[Cloud SQL pricing](https://cloud.google.com/sql/pricing)

## Google Cloud

- **構成**: Cloud Run サービス 1 つ（`role = all`）。Cloud Tasks と Cloud Scheduler はサービスの既定の URL を HTTP で叩く（push 型）。ingress を通さないので CDN の時間の制限やチャレンジを受けず、同じプロジェクトから既定の URL を呼ぶときだけ内部のトラフィックに数えられるので、サービスをインターネットから閉じられる。モジュールの構成は ADR-0164。
- **固定費**: Cloud Run はリクエストベース課金 + min instances 0 で、アイドルの課金はない。ロードバランサが約 $18 / 月（料金ページで未確認）。既定の毎分の tick は DB に触るので Neon のコンピュートが止まらず、Launch で約 $19 / 月になる。開いたストリームが 1 本でもあればインスタンスに課金が続く（1 vCPU・512 MiB で 1 本を 1 か月開き続けると約 $60 / 月。単価は要検証）。
- **減らせるところ**: ドメインマッピングならロードバランサの費用がなくなる（Preview で本番には非推奨、既定の URL が開いたまま）。Neon Free なら DB の費用がなくなるが、100 CU-hours に収めるには tick を 10 分ごとなどに延ばす必要があり、定期ジョブが遅れる。
- **制限**: HTTP/1 のリクエストは 32 MiB まで（サーバーは HTTP/1.1 で受ける）。Cloud Run は `z` で終わる一部のパス（`/healthz` など）を予約し、外からのリクエストを自分で 404 にする。無料の外向き転送は北米発 1 GiB/月だけ。Cloud Run + SQLite は永続ディスクがないため非推奨。

出典（2026-10-16 確認）: [Cloud Run pricing](https://cloud.google.com/run/pricing) / [Google Cloud Free Program](https://docs.cloud.google.com/free/docs/free-cloud-features)

## AWS

- **構成**: ECS Fargate のサービス + ALB。ジョブは DB キュー（`queue = db`）で、定期ジョブはプロセス内の cron が動かす。
- **RDB**: Aurora Serverless v2 は 0 ACU まで自動一時停止できるが、上の事実 1 のとおり停止は見込みにくい。常時 0.5 ACU なら約 $44/月（Aurora Standard で $0.12 / ACU 時間）。
- 要検証: S3 / CloudFront の料金・無料枠。

## ストリームのブローカーの費用（2026-10-19 確認）

- SNS: API リクエストは月 100 万まで無料、以降 $0.50 / 100 万（64 KB ごとに 1 リクエスト）。SQS への配信は課金されない。知らせ 1 件は SNS 1 リクエスト。
- SQS: 月 100 万リクエスト無料、以降 $0.40 / 100 万。空の受信も 1 リクエストで、listen 中のプロセス 1 つで約 129,600 回 / 月（20 秒のロングポーリング）。常時開いたストリーム約 7 本分までが無料枠。
- Pub/Sub: スループット $40 / TiB、毎月 10 GiB まで無料（リクエストごとに最低 1 KB で数える）。subscription 3 つなら知らせ約 268 万件 / 月までが無料枠。subscription の作成・削除は無料。
- Neon のアイドルの `LISTEN` の接続はコンピュートを起こし続けないとみなしている（Neon の文書は明言せず、未計測）。
- Valkey はどのクラウドでも常時課金する。ElastiCache Serverless for Valkey は料金ページでは最小 100 MB で約 $6.13 / 月、ガイドは最小 1 GB（約 $61 / 月）と書き、AWS の文書が食い違う。Memorystore for Valkey は最小の shared-core-nano でも約 $23 / 月。

出典:
[SNS pricing](https://aws.amazon.com/sns/pricing/) /
[SQS pricing](https://aws.amazon.com/sqs/pricing/) /
[Pub/Sub pricing](https://cloud.google.com/pubsub/pricing) /
[Neon: LISTEN / NOTIFY](https://neon.com/guides/pub-sub-listen-notify) /
[ElastiCache pricing](https://aws.amazon.com/elasticache/pricing/) /
[Memorystore for Valkey pricing](https://cloud.google.com/memorystore/valkey/pricing)

## コスト見積りの観点（M6 で埋める）

| 観点 | 計測方法 |
| --- | --- |
| 1 投稿あたりの配送リクエスト数 | フォロワーのドメイン分布 × shared inbox 利用率（Cloud Tasks のオペレーション数に直結） |
| 受信アクティビティ量 | 連合タイムラインの流量（リレー非接続時） |
| DB のアイドル時間 | 5 分以上アクセスが途切れる時間の合計（Neon の scale to zero、Aurora の自動一時停止が効くか） |
| DB 読み書き行数 | タイムライン 1 回取得あたりの読み取り行数・コンピュート時間（Neon の CU-hours） |
| ストレージ | ローカルメディア総量、リモートメディアのキャッシュ方針別 |
| 転送量 | メディア配信（CDN 経由 / 直接）、連合配送の外向き通信 |
