### 4.16 hook

判定フックが外部プラグインから受け取った判定（[ADR-0147](../../../adr/0147-decision-hooks.md) §6）。`internal/app/hook` が所有します。判定は投稿の本文・メールアドレス・IP アドレスを持ちません。本文を持つのは、作者が送る異議申し立てだけです。

| テーブル | 分類 | 主な属性 | 備考 |
| --- | --- | --- | --- |
| `hook_decisions` | イベント | `id`, `hook`（`enum(inbound.status, inbound.follow, signup.check, media.check, local.status)`）, `subject_key`, `account_id?`, `mode`（`enum(observe, enforce)`）, `outcome`（`enum(allow, drop, flag, mark_sensitive, require_approval, reject, timeout, error, invalid, breaker_open)`）, `reason_code`, `note`, `latency_ms`, `decided_at` | `(hook, subject_key)` で一意で、ジョブの再実行は行を読みプラグインを呼ばない。`subject_key` は `inbound.*` では受信したアクティビティの URI。`account_id` は対象のアカウントで、保全とアカウントの消去の基準になる（なければ NULL）。`note` はプラグインのメモ（平文 200 文字まで） |
| `hook_decision_statuses` | イベント（子） | `decision_id`, `status_id` | 判定の効果が及んだ投稿。`status_sensitivity_overrides` の再構築の元になる（ADR-0147 §8）。投稿の ID が決まるトランザクションで書く |
| `hook_decision_overrides` | イベント | `decision_id`, `overridden_by_user_id?`, `overridden_at` | モデレーターの取り消し。判定につき 1 行（ADR-0147 §9）。監査ログの `hook_decision_overridden` に対応する |
| `hook_decision_appeals` | リソース | `id`, `decision_id`, `account_id`（作者 = 判定のアカウント）, `status_text`（投稿しようとした本文。1〜100,000 文字。空白だけは不可）, `comment`（モデレーターへの説明。2,000 文字まで。なければ空）, `submitted_at` | 一意 `(decision_id)`。`enforce` の `local.status` の `reject` で取り消しのないものだけに、判定の `decided_at` から `appeal_window_days` の間（期限ちょうどまで）申し立てられる（ADR-0147 §9） |
| `hook_decision_appeal_decisions` | イベント | `appeal_id`（PK）, `decision`（`enum(upheld, overturned)`）, `decided_by_user_id?`（NULL = CLI）, `decided_at` | 審査は 1 回。同じ結果の再審査は何もせず、反対の結果は拒否する。`overturned` は同じトランザクションで `hook_decision_overrides` と `hook_account_exemptions` を書く。審査はプラグインを呼ばない |
| `hook_account_exemptions` | 現在値 | `hook`（`enum(local.status)`）, `account_id`, `exempted_at` | PK `(hook, account_id)`。行の存在が現在値で（規約 5）、判定や申し立てから導かない。行のあるアカウントはそのフックに送らない |

保持: 定期ジョブ `hook.purge_decisions` が、`hook_decisions_retention_days`（既定 90 日、下限 30 日）を過ぎた判定を子の行ごと消します。次は残します（ADR-0147 §6）。

- `enforce` の `mark_sensitive` で取り消しのない判定のうち、`statuses` に行のある投稿を指すもの
- `retention_holds` が保全するアカウントの判定（`moderation.HeldAccountIDs`）
- `enforce` の `local.status` の `reject` で取り消しのないもののうち、`appeal_window_days` の内のものと、審査待ちの申し立てのあるもの

判定を消すときは、その申し立てと審査も一緒に消します。

アカウントの消去（`account.erase_deleted`）は、そのアカウントの判定・申し立て・`hook_account_exemptions` の行を消します。消去は保全中のアカウントを飛ばします。

`local.status` の `subject_key` は、作成がトランザクションの前に生成する投稿の ID です。編集では `<投稿の ID>#<新しい版の ID>` で、編集ごとに別の判定になります（ADR-0147 §2）。編集の `reject` は版を保存せず、申し立てられません。`flag` はその投稿を `hook_decision_statuses` に結び、通報を起票します。`reject` では投稿の行ができず、作者は申し立てが覆った後に投稿し直します。

`signup.check` の `subject_key` と `account_id` は、登録がトランザクションの前に生成するアカウントの ID です。メールアドレスも IP アドレスも含みません。`reject` や、後の検証で失敗した登録ではアカウントの行ができないので、その判定は保持期間で消えます。

`media.check` の `subject_key` はメディアファイルの ID、`account_id` はアップロードしたアカウントです。判定は初回の処理（`media.process`）で 1 度だけ作り、別の用途への再処理と、添付する投稿の作成・編集は `Gate.Recorded` で記録を読むだけでプラグインを呼びません。`mark_sensitive` と添付の `flag` は、添付する投稿を書くトランザクションで `status_sensitivity_overrides`・`hook_decision_statuses`・通報を書きます。`reject` の判定はメディアの処理結果を `failed` にし、原本も派生も残りません。
