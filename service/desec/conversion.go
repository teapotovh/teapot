package desec

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/nrdcg/desec"
	"sigs.k8s.io/external-dns/endpoint"
)

var (
	ErrInvalidLabelType    = errors.New("invalid type for label record")
	ErrInvalidLabelRecords = errors.New("invalid number of label records")
	ErrSpecialCharacter    = errors.New("contains special character")
	ErrInvalidKeyValuePair = errors.New("invalid key-value pair")
	ErrMissingQuotes       = errors.New("missing quotes")
)

func canonicalize(dn string) string {
	var parts []string
	for _, part := range strings.Split(dn, ".") {
		if len(part) > 0 {
			parts = append(parts, part)
		}
	}
	fqdn := strings.Join(parts, ".") + "."
	return strings.ToLower(fqdn)
}

func rrsetToEndpoint(rrset desec.RRSet) *endpoint.Endpoint {
	return &endpoint.Endpoint{
		DNSName:    canonicalize(rrset.Name),
		Targets:    rrset.Records,
		RecordType: rrset.Type,
		RecordTTL:  endpoint.TTL(rrset.TTL),
	}
}

func rrsetsToEndpoints(ctx context.Context, rrsets []desec.RRSet, logger *slog.Logger) ([]*endpoint.Endpoint, error) {
	var endpoints []*endpoint.Endpoint
	for _, rrset := range rrsets {
		endpoints = append(endpoints, rrsetToEndpoint(rrset))
	}

	return endpoints, nil
}

func partialRRSetFromFQDN(fqdn string, domain string) desec.RRSet {
	fqdn = canonicalize(fqdn)
	domain = canonicalize(domain)
	subName := strings.TrimSuffix(fqdn, "."+domain)
	return desec.RRSet{
		Name:    fqdn,
		Domain:  domain,
		SubName: subName,
	}
}

func endpointsToRRSets(endpoints []*endpoint.Endpoint, domain string) ([]desec.RRSet, error) {
	var rrsets []desec.RRSet

	for _, e := range endpoints {
		record := partialRRSetFromFQDN(e.DNSName, domain)
		record.Type = e.RecordType
		record.Records = e.Targets

		if time.Duration(e.RecordTTL)*time.Second < time.Hour {
			e.RecordTTL = endpoint.TTL(time.Hour / time.Second)
		}
		record.TTL = int(e.RecordTTL)

		rrsets = append(rrsets, record)
	}

	return rrsets, nil
}

func endpointsToRRSetsIdentifiers(endpoints []*endpoint.Endpoint, domain string) []desec.RRSet {
	var rrsets []desec.RRSet

	for _, e := range endpoints {
		record := partialRRSetFromFQDN(e.DNSName, domain)
		record.Type = e.RecordType

		rrsets = append(rrsets, record)
	}

	return rrsets
}
