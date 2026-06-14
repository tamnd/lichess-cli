// Package lichess is the library behind the lichess command line:
// the HTTP client, request shaping, and the typed data models for Lichess.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package lichess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to Lichess.
const DefaultUserAgent = "lichess-cli/0.1.0 (github.com/tamnd/lichess-cli)"

// Host is the site this client talks to.
const Host = "lichess.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host + "/api"

// Config holds all tunables for the client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with sensible defaults for the Lichess API.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to Lichess over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// --- Domain types ---

// User is a Lichess player profile.
type User struct {
	Username     string `kit:"id" json:"username"`
	Title        string `json:"title,omitempty"`
	URL          string `json:"url,omitempty"`
	BulletRating int    `json:"bullet_rating,omitempty"`
	BlitzRating  int    `json:"blitz_rating,omitempty"`
	RapidRating  int    `json:"rapid_rating,omitempty"`
	TotalGames   int    `json:"total_games"`
	WinCount     int    `json:"wins"`
	LossCount    int    `json:"losses"`
	DrawCount    int    `json:"draws"`
}

// PerfStat is performance statistics for one perf type.
type PerfStat struct {
	Username   string  `json:"username"`
	PerfType   string  `json:"perf_type"`
	Games      int     `json:"games"`
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
	Draws      int     `json:"draws"`
	Rating     int     `json:"rating,omitempty"`
	Rank       int     `json:"rank,omitempty"`
	Percentile float64 `json:"percentile,omitempty"`
	WinStreak  int     `json:"win_streak,omitempty"`
	PlayStreak int     `json:"play_streak,omitempty"`
}

// Puzzle is the daily Lichess puzzle.
type Puzzle struct {
	ID       string `kit:"id" json:"id"`
	Rating   int    `json:"rating"`
	Plays    int    `json:"plays"`
	Themes   string `json:"themes"`   // comma-joined
	Solution string `json:"solution"` // space-joined moves
}

// Game is one Lichess game record.
type Game struct {
	ID      string `kit:"id" json:"id"`
	White   string `json:"white"`   // white player name
	Black   string `json:"black"`   // black player name
	Winner  string `json:"winner"`  // "white", "black", or ""
	Status  string `json:"status"`  // status.name
	Variant string `json:"variant"` // speed/perf name
	Moves   string `json:"moves"`   // first 20 space-separated moves
}

// TVGame is the current Lichess TV broadcast game.
type TVGame struct {
	ID          string `json:"id"`
	FEN         string `json:"fen"`
	Color       string `json:"color"`
	Speed       string `json:"speed"`
	WhitePlayer string `json:"white_player"`
	BlackPlayer string `json:"black_player"`
	WhiteRating int    `json:"white_rating,omitempty"`
	BlackRating int    `json:"black_rating,omitempty"`
}

// TopPlayer is one entry in a Lichess leaderboard.
type TopPlayer struct {
	Username string `kit:"id" json:"username"`
	Title    string `json:"title,omitempty"`
	Rating   int    `json:"rating"`
}

// LeaderEntry is an alias kept for backward compat within this package.
type LeaderEntry = TopPlayer

// --- internal API response types ---

type apiUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Title    string `json:"title"`
	Perfs    struct {
		Bullet    apiPerf `json:"bullet"`
		Blitz     apiPerf `json:"blitz"`
		Rapid     apiPerf `json:"rapid"`
		Classical apiPerf `json:"classical"`
	} `json:"perfs"`
	Count struct {
		All  int `json:"all"`
		Win  int `json:"win"`
		Loss int `json:"loss"`
		Draw int `json:"draw"`
	} `json:"count"`
	URL      string `json:"url"`
	Patron   bool   `json:"patron"`
	Verified bool   `json:"verified"`
}

type apiPerf struct {
	Games  int `json:"games"`
	Rating int `json:"rating"`
}

type apiPerfStat struct {
	Stat struct {
		Count struct {
			All  int `json:"all"`
			Win  int `json:"win"`
			Loss int `json:"loss"`
			Draw int `json:"draw"`
		} `json:"count"`
		ResultStreak struct {
			Win struct {
				Cur struct {
					V int `json:"v"`
				} `json:"cur"`
			} `json:"win"`
		} `json:"resultStreak"`
		PlayStreak struct {
			Nb struct {
				V int `json:"v"`
			} `json:"nb"`
		} `json:"playStreak"`
	} `json:"stat"`
	Rank       int     `json:"rank"`
	Percentile float64 `json:"percentile"`
}

