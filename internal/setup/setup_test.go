package setup

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// fakeGen returns a Generator that writes deterministic shell-tagged content.
func fakeGen(body string) Generator {
	return func(shell string, w io.Writer) error {
		_, err := io.WriteString(w, "COMP:"+shell+":"+body+"\n")
		return err
	}
}

func testEnv(t *testing.T, shell string) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		Home:  home,
		Path:  "/usr/bin:/bin",
		Shell: "/usr/bin/" + shell,
	}
}

func mustRun(t *testing.T, e Env, binDir, shell string, gen Generator) *Result {
	t.Helper()
	r, err := Run(e, binDir, shell, gen)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

func errCode(err error) string {
	var e *output.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestRun_BashHappyPath(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, ".local", "bin")
	r := mustRun(t, e, binDir, "bash", fakeGen("v1"))

	if r.Shell != "bash" {
		t.Fatalf("Shell=%q", r.Shell)
	}
	if !r.PathConfigured {
		t.Fatal("PathConfigured want true")
	}
	if !r.CompletionInstalled {
		t.Fatal("CompletionInstalled want true")
	}
	rc, err := e.RcFile("bash")
	if err != nil {
		t.Fatal(err)
	}
	if r.RcFile != rc {
		t.Fatalf("RcFile=%q want %q", r.RcFile, rc)
	}
	if r.Changed == nil {
		t.Fatal("Changed must be non-nil")
	}
	if len(r.Changed) == 0 {
		t.Fatal("first run should report Changed paths")
	}

	content := readFile(t, rc)
	if !strings.Contains(content, BlockStart) || !strings.Contains(content, BlockEnd) {
		t.Fatalf("missing block markers:\n%s", content)
	}
	exportLine := `export PATH="` + binDir + `:$PATH"`
	if !strings.Contains(content, exportLine) {
		t.Fatalf("missing export line:\n%s", content)
	}
	if strings.Count(content, BlockStart) != 1 {
		t.Fatalf("want exactly one block:\n%s", content)
	}

	comp, err := e.CompletionPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	compBody := readFile(t, comp)
	if !strings.HasPrefix(compBody, FileMarker+"\n") {
		t.Fatalf("completion missing FileMarker first line:\n%q", compBody)
	}
	if !strings.Contains(compBody, "COMP:bash:v1") {
		t.Fatalf("completion missing gen body:\n%s", compBody)
	}

	// Changed must list the files we touched.
	changed := map[string]bool{}
	for _, p := range r.Changed {
		changed[p] = true
	}
	if !changed[rc] || !changed[comp] {
		t.Fatalf("Changed missing rc/comp: %v", r.Changed)
	}
}

func TestRun_ZshHappyPath_IncludesFpath(t *testing.T) {
	e := testEnv(t, "zsh")
	binDir := filepath.Join(e.Home, "bin")
	r := mustRun(t, e, binDir, "zsh", fakeGen("z1"))

	if r.Shell != "zsh" || !r.PathConfigured || !r.CompletionInstalled {
		t.Fatalf("result flags: %+v", r)
	}
	rc, _ := e.RcFile("zsh")
	content := readFile(t, rc)
	exportLine := `export PATH="` + binDir + `:$PATH"`
	if !strings.Contains(content, exportLine) {
		t.Fatalf("missing export:\n%s", content)
	}
	comp, _ := e.CompletionPath("zsh")
	fpathDir := filepath.Dir(comp)
	wantFpath := "fpath=(" + fpathDir + " $fpath)"
	if !strings.Contains(content, wantFpath) {
		t.Fatalf("missing fpath line %q in:\n%s", wantFpath, content)
	}
	compBody := readFile(t, comp)
	if !strings.HasPrefix(compBody, FileMarker+"\n") {
		t.Fatalf("completion marker missing:\n%q", compBody)
	}
	if !strings.Contains(compBody, "COMP:zsh:z1") {
		t.Fatalf("gen body missing:\n%s", compBody)
	}
}

func TestRun_FishHappyPath_ConfDNoRc(t *testing.T) {
	e := testEnv(t, "fish")
	binDir := filepath.Join(e.Home, "bin")
	r := mustRun(t, e, binDir, "fish", fakeGen("f1"))

	if r.Shell != "fish" {
		t.Fatalf("Shell=%q", r.Shell)
	}
	if r.RcFile != "" {
		t.Fatalf("fish RcFile must be empty, got %q", r.RcFile)
	}
	if !r.PathConfigured || !r.CompletionInstalled {
		t.Fatalf("flags: %+v", r)
	}

	conf := e.FishConfDPath()
	body := readFile(t, conf)
	if !strings.HasPrefix(body, FileMarker+"\n") {
		t.Fatalf("conf.d missing marker:\n%q", body)
	}
	if !strings.Contains(body, "fish_add_path -g "+binDir) {
		t.Fatalf("missing fish_add_path:\n%s", body)
	}
	// No rc file created.
	if _, err := os.Stat(filepath.Join(e.Home, ".bashrc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .bashrc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.Home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .zshrc: %v", err)
	}

	comp, _ := e.CompletionPath("fish")
	if !strings.Contains(readFile(t, comp), "COMP:fish:f1") {
		t.Fatal("fish completion gen body missing")
	}
	changed := map[string]bool{}
	for _, p := range r.Changed {
		changed[p] = true
	}
	if !changed[conf] || !changed[comp] {
		t.Fatalf("Changed=%v want conf.d+comp", r.Changed)
	}
}

func TestRun_DetectShellFromEnv(t *testing.T) {
	e := testEnv(t, "bash")
	e.Shell = "/bin/bash"
	binDir := filepath.Join(e.Home, "bin")
	r := mustRun(t, e, binDir, "", fakeGen("d"))
	if r.Shell != "bash" {
		t.Fatalf("detected Shell=%q want bash", r.Shell)
	}
}

func TestRun_IdempotentSecondRunChangedEmpty(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, "bin")
	gen := fakeGen("same")
	r1 := mustRun(t, e, binDir, "bash", gen)
	if len(r1.Changed) == 0 {
		t.Fatal("first run must change something")
	}
	r2 := mustRun(t, e, binDir, "bash", gen)
	if r2.Changed == nil {
		t.Fatal("Changed must be non-nil empty slice on no-op")
	}
	if len(r2.Changed) != 0 {
		t.Fatalf("second run Changed=%v want empty", r2.Changed)
	}
	if !r2.PathConfigured || !r2.CompletionInstalled {
		t.Fatalf("flags still true: %+v", r2)
	}
	// Exactly one block.
	rc, _ := e.RcFile("bash")
	content := readFile(t, rc)
	if strings.Count(content, BlockStart) != 1 {
		t.Fatalf("want one block after two runs:\n%s", content)
	}
}

