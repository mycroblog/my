# ADR-0079: CI の検査を Merge Queue に一本化する

- Status: Accepted
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1052, #553, #1064, #1638, #1674, #1734, #2453

## Decision（決定）

本検査（テストを含む重い検査）は Merge Queue に一本化し、PR はその PR のファイルだけで結果が決まる軽い検査にする。
遅い検査は nightly と手動で回し、リリースは遅い検査を通ったコミットを昇格させる。ジョブはツールチェーンと種類ごとに数個にまとめ、ジョブの中を並列で埋める。
1 つの保証は、結果を決められるいちばん早くて安い層で 1 回だけ確かめる（§18）。

### 1. 守るものと防ぐ場所

Merge Queue の全検査が、`main` に入る前の唯一の関門である。

| 層 | いつ | 何を検査するか | 必須 |
| --- | --- | --- | --- |
| 手元 | 任意 | `task check:changed`（ADR-0081）。CI の失敗の再現と書いている途中の確認のためのもの（§10） | — |
| PR の `check`（preflight） | `pull_request` | その PR のファイルだけで結果が決まり、重ねても変わらないもの（§2） | ○ |
| PR の lint | `pull_request` | golangci-lint（PR が変えたモジュールの全体。§3）、スキーマを変えた PR の `db-lint`（ADR-0023） | ○ |
| PR の Docker | `pull_request`（Docker の文脈が変わったとき） | Docker Compose の smoke test（amd64。§4） | ○ |
| Merge Queue の `check`（本検査） | `merge_group` | 統合した状態で、変わった文脈のコンパイル・型・vet・テスト・生成物。client が変わったときは E2E の流れの spec、アダプタが変わったときはそのアダプタのコンテナの契約（§5） | ○ |
| main への push | `push` | 関門にはしない。キャッシュを作るジョブだけ。Go のキャッシュはテストの結果も含めて作る（§7） | — |
| 遅い検査 | nightly・手動 | E2E のロケールと幅の spec・コンテナの契約（全部）・連合・arm64 と amd64 のイメージ・golangci-lint（全体）・ビルドタグの組み合わせ・CodeQL（§8） | 落ちたら止める |
| リリースの関門 | `release.yml` | リリースするコミットで成功した遅い検査があること、なければ遅い検査。upgrade・restore-drill・イメージは毎回（§8） | ○ |

Merge Queue に入るのは PR の `check` を通ったものだけなので、PR だけで回す検査も main に入る前に一度は通っている。

### 2. PR の `check`（preflight）

1 ジョブにまとめ、目安を 2 分以内にする。

| 守るもの | 検査 | 理由 |
| --- | --- | --- |
| Markdown・シェルの書式 | markdownlint、shellcheck | ファイル単位で決まる |
| 依存のライセンス | go-licenses、npm のライセンス | lockfile で決まる。2 つの PR の依存の和は、それぞれが通った依存の和 |
| Go の書式 | gofmt | ファイル単位で決まる |
| TypeScript の書式 | `web:fmt-check`（ADR-0081）。Merge Queue の `repo` でも回す | — |
| Dockerfile と `.tool-versions` の一致、Playwright の版、デザイントークン、CSS の `100vh` | `docker:dockerfile-check`・`web:playwright-image-check`・`web:tokens`・CSS の検査 | ファイル単位または lockfile で決まる |
| `@my/ui` のスクリーンショットと axe（ui の文脈が変わったときだけ、別ジョブ） | `ui-test` | 基準画像は変えた PR の中で直す必要がある（ADR-0110） |
| `app/web` の画面のストーリー（クライアントが変わったときだけ、別ジョブ） | `web:story-test` | §13 |
| メールの型、シェルのテスト、PR の Documentation の節 | `mail:typecheck`・`shell:test`・`docs:pr-impact` | ファイル単位か PR の説明で決まる |

### 3. PR の golangci-lint

- Go（`tools/agentkit`・`pkg/mykit`・`pkg/mybot` を含む）が変わった PR で、preflight とは別のジョブ（`lint`）で回す。変わったモジュールの全体を、領域のタスク（`scripts/task.sh go:lint` など）で検査する。golangci-lint-action はインストールだけに使う（`install-only`）。並列に動く action のインスタンスがイベントの本文を読み書きして互いに壊し、同じパスへバイナリを落とすため。
- 解析そのものは全パッケージに対して行うので、速さは main が作る golangci-lint のキャッシュで決まる。
- Merge Queue では回さない。2 つの PR を重ねて増える指摘は、別々の PR が同じ関数の最後の呼び出し元を 1 つずつ消すような稀な場合に限られ、ビルドは壊さない。
  その指摘も、遅い検査の golangci-lint（全体）が見つける。

### 4. PR の Docker smoke test

