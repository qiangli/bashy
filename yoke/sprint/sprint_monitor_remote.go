package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
)

func init() { observeSprintMonitorRemote = collectRemoteSprintMonitor }

func collectRemoteSprintMonitor(ctx context.Context, opt sprintMonitorOptions) (*sprintMonitorSnapshot, error) {
	client, e := dag.NewCapacityClient()
	if e != nil {
		return nil, e
	}
	raw, e := client.Observe(ctx, opt.Host)
	if e != nil {
		return nil, e
	}
	return decodeRemoteSprintMonitor(raw, opt, time.Now())
}

func decodeRemoteSprintMonitor(raw []byte, opt sprintMonitorOptions, now time.Time) (*sprintMonitorSnapshot, error) {
	var snapshot sprintMonitorSnapshot
	if e := json.Unmarshal(raw, &snapshot); e != nil {
		return nil, e
	}
	if snapshot.SchemaVersion != sprintMonitorSchema || snapshot.At.After(now) || now.Sub(snapshot.At) > 10*time.Second {
		return nil, errors.New("remote monitor schema or freshness mismatch")
	}
	snapshot.Sprint = opt.Sprint
	if opt.Sprint > 0 {
		found := false
		if snapshot.Inventory != nil {
			for _, s := range snapshot.Inventory.Sprints {
				if s.ID == opt.Sprint {
					found = true
					break
				}
			}
			if snapshot.Inventory.Complete && !found {
				return nil, errors.New("selected sprint absent on target")
			}
		}
		var alerts []sprintResourceAlert
		for _, a := range snapshot.Alerts {
			if a.Sprint == opt.Sprint {
				alerts = append(alerts, a)
			}
		}
		snapshot.Alerts = alerts
	}
	if snapshot.Models != nil && (opt.Provider != "" || opt.Model != "") {
		wanted := opt.Model
		if model, ok := fleet.New().Model(wanted); ok {
			wanted = model.Name
		}
		var accounts []llmbudget.AccountReport
		for _, a := range snapshot.Models.Accounts {
			if opt.Provider != "" && !strings.EqualFold(a.Provider, opt.Provider) {
				continue
			}
			match := opt.Model == ""
			for _, m := range a.Models {
				if strings.EqualFold(m, wanted) {
					match = true
				}
			}
			if match {
				accounts = append(accounts, a)
			}
		}
		snapshot.Models.Accounts = accounts
	}
	boundSprintMonitor(&snapshot)
	// Remote observation never evaluates or publishes against local sprint owners.
	snapshot.Origin = opt.Host
	return &snapshot, nil
}
