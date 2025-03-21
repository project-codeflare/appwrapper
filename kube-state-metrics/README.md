This directory contains configuration files for enabling
[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics/)
to report metrics for AppWrapper.

The file [appwrapper-ksm-cm.yaml](./appwrapper-ksm-cm.yaml) defines
a configuration map that can be volume-mounted into the
kube-state-metrics pod and passed via the `--custom-resource-state-config-file`
command line argument.  For development of the AppWrapper metrics,
you may want to add `--custom-resource-state-only=true` to the command
line arguments to suppress generation of metrics for built-in types.

The file [appwrapper-ksm-rbac.yaml](./appwrapper-ksm-rbac.yaml) defines
a clusterrole and clusterrolebinding that add the RBACs
needed to collect AppWrapper metrics to the `kube-state-metrics` service account.
Alternatively, you could edit the existing kube-state-metrics clusterrole to
add these permissions.

The changes to the kube-state-metrics deployment are roughly as shown below:
```yaml
  ...
    spec:
      containers:
      - args:
        - --custom-resource-state-config-file=/appwrapper_ksm.yaml
  ...
        volumeMounts:
        - mountPath: /appwrapper_ksm.yaml
          name: appwrapper-ksm
          readOnly: true
          subPath: appwrapper_ksm.yaml
  ...
      volumes:
      - configMap:
          defaultMode: 420
          name: appwrapper-ksm
        name: appwrapper-ksm
```