- Docker Compose の smoke test（amd64）は、Docker の文脈（`Dockerfile`・`.dockerignore`・`deploy/`・`scripts/docker-smoke.sh`）が変わった PR で、別のジョブ（`docker-smoke`）で回す。同じジョブで警報の規則の検査（`docker:alert-rules`）も回す。
- 守るのは、イメージでしか分からないものである。Dockerfile の各段がビルドできること、`.dockerignore` がイメージに要るファイルを落とさないこと、`deploy/docker/compose.yaml` の配線（環境変数による設定、スキーマの適用と起動の順序、ボリューム）、実行用のイメージ（nonroot・`/data` の権限・CLI）である。
- サーバーの組み立てと起動、WebFinger・Actor・NodeInfo、両方言へのスキーマの適用は、Go のテストと `db-check` がすでに確かめる。そのため、`cmd/` や `internal/platform/` を変えただけの PR では回さない。
- 結果は PR の Docker の構成のファイルでほぼ決まるので、Merge Queue では回さない。イメージに要る新しいファイルを別の PR が足すといった稀な重なりは、リリースの前に遅い検査が見つける。
- arm64 のイメージは、Go を `CGO_ENABLED=0` でクロスコンパイルするので、amd64 との違いはベースのイメージとプラットフォームの指定だけである。遅い検査でだけビルドする。

### 5. Merge Queue の `check`（本検査）

変わった文脈（`scripts/changed-contexts.sh`）に応じて次のジョブを組む。判定の漏れは「知らないパスはすべての文脈」で防ぐ。

| ジョブ | 中身（ジョブの中は並列） | 対象の文脈 |
| --- | --- | --- |
| `server-static` | `go mod tidy -diff`、`go vet`、`CGO_ENABLED=0` のビルド、Go が読む生成物の鮮度（ADR-0081）、`db-check`（両方言） | go・schema・api・i18n |
| `server-test` | `go test ./...`（pgmem、ADR-0015） | go・schema・api・i18n |
| `client-static` | 生成（ADR-0081）→ `tsc`・Oxlint・knip・`vite build`・i18n のカタログの検査 | ui・web・api・i18n |
| `client-test`（2 シャード） | Vitest（`app/web`・`app/ui`・`tools/i18n`）。`app/web` は `vitest --shard=<i>/2` で 2 ジョブに分ける | ui・web・api・i18n |
| `repo` | agentkit の検査と文脈の地図、ADR の番号の重複（ADR-0081）、手元と CI の組み合わせのずれ（§11）。site の文脈が変わったときは、ドキュメントサイトのビルド（ADR-0150 §7）と `app/docs` の `tsc`・Oxlint・knip。`app/docs` は site だけを選び、client の文脈を選ばない | 常に（サイトの検査は site） |

- client の文脈（ui・web・api・i18n）が変わったグループでは、E2E の操作の流れの spec を 1 プロジェクト（ADR-0015 §1）で回す。ジョブ数が同時 20 ジョブの上限に収まるように、シャードの数を決める。
- アダプタの文脈が変わったグループでは、そのアダプタのコンテナの契約（`postgres-contract`・`stream-contract`・`blob-contract`・`voice-contract`・`search-contract`・`mail-integration`）を回す。1 つのジョブの parallel のステップにまとめ、変わっていないアダプタの契約は回さない。
- ビルドタグの組み合わせ（アダプタを外したビルド）は Merge Queue で回さず、遅い検査で回す。組み合わせが壊れるのはアダプタやビルドタグを変えたときに限られ、マージのたびに確かめる必要はない。リリースの前には必ず通る。
- `client-test` を初めから 2 つに分けるのは、`web-test` だけで 242 / 274 秒（中央値 / p90）かかり、Merge Queue の最長のジョブになるからである。
  Vitest 5.0.5 の `--shard <index>/<count>` で分ける（`vitest --help`、2026-10-23 確認）。
- Merge Queue で同時に組むグループは最大 2（`max_entries_to_build: 2`）、1 回にマージするのは最大 5（`max_entries_to_merge: 5`）にする（`.github/rulesets/protect-main.json`）。グループが増えると同時 20 ジョブを使い切り、PR の待ちが延びるからである。
  1 グループのジョブは最大 7（コンテナの契約のジョブを含む）なので、Merge Queue が同時 20 ジョブのうち 14 を超えて使うことはない。
- PR の `check` を本検査にしたいとき（外の貢献者の PR を早めに確かめる、Merge Queue で落ちた原因を PR で調べる）は、PR に `ci:full` のラベルを付ける。そのときだけ、PR でも Merge Queue と同じジョブを回す。

### 6. 必須チェックとスキップ

- 必須チェックは `ci.yml` の `check` の 1 つにし、`pull_request` と `merge_group` で中身を変える。
- PR の実行は、同じ PR に新しい push が来たら取り消す。
- `ci.yml` は `pull_request` と `merge_group` で必ず起動し、ワークフローにパスのフィルタを付けない。文脈を絞るのは、ジョブの `if`（`changes` の出力）だけにする。
  `if` でスキップしたジョブは "Success" を報告し、`needs` の結果は `skipped` になるので、`check`（`if: always()`）はスキップを成功として扱う。
