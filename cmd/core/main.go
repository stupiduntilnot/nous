package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nous/internal/builtins"
	"nous/internal/core"
	"nous/internal/extension"
	"nous/internal/ipc"
	"nous/internal/provider"
)

func main() {
	socket := flag.String("socket", "/tmp/nous-core.sock", "uds socket path")
	providerName := flag.String("provider", "mock", "provider: mock|openai|gemini")
	model := flag.String("model", "", "provider model name")
	apiBase := flag.String("api-base", "", "optional provider API base URL")
	commandTimeout := flag.Duration("command-timeout", 3*time.Second, "ipc command timeout (e.g. 3s, 500ms)")
	enableDemoExt := flag.Bool("enable-demo-extension", false, "register built-in demo extension command/tool")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p, err := provider.Build(*providerName, *model, *apiBase)
	if err != nil {
		log.Fatalf("provider init failed: %v", err)
	}
	engine := core.NewEngine(core.NewRuntime(), p)
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("resolve cwd failed: %v", err)
	}
	engine.SetTools(builtins.DefaultTools(cwd))
	extMgr := extension.NewManager()
	if *enableDemoExt {
		registerDemoExtension(extMgr)
	}
	engine.SetExtensionManager(extMgr)
	loop := core.NewCommandLoop(engine)

	srv := ipc.NewServer(*socket)
	if err := srv.SetCommandTimeout(*commandTimeout); err != nil {
		log.Fatalf("invalid command timeout: %v", err)
	}
	srv.SetEngine(engine, loop)
	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("core server failed: %v", err)
	}
}

func registerDemoExtension(m *extension.Manager) {
	_ = m.RegisterCommand("echo", func(payload map[string]any) (map[string]any, error) {
		text, _ := payload["text"].(string)
		return map[string]any{"echo": text}, nil
	})
	_ = m.RegisterTool("demo.echo", func(args map[string]any) (string, error) {
		text, _ := args["text"].(string)
		return "demo.echo:" + text, nil
	})
}
