package s3cache

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// NewClient creates an S3 client. A nil transport uses the default transport.
func NewClient(endpoint, region string, transport http.RoundTripper) (*minio.Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the S3 URL: %w", err)
	}

	key := u.User.Username()
	secret, _ := u.User.Password()

	if transport == nil {
		transport = http.DefaultTransport
	}

	client, err := minio.New(u.Host, &minio.Options{
		Creds:     credentials.NewStaticV4(key, secret, ""),
		Secure:    u.Scheme == "https",
		Region:    region,
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("error while creating the S3 client: %w", err)
	}

	return client, nil
}
