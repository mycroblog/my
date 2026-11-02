# ADR-0129: 情報ページの AI の表明の法的な説明を短くする

- Status: Accepted
- Date: 2026-11-02
- Deciders: @rosylilly
- Issue: #808

## Context（背景・課題）

[ADR-0057](0057-crawler-opt-out.md) は、サーバーの情報ページ（FR-718）に出す検索と AI の表明の意味の説明について、
「日本の著作権法 30 条の 4 と EU DSM 指令 4 条（機械可読な TDM の留保）に対する位置づけを含む」と決めている。FR-725 も同じ位置づけを情報ページに出すと定めている。

人による文言の確認（#808）で、決定者はこの説明を、個別の法令に触れない次の文に直すと指定した。

> この表明が法的にどう扱われるかは国によって異なり、多くの事業者はこうした表明に自主的に従っています。この文章は法的な助言ではありません。

## Decision Drivers（判断基準）

1. 決定者が確認して指定した文言をそのまま使う
2. 法的な効果を保証する書き方をしない（ADR-0057）
3. `locales/en` と `locales/ja` で同じ内容を伝える（ADR-0009）

## Considered Options（選択肢）

1. 決定者の指定した文言に置き換え、個別の法令への言及をやめる
2. 法令への言及を残し、文だけを短くする

## Decision（決定）

選択肢 1 を採る。ADR-0057 の「サーバーの情報ページ」のうち、意味の説明に著作権法 30 条の 4 と EU DSM 指令 4 条に対する位置づけを含めるという部分を、この ADR で置き換える。

- 情報ページの法的な説明（`client:about.crawlers.legal`）の `ja` は、Context に引用した文とする。
- `en` は同じ内容の "What such a statement means in law depends on the country, and many operators follow such statements voluntarily. This text is not legal advice." とする。
- 説明では個別の国の法令に触れない。

ADR-0057 のそれ以外の決定は変えない。法的な効果を保証する書き方をしないことも、表明の手段（`/robots.txt`、`X-Robots-Tag`、`tdmrep.json` など）も、そのまま続く。

## Consequences（影響）

- 情報ページは、表明の法的な扱いが国によって異なることと、法的な助言ではないことだけを伝える。
- 国ごとの法的な位置づけは情報ページから読めなくなる。
- FR-725 の情報ページに出す内容を、この決定に合わせる。
