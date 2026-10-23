package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr           string
	DatabaseURL    string
	JWTSecret      string
	CookieSecure   bool
	AllowedOrigins []string      // разрешённые Origin для WebSocket (host[:port])
	UploadDir      string        // куда сохраняются загруженные изображения
	AntiSniping    time.Duration // продление торгов при ставке в последние N секунд (0 = выключено)
	CloseInterval  time.Duration // как часто проверять истёкшие лоты
	DemoLogin      bool          // быстрый вход в демо-аккаунты без пароля (выключить в продакшене)
}

func Load() Config {
	return Config{
		Addr:           env("ADDR", ":8080"),
		DatabaseURL:    env("DATABASE_URL", "postgres://localhost:5432/lot?sslmode=disable"),
		JWTSecret:      env("JWT_SECRET", "dev-secret-change-me"),
		CookieSecure:   env("COOKIE_SECURE", "false") == "true",
		AllowedOrigins: strings.Split(env("WS_ALLOWED_ORIGINS", "localhost:5173,127.0.0.1:5173,localhost:8080"), ","),
		UploadDir:      env("UPLOAD_DIR", "./uploads"),
		AntiSniping:    time.Duration(envInt("ANTI_SNIPING_SECONDS", 30)) * time.Second,
		CloseInterval:  time.Duration(envInt("CLOSE_INTERVAL_MS", 1000)) * time.Millisecond,
		DemoLogin:      env("DEMO_LOGIN", "true") == "true",
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
