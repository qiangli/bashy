package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDigitalOceanClient(t *testing.T) {
	var created map[string]any
	var sawDelete string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"id":"unauthorized","message":"Unable to authenticate you"}`)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/v2/sizes" && r.URL.Query().Get("page") == "":
			io.WriteString(w, `{"sizes":[{"slug":"s-1vcpu-1gb","price_hourly":0.00893}],
				"links":{"pages":{"next":"https://api.digitalocean.com/v2/sizes?page=2&per_page=200"}}}`)
		case r.Method == "GET" && r.URL.Path == "/v2/sizes":
			io.WriteString(w, `{"sizes":[{"slug":"gpu-h100x1-80gb","price_hourly":3.39,"gpu_info":{"count":1}}],"links":{}}`)
		case r.Method == "GET" && r.URL.Path == "/v2/account/keys":
			io.WriteString(w, `{"ssh_keys":[{"id":11},{"id":12}]}`)
		case r.Method == "POST" && r.URL.Path == "/v2/droplets":
			_ = json.NewDecoder(r.Body).Decode(&created)
			io.WriteString(w, `{"droplet":{"id":501,"name":"a","status":"new","size_slug":"s-1vcpu-1gb",
				"region":{"slug":"lon1"},"size":{"price_hourly":0.00893},"tags":["bashy-lease"]}}`)
		case r.Method == "GET" && r.URL.Path == "/v2/droplets/501":
			io.WriteString(w, `{"droplet":{"id":501,"name":"a","status":"active","size_slug":"s-1vcpu-1gb",
				"networks":{"v4":[{"ip_address":"10.0.0.2","type":"private"},{"ip_address":"192.0.2.7","type":"public"}]},
				"created_at":"2026-09-27T18:00:00Z"}}`)
		case r.Method == "GET" && r.URL.Path == "/v2/droplets/404":
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"id":"not_found","message":"The resource you were accessing could not be found."}`)
		case r.Method == "GET" && r.URL.Path == "/v2/droplets":
			io.WriteString(w, `{"droplets":[{"id":501,"name":"a","status":"active"}],"links":{}}`)
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/v2/droplets/"):
			sawDelete = strings.TrimPrefix(r.URL.Path, "/v2/droplets/")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	d := &DigitalOcean{Token: "tok", BaseURL: srv.URL, HTTP: srv.Client()}

	s, err := d.Size(ctx, "s-1vcpu-1gb")
	if err != nil || s.PriceHourly != 0.00893 || s.GPU {
		t.Fatalf("size: %+v %v", s, err)
	}
	s, err = d.Size(ctx, "gpu-h100x1-80gb") // second page, via the next link rebased onto the test server
	if err != nil || !s.GPU {
		t.Fatalf("paged gpu size: %+v %v", s, err)
	}
	h, err := d.Create(ctx, Spec{Name: "a", Size: "s-1vcpu-1gb", Region: "lon1", Image: "ubuntu-24-04-x64", Tags: []string{"bashy-lease"}})
	if err != nil || h.ID != "501" || h.Region != "lon1" {
		t.Fatalf("create: %+v %v", h, err)
	}
	if keys, _ := created["ssh_keys"].([]any); len(keys) != 2 {
		t.Fatalf("create did not send the account's ssh keys: %v", created)
	}
	h, err = d.Get(ctx, "501")
	if err != nil || h.IPv4 != "192.0.2.7" || h.Status != "active" || h.Created.IsZero() {
		t.Fatalf("get: %+v %v", h, err)
	}
	if _, err := d.Get(ctx, "404"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if _, err := d.Get(ctx, "../account"); err == nil {
		t.Fatal("a non-numeric id reached the API")
	}
	hs, err := d.List(ctx)
	if err != nil || len(hs) != 1 {
		t.Fatalf("list: %+v %v", hs, err)
	}
	if err := d.Delete(ctx, "501"); err != nil || sawDelete != "501" {
		t.Fatalf("delete: %v %q", err, sawDelete)
	}

	bad := &DigitalOcean{Token: "wrong", BaseURL: srv.URL, HTTP: srv.Client()}
	_, err = bad.List(ctx)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("auth error must name the status and never echo the token: %v", err)
	}
}
