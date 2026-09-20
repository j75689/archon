package mcp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	t.Parallel()
	srv := NewHTTPServer("127.0.0.1:0", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("ReadHeaderTimeout=%v", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Fatalf("ReadTimeout=%v", srv.ReadTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout=%v", srv.IdleTimeout)
	}
}

func TestServeHTTPLoopbackInitializeThenShutdown(t *testing.T) {
	dir := initRepo(t)
	mcpSrv := New(newTestApp(t, dir))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	httpSrv := NewHTTPServer(addr, Bearer("", mcpSrv.HTTPHandler()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ServeHTTP(ctx, httpSrv) }()

	endpoint := "http://" + addr
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	var session *mcpsdk.ClientSession
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err = client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
			Endpoint: endpoint,
		}, nil)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	got, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 7 {
		t.Fatalf("len=%d", len(got.Tools))
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP did not return after cancel")
	}
}

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

func TestHTTPBearerLowercaseSchemeAuthorized(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))
	ts := httptest.NewServer(Bearer("secret", srv.HTTPHandler()))
	defer ts.Close()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: roundTripBearerScheme("bearer", "secret", http.DefaultTransport)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
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

type bearerSchemeRT struct {
	scheme string
	token  string
	next   http.RoundTripper
}

func roundTripBearerScheme(scheme, token string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return bearerSchemeRT{scheme: scheme, token: token, next: next}
}

func (b bearerSchemeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", b.scheme+" "+b.token)
	return b.next.RoundTrip(r)
}
