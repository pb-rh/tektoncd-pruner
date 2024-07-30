package taskrun

import (
	"context"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/taskrun"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	taskRunInformer := taskruninformer.Get(ctx)

	logger := logging.FromContext(ctx)

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient: kubeclient.Get(ctx),
	}
	impl := taskrunreconciler.NewImpl(ctx, r)

	// number of works to process the events
	impl.Concurrency = 1

	// Listen for events on the main resource and enqueue themselves.
	taskRunInformer.Informer().AddEventHandler(controller.HandleAll(filterTaskRun(logger, impl)))

	return impl
}

func filterTaskRun(logger *zap.SugaredLogger, impl *controller.Impl) func(obj interface{}) {
	return func(obj interface{}) {
		taskRun, err := kmeta.DeletionHandlingAccessor(obj)
		if err != nil {
			logger.Errorw("error on enqueue", zap.Error(err))
			return
		}

		if !isStandaloneTaskRun(taskRun) {
			return
		}

		impl.EnqueueKey(types.NamespacedName{Namespace: taskRun.GetNamespace(), Name: taskRun.GetName()})
	}
}

func isStandaloneTaskRun(taskRun metav1.Object) bool {
	// verify the taskRun is not part of a pipelineRun
	if taskRun.GetLabels() != nil && taskRun.GetLabels()[helper.LabelPipelineRunName] != "" {
		return false
	}
	return true
}
