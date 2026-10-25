# ADR-0096: emoji.css をアプリと Storybook から読み込み、theme.css からは読まない

- Status: Accepted
- Date: 2026-10-24
- Deciders: @rosylilly
- Issue: #1176

## Context（背景・課題）

[ADR-0095](0095-noto-color-emoji.md) §2 は、Noto Color Emoji の `@font-face` を `@my/ui` の `emoji.css` に置き、`theme.css` から読むと決めた。
しかし `theme.css` は、Web クライアントと Storybook だけでなく、Go のサーバーが描画する画面（サインイン・登録・同意画面・規約など、[web-client §8](../architecture/web-client/08-server-rendered-pages.md)）のスタイルシート `app/web/src/server-pages.css` も読み込む。
`theme.css` から `emoji.css` を読むと、これらの画面も絵文字のフォントを持つことになる。ADR-0095 の実装の issue（#1176）は、サーバーが描画する画面のフォントを変えないことを求めていた。

そこで実装（PR #1182）は、`theme.css` からは読まず、Web クライアントの `app/web/src/index.css` と Storybook の `app/ui/src/storybook.css` が `@my/ui/emoji.css` を読み込む形にした。
ADR-0095 の本文と実装が食い違うので、読み込む場所を決め直す。関係する要件は FR-605（サーバー単位のテーマ）と、ADR-0095 の FR-101・FR-116 である。

## Decision Drivers（判断基準）

1. サーバーが描画する画面のフォントと読み込む量を変えない（#1176 の要件）。
2. Web クライアントと Storybook のスクリーンショットが、同じ絵文字のフォントで描かれる（ADR-0027・ADR-0095）。
3. フォントの並び（`--font-sans` / `--font-mono`）とサーバー単位のテーマのトークンを 1 か所（`theme.css`）に保つ。

## Considered Options（選択肢）

1. **`theme.css` から読む**（ADR-0095 §2 のとおり）: `theme.css` を読み込むすべての場所が同じフォントになり、読み込みを忘れることが無い。
   ただし、サーバーが描画する画面も絵文字のフォントを持ち、#1176 の要件に反する。
2. **アプリと Storybook が読む**: `theme.css` は `--font-sans` の先頭にファミリー名 `"my Noto Color Emoji"` を置くだけにし、`@font-face` を持つ `emoji.css` は使う側が読み込む。
   `emoji.css` を読まないスタイルシートには、そのファミリーの face が無いので、ブラウザーはその名前を飛ばして OS のフォントで描く。新しい入口は読み込みを忘れないように気を付ける必要がある。
3. **サーバーが描画する画面に別の theme を作る**: `theme.css` から `emoji.css` を読み、サーバーの画面には絵文字のフォントの無い別のトークンのファイルを用意する。
   サーバーの画面は確実に分かれるが、トークンとサーバー単位のテーマの規則（`theme_tokens.gen.go` の生成元）が 2 つに割れ、同期を保つ手間が増える。

## Decision（決定）

選択肢 2 を採用する（決定者の回答「emoji.css を読み込む場所が ADR-0095 → ADR 側を修正」）。ADR-0095 §2 の「`theme.css` から読む `emoji.css`」の部分をこの ADR が置き換える。
置き場所（`@my/ui` の `app/ui/src/fonts/emoji/` と `app/ui/src/emoji.css`、`@my/ui/emoji.css` として公開）と、ADR-0095 のほかの決定は変えない。

- Web クライアントの `app/web/src/index.css` と Storybook の `app/ui/src/storybook.css` が、`theme.css` の後に `emoji.css` を読み込む。
- `theme.css` は `emoji.css` を読み込まない。`--font-sans` と `--font-mono` の先頭に `"my Noto Color Emoji"` を置くのは `theme.css` のままにする（ADR-0095 §3）。
- サーバーが描画する画面（`server-pages.css`）は `emoji.css` を読み込まず、OS の絵文字のフォントで描く。ファミリー名は `--font-sans` に残るが、face が無いので飛ばされる。

## Consequences（影響）

- サーバーが描画する画面は、絵文字のフォントを読み込まず、見た目も変わらない。
- トークンとフォントの並びは `theme.css` 1 か所のままで、サーバー単位のテーマの規則も 1 つで済む。
- Web クライアントの新しい入口（新しいエントリのスタイルシートや、別の Storybook）で Noto Color Emoji を使うには、そのスタイルシートが `@my/ui/emoji.css` を読み込む必要がある。読み忘れても壊れはしないが、絵文字が OS のフォントになる。
- サーバー単位のテーマで `font-sans` / `font-mono` を上書きすると、`"my Noto Color Emoji"` を先頭に残さない限り、Web クライアントの絵文字も OS のフォントになる（[app/ui/README.md](../../app/ui/README.md#server-themes)・[運用の手引き](../operations/instance-settings.md#theme)）。
- 後続の作業は無い。実装は PR #1182 で済んでいる。

## Open Questions / 要検証

- なし
