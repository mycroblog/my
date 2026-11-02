# ADR-0093: API クライアントライブラリ pkg/mykit（Go・TypeScript）

- Status: Accepted（§1 の「ルートに `go.work` を置かずに」を ADR-0135（Dependabot の Go の更新を go.work でまとめる）が置き換える）
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1065

## Context（背景・課題）

クライアント API（FR-501、[ADR-0007](0007-client-api.md)・[ADR-0016](0016-client-api-style.md)）は `api/spec/` を正とし、バンドル `api/openapi.yaml` から
Go のサーバー（oapi-codegen の strict server）と、`app/web` の中だけで使う TS の型（openapi-typescript + openapi-fetch）を生成する
（[ADR-0029](0029-api-code-generation.md)・[ADR-0066](0066-openapi-split-and-bundle.md)）。外部の開発者が使えるクライアントライブラリはない。
Bearer の付与・401 での更新・問題の詳細の解釈（`app/web/src/lib/api/client.ts`）と、SSE の読み取りと再接続（`app/web/src/lib/sse/`）は Web クライアントの中にだけある。

決定者は M5 の作業として、`pkg/mykit` に Go と TypeScript（npm）のクライアントライブラリを置いて公開し、
その上にボット制作ライブラリ `pkg/mybot`（[ADR-0094](0094-bot-library-mybot.md)、#1066）を作ると指示した。
外部に公開する以上、次をまとめて決める必要がある。

- 置き場所とモジュールの単位。利用者がサーバーの依存（DB ドライバ・クラウド SDK）を引き込まないこと。
- ライセンス。[ADR-0011](0011-license.md) はリポジトリ全体を AGPL-3.0-only とするが、クライアントライブラリが AGPL だと、それを組み込むボットやアプリにも AGPL が及ぶ。
- 生成の仕方と `app/web` との関係、公開する API の範囲と形、認証・ストリーミング・実行環境・版・公開・テスト・文書。

サードパーティのクライアントの登録と API の安定性は [ADR-0090](0090-third-party-oauth-apps.md)（#1038）が、リリースの仕組みは #1029 が、英語の文書は #1031 が扱う。
この ADR はそれらに依存する部分を明記し、ライブラリそのものの形を決める。

## Decision Drivers（判断基準）

1. **利用者が使える条件**: ボットやアプリの作者が、自分のコードのライセンスを縛られずに組み込めること。サーバーの依存を引き込まないこと。
2. **仕様とのずれがないこと**: 型は `api/openapi.yaml` から生成し、ずれは CI で検出する（ADR-0029 の方針の延長）。生成を 1 か所にまとめる。
3. **薄さ**: 生成物の上に被せる手書きは、どの利用者も書き直すことになる横断の処理（認証・エラー・ページング・再試行・SSE）だけにする。保守する量を増やさない。
4. **既存の構成との整合**: Bun ワークスペース（[ADR-0026](0026-client-workspace-and-component-architecture.md)）・go-task の文脈・depguard・ライセンス検査・`CGO_ENABLED=0`。
5. **公開の安全性**: npm と Go のタグを、長く生きる秘密なしに、来歴を示して出せること。

## Considered Options（選択肢）

論点ごとに比べる。採るのは各論点の 1。

### A. 置き場所とモジュール

1. `pkg/mykit/` を Go の別モジュール（`github.com/mycroblog/my/pkg/mykit`）のルートにし、TS のパッケージを `pkg/mykit/ts/` に置く
2. 言語ごとに対称に `pkg/mykit/go/`（別モジュール）と `pkg/mykit/ts/` に置く
3. Go はルートの `go.mod` の中のパッケージ `pkg/mykit` にする（`pkg/testkit` と同じ）

### B. ライセンス

1. `pkg/mykit`・`pkg/mybot` と、その生成元の `api/spec/`・`api/openapi.yaml` を MIT にする（ADR-0011 の「リポジトリ全体」の一文を、この範囲について置き換える）
2. 同じ範囲を Apache-2.0 にする
3. AGPL-3.0-only のままにする

### C. コード生成

1. Go は oapi-codegen の client（型とクライアント）を生成し、手書きの薄い層を被せる。TS は openapi-typescript の型 + openapi-fetch に手書きの層を被せる。生成物はコミットし、`api:generate` の差分検査に入れる。`app/web` は型と横断の処理を `mykit` から使うように寄せる
2. 生成せずに手書きのクライアントを書く
3. 生成器を別のもの（Go は ogen、TS は Hey API など）にする

