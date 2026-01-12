package alert

import (
	"log/slog"
	"time"
)

type RateLimitBackOff struct {
	logger *slog.Logger

	release *time.Time
}

func NewRateLimitBackOff(logger *slog.Logger) RateLimitBackOff {
	return RateLimitBackOff{
		logger: logger,
	}
}

func (rlb RateLimitBackOff) ReleaseAt(release time.Time) {
	rlb.logger.Warn("received backoff", "release", release)
	rlb.release = &release
}

// NextBackOff implements backoff.BackOff
func (rlb RateLimitBackOff) NextBackOff() time.Duration {
	if rlb.release != nil {
		duration := time.Until(*rlb.release)
		rlb.release = nil
		return duration
	}

	return 0
}

// NextBackOff implements backoff.BackOff
func (rlb RateLimitBackOff) Reset() {
	rlb.release = nil
}
