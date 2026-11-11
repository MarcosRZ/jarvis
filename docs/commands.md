# Commands

There is no fixed list of phrases. A sentence passes through these layers:

1. **Built-in intents.** Time, repeat, cancel, help and feedback. They are matched locally and deterministically, including a sound-alike correction for misheard words.
2. **Learned associations.** A single action that already succeeded for the same sentence.
3. **Action catalog.** A known verb plus the name, alias or entity ID of exactly one exposed entity. It runs locally, before Home Assistant and the model.
4. **Home Assistant conversation.** Handles the rest, including parameters and questions, using the configured agent.
5. **Local model.** Optional. It receives the safe action catalog and a read-only snapshot of exposed states and forecasts, and can choose up to four actions or answer a question from the snapshot.

The deterministic catalog always runs first. `VOICE_LLM_ORDER` chooses Home Assistant first (default) or the model first for the remaining sentences.

## Action catalog

`voice-client actions` prints the catalog generated from the exposed entities. Each action is data: a stable ID, one entity, one Home Assistant service and fixed arguments. There is no script, shell command, URL or free-form service in the catalog.

Supported domains include light, switch, input_boolean, fan, cover, valve, lock, scene, script, automation, button, climate, water_heater, media_player, vacuum, humidifier, siren, remote, timer and select. Read-only entities are visible to the model but have no actions.

The catalog is rebuilt at start and when the last read is older than 30 seconds. A new, removed or unavailable entity can take up to 30 seconds to change the local actions.

## Language model

`VOICE_LLM_MODE=fallback` asks the model only when the built-in parser, learned memory and catalog do not resolve the sentence. `off` disables it.

The model never receives the Home Assistant token, service names, URLs or unexposed entities. Its answer is constrained to the current action IDs or to `turn_on`/`turn_off` for an allowlisted light or switch. The client checks every answer again before running it.

`VOICE_ROOM` tells the model where the assistant is, so "here" means that room. The room is matched against device names; Home Assistant areas are not used.

## Safety

`VOICE_LLM_SAFETY` controls inferred actions:

| Value | Behavior |
|---|---|
| `guard` | Accept an action only when the sentence names the entity or action; risky domains also need a matching verb |
| `literal` | Accept any action the model returns |
| `states` | Accept only physical-state sentences, such as too much or too little light |

The default `guard` rejects chat and unclear sentences. A wrong inference is still possible, so unexpose any device the local model must not control.

## Learning and feedback

A local action counts as a success unless you give negative feedback within 60 seconds. The feedback phrase is currently `mal`.

Learned sentences are stored in `VOICE_LEARNING_FILE` with mode `0600`. A repeated success raises the score. Negative feedback sets the score to `-1`, so the action is not repeated. A different action replaces the association. Only actions are learned; replies are not, because states and forecasts change.

A sentence with several actions is not stored as one association. Set `VOICE_LEARNING_FILE=off` to disable learning.

## Logs

| Line | Meaning |
|---|---|
| `msg=heard` | Recognized text; only with transcripts enabled |
| `msg=catalog` | Deterministic catalog match |
| `msg=conversation` | Handled by Home Assistant |
| `msg=reasoned` | Action or reply accepted from the model |
| `msg=action` | Local device or catalog call |
| `msg=learning` | Learned association changed |
| `msg=guess` | Sound-alike correction |

Rejected model answers are logged as `reasoned rejected` at debug level.
