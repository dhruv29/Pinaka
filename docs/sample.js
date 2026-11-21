// A made-up bill of materials in the exact shape "pinaka scan -format json"
// writes. The company, repository and cluster are fictional.
window.PINAKA_SAMPLE = {
  subject: "shopfront + prod-eu cluster",
  components: [
    {
      id: "a1", kind: "model", name: "meta-llama/Llama-3.1-8B-Instruct", origin: "cluster", ecosystem: "huggingface", supplier: "Meta",
      evidence: [{ path: "ml/Deployment/vllm-chat", detail: "args --model" }],
      risks: [{ id: "remote-model-download", severity: "medium", summary: "The weights are downloaded from the internet at start-up with no pinned revision, so the model can change without a deployment." }],
      properties: { source: "huggingface" }
    },
    {
      id: "a2", kind: "model", name: "claude-sonnet-4-5", origin: "repo", supplier: "Anthropic",
      evidence: [{ path: "services/assistant/llm.py", line: 14 }, { path: "services/assistant/config.yaml", line: 6 }],
      properties: { context: "code" }
    },
    {
      id: "a3", kind: "model", name: "gpt-4o-mini", origin: "repo", supplier: "OpenAI",
      evidence: [{ path: "services/search/rerank.ts", line: 31 }],
      properties: { context: "code" }
    },
    {
      id: "a4", kind: "model", name: "fraud_scorer.pkl", origin: "repo",
      evidence: [{ path: "models/fraud_scorer.pkl" }],
      risks: [{ id: "pickle-model-file", severity: "high", summary: "A pickle file can run arbitrary code when it is loaded. Anyone who can change this file can run code wherever the model is served." }],
      properties: { format: "pickle", size: "48 MB" }
    },
    {
      id: "a5", kind: "model", name: "BAAI/bge-small-en-v1.5", origin: "repo", ecosystem: "huggingface", supplier: "BAAI",
      evidence: [{ path: "services/search/embed.py", line: 9, detail: "SentenceTransformer" }],
      risks: [{ id: "unpinned-model", severity: "low", summary: "Loaded by name with no revision, so a new upload under the same name is picked up silently." }]
    },
    {
      id: "b1", kind: "serving", name: "vllm/vllm-openai", version: "v0.6.3", origin: "cluster", ecosystem: "oci",
      evidence: [{ path: "ml/Deployment/vllm-chat", detail: "container image" }],
      risks: [{ id: "exposed-inference-endpoint", severity: "high", summary: "Reachable from outside the cluster through a LoadBalancer service, and this server has no authentication unless one is configured." }],
      properties: { exposed: "LoadBalancer", port: "8000", gpus: "2" }
    },
    {
      id: "b2", kind: "serving", name: "ollama/ollama", version: "latest", origin: "cluster", ecosystem: "oci",
      evidence: [{ path: "labs/StatefulSet/ollama", detail: "container image" }],
      risks: [{ id: "unpinned-image", severity: "low", summary: "The image tag is \"latest\" with no digest, so what runs can change between restarts." }]
    },
    {
      id: "c1", kind: "provider", name: "api.anthropic.com", origin: "repo", supplier: "Anthropic",
      evidence: [{ path: "services/assistant/llm.py", line: 11 }]
    },
    {
      id: "c2", kind: "provider", name: "api.openai.com", origin: "cluster", supplier: "OpenAI",
      evidence: [{ path: "web/Deployment/search-api", detail: "env OPENAI_BASE_URL" }]
    },
    {
      id: "c3", kind: "provider", name: "llm-gateway.partner.example", origin: "repo",
      evidence: [{ path: "services/assistant/config.yaml", line: 4, detail: "base_url" }],
      risks: [{ id: "custom-endpoint", severity: "low", summary: "Requests are sent to a host that is not the vendor's own, so prompts and data pass through a third party." }]
    },
    {
      id: "d1", kind: "agent_tool", name: "mcp server: filesystem", origin: "repo",
      evidence: [{ path: ".mcp.json", line: 3, detail: "npx -y @modelcontextprotocol/server-filesystem" }],
      risks: [{ id: "mcp-server-fetched-at-run-time", severity: "medium", summary: "The tool server is downloaded and run each time with no pinned version, so its code can change without review." }]
    },
    {
      id: "d2", kind: "agent_tool", name: "CLAUDE.md", origin: "repo",
      evidence: [{ path: "CLAUDE.md" }], properties: { role: "agent instructions" }
    },
    {
      id: "e1", kind: "library", name: "anthropic", version: "0.39.0", origin: "repo", ecosystem: "pypi", supplier: "Anthropic",
      evidence: [{ path: "services/assistant/requirements.txt", line: 2 }]
    },
    {
      id: "e2", kind: "library", name: "langchain", version: ">=0.2", origin: "repo", ecosystem: "pypi", supplier: "LangChain",
      evidence: [{ path: "services/assistant/requirements.txt", line: 5 }],
      risks: [{ id: "unpinned-library", severity: "low", summary: "An open version range: the next install can bring in a different release." }]
    },
    {
      id: "e3", kind: "library", name: "transformers", version: "4.44.2", origin: "repo", ecosystem: "pypi", supplier: "Hugging Face",
      evidence: [{ path: "services/search/embed.py", line: 22, detail: "trust_remote_code=True" }, { path: "services/search/requirements.txt", line: 1 }],
      risks: [{ id: "remote-code-model", severity: "high", summary: "trust_remote_code=True runs Python shipped with the model repository on your machine when the model loads." }]
    },
    {
      id: "e4", kind: "library", name: "openai", version: "4.67.1", origin: "repo", ecosystem: "npm", supplier: "OpenAI",
      evidence: [{ path: "services/search/package.json", line: 18 }]
    },
    {
      id: "f1", kind: "data_store", name: "qdrant/qdrant", version: "v1.11.3", origin: "cluster", ecosystem: "oci",
      evidence: [{ path: "ml/StatefulSet/qdrant", detail: "container image" }]
    },
    {
      id: "g1", kind: "dataset", name: "acme/support-tickets-2024", origin: "repo", ecosystem: "huggingface",
      evidence: [{ path: "training/finetune.py", line: 40, detail: "load_dataset" }]
    },
    {
      id: "h1", kind: "platform", name: "jupyter/scipy-notebook", version: "2024-10-07", origin: "cluster", ecosystem: "oci",
      evidence: [{ path: "labs/Deployment/notebook", detail: "container image" }],
      risks: [{ id: "notebook-exposed", severity: "high", summary: "The notebook server is started with an empty token and is reachable through an Ingress: anyone who finds the address can run code in the cluster." }],
      properties: { exposed: "Ingress", host: "nb.labs.example" }
    },
    {
      id: "i1", kind: "accelerator", name: "nvidia.com/gpu", origin: "cluster",
      evidence: [{ path: "ml/Deployment/vllm-chat", detail: "2 requested" }, { path: "labs/StatefulSet/ollama", detail: "1 requested" }],
      properties: { total: "3" }
    },
    {
      id: "j1", kind: "credential", name: "OPENAI_API_KEY", origin: "cluster",
      evidence: [{ path: "web/Deployment/search-api", detail: "env, literal value" }],
      risks: [{ id: "literal-ai-credential", severity: "high", summary: "The key is written into the workload as plain text instead of being read from a secret, so anyone who can view the deployment can read it. The value is not recorded here." }]
    },
    {
      id: "j2", kind: "credential", name: "ANTHROPIC_API_KEY", origin: "repo",
      evidence: [{ path: "services/assistant/llm.py", line: 8, detail: "read from the environment" }]
    },
    {
      id: "j3", kind: "credential", name: "HF_TOKEN", origin: "cluster",
      evidence: [{ path: "ml/Deployment/vllm-chat", detail: "env, from secret hf-credentials" }]
    }
  ],
  notes: [
    "Secrets were not requested from the cluster, so credentials held only in secrets are listed by the name that references them.",
    "The cluster has no KServe or Ray resource types installed; none were examined."
  ]
};
