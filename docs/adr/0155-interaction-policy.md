# ADR-0155: 交流の許可（返信・ブースト・引用を誰ができるか）と承認

- Status: Proposed
- Date: 2026-11-09
- Deciders: @rosylilly
- Issue: #2555

## Decision（決定）

**推奨案: 投稿の作者は、返信・ブースト・引用のそれぞれについて「自動で認める相手」と「承認して認める相手」を選びます。許可は 1 つの表で種類ごとに持ち、連合では GoToSocial の `interactionPolicy`（`canReply` / `canAnnounce` / `canQuote`）と、`ReplyRequest` / `AnnounceRequest` / `QuoteRequest` への `Accept` / `Reject` で伝えます。許可を読まないサーバーから届いた許可外の返信は保存しますが、スレッドに出さず、通知もしません。**

許可の判定（§3）と承認の手順（§6〜§8）は、どちらも `v0.1.0` に入れます。引用の表示と引用の方言は [ADR-0069](0069-quote-posts.md) が持ち、誰が引用できるかと引用の承認の手順はこの ADR が持ちます。

次は Decider が決めた点です: 依頼・答え・状態・承認の表を引用も含めて種類によらないものにする（§8）、承認の要るリモートの投稿への返信・ブーストは保留にして作者にだけ承認待ちとして見せ、`Accept` の後に配送する（§7）、新しい利用者の既定値（§5）、`followers` の相手にメンションした人を含める（§2）、利用者の既定値を `identity` の別の表に置く（§4）、`v0.1.0` で取り消せる承認は引用だけにする（§8）。

この推奨案が覆るのは、FEP の段階で返信・ブーストの許可の語彙が GoToSocial と別のものに決まり、Mastodon がそちらを採ったときです（§12）。

### 1. 種類

- 許可で制限するのは **返信**（`reply`）・**ブースト**（`announce`）・**引用**（`quote`）の 3 つです。
- お気に入りと絵文字リアクションは制限しません。`canLike` は送らず、受け取っても読みません。
- 種類は列の値で持ち、表を増やさずに種類を足せるようにします（§4）。

### 2. 相手

- ローカルの作者が選べる相手は、狭い順に次の 4 つです。広いものは狭いものを含みます。

  | 値 | 意味 | `interactionPolicy` の値 |
  | --- | --- | --- |
  | `everyone` | 誰でも | `as:Public` |
  | `followers` | フォロワーと、投稿でメンションした人 | 作者の followers コレクションと、メンションした Actor の IRI と、作者自身 |
  | `mentioned` | 投稿でメンションした人 | メンションした Actor の IRI と、作者自身 |
  | `nobody` | 作者だけ | 作者自身 |

- 作者自身はいつでも返信・ブースト・引用できます（自分のスレッドを続けられるように）。返信では、投稿でメンションした人と、投稿が返信している相手も、どの値でも自動で認めます（GoToSocial と同じ）。
- 種類ごとに 2 つの範囲を選びます。
  - **自動で認める相手**（`automaticApproval`）
  - **承認して認める相手**（`manualApproval`）: 自動の範囲より広いものか、無しです。ここに当てはまる人の交流は、作者が承認するまで保留です（§6）。
- リモートの投稿の許可は、受け取った IRI を次のように読みます。GoToSocial 0.20 より前の名前（`always` は `automaticApproval`、`approvalRequired` は `manualApproval`）も読みます。
  - `as:Public` は `everyone`、作者の followers コレクションは `followers`、作者の following コレクションは `following`、作者自身は `author`
  - ほかの Actor の IRI は、その Actor だけを指す `account`
  - 正体の分からないコレクションは誰にも当てはまらないものとして捨てます。
  - `following` と `account` は、リモートの許可を読むときだけの値です。ローカルの作者は選べません。

### 3. 判定

- 交流できるかは、`content` のドメインの 1 つの関数（`content.MayInteract`。今の `content.MayQuote` の許可の部分を置き換える）で決め、結果は「認める」「承認待ちにする」「断る」の 3 つです。
  投稿の作成・ブースト・`Status` の可否・リモートの依頼への答え・受信した返信とブーストの表示は、どれもこの関数を通します。
