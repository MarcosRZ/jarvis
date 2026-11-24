package commands

import (
	"context"
	"strings"
	"time"

	"jarvis/actions"
	"jarvis/client/internal/data"
	"jarvis/client/internal/learning"
)

// failedWindow is how long a sentence that ended in a failure phrase stays
// available to the "aprende" keyword.
const failedWindow = 60 * time.Second

// teachStep is the point of the teaching dialogue that the next turn answers.
type teachStep int

const (
	teachIdle teachStep = iota
	// teachConfirm waits for one of the exact words sí, no or cancelar.
	teachConfirm
	// teachSentence waits for the sentence to teach.
	teachSentence
	// teachTarget waits for the action or the answer that the sentence means.
	teachTarget
)

// teachState is the teaching dialogue. Only one flow is open at a time.
type teachState struct {
	step     teachStep
	sentence string
	// question tells the prompts whether the sentence asks or orders.
	question bool
}

// teachTrigger reports whether the sentence contains the keyword "aprende".
// The pipeline strips the wake word, so the keyword is the whole trigger.
func teachTrigger(text string) bool {
	return containsAny(learningKey(text), "aprende")
}

// startTeaching opens the teaching flow. A failure within failedWindow is
// reused; otherwise the assistant asks for the sentence first.
func (h *Handler) startTeaching(ctx context.Context) string {
	if h.learning == nil {
		return ReplyStoreFailed
	}
	sentence := ""
	if h.lastFailed != "" && h.now().Sub(h.lastFailedAt) <= failedWindow {
		sentence = h.lastFailed
	}
	if sentence == "" {
		h.teaching = teachState{step: teachSentence}
		return ReplyTeachSentence
	}
	h.teaching = teachState{
		step:     teachConfirm,
		sentence: sentence,
		question: isQuestion(sentence),
	}
	return h.confirmPrompt()
}

// teachingTurn handles one reply of the open dialogue. The caller has already
// checked the global emergency word, so "cancelar" never reaches this method.
func (h *Handler) teachingTurn(ctx context.Context, text string) string {
	norm := learningKey(text)
	switch h.teaching.step {
	case teachConfirm:
		switch norm {
		case "si":
			h.teaching.step = teachTarget
			return h.targetPrompt()
		case "no":
			h.teaching.step = teachSentence
			return h.tellPrompt()
		default:
			return h.confirmPrompt()
		}
	case teachSentence:
		sentence := strings.TrimSpace(text)
		if sentence == "" || norm == "" {
			return ReplyTeachSentence
		}
		h.teaching.sentence = sentence
		h.teaching.question = isQuestion(sentence)
		h.teaching.step = teachConfirm
		return h.confirmPrompt()
	case teachTarget:
		return h.finishTeaching(ctx, text)
	}
	return ""
}

// finishTeaching stores the target of the confirmed sentence.
func (h *Handler) finishTeaching(ctx context.Context, text string) string {
	answer := strings.TrimSpace(text)
	if answer == "" || learningKey(answer) == "" {
		return h.targetPrompt()
	}
	key := learningKey(h.teaching.sentence)
	if key == "" {
		h.teaching = teachState{}
		return ReplyStoreFailed
	}
	if h.teaching.question {
		return h.finishQuestion(ctx, key, answer)
	}
	return h.finishOrder(ctx, key, answer)
}

// finishOrder stores one catalog action, or a pending entry when no action can
// be inferred. Several candidates ask for a clearer explanation.
func (h *Handler) finishOrder(ctx context.Context, key, answer string) string {
	action, found, ambiguous := h.teachAction(ctx, answer)
	switch {
	case ambiguous:
		return ReplyRetryExplanation
	case found:
		entry, err := h.learning.LearnAction(key, action.ID, action.Name, h.now())
		return h.taught(entry, err, "action", action.ID)
	}
	if _, err := h.learning.LearnPending(key, h.now()); err != nil {
		if h.logger != nil {
			h.logger.Warn("learning", "error", err)
		}
		return ReplyStoreFailed
	}
	h.teaching = teachState{}
	h.lastFailed, h.lastFailedAt = "", time.Time{}
	if h.logger != nil {
		h.logger.Info("learning", "result", "pending", "text", key)
	}
	return ReplyPending
}

