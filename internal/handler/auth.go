package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/config"
)

type AuthHandler struct {
	db  *pgxpool.Pool
	cfg *config.Config
}

func NewAuthHandler(db *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
}

type githubCallbackReq struct {
	Code string `json:"code" binding:"required"`
}

func (h *AuthHandler) GitHubCallback(c *gin.Context) {
	// Support both GET (GitHub redirect with ?code=) and POST (frontend fetch)
	code := c.Query("code")
	if code == "" {
		var req githubCallbackReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}
		code = req.Code
	}

	// 1. Exchange code for access token
	accessToken, err := exchangeGitHubToken(h.cfg.GitHubClientID, h.cfg.GitHubClientSecret, code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("github token exchange: %v", err)})
		return
	}

	// 2. Get user info from GitHub
	ghUser, err := fetchGitHubUser(accessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("fetch github user: %v", err)})
		return
	}

	// 3. Upsert user
	userID, err := h.upsertUser(c.Request.Context(), ghUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("upsert user: %v", err)})
		return
	}

	// 4. Generate JWT
	token, err := auth.GenerateToken(userID, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("generate token: %v", err)})
		return
	}

	// If GET (GitHub redirect), redirect to frontend with token
	if c.Request.Method == "GET" {
		c.Redirect(http.StatusFound,
			fmt.Sprintf("/?token=%s&username=%s&avatar=%s", token, ghUser.Login, ghUser.AvatarURL))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":         userID,
			"username":   ghUser.Login,
			"avatar_url": ghUser.AvatarURL,
		},
	})
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func exchangeGitHubToken(clientID, clientSecret, code string) (string, error) {
	resp, err := http.PostForm("https://github.com/login/oauth/access_token", url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGitHubUser(token string) (*githubUser, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (h *AuthHandler) upsertUser(ctx context.Context, u *githubUser) (string, error) {
	var userID string
	err := h.db.QueryRow(ctx, `
		INSERT INTO users (github_id, username, email, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (github_id) DO UPDATE SET username=$2, email=$3, avatar_url=$4
		RETURNING id
	`, u.ID, u.Login, u.Email, u.AvatarURL).Scan(&userID)
	return userID, err
}

func (h *AuthHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/auth/github/callback", h.GitHubCallback)
	r.POST("/auth/github/callback", h.GitHubCallback)
}
