/*
Copyright 2015 The Kubernetes Authors.

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

/*
This file contains defaulting functions and supporting code copied from
https://github.com/kubernetes/kubernetes/tree/v1.29.5/pkg/apis/core/v1.

The defaulting functions are not included in any public Kubernetes module,
but we need to be able to invoke them on the PodSpecTemplate's we construct
to give to Kueue to avoid breaking the implicit assumption of Kueue's
ComparePodSetSlices utility which only works if the two PodSets being compared
have either both been defaulted (or both not defaulted, which isn't viable
since the PodSpecTemplate in Kueue's Workload object will be defaulted by the APIServer).
*/

// nolint
package utils

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	//  Import the crypto sha256 algorithm for the docker image parser to work
	_ "crypto/sha256"
	//  Import the crypto/sha512 algorithm for the docker image parser to work with 384 and 512 sha hashes
	_ "crypto/sha512"

	dockerref "github.com/distribution/reference"
)

// parseImageName parses a docker image string into three parts: repo, tag and digest.
// If both tag and digest are empty, a default image tag will be returned.
func parseImageName(image string) (string, string, string, error) {
	named, err := dockerref.ParseNormalizedNamed(image)
	if err != nil {
		return "", "", "", fmt.Errorf("couldn't parse image name %q: %v", image, err)
	}

	repoToPull := named.Name()
	var tag, digest string

	tagged, ok := named.(dockerref.Tagged)
	if ok {
		tag = tagged.Tag()
	}

	digested, ok := named.(dockerref.Digested)
	if ok {
		digest = digested.Digest().String()
	}
	// If no tag was specified, use the default "latest".
	if len(tag) == 0 && len(digest) == 0 {
		tag = "latest"
	}
	return repoToPull, tag, digest, nil
}

func setDefaults_ResourceList(obj *v1.ResourceList) {
	for key, val := range *obj {
		// TODO(#18538): We round up resource values to milli scale to maintain API compatibility.
		// In the future, we should instead reject values that need rounding.
		const milliScale = -3
		val.RoundUp(milliScale)

		(*obj)[v1.ResourceName(key)] = val
	}
}

func setDefaults_Volume(obj *v1.Volume) {
	if pointer.AllPtrFieldsNil(&obj.VolumeSource) {
		obj.VolumeSource = v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		}
	}
}
func setDefaults_Container(obj *v1.Container) {
	if obj.ImagePullPolicy == "" {
		// Ignore error and assume it has been validated elsewhere
		_, tag, _, _ := parseImageName(obj.Image)

		// Check image tag
		if tag == "latest" {
			obj.ImagePullPolicy = v1.PullAlways
		} else {
			obj.ImagePullPolicy = v1.PullIfNotPresent
		}
	}
	if obj.TerminationMessagePath == "" {
		obj.TerminationMessagePath = v1.TerminationMessagePathDefault
	}
	if obj.TerminationMessagePolicy == "" {
		obj.TerminationMessagePolicy = v1.TerminationMessageReadFile
	}
}

func setDefaults_EphemeralContainer(obj *v1.EphemeralContainer) {
	setDefaults_Container((*v1.Container)(&obj.EphemeralContainerCommon))
}

