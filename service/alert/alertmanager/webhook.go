package alertmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/alertmanager/template"

	"github.com/teapotovh/teapot/lib/httphandler"
)

const StatusFiring = "firing"

func (am *AlertManager) Webhook(w http.ResponseWriter, r *http.Request) error {
	var data template.Data
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		return fmt.Errorf("could not parse webhook data: %w", errors.Join(err, httphandler.ErrBadRequest))
	}

	am.logger.InfoContext(r.Context(), "received webhook call", "receiver", data.Receiver, "status", data.Status, "groupLabels", data.GroupLabels, "commonLabels", data.CommonLabels, "commonAnnotations", data.CommonAnnotations)

	for _, alert := range data.Alerts {
		am.logger.InfoContext(r.Context(), "received alert", "alert", alert)

		if alert.Status == StatusFiring {
		}
	}

	return nil
}
