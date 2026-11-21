package repo

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// hubCall is a call that fetches a model or dataset from the Hugging Face
// Hub. The identifier is the first positional string or the named keyword.
type hubCall struct {
	call       *regexp.Regexp // matches up to and including "("
	keyword    string
	positional bool
	kind       aibom.Kind
	detail     string
}

var hubCalls = []hubCall{
	{regexp.MustCompile(`\.from_pretrained\(`), "pretrained_model_name_or_path", true, aibom.KindModel, "from_pretrained"},
	{regexp.MustCompile(`\bpipeline\(`), "model", false, aibom.KindModel, "pipeline(model=...)"},
	{regexp.MustCompile(`\b(?:hf_hub_download|snapshot_download)\(`), "repo_id", true, aibom.KindModel, "Hub download"},
	{regexp.MustCompile(`\b(?:SentenceTransformer|CrossEncoder)\(`), "model_name_or_path", true, aibom.KindModel, "sentence-transformers model"},
	{regexp.MustCompile(`\bload_dataset\(`), "path", true, aibom.KindDataset, "load_dataset"},
}

var (
	hubID          = regexp.MustCompile(`^[A-Za-z0-9][\w.-]*(?:/[A-Za-z0-9][\w.-]*)?$`)
	firstString    = regexp.MustCompile(`^\s*["']([^"'\s]+)["']`)
	revisionArg    = regexp.MustCompile(`\brevision\s*=\s*["']([^"']+)["']`)
	remoteCodeArg  = regexp.MustCompile(`\btrust_remote_code\s*=\s*True\b`)
	remoteCodeDict = regexp.MustCompile(`["']trust_remote_code["']\s*:\s*True\b`)
	dataFilesArg   = regexp.MustCompile(`\bdata_files\s*=\s*["']([^"']+)["']`)
)

// datasetBuilders are load_dataset's generic loaders. Their first argument
// is a file format, not a dataset; the data files name the dataset.
var datasetBuilders = setOf("json", "csv", "parquet", "text", "arrow", "imagefolder", "audiofolder", "webdataset", "pandas", "sql")

func keywordArg(args, keyword string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(keyword) + `\s*=\s*["']([^"'\s]+)["']`)
	if m := re.FindStringSubmatch(args); m != nil {
		return m[1]
	}
	return ""
}

// hubModels records models and datasets fetched from the Hub, with their
// revision and the remote-code risk.
func (s *scanner) hubModels(f *textFile) {
	for _, call := range hubCalls {
		for _, loc := range call.call.FindAllStringIndex(f.text, -1) {
			args := callArgs(f.text, loc[1])
			id := ""
			if m := firstString.FindStringSubmatch(args); call.positional && m != nil {
				id = m[1]
			} else if call.keyword != "" {
				id = keywordArg(args, call.keyword)
			}
			s.hubReference(f, call, loc[0], id, args)
		}
	}
	for _, loc := range remoteCodeDict.FindAllStringIndex(f.text, -1) {
		s.unresolvedModel(f, loc[0], "trust_remote_code in loader options", risk(riskRemoteCode))
	}
}

func (s *scanner) hubReference(f *textFile, call hubCall, offset int, id, args string) {
	var risks []aibom.Risk
	if call.kind == aibom.KindModel && remoteCodeArg.MatchString(args) {
		risks = append(risks, risk(riskRemoteCode))
	}
	if call.kind == aibom.KindDataset && datasetBuilders[id] {
		if m := dataFilesArg.FindStringSubmatch(args); m != nil {
			s.add(finding{
				component: aibom.Component{Kind: aibom.KindDataset, Name: m[1], Properties: map[string]string{propContext: f.context, "reference": "file", "loader": id}},
				evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: call.detail},
			})
		}
		return
	}
	if !hubID.MatchString(id) {
		if len(risks) > 0 {
			s.unresolvedModel(f, offset, call.detail+" with a name computed at run time", risks...)
		}
		return
	}
	comp := aibom.Component{Kind: call.kind, Name: id, Ecosystem: ecoHuggingFace, Properties: map[string]string{propContext: f.context}}
	if org, _, ok := strings.Cut(id, "/"); ok {
		comp.Supplier = org
	}
	if m := revisionArg.FindStringSubmatch(args); m != nil {
		comp.Version = m[1]
	}
	if _, err := s.root.Lstat(id); err == nil {
		// The "id" is a directory in this tree: a local model, not a download.
		comp.Ecosystem, comp.Properties["reference"] = "", "local"
	} else if call.kind == aibom.KindModel && !pinnedRevision(comp.Version) {
		risks = append(risks, risk(riskUnpinnedModel))
	}
	s.add(finding{component: comp, evidence: aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: call.detail}, risks: risks})
}

