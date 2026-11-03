# ADR-0072: 課金のプランとロールの連動

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #912

## Context（背景・課題）

M4.2（Supporters、epic #925）は、管理者が有効にしたときだけ、利用者が自分で課金のプランに加入し、支払いを続ける間プランに結び付いたロールを持つ機能を入れる（FR-910〜FR-912）。
ロールが運ぶ特典（バッジ・絵文字の作成・会計の閲覧）は [ADR-0070](0070-permission-split-emoji-finance.md) と [ADR-0071](0071-role-badges-and-supporter-list.md) が決めた。この ADR は、プランと加入と、加入の状態からロールを付け外しする規則を決める。

決定者の指示と回答（#925・#912、原文の抜粋）:

> サーバー管理者が有効にしており、プランの設定が成されていればユーザーは自分で課金ユーザーになることができ、月々の支払いを継続することでプランに紐付けられたロールになることができます。
>
> デフォルトの課金形態などは用意せず、あくまで管理者が有効化して導入する機能。
>
> 3\. 推奨: ロールは期間の終わりまで残します。支払いに失敗したら、猶予（たとえば7日）のあとに外します。 → これで OK。年払い → 年払いで何%割引くかを設定できるようにする
>
> 4\. 推奨: 課金のプランに結び付けられるロールの権限を、許可リストに限ります。最初は create_custom_emojis と view_finance だけです。position もモデレーターより下に固定します。 → これで OK
>
> 10\. 推奨: 加入できるのはローカルのユーザーで、メールの確認が済んでいる人だけにします。同時に加入できるプランは1つで、上位のプランは下位のプランの特典を含みます（ロールを1つだけ付ける形）。 → これで OK
>
> 1\. 特商法の表記などを出す場所（インスタンス設定のページ）は機能として用意します。ただ、法的な正しさを my が保証しないことを docs に書きます。 → これで OK
>
> 2\. 加入の操作は Web だけにし、Tauri のアプリには出しません。 → これで OK

spike の確認事項への決定者の回答（#912、原文）:

> 1\. Paddle を選択肢に残すか : 規約で認められないなら無理ですね。落とす
>
> 2\. 返金・チャージバックのとき: 推奨通りで OK
>
> 5\. プランを変えたときの日割り: 推奨案で OK

この ADR の論点への決定者の回答（#932、原文）:

> 1\. 凍結したときの加入（S1）: 推奨案で OK
>
> 2\. プランのロールを手で付けられなくすること（L1）: 手で付けられなくする
>
> 3\. そのほかの推奨案の全体: そのまま Accepted にしてよいか。 : OK
>
> ADR Accepted

前提になる事実と決定:

- spike #909 の[レポート](https://github.com/mycroblog/my/issues/909#issuecomment-6326974258)（2026-10-22 確認）。決済の事業者は Stripe だけにする（回答 1）。
  - 価格は金額を変えられないので、年払いの割引は年の価格を別に作り、金額が変わるときは新しい価格を作って古い価格をアーカイブする（§1.1）。
  - 事業者のイベントは順序が保証されず重複する。webhook は「変わった」という合図として受け、事業者の API からサブスクリプションを取り直して状態を導く（§4・§5）。
  - 事業者の再試行（dunning）の期間は最初の失敗から 7 日と一致しないので、猶予はこちらの Scheduler が持つ（§5）。
- 決済の事業者との境界（ポート `PaymentProvider`、webhook の受け口、設定と秘密、adapter）は ADR-0074（#913）が決める。この ADR はこのモジュールがポートに求めることを契約として書き（§17）、事業者の名前に依存しない。
- 収入の記録と公開は [ADR-0073](0073-income-records-and-publication.md)（#914）が決める。`finance` は加入のモジュールをポート `SupporterDirectory` を通して読む（ADR-0073 §1・§7）。
- ADR-0070: プランのロールの許可リストは「区分が `member` の権限」（§1）。絵文字の作成数の上限は `content` のポート `CustomEmojiQuota` で受け、課金が無効か、ロールがプランから来ていないときは上限なし（§4）。
- ADR-0071 §5 の契約: プランの一覧と「加入中」の定義をこのモジュールが持つ。掲載の選択 `supporter_listings` を持ち、加入が終わる処理と同じトランザクションで消す。プランの変更では、`identity` の「古いロールの剥奪と新しいロールの付与を 1 つのトランザクションで行い、バッジの掲出の選択を移す」操作を使う。
- ロール（FR-720、data-model §4.1）: 付与は `user_role_grants`、剥奪は `user_role_revocations`、投影 `user_roles` は `(user_id, role_id)` が主キーで、持っているロールの付与は何もしない。`moderator` の `position`（既定 100）と権限は管理者が変えられる。

関連: FR-008（アカウントの削除）、FR-704（監査ログ）、FR-720、FR-910〜FR-917、[ADR-0002](0002-modular-monolith-hexagonal.md)、[ADR-0004](0004-job-queue-abstraction.md)、[ADR-0012](0012-v0-breaking-change-policy.md)、[ADR-0020](0020-data-modeling-principles.md)、[ADR-0037](0037-audit-log.md)、[ADR-0058](0058-moderation-admin-separation.md)、[ADR-0067](0067-finance-cost-records.md)、[ADR-0068](0068-billing-import.md)。

決めること: モジュールと依存の向き、権限と有効化、プランの属性と事業者の価格、加入の条件、加入の状態の遷移とロールの規則、プランの変更・解約・価格の変更・非公開・廃止、アカウントの削除・凍結・BAN、安全柵、絵文字の上限、特商法の表記のページ、Web だけの加入、データモデル、監査ログ、API、ポートに求める契約。

## Decision Drivers（判断基準）

1. **特典は支払いの状態に正しく従う**: 払っていない人にロールが残らず、払っている人から外れない。イベントの順序・重複・取りこぼしで結果が変わらない（FR-911）
2. **お金でスタッフの権限を買えない**: プランのロールは `member` の権限だけを持ち、スタッフのどのロールより下にあり、どの操作の順でもこの不変条件が崩れない（FR-912）
3. **事業者に依存しない**: 状態の規則（猶予・期間の終わり・返金）はこちらが持ち、事業者のダッシュボードの設定に左右されない。ポートの向こうを替えても規則は変わらない
4. **管理者の操作で既存の加入者が不意に困らない**: 価格の変更・プランの非公開・廃止・課金の無効化で、同意のない値上げや、解約の手段のない請求が起きない
5. **既存の決定に乗る**: ADR-0002 のモジュール境界、ADR-0020 の分類、ADR-0037 の監査ログ、ADR-0058 の画面、ADR-0070・ADR-0071 の契約
6. **小さく始める**: 最初の実装（#918・#922）が小さく、あとから広げられる

## Considered Options（選択肢）

論点ごとに比べる。各論点の 1 つめを採用する。

