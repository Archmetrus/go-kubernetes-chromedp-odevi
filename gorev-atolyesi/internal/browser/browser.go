package browser

import (
	"context"
	"github.com/chromedp/chromedp"
	"os"
	"time"
)

// Her örnek ayrı, geçici bir tarayıcı profili açar.
func New() (context.Context, func()) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	if p := os.Getenv("CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	alloc, ca := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cb := chromedp.NewContext(alloc)
	ctx, ct := context.WithTimeout(ctx, 30*time.Second)
	return ctx, func() { ct(); cb(); ca() }
}
