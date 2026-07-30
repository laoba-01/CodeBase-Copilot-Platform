package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	Code  string `json:"code" binding:"required"`
	State string `json:"state"`
}

func (h *AuthHandler) GitHubCallback(c *gin.Context) {
	// Support both GET (GitHub redirect with ?code=&state=) and POST (frontend fetch)
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		var req githubCallbackReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}
		code = req.Code
		state = req.State
	}

	// Validate OAuth state parameter to prevent CSRF
	if state == "" || !h.validateState(c, state) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or missing state parameter"})
		return
	}

	// 1. Exchange code for access token
	accessToken, err := exchangeGitHubToken(h.cfg.GitHubClientID, h.cfg.GitHubClientSecret, code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "github authentication failed"})
		return
	}

	// 2. Get user info from GitHub
	ghUser, err := fetchGitHubUser(accessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch user information"})
		return
	}

	// 3. Upsert user
	userID, err := h.upsertUser(c.Request.Context(), ghUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user account"})
		return
	}

	// 4. Generate JWT
	token, err := auth.GenerateToken(userID, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate session"})
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

// generateState creates a random OAuth state token.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// validateState checks the OAuth state parameter against the session cookie.
func (h *AuthHandler) validateState(c *gin.Context, state string) bool {
	cookie, err := c.Cookie("oauth_state")
	if err != nil || cookie == "" {
		return false
	}
	// Clear the cookie after use
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)
	return cookie == state
}

// OAuthAuthorize generates a state token and returns the GitHub OAuth URL.
// buildRedirectURI constructs the OAuth redirect URI from the request's Host header,
// so it works with any domain (localhost, Cloudflare Tunnel, custom domain, etc.).
func buildRedirectURI(c *gin.Context, path string) string {
	scheme := "https"
	host := c.Request.Host
	if host == "" {
		host = c.Request.Header.Get("X-Forwarded-Host")
	}
	if host == "" {
		host = "localhost:8080"
	}
	// Use http for localhost, https for everything else
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		scheme = "http"
	}
	return scheme + "://" + host + path
}

func (h *AuthHandler) OAuthAuthorize(c *gin.Context) {
	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}
	// Set state in a secure cookie
	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	redirectURI := buildRedirectURI(c, "/auth/github/callback")
	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s&scope=repo",
		h.cfg.GitHubClientID, url.QueryEscape(redirectURI), state,
	)
	c.JSON(http.StatusOK, gin.H{"url": authURL})
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// shared HTTP client with timeouts
var httpClient = &http.Client{}

func exchangeGitHubToken(clientID, clientSecret, code string) (string, error) {
	resp, err := httpClient.PostForm("https://github.com/login/oauth/access_token", url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("token exchange response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGitHubUser(token string) (*githubUser, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("parse github user: %w", err)
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

// ── Gitee OAuth ──

func (h *AuthHandler) GiteeAuthorize(c *gin.Context) {
	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}
	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	redirectURI := buildRedirectURI(c, "/auth/gitee/callback")
	authURL := fmt.Sprintf(
		"https://gitee.com/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		h.cfg.GiteeClientID, url.QueryEscape(redirectURI), state,
	)
	c.JSON(http.StatusOK, gin.H{"url": authURL})
}

func (h *AuthHandler) GiteeCallback(c *gin.Context) {
	// Support both GET (Gitee redirect) and POST (frontend fetch)
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		var req githubCallbackReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
			return
		}
		code = req.Code
		state = req.State
	}

	if state == "" || !h.validateState(c, state) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or missing state parameter"})
		return
	}

	// 1. Exchange code for access token (must match the redirect URI used in authorize)
	accessToken, err := exchangeGiteeToken(h.cfg.GiteeClientID, h.cfg.GiteeClientSecret, code, buildRedirectURI(c, "/auth/gitee/callback"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "gitee authentication failed"})
		return
	}

	// 2. Get user info from Gitee
	user, err := fetchGiteeUser(accessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch user information"})
		return
	}

	// 3. Upsert user
	userID, err := h.upsertGiteeUser(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user account"})
		return
	}

	// 4. Generate JWT
	token, err := auth.GenerateToken(userID, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate session"})
		return
	}

	if c.Request.Method == "GET" {
		c.Redirect(http.StatusFound,
			fmt.Sprintf("/?token=%s&username=%s&avatar=%s", token, user.Login, user.AvatarURL))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":         userID,
			"username":   user.Login,
			"avatar_url": user.AvatarURL,
		},
	})
}

type giteeUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type giteeTokenResp struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func exchangeGiteeToken(clientID, clientSecret, code, redirectURI string) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
	}
	resp, err := httpClient.PostForm("https://gitee.com/oauth/token", data)
	if err != nil {
		return "", fmt.Errorf("gitee token request: %w", err)
	}
	defer resp.Body.Close()

	var result giteeTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("gitee token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("gitee oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGiteeUser(token string) (*giteeUser, error) {
	req, err := http.NewRequest("GET", "https://gitee.com/api/v5/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitee api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitee api returned %d", resp.StatusCode)
	}

	var user giteeUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("parse gitee user: %w", err)
	}
	return &user, nil
}

func (h *AuthHandler) upsertGiteeUser(ctx context.Context, u *giteeUser) (string, error) {
	var userID string
	err := h.db.QueryRow(ctx, `
		INSERT INTO users (github_id, gitee_id, username, email, avatar_url, provider)
		VALUES (NULL, $1, $2, $3, $4, 'gitee')
		ON CONFLICT (gitee_id) DO UPDATE SET username=$2, email=$3, avatar_url=$4
		RETURNING id
	`, u.ID, u.Login, u.Email, u.AvatarURL).Scan(&userID)
	return userID, err
}

func (h *AuthHandler) DevLogin(c *gin.Context) {
	// Only available in development mode
	if !h.cfg.DevMode {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// Dev-only: bypass GitHub OAuth, create/use a local dev user
	devUser := &githubUser{
		ID:        0,
		Login:     "dev",
		Email:     "dev@localhost",
		AvatarURL: "",
	}
	userID, err := h.upsertUser(c.Request.Context(), devUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dev user"})
		return
	}
	token, err := auth.GenerateToken(userID, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate session"})
		return
	}
	c.Redirect(http.StatusFound,
		fmt.Sprintf("/?token=%s&username=%s&avatar=%s", token, devUser.Login, devUser.AvatarURL))
}

func (h *AuthHandler) RegisterRoutes(r *gin.RouterGroup) {
	// GitHub
	r.GET("/auth/github/authorize", h.OAuthAuthorize)
	r.GET("/auth/github/callback", h.GitHubCallback)
	r.POST("/auth/github/callback", h.GitHubCallback)
	// Gitee
	r.GET("/auth/gitee/authorize", h.GiteeAuthorize)
	r.GET("/auth/gitee/callback", h.GiteeCallback)
	r.POST("/auth/gitee/callback", h.GiteeCallback)
}