func TestRun_PathAlreadyContains_SkipExport(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, "bin")
	e.Path = "/usr/bin:" + binDir + ":/bin"
	r := mustRun(t, e, binDir, "bash", fakeGen("p"))
	if !r.PathConfigured {
		t.Fatal("PathConfigured still true when already on PATH")
	}
	rc, _ := e.RcFile("bash")
	content := readFile(t, rc)
	exportLine := `export PATH="` + binDir + `:$PATH"`
	if strings.Contains(content, exportLine) {
		t.Fatalf("export line should be skipped when PathContains:\n%s", content)
	}
	// Block still present (markers).
	if !strings.Contains(content, BlockStart) {
		t.Fatalf("block markers expected even without export:\n%s", content)
	}
}

func TestRun_ZshPathPreset_StillHasFpath(t *testing.T) {
	e := testEnv(t, "zsh")
	binDir := filepath.Join(e.Home, "bin")
	e.Path = binDir + ":/usr/bin"
	r := mustRun(t, e, binDir, "zsh", fakeGen("z"))
	if !r.PathConfigured {
		t.Fatal("PathConfigured want true")
	}
	rc, _ := e.RcFile("zsh")
	content := readFile(t, rc)
	if strings.Contains(content, `export PATH="`+binDir+`:$PATH"`) {
		t.Fatalf("export should be skipped:\n%s", content)
	}
	comp, _ := e.CompletionPath("zsh")
	wantFpath := "fpath=(" + filepath.Dir(comp) + " $fpath)"
	if !strings.Contains(content, wantFpath) {
		t.Fatalf("fpath still required:\n%s", content)
	}
}

