// Импорт csv - файла
package importer

import (
	"astroapi/internal/models"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

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
	csvReader.FieldsPerRecord = 6
	csvReader.ReuseRecord = true

	if _, err := csvReader.Read(); err != nil {
		m.logger.Error("failed to read header: ", zap.Error(err))
		return nil, err
	}

	batchSize := 500
	batch := make([]*models.Product, 0, batchSize)
	result := &models.ImportResult{Errors: []string{}}

	rowNum := 1

	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("строка %d: %v", rowNum, err))
			rowNum++
			continue
		}

		prod, err := parseAndValidate(record, rowNum)
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, err.Error())
			rowNum++
			continue
		}

		batch = append(batch, prod)
		rowNum++

		if len(batch) == batchSize {
			if err := flushBatch(ctx, m.db, batch); err != nil {
				m.logger.Error("batch insert failed:", zap.Error(err))
				return nil, err
			}
			result.Imported += len(batch)
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := flushBatch(ctx, m.db, batch); err != nil {
			m.logger.Error("final batch insert failed:", zap.Error(err))
			return nil, err
		}
		result.Imported += len(batch)
	}

	return result, nil
}

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