- **モジュール**: M1 新しいモジュール `supporters` / M2 `finance` に置く / M3 `identity` に置く
- **権限**: P1 新しい権限 `manage_supporters`（区分 `server`） / P2 `manage_settings` / P3 `manage_finance`
- **プランのロールの守り方**: L1 `identity` が「プランが管理するロール」を記録し、手での付け外しと権限・位置の直接の変更を断る / L2 手での付与と共存させ、付与の ID でどちらの付与かを区別する / L3 `identity` がロールを変えるたびにポートで `supporters` に問い合わせる
- **状態の正**: T1 事業者のサブスクリプションを取り直し、そこから導いた状態が変わったときだけ遷移のイベントを記録する / T2 webhook のイベントを届いた順に状態へ当てる
- **猶予**: G1 最初の失敗から 7 日をこちらの Scheduler が数える / G2 事業者の再試行の終わり（解約）まで待つ
- **価格の変更と既存の加入者**: V1 既存の加入はもとの価格のまま、新しい価格は新しい加入とプランの変更から / V2 次の更新から新しい価格にする
- **プランの廃止**: R1 すべての加入を期間の終わりで解約する / R2 既存の加入は続ける（非公開と同じ）
- **課金の無効化**: D1 続いている加入があれば断る / D2 新しい加入だけを止める
- **アカウントの凍結**: S1 期間の終わりで解約を予約する / S2 何もしない / S3 すぐ解約する

## Decision（決定）

**M1・P1・L1・T1・G1・V1・R1・D1・S1 を採用する。** 事業者を Stripe だけにすること、返金・チャージバック（§7）と日割り（§8）の規則、凍結（S1）、プランのロールを手で付けられなくすること（L1）は、決定者の回答のとおりである。

### 1. モジュールと依存の向き（M1）

- 新しいモジュール `supporters`（`internal/domain/supporters`・`internal/app/supporters`）を置き、課金の有効化・プラン・事業者の価格・顧客・加入・加入の状態・猶予・掲載の選択（ADR-0071 の `supporter_listings`）を所有する。
- ポート `PaymentProvider`（§17）は `internal/app/supporters` に置き、adapter は `internal/adapter/payment/<事業者>`（ADR-0074 が決める）。
- 依存の向き（ADR-0002）:
  - `internal/app/supporters` は `internal/app/identity`（ロールの付け外し、管理されたロール、メールの確認）、`internal/app/account`（ローカルか、凍結・削除の状態）、`internal/app/instance`（特商法の表記の文章）を呼ぶ。
  - 監査ログは `AuditRecorded` のドメインイベントで `moderation` に渡す（ADR-0067 と同じ）。
  - `identity`・`account`・`content` は `supporters` を呼ばない。`content` の `CustomEmojiQuota` の実装（§11）と、`finance` の `SupporterDirectory` の実装（§12）は、`internal/platform` が `supporters` の公開 API から組み立てる。
  - アカウントの削除・凍結・BAN（§10）は、`account` と `moderation` のドメインイベントを outbox のジョブで受ける。
- overview §2 のモジュールの表、data-model §4.0 の所有の表、`.golangci.yml` の depguard に `supporters` を足す。

### 2. 権限と有効化（P1・D1）

- 新しい権限 `manage_supporters` を足す。区分は `server`（ADR-0058 §4）で、組み込みのロールでは `admin` だけが持つ。課金の有効化と無効化、プランの作成・変更・廃止、加入者と加入の状態の一覧、加入の解約（§10）に要る。
  個人ごとの支払いの額と収入は ADR-0073 のとおり `manage_finance` で見る。`manage_supporters` の画面は加入者・プラン・状態・期間の終わりを出し、金額の履歴を出さない。
- 管理画面は ADR-0058 §1 の表に「サーバー管理 | サポーター（`/admin/supporters`。課金の有効化、プランの一覧とフォーム、加入者の一覧） | `manage_supporters`」を足す。
- 課金は既定で無効。有効にする条件:
  - デプロイの設定で事業者が設定されている（ADR-0074。webhook の秘密を含む）
  - メールを送る構成である（`mail` が `none` でない。`none` ではメールの確認が済んだ人がいないため、加入の条件を誰も満たさない）
- 有効化と無効化は `supporter_program_changes`（§15）に、有効にしたときの事業者の名前と一緒に記録する。デプロイの設定の事業者が記録と違えば、新しい加入と変更を断り、管理画面に警告を出す（事業者は同時に 1 つ。#925 の回答 9）。
- **無効化は、続いている加入（§7 の `pending`・`active`・`canceling`・`past_due`・`lapsed`）が 1 つでもあれば `409` で断る（D1）。** 管理者は先にプランを廃止し（§9。すべての加入が期間の終わりで解約される）、加入が終わってから無効にする。
  無効のときは、利用者の画面・`/about`・API のどこにもプランと加入の導線を出さない（epic #925 の受け入れ基準）。ここで解約の手段を消しても請求が続かないのは、無効の時点で続いている加入がないからである。
- 事業者を替えるときは、無効にしてから新しい事業者で有効にする。既存の加入者は再加入する（#925 の回答 9。運営者向けの docs に書く）。

### 3. プラン

| 属性 | 型 | 変更 | 規則 |
| --- | --- | --- | --- |
| 名前 | `string(50)` | 版で変える | 1 行。管理者が書く文字列で、どのロケールでも同じ値を出す（インスタンスのルールと同じ）。利用者の内容として `dir="auto"` で描く |
| 説明 | `string(1000)` | 版で変える | 改行を含むプレーンテキスト。空でもよい |
| 月額 | 最小単位の整数（`bigint`、1 以上） | 版で変える | 事業者の最低の請求額（`MinimumAmount`、§17）以上 |
| 通貨 | ISO 4217 | 変えない | プランの作成で決まる。通貨を替えるときは新しいプランを作る |
| 年払いの割引率 | `int`（0〜90 の %） | 版で変える | 年額 = ⌊月額 × 12 × (100 − 割引率) / 100⌋（最小単位で切り捨て）。0 は割引なし |
| 結び付くロール | `roles.id` | 変えない | §5 の条件。1 つのロールは 1 つのプランにだけ結び付く |
| 絵文字の作成数の上限 | `int`（0〜10000） | 版で変える | ロールが `create_custom_emojis` を持つときに効く（§11） |
| 順位 | `int` | 版で変える | 廃止していないプランの間で一意。上下（§4）と並び順（小さい順）を兼ねる |
| 公開・非公開 | `enum(public, hidden)` | 版で変える | §9 |

