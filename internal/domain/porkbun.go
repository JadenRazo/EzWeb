package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// PorkbunProvider queries the Porkbun API for domain pricing.  It uses the
// pricing endpoint as the primary data source (rate limits are generous) and
// supplements with a separate availability check when feasible.
type PorkbunProvider struct {
	apiKey    string
	secretKey string
	client    *http.Client
}

// NewPorkbunProvider creates a PorkbunProvider with the supplied credentials.
func NewPorkbunProvider(apiKey, secretKey string) *PorkbunProvider {
	return &PorkbunProvider{
		apiKey:    apiKey,
		secretKey: secretKey,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Provider.
func (p *PorkbunProvider) Name() string { return "porkbun" }

// porkbunCreds is the common credential body required by every Porkbun API
// call.
type porkbunCreds struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
}

// CheckAvailability implements Provider.  It fetches TLD pricing from
// Porkbun's pricing endpoint and then performs a separate availability check
// for the specific domain.
func (p *PorkbunProvider) CheckAvailability(ctx context.Context, domain string) (*DomainResult, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	tld := extractTLD(domain)

	// Step 1: fetch TLD pricing.
	regPrice, renewPrice, err := p.fetchPricing(ctx, tld)
	if err != nil {
		log.Printf("porkbun: pricing fetch failed for tld %q: %v", tld, err)
		// Fall back to static estimate rather than failing the whole search.
		regPrice = staticPrices[tld].register
		renewPrice = staticPrices[tld].renew
	}

	// Step 2: check domain availability.
	available, err := p.checkDomainAvailability(ctx, domain)
	if err != nil {
		log.Printf("porkbun: availability check failed for %q: %v", domain, err)
		// If availability check fails, still return pricing with unknown status.
		available = false
	}

	return &DomainResult{
		Available:     available,
		RegisterPrice: regPrice,
		RenewPrice:    renewPrice,
		Provider:      "porkbun",
		Currency:      "USD",
		TLD:           tld,
		Domain:        domain,
	}, nil
}

// fetchPricing calls POST /api/json/v3/pricing/get and extracts register and
// renewal prices for the given TLD.
func (p *PorkbunProvider) fetchPricing(ctx context.Context, tld string) (register, renew float64, err error) {
	payload, _ := json.Marshal(porkbunCreds{
		APIKey:    p.apiKey,
		SecretKey: p.secretKey,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.porkbun.com/api/json/v3/pricing/get",
		bytes.NewReader(payload),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body struct {
		Status   string                     `json:"status"`
		Pricing  map[string]json.RawMessage `json:"pricing"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, 0, fmt.Errorf("decode response: %w", err)
	}
	if body.Status != "SUCCESS" {
		return 0, 0, fmt.Errorf("API returned status %q", body.Status)
	}

	raw, ok := body.Pricing[tld]
	if !ok {
		return 0, 0, fmt.Errorf("TLD %q not found in pricing response", tld)
	}

	var tldData struct {
		Registration string `json:"registration"`
		Renewal      string `json:"renewal"`
	}
	if err := json.Unmarshal(raw, &tldData); err != nil {
		return 0, 0, fmt.Errorf("unmarshal TLD pricing: %w", err)
	}

	var regF, renewF float64
	fmt.Sscanf(tldData.Registration, "%f", &regF)
	fmt.Sscanf(tldData.Renewal, "%f", &renewF)
	return regF, renewF, nil
}

// checkDomainAvailability calls POST /api/json/v3/domain/checkAvailability/{domain}.
func (p *PorkbunProvider) checkDomainAvailability(ctx context.Context, domain string) (bool, error) {
	payload, _ := json.Marshal(porkbunCreds{
		APIKey:    p.apiKey,
		SecretKey: p.secretKey,
	})

	url := fmt.Sprintf("https://api.porkbun.com/api/json/v3/domain/checkAvailability/%s", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body struct {
		Status   string `json:"status"`
		Response struct {
			// Porkbun returns "AVAILABLE" or "UNAVAILABLE" (or similar)
			Avail string `json:"avail"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	if body.Status != "SUCCESS" {
		return false, fmt.Errorf("API returned status %q", body.Status)
	}

	return strings.EqualFold(body.Response.Avail, "available"), nil
}
