package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"wegugin/config"
	pb "wegugin/genproto/cruds"
	"wegugin/logs"
	"wegugin/middleware"
	"wegugin/service"
	"wegugin/storage/postgres"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runGatewayServer(grpcAddr string) {
	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// 1. Casbin Enforcer ni ishga tushirish
	enforcer, err := casbin.NewEnforcer("casbin/model.conf", "casbin/policy.csv")
	if err != nil {
		log.Fatalf("Failed to create casbin enforcer: %v", err)
	}

	// 2. Middleware ni yaratish
	authMiddleware := middleware.AuthMiddleware(enforcer)

	// 3. gRPC Gateway handlerini middleware bilan o'rab olish
	wrappedMux := authMiddleware(mux)

	// CRUD service registratsiyasi
	if err := pb.RegisterCrudsServiceHandlerFromEndpoint(
		ctx,
		mux,
		grpcAddr,
		opts,
	); err != nil {
		log.Fatalf("Failed to register gateway: %v", err)
	}

	// Gin serverini sozlash
	r := gin.Default()
	r.Any("/v1/*any", gin.WrapH(wrappedMux))

	// Swagger UI
	r.StaticFS("/swagger-ui", http.Dir("./doc/swagger"))

	log.Println(fmt.Sprintf("HTTP gateway running on %s", config.Load().Server.CRUD_SERVER))
	if err := r.Run(config.Load().Server.CRUD_SERVER); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

func runGRPCServer(store postgres.Store) {

	listener, err := net.Listen("tcp", config.Load().Server.CRUD_SERVICE)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterCrudsServiceServer(s, service.NewService(store, logs.NewLogger()))

	log.Println(fmt.Sprintf("gRPC server running on%s", config.Load().Server.CRUD_SERVICE))
	if err := s.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func main() {
	ctx := context.Background()
	store, err := postgres.ConnectionDb(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	go func() {
		runGRPCServer(store)
	}()
	time.Sleep(time.Second * 2) // 2 soniya kutish
	runGatewayServer(config.Load().Server.CRUD_SERVICE)

}
