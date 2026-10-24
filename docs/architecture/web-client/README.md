# Web クライアントの設計規約

Web クライアント（`app/web`）と共有 UI キット（`app/ui`）を書くときの規約です。
根拠は [ADR-0025](../../adr/0025-web-client-stack.md)（技術スタック）、[ADR-0026](../../adr/0026-client-workspace-and-component-architecture.md)（構成と責務分離）、[ADR-0027](../../adr/0027-ui-quality-gates.md)（品質ゲート）、[ADR-0028](../../adr/0028-html-shell-link-previews-and-pwa.md)（HTML の外枠と PWA）です。
主な出典は react.dev、WAI-ARIA Authoring Practices、WCAG 2.2、Tailwind CSS・TanStack・Testing Library の公式ドキュメントです。

「強制」の列は、lint・型検査・テストで機械的に検査するものを示します。「レビュー」はレビューで確認します。
lint は Oxlint で、`scripts/task.sh web:lint`（`web:check` と `check` の一部）で実行します（[ADR-0049](../../adr/0049-client-lint-oxlint.md)）。
設定は `app/.oxlintrc.json` です。型情報の規則（`typescript/*`）は tsgolint が、React Compiler とフックの規則（`react/*`）は Oxlint が動かし、
ほかのプラグインの規則は Oxlint の JS プラグインとして動きます。独自の制限は、`app/.oxlintrc.json` がセレクタを渡す JS プラグイン `app/oxlint/restrictions.js` の規則
（`restrict/syntax`・`restrict/svg`・`restrict/feature`・`restrict/test`）と、Oxlint の `no-restricted-imports` などの規則の設定です。
`app/oxlint/restrictions.test.js` が各制限の違反例と許可例を、`app/oxlint/violations.test.js` が下の表の lint の行ごとの違反を Oxlint が報告することを検査します。
Oxlint は警告でも失敗します（`options.denyWarnings`）。
ルールを無効化するコメント（`eslint-disable-next-line`）には理由（`-- 理由`）が必要です（`@eslint-community/eslint-comments/require-description`）。

## 節の一覧

