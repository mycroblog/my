# ADR-0031: 共有 UI キットは、ドメインを持たない振る舞いのライブラリにも依存してよい

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #188

## Context

[ADR-0026](0026-client-workspace-and-component-architecture.md) は、共有 UI キット `@my/ui` が依存してよいものを「React・React Aria Components・Tailwind のユーティリティだけ」と定めている。
一方、タイムラインの部品（`Feed`）には長いリストの仮想化が必要で（ADR-0025）、`@tanstack/react-virtual` を使っている（#125）。
また、部品の見た目の定義には `tailwind-variants` と `tailwind-merge` を使っている（ADR-0026 のロジックと UI の分離の節）。

ADR-0026 の依存の制限の目的は、UI キットがアプリのドメイン（API・ルーティング・サーバー状態・i18n のカタログ）を知らないことを保証し、Tauri と共有できるようにすることである。
ドメインを持たない振る舞いのライブラリは、この目的に反しない。

## Decision Drivers

1. UI キットがアプリのドメインを知らないこと（ADR-0026 の目的）
2. 依存の追加を、レビューと lint で管理できること
3. 部品の振る舞い（仮想化・見た目の合成など）を自前で再実装しないこと

## Considered Options

1. **ドメインを持たない振る舞い・スタイルのライブラリを、許可リストで管理して認める**
2. 仮想化などは `app/web` の機能側で行い、UI キットは仮想化しない一覧だけを持つ
3. 必要な振る舞いを UI キット内で自前実装する

## Decision

**1 を採用する。** `@my/ui` が依存してよいのは次のものに限り、許可リストは lint（`eslint-plugin-boundaries`）と `app/ui/package.json` で管理する。

- React、React Aria Components / React Aria
- Tailwind のユーティリティ: `tailwind-variants`、`tailwind-merge`
- ドメインを持たない振る舞いのライブラリ: 現在は `@tanstack/react-virtual`
- アイコン: `react-icons`（Phosphor の `react-icons/pi` だけ、`Icons.tsx` からだけ。[ADR-0043](0043-icon-set.md)）

許可リストに追加できるのは、次をすべて満たすものだけとする。追加は PR の説明で理由を示し、この ADR の一覧を更新する。

- アプリのドメイン（API・ルーティング・サーバー状態・i18n のカタログ・認証）に依存しない
- Web と Tauri の WebView の両方で動く
- AGPL-3.0 と両立するライセンスで、公開から 3 日以上たった版を固定する（ADR-0011、ADR-0025）

ADR-0026 の「依存の向き」の節のうち、`@my/ui` の依存先の一覧はこの ADR が置き換える。依存の向き（`routes → features → (@my/ui, lib)`）と、ドメインを import しない制約はそのまま有効である。

## Consequences

- 仮想化のような汎用の振る舞いを UI キットに置けるので、Web と Tauri で同じ部品を使える。
- 依存が増えるたびに、この ADR の一覧と lint の許可リストを更新する手間がかかる。
