package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClientset creates a Kubernetes clientset based on the provided kubeconfig path.
// If kubeconfigPath is empty, it attempts in-cluster configuration first and falls
// back to the default loading rules (~/.kube/config).
func NewClientset(kubeconfigPath string) (*kubernetes.Clientset, error) {
	cfg, err := buildConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(cfg)
}

func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	// Try in-cluster configuration first.
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	// Fallback to default kubeconfig loading rules if not running in cluster.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
}

// PrintHPAStats fetches and prints basic HPA metrics to stdout.
func PrintHPAStats(ctx context.Context, clientset *kubernetes.Clientset, namespace, hpaName string) error {
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}

	var hpas *autoscalingv2.HorizontalPodAutoscalerList
	var err error

	if hpaName != "" {
		hpa, getErr := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, hpaName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("getting HPA %s/%s: %w", namespace, hpaName, getErr)
		}
		hpas = &autoscalingv2.HorizontalPodAutoscalerList{Items: []autoscalingv2.HorizontalPodAutoscaler{*hpa}}
	} else {
		hpas, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("listing HPAs: %w", err)
		}
	}

	if len(hpas.Items) == 0 {
		fmt.Println("No HorizontalPodAutoscalers found.")
		return nil
	}

	sort.Slice(hpas.Items, func(i, j int) bool {
		hi, hj := hpas.Items[i], hpas.Items[j]
		if hi.Namespace == hj.Namespace {
			return hi.Name < hj.Name
		}
		return hi.Namespace < hj.Namespace
	})

	fmt.Println("NAMESPACE\tNAME\tDESIRED\tCURRENT\tMETRICS")
	for i := range hpas.Items {
		hpa := &hpas.Items[i]
		metricsSummary := summarizeMetrics(hpa)
		fmt.Printf("%s\t%s\t%d\t%d\t%s\n", hpa.Namespace, hpa.Name, hpa.Status.DesiredReplicas, hpa.Status.CurrentReplicas, metricsSummary)
	}

	return nil
}

func summarizeMetrics(hpa *autoscalingv2.HorizontalPodAutoscaler) string {
	if len(hpa.Status.CurrentMetrics) == 0 {
		return "-"
	}

	targets := buildMetricTargetLookup(hpa.Spec.Metrics)

	parts := make([]string, 0, len(hpa.Status.CurrentMetrics))
	for _, metric := range hpa.Status.CurrentMetrics {
		parts = append(parts, describeMetric(metric, targets))
	}

	return strings.Join(parts, "; ")
}

type metricKey struct {
	metricType          autoscalingv2.MetricSourceType
	resourceName        string
	container           string
	metricName          string
	selector            string
	describedAPIVersion string
	describedKind       string
	describedName       string
}

func buildMetricTargetLookup(specs []autoscalingv2.MetricSpec) map[metricKey]autoscalingv2.MetricTarget {
	targets := make(map[metricKey]autoscalingv2.MetricTarget, len(specs))

	for _, spec := range specs {
		switch spec.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if spec.Resource == nil {
				continue
			}
			key := metricKey{
				metricType:   spec.Type,
				resourceName: string(spec.Resource.Name),
			}
			targets[key] = spec.Resource.Target
		case autoscalingv2.PodsMetricSourceType:
			if spec.Pods == nil {
				continue
			}
			key := metricKey{
				metricType: spec.Type,
				metricName: spec.Pods.Metric.Name,
				selector:   selectorString(spec.Pods.Metric.Selector),
			}
			targets[key] = spec.Pods.Target
		case autoscalingv2.ObjectMetricSourceType:
			if spec.Object == nil {
				continue
			}
			key := metricKey{
				metricType:          spec.Type,
				metricName:          spec.Object.Metric.Name,
				selector:            selectorString(spec.Object.Metric.Selector),
				describedAPIVersion: spec.Object.DescribedObject.APIVersion,
				describedKind:       spec.Object.DescribedObject.Kind,
				describedName:       spec.Object.DescribedObject.Name,
			}
			targets[key] = spec.Object.Target
		case autoscalingv2.ExternalMetricSourceType:
			if spec.External == nil {
				continue
			}
			key := metricKey{
				metricType: spec.Type,
				metricName: spec.External.Metric.Name,
				selector:   selectorString(spec.External.Metric.Selector),
			}
			targets[key] = spec.External.Target
		case autoscalingv2.ContainerResourceMetricSourceType:
			if spec.ContainerResource == nil {
				continue
			}
			key := metricKey{
				metricType:   spec.Type,
				resourceName: string(spec.ContainerResource.Name),
				container:    spec.ContainerResource.Container,
			}
			targets[key] = spec.ContainerResource.Target
		}
	}

	return targets
}

