# ADR-0089: アカウントの移転（Move）と別名（alsoKnownAs）、本人による無効化

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1035, #2079

## Decision（決定）

アカウントの移転は、Mastodon と同じ形でフォロワーだけを移す（§1〜§10）。投稿は移さず、フォロー中・ブロック・ミュートは CSV（FR-008）で運ぶ。
本人による無効化は、30 日の猶予の後に削除する。連合では凍結と同じ `Update(Actor)` で表す（§11〜§18）。

### 1. 語彙

- Actor に `alsoKnownAs`（`as:alsoKnownAs`、IRI の配列）と `movedTo`（`as:movedTo`、IRI）を出す。`@context` は既存の Actor と同じ。
- 送る `Move` は `actor` と `object` がこちらの Actor、`target` が移転先の Actor、`to` がこちらの `followers` コレクション（FEP-7628 の push mode。Mastodon の文書の例と同じ形）。`id` は `https://{host}/ap/activities/{uuid}`（[ADR-0018](0018-federated-uris.md)）。

### 2. 移ってくる側: 別名の設定

- ローカルの利用者は、設定で別名（移転元のアカウント）を足し・消しできる。入力はハンドルか URL で、WebFinger と Actor の取得（SSRF ガード付きの `kernel.HTTPClient`）で Actor の `id` に解決して保存する。上限は 5 件。
- 別名の変更は Actor の `Update` を、ほかのプロフィールの変更と同じ宛先へ配送する（`publish` の流れ）。
- 別名は移転を受け入れる表明でしかなく、それだけでは何も起きない（Mastodon と同じ。§9）。

### 3. 送る側: ローカルから外へ移る

