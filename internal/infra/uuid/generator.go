package uuid

import "github.com/google/uuid"

type Generator struct{}

func (Generator) Generate() string {
	return uuid.NewString()
}
