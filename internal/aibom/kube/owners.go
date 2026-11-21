package kube

import (
	"strings"
	"unicode"
)

const (
	// maxOwnerDepth bounds the ownerReference walk; real chains are at most
	// four deep (Pod → ReplicaSet → Deployment → custom resource), and a
	// loop in hostile input must not spin.
	maxOwnerDepth = 8
	maxNameLen    = 253
	podHashLabel  = "pod-template-hash"
)

// objRef identifies a namespaced object.
type objRef struct {
	Namespace, Kind, Name string
}

func refOf(o object) objRef {
	return objRef{Namespace: o.Metadata.Namespace, Kind: o.Kind, Name: o.Metadata.Name}
}

// path renders the evidence path "namespace/Kind/name".
func (r objRef) path() string {
	p := clean(r.Kind) + "/" + clean(r.Name)
	if r.Namespace == "" {
		return p
	}
	return clean(r.Namespace) + "/" + p
}

// clean bounds a string from the input and strips control characters, so
// hostile names cannot forge lines or bloat the report.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > maxNameLen {
		s = s[:maxNameLen]
	}
	return s
}

// ownerIndex maps every object to its metadata, so ownerReferences can be
// followed to the top-level owner. That owner is the evidence path: a
// Deployment and its three Pods are one workload, not four.
type ownerIndex map[objRef]objectMeta

func newOwnerIndex(objs []object) ownerIndex {
	ix := ownerIndex{}
	for _, o := range objs {
		ix[refOf(o)] = o.Metadata
	}
	return ix
}

// top follows controller ownerReferences as far as they go. ReplicaSets are
// not collected, so a Pod's ReplicaSet is resolved to its Deployment by
// stripping the pod-template-hash suffix from the ReplicaSet name.
func (ix ownerIndex) top(ref objRef, meta objectMeta) objRef {
	cur := ref
	for i := 0; i < maxOwnerDepth; i++ {
		owner, ok := controllerOf(meta.OwnerReferences)
		if !ok {
			break
		}
		next := objRef{Namespace: cur.Namespace, Kind: owner.Kind, Name: owner.Name}
		if _, known := ix[next]; !known && next.Kind == "ReplicaSet" {
			if hash := meta.Labels[podHashLabel]; hash != "" && strings.HasSuffix(next.Name, "-"+hash) {
				next = objRef{Namespace: cur.Namespace, Kind: "Deployment", Name: strings.TrimSuffix(next.Name, "-"+hash)}
			}
		}
		if next == cur {
			break
		}
		cur, meta = next, ix[next]
	}
	return cur
}

func controllerOf(refs []ownerRef) (ownerRef, bool) {
	var first *ownerRef
	for i := range refs {
		r := refs[i]
		if r.Kind == "" || r.Name == "" {
			continue
		}
		if r.Controller {
			return r, true
		}
		if first == nil {
			first = &refs[i]
		}
	}
	if first == nil {
		return ownerRef{}, false
	}
	return *first, true
}
