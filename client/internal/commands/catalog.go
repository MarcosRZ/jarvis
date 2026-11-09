package commands

import (
	"slices"
	"strings"

	"jarvis/client/internal/actions"
)

// matchCatalogAction returns the only catalog action that both names the
// entity and matches the verb of the sentence. It is the fast path for common
// phrases; the language model handles everything else.
func matchCatalogAction(text string, list []actions.Action) (actions.Action, bool) {
	norm := learningKey(text)
	if norm == "" || strings.ContainsAny(norm, "0123456789") {
		return actions.Action{}, false
	}
	if containsAny(norm,
		"modo", "modos", "preset", "opcion", "opciones",
		"grados", "temperatura", "calor", "frio", "caliente",
		"ventilacion", "eco", "turbo") {
		return actions.Action{}, false
	}
	intents := actionIntents(norm)
	if len(intents) == 0 {
		return actions.Action{}, false
	}
	words := significantWords(text)
	if len(words) == 0 {
		return actions.Action{}, false
	}
	var match actions.Action
	best, found := 0, 0
	for _, action := range list {
		if !slices.Contains(intents, actionSuffix(action)) {
			continue
		}
		score := actionMentionScore(words, action)
		switch {
		case score == 0:
			continue
		case score > best:
			match, best, found = action, score, 1
		case score == best:
			found++
		}
	}
	return match, found == 1
}

// matchCatalogActions matches every clause of a sentence joined by "y" or "e".
// All clauses must match exactly one action; otherwise the sentence is left to
// Home Assistant or the model. It keeps the requested order and drops a
// repeated action.
func matchCatalogActions(text string, list []actions.Action) ([]actions.Action, bool) {
	parts := splitClauses(learningKey(text))
	if len(parts) == 0 {
		return nil, false
	}
	matched := make([]actions.Action, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		action, ok := matchCatalogAction(part, list)
		if !ok {
			return nil, false
		}
		if seen[action.ID] {
			continue
		}
		seen[action.ID] = true
		matched = append(matched, action)
	}
	return matched, len(matched) > 0
}

// actionIntents maps the verbs of a normalized sentence to the action suffixes
// they can mean. The most specific phrases must be checked first.
func actionIntents(norm string) []string {
	if containsAny(norm,
		"quita el silencio", "quitar el silencio", "quita silencio", "quitar silencio",
		"activa el sonido", "activar el sonido", "quita el mute", "quitar el mute",
		"desmutea", "desmutear") {
		return []string{"unmute"}
	}
	var intents []string
	add := func(kinds ...string) {
		for _, kind := range kinds {
			if !slices.Contains(intents, kind) {
				intents = append(intents, kind)
			}
		}
	}
	if containsAny(norm, "silencia", "silenciar", "silencie", "silencies", "silencio", "mutea", "mutear") {
		add("mute")
	}
	if containsAny(norm,
		"sube el volumen", "subir el volumen", "subele el volumen",
		"aumenta el volumen", "aumentar el volumen", "mas volumen", "sube volumen") {
		add("volume_up")
	}
	if containsAny(norm,
		"baja el volumen", "bajar el volumen", "bajale el volumen",
		"reduce el volumen", "reducir el volumen", "menos volumen", "baja volumen") {
		add("volume_down")
	}
	if containsAny(norm,
		"reproduce", "reproducir", "reanuda", "reanudar",
		"continua la reproduccion", "continuar la reproduccion", "dale al play") {
		add("play")
	}
	if containsAny(norm, "pausa", "pausar", "pausala", "pausalo", "pauses") {
		add("pause")
	}
	if containsAny(norm,
		"deten la", "detener la", "deten el", "detener el", "detenla", "detenlo",
		"parar la reproduccion", "para la reproduccion") {
		add("stop")
	}
	if containsAny(norm, "alterna", "alternar", "cambia el estado", "cambiar el estado") {
		add("toggle")
	}
	if containsAny(norm, "sube", "subir", "levanta", "levantar") {
		add("open")
	}
	if containsAny(norm, "baja", "bajar") {
		add("close")
	}
	if containsAny(norm, "pulsa", "pulsar") {
		add("press")
	}
	if containsAny(norm,
		"enciende", "encender", "encienda", "enciendas", "prende", "prender", "prendas",
		"pon", "poner", "pongas", "activa", "activar", "actives",
		"calienta", "calentar", "calientes") {
		add("turn_on")
	}
	if containsAny(norm, "ejecuta", "ejecutar", "dispara", "disparar") {
		add("trigger", "turn_on")
	}
	if containsAny(norm,
		"apaga", "apagar", "apague", "apagues", "desconecta", "desconectar", "desconectes",
		"desactiva", "desactivar", "desactives") {
		add("turn_off")
	}
	if containsAny(norm, "quita", "quitar") {
		add("turn_off")
	}
	if containsAny(norm, "abre", "abrir", "abras") {
		add("open", "unlock")
	}
	if containsAny(norm, "cierra", "cerrar", "cierres") {
		add("close", "lock")
	}
	if containsAny(norm, "bloquea", "bloquear", "bloquees", "echa el cierre") {
		add("lock")
	}
	if containsAny(norm, "desbloquea", "desbloquear", "desbloquees") {
		add("unlock")
	}
	if containsAny(norm, "inicia", "iniciar", "arranca", "arrancar") {
		add("start", "turn_on")
	}
	if containsAny(norm, "cancela", "cancelar") {
		add("cancel")
	}
	return intents
}

