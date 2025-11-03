package controller

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1alpha1 "github.com/zvikanaparstek/hpa-stats/internal/api/v1alpha1"
)

const (
	annotationKeyEnable   = "ops.zvikanaparstek.dev/replica-alert"
	annotationValueEnable = "enabled"
)

// HPAAnnotationReconciler reconciles HPAs to manage alerts based on annotations.
type HPAAnnotationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile ensures that an HPAReplicaAlert exists when the annotation is present and removes it when absent.
func (r *HPAAnnotationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &hpa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	annotations := hpa.GetAnnotations()
	enabled := annotations[annotationKeyEnable] == annotationValueEnable

	var alert apiv1alpha1.HPAReplicaAlert
	err := r.Get(ctx, req.NamespacedName, &alert)
	if enabled {
		if apierrors.IsNotFound(err) {
			newAlert := apiv1alpha1.HPAReplicaAlert{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hpa.Name,
					Namespace: hpa.Namespace,
					Annotations: map[string]string{
						annotationKeyManagedBy: annotationValueManaged,
						annotationKeyTarget:    fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
					},
				},
				Spec: apiv1alpha1.HPAReplicaAlertSpec{
					TargetRef: apiv1alpha1.ObjectReference{
						APIVersion: autoscalingv2.SchemeGroupVersion.String(),
						Kind:       "HorizontalPodAutoscaler",
						Name:       hpa.Name,
						Namespace:  hpa.Namespace,
					},
				},
			}

			if err := controllerutil.SetControllerReference(&hpa, &newAlert, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.Create(ctx, &newAlert); err != nil {
				return ctrl.Result{}, err
			}

			logger.Info("created HPAReplicaAlert from annotation", "hpa", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}

		updated := false
		if alert.Annotations == nil {
			alert.Annotations = map[string]string{}
		}
		if alert.Annotations[annotationKeyManagedBy] != annotationValueManaged {
			alert.Annotations[annotationKeyManagedBy] = annotationValueManaged
			updated = true
		}
		desiredTarget := apiv1alpha1.ObjectReference{
			APIVersion: autoscalingv2.SchemeGroupVersion.String(),
			Kind:       "HorizontalPodAutoscaler",
			Name:       hpa.Name,
			Namespace:  hpa.Namespace,
		}
		if alert.Spec.TargetRef != desiredTarget {
			alert.Spec.TargetRef = desiredTarget
			updated = true
		}
		if alert.Annotations[annotationKeyTarget] != fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name) {
			alert.Annotations[annotationKeyTarget] = fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name)
			updated = true
		}

		if updated {
			if err := r.Update(ctx, &alert); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("updated HPAReplicaAlert due to annotation", "hpa", req.NamespacedName)
		}

		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if alert.Annotations[annotationKeyManagedBy] == annotationValueManaged {
		if err := r.Delete(ctx, &alert); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("deleted HPAReplicaAlert after annotation removal", "hpa", req.NamespacedName)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler into the manager.
func (r *HPAAnnotationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}