- 操作は client API の 1 つのエンドポイントで、パスワード（パスワードの無い利用者はパスキー）の再認証を要求する。Web は確認のダイアログで、移るもの（フォロワー）と移らないもの（投稿・フォロー中・ブロック・ミュート・ブックマーク）を示し、CSV のエクスポート（FR-008）へのリンクを出す。
- 送る前に、移転先の Actor をキャッシュを使わずに取得し直し、`alsoKnownAs` にこちらの Actor の `id` があることを確かめる。無ければ `422` で断る。移転先がローカルのアカウントでも同じ規則で、同じサーバー内の移転もできる。
- 断る条件: 前回の移転から 30 日以内（Mastodon と同じ待ち時間。2026-10-23 に [Moving or leaving accounts](https://docs.joinmastodon.org/user/moving/) で確認）、移転先が自分自身・移転先がすでに移転済み（`movedTo` がある）、移転先のドメインが `suspend` の処分中、こちらが凍結・BAN 中（そもそもログインできない）。
- 移転の記録と同じトランザクションで、`Move` と Actor の `Update`（`movedTo` 付き）を outbox に積む。Actor の GET もこのトランザクションの後は `movedTo` を返す（Misskey は `Move` を受けると Actor を取り直し、その `movedTo` で移転を処理するため）。宛先は承認済みのリモートのフォロワーの inbox を shared inbox へ集約したもの（`Delete(Actor)` と同じ展開）。ローカルのフォロワーは、受ける側と同じ処理（§4）を直接ジョブで呼ぶ。
- 移転後のアカウント（以下「移転済み」）: ログイン・閲覧・エクスポート・削除・別名の設定・移転の取り消しはできる。投稿・ブースト・反応・フォロー・投票はできない（client API が `403`、problem type `account-moved`）。既存の投稿は残り、外からのフォロー（`Follow`）は `Reject` を返す。プロフィールに「移転しました」の帯と移転先へのリンクを出す。
- 移転の取り消し: 利用者は移転済みを解除できる。Actor の `Update`（`movedTo` なし）を送り、投稿などが再びできるようになる。移ったフォロワーは戻らない（相手のサーバーは `Move` を取り消さない）。取り消しは `account_move_cancellations` に記録し、`account_move_states` の行を消す。取り消しても 30 日の待ち時間は前回の移転から数える。

### 4. 受ける側: 外のアカウントの移転を受ける

- `Move` のハンドラを federation に登録し、次をすべて満たすときだけ処理する。満たさなければ理由をログとメトリクスに残して完了にする。
  1. `actor` と `object` が同じで、署名者が `actor`（federation.md §3.2 の確かめ）。FEP-7628 の pull mode（移転先が送る `Move`）は受けない（Mastodon・Akkoma も受けない）
  2. `target` の Actor をキャッシュを使わずに取得し、その `alsoKnownAs` に `actor` の `id` がある
  3. `target` が `actor` と異なり、`target` 自身が移転済みでない（移転の連鎖をたどらない）
  4. 移転元・移転先のアカウントにも、そのドメインにも、このサーバーの `suspend` の処分がない
- 処理を通ったら、移転元に移転を記録し（§5）、移転元の Actor の表示に移転先を出す。
- 移転元をフォローしているローカルの利用者ごとに、ジョブ `graph.migrate_follows`（冪等キーは移転元と移転先の組）で、100 件ずつのページで次をする。
  - 利用者が移転先をブロックしていない・移転先のドメインをミュートしていないときだけ、移転先へフォローし（`Follow` を送る。移転先が鍵アカウントなら承認待ちになる）、移転元へのフォローを `follow_terminations`（`reason = moved`）で終えて `Undo Follow` を送る。フォローの設定（`show_boosts`・`notify`）は引き継ぐ。移転先をフォローしないときは移転元へのフォローを残す（Mastodon と同じ）。移転先からブロックされていれば相手の `Reject` で終わる。
  - 移転元をブロック・ミュートしているローカルの利用者には、移転先にも同じブロック・ミュート（ミュートは通知の扱いと期限も）を足す（Mastodon・Misskey と同じ）。
  - 利用者に通知 `account_moved`（フォローしている人が移転した。移転先をフォローしたか、しなかった理由）を送る。
- 移転元のフォロワーではない利用者の表示は、移転元のプロフィールの帯だけが変わる。
- 負荷と悪用: 同じ移転元の `Move` は移転先ごとに 1 回だけ処理する（記録の一意制約）。同じ移転元からの別の移転先への `Move` は、前の移転の処理から 7 日以内なら捨てる（Mastodon は移転元ごとに 7 日、GoToSocial は成功から 1 週間）。フォローの移し替えはドメインごとの配送の並列度とサーキットブレーカー（ADR-0006）にそのまま従い、ジョブはページごとに間隔を空ける（値はコードの `MigratePageInterval`）。

### 5. データモデル（所有モジュール: `account`）

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `account_aliases` | 現在値 | `account_id`, `alias_uri`, `added_at` | PK `(account_id, alias_uri)`。ローカルのアカウントだけ。リモートの `alsoKnownAs` は移転の検証のときに取得し、保存しない |
| `account_moves` | イベント | `id`, `account_id`（移転元）, `target_account_id`, `initiated_by`（`enum(self, remote)`）, `activity_uri`（NULL 可。受けた `Move`）, `occurred_at` | 一意 `(account_id, target_account_id)` |
| `account_move_cancellations` | イベント | `move_id`（PK）, `cancelled_at` | ローカルの移転だけ |
| `account_move_states` | 投影 | `account_id`（PK）, `move_id`, `target_account_id`, `since` | 取り消しで行を消す。`account_lifecycle_states` と直交する軸（ADR-0020 規約 2） |

- `graph` の `follow_terminations.reason` には `moved` がある。移し替えは `graph` の公開 API を、`account.moved` のドメインイベントを購読するジョブから呼ぶ。
- 待ち時間の判定は `account_moves` の最新行の `occurred_at` で読み取りのときに行う（状態の列を持たない）。

### 6. 連合の向き（ADR-0006）との関係

| 活動 | `Flow` |
| --- | --- |
| `Move` の受信とフォローの移し替え | `follow_out` |
| `Move` の送信と `movedTo` 付きの `Update(Actor)` | `retract`（新しい内容を出さず、すでにいるフォロワーにだけ行き先を伝えるため。`inbound` でも送れる） |
| 別名の `Update(Actor)` | `publish` |

ADR-0006 の流れの表の `retract` は、`Move` の送信を含む。

### 7. モデレーション

- 凍結・BAN 中のローカルのアカウントは移転できない。`limit` などの制限中は移転できるが、移転先がローカルのアカウントなら制限を移転先にも付けるかをモデレーターに知らせる（監査ログに `account_moved` を記録し、通知のキューに出す）。自動では引き継がない。
- リモートの移転元にこのサーバーの `limit` がある場合も、移転先へは自動で引き継がず、モデレーターに知らせる。
- ドメインの `suspend`（ADR-0058）は、移転元・移転先のどちらのドメインでも移し替えを止める。

### 8. クローズ（ADR-0088）との関係

- 予告の期間に、Web は全員に移転の手順（移転先で別名を設定 → こちらで移転）と CSV のエクスポートを案内する。
- クローズの `Delete(Actor)` は移転済みのアカウントにも送る。フォロワーは移転の時点で移っており、`Delete` で移転先の関係は消えない。移転済みのアカウントへの `Delete` はクローズの配送の最後のフェーズにまとめ、配送が終わるまで移転済みの Actor の GET は `movedTo` を返し続ける（ADR-0088 §4）。
- 移転の送信と移し替えのジョブは、クローズの配送より前に終わっている必要がある。ADR-0088 はこの ADR を前提にする。

### 9. 主要実装の挙動（2026-10-23 にソースコードで確認）

| 実装 | 受けた `Move` の検証 | 受けた後の処理 |
| --- | --- | --- |
| Mastodon（[move.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/lib/activitypub/activity/move.rb)、[move_worker.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/workers/move_worker.rb)、[follow_migration_service.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/services/follow_migration_service.rb)） | `object` が送り主、`target` を取得し直して `alsoKnownAs` に送り主がある。移転元ごとに 7 日に 1 回 | フォロワーごとに移転先へ `Follow`（設定を引き継ぐ）を送ってから移転元を `Undo Follow`。フォローできなければ移転元のフォローは残る。ブロック・ミュートとメモを移転先へ写す |
| Misskey（[ApInboxService.ts](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/activitypub/ApInboxService.ts)、[AccountMoveService.ts](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/AccountMoveService.ts)、[ApPersonService.ts](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/activitypub/models/ApPersonService.ts)） | `Move` を受けると送り主の Actor を取り直し、その `movedTo` と移転先の `alsoKnownAs` で確かめる。移転の連鎖と 14 日以内の再移転は処理しない | フォローの移し替え、ブロック・ミュート・リストを移転先へ写す。送る側は `Update(Actor)` と `Move` をフォロワーへ配送する |
| GoToSocial（[fromfediapi_move.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/311cecaeb6e302509c847d20e21185a7ec41d28d/internal/processing/workers/fromfediapi_move.go)） | `target` を取得し直して `alsoKnownAs` を確かめる。移転先が移転済み・凍結中なら処理しない。同じ組の試行は 5 分、成功は 1 週間あけて | ローカルのフォロワーを移転先へ移し、移転元がローカルの人へのフォローを消す |
| Akkoma（[transmogrifier.ex](https://akkoma.dev/AkkomaGang/akkoma/src/commit/a69f0b9e328dd6c12bf4a308892041035408d03a/lib/pleroma/web/activity_pub/transmogrifier.ex)） | `actor` と `object` が同じ、`target` を取得し直して（5 秒より古ければ）`alsoKnownAs` に送り主がある | フォローの移し替えをジョブで行う |

移るのはフォロワーだけで、別名は移転のためだけにある（Mastodon。2026-10-23 に [Moving or leaving accounts](https://docs.joinmastodon.org/user/moving/) で確認）。

[FEP-7628](https://codeberg.org/fediverse/fep/src/commit/fc90938eec71ac07f5186955926fc27e6b64f15f/fep/7628/fep-7628.md)（FINAL）は、移転元が送る push mode をフォロワー宛てにすること、受けた側が移転先を `Follow` するか利用者に知らせることを求める。§4 の検証と処理はこれを満たす。FEP の pull mode と `copiedTo` は主要実装が使っていないので扱わない。

### 10. 相互運用の確かめ方

`deploy/docker/federation-live` に、Mastodon・GoToSocial・Misskey それぞれとの往復（こちら → 相手、相手 → こちら）のシナリオを置く: 別名の設定、`Move`、相手のサーバーでフォロワーが移ること、こちらでフォローが移り通知が出ること。Akkoma はこのシナリオの相手ではないので、Akkoma が送る `Move` の JSON のフィクスチャを testkit に置いて回帰テストにする。

### 11. 無効化: 本人ができること

- 無効化は本人だけが行います。`initiated_by = self` だけで、モデレーターの操作にはしません。処分は処分で行います（FR-703）。
- 一次のクライアントだけの操作にします（ADR-0090 §4）。無効化には、パスワードか二要素認証のコードをもう一度入力するか、そのアカウントにあるパスキーでの確認が要ります。パスワードもパスキーも持たない人は、連携先のログインで確かめます。
- 次の状態では断ります: 処分でログインできない（`disable_login`・`suspend`・`ban`）、削除の予約中、移転済み、登録の審査中、有料の支援が続いている、ログインできるほかの管理者がいない最後の管理者である。最後の管理者を断るのは、期限の削除でサーバーに管理者がいなくなるためです。ほかの経路と同じく、最後の管理者を守る決まりに従います。支援は解約へ案内します。見えない人に課金を続けず、期限の削除で課金の扱いが問題にならないようにするためです。
- 無効化の間にできるのは、サインイン・再有効化・削除だけです。パスワードの再設定は使えます。データのエクスポートは再有効化してから行います。
- 無効化の時点で、すべてのセッションとアクセストークンを終わらせます。戻ったとき、アプリは認可をやり直します。

### 12. 無効化: 期間と削除

- 無効化から 30 日（`deactivated` の `since` から 30 × 24 時間）で削除します。`account.execute_deletions` が期限を過ぎた `deactivated` を読み直して、`initiated_by = system` の `deleted` を記録します。以後は本人の削除と同じ経路で、このときに `Delete`（Actor）を送り（[federation.md §7.1](../architecture/federation.md)）、7 日後に消去します。ユーザー名は解放しません。
- スキーマは変えません。期限は `since` から求めます。`deletion_due_at` は `deletion_scheduled` のものなので、`deactivated` では使いません。
- 期限の前なら必ず再有効化でき、期限を過ぎたら、ジョブが記録するまでの隙間でも断ります。戻った後の無効化は、`since` が変わるので 30 日を数え直します。
- 無効化の間に処分を受けても期限は変わりません。`ban` は `deletion_scheduled` に移るので、その時点から再有効化できず、`ban` の期日に従います。`ban` の取り消しの `deletion_canceled` は、畳み込みのとおり `active` に戻します。保全（`retention_holds`）は消去を止めるだけで、削除は止めません。

### 13. 無効化: 知らせ

無効化の確認と完了の画面で、削除の日付を伝えます。再有効化の画面にも削除の日付を出します。

無効化の時点で、サーバーがメールを送る構成なら、削除の日付を書いた確認のメールを 1 通送ります。ほかに削除の前のメールはありません。

### 14. 無効化: 再有効化

- 無効化中の人のサインインは、二要素認証まで含めて通常どおり通り、`Access.LoginRestricted` と同じく、サーバーが描く画面だけに入れます。アクセストークンは出しません。
- 画面には、再有効化と削除のボタンを出します。再有効化を押すと、イベント `reactivated` を記録します。
- 処分は解除されません。再有効化の後も、処分の規則に従います。無効化の間に処分でログインを止められた人も、同じ画面から再有効化できます。

### 15. 無効化: ほかの人に見えるもの

- プロフィールは、ローカルでも `404` です。検索・おすすめ・メンションの補完からも除きます。
- 投稿は、タイムライン・検索・スレッド・埋め込みから外します。`feed_author_restrictions` の `profile_view` を、`deactivated` でも作り、`reactivated` で除きます。削除と違って行を戻せます。投稿ごとの判定（スレッド・投稿の GET・埋め込み）は、作者のライフサイクルが `active` でないものを `profile_view` と同じに扱います。
- ブーストも同じように消えます。引用先としては「表示できない投稿」です。
- 通話: 開いているルームを終え、参加を終わらせます（凍結と同じ経路）。
- モデレーターの画面には、無効化中であることと通報を出します。無効化しても通報と処分は続けられます。

### 16. 無効化: その間に届くもの

- **メンションと DM は、ローカルからもリモートからも捨てます。** 戻っても読めません。
  - ローカルの投稿では、存在しないユーザーの名前と同じように扱います。名前は本文の文字のまま残り、宛先にもメンションにもなりません。投稿した人には何も伝えません。
  - リモートの投稿では、この人への `Mention` と宛先を無視します。宛先がこの人だけの投稿は保存しません。
  - 捨てるのはこの人への宛先で、投稿そのものではありません。ほかの人に見える投稿は残り、戻った後に公開の投稿として読めることはあります。
- **通知・メール・Web Push は作りません。** お気に入りとブーストは投稿の数として記録しますが、通知しません。
- **リモートの新しいフォローには `Reject(Follow)` を返します。** 戻っても復元しません。

**フォローとフォロー申請**は、戻るときに元のままであるよう、次のとおりにします。

| 対象 | 振る舞い |
| --- | --- |
| すでにあるフォロー（両方向、ローカル・リモート） | 残す。無効化しても切らない。ほかの人の一覧からは除き、件数は変えない |
| ローカルからの新しいフォロー | 存在しない相手と同じく断る。戻っても復元しない |
| 無効化の前に届いていた、この人への申請 | 待ちのまま残す。相手には待ちのまま見える。答えるのは戻った後で、期限の削除で終わる |
| この人が出していた申請 | 残す。相手の一覧からは除く。リモートの `Accept` / `Reject` は処理する |
| ほかの人によるフォローの解除 | リモートの `Undo(Follow)` は処理する。ローカルの人はプロフィールを開けないので、無効化の間は解除できない |

新しいものは拒み、すでにあるものは触りません。これなら、戻った人にとっても相手にとっても、関係は無効化の前と同じです。

**個人の inbox は受け付けます**（`202`）。`404` では `Reject(Follow)` を返せず、相手の配送が永続の失敗になって、`Undo(Follow)` や `Accept` も届かなくなるからです。`Follow` は `Reject` し、関係と Actor についての活動は処理し、この人への `Create` は上の規則で捨てます。shared inbox も同じで、ほかの宛先には影響しません。

### 17. 無効化: 連合

| 契機 | 送るもの | 宛先 |
| --- | --- | --- |
| `deactivated` | `Update(Actor)`（`suspended: true`、プロフィールを空にする） | `Delete`（Actor）と同じ |
| `reactivated` | `Update(Actor)`（元のプロフィール） | 同上 |
| 30 日後の `deleted` | `Delete`（Actor）（§12） | 既存の経路 |

- 凍結と同じ `federation.ActorDocument.Suspend` を使います。Actor の GET は、id・コレクション・inbox・鍵を残した最小の文書を返します。WebFinger は答えます。
- 凍結中の無効化と、無効化中の処分では、状態が「見えない」から変わらないので何も送りません。見えるようになるのは、処分が無く、かつ `active` のときだけです。
- 投稿の GET は `404`（`410` にしない）です。投稿ごとの `Delete` は送りません。

`suspended` の語彙にほかに候補がなく、連合先の UI では「モデレーターに凍結された」ように見えうることは、新しい語彙を作らないために受け入れ、無効化の画面で説明します。

### 18. 無効化: 繰り返しの上限

再有効化はいつでもできます。無効化は、直前の再有効化から 24 時間たつまで断ります。全フォロワーの宛先への `Update(Actor)` とタイムラインの処理が、繰り返しで重くならないようにするためです。戻る側を止めないのは、本人を閉じ込めないためです。

### この決定を見直す条件

- 主要実装のいずれかが、ここで送る `Move` の形でフォロワーを移さないことが相互運用テストで分かった場合（その実装の求める形に合わせるか、送る `Move` を実装ごとに変えるかを決め直す）。主要実装が FEP-7628 の pull mode や `copiedTo` を送るようになった場合も見直す。
- Mastodon など主な実装に、本人の無効化を表す語彙ができたら、§17 の文書を見直す。
- `suspended` を読まない主な実装が多いと分かったら、本人への説明を強める。
- 気付かないまま削除されたという報告や、乗っ取りによる削除が起きたら、削除の前にメールで知らせる案を見直す。

## 経緯

`Move` は当初 ADR-0006 と vision の非目標で「非対応」だった。M5 で範囲に入れたのは、サービスのクローズ（ADR-0088、#1034）の予告の期間に利用者がフォロワーを移せないと、クローズでフォロー関係がすべて失われるからである。別のサーバーとの行き来にも使う。要件は FR-008・FR-009・FR-405・FR-413・FR-414、NFR-SEC-1。投稿の移し替えなど主要実装もしないことはせず、同意（`alsoKnownAs`）のない移転でフォローを奪われないことを優先した。

無効化は、データモデルに `deactivated` / `reactivated` の値がありながら書くコードがないことが `v0.1.0` 前のスキーマの監査（#2027）で分かり、値を除く推奨に対して、機能を v0 に入れて値を残すと決めた。無期限に隠せる状態として検討を始め、データを持ち続ける終わりのない状態にしないため、いったん離れて考え直せる 30 日の猶予に落ち着いた。処分を逃れる手段にせず（FR-703）、凍結の部品（`Update(Actor)` と `suspended: true` の文書、`Access.LoginRestricted`）と削除の経路を再利用する。

Mastodon に利用者向けの無効化はない（2026-11-03 に確認。[移転と削除の文書](https://docs.joinmastodon.org/user/moving/)、要望 [#33869](https://github.com/mastodon/mastodon/issues/33869) は未回答）。凍結は [ActivityPub の文書](https://docs.joinmastodon.org/spec/activitypub/)のとおり空の文書と `Update` で表し、v4.7.4 の [`process_account_service.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/services/activitypub/process_account_service.rb) は `suspended` が無くなると戻し、[`suspend_account_service.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/services/suspend_account_service.rb) はこの場合にフォローを切らない。無効化はこの文書を借りる。

## 採らなかった案

- データも運ぶ移転: フォロー中・ブロック・ブックマーク・投稿を API や独自のアクティビティで移転先へ運ぶ。利用者の手間は減るが、移転先が同じソフトウェアのときしか効かず、相手のサーバーは解さず、投稿の移し替えには前例がない。
- 移転の非対応: 実装も検証もいらないが、クローズでフォロワーを失い、全員にフォローし直してもらうことになる。
- 無効化の期間を無期限にする: 取り消せない削除が本人の操作でしか起きないのが強みだが、離れた人のデータを無期限に持ち、連合先の凍結の表示にも終わりがない。投稿を残して離れたい人には、削除の前のエクスポートと移転がある。
- 期限を運営者が設定する: 利用者に見えるサーバーごとの規則を増やす。
- 画面で日付を伝えるだけ（メールなし）: 本人の知らないうちに無効化されたときに気付く手段がない。
- 削除の前に期限が近いことをメールで知らせる: 気付かないまま削除される人を減らせるが、離れると決めた人へ後から連絡すること自体が目的に反する。
- サインインで自動的に再有効化する: 手軽だが、盗まれたパスワードやサインインを試すだけの人が、本人の意図と別に公開へ戻してしまう。
- 運営者に頼んで再有効化する: 小さなサーバーで負担になり、期限のある仕組みでは間に合わないことがある。
- 無効化を連合に送らない: 送るものが少なく誤解もない最も強い対案だが、連合先でプロフィールと投稿が見えたままで離れていられない。
- 無効化の時点で `Delete`（Actor）を送る: 確実だが、Mastodon が Actor の `Delete` で投稿とアカウントを消すので戻れない。
- 投稿ごとの `Delete` と作り直し: 戻すときに投稿を送り直すことになり、id の再利用と大量の配送が問題になる。
- 留守の間のものを記録して通知だけ止める: DM を失わないが、戻った途端に留守の間のものが届くので離れていられず、削除される 30 日の間の記録を増やすだけになる。
- 個人の inbox も `404` にする: `Reject(Follow)` を返せず、相手の配送が永続の失敗になる。

## Consequences（影響）

- 良い影響
  - クローズの前に利用者がフォロワーを移せる。ほかのサーバーとの行き来もできる。
  - `alsoKnownAs` の取得し直しと署名者の一致で、同意のない移転を受けない。
  - 移転を `account` のイベントと投影で持ち、ライフサイクルと直交させるので、削除・凍結の判定に手を入れない。
  - 投稿を消さずに、30 日のあいだ離れて考え直せる。無効化にスキーマの変更はなく、データは期限で消える。
- 悪い影響・トレードオフ
  - 投稿・フォロー中・ブックマークは移らない。利用者は CSV で運ぶ必要がある。
  - 受けた `Move` 1 件で、フォロワーの数だけ `Undo Follow` と `Follow` の配送が生じる。大きなサーバーの一斉の移転では配送のキューが伸びる。
  - 移転の取り消しを許すので、相手のサーバーでは移転済みのまま、こちらでは投稿できる状態が生じる（Mastodon のリダイレクトの取り消しと同じ）。
  - ADR-0006 の `retract` の意味が「撤回と行き先の通知」に広がる。
  - **取り消せない削除が、本人の操作なしに起きる。** 無効化したことを忘れた人は、無効化のときのメールのほかに知らせがないまま、投稿とアカウントを失う。
  - **パスワードを盗んだ人は無効化できる。** 本人は確認のメールで気付き、期限までにログインして戻せる。メールを送らない構成のサーバーでは、気付く手掛かりはセッションが切れたことだけである。
  - 留守の間の DM とメンションは失われ、送った人には分からない。
  - 連合先で「凍結された」ように見えることがある。`suspended` を読まない実装には投稿が残る。
  - 正しいパスワードで「間違い」と答えないので、無効化中であることが、パスワードを知っている人には分かる。
  - フォロワーの件数は、無効化中の人を含む。データのエクスポートは、再有効化を挟む。
  - トークンを終わらせるので、戻った後にアプリの認可をやり直す必要がある。

## Open Questions / 要検証

- 要検証: こちらの送る `Move` で、Akkoma の側のフォロワーが実際に移ること。Akkoma は `deploy/docker/federation-live` の `TestMove` の相手ではない。Mastodon・GoToSocial・Misskey は `TestMove` で確かめている。Misskey 2026.10 が移転先が Misskey 自身のアカウントの `Move` の処理に失敗するのは確かめた既知の非互換で、`TestMove` は Misskey では別のサーバーのアカウントへ移す。
- 要検証: Misskey・Akkoma・GoToSocial が、`suspended: true` の `Update(Actor)` を受けて隠すか、戻るか。`federation-live` の `TestSuspension` は Mastodon だけを確かめている。