- この形を採る根拠（GitHub のドキュメント、2026-10-19 確認）:
  - パス・ブランチ・コミットメッセージでスキップされたワークフローのチェックは "Pending" のままで、必須ならマージを止める（[Troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks)）。
  - Merge Queue が必須にする Actions のチェックは `merge_group` で起動しなければならず、起動しないと報告されずにマージが失敗する（[Managing a merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue)）。
  - `merge_group` のフィルタとして書かれているのは `branches` / `branches-ignore` だけである（[Workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)）。Merge Queue で変更範囲によって検査を絞るには、ワークフローの中で差分を判定するしかない。

### 7. main への push

- 関門にはしない。main のコミットは、Merge Queue で検査したマージグループの head そのものである。
- `main.yml` は、PR と Merge Queue が復元するキャッシュだけを作る。キャッシュを作れるのは main だけである。
  実行は、そのブランチと既定のブランチで作られたキャッシュしか復元できない。マージグループは `gh-readonly-queue/main/...` という自分のブランチで動くので、
  そこで保存したキャッシュはほかのグループや PR から使えない（[Dependency caching](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)、2026-10-23 確認）。
- Go のキャッシュは、テストの結果も含めて作る。マージのたびに `go test ./...` を回し、Go のテストの結果のキャッシュによって、Merge Queue と PR では変更の影響を受けたパッケージのテストだけが走るようにする。
  キーはコミットにし、前のコミットのキャッシュを復元して足していく。大きさが上限（リポジトリで 10 GB）に近づく前に、定期的に空から作り直す。
  `go test` が落ちてもマージは止まらない（関門は Merge Queue である）。落ちたことは nightly と同じ形で知らせる（§8）。
  キャッシュが効くのは、パッケージの境界が変更の影響範囲に沿っているときに限る。テストの責務を分ける（§18）までは、今の形（空の `GOCACHE` から日付をキーに作るビルドの結果）のままにする。
  Bun のキャッシュは lockfile をキーにする。
- Go のキャッシュには、復元するジョブが読むものだけを、そのジョブと同じ設定（`CGO_ENABLED=0`。cgo の設定はビルドキャッシュのキーに入る）で、1 つずつ順に作る。
  モジュール、パッケージと Go のツールのビルド、テストのバイナリ（テストの責務を分けた後はテストの結果も）、`go vet` の結果、agentkit のビルド、golangci-lint の解析の結果である。
  golangci-lint は指摘があっても失敗させない。ライセンスや `go mod tidy` のように何も残さない検査を回したり、検査を並列に回して CPU を取り合わせたりしない。
- CodeQL は main への push では回さず、nightly（`slow-suite.yml` の経由で毎日）・リリースの関門・手動で回す。

### 8. 遅い検査: nightly・手動・リリースの関門

- 遅い検査の組（E2E のロケールと幅の spec、`federation-live`、`postgres-contract`、`stream-contract`、`mail-integration`、`voice-contract`、`blob-contract`、`search-contract`、amd64 と arm64 のイメージと smoke test、`upgrade`、`restore-drill`、golangci-lint の全体とビルドタグの組み合わせ、CodeQL）を、1 つの再利用ワークフロー `slow-suite.yml` にまとめる。コンテナの契約は Merge Queue でも、そのアダプタが変わったときに回る（§5）。nightly ではすべてを回し、アダプタを変えずに依存の更新で壊れた分を拾う。
- **nightly**: `nightly.yml` は `slow-suite.yml` を毎日 18:00 UTC に回す。ただし、前回成功した nightly から main が動いていなければ回さない。GitHub の混雑で 2 時間ほど遅れて起動することがあるが、退行を 1 日以内に見つけるには足りる。
- **赤は止める合図**: nightly が落ちたら、落ちたジョブと前回緑のコミットからの差分を書いた issue を自動で起こす。同じ失敗が続く間は同じ issue に追記し、緑に戻ったら閉じる。main が落ちたままの状態を許す期間は設けない。直すまでは、そのマイルストーンのほかの作業より先にする（workflow.md の Failures on main）。retry で通ったテストは flaky の候補として集計して知らせる。
- **リリースの関門**: リリースは `release.yml`（`workflow_dispatch`、入力は版）だけが作る。`release.yml` は、リリースするコミットで成功した遅い検査の組（nightly のもの、または手で回した `slow-suite.yml`）があれば、それを関門の結果として使う。なければそのコミットで `slow-suite.yml` を回す。部分的な実行や別のコミットの結果は数えない。
  `upgrade` と `restore-drill` は、そのコミットのイメージで確かめるので、リリースでは毎回回す。すべて通ったときだけ、そのコミットにタグを打ち、GitHub Release を下書きで作る。
- **手動**: `slow-suite.yml` と各ワークフロー（`web-e2e.yml` など）は、`workflow_dispatch` で任意のブランチに回せる。
- **E2E の頻度**: 操作の流れの spec は、client が変わったマージグループで回す（§5）。ロケールと幅の spec は nightly で回す。PR では自動では回さない（パスやラベルによる起動も持たない）。E2E に触れる PR では、作者が `scripts/task.sh web:e2e` か `web-e2e.yml` を手で回せる。

### 9. E2E の並列化

`web-e2e.yml` は、ビルド・シャード・レポートの 3 段にする。

