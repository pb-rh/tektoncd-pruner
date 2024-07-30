package tektonpruner

import (
	"context"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"

	tektonprunerinformer "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/injection/informers/tektonpruner/v1alpha1/tektonpruner"
	tektonprunerreconciler "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/injection/reconciler/tektonpruner/v1alpha1/tektonpruner"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	tektonPrunerInformer := tektonprunerinformer.Get(ctx)

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient: kubeclient.Get(ctx),
	}
	impl := tektonprunerreconciler.NewImpl(ctx, r)

	// Listen for events on the main resource and enqueue themselves.
	tektonPrunerInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	// load pruner store
	// TODO: update the namespace dynamically
	helper.PrunerConfigStore.LoadOnStartup(ctx, r.kubeclient, "test")

	return impl
}
