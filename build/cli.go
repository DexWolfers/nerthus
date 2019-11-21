package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gitee.com/nerthus/nerthus/internal/build"
)

var (
	gethArchiveFiles = []string{
		"COPYING",
		executablePath("cnts"),
	}

	// Files that end up in the geth-alltools*.zip archive.
	allToolsArchiveFiles = []string{
		"COPYING",
		executablePath("cnts"),
	}
)

func executablePath(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(GOBIN, name)
}

var GOBIN, _ = filepath.Abs(filepath.Join("build", "bin"))

func main() {
	log.SetFlags(log.Lshortfile)

	if _, err := os.Stat(filepath.Join("build", "cli.go")); os.IsNotExist(err) {
		log.Fatal("this script must be run from the root of the repository")
	}

	if len(os.Args) < 2 {
		log.Fatal("need subcommand as first argument")
	}

	switch os.Args[1] {
	case "install":
		doInstall(os.Args[2:])
	case "test":
		doTest(os.Args[2:])
	case "lint":
		doLint(os.Args[2:])
	case "xgo":
		doXgo(os.Args[2:])
	default:
		log.Fatal("unknown command ", os.Args[1])
	}
}

func doInstall(cmdline []string) {
	var (
		arch = flag.String("arch", "", "Architecture to cross build for")
	)

	flag.CommandLine.Parse(cmdline)

	env := build.Env()

	// Check Go version. People regularly open issues about compilation
	// failure with outdated Go. This should save them the trouble.
	if gov := runtime.Version(); !strings.Contains(gov, "devel") {
		// Figure out the minor version number since we can't textually compare (1.10 < 1.9)
		var minor int
		fmt.Sscanf(strings.TrimPrefix(gov, "go1."), "%d", &minor)

		if minor < 9 {
			log.Println("You have Go version", gov)
			log.Println("nerthus requires at least Go version 1.9 and cannot")
			log.Println("be compiled with an earlier version. Please upgrade your Go installation.")
			os.Exit(1)
		}
	}
	packages := []string{"./..."}
	if flag.NArg() > 0 {
		packages = flag.Args()
	}
	packages = build.ExpandPackagesNoVendor(packages)

	if *arch == "" || *arch == runtime.GOARCH {
		goinstall := goTool("install", buildFlags(env)...)
		goinstall.Args = append(goinstall.Args, "-v")
		goinstall.Args = append(goinstall.Args, packages...)
		build.MustRun(goinstall)
		return
	}

	// If we are cross compiling to ARMv5 ARMv6 or ARMv7, clean any previous builds
	// todo

}

func buildFlags(env build.Environment) (flags []string) {
	var ld []string
	if env.Commit != "" {
		ld = append(ld, "-X", "gitee.com/nerthus/nerthus/params.GITCOMMIT="+env.Commit)
	}

	if env.CommitTime != "" {
		ld = append(ld, "-X", "gitee.com/nerthus/nerthus/params.BUILDTIME="+env.CommitTime)
	}

	if runtime.GOOS == "darwin" {
		ld = append(ld, "-s")
	}

	if len(ld) > 0 {
		flags = append(flags, "-ldflags", strings.Join(ld, " "))
	}
	return flags
}

func goTool(subcmd string, args ...string) *exec.Cmd {
	return goToolArch(runtime.GOARCH, subcmd, args...)
}

