package agentos

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/src"
	"github.com/qiangli/yoke/pkg/todo"
	"github.com/qiangli/yoke/pkg/webconsole"
)

var shapeAppLookup = func(name string) (shape.App, error) {
	if name == "console" {
		return webconsole.ConsoleShapeApp(src.Home()), nil
	}
	a, ok := fleet.New().App(name)
	if !ok {
		return shape.App{}, fmt.Errorf("shape: app %q is not registered", name)
	}
	if !a.IsShaped() {
		return shape.App{}, fmt.Errorf("shape: app %q is not a shaped app", name)
	}
	app := shape.App{Name: a.Name, SourceDir: a.Source, Shaping: a.Shaping}
	if a.Shaping != nil && a.Shaping.Kind == "cli" {
		path, revision, err := shapeCLIRelease(a.Name)
		if err != nil {
			return shape.App{}, err
		}
		app.ReleasePath, app.ReleaseRevision = path, revision
	}
	return app, nil
}

var shapeServiceOptions = shape.ProductionOptions
var shapeSubmit = func(ctx context.Context, svc shape.Service, caller shape.Caller, revision string, opts shape.SubmitOptions) (shape.Submission, error) {
	return svc.Submit(ctx, caller, revision, opts)
}
var shapeTrackingRoot = func(actor string) (string, error) {
	store, _, err := todo.ResolveStore(actor, false, false, "")
	if err != nil {
		return "", err
	}
	return store.Dir(), nil
}

