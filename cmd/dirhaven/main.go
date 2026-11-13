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
	"github.com/diegolinan/dirhaven/internal/setupwizard"
)

const version = "0.2.0-dev"

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
	fmt.Fprintln(w, "  dirhaven serve --allow-all                 (all OSes, broad access)")
	fmt.Fprintln(w, "  dirhaven serve --allow-all-drives          (Windows legacy alias)")
	fmt.Fprintln(w, "  dirhaven setup                             (local setup wizard preview)")
	fmt.Fprintln(w, "  dirhaven version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options for serve:")
	fmt.Fprintln(w, "  --allow PATH         Allow this existing directory, repeatable on every OS")
	fmt.Fprintln(w, "  --allow-all          Allow every filesystem path accessible to this process")
	fmt.Fprintln(w, "  --allow-all-drives   Legacy Windows-only all-drive-letter discovery")
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
	case "setup":
		if len(args) != 1 {
			return errors.New("setup does not accept arguments")
		}
		return setupwizard.Start(ctx, stdout)
	case "serve":
		roots, metricsPath, err := parseServeOptions(args[1:], stderr)
		if err != nil {
			return err
		}
		service, err := filesystem.New(roots)
		if err != nil {
			return err
		}
		if metricsPath != "" {
			metrics, err := mcpmetrics.Open(metricsPath)
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

// parseServeOptions keeps the root-authorization policy portable: on Windows
// broad access discovers current and future drive letters dynamically;
// macOS and Linux use the filesystem root, which includes their mount points.
// Explicit --allow paths work on every supported OS and never assume a drive.
func parseServeOptions(args []string, stderr io.Writer) ([]string, string, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var roots rootFlags
	fs.Var(&roots, "allow", "explicitly authorized directory (repeatable)")
	all := fs.Bool("allow-all", false, "authorize all accessible filesystem paths on this OS")
	allDrives := fs.Bool("allow-all-drives", false, "legacy Windows-only drive-letter discovery")
	metricsPath := fs.String("metrics-file", "", "optional private JSONL metrics file")
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	if len(fs.Args()) != 0 {
		return nil, "", errors.New("unexpected positional arguments")
	}
	if *all {
		if runtime.GOOS == "windows" {
			roots = append(roots, "*")
		} else {
			roots = append(roots, string(os.PathSeparator))
		}
	}
	if *allDrives {
		roots = append(roots, "*")
	}
	if len(roots) == 0 {
		return nil, "", errors.New("at least one --allow PATH, --allow-all or --allow-all-drives is required")
	}
	return []string(roots), *metricsPath, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "dirhaven:", err)
		os.Exit(1)
	}
}
