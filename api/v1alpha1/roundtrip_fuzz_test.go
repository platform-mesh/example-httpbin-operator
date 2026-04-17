package v1alpha1

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
)

func FuzzHttpBinRoundTrip(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"httpbin.platform-mesh.io/v1alpha1","kind":"HttpBin","metadata":{"name":"test-httpbin","namespace":"default"},"spec":{"region":"eu-west-1"}}`))
	f.Add([]byte(`{"spec":{"region":"us-east-1"},"status":{"url":"https://httpbin.example.com","ready":true}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzRoundTrip(t, data, &HttpBin{}, &HttpBin{})
	})
}

func FuzzHttpBinDeploymentRoundTrip(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"httpbin.platform-mesh.io/v1alpha1","kind":"HttpBinDeployment","metadata":{"name":"test-deployment","namespace":"default"},"spec":{"service":{"name":"httpbin-svc","type":"ClusterIP","port":80},"deployment":{"name":"httpbin","replicas":3}}}`))
	f.Add([]byte(`{"spec":{"service":{"type":"LoadBalancer","port":8080,"annotations":{"service.beta.kubernetes.io/aws-load-balancer-type":"nlb"}},"deployment":{"replicas":1,"labels":{"app":"httpbin"},"annotations":{"prometheus.io/scrape":"true"}}},"status":{"readyReplicas":1,"url":"https://httpbin.example.com","isDeploymentReady":true}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzRoundTrip(t, data, &HttpBinDeployment{}, &HttpBinDeployment{})
	})
}

func fuzzRoundTrip[T any](t *testing.T, data []byte, obj *T, obj2 *T) {
	t.Helper()
	if err := json.Unmarshal(data, obj); err != nil {
		return
	}
	roundtripped, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if err := json.Unmarshal(roundtripped, obj2); err != nil {
		t.Fatalf("failed to unmarshal roundtripped data: %v", err)
	}
	if !equality.Semantic.DeepEqual(obj, obj2) {
		t.Errorf("roundtrip mismatch for %T", obj)
	}
}
