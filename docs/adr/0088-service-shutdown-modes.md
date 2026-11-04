# ADR-0088: メンテナンス・監視・サービスの終了

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1034, #1023, #1024

## Decision（決定）

サーバーを一時的に止める、外から見張る、計画的に閉じる、の 3 つを決める。§1〜§7 がサービスの終了、§8〜§14 がメンテナンスモードとその書き出し、§15〜§22 が外形監視とステータスページである。

### 1. 状態と遷移

```text
通常 ──予告──▶ 予告 ──クローズ（閉鎖日以降）──▶ クローズ（配送中）──自動──▶ 配送の完了 ──運営者が止める──▶ 停止（静的なページ）
 ▲               │
 └──予告の取り消し─┘
```

- `instance` モジュールに、イベント `service_closure_events`（種類 `announced` / `withdrawn` / `closed` / `delivery_completed`、操作した人、時刻）と、`announced` の部分型 `service_closure_schedules`（閉鎖日 `closes_at`・持ち出しの期限 `export_until`・運営者のメッセージ）を置き、投影 `service_closure_heads`（1 行）が現在の状態を持つ。
- 遷移はドメインの関数で検査する: `withdrawn` は予告のときだけ、`closed` は予告中で `closes_at` を過ぎたときだけ、`delivery_completed` はクローズのときだけ。`closed` の後に戻る遷移は無い。予告の日付の変更は新しい `announced` で表し、利用者に通知する。
- 日付に下限は設けない。運営者は `closes_at` を自由に決める（事業の都合で急に閉じるときにも使えるように）。`export_until` は `closes_at` 以後で、既定は `closes_at` の 30 日後、変えられる。予告から `closes_at` までが 30 日より短いとき、管理画面と CLI は告知の期間が短いと警告する（値は実装の後に調整する）。
- クローズへの切り替えは運営者の操作だけで行い、閉鎖日に自動では切り替えない。閉鎖日を過ぎると、管理画面と管理者への通知でクローズの操作を促す。
- 状態はリクエストとジョブごとに読む（`federation_mode` と同じく、変更はすぐ効く）。各入口は `instance.Closure` の値を受けて、`closure.Allows(state, 操作)` のようなドメインの 1 つの関数で判定する（ADR-0006 の `federation.Allows` と並べる）。
- 操作: 管理 API・管理画面・CLI（`my admin closure announce|withdraw|close|status`）。権限は新しい `manage_service_closure`（区分 `server`、組み込みのロールでは `admin` だけ）。予告と取り消しは管理画面からもできるが、`close` は管理画面では alertdialog（NFR-MOD-7）でドメイン名の入力を求め、CLI では 2 度の確認（`y` とドメイン名の入力。自動化には `--confirm-domain=<domain>`）を求める。確認の画面は、送る `Delete` の数の見積もり（ローカルアカウント数 × 宛先の inbox 数）と、続いているサポーターの加入の数を示す。
- すべての遷移を監査ログ（ADR-0058、`target_kind = instance`）に残す。

### 2. 各状態の振る舞い

| | 予告 | クローズ（配送中） | 配送の完了 | 停止（静的なページ） |
| --- | --- | --- | --- | --- |
| **Web** | 全画面に閉じられない帯（閉鎖日・持ち出しの期限・エクスポートと移転への導線）。ほかはいつもどおり | サインイン後は閉鎖の画面だけ（エクスポートの要求とダウンロード、CSV、サインアウト）。ほかの画面は理由を示す | 同左 | 静的な閉鎖のページ |
| **サインイン・登録** | サインインできる。登録は実効値を `closed` に固定（設定の値は変えない）、招待は使えない、審査待ちの申請は予告の開始で却下（理由: サービスの終了） | サインインとパスワードの再設定だけ。登録・招待はできない | 同左 | できない |
| **client API** | いつもどおり。`GET /instance` に `closure`（状態・`closes_at`・`export_until`・メッセージ） | 自分のアカウントの読み取り・エクスポート（FR-008 と CSV）・セッションの操作・`GET /instance` だけ。ほかは `403`（problem type `service-closed`）。ストリームと Web Push は止める | 同左 | なし |
| **通知・メール** | 予告・日付の変更・取り消しのとき、アプリ内の通知（新しい種類 `service_closure`）と、メールの確認が済んだ全利用者へのメール（[ADR-0086](0086-email-delivery.md) の取引のメール。見落とすとデータを失うので配信停止と苦情による抑止の対象にしない）。閉鎖日の 7 日前と前日にもメール | クローズのときにメール（持ち出しの期限）。ほかは認証のメールだけ | 持ち出しの期限の 7 日前にメール | なし |
| **サポーター（ADR-0072）** | 新しい加入は `409`。予告の開始で続いている加入すべてに `CancelAtPeriodEnd`（ADR-0072 §9 の廃止と同じ）を積み、通知する | 残っている加入を `CancelNow`（理由 `service_closed`）。課金は無効にできる状態になる | 同左 | — |
| **連合: 送るもの** | いつもどおり | `retract`（下の `Delete`）だけ。積まれていたほかの配送は実行時に `skipped`（ADR-0006 と同じ） | なし | なし |
| **連合: inbox** | いつもどおり | 署名の検証より前に `410 Gone` | 同左 | `410`（静的） |
| **連合: ローカルの Actor の GET** | いつもどおり | 凍結と同じ最小の文書（ADR-0058。空のプロフィール、`suspended: true`、鍵あり）。`Delete` の署名を検証させるため | `410 Gone`（`Tombstone`） | `410`（静的） |
| **連合: 投稿・コレクションの GET** | いつもどおり | `410 Gone`（`Tombstone`） | 同左 | `410`（静的） |
| **インスタンス Actor・WebFinger・NodeInfo** | いつもどおり。NodeInfo の `metadata.serviceClosure` に状態と日付 | インスタンス Actor は返す（署名つきの取得のため）。WebFinger は `410`。NodeInfo は状態を出す | インスタンス Actor も `410`。NodeInfo は状態を出す | `410`（静的） |
| **外の取得・取り込み・リレー** | いつもどおり | しない（リレーは脱退を送らず止める） | しない | — |

