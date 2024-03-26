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

package utils

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

// GetPodTemplateSpec extracts a Kueue-compatible PodTemplateSpec at the given path within obj
func GetPodTemplateSpec(obj *unstructured.Unstructured, path string) (*v1.PodTemplateSpec, error) {
	candidatePTS, err := GetRawTemplate(obj.UnstructuredContent(), path)
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
func GetRawTemplate(obj map[string]interface{}, path string) (map[string]interface{}, error) {
	if !strings.HasPrefix(path, "template") {
		return nil, fmt.Errorf("first element of the path must be 'template'")
	}
	remaining := strings.TrimPrefix(path, "template")
	processed := "template"
	var cursor interface{} = obj

	for remaining != "" {
		if strings.HasPrefix(remaining, "[") {
			// Array index expression
			end := strings.Index(remaining, "]")
			if end < 0 {
				return nil, fmt.Errorf("at path position '%v' invalid array index '%v'", processed, remaining)
			}
			index, err := strconv.Atoi(remaining[1:end])
			if err != nil {
				return nil, fmt.Errorf("at path position '%v' invalid index expression '%v'", processed, remaining[1:end])
			}
			asArray, ok := cursor.([]interface{})
			if !ok {
				return nil, fmt.Errorf("at path position '%v' found non-array value", processed)
			}
			if index < 0 || index >= len(asArray) {
				return nil, fmt.Errorf("at path position '%v' out of bounds index '%v'", processed, index)
			}
			cursor = asArray[index]
			remaining = remaining[end+1:]
			processed += remaining[0:end]
		} else if strings.HasPrefix(remaining, ".") {
			// Field reference expression
			remaining = remaining[1:]
			processed += "."
			end := len(remaining)
			if dotIdx := strings.Index(remaining, "."); dotIdx > 0 {
				end = dotIdx
			}
			if bracketIdx := strings.Index(remaining, "["); bracketIdx > 0 && bracketIdx < end {
				end = bracketIdx
			}
			key := remaining[:end]
			asMap, ok := cursor.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("at path position '%v' non-map value", processed)
			}
			cursor, ok = asMap[key]
			if !ok {
				return nil, fmt.Errorf("at path position '%v' missing field '%v'", processed, key)
			}
			remaining = strings.TrimPrefix(remaining, key)
			processed += key
		} else {
			return nil, fmt.Errorf("at path position '%v' invalid path element '%v'", processed, remaining)
		}
	}

	if asMap, ok := cursor.(map[string]interface{}); ok {
		return asMap, nil
	} else {
		return nil, fmt.Errorf("at path position '%v' non-map value", processed)
	}
}

func Replicas(ps workloadv1beta2.AppWrapperPodSet) int32 {
	if ps.Replicas == nil {
		return 1
	} else {
		return *ps.Replicas
	}
}

func ExpectedPodCount(aw *workloadv1beta2.AppWrapper) int32 {
	var expected int32
	for _, c := range aw.Spec.Components {
		for _, s := range c.PodSets {
			expected += Replicas(s)
		}
	}
	return expected
}
