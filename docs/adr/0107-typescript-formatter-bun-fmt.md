# ADR-0107: TypeScript のコードを bun fmt で整形する

- Status: Accepted
- Date: 2026-10-28
- Deciders: @rosylilly
- Issue: #1603

## Context（背景・課題）

Go のコードは `gofmt` で整形し、`go:fmt-check` で検査している。一方、TypeScript のパッケージ（`app/`、`pkg/mykit/ts`、`pkg/mybot/ts`、`tools/i18n`、`tools/mail`）には整形の道具がなく、
[ADR-0049](0049-client-lint-oxlint.md) の Oxlint が lint だけを受け持つ。行の幅と折り返しはファイルごとに違い、`*.ts` / `*.tsx` / `*.js` / `*.mjs` の 148,320 行のうち
120 文字を超える行が 5,216、150 文字を超える行が 1,314 ある（2026-10-28）。並行して作業するエージェントが、それぞれの癖で折り返すので、差分にも整形だけの変更が混ざる。

Bun 1.4.4（2026-10-22 公開）は、Prettier と互換の整形器 `bun fmt` を Bun 自身に入れた（[Bun v1.4.4](https://bun.com/blog/bun-v1.4.4)、`bun fmt --help --all` で 2026-10-28 に確認）。
決定者は次のように述べた（#1603）。

> `bun fmt` が使えるようになったそうなので TS コードを `bun fmt` するのをワークフローに組み込みたい

## Decision Drivers（判断基準）

1. 依存と道具を増やさない（固定した Bun のほかに、版を追う物を足さない）
2. 速く、手元でも CI でも同じ結果になる
3. 今のコードの書き方（二重引用符、セミコロン、末尾のカンマ、2 空白の字下げ）を保ち、一度の整形の差分を小さくする
4. Oxlint と `better-tailwindcss`（ADR-0049）の規則とぶつからない

## Considered Options（選択肢）

1. `bun fmt`（Bun 1.4.4 に入っている）
2. Prettier（npm の `prettier`、MIT）
3. oxfmt（npm の `oxfmt`、MIT。Oxlint と同じ Oxc の整形器）
4. 整形器を入れず、今のまま書き方をレビューで揃える

- 2 と 3 は npm の依存を 1 つ足し、Dependabot の更新と、3 日の公開待ち（`minimumReleaseAge`）の対象が増える。Prettier は JavaScript で動き、1 回に数秒かかる。
  oxfmt は 0.x 版（2026-10-28 に 0.75.0）で、出力が版ごとに変わりうる（要検証）。
- 1 は `bun fmt` の設定に Prettier の名前の設定（`[fmt]` の `printWidth` など）を読み、`.editorconfig` と `.prettierignore` を読む。
  手元で 1,143 ファイルを約 40 ms で整形した。
- 4 は何も足さないが、今の幅と折り返しのばらつきが残る。

## Decision（決定）

**選択肢 1 を採る。** TypeScript と JavaScript のコードは `bun fmt` で整形し、CI で検査する。

- 範囲は `app/`、`pkg/mykit/ts`、`pkg/mybot/ts`、`tools/i18n`、`tools/mail` の `*.ts`・`*.tsx`・`*.js`・`*.mjs` である。CSS・JSON（`locales/`）・Markdown・YAML（`api/spec`）は範囲に入れない。
- 設定はリポジトリの根の `bunfig.toml` の `[fmt]` に置く。`bun fmt` は `[fmt]` を持つ一番近い `bunfig.toml` を読むので、各パッケージの `bunfig.toml`（`[install]` だけ）は `[fmt]` を持たない。
  - `printWidth = 120`。ほかは Prettier の既定（二重引用符、セミコロン、`trailingComma: "all"`、`arrowParens: "always"`、`objectWrap: "preserve"`）で、字下げと改行は `.editorconfig` から読む。
    120 は、コメントと文書を折り返している幅で、今の行の 96.5 % が収まる。一度の整形で変わる量は、幅 100 で 977 ファイル（+40,250 / −9,407 行）、
    120 で 786 ファイル（+21,701 / −5,135 行）、140 で 584 ファイル（+11,212 / −2,951 行）、150 で 472 ファイル（+7,893 / −2,406 行）、160 で 406 ファイル（+5,746 / −2,222 行）だった。
    広い幅ほど一度の差分は小さいが、整形器はコメントを折り返さないので、コードとコメントの幅が揃う 120 を採る。
  - `sortImports`・`sortTailwindcss`・`sortPackageJson`・`jsdoc` は使わない（既定で切れている）。import の順と Tailwind のクラスは Oxlint と `better-tailwindcss` が受け持つ。
- 整形しないファイルは根の `.prettierignore` に挙げる（`.gitignore` のファイルも読まない）。API の仕様から作る `pkg/mykit/ts/src/schema.gen.ts`
  と、Oxlint の違反の見本（`app/oxlint/violations/`。各行の `expect:` のコメントが報告される行を指すので、配置が検査の中身である）と、
  `web:lint-rules` が動く間だけ `app/web/src` に置くその写しを外す。コミットしない生成物（[ADR-0081](0081-generated-files-policy.md)）は `.gitignore` にあるので読まない。
- タスクは `web:fmt`（書き込む）と `web:fmt-check`（`bun fmt --check`。整形が要るファイルがあると失敗する）で、範囲の全部を一度に扱う。
  `web:check`・`i18n:check`・`mail:check`・`mykit:check` が `web:fmt-check` を持つので、`check:changed` はどのパッケージを変えても検査する。
  CI では、Pull Request の preflight と、Merge Queue の `repo` ジョブで動かす（[ADR-0079](0079-ci-single-gate-merge-queue.md)）。
- TypeScript を変えたら、コミットの前に `scripts/task.sh web:fmt` を回す。整形したファイルの衝突は、解いたあとで `web:fmt` を回し直す。
- 一度の整形は整形だけのコミットにし、根の `.git-blame-ignore-revs` に挙げる。

理由:

- 選択肢 1 は依存を足さず（判断基準 1）、固定した Bun の版で手元と CI が同じ結果になり、速い（判断基準 2）。
  Prettier の既定は今のコードの書き方と同じで（判断基準 3）、並べ替えを切れば Oxlint とぶつからない（判断基準 4）。
- 選択肢 2・3 は同じ書き方にできるが、依存を 1 つ足す。選択肢 4 はばらつきを残す。

覆す条件:

- `bun fmt` の出力が Bun の版ごとに大きく変わり、Bun を上げるたびに大きな差分が出るなら、版を固定できる Prettier（選択肢 2）を考え直す。
- 範囲の外のファイル（CSS・JSON・Markdown・YAML）も揃える必要が出たら、範囲を広げる ADR を書く。

## Consequences（影響）

- 良い影響: TypeScript の差分に整形だけの変更が混ざらなくなり、レビューとエージェントの作業が書き方に迷わない。依存は増えない。
- 悪い影響: 一度の整形で 786 ファイルが変わり、開いている Pull Request の多くが衝突する。`.git-blame-ignore-revs` で `git blame` からは外せるが、
  GitHub の外の道具は `git config blame.ignoreRevsFile .git-blame-ignore-revs` が要る。
- `bun fmt` は Prettier と同じく、メソッドの連鎖などで 2 回目の整形で形が変わることがある（一度の整形では 2 ファイル）。`web:fmt` を 2 回回すと落ち着く。
- Bun の版を上げるときは、`web:fmt-check` が通るかを確かめ、変わったら整形だけのコミットを分ける。
- 後続: CSS・JSON・Markdown を `bun fmt` の範囲に入れるかを、要るときに Issue にする。