- 判定の順は次のとおりです。前の段で断れば、後の段を見ません。
  1. 閲覧の可否（`CanView`。公開範囲の判定は ADR-0017 の 1 つの関数のまま）
  2. 公開範囲から決まる制限（`private` は作者だけがブーストでき、`direct` はブースト・引用できない。引用の制限は ADR-0069 §4）
  3. ブロック関係と交流の禁止（FR-708）
  4. 許可: 自動の範囲に当てはまれば「認める」、承認の範囲に当てはまれば「承認待ちにする」、どちらでもなければ「断る」
- 許可を宣言していないリモートの投稿（`interactionPolicy` が無いか、その種類の `can*` が無いか、`null` か `{}`）の返信とブーストは、誰でも自動で認めるものとして扱います。GoToSocial の取り決めと同じです。
  `canQuote` の無い投稿の引用は、ADR-0069 §3 のとおり承認の手順なしの `legacy` です。
- 作者が投稿後に許可を変えても、すでに認めた交流は取り消しません。保留中の依頼は、変えた許可で自動で認められるようになれば認め、それ以外はそのまま作者の判断を待ちます。

### 4. 許可の表

- 投稿の交流の許可を、種類によらない 1 つの表（現在値）で `content` が持ちます。今の `status_quote_policies` はこの表に移り、無くなります。
  - 列: 投稿・種類（`reply` / `announce` / `quote`）・承認（`automatic` / `manual`）・相手（§2 の値）・相手の Actor の IRI（`account` のときだけ）
  - ローカルの投稿は、作成のときに 3 つの種類すべての行を持ちます。リモートの投稿は宣言した種類だけの行を持ち、`Create` / `Update` のたびに置き換えます。
  - その種類の行が無いことが「宣言なし」で、空の配列だけを宣言したリモートの投稿は作者自身の行だけを持ちます。これで「誰でも」「作者だけ」「宣言なし」と、自動と承認の区別が後から読めます。
- 利用者の既定値は、投稿の表と同じ形の別の表に移し、今の `user_preferences.default_quote_policy` は無くなります。既定値の表は利用者の設定なので `identity` が持ち、列は投稿の表から投稿の代わりに利用者を持ち、相手の Actor の IRI を持たない形です。
- 表と列の名前は、実装の issue（#2556）で [data-model §4.4](../architecture/data-model/04-tables/04-content.md#44-content) と §4.1 に書きます。

### 5. 既定値

- 新しい利用者の既定値は、返信とブーストは `everyone` を自動で、引用は今の既定値の `everyone`（今の `public`）を自動で認め、どれも承認の範囲は無しです。
- 投稿の作成で種類ごとの許可を指定でき、指定しなければ既定値を写します。

### 6. 受信: リモートの人の、ローカルの投稿への交流

GoToSocial の取り決めでは、交流する側は、承認の範囲に当てはまるときと、フォロワー・フォロー中であることで自動の範囲に当てはまるとき（第三のサーバーがコレクションを確かめられないため）に、交流を保留にして依頼を送り、`Accept` を受けてから交流を配送します。依頼を送らずに交流を配送してくる実装（Mastodon など）もあるので、両方を受けます。

- **`ReplyRequest` / `AnnounceRequest`**（`object` はローカルの投稿、`instrument` は返信の `Note` か `Announce`。依頼は主体の inbox だけに届く）:
  - §3 で判定し、認めるなら承認（§8）を記録して、`object` に依頼、`result` に `ReplyAuthorization` / `AnnounceAuthorization` を付けた `Accept` を主体の inbox へ送ります。
  - 承認待ちなら依頼として記録し、作者に承認の依頼を通知します。作者が認めたら上の `Accept` を、断ったら `Reject` を送ります。
  - 断るなら `Reject` を送ります。理由は付けません。
  - 依頼の段階では交流を記録しません。認めた後に届く `Create` / `Announce` が `replyAuthorization` / `announceAuthorization` で自分の承認を指していれば、判定せずに記録します。
