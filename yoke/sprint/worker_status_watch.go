package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
)

type workerStatusRuntime struct {
	collect func(context.Context) (workerStatusSnapshot, error)
	wait    func(context.Context, time.Duration) error
}

type workerStatusWatchEvent struct {
	SchemaVersion string                `json:"schema_version"`
	Type          string                `json:"type"`
	At            time.Time             `json:"at"`
	Snapshot      *workerStatusSnapshot `json:"snapshot,omitempty"`
	Changes       []workerStatusChange  `json:"changes,omitempty"`
}

type workerStatusChange struct {
	Entity string                 `json:"entity"`
	Key    []string               `json:"key"`
	Field  string                 `json:"field"`
	Metric *workerStatusMetricKey `json:"metric,omitempty"`
	Before any                    `json:"before"`
	After  any                    `json:"after"`
}

type workerStatusMetricKey struct {
	Name   string `json:"name"`
	Window string `json:"window,omitempty"`
	Source string `json:"source"`
	Unit   string `json:"unit"`
}

type workerStatusMetricReading struct {
	Value          *float64   `json:"value"`
	Classification string     `json:"classification"`
	WindowStart    *time.Time `json:"window_start,omitempty"`
	WindowEnd      *time.Time `json:"window_end,omitempty"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
	Limitation     string     `json:"limitation,omitempty"`
}

func watchWorkerStatus(ctx context.Context, out io.Writer, rt workerStatusRuntime, asJSON, diff bool) error {
	var previous *workerStatusSnapshot
	encoder := json.NewEncoder(out)
	for ctx.Err() == nil {
		current, err := rt.collect(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		event := workerStatusWatchEvent{SchemaVersion: workerStatusSchema, At: current.At}
		if !diff || previous == nil {
			event.Type, event.Snapshot = "snapshot", &current
		} else {
			event.Type, event.Changes = "change", diffWorkerStatus(*previous, current)
		}
		if event.Snapshot != nil || len(event.Changes) != 0 {
			if asJSON {
				err = encoder.Encode(event)
			} else {
				err = renderWorkerStatusWatchEvent(out, event)
			}
			if err != nil {
				return err
			}
		}
		if diff {
			previous = &current
		}
		if err := rt.wait(ctx, 5*time.Second); err != nil && ctx.Err() == nil {
			return err
		}
	}
	return nil
}

type workerStatusField struct {
	name   string
	metric *workerStatusMetricKey
	value  any
}

type workerStatusRecord struct {
	entity string
	key    []string
	value  any
	fields map[string]workerStatusField
}

func diffWorkerStatus(previous, current workerStatusSnapshot) []workerStatusChange {
	before, after := workerStatusRecords(previous), workerStatusRecords(current)
	var changes []workerStatusChange
	for _, key := range workerStatusUnionKeys(before, after) {
		old, oldOK := before[key]
		next, nextOK := after[key]
		switch {
		case !oldOK:
			changes = append(changes, workerStatusChange{Entity: next.entity, Key: next.key, Field: "record", After: next.value})
		case !nextOK:
			changes = append(changes, workerStatusChange{Entity: old.entity, Key: old.key, Field: "record", Before: old.value})
		default:
			for _, fieldKey := range workerStatusUnionKeys(old.fields, next.fields) {
				oldField, nextField := old.fields[fieldKey], next.fields[fieldKey]
				if reflect.DeepEqual(oldField.value, nextField.value) {
					continue
				}
				field := nextField
				if field.name == "" {
					field = oldField
				}
				changes = append(changes, workerStatusChange{
					Entity: next.entity, Key: next.key, Field: field.name, Metric: field.metric,
					Before: oldField.value, After: nextField.value,
				})
			}
		}
	}
	return changes
}

func workerStatusRecords(snapshot workerStatusSnapshot) map[string]workerStatusRecord {
	records := make(map[string]workerStatusRecord)
	add := func(entity string, key []string, value any, fields map[string]workerStatusField) {
		index, _ := json.Marshal(append([]string{entity}, key...))
		records[string(index)] = workerStatusRecord{entity: entity, key: key, value: value, fields: fields}
	}
	for _, worker := range snapshot.Workers {
		add("worker", []string{worker.ID}, worker, workerStatusFields(map[string]any{
			"binding": worker.Binding, "task": worker.Task, "state": worker.State, "state_source": worker.StateSource,
			"limit": worker.Limit, "limit_source": worker.LimitSource, "reset_at": workerStatusTime(worker.ResetAt),
		}))
	}
	for _, account := range snapshot.Accounts {
		fields := workerStatusFields(map[string]any{
			"account_known": account.AccountKnown, "status": account.Status, "active_reservations": account.ActiveReservations,
			"agents": slices.Sorted(slices.Values(account.Agents)), "models": slices.Sorted(slices.Values(account.Models)),
			"limitations": slices.Sorted(slices.Values(account.Limitations)), "retry_at": workerStatusTime(account.RetryAt),
		})
		// The same metric name may be reported for several windows, several
		// sources, or several configured limits within a source. Group only
		// matching identities and retain repeated readings without ordering
		// them by capture time.
		groups := make(map[workerStatusMetricKey][]workerStatusMetricReading)
		for _, metric := range account.Metrics {
			key := workerStatusMetricKey{Name: metric.Name, Window: metric.Window, Source: metric.Source, Unit: metric.Unit}
			groups[key] = append(groups[key], workerStatusMetricReading{
				Value: metric.Value, Classification: metric.Classification, Limitation: metric.Limitation,
				WindowStart: workerStatusTime(metric.WindowStart), WindowEnd: workerStatusTime(metric.WindowEnd), ResetAt: workerStatusTime(metric.ResetAt),
			})
		}
		for key, readings := range groups {
			index, _ := json.Marshal(key)
			slices.SortFunc(readings, func(a, b workerStatusMetricReading) int {
				left, _ := json.Marshal(a)
				right, _ := json.Marshal(b)
				return bytes.Compare(left, right)
			})
			fields["metric "+string(index)] = workerStatusField{name: "metric", metric: &key, value: readings}
		}
		add("account", []string{account.Provider, account.Account, account.Pool, string(account.Lane)}, account, fields)
	}
	add("observation", []string{}, nil, workerStatusFields(map[string]any{"warnings": slices.Sorted(slices.Values(snapshot.Warnings))}))
	return records
}

func workerStatusFields(values map[string]any) map[string]workerStatusField {
	fields := make(map[string]workerStatusField, len(values))
	for name, value := range values {
		fields[name] = workerStatusField{name: name, value: value}
	}
	return fields
}

func workerStatusTime(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	normalized := at.UTC()
	return &normalized
}

func workerStatusUnionKeys[T any](a, b map[string]T) []string {
	keys := make(map[string]struct{}, len(a)+len(b))
	for key := range a {
		keys[key] = struct{}{}
	}
	for key := range b {
		keys[key] = struct{}{}
	}
	return slices.Sorted(maps.Keys(keys))
}

func renderWorkerStatusWatchEvent(out io.Writer, event workerStatusWatchEvent) error {
	if _, err := fmt.Fprintf(out, "%s %s\n", event.At.Format(time.RFC3339), event.Type); err != nil {
		return err
	}
	if event.Snapshot != nil {
		return renderWorkerStatus(out, *event.Snapshot)
	}
	var text strings.Builder
	for _, change := range event.Changes {
		key, _ := json.Marshal(change.Key)
		field := change.Field
		if change.Metric != nil {
			identity, _ := json.Marshal(change.Metric)
			field += " " + string(identity)
		}
		before, err := json.Marshal(change.Before)
		if err != nil {
			return err
		}
		after, err := json.Marshal(change.After)
		if err != nil {
			return err
		}
		fmt.Fprintf(&text, "%s %s %s changed from %s to %s\n", change.Entity, key, field, before, after)
	}
	_, err := io.WriteString(out, text.String())
	return err
}
