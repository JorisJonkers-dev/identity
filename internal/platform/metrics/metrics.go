// Package metrics serves identity's Prometheus metrics on their own listener, which the edge never
// routes: the Identity Provider's host is public, its metrics are not.
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// countTimeout bounds the database count a scrape waits for.
const countTimeout = 2 * time.Second

// Unassigned names the gauge deploy/alerts raises IdentityUsersWaitingForAGroup on.
const Unassigned = "identity_unassigned_users"

// unassigned reads the count on every scrape, so the alert follows the database, not a counter
// one replica kept.
type unassigned struct {
	desc   *prometheus.Desc
	count  func(context.Context) (int, error)
	logger *slog.Logger
}

func (u unassigned) Describe(ch chan<- *prometheus.Desc) { ch <- u.desc }

func (u unassigned) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), countTimeout)
	defer cancel()
	n, err := u.count(ctx)
	if err != nil {
		u.logger.ErrorContext(ctx, "counting unassigned Users", "error", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(u.desc, prometheus.GaugeValue, float64(n))
}

// Handler serves /metrics: the Unassigned count, and the Go runtime's and the process's own.
func Handler(countUnassigned func(context.Context) (int, error), logger *slog.Logger) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		unassigned{
			desc:  prometheus.NewDesc(Unassigned, "Confirmed Users in no Group, waiting for an Admin to place them (docs/adr/0005).", nil, nil),
			count: countUnassigned, logger: logger,
		},
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError}))
	return mux
}
