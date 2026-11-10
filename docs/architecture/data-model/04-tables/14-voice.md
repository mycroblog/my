### 4.14 voice

同じサーバーのローカルの利用者どうしの通話のルーム（FR-920〜FR-932、[ADR-0100](../../../adr/0100-voice-chat-rooms.md) §12）。`internal/app/voice` が所有し、表はすべて `voice` だけが書きます。
通報の子の表 `report_voice_rooms` は `moderation` の表です（§4.3）。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `voice_rooms` | リソース | `id`, `opened_by_account_id`, `opened_at`, `transport`（`enum(p2p, livekit, sora)`。開いたときの接続の方式） | — |
| `voice_room_revisions` | イベント（版） | `id`, `room_id`, `name`（書記素で 1〜100。コード点で 1000 まで。書いたアカウントの消去で `''`）, `description`（`text`。500 文字まで。なしは `''`）, `visibility`（`enum(local, followers, following, mutuals, invited)`）, `speakers`（`enum(everyone, followers, following, mutuals, manual)`）, `icon_emoji_key`（アイコンの絵文字。§4.4 の `reactions.emoji_key` と同じ形で、Unicode 絵文字は fully-qualified の列、カスタム絵文字は `custom_emojis.id`。なしは `''`）, `icon_custom_emoji_id`（NULL 可 = Unicode 絵文字かアイコンなし。`icon_emoji_key` と一致）, `revised_by_account_id`, `revised_at` | インデックス `(room_id, id DESC)`, `(revised_by_account_id, room_id)`（アカウントの消去）。開始の設定が最初の版。アイコンは書くときにリアクションと同じ条件（ADR-0051 §7: Unicode 絵文字か、このサーバーの有効な画像のあるカスタム絵文字）で確かめ、カスタム絵文字の無効・削除は読み取り時に判定してアイコンなしで出す |
| `voice_room_revision_hashtags` | リソース（版の子） | `revision_id`, `position`（`int`。0〜4）, `name`（`string(100)`。表示の綴り）, `name_normalized`（`string(100)`。投稿のハッシュタグと同じ正規化） | PK `(revision_id, position)`。一意 `(revision_id, name_normalized)`。インデックス `(name_normalized, revision_id)` |
| `voice_room_host_changes` | イベント | `id`, `room_id`, `host_account_id`, `reason`（`enum(opened, handed_over, host_disconnected, host_ineligible, host_left, host_absent)`）, `changed_at` | インデックス `(room_id, id DESC)`, `(host_account_id, room_id)` |
| `voice_room_cohost_changes` | イベント | `id`, `room_id`, `account_id`, `change`（`enum(assigned, removed)`）, `changed_at` | インデックス `(room_id, account_id, id DESC)` |
| `voice_room_participations` | リソース | `id`, `room_id`, `account_id`, `joined_at` | インデックス `(room_id, id)`, `(account_id, id DESC)` |
| `voice_room_participation_ends` | イベント | `participation_id`（PK）, `reason`（`enum(left, moved, disconnected, removed, blocked, suspended, room_ended)`）, `ended_at` | 参加ごとに 1 つ |
| `voice_room_role_changes` | イベント | `id`, `participation_id`, `role`（`enum(speaker, listener)`）, `changed_by_account_id`, `changed_at` | インデックス `(participation_id, id DESC)` |
| `voice_room_speak_grants` | イベント | `id`, `room_id`, `account_id`, `change`（`enum(granted, revoked)`）, `changed_by_account_id`, `changed_at` | インデックス `(room_id, account_id, id DESC)`。話者としての招待も `granted` を書く |
| `voice_room_hand_raises` | イベント | `id`, `participation_id`, `change`（`enum(raised, lowered, granted, dismissed)`）, `changed_at` | インデックス `(participation_id, id DESC)` |
| `voice_room_invitations` | リソース | `id`, `room_id`, `invitee_account_id`, `role`（`enum(speaker, listener)`）, `invited_by_account_id`, `invited_at` | 一意 `(room_id, invitee_account_id)` |
| `voice_room_invitation_revocations` | イベント | `invitation_id`（PK）, `revoked_by_account_id`, `revoked_at` | 招待ごとに 1 つ |
| `voice_room_bans` | イベント | `id`, `room_id`, `account_id`, `banned_by_account_id`, `banned_at` | 一意 `(room_id, account_id)`。退出させる操作が書く |
| `voice_room_endings` | イベント | `room_id`（PK）, `reason`（`enum(host, empty, host_disconnected, host_ineligible, moderator, time_limit, disabled, host_absent, unattended)`）, `ended_by_account_id`（NULL 可 = 自動の終了）, `ended_at`, `note`（`text`。500 文字まで。モデレーターが終えた理由の文。なしは `''`） | ルームごとに 1 つ。インデックス `(ended_at, room_id)`（記録の消去） |
| `voice_room_heads` | 投影 | `room_id`（PK）, `revision_id`, `host_account_id`, `state`（`enum(open, ended)`）, `participants_count`（`int`） | インデックス `(state, room_id)`。一覧はこれを引く |
| `voice_room_participants` | 投影 | `account_id`（PK。1 人 1 ルーム）, `room_id`, `participation_id`, `role`, `cohost_change_id`（NULL 可 = 共同ホストでない）, `hand_raised`（`bool`） | 一意 `(participation_id)`。インデックス `(room_id, participation_id)` |
| `voice_room_host_graces` | 現在値 | `room_id`（PK）, `ends_at`（猶予の終わり。決定の内容としての期限） | ホストの猶予の間だけ行がある。予定で開いたルームでは、開いたときから待機の終わりまで（ホストが参加すると消える） |
| `voice_room_schedules` | リソース | `room_id`（PK）, `schedule_key`（`string(200)`。呼び出す側の予定のキー）, `wait_ends_at`（待機の終わり。決定の内容としての期限） | 一意 `(schedule_key)`。予定で開いたルームだけの行（ADR-0100 §16） |
| `voice_room_cohost_candidates` | リソース | `room_id`, `account_id`, `position`（`int`。0〜2。指名の順） | PK `(room_id, account_id)`。予定で開いたルームの共同ホストの候補 |

