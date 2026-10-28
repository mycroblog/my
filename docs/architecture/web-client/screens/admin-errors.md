---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.errors.tsx", "app/web/src/routes/admin.errors_.$id.tsx", "app/web/src/features/admin/components/ErrorGroups.tsx", "app/web/src/features/admin/components/ErrorGroupDetail.tsx", "app/web/src/features/admin/hooks/useErrors.ts", "app/web/src/features/admin/api/errors.ts"]
unavailable: {"増えているエラーを見つける": "the demo scenario records no error", "エラーが 1 時間ごとにどれだけ起きたかを知る": "the demo scenario records no error", "エラーを解決にする": "the demo scenario records no error"}
---

## エラー（`/admin/errors`・`/admin/errors/{id}`）

`manage_errors` を持つ管理者が、サーバーと Web クライアントが報告したエラーのグループを見て、増えているものを見つけ、直したものを解決にする画面（ADR-0103 §8、[§14](../14-admin.md)）。
一覧の各行は件数の文と、直近 7 日の `Sparkline` を持つ。詳細は直近 48 時間の `TrendChart` を持つ（ADR-0113）。

### 1. 使う人と頻度

- 管理者。管理のトップの未解決のエラーのタイルや、`admin_error_group` の通知から来る。新しいエラーや再発があったとき。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 増えているエラーを見つける | 1 |
| エラーが 1 時間ごとにどれだけ起きたかを知る | 2 |
| エラーを解決にする | 3 |

手順は、管理のトップのタイルかメニューで一覧を開く 1 つ、一覧の行の型（リンク）を押す 1 つ、「解決にする」を押す 1 つ。増えているものは一覧の件数と `Sparkline` で開いただけでわかる。

### 3. 情報の優先順位とまとまり

一覧:

1. 絞り込み（状態・報告元。URL に置く）と、エラーのグループの表: 型（詳細へのリンク）・場所・メッセージ、状態、報告元、24 時間と 7 日の件数と直近 7 日の `Sparkline`、最後と最初の発生。

詳細:

1. 状態: 状態と、解決・ミュート・ミュートを外す操作。
2. このエラーについて: 報告元・種類・件数・発生・場所・型・メッセージ・フィンガープリント。
3. 最新の発生: ID とそのコピー、スタック。
4. 時間ごとの発生回数: 直近 48 時間の棒の `TrendChart`（表に切り替えられる）と、直近 90 日の発生のあったすべての時間の表。
5. 履歴: 状態の変更。

### 4. 主な操作と保存の仕方

- 主な操作は、詳細の「解決にする」。
- 確かめずにすぐ送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 絞り込みを変える | その場。一覧を読み直す（URL が変わる） |
| 型を押す | 画面の遷移。詳細 |
| 解決・ミュート・ミュートを外す | その場。状態の区画の `StatusMessage`、フォーカスは区画の見出しへ |
| グラフと表を切り替える | その場。選んだ形はこのブラウザのすべてのグラフに残る |
| ID をコピーする | トースト |

### 6. 入口と出口

- 入口: 管理のトップのタイル、管理のモードのメニューの「エラー」、`admin_error_group` の通知。
- 出口: 詳細の「すべてのエラー」で一覧へ。メニューのほかの画面。
