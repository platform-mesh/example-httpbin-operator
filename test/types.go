package test

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// K8sClientKey is the key used to store the Kubernetes client in context
	K8sClientKey ContextKey = "k8sClient"
)
