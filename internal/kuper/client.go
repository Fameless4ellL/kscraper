package kuper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"kparser/internal/config"
)

type Client struct {
	cfg            config.Config
	httpClient     *http.Client
	accessToken    string
	retryBaseDelay time.Duration
}

func (c *Client) SetRetryBaseDelay(d time.Duration) {
	c.retryBaseDelay = d
}

func (c *Client) SetAccessToken(token string) {
	c.accessToken = token
}

func NewClient(cfg config.Config) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 50
	transport.IdleConnTimeout = 45 * time.Second

	if strings.TrimSpace(cfg.ProxyURL) != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("некорректный proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		retryBaseDelay: 400 * time.Millisecond,
	}, nil
}

func NewClientWithHTTP(cfg config.Config, httpClient *http.Client) *Client {
	return &Client{cfg: cfg, httpClient: httpClient, retryBaseDelay: 400 * time.Millisecond}
}

func (c *Client) Authenticate(ctx context.Context) error {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)

	authURL := strings.TrimRight(c.cfg.BaseURL, "/") + c.cfg.AuthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	body, err := c.doWithRetry(req, 3)
	if err != nil {
		return err
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("не удалось распарсить ответ auth: %w", err)
	}
	if tokenResp.AccessToken == "" && tokenResp.Token == "" {
		return errors.New("в ответе auth нет access_token/token")
	}
	if tokenResp.AccessToken != "" {
		c.accessToken = tokenResp.AccessToken
	} else {
		c.accessToken = tokenResp.Token
	}
	return nil
}

