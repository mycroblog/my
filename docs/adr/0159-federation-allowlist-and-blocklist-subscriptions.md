# ADR-0159: 許可したドメインだけと連合するモードと、ドメインのブロックリストの購読

- Status: Proposed
- Date: 2026-11-09
- Deciders: @rosylilly
- Issue: #2579（タスク #2580・#2581・#2582）

## Decision（決定）

**許可したドメインは処分とは別のテーブルに持ち、インスタンス設定 `federation_scope` が `allowlist` のとき、許可の無いドメインを、ドメインの制限を判定する一か所で `suspend` と同じに扱う。他者が公開するブロックリストは、運営者が設定した URL でも `kernel.HTTPClient` で取得し、下書き（既定）と自動の 2 つのモードで購読する。自動のモードは人が下した処分に触れない。**

§1・§2・§5 の一部（許可のテーブルを分けること、ブロックリストの取得経路、2 つのモード）は decider の決定で、ほかは推奨案です。覆る条件は §10 に書きます。

### 1. 許可したドメインのテーブル（決定）

許可は `domain_moderation_actions` に入れず、`moderation` が持つ別のテーブルにする。ドメインの処分と、その投影の `domain_restrictions` は「何を止めるか」を列挙するもので、許可はその逆の意味を持つからである。

- `domain_allows`【リソース】: `domain`（PK。正規化したホスト、IDNA の A-label）, `comment`（NULL 可。5,000 文字まで。公開しない）, `created_by_user_id`（NULL 可 = CLI）, `created_at`。外すときは行を消す（作成と削除は §8 の監査ログに残る）。
- 許可はサブドメインにも効く。判定は、処分と同じく対象ドメインと親ドメイン群を列挙して `domain IN (…)` で引く。
- 許可は処分を打ち消さない。許可したドメインに処分があれば、処分が効く（GoToSocial の allowlist モードと同じく、ブロックが許可に勝つ）。
- `federation_scope` が `open` の間、許可は判定に何の影響もしない。`open` に戻しても行は消さず、次に `allowlist` にしたとき同じ許可が効く。

### 2. 連合の範囲 `federation_scope`（推奨案）

- インスタンス設定 `federation_scope`（`KindEnum`、`open` / `allowlist`、既定 `open`）を足す。`federation_mode`（ADR-0006、向き）と独立の軸で、`instance_setting_revisions` に記録し、リクエストとジョブごとに読む。
- `allowlist` のとき、許可の無いドメインは、`moderation.FederationPolicy` がドメインの制限を返すところ（`DomainRestrictions` とトランザクションの中の読み取り。federation.md §11 の表が読む唯一の場所）で、`suspend` が生む制限（`federation`・`profile_view` ほか、data-model §5.2）を持つものとして返す。受信・配送・取得・メディアプロキシ・リレーの各入口は今の判定のまま、新しい分岐を持たない。
- 投影の `domain_restrictions` には行を書かない。許可の無いドメインは数に上限がなく、判定のときに `domain_allows` と設定から合成する。
- `allowlist` のとき、ローカルの公開の投稿・Actor の GET は、Authorized Fetch が無効でも、許可したドメインの署名を求める（署名の無い GET は、許可の無いドメインの取得と区別できないため。Mastodon の LIMITED_FEDERATION_MODE が secure mode を前提にするのと同じ）。
- NodeInfo の `metadata` に `federationScope` を出す（`federationMode` と同じ扱い）。

### 3. 切り替えと許可を外すときの影響（推奨案）

