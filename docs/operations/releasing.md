# リリースの手順

リリースを作る人（決定者）向けの手順です。リリースは `release.yml` だけが作ります（[ADR-0079](../adr/0079-ci-single-gate-merge-queue.md)）。
`release.yml` は、指定したコミットで遅い検査（`slow-suite.yml`）を回し、すべて通ったときだけ、そのコミットにタグを打って GitHub Release を下書きで作り、
クライアントライブラリ（[ライブラリ](#ライブラリmykit)）を同じ版で公開します。遅い検査の中身と、ほかにどこで回るかは [workflow.md の「What CI runs where」](../agents/workflow.md#what-ci-runs-where) にあります。

## 版

- 版は `vX.Y.Z` か、プレリリースの `vX.Y.Z-<識別子>`（例: `v0.1.0-rc.1`）です。形が違う版は最初のジョブで落ちます。
- `main` 以外のブランチからは、プレリリースの版だけを作れます。関門をブランチで試すためのもので、正式なリリースは `main` から作ります。
- すでにタグがある版は最初のジョブで落ちます。遅い検査の後にも、タグと同じ名前のリリースがないことを確かめ直します。

## 手順

1. 最新の Nightly が通っていることを確かめる。落ちているなら、先に直す（[workflow.md §9](../agents/workflow.md#failures-on-main-orchestrator)）。

   ```sh
   gh run list -R mycroblog/my --workflow nightly.yml --limit 3
   ```

2. `main` で `release.yml` を回す。回したときの `main` の先頭のコミットが、検査されタグを打たれるコミットになる。

   ```sh
   gh workflow run release.yml -R mycroblog/my --ref main -f version=v0.1.0
   gh run list -R mycroblog/my --workflow release.yml --limit 1
   ```

3. 終わるのを待つ。遅い検査（E2E の全体・連合・実 PostgreSQL・stream の契約・イメージ・CodeQL）は、各ジョブが並行して動いても時間がかかる。
   ジョブ `release` は、遅い検査のジョブが 1 つでも落ちるか取り消されると動かず、タグも Release もできず、ライブラリも公開されない。
   落ちたときは原因を直して `main` に入れ、同じ版でもう一度回す。
   `release` が通った後にジョブ `npm` だけが落ちたときは、原因（多くは trusted publisher の登録）を直してその run の落ちたジョブを回し直す
   （`gh run rerun <run-id> -R mycroblog/my --failed`）。すでに公開した版は飛ばす。
4. 通ったら、下書きの Release（`gh release view v0.1.0 -R mycroblog/my`、または Releases の画面）を見て、自動で作られた変更の一覧を整え、
   「mykit」の節（[ライブラリ](#ライブラリmykit)）と「API の変更」の節を書いて、公開する。プレリリースの版は、プレリリースとして下書きされる。

## API の変更

クライアント API（`/api/v1`）は追加を既定にし、破壊的変更は非推奨の印を付けて少なくとも 1 つの v0.x のマイナーリリースの間残してから行います
（[ADR-0090](../adr/0090-third-party-oauth-apps.md) §8、仕組みは [client-api.md §2.6](../architecture/client-api.md#26-安定性と非推奨)）。

- 下書きの Release には、自動で作られた変更の一覧に加えて「API の変更」の節を必ず置く。節には次を書く。
  - 追加した操作・フィールド・スコープ
  - この版で `deprecated: true` にした操作と、その `Deprecation` と `Sunset` の日付、代わりの操作
  - この版で削除した操作・フィールド（前の版までに非推奨にしたものだけ）
  - セキュリティの修正で猶予なしに変えたもの（理由とともに）

  変更がなければ「なし」と書く。節を作るときは `api/openapi.yaml` の前の版からの差分と、`x-sunset` を持つ操作の一覧（`grep -n x-sunset api/openapi.yaml`）を見る。
- 公開する前に、削除してよい操作を確かめる。操作を削除するには、その操作が前のマイナーリリース以前のリリースですでに `deprecated: true` で配られていて、
  `Sunset` の日が過ぎていなければならない。同じマイナーリリースの中で非推奨にして削除してはならない。
- `Sunset` の日は `Deprecation` の日より前にしない（生成器が検査する）。非推奨にするときは、次のマイナーリリースの予定より後の日にする。

## ライブラリ（mykit）

クライアントライブラリ `mykit` は、サーバーと同じ版の番号で同じリリースに出します（[ADR-0093](../adr/0093-api-client-library-mykit.md) §8）。
版を付ける手順は `scripts/release-libraries.sh`（試験は `scripts/release-libraries-test.sh`、`scripts/task.sh shell:test`）にあります。

- **Go のタグ**: ジョブ `release` が、サーバーのタグ `vX.Y.Z` と同じコミットに `pkg/mykit/vX.Y.Z` を打つ。Go のモジュールはタグを打つだけで公開になり、
  proxy.golang.org が最初に求められたときに取り込む。タグがすでにあれば、最初のジョブで落ちる。
- **npm の公開**: ジョブ `npm` が、`release` の後に `@mycroblog/mykit` をビルドし、`package.json` の版をリリースの版（`v` を除いた `X.Y.Z`）にして公開する。
  - 認証は npm の trusted publishing（GitHub Actions の OIDC。ジョブの権限 `id-token: write`）で、npm のトークンは秘密にも置かない。
    trusted publishing は npm CLI 11.5.1 以降・Node.js 22.14.0 以降が要り、ジョブは `.tool-versions` の Node.js とその npm を使って、版を確かめてから公開する。
  - `npm publish --provenance` で来歴（provenance）を付ける。npm の画面のパッケージの版に、ビルドしたワークフローとコミットが出る。
  - プレリリースの版（`vX.Y.Z-<識別子>`）は dist-tag `next` に、正式な版は `latest` に出す。`npm install @mycroblog/mykit` が入れるのは `latest` だけ。
  - 同じ版がすでに registry にあれば、その公開を飛ばす（落ちたジョブの回し直しのため）。npm は一度使った版の番号を、取り消した後も使わせない。
- **リリースノートの「mykit」の節**: ジョブ `release` が、下書きの Release の先頭に「mykit」の節（Go と npm の入れ方）を置く。公開する前に、節の末尾の注釈を次で置き換える。
  - 手書きの層（再試行・ページング・ストリーミング・トークン）の破壊的変更。Conventional Commits の `!` が付いたコミットから拾う（v0 の間は許す）
  - 生成したクライアントの型や関数で、この版で `Deprecated` にしたものと消したもの（「API の変更」の節と同じ操作）

  どちらもなければ「なし」と書く。
- **最初の公開の前の作業（決定者、1 回だけ。[#1167](https://github.com/mycroblog/my/issues/1167)）**: npm の trusted publisher はパッケージの設定に登録するので、
  パッケージがまだ registry にないうちは登録できない。
  1. npm の組織 `mycroblog` を作り、決定者を所有者にする。
  2. `@mycroblog/mykit` の最初の版を手元から 1 度だけ公開する（2FA を使う。来歴は付かない）。リポジトリの根で次を回し、`package.json` の変更は戻す。

     ```sh
     (cd app && bun install --frozen-lockfile)
     (cd pkg/mykit/ts && bun run build)
     scripts/release-libraries.sh npm-version v0.0.0-placeholder.0 pkg/mykit/ts
     (cd pkg/mykit/ts && npm publish --access public --tag next)
     git checkout -- pkg/mykit/ts/package.json
     ```

  3. npmjs.com のパッケージの Settings の「Trusted Publisher」で GitHub Actions を選び、Organization or user `mycroblog`、Repository `my`、
     Workflow filename `release.yml` を登録する（Environment は空のまま）。
  4. 同じ Settings の「Publishing access」を「Require two-factor authentication and disallow tokens」にする。trusted publishing は OIDC なので影響しない。

  登録の前に `release.yml` を回すと、ジョブ `npm` が認証で落ちる。登録した後に、そのジョブだけを回し直せばよい。

## 関門をブランチで試す

`release.yml` の変更は、プレリリースの版を使って、正式な版のタグを打たずにブランチで試せます。

```sh
gh workflow run release.yml -R mycroblog/my --ref <branch> -f version=v0.0.0-test.1
```

- 遅い検査が落ちるブランチで回すと、`release` のジョブが skip され、タグができないことを確かめられる（`gh api repos/mycroblog/my/git/ref/tags/v0.0.0-test.1` が 404）。
- 通るブランチで回すと、そのブランチの先頭にタグ（サーバーと `pkg/mykit/v0.0.0-test.1`）と下書きの Release ができ、
  `@mycroblog/mykit@0.0.0-test.1` が dist-tag `next` に来歴付きで公開される。npm の版の画面で来歴を確かめたら、タグと Release を消す。

  ```sh
  gh release delete v0.0.0-test.1 -R mycroblog/my --cleanup-tag --yes
  gh api -X DELETE repos/mycroblog/my/git/refs/tags/pkg/mykit/v0.0.0-test.1
  ```

  npm に出た版は、72 時間のうちなら `npm unpublish @mycroblog/mykit@0.0.0-test.1` で取り消せるが、その番号は二度と使えない。
  proxy.golang.org がすでに取り込んだ Go の版は、タグを消しても残る。試す版の番号は、正式な版と重ならない `v0.0.0-test.N` にする。

## 成果物

今の `release.yml` が作るのは、タグ（サーバーとライブラリ）と下書きの Release と、npm のパッケージです。コンテナイメージの公開とリリースノートは、M5（[#30](https://github.com/mycroblog/my/issues/30)）が `release.yml` の `release` のジョブの後に足します。