func goToolArch(arch string, subcmd string, args ...string) *exec.Cmd {
	cmd := build.GoTool(subcmd, args...)
	if subcmd == "build" || subcmd == "install" || subcmd == "test" {
		// Go CGO has a Windows linker error prior to 1.8 (https://github.com/golang/go/issues/8756).
		// Work around issue by allowing multiple definitions for <1.8 builds.
		if runtime.GOOS == "windows" && runtime.Version() < "go1.8" {
			cmd.Args = append(cmd.Args, []string{"-ldflags", "-extldflags -Wl,--allow-multiple-definition"}...)
		}
	}
	cmd.Env = []string{"GOPATH=" + build.GOPATH()}
	if arch == "" || arch == runtime.GOARCH {
		cmd.Env = append(cmd.Env, "GOBIN="+GOBIN)
	} else {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		cmd.Env = append(cmd.Env, "GOARCH="+arch)
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOPATH=") || strings.HasPrefix(e, "GOBIN=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	return cmd
}

func doTest(cmdline []string) {
	var (
		coverage = flag.Bool("coverage", false, "Whether to record code coverage")
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	packages := []string{"./..."}
	if len(flag.CommandLine.Args()) > 0 {
		packages = flag.CommandLine.Args()
	}
	packages = build.ExpandPackagesNoVendor(packages)

	// Run analysis tools before the tests.
	build.MustRun(goTool("vet", packages...))

	// Run the actual tests.
	gotest := goTool("test", buildFlags(env)...)
	// Test a single package at a time. CI builders are slow
	// and some tests run into timeouts under load.
	gotest.Args = append(gotest.Args, "-p", "1")
	if *coverage {
		gotest.Args = append(gotest.Args, "-covermode=atomic", "-cover")
	}

	gotest.Args = append(gotest.Args, packages...)
	build.MustRun(gotest)
}

// runs gometalinter on requested packages
func doLint(cmdline []string) {
	flag.CommandLine.Parse(cmdline)

	packages := []string{"./..."}
	if len(flag.CommandLine.Args()) > 0 {
		packages = flag.CommandLine.Args()
	}
	// Get metalinter and install all supported linters
	build.MustRun(goTool("get", "gopkg.in/alecthomas/gometalinter.v2"))
	build.MustRunCommand(filepath.Join(GOBIN, "gometalinter.v2"), "--install")

	// Run fast linters batched together
	configs := []string{
		"--vendor",
		"--disable-all",
		"--enable=vet",
		"--enable=gofmt",
		"--enable=misspell",
		"--enable=goconst",
		"--min-occurrences=6", // for goconst
	}
	build.MustRunCommand(filepath.Join(GOBIN, "gometalinter.v2"), append(configs, packages...)...)

	// Run slow linters one by one
	for _, linter := range []string{"unconvert", "gosimple"} {
		configs = []string{"--vendor", "--deadline=10m", "--disable-all", "--enable=" + linter}
		build.MustRunCommand(filepath.Join(GOBIN, "gometalinter.v2"), append(configs, packages...)...)
	}
}

func archiveBasename(arch string, env build.Environment) string {
	platform := runtime.GOOS + "-" + arch
	if arch == "arm" {
		platform += os.Getenv("GOARM")
	}
	if arch == "android" {
		platform = "android-all"
	}
	if arch == "ios" {
		platform = "ios-all"
	}
	return platform + "-" + archiveVersion(env)
}

func archiveVersion(env build.Environment) string {
	version := build.VERSION()
	if isUnstableBuild(env) {
		version += "-unstable"
	}
	if env.Commit != "" {
		version += "-" + env.Commit[:8]
	}
	return version
}

// skips archiving for some build configurations.
func maybeSkipArchive(env build.Environment) {
	if env.IsPullRequest {
		log.Printf("skipping because this is a PR build")
		os.Exit(0)
	}
	if env.IsCronJob {
		log.Printf("skipping because this is a cron job")
		os.Exit(0)
	}
	if env.Branch != "master" && !strings.HasPrefix(env.Tag, "v1.") {
		log.Printf("skipping because branch %q, tag %q is not on the whitelist", env.Branch, env.Tag)
		os.Exit(0)
	}
}

func makeWorkdir(wdflag string) string {
	var err error
	if wdflag != "" {
		err = os.MkdirAll(wdflag, 0744)
	} else {
		wdflag, err = ioutil.TempDir("", "cnts-build-")
	}
	if err != nil {
		log.Fatal(err)
	}
	return wdflag
}

func isUnstableBuild(env build.Environment) bool {
	if env.Tag != "" {
		return false
	}
	return true
}

type debMetadata struct {
	Env build.Environment

	// go-ethereum version being built. Note that this
	// is not the debian package version. The package version
	// is constructed by VersionString.
	Version string

	Author       string // "name <email>", also selects signing key
	Distro, Time string
	Executables  []debExecutable
}

type debExecutable struct {
	Name, Description string
}

// Name returns the name of the metapackage that depends
// on all executable packages.
func (meta debMetadata) Name() string {
	if isUnstableBuild(meta.Env) {
		return "nerthus-unstable"
	}
	return "nerthus"
}

// VersionString returns the debian version of the packages.
func (meta debMetadata) VersionString() string {
	vsn := meta.Version
	if meta.Env.Buildnum != "" {
		vsn += "+build" + meta.Env.Buildnum
	}
	if meta.Distro != "" {
		vsn += "+" + meta.Distro
	}
	return vsn
}

// ExeList returns the list of all executable packages.
func (meta debMetadata) ExeList() string {
	names := make([]string, len(meta.Executables))
	for i, e := range meta.Executables {
		names[i] = meta.ExeName(e)
	}
	return strings.Join(names, ", ")
}

// ExeName returns the package name of an executable package.
func (meta debMetadata) ExeName(exe debExecutable) string {
	if isUnstableBuild(meta.Env) {
		return exe.Name + "-unstable"
	}
	return exe.Name
}

// ExeConflicts returns the content of the Conflicts field
// for executable packages.
func (meta debMetadata) ExeConflicts(exe debExecutable) string {
	if isUnstableBuild(meta.Env) {
		// Set up the conflicts list so that the *-unstable packages
		// cannot be installed alongside the regular version.
		//
		// https://www.debian.org/doc/debian-policy/ch-relationships.html
		// is very explicit about Conflicts: and says that Breaks: should
		// be preferred and the conflicting files should be handled via
		// alternates. We might do this eventually but using a conflict is
		// easier now.
		return "nerthus, " + exe.Name
	}
	return ""
}

func stageDebianSource(tmpdir string, meta debMetadata) (pkgdir string) {
	pkg := meta.Name() + "-" + meta.VersionString()
	pkgdir = filepath.Join(tmpdir, pkg)
	if err := os.Mkdir(pkgdir, 0755); err != nil {
		log.Fatal(err)
	}

	// Copy the source code.
	build.MustRunCommand("git", "checkout-index", "-a", "--prefix", pkgdir+string(filepath.Separator))

	// Put the debian build files in place.
	debian := filepath.Join(pkgdir, "debian")
	build.Render("build/deb.rules", filepath.Join(debian, "rules"), 0755, meta)
	build.Render("build/deb.changelog", filepath.Join(debian, "changelog"), 0644, meta)
	build.Render("build/deb.control", filepath.Join(debian, "control"), 0644, meta)
	build.Render("build/deb.copyright", filepath.Join(debian, "copyright"), 0644, meta)
	build.RenderString("8\n", filepath.Join(debian, "compat"), 0644, meta)
	build.RenderString("3.0 (native)\n", filepath.Join(debian, "source/format"), 0644, meta)
	for _, exe := range meta.Executables {
		install := filepath.Join(debian, meta.ExeName(exe)+".install")
		docs := filepath.Join(debian, meta.ExeName(exe)+".docs")
		build.Render("build/deb.install", install, 0644, exe)
		build.Render("build/deb.docs", docs, 0644, exe)
	}

	return pkgdir
}

func gomobileTool(subcmd string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(GOBIN, "gomobile"), subcmd)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = []string{
		"GOPATH=" + build.GOPATH(),
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOPATH=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	return cmd
}

type mavenMetadata struct {
	Version      string
	Package      string
	Develop      bool
	Contributors []mavenContributor
}

type mavenContributor struct {
	Name  string
	Email string
}

func newMavenMetadata(env build.Environment) mavenMetadata {
	// Collect the list of authors from the repo root
	contribs := []mavenContributor{}
	if authors, err := os.Open("AUTHORS"); err == nil {
		defer authors.Close()

		scanner := bufio.NewScanner(authors)
		for scanner.Scan() {
			// Skip any whitespace from the authors list
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			// Split the author and insert as a contributor
			re := regexp.MustCompile("([^<]+) <(.+)>")
			parts := re.FindStringSubmatch(line)
			if len(parts) == 3 {
				contribs = append(contribs, mavenContributor{Name: parts[1], Email: parts[2]})
			}
		}
	}
	// Render the version and package strings
	version := build.VERSION()
	if isUnstableBuild(env) {
		version += "-SNAPSHOT"
	}
	return mavenMetadata{
		Version:      version,
		Package:      "cnts-" + version,
		Develop:      isUnstableBuild(env),
		Contributors: contribs,
	}
}

type podMetadata struct {
	Version      string
	Commit       string
	Archive      string
	Contributors []podContributor
}

type podContributor struct {
	Name  string
	Email string
}

func newPodMetadata(env build.Environment, archive string) podMetadata {
	// Collect the list of authors from the repo root
	contribs := []podContributor{}
	if authors, err := os.Open("AUTHORS"); err == nil {
		defer authors.Close()

		scanner := bufio.NewScanner(authors)
		for scanner.Scan() {
			// Skip any whitespace from the authors list
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			// Split the author and insert as a contributor
			re := regexp.MustCompile("([^<]+) <(.+)>")
			parts := re.FindStringSubmatch(line)
			if len(parts) == 3 {
				contribs = append(contribs, podContributor{Name: parts[1], Email: parts[2]})
			}
		}
	}
	version := build.VERSION()
	if isUnstableBuild(env) {
		version += "-unstable." + env.Buildnum
	}
	return podMetadata{
		Archive:      archive,
		Version:      version,
		Commit:       env.Commit,
		Contributors: contribs,
	}
}

// Cross compilation
// 跨平台编译
func doXgo(cmdline []string) {
	var (
		alltools = flag.Bool("alltools", false, `Flag whether we're building all known tools, or only on in particular`)
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	// Make sure xgo is available for cross compilation
	gogetxgo := goTool("get", "github.com/karalabe/xgo")
	build.MustRun(gogetxgo)

	// If all tools building is requested, build everything the builder wants
	args := append(buildFlags(env), flag.Args()...)

	if *alltools {
		args = append(args, []string{"--dest", GOBIN}...)
		for _, res := range allToolsArchiveFiles {
			if strings.HasPrefix(res, GOBIN) {
				// Binary tool found, cross build it explicitly
				args = append(args, "./"+filepath.Join("cmd", filepath.Base(res)))
				xgo := xgoTool(args)
				build.MustRun(xgo)
				args = args[:len(args)-1]
			}
		}
		return
	}
	// Otherwise xxecute the explicit cross compilation
	path := args[len(args)-1]
	args = append(args[:len(args)-1], []string{"--dest", GOBIN, path}...)

	xgo := xgoTool(args)
	build.MustRun(xgo)
}

func xgoTool(args []string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(GOBIN, "xgo"), args...)
	cmd.Env = []string{
		"GOPATH=" + build.GOPATH(),
		"GOBIN=" + GOBIN,
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOPATH=") || strings.HasPrefix(e, "GOBIN=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	return cmd
}
