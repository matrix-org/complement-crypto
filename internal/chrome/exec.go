package chrome

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/matrix-org/complement/must"
)

func MustExecuteInto[T any](t *testing.T, ctx context.Context, js string) T {
	t.Helper()
	out, err := ExecuteInto[T](t, ctx, js)
	must.NotError(t, js, err)
	if out == nil {
		t.Fatalf("MustExecuteInto: output was nil. JS: %s", js)
	}
	return *out
}

func ExecuteInto[T any](t *testing.T, ctx context.Context, js string) (*T, error) {
	t.Helper()
	//t.Log(js)
	out := new(T)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(js, &out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func AwaitExecute(t *testing.T, ctx context.Context, js string) error {
	var r *runtime.RemoteObject // stop large responses causing errors "Object reference chain is too long (-32000)"
	//t.Log(js)
	return chromedp.Run(ctx,
		chromedp.Evaluate(js, &r, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
}

func AwaitExecuteInto[T any](t *testing.T, ctx context.Context, js string) (*T, error) {
	t.Helper()
	//t.Log(js)
	out := new(T)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(js, &out, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func MustAwaitExecute(t *testing.T, ctx context.Context, js string) {
	t.Helper()
	err := AwaitExecute(t, ctx, js)
	must.NotError(t, js, err)
}

func MustExecute(t *testing.T, ctx context.Context, js string) {
	t.Helper()
	var r *runtime.RemoteObject // stop large responses causing errors "Object reference chain is too long (-32000)"
	//t.Log(js)
	err := chromedp.Run(ctx,
		chromedp.Evaluate(js, &r),
	)
	must.NotError(t, js, err)
}
