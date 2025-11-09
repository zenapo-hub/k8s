package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1alpha1 "github.com/zvikanaparstek/hpa-stats/internal/api/v1alpha1"
)

// StatusCheckerReconciler evaluates HPAReplicaAlert status history and reports direction changes.
type StatusCheckerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
}

// Reconcile inspects the alert's status history and logs the most recent scaling direction.
func (r *StatusCheckerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger().WithValues("alert", req.NamespacedName.String())

	var alert apiv1alpha1.HPAReplicaAlert
	if err := r.Get(ctx, req.NamespacedName, &alert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	summary := summarizeAlert(&alert)

	logger.Info("HPA scaling state evaluated",
		"target", summary.Target,
		"currentReplicas", summary.CurrentReplicas,
		"direction", summary.Direction,
		"latestRecordedAt", summary.LatestRecordedAt,
		"eventsRecorded", summary.EventsRecorded,
		"lastTransitionTime", summary.LastTransitionTime,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler into the controller-runtime manager.
func (r *StatusCheckerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.HPAReplicaAlert{}).
		Complete(r)
}

func (r *StatusCheckerReconciler) logger() logr.Logger {
	if r.Logger.IsZero() {
		return log.Log.WithName("status-checker")
	}
	return r.Logger
}

type alertSummary struct {
	Target             string
	CurrentReplicas    int32
	Direction          string
	LatestRecordedAt   *time.Time
	LastTransitionTime *time.Time
	EventsRecorded     int
}

func summarizeAlert(alert *apiv1alpha1.HPAReplicaAlert) alertSummary {
	summary := alertSummary{
		Target:         fmt.Sprintf("%s/%s", alert.Spec.TargetRef.Namespace, alert.Spec.TargetRef.Name),
		EventsRecorded: len(alert.Status.RecentEvents),
	}

	if alert.Status.LastObservedReplicas != nil {
		summary.CurrentReplicas = *alert.Status.LastObservedReplicas
	}
	if alert.Status.LastTransitionTime != nil {
		t := alert.Status.LastTransitionTime.Time
		summary.LastTransitionTime = &t
	}

	if len(alert.Status.RecentEvents) == 0 {
		summary.Direction = string(ScalingInconclusive)
		return summary
	}

	last := alert.Status.RecentEvents[len(alert.Status.RecentEvents)-1]
	summary.Direction = last.Direction
	if last.LastScaleTime != nil {
		t := last.LastScaleTime.Time
		summary.LatestRecordedAt = &t
	} else {
		t := last.RecordedAt.Time
		summary.LatestRecordedAt = &t
	}

	return summary
}

// ScalingDirection mirrors the controller's direction values for reference.
type ScalingDirection string

const (
	ScalingStable       ScalingDirection = "stable"
	ScalingUp           ScalingDirection = "scaling-up"
	ScalingDown         ScalingDirection = "scaling-down"
	ScalingInconclusive ScalingDirection = "inconclusive"
)
