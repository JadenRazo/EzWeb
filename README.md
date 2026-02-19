# EzWeb - Website Management Platform

A self-contained platform for managing client website contracts. Orchestrates Docker containers across multiple VPS machines via SSH, provides ready-to-go templates for popular site types, and tracks customers/payments.

## Features

- **Site Deployment** - Deploy WordPress, Ghost, Node.js, static sites, and more from built-in templates
- **Server Management** - Manage VPS machines via SSH with key-based authentication
- **Customer Tracking** - Full customer CRUD with company and contact info
- **Payment Tracking** - Invoice management with overdue detection and status color-coding
- **Health Monitoring** - Background health checks with HTTP and container status monitoring
- **Domain Management** - Caddy reverse proxy integration with auto-HTTPS
- **Site Logs** - View container logs directly from the dashboard

## Tech Stack

- **Backend:** Go + Fiber v2
- **Templating:** Templ (type-safe Go HTML)
- **Frontend:** HTMX + Alpine.js + Tailwind CSS (CDN)
- **Database:** SQLite (pure Go, no CGO)
- **Auth:** bcrypt + JWT (HttpOnly cookies)
- **Reverse Proxy:** Caddy (admin API)

## Quick Start

### Prerequisites

- Go 1.24+
- [templ](https://templ.guide/) CLI: `go install github.com/a-h/templ/cmd/templ@latest`

### Setup

```bash
# Clone and enter the project
git clone <repo-url> && cd EzWeb

# Copy environment config
cp .env.example .env

# Edit .env with your settings (change JWT_SECRET!)
nano .env

# Build and run
make run
```

The app starts on `http://localhost:3000`. Default login: `admin` / `admin123`.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `APP_PORT` | `3000` | HTTP listen port |
| `JWT_SECRET` | (required) | Secret key for JWT signing |
| `ADMIN_USER` | `admin` | Initial admin username |
| `ADMIN_PASS` | `admin123` | Initial admin password |
| `DB_PATH` | `./ezweb.db` | SQLite database path |
| `CADDY_ADMIN_URL` | `http://localhost:2019` | Caddy admin API endpoint |

## Build

```bash
# Development
make run

# Production binary (static, no CGO)
make prod-build

# The binary is fully self-contained - compose templates are embedded
./ezweb
```

## Site Templates

| Template | Description |
|---|---|
| WordPress | WordPress CMS + MySQL |
| WooCommerce | WordPress + WooCommerce + MySQL |
| Ghost | Ghost CMS + MySQL |
| Static | Nginx serving static files |
| Node.js | Node.js app container |
| Landing Page | Simple Nginx landing page |
| React SPA | Nginx serving a React SPA |

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).

## Architecture

- SSH exec over Docker TCP API for remote container management
- Compose templates embedded in the binary via `go:embed`
- Caddy admin API via SSH tunnel (never exposed publicly)
- Overdue payment detection computed at query time via SQL
- HTMX partial swaps for all mutations (no full page reloads)
- SSH keys stored on filesystem, DB stores path only
