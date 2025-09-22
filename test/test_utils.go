package test

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
)

// setupOperatorTest creates a Kubernetes client and waits for the operator deployment to be ready
func setupOperatorTest(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	// Create client with scheme
	restConfig := cfg.Client().RESTConfig()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: Scheme})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Wait for operator deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OperatorName + "-controller-manager",
			Namespace: OperatorNs,
		},
	}

	if err := wait.For(conditions.New(cfg.Client().Resources()).DeploymentConditionMatch(deployment, appsv1.DeploymentAvailable, corev1.ConditionTrue),
		wait.WithTimeout(DefaultTimeout),
		wait.WithInterval(DefaultInterval)); err != nil {
		t.Fatalf("failed waiting for operator deployment: %v", err)
	}

	return context.WithValue(ctx, K8sClientKey, k8sClient)
}

// waitForPodReady waits for a pod with the given labels to be ready
func waitForPodReady(ctx context.Context, k8sClient client.Client, t *testing.T, namespace string, labels map[string]string, timeout time.Duration, interval time.Duration) error {
	return wait.For(func(waitCtx context.Context) (done bool, err error) {
		podList := &corev1.PodList{}
		if err := k8sClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
			t.Logf("Error listing pods: %v", err)
			return false, nil
		}

		if len(podList.Items) == 0 {
			t.Log("No pods found yet")
			return false, nil
		}

		pod := podList.Items[0]
		t.Logf("Pod %s status:", pod.Name)
		t.Logf("- Phase: %s", pod.Status.Phase)
		for _, cond := range pod.Status.Conditions {
			t.Logf("- Condition %s: %s (Reason: %s, Message: %s)",
				cond.Type, cond.Status, cond.Reason, cond.Message)
		}

		if pod.Status.Phase == corev1.PodRunning {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}

		return false, nil
	}, wait.WithTimeout(timeout), wait.WithInterval(interval))
}

// createHttpBinDeployment creates a new HttpBinDeployment resource
func createHttpBinDeployment(ctx context.Context, k8sClient client.Client, t *testing.T, name string, port int32) error {
	replicas := int32(1)
	httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
			Service: orchestratev1alpha1.ServiceConfig{
				Type: "NodePort",
				Port: port,
			},
			Deployment: orchestratev1alpha1.DeploymentConfig{
				Replicas: replicas,
			},
		},
	}

	t.Logf("Creating HttpBinDeployment resource %s...", name)
	if err := k8sClient.Create(ctx, httpbinDeployment); err != nil {
		return err
	}

	// Wait for pod to be ready
	labels := map[string]string{
		"app":        "httpbin",
		"httpbin_cr": name,
	}

	t.Logf("Waiting for pod %s to be ready...", name)
	if err := waitForPodReady(ctx, k8sClient, t, "default", labels, DefaultTimeout, DefaultInterval); err != nil {
		return err
	}
	t.Logf("Pod %s is ready", name)

	// Wait for Service to be created and verify it
	service := &corev1.Service{}
	serviceName := types.NamespacedName{
		Name:      name,
		Namespace: "default",
	}

	t.Logf("Waiting for Service %s to be created...", name)
	if err := wait.For(func(ctx context.Context) (done bool, err error) {
		err = k8sClient.Get(ctx, serviceName, service)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Service %s not found yet, waiting...", name)
				return false, nil
			}
			return false, err
		}
		t.Logf("Service %s created with type %s and port %d", name, service.Spec.Type, service.Spec.Ports[0].Port)
		return true, nil
	},
		wait.WithTimeout(DefaultTimeout),
		wait.WithInterval(DefaultInterval)); err != nil {
		return err
	}

	// Verify Service spec
	if service.Spec.Type != corev1.ServiceTypeNodePort {
		t.Errorf("service type = %v; want NodePort", service.Spec.Type)
	}

	if len(service.Spec.Ports) != 1 {
		t.Fatalf("expected 1 service port, got %d", len(service.Spec.Ports))
	}

	if service.Spec.Ports[0].Port != port {
		t.Errorf("service port = %d; want %d", service.Spec.Ports[0].Port, port)
	}

	t.Logf("Service %s verified successfully", name)
	return nil
}

// deleteHttpBinDeployment deletes a HttpBinDeployment resource and waits for its pod to be deleted
func deleteHttpBinDeployment(ctx context.Context, k8sClient client.Client, t *testing.T, name string) error {
	httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	t.Logf("Deleting HttpBinDeployment resource %s...", name)
	if err := k8sClient.Delete(ctx, httpbinDeployment); err != nil {
		return err
	}

	// Wait for pod to be deleted
	labels := map[string]string{
		"app":        "httpbin",
		"httpbin_cr": name,
	}

	t.Logf("Waiting for pod %s to be deleted...", name)
	if err := wait.For(func(ctx context.Context) (done bool, err error) {
		podList := &corev1.PodList{}
		if err := k8sClient.List(ctx, podList, client.InNamespace("default"), client.MatchingLabels(labels)); err != nil {
			return false, err
		}
		return len(podList.Items) == 0, nil
	},
		wait.WithTimeout(DefaultTimeout),
		wait.WithInterval(DefaultInterval)); err != nil {
		return err
	}

	return nil
}
