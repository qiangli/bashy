// bashy-tray is an optional desktop example. It is a separate Go module.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"gopkg.in/yaml.v3"
)

const appURL = "http://127.0.0.1:22749/"

type extraItem struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}
type trayConfig struct {
	Items []extraItem `yaml:"items"`
}

type status struct {
	Running, Paired bool
	Detail          string
}

func (s status) level() string {
	if !s.Running {
		return "off"
	}
	if s.Paired {
		return "on"
	}
	return "partial"
}
func (s status) title() string {
	switch s.level() {
	case "on":
		return "Outpost: connected (" + s.Detail + ")"
	case "partial":
		return "Outpost: running (" + s.Detail + ")"
	default:
		return "Outpost: off"
	}
}

type runner func(context.Context, string, ...string) ([]byte, error)

func command(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
func pollStatus(ctx context.Context, run runner) status {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := run(c, "outpost", "status", "--local-presence")
	if err != nil {
		return status{Detail: "presence unavailable"}
	}
	var p struct {
		Running bool `json:"running"`
	}
	if json.Unmarshal(data, &p) != nil || !p.Running {
		return status{}
	}
	s := status{Running: true, Detail: "unpaired"}
	data, err = run(c, "outpost", "status", "--json")
	if err != nil {
		s.Detail = "status unavailable"
		return s
	}
	var v struct {
		State  string `json:"state"`
		Status struct {
			Configured bool `json:"configured"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &v) != nil {
		s.Detail = "status unavailable"
		return s
	}
	s.Paired = v.Status.Configured && strings.Contains(strings.ToLower(v.State), "paired as ")
	if s.Paired {
		s.Detail = v.State
	} else if v.State != "" {
		s.Detail = v.State
	}
	return s
}
func configPath() (string, error) {
	dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "bashy", "tray.yaml"), nil
}
func loadConfig(path string) ([]extraItem, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg trayConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("tray.yaml: %w", err)
	}
	for _, it := range cfg.Items {
		if strings.TrimSpace(it.Label) == "" {
			return nil, errors.New("tray.yaml: item label is empty")
		}
		if err := validURL(it.URL); err != nil {
			return nil, fmt.Errorf("tray.yaml item %q: %w", it.Label, err)
		}
	}
	return cfg.Items, nil
}
func validURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return errors.New("URL must be absolute http(s) without credentials")
	}
	return nil
}
func adminURL() (string, error) {
	a := strings.TrimSpace(os.Getenv("OUTPOST_ADMIN_ADDR"))
	if a == "" {
		a = "127.0.0.1:17777"
	}
	if strings.Contains(a, "://") {
		return "", errors.New("OUTPOST_ADMIN_ADDR must be host:port")
	}
	u := "http://" + a
	return u, validURL(u)
}
func answers(raw string) bool {
	c := http.Client{Timeout: 750 * time.Millisecond}
	r, e := c.Get(raw)
	if e != nil {
		return false
	}
	r.Body.Close()
	return true
}

type windowProcess struct {
	cmd *exec.Cmd
	in  io.WriteCloser
}
type tray struct {
	mu                       sync.Mutex
	windows                  map[string]*windowProcess
	app                      *exec.Cmd
	appOwned                 bool
	closed                   bool
	admin, apps, start, stop *systray.MenuItem
}

func (t *tray) open(raw, title string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	if w := t.windows[raw]; w != nil {
		if _, err := io.WriteString(w.in, "focus\n"); err == nil {
			return
		}
		delete(t.windows, raw)
	}
	exe, err := os.Executable()
	if err != nil {
		log.Printf("window: %v", err)
		return
	}
	cmd := exec.Command(exe, "window", raw, title)
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("window: %v", err)
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		log.Printf("window: %v", err)
		return
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		in.Close()
		log.Printf("window: %v", err)
		return
	}
	p := &windowProcess{cmd, in}
	t.windows[raw] = p
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			if line := scanner.Text(); line != "READY" {
				log.Printf("window: %s", line)
			}
		}
		err := cmd.Wait()
		t.mu.Lock()
		if t.windows[raw] == p {
			delete(t.windows, raw)
		}
		t.mu.Unlock()
		in.Close()
		if err != nil {
			log.Printf("window %q: %v", title, err)
		}
	}()
}
func (t *tray) ensureApps() error {
	if answers(appURL) {
		return nil
	}
	t.mu.Lock()
	if t.appOwned {
		t.mu.Unlock()
		return waitForApps()
	}
	c := exec.Command("bashy", "app", "serve")
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		t.mu.Unlock()
		return fmt.Errorf("start bashy app serve: %w", err)
	}
	t.app = c
	t.appOwned = true
	t.mu.Unlock()
	go func() {
		err := c.Wait()
		t.mu.Lock()
		if t.app == c {
			t.app = nil
			t.appOwned = false
		}
		t.mu.Unlock()
		if err != nil {
			log.Printf("bashy app serve: %v", err)
		}
	}()
	return waitForApps()
}

func waitForApps() error {
	for i := 0; i < 20; i++ {
		if answers(appURL) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("bashy app serve did not answer at " + appURL)
}
func (t *tray) quit() {
	t.mu.Lock()
	t.closed = true
	for _, w := range t.windows {
		w.in.Close()
		_ = w.cmd.Process.Kill()
	}
	if t.appOwned && t.app != nil {
		_ = t.app.Process.Kill()
	}
	t.mu.Unlock()
	systray.Quit()
}
func icon(level string) []byte {
	col := color.RGBA{130, 130, 130, 255}
	if level == "on" {
		col = color.RGBA{39, 172, 94, 255}
	}
	if level == "partial" {
		col = color.RGBA{232, 166, 36, 255}
	}
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := x-16, y-16
			if dx*dx+dy*dy <= 196 {
				im.Set(x, y, col)
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	if runtime.GOOS != "windows" {
		return b.Bytes()
	}
	// ICO with an embedded PNG is accepted by modern Windows tray hosts.
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, uint16(0))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	ico.Write([]byte{32, 32, 0, 0})
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(32))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(b.Len()))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(22))
	ico.Write(b.Bytes())
	return ico.Bytes()
}
func (t *tray) run(items []extraItem, admin string) {
	systray.Run(func() {
		t.windows = make(map[string]*windowProcess)
		statusItem := systray.AddMenuItem("Outpost: checking…", "")
		statusItem.Disable()
		t.admin = systray.AddMenuItem("Admin", "Open outpost admin")
		t.admin.Disable()
		t.apps = systray.AddMenuItem("Apps", "Open bashy apps")
		for _, it := range items {
			item := systray.AddMenuItem(it.Label, it.URL)
			go func(it extraItem) {
				for range item.ClickedCh {
					t.open(it.URL, it.Label)
				}
			}(it)
		}
		systray.AddSeparator()
		t.start = systray.AddMenuItem("Start outpost", "")
		t.stop = systray.AddMenuItem("Stop outpost", "")
		quit := systray.AddMenuItem("Quit", "")
		refresh := func() {
			s := pollStatus(context.Background(), command)
			systray.SetIcon(icon(s.level()))
			systray.SetTooltip(s.title())
			statusItem.SetTitle(s.title())
			if s.Running {
				t.start.Hide()
				t.stop.Show()
				t.admin.Enable()
			} else {
				t.stop.Hide()
				t.start.Show()
				t.admin.Disable()
			}
		}
		go func() {
			refresh()
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				t.mu.Lock()
				closed := t.closed
				t.mu.Unlock()
				if closed {
					return
				}
				refresh()
			}
		}()
		go func() {
			for range t.admin.ClickedCh {
				t.open(admin, "Outpost Admin")
			}
		}()
		go func() {
			for range t.apps.ClickedCh {
				if err := t.ensureApps(); err != nil {
					log.Print(err)
					continue
				}
				t.open(appURL, "Bashy Apps")
			}
		}()
		action := func(item *systray.MenuItem, verb string) {
			go func() {
				for range item.ClickedCh {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					out, err := command(ctx, "outpost", verb)
					cancel()
					if err != nil {
						log.Printf("outpost %s: %v: %s", verb, err, strings.TrimSpace(string(out)))
					}
					refresh()
				}
			}()
		}
		action(t.start, "start")
		action(t.stop, "stop")
		go func() {
			for range quit.ClickedCh {
				t.quit()
				return
			}
		}()
	}, func() {})
}
func main() {
	if err := mainErr(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bashy-tray:", err)
		os.Exit(1)
	}
}
func mainErr(args []string) error {
	if len(args) > 0 && args[0] == "window" {
		if len(args) != 3 {
			return errors.New("usage: bashy-tray window URL TITLE")
		}
		if err := validURL(args[1]); err != nil {
			return err
		}
		return runWindow(args[1], args[2])
	}
	if len(args) > 0 {
		return errors.New("usage: bashy-tray [window URL TITLE]")
	}
	if err := checkTrayHost(); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	items, err := loadConfig(path)
	if err != nil {
		return err
	}
	admin, err := adminURL()
	if err != nil {
		return err
	}
	(&tray{}).run(items, admin)
	return nil
}
