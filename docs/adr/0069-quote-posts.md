# ADR-0069: 引用投稿（FEP-044f）

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #891

## Decision（決定）

**FEP-044f を主にし、Misskey の方言を併記します。同意の仕組みを持たない引用（方言だけの引用）は、引用先の作者がリモートなら引用として埋め込み、ローカルなら本人の設定で判定します。**

誰が引用できるか（引用の許可・その既定値・`interactionPolicy.canQuote` の読み書き・手動の承認）と、承認の依頼と記録の表は、返信・ブーストと一緒に [ADR-0155](0155-interaction-policy.md) が持ちます。この ADR は、引用の形式・方言・表示・承認の検証を持ちます。

FEP-044f は Draft です。ただし、同意の手順を持つ形式はこれだけで、Mastodon を含む多くの実装が採っています。そのため、事実上もっとも標準的な形式と判断します。

### 1. 資源と状態

すべて `content` モジュールが持ちます。表と列の名前は、実装の issue で [data-model.md](../architecture/data-model/README.md) に書きます。

- **引用**（resource）: 1 つの投稿は、引用先を 1 つだけ持ちます。
  - 持つもの: 引用元の投稿、引用先の IRI、解決できた引用先の投稿（取得前は空）、形式（`fep-044f` / `legacy`）
  - 引用先は投稿の作成時に決まり、編集では変えられません。外すことはできます（下記）。
- **引用の承認の記録**（event）: 依頼・承認・拒否・取り消し・引用元からの取り外し。承認と取り消しには `QuoteAuthorization` の IRI を持ちます。
- **引用の状態**（projection）: 承認の記録から導きます。値は `pending` / `accepted` / `rejected` / `revoked` / `legacy` で、Mastodon の `Quote.state` に合わせます。
  - 引用先の削除・ブロック・ミュート・閲覧不可は状態に持ちません。表示のたびに、閲覧者ごとに判定します（§4）。
