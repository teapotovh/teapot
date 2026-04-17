package desec

import (
	"context"
	"errors"
	"log/slog"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type provider struct {
	logger *slog.Logger

	desec *Desec
}

type domainFilter struct {
	domain string
}

// Match implements endpoint.DomainFilterInterface
func (d *domainFilter) Match(domain string) bool {
	return domain == d.domain
}

// GetDomainFilter implements ednsprovider.Provider
func (p *provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &domainFilter{p.desec.domain}
}

var ErrNotImplemented = errors.New("not implemented")

// Records implements ednsprovider.Provider
func (p *provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	return nil, ErrNotImplemented
}

// ApplyChanges implements ednsprovider.Provider
func (p *provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	return ErrNotImplemented
}

// AdjustEndpoints implements ednsprovider.Provider
func (p *provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return nil, ErrNotImplemented
}
