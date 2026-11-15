package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	hrv1 "github.com/aymane-belassiria/hr-platform/gen/go/hr/v1"
	"github.com/aymane-belassiria/hr-platform/services/gateway/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	grpcAddr := env("EMPLOYEE_GRPC_ADDR", "localhost:50051")
	httpAddr := env("HTTP_ADDR", ":8080")

	//grpc connection

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial employee: %v", err)
	}

	defer conn.Close()

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           api.New(hrv1.NewEmployeeServiceClient(conn)).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("gateway listening on %s (employee=%s)", httpAddr, grpcAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()
	<-ctx.Done()
	log.Println("shuting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
