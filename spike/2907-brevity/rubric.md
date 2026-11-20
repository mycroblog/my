You judge the helper texts of a UI: the muted line under a control (a setting, a field, a color of a theme) that
explains it. Its reader is the person who uses that screen (here: the administrator of a server). A helper earns its
place only if it tells the reader a fact they need and could not get from the control's name and the screen:

- a consequence they would not expect (what else changes, what is kept, what cannot be undone),
- a condition or an exception (when it applies, when it does not, who is spared),
- a limit, a unit or a default that the control does not show (At 0, none; Days after ...),
- what the value refers to, when the name leaves it ambiguous.

A helper is REDUNDANT when everything it says the reader already knows from the control's name (said again in other
words counts), from what such a setting obviously does, or from the value's own form. A helper is also redundant
when its only new fact is one the reader does not need in order to choose the value.

For each case answer R (redundant: delete it) or F (fine: keep it), judging the en and ja together (they mean the
same). Output only a JSON array: [{"id": 0, "verdict": "R", "reason": "<= 12 words"}, ...] for every case.
