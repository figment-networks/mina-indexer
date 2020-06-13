package store

import (
	"github.com/figment-networks/coda-indexer/model"
)

// HeightsStore handles operations on heights
type HeightsStore struct {
	baseStore
}

// Last returns a last height record
func (s HeightsStore) Last() (*model.Height, error) {
	result := &model.Height{}

	err := s.db.
		Order("height DESC").
		First(result).
		Error

	return result, checkErr(err)
}

// LastSuccessful returns the last successful height record
func (s HeightsStore) LastSuccessful() (*model.Height, error) {
	result := &model.Height{}

	err := s.db.
		Where("status = ?", model.HeightStatusOK).
		Order("height DESC").
		First(result).
		Error

	return result, checkErr(err)
}

// StatusCounts returns height sync statuses with counts
func (s HeightsStore) StatusCounts() ([]model.HeightStatusCount, error) {
	result := []model.HeightStatusCount{}

	err := s.db.
		Raw(sqlHeightsReport).
		Scan(&result).
		Error

	return result, err
}

var (
	sqlHeightsReport = `
		SELECT status, COUNT(1) AS num
		FROM heights
		WHERE status != ''
		GROUP BY status
		ORDER BY num DESC`
)