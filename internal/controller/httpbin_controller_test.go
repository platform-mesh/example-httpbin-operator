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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
)

var _ = Describe("HttpBin Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a resource", func() {
		const resourceName = "test-httpbin"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind HttpBin")
			httpbin := &orchestratev1alpha1.HttpBin{}
			err := k8sClient.Get(ctx, typeNamespacedName, httpbin)
			if err != nil && errors.IsNotFound(err) {
				resource := &orchestratev1alpha1.HttpBin{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: orchestratev1alpha1.HttpBinSpec{
						Region: "us-east-1",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &orchestratev1alpha1.HttpBin{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance HttpBin")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
			// Also cleanup any HttpBinDeployment that was created
			deployment := &orchestratev1alpha1.HttpBinDeployment{}
			err = k8sClient.Get(ctx, typeNamespacedName, deployment)
			if err == nil {
				Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
			}
		})

		It("should successfully reconcile and create HttpBinDeployment", func() {
			By("Reconciling the created resource")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// Check that reconciliation requests a requeue
			Expect(result).NotTo(Equal(reconcile.Result{}))

			By("Verifying HttpBinDeployment was created")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)
			}, timeout, interval).Should(Succeed())

			Expect(httpBinDeployment.Spec.Service.Port).To(Equal(int32(80)))
		})

		It("should always use backend port 80 for service", func() {
			By("Creating HttpBin resource")
			httpsResourceName := "test-httpbin-https"
			httpsNamespacedName := types.NamespacedName{
				Name:      httpsResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httpsResourceName,
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinSpec{
					Region: "us-west-1",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				// Cleanup
				_ = k8sClient.Delete(ctx, resource)
				deployment := &orchestratev1alpha1.HttpBinDeployment{}
				_ = k8sClient.Get(ctx, httpsNamespacedName, deployment)
				_ = k8sClient.Delete(ctx, deployment)
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: httpsNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying HttpBinDeployment service port is 80 (SSL terminated at ingress/httproute)")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, httpsNamespacedName, httpBinDeployment)
			}, timeout, interval).Should(Succeed())

			Expect(httpBinDeployment.Spec.Service.Port).To(Equal(int32(80)))
		})

		It("should handle not found resource gracefully", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should propagate status from HttpBinDeployment to HttpBin", func() {
			By("Creating HttpBin and HttpBinDeployment with status")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			// First reconcile to create HttpBinDeployment
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Updating HttpBinDeployment status")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)
			}, timeout, interval).Should(Succeed())

			httpBinDeployment.Status.URL = "https://test.example.com"
			httpBinDeployment.Status.IsDeploymentReady = true
			Expect(k8sClient.Status().Update(ctx, httpBinDeployment)).To(Succeed())

			By("Reconciling again to propagate status")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying HttpBin status was updated")
			httpBin := &orchestratev1alpha1.HttpBin{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, httpBin)).To(Succeed())
			Expect(httpBin.Status.URL).To(Equal("https://test.example.com"))
			Expect(httpBin.Status.Ready).To(BeTrue())
		})

		It("should copy labels from HttpBin to HttpBinDeployment", func() {
			By("Creating HttpBin with labels")
			labeledResourceName := "test-httpbin-labels"
			labeledNamespacedName := types.NamespacedName{
				Name:      labeledResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      labeledResourceName,
					Namespace: "default",
					Labels: map[string]string{
						"custom-label": "custom-value",
						"environment":  "test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				_ = k8sClient.Delete(ctx, resource)
				deployment := &orchestratev1alpha1.HttpBinDeployment{}
				_ = k8sClient.Get(ctx, labeledNamespacedName, deployment)
				_ = k8sClient.Delete(ctx, deployment)
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: labeledNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying HttpBinDeployment has the labels")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, labeledNamespacedName, httpBinDeployment)
			}, timeout, interval).Should(Succeed())

			Expect(httpBinDeployment.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
			Expect(httpBinDeployment.Labels).To(HaveKeyWithValue("environment", "test"))
		})

		It("should set condition when deployment is not ready", func() {
			By("Reconciling to create HttpBinDeployment")
			controllerReconciler := &HttpBinReconciler{
				RemoteClient: k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Setting HttpBinDeployment status to not ready")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)
			}, timeout, interval).Should(Succeed())

			httpBinDeployment.Status.URL = "https://test.example.com"
			httpBinDeployment.Status.IsDeploymentReady = false
			Expect(k8sClient.Status().Update(ctx, httpBinDeployment)).To(Succeed())

			By("Reconciling again")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying HttpBin has progressing condition")
			httpBin := &orchestratev1alpha1.HttpBin{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, httpBin)).To(Succeed())
			Expect(httpBin.Status.Ready).To(BeFalse())

			// Check condition exists
			found := false
			for _, condition := range httpBin.Status.Conditions {
				if condition.Type == orchestratev1alpha1.HttpBinConditionTypeReady {
					found = true
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Reason).To(Equal(orchestratev1alpha1.HttpBinConditionReasonDeploymentProgressing))
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})
