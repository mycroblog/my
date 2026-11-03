# ADR-0069: 引用投稿（FEP-044f）

- Status: Accepted（§3 のフォールバックの形と、§3・§4 の引用元の公開範囲を ADR-0144 が置き換える）
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #891

## Context（背景・課題）

決定者は、M4 に引用投稿を足すと決めました（#891）。原文は次のとおりです。

> M4 の追加タスクとして引用投稿ができるようになってほしい
>
> - FR-114 の優先度を W から M / S に上げるかどうかと、要件の書き直し : S
> - 連合の形式。Mastodon 4.5 の FEP-044f（quote / QuoteRequest / QuoteAuthorization） : もっとも標準的なものを採用

決定者の回答（PR #895、原文）:

> 1. Misskey・Akkoma からの承認の無い引用 : 推奨で OK
> 2. 承認の方式: 承認は自動だけにします。 : OK
> 3. 公開範囲 : OK
> 4. 通知: 引用されたときの通知 quote を足します。 : OK
>
> ADR Accepted

FR-114（引用投稿）は S になります。M4 の完了条件は Must / Should の FR が使えることです（[00-vision.md](../requirements/00-vision.md)）。

main のコードは次のとおりです。

- 受信は引用を読みません。`_misskey_quote` / `quoteUrl` / `quoteUri` は未知のプロパティとして捨て、本文中のリンクとしてだけ残ります（[federation.md §4](../architecture/federation.md#4-as2-の正規化)）。HTML のサニタイズは、フォールバックの `quote-inline` の class を残します（§5）。
- 送信する Note（`internal/app/federation/document.go`）に、引用と `interactionPolicy` はありません。
- 閲覧の可否は、単一のポリシー関数 `content.CanView`（`internal/domain/content/view.go`、ADR-0017）で決めます。
- リモートの投稿は、ジョブ `federation.fetch_object` で取得します（`internal/app/federation/threads.go`）。

引用の形式と、主な実装の対応は次のとおりです（2026-10-22 に、FEP の本文と各実装の既定ブランチのソースで確認）。

| 形式 | 状態 | 中身 |
| --- | --- | --- |
| [FEP-044f](https://codeberg.org/fediverse/fep/src/branch/main/fep/044f/fep-044f.md) | Draft | 引用先を `quote` で示す。引用される側の同意は 3 つで扱う。引用元が `QuoteRequest` を送る。引用される側は `Accept` で `QuoteAuthorization`（承認の証明）を返す。引用元はそれを `quoteAuthorization` で示す。引用の許可は `interactionPolicy.canQuote`（GoToSocial の語彙）で知らせ、取り消しは `QuoteAuthorization` の `Delete` で行う。フォールバックとして本文に `<span class="quote-inline">RE: <a>…</a></span>` を付ける |
| [FEP-e232](https://codeberg.org/fediverse/fep/src/branch/main/fep/e232/fep-e232.md) | Final | 他のオブジェクトへのリンク一般を表す `tag` の `Link`。`mediaType` は AS2 の JSON-LD で、`rel` に `_misskey_quote` の IRI を付けると引用になる。同意の仕組みはない |
| Misskey の方言 | 慣行 | `_misskey_quote` / `quoteUrl`（Fedibird の `quoteUri` を含む）。同意の仕組みはない |

| 実装 | 送る | 読む | 同意 |
| --- | --- | --- | --- |
| Mastodon 4.5 以降（[note_serializer.rb](https://github.com/mastodon/mastodon/blob/main/app/serializers/activitypub/note_serializer.rb)、[status_parser.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/parser/status_parser.rb)） | `quote`・`_misskey_quote`・`quoteUri`・`quoteAuthorization`・`canQuote` の `automaticApproval` | `quote`・`_misskey_quote`・`quoteUrl`・`quoteUri` | FEP-044f。`quote` の無い引用は承認待ちとして本文のリンクだけを出す。`canQuote` の無いリモートの投稿は引用させない |
| Misskey 2026.10（[ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApRendererService.ts)、[ApNoteService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/models/ApNoteService.ts)） | `_misskey_quote`・`quoteUrl` | `_misskey_quote`・`quoteUrl` | なし（FEP-044f は [#16590](https://github.com/misskey-dev/misskey/issues/16590) で未対応） |
| Akkoma 3.21（[transmogrifier.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/transmogrifier.ex)） | `quoteUri`・`quoteUrl` | `quoteUri`・`quoteUrl`・`_misskey_quote` | なし（FEP-044f の [PR #1086](https://akkoma.dev/AkkomaGang/akkoma/pulls/1086) は 2026-08-12 に却下） |
| GoToSocial 0.22 | 引用はしない。`canQuote` を作者だけにして送る | — | `Delete` の `QuoteAuthorization` を受け流す |
| Sharkey | `_misskey_quote`・`quoteUrl`・`quoteUri`・FEP-e232 の `Link` | 左の全部と `quote` | なし |

FEP-044f の実装の一覧には、ほかに Bonfire・Friendica・Iceshrimp.NET・Pixelfed（引用される側だけ）・WordPress の ActivityPub プラグイン・PieFed などがあります。

この ADR は、引用の資源・連合の形・同意・公開範囲・API・通知を決めます。要件 FR-114 の書き直しと FR-205 の通知の拡張の根拠になります。

## Decision Drivers（判断基準）

1. **標準への準拠**（決定者の指示）: もっとも標準的な形式を主にすること。
2. **引用される人の同意**: ローカルの人の投稿を、本人の設定に反して引用させないこと。取り消しが連合先に届くこと。
3. **相互運用（G4）**: Mastodon・Misskey・Akkoma で引用として表示され、相手からの引用もこちらで引用として表示されること。
4. **公開範囲の規律**: 引用で、閲覧できない投稿の中身を見せないこと。`local`（ADR-0017）の「外部へはフォロワーにしか届かない」を崩さないこと。判定は `CanView` を通すこと。
5. **データモデルの規律（ADR-0020）と実装量**: 承認の状態を event と projection で持つこと。手動の承認など、v0 で使われない分岐を増やさないこと。

## Considered Options（選択肢）

### 主にする形式

1. **A: FEP-044f を主にし、Misskey の方言を併記する** — `quote` と同意の手順を実装します。送信では `_misskey_quote`・`quoteUrl`・`quoteUri` を同じ値で併記し、受信ではこれらも読みます。
2. **B: Misskey の方言だけ** — `_misskey_quote` / `quoteUrl` を送受信し、同意の仕組みは持ちません。
3. **C: FEP-e232 の `Link` を主にする** — Final の FEP で、`rel` で引用を示します。

### 同意の仕組みを持たない引用（方言だけの引用）の受信

1. **X: 引用される投稿の作者がリモートなら引用として埋め込み、ローカルなら本人の設定で判定する**
2. **Y: Mastodon と同じく、承認の無い引用は埋め込まず、本文のリンクだけを出す**

## Decision（決定）

**形式は A、同意の仕組みを持たない引用の受信は X にします。**

FEP-044f は Draft です。ただし、同意の手順を持つ形式はこれだけで、Mastodon を含む多くの実装が採っています。そのため、事実上もっとも標準的な形式と判断します（判断基準 1・2）。
FEP-e232 は Final ですが、リンク一般の仕組みです。Mastodon・Misskey・Akkoma は読まないので、主にしても相互運用が得られません。

### 1. 資源と状態

すべて `content` モジュールが持ちます。表と列の名前は、実装の issue で [data-model.md](../architecture/data-model/README.md) に書きます。

- **引用**（resource）: 1 つの投稿は、引用先を 1 つだけ持ちます。
  - 持つもの: 引用元の投稿、引用先の IRI、解決できた引用先の投稿（取得前は空）、形式（`fep-044f` / `legacy`）
  - 引用先は投稿の作成時に決まり、編集では変えられません。外すことはできます（下記）。
- **引用の承認の記録**（event）: 依頼・承認・拒否・取り消し・引用元からの取り外し。承認と取り消しには `QuoteAuthorization` の IRI を持ちます。
- **引用の状態**（projection）: 承認の記録から導きます。値は `pending` / `accepted` / `rejected` / `revoked` / `legacy` で、Mastodon の `Quote.state` に合わせます。
  - 引用先の削除・ブロック・ミュート・閲覧不可は状態に持ちません。表示のたびに、閲覧者ごとに判定します（§4）。
- **ローカルの投稿の引用の許可**: `public`（誰でも）/ `followers`（フォロワー）/ `nobody`（自分だけ）。
  - ローカルの投稿の属性として持ち、既定値は利用者の設定にします。設定の初期値は `public` です。
  - 作者は、投稿後もいつでも変えられます。変えても、出した承認は取り消しません。取り消しは承認ごとに行います。
- **自サーバーが出した承認**（resource）: ローカルの投稿への引用ごとに 1 行です。`QuoteAuthorization` の IRI は自サーバーの URI（[federation.md §1.2](../architecture/federation.md#12-自サーバーの-uri)）で、GET で返します。取り消しは承認の行を残したまま取り消しのイベント（event）として記録し、IRI の GET は 410 を返します。

### 2. 受信

- 引用先は、`quote`・`_misskey_quote`・`quoteUrl`・`quoteURL`・`quoteUri`・FEP-e232 の `Link`（`rel` が `_misskey_quote` の IRI）の順に、最初に見つかったもの 1 つにします。`quote` があれば形式は `fep-044f`、無ければ `legacy` です。
- 引用先がまだ無ければ、`federation.fetch_object` で取得します。返信の親の取得と同じ予算・ドメインの処分（ADR-0036）・`kernel.HTTPClient` の規則に従います。入れ子の引用は、取得した引用先のさらに引用先を取りに行きません（表示は 1 段まで。§5）。
- `fep-044f` の引用は、次のどちらかのとき `accepted` です。それ以外は `pending` です。
  - 引用元と引用先の作者が同じ
  - `quoteAuthorization` の IRI を取得して、FEP-044f の検証を通る
    - 検証の中身: `attributedTo` が引用先の作者、`interactingObject` が引用元、`interactionTarget` が引用先で、IRI のホストが `attributedTo` のホストと同じ（Mastodon と同じ）
  - `quoteAuthorization` を足す `Update` を受けたら検証し直します。`QuoteAuthorization` の `Delete` を受けたら `revoked` にします。
- `legacy` の引用は X で扱います。
  - 引用先の作者がリモートなら `legacy` の状態にして、引用として埋め込みます。Misskey・Akkoma 系には同意の仕組みが無く、利用者もそれを前提に引用しているためです。
  - 引用先がローカルの投稿なら、本人の引用の許可と `CanView` で判定します。通れば `legacy`、通らなければ `rejected` にし、本文のリンクだけを出します。
- リモートからの `QuoteRequest`（引用先がローカルの投稿）には、自動で答えます。`Accept` では `result` に `QuoteAuthorization` を付け、`Reject` は理由を付けずに送ります。`Accept` にするのは、次をすべて満たすときです。
  - 依頼者が引用の許可（§1）に当てはまる
  - 依頼者がブロックし合っていない
  - 依頼者がその投稿を `CanView` で閲覧できる
  - 交流の禁止（FR-708）を受けていない

  手動の承認（`manualApproval`）は持ちません。
- 受信は ADR-0006 の互換拡張に、FEP-044f の語彙と、`_misskey_quote`・`quoteUrl`・`quoteUri` を加えます。ADR-0006 の他の部分は変えません。

### 3. 送信

- 引用元の Note には、次を付けます（Misskey・Akkoma・Sharkey は `quote` を読まないため）。
  - `quote`
  - 同じ IRI の `_misskey_quote`・`quoteUrl`・`quoteUri`
  - 承認があれば `quoteAuthorization`
  - 本文の末尾に、フォールバックの `<span class="quote-inline"><br>RE: <a href="…">…</a></span>`（引用先の表示用 URL）
- FEP-e232 の `Link` は送りません。読む実装はどれも、上のプロパティのどれかも読むためです。
- すべてのローカルの公開投稿に `interactionPolicy.canQuote.automaticApproval` を付け、引用の許可を知らせます。値は `public` なら `as:Public`、`followers` なら作者の followers コレクション、`nobody` なら作者自身です。
  - `@context` には FEP-044f の語彙（`https://w3id.org/fep/044f#`）と GoToSocial の語彙（`https://gotosocial.org/ns#`）、`misskey`・`fedibird` の語彙を足します。
- リモートの投稿を引用したときの手順は次のとおりです。
  1. 引用元の `Create` を配送する
  2. 引用先の作者の inbox へ `QuoteRequest`（`object` は引用先、`instrument` は引用元）を送る。状態は `pending`
  3. `Accept` を受けたら `QuoteAuthorization` を検証し、`accepted` にする。`quoteAuthorization` を足した `Update` を配送する
  4. `Reject` を受けたら `rejected` にする
- リモートの投稿を引用できるかは、引用先の `canQuote` で決めます。
  - `canQuote` があり、自分が `automaticApproval` か `manualApproval` に当てはまらなければ、引用できません。
  - `canQuote` が無い投稿（Misskey・Akkoma 系）は引用でき、`QuoteRequest` を送らずに `legacy` にします。こちらの表示は X と同じく埋め込みです。Mastodon は承認の無い引用として扱い、本文のリンクだけを出します。
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
- 引用元の公開範囲は自由です。ただし埋め込みは、閲覧者が引用先を `CanView` で閲覧できるときだけ出します。閲覧できなければ「表示できない投稿」とだけ出し、中身もリンク先の作者も出しません。
- 閲覧者がブロック・ミュートしている人の投稿や、処分したドメインの投稿は、埋め込みの代わりに理由を出し、開く操作を付けます。理由は Mastodon の `blocked_account` / `blocked_domain` / `muted_account` に合わせます。
- 交流の禁止（FR-708）を受けた人は引用できません。引用先の削除・アカウントの削除・凍結では、引用元は「削除された投稿」を出します。

### 5. 通知・クライアント API・Web

- 通知の種類に `quote` を足します（FR-205）。ローカルの人の投稿が引用され、状態が `accepted` か `legacy` になったときに作ります。ミュート・ブロック・通知の除外の規則はお気に入りと同じです。
  - 引用先の編集の通知（Mastodon の `quoted_update`）は作りません。
- クライアント API（ADR-0016）: パスと名前は [client-api.md](../architecture/client-api.md) の規約で、実装時に決めます。
  - `Status` に `quote`（状態と、埋め込める場合の引用先の `Status`）を足します。引用先の `quote` は埋め込みません（1 段まで）。
  - 自分がその投稿を引用できるかどうかと、自分の投稿の引用の許可を `Status` に足します。
  - 投稿の作成で、引用先と引用の許可を指定できるようにします。
  - 次の操作を足します。
    - 投稿の引用の許可の変更
    - 自分の投稿への引用の一覧（ページ付き）
    - 承認の取り消し
    - 編集での引用の取り外し
  - 利用者の設定に、引用の許可の既定値を足します。
- Web: 設計は [web-client.md](../architecture/web-client/README.md) に従います。
  - ブーストの操作の近くに「引用」を置き、投稿フォームに引用先と引用の許可を出します。
  - 投稿の中に引用先をカードとして埋め込みます。
  - 自分の投稿の引用の一覧から、承認を取り消せるようにします。

### この決定を見直す条件

- Misskey か Akkoma が FEP-044f に対応したら、`legacy` を埋め込む扱い（X）を見直します。それらからの `quote` の無い引用が、同意を避けたものかどうかを区別できるようになるためです。
- FEP-044f の語彙や検証の手順が、Final になるまでに変わったら、それに合わせます。
- 手動の承認が要るようになったら、`manualApproval` と承認待ちの一覧を足します。

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
  - 手動の承認は無く、引用の許可は「全員・フォロワー・自分だけ」の 3 つに限られます。
- 後続で必要になる作業（Issue 化する）
  - #892 受信: 資源・承認の記録・状態のスキーマ、受信の正規化と `QuoteAuthorization` の検証、引用先の取得、`Status.quote`、Web の埋め込み、フィクスチャ（Mastodon 4.5・Misskey・Akkoma の引用）のコントラクトテスト
  - #893 作成と同意: 作成と送信、`QuoteRequest` の送受信、`QuoteAuthorization` の発行・GET・取り消し、`canQuote`、引用の許可の設定、通知 `quote`、Web の操作。`make federation-live` に Mastodon・Misskey との引用の往復を足す
  - [federation.md](../architecture/federation.md)・data-model.md・client-api.md の更新（実装の PR で行う）

## Pros and Cons of the Options（詳細比較・任意）

| | A: FEP-044f + 方言の併記（採用） | B: Misskey の方言だけ | C: FEP-e232 |
| --- | --- | --- | --- |
| Mastodon で引用として見える | ○ | ✕（承認待ちとして本文のリンクだけ） | ✕（読まない） |
| Misskey・Akkoma で引用として見える | ○（併記） | ○ | ✕（読まない） |
| 引用される人の同意 | ○ | ✕ | ✕ |
| 実装量 | 大きい（同意の手順・検証・取り消し） | 小さい | 小さい |
| 仕様の安定度 | Draft | 文書なし | Final |

B の一番の利点は実装の小ささです。Misskey 系との往復だけを見れば、B で足ります。
ただし Mastodon 4.4 以降は、`quote` と承認の無い引用を埋め込みません。そのため M4 の完了条件の相互運用（Mastodon）を満たせず、引用される人の同意も守れません。

| | X: リモートの作者なら埋め込む（採用） | Y: 承認の無い引用は埋め込まない |
| --- | --- | --- |
| Misskey・Akkoma の人どうしの引用 | 引用として見える | 本文のリンクだけ |
| ローカルの人の投稿への方言の引用 | 本人の許可で判定 | 埋め込まない |
| Mastodon との見え方の一致 | ずれる | 一致する |

Y の一番の利点は、Mastodon と見え方が揃い、規則が「承認があるか」だけで済むことです。
ただ、Misskey・Akkoma の人どうしの引用は、相手のサーバーでは同意の手順が無いまま引用として見えています。こちらだけ埋め込まなくても、引用される人を守ることにはなりません。

## Open Questions / 要検証

- 要検証: Pleroma 本家（`git.pleroma.social`）が読み書きする引用のプロパティ（調査時に接続できなかった）。Iceshrimp.NET の送信の形。
- 要検証: Mastodon が、`canQuote` を付けた `local` 相当の投稿（フォロワー限定）に `QuoteRequest` を送ってくるか。送ってきたときに `Reject` で問題がないか（ライブ相互運用テストで確かめる）。
