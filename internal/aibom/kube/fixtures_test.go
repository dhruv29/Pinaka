package kube

// Fixtures shaped like real `kubectl get -o json` output, trimmed to the
// fields that matter.

const secretValue = "sk-live-DO-NOT-LEAK-4f9a2c"

const vllmFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"vllm","namespace":"ai","labels":{"app":"vllm"}},
 "spec":{"replicas":1,"selector":{"matchLabels":{"app":"vllm"}},"template":{"metadata":{"labels":{"app":"vllm"}},"spec":{
  "nodeSelector":{"nvidia.com/gpu.present":"true"},
  "containers":[{"name":"vllm","image":"vllm/vllm-openai:v0.6.3",
   "args":["--model","meta-llama/Llama-3.1-8B-Instruct","--served-model-name","llama3","--port","8000"],
   "env":[{"name":"HF_TOKEN","valueFrom":{"secretKeyRef":{"name":"hf","key":"token"}}}],
   "resources":{"limits":{"nvidia.com/gpu":"1","memory":"32Gi"},"requests":{"cpu":"4"}}}]}}}},
{"apiVersion":"v1","kind":"Pod","metadata":{"name":"vllm-7d9f8b6c5-abcde","namespace":"ai",
  "labels":{"app":"vllm","pod-template-hash":"7d9f8b6c5"},
  "ownerReferences":[{"apiVersion":"apps/v1","kind":"ReplicaSet","name":"vllm-7d9f8b6c5","controller":true}]},
 "spec":{"nodeSelector":{"nvidia.com/gpu.present":"true"},
  "containers":[{"name":"vllm","image":"vllm/vllm-openai:v0.6.3",
   "args":["--model","meta-llama/Llama-3.1-8B-Instruct","--served-model-name","llama3","--port","8000"],
   "env":[{"name":"HF_TOKEN","valueFrom":{"secretKeyRef":{"name":"hf","key":"token"}}}],
   "resources":{"limits":{"nvidia.com/gpu":"1"}}}]}},
{"apiVersion":"v1","kind":"Service","metadata":{"name":"vllm-lb","namespace":"ai"},
 "spec":{"type":"LoadBalancer","selector":{"app":"vllm"},"ports":[{"port":8000,"targetPort":8000,"protocol":"TCP"}]},
 "status":{"loadBalancer":{"ingress":[{"hostname":"a1b2.elb.amazonaws.com"}]}}}
]}`

const ollamaFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"ollama","namespace":"llm"},
 "spec":{"template":{"metadata":{"labels":{"app":"ollama"}},"spec":{"containers":[{"name":"ollama","image":"ollama/ollama",
  "command":["/bin/sh","-c","ollama serve & sleep 5 && ollama pull llama3.1:8b && wait"]}]}}}},
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"ollama-exporter","namespace":"llm"},
 "spec":{"template":{"metadata":{"labels":{"app":"exporter"}},"spec":{"containers":[{"name":"exporter","image":"ghcr.io/example/ollama-exporter:1.0"}]}}}},
{"apiVersion":"v1","kind":"Service","metadata":{"name":"ollama","namespace":"llm"},
 "spec":{"type":"ClusterIP","selector":{"app":"ollama"},"ports":[{"port":11434}]}},
{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"ollama","namespace":"llm"},
 "spec":{"rules":[{"host":"ollama.example.com","http":{"paths":[{"path":"/","pathType":"Prefix",
  "backend":{"service":{"name":"ollama","port":{"number":11434}}}}]}}]}}
]}`

const kserveFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"serving.kserve.io/v1beta1","kind":"InferenceService","metadata":{"name":"llama","namespace":"kserve-test"},
 "spec":{"predictor":{"minReplicas":1,"model":{"modelFormat":{"name":"huggingface"},"storageUri":"hf://meta-llama/Llama-3.2-1B-Instruct",
  "args":["--trust-remote-code"],"resources":{"limits":{"nvidia.com/gpu":"1"}}}}}},
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"llama-predictor","namespace":"kserve-test",
  "ownerReferences":[{"kind":"InferenceService","name":"llama","controller":true}]},
 "spec":{"template":{"metadata":{"labels":{"serving.kserve.io/inferenceservice":"llama"}},"spec":{
  "containers":[{"name":"kserve-container","image":"kserve/huggingfaceserver:v0.14.0@sha256:0123456789abcdef"}]}}}}
]}`

const notebookFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"v1","kind":"Pod","metadata":{"name":"jupyter-0","namespace":"ds","labels":{"app":"jupyter"}},
 "spec":{"securityContext":{"runAsUser":0},"containers":[{"name":"notebook","image":"quay.io/jupyter/scipy-notebook:2024-07-29",
  "args":["start-notebook.py","--ServerApp.token=","--ServerApp.ip=0.0.0.0"],
  "env":[{"name":"OPENAI_API_KEY","value":"` + secretValue + `"}]}]}},
{"apiVersion":"v1","kind":"Service","metadata":{"name":"jupyter","namespace":"ds"},
 "spec":{"type":"NodePort","selector":{"app":"jupyter"},"ports":[{"port":8888,"nodePort":30888}]}}
]}`

const qdrantFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"qdrant","namespace":"vectors"},
 "spec":{"template":{"metadata":{"labels":{"app":"qdrant"}},"spec":{"containers":[{"name":"qdrant",
  "image":"qdrant/qdrant:v1.11.0@sha256:aa11bb22cc33dd44"}]}}}},
{"apiVersion":"v1","kind":"Pod","metadata":{"name":"qdrant-0","namespace":"vectors","labels":{"app":"qdrant"},
  "ownerReferences":[{"kind":"StatefulSet","name":"qdrant","controller":true}]},
 "spec":{"containers":[{"name":"qdrant","image":"qdrant/qdrant:v1.11.0@sha256:aa11bb22cc33dd44"}]}},
{"apiVersion":"v1","kind":"Service","metadata":{"name":"qdrant-headless","namespace":"vectors"},
 "spec":{"clusterIP":"None","selector":{"app":"qdrant"},"ports":[{"port":6333}]}}
]}`

const plainFixture = `{"apiVersion":"v1","kind":"List","items":[
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"shop"},
 "spec":{"template":{"metadata":{"labels":{"app":"web"}},"spec":{"containers":[{"name":"nginx","image":"nginx:1.27",
  "args":["-g","daemon off;"],"env":[{"name":"API_URL","value":"https://api.shop.example.com"}]}]}}}},
{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"db","namespace":"shop"},
 "spec":{"template":{"metadata":{"labels":{"app":"db"}},"spec":{"containers":[{"name":"postgres","image":"postgres:16",
  "env":[{"name":"POSTGRES_PASSWORD","value":"hunter2"}]}]}}}},
{"apiVersion":"v1","kind":"Service","metadata":{"name":"web","namespace":"shop"},
 "spec":{"type":"LoadBalancer","selector":{"app":"web"},"ports":[{"port":80}]}},
{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"web-config","namespace":"shop"},
 "data":{"LOG_LEVEL":"info","app.env":"DATABASE_URL=postgres://db:5432/shop\nFEATURE_X=true\n"}}
]}`

const configMapFixture = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-config","namespace":"bots"},
 "data":{"OPENAI_API_KEY":"` + secretValue + `","app.env":"ANTHROPIC_API_KEY=` + secretValue + `\nMODEL_ID=mistralai/Mistral-7B-Instruct-v0.3\nOPENAI_BASE_URL=https://api.openai.com/v1\nHF_TOKEN=${HF_TOKEN}\n"}}`
