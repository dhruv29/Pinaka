package kube

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	// maxTokens bounds how much of a command line is examined.
	maxTokens = 4096
	rootUID   = 0
)

var (
	quantityPattern = regexp.MustCompile(`^[0-9]{1,6}$`)
	mcpPackage      = regexp.MustCompile(`^(@modelcontextprotocol/server-[a-z0-9-]+|mcp-server-[a-z0-9-]+|@[a-z0-9-]+/mcp-server-[a-z0-9-]+)(@[A-Za-z0-9.^~-]+)?$`)
)

func (a *analyzer) scanContainer(s *scan, c container, name string) {
	tokens, truncated := commandTokens(c)
	if truncated {
		a.note(fmt.Sprintf("%s %s: the command line has more than %d words; the rest was not examined", s.path, name, maxTokens))
	}
	prod := a.scanImage(s, c.Image, name)
	for _, p := range matchCommand(tokens) {
		if p == prod {
			continue // the image already said so, with a version
		}
		a.addProduct(s, p, found{}, name+".command "+strings.Join(p.Commands[0], " "))
		if prod == nil {
			prod = p
		}
	}
	a.scanModels(s, c, tokens, prod, name)
	s.redirected = false
	for _, e := range c.Env {
		if e.ValueFrom == nil && redirectsAIClient(e.Name, e.Value) {
			s.redirected = true
		}
	}
	for _, e := range c.Env {
		a.scanEnvVar(s, e, name)
	}
	s.redirected = false
	a.scanArgCredentials(s, tokens, name)
	a.scanAgentArgs(s, tokens, name)
	a.scanResources(s, c.Resources, name)
	s.privileges = append(s.privileges, containerPrivileges(c.SecurityContext, name)...)
	if prod != nil && prod.Name == "jupyter" && jupyterUnauthenticated(tokens) {
		s.jupyterOpen = true
	}
}

// commandTokens splits command and args into words, also splitting shell
// strings such as `sh -c "ollama serve & ollama pull llama3"`, and reports
// whether it stopped at the word cap.
func commandTokens(c container) ([]string, bool) {
	var out []string
	for _, part := range append(append([]string{}, c.Command...), c.Args...) {
		for _, f := range strings.FieldsFunc(part, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == ';' || r == '&' || r == '|' }) {
			if len(out) == maxTokens {
				return out, true
			}
			out = append(out, strings.Trim(f, `"'`))
		}
	}
	return out, false
}

func (a *analyzer) scanImage(s *scan, image, name string) *product {
	ref, ok := parseImage(image)
	if !ok {
		return nil
	}
	p, confidence, ok := matchImage(ref)
	if !ok {
		return nil
	}
	f := found{version: ref.Tag, ecosystem: "oci", props: map[string]string{
		"image": ref.repo(), "digest_pinned": fmt.Sprint(ref.Digest != ""), "match_confidence": confidence}}
	if p.NameFromImage {
		f.name = ref.Path
	}
	id := a.addProduct(s, p, f, name+".image")
	s.images = append(s.images, imageHit{id: id, ref: ref})
	if p.ModelInImage {
		for _, pre := range p.Prefixes {
			if model := strings.TrimPrefix(ref.repo(), pre); model != ref.repo() && model != "" {
				a.b.add(found{kind: aibom.KindModel, name: model, ecosystem: "ngc", supplier: strings.Split(model, "/")[0],
					props: map[string]string{"source": "NVIDIA NGC (named by the NIM image)", "served_by": p.Name}}, s.path, name+".image")
			}
		}
	}
	return p
}

// flagValues returns the values of a flag written as "--f v" or "--f=v".
func flagValues(tokens []string, flag string) []string {
	var out []string
	for i, t := range tokens {
		switch {
		case t == flag && i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-"):
			out = append(out, tokens[i+1])
		case strings.HasPrefix(t, flag+"="):
			out = append(out, strings.Trim(strings.TrimPrefix(t, flag+"="), `"'`))
		}
	}
	return out
}

func hasToken(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}

// scanArgCredentials flags a credential passed literally on the command
// line, such as `vllm serve --api-key …`. The value is never recorded.
func (a *analyzer) scanArgCredentials(s *scan, tokens []string, name string) {
	for _, flag := range credentialFlags {
		for _, v := range flagValues(tokens, flag) {
			if !isSecretValue(v) {
				continue
			}
			id := a.b.add(found{kind: aibom.KindCredential, name: flag, props: map[string]string{"reference": "literal command-line argument"}}, s.path, name+".args "+flag)
			a.b.riskAt(id, riskLiteralCredential, true, aibom.Evidence{Path: s.path, Detail: name + ".args " + flag}, "passed as the "+flag+" argument")
		}
	}
}

