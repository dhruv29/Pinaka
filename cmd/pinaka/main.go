// Command pinaka writes an AI bill of materials: what AI a code repository
// or a running Kubernetes cluster contains, where each item was seen, and
// which of them deserve a look.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/dhruv29/Pinaka/internal/aibom"
	"github.com/dhruv29/Pinaka/internal/aibom/kube"
	"github.com/dhruv29/Pinaka/internal/aibom/repo"
)

const (
	exitOK = 0
	// exitError means the scan could not be completed.
	exitError = 1
	// exitUsage means the command line was wrong.
	exitUsage = 2
	// exitFlagged means the scan worked and found a risk at or above the
	// -fail-on severity, so a pipeline can stop on it.
	exitFlagged = 3

	formatMarkdown  = "markdown"
	formatJSON      = "json"
	formatCycloneDX = "cyclonedx"

	// stdinName is the file name that means standard input.
	stdinName = "-"
	// outputFileMode keeps a written report private to its owner: it lists
	// where credentials are used and what is exposed.
	outputFileMode = 0o600
)

const usage = `pinaka writes an AI bill of materials for a repository or a Kubernetes cluster.

Usage:
  pinaka scan -repo DIR [-exclude PATH]... [-format markdown|json|cyclonedx] [-out FILE] [-fail-on SEVERITY]
  pinaka scan -kube [-context NAME] [-namespace NAME] ...
  pinaka scan -kube-file FILE ...        (output of "kubectl get ... -o json"; "-" reads standard input)
  pinaka version

-repo can be combined with -kube or -kube-file: the result is one bill covering
what the code declares and what is actually running.

Formats:
  markdown   a report for people, risks first (the default)
  json       Pinaka's own format, for other programs to read
  cyclonedx  a CycloneDX 1.6 document, for inventory and compliance tools

Reading a cluster is strictly read-only: the only kubectl verb used is "get",
and secrets are never requested. Credentials are reported by name, never value.

Exit status: 0 done, 1 the scan failed, 2 bad usage,
3 a risk at or above -fail-on was found (info, low, medium, high or critical).
`

// version is the release name. It is a constant rather than build metadata
// so that two builds of the same source are identical.
const version = "0.1.0-dev"

// collectCluster reads a live cluster. Tests replace it.
var collectCluster = func(ctx context.Context, opts kube.Options) (aibom.BOM, error) {
	return kube.Collect(ctx, kube.KubectlRunner(0), opts)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "scan":
		code, err := runScan(ctx, args[1:], stdin, stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "pinaka: %v\n", err)
		}
		return code
	case "version":
		fmt.Fprintln(stdout, version)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "pinaka: unknown command %q\n\n%s", args[0], usage)
	return exitUsage
}

type scanFlags struct {
	repoDir, kubeFile, kubeContext, namespace string
	live                                      bool
	exclude                                   []string
	format, outPath, failOn                   string
}

func parseScanFlags(args []string, stderr io.Writer) (scanFlags, error) {
	var sf scanFlags
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&sf.repoDir, "repo", "", "directory of source code to scan")
	flags.Func("exclude", `path to leave out of the repository scan, relative to -repo; may be repeated and may use "*" (for example -exclude testdata -exclude "docs/*.js")`, func(value string) error {
		sf.exclude = append(sf.exclude, value)
		return nil
	})
	flags.BoolVar(&sf.live, "kube", false, "read the running cluster with kubectl (read-only)")
	flags.StringVar(&sf.kubeFile, "kube-file", "", `saved "kubectl get ... -o json" output to scan instead of a live cluster; "-" reads standard input`)
	flags.StringVar(&sf.kubeContext, "context", "", "kubectl context to read (with -kube); the current one when empty")
	flags.StringVar(&sf.namespace, "namespace", "", "read only this namespace (with -kube); all namespaces when empty")
	flags.StringVar(&sf.format, "format", formatMarkdown, "output format: markdown, json or cyclonedx")
	flags.StringVar(&sf.outPath, "out", "", "write the result here instead of standard output")
	flags.StringVar(&sf.failOn, "fail-on", "", "exit with status 3 when a risk of this severity or worse is found: info, low, medium, high or critical")
	if err := flags.Parse(args); err != nil {
		return sf, err
	}
	if flags.NArg() > 0 {
		return sf, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	return sf, sf.check()
}