- **依頼の無い返信**（`inReplyTo` がローカルの投稿の `Create`）:
  - 自動の範囲なら、今と同じく記録してスレッドに出します。`as:Public` 以外の理由で認めたときは、`object` に返信、`target` に親、`result` に承認を付けた `Accept` を、返信の作者・`as:Public`・作者のフォロワーへ送り、第三のサーバーが確かめられるようにします（GoToSocial と同じ）。
  - 承認の範囲なら、承認待ちの依頼として記録し、作者に通知します。作者が認めたら上の `Accept` を送ってスレッドに出し、断ったら `Reject` を送ります。
  - どちらでもなければ `Reject` を送ります。
  - 認めていない返信（承認待ち・断った返信）も、届いた投稿としては保存します。ただしスレッド（親の返信の一覧・会話の表示・`replies` コレクション）に出さず、返信とメンションの通知も作りません。返信の作者のプロフィールと、その投稿の個別の表示には出ます。
- **依頼の無いブースト**（ローカルの投稿の `Announce`）: 自動の範囲なら今と同じく記録します。承認の範囲なら依頼として記録して作者の判断を待ち、認めるまで `boosts` に記録しません。どちらでもなければ記録せず `Reject` を送ります。記録しないブーストは、件数・タイムライン・通知に出ません。
- **第三者の交流**（リモートの投稿へのリモートの人の返信・ブースト）: 対象の投稿の自動の範囲に `as:Public` が無いのに、交流が承認のプロパティを持たないか、承認が §7 の検証を通らなければ、返信はスレッドに出さず、ブーストは記録しません。
- **引用**（`QuoteRequest`）: ADR-0069 §2 の自動の答えに、承認待ちを足します。承認の範囲に当てはまる依頼は作者に通知し、作者の判断で `Accept` か `Reject` を送ります。同意の手順を持たない `legacy` の引用は自動の範囲だけで判定し、承認の範囲なら `rejected` にします（依頼を受ける手段がないため）。
- 依頼への `Accept` / `Reject` は依頼のアクティビティを `object` に埋め込みます（ADR-0069 §2 と同じ）。

### 7. 送信: ローカルの人の、リモートの投稿への交流

- リモートの投稿の許可を §3 で判定し、断るなら投稿の作成・ブーストのコマンドで断ります（Web とクライアント API は操作を出さない）。
- **依頼の要らない交流**（自動の範囲に `as:Public` があるか、許可の宣言が無い）: 今と同じく `Create` / `Announce` を配送します。
- **依頼の要る交流**（承認の範囲に当てはまるか、フォロワー・フォロー中・個別の Actor であることで自動の範囲に当てはまる）:
  1. 交流を保留にし、相手の投稿の作者の inbox だけへ `ReplyRequest` / `AnnounceRequest`（`object` は相手の投稿、`instrument` は返信の `Note` か `Announce` を埋め込む）を送ります。
  2. 保留の間、交流は作者本人にだけ「承認待ち」として見せ、ほかのローカルの人にも出さず、配送もしません。
  3. `Accept` を受けたら、`result` の承認を検証して記録し、`replyAuthorization` / `announceAuthorization` を付けた `Create` / `Announce` を元の宛先へ配送します。
  4. `Reject` を受けたら作者に知らせ、交流は作者にだけ「断られた」として見せます。配送はしません。
- **引用**: ADR-0069 §3 の手順のままです。`canQuote` の `manualApproval` に当てはまるときも、同じく `QuoteRequest` を送って `pending` にします。
- 承認の検証は ADR-0069 §2 の `QuoteAuthorization` と同じ規則です。IRI のホストが相手の投稿の作者のホストと同じで、取得した文書の `attributedTo` が相手の投稿の作者、`interactingObject` がこちらの交流、`interactionTarget` が相手の投稿であることを確かめます。

### 8. 依頼と承認の記録

引用の承認の記録を、種類によらない表に移します。どれも `content` が持ちます。