// finishQuestion stores a template when the answer uses a current value, and a
// fixed reply otherwise. A value is never frozen: the template is filled on
// every later use.
func (h *Handler) finishQuestion(ctx context.Context, key, answer string) string {
	if len([]rune(answer)) > 1000 {
		return ReplyStoreFailed
	}
	question := data.Detect(h.teaching.sentence)
	if question.IsData() && h.dataProvider != nil {
		if snapshot, err := h.dataProvider.DataSnapshot(ctx); err == nil {
			h.nameQuestion(h.teaching.sentence, &question, snapshot)
			if template, ok := data.TemplateFromAnswer(question, answer, snapshot); ok {
				entry, err := h.learning.LearnTemplate(key, template, h.now())
				return h.taught(entry, err, "template", template)
			}
		}
	}
	entry, err := h.learning.LearnReply(key, answer, h.now())
	return h.taught(entry, err, "reply", answer)
}

// teachAction infers the action of an order. found is false when nothing could
// be inferred; ambiguous is true when several actions fit.
func (h *Handler) teachAction(ctx context.Context, answer string) (action actions.Action, found, ambiguous bool) {
	if h.actionExec == nil || len(h.actionList) == 0 {
		return actions.Action{}, false, false
	}
	if commands, ok := h.localCatalogAction(ctx, answer, nil); ok {
		if len(commands) == 1 {
			return commands[0].Action, true, false
		}
		return actions.Action{}, false, true
	}
	if h.interpreter == nil || h.interpretMode == InterpretOff {
		return actions.Action{}, false, false
	}
	interpreted, ok := h.askActions(ctx, answer, h.homeContext(ctx))
	if !ok || len(interpreted.Commands) == 0 {
		return actions.Action{}, false, false
	}
	if len(interpreted.Commands) > 1 {
		return actions.Action{}, false, true
	}
	command := interpreted.Commands[0]
	if command.Kind != KindAction {
		return actions.Action{}, false, false
	}
	return command.Action, true, false
}

// taught closes the flow after a stored entry.
func (h *Handler) taught(entry learning.Entry, err error, kind, target string) string {
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("learning", "error", err)
		}
		return ReplyStoreFailed
	}
	h.teaching = teachState{}
	h.lastFailed, h.lastFailedAt = "", time.Time{}
	if h.logger != nil {
		h.logger.Info("learning", "result", "taught", "kind", kind, "target", target, "score", entry.Score)
	}
	return ReplyConfirmed
}

// nameQuestion sets the entity that an entity-state or sensor question names,
// so a template can reference the value that the answer used.
func (h *Handler) nameQuestion(text string, question *data.Question, snapshot data.Snapshot) {
	if question.Kind != data.KindEntityState && question.Kind != data.KindSensor {
		return
	}
	if device, ok := h.namedDevice(text); ok {
		question.Named = device.ID
	}
	if question.Named == "" {
		if entity, ok := snapshot.FindSensor(text); ok {
			question.Named = entity.ID
		}
	}
}

// isQuestion classifies the sentence of the flow. An action verb wins, so
// "¿puedes apagar la tele?" is an order even though it is written as a question.
func isQuestion(sentence string) bool {
	return looksLikeQuestion(sentence) && !data.HasActionVerb(sentence)
}

// asClause trims the punctuation that would break the prompt sentence.
func asClause(sentence string) string {
	return strings.Trim(strings.TrimSpace(sentence), "¿?¡!.,;:")
}

func (h *Handler) confirmPrompt() string {
	clause := asClause(h.teaching.sentence)
	if h.teaching.question {
		return "¿Es correcta la pregunta " + clause + "?"
	}
	return "¿Es correcta la orden " + clause + "?"
}

func (h *Handler) tellPrompt() string {
	if h.teaching.question {
		return ReplyTellQuestion
	}
	return ReplyTellOrder
}

func (h *Handler) targetPrompt() string {
	if h.teaching.question {
		return ReplyAskForAnswer
	}
	return ReplyAskForOrder
}
