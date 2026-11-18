// Command migrate applies pending SQL migrations.
// Usage: migrate up | migrate status
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"biteorbit/services/api/internal/migrations"
)

func main() {
	url := os.Getenv("BITEORBIT_DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "BITEORBIT_DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sqldb, err := sql.Open("pgx", url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open database:", err)
		os.Exit(1)
	}
	defer sqldb.Close()
	if err := sqldb.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "database unreachable:", err)
		os.Exit(1)
	}

	runner := &migrations.Runner{DB: sqldb}
	switch cmd := os.Args[1]; cmd {
	case "up":
		applied, pending, err := runner.Status(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "status:", err)
			os.Exit(1)
		}
		fmt.Printf("applied already: %d, pending: %d\n", len(applied), len(pending))
		justApplied, err := runner.Up(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", err)
			os.Exit(1)
		}
		for _, m := range justApplied {
			fmt.Println("applied:", m)
		}
		if len(justApplied) == 0 {
			fmt.Println("database already up to date")
		}
	case "status":
		applied, pending, err := runner.Status(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "status:", err)
			os.Exit(1)
		}
		fmt.Println("applied:")
		for _, m := range applied {
			fmt.Println("  ✔", m)
		}
		fmt.Println("pending:")
		for _, m := range pending {
			fmt.Println("  •", m)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: migrate up|status\n")
		os.Exit(2)
	}
}
