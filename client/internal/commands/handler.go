// Package commands turns a recognized Spanish text into a typed command from a
// fixed allowlist, and executes it. It never runs shell commands, and the text
// can only select a device from the allowlist given to New.
package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
)

type Kind string

const (
	KindUnknown Kind = "unknown"
	// KindInvalid is a known command with an argument that failed validation.
	KindInvalid Kind = "invalid"
	KindTime    Kind = "time"
	KindRepeat  Kind = "repeat"
	KindCancel  Kind = "cancel"
	KindHelp    Kind = "help"
	KindTurnOn  Kind = "turn-on"
	KindTurnOff Kind = "turn-off"
)

const (
	ReplyUnknown         = "Todavía no sé hacer eso."
	ReplyCancelled       = "Vale, cancelado."
	ReplyNothingToRepeat = "Todavía no has dicho nada."
	ReplyNoDevices       = "Todavía no puedo controlar dispositivos."
	ReplyMissingDevice   = "No me has dicho qué dispositivo."
	ReplyDone            = "Hecho."
)

var ErrBadDevice = errors.New("invalid device in the allowlist")

// phrases maps each normalized text to its command. "que horas" and "que ahora
// es" are Whisper base errors for "¿Qué hora es?", measured on 2026-10-20.
var phrases = map[string]Kind{
	"que hora es":            KindTime,
	"que horas son":          KindTime,
	"que hora son":           KindTime,
	"dime la hora":           KindTime,
	"que horas":              KindTime,
	"que ahora es":           KindTime,
	"que hora es ya":         KindTime,
	"sabes que hora es":      KindTime,
	"repite lo que he dicho": KindRepeat,
	"repite":                 KindRepeat,
	"repitelo":               KindRepeat,
	"cancela":                KindCancel,
	"cancelar":               KindCancel,
	"olvidalo":               KindCancel,
	"que puedes hacer":       KindHelp,
	"que sabes hacer":        KindHelp,
	"ayuda":                  KindHelp,
}

var powerVerbs = map[string]Kind{
	"enciende": KindTurnOn,
	"prende":   KindTurnOn,
	"apaga":    KindTurnOff,
}

var articles = map[string]bool{"el": true, "la": true, "los": true, "las": true}

var plainLetters = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// Device is an allowlisted device. Name is the spoken name with its article,
// for example "la luz de la cocina"; ID is the identifier for the controller.
type Device struct {
	ID   string
	Name string
	// Unavailable is true when the device does not respond now, for example
	// when it is unplugged. The help reply leaves it out.
	Unavailable bool
}

// DeviceController switches devices. A Home Assistant adapter will implement
// it; it receives only IDs from the allowlist.
type DeviceController interface {
	SetPower(ctx context.Context, id string, on bool) error
}

type Command struct {
	Kind   Kind
	Device Device
	// Argument is the device phrase of a KindInvalid command.
	Argument string
	// Suggestions are the likely devices of a KindInvalid command, for the reply.
	Suggestions []Device
	// Corrections are the guesses made for words that did not match exactly.
	Corrections []Correction
}

// DeviceSource gives the current allowlist, for example the devices exposed
// in Home Assistant.
type DeviceSource interface {
	Devices(ctx context.Context) ([]Device, error)
}

// Handler parses and executes commands. It keeps the previous text for
// KindRepeat, so it is not safe for concurrent use.
type Handler struct {
	now        func() time.Time
	devices    map[string]Device
	order      []Device
	controller DeviceController
	previous   string
	matching   Matching
	logger     *slog.Logger
	logLevel   slog.Level

	source    DeviceSource
	every     time.Duration
	report    func(error)
	refreshed time.Time
	fetched   []Device
}

// UseSource makes Handle refresh the allowlist from source when the last
// refresh is older than every. report receives a failed refresh, and the bad
// devices of each new list; the last list stays in use after a failure.
func (h *Handler) UseSource(source DeviceSource, every time.Duration, report func(error)) {
	h.source, h.every, h.report = source, every, report
}

