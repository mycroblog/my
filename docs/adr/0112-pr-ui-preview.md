# ADR-0112: PR ごとの UI のプレビュー: 部品と画面の Storybook を Cloudflare Workers の Previews に置く

- Status: Accepted
- Date: 2026-10-28
- Deciders: @rosylilly
- Issue: #1638

## Context（背景・課題）

ADR-0110 は、決定者が画面と動きを見てコメントし、そのコメントを画面の要件・部品・検査に変える批評のサイクル（L5）を置く。
今の仕組みでは、決定者が PR の UI を見る手段がない。

- 品質ゲート（ADR-0027）のスクリーンショットは `prefers-reduced-motion: reduce` で撮る（D6）ので、マイクロインタラクション（ADR-0110 の P2）が写らない。
- Storybook は `app/ui` の部品だけにある。`app/web` にストーリーはなく、画面（設定の画面など）を、空・エラー・長い文言の状態で見る手段は手元で `my serve` を動かすことだけである。
- PR の検査は 2 分以内の preflight で、Storybook のビルドを足す余地はない（ADR-0079）。

my は OSS で、外からの PR はフォークから来る。フォークの PR の workflow は秘密情報を読めない。

## Decision Drivers（判断基準）

1. 決定者が、PR の画面と部品を、状態と動きを含めてブラウザで見られること。
2. 費用がほぼゼロであること（vision の運用費の目標と同じ向き）。
3. フォークの PR でも動き、秘密情報を漏らさないこと。
4. PR の検査（ADR-0079）を遅くせず、マージの条件にしないこと。
5. 画面のストーリーが、既存のテストの資産（MSW のハンドラとフィクスチャ）を使い回せること。

## Considered Options（選択肢）

置くもの:

- A1. `app/ui` の Storybook だけ。
- A2. `app/ui` の Storybook と、`app/web` の画面のストーリー（MSW で API を差し替える）を足した Storybook。
- A3. `app/web` 全体を、ブラウザの MSW で動くデモとしてビルドしたもの。

置き場:

- B1. Cloudflare Workers の静的アセットと Previews（PR ごとの環境）。
- B2. Chromatic（Storybook の運営元のサービス。公開と見た目の比較と UI のレビュー）。
- B3. GitHub Pages。

## Decision（決定）

**A2 と B1 を採る。**

### 1. 置くもの（A2）

- `app/web` に Storybook を足し、画面のストーリーを置く。ストーリーは機能の部品（`features/<x>/components`）をルートの単位で描き、API は機能のテストの MSW のハンドラ（`features/<x>/test/handlers.ts`）とフィクスチャで差し替える。
  ブラウザの MSW には `msw/browser` を使う（MSW は既存の依存）。
- 画面のストーリーは、ADR-0110 の L1 の作業ごとに、通常・空・読み込み中・エラー・長い文言（`ja`・`en-XA`）・RTL（`ar-XB`）・320 px の状態を持つ。操作は `play` 関数で書き、Storybook の Interactions のパネルで動きを再生できるようにする。
- `app/web` の Storybook は、`app/ui` の Storybook を Storybook の composition（`refs`）で取り込み、1 つの URL で部品と画面の両方を見られるようにする。
- 画面のストーリーは、品質ゲート（ADR-0027）の axe の検査に入れる。スクリーンショットの比較には入れない（画面は E2E のスクリーンショットが受け持つ）。

### 2. 置き場（B1）

