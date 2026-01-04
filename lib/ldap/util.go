package ldap

import (
	"time"

	"github.com/go-ldap/ldap/v3"
)

func bind(metrics *metrics, conn *ldap.Conn, username, password string) error {
	start := time.Now()

	err := conn.Bind(username, password)

	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	duration := time.Since(start).Seconds()
	metrics.duration.WithLabelValues(metricsOperationBind, status).Observe(duration)

	return err
}

func search(metrics *metrics, conn *ldap.Conn, searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
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
) (*ldap.PasswordModifyResult, error) {
	start := time.Now()

	res, err := conn.PasswordModify(passwordModifyRequest)

	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	duration := time.Since(start).Seconds()
	metrics.duration.WithLabelValues(metricsOperationPasswd, status).Observe(duration)

	return res, err
}
