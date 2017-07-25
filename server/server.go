package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher-metadata/metadata"
	ping "github.com/sparrc/go-ping"
)

const (
	pingCount     = 5
	checkInterval = 30 * time.Second
	pingTimeout   = 10 * time.Second
)

type Peer struct {
	UUID      string
	SourceIP  string
	DestIP    string
	IsRouter  bool
	Reachable bool
}

func (p *Peer) pingCheck() {
	logrus.Debugf("Do a ping check for %s", p.DestIP)
	pinger, err := ping.NewPinger(p.DestIP)
	if err != nil {
		logrus.Error(errors.Wrap(err, "Failed to NewPinger"))
		p.Reachable = false
	}
	pinger.SetPrivileged(true)
	pinger.Count = pingCount
	pinger.Timeout = pingTimeout
	pinger.OnFinish = func(stats *ping.Statistics) {
		logrus.Debugf("Finish a ping check for %s", p.DestIP)
		if stats.PacketLoss > 0 {
			p.Reachable = false
		}
	}
	pinger.Run()
}

func (p *Peer) httpCheck() {
	if p.IsRouter {
		logrus.Debugf("Do a http check for %s", p.DestIP)
		resp, err := http.Get(fmt.Sprintf("http://%s:8111/ping", p.DestIP))
		if err != nil || resp.StatusCode != http.StatusOK {
			p.Reachable = false
		}
		resp.Body.Close()
		logrus.Debugf("Finish a http check for %s", p.DestIP)
	}
}

type Server struct {
	mc    metadata.Client
	peers map[string]*Peer
}

func NewServer(mc metadata.Client) *Server {
	return &Server{mc, map[string]*Peer{}}
}

func (s *Server) Run() error {
	for {
		existContainers, err := s.calculatePeers()
		if err != nil {
			return err
		}
		s.checkPeers(existContainers)
		logrus.Debugf("Sleep checking...wait...")
		time.Sleep(checkInterval)
	}
}

func (s *Server) calculatePeers() (map[string]bool, error) {
	self, err := s.mc.GetSelfContainer()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get self container")
	}

	containers, err := s.mc.GetContainers()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get containers")
	}

	locaNetworks, routers, err := getRoutersInfo(s.mc)

	existContainers := map[string]bool{}
	for _, c := range containers {
		if c.PrimaryIp == "" || c.PrimaryIp == self.PrimaryIp {
			continue
		}
		if _, ok := locaNetworks[c.NetworkUUID]; !ok {
			continue
		}

		_, isRouter := routers[c.UUID]
		if !isRouter && c.State != "running" {
			// force check router containers
			continue
		}

		s.peers[c.PrimaryIp] = &Peer{
			UUID:      c.UUID,
			SourceIP:  self.PrimaryIp,
			DestIP:    c.PrimaryIp,
			Reachable: true,
			IsRouter:  isRouter,
		}
		existContainers[c.PrimaryIp] = true
	}

	return existContainers, nil
}

func (s *Server) checkPeers(existContainers map[string]bool) {
	legacyContainers := map[string]bool{}
	for destIP, p := range s.peers {
		if _, ok := existContainers[destIP]; ok {
			p.pingCheck()
			if p.IsRouter {
				p.httpCheck()
			}
		} else {
			legacyContainers[destIP] = true
		}
	}

	for destIP := range legacyContainers {
		delete(s.peers, destIP)
	}
}

func (s *Server) GetPeers() map[string]*Peer {
	return s.peers
}
