package loadbalancer

import (
	"fmt"
	"net/netip"
	"slices"

	v1 "k8s.io/api/core/v1"
)

type Event []netip.Addr

func serviceIPs(s *v1.Service) ([]netip.Addr, error) {
	var result []netip.Addr
	for _, ip := range s.Spec.ExternalIPs {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, fmt.Errorf("could not parse %q as IP address: %w", ip, err)
		}

		result = append(result, addr)
	}

	return result, nil
}

func (lb *LoadBalancer) handle(name string, s *v1.Service, exists bool) error {
	lb.logger.Debug("received update regarding service", "name", name, "exists", exists)

	if !exists {
		delete(lb.state, name)
	} else {
		ips, err := serviceIPs(s)
		if err != nil {
			return fmt.Errorf("could not update state: %w", err)
		}

		lb.state[name] = ips
	}

	event := lb.event()
	if !slices.Equal(event, lb.prevEvent) {
		lb.broker.Publish(event)
		lb.prevEvent = event
	}
	return nil
}

type unit struct{}

func (lb *LoadBalancer) event() Event {
	set := map[netip.Addr]unit{}
	for _, ips := range lb.state {
		for _, ip := range ips {
			if _, ok := set[ip]; !ok {
				set[ip] = unit{}
			}
		}
	}

	var result Event
	for ip := range set {
		result = append(result, ip)
	}
	slices.SortFunc(result, func(a, b netip.Addr) int {
		return a.Compare(b)
	})
	return result
}
