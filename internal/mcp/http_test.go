package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPBearerUnauthorized(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))
	ts := httptest.NewServer(Bearer("secret", srv.HTTPHandler()))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.StatusCode)
	}
}

func TestHTTPBearerWrongTokenUnauthorized(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))
	ts := httptest.NewServer(Bearer("secret", srv.HTTPHandler()))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer wrong")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.StatusCode)
	}
}

func TestHTTPBearerOKInitialize(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))
	ts := httptest.NewServer(Bearer("secret", srv.HTTPHandler()))
	defer ts.Close()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: roundTripBearer("secret", http.DefaultTransport)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	got, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 7 {
		t.Fatalf("len=%d", len(got.Tools))
	}
}

func TestHTTPNoTokenSkipsAuth(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))
	ts := httptest.NewServer(Bearer("", srv.HTTPHandler()))
	defer ts.Close()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

type bearerRT struct {
	token string
	next  http.RoundTripper
}

func roundTripBearer(token string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return bearerRT{token: token, next: next}
}

func (b bearerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}