- `allowlist` に切り替える前に、管理 API と管理画面は、切れるフォローの件数（両方向の承認済みのフォロー）、影響を受けるローカルのアカウントの数、件数の多いドメインを示し、確認を求める。CLI は `-apply` を付けるまで件数だけを出す（CSV のインポートと同じ）。
- 切り替えたとき、および `allowlist` の間に許可を外したときは、ドメインの `suspend` と同じく両方向のフォローを `follow_terminations`（`reason = moderation`）で切り、`relationship_severances` に記録して、影響を受けたローカルの利用者に件数を通知する（FR-717）。仕事はジョブ（`moderation.sever_domain` と同じ形の、ローカルのアカウントごとに 1 トランザクション）で行う。
- 切った理由の行として `allowlist_cutoffs`【イベント】（`id`, `cause`（`enum(scope_switched, allow_removed)`）, `domain`（`allow_removed` のときだけ値を持つ）, `issued_by_user_id`（NULL 可 = CLI）, `occurred_at`）を足し、`relationship_severances` に `allowlist_cutoff_id`（NULL 可）を加えて、`domain_action_id`・`action_id` と合わせて 1 つだけが値を持つ `CHECK` にする。
- `open` に戻しても、切ったフォローは戻さない（処分の解除と同じ）。

### 4. ブロックリストの取得（決定）

購読するブロックリストは、運営者が設定した URL でも `kernel.HTTPClient`（SSRF ガード付き、ADR-0019 §1）で取得する。リストは第三者のサーバーにあり、その第三者がリダイレクトで内部のアドレスへ向けられるからである。運営者だけが決める宛先に専用の `*http.Client` を使う規則（ADR-0146 §7）の対象にはしない。`AGENTS.md` の Design rules と ADR-0019 §1 も同じことを書く。

- スキームは `https` だけ（`kernel.HTTPClient` の既定）。応答の上限は 2 MiB、5,000 件（CSV のインポートと同じ件数）。超えたら失敗にして何も変えない。
- `ETag` / `Last-Modified` で条件付き GET にする。`304` は「変更なし」として記録する。
- ドメインの処分（`Fetcher` の検査）はこの取得に掛けない。ActivityPub の取得ではないため。

### 5. 購読（2 つのモードは決定、ほかは推奨案）

- `domain_list_subscriptions`【リソース】: `id`, `title`, `url`, `format`（`enum(mastodon_csv, plain_text)`）, `mode`（`enum(draft, automatic)`、既定 `draft`）, 重さの対応（`suspend_as`（`enum(suspend, limit, ignore)`、既定 `suspend`）, `silence_as`（`enum(limit, ignore)`、既定 `limit`）, `apply_reject_media`・`apply_reject_reports`（`bool`、既定 真））, `public_comment_from_list`（`bool`、リストの公開コメントを写すか）, `created_by_user_id`, `created_at`。
  - `mastodon_csv` は、今のインポート（`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`）と同じ読み方をし、重さを上の対応で置き換える。`plain_text` は 1 行 1 ドメインで、各行を `#severity = suspend` の行として読む（Mastodon のインポートと同じ）。