- **交流の依頼**（resource）: 種類・交流の対象の投稿（URI）・交流する側（返信の投稿・`Announce`・引用元の投稿の URI と、そのアカウント）・向き（受けたか送ったか）。`(種類, 交流する側, 対象)` で一意です。
- **依頼への答えの記録**（event）: 承認・拒否・取り消し。承認と取り消しは承認の IRI を持ちます。今の `status_quote_events` はここに移ります。
- **依頼の状態**（projection）: `pending` / `accepted` / `rejected` / `revoked`。今の `status_quote_states` はここに移ります。
  引用の `legacy` と `detached`（編集で引用を外した）は引用だけの事実なので、引用の状態は引用（`status_quotes`）・取り外しの記録・依頼の状態から `content.QuoteStateOf` が導き、Mastodon の `Quote.state` に合わせて返します（ADR-0069 §1）。
- **自サーバーが出した承認**（resource）と **承認の取り消し**（event）: 今の `quote_authorizations` と `quote_authorization_revocations` に種類を足したものです。
  IRI は種類ごとのパス（`/ap/reply_authorizations/{id}`・`/ap/announce_authorizations/{id}`・`/ap/quote_authorizations/{id}`）で、引用の承認の IRI は変わりません。GET は承認の文書を返し、取り消した承認は 410 です。
- 取り消しは、v0.1.0 では引用の承認だけです（ADR-0069 §3）。返信とブーストの承認は取り消さず、作者は返信を隠すにはブロックや通報を使います。

### 9. 送る `interactionPolicy`

- すべてのローカルの投稿の `Note` に、`interactionPolicy` の `canReply`・`canAnnounce`・`canQuote` を付けます。どれも `automaticApproval` を持ち、承認の範囲があれば `manualApproval` も持ちます。
- 公開範囲で決まる制限は値に反映します。
  - `private` / `direct` の `canAnnounce` と `canQuote`、`local` の `canQuote` は作者自身です（ADR-0069 §3 の `local`・§4 と同じ）。
  - `local` の投稿の `canReply`・`canAnnounce` は、リモートへは作者自身にします。リモートからはフォロワー限定に見え、ブーストで外へ広がるためです。
- `@context` は今の `federation.LDContext`（GoToSocial の語彙 `https://gotosocial.org/ns#` を含む）に、`canReply`・`canAnnounce`・`manualApproval`・`ReplyRequest`・`AnnounceRequest`・`ReplyAuthorization`・`AnnounceAuthorization`・`replyAuthorization`・`announceAuthorization` を足します。
- 許可を変えたら、今の引用の許可の変更（`content.quote_policy_changed`）と同じく、現在の版の `Update` を配送します。

### 10. 通知・クライアント API・Web

- 通知: ローカルの人の投稿への承認待ちの依頼で、種類ごとの承認の依頼の通知を作ります。認めていない返信からは、返信とメンションの通知を作りません。依頼が答えられたら、送ったローカルの人に結果を通知します。
- クライアント API（ADR-0016）: パスと名前は [client-api.md](../architecture/client-api.md) の規約で、実装の issue（#2559）で決めます。
  - `Status` は種類ごとの許可（自動の範囲と承認の範囲）と、閲覧者がそれぞれの交流を「できる」「承認待ちになる」「できない」かを持ちます。今の `quote_policy` と `quotable` はここに移ります。
  - 自分の交流が承認待ちか断られたかを `Status` が持ちます。
  - 投稿の作成と、投稿後の許可の変更で、種類ごとの許可を指定できます。利用者の設定は種類ごとの既定値を持ちます。
  - 自分の投稿への承認待ちの依頼の一覧（ページ付き）と、承認・拒否の操作を持ちます。
- Web: 投稿フォームに種類ごとの許可を選ぶ操作を、設定に既定値を、通知から承認の依頼の一覧を出します。設計は [web-client](../architecture/web-client/README.md) に従います。

### 11. 削除

- 許可の表・依頼・答え・状態・承認は、投稿の物理削除（保持期間の消去・アカウントの消去）と一緒に消します（今の引用の表と同じ）。

### 12. この決定を見直す条件

- 返信・ブーストの許可を扱う FEP が別の語彙に決まり、主要な実装がそちらを採ったら、送る形を合わせます。
- Mastodon が `canReply` を読んで許可外の返信を送らなくなったら、許可外の返信を保存する扱い（§6）を見直します。
- 承認の取り消しが返信・ブーストでも要るようになったら、§8 の取り消しを足します。

