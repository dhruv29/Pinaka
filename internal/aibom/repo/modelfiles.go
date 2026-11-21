package repo

import (
	"bufio"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	// lfsPointerPrefix starts every Git LFS pointer file. A pointer is a
	// small text file standing in for a large file kept outside the repo.
	lfsPointerPrefix = "version https://git-lfs.github.com/spec/v1"
	// headerBytes is how much of a model file is read: enough to tell a
	// Git LFS pointer from real weights, and no more.
	headerBytes = 512
)

// modelFormat describes one model file extension.
type modelFormat struct {
	format string
	// riskKey names the pickle risk for formats that can run code on load.
	riskKey string
	// needsCompanion means the extension is too generic on its own (.bin,
	// .pb) and only counts next to a file that marks a model directory.
	needsCompanion bool
}

var modelExtensions = map[string]modelFormat{
	".gguf": {format: "gguf"}, ".ggml": {format: "ggml"}, ".safetensors": {format: "safetensors"},
	".onnx": {format: "onnx"}, ".h5": {format: "keras-h5"}, ".keras": {format: "keras"},
	".tflite": {format: "tflite"}, ".mlmodel": {format: "coreml"},
	".pb":   {format: "tensorflow-savedmodel", needsCompanion: true},
	".pt":   {format: "pytorch", riskKey: riskPickleTorch},
	".pth":  {format: "pytorch", riskKey: riskPickleTorch},
	".ckpt": {format: "checkpoint", riskKey: riskPickleTorch},
	".bin":  {format: "pytorch-bin", riskKey: riskPickleTorch, needsCompanion: true},
	".pkl":  {format: "pickle", riskKey: riskPickleRaw}, ".pickle": {format: "pickle", riskKey: riskPickleRaw},
	".joblib": {format: "joblib", riskKey: riskPickleRaw},
}

// modelCompanions mark a directory as holding a model, which is what makes
// a generic .bin or .pb file worth listing.
var modelCompanions = map[string][]string{
	".bin": {"config.json", "tokenizer.json", "tokenizer_config.json", "generation_config.json"},
	".pb":  {"variables", "keras_metadata.pb", "fingerprint.pb"},
}

// modelFile lists rel if it is a model weights file and reports whether it
// was one. Only the first few bytes are read, to recognise a Git LFS
// pointer.
func (s *scanner) modelFile(rel string) bool {
	ext := strings.ToLower(path.Ext(rel))
	format, ok := modelExtensions[ext]
	if !ok || (format.needsCompanion && !s.hasCompanion(rel, ext)) {
		return false
	}
	info, err := s.root.Lstat(rel)
	if err != nil {
		s.gap("could not be read", rel)
		return true
	}
	props := map[string]string{"format": format.format, "size": strconv.FormatInt(info.Size(), 10), propContext: contextOf(rel)}
	detail := "model file"
	if size, isPointer := s.lfsPointer(rel); isPointer {
		props["storage"], props["size"], detail = "git-lfs-pointer", size, "Git LFS pointer to a model file"
	}
	found := finding{
		component: aibom.Component{Kind: aibom.KindModel, Name: rel, Properties: props},
		evidence:  aibom.Evidence{Path: rel, Detail: detail},
	}
	if format.riskKey != "" {
		found.risks = []aibom.Risk{risk(format.riskKey)}
	}
	s.add(found)
	return true
}

func (s *scanner) hasCompanion(rel, ext string) bool {
	if ext == ".pb" && path.Base(rel) == "saved_model.pb" {
		return true
	}
	for _, name := range modelCompanions[ext] {
		if _, err := s.root.Lstat(path.Join(path.Dir(rel), name)); err == nil {
			return true
		}
	}
	return false
}

// lfsPointer reports whether rel is a Git LFS pointer and, if so, the size
// of the file it stands for.
func (s *scanner) lfsPointer(rel string) (string, bool) {
	f, err := s.root.Open(rel)
	if err != nil {
		return "", false
	}
	defer f.Close()
	head, _ := io.ReadAll(io.LimitReader(f, headerBytes))
	if !strings.HasPrefix(string(head), lfsPointerPrefix) {
		return "", false
	}
	size := ""
	lines := bufio.NewScanner(strings.NewReader(string(head)))
	for lines.Scan() {
		if rest, ok := strings.CutPrefix(lines.Text(), "size "); ok {
			size = strings.TrimSpace(rest)
		}
	}
	return size, true
}
