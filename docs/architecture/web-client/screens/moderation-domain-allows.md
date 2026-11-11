---
start: "/moderation/domain-actions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.allowed.tsx", "app/web/src/features/moderation/components/DomainAllows.tsx", "app/web/src/features/moderation/hooks/useDomainAllows.ts", "app/web/src/features/moderation/api/domainLists.ts"]
seed: [{"my": ["admin", "domain", "allow", "add", "-comment", "Partner server", "friend.example"]}]
budget: {"controls": 5, "helpers": 3, "words": 90}
---

## 許可したドメイン（`/moderation/domain-actions/allowed`）

`manage_domains` を持つ管理者が、連合を許可したドメイン（[ADR-0159](../../../adr/0159-federation-allowlist-and-blocklist-subscriptions.md) §1）を読み、足し、外す画面（[§14](../14-admin.md)）。
許可は、サーバーの設定の連合の範囲（`federation_scope`）が「許可したドメインだけ」のときにだけ効き、そのドメインとサブドメインとだけ連合する。範囲を切り替えるのは[サーバーの設定の連合のページ](admin-settings-federation.md)（`manage_settings`）で、この画面は範囲を変えない。
API は許可をすべて（5,000 まで）返すので、表はクライアントで 40 行ずつのページにする。

### 1. 使う人と頻度

- 組織や個人のサーバーの管理者（`manage_domains`）。連合の範囲を絞る前に、連合したいサーバーを並べる。その後は相手が増えたときと、やめるときに。月に数回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| どのドメインを許可しているかを知る | 2 |
| partner.example を許可する | 4 |
| friend.example の許可を外す | 4 |

手順は、メニューの「ドメインへの処分」を押す 1 つ、ページの間のリンクの「許可したドメイン」を押す 1 つから。
許可は、ドメインの欄に入れる 1 つと「許可する」を押す 1 つを足す。外すのは、行の「許可を外す」を押す 1 つと、確かめのダイアログの「許可を外す」を押す 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1` は「ドメインへの処分」で、その下にページの間の `LocalNav`（「処分の一覧」・「ブロックリストの購読」・「許可したドメイン」・「Mastodon の CSV」）を置き、今のページを `aria-current="page"` にする。

1. 許可したドメイン（`h2`）: 許可が効くのは連合の範囲が「許可したドメインだけ」のときで、サブドメインにも効き、処分は許可より強いことの説明。閲覧者が `manage_settings` を持つときは連合の範囲の設定へのリンク。
   表（`RecordTable`、`wholeList`）はドメインの順で、列はドメイン（`DomainName`）・メモ（`dir="auto"`、`lang=""`。なければ「なし」）・許可した日時・操作（「許可を外す」）の 4 つ。結果の `StatusMessage` と失敗（`role="alert"`）は表の上に出す。
2. ドメインを許可（`h2`）: ドメイン（必須、253 文字まで。U-label のまま送る）・メモ（`ReasonField`、5,000 文字まで。公開しない）・「許可する」（`FormActionBar`）。

### 4. 主な操作と保存の仕方

- 主な操作は「許可する」（`primary`）。保存はすぐ保存で、確かめない（許可は外せる）。
- 「許可を外す」は `ConfirmDialog`（`role="alertdialog"`）で確かめてから送る。範囲が「許可したドメインだけ」の間は、外したドメインと、ほかの許可が含まないサブドメインとの連合がすぐ止まり、ここの人とそのアカウントのフォローが切れて戻らないため（ドメインの凍結と同じ、NFR-MOD-7）。範囲が「すべてのサーバー」の間は何も変わらないことも同じダイアログで言う。
- どちらの変更も、許可の一覧と監査ログを取り直す。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 許可する | その場。欄を空にし、表の上の `StatusMessage` が許可したドメインを言う。すでに許可していたドメインは、許可とメモがそのままであることを言う。フォーカスはドメインの欄に残る |
| 許可を断られる | その場。`FormActionBar`（`role="alert"`）に理由を出し、入力を残す |
| 許可を外す | ダイアログで確かめたあと、その場。行が消え、フォーカスは次の行の「許可を外す」（なければ前の行、表が空ならドメインの欄）へ移り、`StatusMessage` が外したドメインを言う。失敗はダイアログの中に出し、ダイアログを開いたままにする |

### 6. 入口と出口

- 入口: ドメインへの処分のページの間のリンク、サーバーの設定の連合のページの連合の範囲のリンク（`manage_domains` を持つ閲覧者）、監査ログの `domain_allow` の対象のリンク。
- 出口: ページの間のリンクのほかのページ、連合の範囲の設定（`manage_settings` を持つ閲覧者）、管理のメニュー。権限がなければ `AdminError` が足りない権限を示す。
