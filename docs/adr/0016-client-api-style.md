# ADR-0016: クライアント API の様式と生成

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #9, #32, #89, #868

## Decision（決定）

### 1. REST + OpenAPI 3.1 をスキーマファーストで採用する

- `api/spec/` の分割したファイルを正とし（§4）、バンドルした `api/openapi.yaml` から Go のサーバーインターフェイスと TS クライアントを生成する。生成物と仕様のずれは CI で検査する。
- パス: `/api/v1/...`。安定性の約束と破壊的変更の扱いは ADR-0090 §8 に従う。
- ページング: UUIDv7 によるキーセットページングで、不透明なカーソル（`cursor` / `next_cursor`）を返す。
- エラー: RFC 9457（`application/problem+json`）。`title` / `detail` は `Accept-Language` に応じてローカライズし、機械判定用に安定した `type` を持たせる。
- リアルタイム: Server-Sent Events（`/api/v1/streaming`）。WebSocket は必要になってから。
- 認可: OAuth 2.0 Authorization Code + PKCE。自前クライアントはファーストパーティとして登録済みのものを使う。スコープは独自に定義する。
- 匿名で取れる公開リソースの GET には `Cache-Control` を付け、CDN でキャッシュできるようにする。

**覆す条件**: ストリーミングや双方向通信の比重が大きい、あるいは OpenAPI の生成ツールの品質が Go 側で不足する場合は Connect RPC が有力。

### 2. Mastodon Client API とは互換させない

**Mastodon Client API（Streaming API、アプリ登録 API を含む）とは一切互換させず、独自のクライアント API だけを提供する。**

- 自前の Web クライアント（PWA）が公式クライアントで（モバイルアプリは v0 の範囲の外）、API は自前クライアントの要件に合わせて設計する。
- 認可に OAuth 2.0（Authorization Code + PKCE）を使うのは、サードパーティのクライアントも同じ形で受け入れるためである（ADR-0090）。
- **連合（ActivityPub / S2S）は従来どおり Mastodon などと相互運用する**（[ADR-0006](0006-federation-protocol-scope.md)）。この決定はクライアント API にだけ適用される。

### 3. コード生成

- **Go: oapi-codegen の strict server**（`net/http`）。
  - OpenAPI 3.1 への対応範囲が ogen より広い。
  - ADR-0019 のミドルウェア構成（認証・レート制限・CSRF）に組み込みやすい。
- **TS**
  - `openapi-typescript`: スキーマから型だけを生成する。
  - `openapi-fetch`: 薄い fetch ラッパー。ミドルウェアで Bearer トークンを付け、401 のときにトークンを更新する。
  - `openapi-react-query`: TanStack Query との接続に使う。
- **SSE（`/api/v1/streaming`）**: どちらの生成ツールもサーバー側を生成しないので、手書きのハンドラにする。
  - `EventSource` はヘッダを送れないので、クライアントは fetch のストリームで読む。
- **CI**
  - `scripts/task.sh api:generate` の後に生成物の差分がないことを検査する（`api:generate-check`）。
  - スキーマを lint する。
  - Go のハンドラの応答をテストでスキーマと照合する（kin-openapi の `openapi3filter`）。
- 生成ツールが扱える 3.1 の部分集合だけを使い、lint で強制する（`$dynamicRef`、`if` / `then`、`prefixItems` などは使わない）。
- API を変えるときは、`api/spec/` を先に編集し、`scripts/task.sh api:generate` でバンドルと生成物を同じ PR で更新する。どの生成物をコミットするかは ADR-0081 が決める。

### 4. 仕様のファイルの配置

パス 1 つ・`components` の項目 1 つを 1 ファイルにした `api/spec/` を正とし、Redocly CLI の `bundle` で作った `api/openapi.yaml` を生成物としてコミットする。生成器・`apitest`・第三者はバンドルを読む。

