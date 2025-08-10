package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr := getenv("ADDR", ":8080")
	dsn := getenv("DATABASE_URL", "postgres://postgres:postgres@db:5432/trellolite?sslmode=disable")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Error("db open", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Error("db ping", "err", err)
		os.Exit(1)
	}

	store := NewStore(db)
	if err := store.Migrate(context.Background()); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./web"))

	// Helper to serve auth pages without caching
	noStore := func(w http.ResponseWriter) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}

	// Explicit handlers for auth pages to avoid any accidental rewrites and disable caching
	mux.HandleFunc("GET /web/login.html", func(w http.ResponseWriter, r *http.Request) {
		noStore(w)
		http.ServeFile(w, r, "./web/login.html")
	})
	mux.HandleFunc("GET /web/register.html", func(w http.ResponseWriter, r *http.Request) {
		noStore(w)
		http.ServeFile(w, r, "./web/register.html")
	})

	// Gate the root index behind auth: exact path only
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// Not root: let other handlers (e.g., /web/) process
			http.NotFound(w, r)
			return
		}
		cookieName := getenv("SESSION_COOKIE_NAME", "trellolite_sess")
		if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
			// Authenticated: serve app shell directly
			http.ServeFile(w, r, "./web/index.html")
			return
		}
		noStore(w)
		http.ServeFile(w, r, "./web/login.html")
	})

	// Static assets under /web/
	mux.Handle("GET /web/", http.StripPrefix("/web/", fs))

	api := newAPI(store, log)
	api.routes(mux)

	srv := &http.Server{Addr: addr, Handler: withLogging(log, mux),
		ReadTimeout: 15 * time.Second, ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}

	go func() {
		log.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) && err != nil {
			log.Error("listen", "err", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Info("shutting down")
	ctxSh, cancelSh := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelSh()
	if err := srv.Shutdown(ctxSh); err != nil {
		log.Error("shutdown", "err", err)
	}
}
