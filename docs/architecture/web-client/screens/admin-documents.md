---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.documents.tsx", "app/web/src/features/admin/components/DocumentSettings.tsx", "app/web/src/features/admin/hooks/useDocumentSettings.ts", "app/web/src/features/admin/api/documents.ts", "app/web/src/features/documents/**", "app/ui/src/LocalNav.tsx"]
---

## 規約とポリシー: 利用規約（`/admin/documents`）

`manage_settings` を持つ管理者が、サーバーの利用規約を Markdown で書くページ（FR-705、#489、[§14](../14-admin.md)）。規約とポリシーの画面の最初のページで、[プライバシーポリシー](admin-documents-privacy-policy.md)と[特定商取引法に基づく表記など](admin-documents-commerce-disclosure.md)は子のルート。文書はどのロケールでも同じ 1 つの値で、言語を選ぶ欄はない。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「利用規約」「プライバシーポリシー」「特定商取引法に基づく表記など」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「規約とポリシー」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- サーバーを立ち上げた管理者。最初に利用規約を書く。立ち上げのときに 1 回。
- 運営を続ける管理者。規約を改めるとき。年に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 利用規約を書き直して保存する | 3 |
| 利用規約の公開ページを開く | 2 |

手順は、管理のメニューの「規約とポリシー」を開く 1 つ、本文を変える 1 つ、「保存する」を押す 1 つ（公開ページは、開く 1 つとリンクの 1 つ）。

### 3. 情報の優先順位とまとまり

1. 利用規約（`h2` の区画）: 最後に保存した日時、公開ページへのリンク（公開しているときだけ。同じタブの `DocumentLink`）、本文（Markdown の `TextArea`、100,000 文字）と「保存する」。

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

- 入口: 管理のモードのメニューの「規約とポリシー」。
- 出口: ページの間のリンクのほかのページ、公開ページ、管理のメニュー。
