package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNotFound is returned by a Provider when the host does not exist (or is
// not visible to the token).
var ErrNotFound = errors.New("host not found")

// Host is one machine as the provider reports it.
type Host struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	IPv4        string    `json:"ipv4,omitempty"`
	Size        string    `json:"size"`
	Region      string    `json:"region"`
	PriceHourly float64   `json:"price_hourly"`
	Tags        []string  `json:"tags,omitempty"`
	Created     time.Time `json:"created"`
}

// Spec is what Create asks the provider for.
type Spec struct {
	Name   string
	Size   string
	Region string
	Image  string
	Tags   []string
}

// Size is one purchasable machine shape.
type Size struct {
	Slug        string
	PriceHourly float64
	GPU         bool
}

// Provider is the whole cloud surface this package needs. It is kept this
// small on purpose: a second provider is one file.
type Provider interface {
	Name() string
	Size(ctx context.Context, slug string) (Size, error)
	Create(ctx context.Context, s Spec) (Host, error)
	Get(ctx context.Context, id string) (Host, error)
	List(ctx context.Context) ([]Host, error)
	Delete(ctx context.Context, id string) error
}

// Policy bounds what an agent may rent without asking. The defaults are
// deliberately small; the operator raises them in policy.json.
type Policy struct {
	Provider      string   `json:"provider"`
	MaxTTL        Duration `json:"max_ttl"`
	MaxCapUSD     float64  `json:"max_cap_usd"`
	DailyCapUSD   float64  `json:"daily_cap_usd"`
	AllowGPU      bool     `json:"allow_gpu"`
	DefaultSize   string   `json:"default_size"`
	DefaultRegion string   `json:"default_region"`
	DefaultImage  string   `json:"default_image"`
	// TokenSecret names the vault secret (or environment variable) holding
	// the provider token. It is never the provider's usual variable
	// (DIGITALOCEAN_ACCESS_TOKEN): that one belongs to the main account.
	TokenSecret string `json:"token_secret"`
	// ProductionTripwire lists host names that must never be visible to the
	// token. If the provider lists one of them, the token reaches production
	// and every mutating verb refuses.
	ProductionTripwire []string `json:"production_tripwire,omitempty"`
}

// DefaultPolicy is what applies when policy.json is absent.
func DefaultPolicy() Policy {
	return Policy{
		Provider:      "digitalocean",
		MaxTTL:        Duration(12 * time.Hour),
		MaxCapUSD:     10,
		DailyCapUSD:   25,
		DefaultSize:   "s-2vcpu-4gb",
		DefaultRegion: "lon1",
		DefaultImage:  "ubuntu-24-04-x64",
		TokenSecret:   "DO_EPHEMERAL_TOKEN",
	}
}

// Duration is a time.Duration that reads and writes as "12h".
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"12h\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// StateDir is $BASHY_HOME/ephemeral-host, or ~/.bashy/ephemeral-host.
func StateDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		return filepath.Join(home, "ephemeral-host"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ephemeral-host: resolve bashy home: %w", err)
	}
	return filepath.Join(home, ".bashy", "ephemeral-host"), nil
}

// LoadPolicy reads policy.json from the state directory over the defaults.
func LoadPolicy(dir string) (Policy, error) {
	p := DefaultPolicy()
	b, err := os.ReadFile(filepath.Join(dir, "policy.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("policy.json: %w", err)
	}
	return p, nil
}

// Tags written on every host. The deadline lives on the machine as well as in
// the ledger so that a reaper on another host, holding only the token, can
// still find what is overdue.
const (
	TagLease    = "bashy-lease"
	tagDeadline = "bashy-deadline-"
	tagSeat     = "bashy-seat-"
	tagSprint   = "bashy-sprint-"
)

var tagUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func tagValue(s string) string {
	s = tagUnsafe.ReplaceAllString(s, "-")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "-")
}

func leaseTags(seat, sprint string, deadline time.Time) []string {
	tags := []string{TagLease, fmt.Sprintf("%s%d", tagDeadline, deadline.Unix())}
	if v := tagValue(seat); v != "" {
		tags = append(tags, tagSeat+v)
	}
	if v := tagValue(sprint); v != "" {
		tags = append(tags, tagSprint+v)
	}
	return tags
}

// DeadlineFromTags returns the deadline a host carries, if any.
func DeadlineFromTags(tags []string) (time.Time, bool) {
	for _, t := range tags {
		if rest, ok := strings.CutPrefix(t, tagDeadline); ok {
			var unix int64
			if _, err := fmt.Sscanf(rest, "%d", &unix); err == nil && unix > 0 {
				return time.Unix(unix, 0).UTC(), true
			}
		}
	}
	return time.Time{}, false
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidName reports whether name is usable as a host name: lower-case
// letters, digits and dashes, no dots (a dotted name makes DigitalOcean
// create a PTR record, which a throwaway box must not do).
func ValidName(name string) bool { return validName.MatchString(name) }
