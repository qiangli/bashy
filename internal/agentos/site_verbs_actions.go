package agentos

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/kb"
	"github.com/qiangli/yoke/pkg/skills"
	"github.com/qiangli/yoke/pkg/webconsole"
)

func siteAttach(cmd *cobra.Command, page *url.URL, asJSON, dryRun bool) error {
	page.Fragment = ""
	// Skills add resolves the origin's well-known index and checks its digest.
	// A discovered direct skill is deliberately secondary to that index.
	target := page.String()
	origin := page.ResolveReference(&url.URL{Path: "/"})
	var report bytes.Buffer
	add := skills.NewSkillsCmd(skillsOptions()...)
	args := []string{"add", "--json", origin.String()}
	if dryRun {
		args = append(args, "--dry-run")
	}
	add.SetArgs(args)
	add.SetOut(&report)
	add.SetErr(cmd.ErrOrStderr())
	if err := add.ExecuteContext(cmd.Context()); err != nil {
		return err
	}
	var admission skills.Admission
	if err := json.Unmarshal(report.Bytes(), &admission); err != nil {
		return err
	}
	if dryRun {
		index := page.ResolveReference(&url.URL{Path: siteSkillIndex}).String()
		if asJSON {
			return siteJSON(cmd, map[string]any{"url": target, "skill": admission, "skills_index": index, "dry_run": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Would install %s and record %s in the host KB (index %s).\n", admission.Name, target, index)
		return nil
	}
	digest := sha256.Sum256([]byte(origin.String()))
	slug := fmt.Sprintf("agent-ready-site-%x", digest[:12])
	store := kb.Open(kb.DefaultDir())
	record := &kb.Page{
		Slug: slug, Type: "fact", Form: "page", Status: "candidate",
		Title:       "Agent-ready site " + origin.Host,
		Description: "Site attached through verified skill admission",
		Resource:    target, Tags: []string{"agent-ready-site"},
		Body: fmt.Sprintf("Attached %s through the skill admission gate.\n\nSite: %s\n", admission.Name, target),
	}
	if _, err := store.Load(slug); os.IsNotExist(err) {
		if err := store.Write(record, "site attach"); err != nil {
			return fmt.Errorf("skill %q is installed but recording the site failed: %w", admission.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("skill %q is installed but reading the site's record failed: %w", admission.Name, err)
	}
	if asJSON {
		return siteJSON(cmd, map[string]any{"url": target, "skill": admission, "kb": slug})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Installed %s and recorded %s in the host KB (%s).\n", admission.Name, target, slug)
	return nil
}

type siteCitation struct {
	URL           string `json:"url"`
	SectionID     string `json:"section_id"`
	Revision      string `json:"revision"`
	SectionSHA256 string `json:"section_sha256"`
	Quote         string `json:"quote"`
}

func siteCite(cmd *cobra.Command, client *http.Client, page *url.URL, asJSON bool) error {
	if page.Fragment == "" {
		return fmt.Errorf("site cite: append a section ID as #id")
	}
	cite := siteCitation{URL: page.String(), SectionID: page.Fragment}
	page.Fragment = ""
	ctx := cmd.Context()
	response, err := siteGet(ctx, client, page, "application/json")
	var content struct {
		Revision string
		Sections []struct{ ID, SHA256, Markdown string }
	}
	if err != nil || json.Unmarshal([]byte(response.body), &content) != nil || content.Sections == nil {
		s, err := discoverSite(ctx, client, page)
		if err != nil {
			return err
		}
		target := s.fields["json"]
		if target == "" {
			return fmt.Errorf("site cite: no JSON twin advertised")
		}
		twin, _ := siteURL(target)
		response, err = siteGet(ctx, client, twin, "application/json")
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(response.body), &content); err != nil {
			return fmt.Errorf("site cite: invalid JSON twin: %w", err)
		}
	}
	for _, section := range content.Sections {
		if section.ID != cite.SectionID {
			continue
		}
		if content.Revision == "" || section.SHA256 == "" {
			return fmt.Errorf("site cite: section %q is missing revision or SHA-256", cite.SectionID)
		}
		cite.Revision, cite.SectionSHA256, cite.Quote = content.Revision, section.SHA256, section.Markdown
		if asJSON {
			return siteJSON(cmd, cite)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\nRevision: %s\nSection SHA-256: %s\n\n%s\n", cite.URL, cite.Revision, cite.SectionSHA256, cite.Quote)
		return nil
	}
	return fmt.Errorf("site cite: section %q not found", cite.SectionID)
}

func siteShape(cmd *cobra.Command, client *http.Client, page *url.URL, asJSON, dryRun bool) error {
	s, err := discoverSite(cmd.Context(), client, page)
	if err != nil {
		return err
	}
	source, err := siteURL(s.fields["source"])
	if err != nil {
		return fmt.Errorf("site shape: site did not advertise an HTTP(S) source repository")
	}
	// Derive a registry name without ever interpolating remote text into argv.
	repo := strings.ToLower(strings.TrimSuffix(path.Base(source.Path), ".git"))
	repo = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, repo)
	repo = strings.Trim(repo, "-")
	if len(repo) > 32 {
		repo = repo[:32]
	}
	digest := sha256.Sum256([]byte(source.String()))
	name := fmt.Sprintf("site-%s-%x", repo, digest[:4])
	if repo == "" {
		name = fmt.Sprintf("site-%x", digest[:4])
	}
	steps := []string{
		"bashy shape wish " + name + " REQUEST",
		"bashy shape verify " + name + " ID",
		"bashy shape preview " + name + " ID",
		"bashy shape approve " + name + " ID",
		"bashy shape undo " + name,
	}
	if !dryRun {
		if err := registerSiteSource(cmd, name, source.String()); err != nil {
			return err
		}
	}
	if asJSON {
		return siteJSON(cmd, map[string]any{"app": name, "type": fleet.AppTypeShaped, "source": source.String(), "steps": steps, "dry_run": dryRun})
	}
	action := "Registered"
	if dryRun {
		action = "Would register"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s as shaped from %s\n%s\n", action, name, source, strings.Join(steps, "\n"))
	return nil
}

func registerSiteSource(cmd *cobra.Command, name, source string) error {
	app := webconsole.NewAppsCmd()
	app.SetArgs([]string{"add", name, "--type", fleet.AppTypeShaped, "--source", source})
	app.SetOut(io.Discard)
	app.SetErr(cmd.ErrOrStderr())
	return app.ExecuteContext(cmd.Context())
}