func TestRun_RcAbsentCreated(t *testing.T) {
	e := testEnv(t, "zsh")
	binDir := filepath.Join(e.Home, "bin")
	rc, _ := e.RcFile("zsh")
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatalf("rc should not exist yet: %v", err)
	}
	mustRun(t, e, binDir, "zsh", fakeGen("c"))
	info, err := os.Stat(rc)
	if err != nil {
		t.Fatalf("rc not created: %v", err)
	}
	if info.Mode().Perm()&0200 == 0 {
		t.Fatalf("rc not writable: mode %v", info.Mode())
	}
}

func TestRun_ReadOnlyRc_IOError_NoSuccessPartial(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, "bin")
	rc, _ := e.RcFile("bash")
	// Make rc path a directory so atomic rename into it fails with EISDIR
	// (works even as root; chmod 0444 is bypassed by CAP_DAC_OVERRIDE).
	if err := os.Mkdir(rc, 0o755); err != nil {
		t.Fatal(err)
	}

	r, err := Run(e, binDir, "bash", fakeGen("x"))
	if err == nil {
		t.Fatalf("expected io_error, got result %+v", r)
	}
	if errCode(err) != "io_error" {
		t.Fatalf("code=%q want io_error; err=%v", errCode(err), err)
	}
	if r != nil {
		t.Fatalf("success Result must not be returned on io_error: %+v", r)
	}
	// Order is rc first then completion: on rc failure we must not report success,
	// and must not leave a "successful" Result that claims partial completion.
	comp, _ := e.CompletionPath("bash")
	if _, err := os.Stat(comp); err == nil {
		t.Fatalf("must not write completion after rc io_error (no partial success): %s exists", comp)
	}
}

func TestRun_UnsupportedShell(t *testing.T) {
	e := testEnv(t, "bash")
	e.Shell = "/bin/powershell"
	_, err := Run(e, filepath.Join(e.Home, "bin"), "powershell", fakeGen("x"))
	if err == nil {
		t.Fatal("expected unsupported_shell")
	}
	if errCode(err) != "unsupported_shell" {
		t.Fatalf("code=%q want unsupported_shell; err=%v", errCode(err), err)
	}
	if !strings.Contains(err.Error(), "tgc completion") {
		t.Fatalf("message should mention manual completion: %v", err)
	}

	// Empty shell arg + unsupported e.Shell basename.
	_, err = Run(e, filepath.Join(e.Home, "bin"), "", fakeGen("x"))
	if errCode(err) != "unsupported_shell" {
		t.Fatalf("detect path: code=%q err=%v", errCode(err), err)
	}
}

func TestRun_ResultJSONTags(t *testing.T) {
	e := testEnv(t, "bash")
	r := mustRun(t, e, filepath.Join(e.Home, "bin"), "bash", fakeGen("j"))
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"shell", "path_configured", "completion_installed", "changed"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing json key %q in %s", k, b)
		}
	}
	if _, ok := m["rc_file"]; !ok {
		t.Fatalf("missing rc_file in %s", b)
	}
}

func TestRemove_BashMarkedArtifacts(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, "bin")
	mustRun(t, e, binDir, "bash", fakeGen("rm"))
	rc, _ := e.RcFile("bash")
	comp, _ := e.CompletionPath("bash")

	r, err := Remove(e, "bash")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if r.Changed == nil {
		t.Fatal("Changed non-nil required")
	}
	content := readFile(t, rc)
	if strings.Contains(content, BlockStart) || strings.Contains(content, binDir) {
		t.Fatalf("block still present:\n%s", content)
	}
	if _, err := os.Stat(comp); !os.IsNotExist(err) {
		t.Fatalf("marked completion should be deleted: %v", err)
	}
	// User content preserved if we had any — recreate with prefix.
}

