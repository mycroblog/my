### 4.9 instance

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `instance_setting_revisions` | イベント（版） | `id`, `key`（`enum(registration_mode, max_status_length, default_locale, local_timeline_access, federated_timeline_access, preview_image, contact_account, contact_email, thumbnail, remote_media_policy, remote_media_cache_ttl, authorized_fetch, domain_actions_visibility, domain_actions_rationale_visibility, appeal_window, retention_session_origins, retention_registration_origins, retention_reports, retention_moderation_actions, retention_audit_log, search_indexing, ai_usage, indexable_default, security_contact, require_media_description, link_cards_enabled, finance_visibility, finance_viewer_item_names, finance_viewer_item_memos, finance_viewer_income, supporter_list_visibility, income_visibility, federation_mode, oauth_metadata_document_clients, dynamic_client_registration, user_app_registration, voice_room_starters, voice_public_summary, retention_voice_participants, invite_creators, member_invite_max_active, member_invite_max_uses, member_invite_max_expires_in)`）, `value`（スカラー値の正規形の文字列表現。型はキーごとにコードで定義）, `revised_by_user_id`（NULL 可 = CLI による変更）, `revised_at` | インデックス `(key, id DESC)` |
| `instance_settings` | 投影 | `key`（PK）, `revision_id`, `value` | |
| `instance_text_revisions` | イベント（版） | `id`, `key`（`enum(name, short_description, description, terms, privacy_policy, preview_image_alt, thumbnail_alt)`）, `body`（空 = 文章を消す）, `revised_by_user_id`（NULL 可 = CLI による変更）, `revised_at` | インデックス `(key, id DESC)`。文章はキーごとに 1 つの値で、どのロケールでも同じ値を表示する（ADR-0009） |
| `instance_texts` | 投影 | `key`（PK）, `revision_id` | |
| `instance_rules` | リソース | `id`, `created_at` | ルールの識別子。通報が引用する（`report_rule_citations`） |
| `instance_rule_revisions` | イベント（版） | `id`, `rule_id`, `position`（`int`）, `text`（`string(300)`。1 行）, `hint`（`string(1000)`。空 = 補足なし）, `revised_by_user_id`（NULL 可 = CLI による変更）, `revised_at` | インデックス `(rule_id, id DESC)`。文と補足は 1 つずつで、どのロケールでも同じ値を表示する |
| `instance_rule_retirements` | イベント | `rule_id`（PK）, `retired_by_user_id`（NULL 可 = CLI による廃止）, `retired_at` | 廃止したルールも通報からは参照できる |
| `instance_rule_heads` | 投影 | `rule_id`（PK）, `revision_id`, `position` | 有効なルールだけ。インデックス `(position)` |
| `instance_theme_revisions` | イベント（版） | `id`, `custom_css`（`string`。空 = なし。64 KiB まで）, `revised_by_user_id`（NULL 可 = CLI による変更）, `revised_at` | トークンもカスタム CSS も無い版はテーマを既定に戻す |
| `instance_theme_tokens` | イベント（版の子） | `theme_revision_id`, `token`（`enum`。上書きできるトークンの生成した一覧）, `scheme`（`enum(light, dark, all)`。色はライトとダーク、ほかは `all`）, `value` | PK `(theme_revision_id, token, scheme)` |
| `instance_theme` | 投影 | `id`（PK。常に 1）, `revision_id` | テーマを保存したことがあれば 1 行 |
| `announcements` | リソース | `id`, `created_at` | サーバーのローカルの利用者全員へのお知らせ（FR-726） |
| `announcement_revisions` | イベント（版） | `id`, `announcement_id`, `body`（`string(5000)`。Markdown）, `starts_at`（NULL 可 = 公開した時から）, `ends_at`（NULL 可 = 取り下げるまで）, `revised_by_user_id`（NULL 可 = CLI による変更）, `revised_at` | インデックス `(announcement_id, id DESC)`。CHECK `ends_at > starts_at`（両方あるとき） |
| `announcement_state_changes` | イベント | `id`, `announcement_id`, `state`（`enum(published, withdrawn)`）, `changed_by_user_id`（NULL 可 = CLI による変更）, `changed_at` | インデックス `(announcement_id, id DESC)` |
| `announcement_heads` | 投影 | `announcement_id`（PK）, `revision_id`, `state`（`enum(draft, published, withdrawn)`）, `published_at`（最後に公開した時刻） | CHECK `(state = 'draft') = (published_at IS NULL)`。インデックス `(state)` |
| `announcement_reads` | 現在値 | `user_id`, `announcement_id`, `read_at` | PK `(user_id, announcement_id)`。既読の履歴には意味がない（ADR-0020 規約 5） |
| `service_closure_events` | イベント | `id`, `kind`（`enum(announced, withdrawn, closed, delivery_completed)`）, `by_user_id`（NULL 可 = CLI とサーバー自身による変更）, `occurred_at` | サービスの終了（FR-808、[ADR-0088](../../../adr/0088-service-shutdown-modes.md)）の状態の変化 |
| `service_closure_schedules` | イベント（`announced` の部分型） | `event_id`（PK）, `closes_at`, `export_until`, `message`（`string(5000)`。空 = なし） | CHECK `export_until >= closes_at` |
| `service_closure_heads` | 投影 | `id`（PK。常に 1）, `state`（`enum(announced, closed, delivery_completed)`）, `event_id`（最後のイベント）, `announcement_event_id`（効いている予定の `announced`） | 終了を予告している間とその後に 1 行。行が無い = 予告なし |

- 設定値は 1 キー 1 スカラーにします。複合的な設定はキーを分けます（JSON にしない）。
  キー・型・既定値・検証は `internal/domain/instance` に定義し、`key` の `CHECK` と一緒に増やします。一度も設定していないキーは既定値として読みます。