## Decision（決定）

A-1・B-1・C-1 を採用する。細部は次のとおり。

### 1. 配置とモジュール

```text
pkg/mykit/
  go.mod              # module github.com/mycroblog/my/pkg/mykit（別モジュール。タグは pkg/mykit/vX.Y.Z）
  LICENSE             # MIT
  README.md           # 英語。Go と TS の両方の入口
  client.go …         # package mykit（手書きの層）
  api.gen.go          # oapi-codegen の client と型（生成物）
  ts/
    package.json      # @mycroblog/mykit
    LICENSE           # MIT（npm の tarball に入れるため複製）
    src/…             # 手書きの層と schema.gen.ts（生成物）
  integration/
    go.mod            # 公開しない試験用モジュール（サーバーと mykit を replace で参照）
```

- **Go は別モジュール**にする。利用者の `go.sum` と依存のグラフにサーバーの依存が入らない。
  import のパスは `github.com/mycroblog/my/pkg/mykit`、パッケージ名は `mykit`。版のタグはサブディレクトリ付きの `pkg/mykit/vX.Y.Z`
  （Go Modules Reference「Module paths」「Mapping versions to commits」、2026-10-23 に go.dev/ref/mod で確認）。
  `pkg/mykit/go/` にしない（A-2）のは、import のパスの末尾が `go` になり、パッケージ名とずれるため。代わりに Go のモジュールの zip に `ts/` の TS のソースが入る（コミットしたファイルだけで、`node_modules` は入らない）。
- `mykit` の `go.mod` は `github.com/mycroblog/my`（サーバーのモジュール）を require しない。これを検査する（下の §8）。
  別モジュールなので、Go の `internal` の規則は効かない。代わりにモジュールの境界が依存を止める。`mykit` は標準ライブラリと、生成物が要る依存だけを使う。生成物をビルドに含めるサードパーティのモジュールは `github.com/oapi-codegen/runtime`（Apache-2.0）・`github.com/apapsch/go-jsonmerge/v2`（MIT）・`github.com/google/uuid`（BSD-3-Clause）の 3 つで、`go.sum` は 23 行になる。runtime の `go.mod` は gin・echo・iris などのフレームワークを require するので、利用者の `go list -m all` には出るが、ビルドには入らない（2026-10-23 に runtime v1.7.0 で、別モジュールとして生成物をビルドして確認）。
- 開発では、ルートに `go.work` を置かずに、`integration/` の試験用モジュールだけが `replace` でサーバーと `mykit` を結ぶ。`tools/go.mod` と同じく、ルートの `./...` からは外れる。
- **TS は `pkg/mykit/ts/`** に置き、npm のパッケージ名は **`@mycroblog/mykit`**（`app/` の私用の `@my/*` とは別の公開のスコープ）。npm の組織 `mycroblog` は決定者が作って所有する。`@mycroblog/mykit`・`@mycroblog/mybot` はレジストリになく、スコープ `mycroblog` の公開のパッケージもない（2026-10-23 に registry.npmjs.org で確認）。組織の名前が取れなければ、そのときに別のスコープを決める。
  Bun ワークスペース（ルートは `app/`）の `workspaces` に `../pkg/mykit/ts` を足し、`app/web` は `workspace:*` で使う。Bun はルートの外のパスをワークスペースとして扱い、`app/bunfig.toml` の `linker = "isolated"` でも `../pkg/mykit/ts` の依存（openapi-fetch）を解決する。`bun.lock` には `../pkg/mykit/ts` の項が入り、`pkg/mykit/ts/node_modules` ができる（2026-10-23 に Bun 1.4.3 で、`app/` と同じ設定の最小のワークスペースを作って確認）。
- `pkg/mybot`（ADR-0094）も同じ形（`pkg/mybot/` が Go の別モジュール、`pkg/mybot/ts/` が `@mycroblog/mybot`）にし、`mykit` だけに依存する。

### 2. ライセンス

- `pkg/mykit/`・`pkg/mybot/` と、生成元の `api/spec/` とそのバンドル `api/openapi.yaml` を **MIT** にする。生成物は仕様から機械的に作るので、仕様が AGPL のままだと生成物の扱いが曖昧になる。仕様を同じライセンスにしてそれを避ける。
  サーバーの実装（`internal/`・`cmd/`・`app/web` など）は AGPL-3.0-only のまま。MIT は AGPL-3.0 と両立するので、サーバーと Web クライアントが MIT のコードを使うことに問題はない。
