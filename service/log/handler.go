package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
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
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("%w: error while reading the: %w", httphandler.ErrInternal, err)
	}

	var events []event
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&events); err != nil {
		l.logger.DebugContext(r.Context(), "failed to parse request", "err", err, "body", string(b))
		return fmt.Errorf("%w: error while decoding the request body: %w", httphandler.ErrBadRequest, err)
	}

	if err := r.Body.Close(); err != nil {
		return fmt.Errorf("%w: error while closing the request body: %w", httphandler.ErrBadRequest, err)
	}

	var wg sync.WaitGroup
	logErrors := make([]error, len(events))
	for i, event := range events {
		wg.Go(func() {
			level := tryExtractLevel(event.Data)
			if err := l.manager.process(event, level); err != nil {
				logErrors[i] = fmt.Errorf("%w: error while storing log: %w", httphandler.ErrInternal, err)
			}
		})
	}
	wg.Wait()

	var collected []error
	for i, err := range logErrors {
		if err != nil {
			collected = append(collected, fmt.Errorf("%d: %w", i, err))
		}
	}

	if len(collected) > 0 {
		return fmt.Errorf("writing one or more logs failed: %w", errors.Join(collected...))
	}

	return nil
}