// actionMentionScore counts how many words of the sentence name the action's
// entity. A specific word such as the room name beats a shared alias such as
// "escena" when several actions of the same domain match.
func actionMentionScore(words []string, action actions.Action) int {
	tokens := actionMentionWords(action)
	score := 0
	for _, word := range words {
		for _, token := range tokens {
			if similarWord(word, token) {
				score++
				break
			}
		}
	}
	return score
}

// actionMentionWords returns the words that identify an action's entity: the
// entity ID, the friendly name without the generated leading verb, and the
// aliases. Dropping the leading verb matters: "enciende la tele" must not
// match a switch whose name starts with "Encender".
func actionMentionWords(action actions.Action) []string {
	tokens := significantWords(strings.ReplaceAll(action.EntityID, ".", " "))
	for _, alias := range action.Aliases {
		tokens = append(tokens, significantWords(alias)...)
	}
	name := significantWords(action.Name)
	if len(name) > 0 && actionNameVerbs[name[0]] {
		name = name[1:]
	}
	return append(tokens, name...)
}

// actionSuffix returns the last component of the stable action ID, for example
// "turn_on" for "light.kitchen.turn_on".
func actionSuffix(action actions.Action) string {
	return action.ID[strings.LastIndexByte(action.ID, '.')+1:]
}

// guardedDomains are the actions that change a physical state with
// consequences beyond a light or a plug. The language model must see a known
// verb in the sentence before one of them can run; naming the entity alone is
// not enough.
var guardedDomains = map[string]bool{
	"automation": true, "button": true, "climate": true, "cover": true,
	"lock": true, "remote": true, "scene": true, "script": true,
	"siren": true, "vacuum": true, "valve": true, "water_heater": true,
}

var actionNameVerbs = map[string]bool{
	"abrir": true, "activar": true, "alternar": true, "apagar": true,
	"bajar": true, "bloquear": true, "cancelar": true, "cerrar": true,
	"desactivar": true, "desbloquear": true, "detener": true, "ejecutar": true,
	"encender": true, "enviar": true, "iniciar": true, "localizar": true,
	"pausar": true, "poner": true, "pulsar": true, "quitar": true,
	"reproducir": true, "seleccionar": true, "silenciar": true,
	"subir": true,
}

// similarWord matches a word with its plural or a clear prefix. It is used only
// for catalog names and aliases, so "calienta" can match "calentar" and "tele"
// can match "television" without loosening the device parser.
func similarWord(a, b string) bool {
	if sameWord(a, b) {
		return true
	}
	if len(a) < 4 || len(b) < 4 {
		return false
	}
	if prefixSimilar(a, b) {
		return true
	}
	// Spanish verb forms change the stem ("calienta" / "calentar"). Only do
	// this for long words: short room names such as "sala" must not match a
	// verb such as "salir".
	if len(a) < 6 || len(b) < 6 {
		return false
	}
	return verbStem(a) == verbStem(b)
}

func prefixSimilar(a, b string) bool {
	common := 0
	for common < len(a) && common < len(b) && a[common] == b[common] {
		common++
	}
	shortest := len(a)
	if len(b) < shortest {
		shortest = len(b)
	}
	return common >= 4 && common*10 >= shortest*7
}

var verbEndings = []string{
	"ando", "iendo", "amos", "emos", "imos",
	"ar", "er", "ir", "as", "es", "an", "en", "a", "e", "o",
}

func verbStem(word string) string {
	word = strings.NewReplacer("ie", "e", "ue", "o").Replace(word)
	for _, ending := range verbEndings {
		if strings.HasSuffix(word, ending) && len(word) >= len(ending)+3 {
			return strings.TrimSuffix(word, ending)
		}
	}
	return word
}
