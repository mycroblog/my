# 連合の設計（ActivityPub S2S）

> 前提にしている ADR: [ADR-0002](../adr/0002-modular-monolith-hexagonal.md)、[ADR-0003](../adr/0003-database-abstraction.md)、[ADR-0004](../adr/0004-job-queue-abstraction.md)、[ADR-0005](../adr/0005-blob-storage-and-media.md)、[ADR-0006](../adr/0006-federation-protocol-scope.md)、[ADR-0017](../adr/0017-visibility-local.md)、[ADR-0018](../adr/0018-federated-uris.md)、[ADR-0019](../adr/0019-security-baseline.md)、[ADR-0020](../adr/0020-data-modeling-principles.md)。
> 署名は [ADR-0032](../adr/0032-http-signature-profile.md)、受信処理は [ADR-0033](../adr/0033-inbox-processing-model.md)、取り込み範囲と保持は [ADR-0034](../adr/0034-remote-content-scope-and-retention.md)、`local` 投稿のメンションは [ADR-0035](../adr/0035-local-visibility-mention-tags.md)、リレーは [ADR-0061](../adr/0061-federation-relays.md) に従います。
> 要件: FR-305、FR-401〜411、FR-701〜702、FR-709、FR-713、FR-716〜717、NFR-MOD-3、NFR-SEC-1〜2、NFR-SEC-4〜5、NFR-PERF-4、NFR-REL-1〜2、NFR-COST-4〜5。テーブルは [論理データモデル](data-model/README.md) §4.8 ほかにあります。

## 1. 構成

### 1.1 モジュールとパッケージ

| パッケージ | 持つもの |
| --- | --- |
| `internal/domain/federation` | AS2 の正規化した型と、受信した JSON からの正規化（§4）。宛先から公開範囲への写像、配送先の算出規則、配送の結果の分類、サーキットブレーカーの規則。標準ライブラリだけを使う純粋な関数 |
| `internal/app/federation` | ユースケース（受信 `Receive`、アクティビティの処理、配送の展開 `FanOut`、配送 `Deliver`、Actor / オブジェクトの取得 `Fetch`、解決 `Resolve`）、ジョブハンドラ、ポート（`Signer`・`Verifier`・リポジトリ） |
| `internal/app/kernel` | `HTTPClient` ポート（連合のほか、メディアプロキシ・リンクプレビュー・Web Push も使うので全モジュール共通に置く） |
| `internal/adapter/httpclient` | SSRF ガード付きの `HTTPClient`（ADR-0019 §1）。コントラクトテストは `testkit.HTTPClientContract` |
| `internal/adapter/htmlsanitizer` | `kernel.HTMLSanitizer` ポートの実装（§5、ADR-0019 §2）。`bluemonday` はこの中に閉じ込める。ポリシーの版は定数 `PolicyVersion`。コントラクトテストは `testkit.HTMLSanitizerContract` |
| `internal/adapter/httpsig` | `Signer` / `Verifier` の実装。RFC 9421 は `github.com/dadrus/httpsig`、draft-cavage-12 は `code.superseriousbusiness.org/httpsig`（ADR-0006）。ライブラリはこの中に閉じ込める |
| `internal/interface/activitypub` | HTTP の入口（Inbox の `POST`、Actor・オブジェクト・コレクションの `GET`、WebFinger、NodeInfo）。ユースケースを呼ぶだけ |
| `internal/interface/media` | メディアプロキシ `/media/proxy/…`（§9） |