// scanAgentArgs finds MCP servers started from a package, e.g.
// `npx -y @modelcontextprotocol/server-github`.
func (a *analyzer) scanAgentArgs(s *scan, tokens []string, name string) {
	for _, t := range tokens {
		m := mcpPackage.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		p := &product{Name: m[1], Kind: aibom.KindAgentTool, Open: true}
		a.addProduct(s, p, found{ecosystem: "npm-or-pypi"}, name+".args "+m[1])
	}
}

func (a *analyzer) scanResources(s *scan, r resources, name string) {
	set := map[string]bool{}
	for k := range r.Limits {
		set[k] = true
	}
	for k := range r.Requests {
		set[k] = true
	}
	for _, res := range sortedKeys(set) {
		supplier, ok := acceleratorSupplier(res)
		if !ok {
			continue
		}
		count, limited := r.Limits[res]
		if !limited {
			count = r.Requests[res]
			s.noLimit = true
		}
		q := string(count)
		if !quantityPattern.MatchString(q) {
			q = "?"
		}
		s.accel[a.b.add(found{kind: aibom.KindAccelerator, name: res, supplier: supplier}, s.path, name+".resources "+res+"="+q)] = true
		s.gpuRequest = true
		s.shared = s.shared || strings.HasSuffix(res, sharedGPUSuffix)
	}
}

func acceleratorSupplier(res string) (string, bool) {
	for _, r := range acceleratorRules {
		if res == r.Resource || (r.Prefix && strings.HasPrefix(res, r.Resource)) {
			return r.Supplier, true
		}
	}
	return "", false
}

// scanPod looks at pod-level placement and privileges.
func (a *analyzer) scanPod(s *scan, pod podSpec) {
	for _, key := range sortedKeys(pod.NodeSelector) {
		a.gpuPlacement(s, key, "spec.nodeSelector "+key)
	}
	keys := []string{}
	for _, t := range pod.Tolerations {
		keys = append(keys, t.Key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		a.gpuPlacement(s, key, "spec.tolerations "+key)
	}
	s.privileges = append(s.privileges, podPrivileges(pod)...)
}

func (a *analyzer) gpuPlacement(s *scan, key, detail string) {
	res, ok := gpuNodeKeys[key]
	if !ok {
		return
	}
	supplier, _ := acceleratorSupplier(res)
	s.accel[a.b.add(found{kind: aibom.KindAccelerator, name: res, supplier: supplier}, s.path, detail)] = true
	s.gpuNodes = true
}

func podPrivileges(pod podSpec) []privilege {
	var out []privilege
	if pod.HostPID {
		out = append(out, privilege{true, "hostPID"})
	}
	if pod.HostNetwork {
		out = append(out, privilege{false, "hostNetwork"})
	}
	if pod.HostIPC {
		out = append(out, privilege{false, "hostIPC"})
	}
	for _, v := range pod.Volumes {
		if v.HostPath != nil {
			out = append(out, privilege{false, "hostPath " + clean(v.HostPath.Path)})
		}
	}
	if sc := pod.SecurityContext; sc != nil && sc.RunAsUser != nil && *sc.RunAsUser == rootUID {
		out = append(out, privilege{false, "pod runs as root (runAsUser 0)"})
	}
	return out
}

func containerPrivileges(sc *securityContext, name string) []privilege {
	if sc == nil {
		return nil
	}
	var out []privilege
	if sc.Privileged != nil && *sc.Privileged {
		out = append(out, privilege{true, name + " is privileged"})
	}
	if sc.RunAsUser != nil && *sc.RunAsUser == rootUID {
		out = append(out, privilege{false, name + " runs as root (runAsUser 0)"})
	}
	return out
}

// jupyterUnauthenticated is true when the token is set empty and no
// password is given: the server then runs code for anyone who reaches it.
func jupyterUnauthenticated(tokens []string) bool {
	emptyToken, password := false, false
	for _, t := range tokens {
		for _, f := range jupyterTokenFlags {
			if strings.HasPrefix(t, f) && strings.Trim(strings.TrimPrefix(t, f), `"'`) == "" {
				emptyToken = true
			}
		}
		for _, f := range jupyterPasswordFlags {
			if strings.HasPrefix(t, f) && strings.Trim(strings.TrimPrefix(t, f), `"'`) != "" {
				password = true
			}
		}
	}
	return emptyToken && !password
}
