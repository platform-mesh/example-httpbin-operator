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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
)

var _ = Describe("HttpBinDeployment Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a resource", func() {
		const resourceName = "test-httpbindeployment"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind HttpBinDeployment")
			httpbindeployment := &orchestratev1alpha1.HttpBinDeployment{}
			err := k8sClient.Get(ctx, typeNamespacedName, httpbindeployment)
			if err != nil && errors.IsNotFound(err) {
				resource := &orchestratev1alpha1.HttpBinDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							"syncagent.kcp.io/remote-object-name":      "remote-name",
							"syncagent.kcp.io/remote-object-namespace": "remote-namespace",
						},
					},
					Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
						Service: orchestratev1alpha1.ServiceConfig{
							Type: "ClusterIP",
							Port: 443,
						},
						Deployment: orchestratev1alpha1.DeploymentConfig{
							Replicas: 1,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &orchestratev1alpha1.HttpBinDeployment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizer to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)

				By("Cleanup the specific resource instance HttpBinDeployment")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Cleanup local resources
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "httpbin-remote-name-remote-namespace",
				Namespace: "default",
			}, deployment)
			if err == nil {
				_ = k8sClient.Delete(ctx, deployment)
			}

			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "httpbin-remote-name-remote-namespace",
				Namespace: "default",
			}, service)
			if err == nil {
				_ = k8sClient.Delete(ctx, service)
			}
		})

		It("should successfully reconcile and create Deployment and Service", func() {
			By("Reconciling the created resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment was created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(httpbinImage))

			By("Verifying Service was created")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, service)
			}, timeout, interval).Should(Succeed())

			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(443)))
		})

		It("should add finalizer to HttpBinDeployment", func() {
			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer was added")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)).To(Succeed())
			Expect(httpBinDeployment.Finalizers).To(ContainElement(finalizerName))
		})

		It("should handle not found resource gracefully", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
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

		It("should use fallback resource name when syncagent labels are missing", func() {
			By("Creating HttpBinDeployment without syncagent labels")
			noLabelsResourceName := "test-no-labels"
			noLabelsNamespacedName := types.NamespacedName{
				Name:      noLabelsResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      noLabelsResourceName,
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "ClusterIP",
						Port: 443,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				// Cleanup
				res := &orchestratev1alpha1.HttpBinDeployment{}
				err := k8sClient.Get(ctx, noLabelsNamespacedName, res)
				if err == nil {
					res.Finalizers = nil
					_ = k8sClient.Update(ctx, res)
					_ = k8sClient.Delete(ctx, res)
				}
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + noLabelsResourceName,
					Namespace: "default",
				}, deployment)
				if err == nil {
					_ = k8sClient.Delete(ctx, deployment)
				}
				service := &corev1.Service{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + noLabelsResourceName,
					Namespace: "default",
				}, service)
				if err == nil {
					_ = k8sClient.Delete(ctx, service)
				}
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: noLabelsNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment was created with fallback name")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + noLabelsResourceName,
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())
		})

		It("should create Service with correct type", func() {
			By("Creating HttpBinDeployment with NodePort service type")
			nodePortResourceName := "test-nodeport"
			nodePortNamespacedName := types.NamespacedName{
				Name:      nodePortResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodePortResourceName,
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "NodePort",
						Port: 8080,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				res := &orchestratev1alpha1.HttpBinDeployment{}
				err := k8sClient.Get(ctx, nodePortNamespacedName, res)
				if err == nil {
					res.Finalizers = nil
					_ = k8sClient.Update(ctx, res)
					_ = k8sClient.Delete(ctx, res)
				}
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + nodePortResourceName,
					Namespace: "default",
				}, deployment)
				if err == nil {
					_ = k8sClient.Delete(ctx, deployment)
				}
				service := &corev1.Service{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + nodePortResourceName,
					Namespace: "default",
				}, service)
				if err == nil {
					_ = k8sClient.Delete(ctx, service)
				}
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nodePortNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Service has correct type and port")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + nodePortResourceName,
					Namespace: "default",
				}, service)
			}, timeout, interval).Should(Succeed())

			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
		})

		It("should create Deployment with custom replicas", func() {
			By("Creating HttpBinDeployment with custom replicas")
			replicasResourceName := "test-replicas"
			replicasNamespacedName := types.NamespacedName{
				Name:      replicasResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      replicasResourceName,
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 3,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				res := &orchestratev1alpha1.HttpBinDeployment{}
				err := k8sClient.Get(ctx, replicasNamespacedName, res)
				if err == nil {
					res.Finalizers = nil
					_ = k8sClient.Update(ctx, res)
					_ = k8sClient.Delete(ctx, res)
				}
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + replicasResourceName,
					Namespace: "default",
				}, deployment)
				if err == nil {
					_ = k8sClient.Delete(ctx, deployment)
				}
				service := &corev1.Service{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + replicasResourceName,
					Namespace: "default",
				}, service)
				if err == nil {
					_ = k8sClient.Delete(ctx, service)
				}
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: replicasNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment has correct replicas")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + replicasResourceName,
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should use default replicas when not specified", func() {
			By("Creating HttpBinDeployment without replicas")
			defaultReplicasResourceName := "test-default-replicas"
			defaultReplicasNamespacedName := types.NamespacedName{
				Name:      defaultReplicasResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultReplicasResourceName,
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				res := &orchestratev1alpha1.HttpBinDeployment{}
				err := k8sClient.Get(ctx, defaultReplicasNamespacedName, res)
				if err == nil {
					res.Finalizers = nil
					_ = k8sClient.Update(ctx, res)
					_ = k8sClient.Delete(ctx, res)
				}
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + defaultReplicasResourceName,
					Namespace: "default",
				}, deployment)
				if err == nil {
					_ = k8sClient.Delete(ctx, deployment)
				}
				service := &corev1.Service{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + defaultReplicasResourceName,
					Namespace: "default",
				}, service)
				if err == nil {
					_ = k8sClient.Delete(ctx, service)
				}
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: defaultReplicasNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment has default replicas (1)")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + defaultReplicasResourceName,
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should include custom labels and annotations in Deployment", func() {
			By("Creating HttpBinDeployment with custom labels and annotations")
			customLabelsResourceName := "test-custom-labels"
			customLabelsNamespacedName := types.NamespacedName{
				Name:      customLabelsResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      customLabelsResourceName,
					Namespace: "default",
					Labels: map[string]string{
						"custom-label": "custom-value",
					},
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Labels: map[string]string{
							"spec-label": "spec-value",
						},
						Annotations: map[string]string{
							"test-annotation": "test-value",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				res := &orchestratev1alpha1.HttpBinDeployment{}
				err := k8sClient.Get(ctx, customLabelsNamespacedName, res)
				if err == nil {
					res.Finalizers = nil
					_ = k8sClient.Update(ctx, res)
					_ = k8sClient.Delete(ctx, res)
				}
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + customLabelsResourceName,
					Namespace: "default",
				}, deployment)
				if err == nil {
					_ = k8sClient.Delete(ctx, deployment)
				}
				service := &corev1.Service{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + customLabelsResourceName,
					Namespace: "default",
				}, service)
				if err == nil {
					_ = k8sClient.Delete(ctx, service)
				}
			}()

			By("Reconciling the resource")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: customLabelsNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment has custom labels and annotations")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + customLabelsResourceName,
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(deployment.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
			Expect(deployment.Labels).To(HaveKeyWithValue("spec-label", "spec-value"))
			Expect(deployment.Annotations).To(HaveKeyWithValue("test-annotation", "test-value"))
		})

		It("should update Deployment when spec changes", func() {
			By("Reconciling to create resources first")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial Deployment")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

			By("Updating HttpBinDeployment spec")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)).To(Succeed())
			httpBinDeployment.Spec.Deployment.Replicas = 2
			Expect(k8sClient.Update(ctx, httpBinDeployment)).To(Succeed())

			By("Reconciling again")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment was updated")
			Eventually(func() int32 {
				deployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, deployment)
				if err != nil {
					return 0
				}
				return *deployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))
		})

		It("should update Service when spec changes", func() {
			By("Reconciling to create resources first")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial Service")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, service)
			}, timeout, interval).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(443)))

			By("Updating HttpBinDeployment spec")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, httpBinDeployment)).To(Succeed())
			httpBinDeployment.Spec.Service.Port = 8080
			Expect(k8sClient.Update(ctx, httpBinDeployment)).To(Succeed())

			By("Reconciling again")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Service was updated")
			Eventually(func() int32 {
				service := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-remote-name-remote-namespace",
					Namespace: "default",
				}, service)
				if err != nil {
					return 0
				}
				return service.Spec.Ports[0].Port
			}, timeout, interval).Should(Equal(int32(8080)))
		})
	})

	Context("When testing helper functions", func() {
		It("should generate correct resource name with syncagent labels", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"syncagent.kcp.io/remote-object-name":      "remote-name",
						"syncagent.kcp.io/remote-object-namespace": "remote-namespace",
					},
				},
			}

			name := reconciler.getResourceName(httpBinDeployment)
			Expect(name).To(Equal("httpbin-remote-name-remote-namespace"))
		})

		It("should generate correct resource name without syncagent labels", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			}

			name := reconciler.getResourceName(httpBinDeployment)
			Expect(name).To(Equal("httpbin-test-deployment"))
		})

		It("should generate correct DNS name with syncagent labels", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"syncagent.kcp.io/remote-object-name":      "remote-name",
						"syncagent.kcp.io/remote-object-namespace": "remote-namespace",
						"syncagent.kcp.io/remote-object-cluster":   "cluster1",
					},
				},
			}

			dnsName := reconciler.getDNSName(httpBinDeployment)
			Expect(dnsName).To(Equal("remote-name-remote-namespace-cluster1.localhost"))
		})

		It("should generate correct DNS name without syncagent labels", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			}

			dnsName := reconciler.getDNSName(httpBinDeployment)
			Expect(dnsName).To(Equal("test-deployment.localhost"))
		})
	})

	Context("When testing updateIngressRules function", func() {
		It("should add new rule when host not found", func() {
			ingress := &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{},
				},
			}

			updated := updateIngressRules("new-host.example.com", "test-service", ingress)
			Expect(updated).To(BeTrue())
			Expect(ingress.Spec.Rules).To(HaveLen(1))
			Expect(ingress.Spec.Rules[0].Host).To(Equal("new-host.example.com"))
			Expect(ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name).To(Equal("test-service"))
		})

		It("should not add duplicate rule when host exists", func() {
			pathType := networkingv1.PathTypePrefix
			ingress := &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "existing-host.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "existing-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			updated := updateIngressRules("existing-host.example.com", "test-service", ingress)
			Expect(updated).To(BeFalse())
			Expect(ingress.Spec.Rules).To(HaveLen(1))
		})
	})

	Context("When testing updateDnsAnnotation function", func() {
		It("should add DNS annotation when not present", func() {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}

			updated := updateDnsAnnotation("new-host.example.com", ingress)
			Expect(updated).To(BeTrue())
			Expect(ingress.Annotations["dns.gardener.cloud/dnsnames"]).To(Equal("new-host.example.com"))
		})

		It("should append DNS name when annotation exists but doesn't contain the host", func() {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "existing-host.example.com",
					},
				},
			}

			updated := updateDnsAnnotation("new-host.example.com", ingress)
			Expect(updated).To(BeTrue())
			Expect(ingress.Annotations["dns.gardener.cloud/dnsnames"]).To(Equal("existing-host.example.com,new-host.example.com"))
		})

		It("should not update when DNS name already exists", func() {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "existing-host.example.com,new-host.example.com",
					},
				},
			}

			updated := updateDnsAnnotation("new-host.example.com", ingress)
			Expect(updated).To(BeFalse())
		})
	})

	Context("When testing deploymentNeedsUpdate function", func() {
		It("should detect replica changes", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 3,
					},
				},
			}

			oldReplicas := int32(1)
			deployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: &oldReplicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":        "httpbin",
								"httpbin_cr": "test",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{},
								},
							},
						},
					},
				},
			}

			needsUpdate := reconciler.deploymentNeedsUpdate(httpBinDeployment, deployment)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should detect label changes", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 1,
						Labels: map[string]string{
							"new-label": "new-value",
						},
					},
				},
			}

			replicas := int32(1)
			deployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":        "httpbin",
								"httpbin_cr": "test",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{},
								},
							},
						},
					},
				},
			}

			needsUpdate := reconciler.deploymentNeedsUpdate(httpBinDeployment, deployment)
			Expect(needsUpdate).To(BeTrue())
		})
	})

	Context("When testing serviceNeedsUpdate function", func() {
		It("should detect service type changes", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "NodePort",
						Port: 443,
					},
				},
			}

			service := &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{Port: 443},
					},
					Selector: map[string]string{
						"app":        "httpbin",
						"httpbin_cr": "test",
					},
				},
			}

			needsUpdate := reconciler.serviceNeedsUpdate(httpBinDeployment, service)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should detect port changes", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "ClusterIP",
						Port: 8080,
					},
				},
			}

			service := &corev1.Service{
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{Port: 443},
					},
					Selector: map[string]string{
						"app":        "httpbin",
						"httpbin_cr": "test",
					},
				},
			}

			needsUpdate := reconciler.serviceNeedsUpdate(httpBinDeployment, service)
			Expect(needsUpdate).To(BeTrue())
		})
	})

	Context("When testing ingressForHttpBin function", func() {
		It("should create ingress with correct configuration", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
			}

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-test-ingress",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 443},
					},
				},
			}

			ingress := reconciler.ingressForHttpBin(httpBinDeployment, service)
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("httpbin-test-ingress"))
			Expect(ingress.Namespace).To(Equal("default"))
			Expect(ingress.Spec.Rules).To(HaveLen(1))
			Expect(ingress.Spec.Rules[0].Host).To(Equal("test-ingress.localhost"))
			Expect(ingress.Annotations).To(HaveKey("dns.gardener.cloud/dnsnames"))
		})

		It("should include custom labels in ingress", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-labels",
					Namespace: "default",
					Labels: map[string]string{
						"custom-label": "custom-value",
					},
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Labels: map[string]string{
							"spec-label": "spec-value",
						},
					},
				},
			}

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-test-ingress-labels",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 443},
					},
				},
			}

			ingress := reconciler.ingressForHttpBin(httpBinDeployment, service)
			Expect(ingress.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
			Expect(ingress.Labels).To(HaveKeyWithValue("spec-label", "spec-value"))
			Expect(ingress.Labels).To(HaveKeyWithValue("app", "httpbin"))
		})
	})

	Context("When testing setHttpBinDeploymentStatusCondition function", func() {
		It("should set condition correctly", func() {
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Generation: 1,
				},
			}

			setHttpBinDeploymentStatusCondition(
				httpBinDeployment,
				metav1.ConditionTrue,
				orchestratev1alpha1.HttpBinDeploymentConditionTypeReady,
				orchestratev1alpha1.HttpBinDeploymentConditionReasonReady,
				"Test message",
			)

			Expect(httpBinDeployment.Status.Conditions).To(HaveLen(1))
			Expect(httpBinDeployment.Status.Conditions[0].Type).To(Equal(orchestratev1alpha1.HttpBinDeploymentConditionTypeReady))
			Expect(httpBinDeployment.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(httpBinDeployment.Status.Conditions[0].Reason).To(Equal(orchestratev1alpha1.HttpBinDeploymentConditionReasonReady))
			Expect(httpBinDeployment.Status.Conditions[0].Message).To(Equal("Test message"))
		})

		It("should update existing condition", func() {
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Generation: 1,
				},
				Status: orchestratev1alpha1.HttpBinDeploymentStatus{
					Conditions: []metav1.Condition{
						{
							Type:   orchestratev1alpha1.HttpBinDeploymentConditionTypeReady,
							Status: metav1.ConditionFalse,
							Reason: "OldReason",
						},
					},
				},
			}

			setHttpBinDeploymentStatusCondition(
				httpBinDeployment,
				metav1.ConditionTrue,
				orchestratev1alpha1.HttpBinDeploymentConditionTypeReady,
				orchestratev1alpha1.HttpBinDeploymentConditionReasonReady,
				"Updated message",
			)

			Expect(httpBinDeployment.Status.Conditions).To(HaveLen(1))
			Expect(httpBinDeployment.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(httpBinDeployment.Status.Conditions[0].Message).To(Equal("Updated message"))
		})
	})

	Context("When testing finalization", func() {
		It("should cleanup local resources when HttpBinDeployment is deleted", func() {
			ctx := context.Background()

			By("Creating HttpBinDeployment that will be deleted")
			deleteResourceName := "test-delete"
			deleteNamespacedName := types.NamespacedName{
				Name:      deleteResourceName,
				Namespace: "default",
			}

			resource := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deleteResourceName,
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling to create resources and add finalizer")
			controllerReconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: deleteNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying resources were created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + deleteResourceName,
					Namespace: "default",
				}, deployment)
			}, timeout, interval).Should(Succeed())

			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + deleteResourceName,
					Namespace: "default",
				}, service)
			}, timeout, interval).Should(Succeed())

			By("Deleting the HttpBinDeployment")
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
			Expect(k8sClient.Get(ctx, deleteNamespacedName, httpBinDeployment)).To(Succeed())
			Expect(k8sClient.Delete(ctx, httpBinDeployment)).To(Succeed())

			By("Reconciling to trigger finalization")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: deleteNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying local resources were cleaned up")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + deleteResourceName,
					Namespace: "default",
				}, deployment)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-" + deleteResourceName,
					Namespace: "default",
				}, service)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When testing deploymentForHttpBin function", func() {
		It("should create deployment with default values", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
				},
			}

			deployment := reconciler.deploymentForHttpBin(httpBinDeployment)
			Expect(deployment).NotTo(BeNil())
			Expect(deployment.Name).To(Equal("httpbin-test-deployment"))
			Expect(deployment.Namespace).To(Equal("default"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(httpbinImage))
		})

		It("should create deployment with custom replicas", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 5,
					},
				},
			}

			deployment := reconciler.deploymentForHttpBin(httpBinDeployment)
			Expect(*deployment.Spec.Replicas).To(Equal(int32(5)))
		})

		It("should exclude selector labels from custom labels", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					Labels: map[string]string{
						"app":        "should-be-ignored",
						"httpbin_cr": "should-be-ignored",
						"custom":     "value",
					},
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Labels: map[string]string{
							"app":         "also-ignored",
							"spec-custom": "spec-value",
						},
					},
				},
			}

			deployment := reconciler.deploymentForHttpBin(httpBinDeployment)
			Expect(deployment.Spec.Selector.MatchLabels["app"]).To(Equal("httpbin"))
			Expect(deployment.Spec.Selector.MatchLabels["httpbin_cr"]).To(Equal("test-deployment"))
			Expect(deployment.Labels["custom"]).To(Equal("value"))
			Expect(deployment.Labels["spec-custom"]).To(Equal("spec-value"))
		})
	})

	Context("When testing serviceForHttpBin function", func() {
		It("should create service with default values", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service).NotTo(BeNil())
			Expect(service.Name).To(Equal("httpbin-test-service"))
			Expect(service.Namespace).To(Equal("default"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(443)))
		})

		It("should create service with custom type and port", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "LoadBalancer",
						Port: 8080,
					},
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
		})

		It("should include DNS annotations", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service.Annotations).To(HaveKey("dns.gardener.cloud/dnsnames"))
			Expect(service.Annotations).To(HaveKeyWithValue("dns.gardener.cloud/ttl", "600"))
			Expect(service.Annotations).To(HaveKeyWithValue("dns.gardener.cloud/class", "garden"))
		})

		It("should exclude selector labels from custom labels in service", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Labels: map[string]string{
						"app":        "should-be-ignored",
						"httpbin_cr": "should-be-ignored",
						"custom":     "value",
					},
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service.Spec.Selector["app"]).To(Equal("httpbin"))
			Expect(service.Spec.Selector["httpbin_cr"]).To(Equal("test-service"))
			Expect(service.Labels["custom"]).To(Equal("value"))
		})
	})

	Context("When testing finalizeHttpBinDeployment function directly", func() {
		It("should handle cleanup when resources don't exist", func() {
			ctx := context.Background()

			reconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent-finalize",
					Namespace: "default",
				},
			}

			// Should not error even if resources don't exist
			err := reconciler.finalizeHttpBinDeployment(ctx, httpBinDeployment)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should cleanup resources that exist", func() {
			ctx := context.Background()

			reconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			// Create resources to clean up
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "finalize-test",
					Namespace: "default",
				},
			}

			// Create deployment
			replicas := int32(1)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-finalize-test",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "test", Image: "test:latest"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

			// Create service
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-finalize-test",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 80},
					},
					Selector: map[string]string{"app": "test"},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Run finalization
			err := reconciler.finalizeHttpBinDeployment(ctx, httpBinDeployment)
			Expect(err).NotTo(HaveOccurred())

			// Verify resources were deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-finalize-test",
					Namespace: "default",
				}, deployment)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "httpbin-finalize-test",
					Namespace: "default",
				}, service)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should cleanup ingress rule when msp ingress exists", func() {
			ctx := context.Background()

			reconciler := &HttpBinDeploymentReconciler{
				RemoteClient: k8sClient,
				LocalClient:  k8sClient,
				Scheme:       k8sClient.Scheme(),
			}

			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "finalize-ingress-test",
					Namespace: "default",
				},
			}

			// Create msp ingress with the DNS name
			dnsName := reconciler.getDNSName(httpBinDeployment)
			pathType := networkingv1.PathTypePrefix
			mspIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "msp",
					Namespace: "default",
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "other.localhost," + dnsName,
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "other.localhost",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "other-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Host: dnsName,
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "httpbin-finalize-ingress-test",
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mspIngress)).To(Succeed())

			defer func() {
				_ = k8sClient.Delete(ctx, mspIngress)
			}()

			// Run finalization
			err := reconciler.finalizeHttpBinDeployment(ctx, httpBinDeployment)
			Expect(err).NotTo(HaveOccurred())

			// Verify ingress was updated (rule removed)
			updatedIngress := &networkingv1.Ingress{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "msp",
				Namespace: "default",
			}, updatedIngress)).To(Succeed())

			// Should have only one rule now
			Expect(len(updatedIngress.Spec.Rules)).To(Equal(1))
			Expect(updatedIngress.Spec.Rules[0].Host).To(Equal("other.localhost"))

			// DNS annotation should not contain the finalized host
			Expect(updatedIngress.Annotations["dns.gardener.cloud/dnsnames"]).NotTo(ContainSubstring(dnsName))
		})
	})

	Context("When testing error handling paths", func() {
		It("should handle service annotation update in service", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-annotations",
					Namespace: "default",
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service.Annotations).To(HaveKey("dns.gardener.cloud/dnsnames"))
			Expect(service.Annotations["dns.gardener.cloud/dnsnames"]).To(Equal("test-annotations.localhost"))
		})
	})

	Context("When testing getDNSName edge cases", func() {
		It("should use base domain when only name and namespace labels are present", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"syncagent.kcp.io/remote-object-name":      "remote-name",
						"syncagent.kcp.io/remote-object-namespace": "remote-namespace",
						// Missing cluster label
					},
				},
			}

			// Should fall back to simple format because cluster label is missing
			dnsName := reconciler.getDNSName(httpBinDeployment)
			Expect(dnsName).To(Equal("test.localhost"))
		})
	})

	Context("When testing getResourceName with partial labels", func() {
		It("should use fallback when only name label is present", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-partial",
					Labels: map[string]string{
						"syncagent.kcp.io/remote-object-name": "remote-name",
						// Missing namespace label
					},
				},
			}

			name := reconciler.getResourceName(httpBinDeployment)
			Expect(name).To(Equal("httpbin-test-partial"))
		})

		It("should use fallback when only namespace label is present", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-partial-ns",
					Labels: map[string]string{
						"syncagent.kcp.io/remote-object-namespace": "remote-namespace",
						// Missing name label
					},
				},
			}

			name := reconciler.getResourceName(httpBinDeployment)
			Expect(name).To(Equal("httpbin-test-partial-ns"))
		})
	})

	Context("When testing service labels in serviceForHttpBin", func() {
		It("should include deployment labels in service", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-labels",
					Namespace: "default",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Labels: map[string]string{
							"deployment-label": "deployment-value",
						},
					},
				},
			}

			service := reconciler.serviceForHttpBin(httpBinDeployment)
			Expect(service.Labels).To(HaveKeyWithValue("deployment-label", "deployment-value"))
			Expect(service.Labels).To(HaveKeyWithValue("app", "httpbin"))
		})
	})

	Context("When testing annotation changes detection", func() {
		It("should detect annotation changes in serviceNeedsUpdate", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "ClusterIP",
						Port: 443,
					},
				},
			}

			// Service with different annotations
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"different-annotation": "value",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{Port: 443, TargetPort: intstr.FromInt32(80), Protocol: corev1.ProtocolTCP},
					},
					Selector: map[string]string{
						"app":        "httpbin",
						"httpbin_cr": "test",
					},
				},
			}

			needsUpdate := reconciler.serviceNeedsUpdate(httpBinDeployment, service)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should detect annotation changes in deploymentNeedsUpdate", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 1,
						Annotations: map[string]string{
							"new-annotation": "new-value",
						},
					},
				},
			}

			replicas := int32(1)
			deployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":        "httpbin",
								"httpbin_cr": "test",
							},
							Annotations: map[string]string{
								"old-annotation": "old-value",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{},
								},
							},
						},
					},
				},
			}

			needsUpdate := reconciler.deploymentNeedsUpdate(httpBinDeployment, deployment)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should not need update when everything matches", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Deployment: orchestratev1alpha1.DeploymentConfig{
						Replicas: 1,
					},
				},
			}

			// Get the expected deployment for comparison
			expectedDep := reconciler.deploymentForHttpBin(httpBinDeployment)

			needsUpdate := reconciler.deploymentNeedsUpdate(httpBinDeployment, expectedDep)
			Expect(needsUpdate).To(BeFalse())
		})
	})

	Context("When testing selector changes in serviceNeedsUpdate", func() {
		It("should detect selector changes", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "ClusterIP",
						Port: 443,
					},
				},
			}

			// Service with different selector
			expectedSvc := reconciler.serviceForHttpBin(httpBinDeployment)
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: expectedSvc.Annotations,
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{Port: 443, TargetPort: intstr.FromInt32(80), Protocol: corev1.ProtocolTCP},
					},
					Selector: map[string]string{
						"app":        "httpbin",
						"httpbin_cr": "wrong-name",
					},
				},
			}

			needsUpdate := reconciler.serviceNeedsUpdate(httpBinDeployment, service)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should not need update when service matches", func() {
			reconciler := &HttpBinDeploymentReconciler{}
			httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
					Service: orchestratev1alpha1.ServiceConfig{
						Type: "ClusterIP",
						Port: 443,
					},
				},
			}

			expectedSvc := reconciler.serviceForHttpBin(httpBinDeployment)
			needsUpdate := reconciler.serviceNeedsUpdate(httpBinDeployment, expectedSvc)
			Expect(needsUpdate).To(BeFalse())
		})
	})
})