- どのプランも月払いと年払いの両方を出す（FR-911）。年払いを出さないプランは持たない。
- 金額は ADR-0067 と同じく最小単位の整数で持ち、API では主単位の 10 進の文字列（`{"amount": "500", "currency": "JPY"}`）で受け渡す。
- **事業者の価格**: プランの作成と、月額か割引率の変更は、同じトランザクションで outbox にジョブ `supporters.ensure_prices` を積む。ジョブは月と年の金額を計算し、いまの価格（`supporter_plan_price_heads`）と金額が違う間隔について `EnsurePrice` で事業者に価格を作り（冪等キーはプラン・間隔・金額・通貨）、価格の行といまの価格を書き換え、古い価格を `ArchivePrice` でアーカイブする。
  月と年の両方の価格ができるまで、そのプランは加入を受け付けない（管理画面に「準備中」と出す）。ジョブが失敗し続ければ管理画面にその旨を出す。
- 管理者は事業者のダッシュボードで価格を作らない。このモジュールが作った価格だけを使う。

### 4. 上位は下位を含む（ロールは 1 つ）

- 利用者が持つプランのロールはいつも 1 つで、上位のプランのロールが下位のプランの特典をすべて持つ（#925 の回答 10）。そのために、廃止していないプランを順位で並べ、隣り合うどの 2 つ（下位 *i*、上位 *j*）についても次を満たすことを、プランの作成・変更・プランのロールの権限の変更のトランザクションで検査し、満たさなければ `422` にする。
  - ロールの `position`: *j* が *i* より高い
  - ロールの権限: *j* が *i* の権限をすべて含む
  - 絵文字の作成数の上限: 両方が `create_custom_emojis` を持つとき、*j* が *i* 以上
- 上位と下位は順位で決まり、プランの変更（§8）の上げる・下げるもこの順位で決まる。

### 5. 課金のロールの安全柵（L1）

- `identity` に現在値の表 `managed_roles`（§15）を足す。プランを作ると、同じトランザクションで結び付くロールを `managed_roles` に入れる（`identity` の公開 API の、トランザクションを受ける操作）。
- プランに結び付けられるロールの条件（プランの作成のときに検査し、満たさなければ `422`）:
  - 組み込みのロール（`admin`・`moderator`）でない、ほかのプランに結び付いていない
  - 持っている人がいない（手で付けた付与と混ざらないため）
  - 権限がすべて区分 `member`（ADR-0070 §1。いまは `create_custom_emojis` と `view_finance`）
  - `position` が、**スタッフのロール**（区分 `moderation` か `server` の権限を 1 つでも持つロール。`moderator` と `admin` を含む）のいちばん低い `position` より低い
- `identity` は `managed_roles` に入ったロールについて、次を `409`（`role_managed_by_plan`）で断る:
  - FR-720 の手での付与と剥奪（API・CLI・管理画面）。付け外しは `supporters` が §7 の規則でだけ行う
  - ロールの権限と `position` の直接の変更と、ロールの削除。権限と位置は `/admin/supporters` のプランのフォームから変え、`supporters` が §4 と上の条件を検査してから `identity` の操作を呼ぶ（監査ログは `role_updated`、操作者はフォームを使った人）
- `identity` は、管理されていないロールの変更が不変条件を壊すときも `409` で断る: スタッフのロールの `position` を管理されたロールのいちばん高い `position` 以下にする変更と、管理されたロールのいちばん高い `position` 以下にあるロールに `member` 以外の権限を足す変更。`identity` は自分の表だけでこれを判定できる。
- バッジ（ADR-0071 §1）は管理されたロールにもふつうに付けられる。
- **手で付けたロールとの関係（FR-720）**: プランのロールは手で付けられないので、プランのロールの付与はいつも加入から来る。プランのロールでないロール（たとえば管理者が作った「絵文字の係」に `create_custom_emojis` を持たせて手で付けたもの）は加入と独立に共存し、加入の終わりで外れない。
  スタッフが加入すれば、スタッフのロールとプランのロールの両方を持つ。
- プランを廃止し（§9）、そのロールを持つ人がいなくなったら、`supporters` がロールを `managed_roles` から外す。ロールはふつうのロールに戻り、管理者は削除できる。

### 6. 加入の条件と開始

- 加入できるのは、課金が有効で、次をすべて満たす利用者（#925 の回答 10）:
  - ローカルの利用者で、アカウントが凍結・削除・削除の予約（BAN）の状態でない
  - 主アドレスのメールの確認が済んでいる（`user_primary_emails` に行がある）
  - 続いている加入（§7）がない（同時に 1 つ。`supporter_live_subscriptions` の一意制約で守る）
- 加入の開始: 利用者がプランと間隔（月・年）を選び、§13 の最終の確認の画面を経て、`POST` で開始する。サーバーは 1 つのトランザクションで加入の行（状態 `pending`）を作り、トランザクションの外で `StartCheckout` を呼んで、Web クライアントが次に進む先（事業者の決済画面）を返す。
  こちらの加入の ID を事業者のサブスクリプションに載せ（§17）、取り直したスナップショットで照合する。2 回目以降は、記録した事業者の顧客（`supporter_customers`）を使う。メールの主アドレスを事業者に渡して入力を省く。
- 決済画面から戻った Web クライアントは、加入の状態を読み直す（webhook を待たずに、戻りの要求がジョブ `supporters.sync` を積む）。
- `pending` のまま 24 時間たった加入は、Scheduler が取り直して、事業者の側にもなければ `abandoned` で閉じる。`pending` の間にもう一度始めると、同じ加入の ID で決済画面を作り直す。

### 7. 加入の状態の遷移とロールの規則（T1・G1）

状態はこちらの規則で導く。webhook・決済画面からの戻り・Scheduler の期限・定期の照合は、どれも「この加入を取り直す」ジョブ `supporters.sync`（冪等、ADR-0004）を積むだけで、ジョブは `GetSubscription` のスナップショットと、こちらが記録した返金・チャージバックとアカウントの事実から状態を導く。
導いた状態か、ロールに効く属性（プラン・間隔・期間の終わり・予約された変更）が直前の遷移と違うときだけ、遷移のイベントを記録する。届く順序・重複・取りこぼしは結果を変えない（判断基準 1）。

| 状態 | ロール | 入る条件 | 期限（Scheduler） |
| --- | --- | --- | --- |
| `pending` 申し込み中 | なし | 加入の開始。事業者のサブスクリプションがまだないか、初回の支払いが済んでいない | 開始から 24 時間 → 取り直して `abandoned` |
| `active` 有効 | 付く | 事業者が有効（試用を含む）で、解約の予約がない | 期間の終わり → 取り直す（更新の取りこぼしを拾う） |
| `canceling` 解約の予約 | 期間の終わりまで付く | 有効で、期間の終わりの解約が予約されている | 期間の終わり → 取り直して `expired` |
| `past_due` 支払いの遅れ（猶予の内） | 付く | 事業者が支払いの遅れを示し、最初の失敗から 7 日たっていない | 最初の失敗 + 7 日 → 取り直して `lapsed` |
| `lapsed` 猶予の切れ | なし | 支払いの遅れが最初の失敗から 7 日を超えた（事業者はまだ再試行している）。一時停止もここに入る | なし（定期の照合が拾う） |
| `expired` 失効 | なし | 事業者の側でサブスクリプションが終わった | 終端 |
| `revoked` 返金・チャージバック | なし | 全額の返金かチャージバックを記録した | 終端 |
| `abandoned` 取りやめ | なし | 初回の支払いが済まないまま終わった | 終端 |