- **ADR-0011 との関係**: ADR-0011 の Decision「リポジトリ全体を AGPL-3.0 とする」の一文だけを、上の範囲について置き換える。
  ADR-0011 は Accepted のままで本文は書き換えず、Status の行にこの ADR が範囲を限って置き換えることを注記する（ADR-0066 と同じ扱い）。AGPL 第 13 条のソースの提示などその他の決定は変えない。
- 置き方: それぞれのディレクトリに MIT の `LICENSE` を置き、ルートの `README.md` のライセンスの節に例外の範囲を書く。
  SPDX ヘッダは `MIT`（`// SPDX-License-Identifier: MIT`、YAML は `# SPDX-License-Identifier: MIT`）。生成物の見出しも `MIT` にする。
- 既存の `api/spec/` の著作者は決定者 1 人（`git shortlog` で確認）なので、決定者の判断で再ライセンスできる。Dependabot の変更は `api/spec/` に及ばない。
- **依存のライセンスの検査**: `mykit`・`mybot` の依存は MIT・Apache-2.0・BSD・ISC など寛容なものだけにする。AGPL と両立するだけの（GPL 系の）依存は利用者に及ぶので入れない。
  Go は `go-licenses`、npm は `tools/i18n` の検査と同じ仕組みに、寛容なものだけの許可リストを渡す。
- B-2（Apache-2.0）も条件を満たすが、B-1 にする。本文が短く、Go と npm の小さなライブラリで最も広く通るため。

### 3. コード生成

- 生成は `api/openapi.yaml`（バンドル）からだけ行い、`scripts/generate.sh` の対象に足す。生成物はコミットし、`api:generate` の差分検査に入れる（ADR-0029・ADR-0066 のまま）。
  - Go: oapi-codegen（`tools/go.mod` の同じ版）で `generate: {models: true, client: true}` を `pkg/mykit/api.gen.go` に生成する。サーバーと同じく `always-prefix-enum-values` にする。
    SSE の `streamEvents` は生成から外し、手書きする。
    この設定で今の `api/openapi.yaml` から client を生成でき、`CGO_ENABLED=0` でビルドと `go vet` が通る（2026-10-23 に `tools/go.mod` の oapi-codegen v2.8.0 で確認。生成物は約 7.6 万行・3.1 MB）。
    操作ごとに `<Op>WithResponse` が型付きの応答（`JSON200 *Status`・`ApplicationproblemJSONDefault *Problem`・応答ヘッダ）を返す。multipart の操作（`UploadMedia`）は `content-type` と `io.Reader` を受ける `<Op>WithBody` だけになるので、下のアップロードの関数で補う。
  - TS: openapi-typescript で `pkg/mykit/ts/src/schema.gen.ts` を生成する。`app/web/src/lib/api/schema.gen.ts` は消し、`app/web` は `@mycroblog/mykit` の型を使う。型の生成は 1 か所になる。
- **手書きの層**は次の横断の処理だけにする。エンドポイントごとの便利な関数は書かない（生成したクライアントをそのまま公開する）。
  - 認証（§5）、`Accept-Language` の付与、`User-Agent`（Go）。
  - **エラー**: 2xx 以外の応答を RFC 9457 の問題の詳細の型 `Problem`（`type`・`title`・`status`・`detail`・拡張）を持つエラーにする（Go は `*mykit.Problem` を `errors.As` で取り出せる型、TS は `ProblemError`）。機械判定は `type` で行う。ネットワークの失敗は別の型にする。
  - **ページング**: `next_cursor` をたどる反復子（Go は `iter.Seq2[T, error]`、TS は `AsyncIterable<T>`）。止めるのは利用者。
  - **レート制限と再試行**: 429 と 503 の `Retry-After` に従って待ち、上限回数まで再試行する。再試行するのは安全なメソッド（GET・HEAD）と、`Idempotency-Key` を付けた要求だけ。既定は有効で、無効にできる。
  - **冪等キー**: spec が `Idempotency-Key` を受ける操作では、利用者が渡さなければライブラリが UUID を作って付け、再試行で同じ値を送る。
  - **メディアのアップロード**: multipart の組み立てを `io.Reader`（Go）・`Blob`（TS）から行う関数を 1 つ置く。
