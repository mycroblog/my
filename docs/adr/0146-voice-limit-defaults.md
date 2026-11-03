# ADR-0146: 通話の上限の既定値

- Status: Accepted
- Date: 2026-11-03
- Deciders: @rosylilly
- Issue: #2073

## Context（背景・課題）

[ADR-0101](0101-voice-chat-transport.md) §1 は `voice_max_participants`・`voice_max_room_duration`・`voice_max_open_rooms` の既定値を `0`（上限なし）にした。
有料のメディアサーバーを使うと、誰も設定しないままルームが開き続けて費用に上限がない。

## Decision Drivers（判断基準）

- 設定を読まずに動かしても、費用に上限があること
- 小さなインスタンスの普通の使い方を妨げないこと

## Considered Options（選択肢）

1. 既定は上限なしのまま、文書で設定を勧める
2. 3 つすべてに既定の上限を置き、`0` で外せるようにする

## Decision（決定）

選択肢 2。ADR-0101 §1 の表のうち、この 3 つの既定値だけをこの ADR が置き換える。

| キー | 既定 |
| --- | --- |
| `voice_max_room_duration` | `4h` |
| `voice_max_participants` | `50` |
| `voice_max_open_rooms` | `10` |

`0` は上限なしを意味する。

## Consequences（影響）

- メディアサーバーの費用は、既定で「10 ルーム × 50 人 × 4 時間」で頭打ちになる。
- 4 時間を超える通話は既定で終わる。必要なインスタンスは設定を変える。
