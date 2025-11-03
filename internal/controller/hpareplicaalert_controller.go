package controller

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1alpha1 "github.com/zvikanaparstek/hpa-stats/internal/api/v1alpha1"
)

const (
	annotationKeyManagedBy = "ops.zvikanaparstek.dev/managed-by"
	annotationValueManaged = "annotation"
	annotationKeyTarget    = "ops.zvikanaparstek.dev/target-hpa"
)

// HPAReplicaAlertReconciler reconciles a HPAReplicaAlert CR.
type HPAReplicaAlertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile implements the main reconciliation loop.
func (r *HPAReplicaAlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var alert apiv1alpha1.HPAReplicaAlert
	if err := r.Get(ctx, req.NamespacedName, &alert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the referenced HPA.
	if alert.Spec.TargetRef.Kind != "HorizontalPodAutoscaler" {
		logger.Info("unsupported target kind", "kind", alert.Spec.TargetRef.Kind)
		return ctrl.Result{}, nil
	}
	targetNamespace := alert.Spec.TargetRef.Namespace
	if targetNamespace == "" {
		targetNamespace = alert.Namespace
	}

	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: targetNamespace,
		Name:      alert.Spec.TargetRef.Name,
	}, &hpa); err != nil {
		logger.Error(err, "retrieving target HPA")
		return ctrl.Result{}, err
	}

	currentReplicas := hpa.Status.CurrentReplicas
	var lastObserved int32
	if alert.Status.LastObservedReplicas != nil {
		lastObserved = *alert.Status.LastObservedReplicas
	}

	if alert.Status.LastObservedReplicas == nil || lastObserved != currentReplicas {
		logger.Info("HPA replica change detected",
			"alert", req.NamespacedName.String(),
			"target", fmt.Sprintf("%s/%s", targetNamespace, alert.Spec.TargetRef.Name),
			"previous", lastObserved,
			"current", currentReplicas,
		)

		alert.Status.LastObservedReplicas = &currentReplicas
		now := metav1.Now()
		alert.Status.LastTransitionTime = &now
		if err := r.Status().Update(ctx, &alert); err != nil {
			logger.Error(err, "updating alert status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager wires the controller into controller-runtime manager.
func (r *HPAReplicaAlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapFunc := func(ctx context.Context, obj client.Object) []reconcile.Request {
		hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
		if !ok {
			return nil
		}

		var alerts apiv1alpha1.HPAReplicaAlertList
		if err := r.Client.List(ctx, &alerts, client.InNamespace(hpa.Namespace)); err != nil {
			log.FromContext(ctx).Error(err, "listing alerts for HPA notify")
			return nil
		}

		requests := make([]reconcile.Request, 0, len(alerts.Items))
		for _, alert := range alerts.Items {
			targetNamespace := alert.Spec.TargetRef.Namespace
			if targetNamespace == "" {
				targetNamespace = alert.Namespace
			}

			if alert.Spec.TargetRef.Kind == "HorizontalPodAutoscaler" &&
				targetNamespace == hpa.Namespace && alert.Spec.TargetRef.Name == hpa.Name {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: alert.Namespace,
					Name:      alert.Name,
				}})
			}
		}

		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.HPAReplicaAlert{}).
		Watches(&autoscalingv2.HorizontalPodAutoscaler{}, handler.EnqueueRequestsFromMapFunc(mapFunc)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
