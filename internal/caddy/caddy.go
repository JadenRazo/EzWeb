package caddy

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"ezweb/internal/models"
)

// sanitizeDomain strips characters that could break or inject blocks into a
// Caddyfile: newlines, backticks, and curly braces.
func sanitizeDomain(domain string) string {
	replacer := strings.NewReplacer(
		"\n", "",
		"\r", "",
		"`", "",
		"{", "",
		"}", "",
	)
	return strings.TrimSpace(replacer.Replace(domain))
}

// validateExtraDirective returns an error if the directive contains a newline
// or a bare opening brace, which could be used to inject arbitrary Caddyfile
// blocks.
func validateExtraDirective(d string) error {
	if strings.ContainsAny(d, "\n\r") {
		return fmt.Errorf("extra directive must not contain newlines: %q", d)
	}
	// Reject lines that open a block — a '{' appearing anywhere in the
	// directive string is enough to break the enclosing site block.
	if strings.Contains(d, "{") {
		return fmt.Errorf("extra directive must not contain '{': %q", d)
	}
	return nil
}

type Manager struct {
	CaddyfilePath string
	AcmeEmail     string
	mu            sync.Mutex
}

func NewManager(caddyfilePath string, acmeEmail string) *Manager {
	if caddyfilePath == "" {
		caddyfilePath = "/etc/caddy/Caddyfile"
	}
	return &Manager{CaddyfilePath: caddyfilePath, AcmeEmail: acmeEmail}
}

// GenerateCaddyfile builds a complete Caddyfile from all managed sites.
func (m *Manager) GenerateCaddyfile(sites []models.Site) (string, error) {
	var b strings.Builder

	b.WriteString("{\n")
	if m.AcmeEmail != "" {
		b.WriteString(fmt.Sprintf("\temail %s\n", m.AcmeEmail))
	}
	b.WriteString("}\n\n")

	for _, site := range sites {
		if site.Domain == "" || site.Status == "pending" {
			continue
		}

		// Strip characters that could break or inject into the Caddyfile.
		site.Domain = sanitizeDomain(site.Domain)
		if site.Domain == "" {
			continue
		}

		rc := site.RoutingConfig

		// Sanitize redirect domains and validate extra directives up front so
		// that a bad value causes the whole reload to fail rather than writing
		// a corrupted Caddyfile.
		if rc != nil {
			sanitized := make([]string, 0, len(rc.RedirectDomains))
			for _, rd := range rc.RedirectDomains {
				if s := sanitizeDomain(rd); s != "" {
					sanitized = append(sanitized, s)
				}
			}
			rc.RedirectDomains = sanitized

			for _, d := range rc.ExtraDirectives {
				if err := validateExtraDirective(d); err != nil {
					return "", fmt.Errorf("site %q: %w", site.Domain, err)
				}
			}
		}

		// Redirect blocks (e.g. www → non-www)
		if rc != nil {
			for _, rd := range rc.RedirectDomains {
				writeRedirectBlock(&b, rd, primaryDomain(site.Domain))
			}
		}

		// Main site block
		if rc != nil && len(rc.Rules) > 0 {
			writeComplexSite(&b, site)
		} else if site.Port > 0 {
			writeSimpleSite(&b, site)
		}
	}

	return b.String(), nil
}

func primaryDomain(domain string) string {
	parts := strings.SplitN(domain, ",", 2)
	return strings.TrimSpace(parts[0])
}

// siteAddress builds the address line from a potentially comma-separated domain field.
func siteAddress(domain string, httpOnly bool) string {
	parts := strings.Split(domain, ",")
	var addrs []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if httpOnly {
			addrs = append(addrs, "http://"+p)
		} else {
			addrs = append(addrs, p)
		}
	}
	return strings.Join(addrs, ", ")
}

