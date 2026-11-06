---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.settings_.branding.tsx", "app/web/src/features/admin/components/BrandSettings.tsx", "app/web/src/features/admin/hooks/useBrandImage.ts", "app/web/src/features/admin/hooks/brandRenditions.ts", "app/web/src/features/admin/api/brand.ts", "app/web/src/features/admin/components/ServerSettings.tsx", "app/ui/src/LocalNav.tsx"]
unavailable: {"アイコンを設定する（SVG を選び、プレビューを見て保存する）": "the agent has no file to choose: the browser tools give it none", "ロゴを設定する": "the agent has no file to choose: the browser tools give it none", "アイコンを組み込みのものに戻す": "the demo scenario sets no icon of its own, and setting one needs the PNG renditions that the web client draws from a chosen file"}
---

## サーバーの設定: アイコンとロゴ（`/admin/settings/branding`）

`manage_settings` を持つ管理者が、サーバーのアイコン・ロゴ・暗い背景のロゴを決めるページ（[ADR-0059](../../../adr/0059-design-tokens-and-instance-theme.md)、[§14](../14-admin.md)）。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`）を置き、今のページ「アイコンとロゴ」を `aria-current="page"` にする。管理のメニューは「サーバーの設定」の 1 項目のままで、このページでも今いる項目になる。

### 1. 使う人と頻度

- サーバーを立ち上げた管理者。最初にアイコンとロゴを決める。立ち上げのときに 1 回。
- 運営を続ける管理者。作り直したときに年に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| アイコンを設定する（SVG を選び、プレビューを見て保存する） | 4 |
| ロゴを設定する | 4 |
| アイコンを組み込みのものに戻す | 3 |

手順は、管理のメニューの「サーバーの設定」を開く 1 つ、ページの間のリンクで「アイコンとロゴ」を開く 1 つ、ファイルを選ぶ 1 つ、その絵の保存のボタンを押す 1 つ（戻すのは、開く 2 つと戻すボタンの 1 つ）。

### 3. 情報の優先順位とまとまり

1. アイコンとロゴ: アイコン・ロゴ・暗い背景のロゴの 3 つのフォーム（`h3`）。それぞれ、今の絵か組み込みを使っていること、選んだ SVG とその使われる場所のプレビュー、
   サーバーが取り除いたもの、ファイルを選ぶボタン（拒まれた理由はその下）、アイコンだけ背景の色。

### 4. 主な操作と保存の仕方

- 3 つの絵は別のものなので、フォームを分けたままにする。どのフォームも主な操作は自分の保存のボタン 1 つ（`primary`）で、ほかの絵は送らない。
- 保存は、まとめて保存だけ。SVG を選ぶと確かめてプレビューを描くが、保存はボタンを押したときだけ行う。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| SVG を選ぶ | その場。ボタンが確かめている間の状態になり、終わるとプレビューと取り除いたものが出て、その件数を読み上げる。拒まれたら理由と直し方をボタンの下に出す |
| アイコンの背景の色を変える | その場。不透明な版のプレビューが新しい色で描き直される |
| アイコン・ロゴの保存 | その場。ボタンの横（`FormActionBar`）に結果を出し、プレビューは「使用中」の絵に替わる。外枠・ランディングページ・サーバーの画面は `GET /instance` を取り直して新しい絵を出す |
| 組み込みに戻す | その場。ボタンの横に結果を出し、「my の組み込みのものを使っています」に替わる |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「サーバーの設定」から、ページの間のリンクの「アイコンとロゴ」。
- 出口: ページの間のリンクのほかのページ、管理のメニュー。保存したアイコン・ロゴは、外枠の先頭・ランディングページ・サーバーの画面（ログインなど）で確かめられる。
