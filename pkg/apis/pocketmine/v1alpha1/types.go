package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Plugin specifies that a plugin should be installed in servers that share the same `pocketmine-server` label.
type Plugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PluginSpec   `json:"spec"`
	Status PluginStatus `json:"status,omitempty"`
}

// PluginSpec is the spec for a Plugin resource.
type PluginSpec struct {
	// Source is the method to install the plugin.
	Source PluginSource `json:"source"`
	// DependencyPolicy specifies the behavior when a plugin has a missing dependency.
	// +kubebuilder:default=AutoCreate
	// +optional
	DependencyPolicy DependencyPolicy `json:"dependencyPolicy"`
}

// PluginSource indicates the method to install a plugin.
// Only either Http or Data should be nonempty.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:MaxProperties=1
type PluginSource struct {
	// Http is the HTTP URL to download the plugin from.
	// +optional
	Http *PluginHttpSource `json:"http,omitempty"`
	// Data is the raw plugin phar contents.
	// +optional
	Data []byte `json:"data,omitempty"`
}

// PluginHttpSource indicates that a plugin should be installed from an HTTP/HTTPS URL.
type PluginHttpSource struct {
	// Url is the HTTP/HTTPS URL to download the plugin from.
	// +kubebuilder:validation:Format=uri
	Url string `json:"url"`
	// TimeoutSeconds is the number of seconds to wait for the plugin to download before timing out.
	// +optional
	// +kubebuilder:default=60
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`
}

// +kubebuilder:validation:Enum=AutoCreate;FailOnMissing
type DependencyPolicy string

const (
	// DependencyPolicyAutoCreate indicates that the PluginSource objects for dependency plugins should be automatically created.
	DependencyPolicyAutoCreate DependencyPolicy = "AutoCreate"
	// DependencyPolicyAutoCreate indicates that failure events should be emitted when a plugin has a missing dependency.
	DependencyPolicyFailOnMissing DependencyPolicy = "FailOnMissing"
)

// PluginStatus is the status for a Plugin resource.
type PluginStatus struct {
	// ExpectedChecksum is the crc32 checksum of the plugin.
	// If the value is nil, the plugin has not been installed on any hosts yet.
	// Reset this value to nil to force update the plugin.
	// +optional
	ExpectedChecksum *uint32 `json:"expectedChecksum,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Plugin `json:"items"`
}
