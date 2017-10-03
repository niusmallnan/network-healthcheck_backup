package healthcheck

import (
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/niusmallnan/network-healthcheck/server"
	"github.com/rancher/go-rancher-metadata/metadata"
)

func StartHealthCheck(listen int, s *server.Server, mc metadata.Client) error {
	http.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		healthy := true
		_, err := mc.GetVersion()
		if err != nil {
			healthy = false
			logrus.Error("Metadata and dns is unreachable")
		}

		s.GetPeers().Range(func(_, v interface{}) bool {
			p := v.(*server.Peer)
			if !p.Reachable {
				healthy = false
				logrus.Errorf("From %s to %s is unreachable, isRouter: %t, UUID: %s", p.SourceIP, p.DestIP, p.IsRouter, p.UUID)
				return false
			}
			return true
		})

		if healthy {
			fmt.Fprint(w, "ok")
		} else {
			http.Error(w, "Network healthcheck error", http.StatusNotFound)
		}
	})
	logrus.Infof("Listening for health checks on 0.0.0.0:%d/healthcheck", listen)
	err := http.ListenAndServe(fmt.Sprintf(":%d", listen), nil)
	return err
}
