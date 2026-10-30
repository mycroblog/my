# ADR-0070: 権限の分割: 絵文字の作成と会計の閲覧

- Status: Accepted（§4 の数えるものと上限が下がったときの扱いを ADR-0124（カスタム絵文字の作成数の上限を毎月リセットする）が置き換える）
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #910

## Context（背景・課題）

M4.2（Supporters、Epic #925）は、課金のプランに結び付いたロールに、絵文字の作成（FR-915）と会計の閲覧（FR-916）を渡す。
決定者の指示（#925、原文）: 「medium プランは medium-support ロールが紐付けられ、同上の機能と共にロールに紐づいた絵文字管理者権限により、自由に絵文字を増やすことができる。しかし、絵文字の更新・削除はできない(自分の作った絵文字を除く)」
「large プランでは large-support ロールが紐付けられ、medium 同等機能と、管理者にしか開示されていない会計レポートの閲覧機能を得る」。

決定者の回答（#925、原文の抜粋）:

> 4\. 推奨: 課金のプランに結び付けられるロールの権限を、許可リストに限ります。最初は create_custom_emojis と view_finance だけです。position もモデレーターより下に固定します。 → これで OK
> 5\. 費用については、名前とメモを見せるかどうかを設定できるようにします(別々に。デフォルトは見せない)。収入については、各プランに何人いるかまでは見せられるように設定できるようにします。デフォルトは全体。誰がいくら払ったか → こちらは見せない
> 6\. 推奨: 1人あたりの作成数の上限を、プランごとに設定できるようにします。作成は監査ログに残します。解約したあとも絵文字は残し、作成者だけが変更できなくなります。 → これで OK
> 管理者による事前の審査（承認制）を挟むかどうか → 承認制を導入する

いまの権限は全部か無しかで、プランのロールに渡せない。

- `manage_custom_emojis`（[ADR-0051](0051-custom-emoji.md) §5）は、ローカルとリモートのすべての絵文字の登録・変更・無効化・削除を許す。
  `custom_emojis` は作成者を記録しない（`internal/app/content/custom_emoji_admin.go` の `register` は `ByUserID` を監査ログのイベントにだけ渡す）。
  画像は作成した管理者のアカウントの upload で、削除すると upload が未添付に戻り、未添付の upload の掃除が Blob ごと消す。
- `manage_finance`（[ADR-0067](0067-finance-cost-records.md) §7）は、費用の記録と集計の閲覧の両方を許す。集計（`GET /admin/cost_totals`）は事業者・区分・通貨ごとの内訳を返し、項目の名前とメモは項目の一覧（`/admin/cost_items`）にある。
- 権限の区分（[ADR-0058](0058-moderation-admin-separation.md) §4）は `moderation` と `server` の 2 つで、`internal/domain/identity/role.go` の `Category` は区分の分からない権限を `server` にする。
  ナビゲーションは、`server` の権限を 1 つでも持つ閲覧者に「管理」を出す（ADR-0058 §2）。管理の API は `/admin/*` で、スコープ `admin:read` / `admin:write` を要する。
- 監査ログ（[ADR-0037](0037-audit-log.md)）はスタッフの操作を `audit_log_entries` に残し、`view_audit` を持つ人が読む。
- 管理者への通知は、`admin_sign_up` などの種類を権限を持つ人に送る（`internal/app/notification/moderators.go`）。

収入の値（手数料を引いた合計、プランごとの人数）の出どころは ADR-E（#914）、プランごとの上限の値と、プランとロールの連動は ADR-A（#912）が決める。
この ADR は、2 つの権限が何を許し、何を見せるかと、それを受ける口を決める。実装の #915（`view_finance`）と #916（`create_custom_emojis`）がこの ADR を待っている。

