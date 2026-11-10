# Home Assistant

With `--ha-url` and `VOICE_HA_TOKEN`, the assistant builds a safe action catalog from the entities exposed to the Home Assistant `conversation` assistant and can control them locally: lights, switches, covers, climate, media players, locks, scenes and more. Sentences that the catalog does not resolve go to the Home Assistant conversation agent; if the agent does not answer a question, the local model can answer from a read-only snapshot of those exposed entities and their forecasts.

**Which devices.** Exposure is the allowlist (Settings > Voice assistants > Expose; in the exposure settings this assistant is `conversation`). The generated action catalog covers every exposed entity whose domain and supported features the client knows how to control: lights, switches, input booleans, fans, covers, valves, locks, scenes, scripts, automations, buttons, climate, water heaters, media players, vacuums, humidifiers, sirens, remotes, timers and selects. A read-only entity (sensor, weather, camera, binary sensor, and so on) has no actions; it is only part of the model context. An entity in the state `unavailable` has no actions until it comes back. The spoken name is the entity name; give each device a name with its article for natural replies, for example "La luz de la cocina". The local light/switch allowlist is still used for the deterministic light-state phrases ("no veo nada", "hay demasiada luz") and for learned light/switch commands; the log line `Home Assistant devices` marks an unavailable light or switch `(unavailable)`.

