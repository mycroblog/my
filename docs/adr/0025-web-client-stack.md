# ADR-0025: Web クライアントの技術スタック

- Status: Accepted（TypeScript を 6.0 系に固定する部分と、TypeScript 7 への移行時期の Open Question を ADR-0048（型検査を TypeScript 7 にし、コンパイラ API を使うツールには TypeScript 6 を並べる） が、技術スタックの表の lint の行を ADR-0049（クライアントの lint を ESLint から Oxlint に移す） が、技術スタックの表の「メモ化」の行のコンパイラの実装を ADR-0065（React Compiler を Oxc の移植で動かし、Babel を外す） が置き換える）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #85

## Context

Web クライアント（PWA、ADR-0008）は React と TailwindCSS をベースにする（オーナーの指定）。
React は、ネット上のコードや一般的な知識に古い・誤った書き方（Effect の濫用、手動メモ化、サーバーデータのコピーなど）が多い。
そのため、React 公式ドキュメント（react.dev）などの一次情報に基づいて、周辺ライブラリと書き方を決める必要がある。
調査結果は #84 の調査報告にある（各ライブラリの版は 2026-10-16 時点の npm `latest`）。

## Decision Drivers

1. React 公式の設計原則（Rules of React、React Compiler、You Might Not Need an Effect）に沿って書けること
2. アクセシビリティ（WCAG 2.2 AA）・RTL・タッチ操作を、部品の土台で保証できること（ADR-0009, NFR-A11Y-1）
3. サーバーが Go の SPA であること（RSC・SSR は使わない、ADR-0028）
4. OpenAPI の型（ADR-0016）をそのまま使えること
5. 依存のライセンスが AGPL-3.0 と両立すること（ADR-0011）
6. 後で Tauri 2（ADR-0008）と UI を共有できること

## Considered Options

- 部品の土台: React Aria Components / Base UI / Radix / Ark UI / 自作
- ルーター: TanStack Router / React Router（v8 の library / data mode）
- サーバー状態: TanStack Query / React 19 の `use` + Suspense だけ / SWR
- テストランナー: Vitest / `bun test`

## Decision

| 領域 | 採用 |
| --- | --- |
| UI ライブラリ | React 19（`react` / `react-dom` 19.3 系）、Strict Mode |
| メモ化 | React Compiler 1.0（`babel-plugin-react-compiler`）を全コードに適用。手動の `useMemo` / `useCallback` / `memo` は原則書かない |
| 言語 | TypeScript（`strict`、`noUncheckedIndexedAccess`、`exactOptionalPropertyTypes`）。typescript-eslint の対応範囲に合わせて 6.0 系に固定 |
| ビルド | Vite 8 + `@vitejs/plugin-react` 6。Bun をパッケージ管理とスクリプト実行に使う |
| スタイル | Tailwind CSS v4（`@tailwindcss/vite`）、`tailwind-variants`、`tailwind-merge` |
| アクセシブルな部品の土台 | React Aria Components（Apache-2.0）。自前の UI キット（ADR-0026）が包んで公開する |
| ルーティング | TanStack Router（ファイルベース、自動コード分割、Zod で検索パラメータを検証） |
| サーバー状態 | TanStack Query 5（`queryOptions`、無限スクロールは `useInfiniteQuery`） |
| 長いリスト | TanStack Virtual |
| API クライアント | `openapi-typescript` + `openapi-fetch` + `openapi-react-query`（ADR-0029） |
| i18n | `react-intl`（FormatJS、ADR-0009）。カタログと型付きアクセサは `tools/i18n` が生成 |
| 検証 | Zod 4（検索パラメータとフォーム） |
| エラー境界 | `react-error-boundary` |
| テスト | Vitest（ブラウザモードを含む）、Testing Library、`user-event`、MSW、Playwright + `@axe-core/playwright`（ADR-0027） |
| 部品カタログ | Storybook 10（`@storybook/react-vite`、a11y アドオン、Vitest アドオン） |
| lint | ESLint（flat config）、typescript-eslint、`eslint-plugin-react-hooks`（コンパイラ由来のルールを含む `recommended`）、`@eslint-react`、TanStack Query / Router のプラグイン、`eslint-plugin-testing-library`、`eslint-plugin-boundaries`、`eslint-plugin-better-tailwindcss`、`knip` |
| Node.js | Vite・Vitest・Storybook・Playwright は Node.js（LTS、`.tool-versions` で固定）で動かす。`bun --bun` は使わない |

選定の理由:

- **React Aria Components**: 候補の中で i18n（`ja` / `ar` の組み込み文言、RTL でのキー操作の反転）が最も強い。主要なスクリーンリーダーでテストされている。マウス・タッチ・キーボードの押下を正規化するので、Tauri の WebView にも向く。
- **TanStack Router**: React Router の型安全なルートは framework mode 限定で、SPA では型安全にならない。
- **TanStack Query**: React 19 の `use` はキャッシュを持たず、無効化・ページングも提供しない。
- **Vitest**: React Compiler の変換をテストにも同じように適用でき、Storybook と組み合わせて実ブラウザでアクセシビリティの検査ができる。
- **Vite・Vitest の実行環境を Node.js にする**: `bun --bun` で動かしても Vite のビルド成果物は Node.js と同一で、速さの差も 1 秒未満と小さい。
  一方、Vitest は `bun --bun` だと jsdom 環境のワーカーを起動できない（2026-10-16 に Vite 8.3.2・Vitest 5.0.3・Bun 1.4.3・Node.js 24.21 で確認、#117）。
  Storybook・Playwright も Node.js を前提とするので、ツールの実行環境は Node.js の 1 つにそろえる。
  パッケージのスクリプトは `bun run` で起動し、`vite` などの bin は shebang に従って Node.js で動く。

具体的なバージョンは各パッケージの `package.json` と lockfile で固定する。公開から 3 日未満の版は使わない（`app/bunfig.toml` の `minimumReleaseAge`）。

## Consequences

- 書き方の規約は [Web クライアントの設計規約](../architecture/web-client/README.md) にまとめ、可能なものは lint で強制する。
- Node.js と Bun の 2 つの JS 実行環境を固定・保守する。
- React Compiler がルール違反のコンポーネントを黙って最適化から外すので、関連する lint は error にする。

## Open Questions / 要検証

- [ ] TypeScript 7（Go 製コンパイラ）への移行時期（typescript-eslint の対応待ち）
