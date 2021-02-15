package generator

import (
	"bytes"
	"context"
	"fmt"
	"net"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/suchwerk/caddy-docker-proxy/plugin/v2/caddyfile"
)

func (g *CaddyfileGenerator) getServiceCaddyfile(service *swarm.Service, logsBuffer *bytes.Buffer) (*caddyfile.Container, error) {
	caddyLabels := g.filterLabels(service.Spec.Labels)

	return labelsToCaddyfile(caddyLabels, service, func() ([]string, error) {
		return g.getServiceProxyTargets(service, logsBuffer, true)
	})
}

func (g *CaddyfileGenerator) getServiceProxyTargets(service *swarm.Service, logsBuffer *bytes.Buffer, ingress bool) ([]string, error) {
	if g.options.ProxyServiceTasks {
		return g.getServiceTasksIps(service, logsBuffer, ingress)
	}

	_, err := g.getServiceVirtualIps(service, logsBuffer, ingress)
	if err != nil {
		return nil, err
	}

	return []string{service.Spec.Name}, nil
}

func (g *CaddyfileGenerator) getServiceVirtualIps(service *swarm.Service, logsBuffer *bytes.Buffer, ingress bool) ([]string, error) {
	virtualIps := []string{}

	for _, virtualIP := range service.Endpoint.VirtualIPs {
		if !ingress || g.ingressNetworks[virtualIP.NetworkID] {
			virtualIps = append(virtualIps, virtualIP.Addr)
		}
	}

	if len(virtualIps) == 0 {
		logsBuffer.WriteString(fmt.Sprintf("[WARNING] Service %v and caddy are not in same network\n", service.ID))
	}

	return virtualIps, nil
}

func (g *CaddyfileGenerator) getServiceTasksIps(service *swarm.Service, logsBuffer *bytes.Buffer, ingress bool) ([]string, error) {
	taskListFilter := filters.NewArgs()
	taskListFilter.Add("service", service.ID)
	taskListFilter.Add("desired-state", "running")

	tasks, err := g.dockerClient.TaskList(context.Background(), types.TaskListOptions{Filters: taskListFilter})
	if err != nil {
		return []string{}, err
	}

	hasRunningTasks := false
	tasksIps := []string{}
	for _, task := range tasks {
		if task.Status.State == swarm.TaskStateRunning {
			hasRunningTasks = true
			for _, networkAttachment := range task.NetworksAttachments {
				if !ingress || g.ingressNetworks[networkAttachment.Network.ID] {
					for _, address := range networkAttachment.Addresses {
						ipAddress, _, _ := net.ParseCIDR(address)
						tasksIps = append(tasksIps, ipAddress.String())
					}
				}
			}
		}
	}

	if !hasRunningTasks {
		logsBuffer.WriteString(fmt.Sprintf("[WARNING] Service %v doesn't have any task in running state\n", service.ID))
	} else if len(tasksIps) == 0 {
		logsBuffer.WriteString(fmt.Sprintf("[WARNING] Service %v and caddy are not in same network\n", service.ID))
	}

	return tasksIps, nil
}
