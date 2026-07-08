package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sgbudje/runright-platform/internal/assistant"
	"github.com/sgbudje/runright-platform/internal/types"
)

// assistantChat handles POST /api/v1/assistant/chat
func (s *Server) assistantChat(c *gin.Context) {
	var req types.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	userID := c.GetString("user_email")

	resp, err := s.assistant.Chat(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// assistantConversations handles GET /api/v1/assistant/conversations
func (s *Server) assistantConversations(c *gin.Context) {
	userID := c.GetString("user_email")

	convs, err := s.assistant.GetConversations(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, convs)
}

// assistantGetConversation handles GET /api/v1/assistant/conversations/:id
func (s *Server) assistantGetConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id required"})
		return
	}

	userID := c.GetString("user_email")

	conv, messages, err := s.assistant.GetConversation(c.Request.Context(), convID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation": conv,
		"messages":     messages,
	})
}

// assistantDeleteConversation handles DELETE /api/v1/assistant/conversations/:id
func (s *Server) assistantDeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id required"})
		return
	}

	userID := c.GetString("user_email")

	if err := s.assistant.DeleteConversation(c.Request.Context(), convID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// assistantDeleteAllConversations handles DELETE /api/v1/assistant/conversations
func (s *Server) assistantDeleteAllConversations(c *gin.Context) {
	userID := c.GetString("user_email")

	n, err := s.assistant.DeleteAllConversations(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// assistantQuickStats handles GET /api/v1/assistant/stats
func (s *Server) assistantQuickStats(c *gin.Context) {
	stats, err := s.assistant.GetQuickStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// assistantSuggestedQuestions handles GET /api/v1/assistant/suggestions
func (s *Server) assistantSuggestedQuestions(c *gin.Context) {
	questions := s.assistant.SuggestedQuestions(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

// assistantStatus handles GET /api/v1/assistant/status
func (s *Server) assistantStatus(c *gin.Context) {
	status := gin.H{
		"configured": s.assistant.IsConfigured(),
	}
	// Add provider info if configured
	if s.assistant.IsConfigured() {
		for k, v := range s.assistant.GetProviderInfo() {
			status[k] = v
		}
	}
	c.JSON(http.StatusOK, status)
}

// registerAssistantRoutes adds assistant endpoints to the router.
func (s *Server) registerAssistantRoutes(v1 *gin.RouterGroup) {
	if s.assistant == nil {
		s.assistant = assistant.NewFromEnv(s.db)
	}

	// Inject embeddings service for RAG if configured
	if s.embeddings != nil && s.embeddings.IsConfigured() {
		s.assistant.SetEmbeddingService(s.embeddings)
	}

	ast := v1.Group("/assistant")
	{
		ast.GET("/status", s.assistantStatus)
		ast.GET("/suggestions", s.assistantSuggestedQuestions)
		ast.GET("/stats", s.assistantQuickStats)
		ast.POST("/chat", s.assistantChat)
		ast.GET("/conversations", s.assistantConversations)
		ast.DELETE("/conversations", s.assistantDeleteAllConversations)
		ast.GET("/conversations/:id", s.assistantGetConversation)
		ast.DELETE("/conversations/:id", s.assistantDeleteConversation)
	}
}