関連: FR-910〜FR-917（#908）、FR-111（カスタム絵文字）、FR-704（監査ログ）、FR-720（ロール）、FR-901〜FR-903（費用）、
[ADR-0002](0002-modular-monolith-hexagonal.md)、[ADR-0009](0009-internationalization.md)、[ADR-0020](0020-data-modeling-principles.md)、ADR-0037、ADR-0051、ADR-0058、[ADR-0060](0060-instance-webhooks.md)、ADR-0067、[ADR-0068](0068-billing-import.md)。

## Decision Drivers（判断基準）

1. **承認されていない絵文字がどこにも出ない**: 申請中・却下された画像が、ピッカー・投稿の抽出・連合・リアクションのどの経路にも載らない（決定者の「承認制」）
2. **お金を払った人に、払った分より強い権限を渡さない**: プランのロールの権限はスタッフの権限と混ざらず、他人の絵文字やサーバーの設定に手が届かない（FR-912）
3. **払った人の個人情報を出さない**: 誰がいくら払ったかは、どの設定でも `manage_finance` を持つ人にしか出ない。項目の名前とメモも既定では出さない
4. **既存の決定にそのまま乗る**: ADR-0020 の分類、ADR-0002 のモジュールの境界、ADR-0037 の監査ログ、ADR-0058 の画面の置き場所、ADR-0067 の公開範囲と集計
5. **承認する人の手間が小さい**: 申請が 1 か所に集まり、承認・却下が 1 回の操作で済み、上限で申請の量が抑えられる
6. **実装が小さい**: 既存の絵文字の読み取りの経路と集計の関数を変えずに済む

## Considered Options（選択肢）

論点ごとに比べる。各論点の 1 つめを採用する。

- **申請の持ち方**: R1 申請を別のリソースにし、承認したときに絵文字を作る / R2 申請のときに絵文字の行を作り、「審査中」の状態で隠す
- **作成者が直接できる変更**: E1 別名・カテゴリ・ピッカーに出すか・削除は直接、画像の差し替えは申請 / E2 すべて直接（作成のときだけ審査） / E3 すべて申請
- **権限の区分と画面**: K1 第 3 の区分 `member`（利用者の機能）を足し、画面は利用者の側（`/settings/*`・`/finance`）、API は利用者の名前空間に置く / K2 `server` の区分に入れ、`/admin/*` に置く
- **上限の受け口**: Q1 `content` のアプリ層のポート `CustomEmojiQuota` で受け、プランの ADR の側が実装する / Q2 インスタンス設定の 1 つの値 / Q3 ロールに上限の属性を持たせる
- **会計の見せ方の設定の置き場所**: S1 インスタンス設定のキー（`manage_settings`） / S2 `finance` の設定（`manage_finance`）

## Decision（決定）

**R1・E1・K1・Q1・S1 を採用する。** 組み込みのロールでは `admin` だけが 2 つの権限を持つ。

決定者の回答（#929、原文）:

> 1\. 費用の内訳: 見せてよい
> 2\. 管理者が手で付けたロールの作成数の上限: その場合上限はなし
> 3\. plans を選んだときに見せる範囲: 人数だけ

### 1. 権限の区分と組み込みのロール（K1）

- 新しい権限 `create_custom_emojis` と `view_finance` を足す。
- ADR-0058 §4 の区分に 3 つめの **`member`（利用者の機能）** を足す。`member` の権限は、自分のものを作る・変える、または読み取りだけの閲覧を許し、他人の行やサーバー全体に効く変更を許さない。
  `create_custom_emojis` と `view_finance` は `member` に入る。`Category` は `member` の権限を明示して返し、区分の分からない権限を `server` にする扱いは変えない。
- FR-912 の許可リストは「区分が `member` の権限」とする。ADR-A（#912）はこの区分でプランのロールの権限を検査する。`member` の権限を後から足すときは、その ADR が区分の条件（自分のものか読み取りだけか）を満たすことを示す。
- ナビゲーション: `member` の権限だけを持つ閲覧者には「モデレーション」も「管理」も出さない（ADR-0058 §2 の規則は `moderation` と `server` の権限だけで決まる）。
  `view_finance` を持つ閲覧者には、レール・サイドバー・メニューのシートの設定の後に「会計」（`/finance`）を足す。`create_custom_emojis` の画面は設定の画面の中（`/settings/custom-emojis`）に置き、ナビゲーションの項目を増やさない。
