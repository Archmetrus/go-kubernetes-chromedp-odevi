package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TotalRunsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "synthetics_runs_total",
			Help: "Total number of synthetic test / Robomotion workflow executions",
		},
		[]string{"test_id", "status"},
	)

	DurationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "synthetics_duration_seconds",
			Help:    "Execution duration of synthetic tests in seconds",
			Buckets: prometheus.ExponentialBuckets(0.5, 2.0, 8),
		},
		[]string{"test_id"},
	)

	VisualDriftGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "synthetics_visual_drift_percentage",
			Help: "Current visual drift percentage compared to baseline",
		},
		[]string{"test_id"},
	)

	SSRFViolationsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "security_ssrf_blocked_total",
			Help: "Total number of SSRF violations blocked by security guard",
		},
		[]string{"host"},
	)
)

func RecordRun(testID, status string, durationSeconds float64, driftPercent *float64) {
	TotalRunsCounter.WithLabelValues(testID, status).Inc()
	DurationHistogram.WithLabelValues(testID).Observe(durationSeconds)
	if driftPercent != nil {
		VisualDriftGauge.WithLabelValues(testID).Set(*driftPercent)
	}
}

func RecordSSRFBlocked(host string) {
	SSRFViolationsCounter.WithLabelValues(host).Inc()
}