- Cloudflare の、静的アセットだけを返す Worker を 1 つ作る。サーバーの実行環境ではない（my のサーバーは Cloudflare Workers を対象にしない方針のまま）。
- **PR ごと**: Workers の Previews（`wrangler preview --name pr-<番号>`）に置く。URL は `pr-<番号>-<Worker 名>.<サブドメイン>.workers.dev` で、`workers.dev` は `X-Robots-Tag: noindex` を返す。
  PR が閉じたら Preview を消す。無料のプランで Worker あたり 100 の Preview を持て、上限では最も古いものから消える（2026-10-28 に [Previews の文書](https://developers.cloudflare.com/workers/previews/)で確認）。
- **main**: main への push で本番の Worker に置く。これが ADR-0110 の L5 で決定者が見る、全画面と部品の一覧になる。

### 3. CI の構成

- **ビルド**（`ui-preview.yml`、`pull_request`、`app/**` と `locales/**` を変える PR）: 2 つの Storybook を静的にビルドし、成果物として上げる。秘密情報を持たない。
  PR の必須の検査にもマージの条件にもしない。
- **公開**（`ui-preview-deploy.yml`、ビルドの `workflow_run`）: main の workflow として動き、Cloudflare の API トークン（秘密情報）を持つ。成果物を静的なファイルとしてだけ扱い、中身を実行しない。
  Worker の設定（`wrangler` の設定）は main の側のものを使い、成果物から Worker のコードを読まない。
- **PR へのコメント**: 公開した URL と、main の Storybook と比べて追加・変更されたストーリー（ビルドの `index.json` の差分）への直接のリンクを、PR の 1 つのコメントにまとめて更新する。
  決定者は、このリストを見る順として使い、コメントは GitHub の PR に書く（L5 の分類は ADR-0110）。
- **閉じた PR**: `pull_request` の `closed` で Preview を消す。

### 4. 実装の最初に確かめること

Previews が静的アセットだけの Worker で動くことは、文書に明記がない。実装の issue の最初に確かめ、動かなければ、同じ Worker のバージョンの URL（`wrangler versions upload --preview-alias pr-<番号>`）に置く。
どちらでも URL・コメント・削除の流れは変わらない。

### 見直す条件

- Cloudflare の無料のプランで、Previews もバージョンの URL も静的アセットに使えない場合（B3 にして、PR ごとの URL を諦め、main の一覧だけを置く）。
- 画面のストーリーの手入れ（MSW のハンドラとルートの差し替え）が、画面の追加のたびに大きな手間になる場合（A3 に切り替える）。

## Consequences（影響）

- 良い影響: 決定者が PR ごとに、画面の状態と動きをブラウザで見てコメントできる。変わったストーリーの一覧が、そのままレビューの順になる。
- 良い影響: 画面のストーリーが、画面の状態（空・エラー・長い文言・RTL）を作る共通の手段になり、ADR-0110 の L3 と L4 の入力にも使える。
- 悪い影響: `app/web` に Storybook の設定とストーリーが増え、画面を足すたびにストーリーを書く。
- 悪い影響: Cloudflare のアカウントと API トークンが、プロジェクトの外部の依存として増える。トークンは Workers の編集だけに権限を絞る。
- 悪い影響: Preview の URL は公開で、PR の未マージの UI を誰でも見られる（OSS なので内容はもともと公開）。
- 後続の作業（issue にする）:
  - Cloudflare のアカウント、Worker、API トークンの用意（決定者）。
  - `app/web` の Storybook と、設定の画面のストーリー（ADR-0110 の試験の画面）。
  - `ui-preview.yml`・`ui-preview-deploy.yml` と PR へのコメント、閉じた PR の掃除。`docs/agents/workflow.md` の「何がどこで動くか」の表への追加。
  - `app/web` の境界の lint（`boundaries/no-unknown-files`）と knip に、ストーリーのファイルを足す。

## Pros and Cons of the Options（詳細比較・任意）

- **A1（部品だけ）**: 既存の Storybook をそのまま置くだけで、いちばん安い。ただし ADR-0110 の問題の中心は画面の情報設計で、部品だけでは批評ができない。
- **A3（アプリ全体のデモ）**: 画面の移動と導線をそのまま確かめられ、L4 の課題テストに近い。ただし特定の状態（エラー・空・長い文言）を作るには、デモの中に状態を切り替える仕組みが要り、状態を列挙して見せる Storybook より批評の入口として弱い。
- **B2（Chromatic）**: 公開・見た目の比較・ストーリー単位の承認が 1 つのサービスでそろう。
  ただし無料のプランは月 5,000 スナップショット（2026-10-28 に[料金の文書](https://chromatic.com/pricing)で確認）で、`app/ui` だけでもストーリー × 幅 × テーマ × 書字方向ですぐ超える。
  見た目の比較は既存の Vitest のスクリーンショットと重なる。
- **B3（GitHub Pages）**: 外部の依存が増えない。ただし 1 つのリポジトリに 1 つのサイトで、PR ごとの URL を持てない。