// Refresh fetches the allowlist from the source now.
func (h *Handler) Refresh(ctx context.Context) error {
	h.refreshed = h.now()
	devices, err := h.source.Devices(ctx)
	if err != nil {
		return fmt.Errorf("refresh devices: %w", err)
	}
	problems := h.SetDevices(devices)
	if !slices.Equal(devices, h.fetched) {
		for _, p := range problems {
			h.report(p)
		}
	}
	h.fetched = devices
	return nil
}

// UseMatching replaces DefaultMatching. logger, if not nil, gets each guess at
// level, with the words heard: use debug level when the text is private.
func (h *Handler) UseMatching(m Matching, logger *slog.Logger, level slog.Level) {
	h.matching, h.logger, h.logLevel = m, logger, level
}

// Devices returns the allowlist in use.
func (h *Handler) Devices() []Device {
	return slices.Clone(h.order)
}

// New checks the allowlist and fails on the first bad device. controller can
// be nil: device commands then get ReplyNoDevices.
func New(now func() time.Time, devices []Device, controller DeviceController) (*Handler, error) {
	h := &Handler{now: now, devices: map[string]Device{}, controller: controller, matching: DefaultMatching}
	if problems := h.SetDevices(devices); len(problems) > 0 {
		return nil, problems[0]
	}
	return h, nil
}

// SetDevices replaces the allowlist, for example with the devices exposed in
// Home Assistant. A bad device is left out, and the problem is returned.
func (h *Handler) SetDevices(devices []Device) []error {
	var problems []error
	h.devices = make(map[string]Device, len(devices))
	h.order = nil
	for _, d := range devices {
		key, err := deviceKey(d)
		if err == nil {
			if other, ok := h.devices[key]; ok {
				err = fmt.Errorf("%w: %q and %q have the same name", ErrBadDevice, other.ID, d.ID)
			}
		}
		if err != nil {
			problems = append(problems, err)
			continue
		}
		h.devices[key] = d
		h.order = append(h.order, d)
	}
	return problems
}

