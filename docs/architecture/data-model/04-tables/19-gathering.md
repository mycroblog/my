### 4.19 gathering

同じサーバーのローカルの利用者のイベント（FR-940〜FR-945、[ADR-0161](../../../adr/0161-events.md) §13）。`internal/app/gathering` が所有し、表はすべて `gathering` だけが書きます。
ADR-0020 の「イベント」（起きた事実の表）と取り違えないよう、表とコードの名前は `gathering` です。状態を NULL 許容の日時のカラムで表さず、事実はイベントの行の有無で表します。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `gatherings` | リソース | `id`, `organizer_account_id`（主催者。変わらない）, `created_at` | インデックス `(organizer_account_id, id)` |
| `gathering_revisions` | イベント（版） | `id`, `gathering_id`, `title`（書記素で 1〜100。コード点で 1000 まで。書いたアカウントの消去で `''`）, `description`（`text`。3,000 文字まで。なしは `''`。書いたアカウントの消去で `''`）, `visibility`（`enum(public, followers, following, mutuals, invited)`）, `time_zone`（IANA の名前。`time.LoadLocation` で検証）, `place_disclosure`（`enum(approved, viewers)`）, `capacity`（`int`。0 = なし、1〜10,000）, `revised_by_account_id`, `revised_at` | インデックス `(gathering_id, id DESC)`, `(revised_by_account_id, id)`（アカウントの消去）。作ったときの設定が最初の版 |
| `gathering_revision_places` | リソース（版の子） | `revision_id`, `position`（`int`。0〜2）, `kind`（`enum(voice, physical, url)`）, `name`（`string(100)`）, `address`（`text`。500 文字まで）, `url`（`string(2000)`。`http` / `https` だけ。サーバーは取りに行かない）, `speakers`（`enum('', everyone, followers, mutuals, manual)`。`voice` のときだけ） | PK `(revision_id, position)`。一意 `(revision_id, kind)`（各種類 1 つまで）。版を書いたアカウントの消去で行を消す |
| `gathering_schedule_confirmations` | イベント | `id`, `gathering_id`, `starts_at`, `ends_at`（UTC）, `confirmed_by_account_id`, `confirmed_at` | `ends_at > starts_at`。インデックス `(gathering_id, id DESC)`。今の日時は最新の行。日時の変更は新しい行 |
| `gathering_invitations` | リソース | `id`, `gathering_id`, `invitee_account_id`, `invited_by_account_id`, `invited_at` | 一意 `(gathering_id, invitee_account_id)`（取り消しの後も招待し直さない）。インデックス `(invitee_account_id, gathering_id)` |
| `gathering_invitation_revocations` | イベント | `invitation_id`（PK）, `revoked_by_account_id`, `revoked_at` | 招待ごとに 1 つ |
| `gathering_rsvps` | イベント | `id`, `gathering_id`, `account_id`, `answer`（`enum(going, maybe, declined)`）, `answered_at` | インデックス `(gathering_id, account_id, id DESC)`。今の答えは最新の行 |
| `gathering_rsvp_approvals` | イベント | `id`, `gathering_id`, `account_id`, `change`（`enum(approved, revoked)`）, `changed_by_account_id`, `changed_at` | インデックス `(gathering_id, account_id, id DESC)` |
| `gathering_coorganizer_changes` | イベント | `id`, `gathering_id`, `account_id`, `change`（`enum(assigned, removed)`）, `changed_at` | インデックス `(gathering_id, account_id, id DESC)`。主催者だけが書く |
| `gathering_cancellations` | イベント | `gathering_id`（PK）, `reason`（`enum(organizer, moderator, organizer_suspended, organizer_deleted)`）, `cancelled_by_account_id`（NULL 可 = 自動の中止）, `note`（`text`。500 文字まで。なしは `''`）, `cancelled_at` | イベントごとに 1 つ |
| `gathering_heads` | 投影 | `gathering_id`（PK）, `organizer_account_id`, `revision_id`, `confirmation_id`, `state`（`enum(scheduled, cancelled)`）, `starts_at`, `ends_at`, `going_count`（`int`） | インデックス `(state, starts_at, gathering_id)`。一覧はこれを引く |
| `gathering_attendees` | 投影 | `gathering_id`, `account_id`, `rsvp_id`（今の答えの行）, `answer`, `approved`（`bool`） | PK `(gathering_id, account_id)`。インデックス `(gathering_id, rsvp_id)`（出欠の一覧の順）, `(account_id, gathering_id)`（リマインダーの変更が読む） |
| `gathering_coorganizers` | 投影 | `gathering_id`, `account_id`, `change_id`（指名の行。指名の順） | PK `(gathering_id, account_id)`。インデックス `(account_id, gathering_id)` |
| `gathering_dues` | 投影 | `gathering_id`（PK）, `next_due_at`（次に `gathering.tick` が扱う時刻） | インデックス `(next_due_at, gathering_id)`。先にリマインダーか開始が残るイベントだけの行 |
| `gathering_reminder_settings` | 現在値 | `account_id`（PK）, `updated_at` | 行があればそのアカウントは選んだ（`gathering_reminder_offsets` の行がなければリマインダーなし）。行がなければ既定（1 日前と 1 時間前） |
| `gathering_reminder_offsets` | 現在値（`gathering_reminder_settings` の子） | `account_id`, `minutes_before`（`int`。5〜10080） | PK `(account_id, minutes_before)`。アカウントごとに 3 つまで（コマンドが検査する） |

