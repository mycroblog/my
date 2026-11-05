# ADR-0010: 設定・パッケージング・イメージの公開

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #11, #2022

## Decision（決定）

### 1. 設定

- 単一の型付き設定スキーマを `core` に定義し、起動時に検証する（NFR-PORT-5）。
- ソース: 環境変数（`MY_` プレフィックス、名称はプロダクト名確定時に変更）→ 任意の設定ファイル（TOML）→ 既定値。
- アダプタ選択は明示的なキー（例: `MY_DATABASE=postgres`, `MY_DATABASE_URL=...`, `MY_QUEUE=sqs`, `MY_BLOB=s3`）。URL スキームからの推測はしない。
- 秘密情報は環境変数 / シークレットストア（Secret Manager, Secrets Manager / SSM, Kubernetes Secret）から。Docker と Kubernetes の secret をファイルで渡せるよう `<NAME>_FILE`（ファイルのパスから読む）も受け付ける。設定ファイルには書けない。

### 2. パッケージング

- モノレポ。サーバーの Go は単一モジュール（`cmd/`, `internal/`, `pkg/testkit`）とする。利用者に配る `pkg/mykit`・`pkg/mybot`（ADR-0093）と開発ツールの `tools` は別のモジュールにする。クライアントアプリは `app/` の下に 1 アプリ 1 ディレクトリで置く（`app/web`。モバイルアプリの `app/mobile` は v0 の範囲の外）。共有パッケージの `app/ui`（ADR-0025）とメールの `app/mail` も同じ場所に置く。
  `app/*` を Bun ワークスペースのメンバーにし、クライアントのツールチェーン（依存・lint・テスト）をサーバーの Go と分けて扱う（レイアウトは [overview](../architecture/overview.md#6-リポジトリ構成)）。
- アダプタはパッケージ単位に分け、ビルドタグで除外できるようにする（NFR-PORT-4）。
- エントリポイント: `cmd/my`（単一バイナリ: `serve` / `worker` / `cli`。Self-hosted・Cloud Run・コンテナ）、`cmd/my-lambda`（AWS Lambda。Lambda Web Adapter で `cmd/my` を流用できるなら不要。M6 で扱う）。
- コンテナイメージ: multi-arch（amd64 / arm64）、非 root、distroless 系。レジストリは `ghcr.io/mycroblog/my` だけとし、公開（public）にする（§4 から §10）。
- 利用者による拡張: 独自アダプタは「ポートの型を実装し、コントラクトテストを通し、独自エントリポイントで組み立てる」方式。
  Go の `plugin` パッケージによる動的ロードは使わない（移植性が低いため）。v0 ではフォークまたはツリー内への追加で拡張する。

### 3. デプロイテンプレート（`deploy/`）

今あるのは `deploy/docker` の Compose で、ほかは M6 で作る（`deploy/monitoring` は監視の設定）。`deploy/` に置くのはインスタンスを動かすものだけで、開発の構成はルートの `compose.yaml`（プロジェクト名 `my-dev`。`go run` のサーバーが使う PostgreSQL と Mailpit）、テストの構成は `test/`（`test/federation-live`、`test/upgrade-test`）に分ける。

| ターゲット | 形式 |
| --- | --- |
| Docker | `deploy/docker/compose.yaml`（SQLite 最小構成）に `compose.postgres.yaml`・`compose.s3.yaml`・`compose.caddy.yaml` を重ねる |
| microk8s / k8s | Kustomize（base + overlays）。Helm chart は需要次第 |
| Google Cloud | OpenTofu モジュール（Cloud Run, Cloud Tasks, GCS, Scheduler） |
| AWS | OpenTofu モジュール（Lambda, SQS, S3, CloudFront, EventBridge Scheduler） |

IaC は OSS ライセンスの OpenTofu を採用（Terraform 互換）。

Docker の構成は次のとおり。

- **設定**: 運営者は `deploy/docker/example.env` を `.env` にコピーする。`.env` は Compose の変数の展開に読まれ、`env_file` でそのままサーバーのコンテナに渡るので、設定リファレンスのすべての設定を `.env` で設定できる。`example.env` は設定スキーマから生成する（`go:config-docs`。古いと `go:test` が落ちる）。
- **KEK**: `.env` に書かない。`env_file` は 1 行ずつ環境変数にするので、`secrets/kek` に置き、Docker secret `/run/secrets/my_kek` を `MY_KEK_FILE` で読む。`.env` に `MY_KEK` があると 2 つが揃い、サーバーは起動を拒む。
- **Compose だけが読む変数**: `MY_VERSION` と `MY_HTTP_PORT` もコンテナに渡るので、未知の変数の警告から外す。
- **S3 互換ストア**: `compose.s3.yaml` は `blob = s3` にし、外部のストア（`.env` の `MY_BLOB_S3_*`）か、プロファイル `garage` で同じプロジェクトに立てる 1 ノードの Garage を使う。Garage にするのは、アダプタのコントラクトテストが動かすストアであり、公式の MinIO のイメージが匿名で取得できないためである。
- **一回限りのサービスの順序**: `upgrade-check`・`schema`・`upgrade`・サーバーの順で、重ねるファイルも変えない。

### 4. イメージのタグ

- 版のタグは `v` を付けない（`0.1.0`）。npm の `X.Y.Z`、多くの公式イメージに合わせる。Git のタグと `my version` は `v0.1.0` のまま。
- 次のとおり付ける。付け替えは公開時のワークフローだけが行う。

| 版 | `X.Y.Z` | `X.Y` |
| --- | --- | --- |
| その `X.Y` で最大のパッチの `vX.Y.Z`（`v0.2.0` の後の `v0.1.5` も、`0.1` だけ動かす） | 付ける（一度だけ） | 付け替える |
| その `X.Y` で最大ではないパッチ | 付ける（一度だけ） | 動かさない |
| pre-release（`v0.1.0-rc.1`） | `0.1.0-rc.1` だけ付ける | 動かさない |

- **`0` と `latest` を v0 の間は作らない。** どちらもマイナー版を越える。Kubernetes の `imagePullPolicy: Always` や `compose pull` が、知らない間に手順の要る版を持ち込む。`my db upgrade -check` が止めるのは contract の前提だけで、それ以外の飛ばしは検査を通るが保証の範囲外である。Compose 以外では `schema apply` も走らず、サーバーは 503 のままになる。
- `0.1` はパッチを受け取るためにある。パッチもスキーマの適用を要するので、一回限りのジョブを `up` が走らせる Compose には向くが、Kubernetes・Cloud Run では §6 のとおり固定する。
- v1 で `1` と `latest` を作るかは、v1 の方針を決めるときに決める。

### 5. `main` のビルド

**`v0.1.0` の時点では公開しない**（`edge` も `sha-<commit>` も作らない）。

- メンテナはリリースの版だけを動かす（ADR-0023 §4）ので要らない。試す人には rc がある。rc は同じ関門を通り、`release.yml` は手で何度でも動かせる。
- スローな検査を通っていない `main` の頭をレジストリに置かない。
- 覆す条件: rc の頻度では足りないと試す人が言ったら、Nightly のスローな検査が通った commit から `edge` と `sha-<commit>` を作り、`sha-` を 30 日で消す。Nightly はその commit を検査しているので、関門の位置はそこが正しい。

### 6. 不変性と保持

- `X.Y.Z` は作り直さない。公開時のワークフローは既存のタグを上書きしない。GHCR に不変タグの機能があるとは確認できなかったので（要検証）、ワークフローが守る。
- 運営者向けの文書は、本番では `0.1.0@sha256:...` の固定を勧める。ノートに digest を書く。
- **削除しない。** 版を通る規則のため、各マイナー版の最新のパッチは永く要る。digest の固定を壊さないよう古いパッチも残す。rc も残す。`v0.0.0-test.N` のイメージだけ、`docs/agents/releasing.md` の試行の手順で手で消す。
- 未タグの版の自動削除を入れない。multi-arch のプラットフォームのマニフェストや証明が未タグの版に見えるなら、消すとイメージが壊れる（GHCR の表示の扱いは要検証）。
- 欠陥のある版は消さず、次のパッチで置き換えてノートに書く。`X.Y` を戻す必要があれば、公開時のワークフローの `workflow_dispatch` で前のパッチの digest に付け替える。
- 基盤のイメージの修正は、新しいパッチ版として出す。

### 7. プラットフォームと種類

- `linux/amd64` と `linux/arm64`。スローな検査がすでに両方をビルドしている。
- **`GO_TAGS` は空の 1 種類だけ**（すべてのアダプタ）。同じイメージがどのホスティングでも動くことを優先する。小さいイメージが欲しい運営者は `scripts/task.sh docker:build GO_TAGS=...` で作れる。
- `e2e_fakes` は 3 か所で止める: `Dockerfile` のタグの検査、`Dockerfile` のバイナリの検査、公開の前に各プラットフォームのイメージからバイナリを取り出して走らせる `scripts/check-release-build.sh`。
- バイナリは **`v0.1.0` では Release に添付しない。** 今の配布の手順はコンテナを前提にしていて、添付すると手順や文書も増える。あとから足しても互換を壊さない。足すときはイメージから取り出す。
- ホスティング: Compose と Kubernetes はそのまま、Cloud Run は公開の `ghcr.io` を直接使える。AWS は ECR へコピーする（pull through cache も使える）。Lambda は 1 アーキテクチャのイメージの ECR へのコピーと runtime interface client が要り、公開するイメージはそのままでは動かない。M6 のテンプレートで扱う。

### 8. サプライチェーン

- **来歴と署名は `actions/attest` にまとめる。** 公開リポジトリでは Sigstore の keyless 署名になり、workflow の ID と commit が digest に結び付く。npm の provenance と同じ考え方である。
- **cosign の署名は足さない。** 署名の基盤は同じ Sigstore で、足すと書き込みと OIDC の権限を持つジョブの依存が増える（Trivy の事件を参照）。覆す条件: 運営者の入場制御（policy-controller など）が `actions/attest` の証明を読めないと分かったとき（要検証）。
- **SBOM は buildx の `sbom: true`** で、インデックスに入れる。インデックスの digest に対する来歴がこれも覆う。`web` ステージに `ARG BUILDKIT_SBOM_SCAN_STAGE=true` を置く。既定の最終ステージだけでは、Go の embed に入る npm パッケージが載らない。開発用のパッケージも載る点は受け入れる。
- **buildx の来歴は切る**（`provenance: false`）。署名のない来歴が重なるだけで、役に立つのは署名のある方である。
- **脆弱性の検査は止めない報告にする。** 別のジョブで、書き込み権限なしに、digest で固定した Grype のコンテナでイメージを調べる。
  - リリース: 下書きの段階でジョブの要約に出し、決定者が読んで判断する。自動で止めると、タグの後で止まり、修正のない指摘で出せなくなる。
  - Nightly: 維持している系列の最新のパッチを調べ、修正のある HIGH 以上で失敗させる（`--only-fixed`・`--fail-on`、フラグは要検証）。既存の Nightly の対応の流れに乗り、パッチを出す。
  - Trivy でもよい。選ぶ理由は事件より、特権なしで動かす形にできることである。
- **Dependabot**: `Dockerfile` の `FROM` を `ARG` を介さず直接書く形にして、更新できるようにする（`docker:dockerfile-check` も直す）。Go・Node.js・Bun のタグは `.tool-versions` と揃える必要があるので手で上げ続け、Dependabot は digest を上げる。実際に PR が来ることを確かめる（要検証）。
- **運営者の確認**（`docs/operations/` に書く）:

  ```sh
  gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
    --signer-workflow mycroblog/my/.github/workflows/release.yml
  docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
  ```

### 9. いつ公開するか

`release.yml` がタグなし（digest だけ）で push して署名し、公開時に**ビルドせず**タグを付ける。

- **`release.yml`（下書きの段階）**: `release` の後に、`image`（1 のビルドと push）・`image-check`（2 の確認）・`image-attest`（3 の証明と添付）のジョブを順に置き、`image-scan` と `release-notes-image` が続く。`npm` ジョブと同じ位置で、スローな検査を通った `github.sha` をチェックアウトする。
  1. キャッシュなしで、両プラットフォームをタグなしで push する（`push-by-digest`）。`VERSION` は `inputs.version`。
  2. 各プラットフォームのバイナリを取り出し、`check-release-build.sh` と `my version` を確かめる。
  3. digest に `actions/attest` を実行し、digest を `image-digest.txt` として下書きの Release に添付する（`--clobber`）。ジョブの要約にも出す。
  - 失敗したら、npm と同じく失敗したジョブを再実行する。別の digest ができても、古いものは参照されないだけである。
  - 決定者は下書きの見直しの中で digest を動かせる。ノートに digest を書く。
- **ワークフロー `release-image.yml`（公開の時）**: `release: published` で動く。決定者が公開するので `GITHUB_TOKEN` の制限に当たらない。やり直し用に `workflow_dispatch` も持つ。**ビルドしない。**
  1. 添付の digest を読み、証明を確かめ、来歴の commit がタグの commit と同じことを確かめる。合わなければ何もタグを付けず失敗する。
  2. §4 の表のとおりタグを付ける（`scripts/release-image.sh`。単体テストする。`scripts/previous-minor.sh` と同じ形）。
  3. タグの digest が元と同じことを確かめ、匿名で pull できることを確かめる。
- 公開前の Git のタグと npm は下書きの段階で公開される。下書きを捨てた版は、npm と同じくその番号を使わない。
- 検査したイメージそのものではない。検査と公開で同じ commit、固定の基盤イメージ、固定のロックファイルからビルドし、違いは `VERSION` の埋め込みだけのはずである（ビットの一致は保証しない）。

### 10. メタデータと AGPL

- ラベルと annotation（インデックスとマニフェストの両方）: `org.opencontainers.image.source`=`https://github.com/mycroblog/my`、`revision`=commit の完全な SHA、`version`=`0.1.0`、`licenses`=`AGPL-3.0-only`、`title`、`description`、`url`、`documentation`、`created`（commit の時刻）、`base.name`、`base.digest`。
- **`source_url` の既定を版のソースに向ける。** 設定がなく、埋め込まれた版がリリースのタグの形（`vX.Y.Z`、`vX.Y.Z-pre`）なら、`GET /instance` の `source_url` は `https://github.com/mycroblog/my/tree/<version>` を返す。それ以外（`git describe` の途中の形、`dev`）はリポジトリの根を返す。
  - 設定で変えられること、改変版の運営者は自分のソースを指すことは変わらない。NodeInfo の `software.repository` はリポジトリなので根のままとする。
  - タグが消えると指す先も消えるので、Immutable releases を有効にする（Release のあるタグを消せなくなる）。

### 11. 覆す条件

- digest だけの push とその digest への証明は、rc の `release-image.yml` で通っている。GHCR がこれらを受け付けなくなったら、`0.1.0` の一度だけのタグを `release.yml` で付ける形に移る。
- `latest` がないために実際に利用者が離れる、または文書で防げないつまずきが続くなら、`latest` だけを足す。
- GHCR の無料の扱いが変わったら、保持の規則と別のレジストリを考える。

## 経緯

利用者が自由にカスタマイズでき（G3）、3 系統のターゲットに配れる（G2, NFR-DIST-2）必要があり、§1 から §3 を決めた。

M5（エピック #30）で `ghcr.io/mycroblog/my` のイメージが要り、§4 から §11 を足した。効いた制約は次のとおり。`release.yml` は `github.sha` でスローな検査を走らせて同じ commit にタグを付ける。下書きの Release を決定者が見直して公開し、そのノートに運営者の更新の手順（**Upgrading**）がある。更新の保証は同じマイナー版の中と 1 つ前のマイナー版の最新のパッチからだけで（ADR-0023 §7）、パッチ版も `schema apply` と `my db upgrade` を要することがあり、足りなければ `/readyz` が 503 を返す。判断の基準は、運営者が手順を知らないまま別の版を受け取らないこと、検査した commit からだけ作ること、どのホスティングでも同じイメージで使えること、出所と版のソースをたどれること（AGPL）、途中で失敗してもやり直せること、特権のあるジョブの依存を増やさないことである。

判断の元にした事実（2026-11-03 に確認）: GHCR の新しいパッケージは非公開が既定で、公開イメージは匿名で pull でき、保存と転送は現在無料（[Billing](https://docs.github.com/en/billing/managing-billing-for-github-packages/about-billing-for-github-packages)）。5,000 回を超えてダウンロードされた公開の版は API で消せない（[REST](https://docs.github.com/en/rest/packages/packages)）。`published` は下書きからの公開で発火し pre-release も含む（[Events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)）。buildx の SBOM は既定で最後のステージだけを調べる（[SBOM](https://docs.docker.com/build/metadata/attestations/sbom/)）。Immutable releases は公開した Release のタグとアセットを固定する（[Immutable releases](https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases)）。2026 年 3 月に `trivy-action` のタグの 76/77 が書き換えられ、Aqua は commit SHA への固定だけが確実と述べた（[Aqua](https://www.aquasec.com/blog/trivy-supply-chain-attack-what-you-need-to-know)）。

## 採らなかった案

- すべて `release.yml` で全タグを付ける: 1 つのワークフローで済むが、ノートより先に `0.1` が動き、更新の手順のない版が運営者に届く。
- すべて公開時にビルドする: 下書きの段階で何も公開されないが、公開の後にビルドや証明が失敗するとイメージのない Release が見え、決定者は動かして確かめられず、ノートに digest を書けない。
- `release.yml` で不変のタグ `0.1.0`、公開時に動くタグ: 動くタグだけ守れるが、ノートより先に `0.1.0` が見え、やり直しで既存のタグとの扱いが要る。§11 の移り先である。
- イメージを先に作りスローな検査がそれを試す: 検査したものそのものを出せるが、検査の組み直しが大きく、失敗した候補がレジストリに残り、E2E の多くはイメージを使わない。
- `latest`（`0` の有無を問わず）: 試すだけの人には最短でタグなしのツールも動く。つまずきは一度のエラーで済むのに対し、作った場合の最悪は運用中のインスタンスの停止である。
- `edge`: 変更をすぐ試せて次のリリースへも上がれるが、`v0.1.0` の時点で試す人は rc で足りる。足すのはいつでもできる。
- cosign: `cosign verify` は広く知られ `gh` なしで確かめられ、1 ステップで足せる。入場制御で読めないと分かれば足す。
- バイナリの添付: 単一の静的バイナリで費用は小さく VM でも動くが、需要が出てから足す。

## Consequences（影響）

- 設定スキーマが「どのアダプタが何を要求するか」のドキュメントを兼ねる（スキーマからドキュメント生成）。
- テンプレート 5 種の保守負荷。M6 で各ターゲットの E2E スモークテストを nightly 化する。
- 動くタグはノートと同時に動き、マイナー版を越えない。公開時のビルドがないので、公開の後に壊れる余地が小さい。
- 運営者はどのホスティングでも同じ digest を使い、出所と版のソースを確かめられる。特権のあるジョブが小さく、検査は権限なしで走る。
- 慣習の `latest` がない。文書の最初の例はかならず版を書く。タグなしの `docker pull ghcr.io/mycroblog/my` は `manifest unknown` で失敗する。
- ワークフローが 2 つに分かれ、タグの計算のスクリプトが増える。
- タグを付けるワークフローの失敗で、公開の後の数分間 pull できないことがある（再実行で直る）。
- 下書きを捨てると digest だけの版が残る（参照されず、害はない）。
- 検査したものとビルドが別である（§9）。
- rc のイメージもずっと残る。rc のたびに npm の番号も消費する。
- Nightly のイメージの検査の失敗は、`main` の不具合ではなく公開済みの版の問題を指す。
- `gh` の入っていない環境では、来歴の確認に手間がかかる。
- 決定者の一度だけの作業: パッケージを公開（public）にする（最初の push は非公開になるので、公開時のワークフローの匿名 pull の確認はこの設定まで失敗する。設定して再実行する。rc の実行は匿名 pull まで通っている）。リポジトリの設定で Immutable releases を有効にする（§10）。

## Open Questions / 要検証

- GHCR の次の動きは文書で確かめられなかった: プラットフォームのマニフェストと証明の未タグの版としての表示、公開イメージへの `gh attestation verify` に `docker login` が要るか、匿名 pull の制限。
- syft が `-s -w` のバイナリから Go のモジュールを読むこと、`BUILDKIT_SBOM_SCAN_STAGE` の結果に `web` の依存が載ることを、rc の SBOM で確かめる。
- Grype の `--only-fixed`・`--fail-on` は公式の文書で確認できなかった（2026-11-03）。
- distroless が含む Debian パッケージのソースの提供をどう扱うかは法的な確認の範囲で、この ADR では断定しない。
