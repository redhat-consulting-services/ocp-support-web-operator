package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SupportGatherSpec struct {
	// GatherTypes specifies which must-gather types to run.
	// Use ["all"] to run all detected operators, or specify individual types
	// like ["virtualization", "odf", "acm"]. Default must-gather always runs.
	// +optional
	GatherTypes []string `json:"gatherTypes,omitempty"`

	// Namespaces limits the gather to specific namespaces (custom gather mode).
	// If set, only these namespaces are collected instead of the full cluster gather.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// ResourceTypes specifies which resource types to collect in custom gather mode.
	// Only used when Namespaces is set.
	// +optional
	ResourceTypes []string `json:"resourceTypes,omitempty"`

	// IncludeLogs controls whether pod logs are collected in custom gather mode.
	// +optional
	IncludeLogs *bool `json:"includeLogs,omitempty"`

	// Anonymize enables data anonymization in the gathered archive.
	// +optional
	Anonymize bool `json:"anonymize,omitempty"`

	// Since limits log collection to this time window (e.g. "6h", "24h", "48h").
	// +optional
	Since string `json:"since,omitempty"`

	// Upload configures automatic upload to Red Hat support after gather completes.
	// +optional
	Upload *UploadSpec `json:"upload,omitempty"`
}

type UploadSpec struct {
	// CaseID is the Red Hat support case number to attach the archive to.
	CaseID string `json:"caseID"`

	// SecretRef references a Secret containing "username" and "password" keys
	// for Red Hat's SFTP server (sftp.access.redhat.com).
	SecretRef corev1.LocalObjectReference `json:"secretRef"`

	// InternalUser routes uploads to the internal Red Hat path (/case-mgmt/)
	// instead of the external path (/incoming/).
	// +optional
	InternalUser bool `json:"internalUser,omitempty"`
}

type SupportGatherStatus struct {
	// Phase is the current state: Pending, Gathering, Uploading, Complete, Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// JobID is the backend gather job ID.
	// +optional
	JobID string `json:"jobID,omitempty"`

	// Progress is the gather completion percentage (0-100).
	// +optional
	Progress int `json:"progress,omitempty"`

	// FileName is the name of the generated archive.
	// +optional
	FileName string `json:"fileName,omitempty"`

	// UploadStatus reports the upload result if upload was configured.
	// +optional
	UploadStatus string `json:"uploadStatus,omitempty"`

	// Error contains the error message if the gather or upload failed.
	// +optional
	Error string `json:"error,omitempty"`

	// CompletionTime is when the gather (and upload) finished.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Conditions provide detailed status information.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Case",type=string,JSONPath=`.spec.upload.caseID`,priority=1
// +kubebuilder:printcolumn:name="Progress",type=integer,JSONPath=`.status.progress`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type SupportGather struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupportGatherSpec   `json:"spec,omitempty"`
	Status SupportGatherStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type SupportGatherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupportGather `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SupportGather{}, &SupportGatherList{})
}
