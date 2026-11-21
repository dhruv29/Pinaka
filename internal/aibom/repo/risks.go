package repo

import "github.com/dhruv29/Pinaka/internal/aibom"

// Risk identifiers. Each is a short stable slug that reports and tests can
// refer to; the severity and wording live in riskTable below.
const (
	riskPickleRaw       = "pickle-model-file"
	riskPickleTorch     = "pickle-model-file-torch"
	riskUnpinnedModel   = "unpinned-model"
	riskUnpinnedLibrary = "unpinned-library"
	riskRemoteCode      = "remote-code-model"
	riskHardcodedCred   = "hardcoded-ai-credential"
	riskMCPRemote       = "mcp-server-remote-or-shell"
	riskDeserialise     = "insecure-deserialisation"
	riskCustomEndpoint  = "custom-endpoint"
	riskLocalEndpoint   = "custom-endpoint-local"
)

// riskTable is the single place where a risk's severity and explanation are
// set. Severities are deliberately conservative: these are observations a
// security team should look at, not proven vulnerabilities.
var riskTable = map[string]aibom.Risk{
	riskPickleRaw: {
		ID: "pickle-model-file", Severity: aibom.SeverityHigh,
		Summary: "A raw pickle file runs arbitrary code when it is loaded; anyone who can replace it controls the process that loads it.",
	},
	riskPickleTorch: {
		ID: "pickle-model-file", Severity: aibom.SeverityMedium,
		Summary: "A PyTorch-style checkpoint is a pickle inside and can run code when loaded unless it is opened with weights_only=True or converted to safetensors.",
	},
	riskUnpinnedModel: {
		ID: "unpinned-model", Severity: aibom.SeverityLow,
		Summary: "The model is fetched by a floating name or without a pinned revision, so what runs can change without a code change.",
	},
	riskUnpinnedLibrary: {
		ID: "unpinned-library", Severity: aibom.SeverityLow,
		Summary: "The AI library has no version or an open-ended range, so a new release is picked up without review.",
	},
	riskRemoteCode: {
		ID: "remote-code-model", Severity: aibom.SeverityHigh,
		Summary: "trust_remote_code=True runs Python shipped with the model repository, so the model's publisher can run code here.",
	},
	riskHardcodedCred: {
		ID: "hardcoded-ai-credential", Severity: aibom.SeverityHigh,
		Summary: "A literal value that looks like a real key is assigned to this AI credential name in the source tree; it should be rotated and moved to a secret store (the value is not recorded here).",
	},
	riskMCPRemote: {
		ID: "mcp-server-remote-or-shell", Severity: aibom.SeverityMedium,
		Summary: "The MCP server is fetched at run time (npx, uvx and the like), reached over a URL, or started through a shell, so its code is not reviewed or pinned in this repository.",
	},
	riskDeserialise: {
		ID: "insecure-deserialisation", Severity: aibom.SeverityMedium,
		Summary: "A model is loaded with a pickle-based loader (torch.load without weights_only=True, pickle.load or joblib.load), which runs code embedded in the file.",
	},
	riskCustomEndpoint: {
		ID: "custom-endpoint", Severity: aibom.SeverityLow,
		Summary: "The AI client's base URL points at a host that is not the vendor's, so prompts and data go to that third party.",
	},
	riskLocalEndpoint: {
		ID: "custom-endpoint", Severity: aibom.SeverityInfo,
		Summary: "The AI client's base URL points at a local or private host, such as a self-hosted model server.",
	},
}

// risk returns the table entry for key. Every key used in this package is
// in the table; a test checks it.
func risk(key string) aibom.Risk {
	return riskTable[key]
}