- [1. React の基本](#1-react-の基本)
- [2. 状態とデータ](#2-状態とデータ)
- [3. 構成と責務分離](#3-構成と責務分離)
- [4. エラー・読み込み・遷移](#4-エラー読み込み遷移)
- [5. アクセシビリティ](#5-アクセシビリティ)
- [6. レスポンシブとスタイル](#6-レスポンシブとスタイル)
- [7. テスト](#7-テスト)
- [8. サーバーが描画する画面](08-server-rendered-pages.md)
- [9. アプリの外枠](09-app-shell.md)
- [10. API クライアントと認証](10-api-client-and-auth.md)
- [11. PWA](11-pwa.md)
- [12. 投稿フォーム](12-compose.md)
- [13. 投稿の詳細と投稿への操作](13-status-detail-and-actions.md)
- [14. 管理画面](14-admin.md)
- [15. 検索とほかのサーバーのアカウントのフォロー](15-search-and-remote-follow.md)
- [16. 見た目・状態・文言](16-look-states-and-wording.md)
- [17. 通報](17-reports.md)
- [18. アカウントの状態と異議申し立て](18-account-status-and-appeals.md)
- [19. インポートとエクスポート](19-import-and-export.md)

## 1. React の基本

| # | 規約 | 強制 |
| --- | --- | --- |
| R1 | コンポーネントとフックは純粋にする。レンダリング中に副作用・変更・`Date.now()` / `Math.random()` を書かない | `react/purity`、`react/globals` |
| R2 | props・state・フックに渡した値・JSX に渡した値を変更しない（`push` / `sort` ではなく `toSorted` / スプレッド） | `react/immutability`（代入を検出する。`push` / `sort` などのメソッド呼び出しはレビュー） |
| R3 | フックはコンポーネントか `use*` 関数のトップレベルでだけ呼ぶ | `react/rules-of-hooks` |
| R4 | React Compiler を全コードに適用する。コンパイラが最適化を諦めるコードは lint とビルドで失敗させる。コンパイラから外す（`"use no memo"`）のは、コンパイルできないライブラリを包む 1 つのフックだけにし、理由を書く（`@my/ui` の `Feed` と `EmojiPicker` が TanStack Virtual の仮想化を読む、それぞれ 1 つのフック） | Oxlint の React Compiler の規則（`react/*`。`react/incompatible-library`・`react/unsupported-syntax` も error）、`vite build`（`app/web/vite-plugins/react-compiler.ts`。React Compiler の Oxc の移植 `oxc-transform-react` が診断を 1 つでも出すと、場所と理由を出して失敗する。`"use no memo"` で外した関数の中の診断は除く。1 つも関数を変換しなくても失敗。ADR-0065）、`restrict/syntax`（`"use no memo"` / `"use no forget"`。必要なときは理由付きの無効化コメント。API クライアントのフック `api.query.useQuery` / `api.query.useMutation` の呼び出し、§10） |
| R5 | `useMemo` / `useCallback` / `memo` は書かない。必要なとき（Effect の依存に安定した参照が要るなど）は理由をコメントに書く | `no-restricted-imports`・`restrict/syntax`（`useMemo` / `useCallback` / `memo` の使用。必要なときは理由付きの無効化コメント）、`react/use-memo` |
| R6 | 他の値から計算できるものは state にせず、レンダリング中に計算する | `react/set-state-in-effect`、`react/no-deriving-state-in-effects` |
| R7 | 利用者の操作で起きる処理（送信・通知・計測）はイベントハンドラに書く。Effect に書かない | レビュー |
| R8 | Effect は外部のシステム（SSE、ブラウザ API、React 以外の部品）と同期するときだけ使い、必ず後始末を返す | レビュー |
| R9 | Effect の中で反応させたくない値は `useEffectEvent` で読む。`exhaustive-deps` を無効化しない | `react/exhaustive-deps` を error、`@eslint-community/eslint-comments/no-restricted-disable`（`react-hooks/*` と、Oxlint の `react/` の React Compiler とフックの規則の無効化を禁止。`Feed` と `EmojiPicker` の仮想化のフックで Oxlint が `"use no memo"` の中でも報告する `react/incompatible-library` だけは除く） |
| R10 | ID が変わったら state を初期化したい場合は `key` を使う。Effect で初期化しない | レビュー |
| R11 | コンポーネントやフックを、ほかのコンポーネントの中で定義しない | `react/static-components`、`@eslint-react/no-nested-component-definitions` |
| R12 | リストの `key` には安定した ID を使う。動的なリストに配列の添字を使わない | `@eslint-react/no-array-index-key`、`@eslint-react/no-missing-key` |
| R13 | `forwardRef`・`<Ctx.Provider>`・関数コンポーネントの `defaultProps` を使わない（`ref` は普通の props、`<Ctx value>`、引数の既定値） | `@eslint-react/no-forward-ref`、`@eslint-react/no-context-provider`、`restrict/syntax`（`defaultProps`） |
| R14 | 数値を `&&` の条件にしない（`{count && <X />}` は `0` を表示する）。`count > 0 &&` と書く | レビュー |

## 2. 状態とデータ

| # | 規約 | 強制 |
| --- | --- | --- |
| S1 | サーバーのデータは TanStack Query だけで扱う。取得は `features/<x>/api` の `queryOptions`、書き込みは `useMutation` で行う。成功時に、応答で確かに分かる変化（投稿・閲覧者・関係・設定）は `setQueryData` / `updateCachedStatuses` でキャッシュに書き、並びや件数が変わり得る一覧・件数には `invalidateQueries`（既定の `refetchType: "active"`）で無効化の印を付ける。表示していないクエリは、次にその画面を開いたときに loader が取り直して待つ（S7）ので、取り直さない。書き込みの成功表示は、表示中のクエリの取り直しを待ってから出す（`await invalidateQueries(...)`）。書き込みが変えるクエリは、その書き込みの関数の隣（`features/<x>/api` のキャッシュ更新関数。例: `cachePostedStatus`）に書き、部品の `onSuccess` はそれを呼ぶ。データの種類ごとの方針は [ADR-0045](../../adr/0045-client-server-cache-policy.md) | `features/*/api` と `lib/` の外で `no-restricted-imports`（`openapi-fetch`・`openapi-react-query`・`lib/api`）・`no-restricted-globals` / `no-restricted-properties`（`fetch`）、`features/*/hooks` と `features/*/components` で `restrict/feature`（キャッシュを書き換える `QueryClient` のメソッド `setQueryData`・`invalidateQueries` などの呼び出し）、`@tanstack/eslint-plugin-query` の `flat/recommended-strict`（`@tanstack/query/exhaustive-deps`・`@tanstack/query/prefer-query-options` など） |
| S2 | サーバーのデータを `useState`・Context・グローバルストアにコピーしない | レビュー |
| S3 | 共有・リロード・戻る / 進むで保ちたい状態（タイムラインの種類・タブ・検索語・開いているメディア）は URL に置き、Zod で検証する | TanStack Router の型 |
| S4 | 状態は使う場所の近くに置き、必要なときだけ最も近い共通の親に持ち上げる | レビュー |
| S5 | 状態は判別可能なユニオンで表す（`{ status: 'idle' } \| { status: 'error'; error: Problem }`）。矛盾し得る真偽値を並べない | レビュー |
| S6 | Context は、変化の少ない値（閲覧者・ロケール・テーマ・複合部品の内部）にだけ使い、更新頻度ごとに分ける | レビュー |
| S7 | ルートは薄くする。パラメータと検索条件を取り出し、loader で、キャッシュにないか無効化されていれば取得して待ち（`queryClient.query({ ...options, staleTime: Infinity })`。無限スクロールは `queryClient.infiniteQuery`。無効化の印を見ない `staleTime: "static"` と、非推奨の `ensureQueryData` は使わない）、それから機能の部品を表示する。ルーターは `defaultStaleReloadMode: "blocking"` で、前に訪れたルートでも loader の終わりを待って描画する（ADR-0045） | `@tanstack/eslint-plugin-router` の `flat/recommended`（`@tanstack/router/create-route-property-order` など）、`restrict/syntax`（`staleTime: "static"`・`ensureQueryData`）、レビュー |
| S8 | 楽観的更新は、まず保留中の mutation の `variables` を表示する方式にする。複数の画面に反映が要るときだけ、キャッシュを書き換えて失敗時に戻す | レビュー |
| S9 | API の型は OpenAPI からの生成物だけを使う。同じ型を手で書かない | 生成物の差分検査（ADR-0029） |
| S10 | SSE の購読は `lib/sse` の 1 つの Effect だけが行い、受け取った内容を Query のキャッシュに書き込む | レビュー |
| S11 | ブラウザの保存（localStorage・sessionStorage）は `lib/storage` で宣言したキーだけを通して読み書きする。新しく残すものは、そこにキーを宣言してから使う（下の「ブラウザの保存」） | `lib/storage` とテストの外で `restrict/storage`（`localStorage`・`sessionStorage` の参照） |

### ブラウザの保存

`lib/storage.ts` が、アプリがブラウザに残すものをすべて宣言する。キーごとに、置き場所（localStorage か sessionStorage）・値の型とその検査（Zod）・保存形式の版・何も残っていないときの既定値・利用者のデータか（セッションが終わると消すか）を決める。
宣言したキーのオブジェクトの `read()` / `write()` / `remove()` で読み書きし、React からは `useStorage(key)`（`useSyncExternalStore`）で読む。
値は `{"v":<版>,"value":<値>}` の JSON で残す。JSON でないもの・版の違うもの・検査を通らないものは、何もないのと同じく既定値として読む。既定値を書くとキーを消す。
同じタブでの書き込みはそのキーの購読者に、ほかのタブでの書き込みは `storage` イベントで伝わる。ストレージが使えない（ブロックされている、容量が足りない）ときは、書いた値をそのページの間だけメモリに置き、ページの動きは変えない。
セッションが終わると、`clearUserStorage()` が利用者のデータと宣言したキーを両方のストレージから消す（§10）。

| キー | 置き場所 | 内容 | 利用者のデータ |
| --- | --- | --- | --- |
| `my:display:theme` | localStorage | 表示のテーマ（§9）。`public/theme.js` も同じキーと形式を読む | いいえ |
| `my:display:locale` | localStorage | 最後に表示したロケール（§9） | いいえ |
| `my:display:autoplay` | localStorage | アニメーションの自動再生（§13） | いいえ |
| `my:display:navigation` | localStorage | ナビゲーションの項目の順番と隠した項目（§9） | いいえ |
| `my:instance:timelines` | localStorage | インスタンスが提供する公開タイムライン（§9） | いいえ |
| `my:emoji:recent` | localStorage | よく使う絵文字（Unicode 絵文字の基の列とカスタム絵文字の `:shortcode:`）と、減衰させた回数・最後に使った時刻（§12、ADR-0091 §3） | いいえ |
| `my:emoji:skin-tone` | localStorage | 絵文字のピッカーで選んだ肌の色（§12、ADR-0091 §3） | いいえ |
| `my:compose:recent-tags` | localStorage | この端末で送った投稿で最近使ったハッシュタグ（16 個まで、新しい順。§12、ADR-0092 §1） | はい |
| `my:user:compose:<アカウント ID>` | sessionStorage | 投稿の下書きと送信中の投稿（§12） | はい |
| `my:user:home-timeline` | localStorage | オフラインで読むホームのタイムラインの新しい投稿（フォロワー限定・ダイレクトを除く）と、それを読み込んだ時刻（§11）。利用者が変わったときも消す | はい |
| `my:oauth-pending` | sessionStorage | 進行中のサインインの verifier・`state`・戻り先（§10） | いいえ |
| `my:preload-error-reloaded-at` | sessionStorage | チャンクの読み込みに失敗して再読み込みした時刻（§11） | いいえ |

## 3. 構成と責務分離

| # | 規約 | 強制 |
| --- | --- | --- |
| A1 | 依存の向きは `routes → features → (@my/ui, lib)` だけにする | `boundaries/dependencies`（要素は `shell`（`web/src` 直下）・`route`・`feature`・`lib`・`ui`、テストだけが import できる `test`（`web/src/test`）と `feature-test`（`web/src/features/*/test`））、`boundaries/no-unknown-files` |
| A2 | `@my/ui` は React・React Aria Components・Tailwind のユーティリティと、ドメインを持たない振る舞いのライブラリ（許可リストは ADR-0031）にだけ依存する。API・Query・Router・機能・i18n のカタログを import しない（文言は props / children で受け取る） | `boundaries/dependencies`（`ui` が import できるのは `ui` 自身と React・React Aria・`@tanstack/react-virtual`・`tailwind-variants`・`tailwind-merge`、`Icons.tsx` だけは `react-icons` だけ）、パッケージの依存関係（Bun の isolated linker） |
| A3 | 機能は、ほかの機能を `index.ts` 経由でだけ使う | `boundaries/dependencies`（ほかの `feature` は `index.ts` だけ） |
| A4 | 機能のロジックは `features/<x>/hooks` のカスタムフックに置く。フックは JSX を返さない。機能の部品はフックの戻り値を `@my/ui` に渡すだけにする。`app/web/src`（外枠・`routes`・`features`・`lib`・テストを含む）は Tailwind のクラスを書かない: JSX の `className`（`@my/ui` の部品に渡すものを含む）と、`tailwind-variants`・`tailwind-merge`・`clsx` などクラスを組み立てる手段の import を使わない。見た目とレイアウト（3 列の外枠を含む）は `@my/ui` の部品（`Stack`・`Cluster`・`Grid`・`Box`・`Text`・`Heading` など）と、その見た目の変種を表す props で書く。props にない見た目は `@my/ui` に props か部品を足す。例外は、`@my/ui` の部品で表せないブラウザやプラットフォーム固有の調整だけで、その行に理由を書いた無効化コメントを付ける | `restrict/classes`（`className` の属性とプロパティ、クラスを組み立てるライブラリの import。`web/src` の全ファイルに掛かり、テストも除かない）、レビュー |
| A5 | 操作できる要素は `@my/ui`（React Aria Components を包んだ部品）から使う。`onClick` を付けた `div` / `span` を書かない | `restrict/syntax`（`div` / `span` の `onClick`・`onKeyDown`・`onPointerDown` など） |
| A6 | `@my/ui` の部品は表示に専念し、状態はできるだけ外から渡す（`value` / `onChange`）。見た目の違いは型付きの props で受け取る。レイアウトの部品（`Stack`・`Cluster`・`Grid`・`Box` など）は `className` と `style` を受け取らない。React Aria Components を包んだ部品が `className` を受け取るのは、その props の型を引き継ぐためで、使えるのは `@my/ui` の中だけである（A4） | レビュー、型検査（レイアウトの部品）、`restrict/classes`（`web/src` から渡さない） |
| A7 | props は型を明示する（`React.FC` を使わない）。ネイティブ要素の props は `ComponentPropsWithRef<'button'>` で拡張する。多態的な `as` props を使わない | `typescript/no-restricted-types`（`FC` / `FunctionComponent`）、レビュー |
| A8 | `dangerouslySetInnerHTML` は、サーバーでサニタイズ済みの投稿本文を表示する 1 つの監査済み部品（`PostContent`）でだけ使う。`PostContent` は本文を `SanitizedHtml` 型で受け取る。この型は `serverSanitizedHtml()` でだけ作り、API の応答でサーバーがサニタイズ済みと定める項目を読む場所でだけ呼ぶ。サニタイズ済みの HTML から文字だけを取り出すときは `@my/ui` の `sanitizedHtmlText` を使う。HTML を解析・挿入する Trusted Types の sink（`DOMParser`・`innerHTML` / `outerHTML` への代入・`insertAdjacentHTML`・`createContextualFragment`・`srcdoc`・`document.write`・`<script>` の生成・`eval` / `new Function`）は `PostContent.tsx` の外では使わない | `@eslint-react/dom-no-dangerously-set-innerhtml` と `restrict/html`（`app/ui/src/PostContent.tsx` とテストだけ許可）、型検査（ただの `string` は `SanitizedHtml` にならない）、レビュー |
| A9 | 利用者に見える文字列はすべて `react-intl` を通す。JSX に文字列リテラルを書かない | `formatjs/no-literal-string-in-jsx`（子要素と `aria-*`・`label`・`placeholder`・`title`・`alt`。テストとストーリーは除く） |

## 4. エラー・読み込み・遷移

| # | 規約 | 強制 |
| --- | --- | --- |
| E1 | ルートごとに `errorComponent` を持ち、ルートにも 1 つエラー境界を置く。独立して失敗し得る区画（埋め込みのカードなど）には `react-error-boundary` を置く | レビュー |
| E2 | mutation の失敗はエラー境界ではなく、mutation の状態と通知で扱う。RFC 9457 の `type` を、翻訳済みの文言に対応づける | レビュー |
| E3 | Suspense の境界は、スケルトンを見せる意味がある区画にだけ置く | レビュー |
| E4 | 画面遷移とタブの切り替えはトランジション（`startTransition`）にし、表示中の画面を保つ | レビュー |

## 5. アクセシビリティ

| # | 規約 | 強制 |
| --- | --- | --- |
| Y1 | すべてのストーリーと E2E の画面で、axe の違反（`wcag2a` / `wcag2aa` / `wcag21a` / `wcag21aa` / `wcag22aa`）がない | Storybook（`a11y.test: 'error'`）、Playwright |
| Y2 | 操作できる要素はすべてロールとアクセシブルネームを持つ。テストは `getByRole` / `getByLabelText` で探し、テスト ID は最後の手段にする | `eslint-plugin-testing-library` の `flat/react`、`testing-library/no-test-id-queries`（使うときは理由付きの無効化コメント） |
| Y3 | コントラストは AA（文字 4.5:1、UI 3:1）をライト / ダークの両方で満たす | `scripts/task.sh web:tokens`（トークンの組）、実ブラウザの axe |
| Y4 | フォーカスできる要素はすべて `focus-visible` のリングを持つ。固定ヘッダでフォーカスを隠さない（`scroll-padding`） | 見た目のスナップショット、レビュー |
| Y5 | ルートが変わったら、新しい画面の `h1`（`tabindex="-1"`）にフォーカスを移し、`document.title` を更新する。外枠が 1 か所で行う（§9） | Playwright、統合テスト |
| Y6 | タイムラインは feed パターンにする（`@my/ui` の `Feed`。`role="feed"`、`<article>`、`aria-posinset` / `aria-setsize`（総数が分からないときは `-1`）、`aria-busy` を必ず戻す、Page Down / Page Up で記事の間を移動し、Control + End / Control + Home で feed の外に出る）。仮想化しても、フォーカスのある記事は表示範囲の外でもマウントしておく | ARIA スナップショット、キーボードのテスト（`Feed` の部品のテストとストーリー） |
| Y7 | 新しい投稿は、利用者の設定 `timeline_auto_insert`（既定は偽）が真のときだけ自動で差し込む（[ADR-0077](../../adr/0077-timeline-auto-insert.md)）。偽のときと、フォーカスや選択がフィードの中にある間は差し込まず、「N 件の新しい投稿」を `@my/ui` の `NewItemsButton`（`role="status"` の中のボタン）で知らせ、押されたときだけ差し込む。自動で差し込むときは、読んでいる投稿の位置をスクロールの補正で保ち、先頭から離れている間だけ `NewItemsButton` を浮かせる。差し込んだ投稿を 1 件ずつ読ませず、件数は `NewItemsButton` だけで知らせる。それ以外の件数・完了・エラーは、共通の `role="status"` / `role="alert"` の通知部品（`@my/ui` の `AnnouncerProvider` / `useAnnouncer`、見える通知は `ToastProvider` / `useToast`、操作の結果を画面に残すときは `StatusMessage`）だけで知らせる。フォーカスを奪わない | ARIA スナップショット、部品のテスト（Announcer・NewItemsButton のテストとストーリー）、レビュー |
| Y8 | CW は disclosure パターン（`aria-expanded` のボタン、閉じた内容は `hidden`）にする | 部品のテスト |
| Y9 | 画像の添付には代替テキストの入力欄を必ず出す。サムネイルには「説明なし」のような欠落の印を出さず（代替テキストのない画像は支援技術にだけ「説明なし」という名前で読ませる）、代替テキストがある画像の右下（書字方向の末尾側）に飾りの小さなノートのアイコンのバッジを付ける（`aria-hidden`） | 部品のテスト |
| Y10 | 利用者が書いた内容（表示名・投稿・プロフィール）と管理者が書いた内容（インスタンスの説明・ルール・規約）は `dir="auto"` で表示し、分かる場合は `lang` を付ける。言語を記録しない管理者の文（どのロケールでも同じ 1 つの値）は `lang=""`（言語不明）にして、画面の言語を引き継がせない | 部品のテスト、ARIA スナップショット |
| Y11 | モーダルのダイアログはクリック・タップだけで閉じられる（APG の dialog）。`@my/ui` の `Dialog` / `Sheet` は `role="dialog"` なら × ボタン（`closeLabel`）を必ず持ち、`alertdialog` は × ボタンの代わりに「キャンセル」を持つ（`closeLabel={null}`）。背景（スクリム）を押すと閉じる（`isDismissable` の既定は真）。閉じると入力を失うフォームのダイアログ（通報・投稿の編集・管理画面の編集、入力欄を持つ `ConfirmDialog`）だけは `isDismissable={false}` にして、背景を押しても閉じない。閉じても下書きが残る投稿のシートは背景で閉じる | 型（`closeLabel` が必須）、部品のテストとストーリー |
| Y12 | アプリの外へ出るリンク（ほかのサイト、ソースコードの公開先、サーバーが描画した HTML（`PostContent`）の中の自インスタンスの画面以外へのリンク）は新しいタブで開く。`@my/ui` の `ExternalLink` が `target="_blank"`、`rel` の `noopener noreferrer`（呼び出し側の `nofollow` などは保つ）、外部へのリンクのアイコン、支援技術に読ませる「（新しいタブで開きます）」（`client:shell.link.new_tab`、`sr-only`。`aria-label` があればその後ろに足す）を付ける。文言と自インスタンスの画面かどうかの判定は、外枠（`App.tsx` の `ExternalLinks`）が `ExternalLinkContext` で 1 度だけ渡す。`Link` や `<a>` に `target` を書かず、`http(s):` の `href` は `ExternalLink` に渡す。`mailto:` はメールのアプリが開くので `Link` のままにする | `restrict/syntax`（`a` / `Link` の `target` と、`http(s):` の文字列の `href`）、部品のテスト（`ExternalLink`・`PostContent`） |

## 6. レスポンシブとスタイル

| # | 規約 | 強制 |
| --- | --- | --- |
| D1 | モバイルファーストで書く（接頭辞なしが最小幅、`md:` は「md 以上」） | レビュー |
| D2 | 画面幅のブレークポイントはアプリの外枠（`@my/ui` の `AppShell.tsx`・`AppNavigation.tsx`）だけで使う。部品にはコンテナクエリ（`@container`、`@md:`）を使う。外枠のストーリーはストーリーのビューポートを変えて確かめる（`ui/src/storyViewports.ts`） | 外枠の 2 つを除く `ui/src` と `features` の `.tsx` で `better-tailwindcss/no-restricted-classes`（`sm:`〜`2xl:`・`max-*:`・`min-[…]:`） |
| D3 | 320 px のどのロケールでも横スクロールが出ない | Playwright（`scrollWidth <= clientWidth`） |
| D4 | タップ領域は 24 × 24 px 以上、主要な操作は `pointer-coarse:` で 44 px 以上にする | axe `target-size`、レビュー |
| D5 | `100vh` を使わない（`svh` / `dvh`）。端に固定する要素には `env(safe-area-inset-*)` を足す | `restrict/syntax`（文字列）、`better-tailwindcss/no-restricted-classes`（`h-screen` など）、`scripts/task.sh web:lint` の grep（CSS） |
| D6 | アニメーションは `motion-safe:` の下に置く | `scripts/task.sh web:tokens`、`better-tailwindcss/no-restricted-classes`（`animate-*`）、レビュー |
| D7 | 方向は論理プロパティのユーティリティで書く。`ml-` / `mr-` / `pl-` / `pr-` / `left-` / `right-` / `text-left` / `text-right` / `rounded-l` / `rounded-r` / `border-l` / `border-r` / `mask-l-` / `mask-r-` を使わない（方向のあるアイコンの反転と、論理方向の版がない `mask-l-` / `mask-r-`（端のフェード）だけ、`rtl:` / `ltr:` の下で許可） | `better-tailwindcss/no-restricted-classes` |
| D8 | 色は `@my/ui/theme.css` の意味のトークン（`--color-surface`、`--color-fg-muted` など）だけを使う。生のパレット（`--palette-*`）や任意の色を部品に書かない（`--color-*: initial` で Tailwind の既定の色を消す）。トークンの一覧・コントラストの組・ライト / ダークの切り替えは [app/ui/README.md](../../../app/ui/README.md) にある | Tailwind の設定、`scripts/task.sh web:tokens`、`better-tailwindcss/no-unknown-classes` |
| D9 | 部品の見た目の違いは `tailwind-variants` の 1 部品 1 レシピで定義する。クラス名を動的に組み立てない。`className` は `tailwind-merge` で root / スロットにだけ合成し、`web/src` からは渡さない（A4） | `better-tailwindcss/no-concatenated-classes`、`better-tailwindcss/no-conflicting-classes`、レビュー |
| D10 | CSP を壊さない。`setAttribute('style')` や `cssText` を使わない（React の `style={}` は可） | `restrict/syntax` |

## 7. テスト

| # | 規約 | 強制 |
| --- | --- | --- |
| T1 | 利用者の使い方に近い形でテストする。フックの内部や state を直接検査しない | レビュー |
| T2 | 操作は `user-event` で再現する。`screen` を使い、`waitFor` + `getBy` ではなく `findBy` を使う | `testing-library/prefer-user-event`、`testing-library/prefer-screen-queries`、`testing-library/prefer-find-by` |
| T3 | ネットワークは MSW だけでモックする。`fetch` や API のモジュールをモックしない | テストの `restrict/test`（`api` を含むモジュールと `openapi-fetch` の `vi.mock` / `vi.doMock`、`fetch` の `vi.spyOn` / `vi.stubGlobal` / 代入）、レビュー |
| T4 | 既定は統合テスト（本物の QueryClient とルーター + MSW）にする。純粋なロジックは単体テスト、主要な流れは 4 ロケールの E2E にする | レビュー |
| T5 | `@my/ui` の部品は必ずストーリーを持つ。ストーリーはアクセシビリティ・見た目・RTL のテストの入力になる。`web/src` に見た目を書かないので（A4）、画面が使う見た目はすべてここで確かめられる | レビュー、`knip`（使われないファイル・export・依存） |
| T6 | 部品のテストは jsdom かブラウザモードで動かす。React Compiler（`app/web` の `react-compiler.ts`、`app/ui` の `@vitejs/plugin-react` の `compiler` オプション）はクライアント向けの変換にだけ効くので、Vitest の `node` 環境では本番と違うコードをテストすることになる | Vitest の設定 |
| T7 | 非同期の結果は Testing Library の `findBy` / `waitFor` で待つ。`vi.waitFor` は使わない。待ち時間の上限はテストごとに変えず、setup の `asyncUtilTimeout` と Vitest の `testTimeout` にまとめる（待ちは実時間なので、負荷の高い機械では既定の 1 秒・5 秒を超える）。React の描画の外で動くフォーカス（React Aria のフォーカスの復帰は `requestAnimationFrame`、ルート変更の見出しへのフォーカスはルーターの `onRendered`）は `waitFor` の中で `toHaveFocus()` を検証する。遷移の後は遷移先にしかないもの（タブなら名前で選んだ `tabpanel` とその中身）を待つ。読み込みの間は前のページが残り、`defaultPendingMs` を超えると pending component に置き換わるので、どのページにもある要素や「ないこと」の確認は遷移の前の画面でも通ってしまう | テストの `restrict/test`（`vi.waitFor`）、Vitest の設定、レビュー |

### テストの置き場所と道具

- 部品のテストは対象の隣に `*.test.ts(x)` で置き、`scripts/task.sh web:test` で動かす（`app/web/vitest.config.ts`、jsdom）。
  - `src/test/setup.ts` が jest-dom のマッチャーを読み込み、MSW のサーバーを起動する。どのハンドラにも合わない要求はテストを失敗させる。
    `findBy` / `waitFor` の上限（`asyncUtilTimeout`）は 10 秒、1 つのテストの上限（`vitest.config.ts` の `testTimeout`）は 30 秒にする（T7）。
    通るテストはこの上限まで待たない。上限は壊れたテストが止まるまでの時間だけを決める。
  - 既定のハンドラは `api/openapi.yaml` の操作ごとに 1 つ置き、その操作を使う機能が持つ。テストごとの差し替えは `server.use()` で行う。
    - 機能のハンドラは `features/<x>/test/handlers.ts` に置き、`handlers` として export する。その機能のテストだけが使うフィクスチャも同じファイルに置く。
    - 複数の機能が使うフィクスチャ（インスタンス・アカウント・投稿・関係・設定・メディア・タイムラインのページ）は `src/test/fixtures.ts` に 1 つずつ置き、機能のファイルで作り直さない。
      API の型は生成された型（`Schemas`）から取る（S9）。
    - ハンドラが共有する部品（`apiBase`・`problem()`・アクセストークンの検査 `withViewer()`・OAuth の応答・`signedInHandler`）と、インスタンスと OAuth のエンドポイントの既定のハンドラは `src/test/msw/http.ts`、
      `GET /streaming` は `src/test/msw/streaming.ts` に置く。
    - `src/test/msw/handlers.ts` はこれらのモジュールを 1 行ずつ `export * as` で束ねるだけにし、`src/test/msw/server.ts` がその全部の `handlers` で MSW のサーバーを作る。
      機能を足すときに共有ファイルで変えるのは、`handlers.ts` への 1 行だけである。
      1 つの操作に既定のハンドラは 1 つなので、束ねる順番は結果を変えない（`handlers.test.ts` が重複を検査する）。
    - `features/<x>/test` は `boundaries` の要素 `feature-test` で、`src/test` と同じ機能のテストからだけ import できる。`src/test` を import できる。
  - 部品は `src/test/render.tsx` の `renderWithProviders`（ランタイム設定と i18n のプロバイダ）で描画する。`user-event` は描画の前に `setup()` する。
  - 統合テストは同じファイルの `renderApp` でアプリ全体（本物のルーターをメモリ上の履歴で、新しい QueryClient）を描画する。`path` で開くアドレス、`locale` でロケールを指定する。
    サインインした状態は `server.use(signedInHandler)`（リフレッシュがハンドラの受け付けるアクセストークンを返す）で作る。
  - jsdom の `FormData` と `File` の本文は、MSW が相手にする Node.js の `fetch` で送れない。ファイルを送るテストは `src/test/multipart.ts` の `nodeUploadFile()` で
    Node.js の `File` を作り、そのテストの間だけ `FormData` を Node.js のものにする。
  - `@my/ui` の部品のテストも `app/ui/src` の部品の隣に置き、`scripts/task.sh ui:unit-test` で動かす（`app/ui/vitest.config.ts` の `unit` プロジェクト、jsdom）。
    `app/ui/test/setup.ts` が jest-dom のマッチャーを読み込み、`app/web` と同じ待ち時間の上限を設定する。キットは通信しないので MSW もプロバイダもなく、Testing Library の `render` で描画する。
    構造は `app/ui/test/aria-snapshot.ts` の `ariaSnapshot()`（Playwright の ARIA スナップショットと同じ形の、ロール・名前・説明・状態の木）で検査する。
- E2E は `app/web/e2e/*.spec.ts` に置き、`scripts/task.sh web:e2e` で動かす（`app/web/playwright.config.ts`、ビルドした `vite preview` が相手）。
  - `vite preview` と `e2e/server.ts` は、実行ごとに選んだ空きポートで待ち受ける（`E2E_PREVIEW_PORT` / `E2E_SERVER_PORT` で固定できる）。
    この実行が起動していないサーバーは再利用しないので、同じマシンの複数の worktree で同時に動かしても互いのサーバーを相手にしない。
    起動済みのサーバーを相手にするときは `E2E_BASE_URL`（クライアント）と `E2E_SERVER_URL`（`my serve -pseudo-locales`）を指定する。
  - サーバーが描画する画面（§8）の E2E（`e2e/auth-pages.spec.ts`）と、本物の CSP の下で SPA がログインして API に届くことを確かめる E2E（`e2e/csp.spec.ts`、CSP の違反が 1 件でもあれば失敗）は、本物の `my serve` が相手になる。`e2e/server.ts` が一時ディレクトリの SQLite に `schema/sqlite/` のファイルを名前の順に適用し、
    管理者とサードパーティの OAuth クライアントを入れてから、`scripts/task.sh go:bin` で作った `bin/my` を起動する。ホストは `localhost`（`__Host-` の Cookie を HTTP で受け取れるのは `localhost` だけ）。
    `MY_WEB_ORIGIN` は `http://localhost:<port>` なので、外枠のランタイム設定の `apiOrigin` もそれになり、SPA は同じサーバーの API に HTTP で届く。
    疑似ロケールは利用者に出さないので、`my serve` は開発・テスト専用のフラグ `-pseudo-locales` を付けたときだけ `en-XA` / `ar-XB` を選べる。`e2e/server.ts` はこれを付けて起動し、各ページがプロジェクトのロケールの `lang` と向き（`ar-XB` は `rtl`）で描画されることを確かめる。
  - Playwright は Service Worker を止めて動かす（`serviceWorkers: 'block'`）。Service Worker が遷移に応えると、ロケールの fixture の `page.route()` を通らないため。
    Service Worker を検査する `e2e/pwa.spec.ts` だけが許可し、本物の `my serve` を相手に 1 つのプロジェクト（`en-1440`、Chromium）で動かす（§11）。
  - JavaScript を無効にしたテスト（`javaScriptEnabled: false`）で、ログイン・登録・認可同意のフォームがスクリプトなしで動くことを確かめる。axe はページの中でスクリプトを動かすので、その画面の axe は同じ Cookie の JavaScript ありのコンテキストで行う。
  - M2 の主要な流れ（`e2e/main-flow.spec.ts`）も本物の `my serve` が相手になる。2 人の利用者がそれぞれのブラウザコンテキストで、登録、サーバーの画面からのログイン（PKCE）、CW と代替テキスト付きの画像のある投稿、
    ホームとローカルのタイムライン、フォロー（フォロワーを承認する鍵アカウントへのリクエストと承認を含む）、フォロワー限定の投稿の見え方、通知までを通し、途中の画面ごとに axe・横スクロール・ARIA スナップショットを検査する。
    利用者は毎回ランダムな名前で作るので、再試行や他のプロジェクトの投稿・フォローと混ざらない。他のプロジェクトも投稿するローカルタイムラインでは、Page Down で feed を進めて自分の投稿を探す。
    `bin/my` のコマンド（`my admin create`、`my registration approve`）は、`e2e/server-cli.ts` が `e2e/server.ts` と同じ設定（実行ごとに選ぶ一時ディレクトリと鍵）でサーバーのデータベースに対して実行する。
    アプリは利用者の表示言語で表示されるので、ログインした利用者の表示言語をクライアント API（`PATCH /api/v1/preferences`）でプロジェクトのロケールにしてから進める。
  - サーバーのレート制限（登録は 1 つのアドレスから 30 分に 5 回、ログインの失敗は 1 分に 5 回）を使い切らないように、登録の画面からアカウントを作るテストは 1 つのプロジェクト（`en-320`）だけで動かす。
    主要な流れも `en-320` でだけ登録の画面から登録し、`my registration approve` で承認する。他のプロジェクトと 2 人目の利用者は `my admin create` で作る。
    エラーの表示は制限を数える前に断られる入力（期限切れのフォーム、形式の誤り）で確かめる。
  - axe は React Aria のライブアナウンサー（`[data-live-announcer]`）を検査しない（`e2e/fixtures.ts` の `axeBuilder`）。React Aria の Button は待機の始まりと終わりを、ボタンを `aria-labelledby` で指す要素で 7 秒間そこに知らせるので、
    その間に画面の移動でボタンが消えると、名前のない要素が残るため。
  - axe は、ページの終わりのあるアニメーションとトランジションがすべて終わってから検査を始める（`e2e/fixtures.ts` の `axeBuilder`。`document.getAnimations()` の `finished` を待つ）。
    種類の変わるボタン（「フォロー」のアクセントから「リクエスト中」の面へ）は `motion-safe:transition-colors` で色を移すので、途中で `color-contrast` が中間の文字色と背景色を読むと違反になるため。
    終わりのないアニメーション（スピナー、スケルトンの明滅）は待たない。
  - プロジェクトは画面幅 320 / 390 / 768 / 1024 / 1440 px × ロケール `en` / `ja` / `en-XA` / `ar-XB` の 20 個。768 px 未満はタッチと粗いポインタのスマートフォンとして動かす。
  - 遅い検査（リリースの関門・nightly・手動、[ADR-0079](../../adr/0079-ci-single-gate-merge-queue.md)）は全体（`E2E_MATRIX=full`）を動かす。PR と Merge Queue では動かさない。既定の部分集合（全ロケールの 320 / 1440 px）は手動だけで、Web に触れる PR では作者が手元で `scripts/task.sh web:e2e` を回すか、PR のブランチに対して `web-e2e.yml` を `workflow_dispatch` で手で回す。
  - CI（`web-e2e.yml`）は `app/web/dist` と `bin/my` を 1 回だけビルドし、全シャードが同じビルドを検査する（[ADR-0047](../../adr/0047-ci-protection-layers.md)）。シャードは部分集合が 4、全体が 8 で、各シャードの blob レポートを 1 つの HTML レポートにまとめる。`scripts/task.sh web:e2e` は手元で全部を 1 回で回す。
  - spec は `e2e/fixtures.ts` から `test` / `expect` を import する。`vite preview` は `index.html`（ランタイム設定の代わりを含む）をそのまま返すので、fixture がプロジェクトのロケールを `<html lang dir>` に書き込み、サーバーの外枠と同じ状態にする。ビルドのファイルへの相対のリンクは、`my serve` と同じくルートからのものにする（ルートより下の画面のため）。`vite preview` は CSP を付けないので、fixture は `my serve`（`e2e/server.ts`）の外枠が送る CSP を取得してそのまま付ける。spec も本番と同じ Trusted Types と取得元の制限の下で動き、CSP が拒む sink（`DOMParser` や `innerHTML` への文字列）はアプリと同じく spec を失敗させる。画像は自分のオリジンからしか読めないので、spec が作る画像は `data:` URL を `servedImage` に渡し、プレビューのオリジンで配る。ランタイム設定の `apiOrigin` もその回の `vite preview` のオリジンに書き換えるので、クライアントの API と OAuth の要求はプレビューに届き、spec が `page.route` で応答できる（ログイン済みの画面は、`/oauth/token/refresh` と API の操作に応答して作る）。axe は `makeAxeBuilder` と `expectNoAxeViolations`（規則と対象のセレクタの一覧が空であることを検査）、横スクロールは `expectNoHorizontalScroll` で検査する。
  - 構造は `toMatchAriaSnapshot` で検査する。ロケールで変わる名前は、スナップショットから省くか正規表現にする。
  - CI は、digest で固定した Playwright の公式イメージで動かす。イメージの版は `@playwright/test` の版と合わせる。
- `@my/ui` のストーリーは `app/ui/src/*.stories.tsx` に置く（CSF 3、`satisfies Meta`）。Storybook は `scripts/task.sh ui:storybook` で開く（`app/ui/.storybook`）。
  - ツールバーでロケール（`en` / `ja` / `en-XA` / `ar-XB`）とテーマ（ライト / ダーク）とサーバー単位のテーマ（既定 / 見本）を切り替える。ロケールは `<html>` の `lang` / `dir` と React Aria の `I18nProvider` に、テーマは `<html>` の `data-theme` に反映する。
    見本のテーマ（`.storybook/sampleTheme.ts`、ADR-0059）は、サーバーのスタイルシートと同じ `@layer` の外の `:root` の規則にして、構築できるスタイルシートで当てる。
    `Foundations/Theme sample`（`src/ThemeSample.stories.tsx`）だけが見本のテーマを固定し、代表的な部品のスクリーンショットを残す。ほかのストーリーは既定のトークンで検査する。
  - ストーリーは `scripts/task.sh ui:test` で実ブラウザのテストになる（`app/ui/vitest.config.ts`、Storybook の Vitest アドオンと Vitest のブラウザモード、Playwright の Chromium）。
    - プロジェクトは方向 × テーマの 4 つ（`en` / `ar-XB` × ライト / ダーク）。すべてのストーリーを 4 つすべてで動かす。
    - axe の違反が 1 つでもあれば失敗する（`parameters.a11y.test: 'error'`、Y1 のタグ）。
    - `@my/ui` の部品の振る舞いのテスト（フォーカスの閉じ込めと復帰、Esc、矢印キー）は、ストーリーの `play` 関数に `storybook/test` の `expect` と文脈の `userEvent` / `canvas` で書く。
      すべてのプロジェクトで動くので、RTL の矢印キーは `document.documentElement.dir` で向きを決めて同じ `play` で検査する。
    - ダイアログ・ポップオーバー・トーストなどのオーバーレイは、`<body>` ではなくストーリーの枠に portal する（`UNSAFE_PortalProvider`）。枠は `contain: layout` なので、`position: fixed` のオーバーレイも枠を覆い、スクリーンショットに写る。
      オーバーレイのストーリーは、デコレーターで枠に高さを与える。
    - 描画と axe のあと、ストーリーの枠（コンテナクエリの基準）を幅 320 / 640 / 1024 px にして `toMatchScreenshot` で比較する（`.storybook/vitest.setup.ts`）。
      `play` 関数のためだけにある長いストーリー（仮想化した `Feed`）は `no-screenshot` のタグで比較を外す。同じ部品のほかのストーリーが見た目を検査する。
    - スクリーンショットは `prefers-reduced-motion: reduce` を模擬して撮る。その状態で枠の中に動いているアニメーションが 1 つでもあれば失敗する（D6）。ストーリーの描画と `play` 関数は模擬なしで動く。
      `forced-colors` のタグのストーリーは、強制カラーモード（`forced-colors: active`）も模擬して撮る（強調表示の層の下線）。
    - `play` 関数（`storybook/test`）で、部品をロールとアクセシブルネームで探して検査する（Y2）。
  - 基準画像は `app/ui/src/__screenshots__/` にあり、固定した Playwright のイメージ（`Taskfile.yml` の `PLAYWRIGHT_IMAGE`）の中でだけ作る。
    手元の OS のフォントや描画では一致しないので、手元で作った画像はコミットしない。
    - 見た目を変えたら `scripts/task.sh ui:snapshots-update`（Docker でそのイメージを使う）で作り直し、差分の画像を確かめてからコミットする。
    - 同じ比較を手元でするときは `scripts/task.sh ui:test-docker` を使う。`scripts/task.sh ui:test` を手元で動かす（`scripts/task.sh ui:test-install` でブラウザを入れる）と、axe は信用できるが、スクリーンショットは一致しない。
  - `scripts/task.sh check` はブラウザなしで動く検査だけを集める。`scripts/task.sh ui:test` は CI の `ui-test` ジョブがそのイメージの中で、`app/ui` を変える PR で動かす（Merge Queue では動かさない、[ADR-0047](../../adr/0047-ci-protection-layers.md)）。失敗したときの実際の画像と差分は成果物 `ui-test-screenshots` にある。
  - ワークフローのイメージと `PLAYWRIGHT_IMAGE`、`playwright` / `@playwright/test` の版が食い違うと、`scripts/task.sh web:playwright-image-check`（`scripts/task.sh check` の一部）が失敗する。
