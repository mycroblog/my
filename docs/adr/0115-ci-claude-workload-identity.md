# ADR-0115: CI から Claude API への認証に Workload Identity Federation を使う

- Status: Accepted
- Date: 2026-10-28
- Deciders: @rosylilly
- Issue: #1674

## Context（背景・課題）

CI から Claude API を使うジョブは 2 つある。nightly の L4 の課題テスト（ADR-0114）と、リリースの draft の本文を書くジョブ（`release.yml` の `release-notes-prose`）である。ADR-0114 は、その API キーを `main` だけが読める GitHub の environment に置くと決めた。
Anthropic は、GitHub Actions のジョブが GitHub の発行する OIDC のトークンを短い有効期間の Anthropic のアクセストークンに交換する Workload Identity Federation（WIF）を提供している。
これを使えば、静的な API キーをリポジトリにも environment にも置かずに済む
（2026-10-28 に [Use WIF with GitHub Actions](https://platform.claude.com/docs/en/manage-claude/wif-providers/github-actions) で確認）。
決定者は、静的なキーではなく WIF を使うよう求めた。

## Decision Drivers（判断基準）

1. 漏れても長く使える秘密情報を持たないこと。
2. Claude API を使えるジョブを、このリポジトリの `main` の特定のジョブに限れること。フォークの PR や他のブランチから使えないこと。ジョブごとに規則を分け、片方を止めてももう片方が動くこと。
3. 費用に上限を設けられること。

## Considered Options（選択肢）

1. **静的な API キー**を、`main` だけが読める environment の secret に置く（ADR-0114 の書き方）。
2. **WIF**: GitHub の OIDC のトークンを、Anthropic のフェデレーションの規則で短い有効期間のアクセストークンに交換する。

## Decision（決定）

**選択肢 2 を採る。** CI から Claude API を使うジョブはすべて WIF で認証し、`ANTHROPIC_API_KEY` を GitHub に置かない。ADR-0114 のうち、API キーを environment に置くことを置き換える。

- **ジョブの範囲**: Claude API を使うジョブは、それぞれ専用の environment で動き、`permissions: id-token: write` を持つ。どの environment も、デプロイできるブランチは `main` だけである。
  - `nightly.yml` の `l4`: environment `ux-nightly`。
  - `release.yml` の `release-notes-prose`: environment `release-notes`。ブランチでの試しのリリースでは動かず、draft は事実だけになる。
- **Anthropic の側**（決定者が Claude Console の Workload identity で設定する）:
  - 発行者は `https://token.actions.githubusercontent.com` を discovery の方式で登録する。
  - CI 専用のワークスペースとサービスアカウントを作り、ワークスペースに使用額の上限を設ける。上限が、CI 全体の Claude API の費用の上限になる。2 つのジョブは同じワークスペースとサービスアカウントを使う。
  - フェデレーションの規則は environment ごとに 1 つ作る。`subject_prefix` を `repo:mycroblog@346242925/my@1421587381:environment:<environment>`（GitHub がこのリポジトリに発行する `sub` は、所有者とリポジトリの数値 ID を含む形である）、`audience` を `https://api.anthropic.com` にし、
    `claims` で `repository_owner: mycroblog` と `ref: refs/heads/main` も確かめる。トークンの有効期間は短く（例 600 秒）する。
- **GitHub の側**: 規則・組織・サービスアカウント・ワークスペースの ID は秘密ではないので、各 environment の variable に置く
  （`ANTHROPIC_FEDERATION_RULE_ID`・`ANTHROPIC_ORGANIZATION_ID`・`ANTHROPIC_SERVICE_ACCOUNT_ID`・`ANTHROPIC_WORKSPACE_ID`）。
- **トークンの受け渡し**: ジョブは GitHub の OIDC のトークンを `ANTHROPIC_IDENTITY_TOKEN_FILE` のファイルに書き、Claude Code が交換する。
  GitHub のトークンは約 5 分で切れるので、ジョブはファイルを 2 分ごとに取り直す。
- **信頼しない入力を読むジョブ**: `release-notes-prose` は PR と Issue の本文を読むので、モデルの入力を信頼しない。書き込みの権限を持たず、モデルに道具を与えず、環境変数のうち `ANTHROPIC_` で始まるものだけを Claude Code に渡す。出力は機械が検査してから別のジョブが draft に入れ、決定者が読んで公開する。

### 見直す条件

- 使うエージェントの道具（Claude Code）が WIF の交換に対応せず、包む手間が大きい場合。そのときは、交換したアクセストークンを、そのジョブの中でだけ環境変数として渡す。静的なキーには戻さない。

## Consequences（影響）

- 良い影響: GitHub に Claude API の秘密情報がなく、漏れて長く使われるおそれがない。使えるジョブは規則で `main` の environment に限られる。
- 良い影響: 専用のワークスペースの上限で、費用が上限を超えない。
- 悪い影響: 上限を 2 つのジョブで分け合うので、nightly が上限まで使うと、リリースの本文の呼び出しは失敗し、draft は事実だけになる。リリースは止まらない。
- 悪い影響: 設定が Claude Console と GitHub に分かれ、規則の `sub` の形を間違えると交換が `401` で失敗する。失敗の理由は Claude Console の認証の履歴で見る。
- 後続の作業: #1674（nightly の L4 のジョブ）と #2108（リリースの本文）がこの方式で認証する。Anthropic と GitHub の設定は決定者が行う。

## Pros and Cons of the Options（詳細比較・任意）

- **選択肢 1**: 設定が GitHub の secret 1 つで済み、どの道具でもそのまま使える。ただしキーは漏れると取り消すまで使え、`main` に限る仕組みも GitHub の environment の設定にだけ頼る。
