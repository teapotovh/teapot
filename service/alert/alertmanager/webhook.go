package alertmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/alertmanager/template"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/service/alert"
)

const (
	StatusFiring          = "firing"
	LabelAlertName        = "alertname"
	LabelSeverity         = "severity"
	AnnotationDescription = "description"

	UnknownAlert      = "Unknown Alert"
	UnknownSeverity   = "unkown-severity"
	AlertManagerLabel = "alertmanager"
)

func (am *AlertManager) Webhook(w http.ResponseWriter, r *http.Request) (err error) {
	defer func() {
		status := metricsStatusSuccess
		if err != nil {
			status = metricsStatusFailed
		}

		am.metrics.total.WithLabelValues(status).Add(1)
	}()

	var data template.Data
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		return fmt.Errorf("could not parse webhook data: %w", errors.Join(err, httphandler.ErrBadRequest))
	}

	am.logger.DebugContext(r.Context(), "received webhook call", "receiver", data.Receiver, "status", data.Status, "groupLabels", data.GroupLabels, "commonLabels", data.CommonLabels, "commonAnnotations", data.CommonAnnotations)

	for _, a := range data.Alerts {
		am.metrics.alerts.WithLabelValues(a.Status).Add(1)
		am.logger.DebugContext(r.Context(), "received alert", "alert", a)

		if a.Status == StatusFiring {
			name, ok := a.Labels[LabelAlertName]
			if !ok {
				name = UnknownAlert
			}
			severity, ok := a.Labels[LabelSeverity]
			if !ok {
				severity = UnknownSeverity
			}
			name = "alertmanager: " + name
			desc := a.Annotations[AnnotationDescription]

			am.alert.Fire(alert.AlertData{
				ID:          a.Fingerprint,
				Title:       name,
				Description: desc,
				Time:        a.StartsAt,
				Labels:      []string{AlertManagerLabel, severity},

				Details: alertDetails(a),
			})
		} else {
			am.logger.InfoContext(r.Context(), "ignoring alert", "alert", a)
		}
	}

	return nil
}

func alertDetails(alert template.Alert) string {
	var builder strings.Builder

	// labels dump
	builder.Write([]byte("<h3>Labels</h3>\n<ul>\n"))
	for key, value := range alert.Labels {
		builder.Write(fmt.Appendf(nil, "<li><code>%s</code>: <code>%s</code></li>\n", key, value))
	}
	builder.Write([]byte("</ul>\n<hr />\n"))

	// annotations dump
	builder.Write([]byte("<h3>Annotations</h3>\n<ul>\n"))
	for key, value := range alert.Annotations {
		builder.Write(fmt.Appendf(nil, "<li><code>%s</code>: <code>%s</code></li>\n", key, value))
	}
	builder.Write([]byte("</ul>\n<hr />\n"))

	builder.Write([]byte("<h3>Timing</h3>\n<ul>\n"))
	builder.Write(fmt.Appendf(nil, "<li>starts at: %s</li>\n", alert.StartsAt))
	builder.Write(fmt.Appendf(nil, "<li>ends at: %s</li>\n", alert.EndsAt))
	builder.Write([]byte("</ul>\n<hr />\n"))

	builder.Write([]byte("<h3>Identifiers</h3>\n<ul>\n"))
	builder.Write(fmt.Appendf(nil, "<li>url: %s</li>\n", alert.GeneratorURL))
	builder.Write(fmt.Appendf(nil, "<li>fingerprint: %s</li>\n", alert.Fingerprint))
	builder.Write([]byte("</ul>\n"))

	return builder.String()
}
