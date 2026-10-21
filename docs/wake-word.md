# Wake word

`wake-test` streams the microphone to openWakeWord and prints each detection. It does not use Whisper or Piper.

```text
$ client/bin/voice-client wake-test
say the wake word (alexa); press Ctrl+C to stop
21:50:12.480  detected alexa  latency 112ms
^C
1 detections in 25s, dropped 0 bytes of audio
```

**Set the sensitivity (threshold).** openWakeWord compares each 80 ms of audio with a threshold from 0 to 1. The service has one threshold for all clients, and the Wyoming protocol cannot change it, so it is set on the server, not in the client:

1. Edit `server/.env` (copy it from `server/.env.example` first). Set `WAKEWORD_THRESHOLD`, for example `WAKEWORD_THRESHOLD=0.6`.
   - A lower value (for example `0.3`) detects a quiet or unclear wake word more easily, but gives more false detections.
   - A higher value (for example `0.7`) gives fewer false detections, but you must say the wake word more clearly.
   - The default is `0.5`.
2. Apply it: `make services-up`. Compose creates the openwakeword container again with the new value.
3. Check it: `cd server && docker compose exec openwakeword cat /proc/1/cmdline | tr '\0' ' '` shows `... --threshold 0.6 --refractory-seconds 2`.
4. Test it with `wake-test` (see the test procedure below).

**Wake word: "Aleksei".** openWakeWord has no Spanish models, and training a custom model is separate work. The built-in English `alexa` model reacts to the "a-lek-s..." sound, so it detects "Aleksei" in Spanish and in English. Test with Piper voices (2026-10-20):

| Phrase | `alexa` model |
|---|---|
| "Aleksei.", "Oye, Aleksei.", "Aleksei, ¿qué hora es?" (Spanish and English voice) | detected |
| "Alexa.", "Alexis.", "Alex, pásame la sal." | detected: these names also wake the assistant |
| "Alejandro viene mañana.", "Es una lección difícil.", "El texto es largo.", "Hay que elegir una opción.", "A lo mejor llueve.", "Los alelíes del jardín.", "Buenos días, ¿cómo estás?" | not detected |

Real voice (macOS, built-in microphone, threshold `0.5`, 2026-10-20): the user said "Aleksei" in `wake-test` and it was detected. No counts were recorded; use the test procedure below for numbers.

To use only "Aleksei", or another word, train a custom model and mount it with `--custom-model-dir`.

**Model.** `--wakeword-model` selects the model. At each connection the client asks openWakeWord with `describe` whether it offers that model, and stops with the list of available models when it does not. Without this check, openWakeWord would silently listen for `okay_nabu`.

**Double triggers.** openWakeWord ignores the same model for `WAKEWORD_REFRACTORY_SECONDS` (default `2`) after a detection, but this restarts on each new connection. The client also ignores detections for `--wakeword-cooldown` (default `2s`) after a detection, also across reconnects.

**Service failure.** When the connection fails, the client logs a warning and connects again after 0.5 s, then 1 s, 2 s, and up to 30 s. Microphone audio from that time stays in the 2 s capture buffer (older audio is dropped), and the client sends it after it connects again.

**Latency.** The client sends audio in 80 ms chunks. `latency` is the time from sending the chunk where openWakeWord found the wake word, to receiving the detection. It does not include the time you spoke before that chunk ended (at most 80 ms) or the capture buffering. Measured with `make integration`: about 112 ms.

**Test procedure: false negatives and false positives.**

1. False negatives: run `wake-test`. Say the wake word 20 times, with a pause of 5 s or more between them. Do this at 0.5 m and at 3 m from the microphone. Missed = 20 minus the number of detections.
2. False positives: run `wake-test` for 30 minutes or more with normal room sound (talking, TV, music) and no wake word. Count the detections.
3. Write down the threshold, the microphone, the distance and the results. Change `WAKEWORD_THRESHOLD` and repeat if necessary.

`make integration` also runs a fixed check. Piper says "Aleksei" (Spanish voice, then English voice), and each must give exactly one detection. Piper then says a Spanish sentence without the wake word, and it must give none.
