package pipelinerun

import (
	"context"
	"fmt"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Reconciler
type Reconciler struct {
	kubeclient     kubernetes.Interface
	ttlQueue       *helper.TTLQueue
	historyLimiter *helper.HistoryLimiter
}

// Check that our Reconciler implements Interface
var _ pipelinerunreconciler.Interface = (*Reconciler)(nil)

func (r *Reconciler) FinalizeKind(ctx context.Context, pr *v1.PipelineRun) reconciler.Event {
	r.ttlQueue.RemoveFromQueue(ctx, pr.GetNamespace(), pr.GetName())
	return nil
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, pr *v1.PipelineRun) reconciler.Event {
	// This logger has all the context necessary to identify which resource is being reconciled.
	logger := logging.FromContext(ctx)

	logger.Infow("received an PipelineRun event",
		"Name", pr.Name,
		"Namespace", pr.Namespace,
	)

	// perform ttl
	err := r.ttlQueue.ProcessEvent(ctx, pr)
	if err != nil {
		logger.Errorw("error on processing ttl for a pipelineRun",
			"Namespace", pr.Namespace,
			"Name", pr.Name,
			zap.Error(err),
		)
	}

	// perform history limit
	err = r.historyLimiter.ProcessEvent(ctx, pr)
	if err != nil {
		logger.Errorw("error on processing history limiting for a pipelineRun",
			"Namespace", pr.Namespace,
			"Name", pr.Name,
			zap.Error(err),
		)
	}
	return err
}

type PipelineRunFuncs struct {
	client pipelineversioned.Interface
}

func (plf *PipelineRunFuncs) Type() string {
	return "PipelineRun"
}

func (plf *PipelineRunFuncs) List(ctx context.Context, namespace, label string) ([]metav1.Object, error) {
	// TODO: should we have to implement pagination support?
	prsList, err := plf.client.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return nil, err
	}

	prs := []metav1.Object{}
	for _, pr := range prsList.Items {
		prs = append(prs, pr.DeepCopy())
	}
	return prs, nil
}

func (plf *PipelineRunFuncs) Get(ctx context.Context, namespace, name string) (metav1.Object, error) {
	return plf.client.TektonV1().PipelineRuns(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (plf *PipelineRunFuncs) Delete(ctx context.Context, namespace, name string) error {
	return plf.client.TektonV1().PipelineRuns(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (plf *PipelineRunFuncs) Update(ctx context.Context, resource metav1.Object) error {
	pr, ok := resource.(*v1.PipelineRun)
	if !ok {
		return fmt.Errorf("invalid type received. Namespace:%s, Name:%s", resource.GetNamespace(), resource.GetName())
	}
	_, err := plf.client.TektonV1().PipelineRuns(resource.GetNamespace()).Update(ctx, pr, metav1.UpdateOptions{})
	return err
}

func (plf *PipelineRunFuncs) GetCompletionTime(resource metav1.Object) (metav1.Time, error) {
	pr, ok := resource.(*v1.PipelineRun)
	if !ok {
		return metav1.Time{}, fmt.Errorf("resource type error, this is not a PipelineRun resource. Namespace:%s, Name:%s", resource.GetNamespace(), resource.GetName())
	}
	if pr.Status.CompletionTime != nil {
		return *pr.Status.CompletionTime, nil
	}
	for _, c := range pr.Status.Conditions {
		if c.Type == apis.ConditionSucceeded && c.Status != corev1.ConditionUnknown {
			finishAt := c.LastTransitionTime
			if finishAt.Inner.IsZero() {
				return metav1.Time{}, fmt.Errorf("unable to find the time when the resource %s/%s finished", pr.Namespace, pr.Name)
			}
			return c.LastTransitionTime.Inner, nil
		}
	}

	// This should never happen if the Resource has finished
	return metav1.Time{}, fmt.Errorf("unable to find the status of the finished Resource %s/%s", pr.Namespace, pr.Name)
}

func (plf *PipelineRunFuncs) Ignore(resource metav1.Object) bool {
	// labels and annotations are not populated, lets wait sometime
	if resource.GetLabels() == nil {
		if resource.GetAnnotations() == nil || resource.GetAnnotations()[helper.AnnotationTTLSecondsAfterFinished] == "" {
			return true
		}
	}
	return false
}

func (plf *PipelineRunFuncs) GetTTL(resource metav1.Object) *int32 {
	pipelineName := plf.getPipelineName(resource)
	return helper.PrunerConfigStore.GetPipelineTTL(resource.GetNamespace(), pipelineName)
}

func (plf *PipelineRunFuncs) getPipelineName(resource metav1.Object) string {
	// get the pipeline name
	labels := resource.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	pipelineName, ok := labels[helper.LabelPipelineName]
	if !ok || pipelineName == "" {
		// if pipeline name not found, use pipelineRun name
		pipelineName = resource.GetName()
	}
	return pipelineName
}

func (plf *PipelineRunFuncs) IsCompleted(resource metav1.Object) bool {
	pr, ok := resource.(*v1.PipelineRun)
	if !ok {
		return false
	}
	if pr.IsPending() {
		return false
	}
	return true
}

func (plf *PipelineRunFuncs) IsSuccessful(resource metav1.Object) bool {
	pr, ok := resource.(*v1.PipelineRun)
	if !ok {
		return false
	}

	if pr.IsPending() {
		return false
	}

	condition := pr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil {
		return false
	}

	runReason := v1.PipelineRunReason(condition.Reason)

	if runReason == v1.PipelineRunReasonSuccessful || runReason == v1.PipelineRunReasonCompleted {
		return true
	}

	return false
}

func (plf *PipelineRunFuncs) IsFailed(resource metav1.Object) bool {
	pr, ok := resource.(*v1.PipelineRun)
	if !ok {
		return false
	}

	if pr.IsPending() {
		return false
	}

	return !plf.IsSuccessful(resource)
}

func (plf *PipelineRunFuncs) GetSuccessHistoryLimitCount(resource metav1.Object) *int32 {
	pipelineName := plf.getPipelineName(resource)
	return helper.PrunerConfigStore.GetPipelineSuccessHistoryLimitCount(resource.GetNamespace(), pipelineName)
}

func (plf *PipelineRunFuncs) GetFailedHistoryLimitCount(resource metav1.Object) *int32 {
	pipelineName := plf.getPipelineName(resource)
	return helper.PrunerConfigStore.GetPipelineFailedHistoryLimitCount(resource.GetNamespace(), pipelineName)
}
