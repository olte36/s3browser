package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// main starts the terminal browser and reports startup or runtime errors.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses configuration, creates the storage service, and runs the UI.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	storageFlag := flag.String("storage", "", "S3-compatible storage URL or alias: aws, gcp")
	credsFlag := flag.String("creds", "raw", "credential source: raw, aws, or gcp")
	accessKeyFlag := flag.String("access-key", "", "access key for -creds raw")
	secretKeyFlag := flag.String("secret-key", "", "secret key for -creds raw")
	sessionTokenFlag := flag.String("session-token", "", "session token for -creds raw")
	flag.Parse()

	endpoint, err := parseEndpoint(*storageFlag)
	if err != nil {
		return err
	}

	auth, err := newCredentialConfig(ctx, *credsFlag, *accessKeyFlag, *secretKeyFlag, *sessionTokenFlag)
	if err != nil {
		return err
	}

	service, err := newMinioService(endpoint, auth)
	if err != nil {
		return err
	}

	program := tea.NewProgram(newModel(ctx, endpoint.Display, service), tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
			return nil
		}
		return err
	}
	return nil
}