- 組み込みのロール: `admin` はすべての権限（2 つを含む）を持つ。`moderator` は持たない（いまと同じ `manage_reports`・`manage_accounts`・`view_audit`）。プランのロールは管理者が作る（ADR-A）。
- 包含: `manage_custom_emojis` を持つ人は申請なしで `/admin/custom-emojis` から作れるので、`create_custom_emojis` の画面と上限は使わない。`manage_finance` を持つ人は `/finance` も読め、そこでは §5 の設定にかかわらずすべての項目を見る。

| 区分 | 画面（Web の経路） | 権限 |
| --- | --- | --- |
| 利用者の機能 | 自分の絵文字（`/settings/custom-emojis`。申請・申請の状態・自分の絵文字の変更と削除・上限と使った数） | `create_custom_emojis` |
| 利用者の機能 | 会計（`/finance`。費用と収入の集計の読み取り） | `view_finance` |
| サーバー管理 | カスタム絵文字（`/admin/custom-emojis`。ADR-0051 の画面に「申請」のタブを足す） | `manage_custom_emojis` |

### 2. 絵文字の作成の申請（R1）

- 利用者は、shortcode と画像（自分の upload。ADR-0051 §2 の制約）で**作成の申請**をする。カテゴリと別名は申請に含めず、承認の後に作成者が付ける。
- `manage_custom_emojis` を持つ人が、申請を**承認**するか、理由（任意、500 文字まで。申請者に見える）を付けて**却下**する。申請者は審査の前なら**取り下げ**られる。
- 承認すると、同じトランザクションで ADR-0051 の絵文字（`custom_emojis`・`custom_emoji_images`・既定の設定）を作り、作成者に申請者を記録する。カテゴリなし・ピッカーに出す・有効で作る。
  承認のときに次を検査し、満たさなければ承認は `409` で失敗する（承認する人は却下する）: 申請者がまだ `create_custom_emojis` を持つ、shortcode がローカルの絵文字にない。
- 画像: 承認のときに、申請者の upload の処理結果を、インスタンスの Actor の upload として複製し、絵文字はその複製を指す（ADR-0051 §4 の複製と同じ）。申請者の upload は未添付に戻り、既存の掃除が消す。
  絵文字の画像が申請者のアカウントの削除・消去（FR-803）と一緒に消えないようにするため。申請中の画像は申請者の未添付の upload のままで、どの公開の文書からも参照されない。
- shortcode の予約: 審査中の作成の申請は shortcode を予約し、同じ shortcode の申請とローカルの絵文字の作成を `422`（ADR-0051 の `EmojiShortcodeTaken`）にする。却下・取り下げで予約は外れる。
- 承認されていない申請は、ADR-0051 の読み取りの経路（ピッカー、本文からの抽出、`GET /ap/emojis/{uuid}`、リアクション、`/admin/custom_emojis` の一覧）に一切現れない。申請は別のテーブルにあり、絵文字の行は承認まで存在しないから（判断基準 1）。

### 3. 作成者による変更と削除（E1）

- 作成者が `create_custom_emojis` を持つ間、自分が作った（`created_by_user_id` が自分の）ローカルの絵文字に対して、別名・カテゴリ・ピッカーに出すかの変更と、削除を直接できる。ADR-0051 の管理の操作と同じアプリ層の操作を、作成者の検査を足して呼ぶ。
- **画像の差し替えは申請にする**（種類 `image` の申請。承認すると ADR-0051 の画像の差し替えになる）。承認された画像を後から別の画像に変えて審査を通り抜けられないようにするため。1 つの絵文字に審査中の画像の申請は 1 つまで。
- 無効化と有効化は `manage_custom_emojis` だけが行う（作成者は削除できる）。shortcode は ADR-0051 のとおり変えられない。
- **権限を失った後**: 絵文字は残り、作成者も変更・削除できなくなる。作成者かどうかは操作のたびに「作成者であり、いま `create_custom_emojis` を持ち、アカウントが有効（凍結・削除されていない）」で決めるので、再加入すればまた変更できる。
  `manage_custom_emojis` を持つ人は、作成者にかかわらずすべての絵文字を ADR-0051 のとおり変更・削除できる。
