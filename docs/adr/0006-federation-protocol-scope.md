# ADR-0006: 連合の範囲と受信・取り込み

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #8, #232, #486, #651, #976

## Decision（決定）

### 1. 対応する仕様・拡張（v0）

| 区分 | 対象 |
| --- | --- |
| 必須 | ActivityPub（S2S のみ）、ActivityStreams 2.0、WebFinger（RFC 7033）、host-meta、NodeInfo 2.1 |
| 署名 | HTTP Signatures draft-cavage-12（`rsa-sha256` / `hs2019`）と RFC 9421、**double-knocking**、`Digest` / `Content-Digest`（RFC 9530）。`public` の投稿の `Create` / `Update` / `Delete` に Linked Data Signatures（`RsaSignature2017`）を付ける（§19）。受信時の LD 署名の検証は互換のため検討 |
| 互換拡張 | Mastodon 拡張（`toot:` 名前空間: `featured`, `discoverable`, `indexable`, `Emoji`, `sensitive`, `Hashtag` 等）、`contentMap`。受信の互換拡張に LitePub の `EmojiReact` と Misskey の `_misskey_reaction`（ADR-0051）、FEP-044f の語彙と `_misskey_quote`・`quoteUrl`・`quoteUri`（ADR-0069） |
| リレー | Mastodon 方式と LitePub 方式の参加、`public` の投稿の送信と受信（§18〜§22） |
| アカウントの移転 | `Move` と別名（`alsoKnownAs`）（ADR-0089） |
| 任意 | Authorized Fetch（secure mode）、Object Integrity Proofs（FEP-8b32）は C |
| 非対応 | AP C2S |

### 2. 相互運用テスト対象

Mastodon 4.x、Misskey、GoToSocial、Akkoma（Pleroma 系）。余力があれば Pixelfed、Threads。

### 3. 実装方針

