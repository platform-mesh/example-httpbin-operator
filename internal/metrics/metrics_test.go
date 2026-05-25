package metrics

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsManual(t *testing.T) {
	// Simulate HttpBin reconcile outcomes
	HttpBinReconciled.WithLabelValues("success").Inc()
	HttpBinReconciled.WithLabelValues("success").Inc()
	HttpBinReconciled.WithLabelValues("created").Inc()
	HttpBinReconciled.WithLabelValues("error").Inc()

	// Simulate Deployment operations
	DeploymentOperations.WithLabelValues("created").Inc()
	DeploymentOperations.WithLabelValues("created").Inc()
	DeploymentOperations.WithLabelValues("updated").Inc()
	DeploymentOperations.WithLabelValues("deleted").Inc()

	// Simulate Service operations
	ServiceOperations.WithLabelValues("created").Inc()
	ServiceOperations.WithLabelValues("updated").Inc()

	// Simulate ready state for two instances
	DeploymentReady.WithLabelValues("httpbin-a").Set(1)
	DeploymentReady.WithLabelValues("httpbin-b").Set(0)
	ReadyReplicas.WithLabelValues("httpbin-a").Set(2)
	ReadyReplicas.WithLabelValues("httpbin-b").Set(0)

	fmt.Println("\n--- httpbin_operator_httpbin_reconciled_total ---")
	fmt.Printf("  success: %.0f\n", testutil.ToFloat64(HttpBinReconciled.WithLabelValues("success")))
	fmt.Printf("  created: %.0f\n", testutil.ToFloat64(HttpBinReconciled.WithLabelValues("created")))
	fmt.Printf("  error:   %.0f\n", testutil.ToFloat64(HttpBinReconciled.WithLabelValues("error")))

	fmt.Println("\n--- httpbin_operator_deployments_total ---")
	fmt.Printf("  created: %.0f\n", testutil.ToFloat64(DeploymentOperations.WithLabelValues("created")))
	fmt.Printf("  updated: %.0f\n", testutil.ToFloat64(DeploymentOperations.WithLabelValues("updated")))
	fmt.Printf("  deleted: %.0f\n", testutil.ToFloat64(DeploymentOperations.WithLabelValues("deleted")))

	fmt.Println("\n--- httpbin_operator_services_total ---")
	fmt.Printf("  created: %.0f\n", testutil.ToFloat64(ServiceOperations.WithLabelValues("created")))
	fmt.Printf("  updated: %.0f\n", testutil.ToFloat64(ServiceOperations.WithLabelValues("updated")))

	fmt.Println("\n--- httpbin_operator_deployment_ready ---")
	fmt.Printf("  httpbin-a: %.0f\n", testutil.ToFloat64(DeploymentReady.WithLabelValues("httpbin-a")))
	fmt.Printf("  httpbin-b: %.0f\n", testutil.ToFloat64(DeploymentReady.WithLabelValues("httpbin-b")))

	fmt.Println("\n--- httpbin_operator_ready_replicas ---")
	fmt.Printf("  httpbin-a: %.0f\n", testutil.ToFloat64(ReadyReplicas.WithLabelValues("httpbin-a")))
	fmt.Printf("  httpbin-b: %.0f\n", testutil.ToFloat64(ReadyReplicas.WithLabelValues("httpbin-b")))
}
