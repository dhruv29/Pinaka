package kube

import (
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// analyzer turns decoded objects into a bill of materials.
type analyzer struct {
	b      *builder
	owners ownerIndex
	expose exposureIndex
	notes  []string
}

func analyze(objs []object, notes []string, subject string) aibom.BOM {
	a := &analyzer{b: newBuilder(), owners: newOwnerIndex(objs), notes: notes}
	a.expose = newExposureIndex(objs, a.note)
	for _, o := range objs {
		a.analyzeObject(o)
	}
	if len(objs) == 0 {
		a.note("no Kubernetes objects were found in the input")
	}
	return a.b.bom(subject, a.notes)
}

func (a *analyzer) note(s string) { a.notes = append(a.notes, s) }

func (a *analyzer) analyzeObject(o object) {
	if o.Kind == kindConfigMap {
		a.scanConfigMap(o)
		return
	}
	w, ok, err := extractWorkload(o)
	if err != nil {
		a.note("skipped " + refOf(o).path() + ": its spec is not in the expected shape")
		return
	}
	if !ok {
		return
	}
	w.path = a.owners.top(w.ref, o.Metadata).path()
	a.scanWorkload(w)
}

// privilege is one host-level privilege a workload holds.
type privilege struct {
	raised bool
	reason string
}

// imageHit and modelHit remember what a risk needs about a component.
type imageHit struct {
	id  string
	ref imageRef
}

type modelHit struct {
	id            string
	remote, trust bool
}

// scan is the state gathered for one workload.
type scan struct {
	path        string
	primary     map[string]*product // component ID → product
	accel       map[string]bool
	images      []imageHit
	models      []modelHit
	privileges  []privilege
	gpuRequest  bool
	gpuNodes    bool
	visibleAll  bool
	noLimit     bool
	shared      bool
	jupyterOpen bool
}

func (a *analyzer) scanWorkload(w workload) {
	s := &scan{path: w.path, primary: map[string]*product{}, accel: map[string]bool{}}
	for _, pod := range w.pods {
		for _, c := range pod.InitContainers {
			a.scanContainer(s, c, "initContainers["+clean(c.Name)+"]")
		}
		for _, c := range pod.Containers {
			a.scanContainer(s, c, "containers["+clean(c.Name)+"]")
		}
		a.scanPod(s, pod)
	}
	for _, m := range w.models {
		a.addStorageModel(s, m)
	}
	a.addKindProducts(s, w.ref.Kind)
	a.applyRisks(s, w)
}

// addKindProducts records products implied by a custom resource kind,
// unless an image already identified the same product with a version.
// extractWorkload has already checked the API group.
func (a *analyzer) addKindProducts(s *scan, kind string) {
	seen := map[*product]bool{}
	for _, p := range s.primary {
		seen[p] = true
	}
	for _, p := range catalog().kinds[kind] {
		if !seen[p] {
			a.addProduct(s, p, found{}, "kind "+kind)
		}
	}
}

// addProduct records a product component. extra supplies version and
// properties when an image was seen.
func (a *analyzer) addProduct(s *scan, p *product, extra found, detail string) string {
	f := extra
	f.kind, f.supplier = p.Kind, p.Supplier
	if f.name == "" {
		f.name = p.Name
	}
	id := a.b.add(f, s.path, detail)
	s.primary[id] = p
	return id
}

func (a *analyzer) applyRisks(s *scan, w workload) {
	targets := sortedKeys(s.primary)
	if len(targets) > 0 {
		a.exposureRisks(s, a.expose.find(w.ref.Namespace, w.labels))
	} else {
		targets = sortedKeys(s.accel)
	}
	if len(s.privileges) > 0 {
		raised, reasons := false, []string{}
		for _, p := range s.privileges {
			raised = raised || p.raised
			reasons = append(reasons, p.reason)
		}
		for _, id := range targets {
			a.b.risk(id, riskPrivileged, raised, s.path, strings.Join(sortedUnique(reasons), ", "))
		}
	}
	for _, img := range s.images {
		if img.ref.unpinned() {
			a.b.risk(img.id, riskUnpinnedImage, false, s.path, "image "+img.ref.repo()+tagSuffix(img.ref))
		}
	}
	for _, m := range s.models {
		if m.remote {
			a.b.risk(m.id, riskRemoteDownload, m.trust, s.path, trustReason(m.trust))
		}
	}
	a.gpuRisks(s)
}

func tagSuffix(r imageRef) string {
	if r.Tag == "" {
		return " (no tag)"
	}
	return ":" + r.Tag
}

func trustReason(trust bool) string {
	if trust {
		return "--trust-remote-code runs code from the downloaded repository"
	}
	return ""
}

func (a *analyzer) exposureRisks(s *scan, exps []exposure) {
	for _, id := range sortedKeys(s.primary) {
		p := s.primary[id]
		riskID, open := riskExposedService, p.Open
		switch {
		case p.Name == "jupyter":
			riskID, open = riskNotebookExposed, s.jupyterOpen
		case p.Kind == aibom.KindServing:
			riskID = riskExposedInference
		}
		if p.Name == "jupyter" && s.jupyterOpen && len(exps) == 0 {
			a.b.risk(id, riskNotebookExposed, true, s.path, "started with an empty token and no password")
		}
		for _, e := range exps {
			reason := e.reason()
			switch {
			case open && p.Name == "jupyter":
				reason += "; started with an empty token and no password"
			case open:
				reason += "; " + p.Name + " answers without authentication by default"
			}
			a.b.risk(id, riskID, open, s.path, reason)
			a.b.setProps(id, map[string]string{"exposed": e.kind, "exposed_host": e.host, "exposed_port": e.port})
		}
	}
}

func (a *analyzer) gpuRisks(s *scan) {
	var reasons []string
	if s.noLimit {
		reasons = append(reasons, "a GPU is requested without a limit, which Kubernetes rejects for extended resources")
	}
	if s.gpuNodes && !s.gpuRequest {
		reasons = append(reasons, "placed on GPU nodes without requesting a GPU")
	}
	if s.visibleAll && !s.gpuRequest {
		reasons = append(reasons, "NVIDIA_VISIBLE_DEVICES=all without a GPU request exposes every GPU on the node")
	}
	for _, id := range sortedKeys(s.accel) {
		for _, r := range reasons {
			a.b.risk(id, riskGPUWithoutLimits, false, s.path, r)
		}
		if s.shared {
			a.b.risk(id, riskSharedGPU, false, s.path, "")
		}
	}
}
