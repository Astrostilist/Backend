package models

import "errors"

var (
	ErrValidateCatalog = errors.New("ошибка при валидации строк каталога")
)

// TODO устар.
type Product struct {
	SKU         string
	Name        string
	Description string
	Price       float64
	Tags        []string
	Category    string
	Rating      float64
}

type ImportResult struct {
	Imported int          `json:"imported"`
	Skipped  int          `json:"skipped"`
	Errors   []ErrCatalog `json:"errors"`
}

type ErrCatalog struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

// CatalogProduct - появился на основании реального файла csv. Многие поля отсутствовали в ТЗ.
// Поэтому структура
type CatalogProduct struct {
	Ext_product_id string   // Внешний ID - уникален в глобальном смысле
	SKU            string   // XML ID  // TODO должно рассчитываться отдельно (уникален в пределах системы)
	Name           string   // Название
	Price          float64  // Закупочная цена торгового предложения
	Category       string   // Группа
	Images         []string // Фото
	Url            string   // Ссылка в магазине

	Article string //

	Tags   []string
	Rating float64
}
