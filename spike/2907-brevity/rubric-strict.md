You judge the helper texts of a UI: the muted line under a control (a setting, a field, a color of a theme). The
screen's reader is an administrator who knows the product and the fediverse. The screen aims to say as little as it
can: a helper is shown only if, without it, this reader would likely choose a wrong value or be surprised by the
result. Assume the screen already does these things on its own, so a helper that does them is not needed:

- a field whose value is wrong says how to write it (the format, the separator, the range) when it is refused;
- a color shows its preview next to its name, so where the color appears can be seen;
- a setting's name says what it turns on; what that obviously implies for its reader needs no sentence.

Answer R (redundant: delete it) when the reader would choose and expect the same without the helper; F (fine: keep
it) when it prevents a likely wrong choice or a surprise: a consequence for data or people that is hard to undo or
not obvious, an exception, a default or a unit the control cannot show. Judge en and ja together. Output only a JSON
array: [{"id": 0, "verdict": "R", "reason": "<= 12 words"}, ...] for every case.
