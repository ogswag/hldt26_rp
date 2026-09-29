package importers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// PhotoFetchTimeout caps one photo download.
const PhotoFetchTimeout = 10 * time.Second

var (
	ErrPhotoAddress  = errors.New("photo: the link leads to a private or local address")
	ErrPhotoResponse = errors.New("photo: the server did not return an image")
	ErrPhotoSize     = errors.New("photo: larger than 8 MB")
)

// blockedPrefixes are address ranges that are not the public internet.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
}

// PublicAddress reports whether ip is on the public internet.
func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// PhotoFetcher downloads photos from links in an uploaded file.
type PhotoFetcher struct {
	client *http.Client
}

// NewPhotoFetcher builds a fetcher that connects only to public addresses, checked after DNS resolution and on
// every redirect. allowPrivate lifts the check for tests against a local server.
func NewPhotoFetcher(allowPrivate bool) *PhotoFetcher {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !PublicAddress(ap.Addr()) {
				return ErrPhotoAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: PhotoFetchTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	return &PhotoFetcher{client: &http.Client{
		Transport: transport,
		Timeout:   PhotoFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("photo: too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrPhotoAddress
			}
			return nil
		},
	}}
}

// Fetch downloads a photo link and prepares it for the catalog.
func (p *PhotoFetcher) Fetch(ctx context.Context, link string) (Photo, error) {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Photo{}, ErrPhotoAddress
	}
	ctx, cancel := context.WithTimeout(ctx, PhotoFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Photo{}, fmt.Errorf("importers.fetch: %w", err)
	}
	req.Header.Set("Accept", "image/*")
	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrPhotoAddress) {
			return Photo{}, ErrPhotoAddress
		}
		return Photo{}, fmt.Errorf("importers.fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return Photo{}, ErrPhotoResponse
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxPhotoBytes+1))
	if err != nil {
		return Photo{}, fmt.Errorf("importers.fetch: %w", err)
	}
	if len(data) > MaxPhotoBytes {
		return Photo{}, ErrPhotoSize
	}
	return NormalizePhoto(data)
}

// PhotoErrorText says in Russian why a photo link failed.
func PhotoErrorText(err error) string {
	switch {
	case errors.Is(err, ErrPhotoAddress):
		return "Ссылка ведёт на внутренний адрес. Укажите ссылку на фото в интернете."
	case errors.Is(err, ErrPhotoResponse):
		return "По ссылке нет картинки. Проверьте ссылку."
	case errors.Is(err, ErrPhotoSize):
		return "Фото больше 8 МБ. Загрузите его вручную в карточке решения."
	case errors.Is(err, ErrPhotoFormat):
		return "Картинка не в формате JPEG, PNG, WebP или GIF."
	case errors.Is(err, ErrPhotoPixels):
		return "Фото больше 40 мегапикселей."
	case errors.Is(err, context.DeadlineExceeded):
		return "Сайт не ответил за 10 секунд. Загрузите фото вручную в карточке решения."
	}
	return "Не удалось скачать фото. Загрузите его вручную в карточке решения."
}
