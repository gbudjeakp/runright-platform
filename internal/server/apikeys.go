package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// APIKey represents a user-managed API key.
type APIKey struct {
	ID         string    `json:"id"`
	TeamID     string    `json:"team_id,omitempty"`
	UserEmail  string    `json:"user_email"`
	Name       string    `json:"name"`
	KeyPrefix  string    `json:"key_prefix"`
	Scopes     []string  `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// generateAPIKey creates a new random API key.
func generateAPIKey() (key string, prefix string, hash string) {
	// Generate 32 random bytes
	keyBytes := make([]byte, 32)
	rand.Read(keyBytes)
	
	// Format as rr_xxxx... (64 hex chars)
	key = "rr_" + hex.EncodeToString(keyBytes)
	prefix = key[:10] // rr_xxxx...
	
	// Hash for storage
	h := sha256.Sum256([]byte(key))
	hash = hex.EncodeToString(h[:])
	
	return key, prefix, hash
}

// --- API Key Handlers ---

func (s *Server) listAPIKeys(c *gin.Context) {
	userEmail := getUserEmail(c)
	teamID := c.Query("team_id")
	ctx := c.Request.Context()

	query := `
		SELECT id, team_id, user_email, name, key_prefix, scopes, last_used_at, expires_at, revoked_at, created_at
		FROM api_keys
		WHERE user_email = $1 AND revoked_at IS NULL
	`
	args := []any{userEmail}

	if teamID != "" {
		query = `
			SELECT id, team_id, user_email, name, key_prefix, scopes, last_used_at, expires_at, revoked_at, created_at
			FROM api_keys
			WHERE team_id = $1 AND revoked_at IS NULL
		`
		args = []any{teamID}
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list API keys"})
		return
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var teamID sql.NullString
		var lastUsedAt, expiresAt, revokedAt sql.NullTime
		var scopes []byte
		if err := rows.Scan(&k.ID, &teamID, &k.UserEmail, &k.Name, &k.KeyPrefix, &scopes, &lastUsedAt, &expiresAt, &revokedAt, &k.CreatedAt); err != nil {
			continue
		}
		if teamID.Valid {
			k.TeamID = teamID.String
		}
		if lastUsedAt.Valid {
			k.LastUsedAt = &lastUsedAt.Time
		}
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		if revokedAt.Valid {
			k.RevokedAt = &revokedAt.Time
		}
		// Parse scopes JSON array
		if len(scopes) > 0 {
			var s []string
			if err := json.Unmarshal(scopes, &s); err == nil {
				k.Scopes = s
			}
		}
		keys = append(keys, k)
	}

	c.JSON(http.StatusOK, gin.H{"api_keys": keys})
}

func (s *Server) createAPIKey(c *gin.Context) {
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	var body struct {
		Name      string   `json:"name" binding:"required"`
		TeamID    string   `json:"team_id"`
		Scopes    []string `json:"scopes"`
		ExpiresIn string   `json:"expires_in"` // e.g., "30d", "90d", "1y", "never"
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}

	// Default scopes
	if len(body.Scopes) == 0 {
		body.Scopes = []string{"read", "write"}
	}

	// Calculate expiration
	var expiresAt *time.Time
	switch body.ExpiresIn {
	case "30d":
		t := time.Now().Add(30 * 24 * time.Hour)
		expiresAt = &t
	case "90d":
		t := time.Now().Add(90 * 24 * time.Hour)
		expiresAt = &t
	case "1y":
		t := time.Now().Add(365 * 24 * time.Hour)
		expiresAt = &t
	case "never", "":
		// No expiration
	}

	// Generate key
	key, prefix, hash := generateAPIKey()

	scopesJSON, _ := json.Marshal(body.Scopes)

	var keyID string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO api_keys (team_id, user_email, name, key_prefix, key_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, nilIfEmpty(body.TeamID), userEmail, body.Name, prefix, hash, scopesJSON, expiresAt).Scan(&keyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create API key"})
		return
	}

	s.logAudit(ctx, userEmail, c, "apikey.create", "api_key", keyID, body.Name, map[string]any{
		"scopes":     body.Scopes,
		"expires_in": body.ExpiresIn,
	})

	// Return the full key only once - it won't be retrievable again
	c.JSON(http.StatusCreated, gin.H{
		"id":         keyID,
		"key":        key,
		"key_prefix": prefix,
		"name":       body.Name,
		"scopes":     body.Scopes,
		"expires_at": expiresAt,
		"message":    "Save this key now. It won't be shown again.",
	})
}

func (s *Server) revokeAPIKey(c *gin.Context) {
	keyID := c.Param("keyId")
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	// Get key name for audit
	var keyName string
	s.db.QueryRowContext(ctx, "SELECT name FROM api_keys WHERE id = $1", keyID).Scan(&keyName)

	result, err := s.db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND (user_email = $2 OR EXISTS (
			SELECT 1 FROM team_members WHERE team_id = api_keys.team_id AND user_email = $2 AND role IN ('owner', 'admin')
		))
	`, keyID, userEmail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke API key"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found or access denied"})
		return
	}

	s.logAudit(ctx, userEmail, c, "apikey.revoke", "api_key", keyID, keyName, nil)
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func (s *Server) updateAPIKeyLastUsed(ctx context.Context, keyHash string) {
	s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1`, keyHash)
}

// validateAPIKeyFromDB checks a DB-issued API key (created via the UI) and
// returns the owner email and scopes. Used by authMiddleware as a fallback
// so programmatic clients can use keys generated in the dashboard.
func (s *Server) validateAPIKeyFromDB(ctx context.Context, key string) (email string, scopes []string, ok bool) {
	if !strings.HasPrefix(key, "rr_") {
		return "", nil, false
	}

	h := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(h[:])

	var scopesJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT user_email, scopes FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > NOW())
	`, hash).Scan(&email, &scopesJSON)
	if err != nil {
		return "", nil, false
	}

	go s.updateAPIKeyLastUsed(context.Background(), hash)

	if len(scopesJSON) > 0 {
		_ = json.Unmarshal(scopesJSON, &scopes)
	}
	return email, scopes, true
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
