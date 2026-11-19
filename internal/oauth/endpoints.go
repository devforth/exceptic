package oauth

import (
	"net/http"
)

// MaxBody bounds the body of every OAuth request, the consent form included.
const MaxBody = 16 << 10

// Requests per client network and minute to the open endpoints: agents
// register once and refresh about hourly, so these leave plenty of room.
// Registrations also have a total, since each one is a synced write; tokens
// have none, so a flood cannot stop connected agents from refreshing.
const (
	registrationsPerMinute = 30
	registrationsTotal     = 120
	tokenRequestsPerMinute = 120
)

// Register adds the open endpoints of the flow to router, each wrapped with
// protect (the IP allowlist). The consent page is the UI's.
//
// Agents look for the metadata in several places (RFC 8414 and 9728 put the
// path after /.well-known/, OpenID Connect before it), so every one of them
// answers, with or without the path of the MCP endpoint.
func (authorization *Server) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	// Registration and tokens are open to anyone the allowlist lets in, so
	// they are rate limited per client address.
	routes := map[string]http.Handler{
		"GET /.well-known/oauth-protected-resource":    http.HandlerFunc(authorization.protectedResourceMetadata),
		"GET /.well-known/oauth-protected-resource/":   http.HandlerFunc(authorization.protectedResourceMetadata),
		"GET /.well-known/oauth-authorization-server":  http.HandlerFunc(authorization.authorizationServerMetadata),
		"GET /.well-known/oauth-authorization-server/": http.HandlerFunc(authorization.authorizationServerMetadata),
		"GET /.well-known/openid-configuration":        http.HandlerFunc(authorization.authorizationServerMetadata),
		"GET /.well-known/openid-configuration/":       http.HandlerFunc(authorization.authorizationServerMetadata),
		"GET " + jwksPath:                              http.HandlerFunc(authorization.jwks),
		"POST " + RegisterPath:                         authorization.limit(&requestLimit{perMinute: registrationsPerMinute, total: registrationsTotal}, authorization.register),
		"POST " + TokenPath:                            authorization.limit(&requestLimit{perMinute: tokenRequestsPerMinute}, authorization.token),
	}

	for pattern, route := range routes {
		router.Handle(pattern, protect(authorization.guard(route)))
	}
}
