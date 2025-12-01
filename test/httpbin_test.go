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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
)

func TestHttpBinOperator(t *testing.T) {
	feat := features.New("HttpBin Operator").
		Setup(setupOperatorTest).
		Assess("Create HttpBin resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			// Create HttpBin resource
			httpbin := &orchestratev1alpha1.HttpBin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httpbin",
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinSpec{
					Region: "us-east-1",
				},
			}

			if err := k8sClient.Create(ctx, httpbin); err != nil {
				t.Fatalf("failed to create HttpBin resource: %v", err)
			}

			// Wait for HttpBinDeployment to be created
			httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			namespacedName := types.NamespacedName{
				Name:      "test-httpbin",
				Namespace: "default",
			}

			if err := wait.For(func(ctx context.Context) (done bool, err error) {
				err = k8sClient.Get(ctx, namespacedName, httpbinDeployment)
				if err != nil {
					return false, nil // Keep waiting
				}
				return true, nil
			},
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for HttpBinDeployment: %v", err)
			}

			// Verify ownership
			if len(httpbinDeployment.OwnerReferences) != 1 {
				t.Fatalf("expected 1 owner reference, got %d", len(httpbinDeployment.OwnerReferences))
			}
			owner := httpbinDeployment.OwnerReferences[0]
			if owner.Name != httpbin.Name {
				t.Errorf("owner reference name = %s; want %s", owner.Name, httpbin.Name)
			}
			if owner.Kind != "HttpBin" {
				t.Errorf("owner reference kind = %s; want HttpBin", owner.Kind)
			}
			if !*owner.Controller {
				t.Error("owner reference Controller field should be true")
			}
			if !*owner.BlockOwnerDeletion {
				t.Error("owner reference BlockOwnerDeletion field should be true")
			}

			return ctx
		}).
		Assess("Delete HttpBin resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			// Delete HttpBin resource
			httpbin := &orchestratev1alpha1.HttpBin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httpbin",
					Namespace: "default",
				},
			}
			if err := k8sClient.Delete(ctx, httpbin); err != nil {
				t.Fatalf("failed to delete HttpBin resource: %v", err)
			}

			// Wait for HttpBinDeployment to be deleted
			httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			namespacedName := types.NamespacedName{
				Name:      "test-httpbin",
				Namespace: "default",
			}

			if err := wait.For(func(ctx context.Context) (done bool, err error) {
				err = k8sClient.Get(ctx, namespacedName, httpbinDeployment)
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, nil // Keep waiting
			},
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for HttpBinDeployment deletion: %v", err)
			}

			return ctx
		}).Feature()

	TestEnv.Test(t, feat)
}
