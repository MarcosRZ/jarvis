package wyoming

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
)

func encode(t *testing.T, events ...Event) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, ev := range events {
		if err := WriteEvent(&buf, ev); err != nil {
			t.Fatalf("WriteEvent: %v", err)
		}
	}
	return buf.Bytes()
}

func mustEvent(t *testing.T, typ string, data any, payload []byte) Event {
	t.Helper()
	ev, err := NewEvent(typ, data, payload)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return ev
}

func TestWriteEventFraming(t *testing.T) {
	payload := []byte{0x00, '\n', 0xff, 0xfe}
	raw := encode(t, mustEvent(t, TypeAudioChunk, AudioFormat{Rate: 16000, Width: 2, Channels: 1}, payload))

	line, rest, ok := bytes.Cut(raw, []byte{'\n'})
	if !ok {
		t.Fatal("no header line")
	}
	var header map[string]any
	if err := json.Unmarshal(line, &header); err != nil {
		t.Fatalf("header is not JSON: %v", err)
	}
	if header["type"] != TypeAudioChunk || header["version"] != ProtocolVersion {
		t.Errorf("header %v", header)
	}
	dataLength := int(header["data_length"].(float64))
	if header["payload_length"].(float64) != float64(len(payload)) {
		t.Errorf("payload_length %v, want %d", header["payload_length"], len(payload))
	}
	if got := string(rest[:dataLength]); got != `{"rate":16000,"width":2,"channels":1}` {
		t.Errorf("data %s", got)
	}
	if !bytes.Equal(rest[dataLength:], payload) {
		t.Errorf("payload %v, want %v", rest[dataLength:], payload)
	}
}

func TestWriteEventOmitsEmptyDataAndPayload(t *testing.T) {
	raw := encode(t, mustEvent(t, TypeDescribe, nil, nil), mustEvent(t, TypeAudioStop, struct{}{}, nil))
	want := `{"type":"describe","version":"` + ProtocolVersion + `"}` + "\n" +
		`{"type":"audio-stop","version":"` + ProtocolVersion + `"}` + "\n"
	if string(raw) != want {
		t.Fatalf("got %q, want %q", raw, want)
	}
}

func TestReadEventRoundTripsBinaryPayload(t *testing.T) {
	payload := []byte{0x00, '\n', 0xff, 0x80, '{'}
	in := mustEvent(t, TypeAudioChunk, AudioFormat{Rate: 22050, Width: 2, Channels: 1}, payload)

	out, err := ReadEvent(bufio.NewReader(bytes.NewReader(encode(t, in))))
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if out.Type != TypeAudioChunk || !bytes.Equal(out.Payload, payload) {
		t.Fatalf("got %+v", out)
	}
	var format AudioFormat
	if err := out.Decode(&format); err != nil || format.Rate != 22050 {
		t.Fatalf("Decode = %+v, %v", format, err)
	}
}

func TestReadEventHandlesPartialReadsAndManyEvents(t *testing.T) {
	raw := encode(t,
		mustEvent(t, TypeTranscript, Transcript{Text: "¿qué hora es?"}, nil),
		mustEvent(t, TypeAudioChunk, AudioFormat{Rate: 16000, Width: 2, Channels: 1}, []byte{1, 2, 3, 4}),
		mustEvent(t, TypeAudioStop, nil, nil),
	)
	r := bufio.NewReader(iotest.OneByteReader(bytes.NewReader(raw)))

	var types []string
	for {
		ev, err := ReadEvent(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("ReadEvent: %v", err)
		}
		types = append(types, ev.Type)
	}
	if strings.Join(types, ",") != "transcript,audio-chunk,audio-stop" {
		t.Fatalf("types %v", types)
	}
}

func TestReadEventMergesInlineData(t *testing.T) {
	raw := `{"type":"detection","data":{"name":"inline","timestamp":5},"data_length":17}` + "\n" + `{"name":"merged"}`
	ev, err := ReadEvent(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	var d Detection
	if err := ev.Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.Name != "merged" || d.Timestamp != 5 {
		t.Fatalf("got %+v, want name from data block and timestamp from header", d)
	}
}

func TestReadEventAcceptsOlderInlineOnlyData(t *testing.T) {
	raw := `{"type":"transcript","data":{"text":"hola"}}` + "\n"
	ev, err := ReadEvent(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	var tr Transcript
	if err := ev.Decode(&tr); err != nil || tr.Text != "hola" {
		t.Fatalf("Decode = %+v, %v", tr, err)
	}
}

func TestReadEventRejectsMalformedInput(t *testing.T) {
	tests := map[string]string{
		"header not JSON":        "not json\n",
		"header without type":    `{"data_length":0}` + "\n",
		"negative data length":   `{"type":"x","data_length":-1}` + "\n",
		"negative payload":       `{"type":"x","payload_length":-1}` + "\n",
		"data not an object":     `{"type":"x","data_length":3}` + "\n" + `[1]`,
		"inline data not object": `{"type":"x","data":"text"}` + "\n",
		"header too long":        `{"type":"` + strings.Repeat("a", maxHeaderBytes) + `"}` + "\n",
		"data too large":         `{"type":"x","data_length":` + strconv.Itoa(maxDataBytes+1) + `}` + "\n",
		"payload too large":      `{"type":"x","payload_length":` + strconv.Itoa(maxPayloadBytes+1) + `}` + "\n",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ReadEvent(bufio.NewReader(strings.NewReader(raw)))
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("error %v, want ErrMalformed", err)
			}
		})
	}
}

func TestReadEventReportsTruncatedEvent(t *testing.T) {
	tests := map[string]string{
		"header without newline": `{"type":"x"`,
		"short data":             `{"type":"x","data_length":10}` + "\n" + `{"a"`,
		"short payload":          `{"type":"x","payload_length":4}` + "\n" + "ab",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ReadEvent(bufio.NewReader(strings.NewReader(raw)))
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("error %v, want io.ErrUnexpectedEOF", err)
			}
		})
	}
}

func TestReadEventCleanEOF(t *testing.T) {
	if _, err := ReadEvent(bufio.NewReader(strings.NewReader(""))); !errors.Is(err, io.EOF) {
		t.Fatalf("error %v, want io.EOF", err)
	}
}

func TestCheckType(t *testing.T) {
	ok := mustEvent(t, TypeTranscript, Transcript{Text: "hola"}, nil)
	if err := CheckType(ok, TypeTranscript); err != nil {
		t.Errorf("CheckType: %v", err)
	}

	serviceErr := mustEvent(t, TypeError, Error{Text: "model not loaded", Code: "no-model"}, nil)
	err := CheckType(serviceErr, TypeTranscript)
	var se *ServiceError
	if !errors.As(err, &se) || se.Text != "model not loaded" || se.Code != "no-model" {
		t.Errorf("error event gave %v", err)
	}

	err = CheckType(mustEvent(t, TypeAudioStart, nil, nil), TypeTranscript)
	if !errors.Is(err, ErrUnexpectedEvent) || !strings.Contains(err.Error(), `"audio-start"`) || !strings.Contains(err.Error(), "transcript") {
		t.Errorf("unexpected event gave %v", err)
	}
}
