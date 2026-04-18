package desec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nrdcg/desec"
	"sigs.k8s.io/external-dns/endpoint"
)

var (
	ErrInvalidLabelType    = errors.New("invalid type for label record")
	ErrInvalidLabelRecords = errors.New("invalid number of label records")
	ErrSpecialCharacter    = errors.New("contains special character")
	ErrInvalidKeyValuePair = errors.New("invalid key-value pair")
	ErrMissingQuotes       = errors.New("missing quotes")
	ErrUnexpectedDomain    = errors.New("unexpected domain")
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

func join(name, dn string) string {
	return canonicalize(name + "." + dn)
}

type labels map[string]string

func rrsetToEndpoint(rrset desec.RRSet, labels labels) *endpoint.Endpoint {
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
	"a":          unit{},
	"aaaa":       unit{},
	"afsdb":      unit{},
	"apl":        unit{},
	"caa":        unit{},
	"cert":       unit{},
	"cname":      unit{},
	"dhcid":      unit{},
	"dname":      unit{},
	"dlv":        unit{},
	"eui48":      unit{},
	"eui64":      unit{},
	"hinfo":      unit{},
	"https":      unit{},
	"kx":         unit{},
	"l32":        unit{},
	"l64":        unit{},
	"loc":        unit{},
	"lp":         unit{},
	"mx":         unit{},
	"naptr":      unit{},
	"nid":        unit{},
	"ns":         unit{},
	"openpgpkey": unit{},
	"ptr":        unit{},
	"rp":         unit{},
	"smimea":     unit{},
	"spf":        unit{},
	"srv":        unit{},
	"sshfp":      unit{},
	"svcb":       unit{},
	"tlsa":       unit{},
	"txt":        unit{},
	"uri":        unit{},
}

func couldBeLabel(rrset desec.RRSet) bool {
	if strings.ToLower(rrset.Type) != "txt" {
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

func parseLabels(rrset desec.RRSet) (labels, error) {
	if strings.ToLower(rrset.Type) != "txt" {
		return nil, fmt.Errorf("%w: expected TXT, got %s", ErrInvalidLabelType, rrset.Type)
	}

	if len(rrset.Records) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one, instead got %d", ErrInvalidLabelRecords, len(rrset.Records))
	}

	value := rrset.Records[0]
	if !strings.HasPrefix(value, "\"") || !strings.HasSuffix(value, "\"") {
		return nil, fmt.Errorf("invalid label value: %w", ErrMissingQuotes)
	}
	value = value[1 : len(value)-1]

	labels := labels{}
	for _, pair := range strings.Split(value, ",") {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: expected exactly 2 parts, got %d", ErrInvalidKeyValuePair, len(parts))
		}

		key, value := parts[0], parts[1]
		labels[key] = value
	}

	return labels, nil
}

func hasSpecialChar(str string) bool {
	return strings.Contains(str, ",") || strings.Contains(str, "=")
}

func endcodeLabels(labels labels) (string, error) {
	var pairs []string
	for key, value := range labels {
		if hasSpecialChar(key) {
			return "", fmt.Errorf("invalid key %q: %w", ErrSpecialCharacter)
		}
		if hasSpecialChar(value) {
			return "", fmt.Errorf("invalid value %q: %w", ErrSpecialCharacter)
		}

		pairs = append(pairs, key+"="+value)
	}

	return "\"" + strings.Join(pairs, ",") + "\"", nil
}

func groupRRSets(ctx context.Context, rrsets []desec.RRSet, logger *slog.Logger) ([]*endpoint.Endpoint, error) {
	potentialLabels := map[string]labels{}

	// First pass: take all TXT RRSets that could be labels and populate potentialLabels
	for _, rrset := range rrsets {
		if couldBeLabel(rrset) {
			labels, err := parseLabels(rrset)
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
		if !hasLabels {
			logger.DebugContext(ctx, "ignoring RRSet as it has no labels associated", "rrset", rrset, "labelName", ln)
			continue
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

func endpointsToRRSets(endpoints []*endpoint.Endpoint, domain string) ([]desec.RRSet, error) {
	var rrsets []desec.RRSet

	for _, endpoint := range endpoints {
		// For each endpoint, two RRSets are created:
		// 1. The actual RRSet for the DNS record
		// 2. The RRSet to store the labels

		record := partialRRSetFromFQDN(endpoint.DNSName, domain)
		record.Type = endpoint.RecordType
		record.Records = endpoint.Targets
		record.TTL = int(endpoint.RecordTTL)

		ln := labelName(record)
		labelRecord := partialRRSetFromFQDN(ln, domain)
		labelRecord.Type = "TXT"
		lbls, err := endcodeLabels(labels(endpoint.Labels))
		if err != nil {
			return nil, fmt.Errorf("could not encode labels for endpoint %q: %w", endpoint.DNSName, err)
		}
		labelRecord.Records = []string{lbls}
		record.TTL = int(endpoint.RecordTTL)

		rrsets = append(rrsets, record, labelRecord)
	}

	return rrsets, nil
}

func endpointsToRRSetsIdentifiers(endpoints []*endpoint.Endpoint, domain string) []desec.RRSet {
	var rrsets []desec.RRSet

	for _, endpoint := range endpoints {
		// For each endpoint, two RRSets are created:
		// 1. The actual RRSet for the DNS record
		// 2. The RRSet to store the labels

		record := partialRRSetFromFQDN(endpoint.DNSName, domain)
		record.Type = endpoint.RecordType

		ln := labelName(record)
		labelRecord := partialRRSetFromFQDN(ln, domain)
		labelRecord.Type = "TXT"

		rrsets = append(rrsets, record, labelRecord)
	}

	return rrsets
}
