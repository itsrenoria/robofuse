package matcher

import "regexp"

// Config holds all matching configuration, typically loaded from a config section.
type Config struct {
	StripPatterns          []*regexp.Regexp
	NoiseTokens            []string
	TitleAliases           map[string][]string
	TransliterationAliases map[string][]string
	AnimeKeywords          []string
	CollectionKeywords     []string
	ForceMoviePatterns     []*regexp.Regexp
	MinScore               int
	MinScoreNoYear         int
	MinMargin              int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	stripPatterns := []string{
		`\bCompl(ete)?\b`, `\bBDRemux\b`, `\bBDRip\b`,
		`\bWEB-?DL\b`, `\bWEBRip\b`, `\bBlu-?Ray\b`,
		`\bHDTV\b`, `\bDVDRip\b`, `\bHDRip\b`,
		`\biNTERNAL\b`, `\bPROPER\b`, `\bREPACK\b`,
		`\bXviD\b`, `\bDivX\b`, `\bAFG\b`,
		`\bEZTVx?\b`, `\bTGx\b`, `\bGalaxyTV\b`,
		`\bNNMClub\b`, `\bRutracker\b`,
		`\b(AMZN|NF|DSNP|HMAX|ATVP|PMTP)\b`,
	}
	forceMoviePatterns := []string{`^\d+\.`}

	compiled := func(patterns []string) []*regexp.Regexp {
		out := make([]*regexp.Regexp, 0, len(patterns))
		for _, p := range patterns {
			if re, err := regexp.Compile(p); err == nil {
				out = append(out, re)
			}
		}
		return out
	}

	cfg := &Config{
		StripPatterns:      compiled(stripPatterns),
		ForceMoviePatterns: compiled(forceMoviePatterns),
		MinScore:           88,
		MinScoreNoYear:     94,
		MinMargin:          6,
	}
	_ = cfg.ApplyDictionaries(DefaultDictionaries())
	return cfg
}
