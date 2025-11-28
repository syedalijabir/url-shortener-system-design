package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	proto "github.com/syedalijabir/protos/storage-service"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type storageServer struct {
	proto.UnimplementedStorageServiceServer
	db *sql.DB
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func getConfig() Config {
	return Config{
		Host:     getEnv("DB_HOST", "postgres"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "password"),
		DBName:   getEnv("DB_NAME", "urlshortener"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func NewStorageServer() (*storageServer, error) {
	config := getConfig()

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode)

	// Wait for PostgreSQL to be ready
	var db *sql.DB
	var err error
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Printf("Failed to connect to PostgreSQL (attempt %d/10): %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}

		err = db.Ping()
		if err == nil {
			break
		}
		log.Printf("Failed to ping PostgreSQL (attempt %d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL after retries: %v", err)
	}

	log.Println("PostgreSQL storage initialized successfully")
	return &storageServer{db: db}, nil
}

func (s *storageServer) SaveURL(ctx context.Context, req *proto.SaveURLRequest) (*proto.SaveURLResponse, error) {
	log.Printf("Storage SaveURL request for: %s -> %s", req.ShortCode, req.OriginalUrl)

	// Use UPSERT (INSERT ON CONFLICT) to handle duplicates
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO urls (short_code, original_url, created_at, updated_at) 
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (short_code) 
		DO UPDATE SET 
			original_url = EXCLUDED.original_url,
			updated_at = EXCLUDED.updated_at
	`, req.ShortCode, req.OriginalUrl, time.Now())

	if err != nil {
		log.Printf("Failed to save URL to PostgreSQL: %v", err)
		return &proto.SaveURLResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	log.Printf("URL saved successfully to PostgreSQL: %s", req.ShortCode)
	return &proto.SaveURLResponse{
		Success: true,
	}, nil
}

func (s *storageServer) GetURL(ctx context.Context, req *proto.GetURLRequest) (*proto.GetURLResponse, error) {
	log.Printf("Storage GetURL request for: %s", req.ShortCode)

	var originalURL string
	var clickCount int64
	var createdAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT original_url, click_count, created_at 
		FROM urls 
		WHERE short_code = $1
	`, req.ShortCode).Scan(&originalURL, &clickCount, &createdAt)

	if err == sql.ErrNoRows {
		log.Printf("URL not found in PostgreSQL: %s", req.ShortCode)
		return &proto.GetURLResponse{
			Found: false,
		}, nil
	} else if err != nil {
		log.Printf("PostgreSQL error: %v", err)
		return &proto.GetURLResponse{
			Found: false,
			Error: err.Error(),
		}, nil
	}

	log.Printf("URL found in PostgreSQL: %s -> %s", req.ShortCode, originalURL)
	return &proto.GetURLResponse{
		OriginalUrl: originalURL,
		Found:       true,
	}, nil
}

func (s *storageServer) IncrementClick(ctx context.Context, req *proto.IncrementClickRequest) (*proto.IncrementClickResponse, error) {
	log.Printf("Storage IncrementClick request for: %s", req.ShortCode)

	result, err := s.db.ExecContext(ctx, `
		UPDATE urls 
		SET click_count = click_count + 1, updated_at = $1
		WHERE short_code = $2
	`, time.Now(), req.ShortCode)

	if err != nil {
		log.Printf("Failed to increment click count: %v", err)
		return &proto.IncrementClickResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return &proto.IncrementClickResponse{
			Success: false,
			Error:   "URL not found",
		}, nil
	}

	log.Printf("Click count incremented in PostgreSQL for %s", req.ShortCode)
	return &proto.IncrementClickResponse{
		Success: true,
	}, nil
}

func (s *storageServer) GetStats(ctx context.Context, req *proto.GetStatsRequest) (*proto.GetStatsResponse, error) {
	log.Printf("Storage GetStats request for: %s", req.ShortCode)

	var originalURL string
	var clickCount int64
	var createdAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT original_url, click_count, created_at 
		FROM urls 
		WHERE short_code = $1
	`, req.ShortCode).Scan(&originalURL, &clickCount, &createdAt)

	if err == sql.ErrNoRows {
		return &proto.GetStatsResponse{
			Error: "URL not found",
		}, nil
	} else if err != nil {
		return &proto.GetStatsResponse{
			Error: err.Error(),
		}, nil
	}

	return &proto.GetStatsResponse{
		ShortCode:  req.ShortCode,
		ClickCount: clickCount,
		CreatedAt:  createdAt.Format(time.RFC3339),
	}, nil
}

func (s *storageServer) Close() error {
	return s.db.Close()
}

func (s *storageServer) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "storage-service",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func main() {
	storageServer, err := NewStorageServer()
	if err != nil {
		log.Fatalf("Failed to create storage server: %v", err)
	}
	defer storageServer.Close()

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	proto.RegisterStorageServiceServer(server, storageServer)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("url.URLService", grpc_health_v1.HealthCheckResponse_SERVING)

	log.Printf("Storage Service with PostgreSQL starting on :50053")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
