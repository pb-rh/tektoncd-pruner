package main

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/metrics"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/webhook"
	"knative.dev/pkg/webhook/certificates"
	"knative.dev/pkg/webhook/configmaps"
	"knative.dev/pkg/webhook/resourcesemantics"
	"knative.dev/pkg/webhook/resourcesemantics/defaulting"
	"knative.dev/pkg/webhook/resourcesemantics/validation"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
)

var types = map[schema.GroupVersionKind]resourcesemantics.GenericCRD{
	// List the types to validate.
	v1alpha1.SchemeGroupVersion.WithKind("TektonPruner"): &v1alpha1.TektonPruner{},
}

var callbacks = map[schema.GroupVersionKind]validation.Callback{}

func newDefaultingAdmissionController(name string) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		return defaulting.NewAdmissionController(ctx,

			// Name of the resource webhook.
			name,

			// The path on which to serve the webhook.
			"/defaulting",

			// The resources to default.
			types,

			// A function that infuses the context passed to Validate/SetDefaults with custom metadata.
			func(ctx context.Context) context.Context {
				// Here is where you would infuse the context with state
				// (e.g. attach a store with configmap data)
				return ctx
			},

			// Whether to disallow unknown fields.
			true,
		)
	}
}

func newValidationAdmissionController(name string) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		return validation.NewAdmissionController(ctx,

			// Name of the validation webhook, it is based on the value of the environment variable WEBHOOK_ADMISSION_CONTROLLER_NAME
			// default is "validation.webhook.pruner.tekton.dev"
			fmt.Sprintf("validation.%s", name),

			// The path on which to serve the webhook.
			"/resource-validation",

			// The resources to validate.
			types,

			// A function that infuses the context passed to Validate/SetDefaults with custom metadata.
			func(ctx context.Context) context.Context {
				// Here is where you would infuse the context with state
				// (e.g. attach a store with configmap data)
				return ctx
			},

			// Whether to disallow unknown fields.
			true,

			// Extra validating callbacks to be applied to resources.
			callbacks,
		)
	}
}

func newConfigValidationController(name string) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		return configmaps.NewAdmissionController(ctx,

			// Name of the configmap webhook, it is based on the value of the environment variable WEBHOOK_ADMISSION_CONTROLLER_NAME
			// default is "config.webhook.pruner.tekton.dev"
			fmt.Sprintf("config.%s", name),

			// The path on which to serve the webhook.
			"/config-validation",

			// The configmaps to validate.
			configmap.Constructors{
				logging.ConfigMapName(): logging.NewConfigFromConfigMap,
				metrics.ConfigMapName(): metrics.NewObservabilityConfigFromConfigMap,
			},
		)
	}
}

func main() {

	serviceName := os.Getenv("WEBHOOK_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "tekton-pruner-webhook"
	}

	secretName := os.Getenv("WEBHOOK_SECRET_NAME")
	if secretName == "" {
		secretName = "tekton-pruner-webhook-certs"
	}

	webhookName := os.Getenv("WEBHOOK_ADMISSION_CONTROLLER_NAME")
	if webhookName == "" {
		webhookName = "webhook.pruner.tekton.dev"
	}

	ctx := webhook.WithOptions(signals.NewContext(), webhook.Options{
		ServiceName: serviceName,
		SecretName:  secretName,
		Port:        webhook.PortFromEnv(8443),
	})

	sharedmain.MainWithContext(ctx, serviceName,
		certificates.NewController,
		newDefaultingAdmissionController(webhookName),
		newValidationAdmissionController(webhookName),
		newConfigValidationController(webhookName),
	)
}