```text
api/
  spec/
    openapi.yaml                        # ルート: info・servers・security・tags と、paths・components の $ref の一覧
    paths/<パス>.yaml                    # Path Item 1 つ。名前はパスの先頭の / を除き / を _ にしたもの（/statuses/{id}/favourite → statuses_{id}_favourite.yaml）
    components/schemas/<名前>.yaml       # スキーマ 1 つ。ファイル名は components の名前と同じ
    components/responses/<名前>.yaml
    components/parameters/<名前>.yaml
    components/headers/<名前>.yaml
  openapi.yaml                          # 生成物（redocly bundle api/spec/openapi.yaml）。手で編集しない
  redocly.yaml                          # lint の設定。api/spec/openapi.yaml を検査する
  api.go                                # openapi.yaml（バンドル）を埋め込む
```

- 名前の規則は `redocly split` の出力と同じにする。パスやスキーマの名前からファイルが 1 つに決まる。
- `securitySchemes` はルートに置く（1 つしかなく、変わらない）。
- 分割したファイルはそれぞれ `# SPDX-License-Identifier: MIT` で始める（ADR-0093 §2）。

### 5. `$ref` の書き方

- ルートは、`paths` のパスごとに `$ref: ./paths/<パス>.yaml` を、`components` の種類ごとにすべての項目を `<名前>: {$ref: ./components/<種類>/<名前>.yaml}` で列挙する。
  列挙しない項目は、どこからも参照されないとバンドルから落ちる（`WebhookMessage` など）。
- パスと `components` のファイルは、`components` を相対パスのファイルで参照する（`../components/schemas/Status.yaml`、同じ種類の中では `./Status.yaml`）。
  ルート以外のファイルで `#/components/...` は書かない（そのファイル自身を指すため）。
- 1 ファイルは 1 つのオブジェクトで、JSON Pointer の断片（`file.yaml#/...`）で他のファイルの中を指さない。パスのファイルは他のパスのファイルを参照しない。
  リモートの URL への `$ref` は書かない。
- 2 か所以上で使う型は `components` の 1 ファイルにする。
- ルートの `paths` はタグのまとまりの順に並べ、新しいパスはそのタグのまとまりの末尾に足す。`components` は種類ごとに名前の順に並べる。
- YAML のコメントはバンドルで消える。クライアントに伝えることは `description` に書き、コメントは保守のための短い注記だけにする。区切りのコメントは置かない。

### 6. 生成と検査

- `scripts/task.sh api:generate`（`scripts/generate.sh`）は、まず `api/spec/openapi.yaml` を Redocly CLI（`tasks/api.yml` で固定した版、今は 2.57.0）で `api/openapi.yaml` にバンドルし、
  「生成物であり `api/spec/` を編集する」旨の SPDX 付きの見出しを付ける。続けて、oapi-codegen（`api.gen.go`）、`securitygen`（`security.gen.go`）、openapi-typescript（`schema.gen.ts`）の生成をバンドルから行う。
- `scripts/task.sh api:generate-check` は、バンドルも一時ディレクトリに作り、コミット済みの `api/openapi.yaml` との差分を他の生成物と同じように検査する。
- `scripts/task.sh api:lint` は `api/spec/openapi.yaml` を lint し、問題の場所を分割したファイルで示す。
  `rule/default-response-is-problem` の正規表現は `../components/responses/Problem.yaml` への参照に合わせる。
  ルートが列挙しないファイルが `api/spec/components/` にあれば失敗させる検査を置く（`api:spec-check`。バンドルから黙って落ちるのを防ぐ）。
- `api/api.go`・`apitest`・`securitygen`・oapi-codegen と openapi-typescript の設定は分割の影響を受けない。新しい依存は足さない（Redocly CLI は lint ですでに使う）。
- `.gitattributes` で `api/openapi.yaml` に `linguist-generated=true` を付け、PR の差分ではたたむ。競合したときは手でマージせず `scripts/task.sh api:generate` で作り直す（他の生成物と同じ）。

### 7. 分割の覆す条件

- oapi-codegen が分割したファイルを 1 つのパッケージに直接生成でき、kin-openapi と openapi-typescript も同じ結果になるなら、バンドルをコミットしない形を再検討する。
- バンドルが元の spec と食い違う不具合が Redocly CLI に見つかり、版を戻しても避けられないなら、1 ファイルの spec に戻す。

## 経緯

