# motion: decisions, figures

A figure that moves on its own to explain something, its time and its easing, the values of `figureMotion` in
web/design-tokens. One line each:
`- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- The fediverse's figure sends a post along a follow every 4.5 s, the two of a 9 s cycle half a cycle apart; each fades in at its author in 300 ms, travels 1.8 s with cubic-bezier(0.4, 0, 0.2, 1), and fades out at the follower it reaches in 300 ms, and nothing moves the rest of the cycle — a loop without end must be still most of the time to leave the eye on the way to an account; a trip from place to place starts and stops gently, so it eases both ways, unlike a screen that enters or leaves; long enough to follow a post's path, short enough to read as sent — the decider's decision on #3080 (loop without end), the values the visual role's — 2026-11-20 — held 0 / missed 0