## 経緯

ローカルの人は、自分の投稿に誰が返信・ブースト・引用できるかを選べなかった。引用だけは ADR-0069 で許可を持ったが、値は 1 つ（`public` / `followers` / `nobody`）で、手動の承認を持たず、リモートの `canQuote` も `automaticApproval` と `manualApproval` を合わせて 1 つの値に畳んで保存していた。そのため、リモートの投稿が承認制かどうかを後から区別できなかった。

返信とブーストにも許可を足すにあたり、種類ごとに表と手順を増やすと、引用と同じ規則（閲覧の可否・ブロック・交流の禁止・承認の検証・取り消し）を 3 回書くことになる。そこで、許可を 1 つの ADR に集め、表も種類によらない 1 つにすることにした。引用の表示と方言は ADR-0069 に残し、誰が引用できるかと承認の手順をこの ADR に移した。

許可の判定と承認の手順は、どちらも `v0.1.0` に入れる。判定だけを先に出すと、承認の範囲を選べず「誰でも」か「断る」かの二択になり、GoToSocial の承認制の投稿にも返信できないためである。

依頼と承認の表は、引用の表を種類によらない形に広げることにした。引用だけ別の流れに残すと、承認の検証と記録を二通り持つことになるためである。`followers` の相手にメンションした人を含めるのは、GoToSocial がメンションした人の返信をいつも認めており、宛てた相手が答えられない形を避けるためである。利用者の既定値は利用者の設定なので `identity` が持ち、投稿の許可の表と同じ形にする。返信・ブーストの承認の取り消しは、使う場面が見えるまで足さない。

連合の語彙は、2026-11-09 に次の資料（各実装の既定ブランチと公開文書）で確かめた。

