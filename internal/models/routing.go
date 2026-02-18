package models

type RoutingConfig struct {
	Rules           []RoutingRule `json:"rules"`
	RedirectDomains []string      `json:"redirect_domains,omitempty"` // e.g. ["www.jadenrazo.dev"] → redirect to primary
	TLSCertPath     string        `json:"tls_cert_path,omitempty"`
	TLSKeyPath      string        `json:"tls_key_path,omitempty"`
	HTTPOnly        bool          `json:"http_only,omitempty"`        // no TLS, serve on :80 only
	ExtraDirectives []string      `json:"extra_directives,omitempty"` // raw Caddyfile lines injected into site block
}

type RoutingRule struct {
	PathPrefix  string            `json:"path_prefix,omitempty"`   // "/api/v1/urls/", empty for catch-all
	Upstream    string            `json:"upstream"`                // "localhost:8090", "45.126.38.75:20091"
	StripPrefix string            `json:"strip_prefix,omitempty"`  // prefix to strip, e.g. "/api/v1" turns /api/v1/urls/x into /urls/x
	RewritePath string            `json:"rewrite_path,omitempty"`  // full rewrite, e.g. "/api/v1/devpanel/health"
	WebSocket   bool              `json:"websocket,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`       // response headers
	CORS        *CORSConfig       `json:"cors,omitempty"`
}

type CORSConfig struct {
	Origins     []string `json:"origins"`
	Methods     string   `json:"methods"`
	Headers     string   `json:"headers"`
	Credentials bool     `json:"credentials,omitempty"`
	MaxAge      int      `json:"max_age,omitempty"`
}
