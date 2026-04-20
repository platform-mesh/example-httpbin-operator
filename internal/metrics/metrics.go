package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	HttpBinReconciled = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "httpbin_operator_httpbin_reconciled_total",
			Help: "Total number of HttpBin reconcile calls by result.",
		},
		[]string{"result"},
	)
	DeploymentOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "httpbin_operator_deployments_total",
			Help: "Total number of local Deployment operations performed.",
		},
		[]string{"operation"},
	)
	ServiceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "httpbin_operator_services_total",
			Help: "Total number of local Service operations performed.",
		},
		[]string{"operation"},
	)
	DeploymentReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "httpbin_operator_deployment_ready",
			Help: "Whether the HttpBinDeployment is ready (1) or not (0).",
		},
		[]string{"name"},
	)
	ReadyReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "httpbin_operator_ready_replicas",
			Help: "Number of ready replicas for each HttpBinDeployment.",
		},
		[]string{"name"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		HttpBinReconciled,
		DeploymentOperations,
		ServiceOperations,
		DeploymentReady,
		ReadyReplicas,
	)
}
