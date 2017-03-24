package main

//go:generate bash -c "mkdir -p $GOPATH/src/github.com/infobloxopen/policy-box/pdp-service && protoc -I $GOPATH/src/github.com/infobloxopen/policy-box/proto/ $GOPATH/src/github.com/infobloxopen/policy-box/proto/service.proto --go_out=plugins=grpc:$GOPATH/src/github.com/infobloxopen/policy-box/pdp-service && ls $GOPATH/src/github.com/infobloxopen/policy-box/pdp-service"

//go:generate bash -c "mkdir -p $GOPATH/src/github.com/infobloxopen/policy-box/pdp-control && protoc -I $GOPATH/src/github.com/infobloxopen/policy-box/proto/ $GOPATH/src/github.com/infobloxopen/policy-box/proto/control.proto --go_out=plugins=grpc:$GOPATH/src/github.com/infobloxopen/policy-box/pdp-control && ls $GOPATH/src/github.com/infobloxopen/policy-box/pdp-control"

import (
	"net"
	"sync"

	log "github.com/Sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"

	pbc "github.com/infobloxopen/policy-box/pdp-control"
	pbs "github.com/infobloxopen/policy-box/pdp-service"

	"github.com/infobloxopen/policy-box/pdp"

	ot "github.com/opentracing/opentracing-go"
)

type Transport struct {
	Interface net.Listener
	Protocol  *grpc.Server
}

type Server struct {
	Path   string
	Policy pdp.EvaluableType
	Lock   *sync.RWMutex

	Requests Transport
	Control  Transport

	Updates *Queue
}

func NewServer(path string) *Server {
	return &Server{Path: path, Lock: &sync.RWMutex{}, Updates: NewQueue()}
}

func (s *Server) LoadPolicies(path string) bool {
	if len(path) == 0 {
		log.Error("Invalid path specified. Failed to Load Policies.")
		return false
	}

	log.WithField("policy", path).Info("Loading policy")
	p, err := pdp.UnmarshalYASTFromFile(path, s.Path)
	if err != nil {
		log.WithFields(log.Fields{"policy": path, "error": err}).Error("Failed load policy")
		return false
	}

	s.Policy = p
	return true
}

func (s *Server) ListenRequests(addr string) bool {
	log.WithField("address", addr).Info("Opening service port")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.WithFields(log.Fields{"address": addr, "error": err}).Fatal("Failed to open service port")
		return false
	}

	s.Requests.Interface = ln
	return true
}

func (s *Server) ListenControl(addr string) bool {
	log.WithField("address", addr).Info("Opening control port")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.WithFields(log.Fields{"address": addr, "error": err}).Fatal("Failed to open control port")
		return false
	}

	s.Control.Interface = ln
	return true
}

func (s *Server) Serve(tracer ot.Tracer) bool{
	go func() bool {
		log.Info("Creating control protocol handler")
		s.Control.Protocol = grpc.NewServer()
		if s.Control.Protocol == nil {
			log.Error("Failed to create GRPC Protocol server.")
			return false

		}
		pbc.RegisterPDPControlServer(s.Control.Protocol, s)

		log.Info("Serving control requests")
		s.Control.Protocol.Serve(s.Control.Interface)
		return true
	}()

	log.Info("Creating service protocol handler")
	if tracer == nil {
		s.Requests.Protocol = grpc.NewServer()
		if s.Requests.Protocol == nil {
			log.Error("Failed to create service GRPC server.")
			return false
		}
	} else {
		onlyIfParent := func(parentSpanCtx ot.SpanContext, method string, req, resp interface{}) bool {
			return parentSpanCtx != nil
		}
		intercept := otgrpc.OpenTracingServerInterceptor(tracer, otgrpc.IncludingSpans(onlyIfParent))
		s.Requests.Protocol = grpc.NewServer(grpc.UnaryInterceptor(intercept))
		if s.Requests.Protocol== nil {
			log.Error("Failed to create GRPC interceptor server.")
			return false
		}
	}
	pbs.RegisterPDPServer(s.Requests.Protocol, s)

	log.Info("Serving decision requests")
	s.Requests.Protocol.Serve(s.Requests.Interface)
	return true
}
