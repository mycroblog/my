---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.custom-emojis_.requests.tsx", "app/web/src/features/admin/components/CustomEmojiRequests.tsx", "app/web/src/features/admin/components/CustomEmojiNav.tsx", "app/web/src/features/admin/hooks/useCustomEmojiRequests.ts", "app/web/src/features/admin/api/customEmojiRequests.ts"]
unavailable: {"審査中の申請を承認する": "the demo scenario makes no custom emoji request"}
---

## カスタム絵文字の申請（`/admin/custom-emojis/requests`）

`manage_custom_emojis` を持つ管理者が、`create_custom_emojis` を持つメンバーの絵文字の申請（新しい絵文字と画像の差し替え）を審査する、カスタム絵文字の画面のページ（ADR-0070 §2、[§14](../14-admin.md)）。
見出しの下のページの切り替えの「申請」で[カスタム絵文字の一覧](admin-custom-emojis.md)から来る。

### 1. 使う人と頻度

- 管理者。申請の通知が来たときに。月に数回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 審査中の申請を承認する | 3 |

メニューで画面を開く 1 つ、ページの切り替えの「申請」を押す 1 つ、行の「承認する」を押す 1 つ。申請の通知からは、通知のリンクの 1 つと「承認する」の 1 つ。

### 3. 情報の優先順位とまとまり

見出しの下にページの切り替え（「申請」が今のページで、審査中の数のバッジ）、その下に区画が 1 つ。

1. カスタム絵文字の申請: 説明、状態の `Select`（既定は審査中）、決めた結果、申請の表（新しい順。申請（画像と `:shortcode:`、内容、申請者）・状態（終わった日時と却下の理由）・申請した日時・「承認する」と「却下する」の 4 列）、前へ / 次へ。

### 4. 主な操作と保存の仕方

- 主な操作は行の「承認する」。押すとすぐ決まる。却下は理由を入れる確認のダイアログを経る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 承認する・却下する | その場。行が審査中の一覧から消え、フォーカスが次の行に移り、表の上に結果。決められない申請の理由も表の上 |
| 状態を選ぶ | その場。表を最初のページから読み直す（URL が変わる） |

### 6. 入口と出口

- 入口: 申請の通知（Web Push・メール・通知の一覧）。カスタム絵文字の一覧のページの切り替え。監査ログの申請の行。
- 出口: ページの切り替えの「絵文字」。申請者のプロフィール。
