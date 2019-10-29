package main

import (
	"fmt"
	threescale_authorizer "github.com/3scale/3scale-authorizer/pkg/authorization"
	"github.com/3scale/3scale-authorizer/pkg/envoy"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale"
	authZ "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"net"
	"time"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

func main() {

	port := 3333
	proxyCache := threescale.NewProxyConfigCache(1*time.Minute, 30*time.Second, 3, 1000)
	err := proxyCache.StartRefreshWorker()
	if err != nil {
		panic(err)
	}
	authorizer := threescale_authorizer.NewAuthorizer(proxyCache)

	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	ea := envoy.EnvoyAuth{
		Server:     *grpcServer,
		Authorizer: *authorizer,
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Println("Failed to listen")
	}

	authZ.RegisterAuthorizationServer(grpcServer, ea)

	log.Printf("Starting Authorization Service on Port %d\n", port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Error(err)
	}

}
