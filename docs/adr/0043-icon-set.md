# ADR-0043: UI キットのアイコンを既存のアイコンセット（Phosphor）から使う

- Status: Accepted
- Date: 2026-10-19
- Deciders: @rosylilly
- Issue: #483

## Context（背景・課題）

`@my/ui` のアイコン集（`app/ui/src/Icons.tsx`、#367）は、23 個のアイコンを 24 × 24 の格子・線幅 2 の SVG のパスとして手で描いて埋め込んでいる（[web-client.md §16.3](../architecture/web-client/16-look-states-and-wording.md#163-アイコン) の G10）。
決定者から「react-icons の Phosphor Icons や Remix Icons などを利用してアイコンを使うべき。現在の SVG 埋込式はデザインのレビューが困難」というフィードバックがあった。

- パスの数字の列は、差分を読んでも形が分からない。形を確かめるには Storybook を開くしかなく、レビューでは見落としやすい。
- 新しい画面がアイコンを必要とするたびに、誰かが同じ描き方で描き足す必要があり、形の一貫性を保つ人がいない。
- 既存のアイコンセットなら、形は公開のカタログで名前から引け、太さや塗りの種類もセットの中でそろっている。

アイコンの出どころは UI キットの依存（[ADR-0031](0031-ui-kit-behavior-dependencies.md) の許可リスト）と、G10〜G13 の決まりを変えるので、ここで決める。
関連する要件は NFR-A11Y-1（アクセシビリティ）、NFR-I18N-4（RTL）、NFR-DIST-1（AGPL-3.0 での配布、[ADR-0011](0011-license.md)）。

## Decision Drivers（判断基準）

1. ライセンスが AGPL-3.0 と両立し、`make web-licenses` の許可リストに入る（ADR-0011）
2. オン / オフを形で示せる（G13、WCAG 1.4.1）: 星・ブックマーク・警告は塗りつぶし、ブーストのような閉じない形は太い線の版が、同じセットの中にある
3. デザインのレビューのしやすさ: 差分にアイコンの名前が出て、名前から公開のカタログで形を確かめられる
4. 今の決まりを保てる: 置き場所が大きさを決める（G11）、飾り（`aria-hidden`、G12）、向きのあるアイコンを RTL で反転する（G13）
5. 木を揺すったあとのバンドルが小さい
6. 数と一貫性: タイムライン・投稿・管理・メディアで必要になる形が 1 つのセットで足りる
7. 公開から 3 日以上たった版を固定でき（ADR-0025）、保守が続いている

## Considered Options（選択肢）

