# ADR-0093: API クライアントライブラリ pkg/mykit とボット制作ライブラリ pkg/mybot

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1065, #1066, #1978

## Decision（決定）

`pkg/mykit` に Go と TypeScript（npm）の API クライアントライブラリを、その上に `pkg/mybot` にボット制作ライブラリを置いて公開する。どちらもサーバーと別のモジュールで、ライセンスは MIT である。

- `pkg/mykit`（§1〜§8）: Go は oapi-codegen の client、TS は openapi-typescript の型 + openapi-fetch を `api/openapi.yaml` から生成し、横断の処理だけの薄い手書きの層を被せる。生成物はコミットする。
- `pkg/mybot`（§9〜§19）: ボットの作者が書くのはハンドラだけで、取りこぼしと重複の扱い・行儀の既定・レート制限はライブラリが持つ。常時 SSE を読み、接続のたび（起動・再接続）に通知の一覧を前回の位置まで読み直し、処理済みの位置と ID を `Store` ポートに記録する。既定の実装（メモリ・ファイル）を同梱する。ボットはライブラリとして組み込み、ランナーは出さない。

### 1. mykit: 配置とモジュール

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
  `pkg/mykit/go/` にしないのは、import のパスの末尾が `go` になり、パッケージ名とずれるため。代わりに Go のモジュールの zip に `ts/` の TS のソースが入る（コミットしたファイルだけで、`node_modules` は入らない）。
- `mykit` の `go.mod` は `github.com/mycroblog/my`（サーバーのモジュール）を require しない。これを検査する（下の §8）。
  別モジュールなので、Go の `internal` の規則は効かない。代わりにモジュールの境界が依存を止める。`mykit` は標準ライブラリと、生成物が要る依存だけを使う。生成物をビルドに含めるサードパーティのモジュールは `github.com/oapi-codegen/runtime`（Apache-2.0）・`github.com/apapsch/go-jsonmerge/v2`（MIT）・`github.com/google/uuid`（BSD-3-Clause）の 3 つで、`go.sum` は 23 行になる。runtime の `go.mod` は gin・echo・iris などのフレームワークを require するので、利用者の `go list -m all` には出るが、ビルドには入らない（2026-10-23 に runtime v1.7.0 で、別モジュールとして生成物をビルドして確認）。
- 開発では、`integration/` の試験用モジュールだけが `replace` でサーバーと `mykit` を結ぶ。`tools/go.mod` と同じく、ルートの `./...` からは外れる。ルートの `go.work` は Dependabot のためだけにあり、`pkg/mykit`・`pkg/mybot` を入れない（ADR-0081）。
- **TS は `pkg/mykit/ts/`** に置き、npm のパッケージ名は **`@mycroblog/mykit`**（`app/` の私用の `@my/*` とは別の公開のスコープ）。npm の組織 `mycroblog` は決定者が作って所有する。`@mycroblog/mykit`・`@mycroblog/mybot` はレジストリになく、スコープ `mycroblog` の公開のパッケージもない（2026-10-23 に registry.npmjs.org で確認）。組織の名前が取れなければ、そのときに別のスコープを決める。
  Bun ワークスペース（ルートは `app/`）の `workspaces` に `../pkg/mykit/ts` を足し、`app/web` は `workspace:*` で使う。Bun はルートの外のパスをワークスペースとして扱い、`app/bunfig.toml` の `linker = "isolated"` でも `../pkg/mykit/ts` の依存（openapi-fetch）を解決する。`bun.lock` には `../pkg/mykit/ts` の項が入り、`pkg/mykit/ts/node_modules` ができる（2026-10-23 に Bun 1.4.3 で、`app/` と同じ設定の最小のワークスペースを作って確認）。
- `pkg/mybot`（§9）も同じ形（`pkg/mybot/` が Go の別モジュール、`pkg/mybot/ts/` が `@mycroblog/mybot`）にし、`mykit` だけに依存する。

### 2. ライセンス

- `pkg/mykit/`・`pkg/mybot/` と、生成元の `api/spec/` とそのバンドル `api/openapi.yaml` を **MIT** にする。生成物は仕様から機械的に作るので、仕様が AGPL のままだと生成物の扱いが曖昧になる。仕様を同じライセンスにしてそれを避ける。
  サーバーの実装（`internal/`・`cmd/`・`app/web` など）は AGPL-3.0-only のまま。MIT は AGPL-3.0 と両立するので、サーバーと Web クライアントが MIT のコードを使うことに問題はない。
