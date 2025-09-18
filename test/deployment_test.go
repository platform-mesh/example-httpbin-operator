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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
)

func TestHttpBinDeploymentOperator(t *testing.T) {
	feat := features.New("HttpBinDeployment Operator").
		Setup(setupOperatorTest).
		Assess("Create HttpBinDeployment resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			// Create HttpBinDeployment resource
			httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httpbin-deployment",
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{},
			}

			if err := k8sClient.Create(ctx, httpbinDeployment); err != nil {
				t.Fatalf("failed to create HttpBinDeployment resource: %v", err)
			}

			// Wait for Deployment to be created and available
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httpbin-deployment",
					Namespace: "default",
				},
			}

			if err := wait.For(conditions.New(cfg.Client().Resources()).DeploymentConditionMatch(deployment, appsv1.DeploymentAvailable, corev1.ConditionTrue),
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for Deployment to be available: %v", err)
			}

			// Get the deployment to verify ownership
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment); err != nil {
				t.Fatalf("failed to get deployment: %v", err)
			}

			// Verify Deployment ownership
			if len(deployment.OwnerReferences) != 1 {
				t.Fatalf("expected 1 owner reference for Deployment, got %d", len(deployment.OwnerReferences))
			}
			owner := deployment.OwnerReferences[0]
			if owner.Name != httpbinDeployment.Name || owner.Kind != "HttpBinDeployment" {
				t.Errorf("incorrect owner reference for Deployment: got name=%s kind=%s, want name=%s kind=HttpBinDeployment",
					owner.Name, owner.Kind, httpbinDeployment.Name)
			}

			// Wait for Service to be created
			service := &corev1.Service{}
			serviceNamespacedName := types.NamespacedName{
				Name:      "test-httpbin-deployment",
				Namespace: "default",
			}

			if err := wait.For(func(ctx context.Context) (done bool, err error) {
				err = k8sClient.Get(ctx, serviceNamespacedName, service)
				return err == nil, nil
			},
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for Service: %v", err)
			}

			// Verify Service ownership
			if len(service.OwnerReferences) != 1 {
				t.Fatalf("expected 1 owner reference for Service, got %d", len(service.OwnerReferences))
			}
			serviceOwner := service.OwnerReferences[0]
			if serviceOwner.Name != httpbinDeployment.Name || serviceOwner.Kind != "HttpBinDeployment" {
				t.Errorf("incorrect owner reference for Service: got name=%s kind=%s, want name=%s kind=HttpBinDeployment",
					serviceOwner.Name, serviceOwner.Kind, httpbinDeployment.Name)
			}

			return ctx
		}).
		Assess("Delete HttpBinDeployment resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			k8sClient := ctx.Value(K8sClientKey).(client.Client)

			// Delete HttpBinDeployment resource
			httpbinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httpbin-deployment",
					Namespace: "default",
				},
			}
			if err := k8sClient.Delete(ctx, httpbinDeployment); err != nil {
				t.Fatalf("failed to delete HttpBinDeployment resource: %v", err)
			}

			// Wait for Deployment to be deleted
			deployment := &appsv1.Deployment{}
			namespacedName := types.NamespacedName{
				Name:      "test-httpbin-deployment",
				Namespace: "default",
			}

			if err := wait.For(func(ctx context.Context) (done bool, err error) {
				err = k8sClient.Get(ctx, namespacedName, deployment)
				return apierrors.IsNotFound(err), nil
			},
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for Deployment deletion: %v", err)
			}

			// Wait for Service to be deleted
			service := &corev1.Service{}
			serviceNamespacedName := types.NamespacedName{
				Name:      "test-httpbin-deployment",
				Namespace: "default",
			}

			if err := wait.For(func(ctx context.Context) (done bool, err error) {
				err = k8sClient.Get(ctx, serviceNamespacedName, service)
				return apierrors.IsNotFound(err), nil
			},
				wait.WithTimeout(DefaultTimeout),
				wait.WithInterval(DefaultInterval)); err != nil {
				t.Fatalf("failed waiting for Service deletion: %v", err)
			}

			return ctx
		}).Feature()

	TestEnv.Test(t, feat)
}
