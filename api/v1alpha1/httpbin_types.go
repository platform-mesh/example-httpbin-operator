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

// HttpBinConditionType represents a condition type for HttpBin
type HttpBinConditionType string

// HttpBinConditionReason represents a reason for a condition's last transition
type HttpBinConditionReason string

const (
	// HttpBinConditionTypeReady represents whether the HttpBin resource is fully available.
	HttpBinConditionTypeReady = "Ready"

	// HttpBinConditionReasonDeploymentReady means the Deployment is available and serving.
	HttpBinConditionReasonDeploymentReady = "DeploymentReady"

	// HttpBinConditionReasonDeploymentProgressing means the Deployment exists but is not yet available.
	HttpBinConditionReasonDeploymentProgressing = "DeploymentProgressing"

	// HttpBinConditionReasonDeploymentFailed means the Deployment could not be created.
	HttpBinConditionReasonDeploymentFailed = "DeploymentFailed"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HttpBinSpec defines the desired state of HttpBin
type HttpBinSpec struct {
	// EnableHTTPS determines if HTTPS should be enabled
	// If true, service port will be 8443
	// If false or not set, service port will be 443
	// +optional
	// +kubebuilder:default=false
	EnableHTTPS bool `json:"enableHTTPS,omitempty"`

	// Foo is an example field of HttpBin. Edit httpbin_types.go to remove/update
	Foo string `json:"foo,omitempty"`
	// Region can be used to filter which HttpBin should be served
	Region string `json:"region,omitempty"`
}

// HttpBinStatus defines the observed state of HttpBin
type HttpBinStatus struct {
	// Conditions represent the latest available observations of an object's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// URL is the HTTPS URL for accessing the httpbin service
	// Format: https://HOST
	// +optional
	// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
	URL string `json:"url,omitempty"`

	// Ready indicates if the underlying HttpBinDeployment's deployment is ready
	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HttpBin is the Schema for the httpbins API
type HttpBin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HttpBinSpec   `json:"spec,omitempty"`
	Status HttpBinStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HttpBinList contains a list of HttpBin
type HttpBinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HttpBin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HttpBin{}, &HttpBinList{})
}
