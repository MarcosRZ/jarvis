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

	"jarvis/client/internal/learning"
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
	// KindPenalty is "mal": the last inferred action was wrong.
	KindPenalty Kind = "penalty"
)

const (
	ReplyUnknown          = "Todavía no sé hacer eso."
	ReplyCancelled        = "Vale, cancelado."
	ReplyNothingToRepeat  = "Todavía no has dicho nada."
	ReplyNoDevices        = "Todavía no puedo controlar dispositivos."
	ReplyMissingDevice    = "No me has dicho qué dispositivo."
	ReplyDone             = "Hecho."
	ReplyPenalized        = "Lo siento. Lo tendré en cuenta."
	ReplyNothingToCorrect = "No hay nada que corregir."
)

var ErrBadDevice = errors.New("invalid device in the allowlist")

// phrases maps each normalized text to its command. "que horas" and "que ahora
// es" are Whisper base errors for "¿Qué hora es?", measured on 2026-10-20.
var phrases = map[string]Kind{
	"que hora es":            KindTime,
	"que horas son":          KindTime,
	"que hora son":           KindTime,
	"dime la hora":           KindTime,
	"dime que hora es":       KindTime,
	"dime que horas son":     KindTime,
	"dime que hora son":      KindTime,
	"dime que horas":         KindTime,
	"me dices la hora":       KindTime,
	"puedes decirme la hora": KindTime,
	"a que hora estamos":     KindTime,
	"que hora tienes":        KindTime,
	"sabes la hora":          KindTime,
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
	"mal":                    KindPenalty,
	"esta mal":               KindPenalty,
	"eso esta mal":           KindPenalty,
	"te has equivocado":      KindPenalty,
	"equivocado":             KindPenalty,
	"no es eso":              KindPenalty,
	"incorrecto":             KindPenalty,
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
	// Source is where a device command came from: "parser", "interpreted",
	// "reasoned", "state" or "learned". It is used for logging and learning.
	Source string
	// Text is the normalized sentence that produced the command. It is only
	// set by Handle, for learning and feedback.
	Text string
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

// InterpretMode sets when Handler asks its Interpreter.
type InterpretMode string

const (
	// InterpretOff never asks the interpreter.
	InterpretOff InterpretMode = "off"
	// InterpretFallback asks the interpreter only when ParseAll returns one
	// unknown or invalid command, after the deterministic parser had its try.
	InterpretFallback InterpretMode = "fallback"
	// InterpretAlways asks the interpreter for every single sentence with a
	// candidate device and for every unknown sentence about a physical state,
	// and keeps the deterministic parser as the fallback when the interpreter
	// fails or finds nothing.
	InterpretAlways InterpretMode = "always"
)

// Valid reports whether m is one of the modes.
func (m InterpretMode) Valid() bool {
	switch m {
	case InterpretOff, InterpretFallback, InterpretAlways:
		return true
	}
	return false
}

// Interpreter turns free text into commands. It receives the devices that are
// plausible for the text, and its result must use only those devices. An empty
// result means that the text is not a command.
type Interpreter interface {
	Interpret(ctx context.Context, text string, devices []Device) ([]Command, error)
}

// feedback is the last device action, so "mal" can penalize it. The window
// keeps a much later "mal" from punishing an unrelated action.
type feedback struct {
	text   string
	device string
	kind   Kind
	source string
	at     time.Time
	valid  bool
}

// Handler parses and executes commands. It keeps the previous text for
// KindRepeat, so it is not safe for concurrent use.
type Handler struct {
	now        func() time.Time
	devices    map[string]Device
	byID       map[string]Device
	order      []Device
	controller DeviceController
	previous   string
	matching   Matching
	logger     *slog.Logger
	logLevel   slog.Level

	interpreter   Interpreter
	interpretMode InterpretMode
	room          string
	learning      *learning.Store
	lastAction    feedback

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

// UseInterpreter sets a language-model interpreter and when Handle asks it.
// InterpretOff, or a nil interpreter, turns it off.
func (h *Handler) UseInterpreter(i Interpreter, mode InterpretMode) {
	h.interpreter, h.interpretMode = i, mode
}

// UseRoom sets the room of the assistant, so a state such as "no veo nada"
// can choose that room's light without asking the model.
func (h *Handler) UseRoom(room string) {
	h.room = room
}

// UseLearning enables learned sentence-to-action mappings. A nil store keeps
// learning off.
func (h *Handler) UseLearning(store *learning.Store) {
	h.learning = store
}

// feedbackWindow is how long after an action "mal" still refers to it.
const feedbackWindow = 60 * time.Second

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
	h.byID = make(map[string]Device, len(devices))
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
		h.byID[d.ID] = d
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

// Handle parses text, optionally asks the interpreter, executes the commands
// and returns the reply.
func (h *Handler) Handle(ctx context.Context, text string) string {
	if h.source != nil && h.now().Sub(h.refreshed) >= h.every {
		if err := h.Refresh(ctx); err != nil {
			h.report(err)
		}
	}
	commands := h.ParseAll(text)
	if h.logger != nil {
		for _, cmd := range commands {
			for _, c := range cmd.Corrections {
				h.logger.Log(ctx, h.logLevel, "guess", "heard", c.Heard, "meant", c.Meant, "score", fmt.Sprintf("%.2f", c.Score), "used", c.Used)
			}
		}
	}
	switch {
	case h.canInterpretCandidates(text, commands):
		if interpreted, ok := h.interpretCandidates(ctx, text); ok {
			commands = interpreted
		}
	case h.canReason(text, commands):
		if inferred, ok := h.interpretReasoning(ctx, text); ok {
			commands = inferred
		}
	}
	key := learningKey(text)
	for i := range commands {
		commands[i].Text = key
	}
	reply := h.executeAll(ctx, commands)
	if !slices.ContainsFunc(commands, func(c Command) bool {
		return c.Kind == KindRepeat || c.Kind == KindPenalty
	}) {
		h.previous = strings.TrimSpace(text)
	}
	return reply
}

// canInterpretCandidates reports whether the text has a power verb and one
// clear device. In fallback mode the model only runs when the parser failed.
func (h *Handler) canInterpretCandidates(text string, commands []Command) bool {
	if h.interpreter == nil || h.interpretMode == InterpretOff {
		return false
	}
	if len(commands) != 1 || !HasPowerVerb(text) || len(h.Candidates(text)) == 0 {
		return false
	}
	if h.interpretMode == InterpretFallback {
		return commands[0].Kind == KindUnknown || commands[0].Kind == KindInvalid
	}
	return true
}

// canReason reports whether the interpreter may infer an action from a text
// without a power verb, for example a state such as "hay demasiada luz".
func (h *Handler) canReason(text string, commands []Command) bool {
	if len(commands) != 1 || HasPowerVerb(text) || !plausibleState(text) {
		return false
	}
	return commands[0].Kind == KindUnknown || commands[0].Kind == KindInvalid
}

// stateWords are the words of a physical state that the interpreter may turn
// into an action. They keep a small model from inventing actions for chat.
var stateWords = map[string]bool{
	"luz": true, "luces": true, "iluminacion": true, "iluminado": true, "iluminada": true,
	"lampara": true, "lamparas": true, "oscuridad": true, "oscuro": true, "oscura": true,
	"claridad": true, "brillo": true, "brillante": true,
	"frio": true, "fria": true, "calor": true, "temperatura": true, "aire": true,
	"calefaccion": true, "ventilador": true, "humedad": true, "veo": true,
}

func plausibleState(text string) bool {
	for _, word := range strings.Fields(withoutFillers(normalize(text))) {
		if stateWords[word] {
			return true
		}
	}
	return false
}

// interpretCandidates asks the interpreter with the one device that the text
// names. The action runs at once because the text already asked for it.
func (h *Handler) interpretCandidates(ctx context.Context, text string) ([]Command, bool) {
	candidates := h.Candidates(text)
	if len(candidates) == 0 {
		return nil, false
	}
	key := learningKey(text)
	var blocked *learning.Entry
	if h.learning != nil {
		if e, found := h.learning.Lookup(key); found {
			if e.Score > 0 {
				if cmd, ok := h.commandFromEntry(e); ok && slices.ContainsFunc(candidates, func(d Device) bool { return d.ID == cmd.Device.ID }) {
					if h.logger != nil {
						h.logger.Log(ctx, h.logLevel, "interpreted", "kind", cmd.Kind, "device", cmd.Device.ID, "source", "learned", "score", e.Score)
					}
					return []Command{cmd}, true
				}
			} else {
				blocked = &e
			}
		}
	}
	commands, ok := h.askInterpreter(ctx, text, candidates, candidates, "interpreted")
	if !ok {
		return nil, false
	}
	if blocked != nil {
		commands = slices.DeleteFunc(commands, func(c Command) bool {
			return c.Device.ID == blocked.Device && string(c.Kind) == blocked.Kind
		})
		if len(commands) == 0 {
			return nil, false
		}
	}
	for i := range commands {
		commands[i].Source = "interpreted"
	}
	return commands, ok
}

// interpretReasoning asks the interpreter with every available device, so that
// it can infer an action and a room from a state or a feeling. The inferred
// action runs at once, like a command that names the device.
func (h *Handler) interpretReasoning(ctx context.Context, text string) ([]Command, bool) {
	key := learningKey(text)
	var blocked *learning.Entry
	if h.learning != nil {
		if e, found := h.learning.Lookup(key); found {
			if e.Score > 0 {
				if cmd, ok := h.commandFromEntry(e); ok {
					if h.logger != nil {
						h.logger.Log(ctx, h.logLevel, "reasoned", "kind", cmd.Kind, "device", cmd.Device.ID, "source", "learned", "score", e.Score)
					}
					return []Command{cmd}, true
				}
			} else {
				blocked = &e
			}
		}
	}
	if cmd, ok := h.stateAction(text); ok {
		if blocked == nil || cmd.Device.ID != blocked.Device || string(cmd.Kind) != blocked.Kind {
			if h.logger != nil {
				h.logger.Log(ctx, h.logLevel, "reasoned", "kind", cmd.Kind, "device", cmd.Device.ID, "source", "state")
			}
			return []Command{cmd}, true
		}
	}
	if h.interpreter == nil || h.interpretMode == InterpretOff {
		return nil, false
	}
	devices := make([]Device, 0, len(h.order))
	for _, d := range h.order {
		if !d.Unavailable {
			devices = append(devices, d)
		}
	}
	if len(devices) == 0 {
		return nil, false
	}
	commands, ok := h.askInterpreter(ctx, text, devices, devices, "reasoned")
	if !ok {
		return nil, false
	}
	if blocked != nil {
		commands = slices.DeleteFunc(commands, func(c Command) bool {
			return c.Device.ID == blocked.Device && string(c.Kind) == blocked.Kind
		})
		if len(commands) == 0 {
			return nil, false
		}
	}
	// A state asks for one fix, so run only the first action that the model
	// inferred; the prompt tells it not to invent more.
	commands[0].Source = "reasoned"
	return commands[:1], true
}

// commandFromEntry turns a learned entry into a command, using the current
// allowlist: a device that is no longer exposed is not used.
func (h *Handler) commandFromEntry(e learning.Entry) (Command, bool) {
	device, ok := h.byID[e.Device]
	if !ok || device.Unavailable {
		return Command{}, false
	}
	kind := Kind(e.Kind)
	if kind != KindTurnOn && kind != KindTurnOff {
		return Command{}, false
	}
	return Command{Kind: kind, Device: device, Source: "learned", Text: e.Text}, true
}

// stateAction resolves the common light phrases without the model. A 0.5B
// model reads "no veo nada" as a negation or as chat and returns nothing; the
// fix is deterministic and still uses only the allowlist and the room.
func (h *Handler) stateAction(text string) (Command, bool) {
	norm := learningKey(text)
	on := false
	switch {
	case containsAny(norm,
		"no veo nada", "no veo bien", "no hay luz", "no puedo ver",
		"esta muy oscuro", "esta oscuro", "hay poca luz", "necesito luz"):
		on = true
	case containsAny(norm,
		"hay demasiada luz", "me sobra luz", "esta muy claro", "hay mucho brillo", "me deslumbra"):
		on = false
	default:
		return Command{}, false
	}
	device, ok := h.roomLight()
	if !ok {
		return Command{}, false
	}
	kind := KindTurnOff
	if on {
		kind = KindTurnOn
	}
	return Command{Kind: kind, Device: device, Source: "state"}, true
}

// roomLight returns the light of the assistant's room. Without a room it only
// answers when there is exactly one light, so it never guesses between rooms.
func (h *Handler) roomLight() (Device, bool) {
	room := normalize(h.room)
	lights := make([]Device, 0, len(h.order))
	for _, d := range h.order {
		if d.Unavailable {
			continue
		}
		fields := strings.Fields(normalize(d.Name))
		if !containsAny(strings.Join(fields, " "), "luz", "luces", "lampara", "lamparas", "iluminacion") {
			continue
		}
		if room != "" && slices.Contains(fields, room) {
			return d, true
		}
		lights = append(lights, d)
	}
	if len(lights) == 1 {
		return lights[0], true
	}
	return Device{}, false
}

// containsAny reports whether norm contains one of the phrases as words.
func containsAny(norm string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(" "+norm+" ", " "+phrase+" ") {
			return true
		}
	}
	return false
}

// askInterpreter calls the interpreter, keeps only the commands that the
// allowlist and allowed devices permit, and logs each one.
func (h *Handler) askInterpreter(ctx context.Context, text string, devices, allowed []Device, msg string) ([]Command, bool) {
	got, err := h.interpreter.Interpret(ctx, text, devices)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("language model", "error", err)
		}
		return nil, false
	}
	commands := h.allowInterpreted(got, allowed)
	if len(commands) == 0 {
		return nil, false
	}
	if h.logger != nil {
		for _, c := range commands {
			h.logger.Log(ctx, h.logLevel, msg, "kind", c.Kind, "device", c.Device.ID)
		}
	}
	return commands, true
}

