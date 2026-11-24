package data

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testSnapshot(now time.Time) Snapshot {
	temperature := 9.0
	tomorrow := 14.0
	low := 6.0
	rain := 80.0
	return Snapshot{
		At:      now,
		Weather: []string{"weather.aemet"},
		Entities: map[string]Entity{
			"weather.aemet": {
				ID:     "weather.aemet",
				Domain: "weather",
				Name:   "AEMET",
				State:  "rainy",
				Attributes: map[string]any{
					"temperature": temperature,
					"humidity":    float64(72),
				},
				Daily: []Forecast{
					{Time: now, Condition: "rainy", Temperature: &temperature, Probability: &rain},
					{Time: now.AddDate(0, 0, 1), Condition: "sunny", Temperature: &tomorrow, Templow: &low},
				},
			},
			"light.lounge": {
				ID:     "light.lounge",
				Domain: "light",
				Name:   "la luz del salón",
				State:  "on",
			},
			"sensor.humidity": {
				ID:     "sensor.humidity",
				Domain: "sensor",
				Name:   "humedad del salón",
				State:  "55",
				Unit:   "%",
			},
		},
	}
}

func TestDetectClassifiesQuestions(t *testing.T) {
	tests := map[string]Question{
		"¿qué temperatura hace?":    {Kind: KindTemperature, Weekday: -1},
		"¿va a llover mañana?":      {Kind: KindRain, Day: 1, Weekday: -1},
		"¿va a llover el sábado?":   {Kind: KindRain, Weekday: 6},
		"¿debo llevar paraguas?":    {Kind: KindUmbrella, Weekday: -1},
		"¿qué tiempo hace?":         {Kind: KindWeather, Weekday: -1},
		"¿cuánta humedad hay?":      {Kind: KindHumidity, Weekday: -1},
		"¿está encendida la luz?":   {Kind: KindEntityState, Weekday: -1},
		"dime las noticias":         {Kind: KindNews, Weekday: -1},
		"cuéntame un chiste":        {Kind: KindNone, Weekday: -1},
		"enciende la luz del salón": {Kind: KindNone, Weekday: -1},
	}
	for text, want := range tests {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %+v, want %+v", text, got, want)
		}
	}
}

func TestHasActionVerb(t *testing.T) {
	if !HasActionVerb("¿puedes apagar la tele?") {
		t.Error("apagar must make the sentence an order")
	}
	if HasActionVerb("¿está encendida la tele?") {
		t.Error("a state question has no action verb")
	}
	if HasActionVerb("¿qué temperatura hace?") {
		t.Error("a datum question has no action verb")
	}
}

func TestResolveAnswersFromTheSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	resolver := Resolver{Now: func() time.Time { return now }}
	snapshot := testSnapshot(now)

	cases := []struct {
		text string
		want string
	}{
		{"¿qué temperatura hace?", "Hay 9 grados y está lloviendo."},
		{"¿qué tiempo hace?", "Está lloviendo y hay 9 grados."},
		{"¿va a llover?", "Sí, está lloviendo."},
		{"¿cuánta humedad hay?", "Hay un 72 por ciento de humedad."},
		{"¿está encendida la luz del salón?", "Sí, está encendida."},
	}
	for _, c := range cases {
		q := Detect(c.text)
		if q.Kind == KindEntityState {
			q.Named = "light.lounge"
		}
		answer, result := resolver.Resolve(snapshot, q, nil, nil)
		if result != Answered || answer != c.want {
			t.Errorf("%q = %q (%v), want %q", c.text, answer, result, c.want)
		}
	}
}

