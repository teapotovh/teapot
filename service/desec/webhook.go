package desec

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	ednsprovider "sigs.k8s.io/external-dns/provider"
)

const (
	MediaTypeFormatAndVersion = "application/external.dns.webhook+json;version=1"
	ContentTypeHeader         = "Content-Type"
	UrlNegotiate              = "/"
	UrlAdjustEndpoints        = "/adjustendpoints"
	UrlRecords                = "/records"
)

type webhook struct {
	logger *slog.Logger

	provider ednsprovider.Provider
}

func (wh *webhook) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		records, err := wh.provider.Records(context.Background())
		if err != nil {
			wh.logger.ErrorContext(r.Context(), "error while fetching records", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(records); err != nil {
			wh.logger.ErrorContext(r.Context(), "failed to encode records", "err", err)
		}

		break
	case http.MethodPost:
		var changes plan.Changes
		if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
			wh.logger.ErrorContext(r.Context(), "failed to decode changes", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err := wh.provider.ApplyChanges(context.Background(), &changes)
		if err != nil {
			wh.logger.ErrorContext(r.Context(), "failed to apply changes", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

		break
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (wh *webhook) AdjustEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var pve []*endpoint.Endpoint
	if err := json.NewDecoder(r.Body).Decode(&pve); err != nil {
		wh.logger.ErrorContext(r.Context(), "failed to decode endpoint adjustments", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
	pve, err := wh.provider.AdjustEndpoints(pve)
	if err != nil {
		wh.logger.ErrorContext(r.Context(), "failed to call adjust endpoints", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	if err := json.NewEncoder(w).Encode(&pve); err != nil {
		wh.logger.ErrorContext(r.Context(), "failed to encode adjusted endpoints", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (wh *webhook) NegotiateHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != UrlNegotiate {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
	err := json.NewEncoder(w).Encode(wh.provider.GetDomainFilter())
	if err != nil {
		wh.logger.ErrorContext(r.Context(), "error while getting domain filter", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