type apiPuzzleResp struct {
	Puzzle struct {
		ID       string   `json:"id"`
		Rating   int      `json:"rating"`
		Plays    int      `json:"plays"`
		Solution []string `json:"solution"`
		Themes   []string `json:"themes"`
	} `json:"puzzle"`
	Game struct {
		ID string `json:"id"`
	} `json:"game"`
}

type apiGame struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Speed   string `json:"speed"`
	Variant string `json:"variant"`
	Players struct {
		White apiPlayer `json:"white"`
		Black apiPlayer `json:"black"`
	} `json:"players"`
	Moves  string `json:"moves"`
	Winner string `json:"winner"`
}

type apiPlayer struct {
	User struct {
		Name string `json:"name"`
	} `json:"user"`
	Rating int `json:"rating"`
}

type apiTV struct {
	ID      string `json:"id"`
	FEN     string `json:"fen"`
	Color   string `json:"color"`
	Speed   string `json:"speed"`
	Players struct {
		White apiPlayer `json:"white"`
		Black apiPlayer `json:"black"`
	} `json:"players"`
}

type apiLeaderboard struct {
	Users []apiLeaderUser `json:"users"`
}

type apiLeaderUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Title    string `json:"title"`
	Perfs    map[string]struct {
		Rating int `json:"rating"`
	} `json:"perfs"`
}

// --- Client methods ---

// GetUser fetches a player profile by username.
func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	u := fmt.Sprintf("%s/user/%s", c.cfg.BaseURL, url.PathEscape(username))
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiUser
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	return &User{
		Username:     api.Username,
		Title:        api.Title,
		URL:          api.URL,
		BulletRating: api.Perfs.Bullet.Rating,
		BlitzRating:  api.Perfs.Blitz.Rating,
		RapidRating:  api.Perfs.Rapid.Rating,
		TotalGames:   api.Count.All,
		WinCount:     api.Count.Win,
		LossCount:    api.Count.Loss,
		DrawCount:    api.Count.Draw,
	}, nil
}

// GetPerfStat fetches performance statistics for a player and perf type.
func (c *Client) GetPerfStat(ctx context.Context, username, perfType string) (*PerfStat, error) {
	if perfType == "" {
		perfType = "blitz"
	}
	u := fmt.Sprintf("%s/user/%s/perf/%s", c.cfg.BaseURL, url.PathEscape(username), perfType)
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiPerfStat
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode perf stat: %w", err)
	}
	return &PerfStat{
		Username:   username,
		PerfType:   perfType,
		Games:      api.Stat.Count.All,
		Wins:       api.Stat.Count.Win,
		Losses:     api.Stat.Count.Loss,
		Draws:      api.Stat.Count.Draw,
		Rank:       api.Rank,
		Percentile: api.Percentile,
		WinStreak:  api.Stat.ResultStreak.Win.Cur.V,
		PlayStreak: api.Stat.PlayStreak.Nb.V,
	}, nil
}

// GetPuzzle fetches the daily puzzle.
func (c *Client) GetPuzzle(ctx context.Context) (*Puzzle, error) {
	u := fmt.Sprintf("%s/puzzle/daily", c.cfg.BaseURL)
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiPuzzleResp
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode puzzle: %w", err)
	}
	return &Puzzle{
		ID:       api.Puzzle.ID,
		Rating:   api.Puzzle.Rating,
		Plays:    api.Puzzle.Plays,
		Themes:   strings.Join(api.Puzzle.Themes, ","),
		Solution: strings.Join(api.Puzzle.Solution, " "),
	}, nil
}

// GetPuzzleByID fetches a puzzle by its ID.
func (c *Client) GetPuzzleByID(ctx context.Context, id string) (*Puzzle, error) {
	u := fmt.Sprintf("%s/puzzle/%s", c.cfg.BaseURL, url.PathEscape(id))
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiPuzzleResp
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode puzzle: %w", err)
	}
	return &Puzzle{
		ID:       api.Puzzle.ID,
		Rating:   api.Puzzle.Rating,
		Plays:    api.Puzzle.Plays,
		Themes:   strings.Join(api.Puzzle.Themes, ","),
		Solution: strings.Join(api.Puzzle.Solution, " "),
	}, nil
}

// GetGames fetches recent games for a player as NDJSON.
func (c *Client) GetGames(ctx context.Context, username string, limit int, perfType string) ([]Game, error) {
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{}
	q.Set("max", fmt.Sprintf("%d", limit))
	if perfType != "" && perfType != "all" {
		q.Set("perfType", perfType)
	}
	u := fmt.Sprintf("%s/games/user/%s?%s", c.cfg.BaseURL, url.PathEscape(username), q.Encode())
	body, err := c.get(ctx, u, "application/x-ndjson")
	if err != nil {
		return nil, err
	}
	return parseNDJSON(body)
}

