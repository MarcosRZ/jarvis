// Package commands turns a recognized Spanish text into a typed command from a
// fixed allowlist, executes it, or forwards it to Home Assistant. It never runs
// shell commands, and a local device command can only select a device from the
// allowlist given to New.
package commands

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"jarvis/actions"
	"jarvis/client/internal/data"
	"jarvis/client/internal/learning"
)

type Kind string

const (
	KindUnknown Kind = "unknown"
	KindTime    Kind = "time"
	KindDate    Kind = "date"
	KindRepeat  Kind = "repeat"
	KindCancel  Kind = "cancel"
	KindHelp    Kind = "help"
	KindTurnOn  Kind = "turn-on"
	KindTurnOff Kind = "turn-off"
	// KindAction is an action of the generated Home Assistant catalog.
	KindAction Kind = "action"
	// KindPenalty is "mal": the last inferred action was wrong.
	KindPenalty Kind = "penalty"
	// KindData is an answer rendered from a taught data template.
	KindData Kind = "data"
	// KindReply is a fixed learned answer.
	KindReply Kind = "reply"
	// KindPending is a taught sentence whose target is not known yet.
	KindPending Kind = "pending"
)

const (
	ReplyUnknown          = "Todavía no sé hacer eso."
	ReplyCancelled        = "Vale, cancelado."
	ReplyOkay             = "okay"
	ReplyNothingToRepeat  = "Todavía no has dicho nada."
	ReplyNoDevices        = "Todavía no puedo controlar dispositivos."
	ReplyDone             = "Hecho."
	ReplyPenalized        = "Lo siento. Lo tendré en cuenta."
	ReplyNothingToCorrect = "No hay nada que corregir."
	ReplyConfirmed        = "Entendido."
	ReplyDataMissing      = "No tengo ese dato."
	ReplyModelDown        = "No puedo responder ahora."
	ReplyNewsDown         = "No tengo noticias ahora."
	ReplyPending          = "No he podido deducir la acción. La dejo pendiente para ajustar en Manhattan."
	ReplyStoreFailed      = "No he podido guardarlo."
	ReplyTeachSentence    = "Dime la frase."
	ReplyAskForOrder      = "¿Qué debo hacer?"
	ReplyAskForAnswer     = "¿Qué debo responder?"
	ReplyRetryExplanation = "Explícamelo de otra forma."
	ReplyTellOrder        = "Dime la orden."
	ReplyTellQuestion     = "Dime la pregunta."
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
	"que dia es hoy":         KindDate,
	"que dia es":             KindDate,
	"que dia estamos":        KindDate,
	"a que dia estamos":      KindDate,
	"dime la fecha":          KindDate,
	"dime que dia es hoy":    KindDate,
	"que fecha es hoy":       KindDate,
	"que fecha es":           KindDate,
	"fecha de hoy":           KindDate,
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

var articles = map[string]bool{"el": true, "la": true, "los": true, "las": true}

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
	// Action is set when Kind is KindAction.
	Action actions.Action
	// Reply is the spoken answer of KindReply and the rendered answer of
	// KindData.
	Reply string
	// Template is the stored template of a KindData command. It is used to
	// count a success on the entry that produced the answer.
	Template string
	// Source is where a command came from: "direct", "catalog", "reasoned",
	// "state", "learned" or "data". It is used for logging and learning.
	Source string
	// Text is the normalized sentence that produced the command. It is only
	// set by Handle, for learning and feedback.
	Text string
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

// FallbackOrder sets whether Home Assistant or the interpreter runs first for
// an unresolved sentence.
type FallbackOrder string

// ActionSafety sets which inferred actions Handler may run.
type ActionSafety string

const (
	// InterpretOff never asks the interpreter.
	InterpretOff InterpretMode = "off"
	// InterpretFallback asks the interpreter when ParseAll does not resolve the
	// sentence, after learned memory and according to the fallback order.
	// Learned sentences do not reach the model.
	InterpretFallback InterpretMode = "fallback"

	// FallbackHAFirst asks Home Assistant before the interpreter for an
	// unresolved sentence.
	FallbackHAFirst FallbackOrder = "ha-first"
	// FallbackModelFirst asks the interpreter before Home Assistant there.
	FallbackModelFirst FallbackOrder = "model-first"

	// ActionGuard runs an inferred action only when the sentence names the
	// chosen device or describes a physical state that the device can fix.
	ActionGuard ActionSafety = "guard"
	// ActionLiteral runs any action that the interpreter returns.
	ActionLiteral ActionSafety = "literal"
	// ActionStates runs an inferred action only for a physical-state sentence.
	ActionStates ActionSafety = "states"
)

// Valid reports whether m is one of the modes.
func (m InterpretMode) Valid() bool {
	switch m {
	case InterpretOff, InterpretFallback:
		return true
	}
	return false
}

// Valid reports whether o is one of the fallback orders.
func (o FallbackOrder) Valid() bool {
	switch o {
	case FallbackHAFirst, FallbackModelFirst:
		return true
	}
	return false
}

// Valid reports whether s is one of the action-safety modes.
func (s ActionSafety) Valid() bool {
	switch s {
	case ActionGuard, ActionLiteral, ActionStates:
		return true
	}
	return false
}

// Interpretation is one model answer: commands to run, or a spoken reply.
// A model returns commands for an action and a reply for a question about the
// read-only context; it should not return both.
type Interpretation struct {
	Commands []Command
	Reply    string
}

// Interpreter turns free text into commands. It receives the devices that are
// plausible for the text, and its result must use only those devices. An empty
// result means that the text is not a command.
type Interpreter interface {
	Interpret(ctx context.Context, text string, devices []Device) ([]Command, error)
}

// ContextInterpreter is an Interpreter that also receives a read-only snapshot
// of Home Assistant, so it can answer questions and choose an action from the
// current state. It may return a reply instead of commands.
type ContextInterpreter interface {
	InterpretContext(ctx context.Context, text string, devices []Device, context string) (Interpretation, error)
}

// AnswerInterpreter answers an unresolved sentence without choosing an action.
// It is the fallback for conversation that no device action can satisfy.
type AnswerInterpreter interface {
	InterpretAnswer(ctx context.Context, text, homeContext string) (string, error)
}

// ContextProvider returns the read-only Home Assistant context for a model.
type ContextProvider interface {
	Context(ctx context.Context) (string, error)
}

// ActionExecutor runs one action of the generated catalog. The implementation
// checks the action again, so the model can only select an existing one.
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, action actions.Action) error
}

