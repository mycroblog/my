# ADR-0146: 引用の連合と公開範囲

- Status: Accepted
- Date: 2026-11-03
- Deciders: @rosylilly
- Issue: #2066

## Context（背景・課題）

[ADR-0069](0069-quote-posts.md) §4 は「引用元の公開範囲は自由」とし、§3 は引用を `QuoteRequest` と、`quote` などを持つ `Note` で連合すると決めています。この 2 つを合わせると、[ADR-0017](0017-visibility-local.md) の「`local` は作者のフォロワーを超えて連合しない」が崩れる場面が 2 つあります。

- `local` の投稿がリモートの投稿を引用すると、引用先の作者の inbox へ `QuoteRequest` が届き、`instrument` が `local` の投稿の IRI を伝える。本文は `GET` が 404 を返すので守られるが、投稿の存在と IRI は読者の外へ出る。`private` と `direct` の引用元も同じ。
- ローカルの利用者が別のローカルの利用者の `local` の投稿を公開の投稿で引用すると、`Note` の `quote`・その別名・`quoteAuthorization`・代替リンク `/@author/...` が、引用先の作者のフォロワーがいないサーバーへ IRI と作者を伝える。`GET /ap/quote_authorizations/{id}` も両方の IRI をだれにでも返す。

## Decision Drivers（判断基準）

- `local` の投稿の IRI と作者も、本文と同じく読者の外へ出さないこと（ADR-0017）
- 引用先の作者の同意を飛ばさないこと（ADR-0069）
- ローカルの閲覧者の埋め込みは今のまま出せること

## Considered Options（選択肢）

1. `local` などの引用元からの `QuoteRequest` だけ送らない: 引用が同意なしで `pending` のまま残り、状態の行き先がない
2. 引用の時点で断る（採用）
3. `local` の投稿の本文だけを守り、IRI と作者は連合してよいとする: ADR-0017 の範囲を狭める

## Decision（決定）

Decider の決定（#2066）: 「A `local` post may not quote a remote post, and `private` and `direct` quoting posts are treated the same. A public quote of a `local` post federates neither its URI nor its author, and that QuoteAuthorization answers 404.」

この決定で、ADR-0069 §4 の「引用元の公開範囲は自由」を次のように置き換えます。

1. 公開範囲が `local`・`private`・`direct` のローカルの投稿は、リモートの投稿を引用できません。投稿の作成が `422` で失敗し（`content.ErrQuoteRemoteAudience`）、投稿も引用も作らず、`QuoteRequest` も送りません。引用先が `canQuote` を宣言しているかどうかによりません。
2. 引用先が `local` の投稿のとき、連合する `Note` は `quote`・その別名・`quoteAuthorization`・代替リンクを持ちません。引用元の公開範囲によりません。`local` や `private` の引用元の `Note` も、引用先の作者のフォロワーがいないサーバーへ届くからです。
3. その引用の承認（`quote_authorizations` の行）と状態は変わらず、ローカルの閲覧者の埋め込みは `CanView` のとおりに出ます。
4. 引用先が `local` の投稿の `GET /ap/quote_authorizations/{id}` は、取り消しや削除の後も含めて `404` を返します。

投稿が消えて公開範囲が分からないこのサーバーの URI も、`local` と同じく `Note` に載せません。

## Consequences（影響）

- `Status.quotable` は引用元の公開範囲を知らないので、リモートの投稿に対しては `true` のままです。公開範囲が合わないと、投稿の作成が専用のメッセージの `422` になります。
- 公開の投稿が `local` の投稿を引用していることは、リモートのサーバーからは分かりません。リモートの読者には引用のない投稿に見えます。
- 公開範囲は編集で変わらず、引用は作成でしか付かないので、作成時の判定だけで足ります。
