package chrome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// We only spin up a single chrome browser for all tests.
// Each test hosts the JS SDK HTML/JS on a random high-numbered port and opens it in a new tab
// for test isolation.
var (
	browserInstance   *Browser
	browserInstanceMu = &sync.Mutex{}
	origins           *Origins
)

// GlobalBrowser returns the browser singleton, making it if needed.
func GlobalBrowser() (*Browser, error) {
	browserInstanceMu.Lock()
	defer browserInstanceMu.Unlock()
	if browserInstance == nil {
		var err error
		browserInstance, err = NewBrowser()
		origins = NewOrigins()
		return browserInstance, err
	}
	return browserInstance, nil
}

type Browser struct {
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

func (b *Browser) NewTab(baseJSURL string, onConsoleLog func(s string)) (*Tab, error) {
	tabCtx, closeTab := chromedp.NewContext(b.Ctx)
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(baseJSURL),
	)
	if err != nil {
		return nil, fmt.Errorf("NewTab: failed to navigate to %s: %s", baseJSURL, err)
	}

	// Listen for console logs for debugging, and to communicate live updates
	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				s, err := strconv.Unquote(string(arg.Value))
				if err != nil {
					s = string(arg.Value)
				}
				onConsoleLog(s)
			}
		}
	})

	return &Tab{
		BaseURL: baseJSURL,
		Ctx:     tabCtx,
		browser: b,
		cancel:  closeTab,
	}, nil
}

// For clients which want persistent storage, we need to ensure when the browser
// starts up a 2nd+ time we serve the same URL so the browser uses the same origin
type Origins struct {
	clientToBaseURL map[string]string
	mu              *sync.RWMutex
}

func NewOrigins() *Origins {
	return &Origins{
		clientToBaseURL: make(map[string]string),
		mu:              &sync.RWMutex{},
	}
}

func (o *Origins) StoreBaseURL(userID, deviceID, baseURL string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.clientToBaseURL[userID+deviceID] = baseURL
}

func (o *Origins) GetBaseURL(userID, deviceID string) (baseURL string) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.clientToBaseURL[userID+deviceID]
}