func TestResolveTomorrowUsesTheForecast(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	resolver := Resolver{Now: func() time.Time { return now }}
	snapshot := testSnapshot(now)
	q := Detect("¿va a llover mañana?")
	answer, result := resolver.Resolve(snapshot, q, nil, nil)
	if result != Answered || !strings.Contains(answer, "No, no va a llover") {
		t.Errorf("tomorrow = %q (%v), want a dry forecast", answer, result)
	}
	q = Detect("¿va a llover el miércoles?")
	answer, result = resolver.Resolve(snapshot, q, nil, nil)
	if result != Answered || !strings.Contains(answer, "No, no va a llover") {
		t.Errorf("wednesday = %q (%v), want the dry forecast", answer, result)
	}
}

func TestResolveNews(t *testing.T) {
	resolver := Resolver{}
	headlines := []string{"elDiario.es: Una noticia."}
	answer, result := resolver.Resolve(testSnapshot(time.Now()), Question{Kind: KindNews}, headlines, nil)
	if result != Answered || answer != headlines[0] {
		t.Errorf("news = %q (%v)", answer, result)
	}
	if _, result := resolver.Resolve(Snapshot{}, Question{Kind: KindNews}, nil, errors.New("down")); result != NewsDown {
		t.Errorf("news down result %v", result)
	}
}

func TestRenderTemplate(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	snapshot := testSnapshot(now)
	template := "Hay {weather.aemet.attributes.temperature} grados y está {weather.aemet.condition}."
	got, err := Render(template, snapshot)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := "Hay 9 grados y está lloviendo."; got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
	if _, err := Render("Hay {weather.aemet.forecast.today.templow} grados.", snapshot); err == nil {
		t.Error("a missing forecast value must fail instead of speaking a placeholder")
	}
}

func TestValidateRejectsBrokenTemplates(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	snapshot := testSnapshot(now)
	bad := []string{
		"Hay 9 grados.",
		"Hay {weather.aemet.attributes.temperature}grados.",
		"Hay  {weather.aemet.attributes.temperature} grados.",
		"Hay {weather.aemet.attributes.temperature} grados",
		"Hay {weather.unknown.attributes.temperature} grados.",
		"Hay {weather.aemet.attributes.missing} grados.",
		"Está rainy y hay {weather.aemet.attributes.temperature} grados.",
		"Hay {weather.aemet.attributes.temperature grados.",
	}
	for _, template := range bad {
		if err := Validate(template, snapshot); err == nil {
			t.Errorf("Validate(%q) accepted a broken template", template)
		}
	}
	good := "Hay {weather.aemet.attributes.temperature} grados y está {weather.aemet.condition}."
	if err := Validate(good, snapshot); err != nil {
		t.Errorf("Validate(%q) = %v", good, err)
	}
}

func TestTemplateFromAnswerKeepsTheValueDynamic(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	snapshot := testSnapshot(now)
	template, ok := TemplateFromAnswer(Question{Kind: KindTemperature, Weekday: -1}, "Hay 9 grados.", snapshot)
	if !ok {
		t.Fatal("no template")
	}
	if strings.Contains(template, "9") {
		t.Errorf("template froze the value: %q", template)
	}
	snapshot.Entities["weather.aemet"].Attributes["temperature"] = float64(21)
	rendered, err := Render(template, snapshot)
	if err != nil {
		t.Fatalf("Render(%q): %v", template, err)
	}
	if rendered != "Hay 21 grados." {
		t.Errorf("template %q rendered %q, want the new value", template, rendered)
	}
	if _, ok := TemplateFromAnswer(Question{Kind: KindWeather, Weekday: -1}, "No lo sé.", snapshot); ok {
		t.Error("a fixed reply must not become a template")
	}
}

func TestFollowUpOnlyReusesADayQuestion(t *testing.T) {
	q, ok := FollowUp("¿y mañana?", KindRain)
	if !ok || q.Kind != KindRain || q.Day != 1 {
		t.Errorf("follow-up %+v, %v", q, ok)
	}
	if _, ok := FollowUp("cuéntame un chiste", KindRain); ok {
		t.Error("a new topic must not reuse the last one")
	}
	if _, ok := FollowUp("¿y mañana?", KindEntityState); ok {
		t.Error("an entity state has no day follow-up")
	}
}
