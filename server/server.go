package server

import (
	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/wikipedia"
)

type Server struct {
	db        *database.Database
	ai        ai.Client
	wikipedia *wikipedia.Wikipedia
}

func NewServer(db *database.Database, ai ai.Client, wikipedia *wikipedia.Wikipedia) *Server {
	return &Server{
		db:        db,
		ai:        ai,
		wikipedia: wikipedia,
	}
}