調べた結果は [#483 の調査報告](https://github.com/mycroblog/my/issues/483#issuecomment-6261371624) にまとめた（数値は 2026-10-19 に npm の registry と各パッケージの中身で確かめた）。
バンドルの大きさは、今の 23 個（オンの版を含む）を Bun で木を揺すって minify し、React を外して gzip した値。今の手描きは 1.3 KB。

| 選択肢 | ライセンス | 数 | 太さ・塗り | 23 個の大きさ（gzip） | 最新版 |
| --- | --- | --- | --- | --- | --- |
| 1. react-icons の Phosphor（`react-icons/pi`） | MIT（react-icons）、MIT（Phosphor） | 1,512 × 6 | Thin / Light / Regular / Bold / Fill / Duotone を別のコンポーネントで | 4.4 KB | 5.7.0（2026-06-30、Phosphor core 2.1.1） |
| 2. Phosphor の公式パッケージ（`@phosphor-icons/react`） | MIT | 1,512 × 6 | 同じ 6 種を `weight` で切り替え | 15.2 KB | 2.1.10（2025-05-22） |
| 3. Lucide（`lucide-react`） | ISC（一部 MIT） | 約 1,870 | 線だけ（塗りの版なし） | 3.7 KB | 毎日公開 |
| 4. Tabler（`@tabler/icons-react`） | MIT | 約 5,100 + 塗り約 1,050 | 線と、一部に塗りの版 | 2.4 KB | 3.49.0（2026-10-05） |
| 5. Remix（`@remixicon/react`、または `react-icons/ri`） | 4.9.0 から Remix Icon License v1.0（4.8.0 までと react-icons の 4.2.0 は Apache-2.0） | 約 1,520 × 2 | Line / Fill | 4.1 KB | 4.9.0（2026-01-29） |
| 6. 手描きの SVG を続ける | AGPL-3.0（自前） | 23 | 自前で描く | 1.3 KB | — |

## Decision（決定）

**選択肢 1（react-icons の Phosphor、`react-icons/pi`）を採用する。** `Icons.tsx` は「意味の名前から Phosphor のアイコンを引く薄い層」にする。

- Phosphor は、どのアイコンにも同じ 6 種の太さ・塗りがあるので、G13 のオン / オフ（星・ブックマーク・警告は Fill、ブーストは Bold）をセットの中だけで表せる。
  Lucide（選択肢 3）は塗りの版がなく、`fill` を付けると警告の「!」や繰り返しの矢印が潰れる。Tabler（選択肢 4）は塗りの版が一部だけで、ブーストのオンの形がない。
- 公式パッケージ（選択肢 2）は 1 つのコンポーネントが 6 種の形をすべて持つので、23 個で 15 KB と react-icons の 3 倍余りになる。中身の Phosphor の版（core 2.1.1）は同じ。
- Remix（選択肢 5）は 4.9.0 から独自のライセンスになり、アイコンだけの配布を禁じる条件が AGPL-3.0 の「それ以上の制限を課さない」（§10）と両立しない。
  `make web-licenses` の許可リストにも入らない。Apache-2.0 の古い版に固定すると、以後の更新を受けられない。
- 手描き（選択肢 6）はバンドルが最小だが、決定者のフィードバックの「レビューが困難」を解けない。

### 薄い層の形

- `Icons.tsx` は今の名前（`ReplyIcon`・`StarIcon` など、意味の名前）の export を保ち、それぞれを Phosphor のコンポーネントの対応で定義する。
  画面の側の import は変わらず、セットをもう一度替えるときも `Icons.tsx` だけを直せばよい。
  対応は 1 か所の表（意味の名前 → オフの形・オンの形・向きの有無）にし、Storybook の一覧もこの表から作る。
- `react-icons` を import してよいのは `Icons.tsx` だけにし、ほかのセット（`react-icons/fa` など）を混ぜないように `react-icons/pi` 以外を lint で禁じる。
  `app/web` は `@my/ui` の export だけを使う（G10 のまま）。
- 層はどのアイコンにも `aria-hidden="true"` と `focusable="false"` を付け（G12）、向きのあるもの（返信・戻る・外部へのリンク）に `rtl:-scale-x-100` を付ける（G13）。
  Phosphor の `mirrored` は書字方向を見ないので使わない。
- 大きさは今のまま置き場所が決める（G11）。react-icons は `width` / `height` に `1em` を書くが、`Button`・`ToggleButton`・`Link`・ナビゲーションの `[&>svg]:size-*` がそれより優先される。
  置き場所が大きさを決めない所でも、`0 × 0` ではなく文字の大きさで描かれる。
- 太さは Regular（24 px で線幅 1.5 px 相当）を基本にし、オンの状態は形が閉じるものを Fill、閉じないものを Bold にする。
  Bold（2.25 px 相当）を基本にすると今の手描きの 2 px に近いが、ブーストのオン（Fill）とオフ（Bold）が形でほとんど区別できず、G13 を満たせない（#501 で両方のスクリーンショットを比べた）。

### Storybook

`Foundations/Icons` の一覧は、表のすべての項目を、意味の名前・Phosphor の名前・オンの形・向きの有無と並べて出す。
今の `ui-test` が 3 つの幅 × light / dark × LTR / RTL でスクリーンショットを撮るので、置き換えや追加のレビューでは、差分に出る Phosphor の名前と、スクリーンショットの差分を見ればよい。

見直す条件: react-icons の公開が長く止まり、Phosphor の新しい版が react-icons に入らなくなった場合、または必要な形が Phosphor にない場面が続いた場合は、公式パッケージ（選択肢 2）か Tabler（選択肢 4）を見直す。

## Consequences（影響）

- アイコンの形のレビューは、パスの数字ではなく Phosphor の名前とスクリーンショットで行える。新しいアイコンは描かずに、表に 1 行足すだけになる。
- 線が今（2 px）より細い 1.5 px 相当になり、角の形も変わるので、`@my/ui` のスクリーンショットの基準はすべて撮り直しになる。
- 23 個で約 3 KB（gzip）大きくなる。
- react-icons は 30 以上のセットを含み、展開後に約 88 MB ある。インストールの時間と `node_modules` の大きさが増える（バンドルには選んだアイコンしか入らない）。
- react-icons は各セットの更新を取り込むまで遅れることがある（Phosphor は core 2.1.1 で最新と同じ、Lucide や Remix は古い）。
- 後続の作業:
  1. #501: `@my/ui` に `react-icons` を足し、ADR-0031 の許可リスト・`app/eslint.config.js` の `@my/ui` の許可・`react-icons/pi` 以外と `Icons.tsx` 以外からの import の禁止を足す。
     `Icons.tsx` を薄い層に書き換え、`Icons.stories.tsx` を表から作る一覧にし、スクリーンショットの基準を `make ui-snapshots-update` で撮り直す。
     [web-client.md §16.3](../architecture/web-client/16-look-states-and-wording.md#163-アイコン) の G10〜G13 と `app/ui/README.md` のアイコンの節を、Phosphor の太さの決まりに合わせて書き換える。
  2. #502: G10 の「機能やルートに SVG のパスを書かない」を、`app/web` の `<svg>` を lint で禁じる形で強制する（今はレビューと `knip` だけ）。
