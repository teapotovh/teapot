package observability

import (
	"net"
	"strconv"
	"time"

	flag "github.com/spf13/pflag"
)

func ObservabilityFlagSet(serviceName string) (*flag.FlagSet, func() ObservabilityConfig) {
	fs := flag.NewFlagSet("observability", flag.ExitOnError)

	ip := fs.IP("observability-ip", net.IPv4zero, "the address on which to open the HTTP server for observability")
	port := fs.Int16("observability-port", 8146, "the port on which to open the HTTP server for observability")

	tracingEndpoint := fs.String(
		"observability-tracing-endpoint",
		"0.0.0.0:4137",
		"the grpc endpoint for OTLP opentelemetry trace collection",
	)
	tracingServiceName := fs.String(
		"observability-tracing-service-name",
		serviceName,
		"the opentelemetry service name for traces produced by this application",
	)
	tracingConnectTimeout := fs.Duration(
		"observability-tracing-connect-timeout",
		time.Minute,
		"the timeout for the initial connection to the opentelemetry-collector",
	)

	return fs, func() ObservabilityConfig {
		return ObservabilityConfig{
			Address: net.JoinHostPort(ip.String(), strconv.Itoa(int(*port))),
			Tracing: ObservabilityTracingConfig{
				Endpoint:       *tracingEndpoint,
				ServiceName:    *tracingServiceName,
				ConnectTimeout: *tracingConnectTimeout,
			},
		}
	}
}
