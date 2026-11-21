package kube

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// maxListDepth bounds Lists nested inside Lists. kubectl never nests
	// them more than once; deeper nesting is hostile or broken input.
	maxListDepth = 2

	kindSecret    = "Secret"
	kindConfigMap = "ConfigMap"
)

// ownerRef is the part of an ownerReference needed to find the top-level
// owner of an object.
type ownerRef struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller bool   `json:"controller"`
}

type objectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Labels          map[string]string `json:"labels"`
	OwnerReferences []ownerRef        `json:"ownerReferences"`
}

// object is any Kubernetes object. Spec, status and data stay raw until the
// kind is known, so a field of an unexpected type only spoils its own object.
type object struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   objectMeta        `json:"metadata"`
	Spec       json.RawMessage   `json:"spec"`
	Status     json.RawMessage   `json:"status"`
	Data       json.RawMessage   `json:"data"`
	Items      []json.RawMessage `json:"items"`
}

// decoder collects objects from one or more inputs, enforcing the object cap
// and recording what it had to skip.
type decoder struct {
	maxObjects int
	objects    []object
	notes      []string
	full       bool
	secrets    int
}

func newDecoder(maxObjects int) *decoder {
	return &decoder{maxObjects: maxObjects}
}

// readCapped reads at most max bytes and fails if there is more, because a
// truncated JSON document cannot be parsed honestly.
func readCapped(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("%sread input: %w", errPrefix, err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%sinput is larger than %d bytes", errPrefix, max)
	}
	return data, nil
}

// add decodes every JSON document in data.
func (d *decoder) add(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	for doc := 1; ; doc++ {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%sdocument %d is not valid JSON: %w", errPrefix, doc, err)
		}
		if err := d.addDocument(raw, 0); err != nil {
			return fmt.Errorf("%sdocument %d is not a Kubernetes object or List: %w", errPrefix, doc, err)
		}
	}
}

func (d *decoder) addDocument(raw json.RawMessage, depth int) error {
	var o object
	if err := json.Unmarshal(raw, &o); err != nil {
		return err
	}
	if !isList(o) {
		d.addObject(o)
		return nil
	}
	if depth >= maxListDepth {
		d.notes = append(d.notes, "skipped a List nested too deeply inside another List")
		return nil
	}
	for i, item := range o.Items {
		if err := d.addDocument(item, depth+1); err != nil {
			d.notes = append(d.notes, fmt.Sprintf("skipped list item %d: it is not a Kubernetes object of the expected shape", i))
		}
	}
	return nil
}

func isList(o object) bool {
	return o.Items != nil || strings.HasSuffix(o.Kind, "List")
}

// addObject keeps an object unless the cap is reached or it is a Secret.
// Secrets are dropped unread: the BOM records credentials by name only.
func (d *decoder) addObject(o object) {
	if d.full {
		return
	}
	if len(d.objects) >= d.maxObjects {
		d.full = true
		d.notes = append(d.notes, fmt.Sprintf("stopped after %d objects; the rest of the input was not examined", d.maxObjects))
		return
	}
	if o.Kind == kindSecret {
		d.secrets++
		return
	}
	if o.Kind != kindConfigMap {
		o.Data = nil
	}
	o.Items = nil
	d.objects = append(d.objects, o)
}

// finish returns the objects and notes gathered so far.
func (d *decoder) finish() ([]object, []string) {
	notes := append([]string{}, d.notes...)
	if d.secrets > 0 {
		notes = append(notes, fmt.Sprintf("ignored %d Secret objects: secret contents are never read", d.secrets))
	}
	return d.objects, notes
}

// decodeInto decodes raw into v, treating a missing or null value as empty.
func decodeInto(raw json.RawMessage, v any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return json.Unmarshal(trimmed, v)
}