- 作成者のアカウントが消去されても絵文字は残り、管理画面は作成者を「削除されたアカウント」と出す。

### 4. 上限（Q1）

- `internal/app/content` にポート `CustomEmojiQuota` を置く: `Limit(ctx, userID) (limit int, limited bool, err error)`。`limited` が false なら上限なし。`internal/platform` が実装を組み立てる。
  プランごとの値と、利用者のプランからの導き方は ADR-A（#912）が決め、その実装を差し込む。課金が無効か、ロールがプランから来ていない（管理者が手で付けた）ときは上限なしを返す。
- 数えるもの: 作成者が自分である、存在するローカルの絵文字（承認されたもの）と、審査中の作成の申請の合計。却下・取り下げ・削除した絵文字は数えない（削除すると枠が空く）。画像の申請は数えない。
  申請のときに合計が上限以上なら `422` にする。上限が下がっても（プランの変更）、既存の絵文字は残し、上限より少なくなるまで新しい申請だけを断る。
- 上限なしのときは、作成した絵文字の数も審査中の申請の数も制限しない（決定者の回答）。申請の量は承認する人が却下で抑える。
- 画面は「使った数 / 上限」を出す。上限の値は API で返す。

### 5. 会計の閲覧（S1）

`view_finance` は、ADR-0067 の費用の集計と ADR-E（#914）の収入の集計を**読むだけ**で、記録・改定・終了・取り消し・取り込みの開始はできない。
見せる範囲はインスタンス設定の 3 つのキーで決める。ADR-0067 §8 の `finance_visibility` と同じく `manage_settings` で守り、サーバーの設定の画面（`/admin/settings`）の費用の区画に置く。
公開範囲（`finance_visibility` と ADR-E の収入の公開のキー）とは独立で、`/finance` は少なくとも公開のページに出ている範囲を出す。

| キー | 型（未設定の値） | 内容 |
| --- | --- | --- |
| `finance_viewer_item_names` | `bool`（`false`） | 費用の項目の名前と、取り込んだ費用のサービス名を見せる |
| `finance_viewer_item_memos` | `bool`（`false`） | 費用の項目のメモを見せる |
| `finance_viewer_income` | `enum(totals, plans)`（`totals`） | 収入を合計だけ（`totals`）か、加えてプランごとの加入者の数まで（`plans`）見せる |

- **常に見せる**: 期間ごとの通貨ごとの費用の合計（按分と現金主義、ADR-0067 §4）、事業者・区分ごとの内訳（項目の名前とメモではないので、設定によらず見せる。決定者の回答）、見込みの印、区切りの規則（ADR-0067 §5）。
  収入は ADR-E が決める手数料を引いた合計を、期間ごと・通貨ごとに見せる。
- **設定で足すもの**: 名前かメモのどちらかが有効なら、期間ごとに項目の行（事業者・区分・その期間の額に、有効なら名前・メモ）を出す。メモだけが有効なら、名前のない行にメモが付く。
  `plans` なら、ADR-E が決める時点でのプランごとの加入者の数を出す。プランごとの金額は出さない（決定者の回答: 人数だけ）。
- **どの設定でも見せないもの**: 誰がいくら払ったか、加入者の名前やアカウント、決済の事業者の顧客や取引の ID、費用の改定の履歴、記録した人。これらは `manage_finance` を持つ人だけが管理画面で見る。
- API: 利用者の名前空間に `GET /finance/cost_totals`（ADR-0067 §9 の `GET /admin/cost_totals` と同じ問い合わせで、設定で省くフィールドを返さず、項目の行 `items` を足す）と、ADR-E の収入の集計を置く。
  どちらもタグ `finance`、新しいスコープ `read:finance`（`read` に含む）と `view_finance` か `manage_finance` を要する。応答は閲覧者ごとに変わらないが、権限で守るので `Cache-Control: private, no-store` にする。
  `GET /instance` には何も足さない（設定の値は `/finance` の応答に含めて返す）。
