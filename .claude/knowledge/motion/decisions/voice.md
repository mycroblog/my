# motion: decisions, voice

The motion of a voice chat room: a reaction and the halo of who speaks, the values of `voice` in web/design-tokens.
One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A reaction rises its lane once in 3200 ms, at full for the middle of its time (12% to 70%), quick out of the bar and slowing as it fades (`ease-out`); with reduced motion it fades in and out where it is in the same time, at an even pace (`linear`), as a crossfade — a reaction is caught from the corner of the eye by one who reads or listens, so it stays long enough to be read once, and passes on its own since nobody answers it — the tokens `voice.duration` and `voice.ease` of #3225 as merged, their reason given in #3324 — 2026-11-20 — held 0 / missed 0
- The halo of who speaks swells and settles in 900 ms each way while they speak, eased at both ends (`ease-in-out`, alternating); with reduced motion it stays as the ring, still — at the pace of speech, it breathes with the voice instead of blinking, and the ring alone tells who speaks — the tokens `voice.duration` and `voice.ease` of #3225 as merged, their reason given in #3324 — 2026-11-20 — held 0 / missed 0
