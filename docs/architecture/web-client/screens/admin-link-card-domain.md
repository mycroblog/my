---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.link-card-domains_.$domain.tsx", "app/web/src/features/admin/components/LinkCardDomainSettings.tsx", "app/web/src/features/admin/hooks/useLinkCardDomains.ts", "app/web/src/features/admin/api/linkCardDomains.ts"]
seed: [{"api": "hana", "method": "PUT", "path": "/admin/link_card_domains/images.example", "body": {"mode": "no_images", "note": "Heavy images"}}]
---

## ルールのページ（`/admin/link-card-domains/{domain}`）

`manage_settings` を持つ管理者が、1 つのドメインのルールの止めるものとメモを変え、ルールを削除するページ（[ADR-0062](../../../adr/0062-link-preview-cards.md)、[§14](../14-admin.md)）。
1 つのルールを返す API はないので、一覧（`GET /admin/link_card_domains`）から読み（ドメインの大文字小文字は区別しない）、区画の中で読み込む（G16）。ルールのないドメインは、その旨と「ルールを追加」の空の状態にする。

### 1. 使う人と頻度

- 管理者。画像だけ止めていたサイトを完全に止めるとき、サイトの事情が変わってルールをやめるとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| images.example の取得を止める | 4 |
| images.example のルールを削除する | 4 |

手順は、メニューで一覧を開く 1 つ、ドメインを押す 1 つ、止めるものの「取得を止める」を選ぶ 1 つと「保存」を押す 1 つ。削除は「削除」と確かめの「削除」の 2 つ。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`「リンクのカードのルール」）と「ルールの一覧へ戻る」。
2. ドメイン（区画の見出しは `DomainName`）: 最後の設定の日時と、止めるもの（`RadioGroup`）・メモ（`ReasonField`）のフォーム。
3. ルールを削除: 削除するとどうなるか（取得が戻り、メモは戻らない）の説明と「削除」。

### 4. 主な操作と保存の仕方

- 主な操作は「保存」。止めるものとメモはまとめて保存（`FormActionBar`）。
- 削除は `ConfirmDialog` で確かめる（クロールが再開し、メモは戻らない）。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 保存 | その場。ボタンの隣に結果（`role="status"`）。失敗も同じ場所（`role="alert"`）で、入力を残す。フォームを変えると結果は消える |
| 削除 | 画面の遷移とトースト。確かめの後、一覧へ移り、削除したことをトーストで知らせる。失敗はダイアログの中 |
| ルールの読み込みの失敗 | その場。`RegionError`（G16）。`h1` と戻るリンクは残る |

### 6. 入口と出口

- 入口: 一覧のドメイン、追加のページの「ルールを保存」、監査ログの対象のリンク（一覧へ）。
- 出口: 「ルールの一覧へ戻る」、削除の後の一覧。
