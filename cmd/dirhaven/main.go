// Command dirhaven serves a local, explicitly scoped filesystem MCP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"

	"github.com/diegolinan/dirhaven/internal/filesystem"
	"github.com/diegolinan/dirhaven/internal/mcpmetrics"
)

const version = "0.1.0-dev"

type rootFlags []string

func (r *rootFlags) String() string { return fmt.Sprint([]string(*r)) }
func (r *rootFlags) Set(s string) error {
	*r = append(*r, s)
	return nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Dirhaven - local-first filesystem MCP server")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  dirhaven serve --allow PATH [--allow PATH ...]")
	fmt.Fprintln(w, "  dirhaven serve --allow-all-drives          (Windows only, broad access)")
	fmt.Fprintln(w, "  dirhaven version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options for serve:")
	fmt.Fprintln(w, "  --allow PATH         Allow this existing directory, repeatable")
	fmt.Fprintln(w, "  --allow-all-drives   Allow all currently accessible Windows drive letters")
	fmt.Fprintln(w, "  --metrics-file PATH  Optional private JSONL metrics file")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "This server uses stdio; configure your MCP host to launch it.")
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("subcommand required")
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "dirhaven %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return nil
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var roots rootFlags
		fs.Var(&roots, "allow", "explicitly authorized directory (repeatable)")
		allDrives := fs.Bool("allow-all-drives", false, "authorize all Windows drive letters, including future mounts")
		metricsPath := fs.String("metrics-file", "", "optional private JSONL metrics file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if len(fs.Args()) != 0 {
			return errors.New("unexpected positional arguments")
		}
		if *allDrives {
			roots = append(roots, "*")
		}
		if len(roots) == 0 {
			return errors.New("at least one --allow PATH or --allow-all-drives is required")
		}
		service, err := filesystem.New(roots)
		if err != nil {
			return err
		}
		if *metricsPath != "" {
			metrics, err := mcpmetrics.Open(*metricsPath)
			if err != nil {
				return fmt.Errorf("cannot open metrics file: %w", err)
			}
			defer metrics.Close()
			service.SetMetrics(metrics)
		}
		return service.Serve(ctx)
	default:
		usage(stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "dirhaven:", err)
		os.Exit(1)
	}
}
