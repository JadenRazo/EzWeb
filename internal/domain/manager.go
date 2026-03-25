package domain

import (
	"context"
	"database/sql"
	"log"
	"os"
	"sort"
	"sync"
)

// Manager holds the set of configured domain providers and an SQLite-backed
// result cache.  It fans out search requests to all providers concurrently and
// merges the results, sorted by register price ascending.
type Manager struct {
	providers     []Provider
	realProviders int // count of non-static providers
	cache         *Cache
}

// NewManager inspects environment variables and initialises every provider
// whose API credentials are present.  StaticProvider is always included as a
// last-resort fallback.
func NewManager(db *sql.DB) *Manager {
	m := &Manager{
		cache: NewCache(db),
	}

	// Cloudflare — requires both token and account ID.
	cfToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	cfAccount := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if cfToken != "" && cfAccount != "" {
		m.providers = append(m.providers, NewCloudflareProvider(cfToken, cfAccount))
		m.realProviders++
		log.Println("domain: cloudflare provider enabled")
	}

	// Namecheap — requires API user and key.
	ncUser := os.Getenv("NAMECHEAP_API_USER")
	ncKey := os.Getenv("NAMECHEAP_API_KEY")
	if ncUser != "" && ncKey != "" {
		m.providers = append(m.providers, NewNamecheapProvider(ncUser, ncKey))
		m.realProviders++
		log.Println("domain: namecheap provider enabled")
	}

	// Porkbun — requires API key and secret.
	pbKey := os.Getenv("PORKBUN_API_KEY")
	pbSecret := os.Getenv("PORKBUN_SECRET_KEY")
	if pbKey != "" && pbSecret != "" {
		m.providers = append(m.providers, NewPorkbunProvider(pbKey, pbSecret))
		m.realProviders++
		log.Println("domain: porkbun provider enabled")
	}

	// Static provider is always the final fallback.
	m.providers = append(m.providers, NewStaticProvider())

	return m
}

// HasProviders returns true when at least one real registrar API is configured.
// The static fallback is not counted as a "real" provider.
func (m *Manager) HasProviders() bool {
	return m.realProviders > 0
}

// Search queries all configured providers concurrently for the given domain,
// deduplicates results by provider name, sorts by RegisterPrice ascending, and
// caches each result.  Errors from individual providers are logged but do not
// abort the search — partial results are always returned to the caller.
func (m *Manager) Search(ctx context.Context, domain string) ([]DomainResult, error) {
	type outcome struct {
		result *DomainResult
		err    error
		name   string
	}

	ch := make(chan outcome, len(m.providers))
	var wg sync.WaitGroup

	for _, p := range m.providers {
		wg.Add(1)
		go func(prov Provider) {
			defer wg.Done()

			provName := prov.Name()

			// Check cache first.
			if cached, ok := m.cache.Get(domain, provName); ok {
				ch <- outcome{result: cached, name: provName}
				return
			}

			res, err := prov.CheckAvailability(ctx, domain)
			if err != nil {
				log.Printf("domain: provider %q error for %q: %v", provName, domain, err)
				ch <- outcome{err: err, name: provName}
				return
			}

			// Persist to cache asynchronously — cache miss does not block the
			// response.
			go func() {
				if cacheErr := m.cache.Set(res); cacheErr != nil {
					log.Printf("domain: cache write failed for %q/%q: %v", domain, provName, cacheErr)
				}
			}()

			ch <- outcome{result: res, name: provName}
		}(p)
	}

	// Close channel once all goroutines finish.
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect results, de-duplicate by provider name.
	seen := make(map[string]struct{}, len(m.providers))
	var results []DomainResult

	for o := range ch {
		if o.err != nil {
			continue
		}
		if _, dup := seen[o.name]; dup {
			continue
		}
		seen[o.name] = struct{}{}
		results = append(results, *o.result)
	}

	// Sort by register price ascending; ties broken alphabetically by provider.
	sort.Slice(results, func(i, j int) bool {
		if results[i].RegisterPrice != results[j].RegisterPrice {
			return results[i].RegisterPrice < results[j].RegisterPrice
		}
		return results[i].Provider < results[j].Provider
	})

	return results, nil
}