- 取得は毎日 1 回（定期ジョブ `moderation.fetch_domain_lists` が購読ごとに `moderation.fetch_domain_list` を積む）と、管理画面・CLI からの「今すぐ取得」。ジョブは冪等で、取得（外部 I/O）とその後の反映のトランザクションを分ける。
- 取得ごとに `domain_list_fetches`【イベント】（`id`, `subscription_id`, `outcome`（`enum(applied, not_modified, failed)`）, `http_status`（NULL 可）, `error_code`（NULL 可）, `entry_count`, `fetched_at`）を記録する。`domain_list_entries`【投影】（`subscription_id`, `domain`, 置き換えた後の処分の種類の集合, `public_comment`, `obfuscate`）は最後に成功した取得の中身で、`domain_list_subscription_heads`【現在値】が `etag`・`last_modified`・連続の失敗数を持つ。
- 購読が下した処分は `domain_moderation_actions` の行のまま、`subscription_domain_actions`【イベント】（`action_id`（PK）, `subscription_id`, `fetch_id`）でどの購読のものかを記録する。処分の表・期限・解除・webhook・公開（FR-718）は今の経路をそのまま通る。
- **下書き**（既定）: 取得の差分（足す・重さが変わる・外れた）を、購読ごとの下書きとして並べる。`manage_domains` を持つ人が下書きを 1 件ずつかまとめて受け入れるか退ける。受け入れた処分も `subscription_domain_actions` に記録し、`issued_by_user_id` は受け入れた人にする。下書きは `domain_list_entries` と購読の処分の差分から導き、退けたものは `domain_list_draft_rejections`【イベント】（`subscription_id`, `domain`, 退けた項目の内容, `rejected_by_user_id`, `rejected_at`）に記録して、その項目の内容が変わるまで再び出さない。
- **自動**: 差分をそのまま反映する。足す・変わる行は処分を発行し（変わるときは購読の古い処分を解除してから）、外れた行は、その購読が下した処分だけを解除する。
- **人の処分には触れない**（決定）: 同じドメインか親ドメインに同じ種類の処分が効いていれば、購読は発行しない（CSV のインポートの「適用済み」と同じ）。購読は、`subscription_domain_actions` に記録の無い処分を、解除も置き換えもしない。GoToSocial の `adopt_orphans` に当たる「人の処分を購読のものにする」機能は持たない。
- 複数の購読が同じドメインを載せるときは、先に発行した購読の処分だけが効き、ほかの購読は「適用済み」とする。その購読が処分を解除したときは、同じジョブの中で、まだそのドメインを載せている自動の購読の項目（`domain_list_entries`）から処分を発行し直す。購読に優先度は持たない。
- 取得に失敗したときは処分を何も変えない。3 回続けて失敗したら、`manage_domains` を持つ人に通知する。
- 自動のモードでも、1 回の取得で外れる項目が、その購読の処分の半分を超え、かつ 10 件を超えるときは、外す分だけを下書きに回す（壊れた応答や空のリストで一度に解除しないため）。
- 購読を消すときは、その購読が下した処分を残すか解除するかを選ぶ（既定は残す。残した処分は人の処分と同じ扱いになる）。
- 許可のリストの購読は持たない（§10）。

### 6. 権限と画面（推奨案）

- 許可したドメインと `federation_scope` は `manage_settings`（サーバー管理）: `/admin/settings/federation` に範囲の切り替えを置き、許可の一覧と追加・削除を `/admin/settings/federation/allowed-domains` に置く。`federation_mode` と同じく、サーバー全体の連合の方針を決める設定だからである。
- 購読と下書きは `manage_domains`（モデレーション）: `/moderation/domain-actions/subscriptions`・`/new`・購読ごとの `/{id}`（取得の履歴と下書き）。購読が生むのはドメインの処分そのものだからである。
- API は `/api/v1/admin/domain_allows`（タグ `admin-server`）と `/api/v1/admin/domain_list_subscriptions`（タグ `admin-moderation`）。CLI は `my admin domain allow …` と `my admin domain subscription …`。
- 採ったら、ADR-0058 §1 の画面と権限の表と §4 に行を足す。

### 7. コードの置き場所（推奨案）

- テーブルとユースケースは `moderation` が持つ（`schema/03-moderation`、`internal/app/moderation`）。`federation_scope` の値は `instance` の設定にあり、`moderation.FederationPolicy` が `internal/app/instance` から読む。
- 取得は `kernel.HTTPClient` を `internal/platform` から受け取る。リストの解析（CSV と平文）は今の CSV のインポートと同じ関数を使う。

### 8. 監査ログと webhook（推奨案）

- 監査ログ（ADR-0058 §7）: `target_kind` に `domain_allow`・`domain_list_subscription` を足し、`action` に `domain_allow_added` / `domain_allow_removed`、`domain_list_subscription_created` / `_updated` / `_deleted`、`domain_list_draft_accepted` / `_rejected`、`domain_list_fetch_queued` を足す。`federation_scope` の変更は今の `setting` の記録に乗る。
- 自動のモードが発行・解除した処分は、今の `domain_action_issued` / `domain_action_lifted` を `actor_kind = system` で記録し、`source_event_id` から購読と取得を引ける。
- webhook（FR-722、ADR-0060）: 購読が下した処分も `domain_moderation_action.created` / `.lifted` を送る（ペイロードに `subscription_id` を足す）。許可は `domain_allow.created` / `.removed` を足し、権限は `manage_settings` とする。

