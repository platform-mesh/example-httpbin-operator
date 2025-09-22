/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestMultipleNodePortDeployments(t *testing.T) {
	feat := features.New("Multiple NodePort Deployments").
		Setup(setupOperatorTest).
		Assess("Create first HttpBinDeployment resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			if err := createHttpBinDeployment(ctx, k8sClient, t, "test-httpbin-deployment-1", 8080); err != nil {
				t.Fatalf("failed to create first HttpBinDeployment: %v", err)
			}

			return ctx
		}).
		Assess("Create second HttpBinDeployment resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			if err := createHttpBinDeployment(ctx, k8sClient, t, "test-httpbin-deployment-2", 9090); err != nil {
				t.Fatalf("failed to create second HttpBinDeployment: %v", err)
			}

			return ctx
		}).
		Assess("Delete HttpBinDeployment resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			// Delete first HttpBinDeployment resource
			if err := deleteHttpBinDeployment(ctx, k8sClient, t, "test-httpbin-deployment-1"); err != nil {
				t.Fatalf("failed to delete first HttpBinDeployment: %v", err)
			}

			// Delete second HttpBinDeployment resource
			if err := deleteHttpBinDeployment(ctx, k8sClient, t, "test-httpbin-deployment-2"); err != nil {
				t.Fatalf("failed to delete second HttpBinDeployment: %v", err)
			}

			t.Log("All resources cleaned up successfully")
			return ctx
		}).Feature()

	TestEnv.Test(t, feat)
}
