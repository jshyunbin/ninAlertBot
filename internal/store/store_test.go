package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParse(t *testing.T) {
	tests := []struct {
		fixture string
		want    Status
	}{
		{"instock.html", Available},
		{"soldout.html", SoldOut},
		{"unknown.html", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			if got := Parse(readFixture(t, tt.fixture)); got != tt.want {
				t.Errorf("Parse(%s) = %v, want %v", tt.fixture, got, tt.want)
			}
		})
	}
}

func TestParsePrefersExplicitMarkers(t *testing.T) {
	// A page that contains the sold-out text in prose but the real available
	// marker should still parse as Available.
	html := `<p>이전에 품절되었습니다</p><div class="stock available"><span>구매 가능</span></div>`
	if got := Parse(html); got != Available {
		t.Errorf("Parse() = %v, want Available", got)
	}
}

func TestHTTPCheckerCheck(t *testing.T) {
	instock := readFixture(t, "instock.html")
	soldout := readFixture(t, "soldout.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/avail":
			w.Write([]byte(instock))
		case "/gone":
			w.Write([]byte(soldout))
		case "/500":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &HTTPChecker{Client: srv.Client(), BaseURL: srv.URL, UserAgent: "test"}

	cases := []struct {
		slug    string
		want    Status
		wantErr bool
	}{
		{"avail", Available, false},
		{"gone", SoldOut, false},
		{"500", Unknown, true},
	}
	for _, tc := range cases {
		got, err := c.Check(context.Background(), tc.slug)
		if (err != nil) != tc.wantErr {
			t.Errorf("Check(%s) err = %v, wantErr %v", tc.slug, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("Check(%s) = %v, want %v", tc.slug, got, tc.want)
		}
	}
}

func TestProductURL(t *testing.T) {
	c := &HTTPChecker{BaseURL: "https://store.nintendo.co.kr/"}
	if got, want := c.ProductURL("beeskb6aakor"), "https://store.nintendo.co.kr/beeskb6aakor"; got != want {
		t.Errorf("ProductURL() = %q, want %q", got, want)
	}
}
