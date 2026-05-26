package models

import (
	"errors"
	"time"
)

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
	Ext_product_id string   `json:"ext_product_id,omitzero"` // Внешний ID - уникален в глобальном смысле
	SKU            string   `json:"sku,omitzero"`            // XML ID  // TODO должно рассчитываться отдельно (уникален в пределах системы)
	Name           string   `json:"name,omitzero"`           // Название
	Price          float64  `json:"price,omitzero"`          // Закупочная цена торгового предложения
	Category       string   `json:"category,omitzero"`       // Группа
	Images         []string `json:"images,omitzero"`         // Фото
	Url            string   `json:"url,omitzero"`            // Ссылка в магазине

	Article string `json:"article,omitzero"`

	Tags   []string `json:"tags,omitzero"`
	Rating float64  `json:"rating,omitzero"`

	CreatedAt time.Time `json:"created_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}