func writeRedirectBlock(b *strings.Builder, from, to string) {
	b.WriteString(fmt.Sprintf("%s {\n", from))
	b.WriteString(fmt.Sprintf("\tredir https://%s{uri} permanent\n", to))
	b.WriteString("}\n\n")
}

func writeSimpleSite(b *strings.Builder, site models.Site) {
	httpOnly := site.RoutingConfig != nil && site.RoutingConfig.HTTPOnly
	b.WriteString(fmt.Sprintf("%s {\n", siteAddress(site.Domain, httpOnly)))
	writeTLSDirective(b, site.RoutingConfig)
	b.WriteString(fmt.Sprintf("\treverse_proxy localhost:%d\n", site.Port))
	b.WriteString("}\n\n")
}

func writeComplexSite(b *strings.Builder, site models.Site) {
	rc := site.RoutingConfig
	b.WriteString(fmt.Sprintf("%s {\n", siteAddress(site.Domain, rc.HTTPOnly)))
	writeTLSDirective(b, rc)

	for _, d := range rc.ExtraDirectives {
		b.WriteString("\t" + d + "\n")
	}

	// Sort: longest path prefix first, catch-all (empty) last
	rules := make([]models.RoutingRule, len(rc.Rules))
	copy(rules, rc.Rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].PathPrefix == "" {
			return false
		}
		if rules[j].PathPrefix == "" {
			return true
		}
		return len(rules[i].PathPrefix) > len(rules[j].PathPrefix)
	})

	for i, rule := range rules {
		if rule.PathPrefix != "" {
			writePathBlock(b, rule, i)
		} else {
			writeCatchAllBlock(b, rule)
		}
	}

	b.WriteString("}\n\n")
}

func writeTLSDirective(b *strings.Builder, rc *models.RoutingConfig) {
	if rc == nil {
		return
	}
	if rc.TLSCertPath != "" && rc.TLSKeyPath != "" {
		b.WriteString(fmt.Sprintf("\ttls %s %s\n", rc.TLSCertPath, rc.TLSKeyPath))
	}
}

func writePathBlock(b *strings.Builder, rule models.RoutingRule, index int) {
	// Build the path matcher — ensure it ends with * for prefix matching
	pathMatcher := rule.PathPrefix
	if !strings.HasSuffix(pathMatcher, "*") {
		pathMatcher = strings.TrimSuffix(pathMatcher, "/") + "/*"
	}

	b.WriteString(fmt.Sprintf("\thandle %s {\n", pathMatcher))

	// CORS preflight inside this handle block
	if rule.CORS != nil {
		writeCORSPreflight(b, rule.CORS, index)
	}

	// Path manipulation
	if rule.RewritePath != "" {
		b.WriteString(fmt.Sprintf("\t\trewrite * %s\n", rule.RewritePath))
	} else if rule.StripPrefix != "" {
		b.WriteString(fmt.Sprintf("\t\turi strip_prefix %s\n", rule.StripPrefix))
	}

	// Response headers (including CORS on actual responses)
	if rule.CORS != nil {
		writeCORSResponseHeaders(b, rule.CORS)
	}
	writeResponseHeaders(b, rule.Headers)

	// Reverse proxy
	writeReverseProxy(b, rule)

	b.WriteString("\t}\n\n")
}

func writeCatchAllBlock(b *strings.Builder, rule models.RoutingRule) {
	b.WriteString("\thandle {\n")

	if rule.CORS != nil {
		writeCORSPreflight(b, rule.CORS, 99)
		writeCORSResponseHeaders(b, rule.CORS)
	}

	writeResponseHeaders(b, rule.Headers)
	writeReverseProxy(b, rule)

	b.WriteString("\t}\n\n")
}