```text
build（ubuntu-latest、1 ジョブ）: app/web のビルドと bin/my のビルド → actions/upload-artifact（app/web/dist と bin/my）
shard 1..N（Playwright のイメージ）: bun install → download-artifact → playwright test --shard=i/N --reporter=blob
report（1 ジョブ、if: always()）: blob を集めて playwright merge-reports で 1 つの HTML にする
```

- シャードは `fullyParallel: true` のテスト単位で分かれる。シャードごとに自分の `vite preview` と `my serve`（SQLite、毎回新しい DB）を起動するので、シードとレート制限はシャードの中で閉じる。
- 成果物を配る主な理由は、時間よりも全シャードが同じビルドを検査することである（ビルドそのものは約 16 秒）。シャードは Go のセットアップを省ける。
- シャードの数は `matrix: pr` が 4、`matrix: full` が 8。
- シャードの worker 数は 2。実測（#562）では 3 にしても、`matrix: pr` の範囲が約 10.0 分から約 8.8 分、`matrix: full` が約 10.6 分から約 10.2 分になるだけで、縮む時間が小さい。

### 10. 手元の実行と Merge Queue から外れた PR

- エージェントも人間も、`check` / `check:changed` をコミット・push・PR の前に走らせる義務を負わない。手元の実行は、CI の失敗の再現と、書いている途中の確認のためのものである。
- PR の `check`（preflight）の範囲（§2）と `ci:full` ラベルの使い方（§5）は、手元の検査の有無に関わらず同じである。
- Merge Queue から外れた PR は、持ち主が直して入れ直す。その間も列の残りは進む。
- 並行する PR の順序や衝突は、計画の問題（workflow.md §9）として扱う。手元で検査を走らせる理由にしない。
- フォーマット（`web:fmt`、`gofmt`）は、preflight が見るものなので、push の前にかける。安く、preflight の失敗を避けられる。
- `agentkit check` は任意のコマンドとして残す。`agentkit pr open` は、手元の検査を通したことを PR の項目にしない。

### 11. CI のジョブの組み方

- go-task は、各検査のコマンドの唯一の定義と、手元の入口（`check`・`check:changed`・領域ごとの `*:check`・生成・開発の起動）に徹する。
- CI のジョブは、ワークフローで組む。準備のステップ（checkout・`setup-go`・`setup-bun`・`setup-task`・生成）の後に、互いに依存しない検査を `parallel` のステップにする。
  各ステップは名前を検査の名前にして、末端のタスクを 1 つ呼ぶ（`scripts/task.sh go:vet`）。
- golangci-lint も、ほかの検査と同じくタスクをステップから呼ぶ。golangci-lint-action はインストールにだけ 1 回使い（§3）、版は `.tool-versions` から読んでタスクと同じ版にする。
- `ci:*` の集約タスクは持たない。CI 向けの `output: group` と `error_only` の設定も持たない（ステップごとにログが分かれる）。
- go-task の `deps` が保証する順序は、ワークフローでも保つ。生成は並列のまとまりの前のステップにする。互いに干渉する検査は、同じまとまりに入れないか、`deps` で順序を持つ 1 つのタスクとして呼ぶ。
  干渉する検査の例: `web:lint-rules` は `app/web/src` に一時的なファイルを置く。golangci-lint は同時に 1 つしか動かない。
