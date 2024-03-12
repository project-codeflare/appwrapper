/*
Copyright 2024 IBM Corporation.

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

package controller

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// GetPodTemplateSpec extracts a Kueue-compatible PodTemplateSpec at the given path within obj
func GetPodTemplateSpec(obj *unstructured.Unstructured, path string) (*v1.PodTemplateSpec, error) {
	candidatePTS, err := getRawTemplate(obj.UnstructuredContent(), path)
	if err != nil {
		return nil, err
	}

	// Extract the PodSpec that should be at candidatePTS.spec
	spec, ok := candidatePTS["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("content at %v does not contain a spec", path)
	}
	podSpec := &v1.PodSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(spec, podSpec, true); err != nil {
		return nil, fmt.Errorf("content at %v.spec not parseable as a v1.PodSpec: %w", path, err)
	}

	// Construct the filtered PodTemplateSpec, copying only the metadata expected by Kueue
	template := &v1.PodTemplateSpec{Spec: *podSpec}
	if metadata, ok := candidatePTS["metadata"].(map[string]interface{}); ok {
		if labels, ok := metadata["labels"].(map[string]string); ok {
			template.ObjectMeta.Labels = labels
		}
		if annotations, ok := metadata["annotations"].(map[string]string); ok {
			template.ObjectMeta.Annotations = annotations
		}
	}

	return template, nil
}

// return the subobject found at the given path, or nil if the path is invalid
func getRawTemplate(obj map[string]interface{}, path string) (map[string]interface{}, error) {
	parts := strings.Split(path, ".")
	if parts[0] != "template" {
		return nil, fmt.Errorf("first element of the path must be 'template'")
	}
	var ok bool
	for i := 1; i < len(parts); i++ {
		obj, ok = obj[parts[i]].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path element '%v' not found", parts[i])
		}
	}
	return obj, nil
}
