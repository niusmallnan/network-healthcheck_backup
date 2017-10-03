package server

import (
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher-metadata/metadata"
	"golang.org/x/sync/syncmap"
)

func getRoutersInfo(mc metadata.Client) (map[string]metadata.Network, map[string]metadata.Container, error) {
	networks, err := mc.GetNetworks()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching networks from metadata")
	}

	host, err := mc.GetSelfHost()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching self host from metadata")
	}

	services, err := mc.GetServices()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching services from metadata")
	}

	ret := map[string]metadata.Network{}
	for _, aNetwork := range networks {
		if aNetwork.EnvironmentUUID != host.EnvironmentUUID {
			continue
		}
		_, ok := aNetwork.Metadata["cniConfig"].(map[string]interface{})
		if !ok {
			continue
		}
		ret[aNetwork.UUID] = aNetwork
	}

	if len(ret) == 0 {
		return nil, nil, nil
	}

	routers := map[string]metadata.Container{}
	for _, service := range services {
		if !(service.Kind == "networkDriverService" &&
			service.Name == service.PrimaryServiceName) {
			continue
		}

		for _, aContainer := range service.Containers {
			routers[aContainer.UUID] = aContainer
		}
	}

	return ret, routers, nil
}

func getSyncMapLength(m *syncmap.Map) int {
	// https://github.com/golang/go/issues/20680
	length := 0
	m.Range(func(_, _ interface{}) bool {
		length++
		return true
	})
	return length
}