- ライブラリ（Go）は次の構成とする（根拠は [#15 の調査報告](https://github.com/mycroblog/my/issues/15#issuecomment-6209363986)）。
  - 署名: RFC 9421 は `github.com/dadrus/httpsig`（MIT）、draft-cavage-12 は `code.superseriousbusiness.org/httpsig`（BSD-3-Clause、go-fed/httpsig の GoToSocial フォーク）。どちらも cgo は不要。
  - 語彙（AS2 / Mastodon 拡張）は自前の型で持つ。`go-ap/activitypub` は未知のプロパティを捨て、タグ付きリリースもないため、コアの型にはしない（受信パースの補助に使うかは実装時に判断）。`go-ap/client` / `go-ap/auth` は依存が重く、go-ap 系の抽象に結合しているため採用しない。
  - ライブラリが担うのは署名ベースの組み立て・暗号処理・`Content-Digest` の検証まで。受け入れポリシー（必須ヘッダ / コンポーネント、時間窓、`Digest` の検証、鍵の所有者と `actor` の一致）、鍵の解決、送信側の double-knocking、配送、Mastodon 拡張の解釈は自前で書く。ライブラリはアダプタ境界の内側に閉じ込める。
- 署名の double-knocking: 受信の振り分けは §4、送信の順序と再送は §5。
- `@target-uri` / `(request-target)` は設定した公開オリジンから組み立て直し、`X-Forwarded-*` は信用しない。
- Inbox は署名検証まで同期、処理は非同期（NFR-PERF-4。§6）。
- **HTTPClient ポートに SSRF ガード** を組み込み、すべての外部取得を通す（NFR-SEC-1）。
- 受信した HTML は許可リストでサニタイズ（NFR-SEC-2）。
- 公開範囲 `local` の配送先・オブジェクト取得の制限は [ADR-0017](0017-visibility-local.md) に従う。
- 配送はドメイン単位に shared inbox へ集約。連続失敗ドメインはサーキットブレーカーで一時停止し、`InstanceHealth` に記録。
- 互換性のためのテストは「実サーバーから採取した JSON フィクスチャ」を `testkit` に蓄積して回帰テストにする。
  実サーバーとのライブ相互運用テスト（docker compose で Mastodon / GoToSocial を起動）は nightly ジョブとする。

### 4. HTTP 署名の受信

| 項目 | draft-cavage-12 | RFC 9421 |
| --- | --- | --- |
| 選ぶ条件 | `Signature-Input` がない | `Signature-Input` がある（`Signature` と両方あれば RFC 9421 だけを検証する） |
| 必須の対象 | `(request-target)`、`host`、`date`（または `(created)`）。POST は加えて `digest` | `@method`、`@target-uri`（または `@authority` + `@path`、クエリ文字列があれば `@query`）、パラメータ `keyid` と `created`。POST は加えて `content-digest` |
| ダイジェスト | `Digest: SHA-256=…`（RFC 3230）。`SHA-512` も受ける | `Content-Digest`（[RFC 9530](https://www.rfc-editor.org/rfc/rfc9530)）の `sha-256` / `sha-512` |
| アルゴリズム | `rsa-sha256`、`hs2019`（鍵の種類から決める） | `alg` があれば鍵の種類と一致することを確かめ、なければ鍵の種類から決める。RSA（2048 ビット以上）の `rsa-v1_5-sha256` / `rsa-pss-sha512` と `ed25519` |
| 時間窓 | `Date` が過去 12 時間より古い・1 時間より未来なら拒否 | `created` が 1 時間より未来なら拒否。`expires`（無ければ `created + 12 時間`）を 1 時間過ぎていたら拒否 |
| クエリ文字列 | クエリ文字列付きで検証し、合わなければクエリ文字列なしで 1 回だけ検証し直す | `@target-uri` は署名されたとおりに組み立てる（再検証しない） |

- `@target-uri` と `(request-target)` は設定の公開オリジンとリクエストのパス・クエリから組み立て、`X-Forwarded-*` は信用しない（§3）。
- 鍵の解決: `keyid` / `keyId` の値そのもので `account_keys` を引く（Mastodon は `{actor}#main-key`、GoToSocial は `{actor}/main-key`）。無い・取得から 24 時間を過ぎている場合は、フラグメントを除いた URL を取得する。
  取得した文書が `Key` なら `owner`（または `controller`）の Actor をたどり、`publicKey`（オブジェクト・配列・ID のどれでもよい）から `id == keyId` のものを使う。
  FEP-521a の `assertionMethod`（`Multikey`）があれば、Ed25519 の鍵もそこから読む。
- 検証に失敗したら、`keyId` ごとに 5 分に 1 回まで Actor を取り直して 1 回だけ再検証する（鍵のローテーション。ADR-0019 §7）。同じ `keyId` で公開鍵が変わったら、古い鍵を失効させて新しい鍵を記録する。
- `Accept-Signature`（[RFC 9421 §5](https://www.rfc-editor.org/rfc/rfc9421#section-5)）は、401 の応答に RFC 9421 の必須の対象を載せて返す。

### 5. HTTP 署名の送信

- 既定は draft-cavage-12、`algorithm="rsa-sha256"`、`keyId` は `{actor}#main-key`。
  POST は `(request-target) host date digest content-type`（`Digest: SHA-256=…`）、GET は `(request-target) host date` に署名する。
- 400 / 401 が返ったら、RFC 9421 で 1 回だけ再送する。対象は `("@method" "@target-uri" "content-digest");created=…;expires=…;keyid="…#main-key";alg="rsa-v1_5-sha256"`（`expires` は `created + 5 分`）。
- 再送が成功したら、そのホストは RFC 9421 で送ることを KV に 7 日記憶し、以後は RFC 9421 を先に試す（失敗したら draft-cavage で 1 回だけ再送する）。
- 相手の 401 に `Accept-Signature` があれば、それに従って RFC 9421 で再送する。
- GET（Actor・オブジェクトの取得）も同じ規則で、インスタンス Actor の鍵で署名する。
- `ed25519` での送信は、主要実装の受信の対応を確かめるまで既定にしない（Mastodon 4.7 以降は受信できる）。

### 6. Inbox の同期の部分

本文の上限（1 MiB）→ IP のレート制限 → 署名ヘッダの解析 → 署名者のドメインのブロック判定 → ダイジェストの照合 → 鍵の解決（未知・期限切れなら、署名した Actor をインスタンス Actor の署名付き GET で取得。5 秒で打ち切る）→ 署名の検証（§4）→ 本文の JSON から `id` / `type` / `actor` だけを読む → 署名者と `actor` の照合 → 重複排除の行と原本とジョブを 1 つのトランザクションで記録 → `202`。

- 鍵を取得できなければ `401` を返し、送信側のリトライに任せる。
- 未知の Actor の自分自身の `Delete` は、鍵を取りに行かず `202` で捨てる（ADR-0019 §7）。

### 7. 重複排除は受信時

`received_activities (activity_uri)` に `ON CONFLICT DO NOTHING` で挿入し、行が増えたときだけ原本（`received_activity_documents`）と `federation.inbox` ジョブを同じトランザクションで記録する。
再送された同じアクティビティはジョブを作らない。処理に失敗したアクティビティは dead job として残り、CLI から再実行する（FR-801）。

### 8. 署名者と `actor` が一致しないアクティビティ

転送・リレーのアクティビティは、ジョブに「信用しない」の印（`trust = refetch`）を付け、ワーカーがオブジェクトを `id` からオリジンに取り直して検証する（[ActivityPub 付録 B.2](https://www.w3.org/TR/activitypub/#security-verification)、ADR-0019 §7）。

### 9. 順序はキューに頼らない

ジョブには `orderingKey = inbox:<actor の URI>` を付ける（#77 の後、同じ Actor のアクティビティは直列化される）が、ハンドラはどの順で届いても同じ結果になるように書く。

- 未知のオブジェクトへの `Delete` は tombstone（`remote_object_tombstones`、保持期間は既定 30 日）を記録し、後から届いた `Create` / `Update` を捨てる。
- 未知のオブジェクトへの `Update` は、取り込み範囲（§11）に入るなら `Create` と同じく扱う。
- 版は `updated` が現在版より新しいときだけ作る（[data-model §2.2](../architecture/data-model/02-consistency.md#22-冪等キーと一意制約)）。
- 対象が見つからない `Undo` / `Accept` / `Reject` / `Like` / `Announce`（ローカルの投稿宛て）は何もせず完了にする。

### 10. 未知の型

対応表にないアクティビティ・オブジェクトの型は、ログとメトリクスに残して完了にする（`202` は同期の時点で返している）。

### 11. 取り込む `Create`

ストレージと書き込み行数はローカルの利用者数とフォロー数に比例させ、連合先の規模に比例させない。外部 HTTP の回数は、利用者の操作に比例する範囲に抑える（取得の増幅を防ぐ）。

受信した `Create` のオブジェクトを保存するのは、次のどれかに当てはまるときだけ。どれにも当てはまらなければ、受信の記録だけを残して捨てる。

- 作者を、ローカルのアカウントが承認済みでフォローしている
- ローカルのアカウントをメンションしている、または宛先（`to` / `cc`）に含めている
- ローカルの投稿への返信、またはローカルの投稿を含むスレッドへの返信
- ローカルのアカウントがブーストした（`Announce` を受けた）・お気に入りした・ブックマークした・ピン留めした、または URL の検索で解決した（FR-305）
- `receive` が `true` の `accepted` のリレーから届いた `public` の投稿（上限と処分の判定を通ったもの。§20）

`Announce` の対象は、ブーストした人がローカルのアカウントにフォローされているとき、または主体が `accepted` のリレーであるときだけ取得する。

取得した投稿として、次も取り込む。ほかの条件（作者の検証・tombstone・同じオリジン）は、federation.md §6.3 の「取得した投稿」と同じ `StatusHandlers.fetched` を通す。

- ローカルの利用者がプロフィールを開いたリモートのアカウントの、outbox の最初のページにある公開（`public` / `unlisted`）の投稿（§14・§15）
- 取得したリモートのアカウントの、`featured`（ピン留め）の最初のページにある公開の投稿（5 件まで。§17）

### 12. 取得とリフレッシュ

- Actor: 取得から 24 時間で古いとみなし、次に使われたとき（署名の検証・表示・配送）に非同期で取り直す。ローカルのアカウントがフォローしている Actor は、定期ジョブで 24 時間ごとに取り直す。取得が 410 なら `remote_deleted` にする。
- 投稿: 受信した `Update` / `Delete` で更新する。定期的な取り直しはしない（投票の集計だけは、締め切り前に表示されたときに取り直す）。
- スレッド: 返信先（`inReplyTo`）は受信時に 1 段だけ非同期で取得する。それより上の祖先と返信の一覧は、利用者がスレッドを開いたときに、深さ 20・件数 100 を上限に非同期で取得する。
- 取得の失敗（404 / 410 / タイムアウト）は KV に短期（1 時間）記録し、同じ URI の取得を繰り返さない。

### 13. 保持と消去

- リモートの投稿は、公開から 90 日（設定 `remote_status_retention_days`）を過ぎ、かつローカルからの参照（返信・ブースト・お気に入り・ブックマーク・ピン留め・通知・ローカルの投稿を含むスレッド）がなければ、消去ジョブが消す（FR-803）。
  消すときは `status_deletions`（`origin = retention`）を記録し、URI を tombstone として残す。後から同じ投稿を取得し直すことは妨げない（tombstone は `remote` / `author` の削除だけが復活を拒む）。
- 運用者は設定 `remote_status_max_retention_days` で、リモートの投稿を保持する期間の上限を決められる。上限を設定すると、公開からその期間を過ぎたリモートの投稿は、ローカルからの参照があっても消去ジョブが消す（`origin = retention`）。既定は上限なし。ローカルの利用者の操作（返信・ブースト・お気に入り・ブックマーク・ピン留め）の行は残し、表示では対象の投稿を「削除済み」として扱う。
- リレー経由でだけ取り込んだ投稿は、`content` の `relayed_statuses` に記録する。
  消去ジョブ `content.purge_remote_statuses` は、`relayed_statuses` にあり、ローカルからの参照が無く、作者をローカルのアカウントがフォローしていない投稿を、公開から **7 日**（設定 `relay_status_retention_days`）で消す（`origin = retention`）。それ以外は 90 日のまま。
- リレーを脱退しても、取り込んだ投稿はすぐには消さない（保持期間で消える）。
- プロフィールを開いて取り込んだ投稿は、ほかの取得した投稿と同じ保持期間に従う。
- 受信の原本（`received_activity_documents`）は 7 日、受信の記録（`received_activities`）は 30 日で消す。
- ローカルのアカウントとの関係がなくなった（誰もフォローしていない、投稿も参照されていない）リモートのアカウントは、プロフィールの版を最新の 1 つに縮める。アカウントの行（URI）は残す。

### 14. プロフィールを開いたときの取得: 始める条件

`GET /api/v1/accounts/{id}/statuses` の最初のページ（カーソルなし、`pinned` なし）が、次のすべてを満たすときだけ、取得のジョブを投入する。応答は待たず、保存済みの投稿だけを返す（スレッドの取得と同じ）。

- 閲覧者がログインしている（未ログインの閲覧は取得の増幅に使わせない）
- アカウントがリモートで有効（`remote_deleted` ではない）、`outbox_url` を持ち、アカウント単位の `federation` の制限（`suspend`）が無い
- 閲覧者とアカウントの間にブロックが無い（プロフィールを通常どおり表示できる）
- そのアカウントについて、直近 24 時間に取得を始めていない（KV の名前空間 `federation.outbox_fetch`、キーは Actor の URI の SHA-256、`IncrBy` で最初の 1 回だけ投入する）
- 閲覧者が直近 5 分に取得を始めさせたアカウントが 30 件未満（KV の名前空間 `federation.outbox_fetch_user`）。超えたら取得せず、ページは返す

ドメイン単位の `federation` の制限・`instance_health` が `unavailable` のドメイン・自サーバーの URI は、`Fetcher` が取得しない（federation.md §6.1）。
ドメインの `public_discovery` の制限（`limit`）は取得を止めない。利用者が自分で開いたプロフィールだからで、取り込んだ投稿は連合タイムライン・検索に出ない（federation.md §11 の表のとおり）。

新しいテーブルは作らない（冷却期間と回数は KV の短期の値）。件数（20 件）・冷却期間（24 時間）・利用者ごとの回数（5 分に 30 アカウント）は既定値で、M3.5 以降の計測で見直す。

### 15. プロフィールを開いたときの取得: 上限

ジョブ `federation.fetch_outbox`（冪等キー `fetch_outbox:<account_id>:<投入した時刻の 24 時間の区切り>`）が、1 回につき次だけを行う。

- outbox のコレクションと、その最初のページ（`first`。埋め込みならそのまま）を取得する。どちらも Actor と同じオリジンのものだけ
- ページの項目のうち、`Create` で、オブジェクトの `attributedTo` がそのアカウントで、宛先に `as:Public` を含むものを、新しい順に最大 20 件取り込む。
  オブジェクトが埋め込みならその文書を、IRI なら取得した文書を使う（取得は 20 件に含む）。保存済みの URI は取得しない
- `Announce`（ブースト）は取り込まない（第三者のサーバーへの取得を増やすため）
- 返信先は取得しない（受信時の 1 段の取得をしない）。利用者が投稿を開けば、スレッドの取得（federation.md §6.3）が補う
- 2 ページ目以降はたどらない。プロフィールで古い投稿を読み進めても、取得は増えない

1 回の取得の文書は合わせて 22 件まで（コレクション・ページ・投稿 20 件）。取得の失敗は `Fetcher` の 1 時間の記録（federation.md §6.1）に乗り、ジョブは完了にする。ストアの失敗は `Retry` にする。

### 16. プロフィールを開いたときの表示

- リモートのアカウントのプロフィールには、投稿の一覧の上に「このサーバーが受け取った投稿だけを表示しています」という案内と、
  元のサーバーのプロフィール（`Account.url`）へのリンク「{domain} で見る」を常に出す。リンクは外部リンクとして示す。
- 取得を始めたことを応答で示し（形は [client-api.md](../architecture/client-api.md) に従ってタスクで決める）、Web クライアントは「元のサーバーから最近の投稿を取得しています」を
  `role="status"` で示して、数秒後に最初のページを 1 回だけ読み直す。読み直した後は表示を消す。取得の失敗は利用者に伝えない（案内とリンクが残る）。
- 投稿が 1 件も無いリモートのプロフィールの空の表示も、同じリンクを出す。

### 17. リモートのピン留め（`featured`）

リモートのアカウントのピン留めは、Actor の取得・更新時と、Actor から `featured` への `Add` / `Remove` を受け取ったときに `featured` を取得して記録し、プロフィールの先頭（`GET /api/v1/accounts/{id}/statuses?pinned=true`）に出す。
主要実装（Mastodon・Misskey・GoToSocial）と同じく、Actor と一緒に同期する。プロフィールを開いたことは取得のきっかけにしない。

- **始める条件**: リモートの Actor の文書を保存するとき（初めての取得、`federation.refresh_actor` などによる取り直し、Actor 自身の `Update`）に、文書が Actor と同じオリジンの `featured` を宣言していれば、
  保存と同じトランザクションでジョブ `federation.fetch_featured`（冪等キー `fetch_featured:<account_id>`）を投入する。取得の回数は Actor の取り直し（24 時間に 1 回）と、下の `Add` / `Remove` の受信に比例する。
- **取得**: ジョブは、実行時にもアカウントが有効なリモートのアカウントで `federation` の制限が無いことを確かめ、`featured` のコレクションを取得して、項目があればそれを、無ければ `first` のページ（Actor と同じオリジンのものだけ）を読む。2 ページ目はたどらない。
  項目の順（新しいピン留めが先）に、そのアカウントの投稿を 5 件まで選ぶ。埋め込みの投稿は `attributedTo` がそのアカウントで宛先に `as:Public` を含むものだけ、IRI だけの項目はアカウントと同じオリジンのものだけを選び、
  保存済みでなければ outbox と同じく取得した投稿（`StatusHandlers.fetched`）として取り込む（返信先は取得しない）。1 回の取得の文書は、コレクション・ページ・投稿 5 件の合わせて 7 件まで。
- **記録**: 選んだ投稿で、そのアカウントのピン留めを置き換える。ピン留めはローカルのアカウントと同じ `status_pins`（ADR-0020 の現在値）に置く。
  「作者が自分の投稿をプロフィールに固定している」という同じ事実で、`pinned=true` の応答、保持期間の判定（ピン留めされた投稿は消去しない）、投稿・アカウントの削除の後始末をそのまま共有できる。
  リモートのアカウントの行は `featured` の写しで、`content.Interactions.ReplaceRemotePins` だけが書く（ローカルのピン留めのコマンドは書かない）。件数の上限はローカルと同じ 5 件（`content.MaxPins`）で、行の `id` はコレクションの順を保つように振る。
- 取得できないコレクションはピン留めをそのまま残し、ジョブを完了にする。空のコレクションと、`featured` を宣言しなくなった文書は、ピン留めを消す。ストアの失敗は `Retry` にする。
- **`Add` / `Remove` の受信**: Actor が自分の `featured` への `Add` / `Remove`（Mastodon・Misskey などがピン留めの変化で送る）を送ってきたら、同じジョブ `federation.fetch_featured` を同じ冪等キーで投入してコレクションを取得し直す。
  `object` の投稿だけを足し引きせず取得し直すのは、上の取得と同じ検証・上限・置き換えに乗せ、届かなかった変化も追いつかせるため。
  `target` は、保存した Actor の文書が宣言する `featured`（Actor と同じオリジン）と一致しなければならず、送り主が Actor であることは inbox の署名の検証（federation.md §3.2）で確かめる。それ以外の `Add` / `Remove` は捨てる。
  冪等キーにより、そのアカウントのジョブが完了するまで次のジョブは投入されないので、続けて届いた通知で取得は重ならない。

### 18. リレーへの参加と脱退（FR-411, FR-724）

運営者が管理 API・CLI・管理画面からリレーを登録して参加・脱退でき、`public` の投稿だけを送り、受け取った投稿は取り直して検証し、短い保持期間で消す。

- 運営者はリレーを **方式（`mastodon` / `litepub`）と URL** で登録する。`mastodon` は inbox の URL、`litepub` は Actor の URI。URL は `https` だけで、`HTTPClient`（SSRF ガード）を通す。
  `litepub` は登録時に Actor を取得して inbox（shared inbox があればそれ）を得る。管理画面は、URL を取得して Actor の文書が返れば `litepub`、そうでなければ `mastodon` を既定に選ぶ。
- 参加は **インスタンス Actor**（`Application`）が送る。`mastodon` は `Follow`（`object` は `as:Public`）、`litepub` は `Follow`（`object` はリレーの Actor）。`Follow` の `id` は `/ap/activities/{id}` で、状態は `pending` になる。
  インスタンス Actor は `Application` なので、Activity-Relay はこれを LitePub 方式のフォロワーとして受け付ける。
- `Accept` / `Reject` は、`object`（の `id`）がそのリレーに送った `Follow` の `id` と一致するものだけを受け付け、`accepted` / `rejected` にする。`accepted` で、`Accept` の主体をリレーの Actor として記録する（以後の受信で、署名者がリレーかを判定する）。
- 脱退は `Undo(Follow)` を送り、状態を `left` にする。`rejected` と `left` のリレーは、同じ登録から参加し直せる（新しい `Follow` の `id`）。登録の削除は `left` か `rejected` のときだけで、削除してもすでに送った投稿の `Update` / `Delete` は届け続ける（宛先は元の `Create` の `deliveries`）。
- インスタンス Actor への `Follow` は、`accepted` のリレーの Actor からのものだけを自動で `Accept` する（LitePub 方式のリレーが返すフォロー）。記録はせず、それ以外の `Follow` は `Reject` する。
- 登録ごとに **受信するか**（`receive`: `true` / `false`）を選ぶ。`false` は送るだけの参加で、届いた中継を取得せずに捨てる。変えるには脱退して登録し直す。
- ドメイン単位の `suspend` のあるホストのリレーは登録できない。参加中のリレーのホストを `suspend` すると、受信は署名の検証より前に `403`、配送は `skipped` になる（federation.md §11 のまま）。状態は変えず、管理画面に警告を出す。

### 19. リレーへの送信

- 送るのは **ローカルの `public` の投稿の `Create` / `Update` / `Delete` だけ**。`unlisted`・`private`・`direct` と、**`local` は送らない**（ADR-0017）。ブーストの `Announce` も送らない（Activity-Relay の Mastodon 方式の経路は `Announce` を扱わない）。
  判定は配送の展開（federation.md §7.2）の公開範囲の規則に足し、visibility の単一のポリシー関数を通す。
- 利用者ごとに「自分の投稿をリレーに送らない」設定は持たない（Mastodon・Misskey と同じ）。リレーに出したくない投稿は、`unlisted` か `local` で投稿する。
- 展開ジョブの `Create` のフェーズに「`accepted` のリレーの inbox」を最後に足す。`deliveries` に記録するので、`Update` / `Delete` は元の `Create` の `deliveries` に届く。
  サイレンスなどで公開範囲が `public` でなくなった投稿は、ほかの宛先と同じく対象にならない。
- **`public` の投稿の `Create` / `Update` / `Delete` の文書に LD 署名（`RsaSignature2017`、作者の鍵）を付ける**。リレーに送る文書だけでなく、同じ文書をすべての宛先に送る（文書は展開ジョブが 1 回だけ組み立てる）。
  正規化（URDNA2015）は `github.com/piprate/json-gold`（Apache-2.0、cgo 不要、v0.9.1 は 2026-10-02）で行い、`@context` は同梱した文書（ActivityStreams・`w3id.org/security/v1`・`w3id.org/identity/v1`）だけで解決する（リモートの文脈を取得しない）。
  LD 署名の生成は送信のこの用途に限る。
- 配送の量は「リレーの数 × 公開投稿の数」だけ増える（リレーへの配送は shared inbox と同じく 1 件）。

### 20. リレーからの受信

- 同期の部分（federation.md §3.1）は変えない。リレーの IP とドメインにも、ほかのサーバーと同じレート制限（1,000 / 5 分、2,000 / 5 分）を掛ける。超えた分は `429` で、リレーの再送に任せる。
- 署名者の Actor が `accepted` のリレーの Actor（`mastodon` は Actor の inbox か shared inbox が登録の inbox の URL と一致するもの）なら、ジョブに `relay_id` を付ける。中継は `trust = refetch` のまま扱う（§8）。
- 非同期の部分では、`relay_id` の付いたアクティビティを次の順に扱う。
  1. 登録の `receive` が `false`、またはリレーが `accepted` でなければ捨てる。
  2. リレーごとの 1 時間の取り込みの上限（設定 `relay_max_activities_per_hour`、既定 1,000 件。KV で数える）を超えたら、取得せずに捨てる。捨てた件数はメトリクスに出す。
  3. 作者（`Create` の `actor`、`Announce` の `object` の作者のホスト）のアカウントかドメインに `federation` か `public_discovery` の制限があれば、取得せずに捨てる（連合タイムラインにも検索にも出ないので取り込まない）。
  4. `Create` / `Update` / `Delete` は §8 のとおりオリジンから取り直す。リレーの `Announce` は、`object` をオリジンから取得して取り込み、**ブーストとしては記録しない**（Mastodon と同じ）。
  5. 取り込むのは取得した投稿が `public` のときだけ。
- 取り込んだ投稿は、ほかのリモートの `public` の投稿と同じく **連合タイムラインとハッシュタグのタイムライン** に出る。ホームには出ず、通知も作らない。連合タイムラインが無効（`federated_timeline_visibility = off`）でも受信は止めない（ハッシュタグには出る）。

### 21. リレーのデータモデル（ADR-0020）

| テーブル | 所有 | 分類 | カラム |
| --- | --- | --- | --- |
| `relays` | `federation` | リソース | `id`, `protocol`（`enum(mastodon, litepub)`）, `url`（登録した URL。一意）, `inbox_url`, `receive`（`bool`）, `created_by_user_id`（NULL 可 = CLI）, `created_at` |
| `relay_events` | `federation` | イベント | `id`, `relay_id`, `kind`（`enum(follow_sent, accepted, rejected, left)`）, `follow_activity_uri`（`follow_sent` の `Follow` の `id`。ほかは対象の `Follow`）, `relay_actor_uri`（`accepted` の主体。ほかは NULL）, `actor_user_id`（NULL 可 = CLI か受信）, `occurred_at` |
| `relay_states` | `federation` | 投影 | `relay_id`（PK）, `state`（`enum(pending, accepted, rejected, left)`）, `follow_activity_uri`, `relay_actor_uri`（NULL 可）, `since` |
| `relayed_statuses` | `content` | イベント | `status_id`（PK）, `relay_id`, `received_at` |

- 参加と脱退の `Follow` / `Undo` は、`relay_events` を書くトランザクションで outbox に積む（`outbound_activities` の `object_type` に `relay` を足す）。
- 登録の削除は `relays` と子の行を消す（`relayed_statuses.relay_id` は参照を持たない ID として残す）。

### 22. リレーの管理 API・CLI・画面・監査ログ

- 管理 API: `GET /api/v1/admin/relays`（状態つきの一覧）、`POST /api/v1/admin/relays`（登録して参加。`protocol`・`url`・`receive`）、`POST /api/v1/admin/relays/{id}/follow`（参加し直す）、`POST /api/v1/admin/relays/{id}/unfollow`（脱退）、`DELETE /api/v1/admin/relays/{id}`（`left` / `rejected` の登録の削除）。
- 権限は `manage_settings`。リレー用の権限は分けない。
- CLI: `my admin relay create|follow|unfollow|remove|list`。
- 管理画面: リレーの一覧（方式・URL・状態・受信の有無・直近 24 時間の取り込み件数と上限で捨てた件数）と、登録・参加・脱退・削除の操作。
- 監査ログ（ADR-0058）: 登録・参加・脱退・削除を `target_kind = relay` で記録する。`Accept` / `Reject` の受信は `actor_kind = remote` で記録する。

### 23. 連合の向きの設定（FR-413）

- インスタンス設定 `federation_mode`（`KindEnum`、`off` / `inbound` / `outbound` / `both`、既定 `both`）。タイムラインの公開範囲（`local_timeline_visibility` など、#975）と同じく、状態を 1 つの enum で表す。`instance_setting_revisions` に記録し（変更者と時刻が残る）、`manage_settings` で管理 API・CLI・管理画面から変える。リクエストとジョブごとに読む（`authorized_fetch` と同じく、変更はすぐに効く）。
- 管理画面は、モードを狭める変更の前に、§27 の影響（届かなくなるもの、承認待ちのまま残るもの）を示して確認を求める。

### 24. 流れの分類

向きは HTTP の向きではなく、内容の流れで区切る。判定は、`internal/domain/federation` の 1 つの関数 `federation.Allows(mode, Flow) bool` に集める。各入口は自分の扱う `Flow` を渡して聞くだけにし、モードの値で分岐しない。

| `Flow` | 内容 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- | --- |
| `publish` | ローカルの投稿・プロフィール・反応を外へ出す（配送と GET） | × | × | ○ | ○ |
| `ingest` | 外の投稿・反応を取り込む（受信と取得） | × | ○ | × | ○ |
| `follow_out` | ローカルの利用者が外のアカウントをフォローする往復（`Follow` / `Undo Follow` の送信、`Accept` / `Reject` の受信、`Move` の受信とフォローの移し替え） | × | ○ | × | ○ |
| `follow_in` | 外のアカウントがローカルの利用者をフォローする往復（`Follow` / `Undo Follow` の受信、`Accept` / `Reject` の送信） | × | 断る | ○ | ○ |
| `retract` | すでに外へ出したものの撤回と行き先の通知（投稿の `Delete`、Actor の `Delete`、`Undo Like` / `Undo Announce` / `Undo EmojiReact`、凍結の `Update(Actor)`、`Move` の送信と `movedTo` 付きの `Update(Actor)`） | × | ○ | ○ | ○ |
| `protect` | 保護のための手続き（`Block` / `Undo Block` の送受信、`Flag` の送受信、外の Actor の自分自身の `Update` / `Delete` の受信） | × | ○ | ○ | ○ |
| `identify` | 署名の検証のための鍵の取得と提供（外の Actor の取得、ローカルの Actor・インスタンス Actor の GET、WebFinger・host-meta・NodeInfo） | × | ○ | ○ | ○ |

- `inbound` の `follow_in` の「断る」は、受けた `Follow` に `Reject` を返す（承認待ちのまま残さない）。
- `retract` は、`inbound` に狭めたあとも、それまでに外へ出したものを消せるようにするために通す。宛先は元の配送（`deliveries`）とその時点のフォロワーで、新しい宛先は増やさない。
- `off` は何も通さない。オフの間の削除は外に伝わらない。例外はサービスのクローズで、その判定はモードより優先し、`off` / `inbound` でも `Delete` を送る（ADR-0088 §2）。
- 別名の `Update(Actor)` は `publish` に入る（ADR-0089）。

### 25. 入口ごとの振る舞い

| 入口 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- |
| inbox（同期、federation.md §3.1） | 署名の検証より前に `403`（ドメインの `suspend` と同じ位置） | 受け付ける | 受け付ける | いまのまま |
| inbox（非同期、federation.md §3.2 のハンドラ） | — | `ingest`・`follow_out`・`follow_in`（断る）・`protect` の型だけ処理し、ほかは `dropped` | `follow_in`・`protect` の型だけ処理し、ほかは `dropped`（`QuoteRequest` には `Reject` を返す） | いまのまま |
| 展開と配送（federation.md §7.2・§7.3） | すべて `skipped` | `follow_out`・`follow_in` の `Reject`・`retract`・`protect` だけ | `publish`・`follow_in`・`retract`・`protect` だけ | いまのまま |
| 外の取得（federation.md §6） | しない | いまのまま | 署名の検証と配送の宛先に要る Actor の取得だけ。投稿・スレッド・outbox の取得はしない。メディアプロキシはフォロワーの Actor の画像だけ | いまのまま |
| ローカルの Actor の GET（federation.md §10） | `404`（`410` にしない） | 鍵の取得のための最小の文書（Authorized Fetch の署名なしと同じ項目） | いまのまま | いまのまま |
| 投稿・`/activity`・コレクション（outbox・followers・following・featured）の GET | `404` | `404`（outbox など空にせず存在を見せない） | いまのまま | いまのまま |
| インスタンス Actor の GET | `404` | いまのまま | いまのまま | いまのまま |
| WebFinger・host-meta | `404` | 答える（`follow_out` で相手がハンドルを確かめるため。Mastodon・GoToSocial は Actor を取得したあと WebFinger を引く） | いまのまま | いまのまま |
| NodeInfo | `404` | 答える。`metadata.federationMode` に値を出す | 同左 | 同左 |
| リレー（§18〜§20） | 参加・送信・受信をしない（登録と状態は残す） | 受信だけ（送信のフェーズを飛ばす） | 送信だけ（受信を捨てる） | いまのまま |

- `inbound` で外の Actor の文書に `Create` などの内容が埋め込まれていても、取り込みの範囲は §11 のまま（ローカルの利用者がフォローしている相手など）。モードは範囲を広げない。
- `inbound` で `Like` / `Announce` / `EmojiReact` がローカルの投稿に届いても、外から見えないはずのものなので捨てる。ローカルの利用者へのメンション（フォロー中の相手から）は `ingest` として取り込む。
- `outbound` で捨てるのは、ローカルの投稿への返信・メンション・ブースト・お気に入り・絵文字リアクション・投票への回答・引用の依頼を含む。外の反応は数にも通知にも出ない。
- ドメインの処分（federation.md §11）はモードと独立に効く。モードが通しても、処分が止めるものは止まる。

### 26. 連合の向きと client API・Web

- `GET /instance` に `federation.mode` を足し、Web はそれで外向きの操作を出し分ける。
- `ingest` か `follow_out` が通らないモード（`off` / `outbound`）では、外のアカウントのフォロー・URL とハンドルの解決（FR-305）を client API が `403`（専用の problem type `federation-restricted`）で断る。検索はローカルの結果だけを返す。Web は外のアカウントを探す・フォローする操作を出さない。
- `publish` が通らないモード（`off` / `inbound`）では、ローカルの利用者は外の投稿にお気に入り・ブースト・リアクション・返信ができる（ローカルに記録される）が、外には届かない。Web は外の投稿への操作と、外のアカウントへのメンション・返信の投稿画面に「連合先には届きません」と示す。引用の依頼は送れないので、外の投稿の引用は出さない。
- 文言は `locales/en`・`locales/ja` に足す（ADR-0009）。

### 27. 連合の向きの切り替え

- モードを狭めても、**フォロー関係・取り込んだ投稿・配送の記録は消さず、`Undo` / `Reject` も送らない**。流れが止まるだけで、広げ直すとそのまま再開する。
  - 狭めている間に作られた投稿・反応は、広げ直しても遡って配送しない（展開は記録の時点のモードで決まり、`skipped` は再送しない）。
  - 外の側では、`inbound` の間はローカルの利用者のフォロワーが何も受け取らず、`outbound` / `off` の間はローカルの利用者のフォローが更新されない状態になる。
- 関係を切りたい運営者のために、CLI `my admin federation sever --direction in|out|both` を置く（ADR-0058 の `follow_terminations`、`reason = moderation` と同じ記録で、送れるモードでは `Reject` / `Undo` を送る）。モードの変更が自動で呼ぶことはしない。
- 展開と配送のジョブは、実行の時点のモードで判定する。切り替えの前に積んだジョブが、狭めた後に送られることはない。
- モードの変更は `instance_setting_revisions` に記録し、監査ログ（ADR-0058）にも `target_kind = setting` で残す（既存の設定の変更と同じ）。

### 28. 連合の向きの相互運用の確認

federation-live（2026-10-22、[#983](https://github.com/mycroblog/my/issues/983#issuecomment-6337103257)）で、Mastodon 4.7・GoToSocial 0.22・Misskey 2026.10 を相手に次を確かめた。

- `inbound` の最小の Actor の文書（名前・アイコン・コレクションなし）からの `Follow` を、3 つとも受け付けて `Accept` を返す。
- Mastodon と GoToSocial は、こちらの Actor を取得したあと `acct:` の WebFinger を引く。Misskey は引かない。`inbound` でも WebFinger に答える。
- `inbound` で外から受けた `Follow` は `Reject` になり、相手の側にもこちらの側にも承認待ちが残らない。
- `off` の Actor・WebFinger・NodeInfo は `404` を返す。Mastodon と Misskey は Actor を取り直して `404` を受けても、アカウントとフォローを消さない。`both` に戻すと投稿がまた届く。
- NodeInfo 2.1 のスキーマは `protocols` を `minItems: 1` とするので、空の配列は返せない。`off` は NodeInfo を `404` にする。

### 29. この決定を見直す条件

- 署名（§4・§5）
  - 主要実装のいずれかが draft-cavage を受け付けなくなった場合（RFC 9421 を先に試す案が有力になる）
  - 送信のリクエスト数の計測で、double-knocking の再送が配送全体の数 % を超える場合（記憶の TTL か順序を見直す）
- Inbox（§6〜§10）
  - 同期の Actor 取得が、Lambda / Cloud Run の計測で p95 の応答時間やコストを許容できないほど押し上げる場合（すぐキューに入れる案が有力になる）
  - 受信時の重複排除で、処理に失敗したアクティビティの再送を受け付けないことが運用上の問題になる場合（処理後に重複排除する案が有力になる）
- 取り込み（§11〜§13）
  - 連合タイムライン（FR-202）に、フォロー外の公開投稿も載せるべきだと決めた場合（すべて取り込む案に近づく）
  - 計測で、関係の判定（受信ごとの問い合わせ）の費用が保存の費用を上回る場合
- プロフィールの取得（§14〜§17）
  - 計測で、プロフィールを開いたことによる取得が外部 HTTP やジョブの大きな割合を占める場合（件数・冷却期間を見直すか、取得しない案に戻す）
  - 連合タイムラインの扱い（取得した投稿を載せるか）を別に決めた場合
- リレー（§18〜§22）
  - 計測で、中継の取り直し（1 件につき投稿と、未知の作者の Actor の取得）の費用が上限の範囲でも許容できない場合: LD 署名の検証で取り直しを省く案を決める。
  - LD 署名の生成が Mastodon・Misskey との相互運用テスト（federation-live）で通らない、または json-gold の正規化が相互運用上の問題を起こす場合: FEP-8b32 の証明か、署名を足さない案（Announce の経路のリレーに限る）を見直す。
  - 主要なリレーのソフトウェアが LitePub 方式の `Follow` を受け付けない場合: LitePub 方式の登録を外す。
- 連合の向き（§23〜§28）
  - 主要実装（Mastodon・Misskey・GoToSocial・Akkoma）のどれかが、`inbound` の最小の Actor の文書でこちらの `Follow` を受け付けない（プロフィールの項目を必須にする）ことが相互運用テストで分かった場合: `inbound` でも Actor の文書全体を出す（プロフィールは外へ出る）か、`inbound` を外す。
  - `outbound` の利用者が、外からの返信・反応を通知で受けたいと求めた場合: `outbound` に「反応だけ取り込む」段を足す（2 つの bool の形のほうが表しやすくなる）。
  - 許可したドメインとだけ連合したい要望が出た場合: 許可リストを、この enum と直交するものとして別の ADR で決める。

## 経緯

「ActivityPub 互換」の実態は仕様書よりも Mastodon をはじめとする主要実装との相互運用性で決まる（G4）。互換性を担うライブラリが無いので、対応範囲と、自前で持つ部分を最初に決めた（FR-404, FR-407）。

署名の送信の順序は、当初「RFC 9421 を先に試す（Mastodon 4.7 と同じ）」としたが、ADR-0019 §7 の記述と食い違っていた。[#232 の調査](https://github.com/mycroblog/my/issues/232)（2026-10-17、Mastodon 4.8.0-dev のソース）で、Mastodon の送信は draft-cavage が先で、GoToSocial と Misskey は draft-cavage だけだと分かり、追加の往復が要らない今の順序にした。

受信（§6〜§10）は同じ調査で主要実装の境界を比べた。偽のリクエストでワーカーとストレージを消費させないこと、at-least-once のジョブと順序の保証されない配送で結果が同じになること、SQS 標準キューや Cloud Tasks のようにキーで直列化できないアダプタでも振る舞いを揃えることから、署名検証まで同期、受信時に重複排除、順序に依存しないハンドラとした。

取り込み（§11〜§13）は、DB の容量と外部 HTTP がそのまま費用になること（G1、NFR-COST-1, NFR-COST-5）から、関係のあるものだけにした。Mastodon も関係の無い `Create` を捨てる。その後、次の 2 つが範囲を広げた。

- プロフィール（FR-204）: フォローしていない相手のプロフィールが空に近く、フォローの判断材料にならなかった。主要実装はどれも outbox を取り込まず、案内とリンクだけを出す（[#486 の調査](https://github.com/mycroblog/my/issues/486#issuecomment-6261357300)）。決定者のフィードバックを受け、上限付きの取り込みに案内とリンクを合わせた。
- リレー（FR-202, FR-405, FR-406, FR-702, FR-704, FR-803）: 当初は v0 の非対応だったが、運営者が選んで参加できるようにすると決まり、関係のない投稿をリレー経由に限って短期間取り込むようにした。ソースの確認（2026-10-20）で、Activity-Relay は原本のまま中継し、Mastodon と Misskey の購読者は署名者と `actor` が違うものを LD 署名（Mastodon は FEP-8b32 の証明でも可）が無ければ捨てると分かった。これが送信の LD 署名の理由で、対応表の「LD 署名の生成は非対応」もこのとき変わった。受信は §8 の取り直しのままで済む。

連合の向き（§23〜§28、FR-401〜FR-411、FR-709）は、#976 で運営者が閉じたサーバー・読むだけ・発信だけを選べるよう求められた。受信のみでもフォローには `Follow` の送信と鍵の提供が要り、送信のみでも `Follow` の受信と `Accept` が要るので、HTTP の向きでは区切れず、内容の流れで区切った。狭めて広げ直してもつながりが戻ること、相手から見て戻せない誤解（`410` で消えたと扱われる、承認待ちのまま残る）を生まないことを条件にした。`Move` は ADR-0089、クローズは ADR-0088 でこの分類に加わった。

## 採らなかった案

- 署名: RFC 9421 を先に試す: 新しい仕様を既定にできるが、RFC 9421 を受けない相手（GoToSocial、Misskey、4.4 以前の Mastodon）に記憶が切れるたびに 2 回送る。
- 署名: draft-cavage だけ: 最も単純だが、RFC 9421 だけを受ける実装に届かず、FR-404 を満たさない。
- Inbox: すぐキューに入れ、署名検証もワーカーで行う（Misskey）: リクエストは最短だが、偽のリクエストでジョブが増え、署名が不正でも `202` で失敗が伝わらない。
- Inbox: 処理後に重複排除し `orderingKey` で順序を保証: 失敗を相手の再送でやり直せ、ハンドラも単純になるが、SQS 標準キュー・Cloud Tasks で保証できず、同時の再送が両方ジョブになる。
- 取り込み: すべて取り込み期間で消す: 単純で連合タイムラインが充実するが、量が連合先の規模に比例する。
- 取り込み: 保存せず表示のたびに取得: ストレージは最小だが、ホームのタイムラインを作れず（ADR-0013）、外部 HTTP が閲覧回数に比例する。
- プロフィール: 取得しない: 主要実装と同じで追加の負荷が無いが、知らない相手のプロフィールがほぼ空のまま。
- プロフィール: 保存せずに代理で表示: 行は増えないが、表示のたびに外部 HTTP が要り、ブースト・お気に入り・返信・`content.CanView` を通せない。
- リレーの方式: Mastodon 方式だけ: 実装は最小で主要なリレーは受け付けるが、LitePub 方式だけのリレーに参加できず、省ける実装も小さい。
- リレーの方式: LitePub 方式だけ（リレー用 Actor の `Announce`）: LD 署名が要らないが、Mastodon 方式だけのリレーに参加できず、Activity-Relay の購読者に原本で届けるには結局 LD 署名が要る。
- リレーの送信: 署名を足さない: 依存が増えないが、主な届け先（Mastodon・Misskey の購読者）に捨てられる。
- リレーの送信: FEP-8b32（`eddsa-jcs-2022`）: JSON-LD の処理が要らないが、検証するのは 2026-07 以降の Mastodon の main だけで、Actor ごとに ed25519 の鍵も要る。FR-410 として別に扱う。
- リレーの受信: LD 署名を検証できればそのまま使う: GET を省けるが、信用できない入力に JSON-LD の処理（文脈の差し替えの攻撃）を行うことになる。
- 向きの設定: 2 つの bool（`federation_inbound` / `federation_outbound`）: `ingest` と `publish` が独立の軸であることを素直に表し、軸を足しやすいが、両方 `false` がオフと読みにくく、`follow_in` の「断る」のような組み合わせの振る舞いを説明しにくい。
- 向きの設定: 許可リスト（Mastodon の `LIMITED_FEDERATION_MODE`）: 特定の相手とだけつながれるが、制御するのは向きではなく相手で、向きと直交する。
- 向きの区切り: HTTP の向き: 単純だが、受信のみでフォローを送れず、送信のみで `Follow` を受けられない。

## Consequences（影響）

- 互換性を担うライブラリが無いため、相互運用の細部は自前で持つ。フィクスチャ回帰テストとライブ相互運用テストへの投資が必須になる。代わりに、互換性を壊したことを CI で検知できる。
- 署名
  - GoToSocial（draft-cavage だけ）と Mastodon（どちらも受ける）には 1 回で届く。RFC 9421 だけを受ける相手には、最初の 1 回だけ 2 回になり、以後は 1 回になる。
  - draft-cavage は仕様として古い。主要実装が RFC 9421 に移ったら順序を見直す必要がある。
  - クエリ文字列の再検証の分だけ、draft-cavage の検証が最大 2 回になる。
- Inbox
  - 偽の署名・未知の鍵・ブロック済みドメインのリクエストは、DB にもキューにも触れずに終わる。同じアクティビティの再送は、1 回の一意制約の照合で終わる。
  - キューアダプタの順序保証の差に結果が左右されない。#77 は性能と競合の削減のためで、正しさの前提ではない。
  - 未知の Actor からの最初のリクエストは、Actor の取得の分だけ遅くなる。リクエストの中で外部 I/O をする（トランザクションの外なので ADR-0003 には反しない）。
  - 受信時に記録するので、処理に失敗したアクティビティは送信側が再送しても処理し直されない。dead job の監視と再実行が必要になる。
  - tombstone の保持期間より後に届いた古い `Create` は取り込まれうる。
- 取り込み
  - ストレージと書き込みが、ローカルの利用者の関係に比例する。巨大なインスタンスから大量の公開投稿が届いて膨らむことがない。
  - 受信ごとに関係の判定が要る（`graph` と `content` の公開 API への問い合わせ）。
  - 消去後に古い投稿へのリンクを開くと、取り直しの分だけ遅い。
- プロフィール
  - フォローしていない相手でも最近の公開投稿（最大 20 件）が見え、そのままブースト・お気に入り・返信できる。外部 HTTP はアカウントごとに 24 時間に 22 件、利用者ごとに 5 分に 30 アカウント分までに収まる。
  - 主要実装はどれも outbox を取り込まないので、相手から見るとこのサーバーだけが outbox を読みにくる。outbox は Authorized Fetch のサーバーでもインスタンス Actor の署名で読めるが、公開の投稿しか返らない。
  - 取り込んだ公開の投稿は、ほかの取得した投稿（スレッド・検索）と同じく、連合タイムラインとハッシュタグのタイムラインの、公開の時刻の位置に載る。
  - 取得は非同期なので、最初に開いたときは空か古く、数秒後の読み直しで出る。保存の行は、利用者が開いたプロフィールの数に比例して増える（90 日で消える）。
  - リモートのアカウントの取得・取り直しと、`featured` への `Add` / `Remove` の受信のたびに、`featured` の取得（文書 7 件まで）が加わる。ピン留めした投稿は保持期間で消えない。
- リレー
  - 小さなインスタンスでも連合タイムラインとハッシュタグを埋め、自分の公開投稿を外に届けられる。参加していないインスタンスの受信・保存・配送は変わらず、参加しても取り込みは 1 時間の上限と 7 日の保持で頭打ちになる。
  - 中継された投稿は取り直すので、偽の中継で投稿を作られない。処分した作者・ドメインの投稿は取得前に捨てる。
  - 中継 1 件ごとにオリジンへの GET（と未知の作者の Actor の取得）が要り、受信のジョブ・行・外部 HTTP が流量に比例して増える。既定の上限で 1 日最大 24,000 件の取得になる（要検証: 主要なリレーの実際の流量）。
  - LD 署名のために JSON-LD の正規化（json-gold）を依存に足す。正規化は CPU を使い、`public` の投稿の配送ごとに 1 回かかる。同梱する `@context` の保守が要る。
  - Mastodon は LD 署名の付いた文書を JSON-LD として compact してから処理するので、こちらの文書が JSON-LD として不正だと LD 署名が捨てられ、中継が届かない（直接の配送は届く）。
  - 送信は `Announce` を送らないので、Pleroma 系のリレーの一部（`Announce` だけを中継するもの）では、こちらの投稿が中継されない（要検証）。
  - リレーの IP にも通常のレート制限を掛けるので、大きなリレーでは `429` で取りこぼす（best effort）。
- 連合の向き
  - 閉じたサーバー・読むだけのサーバー・発信だけのサーバーを設定 1 つで作れ、いつでも `both` に戻せる。URI と関係はそのまま残る。既定の `both` は振る舞いを変えない。
  - 判定が `federation.Allows` に集まり、入口のテストは「この入口はこの `Flow` を聞く」ことだけを確かめればよい。入口の数は多く（受信・配送・取得・GET・WebFinger・NodeInfo・リレー・client API・Web）、実装とテストの量は小さくない。
  - `off` の間の投稿・アカウントの削除は外に伝わらない。`off` に切り替える前に配送済みの投稿は外に残り続ける。管理画面の確認で示す。
  - `inbound` / `outbound` / `off` の間、外の側の関係は「つながっているが何も流れない」状態になる。相手の利用者には理由が見えない。
  - `inbound` では、ローカルの利用者が外の投稿にした返信・反応がローカルにだけ残る。Web の注記に頼る。

## Open Questions / 要検証

- Akkoma の RFC 9421 の受信と送信の対応状況。
- 署名の記憶の TTL 7 日の妥当性。
- 同期の Actor 取得の所要時間（Cloud Run / Lambda のコールドスタートを含む）の計測。
- tombstone の保持期間 30 日の妥当性（Mastodon が `Delete` を先に受けたときの保持期間との比較）。
- 90 日・7 日・30 日・深さ 20・件数 100 の既定値の妥当性（M3 の計測で見直す）。
- Mastodon・GoToSocial のリモート投稿の保持（Mastodon の `vacuum` 系の設定）との比較。
- 取得した投稿を連合タイムラインから外すか（スレッド・検索で取得した投稿も同じ扱いになるので、取り込み範囲の見直しとして別に決める）。
- 主要なリレー（Activity-Relay を使う大きなリレー）の流量と、上限 1 時間 1,000 件・保持 7 日の既定値の妥当性（計測で見直す）。
- aode-relay・ActivityRelay（Python）・Pleroma 系のリレーが、インスタンス Actor（`Application`）からの LitePub 方式の `Follow` と、`Create` の原本を受け付けるか。
- Pleroma / Akkoma が、署名者と `actor` が違う中継をオリジンから取り直すか、LD 署名を検証するか。
- json-gold の URDNA2015 と `RsaSignature2017` の署名が、Misskey の検証を通るか（Mastodon は federation-live の `TestRelay` で確かめている）。
- Akkoma が `inbound` の最小の Actor の文書からの `Follow` を受け付けるか。
- GoToSocial は保存した Actor で答え、取り直さないため、`off` の `404` への振る舞いを確かめていない。
- GoToSocial は、モードを `inbound` から広げたあとも最小の Actor の文書（`followers` なし）を持ち続け、`outbound` で届けたこちらの投稿が相手の利用者のホームに出ない。配送は `202` で受け付けられる。原因は確かめていない。
- サーバーの一覧サイトなどに「ActivityPub を話さないが存在する」ことを出したい場合は、`off` でも NodeInfo を返す形（`protocols` は空にできない）を考える。
