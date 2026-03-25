package domain

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NamecheapProvider queries the Namecheap API for domain availability.
// Pricing is derived from a static table because the domains.getpricing
// endpoint requires a whitelisted IP and adds significant latency.
type NamecheapProvider struct {
	apiUser string
	apiKey  string
	client  *http.Client
}

// NewNamecheapProvider creates a NamecheapProvider with the supplied
// credentials.
func NewNamecheapProvider(apiUser, apiKey string) *NamecheapProvider {
	return &NamecheapProvider{
		apiUser: apiUser,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Provider.
func (p *NamecheapProvider) Name() string { return "namecheap" }

// namecheapResponse is used to parse the outer XML wrapper returned by every
// Namecheap API call.
type namecheapResponse struct {
	XMLName xml.Name `xml:"ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  []struct {
		Number  string `xml:"Number,attr"`
		Message string `xml:",chardata"`
	} `xml:"Errors>Error"`
	CommandResponse struct {
		DomainCheckResult []struct {
			Domain    string `xml:"Domain,attr"`
			Available string `xml:"Available,attr"`
		} `xml:"DomainCheckResult"`
	} `xml:"CommandResponse"`
}

// CheckAvailability implements Provider.  It calls namecheap.domains.check and
// pairs the availability result with static pricing for the TLD.
func (p *NamecheapProvider) CheckAvailability(ctx context.Context, domain string) (*DomainResult, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	tld := extractTLD(domain)

	params := url.Values{}
	params.Set("ApiUser", p.apiUser)
	params.Set("ApiKey", p.apiKey)
	params.Set("UserName", p.apiUser)
	params.Set("Command", "namecheap.domains.check")
	// ClientIp is required by the Namecheap API even when whitelist is not
	// strictly enforced in sandbox mode.  We use a placeholder value.
	params.Set("ClientIp", "0.0.0.0")
	params.Set("DomainList", domain)

	apiURL := "https://api.namecheap.com/xml.response?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("namecheap: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("namecheap: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("namecheap: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("namecheap: read response: %w", err)
	}

	var parsed namecheapResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("namecheap: parse XML: %w", err)
	}

	if parsed.Status == "ERROR" {
		msgs := make([]string, 0, len(parsed.Errors))
		for _, e := range parsed.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("namecheap: API error: %s", strings.Join(msgs, "; "))
	}

	available := false
	for _, result := range parsed.CommandResponse.DomainCheckResult {
		if strings.EqualFold(result.Domain, domain) {
			available = strings.EqualFold(result.Available, "true")
			break
		}
	}

	pricing := staticPrices[tld]
	return &DomainResult{
		Available:     available,
		RegisterPrice: pricing.register,
		RenewPrice:    pricing.renew,
		Provider:      "namecheap",
		Currency:      "USD",
		TLD:           tld,
		Domain:        domain,
	}, nil
}