- **引用の許可**: ADR-0155 §2〜§5 の交流の許可の `quote` です。作者が投稿後に変えても、出した承認は取り消しません。取り消しは承認ごとに行います。
- **自サーバーが出した承認**（resource）: ローカルの投稿への引用ごとに 1 行です。`QuoteAuthorization` の IRI は自サーバーの URI（[federation.md §1.2](../architecture/federation.md#12-自サーバーの-uri)）で、GET で返します。取り消しは承認の行を残したまま取り消しのイベント（event）として記録し、IRI の GET は 410 を返します。

### 2. 受信

- 引用先は、`quote`・`_misskey_quote`・`quoteUrl`・`quoteURL`・`quoteUri`・FEP-e232 の `Link`（`rel` が `_misskey_quote` の IRI）の順に、最初に見つかったもの 1 つにします。`quote` があれば形式は `fep-044f`、無ければ `legacy` です。
- 引用先がまだ無ければ、`federation.fetch_object` で取得します。返信の親の取得と同じ予算・ドメインの処分（ADR-0058）・`kernel.HTTPClient` の規則に従います。入れ子の引用は、取得した引用先のさらに引用先を取りに行きません（表示は 1 段まで。§5）。
- `fep-044f` の引用は、次のどちらかのとき `accepted` です。それ以外は `pending` です。
  - 引用元と引用先の作者が同じ
  - `quoteAuthorization` の IRI を取得して、FEP-044f の検証を通る
    - 検証の中身: `attributedTo` が引用先の作者、`interactingObject` が引用元、`interactionTarget` が引用先で、IRI のホストが `attributedTo` のホストと同じ（Mastodon と同じ）
  - `quoteAuthorization` を足す `Update` を受けたら検証し直します。`QuoteAuthorization` の `Delete` を受けたら `revoked` にします。
- `legacy` の引用は次のように扱います。
  - 引用先の作者がリモートなら `legacy` の状態にして、引用として埋め込みます。Misskey・Akkoma 系には同意の仕組みが無く、利用者もそれを前提に引用しているためです。
  - 引用先がローカルの投稿なら、本人の引用の許可と `CanView` で判定します。通れば `legacy`、通らなければ `rejected` にし、本文のリンクだけを出します。
- リモートからの `QuoteRequest`（引用先がローカルの投稿）には、自動で答えます。`Accept` では `result` に `QuoteAuthorization` を付け、`Reject` は理由を付けずに送ります。`Accept` にするのは、次をすべて満たすときです。
  - 依頼者が引用の許可の自動の範囲に当てはまる（ADR-0155 §3）
  - 依頼者がブロックし合っていない
  - 依頼者がその投稿を `CanView` で閲覧できる
  - 交流の禁止（FR-708）を受けていない

  承認の範囲（`manualApproval`）に当てはまる依頼は、作者の判断を待ちます（ADR-0155 §6）。
- 受信の互換拡張（ADR-0006）は、FEP-044f の語彙と、`_misskey_quote`・`quoteUrl`・`quoteUri` を含みます。

### 3. 送信

- 引用元の Note には、次を付けます（Misskey・Akkoma・Sharkey は `quote` を読まないため）。
  - `quote`
  - 同じ IRI の `_misskey_quote`・`quoteUrl`・`quoteUri`
  - 承認があれば `quoteAuthorization`
  - 本文の先頭に、フォールバックの `<p class="quote-inline">RE: <a href="…">…</a></p>`（引用先の表示用 URL。Mastodon v4.7.4 と同じ形。ADR-0018 §13）
- FEP-e232 の `Link` は送りません。読む実装はどれも、上のプロパティのどれかも読むためです。
- すべてのローカルの投稿に `interactionPolicy.canQuote` を付け、引用の許可を知らせます。値は ADR-0155 §2・§9 のとおりです。
  - `@context` には FEP-044f の語彙（`https://w3id.org/fep/044f#`）と GoToSocial の語彙（`https://gotosocial.org/ns#`）、`misskey`・`fedibird` の語彙を入れます。
- リモートの投稿を引用したときの手順は次のとおりです（引用元の公開範囲の制限は §4）。
  1. 引用元の `Create` を配送する
  2. 引用先の作者の inbox へ `QuoteRequest`（`object` は引用先、`instrument` は引用元）を送る。状態は `pending`
  3. `Accept` を受けたら `QuoteAuthorization` を検証し、`accepted` にする。`quoteAuthorization` を足した `Update` を配送する
  4. `Reject` を受けたら `rejected` にする
- リモートの投稿を引用できるかは、引用先の `canQuote` を ADR-0155 §3 で判定して決めます。
  - `canQuote` が無い投稿（Misskey・Akkoma 系）は引用でき、`QuoteRequest` を送らずに `legacy` にします。こちらの表示は §2 の `legacy` と同じく埋め込みです。Mastodon は承認の無い引用として扱い、本文のリンクだけを出します。
- ローカルの投稿を引用したときは、許可を作成時に判定し、通れば承認を出します。`quoteAuthorization` は最初の `Create` に付けます。
- ローカルの人が自分の投稿への引用の承認を取り消すと、引用元の作者の inbox と自分のフォロワーへ `QuoteAuthorization` の `Delete` を送ります。引用元がローカルの投稿なら、状態だけを変えます。
- 引用元の作者は、編集で引用を外せます。外した引用は承認の記録に残し、`Update` の Note から `quote` と併記のプロパティを除きます。
- `local` の投稿（ADR-0017）の扱いは次のとおりです。
  - リモートへは `canQuote` を作者自身にして送り、リモートからの `QuoteRequest` には `Reject` を返します。リモートからはフォロワー限定に見えるので、引用されると外部に広がるためです。
  - ローカルの人は、引用の許可に従って引用できます。

### 4. 公開範囲・ブロック・モデレーション

- 引用できるのは、引用する人が `CanView` で閲覧できる、削除されていない投稿です。
  - `private` の投稿は、作者自身が `private` か `direct` の投稿で引用するときだけ引用できます。
  - `direct` の投稿は引用できません。
  - どちらも `canQuote` は作者自身として送ります（Mastodon と同じ）。
- ブースト（`Announce`）を引用すると、元の投稿の引用になります。
- `local`・`private`・`direct` の投稿は、リモートの投稿を引用できません。投稿の作成で断ります（`quoteTarget`）。`QuoteRequest` の `instrument` が、引用元の IRI を宛先の外へ知らせるためです（ADR-0018 §15）。
- ローカルの `local` の投稿を `public` や `unlisted` で引用することはできます。その Note を連合するときは、`quote`・併記のプロパティ・`quoteAuthorization`・フォールバックを除きます。ローカルの閲覧者には埋め込みを出します。
  - 除くのは引用元の公開範囲によりません。`local` や `private` の引用元の Note も、引用先の作者のフォロワーがいないサーバーへ届くからです。作者が自分の `local` の投稿を引用するときも同じです。
  - 引用先がこのサーバーの投稿で、行がもう無く公開範囲が分からないときも、同じように除きます。
- その引用の `QuoteAuthorization` の GET は `404` です。承認を取り消した後も、引用先を削除した後も同じです。引用先の行がもう無い承認は公開範囲が分からないので、ほかの承認と同じ `410` の `Tombstone` で、示すのは承認自身の URI だけです。
- 埋め込みは、閲覧者が引用先を `CanView` で閲覧できるときだけ出します。閲覧できなければ「表示できない投稿」とだけ出し、中身もリンク先の作者も出しません。
- 閲覧者がブロック・ミュートしている人の投稿や、処分したドメインの投稿は、埋め込みの代わりに理由を出し、開く操作を付けます。理由は Mastodon の `blocked_account` / `blocked_domain` / `muted_account` に合わせます。
- 交流の禁止（FR-708）を受けた人は引用できません。引用先の削除・アカウントの削除・凍結では、引用元は「削除された投稿」を出します。

### 5. 通知・クライアント API・Web

- 通知の種類 `quote`（FR-205）を持ちます。ローカルの人の投稿が引用され、状態が `accepted` か `legacy` になったときに作ります。ミュート・ブロック・通知の除外の規則はお気に入りと同じです。
  - 引用先の編集の通知（Mastodon の `quoted_update`）は作りません。
- クライアント API（ADR-0016）: パスと名前は [client-api.md](../architecture/client-api.md) の規約で、実装時に決めます。
  - `Status` は `quote`（状態と、埋め込める場合の引用先の `Status`）を持ちます。引用先の `quote` は埋め込みません（1 段まで）。
  - 自分がその投稿を引用できるかどうかと、自分の投稿の引用の許可は、ほかの交流の許可と一緒に `Status` が持ちます（ADR-0155 §10）。
  - 投稿の作成で、引用先を指定できるようにします。引用の許可の指定・変更・既定値は ADR-0155 §10 です。
  - 次の操作を持ちます。
    - 自分の投稿への引用の一覧（ページ付き）
    - 承認の取り消し
    - 編集での引用の取り外し
- Web: 設計は [web-client.md](../architecture/web-client/README.md) に従います。
  - ブーストの操作の近くに「引用」を置き、投稿フォームに引用先と引用の許可を出します。
  - 投稿の中に引用先をカードとして埋め込みます。
  - 自分の投稿の引用の一覧から、承認を取り消せるようにします。

### この決定を見直す条件

- Misskey か Akkoma が FEP-044f に対応したら、`legacy` を埋め込む扱いを見直します。それらからの `quote` の無い引用が、同意を避けたものかどうかを区別できるようになるためです。
- FEP-044f の語彙や検証の手順が、Final になるまでに変わったら、それに合わせます。

## 経緯

M4 に引用投稿を足すことになり、FR-114 は S になった（#891）。連合の形式は、もっとも標準的なものを採る方針で選んだ。基準は、ローカルの人の投稿を本人の設定に反して引用させず取り消しが連合先に届くこと、Mastodon・Misskey・Akkoma と引用として表示し合えること（G4）、閲覧できない投稿の中身を見せず判定を `CanView` に通すこと、承認の状態を event と projection で持ち（ADR-0020）v0 で使われない分岐を増やさないことである。

2026-10-22 に、FEP の本文と各実装の既定ブランチのソースで確認した対応は次のとおり。

| 形式 | 状態 | 中身 |
| --- | --- | --- |
| [FEP-044f](https://codeberg.org/fediverse/fep/src/branch/main/fep/044f/fep-044f.md) | Draft | `quote`、`QuoteRequest`、`QuoteAuthorization`、`interactionPolicy.canQuote`、取り消しは `Delete` |
| [FEP-e232](https://codeberg.org/fediverse/fep/src/branch/main/fep/e232/fep-e232.md) | Final | リンク一般の `tag` の `Link`。同意の仕組みはない |
| Misskey の方言 | 慣行 | `_misskey_quote` / `quoteUrl`（Fedibird の `quoteUri` を含む）。同意の仕組みはない |

| 実装 | 送る | 読む | 同意 |
| --- | --- | --- | --- |
| Mastodon 4.5 以降（[note_serializer.rb](https://github.com/mastodon/mastodon/blob/main/app/serializers/activitypub/note_serializer.rb)、[status_parser.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/parser/status_parser.rb)） | `quote`・`_misskey_quote`・`quoteUri`・`quoteAuthorization`・`canQuote` の `automaticApproval` | `quote`・`_misskey_quote`・`quoteUrl`・`quoteUri` | FEP-044f。`quote` の無い引用は承認待ちとして本文のリンクだけを出す。`canQuote` の無いリモートの投稿は引用させない |
| Misskey 2026.10（[ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApRendererService.ts)、[ApNoteService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/models/ApNoteService.ts)） | `_misskey_quote`・`quoteUrl` | `_misskey_quote`・`quoteUrl` | なし（FEP-044f は [#16590](https://github.com/misskey-dev/misskey/issues/16590) で未対応） |
| Akkoma 3.21（[transmogrifier.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/transmogrifier.ex)） | `quoteUri`・`quoteUrl` | `quoteUri`・`quoteUrl`・`_misskey_quote` | なし（FEP-044f の [PR #1086](https://akkoma.dev/AkkomaGang/akkoma/pulls/1086) は 2026-08-12 に却下） |
| GoToSocial 0.22 | 引用はしない。`canQuote` を作者だけにして送る | — | `Delete` の `QuoteAuthorization` を受け流す |
| Sharkey | `_misskey_quote`・`quoteUrl`・`quoteUri`・FEP-e232 の `Link` | 左の全部と `quote` | なし |

引用の許可は、はじめこの ADR が `public` / `followers` / `nobody` の 1 つの値（手動の承認なし）として持ち、リモートの `canQuote` も自動と手動を合わせた 1 つの値に畳んでいた。返信・ブーストにも許可を足すにあたり、許可と承認の手順を種類によらない形にまとめるため、ADR-0155 に移した（#2555）。

当初、フォールバックは FEP-044f の例の形（本文の末尾の `<span class="quote-inline"><br>RE: …</span>`）で、引用元の公開範囲は自由だった。`v0.1.0` の前の連合の監査（#2027）で、Mastodon の実際の形に合わせて先頭の `<p>` にし、`local` の投稿の IRI と作者が作者のフォロワーのいないサーバーへ届く（#2066）ので引用元の公開範囲を狭めた（ADR-0018）。

## 採らなかった案

- Misskey の方言だけを送受信する: 実装が小さく、Misskey 系との往復だけなら足りる。ただし Mastodon 4.4 以降は `quote` と承認の無い引用を埋め込まないので M4 の完了条件の相互運用（Mastodon）を満たせず、引用される人の同意も守れない。
- FEP-e232 の `Link` を主にする: Final の FEP だが、リンク一般の仕組みで同意がなく、Mastodon・Misskey・Akkoma は読まないので相互運用が得られない。
- 承認の無い引用は Mastodon と同じく埋め込まず、本文のリンクだけを出す: Mastodon と見え方が揃い、規則が「承認があるか」だけで済む。ただし Misskey・Akkoma の人どうしの引用は相手のサーバーでは同意なしに引用として見えており、こちらだけ埋め込まなくても引用される人を守ることにはならない。
- `QuoteRequest` だけを止め、引用元の公開範囲を狭めない（`pending` のまま）/ 引用元の公開範囲を自由のままにする: 前者は `pending` のまま終わらない引用を残し、後者は `local` の約束（ADR-0017）を説明で済ませるだけになる。

## Consequences（影響）

- 良い影響
  - Mastodon とは、同意の手順を含めて引用が往復します。Misskey・Akkoma とも、引用として表示し合えます。
  - ローカルの人の投稿は、本人の許可に反して FEP-044f の実装から引用されず、取り消しも届きます。
  - 引用先の閲覧の可否は、`CanView` の 1 か所で判定します。
- 悪い影響・トレードオフ
  - Misskey・Akkoma の人がローカルの人の投稿を引用すると、同意の手順なしに相手のサーバーで表示されます。こちらでは許可で判定して埋め込みを止めますが、相手のサーバーでの表示は止められません（ActivityPub の本質的な制約。UI で説明する）。
  - こちらの人が Misskey・Akkoma の投稿を引用すると、Mastodon では承認の無い引用として、本文のリンクだけが出ます。
  - こちらは `legacy` の引用を埋め込みますが、Mastodon は埋め込みません。同じ投稿の見え方が、Mastodon とこちらで違います。
  - 1 つの引用を 4 つのプロパティで併記するので、Note の文書が少し大きくなります。
  - `local`・`private`・`direct` の投稿から、リモートの投稿を引用できません。`local` の投稿を引用した公開の投稿は、連合先ではただの投稿に見えます。

## Open Questions / 要検証

- 要検証: Pleroma 本家（`git.pleroma.social`）が読み書きする引用のプロパティ（調査時に接続できなかった）。Iceshrimp.NET の送信の形。
- 要検証: Mastodon が、`canQuote` を付けた `local` 相当の投稿（フォロワー限定）に `QuoteRequest` を送ってくるか。送ってきたときに `Reject` で問題がないか（ライブ相互運用テストで確かめる）。