- **ロールの規則**: 状態が `active`・`canceling`・`past_due` の間はプランのロールが付き、ほかの状態では付かない。遷移を記録するトランザクションで `identity` の操作（付与・剥奪・入れ替え）を呼ぶ。付与と剥奪の操作者は `system` で、監査ログは `identity` が `role_granted`・`role_revoked` を書く。
  ロールの剥奪は ADR-0071 §2 のとおりバッジの掲出の選択を消し、ロールが外れる遷移は同じトランザクションで `supporter_listings` の行を消す（ADR-0071 §5）。
- **加入中の定義**（ADR-0071 §5）: 状態が `active`・`canceling`・`past_due` の加入。ロールが付いている期間と同じである。
- **解約**: 期間の終わりまでロールが残る（`canceling`）。期間の終わりの後は `expired` で外れる（FR-911）。
- **猶予（G1）**: 猶予の起点は、最後に成功した支払いより後の最初の失敗の時刻（スナップショットが持つ。持たなければ、こちらが最初に遅れを観測した時刻）。猶予は 7 日の定数で、事業者の再試行の設定と独立である。
  猶予の間に支払いが成功すれば `active` に戻る。`lapsed` の後に支払いが成功しても `active` に戻り、ロールをもう一度付ける（バッジと掲載の選択は既定の「出さない」から始まる）。
  運営者向けの docs に「事業者の再試行の期間を 7 日以上にし、最後は解約にする」と書く。
- **返金・チャージバック**（決定者の回答 2）:
  - 加入の支払いの**全額の返金**か**チャージバック**（dispute の開始）を記録したら、すぐ `revoked` にしてロールを外し、トランザクションの外で `CancelNow` で事業者のサブスクリプションもすぐ解約する（次の期間の請求を止める）。返金の額はこちらから返さない（返金は運営者が事業者の画面で行う）。
  - **一部の返金**ではロールを外さず、状態を変えない。
  - **dispute に勝っても**ロールを自動では戻さない。`revoked` は終端で、利用者は再加入する。
  - 返金とチャージバックの事実は、事業者の通知から `supporter_subscription_reversals`（§15）に、事業者の ID で一意に記録する。お金の額の記録（収入から引く）は ADR-0073 が決める。
- 定期の照合: ADR-0074 の定期ジョブ `supporters.reconcile_payments` が、前回の時刻から変わったサブスクリプションを `ListSubscriptions` で列挙し、それぞれを `supporters.sync` に渡す。webhook を取りこぼしても追いつく。周期は ADR-0074 が事業者のイベントの保持期間より短く決める。

### 8. プランの変更と解約

- 変更・解約・解約の取り消しは、このサーバーの画面と API から行い（規則をこちらに保つため）、ポートの命令（§17）で事業者に伝える。事業者の顧客ポータルは支払い方法と請求書のためにだけ開き、ポータルでのプランの変更は使わない（ADR-0074 がポータルの設定で無効にする）。ポータルでの解約は期間の終わりの解約だけに限り、スナップショットで `canceling` として観測する。
- プランの変更ができるのは `active` のときだけ（`canceling` は先に解約を取り消す。`past_due` と `lapsed` は先に支払い方法を直す）。変更先は公開のプランだけで、いまの価格を使う。
- **日割り**（決定者の回答 5）:
  - **上げる変更**（順位が上のプランへ、または月払いから年払いへ）: すぐに変え、差額を日割りですぐ請求する（`ChangePrice` の `now_prorated`）。新しいプランのロールへの入れ替えは、スナップショットが新しい価格を示したときに行う。日割りの請求が失敗して事業者が支払いの遅れを示せば、新しいプランで `past_due` になり、§7 の猶予に入る。
  - **下げる変更**（順位が下のプランへ）と**年払いから月払いへ**: 期間の終わりに変える（`period_end`）。それまではいまのプランのロールのままで、予約された変更を画面に出し、利用者は予約を取り消せる。
  - 同じ順位で間隔だけ変えるときも上の規則に従う。上げる変更と下げる変更が同時に起きる組（上位の月払いから下位の年払いへ、など）は、順位で判断する（下げる変更として期間の終わり）。
- ロールの入れ替えは ADR-0071 §2 の `identity` の操作（古いロールの剥奪と新しいロールの付与を 1 つのトランザクションで行い、バッジの掲出の選択を移す）を使う。掲載の選択（`supporter_listings`）は利用者ごとなので残る。
- もとの価格（§9 の据え置き）で続けている利用者がプランか間隔を変えると、変更先のいまの価格になり、同じプランに戻ってももとの価格には戻らない。画面に書く。

### 9. 価格の変更・非公開・廃止（V1・R1）

- **価格の変更（V1）**: 月額か割引率を変えると、新しい価格は新しい加入とプランの変更から使い、既存の加入はもとの価格のまま更新を続ける（同意のない値上げをしない。判断基準 4）。古い価格は事業者でアーカイブされ、新しい加入に使えなくなるが、既存のサブスクリプションは続く（§17 の契約）。
  既存の加入者を新しい価格に移す操作は持たない。移したいときは、新しいプランを作って古いプランを廃止する。
- **非公開**（`hidden`）: 新しい加入とそのプランへの変更を受け付けず、加入の画面と `/about` に出さない。既存の加入は続き、参加者の一覧（ADR-0071）にも出る。公開に戻せる。
- **廃止（R1）**: 戻せない。廃止は、同じトランザクションで廃止のイベントを記録し、そのプランの続いている加入それぞれにジョブを積む: `active` と `past_due` は `CancelAtPeriodEnd`（期間の終わりまでロールが残る）、予約された下げる変更は取り消してから同じく解約、`lapsed` は `CancelNow`（ロールはすでにない）、`pending` は `abandoned` で閉じる。
  加入者には通知（§16）で知らせる。廃止したプランは、管理画面の一覧の「廃止」の欄にだけ出て、加入の画面・参加者の一覧（ADR-0071 §5）・ADR-0070 の `plans` の人数に出ない。最後の加入が終わったら、ロールを `managed_roles` から外す（§5）。
- プランは削除しない。廃止したプランの行は加入・遷移・収入の記録が参照する。

### 10. アカウントの削除・凍結・BAN（S1）

- **アカウントの削除**（本人による削除と、BAN の削除の予約。`account_lifecycle_events` の `deleted`・`deletion_scheduled`）: ジョブが続いている加入を `CancelNow` ですぐ解約し、`expired`（理由 `account_closed`）にしてロールを外し、`supporter_listings` を消す。
  返金は自動では行わない。運営者が必要と判断すれば事業者の画面で返金し、返金は §7 のとおり記録される。BAN を覆しても加入は戻らない（利用者は再加入する）。
