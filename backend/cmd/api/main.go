package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"lot/internal/admin"
	"lot/internal/auth"
	"lot/internal/closer"
	"lot/internal/config"
	"lot/internal/db"
	"lot/internal/httpx"
	"lot/internal/lots"
	"lot/internal/realtime"
	"lot/internal/uploads"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, log); err != nil {
		return err
	}

	tokens := auth.NewTokens(cfg.JWTSecret)
	hub := realtime.NewHub(cfg.AllowedOrigins, func(r *http.Request) (int64, bool) {
		return auth.UserID(r.Context())
	}, log)
	go hub.RunPresence(ctx, 2*time.Second)
	authH := auth.NewHandlers(pool, tokens, cfg.CookieSecure, cfg.DemoLogin)
	lotsRepo := lots.NewRepo(pool, cfg.AntiSniping)
	lotsH := lots.NewHandlers(lotsRepo, hub, log)
	adminH := admin.NewHandlers(pool, lotsRepo, hub, hub.Clients, log)
	uploadH, err := uploads.NewHandler(cfg.UploadDir)
	if err != nil {
		return err
	}

	go closer.Run(ctx, pool, cfg.CloseInterval, log, func(c closer.Closed) {
		hub.Broadcast(lots.LotRoom(c.LotID), "lot:closed", c)
		hub.Broadcast(lots.CatalogRoom, "lot:closed", c)
		// Персональные уведомления победителю и продавцу.
		if c.WinnerID != nil {
			hub.Broadcast(realtime.UserRoom(*c.WinnerID), "lot:won", c)
		}
		hub.Broadcast(realtime.UserRoom(c.SellerID), "lot:sold", c)
	})

	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer, tokens.Middleware)

	r.Get("/ws", hub.ServeHTTP)
	r.Handle("/uploads/*", uploadH.Files())

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "wsClients": hub.Clients()})
		})

		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)
		r.Post("/auth/logout", authH.Logout)
		r.Get("/auth/demo", authH.DemoAccounts)
		r.Post("/auth/demo", authH.DemoLogin)
		r.With(authH.RequireUser).Get("/auth/me", authH.Me)

		r.Get("/categories", lotsH.Categories)
		r.Get("/lots", lotsH.List)
		r.Get("/lots/{id}", lotsH.Get)
		r.Get("/lots/{id}/bids", lotsH.Bids)

		r.Group(func(r chi.Router) {
			r.Use(authH.RequireUser)
			r.Post("/lots", lotsH.Create)
			r.Post("/lots/{id}/bids", lotsH.PlaceBid)
			r.Post("/uploads", uploadH.Upload)
			r.Get("/me/bids", lotsH.MyBids)
			r.Get("/me/lots", lotsH.MyLots)

			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/stats", adminH.Stats)
				r.Get("/bids", adminH.RecentBids)
				r.Get("/lots", adminH.Lots)
				r.Post("/lots/{id}/cancel", adminH.CancelLot)
				r.Get("/users", adminH.Users)
				r.Post("/users/{id}/block", adminH.BlockUser)
				r.Post("/users/{id}/unblock", adminH.UnblockUser)
			})
		})
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