- **`app/web` の移行**: `app/web/src/lib/api/client.ts` の Bearer・401 での更新・エラーと `lib/sse/` の読み取りと再接続を `@mycroblog/mykit` に移し、`app/web` には文言の整形（i18n）・`lib/network` への報告・TanStack Query との接続（openapi-react-query）を残す。
  `mykit` は文言を持たない（i18n は利用者の仕事。サーバーがローカライズした `title`・`detail` はそのまま渡す）。

### 4. 公開する API の範囲

- spec の全操作を生成し公開する。`admin` スコープの操作（`/admin/*`、ADR-0058）も同じクライアントに含める。分けるとタグごとの生成の設定が増え、権限はサーバーがスコープと権限で判定するので、ライブラリで分ける利点が小さい。
  `admin` 系のスコープはファーストパーティのクライアントにだけ与えられる（[ADR-0090](0090-third-party-oauth-apps.md) §4）ので、サードパーティのトークンでこれらの操作を呼ぶと 403 になる。生成物の doc コメントと README にそのことを書く。
- API の安定性は ADR-0090 §8 に従う（追加を既定とし、破壊的変更は `deprecated: true` と `Deprecation`・`Sunset` のヘッダを付けて少なくとも 1 つのマイナーリリースの間残す）。
  spec の `deprecated: true` は生成物の doc コメント（Go の `Deprecated:`、TS の `@deprecated`）に出る。ライブラリは未知のフィールドと列挙値を受け流す。

### 5. 認証

- ライブラリはトークンを**保存しない**。利用者が `TokenSource`（Go は `oauth2.TokenSource` と同じ形の 1 メソッドのインターフェイス、TS は `getAccessToken()` と `refresh()` を持つオブジェクト）を渡す。
  refresh token で更新する実装を主にし、更新しない固定のアクセストークンの実装はテストと短い作業のためにだけ置く。
  ADR-0090 の登録者自身へのトークン発行も、長期のトークンではなく通常のアクセストークンと refresh token を返すので、ボットも更新する実装を使う（ADR-0094 は回転した refresh token を Store に保存する）。
- refresh token は使うたびに新しいものに替わる（RFC 9700 §4.14 の回転）。ライブラリは更新の結果の新しい refresh token を利用者のコールバック（`onTokens`）に必ず渡し、利用者が保存する。
  同時の 401 で更新が重ならないよう、1 つの `TokenSource` の中で更新を直列にする（Web クライアントのセッションと同じ考え方）。
- 401 のときは 1 回だけ更新して送り直す。2 回目の 401 はそのままエラーにする。
- 認可コードの取得（PKCE・リダイレクト・クライアントの登録）は ADR-0090 に依存する。`mykit` は、ADR-0090 が決めるエンドポイントに対する PKCE の補助（`code_verifier` の生成と交換）だけを持ち、ブラウザやローカルのリダイレクトの受け口は持たない。

### 6. ストリーミング

- `GET /api/v1/streaming`（client-api.md §5）を fetch のストリーム（TS）・`net/http` の応答本体（Go）で読む手書きのクライアントを置く。`EventSource` は使わない（ヘッダを送れない、ADR-0029）。
- サーバーは `id` を付けず、`Last-Event-ID` での再開をしない（client-api.md §5）。ライブラリも `Last-Event-ID` を送らず、再接続のたびに「再接続した」ことを利用者に知らせる（Web クライアントの `onOpen(reconnected)` と同じ）。間の出来事はタイムラインと通知を読み直して埋めるのが利用者の責任だと文書に書く。
- 終了・失敗のたびに、指数バックオフとジッタで再接続する。429（`too-many-streams`）・503 は `Retry-After` に従う。30 秒ごとのハートビートのコメントが 2 回分（60 秒）届かなければ接続を切って再接続する（間隔は設定できる）。
- イベントは型付きの判別共用体（TS）・インターフェイスと型 switch（Go）で渡す。知らない `event` は捨てずに「未知のイベント」として渡す（サーバーが先に増やしても壊れない）。

### 7. 実行環境

- **TS**: ESM のみ。`fetch`・`ReadableStream`・`TextDecoderStream`・`AbortSignal` を持つランタイム（現行のブラウザ、Node.js 22 以降、Bun、Deno）を対象にする。Node の組み込みモジュールは使わない。依存は `openapi-fetch` だけにする。
  型の宣言と ESM の JS を出す。ビルドの道具は ADR-0025 以降で採っているものから選ぶ（新しい道具は足さない）。
