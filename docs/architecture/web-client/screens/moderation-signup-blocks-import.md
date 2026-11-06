---
start: "/moderation/signup-blocks"
persona: "marco"
paths: ["app/web/src/routes/moderation.signup-blocks_.import.tsx", "app/web/src/features/moderation/components/SignupBlockImport.tsx", "app/web/src/features/moderation/hooks/useSignupBlockImport.ts", "app/web/src/features/moderation/api/signupBlocks.ts"]
unavailable: {"テキストファイルを選んで一覧を読み込む": "the agent has no file to choose: the browser tools give it none"}
---

## 登録の拒否の一覧からの追加（`/moderation/signup-blocks/import`）

`manage_accounts` を持つモデレーターが、使い捨てのメールアドレスのドメインの公開リストのような値の一覧（1 回に 10,000 件まで）から、同じ種類・効果・期間・メモの拒否をまとめて追加し、追加しなかった行とその理由を読むページ（FR-719、[§14](../14-admin.md)）。
登録の拒否リストの画面の 2 つ目のページで、`h1` の下の `LocalNav` の「一覧から追加する」が今いるページになる。

### 1. 使う人と頻度

モデレーター（`manage_accounts`）。サーバーを始めるとき、公開リストが更新されたとき。月に 1 回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から来て、メールのドメインの一覧を貼り付けて拒否し、追加しなかった行と理由を読む | 4 |
| テキストファイルを選んで一覧を読み込む | 3 |

手順は、一覧の `LocalNav` の「一覧から追加する」を押す 1 つから。種類は既定でメールのドメインなので、一覧の欄に貼り付ける 1 つ、効果を選ぶ 1 つ、「まとめて追加する」を押す 1 つ。 追加しなかった行は、追加の結果の下にすぐ出る。

### 3. 情報の優先順位とまとまり

1. 一覧から追加する（`h2`）: 追加の働きの説明、種類（`RadioGroup`）・一覧（`TextArea`、値の数の検証）と「テキストファイルを選ぶ」（`FileTrigger`）・効果・期間・メモ、「まとめて追加する」、
   結果の `StatusMessage` と、追加しなかった行の表（`h3`、行・値・理由。多いときは最初の 100 行と、残りを出すボタン。押すとボタンがなくなるのでフォーカスは表の見出しへ移る）。

### 4. 主な操作と保存の仕方

- 主な操作は「まとめて追加する」。保存はまとめて保存で、拒否はすぐ効き、詳細のページから 1 つずつ解除できるので、確かめない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| まとめて追加する | その場。`StatusMessage` が追加した数と追加しなかった数を言い、追加しなかった行の表が出る。一覧の欄は空になり、ほかの選択は残る。失敗はフォームの `InlineError` |
| ファイルを選ぶ | その場。ファイルの内容で一覧の欄を置き換え、読み込んだ件数を言う |

### 6. 入口と出口

- 入口: 登録の拒否リストの `LocalNav` の「一覧から追加する」。
- 出口: `LocalNav` の「拒否の一覧」で一覧へ（追加した拒否がある）。権限がなければ `AdminError` が足りない権限を示す。
