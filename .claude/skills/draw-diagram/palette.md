# Diagram palette

## Boxes and text

| Class | Use | Light (fill / stroke) | Dark (fill / stroke) |
| --- | --- | --- | --- |
| `bg` | the canvas | `#ffffff` | `#0d1117` |
| `t`, `m`, `h` | label, note, heading text | `#1f2328`, `#59636e`, `#1f2328` | `#e6edf3`, `#9198a1`, `#e6edf3` |
| `group` | a container of boxes | `#f6f8fa` / `#d1d9e0` | `#151b23` / `#3d444d` |
| `client` | who connects from outside | `#dafbe1` / `#1a7f37` | `#0f2d1b` / `#3fb950` |
| `proxy` | infrastructure in front | `#ffffff` / `#59636e` | `#0d1117` / `#9198a1` |
| `proc` | the `my` processes | `#ddf4ff` / `#0969da` | `#0c2d6b` / `#4493f8` |
| `port` | a part chosen by configuration | `#ffffff` / `#8c959f` | `#0d1117` / `#656c76` |
| `svc` | an outside service | `#fbefff` / `#8250df` | `#2a1650` / `#ab7df8` |
| `danger` | a failure, a refusal, a loss, and what follows it | `#ffebe9` / `#cf222e` | `#3c0d12` / `#f85149` |
| `danger-text` | text that is red without a box | `#cf222e` | `#f85149` |

`stroke-width` 1.5 for quiet boxes and 2 for the main ones; `rx="8"` for boxes, `12` for groups and the canvas.

## Lines

Lines that carry different things look different. Give each style its own arrowhead marker with the matching fill, and
say in the prose what the styles mean when a diagram uses more than one.

| Class (head) | Use | Style | Light | Dark |
| --- | --- | --- | --- | --- |
| `arrow` (`head`) | the usual flow | solid, 2px | `#59636e` | `#9198a1` |
| `signal` (`signal-head`) | a signal or control message | dashed `6 4`, 2px | `#0969da` | `#4493f8` |
| `data` (`data-head`) | data that flows | solid, 3px | `#1a7f37` | `#3fb950` |
| `reply` (uses `head`) | a reply to a request | dashed `8 5`, 2px | `#59636e` | `#9198a1` |
| `arrow danger` (`head danger`) | a failure | solid, 2px | `#cf222e` | `#f85149` |

Examples: `docs/operations/images/voice-methods.svg` and `supporter-flow.svg` (signal, data),
`docs/developers/images/token-flow.svg` (reply).