- **凍結**（`moderation.account_suspension_changed` で凍結が始まったとき）: ジョブが `CancelAtPeriodEnd` で期間の終わりの解約を予約する（S1）。凍結されたアカウントは特典を使えないので、次の期間の請求を止める。ロールは §7 の規則のまま期間の終わりまで残る（凍結の間は使えない）。
  凍結が解かれても自動では戻さず、本人が期間の終わりまでに解約を取り消すか、再加入する。
- 管理者（`manage_supporters`）は、管理画面から 1 つの加入をすぐ解約できる（`CancelNow`、理由 `staff`）。返金は伴わない。監査ログに残す。
- アカウントの消去（ADR-0020 規約 6）は、`supporter_listings` と `supporter_customers`（事業者の顧客との結び付き）の行を消す。加入・遷移・返金の記録は、会計の記録（ADR-0073）と事業者との照合のために残し、data-model の消去の表の「残すもの」に足す。
  加入の行の `user_id` は消去の後も残る `users` の識別子を指すだけで、名前・ハンドル・メールアドレスは引けない。`SupporterDirectory` の利用者の引き当て（§12）は、消去されたアカウントを「削除されたアカウント」として返す（ADR-0070 §3 の作成者と同じ）。

### 11. 絵文字の作成数の上限（ADR-0070 §4）

`content.CustomEmojiQuota.Limit(userID)` の実装を `supporters` の公開 API で組み立てる。

1. 利用者が、管理されていないロール（手で付けたロール）から `create_custom_emojis` を持つ → 上限なし（ADR-0070 の決定者の回答 2）
2. そうでなく、いま持っているプランのロールが `create_custom_emojis` を持つ → そのプランのいまの版の上限
3. それ以外（課金が無効を含む） → 上限なし（`content` は権限を別に検査するので、権限がなければ作れない）

上限を下げても（プランの変更・下げる変更・プランの版）、既存の絵文字は残る（ADR-0070 §4）。

### 12. ADR-0073 の `SupporterDirectory` と ADR-0071 との契約

**加入中の定義**は、状態が `active`・`canceling`・`past_due` の加入である（§7。ロールが付いている期間と同じ）。ADR-0071 の参加者の一覧、ADR-0070 の `plans` の人数、ADR-0073 の `SupporterDirectory` はどれもこの定義で数え、`pending`・`lapsed`・終端の状態を数えない。

`finance` のポート `SupporterDirectory`（ADR-0073）を、`internal/platform` が `supporters` の公開 API で実装する。

- `PlanCounts(ctx)`: 廃止していないプランと、加入中の加入が残る廃止したプランごとに、プランの ID・名前・加入中の数を返す。`supporter_subscription_heads` の `(plan_id, state)` のインデックスで数える。
- 加入から利用者への引き当て: 加入の ID の集合を受け、それぞれの利用者の ID・加入のプラン・アカウントが消去されたかを、`supporter_subscriptions` から 1 回の問い合わせで返す。知らない ID は返さない。`manage_finance` の管理画面だけが使う（ADR-0073 §6）。
- 課金が無効なら、どちらも空を返す。

ADR-0071 との契約:

- プランの一覧（`id`・名前・順位・結び付くロール）を公開 API で読ませる。廃止したプランは出さない（§9）。
- 上位のプランの人は、持っているロールが 1 つなので上位のプランの見出しにだけ出る。
- `supporter_listings` はこのモジュールが持ち、ロールが外れる遷移と同じトランザクションで消す。プランの変更では消さない。
- プランの変更は `identity` の選択を移す操作を使う（§8）。

### 13. 特商法の表記などのページ

- `instance` の文章のキー（`instance_text_revisions.key`）に `commerce_disclosure` を足す。`terms`・`privacy_policy` と同じ形式・同じ権限（`manage_settings`）で書き、どのロケールでも同じ値を出す。
- Web は `/about/commerce` に出し、`/about`・加入の画面・最終の確認の画面からリンクする。文章が空ならページは `404` で、リンクを出さない。運営者は事業者のダッシュボードにこの URL を登録できる（spike §1.9）。
- 課金の有効化は文章の有無を条件にしない（運営者の国の法令をサーバーは知らない）。空のまま有効にすると、管理画面に警告を出す。
- 加入を始める直前の確認の画面に、プランの名前・間隔ごとの金額（更新の後も同じであること）・支払いの時期（いますぐ、以降は期間ごと）・特典が始まる時期・解約の方法と効果（期間の終わりまで特典が続く）・特商法の表記へのリンクを出す。
- **法的な正しさを my は保証しない**ことを、運営者向けの docs（`docs/operations/supporters.md`）と管理画面の文章の欄の説明に書く（#925 の回答 1）。docs には spike §9 の事項（表示事項、最終確認画面、消費税とインボイス）を、運営者が確かめる事項として要検証のまま挙げる。

### 14. 加入の操作は Web だけ

- 加入・プランの変更・解約・解約の取り消し・顧客ポータルの画面（`/settings/supporter`）と、`/about` のプランの区画は、Web クライアントが Tauri のアプリとして動くときは出さない。購入への導線も、Web で加入できることの案内も出さない（#925 の回答 2、spike §10）。
- 特典（プロフィールのバッジ、`/settings/custom-emojis`、`/finance`）はアプリでも使える。Google Play は購入の導線のないアプリでほかで買った特典を使うことを認めている。Apple で同じ形が審査に通るかは要検証（Open Questions）。
- API はクライアントを区別しない（区別はクライアントの規則で、サーバーは強制できない）。

### 15. データモデル（ADR-0020）

`supporters` が持つ（`managed_roles` だけ `identity`）。

