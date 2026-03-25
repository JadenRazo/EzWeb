package metrics

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Collector struct {
	RequestCount    atomic.Int64
	ErrorCount      atomic.Int64
	RequestDuration atomic.Int64 // nanoseconds total
	ActiveRequests  atomic.Int64
	startTime       time.Time
}

var Default = &Collector{startTime: time.Now()}

func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		Default.ActiveRequests.Add(1)
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		Default.ActiveRequests.Add(-1)
		Default.RequestCount.Add(1)
		Default.RequestDuration.Add(duration.Nanoseconds())

		if c.Response().StatusCode() >= 500 {
			Default.ErrorCount.Add(1)
		}

		return err
	}
}

func Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uptime := time.Since(Default.startTime).Seconds()
		totalRequests := Default.RequestCount.Load()
		totalErrors := Default.ErrorCount.Load()
		activeReqs := Default.ActiveRequests.Load()
		totalDuration := Default.RequestDuration.Load()

		var avgDuration float64
		if totalRequests > 0 {
			avgDuration = float64(totalDuration) / float64(totalRequests) / 1e6 // milliseconds
		}

		c.Set("Content-Type", "text/plain; version=0.0.4")

		body := fmt.Sprintf(`# HELP ezweb_uptime_seconds Time since server start
# TYPE ezweb_uptime_seconds gauge
ezweb_uptime_seconds %.2f

# HELP ezweb_http_requests_total Total HTTP requests
# TYPE ezweb_http_requests_total counter
ezweb_http_requests_total %d

# HELP ezweb_http_errors_total Total HTTP 5xx errors
# TYPE ezweb_http_errors_total counter
ezweb_http_errors_total %d

# HELP ezweb_http_active_requests Current active requests
# TYPE ezweb_http_active_requests gauge
ezweb_http_active_requests %d

# HELP ezweb_http_request_duration_avg_ms Average request duration in milliseconds
# TYPE ezweb_http_request_duration_avg_ms gauge
ezweb_http_request_duration_avg_ms %.2f
`, uptime, totalRequests, totalErrors, activeReqs, avgDuration)

		return c.SendString(body)
	}
}
