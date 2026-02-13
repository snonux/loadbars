package app

import (
	"context"
	"sync"

	"github.com/loadbars/loadbars/internal/collector"
	"github.com/loadbars/loadbars/internal/config"
	"github.com/loadbars/loadbars/internal/display"
)

// Run starts the loadbars application: collectors and display.
// It blocks until the user quits (e.g. 'q' key).
func Run(cfg *config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scriptPath := ScriptPath()
	store := NewStore()

	var wg sync.WaitGroup
	for _, host := range cfg.Hosts {
		h := host
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = collector.Run(ctx, h, cfg, store, scriptPath)
		}()
	}

	err := display.Run(ctx, cfg, store)
	cancel()
	wg.Wait()
	return err
}
