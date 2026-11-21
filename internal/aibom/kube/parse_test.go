package kube

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

func mustParse(t *testing.T, input string) aibom.BOM {
	t.Helper()
	bom, err := Parse(strings.NewReader(input), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return bom
}

func find(bom aibom.BOM, kind aibom.Kind, name string) (aibom.Component, bool) {
	for _, c := range bom.Components {
		if c.Kind == kind && c.Name == name {
			return c, true
		}
	}
	return aibom.Component{}, false
}

func mustFind(t *testing.T, bom aibom.BOM, kind aibom.Kind, name string) aibom.Component {
	t.Helper()
	c, ok := find(bom, kind, name)
	if !ok {
		t.Fatalf("no %s %q in %+v", kind, name, bom.Components)
	}
	return c
}

func riskOf(c aibom.Component, id string) (aibom.Risk, bool) {
	for _, r := range c.Risks {
		if r.ID == id {
			return r, true
		}
	}
	return aibom.Risk{}, false
}

func mustRisk(t *testing.T, c aibom.Component, id string, sev aibom.Severity) aibom.Risk {
	t.Helper()
	r, ok := riskOf(c, id)
	if !ok {
		t.Fatalf("%s %q has no %s risk: %+v", c.Kind, c.Name, id, c.Risks)
	}
	if r.Severity != sev {
		t.Fatalf("%s on %q: severity %s, want %s", id, c.Name, r.Severity, sev)
	}
	return r
}

func TestParseFindsVLLMServingItsModelOnAGPUBehindALoadBalancer(t *testing.T) {
	bom := mustParse(t, vllmFixture)

	vllm := mustFind(t, bom, aibom.KindServing, "vllm")
	if vllm.Version != "v0.6.3" || vllm.Properties["digest_pinned"] != "false" || vllm.Properties["exposed"] != "LoadBalancer" ||
		vllm.Properties["exposed_host"] != "a1b2.elb.amazonaws.com" || vllm.Properties["exposed_port"] != "8000" {
		t.Fatalf("vllm = %+v", vllm)
	}
	mustRisk(t, vllm, riskExposedInference, aibom.SeverityHigh)
	model := mustFind(t, bom, aibom.KindModel, "meta-llama/Llama-3.1-8B-Instruct")
	if model.Ecosystem != "huggingface" || model.Properties["served_as"] != "llama3" || !strings.Contains(model.Properties["source"], "downloaded") {
		t.Fatalf("model = %+v", model)
	}
	mustRisk(t, model, riskRemoteDownload, aibom.SeverityLow)
	gpu := mustFind(t, bom, aibom.KindAccelerator, "nvidia.com/gpu")
	if gpu.Evidence[0].Detail != "containers[vllm].resources nvidia.com/gpu=1" {
		t.Fatalf("gpu evidence = %+v", gpu.Evidence)
	}
	cred := mustFind(t, bom, aibom.KindCredential, "HF_TOKEN")
	if cred.Properties["reference"] != "secret hf/token" || len(cred.Risks) != 0 {
		t.Fatalf("HF_TOKEN from a Secret should be recorded without risk: %+v", cred)
	}
	mustFind(t, bom, aibom.KindProvider, "huggingface")
}

func TestParseCountsADeploymentAndItsPodsAsOneWorkload(t *testing.T) {
	bom := mustParse(t, vllmFixture)

	for _, c := range bom.Components {
		for _, e := range c.Evidence {
			if e.Path != "ai/Deployment/vllm" {
				t.Errorf("%s %q has evidence on %q, want the owning Deployment", c.Kind, c.Name, e.Path)
			}
		}
	}
	if vllm := mustFind(t, bom, aibom.KindServing, "vllm"); len(vllm.Evidence) != 1 {
		t.Fatalf("Deployment and Pod produced duplicate evidence: %+v", vllm.Evidence)
	}
}

func TestParseFollowsOwnersThroughJobsAndPresentReplicaSets(t *testing.T) {
	input := `{"kind":"CronJob","apiVersion":"batch/v1","metadata":{"name":"train","namespace":"ml"},"spec":{"jobTemplate":{"spec":{"template":{"spec":{"containers":[{"name":"t","image":"rayproject/ray:2.9.0"}]}}}}}}
{"kind":"Job","apiVersion":"batch/v1","metadata":{"name":"train-123","namespace":"ml","ownerReferences":[{"kind":"CronJob","name":"train"}]},"spec":{"template":{"spec":{"containers":[{"name":"t","image":"rayproject/ray:2.9.0"}]}}}}
{"kind":"Pod","apiVersion":"v1","metadata":{"name":"train-123-x","namespace":"ml","ownerReferences":[{"kind":"Job","name":"train-123","controller":true}]},"spec":{"containers":[{"name":"t","image":"rayproject/ray:2.9.0"}]}}
{"kind":"Pod","apiVersion":"v1","metadata":{"name":"orphan","namespace":"ml","labels":{"pod-template-hash":"zzz"},"ownerReferences":[{"kind":"ReplicaSet","name":"other-abc"}]},"spec":{"containers":[{"name":"t","image":"rayproject/ray:2.9.0"}]}}`

	ray := mustFind(t, mustParse(t, input), aibom.KindPlatform, "ray")

	var paths []string
	for _, e := range ray.Evidence {
		paths = append(paths, e.Path)
	}
	if strings.Join(paths, " ") != "ml/CronJob/train ml/ReplicaSet/other-abc" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestParseFindsOllamaBehindAnIngressAndIgnoresAnExporterNamedLikeIt(t *testing.T) {
	bom := mustParse(t, ollamaFixture)

	ollama := mustFind(t, bom, aibom.KindServing, "ollama")
	r := mustRisk(t, ollama, riskExposedInference, aibom.SeverityHigh)
	if !strings.Contains(r.Summary, "Ingress llm/Ingress/ollama host ollama.example.com port 11434") {
		t.Fatalf("summary = %q", r.Summary)
	}
	mustRisk(t, ollama, riskUnpinnedImage, aibom.SeverityLow)
	model := mustFind(t, bom, aibom.KindModel, "llama3.1")
	if model.Version != "8b" || model.Ecosystem != "ollama" {
		t.Fatalf("model = %+v", model)
	}
	for _, c := range bom.Components {
		for _, e := range c.Evidence {
			if strings.Contains(e.Path, "exporter") {
				t.Fatalf("ollama-exporter was reported: %+v", c)
			}
		}
	}
}

func TestParseReadsAKServeInferenceServiceAndItsStorageURI(t *testing.T) {
	bom := mustParse(t, kserveFixture)

	model := mustFind(t, bom, aibom.KindModel, "meta-llama/Llama-3.2-1B-Instruct")
	if model.Properties["format"] != "huggingface" || model.Evidence[0].Detail != "spec.predictor.model.storageUri" ||
		model.Evidence[0].Path != "kserve-test/InferenceService/llama" {
		t.Fatalf("model = %+v", model)
	}
	var kserve aibom.Component
	for _, c := range bom.Components {
		if c.Name == "kserve" && c.Version == "v0.14.0" {
			kserve = c
		}
	}
	if kserve.Properties["digest_pinned"] != "true" || kserve.Evidence[0].Path != "kserve-test/InferenceService/llama" {
		t.Fatalf("kserve runtime image not attributed to the InferenceService: %+v", bom.Components)
	}
	if _, ok := riskOf(kserve, riskUnpinnedImage); ok {
		t.Fatal("a digest-pinned image was called unpinned")
	}
	mustFind(t, bom, aibom.KindAccelerator, "nvidia.com/gpu")
}

func TestParseFlagsAJupyterNotebookWithAnEmptyTokenOnANodePort(t *testing.T) {
	bom := mustParse(t, notebookFixture)

	nb := mustFind(t, bom, aibom.KindPlatform, "jupyter")
	r := mustRisk(t, nb, riskNotebookExposed, aibom.SeverityHigh)
	if !strings.Contains(r.Summary, "empty token") || nb.Properties["exposed_port"] != "30888" {
		t.Fatalf("notebook = %+v", nb)
	}
	mustRisk(t, nb, riskPrivileged, aibom.SeverityMedium)
	mustRisk(t, mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY"), riskLiteralCredential, aibom.SeverityHigh)
}

func TestParseFindsQdrantAndRecordsDigestPinning(t *testing.T) {
	bom := mustParse(t, qdrantFixture)

	q := mustFind(t, bom, aibom.KindDataStore, "qdrant")
	if q.Properties["digest_pinned"] != "true" || len(q.Risks) != 0 || len(bom.Components) != 1 {
		t.Fatalf("bom = %+v", bom)
	}
}

func TestParseFindsNothingInAnOrdinaryWebApp(t *testing.T) {
	bom := mustParse(t, plainFixture)

	if len(bom.Components) != 0 || len(bom.Notes) != 0 {
		t.Fatalf("nginx + postgres produced %+v", bom)
	}
}

func TestCredentialValuesNeverAppearInOutput(t *testing.T) {
	literalArg := `{"kind":"Pod","apiVersion":"v1","metadata":{"name":"p","namespace":"n"},"spec":{"containers":[{"name":"c","image":"vllm/vllm-openai:v1","args":["--api-key","` + secretValue + `"]}]}}`
	secret := `{"kind":"Secret","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"data":{"OPENAI_API_KEY":"` + secretValue + `"}}`
	for _, input := range []string{notebookFixture, configMapFixture, literalArg, secret} {
		out, err := json.Marshal(mustParse(t, input))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), secretValue) || strings.Contains(string(out), "DO-NOT-LEAK") {
			t.Fatalf("credential value leaked: %s", out)
		}
	}
	c := mustFind(t, mustParse(t, literalArg), aibom.KindCredential, "--api-key")
	mustRisk(t, c, riskLiteralCredential, aibom.SeverityHigh)
}

func TestParseFindsCredentialsModelsAndProvidersInConfigMaps(t *testing.T) {
	bom := mustParse(t, configMapFixture)

	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		mustRisk(t, mustFind(t, bom, aibom.KindCredential, name), riskConfigMapCred, aibom.SeverityHigh)
	}
	if hf := mustFind(t, bom, aibom.KindCredential, "HF_TOKEN"); len(hf.Risks) != 0 {
		t.Fatalf("a ${HF_TOKEN} reference is not a literal: %+v", hf)
	}
	if m := mustFind(t, bom, aibom.KindModel, "mistralai/Mistral-7B-Instruct-v0.3"); m.Properties["configured_in"] != "ConfigMap" {
		t.Fatalf("model = %+v", m)
	}
	if p := mustFind(t, bom, aibom.KindProvider, "openai"); !strings.Contains(p.Properties["host"], "api.openai.com") {
		t.Fatalf("provider = %+v", p)
	}
}

func TestParseOutputIsDeterministicWhateverTheInputOrder(t *testing.T) {
	docs := []string{vllmFixture, ollamaFixture, kserveFixture, notebookFixture, qdrantFixture, configMapFixture}
	forward := strings.Join(docs, "\n")
	var reversed []string
	for i := len(docs) - 1; i >= 0; i-- {
		reversed = append(reversed, docs[i])
	}

	a, _ := json.Marshal(mustParse(t, forward))
	b, _ := json.Marshal(mustParse(t, forward))
	c, _ := json.Marshal(mustParse(t, strings.Join(reversed, "\n")))

	if string(a) != string(b) || string(a) != string(c) {
		t.Fatal("output differs between runs or input orders")
	}
}
