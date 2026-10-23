package auth

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"lot/internal/demo"
	"lot/internal/httpx"
)

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"` // user | admin
	BlockedAt   *time.Time `json:"blockedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

const userColumns = `id, email, display_name, role, blocked_at, created_at`

func (u *User) scanTargets() []any {
	return []any{&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.BlockedAt, &u.CreatedAt}
}

func errBlocked(w http.ResponseWriter) {
	httpx.Error(w, http.StatusForbidden, "account_blocked", "Аккаунт заблокирован администратором")
}

type Handlers struct {
	pool         *pgxpool.Pool
	tokens       *Tokens
	cookieSecure bool
	demoLogin    bool
}

func NewHandlers(pool *pgxpool.Pool, tokens *Tokens, cookieSecure, demoLogin bool) *Handlers {
	return &Handlers{pool: pool, tokens: tokens, cookieSecure: cookieSecure, demoLogin: demoLogin}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if _, err := mail.ParseAddress(req.Email); err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email", "Некорректный email")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 72 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_password", "Пароль должен быть от 8 до 72 символов")
		return
	}
	if n := len([]rune(req.DisplayName)); n < 2 || n > 50 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name", "Имя должно быть от 2 до 50 символов")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
		return
	}

	var u User
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		req.Email, string(hash), req.DisplayName,
	).Scan(u.scanTargets()...)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpx.Error(w, http.StatusConflict, "email_taken", "Пользователь с таким email уже существует")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
		return
	}

	h.setCookie(w, u.ID)
	httpx.JSON(w, http.StatusCreated, u)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	var (
		u    User
		hash string
	)
	err := h.pool.QueryRow(r.Context(), `
		SELECT `+userColumns+`, password_hash FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(req.Email)),
	).Scan(append(u.scanTargets(), &hash)...)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
		return
	}
	if u.BlockedAt != nil {
		errBlocked(w)
		return
	}

	h.setCookie(w, u.ID)
	httpx.JSON(w, http.StatusOK, u)
}

// DemoAccounts — список ролей для быстрого входа (404, если демо-вход выключен).
func (h *Handlers) DemoAccounts(w http.ResponseWriter, r *http.Request) {
	if !h.demoLogin {
		httpx.Error(w, http.StatusNotFound, "demo_disabled", "Демо-вход выключен")
		return
	}
	httpx.JSON(w, http.StatusOK, demo.Accounts)
}

type demoLoginRequest struct {
	Role string `json:"role"`
}

// DemoLogin входит в демо-аккаунт выбранной роли без пароля.
func (h *Handlers) DemoLogin(w http.ResponseWriter, r *http.Request) {
	if !h.demoLogin {
		httpx.Error(w, http.StatusNotFound, "demo_disabled", "Демо-вход выключен")
		return
	}
	var req demoLoginRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	acc, ok := demo.Find(req.Role)
	if !ok {
		httpx.Error(w, http.StatusUnprocessableEntity, "unknown_role", "Неизвестная роль")
		return
	}

	var u User
	err := h.pool.QueryRow(r.Context(),
		`SELECT `+userColumns+` FROM users WHERE email = $1`, acc.Email,
	).Scan(u.scanTargets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "demo_not_seeded", "Демо-аккаунт не найден — запустите go run ./cmd/seed")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
		return
	}
	if u.BlockedAt != nil {
		errBlocked(w)
		return
	}

	h.setCookie(w, u.ID)
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	httpx.JSON(w, http.StatusOK, u)
}

type userKey struct{}

// RequireUser пропускает только авторизованных и незаблокированных пользователей.
// Пользователь читается из БД на каждый запрос, поэтому блокировка и смена роли
// действуют сразу, без ожидания истечения JWT.
func (h *Handlers) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserID(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Требуется вход в систему")
			return
		}
		var u User
		err := h.pool.QueryRow(r.Context(), `SELECT `+userColumns+` FROM users WHERE id = $1`, id).Scan(u.scanTargets()...)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Пользователь не найден")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
			return
		}
		if u.BlockedAt != nil {
			errBlocked(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

// RequireAdmin — только для администраторов; ставится после RequireUser.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := CurrentUser(r.Context()); !ok || u.Role != "admin" {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Доступно только администраторам")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CurrentUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

func (h *Handlers) setCookie(w http.ResponseWriter, userID int64) {
	token, err := h.tokens.Issue(userID)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(tokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
