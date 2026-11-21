// Package kube builds an AI bill of materials for a running Kubernetes
// cluster: which model servers, models, ML platforms, vector stores, GPU
// workloads, agent tools and AI-provider credentials are actually deployed,
// and which of them can be reached from outside.
//
// It works only from what `kubectl get -o json` prints. Parse reads that
// output from any reader; Collect runs the read-only `kubectl get` commands
// itself. Nothing here ever writes to a cluster, and Secrets are never read:
// credentials are recorded by name only.
package kube

import (
	"errors"
	"io"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	// DefaultMaxBytes caps how much JSON one input (or one kubectl command)
	// may produce. Large clusters print a lot, but an unbounded read is an
	// easy way to exhaust memory.
	DefaultMaxBytes int64 = 128 << 20
	// DefaultMaxObjects caps how many Kubernetes objects are examined.
	DefaultMaxObjects = 100000

	errPrefix           = "aibom/kube: "
	defaultParseSubject = "kubernetes objects"
)

// Options tune Parse and Collect. The zero value is usable.
type Options struct {
	// Namespace limits Collect to one namespace. Empty means all namespaces.
	Namespace string
	// Context selects a kubectl context for Collect. Empty means the
	// current context.
	Context string
	// Subject names what was scanned in the BOM. A default is derived when
	// it is empty.
	Subject string
	// MaxBytes caps the input size (per kubectl command for Collect).
	MaxBytes int64
	// MaxObjects caps the number of objects examined. Objects beyond the cap
	// are skipped and a Note says so.
	MaxObjects int
}

func (o Options) maxBytes() int64 {
	if o.MaxBytes > 0 {
		return o.MaxBytes
	}
	return DefaultMaxBytes
}

func (o Options) maxObjects() int {
	if o.MaxObjects > 0 {
		return o.MaxObjects
	}
	return DefaultMaxObjects
}

// Parse reads Kubernetes JSON — a List as printed by `kubectl get -o json`,
// a single object, or several JSON documents one after another — and
// returns the AI bill of materials it describes.
//
// Input that is too large or is not JSON is an error. Individual objects
// that do not have the expected shape are skipped with a Note, so one odd
// object does not hide the rest of the cluster.
func Parse(r io.Reader, opts Options) (aibom.BOM, error) {
	if r == nil {
		return aibom.BOM{}, errors.New(errPrefix + "no input reader")
	}
	data, err := readCapped(r, opts.maxBytes())
	if err != nil {
		return aibom.BOM{}, err
	}
	d := newDecoder(opts.maxObjects())
	if err := d.add(data); err != nil {
		return aibom.BOM{}, err
	}
	subject := opts.Subject
	if subject == "" {
		subject = defaultParseSubject
	}
	objects, notes := d.finish()
	return analyze(objects, notes, subject), nil
}
