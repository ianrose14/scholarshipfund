// Command apply is the "manage my application" web service for the
// Dr. Allison Rose Memorial Fund scholarship. See README.md.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the container image has no zoneinfo
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		// use the field names Cloud Logging understands
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				a.Key = "severity"
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	})))

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("bad configuration", "err", err)
		os.Exit(1)
	}

	var fs FileStorage
	if cfg.DropboxRefreshToken != "" {
		fs = newDropboxStorage(cfg.DropboxAppKey, cfg.DropboxAppSecret, cfg.DropboxRefreshToken)
		slog.Info("using Dropbox storage", "root", cfg.StorageRoot)
	} else {
		fs = &localStorage{base: cfg.LocalDataDir}
		slog.Info("using local storage", "dir", cfg.LocalDataDir, "root", cfg.StorageRoot)
	}
	var mailer Mailer = logMailer{}
	if cfg.ResendAPIKey != "" {
		mailer = newResendMailer(cfg.ResendAPIKey, cfg.EmailFrom, cfg.EmailReplyTo)
	} else {
		slog.Info("RESEND_API_KEY not set; emails will be printed to stdout")
	}

	srv := newServer(cfg, &Store{fs: fs, root: cfg.StorageRoot}, mailer)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", httpServer.Addr, "base_url", cfg.BaseURL)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Cloud Run sends SIGTERM before shutting an instance down.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}
