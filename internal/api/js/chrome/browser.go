package chrome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// We only spin up a single chrome browser for all tests.
// Each test hosts the JS SDK HTML/JS on a random high-numbered port and opens it in a new tab
// for test isolation.
var (
	browserInstance   *Browser
	browserInstanceMu = &sync.Mutex{}
)

// GlobalBrowser returns the browser singleton, making it if needed.
func GlobalBrowser() (*Browser, error) {
	browserInstanceMu.Lock()
	defer browserInstanceMu.Unlock()
	if browserInstance == nil {
		var err error
		browserInstance, err = NewBrowser()
		return browserInstance, err
	}
	return browserInstance, nil
}

type Browser struct {
	BaseURL         string
	Cancel          func()
	Ctx             context.Context // topmost chromedp context
	ctxCancel       func()
	execAllocCancel func()
}

// Create and run a new Chrome browser.
func NewBrowser() (*Browser, error) {
	ansiRedForeground := "\x1b[31m"
	ansiResetForeground := "\x1b[39m"

	colorifyError := func(format string, args ...any) {
		format = ansiRedForeground + time.Now().Format(time.RFC3339) + " " + format + ansiResetForeground
		fmt.Printf(format, args...)
	}
	opts := chromedp.DefaultExecAllocatorOptions[:]
	os.Mkdir("chromedp", os.ModePerm) // ignore errors to allow repeated runs
	wd, _ := os.Getwd()
	userDir := filepath.Join(wd, "chromedp")
	opts = append(opts,
		chromedp.UserDataDir(userDir),
	)
	// increase the WS timeout from 20s (default) to 30s as we see timeouts with 20s in CI
	opts = append(opts, chromedp.WSURLReadTimeout(30*time.Second))

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithBrowserOption(
		chromedp.WithBrowserLogf(colorifyError), chromedp.WithBrowserErrorf(colorifyError), //chromedp.WithBrowserDebugf(log.Printf),
	))

	browser := &Browser{
		Ctx:             ctx,
		ctxCancel:       cancel,
		execAllocCancel: allocCancel,
	}
	return browser, chromedp.Run(ctx)
}

func (b *Browser) Close() {
	b.ctxCancel()
	b.execAllocCancel()
}
