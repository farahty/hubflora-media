package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/handler"
	"github.com/farahty/hubflora-media/internal/middleware"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/queue"
	"github.com/farahty/hubflora-media/internal/repository"
	"github.com/farahty/hubflora-media/internal/storage"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize S3 client
	s3Client, err := storage.NewS3Client(cfg)
	if err != nil {
		slog.Error("failed to create S3 client", "error", err)
		os.Exit(1)
	}

	// Ensure default bucket exists
	ctx := context.Background()
	if err := s3Client.EnsureBucket(ctx); err != nil {
		slog.Warn("failed to ensure bucket", "error", err)
	}

	// Initialize image processor
	proc := processing.NewProcessor()

	// Initialize PostgreSQL connection pool
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Verify DB connection
	if err := dbPool.Ping(ctx); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected")

	// Initialize repositories
	mediaRepo := repository.NewMediaRepository(dbPool)
	variantRepo := repository.NewVariantRepository(dbPool)

	// Initialize Redis/asynq
	redisOpt, redisErr := queue.ParseRedisURL(cfg.RedisURL)
	var asynqClient *asynq.Client
	var asynqInspector *asynq.Inspector
	if redisErr != nil {
		slog.Warn("redis not configured, async processing disabled", "error", redisErr)
	} else {
		asynqClient = asynq.NewClient(redisOpt)
		defer asynqClient.Close()

		asynqInspector = asynq.NewInspector(redisOpt)
		defer asynqInspector.Close()
	}

	// Start asynq worker server (for consuming tasks)
	var asynqServer *asynq.Server
	if redisErr == nil {
		asynqServer = asynq.NewServer(redisOpt, asynq.Config{
			Concurrency: 5,
			Queues:      map[string]int{"default": 1},
		})
		mux := asynq.NewServeMux()
		mux.Handle(queue.TypeVariantGenerate, queue.NewVariantHandler(s3Client, proc))
		go func() {
			if err := asynqServer.Start(mux); err != nil {
				slog.Error("asynq server failed", "error", err)
			}
		}()
	}

	// Initialize JWKS cache for JWT validation
	jwksURL := cfg.BetterAuthURL + "/api/auth/jwks"
	jwksCache := middleware.NewJWKSCache(jwksURL, 1*time.Hour)

	// Build router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Media-API-Key", "X-Media-User-Id", "X-Media-Org-Id", "X-Media-Org-Slug"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Showcase page
	r.Get("/", handler.Showcase())

	// Health check (no auth)
	r.Get("/healthz", handler.Health(dbPool))

	// API routes (with auth)
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.DualAuth(cfg.APIKey, jwksCache, cfg.BetterAuthURL))

		// Upload
		uploadLimiter := middleware.NewRateLimiter(30, time.Minute)
		r.With(uploadLimiter.Middleware).Post("/upload", handler.Upload(cfg, s3Client, proc, asynqClient, mediaRepo, variantRepo))
		r.Post("/upload/presigned", handler.PresignedUpload(cfg, s3Client))

		// Crop (replaces original + optionally regenerates variants)
		r.Post("/crop", handler.Crop(cfg, s3Client, proc, asynqClient, mediaRepo, variantRepo))

		// Variant regeneration
		r.Post("/variants", handler.VariantRegenerate(cfg, asynqClient))
		r.Get("/variants/info", handler.VariantsInfo(cfg, s3Client))

		// Delete
		r.Delete("/", handler.Delete(cfg, s3Client, mediaRepo, variantRepo))

		// Presign download
		r.Get("/presign", handler.Presign(cfg, s3Client))

		// Download file
		r.Get("/download/{bucket}/*", handler.Download(cfg, s3Client))

		// Variant redirect
		r.Get("/variant/{bucket}/{variantName}/*", handler.VariantRedirect(cfg, s3Client))

		// Job status (async task polling)
		r.Get("/job/{jobId}", handler.JobStatus(asynqInspector))

		// New Phase 2 endpoints
		r.Get("/list", handler.ListMedia(mediaRepo))
		r.Post("/batch", handler.BatchGetMedia(mediaRepo, variantRepo))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", handler.GetMedia(mediaRepo, variantRepo))
			r.Patch("/", handler.UpdateMedia(mediaRepo))
			r.Get("/variants", handler.GetMediaVariants(variantRepo))
		})
	})

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		slog.Info("starting hubflora-media server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	if asynqServer != nil {
		asynqServer.Shutdown()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
	}

	slog.Info("server stopped")
}
