---
start: "/moderation/invites"
persona: "hana"
paths: ["app/web/src/routes/moderation.invites_.new.tsx", "app/web/src/features/moderation/components/NewInvite.tsx", "app/web/src/features/moderation/hooks/useInvites.ts", "app/web/src/features/moderation/api/invites.ts"]
---

## 新しい招待（`/moderation/invites/new`）

`manage_invites` を持つ人が、登録できる人数と有効期間を決めて招待を作り、その招待コードと登録のリンクを受け取るページ（FR-001、[§14](../14-admin.md)）。

### 1. 使う人と頻度

管理者か、`manage_invites` を持つロールの人。新しい人を迎えるたび。週に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から来て、1 人が 1 週間使える招待を作る | 2 |
| 人数の上限のない、期限のない招待を作る | 6 |

手順は、一覧の「招待を作成する」を押す 1 つから。既定（1 人、1 週間）のままなら「招待を作成する」を押す 1 つ。
上限と期限をなくすには、「登録できる人数」を開く 1 つと「制限なし」を選ぶ 1 つ、「有効期間」を開く 1 つと「無期限」を選ぶ 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1`（「招待を作成する」）の下に「招待に戻る」のリンクを置く。登録モードが `closed` のときは、招待では誰も登録できないことを区画の前に言う。

1. 招待の内容（`h2`）: 登録できる人数（`Select`、既定は 1 人）・有効期間（`DurationSelect`、既定は 7 日）、「招待を作成する」と結果の `StatusMessage`、作った招待の登録のリンク（`dir="ltr"`、全体を選べる）。

### 4. 主な操作と保存の仕方

- 主な操作は「招待を作成する」（画面で唯一の `primary`）。保存はまとめて保存で、確かめない（失効させれば止められる）。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 招待を作成する | その場。`StatusMessage` が作った招待のコードを言い、その下に登録のリンクを出す。もう 1 つ作ると置き換わる。失敗はフォームの下の `InlineError`（`role="alert"`） |
| 「招待に戻る」を押す | 画面の遷移。[一覧](moderation-invites.md)へ。新しい招待は一覧の先頭にある |

### 6. 入口と出口

- 入口: 一覧の「招待を作成する」。
- 出口: 「招待に戻る」で一覧へ。権限がなければ `AdminError` が足りない権限を示す。
