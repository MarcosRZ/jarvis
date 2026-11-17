// Package actions builds the catalog of safe actions that the assistant can
// run from the entities exposed in Home Assistant. An action is data, not a
// shell command: it names one entity and one Home Assistant service with fixed
// arguments, so the language model can select it but cannot invent it.
package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Version is the catalog format version.
const Version = 1

// Entity is the part of an exposed Home Assistant entity that the catalog
// needs. Attributes carry supported modes, options and friendly names.
type Entity struct {
	EntityID   string
	Domain     string
	Name       string
	State      string
	Attributes map[string]any
}

// Action is one selectable action. ID is stable across generations and is the
// only value that the language model may return. Service is the Home Assistant
// service, for example "light.turn_on"; Data holds fixed service arguments.
// Aliases are extra Spanish words that help match a spoken sentence, for
// example "tele" for a media player.
type Action struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	EntityID    string         `json:"entity_id"`
	Domain      string         `json:"domain"`
	Service     string         `json:"service"`
	Data        map[string]any `json:"data,omitempty"`
	Aliases     []string       `json:"aliases,omitempty"`
}

// Catalog is the generated action list.
type Catalog struct {
	Version   int       `json:"version"`
	Generated time.Time `json:"generated"`
	Actions   []Action  `json:"actions"`
}

var (
	ErrBadAction = errors.New("invalid action")
	ErrBadEntity = errors.New("invalid entity")
)

var entityIDPattern = regexp.MustCompile(`^([a-z0-9_]+)\.([a-z0-9_]+)$`)

// services is the allowlist of Home Assistant services that the generator may
// produce and the runner may call. A catalog entry with any other service is
// rejected, even if it was edited by hand.
var services = map[string]map[string]bool{
	"light":         {"turn_on": true, "turn_off": true, "toggle": true},
	"switch":        {"turn_on": true, "turn_off": true, "toggle": true},
	"input_boolean": {"turn_on": true, "turn_off": true, "toggle": true},
	"fan":           {"turn_on": true, "turn_off": true, "toggle": true, "set_preset_mode": true},
	"cover":         {"open_cover": true, "close_cover": true, "stop_cover": true},
	"valve":         {"open_valve": true, "close_valve": true},
	"lock":          {"lock": true, "unlock": true},
	"scene":         {"turn_on": true},
	"script":        {"turn_on": true, "turn_off": true},
	"automation":    {"trigger": true, "turn_on": true, "turn_off": true},
	"button":        {"press": true},
	"climate":       {"turn_on": true, "turn_off": true, "set_hvac_mode": true},
	"water_heater":  {"turn_on": true, "turn_off": true, "set_operation_mode": true},
	"media_player": {
		"turn_on": true, "turn_off": true, "media_play": true, "media_pause": true,
		"media_stop": true, "volume_up": true, "volume_down": true, "volume_mute": true,
	},
	"vacuum":       {"start": true, "pause": true, "stop": true, "return_to_base": true, "locate": true},
	"humidifier":   {"turn_on": true, "turn_off": true},
	"siren":        {"turn_on": true, "turn_off": true},
	"remote":       {"turn_on": true, "turn_off": true},
	"timer":        {"start": true, "pause": true, "cancel": true},
	"select":       {"select_option": true},
	"input_select": {"select_option": true},
}

// aliases are extra matching words per domain. They are Spanish nouns that a
// person may say instead of the entity name.
var aliases = map[string][]string{
	"light":         {"luz", "luces", "lampara", "iluminacion"},
	"switch":        {"interruptor", "enchufe"},
	"input_boolean": {"interruptor"},
	"fan":           {"ventilador", "ventilacion"},
	"cover":         {"persiana", "cortina", "estor", "toldo", "garaje", "puerta"},
	"valve":         {"valvula", "grifo"},
	"lock":          {"cerradura", "puerta", "candado"},
	"scene":         {"escena"},
	"script":        {"rutina", "script"},
	"automation":    {"automatizacion"},
	"button":        {"boton", "pulsador"},
	"climate":       {"clima", "aire", "calefaccion", "termometro", "termostato"},
	"water_heater":  {"termo", "calentador", "agua caliente"},
	"media_player":  {"tele", "television", "tv", "altavoz", "musica", "reproductor"},
	"vacuum":        {"aspirador", "robot", "roomba"},
	"humidifier":    {"humidificador"},
	"siren":         {"sirena", "alarma"},
	"remote":        {"mando", "remoto"},
	"timer":         {"temporizador", "temporizador"},
	"select":        {"selector"},
	"input_select":  {"selector"},
}

// Generate builds the catalog for the exposed entities that can be controlled.
// Sensors, weather, cameras and other read-only entities are left out: their
// state is already in the model context. Unavailable entities are skipped, so
// the model does not pick an action that is known to fail.
func Generate(entities []Entity, now time.Time) Catalog {
	catalog := Catalog{Version: Version, Generated: now}
	for _, entity := range entities {
		if strings.TrimSpace(entity.EntityID) == "" || entity.State == "unavailable" {
			continue
		}
		catalog.Actions = append(catalog.Actions, generate(entity)...)
	}
	sort.Slice(catalog.Actions, func(i, j int) bool {
		return catalog.Actions[i].Name < catalog.Actions[j].Name
	})
	return catalog
}

