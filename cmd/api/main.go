package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/realtime-notify/internal/auth"
	"github.com/yourname/realtime-notify/internal/config"
	appmw "github.com/yourname/realtime-notify/internal/middleware"
	"github.com/yourname/realtime-notify/internal/pubsub"
	"github.com/yourname/realtime-notify/internal/transport"

	"github.com/yourname/realtime-notify/internal/hub"
	"github.com/yourname/realtime-notify/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	var (
		redisClient *redis.Client
		ps          pubsub.PubSub
	)

	switch cfg.PubSubDriver {
	case "memory":
		// Single-instance / local-dev / test mode: no Redis dependency,
		// but reconnect replay is unavailable (it needs a Redis Stream)
		// and this cannot fan out across more than one process. Config
		// already refuses this in production -- see internal/config.
		slog.Warn("running with PUBSUB_DRIVER=memory: single instance only, no replay, dev/test use only")
		ps = pubsub.NewInMemory()
	default:
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("invalid REDIS_URL", "err", err)
			os.Exit(1)
		}
		redisClient = redis.NewClient(opt)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if redisClient != nil {
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slog.Error("cannot reach redis", "err", err)
			os.Exit(1)
		}
		ps = pubsub.NewRedis(redisClient)
	}

	h := hub.New(ps)
	verifier := auth.NewVerifier(cfg.JWTSecret)
	m := metrics.New(h)

	wsHandler := transport.NewWSHandler(h, ps, verifier, redisClient, cfg.AllowedOrigins)
	sseHandler := transport.NewSSEHandler(h, ps, verifier)
	publishHandler := transport.NewPublishHandler(h)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if redisClient != nil {
			if err := redisClient.Ping(r.Context()).Err(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("redis unavailable"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})
	r.Get("/metrics", m.Handler())

	r.Get("/ws", wsHandler.ServeHTTP)
	r.Get("/sse", sseHandler.ServeHTTP)

	// Serves demo/index.html at /demo -- a zero-build browser client for
	// manually proving pub/sub fan-out works. Not meant for production;
	// harmless to leave mounted since it requires a valid token to do
	// anything.
	r.Handle("/demo", http.RedirectHandler("/demo/", http.StatusMovedPermanently))
	r.Handle("/demo/*", http.StripPrefix("/demo/", http.FileServer(http.Dir("./demo"))))

	r.Route("/api/v1", func(r chi.Router) {
		r.With(appmw.RequirePublishKey(cfg.PublishAPIKey)).Post("/publish", publishHandler.ServeHTTP)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // long-lived WS/SSE connections must not be cut off by a fixed write timeout
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
	_ = ps.Close()
	slog.Info("server stopped")
}