// allowInterpreted drops a command with a device that is not in the allowlist
// or not allowed, and drops the fixed commands, which the deterministic parser
// handles. The caller decides whether the text needs a power verb.
func (h *Handler) allowInterpreted(commands []Command, allowed []Device) []Command {
	valid := make([]Command, 0, len(commands))
	for _, c := range commands {
		c.Argument, c.Suggestions, c.Corrections = "", nil, nil
		switch c.Kind {
		case KindTime, KindRepeat, KindCancel, KindHelp:
			// The fixed parser decides these; the model is only trusted with
			// allowlisted devices.
			continue
		case KindTurnOn, KindTurnOff:
			device, ok := h.byID[c.Device.ID]
			if !ok || !slices.ContainsFunc(allowed, func(d Device) bool { return d.ID == c.Device.ID }) {
				continue
			}
			c.Device = device
			valid = append(valid, c)
		}
	}
	return valid
}

// executeAll runs the commands in order and joins their replies, without
// repeating an identical reply. One action gives the usual single reply.
func (h *Handler) executeAll(ctx context.Context, commands []Command) string {
	if len(commands) == 1 {
		return h.Execute(ctx, commands[0])
	}
	replies := make([]string, 0, len(commands))
	seen := make(map[string]bool, len(commands))
	for _, c := range commands {
		reply := h.Execute(ctx, c)
		if reply == "" || seen[reply] {
			continue
		}
		seen[reply] = true
		replies = append(replies, reply)
	}
	return strings.Join(replies, " ")
}

