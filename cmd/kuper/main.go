package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"kparser/internal/config"
	"kparser/internal/kuper"
	"kparser/internal/writer"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("cfg: %v\n", cfg)

	ctx := context.Background()

	var categories []kuper.Category
	var products []kuper.Product
	var liveClient *kuper.Client

	switch cfg.Mode {
	case config.ModeDemo:
		categories, products, err = kuper.LoadDemoData(cfg.DemoCategoriesPath, cfg.DemoProductsPath)
		if err != nil {
			log.Fatalf("не удалось загрузить demo-данные: %v", err)
		}
	case config.ModeLive:
		client, clientErr := kuper.NewClient(cfg)
		if clientErr != nil {
			log.Fatal(clientErr)
		}
		liveClient = client

		if err = client.Authenticate(ctx); err != nil {
			log.Fatalf("ошибка аутентификации: %v", err)
		}
		if err = client.ValidateStore(ctx); err != nil {
			log.Fatalf("ошибка проверки магазина/адреса: %v", err)
		}

		categories, err = client.FetchCategories(ctx)
		if err != nil {
			log.Fatalf("ошибка получения категорий: %v", err)
		}
	default:
		log.Fatalf("неподдерживаемый режим: %s", cfg.Mode)
	}

	selectedCategories, err := kuper.SelectCategories(categories, cfg)
	if err != nil {
		log.Fatalf("ошибка выбора категорий: %v", err)
	}

	if cfg.Mode == config.ModeLive {
		products, err = liveClient.FetchProductsByCategories(ctx, selectedCategories)
		if err != nil {
			log.Fatalf("ошибка сбора товаров: %v", err)
		}
	} else {
		products = kuper.FilterDemoProductsByCategories(products, selectedCategories, cfg.PerCategoryLimit)
	}

	if len(products) == 0 {
		log.Fatal("товары не найдены по выбранным категориям")
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		log.Fatalf("не удалось создать output-директорию: %v", err)
	}

	if err := writer.WriteCSV(cfg.OutputCSV, products); err != nil {
		log.Fatalf("ошибка записи CSV: %v", err)
	}
	if err := writer.WriteJSON(cfg.OutputJSON, products); err != nil {
		log.Fatalf("ошибка записи JSON: %v", err)
	}

	fmt.Printf("Готово. Собрано товаров: %d\n", len(products))
	fmt.Printf("CSV: %s\n", cfg.OutputCSV)
	fmt.Printf("JSON: %s\n", cfg.OutputJSON)
}
