# ADR-0142: コンテナイメージの公開: GHCR、タグ、サプライチェーン

- Status: Accepted
- Date: 2026-11-03
- Deciders: @rosylilly
- Issue: #2022

## Context（背景・課題）

`release.yml` はタグ、下書きの Release、npm パッケージを作るが、コンテナイメージを公開しない。[ADR-0010](0010-configuration-and-packaging.md) は multi-arch・非 root・distroless のイメージを GHCR に置くと決めている。
M5（エピック #30）には `ghcr.io/mycroblog/my` のイメージが要る。`v0.1.0` はメンテナが運用を始める版で、運営者のホスティングを限定しない。その前に rc を動かし、rc の DB は作り直してよい（#2027）。

効く制約:

- **検査した commit**: `release.yml` は `github.sha` でスローな検査を走らせ、通ったときだけ同じ commit にタグを付ける。イメージもこの commit から作る。
- **下書きの Release**: 下書きを決定者が見直して公開する。npm は下書きの段階で公開している。Release のノートには、運営者の更新の手順（**Upgrading**）がある。
- **版を通る規則**（[ADR-0087](0087-schema-changes-after-launch.md) §4）: 保証するのは同じマイナー版の中と、1 つ前のマイナー版の最新のパッチからの更新だけである。パッチ版も `schema apply` と `my db upgrade` を要することがある。`my serve` は自分では適用せず、足りなければ `/readyz` が 503 を返す。
- **メンテナのインスタンスはリリースの版だけを動かす**（ADR-0087 §1）。

### 確かめた事実（2026-11-03）

