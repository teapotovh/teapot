package desec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nrdcg/desec"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	ednsprovider "sigs.k8s.io/external-dns/provider"
)

var ErrAliasUnsupported = errors.New("ALIAS records are not supported")

type provider struct {
	logger *slog.Logger

	desec        *Desec
	timeout      time.Duration
	managedTypes []string
}

// GetDomainFilter implements ednsprovider.Provider.
func (p *provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &endpoint.DomainFilter{
		Filters: []string{p.desec.domain},
	}
}

// Records implements ednsprovider.Provider.
func (p *provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	rrsets, err := p.getAll(ctx, p.desec.domain, nil)
	if err != nil {
		return nil, fmt.Errorf("error while fetching all RRSets: %w", err)
	}

	rrsets = filterRRSets(rrsets, p.managedTypes)
	p.logger.DebugContext(ctx, "fetched all rrsets", "rrsets", rrsets)

	endpoints, err := groupRRSets(ctx, rrsets, p.logger)
	if err != nil {
		return nil, fmt.Errorf("error while grouping RRSets into endpoints: %w", err)
	}

	return endpoints, nil
}

// AdjustEndpoints implements ednsprovider.Provider.
func (p *provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	var result []*endpoint.Endpoint

	for _, e := range endpoints {
		if strings.ToLower(e.RecordType) == "alias" {
			return nil, ErrAliasUnsupported
		}

		if !strings.HasSuffix(e.DNSName, p.desec.domain) {
			p.logger.Debug(
				"ignoring provided endpoint, expected it to be under domain",
				"domain",
				p.desec.domain,
				"endpoint",
				e,
			)

			continue
		}

		if time.Duration(e.RecordTTL)*time.Second < time.Hour {
			e.RecordTTL = endpoint.TTL(time.Hour / time.Second)
		}

		if strings.ToLower(e.RecordType) == "txt" {
			for i, target := range e.Targets {
				if !strings.HasPrefix(target, "\"") || !strings.HasSuffix(target, "\"") {
					e.Targets[i] = "\"" + e.Targets[i] + "\""
				}
			}
		}

		result = append(result, e)
	}

	return result, nil
}

// ApplyChanges implements ednsprovider.Provider.
func (p *provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	var (
		create []desec.RRSet
		update []desec.RRSet
		remove []desec.RRSet

		err error
	)

	// First, we have a "planning phase" where we compute all the RRSets to
	// perform the API calls. This way, if we have any errors due to validity checks,
	// we fail before we perform partial updates using the API.

	if len(changes.Create) > 0 {
		create, err = endpointsToRRSets(changes.Create, p.desec.domain)
		if err != nil {
			return fmt.Errorf("error while converting new endpoints to RRSets: %w", err)
		}
	}

	if len(changes.UpdateNew) > 0 {
		update, err = endpointsToRRSets(changes.UpdateNew, p.desec.domain)
		if err != nil {
			return fmt.Errorf("error while converting updates to endpoints to RRSets: %w", err)
		}
	}

	if len(changes.Delete) > 0 {
		remove = endpointsToRRSetsIdentifiers(changes.Delete, p.desec.domain)
	}

	if len(create) > 0 {
		for _, rrset := range create {
			p.logger.DebugContext(ctx, "creating rrset", "rrset", rrset)
		}

		if _, err := p.bulkCreate(ctx, p.desec.domain, create); err != nil {
			return fmt.Errorf("error while creating RRSets for the new endpoints: %w", err)
		}

		p.logger.InfoContext(ctx, "created new RRSets", "amount", len(create))
	}

	if len(update) > 0 {
		for _, rrset := range update {
			p.logger.DebugContext(ctx, "updating rrset", "rrset", rrset)
		}

		if _, err := p.bulkUpdate(ctx, desec.FullResource, p.desec.domain, update); err != nil {
			return fmt.Errorf("error while updating RRSets for already-existing endpoints: %w", err)
		}

		p.logger.InfoContext(ctx, "updated existing RRSets", "amount", len(update))
	}

	if len(remove) > 0 {
		for _, rrset := range remove {
			p.logger.DebugContext(ctx, "removing rrset", "rrset", rrset)
		}

		if err := p.bulkDelete(ctx, p.desec.domain, remove); err != nil {
			return fmt.Errorf("error while deleting old RRSets: %w", err)
		}

		p.logger.InfoContext(ctx, "deleted old RRSets", "amount", len(remove))
	}

	return nil
}

func (p *provider) getAll(ctx context.Context, domainName string, filter *desec.RRSetFilter) ([]desec.RRSet, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	return p.desec.client.Records.GetAll(ctx, domainName, filter)
}

func (p *provider) bulkCreate(ctx context.Context, domainName string, rrSets []desec.RRSet) ([]desec.RRSet, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	return p.desec.client.Records.BulkCreate(ctx, domainName, rrSets)
}

func (p *provider) bulkUpdate(
	ctx context.Context,
	mode desec.UpdateMode,
	domainName string,
	rrSets []desec.RRSet,
) ([]desec.RRSet, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	return p.desec.client.Records.BulkUpdate(ctx, mode, domainName, rrSets)
}

func (p *provider) bulkDelete(ctx context.Context, domainName string, rrSets []desec.RRSet) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	return p.desec.client.Records.BulkDelete(ctx, domainName, rrSets)
}

// Ensure *provider implements ednsprovider.Provider.
var _ ednsprovider.Provider = &provider{}
