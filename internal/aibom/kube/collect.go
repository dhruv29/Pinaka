package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	kubectlBinary  = "kubectl"
	verbGet        = "get"
	requestTimeout = "--request-timeout=60s"
	maxStderrBytes = 8 << 10
)

// CommandRunner runs kubectl with args and returns its standard output. An
// error should include kubectl's standard error, which is how "forbidden"
// and "resource type not found" are told apart from real failures. Tests
// inject a fake; KubectlRunner is the real one.
type CommandRunner func(ctx context.Context, args []string) ([]byte, error)

// collectResource is one resource type Collect lists. Optional ones are
// custom resources that many clusters do not have. Secrets are deliberately
// absent: credentials are recorded by name from pod specs only.
type collectResource struct {
	name     string
	optional bool
}

var collectResources = []collectResource{
	{"pods", false}, {"deployments", false}, {"statefulsets", false}, {"daemonsets", false},
	{"jobs", false}, {"cronjobs", false}, {"services", false}, {"ingresses", false}, {"configmaps", false},
	{"inferenceservices.serving.kserve.io", true}, {"rayclusters.ray.io", true}, {"rayservices.ray.io", true},
	{"notebooks.kubeflow.org", true}, {"httproutes.gateway.networking.k8s.io", true},
}

var (
	// contextPattern admits real context names (EKS ARNs, GKE names with
	// underscores, kind-*) but nothing that could read as a flag.
	contextPattern   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:@/-]{0,252}$`)
	namespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
)

// Collect lists AI-relevant resources with read-only `kubectl get` commands
// through run (KubectlRunner when nil) and returns the resulting BOM.
// Resources the caller may not list, or custom resources the cluster does
// not have, become Notes: a gap must not look like an absence.
func Collect(ctx context.Context, run CommandRunner, opts Options) (aibom.BOM, error) {
	if ctx == nil {
		return aibom.BOM{}, errors.New(errPrefix + "no context")
	}
	if err := validateOptions(opts); err != nil {
		return aibom.BOM{}, err
	}
	if run == nil {
		run = KubectlRunner(opts.maxBytes())
	}
	d := newDecoder(opts.maxObjects())
	for _, res := range collectResources {
		if err := ctx.Err(); err != nil {
			return aibom.BOM{}, fmt.Errorf("%scollection cancelled: %w", errPrefix, err)
		}
		if err := collectOne(ctx, run, res, opts, d); err != nil {
			return aibom.BOM{}, err
		}
	}
	objects, notes := d.finish()
	return analyze(objects, notes, collectSubject(opts)), nil
}

func collectOne(ctx context.Context, run CommandRunner, res collectResource, opts Options, d *decoder) error {
	out, err := run(ctx, kubectlArgs(res.name, opts))
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%scollection cancelled: %w", errPrefix, ctx.Err())
		}
		note, ok := failureNote(res, err)
		if !ok {
			return fmt.Errorf("%skubectl get %s: %w", errPrefix, res.name, err)
		}
		d.notes = append(d.notes, note)
		return nil
	}
	if int64(len(out)) > opts.maxBytes() {
		return fmt.Errorf("%skubectl get %s printed more than %d bytes", errPrefix, res.name, opts.maxBytes())
	}
	if err := d.add(out); err != nil {
		return fmt.Errorf("%sparse kubectl get %s: %w", errPrefix, res.name, err)
	}
	return nil
}

// failureNote turns an RBAC denial or a missing resource type into a Note.
// Anything else is a real failure.
func failureNote(res collectResource, err error) (string, bool) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "forbidden"):
		return "could not list " + res.name + ": access is forbidden (RBAC); anything of that type is missing from this BOM, which is a gap, not an absence", true
	case strings.Contains(msg, "doesn't have a resource type"), strings.Contains(msg, "could not find the requested resource"),
		strings.Contains(msg, "(notfound)"), strings.Contains(msg, "no matches for kind"):
		if res.optional {
			return res.name + " is not installed in this cluster, so nothing of that type was looked for", true
		}
		return "could not list " + res.name + ": the resource type was not found; anything of that type is missing from this BOM", true
	}
	return "", false
}

// kubectlArgs builds the argument vector. The verb is always "get"; the
// resource comes from the fixed list; user input (namespace, context) has
// been validated and is passed as a single --flag=value element.
func kubectlArgs(resource string, opts Options) []string {
	args := []string{verbGet, resource, "-o", "json", requestTimeout}
	if opts.Namespace == "" {
		args = append(args, "--all-namespaces")
	} else {
		args = append(args, "--namespace="+opts.Namespace)
	}
	if opts.Context != "" {
		args = append(args, "--context="+opts.Context)
	}
	return args
}

func validateOptions(opts Options) error {
	if opts.Context != "" && !contextPattern.MatchString(opts.Context) {
		return errors.New(errPrefix + "kubectl context name contains characters that are not allowed")
	}
	if opts.Namespace != "" && !namespacePattern.MatchString(opts.Namespace) {
		return errors.New(errPrefix + "namespace is not a valid Kubernetes namespace name")
	}
	return nil
}

func collectSubject(opts Options) string {
	if opts.Subject != "" {
		return opts.Subject
	}
	subject := "current kubectl context"
	if opts.Context != "" {
		subject = "kubectl context " + opts.Context
	}
	if opts.Namespace != "" {
		subject += ", namespace " + opts.Namespace
	}
	return subject
}

// KubectlRunner returns a CommandRunner that runs the kubectl binary found
// on PATH, without a shell and with standard output capped at maxOutput
// bytes. It refuses any verb other than "get".
//
// kubectl receives only the part of the environment it needs to sign in:
// its own configuration, the home directory, and the cloud providers'
// variables, because on managed clusters signing in runs a helper that reads
// a profile, a region or a credentials file. Everything else, such as an API
// key for an unrelated service, is kept from kubectl and its plugins.
func KubectlRunner(maxOutput int64) CommandRunner {
	if maxOutput <= 0 {
		maxOutput = DefaultMaxBytes
	}
	return func(ctx context.Context, args []string) ([]byte, error) {
		if len(args) == 0 || args[0] != verbGet {
			return nil, errors.New(errPrefix + "only kubectl get may be run")
		}
		cmd := exec.CommandContext(ctx, kubectlBinary, args...)
		cmd.Env = signInEnv(os.Environ())
		stdout, stderr := &cappedBuffer{max: maxOutput}, &cappedBuffer{max: maxStderrBytes}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		err := cmd.Run()
		if stdout.over {
			return nil, fmt.Errorf("%skubectl output is larger than %d bytes", errPrefix, maxOutput)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.buf.String()))
		}
		return stdout.buf.Bytes(), nil
	}
}

// cappedBuffer keeps at most max bytes and drains the rest, so kubectl is
// never blocked on a full pipe.
type cappedBuffer struct {
	buf  bytes.Buffer
	max  int64
	over bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.max - int64(c.buf.Len())
	if int64(len(p)) > room {
		c.over = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

// signInNames are passed to kubectl as they are.
var signInNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "TMPDIR": true, "KUBECONFIG": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"http_proxy": true, "https_proxy": true, "no_proxy": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "USE_GKE_GCLOUD_AUTH_PLUGIN": true,
}

// signInPrefixes are the families of variables the cloud sign-in helpers
// read: AWS for EKS, Google for GKE, Azure for AKS.
var signInPrefixes = []string{"AWS_", "GOOGLE_", "CLOUDSDK_", "GCLOUD_", "AZURE_", "XDG_"}

// signInEnv returns the entries of environ that kubectl may see.
func signInEnv(environ []string) []string {
	kept := []string{}
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if signInNames[name] || hasAnyPrefix(name, signInPrefixes) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