| 項目 | 事実 | 出典 |
| --- | --- | --- |
| GHCR の公開 | 新しいパッケージは非公開が既定。公開イメージは匿名で pull できる。`GITHUB_TOKEN` で publish できる | [Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) |
| GHCR の費用 | コンテナの保存と転送は「現在無料」、変更は 1 か月前に通知。保持の規則や上限の記述はない | [Billing](https://docs.github.com/en/billing/managing-billing-for-github-packages/about-billing-for-github-packages) |
| GHCR の削除 | 5,000 回を超えてダウンロードされた公開の版は API で消せない。消したパッケージは 30 日以内なら戻せる | [REST](https://docs.github.com/en/rest/packages/packages)、[Deleting](https://docs.github.com/en/packages/learn-github-packages/deleting-and-restoring-a-package) |
| GHCR の不変タグ | 公式の文書に不変タグの機能の記述は見つからない（要望の [Discussion](https://github.com/orgs/community/discussions/181783) はある）。**要検証** | 同上 |
| OCI のラベル | `source`・`description`（512 文字まで）・`licenses` を GHCR が読む。multi-arch ではインデックスの annotation の `description` を読む | [Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) |
| 来歴の証明 | `actions/attest-build-provenance` は v4 から `actions/attest` の薄い包みで、新しい実装は `actions/attest` を使う。公開リポジトリは全プランで使え、Sigstore の公共のインスタンスで署名する | [README](https://github.com/actions/attest-build-provenance) |
| 来歴の権限と確認 | `id-token: write`・`contents: read`・`attestations: write`・`packages: write`、`subject-name` + `subject-digest` + `push-to-registry`。確認は `gh attestation verify oci://...`。文書は先に `docker login ghcr.io` を求める | [Use attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations) |
| buildx の SBOM | `sbom: true`。SPDX。既定では**最後のステージだけ**を調べる。`BUILDKIT_SBOM_SCAN_STAGE` を Dockerfile の `ARG` にするとそのステージも調べる | [SBOM](https://docs.docker.com/build/metadata/attestations/sbom/) |
| buildx の来歴 | 公開リポジトリの `build-push-action` は既定で `mode=max`。build arg の値を含む | [GitHub Actions](https://docs.docker.com/build/ci/github-actions/attestations/) |
| cosign keyless | 一時鍵で OIDC により署名し、署名はレジストリに置く。確認は `--certificate-identity` と `--certificate-oidc-issuer` | [Signing](https://docs.sigstore.dev/cosign/signing/signing_with_containers/)、[Verify](https://docs.sigstore.dev/cosign/verifying/verify/) |
| Release の event | `published` は下書きからの公開で発火し、pre-release も含む。下書きの `created`・`edited`・`deleted` は発火しない。`GITHUB_TOKEN` の操作は新しい実行を作らない（`workflow_dispatch` を除く） | [Events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)、[GITHUB_TOKEN](https://docs.github.com/en/actions/concepts/security/github_token) |
| Immutable releases | 公開した Release のタグとアセットを固定する。下書きの間は固定しない。イメージの記述はない | [Immutable releases](https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases) |
| Cloud Run | 公開の `ghcr.io` を直接使える。タグはデプロイで digest に解決する | [Deploying](https://docs.cloud.google.com/run/docs/deploying) |
| ECR / Lambda | ECR の pull through cache は GHCR に対応（認証情報が要る）。Lambda は pull through cache も multi-arch も使えず、runtime interface client が要る | [ECR](https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache.html)、[Lambda](https://docs.aws.amazon.com/lambda/latest/dg/images-create.html) |
| Trivy の事件 | 2026 年 3 月、`trivy-action` のタグの 76/77 が悪意のある commit へ書き換えられ、Trivy v0.69.4 も汚染された。Aqua は commit SHA への固定だけが確実と述べる | [Aqua](https://www.aquasec.com/blog/trivy-supply-chain-attack-what-you-need-to-know) |

### リポジトリの事実

- `Dockerfile` の直近の 14 の commit のどれも Dependabot のものではなく、`FROM ${GO_IMAGE}` の形を更新できていないと見られる。GitHub の文書の更新（[github/docs#36563](https://github.com/github/docs/pull/36563/files)）は `ARG` のイメージを更新の対象外と書いている（今の文書での記述は**要検証**）。
- CI に `govulncheck` もイメージの検査もない。依存の脆弱性は Dependabot Alerts だけで見ている。
- 最終ステージに `RUN` がなく、Go はクロスコンパイルで、Web のステージはビルド環境で動くので、エミュレーションなしで両方のアーキテクチャを作れる。

## Decision Drivers（判断基準）

1. 運営者が、更新の手順を知らないまま別の版を受け取らない（データと稼働を守る）
2. 公開するものは、スローな検査を通した commit から作ったものだけで、テスト用のビルドを含まない
3. どのホスティングでも同じイメージで使える
4. 運営者が出所を確かめられ、版のソースへたどれる（AGPL）
5. 公開の途中で失敗しても、やり直せて、半端な状態が運営者に届かない
6. 特権のあるジョブの依存を増やさない。コストはゼロに近い

## Considered Options（選択肢）

### いつ公開するか（問 6）

| | 内容 | 最も強い論拠 | 最悪の場合 |
| --- | --- | --- | --- |
| A | すべて `release.yml` で（全タグ） | 1 つのワークフローで済む | ノートより先に `0.1` が動き、更新の手順のない版が運営者に届く |
| B | すべて公開時にビルド | 下書きの段階で何も公開されない。ワークフローが自然に分かれる | 公開の後でビルドや証明が失敗し、イメージのない Release が見える。決定者は動かして確かめずに公開する。ノートに digest を書けない |
| C | `release.yml` で不変のタグ `0.1.0`、公開時に動くタグ | 動くタグだけ守れる。下書きの間に版を試せる | ノートより先に `0.1.0` が見える。やり直しでは既存のタグとの扱いが要る |
| D（採用） | `release.yml` で**タグなし（digest だけ）**で push して署名、公開時に**ビルドせず**タグを付ける | ノートより先にタグが 1 つも現れない。下書きの間に digest で動かせる。ノートに digest を書ける。やり直しが単純 | タグを付けるワークフローの失敗で、公開の後の数分間 pull できない（再実行で直る）。下書きを捨てると digest だけの版が残る |
| E | イメージを先に作り、スローな検査がそのイメージを試す | 検査したものそのものを出せる | 検査の組み直しが大きい。失敗した候補がレジストリに残る。E2E の多くはイメージを使わないので、得られる保証は一部 |

### タグ（問 1）

| | 内容 | 最も強い論拠 | 最悪の場合 |
| --- | --- | --- | --- |
| 1 | `0.1.0`・`0.1`・`0`・`latest` | 慣習どおりで、タグなしの `docker pull` が動く | 再起動や `pull` が黙ってマイナー版を越え、インスタンスが止まる |
| 2 | `0.1.0`・`0.1`・`latest`（`0` なし） | 最初の試用が楽 | 同上（`latest`） |
| 3（採用） | `0.1.0`・`0.1` だけ。v0 の間 `0` と `latest` はない | 更新の保証の範囲と一致する | `docker pull ghcr.io/mycroblog/my` が `manifest unknown` で失敗する。慣習に慣れた人は一度つまずく。タグなしを前提にするツールは使えない |

## Decision（決定）

**D と 3 を採用する。** レジストリは ADR-0010 のとおり `ghcr.io/mycroblog/my` だけとし、公開（public）にする。

### 1. タグ（問 1）

- 版のタグは `v` を付けない（`0.1.0`）。npm の `X.Y.Z`、多くの公式イメージに合わせる。Git のタグと `my version` は `v0.1.0` のまま。`docs/operations/database.md` の `:v0.2.0` の例は直す。
- 次のとおり付ける。付け替えは公開時のワークフローだけが行う。

| 版 | `X.Y.Z` | `X.Y` |
| --- | --- | --- |
| その `X.Y` で最大のパッチの `vX.Y.Z`（`v0.2.0` の後の `v0.1.5` も、`0.1` だけ動かす） | 付ける（一度だけ） | 付け替える |
| その `X.Y` で最大ではないパッチ | 付ける（一度だけ） | 動かさない |
| pre-release（`v0.1.0-rc.1`） | `0.1.0-rc.1` だけ付ける | 動かさない |

- **`0` と `latest` を v0 の間は作らない。** どちらもマイナー版を越える。Kubernetes の `imagePullPolicy: Always` や `compose pull` が、知らない間に手順の要る版を持ち込む。`my db upgrade -check` が止めるのは contract の前提だけで、それ以外の飛ばしは検査を通るが保証の範囲外である。Compose 以外では `schema apply` も走らず、サーバーは 503 のままになる。
- `0.1` はパッチを受け取るためにある。パッチもスキーマの適用を要するので、一回限りのジョブを `up` が走らせる Compose には向くが、Kubernetes・Cloud Run では §3 のとおり固定する。
- v1 で `1` と `latest` を作るかは、v1 の方針を決めるときに決める。

### 2. `main` のビルド（問 2）

**`v0.1.0` の時点では公開しない**（`edge` も `sha-<commit>` も作らない）。

- メンテナはリリースの版だけを動かす（ADR-0087 §1）ので要らない。試す人には rc がある。rc は同じ関門を通り、`release.yml` は手で何度でも動かせる。
- スローな検査を通っていない `main` の頭をレジストリに置かない。
- 覆す条件: rc の頻度では足りないと試す人が言ったら、Nightly のスローな検査が通った commit から `edge` と `sha-<commit>` を作り、`sha-` を 30 日で消す。Nightly はその commit を検査しているので、関門の位置はそこが正しい。

### 3. 不変性と保持（問 3）

- `X.Y.Z` は作り直さない。公開時のワークフローは既存のタグを上書きしない。GHCR に不変タグの機能があるとは確認できなかったので（要検証）、ワークフローが守る。
- 運営者向けの文書は、本番では `0.1.0@sha256:...` の固定を勧める。ノートに digest を書く。
- **削除しない。** 版を通る規則のため、各マイナー版の最新のパッチは永く要る。digest の固定を壊さないよう古いパッチも残す。rc も残す。`v0.0.0-test.N` のイメージだけ、`docs/operations/releasing.md` の試行の手順で手で消す。
- 未タグの版の自動削除を入れない。multi-arch のプラットフォームのマニフェストや証明が未タグの版に見えるなら、消すとイメージが壊れる（GHCR の表示の扱いは要検証）。
- 欠陥のある版は消さず、次のパッチで置き換えてノートに書く。`X.Y` を戻す必要があれば、公開時のワークフローの `workflow_dispatch` で前のパッチの digest に付け替える。
- 基盤のイメージの修正は、新しいパッチ版として出す。

### 4. プラットフォームと種類（問 4）

- `linux/amd64` と `linux/arm64`。スローな検査がすでに両方をビルドしている。
- **`GO_TAGS` は空の 1 種類だけ**（すべてのアダプタ）。同じイメージがどのホスティングでも動くことを優先する。小さいイメージが欲しい運営者は `scripts/task.sh docker:build GO_TAGS=...` で作れる。
- `e2e_fakes` は 3 か所で止める: `Dockerfile` のタグの検査、`Dockerfile` のバイナリの検査、公開の前に各プラットフォームのイメージからバイナリを取り出して走らせる `scripts/check-release-build.sh`。
- バイナリは **`v0.1.0` では Release に添付しない。** 今の配布の手順はコンテナを前提にしていて、添付すると手順や文書も増える。あとから足しても互換を壊さない。足すときはイメージから取り出す。
- ホスティング: Compose と Kubernetes はそのまま、Cloud Run は公開の `ghcr.io` を直接使える。AWS は ECR へコピーする（pull through cache も使える）。Lambda は 1 アーキテクチャのイメージの ECR へのコピーと runtime interface client が要り、公開するイメージはそのままでは動かない。M6 のテンプレートで扱う。

### 5. サプライチェーン（問 5）

- **来歴と署名は `actions/attest` にまとめる。** 公開リポジトリでは Sigstore の keyless 署名になり、workflow の ID と commit が digest に結び付く。npm の provenance と同じ考え方である。
- **cosign の署名は足さない。** 署名の基盤は同じ Sigstore で、足すと書き込みと OIDC の権限を持つジョブの依存が増える（Trivy の事件を参照）。覆す条件: 運営者の入場制御（policy-controller など）が `actions/attest` の証明を読めないと分かったとき（要検証）。
- **SBOM は buildx の `sbom: true`** で、インデックスに入れる。インデックスの digest に対する来歴がこれも覆う。`web` ステージに `ARG BUILDKIT_SBOM_SCAN_STAGE=true` を足す。既定の最終ステージだけでは、Go の embed に入る npm パッケージが載らない。開発用のパッケージも載る点は受け入れる。
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

### 6. いつ公開するか（問 6）

- **`release.yml`（下書きの段階）**: `release` の後に `image` ジョブを足す。`npm` ジョブと同じ位置で、スローな検査を通った `github.sha` をチェックアウトする。
  1. キャッシュなしで、両プラットフォームをタグなしで push する（`push-by-digest`）。`VERSION` は `inputs.version`。
  2. 各プラットフォームのバイナリを取り出し、`check-release-build.sh` と `my version` を確かめる。
  3. digest に `actions/attest` を実行し、digest を `image-digest.txt` として下書きの Release に添付する（`--clobber`）。ジョブの要約にも出す。
  - 失敗したら、npm と同じく失敗したジョブを再実行する。別の digest ができても、古いものは参照されないだけである。
  - 決定者は下書きの見直しの中で digest を動かせる。ノートに digest を書く。
- **新しいワークフロー `release-image.yml`（公開の時）**: `release: published` で動く。決定者が公開するので `GITHUB_TOKEN` の制限に当たらない。やり直し用に `workflow_dispatch` も持つ。**ビルドしない。**
  1. 添付の digest を読み、証明を確かめ、来歴の commit がタグの commit と同じことを確かめる。合わなければ何もタグを付けず失敗する。
  2. §1 の表のとおりタグを付ける（`scripts/` に置いて単体テストする。`scripts/previous-minor.sh` と同じ形）。
  3. タグの digest が元と同じことを確かめ、匿名で pull できることを確かめる。
- 公開前の Git のタグと npm は今のまま公開される。下書きを捨てた版は、npm と同じくその番号を使わない。
- 検査したイメージそのものではない。検査と公開で同じ commit、固定の基盤イメージ、固定のロックファイルからビルドし、違いは `VERSION` の埋め込みだけのはずである（ビットの一致は保証しない）。

### 7. メタデータと AGPL（問 7）

- ラベルと annotation（インデックスとマニフェストの両方）: `org.opencontainers.image.source`=`https://github.com/mycroblog/my`、`revision`=commit の完全な SHA、`version`=`0.1.0`、`licenses`=`AGPL-3.0-only`、`title`、`description`、`url`、`documentation`、`created`（commit の時刻）、`base.name`、`base.digest`。
- **`source_url` の既定を版のソースに向ける。** 設定がなく、埋め込まれた版がリリースのタグの形（`vX.Y.Z`、`vX.Y.Z-pre`）なら、`GET /instance` の `source_url` は `https://github.com/mycroblog/my/tree/<version>` を返す。それ以外（`git describe` の途中の形、`dev`）はリポジトリの根を返す。
  - 設定で変えられること、改変版の運営者は自分のソースを指すことは今のまま。NodeInfo の `software.repository` はリポジトリなので根のままとする。
  - タグが消えると指す先も消えるので、Immutable releases を有効にする（Release のあるタグを消せなくなる）。

### 覆す条件

- GHCR が digest だけの push や、その digest への証明を受け付けないと rc の前の試行で分かったら、C に移る。`0.1.0` の一度だけのタグを `release.yml` で付ける。
- `latest` がないために実際に利用者が離れる、または文書で防げないつまずきが続くなら、`latest` だけを足す。
- GHCR の無料の扱いが変わったら、保持の規則と別のレジストリを考える。

## Consequences（影響）

- 良い影響
  - 動くタグはノートと同時に動き、マイナー版を越えない。版を通る規則とタグが一致する
  - 公開時のビルドがないので、公開の後に壊れる余地が小さい
  - 運営者はどのホスティングでも同じ digest を使い、出所と版のソースを確かめられる
  - 特権のあるジョブが小さい。検査は権限なしで走る
- 悪い影響・トレードオフ
  - 慣習の `latest` がない。文書の最初の例はかならず版を書く
  - ワークフローが 2 つに分かれ、タグの計算のスクリプトが増える
  - 下書きを捨てると digest だけの版が残る（参照されず、害はない）
  - 検査したものとビルドが別である（§6）
  - rc のイメージもずっと残る。rc のたびに npm の番号も消費する
  - Nightly のイメージの検査の失敗は、`main` の不具合ではなく公開済みの版の問題を指す
  - `gh` の入っていない環境では、来歴の確認に手間がかかる
- 決定者の一度だけの作業
  - 最初の rc の前に、パッケージを公開（public）にする。最初の push は非公開になるので、公開時のワークフローの匿名 pull の確認はこの設定まで失敗する。設定して再実行する
  - リポジトリの設定で Immutable releases を有効にする
- 後続で必要になる作業（Accepted のあとに Issue にする）
  - `release.yml` の `image` ジョブ、`release-image.yml`、タグの計算のスクリプトとテスト、`Dockerfile` の `FROM` と SBOM の `ARG`
  - `slow-suite` の `upgrade` ジョブで公開済みのイメージを使う（`MY_PREVIOUS_IMAGE`）
  - `deploy/docker/compose.yaml` と `.env.example` を公開イメージに向ける。README のクイックスタート
  - `source_url` の既定、`docs/reference/configuration.md` の生成
  - 文書: `docs/operations/releasing.md`（「What a release produces」、下書きでの digest の確認、一度だけの作業、試行で作ったイメージの削除）、運営者向けの確認とホスティングごとの手順、`docs/operations/database.md` のタグの例
  - Nightly のイメージの検査
  - `release.yml` の冒頭のコメント

## Pros and Cons of the Options（詳細比較・任意）

- **`latest` を作る側の論拠は軽くない**: 試すだけの人には最短で、タグなしを前提にするツールも動く。ただしつまずきは一度のエラーで済み、作った場合の最悪は運用中のインスタンスの停止である。最悪の場合の差で 3 を選ぶ。
- **`edge` を作る側の論拠**: 変更をすぐ試せて、ADR-0087 が PR ごとの更新を保証するので `edge` から次のリリースへも上がれる。ただし `v0.1.0` の時点で試す人は rc で足りる。足すのはいつでもできる。
- **cosign を足す側の論拠**: `cosign verify` は広く知られ、GitHub の API や `gh` がなくても確かめられる。1 ステップで足せる。入場制御で読めないと分かれば足す。
- **バイナリを添付する側の論拠**: 単一の静的バイナリなので費用は小さく、コンテナを使わない VM でも動く。需要が出てから足す。

## Open Questions / 要検証

- GHCR の次の動きは文書で確かめられなかった。`v0.0.0-test.N` の試行で確かめる: digest だけの push、その digest への `push-to-registry` の証明、プラットフォームのマニフェストと証明の未タグの版としての表示、公開イメージへの `gh attestation verify` に `docker login` が要るか、匿名 pull の制限。
- `docker buildx imagetools create` は、単一のインデックスから「カーボンコピー」を作ると文書にあるが、digest を保つとは明記がない（[docs](https://docs.docker.com/reference/cli/docker/buildx/imagetools/create/)）。ワークフローが一致を確かめ、合わなければ `crane tag` に替える。
- syft が `-s -w` のバイナリから Go のモジュールを読むこと、`BUILDKIT_SBOM_SCAN_STAGE` の結果に `web` の依存が載ることを、rc の SBOM で確かめる。
- Grype の `--only-fixed`・`--fail-on` は公式の文書で確認できなかった（2026-11-03）。
- distroless が含む Debian パッケージのソースの提供をどう扱うかは法的な確認の範囲で、この ADR では断定しない。
