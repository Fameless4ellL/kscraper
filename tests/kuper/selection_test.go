package kuper_test

import (
	"path/filepath"
	"testing"

	"kparser/internal/config"
	"kparser/internal/kuper"
)

func sampleCategories() []kuper.Category {
	return []kuper.Category{
		{ID: "1001", Name: "Молочная продукция"},
		{ID: "1002", Name: "Хлебобулочные изделия"},
		{ID: "1003", Name: "Фрукты"},
	}
}

func TestSelectCategories_ByID(t *testing.T) {
	cfg := config.Config{CategoryIDs: []string{"1001", "1003"}}
	got, err := kuper.SelectCategories(sampleCategories(), cfg)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 2 || got[0].ID != "1001" || got[1].ID != "1003" {
		t.Errorf("неверный результат: %+v", got)
	}
}

func TestSelectCategories_ByNameSubstring(t *testing.T) {
	cfg := config.Config{CategoryNames: []string{"молочная", "хлеб"}}
	got, err := kuper.SelectCategories(sampleCategories(), cfg)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидалось 2, получено %d (%+v)", len(got), got)
	}
}

func TestSelectCategories_Dedup(t *testing.T) {
	cfg := config.Config{
		CategoryIDs:   []string{"1001"},
		CategoryNames: []string{"Молочная"},
	}
	got, err := kuper.SelectCategories(sampleCategories(), cfg)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("дубль должен быть отфильтрован: %+v", got)
	}
}

func TestSelectCategories_NotFound(t *testing.T) {
	cfg := config.Config{CategoryNames: []string{"электроника"}}
	if _, err := kuper.SelectCategories(sampleCategories(), cfg); err == nil {
		t.Fatal("ожидалась ошибка, когда ничего не найдено")
	}
}

func TestSelectCategories_EmptyInput(t *testing.T) {
	cfg := config.Config{CategoryIDs: []string{"1"}}
	if _, err := kuper.SelectCategories(nil, cfg); err == nil {
		t.Fatal("ожидалась ошибка для пустого списка категорий")
	}
}

func TestLoadDemoData(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	categoriesPath := filepath.Join(root, "categories_demo.json")
	productsPath := filepath.Join(root, "products_demo.json")

	categories, products, err := kuper.LoadDemoData(categoriesPath, productsPath)
	if err != nil {
		t.Fatalf("ошибка чтения demo: %v", err)
	}
	if len(categories) == 0 || len(products) == 0 {
		t.Fatalf("demo пустые: %d категорий, %d товаров", len(categories), len(products))
	}
}

func TestFilterDemoProductsByCategories(t *testing.T) {
	products := []kuper.Product{
		{CategoryID: "1001", Name: "Молоко", Price: 100, URL: "u1"},
		{CategoryID: "1001", Name: "Кефир", Price: 90, URL: "u2"},
		{CategoryID: "1001", Name: "Сметана", Price: 120, URL: "u3"},
		{CategoryID: "1002", Name: "Хлеб", Price: 80, URL: "u4"},
		{CategoryID: "1003", Name: "Яблоки", Price: 200, URL: "u5"},
	}
	categories := []kuper.Category{
		{ID: "1001", Name: "Молочная"},
		{ID: "1002", Name: "Хлеб"},
	}

	got := kuper.FilterDemoProductsByCategories(products, categories, 2)
	if len(got) != 3 {
		t.Fatalf("ожидалось 3 (2 молочки + 1 хлеб), получено %d", len(got))
	}
	count := map[string]int{}
	for _, p := range got {
		count[p.CategoryID]++
	}
	if count["1001"] != 2 {
		t.Errorf("в молочке должно быть 2 (лимит): %d", count["1001"])
	}
	if count["1002"] != 1 {
		t.Errorf("в хлебе должно быть 1: %d", count["1002"])
	}
	if count["1003"] != 0 {
		t.Errorf("исключённая категория не должна попасть: %d", count["1003"])
	}
}