- **ADR-0011 との関係**: 上の範囲は ADR-0011 の「リポジトリは AGPL-3.0 とする」の例外である。AGPL 第 13 条のソースの提示などその他の決定は変えない。
- 置き方: それぞれのディレクトリに MIT の `LICENSE` を置き、ルートの `README.md` のライセンスの節に例外の範囲を書く。
  SPDX ヘッダは `MIT`（`// SPDX-License-Identifier: MIT`、YAML は `# SPDX-License-Identifier: MIT`）。生成物の見出しも `MIT` にする。
- 既存の `api/spec/` の著作者は決定者 1 人（`git shortlog` で確認）なので、決定者の判断で再ライセンスできる。Dependabot の変更は `api/spec/` に及ばない。
- **依存のライセンスの検査**: `mykit`・`mybot` の依存は MIT・Apache-2.0・BSD・ISC など寛容なものだけにする。AGPL と両立するだけの（GPL 系の）依存は利用者に及ぶので入れない。
  Go は `go-licenses`、npm は `tools/i18n` の検査と同じ仕組みに、寛容なものだけの許可リストを渡す。

### 3. コード生成

- 生成は `api/openapi.yaml`（バンドル）からだけ行い、`scripts/generate.sh` の対象に足す。生成物はコミットし、`api:generate` の差分検査に入れる（ADR-0016 のまま）。
  - Go: oapi-codegen（`tools/go.mod` の同じ版）で `generate: {models: true, client: true}` を `pkg/mykit/api.gen.go` に生成する。サーバーと同じく `always-prefix-enum-values` にする。
    SSE の `streamEvents` は生成から外し、手書きする。
    この設定で今の `api/openapi.yaml` から client を生成でき、`CGO_ENABLED=0` でビルドと `go vet` が通る（2026-10-23 に `tools/go.mod` の oapi-codegen v2.8.0 で確認。生成物は約 7.6 万行・3.1 MB）。
    操作ごとに `<Op>WithResponse` が型付きの応答（`JSON200 *Status`・`ApplicationproblemJSONDefault *Problem`・応答ヘッダ）を返す。multipart の操作（`UploadMedia`）は `content-type` と `io.Reader` を受ける `<Op>WithBody` だけになるので、下のアップロードの関数で補う。
  - TS: openapi-typescript で `pkg/mykit/ts/src/schema.gen.ts` を生成する。`app/web/src/lib/api/schema.gen.ts` は置かず、`app/web` は `@mycroblog/mykit` の型を使う。型の生成は 1 か所になる。
- **手書きの層**は次の横断の処理だけにする。エンドポイントごとの便利な関数は書かない（生成したクライアントをそのまま公開する）。
  - 認証（§5）、`Accept-Language` の付与、`User-Agent`（Go）。
  - **エラー**: 2xx 以外の応答を RFC 9457 の問題の詳細の型 `Problem`（`type`・`title`・`status`・`detail`・拡張）を持つエラーにする（Go は `*mykit.Problem` を `errors.As` で取り出せる型、TS は `ProblemError`）。機械判定は `type` で行う。ネットワークの失敗は別の型にする。
  - **ページング**: `next_cursor` をたどる反復子（Go は `iter.Seq2[T, error]`、TS は `AsyncIterable<T>`）。止めるのは利用者。
  - **レート制限と再試行**: 429 と 503 の `Retry-After` に従って待ち、上限回数まで再試行する。再試行するのは安全なメソッド（GET・HEAD）と、`Idempotency-Key` を付けた要求だけ。既定は有効で、無効にできる。
  - **冪等キー**: spec が `Idempotency-Key` を受ける操作では、利用者が渡さなければライブラリが UUID を作って付け、再試行で同じ値を送る。
  - **メディアのアップロード**: multipart の組み立てを `io.Reader`（Go）・`Blob`（TS）から行う関数を 1 つ置く。
- **`app/web` との分担**: `app/web/src/lib/api/client.ts` の Bearer・401 での更新・エラーと `lib/sse/` の読み取りと再接続は `@mycroblog/mykit` に置き、`app/web` には文言の整形（i18n）・`lib/network` への報告・TanStack Query との接続（openapi-react-query）を残す。
  `mykit` は文言を持たない（i18n は利用者の仕事。サーバーがローカライズした `title`・`detail` はそのまま渡す）。

### 4. 公開する API の範囲

- spec の全操作を生成し公開する。`admin` スコープの操作（`/admin/*`、ADR-0058）も同じクライアントに含める。分けるとタグごとの生成の設定が増え、権限はサーバーがスコープと権限で判定するので、ライブラリで分ける利点が小さい。
  `admin` 系のスコープはファーストパーティのクライアントにだけ与えられる（[ADR-0090](0090-third-party-oauth-apps.md) §4）ので、サードパーティのトークンでこれらの操作を呼ぶと 403 になる。生成物の doc コメントと README にそのことを書く。
