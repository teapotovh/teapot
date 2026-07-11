package internal

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

var (
	ErrNoSRV                       = errors.New("domain doesn't have an SRV record")
	ErrEmptySRV                    = errors.New("empty target in SRV record")
	ErrMissingPathKey              = errors.New("missing path key")
	ErrEmptyTXTEntries             = errors.New("empty TXT entries")
	ErrTooManyTXTEntries           = errors.New("too many TXT entries")
	ErrUntyped                     = errors.New("untyped error")
	ErrMultiStatusFailed           = errors.New("HTTP multi-status request failed")
	ErrUnexpectedNumberOfResponses = errors.New("unexpected number of responses")
	ErrMismatchedServerVersion     = errors.New("webdav: server doesn't support DAV class 1")
)

// DiscoverContextURL performs a DNS-based CardDAV/CalDAV service discovery as
// described in RFC 6764. It returns the URL to the CardDAV/CalDAV server.
// Specifically it implements points 2 and 3 from the bootstrapping procedure
// defined in RFC 6764 section 6.
func DiscoverContextURL(ctx context.Context, service, domain string) (string, error) {
	var resolver net.Resolver

	// Only lookup TLS records, plaintext connections are insecure
	_, addrs, err := resolver.LookupSRV(ctx, service+"s", "tcp", domain)

	dnsErr := &net.DNSError{}
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTemporary {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	if len(addrs) == 0 {
		return "", fmt.Errorf("webdav: %w", ErrNoSRV)
	}

	addr := addrs[0]

	target := strings.TrimSuffix(addr.Target, ".")
	if target == "" {
		return "", fmt.Errorf("webdav: %w", ErrEmptySRV)
	}

	txtName := fmt.Sprintf("_%ss._tcp.%s", service, domain)

	txtRecords, err := resolver.LookupTXT(ctx, txtName)

	dnsErr = &net.DNSError{}
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTemporary {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	var path string

	switch len(txtRecords) {
	case 0:
		path = "/.well-known/" + service
	case 1:
		record := txtRecords[0]
		if !strings.HasPrefix(record, "path=") {
			return "", fmt.Errorf("webdav: invalid TXT record for %s: %w", txtName, ErrMissingPathKey)
		}

		path = strings.TrimPrefix(record, "path=")
		if path == "" {
			return "", fmt.Errorf("webdav: while doing discovery for %s: %w", txtName, ErrEmptyTXTEntries)
		}
	default: // more than 1
		return "", fmt.Errorf("webdav: while doing discovery for %s: %w", txtName, ErrTooManyTXTEntries)
	}

	u := url.URL{
		Scheme: "https",
		Path:   path,
	}
	if addr.Port == 443 {
		u.Host = target
	} else {
		u.Host = fmt.Sprintf("%v:%v", target, addr.Port)
	}

	return u.String(), nil
}

// HTTPClient performs HTTP requests. It's implemented by *http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	http     HTTPClient
	endpoint *url.URL
}

func NewClient(c HTTPClient, endpoint string) (*Client, error) {
	if c == nil {
		c = http.DefaultClient
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	if u.Path == "" {
		// This is important to avoid issues with path.Join
		u.Path = "/"
	}

	return &Client{http: c, endpoint: u}, nil
}

func (c *Client) ResolveHref(p string) *url.URL {
	if !strings.HasPrefix(p, "/") {
		p = path.Join(c.endpoint.Path, p)
	}

	return &url.URL{
		Scheme: c.endpoint.Scheme,
		User:   c.endpoint.User,
		Host:   c.endpoint.Host,
		Path:   p,
	}
}

func (c *Client) NewRequest(method string, path string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, c.ResolveHref(path).String(), body)
}

func (c *Client) NewXMLRequest(method string, path string, v any) (*http.Request, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)

	if err := xml.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}

	req, err := c.NewRequest(method, path, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")

	return req, nil
}

