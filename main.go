package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"xbot/config"
	"xbot/server"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
		cancel()
	}()

	fmt.Println("🤖 xbot is starting...")
	srv := server.New(cfg)
	if err := srv.Run(ctx, nil); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
}
