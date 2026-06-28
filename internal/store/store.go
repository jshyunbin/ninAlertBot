// Package store fetches Nintendo Korea store product pages and decides whether
// a product is currently purchasable.
package store

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Status is the availability of a product.
type Status int

const (
	// Unknown means neither the available nor the sold-out marker was found,
	// which usually signals a page-layout change. It is never treated as
	// purchasable.
	Unknown Status = iota
	// SoldOut means the product is currently not purchasable (품절).
	SoldOut
	// Available means the product can be purchased right now (구매 가능).
	Available
)

func (s Status) String() string {
	switch s {
	case SoldOut:
		return "sold_out"
	case Available:
		return "available"
	default:
		return "unknown"
	}
}

// DefaultBaseURL is the Nintendo Korea store origin.
const DefaultBaseURL = "https://store.nintendo.co.kr"

// DefaultUserAgent is a realistic desktop browser UA so the store serves the
// fully rendered HTML.
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Checker reports the availability of a product identified by its slug.
type Checker interface {
	Check(ctx context.Context, slug string) (Status, error)
}

// HTTPChecker is a Checker backed by HTTP requests to the live store.
type HTTPChecker struct {
	Client    *http.Client
	BaseURL   string
	UserAgent string
}

// NewHTTPChecker returns an HTTPChecker with sensible defaults.
func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{
		Client:    &http.Client{Timeout: 20 * time.Second},
		BaseURL:   DefaultBaseURL,
		UserAgent: DefaultUserAgent,
	}
}

// ProductURL returns the public URL for a product slug.
func (c *HTTPChecker) ProductURL(slug string) string {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	return strings.TrimRight(base, "/") + "/" + slug
}

// Check fetches the product page and parses its availability.
func (c *HTTPChecker) Check(ctx context.Context, slug string) (Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ProductURL(slug), nil)
	if err != nil {
		return Unknown, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9")

	resp, err := c.Client.Do(req)
	if err != nil {
		return Unknown, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Unknown, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, slug)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB cap
	if err != nil {
		return Unknown, err
	}
	return Parse(string(body)), nil
}

// Parse decides availability from a product page's HTML.
//
// The store renders a server-side stock element:
//
//	sold out: <div class="stock unavailable"...><span>품절</span></div>
//	in stock: <div class="stock available"...><span>구매 가능</span></div>
//
// We key on the class because the cart button (장바구니) is present in both states.
func Parse(html string) Status {
	switch {
	case strings.Contains(html, `class="stock available"`):
		return Available
	case strings.Contains(html, `class="stock unavailable"`):
		return SoldOut
	default:
		return Unknown
	}
}