func (c *Client) Do(req *http.Request) (resp *http.Response, err error) {
	resp, err = c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode/100 != 2 {
		defer func() {
			if e := resp.Body.Close(); e != nil && err != nil {
				err = fmt.Errorf("error while closing response body: %w", err)
			}
		}()

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/plain"
		}

		var wrappedErr error

		t, _, _ := mime.ParseMediaType(contentType)
		if t == "application/xml" || t == "text/xml" {
			var davErr Error
			if err := xml.NewDecoder(resp.Body).Decode(&davErr); err != nil {
				wrappedErr = err
			} else {
				wrappedErr = &davErr
			}
		} else if strings.HasPrefix(t, "text/") {
			lr := io.LimitedReader{R: resp.Body, N: 1024}

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, &lr); err != nil {
				return nil, fmt.Errorf("error while copying response into LimitedReader: %w", err)
			}

			if err := resp.Body.Close(); err != nil {
				return nil, fmt.Errorf("error while closing response body: %w", err)
			}

			if s := strings.TrimSpace(buf.String()); s != "" {
				if lr.N == 0 {
					s += " […]"
				}

				wrappedErr = fmt.Errorf("%w: %q", ErrUntyped, s)
			}
		}

		return nil, &daverr.HTTPError{Code: resp.StatusCode, Err: wrappedErr}
	}

	return resp, nil
}

func (c *Client) DoMultiStatus(req *http.Request) (msptr *MultiStatus, err error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := resp.Body.Close(); e != nil && err != nil {
			err = fmt.Errorf("error while closing response body: %w", err)
		}
	}()

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("%w: with status code %v", ErrMultiStatusFailed, resp.Status)
	}

	// TODO: the response can be quite large, support streaming Response elements
	var ms MultiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}

	return &ms, nil
}

func (c *Client) PropFind(ctx context.Context, path string, depth Depth, propfind *PropFind) (*MultiStatus, error) {
	req, err := c.NewXMLRequest("PROPFIND", path, propfind)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Depth", depth.String())

	return c.DoMultiStatus(req.WithContext(ctx))
}

// PropFindFlat performs a PROPFIND request with a zero depth.
func (c *Client) PropFindFlat(ctx context.Context, path string, propfind *PropFind) (*Response, error) {
	ms, err := c.PropFind(ctx, path, DepthZero, propfind)
	if err != nil {
		return nil, err
	}

	// If the client followed a redirect, the Href might be different from the request path
	if len(ms.Responses) != 1 {
		return nil, fmt.Errorf(
			"PROPFIND with depth 0: received %d responses: %w",
			len(ms.Responses),
			ErrUnexpectedNumberOfResponses,
		)
	}

	return &ms.Responses[0], nil
}

func parseCommaSeparatedSet(values []string, upper bool) map[string]bool {
	m := make(map[string]bool)

	for _, v := range values {
		fields := strings.FieldsFunc(v, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		for _, f := range fields {
			if upper {
				f = strings.ToUpper(f)
			} else {
				f = strings.ToLower(f)
			}

			m[f] = true
		}
	}

	return m
}

func (c *Client) Options(
	ctx context.Context,
	path string,
) (classes map[string]bool, methods map[string]bool, err error) {
	req, err := c.NewRequest(http.MethodOptions, path, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}

	if err := resp.Body.Close(); err != nil {
		return nil, nil, fmt.Errorf("error while closing response body: %w", err)
	}

	classes = parseCommaSeparatedSet(resp.Header["Dav"], false)
	if !classes["1"] {
		return nil, nil, ErrMismatchedServerVersion
	}

	methods = parseCommaSeparatedSet(resp.Header["Allow"], true)

	return classes, methods, nil
}

// SyncCollection perform a `sync-collection` REPORT operation on a resource.
func (c *Client) SyncCollection(
	ctx context.Context,
	path, syncToken string,
	level Depth,
	limit *Limit,
	prop *Prop,
) (*MultiStatus, error) {
	q := SyncCollectionQuery{
		SyncToken: syncToken,
		SyncLevel: level.String(),
		Limit:     limit,
		Prop:      prop,
	}

	req, err := c.NewXMLRequest("REPORT", path, &q)
	if err != nil {
		return nil, err
	}

	ms, err := c.DoMultiStatus(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return ms, nil
}