- API の安定性は ADR-0090 §8 に従う（追加を既定とし、破壊的変更は `deprecated: true` と `Deprecation`・`Sunset` のヘッダを付けて少なくとも 1 つのマイナーリリースの間残す）。
  spec の `deprecated: true` は生成物の doc コメント（Go の `Deprecated:`、TS の `@deprecated`）に出る。ライブラリは未知のフィールドと列挙値を受け流す。

### 5. 認証

- ライブラリはトークンを**保存しない**。利用者が `TokenSource`（Go は `oauth2.TokenSource` と同じ形の 1 メソッドのインターフェイス、TS は `getAccessToken()` と `refresh()` を持つオブジェクト）を渡す。
  refresh token で更新する実装を主にし、更新しない固定のアクセストークンの実装はテストと短い作業のためにだけ置く。
  ADR-0090 の登録者自身へのトークン発行も、長期のトークンではなく通常のアクセストークンと refresh token を返すので、ボットも更新する実装を使う（§15 は回転した refresh token を `Store` に保存する）。
- refresh token は使うたびに新しいものに替わる（RFC 9700 §4.14 の回転）。ライブラリは更新の結果の新しい refresh token を利用者のコールバック（`onTokens`）に必ず渡し、利用者が保存する。
  同時の 401 で更新が重ならないよう、1 つの `TokenSource` の中で更新を直列にする（Web クライアントのセッションと同じ考え方）。
- 401 のときは 1 回だけ更新して送り直す。2 回目の 401 はそのままエラーにする。
- 認可コードの取得（PKCE・リダイレクト・クライアントの登録）は ADR-0090 に依存する。`mykit` は、ADR-0090 が決めるエンドポイントに対する PKCE の補助（`code_verifier` の生成と交換）だけを持ち、ブラウザやローカルのリダイレクトの受け口は持たない。

### 6. ストリーミング

- `GET /api/v1/streaming`（client-api.md §5）を fetch のストリーム（TS）・`net/http` の応答本体（Go）で読む手書きのクライアントを置く。`EventSource` は使わない（ヘッダを送れない、ADR-0016）。
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

### 9. mybot: 配置・ライセンス・公開

§1・§2・§7・§8 にすべて揃え、違えるところはない。

- Go: `pkg/mybot/` を別のモジュール `github.com/mycroblog/my/pkg/mybot`（パッケージ名 `mybot`、タグ `pkg/mybot/vX.Y.Z`）にし、`mykit` だけに依存する。
  `go.mod` はサーバーのモジュールを require しない（§8 と同じ検査）。最小の Go の版と `CGO_ENABLED=0` も `mykit` と同じ。
- TS: `pkg/mybot/ts/` に `@mycroblog/mybot` を置き、`@mycroblog/mykit` を `dependencies` に置く（同じ版の組で動作を確かめるため）。ESM のみで、対象のランタイムは `mykit` と同じ（Node.js 22 以降・Bun・Deno。ブラウザでの動作は約束しない）。
- ライセンス: §2 のとおり MIT（`pkg/mybot/LICENSE`、SPDX ヘッダ `MIT`）。依存は寛容なライセンスのものだけ。
- 版と公開: サーバー・`mykit` と同じ版の番号で同じリリースに出す（#1029 の仕組み、npm は trusted publishing）。ライブラリの破壊的変更の扱いは §8 と同じで、`mybot` が呼ぶ `/api/v1` の変更は [ADR-0090](0090-third-party-oauth-apps.md) §8 の約束に従う。
- 外部の依存は最小にする。Go・TS とも `mykit` と言語・実行環境の標準（`fetch`・`AbortSignal`）だけで書き、TS の cron の式の解釈だけを依存にする（§13）。

### 10. mybot: `mykit` との境界

| 持つもの | `mykit` | `mybot` |
| --- | --- | --- |
| HTTP・型付きの API・問題の詳細のエラー | ○ | 使うだけ |
| アクセストークンの付与と refresh token のローテーション（`TokenSource`） | ○ | 新しいトークンを受けるコールバック（`onTokens`）で `Store` に保存する |
| SSE の読み取り・ハートビート・再接続（バックオフ、`Retry-After`） | ○ | 接続のたびの追いつき（§11） |
| `429`・`503` の `Retry-After` に従う再試行、`Idempotency-Key` の付与 | ○ | 決まった冪等キー（§14）と送信の予算を上乗せ |
| イベントの振り分け・コマンド・ミドルウェア・定期実行・処理済みの記録・テストの道具 | | ○ |

