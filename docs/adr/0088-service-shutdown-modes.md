# ADR-0088: サービスの終了の予告モードとクローズモード

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1034

## Context（背景・課題）

サーバーを畳むとき、利用者と連合先に迷惑をかけない終わり方が要る。いまあるのは個々のアカウントの削除（FR-008。`Delete`（Actor）を承認済みのフォロワーと既知の shared inbox へ送る、[data-model §8](../architecture/data-model/08-deletion-and-purge.md#8-削除と消去)）とお知らせ（FR-726）だけで、サーバー全体を計画的に閉じる仕組みが無い。DB を消して VM を止めるだけだと、

- 利用者はデータ（FR-008 のエクスポートと CSV）を持ち出せず、移転（`Move`、[ADR-0089](0089-account-migration-move.md)）もできない
- 連合先には Actor と投稿のキャッシュが残り、連合先からこちらへの配送の再試行が続く（相手が到達できないと見なすまで。こちらの基準は 7 日の失敗で、[federation.md §7.3](../architecture/federation.md#73-配送とリトライ) は Mastodon の `DeliveryFailureTracker` と同じ基準としている。Misskey も 7 日の不通で配送を止める（2026-10-23 に [`DeliverProcessorService`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/queue/processors/DeliverProcessorService.ts) で確認））

v0 のリリース（M5）で運営者に渡す以上、始め方と同じく終わり方も用意しておく。関連する要件は FR-001（登録モード）、FR-008（削除とエクスポート）、FR-726（お知らせ）、FR-911（サポーターの加入と解約）、NFR-COST-5（配送の集約）、NFR-MOD-7（取り消せない操作の確認）。関連する決定は ADR-0018（URI）、ADR-0020（データモデル）、ADR-0036（凍結の Actor の文書）、ADR-0038（保持期間）、ADR-0072（サポーター）、ADR-0076（連合の向き）。

参考（確認日 2026-10-23、[tootctl の文書](https://docs.joinmastodon.org/admin/tootctl/#self-destruct)）: Mastodon の `tootctl self-destruct` は 4.3.0 からセルフデストラクトのモードへの切り替えを案内し、そのモードが全アカウントの削除の通知を配送する。モード中、利用者はサインインしてデータをダウンロードできるが、ほかの操作はできない。ローカルのデータは消さない（DB や VPS を消すほうが速いため）。操作は取り消せず、キューが空になるまでワーカーを止めてはいけない。モードには環境変数 `SELF_DESTRUCT`（`LOCAL_DOMAIN` を署名した値）で入り、定期ジョブが各ローカルアカウントの `Delete`（Actor）を既知の inbox すべてへ送る（投稿ごとの `Delete` は送らない）。キューの積み残しが 10,000 件か Redis のメモリの半分を超えると、その回は積まない（2026-10-23 に [`SelfDestructScheduler`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/workers/scheduler/self_destruct_scheduler.rb) と [`SelfDestructHelper`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/helpers/self_destruct_helper.rb) で確認）。

## Decision Drivers（判断基準）

1. **取り消せない操作を事故で起こさない**: クローズは連合先のキャッシュを消す。戻せないので、遷移は一方向にし、確認を重ね、記録を残す
2. **利用者がデータと関係を持ち出せる**: 予告の期間とクローズの間、エクスポートと移転ができる
3. **連合先をきれいに抜ける**: 連合先が Actor と投稿を消せるだけの `Delete` を送り、署名を検証させ、そのあと再試行させない
4. **既存の仕組みに乗る**: アカウントの削除の配送、連合の向き（ADR-0076 の `Flow`）、お知らせ、メンテナンス HTML（[ADR-0084](0084-maintenance-mode.md)）を使い、閉鎖のためだけの経路を増やさない
5. **費用**: 配送の回数を見積もれ、集約と除外で抑えられる（NFR-COST-5）

## Considered Options（選択肢）

論点ごとに比べる。

### A. 状態の持ち方

- A1. `instance` モジュールのイベントと投影（ADR-0020 規約 1・2）
- A2. インスタンス設定の enum（`service_state`）と日付の設定（ADR-0076 の `federation_mode` と同じ形）
- A3. 設定ファイル・環境変数（Mastodon の `SELF_DESTRUCT` のように、起動時の値でモードに入る）

### B. 連合先に送るもの

- B1. 各ローカルアカウントの `Delete`（Actor）だけ（アカウントの削除と同じ宛先）
- B2. B1 に加えて、投稿ごとの `Delete`
- B3. 送らない（Actor・投稿の GET を `410` にするだけで、連合先が取り直したときに消えるのを待つ）

### C. ローカルのデータ

- C1. 消さない。運営者が持ち出しの期限の後に DB とバックアップを破棄する（Mastodon と同じ）
- C2. 配送の完了の後、アカウントの消去ジョブ（`account.erase_deleted`）で全アカウントを消す

## Decision（決定）

**A1・B1・C1 を採用する。** 理由は Pros and Cons を参照。

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
- 状態はリクエストとジョブごとに読む（`federation_mode` と同じく、変更はすぐ効く）。各入口は `instance.Closure` の値を受けて、`closure.Allows(state, 操作)` のようなドメインの 1 つの関数で判定する（ADR-0076 の `federation.Allows` と並べる）。
- 操作: 管理 API・管理画面・CLI（`my admin closure announce|withdraw|close|status`）。権限は新しい `manage_service_closure`（区分 `server`、組み込みのロールでは `admin` だけ）。予告と取り消しは管理画面からもできるが、`close` は管理画面では alertdialog（NFR-MOD-7）でドメイン名の入力を求め、CLI では 2 度の確認（`y` とドメイン名の入力。自動化には `--confirm-domain=<domain>`）を求める。確認の画面は、送る `Delete` の数の見積もり（ローカルアカウント数 × 宛先の inbox 数）と、続いているサポーターの加入の数を示す。
- すべての遷移を監査ログ（ADR-0037、`target_kind = instance`）に残す。

### 2. 各状態の振る舞い

| | 予告 | クローズ（配送中） | 配送の完了 | 停止（静的なページ） |
| --- | --- | --- | --- | --- |
| **Web** | 全画面に閉じられない帯（閉鎖日・持ち出しの期限・エクスポートと移転への導線）。ほかはいつもどおり | サインイン後は閉鎖の画面だけ（エクスポートの要求とダウンロード、CSV、サインアウト）。ほかの画面は理由を示す | 同左 | 静的な閉鎖のページ |
| **サインイン・登録** | サインインできる。登録は実効値を `closed` に固定（設定の値は変えない）、招待は使えない、審査待ちの申請は予告の開始で却下（理由: サービスの終了） | サインインとパスワードの再設定だけ。登録・招待はできない | 同左 | できない |
| **client API** | いつもどおり。`GET /instance` に `closure`（状態・`closes_at`・`export_until`・メッセージ） | 自分のアカウントの読み取り・エクスポート（FR-008 と CSV）・セッションの操作・`GET /instance` だけ。ほかは `403`（problem type `service-closed`）。ストリームと Web Push は止める | 同左 | なし |
| **通知・メール** | 予告・日付の変更・取り消しのとき、アプリ内の通知（新しい種類 `service_closure`）と、メールの確認が済んだ全利用者へのメール（[ADR-0086](0086-email-delivery.md) の取引のメール。見落とすとデータを失うので配信停止と苦情による抑止の対象にしない）。閉鎖日の 7 日前と前日にもメール | クローズのときにメール（持ち出しの期限）。ほかは認証のメールだけ | 持ち出しの期限の 7 日前にメール | なし |
| **サポーター（ADR-0072）** | 新しい加入は `409`。予告の開始で続いている加入すべてに `CancelAtPeriodEnd`（ADR-0072 §9 の廃止と同じ）を積み、通知する | 残っている加入を `CancelNow`（理由 `service_closed`）。課金は無効にできる状態になる | 同左 | — |
| **連合: 送るもの** | いつもどおり | `retract`（下の `Delete`）だけ。積まれていたほかの配送は実行時に `skipped`（ADR-0076 と同じ） | なし | なし |
| **連合: inbox** | いつもどおり | 署名の検証より前に `410 Gone` | 同左 | `410`（静的） |
| **連合: ローカルの Actor の GET** | いつもどおり | 凍結と同じ最小の文書（ADR-0036。空のプロフィール、`suspended: true`、鍵あり）。`Delete` の署名を検証させるため | `410 Gone`（`Tombstone`） | `410`（静的） |
| **連合: 投稿・コレクションの GET** | いつもどおり | `410 Gone`（`Tombstone`） | 同左 | `410`（静的） |
| **インスタンス Actor・WebFinger・NodeInfo** | いつもどおり。NodeInfo の `metadata.serviceClosure` に状態と日付 | インスタンス Actor は返す（署名つきの取得のため）。WebFinger は `410`。NodeInfo は状態を出す | インスタンス Actor も `410`。NodeInfo は状態を出す | `410`（静的） |
| **外の取得・取り込み・リレー** | いつもどおり | しない（リレーは脱退を送らず止める） | しない | — |

- inbox の `410` は、主要な実装がどれも再試行しない失敗として扱うので、連合先からの配送が早く止まる: Mastodon は 401・408・429 以外の 4xx（[`response_error_unsalvageable?`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/helpers/json_ld_helper.rb)）、Misskey は 429 以外の 4xx（[`StatusError`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/misc/status-error.ts)）、GoToSocial は 429 以外の 4xx（[`httpclient`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/httpclient/client.go)）、Akkoma は 405 と 410（[`PublisherWorker`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/workers/publisher_worker.ex)）を再試行しない（2026-10-23 にソースで確認）。Misskey は shared inbox の `410` で、そのサーバー全体への配送を止める（`goneSuspended`、同じく [`DeliverProcessorService`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/queue/processors/DeliverProcessorService.ts)）。
- 配送中に Actor の文書と鍵を返し続けるのは、鍵を取り直す受信側のためである。GoToSocial は、署名の鍵の取得が `410` だと tombstone を記録して inbox に `202` を返し、その `Delete` を処理しない（2026-10-23 に [`authenticate.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/authenticate.go) と [`federatingprotocol.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/federatingprotocol.go) で確認）。
- クローズの判定は `federation_mode`（ADR-0076）より優先する: クローズでは `retract` と、鍵を出すための `identify` だけを通す。`federation_mode` が `off` / `inbound` でも `Delete` は送る（予告の期間に連合を狭めた運営者も、外に残ったものを消せるように）。これは ADR-0076 の「`off` は何も通さない」に対する例外で、ADR-0076 自体は変えない（クローズは別の軸の状態として足す）。管理画面の確認でこの例外を示す。ドメインの処分（§11）はいつもどおり効き、凍結したドメインには送らない。
- メンテナンスモード（ADR-0084）はどの状態でも重ねて使える（すべてを止める）。ステータスページ（[ADR-0085](0085-uptime-monitoring-status-page.md)）への掲出は運用文書の手順に書く。

### 3. クローズの配送

- `closed` を記録したトランザクションで、ジョブ `instance.closure_fanout`（カーソルはアカウントの ID）を outbox に積む。ジョブはローカルのアカウント（インスタンス Actor・削除済みを除く。凍結中は含む）を 100 件ずつ、アカウントごとに `Delete`（Actor）の outbound activity を記録し、既存の展開（[federation.md §7.2](../architecture/federation.md#72-展開fan-out)。承認済みのフォロワー → 既知の shared inbox）に渡す。ライフサイクル（`account_lifecycle_events`）は変えない（消去ジョブを動かさない。C1）。
- 投稿ごとの `Delete` は送らない（B1。アカウントの削除と同じく `Delete`（Actor）が代わりを務める）。
- 速度: 次のページの展開は、前のページの展開が終わってから積む。同時に動く配送はキューの同時実行数の設定で抑え、未処理の配送のジョブが上限（既定 10,000 件、Mastodon と同じ値。要調整）を超えている間は次のページを積まない。shared inbox への集約と `unavailable` のドメインの除外は既存のまま効く。
- 再試行と諦める時点: 既存の配送の方針（最大 9 回、約 4 日、使い切ったら dead job）。dead job は自動では再実行しない（運営者は既存の管理 API で再実行できる）。
- 完了: 定期ジョブが、クローズの `Delete` の配送がすべて終わった状態（`succeeded` / `permanent_failure` / `skipped` / dead job）で、展開も残っていないことを確かめたら `delivery_completed` を記録する。
- 進捗: `GET /api/v1/admin/closure`（と `my admin closure status`）が、処理したアカウント数と、配送の状態ごとの件数を返す。管理画面はそれを示し、ワーカーを止めないよう注意を出す。

### 4. 移転済みのアカウント（ADR-0089）

- 予告の帯と通知・メールは、エクスポートと並べて移転の設定の画面へ案内する。設定の画面は、予告中だけ「サーバーの終了の前に移転できます」と閉鎖日を示す。移転には移転先での別名（`alsoKnownAs`）の設定が要るので、手順の説明へのリンクを置く。
- 予告の期間中も `Move` の送信はいつもどおりできる。クローズの後は、client API がエクスポート以外の操作を断るので、`Move` を始められない（ADR-0089 は `Move` を `retract` に分類するが、クローズで通す `retract` は、このクローズの `Delete` と、クローズの前に積まれた配送だけである）。
- 移転済みのアカウント（`Move` を送ったもの）にも、クローズで `Delete`（Actor）を送る。フォロワーは移転で移っているので、`Delete` で失うものは連合先のキャッシュの「移転しました」の表示だけで、ドメインが止まったあとは取り直しで消える。全アカウントを同じ扱いにして、例外の経路を作らない。ただし順序は最後のフェーズにまとめ、配送中の間は移転済みの Actor の GET に `movedTo` を残す（移転を見に来た相手が辿れるように）。

### 5. ローカルのデータ（C1）

- サーバーは自分ではデータを消さない。配送の完了の後も、`export_until` まではクローズのままエクスポートを提供する。`export_until` を過ぎたら、運営者が静的なページへ切り替え、DB・Blob・バックアップを破棄する（運用文書に書く）。
- ADR-0038 の保持期間は、サーバーが動いている間はいつもどおり効く。`retention_holds`（保全）がある場合は、`close` の確認と運用文書で、破棄の前に対象の記録を CLI で書き出して保管するよう促す（保全の義務はサーバーを閉じても消えないため）。

### 6. 停止の後

- `my admin closure export-html` が、ADR-0084 のメンテナンス HTML の書き出しと同じ仕組みで、閉鎖のページ（サーバー名・閉鎖日・運営者のメッセージ・ロケールごと）と、リバースプロキシ・CDN の設定例（`/ap/*`・`/.well-known/webfinger`・`/@*` は `410`、それ以外は閉鎖のページ）を書き出す。
- 運用文書は、ドメインを手放さず、およそ 1 年は `410` を返し続けるよう勧める。URI は UUID（ADR-0018）なので同じ `id` の Actor が作られる心配は小さいが、ドメインを別人が取るとハンドル（`acct:alice@example.org`）と表示用 URL が別人のものになり、なりすましに使える。

### 7. 機能要件の追加

番号は「運用」の節の次の空き番号（FR-808・FR-809）を使う。この ADR の PR で `docs/requirements/01-functional.md` に足す。

| ID | 要件 | 優先度 |
| --- | --- | --- |
| FR-808 | サービスの終了の予告: `manage_service_closure` を持つ人が、閉鎖日とデータの持ち出しの期限を決めて終了を予告し、変更し、取り消す。予告の間は登録・招待・サポーターの新しい加入を止め、続いている加入を期間の終わりで解約し、全画面の帯・`GET /instance`・NodeInfo・通知・メールで知らせ、エクスポートと移転（`Move`）へ案内する。操作は監査ログに残す | S |
| FR-809 | サービスのクローズ: 閉鎖日の後、取り消せない操作として（管理画面の確認・CLI の 2 度の確認）クローズに切り替え、全ローカルアカウントの `Delete`（Actor）を連合先へ配送し、進捗を管理画面と CLI で示す。クローズの間、利用者はサインインしてデータのエクスポートだけができ、ほかの操作は client API が断る。連合の GET と inbox は `410 Gone` を返し、配送の完了後は静的な閉鎖のページを書き出せる | S |

## Consequences（影響）

- 良い影響
  - 運営者が、利用者と連合先に告知と持ち出しの期間を渡してから、連合先のキャッシュを消して抜けられる。
  - 遷移はイベントとして残り、取り消せない操作はドメインの関数・確認・権限の 3 段で守られる。
  - 配送・展開・リトライ・`Flow` の判定は既存のものを使い、閉鎖のための新しい経路は fan-out のジョブと判定の関数だけで済む。
- 悪い影響・トレードオフ
  - `Delete`（Actor）の配送数は「アカウント数 × 既知の shared inbox 数」に比例する。大きなサーバーでは数百万件になり、Cloud Tasks / SQS では課金も比例する（NFR-COST-5）。見積もりを確認の画面に出すことでしか抑えない。
  - 連合先が `Delete` を取りこぼした（`unavailable`・dead job）場合、そのサーバーにはキャッシュが残る。取り直しの `410` で消えるのを待つしかない。
  - クローズの間の `federation_mode` の例外は、ADR-0076 の判定に「クローズ」という別の軸を足すので、`Flow` の判定は 2 つの入力を持つ。
  - C1 なので、運営者が DB とバックアップの破棄を忘れると個人データが残る。サーバーからは確かめられない。
  - 加入は期間の終わりで解約するだけなので、年払いのサポーターは閉鎖日の後の期間に払った分が残りうる。日割りの返金は運営者が事業者（Stripe）の画面で行い、ADR-0072 の返金の記録に乗る。サーバーは返金を計算しない。
  - 日付に下限が無いので、運営者が告知の期間をごく短くすると、利用者はデータを持ち出しきれない。警告と `export_until`（閉鎖日の後もエクスポートできる期間）でしか守らない。
- 後続で必要になる作業（実装の issue はこの単位で分ける）
  - サーバー: 予告モード（`instance` のイベントと投影、遷移の検査、権限、登録・招待・審査待ちの申請、サポーターの解約、`GET /instance`・NodeInfo、通知の種類、監査ログ、CLI）
  - サーバー: クローズモード（`closure.Allows` と各入口、`instance.closure_fanout`、完了の判定、Actor・投稿・WebFinger・inbox の `410`、進捗の管理 API と CLI）
  - Web: 帯・閉鎖の画面・管理画面（予告・クローズの確認と進捗）、移転の設定の画面の案内、`locales/en`・`locales/ja`
  - メール（ADR-0086 のジョブとテンプレートの上に）: 予告・変更・取り消し・閉鎖日前・クローズ・期限前のテンプレート
  - 運用文書 `docs/operations/service-closure.md`: 手順（予告 → 閉鎖日 → クローズ → 完了の確認 → 期限 → 静的なページ → 破棄）、ワーカーを止めない注意、保全の記録の書き出し、ドメインの扱い、ステータスページの掲出。閉鎖のページの書き出し（ADR-0084 の書き出しの仕組みを使う）
  - 相互運用テスト（federation-live）: 下の Open Questions の項目
  - 文書の更新: federation.md（§10・§14 と新しい節「サービスの終了」）、data-model（`instance` の表と §8）、`docs/operations/instance-settings.md`（登録の実効値）、client-api の problem type

## Pros and Cons of the Options（詳細比較・任意）

### A. 状態の持ち方: A1 イベントと投影（採用）/ A2 インスタンス設定 / A3 設定ファイル

- A1: 遷移の規則（戻れない、閉鎖日より前にクローズできない）をドメインで検査でき、日付と操作者が状態と一緒に残る（判断基準 1）。ADR-0020 規約 2 の「状態を軸ごとのイベントで持つ」に合う。表とジョブが増える。
- A2: 既存の設定の画面・CLI・`instance_setting_revisions` がそのまま使え、実装が最も小さい。ただし設定は任意の値へ自由に戻せるもので、`closed` から `normal` に戻す操作を設定の仕組みの外で別に禁じる必要があり、日付の組（閉鎖日と期限）の整合も設定の単位では検査しにくい。
- A3: 起動時に決まるので誤操作で入らない（Mastodon が取る方向）。ただし利用者への予告（帯・API）に要る日付をプロセスごとに持つことになり、複数のプロセス・Cloud Run のインスタンスで値がずれうる。監査ログに残らず、予告の取り消しも再デプロイを要する。

### B. 連合先に送るもの: B1 `Delete`（Actor）（採用）/ B2 投稿ごとにも / B3 送らない

- B1: アカウントの削除と同じ宛先と同じ形で、Mastodon の self-destruct と同じ。主要な実装は、主体が自分自身を消す `Delete`（Actor）でアカウントを消す（Mastodon は [`DeleteAccountService`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/lib/activitypub/activity/delete.rb) で投稿ごと、Misskey は [`deleteActor`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/activitypub/ApInboxService.ts) が積むアカウントの削除のジョブでノートごと、GoToSocial は [`deleteAccount`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/federatingdb/delete.go)、Akkoma は [`User.delete`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/web/activity_pub/side_effects.ex)。2026-10-23 にソースで確認）。配送数はアカウント数に比例する。
- B2: Actor の削除で投稿を消さない実装にも投稿が消える。ただし配送数が投稿数に比例し（数十倍〜数百倍）、費用と時間が判断基準 5 に合わない。B1 で消えない実装が見つかれば、その実装のドメインに限って投稿の `Delete` を足す余地はある。
- B3: 費用はゼロで実装も最小。ただし取り直しの `410` でアカウントを消すのは主要な実装のうち Mastodon だけで、Misskey・GoToSocial・Akkoma はキャッシュを持ち続ける（下の確認）。判断基準 3 に合わない。

受信側の振る舞い（2026-10-23 にソースで確認）:

- 自分自身の `Delete`（Actor）は、4 実装ともキャッシュの Actor と鍵で処理し、Actor を取り直さない（Akkoma は [`DeleteValidator`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/web/activity_pub/object_validators/delete_validator.ex) がキャッシュの利用者を引く）。
- 取り直しの `410`: Mastodon は WebFinger の `410` で [`GoneError`](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/lib/webfinger.rb) にし、Actor の取得の `410` で [アカウントの削除を積む](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/services/activitypub/fetch_remote_actor_service.rb)。Misskey は WebFinger の失敗をエラーにするだけで消さない（[`RemoteUserResolveService`](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/RemoteUserResolveService.ts)）。GoToSocial は WebFinger の `410` をエラーにし（[`finger.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/transport/finger.go)）、Actor の取り直しの `410` でアカウントを消す処理を持たない（[`dereferencing/account.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/federation/dereferencing/account.go)）。Akkoma は取り直しが `404` / `410` でもキャッシュの利用者を返し続ける（[`User.get_or_fetch_by_ap_id`](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/user.ex)）。
- 実際の相互運用（federation-live の `TestClosure`、2026-10-25、Mastodon 4.7・GoToSocial 0.22・Misskey 2026.10）: 配送中の最小の Actor の文書（`suspended: true`、鍵あり）の下で、3 実装ともクローズの `Delete`（Actor）を受け付け、アカウントと取り込んだ投稿を消した。クローズの直後に相手が送った配送は inbox の `410` を受けて再試行されず、`Delete` の処理の後はこちらへの配送が無い。Misskey は `410` を受けてこのサーバーを `goneSuspended` にした。配送はすべて成功し、`delivery_completed` が記録された後、Actor の GET は `410`（`Tombstone`）を返す。上の「この決定を見直す条件」に当たる食い違いは無い。
- Misskey・GoToSocial・Akkoma のソースには、サーバー全体の閉鎖の仕組み（Mastodon の self-destruct に当たるもの）が無い（`self-destruct` などで検索して見つからない）。

### C. ローカルのデータ: C1 消さない（採用）/ C2 消去ジョブで消す

- C1: 持ち出しの期限までエクスポートに要るデータが残る。破棄は DB と Blob の削除のほうが速く確実（Mastodon と同じ理由）。破棄の確認は運営者に任される。
- C2: サーバーの側で個人データが消えたことを保証でき、バックアップ以外の破棄忘れが起きない。ただしエクスポートの期間とぶつかる（消すと持ち出せない）ので期限の後に回すことになり、そのときには運営者は DB ごと消せる。消去ジョブは件数に上限を持つ（1 回 50 トランザクション）ので大きなサーバーでは時間がかかり、保全（ADR-0038）の扱いも複雑になる。

### この決定を見直す条件

- 主要な実装のどれかが、凍結と同じ最小の Actor の文書を返す配送中の `Delete`（Actor）を受け付けない（相手が取り直して Actor が残っていると判断する）ことが相互運用テストで分かった場合: 配送中から Actor の GET を `410` にし、鍵は既知の鍵での検証に任せる。
- `Delete`（Actor）で投稿を消さない主要な実装が見つかった場合: そのドメインに限って B2 を足す。
- 配送数の見積もりが運営者の想定する費用を超える事例が出た場合: 既知の shared inbox への送信を「フォロワーかフォローのいるドメイン」に絞る段を足す。

## Open Questions / 要検証

- Akkoma が、配送中の最小の Actor の文書の下でクローズの `Delete`（Actor）を受け付けてアカウントと投稿を消し、inbox の `410` を再試行しないか。federation-live に Akkoma が無いため、実際の相互運用では確かめていない（ソースの読みは上のとおり。[#786](https://github.com/mycroblog/my/issues/786) で Akkoma を用意したあとに `TestClosure` を広げる）。
- 配送の速度の値（未処理の配送のジョブの上限 10,000 件、キューの同時実行数）と、短い告知の期間を警告する 30 日を、実装の後に調整する。
