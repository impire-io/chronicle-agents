// Package cli is the chronicle-agents command line: the workspace's
// stores, a schedule, an agent — and the connection flags chronicle's own
// CLI uses.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/mcpserver"
	"github.com/impire-io/chronicle-agents/runner"
	"github.com/impire-io/chronicle-agents/schedule"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle-agents/web"
	"github.com/impire-io/chronicle/bridge"
	"github.com/impire-io/chronicle/cli"
	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
	"github.com/impire-io/chronicle/devdir"
)

const usage = `chronicle-agents — humans and agents collaborating in rooms kept on chronicle

  chronicle-agents store init [--store rooms] [--ticks ticks] [--interface URL]
                                                                  the workspace: the rooms store with the room and message types, and the tick store (again to revise);
                                                                  --interface records where the rooms interface is (hosted: https://agents.<the site's host> by default)
  chronicle-agents schedule create NAME (--every D | --cron EXPR) [--ttl D] [--store ticks]
                                                                  register a schedule: the server writes a tick under schedule/NAME/ticks at every firing
  chronicle-agents run FILE                                       serve one agent from its definition file: one process, one declaration, one credential
  chronicle-agents mcp                                            the MCP server the runner starts for one turn (configured by CHRONICLE_AGENTS_* in the environment); not for people
  chronicle-agents web [--listen 127.0.0.1:8080] [--ws URL] [--public-url URL] [--hosted [--account NAME]]
                                                                  serve the rooms interface: the embedded application, and how the browser reaches chronicle;
                                                                  --hosted signs the person in with GitHub, --account pins the account (otherwise they choose)

Connection flags, on every sentence (default: the chronicle context "local"):
  --context NAME     a saved chronicle context (chronicle context list)
  --url URL          the NATS url
  --nkey FILE        an nkey seed — a user on your own NATS; the principal is the context's, --principal, or the agent's identity
  --creds FILE       a credential file; the principal is the one it names
  --principal NAME   who you are, with --nkey
`

// Run executes one command line.
func Run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return nil
	}
	switch args[0] {
	case "store":
		if len(args) < 2 || args[1] != "init" {
			fmt.Fprint(out, usage)
			return errors.New("store: the verb is init")
		}
		return storeInit(ctx, args[2:], out)
	case "schedule":
		if len(args) < 2 || args[1] != "create" {
			fmt.Fprint(out, usage)
			return errors.New("schedule: the verb is create")
		}
		return scheduleCreate(ctx, args[2:], out)
	case "run":
		return run(ctx, args[1:], out)
	case "mcp":
		return mcpCmd(ctx)
	case "web":
		return webCmd(ctx, args[1:], out)
	case "version":
		fmt.Fprintln(out, Version)
		return nil
	default:
		fmt.Fprint(out, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// Version is stamped by the build; the default names a source build.
var Version = "0.0.0-dev"

type connFlags struct {
	context, url, nkey, creds, principal string
}

func (f *connFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.context, "context", "", "a saved chronicle context")
	fs.StringVar(&f.url, "url", "", "the NATS url")
	fs.StringVar(&f.nkey, "nkey", "", "an nkey seed file")
	fs.StringVar(&f.creds, "creds", "", "a credential file")
	fs.StringVar(&f.principal, "principal", "", "the principal, with --nkey")
}

// resolve turns the flags into a connection: explicit flags win, a named
// context next, the context "local" last. The author is the principal the
// flags or the context name, or the one the caller insists on.
func (f *connFlags) resolve(wantAuthor string) (*client.Client, error) {
	url, nkey, creds, principal := f.url, f.nkey, f.creds, f.principal
	if url == "" && nkey == "" && creds == "" {
		name := f.context
		if name == "" {
			name = cli.LocalContextName
		}
		c, ok, err := cli.LoadContext(name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("no context %q: pass --url with --nkey or --creds, or save one with chronicle context add", name)
		}
		if c.Bridge != "" && c.Nkey == "" && c.Creds == "" {
			// A sign-in context: chronicle's own CLI dials it through the
			// bridge with the login it keeps; chronicle-agents has no way
			// to yet, so it says which credential to use instead.
			return nil, fmt.Errorf("context %q signs in with GitHub, which chronicle-agents cannot speak through yet: use a service account's credential (chronicle service-account create NAME --role admin, then --creds FILE), or --nkey on your own NATS", name)
		}
		url, nkey, creds = c.URL, c.Nkey, c.Creds
		if principal == "" {
			principal = c.Principal
		}
	}
	if url == "" {
		return nil, errors.New("no url: pass --url, or a context that has one")
	}
	if wantAuthor != "" {
		principal = wantAuthor
	}
	switch {
	case creds != "":
		return client.ConnectFile(url, creds)
	case nkey != "":
		if principal == "" {
			return nil, errors.New("--nkey needs a principal: --principal, or the context's")
		}
		return client.ConnectNkeyFile(url, nkey, principal)
	default:
		return nil, errors.New("no credential: pass --nkey or --creds, or a context that has one")
	}
}

// parseWithPositional parses flags that may come before or after one
// positional argument.
func parseWithPositional(fs *flag.FlagSet, args []string, what string) (string, error) {
	var pos string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return "", err
		}
		if fs.NArg() == 0 {
			break
		}
		if pos != "" {
			return "", fmt.Errorf("%s: one %s", fs.Name(), what)
		}
		pos, rest = fs.Arg(0), fs.Args()[1:]
	}
	if pos == "" {
		return "", fmt.Errorf("%s: one %s", fs.Name(), what)
	}
	return pos, nil
}