`mykit` に足りないものがあれば、`mybot` で補わず `mykit` に足す。

### 11. mybot: イベントの受け取り

- 起動すると `verifyCredentials` で自分のアカウントを確かめ（§15）、SSE（`/api/v1/streaming`）を開く。
- **追いつき**: SSE を開くたび（起動と再接続）、開いた後に `listNotifications` を新しい順に読み、`Store` の処理済みの位置（最後に処理した通知の ID）より古いものに届くか、
  追いつきの上限（既定 24 時間より古い通知。設定で変える、0 で追いつかない）に届いたところで止め、古い順に処理する。
  SSE を先に開くので、読み直しの最中に来た通知も落ちない（重複は次の項で除く）。
- **重複の除去**: 通知の ID（UUIDv7 で時系列に並ぶ）ごとに処理済みを `Store` に記録し、記録済みの ID は処理しない。記録は既定で 7 日（追いつきの上限より長い）で捨てる。
- **ハンドラの後に記録する**（at-least-once）。ハンドラの途中でプロセスが止まると同じ通知をもう一度処理することがあるので、返信などの書き込みは §14 の冪等キーで 2 度目を無害にする。
- 再接続（バックオフ・`Retry-After`・ハートビートの途絶の検出）は `mykit` の SSE のクライアントに任せ、`mybot` は `mykit` が知らせる「(再)接続した」を合図に追いつきを走らせる。
- SSE の `update`（ホームの新着）は、作者が `OnStatus` を登録したときだけ扱う。ホームの新着は追いつきをせず、切れていたあいだの分は届かない（文書に書く）。
- ポーリングだけの動き方も `Mode: Polling`（間隔の既定 60 秒）として持つ。SSE が通らないプロキシの環境と、短命の実行（cron で起動して追いついたら終わる `RunOnce`）のため。

### 12. mybot: ハンドラの形

通知の種類（`NotificationType`）に対応するハンドラを出し、Go と TS で同じ名前の概念にする。

| ハンドラ | 起きるとき |
| --- | --- |
| `OnMention` | `mention`。返信・`direct` の投稿（DM）を含み、文脈に `IsReply`・`IsDirect` を持つ。サーバーは公開範囲によらず、投稿の版が新しくメンションしたアカウントに `mention` を作る（`internal/app/notification/projection.go` の `projector.mentions`） |
| `OnCommand(name, ...)` | `OnMention` のうち、本文の先頭の自分へのメンションを除いた最初の語が `name` のもの（例: `@bot help`） |
| `OnFollow` / `OnFollowRequest` | `follow` / `follow_request` |
| `OnReaction` / `OnFavourite` / `OnBoost` / `OnQuote` | 同名の通知 |
| `OnNotification` | どの種類でも（上のどれにも当たらないものを含む） |
| `OnStatus` | SSE の `update`（§11） |
| `Every(cronExpr, ...)` | 定期実行（§13） |

- 本文は HTML なので、ライブラリが平文に直したもの（メンション・リンクを文字列にする）を文脈に入れる。コマンドの解析は、先頭の自分へのメンションを除き、空白で区切った最初の語をコマンド名、残りを引数の文字列にする。
  名前は大文字と小文字を区別しない。どのコマンドにも当たらないメンションは `OnMention` に落ちる。`help` は登録したコマンドの説明から既定で答える（無効にできる）。
- 文脈（Go は `*mybot.Context`、TS は `ctx`）は、元の通知・投稿・送り手と、`Reply(text)`・`React(emoji)`・`Follow()` などの補助を持つ。補助は §14 と §16 の既定を守る。
- **ミドルウェア**: Go は `func(mybot.Handler) mybot.Handler`、TS は `(ctx, next) => Promise<void>`。既定で次を順に入れる: パニック・例外の回収とログ、§16 の行儀の検査、§14 の送信の予算。
  作者は権限（例: 管理者のアカウントだけが使えるコマンド）・ログの形などを足せる。ログは Go が `log/slog`、TS は `console` 互換の受け口にする。
- 並行: ハンドラは既定で最大 4 つ同時に動かし、同じ送り手の通知は順に処理する（`Status` は会話の ID を持たないので、送り手で分ける）。

### 13. mybot: 定期実行

