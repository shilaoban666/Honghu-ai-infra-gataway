package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Registry         *prometheus.Registry
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	FirstToken       *prometheus.HistogramVec
	TokensTotal      *prometheus.CounterVec
	RouteDecisions   *prometheus.CounterVec
	ProviderErrors   *prometheus.CounterVec
	InFlightRequests prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		Registry: registry,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_gateway_requests_total",
			Help: "Total LLM gateway requests.",
		}, []string{"route", "method", "status", "provider"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_gateway_request_duration_seconds",
			Help:    "Gateway request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "provider"}),
		FirstToken: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_gateway_first_token_duration_seconds",
			Help:    "Time to first streamed token in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider"}),
		TokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_gateway_tokens_total",
			Help: "Estimated or reported token usage.",
		}, []string{"provider", "type"}),
		RouteDecisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_gateway_route_decisions_total",
			Help: "Router decisions by provider, task, and reason.",
		}, []string{"provider", "task", "reason"}),
		ProviderErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_gateway_provider_errors_total",
			Help: "Provider errors seen by the gateway.",
		}, []string{"provider"}),
		InFlightRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "llm_gateway_in_flight_requests",
			Help: "Current in-flight gateway requests.",
		}),
	}

	registry.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.FirstToken,
		m.TokensTotal,
		m.RouteDecisions,
		m.ProviderErrors,
		m.InFlightRequests,
	)
	return m
}
