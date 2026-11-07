<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# サーバーを外部から監視する

死活監視サービスをサーバーに向ける方法、監視で運用コストが上がらないようにする方法、メンテナンスモード（[メンテナンス](maintenance.md)）と障害を見分ける方法を説明します。プローブのパスは[運用ガイドのヘルスチェックの節](running.md#health-checks)にあります。

以下に書いた外部サービスの情報には、確認した日付と出典を添えています。サービスのプランや画面は変わるので、制限に頼る前にリンク先のページを確認してください。

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## どの URL を監視するか

| パス | 用途 | DB へのアクセス | メンテナンスモード中 |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | 死活監視、ステータスページ、Web クライアント | プロセスごとに 60 秒に最大 1 回 | `Retry-After` 付きの `503` と `{"status":"maintenance"}` |
| `GET /healthz` | プラットフォームの liveness プローブ | しない | `200` `{"status":"maintenance"}` |
| `GET /readyz` | ロードバランサーや Kubernetes の readiness プローブ | リクエストごと（同時のリクエストは 1 回のチェックを共有） | `200` `{"status":"maintenance"}` |

外部の監視は `/readyz` ではなく **`/api/v1/instance/health`** に向けてください。`/readyz` はリクエストのたびにデータベースに問い合わせるので、間隔の短い監視はスケールトゥゼロのデータベースを起こし続けます。プローブはプラットフォームのためのものです。
モジュール [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) と [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) は、プラットフォーム自身の監視から長い間隔で `/readyz` をチェックします。そのコストはそれぞれの README に書かれています。

`/api/v1/instance/health` の応答（認証なし、`Cache-Control: no-store`）は次のとおりです。

| 状態 | HTTP | 本文 |
| --- | --- | --- |
| 正常 | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| 利用不可（データベースに到達できない、またはスキーマが適用されていない） | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| メンテナンスモード | `Retry-After` 付きの `503` | `{"status":"maintenance"}` |

`/healthz` と `/readyz` は、正常なときに `200` と `{"status":"ok"}` を返します。データベースの準備ができていないとき、`/readyz` は problem document 付きの `503` を返します。
本文に理由、ホスト名、バージョンが含まれることはありません。原因はサーバーのログにあります。

`checked_at` は、応答のもとになったチェックの時刻です。サーバーはデータベースのチェックをプロセスごとに 1 分に最大 1 回実行し、その結果を使い回します。そのため 60 秒より短い間隔でポーリングする監視は同じ `checked_at` を見るだけで、得るものはありません。プロセスが複数あるときは、それぞれが自分のチェックを実行します。

[`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) を設定していても、この 3 つのパスはヘッダーなしで応答するので、CDN がヘッダーを失ったことはこれらには表れません。監視を問題の最初の兆候にしているなら、`https://<host>/about` のような CDN 経由のページも監視してください。

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## 監視を安く保つ

- **スケールトゥゼロのデータベース。** 監視の間隔が、データベースが停止するまでの待ち時間より短いと、データベースは停止しません。
  Neon は 5 分間アクティビティがないとコンピュートを停止します。Free プランではこれを無効にできません
  （2026-10-25 確認、[Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)）。
  目安として、間隔を 5 分以上にすれば、アイドルのデータベースはチェックの合間に停止でき、各チェックはチェックの時間とアイドル時間のぶんだけデータベースを起こします。
  連合するサーバーはほかのサーバーから一日中配送を受けるので、いずれにしてもデータベースは一日の大半起きています。
  追加のコストは静かな時間帯に限られます。
- **アイドル時のコストを最も重視するなら、安い組み合わせにします。** `/healthz`（データベースに一切触れません）を短い間隔で、データベースを使う
  `/api/v1/instance/health` を長い間隔（15 分以上）で監視します。`/healthz` はメンテナンスモード中も `200` を返すので、本文の `"status":"ok"` で判定してください。
- **リクエストそのものは小さなものです。** 5 分ごとに 1 回のチェックは月に約 8,640 リクエスト、1 分ごとなら約 43,200 リクエストです。これは Cloud Run の無料枠
  （月に 200 万リクエスト、180,000 vCPU 秒、360,000 GiB 秒。時間は 100 ms 単位で切り上げ。2026-10-23 確認、[Cloud Run pricing](https://cloud.google.com/run/pricing)）に収まります。

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## メンテナンスモードと障害を見分ける

メンテナンスモード中（[メンテナンスのガイド](maintenance.md)）、ヘルスエンドポイントは `503` を返し、ほとんどの監視サービスはこれを障害と同じに扱います。
ステータスコードだけでは区別できないので、本文で判定してください。

- 本文に `"status":"ok"` があれば正常、それ以外はダウンとして扱います。キーワード監視や JSON の値の監視でこれができます。
- 予定したメンテナンス中にアラートを出さないようにするには、サービスにメンテナンスウィンドウがあればそれを設定するか（下の表を参照）、
  始める前に監視を手動で一時停止します。
- UptimeRobot の Free プランにはメンテナンスウィンドウがありません。監視を一時停止するか、アラートを受け入れてください。

<!-- source: setting-up-the-monitors e91307d78a94 -->
## 監視を設定する

どのサービスでも同じことをチェックします。`GET https://<your-host>/api/v1/instance/health` の応答が `200` で、本文に `"status":"ok"` が含まれていれば正常です。

| サービス | 監視の種類 | 正常の条件 | メンテナンスウィンドウ | 確認日 |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | 本文に `"status":"ok"` が含まれる | Free にはない（Solo 以上） | 2026-10-25、[料金](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status または Keyword | `2xx`、または本文に `"status":"ok"` が含まれる | 未確認 | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` かつ `[BODY].status == ok` | 設定の `maintenance` | 2026-10-25、[README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword または JSON Query | 本文に `"status":"ok"` が含まれる、または `status` が `ok` | 組み込み | 2026-10-23、[README](https://github.com/louislam/uptime-kuma) |
| Upptime | `.upptimerc.yml` の `sites` | `200` かつ本文に `"status":"ok"` が含まれる | なし | 2026-10-25、[Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

種類 **Keyword** の監視を作成します。URL は `https://<your-host>/api/v1/instance/health`、キーワードは `"status":"ok"` とし、キーワードが存在**しない**ときにアラートを出すようにします。
Free プランでは 5 分間隔の監視を 50 個まで作成でき、キーワード監視も含まれます。
「Custom headers & statuses」とメンテナンスウィンドウは Solo プラン（有料、60 秒間隔）から使えます
（2026-10-25 確認、[料金](https://uptimerobot.com/pricing.md)）。チェックは本文で行うので、ここではキーワード監視で十分です。

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

次の 2 つの形が使えます（2026-10-25 確認、[Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/)、[Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)）。

- **HTTP status** 監視: `2xx` の応答を稼働中として扱うので、`200` は稼働中、`503`（利用不可またはメンテナンス）はインシデントを開きます。
- **Keyword** 監視: 「Alert us when」を「URL doesn't contain keyword」にし、キーワードを `"status":"ok"` にします。照合では大文字と小文字を区別しません。

Free プランのチェック頻度とメンテナンスウィンドウは未確認です。公式のページには Free プランについての記載がありません。無料枠の数（監視とハートビートを合わせて 10 個、ステータスページ 1 つ）は [料金のページ](https://betterstack.com/pricing) によるもので、2026-10-23 に確認しました。

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

`config.yaml` に次のように書きます（既定の間隔は `60s`。2026-10-25 確認、[README](https://github.com/TwiN/gatus)）。

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus には、予定した作業のためのトップレベルの `maintenance` セクションがあります（`start` は `hh:mm`、`duration`、`timezone` は既定で `UTC`、`every` は曜日のリストで、空なら毎日）。
README にはメンテナンスウィンドウ中のアラートを避けるためのものとしか書かれていないので、お使いのバージョンでの動作を確認してください。

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

種類 **HTTP(s) - Keyword** の監視をキーワード `"status":"ok"` で追加するか、**HTTP(s) - Json Query** の監視をクエリ `status`、期待値 `ok` で追加します。
受け付けるステータスコードは既定で `200-299` なので、メンテナンスの `503` も障害の `503` もダウンになります。Uptime Kuma には監視のウィンドウのためのメンテナンス機能があります
（[wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) に記載がありますが、詳細は未確認です）。
監視の種類は 2026-10-23 にソースで確認しました（[monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720)、
[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)）。

<!-- source: upptime 90c96c946d06 -->
### Upptime

`.upptimerc.yml` に次のように書きます（2026-10-25 確認、[Configuration](https://upptime.js.org/docs/configuration/)）。

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

既定では `20x` と `30x` が稼働中として数えられます。`200` だけを受け付けるには `expectedStatusCodes: [200]` を設定します。チェックは、すべてのサイトに共通のスケジュールで GitHub Actions から実行されます
（`workflowSchedule.uptime`。既定は 5 分ごとで、これは GitHub Actions で許される最短の間隔でもあります）。オプション名は推奨されないことを示していますが、
本文を調べるにはこの方法を使います。

<!-- source: a-status-page 64712ed6d8a5 -->
## ステータスページ

`status_page_url`（`MY_STATUS_PAGE_URL`、`https:` の URL。[設定リファレンス](../reference/configuration.md#status_page_url)を参照）に、
監視サービスの公開ページを設定します。サーバーはこのリンクを、`GET /api/v1/instance`、サーバーが表示するサインインのページとエラーページ、Web クライアント
（サーバーが応答しないときも含む）、メンテナンスのページに表示します。これで、サーバーにアクセスできない利用者も理由を知ることができます。
この値はデータベースではなく設定から読まれるので、データベースが落ちていても機能します。変更は再起動の後に反映されます。

ページそのものの選択肢は次のとおりです（このサーバーはステータスページの状態を取り込まないので、お知らせはサーバー全体のお知らせとしても投稿してください）。

- 選んだ監視サービスがホストするステータスページ（UptimeRobot の Free プランにはカスタマイズできないステータスページが 1 つ、Better Stack の無料枠には 1 つあります）、
  Gatus と Uptime Kuma が自分で提供するページ、または GitHub Pages 上の Upptime の静的サイト。
- Atlassian Statuspage。これは公開のための場所で、監視ではありません。コンポーネントは Operational、Under maintenance、Degraded performance、
  Partial outage、Major outage の 5 つの状態のいずれかを持ちます（API の値は `operational`、`under_maintenance`、`degraded_performance`、`partial_outage`、`major_outage`。
  2026-10-23 確認、[What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/)、[Statuspage API](https://developer.statuspage.io/)）。
  監視のアラートからコンポーネントの状態を自動で変える方法は未確認です（実際のアカウントでサービスごとの連携を設定する必要があります）。