Mastodon Client API に互換させれば既存クライアント（Ivory, Tusky, Elk, Phanpy など）を使えるが、Mastodon 固有のデータ表現・ページング・エンティティ構造への変換が大量に要り、公開範囲 `local`（[ADR-0017](0017-visibility-local.md)）のような独自機能とも衝突するので、§2 を決めた。

サーバーは Go、クライアントは TypeScript なので、両言語で型を共有し、エージェントが API を変更してもずれを機械的に検出できる必要がある。読み取りを CDN / HTTP キャッシュに載せやすいこと（NFR-COST）、リアルタイム更新の手段があること、サードパーティ開発者にとっての分かりやすさ、生成ツールの成熟度から、§1 を決め、続けて生成ツールを §3 に決めた。

`api/openapi.yaml` は 10,697 行・約 409KB まで育ち、main の 1,583 コミットのうち 100 が変え、並行作業の競合の多くがここで起きた。スパイク（[#868 の報告](https://github.com/mycroblog/my/issues/868#issuecomment-6319895238)）で次を確かめて（2026-10-22）、§4 から §7 を決めた。`redocly split` の出力そのままのバンドルは参照されないスキーマを落とし、ルートが全 `components` を列挙すれば元とデータとして一致する。oapi-codegen v2.8.0 は分割したルートを直接読むと `fatal error: stack overflow` で失敗する。`redocly bundle` は約 0.1 秒である。第三者が 1 ファイルの spec を読めることと、`go build` / `go test` が Bun なしで動くことも条件にした。

## 採らなかった案

- Mastodon Client API 互換（サブセット）+ 名前空間を分けた拡張 API: 既存クライアントが使えるが、変換層が大きく独自機能と衝突する。
- Connect RPC: 型の一貫性が強くストリーミングも標準だが、GET キャッシュは限定的である。§1 の覆す条件の移り先。
- GraphQL（gqlgen）: クライアント主導で取得できるが、キャッシュと N+1・認可の扱いが重い。
- ogen: JSON の処理が性能の問題になれば切り替えを再検討する候補だが、OpenAPI 3.1 の対応範囲が oapi-codegen より狭い。
- `@hey-api/openapi-ts`、orval: `openapi-typescript` + `openapi-fetch` のほうを選んだ。
- spec を分割しない: 仕組みが増えないが、ファイルの大きさと競合は解けない。
- タグごとに 1 ファイル: ファイルが約 30 で関連する操作を並べて読めるが、同じタグの PR が競合し（`admin-server` は 54 操作）、JSON Pointer の参照は読みにくい。
- 項目ごとに分けバンドルをコミットしない: 生成物の競合も減るが、バンドルの解決器（Redocly）と照合の解決器（kin-openapi）が別になり、第三者が読む 1 ファイルも失う。

## Consequences（影響）

- 変換層が無くなり、ドメインモデルをそのまま API に出せる。独自機能を自由に追加できる。
- 既存の Mastodon アプリは使えないため、v0 では **自前クライアントが必須** になる。モバイル体験は PWA が担う（ADR-0028）。
- サードパーティクライアントのエコシステムはゼロから。API スキーマと認可フローを公開できる品質で保つ。
- API の変更はまず `api/spec/` を編集する、というルールをエージェントに課せる。
- REST の設計判断（リソース粒度・命名）を自分たちで決める必要がある。
- JSON の処理が性能の問題になったら、ogen への切り替えを再検討する。
- 操作やスキーマを足す PR は、新しいファイルとルートの数行だけを変える。別のリソースを変える PR どうしは、ルートの隣り合う行を除いて競合しない。
- ファイルの名前から場所が決まり、エージェントは必要なファイルだけを読める。
- 生成物の `api/openapi.yaml` は競合する。解き方は作り直しで機械的だが、なくなりはしない。PR の差分は、編集したファイルとバンドルの両方に出る。
- 約 470 ファイルになり、ルートは 470 行ほどの `$ref` の一覧になる。spec 全体の検索は `api/openapi.yaml` で行える。
- `api:generate` が `bunx` で Redocly CLI を取得する（`api:lint` と同じ）。オフラインでは、キャッシュがないと生成できない。

## Open Questions / 要検証

- TS クライアント生成（openapi-typescript + openapi-fetch）の Tauri での動作
