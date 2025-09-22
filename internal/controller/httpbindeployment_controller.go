package controller

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	orchestratev1alpha1 "http-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	httpbinImage  = "nwallus308/httpbin:latest"
	pollInterval  = 60 * time.Second
	finalizerName = "httpbindeployment.orchestrate.platform-mesh.io/finalizer"

	// TODO: This is only here to silence golangci-lint. Instead the
	// selector labels should be in a list and filtered with that.
	labelHttpbinCr = "httpbin_cr"
	labelApp       = "app"
)

var (
	fDomain           = flag.String("domain", "", "Domain for DNS, setting prevents domain generation from labels")
	fBaseDomain       = flag.String("base-domain", "localhost", "Base domain for DNS names, not used if --domain is set")
	fLocalIngress     = flag.Bool("local-ingress", false, "Manage local ingress")
	fIngressClassName = flag.String("ingress-class-name", "", "Ingress class name to use for local ingress")
	fTlsSecretName    = flag.String("tls-secret-name", "", "Name of the TLS secret to use for local ingress, if empty no TLS will be used")
)

type HttpBinDeploymentReconciler struct {
	RemoteClient client.Client
	LocalClient  client.Client
	Scheme       *runtime.Scheme
}

func (r *HttpBinDeploymentReconciler) getResourceName(m *orchestratev1alpha1.HttpBinDeployment) string {
	// api-syncagent labels
	if m.Labels["syncagent.kcp.io/remote-object-name"] != "" &&
		m.Labels["syncagent.kcp.io/remote-object-namespace"] != "" {
		return fmt.Sprintf("httpbin-%s-%s",
			m.Labels["syncagent.kcp.io/remote-object-name"],
			m.Labels["syncagent.kcp.io/remote-object-namespace"])
	}

	// Fallback to using the HttpBinDeployment name if labels are not
	// set
	return "httpbin-" + m.Name
}

// getDNSName returns the DNS name for the HttpBinDeployment
func (r *HttpBinDeploymentReconciler) getDNSName(m *orchestratev1alpha1.HttpBinDeployment) string {
	if *fDomain != "" {
		return *fDomain
	}

	// api-syncagent labels
	if m.Labels["syncagent.kcp.io/remote-object-name"] != "" &&
		m.Labels["syncagent.kcp.io/remote-object-namespace"] != "" &&
		m.Labels["syncagent.kcp.io/remote-object-cluster"] != "" {
		return fmt.Sprintf("%s-%s-%s.%s",
			m.Labels["syncagent.kcp.io/remote-object-name"],
			m.Labels["syncagent.kcp.io/remote-object-namespace"],
			m.Labels["syncagent.kcp.io/remote-object-cluster"],
			*fBaseDomain)
	}

	// Fallback to using the HttpBinDeployment with base domain
	return fmt.Sprintf("%s.%s", m.Name, *fBaseDomain)
}

