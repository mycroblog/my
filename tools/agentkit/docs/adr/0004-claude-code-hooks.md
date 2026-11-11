# ADR-0004: Claude Code のフックで、高くつく読み方を止め、長い出力を切り詰める

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: なし

## Context（背景・課題）

道具を用意しても、エージェントが従来どおり大きな文書を丸ごと読み、CI を監視し続ければ効果は出ない。
エージェントに依らない規則は AGENTS.md に書くが、Claude Code ではフックでツール呼び出しの前後に介入できる。
決定者の指示（原文）:

> 4 : 固有フックもいれる

## Decision Drivers（判断基準）

- 止めるのは、同じ答えをずっと安く得る方法がある場合だけにする
- フックの失敗でツール呼び出しを止めない（fail-open）
- 許可の流れを変えない（`allow` を返して確認を省かない）

## Considered Options（選択肢）

1. 規則を AGENTS.md に書くだけ
2. 規則に加えて Claude Code のフックを入れる

## Decision（決定）

選択肢 2。`.claude/settings.json` に次を置く。実体は `scripts/agentkit-hook.sh` を通して `bin/agentkit hook <event>` が処理する。

- SessionStart: `bin/agentkit` をビルドし、索引を最新にする（出力しない）
- PreToolUse（Bash・Read）。どれも拒否で、理由には代わりに実行するコマンドを、拒否したコマンドのパターンとパスを入れて返す。規則はエージェントの種類を問わない
  - リポジトリのファイルを対象にした `grep` / `rg` / `ag` / `git grep` → `agentkit grep`（宣言を読むなら `show <Name>`）
  - リポジトリのファイルへの `sed -n 'A,Bp'`・`head`・`tail`・`awk 'NR>=A'` と `cat <file> |` → Read の offset / limit、`show <path>:A-B`、`outline`
  - リポジトリのファイルを書き換える `sed -i`・`perl -pi`・`python3` / `node` のスクリプト → Edit（`locales/` は `scripts/i18n.sh add|mv|rm`）
  - `main` の `git merge` と、`--rebase` も `--ff-only` もない `git pull` → `git fetch origin && git rebase origin/main`、`agentkit regen`、`git push --force-with-lease`
  - CI のポーリング（`gh pr checks`、`gh run watch`、`--log-failed` のない `gh run view`、`--workflow` のない `gh run list`、check run と workflow run の `gh api`、`statusCheckRollup`）→ バックグラウンドの `agentkit pr wait`
  - sleep するループと `pgrep`（`bash -c` の中も）→ `run_in_background` と完了の通知
  - 40KB 以上の Markdown（コードは 80KB 以上）を、範囲を指定せずに Read または `cat` すること → `outline` と `show`
  - 拒否しないもの: パイプのフィルタとしての grep、作業ツリーの外のファイル、git が無視するファイル（`.tmp/`・`node_modules/`・ログ）、シェルが計算するパス、`grep -q`、リビジョンの検索、`--ff-only` の merge とほかのブランチの merge
  - 複合コマンドは、引用とヒアドキュメントの外の `;`・`&&`・`||`・`|`・改行で単純コマンドに分け、`cd` とコマンド内の変数代入を追ってから判定する
- PostToolUse（Bash）: 標準出力と標準エラーの合計が 2 万文字を超える出力を、先頭と末尾だけに切り詰めて `updatedToolOutput` で返し、全体は `.tmp/agentkit/out/` に保存する。agentkit 自身の出力（予算内に収めている）は切り詰めない
- しきい値は環境変数 `AGENTKIT_BIG_FILE`・`AGENTKIT_MAX_OUTPUT` で変えられる
- `bin/agentkit` が無いとき、入力が読めないとき、想定外の入力のときは何もしない。`allow` は返さない

## 経緯

- 再帰的な grep への助言（`additionalContext` で `agentkit find` を勧める）は、681 回出て効果がなかった（エージェントは `| head` を足した）ので、拒否と `agentkit grep` に置き換えた。順位付きの全文検索では、エージェントが要る正確な文字列・正規表現の一致、テストと生成物の除外、出力の上限が得られなかった
- 拒否は、その形を 100% 止めたが、理由がより楽な代わりを名指ししないと、エージェントは同じ働きの別の形に移った。そこで理由に、そのまま実行できるコマンドを入れる
- 大きなファイルの拒否は、`ctx` のパック（`.tmp/agentkit/ctx/*.md`）とハーネスの出力ファイルも止めていたので、リポジトリのファイルだけに絞った
- `gh pr checks --watch` と `gh run watch` だけの拒否は、`gh pr checks` の繰り返しや `gh run view` に移られたので、CI のポーリング全体に広げた。sleep するループの拒否は `timeout N bash -c "until …"` で抜けられたので、`bash -c` の中も見る

## Consequences（影響）

- リポジトリの検索・部分読み・書き換えは、agentkit と Read / Edit を通る。agentkit にない grep の機能（`-v`、`-L`）は、`git ls-files` やファイルをパイプで grep に渡して使う
- 大きな文書を Edit する前の Read も、範囲の指定が必要になる
- 切り詰めた出力の続きは、保存したファイルを grep するか範囲を指定して読む必要がある
- Claude Code 以外のエージェントには効かない。それらには AGENTS.md の規則と `scripts/agentkit.sh` だけが届く
