package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	hrv1 "github.com/aymane-belassiria/hr-platform/gen/go/hr/v1"
	db "github.com/aymane-belassiria/hr-platform/services/employee/db/gen"
	"github.com/aymane-belassiria/hr-platform/services/employee/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := env("DATABASE_URL", "postgres://hr:hr@localhost:54322/employee?sslmode=disable")
	addr := env("GRPC_ADDR", ":50051")

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}

	srv := grpc.NewServer()
	hrv1.RegisterEmployeeServiceServer(srv, service.New(db.New(pool)))
	reflection.Register(srv)

	lis, err := net.Listen("tcp", addr)

	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		srv.Stop()
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
