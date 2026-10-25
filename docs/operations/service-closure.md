# サービスの終了

運営者向けの手順です。サーバーの運営をやめるときに、利用者がデータを持ち出せるようにし、連合先に残ったアカウントを消してから、
サーバーを止めて静的な閉鎖のページに切り替えるまでを順に書きます。
決定は [ADR-0088](../adr/0088-service-shutdown-modes.md)、要件は FR-808・FR-809（[機能要件](../requirements/01-functional.md)）、
連合の振る舞いは [federation.md §15](../architecture/federation.md#15-サービスの終了)、各コマンドの詳しい説明は
[管理者とロール](admin-cli.md#サービスの終了の予告) です。

## 流れ

```text
予告 ──告知の期間──▶ 閉鎖日 ──クローズ──▶ 配送の完了 ──持ち出しの期限──▶ 静的なページ ──▶ DB・Blob・バックアップの破棄
```

| 段階 | サーバー | 運営者がすること |
| --- | --- | --- |
| 1. 予告 | いつもどおり動く。登録・招待・サポーターの新しい加入は止まる | `my admin closure announce`、ステータスページへの掲出 |
| 2. 告知の期間 | いつもどおり動く。利用者はエクスポートと移転ができる | 年払いのサポーターの返金、移転と保全の確認 |
| 3. クローズ（閉鎖日の後） | 利用者はサインインとエクスポートだけ。全アカウントの `Delete` を配送する | `my admin closure check` と `close` |
| 4. 配送の完了 | `delivery_completed` を記録し、ローカルの Actor も `410` | `my admin closure status` で確かめる。ワーカーを止めない |
| 5. 持ち出しの期限まで | クローズのままエクスポートを提供する | 保全の記録の書き出し |
| 6. 静的なページ | 止める | `my admin closure export-html`、前段の切り替え、`my serve` の停止 |
| 7. 破棄 | — | DB・Blob・バックアップの破棄。ドメインはおよそ 1 年持ち続ける |

どの段階でも、メンテナンスモード（[Maintenance](maintenance.md)）を重ねて使えます。メンテナンスの間は、エクスポートも `Delete` の配送も止まります。

## 1. 予告

閉鎖日（`-closes-at`）とデータの持ち出しの期限（`-export-until`、省くと閉鎖日の 30 日後）を決めて予告します。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "ご利用ありがとうございました。"
my admin closure status
```

- 閉鎖日に下限はありませんが、予告から閉鎖日までが 30 日より短いと警告が出ます。利用者がエクスポートと移転をする時間を取ってください。
- 日付を変えるときは `announce` をもう一度、取りやめるときは `my admin closure withdraw` を実行します。どちらも全利用者に通知 `service_closure` が届きます。
- 予告の開始で、登録の実効値が `closed` になり、審査待ちの申請は却下され、続いているサポーターの加入はすべて期間の終わりで解約されます（加入者には通知とメール）。
  取り消しても、却下と解約は戻りません。
- 通知はジョブで作るので、ワーカーを止めないでください。
- ステータスページ（[ADR-0085](../adr/0085-uptime-monitoring-status-page.md)、[外形監視とステータスページ](monitoring.md)）に、閉鎖日・持ち出しの期限・エクスポートと移転の方法を掲出します。
  サーバーが止まった後もステータスページは残るので、閉鎖の後に訪れた人への案内にもなります。

## 2. 告知の期間

- 利用者は、設定の画面からデータのエクスポート（FR-008 と CSV）と、ほかのサーバーへの移転（`Move`、[ADR-0089](../adr/0089-account-migration-move.md)）ができます。
  全画面の帯と通知が、閉鎖日・持ち出しの期限・その画面への導線を示します。
- **年払いのサポーター**: 予告で加入は期間の終わりに解約されますが、期間の終わりが閉鎖日より後の加入は、使えない期間の分も支払われています。
  サーバーは返金しないので、返金は決済事業者の画面（Stripe ならダッシュボードの支払いの画面、PAY.JP なら管理画面の支払いの画面）で、加入者ごとに行います（[サポーターの決済（Stripe）](payments-stripe.md)・[サポーターの決済（PAY.JP）](payments-payjp.md)）。
- 閉鎖日を過ぎると、`manage_service_closure` を持つ人に通知 `admin_service_closure_due` が届きます。サーバーは自動ではクローズしません。

## 3. クローズ

閉鎖日を過ぎたら、クローズの前に点検します。

```sh
my admin closure check
```

- **終わっていない移転**があると警告が出ます。移転が終わるのを待ってからクローズしてください。クローズの後は `Move` を始められません。
- **保全（`retention_holds`、[ADR-0038](../adr/0038-moderation-personal-data-retention.md)）**のあるアカウントがあると警告が出ます。
  保全の義務はサーバーを閉じても消えないので、保全の対象の記録を書き出して、サーバーの外に保管してください（保全の一覧は `my admin retention list`）。

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

点検を終えたらクローズします。取り消せません。`y` とドメイン名の入力で確かめます（自動化には `-confirm-domain`）。

```sh
my admin closure close
```

クローズの後、利用者はサインインしてデータのエクスポートだけができます。残っていたサポーターの加入はすぐに解約され、
全ローカルアカウントの `Delete`（Actor）が連合先へ配送されます（`federation_mode` が `off` / `inbound` でも送ります）。
inbox と、投稿・コレクションの GET は `410 Gone` になります。

## 4. 配送の完了の確認

**`delivery_completed` になるまで、`my serve` とワーカー（`role` が `all` または `worker` のプロセス、push 型のキューなら Cloud Tasks のキューや SQS のイベントソース）を止めないでください。**
`Delete` を配送するのはワーカーで、配送の間はローカルの Actor の文書と鍵を返し続けて、受け手が `Delete` の署名を検証できるようにしています。
途中で止めると、連合先にアカウントが残ります。

```sh
my admin closure status
```

- `accounts with a Delete` が `Delete` を記録したアカウント数、`fan-outs pending` が展開の残り、`deliveries <state>` が配送の状態ごとの件数です。
- 配送は既存の方針で再試行し（最大 9 回、約 4 日）、すべて終わると 10 分ごとの確認が `delivery_completed` を記録します（`state: delivery_completed`）。
  その後はローカルの Actor とインスタンス Actor も `410` です。
- `dead` になった配送は自動では再実行しません。再実行するときは `my job retry` を使います。

## 5. 持ち出しの期限まで

配送の完了の後も、サーバーは `export_until` まではクローズのまま動き、利用者のエクスポートを提供します。
サーバーは自分ではデータを消さないので、期限を過ぎても止めるまでは動き続けます。
保全の記録の書き出し（§3）がまだなら、ここで済ませます。

## 6. 静的なページへの切り替え

持ち出しの期限を過ぎたら、閉鎖のページを書き出します。**DB を読むので、`my serve` を止めて DB を破棄する前に実行します。**

```sh
my admin closure export-html -out ./closure             # 全ロケール。index.html はインスタンスの既定のロケールから
my admin closure export-html -out ./closure -lang ja,en # ロケールと index.html の順を決める
```

- 書き出すもの:
  - `<dir>/<locale>.html` と、全ロケールを順に並べた `<dir>/index.html`。メンテナンスのページ（`my maintenance export`）と同じテンプレートと CSS で、
    サーバー名（`maintenance_server_name`、無ければインスタンスの名前、無ければホスト名）・閉鎖日・予告の運営者のメッセージ・ステータスページへのリンク（`status_page_url`）を示します。
    インライン CSS だけで、スクリプトも外部の資源も持たず、CSP は `<meta>` に入っています。テーマの色の上書きは焼き込み、カスタム CSS は入れません。
  - 前段の設定例 `nginx.conf`・`Caddyfile`・`cloudfront-function.js`。どれも、連合と発見の経路（`/ap/*`・`/inbox`・`/.well-known/webfinger`・
    `/.well-known/host-meta`・`/.well-known/host-meta.json`・`/.well-known/nodeinfo`・`/nodeinfo/*`）とプロフィール（`/@*`）に本文なしの `410` を返し、
    それ以外のパスに、`Accept-Language` の最初の言語のページ（無ければ `index.html`）を返します。
- サーバーがクローズしていないと断ります（終了コード 1）。`delivery_completed` の前は、配送が続いていると警告を出します（書き出しはします）。

前段を切り替えます。ページ（`*.html`）だけを公開の場所に置き、設定例のホスト名・ディレクトリ・証明書のパスを環境に合わせて直します。

| 前段 | 置き方 |
| --- | --- |
| nginx | ページを `/srv/closure` に置き、`nginx.conf` を `http` ブロックから読み込む。ページも `410` で返す |
| Caddy | ページを `/srv/closure` に置き、`Caddyfile` を使う。証明書は Caddy が取り、ページも `410` で返す |
| CloudFront | ページを S3 のバケットの直下に置き、そのバケットを既定のキャッシュビヘイビアのオリジンにして、`cloudfront-function.js` をビューアリクエストの CloudFront Function（`cloudfront-js-2.0`）として関連付ける。キャッシュポリシーのキャッシュキーに `Accept-Language` を入れる。ページは `200` で返る（ビューアリクエストの関数はオリジンの応答の状態を変えられない） |

切り替えた後、`curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` が `410`、
`curl -i https://social.example/` が閉鎖のページを返すことを確かめてから、`my serve` とワーカーを止めます。
外形監視は、ヘルスチェックの監視を止めるか、閉鎖のページを見るものに替えます。

## 7. DB・Blob・バックアップの破棄

止めた後、DB・Blob（メディアとエクスポートのファイル）・KV・キュー、それぞれのバックアップとスナップショットを破棄します。
保全の対象の記録を書き出して保管したことを、破棄の前にもう一度確かめてください。破棄は取り消せません。

**ドメインは手放さず、およそ 1 年は静的なページと `410` を返し続けます。** 連合先は `410` を見て取得と配送を止め、キャッシュを消していきます。
URI は UUID（[ADR-0018](../adr/0018-federated-uris.md)）なので同じ `id` の Actor が作られる心配は小さいですが、
ドメインを別の人が取ると、ハンドル（`acct:alice@social.example`）と表示用の URL が別の人のものになり、なりすましに使えます。
ドメインと証明書の更新を続け、1 年を過ぎても、手放す前にもう一度考えてください。
