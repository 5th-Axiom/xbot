package channel

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"xbot/version"
)

// AdminCallbacks holds callback functions for admin API endpoints.
type AdminCallbacks struct {
	MetricsGet  func() map[string]interface{}
	ChannelsGet func() []string
}

// adminTokenMiddleware validates the admin token from Authorization header.
func (wc *WebChannel) adminTokenMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if wc.config.AdminToken == "" {
			jsonErrorResponse(w, http.StatusForbidden, "admin token not configured")
			return
		}
		auth := r.Header.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			jsonErrorResponse(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		token := auth[7:]
		if subtle.ConstantTimeCompare([]byte(token), []byte(wc.config.AdminToken)) != 1 {
			jsonErrorResponse(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		next(w, r)
	}
}

// handleAdminHealth returns server health information.
func (wc *WebChannel) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "running",
		"version": version.Info(),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// handleAdminMetrics returns agent metrics snapshot.
func (wc *WebChannel) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if wc.adminCallbacks.MetricsGet == nil {
		jsonErrorResponse(w, http.StatusServiceUnavailable, "metrics not available")
		return
	}
	writeJSON(w, http.StatusOK, wc.adminCallbacks.MetricsGet())
}

// handleAdminUsers lists or creates web users.
func (wc *WebChannel) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wc.handleAdminListUsers(w, r)
	case http.MethodPost:
		wc.handleAdminCreateUser(w, r)
	default:
		jsonErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (wc *WebChannel) handleAdminListUsers(w http.ResponseWriter, _ *http.Request) {
	if wc.config.DB == nil {
		jsonErrorResponse(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	rows, err := wc.config.DB.Query("SELECT id, username FROM web_users ORDER BY id")
	if err != nil {
		jsonErrorResponse(w, http.StatusInternalServerError, "failed to query users")
		return
	}
	defer rows.Close()

	type user struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}
	var users []user
	for rows.Next() {
		var u user
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			continue
		}
		users = append(users, u)
	}
	if users == nil {
		users = []user{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (wc *WebChannel) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if wc.config.DB == nil {
		jsonErrorResponse(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		jsonErrorResponse(w, http.StatusBadRequest, "username is required")
		return
	}
	username, password, err := CreateWebUser(wc.config.DB, req.Username)
	if err != nil {
		jsonErrorResponse(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"username": username,
		"password": password,
	})
}

// handleAdminDeleteUser deletes a web user by username.
func (wc *WebChannel) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if wc.config.DB == nil {
		jsonErrorResponse(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		jsonErrorResponse(w, http.StatusBadRequest, "username is required")
		return
	}
	result, err := wc.config.DB.Exec("DELETE FROM web_users WHERE username = ?", req.Username)
	if err != nil {
		jsonErrorResponse(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		jsonErrorResponse(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleAdminChannels returns list of enabled channels.
func (wc *WebChannel) handleAdminChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if wc.adminCallbacks.ChannelsGet == nil {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	writeJSON(w, http.StatusOK, wc.adminCallbacks.ChannelsGet())
}

// CreateWebUser is re-exported from web_auth.go for admin usage.
// The actual implementation is in web_auth.go.
// This wrapper exists to allow usage from web_admin.go without import cycles.
func createWebUserFromDB(db *sql.DB, username string) (string, string, error) {
	return CreateWebUser(db, username)
}
