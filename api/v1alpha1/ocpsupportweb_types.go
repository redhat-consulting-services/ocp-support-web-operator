package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OCPSupportWebSpec struct {
	// +optional
	Image string `json:"image,omitempty"`

	// +optional
	AgentImage string `json:"agentImage,omitempty"`

	// +optional
	OAuthProxyImage string `json:"oauthProxyImage,omitempty"`

	// +optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// +optional
	Route *RouteSpec `json:"route,omitempty"`

	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	OAuthProxyResources *corev1.ResourceRequirements `json:"oauthProxyResources,omitempty"`

	// AllowedGroups is a list of OpenShift groups allowed to access the application.
	// These are enforced by the OAuth proxy. If empty, defaults to ["cluster-admins"].
	// +optional
	AllowedGroups []string `json:"allowedGroups,omitempty"`
}

type RouteSpec struct {
	// +optional
	Host string `json:"host,omitempty"`
}

type OCPSupportWebStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`

	// +optional
	RouteURL string `json:"routeURL,omitempty"`

	// +optional
	AppImage string `json:"appImage,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.routeURL`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type OCPSupportWeb struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OCPSupportWebSpec   `json:"spec,omitempty"`
	Status OCPSupportWebStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type OCPSupportWebList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCPSupportWeb `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCPSupportWeb{}, &OCPSupportWebList{})
}