// generate returns the useful actions of one entity. An unknown domain or a
// missing mode list returns no actions.
func generate(entity Entity) []Action {
	m := entityIDPattern.FindStringSubmatch(entity.EntityID)
	if m == nil || m[1] != entity.Domain {
		return nil
	}
	objectID := m[2]
	name := entityName(entity)
	spoken := spokenEntity(name)

	add := func(suffix, actionName, service string, data map[string]any) Action {
		return Action{
			ID:          entity.Domain + "." + objectID + "." + suffix,
			Name:        actionName,
			Description: describe(actionName, entity.Domain, entity.EntityID),
			EntityID:    entity.EntityID,
			Domain:      entity.Domain,
			Service:     entity.Domain + "." + service,
			Data:        data,
			Aliases:     slices.Clone(aliases[entity.Domain]),
		}
	}

	switch entity.Domain {
	case "light":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
			add("toggle", "Alternar "+spoken, "toggle", nil),
		}
	case "switch", "input_boolean":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
			add("toggle", "Alternar "+spoken, "toggle", nil),
		}
	case "fan":
		list := []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
			add("toggle", "Alternar "+spoken, "toggle", nil),
		}
		for _, preset := range stringList(entity.Attributes["preset_modes"]) {
			list = append(list, add("preset_"+slug(preset), "Poner "+spoken+" en modo "+preset, "set_preset_mode", map[string]any{"preset_mode": preset}))
		}
		return list
	case "cover":
		return []Action{
			add("open", "Abrir "+spoken, "open_cover", nil),
			add("close", "Cerrar "+spoken, "close_cover", nil),
			add("stop", "Detener "+spoken, "stop_cover", nil),
		}
	case "valve":
		return []Action{
			add("open", "Abrir "+spoken, "open_valve", nil),
			add("close", "Cerrar "+spoken, "close_valve", nil),
		}
	case "lock":
		return []Action{
			add("lock", "Bloquear "+spoken, "lock", nil),
			add("unlock", "Desbloquear "+spoken, "unlock", nil),
		}
	case "scene":
		return []Action{add("turn_on", "Activar "+spoken, "turn_on", nil)}
	case "script":
		return []Action{
			add("turn_on", "Ejecutar "+spoken, "turn_on", nil),
			add("turn_off", "Detener "+spoken, "turn_off", nil),
		}
	case "automation":
		return []Action{
			add("trigger", "Ejecutar "+spoken, "trigger", nil),
			add("turn_on", "Activar "+spoken, "turn_on", nil),
			add("turn_off", "Desactivar "+spoken, "turn_off", nil),
		}
	case "button":
		return []Action{add("press", "Pulsar "+spoken, "press", nil)}
	case "climate":
		list := []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
		}
		for _, mode := range stringList(entity.Attributes["hvac_modes"]) {
			list = append(list, add("mode_"+slug(mode), "Poner "+spoken+" en modo "+modeName(mode), "set_hvac_mode", map[string]any{"hvac_mode": mode}))
		}
		return list
	case "water_heater":
		list := []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
		}
		for _, mode := range stringList(entity.Attributes["operation_list"]) {
			list = append(list, add("mode_"+slug(mode), "Poner "+spoken+" en modo "+modeName(mode), "set_operation_mode", map[string]any{"operation_mode": mode}))
		}
		return list
	case "media_player":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
			add("play", "Reproducir en "+spoken, "media_play", nil),
			add("pause", "Pausar "+spoken, "media_pause", nil),
			add("stop", "Detener "+spoken, "media_stop", nil),
			add("volume_up", "Subir el volumen de "+spoken, "volume_up", nil),
			add("volume_down", "Bajar el volumen de "+spoken, "volume_down", nil),
			add("mute", "Silenciar "+spoken, "volume_mute", map[string]any{"is_volume_muted": true}),
			add("unmute", "Quitar el silencio a "+spoken, "volume_mute", map[string]any{"is_volume_muted": false}),
		}
	case "vacuum":
		return []Action{
			add("start", "Iniciar "+spoken, "start", nil),
			add("pause", "Pausar "+spoken, "pause", nil),
			add("stop", "Detener "+spoken, "stop", nil),
			add("return_to_base", "Enviar "+spoken+" a la base", "return_to_base", nil),
			add("locate", "Localizar "+spoken, "locate", nil),
		}
	case "humidifier":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
		}
	case "siren":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
		}
	case "remote":
		return []Action{
			add("turn_on", "Encender "+spoken, "turn_on", nil),
			add("turn_off", "Apagar "+spoken, "turn_off", nil),
		}
	case "timer":
		return []Action{
			add("start", "Iniciar "+spoken, "start", nil),
			add("pause", "Pausar "+spoken, "pause", nil),
			add("cancel", "Cancelar "+spoken, "cancel", nil),
		}
	case "select", "input_select":
		var list []Action
		for _, option := range stringList(entity.Attributes["options"]) {
			list = append(list, add("option_"+slug(option), "Seleccionar "+option+" en "+spoken, "select_option", map[string]any{"option": option}))
		}
		return list
	}
	return nil
}

