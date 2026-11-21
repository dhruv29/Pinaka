package kube

import "fmt"

// expectedKind is a resource kind a full export holds, and what a bill
// cannot tell when the input has none of it.
type expectedKind struct {
	kind, hides string
}

// expectedKinds are what `kubectl get` collects. A saved file may have
// been exported without some of them; an empty result and a kind left out
// of the export look the same, so both are noted.
var expectedKinds = []expectedKind{
	{"Pod", "workloads whose controllers are not in the input are missing"},
	{"Deployment", "Deployments are seen only through their Pods, if those were exported"},
	{"StatefulSet", "StatefulSets are seen only through their Pods, if those were exported"},
	{"DaemonSet", "DaemonSets are seen only through their Pods, if those were exported"},
	{"Job", "Jobs are seen only through their Pods, if those were exported"},
	{"CronJob", "CronJobs that are not running are missing"},
	{"Service", "whether a workload is reachable through a LoadBalancer, NodePort or externalIPs could not be judged"},
	{"Ingress", "whether a workload is reachable through an Ingress could not be judged"},
	{"HTTPRoute", "whether a workload is reachable through a Gateway API HTTPRoute could not be judged"},
	{"ConfigMap", "credentials, models and endpoints kept in ConfigMaps were not examined"},
	{"InferenceService", "models served by KServe are missing if the cluster runs it"},
	{"RayCluster", "Ray clusters are missing if the cluster runs them"},
	{"RayService", "Ray Serve deployments are missing if the cluster runs them"},
	{"Notebook", "Kubeflow notebooks are missing if the cluster runs them"},
}

// absentKindNotes says which expected kinds the input has none of. It is
// for saved input only: Collect asks for every kind and notes failures.
func absentKindNotes(objs []object) []string {
	if len(objs) == 0 {
		return nil
	}
	present := map[string]bool{}
	for _, o := range objs {
		present[o.Kind] = true
	}
	var notes []string
	for _, k := range expectedKinds {
		if !present[k.kind] {
			notes = append(notes, fmt.Sprintf("the input has no %s objects (there may be none, or they were not exported): %s", k.kind, k.hides))
		}
	}
	return notes
}
