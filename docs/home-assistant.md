# Home Assistant

Jarvis uses Home Assistant for the device list, the action catalog and the conversation fallback.

## Setup

1. Create a long-lived token: Profile > Security > Long-lived access tokens.
2. Put the URL and token in `client/.env`:
   ```sh
   VOICE_HA_URL=http://homeassistant.local:8123
   VOICE_HA_TOKEN=...
   ```
3. Expose the entities Jarvis may control: Settings > Voice assistants > Expose.
4. Optional: choose a conversation agent with `VOICE_HA_AGENT`, for example `conversation.ollama`. Empty uses the Home Assistant default.

The exposed entities are both the action catalog and the context the local model may see. Unexpose anything the assistant must not control.

## What runs where

| Sentence | Handled by |
|---|---|
| Built-in intent (time, repeat, cancel, help, feedback) | Local parser |
| Known verb + one exposed entity | Local action catalog |
| Previously learned single action | Local learning store |
| Parameter, mode, channel, setpoint, question | Home Assistant conversation agent |
| Still unresolved | Local model, then Home Assistant, or the reverse with `model-first` |

A deterministic catalog match always runs before Home Assistant and the model. The conversation fallback is stateless: it does not keep a conversation ID, so follow-up questions are not supported.

If the conversation agent is cloud-based, the sentence leaves your network. The local model and the local action catalog do not.

## Development Home Assistant

`make ha-up` starts a Home Assistant container with demo entities and writes a token to `server/homeassistant/dev-token`. Use it only for development and tests.

```sh
make ha-up
make run ENV=dev
make ha-down
```

`make ha-reset` deletes the development data except `configuration.yaml`.

The development login and token are stored in plain files under `server/homeassistant`. Do not expose that directory or port outside your machine.
