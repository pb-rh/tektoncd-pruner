package helper

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

type HistoryLimitResourceFuncs interface {
	Type() string
	Delete(ctx context.Context, namespace, name string) error
	List(ctx context.Context, namespace, label string) ([]metav1.Object, error)
	GetFailedHistoryLimitCount(resource metav1.Object) *int32
	GetSuccessHistoryLimitCount(resource metav1.Object) *int32
	IsSuccessful(resource metav1.Object) bool
	IsFailed(resource metav1.Object) bool
}

type HistoryLimiter struct {
	resourceFn HistoryLimitResourceFuncs
}

func NewHistoryLimiter(name string, resourceFn HistoryLimitResourceFuncs) (*HistoryLimiter, error) {
	hl := &HistoryLimiter{
		resourceFn: resourceFn,
	}
	if hl.resourceFn == nil {
		return nil, fmt.Errorf("resourceFunc interface can not be nil")
	}

	return hl, nil
}

func (hl *HistoryLimiter) ProcessEvent(ctx context.Context, resource metav1.Object) error {
	logger := logging.FromContext(ctx)
	logger.Debugw("processing an event", "resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName())

	if hl.resourceFn.IsSuccessful(resource) {
		return hl.doSuccessfulResourceCleanup(ctx, resource)
	}

	if hl.resourceFn.IsFailed(resource) {
		return hl.doFailedResourceCleanup(ctx, resource)
	}

	return nil
}

func (hl *HistoryLimiter) doSuccessfulResourceCleanup(ctx context.Context, resource metav1.Object) error {
	// get success history limit count
	var successfulHistoryLimit *int32

	// TODO: get label selector
	label := ""

	return hl.doResourceCleanup(ctx, resource, successfulHistoryLimit, label)
}

func (hl *HistoryLimiter) doFailedResourceCleanup(ctx context.Context, resource metav1.Object) error {
	// get failed history limit count
	var failedHistoryLimit *int32

	// TODO: get label selector
	label := ""

	return hl.doResourceCleanup(ctx, resource, failedHistoryLimit, label)
}

func (hl *HistoryLimiter) doResourceCleanup(ctx context.Context, resource metav1.Object, historyLimit *int32, label string) error {
	logger := logging.FromContext(ctx)

	if historyLimit == nil {
		return nil
	}

	resources, err := hl.resourceFn.List(ctx, resource.GetNamespace(), label)
	if err != nil {
		return err
	}

	if int(*historyLimit) > len(resources) {
		return nil
	}

	sort.Slice(resources, func(i, j int) bool {
		// TODO: do nil check?
		return resources[i].GetCreationTimestamp().After(resources[j].GetCreationTimestamp().Time)
	})

	var selectionForDeletion []metav1.Object

	if *historyLimit == 0 {
		// remove all the history
		selectionForDeletion = resources
	} else {
		// TODO: fix the order correctly
		selectionForDeletion = resources[*historyLimit:]
	}

	for _, resource := range selectionForDeletion {
		err := hl.resourceFn.Delete(ctx, resource.GetNamespace(), resource.GetName())
		if err != nil {
			logger.Errorw("error on removing a resource",
				"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
				err,
			)
		}
	}

	return nil
}
