# ADR-0058: モデレーションと管理の分離・監査・保持

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #648, #327

## Decision（決定）

運用者の機能を、Web は経路とナビゲーションでモデレーションとサーバー管理に分け、API は `/admin/*` のままタグで分け、Go はデータを持つモジュールに置く。処分は連合先へ伝え（§6）、操作は追記専用の監査ログに残し（§7）、関わる個人データは種類ごとの期間で消す（§8〜§10）。

### 1. 境界

- **モデレーション**は、人・投稿・リモートのサーバーについての判断と処分である。誰が参加でき、何が見えるかを個別の事案ごとに決める。
- **サーバー管理**は、サーバー自身の設定・サーバーが出す中身・運用・外部との連携である。サーバー全体に一度に効く。

| 区分 | 画面（Web の経路） | 権限 |
| --- | --- | --- |
| モデレーション | 通報（`/moderation/reports`） | `manage_reports` |
| モデレーション | 異議申し立て（`/moderation/appeals`） | `manage_accounts` |
| モデレーション | アカウントの検索と管理画面・処分（`/moderation/accounts`） | `manage_accounts` |
| モデレーション | 登録の審査（`/moderation/registrations`） | `manage_accounts` |
| モデレーション | 登録の拒否リスト（`/moderation/signup-blocks`、#626 の BAN からの作成を含む） | `manage_accounts` |
| モデレーション | ドメインへの処分（`/moderation/domain-actions`、#641 のインポートとエクスポートを含む） | `manage_domains` |
| モデレーション | 連合先（`/moderation/instances`。サーバーごとの配送の状態と再開、そのドメインへの処分への入口） | `manage_domains` |
| モデレーション | 招待（`/moderation/invites`、#624） | `manage_invites` |
| モデレーション | 監査ログ（`/moderation/audit-log`） | `view_audit` |
| サーバー管理 | サーバーの設定（`/admin/settings`。名前・説明・登録モード・既定の言語・タイムライン・連合の設定・処分したドメインの公開、#624） | `manage_settings` |
| サーバー管理 | 規約とポリシー（`/admin/documents`） | `manage_settings` |
| サーバー管理 | ルール（`/admin/rules`、#624） | `manage_rules` |
| サーバー管理 | ロール（`/admin/roles`。ロールと権限の一覧） | `manage_roles` |
| サーバー管理 | テーマ（`/admin/theme`、ADR-0059） | `manage_settings` |
| サーバー管理 | お知らせ（`/admin/announcements`、#665） | `manage_announcements` |
| サーバー管理 | カスタム絵文字（`/admin/custom-emojis`、ADR-0051） | `manage_custom_emojis` |
| サーバー管理 | webhook（`/admin/webhooks`、[ADR-0060](0060-instance-webhooks.md)） | `manage_webhooks` |
| サーバー管理 | リレー（`/admin/relays`、ADR-0006） | `manage_settings` |
| サーバー管理 | dead job（`/admin/dead-jobs`） | `manage_jobs` |
| サーバー管理 | 費用（`/admin/finance`。費用の項目の一覧とフォーム、週・月・年の集計、請求の取り込みの状態、[ADR-0067](0067-finance-cost-records.md)） | `manage_finance` |

どちらにも関わるものの置き場所:

- **監査ログ**はモデレーションに置く。処分の記録が大半で、`view_audit` は既定でモデレーターも持つ。設定とロールの変更も同じログに載り（FR-704）、サーバー管理の画面は自分の変更の行へ絞り込んだ監査ログにリンクする。経路は 1 つにする。
- **ドメインへの処分**はモデレーションに置く（FR-702 の処分であり、通報からも出す）。サーバーの全員に効くので、既定のモデレーターは `manage_domains` を持たない（§4）。
- **連合先**（federation.md §7.3 の配送の状態）はモデレーションに置き、`manage_domains` で出す。ドメインへの処分を決める人がそのサーバーの様子を見る画面で、詳細の「モデレーション」の区画からそのドメインへの処分（`/moderation/domain-actions?domain=…`）にリンクする。Mastodon も連合先をモデレーションに置き、配送の再開をそこに持つ。
- **招待**はモデレーションに置く。誰がサーバーに参加できるかを個別に決める点で、登録の審査と登録の拒否リストと同じ仕事である。登録モード（承認制・招待制）の切り替えはサーバーの設定に残る。
- **アカウントのロールの付与と剥奪**（FR-720）は、アカウントの管理画面（`/moderation/accounts/{id}`）の「ロール」の区画に置き、`manage_roles` を持つ閲覧者にだけ出す。ロールそのものの一覧（`/admin/roles`）はサーバー管理に置く。
- **リモートのカスタム絵文字の無効化**は、ADR-0051 §5 のとおり `manage_custom_emojis` で行い、カスタム絵文字の管理としてサーバー管理に置く。