// storeInit makes the workspace from its declaration (decision 0008 —
// the same value the hosted interface publishes as /install.json): the
// rooms store with its types, the announcement, and the tick store that
// allows schedules. Stores are created when missing and left alone when
// present; the types are declared again (a revision).
func storeInit(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("store init", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	store := fs.String("store", declaration.DefaultStore, "the rooms store")
	ticks := fs.String("ticks", declaration.DefaultTickStore, "the tick store; empty for none")
	iface := fs.String("interface", "", "where the rooms interface is, recorded on the announcement (default: https://agents.<the site's host> with a sign-in context; nothing otherwise, and web records it when it serves)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *iface == "" {
		*iface = f.defaultInterface()
	}
	c, err := f.resolve("")
	if err != nil {
		return err
	}
	defer c.Close()
	w := types.Declaration(*store, *ticks)
	for _, s := range w.Stores {
		var opts []client.StoreOpt
		if s.Schedules {
			opts = append(opts, client.WithSchedules())
		}
		created, err := ensureStore(ctx, c, s.Name, s.Description, opts...)
		if err != nil {
			return err
		}
		switch {
		case created && s.Schedules:
			fmt.Fprintf(out, "store %s created (schedules allowed)\n", s.Name)
		case created:
			fmt.Fprintf(out, "store %s created\n", s.Name)
		case len(s.Types) == 0:
			fmt.Fprintf(out, "store %s present\n", s.Name)
		}
		if err := s.DefineTypes(ctx, c); err != nil {
			return err
		}
		if names := w.WorkspaceTypes(s.Name); len(names) > 0 {
			fmt.Fprintf(out, "types %s declared in store %s (as %s)\n", spokenList(names), s.Name, c.Author())
		}
		if s.Name != w.Announce.Store {
			continue
		}
		// Every workspace starts with General (decision 0009): a place to
		// type from the first second.
		created, err = types.EnsureGeneral(ctx, c, s.Name)
		if err != nil {
			return err
		}
		if created {
			fmt.Fprintf(out, "room %s created in store %s\n", types.GeneralID, s.Name)
		}
		// The workspace announces itself (chronicle-hq decision 0051): the
		// console lists it and links to its interface.
		if err := types.AnnounceInstance(ctx, c, s.Name, Version); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s announced as %s in store %s\n", types.SolutionName, w.Announce.Path, s.Name)
		if *iface != "" {
			if err := types.Locate(ctx, c, s.Name, *iface); err != nil {
				return err
			}
			fmt.Fprintf(out, "the rooms interface is at %s (recorded on the announcement)\n", *iface)
		}
	}
	return nil
}

// defaultInterface is where the rooms interface is when the flags name no
// address: with a sign-in context, the hosted interface beside the site
// the context's install profile came from (design 03 § serving it);
// nothing with a credential, where web records the address when it serves.
func (f *connFlags) defaultInterface() string {
	if f.url != "" || f.nkey != "" || f.creds != "" {
		return ""
	}
	name := f.context
	if name == "" {
		name = cli.LocalContextName
	}
	c, ok, err := cli.LoadContext(name)
	if err != nil || !ok || c.Bridge == "" {
		return ""
	}
	data, err := os.ReadFile(c.Bridge)
	if err != nil {
		return ""
	}
	p, err := bridge.DecodeProfile(data, c.Bridge)
	if err != nil {
		return ""
	}
	return InterfaceFor(p)
}

// InterfaceFor derives the hosted interface's address from an install
// profile: `agents.` before the host of the site the profile names — the
// site is where the web flow's code is traded — and nothing when the
// profile names no site.
func InterfaceFor(p bridge.Profile) string {
	u, err := url.Parse(p.GithubTokenURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return "https://agents." + u.Host
}

// ensureStore creates a store unless it exists; it reports whether it did.
func ensureStore(ctx context.Context, c *client.Client, store, description string, opts ...client.StoreOpt) (bool, error) {
	_, err := c.CreateStore(ctx, store, description, opts...)
	var se *client.ServiceError
	if errors.As(err, &se) && se.Code == contract.CodeStoreExists {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create store %s: %w", store, err)
	}
	return true, nil
}

func scheduleCreate(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("schedule create", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	every := fs.Duration("every", 0, "fire every interval (at least 1s)")
	cron := fs.String("cron", "", "fire on a cron expression")
	ttl := fs.Duration("ttl", 0, "ticks older than this expire on the server (0: they accumulate)")
	store := fs.String("store", declaration.DefaultTickStore, "the tick store")
	name, err := parseWithPositional(fs, args, "schedule name")
	if err != nil {
		return err
	}
	var pattern string
	switch {
	case *every > 0 && *cron != "":
		return errors.New("--every or --cron, not both")
	case *every > 0:
		if *every < time.Second {
			return errors.New("--every: at least 1s")
		}
		pattern = schedule.Every(*every)
	case *cron != "":
		pattern = *cron
	default:
		return errors.New("--every D or --cron EXPR is required")
	}
	c, err := f.resolve("")
	if err != nil {
		return err
	}
	defer c.Close()
	if err := schedule.Create(ctx, c.Conn(), c.Author(), *store, name, pattern, *ttl); err != nil {
		return err
	}
	fmt.Fprintf(out, "schedule %s registered in %s (%s): ticks land under %s\n", name, *store, pattern, schedule.TickPath(name))
	return nil
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	file, err := parseWithPositional(fs, args, "definition file")
	if err != nil {
		return err
	}
	decl, err := declaration.Load(file)
	if err != nil {
		return err
	}
	c, err := f.resolve(decl.Identity)
	if err != nil {
		return err
	}
	defer c.Close()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	url, nkey, creds, _, err := f.connection()
	if err != nil {
		return err
	}
	r, err := runner.New(c, decl, log, runner.Version(Version), runner.WithConnection(runner.Connection{URL: url, Nkey: nkey, Creds: creds}))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s serving from %s (harness %s)\n", decl.Identity, file, harnessName(decl))
	return r.Serve(ctx)
}

// mcpCmd serves the runner's MCP server for one turn, over standard input
// and output, configured by the environment the runner wrote into the
// harness's MCP configuration (the mcpserver package).
func mcpCmd(ctx context.Context) error {
	cfg, err := mcpserver.ConfigFromEnv()
	if err != nil {
		return err
	}
	return mcpserver.Serve(ctx, cfg)
}

// webCmd serves the rooms interface (chronicle-agents-hq design 03): the
// embedded application and /config.json — how the browser reaches
// chronicle. Loopback by default: the local form hands the page the
// context's credential in the clear, as the quick start itself does.
// Hosted, the page signs the person in with GitHub (decision 0007); the
// account is a pin the operator may set, and the deployment serves no
// account's context, so it announces nothing — store init does.
func webCmd(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	listen := fs.String("listen", "127.0.0.1:8080", "the address to serve on")
	ws := fs.String("ws", "", "the websocket URL the browser connects to (default: the quick start's recorded one, beside the context's nkey)")
	publicURL := fs.String("public-url", "", "the address the interface is reachable at, recorded on the announcement (default: http://<listen>)")
	hosted := fs.Bool("hosted", false, "the hosted form: the browser signs in with GitHub through the install's identity bridge")
	profile := fs.String("profile", bridgeProfileURL, "the install profile (hosted)")
	account := fs.String("account", "", "pin the interface to one account (hosted); otherwise the person chooses among the accounts they hold")
	store := fs.String("store", declaration.DefaultStore, "the rooms store")
	ticks := fs.String("ticks", declaration.DefaultTickStore, "the tick store")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// The address the interface is reachable at: on the announcement when
	// web serves from a context, and on /install.json always.
	addr := *publicURL
	if addr == "" {
		addr = "http://" + *listen
	}
	cfg := web.Config{Store: *store, TickStore: *ticks, Version: Version, PublicURL: addr}
	if *hosted {
		cfg.Mode, cfg.Profile, cfg.Account = "hosted", *profile, *account
		if *account == "" {
			fmt.Fprintln(out, "hosted: the person signs in with GitHub and chooses among the accounts they hold")
		} else {
			fmt.Fprintf(out, "hosted: the person signs in with GitHub; the interface is pinned to account %s\n", *account)
		}
		fmt.Fprintln(out, "the announcement is not updated from here: store init --interface records where this interface is")
		if !web.Built() {
			fmt.Fprintln(out, "note: this binary carries no built interface (run make web); the page will say so")
		}
		fmt.Fprintf(out, "rooms interface at http://%s (%s; store %s)\n", *listen, cfg.Mode, cfg.Store)
		return web.Serve(ctx, *listen, cfg)
	}
	{
		url, nkey, creds, principal, err := f.connection()
		if err != nil {
			return err
		}
		cfg.Mode, cfg.Author = "local", principal
		switch {
		case creds != "":
			b, err := os.ReadFile(creds)
			if err != nil {
				return fmt.Errorf("read credential: %w", err)
			}
			cfg.Creds = string(b)
		case nkey != "":
			b, err := os.ReadFile(nkey)
			if err != nil {
				return fmt.Errorf("read nkey: %w", err)
			}
			cfg.NkeySeed = string(b)
			// The quick start's one user has no name of its own: the page asks.
			if cfg.Author == devdir.LocalPrincipal {
				cfg.Author = ""
			}
		default:
			return errors.New("no credential to hand the browser: the context needs --nkey or --creds")
		}
		cfg.Servers = *ws
		if cfg.Servers == "" && nkey != "" {
			if b, err := os.ReadFile(filepath.Join(filepath.Dir(nkey), devdir.WebsocketURLFile)); err == nil {
				cfg.Servers = strings.TrimSpace(string(b))
			}
		}
		if cfg.Servers == "" {
			return fmt.Errorf("no websocket URL: pass --ws, or run chronicle up --websocket-port 9222 (the NATS url is %s; a browser speaks NATS over a websocket only)", url)
		}
	}
	// The announcement learns where the interface is.
	if c, err := f.resolve(""); err == nil {
		if err := types.Locate(ctx, c, *store, addr); err != nil {
			fmt.Fprintf(out, "note: the announcement was not updated: %v\n", err)
		}
		c.Close()
	} else {
		fmt.Fprintf(out, "note: the announcement was not updated: %v\n", err)
	}
	if !web.Built() {
		fmt.Fprintln(out, "note: this binary carries no built interface (run make web); the page will say so")
	}
	fmt.Fprintf(out, "rooms interface at http://%s (%s; store %s)\n", *listen, cfg.Mode, cfg.Store)
	return web.Serve(ctx, *listen, cfg)
}

// bridgeProfileURL is the hosted install's profile (chronicle-js carries the same default).
const bridgeProfileURL = "https://chronicle.impire.dev/.well-known/chronicle/profile.json"

// connection resolves the flags or the context to the raw facts a browser
// needs handed over: url, the nkey path, the creds path, the principal.
func (f *connFlags) connection() (url, nkey, creds, principal string, err error) {
	url, nkey, creds, principal = f.url, f.nkey, f.creds, f.principal
	if url == "" && nkey == "" && creds == "" {
		name := f.context
		if name == "" {
			name = cli.LocalContextName
		}
		c, ok, err := cli.LoadContext(name)
		if err != nil {
			return "", "", "", "", err
		}
		if !ok {
			return "", "", "", "", fmt.Errorf("no context %q: pass --url with --nkey or --creds, or save one with chronicle context add", name)
		}
		url, nkey, creds = c.URL, c.Nkey, c.Creds
		if principal == "" {
			principal = c.Principal
		}
	}
	return url, nkey, creds, principal, nil
}

func harnessName(d declaration.Declaration) string { return d.HarnessName() }

// spokenList joins names as a sentence does: "room, message and agent".
func spokenList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}
