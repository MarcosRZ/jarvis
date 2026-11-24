package data

import "strings"

// Detect classifies a sentence as a data question. It is deliberately narrow:
// a sentence without a known data pattern returns KindNone and goes to the
// caller's fallback, so chat is not answered with "No tengo ese dato.".
func Detect(text string) Question {
	norm := normalize(text)
	q := Question{Weekday: -1}
	switch {
	case containsAny(norm, "noticias", "titulares", "ultima hora"):
		q.Kind = KindNews
	case containsAny(norm, "paraguas"):
		q.Kind = KindUmbrella
	case containsAny(norm, "llover", "lluvia", "llovera", "chubascos", "precipitacion"):
		q.Kind = KindRain
	case containsAny(norm, "humedad"):
		q.Kind = KindHumidity
	case containsAny(norm, "temperatura", "grados", "hace frio", "hace calor", "que frio hace", "que calor hace"):
		q.Kind = KindTemperature
	case containsAny(norm, "tiempo") && containsAny(norm, "hace", "hara", "esta", "como", "va"):
		q.Kind = KindWeather
	case looksLikeStateQuestion(norm):
		q.Kind = KindEntityState
	case looksLikeSensorQuestion(norm):
		q.Kind = KindSensor
	default:
		return q
	}
	q.Day, q.Weekday = parseDay(norm)
	return q
}

func looksLikeStateQuestion(norm string) bool {
	if !containsAny(norm, "esta", "estan", "estado") {
		return false
	}
	return containsPrefix(norm,
		"encendid", "apagad", "abiert", "cerrad", "conectad", "desconectad",
		"funcionando", "puest", "activo", "inactivo")
}

// containsPrefix reports whether any word of norm starts with one of the
// prefixes. It covers the inflected forms of a state adjective.
func containsPrefix(norm string, prefixes ...string) bool {
	for _, word := range strings.Fields(norm) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(word, prefix) {
				return true
			}
		}
	}
	return false
}

func looksLikeSensorQuestion(norm string) bool {
	return containsAny(norm, "cuanto", "cuanta", "cuantos", "cuantas", "cual", "que") &&
		containsAny(norm, "hay", "tiene", "marca", "valor", "nivel")
}

// FollowUp reuses kind for a short follow-up that only names a day, for
// example "¿y mañana?" after a weather answer. It returns false for a sentence
// with its own topic or without a day, so it never stretches the previous
// answer too far.
func FollowUp(text string, kind Kind) (Question, bool) {
	switch kind {
	case KindWeather, KindTemperature, KindRain, KindUmbrella:
	default:
		return Question{}, false
	}
	norm := normalize(text)
	day, weekday := parseDay(norm)
	if day == 0 && weekday < 0 {
		return Question{}, false
	}
	if !containsAny(norm, "y") && len(strings.Fields(norm)) > 3 {
		return Question{}, false
	}
	return Question{Kind: kind, Day: day, Weekday: weekday}, true
}

// parseDay extracts the day of a rain or weather question. Day 0 is today, 1 is
// tomorrow and 2 is the day after tomorrow. Weekday is 0 for Sunday and -1 when
// the sentence names no weekday.
func parseDay(norm string) (int, int) {
	weekday := -1
	if day, ok := weekdays[normWord(norm)]; ok {
		weekday = day
	}
	switch {
	case containsAny(norm, "pasado manana"):
		return 2, weekday
	case containsAny(norm, "manana"):
		return 1, weekday
	case containsAny(norm, "hoy", "ahora"):
		return 0, weekday
	}
	return 0, weekday
}

// weekdays maps the Spanish weekday name to time.Weekday (Sunday is 0).
var weekdays = map[string]int{
	"domingo":   0,
	"lunes":     1,
	"martes":    2,
	"miercoles": 3,
	"jueves":    4,
	"viernes":   5,
	"sabado":    6,
}

func normWord(text string) string {
	words := strings.Fields(text)
	for _, word := range words {
		if _, ok := weekdays[word]; ok {
			return word
		}
	}
	return ""
}

// actionVerbs are the verbs that turn a sentence into an order, even when it is
// written as a question: "¿puedes apagar la tele?" acts. Infinitives are
// included because "puedes apagar" is common in speech.
var actionVerbs = []string{
	"enciende", "encender", "encienda", "enciendeme",
	"apaga", "apagar", "apague", "apagame",
	"pon", "poner", "ponme", "ponlo", "ponla",
	"quita", "quitar", "quitael", "quita la",
	"sube", "subir", "subeme", "subele",
	"baja", "bajar", "bajame", "bajale",
	"silencia", "silenciar", "mutea", "mutear",
	"activa", "activar", "desactiva", "desactivar",
	"abre", "abrir", "cierra", "cerrar",
	"haz", "hacer", "ejecuta", "ejecutar",
	"cambia", "cambiar", "ajusta", "ajustar",
	"calienta", "calentar", "enfria", "enfriar",
	"reproduce", "reproducir", "pausa", "pausar",
	"deten", "detener", "para", "parar",
	"mueve", "mover", "programa", "programar",
	"configura", "configurar", "ilumina", "iluminar",
	"apunta", "apuntar", "marca", "marcar",
}

// HasActionVerb reports whether the sentence contains a verb that asks for an
// action. The action verb wins over the question shape.
func HasActionVerb(text string) bool {
	norm := normalize(text)
	for _, word := range strings.Fields(norm) {
		for _, verb := range actionVerbs {
			if word == verb {
				return true
			}
		}
	}
	return false
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