// powerStems are the beginnings of the verbs that may switch a device. A
// transcript without one is not a device command, in either mode.
var powerStems = []string{"apag", "encend", "enciend", "prend"}

func hasPowerStem(word string) bool {
	for _, stem := range powerStems {
		if strings.HasPrefix(word, stem) {
			return true
		}
	}
	return false
}

// HasPowerVerb reports whether text contains a verb that may switch a device.
func HasPowerVerb(text string) bool {
	for _, word := range strings.Fields(normalize(text)) {
		if hasPowerStem(word) {
			return true
		}
	}
	return false
}

// Candidates returns the one device that the text most likely names, for the
// language model. The words after the power verb are matched against the
// allowlist. A guess needs a score of at least Matching.Suggest and a clear
// lead over the next device; otherwise Candidates returns nil and the model is
// not asked. This keeps the model from choosing a device that the sentence
// does not name.
func (h *Handler) Candidates(text string) []Device {
	words := strings.Fields(withoutFillers(normalize(text)))
	start := -1
	for i, word := range words {
		if hasPowerStem(word) {
			start = i + 1
			break
		}
	}
	if start < 0 || start >= len(words) {
		return nil
	}
	argument := withoutArticle(strings.Join(words[start:], " "))
	r := rank(argument, h.devices)
	if len(r) == 0 || r[0].score < h.matching.Suggest || !clear(h.matching, r) {
		return nil
	}
	return []Device{r[0].value}
}