- コマンドはイベントの行を書いてから、同じトランザクションでドメインイベント（`voice.room_opened`・`voice.room_revised`・`voice.host_changed`・`voice.cohost_changed`・`voice.participation_started`・`voice.participation_ended`・`voice.role_changed`・`voice.hand_raise_changed`・`voice.room_ended`）を発行します。
  `voice` の projector は、そのイベントが触れるルームの `voice_room_heads` の行と、アカウントの `voice_room_participants` の行を、再構築と同じ規則の SQL で作り直します（同期）。
  投影に効かない変化（発話の許可と取り消し・招待・招待の取り消し・禁止）も、行を書いてから `voice.speak_grant_changed`・`voice.invited`・`voice.invitation_revoked`・`voice.banned` を発行します。これらを読む projector はありません（他のモジュールの購読のため）。
- `voice_room_heads` は、ルームの最新の版（`id` の最も大きい行）と最新のホストの交代、終了があれば `ended`、開いたルームなら終わっていない参加の数です。終わったルームの数は 0 です。
- `voice_room_participants` は、終わっていないルームの、終わっていない参加ごとの行です（`voice_room_endings` のあるルームの行は持ちません）。役割は参加の最新の役割の変化（なければ `listener`）、挙手はその参加の最新の挙手の変化が `raised` かどうかです。
  共同ホストは、その参加が始まった後（`id` がその参加より大きい）の、そのルームでのそのアカウントの最新の共同ホストの変化が `assigned` で、そのアカウントがいまのホスト（最新の `voice_room_host_changes`）でないとき、その行の `id` です（指名の順はこの `id` の順）。
  共同ホストであることは参加が終わると外れ、戻った人は聞き手で、ホストが指名し直します（[決定者の回答](https://github.com/mycroblog/my/pull/1372#issuecomment-6407284230)、#1345 の回答 44）。ハートビートの期限（45 秒）の内に繋ぎ直した人は参加が続くので、共同ホストのままです。
  共同ホストがホストになると共同ホストではなくなり、共同ホストの上限（3 人）にも、自動でホストを継ぐ候補（`NextHost`）にも数えません。ホストの交代の projector は、そのルームの参加者の行をすべて作り直します。
  主キーがアカウントなので、終わっていない参加を持つアカウント（別のルームでも、同時に走った同じルームへの参加でも）の参加は projector が `voice.ErrInAnotherRoom` で拒み、トランザクションは何も書きません。別のルームへ移るコマンドは、前の参加を `moved` で終えてから参加します（ADR-0100 §5）。
- 参加の終わり・招待・取り消し・禁止・ルームの終了は、一意制約と衝突で何もしない挿入で 1 回だけ書きます（同時に走った評価も 1 回だけ効く。ADR-0100 §6）。発話の許可は、そのルームでのそのアカウントの最新の `voice_room_speak_grants` が `granted` かどうかです。
- 一覧（`OpenRooms`）は `voice_room_heads` の開いたルームを参加者の多い順に引き、ハッシュタグで絞るときは今の版のタグ（`name_normalized`）を見ます。閲覧者ごとの公開範囲は `internal/domain/voice` の `CanSee` でアプリが絞ります（ADR-0100 §4）。
- 接続の生存（ハートビート）は DB に書かず、`KeyValueStore` に置きます（ADR-0100 §6・§12）。
- モデレーターによる終了（`voice.Service.EndByModerator`、`manage_reports`。ADR-0100 §11）は、ルームを止める操作と同じトランザクションで、理由 `moderator`・終えたモデレーターのアカウント・理由の文（`note`）の `voice_room_endings` を書き、監査ログ `voice_room_ended`（§4.3）を発行します。
  通話を `off` にしていても終えられます。終わったルームと知らないルームは `ErrNotFound` です。
- 参加者の記録の消去（ADR-0100 §13、§8）は、1 時間ごとのジョブ `voice.purge_records` が、`voice_room_endings.ended_at` から `voice_participants_retention_days`（§4.9。回ごとに読み直す）を過ぎたルームの `voice_room_participations`・`voice_room_participation_ends`・`voice_room_role_changes`・`voice_room_hand_raises`・`voice_room_speak_grants`・`voice_room_invitations`・`voice_room_invitation_revocations`・`voice_room_bans` を、ルームを `LockRoom` してから 1 トランザクションに 100 ルームずつ、1 回に 20 バッチまで、ルームの ID 順に消します。回は前の回が止まったルームの続き（`KeyValueStore` のカーソル `voice.purge`。失えば最初から）から始め、最後まで行けば最初に戻るので、残すルームが多くてもほかのルームが後回しのままにはなりません。
  通報が指すルーム（`moderation.Service.ReportedVoiceRooms`。通報は保持期間を過ぎると `report_voice_rooms` ごと消える）と、記録が保全（`retention_holds`）のあるアカウントを参加者か操作した人として名指すルームは残し、後の回が調べ直します。どちらもルームを `LockRoom` した後にトランザクションの中でもう一度確かめます。
- **予定で開くルーム**（ADR-0100 §16）: `voice.Service.OpenScheduled` は、ルーム・最初の版・ホストの交代（`opened`）と、`voice_room_schedules`・待機の終わりを期限とする `voice_room_host_graces`・`voice_room_cohost_candidates` を 1 トランザクションで書きます。参加・開始の投稿・`voice_room_started` は作らず、`voice.apply` の `open` は最初の参加のトランザクションが積みます（冪等キーは `Open` と同じ `open`）。同じ `schedule_key` の 2 度目の呼び出しは、`LockOpening` の後に読み直して、開いたルームを返します。
  ホストが参加の中にいなくて、`voice_room_schedules` と猶予の行があるルームは待機中です。待機中は、最初の参加者が来て全員が去るまで `empty` で終えず（「来たか」は `voice_room_participations` の有無で判定）、待機の終わりに参加している資格のある共同ホスト（指名の順）へ `host_absent` でホストを移すか、参加者がいれば `host_absent`、来て全員去っていれば `empty`、誰も来なければ `unattended` で終えます。照合（`Sessions`）は、参加の記録のないルームでは呼びません。
  候補は参加のトランザクションで、共同ホストが 3 人に満たず、そのルームでそのアカウントの `removed` の共同ホストの変化がなければ、`assigned` の変化を書きます。呼び出す側が写す招待はジョブ `voice.invite_scheduled`（500 人ずつ、招待した人はその時のホスト）が、通知と §7 の件数の制限なしで、`(room_id, invitee_account_id)` の一意で 1 回だけ書きます。
- アカウントの消去（§8）は、そのアカウントが書いた版の `name` と `description` を `''` にします（`voice.Eraser`。ルームを `LockRoom` してから、1 トランザクションに 100 ルームまで）。ルーム・版のほかの列・タグ・ホストの交代・終了は残します。