| 資料・実装 | 中身 |
| --- | --- |
| [GoToSocial の Interaction Controls](https://docs.gotosocial.org/en/latest/federation/interaction_controls/) | `interactionPolicy` の `canLike`・`canReply`・`canAnnounce`、その `automaticApproval` / `manualApproval`（Public・作者の followers / following・個別の Actor の IRI。作者だけは作者の IRI）。`LikeRequest`・`ReplyRequest`・`AnnounceRequest`（`object` が対象、`instrument` が交流）に `Accept`（`result` が `*Authorization`）か `Reject` で答える。承認後の交流は `replyAuthorization` などを持つ。無い副方針は Public を自動で認めるものとみなす。依頼の無い返信は作者の承認待ちとして扱う |
| GoToSocial 0.20〜0.22 のリリースノート | 0.20.0（2025-10-08）で `always` / `approvalRequired` を `automaticApproval` / `manualApproval` に改め両方を送り、0.21.0（2026-02-25）で新しい名前だけにして、依頼を自分でも送るようになった。0.21.0 から `canQuote` を作者だけにして送る。引用は書かない（最新は 0.22.3） |
| [FEP-044f](https://codeberg.org/fediverse/fep/src/branch/main/fep/044f/fep-044f.md)（Draft） | `interactionPolicy` に `canQuote` を足し、`QuoteRequest`・`QuoteAuthorization`。許可は UI の目安で、検証には使わない |
| FEP-5624（Per-object reply control policies） | 2025-06-24 に Withdrawn。返信・ブーストの許可を扱う FEP は、ほかに無い |
| Mastodon 4.5〜4.7（[ActivityPub の文書](https://docs.joinmastodon.org/spec/activitypub/#Quote)、`note_serializer.rb`、`activity.rb`） | `canQuote` の `automaticApproval` だけを送り、`QuoteRequest` に答える。`canReply`・`canAnnounce` は送らず、`ReplyRequest`・`AnnounceRequest` を扱わない |
| [Bonfire](https://bonfirenetworks.org/features) | 境界（見る・返信・引用などの範囲）を GoToSocial の交流の許可と Mastodon の引用の承認に合わせると述べる。1.0rc3 の告知では FEP-044f は対応済み、交流の許可は未定 |

## 採らなかった案

- 種類ごとに表を分ける（`status_reply_policies` など）: 今の引用の表を残せて移行が小さい。ただし同じ規則と列を種類の数だけ持ち、判定も種類ごとに書くことになる。
- 許可外の返信を受け取らずに捨てる: スレッドから確実に消せる。ただし返信はリモートでは公開されており、返信者のプロフィールで見えないと、こちらでの見え方だけが連合先と食い違う。GoToSocial も保存して隠す。
- 相手をアカウントやリストで細かく選べるようにする: Bonfire の境界のように表現が豊かになる。ただし連合先が読める語彙は Public・コレクション・Actor の IRI だけで、v0 では UI と判定が膨らむ。リモートの許可を読むための `account` だけを持つ。
- 承認待ちの返信も最初から全員に配送する: Mastodon の引用と同じ流れで単純になる。ただし作者が断った返信がフォロワーのタイムラインに残り、承認制の意味が薄れ、GoToSocial の取り決め（承認まで保留して配送しない）とも合わない。
- FEP-5624 の `canReply`・`ApproveReply`・`replyApproval` で送る: FEP の形をとれる。ただし FEP は取り下げられ、語彙を読む実装がない。
- お気に入り・絵文字リアクションも制限する: GoToSocial は `canLike` を持つ。ただし Mastodon・Misskey は読まず、制限しても他のサーバーからは止まらないうえ、見えるのは件数だけで害が小さい。

## Consequences（影響）

- 良い影響
  - ローカルの人は、返信・ブースト・引用を種類ごとに、自動と承認の 2 段で制限できます。
  - GoToSocial とは依頼と承認で往復し、Mastodon とは引用の承認が今のまま往復します。
  - 誰が交流できるかを 1 つの関数と 1 つの表で決めるので、種類を足すときに規則を書き直しません。
  - リモートの許可の自動と承認の区別が残るので、承認制の投稿への交流を「承認待ちになる」と示せます。
- 悪い影響・トレードオフ
  - 許可を読まないサーバー（Mastodon・Misskey・Akkoma）の人は、許可外でも返信・ブーストできてしまい、そのサーバーでは表示されます。こちらで隠せるのは、このサーバーでの表示と通知だけです（UI で説明する）。
  - 依頼の要る返信・ブーストは承認まで配送しないので、フォロワーに届くのが遅れ、承認が来ないまま終わるものもあります。フォロワー限定で自動の範囲に入るだけの交流も、依頼を送ることになります。
  - Mastodon から承認制の投稿への返信は依頼を伴わないので、作者が承認するまでスレッドに出ません。
  - 今の引用の表（`status_quote_policies`・`status_quote_events`・`status_quote_states`・`quote_authorizations`・`quote_authorization_revocations`）と `default_quote_policy` を作り直します。`v0.1.0` の前なので移行は持ちません（ADR-0023）。クライアント API の `quote_policy` と `quotable` も変わります。
  - `Note` の文書に `canReply`・`canAnnounce` が増えて少し大きくなります。
- 後続の作業（milestone Interaction Policy）
  - #2556: 許可の 1 つの表と既定値の表、引用の許可からの移し替え
  - #2557: 返信・ブーストの許可の判定（ローカルと連合）
  - #2558: `ReplyRequest`・`AnnounceRequest` と承認の手順、依頼と承認の表
  - #2559: 投稿フォーム・設定・承認の依頼の一覧

## Open Questions / 要検証

要検証（実装の issue で、フィクスチャとライブの相互運用テストで確かめる）:

- GoToSocial の文書は `@context` を `https://gotosocial.org/ns`（`#` なし）と書き、今の `federation.LDContext` は `https://gotosocial.org/ns#` を入れている。GoToSocial と Mastodon がどちらでも同じ語に展開するか。
- Mastodon が `canReply` を読んで、許可外の返信の操作を止めるか（調べた範囲では、読む記述も実装も見つからなかった）。
- Bonfire の境界が `canReply`・`manualApproval` にどう対応し、`ReplyRequest` を送るか・答えるか（文書に連合の詳細がなかった）。
- GoToSocial が、依頼の無い返信を承認したときの `Accept`（`object` が返信）を、こちらから送ったときに受け付けるか。
