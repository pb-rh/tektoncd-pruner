package main

import (
	// The set of controllers this controller process runs.
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/pipelinerun"
	// "github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/taskrun"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/tektonpruner"

	// This defines the shared main for injected controllers.
	"knative.dev/pkg/injection/sharedmain"
)

func main() {
	sharedmain.Main("controller",
		tektonpruner.NewController,
		pipelinerun.NewController,
		// taskrun.NewController,
	)
}