func (r *HttpBinDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { //nolint:gocyclo
	// TODO cyclomatic complexity is very high, this should be
	// refactored
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "namespacedName", req.NamespacedName)

	// Fetch the HttpBinDeployment from remote cluster (Cluster B)
	httpBinDeployment := &orchestratev1alpha1.HttpBinDeployment{}
	err := r.RemoteClient.Get(ctx, req.NamespacedName, httpBinDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted - already handled by finalizer
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get HttpBinDeployment from remote cluster")
		return ctrl.Result{}, err
	}

	// Handle finalizer
	if !httpBinDeployment.DeletionTimestamp.IsZero() {
		// Resource is being deleted
		if controllerutil.ContainsFinalizer(httpBinDeployment, finalizerName) {
			// Run finalization logic
			if err := r.finalizeHttpBinDeployment(ctx, httpBinDeployment); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(httpBinDeployment, finalizerName)
			if err := r.RemoteClient.Update(ctx, httpBinDeployment); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(httpBinDeployment, finalizerName) {
		controllerutil.AddFinalizer(httpBinDeployment, finalizerName)
		if err := r.RemoteClient.Update(ctx, httpBinDeployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle Deployment in local cluster (Cluster A)
	deployment := &appsv1.Deployment{}
	deploymentName := r.getResourceName(httpBinDeployment)

	err = r.LocalClient.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: "default",
	}, deployment)

	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentForHttpBin(httpBinDeployment)
		logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.LocalClient.Create(ctx, dep)
		if err != nil {
			logger.Error(err, "Failed to create new Deployment")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	} else if r.deploymentNeedsUpdate(httpBinDeployment, deployment) {
		newDep := r.deploymentForHttpBin(httpBinDeployment)
		newDep.ResourceVersion = deployment.ResourceVersion

		logger.Info("Updating Deployment", "Deployment.Namespace", newDep.Namespace, "Deployment.Name", newDep.Name)
		err = r.LocalClient.Update(ctx, newDep)
		if err != nil {
			logger.Error(err, "Failed to update Deployment")
			return ctrl.Result{}, err
		}
	}

	// Handle Service in local cluster (Cluster A)
	service := &corev1.Service{}
	svc := r.serviceForHttpBin(httpBinDeployment)

	err = r.LocalClient.Get(ctx, types.NamespacedName{
		Name:      svc.Name,
		Namespace: "default",
	}, service)

	if err != nil && errors.IsNotFound(err) {
		svc := r.serviceForHttpBin(httpBinDeployment)
		logger.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.LocalClient.Create(ctx, svc)
		if err != nil {
			logger.Error(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	} else if r.serviceNeedsUpdate(httpBinDeployment, service) {
		newSvc := r.serviceForHttpBin(httpBinDeployment)
		newSvc.ResourceVersion = service.ResourceVersion
		newSvc.Spec.ClusterIP = service.Spec.ClusterIP

		logger.Info("Updating Service", "Service.Namespace", newSvc.Namespace, "Service.Name", newSvc.Name)
		err = r.LocalClient.Update(ctx, newSvc)
		if err != nil {
			logger.Error(err, "Failed to update Service")
			return ctrl.Result{}, err
		}
	}

	ingress := &networkingv1.Ingress{}

	if *fLocalIngress {
		err = r.LocalClient.Get(ctx, types.NamespacedName{
			Name:      svc.Name,
			Namespace: "default",
		}, ingress)
		desiredIngress := r.ingressForHttpBin(httpBinDeployment, svc)

		if err != nil && errors.IsNotFound(err) {
			logger.Info("Creating a new Ingress", "Ingress.Namespace", desiredIngress.Namespace, "Ingress.Name", desiredIngress.Name)
			err = r.LocalClient.Create(ctx, desiredIngress)
			if err != nil {
				logger.Error(err, "Failed to create new Ingress")
				return ctrl.Result{}, err
			}
		} else if err != nil {
			logger.Error(err, "Failed to get Ingress")
			return ctrl.Result{}, err
		} else {
			if err := r.LocalClient.Update(ctx, desiredIngress); err != nil {
				logger.Error(err, "Failed to update Ingress")
				return ctrl.Result{}, err
			}
		}
		ingress = desiredIngress
	} else {
		err = r.LocalClient.Get(ctx, types.NamespacedName{Name: "msp", Namespace: "default"}, ingress)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("skipping ingress update, ingress does not exists")
			} else {
				logger.Error(err, "Failed to get Ingress")
				return ctrl.Result{}, err
			}
		} else {
			dnsName := r.getDNSName(httpBinDeployment)
			updatedAnnotation := updateDnsAnnotation(dnsName, ingress)
			// Get the actual service name from the created service
			updatedRules := updateIngressRules(dnsName, service.Name, ingress)

			if updatedRules || updatedAnnotation {
				logger.Info("Updating Ingress", "Ingress.Namespace", ingress.Namespace, "Ingress.Name", ingress.Name)
				err = r.LocalClient.Update(ctx, ingress)
				if err != nil {
					logger.Error(err, "Failed to update Ingress")
					return ctrl.Result{}, err
				}
			}
		}
	}

	// Update status in remote cluster
	statusNeedsUpdate := false

	// Update ReadyReplicas and IsDeploymentReady
	if httpBinDeployment.Status.ReadyReplicas != deployment.Status.ReadyReplicas {
		httpBinDeployment.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		httpBinDeployment.Status.IsDeploymentReady = deployment.Status.ReadyReplicas > 0
		statusNeedsUpdate = true
	}

	url := url.URL{}

	// Only if NodePorts are used and a domain is set
	if *fDeploymentServiceType == "NodePort" && *fDomain != "" {
		url.Host = fmt.Sprintf("%s:%d", *fDomain, service.Spec.Ports[0].NodePort)
		url.Path = httpBinDeployment.Name
	} else {
		// Anything else can build from the ingress
		if len(ingress.Spec.Rules) == 0 {
			logger.Info("No ingress rules found, skipping URL update")
			return ctrl.Result{}, nil
		}

		url.Scheme = "http"
		url.Host = ingress.Spec.Rules[0].Host

		if len(ingress.Spec.TLS) > 0 {
			url.Scheme = "https"
		}

		if len(ingress.Spec.Rules[0].HTTP.Paths) == 0 {
			logger.Info("No ingress paths found, skipping URL update")
			return ctrl.Result{}, nil
		}

		url.Path = ingress.Spec.Rules[0].HTTP.Paths[0].Path
	}

	urlS := url.String()
	if httpBinDeployment.Status.URL != urlS {
		httpBinDeployment.Status.URL = urlS
		statusNeedsUpdate = true
	}

	// Update status if any changes were made
	if statusNeedsUpdate {
		logger.Info("Updating HttpBinDeployment status",
			"URL", httpBinDeployment.Status.URL,
			"ReadyReplicas", httpBinDeployment.Status.ReadyReplicas,
			"IsDeploymentReady", httpBinDeployment.Status.IsDeploymentReady)

		err = r.RemoteClient.Status().Update(ctx, httpBinDeployment)
		if err != nil {
			logger.Error(err, "Failed to update HttpBinDeployment status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: pollInterval}, nil
}

func updateIngressRules(dnsName string, serviceName string, ingress *networkingv1.Ingress) bool {
	found := false
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == dnsName {
			found = true
			break
		}
	}

	pathType := networkingv1.PathTypePrefix
	if !found {
		ingress.Spec.Rules = append(ingress.Spec.Rules, networkingv1.IngressRule{
			Host: dnsName,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: serviceName,
									Port: networkingv1.ServiceBackendPort{
										Number: 443,
									},
								},
							},
						},
					},
				},
			},
		})
		return true
	}
	return false
}