// ActionSource returns the current action catalog, for example the actions of
// the entities exposed in Home Assistant.
type ActionSource interface {
	Actions(ctx context.Context) ([]actions.Action, error)
}

// ActionInterpreter is an Interpreter that can choose actions from the
// generated catalog. It is used before the plain command interpreter for a
// sentence that is not a question.
type ActionInterpreter interface {
	InterpretActions(ctx context.Context, text string, actions []actions.Action, homeContext string) (Interpretation, error)
}

// LearnedActionInterpreter is an ActionInterpreter that also receives the
// active learned associations. It is optional so existing interpreters keep
// working unchanged. The entries are read from the store on every call, so a
// phrase learned later is visible without restarting the assistant.
type LearnedActionInterpreter interface {
	ActionInterpreter
	InterpretActionsWithLearning(ctx context.Context, text string, actions []actions.Action, homeContext string, learned []learning.Entry) (Interpretation, error)
}

// Conversation forwards a sentence to Home Assistant, which can control any
// entity exposed to it and answer questions about it. handled is false when the
// agent did not understand the sentence, so the caller can use its own fallback.
type Conversation interface {
	Converse(ctx context.Context, text string) (reply string, handled bool, err error)
}

// feedback is the last device action, so "mal" can penalize it. The window
// keeps a much later "mal" from punishing an unrelated action.
type feedback struct {
	text   string
	device string
	action string
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
	fallbackOrder FallbackOrder
	actionSafety  ActionSafety
	conversation  Conversation
	contextSource ContextProvider
	room          string
	learning      *learning.Store
	lastAction    feedback

	// dataProvider answers data questions from the exposed entities, and news
	// reads the headlines. Both are optional.
	dataProvider data.Provider
	news         data.News
	dataResolver data.Resolver

	// lastFailed is the last sentence that ended in a failure phrase. "aprende"
	// within failedWindow teaches it.
	lastFailed   string
	lastFailedAt time.Time
	// modelFailed records that the interpreter failed, so the failure phrase
	// can distinguish a missing datum from an unreachable model.
	modelFailed bool
	// lastData is the last data topic that was answered. A short "¿y mañana?"
	// reuses it within dataFollowUp.
	lastData   data.Kind
	lastDataAt time.Time
	// teaching is the state of the "aprende" dialogue.
	teaching teachState

	source    DeviceSource
	every     time.Duration
	report    func(error)
	refreshed time.Time
	fetched   []Device

	actionsByID  map[string]actions.Action
	actionList   []actions.Action
	actionExec   ActionExecutor
	actionSource ActionSource
	actionEvery  time.Duration
	actionReport func(error)
	actionFetch  time.Time
	actionDone   []actions.Action
}

// UseSource makes Handle refresh the allowlist from source when the last
// refresh is older than every. report receives a failed refresh, and the bad
// devices of each new list; the last list stays in use after a failure.
func (h *Handler) UseSource(source DeviceSource, every time.Duration, report func(error)) {
	h.source, h.every, h.report = source, every, report
}

// Refresh fetches the allowlist and the action catalog from their sources now.
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

// RefreshActions fetches the action catalog from the action source now. It is
// a no-op without a source.
func (h *Handler) RefreshActions(ctx context.Context) error {
	if h.actionSource == nil {
		return nil
	}
	h.actionFetch = h.now()
	list, err := h.actionSource.Actions(ctx)
	if err != nil {
		return fmt.Errorf("refresh actions: %w", err)
	}
	problems := h.SetActions(list)
	if !slices.EqualFunc(list, h.actionDone, func(a, b actions.Action) bool { return a.ID == b.ID }) {
		for _, p := range problems {
			h.reportAction(p)
		}
	}
	h.actionDone = list
	return nil
}

