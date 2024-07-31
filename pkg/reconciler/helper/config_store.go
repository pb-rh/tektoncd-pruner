package helper

import (
	"context"
	"sync"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/ptr"
)

type PrunerResourceSpec struct {
	TTLSecondsAfterFinished *int32
	SuccessfulHistoryLimit  *int32
	FailedHistoryLimit      *int32
	Pipelines               []tektonprunerv1alpha1.ResourceSpec
	Tasks                   []tektonprunerv1alpha1.ResourceSpec
}

func cloneResource(srcResources []tektonprunerv1alpha1.ResourceSpec) []tektonprunerv1alpha1.ResourceSpec {
	if len(srcResources) > 0 {
		clonedResources := make([]tektonprunerv1alpha1.ResourceSpec, len(srcResources))
		for index, resource := range srcResources {
			clonedResources[index] = *resource.DeepCopy()
		}
		return clonedResources
	}
	return []tektonprunerv1alpha1.ResourceSpec{}
}

func (prs *PrunerResourceSpec) Clone() *PrunerResourceSpec {
	clonedObj := PrunerResourceSpec{}
	if prs.TTLSecondsAfterFinished != nil {
		clonedObj.TTLSecondsAfterFinished = ptr.Int32(*prs.TTLSecondsAfterFinished)
	}
	clonedObj.Pipelines = cloneResource(prs.Pipelines)
	clonedObj.Tasks = cloneResource(prs.Tasks)
	return &clonedObj
}

type PrunerSpec struct {
	TTLSecondsAfterFinished *int32
	SuccessfulHistoryLimit  *int32
	FailedHistoryLimit      *int32
	Namespaces              map[string]PrunerResourceSpec
}

func (ps *PrunerSpec) Clone() *PrunerSpec {
	clonedPrunerSpec := PrunerSpec{}
	if ps.TTLSecondsAfterFinished != nil {
		clonedPrunerSpec.TTLSecondsAfterFinished = ptr.Int32(*ps.TTLSecondsAfterFinished)
	}

	return &clonedPrunerSpec
}

type _prunerStore struct {
	mutex          sync.RWMutex
	defaultSpec    PrunerSpec
	namespacedSpec PrunerSpec
}

var (
	PrunerConfigStore = _prunerStore{mutex: sync.RWMutex{}}
)

func (ps *_prunerStore) LoadOnStartup(ctx context.Context, client kubernetes.Interface, namespace string) error {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	// logger := logging.FromContext(ctx)

	defaultTTL := DefaultTTLSeconds
	defaultSpec := &PrunerSpec{
		TTLSecondsAfterFinished: &defaultTTL,
	}

	cfgMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, DefaultConfigMapName, v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if cfgMap.Data != nil && cfgMap.Data[DefaultConfigKey] != "" {
		err = yaml.Unmarshal([]byte(cfgMap.Data[DefaultConfigKey]), defaultSpec)
		if err != nil {
			return err
		}
	}

	ps.defaultSpec = *defaultSpec.Clone()

	if ps.defaultSpec.Namespaces == nil {
		ps.defaultSpec.Namespaces = map[string]PrunerResourceSpec{}
	}

	if ps.namespacedSpec.Namespaces == nil {
		ps.namespacedSpec.Namespaces = map[string]PrunerResourceSpec{}
	}

	ps.syncWithNamespacedSpec()
	return nil
}

func (ps *_prunerStore) syncWithNamespacedSpec() {
	defaultSpec := ps.defaultSpec.Clone()
	if ps.namespacedSpec.TTLSecondsAfterFinished == nil {
		ps.namespacedSpec.TTLSecondsAfterFinished = defaultSpec.TTLSecondsAfterFinished
	}
	for namespace, defaultSpec := range defaultSpec.Namespaces {
		namespacedSpec, found := ps.namespacedSpec.Namespaces[namespace]
		if !found {
			ps.namespacedSpec.Namespaces[namespace] = *defaultSpec.Clone()
		} else {
			finalSpec := ps.internalMergeResources(*defaultSpec.Clone(), *namespacedSpec.Clone())
			ps.namespacedSpec.Namespaces[namespace] = finalSpec
		}
	}
}

