package core

import (
	"encoding/json"
	"os"
	"time"
)

// InFlight names the action that changes a cache. The window writes it before
// the action starts and removes it when the action ends, so a start that finds
// the file tells the person what was running when the app stopped.
type InFlight struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// InFlightPath is the file that holds the running action.
func InFlightPath() string { return StatePath("inflight.json") }

// WriteInFlight records the action that starts now. A write that fails leaves
// the action without a record; it never stops the action.
func WriteInFlight(f InFlight) error {
	if f.StartedAt.IsZero() {
		f.StartedAt = time.Now()
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(InFlightPath(), data, 0o644)
}

// ClearInFlight removes the record. A missing file is a finished action.
func ClearInFlight() error {
	err := os.Remove(InFlightPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadInFlight reads the action that a previous run left behind, and reports
// whether the file held one. An unreadable file reports false: a record the
// window cannot read tells it nothing.
func ReadInFlight() (InFlight, bool) {
	data, err := os.ReadFile(InFlightPath())
	if err != nil {
		return InFlight{}, false
	}
	var f InFlight
	if err := json.Unmarshal(data, &f); err != nil || f.ID == "" {
		return InFlight{}, false
	}
	return f, true
}