func updateDnsAnnotation(dnsName string, ingress *networkingv1.Ingress) bool {
	currentValue, ok := ingress.Annotations["dns.gardener.cloud/dnsnames"]
	if !ok {
		ingress.Annotations["dns.gardener.cloud/dnsnames"] = dnsName
		return true
	}
	if !strings.Contains(currentValue, dnsName) {
		ingress.Annotations["dns.gardener.cloud/dnsnames"] = fmt.Sprintf("%s,%s", currentValue, dnsName)
		return true
	}

	return false
}

// finalizeHttpBinDeployment handles cleanup of local resources
func (r *HttpBinDeploymentReconciler) finalizeHttpBinDeployment(ctx context.Context, httpBinDeployment *orchestratev1alpha1.HttpBinDeployment) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing HttpBinDeployment", "name", httpBinDeployment.Name)

	// Delete Deployment
	deploymentName := r.getResourceName(httpBinDeployment)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: "default",
		},
	}
	if err := r.LocalClient.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete Service
	serviceName := r.getResourceName(httpBinDeployment)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: "default",
		},
	}
	if err := r.LocalClient.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Clean up ingress DNS entry
	ingress := &networkingv1.Ingress{}
	err := r.LocalClient.Get(ctx, types.NamespacedName{Name: "msp", Namespace: "default"}, ingress)
	if err == nil {
		// Construct DNS name same way as in Reconcile
		dnsName := r.getDNSName(httpBinDeployment)

		// Remove from DNS annotation
		if currentValue, ok := ingress.Annotations["dns.gardener.cloud/dnsnames"]; ok {
			dnsNames := strings.Split(currentValue, ",")
			for i, name := range dnsNames {
				if name == dnsName {
					dnsNames = append(dnsNames[:i], dnsNames[i+1:]...)
					break
				}
			}
			ingress.Annotations["dns.gardener.cloud/dnsnames"] = strings.Join(dnsNames, ",")
		}

		// Remove matching ingress rule
		for i, rule := range ingress.Spec.Rules {
			if rule.Host == dnsName {
				ingress.Spec.Rules = append(ingress.Spec.Rules[:i], ingress.Spec.Rules[i+1:]...)
				break
			}
		}

		err = r.LocalClient.Update(ctx, ingress)
		if err != nil {
			return err
		}
	}

	logger.Info("Successfully finalized HttpBinDeployment")
	return nil
}