- `Every("0 9 * * *", handler, opts)` で 5 フィールドの cron の式（分 時 日 月 曜日）とタイムゾーン（既定は UTC、IANA の名前で指定）を受ける。
- 式の解釈: Go は 5 フィールド（数・範囲 `a-b`・刻み `*/n`・列挙 `a,b`）だけを `mybot` の中で自作し、依存を持たない。秒のフィールドや `@daily` などの別名は受けない。
  TS は `croner`（MIT、依存なし、Node・Deno・Bun で動く。2026-10-23 に npm の `croner` の最新が 10.0.1（2026-02 公開）で、github.com/hexagon/croner が保守中であることを確認）で式を解釈し、次の時刻の計算だけに使う。
  Go と TS で受ける式の範囲を揃えるため、TS でも 6 フィールドと別名を `mybot` の側で拒否する。
- **止まっていたあいだの扱い**: 既定は飛ばす（起動の後の次の時刻から動く）。`CatchUp: Latest` を指定すると、`Store` の最後に実行した時刻より後の予定のうち、最も新しい 1 回だけを起動時に実行する。
  何回分も溜まった分をまとめて投稿することはしない。
- 定期実行のハンドラが投稿するとき、冪等キーは `mybot:every:<名前>:<予定の時刻 RFC 3339>` にする（§14）。同じボットを誤って 2 つ動かしても、同じ時刻の投稿は 1 つになる。

### 14. mybot: 冪等とレート制限

- **冪等キー**: `Reply` と定期実行の投稿は、`createStatus` に決まった `Idempotency-Key` を付ける。返信は `mybot:reply:<通知 ID>:<そのハンドラの中での番号>`（`OnStatus` は通知を持たないので、通知 ID の代わりに投稿の ID を使う）。
  サーバーはアカウントごとにキーを覚えているので（`local_statuses` の一意制約）、§11 で同じ通知をもう一度処理しても投稿は 1 つになる。作者が `mykit` を直接使って投稿するときのために、同じ規則でキーを作る関数も出す。
  お気に入り・リアクション・フォローは、spec のとおり 2 度目の呼び出しが何も変えずに成功するので（`favouriteStatus`・`reactToStatus`・`followAccount` の説明）、キーを付けない。
- **処理済みの記録**: `Store` ポート（Go はインタフェース、TS は型）で、処理済みの通知の ID・追いつきの位置・定期実行の最後の時刻・refresh token を持つ。
  同梱する実装は `MemoryStore`（試験向け。再起動で消える。§15）と `FileStore`（JSON の 1 ファイル、書き込みは一時ファイルからの rename）。
  既定は `FileStore`（`./mybot-state.json`）。DB などを使いたい作者は `Store` を実装する。
  サーバーの既読のマーカー（`notifications/marker`）は位置に使わない。ボットのアカウントを人が Web で開くとマーカーが進み、ボットが未処理の通知を飛ばすため。
- **送信の予算**: `mykit` の `429` の再試行に加え、`mybot` は送信の前にクライアント側のトークンバケットで待つ。既定は投稿が 1 時間に 60（サーバーの 300 / 3 時間の 6 割）、
  そのほかの書き込みが 1 分に 60。応答の `RateLimit` ヘッダの残りが 1 割を切ったら次の窓まで待つ。予算を超えた返信は捨てずに待たせ、待ちが 10 分を超えたものはログに出して捨てる。

### 15. mybot: 認証と bot フラグ

- 認証は `mykit` の `TokenSource` をそのまま受ける。固定のアクセストークンか、refresh token で更新するもの（§5）で、取得の手順は持たない。トークンの取り方は [ADR-0090](0090-third-party-oauth-apps.md) に従い、README に書く。
  主な手順は、ボットのアカウントでログインし、設定の画面でアプリ（confidential client）を登録して、登録者自身へのトークンを redirect なしに発行すること（ADR-0090 の「登録者自身へのトークン発行」）。
  このトークンは長命のトークンではなく、ふつうのアクセストークンと refresh token の組なので、`mybot` は `mykit` の refresh token で更新する `TokenSource` を使う。
  `client_credentials` はないので、ボットは必ずボット用のアカウントの利用者として動く。Authorization Code + PKCE で認可したい作者のために、ループバックのリダイレクトで受け取る小さな例（`examples/login`）を置く。
- refresh token がローテーションされたら（`mykit` の `onTokens`）、`mybot` が新しいものを `Store` に保存する。`mykit` はトークンを保存しないので、ここが `mybot` の足す分になる。古いものを使い回すと RFC 9700 の検出で系列ごと失効するため。
  起動時は `Store` に保存した refresh token を、設定で渡した最初の refresh token より優先する。`MemoryStore` では再起動のたびに最初の refresh token に戻り、それは使用済みなので失効する。`MemoryStore` は試験のためのもので、常駐のボットも `RunOnce` を繰り返し起動するボットも永続の `Store` を使う（README に書く）。
