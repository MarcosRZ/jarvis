package main

type serviceDefinition struct {
	ID          string
	Name        string
	Description string
	Icon        string
	PrimaryPort int
	ExternalHA  bool
}

var serviceRegistry = []serviceDefinition{
	{
		ID:          "whisper",
		Name:        "Whisper",
		Description: "Reconocimiento de voz (STT)",
		Icon:        "voice",
		PrimaryPort: 10300,
	},
	{
		ID:          "llama-server",
		Name:        "llama-server",
		Description: "Modelo de lenguaje local",
		Icon:        "brain",
		PrimaryPort: 8080,
	},
	{
		ID:          "piper",
		Name:        "Piper",
		Description: "Sintesis de voz (TTS)",
		Icon:        "model",
		PrimaryPort: 10200,
	},
	{
		ID:          "openwakeword",
		Name:        "openWakeWord",
		Description: "Deteccion de palabra de activacion",
		Icon:        "mic",
		PrimaryPort: 10400,
	},
	{
		ID:          "dashboard",
		Name:        "M4nhattan",
		Description: "Este panel de control",
		Icon:        "dashboard",
		PrimaryPort: 3000,
	},
	{
		ID:          "homeassistant",
		Name:        "Home Assistant",
		Description: "Entorno de pruebas local",
		Icon:        "home",
		PrimaryPort: 8123,
		ExternalHA:  true,
	},
}

func registryByID(id string) (serviceDefinition, bool) {
	for _, definition := range serviceRegistry {
		if definition.ID == id {
			return definition, true
		}
	}
	return serviceDefinition{}, false
}
