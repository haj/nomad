package consul

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/nomad/structs/config"
)

const (
	allocID          = "12"
	serviceRegPrefix = "test"
	serviceGroupName = "executor"
)

var (
	logger = log.New(os.Stdout, "", log.LstdFlags)
	check1 = structs.ServiceCheck{
		Name:     "check-foo-1",
		Type:     structs.ServiceCheckTCP,
		Interval: 30 * time.Second,
		Timeout:  5 * time.Second,
	}
	service1 = structs.ConsulService{
		Name:      "foo-1",
		Tags:      []string{"tag1", "tag2"},
		PortLabel: "port1",
		Checks: []*structs.ServiceCheck{
			&check1,
		},
	}

	service2 = structs.ConsulService{
		Name:      "foo-2",
		Tags:      []string{"tag1", "tag2"},
		PortLabel: "port2",
	}
)

func TestConsulServiceRegisterServices(t *testing.T) {
	shutdownCh := make(chan struct{})
	cs, err := NewSyncer(&config.ConsulConfig{}, shutdownCh, logger)
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	// Skipping the test if consul isn't present
	if !cs.consulPresent() {
		return
	}
	task := mockTask()
	cs.SetServiceRegPrefix(serviceRegPrefix)
	cs.SetAddrFinder(task.FindHostAndPortFor)
	if err := cs.SyncServices(); err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cs.Shutdown()

	service1 := &structs.ConsulService{Name: task.Name}
	service2 := &structs.ConsulService{Name: task.Name}
	services := []*structs.ConsulService{service1, service2}
	service1.ServiceID = fmt.Sprintf("%s-%s:%s/%s", cs.GenerateServiceID(serviceGroupName, service1), task.Name, allocID)
	service2.ServiceID = fmt.Sprintf("%s-%s:%s/%s", cs.GenerateServiceID(serviceGroupName, service2), task.Name, allocID)

	cs.SetServices(serviceGroupName, services)
	if err := servicesPresent(t, services, cs); err != nil {
		t.Fatalf("err : %v", err)
	}
	// FIXME(sean@)
	// if err := checksPresent(t, []string{check1.Hash(service1ID)}, cs); err != nil {
	// 	t.Fatalf("err : %v", err)
	// }
}

func TestConsulServiceUpdateService(t *testing.T) {
	shutdownCh := make(chan struct{})
	cs, err := NewSyncer(&config.ConsulConfig{}, shutdownCh, logger)
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	// Skipping the test if consul isn't present
	if !cs.consulPresent() {
		return
	}

	task := mockTask()
	cs.SetServiceRegPrefix(serviceRegPrefix)
	cs.SetAddrFinder(task.FindHostAndPortFor)
	if err := cs.SyncServices(); err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cs.Shutdown()

	//Update Service defn 1
	newTags := []string{"tag3"}
	task.ConsulServices[0].Tags = newTags
	if err := cs.SyncServices(); err != nil {
		t.Fatalf("err: %v", err)
	}
	// Make sure all the services and checks are still present
	service1 := &structs.ConsulService{Name: task.Name}
	service2 := &structs.ConsulService{Name: task.Name}
	services := []*structs.ConsulService{service1, service2}
	service1.ServiceID = fmt.Sprintf("%s-%s:%s/%s", cs.GenerateServiceID(serviceGroupName, service1), task.Name, allocID)
	service2.ServiceID = fmt.Sprintf("%s-%s:%s/%s", cs.GenerateServiceID(serviceGroupName, service2), task.Name, allocID)
	if err := servicesPresent(t, services, cs); err != nil {
		t.Fatalf("err : %v", err)
	}
	// FIXME(sean@)
	// if err := checksPresent(t, []string{check1.Hash(service1ID)}, cs); err != nil {
	// 	t.Fatalf("err : %v", err)
	// }

	// check if service defn 1 has been updated
	consulServices, err := cs.client.Agent().Services()
	if err != nil {
		t.Fatalf("errL: %v", err)
	}
	srv, _ := consulServices[service1.ServiceID]
	if !reflect.DeepEqual(srv.Tags, newTags) {
		t.Fatalf("expected tags: %v, actual: %v", newTags, srv.Tags)
	}
}

func servicesPresent(t *testing.T, configuredServices []*structs.ConsulService, syncer *Syncer) error {
	var mErr multierror.Error
	services, err := syncer.client.Agent().Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, configuredService := range configuredServices {
		if _, ok := services[configuredService.ServiceID]; !ok {
			mErr.Errors = append(mErr.Errors, fmt.Errorf("service ID %q not synced", configuredService.ServiceID))
		}
	}
	return mErr.ErrorOrNil()
}

func checksPresent(t *testing.T, checkIDs []string, syncer *Syncer) error {
	var mErr multierror.Error
	checks, err := syncer.client.Agent().Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, checkID := range checkIDs {
		if _, ok := checks[checkID]; !ok {
			mErr.Errors = append(mErr.Errors, fmt.Errorf("check ID %q not synced", checkID))
		}
	}
	return mErr.ErrorOrNil()
}

func mockTask() *structs.Task {
	task := structs.Task{
		Name:           "foo",
		ConsulServices: []*structs.ConsulService{&service1, &service2},
		Resources: &structs.Resources{
			Networks: []*structs.NetworkResource{
				&structs.NetworkResource{
					IP: "10.10.11.5",
					DynamicPorts: []structs.Port{
						structs.Port{
							Label: "port1",
							Value: 20002,
						},
						structs.Port{
							Label: "port2",
							Value: 20003,
						},
					},
				},
			},
		},
	}
	return &task
}
