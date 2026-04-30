package kuper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"kparser/internal/config"
)

func SelectCategories(all []Category, cfg config.Config) ([]Category, error) {
	if len(all) == 0 {
		return nil, errors.New("список категорий пуст")
	}

	byID := make(map[string]Category, len(all))
	for _, c := range all {
		byID[strings.ToLower(strings.TrimSpace(c.ID))] = c
	}

	used := make(map[string]struct{})
	selected := make([]Category, 0, len(cfg.CategoryIDs)+len(cfg.CategoryNames))

	for _, id := range cfg.CategoryIDs {
		key := strings.ToLower(strings.TrimSpace(id))
		if c, ok := byID[key]; ok {
			if _, exists := used[c.ID]; !exists {
				selected = append(selected, c)
				used[c.ID] = struct{}{}
			}
		}
	}

	for _, name := range cfg.CategoryNames {
		name = strings.ToLower(strings.TrimSpace(name))
		for _, c := range all {
			if strings.Contains(strings.ToLower(c.Name), name) {
				if _, exists := used[c.ID]; !exists {
					selected = append(selected, c)
					used[c.ID] = struct{}{}
				}
			}
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("не удалось выбрать категории по фильтрам id=%v names=%v", cfg.CategoryIDs, cfg.CategoryNames)
	}

	return selected, nil
}

func LoadDemoData(categoriesPath, productsPath string) ([]Category, []Product, error) {
	rawCategories, err := os.ReadFile(categoriesPath)
	if err != nil {
		return nil, nil, err
	}
	rawProducts, err := os.ReadFile(productsPath)
	if err != nil {
		return nil, nil, err
	}

	var categories []Category
	if err := json.Unmarshal(rawCategories, &categories); err != nil {
		return nil, nil, err
	}
	var products []Product
	if err := json.Unmarshal(rawProducts, &products); err != nil {
		return nil, nil, err
	}

	return categories, products, nil
}

func FilterDemoProductsByCategories(products []Product, categories []Category, perCategoryLimit int) []Product {
	selected := make(map[string]Category, len(categories))
	for _, c := range categories {
		selected[c.ID] = c
	}

	counts := make(map[string]int, len(categories))
	out := make([]Product, 0, perCategoryLimit*len(categories))

	for _, p := range products {
		if _, ok := selected[p.CategoryID]; !ok {
			continue
		}
		if counts[p.CategoryID] >= perCategoryLimit {
			continue
		}
		out = append(out, p)
		counts[p.CategoryID]++
	}
	return out
}
