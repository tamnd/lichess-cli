package lichess_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/lichess-cli/lichess"
)

func newTestClient(ts *httptest.Server) *lichess.Client {
	cfg := lichess.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return lichess.NewClient(cfg)
}

const fakeUserJSON = `{
  "id": "drnykterstein",
  "username": "DrNykterstein",
  "title": "GM",
  "url": "https://lichess.org/@/DrNykterstein",
  "perfs": {
    "bullet":    {"games": 10450, "rating": 3270},
    "blitz":     {"games": 7893,  "rating": 3151},
    "rapid":     {"games": 100,   "rating": 2900},
    "classical": {"games": 20,    "rating": 2800}
  },
  "count": {"all": 10450, "win": 5678, "loss": 3000, "draw": 772},
  "patron": false,
  "verified": true
}`

const fakePerfStatJSON = `{
  "stat": {
    "count": {"all": 9593, "win": 6559, "loss": 2100, "draw": 934},
    "resultStreak": {"win": {"cur": {"v": 3}}},
    "playStreak":   {"nb":  {"v": 9593}}
  },
  "rank": 1,
  "percentile": 100.0
}`

const fakePuzzleJSON = `{
  "puzzle": {
    "id": "DJWZM",
    "rating": 1765,
    "plays": 90081,
    "solution": ["d1d7","g7f6","d7b7","b8b7","c7b7"],
    "themes": ["crushing","endgame","long"]
  },
  "game": {"id": "kRSCJTml"}
}`

const fakeGamesNDJSON = `{"id":"g1","status":"mate","speed":"bullet","variant":"standard","players":{"white":{"user":{"name":"DrNykterstein"},"rating":3270},"black":{"user":{"name":"Opponent"},"rating":3200}},"moves":"e4 e5 Nf3 Nc6 Bb5 a6","winner":"white"}
{"id":"g2","status":"resign","speed":"blitz","variant":"standard","players":{"white":{"user":{"name":"Opponent"},"rating":3200},"black":{"user":{"name":"DrNykterstein"},"rating":3270}},"moves":"d4 d5","winner":"black"}
`

const fakeLeaderboardJSON = `{
  "users": [
    {"id":"drnykterstein","username":"DrNykterstein","title":"GM","perfs":{"blitz":{"rating":3270}}},
    {"id":"gmopponent","username":"GMOpponent","title":"GM","perfs":{"blitz":{"rating":3200}}}
  ]
}`

func TestGetUserSendsUserAgent(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, fakeUserJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, _ = c.GetUser(context.Background(), "DrNykterstein")
	if !strings.Contains(got, "lichess-cli") {
		t.Errorf("User-Agent = %q, want to contain lichess-cli", got)
	}
}

func TestGetUserParsesProfile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeUserJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	u, err := c.GetUser(context.Background(), "DrNykterstein")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "DrNykterstein" {
		t.Errorf("Username = %q, want DrNykterstein", u.Username)
	}
	if u.Title != "GM" {
		t.Errorf("Title = %q, want GM", u.Title)
	}
	if u.URL != "https://lichess.org/@/DrNykterstein" {
		t.Errorf("URL = %q", u.URL)
	}
	if u.BulletRating != 3270 {
		t.Errorf("BulletRating = %d, want 3270", u.BulletRating)
	}
	if u.BlitzRating != 3151 {
		t.Errorf("BlitzRating = %d, want 3151", u.BlitzRating)
	}
	if u.RapidRating != 2900 {
		t.Errorf("RapidRating = %d, want 2900", u.RapidRating)
	}
	if u.TotalGames != 10450 {
		t.Errorf("TotalGames = %d, want 10450", u.TotalGames)
	}
	if u.WinCount != 5678 {
		t.Errorf("WinCount = %d, want 5678", u.WinCount)
	}
	if u.LossCount != 3000 {
		t.Errorf("LossCount = %d, want 3000", u.LossCount)
	}
	if u.DrawCount != 772 {
		t.Errorf("DrawCount = %d, want 772", u.DrawCount)
	}
}

