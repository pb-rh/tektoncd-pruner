package main

import (
	"log"

	"knative.dev/hack/schema/commands"
	"knative.dev/hack/schema/registry"

	v1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
)

// schema is a tool to dump the schema for TektonPruner resources.
func main() {
	registry.Register(&v1alpha1.TektonPruner{})

	if err := commands.New("github.com/openshift-pipelines/tektoncd-pruner").Execute(); err != nil {
		log.Fatal("Error during command execution: ", err)
	}
}
