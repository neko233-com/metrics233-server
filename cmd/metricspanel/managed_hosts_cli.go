package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"time"

	"github.com/neko233-com/metrics233-server/internal/hostagent"
	"github.com/neko233-com/metrics233-server/internal/model"
)

func managedHostsCommand(client apiClient, action, id, file string, revision int64, artifacts string) error {
	base := "/api/v1/managed-hosts"
	if action == "list" {
		data, err := client.call("GET", base, nil)
		if err != nil {
			return err
		}
		return output(data)
	}
	if action == "save" {
		data, err := readFile(file)
		if err != nil {
			return errors.New("cannot read protected host input")
		}
		var host model.ManagedHost
		if json.Unmarshal(data, &host) != nil {
			return errors.New("invalid host JSON")
		}
		if id != "" {
			if host.ID != "" && host.ID != id {
				return errors.New("host identity mismatch")
			}
			host.ID = id
		}
		if err := host.Validate(); err != nil {
			return err
		}
		path, method := base, "POST"
		if host.ID != "" {
			path += "/" + url.PathEscape(host.ID)
			method = "PUT"
		}
		response, err := client.call(method, path, host)
		if err != nil {
			return err
		}
		return output(response)
	}
	if id == "" {
		return errors.New("--id is required")
	}
	base += "/" + url.PathEscape(id)
	if action == "events" || action == "fingerprint" {
		method, suffix := "GET", "/events"
		if action == "fingerprint" {
			method, suffix = "POST", "/fingerprint"
		}
		data, err := client.call(method, base+suffix, nil)
		if err != nil {
			return err
		}
		return output(data)
	}
	if action != "deploy" && action != "deploy-local" {
		return errors.New("hosts: list | save | fingerprint | deploy | deploy-local | events")
	}
	if revision < 1 {
		return errors.New("--host-version is required; first read hosts list")
	}
	if action == "deploy" {
		data, err := client.call("POST", base+"/deploy", map[string]int64{"version": revision})
		if err != nil {
			return err
		}
		return output(data)
	}
	if artifacts == "" || !filepath.IsAbs(artifacts) {
		return errors.New("--collector-artifacts requires an absolute directory with the trusted binaries and .sha256 manifests")
	}
	data, err := client.call("POST", base+"/deployment-plan", map[string]int64{"version": revision})
	if err != nil {
		return err
	}
	var plan struct {
		Host      model.ManagedHost   `json:"host"`
		Secrets   map[string]string   `json:"secrets"`
		Bootstrap model.HostBootstrap `json:"bootstrap"`
	}
	if json.Unmarshal(data, &plan) != nil {
		return errors.New("invalid protected deployment plan")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := hostagent.Deploy(ctx, plan.Host, plan.Secrets, plan.Bootstrap, artifacts)
	if err != nil {
		return err
	}
	return output(result)
}