### 9. 既存の機能との関係

- CSV のインポート・エクスポート（FR-706）はそのまま残す。購読は同じ解析を使う、定期のインポートである。エクスポートは購読の処分も含めて出す。
- 判定フック（ADR-0147）と同じく、外の判断（リストの公開者）を人が確かめる下書きを既定にする。

### 10. この決定を見直す条件

- 許可のリストを他者と共有する需要（研究機関や組織の連合）がはっきりしたら、許可の購読を足す（GoToSocial は許可の購読も持つ）。
- 購読の重なりで「先に発行した購読が効く」が運営者の意図と食い違う報告が続いたら、GoToSocial のような優先度を足す。
- 署名の無い GET を `allowlist` で拒むことが、許可したドメインの実装（Authorized Fetch を使わないサーバー）との連合を壊すと分かったら、§2 の GET の扱いを見直す。

## 経緯

FR-413 が連合の向きを、FR-702 がドメインの処分を、FR-706 が Mastodon の CSV のインポートとエクスポート（プレビュー付き）を決めている。足りないのは、決めたドメインとだけ連合する方法と、他者が保守するブロックリストに追従する方法だった（#2579）。

- 許可を `domain_moderation_actions` の新しい種類にすると、処分の一覧・公開（FR-718）・投影が「止めるもの」の列挙でなくなるので、テーブルを分けると decider が決めた。
- ブロックリストの URL は運営者が設定するが、置き場所は第三者のサーバーでリダイレクトもされうるので、ADR-0146 §7 の「運営者だけが決める宛先」には当たらず、`kernel.HTTPClient` を通すと decider が決めた。
- 購読のモードは、GoToSocial の `as_draft` と同じく下書きと自動を最初から持ち、自動でも人の処分に触れないと decider が決めた。
- 判定を `moderation.FederationPolicy` の一か所に集めた今の構成（federation.md §11）があるので、`allowlist` を「許可の無いドメインは `suspend`」として、そこだけで足す形にした。

参照した実装（2026-11-09 確認）:

