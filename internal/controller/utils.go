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

// getPodTemplateSpec parses raw as JSON and extracts a Kueue-compatible PodTemplateSpec at the given path within it
func getPodTemplateSpec(raw []byte, path string) (*v1.PodTemplateSpec, error) {
	obj := &unstructured.Unstructured{}
	if _, _, err := unstructured.UnstructuredJSONScheme.Decode(raw, nil, obj); err != nil {
		return nil, err
	}

	// Walk down the path
	parts := strings.Split(path, ".")
	p := obj.UnstructuredContent()
	var ok bool
	for i := 1; i < len(parts); i++ {
		p, ok = p[parts[i]].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path element %v not found (segment %v of %v)", parts[i], i, len(parts))
		}
	}

	// Extract the PodSpec that should be at spec
	candidatePTS := p
	spec, ok := candidatePTS["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("content at %v does not contain a spec", path)
	}
	var template v1.PodTemplateSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(spec, &template.Spec, true); err != nil {
		return nil, fmt.Errorf("content at %v.spec not parseable as a v1.PodSpec: %w", path, err)
	}

	// Add in the annotations and labels (if any)
	if metadata, ok := candidatePTS["metadata"].(map[string]interface{}); ok {
		if labels, ok := metadata["labels"].(map[string]string); ok {
			template.ObjectMeta.Labels = labels
		}
		if annotations, ok := metadata["annotations"].(map[string]string); ok {
			template.ObjectMeta.Annotations = annotations
		}
	}

	return &template, nil
}
