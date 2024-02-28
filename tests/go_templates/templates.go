package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/matrix-org/complement/ct"
)

// PrepareGoScript takes a template filepath from `./tests/go_templates` and injects the templateData given
// as a Go template. It then runs the file through `go build` and prepares a command to run the resulting
// binary. Returns the prepared command and a close function to delete the script and binary. The script
// will not execute until you call cmd.Start() or cmd.Run().
func PrepareGoScript(t ct.TestLike, templatePath string, templateData any) (*exec.Cmd, func()) {
	//  ./tests/go_templates  -+--loads---> ./tests/go_templates/testFoo/test.go
	//                         |                 Contains Go Templates
	//                         |
	//                       executes
	//                         |
	//                         |
	//                         `-----> ./tests/go_templates/script_12345/script{.go}
	//                                    Contains Go script source and binary
	//
	// There were a few iterations on the directory structure for this worth pointing out to future readers.
	// These are the constraints/walls hit:
	// - We want to have a semi-functional IDE auto-complete functionality in the templates. This means the
	//   templates have to be `.go` despite `.gotmpl` being slightly more accurate. We primarily want the
	//   syntax highlighting and automatic import functionality to aid dev UX.
	// - We can't put script.go in the same directory as the template as that then breaks IDE functionality
	//   due to having 2 func main()s.
	// - We can't put script.go in the ./go_templates directory reliably. If the test panics and does not
	//   call the cleanup function then the filesystem is left in a broken state such that subsequent `go test`
	//   calls won't work due to package mismatches (package main on the script vs package templates in this file).
	// - Therefore, we need a temporary directory. We don't want this hardcoded because we want to support
	//   concurrent calls to PrepareGoScript, hence using os.MkdirTemp which adds random numbers to the directory name.
	// - We can't put the temporary directory in `/tmp` because the scripts import packages from complement-crypto.
	//   If it's put in /tmp then these imports cannot be resolved: it has to be relative to this project.
	// - Therefore, we put the temp directory inside ./go_templates.

	// load the template
	_, templateFilename := filepath.Split(templatePath)
	tmpl, err := template.New(templateFilename).ParseFiles("./go_templates/" + templatePath)
	if err != nil {
		ct.Fatalf(t, "failed to parse template %s : %s", templateFilename, err)
	}

	// make a temporary directory where we will build the script to
	tmpDir, err := os.MkdirTemp("./go_templates", "script_*")
	if err != nil {
		ct.Fatalf(t, "failed to create temp script directory: %s", err)
	}

	// Create the Go script by executing the template into it to fill in placeholder values.
	scriptFile, err := os.Create(filepath.Join(tmpDir, "script.go"))
	if err != nil {
		ct.Fatalf(t, "failed to open temporary file: %s", err)
	}
	defer scriptFile.Close()
	if err = tmpl.ExecuteTemplate(scriptFile, templateFilename, templateData); err != nil {
		ct.Fatalf(t, "failed to execute template to file: %s", err)
	}

	// build the script into a binary
	// e.g go build -o ./go_templates/script_102939/script ./go_templates/script_102939/script.go
	binaryPath := filepath.Join(tmpDir, "script")
	cmd := exec.Command("go", "build", "-o", binaryPath, scriptFile.Name())
	t.Logf(cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build script %s: %s", scriptFile.Name(), err)
	}
	return exec.Command(binaryPath), func() {
		// remove the temporary directory on cleanup
		os.RemoveAll(tmpDir)
	}
}
