package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	cache_service "github.com/syedalijabir/protos/cache-service"
	storage_service "github.com/syedalijabir/protos/storage-service"
	url_service "github.com/syedalijabir/protos/url-service"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type urlServer struct {
	url_service.UnimplementedURLServiceServer
	mu            sync.RWMutex
	urls          map[string]string // In-memory cache
	stats         map[string]int64
	createdAt     map[string]time.Time
	cacheClient   cache_service.CacheServiceClient
	storageClient storage_service.StorageServiceClient
}

func NewURLServer() (*urlServer, error) {
	cacheHost := getEnv("CACHE_SERVICE_HOST", "cache-service")
	storageHost := getEnv("STORAGE_SERVICE_HOST", "storage-service")

	cacheConn, err := grpc.Dial(cacheHost+":50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	storageConn, err := grpc.Dial(storageHost+":50053", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &urlServer{
		urls:          make(map[string]string),
		stats:         make(map[string]int64),
		createdAt:     make(map[string]time.Time),
		cacheClient:   cache_service.NewCacheServiceClient(cacheConn),
		storageClient: storage_service.NewStorageServiceClient(storageConn),
	}, nil
}

func (s *urlServer) ShortenURL(ctx context.Context, req *url_service.ShortenRequest) (*url_service.ShortenResponse, error) {
	log.Printf("ShortenURL request for: %s", req.OriginalUrl)

	shortCode := generateShortCode()
	if req.CustomAlias != "" {
		shortCode = req.CustomAlias
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[shortCode]; exists {
		return &url_service.ShortenResponse{
			Error: "Custom alias already exists",
		}, nil
	}

	s.urls[shortCode] = req.OriginalUrl
	s.stats[shortCode] = 0
	s.createdAt[shortCode] = time.Now()

	// Persist to storage (async)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := s.storageClient.SaveURL(ctx, &storage_service.SaveURLRequest{
			ShortCode:   shortCode,
			OriginalUrl: req.OriginalUrl,
		})
		if err != nil {
			log.Printf("Warning: failed to persist URL to storage: %v", err)
		} else {
			log.Printf("URL persisted to storage: %s", shortCode)
		}
	}()

	// Cache the URL (async)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := s.cacheClient.Set(ctx, &cache_service.SetRequest{
			Key:        shortCode,
			Value:      req.OriginalUrl,
			TtlSeconds: 360, // 6 mins (demo only)
		})
		if err != nil {
			log.Printf("Warning: failed to cache URL: %v", err)
		}
	}()

	log.Printf("Shortened URL created: %s -> %s", shortCode, req.OriginalUrl)

	return &url_service.ShortenResponse{
		ShortCode:   shortCode,
		OriginalUrl: req.OriginalUrl,
	}, nil
}

func (s *urlServer) GetOriginalURL(ctx context.Context, req *url_service.GetOriginalRequest) (*url_service.GetOriginalResponse, error) {
	log.Printf("GetOriginalURL request for: %s", req.ShortCode)

	// 1. First try cache (fastest)
	cacheResp, err := s.cacheClient.Get(ctx, &cache_service.GetRequest{Key: req.ShortCode})
	if err == nil && cacheResp.Found {
		log.Printf("Cache hit for: %s", req.ShortCode)
		s.incrementStats(req.ShortCode) // Update stats
		return &url_service.GetOriginalResponse{
			OriginalUrl: cacheResp.Value,
			Found:       true,
		}, nil
	}

	// 2. Try in-memory store
	s.mu.RLock()
	originalURL, exists := s.urls[req.ShortCode]
	s.mu.RUnlock()

	if exists {
		log.Printf("Memory hit for: %s", req.ShortCode)
		// Warm the cache for next time
		go s.warmCache(req.ShortCode, originalURL)
		s.incrementStats(req.ShortCode)
		return &url_service.GetOriginalResponse{
			OriginalUrl: originalURL,
			Found:       true,
		}, nil
	}

	// 3. Try persistent storage (slowest)
	storageResp, err := s.storageClient.GetURL(ctx, &storage_service.GetURLRequest{ShortCode: req.ShortCode})
	if err == nil && storageResp.Found {
		log.Printf("Storage hit for: %s", req.ShortCode)

		s.mu.Lock()
		s.urls[req.ShortCode] = storageResp.OriginalUrl
		s.stats[req.ShortCode] = 0 // Will load actual stats if needed
		s.createdAt[req.ShortCode] = time.Now()
		s.mu.Unlock()

		go s.warmCache(req.ShortCode, storageResp.OriginalUrl)
		s.incrementStats(req.ShortCode)

		return &url_service.GetOriginalResponse{
			OriginalUrl: storageResp.OriginalUrl,
			Found:       true,
		}, nil
	}

	log.Printf("URL not found: %s", req.ShortCode)
	return &url_service.GetOriginalResponse{
		Found: false,
	}, nil
}

func (s *urlServer) GetURLStats(ctx context.Context, req *url_service.StatsRequest) (*url_service.StatsResponse, error) {
	log.Printf("GetURLStats request for: %s", req.ShortCode)

	s.mu.RLock()
	clickCount, exists := s.stats[req.ShortCode]
	createdAt := s.createdAt[req.ShortCode]
	s.mu.RUnlock()

	if !exists {
		storageResp, err := s.storageClient.GetStats(ctx, &storage_service.GetStatsRequest{ShortCode: req.ShortCode})
		if err == nil && storageResp.Error == "" {
			return &url_service.StatsResponse{
				ShortCode:  req.ShortCode,
				ClickCount: storageResp.ClickCount,
				CreatedAt:  storageResp.CreatedAt,
			}, nil
		}
		return &url_service.StatsResponse{
			Error: "URL not found",
		}, nil
	}

	return &url_service.StatsResponse{
		ShortCode:  req.ShortCode,
		ClickCount: clickCount,
		CreatedAt:  createdAt.Format(time.RFC3339),
	}, nil
}

// Helper methods
func (s *urlServer) incrementStats(shortCode string) {
	s.mu.Lock()
	s.stats[shortCode]++
	s.mu.Unlock()

	// Update storage stats async
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := s.storageClient.IncrementClick(ctx, &storage_service.IncrementClickRequest{
			ShortCode: shortCode,
		})
		if err != nil {
			log.Printf("Warning: failed to update storage stats: %v", err)
		}
	}()
}

func (s *urlServer) warmCache(shortCode, originalURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := s.cacheClient.Set(ctx, &cache_service.SetRequest{
		Key:        shortCode,
		Value:      originalURL,
		TtlSeconds: 3600,
	})
	if err != nil {
		log.Printf("Warning: failed to warm cache: %v", err)
	}
}

func generateShortCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func (s *urlServer) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "url-service",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	urlServer, err := NewURLServer()
	if err != nil {
		log.Fatalf("Failed to create URL server: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	url_service.RegisterURLServiceServer(server, urlServer)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	log.Printf("URL Service starting on :50051")
	log.Printf("Connected to:")
	log.Printf("  - Cache Service: :50052")
	log.Printf("  - Storage Service: :50053")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
