# Home Assistant

With `--ha-url` and `VOICE_HA_TOKEN`, the assistant switches lights and switches in Home Assistant.

**Which devices.** The allowlist is the list of `light` and `switch` entities that are exposed to the Home Assistant voice assistant (Settings > Voice assistants > Expose; in the exposure settings this assistant is `conversation`). The spoken name is the entity name in Home Assistant. Give each device a name with its article for natural replies, for example "La luz de la cocina". Locks, covers, scripts and all other domains are never controlled, also when they are exposed. A device in the state `unavailable` (for example, unplugged) stays in the allowlist, but "¿Qué puedes hacer?" leaves it out; the log line `Home Assistant devices` marks it `(unavailable)`.

**New devices need no rebuild and no restart.** At start, and then before a command when the last read is older than 30 s, the client reads the exposed devices again. A device that you expose (or hide) in Home Assistant is usable (or rejected) at the latest 30 s later. The log line `Home Assistant devices` at start shows the count and the names.

**Security.**

- The client reads the exposure list over the WebSocket API (`homeassistant/expose_entity/list`) and the names over `GET /api/states`.
- It switches a device only with `POST /api/services/<light|switch>/<turn_on|turn_off>` and `{"entity_id": "<id>"}`. It checks the ID form (`domain.object_id`, lowercase) and the domain before each call.
- The token comes only from `VOICE_HA_TOKEN`, so it is not in the process list or the shell history. `config-check` shows only `(set)` or `(not set)`.
- A wrong token stops `run` at start with `Home Assistant rejected the access token`. When Home Assistant is down, `run` starts anyway, logs a warning, and tries again at the next command.
- Each Home Assistant request has a limit of 5 s.

**Development Home Assistant.** `server/compose.yaml` has a Home Assistant service (`ghcr.io/home-assistant/home-assistant:2026.10.2`, pinned by digest) in the profile `homeassistant`. It has the `demo` integration, which gives 6 lights and 2 switches. `make services-up` does not start it.

```sh
make ha-up      # start it, and on the first start: create the user, the token and the Spanish names
make run        # build, then run the assistant with client/.env.dev and the dev-token
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
- end to end: Piper says "Aleksei" and "Enciende la luz de la cocina", and the reply is "Hecho." with the light on in Home Assistant.

**Add a command.** Add a `Kind`, its phrases and its case in `Execute`, with tests. The pipeline does not change: it only calls `Handle(ctx, text)`.
