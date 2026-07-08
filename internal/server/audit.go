package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID            string                 `json:"id"`
	TeamID        string                 `json:"team_id,omitempty"`
	ActorEmail    string                 `json:"actor_email"`
	ActorIP       string                 `json:"actor_ip,omitempty"`
	ActorUA       string                 `json:"actor_user_agent,omitempty"`
	Action        string                 `json:"action"`
	ResourceType  string                 `json:"resource_type"`
	ResourceID    string                 `json:"resource_id,omitempty"`
	ResourceName  string                 `json:"resource_name,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
	Status        string                 `json:"status"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// logAudit records an audit log entry.
func (s *Server) logAudit(ctx context.Context, actor string, c *gin.Context, action, resourceType, resourceID, resourceName string, details any) {
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	var detailsJSON []byte
	if details != nil {
		detailsJSON, _ = json.Marshal(details)
	}

	// Get team_id from context if available
	var teamID *string
	if tid, ok := c.Get("team_id"); ok {
		t := tid.(string)
		teamID = &t
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (team_id, actor_email, actor_ip, actor_user_agent, action, resource_type, resource_id, resource_name, details, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'success')
	`, teamID, actor, ip, ua, action, resourceType, resourceID, resourceName, detailsJSON)
	if err != nil {
		// Don't fail the request if audit logging fails
		// TODO: log to stderr or metrics
	}
}

// logAuditError records a failed audit log entry.
func (s *Server) logAuditError(ctx context.Context, actor string, c *gin.Context, action, resourceType, resourceID string, errMsg string) {
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	var teamID *string
	if tid, ok := c.Get("team_id"); ok {
		t := tid.(string)
		teamID = &t
	}

	s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (team_id, actor_email, actor_ip, actor_user_agent, action, resource_type, resource_id, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'failure', $8)
	`, teamID, actor, ip, ua, action, resourceType, resourceID, errMsg)
}

// --- Audit Log Handlers ---

func (s *Server) listAuditLogs(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Query("team_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	action := c.Query("action")
	actor := c.Query("actor")
	resourceType := c.Query("resource_type")

	if limit > 1000 {
		limit = 1000
	}

	// Build query
	query := `
		SELECT id, team_id, actor_email, actor_ip, action, resource_type, resource_id, resource_name, details, status, error_message, created_at
		FROM audit_logs
		WHERE 1=1
	`
	args := []any{}
	argIdx := 1

	if teamID != "" {
		query += " AND team_id = $" + strconv.Itoa(argIdx)
		args = append(args, teamID)
		argIdx++
	}
	if action != "" {
		query += " AND action LIKE $" + strconv.Itoa(argIdx)
		args = append(args, action+"%")
		argIdx++
	}
	if actor != "" {
		query += " AND actor_email = $" + strconv.Itoa(argIdx)
		args = append(args, actor)
		argIdx++
	}
	if resourceType != "" {
		query += " AND resource_type = $" + strconv.Itoa(argIdx)
		args = append(args, resourceType)
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list audit logs"})
		return
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var teamID, actorIP, resourceID, resourceName, errorMessage sql.NullString
		var details []byte
		if err := rows.Scan(&log.ID, &teamID, &log.ActorEmail, &actorIP, &log.Action, &log.ResourceType, &resourceID, &resourceName, &details, &log.Status, &errorMessage, &log.CreatedAt); err != nil {
			continue
		}
		if teamID.Valid {
			log.TeamID = teamID.String
		}
		if actorIP.Valid {
			log.ActorIP = actorIP.String
		}
		if resourceID.Valid {
			log.ResourceID = resourceID.String
		}
		if resourceName.Valid {
			log.ResourceName = resourceName.String
		}
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if len(details) > 0 {
			json.Unmarshal(details, &log.Details)
		}
		logs = append(logs, log)
	}

	// Get total count
	var total int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs WHERE team_id = $1 OR $1 IS NULL", teamID).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) getAuditLog(c *gin.Context) {
	logID := c.Param("logId")
	ctx := c.Request.Context()

	var log AuditLog
	var teamID, actorIP, actorUA, resourceID, resourceName, errorMessage sql.NullString
	var details []byte
	
	err := s.db.QueryRowContext(ctx, `
		SELECT id, team_id, actor_email, actor_ip, actor_user_agent, action, resource_type, resource_id, resource_name, details, status, error_message, created_at
		FROM audit_logs WHERE id = $1
	`, logID).Scan(&log.ID, &teamID, &log.ActorEmail, &actorIP, &actorUA, &log.Action, &log.ResourceType, &resourceID, &resourceName, &details, &log.Status, &errorMessage, &log.CreatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get audit log"})
		return
	}

	if teamID.Valid {
		log.TeamID = teamID.String
	}
	if actorIP.Valid {
		log.ActorIP = actorIP.String
	}
	if actorUA.Valid {
		log.ActorUA = actorUA.String
	}
	if resourceID.Valid {
		log.ResourceID = resourceID.String
	}
	if resourceName.Valid {
		log.ResourceName = resourceName.String
	}
	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}
	if len(details) > 0 {
		json.Unmarshal(details, &log.Details)
	}

	c.JSON(http.StatusOK, log)
}

// exportAuditLogs exports audit logs as CSV or JSON
func (s *Server) exportAuditLogs(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Query("team_id")
	format := c.DefaultQuery("format", "json")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := `
		SELECT id, team_id, actor_email, actor_ip, action, resource_type, resource_id, resource_name, details, status, error_message, created_at
		FROM audit_logs
		WHERE (team_id = $1 OR $1 = '')
	`
	args := []any{teamID}
	argIdx := 2

	if startDate != "" {
		query += " AND created_at >= $" + strconv.Itoa(argIdx)
		args = append(args, startDate)
		argIdx++
	}
	if endDate != "" {
		query += " AND created_at <= $" + strconv.Itoa(argIdx)
		args = append(args, endDate)
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT 10000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export audit logs"})
		return
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var teamID, actorIP, resourceID, resourceName, errorMessage sql.NullString
		var details []byte
		if err := rows.Scan(&log.ID, &teamID, &log.ActorEmail, &actorIP, &log.Action, &log.ResourceType, &resourceID, &resourceName, &details, &log.Status, &errorMessage, &log.CreatedAt); err != nil {
			continue
		}
		if teamID.Valid {
			log.TeamID = teamID.String
		}
		if actorIP.Valid {
			log.ActorIP = actorIP.String
		}
		if resourceID.Valid {
			log.ResourceID = resourceID.String
		}
		if resourceName.Valid {
			log.ResourceName = resourceName.String
		}
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if len(details) > 0 {
			json.Unmarshal(details, &log.Details)
		}
		logs = append(logs, log)
	}

	userEmail := getUserEmail(c)
	s.logAudit(ctx, userEmail, c, "audit.export", "audit_logs", "", "", map[string]any{
		"format":     format,
		"count":      len(logs),
		"start_date": startDate,
		"end_date":   endDate,
	})

	if format == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=audit-logs.csv")
		c.Writer.WriteString("id,timestamp,actor,action,resource_type,resource_id,status\n")
		for _, log := range logs {
			c.Writer.WriteString(log.ID + "," + log.CreatedAt.Format(time.RFC3339) + "," + log.ActorEmail + "," + log.Action + "," + log.ResourceType + "," + log.ResourceID + "," + log.Status + "\n")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}
