# Commands

`internal/commands` is the boundary between the recognized text and any action:

1. **Parse** (`Handler.Parse`): the text is normalized (lowercase, no accents, no punctuation, single spaces) and compared with a fixed list of phrases. The result is a typed `Command` (`Kind`, and a `Device` for device commands). Parse has no side effects.
2. **Execute** (`Handler.Execute`): runs the command and returns the spoken reply. Only `turn-on` and `turn-off` with a device from the allowlist reach a device controller.

The text never selects a program, a shell command, a URL or an ID. There is no language model.

| Kind | Phrases (after normalization) | Reply |
|---|---|---|
| `time` | "que hora es", "que horas son", "que hora son", "dime la hora", "sabes que hora es", "que hora es ya", and the Whisper errors "que horas", "que ahora es" | "Son las 10 y cuarto.", "Es la una en punto." (12-hour spoken Spanish) |
| `repeat` | "repite lo que he dicho", "repite", "repitelo" | "Has dicho: <your previous text>", or "Todavía no has dicho nada." |
| `cancel` | "cancela", "cancelar", "olvidalo" | "Vale, cancelado." |
| `help` | "que puedes hacer", "que sabes hacer", "ayuda" | the list of commands, and the allowlisted devices if there are any |
| `turn-on`, `turn-off` | "enciende", "prende" or "apaga", then a device name from the allowlist | "Hecho." or "No he podido encender la luz de la cocina." |
| `invalid` | "enciende", "prende" or "apaga" with no device, an unknown device, or extra words | "No me has dicho qué dispositivo." or `No conozco el dispositivo "horno".` No action. |
| `unknown` | everything else | "Todavía no sé hacer eso." No action. |

"Repite" repeats the last text that was not itself a repeat, so two repeats give the same text.

**Device allowlist.** A device has an `ID` for the controller (for example `light.kitchen`) and a spoken `Name` with its article (for example `la luz de la cocina`). `commands.New` rejects an allowlist with an empty ID or name, a name with characters other than letters, digits and spaces, a name that starts with a command word, or two names that are the same after normalization. The parser ignores a leading article, so "apaga luz del salón" and "apaga la luz del salón" both match. A device phrase that is not a name exactly goes through the misheard-word correction below: "enciende la luz de la cocina; rm -rf /" is `invalid`, with the kitchen light only as a suggestion.

**Misheard words.** When the text is not a command, a verb or a device name exactly, the parser takes the one that sounds most alike (`commands/match.go`). It compares the phrases without spaces, because Whisper joins and splits words, and after Spanish sound rules (z/c→s, v→b, silent h, ll→y, final m→n, and more). The score is 1 minus the edit distance divided by the longer length. Only names from the allowlist and the fixed phrases can be chosen.

| Score of the best guess | Result | Example (development names) |
|---|---|---|
| `accept` (0.8) or more, and `margin` (0.1) above the next device | the command runs | "Enciende el Udelsalom" → la luz del salón (0.82) |
| `suggest` (0.7) or more | `invalid`; the reply suggests one or two devices and nothing runs | "Enciende la luz del baño" → "No conozco el dispositivo "luz del bano". ¿Querías decir la luz del salón o la luz del techo?" |
| below `suggest` | `invalid` or `unknown`, as before | "Enciende el horno" |

`suggest` is above 0.69, the score of a wrong light that shares "luz del" with the right one. Change the values in `commands.matching` of the configuration file. Each guess is logged as `msg=guess heard=... meant=... score=... used=...`, at info level with `--log-transcripts` and at debug level otherwise.

**Device controller.** `commands.DeviceController` has one method, `SetPower(ctx, id, on)`. It receives only IDs from the allowlist. The unit tests use a fake controller that records each call and can fail. Without `--ha-url`, `run` has no allowlist and no controller, and device commands reply "Todavía no puedo controlar dispositivos." With `--ha-url`, the allowlist and the controller come from Home Assistant (see [home-assistant.md](home-assistant.md)).

**Allowlist from a source.** `Handler.UseSource` makes `Handle` read the allowlist again from a `DeviceSource` when the last read is older than a set time. When the source fails, the last list stays in use. `Handler.SetDevices` leaves out a device with a bad name (for example `Entrance Color + White Lights`) and reports it once per new list; the other devices stay usable. A name that starts with an article is spoken with a lowercase article: "La luz de la cocina" gives "No he podido encender la luz de la cocina."
