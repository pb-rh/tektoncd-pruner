package helper

import (
	"os"
	"strconv"
)

const (
	EnvSystemNamespace                 = "SYSTEM_NAMESPACE"
	EnvTTLConcurrentWorkersPipelineRun = "TTL_CONCURRENT_WORKERS_PIPELINE_RUN"
	EnvTTLConcurrentWorkersTaskRun     = "TTL_CONCURRENT_WORKERS_TASK_RUN"

	LabelPipelineName    = "tekton.dev/pipeline"
	LabelPipelineRunName = "tekton.dev/pipelineRun"
	LabelTaskName        = "tekton.dev/task"
	LabelTaskRunName     = "tekton.dev/taskRun"

	KindPipelineRun = "PipelineRun"
	KindTaskRun     = "TaskRun"

	AnnotationTTLSecondsAfterFinished    = "pruner.tekton.dev/ttlSecondsAfterFinished"
	AnnotationResourceNameLabelKey       = "pruner.tekton.dev/resourceNameLabelKey"
	AnnotationSuccessfulHistoryLimit     = "pruner.tekton.dev/successfulHistoryLimit"
	AnnotationFailedHistoryLimit         = "pruner.tekton.dev/failedHistoryLimit"
	AnnotationHistoryLimitCheckProcessed = "pruner.tekton.dev/historyLimitCheckProcessed"

	// name of the config map to hold pruner global config data
	PrunerConfigMapName = "tekton-pruner-default-spec"
	// name of the key to fetch global config data
	PrunerGlobalConfigKey = "global-config"

	// number of workers on PipelineRun controller
	DefaultTTLConcurrentWorkersPipelineRun = int(5)
	// number of workers on TaskRun controller
	DefaultTTLConcurrentWorkersTaskRun = int(5)
)

func GetEnvValueAsInt(envKey string, defaultValue int) (int, error) {
	strValue := os.Getenv(envKey)
	if strValue == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(strValue)
}
