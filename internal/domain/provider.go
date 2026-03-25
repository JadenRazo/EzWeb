package domain

import "context"

// DomainResult holds the pricing and availability data returned by a provider
// for a single domain lookup.
type DomainResult struct {
	Available     bool
	RegisterPrice float64
	RenewPrice    float64
	Provider      string
	Currency      string
	TLD           string
	Domain        string
}

// Provider is the interface that all domain registrar backends must implement.
type Provider interface {
	Name() string
	CheckAvailability(ctx context.Context, domain string) (*DomainResult, error)
}
