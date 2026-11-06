---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.link-card-domains.tsx", "app/web/src/features/admin/components/LinkCardDomainSettings.tsx", "app/web/src/features/admin/hooks/useLinkCardDomains.ts", "app/web/src/features/admin/api/linkCardDomains.ts"]
seed: [{"api": "hana", "method": "PUT", "path": "/admin/link_card_domains/images.example", "body": {"mode": "no_images", "note": "Heavy images"}}]
---

## リンクのカードのドメイン（`/admin/link-card-domains`）

`manage_settings` を持つ管理者が、リンクのカードの取得を止めるか画像の取得だけを止めるドメインのルールを一覧で見て、探す画面（[ADR-0062](../../../adr/0062-link-preview-cards.md)、[§14](../14-admin.md)）。
ルールの追加は[追加のページ](admin-link-card-domains-new.md)、1 つのルールの変更と削除は[ルールのページ](admin-link-card-domain.md)に分ける。
サーバーは 1,000 までのルールを持ち、API はすべてを返すので、一覧はクライアントで 40 行ずつのページにする。

### 1. 使う人と頻度

- 管理者。画像が重いサイトや、取得をやめるよう頼んできたサイトがあるとき。月に数回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| どのドメインにルールがあるかを知る | 1 |
| images.example のルールを探す | 3 |

手順は、メニューで画面を開く 1 つ（一覧の最初のページでわかる）、検索の欄にドメインの一部を入れる 1 つ、「検索」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 「ルールを追加」（[追加のページ](admin-link-card-domains-new.md)へのリンクのボタン）。
2. ルール: ドメインの一部での検索（`role="search"` のフォーム。URL の `?domain=` に置き、「検索を解除」で戻す）と、ドメインの順の表（40 行ずつのページ）。ドメイン（`DomainName`、ルールのページへのリンク）・止めるもの・最後の設定の日時。

### 4. 主な操作と保存の仕方

- 主な操作は「ルールを追加」（ページの遷移）。一覧では何も保存しない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 「ルールを追加」 | 画面の遷移。追加のページ |
| ドメインを押す | 画面の遷移。ルールのページ |
| 検索する・検索を解除する | その場。URL が変わり、表が最初のページから出直す。合うルールがなければ空の状態 |
| 一覧の読み込みの失敗 | その場。一覧の区画に `RegionError`（G16）。`h1` と「ルールを追加」は残り、「もう一度試す」のフォーカスを読み込みの間も保つ（`useLoadFailure`） |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「接続」のまとまり、監査ログの対象のリンク、ルールのページの「ルールの一覧へ戻る」と削除。
- 出口: 追加のページ、ルールのページ、メニューのほかの画面。
