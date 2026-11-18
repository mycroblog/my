# ts-developer: decisions, forms

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A screen saves in one way: a control that saves at once takes its save state as a required prop (`save` of `Switch` and `SettingRow`, `null` while it has nothing to say), and a field of a form saved as a whole says so (`form` of `SettingRow`) and stands with no switch under the form's submit bar; what a type cannot hold is a lint of restrictions.js on the JSX one file writes (restrict/switch-in-form, settings-sections, heading-size, settings-count) and `settingsFindings` of @my/design-system/test/settings on the rendered tree, which counts what a list is mapped to — a check that a caller can forget is no check — ADR-0110 §6, the orchestrator's brief on #3198 — 2026-11-18 — held 0 / missed 0
