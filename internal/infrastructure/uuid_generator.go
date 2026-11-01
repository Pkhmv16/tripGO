package infrastructure

import "github.com/google/uuid"

type UUIDGenerator struct{}

func (UUIDGenerator) Generate() string {
	return uuid.NewString()
}
