package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/snonux/loadbars/internal/collector"
	"github.com/snonux/loadbars/internal/config"
	"github.com/snonux/loadbars/internal/display"
)

// Run starts the loadbars application: collectors and display.
// It blocks until the user quits (e.g. 'q' key).
func Run(cfg *config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewStore()

	var wg sync.WaitGroup
	for _, host := range cfg.Hosts {
		h := host
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCollectorLoop(ctx, h, cfg, store)
		}()
	}

	err := display.Run(ctx, cfg, store)
	cancel()
	wg.Wait()
	return err
}

func runCollectorLoop(ctx context.Context, host string, cfg *config.Config, store *Store) {
	backoff := time.Second
	for {
		err := collector.Run(ctx, host, cfg, store)
		if err == nil || ctx.Err() != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "!!! collector %s failed: %v\n", host, err)
		if !isRemoteHost(host) {
			return
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func isRemoteHost(host string) bool {
	host = strings.TrimSpace(host)
	if i := strings.Index(host, ":"); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	return host != "localhost" && host != "127.0.0.1"
}
