# Commands

`internal/commands` is the boundary between the recognized text and any action:

1. **Parse** (`Handler.Parse`): the text is normalized (lowercase, no accents, no punctuation, single spaces) and compared with a fixed list of phrases. The result is a typed `Command` (`Kind`, and a `Device` for device commands). Parse has no side effects.
2. **Execute** (`Handler.Execute`): runs the command and returns the spoken reply. Only `turn-on` and `turn-off` with a device from the allowlist reach a device controller.

The text never selects a program, a shell command, a URL or an ID. The fixed parser needs no model; the optional [language model](#language-model) can only choose `turn_on`/`turn_off` for a device from the allowlist.

| Kind | Phrases (after normalization) | Reply |
|---|---|---|
| `time` | "que hora es", "que horas son", "que hora son", "dime la hora", "dime que hora es", "me dices la hora", "puedes decirme la hora", "sabes que hora es", "que hora es ya", and the Whisper errors "que horas", "que ahora es" | "Son las 10 y cuarto.", "Es la una en punto." (12-hour spoken Spanish) |
| `repeat` | "repite lo que he dicho", "repite", "repitelo" | "Has dicho: <your previous text>", or "Todavía no has dicho nada." |
| `cancel` | "cancela", "cancelar", "olvidalo" | "Vale, cancelado." |
| `help` | "que puedes hacer", "que sabes hacer", "ayuda" | the list of commands, and the allowlisted devices if there are any |
| `turn-on`, `turn-off` | "enciende", "prende" or "apaga", then a device name from the allowlist; also a state or feeling that the [language model](#language-model) or a learned sentence turns into an action, for example "hay demasiada luz aquí" | "Hecho." or "No he podido encender la luz de la cocina." |
| `penalty` | "mal", "esta mal", "eso esta mal", "te has equivocado", "equivocado", "no es eso", "incorrecto" | "Lo siento. Lo tendré en cuenta." or "No hay nada que corregir." |
| `invalid` | "enciende", "prende" or "apaga" with no device, an unknown device, or extra words | "No me has dicho qué dispositivo." or `No conozco el dispositivo "horno".` No action. |
| `unknown` | everything else | "Todavía no sé hacer eso." No action, unless the [language model](#language-model) maps it to a device command. |

"Repite" repeats the last text that was not itself a repeat, so two repeats give the same text. Filler words such as "por favor" are ignored, so "Apaga el ventilador, por favor" is the same command as "Apaga el ventilador".

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

**Several clauses.** `ParseAll` tries the whole text first, so a device name that contains " y " still matches. If the whole text is not a valid command, it splits on "y" and "e" and keeps the parts only when every part is a valid command: "Enciende la luz de la cocina y apaga la luz del salón" runs both in order. Fixed actions combine the same way: "¿Qué hora es y qué puedes hacer?" gives the time and then the help text. If any part is not valid, the whole text stays one command and may go to the language model.

## Language model

The fixed parser is fast, deterministic and stays first. With `VOICE_LLM_URL` set, a local llama.cpp server (`server/compose.yaml`) can interpret a single sentence that the parser does not understand. `VOICE_LLM_MODE` sets when it is asked:

| Mode | When the model runs |
|---|---|
| `off` | never |
| `fallback` (default) | when `ParseAll` returns one `unknown` or `invalid` command |
| `always` | every single sentence with a clear candidate device, and any `unknown` sentence about a device or a physical state; several parsed clauses stay deterministic, and if the model fails or returns nothing the fixed parser decides |

The model runs in two situations:

1. **A command with a device.** The handler takes the words after the power verb ("enciende", "prende", "apaga", "encender", "apagar") and matches them against the allowlist. It asks the model only when one device is a clear match, and sends that device as the only choice. The action runs at once, as before.
2. **A state without a power verb.** A sentence such as "hay demasiada luz aquí", "está muy oscuro", "hace frío" or "no veo nada" has no device name. If it contains a word from a small physical-state list (light, dark, bright, cold, heat, air, humidity, "veo"), the handler can infer an action. The common light phrases ("no veo nada", "no veo bien", "no hay luz", "no puedo ver", "está muy oscuro", "hay poca luz", "necesito luz", "hay demasiada luz", "me sobra luz", "está muy claro", "hay mucho brillo", "me deslumbra") are resolved deterministically: the light whose name contains `VOICE_ROOM` is chosen and the action runs at once. This works even if the model is off and avoids the small model reading "no" as turn_off. For any other state the handler sends every available device and lets the model infer an action and a device. The action runs at once, like a command that names the device; the model's prompt says to return one action for a problem or a feeling. The assistant keeps no conversation state: there is no confirmation question.

`VOICE_ROOM` (`--room`) tells the model where the assistant is, for example `Salon`. It resolves "aquí" and "en esta habitación" to that room. The examples in the prompt use the light whose name contains the room ("la luz del salón"), which is how the model picks a device for "hay demasiada luz aquí". Without `VOICE_ROOM`, the model may still interpret a state, but it does not know the room. The room is matched against the device name; Home Assistant areas are not used yet.

The fixed commands (time, repeat, cancel, help) stay deterministic: the model is not asked for them, and if it returns one the handler discards it. An invalid command with a power verb but no clear device (for example "enciende el ventilador", when there is no such device) keeps the parser's reply instead of going to the model.

The prompt contains turn_on/turn_off and the allowlisted devices as IDs and spoken names. A JSON schema constrains the answer to those actions and IDs, so the model cannot name another action, service or entity. The client checks the answer again against the allowlist, and the handler checks it against the candidates. The model never receives the Home Assistant token, a service name, a URL or a shell command. An answer with an unknown action or device is discarded. Up to four device actions run in order, and an identical reply is said once. Actions inferred from a state run without a confirmation, so a wrong inference will act on the wrong device; the allowlist limits the damage to the lights and switches that you exposed.

`fallback` adds no latency to commands that already work; one model answer takes about 2 to 10 s on a small CPU. `VOICE_LLM_TIMEOUT` (default `30s`) bounds an answer. If the model is down, the error is logged and the fixed parser is used. The log line `msg=interpreted` shows each action chosen for a command; `msg=reasoned` shows an action inferred from a state, at info level with `--log-transcripts` and at debug level otherwise. The info-level `msg=action` line records every real call that "Hecho." refers to, with `source` (`parser`, `interpreted`, `reasoned`, `state` or `learned`), `kind`, `device`, `name`, `ok` and `duration`; errors add `error`. The settings are in [configuration.md](configuration.md).

## Learning and feedback

An action inferred from a state is a success when no punishment follows. The handler stores the normalized sentence without fillers, the device and the action in a JSON file (`VOICE_LEARNING_FILE`, default `client/data/learning.json`; the file is written atomically with mode 0600, and its directory is git-ignored). The same sentence then runs from the local store without asking the model, which removes the 2 to 10 s model latency. A repeated success adds one point; a different action replaces the association and starts again at one point.

Say the wake word and then "mal" within 60 seconds to punish the last action. The handler undoes the success counted when the action ran and sets the entry score to -1, so even a long success streak cannot keep the punished action in use. A negative entry is not reused, and its action is dropped from the model's answer: the assistant will not repeat the punished action for that sentence. If the model chooses another action, the association is replaced; if it returns nothing, the reply is "Todavía no sé hacer eso." This is how a correction changes the learned behavior. "Mal" does not undo the action already sent to Home Assistant.

The store is keyed by the whole normalized sentence, so a rephrasing is a new entry. Only actions with `source=interpreted`, `reasoned`, `state` or `learned` are learned; a deterministic parser command is not. The logs show `msg=learning result=success|penalty score=... successes=... failures=... device=... kind=...`, `msg=feedback result=penalty ... learned=...`, and `msg=reasoned source=learned score=...` when a learned sentence is used. The text itself is not in these lines; the `msg=heard` line has it when transcripts are enabled.