func writeCORSPreflight(b *strings.Builder, cors *models.CORSConfig, index int) {
	name := fmt.Sprintf("preflight_%d", index)
	b.WriteString(fmt.Sprintf("\t\t@%s method OPTIONS\n", name))
	b.WriteString(fmt.Sprintf("\t\thandle @%s {\n", name))
	b.WriteString("\t\t\theader Access-Control-Allow-Origin \"{http.request.header.Origin}\"\n")
	if cors.Methods != "" {
		b.WriteString(fmt.Sprintf("\t\t\theader Access-Control-Allow-Methods \"%s\"\n", cors.Methods))
	}
	if cors.Headers != "" {
		b.WriteString(fmt.Sprintf("\t\t\theader Access-Control-Allow-Headers \"%s\"\n", cors.Headers))
	}
	if cors.Credentials {
		b.WriteString("\t\t\theader Access-Control-Allow-Credentials \"true\"\n")
	}
	if cors.MaxAge > 0 {
		b.WriteString(fmt.Sprintf("\t\t\theader Access-Control-Max-Age \"%d\"\n", cors.MaxAge))
	}
	b.WriteString("\t\t\trespond 204\n")
	b.WriteString("\t\t}\n")
}

func writeCORSResponseHeaders(b *strings.Builder, cors *models.CORSConfig) {
	b.WriteString("\t\theader Access-Control-Allow-Origin \"{http.request.header.Origin}\"\n")
	if cors.Methods != "" {
		b.WriteString(fmt.Sprintf("\t\theader Access-Control-Allow-Methods \"%s\"\n", cors.Methods))
	}
	if cors.Headers != "" {
		b.WriteString(fmt.Sprintf("\t\theader Access-Control-Allow-Headers \"%s\"\n", cors.Headers))
	}
	if cors.Credentials {
		b.WriteString("\t\theader Access-Control-Allow-Credentials \"true\"\n")
	}
}

func writeResponseHeaders(b *strings.Builder, headers map[string]string) {
	if len(headers) == 0 {
		return
	}
	for k, v := range headers {
		b.WriteString(fmt.Sprintf("\t\theader %s \"%s\"\n", k, v))
	}
}

func writeReverseProxy(b *strings.Builder, rule models.RoutingRule) {
	b.WriteString(fmt.Sprintf("\t\treverse_proxy %s", rule.Upstream))
	if rule.WebSocket {
		b.WriteString(" {\n")
		b.WriteString("\t\t\tflush_interval -1\n")
		b.WriteString("\t\t\theader_up X-Real-IP {remote_host}\n")
		b.WriteString("\t\t}")
	}
	b.WriteString("\n")
}

// Reload regenerates the Caddyfile from all sites and reloads Caddy.
func (m *Manager) Reload(sites []models.Site) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	content, err := m.GenerateCaddyfile(sites)
	if err != nil {
		return fmt.Errorf("failed to generate Caddyfile: %w", err)
	}

	tmpPath := m.CaddyfilePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write Caddyfile: %w", err)
	}

	out, err := exec.Command("caddy", "validate", "--config", tmpPath).CombinedOutput()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("Caddyfile validation failed: %w\n%s", err, string(out))
	}

	if err := os.Rename(tmpPath, m.CaddyfilePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move Caddyfile into place: %w", err)
	}

	out, err = exec.Command("caddy", "reload", "--config", m.CaddyfilePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Caddy reload failed: %w\n%s", err, string(out))
	}

	return nil
}

func (m *Manager) AddSite(db *sql.DB, site models.Site) error {
	sites, err := models.GetAllSites(db)
	if err != nil {
		return fmt.Errorf("failed to get sites for Caddy reload: %w", err)
	}
	return m.Reload(sites)
}

func (m *Manager) RemoveSite(db *sql.DB, domain string) error {
	sites, err := models.GetAllSites(db)
	if err != nil {
		return fmt.Errorf("failed to get sites for Caddy reload: %w", err)
	}
	var filtered []models.Site
	for _, s := range sites {
		if s.Domain != domain {
			filtered = append(filtered, s)
		}
	}
	return m.Reload(filtered)
}
