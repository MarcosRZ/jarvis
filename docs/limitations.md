# Known limitations

- Wake word: there is no Spanish wake-word model in the image. The five built-in models (`okay_nabu`, `hey_jarvis`, `hey_mycroft`, `alexa`, `hey_rhasspy`) are English. The PoC uses `alexa` with the spoken word "Aleksei" (see [wake-word.md](wake-word.md)), so "Alex", "Alexis" and "Alexa" also wake it. An Amazon Echo near the microphone can also react. A custom model needs separate training and a `--custom-model-dir` mount.
- Wake-word model license is CC BY-NC-SA 4.0: non-commercial use only.
- openWakeWord ignores an unknown `--preload-model` name with no error, so the compose file does not preload a model. The client gives the model name in each `detect` event.
- Whisper `base` is weak on short Spanish phrases. "¿Qué hora es?" from Piper gave "que hora es", "que ahora es", "¡Qué oras!" and "¡Qué odes!" in different runs. A larger model (`small`, `medium`) is more accurate but slower. Measure on the real hardware before you change it.
- Piper engine is GPL-3.0. It runs in a separate container and the client talks to it over the network only.
- Piper `describe` lists all known voices, also voices that are not on disk. Piper downloads a requested voice on the first `synthesize`. `scripts/verify.sh` checks that the voice is listed, not that it is on disk.
- Whisper needs the client to detect the end of speech (no server VAD).
- The wake-word threshold is one server value for all clients (`WAKEWORD_THRESHOLD`). The Wyoming `detect` event has no threshold field, and `detection` has no score.
- The wake-word integration test uses synthesized English speech. Real voices, accents and rooms need the manual test procedure in [wake-word.md](wake-word.md).
- Device suffixes (` #2`) follow the order of the system device list. If that order changes (for example after a reconnect), a suffix can point to the other device.
- Device disconnection was not tested with real hardware. The unit tests cover the error path of the ring buffer only.
- After a device disconnects, the client does not reopen it. The pipeline phases must decide how to recover.
- In `run` and `stt-test`, the wake-word stream reads the microphone until the detection arrives (about 100 to 200 ms after the wake word). A command said with no pause after "Aleksei" can lose its first syllable. Make a short pause after the wake word.
- The end of speech is found with a fixed level (`--speech-level`), not with a voice detector. Loud steady noise (a fan near the microphone) counts as speech and keeps the recording open until `--record-max`.
- Device names are not in `WHISPER_INITIAL_PROMPT`, because the prompt is a fixed server setting and the devices come from Home Assistant. "Enciende la luz de la cocina" was recognized correctly in the end-to-end test; unusual device names can be recognized worse. Add the phrases of your devices to `WHISPER_INITIAL_PROMPT` if necessary.
- A device name must have only letters, digits and spaces (for example not "Luz + LED"). A device with another name is left out, with a warning in the log.
- Device names are matched by sound (see [Misheard words](commands.md)), not by meaning: "el despacho" scores only 0.54 for "la luz del despacho", so it does not match. Use the full name, or a Home Assistant name that you say. Home Assistant aliases are not used yet.
- The development Home Assistant is for development only: its login and token are in plain files in `server/homeassistant`.
