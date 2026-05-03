package log

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/teapotovh/teapot/lib/httphandler"
)

const URLLogs = "/logs"

type event struct {
	Timestamp time.Time       `json:"timestamp"`
	Source    string          `json:"source"`
	Data      json.RawMessage `json:"data"`
}

const LogLevelUnkown = "unknown"

type maybeLogLevel struct {
	Level string `json:"level"`
}

func tryExtractLevel(data json.RawMessage) string {
	var ll maybeLogLevel
	if err := json.Unmarshal(data, &ll); err == nil {
		return ll.Level
	}

	return LogLevelUnkown
}

func (l *Log) handleLogs(w http.ResponseWriter, r *http.Request) error {
	var event event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		return fmt.Errorf("%w: error while decoding the request body: %w", httphandler.ErrBadRequest, err)
	}

	if err := r.Body.Close(); err != nil {
		return fmt.Errorf("%w: error while closing the request body: %w", httphandler.ErrBadRequest, err)
	}

	level := tryExtractLevel(event.Data)
	if err := l.manager.process(event, level); err != nil {
		return fmt.Errorf("%w: error while storing log: %w", httphandler.ErrInternal, err)
	}

	return nil
}
