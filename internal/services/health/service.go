package health

import (
	"context"
	"time"
)

type Database interface {
	Ping(ctx context.Context) error
}

type Service struct {
	db Database
}

func NewService(db Database) *Service {
	return &Service{db: db}
}

func (s *Service) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.db.Ping(ctx)
}
