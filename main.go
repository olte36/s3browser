package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	storageFlag := flag.String("storage", "", "S3-compatible storage URL or alias: aws, gcp")
	credsFlag := flag.String("creds", "raw", "credential source: raw, aws, or gcp")
	accessKeyFlag := flag.String("access-key", "", "access key for -creds raw")
	secretKeyFlag := flag.String("secret-key", "", "secret key for -creds raw")
	sessionTokenFlag := flag.String("session-token", "", "session token for -creds raw")
	flag.Parse()

	endpoint, err := parseEndpoint(*storageFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	auth, err := newCredentialConfig(context.Background(), *credsFlag, *accessKeyFlag, *secretKeyFlag, *sessionTokenFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	service, err := newMinioService(endpoint, auth)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	program := tea.NewProgram(newModel(service), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