- 手元と CI の組み合わせのずれは、`repo` ジョブで検査する。`check` が含む末端のタスクがすべて、CI のどこかのステップで呼ばれているか、CI で回さない理由とともに除外の一覧にあるかを確かめる。
- GitHub Actions は 1 つのジョブの中でステップを並列に動かせる（`parallel`・`background`・`wait`・`wait-all`・`cancel`）。並列のステップはそれぞれのログと実行を持ち、1 つのジョブで同時に動く background のステップは 10 まで、composite action の中では使えない
  （[changelog](https://github.blog/changelog/2026-06-25-actions-steps-can-now-be-run-in-parallel)、[Workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)、2026-10-23 確認）。

### 12. 覆す条件

- Merge Queue で落ちて外れる PR が週に数件を超えるなら、本検査の一部（コンパイル・型検査）を PR にも戻す。`ci:full` の使い方と preflight の範囲も見直す。
- Merge Queue の E2E の流れの spec が律速になり、グループの wall の中央値が 10 分を超えるなら、シャードを増やすか、流れの spec を見直す。
- nightly の自動の issue が、同じ flaky のテストで繰り返し開くなら、retry で通ったテストを自動で隔離する仕組みを考え直す。
- 有料のプラン、大きな runner や self-hosted runner に移り、同時実行の上限が問題でなくなったら、PR でも本検査を回すことや、E2E の部分集合を Merge Queue に入れることを考え直す。
- 重ねて初めて壊れる種類の失敗が、Merge Queue で回さない検査（`ui-test`・`docker-smoke` など）で main に繰り返し入るなら、その検査を Merge Queue に戻す。
- `parallel` のステップが GitHub-hosted runner で期待どおりに動かない（ログが分かれない、失敗が拾えない）なら、ジョブを検査ごとに分けるか、ジョブの中身を集約タスクの呼び出しに戻す。
- 組み合わせのずれの検査（§11）が保守の負担になるなら、検査をやめ、ずれはレビューで見る。

### 13. UI のプレビュー: 置くもの

PR ごとの UI のプレビューとして、部品と画面の Storybook を Cloudflare Workers の Previews に置く。

- `app/web` に Storybook を足し、画面のストーリーを置く。ストーリーはアプリ全体をルートで描き（`app/web/src/test/stories.tsx`。結合テストと同じ組み立て）、API は機能のテストの MSW のハンドラ（`features/<x>/test/handlers.ts`）とフィクスチャで差し替える。
  ブラウザの MSW には `msw/browser` を使う（MSW は既存の依存）。
- 画面のストーリーは、ADR-0110 の L1 の作業ごとに、通常・空・読み込み中・エラー・長い文言（`ja`・`en-XA`）・RTL（`ar-XB`）・320 px の状態を持つ。操作は `play` 関数で書き、Storybook の Interactions のパネルで動きを再生できるようにする。
- `app/web` の Storybook は、`app/ui` の Storybook を Storybook の composition（`refs`）で取り込み、1 つの URL で部品と画面の両方を見られるようにする。
- 画面のストーリーは、品質ゲート（ADR-0110）の axe の検査に入れる。スクリーンショットの比較には入れない（画面は E2E のスクリーンショットが受け持つ）。

### 14. UI のプレビュー: 置き場

- Cloudflare の、静的アセットだけを返す Worker を 1 つ作る。サーバーの実行環境ではない（my のサーバーは Cloudflare Workers を対象にしない方針のまま）。
- **PR ごと**: Workers の Previews（`wrangler preview --name pr-<番号>`）に置く。URL は `pr-<番号>-<Worker 名>.<サブドメイン>.workers.dev` で、`workers.dev` は `X-Robots-Tag: noindex` を返す。
  閉じた PR の Preview は消す（§15）。無料のプランで Worker あたり 100 の Preview を持て、上限では最も古いものから消える（2026-10-28 に [Previews の文書](https://developers.cloudflare.com/workers/previews/)で確認）。
- **main**: main への push で本番の Worker に置く。これが ADR-0110 の L5 で決定者が見る、全画面と部品の一覧になる。

### 15. UI のプレビュー: CI の構成

- **ビルド**（`ui-preview.yml`、`pull_request` と main への `push`。`app/**`・`locales/**` と、ビルドに使う `tasks/ui.yml`・`tasks/web.yml`・`scripts/ui-preview*` が変わったとき）: 2 つの Storybook を静的にビルドし、成果物として上げる。秘密情報を持たない。PR の必須の検査にもマージの条件にもしない。
- **公開**（`ui-preview-deploy.yml`、ビルドの `workflow_run`）: main の workflow として動き、Cloudflare の API トークン（秘密情報）を持つ。成果物を静的なファイルとしてだけ扱い、中身を実行しない。
  Worker の設定（`wrangler` の設定）は main の側のものを使い、成果物から Worker のコードを読まない。
- **PR へのコメント**: 公開した URL と、main の Storybook と比べて追加・変更されたストーリー（ビルドの `index.json` の差分）への直接のリンクを、PR の 1 つのコメントにまとめて更新する。
  決定者は、このリストを見る順として使い、コメントは GitHub の PR に書く（L5 の分類は ADR-0110）。
- **閉じた PR**: `ui-preview-cleanup.yml` が 24 時間に 1 回の schedule と手動（`workflow_dispatch`）で、閉じた PR とマージした PR の Preview を消す。Worker の Preview の一覧を Cloudflare の API で取り、各 PR の状態を GitHub に尋ねる。
  main の workflow とスクリプトだけを `main` に限った environment で動かし、PR のものをチェックアウトもダウンロードもしない。

### 16. UI のプレビュー: 確かめることと見直す条件

- Previews が静的アセットだけの Worker で動くことは、文書に明記がない。実装の issue の最初に確かめ、動かなければ、同じ Worker のバージョンの URL（`wrangler versions upload --preview-alias pr-<番号>`）に置く。
  どちらでも URL・コメントの流れは変わらない。バージョンの URL は消せないので掃除はせず、同じ別名の次のアップロードが置き換える。
- Cloudflare の無料のプランで、Previews もバージョンの URL も静的アセットに使えない場合は、GitHub Pages にして、PR ごとの URL を諦め、main の一覧だけを置く。
- 画面のストーリーの手入れ（MSW のハンドラとルートの差し替え）が、画面の追加のたびに大きな手間になる場合は、`app/web` 全体をブラウザの MSW で動くデモとしてビルドする形に切り替える。

### 17. CI から Claude API への認証

CI から Claude API を使うジョブはすべて Workload Identity Federation（WIF）で認証し、`ANTHROPIC_API_KEY` を GitHub に置かない。

- **ジョブの範囲**: Claude API を使うジョブは、それぞれ専用の environment で動き、`permissions: id-token: write` を持つ。どの environment も、デプロイできるブランチは `main` だけである。
  - `nightly.yml` の `l4`: environment `ux-nightly`。
  - `release.yml` の `release-notes-prose`: environment `release-notes`。ブランチでの試しのリリースでは動かず、draft は事実だけになる。
- **Anthropic の側**（決定者が Claude Console の Workload identity で設定する）:
  - 発行者は `https://token.actions.githubusercontent.com` を discovery の方式で登録する。
  - CI 専用のワークスペースとサービスアカウントを作り、ワークスペースに使用額の上限を設ける。上限が、CI 全体の Claude API の費用の上限になる。2 つのジョブは同じワークスペースとサービスアカウントを使う。
  - フェデレーションの規則は environment ごとに 1 つ作る。`subject_prefix` を `repo:mycroblog@346242925/my@1421587381:environment:<environment>`（GitHub がこのリポジトリに発行する `sub` は、所有者とリポジトリの数値 ID を含む形である）、`audience` を `https://api.anthropic.com` にし、
    `claims` で `repository_owner: mycroblog` と `ref: refs/heads/main` も確かめる。トークンの有効期間は短く（例 600 秒）する。
- **GitHub の側**: 規則・組織・サービスアカウント・ワークスペースの ID は秘密ではないので、各 environment の variable に置く
  （`ANTHROPIC_FEDERATION_RULE_ID`・`ANTHROPIC_ORGANIZATION_ID`・`ANTHROPIC_SERVICE_ACCOUNT_ID`・`ANTHROPIC_WORKSPACE_ID`）。
- **トークンの受け渡し**: ジョブは GitHub の OIDC のトークンを `ANTHROPIC_IDENTITY_TOKEN_FILE` のファイルに書き、Claude Code が交換する。GitHub のトークンは約 5 分で切れるので、ジョブはファイルを 2 分ごとに取り直す。
- **信頼しない入力を読むジョブ**: `release-notes-prose` は PR と Issue の本文を読むので、モデルの入力を信頼しない。書き込みの権限を持たず、モデルに道具を与えず、環境変数のうち `ANTHROPIC_` で始まるものだけを Claude Code に渡す。出力は機械が検査してから別のジョブが draft に入れ、決定者が読んで公開する。
- **見直す条件**: 使うエージェントの道具（Claude Code）が WIF の交換に対応せず、包む手間が大きい場合は、交換したアクセストークンを、そのジョブの中でだけ環境変数として渡す。静的なキーには戻さない。

WIF は GitHub の OIDC のトークンを短い有効期間の Anthropic のアクセストークンに交換する仕組みである（2026-10-28 に [Use WIF with GitHub Actions](https://platform.claude.com/docs/en/manage-claude/wif-providers/github-actions) で確認）。

### 18. 検査の配置の原則

- 1 つの保証は、結果を決められるいちばん早くて安い層で、1 回だけ確かめる。別の層でもう一度回すのは、その層でしか分からないもの（統合した状態、実際のインフラ、リリースの成果物）に限る。
- テストの範囲は、変更の影響範囲で決める。Go では、パッケージの依存とテストの結果のキャッシュに任せる。そのため、パッケージの境界をテストの責務の境界にする: モジュール（`internal/app/<module>`）の変更で走るテストは、そのモジュールとその利用者のものに限る。
  集約点（sqlc の生成物、`schema` の埋め込み、DB アダプタ、`pkg/testkit`、`internal/platform` の結合テスト）をどう分けるかは、#2454 のスパイクで決め、その結果を ADR にする。
- 組み立ての層（`cmd/my`・`internal/platform`）のテストは、ほぼ全体に依存するので毎回走ってよい。その代わり、合わせて Merge Queue の最長のジョブに収まる速さに保つ。
- 遅い層の赤は止める合図にする（§8）。
- リリースは、検査済みのコミットを昇格させる（§8）。

## 経緯

関係する要件は NFR-DEV-1（1 つのコマンドで検査が走る）と NFR-DEV-2（外部サービスなしで検査が完結する）。

v0 の開発が進むと、`web-e2e`（約 15 分）が PR の待ち時間を決めるようになり、E2E を main への push でだけ回し、新しい push で取り消す形にした（2026-10-19）。
続けて、検査を PR・Merge Queue・main・nightly に振り分け、文脈ごとの再利用ワークフロー（`ctx-*.yml`）と `changes` ジョブで組み、E2E をシャードに分けた。
この時点の実測で、main への push の 63% はジョブが 1 つも動かずに取り消され、残りも Merge Queue と同じ木を 2 度検査していた。

4〜5 本ずつ並行して PR を出す運用（workflow.md §9）では、それでも同じ検査の繰り返しと runner の取り合いがリードタイムを決めた。
3 日間の実測（2026-10-20〜23、ジョブ約 5,800 件）で、runner 時間は PR 52.5 h、Merge Queue 28.5 h、main への push 83.6 h で、main の push の 60% が次の push で取り消されていた。
PR と Merge Queue の失敗はどちらも 1% 前後で、Merge Queue が止めた 2 回のうち 1 回は重ねて初めて壊れる型の不一致だった（`tsc`、run 39385861802）。
Merge Queue の価値は、統合した状態でコンパイル・型検査・テストを通すことにある。そこで本検査を Merge Queue に寄せ、main への push をキャッシュの作成だけにし、遅い検査をリリースの関門にした。
Free プランの GitHub-hosted runner は同時 20 ジョブ、キャッシュはリポジトリあたり 10 GB である（[Actions limits](https://docs.github.com/en/actions/reference/limits)、2026-10-23 確認）。

その後の Merge Queue の実測を受けて、ビルドタグの組み合わせを遅い検査へ移し、golangci-lint を PR の変えた行に絞り、smoke test を Docker の文脈が変わった PR だけにした。golangci-lint はその後、action を並列に動かすと壊れるので、領域のタスクで変えたモジュールの全体を見る形に変えた（§3）。

本検査を PR の上で初めて回すと（#1069、`ci:full`）`server-static` が 754 秒かかった。ジョブが `task ci:<job>` 1 つの形では、Actions の画面からどの検査が遅いのかが分からなかったので、ジョブをワークフローの `parallel` のステップで組み直した（§11）。

Merge Queue の前の関門は当初、手元の `check:changed` の役だった。ところが 1 台のマシンで 5 エージェントを並行させた日に、手元の検査が負荷でタイムアウトし（Go の sqlite アダプタのテストが 600 秒に達した）、その日 Merge Queue が止めた 2 件（#1704 は PR をまたいだ相互作用、#1678 は proxy.golang.org のネットワークエラー）は手元の検査では見つからなかった。
そこで検査の義務を、結果を決める場所である Merge Queue に一本化した（§1、§10）。

UI のプレビュー（§13〜§16）は、ADR-0110 の批評のサイクル（L5）で決定者が PR の画面と動きを見るために足した。品質ゲート（ADR-0110）のスクリーンショットは `prefers-reduced-motion: reduce` で撮るのでマイクロインタラクションが写らず、`app/web` にはストーリーがなかった。PR の preflight に足す余地はなく、フォークの PR の workflow は秘密情報を読めない。

Claude API の認証（§17）は、ADR-0015 が API キーを environment に置くとしていたものを、決定者の求めで WIF に変えた。

その後、#2119（`cmd/my` のテストが 600 秒の上限に迫る）を発端に、CI の全体を計測した（#2463、2026-11-07）。リリースは、手順で緑を確かめた nightly と同じ遅い検査を回し直していた（runner 約 245 分）。full nightly は 14 回中 7 回落ちていて、v0 の間は E2E の赤を許していたので、リリースの直前にまとめて直す形になっていた。これは §12 の覆す条件の 1 つ（リリースの関門で見つかる退行が直前の作業に埋もれて直しにくい）にあたる。
E2E は 364 テストを 20 プロジェクトで回し、遅い検査の runner 時間の 70% を使っていたが、ロケールと幅の見た目は画面のストーリー（§13）がすでに PR で見ていた。Merge Queue の `go test ./...` では、テストの結果のキャッシュが 1 つも効いていなかった。それでも、キャッシュだけを入れても、Go に触れた PR の 98% で重いテストが走る。モジュールの依存に循環はないが、集約点が全モジュールを 1 つにまとめるからである。
そこで、検査の配置の原則（§18）を決め、リリースを昇格の形にし、nightly の赤を止める合図にし、E2E の流れの spec とアダプタのコンテナの契約を Merge Queue に入れ、main でテストの結果のキャッシュを作ることにした（キャッシュはテストの責務を分けた後で効く）。

## 採らなかった案

- 今のまま PR と Merge Queue の両方で本検査を回す: PR で全部の結果が分かるが、同じ検査を 2 回回し、runner の取り合いでどちらも遅くなる。
- PR に一本化し、Merge Queue はコンパイル・型検査・vet だけにする: 作者は早く結果を知れるが、テストどうしの組み合わせで壊れるものが main に入る。PR の本検査は main に入る前に古くなる。
- Merge Queue をやめる: 一番単純で待ちの列もないが、重ねて壊れた main は並行で動くエージェント全員を止める。実測では本物の重ね合わせの失敗が 3 日で 1 回あった。
- 遅い検査を main への push ごとに、取り消さず回す: 退行に早く気づくが、マージの多い日は runner を使い続け、workflow.md §9 の並行の作業では結果がすぐには使われない。
- 遅い検査を nightly だけにする: 一番安いが、リリースの直前に確かめる仕組みがない。
- リリースのたびに遅い検査の組を回し直す: 直前の nightly とリリースの間に入ったコミットも必ず確かめられるが、同じコミットでは同じ結果を得るために runner 約 245 分と 30 分を使う。昇格の形でも、緑の結果がないコミットでは回すので、保証は変わらない。
- v0 の間は main が E2E で落ちることを許す: 並行の作業を止めないが、赤が常態になり、退行がリリースの直前にまとまる。
- テストの結果のキャッシュだけを入れ、パッケージの構成は変えない: 一番安いが、集約点のために、ほぼすべての PR で重いテストが走り、時間が縮まない。
- E2E を幅 × ロケールのマトリクスのまま、シャードを増やす: 組み合わせの漏れがないが、同じ不具合が 20 回落ち、見た目はストーリーと重なる。
- 文脈ごとの 1〜2 ジョブ（PR で最大約 14 ジョブ）のまま: 落ちた文脈がジョブ名で分かるが、ジョブごとの準備が足し上がり、同時 20 ジョブを取り合う。
- 文脈ごとの独立したワークフローに `paths` を付ける: 文脈ごとに読め回せるが、スキップで必須チェックが Pending のまま残り、`merge_group` にはパスのフィルタがない。
- E2E を 1 ジョブのまま回す: runner の消費は最も少ないが、全体で 45〜49 分かかる。
- 手元の `check:changed` を必須のまま残す: 自分の PR で閉じた失敗は早く分かるが、並行のマシンを食い合い、Merge Queue が見る統合の失敗は防げない。
- `ci:full` を既定にして PR でも全検査を回す: 結果は早いが、同じ検査を繰り返す。
- CI のジョブを `task ci:<job>` 1 つにする: 手元と CI が同じ組を回せるが、検査ごとの時間とログが見えない。
- ワークフローにタスクを通さずコマンドを直接書く: 検査のコマンドが手元と 2 か所に分かれ、中身がずれる。
- プレビューを `app/ui` の Storybook だけにする: いちばん安いが、問題の中心である画面の情報設計を批評できない。
- プレビューを `app/web` 全体のデモにする: 導線を確かめられるが、特定の状態を作る仕組みが要り、状態を列挙する Storybook より批評の入口として弱い。
- プレビューを Chromatic に置く: 公開・見た目の比較・承認がそろうが、無料は月 5,000 スナップショット（2026-10-28 に[料金の文書](https://chromatic.com/pricing)で確認）ですぐ超え、比較は既存のスクリーンショットと重なる。
- プレビューを GitHub Pages に置く: 外部の依存が増えないが、1 つのリポジトリに 1 つのサイトで、PR ごとの URL を持てない。
- Claude API に静的な API キーを使う: 設定が secret 1 つで済むが、キーは漏れると取り消すまで使え、`main` に限る仕組みも environment の設定にだけ頼る。

## Consequences（影響）

- 良い影響:
  - 本検査は、統合した状態に対して 1 回だけ回る。PR の結果は 2 分以内に返る。並行のマシンが検査で詰まらない。
  - CodeQL は main への push で runner を使わない。main への push は、キャッシュを作るためだけに runner を使う。
  - リリースは、同じコミットで緑の遅い検査があれば、それを回し直さない。
  - nightly の赤が自動で issue になり、放置されない。
  - リリースは、遅い検査をすべて通ったコミットからしか作れない。
  - Actions の画面で、検査ごとの所要時間とログが見え、失敗したステップの名前が落ちた検査の名前になる。
  - 決定者が PR ごとに、画面の状態と動きをブラウザで見てコメントでき、変わったストーリーの一覧がレビューの順になる。画面のストーリーは ADR-0110 の L3 と L4 の入力にも使える。
  - GitHub に Claude API の秘密情報がなく、費用は専用のワークスペースの上限を超えない。
- 悪い影響・トレードオフ:
  - 自分の PR の中で閉じた失敗も、Merge Queue で初めて見つかることが増える。落ちた PR は外れ、同じグループの後ろの PR が組み直しになる。
  - 外の貢献者の PR は、`ci:full` を付けない限り、Merge Queue まで型やテストの結果が分からない。
  - E2E のロケールと幅の退行が main に入ってから見つかるまでが、nightly の最長 1 日になる。流れの spec とコンテナの契約は、該当する変更なら Merge Queue で見つかる。
  - client を変えたマージグループは、E2E の流れの spec の分だけ遅くなり、runner を多く使う。
  - main への push のたびに `go test ./...` を回すので、main の runner の使用量が増える。テストの責務を分けるまでは、キャッシュの効果が小さい。
  - キャッシュを日付で作り直すので、日中に変わったパッケージは Merge Queue で毎回ビルドし直す。
  - `scripts/changed-contexts.sh` の判定に Merge Queue が頼る。
  - E2E のシャードは準備の時間の分だけ runner を多く使う。
  - CI の組み方を、タスクの `deps` とワークフローの 2 か所で持つ。手元で CI のジョブと同じ組を 1 つのコマンドで回す手段はなく、手元は `task check` か `task check:changed` を使う。`parallel` は 2026-06 からの機能で、使われた実績が少ない。
  - `app/web` に Storybook の設定とストーリーが増え、画面を足すたびにストーリーを書く。
  - Cloudflare のアカウントと API トークンが外部の依存として増える。トークンは Workers の編集だけに権限を絞る。Preview の URL は公開で、PR の未マージの UI を誰でも見られる（OSS なので内容はもともと公開）。
  - Claude API の上限を 2 つのジョブで分け合うので、nightly が上限まで使うと、リリースの本文の呼び出しは失敗し、draft は事実だけになる。リリースは止まらない。
  - WIF の設定が Claude Console と GitHub に分かれ、規則の `sub` の形を間違えると交換が `401` で失敗する。失敗の理由は Claude Console の認証の履歴で見る。
