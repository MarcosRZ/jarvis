// Package config loads the client configuration. A command-line flag wins over
// an environment variable, and an environment variable wins over the default.
// The YAML file (--config) has the settings that have no flag or variable.
package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strconv"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/commands"
)

const (
	EnvWakeWordAddr       = "VOICE_WAKEWORD_ADDR"
	EnvSTTAddr            = "VOICE_STT_ADDR"
	EnvTTSAddr            = "VOICE_TTS_ADDR"
	EnvInputDevice        = "VOICE_INPUT_DEVICE"
	EnvOutputDevice       = "VOICE_OUTPUT_DEVICE"
	EnvWakeWordModel      = "VOICE_WAKEWORD_MODEL"
	EnvWakeWordCooldown   = "VOICE_WAKEWORD_COOLDOWN"
	EnvSTTLanguage        = "VOICE_STT_LANGUAGE"
	EnvRecordMax          = "VOICE_RECORD_MAX"
	EnvRecordStartTimeout = "VOICE_RECORD_START_TIMEOUT"
	EnvRecordEndSilence   = "VOICE_RECORD_END_SILENCE"
	EnvSpeechLevel        = "VOICE_SPEECH_LEVEL"
	EnvTTSVoice           = "VOICE_TTS_VOICE"
	EnvTTSSpeaker         = "VOICE_TTS_SPEAKER"
	EnvHAURL              = "VOICE_HA_URL"
	// EnvHAToken has no flag, so that the token is not in the process list.
	EnvHAToken          = "VOICE_HA_TOKEN"
	EnvLogLevel         = "VOICE_LOG_LEVEL"
	EnvConnectTimeout   = "VOICE_CONNECT_TIMEOUT"
	EnvOperationTimeout = "VOICE_OPERATION_TIMEOUT"
	EnvConfig           = "VOICE_CONFIG"
	EnvLogTranscripts   = "VOICE_LOG_TRANSCRIPTS"
	EnvLogHTTPAddr      = "VOICE_LOG_HTTP_ADDR"

	flagWakeWordAddr       = "wakeword-addr"
	flagSTTAddr            = "stt-addr"
	flagTTSAddr            = "tts-addr"
	flagInputDevice        = "input-device"
	flagOutputDevice       = "output-device"
	flagWakeWordModel      = "wakeword-model"
	flagWakeWordCooldown   = "wakeword-cooldown"
	flagSTTLanguage        = "stt-language"
	flagRecordMax          = "record-max"
	flagRecordStartTimeout = "record-start-timeout"
	flagRecordEndSilence   = "record-end-silence"
	flagSpeechLevel        = "speech-level"
	flagTTSVoice           = "tts-voice"
	flagTTSSpeaker         = "tts-speaker"
	flagHAURL              = "ha-url"
	flagLogLevel           = "log-level"
	flagConnectTimeout     = "connect-timeout"
	flagOperationTimeout   = "operation-timeout"
	flagConfig             = "config"
	flagLogTranscripts     = "log-transcripts"
	flagLogHTTPAddr        = "log-http-addr"

	defaultWakeWordAddr       = "127.0.0.1:10400"
	defaultSTTAddr            = "127.0.0.1:10300"
	defaultTTSAddr            = "127.0.0.1:10200"
	defaultWakeWordModel      = "alexa" // detects the spoken word "Aleksei" in Spanish and English
	defaultWakeWordCooldown   = 2 * time.Second
	defaultSTTLanguage        = "es"
	defaultRecordMax          = 8 * time.Second
	defaultRecordStartTimeout = 3 * time.Second
	defaultRecordEndSilence   = time.Second
	defaultSpeechLevel        = -40.0
	defaultTTSVoice           = "es_ES-sharvard-medium"
	defaultTTSSpeaker         = "F"
	defaultLogLevel           = slog.LevelInfo
	defaultConnectTimeout     = 3 * time.Second
	defaultOperationTimeout   = 30 * time.Second

	// Whisper transcribes audio in windows of 30 s.
	maxRecordMax   = 30 * time.Second
	minSpeechLevel = -90.0

	minPort = 1
	maxPort = 65535
)