func (h *Handler) reportAction(err error) {
	if h.actionReport != nil {
		h.actionReport(err)
	}
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

// UseInference sets whether Home Assistant or the interpreter runs first for
// an unresolved sentence, and which inferred actions may run. Invalid values
// keep the current setting.
func (h *Handler) UseInference(order FallbackOrder, safety ActionSafety) {
	if order.Valid() {
		h.fallbackOrder = order
	}
	if safety.Valid() {
		h.actionSafety = safety
	}
}

// UseConversation sets the Home Assistant conversation fallback for sentences
// that the fixed parser and the local reasoning do not resolve. A nil
// conversation turns it off.
func (h *Handler) UseConversation(c Conversation) {
	h.conversation = c
}

// UseContext sets the read-only Home Assistant context for the interpreter.
// A nil provider leaves the model without context.
func (h *Handler) UseContext(c ContextProvider) {
	h.contextSource = c
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

// UseActions sets the action catalog and the executor for it. A nil executor
// keeps catalog actions off. Bad actions are left out and returned.
func (h *Handler) UseActions(list []actions.Action, executor ActionExecutor) []error {
	h.actionExec = executor
	return h.SetActions(list)
}

// SetActions replaces the action catalog. A bad or duplicate action is left
// out, and the problem is returned.
func (h *Handler) SetActions(list []actions.Action) []error {
	var problems []error
	h.actionsByID = make(map[string]actions.Action, len(list))
	h.actionList = nil
	for _, action := range list {
		if err := actions.Validate(action); err != nil {
			problems = append(problems, err)
			continue
		}
		if other, ok := h.actionsByID[action.ID]; ok {
			problems = append(problems, fmt.Errorf("%w: %q and %q have the same ID", actions.ErrBadAction, other.EntityID, action.EntityID))
			continue
		}
		h.actionsByID[action.ID] = action
		h.actionList = append(h.actionList, action)
	}
	return problems
}

// Actions returns the action catalog in use.
func (h *Handler) Actions() []actions.Action {
	return slices.Clone(h.actionList)
}

// UseActionSource makes Handle refresh the action catalog from source when the
// last refresh is older than every. report receives a failed refresh; the last
// catalog stays in use after a failure.
func (h *Handler) UseActionSource(source ActionSource, every time.Duration, report func(error)) {
	h.actionSource, h.actionEvery, h.actionReport = source, every, report
}

// feedbackWindow is how long after an action "mal" still refers to it.
const feedbackWindow = 60 * time.Second

// maxReplyRunes bounds a model reply that the assistant speaks.
const (
	maxReplyRunes = 600
	// maxChatReplyRunes keeps a chat answer to two short sentences, as the
	// behaviour contract asks.
	maxChatReplyRunes = 400
)

// Devices returns the allowlist in use.
func (h *Handler) Devices() []Device {
	return slices.Clone(h.order)
}

// New checks the allowlist and fails on the first bad device. controller can
// be nil: device commands then get ReplyNoDevices.
func New(now func() time.Time, devices []Device, controller DeviceController) (*Handler, error) {
	h := &Handler{
		now:           now,
		devices:       map[string]Device{},
		controller:    controller,
		matching:      DefaultMatching,
		fallbackOrder: FallbackHAFirst,
		actionSafety:  ActionGuard,
	}
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

// deviceKey returns the normalized name without its article, used to reject duplicates.
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

// UseData sets the deterministic data layer: the exposed entities and the news
// service. A nil provider leaves data questions to the fallback.
func (h *Handler) UseData(provider data.Provider, news data.News) {
	h.dataProvider, h.news = provider, news
	h.dataResolver = data.Resolver{Now: h.now}
}

// FlowActive reports whether a conversational flow is open. The voice pipeline
// keeps listening without a wake word while it is true.
func (h *Handler) FlowActive() bool {
	return h.teaching.step != teachIdle
}

// Handle parses text, optionally asks the interpreter, executes the commands
// and returns the reply.
func (h *Handler) Handle(ctx context.Context, text string) string {
	if h.source != nil && h.now().Sub(h.refreshed) >= h.every {
		if err := h.Refresh(ctx); err != nil {
			h.report(err)
		}
	}
	if h.actionSource != nil && h.now().Sub(h.actionFetch) >= h.actionEvery {
		if err := h.RefreshActions(ctx); err != nil {
			h.reportAction(err)
		}
	}
	h.modelFailed = false

	norm := learningKey(text)
	if norm == "cancelar" {
		// The emergency word closes any conversational flow at once.
		h.teaching = teachState{}
		h.lastFailed = ""
		if h.logger != nil {
			h.logger.Info("cancel", "flow", "global")
		}
		return ReplyOkay
	}
	if h.teaching.step != teachIdle {
		return h.teachingTurn(ctx, text)
	}
	if h.learning != nil && teachTrigger(text) {
		return h.startTeaching(ctx)
	}

	commands := h.ParseAll(text)
	if h.logger != nil {
		for _, cmd := range commands {
			for _, c := range cmd.Corrections {
				h.logger.Log(ctx, h.logLevel, "guess", "heard", c.Heard, "meant", c.Meant, "score", fmt.Sprintf("%.2f", c.Score), "used", c.Used)
			}
		}
	}
	reply := ""
	source := replyNone
	if unresolved(commands) {
		if cmd, blocked, ok := h.reasoned(ctx, text); ok {
			commands = []Command{cmd}
			source = commandSource(cmd)
		} else if matched, ok := h.localCatalogAction(ctx, text, blocked); ok {
			commands = matched
		} else if answer, result := h.answerData(ctx, text); result != data.NoMatch {
			if result == data.Answered {
				reply, source = answer, replyData
			} else {
				reply = answer
			}
		} else if answer, ok := h.physicalAnswer(ctx, text); ok {
			reply, source = answer, replyPhysical
		} else if h.fallbackOrder == FallbackModelFirst {
			if interpreted, ok := h.interpretReasoned(ctx, text, blocked); ok {
				commands, reply = interpreted.Commands, interpreted.Reply
				if reply != "" {
					source = replyModel
				}
			} else if answer, ok := h.askConversation(ctx, text); ok {
				reply, source = answer, replyConversation
			}
		} else {
			if answer, ok := h.askConversation(ctx, text); ok {
				reply, source = answer, replyConversation
			} else if interpreted, ok := h.interpretReasoned(ctx, text, blocked); ok {
				commands, reply = interpreted.Commands, interpreted.Reply
				if reply != "" {
					source = replyModel
				}
			}
		}
	}
	// A learned entry is a sentence-to-one-target association, so a sentence
	// with several actions is not stored and "mal" has nothing to correct.
	if len(commands) == 1 {
		commands[0].Text = learningKey(text)
	}
	if reply == "" && !unresolved(commands) {
		reply = h.executeAll(ctx, commands)
	}
	if reply == "" {
		reply = h.failureReply(ctx, text)
	} else if (source == replyModel || source == replyConversation) && data.Detect(text).IsData() {
		// The first answer to a new data phrasing may need the model. It is
		// stored as a template, never as frozen text.
		h.rememberTemplate(ctx, text, reply)
	}
	if source == replyModel || source == replyConversation || source == replyData || source == replyPhysical {
		h.recordAnswer(source, text)
	}
	// A fallback refusal is not an answer: keep the sentence teachable with
	// "aprende", even though the refusal itself is spoken.
	if (source == replyConversation || source == replyModel) && isRefusal(reply) {
		h.lastFailed, h.lastFailedAt = strings.TrimSpace(text), h.now()
	}
	if !slices.ContainsFunc(commands, func(c Command) bool {
		return c.Kind == KindRepeat || c.Kind == KindPenalty
	}) {
		h.previous = strings.TrimSpace(text)
	}
	return reply
}

type replySource int

const (
	replyNone replySource = iota
	replyData
	replyPhysical
	replyModel
	replyConversation
	replyLearnedData
	replyLearnedReply
)

func commandSource(cmd Command) replySource {
	switch cmd.Kind {
	case KindData:
		return replyLearnedData
	case KindReply:
		return replyLearnedReply
	}
	return replyNone
}

// isRefusal reports whether a fallback answer says that it cannot help. The
// Home Assistant agent answers many unsupported orders with such a sentence,
// and the user must still be able to teach them.
func isRefusal(reply string) bool {
	norm := learningKey(reply)
	for _, phrase := range []string{
		"no puedo", "no he podido", "no se", "no lo se", "no te he entendido",
		"no te entiendo", "no estoy seguro", "no tengo informacion",
		"no dispongo", "lo siento", "perdona", "no soy capaz",
	} {
		if strings.Contains(" "+norm+" ", " "+phrase+" ") {
			return true
		}
	}
	return false
}

// failureReply is the phrase for a sentence that no layer could handle. It also
// remembers the sentence for the "aprende" flow.
func (h *Handler) failureReply(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	h.lastFailed, h.lastFailedAt = text, h.now()
	var reply string
	switch {
	case data.Detect(text).IsData():
		reply = ReplyDataMissing
		if h.modelFailed {
			reply = ReplyModelDown
		}
	case looksLikeQuestion(text):
		// A chat question has no data source; the model was the answer
		// source and it did not produce a usable reply.
		reply = ReplyModelDown
	default:
		reply = fmt.Sprintf("Aún no sé procesar la orden %s.", asClause(text))
	}
	if h.logger != nil {
		h.logger.Info("failure", "reply", reply, "model_failed", h.modelFailed)
	}
	return reply
}

// reasoned returns a learned or deterministic action for an unresolved
// sentence, and the learned entry that blocks the model when the user punished
// that action.
func (h *Handler) reasoned(ctx context.Context, text string) (Command, *learning.Entry, bool) {
	key := learningKey(text)
	var blocked *learning.Entry
	if h.learning != nil {
		if e, found := h.learning.Lookup(key); found {
			if e.Score > 0 {
				if cmd, ok := h.commandFromEntry(ctx, e); ok {
					if h.logger != nil {
						attrs := []any{"kind", cmd.Kind, "source", "learned", "score", e.Score}
						switch cmd.Kind {
						case KindAction:
							attrs = append(attrs, "action", cmd.Action.ID, "entity", cmd.Action.EntityID)
						case KindData:
							attrs = append(attrs, "template", e.Template)
						case KindReply:
							attrs = append(attrs, "reply", e.Reply)
						default:
							attrs = append(attrs, "device", cmd.Device.ID)
						}
						h.logger.Log(ctx, h.logLevel, "reasoned", attrs...)
					}
					return cmd, nil, true
				}
			} else {
				blocked = &e
			}
		}
	}
	if cmd, ok := h.stateAction(text); ok {
		if blocked == nil || blocked.Action != "" || cmd.Device.ID != blocked.Device || string(cmd.Kind) != blocked.Kind {
			if h.logger != nil {
				h.logger.Log(ctx, h.logLevel, "reasoned", "kind", cmd.Kind, "device", cmd.Device.ID, "source", "state")
			}
			return cmd, blocked, true
		}
	}
	return Command{}, blocked, false
}

// localCatalogAction matches a high-confidence catalog phrase without the
// model or Home Assistant. It runs after learned memory and before the
// fallback order, so common commands stay fast and are learned.
func (h *Handler) localCatalogAction(ctx context.Context, text string, blocked *learning.Entry) ([]Command, bool) {
	if h.actionExec == nil || len(h.actionList) == 0 {
		return nil, false
	}
	matched, ok := matchCatalogActions(text, h.actionList)
	if !ok {
		return nil, false
	}
	commands := make([]Command, 0, len(matched))
	for _, action := range matched {
		if blocked != nil && blocked.Action == action.ID {
			continue
		}
		if h.logger != nil {
			h.logger.Log(ctx, h.logLevel, "catalog", "action", action.ID, "entity", action.EntityID)
		}
		commands = append(commands, Command{Kind: KindAction, Action: action, Source: "catalog"})
	}
	if len(commands) == 0 {
		return nil, false
	}
	return commands, true
}

// interpretReasoned asks the interpreter with every available device and the
// read-only Home Assistant context. The model can infer an action, which runs
// at once, or answer a question from the context.
func (h *Handler) interpretReasoned(ctx context.Context, text string, blocked *learning.Entry) (Interpretation, bool) {
	if h.interpreter == nil || h.interpretMode == InterpretOff {
		return Interpretation{}, false
	}
	devices := make([]Device, 0, len(h.order))
	for _, d := range h.order {
		if !d.Unavailable {
			devices = append(devices, d)
		}
	}
	homeContext := h.homeContext(ctx)
	interpreted, ok := Interpretation{}, false
	if !LooksLikeQuestion(text) || data.HasActionVerb(text) {
		interpreted, ok = h.askActions(ctx, text, homeContext)
	}
	if !ok && len(devices) > 0 {
		interpreted, ok = h.askInterpreter(ctx, text, devices, devices, homeContext)
	}
	if !ok && (!LooksLikeQuestion(text) || len(devices) == 0) {
		if reply, answered := h.askAnswer(ctx, text, homeContext); answered {
			interpreted, ok = Interpretation{Reply: reply}, true
		}
	}
	if !ok {
		return Interpretation{}, false
	}
	interpreted.Reply = limitReply(interpreted.Reply)
	if len(interpreted.Commands) == 0 {
		if !h.acceptsReply(text, interpreted.Reply) {
			if h.logger != nil {
				h.logger.Log(ctx, slog.LevelDebug, "reasoned rejected", "safety", h.actionSafety, "kind", "reply")
			}
			return Interpretation{}, false
		}
		if h.logger != nil {
			h.logger.Log(ctx, h.logLevel, "reasoned", "kind", "reply", "source", "model")
		}
		return Interpretation{Reply: interpreted.Reply}, true
	}
	accepted := make([]Command, 0, len(interpreted.Commands))
	for _, cmd := range interpreted.Commands {
		if !h.acceptsReasoned(text, cmd) {
			if h.logger != nil {
				attrs := []any{"safety", h.actionSafety, "kind", cmd.Kind}
				if cmd.Kind == KindAction {
					attrs = append(attrs, "action", cmd.Action.ID)
				} else {
					attrs = append(attrs, "device", cmd.Device.ID)
				}
				h.logger.Log(ctx, slog.LevelDebug, "reasoned rejected", attrs...)
			}
			continue
		}
		if blocked != nil {
			if blocked.Action != "" {
				if cmd.Kind == KindAction && cmd.Action.ID == blocked.Action {
					continue
				}
			} else if cmd.Device.ID == blocked.Device && string(cmd.Kind) == blocked.Kind {
				continue
			}
		}
		cmd.Source = "reasoned"
		accepted = append(accepted, cmd)
		if h.logger != nil {
			attrs := []any{"kind", cmd.Kind, "source", "model"}
			if cmd.Kind == KindAction {
				attrs = append(attrs, "action", cmd.Action.ID, "entity", cmd.Action.EntityID)
			} else {
				attrs = append(attrs, "device", cmd.Device.ID)
			}
			h.logger.Log(ctx, h.logLevel, "reasoned", attrs...)
		}
	}
	if len(accepted) == 0 {
		return Interpretation{}, false
	}
	return Interpretation{Commands: accepted}, true
}

// unresolved reports whether the parser did not produce an executable command.
func unresolved(commands []Command) bool {
	return len(commands) == 1 && commands[0].Kind == KindUnknown
}

// askConversation forwards the sentence to Home Assistant, which can control
// any exposed entity and answer questions. It returns false when no conversation
// is configured, the agent did not understand, or the call failed.
func (h *Handler) askConversation(ctx context.Context, text string) (string, bool) {
	if h.conversation == nil {
		return "", false
	}
	reply, handled, err := h.conversation.Converse(ctx, text)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("Home Assistant conversation", "error", err)
		}
		return "", false
	}
	if !handled {
		return "", false
	}
	if h.logger != nil {
		h.logger.Log(ctx, slog.LevelInfo, "conversation", "source", "homeassistant")
	}
	return limitReply(reply), true
}

// limitReply cuts a long chat answer at a sentence boundary within
// maxChatReplyRunes, so a chatty model cannot read a paragraph aloud.
func limitReply(reply string) string {
	runes := []rune(strings.TrimSpace(reply))
	if len(runes) <= maxChatReplyRunes {
		return string(runes)
	}
	cut := string(runes[:maxChatReplyRunes])
	if index := strings.LastIndexAny(cut, ".!?"); index >= 0 {
		cut = cut[:index+1]
	}
	return strings.TrimSpace(cut)
}

// commandFromEntry turns a learned entry into a command, using the current
// allowlist and snapshot: a device or template that no longer exists is not
// used. A pending entry says that it is pending and never acts.
func (h *Handler) commandFromEntry(ctx context.Context, e learning.Entry) (Command, bool) {
	if e.Pending {
		return Command{Kind: KindPending, Source: "learned", Text: e.Text}, true
	}
	if e.Template != "" {
		if h.dataProvider == nil {
			return Command{}, false
		}
		snapshot, err := h.dataProvider.DataSnapshot(ctx)
		if err != nil {
			return Command{}, false
		}
		text, err := data.Render(e.Template, snapshot)
		if err != nil {
			if h.logger != nil {
				h.logger.Warn("data template", "template", e.Template, "error", err)
			}
			return Command{}, false
		}
		return Command{Kind: KindData, Reply: text, Template: e.Template, Source: "learned", Text: e.Text}, true
	}
	if e.Reply != "" {
		return Command{Kind: KindReply, Reply: e.Reply, Source: "learned", Text: e.Text}, true
	}
	if e.Action != "" {
		action, ok := h.actionsByID[e.Action]
		if !ok || h.actionExec == nil {
			return Command{}, false
		}
		return Command{Kind: KindAction, Action: action, Source: "learned", Text: e.Text}, true
	}
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

// acceptsReasoned applies the configured action safety. Guard accepts a
// device that the sentence names, or a physical state that the device can fix.
// States accepts only the latter, and Literal accepts every action.
func (h *Handler) acceptsReasoned(text string, cmd Command) bool {
	if h.actionSafety == ActionLiteral {
		return true
	}
	// An action verb wins over the question shape: "¿puedes apagar la tele?"
	// is still an order. A question without one is an information request.
	if looksLikeQuestion(text) && !data.HasActionVerb(text) {
		return false
	}
	if cmd.Kind == KindAction && h.learnedActionAllows(text, cmd.Action) {
		return true
	}
	if cmd.Kind == KindAction {
		return h.acceptsAction(text, cmd.Action)
	}
	named, namedOK := h.namedDevice(text)
	if h.actionSafety == ActionStates {
		intent := physicalIntent(text)
		return intent != intentNone && h.deviceForIntent(named, namedOK, cmd.Device, intent)
	}
	if intent := physicalIntent(text); intent != intentNone {
		return h.deviceForIntent(named, namedOK, cmd.Device, intent)
	}
	return namedOK && named.ID == cmd.Device.ID
}

// learnedActionAllows accepts an action chosen from a learned association when
// the sentence shares a significant word with the learned phrase. It lets a
// variant such as "macro uno" use the action learned as "macro 1" without
// allowing an arbitrary unmentioned device.
func (h *Handler) learnedActionAllows(text string, action actions.Action) bool {
	words := significantWords(text)
	if len(words) == 0 {
		return false
	}
	for _, entry := range h.learnedEntries() {
		if entry.Action != action.ID {
			continue
		}
		entryWords := significantWords(entry.Text)
		if slices.ContainsFunc(words, func(word string) bool {
			return slices.Contains(entryWords, word)
		}) {
			return true
		}
	}
	return false
}

// acceptsAction applies the same safety to a catalog action. A physical state
// must be fixable by the action's domain; another sentence must name the
// entity, the action or one of its aliases.
func (h *Handler) acceptsAction(text string, action actions.Action) bool {
	if intent := physicalIntent(text); intent != intentNone {
		return actionFitsIntent(action, intent)
	}
	if guardedDomains[action.Domain] {
		intents := actionIntents(learningKey(text))
		if len(intents) == 0 {
			return false
		}
		suffix := actionSuffix(action)
		switch {
		case strings.HasPrefix(suffix, "mode_"),
			strings.HasPrefix(suffix, "preset_"),
			strings.HasPrefix(suffix, "option_"):
			// The sentence names the mode or option itself.
		case !slices.Contains(intents, suffix):
			return false
		}
	}
	return actionNamed(text, action)
}

// actionFitsIntent reports whether an action of the catalog can plausibly fix
// the physical problem.
func actionFitsIntent(action actions.Action, intent intentKind) bool {
	switch intent {
	case intentLight:
		return action.Domain == "light"
	case intentTemperature:
		return action.Domain == "climate" || action.Domain == "water_heater"
	case intentAir:
		return action.Domain == "fan" || action.Domain == "humidifier" || action.Domain == "climate"
	}
	return false
}

// actionNamed reports whether the sentence names the entity, the action or one
// of its aliases.
func actionNamed(text string, action actions.Action) bool {
	words := significantWords(text)
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		for _, name := range actionMentionWords(action) {
			if similarWord(word, name) {
				return true
			}
		}
	}
	return false
}

// deviceForIntent keeps a physical-state action on the device that the sentence
// names, or on any device that can fix the state when it names none.
func (h *Handler) deviceForIntent(named Device, namedOK bool, chosen Device, intent intentKind) bool {
	if namedOK {
		return named.ID == chosen.ID
	}
	return deviceFitsIntent(chosen, intent)
}

// looksLikeQuestion rejects a question, which asks for information rather
// than for a device action. Speech recognition may drop the punctuation, so
// the first word is checked too.
// LooksLikeQuestion reports whether a sentence has the shape of a question or
// a future statement such as "va a llover". The handler uses it to decide
// whether a model reply may be spoken; the language-model prompt uses it to
// choose the small context-only prompt.
func LooksLikeQuestion(text string) bool {
	return looksLikeQuestion(text)
}

func looksLikeQuestion(text string) bool {
	if strings.ContainsAny(text, "?¿") {
		return true
	}
	first, _, _ := strings.Cut(strings.TrimSpace(normalize(text)), " ")
	switch first {
	case "que", "cuanto", "cuanta", "cuantos", "cuantas", "como", "cuando",
		"donde", "quien", "quienes", "cual", "cuales",
		"va", "van", "vamos", "sera", "seran", "hara", "haran":
		return true
	}
	return false
}

type intentKind int

const (
	intentNone intentKind = iota
	intentLight
	intentTemperature
	intentAir
)

// physicalIntent classifies a sentence about a physical problem. It is wider
// than stateAction: the model can still choose the action and the device. A
// question without an action verb is not a physical state, and a physical word
// alone is not an intent: "¿qué temperatura hace?" asks for a datum.
func physicalIntent(text string) intentKind {
	if looksLikeQuestion(text) && !data.HasActionVerb(text) {
		return intentNone
	}
	norm := learningKey(text)
	switch {
	case containsAny(norm,
		"me deslumbra",
		"no veo", "no puedo ver", "poca luz", "mucha luz", "demasiada luz",
		"poca claridad", "mucha claridad", "demasiada claridad",
		"mas luz", "mas claridad", "sin luz", "necesito luz", "me falta luz", "me sobra luz",
		"esta oscuro", "esta muy oscuro", "esta claro", "esta muy claro",
		"mucho brillo", "demasiado brillo", "me sobra brillo"):
		return intentLight
	case containsAny(norm, "frio", "calor", "temperatura", "grados"):
		if !data.HasActionVerb(text) && !firstPersonState(text) {
			return intentNone
		}
		return intentTemperature
	case containsAny(norm, "humedad", "bochorno"):
		if !data.HasActionVerb(text) && !firstPersonState(text) {
			return intentNone
		}
		return intentAir
	}
	return intentNone
}

// firstPersonState reports whether the sentence describes the user's state
// rather than asking for a datum. "Tengo frío" and "no veo nada" are states;
// "temperatura" alone is not.
func firstPersonState(text string) bool {
	norm := learningKey(text)
	return containsAny(norm,
		"tengo", "estoy", "no veo", "no puedo ver", "necesito", "quiero",
		"me falta", "me sobra", "me deslumbra", "me duele",
		"hay poca", "hay mucha", "hace frio", "hace calor",
		"esta oscuro", "esta muy oscuro", "esta claro", "esta muy claro")
}

// deviceFitsIntent reports whether the device can plausibly fix the problem.
func deviceFitsIntent(d Device, intent intentKind) bool {
	name := normalize(d.Name)
	switch intent {
	case intentLight:
		return containsAny(name,
			"luz", "luces", "lampara", "lamparas", "led", "iluminacion",
			"bombilla", "bombillas", "foco", "focos", "plafon", "plafones")
	case intentTemperature:
		return containsAny(name,
			"aire", "calefaccion", "calefactor", "calentador", "radiador",
			"estufa", "termo", "termoestato", "clima", "split", "calor")
	case intentAir:
		return containsAny(name,
			"ventilador", "ventiladores", "ventilacion", "aire", "extractor",
			"extractores", "fan", "humedad", "deshumidificador")
	}
	return false
}

// namedDevice returns the one device whose name the sentence clearly names.
// It ignores articles and filler words, and requires a lead over the next
// device so that "la luz" with two rooms does not pick one at random.
func (h *Handler) namedDevice(text string) (Device, bool) {
	words := significantWords(text)
	if len(words) == 0 {
		return Device{}, false
	}
	type scored struct {
		device Device
		score  float64
	}
	ranked := make([]scored, 0, len(h.order))
	for _, d := range h.order {
		if d.Unavailable {
			continue
		}
		names := significantWords(d.Name)
		if len(names) == 0 {
			continue
		}
		matches := 0
		for _, name := range names {
			for _, word := range words {
				if sameWord(word, name) {
					matches++
					break
				}
			}
		}
		ranked = append(ranked, scored{device: d, score: float64(matches) / float64(len(names))})
	}
	slices.SortFunc(ranked, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.device.ID, b.device.ID))
	})
	if len(ranked) == 0 || ranked[0].score <= 0.5 {
		return Device{}, false
	}
	if len(ranked) > 1 && ranked[0].score-ranked[1].score < h.matching.Margin {
		return Device{}, false
	}
	return ranked[0].device, true
}

