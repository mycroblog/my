# ADR-0049: クライアントの lint を ESLint から Oxlint に移す

- Status: Accepted
- Date: 2026-10-19
- Deciders: @rosylilly
- Issue: #572

## Context（背景・課題）

クライアントのワークスペース（`app/`）の lint は、[ADR-0025](0025-web-client-stack.md) と [ADR-0027](0027-ui-quality-gates.md) が定める ESLint（flat config）と
そのプラグイン（typescript-eslint、`eslint-plugin-react-hooks`、`@eslint-react`、TanStack Query / Router、Testing Library、boundaries、better-tailwindcss、formatjs、eslint-comments）
で行い、[Web クライアントの設計規約](../architecture/web-client/README.md) の「強制」の列の多くを守っている。独自の禁止規則は `app/eslint/restrictions.js` にある。

この ESLint は 1 回に約 50 秒かかり（手元 52.8 秒、CI 50.4 秒）、`make web-lint` と CI の web ジョブで最も長い手順になっている。
時間の大半は typescript-eslint の型情報（TypeScript 6 のコンパイラ）で、ESLint 10 の `--concurrency` は 4 CPU の CI では 1 割ほどしか縮めない。
決定者は ESLint が遅いことを理由に、より速い linter への乗り換えを求めている（#572）。

