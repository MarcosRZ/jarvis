package wyoming

// The data types hold only the fields that the client uses. ReadEvent keeps all
// fields in Event.Data.

type AudioFormat struct {
	Rate     int `json:"rate"`
	Width    int `json:"width"`
	Channels int `json:"channels"`
}

type Detect struct {
	Names []string `json:"names,omitempty"`
}

// Detection.Timestamp is in milliseconds from the start of the audio stream.
type Detection struct {
	Name      string `json:"name"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type Transcribe struct {
	Language string `json:"language,omitempty"`
}

type Transcript struct {
	Text string `json:"text"`
}

type Synthesize struct {
	Text  string `json:"text"`
	Voice *Voice `json:"voice,omitempty"`
}

// Voice.Speaker selects one speaker of a voice with several, for example "F".
type Voice struct {
	Name    string `json:"name,omitempty"`
	Speaker string `json:"speaker,omitempty"`
}

type Error struct {
	Text string `json:"text"`
	Code string `json:"code,omitempty"`
}

type Info struct {
	Wake []Program `json:"wake,omitempty"`
	ASR  []Program `json:"asr,omitempty"`
	TTS  []Program `json:"tts,omitempty"`
}

// Program is a wake, asr or tts entry of Info. TTS programs list voices, the others list models.
type Program struct {
	Name   string  `json:"name"`
	Models []Model `json:"models,omitempty"`
	Voices []Model `json:"voices,omitempty"`
}

// Model.Speakers is set only for a TTS voice with several speakers.
type Model struct {
	Name      string    `json:"name"`
	Languages []string  `json:"languages,omitempty"`
	Speakers  []Speaker `json:"speakers,omitempty"`
}

type Speaker struct {
	Name string `json:"name"`
}
