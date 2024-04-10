---
permalink: /api/v1beta2/
title: AppWrapper API
classes: wide
---


Generated API reference documentation for <no value>.

## Resource Types

- [AppWrapper](#AppWrapper)
  - [AppWrapperComponent](#AppWrapperComponent)
  - [AppWrapperCondition](#AppWrapperCondition)
  - [AppWrapperList](#AppWrapperList)
  - [AppWrapperPhase](#AppWrapperPhase)
  - [AppWrapperPodSet](#AppWrapperPodSet)
  - [AppWrapperPodSetInfo](#AppWrapperPodSetInfo)
  - [AppWrapperSpec](#AppWrapperSpec)
  - [AppWrapperStatus](#AppWrapperStatus)





## `AppWrapper`     {#AppWrapper}


**Appears in:**



<p>AppWrapper is the Schema for the appwrappers API</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>spec</code> <B>[Required]</B><br/>
<a href="#AppWrapperSpec"><code>AppWrapperSpec</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>status</code> <B>[Required]</B><br/>
<a href="#AppWrapperStatus"><code>AppWrapperStatus</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
</tbody>
</table>

## `AppWrapperComponent`     {#AppWrapperComponent}


**Appears in:**

- [AppWrapperSpec](#AppWrapperSpec)


<p>AppWrapperComponent describes a wrapped resource</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>podSets</code> <B>[Required]</B><br/>
<a href="#AppWrapperPodSet"><code>[]AppWrapperPodSet</code></a>
</td>
<td>
   <p>PodSets contained in the component</p>
</td>
</tr>
<tr><td><code>podSetInfos</code> <B>[Required]</B><br/>
<a href="#AppWrapperPodSetInfo"><code>[]AppWrapperPodSetInfo</code></a>
</td>
<td>
   <p>PodSetInfos assigned to the Component by Kueue</p>
</td>
</tr>
<tr><td><code>template</code> <B>[Required]</B><br/>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/runtime#RawExtension"><code>k8s.io/apimachinery/pkg/runtime.RawExtension</code></a>
</td>
<td>
   <p>Template for the component</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperCondition`     {#AppWrapperCondition}

(Alias of `string`)





## `AppWrapperList`     {#AppWrapperList}



<p>AppWrapperList contains a list of appwrappers</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>metadata</code> <B>[Required]</B><br/>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#listmeta-v1-meta"><code>k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>items</code> <B>[Required]</B><br/>
<a href="#AppWrapper"><code>[]AppWrapper</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
</tbody>
</table>

## `AppWrapperPhase`     {#AppWrapperPhase}

(Alias of `string`)

**Appears in:**

- [AppWrapperStatus](#AppWrapperStatus)


<p>AppWrapperPhase is the phase of the appwrapper</p>




## `AppWrapperPodSet`     {#AppWrapperPodSet}


**Appears in:**

- [AppWrapperComponent](#AppWrapperComponent)


<p>AppWrapperPodSet describes an homogeneous set of pods</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>replicas</code> <B>[Required]</B><br/>
<code>int32</code>
</td>
<td>
   <p>Replicas is the number of pods in the set</p>
</td>
</tr>
<tr><td><code>path</code> <B>[Required]</B><br/>
<code>string</code>
</td>
<td>
   <p>Path to the PodTemplateSpec</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperPodSetInfo`     {#AppWrapperPodSetInfo}


**Appears in:**

- [AppWrapperComponent](#AppWrapperComponent)



<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>annotations</code> <B>[Required]</B><br/>
<code>map[string]string</code>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>labels</code> <B>[Required]</B><br/>
<code>map[string]string</code>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>nodeSelector</code> <B>[Required]</B><br/>
<code>map[string]string</code>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
<tr><td><code>tolerations</code> <B>[Required]</B><br/>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#toleration-v1-core"><code>[]k8s.io/api/core/v1.Toleration</code></a>
</td>
<td>
   <span class="text-muted">No description provided.</span></td>
</tr>
</tbody>
</table>

## `AppWrapperSpec`     {#AppWrapperSpec}


**Appears in:**

- [AppWrapper](#AppWrapper)


<p>AppWrapperSpec defines the desired state of the appwrapper</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>components</code> <B>[Required]</B><br/>
<a href="#AppWrapperComponent"><code>[]AppWrapperComponent</code></a>
</td>
<td>
   <p>Components lists the components in the job</p>
</td>
</tr>
<tr><td><code>suspend</code> <B>[Required]</B><br/>
<code>bool</code>
</td>
<td>
   <p>Suspend suspends the job when set to true</p>
</td>
</tr>
</tbody>
</table>

## `AppWrapperStatus`     {#AppWrapperStatus}


**Appears in:**

- [AppWrapper](#AppWrapper)


<p>AppWrapperStatus defines the observed state of the appwrapper</p>


<table class="table">
<thead><tr><th width="30%">Field</th><th>Description</th></tr></thead>
<tbody>


<tr><td><code>phase</code> <B>[Required]</B><br/>
<a href="#AppWrapperPhase"><code>AppWrapperPhase</code></a>
</td>
<td>
   <p>Phase of the AppWrapper object</p>
</td>
</tr>
<tr><td><code>resettingCount</code> <B>[Required]</B><br/>
<code>int32</code>
</td>
<td>
   <p>Retries counts the number of times the AppWrapper has entered the Resetting Phase</p>
</td>
</tr>
<tr><td><code>conditions</code> <B>[Required]</B><br/>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#condition-v1-meta"><code>[]k8s.io/apimachinery/pkg/apis/meta/v1.Condition</code></a>
</td>
<td>
   <p>Conditions</p>
</td>
</tr>
</tbody>
</table>
