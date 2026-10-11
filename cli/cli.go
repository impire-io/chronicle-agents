// Package cli is the chronicle-agents command line: two sentences — store
// init and run — and the connection flags chronicle's own CLI uses.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/runner"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/cli"
	"github.com/impire-io/chronicle/client"
)

const usage = `chronicle-agents — humans and agents collaborating in rooms kept on chronicle

  chronicle-agents store init [--store NAME]   declare the room and message types in a store (again to revise)
  chronicle-agents run FILE                    serve one agent from its definition file: one process, one declaration, one credential

Connection flags, on every sentence (default: the chronicle context "local"):
  --context NAME     a saved chronicle context (chronicle context list)
  --url URL          the NATS url
  --nkey FILE        an nkey seed — a user on your own NATS; the principal is the context's, --principal, or the agent's identity
  --creds FILE       a credential file; the principal is the one it names
  --principal NAME   who you are, with --nkey
  --store NAME       the store (store init); the agent's triggers name theirs
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
	context, url, nkey, creds, principal, store string
}

func (f *connFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.context, "context", "", "a saved chronicle context")
	fs.StringVar(&f.url, "url", "", "the NATS url")
	fs.StringVar(&f.nkey, "nkey", "", "an nkey seed file")
	fs.StringVar(&f.creds, "creds", "", "a credential file")
	fs.StringVar(&f.principal, "principal", "", "the principal, with --nkey")
	fs.StringVar(&f.store, "store", "", "the store")
}

// resolve turns the flags into a connection: explicit flags win, a named
// context next, the context "local" last. The author is the principal the
// flags or the context name, or the one the caller insists on.
func (f *connFlags) resolve(wantAuthor string) (*client.Client, string, error) {
	url, nkey, creds, principal, store := f.url, f.nkey, f.creds, f.principal, f.store
	if url == "" && nkey == "" && creds == "" {
		name := f.context
		if name == "" {
			name = cli.LocalContextName
		}
		c, ok, err := cli.LoadContext(name)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "", fmt.Errorf("no context %q: pass --url with --nkey or --creds, or save one with chronicle context add", name)
		}
		url, nkey, creds = c.URL, c.Nkey, c.Creds
		if principal == "" {
			principal = c.Principal
		}
		if store == "" {
			store = c.Store
		}
	}
	if url == "" {
		return nil, "", errors.New("no url: pass --url, or a context that has one")
	}
	if wantAuthor != "" {
		principal = wantAuthor
	}
	switch {
	case creds != "":
		c, err := client.ConnectFile(url, creds)
		if err != nil {
			return nil, "", err
		}
		return c, store, nil
	case nkey != "":
		if principal == "" {
			return nil, "", errors.New("--nkey needs a principal: --principal, or the context's")
		}
		c, err := client.ConnectNkeyFile(url, nkey, principal)
		if err != nil {
			return nil, "", err
		}
		return c, store, nil
	default:
		return nil, "", errors.New("no credential: pass --nkey or --creds, or a context that has one")
	}
}

func storeInit(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("store init", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, store, err := f.resolve("")
	if err != nil {
		return err
	}
	defer c.Close()
	if store == "" {
		store = declaration.DefaultStore
	}
	if err := types.Declare(ctx, c, store); err != nil {
		return err
	}
	fmt.Fprintf(out, "types %s and %s declared in store %s (as %s)\n", types.Room, types.Message, store, c.Author())
	return nil
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(out)
	var f connFlags
	f.bind(fs)
	// The file may come before or after the flags: parse past it.
	var file string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		if file != "" {
			return errors.New("run: one definition file")
		}
		file, rest = fs.Arg(0), fs.Args()[1:]
	}
	if file == "" {
		return errors.New("run: one definition file")
	}
	decl, err := declaration.Load(file)
	if err != nil {
		return err
	}
	c, _, err := f.resolve(decl.Identity)
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
