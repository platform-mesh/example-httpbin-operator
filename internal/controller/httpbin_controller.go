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
		return ctrl.Result{}, err
	}

	logger.Info("Found HttpBin resource in remote cluster",
		"HttpBin.Namespace", httpBin.Namespace,
		"HttpBin.Name", httpBin.Name,
		"HttpBin.ResourceVersion", httpBin.ResourceVersion,
		"HttpBin.Spec", httpBin.Spec)

	// Define a new HttpBinDeployment object
	httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpBin.Name,
			Namespace: httpBin.Namespace,
			Labels:    httpBin.Labels,
		},
		Spec: orchestratev1alpha1.HttpBinDeploymentSpec{
			Service: orchestratev1alpha1.ServiceConfig{
				Type: *fDeploymentServiceType,
				Port: func() int32 {
					if httpBin.Spec.EnableHTTPS {
						return 8443
					}
					return 443
				}(),
			},
			Deployment: orchestratev1alpha1.DeploymentConfig{
				Labels: map[string]string{
					"httpbin_cr": httpBin.Name,
				},
			},
		},
	}

	logger.Info("Created HttpBinDeployment object",
		"HttpBinDeployment.Name", httpBinDeployment.Name,
		"HttpBinDeployment.Namespace", httpBinDeployment.Namespace,
		"HttpBinDeployment.Spec", httpBinDeployment.Spec)

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

			meta.SetStatusCondition(&httpBin.Status.Conditions, metav1.Condition{
				Type:               orchestratev1alpha1.HttpBinConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             orchestratev1alpha1.HttpBinReasonDeploymentCreateFailed,
				Message:            "Failed to create HttpBinDeployment: " + err.Error(),
				ObservedGeneration: httpBin.GetGeneration(),
			})
			// Optionally update status immediately
			_ = r.RemoteClient.Status().Update(ctx, httpBin)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully created HttpBinDeployment in remote cluster",
			"HttpBinDeployment.Namespace", httpBinDeployment.Namespace,
			"HttpBinDeployment.Name", httpBinDeployment.Name)

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

	// Update HttpBin status based on HttpBinDeployment status
	statusNeedsUpdate := false

	// Format URL as https://HOST
	if found.Status.URL != "" {
		if httpBin.Status.URL != found.Status.URL {
			httpBin.Status.URL = found.Status.URL
			statusNeedsUpdate = true
		}
	}

	if httpBin.Status.Ready != found.Status.IsDeploymentReady {
		httpBin.Status.Ready = found.Status.IsDeploymentReady
		statusNeedsUpdate = true
	}

	if httpBin.Status.Ready {
		meta.SetStatusCondition(&httpBin.Status.Conditions, metav1.Condition{
			Type:               orchestratev1alpha1.HttpBinConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             orchestratev1alpha1.HttpBinReasonDeploymentReady,
			Message:            "HttpBin is deployed and URL is available",
			ObservedGeneration: httpBin.GetGeneration(),
		})
	} else {
		meta.SetStatusCondition(&httpBin.Status.Conditions, metav1.Condition{
			Type:               orchestratev1alpha1.HttpBinConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             orchestratev1alpha1.HttpBinReasonDeploymentNotReady,
			Message:            "Deployment exists but is not yet available",
			ObservedGeneration: httpBin.GetGeneration(),
		})
	}

	// Update status if needed
	if statusNeedsUpdate {
		logger.Info("Updating HttpBin status",
			"URL", httpBin.Status.URL,
			"Ready", httpBin.Status.Ready,
			"Conditions", httpBin.Status.Conditions)

		if err := r.RemoteClient.Status().Update(ctx, httpBin); err != nil {
			logger.Error(err, "Failed to update HttpBin status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
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