- `/admin/finance` は、いまの 3 つの設定を示し、`manage_settings` を持つ閲覧者にはその設定へリンクする（ADR-0067 §7 の公開範囲と同じ扱い）。

### 6. API の置き場所（K1）

- 利用者の名前空間（タグ `custom-emojis`、新しいスコープ `read:custom_emojis` / `write:custom_emojis`。`read` / `write` に含む）:
  `GET /custom_emoji_requests`（自分の申請。キーセットページング）、`POST /custom_emoji_requests`（作成か画像の申請）、`POST /custom_emoji_requests/{id}/withdraw`、
  `GET /custom_emojis/mine`（自分が作った絵文字と、上限・使った数）、`PATCH /custom_emojis/mine/{id}`（別名・カテゴリ・ピッカー）、`DELETE /custom_emojis/mine/{id}`。どれも `create_custom_emojis` を要する。
- 管理（タグ `admin-server`、`admin:read` / `admin:write` と `manage_custom_emojis`）: `GET /admin/custom_emoji_requests`（審査中を既定にし、状態で絞り込む）、
  `POST /admin/custom_emoji_requests/{id}/approve`、`POST /admin/custom_emoji_requests/{id}/reject`（理由）。`/admin/custom_emojis` の表現に `created_by`（作成者のアカウント、NULL 可）を足す。
- 管理の名前空間の `admin:*` のスコープを、スタッフでない利用者のトークンに求めない。`member` の操作を `/admin/*` に置かないのはこのため（判断基準 2）。
- CLI: `my admin emoji requests`（一覧・承認・却下）を足す。経路とフィールドの細部は [client-api.md](../architecture/client-api.md) の手順で決める。

### 7. データモデル（`content` が持つ。ADR-0020）

| テーブル | 分類 | カラム | 制約・規則 |
| --- | --- | --- | --- |
| `custom_emojis` | リソース（既存） | `created_by_user_id`（NULL 可）を足す | 作った利用者。CLI とリモートの絵文字は NULL。承認で作った絵文字は申請者、管理画面で作った絵文字と複製はその管理者。変えない |
| `custom_emoji_requests` | リソース | `id`, `kind`（`enum(create, image)`）, `requester_user_id`, `emoji_id`（NULL 可。`image` の対象）, `shortcode`, `media_id`, `created_at` | `kind = 'create'` は `emoji_id` が NULL、`image` は NULL でない（`CHECK`）。`image` の `shortcode` は対象の写し |
| `custom_emoji_request_outcomes` | イベント | `id`, `request_id`（一意）, `outcome`（`enum(approved, rejected, withdrawn, lapsed)`）, `decided_by_user_id`（NULL 可 = CLI と `lapsed`）, `reason`（NULL 可、500 文字まで。`rejected` だけ）, `emoji_id`（NULL 可。`approved` の結果の絵文字）, `decided_at` | 行がない申請が審査中。`lapsed` は対象の絵文字が削除された画像の申請 |
| `custom_emoji_pending_requests` | 投影 | `request_id`（PK）, `requester_user_id`, `reserved_shortcode`（NULL 可。`create` だけ）, `emoji_id`（NULL 可。`image` だけ） | 一意 `(reserved_shortcode)`, `(emoji_id)`。インデックス `(requester_user_id)`。申請で行を作り、結果の行と同じトランザクションで消す。`custom_emoji_requests` と結果から再構築できる |

- 審査中・却下などの状態を nullable な時刻で表さず、結果のイベントと審査中の投影で表す（ADR-0020 規約 4）。
- 終わった申請の行は残し、申請者の画面の履歴と監査ログの対象の表示名に使う。画像の upload は結果の後に未添付に戻るので、却下された画像の Blob は既存の掃除が消す。
- 絵文字の削除（ADR-0051 §1）は、その絵文字を対象とする審査中の画像の申請を `lapsed` で閉じる。

