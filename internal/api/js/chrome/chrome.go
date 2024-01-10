// package chrome provides helper functions to execute JS in a Chrome browser
//
// This would ordinarily be done via a Chrome struct but Go does not allow
// generic methods, only generic static functions, producing "method must have no type parameters".
package chrome

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/matrix-org/complement-crypto/internal/api"
)

// Void is a type which can be used when you want to run an async function without returning anything.
// It can stop large responses causing errors "Object reference chain is too long (-32000)"
// when we don't care about the response.
type Void *runtime.RemoteObject

// Run an anonymous async iffe in the browser. Set the type parameter to a basic data type
// which can be returned as JSON e.g string, map[string]any, []string. If you do not want
// to return anything, use chrome.Void. For example:
//
//	result, err := RunAsyncFn[string](t, ctx, "return await getSomeString()")
//	void, err := RunAsyncFn[chrome.Void](t, ctx, "doSomething(); await doSomethingElse();")
func RunAsyncFn[T any](t *testing.T, ctx context.Context, js string) (*T, error) {
	t.Helper()
	out := new(T)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`(async () => {`+js+`})()`, &out, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MustRunAsyncFn is RunAsyncFn but fails the test if an error is returned when executing.
//
// Run an anonymous async iffe in the browser. Set the type parameter to a basic data type
// which can be returned as JSON e.g string, map[string]any, []string. If you do not want
// to return anything, use chrome.Void
func MustRunAsyncFn[T any](t *testing.T, ctx context.Context, js string) *T {
	t.Helper()
	result, err := RunAsyncFn[T](t, ctx, js)
	if err != nil {
		api.Fatalf(t, "MustRunAsyncFn: %s", err)
	}
	return result
}
