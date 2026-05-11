package matcher

import "strings"

// DictionaryConfig contains matcher dictionaries that can be supplied without
// changing matcher code. Keep values simple: string lists and alias maps.
type DictionaryConfig struct {
	NoiseTokens            []string
	TitleAliases           map[string][]string
	TransliterationAliases map[string][]string
	AnimeKeywords          []string
	CollectionKeywords     []string
}

// DefaultDictionaries returns dictionary defaults matching the current matcher
// behavior.
func DefaultDictionaries() DictionaryConfig {
	return DictionaryConfig{
		NoiseTokens: []string{
			"complete", "compl", "season", "episode",
			"internal", "proper", "repack", "xvid", "divx", "afg",
			"eztv", "eztvx", "tgx", "galaxytv", "rarbg", "yts", "yify",
			"rus", "ru", "russian", "chinese",
			"jpn", "jp", "japanese", "eng", "english",
			"multi", "dub", "dubbed", "sub", "subbed",
		},
		TitleAliases:           map[string][]string{},
		TransliterationAliases: map[string][]string{},
		AnimeKeywords: []string{
			"subsplease", "erai-raws", "judas", "ember", "asw",
			"dkb", "nep_blanc", "lostyears", "akihitosubs",
			"philosophy-raws", "commie", "coalgirls", "hi10p",
			"dual audio", "dual-audio", "multi-audio",
		},
		CollectionKeywords: []string{
			"collection", "trilogy", "quadrilogy", "saga", "anthology",
			"complete", "boxset", "box set", "franchise",
		},
	}
}

// ApplyDictionaries replaces the dictionary-backed config fields.
func (cfg *Config) ApplyDictionaries(dict DictionaryConfig) error {
	if cfg == nil {
		return nil
	}
	cfg.NoiseTokens = copyStrings(dict.NoiseTokens)
	cfg.TitleAliases = copyAliasMap(dict.TitleAliases)
	cfg.TransliterationAliases = copyAliasMap(dict.TransliterationAliases)
	cfg.AnimeKeywords = copyStrings(dict.AnimeKeywords)
	cfg.CollectionKeywords = copyStrings(dict.CollectionKeywords)
	return nil
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func copyAliasMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, aliases := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = copyStrings(aliases)
	}
	return out
}
