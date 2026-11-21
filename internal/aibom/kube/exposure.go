package kube

import (
	"sort"
	"strings"
)

// exposure is one way a workload can be reached from outside the cluster.
type exposure struct {
	kind, via, host, port string
}

func (e exposure) reason() string {
	s := "via " + e.via
	if e.host != "" {
		s += " host " + e.host
	}
	if e.port != "" {
		s += " port " + e.port
	}
	return s
}

type serviceInfo struct {
	name, typ string
	selector  map[string]string
	ports     []string
	nodePorts []string
	external  bool
	lbHosts   []string
}

type servicePort struct {
	Port     flexString `json:"port"`
	NodePort flexString `json:"nodePort"`
}

type serviceObject struct {
	Spec struct {
		Type        string            `json:"type"`
		Selector    map[string]string `json:"selector"`
		Ports       []servicePort     `json:"ports"`
		ExternalIPs []string          `json:"externalIPs"`
	}
	Status struct {
		LoadBalancer struct {
			Ingress []struct{ IP, Hostname string }
		} `json:"loadBalancer"`
	}
}

type ingressBackend struct {
	Service *struct {
		Name string
		Port struct {
			Number flexString `json:"number"`
			Name   string     `json:"name"`
		}
	} `json:"service"`
	ServiceName string     `json:"serviceName"`
	ServicePort flexString `json:"servicePort"`
}

type ingressSpec struct {
	DefaultBackend *ingressBackend `json:"defaultBackend"`
	Backend        *ingressBackend `json:"backend"`
	Rules          []struct {
		Host string
		HTTP *struct {
			Paths []struct{ Backend ingressBackend }
		} `json:"http"`
	} `json:"rules"`
}

type routeSpec struct {
	Hostnames []string `json:"hostnames"`
	Rules     []struct {
		BackendRefs []struct {
			Kind, Name, Namespace string
			Port                  flexString
		} `json:"backendRefs"`
	} `json:"rules"`
}

// exposureIndex knows which Services are externally reachable and which are
// fronted by an Ingress or HTTPRoute.
type exposureIndex struct {
	services map[string][]serviceInfo // by namespace
	fronted  map[string][]exposure    // by "namespace/service"
}

func newExposureIndex(objs []object, note func(string)) exposureIndex {
	x := exposureIndex{services: map[string][]serviceInfo{}, fronted: map[string][]exposure{}}
	for _, o := range objs {
		var err error
		switch kindOf(o) {
		case "Service":
			err = x.addService(o)
		case "Ingress":
			err = x.addIngress(o)
		case "HTTPRoute":
			err = x.addRoute(o)
		}
		if err != nil {
			note("skipped " + refOf(o).path() + ": its spec is not in the expected shape")
		}
	}
	return x
}

func (x exposureIndex) addService(o object) error {
	var s serviceObject
	if err := decodeInto(o.Spec, &s.Spec); err != nil {
		return err
	}
	_ = decodeInto(o.Status, &s.Status) // status is optional and only adds host names
	info := serviceInfo{name: o.Metadata.Name, typ: s.Spec.Type, selector: s.Spec.Selector, external: len(s.Spec.ExternalIPs) > 0}
	for _, p := range s.Spec.Ports {
		info.ports = appendNonEmpty(info.ports, string(p.Port))
		info.nodePorts = appendNonEmpty(info.nodePorts, string(p.NodePort))
	}
	for _, in := range s.Status.LoadBalancer.Ingress {
		info.lbHosts = appendNonEmpty(info.lbHosts, firstNonEmpty(in.Hostname, in.IP))
	}
	ns := o.Metadata.Namespace
	x.services[ns] = append(x.services[ns], info)
	return nil
}

func (x exposureIndex) addIngress(o object) error {
	var s ingressSpec
	if err := decodeInto(o.Spec, &s); err != nil {
		return err
	}
	via := "Ingress " + refOf(o).path()
	for _, b := range []*ingressBackend{s.DefaultBackend, s.Backend} {
		x.front(o.Metadata.Namespace, b, exposure{kind: "Ingress", via: via, host: "*"})
	}
	for _, r := range s.Rules {
		if r.HTTP == nil {
			continue
		}
		for _, p := range r.HTTP.Paths {
			x.front(o.Metadata.Namespace, &p.Backend, exposure{kind: "Ingress", via: via, host: r.Host})
		}
	}
	return nil
}

func (x exposureIndex) front(ns string, b *ingressBackend, e exposure) {
	if b == nil {
		return
	}
	name, port := b.ServiceName, string(b.ServicePort)
	if b.Service != nil {
		name, port = b.Service.Name, firstNonEmpty(string(b.Service.Port.Number), b.Service.Port.Name)
	}
	if name == "" {
		return
	}
	e.port = port
	x.fronted[ns+"/"+name] = append(x.fronted[ns+"/"+name], e)
}

func (x exposureIndex) addRoute(o object) error {
	var s routeSpec
	if err := decodeInto(o.Spec, &s); err != nil {
		return err
	}
	hosts := strings.Join(s.Hostnames, ",")
	for _, r := range s.Rules {
		for _, b := range r.BackendRefs {
			if b.Kind != "" && b.Kind != "Service" || b.Name == "" {
				continue
			}
			ns := firstNonEmpty(b.Namespace, o.Metadata.Namespace)
			e := exposure{kind: "HTTPRoute", via: "HTTPRoute " + refOf(o).path(), host: hosts, port: string(b.Port)}
			x.fronted[ns+"/"+b.Name] = append(x.fronted[ns+"/"+b.Name], e)
		}
	}
	return nil
}

// find lists how pods with these labels in this namespace are exposed,
// matching Service selectors to the labels as Kubernetes does.
func (x exposureIndex) find(ns string, labels map[string]string) []exposure {
	var out []exposure
	for _, svc := range x.services[ns] {
		if !selects(svc.selector, labels) {
			continue
		}
		via := svc.typ + " Service " + ns + "/" + svc.name
		switch {
		case svc.typ == "LoadBalancer":
			out = append(out, exposure{kind: "LoadBalancer", via: via, host: strings.Join(svc.lbHosts, ","), port: strings.Join(svc.ports, ",")})
		case svc.typ == "NodePort":
			out = append(out, exposure{kind: "NodePort", via: via, port: strings.Join(svc.nodePorts, ",")})
		case svc.external:
			out = append(out, exposure{kind: "externalIPs", via: "Service " + ns + "/" + svc.name + " with externalIPs", port: strings.Join(svc.ports, ",")})
		}
		out = append(out, x.fronted[ns+"/"+svc.name]...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].reason() < out[j].reason() })
	return out
}

// selects reports whether a non-empty selector matches the labels. An empty
// selector selects nothing automatically, so it never counts.
func selects(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	return true
}

func appendNonEmpty(list []string, v string) []string {
	if v == "" {
		return list
	}
	return append(list, v)
}