// ParseAll parses text as one command, or as clauses joined by "y" or "e"
// when every clause is a valid command. It has no side effects. The full text
// is tried first, so a device name that contains " y " still matches.
func (h *Handler) ParseAll(text string) []Command {
	cmd := h.Parse(text)
	if cmd.Kind != KindUnknown && cmd.Kind != KindInvalid {
		return []Command{cmd}
	}
	parts := splitClauses(withoutFillers(normalize(text)))
	if len(parts) < 2 {
		return []Command{cmd}
	}
	commands := make([]Command, 0, len(parts))
	for _, part := range parts {
		c := h.Parse(part)
		if c.Kind == KindUnknown || c.Kind == KindInvalid {
			return []Command{cmd}
		}
		commands = append(commands, c)
	}
	return commands
}

func splitClauses(norm string) []string {
	fields := strings.Fields(norm)
	parts := make([]string, 0, 2)
	start := 0
	for i, word := range fields {
		if i > start && (word == "y" || word == "e") {
			parts = append(parts, strings.Join(fields[start:i], " "))
			start = i + 1
		}
	}
	if start < len(fields) {
		parts = append(parts, strings.Join(fields[start:], " "))
	}
	return parts
}

// Parse maps text to a command. It has no side effects. When a phrase, a verb
// or a device name does not match exactly, it takes the one that sounds most
// alike if the guess is good and clear enough (see Matching); a device guess
// that is not good enough only becomes a suggestion.
func (h *Handler) Parse(text string) Command {
	norm := withoutFillers(normalize(text))
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
		return h.setPower(ctx, cmd)
	case KindPenalty:
		return h.penalize(ctx)
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

func (h *Handler) setPower(ctx context.Context, cmd Command) string {
	device, on, source := cmd.Device, cmd.Kind == KindTurnOn, cmd.Source
	if source == "" {
		source = "parser"
	}
	kind := KindTurnOff
	verb := "apagar"
	if on {
		kind, verb = KindTurnOn, "encender"
	}
	if h.controller == nil {
		h.logAction(ctx, source, kind, device, "no controller", time.Duration(0))
		return ReplyNoDevices
	}
	start := time.Now()
	err := h.controller.SetPower(ctx, device.ID, on)
	if err != nil {
		h.logAction(ctx, source, kind, device, err.Error(), time.Since(start))
		return fmt.Sprintf("No he podido %s %s.", verb, spokenName(device))
	}
	h.logAction(ctx, source, kind, device, "", time.Since(start))
	h.recordSuccess(ctx, cmd)
	return ReplyDone
}

// logAction records what the assistant did. It is logged at info level because
// it is the audit trail of a real device action, not a transcript.
func (h *Handler) logAction(ctx context.Context, source string, kind Kind, device Device, failure string, took time.Duration) {
	if h.logger == nil {
		return
	}
	attrs := []any{
		"source", source,
		"kind", kind,
		"device", device.ID,
		"name", device.Name,
		"ok", failure == "",
		"duration", took.Round(time.Millisecond),
	}
	if failure != "" {
		attrs = append(attrs, "error", failure)
	}
	h.logger.Log(ctx, slog.LevelInfo, "action", attrs...)
}

// recordSuccess counts an action as a success because no punishment followed.
// It remembers the action so a later "mal" can undo that count and penalize it.
func (h *Handler) recordSuccess(ctx context.Context, cmd Command) {
	source := cmd.Source
	if source == "" {
		source = "parser"
	}
	h.lastAction = feedback{text: cmd.Text, device: cmd.Device.ID, kind: cmd.Kind, source: source, at: h.now(), valid: true}
	if h.learning == nil || cmd.Text == "" || !learnable(source) {
		return
	}
	entry, err := h.learning.Learn(cmd.Text, cmd.Device.ID, cmd.Device.Name, string(cmd.Kind), h.now())
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("learning", "error", err)
		}
		return
	}
	if h.logger != nil {
		h.logger.Info("learning", "result", "success", "score", entry.Score, "successes", entry.Successes, "failures", entry.Failures, "device", entry.Device, "kind", entry.Kind)
	}
}

