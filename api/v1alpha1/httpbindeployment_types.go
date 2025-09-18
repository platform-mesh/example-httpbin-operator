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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceConfig defines the configuration for the HttpBin service
type ServiceConfig struct {
	// Name is the name of the service. If not provided, the HttpBinDeployment name will be used.
	// +optional
	Name string `json:"name,omitempty"`

	// Type determines how the Service is exposed. Defaults to ClusterIP.
	// Valid values are:
	// - "ClusterIP": Exposes the service on a cluster-internal IP (default)
	// - "NodePort": Exposes the service on each Node's IP at a static port
	// - "LoadBalancer": Exposes the service externally using a cloud provider's load balancer
	// +optional
	// +kubebuilder:default=ClusterIP
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	Type string `json:"type,omitempty"`

	// Port is the port on which the service will listen
	// +optional
	// +kubebuilder:default=80
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Annotations to be added to the service
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DeploymentConfig defines the configuration for the HttpBin deployment
type DeploymentConfig struct {
	// Name is the name of the deployment. If not provided, the HttpBinDeployment name will be used.
	// +optional
	Name string `json:"name,omitempty"`

	// Replicas is the number of desired replicas
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// Annotations to be added to the deployment and pods
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to be added to the deployment and pods
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// HttpBinDeploymentSpec defines the desired state of HttpBinDeployment
type HttpBinDeploymentSpec struct {
	// Service defines the service configuration for the HttpBin deployment
	// +optional
	Service ServiceConfig `json:"service,omitempty"`

	// Deployment defines the deployment configuration for HttpBin
	// +optional
	Deployment DeploymentConfig `json:"deployment,omitempty"`
}

// HttpBinDeploymentStatus defines the observed state of HttpBinDeployment
type HttpBinDeploymentStatus struct {
	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReadyReplicas is the number of pods targeted by this HttpBin deployment with a Ready Condition
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// URL is the HTTPS URL for accessing the httpbin service
	// Format: https://HOST
	// +optional
	// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
	URL string `json:"url,omitempty"`

	// IsDeploymentReady indicates if the deployment is ready (all replicas are available)
	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isDeploymentReady"
	IsDeploymentReady bool `json:"isDeploymentReady,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Service Type",type="string",JSONPath=".spec.service.type"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.service.port"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HttpBinDeployment is the Schema for the httpbindeployments API
type HttpBinDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HttpBinDeploymentSpec   `json:"spec,omitempty"`
	Status HttpBinDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HttpBinDeploymentList contains a list of HttpBinDeployment
type HttpBinDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HttpBinDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HttpBinDeployment{}, &HttpBinDeploymentList{})
}
