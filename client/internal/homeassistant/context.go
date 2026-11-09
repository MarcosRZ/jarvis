package homeassistant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	// maxContextEntities bounds the prompt context for a small model.
	maxContextEntities = 50
	// maxForecastEntries bounds each weather forecast in the prompt context.
	maxForecastEntries = 4
	// maxContextValueRunes keeps one attribute from dominating the context.
	maxContextValueRunes = 80
)

// contextKeys are the attributes that help a model answer a question about
// the current state of an exposed entity. Other attributes are omitted to keep
// the prompt small.
var contextKeys = []string{
	"friendly_name", "unit_of_measurement", "temperature", "current_temperature",
	"templow", "humidity", "hvac_mode", "hvac_action", "fan_mode", "preset_mode",
	"wind_speed", "pressure", "visibility", "battery_level", "current_position",
}

// contextState is the subset of /api/states used to build the model context.
type contextState struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

// forecastResult is the response of weather.get_forecasts.
type forecastResult struct {
	ServiceResponse map[string]struct {
		Forecast []map[string]any `json:"forecast"`
	} `json:"service_response"`
}

// Context returns a read-only snapshot of the exposed Home Assistant entities
// and the forecast of every exposed weather entity. It does not include the
// access token, service definitions or unexposed entities. The handler gives it
// to the local model, so the model can answer questions about the home and use
// the same data to choose an action.
func (c *Client) Context(ctx context.Context) (string, error) {
	exposed, err := c.exposedEntities(ctx)
	if err != nil {
		return "", err
	}
	var states []contextState
	if err := c.rest(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return "", err
	}
	// Put weather entities first: their forecast answers future questions, and
	// a small model pays more attention to the start of the context.
	sort.SliceStable(states, func(i, j int) bool {
		iWeather := strings.HasPrefix(states[i].EntityID, "weather.")
		jWeather := strings.HasPrefix(states[j].EntityID, "weather.")
		if iWeather != jWeather {
			return iWeather
		}
		return states[i].EntityID < states[j].EntityID
	})

	var b strings.Builder
	b.WriteString("Home Assistant context (read-only):\n")
	entities := 0
	for _, state := range states {
		if !exposed[state.EntityID][assistant] {
			continue
		}
		if entities == maxContextEntities {
			b.WriteString("- ...\n")
			break
		}
		writeState(&b, state)
		if strings.HasPrefix(state.EntityID, "weather.") {
			if forecast, err := c.forecastContext(ctx, state.EntityID); err == nil {
				b.WriteString(forecast)
			}
		}
		entities++
	}
	if entities == 0 {
		return "", nil
	}
	return b.String(), nil
}

// writeState writes one exposed entity and its useful attributes. The unit is
// appended to the value instead of repeating "unit of measurement" for every
// sensor, because the context shares the model window with the command prompt.
func writeState(b *strings.Builder, state contextState) {
	fmt.Fprintf(b, "- %s", state.EntityID)
	if name := strings.TrimSpace(fmt.Sprint(state.Attributes["friendly_name"])); name != "" && name != "<nil>" {
		fmt.Fprintf(b, " (%s)", truncate(name))
	}
	value := state.State
	if strings.HasPrefix(state.EntityID, "weather.") {
		value = weatherCondition(value)
	}
	fmt.Fprintf(b, ": %s", truncate(value))
	unit := strings.TrimSpace(fmt.Sprint(state.Attributes["unit_of_measurement"]))
	if unit == "<nil>" {
		unit = ""
	}
	for _, key := range contextKeys {
		if key == "friendly_name" || key == "unit_of_measurement" {
			continue
		}
		value, ok := state.Attributes[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		if numbers, ok := value.(float64); ok {
			text = fmt.Sprintf("%g", numbers)
		}
		if unit != "" && key != "wind_speed" {
			text += " " + unit
		}
		fmt.Fprintf(b, "; %s %s", strings.ReplaceAll(key, "_", " "), truncate(text))
	}
	b.WriteString("\n")
}

// forecastContext returns a compact forecast block for one weather entity. It
// tries hourly first, which is the useful window for "will it rain", and falls
// back to daily when the integration does not provide hourly data.
func (c *Client) forecastContext(ctx context.Context, entityID string) (string, error) {
	for _, kind := range []string{"hourly", "daily"} {
		body := map[string]any{"entity_id": entityID, "type": kind}
		var result forecastResult
		if err := c.rest(ctx, http.MethodPost, "/api/services/weather/get_forecasts?return_response", body, &result); err != nil {
			continue
		}
		forecast := result.ServiceResponse[entityID].Forecast
		if len(forecast) == 0 {
			continue
		}
		if len(forecast) > maxForecastEntries {
			forecast = forecast[:maxForecastEntries]
		}
		var b strings.Builder
		fmt.Fprintf(&b, "  forecast %s:\n", kind)
		for _, item := range forecast {
			line := forecastLine(item)
			if line != "" {
				fmt.Fprintf(&b, "  - %s\n", line)
			}
		}
		if b.Len() > 0 {
			return b.String(), nil
		}
	}
	return "", errors.New("no weather forecast")
}

// forecastLine keeps the fields that matter for a spoken answer. It uses a
// compact text form instead of JSON so that a forecast fits in a small model.
func forecastLine(item map[string]any) string {
	parts := make([]string, 0, 5)
	for _, key := range []string{"datetime", "condition", "temperature", "precipitation", "precipitation_probability"} {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		if numbers, ok := value.(float64); ok {
			text = fmt.Sprintf("%g", numbers)
		}
		if key == "condition" {
			text = weatherCondition(text)
		}
		switch key {
		case "temperature":
			text += " °C"
		case "precipitation":
			text += " mm"
		case "precipitation_probability":
			text += " %"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", ")
}

// weatherConditions translates the Home Assistant weather state names that
// the model and the speaker would otherwise mix with Spanish. Home Assistant
// uses English condition codes for every weather integration.
var weatherConditions = map[string]string{
	"clear-night":     "noche despejada",
	"cloudy":          "nublado",
	"exceptional":     "excepcional",
	"fog":             "niebla",
	"hail":            "granizo",
	"lightning":       "tormenta",
	"lightning-rainy": "tormenta con lluvia",
	"partlycloudy":    "parcialmente nublado",
	"pouring":         "lluvia fuerte",
	"rainy":           "lluvioso",
	"snowy":           "nieve",
	"snowy-rainy":     "aguanieve",
	"sunny":           "soleado",
	"windy":           "ventoso",
	"windy-variant":   "ventoso",
}

func weatherCondition(condition string) string {
	if translated, ok := weatherConditions[strings.TrimSpace(condition)]; ok {
		return translated
	}
	return condition
}

func truncate(text string) string {
	runes := []rune(text)
	if len(runes) <= maxContextValueRunes {
		return text
	}
	return string(runes[:maxContextValueRunes]) + "…"
}
