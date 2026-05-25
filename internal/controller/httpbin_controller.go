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
	"flag"

	"sigs.k8s.io/controller-runtime/pkg/controller"

	orchestratev1alpha1 "http-operator/api/v1alpha1"
	"http-operator/internal/metrics"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HttpBinReconciler reconciles a HttpBin object in a remote cluster
type HttpBinReconciler struct {
	RemoteClient client.Client
	Scheme       *runtime.Scheme
	RemoteCache  cache.Cache // Add this field
}

// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbins/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbins/finalizers,verbs=update
// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbindeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbindeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=orchestrate.platform-mesh.io,resources=httpbindeployments/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

var (
	fDeploymentServiceType = flag.String("deployment-service-type", "ClusterIP", "Service type for HttpBinDeployment")
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *HttpBinReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Starting reconciliation for HttpBin",
		"namespace", req.Namespace,
		"name", req.Name)

	// Fetch the HttpBin instance from remote cluster
	httpBin := &orchestratev1alpha1.HttpBin{}
	err := r.RemoteClient.Get(ctx, req.NamespacedName, httpBin)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			logger.Info("HttpBin resource not found in remote cluster")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get HttpBin from remote cluster",
			"error", err,
			"error_type", errors.IsNotFound(err),
			"error_forbidden", errors.IsForbidden(err),
			"error_invalid", errors.IsInvalid(err))
		metrics.HttpBinReconciled.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	logger.Info("Found HttpBin resource in remote cluster",
		"HttpBin.Namespace", httpBin.Namespace,
		"HttpBin.Name", httpBin.Name,
		"HttpBin.ResourceVersion", httpBin.ResourceVersion,
		"HttpBin.Spec", httpBin.Spec)

	// Define a new HttpBinDeployment object
	// Service port is always 80 for backend pods, SSL is terminated at ingress/httproute level
	httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpBin.Name,
			Namespace: httpBin.Namespace,
			Labels:    httpBin.Labels,
		},
		Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
			Service: orchestratev1alpha1.ServiceConfig{
				Type: *fDeploymentServiceType,
				Port: 80,
			},
			Deployment: orchestratev1alpha1.DeploymentConfig{
				Labels: map[string]string{
					"httpbin_cr": httpBin.Name,
				},
			},
		},
	}

	// Set HttpBin instance as the owner and controller
	if err := controllerutil.SetControllerReference(httpBin, httpBinDeployment, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference",
			"error", err,
			"HttpBin.Name", httpBin.Name,
			"HttpBin.UID", httpBin.UID,
			"HttpBinDeployment.Name", httpBinDeployment.Name)
		return ctrl.Result{}, err
	}

	// Check if this HttpBinDeployment already exists in remote cluster
	found := &orchestratev1alpha1.HttpBinDeployment{}
	err = r.RemoteClient.Get(ctx, types.NamespacedName{Name: httpBinDeployment.Name, Namespace: httpBinDeployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new HttpBinDeployment in remote cluster",
			"HttpBinDeployment.Namespace", httpBinDeployment.Namespace,
			"HttpBinDeployment.Name", httpBinDeployment.Name)

		err = r.RemoteClient.Create(ctx, httpBinDeployment)
		if err != nil {
			logger.Error(err, "Failed to create HttpBinDeployment in remote cluster",
				"error", err,
				"error_type", errors.IsNotFound(err),
				"error_forbidden", errors.IsForbidden(err),
				"error_invalid", errors.IsInvalid(err),
				"error_already_exists", errors.IsAlreadyExists(err))

			setHttpBinConditionStatusCondition(httpBin, metav1.ConditionFalse, orchestratev1alpha1.HttpBinConditionReasonDeploymentFailed, "Failed to create HttpBinDeployment: "+err.Error())
			_ = r.RemoteClient.Status().Update(ctx, httpBin)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully created HttpBinDeployment in remote cluster",
			"HttpBinDeployment.Namespace", httpBinDeployment.Namespace,
			"HttpBinDeployment.Name", httpBinDeployment.Name,
			"HttpBinDeployment.Spec", httpBinDeployment.Spec)

		metrics.HttpBinReconciled.WithLabelValues("created").Inc()
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get HttpBinDeployment from remote cluster",
			"error", err,
			"error_type", errors.IsNotFound(err),
			"error_forbidden", errors.IsForbidden(err),
			"error_invalid", errors.IsInvalid(err))
		return ctrl.Result{}, err
	}

	logger.Info("Found existing HttpBinDeployment in remote cluster",
		"HttpBinDeployment.Name", found.Name,
		"HttpBinDeployment.Namespace", found.Namespace,
		"HttpBinDeployment.ResourceVersion", found.ResourceVersion,
		"HttpBinDeployment.Spec", found.Spec,
		"HttpBinDeployment.Status", found.Status)

	// Only update status if something actually changed
	statusNeedsUpdate := httpBin.Status.URL != found.Status.URL ||
		httpBin.Status.Ready != found.Status.IsDeploymentReady

	if statusNeedsUpdate {
		httpBin.Status.URL = found.Status.URL
		httpBin.Status.Ready = found.Status.IsDeploymentReady

		if found.Status.IsDeploymentReady {
			setHttpBinConditionStatusCondition(httpBin, metav1.ConditionTrue, orchestratev1alpha1.HttpBinConditionReasonDeploymentReady, "HttpBin is deployed and URL is available")
		} else {
			setHttpBinConditionStatusCondition(httpBin, metav1.ConditionFalse, orchestratev1alpha1.HttpBinConditionReasonDeploymentProgressing, "Deployment exists but is not yet available")
		}

		logger.Info("Updating HttpBin status",
			"URL", httpBin.Status.URL,
			"Ready", httpBin.Status.Ready,
			"Conditions", httpBin.Status.Conditions)

		if err := r.RemoteClient.Status().Update(ctx, httpBin); err != nil {
			logger.Error(err, "Failed to update HttpBin status")
			return ctrl.Result{}, err
		}
	}

	metrics.HttpBinReconciled.WithLabelValues("success").Inc()
	return ctrl.Result{}, nil
}

func setHttpBinConditionStatusCondition(httpBin *orchestratev1alpha1.HttpBin, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&httpBin.Status.Conditions, metav1.Condition{
		Type:               orchestratev1alpha1.HttpBinConditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: httpBin.GetGeneration(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *HttpBinReconciler) SetupWithManager(remoteMgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(remoteMgr).
		For(&orchestratev1alpha1.HttpBin{}).
		// Watch HttpBinDeployment resources that are owned by HttpBin
		Owns(&orchestratev1alpha1.HttpBinDeployment{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