| テーブル | 分類 | カラム | 制約・規則 |
| --- | --- | --- | --- |
| `supporter_program_changes` | イベント | `id`, `state`（`enum(enabled, disabled)`）, `provider`（`string(32)`。`enabled` のときの事業者）, `changed_by_user_id`（NULL 可 = CLI）, `changed_at` | 最新の行が今の状態。行がなければ無効 |
| `supporter_plans` | リソース | `id`, `role_id`, `currency`（`string(3)`）, `created_by_user_id`（NULL 可）, `created_at` | 一意 `(role_id)` |
| `supporter_plan_revisions` | イベント（版） | `id`, `plan_id`, `name`（`string(50)`）, `description`（`string(1000)`）, `rank`（`int`）, `monthly_amount_minor`（`bigint`）, `annual_discount_percent`（`int`、0〜90。`CHECK`）, `emoji_limit`（`int`、0〜10000）, `visibility`（`enum(public, hidden)`）, `revised_by_user_id`（NULL 可）, `revised_at` | インデックス `(plan_id, id DESC)` |
| `supporter_plan_retirements` | イベント | `plan_id`（PK）, `retired_by_user_id`（NULL 可）, `retired_at` | |
| `supporter_plan_heads` | 投影 | `plan_id`（PK）, `revision_id`, `rank`, `visibility` | 廃止していないプランだけ。一意 `(rank)` |
| `supporter_plan_prices` | リソース | `id`, `plan_id`, `interval`（`enum(month, year)`）, `amount_minor`, `currency`, `provider`, `provider_price_id`, `created_at` | 一意 `(provider, provider_price_id)` |
| `supporter_plan_price_heads` | 投影 | `plan_id`, `interval`, `price_id` | PK `(plan_id, interval)`。新しい加入と変更に使う価格 |
| `supporter_plan_price_archivals` | イベント | `price_id`（PK）, `archived_at` | 事業者でアーカイブした価格 |
| `supporter_customers` | リソース | `id`, `user_id`, `provider`, `provider_customer_id`, `created_at` | 一意 `(provider, user_id)`, `(provider, provider_customer_id)` |
| `supporter_subscriptions` | リソース | `id`（事業者に載せるこちらの ID）, `user_id`, `plan_id`（開始のときのプラン）, `price_id`, `provider`, `created_at` | インデックス `(user_id, id DESC)` |
| `supporter_subscription_links` | イベント | `subscription_id`（PK）, `provider`, `provider_subscription_id`, `linked_at` | 一意 `(provider, provider_subscription_id)`。初めてスナップショットを得たときに 1 回 |
| `supporter_subscription_transitions` | イベント（版） | `id`, `subscription_id`, `state`（`enum(pending, active, canceling, past_due, lapsed, expired, revoked, abandoned)`）, `plan_id`, `price_id`, `period_ends_at`（`pending`・`abandoned` 以外は値を持つ。`CHECK`）, `reason`（`enum(started, synced, grace_expired, reversed, plan_retired, account_closed, account_suspended, staff)`）, `occurred_at` | インデックス `(subscription_id, id DESC)` |
| `supporter_grace_periods` | イベント | `transition_id`（PK。`past_due` の遷移）, `failed_at`, `ends_at` | `ends_at = failed_at + 7 日` |
| `supporter_scheduled_changes` | イベント | `transition_id`（PK）, `price_id`, `effective_at` | その遷移の時点で予約されている下げる変更。行がなければ予約なし |
| `supporter_subscription_reversals` | イベント | `id`, `subscription_id`, `kind`（`enum(full_refund, partial_refund, chargeback)`）, `provider`, `provider_reversal_id`, `occurred_at` | 一意 `(provider, provider_reversal_id)` |
| `supporter_subscription_heads` | 投影 | `subscription_id`（PK）, `transition_id`, `user_id`, `plan_id`, `state` | インデックス `(plan_id, state)`、`(user_id)` |
| `supporter_live_subscriptions` | 投影 | `user_id`（PK）, `subscription_id` | 状態が終端でない加入だけ。同時に 1 つを一意制約で守る |
| `supporter_subscription_deadlines` | 投影 | `subscription_id`（PK）, `kind`（`enum(checkout, period_end, grace)`）, `due_at` | インデックス `(due_at)`。Scheduler が期限の来た行に `supporters.sync` を積む |
| `supporter_listings` | 現在値 | `user_id`（PK）, `created_at` | ADR-0071 §6 |
| `managed_roles`（`identity`） | 現在値 | `role_id`（PK）, `manager`（`enum(supporters)`）, `created_at` | 行がある = プランが管理するロール（§5） |

- 状態・廃止・猶予・予約を NULL 可の時刻の列で表さない（ADR-0020 規約 4）。今の状態は遷移の最新の版と投影、猶予と予約は遷移に付く子のイベントの行の有無で表す。
- 投影はすべて同期で、イベントの表から再構築できる（`my projection verify`）。
- 事業者の生のスナップショットとイベントの本文は保存しない。webhook の受信の記録（`(provider, event_id)` の一意）は ADR-0074 が決める。
- 金額の動き（支払い・返金・手数料）の記録は ADR-0073 が決める。この ADR の表は加入の状態に効く事実だけを持つ。

### 16. 監査ログと通知

- **監査ログ（ADR-0037 への追加）**: `target_kind` に `supporter_program`・`supporter_plan`・`supporter_subscription` を足し、`action` に次を足す。操作者はスタッフか CLI。
  - `supporter_program_enabled`・`supporter_program_disabled`（`target_label` は事業者の名前）
  - `supporter_plan_created`・`supporter_plan_revised`・`supporter_plan_retired`（`target_label` はプランの名前）
  - `supporter_subscription_canceled`（§10 の管理者による解約。`target_label` はプランの名前と利用者のハンドル）
- 利用者自身の加入・変更・解約と、事業者の状態からの遷移はスタッフの操作でないので監査ログに書かない（遷移のイベントに残る）。ロールの付け外しは `identity` が `role_granted`・`role_revoked` を操作者 `system` で書く。プランのフォームからのロールの権限と位置の変更は `role_updated` で残る。特商法の表記の変更は既存の `text_changed` で残る。
- **通知**（`client*` 以外の名前空間でも ADR-0009 の制約のとおり、`locales/en`・`locales/ja`）:
  - `supporter_payment_failed`: `past_due` に入ったとき本人に。猶予の終わりの日時と、支払い方法を直す画面へのリンク
  - `supporter_subscription_ended`: `lapsed`・`expired`・`revoked` に入ったとき本人に（理由を含む）
  - `supporter_plan_retired`: 廃止したプランの加入者に。期間の終わりで解約されること
  - メールでも送る（メールを送る構成が有効化の条件なので、いつも送れる）。Tauri のアプリでは通知の文面とリンクの先が加入の画面にならないようにする（§14）

### 17. ポート `PaymentProvider` に求める契約（ADR-0074 が満たす）

この ADR はポートの形と adapter を決めない。spike §4 の形はこの契約を満たす。型は標準ライブラリだけで、事業者の SDK の型を出さない（ADR-0068 と同じ）。

