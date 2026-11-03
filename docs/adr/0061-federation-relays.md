# ADR-0061: 連合リレーへの参加と脱退

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #651

## Context（背景・課題）

決定者は「管理者によって設定される連合リレーへの参加・脱退」を求めている。リレーは、参加したサーバーから受け取った公開投稿を、ほかの参加サーバーへ中継する Actor である。
小さなインスタンスでは、フォローの少なさから連合タイムライン（FR-202）が空に近く、自分の公開投稿も外に届きにくい。リレーはこの両方を補う。

いまの決定と要件は、リレーを持たない前提で書かれている。

- [ADR-0006](0006-federation-protocol-scope.md) の対応範囲の表は、「非対応」に「Linked Data Signatures による署名生成（受信時の検証は互換のため検討）、リレー」を挙げる。
- [ADR-0034](0034-remote-content-scope-and-retention.md) は、ローカルのアカウントとの関係がある `Create` だけを取り込み、見直す条件に「リレーは v0 の非目標」と書く。
- [ADR-0033](0033-inbox-processing-model.md) と ADR-0019 §7 は、署名者と `actor` が一致しない（転送・リレーの）アクティビティを `trust = refetch` にして、オブジェクトをオリジンから取り直す。
- [00-vision.md](../requirements/00-vision.md) の非目標に「リレー」がある。
- 関連する要件: FR-202（連合タイムライン）、FR-405（受信アクティビティ）、FR-406（配送）、FR-702（ドメイン単位の処分）、FR-704（監査ログ）、FR-803（期限切れデータの消去）、NFR-COST-1・NFR-COST-5。
- 関連する決定: [ADR-0004](0004-job-queue-abstraction.md)（ジョブ）、[ADR-0017](0017-visibility-local.md)（`local` は連合しない）、[ADR-0020](0020-data-modeling-principles.md)（データモデル）、[ADR-0036](0036-moderation-federation-propagation.md)（処分の伝播）、[ADR-0037](0037-audit-log.md)（監査ログ）。
  管理の権限は、ADR-0058 モデレーションとサーバー管理の分離（#648）のサーバー管理の側に置く。

### 主要実装とリレーの方式（2026-10-20 にソースで確認）

リレーには 2 つの方式がある。

| | Mastodon 方式 | LitePub 方式 |
| --- | --- | --- |
| 参加 | 運営者がリレーの **inbox の URL** を登録し、インスタンス Actor が `Follow`（`object` は `https://www.w3.org/ns/activitystreams#Public`）を送る。`Accept` で有効、`Reject` で拒否 | 運営者がリレーの **Actor の URI** を登録し、サーバーのリレー用 Actor が、その Actor を `object` にした `Follow` を送る |
| 送信 | 公開投稿の `Create` / `Update` / `Delete` を、作者の HTTP 署名と **Linked Data Signatures（`RsaSignature2017`）** を付けてリレーの inbox に送る | リレー用 Actor が公開投稿を `Announce` し、そのフォロワー（リレー）に届ける |
| 受信 | リレーが原本をそのまま転送する（署名者はリレー、`actor` は作者） | リレーが `Announce` を送る（主体はリレーの Actor） |

