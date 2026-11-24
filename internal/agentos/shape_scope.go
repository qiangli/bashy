package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/yoke/pkg/coopauth"
	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/shape/appserver"
	"github.com/spf13/cobra"
)

// Scope administration is a local owner operation. --user selects a version
// elsewhere in shape; it must never confer authority to grant access here.
func shapeScopeOwner(app shape.App, o *shapeCLIOptions) error {
	if o.Remote != "" || o.CookieFile != "" {
		return fmt.Errorf("shape scope: requires the local owner session")
	}
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	if principal == "" {
		principal = coopauth.SystemUser()
	}
	owner := strings.TrimSpace(app.Owner)
	if owner == "" {
		if cfg, ok := readMCPOutpostConfig(); ok {
			owner = strings.TrimSpace(cfg.OAuth2Email)
		}
	}
	// A local OS session owns the host; a represented cloud principal must be
	// the configured app/account owner, never merely the requested --user.
	if principal == "" || (principal != coopauth.SystemUser() && (owner == "" || !strings.EqualFold(principal, owner))) {
		return fmt.Errorf("shape scope: owner only")
	}
	if o.User != "" && !strings.EqualFold(strings.TrimSpace(o.User), principal) {
		return fmt.Errorf("shape scope: --user/--principal cannot override the owner session")
	}
	_, err := shapeAuthoredActor(o.Actor)
	return err
}

func addShapeScopeCmd(parent *cobra.Command, o *shapeCLIOptions) {
	scope := &cobra.Command{Use: "scope", Short: "Manage owner-defined member scopes in shaped.json"}
	for _, verb := range []string{"ls", "set", "rm"} {
		var appName string
		var circles, groups, emails []string
		use, args := verb+" edit|shape --app APP", cobra.ExactArgs(1)
		if verb == "ls" {
			use, args = "ls [edit|shape] --app APP", cobra.MaximumNArgs(1)
		}
		sub := &cobra.Command{Use: use, Args: args}
		sub.Flags().StringVar(&appName, "app", "", "registered shaped app")
		_ = sub.MarkFlagRequired("app")
		if verb == "set" {
			sub.Short = "Replace a scope audience (circles AND (groups OR emails))"
			sub.Flags().StringSliceVar(&circles, "circle", nil, "member circles")
			sub.Flags().StringSliceVar(&groups, "group", nil, "member groups")
			sub.Flags().StringSliceVar(&emails, "email", nil, "member emails")
		}
		if verb == "rm" {
			sub.Short = "Remove the entire named scope grant"
		}
		sub.RunE = func(c *cobra.Command, args []string) error {
			name := ""
			if len(args) != 0 {
				name = args[0]
			}
			if name != "" && !appserver.ValidScope(name) {
				return fmt.Errorf("shape scope: unknown scope %q (want edit or shape)", name)
			}
			if appName == "" || appName == "." || appName == ".." || strings.ContainsAny(appName, "/\\") {
				return fmt.Errorf("shape scope: app must be one path segment")
			}
			app, err := shapeAppLookup(appName)
			if err != nil {
				return err
			}
			if err := shapeScopeOwner(app, o); err != nil {
				return err
			}
			if app.SourceDir == "" {
				return fmt.Errorf("shape scope: app has no source directory")
			}
			path := filepath.Join(app.SourceDir, appserver.File)
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				data, err = []byte("{}"), nil
			}
			if err != nil {
				return err
			}
			manifest, err := appserver.Parse(data)
			if err != nil {
				return err
			}
			if manifest.Scopes == nil {
				manifest.Scopes = &appserver.Scopes{}
			}
			if verb != "ls" {
				var audience *appserver.Audience
				if verb == "set" {
					audience = &appserver.Audience{Circles: scopeValues(circles), Groups: scopeValues(groups), Emails: scopeValues(emails)}
					if audience.Empty() {
						return fmt.Errorf("shape scope: set needs --circle, --group or --email")
					}
				}
				switch name {
				case appserver.ScopeEdit:
					manifest.Scopes.Edit = audience
				case appserver.ScopeShape:
					manifest.Scopes.Shape = audience
				}
				data, err = json.MarshalIndent(manifest, "", "  ")
				if err != nil {
					return err
				}
				if _, err = appserver.Parse(data); err != nil {
					return err
				}
				if err = writeShapeScopeManifest(path, append(data, '\n')); err != nil {
					return err
				}
			}
			scopes := manifest.Scopes
			if name != "" && verb == "ls" {
				audience, _ := scopes.AudienceFor(name)
				scopes = &appserver.Scopes{}
				if name == appserver.ScopeEdit {
					scopes.Edit = audience
				} else {
					scopes.Shape = audience
				}
			}
			return writeShapeResult(c.OutOrStdout(), map[string]any{"app": appName, "scopes": scopes}, o.JSON)
		}
		scope.AddCommand(sub)
	}
	parent.AddCommand(scope)
}

func scopeValues(values []string) []string {
	var result []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func writeShapeScopeManifest(path string, data []byte) error {
	mode := os.FileMode(0600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".shaped-scope-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
