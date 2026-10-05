package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/LoggeL/loggerbin/internal/config"
	"github.com/LoggeL/loggerbin/internal/server"
	"github.com/LoggeL/loggerbin/internal/store"
)

var version = "1.0.0-dev"

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version":
			fmt.Println("Loggerbin " + version)
			return nil
		case "healthcheck":
			port := os.Getenv("LOGGERBIN_PORT")
			if port == "" {
				port = "8080"
			}
			if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
				return errors.New("invalid healthcheck port")
			}
			client := &http.Client{Timeout: 3 * time.Second}
			r, err := client.Get("http://127.0.0.1:" + port + "/healthz")
			if err != nil {
				return errors.New("healthcheck failed")
			}
			defer r.Body.Close()
			if r.StatusCode != 200 {
				return errors.New("healthcheck failed")
			}
			return nil
		default:
			return errors.New("usage: loggerbin [--version|healthcheck]")
		}
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gcDone := make(chan struct{})
	go func() { defer close(gcDone); db.Sweep(ctx, logger) }()
	app := server.New(cfg, db, logger, version).HTTP()
	failure := make(chan error, 1)
	go func() {
		logger.Info("Loggerbin started", "address", cfg.Address(), "version", version)
		failure <- app.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
	case err := <-failure:
		stop()
		<-gcDone
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = app.Shutdown(shutdown)
	if err != nil {
		app.Close()
	}
	<-gcDone
	logger.Info("Loggerbin stopped")
	return err
}
func main() {
	if err := run(); err != nil {
		slog.Error("Loggerbin failed", "error", err)
		os.Exit(1)
	}
}
