---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.custom-emojis_.new.tsx", "app/web/src/features/admin/components/NewCustomEmoji.tsx", "app/web/src/features/admin/hooks/useCustomEmojis.ts", "app/web/src/features/admin/api/customEmojis.ts"]
unavailable: {"カスタム絵文字を登録する（画像の選択は 1 つと数える）": "the agent has no file to choose: the browser tools give it none"}
---

## カスタム絵文字の登録（`/admin/custom-emojis/new`）

`manage_custom_emojis` を持つ管理者が、画像を選んでこのサーバーのカスタム絵文字を登録するページ（FR-111、ADR-0051、[§14](../14-admin.md)）。
[カスタム絵文字の一覧](admin-custom-emojis.md)の「カスタム絵文字を登録する」から来て、登録したらその[絵文字のページ](admin-custom-emoji.md)へ移る。

### 1. 使う人と頻度

- 管理者。絵文字を足すときに。月に数回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| カスタム絵文字を登録する（画像の選択は 1 つと数える） | 5 |

メニューで画面を開く 1 つ、「カスタム絵文字を登録する」を押す 1 つ、画像を選ぶ 1 つ、shortcode を入れる 1 つ、「登録する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

見出しの下に「カスタム絵文字の一覧」へのリンク、その下にフォームが 1 つ（区画の見出しは持たない）。

1. 登録のフォーム: 画像（選ぶボタン、選んだファイルの名前、100 KB を超えるときの注意）、shortcode、カテゴリ、別名、ピッカーに出すか、「登録する」の送信の帯。

### 4. 主な操作と保存の仕方

- 主な操作は「登録する」。まとめて保存（送信の帯 `FormActionBar`）。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 登録する | 画面の遷移。登録した絵文字のページへ移り、その見出しと絵文字が結果を示す。失敗の理由は送信の帯（入力は残す）、画像がないことは画像の欄 |

### 6. 入口と出口

- 入口: カスタム絵文字の一覧の「カスタム絵文字を登録する」。
- 出口: 登録した絵文字のページ。「カスタム絵文字の一覧」。
