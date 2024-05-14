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
	"regexp"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	pkgcorev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/utils/ptr"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var scheme = runtime.NewScheme()

const templateString = "template"

func init() {
	utilruntime.Must(pkgcorev1.AddToScheme(scheme))
}

// GetPodTemplateSpec extracts a Kueue-compatible PodTemplateSpec at the given path within obj
func GetPodTemplateSpec(obj *unstructured.Unstructured, path string) (*v1.PodTemplateSpec, error) {
	candidatePTS, err := GetRawTemplate(obj.UnstructuredContent(), path)
	if err != nil {
		return nil, err
	}

	// Extract the PodSpec that should be at candidatePTS.spec
	podTemplate := &v1.PodTemplate{}
	spec, ok := candidatePTS["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("content at %v does not contain a spec", path)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(spec, &podTemplate.Template.Spec, true); err != nil {
		return nil, fmt.Errorf("content at %v.spec not parseable as a v1.PodSpec: %w", path, err)
	}

	// Set default values. Required for proper operation of Kueue's ComparePodSetSlices
	scheme.Default(podTemplate)

	// Copy in the subset of the metadate expected by Kueye.
	if metadata, ok := candidatePTS["metadata"].(map[string]interface{}); ok {
		if labels, ok := metadata["labels"].(map[string]string); ok {
			podTemplate.Template.ObjectMeta.Labels = labels
		}
		if annotations, ok := metadata["annotations"].(map[string]string); ok {
			podTemplate.Template.ObjectMeta.Annotations = annotations
		}
	}

	return &podTemplate.Template, nil
}

// GetReplicas parses the value at the given path within obj as an int
func GetReplicas(obj *unstructured.Unstructured, path string) (int32, error) {
	value, err := getValueAtPath(obj.UnstructuredContent(), path)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case int:
		return int32(v), nil
	case int32:
		return v, nil
	case int64:
		return int32(v), nil
	default:
		return 0, fmt.Errorf("at path position '%v' non-int value %v", path, value)
	}
}

// return the subobject found at the given path, or nil if the path is invalid
func GetRawTemplate(obj map[string]interface{}, path string) (map[string]interface{}, error) {
	value, err := getValueAtPath(obj, path)
	if err != nil {
		return nil, err
	}
	if asMap, ok := value.(map[string]interface{}); ok {
		return asMap, nil
	} else {
		return nil, fmt.Errorf("at path position '%v' non-map value", path)
	}
}