// sameWord reports whether two normalized words name the same thing, allowing
// a simple plural. Prefixes are not enough: "cocinar" must not name "cocina".
func sameWord(a, b string) bool {
	return a == b || a == b+"s" || b == a+"s"
}

// significantWords returns the words of text that can identify a device name.
func significantWords(text string) []string {
	var words []string
	for _, word := range strings.Fields(normalize(text)) {
		if len([]rune(word)) < 3 || articles[word] || fillerWords[word] {
			continue
		}
		words = append(words, word)
	}
	return words
}

var fillerWords = map[string]bool{
	"aqui": true, "ahora": true, "con": true, "del": true, "en": true,
	"favor": true, "la": true, "las": true, "lo": true, "los": true,
	"me": true, "mi": true, "mis": true, "para": true, "por": true,
	"porfa": true, "que": true, "se": true, "sin": true, "tu": true,
	"tus": true, "un": true, "una": true, "unas": true, "unos": true,
	"ya": true,
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

// askActions asks the interpreter to choose actions of the generated
// catalog. It returns false when the interpreter has no action support, the
// catalog is empty, or the answer names no known action.
func (h *Handler) askActions(ctx context.Context, text, homeContext string) (Interpretation, bool) {
	if len(h.actionList) == 0 || h.actionExec == nil {
		return Interpretation{}, false
	}
	if matched, ok := matchCatalogActions(text, h.actionList); ok {
		commands := make([]Command, 0, len(matched))
		for _, action := range matched {
			if h.logger != nil {
				h.logger.Log(ctx, h.logLevel, "catalog", "action", action.ID)
			}
			commands = append(commands, Command{Kind: KindAction, Action: action})
		}
		return Interpretation{Commands: commands}, true
	}
	interpreter, ok := h.interpreter.(ActionInterpreter)
	if !ok {
		return Interpretation{}, false
	}
	list := slices.Clone(h.actionList)
	var got Interpretation
	var err error
	if learned, ok := h.interpreter.(LearnedActionInterpreter); ok {
		got, err = learned.InterpretActionsWithLearning(ctx, text, list, homeContext, h.learnedEntries())
	} else {
		got, err = interpreter.InterpretActions(ctx, text, list, homeContext)
	}
	if err != nil {
		h.modelFailed = true
		if h.logger != nil {
			h.logger.Warn("language model", "error", err)
		}
		return Interpretation{}, false
	}
	got.Commands = h.allowActions(got.Commands)
	got.Reply = strings.TrimSpace(got.Reply)
	if len(got.Commands) == 0 && got.Reply == "" {
		return Interpretation{}, false
	}
	return got, true
}

// learnedEntries returns the active sentence-to-action associations. The store
// is snapshotted per call, so a phrase learned seconds before is already here.
func (h *Handler) learnedEntries() []learning.Entry {
	if h.learning == nil {
		return nil
	}
	entries := h.learning.Snapshot(h.now()).Entries
	active := make([]learning.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Score > 0 {
			active = append(active, entry)
		}
	}
	return active
}

// allowActions keeps only catalog actions that still exist in the current
// catalog, whatever the model returned.
func (h *Handler) allowActions(commands []Command) []Command {
	valid := make([]Command, 0, len(commands))
	for _, c := range commands {
		c.Corrections = nil
		if c.Kind != KindAction || h.actionExec == nil {
			continue
		}
		action, ok := h.actionsByID[c.Action.ID]
		if !ok {
			continue
		}
		c.Action = action
		valid = append(valid, c)
	}
	return valid
}

// askInterpreter calls the interpreter, keeps only the commands that the
// allowlist and allowed devices permit, and returns its reply, if any.
func (h *Handler) askInterpreter(ctx context.Context, text string, devices, allowed []Device, homeContext string) (Interpretation, bool) {
	var got Interpretation
	var err error
	if contextInterpreter, ok := h.interpreter.(ContextInterpreter); ok {
		got, err = contextInterpreter.InterpretContext(ctx, text, devices, homeContext)
	} else {
		got.Commands, err = h.interpreter.Interpret(ctx, text, devices)
	}
	if err != nil {
		h.modelFailed = true
		if h.logger != nil {
			h.logger.Warn("language model", "error", err)
		}
		return Interpretation{}, false
	}
	got.Commands = h.allowInterpreted(got.Commands, allowed)
	got.Reply = strings.TrimSpace(got.Reply)
	if len(got.Commands) == 0 && got.Reply == "" {
		return Interpretation{}, false
	}
	return got, true
}

// askAnswer asks for a general answer after the action paths found nothing.
func (h *Handler) askAnswer(ctx context.Context, text, homeContext string) (string, bool) {
	answerer, ok := h.interpreter.(AnswerInterpreter)
	if !ok {
		return "", false
	}
	reply, err := answerer.InterpretAnswer(ctx, text, homeContext)
	if err != nil {
		h.modelFailed = true
		if h.logger != nil {
			h.logger.Warn("language model", "error", err)
		}
		return "", false
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", false
	}
	return reply, true
}

// homeContext returns the Home Assistant context for the model. A failure is
// not fatal: the model still gets the devices and can answer nothing from the
// missing context.
func (h *Handler) homeContext(ctx context.Context) string {
	if h.contextSource == nil {
		return ""
	}
	homeContext, err := h.contextSource.Context(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("Home Assistant context", "error", err)
		}
		return ""
	}
	return strings.TrimSpace(homeContext)
}