- inbox の `410` は、主要な実装がどれも再試行しない失敗として扱うので、連合先からの配送が早く止まる: Mastodon は 401・408・429 以外の 4xx（[`response_error_unsalvageable?`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/helpers/json_ld_helper.rb)）、Misskey は 429 以外の 4xx（[`StatusError`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/misc/status-error.ts)）、GoToSocial は 429 以外の 4xx（[`httpclient`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/httpclient/client.go)）、Akkoma は 405 と 410（[`PublisherWorker`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/workers/publisher_worker.ex)）を再試行しない（2026-10-23 にソースで確認）。Misskey は shared inbox の `410` で、そのサーバー全体への配送を止める（`goneSuspended`、同じく [`DeliverProcessorService`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/queue/processors/DeliverProcessorService.ts)）。
- 配送中に Actor の文書と鍵を返し続けるのは、鍵を取り直す受信側のためである。GoToSocial は、署名の鍵の取得が `410` だと tombstone を記録して inbox に `202` を返し、その `Delete` を処理しない（2026-10-23 に [`authenticate.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/authenticate.go) と [`federatingprotocol.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/federatingprotocol.go) で確認）。
- クローズの判定は `federation_mode`（ADR-0006）より優先する: クローズでは `retract` と、鍵を出すための `identify` だけを通す。`federation_mode` が `off` / `inbound` でも `Delete` は送る（予告の期間に連合を狭めた運営者も、外に残ったものを消せるように）。これは ADR-0006 の「`off` は何も通さない」に対する例外で、ADR-0006 自体は変えない（クローズは別の軸の状態として足す）。管理画面の確認でこの例外を示す。ドメインの処分（federation.md §11）はいつもどおり効き、凍結したドメインには送らない。
- メンテナンスモード（§8）はどの状態でも重ねて使える（すべてを止める）。ステータスページ（§20）への掲出は運用文書の手順に書く。

### 3. クローズの配送

