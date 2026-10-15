// Command linksblog runs the links blog.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "time/tzdata" // the container image has no zoneinfo

	"github.com/derand/links_blog/internal/store"
)

const usage = `Usage: linksblog <command> [flags]

Commands:
  serve     run the web site
  migrate   create the database or upgrade its schema

Run "linksblog <command> -h" for the command's flags.
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	switch cmd, args := flag.Arg(0), flag.Args()[1:]; cmd {
	case "serve":
		err = runServe(args)
	case "migrate":
		err = runMigrate(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// dbFlag registers the -db flag shared by all commands.
func dbFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("LINKSBLOG_DB")
	if def == "" {
		def = "data/linksblog.db"
	}
	return fs.String("db", def, "SQLite database path (env LINKSBLOG_DB)")
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := dbFlag(fs)
	fs.Parse(args)

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		return err
	}
	ctx := context.Background()
	s, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer s.Close()
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	slog.Info("database is up to date", "path", *dbPath, "schema_version", v)
	return nil
}
