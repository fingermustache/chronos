package models

import (
	"encoding/json"
	"testing"
)

func TestScheduleTypeIsValid(t *testing.T) {
	tests := []struct {
		name  string
		value ScheduleType
		want  bool
	}{
		{name: "cron", value: ScheduleTypeCron, want: true},
		{name: "interval", value: ScheduleTypeInterval, want: true},
		{name: "once", value: ScheduleTypeOnce, want: true},
		{name: "invalid", value: ScheduleType("nope"), want: false},
		{name: "empty", value: ScheduleType(""), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.value.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskTypeIsValid(t *testing.T) {
	tests := []struct {
		name  string
		value TaskType
		want  bool
	}{
		{name: "http", value: TaskTypeHTTP, want: true},
		{name: "command", value: TaskTypeCommand, want: true},
		{name: "grpc", value: TaskTypeGRPC, want: true},
		{name: "invalid", value: TaskType("nope"), want: false},
		{name: "empty", value: TaskType(""), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.value.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSONBValue_NilReturnsEmptyObject(t *testing.T) {
	var j JSONB
	got, err := j.Value()
	if err != nil {
		t.Fatalf("Value() unexpected error: %v", err)
	}
	b, ok := got.([]byte)
	if !ok {
		t.Fatalf("Value() type = %T, want []byte", got)
	}
	if string(b) != "{}" {
		t.Errorf("Value() = %s, want {}", string(b))
	}
}

func TestJSONBValue_EncodesMap(t *testing.T) {
	j := JSONB{
		"url":    "https://example.com",
		"method": "GET",
		"retry":  float64(3),
	}
	got, err := j.Value()
	if err != nil {
		t.Fatalf("Value() unexpected error: %v", err)
	}
	b, ok := got.([]byte)
	if !ok {
		t.Fatalf("Value() type = %T, want []byte", got)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if decoded["url"] != "https://example.com" {
		t.Errorf("decoded[url] = %v, want %v", decoded["url"], "https://example.com")
	}
	if decoded["method"] != "GET" {
		t.Errorf("decoded[method] = %v, want %v", decoded["method"], "GET")
	}
	if decoded["retry"] != float64(3) {
		t.Errorf("decoded[retry] = %v, want %v", decoded["retry"], float64(3))
	}
}

func TestJSONBScan_NilSetsEmptyMap(t *testing.T) {
	var j JSONB
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) unexpected error: %v", err)
	}
	if j == nil {
		t.Fatal("Scan(nil) left map nil")
	}
	if len(j) != 0 {
		t.Errorf("Scan(nil) len = %d, want 0", len(j))
	}
}

func TestJSONBScan_FromBytes(t *testing.T) {
	var j JSONB
	err := j.Scan([]byte(`{"service":"Worker","method":"Run"}`))
	if err != nil {
		t.Fatalf("Scan([]byte) unexpected error: %v", err)
	}
	if j["service"] != "Worker" {
		t.Errorf("j[service] = %v, want %v", j["service"], "Worker")
	}
	if j["method"] != "Run" {
		t.Errorf("j[method] = %v, want %v", j["method"], "Run")
	}
}

func TestJSONBScan_FromString(t *testing.T) {
	var j JSONB
	err := j.Scan(`{"seconds":3600}`)
	if err != nil {
		t.Fatalf("Scan(string) unexpected error: %v", err)
	}
	if j["seconds"] != float64(3600) {
		t.Errorf("j[seconds] = %v, want %v", j["seconds"], float64(3600))
	}
}

func TestJSONBScan_UnsupportedType(t *testing.T) {
	var j JSONB
	err := j.Scan(123)
	if err == nil {
		t.Fatal("Scan() expected error, got nil")
	}
}

func TestJSONBScan_InvalidJSON(t *testing.T) {
	var j JSONB
	err := j.Scan(`{"bad":`)
	if err == nil {
		t.Fatal("Scan() expected error, got nil")
	}
}