- Mastodon: `Relay` のモデルは状態 `idle` / `pending` / `accepted` / `rejected` を持ち、代表のアカウント（インスタンス Actor）から `Public` への `Follow` を送り、脱退で `Undo(Follow)` を送る（[`relay.rb`](https://github.com/mastodon/mastodon/blob/main/app/models/relay.rb)）。
  リレーに送るのは `public` の投稿だけ（[`status_reach_finder.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/status_reach_finder.rb) の `relay_inboxes`）。
  受信では、署名者と `actor` が違うアクティビティは **LD 署名か FEP-8b32 の証明（`eddsa-jcs-2022`）を検証できなければ捨てる**（[`process_activity_service.rb`](https://github.com/mastodon/mastodon/blob/main/app/services/activitypub/process_activity_service.rb)、[`object_integrity_proof.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/object_integrity_proof.rb)）。
  有効なリレーを経由した `public` / `unlisted` の `Create` は関係が無くても取り込み（[`activity/create.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/create.rb) の `requested_through_relay?`）、リレーの `Announce` は元の投稿を取得するがブーストとしては記録しない（[`activity/announce.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/announce.rb)）。
- Misskey: Mastodon 方式。システムのユーザー `relay.actor` から `Public` への `Follow` を送り、状態は `requesting` / `accepted` / `rejected`（[`RelayService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/RelayService.ts)、[`ApRendererService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApRendererService.ts) の `renderFollowRelay`）。
  `public` のノートだけを、LD 署名を付けてリレーに送る（[`NoteCreateService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/NoteCreateService.ts)）。
  受信では、HTTP 署名が `actor` と合わなければ `RsaSignature2017` の LD 署名を検証し、無ければ捨てる（[`InboxProcessorService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/queue/processors/InboxProcessorService.ts)）。
- Pleroma / Akkoma: LitePub 方式。リレー用 Actor `/relay` が対象の Actor を `Follow` し、公開の `Create` をリレー用 Actor が `Announce` する（[Akkoma `relay.ex`](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/relay.ex)）。
  署名者と `actor` が合わないアクティビティはオブジェクトを取り直す（要検証）。
- GoToSocial: リレーに対応していない（要検証）。
- YUKIMOCHI Activity-Relay（Go、AGPL-3.0、v2.0.12 は 2026-10-16）: 両方式を受け付ける。`Public` への `Follow` は購読者（Mastodon 方式）、Actor の型が `Application` か ID の末尾が `/relay` の Actor からの `Follow` はフォロワー（LitePub 方式）として登録し、フォロワーにはフォローを返す。
  購読者かフォロワーのドメインから届いた `Public` 宛ての `Create` / `Update` / `Delete` を、購読者には **原本のまま**、フォロワーには `Announce` で中継する（[`api/handle.go`](https://github.com/yukimochi/Activity-Relay/blob/master/api/handle.go)、[`api/resolver.go`](https://github.com/yukimochi/Activity-Relay/blob/master/api/resolver.go)）。
  aode-relay・ActivityRelay（Python）も両方式を受け付ける（要検証）。

ここから、次のことが分かる。

1. どちらの方式で参加しても、こちらからは公開投稿の `Create` をリレーの inbox に送れば中継される（Activity-Relay は送信者が購読者かフォロワーかで区別しない）。
2. 原本のまま中継された投稿を Mastodon と Misskey の購読者が受け取るには、**LD 署名（`RsaSignature2017`）が要る**。LD 署名の無い投稿は、Mastodon（最新版は FEP-8b32 の証明でもよい）と Misskey の購読者に捨てられ、`Announce` を受けるフォロワーと、取り直しをする実装にだけ届く。
3. 受け取る側では、こちらの ADR-0033 の取り直しで、どちらの方式の中継も検証できる。

## Decision Drivers（判断基準）

1. 費用が運営者の選んだリレーの数と流量で決まり、上限を持てること。リレーに参加していないインスタンスの費用は変わらないこと（G1、NFR-COST-1, NFR-COST-5）
2. 公開範囲を守ること。`public` 以外の投稿をリレーに出さない（ADR-0017、visibility の単一のポリシー）
3. 主要なリレーのソフトウェア（Activity-Relay など）と、その先の Mastodon・Misskey・Pleroma（Akkoma）の購読者に、実際に届くこと（G4）
4. 中継された投稿を、オリジンの検証なしに信用しないこと（ADR-0019 §7、ADR-0033）
5. ドメイン単位の処分（FR-702、ADR-0036）がリレー経由の投稿にも効くこと
6. 既存の取り込み・保持（ADR-0034）と配送（federation.md §7）の仕組みに、小さな追加で載ること

## Considered Options（選択肢）

方式（参加の仕方）:

1. **Mastodon 方式と LitePub 方式の両方に参加でき、送信はどちらも `Create` の原本を LD 署名付きでリレーの inbox に送る**
2. Mastodon 方式だけに参加する
3. LitePub 方式だけに参加し、リレー用 Actor が `Announce` で送る

送信の署名:

- A. **公開投稿の `Create` / `Update` / `Delete` に LD 署名（`RsaSignature2017`）を付ける**
- B. 署名を足さない（HTTP 署名だけ）
- C. FEP-8b32 の証明（`eddsa-jcs-2022`）を付ける

受信の検証:

- X. **ADR-0033 のとおりオリジンから取り直す**
- Y. LD 署名を検証できればそのまま使い、できなければ取り直す

## Decision（決定）

**方式 1 + 送信 A + 受信 X を採用する。** 運営者が管理 API・CLI・管理画面からリレーを登録して参加・脱退でき、`public` の投稿だけを送り、受け取った投稿は取り直して検証し、短い保持期間で消す。

### 参加と脱退

- 運営者はリレーを **方式（`mastodon` / `litepub`）と URL** で登録する。`mastodon` は inbox の URL、`litepub` は Actor の URI。URL は `https` だけで、`HTTPClient`（SSRF ガード）を通す。
  `litepub` は登録時に Actor を取得して inbox（shared inbox があればそれ）を得る。管理画面は、URL を取得して Actor の文書が返れば `litepub`、そうでなければ `mastodon` を既定に選ぶ。
- 参加は **インスタンス Actor**（`Application`）が送る。`mastodon` は `Follow`（`object` は `as:Public`）、`litepub` は `Follow`（`object` はリレーの Actor）。`Follow` の `id` は `/ap/activities/{id}` で、状態は `pending` になる。
  インスタンス Actor は `Application` なので、Activity-Relay はこれを LitePub 方式のフォロワーとして受け付ける。
- `Accept` / `Reject` は、`object`（の `id`）がそのリレーに送った `Follow` の `id` と一致するものだけを受け付け、`accepted` / `rejected` にする。`accepted` で、`Accept` の主体をリレーの Actor として記録する（以後の受信で、署名者がリレーかを判定する）。
- 脱退は `Undo(Follow)` を送り、状態を `left` にする。`rejected` と `left` のリレーは、同じ登録から参加し直せる（新しい `Follow` の `id`）。登録の削除は `left` か `rejected` のときだけで、削除してもすでに送った投稿の `Update` / `Delete` は届け続ける（宛先は元の `Create` の `deliveries`）。
- インスタンス Actor への `Follow` は、`accepted` のリレーの Actor からのものだけを自動で `Accept` する（LitePub 方式のリレーが返すフォロー）。記録はせず、それ以外の `Follow` は `Reject` する。
- 登録ごとに **受信するか**（`receive`: `true` / `false`）を選ぶ。`false` は送るだけの参加で、届いた中継を取得せずに捨てる。変えるには脱退して登録し直す。
- ドメイン単位の `suspend` のあるホストのリレーは登録できない。参加中のリレーのホストを `suspend` すると、受信は署名の検証より前に `403`、配送は `skipped` になる（federation.md §11 のまま）。状態は変えず、管理画面に警告を出す。

### 送信

- 送るのは **ローカルの `public` の投稿の `Create` / `Update` / `Delete` だけ**。`unlisted`・`private`・`direct` と、**`local` は送らない**（ADR-0017）。ブーストの `Announce` も送らない（Activity-Relay の Mastodon 方式の経路は `Announce` を扱わない）。
  判定は配送の展開（federation.md §7.2）の公開範囲の規則に足し、visibility の単一のポリシー関数を通す。
- 利用者ごとに「自分の投稿をリレーに送らない」設定は持たない（Mastodon・Misskey と同じ）。リレーに出したくない投稿は、`unlisted` か `local` で投稿する。
- 展開ジョブの `Create` のフェーズに「`accepted` のリレーの inbox」を最後に足す。`deliveries` に記録するので、`Update` / `Delete` は元の `Create` の `deliveries` に届く。
  サイレンスなどで公開範囲が `public` でなくなった投稿は、ほかの宛先と同じく対象にならない。
- **`public` の投稿の `Create` / `Update` / `Delete` の文書に LD 署名（`RsaSignature2017`、作者の鍵）を付ける**。リレーに送る文書だけでなく、同じ文書をすべての宛先に送る（文書は展開ジョブが 1 回だけ組み立てる）。
  正規化（URDNA2015）は `github.com/piprate/json-gold`（Apache-2.0、cgo 不要、v0.9.1 は 2026-10-02）で行い、`@context` は同梱した文書（ActivityStreams・`w3id.org/security/v1`・`w3id.org/identity/v1`）だけで解決する（リモートの文脈を取得しない）。
  LD 署名の生成は送信のこの用途に限る。
- 配送の量は「リレーの数 × 公開投稿の数」だけ増える（リレーへの配送は shared inbox と同じく 1 件）。

### 受信

- 同期の部分（federation.md §3.1）は変えない。リレーの IP とドメインにも、ほかのサーバーと同じレート制限（1,000 / 5 分、2,000 / 5 分）を掛ける。超えた分は `429` で、リレーの再送に任せる。
- 署名者の Actor が `accepted` のリレーの Actor（`mastodon` は Actor の inbox か shared inbox が登録の inbox の URL と一致するもの）なら、ジョブに `relay_id` を付ける。中継は `trust = refetch` のまま扱う（ADR-0033 は変えない）。
- 非同期の部分では、`relay_id` の付いたアクティビティを次の順に扱う。
  1. 登録の `receive` が `false`、またはリレーが `accepted` でなければ捨てる。
  2. リレーごとの 1 時間の取り込みの上限（設定 `relay_max_activities_per_hour`、既定 1,000 件。KV で数える）を超えたら、取得せずに捨てる。捨てた件数はメトリクスに出す。
  3. 作者（`Create` の `actor`、`Announce` の `object` の作者のホスト）のアカウントかドメインに `federation` か `public_discovery` の制限があれば、取得せずに捨てる（連合タイムラインにも検索にも出ないので取り込まない）。
  4. `Create` / `Update` / `Delete` は ADR-0033 のとおりオリジンから取り直す。リレーの `Announce` は、`object` をオリジンから取得して取り込み、**ブーストとしては記録しない**（Mastodon と同じ）。
  5. 取り込むのは取得した投稿が `public` のときだけ。
- 取り込んだ投稿は、ほかのリモートの `public` の投稿と同じく **連合タイムラインとハッシュタグのタイムライン** に出る。ホームには出ず、通知も作らない。連合タイムラインが無効（`federated_timeline_visibility = off`）でも受信は止めない（ハッシュタグには出る）。

### 取り込みと保持（ADR-0034 への追加）

- ADR-0034 の「取り込む `Create`」の条件に「`receive` が `true` の `accepted` のリレーから届いた `public` の投稿（上限と処分の判定を通ったもの）」を加える。`Announce` の対象の取得の条件にも、主体が `accepted` のリレーであることを加える。
- リレー経由でだけ取り込んだ投稿は、`content` の `relayed_statuses` に記録する。
  消去ジョブ `content.purge_remote_statuses` は、`relayed_statuses` にあり、ローカルからの参照が無く、作者をローカルのアカウントがフォローしていない投稿を、公開から **7 日**（設定 `relay_status_retention`）で消す（`origin = retention`）。それ以外は ADR-0034 の 90 日のまま。
- 脱退しても、取り込んだ投稿はすぐには消さない（保持期間で消える）。

### データモデル（ADR-0020）

| テーブル | 所有 | 分類 | カラム |
| --- | --- | --- | --- |
| `relays` | `federation` | リソース | `id`, `protocol`（`enum(mastodon, litepub)`）, `url`（登録した URL。一意）, `inbox_url`, `receive`（`bool`）, `created_by_user_id`（NULL 可 = CLI）, `created_at` |
| `relay_events` | `federation` | イベント | `id`, `relay_id`, `kind`（`enum(follow_sent, accepted, rejected, left)`）, `follow_activity_uri`（`follow_sent` の `Follow` の `id`。ほかは対象の `Follow`）, `relay_actor_uri`（`accepted` の主体。ほかは NULL）, `actor_user_id`（NULL 可 = CLI か受信）, `occurred_at` |
| `relay_states` | `federation` | 投影 | `relay_id`（PK）, `state`（`enum(pending, accepted, rejected, left)`）, `follow_activity_uri`, `relay_actor_uri`（NULL 可）, `since` |
| `relayed_statuses` | `content` | イベント | `status_id`（PK）, `relay_id`, `received_at` |

- 参加と脱退の `Follow` / `Undo` は、`relay_events` を書くトランザクションで outbox に積む（`outbound_activities` の `object_type` に `relay` を足す）。
- 登録の削除は `relays` と子の行を消す（`relayed_statuses.relay_id` は参照を持たない ID として残す）。

### 管理 API・CLI・画面・監査ログ

- 管理 API: `GET /api/v1/admin/relays`（状態つきの一覧）、`POST /api/v1/admin/relays`（登録して参加。`protocol`・`url`・`receive`）、`POST /api/v1/admin/relays/{id}/follow`（参加し直す）、`POST /api/v1/admin/relays/{id}/unfollow`（脱退）、`DELETE /api/v1/admin/relays/{id}`（`left` / `rejected` の登録の削除）。
- 権限は `manage_settings`。リレー用の権限は分けない。
- CLI: `my admin relay add|follow|unfollow|remove|list`。
- 管理画面: リレーの一覧（方式・URL・状態・受信の有無・直近 24 時間の取り込み件数と上限で捨てた件数）と、登録・参加・脱退・削除の操作。
- 監査ログ（ADR-0037）: 登録・参加・脱退・削除を `target_kind = relay` で記録する。`Accept` / `Reject` の受信は `actor_kind = remote` で記録する。

### ほかの ADR・文書のうち置き換える部分

- ADR-0006「対応する仕様・拡張」の表の「非対応」の行のうち、**「リレー」** と **「Linked Data Signatures による署名生成」** を置き換える。リレー（Mastodon 方式・LitePub 方式）に対応し、`public` の投稿の `Create` / `Update` / `Delete` に LD 署名を付ける。
  「受信時の検証は互換のため検討」は置き換えない（受信は取り直しのまま）。
- ADR-0034 の「取り込む `Create`」の条件、`Announce` の対象の取得の条件、「保持と消去」に、上の「取り込みと保持」を加える。「この決定を見直す条件」の「リレーは v0 の非目標」の注記を置き換える。ほかの部分（関係による取り込み・90 日の保持・取得とリフレッシュ）は変えない。
- ADR-0033 は変えない（中継は `trust = refetch` のまま）。
- 00-vision.md の非目標から「リレー」を除く（この PR で更新する）。
- 機能要件に FR-411（リレーへの送信と受信）と FR-724（リレーの参加と脱退の管理）を足す（この PR で更新する）。

### この決定を見直す条件

- 計測で、中継の取り直し（1 件につき投稿と、未知の作者の Actor の取得）の費用が上限の範囲でも許容できない場合: 受信 Y（LD 署名の検証で取り直しを省く）を別の ADR で決める。
- LD 署名の生成が Mastodon・Misskey との相互運用テスト（federation-live）で通らない、または json-gold の正規化が相互運用上の問題を起こす場合: 送信 C（FEP-8b32）か B（Announce の経路のリレーに限る）を見直す。
- 主要なリレーのソフトウェアが LitePub 方式の `Follow` を受け付けない場合: LitePub 方式の登録を外す。

## Consequences（影響）

- 良い影響
  - 運営者はリレーに参加して、小さなインスタンスでも連合タイムラインとハッシュタグを埋め、自分の公開投稿を外に届けられる。
  - リレーに参加していないインスタンスの受信・保存・配送は変わらない。参加しても、取り込みは 1 時間の上限と 7 日の保持で頭打ちになる。
  - 中継された投稿は取り直すので、偽の中継で投稿を作られない。処分した作者・ドメインの投稿は取得前に捨てる。
- 悪い影響・トレードオフ
  - リレーから取り込む投稿は、1 件ごとにオリジンへの GET（と未知の作者の Actor の取得）が要り、受信のジョブ・行・外部 HTTP が流量に比例して増える。既定の上限（1 時間 1,000 件）で 1 日最大 24,000 件の取得になる（要検証: 主要なリレーの実際の流量）。
  - LD 署名のために JSON-LD の正規化（json-gold）を依存に足す。正規化は CPU を使い、`public` の投稿の配送ごとに 1 回かかる。同梱する `@context` の保守が要る。
  - Mastodon は LD 署名の付いた文書を JSON-LD として compact してから処理するので、こちらの文書が JSON-LD として不正だと LD 署名が捨てられ、中継が届かない（直接の配送は届く）。フィクスチャのテストで確かめる。
  - 送信は `Announce` を送らないので、Pleroma 系のリレーの一部（`Announce` だけを中継するもの）では、こちらの投稿が中継されない（要検証）。
  - リレーの IP にも通常のレート制限を掛けるので、大きなリレーでは `429` で取りこぼす（best effort）。
- 後続の作業（#667 の実装として PR を分ける）
  1. `relays` / `relay_events` / `relay_states` と、参加・脱退・`Accept` / `Reject` の処理、管理 API（OpenAPI）・CLI・監査ログ
  2. LD 署名の生成（json-gold、同梱の `@context`、フィクスチャのテスト）と、展開ジョブのリレーのフェーズ
  3. 受信の `relay_id`、上限と処分の判定、リレーの `Announce`、`relayed_statuses` と保持期間
  4. 管理画面（Web クライアント）
  5. federation.md・data-model.md・docs/operations の更新と、federation-live への Activity-Relay の場面の追加
  6. 設定 `relay_max_activities_per_hour`・`relay_status_retention` と、取り込み・破棄の件数のメトリクス（ADR-0055 サービスメトリクスの可観測性、#645）

## Pros and Cons of the Options（詳細比較・任意）

### 方式 1: 両方式に参加でき、送信は `Create` の原本

- 良い: Mastodon 方式しか受け付けないリレー（pub-relay の系統）にも、LitePub 方式のリレーにも参加できる。送信の経路は方式によらず 1 つ。
- 悪い: 登録に方式の選択が要る。LitePub 方式で `Announce` だけを中継するリレーには、こちらの投稿が届かないことがある。

### 方式 2: Mastodon 方式だけ

- 良い: 実装が最小。Activity-Relay を含む主要なリレーは Mastodon 方式を受け付ける。
- 悪い: LitePub 方式だけのリレー（Pleroma 系の運営者が立てたもの）に参加できない。方式 1 との差は `Follow` の `object` と Actor の取得だけで、省ける実装は小さい。

### 方式 3: LitePub 方式だけ（リレー用 Actor の `Announce`）

- 良い: LD 署名が要らない。受け取る側は `Announce` を見てオリジンから取得する。
- 悪い: Mastodon 方式しか持たないリレーに参加できない。Activity-Relay の購読者（Mastodon・Misskey）に原本で届けるには、結局 `Create` に LD 署名が要る。リレー用 Actor のブーストがフォロワーに見えないよう、表示の除外も要る。

### 送信 A: LD 署名

- 良い: Mastodon（全版）と Misskey の購読者が、原本の中継を検証できる。Mastodon・Misskey と同じ形。
- 悪い: JSON-LD の正規化の依存と CPU。`RsaSignature2017` は仕様として廃れつつある。

### 送信 B: 署名を足さない

- 良い: 依存が増えない。
- 悪い: 主な届け先（Activity-Relay の Mastodon・Misskey の購読者）に捨てられ、参加の価値の半分を失う（判断基準 3）。

### 送信 C: FEP-8b32（`eddsa-jcs-2022`）

- 良い: JSON-LD の処理が要らない（JCS、RFC 8785）。新しい方式。
- 悪い: 検証するのは 2026-07 以降の Mastodon の main だけで、Misskey は検証しない（判断基準 3）。Actor ごとに ed25519 の鍵と `assertionMethod` が要る。FR-410（C）として別に扱う。

### 受信 X: 取り直すと Y: LD 署名の検証

- X の良い点: ADR-0033 のまま。JSON-LD の処理を、信用できない入力に対して行わない。
- X の悪い点: 中継 1 件につき GET が 1 回要る。
- Y の良い点: 検証できれば GET を省ける。
- Y の悪い点: 信用できない入力の JSON-LD の処理（文脈の差し替えによる攻撃。Mastodon も compact と上限の検査で守っている）と、ADR-0033 の改定が要る。

## Open Questions / 要検証

- 主要なリレー（Activity-Relay を使う大きなリレー）の流量と、上限 1 時間 1,000 件・保持 7 日の既定値の妥当性（計測で見直す）
- aode-relay・ActivityRelay（Python）・Pleroma 系のリレーが、インスタンス Actor（`Application`）からの LitePub 方式の `Follow` と、`Create` の原本を受け付けるか
- Pleroma / Akkoma が、署名者と `actor` が違う中継をオリジンから取り直すか、LD 署名を検証するか
- json-gold の URDNA2015 と `RsaSignature2017` の署名が、Mastodon・Misskey の検証を通るか（federation-live で確かめる）
