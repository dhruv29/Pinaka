package kube

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// maxConfigLines bounds how many lines of one ConfigMap value are read.
const maxConfigLines = 5000

// minSecretLength is the shortest literal treated as a real key. Shorter
// values are placeholders such as "x" or "abc".
const minSecretLength = 8

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

// placeholderValues are what self-hosted, OpenAI-compatible servers (vLLM,
// Ollama, llama.cpp, LM Studio) tell clients to send as a key. They are not
// secrets, and flagging them would bury the real ones.
var placeholderValues = map[string]bool{
	"empty": true, "none": true, "null": true, "dummy": true, "ollama": true, "sk-no-key-required": true,
	"no-key": true, "nokey": true, "not-needed": true, "not-used": true, "unused": true, "n/a": true,
	"placeholder": true, "changeme": true, "fake": true, "test": true, "lm-studio": true, "sk-xxx": true,
}

// Credential values flow only into isLiteral and isSecretValue and nowhere
// else: the BOM records the NAME of a credential and how it is supplied,
// never the value.

// credentialInfo returns the shared table entry for a credential name, also
// catching families such as AZURE_OPENAI_<anything>_KEY.
func credentialInfo(name string) (aibom.Credential, bool) {
	upper := strings.ToUpper(name)
	if c, ok := aibom.Credentials[upper]; ok {
		return c, true
	}
	for prefix, known := range credentialPrefixes {
		if !strings.HasPrefix(upper, prefix) {
			continue
		}
		for _, suffix := range credentialSuffixes {
			if strings.HasSuffix(upper, suffix) {
				return aibom.Credentials[known], true
			}
		}
	}
	return aibom.Credential{}, false
}

// isLiteral says whether a value is written out rather than empty or a
// reference to one (${VAR}, $(VAR), os.environ/VAR, {{ template }}).
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

// isSecretValue says whether a literal can be a real key rather than a
// placeholder a self-hosted server expects.
func isSecretValue(v string) bool {
	if !isLiteral(v) {
		return false
	}
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	return len(v) >= minSecretLength && !placeholderValues[strings.ToLower(v)]
}

