package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	appmatching "github.com/kungrem23/pulse-backend/internal/app/matching"
	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/config"
	"github.com/kungrem23/pulse-backend/internal/platform/database"

	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/security"
	photostorage "github.com/kungrem23/pulse-backend/internal/storage/s3"
	httpapi "github.com/kungrem23/pulse-backend/internal/transport/http"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	limiter, err := middleware.NewRateLimiter(cfg.AuthRateLimit)
	if err != nil {
		logger.Error("invalid auth rate limit configuration")
		os.Exit(1)
	}

	tokens, err := security.NewTokenIssuer(security.TokenIssuerConfig{
		Secret:          cfg.JWTSecret,
		Issuer:          cfg.JWTIssuer,
		AccessTokenTTL:  cfg.AccessTokenTTL,
		RefreshTokenTTL: cfg.RefreshTokenTTL,
	})
	if err != nil {
		logger.Error("invalid JWT configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	photoStore, err := photostorage.New(photostorage.Config{
		Endpoint:       cfg.PhotoStorage.Endpoint,
		PublicEndpoint: cfg.PhotoStorage.PublicEndpoint,
		Bucket:         cfg.PhotoStorage.Bucket,
		AccessKey:      cfg.PhotoStorage.AccessKey,
		SecretKey:      cfg.PhotoStorage.SecretKey,
		Region:         cfg.PhotoStorage.Region,
		PresignTTL:     cfg.PhotoStorage.PresignTTL,
	})
	if err != nil {
		logger.Error("photo storage configuration failed", "error", err)
		os.Exit(1)
	}

	photos := postgres.NewPhotoRepository(db)
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Dependencies{
			Logger:          logger,
			Database:        db,
			CORSOrigins:     cfg.CORSOrigins,
			AuthRateLimiter: limiter,
			Auth: appauth.NewService(
				postgres.NewUserRepository(db),
				postgres.NewRefreshSessionRepository(db),
				security.NewDefaultPasswordManager(),
				tokens,
			),
			PhotoStore: photoStore,
			Profile:    appprofile.NewService(postgres.NewProfileRepository(db), photos, photoStore),
			Media:      appmedia.NewService(photos, photoStore),
			Discovery: appdiscovery.NewService(
				postgres.NewDiscoveryRepository(db), postgres.NewSearchStateRepository(db), photoStore,
			),
			Matching: appmatching.NewService(
				postgres.NewSwipeRepository(db), postgres.NewMatchRepository(db), postgres.NewSearchStateRepository(db), photoStore,
			),
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	go func() {
		logger.Info("http server started", "address", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