func TestGetPerfStatParsesFields(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakePerfStatJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	ps, err := c.GetPerfStat(context.Background(), "DrNykterstein", "bullet")
	if err != nil {
		t.Fatal(err)
	}
	if ps.Username != "DrNykterstein" {
		t.Errorf("Username = %q", ps.Username)
	}
	if ps.PerfType != "bullet" {
		t.Errorf("PerfType = %q, want bullet", ps.PerfType)
	}
	if ps.Games != 9593 {
		t.Errorf("Games = %d, want 9593", ps.Games)
	}
	if ps.Wins != 6559 {
		t.Errorf("Wins = %d, want 6559", ps.Wins)
	}
	if ps.Losses != 2100 {
		t.Errorf("Losses = %d, want 2100", ps.Losses)
	}
	if ps.Draws != 934 {
		t.Errorf("Draws = %d, want 934", ps.Draws)
	}
	if ps.Rank != 1 {
		t.Errorf("Rank = %d, want 1", ps.Rank)
	}
	if ps.Percentile != 100.0 {
		t.Errorf("Percentile = %f, want 100.0", ps.Percentile)
	}
	if ps.WinStreak != 3 {
		t.Errorf("WinStreak = %d, want 3", ps.WinStreak)
	}
}

func TestGetPuzzleParsesDaily(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakePuzzleJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	p, err := c.GetPuzzle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "DJWZM" {
		t.Errorf("ID = %q, want DJWZM", p.ID)
	}
	if p.Rating != 1765 {
		t.Errorf("Rating = %d, want 1765", p.Rating)
	}
	if p.Plays != 90081 {
		t.Errorf("Plays = %d, want 90081", p.Plays)
	}
	if p.Solution != "d1d7 g7f6 d7b7 b8b7 c7b7" {
		t.Errorf("Solution = %q, want space-joined moves", p.Solution)
	}
	if p.Themes != "crushing,endgame,long" {
		t.Errorf("Themes = %q, want comma-joined themes", p.Themes)
	}
}

func TestGetPuzzleByID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/puzzle/DJWZM") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, fakePuzzleJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	p, err := c.GetPuzzleByID(context.Background(), "DJWZM")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "DJWZM" {
		t.Errorf("ID = %q, want DJWZM", p.ID)
	}
}

func TestGetGamesNDJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprint(w, fakeGamesNDJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	games, err := c.GetGames(context.Background(), "DrNykterstein", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}
	g0 := games[0]
	if g0.ID != "g1" {
		t.Errorf("g0.ID = %q, want g1", g0.ID)
	}
	if g0.White != "DrNykterstein" {
		t.Errorf("g0.White = %q, want DrNykterstein", g0.White)
	}
	if g0.Black != "Opponent" {
		t.Errorf("g0.Black = %q, want Opponent", g0.Black)
	}
	if g0.Winner != "white" {
		t.Errorf("g0.Winner = %q, want white", g0.Winner)
	}
	if g0.Status != "mate" {
		t.Errorf("g0.Status = %q, want mate", g0.Status)
	}
	if g0.Variant != "bullet" {
		t.Errorf("g0.Variant = %q, want bullet", g0.Variant)
	}
	// moves should be first 20 space-separated tokens
	if g0.Moves != "e4 e5 Nf3 Nc6 Bb5 a6" {
		t.Errorf("g0.Moves = %q", g0.Moves)
	}
	if games[1].ID != "g2" {
		t.Errorf("g1.ID = %q, want g2", games[1].ID)
	}
}

func TestGetLeaderboardParsesEntries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeLeaderboardJSON)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	entries, err := c.GetLeaderboard(context.Background(), 5, "blitz")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	e0 := entries[0]
	if e0.Username != "DrNykterstein" {
		t.Errorf("Username = %q", e0.Username)
	}
	if e0.Rating != 3270 {
		t.Errorf("Rating = %d, want 3270", e0.Rating)
	}
	if e0.Title != "GM" {
		t.Errorf("Title = %q, want GM", e0.Title)
	}
}

func TestClientRetriesOn503(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	cfg := lichess.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 2
	c := lichess.NewClient(cfg)
	_, err := c.GetUser(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error on persistent 503")
	}
	// initial attempt + 2 retries = 3 total calls
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}
