package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dytonpictures/werk-plattform/apps/api/internal/auth"
	"github.com/dytonpictures/werk-plattform/apps/api/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type database interface {
	Ping(context.Context) error
}

type queryDatabase interface {
	database
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type application struct {
	cfg    config.Config
	db     database
	logger *slog.Logger
}

func NewHandler(cfg config.Config, db database, logger *slog.Logger) http.Handler {
	a := &application{cfg: cfg, db: db, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /ready", a.ready)
	mux.HandleFunc("GET /api/v1/system/info", a.systemInfo)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("GET /api/v1/auth/me", a.me)
	mux.HandleFunc("GET /api/v1/users", a.usersList)
	mux.HandleFunc("POST /api/v1/users", a.usersCreate)
	mux.HandleFunc("PATCH /api/v1/users/{id}/status", a.userStatus)
	mux.HandleFunc("GET /api/v1/audit", a.auditList)
	return requestMiddleware(mux, logger)
}

func (a *application) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	var id, hash string
	var active bool
	err := q.QueryRow(r.Context(), `SELECT id, password_hash, is_active FROM users WHERE email = lower($1)`, strings.TrimSpace(input.Email)).Scan(&id, &hash, &active)
	if err != nil || !active || !auth.CheckPassword(hash, input.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	token := randomToken()
	tokenHash := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(12 * time.Hour)
	if _, err := q.Exec(r.Context(), `INSERT INTO sessions (id,user_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)`, uuid.New(), id, tokenHash[:], expires); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "werk_session", Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.Environment == "production", SameSite: http.SameSiteLaxMode, Expires: expires})
	writeJSON(w, http.StatusNoContent, nil)
}

func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("werk_session"); err == nil {
		if q, ok := a.db.(queryDatabase); ok {
			hash := sha256.Sum256([]byte(cookie.Value))
			_, _ = q.Exec(r.Context(), `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash[:])
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "werk_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.cfg.Environment == "production", SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("werk_session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	var id, email, name string
	err = q.QueryRow(r.Context(), `SELECT u.id, u.email, u.display_name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.is_active`, hash[:]).Scan(&id, &email, &name)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "email": email, "displayName": name})
}

func (a *application) usersList(w http.ResponseWriter, r *http.Request) {
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	actor, status, err := a.requireAdmin(r.Context(), q, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": httpStatusError(status)})
		return
	}
	rows, err := q.Query(r.Context(), `SELECT id,email,display_name,is_active,created_at FROM users ORDER BY email`)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	defer rows.Close()
	users := make([]map[string]any, 0)
	for rows.Next() {
		var id, email, name string
		var active bool
		var created time.Time
		if rows.Scan(&id, &email, &name, &active, &created) == nil {
			users = append(users, map[string]any{"id": id, "email": email, "displayName": name, "isActive": active, "createdAt": created})
		}
	}
	_ = actor
	writeJSON(w, 200, map[string]any{"items": users})
}

func (a *application) usersCreate(w http.ResponseWriter, r *http.Request) {
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	actor, status, err := a.requireAdmin(r.Context(), q, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": httpStatusError(status)})
		return
	}
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input) != nil || strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.DisplayName) == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_password"})
		return
	}
	id := uuid.New()
	if _, err = q.Exec(r.Context(), `INSERT INTO users (id,email,display_name,password_hash) VALUES ($1,lower($2),$3,$4)`, id, input.Email, input.DisplayName, hash); err != nil {
		writeJSON(w, 409, map[string]string{"error": "user_exists"})
		return
	}
	_, _ = q.Exec(r.Context(), `INSERT INTO audit_events (id,actor_user_id,action,target_type,target_id) VALUES ($1,$2,'user.created','user',$3)`, uuid.New(), actor, id.String())
	writeJSON(w, 201, map[string]string{"id": id.String()})
}

func (a *application) requireAdmin(ctx context.Context, q queryDatabase, r *http.Request) (string, int, error) {
	cookie, err := r.Cookie("werk_session")
	if err != nil {
		return "", http.StatusUnauthorized, err
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	var id string
	if err = q.QueryRow(ctx, `SELECT u.id FROM sessions s JOIN users u ON u.id=s.user_id JOIN user_roles ur ON ur.user_id=u.id JOIN roles ro ON ro.id=ur.role_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.is_active AND ro.name='system_admin'`, hash[:]).Scan(&id); err != nil {
		return "", http.StatusForbidden, err
	}
	return id, 0, nil
}

func httpStatusError(status int) string {
	if status == http.StatusUnauthorized {
		return "unauthorized"
	}
	return "forbidden"
}

func (a *application) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *application) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		a.logger.Warn("readiness database check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *application) systemInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"name":        "WERK",
		"version":     a.cfg.Version,
		"environment": a.cfg.Environment,
		"apiVersion":  "v1",
	})
}

func requestMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := randomID()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if origin := r.Header.Get("Origin"); origin == "http://127.0.0.1:3000" || origin == "http://localhost:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://127.0.0.1:3000" && origin != "http://localhost:3000" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin_denied"})
				return
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
		logger.Info("http request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}

func (a *application) userStatus(w http.ResponseWriter, r *http.Request) {
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	actor, status, err := a.requireAdmin(r.Context(), q, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": httpStatusError(status)})
		return
	}
	var input struct {
		Active bool `json:"active"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	id := r.PathValue("id")
	if !input.Active {
		var admins int
		if err = q.QueryRow(r.Context(), `SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles ro ON ro.id=ur.role_id WHERE u.is_active AND ro.name='system_admin'`).Scan(&admins); err != nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		var targetAdmin bool
		_ = q.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$1 AND ro.name='system_admin')`, id).Scan(&targetAdmin)
		if targetAdmin && admins <= 1 {
			writeJSON(w, 409, map[string]string{"error": "last_system_admin"})
			return
		}
	}
	result, err := q.Exec(r.Context(), `UPDATE users SET is_active=$1,updated_at=now() WHERE id=$2`, input.Active, id)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "user_not_found"})
		return
	}
	if !input.Active {
		_, _ = q.Exec(r.Context(), `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	action := "user.deactivated"
	if input.Active {
		action = "user.activated"
	}
	_, _ = q.Exec(r.Context(), `INSERT INTO audit_events (id,actor_user_id,action,target_type,target_id,metadata) VALUES ($1,$2,$3,'user',$4,jsonb_build_object('active',$5))`, uuid.New(), actor, action, id, input.Active)
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) auditList(w http.ResponseWriter, r *http.Request) {
	q, ok := a.db.(queryDatabase)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	_, status, err := a.requireAdmin(r.Context(), q, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": httpStatusError(status)})
		return
	}
	rows, err := q.Query(r.Context(), `SELECT id,action,target_type,target_id,created_at FROM audit_events ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, action, typ string
		var target *string
		var at time.Time
		if rows.Scan(&id, &action, &typ, &target, &at) == nil {
			items = append(items, map[string]any{"id": id, "action": action, "targetType": typ, "targetId": target, "createdAt": at})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func randomID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}
