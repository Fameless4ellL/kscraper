package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	ModeLive string = "live"
	ModeDemo string = "demo"
)

type Config struct {
	Mode               string
	ProxyURL           string
	BaseURL            string
	AuthPath           string
	StoresPathTemplate string
	CategoriesURLTmpl  string
	ProductsURLTmpl    string
	ProductURLTmpl     string
	RetailChainSlug    string
	MerchantStoreID    string
	ExpectedStoreAddr  string
	ClientID           string
	ClientSecret       string
	PerCategoryLimit   int
	CategoryIDs        []string
	CategoryNames      []string
	OutputDir          string
	OutputCSV          string
	OutputJSON         string
	DemoCategoriesPath string
	DemoProductsPath   string
}

// FromEnv loads configuration from .env file and environment variables
func FromEnv() (Config, error) {
	// Load .env file if it exists, ignore error if it doesn't
	_ = godotenv.Load()

	cfg := Config{
		Mode:               getEnv("APP_MODE", ModeDemo),
		ProxyURL:           getEnv("PROXY_URL", ""),
		BaseURL:            strings.TrimRight(getEnv("BASE_URL", "https://api.kuper.ru"), "/"),
		AuthPath:           getEnv("AUTH_PATH", "/api/v1/auth/token"),
		StoresPathTemplate: getEnv("STORES_PATH_TEMPLATE", "/api/v1/retail-chains/{retail_chain_slug}/stores/{merchant_store_id}"),
		CategoriesURLTmpl:  getEnv("CATEGORIES_URL_TEMPLATE", "{base_url}/api/v1/content/categories?merchant_store_id={merchant_store_id}"),
		ProductsURLTmpl:    getEnv("PRODUCTS_URL_TEMPLATE", "{base_url}/api/v1/content/products?merchant_store_id={merchant_store_id}&category_id={category_id}&limit={limit}"),
		ProductURLTmpl:     getEnv("PRODUCT_URL_TEMPLATE", "https://kuper.ru/product/{id}"),
		RetailChainSlug:    getEnv("RETAIL_CHAIN_SLUG", ""),
		MerchantStoreID:    getEnv("MERCHANT_STORE_ID", ""),
		ExpectedStoreAddr:  getEnv("EXPECTED_STORE_ADDRESS", ""),
		ClientID:           getEnv("CLIENT_ID", ""),
		ClientSecret:       getEnv("CLIENT_SECRET", ""),
		PerCategoryLimit:   getEnvInt("PER_CATEGORY_LIMIT", 20),
		CategoryIDs:        SplitCSV(getEnv("CATEGORY_IDS", "")),
		CategoryNames:      SplitCSV(getEnv("CATEGORY_NAMES", "")),
		OutputDir:          getEnv("OUTPUT_DIR", "output"),
		DemoCategoriesPath: filepath.Clean(getEnv("DEMO_CATEGORIES_PATH", "testdata/categories_demo.json")),
		DemoProductsPath:   filepath.Clean(getEnv("DEMO_PRODUCTS_PATH", "testdata/products_demo.json")),
	}
	fmt.Printf("cfg: %v\n", cfg)

	// Logic for output file paths
	csvPath := getEnv("OUTPUT_CSV", "")
	if csvPath == "" {
		cfg.OutputCSV = filepath.Join(cfg.OutputDir, "products.csv")
	} else {
		cfg.OutputCSV = filepath.Clean(csvPath)
	}

	jsonPath := getEnv("OUTPUT_JSON", "")
	if jsonPath == "" {
		cfg.OutputJSON = filepath.Join(cfg.OutputDir, "products.json")
	} else {
		cfg.OutputJSON = filepath.Clean(jsonPath)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Helper: Get string from env or default
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

// Helper: Get int from env or default
func getEnvInt(key string, fallback int) int {
	valStr := getEnv(key, "")
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func (c Config) Validate() error {
	if c.Mode != ModeLive && c.Mode != ModeDemo {
		return fmt.Errorf("APP_MODE must be '%s' or '%s'", ModeDemo, ModeLive)
	}
	if c.PerCategoryLimit <= 0 {
		return errors.New("PER_CATEGORY_LIMIT must be > 0")
	}
	if len(c.CategoryIDs) == 0 && len(c.CategoryNames) == 0 {
		return errors.New("at least CATEGORY_IDS or CATEGORY_NAMES must be provided")
	}

	if c.Mode == ModeLive {
		checks := map[string]string{
			"PROXY_URL":              c.ProxyURL,
			"BASE_URL":               c.BaseURL,
			"AUTH_PATH":              c.AuthPath,
			"RETAIL_CHAIN_SLUG":      c.RetailChainSlug,
			"MERCHANT_STORE_ID":      c.MerchantStoreID,
			"EXPECTED_STORE_ADDRESS": c.ExpectedStoreAddr,
			"CLIENT_ID":              c.ClientID,
			"CLIENT_SECRET":          c.ClientSecret,
		}
		for key, val := range checks {
			if val == "" {
				return fmt.Errorf("missing required variable for live mode: %s", key)
			}
		}
	}
	return nil
}

func SplitCSV(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
