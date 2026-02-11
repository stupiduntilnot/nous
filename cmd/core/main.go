package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/extension"
	"oh-my-agent/internal/ipc"
	"oh-my-agent/internal/provider"
)

func main() {
	socket := flag.String("socket", "/tmp/pi-core.sock", "uds socket path")
	providerName := flag.String("provider", "mock", "provider: mock|openai|gemini")
	model := flag.String("model", "", "provider model name")
	apiBase := flag.String("api-base", "", "optional provider API base URL")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p, err := provider.Build(*providerName, *model, *apiBase)
	if err != nil {
		log.Fatalf("provider init failed: %v", err)
	}
	engine := core.NewEngine(core.NewRuntime(), p)
	engine.SetExtensionManager(extension.NewManager())
	loop := core.NewCommandLoop(engine)

	srv := ipc.NewServer(*socket)
	srv.SetEngine(engine, loop)
	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("core server failed: %v", err)
	}
}
