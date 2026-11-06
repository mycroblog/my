---
start: "/moderation/signup-blocks"
persona: "marco"
paths: ["app/web/src/routes/moderation.signup-blocks_.$id.tsx", "app/web/src/features/moderation/components/SignupBlockDetail.tsx", "app/web/src/features/moderation/hooks/useSignupBlocks.ts", "app/web/src/features/moderation/api/signupBlocks.ts"]
seed: [{"my": ["admin", "signup-block", "create", "-kind", "email_domain", "-comment", "Throwaway addresses used by spam sign-ups.", "disposable-mail.example"]}]
---

## 登録の拒否の詳細（`/moderation/signup-blocks/{id}`）

`manage_accounts` を持つモデレーターが、1 つの拒否（FR-719、`GET /admin/signup_blocks/{id}`）の効果・メモ・日時を読み、解除するページ（[§14](../14-admin.md)）。

### 1. 使う人と頻度

モデレーター（`manage_accounts`）。なぜその拒否を足したかを確かめるとき、誤って拒否された人から連絡があって解除するとき。月に数回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から拒否を開き、メモを読む | 1 |
| 拒否を解除する | 2 |

手順は、一覧の行の値のリンクを押す 1 つから。解除は「解除」を押す 1 つを足す（確かめない）。

### 3. 情報の優先順位とまとまり

`h1` は拒否の種類と値（「メールのドメイン disposable-mail.example の拒否」）で、その下に「登録の拒否リストに戻る」のリンクを置く。

1. 状態（`h2`）: 状態・追加した日時・終わる日時（解除したものは解除した日時）と、有効な拒否には「解除」と、解除で何が変わるかの説明、結果の `StatusMessage`。
2. 詳細（`h2`）: ブロック対象（メールのドメインは `DomainName`）・種類・効果・追加のしかた（単独か BAN とともにか）・メモ（平文で `dir="auto"`、なければ「なし」）。

### 4. 主な操作と保存の仕方

- 主な操作は「解除」（有効な拒否だけ）。すぐ保存で、確かめない（解除はまた拒否を足せば戻せる）。
- 解除した拒否は、ページの値とすべての一覧のキャッシュに応答を書き、有効な拒否の一覧からは外す。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 解除する | その場。状態が「解除済み」になり、ボタンがなくなるので、フォーカスは状態の見出しへ移り、`StatusMessage`（`role="status"`）が何を解除したかを言う。失敗はトースト（`danger`）で、ボタンとフォーカスはそのまま |
| 「登録の拒否リストに戻る」を押す | 画面の遷移。[一覧](moderation-signup-blocks.md)へ |

### 6. 入口と出口

- 入口: [一覧](moderation-signup-blocks.md)の行の値のリンク、監査ログの `signup_block` の項目のリンク。
- 出口: 一覧。知らない ID は `RouteError`（404）、権限がなければ `AdminError` が足りない権限を示す。
