package mcp

import (
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) HTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return s.MCP
	}, nil)
}

func Bearer(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