func (c *Client) ValidateStore(ctx context.Context) error {
	urlPath := FillTemplate(c.cfg.StoresPathTemplate, map[string]string{
		"retail_chain_slug": c.cfg.RetailChainSlug,
		"merchant_store_id": c.cfg.MerchantStoreID,
	})
	storeURL := JoinURL(c.cfg.BaseURL, urlPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, storeURL, nil)
	if err != nil {
		return err
	}
	c.authorize(req)

	body, err := c.doWithRetry(req, 3)
	if err != nil {
		return err
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	address := strings.ToLower(strings.TrimSpace(ExtractStoreAddress(raw)))
	expected := strings.ToLower(strings.TrimSpace(c.cfg.ExpectedStoreAddr))
	if address == "" {
		return errors.New("адрес магазина не найден в ответе API")
	}
	if !strings.Contains(address, expected) {
		return fmt.Errorf("выбран не тот магазин: в API '%s', ожидалось '%s'", address, expected)
	}
	return nil
}

func (c *Client) FetchCategories(ctx context.Context) ([]Category, error) {
	urlStr := FillTemplate(c.cfg.CategoriesURLTmpl, map[string]string{
		"base_url":          c.cfg.BaseURL,
		"merchant_store_id": c.cfg.MerchantStoreID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)

	body, err := c.doWithRetry(req, 3)
	if err != nil {
		return nil, err
	}

	return ParseCategories(body)
}

func (c *Client) FetchProductsByCategories(ctx context.Context, categories []Category) ([]Product, error) {
	out := make([]Product, 0, len(categories)*c.cfg.PerCategoryLimit)

	for _, cat := range categories {
		urlStr := FillTemplate(c.cfg.ProductsURLTmpl, map[string]string{
			"base_url":          c.cfg.BaseURL,
			"merchant_store_id": c.cfg.MerchantStoreID,
			"category_id":       cat.ID,
			"limit":             strconv.Itoa(c.cfg.PerCategoryLimit),
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		c.authorize(req)

		body, err := c.doWithRetry(req, 3)
		if err != nil {
			return nil, fmt.Errorf("категория %s (%s): %w", cat.Name, cat.ID, err)
		}

		items, err := ParseProducts(body, cat, c.cfg.ProductURLTmpl)
		if err != nil {
			return nil, fmt.Errorf("категория %s (%s): %w", cat.Name, cat.ID, err)
		}
		if len(items) > c.cfg.PerCategoryLimit {
			items = items[:c.cfg.PerCategoryLimit]
		}
		out = append(out, items...)
	}

	return out, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "kuper-parser/1.0")
}

func (c *Client) doWithRetry(req *http.Request, attempts int) ([]byte, error) {
	var bodyBytes []byte
	if req.Body != nil {
		buf, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		bodyBytes = buf
	}

	var lastErr error
	for i := 1; i <= attempts; i++ {
		cloned := req.Clone(req.Context())
		if bodyBytes != nil {
			cloned.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			cloned.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		resp, err := c.httpClient.Do(cloned)
		if err != nil {
			lastErr = err
			c.sleepBackoff(i)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			c.sleepBackoff(i)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, nil
		}
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, trimBody(body))
			c.sleepBackoff(i)
			continue
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	return nil, fmt.Errorf("запрос не выполнен после %d попыток: %w", attempts, lastErr)
}

func (c *Client) sleepBackoff(attempt int) {
	if c.retryBaseDelay <= 0 {
		return
	}
	d := time.Duration(attempt*attempt) * c.retryBaseDelay
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	time.Sleep(d)
}

func ParseCategories(raw []byte) ([]Category, error) {
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}

	items, ok := ExtractArray(node, "data", "categories", "items", "results")
	if !ok {
		return nil, errors.New("массив категорий не найден")
	}

	out := make([]Category, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := FirstString(m, "id", "category_id", "slug")
		name := FirstString(m, "name", "title", "category_name")
		if id == "" || name == "" {
			continue
		}
		out = append(out, Category{ID: id, Name: name})
	}
	if len(out) == 0 {
		return nil, errors.New("не удалось извлечь ни одной категории")
	}
	return out, nil
}

func ParseProducts(raw []byte, category Category, productURLTemplate string) ([]Product, error) {
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}

	items, ok := ExtractArray(node, "data", "products", "items", "results")
	if !ok {
		return nil, errors.New("массив товаров не найден")
	}

	out := make([]Product, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}

		id := FirstString(m, "id", "product_id", "sku")
		name := FirstString(m, "name", "title")
		if id == "" || name == "" {
			continue
		}
		price := FirstNumber(m, "price", "final_price", "current_price")
		if price <= 0 {
			price = findNestedPrice(m)
		}
		if price <= 0 {
			continue
		}

		link := FirstString(m, "url", "link", "product_url")
		if link == "" {
			link = FillTemplate(productURLTemplate, map[string]string{"id": id})
		}

		out = append(out, Product{
			CategoryID:   category.ID,
			CategoryName: category.Name,
			ID:           id,
			Name:         name,
			Price:        price,
			URL:          link,
		})
	}
	return out, nil
}

func ExtractStoreAddress(node any) string {
	root, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	data := root
	if v, ok := root["data"].(map[string]any); ok {
		data = v
	}
	if loc, ok := data["location"].(map[string]any); ok {
		return FirstString(loc, "full_address", "address")
	}
	return FirstString(data, "full_address", "address")
}

func ExtractArray(node any, keys ...string) ([]any, bool) {
	switch v := node.(type) {
	case []any:
		return v, true
	case map[string]any:
		for _, k := range keys {
			if arr, ok := v[k].([]any); ok {
				return arr, true
			}
		}
		for _, k := range keys {
			if nested, ok := v[k].(map[string]any); ok {
				if arr, ok := ExtractArray(nested, keys...); ok {
					return arr, true
				}
			}
		}
	}
	return nil, false
}

func FirstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			case float64:
				return strconv.FormatFloat(t, 'f', -1, 64)
			}
		}
	}
	return ""
}

func FirstNumber(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return round2(t)
			case int:
				return float64(t)
			case string:
				t = strings.ReplaceAll(strings.TrimSpace(t), ",", ".")
				if t == "" {
					continue
				}
				n, err := strconv.ParseFloat(t, 64)
				if err == nil {
					return round2(n)
				}
			}
		}
	}
	return 0
}

func findNestedPrice(m map[string]any) float64 {
	for _, key := range []string{"price", "prices", "cost"} {
		if nested, ok := m[key].(map[string]any); ok {
			val := FirstNumber(nested, "value", "amount", "current", "final")
			if val > 0 {
				return val
			}
		}
	}
	return 0
}

func FillTemplate(tmpl string, values map[string]string) string {
	out := tmpl
	for k, v := range values {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

func JoinURL(baseURL, p string) string {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + p
	}
	u.Path = path.Join(u.Path, p)
	return u.String()
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func trimBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= 400 {
		return s
	}
	return s[:400] + "..."
}