| 必要なこと | 契約 |
| --- | --- |
| 価格 | `EnsurePrice` は冪等で、同じプラン・間隔・金額・通貨に同じ価格を返し、要らなくなった価格をアーカイブする。アーカイブした価格の既存のサブスクリプションは更新を続ける。`MinimumAmount(通貨)` が最低の請求額を返す |
| 決済の開始 | `StartCheckout(こちらの加入の ID・顧客か メール・価格・ロケール・戻り先)` が、Web クライアントが次に進む先（リダイレクトの URL）を返す。こちらの加入の ID を事業者のサブスクリプションに載せ、スナップショットで返す。カード情報はこのサーバーを通らない |
| ポータル | 支払い方法と請求書の画面の URL を返す。ポータルでのプランの変更は無効、解約は期間の終わりだけにする |
| 命令 | `ChangePrice(サブスクリプション・価格・時期 {now_prorated, period_end})`、`CancelAtPeriodEnd`、`ResumeCancellation`、`CancelScheduledChange`（期間の終わりに予約した変更の取り下げ）、`CancelNow`（返金しない）。どれもこちらの ID から作る冪等キーで、2 回呼んでも 1 回と同じ |
| 通知 | webhook の検証の後、`(事業者, event_id)` で重複を除き、「どの加入を取り直すか」（事業者のサブスクリプション）と、お金の動きの ID を返す。順序を仮定しない |
| お金の動き | 返金・チャージバックを取り直すと、種類（返金・チャージバック・チャージバックの取り消し）・対象のサブスクリプション・返金の額と元の支払いの額（`MoneyMovement` の `Gross` と `ParentGross`。全額か一部かをこちらが判断する）・事業者の ID・確定したか（`Final`）が分かる。確定した動きだけで状態を変え、同じ動きを ADR-0073 の `RecordIncome` にも渡す |
| 正 | `GetSubscription` のスナップショット: こちらの加入の ID、顧客、価格、正規化した状態（`incomplete`・`active`・`past_due`・`unpaid`・`paused`・`canceled`・`incomplete_expired`。試用は `active`）、期間の始まりと終わり、期間の終わりの解約の予約、期間の終わりに予約された価格、最後の成功の後の支払いの失敗の時刻（なければなし） |
| 照合 | `ListSubscriptions(時刻・カーソル)` が、その時刻から変わったサブスクリプションを列挙する |
| 失敗 | 一時的な失敗（再試行する）と恒久的な失敗（管理画面に出す）を区別する |

- 正規化した状態から §7 の状態への写しはこのモジュールが持つ: `incomplete` → `pending`、`incomplete_expired` → `abandoned`、`active` → `active` か `canceling`（解約の予約で分ける）、`past_due` と `unpaid` → 猶予の内なら `past_due`、外なら `lapsed`、`paused` → `lapsed`、`canceled` → `expired`。
- webhook のジョブと定期の照合のジョブは、取り直したスナップショットをこのモジュールのユースケース（スナップショットを当てる、冪等で順序に依らない）に渡す。それが §7 の `supporters.sync` である。
- 外向きの HTTP の分類、設定と秘密、webhook の受け口の経路、fake と契約テストは ADR-0074 が決める。

### 18. API の形（[client-api.md](../architecture/client-api.md) の手順で細部を決める）

- 利用者（タグ `supporters`、新しいスコープ `read:supporters` / `write:supporters`。`read` / `write` に含む）:
  `GET /supporter/subscription`（自分の加入: 状態・プラン・間隔・期間の終わり・予約・猶予の終わり）、`POST /supporter/checkout`（`{plan_id, interval}`。条件を満たさなければ `422`、続いている加入があれば `409`）、
  `POST /supporter/subscription/change`（`{plan_id, interval}`）、`POST /supporter/subscription/cancel`、`POST /supporter/subscription/resume`、`POST /supporter/portal`（ポータルの URL）
- 公開（タグ `instance`、`security: []`）: `GET /instance/supporter_plans`（公開のプラン: 名前・説明・月額・年額・順位）。無効なら `404`。`GET /instance` に `supporters_enabled` を足す。
- 管理（タグ `admin-server`、`admin:read` / `admin:write` と `manage_supporters`）: `GET/PUT /admin/supporter_program`、`GET/POST /admin/supporter_plans`、`PATCH /admin/supporter_plans/{id}`（ロールの権限と位置を含む）、`POST /admin/supporter_plans/{id}/retire`、
  `GET /admin/supporter_subscriptions`（状態とプランで絞り込む。キーセットページング）、`POST /admin/supporter_subscriptions/{id}/end`
- CLI: `my admin supporters`（有効化・無効化・プランの一覧・加入の一覧と解約）。

### この決定を覆す条件

- 加入者から猶予の長さを延ばしたい声が多いか、事業者の再試行で後から払われる例が多いと分かったら、猶予の長さをインスタンス設定にする。
- 既存の加入者を新しい価格に移す需要が強いと分かったら、通知と同意を伴う移行の操作を別の ADR で決める（V2）。
- 1 人が複数のプランを同時に支援したいという要望が強いと分かったら、「同時に 1 つ」と「上位は下位を含む」を見直す（ロールを複数付ける形）。
- `member` の区分に、他人に効く権限を入れたくなったら、§5 の安全柵を見直す（ADR-0070 の覆す条件と同じ）。

## Consequences（影響）

- 良い影響
  - 状態は事業者のスナップショットとこちらの事実からいつも同じ規則で導くので、webhook の順序・重複・取りこぼしと、事業者の再試行の設定がロールの結果を変えない。
  - プランのロールは `identity` の表 1 つで守られ、手での付け外し・権限と位置の変更・スタッフのロールの変更のどの順でも、お金でスタッフの権限に届かない。
  - 値上げは既存の加入者に効かず、廃止と無効化で解約の手段のない請求が残らない。
  - ADR-0070 の上限と ADR-0071 の一覧・バッジの契約を、プランの属性と加入中の定義 1 つで満たす。
- 悪い影響・トレードオフ
  - テーブルが 19 増え（`identity` の 1 つを含む）、遷移を導く規則と Scheduler の期限、事業者へのジョブが増える。
  - プランのロールは手で付けられず、管理者が特定の人に特典を無償で渡すには、プランでない別のロールを作って付ける必要がある（上限なしになる）。
  - 価格を据え置くので、古い価格の加入が長く残り、同じプランに金額の違う加入者が並ぶ。収入の集計（ADR-0073）はプランごとの金額を加入の価格から読む必要がある。
  - 猶予の後もしばらく事業者が再試行し、`lapsed` の利用者はロールがないのに後から請求されうる。通知と docs で、支払い方法を直すか解約するよう案内する。
  - 凍結で自動的に解約が予約されるので、短い凍結が解かれた後に本人が解約を取り消し忘れると加入が終わる。
  - `mail = none` のサーバーでは課金を有効にできない。
  - 上位は下位を含むという検査のため、管理者はプランのロールの権限と位置をプランのフォームから変える必要があり、`/admin/roles` からは変えられない。
  - 加入を Web だけにしても、Apple の審査がアプリで特典を使う形を認めるかは確かめていない。