- 要るスコープ: 既定のハンドラの組で `read:notifications`・`read:statuses`（SSE に要る）・`write:statuses`。起動時に足りなければ、足りないスコープを挙げて止まる。
- **bot フラグ**: 起動時に `verifyCredentials` の `bot` を確かめ、偽なら既定ではエラーで止まる。`SetBotFlag: true` を指定すると `updateCredentials` で立てる（`write:accounts` が要る）。
  確かめずに動かすには `AllowNonBot: true` を明示する。どのアプリから投稿しても bot フラグは変わらない（ADR-0090）ので、ボットであることを連合先と利用者に示すのは `mybot` の既定が担う。

### 16. mybot: 振る舞いの既定（行儀）

すべて既定で有効にし、作者が個別に外せる。

- **自分には応答しない**。自分のアカウントからの通知は振り分けない（外せない）。
- **bot フラグのアカウントには応答しない**。送り手の `Account.bot` が真なら、ハンドラを呼ばずに処理済みにする。`RespondToBots: true` で外す。
- **連鎖の上限**: 同じ送り手への返信は 10 分に 5 まで、1 時間に 15 まで。超えた分はハンドラを呼ばずに処理済みにする。bot フラグを立てていない相手のボットとの連鎖も止めるため。
- **公開範囲**: `createStatus` は `visibility` を必須にしていて、サーバーは返信の公開範囲を補わない（`CreateStatusRequest` の `required`。ADR-0017 の「`local` への返信の既定は `local`」はクライアントの既定）。そのため `mybot` が既定を決める。返信の公開範囲は元の投稿より広げない（`direct` には `direct`、`private` には `private`、`local` には `local`、`unlisted` には `unlisted`）。`public` への返信の既定は `unlisted`。
  定期実行の投稿の既定は `unlisted` で、作者が `public`・`local` を明示できる。`local`（ADR-0017）は作者のサーバーの中と作者のフォロワーにだけ届くことを README に書く。
- **返信の宛先**: 返信の本文には元の送り手だけをメンションし、元の投稿の他のメンションを引き継がない（巻き込みを防ぐ）。
- サーバーの規約: ライブラリはサーバーごとの規約を読まない。README に、ボットを動かす前にサーバーの規約（ボットの可否・投稿の頻度）を確かめることを書き、上の既定をその下限の目安とする。

### 17. mybot: 実行の形

- ライブラリとして組み込むだけにし、ランナーの CLI・コンテナのイメージは出さない。
  Go のボットは作者のバイナリになり、TS のボットは作者のスクリプトを `bun`・`node`・`deno` で動かす（対応するランタイムは §7 に揃える）。例には `Dockerfile` を 1 つ置く。
- `Run(ctx)`（Go）・`bot.run({ signal })`（TS）が止まるまで動く。Go の例は `signal.NotifyContext` で SIGINT・SIGTERM を受ける。
- **グレースフルシャットダウン**: 止める合図で SSE を閉じ、新しい通知を受けるのをやめ、動いているハンドラを猶予（既定 10 秒）まで待ち、`Store` を書き出して戻る。猶予を過ぎたハンドラは取り消し（`context` / `AbortSignal`）、
  処理済みに記録しないので、次の起動で追いつきの対象になる（冪等キーで二重の投稿にはならない）。
- `RunOnce(ctx)`: 追いつきと、時刻を過ぎた定期実行を 1 回ずつ済ませて戻る。cron・Cloud Run jobs などの短命の実行のため。
- 1 つのボットを複数のプロセスで動かすことは想定しない（`Store` の排他はしない）。誤って動かしても、冪等キーで二重の投稿にはならないが、同じ会話の順序は保証しない。

### 18. mybot: テスト

- 作者向けに `mybottest`（Go）と `mybot/testing`（TS の subpath export）を出す。
  - 偽のサーバー: HTTP のテストサーバーで、`mybot` が使う操作（`verifyCredentials`・`listNotifications`・`streaming`・`createStatus` と §12 の補助が呼ぶもの）だけを、`api/openapi.yaml` の型で応答する。
  - イベントの注入: `Inject(notification)` で SSE に通知を流し、`Posted()` で投稿を取り出して確かめる。時計を差し替えて定期実行を進められる。
