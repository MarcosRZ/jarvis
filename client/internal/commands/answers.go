package commands

import (
	"context"
	"errors"
	"strings"
	"time"

	"jarvis/client/internal/data"
)

// dataFollowUp is how long a data answer stays available to a follow-up that
// only names a day.
const dataFollowUp = 60 * time.Second

// answerData runs the deterministic data layer. It only answers a sentence
// without an action verb: an order that happens to mention a datum is still an
// order. The result tells Handle whether the datum was answered, missing, or
// whether the news service is down.
func (h *Handler) answerData(ctx context.Context, text string) (string, data.Result) {
	if h.dataProvider == nil || data.HasActionVerb(text) {
		return "", data.NoMatch
	}
	question := data.Detect(text)
	if !question.IsData() && h.lastData != data.KindNone && h.now().Sub(h.lastDataAt) <= dataFollowUp {
		if followUp, ok := data.FollowUp(text, h.lastData); ok {
			question = followUp
		}
	}
	if !question.IsData() {
		return "", data.NoMatch
	}
	snapshot, err := h.dataProvider.DataSnapshot(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("data snapshot", "error", err)
		}
		return ReplyDataMissing, data.Missing
	}
	if question.Kind == data.KindEntityState || question.Kind == data.KindSensor {
		if device, ok := h.namedDevice(text); ok {
			question.Named = device.ID
		}
		if question.Named == "" {
			if entity, ok := snapshot.FindSensor(text); ok {
				question.Named = entity.ID
			}
		}
		if question.Named == "" {
			return "", data.NoMatch
		}
	}
	var headlines []string
	var newsErr error
	if question.Kind == data.KindNews {
		if h.news == nil {
			newsErr = errors.New("news service is not configured")
		} else {
			headlines, newsErr = h.news.Headlines(ctx)
		}
	}
	answer, result := h.dataResolver.Resolve(snapshot, question, headlines, newsErr)
	switch result {
	case data.Answered:
		switch question.Kind {
		case data.KindWeather, data.KindTemperature, data.KindRain, data.KindUmbrella:
			h.lastData, h.lastDataAt = question.Kind, h.now()
		}
		return answer, result
	case data.Missing:
		return ReplyDataMissing, result
	case data.NewsDown:
		return ReplyNewsDown, result
	}
	return "", result
}

// physicalAnswer handles a physical state that no configured device can fix.
// It gives the useful datum and says why it cannot act, instead of letting the
// model invent a device. When a fitting action exists, the fallback decides.
func (h *Handler) physicalAnswer(ctx context.Context, text string) (string, bool) {
	intent := physicalIntent(text)
	if intent == intentNone || h.canFix(intent) {
		return "", false
	}
	// An explicit order is not a physical state: "pon el aire a 22 grados"
	// belongs to the action layers even when no configured device fits.
	if data.HasActionVerb(text) && !firstPersonState(text) {
		return "", false
	}
	switch intent {
	case intentLight:
		return "No tengo ninguna luz configurada.", true
	case intentTemperature:
		datum := ""
		if h.dataProvider != nil {
			if snapshot, err := h.dataProvider.DataSnapshot(ctx); err == nil {
				datum, _ = snapshot.Temperature(h.now())
			}
		}
		return joinSentences(datum, "No tengo ningún aparato de calefacción configurado."), true
	case intentAir:
		datum := ""
		if h.dataProvider != nil {
			if snapshot, err := h.dataProvider.DataSnapshot(ctx); err == nil {
				datum, _ = snapshot.Humidity()
			}
		}
		return joinSentences(datum, "No tengo ningún aparato de ventilación configurado."), true
	}
	return "", false
}

// canFix reports whether the current catalog has an action for the intent.
func (h *Handler) canFix(intent intentKind) bool {
	for _, action := range h.actionList {
		if actionFitsIntent(action, intent) {
			return true
		}
	}
	if intent == intentLight {
		for _, device := range h.order {
			if !device.Unavailable && deviceFitsIntent(device, intent) {
				return true
			}
		}
	}
	return false
}

// rememberTemplate stores the shape of a data answer that needed the model. The
// value is not frozen: the template is filled from the current snapshot on
// every later use.
func (h *Handler) rememberTemplate(ctx context.Context, text, reply string) {
	if h.learning == nil || h.dataProvider == nil {
		return
	}
	question := data.Detect(text)
	if !question.IsData() || question.Kind == data.KindNews {
		return
	}
	snapshot, err := h.dataProvider.DataSnapshot(ctx)
	if err != nil {
		return
	}
	if question.Kind == data.KindEntityState || question.Kind == data.KindSensor {
		if device, ok := h.namedDevice(text); ok {
			question.Named = device.ID
		}
		if question.Named == "" {
			if entity, ok := snapshot.FindSensor(text); ok {
				question.Named = entity.ID
			}
		}
	}
	template, ok := data.TemplateFromAnswer(question, reply, snapshot)
	if !ok {
		return
	}
	if _, err := h.learning.LearnTemplate(learningKey(text), template, h.now()); err != nil {
		if h.logger != nil {
			h.logger.Warn("data template", "error", err)
		}
		return
	}
	if h.logger != nil {
		h.logger.Info("learning", "result", "template", "template", template)
	}
}

// joinSentences joins the non-empty parts with a single space.
func joinSentences(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " ")
}
