package main

import (
	log "github.com/Sirupsen/logrus"
	"os"
)

func main() {
	InitLogging(config.Verbose)
	log.Info("Starting PDP server")

	pdp := NewServer(config.CWD)

	if pdp == nil {
		log.Error("Failed to create Server.")
		os.Exit(1)
	}

	if pdp.LoadPolicies(config.Policy) != true {
		log.Error("Failed to Load Policies.")
		os.Exit(1)
	}

	if pdp.ListenRequests(config.ServiceEP) != true {
		log.Error("Failed to Listen to Requests.")
		os.Exit(1)
	}
	if pdp.ListenControl(config.ControlEP) != true {
		log.Error("Failed to Listen to Control Packets.")
		os.Exit(1)
	}

	tracer, err := InitTracing("zipkin", config.TracingEP)
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Warning("Could not initialize tracing.")
	}
	pdp.Serve(tracer)
}
