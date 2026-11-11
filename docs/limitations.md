# Known limitations

- The bundled wake-word models are English. Other wake words need a custom model.
- Wake-word model licenses are CC BY-NC-SA 4.0: non-commercial use only.
- The built-in intent parser is configured for one language. Other languages rely on Home Assistant or the local model for those intents.
- Recognition accuracy depends on the model and the language. A larger model is more accurate and slower.
- The wake-word threshold is one server value for all clients.
- The end of speech uses a fixed level, not a voice detector. Steady noise can keep the recording open until `VOICE_RECORD_MAX`.
- A command spoken immediately after the wake word can lose its first syllable. Pause briefly after the wake word.
- Device names must contain only letters, digits and spaces. Other names are left out with a warning.
- Device suffixes follow the system device order. Reconnecting a device can change which suffix points to which device.
- After an audio device disconnects, the client does not reopen it. Restart the assistant.
- The local model can choose the wrong exposed device or answer incorrectly. Use `VOICE_LLM_SAFETY=guard`, unexpose devices it must not control, or disable it with `VOICE_LLM_MODE=off`.
- The model has no internet access and no conversation state. It answers only from the snapshot it receives.
- Learning matches the whole normalized sentence, so a rephrasing is a new entry. Negative feedback applies only to the last local action within 60 seconds.
- The room comes from `VOICE_ROOM` and matches device names, not Home Assistant areas.
- The catalog changes at most every 30 seconds after an entity changes.
- The Home Assistant conversation fallback is stateless and does not support follow-up questions.
- Domains or features without a catalog generator (for example climate setpoints or TV channels) rely on the Home Assistant conversation agent.
- The development Home Assistant stores its login and token in plain files. Use it only for development.
