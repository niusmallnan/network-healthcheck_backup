package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/niusmallnan/network-healthcheck/healthcheck"
	"github.com/niusmallnan/network-healthcheck/server"
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher-metadata/metadata"
	"github.com/urfave/cli"
)

var VERSION = "v0.0.0-dev"

func main() {
	app := cli.NewApp()
	app.Name = "network-healthcheck"
	app.Version = VERSION
	app.Usage = "A healthcheck service for Rancher networking"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "metadata-address",
			Usage: "The metadata service address",
			Value: "rancher-metadata",
		},
		cli.IntFlag{
			Name:  "health-check-port",
			Usage: "Port to listen on for healthchecks",
			Value: 9898,
		},
	}
	app.Run(os.Args)
}

func run(c *cli.Context) error {
	if os.Getenv("RANCHER_DEBUG") == "true" {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if os.Getenv("ROUTER_HTTP_CHECK") == "true" {
		server.EnableRouterHTTPCheck()
	}

	mdClient, err := metadata.NewClientAndWait(fmt.Sprintf("http://%s/2016-07-29", c.String("metadata-address")))
	s := server.NewServer(mdClient)

	exit := make(chan error)
	go func(exit chan<- error) {
		err := s.Run()
		exit <- errors.Wrap(err, "Main server exited")
	}(exit)

	go func(exit chan<- error) {
		err := healthcheck.StartHealthCheck(c.Int("health-check-port"), s, mdClient)
		exit <- errors.Wrapf(err, "Healthcheck provider died.")
	}(exit)

	err = <-exit
	logrus.Errorf("Exiting network-healthcheck with error: %v", err)
	return err
}