func TestRemove_UnmarkedCompletionSpared(t *testing.T) {
	e := testEnv(t, "bash")
	binDir := filepath.Join(e.Home, "bin")
	mustRun(t, e, binDir, "bash", fakeGen("u"))
	comp, _ := e.CompletionPath("bash")
	// Overwrite completion with unmarked content.
	if err := os.WriteFile(comp, []byte("# user completion\ncompdef _tgc tgc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Remove(e, "bash")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatalf("unmarked completion must remain: %v", err)
	}
	for _, p := range r.Changed {
		if p == comp {
			t.Fatalf("Changed must not list unmarked completion: %v", r.Changed)
		}
	}
	// Block still removed from rc.
	rc, _ := e.RcFile("bash")
	if strings.Contains(readFile(t, rc), BlockStart) {
		t.Fatal("block should be gone")
	}
}

func TestRemove_FishConfDAndCompletion(t *testing.T) {
	e := testEnv(t, "fish")
	binDir := filepath.Join(e.Home, "bin")
	mustRun(t, e, binDir, "fish", fakeGen("fr"))
	conf := e.FishConfDPath()
	comp, _ := e.CompletionPath("fish")

	r, err := Remove(e, "fish")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Fatalf("marked conf.d should be deleted: %v", err)
	}
	if _, err := os.Stat(comp); !os.IsNotExist(err) {
		t.Fatalf("marked completion should be deleted: %v", err)
	}
	changed := map[string]bool{}
	for _, p := range r.Changed {
		changed[p] = true
	}
	if !changed[conf] || !changed[comp] {
		t.Fatalf("Changed=%v", r.Changed)
	}
}

func TestRemove_UnmarkedFishConfDSpared(t *testing.T) {
	e := testEnv(t, "fish")
	conf := e.FishConfDPath()
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("# my custom path\nfish_add_path /opt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Remove(e, "fish")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(conf); err != nil {
		t.Fatalf("unmarked conf.d must remain: %v", err)
	}
	for _, p := range r.Changed {
		if p == conf {
			t.Fatal("must not report unmarked conf.d as removed")
		}
	}
}

func TestRefreshMarked_OnlyMarked(t *testing.T) {
	e := testEnv(t, "bash")
	// Install marked bash + zsh; leave fish absent; put unmarked file at fish path.
	mustRun(t, e, filepath.Join(e.Home, "bin"), "bash", fakeGen("old"))
	mustRun(t, e, filepath.Join(e.Home, "bin"), "zsh", fakeGen("old"))

	fishComp, _ := e.CompletionPath("fish")
	if err := os.MkdirAll(filepath.Dir(fishComp), 0o755); err != nil {
		t.Fatal(err)
	}
	unmarked := "# not ours\nold fish\n"
	if err := os.WriteFile(fishComp, []byte(unmarked), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also leave a marked-looking absent shell: tcsh not supported / skip.

	refreshed, err := RefreshMarked(e, fakeGen("new"))
	if err != nil {
		t.Fatalf("RefreshMarked: %v", err)
	}
	bashComp, _ := e.CompletionPath("bash")
	zshComp, _ := e.CompletionPath("zsh")

	bashBody := readFile(t, bashComp)
	if !strings.Contains(bashBody, "COMP:bash:new") {
		t.Fatalf("bash not refreshed:\n%s", bashBody)
	}
	if !strings.HasPrefix(bashBody, FileMarker+"\n") {
		t.Fatal("bash lost marker")
	}
	zshBody := readFile(t, zshComp)
	if !strings.Contains(zshBody, "COMP:zsh:new") {
		t.Fatalf("zsh not refreshed:\n%s", zshBody)
	}
	if readFile(t, fishComp) != unmarked {
		t.Fatalf("unmarked fish must be untouched:\n%s", readFile(t, fishComp))
	}

	set := map[string]bool{}
	for _, p := range refreshed {
		set[p] = true
	}
	if !set[bashComp] || !set[zshComp] {
		t.Fatalf("refreshed=%v want bash+zsh", refreshed)
	}
	if set[fishComp] {
		t.Fatal("unmarked fish must not be in refreshed")
	}
}

func TestRefreshMarked_AbsentSkipped(t *testing.T) {
	e := testEnv(t, "bash")
	// No completion files at all.
	refreshed, err := RefreshMarked(e, fakeGen("x"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed) != 0 {
		t.Fatalf("want empty, got %v", refreshed)
	}
}

func TestRun_FishPathContains_SkipAddPath(t *testing.T) {
	e := testEnv(t, "fish")
	binDir := filepath.Join(e.Home, "bin")
	e.Path = binDir + ":/usr/bin"
	r := mustRun(t, e, binDir, "fish", fakeGen("fp"))
	if !r.PathConfigured {
		t.Fatal("PathConfigured true")
	}
	body := readFile(t, e.FishConfDPath())
	if strings.Contains(body, "fish_add_path") {
		t.Fatalf("fish_add_path should be skipped:\n%s", body)
	}
	if !strings.HasPrefix(body, FileMarker) {
		t.Fatalf("marker still written:\n%s", body)
	}
}
