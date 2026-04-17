package desec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	ednsprovider "sigs.k8s.io/external-dns/provider"
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
	rrsets, err := p.desec.client.Records.GetAll(ctx, p.desec.domain, nil)
	if err != nil {
		return nil, fmt.Errorf("error while fetching all RRSets: %w", err)
	}

	slog.DebugContext(ctx, "fetched all rrsets", "rrsets", rrsets)

	endpoints, err := groupRRSets(ctx, rrsets, p.logger)
	if err != nil {
		return nil, fmt.Errorf("error while grouping RRSets into endpoints: %w", err)
	}

	return endpoints, nil
}

// AdjustEndpoints implements ednsprovider.Provider
func (p *provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return nil, ErrNotImplemented
}

// ApplyChanges implements ednsprovider.Provider
func (p *provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	return ErrNotImplemented
}

// Ensure *provider implements ednsprovider.Provider
var _ ednsprovider.Provider = &provider{}
