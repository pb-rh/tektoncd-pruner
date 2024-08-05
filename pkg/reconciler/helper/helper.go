package helper

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// common functions used across history limiter and ttl handler

func getResourceNameLabelKey(resource metav1.Object, defaultLabelKey string) string {
	annotations := resource.GetAnnotations()
	// update user defined label key
	if len(annotations) > 0 && annotations[AnnotationResourceNameLabelKey] != "" {
		defaultLabelKey = annotations[AnnotationResourceNameLabelKey]
	}

	return defaultLabelKey
}

func getResourceName(resource metav1.Object, labelKey string) string {
	labels := resource.GetLabels()
	// if there is no label present, no option to filter
	if len(labels) == 0 {
		return ""
	}

	// get label value
	return labels[labelKey]
}
