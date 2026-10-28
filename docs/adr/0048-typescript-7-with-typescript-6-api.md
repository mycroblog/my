# ADR-0048: 型検査を TypeScript 7 にし、コンパイラ API を使うツールには TypeScript 6 を並べる

- Status: Accepted
- Date: 2026-10-19
- Deciders: @rosylilly
- Issue: #567

## Context（背景・課題）

[ADR-0025](0025-web-client-stack.md) は、クライアントのワークスペース（`app/`）の TypeScript を、typescript-eslint の対応範囲に合わせて 6.0 系に固定している。
決定者は TypeScript を古い版のままにしないことを求めている（#567）。

TypeScript 7.0（Go で書き直したコンパイラ、`typescript@7.0.2`、2026-07-08 公開）は、JavaScript から呼ぶコンパイラ API を持たない。
新しい API は 7.1 で入る予定で、6.0 までの API とは別物になる
（[Announcing TypeScript 7.0](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/)）。
そのため、`typescript` パッケージをそのまま 7 に上げると、コンパイラ API を読み込むツールが動かない（#539 の CI）。

`app/` で TypeScript を使うツールの対応状況（2026-10-19 時点）:

| ツール | TypeScript の使い方 | TS 7 への対応 |
| --- | --- | --- |
| `tsc`（型検査） | CLI | 7.0 で動く。`exactOptionalPropertyTypes` の判定が 6.0 より厳しく、`app/ui` のストーリー 1 件で型エラーになる |
| typescript-eslint 8.71 | コンパイラ API（型情報つきの lint） | 非対応。peer の範囲は `typescript >=4.8.4 <6.1.0`。TS 7 の API が出るまで対応しないとし、TS 6 を並べる方法を案内している（[typescript-eslint#12518](https://github.com/typescript-eslint/typescript-eslint/issues/12518)、追跡は [#10940](https://github.com/typescript-eslint/typescript-eslint/issues/10940)） |
| openapi-typescript 7.13 | コンパイラ API（`ts.factory` で型の AST を作る） | 非対応。[openapi-typescript#2841](https://github.com/openapi-ts/openapi-typescript/issues/2841) が未解決 |
| `@eslint-react`、TanStack Query の ESLint プラグインなど | typescript-eslint の parser と型情報を経由する | typescript-eslint に従う |
| knip 6.39 | oxc-parser と oxc-resolver で解析し、`typescript` に依存しない | 影響なし |
| Vite 8 / Vitest 5 / React Compiler（Babel） | 型を消すだけで、`typescript` を読み込まない | 影響なし |
| TanStack Router のルート生成 | 生成器は `typescript` を読み込まない | 影響なし |
| Storybook 10（`@storybook/react-vite`） | 既定の `reactDocgen: "react-docgen"`（Babel で解析）を使い、`typescript` を読み込む `react-docgen-typescript` は使っていない | 影響なし |
| `tools/i18n` | `tsc --noEmit` だけ | すでに `typescript@7.0.2` で型検査している |

TypeScript の公式は、API が要るツールのために、TS 6.0 の API を再エクスポートする `@typescript/typescript6`（`tsc6` という bin を持つ）を用意している。
`package.json` で `"typescript": "npm:@typescript/typescript6@…"` と `"@typescript/native": "npm:typescript@7.0.2"` の 2 つの別名を並べると、
`typescript` を読み込むツールは 6.0 の API を使い、`tsc` コマンドは 7.0 を使う、という並用の方法を示している
（同記事の「Running side-by-side with TypeScript 6.0」）。

## Decision Drivers（判断基準）

1. TypeScript を古い版のままにしない（決定者の意向）
2. 型検査・lint・API の型生成の検査を弱めない（`make check` の全項目を保つ）
3. 公式が示す方法に従い、独自の仕組みを作らない
4. 公開から 3 日以上たった版だけを使う（`app/bunfig.toml`）。ライセンスが AGPL-3.0 と両立する

## Considered Options（選択肢）

1. `typescript` を 7 に上げる（全部 7）
2. 型検査（`tsc`）だけ 7 にし、コンパイラ API が要るツールには公式の `@typescript/typescript6` を別名で並べる
3. TypeScript 7.1 の API と各ツールの対応を待ち、6.0 のままにする

## Decision（決定）

選択肢 2 を採る。

- `tsc` で型検査する `app/ui/package.json` と `app/web/package.json` の devDependencies を、次の 2 つの別名にする。

  ```json
  "@typescript/native": "npm:typescript@7.0.2",
  "typescript": "npm:@typescript/typescript6@6.0.2",
  ```

- ESLint と knip だけを動かすワークスペースのルート（`app/package.json`）は、`typescript` の別名だけを持つ。
- `tsc` コマンド（`typecheck` スクリプト、`make web-typecheck` / `ui-typecheck`）は `@typescript/native` の TypeScript 7 が動かす。
- `typescript` を `import` / `require` するツール（typescript-eslint、openapi-typescript など）は、`@typescript/typescript6` が再エクスポートする TypeScript 6.0 の API を使う。
- `tools/i18n` はコンパイラ API を使わないので、`typescript@7.0.2` のままにする。
- typescript-eslint と openapi-typescript が TypeScript 7 の API に対応したら、別名をやめて `typescript` 1 つに戻す。

理由:

- 選択肢 1 は、typescript-eslint と openapi-typescript が動かず、lint と API の型生成の検査を外すことになる（判断基準 2 に反する）。
- 選択肢 3 は、7.1 の API の時期も、各ツールの対応の時期も決まっておらず、古い版が残り続ける（判断基準 1 に反する）。
- 選択肢 2 は、TypeScript の公式と typescript-eslint の両方が案内している方法で、`make check` の全項目を保ったまま型検査を 7 にできる。

## Consequences（影響）

- 型検査は TypeScript 7 の速いコンパイラと、7 の厳しい判定で行う。lint と API の型生成は TypeScript 6.0 の型情報で動くので、型検査と lint で TypeScript の版が 2 つになる。
- 依存に TypeScript が 2 つ入り、`node_modules` が大きくなる（ツールだけで、成果物には入らない）。
- `@typescript/typescript6` は `typescript@^6` に依存するので、6.0 系の修正版は lockfile の更新で入る。
- Dependabot は 2 つの別名を別々に上げる。`typescript` の別名を 7 に上げる PR は、上の各ツールが対応するまで閉じる。
- ADR-0025 の、TypeScript を 6.0 系に固定する部分と、TypeScript 7 への移行時期の Open Question は、この ADR が置き換える。

## Open Questions / 要検証

- TypeScript 7.1 の API が出たときに、typescript-eslint と openapi-typescript が対応する時期
