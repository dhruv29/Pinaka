package repo

// codeRules runs every detector that reads source, configuration or docs.
// Each detector looks for its own well-formed shapes, so running them all on
// every text file costs little and misses nothing a narrower dispatch would.
func (s *scanner) codeRules(f *textFile) {
	s.namedModels(f)
	s.ollamaModels(f)
	s.hubModels(f)
	s.deserialisation(f)
	s.providerHosts(f)
	s.baseURLs(f)
	s.credentialNames(f)
}
