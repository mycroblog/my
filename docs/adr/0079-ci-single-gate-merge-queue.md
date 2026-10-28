# ADR-0079: CI の検査を Merge Queue に一本化する

- Status: Accepted（ジョブの中身を `task ci:<ジョブ>` 1 つにする部分を ADR-0083（CI のジョブはワークフローのステップで組み、go-task は手元の入口にする） が、手元の `check:changed` を Merge Queue の前の関門にする部分を ADR-0119（Merge Queue の検査を唯一の関門にし、手元の検査を必須にしない） が置き換える）
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1052

## Context（背景・課題）

[ADR-0047](0047-ci-protection-layers.md) は、検査を PR・Merge Queue・main への push・nightly に振り分け、
文脈（`scripts/changed-contexts.sh`）ごとの再利用ワークフローに分けた。
その後、エージェントが 4〜5 本ずつ並行して PR を出す運用（[workflow.md §9](../agents/workflow.md#9-parallel-batches)）が続いている。
ここで、同じ検査の繰り返しと runner の取り合いが、並列の作業のリードタイムを決めるようになった。

決定者の要望（#1052、原文）:

> CI / Merge Queue を使うかどうかというレベルから再設計したいです。
> 現状同じような内容のテストが重複して走っているだけにも思えるし、adr-index などの生成系も言い方は悪いですがコンフリクトの温床にしかなっておらず、並列作業に対するリードタイム悪化を招いています。
> GitHub Actions も同様に並列作業でエンキューされたものが無駄にキューを圧迫していて、これも根本的に解決したいし、中身が `make ~~~` なのでいろいろ直列に実行していて、本質的に並列できそうなものも整備されない・されにくい状態になっている

関係する要件は NFR-DEV-1（1 つのコマンドで検査が走る）と NFR-DEV-2（外部サービスなしで検査が完結する）。
タスクの実行は [ADR-0080](0080-task-runner-go-task.md)、生成物は [ADR-0081](0081-generated-files-policy.md)、
衝突しにくいファイルの構成は [ADR-0082](0082-conflict-free-file-layout.md) が決める。この ADR は、どこで何を検査するかを決める。

### 実測（2026-10-20〜23）

`gh api` で取った CI と Main のジョブ約 5,800 件の `created_at` / `started_at` / `completed_at` から測った。秒の値は「中央値 / p90」。

| 区分 | 実行 | 結果まで | 3 日間の runner 時間 | 気づいたこと |
| --- | --- | --- | --- | --- |
| PR（`ci.yml`） | 322 回 | 280 / 748 | 52.5 h | 失敗は 4 回。ジョブの起動待ちの p90 が 5〜8 分（`docker-smoke` 498、`docker-arm64` 352、`go` 310） |
| Merge Queue（`ci.yml`） | 216 回 | 282 / 490 | 28.5 h | 失敗は 2 回。中身は PR とほぼ同じ |
| main への push（`main.yml`） | 191 回 | 810 / 929 | 83.6 h | 114 回（60%）が次の push で取り消し。E2E の 4 シャード（各約 10 分）が捨てられる |
| CodeQL | 188 回 | — | 未計測 | main への push ごと。70 回が取り消し |

- 1 つの変更は、エージェントの手元の `make check-changed`、PR の `check`、Merge Queue の `check` の 3 回、同じ文脈の検査を通る。
  PR と Merge Queue の失敗はどちらも 1% 前後で、手元の検査を通ったものがほとんどそのまま通っている。
- Merge Queue が止めた 2 回のうち 1 回は、重ねて初めて壊れる型の不一致だった。一方の PR が `reactions` を必須にし、
  もう一方が E2E のフィクスチャを足していた（`tsc`、run 39385861802）。Merge Queue の価値は、統合した状態でコンパイル・型検査・テストを通すことにある。
- Free プランの GitHub-hosted runner は、同時に 20 ジョブまで動く。キャッシュはリポジトリあたり 10 GB まで
  （[Actions limits](https://docs.github.com/en/actions/reference/limits)、2026-10-23 確認）。
  PR 1 回は最大約 14 ジョブに分かれ、ジョブごとの準備（checkout・ツールの導入・キャッシュの復元）に 60〜165 秒かかる。
- Go のキャッシュは `go-test` が 5.1 GB、`go-lint` が 2.9 GB ある。`go-test` のキャッシュの復元（165 秒）は、テスト本体（133 秒）より長い。
  `go` ジョブでは golangci-lint が 2 回走る（action と `make go-lint`）。
- 14 日間で PR のマージ 235 件に対し、ブランチに main をマージしたコミットが 138 件ある。衝突のたびに Merge Queue から外れ、
  PR と Merge Queue の CI が両方とも最初から回り直す（衝突の原因は ADR-0081・ADR-0082）。

決定者の決定（チャット、原文）:

> 全 ADR Accepted

## Decision Drivers（判断基準）

1. main を壊さない。2 つの PR を重ねて初めて壊れるもの（コンパイル・型・テスト・生成物）は、マージの前に統合した状態で必ず止める
2. 1 つの変更を、同じ検査で何度も検査しない
3. 並列の作業のリードタイム（PR を開いてからマージまで。runner の起動待ちを含む）を短くする
4. 同時 20 ジョブとキャッシュの 10 GB を、結果が使われない実行に使わない
5. エージェントの外の PR（OSS の貢献者・Dependabot）にも、マージの前の検査と早い結果を返す
6. 遅い検査（E2E・実 PostgreSQL・連合・arm64）の退行は、リリースの前に必ず見つけ、普段は遅くとも 1 日で見つける

## Considered Options（選択肢）

本検査（テストを含む重い検査）の場所:

- A. 今のまま（ADR-0047）。PR と Merge Queue の両方で、変わった文脈を検査する
- B. Merge Queue に一本化する。PR は、その PR のファイルだけで結果が決まる軽い検査にする
- C. PR に一本化する。Merge Queue はコンパイル・型検査・vet だけにする
- D. Merge Queue をやめる。PR を通ったものはそのままマージし、重ねて壊れたものは main で見つけて直す

遅い検査:

- S1. 今のまま。main への push ごとに回し、新しい push で取り消す
- S2. main への push ごとに回すが、取り消さない（実行中 1 つと待ち 1 つ）
- S3. リリースを作るときの関門にし、nightly と手動でも回す
- S4. nightly だけ

ジョブの分け方:

- J1. 今のまま。文脈ごとに 1〜2 ジョブ（PR で最大約 14 ジョブ）
- J2. ツールチェーンと種類ごとに数ジョブにまとめ、ジョブの中をタスクの並列実行（ADR-0080）で埋める

## Decision（決定）

> 決定者の回答（チャット、原文）: 本検査の場所は「MQ に一本化」、遅い検査は「リリースを作成しようとしたら回す、とかどうですか？任意に実行するのと定時で回すのも用意はしつつ」。
> リリースの関門の範囲（タグと GitHub Release の下書きまでを決め、成果物は M5 に残す）は「そのままで構いません」（#1053）。
> ビルドタグの組み合わせと golangci-lint の置き場所（PR #1069 のあとの Merge Queue の実測を受けて）:
> 「build-tags については毎度のビルドで確認する必要性を感じません。リリース前の検査で十分に思えます。」
> 「golangci-lint については PR, main で `only-new-issues` して MQ ではスキップでもいいぐらいなのでは？(MQ 状態で項目が増えるとも思えない)」
> PR で `only-new-issues`、Merge Queue では回さず、遅い検査で全体を回し、main では検査せずキャッシュのためだけに回す案に対して「ADR-0079 を書き換えていいです。やってください。案にも賛成です。」
> Docker の smoke test について「docker-smoke と docker-amd64 って何を保証してます？MQ での実行や PR での実行必要ですか？ go build との本質的な違いがわかりません」。
> smoke test を Docker の文脈が変わった PR だけにし、Merge Queue から外し、arm64 を遅い検査だけにする案に対して「すでに MQ 入れてマージまでされたので修正は別の PR でお願いします」。

**B・S3・J2 を採る。**

### 守るものと防ぐ場所

| 層 | いつ | 何を検査するか | 必須 |
| --- | --- | --- | --- |
| 手元 | push の前 | `task check:changed`（ADR-0080）。作者が直すためのもので、CI の代わりではない | — |
| PR の `check`（preflight） | `pull_request` | その PR のファイルだけで結果が決まり、重ねても変わらないもの。下の表 | ○ |
| PR の lint | `pull_request` | golangci-lint（PR が変えた行の指摘だけ） | ○ |
| PR の Docker | `pull_request`（Docker の文脈が変わったとき） | Docker Compose の smoke test（amd64） | ○ |
| Merge Queue の `check`（本検査） | `merge_group` | 統合した状態で、変わった文脈のコンパイル・型・vet・テスト・生成物 | ○ |
| main への push | `push` | 検査はしない。キャッシュを作るジョブだけ（下記） | — |
| 遅い検査 | リリースの関門・nightly・手動 | E2E（全体）・実 PostgreSQL・stream の契約・連合・arm64 と amd64 のイメージ・golangci-lint（全体）・ビルドタグの組み合わせ・CodeQL | リリースでは○ |

PR の `check`（preflight）は 1 ジョブにまとめ、目安を 2 分以内にする:

| 守るもの | 検査 | 理由 |
| --- | --- | --- |
| Markdown・シェルの書式 | markdownlint、shellcheck | ファイル単位で決まる |
| 依存のライセンス | go-licenses、npm のライセンス | lockfile で決まる。2 つの PR の依存の和は、それぞれが通った依存の和 |
| Go の書式 | gofmt | ファイル単位で決まる |
| Dockerfile と `.tool-versions` の一致、Playwright の版、デザイントークン、CSS の `100vh` | 今の `docker-check`・`playwright-image-check`・`web-tokens`・CSS の検査 | ファイル単位または lockfile で決まる |
| `@my/ui` のスクリーンショットと axe（ui の文脈が変わったときだけ、別ジョブ） | `ui-test` | 基準画像は変えた PR の中で直す必要がある（ADR-0027） |

golangci-lint は、Go（`tools/agentkit` を含む）が変わった PR で、preflight とは別のジョブ（`lint`）で回す。指摘は golangci-lint-action の `only-new-issues` で PR が変えた行に絞り、PR に注釈として付ける。
解析そのものは全パッケージに対して行うので、速さは main が作る golangci-lint のキャッシュで決まる。
Merge Queue では回さない。2 つの PR を重ねて増える指摘は、別々の PR が同じ関数の最後の呼び出し元を 1 つずつ消すような稀な場合に限られ、ビルドは壊さない。
その指摘も、PR が変えた行の外に出るものも、遅い検査の golangci-lint（全体）が見つける。

Docker Compose の smoke test（amd64）は、Docker の文脈（`Dockerfile`・`.dockerignore`・`deploy/`・`scripts/docker-smoke.sh`）が変わった PR で、別のジョブ（`docker-smoke`）で回す。
守るのは、イメージでしか分からないものである。Dockerfile の各段がビルドできること、`.dockerignore` がイメージに要るファイルを落とさないこと、`deploy/docker/compose.yaml` の配線（環境変数による設定、スキーマの適用と起動の順序、ボリューム）、実行用のイメージ（nonroot・`/data` の権限・CLI）である。
サーバーの組み立てと起動、WebFinger・Actor・NodeInfo、両方言へのスキーマの適用は、Go のテストと `db-check` がすでに確かめる。そのため、`cmd/` や `internal/platform/` を変えただけの PR では回さない。
結果は PR の Docker の構成のファイルでほぼ決まるので、Merge Queue では回さない。イメージに要る新しいファイルを別の PR が足すといった稀な重なりは、リリースの前に遅い検査が見つける。
arm64 のイメージは、Go を `CGO_ENABLED=0` でクロスコンパイルするので、amd64 との違いはベースのイメージとプラットフォームの指定だけである。遅い検査でだけビルドする。

Merge Queue の `check`（本検査）は、変わった文脈（`scripts/changed-contexts.sh`）に応じて次のジョブを組む。
文脈が変わっていないジョブは今までどおりスキップし、`check` はスキップを成功として扱う（ADR-0047 の仕組みのまま）:

| ジョブ | 中身（ジョブの中は並列） | 対象の文脈 |
| --- | --- | --- |
| `server-static` | `go mod tidy -diff`、`go vet`、`CGO_ENABLED=0` のビルド、Go が読む生成物の鮮度（ADR-0081）、`db-check`（両方言） | go・schema・api・i18n |
| `server-test` | `go test ./...`（pgmem、ADR-0041） | go・schema・api・i18n |
| `client-static` | 生成（ADR-0081）→ `tsc`・Oxlint・knip・`vite build`・i18n のカタログの検査 | ui・web・api・i18n |
| `client-test`（2 シャード） | Vitest（`app/web`・`app/ui`・`tools/i18n`）。`app/web` は `vitest --shard=<i>/2` で 2 ジョブに分ける | ui・web・api・i18n |
| `repo` | agentkit の検査と文脈の地図、ADR の番号の重複（ADR-0081） | 常に |

- ジョブの中身は、ADR-0080 のタスク（`task ci:<ジョブ>`）1 つにする。ワークフローは、ツールの準備とそのタスクの呼び出しだけを書く。
  同じタスクを手元でも同じに回せる。
- ビルドタグの組み合わせ（アダプタを外したビルド）は Merge Queue で回さず、遅い検査で回す。組み合わせが壊れるのはアダプタやビルドタグを変えたときに限られ、マージのたびに確かめる必要はない。リリースの前には必ず通る。
- `client-test` を初めから 2 つに分けるのは、今の `web-test` だけで 242 / 274 秒かかり、Merge Queue の最長のジョブになるからである。
  Vitest 5.0.5 の `--shard <index>/<count>` で分ける（`vitest --help`、2026-10-23 確認）。
- Merge Queue で同時に組むグループは最大 2、1 回にマージするのは最大 5 のまま（`.github/rulesets/protect-main.json`）。1 グループのジョブは最大 6 なので、
  Merge Queue が同時 20 ジョブのうち 12 を超えて使うことはない。
- PR の `check` を本検査にしたいとき（外の貢献者の PR を早めに確かめる、Merge Queue で落ちた原因を PR で調べる）は、
  PR に `ci:full` のラベルを付ける。そのときだけ、PR でも Merge Queue と同じジョブを回す。
- 必須チェックは今までどおり `check` の 1 つにし、`pull_request` と `merge_group` で中身を変える。ワークフローにパスのフィルタは付けない（ADR-0047 の理由のまま）。
  1 つの `check` の中身をイベントで変える形は、ADR-0047 の入力 `scope`（shellcheck などを PR だけで回す）ですでに動いている。

### main への push

- 検査は回し直さない。main のコミットは、Merge Queue で検査したマージグループの head そのものである（ADR-0047 のまま）。
- `main.yml` は、PR と Merge Queue が復元するキャッシュだけを作る。キャッシュを作れるのは main だけである。
  実行は、そのブランチと既定のブランチで作られたキャッシュしか復元できない。マージグループは `gh-readonly-queue/main/...` という自分のブランチで動くので、
  そこで保存したキャッシュはほかのグループや PR から使えない（[Dependency caching](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)、2026-10-23 確認）。
  Go のキャッシュは、空の `GOCACHE` から作り直して保存する（前のキャッシュを復元して足さない）。
  これでキャッシュの大きさを 1 回のビルドの分に保つ。キーは `go.sum`・`tools/go.sum`・Go の版・日付にする。保存するのは、そのキーのキャッシュがまだないときだけにする。
  Bun のキャッシュは lockfile をキーにする（今のまま）。
- Go のキャッシュには、復元するジョブが読むものだけを、そのジョブと同じ設定（`CGO_ENABLED=0`。cgo の設定はビルドキャッシュのキーに入る）で、1 つずつ順に作る。
  モジュール、パッケージと Go のツールのビルド、テストのバイナリ、`go vet` の結果、agentkit のビルド、golangci-lint の解析の結果である。
  検査は回さない（golangci-lint は指摘があっても失敗させない）。ライセンスや `go mod tidy` のように何も残さない検査を回したり、検査を並列に回して CPU を取り合わせたりしない。
- CodeQL は main への push では回さず、毎日の schedule・リリースの関門・手動で回す。

### 遅い検査: リリースの関門・nightly・手動

- 遅い検査の組（E2E の全体〈`matrix: full`、8 シャード〉、`federation-live`、`postgres-contract`、`stream-contract`、amd64 と arm64 のイメージと smoke test、golangci-lint の全体とビルドタグの組み合わせ、CodeQL）を、
  1 つの再利用ワークフロー `slow-suite.yml` にまとめる。
- **リリースの関門**: リリースは `release.yml`（`workflow_dispatch`、入力は版）だけが作る。`release.yml` は、指定したコミットで `slow-suite.yml` を回す。
  すべて通ったときだけ、そのコミットにタグを打ち、GitHub Release を下書きで作る。成果物（イメージの公開・リリースノート）は M5（#30）が足す。
- **nightly**: `nightly.yml` は `slow-suite.yml` を毎日 18:00 UTC に回す。ただし、前回成功した nightly から main が動いていなければ回さない。
- **手動**: `slow-suite.yml` と各ワークフロー（`web-e2e.yml` など）は、`workflow_dispatch` で任意のブランチに回せる。
  E2E の部分集合（`matrix: pr`）は手動でだけ回す。
- main への push ごとの E2E はやめる。並行の作業では、E2E の失敗をマイルストーンの終わりにまとめて直す（workflow.md §9）。
  push ごとの結果は使われず、6 割は途中で捨てられていた。

### 見込みの効果（3 日間の実測から見積もる）

| 区分 | 今の runner 時間 | 見込み | 根拠 |
| --- | --- | --- | --- |
| PR | 52.5 h | 約 15 h | preflight は 1 ジョブ約 90 秒、`ui-test` は ui が変わったときだけ |
| Merge Queue | 28.5 h | 約 20 h | 小さいジョブをまとめて準備を減らす。キャッシュの復元が 165 秒から数十秒になる。golangci-lint は PR で回す |
| main への push | 83.6 h | 約 3 h | キャッシュを作るジョブだけ |
| nightly・リリース | 数 h | 数 h | 今の nightly と同じ量 |

- PR の `check` は、中央値約 280 秒から 2 分以内になる。runner の起動待ちも、取り合う相手が減って短くなる。
- マージまでのリードタイムは、PR の preflight と Merge Queue の本検査（目安 4〜5 分）の和になる。今の PR の `check` と Merge Queue の `check` の和（約 9〜10 分）より短い。

### 覆す条件

- 手元の検査を通らない PR が増え、Merge Queue で落ちて外れる PR が週に数件を超えるなら、本検査の一部（コンパイル・型検査）を PR にも戻す。
- リリースの関門で見つかる退行が、直前のマイルストーンの作業に埋もれて直しにくいなら、nightly を必須にするか、E2E の部分集合を Merge Queue に入れる。
- 有料のプランや self-hosted runner に移り、同時実行の上限が問題でなくなったら、PR でも本検査を回すことを考え直す。

## Consequences（影響）

- 良い影響:
  - 本検査は、統合した状態に対して 1 回だけ回る。PR の結果は 2 分以内に返る。
  - main への push（runner 時間の約半分）と CodeQL が、検査のための runner を使わなくなる。PR と Merge Queue のジョブが、起動を待たずに動きやすくなる。
  - リリースは、遅い検査をすべて通ったコミットからしか作れない。
- 悪い影響・トレードオフ:
  - 手元の検査を飛ばした PR は、Merge Queue で初めて落ちる。落ちた PR は Merge Queue から外れ、同じグループの後ろの PR が組み直しになる。
    エージェントは push の前に `task check:changed` を回すこと（今の規則のまま）が前提になる。
  - 外の貢献者の PR は、`ci:full` を付けない限り、Merge Queue まで型やテストの結果が分からない。
  - E2E の退行が main に入ってから見つかるまでが、push ごとの数十分から、nightly の最長 1 日になる。
  - キャッシュを日付で作り直すので、日中に変わったパッケージは Merge Queue で毎回ビルドし直す。
- 後続の作業（issue にする）:
  - `ci.yml` と `ctx-*.yml` を preflight と本検査のジョブに組み替え、`ci:full` のラベルを足す（ADR-0080 のタスクの移行と一緒に行う）
  - `main.yml` をキャッシュを作るジョブだけにし、Go のキャッシュの作り方を変える
  - `slow-suite.yml` と `release.yml` を作り、`nightly.yml` を `slow-suite.yml` の呼び出しにし、main が動いていない日は飛ばす
  - CodeQL の起動を schedule・リリース・手動にする
  - [workflow.md](../agents/workflow.md)・AGENTS.md・CONTRIBUTING.md・[docs/reference/commands.md](../reference/commands.md) の CI の説明を書き直す

## Pros and Cons of the Options（詳細比較・任意）

- A（今のまま）: PR で全部の結果が分かる。その代わり、同じ検査を PR と Merge Queue で 2 回回し、runner の取り合いでどちらも遅くなる。
- C（PR に一本化）: 作者は PR でテストの結果を早く知る。Merge Queue は 1〜2 分で済む。
  その代わり、テストどうしの組み合わせで壊れるもの（同じ表やフィクスチャを 2 つの PR が変えるなど）は main に入る。
  また、PR の本検査は main に入る前に古くなる（後ろのマージで前提が変わる）。
- D（Merge Queue をやめる）: 仕組みが一番単純で、待ちの列もない。その代わり、重ねて壊れた main は、並行で動くエージェント全員の手元の検査を止める。
  実測では、本物の重ね合わせの失敗が 3 日で 1 回あった。
- S2（push ごと・取り消さない）: 退行に早く気づく。その代わり、マージの多い日は実行がつながり、runner を使い続ける。§9 では結果がすぐには使われない。
- S4（nightly だけ）: 一番安い。リリースの直前にもう一度確かめる仕組みがない。
- J1（文脈ごと）: どの文脈が落ちたかがジョブ名で分かる。その代わり、ジョブごとの準備が足し上がり、同時 20 ジョブを取り合う。
  J2 では、どの検査が落ちたかを、タスクのまとまった出力（ADR-0080 の `output: group`）とジョブの要約で示す。

## Open Questions / 要検証

- なし。
