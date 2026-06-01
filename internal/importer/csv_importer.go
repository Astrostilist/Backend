// Импорт csv - файла
package importer

import (
	"astroapi/internal/models"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

type PostgresRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

type Repository interface {
	RunImportCSV(ctx context.Context /*input *RuleInput*/, r io.Reader) (*models.ImportResult, error)
}

func NewPostgresRepository(db *sql.DB, logger *zap.Logger) *PostgresRepository {
	return &PostgresRepository{db: db, logger: logger}
}

// RunImportCSV - импорт файла .csv
func (m *PostgresRepository) RunImportCSV(ctx context.Context /* db *sql.DB,*/, r io.Reader) (*models.ImportResult, error) {

	csvReader := csv.NewReader(r)
	csvReader.FieldsPerRecord = 22
	csvReader.ReuseRecord = true

	if _, err := csvReader.Read(); err != nil {
		m.logger.Error("failed to read header: ", zap.Error(err))
		return nil, err
	}

	batchSize := 500
	batch := make([]*models.CatalogProduct, 0, batchSize)
	result := &models.ImportResult{Errors: []models.ErrCatalog{}}

	rowNum := 1

	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, models.ErrCatalog{Row: rowNum, Reason: err.Error()})
			rowNum++
			continue
		}

		prod, errCtl, err := parseAndValidateCatalog(record, rowNum)
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, errCtl...)
			rowNum++
			continue
		}

		batch = append(batch, prod)
		rowNum++

		if len(batch) == batchSize {
			// инфо в лог
			m.logger.Sugar().Infof("flushing batch count %d", batchSize)
			start := time.Now()

			if err := flushBatchCatalog(ctx, m.db, batch); err != nil {
				m.logger.Error("batch insert failed:", zap.Error(err))
				return nil, err
			}

			m.logger.Sugar().Infof("batch flushed duration_ms %v", time.Since(start).Milliseconds())

			result.Imported += len(batch)
			batch = batch[:0]
		}
	}

	lenBatch := len(batch)
	if lenBatch > 0 {
		// инфо в лог
		m.logger.Sugar().Infof("flushing batch count %d", lenBatch)
		start := time.Now()

		if err := flushBatchCatalog(ctx, m.db, batch); err != nil {
			m.logger.Error("final batch insert failed:", zap.Error(err))
			return nil, err
		}

		m.logger.Sugar().Infof("batch flushed duration_ms %v", time.Since(start).Milliseconds())

		result.Imported += len(batch)

	}

	return result, nil
}

/*
func parseAndValidate(record []string, rowNum int) (*models.Product, error) {
	sku := strings.TrimSpace(record[0])
	if sku == "" {
		return nil, fmt.Errorf("строка %d: sku обязателен", rowNum)
	}
	name := record[1]
	description := record[2]

	price, err := strconv.ParseFloat(record[3], 64)
	if err != nil || price <= 0 {
		return nil, fmt.Errorf("строка %d: price = %q должен быть >0", rowNum, record[3])
	}

	var tags []string
	if raw := strings.TrimSpace(record[4]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			return nil, fmt.Errorf("строка %d: tags невалидный JSON: %w", rowNum, err)
		}
	}

	category := record[5]
	return &models.Product{
		SKU: sku, Name: name, Description: description,
		Price: price, Tags: tags, Category: category,
	}, nil
}
*/
// parseAndValidateCatalog - валидация строки csv файла.
// Появилась взамен parseAndValidate, работает с расширенной структурой CatalogProduct/
func parseAndValidateCatalog(record []string, rowNum int) (*models.CatalogProduct, []models.ErrCatalog, error) {
	errCtl := []models.ErrCatalog{}

	// * - обяз-но, price == Базовая
	price, err := strconv.ParseFloat(record[0], 64)
	if err != nil || price <= 0 {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Базовая> (price) должно быть > 0"})
	}

	img := record[1]
	images := strings.Split(img, ";")
	validImg := []string{}
	for _, rowStr := range images {
		_, err := url.Parse(rowStr)
		if err != nil {
			errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Фото> содержит невалидный адрес"})
			continue
		}
		validImg = append(validImg, rowStr)
	}

	name := record[2]
	if name == "" {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Наименование> обязательно"})
	}

	article := record[3]

	category := record[6]

	sku := strings.TrimSpace(record[9])
	if sku == "" {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <XML ID> (SKU) обязательно"})
	}

	extProductID := strings.TrimSpace(record[13])
	if extProductID == "" {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Внешний ID> обязательно"})
	}

	urlProd := record[18]
	if urlProd == "" {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Ссылка в магазине> обязательно"})
	}

	_, err = url.Parse(urlProd)
	if err != nil {
		errCtl = append(errCtl, models.ErrCatalog{Row: rowNum, Reason: "поле <Фото> содержит невалидный адрес"})
		urlProd = ""
	}

	// в зависимости от успеха валидации файла формируется return
	if len(errCtl) == 0 {
		return &models.CatalogProduct{
			Article:      article,
			Category:     category,
			ExtProductID: extProductID,
			Images:       validImg,
			Name:         name,
			Price:        price,
			SKU:          sku,
			URL:          urlProd,
		}, errCtl, nil
	}

	return &models.CatalogProduct{
		Article:      article,
		Category:     category,
		ExtProductID: extProductID,
		Images:       validImg,
		Name:         name,
		Price:        price,
		SKU:          sku,
		URL:          urlProd,
	}, errCtl, models.ErrValidateCatalog

}

/*
func flushBatch(ctx context.Context, db *sql.DB, products []*models.Product) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO products (sku, name, description, price, tags, category)
        VALUES ($1, $2, $3, $4, $5::jsonb, $6)
        ON CONFLICT (sku) DO UPDATE SET
            name = EXCLUDED.name,
            description = EXCLUDED.description,
            price = EXCLUDED.price,
            tags = EXCLUDED.tags,
            category = EXCLUDED.category
    `)
	if err != nil {
		return err
	}
	defer func() {
		_ = stmt.Close()
	}()

	for _, p := range products {
		tagsJSON, _ := json.Marshal(p.Tags)
		_, err := stmt.ExecContext(ctx, p.SKU, p.Name, p.Description, p.Price, string(tagsJSON), p.Category)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
*/
// flushBatchCatalog - добавляет распарсенные строки csv в БД.
func flushBatchCatalog(ctx context.Context, db *sql.DB, products []*models.CatalogProduct) error {
	var err error
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO products (sku, ext_product_id, title, article, category, price, url, images)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (sku) DO UPDATE SET
            ext_product_id = EXCLUDED.ext_product_id,    
            title = EXCLUDED.title,
            article = EXCLUDED.article,
            category = EXCLUDED.category,
            price = EXCLUDED.price,
            url = EXCLUDED.url,
            images = EXCLUDED.images,
            updated_at = NOW()           
    `)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() {
		if closerErr := stmt.Close(); closerErr != nil && err == nil {
			err = fmt.Errorf("failed to close statement: %w", closerErr)
		}
	}()

	for i, p := range products {
		// Валидация обязательных полей
		if p.SKU == "" {
			return fmt.Errorf("product at index %d has empty SKU", i)
		}
		if p.ExtProductID == "" {
			return fmt.Errorf("product %s has empty ext_product_id", p.SKU)
		}
		if p.Name == "" {
			return fmt.Errorf("product %s has empty title", p.SKU)
		}

		args := []any{
			p.SKU,
			p.ExtProductID,
			p.Name,
			p.Article,
			p.Category,
			p.Price,
			p.URL,
			pq.Array(p.Images),
		}

		if _, err = stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("failed to insert product %s: %w", p.SKU, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
