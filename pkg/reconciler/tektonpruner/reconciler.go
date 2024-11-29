package tektonpruner

import (
	"context"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	tektonprunerreconciler "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/injection/reconciler/tektonpruner/v1alpha1/tektonpruner"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Reconciler implements simpledeploymentreconciler.Interface for
// SimpleDeployment resources.
type Reconciler struct {
	kubeclient kubernetes.Interface
}

// Check that our Reconciler implements Interface
var _ tektonprunerreconciler.Interface = (*Reconciler)(nil)

func (r *Reconciler) FinalizeKindOld(ctx context.Context, tknPr *tektonprunerv1alpha1.TektonPruner) reconciler.Event {
	// This logger has all the context necessary to identify which resource is being reconciled.
	logger := logging.FromContext(ctx)
	logger.Infow("received a delete event",
		"namespace", tknPr.Namespace, "name", tknPr.Name, "deletionTimestamp", tknPr.GetDeletionTimestamp(),
	)

	// update spec on the common store
	helper.PrunerConfigStore.DeleteNamespacedSpec(tknPr.Namespace)
	return nil
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, tknPr *tektonprunerv1alpha1.TektonPruner) reconciler.Event {
	// This logger has all the context necessary to identify which resource is being reconciled.
	logger := logging.FromContext(ctx)
	logger.Debugw("received an event",
		"namespace", tknPr.Namespace, "name", tknPr.Name,
	)

	// update spec on the common store
	helper.PrunerConfigStore.UpdateNamespacedSpec(tknPr)

	// mark reconciliation completed and this config is ready to use
	tknPr.Status.MarkReady()

	return nil
}
