package helper

import (
	"sync"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// for internal use
// to manage different resources and different fields
type PrunerResourceType string
type PrunerFieldType string

const (
	PrunerResourceTypePipeline PrunerResourceType = "pipeline"
	PrunerResourceTypeTask     PrunerResourceType = "task"

	PrunerFieldTypeTTLSecondsAfterFinished PrunerFieldType = "ttlSecondsAfterFinished"
	PrunerFieldTypeSuccessfulHistoryLimit  PrunerFieldType = "successfulHistoryLimit"
	PrunerFieldTypeFailedHistoryLimit      PrunerFieldType = "failedHistoryLimit"
)

// used to hold the config of a specific namespace
type PrunerResourceSpec struct {
	TTLSecondsAfterFinished *int32
	SuccessfulHistoryLimit  *int32
	FailedHistoryLimit      *int32
	Pipelines               []tektonprunerv1alpha1.ResourceSpec
	Tasks                   []tektonprunerv1alpha1.ResourceSpec
}

// used to hold the config of namespaces
// and global config
type PrunerConfig struct {
	TTLSecondsAfterFinished *int32
	SuccessfulHistoryLimit  *int32
	FailedHistoryLimit      *int32
	Namespaces              map[string]PrunerResourceSpec
}

// defines the store structure
// holds config from ConfigMap (global config) and config from namespaces (namespaced config)
type prunerConfigStore struct {
	mutex            sync.RWMutex
	globalConfig     PrunerConfig
	namespacedConfig PrunerConfig
}

var (
	// store to manage pruner config
	// singleton instance
	PrunerConfigStore = prunerConfigStore{mutex: sync.RWMutex{}}
)

// loads config from configMap (global-config)
// should be called on startup and if there is a change detected on the ConfigMap
func (ps *prunerConfigStore) LoadGlobalConfig(configMap *corev1.ConfigMap) error {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	globalConfig := &PrunerConfig{}
	if configMap.Data != nil && configMap.Data[PrunerGlobalConfigKey] != "" {
		err := yaml.Unmarshal([]byte(configMap.Data[PrunerGlobalConfigKey]), globalConfig)
		if err != nil {
			return err
		}
	}

	ps.globalConfig = *globalConfig

	if ps.globalConfig.Namespaces == nil {
		ps.globalConfig.Namespaces = map[string]PrunerResourceSpec{}
	}

	if ps.namespacedConfig.Namespaces == nil {
		ps.namespacedConfig.Namespaces = map[string]PrunerResourceSpec{}
	}

	return nil
}

func (ps *prunerConfigStore) UpdateNamespacedSpec(prunerCR *tektonprunerv1alpha1.TektonPruner) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	namespace := prunerCR.Namespace

	// update in the local store
	namespacedSpec := PrunerResourceSpec{
		TTLSecondsAfterFinished: prunerCR.Spec.TTLSecondsAfterFinished,
		Pipelines:               prunerCR.Spec.Pipelines,
		Tasks:                   prunerCR.Spec.Tasks,
	}
	ps.namespacedConfig.Namespaces[namespace] = namespacedSpec
}

func (ps *prunerConfigStore) DeleteNamespacedSpec(namespace string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	delete(ps.namespacedConfig.Namespaces, namespace)
}

func getFromPrunerConfig(spec PrunerConfig, namespace, name string, resourceType PrunerResourceType, fieldType PrunerFieldType) *int32 {
	prunerResourceSpec, found := spec.Namespaces[namespace]
	if !found {
		return nil
	}

	var resourceSpecs []tektonprunerv1alpha1.ResourceSpec

	switch resourceType {
	case PrunerResourceTypePipeline:
		resourceSpecs = prunerResourceSpec.Pipelines

	case PrunerResourceTypeTask:
		resourceSpecs = prunerResourceSpec.Tasks
	}

	for _, resourceSpec := range resourceSpecs {
		if resourceSpec.Name == name {
			switch fieldType {
			case PrunerFieldTypeTTLSecondsAfterFinished:
				return resourceSpec.TTLSecondsAfterFinished

			case PrunerFieldTypeSuccessfulHistoryLimit:
				return resourceSpec.SuccessfulHistoryLimit

			case PrunerFieldTypeFailedHistoryLimit:
				return resourceSpec.FailedHistoryLimit
			}
		}
	}
	return nil
}

func getResourceFieldData(namespacedSpec, globalSpec PrunerConfig, namespace, name string, resourceType PrunerResourceType, fieldType PrunerFieldType) *int32 {
	ttl := getFromPrunerConfig(namespacedSpec, namespace, name, resourceType, fieldType)

	if ttl == nil {
		ttl = getFromPrunerConfig(globalSpec, namespace, name, resourceType, fieldType)
	}

	if ttl == nil {
		spec, found := namespacedSpec.Namespaces[namespace]
		if found {
			switch fieldType {
			case PrunerFieldTypeTTLSecondsAfterFinished:
				ttl = spec.TTLSecondsAfterFinished

			case PrunerFieldTypeSuccessfulHistoryLimit:
				ttl = spec.SuccessfulHistoryLimit

			case PrunerFieldTypeFailedHistoryLimit:
				ttl = spec.FailedHistoryLimit
			}
		}
	}

	if ttl == nil {
		switch fieldType {
		case PrunerFieldTypeTTLSecondsAfterFinished:
			ttl = globalSpec.TTLSecondsAfterFinished

		case PrunerFieldTypeSuccessfulHistoryLimit:
			ttl = globalSpec.SuccessfulHistoryLimit

		case PrunerFieldTypeFailedHistoryLimit:
			ttl = globalSpec.FailedHistoryLimit
		}
	}

	return ttl
}

func (ps *prunerConfigStore) GetPipelineTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeTTLSecondsAfterFinished)
}

func (ps *prunerConfigStore) GetPipelineSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeSuccessfulHistoryLimit)
}

func (ps *prunerConfigStore) GetPipelineFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeFailedHistoryLimit)
}

func (ps *prunerConfigStore) GetTaskTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeTTLSecondsAfterFinished)
}

func (ps *prunerConfigStore) GetTaskSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeSuccessfulHistoryLimit)
}

func (ps *prunerConfigStore) GetTaskFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeFailedHistoryLimit)
}