// deploymentNeedsUpdate checks if the deployment needs to be updated
func (r *HttpBinDeploymentReconciler) deploymentNeedsUpdate(httpBinDeployment *orchestratev1alpha1.HttpBinDeployment, deployment *appsv1.Deployment) bool {
	desiredDep := r.deploymentForHttpBin(httpBinDeployment)

	return !reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Resources, desiredDep.Spec.Template.Spec.Containers[0].Resources) ||
		*deployment.Spec.Replicas != *desiredDep.Spec.Replicas ||
		!reflect.DeepEqual(deployment.Spec.Template.Labels, desiredDep.Spec.Template.Labels) ||
		!reflect.DeepEqual(deployment.Spec.Template.Annotations, desiredDep.Spec.Template.Annotations)
}

// serviceNeedsUpdate checks if the service needs to be updated
func (r *HttpBinDeploymentReconciler) serviceNeedsUpdate(httpBinDeployment *orchestratev1alpha1.HttpBinDeployment, service *corev1.Service) bool {
	desiredSvc := r.serviceForHttpBin(httpBinDeployment)

	return service.Spec.Type != desiredSvc.Spec.Type ||
		!reflect.DeepEqual(service.Spec.Ports, desiredSvc.Spec.Ports) ||
		!reflect.DeepEqual(service.Spec.Selector, desiredSvc.Spec.Selector) ||
		!reflect.DeepEqual(service.Annotations, desiredSvc.Annotations)
}

// deploymentForHttpBin returns a httpbin Deployment object
func (r *HttpBinDeploymentReconciler) deploymentForHttpBin(m *orchestratev1alpha1.HttpBinDeployment) *appsv1.Deployment {
	// Create minimal, static selector labels - these must never change
	selectorLabels := map[string]string{
		labelApp:       "httpbin",
		labelHttpbinCr: m.Name,
	}

	// Create full set of labels for pod template and metadata
	podLabels := make(map[string]string)
	// Start with selector labels as base
	for k, v := range selectorLabels {
		podLabels[k] = v
	}
	// Add HttpBinDeployment's own labels, excluding any that would conflict with selector
	for k, v := range m.Labels {
		if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
			podLabels[k] = v
		}
	}
	// Add any additional labels from spec, excluding any that would conflict with selector
	if m.Spec.Deployment.Labels != nil {
		for k, v := range m.Spec.Deployment.Labels {
			if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
				podLabels[k] = v
			}
		}
	}

	replicas := m.Spec.Deployment.Replicas
	if replicas == 0 {
		replicas = 1
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.getResourceName(m),
			Namespace:   "default",
			Labels:      podLabels,
			Annotations: m.Spec.Deployment.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: m.Spec.Deployment.Annotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           httpbinImage,
						Name:            "httpbin",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
						}},
					}},
				},
			},
		},
	}

	// Removed SetControllerReference to prevent cross-cluster owner reference
	return dep
}

