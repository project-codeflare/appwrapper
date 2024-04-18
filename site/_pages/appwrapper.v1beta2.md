---
permalink: /api/workload.codeflare.dev/v1beta2/
title: AppWrapper API
classes: wide
---


Generated API reference documentation for <no value>.

## Resource Types



## `AppWrapper`     {#workload-codeflare-dev-v1beta2-AppWrapper}


**Appears in:**



<p>AppWrapper is the Schema for the appwrappers API</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>spec</code> <B>[Required]</B><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperSpec"><code>AppWrapperSpec</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>status</code> <B>[Required]</B><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperStatus"><code>AppWrapperStatus</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
</tbody>
</table>

## `AppWrapperComponent`     {#workload-codeflare-dev-v1beta2-AppWrapperComponent}


**Appears in:**

- [AppWrapperSpec](#workload-codeflare-dev-v1beta2-AppWrapperSpec)


<p>AppWrapperComponent describes a single wrapped Kubernetes resource</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>annotations</code><br/>
<code>map[string]string</code>
</td>
<td>
   <p>Annotations is an unstructured key value map that may be used to store and retrieve
arbitrary metadata about the Component to customize its treatment by the AppWrapper controller.</p>
</td>
</tr>
<tr><td><code>podSets</code><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperPodSet"><code>[]AppWrapperPodSet</code></a>
</td>
<td>
   <p>PodSets contained in the Component</p>
</td>
</tr>
<tr><td><code>podSetInfos</code><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperPodSetInfo"><code>[]AppWrapperPodSetInfo</code></a>
</td>
<td>
   <p>PodSetInfos assigned to the Component's PodSets by Kueue</p>
</td>
</tr>
<tr><td><code>template</code> <B>[Required]</B><br/>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/runtime#RawExtension"><code>k8s.io/apimachinery/pkg/runtime.RawExtension</code></a>
</td>
<td>
   <p>Template defines the Kubernetes resource for the Component</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperPhase`     {#workload-codeflare-dev-v1beta2-AppWrapperPhase}

(Alias of `string`)

**Appears in:**

- [AppWrapperStatus](#workload-codeflare-dev-v1beta2-AppWrapperStatus)


<p>AppWrapperPhase is the phase of the appwrapper</p>




## `AppWrapperPodSet`     {#workload-codeflare-dev-v1beta2-AppWrapperPodSet}


**Appears in:**

- [AppWrapperComponent](#workload-codeflare-dev-v1beta2-AppWrapperComponent)


<p>AppWrapperPodSet describes an homogeneous set of pods</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>replicas</code><br/>
<code>int32</code>
</td>
<td>
   <p>Replicas is the number of pods in this PodSet</p>
</td>
</tr>
<tr><td><code>replicaPath</code><br/>
<code>string</code>
</td>
<td>
   <p>ReplicaPath is the path within Component.Template to the replica count for this PodSet</p>
</td>
</tr>
<tr><td><code>podPath</code> <B>[Required]</B><br/>
<code>string</code>
</td>
<td>
   <p>PodPath is the path Component.Template to the PodTemplateSpec for this PodSet</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperPodSetInfo`     {#workload-codeflare-dev-v1beta2-AppWrapperPodSetInfo}


**Appears in:**

- [AppWrapperComponent](#workload-codeflare-dev-v1beta2-AppWrapperComponent)


<p>AppWrapperPodSetInfo contains the data that Kueue wants to inject into an admitted PodSpecTemplate</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>annotations</code><br/>
<code>map[string]string</code>
</td>
<td>
   <p>Annotations to be added to the PodSpecTemplate</p>
</td>
</tr>
<tr><td><code>labels</code><br/>
<code>map[string]string</code>
</td>
<td>
   <p>Labels to be added to the PodSepcTemplate</p>
</td>
</tr>
<tr><td><code>nodeSelector</code><br/>
<code>map[string]string</code>
</td>
<td>
   <p>NodeSelectors to be added to the PodSpecTemplate</p>
</td>
</tr>
<tr><td><code>tolerations</code><br/>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#toleration-v1-core"><code>[]k8s.io/api/core/v1.Toleration</code></a>
</td>
<td>
   <p>Tolerations to be added to the PodSpecTemplate</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperSpec`     {#workload-codeflare-dev-v1beta2-AppWrapperSpec}


**Appears in:**

- [AppWrapper](#workload-codeflare-dev-v1beta2-AppWrapper)


<p>AppWrapperSpec defines the desired state of the AppWrapper</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>components</code> <B>[Required]</B><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperComponent"><code>[]AppWrapperComponent</code></a>
</td>
<td>
   <p>Components lists the components contained in the AppWrapper</p>
</td>
</tr>
<tr><td><code>suspend</code><br/>
<code>bool</code>
</td>
<td>
   <p>Suspend suspends the AppWrapper when set to true</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperStatus`     {#workload-codeflare-dev-v1beta2-AppWrapperStatus}


**Appears in:**

- [AppWrapper](#workload-codeflare-dev-v1beta2-AppWrapper)


<p>AppWrapperStatus defines the observed state of the appwrapper</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>phase</code><br/>
<a href="#workload-codeflare-dev-v1beta2-AppWrapperPhase"><code>AppWrapperPhase</code></a>
</td>
<td>
   <p>Phase of the AppWrapper object</p>
</td>
</tr>
<tr><td><code>resettingCount</code><br/>
<code>int32</code>
</td>
<td>
   <p>Retries counts the number of times the AppWrapper has entered the Resetting Phase</p>
</td>
</tr>
<tr><td><code>conditions</code><br/>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#condition-v1-meta"><code>[]k8s.io/apimachinery/pkg/apis/meta/v1.Condition</code></a>
</td>
<td>
   <p>Conditions hold the latest available observations of the AppWrapper current state.</p>
<p>The type of the condition could be:</p>
<ul>
<li>QuotaReserved: The AppWrapper was admitted by Kueue and has quota allocated to it</li>
<li>ResourcesDeployed: The contained resources are deployed (or being deployed) on the cluster</li>
<li>PodsReady: All pods of the contained resources are in the Ready or Succeeded state</li>
<li>Unhealthy: One or more of the contained resources is unhealthy</li>
<li>DeletingResources: The contained resources are in the process of being deleted from the cluster</li>
</ul>
</td>
</tr>
</tbody>
</table>
