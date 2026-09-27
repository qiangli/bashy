package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DigitalOcean speaks the v2 REST API directly: six endpoints do not justify
// a client library on bashy's link graph. The token needs only
// droplet:create/read/delete, ssh_key:read and tag:create/read.
type DigitalOcean struct {
	Token   string
	BaseURL string // https://api.digitalocean.com; tests point it at httptest
	HTTP    *http.Client
}

// NewDigitalOcean returns a client for the public API.
func NewDigitalOcean(token string) *DigitalOcean {
	return &DigitalOcean{Token: token, BaseURL: "https://api.digitalocean.com",
		HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (d *DigitalOcean) Name() string { return "digitalocean" }

type doDroplet struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	SizeSlug string   `json:"size_slug"`
	Tags     []string `json:"tags"`
	Created  string   `json:"created_at"`
	Size     struct {
		PriceHourly float64 `json:"price_hourly"`
	} `json:"size"`
	Region struct {
		Slug string `json:"slug"`
	} `json:"region"`
	Networks struct {
		V4 []struct {
			IP   string `json:"ip_address"`
			Type string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
}

func (dr doDroplet) host() Host {
	h := Host{ID: strconv.FormatInt(dr.ID, 10), Name: dr.Name, Status: dr.Status, Size: dr.SizeSlug,
		Region: dr.Region.Slug, PriceHourly: dr.Size.PriceHourly, Tags: dr.Tags}
	for _, n := range dr.Networks.V4 {
		if n.Type == "public" {
			h.IPv4 = n.IP
			break
		}
	}
	if t, err := time.Parse(time.RFC3339, dr.Created); err == nil {
		h.Created = t
	}
	return h
}

// apiError keeps the provider's message but never the request (which carries
// the token in a header, not the body, so the body is safe to show).
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("digitalocean: HTTP %d: %s", e.Status, e.Message)
}

func (d *DigitalOcean) do(ctx context.Context, method, path string, in, out any) error {
	if d.Token == "" {
		return fmt.Errorf("digitalocean: no token")
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(d.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.Token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := d.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = http.StatusText(resp.StatusCode)
		}
		return &apiError{Status: resp.StatusCode, Message: e.Message}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (d *DigitalOcean) Size(ctx context.Context, slug string) (Size, error) {
	path := "/v2/sizes?per_page=200"
	for path != "" {
		var r struct {
			Sizes []struct {
				Slug        string  `json:"slug"`
				PriceHourly float64 `json:"price_hourly"`
				Available   bool    `json:"available"`
				GPUInfo     *struct {
					Count int `json:"count"`
				} `json:"gpu_info"`
			} `json:"sizes"`
			Links links `json:"links"`
		}
		if err := d.do(ctx, http.MethodGet, path, nil, &r); err != nil {
			return Size{}, err
		}
		for _, s := range r.Sizes {
			if s.Slug == slug {
				gpu := strings.HasPrefix(s.Slug, "gpu-") || (s.GPUInfo != nil && s.GPUInfo.Count > 0)
				return Size{Slug: s.Slug, PriceHourly: s.PriceHourly, GPU: gpu}, nil
			}
		}
		path = r.Links.next()
	}
	return Size{}, fmt.Errorf("digitalocean: unknown size %q", slug)
}

// Create uses every SSH key the account holds: the ephemeral account holds
// only the keys meant for these hosts.
func (d *DigitalOcean) Create(ctx context.Context, s Spec) (Host, error) {
	var keys struct {
		SSHKeys []struct {
			ID int64 `json:"id"`
		} `json:"ssh_keys"`
	}
	if err := d.do(ctx, http.MethodGet, "/v2/account/keys?per_page=200", nil, &keys); err != nil {
		return Host{}, fmt.Errorf("list ssh keys: %w", err)
	}
	if len(keys.SSHKeys) == 0 {
		return Host{}, fmt.Errorf("digitalocean: the account has no SSH key; add one before creating hosts")
	}
	ids := make([]int64, 0, len(keys.SSHKeys))
	for _, k := range keys.SSHKeys {
		ids = append(ids, k.ID)
	}
	in := map[string]any{"name": s.Name, "region": s.Region, "size": s.Size, "image": s.Image,
		"ssh_keys": ids, "tags": s.Tags}
	var r struct {
		Droplet doDroplet `json:"droplet"`
	}
	if err := d.do(ctx, http.MethodPost, "/v2/droplets", in, &r); err != nil {
		return Host{}, err
	}
	return r.Droplet.host(), nil
}

func (d *DigitalOcean) Get(ctx context.Context, id string) (Host, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return Host{}, fmt.Errorf("digitalocean: droplet id %q is not numeric", id)
	}
	var r struct {
		Droplet doDroplet `json:"droplet"`
	}
	if err := d.do(ctx, http.MethodGet, "/v2/droplets/"+id, nil, &r); err != nil {
		return Host{}, err
	}
	return r.Droplet.host(), nil
}

func (d *DigitalOcean) List(ctx context.Context) ([]Host, error) {
	var out []Host
	path := "/v2/droplets?per_page=200"
	for path != "" {
		var r struct {
			Droplets []doDroplet `json:"droplets"`
			Links    links       `json:"links"`
		}
		if err := d.do(ctx, http.MethodGet, path, nil, &r); err != nil {
			return nil, err
		}
		for _, dr := range r.Droplets {
			out = append(out, dr.host())
		}
		path = r.Links.next()
	}
	return out, nil
}

func (d *DigitalOcean) Delete(ctx context.Context, id string) error {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return fmt.Errorf("digitalocean: droplet id %q is not numeric", id)
	}
	return d.do(ctx, http.MethodDelete, "/v2/droplets/"+id, nil, nil)
}

type links struct {
	Pages struct {
		Next string `json:"next"`
	} `json:"pages"`
}

// next turns the absolute "next" link into a path on the same API, so a
// test server's base URL keeps working and a link can never redirect the
// token to another host.
func (l links) next() string {
	if l.Pages.Next == "" {
		return ""
	}
	u, err := url.Parse(l.Pages.Next)
	if err != nil || u.Path == "" {
		return ""
	}
	return u.RequestURI()
}