// serviceForHttpBin returns a httpbin Service object
func (r *HttpBinDeploymentReconciler) serviceForHttpBin(m *orchestratev1alpha1.HttpBinDeployment) *corev1.Service {
	// Create minimal, static selector labels - these must never change
	selectorLabels := map[string]string{
		labelApp:       "httpbin",
		labelHttpbinCr: m.Name,
	}

	// Create full set of labels for service metadata
	serviceLabels := make(map[string]string)
	// Start with selector labels as base
	for k, v := range selectorLabels {
		serviceLabels[k] = v
	}
	// Add HttpBinDeployment's own labels, excluding any that would conflict with selector
	for k, v := range m.Labels {
		if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
			serviceLabels[k] = v
		}
	}
	// Add any additional labels from spec, excluding any that would conflict with selector
	if m.Spec.Deployment.Labels != nil {
		for k, v := range m.Spec.Deployment.Labels {
			if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
				serviceLabels[k] = v
			}
		}
	}

	serviceType := m.Spec.Service.Type
	if serviceType == "" {
		serviceType = "ClusterIP"
	}

	port := m.Spec.Service.Port
	if port == 0 {
		port = 443
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getResourceName(m),
			Namespace: "default",
			Labels:    serviceLabels,
			Annotations: map[string]string{
				"dns.gardener.cloud/dnsnames": r.getDNSName(m),
				"dns.gardener.cloud/ttl":      "600",
				"dns.gardener.cloud/class":    "garden",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceType(serviceType),
			Ports: []corev1.ServicePort{{
				Port:       port,
				TargetPort: intstr.FromInt32(80),
				Protocol:   corev1.ProtocolTCP,
			}},
			Selector: selectorLabels,
		},
	}

	// Removed SetControllerReference to prevent cross-cluster owner reference
	return svc
}

// ingressForHttpBin returns an Ingress object
func (r *HttpBinDeploymentReconciler) ingressForHttpBin(m *orchestratev1alpha1.HttpBinDeployment, svc *corev1.Service) *networkingv1.Ingress {
	// Create minimal, static selector labels - these must never change
	selectorLabels := map[string]string{
		labelApp:       "httpbin",
		labelHttpbinCr: m.Name,
	}

	// Create full set of labels for service metadata
	serviceLabels := make(map[string]string)
	// Start with selector labels as base
	for k, v := range selectorLabels {
		serviceLabels[k] = v
	}
	// Add HttpBinDeployment's own labels, excluding any that would conflict with selector
	for k, v := range m.Labels {
		if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
			serviceLabels[k] = v
		}
	}
	// Add any additional labels from spec, excluding any that would conflict with selector
	if m.Spec.Deployment.Labels != nil {
		for k, v := range m.Spec.Deployment.Labels {
			if k != labelApp && k != labelHttpbinCr { // Exclude selector labels
				serviceLabels[k] = v
			}
		}
	}

	annotations := map[string]string{
		"dns.gardener.cloud/dnsnames": r.getDNSName(m),
		"dns.gardener.cloud/ttl":      "600",
		"dns.gardener.cloud/class":    "garden",
	}

	// By default assume that HttpBin lives at a subdomain. If domain is
	// set HttpBins do not get a separate subdomain so the name is part
	// of the URI.
	path := "/"
	pathType := networkingv1.PathTypePrefix
	if *fBaseDomain == "" {
		path = fmt.Sprintf("/%s/(.*)", m.Name)
		pathType = networkingv1.PathTypeImplementationSpecific
		annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/$1"
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.getResourceName(m),
			Namespace:   svc.Namespace,
			Labels:      serviceLabels,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: r.getDNSName(m),
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: svc.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: svc.Spec.Ports[0].Port,
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

	if *fTlsSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{r.getDNSName(m)},
				SecretName: *fTlsSecretName,
			},
		}
	}

	if *fIngressClassName != "" {
		ingress.Spec.IngressClassName = fIngressClassName
	}

	return ingress
}

// SetupWithManager sets up the controller with the Manager.
func (r *HttpBinDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&orchestratev1alpha1.HttpBinDeployment{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
