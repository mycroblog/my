<!-- source: closing-the-service 30c438995ba1 -->
# サービスの終了

サーバーの運用をやめるときに、順に行うことを説明します。利用者にデータを持ち出してもらい、ほかのサーバーに残るアカウントを削除し、
そのあとサーバーを止めて静的な終了ページに切り替えます。
各コマンドの詳しい説明は[管理者とロール](admin-cli.md#announcing-the-end-of-the-service)にあります。

<!-- source: the-sequence 4557c35b6851 -->
## 手順の流れ

![サービス終了の各段階を上から下へ。左に運営者が行うこと、右にサーバーの状態があり、その間に終了日とエクスポートの期限があります。下の表が各段階を説明しています。](images/service-closure.svg)

表は図を上から順にたどります。「サーバー」の列は、`my admin closure status` が表示する状態から始まります。
終了日（`-closes-at`）で予告期間が終わり、エクスポートの期限（`-export-until`、指定しなければ終了日の 30 日後）で段階 5 が終わります。

| 段階 | サーバー | 運営者が行うこと |
| --- | --- | --- |
| 1. 告知 | `announced`。通常どおり動きます。新規登録、招待、新しいサポーターの申し込みが止まります | `my admin closure announce`、ステータスページにお知らせを掲載 |
| 2. 予告期間 | `announced`。通常どおり動きます。利用者はデータをエクスポートし、ほかのサーバーへ移行できます | 年額のサポーターへの返金、移行と保存義務による保留の確認 |
| 3. 終了（終了日のあと） | `closed`。利用者はサインインとエクスポートだけができます。サーバーはすべてのアカウントについて `Delete` を配送します | `my admin closure check` と `close` |
| 4. 配送の完了 | `delivery_completed` を記録します。ローカルの Actor も `410` を返します | `my admin closure status` で確認。ワーカーは止めない |
| 5. エクスポートの期限まで | `delivery_completed`。終了した状態のまま、エクスポートを提供し続けます | 保存義務による保留の対象の記録をエクスポート |
| 6. 静的ページ | 停止。フロントエンドが静的ページを返し、連合には `410` を返します | `my admin closure export-html`、フロントエンドの切り替え、`my serve` の停止 |
| 7. 破棄 | なし。静的ページと `410` を 1 年ほど返し続けます | DB、blob、バックアップの破棄。ドメインは 1 年ほど保持 |

どの段階もメンテナンスモード（[メンテナンス](maintenance.md)）と組み合わせられます。メンテナンス中は、エクスポートと `Delete` アクティビティの配送が止まります。

<!-- source: 1-announce b34cb9474667 -->
## 1. 告知

終了日（`-closes-at`）と、データを持ち出す期限（`-export-until`、省略すると終了日の 30 日後）を決めて、告知します。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- 終了日に下限はありませんが、告知から終了日までが 30 日より短いと警告が表示されます。利用者がエクスポートして移行する時間を残してください。
- 日付を変えるには `announce` をもう一度実行し、取りやめるには `my admin closure withdraw` を実行します。どちらも、すべての利用者に `service_closure` の通知を送ります。
- 告知すると、実際の登録モードは `closed` になり、保留中の申請は却下され、継続中のサポーターの申し込みはすべて期間の終わりに解約されます（申し込んだ人には通知とメールが届きます）。
  告知を取り下げても、却下と解約は元に戻りません。
- 通知はジョブで作られるため、ワーカーを止めないでください。
- ステータスページ（[監視とステータスページ](monitoring.md)）に、終了日、エクスポートの期限、エクスポートと移行の方法を掲載します。
  ステータスページはサーバーよりも長く残るため、終了後に訪れた人への案内にもなります。

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. 予告期間

- 利用者は設定画面から、データをエクスポートし（データのエクスポートと CSV ファイル）、ほかのサーバーへ移行（`Move`）できます。
  すべての画面のバナーと通知に、終了日、エクスポートの期限、その画面への行き方が表示されます。
- **年額のサポーター**: 告知によって申し込みは期間の終わりに解約されますが、期間が終了日よりあとに終わる申し込みでは、使えない期間の分もすでに支払われています。
  サーバーは返金しないため、決済サービスの画面で申し込んだ人ごとに返金してください（Stripe ではダッシュボードの支払いの画面、PAY.JP では管理画面の支払いの画面）（[サポーターの決済（Stripe）](payments-stripe.md)、[サポーターの決済（PAY.JP）](payments-payjp.md)）。
- 終了日を過ぎると、`manage_service_closure` を持つ人に `admin_service_closure_due` の通知が届きます。サーバーが自動で終了することはありません。

<!-- source: 3-close c851780cf430 -->
## 3. 終了

終了日を過ぎたら、終了する前に確認を実行します。

```sh
my admin closure check
```

- **完了していない移行**があると警告が出ます。終了したあとは誰も `Move` を始められないため、完了を待ってから終了してください。
- いずれかのアカウントに**保存義務による保留（`retention_holds`）**があると警告が出ます。
  保留の理由となっている義務はサーバーを終了しても終わらないため、保留の対象の記録をエクスポートしてサーバーの外に保管してください（保留の一覧は `my admin retention list` で確認できます）。

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

確認が済んだら終了します。これは元に戻せません。`y` とドメイン名を入力して確定します（自動化では `-confirm-domain` を使います）。

```sh
my admin closure close
```

終了したあと、利用者はサインインとデータのエクスポートだけができます。残っているサポーターの申し込みはすぐに解約され、
すべてのローカルアカウントについて `Delete`（Actor）がほかのサーバーへ配送されます（`federation_mode` が `off` や `inbound` のときも配送されます）。
inbox と、投稿やコレクションへの GET は `410 Gone` を返します。

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. 配送の完了を確認する

**状態が `delivery_completed` になるまで、`my serve` とワーカー（`role` が `all` または `worker` のプロセス。プッシュ型のキューでは Cloud Tasks のキュー）を止めないでください。**
`Delete` アクティビティを配送するのはワーカーで、配送している間、受け取った側が `Delete` の署名を検証できるように、サーバーはローカルの Actor のドキュメントと鍵を返し続けます。
早く止めると、ほかのサーバーにアカウントが残ってしまいます。

```sh
my admin closure status
```

- `accounts with a Delete` は `Delete` が記録されたアカウントの数、`fan-outs pending` は残っているファンアウトの作業、`deliveries <state>` は状態ごとの配送の数です。
- 配送は通常のポリシーで再試行されます（約 4 日間で最大 9 回）。すべて終わると、10 分ごとに実行される確認が `delivery_completed` を記録します（`state: delivery_completed`）。
  それ以降は、ローカルの Actor とインスタンスの Actor も `410` を返します。
- `dead` になった配送は自動では再試行されません。再試行するには `my job retry` を使います。

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. エクスポートの期限まで

配送が完了したあとも、サーバーは `export_until` まで終了した状態のまま動き続け、利用者のエクスポートを提供し続けます。
サーバーが自分でデータを削除することはないため、期限を過ぎても、あなたが止めるまで動き続けます。
保存義務による保留の対象の記録（§3）をまだエクスポートしていなければ、ここで行ってください。

<!-- source: 6-switching-to-the-static-page 50a7f30c0ee4 -->
## 6. 静的ページに切り替える

エクスポートの期限を過ぎたら、終了ページを書き出します。**これは DB を読むため、`my serve` を止めて DB を破棄する前に実行してください。**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- 書き出すもの:
  - `<dir>/<locale>.html` と、すべてのロケールを順に並べた `<dir>/index.html`。メンテナンスページ（`my maintenance export`）と同じテンプレートと CSS を使い、
    サーバー名（`maintenance_server_name`、なければインスタンス名、それもなければホスト名）、終了日、告知時の運営者のメッセージ、ステータスページ（`status_page_url`）へのリンクを表示します。
    CSS はインラインだけで、スクリプトや外部のリソースはなく、CSP は `<meta>` タグにあります。テーマの色の上書きは埋め込まれますが、カスタム CSS は含まれません。
  - フロントエンドの設定例 `nginx.conf`、`Caddyfile`、`cloudfront-function.js`。どれも、連合と探索のパス（`/ap/*`、`/inbox`、`/.well-known/webfinger`、
    `/.well-known/host-meta`、`/.well-known/host-meta.json`、`/.well-known/nodeinfo`、`/nodeinfo/*`）とプロフィール（`/@*`）には本文なしの `410` を返し、
    それ以外のパスには `Accept-Language` の最初の言語のページ（なければ `index.html`）を返します。
- サーバーが終了していなければ、コマンドは実行を拒否します（終了コード 1）。`delivery_completed` より前なら、配送が続いていることを警告します（ファイルは書き出します）。

フロントエンドを切り替えます。公開する場所にはページ（`*.html`）だけを置き、例のホスト名、ディレクトリ、証明書のパスを自分の環境に合わせてください。

| フロントエンド | 設定のしかた |
| --- | --- |
| nginx | ページを `/srv/closure` に置き、`http` ブロックから `nginx.conf` を include します。ページも `410` で返されます |
| Caddy | ページを `/srv/closure` に置き、`Caddyfile` を使います。証明書は Caddy が取得し、ページも `410` で返されます |
| CloudFront | ページを S3 バケットの直下に置き、そのバケットを既定のキャッシュ動作のオリジンにして、`cloudfront-function.js` をビューアーリクエストの CloudFront Function（`cloudfront-js-2.0`）として関連付けます。キャッシュポリシーのキャッシュキーに `Accept-Language` を入れます。ページは `200` で返されます（ビューアーリクエストの関数ではオリジンのレスポンスのステータスを変えられないため） |

切り替えたら、`curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` が `410` を返し、
`curl -i https://social.example/` が終了ページを返すことを確かめます。そのあと `my serve` とワーカーを止めます。
死活監視は、ヘルスチェックの監視を止めるか、監視先を終了ページに向けてください。

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. DB、blob、バックアップを破棄する

止めたあと、DB、blob（メディアとエクスポートのファイル）、KV ストア、キューを、それぞれのバックアップやスナップショットとともに破棄します。[検索エンジン](search.md#closing-the-service)を使っているときは、そのインデックスとスナップショットも破棄してください。投稿の本文が含まれています。
何かを破棄する前に、保存義務による保留の対象の記録をエクスポートして保管したことをもう一度確かめてください。破棄は元に戻せません。

**ドメインは保持し、静的ページと `410` を 1 年ほど返し続けてください。** ほかのサーバーは `410` を見て、取得と配送をやめ、キャッシュを消します。
連合で使う URI には UUID を使っているため、同じ `id` の Actor がもう一度作られるおそれは小さいですが、
ほかの人がドメインを取得すると、ハンドル（`acct:alice@social.example`）とプロフィールの URL はその人のものになり、あなたの利用者へのなりすましに使われるおそれがあります。
ドメインと証明書の更新を続け、1 年が過ぎたあとも、ドメインを手放す前にもう一度よく考えてください。
