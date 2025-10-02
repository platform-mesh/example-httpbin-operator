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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

func setupOperator(ctx context.Context, cfg *envconf.Config, operatorImage string) error {
	// Get project root directory
	projectRoot := filepath.Join("..", "")

	// Generate manifests and build operator image
	cmd := exec.Command("make", "-C", projectRoot, "manifests", "generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Build operator image
	cmd = exec.Command("make", "-C", projectRoot, "docker-build", "IMG="+operatorImage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Create client with scheme
	restConfig := cfg.Client().RESTConfig()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: Scheme})
	if err != nil {
		return err
	}

	// Install CRDs
	cmd = exec.Command("make", "-C", projectRoot, "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Create operator namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: OperatorNs,
		},
	}
	err = k8sClient.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// Deploy operator
	cmd = exec.Command("make", "-C", projectRoot, "deploy", "IMG="+operatorImage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func TestMain(m *testing.M) {
	// Create test environment
	TestEnv = env.NewWithConfig(envconf.New())
	operatorImage := getOperatorImage()
	setupCluster := os.Getenv("SETUP_CLUSTER")

	// Setup
	if setupCluster == "true" {
		TestEnv.Setup(
			envfuncs.CreateCluster(kind.NewProvider(), OperatorName),
			envfuncs.LoadDockerImageToCluster(OperatorName, operatorImage),
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				if err := setupOperator(ctx, cfg, operatorImage); err != nil {
					return ctx, err
				}
				return ctx, nil
			},
		)
	}

	// Run test suite
	testResult := TestEnv.Run(m)

	// Only destroy the cluster if tests passed
	if testResult == 0 {
		TestEnv.Finish(
			envfuncs.DestroyCluster(OperatorName),
		)
	} else {
		fmt.Printf("\nTests failed! Keeping Kind cluster '%s' for debugging.\n", OperatorName)
		fmt.Printf("You can access it with 'kubectl --context kind-%s'\n", OperatorName)
	}

	os.Exit(testResult)
}

func getOperatorImage() string {
	img := os.Getenv("IMG")
	if img != "" {
		return img
	}
	// fallback to default if not set
	return DefaultOperatorImage
}