### 2. Web の情報設計

- 経路を `/moderation/*` と `/admin/*` の 2 つの木に分ける。`/moderation` と `/admin` は、それぞれの区分で閲覧者が使える画面の一覧を出す（`AdminPages` の型）。使える画面がない区分は、その理由を `EmptyState` で出す。
- ナビゲーションは、モデレーションの権限（`manage_reports`・`manage_accounts`・`manage_domains`・`view_audit`）のどれかを持つ閲覧者に「モデレーション」、サーバー管理の権限のどれかを持つ閲覧者に「管理」を、設定の後に「モデレーション」「管理」の順で足す。`member` の権限だけを持つ閲覧者にはどちらも出さない（ADR-0070 §1）。
  レールとサイドバーとメニューのシートに出し、タブバーには出さない（ADR-0110 はメニューの中身をこの決定に任せている）。
- 未対応の件数（FR-712）のバッジは「モデレーション」に付ける。「管理」には件数を付けない。
- 設定の画面の「管理」の区画は、「モデレーション」と「管理」の 2 つの区画にする。
- 1 つの画面は 1 つの経路だけを持ち、両方の区分には置かない（`aria-current` が 1 つに決まる）。

### 3. API

- 経路は `/admin/*` に置く。`/admin` はスタッフの操作の名前空間で、利用者本人の `/moderation/actions` と区別する。スコープは `admin:read` / `admin:write` にする。
- OpenAPI のタグを `admin-moderation`（上の表のモデレーションの操作と `/admin/counts`）と `admin-server`（サーバー管理の操作）に分け、[client-api.md](../architecture/client-api.md) のリファレンスと生成コードのまとまりを区分に合わせる。
- 新しいサーバー管理の操作は `/admin/<資源>`（`/admin/announcements`・`/admin/webhooks`・`/admin/relays`・`/admin/custom_emojis`・`/admin/settings` など）、新しいモデレーションの操作（`/admin/invites` など）も `/admin/<資源>` に置き、タグで区分を示す。
- 認可は、スコープの後に操作ごとの権限で決める。経路や名前空間を認可の代わりにしない。

### 4. 権限とロールの既定

- 権限に区分（`moderation`・`server`・`member`。`member` の条件と権限は ADR-0070 §1）を持たせ、`internal/domain/identity` に区分を返す関数を置く。ナビゲーションの出し分けと、`my admin role` などの CLI の表示がこれを使う。
  - モデレーション: `manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit`
  - サーバー管理: `manage_settings`・`manage_rules`・`manage_roles`・`manage_jobs`・`manage_announcements`・`manage_custom_emojis`・`manage_webhooks`・`manage_finance`
  - 機能が足したほかの権限（`manage_supporters`・`manage_oauth_apps` など）を含む全部の値と区分は、`internal/domain/identity` が持つ。
- 新しい権限は、その機能の Issue が足す（`manage_announcements` は #665、`manage_custom_emojis` は ADR-0051、`manage_webhooks` は ADR-0060）。テーマとリレーは設定の一部として `manage_settings` を使い、権限を分けない（ADR-0006 の決定に合わせる）。
- 組み込みのロールの既定: `admin` はすべての権限、`moderator` は `manage_reports`・`manage_accounts`・`view_audit`。モデレーターには「管理」が出ない。
  モデレーターはドメインへの処分（`manage_domains`）と招待（`manage_invites`）を既定では持たない。ドメインへの処分はサーバーの全員に効くので、管理者が決めるか、管理者がそれを持つロールを作って与える（Mastodon の既定の Moderator も連合と拒否を持たない）。
- 異議申し立ての審査は、処分と同じ人が審査するので `manage_accounts` のままにし、独立した権限（Mastodon の `manage_appeals`）は作らない。
- アカウントの検索と管理画面の権限（`manage_accounts`）は、メールアドレスと IP の開示（§9、FR-714）を含むので分けない。

