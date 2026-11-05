### 4.16 hook

判定フックが外部プラグインから受け取った判定（[ADR-0147](../../../adr/0147-decision-hooks.md) §6）。`internal/app/hook` が所有します。投稿の本文・メールアドレス・IP アドレスを持ちません。

| テーブル | 分類 | 主な属性 | 備考 |
| --- | --- | --- | --- |
| `hook_decisions` | イベント | `id`, `hook`（`enum(inbound.status, inbound.follow, signup.check, media.check, local.status)`）, `subject_key`, `account_id?`, `mode`（`enum(observe, enforce)`）, `outcome`（`enum(allow, drop, flag, mark_sensitive, require_approval, reject, timeout, error, invalid, breaker_open)`）, `reason_code`, `note`, `latency_ms`, `decided_at` | `(hook, subject_key)` で一意で、ジョブの再実行は行を読みプラグインを呼ばない。`subject_key` は `inbound.*` では受信したアクティビティの URI。`account_id` は対象のアカウントで、保全とアカウントの消去の基準になる（なければ NULL）。`note` はプラグインのメモ（平文 200 文字まで） |
| `hook_decision_statuses` | イベント（子） | `decision_id`, `status_id` | 判定の効果が及んだ投稿。`status_sensitivity_overrides` の再構築の元になる（ADR-0147 §8）。投稿の ID が決まるトランザクションで書く |
| `hook_decision_overrides` | イベント | `decision_id`, `overridden_by_user_id?`, `overridden_at` | モデレーターの取り消し。判定につき 1 行（ADR-0147 §9）。監査ログの `hook_decision_overridden` に対応する |

保持: 定期ジョブ `hook.purge_decisions` が、`retention_hook_decisions`（既定 90 日、下限 30 日）を過ぎた判定を子の行ごと消します。次は残します（ADR-0147 §6）。

- `enforce` の `mark_sensitive` で取り消しのない判定のうち、`statuses` に行のある投稿を指すもの
- `retention_holds` が保全するアカウントの判定（`moderation.HeldAccountIDs`）
- `enforce` の `local.status` の `reject` のうち、`appeal_window_days` の内のもの

アカウントの消去（`account.erase_deleted`）は、そのアカウントの判定を消します。消去は保全中のアカウントを飛ばします。