// Config has no wake-word threshold: openWakeWord applies only its server
// setting (WAKEWORD_THRESHOLD in server/.env), and the protocol cannot change it.
type Config struct {
	WakeWordAddr       string
	STTAddr            string
	TTSAddr            string
	InputDevice        string
	OutputDevice       string
	WakeWordModel      string
	WakeWordCooldown   time.Duration
	STTLanguage        string
	RecordMax          time.Duration
	RecordStartTimeout time.Duration
	RecordEndSilence   time.Duration
	SpeechLevel        float64
	TTSVoice           string
	TTSSpeaker         string
	// HAURL is empty when no Home Assistant is used.
	HAURL    string
	HAToken  string
	LogLevel slog.Level
	// LogTranscripts logs the recognized text at info level.
	LogTranscripts bool
	// LogHTTPAddr is where the last log lines are served, empty for off.
	LogHTTPAddr      string
	ConnectTimeout   time.Duration
	OperationTimeout time.Duration
	// ConfigFile is the YAML file, empty for none.
	ConfigFile  string
	BeepEnabled bool
	Beep        audio.Beep
	// Matching sets how misheard device names and commands are corrected.
	Matching commands.Matching
}

// Load reads the environment through getenv, adds the configuration flags to fs
// and parses args with it, so the caller can add its own flags to fs first.
// All invalid values are reported together.
func Load(fs *flag.FlagSet, args []string, getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		WakeWordAddr:       envOr(getenv, EnvWakeWordAddr, defaultWakeWordAddr),
		STTAddr:            envOr(getenv, EnvSTTAddr, defaultSTTAddr),
		TTSAddr:            envOr(getenv, EnvTTSAddr, defaultTTSAddr),
		InputDevice:        getenv(EnvInputDevice),
		OutputDevice:       getenv(EnvOutputDevice),
		WakeWordModel:      envOr(getenv, EnvWakeWordModel, defaultWakeWordModel),
		WakeWordCooldown:   defaultWakeWordCooldown,
		STTLanguage:        envOr(getenv, EnvSTTLanguage, defaultSTTLanguage),
		RecordMax:          defaultRecordMax,
		RecordStartTimeout: defaultRecordStartTimeout,
		RecordEndSilence:   defaultRecordEndSilence,
		SpeechLevel:        defaultSpeechLevel,
		TTSVoice:           envOr(getenv, EnvTTSVoice, defaultTTSVoice),
		TTSSpeaker:         envOr(getenv, EnvTTSSpeaker, defaultTTSSpeaker),
		HAURL:              getenv(EnvHAURL),
		HAToken:            getenv(EnvHAToken),
		LogLevel:           defaultLogLevel,
		ConnectTimeout:     defaultConnectTimeout,
		OperationTimeout:   defaultOperationTimeout,
		ConfigFile:         getenv(EnvConfig),
		LogHTTPAddr:        getenv(EnvLogHTTPAddr),
		BeepEnabled:        true,
		Beep:               audio.DefaultBeep,
		Matching:           commands.DefaultMatching,
	}
	cfg.Beep.Notes = slices.Clone(audio.DefaultBeep.Notes)
	for _, d := range []struct {
		env    string
		target *time.Duration
	}{
		{EnvWakeWordCooldown, &cfg.WakeWordCooldown},
		{EnvRecordMax, &cfg.RecordMax},
		{EnvRecordStartTimeout, &cfg.RecordStartTimeout},
		{EnvRecordEndSilence, &cfg.RecordEndSilence},
		{EnvConnectTimeout, &cfg.ConnectTimeout},
		{EnvOperationTimeout, &cfg.OperationTimeout},
	} {
		if v := getenv(d.env); v != "" {
			parsed, err := time.ParseDuration(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: not a duration; use a value such as 500ms or 5s", d.env, v))
			} else {
				*d.target = parsed
			}
		}
	}
	if v := getenv(EnvSpeechLevel); v != "" {
		level, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s=%q: not a number; use a level in dBFS such as -40", EnvSpeechLevel, v))
		} else {
			cfg.SpeechLevel = level
		}
	}
	if v := getenv(EnvLogTranscripts); v != "" {
		on, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s=%q: use true or false", EnvLogTranscripts, v))
		} else {
			cfg.LogTranscripts = on
		}
	}
	if v := getenv(EnvLogLevel); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("%s=%q: use debug, info, warn or error", EnvLogLevel, v))
		}
	}

	fs.StringVar(&cfg.WakeWordAddr, flagWakeWordAddr, cfg.WakeWordAddr, usage("openWakeWord service host:port", EnvWakeWordAddr))
	fs.StringVar(&cfg.STTAddr, flagSTTAddr, cfg.STTAddr, usage("Whisper service host:port", EnvSTTAddr))
	fs.StringVar(&cfg.TTSAddr, flagTTSAddr, cfg.TTSAddr, usage("Piper service host:port", EnvTTSAddr))
	fs.StringVar(&cfg.InputDevice, flagInputDevice, cfg.InputDevice, usage(`microphone name as "devices" shows it, empty for the system default`, EnvInputDevice))
	fs.StringVar(&cfg.OutputDevice, flagOutputDevice, cfg.OutputDevice, usage(`speaker name as "devices" shows it, empty for the system default`, EnvOutputDevice))
	fs.StringVar(&cfg.WakeWordModel, flagWakeWordModel, cfg.WakeWordModel, usage("openWakeWord model name; the sensitivity is WAKEWORD_THRESHOLD in server/.env", EnvWakeWordModel))
	fs.DurationVar(&cfg.WakeWordCooldown, flagWakeWordCooldown, cfg.WakeWordCooldown, usage("ignore detections for this time after a detection, 0 to turn off", EnvWakeWordCooldown))
	fs.StringVar(&cfg.STTLanguage, flagSTTLanguage, cfg.STTLanguage, usage("language code of the spoken commands for Whisper", EnvSTTLanguage))
	fs.DurationVar(&cfg.RecordMax, flagRecordMax, cfg.RecordMax, usage("longest command recording; longer speech is cut, at most 30s", EnvRecordMax))
	fs.DurationVar(&cfg.RecordStartTimeout, flagRecordStartTimeout, cfg.RecordStartTimeout, usage("stop when no speech starts within this time after the wake word", EnvRecordStartTimeout))
	fs.DurationVar(&cfg.RecordEndSilence, flagRecordEndSilence, cfg.RecordEndSilence, usage("stop the recording after this much silence", EnvRecordEndSilence))
	fs.Float64Var(&cfg.SpeechLevel, flagSpeechLevel, cfg.SpeechLevel, usage("RMS level in dBFS from which audio counts as speech, from -90 to below 0", EnvSpeechLevel))
	fs.StringVar(&cfg.TTSVoice, flagTTSVoice, cfg.TTSVoice, usage("Piper voice for spoken responses; Piper downloads it on first use", EnvTTSVoice))
	fs.StringVar(&cfg.TTSSpeaker, flagTTSSpeaker, cfg.TTSSpeaker, usage("speaker of a Piper voice with several speakers, empty for its default", EnvTTSSpeaker))
	fs.StringVar(&cfg.HAURL, flagHAURL, cfg.HAURL, usage("Home Assistant URL, for example http://127.0.0.1:8123; empty for no device control; the token is "+EnvHAToken, EnvHAURL))
	fs.TextVar(&cfg.LogLevel, flagLogLevel, cfg.LogLevel, usage("debug, info, warn or error", EnvLogLevel))
	fs.BoolVar(&cfg.LogTranscripts, flagLogTranscripts, cfg.LogTranscripts, usage("log what Whisper heard at info level; off, only at debug level", EnvLogTranscripts))
	fs.StringVar(&cfg.LogHTTPAddr, flagLogHTTPAddr, cfg.LogHTTPAddr, usage("host:port of a read-only web page with the last log lines of run, kept only in memory; 0.0.0.0:8080 for the local network; empty for off", EnvLogHTTPAddr))
	fs.DurationVar(&cfg.ConnectTimeout, flagConnectTimeout, cfg.ConnectTimeout, usage("maximum time to connect to a voice service", EnvConnectTimeout))
	fs.DurationVar(&cfg.OperationTimeout, flagOperationTimeout, cfg.OperationTimeout, usage("maximum time for one read or write on a voice service connection", EnvOperationTimeout))
	fs.StringVar(&cfg.ConfigFile, flagConfig, cfg.ConfigFile, usage("YAML configuration file, for example client/config.yaml; empty for the defaults", EnvConfig))
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		errs = append(errs, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}

	errs = append(errs,
		validateAddr(flagWakeWordAddr, EnvWakeWordAddr, cfg.WakeWordAddr, defaultWakeWordAddr),
		validateAddr(flagSTTAddr, EnvSTTAddr, cfg.STTAddr, defaultSTTAddr),
		validateAddr(flagTTSAddr, EnvTTSAddr, cfg.TTSAddr, defaultTTSAddr),
	)
	if cfg.LogHTTPAddr != "" {
		errs = append(errs, validateAddr(flagLogHTTPAddr, EnvLogHTTPAddr, cfg.LogHTTPAddr, "0.0.0.0:8080"))
	}
	if cfg.WakeWordModel == "" {
		errs = append(errs, fmt.Errorf("--%s (%s) is empty: use a model name, for example %s", flagWakeWordModel, EnvWakeWordModel, defaultWakeWordModel))
	}
	if cfg.WakeWordCooldown < 0 {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use 0 or a greater value", flagWakeWordCooldown, EnvWakeWordCooldown, cfg.WakeWordCooldown))
	}
	if cfg.STTLanguage == "" {
		errs = append(errs, fmt.Errorf("--%s (%s) is empty: use a language code, for example %s", flagSTTLanguage, EnvSTTLanguage, defaultSTTLanguage))
	}
	if cfg.RecordMax <= 0 || cfg.RecordMax > maxRecordMax {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a value greater than 0 and at most %v", flagRecordMax, EnvRecordMax, cfg.RecordMax, maxRecordMax))
	}
	if cfg.RecordStartTimeout <= 0 || cfg.RecordStartTimeout > cfg.RecordMax {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a value greater than 0 and at most --%s (%v)", flagRecordStartTimeout, EnvRecordStartTimeout, cfg.RecordStartTimeout, flagRecordMax, cfg.RecordMax))
	}
	if cfg.RecordEndSilence <= 0 {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a value greater than 0", flagRecordEndSilence, EnvRecordEndSilence, cfg.RecordEndSilence))
	}
	if !(cfg.SpeechLevel >= minSpeechLevel && cfg.SpeechLevel < 0) {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a level from %v to below 0 dBFS", flagSpeechLevel, EnvSpeechLevel, cfg.SpeechLevel, minSpeechLevel))
	}
	if cfg.TTSVoice == "" {
		errs = append(errs, fmt.Errorf("--%s (%s) is empty: use a voice name, for example %s", flagTTSVoice, EnvTTSVoice, defaultTTSVoice))
	}
	if cfg.HAURL != "" {
		if u, err := url.Parse(cfg.HAURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("--%s (%s) %q: use http://host:port or https://host:port", flagHAURL, EnvHAURL, cfg.HAURL))
		}
		if cfg.HAToken == "" {
			errs = append(errs, fmt.Errorf("--%s is set but %s is empty: create a long-lived access token in Home Assistant (Profile > Security)", flagHAURL, EnvHAToken))
		}
	}
	if cfg.ConnectTimeout <= 0 {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a value greater than 0", flagConnectTimeout, EnvConnectTimeout, cfg.ConnectTimeout))
	}
	if cfg.OperationTimeout <= 0 {
		errs = append(errs, fmt.Errorf("--%s (%s) %v: use a value greater than 0", flagOperationTimeout, EnvOperationTimeout, cfg.OperationTimeout))
	}
	if cfg.ConfigFile != "" {
		errs = append(errs, loadFile(cfg.ConfigFile, &cfg))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func envOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

func usage(text, env string) string {
	return fmt.Sprintf("%s (env %s)", text, env)
}

func validateAddr(flagName, env, addr, example string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return fmt.Errorf("--%s (%s) %q: use host:port, for example %s", flagName, env, addr, example)
	}
	if p, err := strconv.Atoi(port); err != nil || p < minPort || p > maxPort {
		return fmt.Errorf("--%s (%s) %q: port must be a number from %d to %d", flagName, env, addr, minPort, maxPort)
	}
	return nil
}