- このリポジトリの中: ライブラリの単体テスト（Go は `httptest`、TS は `bun test`）と、偽のサーバーの応答を spec と照合するテストを、`mykit` と同じ go-task の文脈（§8 の `tasks/mykit.yml`）で `check:changed` と merge queue に入れる。
  結合テストは `pkg/mybot/integration/` の試験用モジュールで `pkg/testkit` のサーバーを起動し、エコーのボットがメンションに 1 回だけ返すこと（SSE を途中で切って追いつかせる場合と、ハンドラの途中で止めて再起動する場合を含む）を確かめる。

### 19. mybot: 例と文書

- 例: エコー（メンションをそのまま返す）・定時の投稿（`Every` と `CatchUp`）・コマンド（`help`・`ping`・`dice`）の 3 つを Go と TS の両方で置く。例はテストで動かす。
- 英語の README（#1031 の方針）を Go と TS のパッケージに置き、トークンの取り方・スコープ・行儀の既定と外し方・`Store` の選び方・グレースフルシャットダウンを書く。

### この決定を見直す条件

- oapi-codegen の client が spec を扱えない、または生成物が利用者に出すには使いにくいなら、Go だけ別の生成器にするか、型だけ生成して手書きのクライアントにする。
- ADR-0090 の登録者自身へのトークンの形（アクセストークンと refresh token の組）が変わる場合: §15 の手順と `Store` の扱いを、そのトークンの形に合わせて直す。
- 通知の一覧を新しい順に読み直す追いつきが、メンションの多いボットで遅い・レート制限に当たる場合: サーバーの `listNotifications` に「この ID より新しいもの」を古い順で返す引数（例: `min_id`）を足す ADR を書き、§11 をそれに置き換える。
- ボット向けの webhook がほかの理由（連携サービス・FaaS のボット）でサーバーに入る場合: `mybot` に受け口のモードを足す。

## 経緯

クライアント API（FR-501、[ADR-0016](0016-client-api-style.md)・[ADR-0016](0016-client-api-style.md)）は `api/spec/` を正とし、Go のサーバーと `app/web` の中だけで使う TS の型を生成していた（[ADR-0016](0016-client-api-style.md)・[ADR-0016](0016-client-api-style.md)）。Bearer の付与・401 での更新・問題の詳細の解釈と SSE の読み取りと再接続は Web クライアントの中にだけあった。決定者は M5 の作業として、外部の開発者が使える `pkg/mykit` と `pkg/mybot` を公開すると指示した（FR-504、FR-505）。[ADR-0011](0011-license.md) はリポジトリ全体を AGPL-3.0-only としていたが、クライアントライブラリが AGPL だと組み込むボットやアプリにも及ぶので、範囲を限って MIT にした（§2）。

判断の基準は、作者が自分のコードのライセンスを縛られずサーバーの依存を引き込まないこと、型を仕様から生成してずれを CI で検出すること、手書きはどの利用者も書き直す横断の処理だけにすること、npm と Go のタグを長く生きる秘密なしに来歴を示して出せることである。

`mybot` は、`mykit` だけでボットを書くと作者が同じ問題を自分で解くことになるので足した。`/api/v1/streaming`（SSE、FR-207）はイベントに `id` を付けず `Last-Event-ID` で再開できないので、取りこぼさないには通知の一覧（`listNotifications`）を読み直して突き合わせる必要があり、SSE と一覧の両方で同じ通知が届く。ボット同士の連鎖、`direct`・`local`（ADR-0017）の投稿への返信の公開範囲の広げすぎ、レート制限（ADR-0019 §5、投稿 300 / 3 時間・アカウント 1,500 / 5 分）の超過は、既定で安全な側に倒す層が要る。プロフィールの bot フラグは FR-006（`Account.bot`）にある。サーバー単位の webhook（ADR-0060）は管理者が `manage_webhooks` で設定する管理向けのもので、利用者の通知を送らないのでボットには使えない。サードパーティのクライアントの登録と API の安定性は [ADR-0090](0090-third-party-oauth-apps.md)、リリースの仕組みは #1029、英語の文書は #1031 が扱う。

§1 ははじめルートに `go.work` を置かないとしていた。Dependabot がルートだけを更新すると `integration` のモジュールの `go.mod` が古いまま残りマージキューで失敗するので、ADR-0081 が Dependabot だけが使う `go.work` を置いた。

## 採らなかった案

