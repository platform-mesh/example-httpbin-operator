package test

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"

	orchestratev1alpha1 "http-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
)

const (
	// OperatorImage is the image name for the operator
	OperatorImage = "controller:dev"
	// OperatorName is the name of the operator
	OperatorName = "example-httpbin-operator"
	// OperatorNs is the namespace where the operator is deployed
	OperatorNs = "example-httpbin-operator-system"
	// DefaultTimeout is the default timeout for operations
	DefaultTimeout = time.Second * 300
	// DefaultInterval is the default polling interval
	DefaultInterval = time.Second * 1
)

var (
	// TestEnv is the global test environment
	TestEnv env.Environment
	// Scheme contains all API types needed for testing
	Scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))
	utilruntime.Must(appsv1.AddToScheme(Scheme))
	utilruntime.Must(orchestratev1alpha1.AddToScheme(Scheme))
}