調査（[#572 の報告](https://github.com/mycroblog/my/issues/572#issuecomment-6265430885)）で、Biome、Oxlint、Oxlint と最小の ESLint の併用を、
今の 278 規則と独自の禁止規則のすべてについて比べ、同じ機械で時間を測った。

## Decision Drivers（判断基準）

1. lint の時間を大きく縮める（CI の 4 vCPU で効く）
2. web-client.md の「強制」の列の行を、できるだけ lint から外さない
3. 依存とツールの数を減らす
4. 公開から 3 日以上たった版だけを使い、ライセンスが AGPL-3.0 と両立する
5. TypeScript 7（[ADR-0048](0048-typescript-7-with-typescript-6-api.md)）の妨げにならない

## Considered Options（選択肢）

1. ESLint のまま、`--concurrency auto` で並列にする
2. Oxlint 単独（ネイティブの規則、tsgolint による型情報の規則、ESLint のプラグインを JS プラグインとして動かす）
3. Oxlint と、Oxlint の JS プラグインでは動かない型情報が要る規則だけの ESLint を、並列に動かす
4. Biome（ネイティブの規則と GritQL のプラグイン）
5. Biome と、Biome にない規則の ESLint

調査で分かったこと（2026-10-19、各公式の文書と、spike ブランチでの計測）:

| 選択肢 | 時間（手元 24 コア / 4 CPU、warm の中央値） | 「強制」の列から外れる行 |
| --- | --- | --- |
| 1 | 31.0 s / 45.0 s | なし |
| 2 | 8.8 s / 8.6 s | R14 |
| 3 | 8.8 s / 9.6 s | なし |
| 4 | 1.4 s / 2.8 s | A1〜A3、A9、D8、D9、S1 と S7 の TanStack の規則、T2、Y2、R9、R14。D2・D5〜D7 は一部だけ |
| 5 | 1.4 s ＋ 49.7 s | なし |

- Oxlint の JS プラグインは alpha で、ESLint v9 以降のプラグイン API を対象にし、型情報を使う規則は動かない（[JS Plugins](https://oxc.rs/docs/guide/usage/linter/js-plugins.html)）。
  今の規則のうち動かないのは `@eslint-react/no-leaked-conditional-rendering`（R14）と `@eslint-react/no-unused-props` の 2 つだけだった。
- Oxlint の型情報の規則は tsgolint が typescript-go で動かし、typescript-eslint の型情報の規則 61 のうち 59 に対応し、TypeScript 7.0 以上を前提にする
  （[Type-Aware Linting](https://oxc.rs/docs/guide/usage/linter/type-aware.html)）。今の規則のうち対応しないのは `@typescript-eslint/no-unsafe-enum-assignment` だけだった。
- Biome のプラグインは GritQL で、一致したところを報告するだけで、ESLint のプラグインは動かない（[Linter Plugins](https://biomejs.dev/linter/plugins/)）。
  型を使う規則は TypeScript を使わない独自の推論で、今のコードに 74 件の偽陽性を出した。
- 選択肢 5 は、ESLint 側に typescript-eslint の型情報の規則が残るので速くならない。

## Decision（決定）

**選択肢 2 を採用する。** lint は Oxlint だけで行い、ESLint とその実行は使わない。

- lint は Oxlint（`oxlint`、`oxlint-tsgolint`）で行う。設定は `app/.oxlintrc.json` にし、`@oxlint/migrate --type-aware` で今の `app/eslint.config.js` から作る。
  - typescript-eslint の型情報の規則は tsgolint で動かす（`options.typeAware`）。
    tsgolint はファイルごとに一番近い `tsconfig.json` を使うので、`app/ui` と `app/web` の `tsconfig.json` は `references` だけを持つ solution 形式にし、
    アプリの設定は `tsconfig.app.json` に置く（`tsc -p` には `tsconfig.app.json`・`tsconfig.node.json` などを渡す）。これで `app/web/sw` と `app/web/e2e` の型も解決する。
  - `eslint-plugin-react-hooks` の規則は Oxlint のネイティブの `react/*` で動かす。
  - `@eslint-react`、TanStack Query / Router、Testing Library、boundaries、better-tailwindcss、formatjs、eslint-comments は、Oxlint の JS プラグイン（`jsPlugins`）で動かす。
    JS プラグインは alpha だが、決定者はこれを受け入れている。
  - `no-restricted-syntax` は Oxlint にないので、`restrictions.js` のセレクタを受け取って報告する 1 ファイルの JS プラグインをリポジトリに置く。
    `restrictions.test.js` は Oxlint の RuleTester（`oxlint/plugins-dev`）で、各禁止規則が違反を拒むことを確かめる。
- R14（数値を `&&` の条件にしない）は、型情報が要る `@eslint-react/no-leaked-conditional-rendering` が Oxlint では動かないので、lint では検査せず、レビューで確かめる。
  `@eslint-react/no-unused-props` と `@typescript-eslint/no-unsafe-enum-assignment` も検査から外れる。
- 無効化コメントは `eslint-disable` の形のまま使う。R9 の `no-restricted-disable` には、`react-hooks/*` に加えて Oxlint の `react/` の名前（`react/exhaustive-deps` など）を挙げる。
- `make web-lint` は Oxlint・`restrictions.test.js`・knip・CSS の `100vh` の検査を動かす。
- 依存から ESLint、typescript-eslint、`eslint-plugin-react-hooks` を外す。JS プラグインとして使うプラグインは残す。
  knip は `.oxlintrc.json` の `jsPlugins` と `settings` を読んで、JS プラグインと boundaries の resolver を使われている依存として数える。

理由:

- 選択肢 2 は、時間を約 6 分の 1 にし（判断基準 1）、ESLint の実行をなくす（判断基準 3）。外れる「強制」の行は R14 だけで、決定者は R14 を lint で検査しないことを受け入れている（判断基準 2）。
- 選択肢 3 は R14 を保つが、ESLint と typescript-eslint を R14 のためだけに残す。選択肢 1 は CI では速くならない。
  選択肢 4 は最速だが、境界・i18n・Tailwind・Testing Library・TanStack の検査を失う。
- 追加する依存（`oxlint`、`oxlint-tsgolint`）は MIT で、公開から 3 日以上たった版を使う（判断基準 4）。
- 型情報の規則は TypeScript 7 の typescript-go で動き、lint は TypeScript 6 の API に依存しなくなる（判断基準 5）。

## Consequences（影響）

- `make web-lint` の lint の手順が約 50 秒から約 9 秒になる。
- この ADR は、[ADR-0025](0025-web-client-stack.md) の技術スタックの表の「lint」の行と、[ADR-0027](0027-ui-quality-gates.md) の検査の層の表の「lint」の行を置き換える。
  プラグインと規則は同じものを使い、それを動かすのが ESLint から Oxlint に変わる。
- web-client.md の「強制」の列で、R14 はレビューになる。ほかの行の規則名は、Oxlint で動く名前（`react-hooks/purity` は `react/purity` など）に合わせる。
- 「強制」の列の大半（境界・i18n・Tailwind・Testing Library・TanStack）が、alpha の Oxlint の JS プラグインで動く。Oxlint の版を上げるたびに、
  違反のフィクスチャで各行が検出されることを確かめる必要がある。
- React Compiler の規則は、`eslint-plugin-react-hooks` ではなく Oxlint の移植で動く。移植は `"use no memo"` の関数の中でも `react/incompatible-library` を報告するなど、
  細部が公式のプラグインと違う。
- typescript-eslint がなくなるので、ADR-0048 の TypeScript 6 の API の別名が要るのは openapi-typescript だけになる。
- 移行の作業は Issue にする（`make web-lint` と CI の切り替え、`restrictions.js` とそのテストの移植、ESLint とそのプラグインの削除、web-client.md の「強制」の列の更新）。
