package log

import "time"

type manualTicker struct {
	ticker *time.Ticker
	manual chan unit
	out    chan unit
	stop   chan unit
}

func newManualTicker(interval time.Duration) *manualTicker {
	m := &manualTicker{
		ticker: time.NewTicker(interval),
		manual: make(chan unit, 1),
		out:    make(chan unit, 1),
		stop:   make(chan unit),
	}
	go m.run()
	return m
}

func (m *manualTicker) run() {
	for {
		select {
		case <-m.ticker.C:
			select {
			case m.out <- unit{}:
			default: // don't block if consumer is slow
			}
		case <-m.manual:
			select {
			case m.out <- unit{}:
			default:
			}
		case <-m.stop:
			return
		}
	}
}

func (m *manualTicker) Triggered() <-chan unit {
	return m.out
}

func (m *manualTicker) Trigger() {
	select {
	case m.manual <- unit{}:
	default:
	}
}

func (m *manualTicker) Stop() {
	m.ticker.Stop()
	close(m.stop)
}