### 8. 監査ログと webhook

- **ADR-0037 への追加**: `target_kind` に `custom_emoji_request`（`target_label` は `:shortcode:`）を足し、`action` に `custom_emoji_request_submitted`・`custom_emoji_request_withdrawn`・`custom_emoji_request_approved`・`custom_emoji_request_rejected` を足す。
  承認は、申請への `custom_emoji_request_approved` と、作った絵文字への `custom_emoji_created`（画像の申請は `custom_emoji_image_replaced`）の 2 行を、承認した人を操作者として残す。
  作成者による変更と削除は、ADR-0051 の `custom_emoji_updated`・`custom_emoji_deleted` を、作成者を操作者として残す。ADR-0037 の他の規則は変えない。
- 会計の閲覧は記録しない（読み取りは監査ログの対象でない）。§5 の 3 つの設定の変更は、既存の設定の変更（`setting_changed`）として残る。
- webhook（ADR-0060）は、承認で `custom_emoji.created`（画像の申請は `custom_emoji.updated`）、作成者の変更で `custom_emoji.updated`、削除で `custom_emoji.deleted` を出す。申請そのものには webhook のイベントを足さない。

### 9. 通知

- `admin_custom_emoji_request`: 作成と画像の申請を、`manage_custom_emojis` を持つ人に送る（`admin_sign_up` と同じ配り方・重複の排除のキーは申請ごと）。リンクは `/admin/custom-emojis` の申請のタブ。
  ADR-0058 §2 のとおり「管理」には件数のバッジを付けず、審査中の件数は絵文字の画面のタブに出す。
- `custom_emoji_request_decided`: 承認と却下を申請者に送る（却下の理由を含む）。取り下げと `lapsed` は送らない。
- 通知の種類の enum・Web Push・メールの文言は既存の通知と同じ仕組みで足す（ADR-0009 のカタログ、`locales/en` と `locales/ja`）。

### ADR-0051・ADR-0058・ADR-0067 との関係

この ADR は次の文だけを置き換える。3 つの ADR は Accepted のままで、その他の決定と本文は変えない。各 ADR の Status の行に、この ADR が置き換える文を注記する（ADR-0066 が ADR-0016・ADR-0029 にしたのと同じ）。

- ADR-0051 §5「管理 API・CLI・管理画面の絵文字の操作はすべてこの権限で行う」: ローカルの絵文字の作成は `create_custom_emojis` の申請と承認（§2）でも行い、作成者は自分の絵文字の一部の操作（§3）を行う。
- ADR-0051 §1 の `custom_emojis` の行: `created_by_user_id` を足す（§7）。
- ADR-0058 §4「権限に区分（`moderation` / `server`）を持たせ」: 区分は `moderation`・`server`・`member` の 3 つ（§1）。
- ADR-0067 §7「費用の一覧・登録・変更・終了・取り消し・集計の閲覧に要る」のうち集計の閲覧: `view_finance` も、§5 の範囲で集計を読む。

追加（置き換えではない）: ADR-0037 の `target_kind` と `action`（§8）、ADR-0051 §4 の管理 API（§6）。

### この決定を覆す条件

- 承認の待ちが長く、承認する人の負担が問題になったら、信頼できるロールに審査なしの作成を許す権限（たとえば `create_custom_emojis_unreviewed`）を別の ADR で検討する。
- 画像の差し替えの申請がほとんど使われず、作成者が削除と作り直しで済ませていると分かったら、画像の申請をやめる（E1 から画像の差し替えを外す）。
- `member` に入れたい権限が、自分のものでも読み取りだけでもない（他人に効く）と分かったら、区分の条件を見直す。

## Consequences（影響）

