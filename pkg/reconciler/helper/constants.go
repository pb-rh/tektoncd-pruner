package helper

const (
	LabelPipelineName    = "tekton.dev/pipeline"
	LabelPipelineRunName = "tekton.dev/pipelineRun"
	LabelTaskName        = "tekton.dev/task"
	LabelTaskRunName     = "tekton.dev/taskRun"

	AnnotationTTLSecondsAfterFinished = "pruner.tekton.dev/ttlSecondsAfterFinished"

	DefaultTTLSeconds    = int32(600) // 60 * 10 = 10 minutes
	DefaultConfigMapName = "tekton-pruner-default-spec"
	DefaultConfigKey     = "spec"
)
