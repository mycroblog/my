---
start: "/moderation"
persona: "marco"
paths: ["app/web/src/routes/moderation.signup-blocks.tsx", "app/web/src/features/moderation/components/SignupBlocks.tsx", "app/web/src/features/moderation/hooks/useSignupBlocks.ts", "app/web/src/features/moderation/api/signupBlocks.ts"]
seed: [{"my": ["admin", "signup-block", "create", "-kind", "email_domain", "-comment", "Throwaway addresses used by spam sign-ups.", "disposable-mail.example"]}]
---

## 登録の拒否リストの一覧（`/moderation/signup-blocks`）

`manage_accounts` を持つモデレーターが、メールのドメイン・メールアドレス・IP アドレスの範囲による登録の拒否（FR-719）を新しい順に読み、1 つの拒否の詳細か新しい拒否のフォームへ進む画面（[§14](../14-admin.md)）。
一覧は読むだけで、追加するのは[新しい拒否](moderation-signup-block-new.md)と[一覧からの追加](moderation-signup-blocks-import.md)、解除するのは[拒否の詳細](moderation-signup-block.md)で行う。

### 1. 使う人と頻度

- モデレーター（`manage_accounts`）。スパムの登録が続いたとき、登録の審査やアカウントの BAN から来る。週に数回より少ない。
- いま効いている拒否を見直すモデレーター。月に 1 回ほど。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| モデレーションのメニューから来て、いま効いている拒否を読む | 1 |
| 解除・期限切れも含めて、すべての拒否を読む | 3 |
| 拒否のメモを読む | 2 |
| 新しい拒否のフォームを開く | 2 |

手順は、メニューの「登録の拒否リスト」を押す 1 つから。すべての拒否は、ツールバーの「表示する拒否」を開く 1 つと「すべて」を選ぶ 1 つを足す。
メモは、行の値のリンクを押す 1 つを足す（詳細のページにある）。フォームは「拒否を追加する」を押す 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1` の下に画面のページの `LocalNav`（「拒否の一覧」・「一覧から追加する」）を置き、その下に「拒否を追加する」のボタン（`/moderation/signup-blocks/new` へのリンク）、
ツールバーの絞り込み（「有効」が既定、「すべて」は URL の `filter=all`）、拒否の表を並べる。区画の見出し（`h2`）は持たない。

1. 拒否の表（`RecordTable`）: 列はブロック対象（メールのドメインは `DomainName` で 2 つの形、メールアドレスはハッシュ、IP は CIDR。詳細へのリンク）・種類・状態・終わる日時（なければ「解除するまで」）の 4 つ。
   効果・追加のしかた・メモは一覧に出さない。40rem より狭いコンテナでは行ごとのカードになり、長いハッシュも折り返して横にスクロールしない。

### 4. 主な操作と保存の仕方

- 主な操作は「拒否を追加する」（`primary` のリンク）。一覧には保存する操作がない。
- 絞り込みはその場で URL に置き、一覧は最初のページから出直す。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 絞り込みを変える | その場。URL の `filter` が変わり、一覧が最初のページから出直す |
| 行の値を押す | 画面の遷移。[拒否の詳細](moderation-signup-block.md)へ |
| 「拒否を追加する」を押す | 画面の遷移。[新しい拒否](moderation-signup-block-new.md)へ |
| 次のページを読み込む | その場。前後のページへ移る。失敗は一覧の中のエラーと再試行 |

### 6. 入口と出口

- 入口: モデレーションのメニュー（`manage_accounts`）、アカウントの BAN でメールアドレスの拒否を作った後（FR-719）、詳細と新しい拒否のページの「登録の拒否リストに戻る」。
- 出口: 拒否の詳細、新しい拒否、一覧からの追加。権限がなければ `AdminError` が足りない権限を示す。