- 良い影響
  - 承認されていない絵文字は、絵文字の行そのものがないので、既存の読み取りの経路を 1 行も変えずにどこにも出ない。
  - プランのロールの権限は `member` の区分で検査でき、FR-912 の許可リストが権限の定義と一緒に保たれる。スタッフでない利用者に「管理」も `admin:*` のスコープも要らない。
  - 作成者を記録するので、管理画面で誰が作った絵文字かが分かり、権限を失った後の扱いも操作のときの検査 1 つで決まる。
  - 会計の見せ方は 3 つのキーだけで、既定は最も狭い（名前・メモ・プランごとの人数を出さない）。個人ごとの支払いはどの設定でも出ない。
- 悪い影響・トレードオフ
  - 申請・結果・審査中の投影の 3 つのテーブルと、2 つの通知の種類、2 つの画面、新しいスコープ 3 つが増える。
  - 承認のときに画像の Blob を複製するので、承認された絵文字の画像の分だけ一時的に Blob が増える（申請者の upload が掃除されるまで）。
  - 審査中の申請が shortcode を予約するので、使われない申請が審査されないまま残ると、その shortcode を管理者も使えない。管理者は却下してから作る。
  - 作成者は画像を直接差し替えられず、差し替えのたびに審査を待つ。
  - `plans` を選ぶと、加入者の少ないプランでは人数から誰が加入しているかが推測できることがある（参加者の一覧やバッジと合わせると特に）。設定の画面にこのことを書く。
  - `member` の区分が増え、`Category` と ADR-0058 のナビゲーションの規則、`my admin role` の表示が 3 つの区分を扱う。
  - 管理者が手で付けたロールには上限がないので、その利用者が申請を大量に出すと、承認する人が 1 件ずつ却下することになる。
- 後続で必要になる作業
  - #915（`view_finance`）:
    - `internal/domain/identity`: `PermViewFinance`、区分 `CategoryMember` と `Category` の分岐、data-model §4.1 の `role_permissions.permission` の enum と `schema/<dialect>.sql` の `CHECK`（`member` の区分は #915 と #916 の先に入る方が足す）
    - `internal/domain/instance`: 設定 `finance_viewer_item_names`・`finance_viewer_item_memos`・`finance_viewer_income` と `/admin/settings` の費用の区画、`/admin/finance` の設定の表示とリンク
    - `internal/app/finance`: 閲覧者の設定で項目の行を足し、フィールドを省く読み取り（集計の関数は変えない）。`GET /finance/cost_totals`（タグ `finance`、スコープ `read:finance`）とスコープの追加（`knownScopes`・spec・同意画面の文言）
    - Web: `/finance` の画面とナビゲーションの「会計」、ADR-0058 §2 の規則に `member` を足す、`client*` のメッセージ
    - 収入の部分（合計と `plans` の人数）は、ADR-E（#914）が決める値で #923 が同じ画面と API に足す。`finance_viewer_income` のキーは #915 が足し、収入がない間は何も出さない
  - #916（`create_custom_emojis`）:
    - `internal/domain/identity`: `PermCreateCustomEmojis`（`member`）
    - スキーマ: `custom_emojis.created_by_user_id`、`custom_emoji_requests`・`custom_emoji_request_outcomes`・`custom_emoji_pending_requests` と sqlc、data-model §4.4 の表と「ローカルは管理者がアップロードした `media_files`」の記述（インスタンスの Actor の複製）
    - `internal/app/content`: 申請・取り下げ・承認・却下、作成者の検査付きの変更と削除、画像の申請と `lapsed`、shortcode の予約、ポート `CustomEmojiQuota` と上限なしの既定の実装（数も審査中の申請も制限しない。ADR-A の実装は #918 が差し込む）
    - 監査ログの `action` と `target_kind`、`moderation.AuditTargets` に申請の存在の問い合わせ、webhook のイベント
    - 通知 `admin_custom_emoji_request`・`custom_emoji_request_decided`（enum、Web Push、メール、`locales/`）
    - API（§6）、スコープ `read:custom_emojis`・`write:custom_emojis`、CLI `my admin emoji requests`
    - Web: `/settings/custom-emojis`（申請のフォーム・状態・使った数 / 上限・自分の絵文字の変更と削除）と `/admin/custom-emojis` の申請のタブ（画像の確認・承認・理由付きの却下）
  - #912（ADR-A）: プランのロールの許可リストを「区分が `member` の権限」として書き、`CustomEmojiQuota` の実装の元になるプランの上限の値を決める
  - #914（ADR-E）: `view_finance` に見せる収入の合計と、`plans` の人数を数える時点を決める
  - 実装の Issue で client-api.md・web-client.md §9・§14・data-model.md を更新する

