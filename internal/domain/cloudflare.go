package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// CloudflareProvider queries the Cloudflare Registrar API to check domain
// availability and pricing.  It requires a valid API token with registrar
// read access and the account ID that owns the registrar.
type CloudflareProvider struct {
	apiToken  string
	accountID string
	client    *http.Client
}

// NewCloudflareProvider creates a CloudflareProvider with the supplied
// credentials.
func NewCloudflareProvider(apiToken, accountID string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken:  apiToken,
		accountID: accountID,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Provider.
func (p *CloudflareProvider) Name() string { return "cloudflare" }

// CheckAvailability implements Provider.  It calls the Cloudflare Registrar
// domains endpoint.  A 404 response means the domain is not registered through
// Cloudflare (i.e., available); a 200 response means it is already registered.
func (p *CloudflareProvider) CheckAvailability(ctx context.Context, domain string) (*DomainResult, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	tld := extractTLD(domain)

	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/registrar/domains/%s",
		p.accountID, domain,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: request failed: %w", err)
	}
	defer resp.Body.Close()

	// 404 → domain is not registered with Cloudflare → treat as available.
	if resp.StatusCode == http.StatusNotFound {
		return &DomainResult{
			Available: true,
			Provider:  "cloudflare",
			Currency:  "USD",
			TLD:       tld,
			Domain:    domain,
			// Cloudflare does not expose public pricing in this endpoint, so
			// we fall back to the static estimate for register/renew prices.
			RegisterPrice: staticPrices[tld].register,
			RenewPrice:    staticPrices[tld].renew,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloudflare: unexpected status %d for domain %s", resp.StatusCode, domain)
	}

	// Domain exists in Cloudflare Registrar — parse pricing from response.
	var body struct {
		Success bool `json:"success"`
		Result  struct {
			Name              string  `json:"name"`
			RegistrationPrice float64 `json:"registration_price"`
			RenewalPrice      float64 `json:"renewal_price"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Printf("cloudflare: failed to decode response for %s: %v", domain, err)
		return nil, fmt.Errorf("cloudflare: decode response: %w", err)
	}

	if !body.Success {
		msgs := make([]string, 0, len(body.Errors))
		for _, e := range body.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("cloudflare: API error for %s: %s", domain, strings.Join(msgs, "; "))
	}

	// Domain is registered — not available for new registration.
	regPrice := body.Result.RegistrationPrice
	renewPrice := body.Result.RenewalPrice
	if regPrice == 0 {
		regPrice = staticPrices[tld].register
	}
	if renewPrice == 0 {
		renewPrice = staticPrices[tld].renew
	}

	return &DomainResult{
		Available:     false,
		RegisterPrice: regPrice,
		RenewPrice:    renewPrice,
		Provider:      "cloudflare",
		Currency:      "USD",
		TLD:           tld,
		Domain:        domain,
	}, nil
}
