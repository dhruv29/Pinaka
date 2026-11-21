package kube

import "github.com/dhruv29/Pinaka/internal/aibom"

// Risk identifiers.
const (
	riskExposedInference  = "exposed-inference-endpoint"
	riskExposedService    = "exposed-ai-service"
	riskNotebookExposed   = "notebook-exposed"
	riskLiteralCredential = "literal-ai-credential"
	riskConfigMapCred     = "ai-credential-in-configmap"
	riskUnpinnedImage     = "unpinned-image"
	riskPrivileged        = "privileged-ai-workload"
	riskRemoteDownload    = "remote-model-download"
	riskGPUWithoutLimits  = "gpu-without-limits"
	riskSharedGPU         = "shared-gpu"
)

// riskDef is one entry of the risk table. Base is the usual severity;
// Raised applies when the condition in the comment holds for a workload.
type riskDef struct {
	Summary string
	Base    aibom.Severity
	Raised  aibom.Severity
}

var riskTable = map[string]riskDef{
	// Raised: the server answers without authentication by default.
	riskExposedInference: {"An AI inference server is reachable through a LoadBalancer, NodePort, externalIPs, Ingress or HTTPRoute.",
		aibom.SeverityMedium, aibom.SeverityHigh},
	riskExposedService: {"An AI platform, vector store or agent tool is reachable through a LoadBalancer, NodePort, externalIPs, Ingress or HTTPRoute.",
		aibom.SeverityMedium, aibom.SeverityHigh},
	// Raised: the notebook was started with an empty token and no password.
	riskNotebookExposed: {"A Jupyter notebook server runs code for whoever reaches it; it is exposed or has authentication turned off.",
		aibom.SeverityMedium, aibom.SeverityHigh},
	riskLiteralCredential: {"An AI-provider credential is written as a literal value in the pod spec instead of a Secret reference, so anyone who can read the workload can read the key.",
		aibom.SeverityHigh, aibom.SeverityHigh},
	riskConfigMapCred: {"An AI-provider credential is stored in a ConfigMap, which is not a Secret: anyone who can read ConfigMaps in the namespace can read it.",
		aibom.SeverityHigh, aibom.SeverityHigh},
	riskUnpinnedImage: {"The image is referenced as :latest or without a tag and without a digest, so what runs can change on any restart.",
		aibom.SeverityLow, aibom.SeverityLow},
	// Raised: a privileged container or hostPID, which is node takeover.
	riskPrivileged: {"An AI workload runs with host-level privileges (privileged, host namespaces, hostPath mounts or root).",
		aibom.SeverityMedium, aibom.SeverityHigh},
	// Raised: --trust-remote-code, so downloaded repository code is executed.
	riskRemoteDownload: {"Model weights are pulled from the internet at start without a pinned revision, so the model can change under you.",
		aibom.SeverityLow, aibom.SeverityMedium},
	riskGPUWithoutLimits: {"GPU use is not accounted for by the scheduler: a GPU request without a limit, or GPU node placement or NVIDIA_VISIBLE_DEVICES=all without any GPU request.",
		aibom.SeverityInfo, aibom.SeverityInfo},
	riskSharedGPU: {"The workload uses a time-sliced shared GPU resource, which gives no memory or fault isolation between tenants.",
		aibom.SeverityInfo, aibom.SeverityInfo},
}
