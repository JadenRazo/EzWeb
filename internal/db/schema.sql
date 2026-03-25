CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    ssh_port INTEGER DEFAULT 22,
    ssh_user TEXT DEFAULT 'root',
    ssh_key_path TEXT NOT NULL,
    status TEXT DEFAULT 'unknown',
    ssh_host_key TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    company TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL UNIQUE,
    server_id INTEGER REFERENCES servers(id) ON DELETE SET NULL,
    template_slug TEXT,
    customer_id INTEGER REFERENCES customers(id) ON DELETE SET NULL,
    container_name TEXT,
    port INTEGER,
    status TEXT DEFAULT 'pending',
    ssl_enabled INTEGER DEFAULT 0,
    is_local INTEGER DEFAULT 0,
    compose_path TEXT,
    routing_config TEXT,
    ssl_expiry DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    site_id INTEGER REFERENCES sites(id),
    amount REAL NOT NULL,
    due_date DATE NOT NULL,
    paid_at DATETIME,
    status TEXT DEFAULT 'pending',
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_env_vars (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, key)
);

CREATE INDEX IF NOT EXISTS idx_site_env_vars_site_id ON site_env_vars(site_id);

CREATE TABLE IF NOT EXISTS health_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    http_status INTEGER,
    latency_ms INTEGER,
    container_status TEXT,
    checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id INTEGER,
    action TEXT NOT NULL,
    details TEXT,
    ip_address TEXT,
    user_agent TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_health_checks_site_id ON health_checks(site_id);
