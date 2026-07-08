package server

// ownership.go — repository → team → destination routing.
//
// Ownership rules let you declare "team X owns repo Y and should receive
// alerts on destinations A, B" without manually adding that repo to every
// alert rule. The dispatcher automatically augments rule destination lists
// with any ownership-based destinations that match the job's repository.

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sgbudje/runright-platform/internal/notification"
)

// ownershipEntry represents one team's ownership of a repository.
type ownershipEntry struct {
	Repository     string    `json:"repository"`
	TeamName       string    `json:"team_name"`
	DestinationIDs []string  `json:"destination_ids"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// listOwnership returns all ownership entries, optionally filtered by repo.
func (s *Server) listOwnership(c *gin.Context) {
	repo := c.Query("repository")
	var (
		rows interface{ Next() bool; Scan(...any) error; Close() error; Err() error }
		err  error
	)
	if repo != "" {
		rows, err = s.db.QueryContext(c.Request.Context(), `
			SELECT repository, team_name, destination_ids, created_at, updated_at
			FROM repository_ownership
			WHERE repository = $1
			ORDER BY team_name ASC`, repo)
	} else {
		rows, err = s.db.QueryContext(c.Request.Context(), `
			SELECT repository, team_name, destination_ids, created_at, updated_at
			FROM repository_ownership
			ORDER BY repository ASC, team_name ASC`)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []ownershipEntry{}
	for rows.Next() {
		var entry ownershipEntry
		var destJSON []byte
		if err := rows.Scan(&entry.Repository, &entry.TeamName, &destJSON, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(destJSON, &entry.DestinationIDs); err != nil {
			entry.DestinationIDs = []string{}
		}
		out = append(out, entry)
	}
	c.JSON(http.StatusOK, out)
}

// upsertOwnership creates or replaces one ownership entry.
func (s *Server) upsertOwnership(c *gin.Context) {
	var body struct {
		Repository     string   `json:"repository" binding:"required"`
		TeamName       string   `json:"team_name"  binding:"required"`
		DestinationIDs []string `json:"destination_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.DestinationIDs == nil {
		body.DestinationIDs = []string{}
	}
	destJSON, _ := json.Marshal(body.DestinationIDs)

	_, err := s.db.ExecContext(c.Request.Context(), `
		INSERT INTO repository_ownership (repository, team_name, destination_ids, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (repository, team_name) DO UPDATE SET
			destination_ids = EXCLUDED.destination_ids,
			updated_at      = NOW()`,
		body.Repository, body.TeamName, destJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.logAudit(c.Request.Context(), getUserEmail(c), c, "ownership.upsert", "repository_ownership",
		body.Repository+"/"+body.TeamName, body.Repository,
		map[string]any{"team_name": body.TeamName, "destination_ids": body.DestinationIDs})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// deleteOwnership removes one ownership entry.
func (s *Server) deleteOwnership(c *gin.Context) {
	repo := c.Query("repository")
	team := c.Query("team_name")
	if repo == "" || team == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repository and team_name are required"})
		return
	}
	_, err := s.db.ExecContext(c.Request.Context(), `
		DELETE FROM repository_ownership WHERE repository = $1 AND team_name = $2`,
		repo, team)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.logAudit(c.Request.Context(), getUserEmail(c), c, "ownership.delete", "repository_ownership",
		repo+"/"+team, repo, map[string]any{"team_name": team})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// loadOwnershipDestinations returns the union of all destination IDs that
// ownership rules assign to a given repository. Returns nil if none.
func (s *Server) loadOwnershipDestinations(ctx context.Context, repository string) ([]string, error) {
	if repository == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT destination_ids FROM repository_ownership WHERE repository = $1`, repository)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]struct{}{}
	var result []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			continue
		}
		for _, id := range ids {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result, rows.Err()
}

// mergeOwnershipIntoRules augments each rule's DestinationIDs with any
// ownership-based destination IDs for the job's repository. Only adds IDs
// that aren't already present; does not remove existing IDs.
func mergeOwnershipIntoRules(rules []notification.Rule, ownershipDestIDs []string) []notification.Rule {
	if len(ownershipDestIDs) == 0 {
		return rules
	}
	merged := make([]notification.Rule, len(rules))
	for i, rule := range rules {
		existing := map[string]struct{}{}
		for _, id := range rule.DestinationIDs {
			existing[id] = struct{}{}
		}
		var extra []string
		for _, id := range ownershipDestIDs {
			if _, ok := existing[id]; !ok {
				extra = append(extra, id)
			}
		}
		if len(extra) == 0 {
			merged[i] = rule
			continue
		}
		cp := rule
		cp.DestinationIDs = append(append([]string{}, rule.DestinationIDs...), extra...)
		merged[i] = cp
	}
	return merged
}
