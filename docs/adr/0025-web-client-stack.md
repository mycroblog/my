# ADR-0025: Web クライアントのスタックと構成

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #85, #86, #188, #483, #567, #572, #845, #2863, #2948

## Decision（決定）

### 1. 技術スタック

| 領域 | 採用 |
| --- | --- |
| UI ライブラリ | React 19（`react` / `react-dom` 19.3 系）、Strict Mode |
| メモ化 | React Compiler 1.0 の規則を全コードに適用する（実装は §9）。手動の `useMemo` / `useCallback` / `memo` は原則書かない |
| 言語 | TypeScript（`strict`、`noUncheckedIndexedAccess`、`exactOptionalPropertyTypes`）。版は §7 |
| ビルド | Vite 8 + `@vitejs/plugin-react` 6 |
| 実行環境 | Bun だけ。パッケージ管理・スクリプト・すべてのツールを Bun で動かす（§10） |
| スタイル | Tailwind CSS v4（`@tailwindcss/vite`）、`tailwind-variants`（クラスの合成も持つ） |
| アクセシブルな部品の土台 | React Aria Components（Apache-2.0）。自前の UI キット（§2）が包んで公開する |
| ルーティング | TanStack Router（ファイルベース、自動コード分割、Zod で検索パラメータを検証） |
| サーバー状態 | TanStack Query 5（`queryOptions`、無限スクロールは `useInfiniteQuery`） |
| 長いリスト | TanStack Virtual |
| API クライアント | `openapi-typescript` + `openapi-fetch` + `openapi-react-query`（ADR-0016） |
| i18n | `react-intl`（FormatJS、ADR-0009）。カタログと型付きアクセサは `tools/i18n` が生成 |
| 検証 | Zod 4（検索パラメータとフォーム） |
| エラー境界 | `react-error-boundary` |
| テスト | Vitest（ブラウザモードを含む）、Testing Library、`user-event`、MSW、Playwright + `@axe-core/playwright`（ADR-0110） |
| 部品カタログ | Storybook 10（`@storybook/react-vite`、a11y アドオン、Vitest アドオン） |
| lint | Oxlint（`oxlint`、`oxlint-tsgolint`。§8）、`knip` |

音声通話の `livekit-client` と `sora-js-sdk` は ADR-0101 が足す。

選定の理由:

- **React Aria Components**: 候補の中で i18n（`ja` / `ar` の組み込み文言、RTL でのキー操作の反転）が最も強い。主要なスクリーンリーダーでテストされている。マウス・タッチ・キーボードの押下を正規化するので、Tauri の WebView にも向く。
- **TanStack Router**: React Router の型安全なルートは framework mode 限定で、SPA では型安全にならない。
- **TanStack Query**: React 19 の `use` はキャッシュを持たず、無効化・ページングも提供しない。
- **Vitest**: React Compiler の変換をテストにも同じように適用でき、Storybook と組み合わせて実ブラウザでアクセシビリティの検査ができる。

具体的なバージョンは各パッケージの `package.json` と lockfile で固定する。公開から 3 日未満の版は使わない（ワークスペースの `minimumReleaseAge`）。

書き方の規約は [Web クライアントの設計規約](../architecture/web-client/README.md) にまとめ、可能なものは lint で強制する。
React Compiler がルール違反のコンポーネントを黙って最適化から外すので、関連する lint は error にする。

### 2. ワークスペースとパッケージ

リポジトリのルートを唯一の Bun ワークスペースにする。パッケージ管理・TypeScript の基本設定・lint・整形の設定はルートに 1 つずつ置き、各パッケージはそれを継いで違うところだけを上書きする。Web のパッケージは `web/` の下に置き、i18n のツールもメンバーにする。`pkg/mykit/ts` と `pkg/mybot/ts`（ADR-0093）もメンバーで、パスと共有の設定のほかは変えない。