// deviceKey returns the normalized name without its article, the key that Parse matches.
func deviceKey(d Device) (string, error) {
	key := withoutArticle(normalize(d.Name))
	words := strings.Fields(key)
	switch {
	case strings.TrimSpace(d.ID) == "":
		return "", fmt.Errorf("%w: %q has no ID", ErrBadDevice, d.Name)
	case len(words) == 0:
		return "", fmt.Errorf("%w: %q has no name", ErrBadDevice, d.ID)
	case strings.ContainsFunc(d.Name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' }):
		return "", fmt.Errorf("%w: name %q has characters other than letters, digits and spaces", ErrBadDevice, d.Name)
	case powerVerbs[words[0]] != "":
		return "", fmt.Errorf("%w: name %q starts with a command word", ErrBadDevice, d.Name)
	}
	return key, nil
}

// spokenName lowercases a leading article, so that "La luz de la cocina" reads
// well inside a sentence. Other names stay as they are, for example "AC".
func spokenName(d Device) string {
	first, rest, _ := strings.Cut(d.Name, " ")
	if articles[strings.ToLower(first)] {
		return strings.ToLower(first) + " " + rest
	}
	return d.Name
}

// Handle parses text, executes the command and returns the reply.
func (h *Handler) Handle(ctx context.Context, text string) string {
	if h.source != nil && h.now().Sub(h.refreshed) >= h.every {
		if err := h.Refresh(ctx); err != nil {
			h.report(err)
		}
	}
	cmd := h.Parse(text)
	if h.logger != nil {
		for _, c := range cmd.Corrections {
			h.logger.Log(ctx, h.logLevel, "guess", "heard", c.Heard, "meant", c.Meant, "score", fmt.Sprintf("%.2f", c.Score), "used", c.Used)
		}
	}
	reply := h.Execute(ctx, cmd)
	if cmd.Kind != KindRepeat {
		h.previous = strings.TrimSpace(text)
	}
	return reply
}

// Parse maps text to a command. It has no side effects. When a phrase, a verb
// or a device name does not match exactly, it takes the one that sounds most
// alike if the guess is good and clear enough (see Matching); a device guess
// that is not good enough only becomes a suggestion.
func (h *Handler) Parse(text string) Command {
	norm := normalize(text)
	if kind, ok := phrases[norm]; ok {
		return Command{Kind: kind}
	}
	words := strings.Fields(norm)
	if len(words) == 0 {
		return Command{Kind: KindUnknown}
	}
	var corrections []Correction
	kind, ok := powerVerbs[words[0]]
	if !ok {
		var c *Correction
		if kind, c = h.guessKind(words[0], powerVerbs); c == nil {
			if kind, c = h.guessKind(norm, phrases); c != nil {
				return Command{Kind: kind, Corrections: []Correction{*c}}
			}
			return Command{Kind: KindUnknown}
		}
		corrections = append(corrections, *c)
	}
	argument := withoutArticle(strings.Join(words[1:], " "))
	if device, ok := h.devices[argument]; ok {
		return Command{Kind: kind, Device: device, Corrections: corrections}
	}
	return h.guessDevice(kind, argument, corrections)
}

// Execute runs cmd and returns the reply. Only KindTurnOn and KindTurnOff
// with an allowlisted device reach the controller.
func (h *Handler) Execute(ctx context.Context, cmd Command) string {
	switch cmd.Kind {
	case KindTime:
		return spokenTime(h.now())
	case KindRepeat:
		if h.previous == "" {
			return ReplyNothingToRepeat
		}
		return "Has dicho: " + h.previous
	case KindCancel:
		return ReplyCancelled
	case KindHelp:
		return h.help()
	case KindTurnOn, KindTurnOff:
		return h.setPower(ctx, cmd.Device, cmd.Kind == KindTurnOn)
	case KindInvalid:
		if cmd.Argument == "" {
			return ReplyMissingDevice
		}
		reply := fmt.Sprintf("No conozco el dispositivo %q.", cmd.Argument)
		if len(cmd.Suggestions) > 0 {
			names := make([]string, len(cmd.Suggestions))
			for i, d := range cmd.Suggestions {
				names[i] = spokenName(d)
			}
			reply += " ¿Querías decir " + strings.Join(names, " o ") + "?"
		}
		return reply
	default:
		return ReplyUnknown
	}
}

func (h *Handler) setPower(ctx context.Context, device Device, on bool) string {
	if h.controller == nil {
		return ReplyNoDevices
	}
	if err := h.controller.SetPower(ctx, device.ID, on); err != nil {
		verb := "apagar"
		if on {
			verb = "encender"
		}
		return fmt.Sprintf("No he podido %s %s.", verb, spokenName(device))
	}
	return ReplyDone
}

func (h *Handler) help() string {
	reply := "Puedo decirte la hora, repetir lo que has dicho y cancelar."
	var names []string
	for _, d := range h.order {
		if !d.Unavailable {
			names = append(names, spokenName(d))
		}
	}
	if len(names) == 0 {
		return reply
	}
	list := names[0]
	if n := len(names); n > 1 {
		list = strings.Join(names[:n-1], ", ") + " y " + names[n-1]
	}
	return reply + " También puedo encender y apagar " + list + "."
}

// normalize lowercases text, removes accents and punctuation, and joins the
// words with single spaces.
func normalize(text string) string {
	text = plainLetters.Replace(strings.ToLower(text))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, " ")
}

func withoutArticle(phrase string) string {
	first, rest, found := strings.Cut(phrase, " ")
	if found && articles[first] {
		return rest
	}
	return phrase
}

// spokenTime gives the time in the 12-hour form of spoken Spanish. Piper reads
// the digits as Spanish numbers; "una" is written out because "la 1" is read as "la uno".
func spokenTime(t time.Time) string {
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	prefix := fmt.Sprintf("Son las %d", hour)
	if hour == 1 {
		prefix = "Es la una"
	}
	switch m := t.Minute(); m {
	case 0:
		return prefix + " en punto."
	case 15:
		return prefix + " y cuarto."
	case 30:
		return prefix + " y media."
	default:
		return fmt.Sprintf("%s y %d.", prefix, m)
	}
}
