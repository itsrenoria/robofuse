package tvmaze

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	gourl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/robofuse/robofuse/internal/request"
)

// tvmaze.go — TVMaze API client as a fallback when TMDB doesn't match.
// TVMaze has no API key requirement and good anime/older show coverage.

const baseURL = "https://api.tvmaze.com"

// Client makes requests to the TVMaze API.
type Client struct {
	reqClient *request.Client
}

// New creates a new TVMaze client.
func New() *Client {
	return &Client{
		reqClient: request.New(request.WithTimeout(10*time.Second), request.WithMaxRetries(1)),
	}
}

// ShowResult is a simplified match result compatible with TMDB types.
type ShowResult struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Year    int
	Summary string `json:"summary"`
	Image   string `json:"image"` // URL
	URL     string `json:"url"`
}

// searchResult is the TVMaze search API response.
type searchResult struct {
	Score float64    `json:"score"`
	Show  showDetail `json:"show"`
}

type showDetail struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	Summary string     `json:"summary"`
	Image   *imageInfo `json:"image"`
	URL     string     `json:"url"`
}

type imageInfo struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

// SearchShow searches TVMaze by name.
func (c *Client) SearchShow(name string) (*ShowResult, error) {
	return c.SearchShowContext(context.Background(), name, 0)
}

func (c *Client) SearchShowContext(ctx context.Context, name string, year int) (*ShowResult, error) {
	q := gourl.Values{}
	q.Set("q", name)

	var results []searchResult
	if err := c.get(ctx, "/search/shows", q, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	// TODO: year matching requires fetching show details
	var best *searchResult
	bestIsExact := false
	for i := range results {
		r := &results[i]
		exact := strings.EqualFold(r.Show.Name, name)
		if best == nil {
			best = r
			bestIsExact = exact
		} else if bestIsExact && !exact {
			continue // don't replace exact with non-exact
		} else if exact && !bestIsExact {
			best = r
			bestIsExact = true
		} else if !exact && r.Score > best.Score {
			best = r
			bestIsExact = false
		}
	}
	if best == nil {
		return nil, nil
	}

	sr := &ShowResult{
		ID:      best.Show.ID,
		Name:    best.Show.Name,
		Summary: stripHTML(best.Show.Summary),
	}
	if best.Show.Image != nil {
		sr.Image = best.Show.Image.Original
	}

	return sr, nil
}

// GetShow fetches full show details including premiere date for year extraction.
func (c *Client) GetShow(id int) (*ShowResult, error) {
	return c.GetShowContext(context.Background(), id)
}

func (c *Client) GetShowContext(ctx context.Context, id int) (*ShowResult, error) {
	var detail struct {
		ID        int        `json:"id"`
		Name      string     `json:"name"`
		Summary   string     `json:"summary"`
		Premiered string     `json:"premiered"` // YYYY-MM-DD
		Image     *imageInfo `json:"image"`
		URL       string     `json:"url"`
	}
	if err := c.get(ctx, fmt.Sprintf("/shows/%d", id), nil, &detail); err != nil {
		return nil, err
	}

	sr := &ShowResult{
		ID:      detail.ID,
		Name:    detail.Name,
		Summary: stripHTML(detail.Summary),
		URL:     detail.URL,
	}
	if len(detail.Premiered) >= 4 {
		sr.Year, _ = strconv.Atoi(detail.Premiered[:4])
	}
	if detail.Image != nil {
		sr.Image = detail.Image.Original
	}
	return sr, nil
}

func (c *Client) get(ctx context.Context, path string, params gourl.Values, target interface{}) error {
	u, _ := gourl.Parse(baseURL + path)
	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("tvmaze request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.reqClient.Do(req)
	if err != nil {
		return fmt.Errorf("tvmaze request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tvmaze API %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// stripHTML removes basic HTML tags from summary text.
func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<p>", "")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "<b>", "")
	s = strings.ReplaceAll(s, "</b>", "")
	s = strings.ReplaceAll(s, "<i>", "")
	s = strings.ReplaceAll(s, "</i>", "")
	return strings.TrimSpace(s)
}