- 受け入れのポリシー（必須の対象・時間窓・鍵と `actor` の一致）は `internal/app/federation` に書き、暗号処理と署名ベースの組み立てだけをアダプタに任せます（ADR-0006）。
- テーブルの所有者はモジュール単位です（ADR-0002）。`federation` が書くのは §4.8 のテーブルだけで、リモートのアカウント・投稿・フォローなどは、それぞれのモジュールのアプリケーション層の公開 API を **同じトランザクションの中で** 呼んで書きます（利用者登録と同じ形。[data-model §2.1](data-model/02-consistency.md#21-トランザクションの規律)）。

| 受信した内容 | 呼ぶ公開 API（書くモジュール） |
| --- | --- |
| Actor の文書 | `account.UpsertRemoteActor`（`accounts`・`remote_accounts`・`remote_account_snapshots`・プロフィールの版・`account_keys`） |
| `Create` / `Update` の投稿 | `content.IngestRemoteStatus` / `content.ReviseRemoteStatus` |
| `Delete` の投稿 / Actor | `content.DeleteRemoteStatus` / `account.MarkRemoteDeleted` |
| `Announce` / `Undo Announce` | `content.RecordRemoteBoost` / `content.RetractRemoteBoost` |
| `Like` / `Undo Like` | `content.RecordRemoteFavourite` / `content.RemoveRemoteFavourite` |
| `Follow` / `Accept` / `Reject` / `Undo Follow` | `graph.ReceiveFollow` / `graph.AcceptRemote` / `graph.RejectRemote` / `graph.TerminateRemote` |
| `Block` / `Undo Block` | `graph.ReceiveBlock` / `graph.RetractRemoteBlock` |
| `Flag` | `moderation.FileRemoteReport` |

### 1.2 自サーバーの URI

- 受信した文書の中の自サーバーの URI（ホストが設定 `host` のもの）は、ADR-0018 の正規表現でパースして DB を引き、HTTP で取得しません。パースできない自サーバーの URI は存在しないものとして扱います。
- リモートの URI は、`https` で、ホストを正規化した IRI として正規化してから比較・保存します。フラグメントは `#main-key` などの鍵を除いて保存しません。

#### ホスト名の正規化（IDN）

- 外から入るホストはすべて、比較・保存の前に IDNA 2008 の A-label（小文字の punycode）にそろえます。U-label（`例え.jp`）と A-label（`xn--r8jz45g.jp`）、全角や大文字で書かれた同じホストは同じ値になり、オリジンの照合（`id` と `attributedTo`、`keyId` と `actor`、取得結果の `id`）、ドメイン単位の処分、`InstanceHealth`、shared inbox の集約で同じホストとして扱われます。
- 写像は `golang.org/x/net/idna` で行うので、`internal/app/federation` に置きます（ドメイン層は標準ライブラリだけを使うため）。
  - 受信・取得した文書は `federation.Normalize`（アプリケーション層）で読み、文書の中のすべての IRI のホストを写像します。
  - 文書の外から来る IRI（取得する URL・`keyId`・利用者が入力した URL）は `federation.CanonicalIRI`、WebFinger の `acct:` のドメインと、運営者が入力するドメイン単位の処分のドメインは `federation.CanonicalHost` を通します。
  - ドメイン単位の処分の照合は、正規化したホストとその親ドメイン（`federation.DomainAndParents`）で引きます。
- 写像は WHATWG URL のホストの解析と同じ UTS #46 の非推移的処理です（`ß` は `ss` にしない）。Bidi 規則と ContextJ を確かめ、ラベルと名前の長さを検証します。ラベルの 3・4 文字目の `--`（`r3---sn-x.example` など）と `_` は、実在するホストがあるので許します。
- 写像できないホスト（解読できない `xn--` ラベル、許されない文字、空のラベル、長すぎるラベル）は不正です。文書の中の不正な IRI は無いものとして読み、文書の外から来たものはエラーにします。
- IP アドレスはそのままの表記（小文字）で扱います。末尾の `.` は取り除きます。
- **表示**: ホストを人に見せるときは `federation.DisplayHost` で U-label に戻します。ただし、なりすましを防ぐため、どれかのラベルが UTS #39 §5.2 の「highly restrictive」に当てはまらない（文字が 1 つの用字だけでも、ラテン文字＋漢字＋ひらがな・カタカナ、ラテン文字＋漢字＋注音字母、ラテン文字＋漢字＋ハングルのどれかの組み合わせでもない）ときは、ホスト全体を A-label のまま表示します。キリル文字の `а` とラテン文字の混じった `xn--pple-43d.com` は A-label で表示されます。保存と比較には常に A-label を使います。

## 2. 署名

ADR-0032 のプロファイルに従います。要点:

- **受信**: `Signature-Input` があれば RFC 9421、なければ draft-cavage-12 で検証します。POST はダイジェスト（`Digest` か `Content-Digest`）が必須で、受け取った本文から計算した値と照合します。
- **送信**: draft-cavage-12（`rsa-sha256`）で送り、400 / 401 なら RFC 9421 で 1 回だけ再送し、成功したホストを KV に 7 日記憶します。
- **鍵**: リモートの公開鍵は `account_keys` に持ち、取得から 24 時間で古いとみなします。検証に失敗したら、`keyId` ごとに 5 分に 1 回まで取り直して 1 回だけ再検証します（ADR-0019 §5, §7）。
- **署名者と主体**: 鍵の持ち主（`owner`）がアクティビティの `actor` と一致し、`keyId` のホストが `actor` のホストと一致することを確かめます。一致しなければ、オブジェクトを `id` からオリジンに取り直します（§3.3）。
- **Object Integrity Proofs**（[FEP-8b32](https://codeberg.org/fediverse/fep/src/branch/main/fep/8b32/fep-8b32.md)、FR-410）: 配送する文書に、Actor の Ed25519 鍵で `eddsa-jcs-2022` の `proof` を付けます（§7.3）。鍵は Actor の文書の `assertionMethod` に `Multikey`（[FEP-521a](https://codeberg.org/fediverse/fep/src/branch/main/fep/521a/fep-521a.md)）で載せます（§10）。受信では、署名者が `actor` のアクティビティの `proof` を HTTP 署名に加えて検証します（§3.1 の手順 12）。HTTP 署名は省けず、署名者と `actor` が違うアクティビティは `proof` があっても取り直します（ADR-0032、ADR-0033）。
- **Linked Data Signatures**（`RsaSignature2017`、ADR-0061）: `public` の投稿の `Create` / `Update` / `Delete` の文書に、作者の RSA 鍵で LD 署名を付けて送ります（§7.3）。リレーが原本のまま中継した投稿を、Mastodon と Misskey が検証できるようにするためです。受信した LD 署名は検証しません（中継は取り直す。§13）。
- **インスタンス Actor**: オブジェクトの取得（GET）は、インスタンス Actor（`Application`）の鍵で署名します。相手が Authorized Fetch でも取得でき、特定の利用者の鍵を使わないので、相手のブロックに巻き込まれません。

### 2.1 実装

| 型（`internal/app/federation`） | 役割 |
| --- | --- |
| `Verifier` / `Signer`（ポート） | 署名の解析・署名ベースの組み立て・暗号の検証と署名だけを行う。実装は `internal/adapter/httpsig`、コントラクトテストは `testkit.HTTPSignatureContract`（RFC 9421 付録 B のテストベクタ、Mastodon・GoToSocial 形式の draft-cavage の署名、拒否の各条件、double-knocking） |
| `SignatureVerifier` | 受信のポリシー。`Parse`（署名なし `401`、解析不能 `400`）→ `Check`（必須の対象・時間窓・宣言されたアルゴリズム・ダイジェスト。I/O なし）→ `Verify`（鍵の解決と検証、鍵の取り直し）。拒否は `SignatureError`（`Status` と、`401` なら `Accept-Signature` の値） |
| `PublicKeyResolver`（ポート） | `keyId` から公開鍵と持ち主（`owner`）を返す。`refresh` で取り直す。取得できなければ `ErrPublicKeyNotFound`（`401`）、取得先が止まっていれば `ErrPublicKeyUnavailable`（`503`）。実装は `RemoteActors`（§6） |
| `SignedHTTPClient` | 送信の double-knocking。`kernel.HTTPClient` の上で署名し、結果の方式（`delivery_attempts.signature_scheme`）を返す |

- `@target-uri` と `(request-target)` は `NewSignedRequest` が設定の公開オリジンとリクエストのパス・クエリから組み立て、`Host` も公開オリジンのホストにします（別のホスト宛ての署名は検証に通らない）。
- RFC 9421 で `alg` が無い RSA の署名は、`rsa-v1_5-sha256`、`rsa-pss-sha512` の順に検証します（RFC 9421 付録 B の署名は `alg` を持たない）。
- 送信の RFC 9421 の `created` はシステムの時計から取ります（`dadrus/httpsig` が時計を差し替えられないため）。draft-cavage の `Date` は `kernel.Clock` から取ります。
- `code.superseriousbusiness.org/httpsig` は `algorithm="hs2019"` を固定で書くので、RSA の署名はアダプタが `rsa-sha256` に書き換えます（`algorithm` は署名の対象ではない）。
  このライブラリは、署名が `(created)` / `(expires)` を対象にするときだけ、システムの時計で 10 秒を超える未来の `created` と 10 秒を超えて過ぎた `expires` も拒否します（主要実装は `Date` に署名するので影響しない）。
- `Accept-Signature` を付けた `401` には、RFC 9421 で再送します。要求された対象をプロファイルの対象に加え、ラベル・`tag`・`nonce` も要求に合わせます。鍵とアルゴリズムは自分のものを使います。
- KV の名前空間: `federation.signature_scheme`（ホストごとの記憶。値は `rfc9421`、7 日）、`federation.key_refresh`（`keyId` ごとの取り直しの間隔、5 分）。記憶のあるホストで draft-cavage への再送が成功したら記憶を消します。
- ライブラリ（`dadrus/httpsig`、`code.superseriousbusiness.org/httpsig`、`dunglas/httpsfv`）は `internal/adapter/httpsig` の外から使えません（`.golangci.yml` の depguard）。

## 3. 受信（Inbox）

ADR-0033 の推奨案に従い、**署名の検証までを同期**、**処理を非同期** にします（NFR-PERF-4）。

### 3.1 同期の部分

`POST /ap/inbox`（shared inbox）と `POST /ap/actors/{uuid}/inbox`（有効なローカルの Actor でなければ 404）は、次の順に処理します。どこかで失敗したら、DB とキューに触れずに応答します。

連合の向き（[§14](#14-連合の向き)）が `off` のときは、署名の検証より前に、ドメインの `suspend` と同じ位置で `403`（理由 `federation off`）を返します。

| 順 | 処理 | 失敗時 |
| --- | --- | --- |
| 1 | `Content-Type` が `application/activity+json`、`application/ld+json`（AS2 のプロファイル付き）、`application/json` のどれか | `415` |
| 2 | 本文を 1 MiB まで読む（`http.MaxBytesReader`、ADR-0019 §5） | `413` |
| 3 | IP 単位のレート制限（ADR-0019 §5。1,000 / 5 分、IPv6 は /64 単位） | `429` + `Retry-After` |
| 4 | 署名ヘッダを解析し、`keyId` のホストを得る | `401`（署名なし）/ `400`（解析不能） |
| 5 | `keyId` のホストのドメイン単位の処分（`federation` の制限、§11） | `403` |
| 6 | ドメイン単位のレート制限（`keyId` のホスト。2,000 / 5 分） | `429` + `Retry-After` |
| 7 | ダイジェストの照合と、鍵の要らない検査（必須の対象・時間窓・宣言されたアルゴリズム。`SignatureVerifier.Check`） | `401` + `Accept-Signature` |
| 8 | 本文の JSON から `id`・`type`・`actor`・`object`（の `id`）だけを読む（`federation.ReadEnvelope`）。`type` と `https` の `actor` が必須で、IRI は 2,048 バイト、`type` は 255 バイトまで | `400` |
| 9 | 未知の Actor の自分自身の `Delete` / `Update`（`actor == object`、`accounts (uri)` に無い）なら、鍵を取りに行かずに捨てる（ADR-0019 §7） | `202` |
| 10 | 鍵の解決（未知・期限切れなら Actor を取得。5 秒で打ち切る） | `401`（取得できない）/ `503`（取得先がサーキットブレーカーで止まっている） |
| 11 | 署名の検証（ADR-0032。保存した鍵で検証できなければ鍵を取り直して 1 回だけ再検証する） | `401` + `Accept-Signature` |
| 12 | 署名者と `actor` の照合。一致しなければジョブに `trust = refetch` を付ける。一致すれば、本文の `proof`（FEP-8b32）を検証する（下記）。`refetch` のアクティビティと `Announce` は、署名者が参加中（`accepted`）のリレーの Actor ならジョブに `relay_id` を付ける（§13） | `401` + `Accept-Signature`（`proof` が検証できない） |
| 13 | 1 つの短いトランザクションで、`received_activities` に `ON CONFLICT DO NOTHING` で挿入し、行が増えたときだけ `received_activity_documents` と `federation.inbox` ジョブ（`queue.Outbox`）を書く | — |
| 14 | 署名者のドメインの `instance_health` を `available` に戻す（§7.3） | — |
| 15 | `202 Accepted` | — |

- 手順 12 の `proof` の検証（`SignatureVerifier.VerifyProof`）: 本文の最上位の `proof` が 1 つのオブジェクトで、`type` が `DataIntegrityProof`、`cryptosuite` が `eddsa-jcs-2022`、`proofPurpose` が `assertionMethod`、`expires` を過ぎていないときだけ検証します。`verificationMethod` の鍵は HTTP 署名と同じ `PublicKeyResolver` で引き、`assertionMethod` の `Multikey` から読んだ Ed25519 の鍵を使います。
  - `proof` が無い・条件に合わない・鍵を引けない・鍵が Ed25519 でないときは、`proof` を無視して HTTP 署名だけで受け付けます（FEP-8b32 の「検証できない証明は無視して、ほかの認証に頼る」）。
  - 鍵の持ち主が `actor` でない、または署名が合わないときは `401` で拒否します（FEP-8b32 は `proof` を HTTP 署名より優先する）。保存した鍵で合わなければ、HTTP 署名と同じく `keyId` ごとに 5 分に 1 回まで鍵を取り直して 1 回だけ再検証します。拒否の理由は `integrity proof does not verify` です。
  - 検証は `proof` を除き、FEP-8b32 のとおり LD 署名（`signature`）も除いた文書に対して行います。`proof` に `@context` があれば、文書の `@context` がそれで始まることを確かめ、文書の `@context` をそれに置き換えて正規化します（vc-di-eddsa §3.3.2）。
  - 署名者と `actor` が違う（`trust = refetch`）アクティビティの `proof` は検証しません。どのみちオブジェクトをオリジンから取り直すからです。`proof` が通ったときに取り直しや HTTP 署名を省くことはしません（省くなら ADR-0033 の改定が要る）。
- `id` の無いアクティビティは、`actor`・改行・受け取った本文をつないだ SHA-256 の 16 進表記から `urn:my:digest:<hex>` を合成して重複排除のキーにします（[data-model §2.2](data-model/02-consistency.md#22-冪等キーと一意制約)）。
- `id` のホストが `actor` のホストと異なるアクティビティと、鍵の持ち主（`owner`）が `keyId` のホストに無いアクティビティは、`trust = refetch` にします。
- 鍵の取得はトランザクションの外で行います（ADR-0003）。取得した Actor の書き込みは、受信の記録とは別の短いトランザクションです。
- 手順 1・2 と、個人の inbox の宛先の確認（404）は `internal/interface/activitypub` が、手順 3〜14 は `federation.Inbox.Receive` が行います。拒否は `Received.Rejection` の `InboxRejection`（`Status`、`429` の `Retry-After`、`401` の `Accept-Signature`）で、想定外の失敗（500）とは別の戻り値で返します。拒否のログには、ヘッダーの値を引用するエラーではなく、決まった語彙の理由（`InboxRejection.Reason`）だけを出します（ADR-0019）。レート制限のストアが使えないときは、制限を掛けずに通します。
- 受信の記録は `InboxRepository`（`received_activities` と `received_activity_documents`）に書きます。コントラクトテストは `testkit.InboxContract` です。
- 個人の inbox と shared inbox は同じ記録をします。誰宛てかはアクティビティの宛先が決めます。

### 3.2 非同期の部分

ジョブ `federation.inbox`（ペイロードは `activity_uri` と `trust`、リレーが運んだものは `relay_id`、冪等キーは `inbox:<activity_uri のハッシュ>`、`orderingKey` は `inbox:<actor の URI>`）が、原本を読んで処理します。

ハンドラへ進めるのは、連合の向きが通す型だけです。ほかは `dropped` にします（型と流れの対応は [§14.2](#142-入口ごとの振る舞い)）。

1. 原本を正規化する（§4）。正規化できなければ `Fail`（dead job に残る）。`relay_id` の付いたアクティビティは、何かを取得する前にリレーの関門を通す（§13）。
2. `trust = refetch` なら、オブジェクトを `id` からオリジンに取得し直し（§6）、取得した文書を使う。`Announce` 以外で取得できなければ捨てる。
3. 主体（`actor`）のアカウントを解決する（未知なら取得、§6）。主体やそのドメインに `federation` の制限があれば捨てる（`Delete` / `Undo` / `Reject` は通す。§11）。
4. 種類ごとのハンドラ（§3.4）を呼ぶ。ハンドラは 1 つの短いトランザクションで、各モジュールの公開 API を呼んで書く。
5. 結果: 書けたら `Done`、取得の一時的な失敗は `Retry`（ADR-0004 のバックオフ）、恒久的に処理できないものは `Fail`。

- 受信の記録は同期の部分で書くので、送信側が同じアクティビティを再送してもジョブは増えません。処理に失敗したアクティビティは dead job として残り、CLI（FR-801）から再実行します。
- `orderingKey` は同じ Actor のアクティビティを直列化しますが（#77）、正しさはそれに頼りません（§3.3）。
- 実装は `federation.InboxProcessor`（`internal/app/federation`）です。
  - 手順 2 の取り直しでは、埋め込みの文書をすべて信用しないものにします。`Create` / `Update` はオブジェクトを `id` から取得し、`attributedTo` が主体のものに差し替えます。`Delete` は対象がオリジンで消えている（404 / 410 / `Tombstone`）ときだけ通します。自分自身の `Update` はハンドラが Actor を取得し直します。それ以外はアクティビティを `id` から取得し、同じ `id`・型・主体のときにその文書を使います。
  - 主体はポート `InboxActors`（`RemoteActors.Resolve`）で解決し、ローカルの主体は捨てます。制限はポート `DomainPolicy`（主体のホスト）と `AccountPolicy`（主体のアカウント）で判定します（実装は `moderation.FederationPolicy`、§11）。
  - ハンドラは `Handle(型, ハンドラ)` で登録します。主体が自分自身を対象にする `Update` / `Delete` は `HandleOwnActor` で別に登録します（`RemoteActors.RegisterInboxHandlers` が登録する Actor の更新・削除）。登録の無い型は `info` のログを残して完了にします（未知の型・まだハンドラの無い型）。
  - `Undo` は取り消す活動の型ごとに `HandleUndo(型, ハンドラ)` で登録します（`Handle(Undo, …)` は使わない）。`object` に活動が埋め込まれていればその型のハンドラだけに、URI だけなら登録順にハンドラを呼び、`ErrUnknownObject` 以外を返したところで止めます。どれも対象を知らなければ tombstone です。埋め込みの型は振り分けにだけ使い、ハンドラは対象が主体のものであることを DB で確かめます。
  - ハンドラは、対象が無いことを `ErrUnknownObject`、捨てることを `ErrDropActivity`、処理できないことを `ErrUnprocessable`（`Fail`）で返します。取得の `ErrFetchNotFound` / `ErrFetchGone` / `ErrFetchRefused` は捨てる、`ErrFetchFailed` / `ErrFetchUnavailable` とその他のエラーは `Retry` です。
  - 記録や原本が無い・正規化できない・記録と主体が違うジョブは `Fail` です。
  - 結果は `InboxOutcome`（`handled` / `unhandled` / `tombstoned` / `dropped`）です。コントラクトテストは `testkit.InboxProcessingContract` です。

### 3.3 順序と冪等性

ActivityPub の配送は順序を保証しません。ハンドラは、どの順で届いても、何回処理しても同じ結果になるように書きます（NFR-REL-1）。

| 状況 | 振る舞い |
| --- | --- |
| 同じアクティビティの再処理 | 対象テーブルの一意制約（`statuses (uri)`、`boosts (uri)`、`follows (uri)`、`follow_resends (uri)`、`favourites (uri)`、`blocks (uri)`、`reports (uri)`、版の `(status_id, seq)`）で no-op |
| `Create` より先に `Delete` | `remote_object_tombstones` に URI を記録する。後から届いた `Create` / `Update` / 取得は、tombstone があれば捨てる |
| `Create` より先に `Update` | 取り込み範囲（ADR-0034）に入るなら `Create` として扱う。`published` が 1 日より古いものは取り込まない（Mastodon と同じ） |
| 古い `Update` が後から届く | `updated`（無ければ `published`）が現在版の `revised_at` 以下なら版を作らない |
| `Follow` より先に `Undo Follow` | 埋め込みの `Follow` があれば、主体とその `object` の組を `remote_follow_tombstones` に記録し、後から届いた同じ組の最初の `Follow` を捨てる（Misskey の `Undo` は送った `Follow` と違う `id` を持つ）。`object` が URI だけなら、その URI を tombstone に記録して同じ URI の `Follow` を捨てる |
| 承認済みのフォローに別の `id` の `Follow` | フォローの行は増やさず、その `id` を `follow_resends` に記録して `Accept` を送り直す（Mastodon は `Accept` が届くまで `Follow` を送り直す） |
| 対象が無い `Accept` / `Reject` / `Undo Like` / `Undo Announce` | 何もせず完了にする |
| `Like` / `Announce` の対象のローカルの投稿が無い・削除済み | 何もせず完了にする |
| 同じ投稿への同時の `Create` と `Update` | `statuses (uri)` と版の `(status_id, seq)` の一意制約で後の方が再実行され、既存の行を使う |

- tombstone は `origin = remote` の `status_deletions`、未知の URI のための `remote_object_tombstones`、未知の `Follow` の組のための `remote_follow_tombstones`（後の 2 つは 30 日で消す）の 3 つです。
- `remote_object_tombstones` は `InboxProcessor` が書きます。`Delete` / `Undo` のハンドラが無いか、ハンドラが `ErrUnknownObject` を返したとき、対象の URI が主体と同じオリジンで主体自身でなければ記録します（ほかのサーバーのオブジェクトを封じさせない。`federation.TombstoneFor`）。
  `Undo` が主体自身の `Follow` を埋め込んでいれば、URI の代わりに組を `remote_follow_tombstones` に記録します（`federation.FollowTombstoneFor`。主体自身の `Follow` しか封じない）。
- `Delete` / `Undo` 以外のアクティビティは、ハンドラを呼ぶ前に、アクティビティの URI に tombstone があれば捨てます。`Create` / `Update`（自分自身の Actor 以外）は、オブジェクトの URI の tombstone でも捨てます。
  `Follow` は、主体と `object` の組に使い終えていない `remote_follow_tombstones` があれば捨て、同じトランザクションで自分の URI を `Undo` の `activity_uri` で `remote_object_tombstones` に記録します。
  これで組の tombstone は使い終わり、同じ `Follow` の再処理は URI の tombstone で捨てられ、その次の `Follow` は通ります。
- アクティビティの `object` が URI だけのとき、受信した文書の中に埋め込まれていない対象は、ローカルの DB から引くか、取り込み範囲に入るときだけ取得します。
  `Create` / `Update` の投稿が URI だけなら、作者をローカルのアカウントがフォローしているときだけ取得します（投稿が無いと、メンションや返信先で範囲を判定できない）。

### 3.4 アクティビティごとのハンドラ

| アクティビティ | 対象 | 処理 |
| --- | --- | --- |
| `Follow` | インスタンス Actor | 参加中のリレーの Actor からのものは `Accept` を、それ以外は `Reject` をインスタンス Actor が送る。フォローとしては記録しない（§13） |
| `Follow` | ローカルの Actor | `graph.ReceiveFollow`。鍵アカウントでなければ承認し、`Accept` を配送する。鍵アカウントなら承認待ち + 通知。ブロック関係があれば `Reject` を配送する。承認済みのフォローがあれば `follow_resends` に記録して `Accept` を送り直す。移転したアカウントへの `Follow` は記録せず、`Reject`（`object_type = refusal`）を返す（ADR-0089） |
| `Accept` / `Reject` | リレーへの `Follow`（`object` の `id` がリレーの現在の `Follow`） | リレーの状態を `accepted`（主体を記録）/ `rejected` にする（§13）。リレーの Actor でない主体のものは捨てる |
| `Accept` | 自分が送った `Follow`（URI か埋め込み） | `graph.AcceptRemote`（`follow_acceptances`、`accepted_by = remote`）。`object` の `id` が主体をフォローされる側とするフォローに無ければ、埋め込みの `object.actor`（ローカル）と `actor`（相手）の承認待ち・承認済みのフォローで照合する。埋め込みが `Follow` 以外（GoToSocial の返信の承認など）なら捨てる |
| `Reject` | 自分が送った `Follow`、または承認済みのフォロー | `graph.RejectRemote`。照合は `Accept` と同じ。承認待ちなら `follow_rejections`（`rejected_by = remote`）、承認済みなら `follow_terminations`（`removed_by_followee`） |
| `Undo` | `Follow` / `Like` / `EmojiReact` / `Announce` / `Block` / `Accept` | 対象の取り消し。`Undo Follow` は `graph.TerminateRemote`（`unfollowed`）で、`object` の `id` が主体のフォローに無ければ、埋め込みの `object.object`（ローカル）とのフォローで照合する（Misskey の `Undo` は送った `Follow` と違う `id` を持つ）。`Undo Accept` は `graph.RejectRemote` でフォローの終了として扱う。`Undo Like` / `Undo EmojiReact` は、主体が作った同じ URI の `reactions`、`favourites` の順に引き、どちらにも無ければ埋め込みの `object` を `Like` / `EmojiReact` の行と同じ規則で振り分けて、リアクションなら主体のその投稿への同じ絵文字のリアクションを、お気に入りなら主体のその投稿へのお気に入りを消す（ADR-0052 §1）。対象が URI だけなら、主体が作った同じ URI の `follows` / `reactions` / `favourites` / `boosts` / `blocks` を順に引く |
| `Create` | `Note` / `Question`。`Article` / `Page` / `Event` / `Image` / `Video` / `Audio` は変換して取り込む | 取り込み範囲（ADR-0034）に入れば `content.IngestRemoteStatus`。オブジェクトの `id` は `actor` と同じオリジンで、`attributedTo` は（あれば）`actor` でなければならない。保存済みの投稿は何もせず、削除済みの投稿は捨てる |
| `Create` | `name` を持ち、投票のあるローカルの投稿への返信の `Note`（投票への回答。Mastodon・Misskey・Pleroma（Akkoma）・GoToSocial が送る形） | `content.RecordRemotePollVote`（FR-112）。`name` が選択肢の題で、投稿としては取り込まない。同じ `Note` は 1 票で、締め切り後・無い選択肢・単一選択の 2 つ目の回答・主体が見られない投稿への回答は捨てる。投票のない投稿への返信は投稿として扱う |
| `Update` | 投稿 | `content.ReviseRemoteStatus`（新しい版）。投稿の作者とアクティビティの主体が一致し、オリジンが同じであること（[ActivityPub §7.3](https://www.w3.org/TR/activitypub/#update-activity-inbox)）。未知の投稿は §3.3 のとおり `Create` として扱う |
| `Update` | Actor（自分自身） | `RemoteActors.ReceiveActorUpdate`。オリジンが同じ埋め込みの文書で `account.UpsertRemoteActor`（内容が変わればプロフィールの版が増える）。埋め込みが無い・`id` が主体と一致しなければ取得し直す。削除済みの Actor の `Update` は捨てる |
| `Delete` | 投稿・`Tombstone` | `content.DeleteRemoteStatus`（`origin = remote`）。未知の URI が、承認済みの引用の `QuoteAuthorization` なら、その引用を `content.RevokeQuoteAuthorization` で取り消す（ADR-0069 §2）。どちらでもなければ tombstone |
| `QuoteRequest` | ローカルの投稿（`object`）を引用する主体の投稿（`instrument`。主体と同じオリジン） | `content.AnswerQuoteRequest` が自動で答える（ADR-0069 §2）。主体がリモートで、投稿を `ContextObjectFetch` で閲覧でき、ブロック関係と交流の禁止（FR-708）がなく、投稿の引用の許可に当てはまる（`content.MayQuote`。`local` の投稿はリモートからは引用させない）なら、`quote_authorizations` に承認を記録して、`result` にその `QuoteAuthorization` を付けた `Accept` を返す。同じ引用の 2 度目の依頼には同じ承認を返す。それ以外は `Reject`。どちらも依頼を埋め込み、主体の inbox だけに送る。引用元の投稿が保存済みなら、その引用を承認する |
| `Accept` / `Reject` | このサーバーが送った `QuoteRequest`（`id` が `{引用元の IRI}/quote_request`） | `QuoteHandlers.ReceiveQuoteAnswer`。主体は引用先の作者でなければならない。`Accept` は `result` の `QuoteAuthorization` を取得し、§6.3 の引用先の検証と同じ規則で確かめてから `content.AcceptQuote` で承認する（承認は §7.1 の `Update` で伝わる）。`Reject` は `content.RejectQuote` で拒否にする。フォローへの答えより先に照合する |
| `Delete` | Actor（自分自身） | `RemoteActors.ReceiveActorDelete`。`account.MarkRemoteDeleted`（`remote_deleted`）で、プロフィールと投稿（タイムラインでは `feed_author_restrictions` の `profile_view`）がすぐに表示されなくなる。未知の Actor なら何もしない。投稿・フォロー・ブーストは消去ジョブが消す |
| `Announce` | 投稿（主体が参加中のリレーの Actor で `relay_id` の付いたもの） | 投稿をオリジンから取得し、`public` なら取り込んで `relayed_statuses` に記録する。ブーストとしては記録しない（§13） |
| `Announce` | 投稿（URI か埋め込み） | `content.RecordRemoteBoost`。ローカルの投稿と保存済みの投稿は `id` で引く。保存していないリモートの投稿は、ブーストした人をローカルのアカウントがフォローしているときだけ取り込み（ADR-0034）、埋め込みの投稿は、オリジンが主体と同じでなければ `id` から取得し直す。ブーストの公開範囲は、`Announce` の宛先から決めた公開範囲と元投稿の公開範囲の狭いほう（`direct` になるものは捨てる）。`local` の投稿、ほかの人の `private` の投稿、主体が閲覧できないローカルの投稿は捨てる |
| `EmojiReact` | ローカルの投稿 | `content.RecordRemoteReaction`（絵文字リアクション、FR-116、ADR-0052）。`content` が絵文字でなければ捨てる。絵文字は、1 つの Unicode 絵文字（`emoji-test.txt` の表で fully-qualified の形に正規化する）か、`:shortcode:` で `tag` に `name` が一致する（コロンと `@host` の接尾辞を問わない）`Emoji` がちょうど 1 つあるカスタム絵文字。カスタム絵文字は ADR-0051 の規則で `custom_emojis` に画像と取り込み、ドメインは `Emoji` の `id` のホスト（`id` が無ければ主体のドメイン）。同じ主体・投稿・絵文字のリアクションは 1 つで、絵文字ごとに複数を記録する。主体が閲覧できない投稿（`ContextObjectFetch`）・ブロック関係・交流の禁止（FR-708）は捨てる |
| `Like` | ローカルの投稿 | `_misskey_reaction`、無ければ `content` が `EmojiReact` と同じ規則で絵文字なら `content.RecordRemoteReaction`（Misskey のリアクション、FEP-c0e0）。それ以外は `content.RecordRemoteFavourite`（お気に入り）。主体が閲覧できない投稿（`ContextObjectFetch`）は捨てる |
| `Block` | ローカルの Actor | `graph.ReceiveBlock`。両方向のフォローを終了し、相手の投稿を閲覧者から隠す（`feed_exclusions`）。対象がローカルの利用者の Actor でなければ捨てる |
| `Flag` | ローカルの Actor / 投稿 | `moderation.FileRemoteReport`（`reports.uri`）。送信者のドメインに `report_intake` の制限があれば捨てる。通報の対象は `object` の最初のローカルの Actor、無ければ最初のローカルの投稿の作者で、その作者の投稿を `report_statuses` に残す。ローカルの対象が無ければ捨てる |
| `Add` / `Remove` | 主体の `featured` | `StatusHandlers.FeaturedChanged`。`target` が、保存した主体の Actor の文書が宣言する `featured` と一致し、主体と同じオリジンなら、ジョブ `federation.fetch_featured` を投入してピン留めの一覧を取得し直す（§6.2）。`object` は読まない。署名者が主体であることは §3.2 で確かめてあるので、主体がコレクションの持ち主になる。ほかの `target`（主体のほかのコレクション・別のオリジン・`target` なし）と、ほかの人の `featured` への `Add` / `Remove` は捨てる |
| `Move` | 主体自身（`object` が `actor`。FEP-7628 の push mode） | `federation.RegisterMoveHandler`（ADR-0089）。次をすべて満たすときだけ処理し、満たさなければ理由を `info` のログとメトリクス `my.move.activities`（`my.outcome`）に残して完了にする: `object` が主体である（`not_own_actor`。移転先が送る pull mode は受けない）、`target` があって主体と異なる（`same_account`）、`target` をキャッシュを使わずに取り直せて（`RemoteActors.RefreshActor`。§6.2 と同じく保存する）その `id` が `target` である（`target_unavailable`）、その `alsoKnownAs` に主体がある（`alias_missing`。ローカルの `target` はポート `LocalAliases` の別名で確かめ、ポートの無いいまはだれも受け入れない）、`target` が `movedTo` を持たず、このサーバーでも移転済みでない（`target_moved`。連鎖をたどらない）、主体・`target` のアカウントにもドメインにも `suspend` の処分が無い（`suspended`）。通ったら `account.RecordRemoteMove` が `account_moves`（`initiated_by = remote`、`activity_uri` は `Move`）と `account_move_states` に記録する（`recorded`）。同じ移転先への `Move` は何も増やさず（`duplicate`）、別の移転先への `Move` は前の移転から 7 日以内なら捨てる（`too_soon`）。`account.moved` を購読する `graph` がジョブ `graph.migrate_follows` でローカルの関係を移す（下） |
| 未知の型 | — | ログとメトリクスに残して完了にする |

- `Move` の移し替え（`graph.migrate_follows`、冪等キーは移転元と移転先の組）は、移転元をフォロー・ブロック・ミュートしているローカルの利用者を 100 件ずつのページでたどり、ページの間を 10 秒あけます（`graph.MigratePageInterval`）。利用者ごとに 1 つの短いトランザクションです。
  - 移転元を承認済みでフォローしている利用者が、移転先をブロックしておらず、移転先からブロックされておらず、移転先のドメインをミュートしていなければ、移転先へフォローし（`show_boosts`・`notify` を引き継ぐ。移転先がリモートか鍵アカウントなら承認待ちで、`Follow` を送る）、移転元へのフォローを `follow_terminations`（`reason = moved`）で終えて `Undo Follow` を送ります。そうでなければ移転元へのフォローを残します（Mastodon と同じ）。どちらでも `graph.follow_migrated` が通知 `account_moved`（移転元から。移転先をフォローしたか、しなかった理由）を作ります。
  - 移転元をブロックしている利用者は移転先もブロックし（`Block` を送る）、移転元をミュートしている利用者は、移転先をまだミュートしていなければ、同じ通知の扱いと期限で移転先をミュートします。
  - ジョブをもう一度動かしても、移ったフォロワーは移転元をフォローしておらず、ブロック・ミュートは既にあり、通知は移転ごとに 1 つ（重複排除キー）なので、何も増えません。
- 移転元にこのサーバーの `limit` があれば（`moderation.FederationPolicy.AccountLimited`）、移転先へは引き継がず、`account.moved` から `manage_accounts` を持つモデレーターに通知 `admin_account_moved` を出します（§11）。
- ブースト・お気に入り・絵文字リアクションのハンドラは `federation.RegisterInteractionHandlers` です。`Announce` の対象の取り込みは投稿のハンドラ（`StatusHandlers`）と同じ規則で行います。コントラクトテストは `testkit.RemoteInteractionContract` で、主要実装のフィクスチャの `Announce` / `Like` / `EmojiReact` / `Undo` を受け取り、お気に入りと絵文字リアクションの `Like` / `Undo Like` を送ります（`Like` の枠の付け替えでは、外したものの `Undo` が新しい `Like` の前に記録され、同じ inbox へその順に届く。§7.2 の順序キー）。
- リアクションの送信（ローカルの人のリアクション）はまだありません。リモートの人のリアクションはローカルの投稿へのものだけを記録し、何も送りません。

## 4. AS2 の正規化

受信した文書は JSON-LD として処理せず、AS2 の正規の `@context` で圧縮された JSON として読みます。AS2 は消費者に JSON-LD の処理を求めていません（[AS2 Core §2](https://www.w3.org/TR/activitystreams-core/#jsonld)）。
`internal/domain/federation` の正規化関数が、主要実装の方言の差をここで吸収し、以降のコードは正規化した型だけを扱います。

| 項目 | 規則 |
| --- | --- |
| `@context` | 値は解釈しない。トップレベルの文書には `https://www.w3.org/ns/activitystreams` を含むことを求める（無ければ正規の文脈を仮定する。AS2 Core §2.1）。`signature`（LD 署名）は検証せず、読み捨てる |
| 単数と配列 | 配列になり得るプロパティは、単数・配列・`null` のどれでも配列として読む（`type` も配列を許し、知っている型の最初のものを使う） |
| リンクとオブジェクト | `object`・`inReplyTo`・`attributedTo`・`actor` などは、文字列（IRI）・`Link`（`href`）・埋め込みのオブジェクト（`id`）のどれでも IRI を取り出す。埋め込みの中身を使うのはオリジンが同じときだけ |
| 公開の宛先 | `https://www.w3.org/ns/activitystreams#Public`、`as:Public`、`Public` を同じ値として扱う（[ActivityPub §5.6](https://www.w3.org/TR/activitypub/#public-addressing)） |
| 公開範囲 | `to` に公開 → `public`、`cc` に公開 → `unlisted`、作者の `followers` が `to` / `cc` に → `private`、それ以外 → `direct`。リモートの投稿が `local` になることはない |
| 本文 | `contentMap` の言語ごとに 1 行（`status_revision_bodies`）。`content` だけなら `contentMap` の最初の言語、なければ `und` の 1 行。言語タグは BCP 47 として正規化し、不正なら `und`（NFR-I18N-5） |
| CW | `summary`（無ければ `summaryMap` の最初の値）をプレーンテキストにして `spoiler_text`。`sensitive` は `toot:sensitive` |
| 変換する型 | `Article` / `Page` / `Event` / `Image` / `Video` / `Audio` は、`name` を太字の段落、`summary` を段落にし、`url` へのリンクを付けた本文にする（Mastodon の `CONVERTED_TYPES` と同じ） |
| `tag` | `Mention`（`href` を Actor の IRI として解決）、`Hashtag`（`name` から `#` を除いて `tags`）、`Emoji`（`icon.url`、カスタム絵文字。`name` の前後の `:` を外した shortcode が 128 文字を超えるものと、同じ shortcode の 2 つ目以降は捨てる）。Actor の `tag` の `Emoji` も同じ規則で読み、プロフィールの版に結ぶ |
| 宛先のメンション | `to` / `cc` に含まれるローカルの Actor は、`tag` に無くてもメンションとして記録する（`direct` / `private` の閲覧判定のため） |
| `attachment` | `Document` / `Image` / `Video` / `Audio`。`url`（文字列か `Link`）、`mediaType`、`name`（代替テキスト）、`blurhash`、`focalPoint`、`width` / `height`。`href` だけの `Link` は添付として扱わない。4 件を超える分は捨てる |
| `url` | 文字列・`Link`・配列のどれでもよい。`text/html` の最初のものを表示用 URL にする |
| 時刻 | `published` / `updated` は RFC 3339 として読み、未来の時刻は現在時刻に丸める。投稿の `id`（UUIDv7）は `published` から作る（#71） |
| 投票 | `Question` の `oneOf` / `anyOf`（各 `name` と `replies.totalItems`）、`endTime`、`votersCount`（FR-112）。変換しない投稿の `name` は、投票への回答の選択肢の題として読む（§3.4） |
| 返信の一覧 | 投稿の `replies` はコレクションへの参照（IRI か埋め込み）として読む。`Collection` / `OrderedCollection` / `CollectionPage` / `OrderedCollectionPage` は、`items`（順序付きなら `orderedItems`）を 100 件まで、`first` と `next` をページへの参照として読む（Mastodon は最初のページを埋め込む） |
| 方言 | Misskey の `_misskey_content`（MFM の原文）・`_misskey_summary` は使わず、`content` / `summary` を使う。引用（FR-114、[ADR-0069](../adr/0069-quote-posts.md) §2）は、FEP-044f の `quote`・`_misskey_quote`・`quoteUrl`・`quoteURL`・`quoteUri`・`rel` が `_misskey_quote` の IRI の FEP-e232 の `Link`（`mediaType` が AS2 の JSON-LD）の順に最初の 1 つを読む。`quote` なら形式は `fep_044f` で `quoteAuthorization` も読み、それ以外は `legacy`。`QuoteAuthorization` の文書は `attributedTo`・`interactingObject`・`interactionTarget` を読む。本文の `quote-inline` のフォールバックはそのまま残す。投稿の `interactionPolicy.canQuote` は `automaticApproval` と `manualApproval` を合わせて、`Public` があれば `public`、作者の followers があれば `followers`、それ以外は `nobody` として読み、無ければ方針なしとする。`isCat` などの未知のプロパティは無視する |
| 未知のプロパティ | 無視する（[AS2 Core §5](https://www.w3.org/TR/activitystreams-core/#extensibility)）。原本は `received_activity_documents` に短期間残る |
| 文字列 | 文書を読むとき、すべての文字列（キーを含む）のヌル文字（`\u0000`）を `U+FFFD` に置き換える。不正な UTF-8 と対になっていないサロゲートも JSON の復号で `U+FFFD` になるので、以降の値はどれもそのまま両方の方言の文字列の列に書ける（[data-model §1.2](data-model/01-conventions.md#12-論理型)）。取り除かずに置き換えるのは、前後の文字列がつながって別の値（`java\u0000script:` → `javascript:`）にならないようにするため。IRI では `%EF%BF%BD` になる |
| 上限 | 文書のネストの深さ 16、`tag` 100 件、`to` / `cc` 合わせて 1,000 件、本文 HTML 100 KiB。超えたら、その文書を正規化できないものとして扱う |

## 5. リモート HTML のサニタイズ

- リモートの投稿本文（`content` / `contentMap`）・プロフィールの `summary`・補足情報（`attachment` の `PropertyValue` の `value`）は、**取り込み時に** サーバー側のサニタイザ（`bluemonday`、ADR-0019 §2）を通し、サニタイズ済みの HTML を保存します。
  サニタイズ済みの HTML には、使ったポリシーの版（`html_policy_version`）を記録し、ポリシーを変えたら消去と同じく一定件数ずつ再サニタイズします。
- ポリシーは Mastodon の `MASTODON_STRICT` に合わせた許可リストです。
  - 要素: `p` `br` `span` `a` `del` `s` `pre` `blockquote` `code` `b` `strong` `u` `i` `em` `ul` `ol` `li` `ruby` `rt` `rp`
  - 属性: すべての要素の `lang`、`a[href rel class translate]`、`span[class translate]`、`p[class]`、`ol[start reversed]`、`li[value]`
  - `class` は microformats（`h-` `p-` `u-` `dt-` `e-` で始まるもの）と `mention` `hashtag` `ellipsis` `invisible` `quote-inline` だけを残す。`translate` は `no` だけを残す
  - `href` のスキームは `http` `https` `dat` `dweb` `ipfs` `ipns` `ssb` `gopher` `xmpp` `magnet` `gemini`。それ以外の `a` は中のテキストだけを残す
  - `a` には `rel="nofollow noopener noreferrer"` と `target="_blank"` を付ける
  - `h1`〜`h6` は `<p><strong>…</strong></p>` に、許可外のブロック要素は `p` に変える。`<math>` は TeX の注釈があればそのテキストにする
- ローカルの投稿の HTML は、サーバーがプレーンテキストから生成します（`internal/app/content` の `text.go`）。生成する要素と属性はこの許可リストの部分集合です。
- **Web クライアントの Trusted Types ポリシー `my-server-sanitized-html`（`@my/ui` の `PostContent`）に渡る HTML は、ローカルで生成したものか、このサニタイザを通したものだけです**（[web-client/README.md](web-client/README.md)）。
  クライアント API は、本文・プロフィール・補足情報の HTML を保存したサニタイズ済みの値からだけ返し、受信した文書の値をそのまま返す経路を持ちません。表示名・ユーザー名・投票の選択肢・代替テキストはプレーンテキストとして返します（タグを除く）。
- カスタム絵文字の `:shortcode:` は HTML に `img` として埋め込まず、クライアントが絵文字の一覧から DOM 要素を差し込みます（ADR-0019 §2）。

## 6. リモートの Actor / オブジェクトの取得

### 6.1 取得の規則

- 取得するかどうかは連合の向きで決まります（[§14](#14-連合の向き)）。`off` は何も取得せず、`outbound` は署名の検証と配送の宛先に要る Actor だけを取得し、投稿・スレッド・outbox は取得しません。メディアプロキシは、`outbound` ではローカルのアカウントをフォローしているリモートのアカウントの画像だけを取得し、`off` では取得しません。

- すべて `HTTPClient` を通し、インスタンス Actor の鍵で署名した GET で取得します（§2）。`Accept: application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`。
- 応答の `Content-Type` が AS2 でなければ採用しません。本文は 1 MiB まで（ADR-0019 §1）。
- **同じオリジン**: 最後のリダイレクト先のオリジンと、文書の `id` のオリジンが一致しなければ採用しません。要求した URL と `id` が異なるときは、`id` を 1 回だけ取得し直して一致を確かめます（[ActivityPub 付録 B.2](https://www.w3.org/TR/activitypub/#security-verification)、FEP-fe34）。
- 投稿は、`attributedTo` のオリジンが投稿の `id` と同じでなければ採用しません。
- 再帰的な取得（返信先・埋め込み・`Announce` の対象）は、1 回の処理につき深さと件数に上限を置きます（[ActivityPub 付録 B.5](https://www.w3.org/TR/activitypub/#security-recursive-objects)、ADR-0034）。
- 404 / 410 と取得の失敗は、KV に 1 時間記録して同じ URI の取得を繰り返しません（名前空間 `federation.fetch_failure`、キーは URI の SHA-256）。Actor の 410 は `remote_deleted` にし、ほかの失敗は `remote_account_heads.consecutive_fetch_failures` に数えます。
- ドメイン単位の `federation` の制限があるドメインと、自サーバーの URI からは取得しません。`instance_health` が `unavailable` のドメインからも取得しません（鍵の解決は `503`）。
- 実装は `internal/app/federation` の `Fetcher`（取得の規則）と `RemoteActors`（Actor の解決・保存・ジョブ、`PublicKeyResolver`）です。コントラクトテストは `testkit.RemoteActorContract` です。

### 6.2 Actor とハンドル

- Actor の文書から `account.UpsertRemoteActor` が `remote_account_snapshots`（内容が変わったときだけ）、プロフィールの版（サニタイズした内容が変わったときだけ）、公開鍵を書きます。アバターとヘッダーは `content.MediaService.RecordRemoteMedia` で記録します。
- 鍵は、`publicKey` の PEM（PKIX と PKCS #1）、`publicKey` が `id` だけで示す鍵の文書（取得する。2 件まで）、FEP-521a の `assertionMethod` の `Multikey`（Ed25519）から読みます。鍵の `id` は Actor と同じオリジンで、`owner` / `controller` は Actor でなければなりません。
- `keyId` を取得して `Key` の文書が返ったら、`owner`（または `controller`）の Actor を取得します（ADR-0032）。
- 初めて文書を保存したドメインは `remote_instances` に記録し、ジョブ `federation.observe_instance` が NodeInfo 2.x（`/.well-known/nodeinfo` が指す同じホストの文書）のソフトウェア名と版を `instance_nodeinfo_observations` に記録します（前回と同じなら記録しない）。
- ハンドル（`account_handles`）は WebFinger で確かめたものだけです。`acct:{preferredUsername}@{Actor のホスト}` を引き、`self` リンクが Actor の `id` を指すこと、`subject` のドメインが異なるならそのドメインでも同じことを確かめます（ADR-0018 のドメイン分離に相当する構成）。
  - 答え（404 を含む）が確かめなければ `mismatch` を記録し、ハンドルを持たないアカウントとして扱います。
  - WebFinger が答えなかった（接続の失敗・429・5xx）ときは何も記録せず、それまでのハンドルを残します。
- `@user@host` / URL での解決（FR-305）は、`RemoteActors.ResolveHandle` と `ResolveURL` が行います。
  - ハンドルは、保存済みの `account_handles` にあればそれを返します。なければ `https://{domain}` の WebFinger で `acct:{user}@{domain}` を引き、`self` リンクの Actor を `Resolve`（保存済みなら取得しない）で取得・保存します。WebFinger にも §6.1 の取得先の規則（自サーバー・`federation` の制限・`unavailable` のドメインには送らない）が掛かります。
  - URL は、Actor の `id` か Actor を返す URL（プロフィールのページなど）として `Resolve` します。
  - どちらも、ハンドルを WebFinger で確かめた Actor だけを返し、それ以外（WebFinger が Actor を示さない・取得できない・ハンドルが `mismatch`）は `ErrUnresolvable` です。
  - ハンドルの解決は、返す Actor のハンドルが求めたハンドル（ユーザー名は大文字小文字を区別しない、ホストは正規化）と一致することも確かめます。
  - ローカル投稿のメンション（data-model §4.4）は `ResolveHandle` を、トランザクションの外で、件数と時間に上限を置いて呼びます。
  - クライアント API は `GET /api/v1/accounts/lookup` の `resolve=true` で呼びます（トークン必須、アカウントごとに 30 回 / 5 分）。
- 検索の解決は `federation.Resolver.Resolve` です。ハンドル（`@user@host` / `user@host`）は `ResolveHandle` に渡します。
  URL は、正規化した IRI で保存済みのアカウント・投稿を引き、無ければ文書を 1 回だけ取得して、Actor なら保存してから `ResolveURL` と同じくハンドルを確かめ、投稿なら §6.3 の取得した投稿として取り込みます（ADR-0034 の「URL の検索で解決した」）。投稿の返信先は受信時と同じく 1 段だけ取得します。
  自サーバーの URL は取得せず、保存済みのものだけを返します。それ以外（コレクションなど）と取得できないものは `ErrUnresolvable` です。
- ピン留め（`featured`、[ADR-0044](../adr/0044-remote-profile-recent-posts.md)）: Actor の文書を保存するとき（取得・取り直し・Actor 自身の `Update`）、文書が Actor と同じオリジンの `featured` を宣言していれば、保存のトランザクションでジョブ `federation.fetch_featured`（ペイロードは `account_id`、冪等キー `fetch_featured:<account_id>`）を `queue.Outbox` に投入します。
  - Actor が自分の `featured` への `Add` / `Remove`（Mastodon・Misskey などがピン留めの変化で送る）を送ってきたときも、同じジョブを同じ冪等キーで投入します（§3.4）。冪等キーにより、そのアカウントのジョブが完了するまで次のジョブは投入されず、続けて届いた通知の取得は重なりません。
  - ジョブ（`StatusHandlers`）は、実行時にもアカウントの条件（有効なリモート・`federation` の制限なし・同じオリジンの `featured`）を確かめ直し、コレクションの項目か、無ければ `first` のページ（同じオリジンのものだけ。2 ページ目はたどらない）を読みます。
  - 項目の順に、埋め込みなら `attributedTo` がそのアカウントで宛先に `as:Public` を含む投稿、IRI ならアカウントと同じオリジンの投稿を 5 件（`FeaturedMaxPosts`）まで選び、保存済みでなければ §6.3 の outbox と同じく取得した投稿として取り込みます（返信先は取得しない）。1 回の文書はコレクション・ページ・投稿の合わせて 7 件（`FeaturedMaxDocuments`）までです。
  - 選んだ投稿で `content.Interactions.ReplaceRemotePins` がそのアカウントの `status_pins` を置き換えます（コレクションの順、新しいピン留めが先）。空のコレクションと `featured` を宣言しなくなった文書はピン留めを消し、コレクションが取得できなければピン留めを残してジョブを完了にします。ストアの失敗は `Retry` です。
  - `GET /api/v1/accounts/{id}/statuses?pinned=true` はローカルと同じく `status_pins` を返すので、リモートのアカウントのプロフィールの先頭にも固定した投稿が出ます。コントラクトテストは `testkit.RemoteStatusContract` で、Mastodon・Misskey・GoToSocial の `featured.json` を取り込みます。
- 取得から 24 時間を過ぎた Actor は、次に使われたとき（署名の検証・表示・配送）に非同期で取り直します（ジョブ `federation.refresh_actor`、冪等キー `refresh_actor:<account_id>`）。ローカルのアカウントがフォローしている Actor は、定期ジョブ `federation.refresh_actors`（1 時間ごと）が `remote_account_heads (last_fetched_at)` の古い順に 200 件ずつ読み、フォローされているものを取り直します（ADR-0034）。カーソル（`last_fetched_at` と `account_id`）を持った次のジョブが続きを読みます。

### 6.3 投稿

- 取り込み範囲は ADR-0034 に従います（ローカルのアカウントとの関係がある投稿と、参加中のリレーが運んだ `public` の投稿。ADR-0061）。判定は `federation.StatusHandlers`（`internal/app/federation`）が、受信ごとに次の順で行います。
  1. 保存済みのアカウントのうち、投稿がメンションする・宛先に含めるローカルのアカウント（`active` なもの）がある
  2. 作者に承認済みのフォロワーがいる（リモートの Actor のフォロワーはローカルのアカウントだけ。`graph` の `ActiveFollowerIDs`）
  3. 返信先がローカルの投稿か、返信先の `conversation_uri` に削除されていないローカルの投稿がある
  4. 参加中のリレーが運んだ（`relay_id` の付いた）`public` の投稿（§13）
  - どれにも当てはまらない `Create` は `ErrDropActivity` で捨て、受信の記録だけが残ります（`info` のログ）。未知のリモートの Actor へのメンションは、範囲に入ると決まってから取得します。
- 書き込む行とメンション・ハッシュタグ・カスタム絵文字・添付・投票の扱いは [data-model §4.4 のリモート投稿のコマンド](data-model/04-tables/04-content.md#リモート投稿のコマンド) にあります。コントラクトテストは `testkit.RemoteStatusContract` で、主要実装のフィクスチャの `Note` を取り込みます。
- 返信先は受信時に 1 段だけ非同期（`federation.fetch_object`、ペイロードに残りの深さ）で取得し、スレッドの祖先と返信の一覧は利用者がスレッドを開いたときに深さ 20・件数 100 まで取得します。
  - **取得した投稿**: 取得した文書が投稿で、`attributedTo` があり、そのオリジンが投稿の `id` と同じで、作者を解決でき（未知なら取得）、作者が有効なリモートのアカウントで、URI に削除（`status_deletions` の tombstone）も `remote_object_tombstones` も無いときだけ、`content.IngestRemoteStatus`（`activity_uri` は NULL）で取り込みます。取り込み範囲の判定はしません（受信した投稿の返信先・利用者が開いたスレッド・検索で解決した投稿・フォローされている人がブーストした投稿は ADR-0034 の範囲、利用者が開いたプロフィールの outbox の最初のページの公開投稿は ADR-0044 の範囲です）。`Announce` の対象・`fetch_object`・`fetch_outbox`・`Resolver` は、この取り込み（`StatusHandlers.fetched`）を共有します。`Announce` の対象と検索で解決した投稿は、受信した投稿と同じく返信先を 1 段だけ取得します。
  - **受信時**: 取り込んだ返信の親が未知（`conversation_uri` が NULL）でリモートなら、取り込みのトランザクションで `federation.fetch_object`（祖先 0）を `queue.Outbox` に投入します。取得した親の親は取得しません。
  - **引用先**（ADR-0069 §2）: 引用のある投稿を取り込む・編集を受けるトランザクションで、`federation.verify_quote` を `queue.Outbox` に投入します（ペイロードは投稿の `uri`、`quoteAuthorization` の `authorization`、引用先を取得するかの `fetch`。冪等キーは `verify_quote:<ペイロードの SHA-256>`）。
    `fetch` は inbox で受けた投稿だけが真で、取得した投稿（返信先・スレッド・リレー・outbox）の引用先は、保存済みでなければ取得しません（引用は 1 段）。ジョブは次を行います。
    1. 引用先が未保存のリモートの投稿で `fetch` が真なら、§6.3 の取得した投稿として取り込む。
    2. `fep_044f` の引用で、引用先がリモートの投稿なら承認を確かめる。引用元と引用先の作者が同じなら、`QuoteAuthorization` なしで承認する。
       そうでなければ `quoteAuthorization` を取得し、`QuoteAuthorization` で、IRI のホストと `attributedTo` が引用先の作者のもの、`interactingObject` が引用元、`interactionTarget` が引用先のときだけ `content.AcceptQuote` で承認する（Mastodon の `VerifyQuoteService` と同じ）。
    3. 検証できない・取得できない承認は `info` のログを残してジョブを完了にし、引用は `pending` のまま残ります。`quoteAuthorization` を足す `Update` を受けると、新しいペイロードでもう一度確かめます。
    ローカルの投稿への `fep_044f` の引用は、`quoteAuthorization` がこのサーバーの `quote_authorizations` の、取り消されていない承認で、引用元と引用先が一致するときだけ承認します（`content.VerifyLocalQuoteAuthorization`）。
  - **スレッドを開いたとき**: `GET /api/v1/statuses/{id}/context` が、閲覧できた投稿について `StatusHandlers.FetchThread` を呼び、応答は待たずに保存済みのものだけを返します（取得したものは次に開いたときに出る）。返信先の無いローカルの投稿は何もしません（返信は配送で届く）。
    同じ投稿は 1 時間に 1 回だけです（KV の名前空間 `federation.thread_fetch`、キーは URI の SHA-256、`IncrBy` で最初の 1 回だけ投入する）。
  - `federation.fetch_object` のペイロードは `uri`、スレッドの起点の `thread`（受信時は空）、残りの祖先の深さ `ancestors`、残りの返信の深さ `replies`、祖先の取得の印 `up` で、冪等キーは `fetch_object:<ペイロードの SHA-256>` です。1 つのジョブは 1 つの投稿を扱います。
    1. 未保存なら取得して取り込む。保存済みのリモートの投稿で `replies` が残っていれば、返信の一覧を読むために文書を取得する。
    2. `ancestors` が残っていれば、保存済みの祖先をたどり、最初の未知のリモートの祖先の取得を、残りの深さを 1 減らして投入する。
    3. `replies` が残っていれば、`replies` のコレクション（投稿と同じオリジンのものだけ）とそのページを、埋め込みならそのまま、そうでなければ取得して読み、リモートの返信ごとに、深さを 1 減らした取得を投入する（最後の段で保存済みの返信は投入しない）。
  - **件数の上限**: スレッドの取得が取得する文書（投稿・コレクション・ページ）は、起点ごとの KV のカウンタ（名前空間 `federation.thread_budget`、1 時間）で数えて合わせて 100 件までです。祖先は深さ 20 までなので 20 件を超えず、返信の側（起点の文書・コレクション・ページ・返信）は 100 − 20 = 80 件で止めて祖先の分を残します。上限を超えた取得はカウンタを戻して何もしません。
  - 取得できない・取り込まない文書は `info` のログを残してジョブを完了にします。取得の失敗は `Fetcher` が 1 時間記憶するので（§6.1）、同じ URI を別のジョブが取り直しません。ストアの失敗は `Retry` です。
- リモートのアカウントのプロフィールを開いたときは、outbox の最初のページから最近の公開投稿を非同期で取り込みます（[ADR-0044](../adr/0044-remote-profile-recent-posts.md)）。
  - **始める条件**: `GET /api/v1/accounts/{id}/statuses` の最初のページ（`cursor` も `pinned` も無い）が `StatusHandlers.FetchOutbox` を呼び、応答は待たずに保存済みの投稿だけを返します。
    閲覧者がログインしていて、アカウントが有効なリモートのアカウント（`remote_deleted` ではない）で、最新の文書の `outbox` が Actor と同じオリジンにあり、アカウントの `federation` の制限が無く、閲覧者とアカウントの間にどちらの向きのブロックも無いときだけ、次の 2 つの KV のカウンタを `IncrBy` で数えます。
    アカウントごと（名前空間 `federation.outbox_fetch`、キーは Actor の URI の SHA-256、24 時間）は最初の 1 回だけ、閲覧者ごと（名前空間 `federation.outbox_fetch_user`、キーは閲覧者のアカウント ID、5 分）は 30 アカウントまでです。
    閲覧者の上限を超えたら、両方のカウンタを戻して投入しません（そのアカウントはほかの閲覧者が始められる）。投入できたら、応答の `remote_fetch_started` を `true` にします（Web クライアントは数秒後に最初のページを 1 回だけ読み直す）。
    ドメインの `federation` の制限・`unavailable` のドメイン・自サーバーの URI は `Fetcher` が取得しません（§6.1）。ドメインの `public_discovery` の制限は取得を止めません（§11）。
  - **ジョブ** `federation.fetch_outbox`: ペイロードは `account_id`、冪等キーは `fetch_outbox:<account_id>:<投入した日（UTC）>` です。実行時にもアカウントの条件（有効なリモート・`federation` の制限なし・同じオリジンの outbox）を確かめ直します。
    1. outbox を取得し、項目があればそれを、無ければ `first`（埋め込みならそのまま、IRI なら取得）のページを読みます。どちらも Actor と同じオリジンのものだけで、2 ページ目（`next`）はたどりません。
    2. ページの項目のうち、埋め込まれた `Create` で、`actor` がそのアカウント、オブジェクトが埋め込みなら `attributedTo` がそのアカウントで宛先（`to` / `cc`）に `as:Public` を含み、IRI ならアカウントと同じオリジンで `Create` の宛先に `as:Public` を含むものを、新しい順（すべてに `published` があればその順、無ければページの順）に 20 件まで選びます。`Announce` と IRI だけの項目は選びません。
    3. 選んだ投稿のうち保存済みのものは取得しません。埋め込みのものはその文書を、IRI のものは取得した文書（`attributedTo` がそのアカウントで宛先に `as:Public` を含むものだけ）を、§6.3 の取得した投稿として取り込みます。返信先は取得しません（利用者が投稿を開けばスレッドの取得が補う）。
  - **件数の上限**: 1 回の取得の文書は、outbox・ページ・投稿 20 件の合わせて 22 件までです（`OutboxMaxDocuments`）。取得の失敗と取り込まない文書は `info` のログを残して次の投稿に進み、outbox やページが取得できなければジョブを完了にします。ストアの失敗は `Retry` です。
  - 取り込んだ投稿は、ほかの取得した投稿と同じく保持期間（ADR-0034、`content.purge_remote_statuses`）の対象で、プロフィールのタイムラインに出ます。新しいテーブルは使いません。
  - コントラクトテストは `testkit.RemoteStatusContract` で、Mastodon・Misskey・GoToSocial の outbox のフィクスチャ（`outbox.json`・`outbox-page.json`）を取り込みます。
- 投稿の ID は `published` から作った UUIDv7 です（[data-model §1.3](data-model/01-conventions.md#13-id-とキーセットページング)、#71）。
- 保持期間を過ぎたリモートの投稿は `content.purge_remote_statuses` が、関係の無くなったリモートのアカウントの過去のプロフィールは `account.prune_remote_profiles` が、受信の原本・記録と配送の記録は `federation.purge` が、どこからも参照されなくなったリモートのカスタム絵文字（管理者が無効にしたものを除く）は `content.purge_remote_emojis`（1 日ごと、ADR-0051 §1）が、件数を区切って消します（ADR-0034、[data-model §4.4 のリモート投稿の保持期間](data-model/04-tables/04-content.md#リモート投稿の保持期間)、§4.2、§4.8）。
  保持期間で消えた投稿（`origin = retention`）は tombstone にならないので、同じ URI の `Create` や取得でまた取り込めます。コントラクトテストは `testkit.RetentionContract` です。

### 6.4 投稿の取り直し

- 保存したリモートの投稿は、受け取った `Update` で新しくします（§3.4）。オリジンから取り直すのは、運営者が管理 API（`POST /api/v1/admin/statuses/{id}/refetch`・`POST /api/v1/admin/accounts/{id}/refetch_statuses`・`POST /api/v1/admin/instances/{domain}/refetch_statuses`、権限 `manage_domains`）か CLI（`my admin refetch status|account|domain`）で頼んだときだけで、定期ジョブはありません（FR-407）。
- 頼むユースケースは `federation.Refetches` です。対象を確かめ（未知・削除済みの投稿と未知のアカウントは `ErrNotFound`、ローカルの投稿・アカウントと自サーバーのドメインは `ErrNotRemote`、ホスト名でないドメインは `ErrInvalidHost`）、監査ログ（`status_refetch_queued`・`account_statuses_refetch_queued`・`domain_statuses_refetch_queued`）と最初のジョブを 1 つのトランザクションで `queue.Outbox` に書きます。
- ジョブ `federation.refetch_status`（ペイロードは `status_id`、冪等キー `refetch_status:<status_id>`）は、`StatusHandlers.Refetch` で投稿を 1 件取り直します。取得は §6.1 の規則どおり（`Fetcher`。`federation` の制限・`unavailable` のドメイン・失敗の記憶）です。
  - 文書が投稿で `id` が投稿の URI と同じで、`attributedTo` が投稿の作者なら、受け取った `Update` と同じく `content.ReviseRemoteStatus` で記録します（メンションの解決を含む）。版の時刻が新しければ版を足し、そうでなければ投票の票数だけを記録します。
  - 410 Gone と、投稿の URI の `Tombstone` は、作者の `Delete` と同じく `content.DeleteRemoteStatus` で削除します（tombstone が残る）。404 は削除しません。フォロワー限定の投稿などはインスタンス Actor の取得に 404 を返すことがあるためです。
  - 取得できない・ほかの作者の・投稿でない文書は `info` のログを残してジョブを完了にし、投稿をそのまま残します。ローカル・削除済み・未知の投稿は何もしません。ストアの失敗は `Retry` です。
- ジョブ `federation.refetch_statuses` は、アカウントかドメインの投稿をたどって `federation.refetch_status` を投入します。ペイロードは頼んだ監査ログの ID（`request`）、`account_id` か `domain`、カーソル（`before`・`before_account`）、残りの件数（`remaining`）で、冪等キーは `refetch_statuses:<request>:<before_account>:<before>` です。
  - アカウントは、削除されていないリモートの投稿を新しい順（`content.LiveRemoteStatusIDs`）に 100 件（`RefetchBatch`）ずつ、合わせて 1,000 件（`RefetchMaxStatuses`）までたどります。
  - ドメインは、そのドメインのリモートのアカウントを新しい順（`account.Search`）に 5 件（`RefetchAccountsBatch`）ずつ読み、アカウントごとに新しい投稿 20 件（`RefetchStatusesPerAccount`）まで、合わせて 1,000 件までたどります。
  - 続きのジョブは 1 分後（`RefetchBatchInterval`）に実行し、1 つのサーバーへの取得が一度に集まらないようにします。

## 7. 送信（配送）

### 7.1 記録

展開と配送のジョブは、実行の時点の連合の向きで何を送るかを決めます（[§14](#14-連合の向き)）。向きが送らないものは `skipped` にし、広げ直しても再送しません。

ローカルの操作のトランザクションで、`federation` が他のモジュールのドメインイベントを購読し（`uow.Projectors`）、`outbound_activities` と `federation.fanout` ジョブ（`queue.Outbox`）を同じトランザクションで書きます。

| ドメインイベント | アクティビティ | 宛先 |
| --- | --- | --- |
| `content.status_created` | `Create` | §7.2 の公開範囲ごとの宛先 |
| `content.status_revised` | `Update` | 元の `Create` の `deliveries` の inbox（`public` / `unlisted` は加えてその時点のフォロワー。[data-model §7](data-model/07-visibility-and-delivery.md#7-公開範囲と配送)） |
| `content.status_deleted`（アカウントの消去による `origin = account_deletion` を除く。`Delete`（Actor）が代わりを務める） | `Delete`（`Tombstone`） | 元の `Create` の `deliveries` の inbox（`public` / `unlisted` は加えてその時点のフォロワー） |
| `content.boosted` / `content.boost_retracted`（`Federates` のときだけ） | `Announce` / `Undo Announce` | ブーストした人のリモートフォロワー + 元投稿の作者（リモートなら） |
| `content.favourited` / `content.unfavourited`（リモートの投稿だけ） | `Like` / `Undo Like`（本文は projector がイベントから組み立てて保存する。Mastodon と同じく宛先を持たない） | 投稿の作者の inbox |
| `content.reacted` / `content.unreacted`（ローカルの人のリモートの投稿へのものだけ） | `content` と `_misskey_reaction`（同じ文字列。Unicode 絵文字か `:shortcode:`）を付けた `Like`、カスタム絵文字なら `tag` に `Emoji` を 1 つ（ADR-0051 の形）。`@context` に `misskey` と `_misskey_reaction` を足す。`Undo Like` はその `Like` を埋め込む（ADR-0052 §3。お気に入りと同じ `Like` の枠の活動） | 投稿の作者の inbox |
| `content.poll_voted`（リモートの投票だけ） | 選んだ選択肢ごとに `Create`。`object` は `Note`（`id` は回答の `uri`、`name` は選択肢の題、`inReplyTo` は `Question`、`attributedTo` は回答した人、`to` は投票の作者）で、`Create` の `id` は `{Note}/activity`。Mastodon が送る形で、Mastodon・Misskey・Pleroma（Akkoma）・GoToSocial は回答として数える（FR-112）。本文は projector がイベントから組み立てて保存する | 投稿の作者の inbox |
| `content.poll_voted`（ローカルの投票。ローカルとリモートのアカウントの回答）、`content.poll_closed`（ローカルの投票） | ジョブ `federation.poll_update`（冪等キーは投票ごと）を、回答なら 3 分後（`PollUpdateDelay`。その間の回答は 1 つの `Update` にまとまる）に、締め切りならすぐに動かす。ジョブは投票が投稿の現在版のものなら、その版の `Update` を記録する。`Question` の票数は展開がその時点の集計から組み立てる | 元の `Create` の `deliveries` の inbox（`public` / `unlisted` は加えてその時点のフォロワー）+ 回答したリモートのアカウント |
| `graph.follow_requested` / `graph.followed`（相手がリモート） | `Follow` | 相手の inbox |
| `graph.follow_terminated`（`unfollowed` / `blocked` / `moderation` / `moved`、自分がフォローした相手がリモート） | `Undo Follow` | 相手の inbox |
| `graph.followed` / `graph.follow_accepted` / `graph.follow_rejected`（フォローした人がリモート。相手の `Accept` / `Reject` によるものを除く） | `Accept` / `Reject` | フォローした人の inbox |
| `graph.follow_resent`（フォローした人がリモート） | `Accept`（もう一度届いた `Follow` を埋め込む） | フォローした人の inbox |
| `graph.follow_terminated`（`removed_by_followee`、フォローした人がリモート） | `Reject` | フォローした人の inbox |
| `graph.blocked` / `graph.unblocked`（相手がリモート） | `Block` / `Undo Block` | 相手の inbox |
| `account.profile_revised`（ローカルのプロフィールの 2 版目以降。最初の版はアカウントの作成で、まだだれもフォローしていない） | `Update`（Actor の文書） | リモートフォロワーの inbox |
| `account.lifecycle_event_recorded`（ローカルのアカウントの `deleted`） | `Delete`（Actor の `id`） | リモートフォロワーの inbox + 既知の shared inbox（[data-model §8](data-model/08-deletion-and-purge.md#8-削除と消去)） |
| `moderation.account_suspension_changed`（ローカルのアカウントの凍結と、その終わり。§11.1） | `Update`（Actor の文書） | リモートフォロワーの inbox + 既知の shared inbox |
| `account.moved`（ローカルのアカウントが自分で移転したとき。`initiated_by = self`。ADR-0089） | `movedTo` 付きの `Update`（Actor の文書）、続けて `Move`（`object_type = move`、`object_id` は `account_moves.id`。`actor` と `object` はこちらの Actor、`target` は移転先の Actor、`to` はこちらの `followers`、`id` は `/ap/activities/{uuid}`） | リモートフォロワーの inbox（shared inbox に集約） |
| `account.move_cancelled`（ローカルのアカウントの移転の取り消し） | `Update`（Actor の文書。`movedTo` なし） | リモートフォロワーの inbox |
| `moderation.report_filed`（`forwarded` の通報だけ。§11） | `Flag`（主体はインスタンス Actor） | 通報されたアカウントの inbox |
| `content.quote_recorded`（ローカルの投稿の、引用の許可を宣言したリモートの投稿の `fep_044f` の引用。ADR-0069 §3） | `QuoteRequest`（`id` は `{引用元の IRI}/quote_request`、`object` は引用先、`instrument` は引用元。本文は projector が組み立てて保存する） | 引用先の作者の inbox |
| `content.quote_changed`（ローカルの投稿の引用が、リモートの `QuoteAuthorization` で承認されたとき）、`content.quote_policy_changed` | 現在の版の `Update`（`quoteAuthorization` と `interactionPolicy` が新しくなる。版が増えないので `updated` は版の時刻のまま） | 元の `Create` の `deliveries` の inbox（`public` / `unlisted` は加えてその時点のフォロワー） |
| `content.quote_authorization_revoked`（引用元の作者がリモートのとき） | `Delete`（`object` は `QuoteAuthorization` の IRI。`to` は引用元の作者、`cc` は自分の followers。本文は projector が組み立てて保存する） | 引用元の作者の inbox + 引用先の作者のリモートフォロワーの inbox |

- フォローの活動（`object_type = follow`）の主体は、`Follow` と `Undo` ではフォローした人、`Accept` と `Reject` ではフォローされた人です。`Follow` の `id` はフォローの `uri`（`/ap/activities/{follow_id}`）で、`Accept` / `Reject` / `Undo` はフォローの `Follow`（受け取ったフォローなら相手が送った `id`。送り直された `Follow` への `Accept` はその `id`）を埋め込みます。宛先は相手だけで、`to` に相手を入れます。
  相手が送った `Accept` / `Reject` / `Undo` と、どちらかのアカウントの削除（`account_deleted`）による変化は、何も送り返しません。
- 記録するのはローカルの投稿・ブーストの活動だけです。リモートの投稿のイベントと、`local` 投稿のブースト（`Boost.Federates` が偽）は記録しません（ADR-0017）。
- projector は表の 1 行につき 1 つです（`federation.RegisterOutboundProjectors`）。送信する活動を増やすときは、その活動を送るユースケースと一緒に projector を足します。
- `inline` のキュー（テストと開発用）はトランザクションでの投入ができないので、`queue.AfterCommit` がコミットの後に投入します。
- アクティビティの文書は `federation.fanout` が 1 回だけ組み立て、`outbound_activity_documents` に保存します。すべての配送とリトライが同じ本文を送ります（再試行で内容が変わらない）。
- 投稿の `Delete` の文書は、projector が削除のトランザクションでイベントだけから組み立てて保存します。消去ジョブが投稿の行を消した後でも、`Delete` は保存した文書で元の `Create` の `deliveries` の inbox に届きます。
  宛先は投稿自身の公開範囲で作者の followers に向け（§7.4）、メンション先は入れません（`Delete` は対象が消えたことしか伝えない）。
- 投稿の文書は、イベントが指す版から組み立てます（後の編集で内容が変わらない）。

### 7.2 展開（fan-out）

`federation.fanout`（ペイロードは `outbound_activity_id`、`Create` / `Update` が示す版の `revision_id`、カーソル）が、宛先を一定件数（500 件）ずつ算出します。
カーソルは宛先の出どころ（フェーズ）とその中の位置で、1 つのジョブは 1 つのフェーズの 1 回分だけを処理します。

| アクティビティ | フェーズ（順に） |
| --- | --- |
| `Create` | 版のメンション先 → 作者の承認済みのフォロワー → （`public` だけ）参加中のリレーの inbox（§13） |
| `Update` / `Delete` | 元の `Create` の `deliveries` → （`public` / `unlisted` だけ）その時点のフォロワー → （投票のある投稿の `Update` だけ）回答したアカウント（`ContextObjectFetch` で判定する。回答した人は投稿を見ているが、配送の相手とは限らないため） |
| `Announce` / `Undo Announce` | ブーストした人の承認済みのフォロワー → 元投稿の作者 |
| `Like` / `Undo Like`、投票への回答の `Create` | 投稿の作者 |
| `Update`（Actor）、`Move` | Actor の承認済みのフォロワー |
| `Delete`（Actor）、凍結とその終わりの `Update`（Actor。ペイロードの `shared_inboxes`） | Actor の承認済みのフォロワー → 既知の shared inbox（`account.KnownSharedInboxes`） |
| `Follow` / `Accept` / `Reject` / `Undo Follow` | フォローの相手（その Actor の inbox。無ければ shared inbox）。公開範囲では判定しない |
| `Flag` | 通報されたアカウント（その Actor の inbox。shared inbox に集約しない）。公開範囲では判定しない |
| リレーの `Follow` / `Undo` と、インスタンス Actor への `Follow` に答える `Accept` / `Reject` | ペイロードの `inbox`（記録したときのリレーか相手の inbox）。本文は記録のトランザクションで保存済み。公開範囲では判定しない |

1. 宛先のアカウントを集める: 作者の承認済みのフォロワー（`graph` の公開 API をカーソルで）、版のメンション先、元投稿の作者。ローカルのアカウントは inbox を持たないので落ちます。
2. 公開範囲の規則で絞る: `content.CanView`（`ContextDelivery`、[visibility.md](visibility.md)）を宛先のアカウントごとに判定する。
   - フォロワーは `Follow = active` として判定します（ブロックは両方向のフォローを同じトランザクションで終わらせるので、承認済みのフォロワーとの間にブロックはありません）。メンション先は `graph` の公開 API から関係（フォロー・ブロック）を読みます。
   - ブーストは「ブーストした人の、ブーストの公開範囲の投稿」として判定し、元投稿の作者をメンション先として扱います。
   - `Delete` は対象が消えたことしか伝えないので判定しません。Actor の `Update` も、プロフィールは公開なので判定しません。
   - `local` は承認済みのフォロワーだけで、**フォロワーでないリモートのメンション先には送りません**（ADR-0017）。
   - `local` 投稿の文書の `tag` には、フォロワーでないリモートのアカウントへの `Mention` を入れません（ADR-0035）。本文のリンクは残します。
3. inbox を決める: アカウントの `shared_inbox_url`（無ければ `inbox_url`）。同じ inbox は 1 つにまとめます（shared inbox への集約、NFR-COST-5、[ActivityPub §7.1.3](https://www.w3.org/TR/activitypub/#shared-inbox-delivery)）。
4. ドメインの状態で絞る: `federation` の制限（ドメイン単位の `suspend`、§11）があるドメインと、`instance_health` が `unavailable`（§7.3）のドメインには送りません（`Follow` と `Accept` / `Reject` は `unavailable` でも送る）。
5. `deliveries` に `ON CONFLICT DO NOTHING` で挿入し、新しい行ごとに `federation.deliver` ジョブ（冪等キー `deliver:<delivery_id>`）を投入します。
6. 残りがあれば、カーソルを進めた `federation.fanout` を投入します。

`Like` / `Undo Like` のジョブは順序キー（ADR-0004 の `orderingKey`）を持ちます。
同じローカルのアカウントから同じリモートの投稿への `Like` / `Undo Like` は、お気に入りでも絵文字リアクションでも（`Like` の枠、ADR-0052 §1）、`federation.fanout` が `like:<アカウントの ID>:<投稿の ID>`、`federation.deliver` がそれに `:<inbox>` を足したキーを持ちます。
枠の付け替えは外したものの `Undo` と新しい `Like` を送るので、1 人 1 投稿に 1 リアクションの実装（Misskey）では、`Like` が `Undo` を追い越すと拒否されて何も残りません。
展開がキーで直列化されるので後の活動の配送は前の活動の配送より後に投入され、配送は同じ inbox への前の配送がリトライ中でも完了か dead job になるまで待ちます。
ほかの宛先の配送は待ちません。ほかの活動は順序キーを持たず、受信側が順序に依存しない処理をする前提です（§3.3）。

### 7.3 配送とリトライ

`federation.deliver`（ペイロードは `delivery_id`）:

1. 宛先のドメインに送らない活動なら（`unavailable`、またはドメイン単位の `suspend`。§7.2 の手順 4 と同じ `federation.Deliverable`）、試行せず `skipped` を記録して完了にする。
2. inbox のホストのサーキットブレーカーが開いていたら、送らずに `retryable_failure`（`signature_scheme` は NULL）を記録して `Retry` を返す。
3. `outbound_activity_documents` の本文に、`public` の投稿の `Create` / `Update` / `Delete`（`to` に `as:Public` を含む）なら Actor の RSA 鍵で LD 署名（`signature`）を、それ以外なら Actor の Ed25519 鍵（`{actor}#ed25519-key`）で `proof`（FEP-8b32）を付け、Actor の RSA 鍵（`{actor}#main-key`）で HTTP 署名して POST する（§2 の double-knocking）。
   - LD 署名は Mastodon の `LinkedDataSignature#sign!` と同じ形です。署名の選択肢（`type`・`id`・`signatureValue` を除き、`@context` を `https://w3id.org/identity/v1` にしたもの）と、`signature` を除いた文書を、それぞれ URDNA2015 で正規化した N-Quads の SHA-256（16 進）をつなげ、RSASSA-PKCS1-v1_5・SHA-256 で署名します。`creator` は `{actor}#main-key`、`created` は活動の時刻です。RSA の署名は決定的なので、同じ活動の配送とリトライはどれも同じバイト列を送ります。
   - 正規化は `internal/adapter/jsonld`（`github.com/piprate/json-gold`、ポート `federation.Canonicalizer`）が行い、`@context` は同梱した ActivityStreams・`w3id.org/security/v1`・`w3id.org/identity/v1` の文書だけで解決します（リモートの文脈を取得しない）。jsonld.js（Misskey の処理系）と同じ N-Quads と署名になることをテストが確かめます。
   - LD 署名を付けた文書には `proof` を付けません。LD 署名の後に足す `proof` は LD 署名を壊し、先に足す `proof` は data integrity の文脈を受け手の LD の処理系が持たないためです。LD 署名を作れなかった文書は、`warn` のログを残して `proof` を付けて送ります。
   - `proof` は `eddsa-jcs-2022`（JCS（[RFC 8785](https://www.rfc-editor.org/rfc/rfc8785)）・SHA-256・Ed25519）で、`created` は活動の時刻（`outbound_activities.created_at`）です。Ed25519 の署名は決定的なので、同じ活動の配送とリトライはどれも同じバイト列を送ります。保存する本文は変えません。
   - 本文の `@context` に `https://w3id.org/security/data-integrity/v2` が無ければ足し、`proof` にも同じ `@context` を付けます（FEP-8b32 の例と同じ）。
   - Ed25519 の鍵が無い Actor の文書と、LD 署名（`signature`）の付いた文書には付けません。
   - JCS と `proof` の作成・検証は `internal/domain/federation`（`CanonicalJSON`、`SignEdDSAJCS2022`、`ReadProof`）にあり、FEP-8b32 と FEP-521a のテストベクタで確かめます。
4. 結果を分類して `delivery_attempts` に記録し、同じトランザクションでイベント `federation.delivery_attempted` を発行する。同期 projector が `instance_health` を更新する。

| 応答 | 分類 | ジョブ |
| --- | --- | --- |
| 2xx | `succeeded` | `Done` |
| 408 / 429 / 5xx（501 を除く）/ 接続・TLS・タイムアウトのエラー | `retryable_failure` | `Retry`（`Retry-After` があればそれ以降） |
| 400 / 401（double-knocking の後）/ 403 / 404 / 410 / 413 / 422 などの 4xx、501 / `HTTPClient` が拒否した宛先（SSRF ガード・リダイレクトの上限） | `permanent_failure` | `Done`（相手が受け取らないと答えた、または送れない宛先なので、リトライしない） |

- `Retry-After` は秒数と HTTP の日付のどちらも読み、24 時間で打ち切ります。ジョブの期限（リース）や停止で応答の前に打ち切られた試行は記録せず、そのまま `Retry` にします。
- 配送や Actor が見つからない、Actor に署名の鍵がないジョブは `Fail` にします（dead job に残る）。

- リトライの間隔は ADR-0004 の配送の例（1 分、5 分、30 分、2 時間、12 時間、24 時間 × 3。最大 9 回、約 4 日）で、ジッタを掛けます（`RegisterWithPolicy`）。使い切ったジョブは dead job になり、管理 API（`GET /api/v1/admin/jobs?state=dead` と `POST /api/v1/admin/jobs/{id}/retry`、権限 `manage_jobs`）から再実行できます（NFR-REL-2）。
- **短期のサーキットブレーカー**: プロセス内で inbox のホストごとに、`retryable_failure` が連続 10 回続いたら 60 秒間はリクエストせずに `Retry` を返します（Mastodon の `Stoplight` と同じ値）。
  60 秒の後に送った 1 回がまた失敗すると、すぐにまた開きます。成功と `permanent_failure`（相手が答えた）で閉じます。開いている間の `Retry` も試行の 1 回に数え、待ち時間は上のリトライの間隔に従います（60 秒ごとに試行を使い切らない）。
- **`InstanceHealth`（長期）**: `retryable_failure` を記録した日（UTC の日付）がドメインごとに異なる 7 日に達したら、`instance_availability_events`（`marked_unavailable`）を記録し、`instance_health.state` を `unavailable` にします（Mastodon の `DeliveryFailureTracker` と同じ基準）。
  配送の成功、またはそのドメインから署名付きのリクエストを受けたら `marked_available` を記録して `available` に戻します。`unavailable` のドメインには、定期ジョブ `federation.probe_instances` が 1 日 1 回 NodeInfo を取得し、成功したら戻します。
  運営者は管理 API（`POST /api/v1/admin/instances/{domain}/mark_available`、権限 `manage_domains`）でも戻せます。理由は `operator` で、失敗の日数も 0 に戻ります。
  `instance_health` と `instance_availability_events` は管理 API（`GET /api/v1/admin/instances`、`.../{domain}/availability_events`）で読めます。
  - 署名付きの受信と probe の成功は、`federation.Health.Reached`（理由 `inbound_request` / `probe_succeeded`）がイベント `federation.instance_reached` を発行し、配送と同じ projector が `instance_health` を更新します。失敗の記録が無いドメインでは、コミット済みの値を読むだけで書きません。
  - probe は `/.well-known/nodeinfo` と、そこから同じホストの NodeInfo 2.x の文書を署名なしで取得し、どちらも 2xx の JSON なら成功とします。1 つのジョブは 50 ドメインを順に処理し、残りがあれば続きのジョブを投入します。
- 1 件の配送は 1 つのジョブです。Cloud Tasks / SQS では 1 回の試行が 1 オペレーションとして課金されるので、shared inbox への集約と `unavailable` のドメインの除外で回数を抑えます（NFR-COST-5）。

### 7.4 文書の形

- `Create` の `id` は `{object}/activity`、それ以外は `/ap/activities/{uuid}`（ADR-0018）。
- 宛先: `public` は `to: [Public]`・`cc: [followers, メンション先]`、`unlisted` は `to: [followers]`・`cc: [Public, メンション先]`、`private` と `local` は `to: [followers]`（`private` は加えて `cc: [メンション先]`）、`direct` は `to: [メンション先]`。
- ローカルの Actor の文書は、アバターを `icon`、ヘッダーを `image`（どちらも `Image` で `mediaType` と `url`）に載せます。URL は `large` の版で、アニメーションなら動く版（GIF）です。静止版は受け取ったサーバーが作ります（[ADR-0054](../adr/0054-media-variants-video-audio.md) §5）。凍結中の文書には載せません。
  `tag` には、現在のプロフィールの版の表示名・自己紹介・補足情報で使った有効なカスタム絵文字の `Emoji` を載せます（下の Note と同じ形）。
- Actor の `Update` は `to: [Public]` で、`object` に `GET` と同じ Actor の文書（`federation.LocalActorDocument`、`@context` なし）を埋め込みます。文書は展開の始めの時点の Actor から組み立てます（後の版はそれぞれの `Update` を送る）。その時点で凍結中なら、GET と同じ凍結中の文書です（§11.1）。
  有効でなくなったアカウントの `Update` は送りません。ただし削除済みでない凍結中のアカウント（BAN で削除を予約したものを含む）の `Update` は送ります。
- Actor の `Delete` は `to: [Public]` で、`object` は Actor の `id` です。
- `contentMap` は版の言語ごとの本文、`content` は版の既定の言語の本文です（NFR-I18N-5）。言語が `und` の本文は `contentMap` に入れません。`sensitive`・`summary`（CW）・`tag`（`Mention` / `Hashtag` / `Emoji`）・`attachment`（メディアの公開 URL・`mediaType`・`name`・`blurhash`・`focalPoint`・`width` / `height`）を付けます。
  - `sensitive` は CW があれば真です（Mastodon と同じ）。`summary` と `contentMap` は空なら省き、`sensitive`・`content`・`tag`・`attachment` は常に付けます。
  - `attachment` は処理の終わったメディアだけで、種類を問わず `type` は `Document`（Mastodon と同じ。種類は `mediaType` で伝える）、URL・`mediaType`・寸法は `large` の版のもので、アニメーションなら動く版（GIF）です（[ADR-0054](../adr/0054-media-variants-video-audio.md) §5）。焦点が中央なら `focalPoint` を省きます。
    動画・音声は remux したファイルを `url` に、ポスターの `large` を `icon`（`Image`。ポスターがなければ省く）に、長さを `duration`（`PT12.5S` の形の `xsd:duration`）に載せます（Mastodon と同じ形）。
  - `Mention` は `href` が Actor の IRI、`name` が `@user@domain`（ローカルのアカウントは設定 `account_domain`）、`Hashtag` は `href` が `https://{host}/tags/{name}`（ADR-0018）、`name` が `#name` です。
  - `Emoji`（[ADR-0051](../adr/0051-custom-emoji.md) §3）は、版の本文と CW で使った有効なカスタム絵文字です。`id` はローカルなら `https://{host}/ap/emojis/{uuid}`（リモートなら記録した `uri`、無ければ省く）、`name` は `:shortcode:`、`updated` は現在の画像を設定した時刻、`icon` は `Image` で、`url` と `mediaType` は `large` の版（アニメーションなら動く版の GIF、静止画は常に PNG）です。
    ローカルの投稿とプロフィールの絵文字は、保存のときに原文の `:shortcode:` から有効なローカルの絵文字だけを版に記録したものです（data-model §4.4）。リアクション（ADR-0052）も同じ形を使います（`federation.EmojiTag`）。
- 引用（FR-114、ADR-0069 §3）は、引用元の作者が外していなければ、`quote` と同じ IRI の `_misskey_quote`・`quoteUrl`・`quoteUri`、承認されていれば `quoteAuthorization` を付け、本文（`content` と `contentMap` のどの言語も）の前に `<p class="quote-inline">RE: <a href="{引用先の表示用 URL}">…</a></p>` を置きます（Mastodon と同じ形）。
  どの投稿にも `interactionPolicy.canQuote.automaticApproval` を付けます。`public` / `unlisted` の投稿は引用の許可に従って `Public`・作者の followers・作者自身のどれか、`private` / `direct` / `local` は作者自身です。手で承認することはないので `manualApproval` は付けません。
- 投票のある投稿は `Question` です（FR-112）。単一選択は `oneOf`、複数選択は `anyOf` に、選択肢ごとの `Note`（`name` は題、`replies` は `totalItems` だけの `Collection`）を載せ、`endTime`（締め切り）、締め切った後は `closed`（`endTime` と同じ時刻）、`votersCount`（`toot:votersCount`）を付けます。
  票数は文書を組み立てる時点の集計で、作者が締め切りまで票数を隠す投票では締め切りまで 0 です。編集されていない投稿の票数の `Update` は `updated` を持たず、受け取ったサーバーは投稿を編集済みにしません（Mastodon は投票だけの更新として扱う）。
- Note は `federation.notes`（`internal/app/federation`）の 1 か所で組み立て、配送（`Create` / `Update` の `object`）とオブジェクトの GET（§10）が同じものを使います。版のメディア・メンション先・ハッシュタグ・カスタム絵文字は `content.Reader.Parts` で引きます。
- `@context` は AS2・security・Mastodon 拡張（`toot:`。`Emoji` は `toot:Emoji`）と、引用の語彙（FEP-044f の `quote`・`quoteAuthorization`・`QuoteRequest`・`QuoteAuthorization`、GoToSocial の `interactionPolicy`・`canQuote`・`automaticApproval`・`interactingObject`・`interactionTarget`、Misskey の `_misskey_quote`、`quoteUrl`、Fedibird の `quoteUri`）です（`federation.LDContext`）。配送では `proof` のために data integrity の文脈を足します（§7.3）。
- ローカルの Actor の文書と、それを埋め込む `Update` の `@context` は、security の後に Controlled Identifiers（`https://www.w3.org/ns/cid/v1`）を足したものです（`federation.ActorLDContext`）。`assertionMethod` に Ed25519 の鍵を `Multikey`（`id` は `{actor}#ed25519-key`、`controller` は Actor、`publicKeyMultibase` は base58btc の `z6Mk…`）で載せます（FEP-521a）。RSA の鍵は `publicKey` に載せます。

## 8. 公開範囲 `local` と連合

ADR-0017 を連合の各経路に当てはめた結果です。

| 経路 | 振る舞い |
| --- | --- |
| 配送 | 承認済みのリモートフォロワーの inbox（shared inbox に集約）だけ。フォロワーでないメンション先には送らない |
| 文書 | `to` は followers だけ。`as:Public` を入れない。フォロワーでないリモートのアカウントへの `Mention` を `tag` に入れない（ADR-0035） |
| オブジェクトの GET | 署名者のドメインに作者の承認済みのフォロワーがいるときだけ返す。それ以外は 404（ADR-0018） |
| ブースト | ローカルのブーストは連合しない。リモートからはフォロワー限定に見えるのでブーストされない |
| `Update` / `Delete` | 元の `Create` の `deliveries` の inbox だけ |
| outbox コレクション | 列挙しない（`public` / `unlisted` だけ、ADR-0018） |

## 9. リモートメディア

ADR-0005 の方針（既定はキャッシュしない）に従います。

- リモートの添付・アバター・ヘッダー・カスタム絵文字は、`media_files`（`origin = remote`）と `remote_media` に、受信した文書が宣言した値（URL・`mediaType`・寸法・`blurhash`・焦点・代替テキスト）を記録します。取り込み時にファイルは取得しません。
- 取り込みは `content.MediaService.RecordRemoteMedia` を受信のトランザクションの中で呼び、同じ所有者・同じ URL のファイルは最初に記録したものを返します。
- クライアント API は、リモートのメディアの URL を **メディアプロキシの URL** `https://{host}/media/proxy/{media_id}/{variant}?sig=…`（`content.MediaProxy.URL`）で返します。
  `sig` は `media_id` と `variant` の HMAC-SHA256（base64url）で、プロキシを任意の URL の中継に使わせません。
  鍵は設定 `kek` から HKDF-SHA256 で導くので、プロキシに専用の秘密はありません。`kek_previous` から導いた鍵も検証に使うので、KEK のローテーション中も前の URL が使えます。
  `variant` は元のファイルそのものの `full` と、静止画の小さい版の `small` です（[ADR-0054](../adr/0054-media-variants-video-audio.md) §7）。
  - `small` は、記録したときの用途（`remote_media.purpose`: 添付・アイコン・ヘッダー・絵文字）の S の箱（ローカルの版と同じ、data-model §4.4）に縮めた静止画の WebP です。アニメーションは最初のフレームです。
    プロキシが `full` と同じように取得して（キャッシュがあればそこから読んで）、要求のたびに `media.Processor.Preview` で作ります。宣言が動画・音声・`gifv` のもの、中身が画像でないもの、処理できないものは 404 です。
    `remote_media_policy` がそのファイルをキャッシュするときは、`remote_media_cache` に URL と用途を合わせたキー（`{URL}#small-{purpose}`）でも保存します。M / L は作らず、`full` を使います。
  - クライアント API はリモートの画像の `small` にこの URL を、`medium` / `large` に `full` を載せます。リモートのアカウントのアイコン・ヘッダーと添付の `preview_url` が小さくなり、アニメーションを止めて表示できます。
- プロキシ（`content.MediaProxy` と `internal/interface/media`）は、`HTTPClient` で元の URL を取得し、形式を中身の先頭のバイトで判定して（ADR-0019 §8。許可リストは JPEG / PNG / GIF / WebP / AVIF / MP4 / WebM / MP3 / Ogg で、SVG は拒否）、
  判定した `Content-Type`、`Cache-Control: public, max-age={remote_media_cache_ttl}`、`X-Content-Type-Options: nosniff`、`Content-Security-Policy: default-src 'none'; sandbox`、`Cross-Origin-Resource-Policy: cross-origin` を付けて中継します。
  リモートのヘッダ（`Content-Type` など）と利用者のリクエストのヘッダは使いません。CDN がキャッシュします（NFR-COST-4）。
- サイズの上限は、中身で判定した種類ごとに画像 16 MiB、動画・音声 100 MiB です（ADR-0019 §1）。`Content-Length` が上限を超えるものは本文を読まずに断ります。
  長さのない画像は上限まで読んでから返し、長さのない動画・音声は中継の途中で上限を超えたらレスポンスを中断します（CDN が途中までのファイルをキャッシュしないように）。
- カスタム絵文字の画像（`remote_media.purpose = emoji`、`custom_emoji_images` が指すもの）は、形式を PNG / GIF / WebP に絞り、上限を設定 `remote_custom_emoji_max_bytes`（既定 1 MiB）にします（ADR-0051 §2）。
  超えるもの・ほかの形式のものは 404 で、上限を下げる前に保存したキャッシュの複製も同じです。
- 署名がない・合わない URL、記録のないメディア、`media_fetch` の制限、SSRF ガードが拒否した宛先、リモートの 4xx、許可リストにない形式、上限を超えるものは、どれも 404 です。リモートに届かないときと 5xx は 502 です。エラーのレスポンスは `Cache-Control: no-store` です。
- インスタンス設定 `remote_media_policy`（`none` / `avatars` / `all`、既定 `none`）と `remote_media_cache_ttl`（秒、既定 86400）で、TTL 付きのキャッシュを選べます（FR-409）。
  - `none` では、プロキシは毎回取得して中継し、キャッシュを読みも書きもしません。
  - `all` ではすべてのリモートのメディアを、`avatars` ではプロフィールのアバター・ヘッダー（`account` モジュールの `IsMediaReferenced`）とカスタム絵文字の画像（`custom_emoji_images`）だけを、キャッシュします。
  - キャッシュは URL ごとに 1 つで、Blob の非公開のキー `remote/{URL の SHA-256 の 16 進}` に置き、`remote_media_cache` に記録します。
    期限（`expires_at` = 取得時刻 + `remote_media_cache_ttl`）の内はそこから返し、期限切れ・行がない・Blob がないときは取得し直します。
    取得したファイルは中身で判定した形式とサイズの上限の検査を通したうえで Blob に最後まで書いてから返すので、上限を超えるもの・途中で切れたものは残りも返りもしません。
  - 返した時刻（`last_accessed_at`）は 1 時間より細かくは書きません。返すたびにデータベースへ書かないためです。
  - 1 時間ごとのジョブ `media.cleanup_remote` が、期限切れのものを消し、続けて合計サイズが設定 `remote_media_cache_max_bytes`（既定 1 GiB）を超える分を `last_accessed_at` の古い順に消します（FR-803）。
    容量の上限は Blob の費用に効くので、インスタンス設定ではなく運用者の設定です。1 回の実行で見る行数には上限があり、残りは次の実行が消します。
    Blob を先に消してから行を消すので、途中で失敗しても行が残って次の実行が消し直し、どこからも指されない Blob は残りません。行は読んだときの `fetched_at` のままのときだけ消すので、その間に取得し直したものは残ります。
- ドメイン単位の `media_fetch` の制限（`reject_media`）は、メディアの所有者のドメインと URL のホストの両方に当て、どちらかが制限されていればプロキシは取得せず 404 を返します。
  判定はポート `content.MediaFetchPolicy` を通します。実装は `moderation.FederationPolicy` です（§11）。
  `media_sensitive` の制限（`force_sensitive`）は表示のときに添付をセンシティブにします。
- サムネイル・blurhash がない動画は、クライアントが「プレビューなし」として表示します。

### 9.1 リンクのカード

投稿の本文にあるリンクの先のページから、題名・説明・画像を取り出してカードにします（FR-117、[ADR-0062](../adr/0062-link-preview-cards.md)）。
自サーバーのページに付ける OGP / X Card の meta（ADR-0028 の「プレビュー」）とは別の仕組みです。実装は `content.LinkCards`（`internal/app/content/link_card*.go`）です。

- **契機**: 投稿の作成・編集のイベント（ローカルとリモートの両方。リモートは `Create` / `Update` の取り込み）の投影が、現在の版から URL を選んで `status_link_cards` に書き、
  そのカードがない・取得から 14 日を過ぎているときに、ジョブ `content.fetch_link_card`（引数は投稿の ID）を同じトランザクションで outbox に積みます。リモートの投稿のジョブは 0〜60 秒のランダムな遅延を付けます。
  ジョブは実行時に `status_link_cards` の現在の URL を読むので、編集で URL が変わっても古い URL は取りません。公開範囲によらず取得します。
- **選ぶ URL**: 本文の最初の `http` / `https` のリンクで、メンション・ハッシュタグ・自サーバーのホスト（`host`・`account_domain`・`web_origin` のホスト）のものを除きます。
  ローカルの投稿は `ParseText` の区分で、リモートの投稿はサニタイズ済み HTML の `a` の `class`（`mention`, `hashtag`, `u-url`）・`rel="tag"`・`@` や `#` で始まるリンクの文字で、メンションとハッシュタグを見分けます。
  添付か投票のある投稿はカードを持ちません。URL は `content.NormalizeLinkURL`（スキームとホストを小文字に、既定のポート・フラグメント・利用者情報を除く。クエリはそのまま）で正規化し、カードはこの URL ごとに共有します。
- **取得**: `HTTPClient` に `HTTPPurposeHTML` で `GET` を送ります（`Accept: text/html, application/xhtml+xml`、`Accept-Language` はインスタンスの `default_locale`。Cookie と `Referer` は送りません）。
  SSRF ガード・リダイレクト・本文 1 MiB・30 秒は §1.1 と ADR-0019 §1 のとおりで、`https` 以外の URL は取得しません。状態コード 200 で `text/html` / `application/xhtml+xml` の応答だけを読みます。
- **解析**: `golang.org/x/net/html` のトークナイザで `<head>` だけを読み（`</head>` か `<body>` で止める）、文字コードは `golang.org/x/net/html/charset` で UTF-8 にします。
  題名は `og:title` → `twitter:title` → JSON-LD の `headline` → `<title>`、説明は `og:description` → `twitter:description` → JSON-LD の `description` → `<meta name="description">`、
  サイト名は `og:site_name` → JSON-LD の `publisher.name` → ホスト、作者は `article:author`（URL でないもの）→ `<meta name="author">` → JSON-LD の `author.name`、
  公開日時は `article:published_time` → JSON-LD の `datePublished`（RFC 3339 のもの）、画像は `og:image:secure_url` → `og:image` → `twitter:image`、言語は `<html lang>` です。
  JSON-LD は `<script type="application/ld+json">`（64 KiB まで）を JSON として読み、最上位か `@graph` の `NewsArticle` / `Article` / `BlogPosting` / `WebPage` の最初のものだけを使います。`@context` は取得しません。
  値はプレーンテキストにして、題名 200・説明 500・サイト名 100・作者 100・画像の代替テキスト 1,500 の書記素で切ります。`og:url` は応答の最終 URL と同じオリジンのときだけカードの URL にします。
  題名のないページはカードにしません。oEmbed・`twitter:player`・`og:video` の埋め込みは扱いません。
- **失敗と同時取得**: 取得・解釈の失敗は URL の SHA-256 をキーに KV（名前空間 `content.link_card_failure`）へ 24 時間記録し、その間は取りません。
  取得の前に URL ごとのロック（`content.link_card_lock`、60 秒）を取り、取得中なら 60 秒後に再試行します。
- **画像**: カードのホストと画像のホストのどちらにも `media_fetch` の制限がなければ、画像を `HTTPPurposeImage` で 1 回だけ取得し、中身で形式を判定して（§9 の許可リストのうち画像。SVG は拒否）、
  `media.Processor` で寸法と blurhash を求め、`media_files`（`origin = remote`、所有者なし）と `link_card_media` に記録します。本体は保存しません。同じ URL の画像は最初の記録を使います。
  クライアント API はメディアプロキシの `small`（添付と同じ S の箱の静止画）の URL だけを返し、プロキシは `link_card_media` を添付の画像と同じ規則で中継・キャッシュします。
- **`fediverse:creator`**: 最初の `<meta name="fediverse:creator">` を `RemoteActors.VerifyCreator` で WebFinger から解決し（10 秒まで）、Actor の文書を取り直して、
  その `attributionDomains` がカードの URL のホストかその親ドメインを含むときだけ、カードの作者のアカウントにします。ローカルのアカウントは検証しません。
- **処分**: カードの URL のホスト（要求の URL と最終 URL）に `federation` の制限（`suspend`）があれば、取得せず、既存のカードも出しません。
  `media_fetch` の制限（`reject_media`）がカード・画像のホストにあれば、画像を取得せず、既存の画像も出しません。投稿がセンシティブか、作者に `media_sensitive` の制限があれば、画像はセンシティブとして表示します。
  カードは連合しません。
- **保持**: 1 日 1 回のジョブ `content.cleanup_link_cards` が、どの投稿からも参照されず取得から 14 日を過ぎたカードと、どのカードも使わない画像（`link_card_media` と `media_files`）を消します（FR-803）。
- インスタンス設定 `link_cards_enabled`（既定 `true`）が偽のとき、ジョブは何もせず、API はカードを返しません。

## 10. コレクションとオブジェクトの GET

連合の向きが `publish` を通さないとき、投稿・`/activity`・コレクション（outbox・followers・following・featured）・カスタム絵文字・引用の承認の GET は `404` です（`410` にしません）。ローカルの Actor・インスタンス Actor・WebFinger・host-meta・NodeInfo の扱いは [§14.2](#142-入口ごとの振る舞い) にあります。

- 移転したローカルの Actor（`account_move_states` に行がある。ADR-0089）の文書は、移転を記録したトランザクションの後から取り消しまで、`movedTo`（移転先の Actor の `id`。`@context` の `as:movedTo`）を持ちます。Misskey は `Move` を受けると Actor を取り直し、この `movedTo` で移転を処理するので、`Move` より先に GET が返します。最小の文書（Authorized Fetch の署名なし、連合の向きが `inbound`）にも載せます。`inbound` でも `Move` を送るためです（§14.1）。エクスポートの Actor の文書も同じです。

- `GET /ap/objects/{uuid}`（`/activity` を含む）は、署名から閲覧者（署名者のドメイン）を決め、`content.CanView`（`ContextObjectFetch`）を通します。権限が無いときと存在しないときは同じ 404 です（ADR-0018）。削除済みは、削除の前に閲覧できた相手にだけ `410 Gone` と `Tombstone` を返し、それ以外には 404 を返します（[ActivityPub §6.4](https://www.w3.org/TR/activitypub/#delete-activity-outbox)）。
- 判定は `federation.Objects`（`internal/app/federation`）が行います。
  - 署名は §2 の受信の規則（`SignatureVerifier`）で検証します。署名が無い・検証できない GET は未ログインとして扱い、401 にはしません（Mastodon と同じ。401 にするのは Authorized Fetch だけ）。
  - 閲覧者のドメインは鍵の持ち主（`owner`）の Actor のホストです。`keyId` のホストと一致しないとき、ドメイン単位の `federation` の制限（§11）があるとき、自サーバーのときは未ログインとして扱います。
  - 関係は署名者のドメインで集約します（[visibility.md §1](visibility.md#1-入力)）。ブロックは署名者のアカウントと作者の間のものです。メンション先とフォロワーは、`CanView` が許さなかったときだけ読みます（フォロワーは 1,000 件ずつ、そのドメインのアカウントがいるかを `account.AnyOnDomain` で確かめる）。
  - 未ログインでも同じ結果になる応答（`public` / `unlisted`）だけ `Cache-Control: public, max-age=60` を付け、それ以外は `private, no-store` です。
  - HTML を求めるリクエスト（ブラウザ）は署名を持たないので、未ログインで閲覧できる投稿だけ表示用 URL へ `303` でリダイレクトし、それ以外は 404 です。
- 投稿の表示用 URL `/@{username}/status/{uuid}`（[ADR-0053](../adr/0053-status-page-url.md)）に ActivityPub の `Accept` で来たリクエストは、未ログインで閲覧できる削除されていないローカルの投稿だけ、その `id` へ `302` でリダイレクトし、それ以外は 404 です（`federation.Objects.PageStatusURI`）。
  URL を貼った検索（FR-305）で、ほかのサーバーが投稿を引けるようにするためです。`id` の GET は上の規則で改めて判定します。
  ローカルの投稿の `url`（`Note` とクライアント API）は保存せず、読むときに作者の現在のユーザー名と投稿 ID から作ります。
- `GET /ap/quote_authorizations/{uuid}` は、このサーバーが出した引用の承認の `QuoteAuthorization`（`attributedTo` は引用先の作者、`interactingObject` は引用元、`interactionTarget` は引用先）を `@context` 付きで返します（ADR-0069 §1）。
  引用元がすでに示しているものなので、だれにでも返し、`public, max-age=60` です（Authorized Fetch のときは署名を求める）。取り消した承認と、引用先が削除された承認は `410 Gone` と `Tombstone`、無いものは 404 です。
- `GET /ap/emojis/{uuid}` は、有効なローカルのカスタム絵文字の `Emoji` を `@context` 付きで返します（Mastodon は `id` を引いて絵文字を解決する。ADR-0051 §3）。存在しない・削除した・無効の絵文字と、HTML を求めるリクエストは同じ 404 です。絵文字と画像は公開のものなので、Authorized Fetch は当てず、`public, max-age=60` です。
- コレクションはだれにも同じ内容を返します。outbox・followers・following は `totalItems` と最初のページ（`?page=true`）へのリンクを持つ `OrderedCollection` で、ページ（`OrderedCollectionPage`）は `partOf` と次のページ（`?max_id={カーソル}&page=true`）を持ちます（Mastodon と同じ形）。
  - outbox は、未ログインの閲覧者がプロフィールで見られる投稿（`public` / `unlisted`）の `Create` を、新しい順に 20 件ずつ返します。ブーストは載せません。`totalItems` は削除されていない投稿の数です（Mastodon と同じ）。
  - followers / following は、承認済みのフォローの相手の Actor の `id` を、新しいフォロー順に 40 件ずつ返します。有効でないアカウントは省き、`totalItems` はフォローの件数です。
  - featured は、ピン留めした投稿のうち未ログインで閲覧できるもの（`public` / `unlisted`）の `Note` を `orderedItems` に並べます。featured-tags は空の `Collection` です。
- Authorized Fetch（FR-408、S）はインスタンス設定 `authorized_fetch`（既定 `false`、[data-model §4.9](data-model/04-tables/09-instance.md#49-instance)）で有効にします。設定はリクエストごとに読むので、変更はすぐに効きます。
  有効なときは、検証できる署名の無い GET（署名なし・検証できない・未ログインとして扱う署名者）に次のように答えます。

  | 対象 | 署名なし | 検証できる署名 |
  | --- | --- | --- |
  | 利用者の Actor | 鍵の取得のための最小の文書（`@context`・`id`・`type`・`preferredUsername`・`inbox`・`publicKey`・`endpoints`。Mastodon と同じ項目。移転済みなら `movedTo` も） | Actor の文書全体 |
  | インスタンス Actor | 文書全体（もともと最小の文書） | 文書全体 |
  | オブジェクト（`/activity` を含む）・コレクション | `401` | 無効なときと同じ判定 |
  | WebFinger・NodeInfo・host-meta・HTML を求めるリクエスト | 無効なときと同じ | 無効なときと同じ |

  - `401` は、存在するかどうかを確かめる前に返します（存在が漏れない）。ADR-0032 の GET の署名を求める `Accept-Signature: sig1=("@method" "@target-uri");created` を付けるので、double-knocking のクライアントは RFC 9421 で再送します。
  - 応答は署名によって変わるので、すべて `private, no-store` です。だれにも同じ文書を返すインスタンス Actor だけは `public, max-age=60` のままです。
  - インスタンス Actor の GET は署名を検証せずに答えるので、Authorized Fetch を使うサーバーどうしでも、インスタンス Actor の鍵で署名した取得を、相手がその鍵を取得して（§6 のとおり自分のインスタンス Actor の鍵で署名して）検証できます。鍵の取得が鍵の検証を待つ循環になりません。利用者の Actor の最小の文書は、署名せずに鍵を取得する実装のためのものです。
  - 無効なとき、Actor とコレクションの GET は署名を検証しません（だれにも同じ応答なので）。

## 11. モデレーションとの連携

`moderation` の投影（`account_restrictions`・`domain_restrictions`、[data-model §5.2](data-model/05-states-and-policies.md#52-処分の種類と制限能力の剥奪)）を、`moderation` の公開 API（`moderation.FederationPolicy`）から読みます。`federation` はテーブルを直接見ません。

| 制限 | 受信 | 送信 | 取得・表示 |
| --- | --- | --- | --- |
| ドメインの `federation`（`suspend`） | 署名の検証より前に `403`（§3.1） | 送らない | 取得しない。既存の投稿・アカウントは `profile_view` で隠す |
| ドメインの `public_discovery` / `unsolicited_reach`（`limit`） | 受け付ける | 送る | 連合タイムライン・検索に出さない。フォローしていない相手への通知を作らない |
| ドメインの `media_fetch`（`reject_media`） | 受け付ける | 送る | メディアプロキシが取得しない |
| ドメインの `report_intake`（`reject_reports`） | `Flag` を捨てる | — | — |
| ドメインの `media_sensitive`（`force_sensitive`） | 受け付ける | 送る | 添付をセンシティブとして表示 |
| アカウントの `federation`（`suspend` / `ban`） | リモートなら `Delete` / `Undo` / `Reject` 以外を捨てる | ローカルなら、下の「処分の伝播」のアクティビティ以外を送らない | ローカルの Actor の GET は、空のプロフィールに `suspended: true` を付けた Actor を返す（鍵は返す）。投稿などのオブジェクトの GET は `404` |

- 判定はサブドメインにも効きます（[data-model §4.3](data-model/04-tables/03-moderation.md#ドメイン単位の処分)）。
- プロフィールを開いたときの outbox の取得（§6.3、ADR-0044）は、アカウントの `federation` の制限があれば始めず、ドメインの `federation` の制限は `Fetcher` が止めます。ドメインの `public_discovery` の制限は止めません（利用者が自分で開いたプロフィールで、取り込んだ投稿は連合タイムライン・検索に出ない）。
- `Block` の受信は `graph` の関係として記録します（相手からのブロックで、モデレーションの処分ではありません）。
- `Move` の受信（§3.4、ADR-0089）は、移転元・移転先のアカウントかドメインに `suspend` があれば捨てます。処分は移転先へ引き継ぎません。移転元に `limit` があれば（`FederationPolicy.AccountLimited`、`visibility_ceiling_private` の制限）、`manage_accounts` を持つモデレーターに通知 `admin_account_moved` を出し、移転先に処分を下すかはモデレーターが決めます。
- `Flag` の受信は `reports`（`uri` は `Flag` の `id`、`id` が無ければ受信の記録と同じ `urn:my:digest:…`。同じ `Flag` の再処理は何もしない）と `report_states`（`open`）になり、モデレーターの通報一覧に入ります（FR-701）。コメントは 5,000 文字で切ります。
- `moderation.FederationPolicy` は、呼ばれるたびに期限内の制限を読みます（`DomainPolicy`・`AccountPolicy`・`content.MediaFetchPolicy` と、`Flag` の `RejectsReports`）。処分は `moderation.Service` の `IssueDomainAction` / `LiftDomainAction` と、アカウントへの `IssueAccountAction`（リモートのアカウントには制限だけを生む `limit` / `suspend` / `force_sensitive`）/ `LiftAccountAction` が記録します。
  ドメイン単位の処分の発行・解除・一覧と、通報の一覧・担当・解決は、管理 API（`/api/v1/admin/domain_actions`・`/api/v1/admin/reports`、権限 `manage_domains` / `manage_reports`）と CLI（`my admin domain` / `my admin report`）から行います（[運用手順](../operations/moderation.md)）。入力されたドメインは `CanonicalHost` で正規化してから渡します。
- `Block` / `Undo Block` の送信（§7.1）は、ローカルのアカウントがリモートのアカウントをブロック・解除したときだけです。`Block` の `id` はブロックの URI（`/ap/activities/{block_id}`）、宛先はブロックした相手だけで、相手の個人の inbox に送ります（shared inbox に集約しない。Mastodon と同じ）。
- 通報の転送（FR-709）は、ローカルの通報者が転送を選んだリモートのアカウントへの通報だけを、受け付けのトランザクションで outbox に積みます。
  `Flag` の主体はインスタンス Actor、`object` は対象の Actor の `id` と通報した投稿の URI、`content` はコメント、宛先は対象のアカウントの inbox だけです。通報者・IP・メールアドレスは載せません。
  `Flag` の `id` は `/ap/activities/{id}`（`outbound_activities` の ID）で、Mastodon と同じく `to` / `cc` を持ちません。文書は展開ジョブが組み立てるので、それまでに消えた投稿の URI は入りません。
  返信先の第三者のサーバーへは転送しません（[ADR-0036](../adr/0036-moderation-federation-propagation.md)）。

### 11.1 処分の伝播

処分が連合先に送るものは [ADR-0036](../adr/0036-moderation-federation-propagation.md) のとおりです。どれも処分のトランザクションで outbox に積み、§7 の配送とリトライに乗せます（NFR-MOD-3）。

| 処分 | 送るもの | 宛先 |
| --- | --- | --- |
| ローカルの `suspend` / `ban` | `Update(Actor)`（`suspended: true`、名前・アイコン・ヘッダー・自己紹介・プロフィール補足情報を空にする） | `Delete`（Actor）と同じ（§7.2） |
| ローカルの `suspend` の解除・期限切れ | `Update(Actor)`（元のプロフィール）。期限切れは、期限の後の最初の定期ジョブ（`moderation.expire_restrictions`、5 分ごと）が積む | 同上 |
| ローカルの `ban` | 上の `Update` と、`deletion_scheduled` の期日の `Delete`（Actor） | 同上 |
| ローカルの投稿への `delete_statuses` | 投稿ごとの `Delete` | 本人の削除と同じ（§7.2） |
| リモートのアカウントの `suspend` | そのアカウントのローカルの利用者へのフォローに `Reject(Follow)`、ローカルの利用者からのフォローに `Undo(Follow)` | そのアカウントの inbox |
| ドメインの `suspend` | 送らない。両方向のフォローを `follow_terminations`（`reason = moderation`）で切り、`relationship_severances` を記録する（ジョブ `moderation.sever_domain`） | — |
| それ以外（`limit`・`restrict_posting`・`force_sensitive` など） | 送らない | — |

- `suspended` は Mastodon 拡張（`toot:suspended`）で、`@context` は §7.4 の Actor と同じです。凍結中の文書は `federation.ActorDocument.Suspend` が作り、`id`・コレクション・inbox・鍵は残します。
- 凍結とその終わりは、`moderation` が処分・解除・期限切れの行の削除のトランザクションで発行する `moderation.account_suspension_changed`（[data-model §4.3](data-model/04-tables/03-moderation.md#制限投影)）から、`federation.RegisterSuspensionProjectors` が同じトランザクションで `Update`（Actor）を積みます。重なった凍結の 2 つ目と、1 つが残る解除は何も送りません。
  文書は展開の始めの時点の状態から組み立てるので（§7.4）、凍結と解除が続けて起きても、後から展開した `Update` ほど新しい状態を表します。
- BAN の `Delete`（Actor）は、削除の予約の期日に `account.execute_deletions` がアカウントの削除（`deleted`）を記録したときに、§7.1 のアカウント削除と同じ経路で送ります。凍結中でも `Delete` は止めません。
- リモートのアカウントの `suspend` は、凍結を始めた処分のトランザクションで `graph.SeverByModeration` が両方向のフォローを切り、その `FollowTerminated`（`reason = moderation`）から §7.1 の projector が `Reject`（そのアカウントのフォロー）と `Undo`（ローカルの利用者のフォロー）を積みます。
  ドメインの `suspend` で切るフォローの `FollowTerminated` は `Unannounced` で、何も積みません（配送も止まっています）。
- 処分を解除しても、切ったフォローは戻しません。
- 受信した Actor の `suspended: true` は、リモートのアカウントの表示に出すだけで、このサーバーの処分にはしません。

## 12. テスト

| 種類 | 内容 | 実行 |
| --- | --- | --- |
| リレー | `testkit.RelayContract`（登録・イベントと状態・照合・削除・再構築）、`testkit.RetentionContract` の `PurgesRelayedStatuses`、`testkit.CanonicalizerContract`（`internal/adapter/jsonld` の jsonld.js と同じ正規化と署名。`pkg/testkit/testdata/jsonld/sign.mjs` で作ったベクタ）、`internal/platform` の `TestRelays`（参加・`Accept`・送信と LD 署名・中継の取り直し・上限・LitePub の `Announce`・脱退・削除・監査ログ） | `scripts/task.sh check`（毎 PR） |
| フィクスチャの回帰テスト | 実サーバー（Mastodon・Misskey・GoToSocial・Akkoma）から採取したアクティビティ・Actor・オブジェクトの JSON（`pkg/testkit/fixtures/federation/<実装>/<版>/…`）を正規化・ハンドラに通し、結果（正規化した値・書いた行）を golden ファイルと比べる。送信する文書は主要実装の形と比べる（既存の `internal/interface/activitypub/testdata/peers`） | `scripts/task.sh check`（毎 PR） |
| 署名のテストベクタ | RFC 9421 付録 B のテストベクタ、Mastodon と同じ形式の draft-cavage の署名、クエリ文字列・ダイジェストの改ざん | `scripts/task.sh check` |
| 順序のテスト | 同じアクティビティ列の順序を入れ替え・重複させて処理し、最終的な行が同じになることを確かめる性質テスト（`testkit.InboxProcessingContract` の `OrderAndDuplicates`。ケースは `inboxOrderCases` で、ハンドラを足すときにケースも足す） | `scripts/task.sh check` |
| SSRF ガード | `testkit.HTTPClientContract`（ADR-0019 §1） | `scripts/task.sh check` |
| ライブの相互運用 | docker compose で Mastodon（secure mode）・GoToSocial・Misskey とこのサーバーを、私設 CA の TLS とコンテナのネットワークの DNS で立て、WebFinger・Actor の署名付き取得・フォロー（承認・解除）・投稿・返信・ブースト・お気に入り・削除・編集・公開範囲ごとの配送先（ADR-0035 の `local` のメンション先を含む）と、ADR-0032 の署名の各方式（プロキシが 401 を返して RFC 9421 への切り替えを起こす）を往復させ、ローカルのアカウントの凍結とその解除が Mastodon に表示されること（#44、§11.1）と、Activity-Relay のリレーを介して公開投稿が Mastodon と往復すること（Mastodon が LD 署名を検証する。§13）を確かめる。構成・シナリオ・実装ごとの互換性の差異は `deploy/docker/federation-live/`（README.md） | 遅い検査（リリースの関門・nightly）/ 手動（CI ジョブ `federation-live`、`scripts/task.sh docker:federation-live`） |

- フィクスチャは採取したときのホスト・名前・本文・鍵を例の値に置き換えて保存し、実装と版をディレクトリ名に残します。新しい不具合を直すときは、その文書をフィクスチャに足します（ADR-0006）。
- 置き換えと追加の手順は [pkg/testkit/README.md](../../pkg/testkit/README.md#federation-fixtures)、各ファイルの出典とライセンスは `pkg/testkit/fixtures/federation/<実装>/README.md` にあります。正規化の golden は `pkg/testkit/testdata/golden/normalize/` にあり、`go test ./pkg/testkit -update` で書き直します。
- `scripts/task.sh check` のテストはネットワークを使わず、`HTTPClient` をホスト名で振り分けるインメモリの実装に差し替えます（ADR-0015）。
- ライブの相互運用テストは、このサーバーが受信したアクティビティを保存したとおりに成果物へ書き出します。フィクスチャの採取元にする手順は [deploy/docker/federation-live/README.md](../../deploy/docker/federation-live/README.md#adding-a-captured-activity-as-a-fixture) にあります。

## 13. リレー

[ADR-0061](../adr/0061-federation-relays.md)（FR-411、FR-724）の実装です。運営者が登録したリレーにインスタンス Actor が参加し、`public` の投稿を送り、リレーが運んだ `public` の投稿を取り込みます。
リレーに参加していないインスタンスでは、受信の照合（下記）のほかに行もジョブも増えません。
連合の向き（[§14](#14-連合の向き)）が `off` のときは、参加・送信・受信をしません（登録と状態は残します）。`inbound` は受信だけ、`outbound` は送信だけです。参加・脱退の `Follow` / `Undo` は取り込みか公開が通るときに送り、リレーの `Accept` は `publish` として扱います。

### 13.1 参加と脱退

- 管理 API（`/api/v1/admin/relays`、権限 `manage_settings`）・CLI（`my admin relay`）・管理画面（`/admin/relays`）から、`federation.Relays` のユースケースを呼びます（[運用手順](../operations/relays.md)）。
- 登録は方式（`mastodon` / `litepub`）と URL です。URL は `https` で、`CanonicalIRI` で正規化します。ドメイン単位の `suspend` のあるホストは登録できません。
  `litepub` は登録のときに URL を取得し（インスタンス Actor の鍵で署名した GET、§6.1）、Actor の shared inbox（無ければ inbox）を配送先にします。方式を省くと、URL が Actor の文書を返せば `litepub`、そうでなければ `mastodon` です。
- 参加はインスタンス Actor の `Follow` です。`id` は `/ap/activities/{relay_events.id}`、`object` は `mastodon` なら `as:Public`、`litepub` ならリレーの Actor です。
  `relay_events`（`follow_sent`）・`relay_states`（`pending`）・`outbound_activities`（`object_type = relay`）・保存した本文・展開ジョブを 1 つのトランザクションで書きます。
- 脱退は `Undo(Follow)`（`left`）、参加し直しは新しい `id` の `Follow`（`rejected` か `left` から）です。登録の削除は `left` か `rejected` のときだけで、`relays` と子の行を消します。
- 状態の遷移は `federation.RelayStatus.Next` が決めます（[data-model §4.8](data-model/04-tables/08-federation.md#48-federation)）。`Accept` / `Reject` は、`object` の `id` がリレーの現在の `Follow` のものだけを受け付けます。
  主体は、`litepub` なら登録した Actor、`mastodon` なら inbox か shared inbox が登録の inbox と一致する Actor でなければ捨てます。`accepted` で主体を `relay_actor_uri` に記録します。同じ答えをもう一度受けても何も変えません。
- インスタンス Actor への `Follow` は、参加中のリレーの Actor からのもの（`litepub` のリレーはフォローし返す）に `Accept` を、それ以外に `Reject` を送ります。フォローとしては記録しません。
- 監査ログは登録・参加・脱退・削除（`relay_added`・`relay_followed`・`relay_unfollowed`・`relay_removed`）と、受け取った答え（`relay_accepted`・`relay_rejected`、`actor_kind = remote`）を `target_kind = relay` で記録します。

### 13.2 送信

- 展開ジョブの `Create` のフェーズの最後に、実効の公開範囲が `public` の投稿だけ、参加中（`accepted`）のリレーの inbox を加えます（§7.2）。`unlisted`・`private`・`direct`・`local` と、ブーストの `Announce` は送りません。
  `deliveries` に記録するので、`Update` / `Delete` は元の `Create` の `deliveries` に届きます（登録を消したリレーにも）。
- `public` の投稿の `Create` / `Update` / `Delete` には、すべての宛先に LD 署名を付けて送ります（§7.3）。
- リレーへの配送も、ほかの宛先と同じくドメインの状態（`suspend`・`unavailable`）で止まります。

### 13.3 受信

- 同期の部分（§3.1）は変えません。`refetch` のアクティビティと `Announce` だけ、署名者がどれかの参加中のリレーの Actor（`relay_states.relay_actor_uri`）かを引き、そうならジョブに `relay_id` を付けます。
- 非同期の部分は、`relay_id` の付いたアクティビティを、何かを取得する前にリレーの関門（`federation.Relays.Admit`）に通します。
  1. 登録が `receive = false`、またはリレーが `accepted` でなければ捨てる。
  2. リレーごとの 1 時間（UTC の時）の件数を KV（名前空間 `federation.relay_ingest`、キーは `<relay_id>:<YYYYMMDDHH>`、25 時間）で数え、設定 `relay_ingest_hourly_limit`（既定 1,000）を超えたら捨てる。
  3. `Create` / `Update` の `actor`、`Announce` の `object` のホストのドメインか、保存済みのアカウントに `federation` か `public_discovery` の制限があれば捨てる（`moderation.FederationPolicy.DiscoveryRestricted`）。`Delete` / `Undo` は捨てない。
- 通ったものは、`trust = refetch` のとおりオリジンから取り直して（§3.2）、ほかの受信と同じハンドラで扱います。
  - `Create`（と、§3.3 で `Create` として扱う `Update`）は、取り込み範囲（§6.3）に入らなくても、`public` なら取り込みます。返信先と未知のメンション先は取得せず、取り込みのトランザクションで `relayed_statuses` に記録します。
  - リレーの Actor の `Announce` は、`object` をいつもオリジンから取得し、作者の制限を確かめて、`public` なら取り込んで記録します。ブーストとしては記録しません。
- 取り込んだ投稿は、ほかのリモートの `public` の投稿と同じく連合タイムラインとハッシュタグのタイムラインに出ます。ホームには出ず、通知も作りません。
- 関門の結果はメトリクス `my.relay.activities`（`my.outcome` が `admitted` / `over_limit` / `restricted` / `not_receiving`）に出ます。管理画面と CLI は、直近 24 時間の時ごとの件数から、取り込んだ件数（上限まで）と上限で捨てた件数を示します。
- `relayed_statuses` の投稿は、`content.purge_relayed_statuses` が公開から設定 `relay_status_retention`（既定 7 日）で消します（[data-model §4.4](data-model/04-tables/04-content.md#リモート投稿の保持期間)）。脱退しても、取り込んだ投稿はすぐには消しません。

## 14. 連合の向き

[ADR-0076](../adr/0076-federation-direction.md)（FR-413）の実装です。インスタンス設定 `federation_mode`（`off` / `inbound` / `outbound` / `both`、既定 `both`）が、このサーバーが連合する向きを決めます。値はリクエストとジョブごとに読み、変更はすぐに効きます。

判定は `internal/domain/federation` の `federation.Allows(mode, Flow) bool` に集め、各入口は自分の扱う `Flow` を渡して聞くだけにします（モードの値では分岐しません）。応用層は `appfederation.ModeSettings` でモードを読み、`ErrFederationRestricted` で「モードが通さない」ことを返します。

### 14.1 流れの分類

| `Flow` | 内容 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- | --- |
| `publish` | ローカルの投稿・プロフィール・反応を外へ出す（配送と GET） | × | × | ○ | ○ |
| `ingest` | 外の投稿・反応を取り込む（受信と取得） | × | ○ | × | ○ |
| `follow_out` | ローカルの利用者が外のアカウントをフォローする往復（`Follow` / `Undo Follow` の送信、`Accept` / `Reject` の受信、`Move`） | × | ○ | × | ○ |
| `follow_in` | 外のアカウントがローカルの利用者をフォローする往復（`Follow` / `Undo Follow` の受信、`Accept` / `Reject` の送信） | × | 断る | ○ | ○ |
| `retract` | すでに外へ出したものの撤回（投稿の `Delete`、Actor の `Delete`、`Undo Like` / `Undo Announce` / `Undo EmojiReact`、凍結の `Update(Actor)`）と、移転の知らせ（`Move` の送信、`movedTo` 付きの `Update(Actor)`。ADR-0089） | × | ○ | ○ | ○ |
| `protect` | 保護のための手続き（`Block` / `Undo Block` と `Flag` の送受信、外の Actor の自分自身の `Update` / `Delete` の受信） | × | ○ | ○ | ○ |
| `identify` | 署名の検証のための鍵の取得と提供（外の Actor の取得、ローカルの Actor・インスタンス Actor の GET、WebFinger・host-meta・NodeInfo） | × | ○ | ○ | ○ |

- `inbound` の `follow_in` の「断る」は、受けた `Follow` に `Reject` を返すことです。承認待ちを作りません。この `Reject` は対応するフォローの行が無いので、`outbound_activities.object_type = 'refusal'` として記録して配送します（[data-model §4.9](data-model/README.md)）。
- `retract` は、`inbound` に狭めたあとも、それまでに外へ出したものを消せるように通します。宛先は元の配送（`deliveries`）とその時点のフォロワーだけで、共有 inbox やリレーのフェーズは付けません。
- ドメインの処分（§11）はモードと独立に効きます。モードが通しても、処分が止めるものは止まります。

### 14.2 入口ごとの振る舞い

| 入口 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- |
| inbox（同期、§3.1） | 署名の検証より前に `403` | 受け付ける | 受け付ける | 受け付ける |
| inbox（非同期、§3.2） | — | 下記の型だけ処理し、ほかは `dropped` | 下記の型だけ処理し、ほかは `dropped` | すべて処理する |
| 展開と配送（§7.2・§7.3） | すべて `skipped` | `follow_out`・`follow_in` の `Reject`・`retract`・`protect` だけ送る | `publish`・`follow_in`・`retract`・`protect` だけ送る | すべて送る |
| 外の取得（§6） | しない | する | 署名の検証と配送の宛先に要る Actor だけ。メディアプロキシはフォロワーの Actor の画像だけ | する |
| ローカルの Actor の GET（§10） | `404` | 鍵の取得のための最小の文書（Authorized Fetch の署名なしと同じ項目） | 全体を返す | 全体を返す |
| 投稿・`/activity`・コレクション・カスタム絵文字・引用の承認の GET（§10） | `404` | `404` | 返す | 返す |
| インスタンス Actor の GET | `404` | 返す | 返す | 返す |
| WebFinger・host-meta | `404` | 答える | 答える | 答える |
| NodeInfo | `404` | 答える | 答える | 答える |
| リレー（§13） | 参加・送信・受信をしない | 受信だけ | 送信だけ（受信は捨てる） | 送受信 |

`off` では、ローカルの Actor・投稿・コレクションがどれも `404` です（`410` にしません）。NodeInfo は `metadata.federationMode` にモードを出します。

非同期の inbox は、型を次の流れに対応させます。

| 型 | 流れ |
| --- | --- |
| `Follow`（ローカルのアカウントへ） | `follow_in`（`inbound` は断る） |
| `Accept` / `Reject` | `follow_out`、リレーの参加への応答、`QuoteRequest` への応答のどれかが通れば進み、ハンドラが自分の流れを確かめる |
| `Undo` | 取り消す型の流れ（`Follow` は `follow_in`、`Block` は `protect`、`Accept` は `follow_out`、反応は `ingest`） |
| `Block`・`Flag`、外の Actor の自分自身の `Update` / `Delete` | `protect` |
| `Like`・`EmojiReact` | `ingest` と `publish` の両方（外から見えないローカルの投稿への反応は捨てる） |
| `Announce` | `ingest`。ローカルの投稿のブーストは `publish` も要る |
| `QuoteRequest` | `publish`（取り込みが通らないモードでは `Reject` を返す） |
| `Move` | `follow_out` |
| `Create`・`Update`・`Delete`・`Add`・`Remove`、ほか | `ingest` |

- `inbound` で外の文書に `Create` などが埋め込まれていても、取り込みの範囲は §6.3 のままです。モードは範囲を広げません。
- `outbound` で捨てるものには、ローカルの投稿への返信・メンション・ブースト・お気に入り・絵文字リアクション・投票への回答・引用の依頼が含まれます。外の反応は数にも通知にも出ません。
- `inbound` でも、ローカルの利用者へのメンション（フォロー中の相手から）は `ingest` として取り込みます。

### 14.3 切り替え

- モードを狭めても、フォロー関係・取り込んだ投稿・配送の記録は消さず、`Undo` / `Reject` も送りません。流れが止まるだけで、広げ直すとそのまま再開します。
  - 狭めている間に作られた投稿・反応は、広げ直しても遡って配送しません（展開は実行の時点のモードで決まり、`skipped` は再送しません）。
  - 展開と配送のジョブは実行の時点のモードで判定するので、狭める前に積んだジョブが、狭めた後に送られることはありません。
  - `off` の間の削除は外に伝わりません。
- 関係を切るには、CLI `my admin federation sever --direction in|out|both` を使います（[運用手順](../operations/instance-settings.md)）。ローカルのアカウントと外のアカウントのあいだの承認待ちと有効なフォローを `follow_terminations`（`reason = moderation`、ADR-0036）で終わらせ、そのときのモードが送るときだけ `Reject`（`in`）・`Undo`（`out`）を記録します。モードの変更が自動で呼ぶことはありません。
- モードの変更は `instance_setting_revisions` に記録し、監査ログ（ADR-0037）に `target_kind = setting` で残します。
