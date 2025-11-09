package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

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

const (
	recentConditionWindow = 3 * time.Minute
)

const (
	maxRecentEvents = 50
)

type ScalingDirection string

const (
	ScalingStable       ScalingDirection = "stable"
	ScalingUp           ScalingDirection = "scaling-up"
	ScalingDown         ScalingDirection = "scaling-down"
	ScalingInconclusive ScalingDirection = "inconclusive"
)

// HPAReplicaAlertReconciler reconciles a HPAReplicaAlert CR.
type HPAReplicaAlertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func determineScalingDirection(hpa *autoscalingv2.HorizontalPodAutoscaler) ScalingDirection {
	current := hpa.Status.CurrentReplicas
	desired := hpa.Status.DesiredReplicas

	switch {
	case desired > current:
		return ScalingUp
	case desired < current:
		return ScalingDown
	default:
		// Desired == Current. If conditions disagree or HPA lacks desired state, report stable.
		return ScalingStable
	}
}

func summarizeRecentConditions(hpa *autoscalingv2.HorizontalPodAutoscaler, window time.Duration) string {
	if len(hpa.Status.Conditions) == 0 {
		return "no conditions recorded"
	}

	cutoff := time.Now().Add(-window)
	parts := make([]string, 0, len(hpa.Status.Conditions))

	for _, cond := range hpa.Status.Conditions {
		if cond.LastTransitionTime.IsZero() || cond.LastTransitionTime.Time.Before(cutoff) {
			continue
		}

		age := time.Since(cond.LastTransitionTime.Time).Round(time.Second)
		snippet := fmt.Sprintf("%s=%s reason=%s age=%s", cond.Type, cond.Status, cond.Reason, age)
		if cond.Message != "" {
			snippet += fmt.Sprintf(" message=%s", cond.Message)
		}
		parts = append(parts, snippet)
	}

	if len(parts) == 0 {
		return fmt.Sprintf("no condition changes in last %s", window.Round(time.Second))
	}

	return strings.Join(parts, "; ")
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
		lastScaleTime := ""
		if hpa.Status.LastScaleTime != nil {
			lastScaleTime = hpa.Status.LastScaleTime.Time.UTC().Format(time.RFC3339)
		}

		direction := determineScalingDirection(&hpa)
		recentConditions := summarizeRecentConditions(&hpa, recentConditionWindow)
		now := metav1.Now()

		logger.Info("HPA replica change detected",
			"alert", req.NamespacedName.String(),
			"target", fmt.Sprintf("%s/%s", targetNamespace, alert.Spec.TargetRef.Name),
			"previous", lastObserved,
			"current", currentReplicas,
			"desired", hpa.Status.DesiredReplicas,
			"direction", direction,
			"lastScaleTime", lastScaleTime,
			"recentConditions", recentConditions,
		)

		event := apiv1alpha1.ScalingEvent{
			RecordedAt:       now,
			CurrentReplicas:  currentReplicas,
			DesiredReplicas:  hpa.Status.DesiredReplicas,
			Direction:        string(direction),
			ConditionSummary: recentConditions,
		}
		if alert.Status.LastObservedReplicas != nil {
			prev := *alert.Status.LastObservedReplicas
			event.PreviousReplicas = &prev
		}
		if hpa.Status.LastScaleTime != nil {
			event.LastScaleTime = hpa.Status.LastScaleTime.DeepCopy()
		}

		alert.Status.RecentEvents = append(alert.Status.RecentEvents, event)
		if len(alert.Status.RecentEvents) > maxRecentEvents {
			start := len(alert.Status.RecentEvents) - maxRecentEvents
			alert.Status.RecentEvents = append([]apiv1alpha1.ScalingEvent(nil), alert.Status.RecentEvents[start:]...)
		}

		alert.Status.LastObservedReplicas = &currentReplicas
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
