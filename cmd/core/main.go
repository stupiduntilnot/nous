package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"oh-my-agent/internal/ipc"
)

func main() {
	socket := flag.String("socket", "/tmp/pi-core.sock", "uds socket path")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := ipc.NewServer(*socket)
	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("core server failed: %v", err)
	}
}
