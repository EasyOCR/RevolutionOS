package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// installedScriptsDir is where build.sh copies scripts/orchestrator/ to in a
// real RevolutionOS image (see image/build_files/build.sh). Checked first;
// falls back to dev-checkout-relative resolution below so `wails dev`/`wails
// build` still work from a plain git clone with nothing installed.
const installedScriptsDir = "/usr/libexec/revolutionos/orchestrator"

// App struct
type App struct {
	ctx        context.Context
	scriptsDir string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// resolveScriptsDir finds scripts/orchestrator/ whether this is a real
// installed image (scripts copied to installedScriptsDir by build.sh) or a
// dev checkout (this binary at ui/build/bin/<name>, scripts one level up
// from ui/ at the repo root).
func resolveScriptsDir() string {
	if info, err := os.Stat(installedScriptsDir); err == nil && info.IsDir() {
		return installedScriptsDir
	}
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(filepath.Dir(file))
	return filepath.Join(repoRoot, "scripts", "orchestrator")
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.scriptsDir = resolveScriptsDir()
}

// Instance describes one orchestrator instance's current state.
type Instance struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Display string `json:"display"`
}

func (a *App) script(name string) string {
	return filepath.Join(a.scriptsDir, name)
}

func (a *App) runScript(name string, args ...string) (string, error) {
	cmd := exec.Command(a.script(name), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// ListInstances returns every created instance and whether it's running.
// Parses list-instances.sh's "<name>: running on <display>" / "<name>: stopped" output.
func (a *App) ListInstances() ([]Instance, error) {
	out, err := a.runScript("list-instances.sh")
	if err != nil {
		return nil, err
	}

	var instances []Instance
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])

		inst := Instance{Name: name}
		if strings.HasPrefix(status, "running on ") {
			inst.Running = true
			inst.Display = strings.TrimPrefix(status, "running on ")
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// CreateInstance sets up a new isolated instance (directory + fake HOME only,
// doesn't start anything - matches create-instance.sh's behavior).
func (a *App) CreateInstance(name string) error {
	_, err := a.runScript("create-instance.sh", name)
	return err
}

// StartInstance launches an instance's isolated X server + metacity + Sober +
// RevolutionMacro. backend should be "xvfb" (headless, default) or "xephyr"
// (visible, for spectating).
func (a *App) StartInstance(name string, backend string) error {
	args := []string{name}
	if backend == "xephyr" {
		args = append(args, "--visible")
	}
	_, err := a.runScript("start-instance.sh", args...)
	return err
}

// StopInstance tears down everything for a named instance.
func (a *App) StopInstance(name string) error {
	_, err := a.runScript("stop-instance.sh", name)
	return err
}

// Account is an Instance plus the Roblox @username it's logged in as (empty
// if never logged in yet) - the unit the UI actually presents to the user.
// Internal instance names (test1, acct3, ...) are plumbing from here on,
// mirroring what the `multisober` CLI already does for the terminal.
type Account struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Running  bool   `json:"running"`
	Display  string `json:"display"`
}

var usernameRe = regexp.MustCompile(`"Username":"([^"]*)"`)

// instancesRoot mirrors lib.sh's INSTANCES_ROOT: $REVOLUTIONOS_INSTANCES_ROOT
// if set, else $HOME/.local/share/revolutionos/instances.
func instancesRoot() string {
	if v := os.Getenv("REVOLUTIONOS_INSTANCES_ROOT"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "revolutionos", "instances")
}

func instanceHome(name string) string {
	return filepath.Join(instancesRoot(), name, "home")
}

// instanceUsername reads the Roblox @username straight from Sober's own
// local cache - same source multisober/lib.sh's instance_username() uses.
// Returns "" (not an error) for an instance that's never logged in yet.
func instanceUsername(name string) string {
	path := filepath.Join(instanceHome(name), ".var", "app", "org.vinegarhq.Sober",
		"data", "sober", "appData", "LocalStorage", "appStorage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := usernameRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// ListAccounts is ListInstances with each instance's logged-in @username
// attached - what the UI actually renders.
func (a *App) ListAccounts() ([]Account, error) {
	instances, err := a.ListInstances()
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(instances))
	for _, inst := range instances {
		accounts = append(accounts, Account{
			Name:     inst.Name,
			Username: instanceUsername(inst.Name),
			Running:  inst.Running,
			Display:  inst.Display,
		})
	}
	return accounts, nil
}

// LaunchAccount finds whichever instance is logged in as the given Roblox
// username (case-insensitive, matching Roblox's own login behavior) and
// starts it visibly. Mirrors `multisober launch <username>`.
func (a *App) LaunchAccount(username string) error {
	instances, err := a.ListInstances()
	if err != nil {
		return err
	}
	want := strings.ToLower(username)
	for _, inst := range instances {
		if strings.ToLower(instanceUsername(inst.Name)) == want {
			return a.StartInstance(inst.Name, "xephyr")
		}
	}
	return fmt.Errorf("no instance is logged in as %q", username)
}

// AddAccount creates a new instance with an auto-generated internal name
// and starts it visibly so the user can log into a new Roblox account in
// the window that opens. Mirrors `multisober addinstance`.
func (a *App) AddAccount() error {
	name := nextAutoInstanceName()
	if err := a.CreateInstance(name); err != nil {
		return err
	}
	return a.StartInstance(name, "xephyr")
}

// nextAutoInstanceName mirrors lib.sh's next_auto_instance_name(): internal
// names are plumbing once accounts are addressed by username, so this just
// needs to be unique, not meaningful.
func nextAutoInstanceName() string {
	root := instancesRoot()
	for n := 1; ; n++ {
		name := "acct" + strconv.Itoa(n)
		if _, err := os.Stat(filepath.Join(root, name)); os.IsNotExist(err) {
			return name
		}
	}
}
