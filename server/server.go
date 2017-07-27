package server

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher-metadata/metadata"
	fastping "github.com/tatsushid/go-fastping"
)

const (
	pingCount     = 5
	checkInterval = 10 * time.Second
	pingMaxRTT    = time.Second
)

type Peer struct {
	UUID      string
	SourceIP  string
	DestIP    string
	IsRouter  bool
	Reachable bool
}

func (p *Peer) pingCheck() {
	result := map[string]int{}
	for i := 1; i <= pingCount; i++ {
		ping := fastping.NewPinger()
		ping.Network("ip")
		ping.AddIP(p.DestIP)
		ping.MaxRTT = pingMaxRTT
		ping.OnRecv = func(ip *net.IPAddr, d time.Duration) {
			if ip.String() == p.DestIP {
				logrus.Debugf("Received a ping check from %s, %d/%d", p.DestIP, i, pingCount)
				result[p.DestIP] = result[p.DestIP] + 1
			}
		}
		logrus.Debugf("Do a ping check to %s, %d/%d", p.DestIP, i, pingCount)
		err := ping.Run()
		if err != nil {
			logrus.Errorf("Failed to ping %s, %v", p.DestIP, err)
		}
	}
	if result[p.DestIP] < pingCount {
		logrus.Warnf("Lose ping data from %s", p.DestIP)
		p.Reachable = false
	}
}

func (p *Peer) httpCheck() {
	if p.IsRouter {
		logrus.Debugf("Do a http check for %s", p.DestIP)
		resp, err := http.Get(fmt.Sprintf("http://%s:8111/ping", p.DestIP))
		if err != nil || resp.StatusCode != http.StatusOK {
			p.Reachable = false
		}
		defer resp.Body.Close()
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
