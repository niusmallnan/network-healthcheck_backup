package server

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/j-keck/arping"
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher-metadata/metadata"
	fastping "github.com/tatsushid/go-fastping"
	"golang.org/x/sync/syncmap"
)

const (
	pingCount     = 5
	checkInterval = 10 * time.Second
	commonTimeout = time.Second
)

var (
	routerHTTPCheck = false
)

type Peer struct {
	UUID      string
	SourceIP  string
	DestIP    string
	IsRouter  bool
	Reachable bool
}

func (p *Peer) pingCheck() {
	receivedCount := 0
	for i := 1; i <= pingCount; i++ {
		ping := fastping.NewPinger()
		ping.Network("ip")
		ping.AddIP(p.DestIP)
		ping.MaxRTT = commonTimeout
		ping.OnRecv = func(ip *net.IPAddr, d time.Duration) {
			if ip.String() == p.DestIP {
				logrus.Debugf("Received a ping reply from %s, %d/%d", p.DestIP, i, pingCount)
				receivedCount++
			}
		}
		logrus.Debugf("Do a ping check to %s, %d/%d", p.DestIP, i, pingCount)
		err := ping.Run()
		if err != nil {
			logrus.Errorf("Failed to ping %s, %v", p.DestIP, err)
		}
	}
	if receivedCount < pingCount {
		logrus.Warnf("Lose ping data from %s", p.DestIP)
		p.Reachable = false
	} else {
		p.Reachable = true
	}
}

func (p *Peer) arpingCheck() {
	result := map[string]bool{}
	for i := 1; i <= pingCount; i++ {
		logrus.Debugf("Do an arping check to %s, %d/%d", p.DestIP, i, pingCount)
		hwAddr, _, err := arping.Ping(net.ParseIP(p.DestIP))
		if err != nil {
			logrus.Errorf("Failed to arping %s, %v", p.DestIP, err)
			continue
		}
		if hwAddr.String() != "" {
			result[hwAddr.String()] = true
		}
		logrus.Debugf("Received an arping reply from %s, mac: %s, %d/%d", p.DestIP, hwAddr.String(), i, pingCount)
		time.Sleep(commonTimeout)
	}
	if len(result) > 1 {
		logrus.Warnf("Get multiple MAC from %s", p.DestIP)
		p.Reachable = false
	} else {
		p.Reachable = true
	}
}

func (p *Peer) httpCheck() {
	if p.IsRouter {
		logrus.Debugf("Do a http check for %s", p.DestIP)
		resp, err := http.Get(fmt.Sprintf("http://%s:8111/ping", p.DestIP))
		if err != nil || resp.StatusCode != http.StatusOK {
			logrus.Warnf("Router container %s is unreachable", p.DestIP)
			p.Reachable = false
		} else {
			p.Reachable = true
		}
		if resp != nil {
			resp.Body.Close()
		}
		logrus.Debugf("Finish a http check for %s", p.DestIP)
	}
}

type Server struct {
	mc               metadata.Client
	peers            *syncmap.Map
	unreachablePeers *syncmap.Map
}

func NewServer(mc metadata.Client) *Server {
	return &Server{
		mc:               mc,
		peers:            new(syncmap.Map),
		unreachablePeers: new(syncmap.Map),
	}
}

func (s *Server) RunLoop() error {
	for {
		logrus.Debugf("Start main loop checking...")
		existContainers, err := s.calculatePeers()
		if err != nil {
			return err
		}
		s.checkPeers(existContainers)
		logrus.Debugf("Sleep main loop checking...wait...")
		time.Sleep(checkInterval)
	}
}

func (s *Server) RunRetryLoop() error {
	for {
		time.Sleep(checkInterval)
		logrus.Debugf("Start retry loop checking...")
		if getSyncMapLength(s.unreachablePeers) == 0 {
			logrus.Debugf("Nothing to do in retry loop...")
			continue
		}

		containersMap := make(map[string]bool)
		containers, err := s.mc.GetContainers()
		if err != nil {
			return errors.Wrap(err, "Failed to get containers")
		}
		for _, c := range containers {
			containersMap[c.UUID] = true
		}

		s.unreachablePeers.Range(func(k, v interface{}) bool {
			destIP := k.(string)
			p := v.(*Peer)
			if _, ok := containersMap[p.UUID]; !ok {
				s.peers.Delete(destIP)
				s.unreachablePeers.Delete(destIP)
				return true
			}
			p.pingCheck()
			p.arpingCheck()
			if routerHTTPCheck {
				p.httpCheck()
			}

			if p.Reachable {
				s.unreachablePeers.Delete(destIP)
			}

			return true
		})
		logrus.Debugf("Sleep retry loop checking...wait...")
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

		p := &Peer{
			UUID:      c.UUID,
			SourceIP:  self.PrimaryIp,
			DestIP:    c.PrimaryIp,
			Reachable: true,
			IsRouter:  isRouter,
		}
		s.peers.Store(c.PrimaryIp, p)

		existContainers[c.PrimaryIp] = true
	}

	return existContainers, nil
}

func (s *Server) checkPeers(existContainers map[string]bool) {
	s.peers.Range(func(k, v interface{}) bool {
		destIP := k.(string)
		if _, ok := existContainers[destIP]; !ok {
			s.peers.Delete(destIP)
			return true
		}
		p := v.(*Peer)
		p.pingCheck()
		p.arpingCheck()
		if routerHTTPCheck {
			p.httpCheck()
		}

		if !p.Reachable {
			s.unreachablePeers.Store(destIP, p)
		}

		return true
	})
}

func (s *Server) GetPeers() *syncmap.Map {
	return s.peers
}

func EnableRouterHTTPCheck() {
	routerHTTPCheck = true
}
