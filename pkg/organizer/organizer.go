// Package organizer provides media file organization functionality.
// It parses torrent filenames using ptt-go and organizes them into
// Movies/Series/Anime folder structures.
package organizer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	ptt "github.com/itsrenoria/ptt-go"
	"github.com/robofuse/robofuse/pkg/namefmt"
)

var parserPool = sync.Pool{
	New: func() interface{} {
		p := ptt.NewParser()
		ptt.AddDefaults(p)
		return p
	},
}

// organizer.go handles parsing and output path construction for media items.

// ContentPathOptions holds all inputs needed to calculate an organized destination path.
type ContentPathOptions struct {
	// File identity
	Filename      string // e.g. "He.Man.S01E01.1080p.avi.strm"
	TorrentFolder string // e.g. "He.Man.S01.1080p"
	RDID          string // extracted RD link ID

	// TMDB enrichment (may be empty if not matched)
	TMDBTitle         string
	TMDBOriginalTitle string
	TMDBYear          int
	TMDBType          string // "movie" or "show"
	TMDBContentRating string
	EpisodeSeason     int
	EpisodeNumber     int
	EpisodeTitle      string

	// Routing rules
	AdultPatterns []string
	FolderRules   []FolderRule
	KidsMaxRating string
	KidsFolder    string
	AnimeFolder   string
	MovieFolder   string
	SeriesFolder  string

	// Folder templates
	MovieFolderTemplate  string
	SeriesFolderTemplate string
	SeasonFolderTemplate string

	// Pre-determined classification
	Category    string // pre-determined category from categorizeTorrent
	TMDBIsAnime bool   // true if TMDB identified this as anime

	// Existing folders lookup
	OrganizedDir string
}

// FolderRule is a custom routing rule.
type FolderRule struct {
	Pattern  string
	Target   string
	SkipTMDB bool
	Adult    bool
}

// ExistingFolderOptions holds inputs for FindExistingSeriesFolder.
type ExistingFolderOptions struct {
	OrganizedDir string
	BaseFolder   string
	Title        string
	Year         int
}

var illegalCharsRegex = regexp.MustCompile(`[<>:"/\\|?*]`)

// cleanFilename removes illegal filesystem characters.
func cleanFilename(name string) string {
	return illegalCharsRegex.ReplaceAllString(name, "")
}

func hasAnimeKeyword(names ...string) bool {
	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "anime") {
			return true
		}
	}
	return false
}

