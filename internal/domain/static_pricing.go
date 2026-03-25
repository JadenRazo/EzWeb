package domain

import (
	"context"
	"strings"
)

// tldPricing holds the static register and renewal prices for a TLD.
type tldPricing struct {
	register float64
	renew    float64
}

// staticPrices contains ballpark market prices for common TLDs.  These are
// intentionally conservative estimates; users should verify with a live
// provider before quoting.
var staticPrices = map[string]tldPricing{
	"com":    {register: 12.99, renew: 12.99},
	"net":    {register: 13.99, renew: 13.99},
	"org":    {register: 13.99, renew: 13.99},
	"io":     {register: 39.99, renew: 39.99},
	"dev":    {register: 14.99, renew: 14.99},
	"co":     {register: 29.99, renew: 29.99},
	"me":     {register: 19.99, renew: 19.99},
	"app":    {register: 14.99, renew: 14.99},
	"xyz":    {register: 9.99, renew: 9.99},
	"info":   {register: 9.99, renew: 9.99},
	"biz":    {register: 11.99, renew: 11.99},
	"us":     {register: 9.99, renew: 9.99},
	"tech":   {register: 49.99, renew: 49.99},
	"online": {register: 39.99, renew: 39.99},
	"store":  {register: 59.99, renew: 59.99},
	"site":   {register: 24.99, renew: 24.99},
}

// StaticProvider returns hardcoded price estimates when no live registrar API
// keys are configured.  It never performs a real availability check — every
// domain is returned as Available: true so the UI can still show ballpark
// pricing.  Results are tagged with Provider "estimate" so the UI can display
// an appropriate disclaimer.
type StaticProvider struct{}

// NewStaticProvider creates a StaticProvider.
func NewStaticProvider() *StaticProvider {
	return &StaticProvider{}
}

// Name implements Provider.
func (s *StaticProvider) Name() string { return "estimate" }

// CheckAvailability implements Provider.  It returns a best-effort price
// estimate for the domain's TLD regardless of actual registration status.
func (s *StaticProvider) CheckAvailability(_ context.Context, domain string) (*DomainResult, error) {
	tld := extractTLD(domain)
	pricing, ok := staticPrices[tld]
	if !ok {
		// Unknown TLD — return a generic estimate so the search still yields a row.
		pricing = tldPricing{register: 19.99, renew: 19.99}
	}

	return &DomainResult{
		Available:     true,
		RegisterPrice: pricing.register,
		RenewPrice:    pricing.renew,
		Provider:      "estimate",
		Currency:      "USD",
		TLD:           tld,
		Domain:        domain,
	}, nil
}

// extractTLD returns the last label of a domain name in lower case, e.g.
// "example.com" → "com", "my.company.io" → "io".
func extractTLD(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}
	return parts[len(parts)-1]
}