- 後続で必要になる作業
  - #918（プランと加入の core）:
    - `internal/domain/supporters`・`internal/app/supporters`、overview §2・data-model §4.0 と新しい §4.x・depguard、`schema/<dialect>.sql` と sqlc（§15 の表と `identity` の `managed_roles`）
    - `identity`: `PermManageSupporters`（`server`）と `role_permissions.permission` の enum、`managed_roles` の操作と §5 の `409` の検査、`system` の操作者でのトランザクションを受ける付与・剥奪（ADR-0071 の入れ替えの操作は #917 が足す）、FR-720 の文と data-model §4.1 の規則
    - 状態の導出（§7 の表の表駆動のテスト: 順序の入れ替わり・重複・取りこぼし・猶予の境界・`lapsed` からの復帰・返金の全額と一部・dispute）、`supporters.sync`・`supporters.ensure_prices` のジョブ、Scheduler の期限と定期の照合
    - プランの検査（§4・§5）、価格の変更・非公開・廃止（§9）、無効化の `409`（§2）、アカウントの削除・凍結・BAN のジョブ（§10）、消去の表
    - `CustomEmojiQuota` の実装（§11）を `internal/platform` で差し込む（#916 のポートと既定の実装が先）
    - `SupporterDirectory` の実装（§12。`PlanCounts` と加入から利用者への引き当て）を `internal/platform` で差し込む（#923 のポートと空の実装が先か、#918 がポートを足す）
    - 監査ログの `target_kind`・`action`、通知 3 種（enum・Web Push・メール・`locales/`）
    - 管理の API・CLI と `/admin/supporters` の画面、`instance` の文章のキー `commerce_disclosure` と `/about/commerce`
    - ポートの fake で全体を確かめる（Stripe の adapter は #920）
  - #922（加入・解約の画面と参加者の一覧）:
    - `/settings/supporter`（プランの選択・最終の確認の画面・決済画面への移動と戻り・状態・変更と予約の取り消し・解約と取り消し・ポータル）、`/about` のプランの区画、Tauri のときに出さない規則
    - 利用者と公開の API（§18）とスコープ `read:supporters`・`write:supporters`（`knownScopes`・spec・同意画面の文言）
    - ADR-0071 の参加者の一覧（`supporter_listings` は #918 が作り、消す処理は §7 の遷移に入る）
  - #916: `CustomEmojiQuota` の既定の実装（上限なし）を先に入れ、#918 が §11 の実装に差し替える。
  - #919・#920（ADR-0074）: §17 の契約を満たすポートと adapter、ポータルの設定（プランの変更を無効、解約は期間の終わり）、取り直したスナップショットを `supporters.sync` に渡すジョブ。
  - #923（ADR-0073）: `SupporterDirectory` のポートと空の実装。人数は §12 の加入中の定義で数える。
  - `docs/operations/supporters.md`: 有効化の手順、事業者の再試行の設定、事業者の切り替えと再加入、返金の扱い、法的な正しさを保証しないことと運営者が確かめる事項。

## Pros and Cons of the Options（詳細比較・任意）

### モジュール: M1 `supporters`（採用）/ M2 `finance` / M3 `identity`

- M1 の良い点: 加入の状態・ポート・定期ジョブが 1 つにまとまり、`finance` は費用と収入の記録、`identity` はロールの仕組みのままでいられる。悪い点: モジュールが増え、`identity` と `account` への呼び出しが増える。
- M2 の最も強い論点: 収入（ADR-0073）と同じ事業者のデータを扱い、事業者の取り込みのポート（ADR-0068）と並ぶ。悪い点: 会計のモジュールがロールの付け外しとアカウントの状態に依存し、`manage_finance` の範囲と混ざる。
- M3 の良い点: ロールの付け外しがモジュールの中で済む。悪い点: 認証とロールのモジュールに決済の事業者と Scheduler の規則が入る。

### 権限: P1 `manage_supporters`（採用）/ P2 `manage_settings` / P3 `manage_finance`

- P2 の最も強い論点: 有効化はサーバーの設定の 1 つで、権限が増えない。悪い点: 設定を任された人が、ロールの権限（プランのロール）とお金の流れを同時に動かせる。
- P3 の良い点: お金の担当がプランも扱う。悪い点: `manage_finance` は記録の権限（ADR-0067）で、ロールと加入者の管理が入ると範囲が広がる。

### ロールの守り方: L1 管理されたロール（採用）/ L2 共存 / L3 ポート

- L2 の最も強い論点: 管理者がプランのロールを特定の人に無償で付けられ、FR-720 の操作がそのまま使える。悪い点: `user_roles` は 1 つのロールに 1 つの付与しか持たないので、加入の終わりに剥奪すると手での付与も消えるか、付与の出どころを `identity` に持ち込むことになる。手で付けた人の上限（ADR-0070 は上限なし）と、プランのロールでの上限が同じロールで食い違う。
- L3 の良い点: `identity` に表が増えない。悪い点: `identity` のロールの変更のたびに別のモジュールへの問い合わせが要り、組み立ての漏れで検査が抜ける。

### 状態の正: T1 取り直して導く（採用）/ T2 イベントを順に当てる

- T2 の最も強い論点: 事業者の API への取り直しがなく、イベントの本文だけで速く決まる。悪い点: 両社とも順序を保証しないと明記しており、遅れて届いた古いイベントでロールが戻る。取りこぼしは照合でしか直せない。

### 猶予: G1 こちらで 7 日（採用）/ G2 事業者の再試行の終わり

- G2 の良い点: 実装が小さく、事業者の解約と同時にロールが外れる。悪い点: 再試行の期間は事業者と運営者の設定で 1 週間〜2 か月と変わり、決定者の 7 日と一致しない（spike §5）。

### 価格の変更: V1 据え置き（採用）/ V2 次の更新から

- V2 の最も強い論点: 同じプランの加入者がいつも同じ金額で、収入の集計と画面が単純。悪い点: 同意のない値上げになり、事前の通知と同意の仕組み（と、法令の確認）が要る。

### 廃止: R1 期間の終わりで解約（採用）/ R2 既存は続ける

- R2 の良い点: 加入者が何もしなくてよい。悪い点: 非公開（`hidden`）と同じになり、廃止したプランのロールがいつまでも管理されたまま残る。参加者の一覧に出さない（ADR-0071）のに特典は続く。

### 無効化: D1 続いている加入があれば断る（採用）/ D2 新規だけ止める

- D2 の最も強い論点: 管理者はいつでもすぐ止められる。悪い点: 「無効のときは何も出ない」と両立させると、続いている加入の解約の画面も消え、請求だけが続く。

### 凍結: S1 解約を予約（採用）/ S2 何もしない / S3 すぐ解約

- S2 の良い点: 短い凍結が解かれれば元どおり。悪い点: 使えない特典に請求が続く。
- S3 の良い点: 請求も特典もすぐ止まる。悪い点: 凍結は覆ることがあり、払った期間の特典まで失う。

## Open Questions / 要検証

- 要検証: Stripe でアーカイブした価格の既存のサブスクリプションが更新を続けること（spike は「使ったことのある価格は削除できない」までを確かめた）。#920 の adapter の契約テストと、#924 のテストモードでの確認で確かめる。
- 要検証: §13 の確認の画面と事業者の決済画面で、特商法 12 条の 6 の最終確認画面の表示を満たせるか（spike §9 A3）。#924 のテストモードで、Stripe の決済画面に出る項目を確かめる。満たすかの判断は運営者が行い、my は保証しない。
- 要検証: Apple の審査が、購入の導線のないアプリで Web で買った特典を使わせる形を認めるか（spike §10）。Tauri のアプリの配布（M5）の前に確かめる。