- **Go**: サーバーと同じ最小版（`go.mod` の `go` 行。今は 1.27）。cgo を使わず `CGO_ENABLED=0` で使える。HTTP は利用者が渡す `*http.Client`（既定は新しい `http.Client`。`http.DefaultClient` は使わない）。

### 8. 版と互換・公開・テスト・文書

- **版**: ライブラリはサーバーと同じ版の番号を使い、同じリリースで出す（Go のタグ `pkg/mykit/vX.Y.Z`・`pkg/mybot/vX.Y.Z`、npm の `X.Y.Z`）。生成元の spec がサーバーの版と 1 対 1 なので、対応表が要らない。
  ライブラリの変更は ADR-0090 §8 の API の安定性に従う。API の破壊的変更で生成物の型や関数が消えるときは、その前のマイナーリリースで doc コメントが `Deprecated` になる。
  手書きの層の破壊的変更は v0 の間は許すが、リリースノートの「mykit」の節に書き、Conventional Commits の `!` を付ける。サーバーより新しいライブラリが古いサーバーを呼ぶ互換は v0 では保証しない（README に書く）。
- **公開**: #1029 のリリースの仕組みに組み込む。npm は GitHub Actions からの **trusted publishing**（OIDC。長く生きる npm のトークンを置かない）で公開し、来歴（provenance）を付ける。
  公開は trusted publishing からだけにし、パッケージの設定で公開に 2FA を求めトークンを禁じる（trusted publishing は OIDC なので影響しない）。npm の組織 `mycroblog` の所有者は決定者。
  （npm Docs「Trusted publishing for npm packages」、2026-10-23 に docs.npmjs.com/trusted-publishers で確認: GitHub-hosted runner に対応し、公開のリポジトリからの公開では来歴を既定で作る。npm CLI 11.5.1 以降・Node 22.14.0 以降が要る。）
  Go はタグを打つだけで proxy.golang.org が取り込む。
- **テスト**:
  - 単体テスト: 手書きの層（再試行・ページング・SSE の解析と再接続・トークンの更新）を Go は `httptest`、TS は `bun test` で試す。
  - 結合テスト: `pkg/mykit/integration/` の試験用モジュールで、`pkg/testkit` を使ってサーバーをプロセス内で起動し、Go のクライアントで主要な流れ（投稿・タイムラインのページング・ストリーミング・アップロード・429）を通す。TS の結合テストは `app/web` の E2E が `mykit` を通るので、別には持たない。
  - CI: go-task に `mykit` の文脈（`tasks/mykit.yml`。Go の vet・lint・test・build・licenses、TS の typecheck・lint・test・licenses、`go.mod` がサーバーのモジュールを require しない検査）を足し、`check:changed` と merge queue の `check` に入れる。agentkit の `context-map.yaml` に `pkg/mykit`・`pkg/mybot` の領域を足す。
- **文書**: 英語の README（インストール・認証・ページング・ストリーミング・エラー・再試行の例）を `pkg/mykit/README.md` に、Go の例は `example_test.go` に置く。#1031 の英語の文書の方針に従う。

#### 見直す条件

- oapi-codegen の client が spec を扱えない、または生成物が利用者に出すには使いにくいなら、Go だけ C-3（別の生成器）か、型だけ生成して手書きのクライアントにする。

## Consequences（影響）

- 良い影響
  - ボット・アプリの作者が、ライセンスとサーバーの依存に縛られずに型付きのクライアントを使える。`pkg/mybot` の土台になる。
  - TS の型の生成が 1 か所になり、Web クライアントの横断の処理（認証・エラー・SSE）がライブラリとして試験され、公開の利用者と同じコードを通る。
  - 仕様とライブラリのずれは今と同じ差分検査で止まる。
