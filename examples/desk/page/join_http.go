package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"desk-kit"
)

var errJoinAudience = errors.New("this page is not shared with your network")

type participantContextKey struct{}

type participantRequest struct {
	Key string
	Participant
}

func withParticipant(r *http.Request, key string, p Participant) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), participantContextKey{}, participantRequest{Key: key, Participant: p}))
}

func requestParticipant(r *http.Request) (participantRequest, bool) {
	p, ok := r.Context().Value(participantContextKey{}).(participantRequest)
	return p, ok
}

func joinStatus(err error) int {
	switch {
	case errors.Is(err, errJoinNeeded), errors.Is(err, errKicked), errors.Is(err, errTunnelAddress):
		return http.StatusForbidden
	case errors.Is(err, errHandleInUse):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// Only a process launched by the console, on its dedicated loopback address,
// has the console's authenticated principal in its environment. The shaped
// proxy deliberately strips X-Bashy-Principal before forwarding.
func processConsolePrincipal(addr string) string {
	if os.Getenv("BASHY_APP") == "page" && os.Getenv("BASHY_APP_ADDR") == addr && kit.CheckLoopback(addr) == nil {
		return strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	}
	return ""
}

func joinPolicy(manifest []byte) bool {
	var cfg struct {
		Server struct {
			Args []string `json:"args"`
		} `json:"server"`
	}
	if json.Unmarshal(manifest, &cfg) != nil {
		return false
	}
	options, err := parseCLIOptions(cfg.Server.Args)
	return err == nil && options.conflicts == "suffix"
}

func joinFormAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && kit.SameOrigin(r, u)
}

// joinRoot displays a code form and redirects only to a store-verified code.
func joinRoot(w http.ResponseWriter, r *http.Request, publicRoot string, exists func(string) bool) {
	data := joinScreenData{Action: publicRoot + "/", Home: publicRoot + "/"}
	switch r.Method {
	case http.MethodGet:
		writeJoinScreen(w, http.StatusOK, data)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if !joinFormAllowed(r) || r.ParseForm() != nil {
			http.Error(w, "invalid join request", http.StatusBadRequest)
			return
		}
		code, err := chooseJoinCode(r.PostForm.Get("code"), func(string) bool { return false })
		if strings.TrimSpace(r.PostForm.Get("code")) == "" || err != nil || !exists(code) {
			data.Error = "No page has that join code."
			writeJoinScreen(w, http.StatusNotFound, data)
			return
		}
		w.Header().Set("Location", pageCodeURL(publicRoot, code))
		w.WriteHeader(http.StatusSeeOther)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type pageJoinOptions struct {
	consolePrincipal string
	outpostSecret    string
	suffix           bool
}

// pageJoin is the common enrollment and action gate. mutate must load and
// save the same page under the store lock used for native block mutations.
func (o pageJoinOptions) pageJoin(w http.ResponseWriter, r *http.Request, mutate func(func(*PageItem) error) error,
	title, action, home string, html bool, next http.Handler,
) {
	now := time.Now()
	caller := identifyParticipant(r, o.consolePrincipal, o.outpostSecret, now)
	var handle *string
	submission := html && r.Method == http.MethodPost
	if submission {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if !joinFormAllowed(r) || r.ParseForm() != nil {
			http.Error(w, "invalid join request", http.StatusBadRequest)
			return
		}
		value := r.PostForm.Get("handle")
		if r.PostForm.Get("random") == "1" {
			value = randomHandle()
		}
		handle = &value
	}
	if caller.principal != "" || submission {
		mintParticipantCookie(w, r, &caller)
	}
	var p participantRequest
	var participantErr error
	err := mutate(func(page *PageItem) error {
		key, err := resolveParticipant(page, caller, handle, o.suffix, now)
		if err != nil {
			participantErr = err
			return err
		}
		p = participantRequest{Key: key, Participant: *page.Participants[key]}
		return nil
	})
	if participantErr != nil {
		if html {
			status := joinStatus(participantErr)
			hint := participantErr.Error()
			if errors.Is(participantErr, errJoinNeeded) {
				status, hint = http.StatusOK, ""
			}
			writeJoinScreen(w, status, joinScreenData{Page: title, Error: hint, Action: action, Home: home})
		} else {
			kit.Fail(w, joinStatus(participantErr), participantErr.Error())
		}
		return
	}
	if errors.Is(err, errJoinAudience) {
		kit.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not load the participant")
		return
	}
	done := traceParticipant(r, p.Participant, now)
	defer done()
	if submission {
		w.Header().Set("Location", action)
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	next.ServeHTTP(w, withParticipant(r, p.Key, p.Participant))
}

func moderationMessage(pageID, action, source, target string) string {
	return fmt.Sprintf("page %q participant %s source=%q target=%q", pageID, action, source, target)
}
