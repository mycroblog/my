# データモデル

> 前提にしている ADR: [ADR-0003](../adr/0003-database-abstraction.md)、[ADR-0020](../adr/0020-data-modeling-principles.md)。

各モジュールのデータモデル（実体・その一生と状態・列の理由・実体の間の不変条件）は、そのパッケージの `doc.go` にあります。表の形は `schema/<NN>-<module>/<dialect>.sql` です。ここには、1 つのモジュールに収まらないことだけを書きます。

## Identity, accounts, moderation, instance

- 利用者（`users`）はログインの主体で、ローカルのアカウント（`local_accounts`）として名乗ります。アカウント（`accounts`）はローカルとリモートの部分型を持ち、プロフィールの版・ライフサイクル・移転のイベントを持ちます。処分（`moderation_actions`）はアカウントに下され、通報（`reports`）に基づくことがあり、解除のイベントで終わります。
- 投稿・ブースト・お気に入り・リアクション・フォロー・ブロック・ミュートのコマンドは、自分のモジュールのリソースかイベントを書き、同じトランザクションで、それを購読するモジュールの同期の投影（タイムラインの `feed_*`、通知とその除外、カウンタ）を更新し、配送のジョブを outbox に積みます。処分も同じで、制限の投影と、タイムラインがその制限を写した `feed_author_restrictions` を同じトランザクションで書きます。
- タイムラインの SQL は、`moderation` の制限を `feed_author_restrictions`・`feed_domain_restrictions` に写した投影で絞ります。写像の規則は `moderation.AccountPolicy` の判定と同じです。

## Content, graph, timelines, projections

- リモートの投稿・ブースト・お気に入り・リアクションは `federation` のハンドラが受け取り、取り込むかを先に決めてから（ADR-0006）、1 つのトランザクションで `content` の公開 API を呼びます。ローカルのコマンドと同じドメインイベントを出すので、1 つの投影が両方を扱います。
- データを消す順序は 1 つです。削除をイベントとして記録し、同じトランザクションで投影から外してすぐに見えなくし、物理的な削除は件数を区切って動くジョブに任せます。
