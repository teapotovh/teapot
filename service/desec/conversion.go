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
	ErrInvalidLabelRecords = errors.New("invalid number of label records")
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

func join(name, dn string) string {
	return canonicalize(name + "." + dn)
}

type labels map[string]string

func rrsetToEndpoint(rrset desec.RRSet, labels labels) *endpoint.Endpoint {
	dnsName := join(rrset.Name, rrset.Domain)
	return &endpoint.Endpoint{
		DNSName:    dnsName,
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
	"cdnskey":    unit{},
	"cds":        unit{},
	"cert":       unit{},
	"cname":      unit{},
	"dhcid":      unit{},
	"dname":      unit{},
	"dnskey":     unit{},
	"dlv":        unit{},
	"ds":         unit{},
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
	_, ok := recordTypesSet[parts[0]]
	return ok
}

func labelName(rrset desec.RRSet) string {
	return canonicalize(rrset.Type + labelSeparator + rrset.Name)
}

func parseLabels(rrset desec.RRSet) (labels, error) {
	if len(rrset.Records) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one, instead got %d", ErrInvalidLabelRecords, len(rrset.Records))
	}

	value := rrset.Records[0]
	fmt.Println(value, strings.HasPrefix(value, "\""), strings.HasSuffix(value, "\""))
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
