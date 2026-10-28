# ADR-0065: React Compiler を Oxc の移植で動かし、Babel を外す

- Status: Accepted
- Date: 2026-10-21
- Deciders: @rosylilly
- Issue: #845

## Context（背景・課題）

[ADR-0025](0025-web-client-stack.md) は、技術スタックの表の「メモ化」の行で React Compiler 1.0（`babel-plugin-react-compiler`）を全コードに適用すると決めている。
[Web クライアントの設計規約](../architecture/web-client/README.md) の R4 は、コンパイラが最適化を諦めた関数があれば `vite build` を失敗させ、
`"use no memo"` で外した関数だけを除き、1 つも変換しなければ失敗させる（`app/web/vite-plugins/react-compiler-check.ts`）。
`app/web` と `app/ui` はそのために `@babel/core`・`@rolldown/plugin-babel`・`babel-plugin-react-compiler` を直接の依存に持ち、
`@vitejs/plugin-react` の `reactCompilerPreset` を Vite 8（Rolldown）の Babel プラグインとして動かす。Babel はツールチェーンの中でこのためだけにある。

`babel-plugin-react-compiler` は Babel 7 にしか対応せず、Babel 8 への更新（#837）を止めている。
決定者は「oxc-transform-react に移行することで根本的に Babel の依存取り除くのはどうでしょう」と提案し、その方針を承認した（#845）。
React チームは React Compiler を Rust に移植しており（[react/react の `compiler/crates`](https://github.com/react/react/tree/main/compiler/crates)）、
Oxc（`oxc-project/oxc`、MIT）はそれを取り込んだ `oxc-transform-react`（0.x）を npm に出している。

スパイク（[#845 の報告](https://github.com/mycroblog/my/issues/845#issuecomment-6308203825)、ブランチ `spike/845-oxc-react-compiler`）で、
`oxc-transform-react` 0.153.0 を `app/web` と `app/ui` の全コードに適用し、Babel 版との出力の差、R4 の仕組み、組み込み方と時間、既存の検査、成熟度を確かめた。

## Decision Drivers（判断基準）

1. 本番のコードを正しく変換する。Babel 版と比べて、メモ化する関数と諦める関数が変わらない
2. web-client.md R4 のゲート（諦めた関数で失敗、`"use no memo"` の扱い、1 つも変換しなければ失敗）を保つ
3. React Compiler のために Babel を持たない。Babel 7 に縛られない
4. ビルドとテストを速くする
5. 依存のライセンスが AGPL-3.0 と両立し、公開から 3 日以上たった版だけを使う（ADR-0011、ADR-0025）
6. 不具合が出たときに、短い手順で Babel 版に戻せる

## Considered Options（選択肢）

1. Babel 7 のまま（`babel-plugin-react-compiler` 1.0.0 を `@rolldown/plugin-babel` で動かす）
2. `oxc-transform-react` を自前の Vite プラグインで呼び、R4 のゲートを Oxc の診断で作る
3. `@vitejs/plugin-react` の `compiler` オプション（`oxc-transform-react` を読む experimental の機能）だけを使う

スパイクで分かったこと（2026-10-21、`oxc-transform-react` 0.153.0 と `babel-plugin-react-compiler` 1.0.0、同じ機械）:

| 項目 | Babel 1.0.0 | Oxc 0.153.0 |
| --- | --- | --- |
| メモ化する関数（636 ファイル） | 782 | 782（ファイルごとに全ファイルで同数） |
| 最適化を諦めた関数 | 0 | 0 |
| 出力の差 | — | 616 ファイルはコメントを除いて同じ。14 は書式だけ。6 はリアクティブスコープの切り方（`x ?? []` を別のスコープにするなど） |
| `app/web` の `vite build` | 9.3〜11.3 s | 1.84〜1.90 s |
| `dist/assets` の大きさ | 3,741,375 B | 3,741,718 B |
| Storybook の preview のビルド | 3.28 s | 1.96 s |
| `make web-check` / `make ui-check` | 通る | 通る |

- `oxc-transform-react` には Babel 版の `logger` がない。`reactCompiler.reportDiagnostics: true` で、諦めた関数・ルールの抑制・lint の所見が
  `result.errors`（`severity`・`message`・UTF-8 のバイト位置の `labels`・`helpMessage`）に入る。関数の位置と成功のイベントはない。
- `"use no memo"` の関数はコンパイルしないが、互換のないライブラリの診断は出す（[oxc#26140](https://github.com/oxc-project/oxc/issues/26140)）。
- `@vitejs/plugin-react` の `compiler` オプションは診断を警告として出すだけで（`logDiagnostics`）、ゲートに使える形で渡さない。
  peer は 6.1.1 が `oxc-transform-react@^0.145.0`、6.1.2 が `^0.152.0` で、0.x の `^` はマイナー版を固定する。
- コンパイラの中身は React チームの Rust の移植で、Oxc は [oxc-project/forked-react-compiler](https://github.com/oxc-project/forked-react-compiler)
  （`Cargo.toml` だけを直した MIT のフォーク）から `crates/oxc_react_compiler` に取り込み、Oxc の AST との変換層を持つ。
  React の e2e の対応表（[`compiler/crates/TODO.md`](https://github.com/react/react/blob/e8738f0c628b4c321c5ce8077390ba48a8208341/compiler/crates/TODO.md)、2026-10-12）は
  Babel 1792/1802、SWC 1786/1802、OXC 1704/1795（OXC の数は測り直していない）。
- リリースは 0.142.0（2026-08-08）から 0.155.0（2026-10-19）までほぼ毎週。README は experimental で、本番の前に出力を確かめるよう書く。
- 開いている React Compiler の issue 18 件の入力を両方でコンパイルすると、12 件は Babel 1.0.0 と同じ出力か同じ失敗（本家のコンパイラの挙動）、
  5 件は Oxc だけの誤変換（[#26161](https://github.com/oxc-project/oxc/issues/26161) BigInt、[#27402](https://github.com/oxc-project/oxc/issues/27402) 孤立サロゲート、
  [#27429](https://github.com/oxc-project/oxc/issues/27429) 関数の中の `declare const`、[#27401](https://github.com/oxc-project/oxc/issues/27401) 入れ子のメモ化ブロックの `let`、
  [#27113](https://github.com/oxc-project/oxc/issues/27113) `useMemo` の依存）、1 件は Rust の panic で Node のプロセスごと終わる（[#27404](https://github.com/oxc-project/oxc/issues/27404)）。
  今の `app/web` と `app/ui` にこれらの形のコードはない。

## Decision（決定）

**選択肢 2 を採用する。** React Compiler は `oxc-transform-react` で動かし、`app/web` と `app/ui` の直接の依存から Babel を外す。

- この ADR は、[ADR-0025](0025-web-client-stack.md) の技術スタックの表の「メモ化」の行のうち、コンパイラの実装（`babel-plugin-react-compiler`）を置き換える。
  React Compiler 1.0 の規則を全コードに適用し、手動の `useMemo` / `useCallback` / `memo` を原則書かないことは変えない。
- `app/web` は、`@rolldown/plugin-babel` と `reactCompilerPreset` の代わりに、`oxc-transform-react` を呼ぶ 1 ファイルの Vite プラグイン
  （`app/web/vite-plugins/react-compiler.ts`）を置く。
  - client の環境のファイルだけを変換し（web-client.md T6）、plugin-react と同じコードのフィルタを使う。`node_modules` は変換しない。
  - `jsx: "preserve"` で TypeScript だけを除いてコンパイラを通し、JSX と React Refresh は Vite 自身の Oxc の変換に任せる（Babel 版と同じ分担）。
  - R4 のゲートは同じプラグインが持つ。`reportDiagnostics` の診断が 1 つでもあれば、場所と理由を挙げて `vite build` を失敗させる。
    `"use no memo"` / `"use no forget"` で始まる関数の中の診断は除く。関数の範囲は Vite が出す `parseSync`（Rolldown の Oxc のパーサ）で求め、
    診断の UTF-8 のバイト位置を UTF-16 の位置に直して比べる。出力の `_c(N)` が 1 つもなければ失敗させる。
  - Babel 版のゲートは位置のない検証を数えないが、このゲートは診断のすべてを失敗にするので、より厳しい。決定者はこの厳しさを受け入れている。今のコードでの差は 0 件。
- `app/ui`（Storybook と Vitest）は、`@vitejs/plugin-react` の `compiler` オプションで同じ `oxc-transform-react` を使う。ゲートは `app/web` のビルドが持つ。
- 0.x の experimental な `oxc-transform-react` を本番のビルドに使う。決定者はこれを受け入れている。
- 版は、`app/web` と `app/ui` で同じ `oxc-transform-react` の版に固定し、`@vitejs/plugin-react` の peer の範囲に入る版にそろえる。
- 版を上げるときは、`app/web` のビルドが出すメモ化した関数の数と、`make web-check`・`make ui-test`・`make web-e2e` の結果を PR に書く。
  数が変わったら、変わったファイルの出力を読んで確かめる。
- Oxc だけの誤変換の issue に当たる書き方をコードで使うときは、その issue を Oxc で直した版にしてから使う。
- 次のどれかが起きたら、Babel 版に戻す新しい ADR を書く。
  - Oxc だけの誤変換が本番のコードに当たり、直った版が 3 日の待ちを含めて 4 週間以内に使えない
  - `oxc-transform-react` の公開が 8 週間止まる、または Oxc が React Compiler の移植をやめる
  - `@vitejs/plugin-react` が `compiler` オプションを外し、今の Vite に合う版の `oxc-transform-react` がなくなる

理由:

- 選択肢 2 は、Babel 版とメモ化する関数がファイルごとに同じで、テストも通る（判断基準 1）。R4 のゲートの 3 つの仕組みを Oxc の診断と Vite のパーサで作れ、
  スパイクで描画中の ref を入れたコードがビルドで失敗した（判断基準 2）。React Compiler のための Babel がなくなり、Babel 7 の縛りが消える（判断基準 3）。
  `app/web` のビルドは約 5.5 倍速い（判断基準 4）。追加する `oxc-transform-react` は MIT で、公開から 3 日以上たった版を使う（判断基準 5）。
  プラグインは 1 ファイルで、戻すときは Babel のプリセットと今の検査を戻すだけで済む（判断基準 6）。
- 選択肢 1 は今いちばん枯れているが、判断基準 3 と 4 を満たさない。Babel 8 への更新は、`babel-plugin-react-compiler` が Babel 8 に対応するまでできない。
- 選択肢 3 は設定が最も短いが、診断を構造のある形で受け取れず、R4 のゲートを作れない（判断基準 2）。`app/ui` のようにゲートが要らない所には足りる。
- lint の React Compiler の規則はすでに Oxlint で動き（[ADR-0049](0049-client-lint-oxlint.md)）、Oxlint の規則も同じ `oxc_react_compiler` クレートを使う
  （[`crates/oxc_linter/Cargo.toml`](https://github.com/oxc-project/oxc/blob/main/crates/oxc_linter/Cargo.toml)、2026-10-21 に確認）。
  ビルドもこのクレートになり、lint とビルドが同じコンパイラで諦める関数を判断する。
- 移行のときに `make ui-test`・`make web-e2e` が Babel 版と違う結果になる、または `@vitejs/plugin-react` の peer の範囲に入る版で
  スパイクと同じ結果にならないときは、移行を止めてこの ADR を見直す。

## Consequences（影響）

- 良い影響
  - `app/web` と `app/ui` の直接の依存から `@babel/core`・`@rolldown/plugin-babel`・`babel-plugin-react-compiler`・`@types/babel__core` がなくなる。
  - `vite build` が約 10 秒から約 2 秒になり、Storybook とテストの変換も速くなる。
  - 諦めた関数の報告が、関数の先頭ではなく原因の式の場所を指す。
- 悪い影響・トレードオフ
  - 0.x の experimental な変換を本番のビルドに使う。0.x はマイナー版でオプションが変わり得る。React の e2e では Babel 版より失敗が多く、
    Oxc だけの誤変換の issue が開いている。版を上げるたびに出力の確認が要る。
  - Rust の panic はビルドのエラーではなく、Node のプロセスの異常終了になる。ビルドは失敗するが、原因のファイルを示さない。
  - Babel は推移的な依存として `app/bun.lock` に残る。`@tanstack/router-plugin`（ルートのコード分割がビルドでルートのファイルに Babel を通す）、
    `vite-plugin-pwa` の `workbox-build`、Storybook の `react-docgen`、`@babel/code-frame` を使う Testing Library などが持つ。
    それらの Babel の版はそれぞれのライブラリが決める。
  - Oxc はコンパイルした関数の中のコメントを出力から落とす。開発サーバーの出力が読みにくくなるが、ソースマップは残る。
  - `"use no memo"` の関数の診断を除くために、ゲートが自前でソースを解析する。
- 後続で必要になる作業（Issue 化する）
  - 移行: `app/web` と `app/ui` の Vite の設定の切り替え、`react-compiler-check.ts` を `oxc-transform-react` のプラグインとそのテストに置き換える、Babel の依存の削除。
    web-client.md の R4 と T6、`make web-dist` の説明を新しい仕組みに合わせる。移行の PR で `make ui-test` と `make web-e2e` を回す。

## Pros and Cons of the Options（詳細比較・任意）

| 選択肢 | 良い点 | 悪い点 |
| --- | --- | --- |
| 1. Babel 7 のまま | React の公式のプラグインで、e2e の対応が最も多い。今の仕組みのまま | Babel 7 に縛られる。`vite build` が約 5.5 倍遅い。React Compiler のためだけに Babel を持つ |
| 2. 自前のプラグインで `oxc-transform-react` | Babel 版と同じ関数をメモ化し、R4 を保つ。速い。lint と同じ系統 | 0.x の experimental。Oxc だけの誤変換と panic の issue がある。プラグインとゲートを自前で保守する |
| 3. plugin-react の `compiler` オプションだけ | 設定が 1 行 | R4 のゲートを作れない。experimental。peer がマイナー版を固定する |

## Open Questions / 要検証

- 要検証: `@vitejs/plugin-react` の peer の範囲に入る版（6.1.2 と `oxc-transform-react` 0.152.x）でも、スパイクの 0.153.0 と同じ出力と結果になるか（移行の Issue）
- 要検証: `make ui-test`（ストーリーのスクリーンショット）と `make web-e2e` が Oxc 版でも通るか。スパイクでは回していない（移行の Issue）
