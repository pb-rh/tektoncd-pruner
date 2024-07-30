package pipelinerun

import (
	"context"

	"go.uber.org/zap"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/pipelinerun"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"k8s.io/utils/clock"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	pipelineRunInformer := pipelineruninformer.Get(ctx)

	logger := logging.FromContext(ctx)

	pipelineRunFuncs := &PipelineRunFuncs{
		client: pipelineclient.Get(ctx),
	}
	ttlQueue, err := helper.NewTTLQueue("pipeline_runs_to_delete", clock.RealClock{}, 5, pipelineRunFuncs)
	if err != nil {
		logger.Fatal("error on getting ttl queue", zap.Error(err))
	}

	historyLimiter, err := helper.NewHistoryLimiter("pipeline_runs_to_delete", pipelineRunFuncs)
	if err != nil {
		logger.Fatal("error on getting history limiter", zap.Error(err))
	}

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient:     kubeclient.Get(ctx),
		ttlQueue:       ttlQueue,
		historyLimiter: historyLimiter,
	}

	ctrlOptions := controller.Options{
		FinalizerName: "pruner_ttl_controller",
	}

	impl := pipelinerunreconciler.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options { return ctrlOptions })

	// number of works to process the events
	impl.Concurrency = 1

	// Listen for events on the main resource and enqueue themselves.
	pipelineRunInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	// start ttl queue workers
	go r.ttlQueue.StartWorkers(ctx)

	return impl
}
