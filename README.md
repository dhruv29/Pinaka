# Pinaka

Pinaka writes an **AI bill of materials**: what AI a code repository or a
running Kubernetes cluster contains, where each item was seen, and which ones
deserve a look first.

Security, platform and compliance teams are being asked "what AI are we
running?". Pinaka answers that from the two places the truth lives: what the
code declares and what is actually deployed.

A sample bill, and a viewer for your own, is at
<https://dhruv29.github.io/Pinaka/>.

## What it finds

| Kind | Examples |
|---|---|
| Models | weight files (`.gguf`, `.safetensors`, `.onnx`, `.pt`, `.pkl`…), hosted model names in code, Hugging Face ids in loader calls, models named in a serving command |
| AI libraries | from dependency manifests for Python, JavaScript, Go, Rust and Java, with versions |
| Hosted providers | API hosts the code or a workload calls; the vendor behind an SDK dependency or a key name, marked as inferred, unless the client is pointed elsewhere; and clients pointed at a non-vendor or in-cluster address (host only) |
| Model serving | vLLM, Ollama, Triton, text-generation-inference, KServe and others, from container images |
| Data stores and platforms | vector databases, notebooks, experiment tracking |
| Agent tooling | MCP server configurations, agent instruction files |
| Accelerators | GPU requests per workload |
| Credentials | **by name only**: which AI keys are referenced, and where. Values are never recorded, nor are passwords or signed queries in model locations |

Each item carries its evidence: a file and line, or `namespace/Kind/name` and
the field. Each risk also says where it was raised.

## What it flags

Observations with their reasons, not proven vulnerabilities:

- a model file in a format that can run code when loaded (pickle and friends)
- `trust_remote_code=True`, and loads that deserialise unsafely
- an AI key written into code (also as an `api_key=` or `apiKey:` argument)
  or into a workload as plain text. Placeholders that self-hosted servers
  expect, such as `EMPTY` or `sk-no-key-required`, are not flagged
- an inference server, notebook or vector store reachable from outside the
  cluster, especially one that ships without authentication
- models, libraries and images that are not pinned
- an agent tool server that is downloaded afresh each time it runs
- an AI client pointed at a third party's address

## Use

```bash
make build                     # one binary, no dependencies

./pinaka scan -repo .                              # a repository
./pinaka scan -kube                                # the cluster kubectl points at
./pinaka scan -kube -context prod -namespace ml    # one context, one namespace
./pinaka scan -repo . -kube                        # both, as one bill

kubectl get pods,deployments,statefulsets,daemonsets,jobs,cronjobs,services,ingresses,configmaps -A -o json > cluster.json
./pinaka scan -kube-file cluster.json              # a cluster you were only given a file from
```

In one bill, an item found in both the code and the cluster is listed once,
with `repo+cluster` as where it was found.

A saved file can only show what was exported. Kinds it does not contain are
listed under "Not examined" with what they hide (no Services, say, means
exposure could not be judged). If the cluster runs KServe, Ray, Kubeflow or
the Gateway API, add `inferenceservices.serving.kserve.io`,
`rayclusters.ray.io`, `rayservices.ray.io`, `notebooks.kubeflow.org` and
`httproutes.gateway.networking.k8s.io` to the `kubectl get` list.

Output:

| Flag | Result |
|---|---|
| `-format markdown` | a report for people, risks first (the default) |
| `-format json` | Pinaka's own format, for other programs; the demo page opens it |
| `-format cyclonedx` | a CycloneDX 1.6 document for inventory and compliance tools |
| `-out FILE` | write to a file (owner-readable only) instead of standard output |
| `-fail-on SEVERITY` | exit with status 3 when a risk of that severity or worse is found, to stop a pipeline |
| `-exclude PATH` | leave a path out of the repository scan; repeatable, `*` allowed. What was left out is listed in the bill |

Exit status: 0 done, 1 the scan failed, 2 bad usage, 3 `-fail-on` was tripped.

## How it runs

- **Nothing is installed in your containers or your cluster.** Pinaka is one
  program you run from a laptop, a pipeline or a scheduled job.
- **It only reads.** A repository scan opens files and makes no network calls.
  A cluster scan runs `kubectl get` and nothing else, with your own `kubectl`
  login, and never requests secrets. A read-only account is enough.
- **Gaps are stated.** Skipped directories, symbolic links and special files,
  resource types the cluster does not have or a saved file does not contain,
  things your login may not read and limits that were reached (including
  long command lines and ConfigMap values read only in part) are listed under
  "Not examined", so a gap is never mistaken for an absence.
- **Untrusted input.** File contents and cluster objects are treated as
  hostile: sizes are capped, symbolic links are not followed out of the
  repository, and names from a scan cannot inject markup into a report.

## Limits

- Pinaka reports what is declared in code and what is deployed. It does not
  look inside running containers or watch network traffic, so AI use with no
  visible configuration does not appear.
- The cluster scan has been tested against recorded `kubectl` output, not yet
  against a live cluster.
- Model names are recognised by pattern, so a model named in a comment or a
  test is listed too; each item says whether it was seen in code, a test or a
  document.
- Risk severities are judgements, not measurements.
- Not built: `package-lock.json`, `yarn.lock` and `Cargo.lock` parsing; MCP
  configurations written in YAML; Airflow and Argo workloads; pushing the
  inventory into another system. The JSON output is the hand-off for that.

## Development

```bash
make check     # format, vet, tests with the race detector
make build     # reproducible binary
```

Go 1.27, standard library only.

## Licence

GNU Affero General Public License, version 3. See [LICENSE](LICENSE).