func (sf scanFlags) check() error {
	switch {
	case sf.repoDir == "" && !sf.live && sf.kubeFile == "":
		return errors.New("nothing to scan: give -repo, -kube or -kube-file")
	case sf.live && sf.kubeFile != "":
		return errors.New("-kube and -kube-file cannot be used together")
	case !sf.live && (sf.kubeContext != "" || sf.namespace != ""):
		return errors.New("-context and -namespace need -kube")
	case sf.format != formatMarkdown && sf.format != formatJSON && sf.format != formatCycloneDX:
		return fmt.Errorf("unknown format %q: use markdown, json or cyclonedx", sf.format)
	case sf.failOn != "" && !aibom.AtLeast(aibom.Severity(sf.failOn), aibom.SeverityInfo):
		return fmt.Errorf("unknown severity %q for -fail-on: use info, low, medium, high or critical", sf.failOn)
	}
	return nil
}

func runScan(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	sf, err := parseScanFlags(args, stderr)
	if err != nil {
		return exitUsage, err
	}
	bom, err := scanAll(ctx, sf, stdin)
	if err != nil {
		return exitError, err
	}
	rendered, err := render(bom, sf.format)
	if err != nil {
		return exitError, err
	}
	if err := write(rendered, sf.outPath, stdout); err != nil {
		return exitError, err
	}
	fmt.Fprintf(stderr, "pinaka: %d component(s), %d risk(s) flagged\n", len(bom.Components), len(aibom.Flagged(bom)))
	if sf.failOn != "" && aibom.AtLeast(aibom.WorstSeverity(bom), aibom.Severity(sf.failOn)) {
		return exitFlagged, fmt.Errorf("a risk of severity %s or worse was found", sf.failOn)
	}
	return exitOK, nil
}

// scanAll runs every requested scan and merges the results into one bill.
func scanAll(ctx context.Context, sf scanFlags, stdin io.Reader) (aibom.BOM, error) {
	var boms []aibom.BOM
	subject := ""
	if sf.repoDir != "" {
		root, err := filepath.Abs(sf.repoDir)
		if err != nil {
			return aibom.BOM{}, err
		}
		bom, err := repo.Scan(ctx, root, repo.Options{Exclude: sf.exclude})
		if err != nil {
			return aibom.BOM{}, err
		}
		boms, subject = append(boms, bom), filepath.Base(root)
	}
	if sf.live || sf.kubeFile != "" {
		bom, err := scanCluster(ctx, sf, stdin)
		if err != nil {
			return aibom.BOM{}, err
		}
		boms = append(boms, bom)
		if subject == "" {
			subject = bom.Subject
		} else {
			subject += " + " + bom.Subject
		}
	}
	return aibom.Merge(subject, boms...), nil
}

func scanCluster(ctx context.Context, sf scanFlags, stdin io.Reader) (aibom.BOM, error) {
	if sf.live {
		return collectCluster(ctx, kube.Options{Context: sf.kubeContext, Namespace: sf.namespace})
	}
	if sf.kubeFile == stdinName {
		return kube.Parse(stdin, kube.Options{})
	}
	file, err := os.Open(sf.kubeFile)
	if err != nil {
		return aibom.BOM{}, err
	}
	defer file.Close()
	return kube.Parse(file, kube.Options{Subject: filepath.Base(sf.kubeFile)})
}

func render(bom aibom.BOM, format string) ([]byte, error) {
	switch format {
	case formatJSON:
		raw, err := json.MarshalIndent(bom, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	case formatCycloneDX:
		raw, err := aibom.CycloneDX(bom)
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	}
	return []byte(aibom.Markdown(bom)), nil
}

func write(content []byte, outPath string, stdout io.Writer) error {
	if outPath == "" {
		_, err := stdout.Write(content)
		return err
	}
	return os.WriteFile(outPath, content, outputFileMode)
}