- GoToSocial の連合のモード（[Federation Modes](https://docs.gotosocial.org/en/latest/admin/federation_modes/)）: `instance-federation-mode` が `blocklist`（既定）/ `allowlist`（実験的）。`allowlist` ではブロックが許可に勝ち、`blocklist` では明示の許可がブロックに勝つ。切り替えでフォローがどうなるかは書かれていない。
- GoToSocial のドメインの権限の購読（[Domain Permission Subscriptions](https://docs.gotosocial.org/en/latest/admin/domain_permission_subscriptions/)）: CSV・JSON・平文、既定で 24 時間ごと。`as_draft`・`remove_retracted`・`adopt_orphans`、0〜255 の優先度、自動で作らないドメインの除外（excludes）、許可の購読も持つ。ブロックの購読は `suspend` だけを扱い、`silence` などは読み飛ばす。失敗時の扱いとサイズの上限は書かれていない。
- Mastodon の `LIMITED_FEDERATION_MODE`（[Configuring your environment](https://docs.joinmastodon.org/admin/config/)）: 手で許可したサーバーとだけ連合し、公開のページと一部の REST API を止め、secure mode（`AUTHORIZED_FETCH`）を前提にする。このモードの間はドメインのブロックが効かず、許可（DomainAllow）だけが効く。既存のデータは `tootctl domain purge --limited-federation-mode` で消す。Mastodon 本体はブロックリストの購読を持たない（要検証: 外部のツールで補う運用が一般的）。

## 採らなかった案

- 許可を `domain_moderation_actions` の種類にする: テーブルとユースケースが 1 つで済むが、処分の列挙・公開・投影の意味が逆転し、許可を処分の一覧から除く例外がどこにでも要る（decider の決定）。
- ブロックリストを専用の `*http.Client`（ADR-0146 §7 の形）で取得する: 社内のアドレスに置いたリストも読めるが、第三者のサーバーがリダイレクトで内部に向けられる（decider の決定）。社内のリストは CSV のインポートで足りる。
- `allowlist` のとき、Mastodon のようにドメインの処分を効かなくする: 単純だが、許可したドメインを `limit` や `reject_media` にできなくなる。GoToSocial と同じく処分を効かせる。
- 許可の無いドメインを投影の `domain_restrictions` に書く: 判定の読み取りが今のまま済むが、許可の無いドメインは数え上げられない（知らないドメインも対象）。
- 自動のモードに `adopt_orphans` を持つ: 手で入れた処分もリストに追従させられるが、リストからの削除で人の判断が消える。人の処分に触れないという決定に反する。
- 購読に GoToSocial のような優先度を持つ: 重なりの扱いを運営者が決められるが、`suspend` と `limit` の対応を購読ごとに持つ今の形では、重なりは少なく、先に発行した購読が効く形で足りる。
- 許可の購読も最初から持つ: GoToSocial にはあるが、許可を第三者に任せる需要がまだ見えず、自動で許可を足すと allowlist を選んだ理由が薄れる。

## Consequences（影響）

- 良い: 判定の場所が増えず、各入口は今の `FederationPolicy` を聞くだけで `allowlist` に従う。購読の処分は今の処分と同じ行なので、一覧・期限・解除・公開・webhook がそのまま使える。
- 良い: 下書きが既定なので、リストの公開者の判断がそのまま連合を切ることはない。自動でも、壊れた応答で一度に解除しない。
- 悪い: `allowlist` への切り替えは、大きなインスタンスでは多くのフォローを一度に切り、戻せない。プレビューと確認で防ぐしかない。
- 悪い: `allowlist` で署名の無い GET を拒むと、署名しない実装やクローラーからは公開の投稿が見えなくなる。
- 悪い: 判定のたびに `domain_allows` と設定を読むので、`allowlist` では読み取りが 1 つ増える（ドメインの処分と同じキャッシュの対象にする）。
- 悪い: スキーマに `domain_allows`・`allowlist_cutoffs`・購読の 6 つのテーブルが増え、`relationship_severances` の `CHECK` が変わる（v0.1.0 の後なら、前の `main` から上げられる形にする）。
- 後続（このマイルストーンのタスク）: #2580 許可のテーブル・`federation_scope`・切り替えのプレビューと切断、#2581 購読・取得・下書き・自動の反映、#2582 管理画面。採ったら ADR-0058 §1・§4・§7、ADR-0060 のイベントの表、federation.md §11、data-model §4.3、運用の文書（`docs/operations/moderation.md`）を合わせる。

## Open Questions / 要検証

- 権限: 許可と `federation_scope` を `manage_settings`、購読を `manage_domains` に分ける推奨案でよいか。`allowlist` の間の許可の削除はドメインの凍結と同じ効果を持つので、許可も `manage_domains` に置く案もある（decider が決める）。
- `allowlist` の間、ローカルの公開の投稿・Actor の署名の無い GET を拒む（§2）か。拒まないと、許可の無いサーバーも公開の投稿を読める。
- 自動のモードで、一度に外れる項目を下書きに回すしきい値（半分かつ 10 件）でよいか。
- 応答の上限（2 MiB・5,000 件）は、公開されている大きなブロックリストに足りるか（要検証: 主要な公開リストの件数を実装時に確かめる）。
- GoToSocial の JSON 形式（Mastodon の `/api/v1/instance/domain_blocks` の応答）を購読の形式に足すか。足せば、処分を公開している他のサーバーをそのまま購読できる。
