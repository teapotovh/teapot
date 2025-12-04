package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"slices"

	"github.com/vishvananda/netlink"

	"github.com/teapotovh/teapot/lib/run"
	tnet "github.com/teapotovh/teapot/service/net"
	"github.com/teapotovh/teapot/service/net/internal"
	"github.com/teapotovh/teapot/service/net/wireguard"
)

var (
	ErrBirdNotRunning = errors.New("bird process not running")
)

const (
	CustomProtocol = 200
)

type route struct {
	target netip.Prefix
	via    netip.Addr
}

func directRoute(target netip.Prefix) route {
	return route{target: target, via: netip.IPv4Unspecified()}
}

func (r route) isDirect() bool {
	return r.via == netip.IPv4Unspecified()
}

func (r route) String() string {
	if r.isDirect() {
		return fmt.Sprintf("dst=%s", r.target)
	}

	return fmt.Sprintf("dst=%s, via=%s", r.target, r.via)
}

type unit struct{}

type Router struct {
	logger *slog.Logger
	net    *tnet.Net

	routes map[route]unit
	link   netlink.Link

	cluster tnet.ClusterEvent
}

type RouterConfig struct {
	Device string
}

func NewRouter(net *tnet.Net, config RouterConfig, logger *slog.Logger) (*Router, error) {
	link, err := netlink.LinkByName(config.Device)
	if err != nil {
		return nil, fmt.Errorf("could not get network device %q: %w", config.Device, err)
	}

	return &Router{
		logger: logger,
		net:    net,

		routes: make(map[route]unit),
		link:   link,
	}, nil
}

func (r *Router) netlinkRoute(route route) (*netlink.Route, error) {
	dst, err := internal.PrefixToIPNet(route.target)
	if err != nil {
		return nil, fmt.Errorf("error while parsing route target: %w", err)
	}

	nlr := netlink.Route{
		Dst:       dst,
		LinkIndex: r.link.Attrs().Index,
		Protocol:  CustomProtocol,
	}
	if !route.isDirect() {
		nlr.Gw = route.via.AsSlice()
	} else {
		nlr.Scope = netlink.SCOPE_LINK
	}

	return &nlr, nil
}

func (r *Router) addRoute(route route) error {
	r.logger.Info("adding route", "route", route, "device", r.link.Attrs().Name)

	nlr, err := r.netlinkRoute(route)
	if err != nil {
		return err
	}

	if err := netlink.RouteAdd(nlr); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("error while adding netlink route: %w", err)
	}

	r.routes[route] = unit{}
	return nil
}

func (r *Router) delRoute(route route) error {
	r.logger.Info("deleting route", "route", route, "device", r.link.Attrs().Name)

	nlr, err := r.netlinkRoute(route)
	if err != nil {
		return err
	}

	if err := netlink.RouteDel(nlr); err != nil {
		return fmt.Errorf("error while removing netlink route: %w", err)
	}

	delete(r.routes, route)
	return nil
}

func (r *Router) configureRoutes() error {
	routes := make(map[route]unit)
	// Derive routes from the cluster information:
	for _, node := range r.cluster {
		if node.IsLocal {
			continue
		}

		// 1. For each node, we add a point-to-point route over the wireguard device
		target := netip.PrefixFrom(node.InternalIP, wireguard.NodePrefix)
		routes[directRoute(target)] = unit{}

		// 2. For each node's CIDRs we add a route using that node's internal IP
		// as the destination.
		for _, cidr := range node.CIDRs {
			routes[route{target: cidr, via: node.InternalIP}] = unit{}
		}
	}

	// Diff the currently configured routes with the derived desired state
	var toadd []route
	for droute := range routes {
		if _, ok := r.routes[droute]; !ok {
			toadd = append(toadd, droute)
		}
	}

	// Sort the `toadd` routes. When adding routes, we must be careful. The routes
	// for the point-to-point communication, which will later serve as gateways
	// must be added before the CIDR routes.
	slices.SortFunc(toadd, func(r1, r2 route) int {
		if r1.isDirect() && !r2.isDirect() {
			return -1
		} else if r2.isDirect() && !r1.isDirect() {
			return 1
		} else {
			return 0
		}
	})

	for _, route := range toadd {
		// Desired route is missing, let's add it
		if err := r.addRoute(route); err != nil {
			return fmt.Errorf("error while adding new route %q: %s", route, err)
		}
	}

	for eroute := range r.routes {
		if _, ok := routes[eroute]; !ok {
			// Existing route is no longer desired, let's remove it
			if err := r.delRoute(eroute); err != nil {
				return fmt.Errorf("error while removing stale route %q: %s", eroute, err)
			}
		}
	}

	return nil
}

func (r *Router) cleanupRoutes() error {
	for eroute := range r.routes {
		if err := r.delRoute(eroute); err != nil {
			return fmt.Errorf("error while removing stale route %q: %s", eroute, err)
		}
	}

	return nil
}

func (r *Router) Run(ctx context.Context, notify run.Notify) error {
	csub := r.net.Cluster().Broker().Subscribe()
	defer csub.Unsubscribe()

	// TODO: handle error
	defer r.cleanupRoutes()

	notify.Notify()
	for cluster := range csub.Iter(ctx) {
		r.cluster = cluster

		if err := r.configureRoutes(); err != nil {
			return fmt.Errorf("error while configuring routes: %w", err)
		}
	}

	return nil
}