| パッケージ | 持つもの |
| --- | --- |
| `@my/design-tokens` | 値: 色・余白・文字・ブレークポイント・動きの時間とイージング（ADR-0059） |
| `@my/design-system` | 見た目と振る舞いの仕組み: Tailwind、部品、動き、スクロール、フィードバック、ショートカット |
| `@my/platform` | ブラウザ・PWA・OS ができることとその差（ADR-0028） |
| `@my/rtc` | 通話の音声を運ぶもの（ADR-0101） |
| `@my/app-data` | API（mykit）、エンティティのキャッシュ、同期する状態、ストリーム、送信待ち（ADR-0028） |
| `@my/app-components` | 機能をまたいで使う表示の部品と、本文の描画 |
| `@my/app-features` | 機能ごとのディレクトリ（タイムライン・投稿・通話・通知・メディア・管理・認証・決済…） |
| `@my/app` | 組み立てだけ: シェルとスロット、ルート、エントリ（ADR-0028）、方針 |

層の外に、`@my/i18n`、`@my/mail`（ADR-0086）、`@my/e2e`、`@my/docs`（ADR-0150）、開発用の `@my/devtools`（ADR-0168）を置く。

### 3. 名前と依存の向き

- **名前の規則は 1 つ**: `app-` の付くパッケージは mycroblog を知り、付かないものは知らない。
- 上下は名前ではなく依存の向きで決め、検査で縛る。逆向きは禁止する。

  ```text
  design-tokens ← design-system ← app-components ← app-features ← app
  platform, rtc ←──────────────── app-data ←─────── app-features
  ```

- 機能は他の機能を import しない。機能どうしは app-data を通すか、app が配線する。
- app は状態を持たず、データを取得しない。ルートと置くもの、遷移の種類、キーマップ、ビューポートごとの配置、エントリの起動を宣言する。

### 4. 層の決まり

- **Tailwind のクラスを書くのは `@my/design-system` だけにする。** 上の層は部品と variant を組み合わせ、`className` を渡さない。デザインシステムを替えればすべての画面に届く。足りない variant はデザインシステムに足す。
- **差し込むもの**: デザインシステムの部品の文言（「閉じる」など）は部品と一緒に持ち、ロケールは app が provider で渡す（React Aria と同じ形）。日付・数値・相対時刻の整形（ADR-0009）と計測の送り先（ADR-0167）も差し込む。`@my/design-system` と `@my/rtc` は `@my/i18n` に依存しない。
- **`@my/app-components` は表示だけ**: データは props で受け取り、取得しない。リモートの本文（カスタム絵文字・メンション・リンク・ハッシュタグ・`dir="auto"`・キーワードフィルタの畳み）は 1 つの描画器だけが描く。
- **機能のロジックは UI を持たないフックに置き、サーバーのデータは app-data だけで扱う。** `useState` や Context にコピーしない。
- **仕組みは下の層、方針は上の層に置く。** 層をまたぐ関心事の持ち主:

| 関心事 | design-tokens / design-system | app-components / app-features | app |
| --- | --- | --- | --- |
| アクセシビリティ | 部品単体: role、フォーカス、キー操作、コントラスト、動きの抑制 | 意味: アクセシブルな名前、ラベル、読み上げ | ページ: h1、ランドマーク、タイトル、遷移時のフォーカスと読み上げ、スキップリンク |
| ショートカット | 仕組み: スコープ、IME と入力欄、`aria-keyshortcuts`、一覧 | 操作の ID（`post.reply`）を宣言して受ける | キーマップ: どのキーがどの操作か |
| ビューポート | ブレークポイントはトークン。コンテナに合わせる配置の部品 | コンテナクエリだけ。ビューポートの判定は禁止 | ビューポートで決めるのはここだけ: ナビゲーション、ダイアログか全画面か、ペイン |
| 遷移と動き | 操作の種類（`push`・`pop`・`modal`・`replace`・`none`）とその動き、View Transitions、共有要素 | 共有要素に名前を付ける | どの移動がどの種類か |
| フィードバック | 押下と処理中、トースト、その場の結果、ライブリージョン | 何をいつ言うか。振動は platform | — |
| スクロール | 仮想化、上への挿入で位置を保つアンカー、位置の保存と復元、sticky | タイムラインの意味: 自動で差し込むか「N 件の新着」か、欠け、既読の位置 | ルートごとに復元するか先頭か |
| 大きな表・画像 | データグリッド、画像ビューア | 管理の一覧。アップロード・縮小・代替テキスト・blurhash | — |

### 5. `@my/design-system` の依存の許可リスト

ドメインを持たない振る舞い・スタイルのライブラリを、許可リストで管理して認める。`@my/design-system` が依存してよいのは次のものに限り、許可リストは lint とパッケージの依存で管理する。

