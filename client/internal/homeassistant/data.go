package homeassistant

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	hdata "jarvis/client/internal/data"
)

const (
	// exposureTTL is how long the exposure map is cached. It changes rarely and
	// the WebSocket round trip would otherwise delay every data question.
	exposureTTL = time.Minute
	// forecastTTL caches the weather forecast: it changes slowly and a small
	// question must not wait for two service calls every time.
	forecastTTL = 5 * time.Minute
)

type cachedExposure struct {
	at     time.Time
	values map[string]map[string]bool
}

type cachedForecast struct {
	at     time.Time
	daily  []hdata.Forecast
	hourly []hdata.Forecast
}

var exposureCache = struct {
	sync.Mutex
	byClient map[*Client]cachedExposure
}{byClient: map[*Client]cachedExposure{}}

var forecastCache = struct {
	sync.Mutex
	byEntity map[string]cachedForecast
}{byEntity: map[string]cachedForecast{}}

// DataSnapshot returns the exposed entities and their forecasts for the
// deterministic data layer. It is read-only and never includes the token.
func (c *Client) DataSnapshot(ctx context.Context) (hdata.Snapshot, error) {
	exposed, err := c.exposedEntities(ctx)
	if err != nil {
		return hdata.Snapshot{}, err
	}
	var states []contextState
	if err := c.rest(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return hdata.Snapshot{}, err
	}
	snapshot := hdata.Snapshot{
		Entities: make(map[string]hdata.Entity, len(states)),
		At:       time.Now(),
	}
	for _, state := range states {
		if !exposed[state.EntityID][assistant] {
			continue
		}
		domain, _, _ := strings.Cut(state.EntityID, ".")
		entity := hdata.Entity{
			ID:         state.EntityID,
			Domain:     domain,
			Name:       friendlyName(state),
			State:      state.State,
			Unit:       textAttribute(state.Attributes, "unit_of_measurement"),
			Attributes: state.Attributes,
		}
		if domain == "weather" {
			entity.Daily, entity.Hourly = c.forecasts(ctx, state.EntityID)
			snapshot.Weather = append(snapshot.Weather, state.EntityID)
		}
		snapshot.Entities[state.EntityID] = entity
	}
	sortWeather(snapshot.Weather)
	return snapshot, nil
}

// sortWeather puts the Spanish AEMET entity first, then the configured
// forecast, then the rest in a stable order.
func sortWeather(ids []string) {
	rank := func(id string) int {
		switch id {
		case "weather.aemet":
			return 0
		case "weather.forecast_casa":
			return 1
		}
		return 2
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if rank(ids[i]) != rank(ids[j]) {
			return rank(ids[i]) < rank(ids[j])
		}
		return ids[i] < ids[j]
	})
}

func friendlyName(state contextState) string {
	if name := textAttribute(state.Attributes, "friendly_name"); name != "" {
		return name
	}
	_, object, _ := strings.Cut(state.EntityID, ".")
	return strings.ReplaceAll(object, "_", " ")
}

func textAttribute(attributes map[string]any, key string) string {
	value, ok := attributes[key]
	if !ok || value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

// forecasts returns the cached daily and hourly forecast of an entity, or asks
// Home Assistant for both.
func (c *Client) forecasts(ctx context.Context, entityID string) ([]hdata.Forecast, []hdata.Forecast) {
	forecastCache.Lock()
	cached, ok := forecastCache.byEntity[entityID]
	forecastCache.Unlock()
	if ok && time.Since(cached.at) < forecastTTL {
		return cached.daily, cached.hourly
	}
	daily := c.forecast(ctx, entityID, "daily")
	hourly := c.forecast(ctx, entityID, "hourly")
	forecastCache.Lock()
	forecastCache.byEntity[entityID] = cachedForecast{at: time.Now(), daily: daily, hourly: hourly}
	forecastCache.Unlock()
	return daily, hourly
}

func (c *Client) forecast(ctx context.Context, entityID, kind string) []hdata.Forecast {
	body := map[string]any{"entity_id": entityID, "type": kind}
	var result forecastResult
	if err := c.rest(ctx, http.MethodPost, "/api/services/weather/get_forecasts?return_response", body, &result); err != nil {
		return nil
	}
	raw := result.ServiceResponse[entityID].Forecast
	out := make([]hdata.Forecast, 0, len(raw))
	for _, item := range raw {
		out = append(out, parseForecast(item))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

func parseForecast(item map[string]any) hdata.Forecast {
	f := hdata.Forecast{
		Condition:   textAttribute(item, "condition"),
		Temperature: floatAttribute(item, "temperature"),
		Templow:     floatAttribute(item, "templow"),
		Rain:        floatAttribute(item, "precipitation"),
		Probability: floatAttribute(item, "precipitation_probability"),
		WindSpeed:   floatAttribute(item, "wind_speed"),
	}
	if text := textAttribute(item, "datetime"); text != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				f.Time = parsed
				break
			}
		}
	}
	return f
}

func floatAttribute(attributes map[string]any, key string) *float64 {
	value, ok := attributes[key]
	if !ok || value == nil {
		return nil
	}
	switch v := value.(type) {
	case float64:
		return &v
	case int:
		f := float64(v)
		return &f
	}
	return nil
}