## Pros and Cons of the Options（詳細比較・任意）

### 申請: R1 別のリソース（採用）/ R2 絵文字の行を「審査中」で作る

- R1 の良い点: 判断基準 1 を構造で満たす。既存の読み取りの経路を変えない。悪い点: テーブルが 3 つ増え、承認のときに絵文字を作り直す（画像の複製を含む）。
- R2 の最も強い論点: 申請と絵文字が同じ行なので、承認は状態を変えるだけで、画像の複製もいらない。shortcode の一意性も既存の制約がそのまま守る。
  悪い点: ピッカー・本文からの抽出・`GET /ap/emojis/{uuid}`・リアクション・`ExistingIDs`・連合の `Emoji` タグのすべてに「審査中を除く」を足す必要があり、1 か所の漏れで承認されていない画像が連合先に送られる。連合先に複製された画像は取り消せない。

### 作成者の変更: E1 画像だけ申請（採用）/ E2 すべて直接 / E3 すべて申請

- E2 の最も強い論点: 決定者の「自分の作った絵文字を除く」をそのまま読むと、作成者は自由に更新できる。審査の待ちがなく、実装も小さい。悪い点: 承認の後に画像を差し替えれば審査を素通りでき、承認制の意味がなくなる。
- E3 の良い点: 承認する人がすべての変更を見る。悪い点: 別名やカテゴリの変更まで待たせ、承認する人の手間が増える（判断基準 5）。別名とカテゴリはピッカーの検索と並びにしか効かない。

### 区分: K1 `member` を足す（採用）/ K2 `server` に入れる

- K2 の最も強い論点: 区分も名前空間も増えず、`/admin/finance` を読み取りだけで出せば画面も 1 つで済む。悪い点: 支払った利用者のナビゲーションに「管理」が出て、使える画面が 1 つだけの「管理」になる。
  スタッフでない人のトークンに `admin:*` を求め、FR-912 の許可リストを権限ごとの表で別に持つことになる。
- K1 の悪い点: 区分・画面・スコープが増え、`/admin/finance` と `/finance` の 2 つの画面が同じ集計を出す。

### 上限: Q1 ポート（採用）/ Q2 インスタンス設定 / Q3 ロールの属性

- Q2 の良い点: 実装が最も小さい。悪い点: プランごとに変えられない（決定者の回答はプランごと）。
- Q3 の最も強い論点: プランはロールに結び付くので、ロールに上限を持たせれば `identity` だけで決まり、手で付けたロールにも上限が効く。悪い点: 上限は課金のプランの属性（FR-910）で、ロールの属性にするとプランとロールの 2 か所に値ができる。
  ADR-A がロールの側に持つと決めれば、Q1 のポートの実装をそれに替えるだけで済む。

### 見せ方の設定: S1 インスタンス設定（採用）/ S2 `finance` の設定

- S1 の良い点: 公開範囲（`finance_visibility`）と同じく、サーバーが誰に何を見せるかを `manage_settings` を持つ人が 1 か所で決める（ADR-0067 §8 の理由と同じ）。
- S2 の最も強い論点: 項目の名前とメモを書いた本人（`manage_finance`）が、それを誰に見せるかを決められる。悪い点: 見せる範囲の設定が 2 つの権限に分かれ、ADR-0067 §8 と食い違う。

## Open Questions / 要検証

- 無し。