func (a *analyzer) scanEnvVar(s *scan, e envVar, name string) {
	if !envNamePattern.MatchString(e.Name) {
		return
	}
	detail := name + ".env " + e.Name
	if info, ok := credentialInfo(e.Name); ok {
		reference, riskID, reason := envReference(e)
		id := a.addCredential(s.path, e.Name, info, reference, detail, !s.redirected)
		if riskID != "" {
			a.b.riskAt(id, riskID, true, aibom.Evidence{Path: s.path, Detail: detail}, reason)
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
	case isSecretValue(e.Value):
		return "literal value", riskLiteralCredential, e.Name + " set as a literal env value"
	case isLiteral(e.Value):
		return "placeholder value", "", ""
	}
	return "empty value", "", ""
}

// addCredential records a credential by name. The hosted provider the key
// is for is listed too, unless inferProvider is false because the client
// is pointed at another address.
func (a *analyzer) addCredential(path, name string, info aibom.Credential, reference, detail string, inferProvider bool) string {
	if p, ok := aibom.ProviderNamed(info.Provider); ok && inferProvider {
		a.b.add(found{kind: aibom.KindProvider, name: p.Name, supplier: p.Supplier}, path, detail+" (inferred from the credential name)")
	}
	return a.b.add(found{kind: aibom.KindCredential, name: strings.ToUpper(name), supplier: info.Supplier,
		props: map[string]string{"provider": info.Provider, "reference": reference}}, path, detail)
}

// isURLName says a setting holds an endpoint, so its value may be parsed
// for a host. Other values are never looked at.
func isURLName(name string) bool {
	upper := strings.ToUpper(name)
	for _, suffix := range urlNameSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// isAIEndpointName says an endpoint setting is for an AI client, so a host
// that is no vendor's is still worth listing. DATABASE_URL is not.
func isAIEndpointName(name string) bool {
	upper := strings.ToUpper(name)
	if !isURLName(upper) {
		return false
	}
	for _, word := range aiEndpointWords {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

// endpointHost returns host[:port] of an http(s) URL setting, or "".
func endpointHost(name, value string) string {
	if !isURLName(name) {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(strings.Trim(value, `"'`)))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

// redirectsAIClient reports an AI endpoint setting pointed at a host that is
// no vendor's: the client's key then goes there, not to the vendor its name
// suggests.
func redirectsAIClient(name, value string) bool {
	host := endpointHost(name, value)
	if host == "" || !isAIEndpointName(name) {
		return false
	}
	_, vendor := aibom.ProviderForHost(host)
	return !vendor
}

// scanProviderURL records the provider an endpoint setting points at: a
// known AI API host, or for an AI client's setting any other host as a
// custom endpoint. Only the host is kept, never the full value.
func (a *analyzer) scanProviderURL(path, name, value, detail string) {
	host := endpointHost(name, value)
	if host == "" {
		return
	}
	if p, ok := aibom.ProviderForHost(host); ok {
		a.b.add(found{kind: aibom.KindProvider, name: p.Name, supplier: p.Supplier, props: map[string]string{"host": host}}, path, detail)
		return
	}
	if !isAIEndpointName(name) {
		return
	}
	scope, raised := "private", false
	if !aibom.IsPrivateHost(host) {
		scope, raised = "external", true
	}
	id := a.b.add(found{kind: aibom.KindProvider, name: aibom.CustomEndpointName(host), props: map[string]string{"host": host, "scope": scope}}, path, detail)
	a.b.riskAt(id, riskCustomEndpoint, raised, aibom.Evidence{Path: path, Detail: detail}, name+" points at "+scope+" host "+host)
}

// configEntry is one setting read from a ConfigMap.
type configEntry struct {
	name, value, detail string
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
	var entries []configEntry
	for _, key := range sortedKeys(data) {
		field := "data[" + clean(key) + "]"
		entries = append(entries, configEntry{key, data[key], field})
		lines, truncated := keyValueLines(data[key])
		if truncated {
			a.note(fmt.Sprintf("%s %s: only the first %d lines were read", path, field, maxConfigLines))
		}
		for _, kv := range lines {
			entries = append(entries, configEntry{kv[0], kv[1], field + " " + kv[0]})
		}
	}
	redirected := false
	for _, e := range entries {
		redirected = redirected || redirectsAIClient(e.name, e.value)
	}
	for _, e := range entries {
		a.scanConfigEntry(path, e, !redirected)
	}
}

func (a *analyzer) scanConfigEntry(path string, e configEntry, inferProvider bool) {
	name, value, detail := e.name, e.value, e.detail
	if !envNamePattern.MatchString(name) {
		return
	}
	if info, ok := credentialInfo(name); ok {
		switch {
		case isSecretValue(value):
			id := a.addCredential(path, name, info, "literal value in ConfigMap", detail, inferProvider)
			a.b.riskAt(id, riskConfigMapCred, true, aibom.Evidence{Path: path, Detail: detail}, strings.ToUpper(name)+" stored in "+detail)
		case isLiteral(value):
			a.addCredential(path, name, info, "placeholder value in ConfigMap", detail, inferProvider)
		default:
			a.addCredential(path, name, info, "reference in ConfigMap", detail, inferProvider)
		}
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
	a.scanProviderURL(path, name, value, detail)
}

// keyValueLines extracts NAME=value and name: value pairs from a config
// file held in a ConfigMap value, and reports whether it stopped at the
// line cap.
func keyValueLines(v string) ([][2]string, bool) {
	if !strings.Contains(v, "\n") && !strings.ContainsAny(v, "=:") {
		return nil, false
	}
	var out [][2]string
	for i, line := range strings.Split(v, "\n") {
		if i == maxConfigLines {
			return out, true
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
	return out, false
}