// GetTV fetches the current TV game.
func (c *Client) GetTV(ctx context.Context) (*TVGame, error) {
	u := fmt.Sprintf("%s/tv/current", c.cfg.BaseURL)
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiTV
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode tv: %w", err)
	}
	return &TVGame{
		ID:          api.ID,
		FEN:         api.FEN,
		Color:       api.Color,
		Speed:       api.Speed,
		WhitePlayer: api.Players.White.User.Name,
		BlackPlayer: api.Players.Black.User.Name,
		WhiteRating: api.Players.White.Rating,
		BlackRating: api.Players.Black.Rating,
	}, nil
}

// GetLeaderboard fetches the leaderboard for a perf type.
func (c *Client) GetLeaderboard(ctx context.Context, nb int, perfType string) ([]LeaderEntry, error) {
	if nb <= 0 {
		nb = 5
	}
	if perfType == "" {
		perfType = "bullet"
	}
	u := fmt.Sprintf("%s/player/top/%d/%s", c.cfg.BaseURL, nb, perfType)
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	var api apiLeaderboard
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, fmt.Errorf("decode leaderboard: %w", err)
	}
	entries := make([]LeaderEntry, 0, len(api.Users))
	for _, u := range api.Users {
		rating := 0
		if p, ok := u.Perfs[perfType]; ok {
			rating = p.Rating
		}
		entries = append(entries, LeaderEntry{
			Username: u.Username,
			Title:    u.Title,
			Rating:   rating,
		})
	}
	return entries, nil
}

// --- NDJSON parsing ---

func parseNDJSON(body []byte) ([]Game, error) {
	var games []Game
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var api apiGame
		if err := json.Unmarshal(line, &api); err != nil {
			return nil, fmt.Errorf("decode game line: %w", err)
		}
		games = append(games, Game{
			ID:      api.ID,
			White:   api.Players.White.User.Name,
			Black:   api.Players.Black.User.Name,
			Winner:  api.Winner,
			Status:  api.Status,
			Variant: api.Speed, // use speed as the variant label
			Moves:   first20Moves(api.Moves),
		})
	}
	return games, sc.Err()
}

// first20Moves returns the first 20 space-separated tokens from a moves string.
func first20Moves(moves string) string {
	parts := strings.Fields(moves)
	if len(parts) > 20 {
		parts = parts[:20]
	}
	return strings.Join(parts, " ")
}

// --- HTTP layer ---

func (c *Client) get(ctx context.Context, u string, accept string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, u, accept)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", u, lastErr)
}

func (c *Client) do(ctx context.Context, u string, accept string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*500*time.Millisecond, 5*time.Second)
}

// --- scaffold compatibility: Page type kept for domain.go ---

// Page is used by the kit domain driver as the URI-addressable resource.
type Page struct {
	ID    string `json:"id" kit:"id"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty" kit:"body"`
}

// GetPage fetches one page by its path.
func (c *Client) GetPage(ctx context.Context, p string) (*Page, error) {
	p = strings.Trim(p, "/")
	u := "https://" + Host + "/" + p
	body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	return &Page{ID: p, URL: u, Title: p, Body: pageText(body)}, nil
}

// PageLinks is kept for the kit domain driver.
func (c *Client) PageLinks(ctx context.Context, p string, limit int) ([]*Page, error) {
	p = strings.Trim(p, "/")
	body, err := c.get(ctx, "https://"+Host+"/"+p, "")
	if err != nil {
		return nil, err
	}
	var out []*Page
	seen := map[string]bool{}
	for _, lp := range linkPaths(body) {
		if seen[lp] {
			continue
		}
		seen[lp] = true
		out = append(out, &Page{ID: lp, URL: "https://" + Host + "/" + lp})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func linkPaths(body []byte) []string {
	// simple href extractor for scaffold compatibility
	var out []string
	s := string(body)
	for {
		i := strings.Index(s, `href="`)
		if i < 0 {
			break
		}
		s = s[i+6:]
		j := strings.IndexByte(s, '"')
		if j < 0 {
			break
		}
		p := s[:j]
		s = s[j:]
		if strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "#?:") {
			p = strings.Trim(p, "/")
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func pageText(body []byte) string {
	s := string(body)
	// strip tags
	out := make([]byte, 0, len(s))
	inTag := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
		} else if s[i] == '>' {
			inTag = false
			out = append(out, ' ')
		} else if !inTag {
			out = append(out, s[i])
		}
	}
	r := strings.Join(strings.Fields(string(out)), " ")
	if len(r) > 500 {
		r = r[:500]
	}
	return r
}