- `closed` を記録したトランザクションで、ジョブ `instance.closure_delete_actors`（カーソルはアカウントの ID）を outbox に積む。ジョブはローカルのアカウント（インスタンス Actor・削除済みを除く。凍結中は含む）を 100 件ずつ、アカウントごとに `Delete`（Actor）の outbound activity を記録し、既存の展開（[federation.md §7.2](../architecture/federation.md#72-展開fan-out)。承認済みのフォロワー → 既知の shared inbox）に渡す。ライフサイクル（`account_lifecycle_events`）は変えない（消去ジョブを動かさない。§5）。
- 投稿ごとの `Delete` は送らない（アカウントの削除と同じく `Delete`（Actor）が代わりを務める）。
- 速度: 次のページの展開は、前のページの展開が終わってから積む。同時に動く配送はキューの同時実行数の設定で抑え、未処理の配送のジョブが上限（既定 10,000 件、Mastodon と同じ値。要調整）を超えている間は次のページを積まない。shared inbox への集約と `unavailable` のドメインの除外は既存のまま効く。
- 再試行と諦める時点: 既存の配送の方針（最大 9 回、約 4 日、使い切ったら dead job）。dead job は自動では再実行しない（運営者は既存の管理 API で再実行できる）。
- 完了: 定期ジョブが、クローズの `Delete` の配送がすべて終わった状態（`succeeded` / `permanent_failure` / `skipped` / dead job）で、展開も残っていないことを確かめたら `delivery_completed` を記録する。
- 進捗: `GET /api/v1/admin/closure`（と `my admin closure status`）が、処理したアカウント数と、配送の状態ごとの件数を返す。管理画面はそれを示し、ワーカーを止めないよう注意を出す。

### 4. 移転済みのアカウント（ADR-0089）

- 予告の帯と通知・メールは、エクスポートと並べて移転の設定の画面へ案内する。設定の画面は、予告中だけ「サーバーの終了の前に移転できます」と閉鎖日を示す。移転には移転先での別名（`alsoKnownAs`）の設定が要るので、手順の説明へのリンクを置く。
- 予告の期間中も `Move` の送信はいつもどおりできる。クローズの後は、client API がエクスポート以外の操作を断るので、`Move` を始められない（ADR-0089 は `Move` を `retract` に分類するが、クローズで通す `retract` は、このクローズの `Delete` と、クローズの前に積まれた配送だけである）。
- 移転済みのアカウント（`Move` を送ったもの）にも、クローズで `Delete`（Actor）を送る。フォロワーは移転で移っているので、`Delete` で失うものは連合先のキャッシュの「移転しました」の表示だけで、ドメインが止まったあとは取り直しで消える。全アカウントを同じ扱いにして、例外の経路を作らない。ただし順序は最後のフェーズにまとめ、配送中の間は移転済みの Actor の GET に `movedTo` を残す（移転を見に来た相手が辿れるように）。

### 5. ローカルのデータ

- サーバーは自分ではデータを消さない。配送の完了の後も、`export_until` まではクローズのままエクスポートを提供する。`export_until` を過ぎたら、運営者が静的なページへ切り替え、DB・Blob・バックアップを破棄する（運用文書に書く）。
- ADR-0058 の保持期間は、サーバーが動いている間はいつもどおり効く。`retention_holds`（保全）がある場合は、`close` の確認と運用文書で、破棄の前に対象の記録を CLI で書き出して保管するよう促す（保全の義務はサーバーを閉じても消えないため）。

### 6. 停止の後

- `my admin closure export-html` が、§12 のメンテナンス HTML の書き出しと同じ仕組みで、閉鎖のページ（サーバー名・閉鎖日・運営者のメッセージ・ロケールごと）と、リバースプロキシ・CDN の設定例（`/ap/*`・`/.well-known/webfinger`・`/@*` は `410`、それ以外は閉鎖のページ）を書き出す。
- 運用文書は、ドメインを手放さず、およそ 1 年は `410` を返し続けるよう勧める。URI は UUID（ADR-0018）なので同じ `id` の Actor が作られる心配は小さいが、ドメインを別人が取るとハンドル（`acct:alice@example.org`）と表示用 URL が別人のものになり、なりすましに使える。

### 7. サービスの終了の機能要件

`docs/requirements/01-functional.md` の「運用」の表に次を持つ。

| ID | 要件 | 優先度 |
| --- | --- | --- |
| FR-808 | サービスの終了の予告: `manage_service_closure` を持つ人が、閉鎖日とデータの持ち出しの期限を決めて終了を予告し、変更し、取り消す。予告の間は登録・招待・サポーターの新しい加入を止め、続いている加入を期間の終わりで解約し、全画面の帯・`GET /instance`・NodeInfo・通知・メールで知らせ、エクスポートと移転（`Move`）へ案内する。操作は監査ログに残す | S |
| FR-809 | サービスのクローズ: 閉鎖日の後、取り消せない操作として（管理画面の確認・CLI の 2 度の確認）クローズに切り替え、全ローカルアカウントの `Delete`（Actor）を連合先へ配送し、進捗を管理画面と CLI で示す。クローズの間、利用者はサインインしてデータのエクスポートだけができ、ほかの操作は client API が断る。連合の GET と inbox は `410 Gone` を返し、配送の完了後は静的な閉鎖のページを書き出せる | S |

### 8. メンテナンスモードの切り替え

メンテナンスモードは通常のサーバーとは別の小さな HTTP ハンドラで、設定の一部だけを読んで起動する。切り替えの手段も応答に使う値も、DB・KV・キュー・Blob・Mailer・外への HTTP に依存しない。`my serve` を動かせないときの補助として、同じテンプレートで静的な HTML を書き出す（§12）。

- 設定キー `maintenance`（真偽値。環境変数 `MY_MAINTENANCE`、TOML、`my serve -maintenance`）が真なら、`my serve` はメンテナンスモードで起動する。切り替えは再起動で行う。
  - 起動中の切り替えは採らない。複数のインスタンス（Cloud Run・Lambda・k8s のレプリカ）に状態を揃えるには共有の置き場が要り、それは DB や KV そのものだから。
    Cloud Run・Lambda・k8s では環境変数の変更が新しいリビジョン（ロールアウト）になり、通常のデプロイと同じ操作で全インスタンスが揃う。
- `role`（web / worker / all）にかかわらず、メンテナンスモードのプロセスは HTTP だけを受け、ジョブの取得・定期実行・ストリームのブローカーへの接続をしない。
- 起動の経路を分ける: `cmd/my` は設定の読み込みの前に `maintenance` だけを見て、真なら `platform.NewMaintenanceApp` を組み立てる。
  これは `platform.NewApp` を呼ばず、アダプタを 1 つも作らない。応答とテンプレートは新しいパッケージ `internal/interface/maintenance`（標準ライブラリと `internal/i18n` だけに依存）に置く。

### 9. メンテナンスモードの設定の読み込みと検証

- メンテナンスモードで読む設定は次に限る。アダプタの選択と接続のキー（`db`・`database_url`・`queue`・`blob`・`kv`・`mail`・`stream_broker` など）は**読まず、検証もしない**。
  作業中に DB の URL が一時的に無効でも起動できるようにするため。
  - 共通: 待ち受けのアドレス、サーバーのホストとオリジン（Web・API）、ログとテレメトリのうち外への接続が要らないもの（標準エラーへのログ）。
  - メンテナンス用（新設）:

| キー | 環境変数 | 内容 |
| --- | --- | --- |
| `maintenance` | `MY_MAINTENANCE` | 真ならメンテナンスモードで起動する |
| `maintenance_server_name` | `MY_MAINTENANCE_SERVER_NAME` | ページに出すサーバーの名前。空ならホスト名 |
| `maintenance_message` | `MY_MAINTENANCE_MESSAGE` | 告知文（プレーンテキスト、上限 2,000 文字）。ADR-0009 §2 の管理者が入力する値と同じく 1 つの値をどのロケールでも出す |
| `maintenance_until` | `MY_MAINTENANCE_UNTIL` | 終了予定の時刻（RFC 3339）。任意 |
| `maintenance_retry_after` | `MY_MAINTENANCE_RETRY_AFTER` | `maintenance_until` が無いか過ぎたときの `Retry-After` の秒数。既定 600 |

- ステータスページの URL は §20 の設定キー `status_page_url`（`MY_STATUS_PAGE_URL`、`https` のみ）を読む。設定キーなのでメンテナンスモードでも読め、書き出しも同じ値を使う。
- `my config check -maintenance` は上の範囲だけを検証する。通常の `my config check` は `maintenance_*` の値の形も検証する。
- 通常の運用で `maintenance_*` を常に設定しておいてよい（`maintenance` が偽なら使われない）。

### 10. メンテナンスモードの応答の表

共通: どの応答にも `Retry-After`（`maintenance_until` が未来ならそこまでの秒数、上限 86,400。それ以外は `maintenance_retry_after`）と
`Cache-Control: no-store` と、ADR-0019 のセキュリティヘッダを付ける。応答は振り分けのための最小のルーティングだけで決め、通常のルート表は使わない。

| 対象 | 判定 | 状態 | 本文 |
| --- | --- | --- | --- |
| `/healthz` | パス | 200 | `{"status":"maintenance"}` |
| `/readyz` | パス | 200 | `{"status":"maintenance"}`。ロードバランサ・k8s がインスタンスを外さず、メンテナンスの応答が利用者に届くように 200 にする。監視は本文で区別する（§16） |
| 外形監視の端点 `GET /api/v1/instance/health`（§15） | パス | 503 | `{"status":"maintenance"}`（形は §15）。監視がキーワードで「メンテナンス中」と判定できる |
| 上の端点を除く client API（`/api/*`）と OAuth の端点のうち JSON を返すもの（`POST /oauth/token`・`/oauth/token/refresh` など） | パス | 503 | `application/problem+json`、`type: /api/v1/problems/maintenance`、`title` は `Accept-Language` で選んだ「メンテナンス中」、拡張メンバー `message`・`until`（設定があれば）・`status_page_url`（§20 の設定があれば） |
| ActivityPub（Inbox の POST、Actor・オブジェクト・コレクションの GET） | `Accept` / `Content-Type` が ActivityPub、または `/ap/*`・`/inbox` | 503 | `application/json` の短い本文（`{"error":"maintenance"}`） |
| WebFinger（`/.well-known/webfinger`）・NodeInfo（`/.well-known/nodeinfo`、`/nodeinfo/*`）・host-meta | パス | 503 | 同上 |
| キューの push の受け口（Cloud Tasks・Lambda の SQS 受け口）・決済の webhook など外部からの通知 | パス | 503 | 同上。送り手の再試行に任せる |
| Web の HTML（ページ・サインイン・認可同意など、上に当たらない `GET`・`HEAD`） | 残り | 503 | メンテナンスのページ（§11） |
| Web クライアントの SPA と静的なアセット（JS・CSS・`sw.js`・`manifest.webmanifest`） | — | 503 | 出さない。メンテナンスのページが自己完結（インライン CSS・スクリプトなし）なので要らない |
| 上に当たらない `POST` などの非安全メソッド | 残り | 503 | メンテナンスのページ（`Accept` が JSON なら Problem） |

- **Web クライアントと Tauri アプリ**: Problem の `type` が `/api/v1/problems/maintenance` なら、画面全体をメンテナンスの表示（`message`・`until`・ステータスページへのリンク）に替え、
  `Retry-After` の後か利用者の操作で再読み込みする。**503 でトークンを捨てない・サインアウトしない**。キャッシュ済みの Service Worker が古いシェルを出しても、API の応答で同じ表示になる。
  ブラウザは `sw.js` の更新の取得が 2xx でなければ更新をやめ、登録済みの Service Worker を使い続ける（2026-10-23 に [Service Workers の Update 手順](https://w3c.github.io/ServiceWorker/#update-algorithm)で確認。登録を消すのは既存の worker が無いときだけ）。
- **連合先**: 503 と `Retry-After` は、こちらの配送の分類（[federation.md §7.3](../architecture/federation.md)）と同じく、送り手から見て「再試行する失敗」に当たる。
  - Mastodon は 5xx を再試行する（再試行しないのは 501 と、401・408・429 を除く 4xx）。再試行は 16 回で、`n` 回目の待ちは `n⁴ + 15` 秒にジッタ（最大 `0.5 × n⁴`）を足したもの（合計およそ 2〜3 日）。
    `Retry-After` は読まない。失敗した試行のたびにその日（UTC）を記録し、異なる 7 日に達したドメインを配送の対象から外す（`DeliveryFailureTracker`。こちらの `InstanceHealth` はこれに合わせている）。
    出典: [`delivery_worker.rb`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/workers/activitypub/delivery_worker.rb)・[`delivery_failure_tracker.rb`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/lib/delivery_failure_tracker.rb)・[`json_ld_helper.rb`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/helpers/json_ld_helper.rb)（2026-10-23 に確認）。
  - Misskey は 5xx を再試行し（再試行しないのは 429 を除く 4xx）、既定 12 回、待ちは `(2ⁿ − 1)` 分（上限 8 時間）に最大 20% のジッタで、合計およそ 1.5 日。`Retry-After` は読まない。
    失敗すると相手を「応答なし」にし、その状態が 7 日続くと自動で配送を停止する（`autoSuspendedForNotResponding`）。
    出典: [`DeliverProcessorService.ts`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/queue/processors/DeliverProcessorService.ts)・[`QueueService.ts`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/QueueService.ts)・[`QueueProcessorService.ts`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/queue/QueueProcessorService.ts)（2026-10-23 に確認）。
  - Akkoma は配送のジョブ（`federator_outgoing`）を 6 回まで試し、待ちは 16 秒・約 1 分・約 12 分・約 2.3 時間・約 21 時間で、合計およそ 1 日（[`config.exs`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/config/config.exs)・[`publisher_worker.ex`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/workers/publisher_worker.ex)・[`worker_helper.ex`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/workers/worker_helper.ex)、2026-10-23 に確認）。
  - Pleroma は配送のジョブを 13 回まで試し、`n` 回目の後の待ちは `n⁵ + 15` 秒にジッタ（最大 `30n` 秒）を足したもので、合計およそ 7 日。最後の試行も失敗すると相手を到達不能とし、以後の配送のジョブを取り消す
    （2026-10-23 に git.pleroma.social の GitHub ミラーの [`publisher_worker.ex`](https://github.com/minibikini/pleroma/blob/149236e468c453495e0dcf83a9728057436b463b/lib/pleroma/workers/publisher_worker.ex) で確認）。
  - 最も早く尽きるのが Akkoma のおよそ 1 日なので、メンテナンスは**数時間以内**を目安にし、1 日を超えると連合先の再試行が尽きて受け取れない活動が出る。運用の文書にこの目安を書く。
- **検索エンジン**: Google は 503 を一時的な停止として扱い、`Retry-After` を再クロールの時期に使うことがある。長く続く 503 は恒久的な停止と見なされ索引から外されうる（2026-10-23 に [Google Search Central Blog](https://developers.google.com/search/blog/2011/01/how-to-deal-with-planned-site-downtime) で確認）。
  `/robots.txt` も 503 を返す。`robots.txt` が 5xx の間、Google は最初の 12 時間はサイトのクロールを止め、その後は最後に取得できた版を使う（2026-10-23 に [robots.txt の仕様](https://developers.google.com/crawling/docs/robots-txt/robots-txt-spec)で確認）。
  数時間のメンテナンスでクロールが止まるのは望ましく、`robots.txt` の中身は DB の設定（FR-725）から作るのでメンテナンス中には作れない。
  Google は 1〜2 日を超えてサイトを止めるときは `robots.txt` に 503 を返さないよう勧めているので（[サイトの一時停止](https://developers.google.com/search/docs/crawling-indexing/pause-online-business)）、それより長い停止はメンテナンスモードではなくサービスの終了（§1〜§7）で扱う。

### 11. メンテナンスのページ（共通のテンプレート）

- `internal/interface/maintenance` の 1 つの `html/template` を、メンテナンスモードの応答と書き出しの両方が使う。
- 中身: サーバーの名前、「メンテナンス中」の見出しと説明、告知文（`dir="auto"`・`lang=""`。ADR-0009 §5）、終了予定の時刻（あれば）、ステータスページへのリンク（あれば）。
- 文言は新しいメッセージ（`server` の名前空間、`maintenance.*`）を `locales/en` と `locales/ja` に足し、生成した型付きアクセサで整形する。
  ロケールは `Accept-Language` → `en` で選ぶ（利用者設定とインスタンスの既定ロケールは DB にあるので使わない）。ルート要素の `lang` と `dir` をロケールから設定する。
- 終了予定の時刻: Go の側の名前空間は `date` / `time` の引数を使えない（AGENTS.md の i18n の規則）ので、`<time datetime="…">` に UTC の RFC 3339 の値を入れ、
  表示は ISO 8601 の UTC（例 `2026-10-24 03:00 UTC`）を文字列の引数として渡す。ロケールに合わせた整形はしない。
  Go の側に CLDR の日付の整形を持つと、メンテナンスのページのためだけに整形の実装とそのテストが要るため。
- 見た目: CSS は `<style>` にインラインで 1 つ。色・文字・余白は `theme.css` の既定のトークン（ライト / ダーク）から生成した値を使い、`prefers-color-scheme` でダークにする。CSS は論理プロパティだけを使う。
  画像は使わない（ロゴを出すなら `data:` のインライン SVG）。スクリプトは使わない。
- CSP: `default-src 'none'; style-src 'sha256-<インライン CSS のハッシュ>'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`。
  ADR-0019 §9 を緩めないため、`'unsafe-inline'` ではなくハッシュで許す。書き出した HTML には同じ方針を `<meta http-equiv="Content-Security-Policy">` で入れる（`frame-ancestors` は meta では効かないので除く）。

### 12. メンテナンスのページの書き出しコマンド

- 形: `my maintenance export -out <dir> [-lang en,ja] [-config <file>] [-offline]`。
- 出力: 自己完結した HTML（インライン CSS、外部の資源なし）を、言語ごとに `<dir>/<lang>.html`、`-lang` の言語を順に並べた 1 枚を `<dir>/index.html` に書く。
  `index.html` は `Accept-Language` で振り分けられない前段（CloudFront・Cloud Storage のエラーページなど）向けで、どの言語の読み手にも 1 枚で伝わるように全言語を並べる。
  `<lang>.html` は nginx などで振り分ける前段向け。
- 値の出所: 設定の `maintenance_*` を使う。`-offline` でなく DB に届けば（短いタイムアウト）、インスタンスの名前（設定の `maintenance_server_name` が空のとき）・
  既定ロケール（`-lang` が無いときの先頭）・テーマのトークンの上書き（ADR-0059 §2。任意の CSS は含めない）を DB から読んで焼き込む。
  届かなければ警告を出して設定と既定のテーマで書き出し、終了コードは 0 にする。
- `my serve` を動かさずに実行でき、メンテナンスモードと同じテンプレートなので、どちらから出しても同じ見た目と文言になる。クローズ後のページ（§6）もこのテンプレートと書き出しを使う。

### 13. ターゲットごとの置き方（運用文書の範囲）

`docs/operations/maintenance.md` を新設し、次を書く。IaC のテンプレートへの組み込みは M6（ADR-0010 の `deploy/`）。

- 手順: 告知（§20 のステータスページ、FR-726 のお知らせ）→ `MY_MAINTENANCE=true` で再デプロイ → 作業 → 外して再デプロイ。
  push 型のキューは作業の前に止める。止めないと 503 の再試行で試行回数を使い切る。
  - Cloud Tasks: `gcloud tasks queues pause` で配信を止め（タスクの追加は続けられる）、`resume` で再開する。試行の上限はキューの `retryConfig.maxAttempts`（ドキュメントの既定の例は 100、`maxBackoff` 3600 秒）で決まる（2026-10-23 に [キューの設定](https://cloud.google.com/tasks/docs/configuring-queues)と [Queue の REST リファレンス](https://cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues)で確認）。
  - SQS（Lambda のイベントソース）: イベントソースマッピングの `Enabled` を偽にするとポーリングと呼び出しが止まる（2026-10-23 に [UpdateEventSourceMapping](https://docs.aws.amazon.com/lambda/latest/api/API_UpdateEventSourceMapping.html) で確認）。
    止めないと失敗のたびに受信回数が増え、redrive policy の `maxReceiveCount` に達したメッセージはデッドレターキューへ移る（2026-10-23 に [SQS のデッドレターキュー](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html)で確認）。
- Cloud Run・Lambda: 環境変数を変えた新しいリビジョン。`my serve` も動かせないときは、前段のエラーページに `index.html` を置く。
  - CloudFront: カスタムエラーレスポンスで、オリジンの 5xx に S3 などに置いた `index.html` を返し、ビューアへの状態コード（`ResponseCode`）を 503 にできる。
    エラーページはオリジンと別の場所（S3）に置く（同じオリジンだと 5xx の間は取れない）。`Retry-After` はレスポンスヘッダーポリシーのカスタムヘッダーで付けられるが、ポリシーは付けたキャッシュビヘイビアのすべての応答に付くので、メンテナンスの間だけポリシーを付ける
    （2026-10-23 に [CustomErrorResponse](https://docs.aws.amazon.com/cloudfront/latest/APIReference/API_CustomErrorResponse.html)・[エラーページの設定](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/custom-error-pages-procedure.html)・[レスポンスヘッダーポリシー](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/understanding-response-headers-policies.html)で確認）。
  - Google Cloud: カスタムエラーレスポンスのポリシーは**グローバル外部アプリケーションロードバランサだけ**が持ち（リージョンとクラシックは非対応）、バックエンドの 5xx に、バックエンドバケット（Cloud Storage）の `index.html` を状態コードを上書き（`overrideResponseCode`）して返せる。
    リクエストのヘッダー（`Content-Type` など）で HTML と JSON を出し分けられる。エラーの取得元に定義したカスタムレスポンスヘッダーは返す応答に付かないので、この経路では `Retry-After` を返せない。
    ポリシーが効くのはバックエンドから来た応答だけで、Google Front End が作る応答には効かない（2026-10-23 に [カスタムエラーレスポンスの概要](https://cloud.google.com/load-balancing/docs/https/custom-error-response)で確認）。
    Cloud Run を直接（ロードバランサなしで）公開する最小構成には前段が無いので、メンテナンスモード（§8）だけで対応する。
- Docker compose: `MY_MAINTENANCE=true` で `my` のサービスを再作成。前段に nginx などがあれば `<lang>.html` を `error_page 502 503 504` に置く例。
- microk8s: Deployment の環境変数の変更（ロールアウト）。Ingress の既定のバックエンド・エラーページの例。
- ALB の固定応答は状態コード・`Content-Type`・最大 1,024 文字の本文だけを持ち、`Retry-After` などのヘッダを付けられない（2026-10-23 に [FixedResponseActionConfig](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_FixedResponseActionConfig.html) で確認）。
  書き出した HTML は 1,024 文字に収まらないので ALB の固定応答には置けず、短い文と 503 だけを返す例を書く。`Retry-After` を返せない前段では、連合先は §10 の自前の間隔で再試行する。

### 14. メンテナンスの機能要件

`docs/requirements/01-functional.md` の「運用」の表に次を持つ。

| ID | 要件 | 優先度 |
| --- | --- | --- |
| FR-805 | メンテナンスモード: 設定 `maintenance`（環境変数・設定ファイル・`my serve -maintenance`）で、DB・KV・キュー・Blob・Mailer・外への HTTP に触れずに起動し、Web にはメンテナンスのページを、client API には `/api/v1/problems/maintenance` の Problem を、ActivityPub・WebFinger・NodeInfo には応答を、いずれも `503` と `Retry-After` で返す。`/healthz`・`/readyz` は 200 でメンテナンス中と分かる本文を返す。告知文・終了予定・ステータスページへのリンクを設定から出し、文言は `Accept-Language` で選ぶ | M |
| FR-806 | メンテナンスのページの書き出し: `my maintenance export` が、メンテナンスモードと同じテンプレートで、言語ごとの自己完結した静的な HTML を書き出す。DB に届けばサーバーの名前とテーマを焼き込み、届かなくても設定から書き出せる | S |

### 15. 状態エンドポイント `GET /api/v1/instance/health`

- 認証なし。クライアント API の一部として OpenAPI に載せる（ADR-0016）。Web クライアントも、通信の失敗が「サーバーの障害」か「自分の回線」かを見分けるのに使える
- 判定は readiness と同じ `App.Ready`（DB に届き、スキーマが適用済み）。`/readyz` と同じ 3 秒の上限を持つ
- **結果をプロセスの中で 60 秒キャッシュ** し（60 秒は固定で、設定にしない）、同時の要求は 1 回の検査にまとめる（`singleflight`）。1 プロセスが DB に触れるのは監視の頻度や要求の数によらず最大で 1 分に 1 回。
  KV を使うレート制限（`kernel.RateLimiter`）は、KV が DB のこともあり DB を起こしうるので使わない。キャッシュがレート制限の代わりになる
- 応答（`Content-Type: application/json`、`Cache-Control: no-store`）:

  | 状態 | HTTP | 本文 |
  | --- | --- | --- |
  | 正常 | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
  | 障害（DB に届かない・スキーマ未適用） | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
  | メンテナンス中（§8） | `503` と `Retry-After` | `{"status":"maintenance"}`。§18 |

- 本文はこの 2〜3 個のキーだけで、値は固定の語。理由・エラーの文言・ホスト名・版・アダプタの名前を出さない（ADR-0019）。詳細はいまのとおりログと失敗の数の指標（ADR-0055 §12）に残す。
  `status` は機械向けの語で、ローカライズしない
- 部品ごとの状態（キュー・KV・Blob・配送の滞留・失敗したジョブ）は v0 の公開の応答に入れない。滞留や失敗は「落ちている」ではなく運営者が見る量なので、
  管理画面（NFR-OPS-3）とサービスの指標（ADR-0055）で扱う

### 16. プローブとの分担（`/healthz`・`/readyz` の意味は FR-802 と §10 のまま）

| | `/healthz` | `/readyz` | `/api/v1/instance/health` |
| --- | --- | --- | --- |
| 使う者 | プラットフォームの liveness | ロードバランサ・Kubernetes の readiness | 外形監視・ステータスページ・Web クライアント |
| DB に触れるか | 触れない | 毎回（同時の要求は `singleflight` でまとめる） | 最大 1 分に 1 回 |
| メンテナンス中（§8） | `200 {"status":"maintenance"}` | `200 {"status":"maintenance"}`（インスタンスを外さない） | `503`・`Retry-After`・`{"status":"maintenance"}` |

- `/readyz` にも `singleflight` を足し、連打されても同時の検査は 1 本にする（意味は変えない）
- 運用文書では、外形監視に `/readyz` を向けないよう案内する。`/healthz` を監視するときは、メンテナンス中も 200 になるので、本文に `"ok"` があることを判定に使う

### 17. 費用の目安と監視の頻度

- scale to zero の DB で、監視の間隔が DB の停止までの時間（Neon は 5 分）より短いと、DB は止まらなくなる。連合サーバーは外からの配送で DB が起きている時間が長いので（[deployment-targets.md](../architecture/deployment-targets.md)）、増える費用は無通信の時間帯の分に限られる
- アイドル時の費用を優先する運営者には、`/healthz`（DB に触れない）を監視し、DB を含む検査は間隔を長くする構成を運用文書で示す。サーバーレスでは、監視の要求そのものの費用は小さい。5 分ごとで月約 8,640 回、1 分ごとで約 43,200 回で、
  Lambda の無料枠（月 100 万リクエスト・40 万 GB 秒、超えると 100 万リクエストあたり $0.20、時間は 1 ms 単位。2026-10-23 に [AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/) で確認）に収まる。
  Cloud Run のリクエストごとの課金も、無料枠（月 200 万リクエスト・18 万 vCPU 秒・36 万 GiB 秒。時間は 100 ms 単位に切り上げ）に収まる
  （2026-10-23 に [Cloud Run pricing](https://cloud.google.com/run/pricing) で確認）

### 18. メンテナンスモードとの関係

- メンテナンスモード（§8）の間、`/api/v1/instance/health` は DB に触れずに `503`・`Retry-After`（値は §10 に従う）・`{"status":"maintenance"}` を返す。
  §10 の応答の表で、client API のほかのパス（maintenance の Problem）とは別の行になり、形は §15 が持つ。本文の形は正常・障害・メンテナンス中で同じ `{"status": ...}`
- 多くの外形監視は 2xx 以外を「停止」とみなすので、`503` だけではメンテナンスと障害を区別できない。区別は本文のキーワード（`"maintenance"`）か JSON の値と、各サービスのメンテナンスの時間帯の設定で行うよう運用文書に書く。
  UptimeRobot の Free にはメンテナンスの時間帯がないので（§19）、メンテナンス中は監視を手で止めるか、停止の通知を受け入れる

### 19. 主な外形監視との対応表

| サービス | 推奨の監視の形 | 判定 | 確認 |
| --- | --- | --- | --- |
| UptimeRobot | キーワードの監視（Free で使える。間隔 5 分・50 監視） | 本文に `"status":"ok"` があれば正常 | 2026-10-23 に [料金ページ](https://uptimerobot.com/pricing.md) で確認。状態コードの扱いとヘッダを変える「Custom headers & statuses」とメンテナンスの時間帯は Free になく、Solo（有料、間隔 60 秒）から |
| Better Stack | HTTP の状態コードの監視（2xx を正常とみなす）か、キーワードの監視（本文にキーワードが無ければ通知。大文字と小文字を区別しない） | `200`、または本文に `"status":"ok"` | 2026-10-23 に [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/)・[Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)・[料金](https://betterstack.com/pricing)（Free は監視とハートビートで 10・ステータスページ 1）で確認 |
| Gatus | 条件 `[STATUS] == 200` と `[BODY].status == ok`。メンテナンスの時間帯を設定できる | 同左 | [README](https://github.com/TwiN/gatus)、2026-10-23 確認（既定の間隔 60 秒） |
| Uptime Kuma | HTTP(s) - Keyword（本文に `"status":"ok"`）か JSON Query（`status` が `ok`）。受け付ける状態コードの既定は `200-299` | 同左 | 2026-10-23 に [monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720)・[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349) で確認 |
| Upptime | GitHub Actions からの HTTP の監視。既定で 2xx と 3xx を正常とみなし、`__dangerous__body_down_if_text_missing` で本文の語を条件にできる | `200` と本文に `"status":"ok"` | 2026-10-23 に [Configuration](https://upptime.js.org/docs/configuration/) で確認（間隔は既定・最短とも 5 分） |
| Atlassian Statuspage | 監視ではなく掲出先。上の監視の通知や API から部品の状態を変える | — | 部品の状態は Operational・Under maintenance・Degraded performance・Partial outage・Major outage の 5 つ（2026-10-23 に [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/) で確認）。API の値は `operational`・`under_maintenance`・`degraded_performance`・`partial_outage`・`major_outage`（2026-10-23 に [Statuspage API](https://developer.statuspage.io/) の部品の `status` の enum で確認）。監視の通知から部品の状態を自動で変える連携は要検証（サービスごとの連携の設定が要る） |

### 20. ステータスページの URL（設定 `status_page_url`）

- 設定 `status_page_url`（環境変数 `MY_STATUS_PAGE_URL`・設定ファイル。ADR-0010）: `https:` の絶対 URL のみ（空 = 未設定）。`my config check` で検証する。変更はプロセスの再起動で反映する
- 設定に置くのは、**DB に届かないときにこそ要る値** だから。メンテナンスモード（§8）はアダプタを開かず設定だけを読み、この同じキーをメンテナンスのページ・Problem の `status_page_url`・
  書き出し（`my maintenance export`）に使う。メンテナンス専用のキーは持たない
- [instance-settings.md](../operations/instance-settings.md) の「運営中に変える方針の値はインスタンス設定」という目安からは外れる。ステータスページの URL はめったに変わらず、
  デプロイの構成（どこで運用の状態を告げるか）に近いので、再起動での反映で足りると判断する。同じキーをインスタンス設定に置かない（両方に同じキーを持たない規則）
- 出す場所:
  - `GET /api/v1/instance` の `status_page_url`（未設定なら出さない）
  - サーバーの情報のページ（`/about`）と、サーバーが描画するサインインの画面
  - Web クライアント: 設定の画面のヘルプの区画と、エラー・オフラインの表示（G16・G17）。サーバーが答えないときは `GET /instance` を読めないので、
    **Web クライアントは最後に読んだ `status_page_url` をアプリが残し（Service Worker は API に答えないので、オフラインのホームのタイムラインと同じくアプリが残す。web-client §11、ADR-0028）、そこから URL を出す**
  - メンテナンスモードの応答と書き出したメンテナンスのページ（§10・§12）
  - DB の障害で返す HTML のエラーページ
- NodeInfo の `metadata` には v0 では足さない。Mastodon の NodeInfo の `metadata` にはステータスページのキーがなく（[node_info/serializer.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/serializers/node_info/serializer.rb#L40-L47)、2026-10-23 確認）、
  Misskey のリポジトリにも `statusPage`・`status_page` は見当たらない（GitHub のコード検索、2026-10-23）。Akkoma の `metadata` にもない（[nodeinfo.ex](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/web/nodeinfo/nodeinfo.ex#L52-L92)、2026-10-23 確認）。対応するキーが分かったら足す
- Mastodon は同じ値を管理画面の設定 `status_page_url` に持ち、インスタンスの API の `configuration.urls.status` と Web のフッターに出す
  （[instance_serializer.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/serializers/rest/instance_serializer.rb#L59-L67)、2026-10-23 確認）。出す場所はこれと揃う

### 21. ステータスページの状態の取り込み

v0 では取り込まない。サービスごとに API の形が違い、取得には `kernel.HTTPClient`・定期のジョブ・キャッシュ・取得の失敗の扱いが要る。
告知はサーバー全体へのお知らせ（FR-726）で代えられる。

### 22. 外形監視とステータスページの機能要件

- **FR-807**（運用、M）: 外形監視向けの状態エンドポイント: 認証なしで、サーバーの状態（正常・障害・メンテナンス中）を HTTP の状態コードと JSON で返す。DB の検査の結果をキャッシュし、監視の頻度によらず DB に触れる回数に上限を設ける。内部の情報（ホスト名・版・エラーの文言）を出さない
- **FR-810**（運用、S）: ステータスページの URL: 設定で `https:` の URL を与えると、DB に届かないときも含めて、クライアント API・サーバーの情報ページ・サインインの画面・Web クライアントのエラーとオフラインの表示・メンテナンスのページに出す

### 見直す条件

- 主要な実装のどれかが、凍結と同じ最小の Actor の文書を返す配送中の `Delete`（Actor）を受け付けない（相手が取り直して Actor が残っていると判断する）ことが相互運用テストで分かった場合: 配送中から Actor の GET を `410` にし、鍵は既知の鍵での検証に任せる
- `Delete`（Actor）で投稿を消さない主要な実装が見つかった場合: そのドメインに限って投稿ごとの `Delete` を足す
- 配送数の見積もりが運営者の想定する費用を超える事例が出た場合: 既知の shared inbox への送信を「フォロワーかフォローのいるドメイン」に絞る段を足す
- 運用者が無停止での切り替え（再デプロイなし）を強く求め、かつ全ターゲットで共有できる DB 以外の置き場（例: 設定の再読み込みをする k8s の ConfigMap、Cloud Run の再デプロイなしの環境の変更）が使えると分かった場合: 起動中の切り替えを足す
- 連合先の再試行が 503 でも短時間で尽きる（数時間のメンテナンスで配送を止められる）と分かった場合: §10 の応答の表を見直す
- 主な外形監視が `503` と本文から「メンテナンス」を自動で識別できる形（例: 特定の状態コードやヘッダ）を持つと分かった場合: §18 の応答をそれに合わせる
- 運営者が部品ごとの状態を外から監視する需要が強い場合: トークン付きの詳しい応答を足す。その場合も公開の応答は §15 のまま
- ステータスページをサーバー自身が持つ方針になった場合: §20・§21 を見直す

## 経緯

サーバーを畳むとき、個々のアカウントの削除（FR-008）とお知らせ（FR-726）しかなく、DB を消して VM を止めるだけでは、利用者はデータを持ち出せず移転もできず、連合先にはキャッシュが残り再試行が続く（#1034）。Mastodon の `tootctl self-destruct`（モードの間はデータのダウンロードだけができ、各アカウントの `Delete`（Actor）を既知の inbox へ送り、ローカルのデータは消さない。2026-10-23 に [tootctl の文書](https://docs.joinmastodon.org/admin/tootctl/#self-destruct) で確認）を参考に、始め方と同じく終わり方を v0 で運営者に渡すことにした。

実際の相互運用（federation-live の `TestClosure`、2026-10-25、Mastodon 4.7・GoToSocial 0.22・Misskey 2026.10）で、配送中の最小の Actor の文書の下で 3 実装ともクローズの `Delete`（Actor）を受け付け、アカウントと取り込んだ投稿を消した。クローズの直後に相手が送った配送は inbox の `410` を受けて再試行されず、Misskey は `410` を受けてこのサーバーを `goneSuspended` にした。配送はすべて成功し、`delivery_completed` の後は Actor の GET が `410`（`Tombstone`）を返す。見直す条件に当たる食い違いは無い。ソースでも、Mastodon・Misskey・GoToSocial・Akkoma は自分自身の `Delete`（Actor）をキャッシュの Actor と鍵で処理し、アカウントを消す（2026-10-23 に確認）。

DB の作り直し、PostgreSQL の版の更新、KV やキューの入れ替えのあいだ、リクエストは 500 や `/readyz` の 503 になり、ロードバランサがインスタンスを外して自前のエラーを返し、連合先は配送の失敗を数える。そこでミドルウェアに触れず一時的に使えないと答えるモードと、`my serve` も動かせないときの静的な HTML を決めた（#1023）。

外形監視を `/readyz` に向けると、毎回 DB に触れて scale to zero の DB（Neon Free は 5 分無通信で停止）を起こし続け NFR-COST-1 を崩し、第三者も起こせる。メンテナンス中の `/healthz`・`/readyz` は 200 なので、状態コードだけの監視は正常と判定する。そこで監視向けの端点を分け、DB の検査の回数に上限を設けた。ステータスページの URL は障害時にこそ要るので、DB ではなく設定に置いた（#1024）。

## 採らなかった案

- 閉鎖の状態をインスタンス設定の enum（`service_state`）で持つ: 既存の設定の画面と CLI がそのまま使え、実装が最小。だが `closed` から戻す操作を別に禁じる必要があり、閉鎖日と期限の整合も検査しにくい
- 閉鎖の状態を設定ファイル・環境変数で持つ（Mastodon の `SELF_DESTRUCT`）: 誤操作で入らない。だが予告の日付をプロセスごとに持ち、インスタンス間でずれ、監査ログに残らず、取り消しに再デプロイが要る
- 投稿ごとの `Delete` も送る: Actor の削除で投稿を消さない実装にも効く。だが配送数が投稿数に比例し（数十倍〜数百倍）、費用と時間が合わない
- `Delete` を送らず `410` だけにする: 費用がゼロ。だが取り直しの `410` でアカウントを消すのは主要な実装のうち Mastodon だけで、Misskey・GoToSocial・Akkoma はキャッシュを持ち続ける（2026-10-23 にソースで確認）
- 配送の完了の後に消去ジョブ（`account.erase_deleted`）で全アカウントを消す: 個人データが消えたことを保証でき、破棄忘れが起きない。だがエクスポートの期間とぶつかり、期限の後なら運営者は DB ごと消せる。消去ジョブは 1 回 50 トランザクションの上限があり、保全の扱いも複雑になる
- メンテナンスモードを起動中に切り替える（シグナル・ファイルの監視・管理 API）: 再デプロイなしに即座に切り替えられ、単一の VM なら十分に動く。だが複数のインスタンスを揃える共有の置き場がなく（DB・KV は使えない）、Cloud Run・Lambda にはシグナルを送れない
- メンテナンスを前段の静的な応答だけで済ませる: サーバーのコードを変えない。だが前段が無い構成があり、`Retry-After` や JSON の答え分けが難しいので、単独では採らず補助（書き出し）とした
- 通常の運用中に DB の値をファイルへ書き出し、メンテナンスモードが読む: 名前とテーマが通常の画面と揃い、管理画面で告知文を書ける。だが Cloud Run・Lambda のファイルシステムは揮発し、共有の置き場と古さの管理が要る
- `/readyz` をそのまま外形監視に使う: 何も足さない。だが毎回 DB に触れ、「LB には受け付け可、監視にはメンテナンス中」を 1 つの応答で返せず、キャッシュするとプローブの反応が遅れる
- 部品ごとの詳しい状態をトークン付きで返す: どの部品が壊れたかまで分かる。だがトークンの管理が要り、UptimeRobot のヘッダの追加は有料で、検査がミドルウェアを起こす。同じ情報は管理画面と指標にある
- ステータスページの URL をインスタンス設定に置く: 再起動なしに変えられ、Mastodon も同じ。だがメンテナンスモードや DB の障害中に読めず、専用の設定キーを別に要する
- ステータスページの状態を取り込む: 利用者がページを開かずに Web で知れる。だがサービスごとの API の違い、外向きの取得、定期のジョブとキャッシュが増え、FR-726 のお知らせで代えられる

## Consequences（影響）

- 良い影響
  - 運営者が、利用者と連合先に告知と持ち出しの期間を渡してから、連合先のキャッシュを消して抜けられる
  - 遷移はイベントとして残り、取り消せない操作はドメインの関数・確認・権限の 3 段で守られる
  - 閉鎖のための新しい経路は fan-out のジョブと判定の関数だけで済む
  - DB・KV・キューの作業のあいだも、利用者・クライアント・連合先・監視が「メンテナンス中」と区別でき、切り替えはどのターゲットでも環境変数の変更と再デプロイだけになる
  - 監視の頻度や第三者の連打があっても、1 プロセスが DB に触れるのは最大 1 分に 1 回で、費用の上限が読める
  - サーバーが答えないときも、Web クライアントは残した値からステータスページへ案内できる
- 悪い影響・トレードオフ
  - `Delete`（Actor）の配送数は「アカウント数 × 既知の shared inbox 数」に比例する。大きなサーバーでは数百万件になり、Cloud Tasks / SQS では課金も比例する（NFR-COST-5）。見積もりを確認の画面に出すことでしか抑えない
  - 連合先が `Delete` を取りこぼした（`unavailable`・dead job）場合、そのサーバーにはキャッシュが残る
  - クローズの間の `federation_mode` の例外で、`Flow` の判定は 2 つの入力を持つ
  - 運営者が DB とバックアップの破棄を忘れると個人データが残る。サーバーからは確かめられない
  - 加入は期間の終わりで解約するだけなので、年払いのサポーターは閉鎖日の後の期間に払った分が残りうる。日割りの返金は運営者が事業者（Stripe）の画面で行い、ADR-0072 の返金の記録に乗る。サーバーは返金を計算しない
  - 日付に下限が無いので、告知の期間をごく短くすると利用者はデータを持ち出しきれない。警告と `export_until` でしか守らない
  - メンテナンスの切り替えに再起動が要り、その間の数秒〜数十秒は前段のエラーになりうる
  - メンテナンスモードではサーバーの名前とテーマが設定の値と既定のテーマになり、通常の画面と揃わないことがある。告知文は管理画面から変えられない
  - `/readyz` がメンテナンス中に 200 を返すので、監視は本文を見る必要がある
  - 終了予定の時刻はロケールに合わせて整形されない（ISO 8601 の UTC）
  - 長いメンテナンスでは連合先の再試行が尽き、その間の活動を受け取れない
  - 状態エンドポイントの状態は最大 60 秒古く、障害と復旧の検知が最大 1 分遅れる。キャッシュはプロセスごとなので、複数台では台ごとに結果が違いうる
  - 集約した状態だけなので、キューの詰まり・配送の滞留・Blob の障害は外形監視で分からない
  - `503` を「停止」とみなす監視では、メンテナンス中も停止として通知される
  - 初めて開いた Web クライアントは、サーバーが答えないときにステータスページを出せない。URL の変更に再デプロイが要る

## Open Questions / 要検証

- Akkoma が、配送中の最小の Actor の文書の下でクローズの `Delete`（Actor）を受け付けてアカウントと投稿を消し、inbox の `410` を再試行しないか。federation-live に Akkoma が無いため、実際の相互運用では確かめていない（[#786](https://github.com/mycroblog/my/issues/786) で Akkoma を用意したあとに `TestClosure` を広げる）
- 配送の速度の値（未処理の配送のジョブの上限 10,000 件、キューの同時実行数）と、短い告知の期間を警告する 30 日を、実装の後に調整する
- `maintenance_retry_after` の既定 600 秒と、`maintenance_message` の上限 2,000 文字は、実装後の運用で見直す
- Better Stack の Free の確認の間隔とメンテナンスの時間帯（公式の料金ページに Free の範囲の記載がなく、第三者の記事だけが間隔 3 分とする）
- Atlassian Statuspage で、外形監視の通知から部品の状態を自動で変える連携の方法（サービスごとの実際のアカウントで確かめる）