// get the value found at the given path or an error if the path is invalid
func getValueAtPath(obj map[string]interface{}, path string) (interface{}, error) {
	processed := templateString
	if !strings.HasPrefix(path, processed) {
		return nil, fmt.Errorf("first element of the path must be 'template'")
	}
	remaining := strings.TrimPrefix(path, processed)
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
			processed += remaining[0:end]
			remaining = remaining[end+1:]
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

	return cursor, nil
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

// inferReplicas parses the value at the given path within obj as an int or return 1 or error
func inferReplicas(obj map[string]interface{}, path string) (int32, error) {
	if path == "" {
		// no path specified, default to one replica
		return 1, nil
	}

	// check obj is well formed
	index := strings.LastIndex(path, ".")
	if index >= 0 {
		var err error
		obj, err = GetRawTemplate(obj, path[:index])
		if err != nil {
			return 0, err
		}
	}

	// check type and value
	switch v := obj[path[index+1:]].(type) {
	case nil:
		return 1, nil // default to 1
	case int:
		return int32(v), nil
	case int32:
		return v, nil
	case int64:
		return int32(v), nil
	default:
		return 0, fmt.Errorf("at path position '%v' non-int value %v", path, v)
	}
}

// where to find a replica count and a PodTemplateSpec in a resource
type resourceTemplate struct {
	path     string // path to pod template spec
	replicas string // path to replica count
}

// map from known GVKs to resource templates
var templatesForGVK = map[schema.GroupVersionKind][]resourceTemplate{
	{Group: "", Version: "v1", Kind: "Pod"}:             {{path: "template"}},
	{Group: "apps", Version: "v1", Kind: "Deployment"}:  {{path: "template.spec.template", replicas: "template.spec.replicas"}},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"}: {{path: "template.spec.template", replicas: "template.spec.replicas"}},
}

// inferPodSets infers PodSets for RayJobs and RayClusters
func inferRayPodSets(obj *unstructured.Unstructured, clusterSpecPrefix string) ([]workloadv1beta2.AppWrapperPodSet, error) {
	podSets := []workloadv1beta2.AppWrapperPodSet{}

	podSets = append(podSets, workloadv1beta2.AppWrapperPodSet{Replicas: ptr.To(int32(1)), Path: clusterSpecPrefix + "headGroupSpec.template"})
	if workers, err := getValueAtPath(obj.UnstructuredContent(), clusterSpecPrefix+"workerGroupSpecs"); err == nil {
		if workers, ok := workers.([]interface{}); ok {
			for i := range workers {
				workerGroupSpecPrefix := fmt.Sprintf(clusterSpecPrefix+"workerGroupSpecs[%v].", i)
				// validate path to replica template
				if _, err := getValueAtPath(obj.UnstructuredContent(), workerGroupSpecPrefix+templateString); err == nil {
					// infer replica count
					replicas, err := inferReplicas(obj.UnstructuredContent(), workerGroupSpecPrefix+"replicas")
					if err != nil {
						return nil, err
					}
					podSets = append(podSets, workloadv1beta2.AppWrapperPodSet{Replicas: ptr.To(replicas), Path: workerGroupSpecPrefix + templateString})
				}
			}
		}
	}
	return podSets, nil
}

// InferPodSets infers PodSets for known GVKs
func InferPodSets(obj *unstructured.Unstructured) ([]workloadv1beta2.AppWrapperPodSet, error) {
	gvk := obj.GroupVersionKind()
	podSets := []workloadv1beta2.AppWrapperPodSet{}

	switch gvk {
	case schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}:
		var replicas int32 = 1
		if parallelism, err := GetReplicas(obj, "template.spec.parallelism"); err == nil {
			replicas = parallelism
		}
		if completions, err := GetReplicas(obj, "template.spec.completions"); err == nil && completions < replicas {
			replicas = completions
		}
		podSets = append(podSets, workloadv1beta2.AppWrapperPodSet{Replicas: ptr.To(replicas), Path: "template.spec.template"})

	case schema.GroupVersionKind{Group: "kubeflow.org", Version: "v1", Kind: "PyTorchJob"}:
		for _, replicaType := range []string{"Master", "Worker"} {
			prefix := "template.spec.pytorchReplicaSpecs." + replicaType + "."
			// validate path to replica template
			if _, err := getValueAtPath(obj.UnstructuredContent(), prefix+templateString); err == nil {
				// infer replica count
				replicas, err := inferReplicas(obj.UnstructuredContent(), prefix+"replicas")
				if err != nil {
					return nil, err
				}
				podSets = append(podSets, workloadv1beta2.AppWrapperPodSet{Replicas: ptr.To(replicas), Path: prefix + templateString})
			}
		}

	case schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: "RayCluster"}:
		rayPodSets, err := inferRayPodSets(obj, "template.spec.")
		if err != nil {
			return nil, err
		}
		podSets = append(podSets, rayPodSets...)

	case schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: "RayJob"}:
		rayPodSets, err := inferRayPodSets(obj, "template.spec.rayClusterSpec.")
		if err != nil {
			return nil, err
		}
		podSets = append(podSets, rayPodSets...)

	default:
		for _, template := range templatesForGVK[gvk] {
			// validate path to template
			if _, err := getValueAtPath(obj.UnstructuredContent(), template.path); err == nil {
				replicas, err := inferReplicas(obj.UnstructuredContent(), template.replicas)
				// infer replica count
				if err != nil {
					return nil, err
				}
				podSets = append(podSets, workloadv1beta2.AppWrapperPodSet{Replicas: ptr.To(replicas), Path: template.path})
			}
		}
	}

	return podSets, nil
}

// ValidatePodSets compares declared and inferred PodSets for known GVKs
func ValidatePodSets(obj *unstructured.Unstructured, podSets []workloadv1beta2.AppWrapperPodSet) error {
	declared := map[string]workloadv1beta2.AppWrapperPodSet{}

	// construct a map with declared PodSets and find duplicates
	for _, p := range podSets {
		if _, ok := declared[p.Path]; ok {
			return fmt.Errorf("duplicate PodSets with path '%v'", p.Path)
		}
		declared[p.Path] = p
	}

	// infer PodSets
	inferred, err := InferPodSets(obj)
	if err != nil {
		return err
	}

	// nothing inferred, nothing to validate
	if len(inferred) == 0 {
		return nil
	}

	// compare PodSet counts
	if len(inferred) != len(declared) {
		return fmt.Errorf("PodSet count %v differs from expected count %v", len(declared), len(inferred))
	}

	// match inferred PodSets to declared PodSets
	for _, ips := range inferred {
		dps, ok := declared[ips.Path]
		if !ok {
			return fmt.Errorf("PodSet with path '%v' is missing", ips.Path)
		}

		ipr := ptr.Deref(ips.Replicas, 1)
		dpr := ptr.Deref(dps.Replicas, 1)
		if ipr != dpr {
			return fmt.Errorf("replica count %v differs from expected count %v for PodSet at path position '%v'", dpr, ipr, ips.Path)
		}
	}

	return nil
}

var labelRegex = regexp.MustCompile(`[^-_.\w]`)

// SanitizeLabel sanitizes a string for use as a label
func SanitizeLabel(label string) string {
	// truncate to max length
	if len(label) > 63 {
		label = label[0:63]
	}
	// replace invalid characters with underscores
	label = labelRegex.ReplaceAllString(label, "_")
	// trim non-alphanumeric characters at both ends
	label = strings.Trim(label, "-_.")
	return label
}