// penalize handles "mal": it penalizes the last action if it is recent. The
// success counted at execution time is undone by the store.
func (h *Handler) penalize(ctx context.Context) string {
	last := h.lastAction
	h.lastAction.valid = false
	if !last.valid || h.now().Sub(last.at) > feedbackWindow {
		return ReplyNothingToCorrect
	}
	learned := false
	if h.learning != nil && last.text != "" && learnable(last.source) {
		entry, ok, err := h.learning.Penalize(last.text, last.device, string(last.kind), h.now())
		switch {
		case err != nil:
			if h.logger != nil {
				h.logger.Warn("learning", "error", err)
			}
		case ok:
			learned = true
			if h.logger != nil {
				h.logger.Info("learning", "result", "penalty", "score", entry.Score, "successes", entry.Successes, "failures", entry.Failures, "device", entry.Device, "kind", entry.Kind)
			}
		}
	}
	if h.logger != nil {
		h.logger.Info("feedback", "result", "penalty", "source", last.source, "device", last.device, "kind", last.kind, "learned", learned)
	}
	return ReplyPenalized
}

func learnable(source string) bool {
	switch source {
	case "interpreted", "reasoned", "state", "learned":
		return true
	}
	return false
}

// learningKey is the normalized sentence without fillers, the store key.
func learningKey(text string) string {
	return withoutFillers(normalize(text))
}

func (h *Handler) help() string {
	reply := "Puedo decirte la hora, repetir lo que has dicho, cancelar y corregir la última acción si digo mal."
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

// fillers are words that do not change the command. They are common at the end
// of a spoken sentence ("apaga el ventilador, por favor") and would otherwise
// make the device phrase unknown.
var fillers = map[string]bool{"por": true, "favor": true, "porfa": true, "please": true, "gracias": true}

func withoutFillers(norm string) string {
	words := strings.Fields(norm)
	kept := words[:0]
	for _, word := range words {
		if !fillers[word] {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
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
