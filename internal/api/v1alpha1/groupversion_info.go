package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

var (
    GroupVersion = schema.GroupVersion{Group: "ops.zvikanaparstek.dev", Version: "v1alpha1"}

    SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
        scheme.AddKnownTypes(GroupVersion, &HPAReplicaAlert{}, &HPAReplicaAlertList{})
        metav1.AddToGroupVersion(scheme, GroupVersion)
        return nil
    })

    AddToScheme = SchemeBuilder.AddToScheme
)
