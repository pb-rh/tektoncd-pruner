/*
Copyright 2024 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

const (
	TektonPrunerConditionReady = apis.ConditionReady
)

// TektonPrunerSpec defines the desired state of TektonPruner
type TektonPrunerSpec struct {
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
	// +optional
	SuccessfulHistoryLimit *int32 `json:"successfulHistoryLimit,omitempty"`
	// +optional
	FailedHistoryLimit *int32 `json:"failedHistoryLimit,omitempty"`
	// +optional
	Pipelines []ResourceSpec `json:"pipelines,omitempty"`
	// +optional
	Tasks []ResourceSpec `json:"tasks,omitempty"`
}

type ResourceSpec struct {
	Name string `json:"name"`
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
	// +optional
	SuccessfulHistoryLimit *int32 `json:"successfulHistoryLimit,omitempty"`
	// +optional
	FailedHistoryLimit *int32 `json:"failedHistoryLimit,omitempty"`
}

// TektonPrunerStatus defines the observed state of TektonPruner
type TektonPrunerStatus struct {
	duckv1.Status `json:",inline"`
}

// TektonPruner is the Schema for the tektonpruners API
//
// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
type TektonPruner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TektonPrunerSpec   `json:"spec,omitempty"`
	Status TektonPrunerStatus `json:"status,omitempty"`
}

// TektonPrunerList contains a list of TektonPruner
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TektonPrunerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TektonPruner `json:"items"`
}

// GetStatus retrieves the status of the resource. Implements the KRShaped interface.
func (tr *TektonPruner) GetStatus() *duckv1.Status {
	return &tr.Status.Status
}