// pinnedRevision reports whether a revision was given that is not one of
// the default branch names, which follow new pushes. A tag can in principle
// be moved too, but naming one is a deliberate pin a reviewer can see.
func pinnedRevision(rev string) bool {
	return rev != "" && rev != "main" && rev != "master"
}

// unresolvedModel records a risky model load whose model name is only known
// at run time, so the risk is not lost for want of a name. There is one
// such component per file.
func (s *scanner) unresolvedModel(f *textFile, offset int, detail string, risks ...aibom.Risk) {
	s.add(finding{
		component: aibom.Component{
			Kind: aibom.KindModel, Name: fmt.Sprintf("unresolved model (%s)", f.path),
			Properties: map[string]string{propContext: f.context, "reference": "computed at run time"},
		},
		evidence: aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: detail},
		risks:    risks,
	})
}

var (
	torchLoad        = regexp.MustCompile(`\btorch\.load\(`)
	weightsOnly      = regexp.MustCompile(`\bweights_only\s*=\s*True\b`)
	pickleLoad       = regexp.MustCompile(`\b(?:pickle|cloudpickle|dill|joblib)\.load\(`)
	modelPathLiteral = regexp.MustCompile(`["']([^"'\s]+\.(?:pkl|pickle|joblib|pt|pth|ckpt|bin))["']`)
	modelFileRef     = regexp.MustCompile(`(?i)\.(?:pkl|pickle|joblib|pt|pth|ckpt|bin)\b|model|checkpoint`)
)

// deserialisation flags pickle-based loads of models: torch.load without
// weights_only=True, and pickle or joblib loads whose call or line is about
// a model. A pickle.load of unrelated data is not this inventory's concern.
func (s *scanner) deserialisation(f *textFile) {
	for _, loc := range torchLoad.FindAllStringIndex(f.text, -1) {
		args := callArgs(f.text, loc[1])
		if !weightsOnly.MatchString(args) {
			s.loadedModel(f, loc[0], args, "torch.load without weights_only=True")
		}
	}
	for _, loc := range pickleLoad.FindAllStringIndex(f.text, -1) {
		args := callArgs(f.text, loc[1])
		if modelFileRef.MatchString(args) || modelFileRef.MatchString(f.lineText(loc[0])) {
			s.loadedModel(f, loc[0], args, "pickle-based load of a model")
		}
	}
}

// loadedModel attaches the deserialisation risk to the file the call names,
// or to the file's unresolved model when the path is computed.
func (s *scanner) loadedModel(f *textFile, offset int, args, detail string) {
	r := risk(riskDeserialise)
	m := firstString.FindStringSubmatch(args)
	if m == nil {
		// pickle.load(open("clf.pkl", "rb")): the file is an inner argument.
		m = modelPathLiteral.FindStringSubmatch(args)
	}
	if m == nil {
		s.unresolvedModel(f, offset, detail, r)
		return
	}
	s.add(finding{
		component: aibom.Component{Kind: aibom.KindModel, Name: m[1], Properties: map[string]string{propContext: f.context, "reference": "file"}},
		evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: detail},
		risks:     []aibom.Risk{r},
	})
}

// lineText returns the whole line holding offset.
func (f *textFile) lineText(offset int) string {
	line := sort.Search(len(f.starts), func(i int) bool { return f.starts[i] > offset })
	start := f.starts[line-1]
	end := len(f.text)
	if line < len(f.starts) {
		end = f.starts[line]
	}
	return f.text[start:end]
}