### 5. コードの置き場所

- **Go**: サーバー管理の機能は、そのデータを持つモジュールに置く。設定・規約・ルール・テーマ・お知らせは `instance`、ロールと招待は `identity`、リレーと連合先の配送の状態は `federation`、dead job は `queue`、カスタム絵文字と webhook はそれぞれの ADR（ADR-0051・ADR-0060）が決めるモジュールに置く。`internal/app/moderation` はモデレーションだけを持つ。
  `internal/app/admin` は作らず、`internal/interface/admin` も持たない（[overview §6](../architecture/overview.md#6-リポジトリ構成)）。管理の HTTP の入口は `internal/interface/clientapi` だけにする。
  `clientapi` のハンドラのファイルは、`admin_*.go` の名前で区分ごとにまとめる。
- **Web**: モデレーションは `features/moderation`、サーバー管理は `features/admin` に置く。両方が使う部品（`RequirePermission`・`RecordList`・`DetailSection`・`useFocusAfterChange`・権限の読み出し）は片方の `index.ts` から出す（web-client.md の規則 A3）。
  ルートは `routes/moderation.*.tsx` と `routes/admin.*.tsx`、権限の検査（`loadWithPermission`・`AdminError`）は両方の木が使う。

### 6. 処分を連合先へ伝える

Mastodon と同じ形にする。凍結は `Update(Actor)` に `suspended: true`、BAN は猶予期間の後に `Delete(Actor)`、ドメインの凍結はフォローを切って記録する。

| 処分 | 送るもの | 宛先 |
| --- | --- | --- |
| ローカルの `suspend` / `ban` | `Update(Actor)`（`suspended: true`、名前・アイコン・自己紹介・プロフィール補足情報を空にした Actor） | 承認済みのリモートのフォロワーの inbox と既知の shared inbox（`Delete(Actor)` と同じ。[data-model §8](../architecture/data-model/08-deletion-and-purge.md#8-削除と消去)） |
| ローカルの `suspend` の解除・期限切れ | `Update(Actor)`（元のプロフィール） | 同上 |
| ローカルの `ban` | 上の `Update` に加えて、`deletion_scheduled` の期日に `Delete(Actor)`（アカウント削除と同じ） | 同上 |
| ローカルの投稿への `delete_statuses` | 投稿ごとの `Delete`（本人の削除と同じ） | 投稿を配送した宛先 |
| ローカルの `limit` / `restrict_posting` / `force_sensitive` など | 送らない（以後の投稿が上限どおりの公開範囲で配送されるだけ） | — |
| リモートのアカウントの `suspend` | そのアカウントからローカルの利用者へのフォローに `Reject(Follow)`、ローカルの利用者からそのアカウントへのフォローに `Undo(Follow)` | そのアカウントの inbox |
| ドメインの `suspend` | 送らない（配送を止めるため）。両方向のフォローを `follow_terminations`（`reason = moderation`）で切る | — |
| 通報の転送（FR-709） | `Flag`（主体はインスタンス Actor、`object` は対象の Actor と投稿の URI、`content` はコメント） | 対象のアカウントの inbox |

- `suspended` は Mastodon 拡張（`toot:suspended`）の語彙で、`@context` は既存の Actor と同じものを使う。凍結中の Actor の GET は、`404` ではなく、空のプロフィールに `suspended: true` を付けた Actor を返す（鍵は返す。`Delete` の署名を検証できるようにするため）。
- 凍結と BAN の配送は、処分のトランザクションで outbox に積む（ADR-0003）。期限切れは読み取り時の判定なので、期限の時刻に `Update` を積む定期ジョブを置く。
- 切ったフォローは、処分を解除しても戻さない。ドメインの凍結で切れた関係は、影響を受けたローカルの利用者ごとに件数を記録して通知する（FR-717）。
- 通報の転送は、ローカルの通報者が転送を選んだ通報だけを、通報を受け付けたトランザクションで outbox に積む。通報者・IP・メールアドレスは送らない。返信先の第三者のサーバーへの転送はしない。
- 受信した `suspended: true` の `Update(Actor)` は、リモートのアカウントの表示に「相手のサーバーで凍結」として出すだけで、このサーバーの処分にはしない。

### 7. 監査ログ

処分のイベントとは別の、追記専用の記録にする。

- `audit_log_entries` は `moderation` が持つ【イベント】で、カラムは `id`, `actor_kind`（`enum(user, cli, system, remote)`）, `actor_user_id`（NULL 可）, `action`（操作の enum）, `target_kind`, `target_id`, `target_label`（記録時の対象の表示名の写し。`@user@host`・ドメイン・設定のキーなど）, `source_event_id`（元のイベントの行の ID）, `occurred_at`。
- `target_kind` は `enum(account, status, domain, report, appeal, user, role, setting, rule, signup_block)` に、機能が足す値を加えたものである（ADR-0051 の `custom_emoji`、ADR-0060 の `webhook`、ADR-0006 の `relay`、ADR-0067 の `cost_item`、ADR-0070 の `custom_emoji_request`、ADR-0072 の `supporter_program`・`supporter_plan`・`supporter_subscription`、ADR-0088 の `instance`、ほかに `announcement`・`invite`・`billing_source`・`payment_provider`・`oauth_client`・`oauth_client_host`・`voice_room`・`login_provider`・`link_card_domain`）。全部の値は `schema/<dialect>/03-moderation.sql` にある。
- 各モジュールのユースケースは、元のイベントと同じトランザクションで `AuditRecorded` のドメインイベントを発行し、`moderation` の購読者がそれを書く（`uow.Event`。他のモジュールの `internal/app` を呼ぶ既存の経路だけを使う）。
- 理由・メモの本文は写さず、`source_event_id` で元のイベントから引く。監査ログ自体には IP アドレスを残さない。
- 更新・削除の API と CLI を持たない。消去は §8 の保持期間を過ぎた行を消す定期ジョブだけが行う。
- IP アドレスとメールアドレスを表示した操作（アカウントの詳細で「表示」を押したとき）も `action = revealed_private_data` で記録する。
- CLI は操作者を `actor_kind = cli` で記録する。CLI の実行者の OS ユーザー名は残さない。
- 改ざん検知（ハッシュ連鎖など）は持たない。DB に直接書ける運営者は監査ログも書き換えられ、1 人で運用するインスタンスでは効果が薄いため。

### 8. 個人データの保持期間（既定値と下限）

記録の種類ごとに既定の保持期間を持ち、インスタンス設定で変えられる（下限つき）。

| 記録 | 既定 | 下限 | 起点 | 期限を過ぎたら |
| --- | --- | --- | --- | --- |
| ログイン元の IP と User-Agent | 180 日 | 90 日 | セッションの作成 | 行を消す |
| 登録元の IP | 180 日 | 90 日 | 登録 | 行を消す |
| 通報（コメント・対象の投稿への参照・メモ・担当と解決のイベント） | 365 日 | 180 日 | 最後の解決 | 消す。処分が参照する通報は処分と同じ期限まで残す |
| 処分（種類・理由・メモ・対象の投稿）と異議申し立て | 365 日 | 180 日 | 処分の終わり（解除・期限切れ・異議の審査の遅い方）。無期限の処分は効いている間ずっと残す | 消す |
| 監査ログ（§7） | 730 日 | 365 日 | 記録 | 消す |
| 登録の拒否リスト | 拒否が効いている間 | — | — | 解除・期限切れから 30 日で消す |

- 期間は `instance_settings` のキー（`retention_*`）で変えられる。下限より短くはできない。
- 通報や処分が参照する投稿は、本人が削除しても、参照する記録の期限まで物理削除しない（表示からは消える）。証拠が先に消えないようにするため。
- 消去は FR-803 の定期ジョブと同じ仕組み（1 回の件数に上限）で行う。
- 運営者は、アカウント単位で消去を止める保全（`retention_holds`）を CLI で設定・解除できる。裁判所の消去禁止命令（情報流通プラットフォーム対処法 16 条）や開示請求の対応中に使う。

### 9. 開示の範囲

- 通報者の身元（アカウント・転送元のサーバー）は、通報された本人に見せない。本人への処分の通知・本人の処分の画面（FR-715）・データエクスポート・連合（§6）のどれにも出さない。
- 本人の処分の画面とデータエクスポート（FR-008）には、自分への処分（種類・公開の理由・対象の投稿・期限）、自分の異議申し立てとその結果、自分が出した通報（対象と結果）を入れる。非公開のメモは入れない。
- 開示請求（GDPR 15 条・個人情報保護法 33 条）には、運営者が CLI でアカウントのモデレーション記録をすべて書き出して応じる。非公開のメモを含め、通報者の身元だけを伏せる。
- IP アドレスとメールアドレスは `manage_accounts` を持つ人にだけ、明示的な「表示」の操作で見せ、その操作を監査ログに残す（§7）。
  例外は、審査待ちの登録の一覧（`GET /admin/registrations`）のメールアドレスで、表示の操作なしに返し、監査ログにも残さない。審査の判断材料（使い捨てのドメインや同じ人の再登録の見分け）で、1 件ずつ表示させると監査ログが審査の記録で埋まるだけだからである。決定済みの登録には返さない（承認・却下の答えも同じ）。

### 10. 異議申し立ての期間

- 処分の通知から 180 日（インスタンス設定で 20〜365 日）。Mastodon の 20 日より長いのは、DSA 20 条の 6 か月に合わせ、処分の記録の保持期間の下限と揃えるため。

### この決定を見直す条件

- 第三者のモデレーション用ツールに、サーバー管理を触れないトークンを渡す必要が出たとき（モデレーションの経路とスコープを分ける）。モデレーターと管理者を分けて運用するサーバーがほとんどないと分かったとき（`/admin` の一覧を見出しで分ければ足りる）。
- Misskey または GoToSocial が `suspended` を解釈せず、連合先での表示が凍結前のまま残ることが相互運用テストで分かった場合は、凍結でも `Delete(Actor)` を送る案を見直す。
- 複数のモデレーターで運用するインスタンスで、運営者自身の操作を第三者が検証する要件が出た場合は、ハッシュ連鎖か外部への複製を足す。
- 日本または EU の監督機関や裁判所が、連合型 SNS の運営者に具体的な保存期間を示した場合は、その値に合わせる。

## 経緯

運用者の機能は Web も API も 1 つの `/admin/*` にまとまり、ロールの権限だけで分かれていた。M4 で設定・ルール・招待（#624）、テーマ（ADR-0059）、お知らせ（#665）、webhook（ADR-0060）、リレー（ADR-0006）、カスタム絵文字（ADR-0051）が足されるので、このままではモデレーターの毎日の仕事と管理者がたまに触る設定が 1 つのリストに混ざり、各機能が別々の置き場所を選ぶ。そこで置き場所の規則を先に決めた（FR-602、FR-701〜FR-721）。判断基準は、モデレーターが使えるものだけを見ること、権限が認可の唯一の根拠であること、これから足す機能の置き場所が一意に決まること、モジュールの境界（ADR-0002）を崩さないこと、意味のない改名で API と生成コードを動かさないこと。ほかの実装（2026-10-20 に一次資料で確認）は、Mastodon・GoToSocial・Misskey とも API を 1 つの `admin` の名前空間に置いて操作ごとの権限で分け、Web は GoToSocial が経路（[moderation/router.tsx](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/web/source/settings/views/moderation/router.tsx)）、Mastodon（[config/navigation.rb](https://github.com/mastodon/mastodon/blob/main/config/navigation.rb)）と Misskey がメニューの見出しで分ける。Mastodon の既定の Moderator は連合と拒否の権限を持たない（[config/roles.yml](https://github.com/mastodon/mastodon/blob/main/config/roles.yml)）。権限の区分は当初 `moderation` と `server` の 2 つで、ADR-0070 が `member` を足した。

処分の連合（FR-703、FR-713、FR-702、FR-716、FR-717、FR-709、NFR-MOD-3、NFR-MOD-5）: 処分はこのサーバーの中でだけ効き、凍結しても連合先に投稿とプロフィールが残っていた。2026-10-17 にソースで確認した挙動（[#327 の調査報告](https://github.com/mycroblog/my/issues/327)）では、Mastodon は凍結で `suspended: true` の `Update` を送り、データを 30 日後に消すときに `Delete` を送る（[`suspended`](https://docs.joinmastodon.org/spec/activitypub/#suspended)）。Misskey は凍結で `Delete(Person)` を送り、解除で `Undo(Delete)` を送る。GoToSocial のドメインブロックは関係・投稿・メディアを消す。サイレンスは連合せず、通報の転送はインスタンス Actor から `Flag` を送る（[Flag](https://docs.joinmastodon.org/spec/activitypub/#Flag)）。解除できる処分を連合先で戻せること、既存の語彙だけを使うこと、通報者の身元を出さないこと、送る数をフォロワーのサーバー数に比例する範囲に抑えること（NFR-COST-5）を基準に選んだ。

監査ログ（FR-704、NFR-MOD-1）: 対象の操作は `moderation`・`identity`・`instance` にまたがり、各イベントテーブルを横断して読むと `moderation` が他のモジュールのテーブルを読むことになる（ADR-0002）。Mastodon は専用の `admin_action_logs`（[`Admin::ActionLog`](https://github.com/mastodon/mastodon/blob/main/app/models/admin/action_log.rb)）、Misskey は `moderation_log`、Pleroma / Akkoma は `ModerationLog` を持つ。GDPR 32 条と個人情報保護法 23 条は処理の記録を検証できる仕組みを求める。

保持期間（FR-008、FR-701、FR-707、FR-714、FR-719、NFR-MOD-5）: [data-model §8](../architecture/data-model/08-deletion-and-purge.md#8-削除と消去) はアカウントを削除しても処分の記録を残すが、いつまでかは決まっていなかった。法令とガイドライン（調査報告 [#327](https://github.com/mycroblog/my/issues/327) §法令。法的助言ではない）:

- GDPR は趣味のインスタンスにも及ぶ（2 条 2 項 c の家庭内の例外は、他人に SNS を提供する者には当たらない。[前文 18](https://gdpr-info.eu/recitals/no-18/)）。記録の種類ごとに保持期間を正当化できること（[5 条 1 項 e](https://gdpr-info.eu/art-5-gdpr/)）、本人への開示（[15 条](https://gdpr-info.eu/art-15-gdpr/)。他人の権利を害する部分は伏せられる。15 条 4 項）、IP アドレスは個人データ（CJEU Breyer C-582/14）。
- 個人情報保護法は非営利・個人の運営者にも及ぶ（16 条 2 項、[通則ガイドライン](https://www.ppc.go.jp/personalinfo/legal/guidelines_tsusoku/) 2-5）。不要になった個人データを遅滞なく消す努力義務（22 条）、開示（33 条。第三者の権利利益を害するおそれがあれば一部を開示しない）。
- 情報流通プラットフォーム対処法の発信者情報開示に応じるため、総務省はログイン等の通信記録を「少なくとも 3〜6 か月程度」保存することを望ましいとしている（[総務省 2025-09 の報告書](https://www.soumu.go.jp/main_content/001030756.pdf)）。法令上の保存義務はない。
- DSA 20 条は、処分の通知から少なくとも 6 か月の異議申し立てを求める（小規模事業者は 19 条で除外。非営利のインスタンスが「情報社会サービス」に当たるかは不明）。
- Mastodon は IP とセッションを既定で 1 年保持し、異議申し立ては処分から 20 日、監査ログは消さない（[設定](https://docs.joinmastodon.org/admin/config/)、[`AccountWarning`](https://github.com/mastodon/mastodon/blob/main/app/models/account_warning.rb)）。

## 採らなかった案

- Web を `/admin` のまま一覧の見出しで分ける（Mastodon の形）: 経路が変わらず変更が一覧だけで済むが、モデレーターの入口は「管理」のままで件数のバッジと名前が合わず、区分の境界が URL に出ない。
- Web を 1 つの一覧のままにする: 変更はないが、#648 の整理に応えず、使えない項目が増える。
- API を `/moderation/*` と `/admin/*` に分け、スコープも分ける: 経路がそろいモデレーションだけのトークンを作れるが、本人の `/moderation/actions` と名前空間が混ざり、OAuth の同意画面も変わり、ほかの実装に例がない。
- API を `/admin/moderation/*` と `/admin/server/*` の 2 段にする: 経路で区分が分かるが、すべての経路と生成コードが動き、どちらにも関わる操作の置き場所をもう一度決める必要がある。
- `internal/app/admin` に集める: 管理のコードが 1 か所に見えるが、データの所有者が別なので、呼ぶだけの層になるかデータを二重に持ち、ADR-0020 と衝突しやすい。
- 凍結でもすぐ `Delete(Actor)` を送る（Misskey の方式）: 連合先の複製を確実に消せるが、Mastodon は `Undo(Delete)` を解釈しないので、期限付きの凍結が Mastodon の連合先では実質の BAN になる（`Undo(Delete)` の扱いは要検証）。
- 処分を連合先に何も送らない: 処分の誤りが広がらないが、凍結したアカウントの内容が連合先に残り続ける。
- 監査ログを各イベントテーブルの横断で読む: 二重の記録がなく元のイベントが唯一の事実になるが、一覧のたびに複数のテーブルを合成して並べ替え、対象が消去されると読めない。
- 監査ログを構造化ログ（NFR-OPS-1）に出すだけ: 実装は最小だが、保管先が構成ごとに違い、管理画面から引けない。
- Mastodon と同じく IP だけ 1 年で消し、ほかは残し続ける: 運営者が予想しやすいが、処分・通報・監査ログを無期限に持つことを GDPR 5 条 1 項 e に照らして説明できない。
- 保持期間を運営者に任せる: 法域ごとの事情に合わせられるが、既定のままでは何も消えない。

## Consequences（影響）

- 良い: モデレーターのナビゲーションには使える画面だけが並び、件数のバッジが仕事の入口と一致する。これから足す機能の置き場所と権限が表から決まる。
- 良い: 新しい権限は、権限の分け方が運用で意味を持つもの（お知らせ・カスタム絵文字・webhook）にとどめ、設定の一部は `manage_settings` にまとめる。
- 良い: 期限付きの凍結を解除すれば、Mastodon の連合先ではプロフィールが戻る。BAN と削除は既存の `Delete(Actor)` の経路を使い回せる。
- 良い: 監査ログの一覧は 1 つのテーブルのキーセットページングで引ける（NFR-MOD-8）。対象が消去されても `target_label` で読める。
- 良い: 既定値のまま運用すれば、プライバシーポリシーに書く保持期間が決まる。IP を 180 日で消すので、漏洩したときの影響が小さい。
- 悪い: 既定のモデレーターは連合先・ドメインへの処分・招待の画面を使えない。モデレーターに任せるサーバーは、`manage_domains` や `manage_invites` を持つロールを作って与える。
- 悪い: ナビゲーションの項目が、モデレーターと管理者を兼ねる人には 1 つ増える。Web の経路（`/moderation/*`）と API の経路（`/admin/*`）が一致しない。
- 悪い: スコープを分けないので、`admin:*` のトークンはロールの権限の範囲で両方の区分を操作できる。第三者のツールを区分で縛れない。「サーバー管理」のコードは複数のモジュールに散り、区分は Web・タグ・権限の区分でだけ見える。
- 悪い: `suspended` を解釈しない実装では、凍結中もキャッシュした投稿が残る。ドメインの凍結を解除しても関係は戻らず、利用者はフォローし直す必要がある。
- 悪い: 監査ログは操作ごとに 1 行の書き込みが増える。新しい管理操作を足すとき、`action` の enum と発行を忘れないようにテストで検査する必要がある。`target_label` は個人データ（ハンドル）を含むので、保持期間（§8）の対象になる。
- 悪い: 365 日より前の処分は再犯の判断に使えない。発信者情報開示の請求が投稿から 180 日を過ぎて届くと、ログイン元の IP は残っていない。
- `sessions.ip` / `user_agent` は消去の単位を分けるため別の表（`session_origins`）に置き、登録元の IP は `registration_origins` に置く（[data-model §4.1](../architecture/data-model/04-tables/01-identity.md#41-identity)）。
- 運営者向けに、記録の種類・目的・保持期間・提供先（連合先）を書いたプライバシーポリシーと処理の記録（GDPR 30 条）のひな形を `docs/operations/` に置く。

## Open Questions / 要検証

- Misskey と GoToSocial が `suspended: true` の `Update(Actor)` をどう扱うか（#319 の相互運用テストに足す）
- 凍結した Actor の `Update` を、フォロワー以外の既知の shared inbox にまで送るか（Mastodon は届く限り送る。費用と効果を測る）
- 非営利のインスタンスが DSA の「情報社会サービス」と電気通信事業法の電気通信事業者に当たるか（専門家の確認）
- 連合の配送が GDPR 第 5 章・個人情報保護法 28 条の越境移転に当たるか、プライバシーポリシーでどう説明するか
