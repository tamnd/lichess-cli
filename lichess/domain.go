package lichess

import (
	"context"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes lichess as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/lichess-cli/lichess"
//
// The init below registers it; the host then dereferences lichess:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone lichess binary (see cli.NewApp), so the binary and a host share
// one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the lichess driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "lichess",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "lichess",
			Short:  "Browse public Lichess chess data from the command line.",
			Long: `A command line for Lichess (lichess.org/api).

lichess reads public chess data over plain HTTPS, shapes it into clean records,
and prints output that pipes into the rest of your tools. No API key, nothing to
run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/lichess-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// user: player profile
	kit.Handle(app, kit.OpMeta{Name: "user", Group: "player", Single: true,
		Summary: "Fetch a player profile", URIType: "user", Resolver: true,
		Args: []kit.Arg{{Name: "username", Help: "Lichess username"}}}, getUser)

	// perf: performance statistics
	kit.Handle(app, kit.OpMeta{Name: "perf", Group: "player", Single: true,
		Summary: "Fetch performance stats for a player",
		Args:    []kit.Arg{{Name: "username", Help: "Lichess username"}}}, getPerfStat)

	// puzzle: daily puzzle or by ID
	kit.Handle(app, kit.OpMeta{Name: "puzzle", Group: "content", Single: true,
		Summary: "Fetch the daily puzzle or a puzzle by ID",
		Args:    []kit.Arg{{Name: "id", Help: "puzzle ID (omit for daily)", Optional: true}}}, getPuzzle)

	// games: recent games (NDJSON stream)
	kit.Handle(app, kit.OpMeta{Name: "games", Group: "content", List: true,
		Summary: "List recent games for a player",
		Args:    []kit.Arg{{Name: "username", Help: "Lichess username"}}}, listGames)

	// tv: current TV game
	kit.Handle(app, kit.OpMeta{Name: "tv", Group: "content", Single: true,
		Summary: "Fetch the current TV game"}, getTV)

	// top: leaderboard
	kit.Handle(app, kit.OpMeta{Name: "top", Group: "content", List: true,
		Summary: "Fetch the leaderboard for a perf type"}, listTop)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient(DefaultConfig())
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.cfg.Timeout = cfg.Timeout
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- input types ---

type usernameIn struct {
	Username string  `kit:"arg" help:"Lichess username"`
	Client   *Client `kit:"inject"`
}

type perfIn struct {
	Username string  `kit:"arg" help:"Lichess username"`
	Type     string  `kit:"flag" help:"perf type: bullet blitz rapid classical"`
	Client   *Client `kit:"inject"`
}

type puzzleIn struct {
	ID     string  `kit:"arg,optional" help:"puzzle ID (omit for daily)"`
	Client *Client `kit:"inject"`
}

type gamesIn struct {
	Username string  `kit:"arg" help:"Lichess username"`
	Limit    int     `kit:"flag,inherit" help:"max games"`
	Variant  string  `kit:"flag" help:"variant filter: bullet blitz rapid"`
	Client   *Client `kit:"inject"`
}

type topIn struct {
	Limit   int     `kit:"flag,inherit" help:"number of players"`
	Variant string  `kit:"flag" help:"variant: bullet blitz rapid classical puzzle"`
	Client  *Client `kit:"inject"`
}

type noArgs struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getUser(ctx context.Context, in usernameIn, emit func(*User) error) error {
	u, err := in.Client.GetUser(ctx, in.Username)
	if err != nil {
		return mapErr(err)
	}
	return emit(u)
}

func getPerfStat(ctx context.Context, in perfIn, emit func(*PerfStat) error) error {
	perfType := in.Type
	if perfType == "" {
		perfType = "blitz"
	}
	ps, err := in.Client.GetPerfStat(ctx, in.Username, perfType)
	if err != nil {
		return mapErr(err)
	}
	return emit(ps)
}

func getPuzzle(ctx context.Context, in puzzleIn, emit func(*Puzzle) error) error {
	var (
		p   *Puzzle
		err error
	)
	id := strings.TrimSpace(in.ID)
	if id == "" || id == "daily" {
		p, err = in.Client.GetPuzzle(ctx)
	} else {
		p, err = in.Client.GetPuzzleByID(ctx, id)
	}
	if err != nil {
		return mapErr(err)
	}
	return emit(p)
}

func listGames(ctx context.Context, in gamesIn, emit func(*Game) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	games, err := in.Client.GetGames(ctx, in.Username, limit, in.Variant)
	if err != nil {
		return mapErr(err)
	}
	for i := range games {
		if err := emit(&games[i]); err != nil {
			return err
		}
	}
	return nil
}

func getTV(ctx context.Context, in noArgs, emit func(*TVGame) error) error {
	tv, err := in.Client.GetTV(ctx)
	if err != nil {
		return mapErr(err)
	}
	return emit(tv)
}

func listTop(ctx context.Context, in topIn, emit func(*TopPlayer) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	variant := in.Variant
	if variant == "" {
		variant = "blitz"
	}
	entries, err := in.Client.GetLeaderboard(ctx, limit, variant)
	if err != nil {
		return mapErr(err)
	}
	for i := range entries {
		if err := emit(&entries[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- URI driver ---

// Classify turns any accepted input into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = pagePath(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized lichess reference: %q", input)
	}
	return "user", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "user":
		return "https://" + Host + "/@/" + strings.Trim(id, "/"), nil
	default:
		return "", errs.Usage("lichess has no resource type %q", uriType)
	}
}

// --- helpers ---

func pagePath(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return strings.Trim(u.Path, "/")
	}
	return strings.Trim(input, "/")
}

func mapErr(err error) error {
	return err
}