- コマンドはイベントの行を書いてから、同じトランザクションでドメインイベント（`gathering.created`・`gathering.revised`・`gathering.rescheduled`・`gathering.answered`・`gathering.approval_changed`・`gathering.coorganizer_changed`・`gathering.cancelled`）を発行します。
  `gathering` の projector は、そのイベントが触れるイベントの `gathering_heads` の行、アカウントの `gathering_attendees` の行、イベントの `gathering_coorganizers` の行を、再構築と同じ規則の SQL で作り直します（同期）。
  招待と取り消しも行を書いてから `gathering.invited`・`gathering.invitation_revoked` を発行します。これらを読む projector はありません（通知のため）。
- `gathering_heads` は、最新の版と最新の日時の確定、中止があれば `cancelled`、`gathering_attendees` の `going` の数です。終わったイベントは `scheduled` のままで、終わったことは `ends_at` で分かります。一覧（`UpcomingGatherings`）は `state = scheduled` で `ends_at` が今より後の行を `(starts_at, gathering_id)` の順に引き、閲覧者ごとの公開範囲は `internal/domain/gathering` の `CanSee` でアプリが絞ります（§5.7）。
- `gathering_attendees` は、アカウントの最新の `gathering_rsvps` の行です。`approved` は、答えが `declined` でなく、そのアカウントの最新の `gathering_rsvp_approvals` が `approved` で、その承認より後に `declined` の答えがないときに真です（「行かない」に変えると承認は外れ、答え直しても戻らない）。
- `gathering_coorganizers` は、そのアカウントの最新の `gathering_coorganizer_changes` が `assigned` の行です（最大 3 人。コマンドが検査する）。
- 定員の検査は、イベントを `LockGathering`（PostgreSQL はイベントの勧告ロック、SQLite は書き込みが 1 つずつなので何もしない）でロックしてから `gathering_heads.going_count` を読み、`gathering_rsvps` を書きます。招待・取り消し・中止は一意制約と衝突で何もしない挿入で 1 回だけ書きます。
- `gathering_dues` は、確定（作成と日時の変更）では今から、出欠とリマインダーの変更では期限が来ている行を残して（次の `gathering.tick` がそこからの分を扱う）、「行く」「たぶん」の人のリマインダー（選んだ分と、選んでいない人がいれば既定）と開始から次の時刻を求め直し、中止で消します（同じトランザクション）。
  `gathering.tick`（1 分ごと）は `next_due_at` が今以前の行だけを読み、イベントをロックして、期限から今までに来たリマインダーと開始の `gathering.notify` のジョブを outbox で積み、次の時刻を書きます（なければ行を消す）。`next_due_at` は状態ではなく、確定・出欠・リマインダーの設定・中止から求め直せる値です（[ADR-0161](../../../adr/0161-events.md) §12）。
- リマインダーの設定は「選んだか」を `gathering_reminder_settings` の行の有無で、選んだ時間を `gathering_reminder_offsets` の行で表し、0 個（リマインダーなし）を既定と区別します。NULL も JSON も使いません。
- アカウントの消去（§8）は、そのアカウントが書いた `gathering_revisions` の `title` と `description` を `''` にし、その版の `gathering_revision_places` を消し、そのアカウントの `gathering_reminder_settings` と `gathering_reminder_offsets` を消します。イベント・日時・招待・出欠・承認・中止の行は参照だけで、残ります。