- 現在値の投影は、キーごとに `id` が最大の版を指します。
- メディアを指す設定（`preview_image`）の値は `media_files.id` の論理参照で、空文字は未設定です。
  どれかの版（現在値でなくても）が指すメディアは、`content` モジュールの未添付アップロードの消去（§4.4）から外れます。
- 文章は管理者が好きな言語で書く 1 つの値で、読み手のロケールによらず同じ値を返します。空の文章は無いものとして扱います。
  書かれた言語は記録しないので、表示する側は `dir="auto"` と `lang=""`（言語が不明）で出します。
- `terms`（利用規約）と `privacy_policy`（プライバシーポリシー）は Markdown の文書で、ほかの文章はプレーンテキストです。
  文書は版に Markdown のまま記録し、文書の API（`GET /instance/documents/{kind}`）が読むたびに HTML にしてサニタイズします（`instance.DocumentRenderer`）。HTML は保存しません。
  文書が無ければ、文書の API は 404 で、登録の画面・各画面のフッター・`GET /api/v1/instance` の `terms_url` / `privacy_policy_url` もそれを指しません。
- 環境変数・設定ファイルによる設定（ADR-0010）とインスタンス設定は、同じキーを持ちません。
  再起動で反映するデプロイの構成（アダプタ・接続先・ホスト名・秘密情報）は設定に、運営者が稼働中に変える方針と文章はこの表に置きます。
  どちらかがもう一方を上書きすることはありません（[設定リファレンス](../../../reference/configuration.md)、[運用手順](../../../operations/instance-settings.md)）。
- 新しい設定は、それを使う機能と一緒にキーを追加します。
- ルールの文の改訂と並べ替えは、どちらも位置と文と補足を写した新しい版を記録します。`rule_id` は変わらないので、通報の引用（`report_rule_citations`）はそのまま残ります。
  - 新しいルールは有効なルールの最後（最大の `position` + 1）に置きます。有効なルールは 100 個までです。
  - 並べ替えは、移すルールを有効なルールの中の指定の位置（1 から）に入れて 1, 2, ... と振り直し、`position` が変わるルールにだけ版を記録します。
    廃止で空いた番号は、次の並べ替えで詰まります。同じ `position` のルールは `rule_id` の順に並べます（PostgreSQL の READ COMMITTED で同時に追加した場合）。
- `instance_rule_heads` は、廃止していないルールの `id` が最大の版を指します。廃止すると行が消え、廃止は取り消せません。
- ルールの文と補足は、文章と同じく読み手のロケールによらず同じ値を返します。
- テーマ（[ADR-0059](../../../adr/0059-design-tokens-and-instance-theme.md)）は版ごとにテーマ全体（上書きしたトークンとカスタム CSS）を記録し、`instance_theme` が `id` の最大の版を指します。
  `instance_theme_tokens.token` の `CHECK` は、`app/ui/src/theme.css` から生成したトークンの一覧（`internal/domain/instance/theme_tokens.gen.go`）と同じで、`schema` のテストが食い違いを検出します。
  値は保存の前にドメインが種類ごとの形・範囲と色の組のコントラストを検査し、検査済みの値だけからスタイルシートを組み立てます。版の内容は変わらないので、過去の版のスタイルシートの URL も同じ内容を返し続けます。
- お知らせ（FR-726）は、`manage_announcements` を持つ人が下書きとして作り、公開し、取り下げます。本文と表示の期間（`starts_at`・`ends_at`）を変えるたびに版を記録し、`announcement_heads` が `id` の最大の版と、状態の変化の最後の結果を指します。
  - 取り下げは取り消せません。取り下げたお知らせは変更も再公開もできず、新しいお知らせを作り直します。下書きのまま取り下げることもでき、そのときの `published_at` は取り下げた時刻です。
  - 利用者に見えるのは、公開中で今が期間の中にあるお知らせです。期間の始まりと公開の遅いほう（`ShowsFrom`）の新しい順に並べます。
  - 本文は文書と同じく版に Markdown のまま記録し、読むたびに HTML にしてサニタイズします（`instance.DocumentRenderer`、ADR-0019）。
  - 既読は `announcement_reads` の行の有無で表し、アカウントの消去（§8）で消えます。
- サービスの終了（FR-808、[ADR-0088](../../../adr/0088-service-shutdown-modes.md) §1）は、`manage_service_closure` を持つ人が管理 API・CLI（`my admin closure`）から予告し、日付を変え、取り消します。
  - 遷移は `internal/domain/instance` の `Closure.Check` が検査します。`announced` は予告が無いときと予告中（日付の変更）、`withdrawn` は予告中だけ、`closed` は予告中で `closes_at` を過ぎたときだけ、`delivery_completed` はクローズの後だけです。`closed` の後に戻る遷移はありません。
  - 日付の変更は新しい `announced` で表し、`service_closure_heads.announcement_event_id` がそれを指します。取り消すと行が消え、その後の `announced` は新しい予告です。
  - `closes_at` に下限はありません。`export_until` は `closes_at` 以後で、指定が無ければ `closes_at` の 30 日後です。予告から `closes_at` までが 30 日より短いとき、管理 API と CLI は告知の期間が短いと警告します。
  - 状態はリクエストとジョブごとに読み、各入口は `instance.ClosureAllows(state, 操作)` で判定します。遷移はすべて監査ログ（`target_kind = instance`）に残ります。
  - `delivery_completed` は、クローズの `Delete`（Actor）の配送がすべて終わったことを `federation` の定期ジョブが確かめて、サーバー自身の変更（`by_user_id` は NULL）として記録します（[federation.md §15](../../federation.md#15-サービスの終了)）。
