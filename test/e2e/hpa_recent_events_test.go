//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	apiv1alpha1 "github.com/zvikanaparstek/hpa-stats/internal/api/v1alpha1"
)

func TestHPAReplicaAlertRecordsRecentEvents(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("set E2E=1 to run cluster end-to-end tests")
	}

	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("loading kubeconfig: %v", err)
	}

	t.Log("connected to cluster; starting setup")

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		autoscalingv2.AddToScheme,
		corev1.AddToScheme,
		appsv1.AddToScheme,
		apiv1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("adding scheme: %v", err)
		}
	}

	kubeClient, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	nsName := "hpa-alert-e2e-" + rand.String(5)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("creating namespace: %v", err)
	}
	t.Logf("created namespace %q", nsName)
	t.Cleanup(func() {
		_ = kubeClient.Delete(context.Background(), namespace)
	})

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-server",
			Namespace: nsName,
			Labels:    map[string]string{"app": "e2e-server"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "e2e-server"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "e2e-server"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "pause",
						Image: "gcr.io/google-containers/pause:3.9",
					}},
				},
			},
		},
	}

	if err := kubeClient.Create(ctx, deployment); err != nil {
		t.Fatalf("creating deployment: %v", err)
	}
	t.Logf("created deployment %s/%s", nsName, deployment.Name)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-hpa",
			Namespace: nsName,
			Annotations: map[string]string{
				"ops.zvikanaparstek.dev/replica-alert": "enabled",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
			},
			MinReplicas: pointer.Int32(1),
			MaxReplicas: 5,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: pointer.Int32(50),
					},
				},
			}},
		},
	}

	if err := kubeClient.Create(ctx, hpa); err != nil {
		t.Fatalf("creating hpa: %v", err)
	}
	t.Logf("created annotated HPA %s/%s", nsName, hpa.Name)

	alertKey := types.NamespacedName{Namespace: nsName, Name: hpa.Name}
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		alert := &apiv1alpha1.HPAReplicaAlert{}
		if err := kubeClient.Get(ctx, alertKey, alert); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		t.Fatalf("waiting for HPAReplicaAlert: %v", err)
	}
	t.Logf("observed HPAReplicaAlert %s/%s", nsName, hpa.Name)

	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: hpa.Name}, hpa); err != nil {
		t.Fatalf("fetching hpa: %v", err)
	}

	now := metav1.Now()
	hpa.Status.CurrentReplicas = 1
	hpa.Status.DesiredReplicas = 3
	hpa.Status.LastScaleTime = &now
	hpa.Status.Conditions = []autoscalingv2.HorizontalPodAutoscalerCondition{{
		Type:               autoscalingv2.ScalingActive,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "E2ETest",
		Message:            "triggered by e2e test",
	}}

	if err := kubeClient.Status().Update(ctx, hpa); err != nil {
		t.Fatalf("updating hpa status: %v", err)
	}
	t.Log("patched HPA status to simulate scaling event")

	var recordedEvent apiv1alpha1.ScalingEvent
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		alert := &apiv1alpha1.HPAReplicaAlert{}
		if err := kubeClient.Get(ctx, alertKey, alert); err != nil {
			return false, err
		}
		if len(alert.Status.RecentEvents) == 0 {
			return false, nil
		}
		recordedEvent = alert.Status.RecentEvents[len(alert.Status.RecentEvents)-1]
		return true, nil
	}); err != nil {
		t.Fatalf("waiting for scaling event: %v", err)
	}

	t.Logf("recorded event: prev=%v current=%d desired=%d direction=%s", recordedEvent.PreviousReplicas, recordedEvent.CurrentReplicas, recordedEvent.DesiredReplicas, recordedEvent.Direction)

	if recordedEvent.CurrentReplicas != 1 {
		t.Fatalf("unexpected current replicas: %d", recordedEvent.CurrentReplicas)
	}
	if recordedEvent.DesiredReplicas != 3 {
		t.Fatalf("unexpected desired replicas: %d", recordedEvent.DesiredReplicas)
	}
	if recordedEvent.Direction != "scaling-up" {
		t.Fatalf("unexpected direction: %s", recordedEvent.Direction)
	}
}
