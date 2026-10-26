## 5. 状態とポリシー

### 5.1 アカウントの状態の軸

| 軸 | 対象 | 投影 | 値 |
| --- | --- | --- | --- |
| 登録審査 | `users` | `user_registration_states` | `pending` / `approved` / `rejected` |
| メール確認 | `users` | `user_primary_emails` + `user_email_verifications` | 確認済みか |
| ライフサイクル | `accounts` | `account_lifecycle_states` | `active` / `deactivated` / `deletion_scheduled` / `deleted` |
| モデレーション上の制限 | `accounts`（とドメイン） | `account_restrictions` + `domain_restrictions` | 能力の集合（§5.2） |
| 権限 | `users` | `user_roles` + `role_permissions` | 権限の集合 |

軸は互いに独立しています。たとえば「承認済み・メール確認済み・有効・サイレンス中・モデレーター」のような組み合わせを、状態 enum を増やさずに表せます。

### 5.2 処分の種類と制限（能力の剥奪）

処分の種類（`moderation_actions.kind`）と、それが生む制限（`account_restrictions.restriction`）の対応表です。この表はドメインのコードに定義し、変更したら `account_restrictions` を再構築します（§9）。

| 処分 `kind` | 日本語 | 生む制限 | 備考 |
| --- | --- | --- | --- |
| `warn` | 警告 | なし | 通知だけ |
| `mark_statuses_sensitive` | 投稿のセンシティブ化 | なし（対象投稿に `status_sensitivity_overrides`） | `moderation_action_statuses` で対象を指定。期限なし（`expires_at` は NULL）。解除すれば元の表示に戻る |
| `delete_statuses` | 投稿の削除 | なし（対象投稿に `status_deletions`、`origin = moderation`、`moderation_action_id`） | 同上。期限なし。本人の削除と同じく投影から外し、作者にも表示しない。解除しても投稿は戻らない |
| `force_sensitive` | メディアの強制センシティブ | `media_sensitive` | 以後の添付をすべてセンシティブとして表示 |
| `limit` | サイレンス | `visibility_ceiling_private`, `public_discovery`, `unsolicited_reach` | 公開範囲の上限がフォロワー限定になる（§5.4） |
| `restrict_posting` | 投稿禁止 | `post` | 閲覧・フォロー・お気に入りはできる |
| `restrict_interaction` | 交流の禁止（S） | `interact` | フォロー・お気に入り・ブースト・投票・返信を禁止 |
| `disable_login` | ログイン停止 | `login` | 公開済みの内容は表示されたまま |
| `suspend` | 凍結 | `login`, `post`, `interact`, `profile_view`, `federation` | 期限付き可。解除すれば元に戻る。データは保持 |
| `ban` | BAN | `suspend` と同じ | **期限なし**（`expires_at` は NULL 必須）で、**解除できない**（FR-703。取り消せない操作として扱う、NFR-MOD-7）。例外は、削除までの 30 日のあいだの異議申し立てを覆したときで、削除を取り消して解除する（§4.3）。`account_lifecycle_events` に 30 日後の `deletion_scheduled` を発生させ（§4.2）、任意で `signup_blocks` を作る |

制限（能力）の enum:

| `restriction` | 意味 | 判定に使う場面 |
| --- | --- | --- |
| `login` | アプリ（認可・API）の利用の禁止 | 認証。有効なセッションとトークンは処分時に失効させる（`session_revocations` / `oauth_token_revocations`、`reason = moderation`）。サーバーが返す画面には処分を確かめるためにログインできる（§4.1） |
| `post` | 投稿・返信・ブースト・編集の禁止 | 投稿系ユースケース |
| `interact` | フォロー・お気に入り・ブースト・投票の禁止 | 交流系ユースケース |
| `visibility_ceiling_private` | 公開範囲の上限を `private` にする | 投稿時の公開範囲の決定、`canView`（§5.4） |
| `public_discovery` | ローカル / 連合 / ハッシュタグのタイムライン・検索・おすすめに出さない | タイムライン・検索 |
| `unsolicited_reach` | フォローされていない相手への通知・メンションを届けない | 通知生成 |
| `media_sensitive` | 添付を常にセンシティブとして表示 | 表示 |
| `profile_view` | プロフィール・投稿を本人とモデレーター以外に表示しない | `canView`、Actor・オブジェクトの GET（404） |
| `federation` | 連合しない（リモートなら受信を破棄、ローカルなら配送しない） | Inbox 処理・配送 |
| `media_fetch` | メディアを取得・表示しない（ドメイン単位の `reject_media`） | メディアプロキシ |
| `report_intake` | 通報を受け付けない（ドメイン単位の `reject_reports`） | `Flag` の受信 |

