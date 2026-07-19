package ldap

import (
	"context"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/teapotovh/teapot/lib/observability"
	"go.opentelemetry.io/otel/attribute"
)

func bind(ctx context.Context, metrics *metrics, conn *ldap.Conn, username, password string) (err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.bind")
	defer observability.SpanEnd(span, err)

	span.SetAttributes(attribute.String("username", username))

	start := time.Now()

	err = conn.Bind(username, password)

	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	duration := time.Since(start).Seconds()
	metrics.duration.WithLabelValues(metricsOperationBind, status).Observe(duration)

	return err
}

func search(ctx context.Context, metrics *metrics, conn *ldap.Conn, searchRequest *ldap.SearchRequest) (result *ldap.SearchResult, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.search")
	defer observability.SpanEnd(span, err)

	start := time.Now()

	res, err := conn.Search(searchRequest)

	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	duration := time.Since(start).Seconds()
	metrics.duration.WithLabelValues(metricsOperationSearch, status).Observe(duration)

	return res, err
}

func passwd(
	metrics *metrics,
	conn *ldap.Conn,
	passwordModifyRequest *ldap.PasswordModifyRequest,
) (res *ldap.PasswordModifyResult, err error) {
	start := time.Now()

	res, err = conn.PasswordModify(passwordModifyRequest)

	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	duration := time.Since(start).Seconds()
	metrics.duration.WithLabelValues(metricsOperationPasswd, status).Observe(duration)

	return res, err
}
