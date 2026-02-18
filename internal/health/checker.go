package health

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezweb/internal/docker"
	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"

	dockertypes "github.com/docker/docker/api/types/container"
)

type Checker struct {
	DB             *sql.DB
	Interval       time.Duration
	Client         *http.Client
	Webhook        *WebhookSender
	AlertThreshold int
	failures       map[int]int
	alertedSites   map[int]bool
	mu             sync.Mutex
}

func NewChecker(db *sql.DB, interval time.Duration, webhookURL string, webhookFormat string, alertThreshold int) *Checker {
	var webhook *WebhookSender
	if webhookURL != "" {
		webhook = NewWebhookSender(webhookURL, webhookFormat)
	}
	if alertThreshold <= 0 {
		alertThreshold = 3
	}
	return &Checker{
		DB:             db,
		Interval:       interval,
		Client:         &http.Client{Timeout: 10 * time.Second},
		Webhook:        webhook,
		AlertThreshold: alertThreshold,
		failures:       make(map[int]int),
		alertedSites:   make(map[int]bool),
	}
}

func (ch *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(ch.Interval)
	defer ticker.Stop()

	ch.checkAll()

	for {
		select {
		case <-ctx.Done():
			log.Println("Health checker stopped")
			return
		case <-ticker.C:
			ch.checkAll()
		}
	}
}

func (ch *Checker) checkAll() {
	// Prune health checks older than 30 days
	ch.DB.Exec("DELETE FROM health_checks WHERE checked_at < datetime('now', '-30 days')")

	sites, err := models.GetAllSites(ch.DB)
	if err != nil {
		log.Printf("Health checker: failed to get sites: %v", err)
		return
	}

	for _, site := range sites {
		if site.Status == "pending" {
			continue
		}
		go ch.checkSite(site)
	}
}

func (ch *Checker) checkSite(site models.Site) {
	hc := &models.HealthCheck{
		SiteID: site.ID,
	}

	// HTTP check
	if site.Domain != "" {
		scheme := "http"
		if site.SSLEnabled {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s", scheme, site.Domain)
		start := time.Now()
		resp, err := ch.Client.Get(url)
		latency := time.Since(start).Milliseconds()

		if err != nil {
			hc.HTTPStatus = 0
			hc.LatencyMs = int(latency)
		} else {
			hc.HTTPStatus = resp.StatusCode
			hc.LatencyMs = int(latency)
			resp.Body.Close()
		}
	}

	// Container status check
	if site.IsLocal {
		ch.checkLocalContainer(site, hc)
	} else if site.ServerID.Valid {
		ch.checkRemoteContainer(site, hc)
	}

	if err := models.CreateHealthCheck(ch.DB, hc); err != nil {
		log.Printf("Health checker: failed to save check for site %d: %v", site.ID, err)
	}

	isDown := hc.HTTPStatus == 0 || hc.HTTPStatus >= 500 || hc.ContainerStatus == "not_found" || hc.ContainerStatus == "exited"

	ch.mu.Lock()
	if isDown {
		ch.failures[site.ID]++
		count := ch.failures[site.ID]
		alerted := ch.alertedSites[site.ID]
		ch.mu.Unlock()

		if count >= ch.AlertThreshold && !alerted && ch.Webhook != nil {
			errMsg := fmt.Sprintf("HTTP: %d, Container: %s", hc.HTTPStatus, hc.ContainerStatus)
			if err := ch.Webhook.SendAlert(site.Domain, count, errMsg); err != nil {
				log.Printf("Webhook alert failed for %s: %v", site.Domain, err)
			} else {
				ch.mu.Lock()
				ch.alertedSites[site.ID] = true
				ch.mu.Unlock()
			}
		}
	} else {
		wasDown := ch.failures[site.ID] > 0
		alerted := ch.alertedSites[site.ID]
		ch.failures[site.ID] = 0
		ch.alertedSites[site.ID] = false
		ch.mu.Unlock()

		if wasDown && alerted && ch.Webhook != nil {
			if err := ch.Webhook.SendRecovery(site.Domain); err != nil {
				log.Printf("Webhook recovery failed for %s: %v", site.Domain, err)
			}
		}
	}
}

func (ch *Checker) checkLocalContainer(site models.Site, hc *models.HealthCheck) {
	cli, err := docker.NewLocalClient()
	if err != nil {
		hc.ContainerStatus = "docker_error"
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containerName := site.ContainerName
	if containerName == "" {
		containerName = strings.ReplaceAll(site.Domain, ".", "-")
	}

	inspect, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		// Try listing containers by compose project label
		if site.ComposePath != "" {
			containers, listErr := cli.ContainerList(ctx, dockertypes.ListOptions{All: true})
			if listErr == nil {
				for _, c := range containers {
					for _, name := range c.Names {
						if strings.Contains(name, containerName) {
							hc.ContainerStatus = c.State
							return
						}
					}
				}
			}
		}
		hc.ContainerStatus = "not_found"
		return
	}

	hc.ContainerStatus = inspect.State.Status
}

func (ch *Checker) checkRemoteContainer(site models.Site, hc *models.HealthCheck) {
	server, err := models.GetServerByID(ch.DB, int(site.ServerID.Int64))
	if err != nil {
		return
	}

	client, err := sshutil.NewClientWithHostKey(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey)
	if err != nil {
		hc.ContainerStatus = "ssh_error"
		return
	}
	defer client.Close()

	output, err := sshutil.RunCommand(client, fmt.Sprintf("docker inspect --format='{{.State.Status}}' %s 2>/dev/null || echo 'not found'", site.ContainerName))
	if err != nil {
		hc.ContainerStatus = "unknown"
		return
	}

	hc.ContainerStatus = strings.TrimSpace(output)
}
