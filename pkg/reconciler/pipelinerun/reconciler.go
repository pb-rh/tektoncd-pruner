package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Reconciler
type Reconciler struct {
	kubeclient     kubernetes.Interface
	ttlHandler     *helper.TTLHandler
	historyLimiter *helper.HistoryLimiter
}

// Check that our Reconciler implements Interface
var _ pipelinerunreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, pr *pipelinev1.PipelineRun) reconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Debugw("received a PipelineRun event",
		"namespace", pr.Namespace, "name", pr.Name,
	)

	// execute the history limiter earlier than the ttl handler

	// execute history limit action
	err := r.historyLimiter.ProcessEvent(ctx, pr)
	if err != nil {
		logger.Errorw("error on processing history limiting for a PipelineRun",
			"namespace", pr.Namespace, "name", pr.Name,
			zap.Error(err),
		)
		return err
	}

	// execute ttl handler
	err = r.ttlHandler.ProcessEvent(ctx, pr)
	if err != nil {
		isRequeueKey, _ := controller.IsRequeueKey(err)
		// the error is not a requeue error, print the error
		if !isRequeueKey {
			data, _ := json.Marshal(pr)
			logger.Errorw("error on processing ttl for a PipelineRun",
				"namespace", pr.Namespace, "name", pr.Name,
				"resource", string(data),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}

type PipelineRunFuncs struct {
	client pipelineversioned.Interface
}

func (prf *PipelineRunFuncs) Type() string {
	return helper.KindPipelineRun
}

func (prf *PipelineRunFuncs) List(ctx context.Context, namespace, label string) ([]metav1.Object, error) {
	// TODO: should we have to implement pagination support?
	prsList, err := prf.client.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return nil, err
	}

	prs := []metav1.Object{}
	for _, pr := range prsList.Items {
		prs = append(prs, pr.DeepCopy())
	}
	return prs, nil
}

func (prf *PipelineRunFuncs) Get(ctx context.Context, namespace, name string) (metav1.Object, error) {
	return prf.client.TektonV1().PipelineRuns(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (prf *PipelineRunFuncs) Delete(ctx context.Context, namespace, name string) error {
	return prf.client.TektonV1().PipelineRuns(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (prf *PipelineRunFuncs) Update(ctx context.Context, resource metav1.Object) error {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return fmt.Errorf("invalid type received. Namespace:%s, Name:%s", resource.GetNamespace(), resource.GetName())
	}
	_, err := prf.client.TektonV1().PipelineRuns(resource.GetNamespace()).Update(ctx, pr, metav1.UpdateOptions{})
	return err
}

func (prf *PipelineRunFuncs) GetCompletionTime(resource metav1.Object) (metav1.Time, error) {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return metav1.Time{}, fmt.Errorf("resource type error, this is not a PipelineRun resource. namespace:%s, name:%s, type:%T",
			resource.GetNamespace(), resource.GetName(), resource)
	}
	if pr.Status.CompletionTime != nil {
		return *pr.Status.CompletionTime, nil
	}
	for _, c := range pr.Status.Conditions {
		if c.Type == apis.ConditionSucceeded && c.Status != corev1.ConditionUnknown {
			finishAt := c.LastTransitionTime
			if finishAt.Inner.IsZero() {
				return metav1.Time{}, fmt.Errorf("unable to find the time when the resource '%s/%s' finished", pr.Namespace, pr.Name)
			}
			return c.LastTransitionTime.Inner, nil
		}
	}

	// This should never happen if the Resource has finished
	return metav1.Time{}, fmt.Errorf("unable to find the status of the finished resource: %s/%s", pr.Namespace, pr.Name)
}

func (prf *PipelineRunFuncs) Ignore(resource metav1.Object) bool {
	// labels and annotations are not populated, lets wait sometime
	if resource.GetLabels() == nil {
		if resource.GetAnnotations() == nil || resource.GetAnnotations()[helper.AnnotationTTLSecondsAfterFinished] == "" {
			return true
		}
	}
	return false
}

func (prf *PipelineRunFuncs) IsCompleted(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return false
	}

	if pr.Status.StartTime == nil {
		return false
	}

	if pr.Status.CompletionTime != nil {
		return true
	}

	if pr.IsPending() {
		return false
	}

	// check the status from conditions
	condition := pr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil || condition.Status == corev1.ConditionUnknown {
		return false
	}

	return true
}

func (prf *PipelineRunFuncs) IsSuccessful(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
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

	runReason := pipelinev1.PipelineRunReason(condition.Reason)

	if runReason == pipelinev1.PipelineRunReasonSuccessful || runReason == pipelinev1.PipelineRunReasonCompleted {
		return true
	}

	return false
}

func (prf *PipelineRunFuncs) IsFailed(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return false
	}

	if pr.IsPending() {
		return false
	}

	return !prf.IsSuccessful(resource)
}

func (prf *PipelineRunFuncs) GetDefaultLabelKey() string {
	return helper.LabelPipelineName
}

func (prf *PipelineRunFuncs) GetTTLSecondsAfterFinished(namespace, pipelineName string) *int32 {
	return helper.PrunerConfigStore.GetPipelineTTLSecondsAfterFinished(namespace, pipelineName)
}

func (prf *PipelineRunFuncs) GetSuccessHistoryLimitCount(namespace, name string) *int32 {
	return helper.PrunerConfigStore.GetPipelineSuccessHistoryLimitCount(namespace, name)
}

func (prf *PipelineRunFuncs) GetFailedHistoryLimitCount(namespace, name string) *int32 {
	return helper.PrunerConfigStore.GetPipelineFailedHistoryLimitCount(namespace, name)
}
