# ADR-0008: Web クライアントとモバイルアプリ

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #10

## Context

Web と、Tauri 2 もしくはネイティブのモバイルアプリを提供したい（G6）。多言語対応（G5）とコード共有のしやすさも重要。

## Considered Options（モバイル）

| 選択肢 | 長所 | 短所 |
| --- | --- | --- |
| **Tauri 2（Web UI 共有）** | Web クライアントの UI・i18n・API クライアントをほぼそのまま再利用。Rust でネイティブ機能拡張可能 | モバイル対応は比較的新しい。リモート Push は公式プラグインがなくコミュニティ製（APNs / FCM）に依存（要検証）。WebView 差異 |
| ネイティブ（Swift / Kotlin） | 最高の UX・OS 統合 | 2 コードベース + Web で 3 重実装、i18n カタログの共有に変換が必要 |
| React Native / Expo | 単一コードベースでネイティブ UI、Push が成熟 | Web UI とは別実装になりやすい |

## Decision

- **Web クライアント**: TypeScript の SPA を PWA として提供（ツールチェーンは Bun。サーバーは Go なので、API の型は OpenAPI 等から生成する）（インストール可能、Web Push、iOS はホーム画面追加で Push 可）。
  静的アセットとしてビルドし、各ターゲットの CDN / 静的配信（Cloud Storage + CDN、S3 + CloudFront、Go バイナリへの `embed` 同梱）から配る。
  UI フレームワークは別 Issue で選定（判断基準: エージェントの習熟度、バンドルサイズ、i18n ライブラリとの相性、Tauri との相性）。
- **モバイル**: **Tauri 2** で Web クライアントの UI を共有する方針を第一候補とする。モバイルアプリは v0 の範囲に含めない。
  既存の Mastodon アプリは使えない（ADR-0007）ため、v0 の間のモバイル体験は **PWA が担う**。PWA の完成度を v0 の必須要件とする。
  Push 通知・バックグラウンド処理・WebView の性能が要件を満たさない場合は、ネイティブ（または RN）に切り替える ADR を別途起こす。
- Web UI は最初から「モバイルファースト・タッチ操作・セーフエリア・オフライン耐性」を前提に作る（Tauri 移植のため）。

## Consequences

- UI・i18n・API クライアントの実装は 1 つで済む。
- Tauri モバイルの成熟度リスクを v0 では負わない。

## Open Questions / 要検証

- [ ] #31 Spike: Tauri 2 で Web クライアントをラップし、iOS / Android で APNs / FCM のリモート Push を受信できるか
- [ ] UI フレームワーク選定（React / Solid / Svelte 等）
