package supervise

import (
	"log/slog"
	"time"
)

const restartBackoff = 2 * time.Second

func Go(name string, fn func()) {
	go run(name, fn)
}

func run(name string, fn func()) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("supervised goroutine panicked", "goroutine", name, "panic", r, "restart_in", restartBackoff)
				}
			}()
			fn()
		}()
		time.Sleep(restartBackoff)
	}
}