// describe is the one-line explanation that the model sees next to the ID.
func describe(name, domain, id string) string {
	text := name + " (" + id
	if words := aliases[domain]; len(words) > 0 {
		text += "; tambien: " + strings.Join(words, ", ")
	}
	return text + ")"
}

// entityName returns a friendly name, or the object ID with spaces.
func entityName(entity Entity) string {
	if name, ok := entity.Attributes["friendly_name"].(string); ok {
		if name = strings.TrimSpace(name); name != "" {
			return name
		}
	}
	if name, ok := entity.Attributes["name"].(string); ok {
		if name = strings.TrimSpace(name); name != "" {
			return name
		}
	}
	m := entityIDPattern.FindStringSubmatch(entity.EntityID)
	if m == nil {
		return entity.EntityID
	}
	return strings.ReplaceAll(m[2], "_", " ")
}

// spokenEntity lowercases a leading article, so that a name reads well inside
// an action such as "Encender la luz del salón".
func spokenEntity(name string) string {
	first, rest, found := strings.Cut(name, " ")
	if !found {
		return name
	}
	switch strings.ToLower(first) {
	case "el", "la", "los", "las":
		return strings.ToLower(first) + " " + rest
	}
	return name
}

// stringList turns a Home Assistant attribute list into strings. Attributes
// decoded from JSON are []any, but tests may build []string.
func stringList(value any) []string {
	var list []string
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				list = append(list, text)
			}
		}
	case []string:
		for _, item := range v {
			if text := strings.TrimSpace(item); text != "" {
				list = append(list, text)
			}
		}
	}
	return list
}

// modeName translates the Home Assistant English mode codes that appear in the
// generated action name.
func modeName(mode string) string {
	names := map[string]string{
		"off": "apagado", "heat": "calor", "cool": "frio", "auto": "automatico",
		"dry": "secado", "fan_only": "solo ventilador", "heat_cool": "calor y frio",
		"eco": "eco", "boost": "maximo", "comfort": "confort", "sleep": "sueno",
		"home": "casa", "away": "fuera", "activity": "actividad",
	}
	if name, ok := names[mode]; ok {
		return name
	}
	return mode
}

var plainLetters = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// slug makes a stable ID component from a mode or an option.
func slug(text string) string {
	text = plainLetters.Replace(strings.ToLower(text))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore:
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// Validate checks an action that may come from a generated or edited catalog.
// It rejects any service that is not in the allowlist and any argument that
// the generator does not produce.
func Validate(action Action) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w %q: %s", ErrBadAction, action.ID, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(action.ID) == "" {
		return bad("empty ID")
	}
	m := entityIDPattern.FindStringSubmatch(action.EntityID)
	if m == nil {
		return bad("entity_id %q is not domain.object_id", action.EntityID)
	}
	if m[1] != action.Domain {
		return bad("domain %q does not match entity_id %q", action.Domain, action.EntityID)
	}
	serviceDomain, service, ok := strings.Cut(action.Service, ".")
	if !ok || serviceDomain != action.Domain {
		return bad("service %q is not in domain %q", action.Service, action.Domain)
	}
	if !services[action.Domain][service] {
		return bad("service %q is not allowed for domain %q", action.Service, action.Domain)
	}
	if strings.TrimSpace(action.Name) == "" {
		return bad("empty name")
	}
	if len(action.Data) > 0 && !allowedData(action.Domain, service, action.Data) {
		return bad("unexpected data for %q", action.Service)
	}
	return nil
}

// allowedData keeps the fixed arguments within the small set that the
// generator produces. This is the second check that keeps an edited catalog
// from sending arbitrary service data.
func allowedData(domain, service string, data map[string]any) bool {
	switch domain + "." + service {
	case "climate.set_hvac_mode":
		_, ok := data["hvac_mode"]
		return ok && len(data) == 1
	case "water_heater.set_operation_mode":
		_, ok := data["operation_mode"]
		return ok && len(data) == 1
	case "fan.set_preset_mode":
		_, ok := data["preset_mode"]
		return ok && len(data) == 1
	case "select.select_option", "input_select.select_option":
		_, ok := data["option"]
		return ok && len(data) == 1
	case "media_player.volume_mute":
		_, ok := data["is_volume_muted"]
		return ok && len(data) == 1
	}
	return false
}

// Save writes the catalog as JSON, creating the parent directory. The file is
// replaceable atomically, so a reader never sees a half-written catalog.
func Save(path string, catalog Catalog) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty actions file")
	}
	b, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("encode actions: %w", err)
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create actions directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".actions-*.json")
	if err != nil {
		return fmt.Errorf("create temporary actions file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write actions file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close actions file: %w", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return fmt.Errorf("set actions file mode: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace actions file: %w", err)
	}
	return nil
}