func setDefaults_PodSpec(obj *v1.PodSpec) {
	// New fields added here will break upgrade tests:
	// https://github.com/kubernetes/kubernetes/issues/69445
	// In most cases the new defaulted field can added to SetDefaults_Pod instead of here, so
	// that it only materializes in the Pod object and not all objects with a PodSpec field.
	if obj.DNSPolicy == "" {
		obj.DNSPolicy = v1.DNSClusterFirst
	}
	if obj.RestartPolicy == "" {
		obj.RestartPolicy = v1.RestartPolicyAlways
	}
	if obj.SecurityContext == nil {
		obj.SecurityContext = &v1.PodSecurityContext{}
	}
	if obj.TerminationGracePeriodSeconds == nil {
		period := int64(v1.DefaultTerminationGracePeriodSeconds)
		obj.TerminationGracePeriodSeconds = &period
	}
	if obj.SchedulerName == "" {
		obj.SchedulerName = v1.DefaultSchedulerName
	}
}
func setDefaults_Probe(obj *v1.Probe) {
	if obj.TimeoutSeconds == 0 {
		obj.TimeoutSeconds = 1
	}
	if obj.PeriodSeconds == 0 {
		obj.PeriodSeconds = 10
	}
	if obj.SuccessThreshold == 0 {
		obj.SuccessThreshold = 1
	}
	if obj.FailureThreshold == 0 {
		obj.FailureThreshold = 3
	}
}
func setDefaults_SecretVolumeSource(obj *v1.SecretVolumeSource) {
	if obj.DefaultMode == nil {
		perm := int32(v1.SecretVolumeSourceDefaultMode)
		obj.DefaultMode = &perm
	}
}
func setDefaults_ConfigMapVolumeSource(obj *v1.ConfigMapVolumeSource) {
	if obj.DefaultMode == nil {
		perm := int32(v1.ConfigMapVolumeSourceDefaultMode)
		obj.DefaultMode = &perm
	}
}
func setDefaults_DownwardAPIVolumeSource(obj *v1.DownwardAPIVolumeSource) {
	if obj.DefaultMode == nil {
		perm := int32(v1.DownwardAPIVolumeSourceDefaultMode)
		obj.DefaultMode = &perm
	}
}
func setDefaults_ProjectedVolumeSource(obj *v1.ProjectedVolumeSource) {
	if obj.DefaultMode == nil {
		perm := int32(v1.ProjectedVolumeSourceDefaultMode)
		obj.DefaultMode = &perm
	}
}
func setDefaults_ServiceAccountTokenProjection(obj *v1.ServiceAccountTokenProjection) {
	hour := int64(time.Hour.Seconds())
	if obj.ExpirationSeconds == nil {
		obj.ExpirationSeconds = &hour
	}
}
func setDefaults_PersistentVolumeClaim(obj *v1.PersistentVolumeClaim) {
	if obj.Status.Phase == "" {
		obj.Status.Phase = v1.ClaimPending
	}
}
func setDefaults_PersistentVolumeClaimSpec(obj *v1.PersistentVolumeClaimSpec) {
	if obj.VolumeMode == nil {
		obj.VolumeMode = new(v1.PersistentVolumeMode)
		*obj.VolumeMode = v1.PersistentVolumeFilesystem
	}
}
func setDefaults_ISCSIVolumeSource(obj *v1.ISCSIVolumeSource) {
	if obj.ISCSIInterface == "" {
		obj.ISCSIInterface = "default"
	}
}
func setDefaults_AzureDiskVolumeSource(obj *v1.AzureDiskVolumeSource) {
	if obj.CachingMode == nil {
		obj.CachingMode = new(v1.AzureDataDiskCachingMode)
		*obj.CachingMode = v1.AzureDataDiskCachingReadWrite
	}
	if obj.Kind == nil {
		obj.Kind = new(v1.AzureDataDiskKind)
		*obj.Kind = v1.AzureSharedBlobDisk
	}
	if obj.FSType == nil {
		obj.FSType = new(string)
		*obj.FSType = "ext4"
	}
	if obj.ReadOnly == nil {
		obj.ReadOnly = new(bool)
		*obj.ReadOnly = false
	}
}
func setDefaults_HTTPGetAction(obj *v1.HTTPGetAction) {
	if obj.Path == "" {
		obj.Path = "/"
	}
	if obj.Scheme == "" {
		obj.Scheme = v1.URISchemeHTTP
	}
}

func setDefaults_ObjectFieldSelector(obj *v1.ObjectFieldSelector) {
	if obj.APIVersion == "" {
		obj.APIVersion = "v1"
	}
}

func setDefaults_RBDVolumeSource(obj *v1.RBDVolumeSource) {
	if obj.RBDPool == "" {
		obj.RBDPool = "rbd"
	}
	if obj.RadosUser == "" {
		obj.RadosUser = "admin"
	}
	if obj.Keyring == "" {
		obj.Keyring = "/etc/ceph/keyring"
	}
}

func setDefaults_ScaleIOVolumeSource(obj *v1.ScaleIOVolumeSource) {
	if obj.StorageMode == "" {
		obj.StorageMode = "ThinProvisioned"
	}
	if obj.FSType == "" {
		obj.FSType = "xfs"
	}
}

func setDefaults_HostPathVolumeSource(obj *v1.HostPathVolumeSource) {
	typeVol := v1.HostPathUnset
	if obj.Type == nil {
		obj.Type = &typeVol
	}
}

