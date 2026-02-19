package health

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ezweb/internal/docker"
	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"

	dockertypes "github.com/docker/docker/api/types/container"
)

const maxConcurrentChecks = 10

type Checker struct {
	DB             *sql.DB
	Interval       time.Duration
	Client         *http.Client
	Webhook        *WebhookSender
	Email          *EmailSender
	AlertThreshold        int
	HealthRetentionDays   int
	ActivityRetentionDays int
	failures              map[int]int
	alertedSites   map[int]bool
	mu             sync.Mutex
	semaphore      chan struct{}
	running        atomic.Int32
}

func NewChecker(db *sql.DB, interval time.Duration, webhookURL string, webhookFormat string, alertThreshold int, healthRetentionDays int, activityRetentionDays int, email *EmailSender) *Checker {
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
		Email:          email,
		AlertThreshold: alertThreshold,
		HealthRetentionDays:   healthRetentionDays,
		ActivityRetentionDays: activityRetentionDays,
		failures:              make(map[int]int),
		alertedSites:          make(map[int]bool),
		semaphore:             make(chan struct{}, maxConcurrentChecks),
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
	// Skip this cycle if the previous round has not finished yet.
	if !ch.running.CompareAndSwap(0, 1) {
		log.Println("Health checker: previous round still running, skipping cycle")
		return
	}
	defer ch.running.Store(0)

	// Prune health checks older than 30 days.
	ch.DB.Exec(fmt.Sprintf("DELETE FROM health_checks WHERE checked_at < datetime('now', '-%d days')", ch.HealthRetentionDays))
	// Prune activity log entries older than configured retention.
	ch.DB.Exec(fmt.Sprintf("DELETE FROM activity_log WHERE created_at < datetime('now', '-%d days')", ch.ActivityRetentionDays))

	sites, err := models.GetAllSites(ch.DB)
	if err != nil {
		log.Printf("Health checker: failed to get sites: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, site := range sites {
		if site.Status == "pending" {
			continue
		}
		wg.Add(1)
		// Acquire a semaphore slot before launching to cap concurrency.
		ch.semaphore <- struct{}{}
		go func(s models.Site) {
			defer wg.Done()
			defer func() { <-ch.semaphore }()
			ch.checkSite(s)
		}(site)
	}
	wg.Wait()
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

	// SSL certificate expiry check — only performed for SSL-enabled sites with
	// a known domain. The result is stored on the site record and a webhook
	// alert is sent when the cert expires within 14 days.
	if site.SSLEnabled && site.Domain != "" {
		if expiry, certErr := CheckCertExpiry(site.Domain); certErr == nil {
			if updateErr := models.UpdateSiteSSLExpiry(ch.DB, site.ID, expiry); updateErr != nil {
				log.Printf("Health checker: failed to store ssl_expiry for site %d: %v", site.ID, updateErr)
			}
			daysUntilExpiry := int(time.Until(expiry).Hours() / 24)
			if daysUntilExpiry <= 14 && daysUntilExpiry > 0 && ch.Webhook != nil {
				msg := fmt.Sprintf("SSL certificate expires in %d days", daysUntilExpiry)
				if err := ch.Webhook.SendAlert(site.Domain, 0, msg); err != nil {
					log.Printf("Webhook cert-expiry alert failed for %s: %v", site.Domain, err)
				}
			}
		} else {
			log.Printf("Health checker: cert check failed for %s: %v", site.Domain, certErr)
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

	// Align with MCP definition: any 4xx or 5xx response, or a network failure
	// (status 0), is treated as the site being down.
	isDown := hc.HTTPStatus == 0 || hc.HTTPStatus >= 400 || hc.ContainerStatus == "not_found" || hc.ContainerStatus == "exited"

	// Hold the lock across the entire read-modify-decide block so that the
	// failure counter and the alerted flag are always updated atomically.
	// Only the webhook I/O (which can block) is performed outside the lock.
	var shouldAlert bool
	var shouldRecover bool

	ch.mu.Lock()
	if isDown {
		ch.failures[site.ID]++
		count := ch.failures[site.ID]
		alerted := ch.alertedSites[site.ID]
		if count >= ch.AlertThreshold && !alerted {
			// Mark as alerted now, before releasing the lock, so a concurrent
			// goroutine racing on the same site cannot also trigger an alert.
			ch.alertedSites[site.ID] = true
			shouldAlert = true
		}
	} else {
		wasDown := ch.failures[site.ID] > 0
		alerted := ch.alertedSites[site.ID]
		ch.failures[site.ID] = 0
		ch.alertedSites[site.ID] = false
		shouldRecover = wasDown && alerted
	}
	ch.mu.Unlock()

	// Perform webhook I/O outside the lock to avoid holding it during network
	// calls, which could block other goroutines from updating their state.
	if shouldAlert && ch.Webhook != nil {
		errMsg := fmt.Sprintf("HTTP: %d, Container: %s", hc.HTTPStatus, hc.ContainerStatus)
		if err := ch.Webhook.SendAlert(site.Domain, ch.failures[site.ID], errMsg); err != nil {
			log.Printf("Webhook alert failed for %s: %v", site.Domain, err)
			// Roll back the alerted flag so the next cycle can retry.
			ch.mu.Lock()
			ch.alertedSites[site.ID] = false
			ch.mu.Unlock()
		}
	}

	if shouldRecover && ch.Webhook != nil {
		if err := ch.Webhook.SendRecovery(site.Domain); err != nil {
			log.Printf("Webhook recovery failed for %s: %v", site.Domain, err)
		}
	}

	if shouldAlert && ch.Email != nil {
		errMsg := fmt.Sprintf("HTTP: %d, Container: %s", hc.HTTPStatus, hc.ContainerStatus)
		if err := ch.Email.SendAlert(site.Domain, ch.failures[site.ID], errMsg); err != nil {
			log.Printf("Email alert failed for %s: %v", site.Domain, err)
		}
	}

	if shouldRecover && ch.Email != nil {
		if err := ch.Email.SendRecovery(site.Domain); err != nil {
			log.Printf("Email recovery failed for %s: %v", site.Domain, err)
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