- React、React Aria Components / React Aria
- Tailwind CSS と `tailwind-variants`
- ドメインを持たない振る舞いのライブラリ: 現在は `@tanstack/react-virtual`
- アイコン: `react-icons`（Phosphor の `react-icons/pi` だけ、`Icons.tsx` からだけ。§6）

許可リストに追加できるのは、次をすべて満たすものだけとする。追加は PR の説明で理由を示し、この節の一覧を更新する。

- アプリのドメイン（API・ルーティング・サーバー状態・i18n のカタログ・認証）に依存しない
- Web と Tauri の WebView の両方で動く
- AGPL-3.0 と両立するライセンスで、公開から 3 日以上たった版を固定する（ADR-0011、§1）

### 6. アイコン

`@my/design-system` のアイコンは react-icons の Phosphor（`react-icons/pi`）から使う。`Icons.tsx` は「意味の名前から Phosphor のアイコンを引く薄い層」にする（web-client [§16.3](../architecture/web-client/16-look-states-and-wording.md#163-アイコン) の G10〜G13）。

- `Icons.tsx` は意味の名前（`ReplyIcon`・`StarIcon` など）の export を保ち、それぞれを Phosphor のコンポーネントの対応で定義する。
  画面の側の import は変わらず、セットをもう一度替えるときも `Icons.tsx` だけを直せばよい。
  対応は 1 か所の表（意味の名前 → オフの形・オンの形・向きの有無）にし、Storybook の一覧もこの表から作る。
- `react-icons` を import してよいのは `Icons.tsx` だけにし、ほかのセット（`react-icons/fa` など）を混ぜないように `react-icons/pi` 以外を lint で禁じる。
  上の層は `@my/design-system` の export だけを使う（G10）。
- 層はどのアイコンにも `aria-hidden="true"` と `focusable="false"` を付け（G12）、向きのあるもの（返信・戻る・外部へのリンク）に `rtl:-scale-x-100` を付ける（G13）。
  Phosphor の `mirrored` は書字方向を見ないので使わない。
- 大きさは置き場所が決める（G11）。react-icons は `width` / `height` に `1em` を書くが、`Button`・`ToggleButton`・`Link`・ナビゲーションの `[&>svg]:size-*` がそれより優先される。
  置き場所が大きさを決めない所でも、`0 × 0` ではなく文字の大きさで描かれる。
- 太さは Regular（24 px で線幅 1.5 px 相当）を基本にし、オンの状態は形が閉じるもの（星・ブックマーク・警告）を Fill、閉じないもの（ブースト）を Bold にする。
  Bold（2.25 px 相当）を基本にすると、ブーストのオン（Fill）とオフ（Bold）が形でほとんど区別できず、G13 を満たせない（#501 で両方のスクリーンショットを比べた）。
- `Foundations/Icons` の一覧は、表のすべての項目を、意味の名前・Phosphor の名前・オンの形・向きの有無と並べて出す。置き換えや追加のレビューでは、差分に出る Phosphor の名前と、この一覧を見ればよい。
- 見直す条件: react-icons の公開が長く止まり、Phosphor の新しい版が react-icons に入らなくなった場合、または必要な形が Phosphor にない場面が続いた場合は、公式パッケージ（`@phosphor-icons/react`）か Tabler（`@tabler/icons-react`）を見直す。

### 7. TypeScript の版

TypeScript はワークスペースのルートの 1 つ、TypeScript 7 にする。型検査も lint（tsgolint、§8）も TypeScript 7 で動く。

- TypeScript 6 のコンパイラ API が要るのは API の型の生成（openapi-typescript）だけなので、`@typescript/typescript6` の別名はそれを動かすパッケージ（ADR-0093）だけが持つ。ほかのパッケージは持たない。
- openapi-typescript が TypeScript 7 の API に対応したら、別名をやめる。

### 8. lint

lint は Oxlint だけで行い、ESLint とその実行は使わない。

- lint は Oxlint（`oxlint`、`oxlint-tsgolint`）で行う。設定はワークスペースのルートに 1 つ置き、パッケージは違う規則（§3 の向き、§4 の禁止）だけを上書きする。
  - typescript-eslint の型情報の規則は tsgolint で動かす（`options.typeAware`）。
    tsgolint はファイルごとに一番近い `tsconfig.json` を使うので、複数の設定を持つパッケージの `tsconfig.json` は `references` だけを持つ solution 形式にする。
  - `eslint-plugin-react-hooks` の規則は Oxlint のネイティブの `react/*` で動かす。
  - `@eslint-react`、TanStack Query / Router、Testing Library、boundaries、better-tailwindcss、formatjs、eslint-comments は、Oxlint の JS プラグイン（`jsPlugins`）で動かす。
    JS プラグインは alpha だが、決定者はこれを受け入れている。
  - `no-restricted-syntax` は Oxlint にないので、`restrictions.js` のセレクタを受け取って報告する 1 ファイルの JS プラグインをリポジトリに置く。
    `restrictions.test.js` は Oxlint の RuleTester（`oxlint/plugins-dev`）で、各禁止規則が違反を拒むことを確かめる。
- R14（数値を `&&` の条件にしない）は、型情報が要る `@eslint-react/no-leaked-conditional-rendering` が Oxlint では動かないので、lint では検査せず、レビューで確かめる。決定者はこれを受け入れている。
  `@eslint-react/no-unused-props` と `@typescript-eslint/no-unsafe-enum-assignment` も検査から外れる。
- web-client の「強制」の列の規則名は、Oxlint で動く名前（`react-hooks/purity` は `react/purity` など）に合わせる。
- 無効化コメントは `eslint-disable` の形のまま使う。R9 の `no-restricted-disable` には、`react-hooks/*` に加えて Oxlint の `react/` の名前（`react/exhaustive-deps` など）を挙げる。
- lint のタスクは Oxlint・`restrictions.test.js`・knip・CSS の `100vh` の検査を動かす。
- 依存から ESLint、typescript-eslint、`eslint-plugin-react-hooks` を外す。JS プラグインとして使うプラグインは残す。
  knip は `.oxlintrc.json` の `jsPlugins` と `settings` を読んで、JS プラグインと boundaries の resolver を使われている依存として数える。
- Oxlint の版を上げるたびに、違反のフィクスチャで「強制」の列の各行が検出されることを確かめる。

### 9. React Compiler の実装

React Compiler は `oxc-transform-react` で動かし、React のパッケージの直接の依存から Babel を外す。

- `@my/app` は、`oxc-transform-react` を呼ぶ 1 ファイルの Vite プラグインを置く。
  - client の環境のファイルだけを変換し（web-client の T6）、plugin-react と同じコードのフィルタを使う。`node_modules` は変換しない。
  - `jsx: "preserve"` で TypeScript だけを除いてコンパイラを通し、JSX と React Refresh は Vite 自身の Oxc の変換に任せる。
  - web-client の R4 のゲートは同じプラグインが持つ。`reportDiagnostics` の診断が 1 つでもあれば、場所と理由を挙げて `vite build` を失敗させる。
    `"use no memo"` / `"use no forget"` で始まる関数の中の診断は除く。関数の範囲は Vite が出す `parseSync`（Rolldown の Oxc のパーサ）で求め、
    診断の UTF-8 のバイト位置を UTF-16 の位置に直して比べる。出力の `_c(N)` が 1 つもなければ失敗させる。
  - このゲートは診断のすべてを失敗にするので、位置のない検証を数えなかった Babel 版より厳しい。決定者はこの厳しさを受け入れている。
- ほかの React のパッケージ（Storybook と Vitest）は、`@vitejs/plugin-react` の `compiler` オプションで同じ `oxc-transform-react` を使う。ゲートは `@my/app` のビルドが持つ。
- 0.x の experimental な `oxc-transform-react` を本番のビルドに使う。決定者はこれを受け入れている。
- 版は、すべてのパッケージで同じ `oxc-transform-react` の版に固定する。`@vitejs/plugin-react` の peer の範囲の外の版も使う。
- 版を上げるときは、`@my/app` のビルドが出すメモ化した関数の数と、検査と E2E の結果を PR に書く。
  数が変わったら、変わったファイルの出力を読んで確かめる。
- Oxc だけの誤変換の issue に当たる書き方をコードで使うときは、その issue を Oxc で直した版にしてから使う。
- 次のどれかが起きたら、Babel 版に戻す決定をする。
  - Oxc だけの誤変換が本番のコードに当たり、直った版が 3 日の待ちを含めて 4 週間以内に使えない
  - `oxc-transform-react` の公開が 8 週間止まる、または Oxc が React Compiler の移植をやめる
  - `@vitejs/plugin-react` が `compiler` オプションを外し、今の Vite に合う版の `oxc-transform-react` がなくなる

### 10. ツールの実行環境

ワークスペースのツールは Bun だけで動かし、Node.js を要する道具・手順・イメージを持たない（決定者、2026-11-12）。
実行環境・lockfile・版を固定して追従するものが 1 つで済み、2 つ目の toolchain を入れることも直すこともない。

- 子のプロセスは `process.execPath` で起こす。CI のイメージはブラウザとその共有ライブラリのために使い、中の Node.js と npm を使わない。
- 公開する npm パッケージを Node.js 22 と Deno で動かす試験（ADR-0093 §9）は利用者の環境を試すもので、対象外とする。
- npm パッケージを公開する job だけは Node.js と npm を入れる。trusted publishing の公式の道具で公開の信頼性を保ち、公開するパッケージはもともと Node.js で動く。

| ツール | 動かし方 |
| --- | --- |
| Vite・Vitest（ブラウザモードを含む）・Storybook・Playwright・Astro・openapi-typescript・tsc・Oxlint・knip・Lingui CLI | `bun --bun <bin>` |
| Redocly CLI・markdownlint-cli2・Wrangler | `bunx --bun` |
| 整形・テスト・i18n のツール・リポジトリのスクリプト | `bun fmt`・`bun test`・`bun <file>` |
| Claude Code | ネイティブのインストーラ |

## 経緯

Web クライアント（PWA、ADR-0028）は、オーナーの指定で React と TailwindCSS をベースにする。React は、ネット上のコードや一般的な知識に古い・誤った書き方（Effect の濫用、手動メモ化、サーバーデータのコピーなど）が多いので、react.dev などの一次情報に基づいて周辺ライブラリと書き方を決めた（#84 の調査報告。版は 2026-10-16 時点）。判断基準は、Rules of React に沿うこと、WCAG 2.2 AA・RTL・タッチ操作を部品の土台で保証できること（ADR-0009、NFR-A11Y-1）、Go のサーバーの SPA であること（RSC・SSR なし、ADR-0028）、OpenAPI の型（ADR-0016）をそのまま使えること、ライセンス（ADR-0011）、Tauri 2 と UI を共有できることだった。

部品がアプリのドメインに依存すると Tauri と共有できず、依存を後から剥がすのは高くつくので、境界をレビューではなく構造と lint で保証する形を最初に選んだ（§2〜§4）。その後 `Feed` に仮想化が要り（#125）、依存の制限の目的はドメインを知らないことだと確かめて、許可リストに広げた（§5）。

手描きの SVG のアイコン（23 個、24 × 24、線幅 2）は、差分から形が分からずレビューが困難で、描き足す人ごとに形の一貫性も保てなかったので、決定者の指摘でアイコンセットに替えた（§6、#483 の調査報告）。

TypeScript は typescript-eslint の対応範囲に合わせて 6.0 系に固定していたが、決定者は古い版のままにしないことを求めた（#567）。TypeScript 7.0 は JavaScript から呼ぶコンパイラ API を持たないので、公式が案内する並用を採った（§7）。

ESLint は 1 回に約 50 秒かかり、CI の web ジョブで最も長い手順で、大半が typescript-eslint の型情報だった。決定者の求めで移行先を調べ（#572 の報告）、278 規則と独自の禁止規則のすべてを比べて Oxlint を採った。lint の手順は約 9 秒になった（§8）。

決定者は #2863 で、継ぎ足しで歪んだ構成をすべて作り直すと決めた。`app/` をルートにしながらその外の `pkg/*/ts` を抱え、i18n のツールが別の lockfile を持ち、TypeScript が 2 つ並んでいた。`@my/ui` はドメインの部品（フォローボタン、通話のドック）を汎用の部品と混ぜて持ち、レイアウトや部品を差し替えても全画面に届かなかった。遷移・スクロール・通話・プラットフォームの差の置き場もなく、`app/web` に集まっていた。名前は「どちらが上か」が読めることを、責任は「仕組みは下、方針は上」を基準に、決定者と詰めた（§2〜§4）。

`babel-plugin-react-compiler` は Babel 7 にしか対応せず、Babel 8 への更新（#837）を止めていた。決定者の提案で `oxc-transform-react` への移行を調べた（#845 の報告。0.153.0 で 636 ファイルを比べ、メモ化する関数は 782 で全ファイル同数、諦めた関数は 0。出力の差は 6 ファイルのリアクティブスコープの切り方だけで、`app/web` の `vite build` は 9.3〜11.3 s から約 1.9 s になった）。移行（#849）では、peer の範囲に入る 0.152.0 の出力が 0.153.0 と全ファイルで同じで、スクリーンショットと E2E も Babel 版と同じ結果だった。lint の React Compiler の規則も同じ `oxc_react_compiler` クレートを使うので（2026-10-21 に確認）、lint とビルドが同じコンパイラで諦める関数を判断する（§9）。

## 採らなかった案

- 部品の土台 Base UI / Radix / Ark UI / 自作: i18n（組み込み文言、RTL でのキー操作の反転）で React Aria Components に及ばない（#84）。
- React Router（v8 の library / data mode）: 型安全なルートが framework mode 限定で、SPA では得られない。
- React 19 の `use` + Suspense だけ: 依存が増えないが、キャッシュ・無効化・ページングがない。SWR は同じ調査（#84）で比べて採らなかった。
- `bun test`: Bun だけで済むが、React Compiler の変換と Storybook を組み合わせた実ブラウザの検査ができない。
- UI キットをまず `app/web/src/ui` に置いて後で切り出す、層を分けず機能ごとに部品を作る: 始めるのは軽いが、依存を後から剥がすのは高くつき、境界を構造で強制できない。
- UI キットと Web クライアントの 2 つのパッケージのまま: 移行が要らないが、ドメインの部品の置き場がなく、遷移や通話の仕組みがアプリに集まる。`ui` と `parts` のような名前: 短いが、どちらが上かが名前から読めない。
- 機能ごとにパッケージを分ける: 境界がパッケージで守られるが、設定が機能の数だけ増える。差し替えの単位はデザインシステムと部品なので、機能はディレクトリと lint で足りる。
- 上の層でも Tailwind のクラスを書ける: 速く書けるが、デザインシステムを差し替えても届かない場所が残る。
- `app/` をワークスペースのルートのまま: 移す手間がないが、その外のパッケージを `../` で抱え、設定が 2 段に割れたままになる。
- 仮想化を `app/web` の機能側で行う: UI キットの依存は増えないが、Tauri と同じ部品にできない。UI キット内で自前実装する: 依存は増えないが、振る舞いを再実装することになる。
- 公式の `@phosphor-icons/react`: 版は同じだが 1 つのコンポーネントが 6 種の形を持ち、23 個で 15.2 KB（react-icons は 4.4 KB、いずれも gzip）。
- Lucide: 3.7 KB と小さいが塗りの版がなく、`fill` を付けると警告の「!」や繰り返しの矢印が潰れる。Tabler: 2.4 KB と最小だが塗りの版が一部だけで、ブーストのオンの形がない。
- Remix: Line / Fill があり 4.1 KB だが、4.9.0 から独自のライセンスで、アイコンだけの配布を禁じる条件が AGPL-3.0 第 10 条の「それ以上の制限を課さない」と両立せず、`scripts/task.sh web:licenses` の許可リストに入らない。Apache-2.0 の古い版に固定すると更新を受けられない。
- 手描きの SVG を続ける: バンドルは 1.3 KB で最小だが、レビューが困難なままである。
- `typescript` を全部 7 に上げる: 単純だが openapi-typescript などが動かず、検査を外すことになる。TypeScript 7.1 と各ツールの対応を待つ: 別名が要らないが、時期が決まっておらず古い版が残り続ける。
- ESLint のまま `--concurrency auto`: 全規則が残るが、4 CPU の CI で 45.0 s と速くならない（Oxlint 8.6 s）。
- Oxlint と型情報の規則だけの ESLint の併用: R14 を保てる（9.6 s）が、ESLint と typescript-eslint を R14 のためだけに残す。
- Biome: 最速（4 CPU で 2.8 s）だが、境界・i18n・Tailwind・Testing Library・TanStack の検査を失い、型の規則は今のコードに 74 件の偽陽性を出した。Biome と ESLint の併用: 全規則が残るが、型情報の規則が残って速くならない（1.4 s ＋ 49.7 s）。
- Babel 7 のまま: 今いちばん枯れていて React の e2e の対応も最も多いが、Babel 7 に縛られ、ビルドが約 5.5 倍遅い。
- `@vitejs/plugin-react` の `compiler` オプションだけ: 設定が最も短いが、診断を警告として出すだけで構造のある形で受け取れず、R4 のゲートを作れない。ゲートが要らないパッケージには足りる。
- ツールを Node.js で動かす: Vite・Storybook・Playwright が公式に対応する実行環境だが、実行環境と toolchain が 2 つになり、それぞれを固定して追従する。

## Consequences（影響）

- 部品がドメインを知らないことが構造で保証され、デザインシステムは Tauri と共有できる。デザインシステムを替えれば、Go が描画していた画面（ADR-0090 §16、ADR-0074 §11）を含むすべての画面に届く。ワークスペースの管理（パッケージ間の型参照、Tailwind の `@source` 設定）は増える。
- デザインシステムに variant がない見た目は、上の層では作れない。作る前にデザインシステムに足す。
- JS の実行環境は Bun の 1 つになる。Storybook・Playwright などは Bun を公式に対応しないので、版を上げるたびに Bun で動くことを検査で確かめる。
- 依存が増えるたびに、§5 の一覧と lint の許可リストを更新する手間がかかる。
- アイコンの形のレビューは、パスの数字ではなく Phosphor の名前と一覧で行える。新しいアイコンは描かずに表に 1 行足すだけになる。線が 2 px から 1.5 px 相当に細くなり、23 個で約 3 KB（gzip）大きくなる。
- react-icons は 30 以上のセットを含み、展開後に約 88 MB ある。インストールの時間と `node_modules` の大きさが増える（バンドルには選んだアイコンしか入らない）。各セットの更新の取り込みが遅れることがある。
- 型検査は TypeScript 7 の速いコンパイラと厳しい判定で行う。TypeScript 6.0 は API の型生成のパッケージにだけ残る。
- 「強制」の列の大半（境界・i18n・Tailwind・Testing Library・TanStack）が、alpha の Oxlint の JS プラグインで動く。
- React Compiler の lint の規則は Oxlint の移植で動く。移植は `"use no memo"` の関数の中でも `react/incompatible-library` を報告するなど、細部が公式のプラグインと違う。
- 0.x の experimental な変換を本番のビルドに使う。0.x はマイナー版でオプションが変わり得る。React の e2e では Babel 版より失敗が多く（2026-10-12 の対応表で Babel 1792/1802、OXC 1704/1795）、Oxc だけの誤変換の issue が開いている（oxc-project/oxc の #26161、#27402、#27429、#27401、#27113）。版を上げるたびに出力の確認が要る。
- Rust の panic（oxc-project/oxc の #27404）はビルドのエラーではなく、プロセスの異常終了になる。ビルドは失敗するが、原因のファイルを示さない。
- Babel は推移的な依存として `bun.lock` に残る。`@formatjs/cli-lib`、`magicast`、`@babel/code-frame` を使うライブラリなどが持ち、版はそれぞれのライブラリが決める。
- Oxc はコンパイルした関数の中のコメントを出力から落とす。開発サーバーの出力が読みにくくなるが、ソースマップは残る。`"use no memo"` の関数の診断を除くために、ゲートが自前でソースを解析する。
- `vite build` は速くなり、諦めた関数の報告が、関数の先頭ではなく原因の式の場所を指す。

## Open Questions / 要検証

- TypeScript 7.1 の API が出たときに、openapi-typescript が対応する時期
- jsdom のテストの MSW は Bun の `fetch` を横取りしない: `msw/experimental` の fetch の interceptor で受ける。だめなら Bun で動く別のライブラリを探す。
- Oxlint の RuleTester は Bun を拒む: 独自の規則を Bun で試せる別の lint か、禁止規則を Oxlint の CLI の出力で確かめるかを比べる。
- Wrangler は Bun を対応外とする: `bunx --bun wrangler deploy` で配置できるか。