- 新しい処分は、`kind` に値を足して対応表に 1 行書くだけで追加できます（テーブル変更なし）。
- リモートアカウントへの処分は、制限だけを生む `limit` / `suspend` / `force_sensitive` を同じ表で扱います。ほかの種類（`login` や `post` の制限、本人への通知、投稿への処分、`ban`）はリモートには意味がないので受け付けません（`moderation.ErrUnsupportedAction`）。`notify_target` は常に偽です。
- リモートのアカウントの `suspend` は、同じトランザクションで両方向のフォローを切ります（`relationship_severances`）。切断は、ローカルのアカウントと同じく凍結の始まりに発行される `moderation.account_suspension_changed`（凍結した処分の ID を持つ）と同じ時点で行います。
- ローカルのアカウントの `suspend` / `ban` とその解除・期限切れ、`delete_statuses` が連合先に送るものは [ADR-0036](../../adr/0036-moderation-federation-propagation.md) と [federation.md §11](../federation.md#11-モデレーションとの連携) にあります。
- ドメイン単位の処分の対応: `limit` → `public_discovery`, `unsolicited_reach`、`suspend` → `profile_view`, `federation`、`reject_media` → `media_fetch`、`reject_reports` → `report_intake`、`force_sensitive` → `media_sensitive`。

### 5.3 判定（RestrictionPolicy）

ドメイン層の `RestrictionPolicy` が、次の 4 つから「有効な制限の集合」を作ります。

1. `account_restrictions`（期限内のもの）
2. `domain_restrictions`（対象ドメインと親ドメイン、期限内のもの）
3. ライフサイクル（`deactivated` / `deletion_scheduled` / `deleted` は `login`・`post`・`interact`・`profile_view` 相当として扱います。本人による無効化は再有効化でログインできるため、`login` は除きます）
4. 登録審査（`approved` 以外は、ログインはできても投稿・交流はできない）

ユースケースは `policy.CanLogin()`、`policy.CanPost()`、`policy.VisibilityCeiling()`、`policy.Discoverable()` などを通じてだけ判定し、テーブルを直接見ません。
1 と 2 は `moderation.AccountPolicy` が呼ばれるたびに読み、各モジュールは自分のポートを通して判定します。

| モジュール | ポート | 判定すること |
| --- | --- | --- |
| `content` | `RestrictionPolicy`（`AccountPolicy.Content()`） | 投稿・編集（`post`）、返信・お気に入り・ブースト（`interact`。ブーストは `post` も）、公開範囲の上限、閲覧、添付のセンシティブ表示（`media_sensitive`） |
| `graph` | `InteractionPolicy` | フォロー（`interact`） |
| `identity` | `LoginPolicy` | 認可とアクセストークンの照合（`login`。要求ごと） |
| `notification` | `ReachPolicy` | 通知の作成（`unsolicited_reach`。受け取る人が差出人をフォローしていなければ作らない） |
| クライアント API | `ProfileRestrictions` | アカウントの表示（`profile_view`。本人以外には存在しないものと同じ 404） |

`login` を含む制限の変化（`moderation.account_restrictions_changed`）を受けて、`identity` の同期の購読者が処分と同じトランザクションでその利用者のセッションとトークンを失効させます（`reason = moderation`）。解除しても失効したものは戻りません。
タイムラインの SQL は、`feed_author_restrictions` / `feed_domain_restrictions` を同じ規則で写像した条件を使います（共通のクエリビルダ）。

### 5.4 サイレンスと公開範囲

オーナーの定義: サイレンスされたアカウントの **全部の投稿がフォロワー限定になる**。

- **投稿時**: 要求された公開範囲を上限で切り詰めます。`public` / `local` / `unlisted` は `private` になり、`private` / `direct` はそのままです。切り詰めた結果を `statuses.visibility`（不変）に記録します。
- **過去の投稿**: `statuses.visibility` は書き換えません。`canView` が「実効公開範囲 = min(元の公開範囲, 上限)」で判定するので、このサーバー上ではサイレンス中は過去の投稿もフォロワー限定として扱われ、解除すれば元に戻ります。
- **連合**: 公開として配送済みの過去の投稿を、連合先から取り消すことはできません（ActivityPub の制約）。サイレンス後の新しい投稿は `private` として配送されます。
- 実効公開範囲は `content.EffectiveVisibility` が求めます。`canView` の判定表は [公開範囲の閲覧判定](../visibility.md) にあります。
- **投稿の前**: 上限と、要求どおりに記録される公開範囲（`content.PostingVisibilities`。`EffectiveVisibility` が変えないもの）は `content.Service.PostingLimits` が返し、クライアント API は閲覧者の `CredentialAccount` の `posting` で示します。クライアントは投稿の前に上限を知らせられます。

### 5.5 通話のルームの判定

通話のルーム（§4.14）が見えるか・許可なしに話者になれるか・ホストになれるかは、`internal/domain/voice` の純粋な関数が決めます（[ADR-0100](../../adr/0100-voice-chat-rooms.md) §2・§4）。
フォロー（承認済みの `follow_edges`）・ブロック（`block_edges`、どちら向きも）・招待・禁止・明示の許可・アカウントの状態と制限は `internal/app/voice` が集めて `voice.Facts` / `voice.HostFacts` で渡します。

- `CanSee(room, viewer, facts)`: 1. 終わったルームは偽、2. 未ログイン・リモート・凍結・削除の閲覧者は偽、3. ホストか共同ホストとブロックの関係なら偽、4. 退出させられた（禁止の）閲覧者は偽、5. ホスト・共同ホスト・参加者は真、6. 取り消されていない招待があれば真、7. `visibility` が `local` なら真、`followers` はホストをフォローしていれば真、`mutuals` は相互フォローなら真、`invited` は偽。
- `CanSpeakFreely(room, participant, facts)`: 1. ホスト・共同ホストは真、2. 制限（limited）されたアカウントは明示の許可がなければ偽、3. 明示の許可（挙手の許可・ホストの許可・話者としての招待）があれば真、4. `speakers` が `everyone` なら真、`followers` はホストをフォローしていれば真、`mutuals` は相互フォローなら真、`manual` は偽。
- `CanHost(facts)`: ローカルの有効な（凍結・削除されていない）アカウントで、制限されておらず、`voice_room_starters` が `everyone` か権限 `start_voice_rooms` を持つ。`NextHost` は指名の順の共同ホストのうち `CanHost` が真の最初の人を返し、いなければルームは終わります。
- 「フォロワー」「相互フォロー」の基準はその時のホスト（`voice_room_heads.host_account_id`）です。範囲を狭めても、ホストが交代しても、参加中の人は残ります（ADR-0100 §3）。
