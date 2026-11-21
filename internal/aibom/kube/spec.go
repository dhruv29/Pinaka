package kube

import (
	"bytes"
	"encoding/json"
	"sort"
)

// flexString accepts a JSON string or number, because Kubernetes writes
// quantities and ports either way.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

type keyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type envSource struct {
	SecretKeyRef    *keyRef `json:"secretKeyRef"`
	ConfigMapKeyRef *keyRef `json:"configMapKeyRef"`
}

// envVar holds an environment variable. Value may be a credential: it is
// only ever tested for emptiness or, for non-credential names, parsed.
type envVar struct {
	Name      string     `json:"name"`
	Value     string     `json:"value"`
	ValueFrom *envSource `json:"valueFrom"`
}

type resources struct {
	Limits   map[string]flexString `json:"limits"`
	Requests map[string]flexString `json:"requests"`
}

type securityContext struct {
	Privileged *bool  `json:"privileged"`
	RunAsUser  *int64 `json:"runAsUser"`
}

type container struct {
	Name            string           `json:"name"`
	Image           string           `json:"image"`
	Command         []string         `json:"command"`
	Args            []string         `json:"args"`
	Env             []envVar         `json:"env"`
	Resources       resources        `json:"resources"`
	SecurityContext *securityContext `json:"securityContext"`
}

type volume struct {
	Name     string `json:"name"`
	HostPath *struct {
		Path string `json:"path"`
	} `json:"hostPath"`
}

type podSpec struct {
	Containers      []container            `json:"containers"`
	InitContainers  []container            `json:"initContainers"`
	NodeSelector    map[string]string      `json:"nodeSelector"`
	Tolerations     []struct{ Key string } `json:"tolerations"`
	HostNetwork     bool                   `json:"hostNetwork"`
	HostPID         bool                   `json:"hostPID"`
	HostIPC         bool                   `json:"hostIPC"`
	Volumes         []volume               `json:"volumes"`
	SecurityContext *securityContext       `json:"securityContext"`
}

type podTemplate struct {
	Metadata objectMeta `json:"metadata"`
	Spec     podSpec    `json:"spec"`
}

type rayClusterSpec struct {
	HeadGroupSpec    struct{ Template podTemplate }   `json:"headGroupSpec"`
	WorkerGroupSpecs []struct{ Template podTemplate } `json:"workerGroupSpecs"`
}

// isvcModel is a KServe predictor entry ("model", "sklearn", "huggingface"
// …): container fields plus where the weights live.
type isvcModel struct {
	container
	StorageURI  string `json:"storageUri"`
	ModelFormat struct {
		Name string `json:"name"`
	} `json:"modelFormat"`
}

// storageModel is a model location named by a custom resource.
type storageModel struct {
	uri, field, format string
}

// workload is anything that runs pods, reduced to what the detectors need.
type workload struct {
	ref    objRef
	path   string
	labels map[string]string
	pods   []podSpec
	models []storageModel
}

// crdGroups pins the custom kinds read here to their API groups, so an
// unrelated "Notebook" kind from another vendor is not misread.
var crdGroups = map[string]string{
	"Notebook":         "kubeflow.org/",
	"InferenceService": "serving.kserve.io/",
	"RayCluster":       "ray.io/",
	"RayService":       "ray.io/",
	"HTTPRoute":        "gateway.networking.k8s.io/",
}

var templateKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true, "Job": true, "Notebook": true,
}

// kindOf returns the object's kind, or "" for a custom kind from the wrong
// API group.
func kindOf(o object) string {
	if group, ok := crdGroups[o.Kind]; ok && !hasPrefix(o.APIVersion, group) {
		return ""
	}
	return o.Kind
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

// extractWorkload returns the pods an object runs. ok is false for objects
// that run nothing.
func extractWorkload(o object) (workload, bool, error) {
	w := workload{ref: refOf(o)}
	kind := kindOf(o)
	var err error
	switch {
	case kind == "Pod":
		var s podSpec
		err = decodeInto(o.Spec, &s)
		w.labels, w.pods = o.Metadata.Labels, []podSpec{s}
	case templateKinds[kind]:
		var s struct{ Template podTemplate }
		err = decodeInto(o.Spec, &s)
		w.labels, w.pods = s.Template.Metadata.Labels, []podSpec{s.Template.Spec}
	case kind == "CronJob":
		var s struct {
			JobTemplate struct {
				Spec struct{ Template podTemplate }
			}
		}
		err = decodeInto(o.Spec, &s)
		t := s.JobTemplate.Spec.Template
		w.labels, w.pods = t.Metadata.Labels, []podSpec{t.Spec}
	case kind == "RayCluster":
		var s rayClusterSpec
		err = decodeInto(o.Spec, &s)
		w.labels, w.pods = rayPods(s)
	case kind == "RayService":
		var s struct{ RayClusterConfig rayClusterSpec }
		err = decodeInto(o.Spec, &s)
		w.labels, w.pods = rayPods(s.RayClusterConfig)
	case kind == "InferenceService":
		err = extractInferenceService(o, &w)
	default:
		return w, false, nil
	}
	return w, err == nil, err
}

func rayPods(s rayClusterSpec) (map[string]string, []podSpec) {
	pods := []podSpec{s.HeadGroupSpec.Template.Spec}
	for _, g := range s.WorkerGroupSpecs {
		pods = append(pods, g.Template.Spec)
	}
	return s.HeadGroupSpec.Template.Metadata.Labels, pods
}

// extractInferenceService reads predictor, transformer and explainer. Each
// is a pod spec inline plus framework entries that carry a storageUri.
func extractInferenceService(o object, w *workload) error {
	var spec map[string]json.RawMessage
	if err := decodeInto(o.Spec, &spec); err != nil {
		return err
	}
	for _, part := range []string{"predictor", "transformer", "explainer"} {
		raw, ok := spec[part]
		if !ok {
			continue
		}
		var pod podSpec
		var entries map[string]json.RawMessage
		if err := decodeInto(raw, &pod); err != nil {
			return err
		}
		if err := decodeInto(raw, &entries); err != nil {
			return err
		}
		addKServeEntries(w, &pod, "spec."+part, entries)
		w.pods = append(w.pods, pod)
	}
	return nil
}

func addKServeEntries(w *workload, pod *podSpec, field string, entries map[string]json.RawMessage) {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := bytes.TrimSpace(entries[key])
		var m isvcModel
		if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.StorageURI == "" && m.ModelFormat.Name == "" {
			continue
		}
		format := m.ModelFormat.Name
		if key != "model" || format == "" {
			format = firstNonEmpty(format, key)
		}
		if m.StorageURI != "" {
			w.models = append(w.models, storageModel{uri: m.StorageURI, field: field + "." + key + ".storageUri", format: format})
		}
		if m.Name == "" {
			m.Name = "kserve-container"
		}
		pod.Containers = append(pod.Containers, m.container)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
