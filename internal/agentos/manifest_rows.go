package agentos

import "mvdan.cc/sh/v3/polyglot"

// manifest_rows.go — the manifest fences (Sprint 238): a script carries its
// project manifest inline and drives the toolchain bashy provisions from the
// directory awd chose. Rows are data over the text runtime; the processors
// are bashy's own verbs re-entered; nothing is written into the caller's
// tree — manifests, dependency caches and outputs live under the fence root.
// The out-of-tree conventions were measured before they were chosen:
// bashsharp/docs/fenced-text-blocks-plan.md §Manifest fences.
//
//	~~~cargo as rs        rs.build() rs.test() rs.check() rs.run(args…)        shadow: src tests examples benches build.rs
//	~~~pyproject as py    py.sync() py.run(cmd…) py.test() py.build()          uv --project
//	~~~gomod as mod       mod.tidy() mod.build() mod.vet() mod.test() mod.run() -overlay + -modfile
//	~~~cmake as cm        cm.configure() cm.build() cm.test() cm.install()     -S {dir} -B {root}/build -DBASHPP_CWD={cwd}
//	~~~makefile as mk     mk.build() mk.test() mk.clean() mk.target(name…)     -f {file}, in {cwd}
//	~~~package as npm     npm.install() npm.run(script…) npm.test() npm.build() --prefix {dir}; scripts see INIT_CWD

var cargoRow = polyglot.Text{Type: "cargo", FileName: "Cargo.toml", Tool: "cargo", WorkDir: "{cwd}",
	Shadow: []string{"src", "tests", "examples", "benches", "build.rs"},
	Verbs: []polyglot.Verb{
		{Name: "build", Args: []string{"build", "-q", "--manifest-path", "{file}"}, Env: []string{"CARGO_TARGET_DIR={root}/target"}, Effects: []string{"net", "write"}},
		{Name: "test", Args: []string{"test", "-q", "--manifest-path", "{file}"}, Env: []string{"CARGO_TARGET_DIR={root}/target"}, Effects: []string{"net", "write", "exec"}},
		{Name: "check", Args: []string{"check", "-q", "--manifest-path", "{file}"}, Env: []string{"CARGO_TARGET_DIR={root}/target"}, Effects: []string{"net", "write"}},
		{Name: "run", Args: []string{"run", "-q", "--manifest-path", "{file}", "--"}, Env: []string{"CARGO_TARGET_DIR={root}/target"}, Effects: []string{"net", "write", "exec"}},
	}}

var pyprojectRow = polyglot.Text{Type: "pyproject", FileName: "pyproject.toml", Tool: "uv", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "sync", Args: []string{"sync", "-q", "--project", "{dir}"}, Effects: []string{"net", "write"}},
	{Name: "run", Args: []string{"run", "-q", "--project", "{dir}"}, Effects: []string{"net", "write", "exec"}},
	{Name: "test", Args: []string{"run", "-q", "--project", "{dir}", "pytest"}, Effects: []string{"net", "write", "exec"}},
	{Name: "build", Args: []string{"build", "-q", "--project", "{dir}", "--out-dir", "{root}/dist"}, Effects: []string{"net", "write"}},
}}

var gomodRow = polyglot.Text{Type: "gomod", FileName: "go.mod", Tool: "go", WorkDir: "{cwd}", Overlay: []string{"go.sum"}, Verbs: []polyglot.Verb{
	{Name: "tidy", Args: []string{"mod", "tidy", "-overlay", "{overlay}", "-modfile", "{file}"}, Effects: []string{"net", "write"}},
	{Name: "build", Args: []string{"build", "-overlay", "{overlay}", "-modfile", "{file}", "-o", "{root}/bin/"}, Effects: []string{"net", "write"}},
	{Name: "vet", Args: []string{"vet", "-overlay", "{overlay}", "-modfile", "{file}"}, Effects: []string{"net", "write"}},
	{Name: "test", Args: []string{"test", "-overlay", "{overlay}", "-modfile", "{file}"}, Effects: []string{"net", "write", "exec"}},
	{Name: "run", Args: []string{"run", "-overlay", "{overlay}", "-modfile", "{file}"}, Effects: []string{"net", "write", "exec"}},
}}

var cmakeRow = polyglot.Text{Type: "cmake", FileName: "CMakeLists.txt", Tool: "cmake", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "configure", Args: []string{"-S", "{dir}", "-B", "{root}/build", "-DBASHPP_CWD={cwd}"}, Effects: []string{"write", "exec"}},
	{Name: "build", Args: []string{"--build", "{root}/build"}, Effects: []string{"write", "exec"}},
	{Name: "test", Tool: "cmake", Args: []string{"--build", "{root}/build", "--target", "test"}, Effects: []string{"exec"}},
	{Name: "install", Args: []string{"--install", "{root}/build"}, Effects: []string{"write"}},
}}

var makefileRow = polyglot.Text{Type: "makefile", FileName: "Makefile", Tool: "make", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "build", Args: []string{"-f", "{file}"}, Effects: []string{"write", "exec"}},
	{Name: "test", Args: []string{"-f", "{file}", "test"}, Effects: []string{"write", "exec"}},
	{Name: "clean", Args: []string{"-f", "{file}", "clean"}, Effects: []string{"write", "exec"}},
	{Name: "target", Args: []string{"-f", "{file}"}, Effects: []string{"write", "exec"}},
}}

var packageRow = polyglot.Text{Type: "package", FileName: "package.json", Tool: "npm", WorkDir: "{cwd}", Verbs: []polyglot.Verb{
	{Name: "install", Args: []string{"--prefix", "{dir}", "install", "-s"}, Effects: []string{"net", "write"}},
	{Name: "run", Args: []string{"--prefix", "{dir}", "run", "-s"}, Effects: []string{"exec"}},
	{Name: "test", Args: []string{"--prefix", "{dir}", "test", "-s"}, Effects: []string{"exec"}},
	{Name: "build", Args: []string{"--prefix", "{dir}", "run", "-s", "build"}, Effects: []string{"write", "exec"}},
}}

func init() {
	polyglot.RegisterLanguage(polyglot.TextRow("cargo", nil, cargoRow))
	polyglot.RegisterLanguage(polyglot.TextRow("pyproject", []string{"uv"}, pyprojectRow))
	gomod := polyglot.TextRow("gomod", nil, gomodRow)
	gomod.ModuleFor = "go" // a ~~~go code fence in the same unit builds against this manifest
	polyglot.RegisterLanguage(gomod)
	polyglot.RegisterLanguage(polyglot.TextRow("cmake", nil, cmakeRow))
	polyglot.RegisterLanguage(polyglot.TextRow("makefile", []string{"make"}, makefileRow))
	polyglot.RegisterLanguage(polyglot.TextRow("package", []string{"npm"}, packageRow))
}
