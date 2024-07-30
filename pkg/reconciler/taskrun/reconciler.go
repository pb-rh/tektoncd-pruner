package taskrun

import (
	"context"

	"k8s.io/client-go/kubernetes"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const (
	AnnotationPipelineRunName = "tekton.dev/pipelineRun"
)

// Reconciler implements simpledeploymentreconciler.Interface for
// SimpleDeployment resources.
type Reconciler struct {
	kubeclient kubernetes.Interface
}

// Check that our Reconciler implements Interface
var _ taskrunreconciler.Interface = (*Reconciler)(nil)

func (r *Reconciler) FinalizeKind(ctx context.Context, tr *v1.TaskRun) reconciler.Event {
	logger := logging.FromContext(ctx)

	logger.Infow("received a TaskRun deletion event",
		"Name", tr.Name,
		"Namespace", tr.Namespace,
	)

	if !isStandaloneTaskRun(tr) {
		return nil
	}

	return nil
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, tr *v1.TaskRun) reconciler.Event {
	// This logger has all the context necessary to identify which resource is being reconciled.
	logger := logging.FromContext(ctx)

	logger.Infow("received a TaskRun update event",
		"Name", tr.Name,
		"Namespace", tr.Namespace,
	)

	if !isStandaloneTaskRun(tr) {
		return nil
	}

	return nil
}
