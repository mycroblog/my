---
start: "/moderation/domain-actions/subscriptions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.subscriptions_.$id.tsx", "app/web/src/features/moderation/components/DomainListSubscriptionDetail.tsx", "app/web/src/features/moderation/components/domainListParts.tsx", "app/web/src/features/moderation/hooks/useDomainListSubscriptions.ts", "app/web/src/features/moderation/api/domainLists.ts"]
unavailable: {"下書きをまとめて受け入れる": "The demo server has no subscription whose list it can fetch, so there are no drafts.", "下書きを 1 件退ける": "The demo server has no subscription whose list it can fetch, so there are no drafts.", "今すぐ取得する": "The demo server has no subscription whose list it can fetch.", "モードを自動にする": "The demo server has no subscription.", "購読をやめる": "The demo server has no subscription."}
---

## ブロックリストの購読のページ（`/moderation/domain-actions/subscriptions/{id}`）

`manage_domains` を持つ管理者が、1 つの購読（`GET /admin/domain_list_subscriptions/{id}`、[ADR-0159](../../../adr/0159-federation-allowlist-and-blocklist-subscriptions.md) §5）の下書きを受け入れるか退け、取得の履歴を読み、
今すぐ取得し、設定を変え、購読をやめるページ（[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者（`manage_domains`）。下書きのモードでは、リストが変わるたびに下書きを確かめる。週に 1 回ほど。取得の失敗の通知を受けたときは履歴を読む。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 下書きをまとめて受け入れる | 3 |
| 下書きを 1 件退ける | 2 |
| 今すぐ取得する | 2 |
| モードを自動にする | 3 |
| 購読をやめる | 3 |

手順は、一覧の購読の名前を押す 1 つから。まとめての受け入れは「すべて受け入れる」を押す 1 つと、凍結を含むときの確かめの「受け入れる」を押す 1 つを足す。1 件を退けるのは、行の「退ける」を押す 1 つを足す。
取得は「今すぐ取得」を押す 1 つ。モードは「自動」を選ぶ 1 つと「設定を保存」を押す 1 つ。やめるのは「購読をやめる」を押す 1 つと、確かめのダイアログの「購読をやめる」を押す 1 つ。

### 3. 情報の優先順位とまとまり

`h1` は購読の名前（`dir="auto"`。権限のない閲覧者には画面の名前）で、その下に「購読の一覧に戻る」のリンクを置く。

1. 下書き（`h2`）: 待っている下書きの件数と、なぜ待つか（下書きのモードでは差分のすべて、自動のモードでは知らない重さと、一度に多すぎる解除）の説明。
   下書きがあれば「すべて受け入れる」と「すべて退ける」、その下に表（`RecordTable`、`wholeList`。解除が先、ドメインと種類の順）。列はドメイン（`DomainName`）・変更（「{種類}を出す」か「{種類}を解除する」と、待つ理由が下書きのモードでないときはその理由、リストの公開コメントがあれば `dir="auto"` で）・操作（「受け入れる」と「退ける」）の 3 つ。
   結果の `StatusMessage` と失敗（`role="alert"`）は表の上に出す。下書きがなければ空の状態。
2. 取得（`h2`）: URL（`dir="ltr"`）・形式・最後の取得と、続けて失敗している回数（3 回で `manage_domains` を持つ人に通知すること）、「今すぐ取得」、
   その下に取得の履歴の表（`RecordTable`、カーソルで 40 件ずつ、新しい順）。列は日時・結果（反映した・変更なし・失敗とその理由、HTTP のステータス）・取得した後の件数の 3 つ。
3. 設定（`h2`）: [追加のページ](moderation-domain-list-subscription-new.md)の名前と反映のしかたの欄（URL と形式は変えられない）と、「設定を保存」（`FormActionBar`）。保存すると取得を 1 回積み、新しい設定で反映することの説明。
4. 購読をやめる（`h2`）: やめると取得しなくなり、出した処分は人が出した処分として残ることの説明と、「購読をやめる」（`danger`）。

### 4. 主な操作と保存の仕方

- 主な操作は「すべて受け入れる」（下書きがあるときの `primary`）。下書きの受け入れと退けはすぐ保存で、読み込んだ下書きの `key` を送る（読み込んだ後に出た下書きは含めない）。
  受け入れる下書きが凍結を出すものを含むときだけ、凍結するドメインの数を言う `ConfirmDialog` で確かめる（NFR-MOD-7）。退けるのは確かめない（リストのその行が変わればまた出る）。
- 「今すぐ取得」は確かめずに積む（`202`）。取得は非同期なので、購読の最後の取得が変わるまで 3 秒ごとに 1 分まで購読を読み直し、変わったら履歴を取り直す。
- 設定はまとめて保存（変えた欄だけを `PATCH` で送る）。
- 「購読をやめる」は `ConfirmDialog`（`role="alertdialog"`）の中の `Checkbox`「この購読が出した処分を解除する」（既定は外す）とともに確かめてから送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 受け入れる・退ける（1 件） | その場（凍結を出す受け入れはダイアログで確かめたあと）。行が消え、フォーカスは次の行の「受け入れる」（なければ前の行、表が空なら区画の見出し）へ移り、`StatusMessage` が何をしたかを言う。処理済みだった下書きはその数を言う |
| すべて受け入れる・すべて退ける | その場（凍結を含む受け入れはダイアログで確かめたあと）。表が空の状態になり、フォーカスは区画の見出しへ移り、`StatusMessage` が件数を言う。失敗は `role="alert"` で、下書きは残る |
| 今すぐ取得 | その場。`StatusMessage` が取得を積んだことを言い、取得が終わったら結果を言って、最後の取得と履歴が変わる。失敗は `role="alert"` |
| 設定を保存 | その場。`FormActionBar` に結果を出す。変えずに保存したら保存するものがないこと。拒否は理由を出して入力を残す |
| 購読をやめる | ダイアログで確かめたあと、画面の遷移。一覧へ移り、トーストが何をしたかを言う。失敗はダイアログの中に出す |

### 6. 入口と出口

- 入口: [購読の一覧](moderation-domain-list-subscriptions.md)の購読の名前、[追加のページ](moderation-domain-list-subscription-new.md)の「購読する」、監査ログの `domain_list_subscription` の対象のリンク。
- 出口: 一覧（戻るリンクとやめたとき）。知らない ID は `RouteError`（404）、権限がなければ `AdminError` が足りない権限を示す。