func executeShapeCLI(ctx context.Context, r shapeCLIRequest) (any, error) {
	if r.Remote != "" {
		return executeShapeRemote(ctx, r)
	}
	if r.App == "console" {
		app := webconsole.ConsoleShapeApp(src.Home())
		if err := app.Seed(); err != nil {
			return nil, err
		}
	}
	app, lookupErr := shapeAppLookup(r.App)
	hostKind := lookupErr == nil && app.Shaping != nil && app.Shaping.Kind != "" && app.Shaping.Kind != "web"
	if !hostKind {
		// Existing web version operations retain their original principal semantics.
		switch r.Verb {
		case "history":
			return src.History("", r.App, r.User)
		case "restore":
			return src.RestoreSealed(ctx, "", r.App, r.ID, r.User, src.RestoreSealedOptions{})
		case "undo":
			return src.UndoSealed(ctx, "", r.App, r.User, src.RestoreSealedOptions{})
		case "release":
			return src.Release("", r.App, r.ID, r.User, src.ReleaseOptions{Repair: r.Repair, Contract: r.Contract})
		case "offer":
			return src.Offer("", r.App, r.ID, r.User, src.OfferOptions{To: r.To, All: r.All})
		case "adopt":
			return src.Adopt("", r.App, r.ID, r.User, src.AdoptOptions{Preview: r.Preview})
		}
	}
	if lookupErr != nil {
		return nil, lookupErr
	}
	opts := shapeServiceOptions(src.Home())
	opts.Track = shape.NewTracking(src.Home())
	if r.Agent != "" {
		opts.Delegate = shape.BashyDelegate{Agent: r.Agent}
	}
	svc := shape.Service{App: app, Options: opts}
	caller := shape.Caller{Principal: r.User, Actor: r.Actor, RepresentedUser: r.User}
	if hostKind {
		switch r.Verb {
		case "status":
			host, err := svc.HostStatus(ctx, caller)
			if err != nil {
				return host, err
			}
			state, err := svc.Status(ctx, caller)
			if err != nil {
				return host, err
			}
			return shapeHostStatusView{Status: state, Host: host}, nil
		case "history":
			return src.History("", r.App, shape.HostPrincipal)
		case "undo":
			return svc.HostUndo(ctx, caller)
		case "restore":
			return svc.HostRestore(ctx, caller, r.ID)
		case "release":
			return src.Release("", r.App, r.ID, shape.HostPrincipal, src.ReleaseOptions{Repair: r.Repair, Contract: r.Contract, Actor: r.Actor})
		case "incident":
			return nil, fmt.Errorf("shape: native CLI/service incident repair is not supported in this MVP; web incidents are unchanged")
		case "offer", "adopt":
			return nil, fmt.Errorf("shape: %s is not supported for host-kind artifacts in this MVP; web sharing is unchanged", r.Verb)
		case "preview":
			if svc.Kind() == "cli" {
				return svc.RunProgramPreview(ctx, caller, r.ID, r.PreviewArgs...)
			}
			return svc.PreviewProgram(ctx, caller, r.ID)
		}
	}
	wish := shape.Wish{Request: r.Request, Page: r.Page, Element: r.Element, Story: r.Story, Todo: r.Todo}
	if r.Verb == "wish" {
		root, err := shapeTrackingRoot(r.Actor)
		if err != nil {
			return nil, err
		}
		wish.TrackingRoot = root
		if r.From != "" && strings.TrimSpace(wish.Request) == "" {
			wish.Request = "Import hand-made app changes"
		}
	}
	switch r.Verb {
	case "submit":
		submitOpts := shape.SubmitOptions{Story: r.Story}
		if r.Story != "" {
			root, err := shapeTrackingRoot(r.Actor)
			if err != nil {
				return nil, err
			}
			submitOpts.TrackingRoot = root
		}
		return shapeSubmit(ctx, svc, caller, r.ID, submitOpts)
	case "status":
		return svc.Status(ctx, caller)
	case "wish":
		if r.From != "" {
			return svc.Import(ctx, caller, wish, r.From)
		}
		if hostKind {
			return svc.Wish(ctx, caller, wish)
		}
		return webconsole.LocalShapeWish(ctx, app, caller, opts, wish)
	case "show":
		return svc.Show(ctx, caller, r.ID)
	case "diff":
		return svc.Diff(ctx, caller, r.ID)
	case "verify":
		c, err := svc.Verify(ctx, caller, r.ID)
		if err == nil && c.Verdict != nil && !c.Verdict.Pass {
			err = fmt.Errorf("shape: verification failed: %s", c.Verdict.Detail)
		}
		return c, err
	case "approve":
		return svc.Approve(ctx, caller, r.ID)
	case "reject":
		return svc.Reject(ctx, caller, r.ID)
	case "suggest":
		examples, err := webconsole.LocalShapeSuggest(ctx, app, caller, opts, r.Page)
		if err != nil {
			return nil, err
		}
		return map[string]any{"examples": examples}, nil
	case "incident":
		if r.Request == "" {
			return webconsole.LocalShapeIncidents(ctx, app, caller, opts, r.ID)
		}
		return webconsole.LocalShapeReportIncident(ctx, app, caller, opts, webconsole.LocalIncidentReport{Kind: webconsole.IncidentKind(r.IncidentKind), Path: r.IncidentPath, Message: r.Request})
	case "preview":
		c, err := svc.Show(ctx, caller, r.ID)
		if err != nil {
			return nil, err
		}
		if c.Rev == "" || (c.Status != shape.Ready && c.Status != shape.Verified) {
			return nil, fmt.Errorf("shape: candidate %s has no previewable revision", r.ID)
		}
		u, err := shapeConsoleURL(r.Console)
		if err != nil {
			return nil, err
		}
		u.Path = strings.TrimRight(u.Path, "/") + "/shape/" + url.PathEscape(r.App) + "/preview/" + url.PathEscape(r.ID) + "/"
		return map[string]any{"app": r.App, "principal": r.User, "candidate": c, "url": u.String()}, nil
	}
	return nil, fmt.Errorf("shape: %s needs shared-service support that is not available in this candidate", r.Verb)
}

func shapeConsoleURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("shape: invalid console URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("shape: console URL must be an http(s) origin/base path without credentials, query or fragment")
	}
	return u, nil
}

// shapeHostStatusView keeps candidate discovery while exposing host artifact identity.
type shapeHostStatusView struct {
	shape.Status
	Host shape.HostStatus `json:"host"`
}
