# ドキュメント目次

## 要件

- [ビジョンとスコープ](requirements/00-vision.md)
- [機能要件（v0）](requirements/01-functional.md)
- [非機能要件](requirements/02-non-functional.md)
- [用語集](requirements/03-glossary.md)

## 設計

- [アーキテクチャ概要（基礎設計）](architecture/overview.md)
- [論理データモデル](architecture/data-model/README.md)
- [公開範囲の閲覧判定（canView）](architecture/visibility.md)
- [連合の設計（ActivityPub S2S）](architecture/federation.md)
- [デプロイターゲットと低コスト構成](architecture/deployment-targets.md)
- [Web クライアントの設計規約](architecture/web-client/README.md)
- [ADR 一覧](adr/README.md)

## リファレンス

- [設定リファレンス](reference/configuration.md)（設定スキーマから生成）
- [ビルドタグ](reference/build-tags.md)（アダプタをバイナリから外すタグと、その検査）
- [コマンド](reference/commands.md)（go-task のタスクと、固定しているツールチェーン）
- [webhook リファレンス](reference/webhooks.md)（受け手が受け取るヘッダ・署名・イベント・本文）
- [サードパーティのアプリの開発者向けの手引き（英語）](reference/third-party-apps.md)（アプリの登録・認可の流れ・スコープ・レート制限・API の安定性）

## 運用

- [管理者とロール](operations/admin-cli.md)
- [インスタンス設定](operations/instance-settings.md)
- [利用者登録（登録モード・招待・承認・メール確認）](operations/registration.md)
- [ボット対策の CAPTCHA（Turnstile / hCaptcha / reCAPTCHA）](operations/captcha.md)
- [ドメイン単位の処分と通報の対応](operations/moderation.md)
- [鍵暗号化鍵（KEK）の管理](operations/key-management.md)
- [DB の初期化と作り直し](operations/database.md)
- [サーバーの起動・停止とヘルスチェック](operations/running.md)
- [リンクプレビュー（OGP / X Card / fediverse:creator）](operations/link-previews.md)
- [検索エンジンと AI クローラー（robots.txt・noindex・AI の拒否の表明）](operations/crawlers.md)
- [サーバー単位の webhook](operations/webhooks.md)
- [連合リレー](operations/relays.md)
- [リリースの手順](operations/releasing.md)
- [クラウドの利用料金の取り込み（AWS Cost Explorer、Google Cloud の BigQuery への export）](operations/billing-import.md)
- [サポーターの決済（Stripe）](operations/payments-stripe.md)
- [サポーター（課金のプラン）](operations/supporters.md)

## 開発プロセス

- [コントリビュートの手引き](../CONTRIBUTING.md)
- [エージェント向け指示](../AGENTS.md)
- [Issue 作業手順（playbook）](agents/workflow.md)
- [ADR の書き方（playbook）](agents/writing-adrs.md)
