package desec

import (
	"context"
	"errors"
	"log/slog"
	"slices"
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

	for part := range strings.SplitSeq(dn, ".") {
		if len(part) > 0 {
			parts = append(parts, part)
		}
	}

	fqdn := strings.Join(parts, ".") + "."

	return strings.ToLower(fqdn)
}

func rrsetToEndpoint(rrset desec.RRSet, labels endpoint.Labels) *endpoint.Endpoint {
	return &endpoint.Endpoint{
		DNSName:    canonicalize(rrset.Name),
		Targets:    rrset.Records,
		RecordType: rrset.Type,
		RecordTTL:  endpoint.TTL(rrset.TTL),
		Labels:     map[string]string(labels),
	}
}

var labelSeparator = "-"

type unit struct{}

var recordTypesSet = map[string]unit{
	"a":          {},
	"aaaa":       {},
	"afsdb":      {},
	"apl":        {},
	"caa":        {},
	"cert":       {},
	"cname":      {},
	"dhcid":      {},
	"dname":      {},
	"dlv":        {},
	"eui48":      {},
	"eui64":      {},
	"hinfo":      {},
	"https":      {},
	"kx":         {},
	"l32":        {},
	"l64":        {},
	"loc":        {},
	"lp":         {},
	"mx":         {},
	"naptr":      {},
	"nid":        {},
	"ns":         {},
	"openpgpkey": {},
	"ptr":        {},
	"rp":         {},
	"smimea":     {},
	"spf":        {},
	"srv":        {},
	"sshfp":      {},
	"svcb":       {},
	"tlsa":       {},
	"txt":        {},
	"uri":        {},
}

func couldBeLabel(rrset desec.RRSet) bool {
	if strings.ToLower(rrset.Type) != "txt" {
		return false
	}

	if len(rrset.Records) <= 0 {
		return false
	}

	parts := strings.Split(rrset.Name, labelSeparator)
	if len(parts) <= 1 {
		return false
	}

	_, ok := recordTypesSet[parts[0]]

	return ok
}

func labelName(rrset desec.RRSet) string {
	return canonicalize(rrset.Type + labelSeparator + rrset.Name)
}

func groupRRSets(ctx context.Context, rrsets []desec.RRSet, logger *slog.Logger) ([]*endpoint.Endpoint, error) {
	potentialLabels := map[string]endpoint.Labels{}

	// First pass: take all TXT RRSets that could be labels and populate potentialLabels
	for _, rrset := range rrsets {
		if couldBeLabel(rrset) {
			labels, err := endpoint.NewLabelsFromStringPlain(rrset.Records[0])
			if err != nil {
				logger.DebugContext(ctx, "ignoring potential labels RRSet due to error", "rrset", rrset, "err", err)
				continue
			}

			potentialLabels[rrset.Name] = labels
		}
	}

	var endpoints []*endpoint.Endpoint

	for _, rrset := range rrsets {
		ln := labelName(rrset)
		labels, hasLabels := potentialLabels[ln]
		_, isLabel := potentialLabels[rrset.Name]
		// If the RRSet is not a label itself and it has no labels, we have some discrepancy
		if !isLabel && !hasLabels {
			logger.WarnContext(ctx, "RRSet has no labels associated", "rrset", rrset, "labelName", ln)
		}

		endpoint := rrsetToEndpoint(rrset, labels)
		endpoints = append(endpoints, endpoint)
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

func ttl(raw endpoint.TTL) int {
	if time.Duration(raw)*time.Second < time.Hour {
		return int(time.Hour / time.Second)
	}

	return int(raw)
}

func endpointsToRRSets(endpoints []*endpoint.Endpoint, domain string) ([]desec.RRSet, error) {
	var rrsets []desec.RRSet

	for _, e := range endpoints {
		record := partialRRSetFromFQDN(e.DNSName, domain)
		record.Type = e.RecordType
		record.Records = e.Targets
		record.TTL = ttl(e.RecordTTL)

		rrsets = append(rrsets, record)
	}

	return rrsets, nil
}

func endpointsToRRSetsIdentifiers(endpoints []*endpoint.Endpoint, domain string) []desec.RRSet {
	var rrsets []desec.RRSet

	for _, e := range endpoints {
		record := partialRRSetFromFQDN(e.DNSName, domain)
		record.Type = e.RecordType
		record.TTL = ttl(e.RecordTTL)

		rrsets = append(rrsets, record)
	}

	return rrsets
}

func filterRRSets(rrsets []desec.RRSet, managedTypes []string) []desec.RRSet {
	var result []desec.RRSet

	for _, rrset := range rrsets {
		if slices.Contains(managedTypes, strings.ToLower(rrset.Type)) {
			result = append(result, rrset)
		}
	}

	return result
}
