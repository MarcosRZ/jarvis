# Client commands

Reference of the `voice-client` commands. To build it, see [development.md](development.md).

```sh
client/bin/voice-client version
client/bin/voice-client config-check [flags]   # validate and print the effective configuration
client/bin/voice-client devices                # list audio input and output devices
client/bin/voice-client mic-test [--duration 10s] [flags]   # capture and show the peak level each second
client/bin/voice-client loopback [--duration 5s] [flags]    # record, then play the recording
client/bin/voice-client wake-test [flags]      # listen for the wake word and print each detection
client/bin/voice-client stt-test [flags]       # wake word, then print the command that Whisper recognizes
client/bin/voice-client say --text "..." [--text "..."] [flags]   # speak each text in order with Piper
client/bin/voice-client run [flags]            # the assistant; stop with Ctrl+C (SIGINT) or SIGTERM
client/bin/voice-client <command> -h           # list the flags of a command
```

`version` and `config-check` need no voice service and no audio device. Exit codes: 0 OK, 1 runtime error, 2 usage error, 3 invalid configuration. `mic-test` accepts up to `1h`. `loopback` accepts up to `30s`, because it keeps the recording in memory.

`devices` shows each device with the name that `--input-device` and `--output-device` accept. When two devices of one kind have the same name, the second one gets ` #2`, the third ` #3`, in the order of the system list:

```text
output devices:
  MSI G32CQ4 E2
  MSI G32CQ4 E2 #2
  Altavoces del MacBook Pro (default)
```

`mic-test` ends with a summary. Use it to check memory and lost audio over a long capture:

```text
captured 1m0.03s of audio in 1m0.001s, dropped 0 bytes, heap 0.3 MiB at start and 0.4 MiB at end
```
