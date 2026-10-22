package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/neko233-com/metrics233-server/internal/hostagent"
)

var version = "0.1.0"

func main() {
	flags := flag.NewFlagSet("metrics233-collector", flag.ContinueOnError)
	config := flags.String("config", "", "protected bootstrap JSON path")
	once := flags.Bool("once", false, "collect and report once; no background process")
	showVersion := flags.Bool("version", false, "print collector version")
	if flags.Parse(os.Args[1:]) != nil {
		os.Exit(2)
	}
	if *showVersion {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"version": version})
		return
	}
	if *config == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: metrics233-collector --config /protected/bootstrap.json [--once]")
		os.Exit(2)
	}
	cfg, err := hostagent.ReadBootstrap(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	agent, err := hostagent.New(cfg, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = agent.Run(ctx, *once, func(state string) { _ = json.NewEncoder(os.Stdout).Encode(map[string]string{"state": state}) }); err != nil {
		fmt.Fprintln(os.Stderr, "collector cycle failed; inspect host status")
		os.Exit(1)
	}
}