// acceptsReply reports whether a model reply is safe to speak. Replies are
// limited in length, must not be an echo of the sentence, and must contain
// words. A question that names a device or a physical word is still a question:
// rejecting its answer here was the bug that made "¿qué temperatura hace?"
// impossible to answer.
func (h *Handler) acceptsReply(text, reply string) bool {
	reply = strings.TrimSpace(reply)
	if reply == "" || len([]rune(reply)) > maxReplyRunes {
		return false
	}
	if !strings.ContainsFunc(reply, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return false
	}
	return !replyEchoes(text, reply)
}

// replyEchoes reports whether most words of the reply also appear in the
// sentence. It rejects a model that parrots the question with small changes,
// which the old exact-match check missed.
func replyEchoes(text, reply string) bool {
	replyWords := strings.Fields(normalize(reply))
	if len(replyWords) < 2 {
		return false
	}
	seen := make(map[string]struct{}, len(replyWords))
	for _, w := range strings.Fields(normalize(text)) {
		seen[w] = struct{}{}
	}
	common := 0
	for _, w := range replyWords {
		if _, ok := seen[w]; ok {
			common++
		}
	}
	return float64(common)/float64(len(replyWords)) >= 0.8
}

// allowInterpreted drops a command with a device that is not in the allowlist
// or not allowed, and drops the fixed commands, which the deterministic parser
// handles.
func (h *Handler) allowInterpreted(commands []Command, allowed []Device) []Command {
	valid := make([]Command, 0, len(commands))
	for _, c := range commands {
		c.Corrections = nil
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

// ParseAll parses text as one fixed command, or as clauses joined by "y" or
// "e" when every clause is a fixed command. It has no side effects.
func (h *Handler) ParseAll(text string) []Command {
	cmd := h.Parse(text)
	if cmd.Kind != KindUnknown {
		return []Command{cmd}
	}
	parts := splitClauses(withoutFillers(normalize(text)))
	if len(parts) < 2 {
		return []Command{cmd}
	}
	commands := make([]Command, 0, len(parts))
	for _, part := range parts {
		c := h.Parse(part)
		if c.Kind == KindUnknown {
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

// Parse maps text to a fixed command. It has no side effects. When a phrase
// does not match exactly, it takes the one that sounds most alike if the guess
// is good and clear enough (see Matching); it never guesses a device.
func (h *Handler) Parse(text string) Command {
	norm := withoutFillers(normalize(text))
	if kind, ok := phrases[norm]; ok {
		return Command{Kind: kind}
	}
	if strings.TrimSpace(norm) == "" {
		return Command{Kind: KindUnknown}
	}
	if kind, c := h.guessKind(norm, phrases); c != nil {
		return Command{Kind: kind, Corrections: []Correction{*c}}
	}
	return Command{Kind: KindUnknown}
}

// Execute runs cmd and returns the reply. Only KindTurnOn and KindTurnOff
// with an allowlisted device reach the controller.
func (h *Handler) Execute(ctx context.Context, cmd Command) string {
	switch cmd.Kind {
	case KindTime:
		return spokenTime(h.now())
	case KindDate:
		return spokenDate(h.now())
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
	case KindAction:
		return h.runAction(ctx, cmd)
	case KindData, KindReply:
		return cmd.Reply
	case KindPending:
		return ReplyPending
	case KindPenalty:
		return h.penalize(ctx)
	default:
		return ReplyUnknown
	}
}

func (h *Handler) setPower(ctx context.Context, cmd Command) string {
	device, on, source := cmd.Device, cmd.Kind == KindTurnOn, cmd.Source
	if source == "" {
		source = "direct"
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

// runAction runs one catalog action through its executor. The action was
// checked against the current catalog by allowActions or commandFromEntry.
func (h *Handler) runAction(ctx context.Context, cmd Command) string {
	action, source := cmd.Action, cmd.Source
	if source == "" {
		source = "direct"
	}
	device := Device{ID: action.EntityID, Name: action.Name}
	if h.actionExec == nil {
		h.logAction(ctx, source, KindAction, device, "no executor", 0)
		return ReplyUnknown
	}
	start := time.Now()
	err := h.actionExec.ExecuteAction(ctx, action)
	if err != nil {
		h.logAction(ctx, source, KindAction, device, err.Error(), time.Since(start))
		return fmt.Sprintf("No he podido %s.", lowerFirst(action.Name))
	}
	h.logAction(ctx, source, KindAction, device, "", time.Since(start))
	h.recordSuccess(ctx, cmd)
	return ReplyDone
}

// lowerFirst lowercases the first rune of a generated action name, so that it
// fits in a sentence.
func lowerFirst(text string) string {
	runes := []rune(text)
	if len(runes) == 0 || unicode.IsLower(runes[0]) {
		return text
	}
	return string(unicode.ToLower(runes[0])) + string(runes[1:])
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
	// Without a sentence there is no association to learn or correct, for
	// example when one sentence requested several actions.
	if cmd.Text == "" {
		return
	}
	source := cmd.Source
	if source == "" {
		source = "direct"
	}
	h.lastAction = feedback{text: cmd.Text, device: cmd.Device.ID, kind: cmd.Kind, source: source, at: h.now(), valid: true}
	switch cmd.Kind {
	case KindAction:
		h.lastAction.device = cmd.Action.EntityID
		h.lastAction.action = cmd.Action.ID
	case KindData, KindReply:
		h.lastAction.device = ""
	}
	if h.learning == nil || !learnable(source) {
		return
	}
	var entry learning.Entry
	var err error
	switch cmd.Kind {
	case KindAction:
		entry, err = h.learning.LearnAction(cmd.Text, cmd.Action.ID, cmd.Action.Name, h.now())
	case KindData:
		entry, err = h.learning.LearnTemplate(cmd.Text, cmd.Template, h.now())
	case KindReply:
		entry, err = h.learning.LearnReply(cmd.Text, cmd.Reply, h.now())
	default:
		entry, err = h.learning.Learn(cmd.Text, cmd.Device.ID, cmd.Device.Name, string(cmd.Kind), h.now())
	}
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("learning", "error", err)
		}
		return
	}
	if h.logger != nil {
		attrs := []any{"result", "success", "score", entry.Score, "successes", entry.Successes, "failures", entry.Failures, "kind", entry.Kind}
		if entry.Action != "" {
			attrs = append(attrs, "action", entry.Action)
		} else if entry.Device != "" {
			attrs = append(attrs, "device", entry.Device)
		}
		h.logger.Info("learning", attrs...)
	}
}

// recordAnswer remembers a spoken answer so a later "mal" can penalize it. A
// built-in answer has no entry, so the penalty has nothing to change; a learned
// template or reply does.
func (h *Handler) recordAnswer(source replySource, text string) {
	kind := KindReply
	switch source {
	case replyData, replyPhysical, replyLearnedData:
		kind = KindData
	}
	h.lastAction = feedback{text: learningKey(text), kind: kind, source: "data", at: h.now(), valid: true}
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
	if h.learning != nil && last.text != "" && (learnable(last.source) || last.kind == KindData || last.kind == KindReply) {
		var entry learning.Entry
		var ok bool
		var err error
		switch {
		case last.kind == KindData || last.kind == KindReply:
			entry, ok, err = h.learning.PenalizeEntry(last.text, h.now())
		case last.action != "":
			entry, ok, err = h.learning.PenalizeAction(last.text, last.action, h.now())
		default:
			entry, ok, err = h.learning.Penalize(last.text, last.device, string(last.kind), h.now())
		}
		switch {
		case err != nil:
			if h.logger != nil {
				h.logger.Warn("learning", "error", err)
			}
		case ok:
			learned = true
			if h.logger != nil {
				attrs := []any{"result", "penalty", "score", entry.Score, "successes", entry.Successes, "failures", entry.Failures, "kind", entry.Kind}
				if entry.Action != "" {
					attrs = append(attrs, "action", entry.Action)
				} else {
					attrs = append(attrs, "device", entry.Device)
				}
				h.logger.Info("learning", attrs...)
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
	case "interpreted", "reasoned", "state", "learned", "catalog":
		return true
	}
	return false
}

// learningKey is the normalized sentence without fillers, the store key.
func learningKey(text string) string {
	return learning.Key(text)
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
	return learning.Normalize(text)
}

func withoutArticle(phrase string) string {
	first, rest, found := strings.Cut(phrase, " ")
	if found && articles[first] {
		return rest
	}
	return phrase
}

// withoutFillers removes words that do not change the command from a
// normalized sentence.
func withoutFillers(norm string) string {
	return learning.WithoutFillers(norm)
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

var weekdaysSpanish = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

var monthsSpanish = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// spokenDate gives the date in spoken Spanish, for example "Hoy es martes, 24
// de noviembre de 2026.".
func spokenDate(t time.Time) string {
	return fmt.Sprintf("Hoy es %s, %d de %s de %d.",
		weekdaysSpanish[int(t.Weekday())], t.Day(), monthsSpanish[int(t.Month())-1], t.Year())
}