func describeMetric(metric autoscalingv2.MetricStatus, targets map[metricKey]autoscalingv2.MetricTarget) string {
	switch metric.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if metric.Resource == nil {
			return "resource: <nil>"
		}

		key := metricKey{
			metricType:   metric.Type,
			resourceName: string(metric.Resource.Name),
		}

		var targetStr string
		if target, ok := targets[key]; ok && target.Type != "" {
			targetStr = formatMetricTarget(target)
		}

		current := formatMetricValueStatus(metric.Resource.Current)
		if targetStr == "" {
			return fmt.Sprintf("resource[%s] current(%s)", metric.Resource.Name, current)
		}
		return fmt.Sprintf("resource[%s] current(%s) target(%s)", metric.Resource.Name, current, targetStr)

	case autoscalingv2.PodsMetricSourceType:
		if metric.Pods == nil {
			return "pods: <nil>"
		}

		key := metricKey{
			metricType: metric.Type,
			metricName: metric.Pods.Metric.Name,
			selector:   selectorString(metric.Pods.Metric.Selector),
		}

		var targetStr string
		if target, ok := targets[key]; ok && target.Type != "" {
			targetStr = formatMetricTarget(target)
		}

		current := formatMetricValueStatus(metric.Pods.Current)
		summary := fmt.Sprintf("pods[%s] current(%s)", metric.Pods.Metric.Name, current)
		if metric.Pods.Metric.Selector != nil {
			summary += fmt.Sprintf(" selector(%s)", selectorString(metric.Pods.Metric.Selector))
		}
		if targetStr != "" {
			summary += fmt.Sprintf(" target(%s)", targetStr)
		}
		return summary

	case autoscalingv2.ObjectMetricSourceType:
		if metric.Object == nil {
			return "object: <nil>"
		}

		key := metricKey{
			metricType:          metric.Type,
			metricName:          metric.Object.Metric.Name,
			selector:            selectorString(metric.Object.Metric.Selector),
			describedAPIVersion: metric.Object.DescribedObject.APIVersion,
			describedKind:       metric.Object.DescribedObject.Kind,
			describedName:       metric.Object.DescribedObject.Name,
		}

		var targetStr string
		if target, ok := targets[key]; ok && target.Type != "" {
			targetStr = formatMetricTarget(target)
		}

		current := formatMetricValueStatus(metric.Object.Current)
		descriptor := fmt.Sprintf("%s/%s/%s", metric.Object.DescribedObject.APIVersion, metric.Object.DescribedObject.Kind, metric.Object.DescribedObject.Name)
		summary := fmt.Sprintf("object[%s @ %s] current(%s)", metric.Object.Metric.Name, descriptor, current)
		if metric.Object.Metric.Selector != nil {
			summary += fmt.Sprintf(" selector(%s)", selectorString(metric.Object.Metric.Selector))
		}
		if targetStr != "" {
			summary += fmt.Sprintf(" target(%s)", targetStr)
		}
		return summary

	case autoscalingv2.ExternalMetricSourceType:
		if metric.External == nil {
			return "external: <nil>"
		}

		key := metricKey{
			metricType: metric.Type,
			metricName: metric.External.Metric.Name,
			selector:   selectorString(metric.External.Metric.Selector),
		}

		var targetStr string
		if target, ok := targets[key]; ok && target.Type != "" {
			targetStr = formatMetricTarget(target)
		}

		current := formatMetricValueStatus(metric.External.Current)
		summary := fmt.Sprintf("external[%s] current(%s)", metric.External.Metric.Name, current)
		if metric.External.Metric.Selector != nil {
			summary += fmt.Sprintf(" selector(%s)", selectorString(metric.External.Metric.Selector))
		}
		if targetStr != "" {
			summary += fmt.Sprintf(" target(%s)", targetStr)
		}
		return summary

	case autoscalingv2.ContainerResourceMetricSourceType:
		if metric.ContainerResource == nil {
			return "container: <nil>"
		}

		key := metricKey{
			metricType:   metric.Type,
			resourceName: string(metric.ContainerResource.Name),
			container:    metric.ContainerResource.Container,
		}

		var targetStr string
		if target, ok := targets[key]; ok && target.Type != "" {
			targetStr = formatMetricTarget(target)
		}

		current := formatMetricValueStatus(metric.ContainerResource.Current)
		if targetStr == "" {
			return fmt.Sprintf("container[%s/%s] current(%s)", metric.ContainerResource.Container, metric.ContainerResource.Name, current)
		}
		return fmt.Sprintf("container[%s/%s] current(%s) target(%s)", metric.ContainerResource.Container, metric.ContainerResource.Name, current, targetStr)

	default:
		return fmt.Sprintf("unknown metric type: %s", metric.Type)
	}
}

func selectorString(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}

	return metav1.FormatLabelSelector(selector)
}

func formatMetricValueStatus(status autoscalingv2.MetricValueStatus) string {
	parts := make([]string, 0, 3)

	if status.Value != nil {
		parts = append(parts, fmt.Sprintf("value=%s", status.Value.String()))
	}

	if status.AverageValue != nil {
		parts = append(parts, fmt.Sprintf("avg=%s", status.AverageValue.String()))
	}

	if status.AverageUtilization != nil {
		parts = append(parts, fmt.Sprintf("avgUtil=%d%%", *status.AverageUtilization))
	}

	if len(parts) == 0 {
		return "-"
	}

	return strings.Join(parts, ",")
}

func formatMetricTarget(target autoscalingv2.MetricTarget) string {
	if target.Type == "" {
		return ""
	}

	parts := []string{fmt.Sprintf("type=%s", target.Type)}

	if target.Value != nil {
		parts = append(parts, fmt.Sprintf("value=%s", target.Value.String()))
	}

	if target.AverageValue != nil {
		parts = append(parts, fmt.Sprintf("avg=%s", target.AverageValue.String()))
	}

	if target.AverageUtilization != nil {
		parts = append(parts, fmt.Sprintf("avgUtil=%d%%", *target.AverageUtilization))
	}

	return strings.Join(parts, ",")
}
