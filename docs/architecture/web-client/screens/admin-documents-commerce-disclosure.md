---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.documents_.commerce-disclosure.tsx", "app/web/src/features/admin/components/DocumentSettings.tsx", "app/web/src/features/admin/hooks/useDocumentSettings.ts", "app/web/src/features/admin/api/documents.ts", "app/web/src/features/documents/**", "app/ui/src/LocalNav.tsx"]
---

## 規約とポリシー: 特定商取引法に基づく表記など（`/admin/documents/commerce-disclosure`）

`manage_settings` を持つ管理者が、サポーターのプランの販売者として示す表記（`commerce_disclosure`、[ADR-0072](../../../adr/0072-supporter-plans-and-roles.md) §13）を Markdown で書くページ（FR-705、[§14](../14-admin.md)）。法的な正しさをサーバーが保証せず、運営者が自分の国の法令で確かめることを説明に書く。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「利用規約」「プライバシーポリシー」「特定商取引法に基づく表記など」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「規約とポリシー」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- サポーターのプランを始める管理者。課金を有効にする前に 1 回。
- 運営を続ける管理者。販売者の情報が変わるとき。年に 1 回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 特定商取引法に基づく表記を書いて保存する | 4 |

手順は、管理のメニューの「規約とポリシー」を開く 1 つ、ページの間のリンクで「特定商取引法に基づく表記など」を開く 1 つ、本文を変える 1 つ、「保存する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 特定商取引法に基づく表記など（`h2` の区画）: 法令に合っているかは運営者が確かめることの説明、最後に保存した日時、公開ページへのリンク（公開しているときだけ。同じタブの `DocumentLink`）、本文（Markdown の `TextArea`、100,000 文字）と「保存する」。

### 4. 主な操作と保存の仕方

- 主な操作は「保存する」（ページに 1 つ、`primary`）。保存は、まとめて保存だけ。この文書だけを `PUT /admin/instance_texts/{key}` で送り、ほかの文書は送らない。
- 保存してある本文を空にして保存すると削除になるので、`ConfirmDialog`（`danger`）で確かめる。
- 保存していない本文があるままほかのページへ移ろうとすると、`alertdialog` で確かめる（最初のフォーカスは「編集を続ける」）。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 保存 | その場。本文・`GET /instance`（リンク）・監査ログを取り直してから、監査ログに記録したことをフォームの `StatusMessage` に出す |
| 本文を空にして保存 | `ConfirmDialog` で確かめてから削除し、`StatusMessage` に出す。公開ページへのリンクが消える |
| サーバーが拒否 | その場。フォームの下（`role="alert"`）に理由を出し、入力を残す |
| 保存せずにほかのページへ | `alertdialog` で確かめる。「変更を破棄する」で移り、「編集を続ける」と Esc で残る |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「規約とポリシー」から、ページの間のリンクの「特定商取引法に基づく表記など」。
- 出口: ページの間のリンクのほかのページ、公開ページ、管理のメニュー。
