package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// HPAReplicaAlertSpec defines the desired behavior of the alert.
type HPAReplicaAlertSpec struct {
	// TargetRef references the HorizontalPodAutoscaler to monitor.
	TargetRef ObjectReference `json:"targetRef"`
}

// HPAReplicaAlertStatus reports observed replica counts.
type HPAReplicaAlertStatus struct {
	LastObservedReplicas *int32       `json:"lastObservedReplicas,omitempty"`
	LastTransitionTime   *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HPAReplicaAlert is the Schema for the alerts API.
type HPAReplicaAlert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HPAReplicaAlertSpec   `json:"spec,omitempty"`
	Status HPAReplicaAlertStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HPAReplicaAlertList contains a list of HPAReplicaAlert.
type HPAReplicaAlertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HPAReplicaAlert `json:"items"`
}

// ObjectReference holds a reference to a Kubernetes object.
type ObjectReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// DeepCopyInto copies the receiver into the provided out parameter.
func (in *HPAReplicaAlertSpec) DeepCopyInto(out *HPAReplicaAlertSpec) {
	*out = *in
	out.TargetRef = in.TargetRef
}

// DeepCopy creates a new deep copy of the receiver.
func (in *HPAReplicaAlertSpec) DeepCopy() *HPAReplicaAlertSpec {
	if in == nil {
		return nil
	}
	out := new(HPAReplicaAlertSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HPAReplicaAlertStatus) DeepCopyInto(out *HPAReplicaAlertStatus) {
	*out = *in
	if in.LastObservedReplicas != nil {
		out.LastObservedReplicas = new(int32)
		*out.LastObservedReplicas = *in.LastObservedReplicas
	}
	if in.LastTransitionTime != nil {
		out.LastTransitionTime = in.LastTransitionTime.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *HPAReplicaAlertStatus) DeepCopy() *HPAReplicaAlertStatus {
	if in == nil {
		return nil
	}
	out := new(HPAReplicaAlertStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HPAReplicaAlert) DeepCopyInto(out *HPAReplicaAlert) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = HPAReplicaAlertStatus{}
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the receiver.
func (in *HPAReplicaAlert) DeepCopy() *HPAReplicaAlert {
	if in == nil {
		return nil
	}
	out := new(HPAReplicaAlert)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject satisfies the runtime.Object interface.
func (in *HPAReplicaAlert) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *HPAReplicaAlertList) DeepCopyInto(out *HPAReplicaAlertList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]HPAReplicaAlert, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *HPAReplicaAlertList) DeepCopy() *HPAReplicaAlertList {
	if in == nil {
		return nil
	}
	out := new(HPAReplicaAlertList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject satisfies the runtime.Object interface.
func (in *HPAReplicaAlertList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *ObjectReference) DeepCopyInto(out *ObjectReference) {
	*out = *in
}

// DeepCopy creates a new deep copy of the receiver.
func (in *ObjectReference) DeepCopy() *ObjectReference {
	if in == nil {
		return nil
	}
	out := new(ObjectReference)
	in.DeepCopyInto(out)
	return out
}