// safeFolderName returns a sanitized single folder name component.
// Strips path separators and parent directory traversal.
func safeFolderName(name string) string {
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, `\`, "")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.TrimSpace(name)
	if name == "" {
		return "Unknown"
	}
	return name
}

func folderTemplateValues(title, originalTitle string, year int, season, episode []int, episodeTitle, extension string) namefmt.Values {
	v := namefmt.Values{
		Title:         title,
		OriginalTitle: originalTitle,
		Year:          year,
		EpisodeTitle:  episodeTitle,
		Extension:     extension,
	}
	if len(season) > 0 {
		v.Season = season[0]
	}
	if len(episode) > 0 {
		v.Episode = episode[0]
	}
	return v
}

func formatFolderComponent(tmpl string, values namefmt.Values) string {
	if tmpl == "" {
		return ""
	}
	folder := namefmt.Clean(namefmt.Format(tmpl, values))
	return safeFolderName(folder)
}

func animeExtraFolder(filename, episodeTitle string) string {
	title := strings.TrimSpace(episodeTitle)
	if title == "" {
		title = namefmt.EpisodeTitleFromFilename(filename)
	}
	if label, ok := canonicalAnimeExtraLabel(title); ok {
		switch label {
		case "Special":
			return "Specials"
		default:
			return "Extras"
		}
	}
	return ""
}

func canonicalAnimeExtraLabel(title string) (string, bool) {
	token := strings.TrimSpace(title)
	if token == "" {
		return "", false
	}
	fields := strings.Fields(token)
	if len(fields) == 0 {
		return "", false
	}
	switch strings.ToUpper(fields[0]) {
	case "NCOP", "NCED", "PV", "CM", "OVA", "OAD", "ONA":
		return strings.ToUpper(fields[0]), true
	case "SPECIAL", "PREVIEW", "TRAILER", "TEASER":
		word := strings.ToLower(fields[0])
		return strings.ToUpper(word[:1]) + word[1:], true
	default:
		return "", false
	}
}

// FindExistingSeriesFolder looks for an existing series folder in the organized directory.
func FindExistingSeriesFolder(opts ExistingFolderOptions) string {
	searchDir := filepath.Join(opts.OrganizedDir, opts.BaseFolder)
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return ""
	}

	normalizedTitle := strings.ToLower(strings.TrimSpace(opts.Title))
	targetWithYear := opts.Title
	if opts.Year > 0 {
		targetWithYear = fmt.Sprintf("%s (%d)", opts.Title, opts.Year)
	}

	// Check for exact matches first
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.EqualFold(entry.Name(), targetWithYear) {
			return entry.Name()
		}
	}

	// Check for title match with/without year
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folderLower := strings.ToLower(entry.Name())
		if strings.HasPrefix(folderLower, normalizedTitle) {
			remainder := strings.TrimSpace(strings.TrimPrefix(folderLower, normalizedTitle))
			if remainder == "" || (strings.HasPrefix(remainder, "(") && strings.HasSuffix(remainder, ")") && len(remainder) == 6) {
				return entry.Name()
			}
		}
	}

	return ""
}

// isAdultPath checks if a source path matches any adult pattern or folder rule.
func isAdultPath(sourceRelPath string, adultPatterns []string, folderRules []FolderRule) bool {
	// Check folder portion
	folder := strings.ToLower(filepath.Dir(sourceRelPath))
	for _, p := range adultPatterns {
		if p != "" && strings.Contains(folder, strings.ToLower(p)) {
			return true
		}
	}
	for _, r := range folderRules {
		if !r.Adult || r.Pattern == "" {
			continue
		}
		if strings.HasPrefix(r.Pattern, "~") {
			re, err := regexp.Compile(r.Pattern[1:])
			if err == nil && re.MatchString(filepath.Dir(sourceRelPath)) {
				return true
			}
		} else if strings.Contains(folder, strings.ToLower(r.Pattern)) {
			return true
		}
	}

	// Check filename portion
	filename := strings.ToLower(filepath.Base(sourceRelPath))
	for _, p := range adultPatterns {
		if p != "" && strings.Contains(filename, strings.ToLower(p)) {
			return true
		}
	}
	for _, r := range folderRules {
		if !r.Adult || r.Pattern == "" {
			continue
		}
		if strings.HasPrefix(r.Pattern, "~") {
			re, err := regexp.Compile(r.Pattern[1:])
			if err == nil && re.MatchString(filepath.Base(sourceRelPath)) {
				return true
			}
		} else if strings.Contains(filename, strings.ToLower(r.Pattern)) {
			return true
		}
	}

	return false
}

// buildAdultPath builds a destination path for adult content using folder rules.
func buildAdultPath(sourceRelPath, filename, rdID string, folderRules []FolderRule) string {
	folderName := filepath.Base(filepath.Dir(sourceRelPath))
	cleanFolder := cleanFilename(folderName)

	// Determine target folder from rules
	target := "X"
	folderLower := strings.ToLower(filepath.Dir(sourceRelPath))
	for _, r := range folderRules {
		if r.Pattern == "" {
			continue
		}
		if !r.Adult {
			continue
		}
		if strings.Contains(folderLower, strings.ToLower(r.Pattern)) {
			target = r.Target
			break
		}
	}

	ext := realSTRMExt(filename)
	idSuffix := ""
	if rdID != "" {
		idSuffix = fmt.Sprintf(" [%s]", rdID)
	}
	baseName := mediaBaseName(filename)
	cleanFile := cleanFilename(fmt.Sprintf("%s%s%s", baseName, idSuffix, ext))

	cleanTarget := safeFolderName(target)
	return filepath.Join(cleanTarget, cleanFolder, cleanFile)
}

// CalculateContentPath determines content type and destination relative path
// for a media file. Returns the content type (movie/series/anime/adult/kids)
// and the relative destination path within the organized directory.
//
// This is a standalone version of getContentTypeAndPath that accepts explicit
// options instead of relying on the Organizer struct.
func CalculateContentPath(opts ContentPathOptions) (contentType string, destRelPath string) {
	fullRelPath := filepath.Join(opts.TorrentFolder, opts.Filename)

	if isAdultPath(fullRelPath, opts.AdultPatterns, opts.FolderRules) {
		return "adult", buildAdultPath(fullRelPath, opts.Filename, opts.RDID, opts.FolderRules)
	}

	nameNoExt := strings.TrimSuffix(opts.Filename, filepath.Ext(opts.Filename))
	parser := parserPool.Get().(*ptt.Parser)
	defer parserPool.Put(parser)
	parsed := parser.Parse(nameNoExt)

	parentFolderName := ""
	if opts.TorrentFolder != "" && opts.TorrentFolder != "." {
		parentFolderName = filepath.Base(opts.TorrentFolder)
	}
	var parentParsed *ptt.TorrentInfo
	if parentFolderName != "" {
		parentParsed = parser.Parse(parentFolderName)
	}

	// Extract info from filename
	fTitle := parsed.Title
	fYear := parsed.Year
	fSeason := parsed.Seasons
	fEpisode := parsed.Episodes
	fAnime := parsed.Anime

	// Extract info from parent
	pTitle := ""
	pYear := 0
	var pSeason, pEpisode []int
	pAnime := false
	if parentParsed != nil {
		pTitle = parentParsed.Title
		pYear = parentParsed.Year
		pSeason = parentParsed.Seasons
		pEpisode = parentParsed.Episodes
		pAnime = parentParsed.Anime
	}

	hasStoredEpisode := opts.EpisodeSeason > 0 || opts.EpisodeNumber > 0

	// Determine if series (PTT provides base detection; category override refines type)
	isSeriesFilename := hasStoredEpisode || len(fSeason) > 0 || len(fEpisode) > 0 || fAnime
	isSeriesParent := len(pSeason) > 0 || len(pEpisode) > 0 || pAnime

	var finalType, title string
	var year int
	var season, episode []int

	if isSeriesParent {
		finalType = "series"

		if pTitle != "" {
			title = pTitle
		} else {
			title = "Unknown"
		}

		if pYear > 0 {
			year = pYear
		} else {
			year = fYear
		}

		if opts.EpisodeSeason > 0 {
			season = []int{opts.EpisodeSeason}
		} else if len(fSeason) > 0 {
			season = fSeason
		} else {
			season = pSeason
		}

		if opts.EpisodeNumber > 0 {
			episode = []int{opts.EpisodeNumber}
		} else if len(fEpisode) > 0 {
			episode = fEpisode
		}
	} else if isSeriesFilename {
		finalType = "series"
		if fTitle != "" {
			title = fTitle
		} else {
			title = "Unknown"
		}
		year = fYear
		if opts.EpisodeSeason > 0 {
			season = []int{opts.EpisodeSeason}
		} else {
			season = fSeason
		}
		if opts.EpisodeNumber > 0 {
			episode = []int{opts.EpisodeNumber}
		} else {
			episode = fEpisode
		}
	} else {
		finalType = "movie"
		if fTitle != "" {
			title = fTitle
		} else if pTitle != "" {
			title = pTitle
		} else {
			title = "Unknown"
		}
		if fYear > 0 {
			year = fYear
		} else {
			year = pYear
		}
	}

	// Category override: if upstream already determined the type, trust it over PTT
	if opts.Category == "anime" {
		finalType = "anime"
	} else if opts.Category == "series" {
		finalType = "series"
	} else if opts.Category == "movie" {
		finalType = "movie"
	} else if opts.Category == "adult" {
		finalType = "adult"
	} else if opts.Category == "unmatched" {
		finalType = "unmatched"
	}

	if finalType == "unmatched" {
		baseName := mediaBaseName(opts.Filename)
		ext := realSTRMExt(opts.Filename)
		idSuffix := ""
		if opts.RDID != "" {
			idSuffix = fmt.Sprintf(" [%s]", opts.RDID)
		}
		cleanFile := cleanFilename(fmt.Sprintf("%s%s%s", baseName, idSuffix, ext))
		folder := cleanFilename(opts.TorrentFolder)
		if folder == "" {
			folder = "Unknown"
		}
		return "unmatched", filepath.Join("unmatched", folder, cleanFile)
	}

	// TMDB override: if we have an official match, use its title, year, and type
	if opts.TMDBTitle != "" && finalType != "adult" {
		wasAnime := finalType == "anime"
		title = opts.TMDBTitle
		if opts.TMDBYear > 0 {
			year = opts.TMDBYear
		}
		if opts.TMDBType == "show" {
			if !wasAnime {
				finalType = "series"
			}
		} else if opts.TMDBType == "movie" {
			finalType = "movie"
		}
	}

	// TMDB anime override: if TMDB says anime, route to anime folder.
	if opts.TMDBIsAnime {
		finalType = "anime"
	}

	// Kids content routing: if rating qualifies, override to kids folder
	if opts.KidsMaxRating != "" && opts.TMDBContentRating != "" {
		if RatingIsKids(opts.TMDBContentRating, opts.KidsMaxRating) {
			kidsFolder := opts.KidsFolder
			if kidsFolder == "" {
				kidsFolder = "Kids"
			}
			kidsFolder = safeFolderName(kidsFolder)
			kidsType := "Movies"
			if finalType == "series" {
				kidsType = "Series"
			} else if finalType == "anime" {
				kidsType = "Anime"
			}
			baseFolder := filepath.Join(kidsFolder, kidsType)
			// Build path under kids folder
			cleanTitle := cleanFilename(title)
			if year > 0 {
				cleanTitle = cleanFilename(fmt.Sprintf("%s (%d)", title, year))
			}
			ext := realSTRMExt(opts.Filename)
			idSuffix := ""
			if opts.RDID != "" {
				idSuffix = fmt.Sprintf(" [%s]", opts.RDID)
			}
			baseName := mediaBaseName(opts.Filename)
			cleanFile := cleanFilename(fmt.Sprintf("%s%s%s", baseName, idSuffix, ext))
			return "kids", filepath.Join(baseFolder, cleanTitle, cleanFile)
		}
	}

	// Determine base folder
	animeMovie := opts.TMDBIsAnime && opts.TMDBType == "movie"
	var baseFolder string
	switch {
	case finalType == "adult":
		return "adult", buildAdultPath(fullRelPath, opts.Filename, opts.RDID, opts.FolderRules)
	case finalType == "anime" && opts.AnimeFolder != "":
		baseFolder = safeFolderName(opts.AnimeFolder)
	case finalType == "anime":
		baseFolder = "Anime"
	case finalType == "series":
		baseFolder = opts.SeriesFolder
		if baseFolder == "" {
			baseFolder = "Series"
		}
	default:
		baseFolder = opts.MovieFolder
		if baseFolder == "" {
			baseFolder = "Movies"
		}
	}

	ext := realSTRMExt(opts.Filename)
	values := folderTemplateValues(title, opts.TMDBOriginalTitle, year, season, episode, opts.EpisodeTitle, ext)

	var formattedTitle string
	var titleFolderTemplate string
	if finalType == "movie" || animeMovie {
		titleFolderTemplate = opts.MovieFolderTemplate
	} else {
		titleFolderTemplate = opts.SeriesFolderTemplate
	}

	if titleFolderTemplate != "" {
		formattedTitle = formatFolderComponent(titleFolderTemplate, values)
	} else {
		// Check for existing folder
		existingFolder := FindExistingSeriesFolder(ExistingFolderOptions{
			OrganizedDir: opts.OrganizedDir,
			BaseFolder:   baseFolder,
			Title:        title,
			Year:         year,
		})
		if existingFolder != "" {
			formattedTitle = existingFolder
		} else {
			formattedTitle = title
			if year > 0 {
				formattedTitle = fmt.Sprintf("%s (%d)", formattedTitle, year)
			}
			formattedTitle = cleanFilename(formattedTitle)
		}
	}

	// ID suffix
	idSuffix := ""
	if opts.RDID != "" {
		idSuffix = fmt.Sprintf(" [%s]", opts.RDID)
	}

	if finalType == "movie" || animeMovie {
		baseName := mediaBaseName(opts.Filename)
		cleanFile := cleanFilename(fmt.Sprintf("%s%s%s", baseName, idSuffix, ext))
		destRelPath = filepath.Join(baseFolder, formattedTitle, cleanFile)
	} else {
		// Series or Anime
		var seasonFolder string
		if finalType == "anime" {
			switch extraFolder := animeExtraFolder(opts.Filename, opts.EpisodeTitle); {
			case extraFolder != "":
				seasonFolder = extraFolder
			case len(season) > 0:
				if opts.SeasonFolderTemplate != "" {
					seasonFolder = formatFolderComponent(opts.SeasonFolderTemplate, values)
				} else {
					seasonFolder = fmt.Sprintf("Season %02d", season[0])
				}
			default:
				seasonFolder = ""
			}
		} else if opts.SeasonFolderTemplate != "" {
			seasonFolder = formatFolderComponent(opts.SeasonFolderTemplate, values)
		} else if len(season) > 0 {
			seasonFolder = fmt.Sprintf("Season %02d", season[0])
		} else {
			seasonFolder = "Season Unknown"
		}

		baseName := mediaBaseName(opts.Filename)
		cleanFile := cleanFilename(fmt.Sprintf("%s%s%s", baseName, idSuffix, ext))

		parts := []string{baseFolder, formattedTitle}
		if seasonFolder != "" {
			parts = append(parts, seasonFolder)
		}
		parts = append(parts, cleanFile)
		destRelPath = filepath.Join(parts...)
	}

	return finalType, destRelPath
}

func mediaBaseName(filename string) string {
	base := filepath.Base(filename)
	if strings.HasSuffix(strings.ToLower(base), ".strm") {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// realSTRMExt extracts the real media extension from a .strm filename.
// Always returns .original.strm format.
// "Bluey.avi.strm" → ".avi.strm", "Movie.mkv" → ".mkv.strm", "Movie.strm" → ".strm"
func realSTRMExt(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".strm") {
		ext := strings.ToLower(filepath.Ext(strings.TrimSuffix(filename, ".strm")))
		knownExts := map[string]bool{
			".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
			".wmv": true, ".flv": true, ".mpeg": true, ".mpg": true,
			".m4v": true, ".ts": true, ".webm": true, ".vob": true,
			".m2ts": true, ".3gp": true,
		}
		if knownExts[ext] {
			return ext + ".strm"
		}
		return ".strm"
	}
	return filepath.Ext(filename) + ".strm"
}

// RatingIsKids returns true if the content rating qualifies for kids routing.
func RatingIsKids(rating, max string) bool {
	idx := map[string]int{
		"TV-Y": 1, "G": 1,
		"TV-Y7": 2, "PG": 2,
		"TV-G":  3,
		"TV-PG": 4, "PG-13": 4,
		"TV-14": 5,
		"R":     6, "TV-MA": 6,
		"NC-17": 7,
	}
	ratingIdx, ratingKnown := idx[rating]
	maxIdx, maxKnown := idx[max]
	if !ratingKnown || !maxKnown {
		return false
	}
	return ratingIdx <= maxIdx
}
