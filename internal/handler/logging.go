package handler

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InitLogger configures structured JSON logging for production.
// In development mode, uses human-readable text format.
func InitLogger(devMode bool) {
	var handler slog.Handler
	if devMode {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))
}

// RequestIDKey is the context key for the request ID.
const RequestIDKey = "request_id"

// RequestID returns middleware that ensures every request has a unique ID.
// It reads X-Request-ID from the incoming request (set by nginx), or generates
// a new UUID. The ID is stored in the Gin context and set as a response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// GetRequestID extracts the request ID from the context.
func GetRequestID(c *gin.Context) string {
	if id, ok := c.Get(RequestIDKey); ok {
		return id.(string)
	}
	return ""
}

// StructuredLogging returns middleware that logs each request in JSON format
// with method, path, status, duration, request ID, and client IP.
func StructuredLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Log after request completes
		duration := time.Since(start)
		status := c.Writer.Status()
		requestID := GetRequestID(c)
		method := c.Request.Method
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		bodySize := c.Writer.Size()

		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.Int("status", status),
			slog.Duration("duration", duration),
			slog.String("client_ip", clientIP),
			slog.String("user_agent", userAgent),
			slog.Int("body_size", bodySize),
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(context.Background(), level, "request", attrs...)
	}
}