Every other exposed entity and every sentence that the catalog does not resolve is sent to the Home Assistant conversation agent, which also answers questions about exposed entities, such as weather. If it does not handle a question, the local model can answer from the read-only state/forecast snapshot (up to 50 entities and 4 forecast entries). The available actions and parameters there are the ones that agent understands; see [commands.md](commands.md#home-assistant-conversation-fallback). Home Assistant aliases and areas work there, and the local device names are not needed.

Expose the weather entity to `conversation`, not only to Home Assistant's Amazon Alexa integration (`cloud.alexa`), or neither Home Assistant Assist nor this client can see it. `cloud.alexa` here is the Home Assistant integration; it has nothing to do with this client's "alexa" wake word.

**Action catalog.** At start and before a command when the last read is older than 30 s, the client reads the exposed entities and generates the actions it can run safely. An action is data, not a shell command:

- `id`: stable ID, `<domain>.<object_id>.<suffix>`, for example `media_player.lg_tv.volume_up` or `scene.movie.turn_on`. It is the only value that the language model may return.
- `name` and `description`: the Spanish text shown to the model, with the entity ID and aliases (`tele`, `luz`, `escena`, and so on).
- `entity_id`, `domain` and `service`: the fixed Home Assistant call, for example `media_player.volume_up`.
- `data`: fixed arguments, for example `{"is_volume_muted": true}` or `{"hvac_mode": "heat"}`. The model never writes these arguments.
- `aliases`: extra Spanish words for matching.

The catalog stays in memory (`Handler.UseActionSource`). `voice-client actions` writes a snapshot to `VOICE_ACTIONS_FILE` (default `client/data/actions.json`; the directory is git-ignored) and prints the action count and the action list. The snapshot is for inspection; the assistant always regenerates the catalog from the live exposure list.

**How a sentence uses the catalog.** After the fixed parser, learned memory and the deterministic light-state phrases, `Handler` tries a deterministic match against the catalog. It needs a known verb (encender, poner, activar → turn_on; apagar, quitar → turn_off; alternar → toggle; silenciar → mute; subir el volumen → volume_up; pausar → pause; and so on) and the name or an alias of exactly one entity in each clause joined by "y" or "e". A unique clause match runs at once, before Home Assistant and before the model; all matched actions run in order. A single-action sentence is learned; a sentence with several actions is not stored as one association. If a clause is ambiguous or uses another verb, the local model gets the whole catalog and the read-only context and may return one or more action IDs, up to four. The handler checks each ID against the current catalog and the safety guard (the sentence must name the entity or describe a matching physical state); `ExecuteAction` validates the action again before the service call. A model cannot invent a service, an entity or an argument. Say "mal" to punish the last single-action association; the catalog matcher and the model then stop reusing it. The Home Assistant conversation agent has its own state and is not affected by the local learned store.

**Refreshing.** The catalog is regenerated with the exposure list before a command when the last read is older than 30 s. A newly exposed device is actionable at the latest 30 s later; a removed or unavailable entity stops being a valid action. The catalog, the light/switch allowlist and learned commands use only the exposed entities, so an unexposed device cannot be selected by the model.

**New devices need no rebuild and no restart.** At start, and then before a command when the last read is older than 30 s, the client reads the exposed entities and rebuilds the light/switch allowlist and the action catalog. A device that you expose (or hide) is usable (or rejected) by the local paths at the latest 30 s later. The conversation agent reads the exposure list itself, so its changes apply at once. The log line `Home Assistant devices` shows the count and the names of the local lights and switches; `Home Assistant actions` shows the generated action count.

**Security.**

- The client reads the exposure list over the WebSocket API (`homeassistant/expose_entity/list`), the names and states over `GET /api/states`, and forecasts with `POST /api/services/weather/get_forecasts?return_response` for exposed weather entities. Those reads are passed to the local model as text without the token; unexposed entities are left out.
- It runs a catalog action only with `POST /api/services/<domain>/<service>` and a body of `{"entity_id": "<id>"}` plus the fixed `data` of the action. Before each call it checks the action ID, the entity ID form (`domain.object_id`, lowercase), the service allowlist and the arguments. The direct light/switch path is limited to exposed lights and switches.
- The language model never sends a service name, an entity ID or an argument. It can only return an action ID from the current catalog, and the handler and `ExecuteAction` validate it again. The catalog is generated only from the exposed entities.
- Any other sentence is sent as text to `POST /api/conversation/process`, with `VOICE_STT_LANGUAGE` and, when set, `VOICE_HA_AGENT`. Home Assistant applies its exposure rules and its conversation agent decides the action. The client does not send a service name or a token to the model, and it does not build arbitrary service calls. If the agent is cloud-based, the sentence leaves your network.
- The token comes only from `VOICE_HA_TOKEN`, so it is not in the process list or the shell history. `config-check` shows only `(set)` or `(not set)`.
- A wrong token stops `run` at start with `Home Assistant rejected the access token`. When Home Assistant is down, `run` starts anyway, logs a warning, and tries again at the next command.
- Each local Home Assistant request has a limit of 5 s. A conversation request uses `--operation-timeout` (default `30s`), because an LLM agent can take longer than a light call.

**Development Home Assistant.** `server/compose.yaml` has a Home Assistant service (`ghcr.io/home-assistant/home-assistant:2026.10.2`, pinned by digest) in the profile `homeassistant`. It has the `demo` integration, which gives 6 lights and 2 switches. `make services-up` does not start it.

```sh
make ha-up      # start it, and on the first start: create the user, the token and the Spanish names
make run ENV=dev  # build, then run the assistant with client/.env.dev and the dev-token
make ha-down    # stop and remove the container; the data stays in server/homeassistant
make ha-reset   # also delete the data, except configuration.yaml, for a new first start
```

On the first start, `client/cmd/ha-dev-setup`:

1. Creates the user `dev` with a random password, and completes the onboarding.
2. Saves a long-lived token (3650 days) to `server/homeassistant/dev-token` and the password to `server/homeassistant/dev-password`, both with mode 0600. Both are git-ignored, and the tool prints neither.
3. Gives the demo devices Spanish names: La luz de la cocina, La luz del salón, La luz del dormitorio, La luz del techo, La luz del despacho, La luz de la entrada, Las luces decorativas, El aire acondicionado.

On the next starts it only checks that the saved token works. Open http://127.0.0.1:8123 and log in as `dev` with the password from `dev-password` to change the exposure or the names.

**Tests.** `make integration` runs the Home Assistant tests when `server/homeassistant/dev-token` exists, and skips them otherwise:

- the exposed devices are listed;
- a device that is un-exposed is gone from the list, and comes back when it is exposed again, with no restart;
- `SetPower` switches a light off, on and off, checked with its state;
- `Actions` and `ExecuteAction` use the exposed entities and a safe service call;
- `Converse` sends a harmless sentence to the default conversation agent, and `Handle` speaks its answer;
- end to end: Piper says "Alexa" and "Enciende la luz de la cocina", and the reply is "Hecho." with the light on in Home Assistant.

**Add a command.** For a new fixed command, add a `Kind`, its phrases and its case in `Execute`, with tests. A new controllable device domain needs a generator in `client/internal/actions/actions.go` and its service in the allowlist; expose it and it gets safe actions after the next refresh. A domain without a generator can still be controlled through the conversation agent. The pipeline does not change: it only calls `Handle(ctx, text)`.