CREATE INDEX IF NOT EXISTS idx_health_checks_checked_at ON health_checks(checked_at);
CREATE INDEX IF NOT EXISTS idx_sites_server_id ON sites(server_id);
CREATE INDEX IF NOT EXISTS idx_sites_customer_id ON sites(customer_id);
CREATE INDEX IF NOT EXISTS idx_sites_status ON sites(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sites_port_unique ON sites(port) WHERE port > 0;
CREATE INDEX IF NOT EXISTS idx_payments_customer_id ON payments(customer_id);
CREATE INDEX IF NOT EXISTS idx_payments_due_date ON payments(due_date);
CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti TEXT PRIMARY KEY,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_revoked_tokens_expires ON revoked_tokens(expires_at);

-- Add ip_address and user_agent to activity_log if upgrading from an older schema.
-- SQLite does not support ADD COLUMN IF NOT EXISTS, so we ignore the error
-- if the column already exists (handled by the Go migration code).

CREATE INDEX IF NOT EXISTS idx_activity_log_created_at ON activity_log(created_at);
CREATE INDEX IF NOT EXISTS idx_activity_log_entity ON activity_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_activity_log_entity_time ON activity_log(entity_type, entity_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sites_domain ON sites(domain);

CREATE TABLE IF NOT EXISTS totp_used_codes (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, code)
);
CREATE INDEX IF NOT EXISTS idx_totp_used_codes_used_at ON totp_used_codes(used_at);

-- Business settings (key/value store for branding, PDF config, portal)
CREATE TABLE IF NOT EXISTS business_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed default business settings
INSERT OR IGNORE INTO business_settings (key, value) VALUES
    ('business_name', ''),
    ('tagline', ''),
    ('email', ''),
    ('phone', ''),
    ('address', ''),
    ('logo_path', ''),
    ('website_url', ''),
    ('tax_rate', '0'),
    ('default_currency', 'USD'),
    ('quote_validity_days', '30'),
    ('terms_text', '');

-- Pricing tiers per template type
CREATE TABLE IF NOT EXISTS pricing_tiers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_slug TEXT NOT NULL,
    label TEXT NOT NULL,
    setup_fee REAL NOT NULL DEFAULT 0,
    monthly_price REAL NOT NULL DEFAULT 0,
    yearly_price REAL NOT NULL DEFAULT 0,
    description TEXT,
    is_active INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pricing_tiers_slug ON pricing_tiers(template_slug);

-- Seed default pricing tiers
INSERT OR IGNORE INTO pricing_tiers (template_slug, label, setup_fee, monthly_price, yearly_price, description) VALUES
    ('wordpress', 'WordPress Site', 500, 49.99, 499, 'Full WordPress CMS with custom theme'),
    ('static', 'Static Website', 300, 19.99, 199, 'Clean, fast static website'),
    ('nodejs', 'Node.js Application', 800, 79.99, 799, 'Custom Node.js web application'),
    ('ghost', 'Ghost Blog', 400, 39.99, 399, 'Professional Ghost-powered blog'),
    ('woocommerce', 'WooCommerce Store', 1000, 99.99, 999, 'Full e-commerce store with WooCommerce'),
    ('landing', 'Landing Page', 200, 14.99, 149, 'High-converting landing page'),
    ('react-spa', 'React Application', 700, 59.99, 599, 'Modern React single-page application');

-- Optional add-ons for quotes
CREATE TABLE IF NOT EXISTS addons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    price REAL NOT NULL DEFAULT 0,
    price_type TEXT NOT NULL DEFAULT 'one_time',
    is_active INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed default addons
INSERT OR IGNORE INTO addons (name, description, price, price_type) VALUES
    ('Monthly Maintenance', 'Bug fixes, updates, and minor changes', 99.99, 'monthly'),
    ('Email Setup', 'Professional email configuration', 50, 'one_time'),
    ('SEO Optimization', 'On-page SEO and meta tag setup', 200, 'one_time'),
    ('SSL Certificate', 'Premium SSL certificate setup', 0, 'one_time'),
    ('Analytics Setup', 'Google Analytics or Plausible integration', 75, 'one_time'),
    ('Content Migration', 'Migrate content from existing site', 150, 'one_time'),
    ('Logo Design', 'Professional logo design package', 300, 'one_time'),
    ('Social Media Integration', 'Connect social feeds and sharing', 100, 'one_time');

-- Quotes / Proposals
CREATE TABLE IF NOT EXISTS quotes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    customer_id INTEGER REFERENCES customers(id) ON DELETE SET NULL,
    customer_name TEXT NOT NULL,
    customer_email TEXT,
    customer_phone TEXT,
    customer_company TEXT,
    template_slug TEXT,
    domain_name TEXT,
    domain_price REAL DEFAULT 0,
    domain_registrar TEXT,
    setup_fee REAL NOT NULL DEFAULT 0,
    monthly_price REAL NOT NULL DEFAULT 0,
    yearly_price REAL NOT NULL DEFAULT 0,
    billing_cycle TEXT NOT NULL DEFAULT 'monthly',
    discount_percent REAL DEFAULT 0,
    tax_rate REAL DEFAULT 0,
    subtotal REAL NOT NULL DEFAULT 0,
    tax_amount REAL NOT NULL DEFAULT 0,
    total REAL NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft',
    notes TEXT,
    valid_until DATE,
    sent_at DATETIME,
    accepted_at DATETIME,
    rejected_at DATETIME,
    converted_site_id INTEGER REFERENCES sites(id) ON DELETE SET NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_quotes_public_id ON quotes(public_id);
CREATE INDEX IF NOT EXISTS idx_quotes_status ON quotes(status);
CREATE INDEX IF NOT EXISTS idx_quotes_customer_id ON quotes(customer_id);

-- Join table for addons on a quote
CREATE TABLE IF NOT EXISTS quote_addons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    quote_id INTEGER NOT NULL REFERENCES quotes(id) ON DELETE CASCADE,
    addon_id INTEGER NOT NULL REFERENCES addons(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL DEFAULT 1,
    price REAL NOT NULL DEFAULT 0,
    price_type TEXT NOT NULL DEFAULT 'one_time'
);

CREATE INDEX IF NOT EXISTS idx_quote_addons_quote ON quote_addons(quote_id);

-- Domain price cache for registrar lookups
CREATE TABLE IF NOT EXISTS domain_price_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL,
    tld TEXT NOT NULL,
    provider TEXT NOT NULL,
    available INTEGER DEFAULT 0,
    register_price REAL DEFAULT 0,
    renew_price REAL DEFAULT 0,
    cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_domain_cache_domain ON domain_price_cache(domain, provider);
CREATE INDEX IF NOT EXISTS idx_domain_cache_time ON domain_price_cache(cached_at);

-- Quote requests from client portal
CREATE TABLE IF NOT EXISTS quote_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT,
    company TEXT,
    project_type TEXT,
    description TEXT,
    budget_range TEXT,
    status TEXT NOT NULL DEFAULT 'new',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_quote_requests_status ON quote_requests(status);

-- Client magic link auth tokens
CREATE TABLE IF NOT EXISTS client_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_client_tokens_hash ON client_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_client_tokens_expires ON client_tokens(expires_at);

-- Portfolio items for client portal
CREATE TABLE IF NOT EXISTS portfolio_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT,
    url TEXT,
    screenshot_path TEXT,
    template_slug TEXT,
    display_order INTEGER DEFAULT 0,
    is_visible INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Subscriptions for recurring billing
CREATE TABLE IF NOT EXISTS subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    site_id INTEGER REFERENCES sites(id) ON DELETE SET NULL,
    amount REAL NOT NULL,
    billing_cycle TEXT NOT NULL DEFAULT 'monthly',
    next_due_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    auto_generate_invoice INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_customer ON subscriptions(customer_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_due ON subscriptions(next_due_date);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);

-- Seed templates
INSERT OR IGNORE INTO site_templates (slug, label, description) VALUES
    ('wordpress', 'WordPress', 'Full WordPress CMS with MySQL'),
    ('static', 'Static Site', 'Nginx serving static HTML/CSS/JS'),
    ('nodejs', 'Node.js App', 'Node.js application with PM2'),
    ('ghost', 'Ghost Blog', 'Ghost CMS with MySQL'),
    ('woocommerce', 'WooCommerce', 'WordPress + WooCommerce with MySQL'),
    ('landing', 'Landing Page', 'Simple Nginx landing page'),
    ('react-spa', 'React SPA', 'React single-page app served by Nginx');
