---
start: "/moderation/signup-blocks"
persona: "marco"
paths: ["app/web/src/routes/moderation.signup-blocks_.new.tsx", "app/web/src/features/moderation/components/NewSignupBlock.tsx", "app/web/src/features/moderation/components/signupBlockFields.ts", "app/web/src/features/moderation/hooks/useSignupBlocks.ts", "app/web/src/features/moderation/api/signupBlocks.ts"]
---

## 新しい登録の拒否（`/moderation/signup-blocks/new`）

`manage_accounts` を持つモデレーターが、メールのドメイン・メールアドレス・IP アドレスの範囲による登録の拒否（FR-719）を 1 つ追加するページ（[§14](../14-admin.md)）。

### 1. 使う人と頻度

モデレーター（`manage_accounts`）。使い捨てのメールアドレスのサービスや、ボットが登録してくる範囲を見つけたとき。週に数回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から来て、メールのドメインの登録を拒否する | 4 |
| IP アドレスの範囲からの登録を承認制にする | 5 |
| 期限とメモを付けて、メールアドレスの登録を拒否する | 8 |

手順は、一覧の「拒否を追加する」を押す 1 つから。メールのドメインは、種類が既定で選んであるので、値の入力 1 つ、効果を選ぶ 1 つ、「拒否を追加する」を押す 1 つ。
IP の範囲は、種類を選ぶ 1 つを足す。期限とメモは、期間の選択を開く 1 つと選ぶ 1 つ、メモの入力 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1`（「拒否を追加する」）の下に「登録の拒否リストに戻る」のリンクを置く。

1. 拒否の内容（`h2`）: 種類（`RadioGroup`、既定はメールのドメイン）・値（種類ごとのラベルと説明、必須）・効果（必須の `RadioGroup`、「拒否する」と「承認制にする」）・期間（`DurationSelect`、既定は「解除するまで」）・
   メモ（`ReasonField`、モデレーターだけが見る）、その下に「拒否を追加する」と結果の `StatusMessage`。

### 4. 主な操作と保存の仕方

- 主な操作は「拒否を追加する」（画面で唯一の `primary`）。保存はまとめて保存で、ブラウザの制約の検証（未入力の値と効果）を通ってから送る。
- 拒否はすぐ効き、詳細のページから同じくらい簡単に解除できるので、確かめない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 拒否を追加する | その場。フォームの下の `StatusMessage` が何がどの値に効いたかを言い、フォームは種類を残して空になる。失敗はフォームの下の `InlineError`（`role="alert"`）で、入力は残る |
| 「登録の拒否リストに戻る」を押す | 画面の遷移。[一覧](moderation-signup-blocks.md)へ。新しい拒否は一覧にある |

### 6. 入口と出口

- 入口: 一覧の「拒否を追加する」。
- 出口: 「登録の拒否リストに戻る」で一覧へ。権限がなければ `AdminError` が足りない権限を示す。
