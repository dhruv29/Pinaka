package kube

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// maxConfigLines bounds how many lines of one ConfigMap value are read.
const maxConfigLines = 5000

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

// Credential values flow only into isLiteral and nowhere else: the BOM
// records the NAME of a credential and how it is supplied, never the value.

func credentialProvider(name string) (string, bool) {
	upper := strings.ToUpper(name)
	if p, ok := credentialNames[upper]; ok {
		return p, true
	}
	for prefix, provider := range credentialPrefixes {
		if !strings.HasPrefix(upper, prefix) {
			continue
		}
		for _, suffix := range credentialSuffixes {
			if strings.HasSuffix(upper, suffix) {
				return provider, true
			}
		}
	}
	return "", false
}

// isLiteral says whether a value is an actual secret rather than empty or
// a reference to one (${VAR}, $(VAR), os.environ/VAR, {{ template }}).
func isLiteral(v string) bool {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	if v == "" || v == "null" || v == "~" {
		return false
	}
	for _, ref := range []string{"${", "$(", "os.environ/", "{{", "<"} {
		if strings.HasPrefix(v, ref) {
			return false
		}
	}
	return true
}

func (a *analyzer) scanEnvVar(s *scan, e envVar, name string) {
	if !envNamePattern.MatchString(e.Name) {
		return
	}
	detail := name + ".env " + e.Name
	if provider, ok := credentialProvider(e.Name); ok {
		reference, riskID, reason := envReference(e)
		id := a.addCredential(s.path, e.Name, provider, reference, detail)
		if riskID != "" {
			a.b.risk(id, riskID, true, s.path, reason)
		}
		return
	}
	if e.ValueFrom != nil || e.Value == "" {
		return
	}
	if e.Name == visibleDevicesEnv && e.Value == "all" {
		s.visibleAll = true
		s.accel[a.b.add(found{kind: aibom.KindAccelerator, name: "nvidia.com/gpu", supplier: "NVIDIA"}, s.path, detail)] = true
	}
	a.scanProviderURL(s.path, e.Name, e.Value, detail)
}

// envReference describes how a credential env var is supplied, and the
// risk that carries.
func envReference(e envVar) (reference, riskID, reason string) {
	switch {
	case e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil:
		r := e.ValueFrom.SecretKeyRef
		return "secret " + clean(r.Name) + "/" + clean(r.Key), "", ""
	case e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil:
		r := e.ValueFrom.ConfigMapKeyRef
		return "configmap " + clean(r.Name) + "/" + clean(r.Key), riskConfigMapCred, e.Name + " read from ConfigMap " + clean(r.Name)
	case e.ValueFrom != nil:
		return "other valueFrom source", "", ""
	case isLiteral(e.Value):
		return "literal value", riskLiteralCredential, e.Name + " set as a literal env value"
	}
	return "empty value", "", ""
}

func (a *analyzer) addCredential(path, name, provider, reference, detail string) string {
	a.b.add(found{kind: aibom.KindProvider, name: provider}, path, detail)
	return a.b.add(found{kind: aibom.KindCredential, name: strings.ToUpper(name),
		props: map[string]string{"provider": provider, "reference": reference}}, path, detail)
}

// scanProviderURL records a provider when an endpoint setting points at a
// known AI API host. Only the host is kept, never the full value.
func (a *analyzer) scanProviderURL(path, name, value, detail string) {
	upper := strings.ToUpper(name)
	isURLName := false
	for _, suffix := range urlNameSuffixes {
		isURLName = isURLName || strings.HasSuffix(upper, suffix)
	}
	if !isURLName {
		return
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return
	}
	host := strings.ToLower(u.Hostname())
	for suffix, provider := range providerHosts {
		if host == suffix || strings.HasSuffix(host, "."+suffix) || strings.HasSuffix(host, "-"+suffix) {
			a.b.add(found{kind: aibom.KindProvider, name: provider, props: map[string]string{"host": host}}, path, detail)
			return
		}
	}
}

// scanConfigMap reads keys, and KEY=value / key: value lines inside values,
// with the same rules as env vars. ConfigMaps are not Secrets, so any
// credential found here is a risk.
func (a *analyzer) scanConfigMap(o object) {
	var data map[string]string
	if err := decodeInto(o.Data, &data); err != nil {
		a.note("skipped " + refOf(o).path() + ": its data is not in the expected shape")
		return
	}
	path := refOf(o).path()
	for _, key := range sortedKeys(data) {
		field := "data[" + clean(key) + "]"
		a.scanConfigEntry(path, key, data[key], field)
		for _, kv := range keyValueLines(data[key]) {
			a.scanConfigEntry(path, kv[0], kv[1], field+" "+kv[0])
		}
	}
}

func (a *analyzer) scanConfigEntry(path, name, value, detail string) {
	if !envNamePattern.MatchString(name) {
		return
	}
	if provider, ok := credentialProvider(name); ok {
		if !isLiteral(value) {
			a.addCredential(path, name, provider, "reference in ConfigMap", detail)
			return
		}
		id := a.addCredential(path, name, provider, "literal value in ConfigMap", detail)
		a.b.risk(id, riskConfigMapCred, true, path, strings.ToUpper(name)+" stored in "+detail)
		return
	}
	if !isLiteral(value) {
		return
	}
	if modelEnvNames[strings.ToUpper(name)] {
		ref, ok := classifyModel(candidate{value: strings.Trim(value, `"'`)}, modelContext{})
		if ok {
			a.b.add(found{kind: aibom.KindModel, name: ref.name, version: ref.version, supplier: ref.supplier, ecosystem: ref.ecosystem,
				props: map[string]string{"source": ref.source, "configured_in": "ConfigMap"}}, path, detail)
		}
		return
	}
	a.scanProviderURL(path, name, strings.Trim(value, `"'`), detail)
}

// keyValueLines extracts NAME=value and name: value pairs from a config
// file held in a ConfigMap value.
func keyValueLines(v string) [][2]string {
	if !strings.Contains(v, "\n") && !strings.ContainsAny(v, "=:") {
		return nil
	}
	var out [][2]string
	for i, line := range strings.Split(v, "\n") {
		if i == maxConfigLines {
			break
		}
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if k, val, okColon := strings.Cut(line, ":"); okColon && (!ok || len(k) < len(name)) {
			name, value, ok = k, val, true
		}
		if ok {
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(name, "- ")), `"'`)
			out = append(out, [2]string{name, strings.TrimSuffix(strings.TrimSpace(value), ",")})
		}
	}
	return out
}
