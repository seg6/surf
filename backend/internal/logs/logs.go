// Package logs emits Surf's structured record schema through the process-wide
// logger. The configured logstore writer persists the JSON line unchanged.
package logs

import (
	"encoding/json"
	"log"
	"time"

	"surf-backend/internal/logstore"
)

func Event(level, component, message string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	record := logstore.Record{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: level,
		Component: component, Message: message, Fields: fields,
	}
	data, err := json.Marshal(record)
	if err != nil {
		log.Printf("logging: encode structured event: %v", err)
		return
	}
	log.Print(string(data))
}

func Info(component, message string, fields map[string]any) {
	Event("info", component, message, fields)
}

func Warn(component, message string, fields map[string]any) {
	Event("warn", component, message, fields)
}

func Error(component, message string, fields map[string]any) {
	Event("error", component, message, fields)
}
