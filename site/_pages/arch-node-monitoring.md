---
permalink: /arch-node-monitoring/
title: "Node Monitoring"
classes: wide
---

The AppWrapper controller can optionally monitor Kubernetes Nodes and
dynamically adjust the `lendingLimits` on a designated `ClusterQueue`
to account for dynamically unavailable resources. This capability is
designed to enable cluster admins of an
[MLBatch cluster](https://github.com/project-codeflare/mlbatch) to fully
automate the small scale quota adjustments required to maintain full cluster
utilization in the presence of isolated node failures and/or
minor maintenance activities.  The monitoring detects both Nodes that
are marked as `Unscheduable` via standard Kubernetes mechanisms and Nodes
that have resources that Autopilot has flagged as unhealthy (see [Fault Tolerance](/arch-fault-tolerance)).
The `lendingLimit` of a designated slack capacity `ClusterQueue` is
automatically adjusted to reflect the current dynamically unavailable resources.

Node monitoring is enabled by the following additional configuration:
```yaml
slackQueueName: "slack-queue"
autopilot:
  monitorNodes: true
```

See [node_health_monitor.go]({{ site.gh_main_url }}/internal/controller/appwrapper/node_health_monitor.go)
for the implementation.
