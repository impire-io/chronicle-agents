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
	"os"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/runner"
	"github.com/impire-io/chronicle-agents/schedule"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/cli"
	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
)

const usage = `chronicle-agents — humans and agents collaborating in rooms kept on chronicle

  chronicle-agents store init [--store rooms] [--ticks ticks]      the workspace: the rooms store with the room and message types, and the tick store (again to revise)
  chronicle-agents schedule create NAME (--every D | --cron EXPR) [--ttl D] [--store ticks]
                                                                  register a schedule: the server writes a tick under schedule/NAME/ticks at every firing
  chronicle-agents run FILE                                       serve one agent from its definition file: one process, one declaration, one credential

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

// storeInit makes the workspace: the rooms store with its two types, and
// the tick store that allows schedules. Both are created when missing and
// left alone when present; the types are declared again (a revision).
func storeInit(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("store init", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	store := fs.String("store", declaration.DefaultStore, "the rooms store")
	ticks := fs.String("ticks", declaration.DefaultTickStore, "the tick store; empty for none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := f.resolve("")
	if err != nil {
		return err
	}
	defer c.Close()
	if created, err := ensureStore(ctx, c, *store, "rooms where humans and agents collaborate (chronicle-agents)"); err != nil {
		return err
	} else if created {
		fmt.Fprintf(out, "store %s created\n", *store)
	}
	if err := types.Declare(ctx, c, *store); err != nil {
		return err
	}
	fmt.Fprintf(out, "types %s and %s declared in store %s (as %s)\n", types.Room, types.Message, *store, c.Author())
	if *ticks != "" {
		if created, err := ensureStore(ctx, c, *ticks, "the tick store: schedules and their ticks (chronicle-agents)", client.WithSchedules()); err != nil {
			return err
		} else if created {
			fmt.Fprintf(out, "store %s created (schedules allowed)\n", *ticks)
		} else {
			fmt.Fprintf(out, "store %s present\n", *ticks)
		}
	}
	return nil
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
	r, err := runner.New(c, decl, log)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s serving from %s (harness %s)\n", decl.Identity, file, harnessName(decl))
	return r.Serve(ctx)
}

func harnessName(d declaration.Declaration) string {
	if d.Harness.Preset != "" {
		return d.Harness.Preset
	}
	return d.Harness.Template.Command
}
