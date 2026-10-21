# Windows

Details of the Windows setup. The install steps are in the [README](../README.md#2-windows-mini-pc).

The voice services run in Docker Desktop (WSL 2), as on the Mac. The client runs as `voice-client.exe`, not in Docker: a Linux container on Windows has no direct access to the Windows microphone and speaker. Tested on 2026-10-21 on an Intel N100 mini PC with Windows, Docker Desktop and a real microphone and speaker.

**The .exe.** GitHub Actions builds it (`.github/workflows/build.yml`, see [CI and releases](development.md#ci-and-releases)); `run.ps1 -Update` downloads it from the latest release. On the Mac, `make windows` builds the same file in a `golang:1.27.2-bookworm` container with MinGW-w64. The file needs only `KERNEL32.dll` and `msvcrt.dll`; miniaudio loads WASAPI at run time.

**On the Windows PC, once:**

1. Docker Desktop with the WSL 2 engine, and "Start Docker Desktop when you sign in" on.
2. `git clone` this repository. `.gitattributes` keeps LF line endings, also on Windows.
3. Create `client/.env.prod` from `client/.env.prod.example` (better with a new Home Assistant token for this PC), then run `.\scripts\run.ps1 -Update -NoServices version` to download `client/bin/voice-client.exe`. Optionally copy `server/models/` (about 1 GB) to skip the first download.
4. Settings > Privacy & security > Microphone: turn on "Let desktop apps access your microphone".
5. Allow local scripts once: `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.

**Run and test** (PowerShell, in the repository folder). `scripts/run.ps1` is the `make run` of Windows: it waits for Docker, starts the voice services, loads `client/.env.prod` (`-Env dev` for the other file; the `VOICE_*` variables of the system are removed first) and runs the client with the given command, `run` by default.

```powershell
.\scripts\run.ps1 devices      # microphone and speaker names, for VOICE_INPUT_DEVICE and VOICE_OUTPUT_DEVICE
.\scripts\run.ps1 mic-test     # shows the microphone level each second
.\scripts\run.ps1 loopback     # records 5 s and plays it back
.\scripts\run.ps1 wake-test    # say "Aleksei"
.\scripts\run.ps1 stt-test     # say a command
.\scripts\run.ps1              # the assistant
```

**Start at log-on.** `.\scripts\install-autostart.ps1` registers the scheduled task "Jarvis voice assistant": at log-on it runs `run.ps1 -KeepRunning` in a hidden window and starts the client again 5 s after it stops. The window is hidden, so read the log over the network (see [Log web page](assistant.md#log-web-page)). A Windows service would have no access to the microphone, so it is a task in your session; turn on the automatic log-on of Windows. `-Uninstall` removes the task.

**Update:** stop the assistant, `git pull`, then `.\scripts\run.ps1 -Update`. `-Update` downloads `voice-client.exe` and its SHA-256 from the latest release of the `origin` repository, checks the hash, and replaces the file only when it matches; a running assistant keeps the old file locked, so stop it first. Then restart the task (or log off and on).