- `pkg/mykit/go/` と `pkg/mykit/ts/` に対称に置く: 言語の境界が明快だが、import のパスが `.../pkg/mykit/go` になりパッケージ名とずれる。
- Go をルートの `go.mod` の中のパッケージにする: タグも `go.mod` も増えないが、利用者がサーバーのモジュール全体を依存のグラフに入れ、版がサーバーのタグに縛られ、ライセンスもモジュールの単位で分けにくい。
- Apache-2.0 にする: 条件を満たし特許の許諾もあるが、MIT のほうが本文が短く、Go と npm の小さなライブラリで最も広く通る。
- AGPL-3.0-only のままにする: リポジトリが一様で改変したクライアントのソースも公開されるが、作者は自分のコードを AGPL にしない限り使えず、ライブラリを出す目的の大半を損なう。
- 生成せずに手書きのクライアントを書く: 使い勝手を自由に設計できるが、spec とのずれを型で止められず、保守の量が操作の数に比例する。
- 生成器を別のもの（Go は ogen、TS は Hey API など）にする: 表現が豊かだが、生成器が 2 種類になり、ADR-0016 を広げる新しい ADR が要る。
- 通知のポーリングだけにし、`notifications/marker` の `last_read_id` を位置にする: ボットが状態を持たず最も単純だが、人が Web で開くとマーカーが進んで未処理を飛ばす。refresh token のローテーションを保存する以上（§15）「状態を持たない」利点はどの形でも得られず、`Store` は 1 ファイルの `FileStore` を既定にして手間を抑える。ポーリングは `Mode: Polling` として取り入れた（§11）。
- ボット向けの webhook をサーバーに新設する: 配送の保証をサーバーの再送に任せ FaaS で動かせるが、サーバーの機能・費用と SSRF の面が増え、受け口の公開 URL が要る。
- ランナー中心（CLI とコンテナのイメージ）: コードを書かない人にも開けるが、TS のスクリプトを読むランナーは結局ライブラリと同じ API を要し、Go では作者のバイナリになるので、価値は TS の一部に限られる。
- Go の cron に `robfig/cron`（MIT）を使う: 最後の版が 2020-01 の v3.0.1 で保守が止まっている（2026-10-23 に確認）。5 フィールドだけなら自作が小さい。

## Consequences（影響）

- 良い影響
  - ボット・アプリの作者が、ライセンスとサーバーの依存に縛られずに型付きのクライアントを使える。
  - TS の型の生成が 1 か所になり、Web クライアントの横断の処理がライブラリとして試験され、公開の利用者と同じコードを通る。
  - 仕様とライブラリのずれは差分検査で止まる。
  - ボットの作者は取りこぼし・重複・連鎖・公開範囲を気にせず、ハンドラだけを書ける。
  - サーバーに新しい機能・費用が要らない。SSE の接続数の上限（アカウントごとに 8）の中で 1 本だけ使う。
  - 返信と定期実行の投稿は冪等キーで二重にならないので、`Store` を失っても重複の投稿は起きない。
- 悪い影響・トレードオフ
  - リポジトリの中にライセンスの異なる範囲ができる。SPDX ヘッダと `LICENSE` の置き方を間違えやすく、検査が要る。
  - Go の別モジュールが増え、タグ・`go.sum`・lint の設定・CI の文脈が増える。サーバーの変更で spec を変えると `mykit` の生成物も同じ PR で変わる。
  - `app/web` が `mykit` に依存することで、Web クライアントのための変更がライブラリの公開の API の変更になる。v0 では許すが、v1 では分ける判断が要る。
  - 生成物の名前（oapi-codegen の型名・関数名）が利用者から見え、spec の `operationId` やスキーマ名の変更が破壊的変更になる。
  - Go のモジュールの zip に TS のソースが入る。
  - Go の生成物は大きく、`api:generate` の時間とリポジトリの差分の量が増える。
  - `pkg/mykit/ts` は `app/` の外にあるので、`app/` を前提にした道具（oxlint・knip・`web:check` の文脈）の設定に足す必要がある。
  - 既定の `FileStore` は書き込めるディスクを要る。読み取り専用のコンテナや Cloud Run では、永続のボリュームか自前の `Store`（DB・オブジェクトストレージ）を用意する必要がある。refresh token のローテーションを保存しないと次の起動で認証できないので、常駐のボットにも繰り返し起動するボットにも `MemoryStore` は使えない。
  - 新しい順の一覧を読み直す追いつきは、長く止まっていたボットではページを多く読む。
  - 行儀の既定（bot に応答しない・連鎖の上限・`unlisted` の既定）は、意図してボット同士で会話させたい作者には外す手間になる。
  - ホームの新着（`OnStatus`）は切断のあいだの分を取りこぼす。
  - ランナーを出さないので、コードを書かずにボットを動かしたい人には向かない。
  - 2 言語でハンドラの名前・既定の値・テストの道具を揃え続ける手間がある。

## Open Questions / 要検証

- 連鎖の上限・送信の予算・追いつきの上限の既定の値（§11・§14・§16）。実際のボットで調整する。