func (ps *_prunerStore) internalMergeResources(defaultSpec, namespacedSpec PrunerResourceSpec) PrunerResourceSpec {
	finalSpec := PrunerResourceSpec{
		TTLSecondsAfterFinished: defaultSpec.TTLSecondsAfterFinished,
	}
	if namespacedSpec.TTLSecondsAfterFinished != nil {
		finalSpec.TTLSecondsAfterFinished = namespacedSpec.TTLSecondsAfterFinished
	}

	// merge pipelines
	pipelines := defaultSpec.Pipelines
	for _, pipelineNS := range namespacedSpec.Pipelines {
		found := false
		for index, pipelineDefault := range pipelines {
			if pipelineDefault.Name == pipelineNS.Name {
				found = true
				pipelines[index] = pipelineNS
				continue
			}
		}
		if !found {
			pipelines = append(pipelines, pipelineNS)
		}
	}
	finalSpec.Pipelines = pipelines

	// merge tasks
	tasks := defaultSpec.Tasks
	for _, taskNS := range namespacedSpec.Tasks {
		found := false
		for index, taskDefault := range tasks {
			if taskDefault.Name == taskNS.Name {
				found = true
				tasks[index] = taskNS
				continue
			}
		}
		if !found {
			tasks = append(tasks, taskNS)
		}
	}
	finalSpec.Tasks = tasks

	return finalSpec
}

func (ps *_prunerStore) UpdateSpec(prunerCR *tektonprunerv1alpha1.TektonPruner) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	namespace := prunerCR.Namespace

	// update in the local store
	namespacedSpec := PrunerResourceSpec{
		TTLSecondsAfterFinished: prunerCR.Spec.TTLSecondsAfterFinished,
		Pipelines:               prunerCR.Spec.Pipelines,
		Tasks:                   prunerCR.Spec.Tasks,
	}

	defaultSpec, found := ps.defaultSpec.Namespaces[namespace]
	if found {
		ps.namespacedSpec.Namespaces[namespace] = *namespacedSpec.Clone()
		return
	}

	finalSpec := ps.internalMergeResources(*defaultSpec.Clone(), *namespacedSpec.Clone())
	ps.namespacedSpec.Namespaces[namespace] = finalSpec

}

func (ps *_prunerStore) DeleteSpec(namespace string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	delete(ps.namespacedSpec.Namespaces, namespace)
}

func (ps *_prunerStore) GetPipelineTTL(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	resourceSpec, found := ps.namespacedSpec.Namespaces[namespace]
	if !found {
		return ps.namespacedSpec.TTLSecondsAfterFinished
	}
	for _, pipeline := range resourceSpec.Pipelines {
		if pipeline.Name == name {
			return pipeline.TTLSecondsAfterFinished
		}
	}
	if resourceSpec.TTLSecondsAfterFinished != nil {
		return resourceSpec.TTLSecondsAfterFinished
	}
	return ps.namespacedSpec.TTLSecondsAfterFinished
}

func (ps *_prunerStore) GetTaskTTL(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	resourceSpec, found := ps.namespacedSpec.Namespaces[namespace]
	if !found {
		return ps.namespacedSpec.TTLSecondsAfterFinished
	}
	for _, task := range resourceSpec.Tasks {
		if task.Name == name {
			return task.TTLSecondsAfterFinished
		}
	}
	if resourceSpec.TTLSecondsAfterFinished != nil {
		return resourceSpec.TTLSecondsAfterFinished
	}
	return ps.namespacedSpec.TTLSecondsAfterFinished
}

func (ps *_prunerStore) GetPipelineSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	resourceSpec, found := ps.namespacedSpec.Namespaces[namespace]
	if !found {
		return ps.namespacedSpec.SuccessfulHistoryLimit
	}
	for _, pipeline := range resourceSpec.Pipelines {
		if pipeline.Name == name {
			return pipeline.SuccessfulHistoryLimit
		}
	}
	if resourceSpec.SuccessfulHistoryLimit != nil {
		return resourceSpec.SuccessfulHistoryLimit
	}
	return ps.namespacedSpec.SuccessfulHistoryLimit
}

func (ps *_prunerStore) GetPipelineFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	resourceSpec, found := ps.namespacedSpec.Namespaces[namespace]
	if !found {
		return ps.namespacedSpec.FailedHistoryLimit
	}
	for _, pipeline := range resourceSpec.Pipelines {
		if pipeline.Name == name {
			return pipeline.FailedHistoryLimit
		}
	}
	if resourceSpec.FailedHistoryLimit != nil {
		return resourceSpec.FailedHistoryLimit
	}
	return ps.namespacedSpec.FailedHistoryLimit
}
