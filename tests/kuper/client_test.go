package kuper_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kparser/internal/config"
	"kparser/internal/kuper"
)

func newTestClient(t *testing.T, cfg config.Config) *kuper.Client {
	t.Helper()
	c := kuper.NewClientWithHTTP(cfg, &http.Client{Timeout: 5 * time.Second})
	c.SetRetryBaseDelay(0)
	return c
}

func defaultCfg(serverURL string) config.Config {
	return config.Config{
		BaseURL:            serverURL,
		AuthPath:           "/auth/token",
		StoresPathTemplate: "/api/v1/retail-chains/{retail_chain_slug}/stores/{merchant_store_id}",
		CategoriesURLTmpl:  "{base_url}/api/v1/content/categories?merchant_store_id={merchant_store_id}",
		ProductsURLTmpl:    "{base_url}/api/v1/content/products?store={merchant_store_id}&cat={category_id}&limit={limit}",
		ProductURLTmpl:     "https://kuper.ru/product/{id}",
		RetailChainSlug:    "chetverochka",
		MerchantStoreID:    "4896",
		ExpectedStoreAddr:  "Москва, Тверская",
		ClientID:           "id",
		ClientSecret:       "secret",
		PerCategoryLimit:   20,
	}
}

func TestClient_Authenticate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" || r.Method != http.MethodPost {
			t.Errorf("неверный auth-запрос: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != "client_credentials" {
			t.Errorf("неверный grant_type: %s", form.Get("grant_type"))
		}
		if form.Get("client_id") != "id" || form.Get("client_secret") != "secret" {
			t.Errorf("неверные креденшены")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"TOKEN_123"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatalf("ожидался успех, получили ошибку: %v", err)
	}
}

func TestClient_Authenticate_FallbackTokenField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"FALLBACK"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, defaultCfg(srv.URL))
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatalf("ожидался успех: %v", err)
	}
}

func TestClient_Authenticate_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad creds", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := newTestClient(t, defaultCfg(srv.URL))
	if err := c.Authenticate(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка 401")
	}
}

func TestClient_ValidateStore_Success(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		_, _ = w.Write([]byte(`{
			"data": {
				"merchant_store_id":"4896",
				"location":{"full_address":"г. Москва, Тверская ул., 1"}
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	c.SetAccessToken("T")
	if err := c.ValidateStore(context.Background()); err != nil {
		t.Fatalf("ожидался успех: %v", err)
	}
	if got != "/api/v1/retail-chains/chetverochka/stores/4896" {
		t.Errorf("неверный путь запроса: %q", got)
	}
}

func TestClient_ValidateStore_WrongAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"location":{"full_address":"СПб, Невский, 100"}}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	c.SetAccessToken("T")
	err := c.ValidateStore(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка несовпадения адреса")
	}
	if !strings.Contains(err.Error(), "выбран не тот магазин") {
		t.Errorf("неверная ошибка: %v", err)
	}
}

func TestClient_FetchCategories_SendsBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T" {
			t.Errorf("отсутствует Bearer-токен: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"1001","name":"Молочная"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	c.SetAccessToken("T")
	got, err := c.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1001" {
		t.Fatalf("неверный результат: %+v", got)
	}
}

func TestClient_FetchProductsByCategories_AppliesLimit(t *testing.T) {
	var seenLimit, seenCat string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		seenLimit = r.URL.Query().Get("limit")
		seenCat = r.URL.Query().Get("cat")
		_, _ = w.Write([]byte(`{"products":[
			{"id":"a","name":"A","price":10},
			{"id":"b","name":"B","price":20},
			{"id":"c","name":"C","price":30}
		]}`))
	}))
	defer srv.Close()

	cfg := defaultCfg(srv.URL)
	cfg.PerCategoryLimit = 2
	c := newTestClient(t, cfg)
	c.SetAccessToken("T")

	products, err := c.FetchProductsByCategories(context.Background(), []kuper.Category{
		{ID: "1001", Name: "Молочная"},
	})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if seenLimit != "2" {
		t.Errorf("limit не пробрасывается в URL: %q", seenLimit)
	}
	if seenCat != "1001" {
		t.Errorf("category_id не пробрасывается в URL: %q", seenCat)
	}
	if len(products) != 2 {
		t.Fatalf("должно быть применено ограничение 2: %d", len(products))
	}
	if calls.Load() != 1 {
		t.Errorf("ожидался 1 запрос, было %d", calls.Load())
	}
}

func TestClient_DoWithRetry_RetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"1","name":"OK"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	c.SetAccessToken("T")
	got, err := c.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("ретрай должен победить: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("ожидалось 3 попытки, было %d", calls.Load())
	}
	if len(got) != 1 {
		t.Errorf("неверный результат: %+v", got)
	}
}

func TestClient_DoWithRetry_DoesNotRetry4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := newTestClient(t, defaultCfg(srv.URL))
	c.SetAccessToken("T")
	if _, err := c.FetchCategories(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка 400")
	}
	if calls.Load() != 1 {
		t.Errorf("4xx не должны ретраиться, было попыток: %d", calls.Load())
	}
}

func TestClient_FullFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/token":
			_, _ = w.Write([]byte(`{"access_token":"FLOW"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/retail-chains/"):
			_, _ = w.Write([]byte(`{"data":{"location":{"full_address":"г. Москва, Тверская ул., 1"}}}`))
		case r.URL.Path == "/api/v1/content/categories":
			_, _ = w.Write([]byte(`{"data":[
				{"id":"1001","name":"Молочная продукция"},
				{"id":"1002","name":"Хлебобулочные изделия"}
			]}`))
		case r.URL.Path == "/api/v1/content/products":
			cat := r.URL.Query().Get("cat")
			_, _ = w.Write([]byte(`{"products":[{"id":"` + cat + `-1","name":"Товар","price":50}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := defaultCfg(srv.URL)
	cfg.CategoryIDs = []string{"1001", "1002"}
	cfg.PerCategoryLimit = 5

	c := newTestClient(t, cfg)
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := c.ValidateStore(context.Background()); err != nil {
		t.Fatalf("validate: %v", err)
	}
	cats, err := c.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("categories: %v", err)
	}
	selected, err := kuper.SelectCategories(cats, cfg)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	products, err := c.FetchProductsByCategories(context.Background(), selected)
	if err != nil {
		t.Fatalf("products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("ожидалось 2 товара (по 1 на категорию), получено %d", len(products))
	}
	if products[0].URL == "" || !strings.HasPrefix(products[0].URL, "https://kuper.ru/product/") {
		t.Errorf("URL должен сгенерироваться по шаблону: %q", products[0].URL)
	}
}
