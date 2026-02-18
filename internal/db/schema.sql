CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
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
    server_id INTEGER REFERENCES servers(id),
    template_slug TEXT,
    customer_id INTEGER REFERENCES customers(id),
    container_name TEXT,
    port INTEGER,
    status TEXT DEFAULT 'pending',
    ssl_enabled INTEGER DEFAULT 0,
    is_local INTEGER DEFAULT 0,
    compose_path TEXT,
    routing_config TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    site_id INTEGER REFERENCES sites(id),
    amount REAL NOT NULL,
    due_date DATE NOT NULL,
    paid_at DATETIME,
    status TEXT DEFAULT 'pending',
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS health_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id),
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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed templates
INSERT OR IGNORE INTO site_templates (slug, label, description) VALUES
    ('wordpress', 'WordPress', 'Full WordPress CMS with MySQL'),
    ('static', 'Static Site', 'Nginx serving static HTML/CSS/JS'),
    ('nodejs', 'Node.js App', 'Node.js application with PM2'),
    ('ghost', 'Ghost Blog', 'Ghost CMS with MySQL'),
    ('woocommerce', 'WooCommerce', 'WordPress + WooCommerce with MySQL'),
    ('landing', 'Landing Page', 'Simple Nginx landing page'),
    ('react-spa', 'React SPA', 'React single-page app served by Nginx');