func setObjectDefaults_PodTemplate(in *v1.PodTemplate) {
	setDefaults_PodSpec(&in.Template.Spec)
	for i := range in.Template.Spec.Volumes {
		a := &in.Template.Spec.Volumes[i]
		setDefaults_Volume(a)
		if a.VolumeSource.HostPath != nil {
			setDefaults_HostPathVolumeSource(a.VolumeSource.HostPath)
		}
		if a.VolumeSource.Secret != nil {
			setDefaults_SecretVolumeSource(a.VolumeSource.Secret)
		}
		if a.VolumeSource.ISCSI != nil {
			setDefaults_ISCSIVolumeSource(a.VolumeSource.ISCSI)
		}
		if a.VolumeSource.RBD != nil {
			setDefaults_RBDVolumeSource(a.VolumeSource.RBD)
		}
		if a.VolumeSource.DownwardAPI != nil {
			setDefaults_DownwardAPIVolumeSource(a.VolumeSource.DownwardAPI)
			for j := range a.VolumeSource.DownwardAPI.Items {
				b := &a.VolumeSource.DownwardAPI.Items[j]
				if b.FieldRef != nil {
					setDefaults_ObjectFieldSelector(b.FieldRef)
				}
			}
		}
		if a.VolumeSource.ConfigMap != nil {
			setDefaults_ConfigMapVolumeSource(a.VolumeSource.ConfigMap)
		}
		if a.VolumeSource.AzureDisk != nil {
			setDefaults_AzureDiskVolumeSource(a.VolumeSource.AzureDisk)
		}
		if a.VolumeSource.Projected != nil {
			setDefaults_ProjectedVolumeSource(a.VolumeSource.Projected)
			for j := range a.VolumeSource.Projected.Sources {
				b := &a.VolumeSource.Projected.Sources[j]
				if b.DownwardAPI != nil {
					for k := range b.DownwardAPI.Items {
						c := &b.DownwardAPI.Items[k]
						if c.FieldRef != nil {
							setDefaults_ObjectFieldSelector(c.FieldRef)
						}
					}
				}
				if b.ServiceAccountToken != nil {
					setDefaults_ServiceAccountTokenProjection(b.ServiceAccountToken)
				}
			}
		}
		if a.VolumeSource.ScaleIO != nil {
			setDefaults_ScaleIOVolumeSource(a.VolumeSource.ScaleIO)
		}
		if a.VolumeSource.Ephemeral != nil {
			if a.VolumeSource.Ephemeral.VolumeClaimTemplate != nil {
				setDefaults_PersistentVolumeClaimSpec(&a.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec)
				setDefaults_ResourceList(&a.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.Resources.Limits)
				setDefaults_ResourceList(&a.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests)
			}
		}
	}
	for i := range in.Template.Spec.InitContainers {
		a := &in.Template.Spec.InitContainers[i]
		setDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					setDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		setDefaults_ResourceList(&a.Resources.Limits)
		setDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			setDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.LivenessProbe.ProbeHandler.HTTPGet)
			}
			if a.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.ReadinessProbe != nil {
			setDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.ReadinessProbe.ProbeHandler.HTTPGet)
			}
			if a.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.StartupProbe != nil {
			setDefaults_Probe(a.StartupProbe)
			if a.StartupProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.StartupProbe.ProbeHandler.HTTPGet)
			}
			if a.StartupProbe.ProbeHandler.GRPC != nil {
				if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Template.Spec.Containers {
		a := &in.Template.Spec.Containers[i]
		setDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					setDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		setDefaults_ResourceList(&a.Resources.Limits)
		setDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			setDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.LivenessProbe.ProbeHandler.HTTPGet)
			}
			if a.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.ReadinessProbe != nil {
			setDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.ReadinessProbe.ProbeHandler.HTTPGet)
			}
			if a.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.StartupProbe != nil {
			setDefaults_Probe(a.StartupProbe)
			if a.StartupProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.StartupProbe.ProbeHandler.HTTPGet)
			}
			if a.StartupProbe.ProbeHandler.GRPC != nil {
				if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Template.Spec.EphemeralContainers {
		a := &in.Template.Spec.EphemeralContainers[i]
		setDefaults_EphemeralContainer(a)
		for j := range a.EphemeralContainerCommon.Ports {
			b := &a.EphemeralContainerCommon.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		for j := range a.EphemeralContainerCommon.Env {
			b := &a.EphemeralContainerCommon.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					setDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		setDefaults_ResourceList(&a.EphemeralContainerCommon.Resources.Limits)
		setDefaults_ResourceList(&a.EphemeralContainerCommon.Resources.Requests)
		if a.EphemeralContainerCommon.LivenessProbe != nil {
			setDefaults_Probe(a.EphemeralContainerCommon.LivenessProbe)
			if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.HTTPGet)
			}
			if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.EphemeralContainerCommon.ReadinessProbe != nil {
			setDefaults_Probe(a.EphemeralContainerCommon.ReadinessProbe)
			if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.HTTPGet)
			}
			if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.EphemeralContainerCommon.StartupProbe != nil {
			setDefaults_Probe(a.EphemeralContainerCommon.StartupProbe)
			if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.HTTPGet != nil {
				setDefaults_HTTPGetAction(a.EphemeralContainerCommon.StartupProbe.ProbeHandler.HTTPGet)
			}
			if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.EphemeralContainerCommon.Lifecycle != nil {
			if a.EphemeralContainerCommon.Lifecycle.PostStart != nil {
				if a.EphemeralContainerCommon.Lifecycle.PostStart.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.EphemeralContainerCommon.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.EphemeralContainerCommon.Lifecycle.PreStop != nil {
				if a.EphemeralContainerCommon.Lifecycle.PreStop.HTTPGet != nil {
					setDefaults_HTTPGetAction(a.EphemeralContainerCommon.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	setDefaults_ResourceList(&in.Template.Spec.Overhead)
}