- 悪い影響・トレードオフ
  - リポジトリの中にライセンスの異なる範囲ができる。SPDX ヘッダと `LICENSE` の置き方を間違えやすく、検査が要る。`api/spec/` の新しいファイルの見出しが `MIT` になる。
  - Go の別モジュールが増え、タグ・`go.sum`・lint の設定・CI の文脈が増える。サーバーの変更で spec を変えると `mykit` の生成物も同じ PR で変わる。
  - `app/web` が `mykit` に依存することで、Web クライアントのための変更がライブラリの公開の API の変更になる。v0 では許すが、v1 では分ける判断が要る。
  - 公開の API を持つことで、生成物の名前（oapi-codegen の型名・関数名）が利用者から見える。spec の `operationId` やスキーマ名の変更が破壊的変更になる。
  - Go のモジュールの zip に TS のソースが入る。
  - Go の生成物は約 7.6 万行・3.1 MB と大きく、`api:generate` の時間とリポジトリの差分の量が増える。
  - `pkg/mykit/ts` は `app/` の外にあるので、`app/` を前提にした道具（oxlint・knip・`web:check` の文脈）の設定に足す必要がある。
- 後続で必要になる作業（それぞれ issue にする）
  1. ライセンスの範囲の変更: `api/spec/`・`api/openapi.yaml` の SPDX を `MIT` に、生成の見出し、`README.md` のライセンスの節、ADR-0011 の Status の注記、SPDX の範囲の検査。
  2. Go の `pkg/mykit`: モジュール・生成の設定・手書きの層（認証・エラー・ページング・再試行・冪等キー・アップロード）・単体テスト。
  3. TS の `@mycroblog/mykit`: ワークスペース・生成・手書きの層・単体テスト。
  4. ストリーミングのクライアント（Go・TS）。
  5. 結合テストの試験用モジュール `pkg/mykit/integration/` と `tasks/mykit.yml`・`context-map.yaml`・CI の文脈。
  6. `app/web` の移行（`lib/api/client.ts`・`lib/sse/` を `mykit` の上に）。
  7. 公開: npm の組織とパッケージの用意、trusted publishing、Go のタグを #1029 のリリースに組み込む。
  8. 英語の README と例（#1031 と合わせる）。
- 要件（`docs/requirements/` への追記）
  - `01-functional.md`「クライアント API」: 「FR-504 | Go と TypeScript（npm）の API クライアントライブラリ `pkg/mykit` を寛容なライセンス（MIT）で公開する。型は API の仕様から生成し、認証・エラー・ページング・再試行・ストリーミングを扱う（ADR-0093） | S」
    ボット制作ライブラリは ADR-0094 が「FR-505」として足す。
  - `00-vision.md` の M5 の範囲に「API クライアントライブラリ（`pkg/mykit`）とボット制作ライブラリ（`pkg/mybot`）の公開」を足す。

## Pros and Cons of the Options（詳細比較・任意）

### A. 置き場所

- A-1: import のパスがきれい（`.../pkg/mykit`）。Go のモジュールに `ts/` が同居する。
- A-2: 言語が対称で境界が明快。import のパスが `.../pkg/mykit/go` になり、パッケージ名（`mykit`）とずれる。
- A-3: タグも `go.mod` も増えない。利用者がサーバーのモジュール全体（DB ドライバ・クラウド SDK）を依存のグラフに入れ、版がサーバーのタグ `vX.Y.Z` に縛られる。モジュールのグラフの刈り込みでビルドには入らないが、`go.sum` と `go mod download` は重い。ライセンスもモジュールの単位で分けにくい。

### B. ライセンス

- B-1（MIT）: 最も広く受け入れられ、短い。特許の明示の許諾はない。
- B-2（Apache-2.0）: 特許の許諾と商標の扱いがある。AGPL-3.0 と両立する。NOTICE の扱いが利用者に少し手間。
- B-3（AGPL）: 例外がなくリポジトリが一様で、改変したクライアントのソースの公開も求められる。ボット・アプリの作者は自分のコードを AGPL にしない限り使えず、ライブラリを出す目的の大半を損なう。

### C. 生成

- C-1: 今の道具と版をそのまま使い、新しい依存がない。oapi-codegen の client の使い勝手は ogen などより素朴（応答は状態コードと content type ごとのフィールドを持つ構造体で、利用者が `StatusCode` を見て選ぶ）。
- C-2: 使い勝手を自由に設計できる。spec とのずれを型で止められず、保守の量が操作の数に比例する。
- C-3: ogen は型の表現が豊かで、TS の Hey API は SDK の形まで生成する。生成器が 2 種類（サーバーと別）になり、ADR-0029 の「生成ツール」を広げる新しい ADR が要る。

## Open Questions / 要検証

- なし（npm の組織の名前が取れないときは、そのときに別のスコープを決める）。
