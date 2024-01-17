package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/matrix-org/complement/ct"
)

// PrepareGoScript takes a template filename from `./tests/templates` and injects the templateData given
// as a Go template. It then runs the file through `go build` and prepares a command to run the resulting
// binary. Returns the prepared command and a close function to delete the script and binary. The script
// will not execute until you call cmd.Start() or cmd.Run().
func PrepareGoScript(t ct.TestLike, templatePath string, templateData any) (*exec.Cmd, func()) {
	_, templateFilename := filepath.Split(templatePath)
	tmpl, err := template.New(templateFilename).ParseFiles("./go_templates/" + templatePath)
	if err != nil {
		ct.Fatalf(t, "failed to parse template %s : %s", templateFilename, err)
	}
	scriptFile, err := os.CreateTemp("./go_templates", "script_*.go")
	if err != nil {
		ct.Fatalf(t, "failed to open temporary file: %s", err)
	}
	defer scriptFile.Close()
	if err = tmpl.ExecuteTemplate(scriptFile, templateFilename, templateData); err != nil {
		ct.Fatalf(t, "failed to execute template to file: %s", err)
	}
	// TODO: should we build output to the random number?
	// e.g go build -o ./templates/script ./templates/script_3523965439.go
	cmd := exec.Command("go", "build", "-o", "./go_templates/script", scriptFile.Name())
	t.Logf(cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build script %s: %s", scriptFile.Name(), err)
	}
	return exec.Command("./go_templates/script"), func() {
		os.Remove(scriptFile.Name())
		os.Remove("./go_templates/script")
	}
}
